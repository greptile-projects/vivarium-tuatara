package main

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/reviewplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
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

type findingResolutionInput struct {
	RequestID      string                       `json:"request_id"`
	FindingID      string                       `json:"finding_id"`
	Action         string                       `json:"action"`
	Classification string                       `json:"classification"`
	Rationale      string                       `json:"rationale"`
	Dissent        string                       `json:"dissent"`
	SupersedesID   string                       `json:"supersedes_id"`
	DuplicateOfID  string                       `json:"duplicate_of_id"`
	ExpiresAt      string                       `json:"expires_at"`
	Links          []reviewplans.ResolutionLink `json:"links"`
	Evidence       []reviewplans.WorkCitation   `json:"evidence"`
}

func registerReviewWorkRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, plans *reviewplans.Store, orgs *organizations.Store, checks *checkruns.Store, previewStore *previews.Store, decisionStore *decisions.Store, proposalStore *proposals.Store, sessions *changesessions.Store, workspaceStore *workspaces.Store) {
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
		resolutions, err := plans.ListFindingResolutions(repo.ID, pull.ID)
		if err != nil {
			writeAPIError(w, 500, "review_resolution_unavailable", "finding decisions could not be read")
			return
		}
		resolutionProjection := projectFindingResolutions(entries, resolutions, pull.SourceCommitID, checks, repo.ID, pull.ID)
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
		writeJSON(w, 200, map[string]any{"plan_id": plan.ID, "plan_version": plan.Version, "source_revision": plan.SourceRevision, "target_revision": plan.TargetRevision, "stale": plan.Stale, "queues": queues, "finding_resolutions": resolutionProjection, "viewer_id": actor.UserID, "authority": "Shared review work and finding decisions preserve exact reasoning but grant no branch, agent, approval, merge, exception, evidence, disclosure, or operational authority."})
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
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/review-findings/resolutions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", true)
		if !ok {
			return
		}
		var in findingResolutionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "finding resolution input is invalid")
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
		work, err := plans.ListWork(repo.ID, pull.ID)
		if err != nil {
			writeAPIError(w, 500, "review_work_unavailable", "review findings could not be read")
			return
		}
		index := slices.IndexFunc(work, func(x reviewplans.WorkEntry) bool { return x.ID == in.FindingID && x.Kind == "finding" })
		if index < 0 {
			writeAPIError(w, 422, "review_finding_invalid", "finding must belong to this pull review ledger")
			return
		}
		finding := work[index]
		if !validResolutionLinks(git, repo.ID, pull, in.Links, proposalStore, sessions, workspaceStore) {
			writeAPIError(w, 422, "review_resolution_link_invalid", "resolution links must resolve in this repository and ordinary work boundary")
			return
		}
		history, e := plans.List(repo.ID, pull.ID, "", "")
		if e != nil {
			writeAPIError(w, 500, "review_plan_unavailable", "review plan history could not be read")
			return
		}
		planIndex := slices.IndexFunc(history, func(x reviewplans.Plan) bool { return x.ID == finding.PlanID })
		if planIndex < 0 {
			writeAPIError(w, 422, "review_finding_invalid", "finding plan is unavailable")
			return
		}
		areaPlan := history[planIndex]
		areaIndex := slices.IndexFunc(areaPlan.Areas, func(x reviewplans.Area) bool { return x.ID == finding.AreaID })
		if areaIndex < 0 {
			writeAPIError(w, 422, "review_finding_invalid", "finding area is unavailable")
			return
		}
		area := areaPlan.Areas[areaIndex]
		if len(in.Evidence) > 0 && !validWorkCitations(repo.ID, pull.ID, pull.SourceCommitID, area, areaPlan, in.Evidence, checks, previewStore, decisionStore) {
			writeAPIError(w, 422, "review_verification_invalid", "verification evidence must resolve at the current candidate revision")
			return
		}
		actorID, actorType := actor.UserID, "human"
		if actor.AgentID != "" {
			actorID, actorType = actor.AgentID, "agent"
		}
		ownerAction := slices.Contains([]string{"accept", "supersede", "resolved", "accepted_risk", "exception"}, in.Action)
		if ownerAction && (actorType != "human" || (actorID != pull.AuthorID && actorID != repo.OwnerID)) {
			writeAPIError(w, 403, "review_owner_decision_required", "only the pull author or current repository owner can publish this decision")
			return
		}
		var agentAssignment reviewplans.Assignment
		if actorType == "agent" {
			assignments, e := plans.ListAssignments(repo.ID, pull.ID)
			if e != nil {
				writeAPIError(w, 500, "review_assignment_unavailable", "agent review authority could not be read")
				return
			}
			i := slices.IndexFunc(assignments, func(x reviewplans.Assignment) bool {
				return x.PlanID == finding.PlanID && x.AreaID == finding.AreaID && x.PrincipalType == "agent" && x.PrincipalID == actorID && x.Status == "accepted"
			})
			if i < 0 {
				writeAPIError(w, 403, "review_resolution_forbidden", "agents may discuss only findings in their accepted exact review area")
				return
			}
			agentAssignment = assignments[i]
		}
		var expiry *time.Time
		if in.ExpiresAt != "" {
			v, e := time.Parse(time.RFC3339, in.ExpiresAt)
			if e != nil {
				writeAPIError(w, 422, "review_exception_invalid", "exception expiry must be RFC3339 and within 30 days")
				return
			}
			expiry = &v
		}
		if in.Action == "exception" && !slices.ContainsFunc(in.Links, func(link reviewplans.ResolutionLink) bool { return link.Kind == "follow_up" }) {
			writeAPIError(w, 422, "review_exception_invalid", "an emergency review exception must link to ordinary follow-up work")
			return
		}
		existing, _ := plans.ListFindingResolutions(repo.ID, pull.ID)
		if in.SupersedesID != "" && !slices.ContainsFunc(existing, func(x reviewplans.FindingResolution) bool {
			return x.ID == in.SupersedesID && x.FindingID == finding.ID
		}) {
			writeAPIError(w, 422, "review_supersession_invalid", "superseded decision must belong to this finding")
			return
		}
		if in.DuplicateOfID != "" && !slices.ContainsFunc(work, func(x reviewplans.WorkEntry) bool { return x.ID == in.DuplicateOfID && x.Kind == "finding" }) {
			writeAPIError(w, 422, "review_duplicate_invalid", "duplicate target must be another retained finding")
			return
		}
		value := reviewplans.FindingResolution{RequestID: in.RequestID, RepositoryID: repo.ID, PullRequestID: pull.ID, FindingID: finding.ID, FindingRevision: finding.SourceRevision, CandidateRevision: pull.SourceCommitID, ActorType: actorType, ActorID: actorID, Action: in.Action, Classification: in.Classification, Rationale: in.Rationale, Dissent: in.Dissent, SupersedesID: in.SupersedesID, DuplicateOfID: in.DuplicateOfID, Links: in.Links, Evidence: in.Evidence, ExpiresAt: expiry}
		persist := func() error {
			return pulls.WithSourceRevision(repo.ID, pull.ID, pull.SourceCommitID, func(current pullrequests.PullRequest) error {
				value.CandidateRevision = current.SourceCommitID
				value, err = plans.CreateFindingResolution(value)
				return err
			})
		}
		if actorType == "human" {
			err = catalog.WithCurrentParticipant(actorID, repo.ID, persist)
		} else {
			if orgs == nil || repo.OrganizationID == "" {
				err = reviewplans.ErrInvalid
			} else {
				err = orgs.WithCurrentReviewAgentGrant(repo.OrganizationID, agentAssignment.AgentGrantID, actorID, repo.ID, persist)
			}
		}
		if err != nil {
			if errors.Is(err, pullrequests.ErrSourceChanged) || errors.Is(err, pullrequests.ErrNotReady) {
				writeAPIError(w, 409, "review_resolution_stale", "the pull source changed while the finding decision was published")
				return
			}
			code := 422
			if errors.Is(err, reviewplans.ErrResolutionConflict) {
				code = 409
			}
			writeAPIError(w, code, "review_resolution_changed", "resolution identity, authority, or content changed")
			return
		}
		writeJSON(w, 201, value)
	})
}

func projectFindingResolutions(work []reviewplans.WorkEntry, values []reviewplans.FindingResolution, current string, checks *checkruns.Store, repo, pull string) []map[string]any {
	out := []map[string]any{}
	for _, finding := range work {
		if finding.Kind != "finding" {
			continue
		}
		events := []reviewplans.FindingResolution{}
		for _, v := range values {
			if v.FindingID == finding.ID {
				events = append(events, v)
			}
		}
		state := "applicable"
		if finding.SourceRevision != current {
			state = "stale"
		}
		verified := false
		for _, v := range events {
			if v.CandidateRevision == current && slices.Contains([]string{"accept", "defer", "supersede", "resolved", "accepted_risk", "exception", "remains_applicable"}, v.Action) {
				if v.Action == "exception" && (v.ExpiresAt == nil || !v.ExpiresAt.After(time.Now().UTC())) {
					continue
				}
				state = v.Action
				if v.Action == "resolved" {
					verified = len(v.Evidence) > 0
					for _, e := range v.Evidence {
						if e.Kind != "check" || checks == nil {
							verified = false
							continue
						}
						run, err := checks.Get(repo, pull, e.Value)
						verified = verified && err == nil && run.CommitID == current && run.State == "succeeded"
					}
				}
			}
		}
		out = append(out, map[string]any{"finding": finding, "events": events, "current_state": state, "verified": verified})
	}
	return out
}

func validResolutionLinks(git *storage.Store, repoID string, pull pullrequests.PullRequest, links []reviewplans.ResolutionLink, proposalStore *proposals.Store, sessions *changesessions.Store, workspaceStore *workspaces.Store) bool {
	for _, l := range links {
		switch l.Kind {
		case "commit":
			r, e := git.Open(repoID)
			if e != nil {
				return false
			}
			if _, e = r.ReadCommit(storage.ObjectID(l.ResourceID)); e != nil {
				return false
			}
			if l.Revision != "" && l.Revision != l.ResourceID {
				return false
			}
		case "task":
			if proposalStore == nil || l.ContainerID == "" {
				return false
			}
			if _, e := proposalStore.GetTask(repoID, l.ContainerID, l.ResourceID); e != nil {
				return false
			}
		case "change_session":
			if sessions == nil {
				return false
			}
			s, e := sessions.Get(repoID, pull.ID, l.ResourceID)
			if e != nil || s.RepositoryID != repoID {
				return false
			}
		case "workspace":
			if workspaceStore == nil {
				return false
			}
			x, e := workspaceStore.Get(l.ResourceID)
			if e != nil || x.RepositoryID != repoID {
				return false
			}
		case "follow_up":
			if proposalStore == nil || l.ContainerID == "" {
				return false
			}
			if _, e := proposalStore.GetTask(repoID, l.ContainerID, l.ResourceID); e != nil {
				return false
			}
		default:
			return false
		}
	}
	return true
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
