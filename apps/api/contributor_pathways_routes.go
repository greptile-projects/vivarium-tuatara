package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributorpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerContributorPathwayRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, pathways *contributorpathways.Store, releaseStore *releases.Store, issueStore *issues.Store, proposalStore *proposals.Store, workspaceStore *workspaces.Store, credentials *auth.Store) {
	present := func(repositoryID, actorID string, revision contributorpathways.Revision) contributorpathways.Revision {
		repo, _ := repos.GetByID(repositoryID)
		canReadPrivate := actorID == repo.OwnerID
		if !canReadPrivate && actorID != "" {
			canReadPrivate, _ = repos.HasCollaborator(actorID, repositoryID)
		}
		gitRepo, _ := git.Open(repositoryID)
		for i := range revision.Requirements {
			link := &revision.Requirements[i]
			link.Status, link.StatusDetail = "current", "Requirement is available."
			switch link.Kind {
			case "ownership":
				if repo.OwnerID == "" {
					link.Status, link.StatusDetail = "inaccessible", "Repository ownership is unavailable."
				}
			case "documentation":
				if gitRepo == nil {
					link.Status, link.StatusDetail = "inaccessible", "Repository content is unavailable."
					break
				}
				ref, err := gitRepo.ReadReference("refs/heads/" + repo.DefaultBranch)
				if err != nil {
					link.Status, link.StatusDetail = "inaccessible", "The default branch is unavailable."
					break
				}
				commit, err := gitRepo.ReadCommit(storage.ObjectID(ref.Target))
				if err != nil {
					link.Status, link.StatusDetail = "inaccessible", "The default branch is unreadable."
					break
				}
				entries, err := gitRepo.WalkTree(commit.Tree)
				found := false
				for _, entry := range entries {
					if entry.Path == link.Path && entry.Type == storage.BlobObject {
						found = true
						break
					}
				}
				if err != nil || !found {
					link.Status, link.StatusDetail = "stale", "The documented path no longer exists on the default branch."
				} else if link.Revision != "" && link.Revision != ref.Target {
					link.Status, link.StatusDetail = "stale", "The document has moved beyond the linked revision."
				}
			case "release":
				if releaseStore == nil {
					link.Status, link.StatusDetail = "inaccessible", "Release records are unavailable."
				} else if _, err := releaseStore.Get(repositoryID, link.ResourceID); err != nil {
					link.Status, link.StatusDetail = "stale", "The linked release is no longer available."
				}
			case "issue":
				if issueStore == nil {
					link.Status, link.StatusDetail = "inaccessible", "Issue records are unavailable."
				} else if v, err := issueStore.Get(repositoryID, link.ResourceID); err != nil || v.Visibility != "public" && !canReadPrivate {
					link.Status, link.StatusDetail = "inaccessible", "The linked issue is not accessible."
				}
			case "proposal":
				if proposalStore == nil {
					link.Status, link.StatusDetail = "inaccessible", "Proposal records are unavailable."
				} else if _, err := proposalStore.Get(repositoryID, link.ResourceID); err != nil {
					link.Status, link.StatusDetail = "stale", "The linked proposal is no longer available."
				}
			case "workspace_definition":
				if workspaceStore == nil {
					link.Status, link.StatusDetail = "inaccessible", "Workspace records are unavailable."
				} else if v, err := workspaceStore.Get(link.ResourceID); err != nil || v.RepositoryID != repositoryID {
					link.Status, link.StatusDetail = "stale", "The linked workspace definition is no longer available."
				} else if actorID == "" || !canReadPrivate || v.Policy.Sharing == "private" && actorID != v.CreatorID && actorID != repo.OwnerID {
					link.Status, link.StatusDetail = "inaccessible", "The linked workspace definition is not accessible."
				}
			}
		}
		return revision
	}
	mux.HandleFunc("GET /repositories/{id}/contributor-pathway", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		actorID := ""
		if authenticated {
			actorID = actor.UserID
		}
		current, err := pathways.Current(r.PathValue("id"))
		if errors.Is(err, contributorpathways.ErrNotFound) {
			writeAPIError(w, 404, "contributor_pathway_not_found", "contributor pathway has not been published")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "contributor_pathway_read_failed", "contributor pathway could not be read")
			return
		}
		history, err := pathways.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "contributor_pathway_read_failed", "contributor pathway could not be read")
			return
		}
		acks, err := pathways.Acknowledgements(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "contributor_pathway_read_failed", "contributor pathway could not be read")
			return
		}
		visibleAcks := []contributorpathways.Acknowledgement{}
		repository, _ := repos.GetByID(r.PathValue("id"))
		if actorID == repository.OwnerID {
			visibleAcks = acks
		} else if actorID != "" {
			for _, acknowledgement := range acks {
				if acknowledgement.ActorID == actorID {
					visibleAcks = append(visibleAcks, acknowledgement)
				}
			}
		}
		writeJSON(w, 200, map[string]any{"pathway": present(r.PathValue("id"), actorID, current), "history": history, "acknowledgements": visibleAcks, "acknowledgement_count": len(acks)})
	})
	mux.HandleFunc("PUT /repositories/{id}/contributor-pathway", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "owner_required", "only the repository owner can publish contributor expectations")
			return
		}
		var input struct {
			ExpectedVersion int                          `json:"expected_version"`
			Pathway         contributorpathways.Revision `json:"pathway"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		input.Pathway.RepositoryID, input.Pathway.PublishedBy = r.PathValue("id"), actor.UserID
		input.Pathway.ID, input.Pathway.Version = "", 0
		for i := range input.Pathway.Requirements {
			input.Pathway.Requirements[i].Status, input.Pathway.Requirements[i].StatusDetail = "", ""
		}
		created, err := pathways.Publish(input.Pathway, input.ExpectedVersion)
		if errors.Is(err, contributorpathways.ErrConflict) {
			writeAPIError(w, 409, "contributor_pathway_changed", "contributor pathway version changed")
			return
		}
		if errors.Is(err, contributorpathways.ErrInvalid) {
			writeAPIError(w, 422, "invalid_contributor_pathway", "all contributor expectations and valid requirement links are required")
			return
		}
		if err != nil && !errors.Is(err, contributorpathways.ErrDurabilityUncertain) {
			writeAPIError(w, 500, "contributor_pathway_write_failed", "contributor pathway could not be published")
			return
		}
		w.Header().Set("Location", "/repositories/"+r.PathValue("id")+"/contributor-pathway")
		status := http.StatusCreated
		if errors.Is(err, contributorpathways.ErrDurabilityUncertain) {
			w.Header().Set("Vivarium-Durability", "uncertain")
			status = http.StatusAccepted
		}
		writeJSON(w, status, present(r.PathValue("id"), actor.UserID, created))
	})
	mux.HandleFunc("POST /repositories/{id}/contributor-pathway/acknowledgements", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		repo, err := repos.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		collaborator, _ := repos.HasCollaborator(actor.UserID, repo.ID)
		if repo.Visibility != repositories.Public && actor.UserID != repo.OwnerID && !collaborator {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		var input struct {
			Version int `json:"version"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		ack, err := pathways.Acknowledge(repo.ID, input.Version, actor.UserID)
		if errors.Is(err, contributorpathways.ErrAcknowledged) {
			writeAPIError(w, 409, "pathway_already_acknowledged", "this pathway revision is already acknowledged")
			return
		}
		if errors.Is(err, contributorpathways.ErrNotFound) {
			writeAPIError(w, 422, "invalid_pathway_version", "pathway version does not exist")
			return
		}
		if err != nil && !errors.Is(err, contributorpathways.ErrDurabilityUncertain) {
			writeAPIError(w, 500, "pathway_acknowledgement_failed", "acknowledgement could not be retained")
			return
		}
		status := http.StatusCreated
		if errors.Is(err, contributorpathways.ErrDurabilityUncertain) {
			w.Header().Set("Vivarium-Durability", "uncertain")
			status = http.StatusAccepted
		}
		writeJSON(w, status, ack)
	})
}
