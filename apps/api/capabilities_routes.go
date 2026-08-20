package main

import (
	"errors"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capabilities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type capabilityInput struct {
	ExpectedVersion int                   `json:"expected_version"`
	Revision        capabilities.Revision `json:"revision"`
}

type retirementEventInput struct {
	ExpectedVersion int                          `json:"expected_version"`
	Event           capabilities.RetirementEvent `json:"event"`
}

func registerCapabilityRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, inventory *capabilities.Store, releaseStore *releases.Store) {
	mux.HandleFunc("GET /repositories/{id}/capabilities", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		out, err := inventory.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "capabilities_unavailable", "capability inventory could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"capabilities": projectCapabilitiesForReader(catalog, actor.UserID, out)})
	})
	mux.HandleFunc("GET /repositories/{id}/capabilities/{capability_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		out, err := inventory.Get(r.PathValue("id"), r.PathValue("capability_id"))
		if err != nil {
			writeAPIError(w, 404, "capability_not_found", "capability not found")
			return
		}
		writeJSON(w, 200, projectCapabilitiesForReader(catalog, actor.UserID, []capabilities.Capability{out})[0])
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in capabilityInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete capability revision is required")
				return
			}
			if !capabilityProvenanceResolves(git, releaseStore, r.PathValue("id"), &in.Revision) {
				writeAPIError(w, 400, "invalid_capability_provenance", "the release, commits, and selected paths must resolve exactly")
				return
			}
			ownerIDs := append([]string{actor.UserID}, in.Revision.OwnerIDs...)
			consumerRepositories := []string{}
			for _, consumer := range in.Revision.Consumers {
				if consumer.RepositoryID != "" {
					consumerRepositories = append(consumerRepositories, consumer.RepositoryID)
				}
			}
			var out capabilities.Capability
			var err error
			err = catalog.WithCurrentParticipantsAndReadAccess(ownerIDs, r.PathValue("id"), actor.UserID, consumerRepositories, func() error {
				if !capabilityConsumerProvenanceResolves(git, in.Revision) {
					return capabilities.ErrInvalid
				}
				if revise {
					out, err = inventory.Revise(r.PathValue("id"), r.PathValue("capability_id"), in.ExpectedVersion, actor.UserID, in.Revision)
				} else {
					out, err = inventory.Create(r.PathValue("id"), actor.UserID, in.Revision)
				}
				return err
			})
			writeCapability(w, out, err, map[bool]int{true: 200, false: 201}[revise])
		}
	}
	mux.HandleFunc("POST /repositories/{id}/capabilities", publish(false))
	mux.HandleFunc("POST /repositories/{id}/capabilities/{capability_id}/revisions", publish(true))
	mux.HandleFunc("POST /repositories/{id}/capabilities/{capability_id}/retirement-plans", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "retirement_plan_forbidden", "agents may assess plans but cannot open them")
			return
		}
		var plan capabilities.RetirementPlan
		if decodeJSON(r, &plan) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete retirement contract is required")
			return
		}
		out, err := inventory.OpenRetirement(r.PathValue("id"), r.PathValue("capability_id"), actor.UserID, plan)
		writeCapability(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/capabilities/{capability_id}/retirement-plans/{plan_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok || !authenticated {
			return
		}
		var in retirementEventInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an expected version and cited retirement event are required")
			return
		}
		if in.Event.Type == "policy_decision" {
			repository, repositoryErr := catalog.GetByID(r.PathValue("id"))
			collaborator, collaboratorErr := catalog.HasCollaborator(actor.UserID, r.PathValue("id"))
			if actor.AgentID != "" || repositoryErr != nil || (repository.OwnerID != actor.UserID && (collaboratorErr != nil || !collaborator)) {
				writeAPIError(w, 403, "retirement_policy_forbidden", "only a human repository participant may record a bounded policy decision")
				return
			}
		}
		actorID, actorType := actor.UserID, "human"
		if actor.AgentID != "" {
			actorID, actorType = actor.AgentID, "read_only_agent"
		}
		out, err := inventory.AppendRetirementEvent(r.PathValue("id"), r.PathValue("capability_id"), r.PathValue("plan_id"), actorID, actorType, in.ExpectedVersion, in.Event)
		writeCapability(w, out, err, 200)
	})
}

func projectCapabilitiesForReader(catalog *repositories.Store, actorID string, values []capabilities.Capability) []capabilities.Capability {
	canRead := func(id string) bool {
		if id == "" {
			return true
		}
		repository, err := catalog.GetByID(id)
		if err != nil {
			return false
		}
		if repository.Visibility == repositories.Public || repository.OwnerID == actorID {
			return true
		}
		allowed, err := catalog.HasCollaborator(actorID, id)
		return err == nil && allowed
	}
	for valueIndex := range values {
		restrictedCurrentConsumers := map[int]bool{}
		for revisionIndex := range values[valueIndex].Revisions {
			revision := &values[valueIndex].Revisions[revisionIndex]
			for consumerIndex := range revision.Consumers {
				consumer := &revision.Consumers[consumerIndex]
				if canRead(consumer.RepositoryID) {
					continue
				}
				if revisionIndex == len(values[valueIndex].Revisions)-1 {
					restrictedCurrentConsumers[consumerIndex] = true
				}
				*consumer = capabilities.Consumer{Name: "restricted", Environment: "restricted", Discovery: "unknown", EvidenceState: "inaccessible", CompatibilityPromise: "restricted"}
			}
		}
		for planIndex := range values[valueIndex].RetirementPlans {
			plan := &values[valueIndex].RetirementPlans[planIndex]
			restrictedAudiences := map[string]bool{}
			restrictedOwners := map[string]bool{}
			for consumerIndex := range plan.Audiences {
				if !restrictedCurrentConsumers[consumerIndex] {
					continue
				}
				restrictedAudiences[plan.Audiences[consumerIndex].Name] = true
				for _, ownerID := range plan.Audiences[consumerIndex].OwnerIDs {
					restrictedOwners[ownerID] = true
				}
				plan.Audiences[consumerIndex] = capabilities.Audience{Name: "restricted", OwnerIDs: []string{"restricted"}, Impact: "restricted affected audience", Commitment: "restricted", EmbargoedDependency: true}
			}
			for diagnosticIndex := range plan.FrozenDiagnostics {
				diagnostic := &plan.FrozenDiagnostics[diagnosticIndex]
				if diagnostic.ConsumerIndex != nil && restrictedCurrentConsumers[*diagnostic.ConsumerIndex] {
					diagnostic.Consumer = "restricted"
				}
			}
			for blockerIndex := range plan.Blockers {
				blocker := &plan.Blockers[blockerIndex]
				if restrictedAudiences[blocker.Audience] {
					blocker.Audience = "restricted"
				}
				if restrictedOwners[blocker.OwnerID] {
					blocker.OwnerID = "restricted"
				}
			}
			for ownerIndex := range plan.RequiredOwnerIDs {
				if restrictedOwners[plan.RequiredOwnerIDs[ownerIndex]] {
					plan.RequiredOwnerIDs[ownerIndex] = "restricted"
				}
			}
			for exceptionIndex := range plan.Exceptions {
				if restrictedAudiences[plan.Exceptions[exceptionIndex].Audience] {
					plan.Exceptions[exceptionIndex].Audience = "restricted"
				}
			}
		}
		for diagnosticIndex := range values[valueIndex].Diagnostics {
			diagnostic := &values[valueIndex].Diagnostics[diagnosticIndex]
			if diagnostic.ConsumerIndex != nil && restrictedCurrentConsumers[*diagnostic.ConsumerIndex] {
				diagnostic.Consumer = "restricted"
			}
		}
	}
	return values
}

func capabilityConsumerProvenanceResolves(git *storage.Store, revision capabilities.Revision) bool {
	for _, consumer := range revision.Consumers {
		if consumer.EvidenceState != "current" {
			continue
		}
		repository, err := git.Open(consumer.RepositoryID)
		if err != nil {
			return false
		}
		if _, err = repository.ReadCommit(storage.ObjectID(strings.ToLower(consumer.Revision))); err != nil {
			return false
		}
	}
	return true
}

func capabilityProvenanceResolves(git *storage.Store, releases *releases.Store, repoID string, r *capabilities.Revision) bool {
	if git == nil || releases == nil {
		return false
	}
	release, err := releases.Get(repoID, r.ReleaseID)
	if err != nil || release.CommitID != strings.ToLower(r.CommitID) {
		return false
	}
	r.CommitID = release.CommitID
	r.ReleaseVersion = release.Version
	repo, err := git.Open(repoID)
	if err != nil {
		return false
	}
	trees := map[string]map[string]bool{}
	for i := range r.Items {
		x := &r.Items[i]
		x.Revision = strings.ToLower(x.Revision)
		if _, ok := trees[x.Revision]; !ok {
			commit, e := repo.ReadCommit(storage.ObjectID(x.Revision))
			if e != nil {
				return false
			}
			entries, e := repo.WalkTree(commit.Tree)
			if e != nil {
				return false
			}
			files := map[string]bool{}
			for _, entry := range entries {
				if entry.Type == storage.BlobObject {
					files[entry.Path] = true
				}
			}
			trees[x.Revision] = files
		}
		if x.Kind != "release" {
			if x.Path == "" || strings.HasPrefix(x.Path, "/") || path.Clean(x.Path) != x.Path || x.Path == "." || strings.HasPrefix(x.Path, "../") || !trees[x.Revision][x.Path] {
				return false
			}
		}
	}
	return true
}
func writeCapability(w http.ResponseWriter, v capabilities.Capability, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, capabilities.ErrNotFound):
		writeAPIError(w, 404, "capability_not_found", "capability not found")
	case errors.Is(err, capabilities.ErrPlanNotFound):
		writeAPIError(w, 404, "retirement_plan_not_found", "retirement plan not found")
	case errors.Is(err, capabilities.ErrConflict):
		writeAPIError(w, 409, "capability_conflict", "the capability changed; reload before publishing")
	case errors.Is(err, capabilities.ErrInvalid):
		writeAPIError(w, 400, "invalid_capability", "define exact capability evidence or a complete, bounded retirement contract")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "capability_forbidden", "only current repository participants may own or publish capabilities")
	default:
		log.Printf("capability storage: %v", err)
		writeAPIError(w, 500, "capabilities_unavailable", "capability inventory could not be persisted")
	}
}
