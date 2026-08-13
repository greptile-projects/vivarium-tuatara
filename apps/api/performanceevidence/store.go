// Package performanceevidence retains sanitized, exact-revision performance trials.
package performanceevidence

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid performance trial")
var ErrNotFound = errors.New("performance trial not found")

type Source struct {
	Kind      string `json:"kind"`
	Revision  string `json:"revision"`
	ReleaseID string `json:"release_id,omitempty"`
}
type Environment struct {
	Name           string `json:"name"`
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	Runtime        string `json:"runtime"`
	Hardware       string `json:"hardware,omitempty"`
	ContainerImage string `json:"container_image,omitempty"`
}
type Sampling struct {
	Warmup  int    `json:"warmup"`
	Samples int    `json:"samples"`
	Method  string `json:"method"`
}
type Timing struct {
	Metric   string    `json:"metric"`
	Unit     string    `json:"unit"`
	Values   []float64 `json:"values"`
	Mean     float64   `json:"mean"`
	Minimum  float64   `json:"minimum"`
	Maximum  float64   `json:"maximum"`
	Variance float64   `json:"variance"`
}
type ResourceProfile struct {
	CPUSeconds   float64 `json:"cpu_seconds"`
	PeakMemoryMB float64 `json:"peak_memory_mb"`
	ReadBytes    int64   `json:"read_bytes"`
	WriteBytes   int64   `json:"write_bytes"`
}
type Artifact struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Cost struct {
	Amount float64 `json:"amount"`
	Unit   string  `json:"unit"`
}
type Trial struct {
	ID           string          `json:"id"`
	RepositoryID string          `json:"repository_id"`
	GoalID       string          `json:"goal_id,omitempty"`
	ContextKind  string          `json:"context_kind"`
	ContextID    string          `json:"context_id"`
	Mode         string          `json:"mode"`
	Source       Source          `json:"source"`
	Workload     string          `json:"workload"`
	Inputs       string          `json:"inputs"`
	Sanitization []string        `json:"sanitization"`
	Environment  Environment     `json:"environment"`
	Sampling     Sampling        `json:"sampling"`
	Timings      []Timing        `json:"timings"`
	Resources    ResourceProfile `json:"resources"`
	Traces       []Artifact      `json:"traces"`
	Logs         []string        `json:"logs"`
	Artifacts    []Artifact      `json:"artifacts"`
	Cost         Cost            `json:"cost"`
	CreatedBy    string          `json:"created_by"`
	CreatedAt    time.Time       `json:"created_at"`
}
type Comparison struct {
	Metric        string  `json:"metric"`
	Unit          string  `json:"unit"`
	BaselineMean  float64 `json:"baseline_mean"`
	CurrentMean   float64 `json:"current_mean"`
	ChangePercent float64 `json:"change_percent"`
	Comparable    bool    `json:"comparable"`
	Reason        string  `json:"reason,omitempty"`
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
func (s *Store) Create(v Trial) (Trial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(v) {
		return Trial{}, ErrInvalid
	}
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return Trial{}, e
	}
	v.ID = hex.EncodeToString(b[:])
	v.CreatedAt = s.now()
	// Production captures retain the declared recipe and sanitization policy, not
	// producer-supplied operational input. A declaration alone cannot prove that
	// an arbitrary value contains no private user data.
	if v.Mode == "production_capture" {
		v.Inputs = "[sanitized production-derived workload]"
		v.Logs = sanitizedProductionLogs(v.Logs)
	}
	for i := range v.Timings {
		summarize(&v.Timings[i])
	}
	body, _ := json.Marshal(v)
	tmp, e := os.CreateTemp(s.root, ".trial-*")
	if e != nil {
		return Trial{}, e
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0600)
	_, e = tmp.Write(body)
	if e == nil {
		e = tmp.Sync()
	}
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	return v, e
}
func (s *Store) Get(id string) (Trial, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.read(id) }
func (s *Store) List(repositoryID string) ([]Trial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Trial{}
	for _, x := range es {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		v, er := s.read(strings.TrimSuffix(x.Name(), ".json"))
		if er != nil {
			return nil, er
		}
		if v.RepositoryID == repositoryID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Compare(a, b Trial) []Comparison {
	out := []Comparison{}
	old := map[string]Timing{}
	for _, t := range a.Timings {
		old[t.Metric+"\x00"+t.Unit] = t
	}
	for _, t := range b.Timings {
		x := Comparison{Metric: t.Metric, Unit: t.Unit, CurrentMean: t.Mean}
		o, ok := old[t.Metric+"\x00"+t.Unit]
		x.Comparable = ok && a.Workload == b.Workload && a.Environment == b.Environment && a.Sampling == b.Sampling
		if !x.Comparable {
			x.Reason = "workload, complete environment, warmup, sampling method/count, metric, and unit must match"
		} else {
			x.BaselineMean = o.Mean
			if o.Mean != 0 {
				x.ChangePercent = (t.Mean - o.Mean) / o.Mean * 100
			}
		}
		out = append(out, x)
	}
	return out
}
func (s *Store) read(id string) (Trial, error) {
	if len(id) != 32 {
		return Trial{}, ErrNotFound
	}
	body, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(e) {
		return Trial{}, ErrNotFound
	}
	var v Trial
	if e != nil || json.Unmarshal(body, &v) != nil || v.ID != id {
		return Trial{}, ErrNotFound
	}
	if v.Mode == "production_capture" {
		v.Inputs = "[sanitized production-derived workload]"
		v.Logs = sanitizedProductionLogs(v.Logs)
	}
	return v, nil
}

func sanitizedProductionLogs(logs []string) []string {
	if len(logs) == 0 {
		return logs
	}
	result := make([]string, len(logs))
	for i := range result {
		result[i] = "[sanitized production log entry]"
	}
	return result
}
func valid(v Trial) bool {
	if v.RepositoryID == "" || v.CreatedBy == "" || len(v.Source.Revision) != 40 || (v.Source.Kind != "revision" && v.Source.Kind != "release") || (v.Mode != "benchmark" && v.Mode != "production_capture") || v.Workload == "" || v.Inputs == "" || v.Environment.Name == "" || v.Sampling.Samples < 1 || v.Sampling.Samples > 10000 || v.Sampling.Warmup < 0 || v.Sampling.Method == "" || len(v.Timings) == 0 || len(v.Logs) > 200 {
		return false
	}
	if v.Mode == "production_capture" && len(v.Sanitization) == 0 {
		return false
	}
	for _, line := range v.Logs {
		if len(line) > 4000 || containsCredential(line) {
			return false
		}
	}
	for _, t := range v.Timings {
		if t.Metric == "" || t.Unit == "" || len(t.Values) != v.Sampling.Samples {
			return false
		}
		for _, n := range t.Values {
			if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
				return false
			}
		}
	}
	for _, a := range append(v.Traces, v.Artifacts...) {
		if a.Name == "" || len(a.SHA256) != 64 || a.Size < 0 {
			return false
		}
	}
	return true
}
func containsCredential(v string) bool {
	x := strings.ToLower(v)
	for _, p := range []string{"authorization:", "bearer ", "password=", "password:", "token=", "token:", "secret=", "secret:", "cookie:", "x-api-key", "api-key", "api_key", "apikey"} {
		if strings.Contains(x, p) {
			return true
		}
	}
	var structured any
	if json.Unmarshal([]byte(v), &structured) == nil && structuredCredential(structured) {
		return true
	}
	return false
}

func structuredCredential(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(key))
			if normalized == "token" || normalized == "accesstoken" || normalized == "refreshtoken" || normalized == "password" || normalized == "secret" || normalized == "apikey" || normalized == "authorization" || normalized == "cookie" {
				return true
			}
			if structuredCredential(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if structuredCredential(child) {
				return true
			}
		}
	}
	return false
}
func summarize(t *Timing) {
	t.Minimum, t.Maximum = t.Values[0], t.Values[0]
	var sum float64
	for _, v := range t.Values {
		sum += v
		if v < t.Minimum {
			t.Minimum = v
		}
		if v > t.Maximum {
			t.Maximum = v
		}
	}
	t.Mean = sum / float64(len(t.Values))
	for _, v := range t.Values {
		d := v - t.Mean
		t.Variance += d * d
	}
	t.Variance /= float64(len(t.Values))
}
