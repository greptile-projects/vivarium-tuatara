package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productexperiments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type productExperimentInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Source          productexperiments.Source   `json:"source"`
	Revision        productexperiments.Revision `json:"revision"`
	Signals         []productexperiments.Signal `json:"signals"`
}
type productExperimentCommentInput struct {
	Body string `json:"body"`
}
type productExperimentApprovalInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Decision        string `json:"decision"`
	Note            string `json:"note"`
}

func registerProductExperimentRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *productexperiments.Store) {
	writeProjected := func(w http.ResponseWriter, experiment productexperiments.Experiment, status int) {
		all, err := store.List(experiment.RepositoryID)
		if err != nil {
			writeAPIError(w, 500, "product_experiments_unavailable", "experiments could not be projected")
			return
		}
		for _, other := range all {
			if other.ID != experiment.ID && productexperiments.Overlaps(experiment, other) {
				productexperiments.AddOverlap(&experiment, other)
			}
		}
		writeJSON(w, status, experiment)
	}
	mux.HandleFunc("POST /repositories/{id}/product-experiments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in productExperimentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete experiment plan is required")
			return
		}
		var out productexperiments.Experiment
		err := catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error {
			var e error
			out, e = store.Create(r.PathValue("id"), actor.UserID, in.Source, in.Revision, in.Signals)
			return e
		})
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/product-experiments", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		all, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "product_experiments_unavailable", "experiments could not be read")
			return
		}
		for i := range all {
			for _, other := range all {
				if all[i].ID != other.ID && productexperiments.Overlaps(all[i], other) {
					productexperiments.AddOverlap(&all[i], other)
				}
			}
		}
		writeJSON(w, 200, map[string]any{"experiments": all})
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		mutateExperiment(w, r, catalog, credentials, store, func(actor string, in productExperimentInput) (productexperiments.Experiment, error) {
			return store.Revise(r.PathValue("experiment_id"), in.ExpectedVersion, actor, in.Revision, in.Signals)
		}, writeProjected)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentCommentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "comment body is required")
			return
		}
		out, err := store.Comment(current.ID, actor.UserID, in.Body)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/approvals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentApprovalInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a current plan decision is required")
			return
		}
		out, err := store.Approve(current.ID, actor.UserID, in.Decision, in.Note, in.ExpectedVersion)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
}
func mutateExperiment(w http.ResponseWriter, r *http.Request, catalog *repositories.Store, credentials *auth.Store, store *productexperiments.Store, fn func(string, productExperimentInput) (productexperiments.Experiment, error), write func(http.ResponseWriter, productexperiments.Experiment, int)) {
	actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
	if !ok {
		return
	}
	current, err := store.Get(r.PathValue("experiment_id"))
	if err != nil || current.RepositoryID != r.PathValue("id") {
		writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
		return
	}
	var in productExperimentInput
	if decodeJSON(r, &in) != nil {
		writeAPIError(w, 400, "invalid_request", "a complete experiment revision is required")
		return
	}
	var out productexperiments.Experiment
	err = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error { var e error; out, e = fn(actor.UserID, in); return e })
	if err != nil {
		writeProductExperimentError(w, err)
		return
	}
	write(w, out, 200)
}
func writeProductExperimentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, productexperiments.ErrConflict):
		writeAPIError(w, 409, "product_experiment_conflict", "the plan changed; reload before acting")
	case errors.Is(err, productexperiments.ErrInvalid):
		writeAPIError(w, 400, "invalid_product_experiment", "source, hypothesis, variants, audience, metrics, evidence, duration, owners, stop conditions, and versioned signals are required")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "product_experiment_forbidden", "only a current repository participant may change experiments")
	default:
		log.Printf("product experiment storage: %v", err)
		writeAPIError(w, 500, "product_experiments_unavailable", "experiment could not be persisted")
	}
}
