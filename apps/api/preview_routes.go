package main

import (
	"errors"
	"net/http"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func registerPreviewRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, pulls *pullrequests.Store, runs *checkruns.Store, store *previews.Store, authStore *auth.Store, userStore *users.Store, proposalStore *proposals.Store, decisionStore *decisions.Store, issueStore *issues.Store, activityStore *activities.Store) {
	project := func(p previews.Preview) previews.Preview {
		if run, e := runs.Get(p.RepositoryID, p.PullRequestID, p.BuildRunID); e == nil {
			p.State = run.State
		}
		return p
	}
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, authStore, r.PathValue("id"))
		if !ok {
			return
		}
		participant := false
		if authenticated {
			repository, _ := catalog.GetByID(r.PathValue("id"))
			collaborator, _ := catalog.HasCollaborator(actor.UserID, r.PathValue("id"))
			participant = repository.OwnerID == actor.UserID || collaborator
		}
		pull, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, e) {
			return
		}
		list, e := store.List(pull.RepositoryID, pull.ID, pull.SourceCommitID)
		if e != nil {
			writeAPIError(w, 500, "internal_error", "previews unavailable")
			return
		}
		for i := range list {
			list[i] = project(list[i])
			if !participant {
				list[i].Invitations, list[i].AudienceEvents, list[i].Feedback = nil, nil, nil
			}
		}
		writeJSON(w, 200, map[string]any{"previews": list})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/invitations", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "preview_owner_required", "only the repository owner can manage preview guests")
			return
		}
		var in struct {
			UserID     string    `json:"user_id"`
			SourceKind string    `json:"source_kind"`
			SourceID   string    `json:"source_id"`
			Role       string    `json:"role"`
			ExpiresAt  time.Time `json:"expires_at"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		p, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("preview_id"))
		if err != nil {
			writeAPIError(w, 404, "preview_not_found", "preview not found")
			return
		}
		usersToInvite, err := resolvePreviewAudience(in.UserID, in.SourceKind, in.SourceID, p.RepositoryID, userStore, proposalStore, decisionStore, issueStore)
		if err != nil {
			writeAPIError(w, 422, "preview_audience_invalid", err.Error())
			return
		}
		if !containsString(p.Definition.Access.Actions, in.Role) {
			writeAPIError(w, 422, "preview_role_restricted", "the preview definition does not allow this role")
			return
		}
		for _, userID := range usersToInvite {
			p, err = store.Invite(p.RepositoryID, p.PullRequestID, p.ID, actor.UserID, userID, in.Role, in.SourceKind, in.SourceID, in.ExpiresAt)
			if err != nil {
				writeAPIError(w, 422, "preview_invitation_invalid", "invitation must be named, bounded to 30 days, and allowed by policy")
				return
			}
			target := userID
			recordActivity(activityStore, catalog, activities.Event{Kind: "preview.invited", ActorID: actor.UserID, RepositoryID: p.RepositoryID, ResourceType: "preview", ResourceID: p.ID, ResourceTitle: "Change preview", TargetUserID: &target})
		}
		writeJSON(w, 201, previewAudience(p, time.Now().UTC()))
	})
	mux.HandleFunc("DELETE /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/invitations/{invitation_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "preview_owner_required", "only the repository owner can manage preview guests")
			return
		}
		p, err := store.Revoke(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("preview_id"), r.PathValue("invitation_id"), actor.UserID)
		if err != nil {
			writeAPIError(w, 404, "preview_invitation_not_found", "preview invitation not found")
			return
		}
		writeJSON(w, 200, previewAudience(p, time.Now().UTC()))
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/audience", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		p, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("preview_id"))
		if err != nil {
			writeAPIError(w, 404, "preview_not_found", "preview not found")
			return
		}
		_ = actor
		writeJSON(w, 200, previewAudience(p, time.Now().UTC()))
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/feedback", func(w http.ResponseWriter, r *http.Request) {
		actor, invitation, p, ok := authorizePreviewGuest(w, r, catalog, store, authStore)
		if !ok {
			return
		}
		if invitation.Role != "feedback" || !containsString(p.Definition.Access.Actions, "feedback") {
			writeAPIError(w, 403, "preview_feedback_restricted", "this invitation does not allow feedback")
			return
		}
		var in struct {
			Body string `json:"body"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if len(strings.TrimSpace(in.Body)) < 1 || utf8.RuneCountInString(in.Body) > 4000 {
			writeAPIError(w, 422, "preview_feedback_invalid", "feedback must be 1-4000 characters")
			return
		}
		if _, err := store.AddFeedback(p.RepositoryID, p.PullRequestID, p.ID, actor.UserID, invitation.ID, in.Body); err != nil {
			writeAPIError(w, 409, "preview_feedback_restricted", "the invitation is no longer active")
			return
		}
		recordActivity(activityStore, catalog, activities.Event{Kind: "preview.feedback", ActorID: actor.UserID, RepositoryID: p.RepositoryID, ResourceType: "preview", ResourceID: p.ID, ResourceTitle: "Change preview feedback"})
		writeJSON(w, 201, map[string]any{"recorded": true, "actor_id": actor.UserID, "role": invitation.Role})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/previews", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		pull, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, e) {
			return
		}
		if pull.Status != pullrequests.Open {
			writeAPIError(w, 409, "pull_request_not_open", "pull request is not open")
			return
		}
		repository, e := git.Open(pull.RepositoryID)
		if e != nil {
			writeAPIError(w, 503, "preview_source_unavailable", "preview source unavailable")
			return
		}
		data, e := exec.Command("git", "--git-dir="+repository.Path(), "show", pull.SourceCommitID+":"+previews.ConfigPath).Output()
		if e != nil {
			writeAPIError(w, 422, "preview_definition_missing", "candidate must contain .vivarium/preview.json")
			return
		}
		config, hash, e := previews.ParseConfig(data)
		if e != nil {
			writeAPIError(w, 422, "invalid_preview_definition", e.Error())
			return
		}
		quoted := strings.ReplaceAll(config.OutputPath, "'", "'\\''")
		command := config.Build + " && test -d '" + quoted + "' && cp -R '" + quoted + "'/. \"$VIVARIUM_OUTPUT\"/"
		createdRuns, e := runs.CreateRequested(pull.RepositoryID, pull.ID, pull.SourceCommitID, []checkruns.Definition{{Name: "preview-" + hash[:12], Image: config.Image, Command: command, WorkingDirectory: config.WorkingDirectory, Environment: config.Environment, TimeoutSeconds: config.Resources.TimeoutSeconds, CPUs: config.Resources.CPUs, MemoryMB: config.Resources.MemoryMB, StorageMB: config.Resources.StorageMB}}, actor.UserID)
		if e != nil || len(createdRuns) != 1 {
			writeAPIError(w, 503, "preview_build_unavailable", "preview build unavailable")
			return
		}
		p, e := store.Create(pull.RepositoryID, pull.ID, pull.SourceCommitID, actor.UserID, hash, createdRuns[0].ID, config)
		if e != nil {
			writeAPIError(w, 503, "preview_storage_unavailable", "preview storage unavailable")
			return
		}
		go runs.Execute(createdRuns[0], repository.Path())
		writeJSON(w, 201, p)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/events", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, authStore, r.PathValue("id")); !ok {
			return
		}
		p, e := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("preview_id"))
		if errors.Is(e, previews.ErrNotFound) {
			writeAPIError(w, 404, "preview_not_found", "preview not found")
			return
		}
		events, e := runs.Events(p.RepositoryID, p.PullRequestID, p.BuildRunID, 0)
		if e != nil {
			writeAPIError(w, 503, "preview_logs_unavailable", "preview logs unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"events": events})
	})
	serveContent := func(w http.ResponseWriter, r *http.Request) {
		if _, _, _, ok := authorizePreviewGuest(w, r, catalog, store, authStore); !ok {
			return
		}
		p, e := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("preview_id"))
		if e != nil {
			writeAPIError(w, 404, "preview_not_found", "preview not found")
			return
		}
		run, e := runs.Get(p.RepositoryID, p.PullRequestID, p.BuildRunID)
		if e != nil || run.State != "succeeded" {
			writeAPIError(w, 409, "preview_not_ready", "preview is not ready")
			return
		}
		requested := r.PathValue("asset")
		if requested == "" {
			requested = "index.html"
		}
		requested = path.Clean(strings.TrimPrefix(requested, "/"))
		if requested == "." || requested == ".." || strings.HasPrefix(requested, "../") {
			writeAPIError(w, 404, "preview_asset_not_found", "preview asset not found")
			return
		}
		var selected *checkruns.Artifact
		for i := range run.Artifacts {
			if run.Artifacts[i].Path == requested {
				selected = &run.Artifacts[i]
				break
			}
		}
		if selected == nil {
			writeAPIError(w, 404, "preview_asset_not_found", "preview asset not found")
			return
		}
		file, a, e := runs.OpenArtifact(p.RepositoryID, p.PullRequestID, p.BuildRunID, selected.ID)
		if e != nil {
			writeAPIError(w, 503, "preview_content_unavailable", "preview content unavailable")
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", a.ContentType)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'none'; form-action 'none'; frame-ancestors 'none'; sandbox allow-scripts")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "private, no-store")
		http.ServeContent(w, r, path.Base(a.Path), a.CreatedAt, file)
	}
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/content", serveContent)
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/content/{asset...}", serveContent)
}

func previewAudience(p previews.Preview, now time.Time) map[string]any {
	return map[string]any{"preview_id": p.ID, "revision": p.Revision, "policy": p.Definition.Access, "invitations": p.Invitations, "events": p.AudienceEvents, "feedback": p.Feedback, "effective_access": map[string]any{"active_invitation_count": func() int {
		n := 0
		for _, i := range p.Invitations {
			if i.RevokedAt == nil && i.ExpiresAt.After(now) {
				n++
			}
		}
		return n
	}(), "grants_repository_access": false, "grants_workspace_access": false, "grants_deployment_access": false, "grants_environment_access": false, "credentials_exposed": false, "private_services_exposed": false}}
}
func resolvePreviewAudience(userID, kind, sourceID, repositoryID string, userStore *users.Store, proposalStore *proposals.Store, decisionStore *decisions.Store, issueStore *issues.Store) ([]string, error) {
	set := map[string]bool{}
	if kind == "user" {
		if userID == "" {
			return nil, errors.New("user_id is required")
		}
		set[userID] = true
	} else if kind == "proposal" && proposalStore != nil {
		p, e := proposalStore.Get(repositoryID, sourceID)
		if e != nil {
			return nil, errors.New("proposal not found")
		}
		set[p.AuthorID] = true
		comments, e := proposalStore.ListComments(repositoryID, sourceID)
		if e != nil {
			return nil, e
		}
		for _, c := range comments {
			set[c.AuthorID] = true
		}
	} else if kind == "decision" && decisionStore != nil {
		d, e := decisionStore.Get(sourceID)
		if e != nil || d.RepositoryID != repositoryID {
			return nil, errors.New("decision not found")
		}
		for _, p := range d.Scope.Participants {
			set[p.UserID] = true
		}
	} else if kind == "issue" && issueStore != nil {
		i, e := issueStore.Get(repositoryID, sourceID)
		if e != nil {
			return nil, errors.New("issue not found")
		}
		set[i.ReporterID] = true
		for _, c := range i.Discussion {
			set[c.AuthorID] = true
		}
	} else {
		return nil, errors.New("source_kind must be user, issue, decision, or proposal")
	}
	out := []string{}
	for id := range set {
		if _, e := userStore.Get(id); e != nil {
			return nil, errors.New("audience contains an unknown user")
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}
func authorizePreviewGuest(w http.ResponseWriter, r *http.Request, catalog *repositories.Store, store *previews.Store, authStore *auth.Store) (auth.Credential, previews.Invitation, previews.Preview, bool) {
	actor, ok := authenticateRequest(w, r, authStore, "repositories:read", false)
	if !ok {
		return auth.Credential{}, previews.Invitation{}, previews.Preview{}, false
	}
	repo, e := catalog.GetByID(r.PathValue("id"))
	if e != nil {
		writeAPIError(w, 404, "repository_not_found", "repository not found")
		return actor, previews.Invitation{}, previews.Preview{}, false
	}
	collaborator, _ := catalog.HasCollaborator(actor.UserID, repo.ID)
	p, e := store.Get(repo.ID, r.PathValue("pull_id"), r.PathValue("preview_id"))
	if e != nil {
		writeAPIError(w, 404, "preview_not_found", "preview not found")
		return actor, previews.Invitation{}, previews.Preview{}, false
	}
	if actor.UserID == repo.OwnerID || collaborator {
		return actor, previews.Invitation{Role: "feedback"}, p, true
	}
	updated, inv, e := store.Enter(repo.ID, p.PullRequestID, p.ID, actor.UserID)
	if e != nil {
		writeAPIError(w, 404, "preview_not_found", "preview not found")
		return actor, previews.Invitation{}, previews.Preview{}, false
	}
	return actor, inv, updated, true
}
