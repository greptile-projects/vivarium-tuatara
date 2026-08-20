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
	case errors.Is(err, capabilities.ErrConflict):
		writeAPIError(w, 409, "capability_conflict", "the capability changed; reload before publishing")
	case errors.Is(err, capabilities.ErrInvalid):
		writeAPIError(w, 400, "invalid_capability", "define exact items, owners, consumers, evidence, compatibility promises, and unknown use")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "capability_forbidden", "only current repository participants may own or publish capabilities")
	default:
		log.Printf("capability storage: %v", err)
		writeAPIError(w, 500, "capabilities_unavailable", "capability inventory could not be persisted")
	}
}
