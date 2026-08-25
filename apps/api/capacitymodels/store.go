// Package capacitymodels persists inspectable, revision-exact capacity forecasts.
package capacitymodels

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("capacity model not found")
var ErrInvalid = errors.New("invalid capacity model")
var ErrConflict = errors.New("capacity model conflict")

type Window struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
type Evidence struct {
	ID                     string   `json:"id"`
	Kind                   string   `json:"kind"`
	Label                  string   `json:"label"`
	ResourceID             string   `json:"resource_id"`
	ReleaseID              string   `json:"release_id"`
	ReleaseRevision        string   `json:"release_revision"`
	Window                 Window   `json:"observation_window"`
	Sanitization           string   `json:"sanitization"`
	InstrumentationVersion string   `json:"instrumentation_version"`
	AudienceIDs            []string `json:"audience_ids,omitempty"`
	Anomalous              bool     `json:"anomalous"`
	AnomalyReason          string   `json:"anomaly_reason,omitempty"`
	AddedBy                string   `json:"added_by,omitempty"`
}
type Assumption struct {
	ID          string   `json:"id"`
	Statement   string   `json:"statement"`
	EvidenceIDs []string `json:"evidence_ids"`
	Confidence  float64  `json:"confidence"`
	AddedBy     string   `json:"added_by,omitempty"`
}
type Segment struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	DemandUnit  string   `json:"demand_unit"`
	Baseline    float64  `json:"baseline"`
	GrowthRate  float64  `json:"growth_rate"`
	EvidenceIDs []string `json:"evidence_ids"`
}
type Saturation struct {
	ID            string    `json:"id"`
	SegmentID     string    `json:"segment_id"`
	Resource      string    `json:"resource"`
	Limit         float64   `json:"limit"`
	Unit          string    `json:"unit"`
	ExpectedAt    time.Time `json:"expected_at"`
	LowerAt       time.Time `json:"lower_at"`
	UpperAt       time.Time `json:"upper_at"`
	EvidenceIDs   []string  `json:"evidence_ids"`
	AssumptionIDs []string  `json:"assumption_ids"`
	Explanation   string    `json:"explanation"`
}
type CostPoint struct {
	Demand     float64 `json:"demand"`
	Cost       float64 `json:"cost"`
	DemandUnit string  `json:"demand_unit"`
	Currency   string  `json:"currency"`
	Period     string  `json:"period"`
}
type Scenario struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	DemandMultiplier float64  `json:"demand_multiplier"`
	AssumptionIDs    []string `json:"assumption_ids"`
	SaturationIDs    []string `json:"saturation_ids"`
}
type Revision struct {
	RequestID        string       `json:"request_id"`
	Version          int          `json:"version"`
	Title            string       `json:"title"`
	Summary          string       `json:"summary"`
	ObjectiveID      string       `json:"objective_id"`
	ObjectiveVersion int          `json:"objective_version"`
	Evidence         []Evidence   `json:"evidence"`
	Assumptions      []Assumption `json:"assumptions"`
	Segments         []Segment    `json:"workload_segments"`
	Saturations      []Saturation `json:"saturation_points"`
	CostCurve        []CostPoint  `json:"cost_curve"`
	Scenarios        []Scenario   `json:"demand_scenarios"`
	Method           string       `json:"method"`
	CreatedBy        string       `json:"created_by"`
	CreatedAt        time.Time    `json:"created_at"`
}
type Event struct {
	RequestID         string    `json:"request_id"`
	Kind              string    `json:"kind"`
	Statement         string    `json:"statement"`
	EvidenceIDs       []string  `json:"evidence_ids"`
	SupersedesVersion int       `json:"supersedes_version,omitempty"`
	ActorType         string    `json:"actor_type"`
	ActorID           string    `json:"actor_id"`
	CreatedAt         time.Time `json:"created_at"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	ResourceID   string `json:"resource_id,omitempty"`
	AttributedTo string `json:"attributed_to,omitempty"`
}
type Model struct {
	ID             string       `json:"id"`
	RepositoryID   string       `json:"repository_id"`
	CurrentVersion int          `json:"current_version"`
	Revisions      []Revision   `json:"revisions"`
	Events         []Event      `json:"events"`
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
func (s *Store) Create(repositoryID, actorType, actorID, requestID string, r Revision) (Model, error) {
	var out Model
	err := s.lock(func() error {
		if blank(requestID) || validate(r) != nil {
			return ErrInvalid
		}
		id := stableID(repositoryID, actorID, requestID)
		digest := revisionDigest(r)
		if old, e := s.read(id); e == nil {
			if revisionDigest(old.Revisions[0]) != digest {
				return ErrConflict
			}
			out = old
			return nil
		}
		now := s.now()
		stamp(&r, actorID, requestID, 1, now)
		out = Model{ID: id, RepositoryID: repositoryID, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.project(out, actorID), err
}
func (s *Store) Revise(id string, expected int, actorID, requestID string, r Revision) (Model, error) {
	var out Model
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		digest := revisionDigest(r)
		for _, x := range v.Revisions {
			if x.RequestID == requestID {
				if revisionDigest(x) != digest {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if blank(requestID) || v.CurrentVersion != expected || validate(r) != nil {
			return ErrConflict
		}
		stamp(&r, actorID, requestID, expected+1, s.now())
		v.CurrentVersion++
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		return s.write(v)
	})
	return s.project(out, actorID), err
}
func (s *Store) AddEvent(id, actorType, actorID string, expected int, e Event) (Model, error) {
	var out Model
	err := s.lock(func() error {
		v, x := s.read(id)
		if x != nil {
			return x
		}
		for _, old := range v.Events {
			if old.RequestID == e.RequestID {
				if eventDigest(old) != eventDigest(e) {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if v.CurrentVersion != expected || blank(e.RequestID) || blank(e.Statement) || (e.Kind != "challenge" && e.Kind != "support" && e.Kind != "supersede") || e.Kind == "supersede" && (e.SupersedesVersion < 1 || e.SupersedesVersion > v.CurrentVersion) {
			return ErrInvalid
		}
		e.ActorType = actorType
		e.ActorID = actorID
		e.CreatedAt = s.now()
		v.Events = append(v.Events, e)
		v.UpdatedAt = e.CreatedAt
		out = v
		return s.write(v)
	})
	return s.project(out, actorID), err
}
func (s *Store) Get(id, viewer string) (Model, error) {
	var v Model
	err := s.lock(func() error { var e error; v, e = s.read(id); return e })
	return s.project(v, viewer), err
}
func (s *Store) List(repositoryID, viewer string) ([]Model, error) {
	xs := []Model{}
	err := s.lock(func() error {
		es, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, f := range es {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			v, e := s.read(strings.TrimSuffix(f.Name(), ".json"))
			if e != nil {
				return e
			}
			if v.RepositoryID == repositoryID {
				xs = append(xs, s.project(v, viewer))
			}
		}
		return nil
	})
	sort.Slice(xs, func(i, j int) bool { return xs[i].UpdatedAt.After(xs[j].UpdatedAt) })
	return xs, err
}
func stamp(r *Revision, actor, request string, version int, now time.Time) {
	r.RequestID = request
	r.Version = version
	r.CreatedBy = actor
	r.CreatedAt = now
	for i := range r.Evidence {
		r.Evidence[i].AddedBy = actor
	}
}
func (s *Store) project(v Model, viewer string) Model {
	if len(v.Revisions) == 0 {
		return v
	}
	d := []Diagnostic{}
	instrumentation := map[string]bool{}
	for ri := range v.Revisions {
		for ei := range v.Revisions[ri].Evidence {
			e := &v.Revisions[ri].Evidence[ei]
			instrumentation[e.InstrumentationVersion] = true
			if len(e.AudienceIDs) > 0 && !contains(e.AudienceIDs, viewer) {
				id := e.ID
				actor := e.AddedBy
				*e = Evidence{ID: id, Kind: e.Kind, Label: "Restricted evidence", Window: e.Window, ReleaseID: e.ReleaseID, ReleaseRevision: e.ReleaseRevision}
				d = append(d, Diagnostic{Kind: "inaccessible_evidence", Severity: "warning", Message: "Evidence details are outside this reader's audience.", ResourceID: id, AttributedTo: actor})
			}
			if e.Anomalous {
				d = append(d, Diagnostic{Kind: "anomalous_event", Severity: "warning", Message: "An anomalous observation remains included in the model.", ResourceID: e.ID, AttributedTo: e.AddedBy})
			}
		}
	}
	if len(instrumentation) > 1 {
		d = append(d, Diagnostic{Kind: "instrumentation_change", Severity: "warning", Message: "Evidence uses multiple instrumentation versions; comparability must be justified."})
	}
	latest := v.Revisions[len(v.Revisions)-1]
	visible := map[string]bool{}
	for _, e := range latest.Evidence {
		if e.ResourceID != "" {
			visible[e.ID] = true
		}
	}
	for _, a := range latest.Assumptions {
		for _, id := range a.EvidenceIDs {
			if !visible[id] {
				d = append(d, Diagnostic{Kind: "inaccessible_evidence", Severity: "warning", Message: "An assumption depends on unavailable evidence.", ResourceID: id, AttributedTo: a.AddedBy})
			}
		}
	}
	for _, e := range v.Events {
		if e.Kind == "challenge" {
			d = append(d, Diagnostic{Kind: "forecast_disagreement", Severity: "warning", Message: e.Statement, AttributedTo: e.ActorID})
		}
	}
	v.Diagnostics = d
	return v
}
func validate(r Revision) error {
	if blank(r.Title) || blank(r.ObjectiveID) || r.ObjectiveVersion < 1 || blank(r.Method) || len(r.Evidence) == 0 || len(r.Assumptions) == 0 || len(r.Segments) == 0 || len(r.Saturations) == 0 || len(r.CostCurve) < 2 || len(r.Scenarios) == 0 {
		return ErrInvalid
	}
	ids := map[string]bool{}
	kinds := map[string]bool{"usage": true, "performance": true, "reliability": true, "deployment": true, "infrastructure": true, "dependency": true, "experiment": true, "roadmap": true}
	for _, e := range r.Evidence {
		if blank(e.ID) || ids[e.ID] || !kinds[e.Kind] || blank(e.ResourceID) || blank(e.ReleaseID) || blank(e.ReleaseRevision) || !e.Window.End.After(e.Window.Start) || blank(e.Sanitization) || blank(e.InstrumentationVersion) {
			return ErrInvalid
		}
		ids[e.ID] = true
	}
	ass := map[string]bool{}
	for _, a := range r.Assumptions {
		if blank(a.ID) || ass[a.ID] || blank(a.Statement) || a.Confidence < 0 || a.Confidence > 1 || !finite(a.Confidence) {
			return ErrInvalid
		}
		ass[a.ID] = true
	}
	seg := map[string]bool{}
	for _, x := range r.Segments {
		if blank(x.ID) || seg[x.ID] || blank(x.Name) || blank(x.DemandUnit) || x.Baseline < 0 || !finite(x.GrowthRate) {
			return ErrInvalid
		}
		seg[x.ID] = true
	}
	sat := map[string]bool{}
	for _, x := range r.Saturations {
		if blank(x.ID) || sat[x.ID] || !seg[x.SegmentID] || blank(x.Resource) || x.Limit <= 0 || blank(x.Unit) || x.ExpectedAt.IsZero() || x.LowerAt.After(x.ExpectedAt) || x.UpperAt.Before(x.ExpectedAt) || blank(x.Explanation) {
			return ErrInvalid
		}
		sat[x.ID] = true
	}
	for _, x := range r.CostCurve {
		if x.Demand < 0 || x.Cost < 0 || blank(x.DemandUnit) || blank(x.Currency) || blank(x.Period) {
			return ErrInvalid
		}
	}
	for _, x := range r.Scenarios {
		if blank(x.ID) || blank(x.Name) || x.DemandMultiplier <= 0 {
			return ErrInvalid
		}
		for _, id := range x.SaturationIDs {
			if !sat[id] {
				return ErrInvalid
			}
		}
	}
	return nil
}
func blank(x string) bool   { return strings.TrimSpace(x) == "" }
func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func revisionDigest(r Revision) string {
	r.RequestID = ""
	r.Version = 0
	r.CreatedBy = ""
	r.CreatedAt = time.Time{}
	for i := range r.Evidence {
		r.Evidence[i].AddedBy = ""
	}
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func eventDigest(e Event) string {
	e.ActorID = ""
	e.ActorType = ""
	e.CreatedAt = time.Time{}
	b, _ := json.Marshal(e)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func stableID(a, b, c string) string {
	h := sha256.Sum256([]byte(a + "\x00" + b + "\x00" + c))
	return hex.EncodeToString(h[:16])
}
func (s *Store) read(id string) (Model, error) {
	var v Model
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Model) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.root, ".model-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	if x := f.Close(); e == nil {
		e = x
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
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
