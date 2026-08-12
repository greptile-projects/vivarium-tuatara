package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/charters"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/governance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type charterPreview struct {
	Valid                bool           `json:"valid"`
	Blockers             []string       `json:"blockers"`
	Relationships        []string       `json:"relationships"`
	EligibleParticipants int            `json:"eligible_participants"`
	DecisionEligibility  map[string]int `json:"decision_eligibility"`
}
type charterStandingView struct {
	charters.Standing
	EffectiveStatus     string   `json:"effective_status"`
	Eligibility         string   `json:"eligibility"`
	AvailableActions    []string `json:"available_actions"`
	NominationAvailable bool     `json:"nomination_available"`
	OperationalAccess   []string `json:"operational_access"`
	AuthorityNote       string   `json:"authority_note"`
}
type charterContinuityView struct {
	charters.ContinuityAction
	EffectiveStatus string `json:"effective_status"`
	ReviewRequired  bool   `json:"review_required"`
	AuthorityNote   string `json:"authority_note"`
}

func registerCharterRoutes(mux *http.ServeMux, store *charters.Store, governed *governance.Store, repos *repositories.Store, orgs *organizations.Store, credentials *auth.Store) {
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
		p := charterPreview{Valid: true, Blockers: []string{}, Relationships: []string{}, DecisionEligibility: map[string]int{}}
		identitySources := map[string]map[string]bool{}
		add := func(source, identity string) {
			if identitySources[source] == nil {
				identitySources[source] = map[string]bool{}
			}
			identitySources[source][identity] = true
		}
		if kind == "repository" {
			repo, err := repos.GetByID(id)
			if err != nil {
				p.Blockers = append(p.Blockers, "Repository ownership is unavailable.")
			} else {
				add("repository_owner", repo.OwnerID)
				collabs, _ := repos.ListCollaborators(repo.OwnerID, id)
				for _, collaborator := range collabs {
					add("repository_collaborator", collaborator.UserID)
				}
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
				add("organization_owner", o.CreatedBy)
				for _, member := range o.Members {
					add("organization_member", member.UserID)
				}
				for _, team := range o.Teams {
					for _, member := range team.Members {
						if member.Role == "maintainer" {
							add("team_maintainer", member.UserID)
						}
					}
				}
				for _, agent := range o.Agents {
					add("approved_agent", "agent:"+agent.ID)
				}
				p.Relationships = append(p.Relationships, "Organization ownership, team responsibility, agent grants, and active policy remain independently enforced.")
			}
		}
		roleEligibility := map[string]map[string]bool{}
		for _, role := range v.Roles {
			identities := map[string]bool{}
			for _, source := range role.Eligibility {
				for identity := range identitySources[source] {
					identities[identity] = true
				}
			}
			roleEligibility[role.Name] = identities
		}
		for _, d := range v.DecisionClasses {
			identities := map[string]bool{}
			for _, role := range d.EligibleRoles {
				for identity := range roleEligibility[role] {
					identities[identity] = true
				}
			}
			eligible := len(identities)
			p.DecisionEligibility[d.Name] = eligible
			if eligible > p.EligibleParticipants {
				p.EligibleParticipants = eligible
			}
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
	standingViews := func(kind, id, actorID string, record charters.Record) []charterStandingView {
		live := map[string]bool{}
		access := map[string][]string{}
		ownerID := ""
		if kind == "repository" {
			if repo, err := repos.GetByID(id); err == nil {
				ownerID = repo.OwnerID
				live[repo.OwnerID] = true
				access[repo.OwnerID] = []string{"repository owner access (independent of charter)"}
				if collaborators, err := repos.ListCollaborators(repo.OwnerID, id); err == nil {
					for _, c := range collaborators {
						live[c.UserID] = true
						access[c.UserID] = []string{"repository collaborator access (independent of charter)"}
					}
				}
			}
		} else if orgs != nil {
			if org, err := orgs.Get(id); err == nil {
				ownerID = org.CreatedBy
				live[org.CreatedBy] = true
				access[org.CreatedBy] = []string{"organization owner access (independent of charter)"}
				for _, m := range org.Members {
					live[m.UserID] = true
					access[m.UserID] = []string{"organization membership (independent of charter)"}
				}
			}
		}
		now := time.Now()
		out := make([]charterStandingView, 0, len(record.Standings))
		for _, st := range record.Standings {
			effective := st.Status
			eligibility := "evidence approved under charter revision"
			if !st.ExpiresAt.After(now) {
				effective = "expired"
				eligibility = "term expired"
			} else if !live[st.PrincipalID] && len(st.Evidence) > 0 && allRelationshipEvidence(st.Evidence) {
				effective = "identity_revoked"
				eligibility = "the ownership or membership evidence that established standing is no longer live"
			}
			actions := []string{}
			nominationAvailable := false
			if actorID == st.PrincipalID {
				if st.Status == "invited" {
					actions = []string{"accept", "decline"}
				}
				if effective == "active" {
					actions = append(actions, "recuse")
					nominationAvailable = true
				}
				if st.Status == "suspended" || st.Status == "revoked" {
					actions = append(actions, "appeal")
				}
			}
			if actorID == ownerID && actorID != st.PrincipalID && st.ExpiresAt.After(now) {
				if st.Status == "active" || st.Status == "recused" {
					actions = append(actions, "suspend")
				}
				if st.Status == "suspended" {
					actions = append(actions, "reinstate")
				}
				if st.Status != "revoked" {
					actions = append(actions, "revoke")
				}
			}
			out = append(out, charterStandingView{Standing: st, EffectiveStatus: effective, Eligibility: eligibility, AvailableActions: actions, NominationAvailable: nominationAvailable, OperationalAccess: access[st.PrincipalID], AuthorityNote: "Governance standing and votes grant no code, secret, merge, deployment, or credential authority."})
		}
		return out
	}
	continuityViews := func(record charters.Record) []charterContinuityView {
		now := time.Now()
		out := make([]charterContinuityView, 0, len(record.Continuity))
		for _, x := range record.Continuity {
			status := x.Status
			if x.Status == "active" && !x.ExpiresAt.After(now) {
				status = "expired"
			}
			out = append(out, charterContinuityView{ContinuityAction: x, EffectiveStatus: status, ReviewRequired: (x.Status == "pending" || x.Status == "active") && !x.ReviewAt.After(now), AuthorityNote: "Continuity approval does not itself grant resource access; independent owners must approve and revoke derived credentials."})
		}
		return out
	}
	verifyGovernance := func(scopeType, scopeID string, version int, action *charters.ContinuityAction) bool {
		if governed == nil {
			return false
		}
		proposal, err := governed.Get(action.GovernanceProposalID)
		if err != nil || proposal.ScopeType != scopeType || proposal.ScopeID != scopeID || proposal.CharterVersion != version || proposal.Tally == nil || proposal.Tally.Status != "accepted" || proposal.Tally.Contested || proposal.Tally.Result == "" {
			return false
		}
		if action.GovernanceTallySHA256 != "" && action.GovernanceTallySHA256 != proposal.Tally.VerificationSHA256 {
			return false
		}
		action.GovernanceTallySHA256 = proposal.Tally.VerificationSHA256
		return action.GovernanceTallySHA256 != ""
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
			writeJSON(w, 200, map[string]any{"charter": record, "preview": preview(kind, r.PathValue("id"), current), "standing": standingViews(kind, r.PathValue("id"), actor.UserID, record), "continuity": continuityViews(record)})
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
		mux.HandleFunc("POST "+prefix+"/standing", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authorize(w, r, kind, r.PathValue("id"), "repositories:write", true)
			if !ok {
				return
			}
			var in struct {
				ExpectedVersion  int                 `json:"expected_version"`
				CharterVersion   int                 `json:"charter_version"`
				PrincipalType    string              `json:"principal_type"`
				PrincipalID      string              `json:"principal_id"`
				Role             string              `json:"role"`
				Responsibilities string              `json:"responsibilities"`
				Evidence         []charters.Evidence `json:"evidence"`
				ExpiresAt        time.Time           `json:"expires_at"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_standing", "standing invitation is required")
				return
			}
			record, err := store.Invite(kind, r.PathValue("id"), actor.UserID, in.ExpectedVersion, in.CharterVersion, in.PrincipalType, in.PrincipalID, in.Role, in.Responsibilities, in.Evidence, in.ExpiresAt)
			if !writeCharterError(w, err) {
				writeJSON(w, 201, map[string]any{"charter": record, "standing": standingViews(kind, r.PathValue("id"), actor.UserID, record)})
			}
		})
		mux.HandleFunc("POST "+prefix+"/standing/{standingID}/actions", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
			if !ok {
				return
			}
			var in struct {
				Action             string `json:"action"`
				Reason             string `json:"reason"`
				ConflictOfInterest string `json:"conflict_of_interest"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_standing_action", "standing action is required")
				return
			}
			selfAction := map[string]bool{"accept": true, "decline": true, "recuse": true, "appeal": true}[in.Action]
			if !selfAction {
				if _, ok := authorize(w, r, kind, r.PathValue("id"), "repositories:write", true); !ok {
					return
				}
			}
			record, err := store.ActOnStanding(kind, r.PathValue("id"), r.PathValue("standingID"), actor.UserID, in.Action, in.Reason, in.ConflictOfInterest)
			if !writeCharterError(w, err) {
				writeJSON(w, 200, map[string]any{"charter": record, "standing": standingViews(kind, r.PathValue("id"), actor.UserID, record)})
			}
		})
		mux.HandleFunc("GET "+prefix+"/standing/{standingID}", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
			if !ok {
				return
			}
			record, err := store.Get(kind, r.PathValue("id"))
			if err != nil {
				writeCharterError(w, err)
				return
			}
			views := standingViews(kind, r.PathValue("id"), actor.UserID, record)
			for _, view := range views {
				if view.ID == r.PathValue("standingID") && view.PrincipalID == actor.UserID {
					writeJSON(w, 200, view)
					return
				}
			}
			writeAPIError(w, 404, "standing_not_found", "governance standing not found")
		})
		mux.HandleFunc("POST "+prefix+"/continuity", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authorize(w, r, kind, r.PathValue("id"), "repositories:write", true)
			if !ok {
				return
			}
			var in struct {
				ExpectedVersion int                       `json:"expected_version"`
				CharterVersion  int                       `json:"charter_version"`
				Action          charters.ContinuityAction `json:"action"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_continuity", "continuity action is required")
				return
			}
			if !verifyGovernance(kind, r.PathValue("id"), in.CharterVersion, &in.Action) {
				writeAPIError(w, 422, "invalid_governance_result", "continuity requires an accepted, uncontested result for this active charter revision")
				return
			}
			record, err := store.CreateContinuity(kind, r.PathValue("id"), actor.UserID, in.ExpectedVersion, in.CharterVersion, in.Action)
			if !writeCharterError(w, err) {
				writeJSON(w, 201, map[string]any{"charter": record, "continuity": continuityViews(record)})
			}
		})
		mux.HandleFunc("POST "+prefix+"/continuity/{actionID}/actions", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authorize(w, r, kind, r.PathValue("id"), "repositories:write", true)
			if !ok {
				return
			}
			var in struct {
				Action string `json:"action"`
				Reason string `json:"reason"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_continuity_action", "action and reason are required")
				return
			}
			if in.Action == "approve" {
				record, err := store.Get(kind, r.PathValue("id"))
				if err != nil {
					writeCharterError(w, err)
					return
				}
				var continuity *charters.ContinuityAction
				for i := range record.Continuity {
					if record.Continuity[i].ID == r.PathValue("actionID") {
						continuity = &record.Continuity[i]
						break
					}
				}
				if continuity == nil || !verifyGovernance(kind, r.PathValue("id"), continuity.CharterVersion, continuity) {
					writeAPIError(w, 409, "governance_result_changed", "the governed result is unavailable or no longer matches this continuity action")
					return
				}
			}
			record, err := store.ActOnContinuity(kind, r.PathValue("id"), r.PathValue("actionID"), actor.UserID, in.Action, in.Reason)
			if !writeCharterError(w, err) {
				writeJSON(w, 200, map[string]any{"charter": record, "continuity": continuityViews(record)})
			}
		})
	}
}
func allRelationshipEvidence(evidence []charters.Evidence) bool {
	for _, item := range evidence {
		if item.Kind != "ownership" && item.Kind != "membership" {
			return false
		}
	}
	return true
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
