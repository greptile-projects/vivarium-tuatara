package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type designProposalInput struct {
	ExpectedVersion int                      `json:"expected_version"`
	OwnerIDs        []string                 `json:"owner_ids"`
	Revision        designproposals.Revision `json:"revision"`
}

func registerDesignProposalRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *designproposals.Store) {
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
		normalizeDesignEvidence(&in.Revision)
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
		normalizeDesignEvidence(&in.Revision)
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
		for i := range in.Evidence {
			in.Evidence[i].Accessible = in.Evidence[i].Kind == source.Kind && in.Evidence[i].ResourceID == source.ResourceID
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
func normalizeDesignEvidence(r *designproposals.Revision) {
	for i := range r.Evidence {
		r.Evidence[i].Accessible = r.Evidence[i].Kind == r.Source.Kind && r.Evidence[i].ResourceID == r.Source.ResourceID
		if !r.Evidence[i].Accessible && r.Evidence[i].Gap == "" {
			r.Evidence[i].Gap = "citation visibility was not established; no evidence content was copied"
		}
	}
}

func redactDesignArtifacts(v *designproposals.Proposal, actor string) {
	for ri := range v.Revisions {
		for ai := range v.Revisions[ri].Artifacts {
			a := &v.Revisions[ri].Artifacts[ai]
			allowed := false
			for _, id := range a.Audience {
				if id == actor {
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
