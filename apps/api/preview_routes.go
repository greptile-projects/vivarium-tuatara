package main

import (
	"errors"
	"net/http"
	"os/exec"
	"path"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerPreviewRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, pulls *pullrequests.Store, runs *checkruns.Store, store *previews.Store, authStore *auth.Store) {
	project := func(p previews.Preview) previews.Preview {
		if run, e := runs.Get(p.RepositoryID, p.PullRequestID, p.BuildRunID); e == nil {
			p.State = run.State
		}
		return p
	}
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, authStore, r.PathValue("id")); !ok {
			return
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
		}
		writeJSON(w, 200, map[string]any{"previews": list})
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
		createdRuns, e := runs.CreateRequested(pull.RepositoryID, pull.ID, pull.SourceCommitID, []checkruns.Definition{{Name: "preview-" + hash[:12], Image: config.Image, Command: command, WorkingDirectory: config.WorkingDirectory, Environment: config.Environment, TimeoutSeconds: config.Resources.TimeoutSeconds}}, actor.UserID)
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
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/content", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, authStore, r.PathValue("id")); !ok {
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
		var selected *checkruns.Artifact
		for i := range run.Artifacts {
			if run.Artifacts[i].Path == "index.html" {
				selected = &run.Artifacts[i]
				break
			}
		}
		if selected == nil {
			writeAPIError(w, 422, "preview_entrypoint_missing", "preview did not publish index.html")
			return
		}
		file, a, e := runs.OpenArtifact(p.RepositoryID, p.PullRequestID, p.BuildRunID, selected.ID)
		if e != nil {
			writeAPIError(w, 503, "preview_content_unavailable", "preview content unavailable")
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", a.ContentType)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
		http.ServeContent(w, r, path.Base(a.Path), a.CreatedAt, file)
	})
}
