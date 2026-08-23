package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/debugworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/regressioninvestigations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
)

func registerRegressionInvestigationRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, investigations *regressioninvestigations.Store, issueStore *issues.Store, supportStore *supportthreads.Store, checkStore *checkruns.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, debugStore *debugworkspaces.Store) {
	actor := func(c auth.Credential) string {
		if c.AgentID != "" {
			return c.AgentID
		}
		return c.UserID
	}
	project := func(v regressioninvestigations.Investigation) regressioninvestigations.Investigation { // Staleness is live and never rewrites retained evidence.
		if v.Evidence == nil {
			v.Evidence = []regressioninvestigations.Evidence{}
		}
		if v.Diagnostics == nil {
			v.Diagnostics = []string{}
		}
		r, err := git.Open(v.RepositoryID)
		if err != nil {
			v.Diagnostics = append(v.Diagnostics, "repository history is unavailable")
			v.Comparable = false
			return v
		}
		if exec.Command("git", "--git-dir="+r.Path(), "cat-file", "-e", v.KnownGood.Revision+"^{commit}").Run() != nil {
			v.Diagnostics = append(v.Diagnostics, "known-good revision is missing")
			v.Comparable = false
		}
		if exec.Command("git", "--git-dir="+r.Path(), "cat-file", "-e", v.KnownBad.Revision+"^{commit}").Run() != nil {
			v.Diagnostics = append(v.Diagnostics, "known-bad revision is missing")
			v.Comparable = false
		}
		for i := range v.Evidence {
			ev := &v.Evidence[i]
			current := false
			permitted := ev.Visibility == "repository" || ev.Visibility == "participants"
			if permitted && ev.Kind == "commit" {
				current = ev.ResourceID == v.RepositoryID && ev.Revision != "" && exec.Command("git", "--git-dir="+r.Path(), "cat-file", "-e", ev.Revision+"^{commit}").Run() == nil
			} else if permitted {
				current = validRegressionSource(regressioninvestigations.Reference{Kind: ev.Kind, ResourceID: ev.ResourceID, Revision: ev.Revision, Label: ev.Label}, v.RepositoryID, issueStore, supportStore, checkStore, releaseStore, deploymentStore, debugStore)
			}
			*ev = projectRegressionEvidence(*ev, current)
		}
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/regression-investigations", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, e := investigations.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "regression_investigations_unavailable", "regression investigations could not be read")
			return
		}
		for i := range values {
			values[i] = project(values[i])
		}
		writeJSON(w, 200, map[string]any{"regression_investigations": values})
	})
	mux.HandleFunc("GET /repositories/{id}/regression-investigations/{investigation_id}", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := investigations.Get(r.PathValue("id"), r.PathValue("investigation_id"))
		if e != nil {
			writeAPIError(w, 404, "regression_investigation_not_found", "regression investigation not found")
			return
		}
		writeJSON(w, 200, project(v))
	})
	mux.HandleFunc("POST /repositories/{id}/regression-investigations", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in regressioninvestigations.Investigation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a regression search boundary is required")
			return
		}
		repoID := r.PathValue("id")
		in.RepositoryID = repoID
		in.Diagnostics = []string{}
		in.Comparable = false
		reconciled, found, reconcileErr := investigations.Reconcile(in, actor(c))
		if reconcileErr != nil {
			writeRegressionInvestigation(w, project(reconciled), reconcileErr, 201)
			return
		}
		if found {
			writeJSON(w, 201, project(reconciled))
			return
		}
		repository, catalogErr := catalog.GetByID(repoID)
		if catalogErr != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		for _, ownerID := range in.OwnerIDs {
			participant, participantErr := catalog.HasCollaborator(ownerID, repoID)
			if participantErr != nil || (ownerID != repository.OwnerID && !participant) {
				writeAPIError(w, 422, "regression_owner_invalid", "every owner must be a current repository participant")
				return
			}
		}
		if !validRegressionSource(in.Source, repoID, issueStore, supportStore, checkStore, releaseStore, deploymentStore, debugStore) {
			writeAPIError(w, 422, "regression_source_invalid", "source must resolve to an issue, support thread, failed check, release, deployment, or reproduction in this repository")
			return
		}
		gr, e := git.Open(repoID)
		if e != nil {
			writeAPIError(w, 422, "regression_history_unavailable", "repository history is unavailable")
			return
		}
		resolve := func(b *regressioninvestigations.Boundary) bool {
			if b.Kind == "release" {
				x, e := releaseStore.Get(repoID, b.ResourceID)
				if e != nil || x.CommitID != b.Revision {
					return false
				}
			}
			return exec.Command("git", "--git-dir="+gr.Path(), "cat-file", "-e", b.Revision+"^{commit}").Run() == nil
		}
		if !resolve(&in.KnownGood) || !resolve(&in.KnownBad) {
			writeAPIError(w, 422, "regression_boundary_missing", "known-good and known-bad revisions must resolve in this repository")
			return
		}
		if exec.Command("git", "--git-dir="+gr.Path(), "merge-base", "--is-ancestor", in.KnownGood.Revision, in.KnownBad.Revision).Run() != nil {
			writeAPIError(w, 422, "regression_boundary_incomparable", "known-good must be an ancestor of known-bad")
			return
		}
		in.Comparable = true
		for i := range in.Evidence {
			ev := &in.Evidence[i]
			ev.Available, ev.Stale, ev.Diagnostic = false, false, ""
			if ev.Visibility != "repository" && ev.Visibility != "participants" {
				ev.Diagnostic = "evidence visibility is not permitted"
			} else if ev.Kind == "commit" {
				ev.Available = ev.ResourceID == repoID && ev.Revision != "" && exec.Command("git", "--git-dir="+gr.Path(), "cat-file", "-e", ev.Revision+"^{commit}").Run() == nil
			} else {
				ev.Available = validRegressionSource(regressioninvestigations.Reference{Kind: ev.Kind, ResourceID: ev.ResourceID, Revision: ev.Revision, Label: ev.Label}, repoID, issueStore, supportStore, checkStore, releaseStore, deploymentStore, debugStore)
			}
			if !ev.Available && ev.Diagnostic == "" {
				ev.Diagnostic = "evidence does not resolve in this repository"
			}
			if !ev.Available {
				in.Diagnostics = append(in.Diagnostics, ev.Label+": "+ev.Diagnostic)
			}
		}
		out, e := investigations.Create(in, actor(c))
		writeRegressionInvestigation(w, project(out), e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/regression-investigations/{investigation_id}/events", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Kind            string `json:"kind"`
			Message         string `json:"message"`
			Value           string `json:"value"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributable event is required")
			return
		}
		out, e := investigations.Append(r.PathValue("id"), r.PathValue("investigation_id"), actor(c), in.Kind, in.Message, in.Value, in.ExpectedVersion)
		writeRegressionInvestigation(w, project(out), e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/regression-investigations/{investigation_id}/scenarios", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                               `json:"expected_version"`
			Scenario        regressioninvestigations.Scenario `json:"scenario"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded regression scenario is required")
			return
		}
		current, e := investigations.Get(r.PathValue("id"), r.PathValue("investigation_id"))
		if e != nil {
			writeRegressionInvestigation(w, current, e, 201)
			return
		}
		if strings.TrimSpace(in.Scenario.ExpectedBehavior) == "" {
			in.Scenario.ExpectedBehavior = current.ExpectedBehavior
		}
		if strings.TrimSpace(in.Scenario.RegressedBehavior) == "" {
			in.Scenario.RegressedBehavior = current.RegressedBehavior
		}
		if len(in.Scenario.AcceptanceCriteria) == 0 {
			in.Scenario.AcceptanceCriteria = append([]string{}, current.AcceptanceCriteria...)
		}
		out, e := investigations.AddScenario(current.RepositoryID, current.ID, actor(c), in.Scenario, in.ExpectedVersion)
		writeRegressionInvestigation(w, project(out), e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/regression-investigations/{investigation_id}/scenarios/{scenario_id}/attempts", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                                   `json:"expected_version"`
			RequestID       string                                `json:"request_id"`
			TargetKind      string                                `json:"target_kind"`
			TargetID        string                                `json:"target_id"`
			Revision        string                                `json:"revision"`
			Dependencies    []regressioninvestigations.Dependency `json:"dependencies"`
			Repeats         int                                   `json:"repeats"`
		}
		if decodeJSON(r, &in) != nil || in.ExpectedVersion < 1 {
			writeAPIError(w, 400, "invalid_request", "an exact historical target is required")
			return
		}
		current, e := investigations.Get(r.PathValue("id"), r.PathValue("investigation_id"))
		if e != nil {
			writeRegressionInvestigation(w, current, e, 201)
			return
		}
		var scenario *regressioninvestigations.Scenario
		for i := range current.Scenarios {
			if current.Scenarios[i].ID == r.PathValue("scenario_id") {
				scenario = &current.Scenarios[i]
				break
			}
		}
		if scenario == nil {
			writeAPIError(w, 404, "regression_scenario_not_found", "regression scenario not found")
			return
		}
		attempt := regressioninvestigations.Attempt{RequestID: in.RequestID, ScenarioID: scenario.ID, TargetKind: in.TargetKind, TargetID: strings.TrimSpace(in.TargetID), Revision: strings.ToLower(strings.TrimSpace(in.Revision)), Dependencies: in.Dependencies, Environment: scenario.Environment, Inputs: scenario.Inputs, Repeats: in.Repeats}
		if attempt.Dependencies == nil {
			attempt.Dependencies = []regressioninvestigations.Dependency{}
		}
		if in.TargetKind == "release" {
			release, releaseErr := releaseStore.Get(current.RepositoryID, attempt.TargetID)
			if releaseErr == nil && (attempt.Revision == "" || attempt.Revision == release.CommitID) {
				attempt.Revision = release.CommitID
			} else {
				attempt.Classification, attempt.Diagnostic = "untestable_revision", "attested release does not resolve to the selected revision"
			}
		} else if in.TargetKind != "commit" {
			attempt.Classification, attempt.Diagnostic = "untestable_revision", "target must be a repository commit or attested release"
		}
		gr, gitErr := git.Open(current.RepositoryID)
		if attempt.Classification == "" && (gitErr != nil || len(attempt.Revision) != 40 || exec.Command("git", "--git-dir="+gr.Path(), "cat-file", "-e", attempt.Revision+"^{commit}").Run() != nil) {
			attempt.Classification, attempt.Diagnostic = "untestable_revision", "selected revision is unavailable in repository history"
		}
		dependencyArchives := map[string]string{}
		if attempt.Classification == "" {
			for _, variant := range scenario.EnvironmentVariants {
				if variant.Revision == attempt.Revision {
					attempt.Environment = variant.Environment
					break
				}
			}
			seenDependencies := map[string]bool{}
			dependencyBytes := 0
			if len(attempt.Dependencies) > 8 {
				attempt.Classification, attempt.Diagnostic = "incompatible_setup", "dependency combination exceeds the eight-snapshot bound"
			}
			for i := range attempt.Dependencies {
				if attempt.Classification != "" {
					break
				}
				dependency := &attempt.Dependencies[i]
				dependency.Name, dependency.RepositoryID, dependency.Revision = strings.TrimSpace(dependency.Name), strings.TrimSpace(dependency.RepositoryID), strings.ToLower(strings.TrimSpace(dependency.Revision))
				dependency.Path = "dependencies/" + dependency.Name
				if !validRegressionDependencyName(dependency.Name) || seenDependencies[dependency.Name] || len(dependency.Revision) != 40 {
					attempt.Classification, attempt.Diagnostic = "missing_dependencies", "dependency combination contains a missing revision"
					break
				}
				seenDependencies[dependency.Name] = true
				_, _, permitted := authorizeRepositoryRead(w, r, catalog, credentials, dependency.RepositoryID)
				if !permitted {
					return
				}
				dependencyGit, openErr := git.Open(dependency.RepositoryID)
				if openErr != nil || exec.Command("git", "--git-dir="+dependencyGit.Path(), "cat-file", "-e", dependency.Revision+"^{commit}").Run() != nil {
					attempt.Classification, attempt.Diagnostic = "missing_dependencies", "dependency combination contains a missing revision"
					break
				}
				archive, archiveErr := exec.Command("git", "--git-dir="+dependencyGit.Path(), "archive", dependency.Revision).Output()
				dependencyBytes += len(archive)
				if archiveErr != nil || len(archive) > 32*1024*1024 || dependencyBytes > 64*1024*1024 {
					attempt.Classification, attempt.Diagnostic = "incompatible_setup", "dependency snapshot could not be bounded and materialized"
					break
				}
				sum := sha256.Sum256(archive)
				dependency.ArchiveSHA256 = hex.EncodeToString(sum[:])
				dependencyArchives[dependency.Name] = base64.StdEncoding.EncodeToString(archive)
			}
		}
		for _, input := range scenario.Inputs {
			if input.Kind == "unsafe" {
				attempt.Classification, attempt.Diagnostic = "unsafe_fixture", "scenario fixture is not synthetic or privacy-preserving"
				break
			}
		}
		if attempt.Repeats == 0 {
			attempt.Repeats = 1
		}
		if attempt.Repeats < 1 || attempt.Repeats > 5 {
			writeAPIError(w, 422, "regression_repeats_invalid", "repeat count must be between one and five")
			return
		}
		reservedInvestigation, reserved, existed, reserveErr := investigations.ReserveAttempt(current.RepositoryID, current.ID, actor(c), attempt, in.ExpectedVersion)
		if reserveErr != nil {
			writeRegressionInvestigation(w, project(reservedInvestigation), reserveErr, 201)
			return
		}
		if existed && reserved.State == "completed" {
			writeJSON(w, 201, project(reservedInvestigation))
			return
		}
		attempt = reserved
		if attempt.Classification == "" {
			command := attempt.Environment.Command
			if strings.TrimSpace(attempt.Environment.SetupCommand) != "" {
				command = attempt.Environment.SetupCommand + " && " + command
			}
			attempt.Command = command
			passed, failed := false, false
			for n := 0; n < attempt.Repeats; n++ {
				inputs := []checkruns.InputFile{}
				for _, input := range scenario.Inputs {
					raw, decodeErr := base64.StdEncoding.DecodeString(input.Data)
					sum := sha256.Sum256(raw)
					if decodeErr != nil || hex.EncodeToString(sum[:]) != input.SHA256 {
						attempt.Classification, attempt.Diagnostic = "unsafe_fixture", "fixture content does not match its retained digest"
						break
					}
					inputs = append(inputs, checkruns.InputFile{Name: input.Name, SHA256: input.SHA256, Data: input.Data})
				}
				if attempt.Classification != "" {
					break
				}
				for _, dependency := range attempt.Dependencies {
					inputs = append(inputs, checkruns.InputFile{Name: dependency.Path, SHA256: dependency.ArchiveSHA256, Data: dependencyArchives[dependency.Name], Archive: true})
				}
				environment := map[string]string{}
				for k, v := range attempt.Environment.Environment {
					environment[k] = v
				}
				for _, dependency := range attempt.Dependencies {
					environment["VIVARIUM_DEPENDENCY_"+strings.ToUpper(strings.ReplaceAll(dependency.Name, "-", "_"))+"_REVISION"] = dependency.Revision
					environment["VIVARIUM_DEPENDENCY_"+strings.ToUpper(strings.ReplaceAll(dependency.Name, "-", "_"))+"_PATH"] = "/workspace/" + dependency.Path
				}
				definition := checkruns.Definition{Name: "regression " + attempt.ID + " " + string(rune('a'+n)), Image: attempt.Environment.Image, Command: command, WorkingDirectory: attempt.Environment.WorkingDirectory, Environment: environment, TimeoutSeconds: attempt.Environment.TimeoutSeconds, CPUs: attempt.Environment.CPUs, MemoryMB: attempt.Environment.MemoryMB, StorageMB: attempt.Environment.StorageMB, Inputs: inputs}
				existingRuns, _ := checkStore.List(current.RepositoryID, "regression-"+current.ID)
				var runs []checkruns.Run
				for _, candidate := range existingRuns {
					if candidate.CommitID == attempt.Revision && candidate.Definition.Name == definition.Name {
						runs = []checkruns.Run{candidate}
						break
					}
				}
				var createErr error
				if len(runs) == 0 {
					runs, createErr = checkStore.CreateRequested(current.RepositoryID, "regression-"+current.ID, attempt.Revision, []checkruns.Definition{definition}, actor(c))
				}
				if createErr != nil || len(runs) != 1 {
					attempt.Classification, attempt.Diagnostic = "incompatible_setup", "isolated environment could not be created"
					break
				}
				if runs[0].State == "queued" || runs[0].State == "running" || runs[0].State == "cleanup_pending" {
					checkStore.Execute(runs[0], gr.Path())
				}
				run, getErr := checkStore.Get(current.RepositoryID, "regression-"+current.ID, runs[0].ID)
				if getErr != nil {
					attempt.Classification, attempt.Diagnostic = "incompatible_setup", "isolated result could not be retained"
					break
				}
				events, _ := checkStore.Events(current.RepositoryID, "regression-"+current.ID, run.ID, 0)
				logs := strings.Builder{}
				output := strings.Builder{}
				for _, event := range events {
					if event.Kind == "log" {
						logs.WriteString(event.Message)
						if event.Stream == "stdout" {
							output.WriteString(event.Message)
						}
					}
				}
				if regressionRunActive(run.State) {
					latest, _ := investigations.Get(current.RepositoryID, current.ID)
					writeJSON(w, http.StatusAccepted, project(latest))
					return
				}
				artifacts := []regressioninvestigations.AttemptArtifact{}
				for _, a := range run.Artifacts {
					artifacts = append(artifacts, regressioninvestigations.AttemptArtifact{Path: a.Path, SHA256: a.SHA256, Size: a.Size, ContentType: a.ContentType})
				}
				duration := int64(0)
				if run.StartedAt != nil && run.CompletedAt != nil {
					duration = run.CompletedAt.Sub(*run.StartedAt).Milliseconds()
				}
				attempt.Runs = append(attempt.Runs, regressioninvestigations.AttemptRun{RunID: run.ID, State: run.State, ExitCode: run.ExitCode, Failure: run.Failure, FailureKind: run.FailureKind, Output: output.String(), Logs: logs.String(), Artifacts: artifacts, DurationMS: duration})
				attempt.CostComputeSeconds += float64(duration) / 1000 * attempt.Environment.CPUs
				if run.State == "succeeded" {
					passed = true
				} else {
					failed = true
					if regressionSetupFailure(run.FailureKind) {
						attempt.Classification, attempt.Diagnostic = "incompatible_setup", strings.TrimSpace(run.Failure+"\n"+logs.String())
						break
					}
				}
			}
			if attempt.Classification == "" {
				if passed && failed {
					attempt.Classification = "flaky"
				} else if passed {
					attempt.Classification = "passed"
				} else {
					attempt.Classification = "failed"
				}
			}
		}
		attempt.CompletedAt = time.Now().UTC()
		out, e := investigations.FinalizeAttempt(current.RepositoryID, current.ID, actor(c), attempt.ID, attempt)
		writeRegressionInvestigation(w, project(out), e, 201)
	})
}

func validRegressionDependencyName(v string) bool {
	if len(v) < 1 || len(v) > 100 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func regressionSetupFailure(failureKind string) bool { return failureKind == "setup" }

func regressionRunActive(state string) bool {
	return state == "queued" || state == "running" || state == "cleanup_pending"
}

func projectRegressionEvidence(evidence regressioninvestigations.Evidence, current bool) regressioninvestigations.Evidence {
	if current {
		evidence.Available = true
		evidence.Stale = false
		evidence.Diagnostic = ""
		return evidence
	}
	if evidence.Available {
		evidence.Available = false
		evidence.Stale = true
		evidence.Diagnostic = "retained evidence no longer resolves to its required source state"
	}
	return evidence
}

func validRegressionSource(s regressioninvestigations.Reference, repo string, is *issues.Store, ss *supportthreads.Store, cs *checkruns.Store, rs *releases.Store, ds *deployments.Store, dws *debugworkspaces.Store) bool {
	switch s.Kind {
	case "issue":
		x, e := is.Get(repo, s.ResourceID)
		return e == nil && x.ID != ""
	case "support_thread":
		x, e := ss.Get(repo, s.ResourceID)
		return e == nil && x.ID != ""
	case "release":
		x, e := rs.Get(repo, s.ResourceID)
		return e == nil && (s.Revision == "" || s.Revision == x.CommitID)
	case "deployment":
		x, e := ds.GetPromotion(repo, s.ResourceID)
		return e == nil && x.ID != ""
	case "failed_check":
		parts := strings.Split(s.ResourceID, "/")
		if len(parts) != 2 {
			return false
		}
		x, e := cs.Get(repo, parts[0], parts[1])
		return e == nil && x.State == "failed" && (s.Revision == "" || s.Revision == x.CommitID)
	case "reproduction":
		parts := strings.Split(s.ResourceID, "/")
		if len(parts) != 2 {
			return false
		}
		x, e := dws.Get(repo, parts[0])
		if e != nil {
			return false
		}
		for _, q := range x.ReplayScenarios {
			if q.ID == parts[1] && (q.Status == "reproduced" || q.Status == "demonstrated") {
				return true
			}
		}
	}
	return false
}
func writeRegressionInvestigation(w http.ResponseWriter, v regressioninvestigations.Investigation, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, regressioninvestigations.ErrInvalid):
		writeAPIError(w, 422, "invalid_regression_investigation", "regression investigation is incomplete or invalid")
	case errors.Is(e, regressioninvestigations.ErrConflict):
		writeAPIError(w, 409, "regression_investigation_changed", "regression investigation changed; refresh and retry")
	case errors.Is(e, regressioninvestigations.ErrNotFound):
		writeAPIError(w, 404, "regression_investigation_not_found", "regression investigation not found")
	default:
		writeAPIError(w, 500, "regression_investigation_unavailable", "regression investigation could not be persisted")
	}
}
