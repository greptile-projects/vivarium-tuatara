// Package threatmodels retains revision-bound, collaborative security analysis.
package threatmodels

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

var ErrNotFound = errors.New("threat model not found")
var ErrInvalid = errors.New("invalid threat model")
var ErrConflict = errors.New("threat model version conflict")

type Source struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Summary    string `json:"summary"`
}
type EntryPoint struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Access   string `json:"access"`
	Boundary string `json:"boundary"`
}
type Privilege struct {
	ID         string `json:"id"`
	Principal  string `json:"principal"`
	Capability string `json:"capability"`
	Scope      string `json:"scope"`
}
type DataFlow struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	To         string `json:"to"`
	Data       string `json:"data"`
	Protection string `json:"protection"`
}
type Dependency struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Revision string `json:"revision"`
	Trust    string `json:"trust"`
}
type AbusePath struct {
	ID            string   `json:"id"`
	Goal          string   `json:"attacker_goal"`
	EntryPointIDs []string `json:"entry_point_ids"`
	PrivilegeIDs  []string `json:"privilege_ids"`
	DataFlowIDs   []string `json:"data_flow_ids"`
	DependencyIDs []string `json:"dependency_ids"`
	Steps         []string `json:"steps"`
	Impact        string   `json:"impact"`
	MitigationIDs []string `json:"mitigation_ids"`
	ResidualRisk  string   `json:"residual_risk"`
	OwnerIDs      []string `json:"owner_ids"`
}
type Mitigation struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	EvidenceIDs []string `json:"evidence_ids"`
	OwnerIDs    []string `json:"owner_ids"`
}
type Evidence struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Summary    string `json:"summary"`
	Accessible bool   `json:"accessible"`
	Gap        string `json:"gap,omitempty"`
}
type Alternative struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	SecurityEffect string   `json:"security_effect"`
	Tradeoffs      []string `json:"tradeoffs"`
	EvidenceIDs    []string `json:"evidence_ids"`
}
type Revision struct {
	Version             int           `json:"version"`
	Title               string        `json:"title"`
	Source              Source        `json:"source"`
	ArchitectureDigest  string        `json:"architecture_digest"`
	TrustBoundaryDigest string        `json:"trust_boundary_digest"`
	EntryPoints         []EntryPoint  `json:"entry_points"`
	Privileges          []Privilege   `json:"privileges"`
	DataFlows           []DataFlow    `json:"data_flows"`
	Dependencies        []Dependency  `json:"dependencies"`
	AbusePaths          []AbusePath   `json:"abuse_paths"`
	Mitigations         []Mitigation  `json:"mitigations"`
	Evidence            []Evidence    `json:"evidence"`
	Alternatives        []Alternative `json:"alternatives"`
	OwnerIDs            []string      `json:"owner_ids"`
	Assumptions         []string      `json:"assumptions"`
	CreatedBy           string        `json:"created_by"`
	CreatedAt           time.Time     `json:"created_at"`
}
type Event struct {
	ID                string    `json:"id"`
	ModelVersion      int       `json:"model_version"`
	Kind              string    `json:"kind"`
	Body              string    `json:"body"`
	ResourceIDs       []string  `json:"resource_ids"`
	EvidenceIDs       []string  `json:"evidence_ids"`
	AlternativeIDs    []string  `json:"alternative_ids,omitempty"`
	RequestedOwnerIDs []string  `json:"requested_owner_ids,omitempty"`
	ActorID           string    `json:"actor_id"`
	ActorType         string    `json:"actor_type"`
	CreatedAt         time.Time `json:"created_at"`
}
type Acknowledgement struct {
	ModelVersion int       `json:"model_version"`
	OwnerID      string    `json:"owner_id"`
	Decision     string    `json:"decision"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}
type Freshness struct {
	Fresh   bool     `json:"fresh"`
	Reasons []string `json:"reasons"`
}
type Model struct {
	ID               string            `json:"id"`
	RepositoryID     string            `json:"repository_id"`
	CurrentVersion   int               `json:"current_version"`
	Revisions        []Revision        `json:"revisions"`
	Events           []Event           `json:"events"`
	Acknowledgements []Acknowledgement `json:"acknowledgements"`
	Freshness        Freshness         `json:"freshness"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}
type CurrentSource struct {
	Revision             string
	ArchitectureDigest   string
	TrustBoundaryDigest  string
	DependencyRevisions  map[string]string
	PermittedEvidenceIDs map[string]bool
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
func (s *Store) Create(repo, actor string, r Revision) (Model, error) {
	var out Model
	err := s.lock(func() error {
		if validate(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Model{ID: id(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, Events: []Event{}, Acknowledgements: []Acknowledgement{}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return project(out, CurrentSource{}), err
}
func (s *Store) Revise(repo, model string, expected int, actor string, r Revision) (Model, error) {
	var out Model
	err := s.lock(func() error {
		v, e := s.read(model)
		if e != nil {
			return e
		}
		if v.RepositoryID != repo {
			return ErrNotFound
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
	return project(out, CurrentSource{}), err
}
func (s *Store) AddEvent(repo, model string, expected int, actor, actorType string, e Event) (Model, error) {
	var out Model
	err := s.lock(func() error {
		v, x := s.read(model)
		if x != nil {
			return x
		}
		if v.RepositoryID != repo {
			return ErrNotFound
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		r := v.Revisions[len(v.Revisions)-1]
		if validateEvent(e, r, actorType) != nil {
			return ErrInvalid
		}
		e.ID = id()
		e.ModelVersion = expected
		e.ActorID = actor
		e.ActorType = actorType
		e.CreatedAt = s.now()
		v.Events = append(v.Events, e)
		v.UpdatedAt = e.CreatedAt
		out = v
		return s.write(v)
	})
	return project(out, CurrentSource{}), err
}
func (s *Store) Acknowledge(repo, model string, version int, actor string, a Acknowledgement) (Model, error) {
	var out Model
	err := s.lock(func() error {
		v, x := s.read(model)
		if x != nil {
			return x
		}
		if v.RepositoryID != repo {
			return ErrNotFound
		}
		r := v.Revisions[len(v.Revisions)-1]
		if version != v.CurrentVersion || !contains(r.OwnerIDs, actor) || a.OwnerID != actor || !one(a.Decision, "acknowledged", "changes_requested") || strings.TrimSpace(a.Note) == "" {
			return ErrInvalid
		}
		a.ModelVersion = version
		a.CreatedAt = s.now()
		v.Acknowledgements = append(v.Acknowledgements, a)
		v.UpdatedAt = a.CreatedAt
		out = v
		return s.write(v)
	})
	return project(out, CurrentSource{}), err
}
func (s *Store) Get(repo, model string, current CurrentSource) (Model, error) {
	var out Model
	err := s.lock(func() error {
		var e error
		out, e = s.read(model)
		if e == nil && out.RepositoryID != repo {
			return ErrNotFound
		}
		return e
	})
	return project(out, current), err
}
func (s *Store) List(repo string, current func(Revision) (CurrentSource, error)) ([]Model, error) {
	out := []Model{}
	err := s.lock(func() error {
		xs, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, x := range xs {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			v, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
			if e != nil {
				return e
			}
			if v.RepositoryID == repo {
				c, _ := current(v.Revisions[len(v.Revisions)-1])
				out = append(out, project(v, c))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}
func stamp(r *Revision, v int, actor string, now time.Time) {
	r.Version = v
	r.CreatedBy = actor
	r.CreatedAt = now
}
func validate(r Revision) error {
	if r.Title == "" || !one(r.Source.Kind, "design_proposal", "pull_request", "api_evolution", "schema_evolution", "infrastructure_plan", "product_experiment") || r.Source.ResourceID == "" || r.Source.Revision == "" || r.Source.Summary == "" || r.ArchitectureDigest == "" || r.TrustBoundaryDigest == "" || len(r.EntryPoints) == 0 || len(r.Privileges) == 0 || len(r.DataFlows) == 0 || len(r.AbusePaths) == 0 || len(r.Mitigations) == 0 || len(r.Evidence) == 0 || len(r.Alternatives) == 0 || len(r.OwnerIDs) == 0 || len(r.Assumptions) == 0 {
		return ErrInvalid
	}
	ep, pr, df, de, mi, ev, al := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, x := range r.EntryPoints {
		if x.ID == "" || x.Name == "" || x.Access == "" || x.Boundary == "" || ep[x.ID] {
			return ErrInvalid
		}
		ep[x.ID] = true
	}
	for _, x := range r.Privileges {
		if x.ID == "" || x.Principal == "" || x.Capability == "" || x.Scope == "" || pr[x.ID] {
			return ErrInvalid
		}
		pr[x.ID] = true
	}
	for _, x := range r.DataFlows {
		if x.ID == "" || x.From == "" || x.To == "" || x.Data == "" || x.Protection == "" || df[x.ID] {
			return ErrInvalid
		}
		df[x.ID] = true
	}
	for _, x := range r.Dependencies {
		if x.ID == "" || x.Name == "" || x.Revision == "" || x.Trust == "" || de[x.ID] {
			return ErrInvalid
		}
		de[x.ID] = true
	}
	for _, x := range r.Mitigations {
		if x.ID == "" || x.Description == "" || !one(x.Status, "proposed", "accepted", "implemented", "rejected") || len(x.OwnerIDs) == 0 || mi[x.ID] {
			return ErrInvalid
		}
		mi[x.ID] = true
	}
	for _, x := range r.Evidence {
		if x.ID == "" || x.Kind == "" || x.ResourceID == "" || x.Revision == "" || x.Summary == "" || (!x.Accessible && x.Gap == "") || ev[x.ID] {
			return ErrInvalid
		}
		ev[x.ID] = true
	}
	for _, x := range r.Alternatives {
		if x.ID == "" || x.Name == "" || x.Description == "" || x.SecurityEffect == "" || len(x.Tradeoffs) == 0 || al[x.ID] {
			return ErrInvalid
		}
		al[x.ID] = true
	}
	for _, x := range r.AbusePaths {
		if x.ID == "" || x.Goal == "" || len(x.EntryPointIDs) == 0 || len(x.Steps) == 0 || x.Impact == "" || len(x.MitigationIDs) == 0 || x.ResidualRisk == "" || len(x.OwnerIDs) == 0 {
			return ErrInvalid
		}
		for _, id := range x.EntryPointIDs {
			if !ep[id] {
				return ErrInvalid
			}
		}
		for _, id := range x.PrivilegeIDs {
			if !pr[id] {
				return ErrInvalid
			}
		}
		for _, id := range x.DataFlowIDs {
			if !df[id] {
				return ErrInvalid
			}
		}
		for _, id := range x.DependencyIDs {
			if !de[id] {
				return ErrInvalid
			}
		}
		for _, id := range x.MitigationIDs {
			if !mi[id] {
				return ErrInvalid
			}
		}
	}
	for _, x := range r.Mitigations {
		for _, id := range x.EvidenceIDs {
			if !ev[id] {
				return ErrInvalid
			}
		}
	}
	for _, x := range r.Alternatives {
		for _, id := range x.EvidenceIDs {
			if !ev[id] {
				return ErrInvalid
			}
		}
	}
	return nil
}
func validateEvent(e Event, r Revision, actorType string) error {
	if !one(actorType, "human", "agent") || !one(e.Kind, "finding", "challenge", "alternative_comparison", "acknowledgement_request") || strings.TrimSpace(e.Body) == "" || len(e.EvidenceIDs) == 0 {
		return ErrInvalid
	}
	evidence := map[string]bool{}
	resources := map[string]bool{}
	alternatives := map[string]bool{}
	owners := map[string]bool{}
	for _, x := range r.Evidence {
		evidence[x.ID] = x.Accessible
	}
	for _, x := range r.EntryPoints {
		resources[x.ID] = true
	}
	for _, x := range r.Privileges {
		resources[x.ID] = true
	}
	for _, x := range r.DataFlows {
		resources[x.ID] = true
	}
	for _, x := range r.Dependencies {
		resources[x.ID] = true
	}
	for _, x := range r.AbusePaths {
		resources[x.ID] = true
	}
	for _, x := range r.Mitigations {
		resources[x.ID] = true
	}
	for _, x := range r.Alternatives {
		alternatives[x.ID] = true
	}
	for _, x := range r.OwnerIDs {
		owners[x] = true
	}
	for _, id := range e.EvidenceIDs {
		if !evidence[id] {
			return ErrInvalid
		}
	}
	for _, id := range e.ResourceIDs {
		if !resources[id] {
			return ErrInvalid
		}
	}
	for _, id := range e.AlternativeIDs {
		if !alternatives[id] {
			return ErrInvalid
		}
	}
	for _, id := range e.RequestedOwnerIDs {
		if !owners[id] {
			return ErrInvalid
		}
	}
	if e.Kind == "alternative_comparison" && len(e.AlternativeIDs) < 2 {
		return ErrInvalid
	}
	if e.Kind == "acknowledgement_request" && len(e.RequestedOwnerIDs) == 0 {
		return ErrInvalid
	}
	return nil
}
func project(v Model, c CurrentSource) Model {
	v.Freshness = Freshness{Fresh: true, Reasons: []string{}}
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	// Reader projections retain only the explicit existence and gap for evidence
	// the model author could see but the current repository audience may not.
	for revisionIndex := range v.Revisions {
		for evidenceIndex := range v.Revisions[revisionIndex].Evidence {
			evidence := &v.Revisions[revisionIndex].Evidence[evidenceIndex]
			if !evidence.Accessible || !c.PermittedEvidenceIDs[evidence.ID] {
				evidence.Accessible = false
				evidence.Kind = "restricted_gap"
				evidence.ResourceID = ""
				evidence.Revision = ""
				evidence.Summary = ""
			}
		}
	}
	add := func(reason string) {
		v.Freshness.Fresh = false
		v.Freshness.Reasons = append(v.Freshness.Reasons, reason)
	}
	if c.Revision == "" {
		add("source unavailable or no longer permitted")
		return v
	}
	if c.Revision != r.Source.Revision {
		add("source revision changed")
	}
	if c.ArchitectureDigest != "" && c.ArchitectureDigest != r.ArchitectureDigest {
		add("architecture changed")
	}
	if c.TrustBoundaryDigest != "" && c.TrustBoundaryDigest != r.TrustBoundaryDigest {
		add("trust boundaries changed")
	}
	for _, d := range r.Dependencies {
		if x, ok := c.DependencyRevisions[d.ID]; ok && x != d.Revision {
			add("dependency " + d.ID + " changed")
		}
	}
	return v
}
func one(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
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
func id() string                      { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(v string) string { return filepath.Join(s.root, v+".json") }
func (s *Store) read(v string) (Model, error) {
	if strings.ContainsAny(v, "/\\") {
		return Model{}, ErrNotFound
	}
	b, e := os.ReadFile(s.path(v))
	if os.IsNotExist(e) {
		return Model{}, ErrNotFound
	}
	if e != nil {
		return Model{}, e
	}
	var out Model
	if json.Unmarshal(b, &out) != nil || out.ID != v {
		return Model{}, ErrInvalid
	}
	return out, nil
}
func (s *Store) write(v Model) error {
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
	}
	return e
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
