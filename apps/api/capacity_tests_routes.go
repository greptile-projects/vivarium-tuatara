package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacitymodels"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacityobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacitytests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type capacityTestInput struct {
	RequestID string             `json:"request_id"`
	Plan      capacitytests.Plan `json:"plan"`
}
type capacityRunInput struct {
	RequestID string            `json:"request_id"`
	Run       capacitytests.Run `json:"run"`
}

func registerCapacityTestRoutes(mux *http.ServeMux, repos *repositories.Store, credentials *auth.Store, objectives *capacityobjectives.Store, models *capacitymodels.Store, releaseStore *releases.Store, tests *capacitytests.Store) {
	read := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool) {
		c, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		return c, ok
	}
	mux.HandleFunc("GET /repositories/{id}/capacity-tests", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := read(w, r); !ok {
			return
		}
		xs, e := tests.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "capacity_tests_unavailable", "capacity tests could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"capacity_tests": xs})
	})
	mux.HandleFunc("GET /repositories/{id}/capacity-tests/{test_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := read(w, r); !ok {
			return
		}
		p, e := tests.Get(r.PathValue("id"), r.PathValue("test_id"))
		if e != nil {
			writeAPIError(w, 404, "capacity_test_not_found", "capacity test not found")
			return
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("GET /repositories/{id}/capacity-tests/{test_id}/comparison", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := read(w, r); !ok {
			return
		}
		v, e := tests.Compare(r.PathValue("id"), r.PathValue("test_id"))
		if e != nil {
			writeAPIError(w, 404, "capacity_test_not_found", "capacity test not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/capacity-tests", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var in capacityTestInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded capacity test is required")
			return
		}
		objective, e := objectives.Get(in.Plan.ObjectiveID)
		if e != nil || objective.RepositoryID != r.PathValue("id") || in.Plan.ObjectiveVersion < 1 || in.Plan.ObjectiveVersion > len(objective.Revisions) {
			writeAPIError(w, 422, "capacity_objective_invalid", "the exact capacity objective revision does not resolve")
			return
		}
		if in.Plan.ModelID != "" {
			m, x := models.Get(in.Plan.ModelID, c.UserID)
			if x != nil || m.RepositoryID != r.PathValue("id") || in.Plan.ModelVersion < 1 || in.Plan.ModelVersion > len(m.Revisions) {
				writeAPIError(w, 422, "capacity_model_invalid", "the exact capacity model revision does not resolve")
				return
			}
		}
		for _, candidate := range in.Plan.Candidates {
			for _, component := range candidate.Components {
				if component.Kind == "release" {
					release, x := releaseStore.Get(r.PathValue("id"), component.ResourceID)
					if x != nil || release.CommitID != component.Revision {
						writeAPIError(w, 422, "capacity_candidate_invalid", "release components must bind an authoritative exact repository release")
						return
					}
				}
			}
		}
		_, actor := capacityActor(c)
		out, e := tests.Create(r.PathValue("id"), actor, in.RequestID, in.Plan)
		writeCapacityTest(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/capacity-tests/{test_id}/runs", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var in capacityRunInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "complete retained execution evidence is required")
			return
		}
		kind, actor := capacityActor(c)
		out, e := tests.AddRun(r.PathValue("id"), kind, actor, r.PathValue("test_id"), in.RequestID, in.Run)
		switch {
		case e == nil:
			writeJSON(w, 201, out)
		case errors.Is(e, capacitytests.ErrNotFound):
			writeAPIError(w, 404, "capacity_test_not_found", "capacity test not found")
		case errors.Is(e, capacitytests.ErrConflict):
			writeAPIError(w, 409, "capacity_run_conflict", "request identity was reused with changed evidence")
		default:
			writeAPIError(w, 422, "capacity_run_invalid", "evidence must match a retained candidate and scenario and expose failures, noise, comparability, correctness, limits, metrics, and logs")
		}
	})
}
func writeCapacityTest(w http.ResponseWriter, p capacitytests.Plan, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, p)
	case errors.Is(e, capacitytests.ErrConflict):
		writeAPIError(w, 409, "capacity_test_conflict", "request identity was reused with changed content")
	case errors.Is(e, capacitytests.ErrInvalid):
		writeAPIError(w, 422, "capacity_test_invalid", "two exact scaling alternatives and bounded repository-defined synthetic scenarios are required")
	default:
		writeAPIError(w, 500, "capacity_tests_unavailable", "capacity test could not be persisted")
	}
}
