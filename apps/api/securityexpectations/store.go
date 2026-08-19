// Package securityexpectations persists versioned repository security intent.
package securityexpectations

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

var ErrNotFound = errors.New("security expectation not found")
var ErrInvalid = errors.New("invalid security expectation")
var ErrConflict = errors.New("security expectation version conflict")

type Scope struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Name       string `json:"name"`
}
type Asset struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Classification string   `json:"classification"`
	Protection     string   `json:"protection"`
	OwnerIDs       []string `json:"owner_ids"`
}
type Boundary struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Direction     string   `json:"direction"`
	AssetIDs      []string `json:"asset_ids"`
	Guarantees    []string `json:"guarantees"`
	ConflictsWith []string `json:"conflicts_with,omitempty"`
}
type Actor struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Trust        string   `json:"trust"`
	Capabilities []string `json:"capabilities"`
}
type AbuseCase struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	ActorIDs    []string `json:"actor_ids"`
	AssetIDs    []string `json:"asset_ids"`
	BoundaryIDs []string `json:"boundary_ids"`
	Scenario    string   `json:"scenario"`
	Impact      string   `json:"impact"`
	Severity    string   `json:"severity"`
	ControlIDs  []string `json:"control_ids"`
	OwnerIDs    []string `json:"owner_ids"`
}
type Control struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Requirement string   `json:"requirement"`
	Kind        string   `json:"kind"`
	OwnerIDs    []string `json:"owner_ids"`
	Evidence    string   `json:"evidence,omitempty"`
	Status      string   `json:"status"`
}
type SeverityPolicy struct {
	Level       string `json:"level"`
	Response    string `json:"response"`
	ReleaseRule string `json:"release_rule"`
}
type Link struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision,omitempty"`
	Summary    string `json:"summary"`
}
type Exception struct {
	ID          string    `json:"id"`
	AbuseCaseID string    `json:"abuse_case_id"`
	Rationale   string    `json:"rationale"`
	GrantedBy   string    `json:"granted_by"`
	ExpiresAt   time.Time `json:"expires_at"`
	FollowUp    string    `json:"follow_up"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	ResourceID   string `json:"resource_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type Revision struct {
	Version        int              `json:"version"`
	Title          string           `json:"title"`
	Summary        string           `json:"summary"`
	Scopes         []Scope          `json:"scopes"`
	Assets         []Asset          `json:"protected_assets"`
	Boundaries     []Boundary       `json:"trust_boundaries"`
	Actors         []Actor          `json:"actors"`
	AbuseCases     []AbuseCase      `json:"abuse_cases"`
	Controls       []Control        `json:"required_controls"`
	SeverityPolicy []SeverityPolicy `json:"severity_policy"`
	Links          []Link           `json:"commitment_links"`
	Exceptions     []Exception      `json:"exceptions"`
	OwnerIDs       []string         `json:"owner_ids"`
	Rationale      string           `json:"rationale"`
	CreatedBy      string           `json:"created_by"`
	CreatedAt      time.Time        `json:"created_at"`
}
type Expectation struct {
	ID             string       `json:"id"`
	RepositoryID   string       `json:"repository_id"`
	CurrentVersion int          `json:"current_version"`
	Revisions      []Revision   `json:"revisions"`
	Diagnostics    []Diagnostic `json:"diagnostics"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
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
func (s *Store) Create(repo, actor string, r Revision) (Expectation, error) {
	var out Expectation
	err := s.lock(func() error {
		if validate(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Expectation{ID: id(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.project(out), err
}
func (s *Store) Revise(expectationID string, expected int, actor string, r Revision) (Expectation, error) {
	var out Expectation
	err := s.lock(func() error {
		v, e := s.read(expectationID)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validate(r) != nil {
			return ErrInvalid
		}
		stamp(&r, expected+1, actor, s.now())
		v.CurrentVersion = r.Version
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		return s.write(v)
	})
	return s.project(out), err
}
func (s *Store) Get(expectationID string) (Expectation, error) {
	var out Expectation
	err := s.lock(func() error { var e error; out, e = s.read(expectationID); return e })
	return s.project(out), err
}
func (s *Store) List(repo string) ([]Expectation, error) {
	out := []Expectation{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, x := range entries {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			v, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
			if e != nil {
				return e
			}
			if v.RepositoryID == repo {
				out = append(out, s.project(v))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}
func stamp(r *Revision, version int, actor string, now time.Time) {
	r.Version = version
	r.CreatedBy = actor
	r.CreatedAt = now
}

func validate(r Revision) error {
	if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Summary) == "" || strings.TrimSpace(r.Rationale) == "" || len(r.Scopes) == 0 || len(r.Assets) == 0 || len(r.Boundaries) == 0 || len(r.Actors) == 0 || len(r.AbuseCases) == 0 || len(r.Controls) == 0 || len(r.SeverityPolicy) == 0 {
		return ErrInvalid
	}
	scopeKinds := set("repository", "service", "interface", "package", "extension", "environment", "journey")
	for _, x := range r.Scopes {
		if !scopeKinds[x.Kind] || strings.TrimSpace(x.Name) == "" || (x.Kind != "repository" && strings.TrimSpace(x.ResourceID) == "") {
			return ErrInvalid
		}
	}
	assets := map[string]bool{}
	for _, x := range r.Assets {
		if strings.TrimSpace(x.ID) == "" || assets[x.ID] || strings.TrimSpace(x.Name) == "" || strings.TrimSpace(x.Classification) == "" || strings.TrimSpace(x.Protection) == "" {
			return ErrInvalid
		}
		assets[x.ID] = true
	}
	boundaries := map[string]bool{}
	for _, x := range r.Boundaries {
		if strings.TrimSpace(x.ID) == "" || boundaries[x.ID] || strings.TrimSpace(x.Name) == "" || strings.TrimSpace(x.From) == "" || strings.TrimSpace(x.To) == "" || !set("inbound", "outbound", "bidirectional")[x.Direction] || len(x.Guarantees) == 0 {
			return ErrInvalid
		}
		boundaries[x.ID] = true
		for _, a := range x.AssetIDs {
			if !assets[a] {
				return ErrInvalid
			}
		}
	}
	for _, x := range r.Boundaries {
		for _, c := range x.ConflictsWith {
			if !boundaries[c] || c == x.ID {
				return ErrInvalid
			}
		}
	}
	actors := map[string]bool{}
	for _, x := range r.Actors {
		if strings.TrimSpace(x.ID) == "" || actors[x.ID] || strings.TrimSpace(x.Name) == "" || !set("human", "agent", "service", "external", "attacker")[x.Kind] || !set("trusted", "partially_trusted", "untrusted")[x.Trust] || len(x.Capabilities) == 0 {
			return ErrInvalid
		}
		actors[x.ID] = true
	}
	controls := map[string]bool{}
	for _, x := range r.Controls {
		if strings.TrimSpace(x.ID) == "" || controls[x.ID] || strings.TrimSpace(x.Name) == "" || strings.TrimSpace(x.Requirement) == "" || !set("prevent", "detect", "respond", "recover")[x.Kind] || !set("supported", "planned", "unsupported")[x.Status] {
			return ErrInvalid
		}
		controls[x.ID] = true
	}
	abuse := map[string]bool{}
	for _, x := range r.AbuseCases {
		if strings.TrimSpace(x.ID) == "" || abuse[x.ID] || strings.TrimSpace(x.Title) == "" || strings.TrimSpace(x.Scenario) == "" || strings.TrimSpace(x.Impact) == "" || !set("low", "medium", "high", "critical")[x.Severity] || len(x.ActorIDs) == 0 || len(x.AssetIDs) == 0 || len(x.BoundaryIDs) == 0 || len(x.ControlIDs) == 0 {
			return ErrInvalid
		}
		abuse[x.ID] = true
		for _, id := range x.ActorIDs {
			if !actors[id] {
				return ErrInvalid
			}
		}
		for _, id := range x.AssetIDs {
			if !assets[id] {
				return ErrInvalid
			}
		}
		for _, id := range x.BoundaryIDs {
			if !boundaries[id] {
				return ErrInvalid
			}
		}
		for _, id := range x.ControlIDs {
			if !controls[id] {
				return ErrInvalid
			}
		}
	}
	levels := map[string]bool{}
	for _, x := range r.SeverityPolicy {
		if !set("low", "medium", "high", "critical")[x.Level] || levels[x.Level] || strings.TrimSpace(x.Response) == "" || strings.TrimSpace(x.ReleaseRule) == "" {
			return ErrInvalid
		}
		levels[x.Level] = true
	}
	for _, x := range r.Links {
		if !set("design", "privacy", "infrastructure", "api", "quality", "release")[x.Kind] || strings.TrimSpace(x.ResourceID) == "" || strings.TrimSpace(x.Summary) == "" {
			return ErrInvalid
		}
	}
	exceptions := map[string]bool{}
	for _, x := range r.Exceptions {
		if strings.TrimSpace(x.ID) == "" || exceptions[x.ID] || !abuse[x.AbuseCaseID] || strings.TrimSpace(x.Rationale) == "" || strings.TrimSpace(x.GrantedBy) == "" || x.ExpiresAt.IsZero() || strings.TrimSpace(x.FollowUp) == "" {
			return ErrInvalid
		}
		exceptions[x.ID] = true
	}
	return nil
}
func (s *Store) project(v Expectation) Expectation {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	attr := r.CreatedBy
	if len(r.OwnerIDs) == 0 {
		d = append(d, Diagnostic{"missing_owner", "blocking", "The security expectation has no accountable owner.", "", attr})
	}
	for _, x := range r.Assets {
		if len(x.OwnerIDs) == 0 {
			d = append(d, Diagnostic{"missing_owner", "blocking", "Protected asset has no accountable owner.", x.ID, attr})
		}
	}
	for _, x := range r.AbuseCases {
		if len(x.OwnerIDs) == 0 {
			d = append(d, Diagnostic{"missing_owner", "blocking", "Abuse case has no accountable owner.", x.ID, attr})
		}
	}
	for _, x := range r.Controls {
		if len(x.OwnerIDs) == 0 {
			d = append(d, Diagnostic{"missing_owner", "blocking", "Required control has no accountable owner.", x.ID, attr})
		}
	}
	for _, x := range r.Boundaries {
		if len(x.ConflictsWith) > 0 {
			d = append(d, Diagnostic{"contradictory_boundary", "blocking", "Trust boundary explicitly conflicts with another retained boundary.", x.ID, attr})
		}
	}
	for _, x := range r.Controls {
		if x.Status == "unsupported" || (x.Status == "planned" && strings.TrimSpace(x.Evidence) == "") {
			d = append(d, Diagnostic{"unsupported_guarantee", "blocking", "A required control does not have current supporting evidence.", x.ID, attr})
		}
	}
	now := s.now()
	for _, x := range r.Exceptions {
		severity := "warning"
		message := "Security exception expires within seven days."
		if !x.ExpiresAt.After(now) {
			severity = "blocking"
			message = "Security exception has expired."
		} else if x.ExpiresAt.After(now.Add(7 * 24 * time.Hour)) {
			continue
		}
		d = append(d, Diagnostic{"expiring_exception", severity, message, x.AbuseCaseID, x.GrantedBy})
	}
	v.Diagnostics = d
	return v
}
func set(values ...string) map[string]bool {
	m := map[string]bool{}
	for _, v := range values {
		m[v] = true
	}
	return m
}
func id() string                      { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(v string) string { return filepath.Join(s.root, v+".json") }
func (s *Store) read(v string) (Expectation, error) {
	if strings.ContainsAny(v, "/\\") {
		return Expectation{}, ErrNotFound
	}
	b, e := os.ReadFile(s.path(v))
	if os.IsNotExist(e) {
		return Expectation{}, ErrNotFound
	}
	if e != nil {
		return Expectation{}, e
	}
	var out Expectation
	if json.Unmarshal(b, &out) != nil || out.ID != v {
		return Expectation{}, ErrInvalid
	}
	return out, nil
}
func (s *Store) write(v Expectation) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.path(v.ID) + ".tmp-" + id()
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	if e = os.Rename(tmp, s.path(v.ID)); e != nil {
		_ = os.Remove(tmp)
		return e
	}
	return nil
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
