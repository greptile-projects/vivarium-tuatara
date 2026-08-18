package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
	productfeedback "github.com/greptile-projects/vivarium-tuatara/apps/api/feedback"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/roadmaps"
)

type designProposalInput struct {
	ExpectedVersion int                      `json:"expected_version"`
	OwnerIDs        []string                 `json:"owner_ids"`
	Revision        designproposals.Revision `json:"revision"`
}

func registerDesignProposalRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *designproposals.Store, issueStore *issues.Store, feedbackStore *productfeedback.Store, roadmapStore *roadmaps.Store, assessmentStore *accessibilityassessments.Store, pullStore *pullrequests.Store) {
	mux.HandleFunc("GET /repositories/{id}/design-proposals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "design_proposals_unavailable", "design proposals could not be read")
			return
		}
		for i := range v {
			projectDesignEvidence(&v[i], actor.UserID, designReaderIsParticipant(catalog, r.PathValue("id"), actor.UserID), issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
			redactDesignArtifacts(&v[i], actor.UserID)
		}
		writeJSON(w, 200, map[string]any{"design_proposals": v})
	})
	mux.HandleFunc("GET /repositories/{id}/design-proposals/{proposal_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := store.Get(r.PathValue("id"), r.PathValue("proposal_id"))
		projectDesignEvidence(&v, actor.UserID, designReaderIsParticipant(catalog, r.PathValue("id"), actor.UserID), issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
		redactDesignArtifacts(&v, actor.UserID)
		writeDesignProposal(w, v, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/design-proposals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in designProposalInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete design proposal is required")
			return
		}
		normalizeDesignEvidence(r.PathValue("id"), &in.Revision, issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
		owners := append([]string(nil), in.OwnerIDs...)
		if len(owners) == 0 {
			owners = []string{actor.UserID}
		}
		var out designproposals.Proposal
		var e error
		e = catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error { out, e = store.Create(r.PathValue("id"), actor.UserID, owners, in.Revision); return e })
		redactDesignArtifacts(&out, actor.UserID)
		writeDesignProposal(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/design-proposals/{proposal_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in designProposalInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete successor revision is required")
			return
		}
		normalizeDesignEvidence(r.PathValue("id"), &in.Revision, issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
		out, e := store.Revise(r.PathValue("id"), r.PathValue("proposal_id"), actor.UserID, in.ExpectedVersion, in.Revision)
		redactDesignArtifacts(&out, actor.UserID)
		writeDesignProposal(w, out, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/design-proposals/{proposal_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in designproposals.Comment
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a revision-bound comment is required")
			return
		}
		current, getErr := store.Get(r.PathValue("id"), r.PathValue("proposal_id"))
		if getErr != nil || in.Revision < 1 || in.Revision > current.CurrentVersion {
			writeDesignProposal(w, current, designproposals.ErrInvalid, 0)
			return
		}
		source := current.Revisions[in.Revision-1].Source
		sourceAccessible := designSourceExists(r.PathValue("id"), source, issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
		for i := range in.Evidence {
			in.Evidence[i].Accessible = sourceAccessible && in.Evidence[i].Kind == source.Kind && in.Evidence[i].ResourceID == source.ResourceID
			if !in.Evidence[i].Accessible && in.Evidence[i].Gap == "" {
				in.Evidence[i].Gap = "citation visibility was not established; no asset content was copied"
			}
		}
		out, e := store.Comment(r.PathValue("id"), r.PathValue("proposal_id"), actor.UserID, in)
		redactDesignArtifacts(&out, actor.UserID)
		writeDesignProposal(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/design-proposals/{proposal_id}/acknowledgements", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in designproposals.Acknowledgement
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a current-revision owner acknowledgement is required")
			return
		}
		out, e := store.Acknowledge(r.PathValue("id"), r.PathValue("proposal_id"), actor.UserID, in)
		redactDesignArtifacts(&out, actor.UserID)
		writeDesignProposal(w, out, e, 201)
	})
}

// Citation metadata is retained, but caller claims never make research or assets visible.
// An explicit gap lets collaborators request access without propagating the underlying content.
func normalizeDesignEvidence(repositoryID string, r *designproposals.Revision, issueStore *issues.Store, feedbackStore *productfeedback.Store, roadmapStore *roadmaps.Store, assessmentStore *accessibilityassessments.Store, pullStore *pullrequests.Store) {
	sourceAccessible := designSourceExists(repositoryID, r.Source, issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
	for i := range r.Evidence {
		r.Evidence[i].Accessible = sourceAccessible && r.Evidence[i].Kind == r.Source.Kind && r.Evidence[i].ResourceID == r.Source.ResourceID
		if !r.Evidence[i].Accessible && r.Evidence[i].Gap == "" {
			r.Evidence[i].Gap = "citation visibility was not established; no evidence content was copied"
		}
	}
}

func designSourceExists(repositoryID string, source designproposals.Source, issueStore *issues.Store, feedbackStore *productfeedback.Store, roadmapStore *roadmaps.Store, assessmentStore *accessibilityassessments.Store, pullStore *pullrequests.Store) bool {
	switch source.Kind {
	case "issue":
		if issueStore == nil {
			return false
		}
		v, err := issueStore.Get(repositoryID, source.ResourceID)
		return err == nil && v.RepositoryID == repositoryID
	case "feedback":
		if feedbackStore == nil {
			return false
		}
		v, err := feedbackStore.Get(source.ResourceID)
		return err == nil && v.RepositoryID == repositoryID
	case "roadmap_outcome":
		if roadmapStore == nil {
			return false
		}
		v, err := roadmapStore.Get(repositoryID)
		if err != nil {
			return false
		}
		for _, revision := range v.Revisions {
			for _, item := range revision.Items {
				if item.ID == source.ResourceID || item.OpportunityID == source.ResourceID {
					return true
				}
			}
		}
		return false
	case "accessibility_finding":
		if assessmentStore == nil {
			return false
		}
		values, err := assessmentStore.List(repositoryID, "", "")
		if err != nil {
			return false
		}
		for _, assessment := range values {
			for _, finding := range assessment.Findings {
				if finding.ID == source.ResourceID {
					return true
				}
			}
		}
		return false
	case "pull_request":
		if pullStore == nil {
			return false
		}
		v, err := pullStore.Get(repositoryID, source.ResourceID)
		return err == nil && v.RepositoryID == repositoryID
	default:
		return false
	}
}

func designReaderIsParticipant(catalog *repositories.Store, repositoryID, actor string) bool {
	if actor == "" {
		return false
	}
	repository, err := catalog.GetByID(repositoryID)
	if err != nil {
		return false
	}
	if repository.OwnerID == actor {
		return true
	}
	ok, _ := catalog.HasCollaborator(actor, repositoryID)
	return ok
}

func projectDesignEvidence(v *designproposals.Proposal, actor string, participant bool, issueStore *issues.Store, feedbackStore *productfeedback.Store, roadmapStore *roadmaps.Store, assessmentStore *accessibilityassessments.Store, pullStore *pullrequests.Store) {
	visible := func(source designproposals.Source) bool {
		if actor == "" || !designSourceExists(v.RepositoryID, source, issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore) {
			return false
		}
		switch source.Kind {
		case "issue":
			return participant
		case "feedback":
			item, err := feedbackStore.Get(source.ResourceID)
			return err == nil && (item.Audience != "organization_private" || participant || item.ReporterID == actor)
		default:
			return true
		}
	}
	project := func(evidence *designproposals.Evidence, source designproposals.Source) {
		evidence.Accessible = visible(source) && evidence.Kind == source.Kind && evidence.ResourceID == source.ResourceID
		if !evidence.Accessible && evidence.Gap == "" {
			evidence.Gap = "citation is not visible to the current reader; no evidence content was copied"
		}
	}
	for ri := range v.Revisions {
		source := v.Revisions[ri].Source
		for ei := range v.Revisions[ri].Evidence {
			project(&v.Revisions[ri].Evidence[ei], source)
		}
	}
	for ci := range v.Comments {
		revision := v.Comments[ci].Revision
		if revision < 1 || revision > len(v.Revisions) {
			continue
		}
		source := v.Revisions[revision-1].Source
		for ei := range v.Comments[ci].Evidence {
			project(&v.Comments[ci].Evidence[ei], source)
		}
	}
}

func redactDesignArtifacts(v *designproposals.Proposal, actor string) {
	for ri := range v.Revisions {
		for ai := range v.Revisions[ri].Artifacts {
			a := &v.Revisions[ri].Artifacts[ai]
			allowed := false
			for _, id := range a.Audience {
				if actor != "" && id != "" && id == actor {
					allowed = true
					break
				}
			}
			if !allowed {
				a.Content = ""
				a.Interactions = nil
				a.Description = "Restricted artifact; request explicit audience access."
			}
		}
	}
}
func writeDesignProposal(w http.ResponseWriter, v designproposals.Proposal, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, designproposals.ErrNotFound):
		writeAPIError(w, 404, "design_proposal_not_found", "design proposal not found")
	case errors.Is(e, designproposals.ErrConflict):
		writeAPIError(w, 409, "design_proposal_conflict", "the design proposal changed; reload before publishing")
	case errors.Is(e, designproposals.ErrInvalid):
		writeAPIError(w, 400, "invalid_design_proposal", "define the source, goal, journeys, states, constraints, alternatives, success measures, affected components, and revision-bound contribution")
	case errors.Is(e, repositories.ErrInvalidCollaborator), errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 403, "design_proposal_forbidden", "owners must be current repository participants")
	default:
		log.Printf("design proposal storage: %v", e)
		writeAPIError(w, 500, "design_proposals_unavailable", "design proposal could not be persisted")
	}
}
