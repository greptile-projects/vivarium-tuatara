package main

import (
	"errors"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/localization"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"net/http"
)

type extractionInput struct {
	SourceRevision string                     `json:"source_revision"`
	Map            localization.ExtractionMap `json:"map"`
	Locales        []string                   `json:"locales"`
	Units          []localization.Unit        `json:"units"`
}
type translationInput struct {
	SourceRevision string `json:"source_revision"`
	UnitID         string `json:"unit_id"`
	Locale         string `json:"locale"`
	Text           string `json:"text"`
	Note           string `json:"note"`
}

func registerLocalizationRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, store *localization.Store) {
	pull := func(w http.ResponseWriter, r *http.Request) (pullrequests.PullRequest, bool) {
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeAPIError(w, 404, "pull_not_found", "pull request not found")
			return p, false
		}
		return p, true
	}
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/localization", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		p, ok := pull(w, r)
		if !ok {
			return
		}
		v, e := store.Get(p.RepositoryID, p.ID, p.SourceCommitID)
		if errors.Is(e, localization.ErrNotFound) {
			writeJSON(w, 200, map[string]any{"repository_id": p.RepositoryID, "pull_id": p.ID, "current_revision": p.SourceCommitID, "extractions": []any{}, "translations": []any{}, "counts": map[string]any{}})
			return
		}
		if e != nil {
			writeAPIError(w, 500, "localization_unavailable", "localization review could not be read")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/localization/extractions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		p, ok := pull(w, r)
		if !ok {
			return
		}
		var in extractionInput
		if decodeJSON(r, &in) != nil || in.SourceRevision != p.SourceCommitID {
			writeAPIError(w, 409, "localization_revision_changed", "extraction must match the current pull source revision")
			return
		}
		v, e := store.Extract(p.RepositoryID, p.ID, in.SourceRevision, actor.UserID, in.Map, in.Locales, in.Units)
		if e != nil {
			writeAPIError(w, 400, "invalid_localization_extraction", "a complete repository-defined map and contextual units are required")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/localization/translations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" {
			writeAuthenticationRequired(w, false)
			return
		}
		p, ok := pull(w, r)
		if !ok {
			return
		}
		var in translationInput
		if decodeJSON(r, &in) != nil || in.SourceRevision != p.SourceCommitID {
			writeAPIError(w, 409, "localization_revision_changed", "translation must match the current pull source revision")
			return
		}
		v, e := store.Propose(p.RepositoryID, p.ID, in.SourceRevision, in.UnitID, in.Locale, in.Text, in.Note, actor.UserID)
		if e != nil {
			writeAPIError(w, 400, "invalid_translation", "the unit, locale, source revision, and translation must match the current extraction")
			return
		}
		writeJSON(w, 201, v)
	})
}
