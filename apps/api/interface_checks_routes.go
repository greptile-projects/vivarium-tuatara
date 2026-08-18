package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/interfacechecks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerInterfaceCheckRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, previewStore *previews.Store, designs *designproposals.Store, checks *interfacechecks.Store) {
	project := func(pull pullrequests.PullRequest, all []interfacechecks.Check) map[string]any {
		current := []interfacechecks.Check{}
		stale := []interfacechecks.Check{}
		for _, c := range all {
			if c.Revision == pull.SourceCommitID {
				current = append(current, c)
			} else {
				stale = append(stale, c)
			}
		}
		return map[string]any{"revision": pull.SourceCommitID, "checks": current, "stale_checks": stale}
	}
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/interface-checks", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, e) {
			return
		}
		all, e := checks.List(p.RepositoryID, p.ID)
		if e != nil {
			writeAPIError(w, 500, "interface_checks_unavailable", "interface evidence is unavailable")
			return
		}
		writeJSON(w, 200, project(p, all))
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/interface-checks", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, e) {
			return
		}
		var in interfacechecks.Check
		if decodeJSONLimit(r, &in, 2<<20) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.RepositoryID = p.RepositoryID
		in.PullRequestID = p.ID
		in.CreatedBy = actor.UserID
		if in.Revision != p.SourceCommitID {
			writeAPIError(w, 409, "interface_revision_changed", "evidence must name the pull's exact current revision")
			return
		}
		preview, e := previewStore.Get(p.RepositoryID, p.ID, in.PreviewID)
		if e != nil || preview.Revision != p.SourceCommitID || preview.Stale || preview.State != "succeeded" {
			writeAPIError(w, 422, "invalid_interface_preview", "evidence requires a successful exact-revision bounded preview")
			return
		}
		design, e := designs.Get(p.RepositoryID, in.DesignProposalID)
		if e != nil || design.CurrentVersion != in.DesignVersion || design.Implementation == nil {
			writeAPIError(w, 422, "invalid_interface_specification", "evidence must cite the accepted implemented design revision")
			return
		}
		spec := design.Revisions[len(design.Revisions)-1]
		journeyFound := false
		for _, journey := range spec.Journeys {
			if journey.Name == in.Journey {
				journeyFound = true
			}
		}
		requirements := append(append([]string{}, spec.AcceptanceCriteria...), spec.ComponentContracts...)
		for _, requirement := range in.AffectedRequirements {
			found := false
			for _, accepted := range requirements {
				if accepted == requirement {
					found = true
				}
			}
			if !found {
				journeyFound = false
			}
		}
		if !journeyFound {
			writeAPIError(w, 422, "invalid_interface_requirements", "journey and affected requirements must come from the accepted design revision")
			return
		}
		blob, digest, found := infrastructureCommitBlob(git, p.RepositoryID, p.SourceCommitID, in.DefinitionPath)
		_ = blob
		if !found || digest != in.DefinitionDigest {
			writeAPIError(w, 422, "invalid_interface_definition", "the repository-defined check must resolve at the exact candidate revision")
			return
		}
		out, e := checks.Create(in)
		if errors.Is(e, interfacechecks.ErrInvalid) {
			writeAPIError(w, 422, "invalid_interface_check", "contexts, coverage, differences, artifacts, performance, and requirements must be complete")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "interface_checks_unavailable", "interface evidence could not be retained")
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/interface-checks/{check_id}/classifications", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, e) {
			return
		}
		var in struct {
			Revision     string `json:"revision"`
			DifferenceID string `json:"difference_id"`
			Outcome      string `json:"outcome"`
			Rationale    string `json:"rationale"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		c, e := checks.Get(p.RepositoryID, p.ID, r.PathValue("check_id"))
		if e != nil {
			writeAPIError(w, 404, "interface_check_not_found", "interface check not found")
			return
		}
		if in.Revision != p.SourceCommitID || c.Revision != p.SourceCommitID {
			writeAPIError(w, 409, "interface_revision_changed", "classification applies only to current-revision evidence")
			return
		}
		out, e := checks.Classify(p.RepositoryID, p.ID, c.ID, in.DifferenceID, in.Outcome, in.Rationale, actor.UserID)
		if e != nil {
			writeAPIError(w, 409, "interface_classification_invalid", "difference is already classified or the decision is invalid")
			return
		}
		writeJSON(w, 201, out)
	})
}
