package main

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/reviewplans"
)

type reviewAssignmentInput struct {
	RequestID      string     `json:"request_id"`
	PlanID         string     `json:"plan_id"`
	AreaID         string     `json:"area_id"`
	PrincipalType  string     `json:"principal_type"`
	PrincipalID    string     `json:"principal_id"`
	AgentGrantID   string     `json:"agent_grant_id"`
	Deadline       *time.Time `json:"deadline"`
	EscalationPath string     `json:"escalation_path"`
}
type reviewAssignmentTransition struct {
	Action         string     `json:"action"`
	Reason         string     `json:"reason"`
	RequestID      string     `json:"request_id"`
	PrincipalType  string     `json:"principal_type"`
	PrincipalID    string     `json:"principal_id"`
	AgentGrantID   string     `json:"agent_grant_id"`
	Deadline       *time.Time `json:"deadline"`
	EscalationPath string     `json:"escalation_path"`
}

func registerReviewAssignmentRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, plans *reviewplans.Store, orgs *organizations.Store) {
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/review-assignments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		repo, e := catalog.GetByID(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		p, e := pulls.Get(repo.ID, r.PathValue("pull_id"))
		if e != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		history, e := plans.List(repo.ID, p.ID, p.SourceCommitID, p.TargetCommitID)
		if e != nil || len(history) == 0 {
			writeAPIError(w, 409, "review_plan_required", "a current review plan is required")
			return
		}
		plan := history[len(history)-1]
		assignments, e := plans.ListAssignments(repo.ID, p.ID)
		if e != nil {
			writeAPIError(w, 500, "review_assignments_unavailable", "review assignments could not be read")
			return
		}
		repositoryAssignments, e := plans.ListRepositoryAssignments(repo.ID)
		if e != nil {
			writeAPIError(w, 500, "review_assignments_unavailable", "review assignment capacity could not be read")
			return
		}
		suggestions := deriveReviewerSuggestions(repo, p, plan, repositoryAssignments, pulls, orgs)
		assignments = projectReviewAssignments(assignments, repo, orgs)
		writeJSON(w, 200, map[string]any{"plan_id": plan.ID, "plan_version": plan.Version, "suggestions": suggestions, "assignments": assignments, "viewer_id": actor.UserID})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/review-assignments", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok || actor.AgentID != "" {
			return
		}
		var in reviewAssignmentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "review assignment input is invalid")
			return
		}
		repo, e := catalog.GetByID(r.PathValue("id"))
		if e != nil || repo.OwnerID != actor.UserID {
			writeAPIError(w, 403, "review_assignment_forbidden", "only the current repository owner can invite reviewers")
			return
		}
		p, e := pulls.Get(repo.ID, r.PathValue("pull_id"))
		if e != nil || p.Status != pullrequests.Open {
			writeAPIError(w, 404, "pull_request_not_found", "open pull request not found")
			return
		}
		plan, area, ok := currentReviewArea(w, plans, p, in.PlanID, in.AreaID)
		if !ok {
			return
		}
		candidates, e := plans.ListRepositoryAssignments(repo.ID)
		if e != nil {
			writeAPIError(w, 503, "review_capacity_unavailable", "reviewer capacity could not be resolved")
			return
		}
		suggestion, eligible := findSuggestion(deriveReviewerSuggestions(repo, p, plan, candidates, pulls, orgs), in.PrincipalType, in.PrincipalID, in.AgentGrantID, area.ID)
		if !eligible || !suggestion.Eligible {
			writeAPIError(w, 422, "reviewer_ineligible", "reviewer is unavailable, conflicted, overloaded, revoked, or lacks permitted evidence")
			return
		}
		value := reviewplans.Assignment{RequestID: in.RequestID, RepositoryID: repo.ID, PullRequestID: p.ID, PlanID: plan.ID, PlanVersion: plan.Version, AreaID: area.ID, PrincipalType: in.PrincipalType, PrincipalID: in.PrincipalID, AgentGrantID: in.AgentGrantID, Deadline: in.Deadline, EscalationPath: strings.TrimSpace(in.EscalationPath), AssignedBy: actor.UserID}
		persist := func() error { var x error; value, x = plans.CreateAssignment(value); return x }
		if in.PrincipalType == "human" {
			e = catalog.WithCurrentParticipant(in.PrincipalID, repo.ID, persist)
		} else if orgs != nil && repo.OrganizationID != "" {
			e = orgs.WithCurrentAgentGrant(repo.OrganizationID, in.AgentGrantID, in.PrincipalID, repo.ID, persist)
		} else {
			e = reviewplans.ErrInvalid
		}
		if e != nil {
			writeAPIError(w, 409, "review_assignment_changed", "review eligibility or request identity changed")
			return
		}
		writeJSON(w, 201, value)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/review-assignments/{assignment_id}/transitions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateReviewMutation(w, r, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		var in reviewAssignmentTransition
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "transition input is invalid")
			return
		}
		repo, e := catalog.GetByID(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		values, e := plans.ListAssignments(repo.ID, r.PathValue("pull_id"))
		if e != nil {
			writeAPIError(w, 500, "review_assignments_unavailable", "review assignments could not be read")
			return
		}
		i := slices.IndexFunc(values, func(v reviewplans.Assignment) bool { return v.ID == r.PathValue("assignment_id") })
		if i < 0 {
			writeAPIError(w, 404, "review_assignment_not_found", "review assignment not found")
			return
		}
		current := values[i]
		actorID := actor.UserID
		if actor.AgentID != "" {
			actorID = actor.AgentID
		}
		maintainer := actor.AgentID == "" && actor.UserID == repo.OwnerID
		assignee := actorID == current.PrincipalID
		if (in.Action == "release" || in.Action == "replace") && !maintainer || (in.Action != "release" && in.Action != "replace") && !assignee {
			writeAPIError(w, 403, "review_assignment_forbidden", "only the assignee may respond and only the maintainer may release or replace")
			return
		}
		var replacement *reviewplans.Assignment
		if in.Action == "replace" {
			replacement = &reviewplans.Assignment{RequestID: in.RequestID, PrincipalType: in.PrincipalType, PrincipalID: in.PrincipalID, AgentGrantID: in.AgentGrantID, Deadline: in.Deadline, EscalationPath: strings.TrimSpace(in.EscalationPath)}
			p, pullErr := pulls.Get(repo.ID, r.PathValue("pull_id"))
			history, planErr := plans.List(repo.ID, r.PathValue("pull_id"), p.SourceCommitID, p.TargetCommitID)
			if pullErr != nil || planErr != nil || len(history) == 0 || history[len(history)-1].ID != current.PlanID {
				writeAPIError(w, 409, "review_plan_changed", "the exact review plan changed; reassess reviewer eligibility")
				return
			}
			repositoryAssignments, capacityErr := plans.ListRepositoryAssignments(repo.ID)
			if capacityErr != nil {
				writeAPIError(w, 503, "review_capacity_unavailable", "reviewer capacity could not be resolved")
				return
			}
			suggestion, found := findSuggestion(deriveReviewerSuggestions(repo, p, history[len(history)-1], repositoryAssignments, pulls, orgs), in.PrincipalType, in.PrincipalID, in.AgentGrantID, current.AreaID)
			if !found || !suggestion.Eligible {
				writeAPIError(w, 422, "reviewer_ineligible", "replacement is unavailable, conflicted, overloaded, revoked, or lacks permitted evidence")
				return
			}
		}
		var out reviewplans.Assignment
		persist := func() error {
			var transitionErr error
			out, transitionErr = plans.Transition(repo.ID, r.PathValue("pull_id"), current.ID, actorID, in.Action, in.Reason, replacement)
			return transitionErr
		}
		if replacement == nil && assignee && current.PrincipalType == "human" {
			e = catalog.WithCurrentParticipant(current.PrincipalID, repo.ID, persist)
		} else if replacement == nil && assignee && current.PrincipalType == "agent" && orgs != nil && repo.OrganizationID != "" {
			if in.Action == "accept" {
				e = orgs.WithCurrentReviewAgentGrant(repo.OrganizationID, current.AgentGrantID, current.PrincipalID, repo.ID, persist)
			} else {
				e = orgs.WithCurrentAgentGrant(repo.OrganizationID, current.AgentGrantID, current.PrincipalID, repo.ID, persist)
			}
		} else if replacement != nil && replacement.PrincipalType == "human" {
			e = catalog.WithCurrentParticipant(replacement.PrincipalID, repo.ID, persist)
		} else if replacement != nil && orgs != nil && repo.OrganizationID != "" {
			e = orgs.WithCurrentAgentGrant(repo.OrganizationID, replacement.AgentGrantID, replacement.PrincipalID, repo.ID, persist)
		} else {
			e = persist()
		}
		if e != nil {
			code := 422
			if errors.Is(e, reviewplans.ErrConflict) {
				code = 409
			}
			writeAPIError(w, code, "review_assignment_transition_invalid", "review assignment transition is invalid or stale")
			return
		}
		writeJSON(w, 200, out)
	})
}

func currentReviewArea(w http.ResponseWriter, plans *reviewplans.Store, p pullrequests.PullRequest, planID, areaID string) (reviewplans.Plan, reviewplans.Area, bool) {
	history, e := plans.List(p.RepositoryID, p.ID, p.SourceCommitID, p.TargetCommitID)
	if e != nil || len(history) == 0 {
		writeAPIError(w, 409, "review_plan_required", "a current review plan is required")
		return reviewplans.Plan{}, reviewplans.Area{}, false
	}
	plan := history[len(history)-1]
	if plan.Stale || plan.ID != planID {
		writeAPIError(w, 409, "review_plan_changed", "the exact current review plan is required")
		return reviewplans.Plan{}, reviewplans.Area{}, false
	}
	i := slices.IndexFunc(plan.Areas, func(a reviewplans.Area) bool { return a.ID == areaID })
	if i < 0 {
		writeAPIError(w, 422, "review_area_invalid", "review area is invalid")
		return reviewplans.Plan{}, reviewplans.Area{}, false
	}
	return plan, plan.Areas[i], true
}

func deriveReviewerSuggestions(repo repositories.Repository, p pullrequests.PullRequest, plan reviewplans.Plan, assignments []reviewplans.Assignment, pulls *pullrequests.Store, orgs *organizations.Store) []reviewplans.ReviewerSuggestion {
	load := map[string]int{}
	for _, a := range assignments {
		if a.Status == "invited" || a.Status == "accepted" {
			load[a.PrincipalID]++
		}
	}
	areas := make([]string, len(plan.Areas))
	for i, a := range plan.Areas {
		areas[i] = a.ID
	}
	out := []reviewplans.ReviewerSuggestion{}
	for _, id := range repo.ParticipantIDs() {
		s := reviewplans.ReviewerSuggestion{PrincipalType: "human", PrincipalID: id, AreaIDs: areas, Eligible: true, Availability: "available", ActiveLoad: load[id], Evidence: []reviewplans.MatchEvidence{{Kind: "repository_participation", Summary: "Current repository participation permits access to the review context."}}}
		if id == repo.OwnerID {
			s.Evidence = append(s.Evidence, reviewplans.MatchEvidence{Kind: "code_ownership", Summary: "Current repository ownership makes this participant accountable for changed code."})
		}
		if id == p.AuthorID {
			s.Eligible = false
			s.Conflict = "The pull author cannot independently review their own change."
		}
		if s.ActiveLoad >= 3 {
			s.Eligible = false
			s.Availability = "overloaded"
		}
		if id == repo.OwnerID {
			s.Eligible = true
			s.Availability = "available"
		}
		reviews, _ := pulls.ListReviews(repo.ID, p.ID)
		if slices.ContainsFunc(reviews, func(r pullrequests.Review) bool { return r.ReviewerID == id }) {
			s.Evidence = append(s.Evidence, reviewplans.MatchEvidence{Kind: "demonstrated_knowledge", Summary: "An attributable review exists on this project pull."})
		}
		out = append(out, s)
	}
	if orgs != nil && repo.OrganizationID != "" {
		if org, e := orgs.Get(repo.OrganizationID); e == nil {
			for _, agent := range org.Agents {
				for _, grant := range org.AccessGrants {
					if grant.PrincipalType != "agent" || grant.PrincipalID != agent.ID || grant.RevokedAt != nil || !slices.Contains(grant.Resources, organizations.ResourceScope{Kind: "repository", ID: repo.ID}) {
						continue
					}
					s := reviewplans.ReviewerSuggestion{PrincipalType: "agent", PrincipalID: agent.ID, AgentGrantID: grant.ID, AreaIDs: areas, Eligible: false, Availability: "unknown", ActiveLoad: load[agent.ID], Evidence: []reviewplans.MatchEvidence{{Kind: "approved_agent_grant", Summary: "A current organization grant covers this exact repository."}}}
					if len(agent.Profiles) > 0 {
						profile := agent.Profiles[len(agent.Profiles)-1]
						s.Availability = profile.Availability
						for _, capability := range append(agent.Capabilities, profile.SupportedTasks...) {
							if strings.Contains(strings.ToLower(capability), "review") {
								s.Eligible = true
								s.Evidence = append(s.Evidence, reviewplans.MatchEvidence{Kind: "approved_capability", Summary: "The current approved profile declares review capability."})
								break
							}
						}
						if len(profile.ConflictDisclosures) > 0 {
							s.Eligible = false
							s.Conflict = "The agent profile contains a conflict disclosure requiring human resolution."
						}
					} else {
						s.MissingEvidence = []string{"No current approved agent profile is available."}
					}
					if strings.Contains(strings.ToLower(s.Availability), "unavailable") || s.ActiveLoad >= 3 {
						s.Eligible = false
						if s.ActiveLoad >= 3 {
							s.Availability = "overloaded"
						}
					}
					out = append(out, s)
				}
			}
		}
	}
	return out
}
func findSuggestion(values []reviewplans.ReviewerSuggestion, kind, id, grant, area string) (reviewplans.ReviewerSuggestion, bool) {
	for _, v := range values {
		if v.PrincipalType == kind && v.PrincipalID == id && v.AgentGrantID == grant && slices.Contains(v.AreaIDs, area) {
			return v, true
		}
	}
	return reviewplans.ReviewerSuggestion{}, false
}
func projectReviewAssignments(values []reviewplans.Assignment, repo repositories.Repository, orgs *organizations.Store) []reviewplans.Assignment {
	for i := range values {
		if values[i].Status != "invited" && values[i].Status != "accepted" {
			continue
		}
		valid := false
		if values[i].PrincipalType == "human" {
			valid = repo.HasParticipant(values[i].PrincipalID)
		} else if orgs != nil && repo.OrganizationID != "" {
			if org, e := orgs.Get(repo.OrganizationID); e == nil {
				valid = slices.ContainsFunc(org.AccessGrants, func(g organizations.AccessGrant) bool {
					return g.ID == values[i].AgentGrantID && g.PrincipalID == values[i].PrincipalID && g.RevokedAt == nil && slices.Contains(g.Resources, organizations.ResourceScope{Kind: "repository", ID: repo.ID})
				})
			}
		}
		if !valid {
			values[i].Status = "revoked"
			values[i].ActionRequired = "Assign another eligible reviewer to this area."
		}
	}
	return values
}
