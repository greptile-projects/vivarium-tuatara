package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/protectionplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoverycommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoveryexercises"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type recoveryExerciseInput struct {
	Name      string                   `json:"name"`
	Scenario  string                   `json:"scenario"`
	PlanID    string                   `json:"plan_id"`
	CaptureID string                   `json:"capture_id"`
	Steps     []recoveryexercises.Step `json:"steps"`
}
type recoveryFindingInput struct {
	ExpectedVersion int                       `json:"expected_version"`
	Finding         recoveryexercises.Finding `json:"finding"`
}
type recoveryTaskInput struct {
	Title             string `json:"title"`
	AssigneeType      string `json:"assignee_type"`
	AssigneeID        string `json:"assignee_id"`
	DependsOnPrevious bool   `json:"depends_on_previous"`
}
type recoveryImprovementInput struct {
	InvestigationID    string              `json:"investigation_id"`
	FindingID          string              `json:"finding_id"`
	Title              string              `json:"title"`
	Body               string              `json:"body"`
	AcceptanceCriteria []string            `json:"acceptance_criteria"`
	Tasks              []recoveryTaskInput `json:"tasks"`
}
type recoveryVerificationInput struct {
	FollowUpExerciseID string `json:"follow_up_exercise_id"`
}

func registerRecoveryExerciseRoutes(mux *http.ServeMux, git *storage.Store, environments *deployments.Store, releaseStore *releases.Store, catalog *repositories.Store, credentials *auth.Store, plans *protectionplans.Store, commitments *recoverycommitments.Store, exercises *recoveryexercises.Store, proposalStore *proposals.Store) {
	mux.HandleFunc("GET /repositories/{id}/recovery-exercises", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, e := exercises.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "recovery_exercises_unavailable", "recovery exercise evidence could not be read")
			return
		}
		for i := range values {
			refreshExercise(&values[i], git, environments, plans, commitments)
		}
		writeJSON(w, 200, map[string]any{"exercises": values})
	})
	mux.HandleFunc("POST /repositories/{id}/recovery-exercises", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in recoveryExerciseInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded recovery exercise is required")
			return
		}
		p, e := plans.Get(in.PlanID)
		if e != nil || p.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "protection_plan_not_found", "protection plan not found")
			return
		}
		if !exerciseContains(p.AccessorIDs, actor.UserID) {
			writeAPIError(w, 403, "recovery_exercise_forbidden", "only a declared recovery accessor may launch this exercise")
			return
		}
		var capture protectionplans.Capture
		found := false
		for _, c := range p.Captures {
			if c.ID == in.CaptureID {
				capture = c
				found = true
				break
			}
		}
		if !found || !capture.Recoverable {
			writeAPIError(w, 409, "recovery_capture_unavailable", "the selected protected capture is not recoverable")
			return
		}
		restored, e := plans.Restore(p.ID, capture.ID)
		if e != nil {
			writeAPIError(w, 409, "recovery_capture_unavailable", "the selected protected capture failed integrity verification")
			return
		}
		exercise := recoveryexercises.Exercise{Name: strings.TrimSpace(in.Name), Scenario: strings.TrimSpace(in.Scenario), PlanID: p.ID, PlanVersion: capture.PlanVersion, CommitmentID: p.CommitmentID, CommitmentVersion: capture.CommitmentVersion, CaptureID: capture.ID, SourceRevision: capture.SourceRevision, Steps: in.Steps}
		execute, cleanup := newExerciseExecutor(restored, p.ValidationChecks)
		defer cleanup()
		out, e := exercises.Run(r.PathValue("id"), actor.UserID, exercise, execute)
		if errors.Is(e, recoveryexercises.ErrInvalid) {
			writeAPIError(w, 400, "invalid_recovery_exercise", "use ordered restore, integrity, journey, or manual steps with declared objectives")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "recovery_exercises_unavailable", "recovery exercise evidence could not be persisted")
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("POST /repositories/{id}/recovery-exercises/{exercise_id}/investigations", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if !authenticated {
			writeAuthenticationRequired(w, false)
			return
		}
		var in recoveryexercises.Investigation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_recovery_investigation", "cited permitted evidence is required")
			return
		}
		exercise, err := exercises.Get(r.PathValue("id"), r.PathValue("exercise_id"))
		if err != nil {
			writeAPIError(w, 404, "recovery_exercise_not_found", "recovery exercise not found")
			return
		}
		if !recoveryEvidenceResolves(git, releaseStore, plans, commitments, exercise, in.Evidence) {
			writeAPIError(w, 422, "recovery_evidence_unresolved", "every citation must resolve to permitted exact repository evidence")
			return
		}
		typ := "human"
		if actor.AgentID != "" {
			typ = "agent"
		}
		out, err := exercises.OpenInvestigation(r.PathValue("id"), exercise.ID, actor.UserID, typ, in)
		writeRecoveryMutation(w, out, err)
	})
	mux.HandleFunc("POST /repositories/{id}/recovery-exercises/{exercise_id}/investigations/{investigation_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if !authenticated {
			writeAuthenticationRequired(w, false)
			return
		}
		var in recoveryFindingInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_recovery_finding", "a cited finding and uncertainty are required")
			return
		}
		typ := "human"
		if actor.AgentID != "" {
			typ = "agent"
		}
		out, err := exercises.AddFinding(r.PathValue("id"), r.PathValue("exercise_id"), r.PathValue("investigation_id"), actor.UserID, typ, in.ExpectedVersion, in.Finding)
		writeRecoveryMutation(w, out, err)
	})
	mux.HandleFunc("POST /repositories/{id}/recovery-exercises/{exercise_id}/improvements", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "recovery_improvement_owner_required", "only the repository owner may convert a recovery gap into work")
			return
		}
		var in recoveryImprovementInput
		if decodeJSON(r, &in) != nil || proposalStore == nil || len(in.Tasks) == 0 || len(in.Tasks) > 20 {
			writeAPIError(w, 400, "invalid_recovery_improvement", "ordered owned tasks and acceptance criteria are required")
			return
		}
		exercise, err := exercises.Get(r.PathValue("id"), r.PathValue("exercise_id"))
		if err != nil {
			writeAPIError(w, 404, "recovery_exercise_not_found", "recovery exercise not found")
			return
		}
		findingExists := false
		for _, investigation := range exercise.Investigations {
			if investigation.ID == in.InvestigationID {
				for _, finding := range investigation.Findings {
					findingExists = findingExists || finding.ID == in.FindingID
				}
			}
		}
		if !findingExists || len(in.AcceptanceCriteria) == 0 || len(in.AcceptanceCriteria) > 20 {
			writeAPIError(w, 422, "recovery_improvement_invalid", "the cited finding and acceptance criteria must remain valid")
			return
		}
		for _, criterion := range in.AcceptanceCriteria {
			if !recoveryEvidenceText(criterion) {
				writeAPIError(w, 422, "recovery_improvement_invalid", "acceptance criteria must be bounded and non-secret")
				return
			}
		}
		repository, err := catalog.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 503, "repository_unavailable", "repository context is unavailable")
			return
		}
		bare, err := git.Open(repository.ID)
		if err != nil {
			writeAPIError(w, 503, "repository_unavailable", "repository context is unavailable")
			return
		}
		data, err := exec.Command("git", "--git-dir="+bare.Path(), "rev-parse", "refs/heads/"+repository.DefaultBranch).Output()
		revision := strings.TrimSpace(string(data))
		if err != nil || len(revision) != 40 {
			writeAPIError(w, 409, "recovery_improvement_base_unavailable", "the default branch has no exact implementation base")
			return
		}
		items := []proposals.ReasoningItem{{ID: "exercise", Kind: "recovery_exercise", Summary: exercise.ID, Status: "failed_or_risky"}, {ID: "finding", Kind: "recovery_finding", Summary: in.FindingID, Status: "cited"}}
		for i, criterion := range in.AcceptanceCriteria {
			items = append(items, proposals.ReasoningItem{ID: fmt.Sprintf("criterion-%d", i), Kind: "acceptance_criterion", Summary: criterion, Status: "required"})
		}
		origin := proposals.ReasoningOrigin{RecoveryExerciseID: exercise.ID, RecoveryInvestigationID: in.InvestigationID, RecoveryFindingID: in.FindingID, Revision: revision, Items: items, AnalysisStatus: "recovery_improvement"}
		for _, item := range items {
			origin.SelectedItemIDs = append(origin.SelectedItemIDs, item.ID)
		}
		tasks, participants := []proposals.ImplementationTaskInput{}, []string{actor.UserID}
		for _, task := range in.Tasks {
			tasks = append(tasks, proposals.ImplementationTaskInput{Title: task.Title, Outcome: "Satisfy recovery criteria: " + strings.Join(in.AcceptanceCriteria, "; "), Risk: "Production state remains protected; delivery uses ordinary review and release controls.", VerificationPlan: "Run a fresh isolated recovery exercise against changed plan or source evidence.", AssigneeType: task.AssigneeType, AssigneeID: task.AssigneeID, DependsOnPrevious: task.DependsOnPrevious})
			if task.AssigneeType == "human" {
				participants = append(participants, task.AssigneeID)
			}
		}
		var proposal proposals.Proposal
		var made []proposals.Task
		err = catalog.WithCurrentParticipants(participants, repository.ID, func() error {
			return bare.WithReferenceTarget("refs/heads/"+repository.DefaultBranch, revision, func() error {
				var e error
				proposal, made, e = proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: repository.ID, ActorID: actor.UserID, Title: in.Title, Body: in.Body, Origin: origin, Tasks: tasks})
				return e
			})
		})
		if err != nil {
			writeAPIError(w, 422, "recovery_improvement_invalid", "authority, evidence, assignment, or implementation base changed")
			return
		}
		taskIDs := []string{}
		for _, task := range made {
			taskIDs = append(taskIDs, task.ID)
		}
		updated, improvement, err := exercises.LinkImprovement(repository.ID, exercise.ID, actor.UserID, recoveryexercises.Improvement{InvestigationID: in.InvestigationID, FindingID: in.FindingID, ProposalID: proposal.ID, TaskIDs: taskIDs, BaseRevision: revision, Criteria: in.AcceptanceCriteria})
		if err != nil {
			writeAPIError(w, 409, "recovery_improvement_link_pending", "ordinary work was published but its recovery link is pending an exact retry")
			return
		}
		writeJSON(w, 201, map[string]any{"exercise": updated, "improvement": improvement, "proposal": proposal, "tasks": made})
	})
	mux.HandleFunc("POST /repositories/{id}/recovery-exercises/{exercise_id}/improvements/{improvement_id}/verifications", func(w http.ResponseWriter, r *http.Request) {
		_, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "recovery_verification_owner_required", "only the repository owner may accept fresh rehearsal evidence")
			return
		}
		var in recoveryVerificationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_recovery_verification", "a fresh successful exercise is required")
			return
		}
		follow, err := exercises.Get(r.PathValue("id"), in.FollowUpExerciseID)
		if err == nil {
			refreshExercise(&follow, git, environments, plans, commitments)
		}
		if err != nil || !follow.Current {
			writeAPIError(w, 422, "recovery_verification_stale", "follow-up evidence must be current")
			return
		}
		out, err := exercises.VerifyImprovement(r.PathValue("id"), r.PathValue("exercise_id"), r.PathValue("improvement_id"), follow.ID)
		writeRecoveryMutation(w, out, err)
	})
}

func writeRecoveryMutation(w http.ResponseWriter, out recoveryexercises.Exercise, err error) {
	if errors.Is(err, recoveryexercises.ErrNotFound) {
		writeAPIError(w, 404, "recovery_evidence_not_found", "recovery evidence not found")
		return
	}
	if err != nil {
		writeAPIError(w, 409, "recovery_evidence_changed", "recovery evidence is invalid or changed")
		return
	}
	writeJSON(w, 201, out)
}
func recoveryEvidenceResolves(git *storage.Store, releaseStore *releases.Store, plans *protectionplans.Store, commitments *recoverycommitments.Store, exercise recoveryexercises.Exercise, evidence []recoveryexercises.Evidence) bool {
	plan, planErr := plans.Get(exercise.PlanID)
	commitment, commitmentErr := commitments.Get(exercise.CommitmentID)
	for _, e := range evidence {
		ok := false
		switch e.Kind {
		case "exercise_result":
			for _, result := range exercise.Results {
				ok = ok || e.ResourceID == result.StepID
			}
		case "code":
			repo, err := git.Open(exercise.RepositoryID)
			if err == nil {
				resolved, resolveErr := resolveRevision(repo, e.Revision)
				ok = resolveErr == nil && string(resolved) == e.Revision
			}
		case "release":
			if releaseStore != nil {
				value, err := releaseStore.Get(exercise.RepositoryID, e.ResourceID)
				ok = err == nil && value.CommitID == e.Revision
			}
		case "protection_plan", "configuration":
			ok = planErr == nil && e.ResourceID == plan.ID && (e.Revision == "" || e.Revision == fmt.Sprint(plan.Version))
		case "recovery_commitment":
			ok = commitmentErr == nil && e.ResourceID == commitment.ID && (e.Revision == "" || e.Revision == fmt.Sprint(commitment.CurrentVersion))
		case "ownership":
			if commitmentErr == nil {
				revision := commitment.Revisions[len(commitment.Revisions)-1]
				ok = e.ResourceID == exercise.StartedBy
				for _, owner := range revision.OwnerIDs {
					ok = ok || e.ResourceID == owner
				}
			}
		case "dependency":
			if commitmentErr == nil {
				revision := commitment.Revisions[len(commitment.Revisions)-1]
				for _, target := range revision.Targets {
					for _, dependency := range target.Dependencies {
						ok = ok || e.ResourceID == dependency.TargetID
					}
				}
			}
		}
		if !ok {
			return false
		}
	}
	return len(evidence) > 0
}
func recoveryEvidenceText(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n") {
		return false
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"password=", "token=", "secret=", "authorization:", "private key"} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

type recoveryRuntime struct {
	root        string
	provisioned bool
	restored    int
}

func newExerciseExecutor(source protectionplans.RestoredSource, declared []string) (func(recoveryexercises.Step) (string, string, string, bool), func()) {
	runtime := &recoveryRuntime{}
	cleanup := func() {
		if runtime.root != "" {
			_ = os.RemoveAll(runtime.root)
		}
	}
	execute := func(step recoveryexercises.Step) (string, string, string, bool) {
		switch step.Kind {
		case "restore":
			if step.Command != "restore:protected-manifest" {
				return "failed", "restore command is not permitted", "", false
			}
			if runtime.provisioned {
				return "failed", "isolated environment was already populated", "", false
			}
			root, err := os.MkdirTemp("", "vivarium-recovery-")
			if err != nil {
				return "failed", "isolated environment could not be provisioned", "", false
			}
			runtime.root = root
			if err = restoreExercisePayload(runtime, source); err != nil {
				cleanup()
				runtime.root = ""
				return "failed", "protected capture could not be applied to isolated environment", "", false
			}
			runtime.provisioned = true
			return "passed", "protected capture applied to ephemeral isolated environment", "restored artifacts: " + exerciseInt(runtime.restored), false
		case "integrity":
			if step.Command != "verify:manifest" && step.Command != "verify:dependencies" {
				return "failed", "integrity command is not permitted", "", false
			}
			if step.Command == "verify:dependencies" {
				ids := map[string]bool{}
				for _, e := range source.Manifest {
					ids[e.Version] = true
				}
				for _, e := range source.Manifest {
					dependencies := e.Dependencies
					// Parent commits are provenance; only the captured root tree
					// is a required restore input.
					if e.Kind == "commit" && len(dependencies) > 1 {
						dependencies = dependencies[:1]
					}
					for _, d := range dependencies {
						if !ids[d] {
							return "failed", "missing declared dependency", "", false
						}
					}
				}
			}
			sum := sha256.Sum256(source.Payload)
			return "passed", "integrity verified without exposing restored content", "sha256:" + hex.EncodeToString(sum[:]), false
		case "journey":
			name := strings.TrimPrefix(step.Command, "journey:")
			if name == step.Command || !exerciseContains(declared, name) {
				return "failed", "journey is not declared by the protection plan", "", false
			}
			if !runtime.provisioned {
				return "failed", "isolated environment has not been restored", "", false
			}
			if name != "smoke" {
				return "failed", "declared journey has no bounded runner implementation", "", false
			}
			if err := runRecoverySmoke(runtime); err != nil {
				return "failed", "restored application failed its declared smoke journey", "", false
			}
			return "passed", "bounded restored application returned its declared output", "journey:smoke", false
		case "manual":
			if step.Command != "manual:confirm" {
				return "failed", "manual command is not permitted", "", true
			}
			return "passed", "manual recovery step acknowledged by launch actor", "", true
		}
		return "failed", "unsupported bounded command", "", false
	}
	return execute, cleanup
}

type recoverySmokeContract struct {
	Version              string   `json:"version"`
	Image                string   `json:"image"`
	Entrypoint           string   `json:"entrypoint"`
	Arguments            []string `json:"arguments,omitempty"`
	TimeoutSeconds       int      `json:"timeout_seconds"`
	ExpectedExitCode     int      `json:"expected_exit_code"`
	ExpectedStdoutSHA256 string   `json:"expected_stdout_sha256"`
}

type recoveryApplicationRunner func(root string, contract recoverySmokeContract) (int, []byte, error)

var executeRecoveryApplication recoveryApplicationRunner = runRecoveryApplicationContainer

func runRecoverySmoke(runtime *recoveryRuntime) error {
	contracts, err := filepath.Glob(filepath.Join(runtime.root, "workspace", "*", ".vivarium", "recovery-smoke.json"))
	if err != nil || len(contracts) != 1 {
		return recoveryexercises.ErrInvalid
	}
	body, err := os.ReadFile(contracts[0])
	if err != nil || len(body) > 16*1024 {
		return recoveryexercises.ErrInvalid
	}
	var contract recoverySmokeContract
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&contract) != nil || contract.Version != "v1" || contract.TimeoutSeconds < 1 || contract.TimeoutSeconds > 60 || contract.ExpectedExitCode != 0 || len(contract.ExpectedStdoutSHA256) != 64 || len(contract.Arguments) > 16 || !validRecoveryImage(contract.Image) {
		return recoveryexercises.ErrInvalid
	}
	if _, err = hex.DecodeString(contract.ExpectedStdoutSHA256); err != nil {
		return recoveryexercises.ErrInvalid
	}
	for _, argument := range contract.Arguments {
		if len(argument) > 256 || strings.ContainsAny(argument, "\x00\r\n") {
			return recoveryexercises.ErrInvalid
		}
	}
	cleanEntrypoint := filepath.Clean(contract.Entrypoint)
	if cleanEntrypoint == "." || filepath.IsAbs(cleanEntrypoint) || cleanEntrypoint == ".." || strings.HasPrefix(cleanEntrypoint, ".."+string(filepath.Separator)) {
		return recoveryexercises.ErrInvalid
	}
	targetRoot := filepath.Dir(filepath.Dir(contracts[0]))
	applicationEntrypoint := filepath.Join(targetRoot, cleanEntrypoint)
	info, err := os.Stat(applicationEntrypoint)
	if err != nil || !info.Mode().IsRegular() || !recoveryExecutable(applicationEntrypoint) || os.Chmod(applicationEntrypoint, 0500) != nil {
		return recoveryexercises.ErrInvalid
	}
	exitCode, stdout, err := executeRecoveryApplication(targetRoot, contract)
	if err != nil || len(stdout) > 1<<20 {
		return recoveryexercises.ErrInvalid
	}
	sum := sha256.Sum256(stdout)
	if exitCode != contract.ExpectedExitCode || hex.EncodeToString(sum[:]) != strings.ToLower(contract.ExpectedStdoutSHA256) {
		return recoveryexercises.ErrInvalid
	}
	return nil
}

func recoveryExecutable(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	header := make([]byte, 4)
	n, err := io.ReadFull(file, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return false
	}
	return n >= 2 && (bytes.Equal(header[:2], []byte("#!")) || (n == 4 && bytes.Equal(header, []byte{0x7f, 'E', 'L', 'F'})))
}

func validRecoveryImage(image string) bool {
	parts := strings.Split(image, "@sha256:")
	if len(parts) != 2 || parts[0] == "" || len(parts[1]) != 64 {
		return false
	}
	if strings.HasPrefix(parts[0], "-") || strings.ContainsAny(parts[0], " \t\r\n") {
		return false
	}
	_, err := hex.DecodeString(parts[1])
	return err == nil
}

func runRecoveryApplicationContainer(root string, contract recoverySmokeContract) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(contract.TimeoutSeconds)*time.Second)
	defer cancel()
	nameSum := sha256.Sum256([]byte(root))
	containerName := "vivarium-recovery-" + hex.EncodeToString(nameSum[:8])
	args := recoveryContainerArguments(root, containerName, contract)
	command := exec.CommandContext(ctx, "docker", args...)
	defer exec.Command("docker", "rm", "--force", containerName).Run()
	stdout := &boundedOutput{remaining: 1 << 20}
	command.Stdout = stdout
	command.Stderr = &recoveryDiscard{remaining: 64 << 10}
	err := command.Run()
	if ctx.Err() != nil {
		return -1, nil, ctx.Err()
	}
	if stdout.overflow {
		return -1, nil, recoveryexercises.ErrInvalid
	}
	if err == nil {
		return 0, stdout.buffer.Bytes(), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), stdout.buffer.Bytes(), nil
	}
	return -1, nil, err
}

func recoveryContainerArguments(root, name string, contract recoverySmokeContract) []string {
	args := []string{"run", "--name", name, "--rm", "--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=128m", "--cpus=1", "--user", exerciseInt(os.Getuid()) + ":" + exerciseInt(os.Getgid()), "--mount", "type=bind,src=" + root + ",dst=/workspace,readonly", "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=16m,mode=1777", "--workdir", "/workspace", "--env", "HOME=/tmp", contract.Image, "./" + filepath.ToSlash(filepath.Clean(contract.Entrypoint))}
	return append(args, contract.Arguments...)
}

type recoveryDiscard struct{ remaining int }

func (w *recoveryDiscard) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		w.remaining = 0
		return len(p), nil
	}
	w.remaining -= len(p)
	return len(p), nil
}

type boundedOutput struct {
	buffer    bytes.Buffer
	remaining int
	overflow  bool
}

func (w *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if n > w.remaining {
		w.buffer.Write(p[:w.remaining])
		w.remaining = 0
		w.overflow = true
		return n, nil
	}
	w.remaining -= n
	w.buffer.Write(p)
	return n, nil
}

func restoreExercisePayload(runtime *recoveryRuntime, source protectionplans.RestoredSource) error {
	var resources map[string]json.RawMessage
	if json.Unmarshal(source.Payload, &resources) != nil {
		return protectionplans.ErrInvalid
	}
	for targetID, raw := range resources {
		targetSum := sha256.Sum256([]byte(targetID))
		targetDirectory := hex.EncodeToString(targetSum[:8])
		var repository struct {
			Objects map[string]storage.Object `json:"objects"`
		}
		if json.Unmarshal(raw, &repository) == nil && len(repository.Objects) > 0 {
			for _, entry := range source.Manifest {
				object, ok := repository.Objects[entry.Version]
				if !ok || object.Type != storage.BlobObject {
					continue
				}
				clean := filepath.Clean(entry.Path)
				if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
					return protectionplans.ErrInvalid
				}
				sum := sha256.Sum256(object.Content)
				if hex.EncodeToString(sum[:]) != entry.SHA256 {
					return protectionplans.ErrInvalid
				}
				destination := filepath.Join(runtime.root, "workspace", targetDirectory, clean)
				if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
					return err
				}
				if err := os.WriteFile(destination, object.Content, 0600); err != nil {
					return err
				}
				runtime.restored++
			}
			continue
		}
		// Governed environment captures contain only the credential-free public
		// projection. Restore it under an opaque name to avoid trusting target IDs.
		destination := filepath.Join(runtime.root, "environment", targetDirectory+".json")
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(destination, raw, 0600); err != nil {
			return err
		}
		runtime.restored++
	}
	if runtime.restored == 0 {
		return protectionplans.ErrInvalid
	}
	manifest, err := json.Marshal(source.Manifest)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(runtime.root, ".recovery", "manifest.json")
	if err = os.MkdirAll(filepath.Dir(manifestPath), 0700); err != nil {
		return err
	}
	return os.WriteFile(manifestPath, manifest, 0600)
}
func refreshExercise(x *recoveryexercises.Exercise, git *storage.Store, environments *deployments.Store, plans *protectionplans.Store, commitments *recoverycommitments.Store) {
	x.Current = true
	x.StaleReasons = []string{}
	p, e := plans.Get(x.PlanID)
	if e != nil {
		x.Current = false
		x.StaleReasons = append(x.StaleReasons, "protection_plan_unavailable")
		return
	}
	if p.Version != x.PlanVersion {
		x.Current = false
		x.StaleReasons = append(x.StaleReasons, "protection_plan_changed")
	}
	commitment, commitmentErr := commitments.Get(x.CommitmentID)
	if commitmentErr != nil || commitment.CurrentVersion != x.CommitmentVersion {
		x.Current = false
		x.StaleReasons = append(x.StaleReasons, "recovery_commitment_changed")
	}
	found := false
	for _, c := range p.Captures {
		if c.ID == x.CaptureID {
			found = true
			if !c.Recoverable {
				x.Current = false
				x.StaleReasons = append(x.StaleReasons, "protected_capture_unavailable")
			}
			if c.SourceRevision != x.SourceRevision {
				x.Current = false
				x.StaleReasons = append(x.StaleReasons, "protected_dependencies_changed")
			}
			frozen := p
			frozen.Resources = c.Resources
			var currentSource protectionplans.Source
			var sourceErr error
			for _, resource := range frozen.Resources {
				if resource.Kind == "environment" && environments == nil {
					sourceErr = protectionplans.ErrInvalid
				}
			}
			if sourceErr == nil {
				currentSource, sourceErr = buildProtectionSource(git, environments, frozen)
			}
			if sourceErr != nil || currentSource.Revision != x.SourceRevision {
				x.Current = false
				x.StaleReasons = append(x.StaleReasons, "protected_dependencies_changed")
			}
		}
	}
	if !found {
		x.Current = false
		x.StaleReasons = append(x.StaleReasons, "protected_capture_unavailable")
	}
	sort.Strings(x.StaleReasons)
}
func exerciseContains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func exerciseInt(v int) string {
	if v == 0 {
		return "0"
	}
	b := make([]byte, 0, 10)
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
