// Package capacityobjectives persists versioned demand and scaling contracts.
package capacityobjectives

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

var ErrNotFound = errors.New("capacity objective not found")
var ErrInvalid = errors.New("invalid capacity objective")
var ErrConflict = errors.New("capacity objective version conflict")
var ErrCommitted = errors.New("capacity objective may have committed")

type Scope struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Name       string `json:"name"`
}
type Forecast struct {
	ID           string    `json:"id"`
	Segment      string    `json:"segment"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	Value        float64   `json:"value"`
	Unit         string    `json:"unit"`
	Confidence   string    `json:"confidence"`
	Evidence     []string  `json:"evidence"`
	AttributedTo string    `json:"attributed_to,omitempty"`
}
type TrafficShape struct {
	Name           string  `json:"name"`
	Pattern        string  `json:"pattern"`
	PeakMultiplier float64 `json:"peak_multiplier"`
	BurstDuration  string  `json:"burst_duration"`
}
type Seasonality struct {
	Name       string  `json:"name"`
	Window     string  `json:"window"`
	Multiplier float64 `json:"multiplier"`
	Rationale  string  `json:"rationale"`
}
type ServiceLevel struct {
	Name      string  `json:"name"`
	Indicator string  `json:"indicator"`
	Target    float64 `json:"target"`
	Unit      string  `json:"unit"`
	Window    string  `json:"window"`
}
type Threshold struct {
	Resource string  `json:"resource"`
	Signal   string  `json:"signal"`
	Warning  float64 `json:"warning"`
	Critical float64 `json:"critical"`
	Unit     string  `json:"unit"`
}
type DependencyLimit struct {
	Name       string  `json:"name"`
	ResourceID string  `json:"resource_id,omitempty"`
	Limit      float64 `json:"limit"`
	Unit       string  `json:"unit"`
	Signal     string  `json:"signal"`
}
type Region struct {
	Name        string  `json:"name"`
	DemandShare float64 `json:"demand_share"`
	Residency   string  `json:"residency,omitempty"`
}
type Budget struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Period   string  `json:"period"`
}
type LeadTime struct {
	Duration string `json:"duration"`
	Trigger  string `json:"trigger"`
}
type Criterion struct {
	Name      string `json:"name"`
	Condition string `json:"condition"`
	Evidence  string `json:"evidence"`
}
type Link struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Version    string `json:"version,omitempty"`
	Label      string `json:"label"`
	AddedBy    string `json:"added_by,omitempty"`
}
type Assumption struct {
	ID           string    `json:"id"`
	Statement    string    `json:"statement"`
	Evidence     []string  `json:"evidence"`
	ExpiresAt    time.Time `json:"expires_at"`
	AttributedTo string    `json:"attributed_to,omitempty"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	ResourceID   string `json:"resource_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type Revision struct {
	RequestID        string            `json:"request_id"`
	Version          int               `json:"version"`
	Title            string            `json:"title"`
	Summary          string            `json:"summary"`
	Scope            Scope             `json:"scope"`
	Forecasts        []Forecast        `json:"demand_forecasts"`
	TrafficShapes    []TrafficShape    `json:"traffic_shapes"`
	Seasonality      []Seasonality     `json:"seasonality"`
	ServiceLevels    []ServiceLevel    `json:"service_levels"`
	Thresholds       []Threshold       `json:"bottleneck_thresholds"`
	DependencyLimits []DependencyLimit `json:"dependency_limits"`
	Regions          []Region          `json:"regions"`
	OwnerIDs         []string          `json:"owner_ids"`
	Budget           Budget            `json:"budget"`
	LeadTime         LeadTime          `json:"lead_time"`
	SuccessCriteria  []Criterion       `json:"success_criteria"`
	RollbackCriteria []Criterion       `json:"rollback_criteria"`
	Links            []Link            `json:"links"`
	Assumptions      []Assumption      `json:"assumptions"`
	Rationale        string            `json:"rationale"`
	CreatedBy        string            `json:"created_by"`
	CreatedAt        time.Time         `json:"created_at"`
}
type Objective struct {
	ID             string       `json:"id"`
	RequestID      string       `json:"request_id"`
	RequestDigest  string       `json:"request_digest"`
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
func (s *Store) Create(repositoryID, actor, requestID string, r Revision) (Objective, error) {
	var out Objective
	err := s.lock(func() error {
		if blank(requestID) {
			return ErrInvalid
		}
		if validate(r) != nil {
			return ErrInvalid
		}
		digest := revisionDigest(r)
		id := stableID(repositoryID, actor, requestID)
		if existing, e := s.read(id); e == nil {
			if existing.RequestID != requestID || existing.RequestDigest != digest {
				return ErrConflict
			}
			out = existing
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		now := s.now()
		stamp(&r, actor)
		r.RequestID = requestID
		r.Version = 1
		r.CreatedAt = now
		out = Objective{ID: id, RequestID: requestID, RequestDigest: digest, RepositoryID: repositoryID, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.project(out), err
}
func (s *Store) Revise(id string, expected int, actor, requestID string, r Revision) (Objective, error) {
	var out Objective
	err := s.lock(func() error {
		if blank(requestID) {
			return ErrInvalid
		}
		v, e := s.read(id)
		if e != nil {
			return e
		}
		digest := revisionDigest(r)
		if v.CurrentVersion == expected+1 && len(v.Revisions) > 0 {
			latest := v.Revisions[len(v.Revisions)-1]
			if latest.RequestID == requestID && revisionDigest(latest) == digest {
				out = v
				return nil
			}
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validate(r) != nil {
			return ErrInvalid
		}
		stamp(&r, actor)
		r.RequestID = requestID
		r.Version = expected + 1
		r.CreatedAt = s.now()
		v.CurrentVersion = r.Version
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		return s.write(v)
	})
	return s.project(out), err
}
func (s *Store) Get(id string) (Objective, error) {
	var v Objective
	err := s.lock(func() error { var e error; v, e = s.read(id); return e })
	return s.project(v), err
}
func (s *Store) List(repositoryID string) ([]Objective, error) {
	values := []Objective{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			v, e := s.read(strings.TrimSuffix(entry.Name(), ".json"))
			if e != nil {
				return e
			}
			if v.RepositoryID == repositoryID {
				values = append(values, s.project(v))
			}
		}
		return nil
	})
	sort.Slice(values, func(i, j int) bool { return values[i].UpdatedAt.After(values[j].UpdatedAt) })
	return values, err
}

func stamp(r *Revision, actor string) {
	r.CreatedBy = actor
	for i := range r.Forecasts {
		r.Forecasts[i].AttributedTo = actor
	}
	for i := range r.Assumptions {
		r.Assumptions[i].AttributedTo = actor
	}
	for i := range r.Links {
		r.Links[i].AddedBy = actor
	}
}
func (s *Store) project(v Objective) Objective {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	signals := map[string]bool{}
	for _, t := range r.Thresholds {
		signals[t.Signal] = true
		if strings.TrimSpace(t.Signal) == "" {
			d = append(d, diag("missing_signal", "blocking", "A bottleneck threshold has no observable signal.", t.Resource, r.CreatedBy))
		}
	}
	for _, l := range r.DependencyLimits {
		signals[l.Signal] = true
		if strings.TrimSpace(l.Signal) == "" {
			d = append(d, diag("missing_signal", "blocking", "A dependency limit has no observable signal.", l.Name, r.CreatedBy))
		}
	}
	for _, f := range r.Forecasts {
		if len(f.Evidence) == 0 || f.Confidence == "unsupported" {
			d = append(d, diag("unsupported_forecast", "blocking", "The demand forecast lacks supporting evidence.", f.ID, f.AttributedTo))
		}
	}
	now := s.now()
	for _, a := range r.Assumptions {
		if len(a.Evidence) == 0 {
			d = append(d, diag("unsupported_assumption", "warning", "The assumption has no supporting evidence.", a.ID, a.AttributedTo))
		}
		if !a.ExpiresAt.After(now) {
			d = append(d, diag("expired_assumption", "blocking", "The assumption has expired.", a.ID, a.AttributedTo))
		} else if a.ExpiresAt.Before(now.Add(30 * 24 * time.Hour)) {
			d = append(d, diag("expiring_assumption", "warning", "The assumption expires within 30 days.", a.ID, a.AttributedTo))
		}
	}
	if len(v.Revisions) > 1 {
		previous := v.Revisions[len(v.Revisions)-2]
		old := map[string]string{}
		for _, l := range previous.Links {
			old[linkIdentity(l)] = l.ResourceID
		}
		for _, l := range r.Links {
			if id := old[linkIdentity(l)]; id != "" && id != l.ResourceID {
				d = append(d, diag("conflicting_commitment", "warning", "This linked commitment differs from the prior version.", l.ResourceID, l.AddedBy))
			}
		}
	}
	_ = signals
	v.Diagnostics = d
	return v
}
func diag(kind, severity, message, id, actor string) Diagnostic {
	return Diagnostic{Kind: kind, Severity: severity, Message: message, ResourceID: id, AttributedTo: actor}
}
func validate(r Revision) error {
	kinds := map[string]bool{"service": true, "api": true, "job": true, "workspace": true, "package_delivery": true, "user_journey": true}
	if !kinds[r.Scope.Kind] || blank(r.Title) || blank(r.Scope.Name) || len(r.Forecasts) == 0 || len(r.TrafficShapes) == 0 || len(r.ServiceLevels) == 0 || len(r.Thresholds) == 0 || len(r.DependencyLimits) == 0 || len(r.Regions) == 0 || len(r.OwnerIDs) == 0 || len(r.SuccessCriteria) == 0 || len(r.RollbackCriteria) == 0 || blank(r.LeadTime.Duration) || blank(r.LeadTime.Trigger) || !positive(r.Budget.Amount) || blank(r.Budget.Currency) || blank(r.Budget.Period) {
		return ErrInvalid
	}
	ids := map[string]bool{}
	for _, f := range r.Forecasts {
		if blank(f.ID) || ids[f.ID] || blank(f.Segment) || !f.End.After(f.Start) || !positive(f.Value) || blank(f.Unit) || (f.Confidence != "supported" && f.Confidence != "uncertain" && f.Confidence != "unsupported") {
			return ErrInvalid
		}
		ids[f.ID] = true
	}
	for _, x := range r.TrafficShapes {
		if blank(x.Name) || blank(x.Pattern) || x.PeakMultiplier < 1 || blank(x.BurstDuration) {
			return ErrInvalid
		}
	}
	for _, x := range r.Seasonality {
		if blank(x.Name) || blank(x.Window) || x.Multiplier <= 0 || blank(x.Rationale) {
			return ErrInvalid
		}
	}
	for _, x := range r.ServiceLevels {
		if blank(x.Name) || blank(x.Indicator) || !positive(x.Target) || blank(x.Unit) || blank(x.Window) {
			return ErrInvalid
		}
	}
	for _, x := range r.Thresholds {
		if blank(x.Resource) || !finite(x.Warning) || !finite(x.Critical) || x.Warning < 0 || x.Critical <= x.Warning || blank(x.Unit) {
			return ErrInvalid
		}
	}
	for _, x := range r.DependencyLimits {
		if blank(x.Name) || !positive(x.Limit) || blank(x.Unit) {
			return ErrInvalid
		}
	}
	share := 0.0
	for _, x := range r.Regions {
		if blank(x.Name) || x.DemandShare < 0 || x.DemandShare > 1 {
			return ErrInvalid
		}
		share += x.DemandShare
	}
	if math.Abs(share-1) > 0.000001 {
		return ErrInvalid
	}
	for _, x := range append(append([]Criterion{}, r.SuccessCriteria...), r.RollbackCriteria...) {
		if blank(x.Name) || blank(x.Condition) || blank(x.Evidence) {
			return ErrInvalid
		}
	}
	validLinks := map[string]bool{"roadmap": true, "experiment": true, "performance_goal": true, "service_objective": true, "infrastructure": true, "release": true, "funding": true}
	linkIDs := map[string]bool{}
	for _, x := range r.Links {
		identity := linkIdentity(x)
		if !validLinks[x.Kind] || blank(x.ResourceID) || blank(x.Label) || linkIDs[identity] {
			return ErrInvalid
		}
		linkIDs[identity] = true
	}
	for _, x := range r.Assumptions {
		if blank(x.ID) || blank(x.Statement) || x.ExpiresAt.IsZero() {
			return ErrInvalid
		}
	}
	return nil
}
func blank(v string) bool        { return strings.TrimSpace(v) == "" }
func finite(v float64) bool      { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func positive(v float64) bool    { return finite(v) && v > 0 }
func linkIdentity(v Link) string { return v.Kind + "\x00" + v.Label }
func revisionDigest(v Revision) string {
	v.Version = 0
	v.RequestID = ""
	v.CreatedBy = ""
	v.CreatedAt = time.Time{}
	for i := range v.Forecasts {
		v.Forecasts[i].AttributedTo = ""
	}
	for i := range v.Assumptions {
		v.Assumptions[i].AttributedTo = ""
	}
	for i := range v.Links {
		v.Links[i].AddedBy = ""
	}
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func stableID(repositoryID, actor, requestID string) string {
	sum := sha256.Sum256([]byte(repositoryID + "\x00" + actor + "\x00" + requestID))
	return hex.EncodeToString(sum[:16])
}
func (s *Store) read(id string) (Objective, error) {
	var v Objective
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
func (s *Store) write(v Objective) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".capacity-")
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
	closeErr := tmp.Close()
	if e == nil {
		e = closeErr
	}
	renamed := false
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
		renamed = e == nil
	}
	if e == nil {
		dir, x := os.Open(s.root)
		if x != nil {
			return x
		}
		e = dir.Sync()
		_ = dir.Close()
	}
	if e != nil && renamed {
		return errors.Join(ErrCommitted, e)
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
