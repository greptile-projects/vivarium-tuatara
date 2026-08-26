package main

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/debugworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/observabilitygaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/runbooks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
)

type observabilityGapInput struct {
	RequestID       string                     `json:"request_id"`
	ExpectedVersion int                        `json:"expected_version"`
	Revision        observabilitygaps.Revision `json:"revision"`
}

func registerObservabilityGapRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *observabilitygaps.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, objectiveStore *serviceobjectives.Store, incidentStore *incidents.Store, debugStore *debugworkspaces.Store, runbookStore *runbooks.Store, supportStore *supportthreads.Store) {
	mux.HandleFunc("GET /repositories/{id}/observability-gaps", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		xs, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "observability_gaps_unavailable", "observability gaps could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"observability_gaps": xs})
	})
	mux.HandleFunc("GET /repositories/{id}/observability-gaps/{gap_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := store.Get(r.PathValue("gap_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "observability_gap_not_found", "observability gap not found")
			return
		}
		writeJSON(w, 200, v)
	})
	publish := func(w http.ResponseWriter, r *http.Request, revise bool) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in observabilityGapInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete observability gap revision is required")
			return
		}
		if !observabilitySourceResolves(r.PathValue("id"), in.Revision.Source, objectiveStore, incidentStore, debugStore, runbookStore, supportStore, deploymentStore) {
			writeAPIError(w, 422, "observability_source_invalid", "the exact operational source does not resolve in this repository")
			return
		}
		for _, e := range in.Revision.Evidence {
			release, x := releaseStore.Get(r.PathValue("id"), e.ReleaseID)
			if x != nil || release.CommitID != e.ReleaseRevision {
				writeAPIError(w, 422, "observability_evidence_release_invalid", "every evidence item must bind an exact authoritative repository release")
				return
			}
			if _, x = deploymentStore.GetEnvironment(r.PathValue("id"), e.Environment); x != nil {
				writeAPIError(w, 422, "observability_evidence_environment_invalid", "every evidence environment must resolve in this repository")
				return
			}
			promotions, x := deploymentStore.ListPromotions(r.PathValue("id"))
			if x != nil {
				writeAPIError(w, 500, "observability_gaps_unavailable", "deployment provenance could not be verified")
				return
			}
			matched := false
			for _, promotion := range promotions {
				if promotion.EnvironmentID == e.Environment && promotion.ReleaseID == e.ReleaseID && promotion.CommitID == e.ReleaseRevision && promotion.State == "succeeded" {
					matched = true
					break
				}
			}
			if !matched {
				writeAPIError(w, 422, "observability_evidence_promotion_invalid", "every evidence item must bind a successful exact release promotion to its environment")
				return
			}
		}
		participants := append([]string{actor.UserID}, in.Revision.OwnerIDs...)
		participants = append(participants, in.Revision.AudienceIDs...)
		var out observabilitygaps.Gap
		e := catalog.WithCurrentParticipants(participants, r.PathValue("id"), func() error {
			var x error
			if revise {
				out, x = store.Revise(r.PathValue("id"), r.PathValue("gap_id"), in.ExpectedVersion, actor.UserID, in.RequestID, in.Revision)
			} else {
				out, x = store.Create(r.PathValue("id"), actor.UserID, in.RequestID, in.Revision)
			}
			return x
		})
		writeObservabilityGap(w, out, e, map[bool]int{true: 200, false: 201}[revise])
	}
	mux.HandleFunc("POST /repositories/{id}/observability-gaps", func(w http.ResponseWriter, r *http.Request) { publish(w, r, false) })
	mux.HandleFunc("POST /repositories/{id}/observability-gaps/{gap_id}/revisions", func(w http.ResponseWriter, r *http.Request) { publish(w, r, true) })
}

func observabilitySourceResolves(repo string, source observabilitygaps.Source, objectives *serviceobjectives.Store, incidentStore *incidents.Store, debugStore *debugworkspaces.Store, runbookStore *runbooks.Store, supportStore *supportthreads.Store, deploymentStore *deployments.Store) bool {
	if source.Kind == "manual" {
		return true
	}
	switch source.Kind {
	case "service_objective":
		v, err := objectives.Get(source.ResourceID)
		return err == nil && v.RepositoryID == repo && source.Revision == strconv.Itoa(v.CurrentVersion)
	case "incident":
		v, err := incidentStore.Get(source.ResourceID)
		if err != nil || source.Revision != strconv.Itoa(v.Version) {
			return false
		}
		for _, scope := range v.Scopes {
			if scope.RepositoryID == repo {
				return true
			}
		}
		return false
	case "debugging_workspace":
		v, err := debugStore.Get(repo, source.ResourceID)
		return err == nil && source.Revision == strconv.Itoa(v.Version)
	case "runbook":
		v, err := runbookStore.Get(source.ResourceID)
		return err == nil && v.RepositoryID == repo && source.Revision == strconv.Itoa(v.CurrentVersion)
	case "support_thread":
		v, err := supportStore.Get(repo, source.ResourceID)
		return err == nil && source.Revision == strconv.Itoa(v.Version)
	case "deployment":
		v, err := deploymentStore.GetPromotion(repo, source.ResourceID)
		return err == nil && source.Revision == v.CommitID
	default:
		return false
	}
}
func writeObservabilityGap(w http.ResponseWriter, out observabilitygaps.Gap, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, out)
	case errors.Is(e, observabilitygaps.ErrNotFound):
		writeAPIError(w, 404, "observability_gap_not_found", "observability gap not found")
	case errors.Is(e, observabilitygaps.ErrConflict):
		writeAPIError(w, 409, "observability_gap_conflict", "the gap changed or this request identity was reused with different content")
	case errors.Is(e, observabilitygaps.ErrInvalid):
		writeAPIError(w, 400, "invalid_observability_gap", "question, behavior, audience, decision, scope, timeliness, source, owners, criteria, and exact evidence status are required")
	case errors.Is(e, repositories.ErrInvalidCollaborator), errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 403, "observability_gap_forbidden", "all owners and audience members must be current repository participants")
	default:
		log.Printf("observability gap storage: %v", e)
		writeAPIError(w, 500, "observability_gaps_unavailable", "observability gap could not be persisted")
	}
}
