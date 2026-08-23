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
	CredentialExpiresAt *time.Time     `json:"credential_expires_at,omitempty"`
	StartedAt           *time.Time     `json:"started_at,omitempty"`
	FinishedAt          *time.Time     `json:"finished_at,omitempty"`
	Outputs             map[string]any `json:"outputs,omitempty"`
	ActionsUsed         int            `json:"actions_used"`
	FailureCode         string         `json:"failure_code,omitempty"`
}

type Execution struct {
	ID               string       `json:"id"`
	RepositoryID     string       `json:"repository_id"`
	WorkflowID       string       `json:"workflow_id"`
	WorkflowVersion  int          `json:"workflow_version"`
	WorkflowSource   Source       `json:"workflow_source"`
	Trigger          TriggerEvent `json:"trigger"`
	Status           string       `json:"status"`
	Version          int          `json:"version"`
	BudgetActions    int          `json:"budget_actions"`
	ActionsUsed      int          `json:"actions_used"`
	Steps            []StepRun    `json:"steps"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
	FinishedAt       *time.Time   `json:"finished_at,omitempty"`
	CancellationCode string       `json:"cancellation_code,omitempty"`
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
			steps = append(steps, StepRun{StepID: st.ID, Status: "pending", Outputs: map[string]any{}})
		}
		out = Execution{ID: id, RepositoryID: w.RepositoryID, WorkflowID: w.ID, WorkflowVersion: workflowVersion, WorkflowSource: rev.Source, Trigger: event, Status: "running", Version: 1, BudgetActions: rev.Definition.BudgetActions, Steps: steps, CreatedAt: now, UpdatedAt: now}
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
		ex.Version++
		ex.UpdatedAt = now
		if err = s.writeExecution(ex); err != nil {
			return err
		}
		lease = StepLease{Execution: ex, Step: *sr, Token: token, Authority: append([]string{}, st.Invocation.Authority...), Inputs: stepInputs(ex, st)}
		return nil
	})
	return lease, err
}

func (s *Store) CompleteStep(executionID, stepID, token string, actions int, outputs map[string]any, failure string) (Execution, error) {
	var ex Execution
	err := s.lock(func() error {
		var err error
		ex, err = s.readExecution(executionID)
		if err != nil {
			return err
		}
		if ex.Status != "running" {
			return ErrExecutionBlocked
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
		if actions < 0 || actions > st.BudgetActions || ex.ActionsUsed+actions > ex.BudgetActions {
			return ErrExecutionBlocked
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
		if failure != "" {
			sr.Status = "interrupted"
			if sr.Attempt >= st.Retries+1 {
				sr.Status = "failed"
			}
			sr.FailureCode = failure
		} else {
			sr.Status = "succeeded"
		}
		finishExecution(&ex, now)
		ex.Version++
		ex.UpdatedAt = now
		return s.writeExecution(ex)
	})
	return ex, err
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
		if s == nil || s.Status != "succeeded" {
			return false
		}
	}
	return true
}
func stepInputs(ex Execution, st Step) map[string]any {
	out := map[string]any{}
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
		if s.Status != "succeeded" && s.Status != "failed" && s.Status != "cancelled" {
			all = false
		}
	}
	if failed {
		ex.Status = "failed"
		ex.FinishedAt = &now
	} else if all {
		ex.Status = "succeeded"
		ex.FinishedAt = &now
	}
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
func containsCredential(v any) bool {
	b, e := json.Marshal(v)
	return e != nil || credentialText.Match(b)
}
