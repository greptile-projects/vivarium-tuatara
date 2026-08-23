// Package collaborationworkflows retains repository-reviewed recurring collaboration definitions.
package collaborationworkflows

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrInvalid = errors.New("invalid collaboration workflow")
var ErrConflict = errors.New("collaboration workflow version conflict")
var ErrNotFound = errors.New("collaboration workflow not found")

type Trigger struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Event      string      `json:"event"`
	Conditions []Condition `json:"conditions,omitempty"`
	Inputs     []Input     `json:"inputs,omitempty"`
}
type Input struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Source   string `json:"source"`
}
type Condition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}
type Invocation struct {
	Kind       string   `json:"kind"`
	Action     string   `json:"action,omitempty"`
	WorkflowID string   `json:"workflow_id,omitempty"`
	AgentID    string   `json:"agent_id,omitempty"`
	Component  string   `json:"component,omitempty"`
	Authority  []string `json:"authority"`
	Emits      []string `json:"emits,omitempty"`
}
type Step struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Needs          []string          `json:"needs,omitempty"`
	Conditions     []Condition       `json:"conditions,omitempty"`
	Invocation     Invocation        `json:"invocation"`
	Inputs         map[string]string `json:"inputs,omitempty"`
	Outputs        []string          `json:"outputs,omitempty"`
	Retries        int               `json:"retries"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	BudgetActions  int               `json:"budget_actions"`
	OwnerIDs       []string          `json:"owner_ids"`
	Completion     []string          `json:"completion"`
}
type Policy struct {
	ID                string   `json:"id"`
	AllowActions      []string `json:"allow_actions,omitempty"`
	DenyActions       []string `json:"deny_actions,omitempty"`
	RequiredAuthority []string `json:"required_authority,omitempty"`
}
type Definition struct {
	Name          string    `json:"name"`
	Outcome       string    `json:"outcome"`
	Description   string    `json:"description"`
	OwnerIDs      []string  `json:"owner_ids"`
	Triggers      []Trigger `json:"triggers"`
	Steps         []Step    `json:"steps"`
	Outputs       []string  `json:"outputs"`
	Completion    []string  `json:"completion"`
	BudgetActions int       `json:"budget_actions"`
	Policies      []Policy  `json:"policies,omitempty"`
}
type Source struct {
	Revision string `json:"revision"`
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Message      string `json:"message"`
	AttributedTo string `json:"attributed_to"`
	StepID       string `json:"step_id,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
}
type Authority struct {
	StepID    string   `json:"step_id"`
	Principal string   `json:"principal"`
	Grants    []string `json:"grants"`
	Boundary  string   `json:"boundary"`
}
type Preview struct {
	Definition         Definition   `json:"definition"`
	Source             Source       `json:"source"`
	Subscriptions      []string     `json:"subscriptions"`
	EffectiveAuthority []Authority  `json:"effective_authority"`
	ExecutionOrder     [][]string   `json:"execution_order"`
	Diagnostics        []Diagnostic `json:"diagnostics"`
	Activatable        bool         `json:"activatable"`
}
type Revision struct {
	Version            int         `json:"version"`
	Definition         Definition  `json:"definition"`
	Source             Source      `json:"source"`
	Subscriptions      []string    `json:"subscriptions"`
	EffectiveAuthority []Authority `json:"effective_authority"`
	ExecutionOrder     [][]string  `json:"execution_order"`
	ActivatedBy        string      `json:"activated_by"`
	ActivatedAt        time.Time   `json:"activated_at"`
}
type Workflow struct {
	ID             string     `json:"id"`
	RepositoryID   string     `json:"repository_id"`
	CurrentVersion int        `json:"current_version"`
	Status         string     `json:"status"`
	Revisions      []Revision `json:"revisions"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
type ResourceCheck func(Invocation) (bool, string)
type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}

func (s *Store) Preview(repo string, d Definition, src Source, check ResourceCheck) Preview {
	p := Preview{Definition: d, Source: src, Subscriptions: []string{}, EffectiveAuthority: []Authority{}, ExecutionOrder: [][]string{}, Diagnostics: []Diagnostic{}}
	seen := map[string]bool{}
	stepByID := map[string]Step{}
	add := func(kind, msg, who, step, res string) {
		p.Diagnostics = append(p.Diagnostics, Diagnostic{Kind: kind, Message: msg, AttributedTo: who, StepID: step, ResourceID: res})
	}
	if strings.TrimSpace(d.Name) == "" || strings.TrimSpace(d.Outcome) == "" || len(d.OwnerIDs) == 0 || len(d.Triggers) == 0 || len(d.Steps) == 0 || len(d.Completion) == 0 || d.BudgetActions < 1 {
		add("incomplete_definition", "name, shared outcome, owners, triggers, steps, budget, and completion criteria are required", "configuration", "", "")
	}
	for _, t := range d.Triggers {
		if !oneOf(t.Kind, "repository_event", "schedule", "manual") || t.ID == "" || t.Event == "" {
			add("invalid_trigger", "trigger kind, id, and event must be typed", "configuration", "", t.ID)
		} else {
			p.Subscriptions = append(p.Subscriptions, t.Kind+":"+t.Event)
		}
		validateConditions(t.Conditions, func(m string) { add("invalid_condition", m, "configuration", "", t.ID) })
		for _, in := range t.Inputs {
			if in.Name == "" || !oneOf(in.Type, "string", "number", "boolean", "object") || in.Source == "" {
				add("invalid_input", "trigger inputs require a name, closed type, and event source", "configuration", "", t.ID)
			}
		}
	}
	for _, st := range d.Steps {
		if st.ID == "" || seen[st.ID] {
			add("invalid_graph", "step ids must be non-empty and unique", "configuration", st.ID, "")
		}
		seen[st.ID] = true
		stepByID[st.ID] = st
		if st.Name == "" || len(st.OwnerIDs) == 0 || len(st.Completion) == 0 || st.TimeoutSeconds < 1 || st.TimeoutSeconds > 86400 || st.Retries < 0 || st.Retries > 10 || st.BudgetActions < 1 {
			add("invalid_step", "each step requires a name, owners, completion criteria, bounded retries, timeout, and action budget", "configuration", st.ID, "")
		}
		if st.BudgetActions > d.BudgetActions {
			add("budget_conflict", "step budget exceeds the workflow budget", "configuration", st.ID, "")
		}
		validateConditions(st.Conditions, func(m string) { add("invalid_condition", m, "configuration", st.ID, "") })
	}
	for _, st := range d.Steps {
		for _, need := range st.Needs {
			if need == st.ID || !seen[need] {
				add("invalid_graph", "step dependency must name a different existing step", "configuration", st.ID, need)
			}
		}
		inv := st.Invocation
		resource := inv.Action + inv.Component + inv.WorkflowID + inv.AgentID
		if !oneOf(inv.Kind, "platform_action", "component", "agent", "workflow") || len(inv.Authority) == 0 {
			add("invalid_invocation", "step invocation kind and explicit authority are required", "configuration", st.ID, resource)
		}
		if inv.Kind == "workflow" && inv.WorkflowID == "self" {
			add("trigger_loop", "a workflow cannot invoke itself", "configuration", st.ID, inv.WorkflowID)
		}
		if check != nil {
			if ok, reason := check(inv); !ok {
				add("inaccessible_resource", reason, "resource_resolver", st.ID, resource)
			}
		}
		principal := "platform"
		if inv.Kind == "agent" {
			principal = "agent:" + inv.AgentID
		}
		p.EffectiveAuthority = append(p.EffectiveAuthority, Authority{StepID: st.ID, Principal: principal, Grants: unique(inv.Authority), Boundary: "only the selected invocation; workflow records grant no additional authority"})
	}
	order, cycle := levels(stepByID)
	p.ExecutionOrder = order
	if cycle {
		add("invalid_graph", "step dependencies contain a cycle", "configuration", "", "")
	}
	for _, pol := range d.Policies {
		for _, a := range pol.AllowActions {
			if contains(pol.DenyActions, a) {
				add("conflicting_policy", "policy both allows and denies "+a, pol.ID, "", a)
			}
		}
		for _, st := range d.Steps {
			if st.Invocation.Action != "" && contains(pol.DenyActions, st.Invocation.Action) {
				add("conflicting_policy", "step invokes action denied by policy", pol.ID, st.ID, st.Invocation.Action)
			}
			for _, req := range pol.RequiredAuthority {
				if !contains(st.Invocation.Authority, req) {
					add("conflicting_policy", "step lacks policy-required authority "+req, pol.ID, st.ID, req)
				}
			}
		}
	}
	for _, t := range d.Triggers {
		for _, st := range d.Steps {
			if contains(st.Invocation.Emits, t.Event) {
				add("trigger_loop", "a step emits an event subscribed to by the same workflow", "configuration", st.ID, t.Event)
			}
		}
	}
	sort.Strings(p.Subscriptions)
	p.Activatable = len(p.Diagnostics) == 0
	return p
}

func (s *Store) Create(repo, actor string, p Preview) (Workflow, error) {
	if !p.Activatable {
		return Workflow{}, ErrInvalid
	}
	var w Workflow
	err := s.lock(func() error {
		now := s.now()
		w = Workflow{ID: id(), RepositoryID: repo, CurrentVersion: 1, Status: "active", Revisions: []Revision{revision(1, actor, now, p)}, CreatedAt: now, UpdatedAt: now}
		return s.write(w)
	})
	return w, err
}
func (s *Store) Revise(id string, expected int, actor string, p Preview) (Workflow, error) {
	if !p.Activatable {
		return Workflow{}, ErrInvalid
	}
	var w Workflow
	err := s.lock(func() error {
		var e error
		w, e = s.read(id)
		if e != nil {
			return e
		}
		if w.CurrentVersion != expected {
			return ErrConflict
		}
		now := s.now()
		w.CurrentVersion++
		w.Revisions = append(w.Revisions, revision(w.CurrentVersion, actor, now, p))
		w.UpdatedAt = now
		return s.write(w)
	})
	return w, err
}
func (s *Store) Get(id string) (Workflow, error) {
	var w Workflow
	err := s.lock(func() error { var e error; w, e = s.read(id); return e })
	return w, err
}
func (s *Store) List(repo string) ([]Workflow, error) {
	out := []Workflow{}
	err := s.lock(func() error {
		es, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, x := range es {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			w, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
			if e != nil {
				return e
			}
			if w.RepositoryID == repo {
				out = append(out, w)
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}
func revision(v int, a string, t time.Time, p Preview) Revision {
	return Revision{Version: v, Definition: p.Definition, Source: p.Source, Subscriptions: p.Subscriptions, EffectiveAuthority: p.EffectiveAuthority, ExecutionOrder: p.ExecutionOrder, ActivatedBy: a, ActivatedAt: t}
}
func (s *Store) read(id string) (Workflow, error) {
	var w Workflow
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(e) {
		return w, ErrNotFound
	}
	if e != nil {
		return w, e
	}
	e = json.Unmarshal(b, &w)
	return w, e
}
func (s *Store) write(w Workflow) error {
	b, e := json.MarshalIndent(w, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".workflow-")
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
	if e != nil {
		return e
	}
	if e = os.Rename(name, filepath.Join(s.root, w.ID+".json")); e != nil {
		return e
	}
	d, e := os.Open(s.root)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func oneOf(v string, x ...string) bool {
	for _, a := range x {
		if v == a {
			return true
		}
	}
	return false
}
func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func unique(xs []string) []string {
	o := []string{}
	for _, x := range xs {
		if x != "" && !contains(o, x) {
			o = append(o, x)
		}
	}
	sort.Strings(o)
	return o
}
func validateConditions(cs []Condition, add func(string)) {
	for _, c := range cs {
		if c.Field == "" || c.Value == "" || !oneOf(c.Operator, "equals", "not_equals", "contains", "matches", "greater_than", "less_than") {
			add("conditions require a field, closed operator, and value")
		}
	}
}
func levels(steps map[string]Step) ([][]string, bool) {
	done := map[string]bool{}
	out := [][]string{}
	for len(done) < len(steps) {
		level := []string{}
		for id, s := range steps {
			if done[id] {
				continue
			}
			ready := true
			for _, n := range s.Needs {
				if !done[n] {
					ready = false
				}
			}
			if ready {
				level = append(level, id)
			}
		}
		if len(level) == 0 {
			return out, true
		}
		sort.Strings(level)
		out = append(out, level)
		for _, id := range level {
			done[id] = true
		}
	}
	return out, false
}
