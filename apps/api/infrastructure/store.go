// Package infrastructure persists repository infrastructure declarations and sanitized observations.
package infrastructure

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("infrastructure definition not found")
var ErrInvalid = errors.New("invalid infrastructure definition")
var ErrConflict = errors.New("infrastructure definition version conflict")

var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bgh[pousr]_[a-z0-9]{20,}\b`),
	regexp.MustCompile(`(?i)\bgithub_pat_[a-z0-9_]{20,}\b`),
	regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),
	regexp.MustCompile(`(?i)\b(?:xox[baprs]-[a-z0-9-]{10,}|sk-(?:proj-)?[a-z0-9_-]{20,})\b`),
	regexp.MustCompile(`(?i)\beyJ[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\b`),
}

type ConfigurationBoundary struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Sensitivity string `json:"sensitivity"`
	Required    bool   `json:"required"`
}
type Constraint struct {
	Kind  string  `json:"kind"`
	Limit float64 `json:"limit"`
	Unit  string  `json:"unit"`
	Note  string  `json:"note,omitempty"`
}
type Commitments struct {
	Security    []string `json:"security"`
	Privacy     []string `json:"privacy"`
	Reliability []string `json:"reliability"`
	Continuity  []string `json:"continuity"`
	Regions     []string `json:"regions"`
}
type Resource struct {
	ID             string                  `json:"id"`
	Kind           string                  `json:"kind"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description"`
	OwnerIDs       []string                `json:"owner_ids"`
	Provider       string                  `json:"provider"`
	ProviderRef    string                  `json:"provider_ref,omitempty"`
	ProviderAccess string                  `json:"provider_access"`
	EnvironmentID  string                  `json:"environment_id,omitempty"`
	ReleaseID      string                  `json:"release_id,omitempty"`
	DependsOn      []string                `json:"depends_on"`
	Configuration  []ConfigurationBoundary `json:"configuration"`
	Constraints    []Constraint            `json:"constraints"`
	Commitments    Commitments             `json:"commitments"`
}
type Revision struct {
	Version   int        `json:"version"`
	Title     string     `json:"title"`
	Summary   string     `json:"summary"`
	Revision  string     `json:"revision"`
	Resources []Resource `json:"resources"`
	OwnerIDs  []string   `json:"owner_ids"`
	Rationale string     `json:"rationale"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
}
type Observation struct {
	ID                string    `json:"id"`
	DefinitionVersion int       `json:"definition_version"`
	ResourceID        string    `json:"resource_id,omitempty"`
	ProviderResource  string    `json:"provider_resource"`
	ObservedRevision  string    `json:"observed_revision"`
	EnvironmentID     string    `json:"environment_id,omitempty"`
	ReleaseID         string    `json:"release_id,omitempty"`
	Status            string    `json:"status"`
	Summary           string    `json:"summary"`
	Visibility        string    `json:"visibility"`
	Managed           bool      `json:"managed"`
	ObservedAt        time.Time `json:"observed_at"`
	RecordedBy        string    `json:"recorded_by"`
	RecordedAt        time.Time `json:"recorded_at"`
}
type Diagnostic struct {
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	ResourceID string `json:"resource_id,omitempty"`
}
type Definition struct {
	ID             string        `json:"id"`
	RepositoryID   string        `json:"repository_id"`
	CurrentVersion int           `json:"current_version"`
	Revisions      []Revision    `json:"revisions"`
	Observations   []Observation `json:"observations"`
	Diagnostics    []Diagnostic  `json:"diagnostics"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
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
func (s *Store) Create(repo, actor string, r Revision) (Definition, error) {
	var out Definition
	err := s.lock(func() error {
		if validateRevision(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Definition{ID: randomID(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, Observations: []Observation{}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.project(out, true), err
}
func (s *Store) Revise(id string, expected int, actor string, r Revision) (Definition, error) {
	var out Definition
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validateRevision(r) != nil {
			return ErrInvalid
		}
		stamp(&r, expected+1, actor, s.now())
		v.CurrentVersion = r.Version
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		return s.write(v)
	})
	return s.project(out, true), err
}
func (s *Store) Observe(id, actor string, o Observation) (Definition, error) {
	var out Definition
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		if validateObservation(v, o, s.now()) != nil {
			return ErrInvalid
		}
		o.ID = randomID()
		o.RecordedBy = actor
		o.RecordedAt = s.now()
		v.Observations = append(v.Observations, o)
		v.UpdatedAt = o.RecordedAt
		out = v
		return s.write(v)
	})
	return s.project(out, true), err
}
func (s *Store) Get(id string, participant bool) (Definition, error) {
	var out Definition
	err := s.lock(func() error { var e error; out, e = s.read(id); return e })
	return s.project(out, participant), err
}
func (s *Store) List(repo string, participant bool) ([]Definition, error) {
	out := []Definition{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.repoDir(repo))
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			v, e := s.readFile(filepath.Join(s.repoDir(repo), entry.Name()))
			if e != nil {
				return e
			}
			if v.RepositoryID == repo {
				out = append(out, s.project(v, participant))
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
func validateRevision(r Revision) error {
	kinds := map[string]bool{"environment": true, "service": true, "network": true, "identity": true, "data_store": true, "compute": true, "external_dependency": true}
	access := map[string]bool{"public": true, "participant": true, "inaccessible": true}
	sources := map[string]bool{"literal": true, "environment": true, "file": true, "secret": true, "provider": true}
	sensitivities := map[string]bool{"public": true, "internal": true, "secret_backed": true}
	if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Revision) == "" || len(r.Resources) == 0 || len(r.OwnerIDs) == 0 || unsafe(r.Title, r.Summary, r.Revision, r.Rationale) {
		return ErrInvalid
	}
	ids := map[string]bool{}
	for _, x := range r.Resources {
		if x.ID == "" || ids[x.ID] || !kinds[x.Kind] || x.Name == "" || len(x.OwnerIDs) == 0 || x.Provider == "" || !access[x.ProviderAccess] || unsafe(x.ID, x.Name, x.Description, x.Provider, x.ProviderRef, x.EnvironmentID, x.ReleaseID) {
			return ErrInvalid
		}
		ids[x.ID] = true
		for _, c := range x.Configuration {
			if c.Name == "" || !sources[c.Source] || !sensitivities[c.Sensitivity] || unsafe(c.Name) {
				return ErrInvalid
			}
		}
		for _, c := range x.Constraints {
			if (c.Kind != "cost" && c.Kind != "capacity") || c.Limit < 0 || c.Unit == "" || unsafe(c.Unit, c.Note) {
				return ErrInvalid
			}
		}
		for _, v := range append(append(append(append([]string{}, x.Commitments.Security...), x.Commitments.Privacy...), x.Commitments.Reliability...), append(x.Commitments.Continuity, x.Commitments.Regions...)...) {
			if strings.TrimSpace(v) == "" || unsafe(v) {
				return ErrInvalid
			}
		}
	}
	for _, x := range r.Resources {
		for _, dep := range x.DependsOn {
			if !ids[dep] || dep == x.ID {
				return ErrInvalid
			}
		}
	}
	return nil
}
func validateObservation(v Definition, o Observation, now time.Time) error {
	if o.DefinitionVersion < 1 || o.DefinitionVersion > v.CurrentVersion || o.ProviderResource == "" || o.ObservedRevision == "" || o.Summary == "" || o.ObservedAt.IsZero() || o.ObservedAt.After(now) || (o.Visibility != "public" && o.Visibility != "participant") || (o.Status != "healthy" && o.Status != "degraded" && o.Status != "unknown") || unsafe(o.ProviderResource, o.ObservedRevision, o.Summary) {
		return ErrInvalid
	}
	if o.Managed {
		if o.ResourceID == "" || !hasResource(v.Revisions[o.DefinitionVersion-1], o.ResourceID) {
			return ErrInvalid
		}
		for _, resource := range v.Revisions[o.DefinitionVersion-1].Resources {
			if resource.ID == o.ResourceID && ((resource.EnvironmentID != "" && resource.EnvironmentID != o.EnvironmentID) || (resource.ReleaseID != "" && resource.ReleaseID != o.ReleaseID)) {
				return ErrInvalid
			}
		}
	} else if o.ResourceID != "" {
		return ErrInvalid
	}
	return nil
}
func hasResource(r Revision, id string) bool {
	for _, x := range r.Resources {
		if x.ID == id {
			return true
		}
	}
	return false
}
func unsafe(values ...string) bool {
	for _, v := range values {
		l := strings.ToLower(v)
		for _, needle := range []string{"-----begin private key", "bearer ", "api_key=", "apikey=", "password=", "secret=", "token="} {
			if strings.Contains(l, needle) {
				return true
			}
		}
		for _, pattern := range credentialPatterns {
			if pattern.MatchString(v) {
				return true
			}
		}
	}
	return false
}
func (s *Store) project(v Definition, participant bool) Definition {
	if len(v.Revisions) == 0 {
		return v
	}
	current := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	add := func(k, sev, msg, id string) { d = append(d, Diagnostic{k, sev, msg, id}) }
	refs := map[string]Resource{}
	for _, x := range current.Resources {
		refs[x.ID] = x
		if x.ProviderAccess == "inaccessible" {
			add("inaccessible_provider", "warning", "Provider state is declared inaccessible; no observation is implied.", x.ID)
		}
		for _, c := range x.Configuration {
			if c.Source == "secret" || c.Sensitivity == "secret_backed" {
				add("secret_backed_value", "info", "Configuration is secret-backed; only its boundary is shown.", x.ID)
				break
			}
		}
	}
	providerOwners := map[string][]string{}
	providerResource := map[string]string{}
	for _, x := range current.Resources {
		if x.ProviderRef == "" {
			continue
		}
		key := x.Provider + "\x00" + x.ProviderRef
		if owners, ok := providerOwners[key]; ok && !sameStrings(owners, x.OwnerIDs) {
			add("conflicting_ownership", "blocking", "Resources sharing a provider identity declare conflicting owners.", x.ID)
			add("conflicting_ownership", "blocking", "Resources sharing a provider identity declare conflicting owners.", providerResource[key])
		} else {
			providerOwners[key] = x.OwnerIDs
			providerResource[key] = x.ID
		}
	}
	projected := make([]Observation, 0, len(v.Observations))
	observed := map[string]bool{}
	for _, o := range v.Observations {
		age := s.now().Sub(o.ObservedAt)
		current := o.DefinitionVersion == v.CurrentVersion && age >= 0 && age <= 24*time.Hour
		if o.DefinitionVersion != v.CurrentVersion {
			add("stale_observation", "warning", "Observation targets an earlier definition revision.", o.ResourceID)
		} else if age < 0 {
			add("invalid_observation_time", "blocking", "Observation timestamp is later than the projection clock.", o.ResourceID)
		} else if age > 24*time.Hour {
			add("stale_observation", "warning", "Observation is older than 24 hours.", o.ResourceID)
		}
		if !o.Managed {
			add("unmanaged_resource", "warning", "Observed provider resource is not represented by this definition.", "")
		} else if current {
			observed[o.ResourceID] = true
		}
		if !participant && o.Visibility != "public" {
			o.ProviderResource = "restricted"
			o.ObservedRevision = "restricted"
			o.Summary = "Participant-only observation"
		}
		projected = append(projected, o)
	}
	for id, x := range refs {
		if x.ProviderAccess != "inaccessible" && !observed[id] {
			add("missing_observation", "info", "No current permitted observation is available.", id)
		}
	}
	if !participant {
		for revisionIndex := range v.Revisions {
			for resourceIndex := range v.Revisions[revisionIndex].Resources {
				resource := &v.Revisions[revisionIndex].Resources[resourceIndex]
				if resource.ProviderAccess != "public" && resource.ProviderRef != "" {
					resource.ProviderRef = "restricted"
				}
			}
		}
	}
	v.Observations = projected
	v.Diagnostics = d
	return v
}
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		m[x]--
		if m[x] < 0 {
			return false
		}
	}
	return true
}
func (s *Store) repoDir(repo string) string {
	return filepath.Join(s.root, "repo-"+base64.RawURLEncoding.EncodeToString([]byte(repo)))
}
func (s *Store) read(id string) (Definition, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return Definition{}, e
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		p := filepath.Join(s.root, entry.Name(), id+".json")
		v, e := s.readFile(p)
		if e == nil {
			return v, nil
		}
		if !os.IsNotExist(e) {
			return Definition{}, e
		}
	}
	return Definition{}, ErrNotFound
}
func (s *Store) readFile(path string) (Definition, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return Definition{}, e
	}
	var v Definition
	if json.Unmarshal(b, &v) != nil {
		return Definition{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Definition) error {
	dir := s.repoDir(v.RepositoryID)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(dir, ".infrastructure-*.tmp")
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
	if e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(dir, v.ID+".json"))
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lockPath := filepath.Join(s.root, ".lock")
	f, e := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
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
func randomID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
