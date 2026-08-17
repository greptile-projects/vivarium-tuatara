package main

import (
	"errors"
	"net/http"
	"os/exec"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentevaluations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type evaluationSuiteInput struct {
	Name            string                    `json:"name"`
	RepositoryID    string                    `json:"repository_id"`
	ExpectedVersion int                       `json:"expected_version"`
	Revision        agentevaluations.Revision `json:"revision"`
}
type evaluationRunInput struct {
	SuiteVersion int                       `json:"suite_version"`
	Trial        agentevaluations.RunInput `json:"trial"`
}
type evaluationDecisionInput struct {
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
}

func registerAgentEvaluationRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, orgs *organizations.Store, evaluations *agentevaluations.Store) {
	require := func(w http.ResponseWriter, r *http.Request, scope string) (auth.Credential, organizations.Organization, bool) {
		actor, ok := authenticateRequest(w, r, credentials, scope, false)
		if !ok {
			return actor, organizations.Organization{}, false
		}
		org, err := orgs.Get(r.PathValue("id"))
		if err != nil || !organizations.HasRole(org, actor.UserID, "") {
			writeAPIError(w, 404, "organization_not_found", "organization not found")
			return actor, org, false
		}
		return actor, org, true
	}
	belongs := func(orgID, repositoryID string) bool {
		items, err := catalog.ListOrganization(orgID)
		if err != nil {
			return false
		}
		for _, x := range items {
			if x.ID == repositoryID {
				return true
			}
		}
		return false
	}
	writeErr := func(w http.ResponseWriter, err error) {
		switch {
		case errors.Is(err, agentevaluations.ErrNotFound):
			writeAPIError(w, 404, "evaluation_not_found", "evaluation not found")
		case errors.Is(err, agentevaluations.ErrConflict):
			writeAPIError(w, 409, "evaluation_conflict", "evaluation suite changed; reload its current version")
		case errors.Is(err, agentevaluations.ErrInvalid):
			writeAPIError(w, 422, "evaluation_invalid", "evaluation evidence is incomplete, unsafe, or outside its bounds")
		default:
			writeAPIError(w, 500, "evaluation_unavailable", "evaluation evidence is unavailable")
		}
	}
	mux.HandleFunc("GET /organizations/{id}/agent-evaluation-suites", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := require(w, r, "repositories:read"); !ok {
			return
		}
		v, e := evaluations.PublicList(r.PathValue("id"))
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"suites": v})
	})
	mux.HandleFunc("POST /organizations/{id}/agent-evaluation-suites", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in evaluationSuiteInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete evaluation suite is required")
			return
		}
		if !belongs(org.ID, in.RepositoryID) {
			writeAPIError(w, 422, "evaluation_repository_invalid", "suite repository must belong to this organization")
			return
		}
		repo, e := git.Open(in.RepositoryID)
		if e != nil || exec.Command("git", "--git-dir="+repo.Path(), "cat-file", "-e", in.Revision.RepositoryRevision+"^{commit}").Run() != nil {
			writeAPIError(w, 422, "evaluation_revision_invalid", "the exact repository revision is unavailable")
			return
		}
		in.Revision.CreatedBy = actor.UserID
		var v agentevaluations.Suite
		if in.ExpectedVersion == 0 {
			e = catalog.WithCurrentParticipant(actor.UserID, in.RepositoryID, func() error {
				var x error
				v, x = evaluations.Create(agentevaluations.Suite{OrganizationID: org.ID, RepositoryID: in.RepositoryID, Name: in.Name}, in.Revision)
				return x
			})
		} else {
			writeAPIError(w, 422, "evaluation_invalid", "create requires expected_version zero")
			return
		}
		if e != nil {
			writeErr(w, e)
			return
		}
		view, _ := evaluations.PublicGet(v.ID)
		writeJSON(w, 201, view)
	})
	mux.HandleFunc("PUT /organizations/{id}/agent-evaluation-suites/{suite_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in evaluationSuiteInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete successor suite is required")
			return
		}
		current, e := evaluations.Get(r.PathValue("suite_id"))
		if e != nil || current.OrganizationID != org.ID {
			writeErr(w, agentevaluations.ErrNotFound)
			return
		}
		in.Revision.CreatedBy = actor.UserID
		repo, e := git.Open(current.RepositoryID)
		if e != nil || exec.Command("git", "--git-dir="+repo.Path(), "cat-file", "-e", in.Revision.RepositoryRevision+"^{commit}").Run() != nil {
			writeAPIError(w, 422, "evaluation_revision_invalid", "the exact repository revision is unavailable")
			return
		}
		var v agentevaluations.Suite
		e = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error {
			var x error
			v, x = evaluations.Revise(current.ID, in.ExpectedVersion, in.Revision)
			return x
		})
		if e != nil {
			writeErr(w, e)
			return
		}
		view, _ := evaluations.PublicGet(v.ID)
		writeJSON(w, 200, view)
	})
	mux.HandleFunc("GET /organizations/{id}/agent-evaluation-runs", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		var v []agentevaluations.Run
		var e error
		if organizations.HasRole(org, actor.UserID, "owner") {
			v, e = evaluations.ListEvaluatorRuns(org.ID)
		} else {
			v, e = evaluations.ListRuns(org.ID)
		}
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"runs": v})
	})
	mux.HandleFunc("POST /organizations/{id}/agent-evaluation-suites/{suite_id}/runs", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in evaluationRunInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "bounded trial evidence is required")
			return
		}
		suite, e := evaluations.Get(r.PathValue("suite_id"))
		if e != nil || suite.OrganizationID != org.ID {
			writeErr(w, agentevaluations.ErrNotFound)
			return
		}
		validAgent := false
		for _, a := range org.Agents {
			if a.ID == in.Trial.AgentID && in.Trial.AgentProfileVersion > 0 && in.Trial.AgentProfileVersion <= len(a.Profiles) {
				validAgent = true
			}
		}
		if !validAgent {
			writeAPIError(w, 422, "evaluation_agent_invalid", "candidate and exact published profile version are required")
			return
		}
		var run agentevaluations.Run
		e = catalog.WithCurrentParticipant(actor.UserID, suite.RepositoryID, func() error {
			var x error
			run, x = evaluations.CreateRun(suite.ID, in.SuiteVersion, in.Trial, actor.UserID)
			return x
		})
		if e != nil {
			writeErr(w, e)
			return
		}
		run, e = evaluations.GetRun(run.ID)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, run)
	})
	mux.HandleFunc("POST /organizations/{id}/agent-evaluation-runs/{run_id}/decisions", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		if !organizations.HasRole(org, actor.UserID, "owner") {
			writeAPIError(w, 403, "evaluation_owner_required", "only organization owners can decide protected evaluation evidence")
			return
		}
		run, e := evaluations.GetEvaluatorRun(r.PathValue("run_id"))
		if e != nil || run.OrganizationID != org.ID {
			writeErr(w, agentevaluations.ErrNotFound)
			return
		}
		var in evaluationDecisionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a decision and rationale are required")
			return
		}
		var v agentevaluations.Run
		e = catalog.WithCurrentParticipant(actor.UserID, run.RepositoryID, func() error {
			var x error
			v, x = evaluations.Decide(run.ID, actor.UserID, in.Decision, in.Rationale)
			return x
		})
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, v)
	})
}
