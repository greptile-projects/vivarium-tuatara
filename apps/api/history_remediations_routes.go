package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/historyremediations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerHistoryRemediationRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *historyremediations.Store) {
	actorID := func(c auth.Credential) string {
		if c.AgentID != "" {
			return c.AgentID
		}
		return c.UserID
	}
	canSee := func(v historyremediations.Remediation, actor string) bool {
		if v.CreatedBy == actor {
			return true
		}
		for _, ids := range [][]string{v.AudienceIDs, v.OwnerIDs} {
			for _, id := range ids {
				if id == actor {
					return true
				}
			}
		}
		for _, a := range v.RequiredApprovals {
			for _, id := range a.ApproverIDs {
				if id == actor {
					return true
				}
			}
		}
		return false
	}
	public := func(v historyremediations.Remediation) historyremediations.Remediation {
		v.RequestDigest = ""
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/history-remediations", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		xs, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "history_remediations_unavailable", "history remediations could not be read")
			return
		}
		out := []historyremediations.Remediation{}
		for _, v := range xs {
			if canSee(v, actorID(c)) {
				out = append(out, public(v))
			}
		}
		writeJSON(w, 200, map[string]any{"history_remediations": out})
	})
	mux.HandleFunc("GET /repositories/{id}/history-remediations/{remediation_id}", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := store.Get(r.PathValue("id"), r.PathValue("remediation_id"))
		if e != nil || !canSee(v, actorID(c)) {
			writeAPIError(w, 404, "history_remediation_not_found", "history remediation not found")
			return
		}
		writeJSON(w, 200, public(v))
	})
	mux.HandleFunc("POST /repositories/{id}/history-remediations", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in historyremediations.Remediation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a history remediation is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		// Every named participant and affected repository is resolved before restricted state is retained.
		people := append(append([]string{}, in.AudienceIDs...), in.OwnerIDs...)
		for _, a := range in.RequiredApprovals {
			people = append(people, a.ApproverIDs...)
		}
		for _, id := range people {
			member, _ := catalog.HasCollaborator(id, in.RepositoryID)
			repo, _ := catalog.GetByID(in.RepositoryID)
			if repo.ID == "" || (repo.OwnerID != id && !member) {
				writeAPIError(w, 422, "history_remediation_participant_invalid", "audience, owners, and approvers must be current source-repository participants")
				return
			}
		}
		for _, scope := range in.Scopes {
			repo, e := catalog.GetByID(scope.RepositoryID)
			if e != nil {
				writeAPIError(w, 422, "history_remediation_scope_unavailable", "every affected repository must resolve")
				return
			}
			allowed, _ := catalog.HasCollaborator(c.UserID, scope.RepositoryID)
			if repo.OwnerID != c.UserID && !allowed {
				writeAPIError(w, 403, "history_remediation_scope_forbidden", "the creator must maintain every affected repository")
				return
			}
			if scope.Revision != "" && (len(scope.Revision) != 40 || !catalog.HasCommit(scope.RepositoryID, scope.Revision)) {
				writeAPIError(w, 422, "history_remediation_object_missing", "exact scoped revisions must resolve")
				return
			}
		}
		clean := in
		clean.ID = ""
		clean.RequestDigest = ""
		clean.CreatedBy = ""
		clean.CreatedAt = clean.CreatedAt.UTC()
		clean.Authority = ""
		b, _ := json.Marshal(clean)
		sum := sha256.Sum256(b)
		out, e := store.Create(in, actorID(c), hex.EncodeToString(sum[:]))
		if errors.Is(e, historyremediations.ErrConflict) {
			writeAPIError(w, 409, "history_remediation_request_conflict", "request_id was already used for a different remediation")
			return
		}
		if errors.Is(e, historyremediations.ErrInvalid) {
			writeAPIError(w, 422, "history_remediation_invalid", "source, payload-free content description, exact scope, discovery digests, audience, owners, and approvals are required")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "history_remediation_unavailable", "history remediation could not be opened")
			return
		}
		writeJSON(w, 201, public(out))
	})
}
