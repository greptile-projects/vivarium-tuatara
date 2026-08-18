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
			design, err := designs.Get(c.RepositoryID, c.DesignProposalID)
			if c.Revision == pull.SourceCommitID && err == nil && interfaceCheckMatchesDesign(design, c) {
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
		blob, digest, found := infrastructureCommitBlob(git, p.RepositoryID, p.SourceCommitID, in.DefinitionPath)
		_ = blob
		if !found || digest != in.DefinitionDigest {
			writeAPIError(w, 422, "invalid_interface_definition", "the repository-defined check must resolve at the exact candidate revision")
			return
		}
		var out interfacechecks.Check
		e = designs.WithCurrentVersion(p.RepositoryID, in.DesignProposalID, in.DesignVersion, func(design designproposals.Proposal) error {
			if !interfaceCheckMatchesDesign(design, in) {
				return designproposals.ErrInvalid
			}
			return pulls.WithSourceRevision(p.RepositoryID, p.ID, in.Revision, func(pullrequests.PullRequest) error {
				var createErr error
				out, createErr = checks.Create(in)
				return createErr
			})
		})
		if errors.Is(e, designproposals.ErrConflict) {
			writeAPIError(w, 409, "interface_design_changed", "the accepted design moved while interface evidence was retained")
			return
		}
		if errors.Is(e, designproposals.ErrInvalid) || errors.Is(e, designproposals.ErrNotFound) {
			writeAPIError(w, 422, "invalid_interface_specification", "evidence must cite the accepted implemented design revision")
			return
		}
		if errors.Is(e, pullrequests.ErrSourceChanged) || errors.Is(e, pullrequests.ErrNotReady) {
			writeAPIError(w, 409, "interface_revision_changed", "the pull moved while interface evidence was retained")
			return
		}
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
		var out interfacechecks.Check
		e = designs.WithCurrentVersion(p.RepositoryID, c.DesignProposalID, c.DesignVersion, func(design designproposals.Proposal) error {
			if !interfaceCheckMatchesDesign(design, c) {
				return designproposals.ErrInvalid
			}
			return pulls.WithSourceRevision(p.RepositoryID, p.ID, in.Revision, func(pullrequests.PullRequest) error {
				var classifyErr error
				out, classifyErr = checks.Classify(p.RepositoryID, p.ID, c.ID, in.DifferenceID, in.Outcome, in.Rationale, actor.UserID)
				return classifyErr
			})
		})
		if errors.Is(e, designproposals.ErrConflict) || errors.Is(e, designproposals.ErrInvalid) || errors.Is(e, designproposals.ErrNotFound) {
			writeAPIError(w, 409, "interface_design_changed", "classification applies only while the accepted design revision remains current")
			return
		}
		if errors.Is(e, pullrequests.ErrSourceChanged) || errors.Is(e, pullrequests.ErrNotReady) {
			writeAPIError(w, 409, "interface_revision_changed", "the pull moved while the classification was retained")
			return
		}
		if e != nil {
			writeAPIError(w, 409, "interface_classification_invalid", "difference is already classified or the decision is invalid")
			return
		}
		writeJSON(w, 201, out)
	})
}

func interfaceCheckMatchesDesign(design designproposals.Proposal, check interfacechecks.Check) bool {
	if design.CurrentVersion != check.DesignVersion || design.Implementation == nil || design.Implementation.DesignVersion != check.DesignVersion || len(design.Revisions) != design.CurrentVersion {
		return false
	}
	spec := design.Revisions[check.DesignVersion-1]
	journeyFound := false
	for _, journey := range spec.Journeys {
		if journey.Name == check.Journey {
			journeyFound = true
		}
	}
	if !journeyFound {
		return false
	}
	accepted := map[string]bool{}
	for _, requirement := range append(append([]string{}, spec.AcceptanceCriteria...), spec.ComponentContracts...) {
		accepted[requirement] = true
	}
	for _, requirement := range check.AffectedRequirements {
		if !accepted[requirement] {
			return false
		}
	}
	for _, difference := range check.Differences {
		if !accepted[difference.Requirement] {
			return false
		}
	}
	return true
}
