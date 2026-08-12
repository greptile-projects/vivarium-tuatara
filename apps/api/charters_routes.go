package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/charters"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type charterPreview struct {
	Valid                bool     `json:"valid"`
	Blockers             []string `json:"blockers"`
	Relationships        []string `json:"relationships"`
	EligibleParticipants int      `json:"eligible_participants"`
}

func registerCharterRoutes(mux *http.ServeMux, store *charters.Store, repos *repositories.Store, orgs *organizations.Store, credentials *auth.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request, kind, id, scope string, write bool) (auth.Credential, bool) {
		if kind == "repository" {
			if !write {
				actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, id)
				return actor, ok
			}
			actor, owner, ok := authorizeRepositoryParticipant(w, r, repos, credentials, id, scope)
			if !ok {
				return actor, false
			}
			if write && !owner {
				writeAPIError(w, 403, "owner_required", "only the repository owner may change its charter")
				return actor, false
			}
			return actor, true
		}
		actor, ok := authenticateRequest(w, r, credentials, scope, false)
		if !ok {
			return actor, false
		}
		if orgs == nil {
			writeAPIError(w, 404, "organization_not_found", "organization not found")
			return actor, false
		}
		o, err := orgs.Get(id)
		if err != nil {
			writeAPIError(w, 404, "organization_not_found", "organization not found")
			return actor, false
		}
		member := o.CreatedBy == actor.UserID
		for _, m := range o.Members {
			if m.UserID == actor.UserID {
				member = true
			}
		}
		if !member {
			writeAPIError(w, 404, "organization_not_found", "organization not found")
			return actor, false
		}
		if write && o.CreatedBy != actor.UserID {
			writeAPIError(w, 403, "owner_required", "only the organization owner may change its charter")
			return actor, false
		}
		return actor, true
	}
	preview := func(kind, id string, v charters.Revision) charterPreview {
		p := charterPreview{Valid: true, Blockers: []string{}, Relationships: []string{}}
		eligible := 1
		if kind == "repository" {
			repo, err := repos.GetByID(id)
			if err != nil {
				p.Blockers = append(p.Blockers, "Repository ownership is unavailable.")
			} else {
				collabs, _ := repos.ListCollaborators(repo.OwnerID, id)
				eligible += len(collabs)
				p.Relationships = append(p.Relationships, "Repository owner and current collaborators are eligible identity sources.")
				checks, _ := repos.RequiredChecks(id, repo.DefaultBranch)
				if len(checks) > 0 {
					p.Relationships = append(p.Relationships, "Default-branch required checks remain independently enforced.")
				}
				if repo.OrganizationID != "" {
					p.Relationships = append(p.Relationships, "Organization teams and active policy remain independently authoritative.")
				}
			}
		} else if orgs != nil {
			o, err := orgs.Get(id)
			if err != nil {
				p.Blockers = append(p.Blockers, "Organization ownership is unavailable.")
			} else {
				eligible = len(o.Members)
				if eligible < 1 {
					eligible = 1
				}
				p.Relationships = append(p.Relationships, "Organization ownership, team responsibility, agent grants, and active policy remain independently enforced.")
			}
		}
		p.EligibleParticipants = eligible
		for _, d := range v.DecisionClasses {
			if d.Participation > eligible || d.Quorum > eligible {
				p.Blockers = append(p.Blockers, "Decision class "+d.Name+" requires more participants than are currently eligible.")
			}
			for _, resource := range d.ProtectedResources {
				parts := strings.SplitN(resource, ":", 2)
				if len(parts) != 2 || !map[string]bool{"branch": true, "release": true, "environment": true, "security": true, "agent": true}[parts[0]] {
					p.Blockers = append(p.Blockers, "Protected resource "+resource+" is not a supported branch, release, environment, security, or agent reference.")
				}
			}
		}
		p.Valid = len(p.Blockers) == 0
		return p
	}
	for _, kind := range []string{"repository", "organization"} {
		prefix := "/repositories/{id}/charter"
		if kind == "organization" {
			prefix = "/organizations/{id}/charter"
		}
		mux.HandleFunc("GET "+prefix, func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authorize(w, r, kind, r.PathValue("id"), "repositories:read", false)
			_ = actor
			if !ok {
				return
			}
			record, err := store.Get(kind, r.PathValue("id"))
			if errors.Is(err, charters.ErrNotFound) {
				writeAPIError(w, 404, "charter_not_found", "no charter has been published")
				return
			}
			if err != nil {
				writeAPIError(w, 500, "charter_read_failed", "charter could not be read")
				return
			}
			current := record.Revisions[len(record.Revisions)-1]
			writeJSON(w, 200, map[string]any{"charter": record, "preview": preview(kind, r.PathValue("id"), current)})
		})
		mux.HandleFunc("POST "+prefix+"/revisions", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authorize(w, r, kind, r.PathValue("id"), "repositories:write", true)
			if !ok {
				return
			}
			var in struct {
				ExpectedVersion int               `json:"expected_version"`
				Charter         charters.Revision `json:"charter"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_charter", "charter content is required")
				return
			}
			record, err := store.Publish(kind, r.PathValue("id"), actor.UserID, in.ExpectedVersion, in.Charter)
			writeCharterError(w, err)
			if err == nil {
				writeJSON(w, 201, map[string]any{"charter": record, "preview": preview(kind, r.PathValue("id"), record.Revisions[len(record.Revisions)-1])})
			}
		})
		mux.HandleFunc("POST "+prefix+"/revisions/{version}/approvals", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authorize(w, r, kind, r.PathValue("id"), "repositories:write", true)
			if !ok {
				return
			}
			var in struct {
				Version  int    `json:"version"`
				Decision string `json:"decision"`
				Reason   string `json:"reason"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_approval", "approval is required")
				return
			}
			record, err := store.Approve(kind, r.PathValue("id"), actor.UserID, in.Version, in.Decision, in.Reason)
			if !writeCharterError(w, err) {
				writeJSON(w, 201, record)
			}
		})
		mux.HandleFunc("POST "+prefix+"/revisions/{version}/activate", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authorize(w, r, kind, r.PathValue("id"), "repositories:write", true)
			if !ok {
				return
			}
			var in struct {
				Version int `json:"version"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_activation", "version is required")
				return
			}
			record, err := store.Get(kind, r.PathValue("id"))
			if err != nil {
				writeCharterError(w, err)
				return
			}
			if in.Version < 1 || in.Version > len(record.Revisions) {
				writeAPIError(w, 409, "charter_changed", "charter version changed")
				return
			}
			assessment := preview(kind, r.PathValue("id"), record.Revisions[in.Version-1])
			if !assessment.Valid {
				writeJSON(w, 422, map[string]any{"error": map[string]string{"code": "charter_conflict", "message": "charter conflicts with current project authority"}, "preview": assessment})
				return
			}
			record, err = store.Activate(kind, r.PathValue("id"), actor.UserID, in.Version)
			if !writeCharterError(w, err) {
				writeJSON(w, 200, map[string]any{"charter": record, "preview": assessment})
			}
		})
		mux.HandleFunc("POST "+prefix+"/exceptions", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authorize(w, r, kind, r.PathValue("id"), "repositories:write", true)
			if !ok {
				return
			}
			var in struct {
				Version       int       `json:"version"`
				DecisionClass string    `json:"decision_class"`
				Resource      string    `json:"resource"`
				Reason        string    `json:"reason"`
				ExpiresAt     time.Time `json:"expires_at"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_exception", "exception is required")
				return
			}
			record, err := store.Except(kind, r.PathValue("id"), actor.UserID, in.Version, in.DecisionClass, in.Resource, in.Reason, in.ExpiresAt)
			if !writeCharterError(w, err) {
				writeJSON(w, 201, record)
			}
		})
	}
}
func writeCharterError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, charters.ErrNotFound):
		writeAPIError(w, 404, "charter_not_found", "charter not found")
	case errors.Is(err, charters.ErrConflict):
		writeAPIError(w, 409, "charter_changed", "charter version changed")
	case errors.Is(err, charters.ErrInvalid):
		writeAPIError(w, 422, "invalid_charter", "charter rules or approval are invalid")
	default:
		writeAPIError(w, 500, "charter_write_failed", "charter could not be written")
	}
	return true
}
