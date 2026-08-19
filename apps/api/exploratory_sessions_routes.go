package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/exploratorysessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/qualityplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/testscenarios"
)

func registerExploratorySessionRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, sessions *exploratorysessions.Store, pulls *pullrequests.Store, releaseStore *releases.Store, issueStore *issues.Store, plans *qualityplans.Store, proposalStore *proposals.Store, scenarios *testscenarios.Store) {
	project := func(repo, actor string, v exploratorysessions.Session) (exploratorysessions.Session, bool) {
		if !slices.Contains(v.Access, actor) {
			return exploratorysessions.Session{}, false
		}
		v.Stale, v.StaleReason = exploratorySessionStale(repo, v, pulls, releaseStore, issueStore, plans)
		return v, true
	}
	mux.HandleFunc("GET /repositories/{id}/exploratory-sessions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, e := sessions.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "exploratory_sessions_unavailable", "exploratory sessions could not be read")
			return
		}
		out := []exploratorysessions.Session{}
		for _, v := range values {
			if x, visible := project(r.PathValue("id"), actor.UserID, v); visible {
				out = append(out, x)
			}
		}
		writeJSON(w, 200, map[string]any{"sessions": out})
	})
	mux.HandleFunc("GET /repositories/{id}/exploratory-sessions/{session_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := sessions.Get(r.PathValue("session_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "exploratory_session_not_found", "exploratory session not found")
			return
		}
		out, visible := project(v.RepositoryID, actor.UserID, v)
		if !visible {
			writeAPIError(w, 404, "exploratory_session_not_found", "exploratory session not found")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/exploratory-sessions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_control_required", "a human participant must bound an exploratory session")
			return
		}
		var in exploratorysessions.Session
		if decodeJSON(r, &in) != nil || !exploratorysessions.ValidSession(in, time.Now().UTC()) {
			writeAPIError(w, 400, "invalid_exploratory_session", "an exact source, explicit audience, bounded data/budget/actions, and risk charters are required")
			return
		}
		if !slices.Contains(in.Access, actor.UserID) || !explorationParticipants(catalog, r.PathValue("id"), in) {
			writeAPIError(w, 422, "exploratory_access_invalid", "session access and human assignees must be current repository participants")
			return
		}
		encoded, _ := json.Marshal(in)
		if reusableSecret.Match(encoded) {
			writeAPIError(w, 400, "exploratory_session_sensitive", "session metadata cannot retain credentials or secret-shaped content")
			return
		}
		if !exploratorySourceResolves(git, r.PathValue("id"), in.Source, pulls, releaseStore, issueStore, plans) {
			writeAPIError(w, 422, "exploratory_source_invalid", "the source must resolve at the exact declared repository revision")
			return
		}
		out, e := sessions.Create(r.PathValue("id"), actor.UserID, in)
		writeExploratorySession(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/exploratory-sessions/{session_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authorizeExploratoryEvent(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		current, e := sessions.Get(r.PathValue("session_id"))
		if e != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "exploratory_session_not_found", "exploratory session not found")
			return
		}
		var in exploratorysessions.EventInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_exploratory_event", "a bounded timeline event is required")
			return
		}
		encoded, _ := json.Marshal(in)
		if reusableSecret.Match(encoded) {
			writeAPIError(w, 400, "exploratory_event_sensitive", "timeline metadata cannot retain credentials or secret-shaped content")
			return
		}
		actorID := actor.UserID
		if actor.AgentID != "" {
			actorID = actor.AgentID
			in.ActorType = "agent"
			in.ActorID = actor.AgentID
		} else {
			if !slices.Contains(current.Access, actor.UserID) {
				writeAPIError(w, 404, "exploratory_session_not_found", "exploratory session not found")
				return
			}
			in.ActorType = "human"
			in.ActorID = ""
		}
		out, e := sessions.Append(current.ID, actorID, in)
		if e == nil {
			out.Stale, out.StaleReason = exploratorySessionStale(current.RepositoryID, out, pulls, releaseStore, issueStore, plans)
		}
		writeExploratorySession(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/exploratory-sessions/{session_id}/findings/{finding_id}/repair", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "exploratory_repair_human_required", "a human collaborator must authorize ordinary repair work")
			return
		}
		if issueStore == nil || proposalStore == nil {
			writeAPIError(w, 503, "exploratory_repair_unavailable", "governed issue and task publication is unavailable")
			return
		}
		var in struct {
			ExpectedVersion     int      `json:"expected_version"`
			Title               string   `json:"title"`
			ExpectedBehavior    string   `json:"expected_behavior"`
			Severity            string   `json:"severity"`
			Environment         string   `json:"environment"`
			EvidenceEventIDs    []string `json:"evidence_event_ids"`
			ReproductionEventID string   `json:"reproduction_event_id"`
			AcceptanceCriteria  []string `json:"acceptance_criteria"`
			AssigneeType        string   `json:"assignee_type"`
			AssigneeID          string   `json:"assignee_id"`
			QualityPlanID       string   `json:"quality_plan_id"`
			QualityPlanVersion  int      `json:"quality_plan_version"`
			RequirementIDs      []string `json:"requirement_ids"`
		}
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.ExpectedBehavior) == "" || strings.TrimSpace(in.Environment) == "" {
			writeAPIError(w, 422, "exploratory_repair_invalid", "a confirmed reproduction, bounded evidence, acceptance criteria, issue context, and owner are required")
			return
		}
		encoded, _ := json.Marshal(in)
		if reusableSecret.Match(encoded) || len(in.Title) > 4096 || len(in.ExpectedBehavior) > 4096 || len(in.Environment) > 4096 || !slices.Contains([]string{"low", "medium", "high", "critical"}, in.Severity) || !slices.Contains([]string{"human", "agent"}, in.AssigneeType) {
			writeAPIError(w, 422, "exploratory_repair_invalid", "repair metadata must be bounded, non-sensitive, and use a supported severity and owner type")
			return
		}
		current, err := sessions.Get(r.PathValue("session_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") || !slices.Contains(current.Access, actor.UserID) {
			writeAPIError(w, 404, "exploratory_session_not_found", "exploratory session not found")
			return
		}
		if current.Stale || func() bool {
			stale, _ := exploratorySessionStale(current.RepositoryID, current, pulls, releaseStore, issueStore, plans)
			return stale
		}() {
			writeAPIError(w, 409, "exploratory_candidate_stale", "the exact candidate changed before repair was authorized")
			return
		}
		if in.AssigneeType == "human" {
			if strings.TrimSpace(in.AssigneeID) == "" {
				in.AssigneeID = actor.UserID
			}
			repo, _ := catalog.GetByID(current.RepositoryID)
			collaborator, _ := catalog.HasCollaborator(in.AssigneeID, current.RepositoryID)
			if repo.OwnerID != in.AssigneeID && !collaborator {
				writeAPIError(w, 422, "exploratory_repair_assignee_invalid", "a human repair owner must be a current repository participant")
				return
			}
		}
		if !exploratoryOpaqueID(in.AssigneeID) {
			writeAPIError(w, 422, "exploratory_repair_assignee_invalid", "the repair owner must have a stable platform identity")
			return
		}
		if in.QualityPlanID != "" {
			plan, planErr := plans.Get(in.QualityPlanID)
			if planErr != nil || plan.RepositoryID != current.RepositoryID || in.QualityPlanVersion < 1 || in.QualityPlanVersion > plan.CurrentVersion || !qualityRequirementsExist(plan, in.QualityPlanVersion, in.RequirementIDs) {
				writeAPIError(w, 422, "exploratory_quality_plan_invalid", "the frozen quality plan version and requirements must resolve in this repository")
				return
			}
		}
		request := exploratorysessions.RepairInput{ExpectedVersion: in.ExpectedVersion, FindingID: r.PathValue("finding_id"), EvidenceEventIDs: in.EvidenceEventIDs, ReproductionEventID: in.ReproductionEventID, AcceptanceCriteria: in.AcceptanceCriteria, AssigneeType: in.AssigneeType, AssigneeID: in.AssigneeID, QualityPlanID: in.QualityPlanID, QualityPlanVersion: in.QualityPlanVersion, RequirementIDs: in.RequirementIDs}
		reservedSession, repair, err := sessions.ReserveRepair(current.ID, actor.UserID, request)
		if errors.Is(err, exploratorysessions.ErrConflict) {
			writeAPIError(w, 409, "exploratory_repair_changed", "the finding or its frozen repair handoff changed")
			return
		}
		if errors.Is(err, exploratorysessions.ErrFindingNotConfirmed) {
			writeAPIError(w, 409, "exploratory_finding_not_confirmed", "only a current bug classification with an exact reproduction can enter repair")
			return
		}
		if err != nil {
			writeExploratorySession(w, exploratorysessions.Session{}, err, 0)
			return
		}
		events := map[string]exploratorysessions.Event{}
		steps := []string{}
		observed := []string{}
		items := []proposals.ReasoningItem{}
		for _, event := range reservedSession.Events {
			events[event.ID] = event
		}
		for _, id := range repair.EvidenceEventIDs {
			event := events[id]
			observed = append(observed, event.Summary)
			steps = append(steps, event.Summary)
			items = append(items, proposals.ReasoningItem{ID: event.ID, Kind: "exploratory_evidence", Summary: event.Summary, Status: "permitted"})
		}
		reproduction := events[repair.ReproductionID]
		items = append(items, proposals.ReasoningItem{ID: reproduction.ID, Kind: "minimized_reproduction", Summary: reproduction.Summary, Status: "confirmed"})
		issue, issueErr := issueStore.CreateEscalated(issues.Issue{RepositoryID: current.RepositoryID, ReporterID: actor.UserID, Title: strings.TrimSpace(in.Title), ExpectedBehavior: strings.TrimSpace(in.ExpectedBehavior), ObservedBehavior: strings.Join(observed, "\n"), Severity: in.Severity, Environment: strings.TrimSpace(in.Environment), ReproductionSteps: steps, Visibility: "repository", Triage: issues.Triage{Classification: "bug", Priority: in.Severity, AssigneeID: in.AssigneeID, SuspectedRevision: repair.AffectedRevision}, Links: []issues.Link{{ID: issues.NewEvidenceID(), Kind: "exploratory_finding", RepositoryID: current.RepositoryID, ResourceID: current.ID + ":" + repair.FindingID, Revision: repair.AffectedRevision, Label: "Exploratory finding " + repair.FindingID, AddedBy: actor.UserID, CreatedAt: time.Now().UTC()}}}, repair.RecoveryID)
		if issueErr != nil && !errors.Is(issueErr, issues.ErrDurabilityUncertain) {
			writeAPIError(w, 500, "exploratory_issue_failed", "the reserved finding could not be reconciled into an issue")
			return
		}
		criteria := strings.Join(repair.AcceptanceCriteria, "\n- ")
		origin := proposals.ReasoningOrigin{ExploratorySessionID: current.ID, ExploratoryFindingID: repair.FindingID, ExploratoryRepairID: repair.RecoveryID, IssueID: "", Revision: repair.AffectedRevision, SelectedItemIDs: append(append([]string{}, repair.EvidenceEventIDs...), repair.ReproductionID), Items: items, AnalysisStatus: "exploratory_repair"}
		proposal, tasks, proposalErr := proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: current.RepositoryID, ActorID: actor.UserID, Title: "Repair: " + strings.TrimSpace(in.Title), Body: "Governed repair for exploratory finding " + repair.FindingID + " linked to issue " + issue.ID + ".\n\nAcceptance criteria:\n- " + criteria, Origin: origin, Tasks: []proposals.ImplementationTaskInput{{Title: "Repair " + strings.TrimSpace(in.Title), Outcome: "The exact candidate failure no longer reproduces and durable regression coverage detects its return.", Risk: in.Severity + " exploratory defect at " + repair.AffectedRevision, VerificationPlan: "Demonstrate failure against the affected revision, pass on the repair revision, and publish the regression scenario.\n- " + criteria, AssigneeType: in.AssigneeType, AssigneeID: in.AssigneeID}}})
		if proposalErr != nil && !errors.Is(proposalErr, proposals.ErrDurabilityUncertain) {
			writeAPIError(w, 500, "exploratory_task_failed", "the issue was retained but repair work could not be reconciled")
			return
		}
		updated, finalizeErr := sessions.FinalizeRepair(current.ID, repair.RecoveryID, issue.ID, proposal.ID, tasks[0].ID)
		status := 201
		if finalizeErr != nil || errors.Is(issueErr, issues.ErrDurabilityUncertain) || errors.Is(proposalErr, proposals.ErrDurabilityUncertain) {
			status = 202
		}
		if finalizeErr != nil {
			updated = reservedSession
		} else {
			for _, linked := range updated.Repairs {
				if linked.RecoveryID == repair.RecoveryID {
					repair = linked
					break
				}
			}
		}
		writeJSON(w, status, map[string]any{"session": updated, "repair": repair, "issue": issue, "proposal": proposal, "task": tasks[0], "recovery_pending": finalizeErr != nil})
	})
	mux.HandleFunc("POST /repositories/{id}/exploratory-sessions/{session_id}/findings/{finding_id}/coverage", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "exploratory_coverage_human_required", "a human collaborator must publish lasting finding coverage")
			return
		}
		var in struct {
			ScenarioID      string `json:"scenario_id"`
			ExpectedVersion int    `json:"expected_version"`
		}
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.ScenarioID) == "" || scenarios == nil {
			writeAPIError(w, 422, "exploratory_coverage_invalid", "a published regression scenario is required")
			return
		}
		current, err := sessions.Get(r.PathValue("session_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") || current.Version != in.ExpectedVersion || !slices.Contains(current.Access, actor.UserID) {
			writeAPIError(w, 409, "exploratory_coverage_changed", "the session changed before coverage was linked")
			return
		}
		var repair *exploratorysessions.Repair
		for i := range current.Repairs {
			if current.Repairs[i].FindingID == r.PathValue("finding_id") {
				repair = &current.Repairs[i]
			}
		}
		if repair == nil || repair.State != "linked" {
			writeAPIError(w, 409, "exploratory_repair_incomplete", "the finding must first enter governed repair")
			return
		}
		scenario, err := scenarios.Get(in.ScenarioID)
		if err != nil || scenario.RepositoryID != current.RepositoryID || scenario.Implementation.PullRequestID == "" {
			writeAPIError(w, 422, "exploratory_coverage_invalid", "the scenario must resolve to the repair repository and an exact pull revision")
			return
		}
		issueSource := false
		for _, source := range scenario.Sources {
			issueSource = issueSource || source.Kind == "issue" && source.ResourceID == repair.IssueID && (source.Revision == repair.AffectedRevision || source.Revision == scenario.Implementation.CommitID)
		}
		if !issueSource || scenario.QualityPlanID != repair.QualityPlanID || scenario.QualityPlanVersion != repair.QualityPlanVersion || !exploratorySameStrings(scenario.RequirementIDs, repair.RequirementIDs) {
			writeAPIError(w, 422, "exploratory_coverage_invalid", "the scenario must cite the linked issue and frozen quality requirements")
			return
		}
		pull, err := pulls.Get(current.RepositoryID, scenario.Implementation.PullRequestID)
		if err != nil || pull.SourceCommitID != scenario.Implementation.CommitID || pull.TaskID == nil || *pull.TaskID != repair.TaskID {
			writeAPIError(w, 422, "exploratory_coverage_invalid", "the scenario must be implemented by the finding's exact repair pull")
			return
		}
		updated, err := sessions.LinkCoverage(current.ID, repair.FindingID, scenario.ID, pull.ID, scenario.Implementation.CommitID, in.ExpectedVersion)
		if errors.Is(err, exploratorysessions.ErrConflict) {
			writeAPIError(w, 409, "exploratory_coverage_changed", "different lasting coverage is already linked")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "exploratory_coverage_unavailable", "lasting coverage could not be linked")
			return
		}
		writeJSON(w, 201, map[string]any{"session": updated, "repair": func() exploratorysessions.Repair {
			for _, x := range updated.Repairs {
				if x.FindingID == repair.FindingID {
					return x
				}
			}
			return exploratorysessions.Repair{}
		}(), "scenario": scenario, "pull_request": pull})
	})
}

// Exploratory agents operate through an ordinary task credential. That
// credential is repository-bound and carries Git write scope, not general API
// mutation authority; the session store separately requires its authenticated
// agent identity to match an exact charter and allowed action.
func authorizeExploratoryEvent(w http.ResponseWriter, r *http.Request, catalog *repositories.Store, credentials *auth.Store, repositoryID string) (auth.Credential, bool) {
	actor, authenticated, err := authenticateOptionalCredential(r, credentials, "repositories:write")
	if errors.Is(err, auth.ErrNotFound) {
		actor, authenticated, err = authenticateOptionalCredential(r, credentials, "git:write")
		if err == nil && authenticated && (actor.AgentID == "" || actor.RepositoryID != repositoryID) {
			writeAPIError(w, 403, "exploratory_agent_scope_invalid", "agent exploration requires an exact repository-bound task credential")
			return auth.Credential{}, false
		}
	}
	if err != nil || !authenticated {
		writeAuthenticationRequired(w, false)
		return auth.Credential{}, false
	}
	repository, err := catalog.GetByID(repositoryID)
	if err != nil {
		writeRepositoryError(w, err)
		return auth.Credential{}, false
	}
	if actor.AgentID != "" && actor.RepositoryID == repositoryID {
		return actor, true
	}
	collaborator, err := catalog.HasCollaborator(actor.UserID, repositoryID)
	if err != nil {
		writeRepositoryError(w, err)
		return auth.Credential{}, false
	}
	if actor.UserID != repository.OwnerID && !collaborator {
		writeAPIError(w, 404, "repository_not_found", "repository not found")
		return auth.Credential{}, false
	}
	return actor, true
}

func exploratorySameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func exploratoryOpaqueID(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16
}

func qualityRequirementsExist(plan qualityplans.Plan, version int, ids []string) bool {
	for _, revision := range plan.Revisions {
		if revision.Version == version {
			found := map[string]bool{}
			for _, requirement := range revision.Requirements {
				found[requirement.ID] = true
			}
			for _, id := range ids {
				if !found[id] {
					return false
				}
			}
			return true
		}
	}
	return false
}

func explorationParticipants(catalog *repositories.Store, repo string, v exploratorysessions.Session) bool {
	valid := func(id string) bool {
		r, e := catalog.GetByID(repo)
		if e != nil {
			return false
		}
		if r.OwnerID == id {
			return true
		}
		ok, e := catalog.HasCollaborator(id, repo)
		return e == nil && ok
	}
	for _, id := range v.Access {
		if !valid(id) {
			return false
		}
	}
	for _, c := range v.Charters {
		if c.AssigneeType == "human" && !valid(c.AssigneeID) {
			return false
		}
	}
	return true
}
func exploratorySourceResolves(git *storage.Store, repo string, s exploratorysessions.Source, pulls *pullrequests.Store, releasesStore *releases.Store, issuesStore *issues.Store, plans *qualityplans.Store) bool {
	r, e := git.Open(repo)
	if e != nil {
		return false
	}
	if _, e = r.ReadCommit(storage.ObjectID(s.Revision)); e != nil {
		return false
	}
	switch s.Kind {
	case "pull_preview":
		v, e := pulls.Get(repo, s.ResourceID)
		return e == nil && v.SourceCommitID == s.Revision
	case "release_candidate":
		v, e := releasesStore.Get(repo, s.ResourceID)
		return e == nil && v.CommitID == s.Revision
	case "issue":
		v, e := issuesStore.Get(repo, s.ResourceID)
		return e == nil && v.RepositoryID == repo
	case "quality_plan":
		v, e := plans.Get(s.ResourceID)
		return e == nil && v.RepositoryID == repo
	default:
		return false
	}
}
func exploratorySessionStale(repo string, v exploratorysessions.Session, pulls *pullrequests.Store, releasesStore *releases.Store, issuesStore *issues.Store, plans *qualityplans.Store) (bool, string) {
	switch v.Source.Kind {
	case "pull_preview":
		p, e := pulls.Get(repo, v.Source.ResourceID)
		if e != nil || p.SourceCommitID != v.Source.Revision {
			return true, "pull candidate moved or is unavailable"
		}
	case "release_candidate":
		x, e := releasesStore.Get(repo, v.Source.ResourceID)
		if e != nil || x.CommitID != v.Source.Revision {
			return true, "release candidate changed or is unavailable"
		}
	case "issue":
		if _, e := issuesStore.Get(repo, v.Source.ResourceID); e != nil {
			return true, "source issue is unavailable"
		}
	case "quality_plan":
		if _, e := plans.Get(v.Source.ResourceID); e != nil {
			return true, "source quality plan is unavailable"
		}
	}
	return false, ""
}
func writeExploratorySession(w http.ResponseWriter, v exploratorysessions.Session, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, exploratorysessions.ErrNotFound):
		writeAPIError(w, 404, "exploratory_session_not_found", "exploratory session not found")
	case errors.Is(e, exploratorysessions.ErrConflict):
		writeAPIError(w, 409, "exploratory_session_conflict", "session changed; reload its shared timeline")
	case errors.Is(e, exploratorysessions.ErrInvalid):
		writeAPIError(w, 400, "invalid_exploratory_event", "event violates session state, scope, budget, or agent charter")
	default:
		log.Printf("exploratory session storage: %v", e)
		writeAPIError(w, 500, "exploratory_sessions_unavailable", "exploratory session could not be persisted")
	}
}
