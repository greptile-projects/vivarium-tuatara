package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityscenarios"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/threatmodels"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

type securityAttemptInput struct {
	Attempt       securityscenarios.Attempt `json:"attempt"`
	OutcomeIDs    []string                  `json:"outcome_ids"`
	PullRequestID string                    `json:"pull_request_id"`
}
type securityReviewInput struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}
type securityCheckConfig struct {
	Version int `json:"version"`
	Checks  []struct {
		AbusePathID string `json:"abuse_path_id"`
		Command     string `json:"command"`
		Isolation   string `json:"isolation"`
	} `json:"checks"`
}

func registerSecurityScenarioRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, scenarios *securityscenarios.Store, models *threatmodels.Store, workspacesStore *workspaces.Store, previewStore *previews.Store, runs *checkruns.Store) {
	mux.HandleFunc("GET /repositories/{id}/security-scenarios", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := scenarios.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "security_scenarios_unavailable", "security scenarios could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"security_scenarios": v})
	})
	mux.HandleFunc("POST /repositories/{id}/security-scenarios", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authorizeThreatModelContributor(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		var in securityscenarios.Scenario
		if decodeJSON(r, &in) != nil || !securityscenarios.Valid(in) {
			writeAPIError(w, 400, "invalid_security_scenario", "a complete bounded abuse and defense scenario is required")
			return
		}
		model, e := models.Get(r.PathValue("id"), in.ThreatModelID, threatmodels.CurrentSource{})
		rev, retained := retainedThreatModelRevision(model, in.ThreatModelVersion)
		if e != nil || !retained {
			writeAPIError(w, 422, "security_scenario_threat_invalid", "the exact threat-model revision is required")
			return
		}
		if actor.AgentID != "" && (rev.Source.Kind != "pull_request" || actor.RepositoryID != r.PathValue("id") || actor.PullRequestID != rev.Source.ResourceID || actor.GitWriteBranch == "") {
			writeAPIError(w, 403, "security_scenario_agent_scope_invalid", "agent scenarios require the exact source-pull task credential")
			return
		}
		if rev.Source.Kind == "pull_request" && rev.Source.Revision != in.CommitID {
			writeAPIError(w, 422, "security_scenario_candidate_invalid", "pull-sourced scenarios must evaluate the exact modeled candidate")
			return
		}
		if !scenarioGraph(rev, in) {
			writeAPIError(w, 422, "security_scenario_path_invalid", "the abuse path, mitigations, and dependencies must resolve in the threat model")
			return
		}
		body, digest, found := infrastructureCommitBlob(git, r.PathValue("id"), in.CommitID, in.CheckPath)
		if !found || digest != in.CheckSHA256 || reusableSecret.Match(body) {
			writeAPIError(w, 422, "security_check_invalid", "the candidate-defined security check must resolve and contain no secret-shaped material")
			return
		}
		var config securityCheckConfig
		if json.Unmarshal(body, &config) != nil || config.Version < 1 {
			writeAPIError(w, 422, "security_check_invalid", "the candidate security-check definition is invalid")
			return
		}
		defined := false
		for _, check := range config.Checks {
			defined = defined || check.AbusePathID == in.AbusePathID && check.Command == in.Command && check.Isolation == in.Isolation
		}
		if !defined {
			writeAPIError(w, 422, "security_check_invalid", "the command and isolation must be candidate-defined for this abuse path")
			return
		}
		for _, f := range in.Fixtures {
			data, d, found := infrastructureCommitBlob(git, r.PathValue("id"), in.CommitID, f.Path)
			if !found || d != f.SHA256 || reusableSecret.Match(data) {
				writeAPIError(w, 422, "security_fixture_unsafe", "fixtures must be exact synthetic, anonymized, or public candidate files without credentials")
				return
			}
		}
		encoded, _ := json.Marshal(in)
		if reusableSecret.Match(encoded) || unsafeSecurityText(in) {
			writeAPIError(w, 400, "security_scenario_unsafe", "destructive effects, secrets, production data, hidden material, and unbounded capabilities are forbidden")
			return
		}
		kind := "human"
		identity := actor.UserID
		if actor.AgentID != "" {
			kind = "agent"
			identity = actor.AgentID
		}
		out, e := scenarios.Create(r.PathValue("id"), identity, kind, in)
		writeSecurityScenario(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/security-scenarios/{scenario_id}/review", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok || actor.AgentID != "" {
			if ok {
				writeAPIError(w, 403, "security_review_human_required", "only an affected human owner can review a scenario")
			}
			return
		}
		var in securityReviewInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a review is required")
			return
		}
		v, e := scenarios.Get(r.PathValue("id"), r.PathValue("scenario_id"))
		if e != nil {
			writeSecurityScenario(w, v, e, 200)
			return
		}
		if actor.AgentID != "" {
			m, modelErr := models.Get(v.RepositoryID, v.ThreatModelID, threatmodels.CurrentSource{})
			if modelErr != nil || v.ThreatModelVersion < 1 || v.ThreatModelVersion > len(m.Revisions) {
				writeAPIError(w, 403, "security_scenario_agent_scope_invalid", "agent evidence requires the exact source-pull credential")
				return
			}
			rev := m.Revisions[v.ThreatModelVersion-1]
			if rev.Source.Kind != "pull_request" || actor.RepositoryID != v.RepositoryID || actor.PullRequestID != rev.Source.ResourceID || actor.GitWriteBranch == "" {
				writeAPIError(w, 403, "security_scenario_agent_scope_invalid", "agent evidence requires the exact source-pull credential")
				return
			}
		}
		m, e := models.Get(r.PathValue("id"), v.ThreatModelID, threatmodels.CurrentSource{})
		if e != nil || !contains(m.Revisions[v.ThreatModelVersion-1].OwnerIDs, actor.UserID) {
			writeAPIError(w, 403, "security_review_owner_required", "review requires a named threat-model owner")
			return
		}
		out, e := scenarios.Review(r.PathValue("id"), v.ID, actor.UserID, in.Decision, in.Note)
		writeSecurityScenario(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/security-scenarios/{scenario_id}/attempts", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authorizeThreatModelContributor(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		var in securityAttemptInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "exact execution evidence is required")
			return
		}
		v, e := scenarios.Get(r.PathValue("id"), r.PathValue("scenario_id"))
		if e != nil {
			writeSecurityScenario(w, v, e, 200)
			return
		}
		a := in.Attempt
		if a.ExecutionKind == "workspace" {
			ws, e := workspacesStore.Get(a.WorkspaceID)
			if e != nil || ws.RepositoryID != v.RepositoryID || ws.CommitID != v.CommitID {
				writeAPIError(w, 422, "security_workspace_invalid", "attempts require an isolated exact-revision repository workspace")
				return
			}
			selected := map[string]bool{}
			for _, id := range in.OutcomeIDs {
				selected[id] = true
			}
			a.Commands = []securityscenarios.Command{}
			for _, o := range ws.Commands {
				if selected[o.ID] {
					if !securityCommandDigestMatches(o.CommandSHA256, v.Command) {
						writeAPIError(w, 422, "security_outcome_command_invalid", "every selected outcome must execute the reviewed scenario command")
						return
					}
					a.Commands = append(a.Commands, securityscenarios.Command{OutcomeID: o.ID, SHA256: o.CommandSHA256, Directory: o.Directory, ExitCode: o.ExitCode, Log: o.Output, StartedAt: o.StartedAt, CompletedAt: o.CompletedAt})
				}
			}
			if len(a.Commands) != len(selected) {
				writeAPIError(w, 422, "security_outcome_invalid", "every selected command must be a retained workspace outcome")
				return
			}
		}
		if a.ExecutionKind == "preview" {
			p, e := previewStore.Get(v.RepositoryID, in.PullRequestID, a.PreviewID)
			if e != nil || p.Revision != v.CommitID || p.State != "ready" || p.Stale {
				writeAPIError(w, 422, "security_preview_invalid", "attempts require a successful current exact-revision preview")
				return
			}
			run, runErr := runs.Get(v.RepositoryID, in.PullRequestID, p.BuildRunID)
			if runErr != nil || run.CommitID != v.CommitID || run.State != "succeeded" || run.ExitCode == nil || run.StartedAt == nil || run.CompletedAt == nil || !securityCommandMatches(run.Definition.Command, v.Command) {
				writeAPIError(w, 422, "security_preview_run_invalid", "preview evidence requires its successful retained candidate build run")
				return
			}
			a.Commands = []securityscenarios.Command{{OutcomeID: run.ID, SHA256: securityDigest(run.Definition.Command), Directory: run.Definition.WorkingDirectory, ExitCode: *run.ExitCode, StartedAt: *run.StartedAt, CompletedAt: *run.CompletedAt}}
		}
		encoded, _ := json.Marshal(a)
		if reusableSecret.Match(encoded) {
			writeAPIError(w, 400, "security_evidence_sensitive", "commands, logs, traces, and artifacts must be sanitized")
			return
		}
		identity := actor.UserID
		if actor.AgentID != "" {
			identity = actor.AgentID
		}
		out, e := scenarios.AddAttempt(v.RepositoryID, v.ID, identity, a)
		writeSecurityScenario(w, out, e, 201)
	})
}

func retainedThreatModelRevision(model threatmodels.Model, version int) (threatmodels.Revision, bool) {
	if version < 1 || version > len(model.Revisions) || model.Revisions[version-1].Version != version {
		return threatmodels.Revision{}, false
	}
	return model.Revisions[version-1], true
}

func scenarioGraph(r threatmodels.Revision, s securityscenarios.Scenario) bool {
	var path *threatmodels.AbusePath
	for i := range r.AbusePaths {
		if r.AbusePaths[i].ID == s.AbusePathID {
			path = &r.AbusePaths[i]
		}
	}
	if path == nil {
		return false
	}
	for _, id := range s.MitigationIDs {
		if !contains(path.MitigationIDs, id) {
			return false
		}
	}
	for _, id := range s.DependencyIDs {
		if !contains(path.DependencyIDs, id) {
			return false
		}
	}
	return true
}
func unsafeSecurityText(s securityscenarios.Scenario) bool {
	b, _ := json.Marshal(s)
	x := strings.ToLower(string(b))
	for _, q := range []string{"production data", "production environment", "real user", "hidden test", "delete production", "drop database", "unbounded network"} {
		if strings.Contains(x, q) {
			return true
		}
	}
	return false
}
func securityDigest(v string) string { x := sha256.Sum256([]byte(v)); return hex.EncodeToString(x[:]) }
func securityCommandDigestMatches(retainedDigest, reviewedCommand string) bool {
	return retainedDigest != "" && retainedDigest == securityDigest(reviewedCommand)
}
func securityCommandMatches(retainedCommand, reviewedCommand string) bool {
	return retainedCommand != "" && retainedCommand == reviewedCommand
}
func writeSecurityScenario(w http.ResponseWriter, v securityscenarios.Scenario, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, securityscenarios.ErrNotFound):
		writeAPIError(w, 404, "security_scenario_not_found", "security scenario not found")
	case errors.Is(e, securityscenarios.ErrConflict):
		writeAPIError(w, 409, "security_scenario_conflict", "the scenario was already reviewed")
	case errors.Is(e, securityscenarios.ErrInvalid):
		writeAPIError(w, 400, "invalid_security_scenario", "scenario or attempt evidence is invalid")
	default:
		log.Printf("security scenario storage: %v", e)
		writeAPIError(w, 500, "security_scenarios_unavailable", "security scenario could not be persisted")
	}
}
