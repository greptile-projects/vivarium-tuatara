package main

import (
	"errors"
	"net/http"
	"os/exec"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/performanceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/performancegoals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerPerformanceEvidenceRoutes(mux *http.ServeMux, gitStore *storage.Store, catalog *repositories.Store, credentials *auth.Store, goals *performancegoals.Store, releaseStore *releases.Store, trials *performanceevidence.Store) {
	mux.HandleFunc("POST /repositories/{id}/performance-trials", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in performanceevidence.Trial
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete sanitized performance trial is required")
			return
		}
		in.RepositoryID, in.CreatedBy = r.PathValue("id"), actor.UserID
		if in.GoalID != "" {
			goal, e := goals.Get(in.GoalID)
			if e != nil || goal.RepositoryID != in.RepositoryID {
				writeAPIError(w, 422, "performance_context_invalid", "the goal is not available in this repository")
				return
			}
		}
		if in.Source.Kind == "release" {
			if releaseStore == nil {
				writeAPIError(w, 503, "performance_context_unavailable", "release evidence is unavailable")
				return
			}
			rel, e := releaseStore.Get(in.RepositoryID, in.Source.ReleaseID)
			if e != nil || rel.CommitID != in.Source.Revision {
				writeAPIError(w, 422, "performance_source_invalid", "the release does not attest the exact revision")
				return
			}
		}
		repo, e := gitStore.Open(in.RepositoryID)
		if e != nil || exec.Command("git", "--git-dir="+repo.Path(), "cat-file", "-e", in.Source.Revision+"^{commit}").Run() != nil {
			writeAPIError(w, 422, "performance_source_invalid", "the exact source revision is unavailable")
			return
		}
		var created performanceevidence.Trial
		e = catalog.WithCurrentParticipant(actor.UserID, in.RepositoryID, func() error { var x error; created, x = trials.Create(in); return x })
		if errors.Is(e, performanceevidence.ErrInvalid) {
			writeAPIError(w, 422, "performance_trial_invalid", "trial evidence must be bounded, complete, sanitized, and internally consistent")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "performance_evidence_unavailable", "performance evidence could not be persisted")
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/performance-trials", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		items, e := trials.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "performance_evidence_unavailable", "performance evidence could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"trials": items})
	})
	mux.HandleFunc("GET /repositories/{id}/performance-trials/{trial_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := trials.Get(r.PathValue("trial_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "performance_trial_not_found", "performance trial not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /repositories/{id}/performance-trials/{trial_id}/compare/{baseline_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		current, e1 := trials.Get(r.PathValue("trial_id"))
		baseline, e2 := trials.Get(r.PathValue("baseline_id"))
		if e1 != nil || e2 != nil || current.RepositoryID != r.PathValue("id") || baseline.RepositoryID != current.RepositoryID {
			writeAPIError(w, 404, "performance_trial_not_found", "performance trial not found")
			return
		}
		writeJSON(w, 200, map[string]any{"baseline": baseline, "current": current, "comparisons": trials.Compare(baseline, current)})
	})
}
