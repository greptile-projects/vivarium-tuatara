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
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

const (
	uploadPackService  = "git-upload-pack"
	receivePackService = "git-receive-pack"
	primaryBranchHook  = `#!/bin/sh
while read -r old new ref
do
	if test "$ref" != "refs/heads/main"
	then
		echo "only refs/heads/main may be updated" >&2
		exit 1
	fi
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("listening on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, newAuthenticatedAppHandler(store, userStore, authStore, repositoryStore)); err != nil {
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
	mux := http.NewServeMux()
	var repositoryCatalog *repositories.Store
	if len(catalogs) > 0 {
		repositoryCatalog = catalogs[0]
	}
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
		registerRepositoryRoutes(mux, repositoryCatalog, authStore)
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
			if _, ok := authorizeGitRepository(w, r, authStore, repositoryCatalog, r.PathValue("remote"), required); !ok {
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
		runGitService(w, r, repo, service, true)
	})
	mux.HandleFunc("POST /git/{remote}/git-upload-pack", func(w http.ResponseWriter, r *http.Request) {
		if authStore != nil {
			if _, ok := authorizeGitRepository(w, r, authStore, repositoryCatalog, r.PathValue("remote"), "git:read"); !ok {
				return
			}
		}
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
		setGitCacheHeaders(w)
		runGitService(w, r, repo, uploadPackService, false)
	})
	mux.HandleFunc("POST /git/{remote}/git-receive-pack", func(w http.ResponseWriter, r *http.Request) {
		if authStore != nil {
			if _, ok := authorizeGitRepository(w, r, authStore, repositoryCatalog, r.PathValue("remote"), "git:write"); !ok {
				return
			}
		}
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
		setGitCacheHeaders(w)
		runGitService(w, r, repo, receivePackService, false)
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

func registerRepositoryRoutes(mux *http.ServeMux, store *repositories.Store, authStore *auth.Store) {
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
		owned, err := store.List(actor.UserID)
		if writeRepositoryError(w, err) {
			return
		}
		page, next, ok := paginate(r, owned, func(repository repositories.Repository) string { return repository.ID })
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
		if actor.UserID != repository.OwnerID {
			writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
			return
		}
		writeJSON(w, http.StatusOK, repository)
	})
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

func authorizeGitRepository(w http.ResponseWriter, r *http.Request, authStore *auth.Store, catalog *repositories.Store, remote, scope string) (auth.Credential, bool) {
	// Handlers without an application catalog are retained for storage-level
	// compatibility tests. Production always supplies the catalog.
	if catalog == nil {
		return authenticateRequest(w, r, authStore, scope, true)
	}
	id, ok := strings.CutSuffix(remote, ".git")
	if !ok || id == "" {
		http.Error(w, "repository not found", http.StatusNotFound)
		return auth.Credential{}, false
	}
	repository, err := catalog.GetByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			http.Error(w, "repository not found", http.StatusNotFound)
		} else {
			http.Error(w, "repository unavailable", http.StatusInternalServerError)
		}
		return auth.Credential{}, false
	}
	if scope == "git:read" && repository.Visibility == repositories.Public {
		return auth.Credential{}, true
	}
	actor, authenticated, valid := authenticateOptionalRequest(w, r, authStore, scope, true)
	if !valid {
		return auth.Credential{}, false
	}
	if !authenticated {
		writeAuthenticationRequired(w, true)
		return auth.Credential{}, false
	}
	if actor.UserID != repository.OwnerID {
		http.Error(w, "repository not found", http.StatusNotFound)
		return auth.Credential{}, false
	}
	return actor, true
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
	values := r.URL.Query()
	if len(values["limit"]) > 1 || len(values["after"]) > 1 {
		return nil, nil, false
	}
	limit := 30
	if raw := values.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return nil, nil, false
		}
		limit = parsed
	}
	start := 0
	if after := values.Get("after"); after != "" {
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
	runGitService(w, r, repo, uploadPackService, advertise)
}

func runGitService(w http.ResponseWriter, r *http.Request, repo *storage.Repository, service string, advertise bool) {
	commandName := strings.TrimPrefix(service, "git-")
	args := []string{commandName, "--stateless-rpc"}
	var removeHooks func()
	if service == receivePackService {
		// Receive-pack applies each requested ref update transactionally. The
		// client distinguishes ordinary progress from explicit force updates,
		// while the hook keeps the remote constrained to its primary branch.
		args = append([]string{
			"-c", "receive.denyNonFastForwards=false",
			"-c", "receive.denyDeletes=false",
			"-c", "receive.denyDeleteCurrent=ignore",
			"-c", "receive.hideRefs=refs/",
			"-c", "receive.hideRefs=!refs/heads/main",
		}, args...)
		if !advertise {
			hooksPath, err := os.MkdirTemp("", "vivarium-receive-hooks-")
			if err != nil {
				log.Printf("prepare %s for repository %s: %v", service, repo.ID(), err)
				return
			}
			removeHooks = func() { _ = os.RemoveAll(hooksPath) }
			defer removeHooks()
			if err := os.WriteFile(hooksPath+"/pre-receive", []byte(primaryBranchHook), 0o700); err != nil {
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
