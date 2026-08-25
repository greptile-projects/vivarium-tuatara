package main

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/reviewplans"
)

type reviewWorkInput struct {
	RequestID     string                     `json:"request_id"`
	PlanID        string                     `json:"plan_id"`
	AreaID        string                     `json:"area_id"`
	Kind          string                     `json:"kind"`
	Conclusion    string                     `json:"conclusion"`
	Body          string                     `json:"body"`
	Uncertainty   string                     `json:"uncertainty"`
	Citations     []reviewplans.WorkCitation `json:"citations"`
	RecipientType string                     `json:"recipient_type"`
	RecipientID   string                     `json:"recipient_id"`
}

func registerReviewWorkRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, plans *reviewplans.Store, orgs *organizations.Store, checks *checkruns.Store, previewStore *previews.Store, decisionStore *decisions.Store) {
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/review-work", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		repo, err := catalog.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		pull, err := pulls.Get(repo.ID, r.PathValue("pull_id"))
		if err != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		history, err := plans.List(repo.ID, pull.ID, pull.SourceCommitID, pull.TargetCommitID)
		if err != nil || len(history) == 0 {
			writeAPIError(w, 409, "review_plan_required", "a review plan is required")
			return
		}
		assignments, err := plans.ListAssignments(repo.ID, pull.ID)
		if err != nil {
			writeAPIError(w, 500, "review_work_unavailable", "review work could not be read")
			return
		}
		entries, err := plans.ListWork(repo.ID, pull.ID)
		if err != nil {
			writeAPIError(w, 500, "review_work_unavailable", "review work could not be read")
			return
		}
		plan := history[len(history)-1]
		projected := projectReviewAssignments(assignments, repo, orgs)
		queues := make([]map[string]any, 0, len(plan.Areas))
		for _, area := range plan.Areas {
			areaEntries := []reviewplans.WorkEntry{}
			findings, uncertainty := 0, 0
			conclusions := []string{}
			for _, entry := range entries {
				if entry.PlanID == plan.ID && entry.AreaID == area.ID {
					areaEntries = append(areaEntries, entry)
					if entry.Kind == "finding" {
						findings++
						if entry.Conclusion != "" && !slices.Contains(conclusions, entry.Conclusion) {
							conclusions = append(conclusions, entry.Conclusion)
						}
					}
					if entry.Kind == "uncertainty" || entry.Uncertainty != "" {
						uncertainty++
					}
				}
			}
			areaAssignments := []reviewplans.Assignment{}
			for _, a := range projected {
				if a.PlanID == plan.ID && a.AreaID == area.ID {
					areaAssignments = append(areaAssignments, a)
				}
			}
			queues = append(queues, map[string]any{"area": area, "assignments": areaAssignments, "entries": areaEntries, "coverage": map[string]any{"entry_count": len(areaEntries), "finding_count": findings, "uncertainty_count": uncertainty, "conflicting_conclusions": len(conclusions) > 1}, "dependencies": area.DependsOn})
		}
		writeJSON(w, 200, map[string]any{"plan_id": plan.ID, "plan_version": plan.Version, "source_revision": plan.SourceRevision, "target_revision": plan.TargetRevision, "stale": plan.Stale, "queues": queues, "viewer_id": actor.UserID, "authority": "Shared review work coordinates exact-area investigation and never grants approval, merge, evidence, disclosure, or operational authority."})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/review-work", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", true)
		if !ok {
			return
		}
		var in reviewWorkInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "review work input is invalid")
			return
		}
		repo, err := catalog.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		pull, err := pulls.Get(repo.ID, r.PathValue("pull_id"))
		if err != nil || pull.Status != pullrequests.Open {
			writeAPIError(w, 404, "pull_request_not_found", "open pull request not found")
			return
		}
		plan, area, current := currentReviewArea(w, plans, pull, in.PlanID, in.AreaID)
		if !current {
			return
		}
		actorID, actorType := actor.UserID, "human"
		if actor.AgentID != "" {
			actorID, actorType = actor.AgentID, "agent"
		}
		assignments, err := plans.ListAssignments(repo.ID, pull.ID)
		if err != nil {
			writeAPIError(w, 500, "review_work_unavailable", "review responsibility could not be read")
			return
		}
		assignmentIndex := slices.IndexFunc(assignments, func(a reviewplans.Assignment) bool {
			return a.PlanID == plan.ID && a.AreaID == area.ID && a.PrincipalID == actorID && a.PrincipalType == actorType && a.Status == "accepted"
		})
		if assignmentIndex < 0 {
			writeAPIError(w, 403, "review_work_forbidden", "only the accepted reviewer for this exact area can publish review work")
			return
		}
		if !validWorkCitations(repo.ID, pull.ID, plan.SourceRevision, area, plan, in.Citations, checks, previewStore, decisionStore) {
			writeAPIError(w, 422, "review_citation_out_of_scope", "citations must belong to the assigned exact review area and public pull surface")
			return
		}
		if in.RecipientID != "" && in.RecipientType == "human" && !repo.HasParticipant(in.RecipientID) {
			writeAPIError(w, 422, "review_recipient_invalid", "handoff recipient is not a current repository participant")
			return
		}
		value := reviewplans.WorkEntry{RequestID: in.RequestID, RepositoryID: repo.ID, PullRequestID: pull.ID, PlanID: plan.ID, PlanVersion: plan.Version, AreaID: area.ID, SourceRevision: plan.SourceRevision, TargetRevision: plan.TargetRevision, ActorType: actorType, ActorID: actorID, Kind: in.Kind, Conclusion: in.Conclusion, Body: in.Body, Uncertainty: in.Uncertainty, Citations: in.Citations, RecipientType: in.RecipientType, RecipientID: in.RecipientID}
		assignment := assignments[assignmentIndex]
		persist := func() error {
			var createErr error
			value, createErr = plans.CreateAssignedWork(value, assignment.ID)
			return createErr
		}
		if actorType == "human" {
			err = catalog.WithCurrentParticipant(actorID, repo.ID, persist)
		} else if orgs != nil && repo.OrganizationID != "" {
			err = orgs.WithCurrentReviewAgentGrant(repo.OrganizationID, assignment.AgentGrantID, actorID, repo.ID, persist)
		} else {
			err = reviewplans.ErrInvalid
		}
		if err != nil {
			code := 422
			if errors.Is(err, reviewplans.ErrWorkConflict) {
				code = 409
			}
			writeAPIError(w, code, "review_work_changed", "review work identity, scope, or live reviewer authority changed")
			return
		}
		writeJSON(w, 201, value)
	})
}

func validWorkCitations(repoID, pullID, revision string, area reviewplans.Area, plan reviewplans.Plan, citations []reviewplans.WorkCitation, checks *checkruns.Store, previewStore *previews.Store, decisionStore *decisions.Store) bool {
	for _, citation := range citations {
		value := strings.TrimSpace(citation.Value)
		switch citation.Kind {
		case "file", "diff":
			if !slices.Contains(area.Paths, value) {
				return false
			}
		case "symbol":
			path, _, found := strings.Cut(value, "#")
			if !found || !slices.Contains(area.Paths, path) {
				return false
			}
		case "requirement":
			valid := slices.Contains(area.Questions, value) || slices.Contains(plan.PolicyRequirements, value) || slices.ContainsFunc(area.Evidence, func(e reviewplans.Evidence) bool { return e.Description == value })
			if !valid {
				return false
			}
		case "check":
			if checks == nil {
				return false
			}
			run, err := checks.Get(repoID, pullID, value)
			if err != nil || run.CommitID != revision {
				return false
			}
		case "preview":
			if previewStore == nil {
				return false
			}
			preview, err := previewStore.Get(repoID, pullID, value)
			if err != nil || preview.Revision != revision {
				return false
			}
		case "decision":
			if decisionStore == nil {
				return false
			}
			decision, err := decisionStore.Get(value)
			if err != nil || decision.RepositoryID != repoID || !decisionReferencesPull(decision, pullID) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func decisionReferencesPull(decision decisions.Decision, pullID string) bool {
	if slices.Contains([]string{"pull", "pull_request"}, decision.Source.Kind) && decision.Source.ResourceID == pullID {
		return true
	}
	return slices.ContainsFunc(decision.Scope.AffectedResources, func(resource decisions.Resource) bool {
		return slices.Contains([]string{"pull", "pull_request"}, resource.Kind) && resource.ResourceID == pullID
	})
}
