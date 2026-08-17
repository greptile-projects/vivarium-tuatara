package organizations

import (
	"slices"
	"sort"
	"strings"
	"time"
)

// AgentMatchRequest is a privacy-bounded description of work. The source is
// retained only in the response; matching never copies the source title/body.
type AgentMatchRequest struct {
	SourceKind          string          `json:"source_kind"`
	SourceID            string          `json:"source_id"`
	RepositoryID        string          `json:"repository_id,omitempty"`
	Workflow            string          `json:"workflow"`
	RequiredPermissions []ResourceScope `json:"required_permissions"`
	DeploymentBoundary  string          `json:"deployment_boundary,omitempty"`
}

type AgentMatch struct {
	AgentID              string                  `json:"agent_id"`
	Name                 string                  `json:"name"`
	Eligible             bool                    `json:"eligible"`
	Score                int                     `json:"score"`
	Reasons              []string                `json:"reasons"`
	MissingEvidence      []string                `json:"missing_evidence"`
	StaleEvidence        []string                `json:"stale_evidence"`
	Conflicts            []string                `json:"conflicts"`
	EffectivePermissions []ResourceScope         `json:"effective_permissions"`
	DeploymentBoundary   []string                `json:"deployment_boundary"`
	Pricing              string                  `json:"pricing,omitempty"`
	Availability         string                  `json:"availability,omitempty"`
	VerifiedEvaluations  []AgentVerifiedEvidence `json:"verified_evaluations"`
	ComparableOutcomes   []StewardshipOutcome    `json:"comparable_outcomes"`
}

type AgentMatchSet struct {
	SourceKind   string       `json:"source_kind"`
	SourceID     string       `json:"source_id"`
	RepositoryID string       `json:"repository_id,omitempty"`
	Workflow     string       `json:"workflow"`
	Explanation  string       `json:"explanation"`
	Matches      []AgentMatch `json:"matches"`
}

var matchSourceKinds = map[string]bool{"task": true, "proposal": true, "issue": true, "decision": true, "incident": true, "stewardship_mandate": true, "team_role": true}
var deploymentBoundaries = map[string]bool{"platform": true, "operator_managed": true, "customer_managed": true, "external_service": true}

func MatchAgents(v Organization, in AgentMatchRequest, now time.Time) (AgentMatchSet, error) {
	in.SourceKind, in.SourceID, in.RepositoryID, in.Workflow, in.DeploymentBoundary = strings.TrimSpace(in.SourceKind), strings.TrimSpace(in.SourceID), strings.TrimSpace(in.RepositoryID), strings.TrimSpace(in.Workflow), strings.TrimSpace(in.DeploymentBoundary)
	if !matchSourceKinds[in.SourceKind] || in.SourceID == "" || len(in.SourceID) > 200 || in.Workflow == "" || len(in.Workflow) > 300 || len(in.RequiredPermissions) > 50 || (in.RepositoryID != "" && !validID(in.RepositoryID)) || (in.DeploymentBoundary != "" && !deploymentBoundaries[in.DeploymentBoundary]) {
		return AgentMatchSet{}, ErrInvalid
	}
	for _, permission := range in.RequiredPermissions {
		if !validResourceKind(permission.Kind) || permission.ID == "" || len(permission.ID) > 200 {
			return AgentMatchSet{}, ErrInvalid
		}
	}
	out := AgentMatchSet{SourceKind: in.SourceKind, SourceID: in.SourceID, RepositoryID: in.RepositoryID, Workflow: in.Workflow, Explanation: "Candidates are ordered by disclosed workflow fit, live exact-resource grants, policy compatibility, current evidence, and comparable retained outcomes. The score is explanatory, not authority or an execution decision.", Matches: []AgentMatch{}}
	policy := EffectivePolicy{}
	if in.RepositoryID != "" {
		policy = EffectivePolicies(v, in.RepositoryID, ResponsibleTeamIDs(v, in.RepositoryID), false, now)
	}
	for _, agent := range v.Agents {
		m := AgentMatch{AgentID: agent.ID, Name: agent.Name, Eligible: true, Reasons: []string{}, MissingEvidence: []string{}, StaleEvidence: []string{}, Conflicts: []string{}, EffectivePermissions: []ResourceScope{}, DeploymentBoundary: []string{}, VerifiedEvaluations: []AgentVerifiedEvidence{}, ComparableOutcomes: []StewardshipOutcome{}}
		if len(agent.Profiles) == 0 {
			m.Eligible = false
			m.MissingEvidence = append(m.MissingEvidence, "No versioned agent profile is available.")
		} else {
			profile := agent.Profiles[len(agent.Profiles)-1]
			m.Pricing, m.Availability = profile.Pricing, profile.Availability
			m.DeploymentBoundary = append([]string{}, profile.DeploymentBoundaries...)
			workflow := strings.ToLower(in.Workflow)
			fit := false
			for _, supported := range append(append([]string{}, profile.SupportedTasks...), agent.Capabilities...) {
				candidate := strings.ToLower(supported)
				if candidate == workflow || strings.Contains(candidate, workflow) || strings.Contains(workflow, candidate) {
					fit = true
					break
				}
			}
			if fit {
				m.Score += 30
				m.Reasons = append(m.Reasons, "The current profile explicitly supports this workflow.")
			} else {
				m.Eligible = false
				m.MissingEvidence = append(m.MissingEvidence, "The current profile does not claim this workflow.")
			}
			if in.DeploymentBoundary == "" || slices.Contains(m.DeploymentBoundary, in.DeploymentBoundary) {
				m.Score += 10
				m.Reasons = append(m.Reasons, "The disclosed execution boundary matches the requested boundary.")
			} else {
				m.Eligible = false
				m.Conflicts = append(m.Conflicts, "The disclosed execution boundary does not match the requested boundary.")
			}
			if now.Sub(profile.PublishedAt) > 180*24*time.Hour {
				m.StaleEvidence = append(m.StaleEvidence, "The current profile is more than 180 days old.")
			} else {
				m.Score += 10
			}
			for _, evidence := range profile.VerifiedEvidence {
				if evidence.Kind != "stable_identity" && evidence.Kind != "current_operators" {
					m.VerifiedEvaluations = append(m.VerifiedEvaluations, evidence)
				}
			}
			if len(m.VerifiedEvaluations) == 0 {
				m.MissingEvidence = append(m.MissingEvidence, "No platform-verified evaluation is available for this workflow.")
			} else {
				m.Score += 10
			}
			if len(profile.ConflictDisclosures) == 0 {
				m.MissingEvidence = append(m.MissingEvidence, "No conflict-of-interest disclosure is available.")
			} else {
				m.Conflicts = append(m.Conflicts, profile.ConflictDisclosures...)
			}
		}
		for _, grant := range v.AccessGrants {
			if grant.PrincipalType != "agent" || grant.PrincipalID != agent.ID || grant.RevokedAt != nil || (grant.ExpiresAt != nil && !grant.ExpiresAt.After(now)) {
				continue
			}
			for _, resource := range grant.Resources {
				if !resourceDenied(grant, resource) && !slices.Contains(m.EffectivePermissions, resource) {
					m.EffectivePermissions = append(m.EffectivePermissions, resource)
				}
			}
		}
		missingPermission := false
		for _, required := range in.RequiredPermissions {
			if !slices.Contains(m.EffectivePermissions, required) {
				missingPermission = true
				m.MissingEvidence = append(m.MissingEvidence, "No live independent grant covers "+required.Kind+":"+required.ID+".")
			}
		}
		if missingPermission {
			m.Eligible = false
		} else if len(in.RequiredPermissions) > 0 {
			m.Score += 25
			m.Reasons = append(m.Reasons, "Live independent grants cover every requested resource.")
		}
		if policy.Rules.AgentAuthority == "disabled" {
			m.Eligible = false
			m.Conflicts = append(m.Conflicts, "Effective organization policy disables agent authority for this repository.")
		} else if in.RepositoryID != "" {
			m.Score += 10
			m.Reasons = append(m.Reasons, "No effective organization policy disables agent collaboration.")
		}
		for _, mandate := range v.StewardshipMandates {
			if len(mandate.Revisions) == 0 || mandate.Revisions[len(mandate.Revisions)-1].AgentID != agent.ID {
				continue
			}
			for _, outcome := range mandate.Outcomes {
				if strings.Contains(strings.ToLower(outcome.Summary+" "+outcome.Goal), strings.ToLower(in.Workflow)) {
					m.ComparableOutcomes = append(m.ComparableOutcomes, outcome)
				}
			}
		}
		if len(m.ComparableOutcomes) == 0 {
			m.MissingEvidence = append(m.MissingEvidence, "No attributed outcome on comparable work is available.")
		} else {
			m.Score += min(15, len(m.ComparableOutcomes)*5)
			m.Reasons = append(m.Reasons, "Attributed outcomes exist for comparable retained work.")
			for _, outcome := range m.ComparableOutcomes {
				m.VerifiedEvaluations = append(m.VerifiedEvaluations, AgentVerifiedEvidence{Kind: "attributed_outcome", Statement: outcome.Status + ": " + outcome.Summary, VerifiedAt: outcome.RecordedAt})
			}
		}
		out.Matches = append(out.Matches, m)
	}
	sort.SliceStable(out.Matches, func(i, j int) bool {
		if out.Matches[i].Eligible != out.Matches[j].Eligible {
			return out.Matches[i].Eligible
		}
		if out.Matches[i].Score != out.Matches[j].Score {
			return out.Matches[i].Score > out.Matches[j].Score
		}
		return out.Matches[i].Name < out.Matches[j].Name
	})
	return out, nil
}
