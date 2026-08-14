package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/dataflows"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/privacychecks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerPrivacyCheckRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, releaseStore *releases.Store, previewStore *previews.Store, flows *dataflows.Store, checks *privacychecks.Store) {
	mux.HandleFunc("POST /repositories/{id}/privacy-check-policies", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "privacy_policy_forbidden", "only the repository owner may govern runtime privacy evidence")
			return
		}
		var in privacychecks.Policy
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		for _, id := range in.PrivacyOwnerIDs {
			if !privacyParticipant(catalog, r.PathValue("id"), id) {
				writeAPIError(w, 422, "invalid_privacy_check", "privacy owners must be current repository participants")
				return
			}
		}
		out, e := checks.CreatePolicy(r.PathValue("id"), actor.UserID, in)
		writePrivacyCheck(w, out, e, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/privacy-check-policies", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, e := checks.Policies(r.PathValue("id"))
		if e != nil {
			writePrivacyCheck(w, nil, e, 0)
			return
		}
		writeJSON(w, 200, map[string]any{"policies": out})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/privacy-check-runs", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok || (!participant && actor.UserID == "") {
			return
		}
		var in privacychecks.Run
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil || in.Revision != p.SourceCommitID {
			writeAPIError(w, 409, "privacy_check_revision_changed", "evidence must name the pull's exact current revision")
			return
		}
		preview, e := previewStore.Get(p.RepositoryID, p.ID, in.PreviewID)
		if e != nil || preview.Revision != in.Revision || preview.Definition.Access.Network != "none" {
			writeAPIError(w, 422, "invalid_privacy_preview", "privacy checks require the pull's isolated network-none exact-revision preview")
			return
		}
		flow, e := flows.Get(p.RepositoryID, in.DataFlowID)
		if e != nil || in.DataFlowVersion < 1 || in.DataFlowVersion > len(flow.Revisions) || flow.Revisions[in.DataFlowVersion-1].CodeRevision != in.Revision {
			writeAPIError(w, 422, "invalid_privacy_flow", "runtime evidence must bind a current exact-revision data-flow declaration")
			return
		}
		in.PullRequestID = p.ID
		var out privacychecks.Run
		e = pulls.WithSourceRevision(p.RepositoryID, p.ID, in.Revision, func(pullrequests.PullRequest) error {
			var x error
			out, x = checks.AddRun(p.RepositoryID, actor.UserID, in)
			return x
		})
		if errors.Is(e, pullrequests.ErrSourceChanged) {
			writeAPIError(w, 409, "privacy_check_revision_changed", "the pull moved while evidence was retained")
			return
		}
		writePrivacyCheck(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/privacy-check-acknowledgements", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			PolicyID      string `json:"policy_id"`
			Revision      string `json:"revision"`
			PullRequestID string `json:"pull_request_id,omitempty"`
			Rationale     string `json:"rationale"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		out, e := checks.Acknowledge(r.PathValue("id"), actor.UserID, in.PolicyID, in.Revision, in.PullRequestID, in.Rationale)
		writePrivacyCheck(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/privacy-check-exceptions", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "privacy_exception_forbidden", "only the repository owner may publish a bounded privacy exception")
			return
		}
		var in privacychecks.Exception
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		out, e := checks.AddException(r.PathValue("id"), actor.UserID, in)
		writePrivacyCheck(w, out, e, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/privacy-readiness", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		changes, e := pulls.Changes(p.RepositoryID, p.ID)
		if e != nil {
			writePrivacyCheck(w, nil, e, 0)
			return
		}
		paths := []string{}
		for _, c := range changes {
			paths = append(paths, c.Path)
		}
		out, e := checks.Evaluate(p.RepositoryID, p.SourceCommitID, p.TargetBranch, p.ID, paths)
		writePrivacyCheck(w, out, e, 200)
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}/privacy-readiness", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := releaseStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if e != nil {
			writeAPIError(w, 404, "release_not_found", "release not found")
			return
		}
		if v.TargetBranch == "" || v.ChangedPaths == nil {
			writeAPIError(w, 500, "privacy_readiness_unavailable", "release candidate path context is unavailable")
			return
		}
		out, e := checks.Evaluate(v.RepositoryID, v.CommitID, v.TargetBranch, "", v.ChangedPaths)
		writePrivacyCheck(w, out, e, 200)
	})
}

func privacyParticipant(c *repositories.Store, repo, id string) bool {
	r, e := c.GetByID(repo)
	if e != nil {
		return false
	}
	if r.OwnerID == id {
		return true
	}
	ok, _ := c.HasCollaborator(id, repo)
	return ok
}
func writePrivacyCheck(w http.ResponseWriter, out any, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, out)
	case errors.Is(e, privacychecks.ErrInvalid):
		writeAPIError(w, 422, "invalid_privacy_check", "privacy policy, sanitized evidence, acknowledgement, or exception is incomplete")
	case errors.Is(e, privacychecks.ErrNotFound):
		writeAPIError(w, 404, "privacy_check_not_found", "privacy check record not found")
	default:
		writeAPIError(w, 500, "privacy_checks_unavailable", "privacy runtime evidence could not be persisted")
	}
}
