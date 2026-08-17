package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/durableschemas"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerDurableSchemaRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *durableschemas.Store, pulls *pullrequests.Store, decisionStore *decisions.Store) {
	base := "/repositories/{id}/durable-schemas"
	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "durable_schemas_unavailable", "durable schemas could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"schemas": v})
	})
	mux.HandleFunc("GET "+base+"/{schema_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := store.Get(r.PathValue("id"), r.PathValue("schema_id"))
		if e != nil {
			writeAPIError(w, 404, "durable_schema_not_found", "durable schema not found")
			return
		}
		writeJSON(w, 200, v)
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in struct {
				ExpectedVersion int                     `json:"expected_version"`
				Revision        durableschemas.Revision `json:"revision"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete reviewed schema revision is required")
				return
			}
			pr, e := pulls.Get(r.PathValue("id"), in.Revision.PullRequestID)
			if e != nil || pr.Status != pullrequests.Merged || pr.MergeCommitID == nil || *pr.MergeCommitID != in.Revision.ReviewedCommit {
				writeAPIError(w, 400, "invalid_reviewed_history", "schema revisions must cite the exact merge commit of a merged repository pull request")
				return
			}
			owners := append([]string{}, in.Revision.OwnerIDs...)
			var out durableschemas.Schema
			e = catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error {
				if revise {
					out, e = store.Revise(r.PathValue("id"), r.PathValue("schema_id"), in.ExpectedVersion, actor.UserID, in.Revision)
				} else {
					out, e = store.Create(r.PathValue("id"), actor.UserID, in.Revision)
				}
				return e
			})
			writeDurableSchema(w, out, e, map[bool]int{false: 201, true: 200}[revise])
		}
	}
	mux.HandleFunc("POST "+base, publish(false))
	mux.HandleFunc("POST "+base+"/{schema_id}/revisions", publish(true))
	mux.HandleFunc("POST "+base+"/{schema_id}/migrations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in durableschemas.Migration
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete migration plan is required")
			return
		}
		valid := false
		if in.SourceKind == "pull_request" {
			p, e := pulls.Get(r.PathValue("id"), in.SourceID)
			valid = e == nil && (p.Status == pullrequests.Open || p.Status == pullrequests.Merged)
		} else if in.SourceKind == "decision" && decisionStore != nil {
			d, e := decisionStore.Get(in.SourceID)
			if e == nil {
				for _, x := range d.Scope.AffectedResources {
					if x.RepositoryID == r.PathValue("id") {
						valid = true
						break
					}
				}
			}
		}
		if !valid {
			writeAPIError(w, 400, "invalid_migration_source", "migration source must be a visible repository pull request or affected decision")
			return
		}
		owners := []string{}
		for _, o := range in.Operations {
			owners = append(owners, o.OwnerIDs...)
		}
		for _, s := range in.Steps {
			owners = append(owners, s.RequiredApproverIDs...)
		}
		var out durableschemas.Schema
		e := catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error {
			var x error
			out, x = store.AddMigration(r.PathValue("id"), r.PathValue("schema_id"), actor.UserID, in)
			return x
		})
		writeDurableSchema(w, out, e, 201)
	})
	mux.HandleFunc("POST "+base+"/{schema_id}/migrations/{migration_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                  `json:"expected_version"`
			Event           durableschemas.Event `json:"event"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributable migration event is required")
			return
		}
		out, e := store.AddEvent(r.PathValue("id"), r.PathValue("schema_id"), r.PathValue("migration_id"), actor.UserID, in.ExpectedVersion, in.Event)
		writeDurableSchema(w, out, e, 200)
	})
}
func writeDurableSchema(w http.ResponseWriter, v durableschemas.Schema, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, durableschemas.ErrConflict):
		writeAPIError(w, 409, "durable_schema_conflict", "the durable-state record changed; reload before writing")
	case errors.Is(e, durableschemas.ErrInvalid):
		writeAPIError(w, 400, "invalid_durable_schema", "define reviewed schema provenance, owners, compatibility, retention, privacy, and a completely sequenced migration")
	case errors.Is(e, repositories.ErrInvalidCollaborator), errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 403, "durable_schema_forbidden", "all owners and approvers must be current repository participants")
	case errors.Is(e, durableschemas.ErrNotFound):
		writeAPIError(w, 404, "durable_schema_not_found", "durable schema not found")
	default:
		log.Printf("durable schema storage: %v", e)
		writeAPIError(w, 500, "durable_schemas_unavailable", "durable schema could not be persisted")
	}
}
