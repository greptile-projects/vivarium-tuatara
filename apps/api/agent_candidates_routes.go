package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentcandidates"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentevaluations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentprojects"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type candidateCreateInput struct {
	IdempotencyKey string `json:"idempotency_key"`
	PullRevision   string `json:"pull_revision"`
	ProjectID      string `json:"project_id"`
	ProjectVersion int    `json:"project_version"`
	Suites         []struct {
		SuiteID string `json:"suite_id"`
		Version int    `json:"version"`
	} `json:"suites"`
	BaselineCandidateID string `json:"baseline_candidate_id"`
}

func registerAgentCandidateRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, projects *agentprojects.Store, suites *agentevaluations.Store, candidates *agentcandidates.Store) {
	writeErr := func(w http.ResponseWriter, e error) {
		switch {
		case errors.Is(e, agentcandidates.ErrNotFound):
			writeAPIError(w, 404, "agent_candidate_not_found", "agent candidate not found")
		case errors.Is(e, agentcandidates.ErrConflict):
			writeAPIError(w, 409, "agent_candidate_conflict", "the pull candidate changed")
		case errors.Is(e, agentcandidates.ErrInvalid):
			writeAPIError(w, 422, "agent_candidate_invalid", "candidate or bounded evaluation evidence is invalid")
		default:
			writeAPIError(w, 500, "agent_candidate_unavailable", "agent candidate evidence is unavailable")
		}
	}
	authRead := func(w http.ResponseWriter, r *http.Request) bool {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		return ok
	}
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/agent-candidates", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in candidateCreateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact pull candidate and selected suites are required")
			return
		}
		project, e := projects.Get(in.ProjectID)
		if e != nil || project.RepositoryID != r.PathValue("id") || in.ProjectVersion < 1 || in.ProjectVersion > len(project.Revisions) {
			writeErr(w, agentcandidates.ErrInvalid)
			return
		}
		rev := project.Revisions[in.ProjectVersion-1]
		components := []agentcandidates.ComponentDigest{{Kind: "behavior_contract", ID: in.ProjectID, Digest: agentcandidates.Digest(rev)}}
		for _, x := range rev.Sources {
			components = append(components, agentcandidates.ComponentDigest{Kind: x.Kind, ID: x.ID, Digest: agentcandidates.Digest(x)})
		}
		for _, x := range rev.Tools {
			components = append(components, agentcandidates.ComponentDigest{Kind: "tool", ID: x.Name, Digest: agentcandidates.Digest(x)})
		}
		for _, x := range rev.Models {
			components = append(components, agentcandidates.ComponentDigest{Kind: "model", ID: x.Name, Digest: agentcandidates.Digest(x)})
		}
		selected := make([]agentcandidates.SuiteSelection, 0, len(in.Suites))
		for _, x := range in.Suites {
			suite, se := suites.Get(x.SuiteID)
			if se != nil || suite.RepositoryID != r.PathValue("id") || x.Version < 1 || x.Version > len(suite.Revisions) {
				writeErr(w, agentcandidates.ErrInvalid)
				return
			}
			sr := suite.Revisions[x.Version-1]
			d := agentcandidates.Digest(sr)
			scenarioIDs := make([]string, 0, len(sr.Scenarios))
			for _, scenario := range sr.Scenarios {
				scenarioIDs = append(scenarioIDs, scenario.ID)
			}
			selected = append(selected, agentcandidates.SuiteSelection{SuiteID: x.SuiteID, Version: x.Version, Digest: d, ScenarioIDs: scenarioIDs})
			components = append(components, agentcandidates.ComponentDigest{Kind: "scenario_judge", ID: x.SuiteID, Digest: d})
		}
		if in.BaselineCandidateID != "" {
			b, be := candidates.Get(in.BaselineCandidateID)
			if be != nil || b.RepositoryID != r.PathValue("id") {
				writeErr(w, agentcandidates.ErrInvalid)
				return
			}
		}
		var out agentcandidates.Candidate
		e = pulls.WithSourceRevision(r.PathValue("id"), r.PathValue("pull_id"), in.PullRevision, func(p pullrequests.PullRequest) error {
			var ce error
			out, ce = candidates.Create(agentcandidates.Candidate{IdempotencyKey: in.IdempotencyKey, RepositoryID: p.RepositoryID, PullRequestID: p.ID, PullRevision: p.SourceCommitID, ProjectID: project.ID, ProjectVersion: in.ProjectVersion, ContractDigest: agentcandidates.Digest(rev), Components: components, Suites: selected, BaselineCandidateID: in.BaselineCandidateID, CreatedBy: actor.UserID})
			return ce
		})
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/agent-candidates", func(w http.ResponseWriter, r *http.Request) {
		if !authRead(w, r) {
			return
		}
		items, e := candidates.List(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeErr(w, e)
			return
		}
		type view struct {
			agentcandidates.Candidate
			Stale      bool                       `json:"stale"`
			Runs       []agentcandidates.Run      `json:"runs"`
			Comparison agentcandidates.Comparison `json:"comparison"`
		}
		out := []view{}
		p, _ := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		for _, c := range items {
			runs, re := candidates.Runs(c.ID)
			cmp, ce := candidates.Compare(c)
			if re != nil || ce != nil {
				writeErr(w, agentcandidates.ErrInvalid)
				return
			}
			out = append(out, view{Candidate: c, Stale: p.SourceCommitID != c.PullRevision, Runs: runs, Comparison: cmp})
		}
		writeJSON(w, 200, map[string]any{"candidates": out})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/agent-candidates/{candidate_id}/runs", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		c, e := candidates.Get(r.PathValue("candidate_id"))
		if e != nil || c.RepositoryID != r.PathValue("id") || c.PullRequestID != r.PathValue("pull_id") {
			writeErr(w, agentcandidates.ErrNotFound)
			return
		}
		var in agentcandidates.Run
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "complete isolated scenario evidence is required")
			return
		}
		in.ID = ""
		in.CandidateID = c.ID
		out, e := candidates.CreateRun(in, actor.UserID)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, out)
	})
}
