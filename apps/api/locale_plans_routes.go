package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/localeplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type localePlanInput struct {
	ExpectedVersion int                  `json:"expected_version"`
	Revision        localeplans.Revision `json:"revision"`
}

func registerLocalePlanRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, plans *localeplans.Store) {
	mux.HandleFunc("POST /repositories/{id}/locale-plans", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in localePlanInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete locale plan revision is required")
			return
		}
		if !localeResourcesResolve(git, r.PathValue("id"), in.Revision.Resources) {
			writeAPIError(w, 422, "invalid_locale_source_revision", "every translatable resource must bind an existing exact commit in this repository")
			return
		}
		var out localeplans.Plan
		err := catalog.WithCurrentParticipants(localePlanParticipants(actor.UserID, in.Revision), r.PathValue("id"), func() error {
			var e error
			out, e = plans.Create(r.PathValue("id"), actor.UserID, in.Revision)
			return e
		})
		writeLocalePlan(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/locale-plans", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, err := plans.List(r.PathValue("id"), localeHead(git, catalog, r.PathValue("id")))
		if err != nil {
			writeAPIError(w, 500, "locale_plans_unavailable", "locale plans could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"plans": values})
	})
	mux.HandleFunc("GET /repositories/{id}/locale-plans/{plan_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := plans.Get(r.PathValue("plan_id"), localeHead(git, catalog, r.PathValue("id")))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "locale_plan_not_found", "locale plan not found")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/locale-plans/{plan_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := plans.Get(r.PathValue("plan_id"), "")
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "locale_plan_not_found", "locale plan not found")
			return
		}
		var in localePlanInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete locale plan revision are required")
			return
		}
		if !localeResourcesResolve(git, current.RepositoryID, in.Revision.Resources) {
			writeAPIError(w, 422, "invalid_locale_source_revision", "every translatable resource must bind an existing exact commit in this repository")
			return
		}
		var out localeplans.Plan
		err = catalog.WithCurrentParticipants(localePlanParticipants(actor.UserID, in.Revision), current.RepositoryID, func() error {
			var e error
			out, e = plans.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
			return e
		})
		writeLocalePlan(w, out, err, 200)
	})
}
func localePlanParticipants(actor string, revision localeplans.Revision) []string {
	seen, values := map[string]bool{}, []string{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			values = append(values, id)
		}
	}
	add(actor)
	for _, locale := range revision.Locales {
		for _, id := range append(locale.OwnerIDs, locale.ReviewerIDs...) {
			add(id)
		}
	}
	for _, journey := range revision.Journeys {
		for _, id := range journey.OwnerIDs {
			add(id)
		}
	}
	return values
}
func localeResourcesResolve(git *storage.Store, repo string, resources []localeplans.Resource) bool {
	r, e := git.Open(repo)
	if e != nil {
		return false
	}
	for _, x := range resources {
		if _, e = r.ReadCommit(storage.ObjectID(x.SourceRevision)); e != nil {
			return false
		}
	}
	return true
}
func localeHead(git *storage.Store, catalog *repositories.Store, id string) string {
	meta, e := catalog.GetByID(id)
	if e != nil {
		return ""
	}
	r, e := git.Open(id)
	if e != nil {
		return ""
	}
	ref, e := r.ReadReference("refs/heads/" + meta.DefaultBranch)
	if e != nil {
		return ""
	}
	return ref.Target
}
func writeLocalePlan(w http.ResponseWriter, v localeplans.Plan, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, localeplans.ErrConflict):
		writeAPIError(w, 409, "locale_plan_conflict", "the locale plan changed; reload before publishing")
	case errors.Is(err, localeplans.ErrInvalid):
		writeAPIError(w, 400, "invalid_locale_plan", "the plan must completely define its subject, locales, formatting, journeys, exact resources, thresholds, and rationale")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "locale_plan_forbidden", "only a current repository participant may publish locale plans")
	default:
		log.Printf("locale plan storage: %v", err)
		writeAPIError(w, 500, "locale_plans_unavailable", "locale plans could not be persisted")
	}
}
