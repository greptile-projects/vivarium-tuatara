package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
)

type serviceObjectiveInput struct {
	ExpectedVersion int                        `json:"expected_version"`
	Revision        serviceobjectives.Revision `json:"revision"`
}
type signalMappingInput struct {
	ExpectedVersion int                                     `json:"expected_version"`
	Revision        serviceobjectives.SignalMappingRevision `json:"revision"`
}
type observationInput struct {
	Observation serviceobjectives.Observation `json:"observation"`
}

func registerServiceObjectiveRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, contracts *serviceobjectives.Store) {
	mux.HandleFunc("GET /repositories/{id}/service-objectives", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, err := contracts.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "service_objectives_unavailable", "service objectives could not be read")
			return
		}
		participant := serviceObjectiveReaderParticipant(r, catalog, credentials, r.PathValue("id"))
		for i := range values {
			values[i] = contracts.ProjectForReader(values[i], participant)
		}
		writeJSON(w, 200, map[string]any{"service_objectives": values})
	})
	mux.HandleFunc("GET /repositories/{id}/service-objectives/{objective_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		writeJSON(w, 200, contracts.ProjectForReader(out, serviceObjectiveReaderParticipant(r, catalog, credentials, out.RepositoryID)))
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in serviceObjectiveInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete service objective revision is required")
			return
		}
		var out serviceobjectives.Contract
		err := catalog.WithCurrentParticipants(serviceObjectiveParticipants(actor.UserID, in.Revision), r.PathValue("id"), func() error {
			var e error
			out, e = contracts.Create(r.PathValue("id"), actor.UserID, in.Revision)
			return e
		})
		writeServiceObjective(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in serviceObjectiveInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete service objective revision are required")
			return
		}
		var out serviceobjectives.Contract
		err = catalog.WithCurrentParticipants(serviceObjectiveParticipants(actor.UserID, in.Revision), current.RepositoryID, func() error {
			var e error
			out, e = contracts.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
			return e
		})
		writeServiceObjective(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/signal-mappings", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in signalMappingInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete sanitized signal mapping is required")
			return
		}
		var out serviceobjectives.Contract
		err = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error {
			var e error
			out, e = contracts.PublishMapping(current.ID, actor.UserID, in.Revision)
			return e
		})
		writeReliabilityEvidence(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/signal-mappings/{mapping_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in signalMappingInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete sanitized signal mapping are required")
			return
		}
		var out serviceobjectives.Contract
		err = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error {
			var e error
			out, e = contracts.ReviseMapping(current.ID, r.PathValue("mapping_id"), in.ExpectedVersion, actor.UserID, in.Revision)
			return e
		})
		writeReliabilityEvidence(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/observations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in observationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete sanitized observation is required")
			return
		}
		var out serviceobjectives.Contract
		err = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error {
			var e error
			out, e = contracts.RecordObservation(current.ID, actor.UserID, in.Observation)
			return e
		})
		writeReliabilityEvidence(w, out, err, 201)
	})
}

func serviceObjectiveReaderParticipant(r *http.Request, catalog *repositories.Store, credentials *auth.Store, repositoryID string) bool {
	actor, authenticated, err := authenticateOptionalCredential(r, credentials, "repositories:read")
	if err != nil || !authenticated {
		return false
	}
	repo, err := catalog.GetByID(repositoryID)
	if err != nil {
		return false
	}
	if repo.OwnerID == actor.UserID {
		return true
	}
	ok, err := catalog.HasCollaborator(actor.UserID, repositoryID)
	return err == nil && ok
}
func writeReliabilityEvidence(w http.ResponseWriter, v serviceobjectives.Contract, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, serviceobjectives.ErrConflict):
		writeAPIError(w, 409, "signal_mapping_conflict", "the signal mapping changed; reload before publishing")
	case errors.Is(err, serviceobjectives.ErrMappingNotFound):
		writeAPIError(w, 404, "signal_mapping_not_found", "signal mapping not found")
	case errors.Is(err, serviceobjectives.ErrInvalid):
		writeAPIError(w, 400, "invalid_reliability_evidence", "evidence must bind an exact objective and mapping revision, sanitized sources, a measurement window, counts, uncertainty, gaps, and delivered-software provenance")
	default:
		log.Printf("reliability evidence storage: %v", err)
		writeAPIError(w, 500, "reliability_evidence_unavailable", "reliability evidence could not be persisted")
	}
}
func serviceObjectiveParticipants(actor string, r serviceobjectives.Revision) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(v string) {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	add(actor)
	for _, v := range r.OwnerIDs {
		add(v)
	}
	for _, x := range r.Objectives {
		for _, v := range x.OwnerIDs {
			add(v)
		}
	}
	for _, x := range r.Journeys {
		for _, v := range x.OwnerIDs {
			add(v)
		}
	}
	for _, x := range r.Dependencies {
		for _, v := range x.OwnerIDs {
			add(v)
		}
	}
	for _, x := range r.Severities {
		for _, v := range x.OwnerIDs {
			add(v)
		}
	}
	for _, v := range r.ExceptionPolicy.ApprovalOwnerIDs {
		add(v)
	}
	for _, x := range r.Exceptions {
		add(x.ApprovedBy)
	}
	return out
}
func writeServiceObjective(w http.ResponseWriter, v serviceobjectives.Contract, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, serviceobjectives.ErrConflict):
		writeAPIError(w, 409, "service_objective_conflict", "the service objective changed; reload before publishing")
	case errors.Is(err, serviceobjectives.ErrInvalid):
		writeAPIError(w, 400, "invalid_service_objective", "the contract must completely define scope, indicators, objectives, windows, journeys, budgets, severity, ownership, exception policy, and rationale")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "service_objective_forbidden", "only current repository participants may publish service objectives")
	default:
		log.Printf("service objective storage: %v", err)
		writeAPIError(w, 500, "service_objectives_unavailable", "service objectives could not be persisted")
	}
}
