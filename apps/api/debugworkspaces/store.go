// Package debugworkspaces persists shared, revision-exact starting context for production debugging.
package debugworkspaces

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

var ErrNotFound = errors.New("debugging workspace not found")
var ErrInvalid = errors.New("invalid debugging workspace")
var ErrConflict = errors.New("debugging workspace changed")
var ErrForbidden = errors.New("debugging workspace action forbidden")

type Reference struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Label      string `json:"label"`
}
type Evidence struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	Reference         string `json:"reference"`
	Label             string `json:"label"`
	Visibility        string `json:"visibility"`
	Sanitization      string `json:"sanitization"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}
type Hypothesis struct {
	ID        string    `json:"id"`
	Statement string    `json:"statement"`
	Status    string    `json:"status"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}
type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type ProbePolicy struct {
	DataCategories []string `json:"data_categories"`
	Privacy        string   `json:"privacy"`
	Security       string   `json:"security"`
	RetentionHours int      `json:"retention_hours"`
	SamplePercent  int      `json:"sample_percent"`
	MaxCostCents   int      `json:"max_cost_cents"`
	MaxLoadPercent int      `json:"max_load_percent"`
}
type ProbeArtifact struct {
	Kind      string `json:"kind"`
	Digest    string `json:"digest"`
	SizeBytes int64  `json:"size_bytes"`
	Reference string `json:"reference"`
	Redaction string `json:"redaction"`
}
type ProbeAction struct {
	ID              string          `json:"id"`
	ActorID         string          `json:"actor_id"`
	Outcome         string          `json:"outcome"`
	StartedAt       time.Time       `json:"started_at"`
	FinishedAt      time.Time       `json:"finished_at"`
	Provenance      string          `json:"provenance"`
	Transformations []string        `json:"transformations"`
	Gaps            []string        `json:"gaps"`
	Artifacts       []ProbeArtifact `json:"artifacts"`
	CreatedAt       time.Time       `json:"created_at"`
}
type Probe struct {
	ID                 string        `json:"id"`
	Version            int           `json:"version"`
	Kind               string        `json:"kind"`
	Purpose            string        `json:"purpose"`
	DefinitionPath     string        `json:"definition_path,omitempty"`
	DefinitionRevision string        `json:"definition_revision,omitempty"`
	AudienceUserIDs    []string      `json:"audience_user_ids"`
	RequestedPolicy    ProbePolicy   `json:"requested_policy"`
	ApprovedPolicy     *ProbePolicy  `json:"approved_policy,omitempty"`
	Status             string        `json:"status"`
	RequestedBy        string        `json:"requested_by"`
	RequestedAt        time.Time     `json:"requested_at"`
	DecidedBy          string        `json:"decided_by,omitempty"`
	DecisionReason     string        `json:"decision_reason,omitempty"`
	ApprovedAt         *time.Time    `json:"approved_at,omitempty"`
	ExpiresAt          time.Time     `json:"expires_at"`
	RevokedBy          string        `json:"revoked_by,omitempty"`
	RevokedAt          *time.Time    `json:"revoked_at,omitempty"`
	Actions            []ProbeAction `json:"actions"`
}
type Workspace struct {
	ID                 string       `json:"id"`
	RepositoryID       string       `json:"repository_id"`
	Version            int          `json:"version"`
	Title              string       `json:"title"`
	Summary            string       `json:"summary"`
	Trigger            Reference    `json:"trigger"`
	Release            Reference    `json:"release"`
	Environment        Reference    `json:"environment"`
	TimeStart          time.Time    `json:"time_start"`
	TimeEnd            time.Time    `json:"time_end"`
	UserJourney        string       `json:"user_journey"`
	OwnerIDs           []string     `json:"owner_ids"`
	Severity           string       `json:"severity"`
	Audience           string       `json:"audience"`
	AccessUserIDs      []string     `json:"access_user_ids"`
	Status             string       `json:"status"`
	Source             Reference    `json:"source"`
	Packages           []Reference  `json:"packages"`
	Configuration      Reference    `json:"configuration"`
	Infrastructure     Reference    `json:"infrastructure"`
	Evidence           []Evidence   `json:"permitted_evidence"`
	UnavailableContext []string     `json:"unavailable_context"`
	Hypotheses         []Hypothesis `json:"hypotheses"`
	History            []Event      `json:"history"`
	Probes             []Probe      `json:"probes"`
	CreatedBy          string       `json:"created_by"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
}
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
func (s *Store) Create(v Workspace, actor string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Workspace{}, err
	}
	defer unlock()
	if !valid(v, actor) {
		return Workspace{}, ErrInvalid
	}
	now := s.now()
	v.ID = id()
	v.Version = 1
	v.Status = "open"
	v.CreatedBy = actor
	v.CreatedAt = now
	v.UpdatedAt = now
	v.OwnerIDs = unique(v.OwnerIDs)
	v.AccessUserIDs = unique(v.AccessUserIDs)
	if v.Evidence == nil {
		v.Evidence = []Evidence{}
	}
	if v.Packages == nil {
		v.Packages = []Reference{}
	}
	if v.UnavailableContext == nil {
		v.UnavailableContext = []string{}
	}
	for i := range v.Evidence {
		v.Evidence[i].ID = id()
	}
	v.Hypotheses = []Hypothesis{}
	v.Probes = []Probe{}
	v.History = []Event{{ID: id(), Kind: "opened", ActorID: actor, To: "open", CreatedAt: now}}
	return v, s.write(v)
}

func (s *Store) RequestProbe(repo, wid, actor string, in Probe, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Workspace{}, err
	}
	defer unlock()
	v, err := s.read(repo, wid)
	if err != nil {
		return Workspace{}, err
	}
	if v.Version != expected {
		return Workspace{}, ErrConflict
	}
	now := s.now()
	if !contains(in.AudienceUserIDs, actor) || !validProbe(in, v, now) {
		return Workspace{}, ErrInvalid
	}
	in.ID, in.Version, in.Status, in.RequestedBy, in.RequestedAt = id(), 1, "pending", actor, now
	in.AudienceUserIDs = unique(in.AudienceUserIDs)
	in.RequestedPolicy.DataCategories = uniqueWords(in.RequestedPolicy.DataCategories)
	in.ApprovedPolicy, in.Actions = nil, []ProbeAction{}
	v.Probes = append(v.Probes, in)
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, Event{ID: id(), Kind: "probe_requested", ActorID: actor, To: in.ID, CreatedAt: now})
	return v, s.write(v)
}

func (s *Store) DecideProbe(repo, wid, pid, actor, decision, reason string, policy ProbePolicy, expires time.Time, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Workspace{}, err
	}
	defer unlock()
	v, err := s.read(repo, wid)
	if err != nil {
		return Workspace{}, err
	}
	if v.Version != expected {
		return Workspace{}, ErrConflict
	}
	if !contains(v.OwnerIDs, actor) {
		return Workspace{}, ErrForbidden
	}
	i := probeIndex(v.Probes, pid)
	if i < 0 {
		return Workspace{}, ErrNotFound
	}
	p, now := &v.Probes[i], s.now()
	if p.Status != "pending" || !one(decision, "approved", "denied") || strings.TrimSpace(reason) == "" {
		return Workspace{}, ErrInvalid
	}
	if decision == "approved" {
		policy.DataCategories = uniqueWords(policy.DataCategories)
		if !validPolicy(policy) || !narrower(policy, p.RequestedPolicy) || !expires.After(now) || expires.After(p.ExpiresAt) {
			return Workspace{}, ErrInvalid
		}
		p.ApprovedPolicy, p.ApprovedAt, p.ExpiresAt = &policy, &now, expires
	}
	p.Status, p.DecidedBy, p.DecisionReason, p.Version = decision, actor, strings.TrimSpace(reason), p.Version+1
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, Event{ID: id(), Kind: "probe_" + decision, ActorID: actor, To: pid, Message: p.DecisionReason, CreatedAt: now})
	return v, s.write(v)
}

func (s *Store) ReportProbe(repo, wid, pid, actor string, in ProbeAction, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Workspace{}, err
	}
	defer unlock()
	v, err := s.read(repo, wid)
	if err != nil {
		return Workspace{}, err
	}
	if v.Version != expected {
		return Workspace{}, ErrConflict
	}
	i := probeIndex(v.Probes, pid)
	if i < 0 {
		return Workspace{}, ErrNotFound
	}
	p, now := &v.Probes[i], s.now()
	if p.RequestedBy != actor {
		return Workspace{}, ErrForbidden
	}
	if p.Status != "approved" || !now.Before(p.ExpiresAt) {
		return Workspace{}, ErrInvalid
	}
	if !validAction(in, *p, now) {
		return Workspace{}, ErrInvalid
	}
	in.ID, in.ActorID, in.CreatedAt = id(), actor, now
	p.Actions = append(p.Actions, in)
	p.Version++
	if in.Outcome == "complete" {
		p.Status = "completed"
	} else {
		p.Status = in.Outcome
	}
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, Event{ID: id(), Kind: "probe_" + in.Outcome, ActorID: actor, To: pid, CreatedAt: now})
	return v, s.write(v)
}

func (s *Store) RevokeProbe(repo, wid, pid, actor, reason string, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Workspace{}, err
	}
	defer unlock()
	v, err := s.read(repo, wid)
	if err != nil {
		return Workspace{}, err
	}
	if v.Version != expected {
		return Workspace{}, ErrConflict
	}
	if !contains(v.OwnerIDs, actor) {
		return Workspace{}, ErrForbidden
	}
	i := probeIndex(v.Probes, pid)
	if i < 0 {
		return Workspace{}, ErrNotFound
	}
	p, now := &v.Probes[i], s.now()
	if !one(p.Status, "pending", "approved") || strings.TrimSpace(reason) == "" {
		return Workspace{}, ErrInvalid
	}
	p.Status, p.RevokedBy, p.RevokedAt, p.DecisionReason, p.Version = "revoked", actor, &now, strings.TrimSpace(reason), p.Version+1
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, Event{ID: id(), Kind: "probe_revoked", ActorID: actor, To: pid, Message: reason, CreatedAt: now})
	return v, s.write(v)
}
func (s *Store) Get(repo, wid string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo, wid)
}
func (s *Store) List(repo string) ([]Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.root, repo))
	if errors.Is(err, os.ErrNotExist) {
		return []Workspace{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Workspace{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		v, x := s.read(repo, strings.TrimSuffix(e.Name(), ".json"))
		if x != nil {
			return nil, x
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) Update(repo, wid, actor, kind, value, message string, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Workspace{}, err
	}
	defer unlock()
	v, err := s.read(repo, wid)
	if err != nil {
		return Workspace{}, err
	}
	if v.Version != expected {
		return Workspace{}, ErrConflict
	}
	now := s.now()
	ev := Event{ID: id(), Kind: kind, ActorID: actor, Message: strings.TrimSpace(message), CreatedAt: now}
	switch kind {
	case "status":
		if !one(value, "open", "investigating", "blocked", "resolved", "closed") {
			return Workspace{}, ErrInvalid
		}
		ev.From = v.Status
		ev.To = value
		v.Status = value
	case "hypothesis":
		if strings.TrimSpace(value) == "" || len(value) > 4000 {
			return Workspace{}, ErrInvalid
		}
		h := Hypothesis{ID: id(), Statement: strings.TrimSpace(value), Status: "proposed", CreatedBy: actor, CreatedAt: now}
		v.Hypotheses = append(v.Hypotheses, h)
		ev.To = h.ID
	default:
		return Workspace{}, ErrInvalid
	}
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, ev)
	return v, s.write(v)
}
func valid(v Workspace, actor string) bool {
	if !closed(v.RepositoryID) || !closed(actor) || strings.TrimSpace(v.Title) == "" || len(v.Title) > 200 || strings.TrimSpace(v.Summary) == "" || len(v.Summary) > 5000 || !one(v.Trigger.Kind, "issue", "incident", "support_thread", "deployment", "service_objective", "trace", "manual_observation") || strings.TrimSpace(v.Trigger.Label) == "" || !closed(v.Release.ResourceID) || len(v.Release.Revision) != 40 || !closed(v.Environment.ResourceID) || v.TimeStart.IsZero() || !v.TimeStart.Before(v.TimeEnd) || v.TimeEnd.Sub(v.TimeStart) > 31*24*time.Hour || strings.TrimSpace(v.UserJourney) == "" || len(v.OwnerIDs) == 0 || !one(v.Severity, "low", "medium", "high", "critical") || !one(v.Audience, "repository", "restricted") || len(v.Source.Revision) != 40 {
		return false
	}
	if v.Trigger.Kind != "manual_observation" && v.Trigger.Kind != "trace" && !closed(v.Trigger.ResourceID) {
		return false
	}
	for _, id := range append(append([]string{}, v.OwnerIDs...), v.AccessUserIDs...) {
		if !closed(id) {
			return false
		}
	}
	if v.Audience == "restricted" && len(v.AccessUserIDs) == 0 {
		return false
	}
	for _, e := range v.Evidence {
		if !one(e.Kind, "log", "trace", "metric", "profile", "snapshot", "report", "link", "observation") || strings.TrimSpace(e.Label) == "" || len(e.Reference) > 2000 || !one(e.Visibility, "repository", "restricted") || strings.TrimSpace(e.Sanitization) == "" || (e.Available && strings.TrimSpace(e.Reference) == "") || (!e.Available && strings.TrimSpace(e.UnavailableReason) == "") {
			return false
		}
	}
	return true
}
func (s *Store) read(repo, wid string) (Workspace, error) {
	var v Workspace
	if !closed(repo) || !closed(wid) {
		return v, ErrNotFound
	}
	b, err := os.ReadFile(filepath.Join(s.root, repo, wid+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if json.Unmarshal(b, &v) != nil || v.ID != wid || v.RepositoryID != repo {
		return Workspace{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) write(v Workspace) error {
	dir := filepath.Join(s.root, v.RepositoryID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".debug-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	cerr := tmp.Close()
	if err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(dir, v.ID+".json"))
	}
	if err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	cerr = d.Close()
	if err == nil {
		err = cerr
	}
	return err
}
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func closed(v string) bool {
	if len(v) != 32 {
		return false
	}
	b, e := hex.DecodeString(v)
	return e == nil && len(b) == 16 && v == strings.ToLower(v)
}
func one(v string, a ...string) bool {
	for _, x := range a {
		if v == x {
			return true
		}
	}
	return false
}
func unique(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		if closed(v) && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
func uniqueWords(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
func contains(in []string, wanted string) bool {
	for _, v := range in {
		if v == wanted {
			return true
		}
	}
	return false
}
func probeIndex(in []Probe, wanted string) int {
	for i := range in {
		if in[i].ID == wanted {
			return i
		}
	}
	return -1
}
func validProbe(p Probe, w Workspace, now time.Time) bool {
	if !one(p.Kind, "logs", "traces", "profile", "state_snapshot", "dynamic_diagnostic") || strings.TrimSpace(p.Purpose) == "" || len(p.Purpose) > 2000 || !validPolicy(p.RequestedPolicy) || !p.ExpiresAt.After(now) || p.ExpiresAt.After(now.Add(24*time.Hour)) || len(p.AudienceUserIDs) == 0 {
		return false
	}
	allowed := append(append([]string{}, w.OwnerIDs...), w.AccessUserIDs...)
	allowed = append(allowed, w.CreatedBy)
	for _, actor := range p.AudienceUserIDs {
		if !closed(actor) || (w.Audience == "restricted" && !contains(allowed, actor)) {
			return false
		}
	}
	if p.Kind == "dynamic_diagnostic" {
		path := strings.TrimSpace(p.DefinitionPath)
		if !strings.HasPrefix(path, ".vivarium/diagnostics/") || !strings.HasSuffix(path, ".json") || strings.Contains(path, "..") || p.DefinitionRevision != w.Source.Revision {
			return false
		}
	} else if p.DefinitionPath != "" || p.DefinitionRevision != "" {
		return false
	}
	return !sensitive(p.Purpose + p.DefinitionPath)
}
func validPolicy(p ProbePolicy) bool {
	if len(p.DataCategories) == 0 || strings.TrimSpace(p.Privacy) == "" || strings.TrimSpace(p.Security) == "" || p.RetentionHours < 1 || p.RetentionHours > 720 || p.SamplePercent < 1 || p.SamplePercent > 100 || p.MaxCostCents < 0 || p.MaxCostCents > 10000000 || p.MaxLoadPercent < 1 || p.MaxLoadPercent > 100 {
		return false
	}
	for _, c := range p.DataCategories {
		if !one(c, "application_logs", "request_metadata", "stack_traces", "timing_spans", "runtime_profile", "configuration_shape", "aggregate_state") {
			return false
		}
	}
	return len(p.Privacy) <= 2000 && len(p.Security) <= 2000 && !sensitive(p.Privacy+p.Security)
}
func narrower(a, requested ProbePolicy) bool {
	if a.Privacy != requested.Privacy || a.Security != requested.Security {
		return false
	}
	if a.RetentionHours > requested.RetentionHours || a.SamplePercent > requested.SamplePercent || a.MaxCostCents > requested.MaxCostCents || a.MaxLoadPercent > requested.MaxLoadPercent {
		return false
	}
	for _, c := range a.DataCategories {
		if !contains(requested.DataCategories, c) {
			return false
		}
	}
	return true
}
func validAction(a ProbeAction, p Probe, now time.Time) bool {
	if p.ApprovedAt == nil || !one(a.Outcome, "complete", "partial", "overloaded", "denied") || a.StartedAt.IsZero() || a.FinishedAt.Before(a.StartedAt) || a.FinishedAt.After(now) || a.StartedAt.Before(*p.ApprovedAt) || a.FinishedAt.After(p.ExpiresAt) || strings.TrimSpace(a.Provenance) == "" || len(a.Provenance) > 2000 {
		return false
	}
	if a.Outcome == "complete" && len(a.Gaps) > 0 {
		return false
	}
	if a.Outcome != "complete" && len(a.Gaps) == 0 {
		return false
	}
	if len(a.Artifacts) > 20 || len(a.Transformations) == 0 || sensitive(a.Provenance+strings.Join(a.Transformations, " ")+strings.Join(a.Gaps, " ")) {
		return false
	}
	for _, x := range a.Artifacts {
		if !one(x.Kind, "log", "trace", "profile", "snapshot", "diagnostic") || len(x.Digest) != 64 || !hexLower(x.Digest) || x.SizeBytes < 0 || x.SizeBytes > 100*1024*1024 || strings.TrimSpace(x.Reference) == "" || strings.TrimSpace(x.Redaction) == "" || sensitive(x.Reference+x.Redaction) {
			return false
		}
	}
	return true
}
func hexLower(v string) bool {
	b, e := hex.DecodeString(v)
	return e == nil && len(b) == 32 && v == strings.ToLower(v)
}
func sensitive(v string) bool {
	l := strings.ToLower(v)
	for _, marker := range []string{"bearer ", "password=", "secret=", "api_key=", "-----begin private key", "ghp_", "github_pat_", "sk-proj-", "xoxb-"} {
		if strings.Contains(l, marker) {
			return true
		}
	}
	return false
}
