package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacitymodels"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacityobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type capacityModelInput struct {
	RequestID       string                  `json:"request_id"`
	ExpectedVersion int                     `json:"expected_version"`
	Revision        capacitymodels.Revision `json:"revision"`
}
type capacityModelEventInput struct {
	ExpectedVersion int                  `json:"expected_version"`
	Event           capacitymodels.Event `json:"event"`
}

func capacityActor(c auth.Credential) (string, string) {
	if c.AgentID != "" {
		return "agent", c.AgentID
	}
	return "human", c.UserID
}

func registerCapacityModelRoutes(mux *http.ServeMux, repos *repositories.Store, credentials *auth.Store, objectives *capacityobjectives.Store, models *capacitymodels.Store) {
	read := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool) {
		c, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		return c, ok
	}
	mux.HandleFunc("GET /repositories/{id}/capacity-models", func(w http.ResponseWriter, r *http.Request) {
		c, ok := read(w, r)
		if !ok {
			return
		}
		_, viewer := capacityActor(c)
		xs, e := models.List(r.PathValue("id"), viewer)
		if e != nil {
			writeAPIError(w, 500, "capacity_models_unavailable", "capacity models could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"capacity_models": xs})
	})
	mux.HandleFunc("GET /repositories/{id}/capacity-models/{model_id}", func(w http.ResponseWriter, r *http.Request) {
		c, ok := read(w, r)
		if !ok {
			return
		}
		_, viewer := capacityActor(c)
		v, e := models.Get(r.PathValue("model_id"), viewer)
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "capacity_model_not_found", "capacity model not found")
			return
		}
		writeJSON(w, 200, v)
	})
	publish := func(w http.ResponseWriter, r *http.Request, revise bool) {
		c, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		kind, actor := capacityActor(c)
		var in capacityModelInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete revision-exact capacity model is required")
			return
		}
		objective, e := objectives.Get(in.Revision.ObjectiveID)
		if e != nil || objective.RepositoryID != r.PathValue("id") || in.Revision.ObjectiveVersion < 1 || in.Revision.ObjectiveVersion > len(objective.Revisions) {
			writeAPIError(w, 422, "capacity_objective_invalid", "the exact capacity objective revision does not resolve")
			return
		}
		var out capacitymodels.Model
		if revise {
			out, e = models.Revise(r.PathValue("model_id"), in.ExpectedVersion, actor, in.RequestID, in.Revision)
		} else {
			out, e = models.Create(r.PathValue("id"), kind, actor, in.RequestID, in.Revision)
		}
		if revise {
			writeCapacityModel(w, out, e, 200)
		} else {
			writeCapacityModel(w, out, e, 201)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/capacity-models", func(w http.ResponseWriter, r *http.Request) { publish(w, r, false) })
	mux.HandleFunc("POST /repositories/{id}/capacity-models/{model_id}/revisions", func(w http.ResponseWriter, r *http.Request) { publish(w, r, true) })
	mux.HandleFunc("POST /repositories/{id}/capacity-models/{model_id}/events", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		kind, actor := capacityActor(c)
		var in capacityModelEventInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a challenge, support, or supersede event is required")
			return
		}
		out, e := models.AddEvent(r.PathValue("model_id"), kind, actor, in.ExpectedVersion, in.Event)
		writeCapacityModel(w, out, e, 201)
	})
}
func writeCapacityModel(w http.ResponseWriter, out capacitymodels.Model, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, out)
	case errors.Is(e, capacitymodels.ErrNotFound):
		writeAPIError(w, 404, "capacity_model_not_found", "capacity model not found")
	case errors.Is(e, capacitymodels.ErrConflict):
		writeAPIError(w, 409, "capacity_model_conflict", "the model changed or this request identity was reused with different content")
	case errors.Is(e, capacitymodels.ErrInvalid):
		writeAPIError(w, 400, "invalid_capacity_model", "exact releases, windows, sanitized evidence, assumptions, segments, saturation uncertainty, costs, scenarios, and method are required")
	default:
		writeAPIError(w, 500, "capacity_models_unavailable", "capacity model could not be persisted")
	}
}
