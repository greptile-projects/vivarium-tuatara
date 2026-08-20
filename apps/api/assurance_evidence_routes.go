package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerAssuranceEvidenceRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, programs *assuranceprograms.Store, evidence *assuranceevidence.Store) {
	mux.HandleFunc("GET /repositories/{id}/assurance-evidence", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		definitions, e := evidence.ListDefinitions(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "assurance_evidence_unavailable", "assurance evidence could not be read")
			return
		}
		packages, e := evidence.ListPackages(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "assurance_evidence_unavailable", "assurance evidence could not be read")
			return
		}
		visible := map[string]bool{}
		filteredDefinitions := definitions[:0]
		for _, definition := range definitions {
			if evidenceAudienceAllows(definition.Audience, actor.UserID, authenticated) {
				visible[definition.ID] = true
				filteredDefinitions = append(filteredDefinitions, definition)
			}
		}
		definitions = filteredDefinitions
		filteredPackages := packages[:0]
		for _, evidencePackage := range packages {
			if visible[evidencePackage.DefinitionID] {
				filteredPackages = append(filteredPackages, evidencePackage)
			}
		}
		packages = filteredPackages
		writeJSON(w, 200, map[string]any{"definitions": definitions, "packages": packages})
	})
	mux.HandleFunc("POST /repositories/{id}/assurance-evidence/definitions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in assuranceevidence.Definition
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete evidence definition is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		in.OwnerID = actor.UserID
		program, e := programs.Get(in.ProgramID)
		if e != nil || program.RepositoryID != in.RepositoryID || in.ProgramVersion < 1 || in.ProgramVersion > len(program.Revisions) {
			writeAPIError(w, 400, "invalid_control_version", "the evidence definition must bind an exact assurance program revision")
			return
		}
		revision := program.Revisions[in.ProgramVersion-1]
		found := false
		owner := false
		for _, c := range revision.Controls {
			if c.ID == in.ControlID {
				found = true
				for _, id := range c.OwnerIDs {
					if id == actor.UserID {
						owner = true
					}
				}
			}
		}
		if !found || !owner {
			writeAPIError(w, 403, "control_owner_required", "only a named owner of the exact control version may define its evidence")
			return
		}
		for _, id := range in.Audience {
			if id == "repository" {
				continue
			}
			if e := catalog.WithCurrentParticipants([]string{id}, in.RepositoryID, func() error { return nil }); e != nil {
				writeAPIError(w, 400, "invalid_evidence_audience", "audience members must be current repository participants")
				return
			}
		}
		out, e := evidence.CreateDefinition(in)
		writeAssuranceEvidence(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/assurance-evidence/definitions/{definition_id}/packages", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		definition, e := evidence.GetDefinition(r.PathValue("definition_id"))
		if e != nil || definition.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "assurance_evidence_not_found", "evidence definition not found")
			return
		}
		if definition.OwnerID != actor.UserID {
			writeAPIError(w, 403, "control_owner_required", "only the evidence definition owner may collect a package")
			return
		}
		var in struct {
			Sources []assuranceevidence.Source `json:"sources"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "source records are required")
			return
		}
		out, e := evidence.CreatePackage(definition, actor.UserID, in.Sources)
		writeAssuranceEvidence(w, out, e, 201)
	})
}

func evidenceAudienceAllows(audience []string, userID string, authenticated bool) bool {
	for _, member := range audience {
		if member == "repository" || (authenticated && member == userID) {
			return true
		}
	}
	return false
}

func writeAssuranceEvidence(w http.ResponseWriter, v any, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, assuranceevidence.ErrNotFound):
		writeAPIError(w, 404, "assurance_evidence_not_found", "assurance evidence not found")
	case errors.Is(e, assuranceevidence.ErrInvalid):
		writeAPIError(w, 400, "invalid_assurance_evidence", "periods, schedule, queries, source provenance, and transformations must be complete and consistent")
	default:
		writeAPIError(w, 500, "assurance_evidence_unavailable", "assurance evidence could not be persisted")
	}
}
