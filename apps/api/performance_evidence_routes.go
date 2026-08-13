package main

import (
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/performanceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/performancegoals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerPerformanceEvidenceRoutes(mux *http.ServeMux, gitStore *storage.Store, catalog *repositories.Store, credentials *auth.Store, goals *performancegoals.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, pulls *pullrequests.Store, trials *performanceevidence.Store) {
	mux.HandleFunc("POST /repositories/{id}/performance-merge-policies", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "owner_required", "only repository owners can govern merge performance")
			return
		}
		var in performanceevidence.MergePolicy
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a performance merge policy is required")
			return
		}
		in.RepositoryID, in.CreatedBy = r.PathValue("id"), actor.UserID
		for _, goalID := range in.GoalIDs {
			goal, e := goals.Get(goalID)
			if e != nil || goal.RepositoryID != in.RepositoryID {
				writeAPIError(w, 422, "performance_policy_invalid", "every goal must belong to this repository")
				return
			}
		}
		created, e := trials.PutMergePolicy(in)
		if e != nil {
			writeAPIError(w, 422, "performance_policy_invalid", "branch, selectors, goals, regression threshold, and confidence are required")
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/performance-merge-policies", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		items, e := trials.ListMergePolicies(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "performance_evidence_unavailable", "performance policy could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"policies": items})
	})
	mux.HandleFunc("POST /repositories/{id}/performance-release-observations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in performanceevidence.ReleaseObservation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "release performance evidence is required")
			return
		}
		in.RepositoryID, in.CreatedBy = r.PathValue("id"), actor.UserID
		if releaseStore == nil || deploymentStore == nil {
			writeAPIError(w, 503, "performance_release_unavailable", "release and deployment evidence are unavailable")
			return
		}
		release, e := releaseStore.Get(in.RepositoryID, in.ReleaseID)
		if e != nil || release.CommitID != in.CommitID {
			writeAPIError(w, 422, "performance_release_invalid", "release must attest the evaluated candidate")
			return
		}
		deployment, e := deploymentStore.GetPromotion(in.RepositoryID, in.DeploymentID)
		if e != nil || deployment.ReleaseID != in.ReleaseID || deployment.CommitID != in.CommitID {
			writeAPIError(w, 422, "performance_release_invalid", "deployment must carry the same attested release candidate")
			return
		}
		created, e := trials.CreateReleaseObservation(in)
		if e != nil {
			writeAPIError(w, 422, "performance_release_invalid", "observed evidence must match the attested candidate goal and revision")
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("POST /repositories/{id}/performance-trials", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in performanceevidence.Trial
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete sanitized performance trial is required")
			return
		}
		in.RepositoryID, in.CreatedBy = r.PathValue("id"), actor.UserID
		if in.GoalID != "" {
			goal, e := goals.Get(in.GoalID)
			if e != nil || goal.RepositoryID != in.RepositoryID {
				writeAPIError(w, 422, "performance_context_invalid", "the goal is not available in this repository")
				return
			}
		}
		if in.Source.Kind == "release" {
			if releaseStore == nil {
				writeAPIError(w, 503, "performance_context_unavailable", "release evidence is unavailable")
				return
			}
			rel, e := releaseStore.Get(in.RepositoryID, in.Source.ReleaseID)
			if e != nil || rel.CommitID != in.Source.Revision {
				writeAPIError(w, 422, "performance_source_invalid", "the release does not attest the exact revision")
				return
			}
		}
		repo, e := gitStore.Open(in.RepositoryID)
		if e != nil || exec.Command("git", "--git-dir="+repo.Path(), "cat-file", "-e", in.Source.Revision+"^{commit}").Run() != nil {
			writeAPIError(w, 422, "performance_source_invalid", "the exact source revision is unavailable")
			return
		}
		var created performanceevidence.Trial
		e = catalog.WithCurrentParticipant(actor.UserID, in.RepositoryID, func() error { var x error; created, x = trials.Create(in); return x })
		if errors.Is(e, performanceevidence.ErrInvalid) {
			writeAPIError(w, 422, "performance_trial_invalid", "trial evidence must be bounded, complete, sanitized, and internally consistent")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "performance_evidence_unavailable", "performance evidence could not be persisted")
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/performance-trials", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		items, e := trials.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "performance_evidence_unavailable", "performance evidence could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"trials": items})
	})
	mux.HandleFunc("GET /repositories/{id}/performance-trials/{trial_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := trials.Get(r.PathValue("trial_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "performance_trial_not_found", "performance trial not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /repositories/{id}/performance-trials/{trial_id}/compare/{baseline_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		current, e1 := trials.Get(r.PathValue("trial_id"))
		baseline, e2 := trials.Get(r.PathValue("baseline_id"))
		if e1 != nil || e2 != nil || current.RepositoryID != r.PathValue("id") || baseline.RepositoryID != current.RepositoryID {
			writeAPIError(w, 404, "performance_trial_not_found", "performance trial not found")
			return
		}
		writeJSON(w, 200, map[string]any{"baseline": baseline, "current": current, "comparisons": trials.Compare(baseline, current)})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/performance-evaluations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		pull, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		if pull.Status != pullrequests.Open {
			writeAPIError(w, 409, "performance_evaluation_stale", "performance evaluation requires the open pull's current revision")
			return
		}
		var in performanceevidence.Evaluation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete performance evaluation is required")
			return
		}
		in.RepositoryID, in.PullRequestID, in.Revision, in.CreatedBy = pull.RepositoryID, pull.ID, pull.SourceCommitID, actor.UserID
		if goal, err := goals.Get(in.GoalID); err != nil || goal.RepositoryID != pull.RepositoryID {
			writeAPIError(w, 422, "performance_evaluation_invalid", "the goal is unavailable in this repository")
			return
		}
		created, e := trials.CreateEvaluation(in)
		if errors.Is(e, performanceevidence.ErrInvalid) {
			writeAPIError(w, 422, "performance_evaluation_invalid", "use a supported diagnosis, valid baseline, exact candidate trial, correctness checks, commands, scenarios, and risks")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "performance_evidence_unavailable", "evaluation could not be persisted")
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/performance-evaluations", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		pull, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		items, e := trials.ListEvaluations(pull.RepositoryID, pull.ID, pull.SourceCommitID)
		if e != nil {
			writeAPIError(w, 500, "performance_evidence_unavailable", "evaluations could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"evaluations": items})
	})
	mux.HandleFunc("GET /repositories/{id}/performance-investigations", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		items, e := trials.ListInvestigations(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "performance_diagnosis_unavailable", "investigations could not be read")
			return
		}
		for i := range items {
			items[i] = trials.ProjectStaleness(items[i])
		}
		writeJSON(w, 200, map[string]any{"investigations": items})
	})
	mux.HandleFunc("POST /repositories/{id}/performance-investigations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in performanceevidence.Investigation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded investigation is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		in.CreatedBy = actor.UserID
		repo, openErr := gitStore.Open(in.RepositoryID)
		if openErr != nil {
			writeAPIError(w, 422, "performance_investigation_invalid", "the repository evidence could not be resolved")
			return
		}
		for i := range in.References {
			ref := &in.References[i]
			switch ref.Kind {
			case "commit":
				ref.ID = ref.Revision
			case "release":
				// The release store owns this identity; retain its canonical ID.
			case "symbol":
				ref.ID = ref.Revision + ":" + ref.Path + "#" + ref.Symbol
			case "dependency", "runtime_path":
				ref.ID = ref.Revision + ":" + ref.Path
			}
		}
		resolveReference := func(ref performanceevidence.Reference) bool {
			if ref.Revision != "" {
				visibleRevision, err := resolveRevision(repo, ref.Revision)
				if err != nil || string(visibleRevision) != ref.Revision {
					return false
				}
			}
			switch ref.Kind {
			case "commit":
				return ref.ID == ref.Revision && ref.Revision != ""
			case "symbol", "dependency", "runtime_path":
				path := strings.TrimSpace(ref.Path)
				if ref.Revision == "" || path == "" || strings.HasPrefix(path, "/") || strings.Contains("/"+path+"/", "/../") || (ref.Kind == "symbol" && strings.TrimSpace(ref.Symbol) == "") {
					return false
				}
				return exec.Command("git", "--git-dir="+repo.Path(), "cat-file", "-e", ref.Revision+":"+path).Run() == nil
			case "release":
				if releaseStore == nil || ref.Revision == "" {
					return false
				}
				release, err := releaseStore.Get(in.RepositoryID, ref.ID)
				return err == nil && release.CommitID == ref.Revision
			default:
				return false
			}
		}
		created, e := trials.CreateInvestigation(in, resolveReference)
		if errors.Is(e, performanceevidence.ErrInvalid) {
			writeAPIError(w, 422, "performance_investigation_invalid", "select repository-visible trials and revision-aware references")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "performance_diagnosis_unavailable", "investigation could not be persisted")
			return
		}
		writeJSON(w, 201, created)
	})
	readInvestigation := func(w http.ResponseWriter, r *http.Request) (performanceevidence.Investigation, bool) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return performanceevidence.Investigation{}, false
		}
		v, e := trials.GetInvestigation(r.PathValue("investigation_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "performance_investigation_not_found", "investigation not found")
			return v, false
		}
		return trials.ProjectStaleness(v), true
	}
	mux.HandleFunc("GET /repositories/{id}/performance-investigations/{investigation_id}", func(w http.ResponseWriter, r *http.Request) {
		v, ok := readInvestigation(w, r)
		if ok {
			writeJSON(w, 200, v)
		}
	})
	mux.HandleFunc("POST /repositories/{id}/performance-investigations/{investigation_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		v, e := trials.GetInvestigation(r.PathValue("investigation_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "performance_investigation_not_found", "investigation not found")
			return
		}
		var in performanceevidence.Finding
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a cited finding is required")
			return
		}
		updated, e := trials.AddFinding(v.ID, actor.UserID, in)
		if errors.Is(e, performanceevidence.ErrInvalid) {
			writeAPIError(w, 422, "performance_finding_invalid", "findings require selected citations, confidence, and bounded flame stacks")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "performance_diagnosis_unavailable", "finding could not be persisted")
			return
		}
		writeJSON(w, 201, trials.ProjectStaleness(updated))
	})
	respond := func(confirm bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in struct {
				Body string `json:"body"`
			}
			if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Body) == "" {
				writeAPIError(w, 422, "invalid_response", "a response is required")
				return
			}
			investigation, getErr := trials.GetInvestigation(r.PathValue("investigation_id"))
			if getErr != nil || investigation.RepositoryID != r.PathValue("id") {
				writeAPIError(w, 404, "performance_investigation_not_found", "investigation not found")
				return
			}
			updated, e := trials.Respond(r.PathValue("investigation_id"), r.PathValue("finding_id"), actor.UserID, in.Body, confirm)
			if errors.Is(e, performanceevidence.ErrNotFound) {
				writeAPIError(w, 404, "performance_finding_not_found", "finding not found")
				return
			}
			if e != nil {
				writeAPIError(w, 422, "invalid_response", "response could not be retained")
				return
			}
			writeJSON(w, 201, trials.ProjectStaleness(updated))
		}
	}
	mux.HandleFunc("POST /repositories/{id}/performance-investigations/{investigation_id}/findings/{finding_id}/challenges", respond(false))
	mux.HandleFunc("POST /repositories/{id}/performance-investigations/{investigation_id}/findings/{finding_id}/confirmations", respond(true))
	mux.HandleFunc("POST /repositories/{id}/performance-investigations/{investigation_id}/agent-access", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpiresIn int `json:"expires_in"`
		}
		if decodeJSON(r, &in) != nil || in.ExpiresIn < 300 || in.ExpiresIn > 86400 {
			writeAPIError(w, 422, "invalid_investigation_access", "expiry must be 5 minutes to 24 hours")
			return
		}
		v, e := trials.GetInvestigation(r.PathValue("investigation_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "performance_investigation_not_found", "investigation not found")
			return
		}
		issued, e := credentials.Issue(actor.UserID, auth.API, "Performance investigation", []string{"performance:investigate"}, time.Duration(in.ExpiresIn)*time.Second)
		if e != nil {
			writeAPIError(w, 500, "investigation_failed", "credential could not be issued")
			return
		}
		v, e = trials.BindCredential(v.ID, issued.ID)
		if e != nil {
			_, _ = credentials.Revoke(actor.UserID, issued.ID)
			writeAPIError(w, 500, "investigation_failed", "credential binding could not be persisted")
			return
		}
		writeJSON(w, 201, map[string]any{"credential": issued, "investigation": trials.ProjectStaleness(v)})
	})
	mux.HandleFunc("GET /performance-investigations/{investigation_id}", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "performance:investigate", false)
		if !ok {
			return
		}
		v, e := trials.GetInvestigation(r.PathValue("investigation_id"))
		if e != nil || v.CredentialID != credential.ID {
			writeAPIError(w, 404, "performance_investigation_not_found", "investigation not found")
			return
		}
		if e = catalog.WithCurrentParticipant(credential.UserID, v.RepositoryID, func() error { return nil }); e != nil {
			writeAPIError(w, 403, "investigation_access_changed", "the credential owner lost repository access")
			return
		}
		selected := []performanceevidence.Trial{}
		for _, id := range v.TrialIDs {
			if t, e := trials.Get(id); e == nil {
				selected = append(selected, t)
			}
		}
		writeJSON(w, 200, map[string]any{"investigation": trials.ProjectStaleness(v), "trials": selected})
	})
	mux.HandleFunc("POST /performance-investigations/{investigation_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "performance:investigate", false)
		if !ok {
			return
		}
		v, e := trials.GetInvestigation(r.PathValue("investigation_id"))
		if e != nil || v.CredentialID != credential.ID {
			writeAPIError(w, 404, "performance_investigation_not_found", "investigation not found")
			return
		}
		if e = catalog.WithCurrentParticipant(credential.UserID, v.RepositoryID, func() error { return nil }); e != nil {
			writeAPIError(w, 403, "investigation_access_changed", "the credential owner lost repository access")
			return
		}
		var in performanceevidence.Finding
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a cited finding is required")
			return
		}
		updated, e := trials.AddFinding(v.ID, "agent:"+credential.ID, in)
		if errors.Is(e, performanceevidence.ErrInvalid) {
			writeAPIError(w, 422, "performance_finding_invalid", "findings require only selected citations, confidence, and bounded flame stacks")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "performance_diagnosis_unavailable", "finding could not be persisted")
			return
		}
		writeJSON(w, 201, trials.ProjectStaleness(updated))
	})
}
