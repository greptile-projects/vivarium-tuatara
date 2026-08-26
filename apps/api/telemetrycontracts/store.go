// Package telemetrycontracts persists immutable signal designs and attributable review.
package telemetrycontracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("telemetry contract not found")
var ErrInvalid = errors.New("invalid telemetry contract")
var ErrConflict = errors.New("telemetry contract conflict")

type Field struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description"`
	Sensitive   bool   `json:"sensitive"`
	Redaction   string `json:"redaction,omitempty"`
}
type Dimension struct {
	Name          string `json:"name"`
	Bounded       bool   `json:"bounded"`
	MaximumValues int    `json:"maximum_values,omitempty"`
	Source        string `json:"source"`
}
type SourceSymbol struct {
	RepositoryID string `json:"repository_id"`
	Revision     string `json:"revision"`
	Path         string `json:"path"`
	Symbol       string `json:"symbol"`
}
type Signal struct {
	ID                   string             `json:"id"`
	Name                 string             `json:"name"`
	Kind                 string             `json:"kind"`
	Description          string             `json:"description"`
	Unit                 string             `json:"unit,omitempty"`
	Fields               []Field            `json:"schema"`
	Dimensions           []Dimension        `json:"dimensions"`
	Sampling             string             `json:"sampling"`
	Aggregation          string             `json:"aggregation"`
	Correlation          []string           `json:"correlation"`
	RetentionDays        int                `json:"retention_days"`
	ExpectedEventsPerDay int64              `json:"expected_events_per_day"`
	QualityThresholds    map[string]float64 `json:"quality_thresholds"`
	Collector            string             `json:"collector"`
	SourceSymbols        []SourceSymbol     `json:"source_symbols"`
	ServiceBoundaries    []string           `json:"service_boundaries"`
}
type Impact struct {
	Privacy            string   `json:"privacy"`
	Security           string   `json:"security"`
	Residency          string   `json:"residency"`
	Performance        string   `json:"performance"`
	Cardinality        int64    `json:"estimated_cardinality"`
	StorageBytesPerDay int64    `json:"storage_bytes_per_day"`
	MonthlyCostCents   int64    `json:"monthly_cost_cents"`
	Assumptions        []string `json:"assumptions"`
}
type Alternative struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Tradeoffs        string `json:"tradeoffs"`
	MonthlyCostCents int64  `json:"monthly_cost_cents"`
	Privacy          string `json:"privacy"`
}
type Revision struct {
	RequestID           string        `json:"request_id"`
	Version             int           `json:"version"`
	GapID               string        `json:"observability_gap_id"`
	GapVersion          int           `json:"observability_gap_version"`
	Title               string        `json:"title"`
	Signals             []Signal      `json:"signals"`
	Impact              Impact        `json:"impact"`
	Alternatives        []Alternative `json:"alternatives"`
	OwnerIDs            []string      `json:"owner_ids"`
	ConsumerIDs         []string      `json:"consumer_ids"`
	SupportedCollectors []string      `json:"supported_collectors"`
	CreatedBy           string        `json:"created_by"`
	CreatedAt           time.Time     `json:"created_at"`
}
type Citation struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Digest     string `json:"digest"`
	Verified   bool   `json:"-"`
}
type Challenge struct {
	RequestID       string     `json:"request_id"`
	ContractVersion int        `json:"contract_version"`
	AlternativeID   string     `json:"alternative_id,omitempty"`
	Assumption      string     `json:"assumption"`
	Rationale       string     `json:"rationale"`
	Citations       []Citation `json:"citations"`
	AuthorType      string     `json:"author_type"`
	AuthorID        string     `json:"author_id"`
	CreatedAt       time.Time  `json:"created_at"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	SignalID     string `json:"signal_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type Contract struct {
	ID             string       `json:"id"`
	RepositoryID   string       `json:"repository_id"`
	CurrentVersion int          `json:"current_version"`
	Revisions      []Revision   `json:"revisions"`
	Challenges     []Challenge  `json:"challenges"`
	Diagnostics    []Diagnostic `json:"diagnostics"`
	Complete       bool         `json:"complete"`
	Acceptance     *Acceptance  `json:"acceptance,omitempty"`
	Deliveries     []Delivery   `json:"deliveries"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}
type Acceptance struct {
	RequestID  string    `json:"request_id"`
	Version    int       `json:"contract_version"`
	AcceptedBy string    `json:"accepted_by"`
	Rationale  string    `json:"rationale"`
	CreatedAt  time.Time `json:"created_at"`
}
type Delivery struct {
	RequestID       string    `json:"request_id"`
	ContractVersion int       `json:"contract_version"`
	RepositoryID    string    `json:"repository_id"`
	ProposalID      string    `json:"proposal_id"`
	TaskIDs         []string  `json:"task_ids"`
	BaseRevision    string    `json:"base_revision"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	Status          string    `json:"status"`
}
type Artifact struct {
	Kind      string `json:"kind"`
	Digest    string `json:"digest"`
	SizeBytes int64  `json:"size_bytes"`
	Summary   string `json:"summary"`
}
type VerificationResult struct {
	Requirement string `json:"requirement"`
	Outcome     string `json:"outcome"`
	Summary     string `json:"summary"`
}
type Verification struct {
	RequestID            string               `json:"request_id"`
	RepositoryID         string               `json:"repository_id"`
	PullRequestID        string               `json:"pull_request_id"`
	ContractID           string               `json:"contract_id"`
	ContractRepositoryID string               `json:"contract_repository_id"`
	ContractVersion      int                  `json:"contract_version"`
	Revision             string               `json:"revision"`
	PreviewID            string               `json:"preview_id"`
	CheckRunIDs          []string             `json:"check_run_ids"`
	Journey              string               `json:"journey"`
	FailureScenario      string               `json:"failure_scenario"`
	Isolation            string               `json:"isolation"`
	ProductionData       bool                 `json:"production_data"`
	Results              []VerificationResult `json:"results"`
	Artifacts            []Artifact           `json:"artifacts"`
	Coverage             []string             `json:"coverage"`
	CostCents            int64                `json:"cost_cents"`
	OverheadPercent      float64              `json:"overhead_percent"`
	AuthorType           string               `json:"author_type"`
	AuthorID             string               `json:"author_id"`
	CreatedAt            time.Time            `json:"created_at"`
}
type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

var verificationRequirements = map[string]bool{"emission": true, "schema": true, "units": true, "correlation": true, "sampling": true, "redaction": true, "access": true, "performance": true, "failure_behavior": true}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}
func (s *Store) Create(repo, actor, request string, r Revision) (Contract, error) {
	var out Contract
	e := s.lock(func() error {
		if request == "" || validate(r) != nil {
			return ErrInvalid
		}
		id := stable(repo, actor, request)
		if v, x := s.read(id); x == nil {
			if revisionDigest(v.Revisions[0]) != revisionDigest(r) {
				return ErrConflict
			}
			out = v
			return nil
		} else if !errors.Is(x, ErrNotFound) {
			return x
		}
		now := s.now()
		stamp(&r, actor, request, 1, now)
		out = Contract{ID: id, RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return project(out), e
}
func (s *Store) Revise(repo, id string, expected int, actor, request string, r Revision) (Contract, error) {
	var out Contract
	e := s.lock(func() error {
		v, x := s.read(id)
		if x != nil || v.RepositoryID != repo {
			if x == nil {
				return ErrNotFound
			}
			return x
		}
		for _, old := range v.Revisions {
			if old.RequestID == request {
				if revisionDigest(old) != revisionDigest(r) {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if expected != v.CurrentVersion {
			return ErrConflict
		}
		if request == "" || validate(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, actor, request, expected+1, now)
		v.CurrentVersion++
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return project(out), e
}
func (s *Store) Challenge(repo, id, request string, version int, authorType, authorID, alternative, assumption, rationale string, citations []Citation) (Contract, error) {
	var out Contract
	e := s.lock(func() error {
		v, x := s.read(id)
		if x != nil || v.RepositoryID != repo {
			if x == nil {
				return ErrNotFound
			}
			return x
		}
		for _, c := range v.Challenges {
			if c.RequestID == request {
				candidate := Challenge{RequestID: request, ContractVersion: version, AlternativeID: alternative, Assumption: assumption, Rationale: rationale, Citations: citations, AuthorType: authorType, AuthorID: authorID}
				if challengeDigest(c) != challengeDigest(candidate) {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if request == "" || version != v.CurrentVersion || assumption == "" || rationale == "" || len(citations) == 0 {
			return ErrInvalid
		}
		for _, c := range citations {
			if !c.Verified || c.Kind == "" || c.ResourceID == "" || c.Revision == "" || c.Digest == "" {
				return ErrInvalid
			}
		}
		v.Challenges = append(v.Challenges, Challenge{RequestID: request, ContractVersion: version, AlternativeID: alternative, Assumption: assumption, Rationale: rationale, Citations: citations, AuthorType: authorType, AuthorID: authorID, CreatedAt: s.now()})
		v.UpdatedAt = s.now()
		out = v
		return s.write(v)
	})
	return project(out), e
}
func (s *Store) Get(repo, id string) (Contract, error) {
	var v Contract
	e := s.lock(func() error {
		var x error
		v, x = s.read(id)
		if x == nil && v.RepositoryID != repo {
			return ErrNotFound
		}
		return x
	})
	return project(v), e
}
func (s *Store) List(repo string) ([]Contract, error) {
	xs := []Contract{}
	e := s.lock(func() error {
		fs, x := os.ReadDir(s.root)
		if x != nil {
			return x
		}
		for _, f := range fs {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			v, x := s.read(strings.TrimSuffix(f.Name(), ".json"))
			if x != nil {
				return x
			}
			if v.RepositoryID == repo {
				xs = append(xs, project(v))
			}
		}
		return nil
	})
	sort.Slice(xs, func(i, j int) bool { return xs[i].UpdatedAt.After(xs[j].UpdatedAt) })
	return xs, e
}
func (s *Store) Accept(repo, id, actor, request, rationale string, version int) (Contract, error) {
	var out Contract
	e := s.lock(func() error {
		v, err := s.read(id)
		if err != nil || v.RepositoryID != repo {
			if err == nil {
				return ErrNotFound
			}
			return err
		}
		if v.Acceptance != nil && v.Acceptance.RequestID == request {
			if v.Acceptance.Version != version || v.Acceptance.AcceptedBy != actor || v.Acceptance.Rationale != strings.TrimSpace(rationale) {
				return ErrConflict
			}
			out = v
			return nil
		}
		if request == "" || strings.TrimSpace(rationale) == "" || version != v.CurrentVersion || !project(v).Complete {
			return ErrInvalid
		}
		v.Acceptance = &Acceptance{RequestID: request, Version: version, AcceptedBy: actor, Rationale: strings.TrimSpace(rationale), CreatedAt: s.now()}
		v.UpdatedAt = s.now()
		out = v
		return s.write(v)
	})
	return project(out), e
}
func DeliveryIdentities(contractID, repositoryID string, version, taskCount int) (string, []string) {
	proposal := stableHex("telemetry-delivery", contractID, repositoryID, fmt.Sprint(version))
	tasks := make([]string, taskCount)
	for i := range tasks {
		tasks[i] = stableHex("telemetry-task", contractID, repositoryID, fmt.Sprint(version), fmt.Sprint(i))
	}
	return proposal, tasks
}
func stableHex(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:16])
}
func (s *Store) ReserveDelivery(repo, id string, d Delivery) (Contract, error) {
	var out Contract
	e := s.lock(func() error {
		v, err := s.read(id)
		if err != nil || v.RepositoryID != repo {
			if err == nil {
				return ErrNotFound
			}
			return err
		}
		for _, old := range v.Deliveries {
			if old.RequestID == d.RequestID {
				if old.Status == "pending" || old.Status == "created" {
					d.Status = old.Status
				}
				old.CreatedAt = time.Time{}
				candidate := d
				candidate.CreatedAt = time.Time{}
				if semanticDigest(old) != semanticDigest(candidate) {
					return ErrConflict
				}
				out = v
				return nil
			}
			if old.RepositoryID == d.RepositoryID && old.ContractVersion == d.ContractVersion {
				return ErrConflict
			}
		}
		if d.RequestID == "" || d.RepositoryID == "" || d.ProposalID == "" || len(d.TaskIDs) == 0 || len(d.BaseRevision) != 40 || v.Acceptance == nil || d.ContractVersion != v.Acceptance.Version || d.ContractVersion != v.CurrentVersion {
			return ErrInvalid
		}
		d.Status = "pending"
		d.CreatedAt = s.now()
		v.Deliveries = append(v.Deliveries, d)
		v.UpdatedAt = s.now()
		out = v
		return s.write(v)
	})
	return project(out), e
}
func (s *Store) FinalizeDelivery(repo, id, request string) (Contract, error) {
	var out Contract
	e := s.lock(func() error {
		v, err := s.read(id)
		if err != nil || v.RepositoryID != repo {
			if err == nil {
				return ErrNotFound
			}
			return err
		}
		for i := range v.Deliveries {
			if v.Deliveries[i].RequestID == request {
				if v.Deliveries[i].Status != "pending" && v.Deliveries[i].Status != "created" {
					return ErrInvalid
				}
				v.Deliveries[i].Status = "created"
				v.UpdatedAt = s.now()
				out = v
				return s.write(v)
			}
		}
		return ErrNotFound
	})
	return project(out), e
}
func (s *Store) AddVerification(repo, actorType, actorID string, v Verification) (Verification, error) {
	var out Verification
	e := s.lock(func() error {
		xs, err := s.readVerifications(repo)
		if err != nil {
			return err
		}
		for _, old := range xs {
			if old.RequestID == v.RequestID {
				old = verificationIdentity(old)
				candidate := verificationIdentity(v)
				if semanticDigest(old) != semanticDigest(candidate) {
					return ErrConflict
				}
				out = old
				return nil
			}
		}
		contract, err := s.read(v.ContractID)
		if err != nil || contract.RepositoryID != v.ContractRepositoryID || contract.CurrentVersion != v.ContractVersion || contract.Acceptance == nil || contract.Acceptance.Version != v.ContractVersion {
			return ErrInvalid
		}
		if v.RequestID == "" || v.PullRequestID == "" || len(v.Revision) != 40 || v.PreviewID == "" || len(v.CheckRunIDs) == 0 || v.Journey == "" || v.FailureScenario == "" || v.Isolation != "ephemeral_network_none" || v.ProductionData || v.CostCents < 0 || v.OverheadPercent < 0 || len(v.Results) == 0 {
			return ErrInvalid
		}
		seen := map[string]bool{}
		v.Coverage = []string{}
		for i := range v.Results {
			x := &v.Results[i]
			if !verificationRequirements[x.Requirement] || seen[x.Requirement] || (x.Outcome != "passed" && x.Outcome != "failed") {
				return ErrInvalid
			}
			seen[x.Requirement] = true
			v.Coverage = append(v.Coverage, x.Requirement)
			x.Summary = x.Requirement + " " + x.Outcome + " in isolated synthetic verification"
		}
		if len(seen) != len(verificationRequirements) {
			return ErrInvalid
		}
		sort.Strings(v.Coverage)
		for i := range v.Artifacts {
			a := &v.Artifacts[i]
			if !map[string]bool{"signal": true, "log": true, "trace": true, "coverage": true, "cost": true, "contract_diff": true}[a.Kind] || len(a.Digest) != 64 || a.SizeBytes < 0 || a.SizeBytes > 5<<20 {
				return ErrInvalid
			}
			if _, err := hex.DecodeString(a.Digest); err != nil {
				return ErrInvalid
			}
			a.Summary = "sanitized " + a.Kind + " metadata retained; payload omitted"
		}
		v.RepositoryID = repo
		v.AuthorType = actorType
		v.AuthorID = actorID
		v.CreatedAt = s.now()
		xs = append(xs, v)
		if err = s.writeVerifications(repo, xs); err != nil {
			return err
		}
		out = v
		return nil
	})
	return out, e
}

func verificationIdentity(v Verification) Verification {
	v.CreatedAt = time.Time{}
	v.AuthorType = ""
	v.AuthorID = ""
	v.RepositoryID = ""
	v.Coverage = nil
	for i := range v.Results {
		v.Results[i].Summary = ""
	}
	for i := range v.Artifacts {
		v.Artifacts[i].Summary = ""
	}
	return v
}
func (s *Store) Verifications(repo, pull string) ([]Verification, error) {
	var out []Verification
	e := s.lock(func() error {
		xs, err := s.readVerifications(repo)
		if err != nil {
			return err
		}
		for _, v := range xs {
			if v.PullRequestID == pull {
				out = append(out, v)
			}
		}
		return nil
	})
	return out, e
}
func (s *Store) readVerifications(repo string) ([]Verification, error) {
	b, err := os.ReadFile(filepath.Join(s.root, "verifications", repo+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return []Verification{}, nil
	}
	if err != nil {
		return nil, err
	}
	var xs []Verification
	err = json.Unmarshal(b, &xs)
	return xs, err
}
func (s *Store) writeVerifications(repo string, xs []Verification) error {
	dir := filepath.Join(s.root, "verifications")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(xs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, repo+".json"), b, 0600)
}
func semanticDigest(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func validate(r Revision) error {
	if r.GapID == "" || r.GapVersion < 1 || r.Title == "" || len(r.Signals) == 0 || len(r.OwnerIDs) == 0 || len(r.ConsumerIDs) == 0 || len(r.SupportedCollectors) == 0 || r.Impact.Privacy == "" || r.Impact.Security == "" || r.Impact.Residency == "" || r.Impact.Performance == "" || r.Impact.Cardinality < 0 || r.Impact.StorageBytesPerDay < 0 || r.Impact.MonthlyCostCents < 0 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, s := range r.Signals {
		if s.ID == "" || s.Name == "" || seen[s.ID] || !map[string]bool{"metric": true, "log": true, "trace": true, "profile": true, "event": true}[s.Kind] || s.Description == "" || len(s.Fields) == 0 || s.Sampling == "" || s.Aggregation == "" || s.RetentionDays < 1 || s.ExpectedEventsPerDay < 0 || len(s.QualityThresholds) == 0 || s.Collector == "" || len(s.SourceSymbols) == 0 || len(s.ServiceBoundaries) == 0 {
			return ErrInvalid
		}
		seen[s.ID] = true
		for _, f := range s.Fields {
			if f.Name == "" || f.Type == "" || f.Description == "" {
				return ErrInvalid
			}
		}
		for _, d := range s.Dimensions {
			if d.Name == "" || d.Source == "" {
				return ErrInvalid
			}
		}
		for _, x := range s.SourceSymbols {
			if x.RepositoryID == "" || x.Revision == "" || x.Path == "" || x.Symbol == "" {
				return ErrInvalid
			}
		}
	}
	return nil
}
func project(v Contract) Contract {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	ds := []Diagnostic{}
	add := func(k, severity, msg, id string) {
		ds = append(ds, Diagnostic{Kind: k, Severity: severity, Message: msg, SignalID: id, AttributedTo: r.CreatedBy})
	}
	names := map[string]string{}
	for _, s := range r.Signals {
		if prior, ok := names[s.Name]; ok {
			add("conflicting_definition", "blocking", "Signal name is also defined by "+prior+".", s.ID)
		} else {
			names[s.Name] = s.ID
		}
		supported := false
		for _, c := range r.SupportedCollectors {
			if c == s.Collector {
				supported = true
			}
		}
		if !supported {
			add("unsupported_collector", "blocking", "Collector is not declared supported.", s.ID)
		}
		for _, f := range s.Fields {
			if f.Sensitive && f.Redaction == "" {
				add("sensitive_field", "blocking", "Sensitive field "+f.Name+" has no redaction policy.", s.ID)
			}
		}
		for _, d := range s.Dimensions {
			if !d.Bounded || d.MaximumValues < 1 {
				add("unbounded_dimension", "blocking", "Dimension "+d.Name+" has no finite bound.", s.ID)
			}
		}
	}
	if len(r.Alternatives) == 0 {
		add("alternatives_missing", "incomplete", "No alternative design is available for comparison.", "")
	}
	if len(r.Impact.Assumptions) == 0 {
		add("assumptions_missing", "incomplete", "Impact estimates have no explicit assumptions.", "")
	}
	v.Diagnostics = ds
	v.Complete = true
	for _, d := range ds {
		if d.Severity == "blocking" || d.Severity == "incomplete" {
			v.Complete = false
		}
	}
	return v
}
func stamp(r *Revision, actor, request string, version int, now time.Time) {
	r.RequestID = request
	r.Version = version
	r.CreatedBy = actor
	r.CreatedAt = now
}
func revisionDigest(r Revision) string {
	r.RequestID = ""
	r.Version = 0
	r.CreatedBy = ""
	r.CreatedAt = time.Time{}
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func challengeDigest(c Challenge) string {
	c.CreatedAt = time.Time{}
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func stable(p ...string) string {
	h := sha256.Sum256([]byte(strings.Join(p, "\x00")))
	return "tc_" + hex.EncodeToString(h[:12])
}
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Contract, error) {
	var v Contract
	b, e := os.ReadFile(s.path(id))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *Store) write(v Contract) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.root, ".contract-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	_ = f.Chmod(0600)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	if x := f.Close(); e == nil {
		e = x
	}
	if e == nil {
		e = os.Rename(name, s.path(v.ID))
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
