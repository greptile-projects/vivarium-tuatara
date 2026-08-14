// Package accessibilityreports retains privacy-bounded accessibility barrier evidence.
package accessibilityreports

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
	"time"
)

var ErrNotFound = errors.New("accessibility report not found")
var ErrInvalid = errors.New("invalid accessibility report")

type Target struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Location   string `json:"location,omitempty"`
}
type Consent struct {
	ShareIdentity      bool `json:"share_identity"`
	ShareDeviceDetails bool `json:"share_device_details"`
}
type Artifact struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
	ContentRef  string `json:"content_ref"`
	Redacted    bool   `json:"redacted"`
}
type Environment struct {
	Browser                    string `json:"browser"`
	BrowserVersion             string `json:"browser_version"`
	Device                     string `json:"device"`
	OperatingSystem            string `json:"operating_system"`
	AssistiveTechnology        string `json:"assistive_technology"`
	AssistiveTechnologyVersion string `json:"assistive_technology_version"`
	InputMode                  string `json:"input_mode,omitempty"`
}
type Attempt struct {
	ID          string      `json:"id"`
	RunnerID    string      `json:"runner_id"`
	Revision    string      `json:"revision"`
	Boundary    string      `json:"boundary"`
	Environment Environment `json:"environment"`
	Outcome     string      `json:"outcome"`
	Notes       string      `json:"notes"`
	Evidence    []Artifact  `json:"evidence"`
	CreatedAt   time.Time   `json:"created_at"`
}
type Report struct {
	ID                  string      `json:"id"`
	RepositoryID        string      `json:"repository_id"`
	ReporterID          string      `json:"reporter_id,omitempty"`
	Target              Target      `json:"target"`
	AccessNeeds         []string    `json:"access_needs"`
	ExpectedOutcome     string      `json:"expected_outcome"`
	Steps               []string    `json:"steps"`
	ReporterEnvironment Environment `json:"reporter_environment"`
	Evidence            []Artifact  `json:"evidence"`
	Consent             Consent     `json:"consent"`
	Attempts            []Attempt   `json:"attempts"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
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
func (s *Store) Create(repo, reporter string, x Report) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x.RepositoryID = repo
	x.ReporterID = reporter
	x.ID = id()
	x.Attempts = nil
	x.CreatedAt = s.now()
	x.UpdatedAt = x.CreatedAt
	if !validReport(x) {
		return Report{}, ErrInvalid
	}
	return x, s.write(x)
}
func (s *Store) AddAttempt(reportID, runner string, x Attempt) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.get(reportID)
	if e != nil {
		return Report{}, e
	}
	x.ID = id()
	x.RunnerID = runner
	x.CreatedAt = s.now()
	x.Revision = v.Target.Revision
	if !validAttempt(x) {
		return Report{}, ErrInvalid
	}
	v.Attempts = append(v.Attempts, x)
	v.UpdatedAt = x.CreatedAt
	return v, s.write(v)
}
func (s *Store) Get(id string) (Report, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.get(id) }
func (s *Store) List(repo string) ([]Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Report{}
	for _, f := range es {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		v, e := s.get(strings.TrimSuffix(f.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		if v.RepositoryID == repo {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func Project(x Report, viewer string, participant bool) Report {
	if viewer != x.ReporterID && !(participant && x.Consent.ShareIdentity) {
		x.ReporterID = ""
	}
	if viewer != x.ReporterID && !x.Consent.ShareDeviceDetails {
		x.ReporterEnvironment.Device = ""
		x.ReporterEnvironment.OperatingSystem = ""
		x.ReporterEnvironment.BrowserVersion = ""
		x.ReporterEnvironment.AssistiveTechnologyVersion = ""
		x.ReporterEnvironment.InputMode = ""
	}
	return x
}

func validReport(x Report) bool {
	kinds := map[string]bool{"release": true, "page": true, "documentation_journey": true, "preview": true}
	if !kinds[x.Target.Kind] || !bounded(x.Target.ResourceID, 256) || !bounded(x.Target.Revision, 256) || len(x.AccessNeeds) == 0 || len(x.AccessNeeds) > 20 || len(x.Steps) == 0 || len(x.Steps) > 50 || !bounded(x.ExpectedOutcome, 4000) || len(x.Evidence) > 12 {
		return false
	}
	for _, value := range append(append([]string{}, x.AccessNeeds...), x.Steps...) {
		if !bounded(value, 2000) {
			return false
		}
	}
	return validArtifacts(x.Evidence)
}
func validAttempt(x Attempt) bool {
	out := map[string]bool{"reproducible": true, "intermittent": true, "environment_specific": true, "unconfirmed": true}
	bound := map[string]bool{"workspace": true, "preview": true}
	return out[x.Outcome] && bound[x.Boundary] && bounded(x.Environment.Browser, 200) && bounded(x.Environment.Device, 500) && bounded(x.Environment.AssistiveTechnology, 200) && len(x.Notes) <= 4000 && len(x.Evidence) <= 12 && validArtifacts(x.Evidence)
}
func validArtifacts(xs []Artifact) bool {
	kinds := map[string]bool{"screenshot": true, "recording": true, "accessibility_tree": true, "speech_output": true, "input_trace": true}
	for _, x := range xs {
		if !kinds[x.Kind] || !x.Redacted || !bounded(x.Description, 2000) || !strings.HasPrefix(x.ContentRef, "artifact://") || strings.ContainsAny(x.ContentRef, "?#") || len(x.ContentRef) > 2048 {
			return false
		}
	}
	return true
}
func bounded(value string, limit int) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= limit
}
func (s *Store) get(id string) (Report, error) {
	if len(id) != 32 {
		return Report{}, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return Report{}, ErrNotFound
	}
	var x Report
	if e == nil {
		e = json.Unmarshal(b, &x)
	}
	return x, e
}
func (s *Store) write(x Report) error {
	b, e := json.MarshalIndent(x, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(s.root, x.ID+".json"), b, 0600)
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
