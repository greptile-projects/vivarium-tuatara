package main

import (
	"errors"
	"net/http"
	"os/exec"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/debugworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/regressioninvestigations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
)

func registerRegressionInvestigationRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, investigations *regressioninvestigations.Store, issueStore *issues.Store, supportStore *supportthreads.Store, checkStore *checkruns.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, debugStore *debugworkspaces.Store) {
	actor := func(c auth.Credential) string {
		if c.AgentID != "" {
			return c.AgentID
		}
		return c.UserID
	}
	project := func(v regressioninvestigations.Investigation) regressioninvestigations.Investigation { // Staleness is live and never rewrites retained evidence.
		r, err := git.Open(v.RepositoryID)
		if err != nil {
			v.Diagnostics = append(v.Diagnostics, "repository history is unavailable")
			v.Comparable = false
			return v
		}
		if exec.Command("git", "--git-dir="+r.Path(), "cat-file", "-e", v.KnownGood.Revision+"^{commit}").Run() != nil {
			v.Diagnostics = append(v.Diagnostics, "known-good revision is missing")
			v.Comparable = false
		}
		if exec.Command("git", "--git-dir="+r.Path(), "cat-file", "-e", v.KnownBad.Revision+"^{commit}").Run() != nil {
			v.Diagnostics = append(v.Diagnostics, "known-bad revision is missing")
			v.Comparable = false
		}
		for i := range v.Evidence {
			if v.Evidence[i].Revision != "" && exec.Command("git", "--git-dir="+r.Path(), "cat-file", "-e", v.Evidence[i].Revision+"^{commit}").Run() != nil {
				v.Evidence[i].Stale = true
				v.Evidence[i].Diagnostic = "referenced revision is no longer available"
			}
		}
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/regression-investigations", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, e := investigations.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "regression_investigations_unavailable", "regression investigations could not be read")
			return
		}
		for i := range values {
			values[i] = project(values[i])
		}
		writeJSON(w, 200, map[string]any{"regression_investigations": values})
	})
	mux.HandleFunc("GET /repositories/{id}/regression-investigations/{investigation_id}", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := investigations.Get(r.PathValue("id"), r.PathValue("investigation_id"))
		if e != nil {
			writeAPIError(w, 404, "regression_investigation_not_found", "regression investigation not found")
			return
		}
		writeJSON(w, 200, project(v))
	})
	mux.HandleFunc("POST /repositories/{id}/regression-investigations", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in regressioninvestigations.Investigation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a regression search boundary is required")
			return
		}
		repoID := r.PathValue("id")
		in.RepositoryID = repoID
		in.Diagnostics = []string{}
		repository, catalogErr := catalog.GetByID(repoID)
		if catalogErr != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		for _, ownerID := range in.OwnerIDs {
			participant, participantErr := catalog.HasCollaborator(ownerID, repoID)
			if participantErr != nil || (ownerID != repository.OwnerID && !participant) {
				writeAPIError(w, 422, "regression_owner_invalid", "every owner must be a current repository participant")
				return
			}
		}
		if !validRegressionSource(in.Source, repoID, issueStore, supportStore, checkStore, releaseStore, deploymentStore, debugStore) {
			writeAPIError(w, 422, "regression_source_invalid", "source must resolve to an issue, support thread, failed check, release, deployment, or reproduction in this repository")
			return
		}
		gr, e := git.Open(repoID)
		if e != nil {
			writeAPIError(w, 422, "regression_history_unavailable", "repository history is unavailable")
			return
		}
		resolve := func(b *regressioninvestigations.Boundary) bool {
			if b.Kind == "release" {
				x, e := releaseStore.Get(repoID, b.ResourceID)
				if e != nil || x.CommitID != b.Revision {
					return false
				}
			}
			return exec.Command("git", "--git-dir="+gr.Path(), "cat-file", "-e", b.Revision+"^{commit}").Run() == nil
		}
		if !resolve(&in.KnownGood) || !resolve(&in.KnownBad) {
			writeAPIError(w, 422, "regression_boundary_missing", "known-good and known-bad revisions must resolve in this repository")
			return
		}
		if exec.Command("git", "--git-dir="+gr.Path(), "merge-base", "--is-ancestor", in.KnownGood.Revision, in.KnownBad.Revision).Run() != nil {
			writeAPIError(w, 422, "regression_boundary_incomparable", "known-good must be an ancestor of known-bad")
			return
		}
		in.Comparable = true
		for i := range in.Evidence {
			ev := &in.Evidence[i]
			ev.Available = ev.ResourceID != "" && (ev.Visibility == "repository" || ev.Visibility == "participants")
			if ev.Revision != "" && exec.Command("git", "--git-dir="+gr.Path(), "cat-file", "-e", ev.Revision+"^{commit}").Run() != nil {
				ev.Available = false
				ev.Diagnostic = "referenced revision is missing"
			}
			if !ev.Available && ev.Diagnostic == "" {
				ev.Diagnostic = "evidence is missing or not permitted"
				in.Diagnostics = append(in.Diagnostics, ev.Label+": "+ev.Diagnostic)
			}
		}
		out, e := investigations.Create(in, actor(c))
		writeRegressionInvestigation(w, project(out), e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/regression-investigations/{investigation_id}/events", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Kind            string `json:"kind"`
			Message         string `json:"message"`
			Value           string `json:"value"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributable event is required")
			return
		}
		out, e := investigations.Append(r.PathValue("id"), r.PathValue("investigation_id"), actor(c), in.Kind, in.Message, in.Value, in.ExpectedVersion)
		writeRegressionInvestigation(w, project(out), e, 201)
	})
}

func validRegressionSource(s regressioninvestigations.Reference, repo string, is *issues.Store, ss *supportthreads.Store, cs *checkruns.Store, rs *releases.Store, ds *deployments.Store, dws *debugworkspaces.Store) bool {
	switch s.Kind {
	case "issue":
		x, e := is.Get(repo, s.ResourceID)
		return e == nil && x.ID != ""
	case "support_thread":
		x, e := ss.Get(repo, s.ResourceID)
		return e == nil && x.ID != ""
	case "release":
		x, e := rs.Get(repo, s.ResourceID)
		return e == nil && (s.Revision == "" || s.Revision == x.CommitID)
	case "deployment":
		x, e := ds.GetPromotion(repo, s.ResourceID)
		return e == nil && x.ID != ""
	case "failed_check":
		parts := strings.Split(s.ResourceID, "/")
		if len(parts) != 2 {
			return false
		}
		x, e := cs.Get(repo, parts[0], parts[1])
		return e == nil && x.State == "failed" && (s.Revision == "" || s.Revision == x.CommitID)
	case "reproduction":
		parts := strings.Split(s.ResourceID, "/")
		if len(parts) != 2 {
			return false
		}
		x, e := dws.Get(repo, parts[0])
		if e != nil {
			return false
		}
		for _, q := range x.ReplayScenarios {
			if q.ID == parts[1] && (q.Status == "reproduced" || q.Status == "demonstrated") {
				return true
			}
		}
	}
	return false
}
func writeRegressionInvestigation(w http.ResponseWriter, v regressioninvestigations.Investigation, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, regressioninvestigations.ErrInvalid):
		writeAPIError(w, 422, "invalid_regression_investigation", "regression investigation is incomplete or invalid")
	case errors.Is(e, regressioninvestigations.ErrConflict):
		writeAPIError(w, 409, "regression_investigation_changed", "regression investigation changed; refresh and retry")
	case errors.Is(e, regressioninvestigations.ErrNotFound):
		writeAPIError(w, 404, "regression_investigation_not_found", "regression investigation not found")
	default:
		writeAPIError(w, 500, "regression_investigation_unavailable", "regression investigation could not be persisted")
	}
}
