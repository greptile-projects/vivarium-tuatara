package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/observabilitygaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type observabilityGapInput struct {
	RequestID       string                     `json:"request_id"`
	ExpectedVersion int                        `json:"expected_version"`
	Revision        observabilitygaps.Revision `json:"revision"`
}

func registerObservabilityGapRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *observabilitygaps.Store, releaseStore *releases.Store) {
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
		for _, e := range in.Revision.Evidence {
			release, x := releaseStore.Get(r.PathValue("id"), e.ReleaseID)
			if x != nil || release.CommitID != e.ReleaseRevision {
				writeAPIError(w, 422, "observability_evidence_release_invalid", "every evidence item must bind an exact authoritative repository release")
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
