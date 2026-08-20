package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityfindings"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityscenarios"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/threatmodels"
)

func registerSecurityFindingRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, findings *securityfindings.Store, models *threatmodels.Store, scenarios *securityscenarios.Store, proposalStore *proposals.Store, pulls *pullrequests.Store) {
	owner := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool) {
		a, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return a, false
		}
		repo, _ := catalog.GetByID(r.PathValue("id"))
		if a.AgentID != "" || repo.OwnerID != a.UserID {
			writeAPIError(w, 403, "security_finding_owner_required", "the repository owner must govern finding disclosure and repair")
			return a, false
		}
		return a, true
	}
	writeErr := func(w http.ResponseWriter, e error) {
		switch {
		case errors.Is(e, securityfindings.ErrNotFound):
			writeAPIError(w, 404, "security_finding_not_found", "security finding not found")
		case errors.Is(e, securityfindings.ErrConflict):
			writeAPIError(w, 409, "security_finding_changed", "security finding changed")
		case errors.Is(e, securityfindings.ErrInvalid):
			writeAPIError(w, 422, "security_finding_invalid", "security finding state is invalid")
		default:
			writeAPIError(w, 500, "security_findings_unavailable", "security finding could not be persisted")
		}
	}
	mux.HandleFunc("GET /repositories/{id}/security-findings", func(w http.ResponseWriter, r *http.Request) {
		a, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		identity := a.UserID
		if a.AgentID != "" {
			identity = a.AgentID
		}
		v, e := findings.List(r.PathValue("id"), identity)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"security_findings": v})
	})
	mux.HandleFunc("POST /repositories/{id}/security-findings", func(w http.ResponseWriter, r *http.Request) {
		a, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in securityfindings.Finding
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		encoded, _ := json.Marshal(in)
		if reusableSecret.Match(encoded) {
			writeAPIError(w, 422, "security_finding_evidence_unsafe", "finding records cannot retain credential-shaped evidence")
			return
		}
		m, e := models.Get(r.PathValue("id"), in.ThreatModelID, threatmodels.CurrentSource{})
		if e != nil || in.ThreatModelVersion < 1 || in.ThreatModelVersion > len(m.Revisions) {
			writeAPIError(w, 422, "security_finding_threat_invalid", "the exact threat-model revision is required")
			return
		}
		rev := m.Revisions[in.ThreatModelVersion-1]
		path := false
		for _, x := range rev.AbusePaths {
			path = path || x.ID == in.AbusePathID
		}
		if !path || rev.Source.Revision != in.CandidateCommitID {
			writeAPIError(w, 422, "security_finding_candidate_invalid", "the exact modeled abuse path and candidate are required")
			return
		}
		for _, id := range in.Audience {
			repo, _ := catalog.GetByID(r.PathValue("id"))
			collab, _ := catalog.HasCollaborator(id, r.PathValue("id"))
			if id != a.UserID && id != a.AgentID && id != repo.OwnerID && !collab {
				writeAPIError(w, 422, "security_finding_audience_invalid", "audience members must be current repository participants")
				return
			}
		}
		out, e := findings.Create(r.PathValue("id"), a.UserID, in)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("POST /repositories/{id}/security-findings/{finding_id}/classification", func(w http.ResponseWriter, r *http.Request) {
		a, ok := owner(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int      `json:"expected_version"`
			Classification  string   `json:"classification"`
			Rationale       string   `json:"rationale"`
			Audience        []string `json:"audience"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		repo, _ := catalog.GetByID(r.PathValue("id"))
		current, _ := findings.Get(r.PathValue("id"), r.PathValue("finding_id"))
		for _, id := range in.Audience {
			collab, _ := catalog.HasCollaborator(id, r.PathValue("id"))
			if id != repo.OwnerID && !collab && !slices.Contains(current.Audience, id) {
				writeAPIError(w, 422, "security_finding_audience_invalid", "audience members must be current repository participants")
				return
			}
		}
		v, e := findings.Decide(r.PathValue("id"), r.PathValue("finding_id"), a.UserID, in.ExpectedVersion, in.Classification, in.Rationale, in.Audience)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/security-findings/{finding_id}/repair", func(w http.ResponseWriter, r *http.Request) {
		a, ok := owner(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			AssigneeType    string `json:"assignee_type"`
			AssigneeID      string `json:"assignee_id"`
			Title           string `json:"title"`
			WorkspaceKind   string `json:"workspace_kind"`
		}
		if decodeJSON(r, &in) != nil || !slices.Contains([]string{"task", "change_session", "shared_workspace"}, in.WorkspaceKind) {
			writeAPIError(w, 422, "security_repair_invalid", "an ordinary task, change session, or shared workspace handoff is required")
			return
		}
		v, e := findings.Get(r.PathValue("id"), r.PathValue("finding_id"))
		if e != nil || !securityfindings.Visible(v, a.UserID) {
			writeErr(w, securityfindings.ErrNotFound)
			return
		}
		if v.Version != in.ExpectedVersion || securityfindings.CurrentClassification(v) != "confirmed" {
			writeErr(w, securityfindings.ErrConflict)
			return
		}
		if in.AssigneeType == "human" {
			repo, _ := catalog.GetByID(v.RepositoryID)
			c, _ := catalog.HasCollaborator(in.AssigneeID, v.RepositoryID)
			if in.AssigneeID != repo.OwnerID && !c {
				writeAPIError(w, 422, "security_repair_assignee_invalid", "human assignees must be current repository participants")
				return
			}
		}
		items := []proposals.ReasoningItem{}
		selected := []string{}
		for _, x := range v.Evidence {
			items = append(items, proposals.ReasoningItem{ID: x.ID, Kind: "permitted_security_evidence", Summary: x.Summary, Status: "audience_restricted"})
			selected = append(selected, x.ID)
		}
		criteria := strings.Join(v.AcceptanceCriteria, "\n- ")
		origin := proposals.ReasoningOrigin{SecurityFindingID: v.ID, SecurityFindingVersion: v.Version, ThreatModelID: v.ThreatModelID, ThreatModelVersion: v.ThreatModelVersion, Revision: v.CandidateCommitID, SelectedItemIDs: selected, Items: items, AnalysisStatus: "security_finding_repair"}
		p, tasks, e := proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: v.RepositoryID, ActorID: a.UserID, Title: "Security repair: " + in.Title, Body: "Audience-controlled repair for security finding " + v.ID + ". Launch the requested " + in.WorkspaceKind + " through ordinary task controls.\n\nAcceptance criteria:\n- " + criteria, Origin: origin, Tasks: []proposals.ImplementationTaskInput{{Title: in.Title, Outcome: "Contain the exact modeled abuse path without expanding authority or disclosure.", Risk: v.Severity + " security weakness at " + v.CandidateCommitID, VerificationPlan: "Demonstrate the weakness against the affected base and containment at the exact repair commit; retain review and safe scenario coverage.\n- " + criteria, AssigneeType: in.AssigneeType, AssigneeID: in.AssigneeID}}})
		if e != nil || len(tasks) == 0 {
			writeAPIError(w, 500, "security_repair_unavailable", "ordinary governed work could not be created")
			return
		}
		v, e = findings.LinkRepair(v.RepositoryID, v.ID, a.UserID, in.ExpectedVersion, securityfindings.Repair{ProposalID: p.ID, TaskID: tasks[0].ID, AssigneeType: in.AssigneeType, AssigneeID: in.AssigneeID})
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, map[string]any{"security_finding": v, "proposal": p, "task": tasks[0]})
	})
	mux.HandleFunc("POST /repositories/{id}/security-findings/{finding_id}/protection", func(w http.ResponseWriter, r *http.Request) {
		a, ok := owner(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			PullRequestID   string `json:"pull_request_id"`
			ScenarioID      string `json:"scenario_id"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, e := findings.Get(r.PathValue("id"), r.PathValue("finding_id"))
		if e != nil || v.Repair == nil {
			writeErr(w, securityfindings.ErrNotFound)
			return
		}
		p, e := pulls.Get(v.RepositoryID, in.PullRequestID)
		if e != nil || p.TaskID == nil || *p.TaskID != v.Repair.TaskID {
			writeAPIError(w, 422, "security_repair_pull_invalid", "the pull must be the ordinary contribution for the frozen repair task")
			return
		}
		s, e := scenarios.Get(v.RepositoryID, in.ScenarioID)
		if e != nil || s.ThreatModelID != v.ThreatModelID || s.AbusePathID != v.AbusePathID || s.CommitID != p.SourceCommitID || s.Review == nil || s.Review.Decision != "approved" || !passedSecurityAttempt(s) {
			writeAPIError(w, 422, "security_repair_scenario_invalid", "an owner-reviewed passing scenario at the exact repair commit is required")
			return
		}
		baseProved := false
		all, _ := scenarios.List(v.RepositoryID)
		for _, x := range all {
			baseProved = baseProved || (x.ThreatModelID == v.ThreatModelID && x.AbusePathID == v.AbusePathID && x.CommitID == v.CandidateCommitID && failedSecurityAttempt(x))
		}
		if !baseProved {
			writeAPIError(w, 422, "security_finding_base_unproven", "a retained failed abuse attempt against the affected base is required")
			return
		}
		v, e = findings.Complete(v.RepositoryID, v.ID, a.UserID, in.ExpectedVersion, p.ID, p.SourceCommitID, s.ID)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, v)
	})
}
func passedSecurityAttempt(s securityscenarios.Scenario) bool {
	for _, a := range s.Attempts {
		if a.Result == "passed" && a.Coverage.AbuseAttempted && len(a.Coverage.ContainmentIDs) > 0 {
			return true
		}
	}
	return false
}
func failedSecurityAttempt(s securityscenarios.Scenario) bool {
	for _, a := range s.Attempts {
		if a.Result == "failed" && a.Coverage.AbuseAttempted {
			return true
		}
	}
	return false
}
