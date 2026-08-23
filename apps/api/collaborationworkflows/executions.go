package collaborationworkflows

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var ErrExecutionConflict = errors.New("workflow execution conflict")
var ErrExecutionBlocked = errors.New("workflow execution blocked")
var ErrCredential = errors.New("invalid step credential")

var credentialText = regexp.MustCompile(`(?i)(bearer\s+[a-z0-9._~+/=-]{12,}|(?:token|password|secret|api[_-]?key)\s*[:=]\s*\S+)`)
var commitID = regexp.MustCompile(`^[0-9a-f]{40}$`)
var sha256ID = regexp.MustCompile(`^[0-9a-f]{64}$`)

type TriggerEvent struct {
	ID                string            `json:"id"`
	Kind              string            `json:"kind"`
	Name              string            `json:"name"`
	ActorID           string            `json:"actor_id"`
	OccurredAt        time.Time         `json:"occurred_at"`
	Inputs            map[string]any    `json:"inputs"`
	ResourceRevisions map[string]string `json:"resource_revisions"`
}

type StepRun struct {
	StepID              string         `json:"step_id"`
	Status              string         `json:"status"`
	Attempt             int            `json:"attempt"`
	CredentialID        string         `json:"credential_id,omitempty"`
	CredentialSHA256    string         `json:"credential_sha256,omitempty"`
	CompletionSHA256    string         `json:"completion_sha256,omitempty"`
	CredentialExpiresAt *time.Time     `json:"credential_expires_at,omitempty"`
	StartedAt           *time.Time     `json:"started_at,omitempty"`
	FinishedAt          *time.Time     `json:"finished_at,omitempty"`
	Outputs             map[string]any `json:"outputs,omitempty"`
	ActionsUsed         int            `json:"actions_used"`
	FailureCode         string         `json:"failure_code,omitempty"`
	Attempts            []StepAttempt  `json:"attempts"`
	ProvidedInputs      map[string]any `json:"provided_inputs,omitempty"`
	TakenOverBy         string         `json:"taken_over_by,omitempty"`
}

type StepLog struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}
type StepArtifact struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	Restricted bool   `json:"restricted,omitempty"`
}
type AgentSession struct {
	ID      string `json:"id"`
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
	URL     string `json:"url,omitempty"`
}
type StepAttempt struct {
	Number       int            `json:"number"`
	Status       string         `json:"status"`
	StartedAt    time.Time      `json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	Inputs       map[string]any `json:"inputs"`
	Outputs      map[string]any `json:"outputs,omitempty"`
	Logs         []StepLog      `json:"logs"`
	Artifacts    []StepArtifact `json:"artifacts"`
	AgentSession *AgentSession  `json:"agent_session,omitempty"`
	CostUnits    float64        `json:"cost_units"`
	ActionsUsed  int            `json:"actions_used"`
	FailureCode  string         `json:"failure_code,omitempty"`
	Provenance   []string       `json:"provenance"`
}
type Intervention struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	StepID    string    `json:"step_id,omitempty"`
	ActorID   string    `json:"actor_id"`
	Reason    string    `json:"reason"`
	InputName string    `json:"input_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Version   int       `json:"version"`
}

type Execution struct {
	ID                   string         `json:"id"`
	RepositoryID         string         `json:"repository_id"`
	WorkflowID           string         `json:"workflow_id"`
	WorkflowVersion      int            `json:"workflow_version"`
	WorkflowSource       Source         `json:"workflow_source"`
	Trigger              TriggerEvent   `json:"trigger"`
	Status               string         `json:"status"`
	Version              int            `json:"version"`
	BudgetActions        int            `json:"budget_actions"`
	ActionsUsed          int            `json:"actions_used"`
	CostUnits            float64        `json:"cost_units"`
	Steps                []StepRun      `json:"steps"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	FinishedAt           *time.Time     `json:"finished_at,omitempty"`
	CancellationCode     string         `json:"cancellation_code,omitempty"`
	Interventions        []Intervention `json:"interventions"`
	PredictedNextActions []string       `json:"predicted_next_actions"`
	PausedBy             string         `json:"paused_by,omitempty"`
}

type StepLease struct {
	Execution Execution      `json:"execution"`
	Step      StepRun        `json:"step"`
	Token     string         `json:"token"`
	Authority []string       `json:"authority"`
	Inputs    map[string]any `json:"inputs"`
}

func (s *Store) StartExecution(workflowID string, workflowVersion int, event TriggerEvent) (Execution, error) {
	var out Execution
	err := s.lock(func() error {
		w, err := s.read(workflowID)
		if err != nil {
			return err
		}
		if w.Status != "active" || workflowVersion != w.CurrentVersion || workflowVersion > len(w.Revisions) {
			return ErrExecutionBlocked
		}
		rev := w.Revisions[workflowVersion-1]
		if event.ID == "" || event.ActorID == "" || event.OccurredAt.IsZero() || !triggerMatches(rev.Definition.Triggers, event) {
			return ErrInvalid
		}
		for k, v := range event.ResourceRevisions {
			if k == "" || !commitID.MatchString(v) {
				return ErrInvalid
			}
		}
		allowed := map[string]bool{}
		for _, tr := range rev.Definition.Triggers {
			if tr.Kind == event.Kind && tr.Event == event.Name {
				for _, in := range tr.Inputs {
					allowed[in.Name] = true
					if in.Required {
						if _, ok := event.Inputs[in.Name]; !ok {
							return ErrInvalid
						}
					}
				}
			}
		}
		for k, v := range event.Inputs {
			if !allowed[k] || containsCredential(v) {
				return ErrInvalid
			}
		}
		id := executionID(w.RepositoryID, workflowID, event.ID)
		if existing, e := s.readExecution(id); e == nil {
			if existing.WorkflowVersion != workflowVersion || existing.Trigger.ActorID != event.ActorID || !equalJSON(existing.Trigger, event) {
				return ErrExecutionConflict
			}
			out = existing
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		// One active execution per workflow and a conservative 60 starts/hour bound.
		now := s.now()
		recent := 0
		for _, ex := range s.listExecutionsUnlocked(w.RepositoryID, workflowID) {
			if ex.Status == "running" {
				return ErrExecutionBlocked
			}
			if now.Sub(ex.CreatedAt) < time.Hour {
				recent++
			}
		}
		if recent >= 60 {
			return ErrExecutionBlocked
		}
		steps := make([]StepRun, 0, len(rev.Definition.Steps))
		for _, st := range rev.Definition.Steps {
			status := "pending"
			if st.Approval != "" {
				status = "waiting_approval"
			} else if len(st.RequestedInputs) > 0 {
				status = "waiting_input"
			} else if st.Manual {
				status = "waiting_manual"
			}
			steps = append(steps, StepRun{StepID: st.ID, Status: status, Outputs: map[string]any{}, Attempts: []StepAttempt{}, ProvidedInputs: map[string]any{}})
		}
		out = Execution{ID: id, RepositoryID: w.RepositoryID, WorkflowID: w.ID, WorkflowVersion: workflowVersion, WorkflowSource: rev.Source, Trigger: event, Status: "running", Version: 1, BudgetActions: rev.Definition.BudgetActions, Steps: steps, Interventions: []Intervention{}, CreatedAt: now, UpdatedAt: now}
		deriveNextActions(&out, rev.Definition)
		return s.writeExecution(out)
	})
	return out, err
}

func (s *Store) ClaimStep(executionID, stepID string, expectedVersion int, actorAllowed bool) (StepLease, error) {
	var lease StepLease
	err := s.lock(func() error {
		ex, err := s.readExecution(executionID)
		if err != nil {
			return err
		}
		if !actorAllowed || ex.Status != "running" || ex.Version != expectedVersion {
			return ErrExecutionConflict
		}
		w, err := s.read(ex.WorkflowID)
		if err != nil {
			return err
		}
		def := w.Revisions[ex.WorkflowVersion-1].Definition
		st, ok := definitionStep(def, stepID)
		if !ok {
			return ErrNotFound
		}
		sr := executionStep(&ex, stepID)
		if sr.Status == "running" && sr.CredentialExpiresAt != nil && sr.CredentialExpiresAt.After(s.now()) {
			return ErrExecutionConflict
		}
		if sr.Status == "succeeded" || sr.Status == "cancelled" || !dependenciesSucceeded(ex, st.Needs) {
			return ErrExecutionBlocked
		}
		if sr.Attempt >= st.Retries+1 {
			sr.Status = "failed"
			sr.FailureCode = "retry_limit_exceeded"
			finishExecution(&ex, s.now())
			ex.Version++
			ex.UpdatedAt = s.now()
			return s.writeExecution(ex)
		}
		if ex.ActionsUsed >= ex.BudgetActions {
			return ErrExecutionBlocked
		}
		tokenBytes := make([]byte, 32)
		if _, err = rand.Read(tokenBytes); err != nil {
			return err
		}
		token := hex.EncodeToString(tokenBytes)
		sum := sha256.Sum256([]byte(token))
		now := s.now()
		expiry := now.Add(time.Duration(st.TimeoutSeconds) * time.Second)
		sr.Status = "running"
		sr.Attempt++
		sr.CredentialID = randomID(tokenBytes)
		sr.CredentialSHA256 = hex.EncodeToString(sum[:])
		sr.CredentialExpiresAt = &expiry
		sr.StartedAt = &now
		sr.FailureCode = ""
		sr.CompletionSHA256 = ""
		sr.Attempts = append(sr.Attempts, StepAttempt{Number: sr.Attempt, Status: "running", StartedAt: now, Inputs: stepInputs(ex, st), Logs: []StepLog{}, Artifacts: []StepArtifact{}, Provenance: []string{"workflow:" + ex.WorkflowID, "source:" + ex.WorkflowSource.Revision, "event:" + ex.Trigger.ID}})
		ex.Version++
		ex.UpdatedAt = now
		deriveNextActions(&ex, def)
		if err = s.writeExecution(ex); err != nil {
			return err
		}
		lease = StepLease{Execution: ex, Step: *sr, Token: token, Authority: append([]string{}, st.Invocation.Authority...), Inputs: stepInputs(ex, st)}
		return nil
	})
	return lease, err
}

func (s *Store) CompleteStep(executionID, stepID, token string, actions int, outputs map[string]any, failure string) (Execution, error) {
	return s.CompleteStepEvidence(executionID, stepID, token, actions, outputs, failure, nil, nil, nil, 0, nil)
}
func (s *Store) CompleteStepEvidence(executionID, stepID, token string, actions int, outputs map[string]any, failure string, logs []StepLog, artifacts []StepArtifact, session *AgentSession, cost float64, provenance []string) (Execution, error) {
	var ex Execution
	err := s.lock(func() error {
		var err error
		ex, err = s.readExecution(executionID)
		if err != nil {
			return err
		}
		w, err := s.read(ex.WorkflowID)
		if err != nil {
			return err
		}
		st, ok := definitionStep(w.Revisions[ex.WorkflowVersion-1].Definition, stepID)
		if !ok {
			return ErrNotFound
		}
		sr := executionStep(&ex, stepID)
		sum := sha256.Sum256([]byte(token))
		completion := completionDigest(hex.EncodeToString(sum[:]), actions, outputs, failure, logs, artifacts, session, cost, provenance)
		if sr.CompletionSHA256 != "" {
			if sr.CompletionSHA256 == completion {
				return nil
			}
			return ErrExecutionConflict
		}
		if ex.Status != "running" {
			return ErrExecutionBlocked
		}
		if sr.Status != "running" || sr.CredentialSHA256 == "" || sr.CredentialSHA256 != hex.EncodeToString(sum[:]) {
			return ErrCredential
		}
		now := s.now()
		if sr.CredentialExpiresAt == nil || !sr.CredentialExpiresAt.After(now) {
			sr.Status = "interrupted"
			sr.FailureCode = "credential_expired"
			sr.CredentialSHA256 = ""
			ex.Version++
			ex.UpdatedAt = now
			_ = s.writeExecution(ex)
			return ErrCredential
		}
		if actions < 0 || actions > st.BudgetActions || ex.ActionsUsed+actions > ex.BudgetActions || cost < 0 {
			return ErrExecutionBlocked
		}
		if containsCredential(logs) || containsCredential(artifacts) || containsCredential(provenance) || containsCredential(session) {
			return ErrInvalid
		}
		for _, l := range logs {
			if !oneOf(l.Level, "debug", "info", "warning", "error") || l.Time.IsZero() || len(l.Message) > 10000 {
				return ErrInvalid
			}
		}
		for _, a := range artifacts {
			if a.Name == "" || !sha256ID.MatchString(a.SHA256) || a.Size < 0 {
				return ErrInvalid
			}
		}
		allowed := map[string]bool{}
		for _, k := range st.Outputs {
			allowed[k] = true
		}
		clean := map[string]any{}
		for k, v := range outputs {
			if !allowed[k] || containsCredential(v) {
				return ErrInvalid
			}
			clean[k] = v
		}
		sr.ActionsUsed = actions
		ex.ActionsUsed += actions
		sr.Outputs = clean
		sr.CredentialSHA256 = ""
		sr.CredentialExpiresAt = nil
		sr.FinishedAt = &now
		sr.CompletionSHA256 = completion
		if len(sr.Attempts) == 0 {
			return ErrExecutionConflict
		}
		attempt := &sr.Attempts[len(sr.Attempts)-1]
		attempt.FinishedAt, attempt.Outputs, attempt.Logs, attempt.Artifacts, attempt.AgentSession, attempt.CostUnits, attempt.ActionsUsed, attempt.FailureCode = &now, clean, append([]StepLog{}, logs...), append([]StepArtifact{}, artifacts...), session, cost, actions, failure
		attempt.Provenance = append(attempt.Provenance, provenance...)
		ex.CostUnits += cost
		if failure != "" {
			sr.Status = "interrupted"
			if sr.Attempt >= st.Retries+1 {
				sr.Status = "failed"
			}
			sr.FailureCode = failure
		} else {
			sr.Status = "succeeded"
		}
		attempt.Status = sr.Status
		finishExecution(&ex, now)
		deriveNextActions(&ex, w.Revisions[ex.WorkflowVersion-1].Definition)
		ex.Version++
		ex.UpdatedAt = now
		return s.writeExecution(ex)
	})
	return ex, err
}

func (s *Store) Intervene(id, actor, kind, stepID, reason, inputName string, value any, expected int) (Execution, error) {
	var ex Execution
	err := s.lock(func() error {
		var err error
		ex, err = s.readExecution(id)
		if err != nil {
			return err
		}
		if ex.Version != expected || actor == "" || strings.TrimSpace(reason) == "" {
			return ErrExecutionConflict
		}
		if containsCredential(value) || containsCredential(reason) {
			return ErrInvalid
		}
		w, err := s.read(ex.WorkflowID)
		if err != nil {
			return err
		}
		def := w.Revisions[ex.WorkflowVersion-1].Definition
		st, hasStep := definitionStep(def, stepID)
		sr := executionStep(&ex, stepID)
		now := s.now()
		switch kind {
		case "pause":
			if ex.Status != "running" {
				return ErrExecutionBlocked
			}
			ex.Status, ex.PausedBy = "paused", actor
		case "resume":
			if ex.Status != "paused" {
				return ErrExecutionBlocked
			}
			ex.Status, ex.PausedBy = "running", ""
		case "cancel":
			if ex.Status != "running" && ex.Status != "paused" {
				return ErrExecutionBlocked
			}
			ex.Status, ex.CancellationCode, ex.FinishedAt = "cancelled", reason, &now
			for i := range ex.Steps {
				if !oneOf(ex.Steps[i].Status, "succeeded", "skipped") {
					ex.Steps[i].Status = "cancelled"
					ex.Steps[i].CredentialSHA256 = ""
					ex.Steps[i].CredentialExpiresAt = nil
				}
			}
		case "retry":
			if !hasStep || sr == nil || !oneOf(sr.Status, "interrupted", "failed") || sr.Attempt >= st.Retries+1 {
				return ErrExecutionBlocked
			}
			sr.Status, sr.FailureCode, ex.Status, ex.FinishedAt = "pending", "", "running", nil
			for i := range ex.Steps {
				other := &ex.Steps[i]
				if other.FailureCode != "execution_terminal" {
					continue
				}
				other.FailureCode, other.FinishedAt = "", nil
				otherDef, _ := definitionStep(def, other.StepID)
				switch {
				case otherDef.Approval != "":
					other.Status = "waiting_approval"
				case len(otherDef.RequestedInputs) > 0:
					other.Status = "waiting_input"
				case otherDef.Manual:
					other.Status = "waiting_manual"
				default:
					other.Status = "pending"
				}
			}
		case "skip":
			if !hasStep || sr == nil || !st.Optional || !oneOf(sr.Status, "pending", "waiting_input", "waiting_approval", "waiting_manual", "interrupted") {
				return ErrExecutionBlocked
			}
			sr.Status, sr.FinishedAt = "skipped", &now
			finishExecution(&ex, now)
		case "provide_input":
			if !hasStep || sr == nil || sr.Status != "waiting_input" || !contains(st.RequestedInputs, inputName) || inputName == "" {
				return ErrExecutionBlocked
			}
			if sr.ProvidedInputs == nil {
				sr.ProvidedInputs = map[string]any{}
			}
			sr.ProvidedInputs[inputName] = value
			ready := true
			for _, n := range st.RequestedInputs {
				if _, ok := sr.ProvidedInputs[n]; !ok {
					ready = false
				}
			}
			if ready && sr.Status == "waiting_input" {
				sr.Status = "pending"
			}
		case "approve":
			if !hasStep || sr == nil || st.Approval == "" || sr.Status != "waiting_approval" {
				return ErrExecutionBlocked
			}
			sr.Status = "pending"
		case "take_over":
			if !hasStep || sr == nil || !st.Manual || !oneOf(sr.Status, "waiting_manual", "pending") {
				return ErrExecutionBlocked
			}
			sr.Status, sr.TakenOverBy = "pending", actor
		default:
			return ErrInvalid
		}
		ex.Version++
		ex.UpdatedAt = now
		ex.Interventions = append(ex.Interventions, Intervention{ID: executionID(ex.ID, kind, strings.Join([]string{actor, stepID, inputName, now.String()}, "\x00")), Kind: kind, StepID: stepID, ActorID: actor, Reason: reason, InputName: inputName, CreatedAt: now, Version: ex.Version})
		deriveNextActions(&ex, def)
		return s.writeExecution(ex)
	})
	return ex, err
}

func PublicExecution(ex Execution) Execution {
	for i := range ex.Steps {
		ex.Steps[i].CredentialSHA256 = ""
		ex.Steps[i].CompletionSHA256 = ""
		for j := range ex.Steps[i].Attempts {
			visible := make([]StepArtifact, 0, len(ex.Steps[i].Attempts[j].Artifacts))
			for _, artifact := range ex.Steps[i].Attempts[j].Artifacts {
				if !artifact.Restricted {
					visible = append(visible, artifact)
				}
			}
			ex.Steps[i].Attempts[j].Artifacts = visible
		}
	}
	return ex
}

func (s *Store) CancelExecution(id, code string) (Execution, error) {
	var ex Execution
	err := s.lock(func() error {
		var e error
		ex, e = s.readExecution(id)
		if e != nil {
			return e
		}
		if ex.Status != "running" {
			return ErrExecutionConflict
		}
		now := s.now()
		ex.Status = "cancelled"
		ex.CancellationCode = code
		ex.FinishedAt = &now
		ex.UpdatedAt = now
		ex.Version++
		for i := range ex.Steps {
			if ex.Steps[i].Status == "pending" || ex.Steps[i].Status == "running" || ex.Steps[i].Status == "interrupted" {
				ex.Steps[i].Status = "cancelled"
				ex.Steps[i].CredentialSHA256 = ""
				ex.Steps[i].CredentialExpiresAt = nil
			}
		}
		return s.writeExecution(ex)
	})
	return ex, err
}
func (s *Store) GetExecution(id string) (Execution, error) {
	var ex Execution
	err := s.lock(func() error { var e error; ex, e = s.readExecution(id); return e })
	return ex, err
}
func (s *Store) ListExecutions(repo, workflow string) ([]Execution, error) {
	var out []Execution
	err := s.lock(func() error { out = s.listExecutionsUnlocked(repo, workflow); return nil })
	return out, err
}

func (s *Store) readExecution(id string) (Execution, error) {
	var ex Execution
	b, e := os.ReadFile(filepath.Join(s.root, "execution-"+id+".json"))
	if os.IsNotExist(e) {
		return ex, ErrNotFound
	}
	if e != nil {
		return ex, e
	}
	e = json.Unmarshal(b, &ex)
	return ex, e
}
func (s *Store) writeExecution(ex Execution) error {
	b, e := json.MarshalIndent(ex, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".execution-")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, "execution-"+ex.ID+".json"))
	}
	if e != nil {
		return e
	}
	d, e := os.Open(s.root)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func (s *Store) listExecutionsUnlocked(repo, workflow string) []Execution {
	out := []Execution{}
	es, _ := os.ReadDir(s.root)
	for _, f := range es {
		if f.IsDir() || !strings.HasPrefix(f.Name(), "execution-") || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(f.Name(), "execution-"), ".json")
		ex, e := s.readExecution(id)
		if e == nil && ex.RepositoryID == repo && (workflow == "" || ex.WorkflowID == workflow) {
			out = append(out, ex)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
func triggerMatches(ts []Trigger, e TriggerEvent) bool {
	for _, t := range ts {
		if t.Kind == e.Kind && t.Event == e.Name {
			return true
		}
	}
	return false
}
func definitionStep(d Definition, id string) (Step, bool) {
	for _, s := range d.Steps {
		if s.ID == id {
			return s, true
		}
	}
	return Step{}, false
}
func executionStep(ex *Execution, id string) *StepRun {
	for i := range ex.Steps {
		if ex.Steps[i].StepID == id {
			return &ex.Steps[i]
		}
	}
	return nil
}
func dependenciesSucceeded(ex Execution, ids []string) bool {
	for _, id := range ids {
		s := executionStep(&ex, id)
		if s == nil || (s.Status != "succeeded" && s.Status != "skipped") {
			return false
		}
	}
	return true
}
func stepInputs(ex Execution, st Step) map[string]any {
	out := map[string]any{}
	if current := executionStep(&ex, st.ID); current != nil {
		for k, v := range current.ProvidedInputs {
			out[k] = v
		}
	}
	for key, ref := range st.Inputs {
		if strings.HasPrefix(ref, "event.") {
			out[key] = ex.Trigger.Inputs[strings.TrimPrefix(ref, "event.")]
			continue
		}
		parts := strings.Split(ref, ".")
		if len(parts) == 3 && parts[0] == "steps" {
			if prior := executionStep(&ex, parts[1]); prior != nil {
				out[key] = prior.Outputs[parts[2]]
			}
		}
	}
	return out
}
func finishExecution(ex *Execution, now time.Time) {
	all := true
	failed := false
	for _, s := range ex.Steps {
		if s.Status == "failed" {
			failed = true
		}
		if s.Status != "succeeded" && s.Status != "skipped" && s.Status != "failed" && s.Status != "cancelled" {
			all = false
		}
	}
	if failed {
		ex.Status = "failed"
		ex.FinishedAt = &now
		for i := range ex.Steps {
			step := &ex.Steps[i]
			if step.Status == "pending" || step.Status == "running" || step.Status == "interrupted" {
				step.Status = "cancelled"
				step.FailureCode = "execution_terminal"
				step.CredentialSHA256 = ""
				step.CredentialExpiresAt = nil
				step.FinishedAt = &now
			}
		}
	} else if all {
		ex.Status = "succeeded"
		ex.FinishedAt = &now
	}
}
func deriveNextActions(ex *Execution, def Definition) {
	next := []string{}
	if ex.Status == "paused" {
		next = append(next, "resume or cancel this execution")
	}
	if ex.Status == "running" {
		for _, sr := range ex.Steps {
			st, _ := definitionStep(def, sr.StepID)
			switch sr.Status {
			case "pending":
				if dependenciesSucceeded(*ex, st.Needs) {
					if st.Manual {
						next = append(next, "take over "+st.Name)
					} else {
						next = append(next, "run "+st.Name)
					}
				}
			case "waiting_input":
				next = append(next, "provide requested input for "+st.Name)
			case "waiting_approval":
				next = append(next, "approve "+st.Name)
			case "waiting_manual":
				next = append(next, "take over "+st.Name)
			case "interrupted":
				if sr.Attempt < st.Retries+1 {
					next = append(next, "retry "+st.Name)
				}
			}
			if st.Optional && !oneOf(sr.Status, "succeeded", "skipped", "cancelled") {
				next = append(next, "skip optional "+st.Name)
			}
		}
	}
	if len(next) == 0 && oneOf(ex.Status, "succeeded", "failed", "cancelled") {
		next = append(next, "inspect retained attempts and provenance")
	}
	ex.PredictedNextActions = next
}
func executionID(repo, wf, event string) string {
	h := sha256.Sum256([]byte(repo + "\x00" + wf + "\x00" + event))
	return hex.EncodeToString(h[:16])
}
func randomID(b []byte) string { return hex.EncodeToString(b[:8]) }
func equalJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func completionDigest(tokenDigest string, actions int, outputs map[string]any, failure string, evidence ...any) string {
	b, _ := json.Marshal(struct {
		TokenDigest string         `json:"token_digest"`
		Actions     int            `json:"actions"`
		Outputs     map[string]any `json:"outputs"`
		Failure     string         `json:"failure"`
		Evidence    []any          `json:"evidence"`
	}{tokenDigest, actions, outputs, failure, evidence})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func containsCredential(v any) bool {
	b, e := json.Marshal(v)
	return e != nil || credentialText.Match(b)
}
