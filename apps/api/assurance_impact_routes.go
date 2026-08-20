package main

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceimpact"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type assuranceImpactInput struct {
	ProgramID      string `json:"program_id"`
	ProgramVersion int    `json:"program_version"`
	Decisions      []struct {
		ControlID        string                   `json:"control_id"`
		Applicability    string                   `json:"applicability"`
		Rationale        string                   `json:"rationale"`
		Tests            []assuranceimpact.Action `json:"tests"`
		Notices          []assuranceimpact.Action `json:"notices"`
		RetentionActions []assuranceimpact.Action `json:"retention_actions"`
		ExceptionIDs     []string                 `json:"exception_ids"`
		Mitigation       string                   `json:"mitigation"`
		ResidualRisk     string                   `json:"residual_risk"`
	} `json:"decisions"`
}
type assuranceImpactEventInput struct {
	ExpectedVersion int                   `json:"expected_version"`
	Event           assuranceimpact.Event `json:"event"`
}
type assuranceImpactAckInput struct {
	ExpectedVersion int    `json:"expected_version"`
	ControlID       string `json:"control_id"`
}

func registerAssuranceImpactRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, impacts *assuranceimpact.Store, programs *assuranceprograms.Store, pulls *pullrequests.Store) {
	current := func(a assuranceimpact.Assessment) assuranceimpact.Current {
		p, e := pulls.Get(a.RepositoryID, a.Candidate.ID)
		if e != nil {
			return assuranceimpact.Current{}
		}
		program, e := programs.Get(a.ProgramID)
		if e != nil || program.RepositoryID != a.RepositoryID || len(program.Revisions) == 0 {
			return assuranceimpact.Current{CandidateRevision: p.SourceCommitID}
		}
		changes, e := pulls.Changes(a.RepositoryID, a.Candidate.ID)
		if e != nil {
			return assuranceimpact.Current{}
		}
		return assuranceImpactCurrent(p.SourceCommitID, program.Revisions[len(program.Revisions)-1], changes, assuranceImpactParticipants(catalog, a.RepositoryID, program.Revisions[len(program.Revisions)-1]))
	}
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/assurance-impact", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, e := impacts.List(r.PathValue("id"), current)
		if e != nil {
			writeAPIError(w, 500, "assurance_impacts_unavailable", "assurance impacts could not be read")
			return
		}
		filtered := []assuranceimpact.Assessment{}
		for _, v := range values {
			if v.Candidate.Kind == "pull_request" && v.Candidate.ID == r.PathValue("pull_id") {
				filtered = append(filtered, v)
			}
		}
		writeJSON(w, 200, map[string]any{"assurance_impacts": filtered})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/assurance-impact", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok || actor.AgentID != "" {
			return
		}
		var in assuranceImpactInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete assurance impact decision is required")
			return
		}
		pull, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		program, e := programs.Get(in.ProgramID)
		if e != nil || program.RepositoryID != r.PathValue("id") || in.ProgramVersion < 1 || in.ProgramVersion > len(program.Revisions) {
			writeAPIError(w, 400, "invalid_assurance_impact", "an exact assurance program revision is required")
			return
		}
		revision := program.Revisions[in.ProgramVersion-1]
		changes, e := pulls.Changes(pull.RepositoryID, pull.ID)
		if e != nil {
			writeAPIError(w, 500, "assurance_impacts_unavailable", "candidate changes could not be derived")
			return
		}
		decisions := map[string]assuranceImpactInputDecision{}
		for _, d := range in.Decisions {
			decisions[d.ControlID] = assuranceImpactInputDecision{d.Applicability, d.Rationale, d.Tests, d.Notices, d.RetentionActions, d.ExceptionIDs, d.Mitigation, d.ResidualRisk}
		}
		derived := deriveAssuranceImpacts(revision, changes, decisions)
		owners := []string{actor.UserID}
		for _, x := range derived {
			owners = append(owners, x.RequiredOwnerIDs...)
		}
		var out assuranceimpact.Assessment
		e = catalog.WithCurrentParticipants(owners, pull.RepositoryID, func() error {
			var x error
			out, x = impacts.Create(pull.RepositoryID, actor.UserID, assuranceimpact.Candidate{Kind: "pull_request", ID: pull.ID, Revision: pull.SourceCommitID}, program.ID, in.ProgramVersion, derived)
			return x
		})
		writeAssuranceImpact(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/assurance-impacts/{impact_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authorizeThreatModelContributor(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		var in assuranceImpactEventInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a cited assurance analysis event is required")
			return
		}
		actorID, actorType := actor.UserID, "human"
		if actor.AgentID != "" {
			assessment, e := impacts.Get(r.PathValue("id"), r.PathValue("impact_id"), assuranceimpact.Current{})
			if e != nil || assessment.Candidate.Kind != "pull_request" {
				writeAPIError(w, 403, "assurance_impact_agent_scope_invalid", "agent analysis requires the exact source pull task credential")
				return
			}
			pull, e := pulls.Get(assessment.RepositoryID, assessment.Candidate.ID)
			if e != nil || pull.TaskID == nil || actor.RepositoryID != assessment.RepositoryID || actor.GitWriteBranch != "refs/heads/"+pull.SourceBranch || pull.SourceCommitID != assessment.Candidate.Revision {
				writeAPIError(w, 403, "assurance_impact_agent_scope_invalid", "agent analysis requires the exact source pull task credential")
				return
			}
			actorID, actorType = actor.AgentID, "agent"
		}
		assessment, _ := impacts.Get(r.PathValue("id"), r.PathValue("impact_id"), assuranceimpact.Current{})
		out, e := impacts.AddEvent(r.PathValue("id"), assessment.ID, in.ExpectedVersion, actorID, actorType, in.Event, current(assessment))
		writeAssuranceImpact(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/assurance-impacts/{impact_id}/acknowledgements", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok || actor.AgentID != "" {
			return
		}
		var in assuranceImpactAckInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a control acknowledgement is required")
			return
		}
		assessment, _ := impacts.Get(r.PathValue("id"), r.PathValue("impact_id"), assuranceimpact.Current{})
		out, e := impacts.Acknowledge(r.PathValue("id"), r.PathValue("impact_id"), in.ControlID, actor.UserID, in.ExpectedVersion, current(assessment))
		writeAssuranceImpact(w, out, e, 201)
	})
}

type assuranceImpactInputDecision struct {
	Applicability, Rationale  string
	Tests, Notices, Retention []assuranceimpact.Action
	Exceptions                []string
	Mitigation, Residual      string
}

func deriveAssuranceImpacts(r assuranceprograms.Revision, changes []pullrequests.FileChange, decisions map[string]assuranceImpactInputDecision) []assuranceimpact.ControlImpact {
	paths := []string{}
	for _, c := range changes {
		paths = append(paths, c.Path)
	}
	scopes := map[string]assuranceprograms.Scope{}
	for _, s := range r.Scopes {
		scopes[s.ID] = s
	}
	out := []assuranceimpact.ControlImpact{}
	for _, c := range r.Controls {
		affected := []string{}
		for _, m := range c.Mappings {
			s := scopes[m.ScopeID]
			for _, p := range paths {
				if s.Kind == "repository" || s.Path == p || s.Path != "" && strings.HasPrefix(p, strings.TrimSuffix(s.Path, "/")+"/") {
					affected = appendUnique(affected, p)
				}
			}
		}
		if len(affected) == 0 {
			continue
		}
		d, ok := decisions[c.ID]
		if !ok {
			d.Applicability = "uncertain"
			d.Rationale = "Applicability awaits cited reviewer analysis."
		}
		evidence := []string{}
		for _, x := range c.EvidenceCriteria {
			evidence = append(evidence, x.ID)
		}
		out = append(out, assuranceimpact.ControlImpact{ControlID: c.ID, ControlTitle: c.Title, ControlDigest: assuranceimpact.Digest(c), Applicability: d.Applicability, Rationale: d.Rationale, AffectedPaths: affected, ChangedEvidenceIDs: evidence, RequiredOwnerIDs: c.OwnerIDs, Tests: d.Tests, Notices: d.Notices, RetentionActions: d.Retention, ExceptionIDs: d.Exceptions, Mitigation: d.Mitigation, ResidualRisk: d.Residual})
	}
	return out
}
func assuranceImpactCurrent(candidate string, r assuranceprograms.Revision, changes []pullrequests.FileChange, participants map[string]bool) assuranceimpact.Current {
	m := map[string]string{}
	for _, impact := range deriveAssuranceImpacts(r, changes, map[string]assuranceImpactInputDecision{}) {
		m[impact.ControlID] = impact.ControlDigest
	}
	return assuranceimpact.Current{CandidateRevision: candidate, ControlDigests: m, ParticipantIDs: participants}
}

func assuranceImpactParticipants(catalog *repositories.Store, repo string, revision assuranceprograms.Revision) map[string]bool {
	current := map[string]bool{}
	repository, err := catalog.GetByID(repo)
	if err != nil {
		return current
	}
	for _, control := range revision.Controls {
		for _, owner := range control.OwnerIDs {
			if owner == repository.OwnerID {
				current[owner] = true
				continue
			}
			present, err := catalog.HasCollaborator(owner, repo)
			if err == nil && present {
				current[owner] = true
			}
		}
	}
	return current
}
func appendUnique(xs []string, v string) []string {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}
func writeAssuranceImpact(w http.ResponseWriter, v assuranceimpact.Assessment, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, assuranceimpact.ErrNotFound):
		writeAPIError(w, 404, "assurance_impact_not_found", "assurance impact not found")
	case errors.Is(e, assuranceimpact.ErrConflict):
		writeAPIError(w, 409, "assurance_impact_conflict", "the assurance impact changed; reload before contributing")
	case errors.Is(e, assuranceimpact.ErrForbidden):
		writeAPIError(w, 403, "assurance_impact_acknowledgement_forbidden", "only a required current control owner may acknowledge")
	case errors.Is(e, assuranceimpact.ErrInvalid):
		writeAPIError(w, 400, "invalid_assurance_impact", "candidate, applicability, actions, citations, and residual risk must be complete")
	default:
		log.Printf("assurance impact storage: %v", e)
		writeAPIError(w, 500, "assurance_impacts_unavailable", "assurance impact could not be persisted")
	}
}
