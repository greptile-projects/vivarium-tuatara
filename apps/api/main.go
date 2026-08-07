package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

const (
	uploadPackService   = "git-upload-pack"
	receivePackService  = "git-receive-pack"
	branchNamespaceHook = `#!/bin/sh
while read -r old new ref
do
	case "$ref" in
		refs/heads/*) ;;
		*)
			echo "only branches may be updated" >&2
			exit 1
			;;
	esac
done
`
	contributorBranchHook = `#!/bin/sh
while read -r old new ref
do
	case "$ref" in
		refs/heads/main)
			echo "contributors may not update the default branch" >&2
			exit 1
			;;
		refs/heads/*) ;;
		*)
			echo "only branches may be updated" >&2
			exit 1
			;;
	esac
done
`
)

func main() {
	root := os.Getenv("GIT_STORAGE_ROOT")
	if root == "" {
		root = "repositories"
	}
	store, err := storage.New(root)
	if err != nil {
		log.Fatal(err)
	}
	userRoot := os.Getenv("USER_STORAGE_ROOT")
	if userRoot == "" {
		userRoot = "users"
	}
	userStore, err := users.New(userRoot)
	if err != nil {
		log.Fatal(err)
	}
	authRoot := os.Getenv("AUTH_STORAGE_ROOT")
	if authRoot == "" {
		authRoot = "credentials"
	}
	authStore, err := auth.New(authRoot)
	if err != nil {
		log.Fatal(err)
	}
	repositoryRoot := os.Getenv("REPOSITORY_STORAGE_ROOT")
	if repositoryRoot == "" {
		repositoryRoot = "repository-records"
	}
	repositoryStore, err := repositories.New(repositoryRoot, store)
	if err != nil {
		log.Fatal(err)
	}
	proposalRoot := os.Getenv("PROPOSAL_STORAGE_ROOT")
	if proposalRoot == "" {
		proposalRoot = "proposals"
	}
	proposalStore, err := proposals.New(proposalRoot)
	if err != nil {
		log.Fatal(err)
	}
	pullRequestRoot := os.Getenv("PULL_REQUEST_STORAGE_ROOT")
	if pullRequestRoot == "" {
		pullRequestRoot = "pull-requests"
	}
	pullRequestStore, err := pullrequests.New(pullRequestRoot, store)
	if err != nil {
		log.Fatal(err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("listening on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, newPlatformHandler(store, userStore, authStore, repositoryStore, proposalStore, pullRequestStore)); err != nil {
		log.Fatal(err)
	}
}

func newHandler(store *storage.Store) http.Handler {
	return newAppHandler(store, nil)
}

func newAppHandler(store *storage.Store, userStore *users.Store) http.Handler {
	return newAuthenticatedAppHandler(store, userStore, nil)
}

func newAuthenticatedAppHandler(store *storage.Store, userStore *users.Store, authStore *auth.Store, catalogs ...*repositories.Store) http.Handler {
	var repositoryCatalog *repositories.Store
	if len(catalogs) > 0 {
		repositoryCatalog = catalogs[0]
	}
	return newPlatformHandler(store, userStore, authStore, repositoryCatalog, nil)
}

func newPlatformHandler(store *storage.Store, userStore *users.Store, authStore *auth.Store, repositoryCatalog *repositories.Store, proposalStore *proposals.Store, pullRequestStores ...*pullrequests.Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	if userStore != nil {
		registerUserRoutes(mux, userStore, authStore)
	}
	if authStore != nil {
		registerAuthRoutes(mux, authStore)
	}
	if authStore != nil && repositoryCatalog != nil {
		registerRepositoryRoutes(mux, store, repositoryCatalog, userStore, authStore)
	}
	if authStore != nil && repositoryCatalog != nil && proposalStore != nil {
		registerProposalRoutes(mux, repositoryCatalog, proposalStore, authStore)
	}
	if authStore != nil && repositoryCatalog != nil && len(pullRequestStores) > 0 && pullRequestStores[0] != nil {
		registerPullRequestRoutes(mux, repositoryCatalog, proposalStore, pullRequestStores[0], authStore)
	}
	mux.HandleFunc("GET /git/{remote}/info/refs", func(w http.ResponseWriter, r *http.Request) {
		service := r.URL.Query().Get("service")
		if service != uploadPackService && service != receivePackService {
			http.Error(w, "unsupported Git service", http.StatusBadRequest)
			return
		}
		required := "git:read"
		if service == receivePackService {
			required = "git:write"
		}
		if authStore != nil {
			if _, _, ok := authorizeGitRepository(w, r, authStore, repositoryCatalog, r.PathValue("remote"), required); !ok {
				return
			}
		}
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-"+service+"-advertisement")
		setGitCacheHeaders(w)
		if _, err := io.WriteString(w, pktLine("# service="+service+"\n")+"0000"); err != nil {
			return
		}
		runGitService(w, r, repo, service, true, false)
	})
	mux.HandleFunc("POST /git/{remote}/git-upload-pack", func(w http.ResponseWriter, r *http.Request) {
		if authStore != nil {
			if _, _, ok := authorizeGitRepository(w, r, authStore, repositoryCatalog, r.PathValue("remote"), "git:read"); !ok {
				return
			}
		}
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
		setGitCacheHeaders(w)
		runGitService(w, r, repo, uploadPackService, false, false)
	})
	mux.HandleFunc("POST /git/{remote}/git-receive-pack", func(w http.ResponseWriter, r *http.Request) {
		contributor := false
		if authStore != nil {
			_, owner, ok := authorizeGitRepository(w, r, authStore, repositoryCatalog, r.PathValue("remote"), "git:write")
			if !ok {
				return
			}
			contributor = !owner
		}
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
		setGitCacheHeaders(w)
		runGitService(w, r, repo, receivePackService, false, contributor)
	})
	return mux
}

type userInput struct {
	Handle      *string `json:"handle"`
	DisplayName *string `json:"display_name"`
}

func registerUserRoutes(mux *http.ServeMux, store *users.Store, authStore *auth.Store) {
	if authStore != nil {
		mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authenticateRequest(w, r, authStore, "", false)
			if !ok {
				return
			}
			user, err := store.Get(actor.UserID)
			if writeUserError(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, user)
		})
	}
	mux.HandleFunc("POST /users", func(w http.ResponseWriter, r *http.Request) {
		var input userInput
		if err := decodeJSON(r, &input); err != nil || input.Handle == nil || input.DisplayName == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "handle and display_name are required")
			return
		}
		var issued auth.IssuedCredential
		user, err := store.CreateWithBootstrap(*input.Handle, *input.DisplayName, func(user users.User) error {
			if authStore == nil {
				return nil
			}
			var issueErr error
			issued, issueErr = authStore.Issue(user.ID, auth.Session, "web session", []string{"credentials:write", "profile:write", "repositories:read", "repositories:write"}, 24*time.Hour)
			return issueErr
		})
		if err != nil && issued.ID != "" {
			if _, revokeErr := authStore.Revoke(issued.UserID, issued.ID); revokeErr != nil {
				log.Printf("revoke credential %s after user bootstrap failure: %v", issued.ID, revokeErr)
			}
		}
		if writeUserError(w, err) {
			return
		}
		w.Header().Set("Location", "/users/"+user.ID)
		if authStore == nil {
			writeJSON(w, http.StatusCreated, user)
			return
		}
		setSessionCookie(w, issued.Token, issued.ExpiresAt)
		writeJSON(w, http.StatusCreated, map[string]any{"user": user, "credential": issued})
	})
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		user, err := store.Get(r.PathValue("id"))
		if writeUserError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, user)
	})
	mux.HandleFunc("PATCH /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		if authStore != nil {
			credential, ok := authenticateRequest(w, r, authStore, "profile:write", false)
			if !ok {
				return
			}
			if credential.UserID != r.PathValue("id") {
				writeAPIError(w, http.StatusForbidden, "forbidden", "credential belongs to another user")
				return
			}
		}
		var input userInput
		if err := decodeJSON(r, &input); err != nil || (input.Handle == nil && input.DisplayName == nil) {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "at least one of handle or display_name is required")
			return
		}
		user, err := store.Patch(r.PathValue("id"), users.ProfilePatch{
			Handle: input.Handle, DisplayName: input.DisplayName,
		})
		if writeUserError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, user)
	})
}

type repositoryInput struct {
	Name *string `json:"name"`
}

type repositoryPatch struct {
	Visibility *string `json:"visibility"`
}

type collaboratorInput struct {
	UserID *string `json:"user_id"`
}

type proposalInput struct {
	Title *string `json:"title"`
	Body  *string `json:"body"`
}

type proposalPatch struct {
	Title  *string `json:"title"`
	Body   *string `json:"body"`
	Status *string `json:"status"`
}

type commentInput struct {
	Body *string `json:"body"`
}

type pullRequestInput struct {
	Title        *string `json:"title"`
	Body         *string `json:"body"`
	SourceBranch *string `json:"source_branch"`
	TargetBranch *string `json:"target_branch"`
	ProposalID   *string `json:"proposal_id"`
}

type reviewInput struct {
	Decision *string `json:"decision"`
}

func registerPullRequestRoutes(mux *http.ServeMux, repositoriesStore *repositories.Store, proposalStore *proposals.Store, store *pullrequests.Store, authStore *auth.Store) {
	mux.HandleFunc("GET /repositories/{id}/pulls", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		all, err := store.List(r.PathValue("id"))
		if writePullRequestError(w, err) {
			return
		}
		page, next, ok := paginate(r, all, func(p pullrequests.PullRequest) string { return p.ID })
		if !ok {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"pull_requests": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input pullRequestInput
		if decodeJSON(r, &input) != nil || input.Title == nil || input.Body == nil || input.SourceBranch == nil || input.TargetBranch == nil {
			writeAPIError(w, 400, "invalid_pull_request", "title, body, source_branch, and target_branch are required")
			return
		}
		if input.ProposalID != nil {
			if proposalStore == nil {
				writeAPIError(w, 400, "invalid_pull_request", "proposal_id is invalid")
				return
			}
			if _, err := proposalStore.Get(r.PathValue("id"), *input.ProposalID); errors.Is(err, proposals.ErrNotFound) {
				writeAPIError(w, 400, "invalid_pull_request", "proposal_id is invalid")
				return
			} else if err != nil {
				log.Printf("proposal storage while creating pull request: %v", err)
				writeAPIError(w, 500, "internal_error", "proposal storage unavailable")
				return
			}
		}
		created, err := store.Create(r.PathValue("id"), actor.UserID, *input.Title, *input.Body, *input.SourceBranch, *input.TargetBranch, input.ProposalID)
		location := "/repositories/" + r.PathValue("id") + "/pulls/" + created.ID
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			w.Header().Set("Location", location)
			writeUncertainMutation(w, created)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		w.Header().Set("Location", location)
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		pullRequest, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, 200, pullRequest)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/synchronize", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		existing, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		if existing.AuthorID != actor.UserID {
			writeAPIError(w, http.StatusNotFound, "pull_request_not_found", "pull request not found")
			return
		}
		updated, err := store.SynchronizeSource(r.PathValue("id"), existing.ID)
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			writeUncertainMutation(w, updated)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, updated)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/commits", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		commits, err := store.Commits(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, 200, map[string]any{"commits": commits})
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/files", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		changes, err := store.Changes(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, 200, map[string]any{"files": changes})
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/merge-readiness", func(w http.ResponseWriter, r *http.Request) {
		_, owner, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		report, err := store.Readiness(r.PathValue("id"), r.PathValue("pull_id"), owner)
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, report)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/merge", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
			return
		}
		merged, err := store.Merge(r.PathValue("id"), r.PathValue("pull_id"), actor.UserID)
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			writeUncertainMutation(w, merged)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		if merged.ProposalID != nil && proposalStore != nil {
			proposal, proposalErr := proposalStore.Get(r.PathValue("id"), *merged.ProposalID)
			if proposalErr == nil && proposal.Status == proposals.Open {
				closed := proposals.Closed
				_, proposalErr = proposalStore.Update(r.PathValue("id"), proposal.ID, proposals.Patch{Status: &closed})
			}
			if errors.Is(proposalErr, proposals.ErrDurabilityUncertain) {
				writeUncertainMutation(w, merged)
				return
			}
			if proposalErr != nil && !errors.Is(proposalErr, proposals.ErrDurabilityUncertain) {
				log.Printf("close linked proposal after merge: %v", proposalErr)
				writeAPIError(w, http.StatusInternalServerError, "internal_error", "linked proposal closure unavailable; retry merge")
				return
			}
		}
		writeJSON(w, http.StatusOK, merged)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		all, err := store.ListComments(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		page, next, ok := paginate(r, all, func(c pullrequests.Comment) string { return c.ID })
		if !ok {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"comments": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var input commentInput
		if decodeJSON(r, &input) != nil || input.Body == nil {
			writeAPIError(w, 400, "invalid_comment", "body is required")
			return
		}
		comment, err := store.AddComment(r.PathValue("id"), r.PathValue("pull_id"), actor.UserID, *input.Body)
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			w.Header().Set("Location", r.URL.Path+"/"+comment.ID)
			writeUncertainMutation(w, comment)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		w.Header().Set("Location", r.URL.Path+"/"+comment.ID)
		writeJSON(w, 201, comment)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/reviews", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		all, err := store.ListReviews(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		page, next, ok := paginate(r, all, func(review pullrequests.Review) string { return review.ID })
		if !ok {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"reviews": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/reviews", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var input reviewInput
		if decodeJSON(r, &input) != nil || input.Decision == nil || (*input.Decision != pullrequests.Approved && *input.Decision != pullrequests.ChangesRequested) {
			writeAPIError(w, 400, "invalid_review", "decision must be approved or changes_requested")
			return
		}
		review, err := store.SetReview(r.PathValue("id"), r.PathValue("pull_id"), actor.UserID, *input.Decision)
		location := r.URL.Path + "/" + review.ID
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			w.Header().Set("Location", location)
			writeUncertainMutation(w, review)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		w.Header().Set("Location", location)
		writeJSON(w, 200, review)
	})
	mux.HandleFunc("DELETE /repositories/{id}/pulls/{pull_id}/reviews/{review_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		review, err := store.WithdrawReview(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("review_id"), actor.UserID)
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			writeUncertainMutation(w, review)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, 200, review)
	})
}

func writePullRequestError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, pullrequests.ErrNotFound):
		writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
	case errors.Is(err, pullrequests.ErrInvalid):
		writeAPIError(w, 400, "invalid_pull_request", "pull request content or branches are invalid")
	case errors.Is(err, pullrequests.ErrBranchNotFound):
		writeAPIError(w, 400, "branch_not_found", "source or target branch does not identify a commit")
	case errors.Is(err, pullrequests.ErrSourceChanged):
		writeAPIError(w, http.StatusConflict, "source_branch_changed", "source branch must be synchronized before review")
	case errors.Is(err, pullrequests.ErrNotReady):
		writeAPIError(w, 409, "pull_request_not_ready", "pull request is not ready to merge")
	default:
		log.Printf("pull request storage: %v", err)
		writeAPIError(w, 500, "internal_error", "pull request storage unavailable")
	}
	return true
}

func registerProposalRoutes(mux *http.ServeMux, repositoriesStore *repositories.Store, store *proposals.Store, authStore *auth.Store) {
	mux.HandleFunc("GET /repositories/{id}/proposals", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		all, err := store.List(r.PathValue("id"))
		if writeProposalError(w, err) {
			return
		}
		page, next, ok := paginate(r, all, func(p proposals.Proposal) string { return p.ID })
		if !ok {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"proposals": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /repositories/{id}/proposals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input proposalInput
		if decodeJSON(r, &input) != nil || input.Title == nil || input.Body == nil {
			writeAPIError(w, 400, "invalid_proposal", "title and body are required")
			return
		}
		proposal, err := store.Create(r.PathValue("id"), actor.UserID, *input.Title, *input.Body)
		if errors.Is(err, proposals.ErrDurabilityUncertain) {
			location := "/repositories/" + r.PathValue("id") + "/proposals/" + proposal.ID
			w.Header().Set("Location", location)
			writeUncertainMutation(w, proposal)
			return
		}
		if writeProposalError(w, err) {
			return
		}
		location := "/repositories/" + r.PathValue("id") + "/proposals/" + proposal.ID
		w.Header().Set("Location", location)
		writeJSON(w, 201, proposal)
	})
	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		proposal, err := store.Get(r.PathValue("id"), r.PathValue("proposal_id"))
		if writeProposalError(w, err) {
			return
		}
		writeJSON(w, 200, proposal)
	})
	mux.HandleFunc("PATCH /repositories/{id}/proposals/{proposal_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		existing, err := store.Get(r.PathValue("id"), r.PathValue("proposal_id"))
		if writeProposalError(w, err) {
			return
		}
		var input proposalPatch
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_proposal", "proposal patch is invalid")
			return
		}
		if existing.AuthorID != actor.UserID && (!owner || input.Title != nil || input.Body != nil || input.Status == nil) {
			writeAPIError(w, 404, "proposal_not_found", "proposal not found")
			return
		}
		updated, err := store.Update(r.PathValue("id"), existing.ID, proposals.Patch{Title: input.Title, Body: input.Body, Status: input.Status})
		if errors.Is(err, proposals.ErrDurabilityUncertain) {
			writeUncertainMutation(w, updated)
			return
		}
		if writeProposalError(w, err) {
			return
		}
		writeJSON(w, 200, updated)
	})
	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		all, err := store.ListComments(r.PathValue("id"), r.PathValue("proposal_id"))
		if writeProposalError(w, err) {
			return
		}
		page, next, ok := paginate(r, all, func(c proposals.Comment) string { return c.ID })
		if !ok {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"comments": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /repositories/{id}/proposals/{proposal_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var input commentInput
		if decodeJSON(r, &input) != nil || input.Body == nil {
			writeAPIError(w, 400, "invalid_comment", "body is required")
			return
		}
		comment, err := store.AddComment(r.PathValue("id"), r.PathValue("proposal_id"), actor.UserID, *input.Body)
		if errors.Is(err, proposals.ErrDurabilityUncertain) {
			w.Header().Set("Location", r.URL.Path+"/"+comment.ID)
			writeUncertainMutation(w, comment)
			return
		}
		if writeProposalError(w, err) {
			return
		}
		w.Header().Set("Location", r.URL.Path+"/"+comment.ID)
		writeJSON(w, 201, comment)
	})
}

func writeUncertainMutation(w http.ResponseWriter, resource any) {
	w.Header().Set("Vivarium-Durability", "uncertain")
	writeJSON(w, http.StatusAccepted, resource)
}

func authorizeRepositoryRead(w http.ResponseWriter, r *http.Request, store *repositories.Store, authStore *auth.Store, id string) (auth.Credential, bool, bool) {
	repository, err := store.GetByID(id)
	if writeRepositoryError(w, err) {
		return auth.Credential{}, false, false
	}
	if repository.Visibility == repositories.Public {
		return auth.Credential{}, false, true
	}
	actor, authenticated, ok := authenticateOptionalRequest(w, r, authStore, "repositories:read", false)
	if !ok {
		return auth.Credential{}, false, false
	}
	if !authenticated {
		writeAuthenticationRequired(w, false)
		return auth.Credential{}, false, false
	}
	collaborator, err := store.HasCollaborator(actor.UserID, id)
	if err != nil {
		writeRepositoryError(w, err)
		return auth.Credential{}, false, false
	}
	if actor.UserID != repository.OwnerID && !collaborator {
		writeAPIError(w, 404, "repository_not_found", "repository not found")
		return auth.Credential{}, false, false
	}
	return actor, true, true
}

func authorizeRepositoryParticipant(w http.ResponseWriter, r *http.Request, store *repositories.Store, authStore *auth.Store, id, scope string) (auth.Credential, bool, bool) {
	actor, ok := authenticateRequest(w, r, authStore, scope, false)
	if !ok {
		return auth.Credential{}, false, false
	}
	repository, err := store.GetByID(id)
	if writeRepositoryError(w, err) {
		return auth.Credential{}, false, false
	}
	owner := actor.UserID == repository.OwnerID
	collaborator, err := store.HasCollaborator(actor.UserID, id)
	if err != nil {
		writeRepositoryError(w, err)
		return auth.Credential{}, false, false
	}
	if !owner && !collaborator {
		writeAPIError(w, 404, "repository_not_found", "repository not found")
		return auth.Credential{}, false, false
	}
	return actor, owner, true
}

func writeProposalError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, proposals.ErrNotFound) {
		writeAPIError(w, 404, "proposal_not_found", "proposal not found")
	} else if errors.Is(err, proposals.ErrInvalid) {
		writeAPIError(w, 400, "invalid_proposal", "proposal content or status is invalid")
	} else {
		log.Printf("proposal storage: %v", err)
		writeAPIError(w, 500, "internal_error", "proposal storage unavailable")
	}
	return true
}

func registerRepositoryRoutes(mux *http.ServeMux, gitStore *storage.Store, store *repositories.Store, userStore *users.Store, authStore *auth.Store) {
	mux.HandleFunc("POST /repositories", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		var input repositoryInput
		if decodeJSON(r, &input) != nil || input.Name == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "name is required")
			return
		}
		repository, err := store.Create(actor.UserID, *input.Name)
		if writeRepositoryError(w, err) {
			return
		}
		w.Header().Set("Location", "/repositories/"+repository.ID)
		writeJSON(w, http.StatusCreated, repository)
	})
	mux.HandleFunc("GET /repositories", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:read", false)
		if !ok {
			return
		}
		accessible, err := store.ListAccessible(actor.UserID)
		if writeRepositoryError(w, err) {
			return
		}
		page, next, ok := paginate(r, accessible, func(repository repositories.Repository) string { return repository.ID })
		if !ok {
			writeAPIError(w, http.StatusBadRequest, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"repositories": page, "next_cursor": next})
	})
	mux.HandleFunc("GET /repositories/{id}", func(w http.ResponseWriter, r *http.Request) {
		repository, err := store.GetByID(r.PathValue("id"))
		if writeRepositoryError(w, err) {
			return
		}
		if repository.Visibility == repositories.Public {
			writeJSON(w, http.StatusOK, repository)
			return
		}
		actor, authenticated, ok := authenticateOptionalRequest(w, r, authStore, "repositories:read", false)
		if !ok {
			return
		}
		if !authenticated {
			writeAuthenticationRequired(w, false)
			return
		}
		collaborator, accessErr := store.HasCollaborator(actor.UserID, repository.ID)
		if accessErr != nil {
			writeRepositoryError(w, accessErr)
			return
		}
		if actor.UserID != repository.OwnerID && !collaborator {
			writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
			return
		}
		writeJSON(w, http.StatusOK, repository)
	})
	registerRepositoryBrowseRoutes(mux, gitStore, store, authStore)
	mux.HandleFunc("PATCH /repositories/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		var input repositoryPatch
		if decodeJSON(r, &input) != nil || input.Visibility == nil || (*input.Visibility != repositories.Private && *input.Visibility != repositories.Public) {
			writeAPIError(w, http.StatusBadRequest, "invalid_repository", "visibility must be private or public")
			return
		}
		repository, err := store.SetVisibility(actor.UserID, r.PathValue("id"), *input.Visibility)
		if writeRepositoryError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, repository)
	})
	mux.HandleFunc("DELETE /repositories/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		if writeRepositoryError(w, store.Delete(actor.UserID, r.PathValue("id"))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /repositories/{id}/collaborators", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		collaborators, err := store.ListCollaborators(actor.UserID, r.PathValue("id"))
		if writeRepositoryError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"collaborators": collaborators})
	})
	mux.HandleFunc("POST /repositories/{id}/collaborators", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		var input collaboratorInput
		if decodeJSON(r, &input) != nil || input.UserID == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_collaborator", "user_id is required")
			return
		}
		if _, err := userStore.Get(*input.UserID); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_collaborator", "user_id must identify an existing user")
			return
		}
		collaborator, err := store.AddCollaborator(actor.UserID, r.PathValue("id"), *input.UserID)
		if writeRepositoryError(w, err) {
			return
		}
		w.Header().Set("Location", "/repositories/"+r.PathValue("id")+"/collaborators/"+collaborator.UserID)
		writeJSON(w, http.StatusCreated, collaborator)
	})
	mux.HandleFunc("DELETE /repositories/{id}/collaborators/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		if writeRepositoryError(w, store.RemoveCollaborator(actor.UserID, r.PathValue("id"), r.PathValue("user_id"))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func writeRepositoryError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
	case errors.Is(err, repositories.ErrNameTaken):
		writeAPIError(w, http.StatusConflict, "repository_name_taken", "repository name is already in use")
	case errors.Is(err, repositories.ErrInvalidName):
		writeAPIError(w, http.StatusBadRequest, "invalid_repository", "repository name is invalid")
	case errors.Is(err, repositories.ErrInvalidCollaborator):
		writeAPIError(w, http.StatusBadRequest, "invalid_collaborator", "repository collaborator is invalid")
	default:
		log.Printf("repository storage: %v", err)
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "repository storage unavailable")
	}
	return true
}

type credentialInput struct {
	Kind      auth.Kind `json:"kind"`
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	ExpiresIn int64     `json:"expires_in"`
}

func registerAuthRoutes(mux *http.ServeMux, store *auth.Store) {
	mux.HandleFunc("GET /auth/credentials", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, store, "credentials:write", false)
		if !ok {
			return
		}
		credentials, err := store.List(actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "internal_error", "credential storage unavailable")
			return
		}
		page, next, valid := paginate(r, credentials, func(credential auth.Credential) string { return credential.ID })
		if !valid {
			writeAPIError(w, http.StatusBadRequest, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"credentials": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /auth/credentials", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, store, "credentials:write", false)
		if !ok {
			return
		}
		var input credentialInput
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_request", "invalid credential request")
			return
		}
		if input.ExpiresIn <= 0 || input.ExpiresIn > int64((90*24*time.Hour)/time.Second) {
			writeAPIError(w, 400, "invalid_credential", "kind, name, scopes, or lifetime is invalid")
			return
		}
		issued, err := store.Issue(actor.UserID, input.Kind, input.Name, input.Scopes, time.Duration(input.ExpiresIn)*time.Second)
		if err != nil {
			writeAPIError(w, 400, "invalid_credential", "kind, name, scopes, or lifetime is invalid")
			return
		}
		writeJSON(w, 201, issued)
	})
	mux.HandleFunc("DELETE /auth/credentials/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, store, "credentials:write", false)
		if !ok {
			return
		}
		if _, err := store.Revoke(actor.UserID, r.PathValue("id")); err != nil {
			writeAPIError(w, 404, "credential_not_found", "credential not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /auth/session", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, store, "credentials:write", false)
		if !ok {
			return
		}
		if _, err := store.Revoke(actor.UserID, actor.ID); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "credential storage unavailable")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "vivarium_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		w.WriteHeader(http.StatusNoContent)
	})
}

func authenticateRequest(w http.ResponseWriter, r *http.Request, store *auth.Store, scope string, git bool) (auth.Credential, bool) {
	credential, authenticated, ok := authenticateOptionalRequest(w, r, store, scope, git)
	if !ok {
		return auth.Credential{}, false
	}
	if !authenticated {
		writeAuthenticationRequired(w, git)
		return auth.Credential{}, false
	}
	return credential, true
}

func authenticateOptionalRequest(w http.ResponseWriter, r *http.Request, store *auth.Store, scope string, git bool) (auth.Credential, bool, bool) {
	token := ""
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		token = strings.TrimPrefix(header, "Bearer ")
	} else if _, password, ok := r.BasicAuth(); ok {
		token = password
	} else if cookie, err := r.Cookie("vivarium_session"); err == nil {
		token = cookie.Value
	}
	if token == "" {
		return auth.Credential{}, false, true
	}
	credential, err := store.Authenticate(token, scope)
	if err != nil {
		if !errors.Is(err, auth.ErrNotFound) {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "credential storage unavailable")
			return auth.Credential{}, false, false
		}
		writeAuthenticationRequired(w, git)
		return auth.Credential{}, false, false
	}
	return credential, true, true
}

func writeAuthenticationRequired(w http.ResponseWriter, git bool) {
	if git {
		w.Header().Set("WWW-Authenticate", `Basic realm="vivarium-git"`)
	} else {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	writeAPIError(w, http.StatusUnauthorized, "unauthorized", "valid authentication is required")
}

func authorizeGitRepository(w http.ResponseWriter, r *http.Request, authStore *auth.Store, catalog *repositories.Store, remote, scope string) (auth.Credential, bool, bool) {
	// Handlers without an application catalog are retained for storage-level
	// compatibility tests. Production always supplies the catalog.
	if catalog == nil {
		actor, ok := authenticateRequest(w, r, authStore, scope, true)
		return actor, true, ok
	}
	id, ok := strings.CutSuffix(remote, ".git")
	if !ok || id == "" {
		http.Error(w, "repository not found", http.StatusNotFound)
		return auth.Credential{}, false, false
	}
	repository, err := catalog.GetByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			http.Error(w, "repository not found", http.StatusNotFound)
		} else {
			http.Error(w, "repository unavailable", http.StatusInternalServerError)
		}
		return auth.Credential{}, false, false
	}
	if scope == "git:read" && repository.Visibility == repositories.Public {
		return auth.Credential{}, false, true
	}
	actor, authenticated, valid := authenticateOptionalRequest(w, r, authStore, scope, true)
	if !valid {
		return auth.Credential{}, false, false
	}
	if !authenticated {
		writeAuthenticationRequired(w, true)
		return auth.Credential{}, false, false
	}
	owner := actor.UserID == repository.OwnerID
	collaborator, accessErr := catalog.HasCollaborator(actor.UserID, id)
	if accessErr != nil {
		http.Error(w, "repository unavailable", http.StatusInternalServerError)
		return auth.Credential{}, false, false
	}
	if !owner && !collaborator {
		http.Error(w, "repository not found", http.StatusNotFound)
		return auth.Credential{}, false, false
	}
	return actor, owner, true
}

func setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: "vivarium_session", Value: token, Path: "/", Expires: expires, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func paginate[T any](r *http.Request, all []T, id func(T) string) ([]T, *string, bool) {
	limit, after, ok := paginationParameters(r)
	if !ok {
		return nil, nil, false
	}
	start := 0
	if after != "" {
		start = -1
		for index, item := range all {
			if id(item) == after {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return nil, nil, false
		}
	}
	end := min(start+limit, len(all))
	page := all[start:end]
	var next *string
	if end < len(all) {
		cursor := id(all[end-1])
		next = &cursor
	}
	return page, next, true
}

func paginationParameters(r *http.Request) (int, string, bool) {
	values := r.URL.Query()
	limitValues, hasLimit := values["limit"]
	afterValues, hasAfter := values["after"]
	if len(limitValues) > 1 || len(afterValues) > 1 || (hasLimit && limitValues[0] == "") || (hasAfter && afterValues[0] == "") {
		return 0, "", false
	}
	limit := 30
	if hasLimit {
		raw := limitValues[0]
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return 0, "", false
		}
		limit = parsed
	}
	after := ""
	if hasAfter {
		after = afterValues[0]
	}
	return limit, after, true
}

func writeUserError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, users.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "user_not_found", "user not found")
	case errors.Is(err, users.ErrHandleTaken):
		writeAPIError(w, http.StatusConflict, "handle_taken", "handle is already in use")
	case errors.Is(err, users.ErrInvalidProfile):
		writeAPIError(w, http.StatusBadRequest, "invalid_profile", err.Error())
	default:
		log.Printf("user storage: %v", err)
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "user storage unavailable")
	}
	return true
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func openRemoteRepository(w http.ResponseWriter, store *storage.Store, remote string) (*storage.Repository, bool) {
	id, ok := strings.CutSuffix(remote, ".git")
	if !ok || id == "" {
		http.Error(w, "repository not found", http.StatusNotFound)
		return nil, false
	}
	repo, err := store.Open(id)
	if errors.Is(err, storage.ErrRepositoryNotFound) || errors.Is(err, storage.ErrInvalidID) {
		http.Error(w, "repository not found", http.StatusNotFound)
		return nil, false
	}
	if err != nil {
		http.Error(w, "repository unavailable", http.StatusInternalServerError)
		return nil, false
	}
	return repo, true
}

func runUploadPack(w http.ResponseWriter, r *http.Request, repo *storage.Repository, advertise bool) {
	runGitService(w, r, repo, uploadPackService, advertise, false)
}

func runGitService(w http.ResponseWriter, r *http.Request, repo *storage.Repository, service string, advertise, contributor bool) {
	commandName := strings.TrimPrefix(service, "git-")
	args := []string{commandName, "--stateless-rpc"}
	var removeHooks func()
	if service == receivePackService {
		// Receive-pack applies each requested ref update transactionally. The
		// client distinguishes ordinary progress from explicit force updates,
		// while the hook keeps writes constrained to branch references.
		args = append([]string{
			"-c", "receive.denyNonFastForwards=false",
			"-c", "receive.denyDeletes=false",
			"-c", "receive.denyDeleteCurrent=ignore",
		}, args...)
		if !advertise {
			hooksPath, err := os.MkdirTemp("", "vivarium-receive-hooks-")
			if err != nil {
				log.Printf("prepare %s for repository %s: %v", service, repo.ID(), err)
				return
			}
			removeHooks = func() { _ = os.RemoveAll(hooksPath) }
			defer removeHooks()
			hook := branchNamespaceHook
			if contributor {
				hook = contributorBranchHook
			}
			if err := os.WriteFile(hooksPath+"/pre-receive", []byte(hook), 0o700); err != nil {
				log.Printf("prepare %s for repository %s: %v", service, repo.ID(), err)
				return
			}
			args = append([]string{"-c", "core.hooksPath=" + hooksPath}, args...)
		}
	}
	if advertise {
		args = append(args, "--advertise-refs")
	}
	args = append(args, repo.Path())
	command := exec.CommandContext(r.Context(), "git", args...)
	// Git services can spawn pack processes. Give the invocation a dedicated
	// process group and cancel the whole group so descendants cannot outlive an
	// abandoned HTTP request.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.Stdout = w
	command.Stderr = os.Stderr
	if !advertise {
		command.Stdin = r.Body
	}
	if protocol := r.Header.Get("Git-Protocol"); protocol != "" && !strings.ContainsAny(protocol, "\x00\r\n") {
		command.Env = append(os.Environ(), "GIT_PROTOCOL="+protocol)
	}
	if err := command.Run(); err != nil {
		log.Printf("serve %s for repository %s: %v", service, repo.ID(), err)
	}
}

func pktLine(payload string) string {
	return fmt.Sprintf("%04x%s", len(payload)+4, payload)
}

func setGitCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
	w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	w.Header().Set("Pragma", "no-cache")
}
