// Package interfacechecks retains revision-exact interface comparison evidence.
package interfacechecks

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

var ErrInvalid = errors.New("invalid interface check")
var ErrNotFound = errors.New("interface check not found")
var ErrConflict = errors.New("interface check conflict")

type Context struct {
	Viewport            string `json:"viewport"`
	Theme               string `json:"theme"`
	Content             string `json:"content"`
	Locale              string `json:"locale"`
	Interaction         string `json:"interaction"`
	AssistiveTechnology string `json:"assistive_technology"`
}
type Difference struct {
	ID                  string `json:"id"`
	Kind                string `json:"kind"`
	Summary             string `json:"summary"`
	Requirement         string `json:"requirement"`
	BaselineArtifactID  string `json:"baseline_artifact_id,omitempty"`
	CandidateArtifactID string `json:"candidate_artifact_id,omitempty"`
}
type Artifact struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Digest    string `json:"digest"`
	SizeBytes int64  `json:"size_bytes"`
}
type Performance struct {
	Metric    string  `json:"metric"`
	Unit      string  `json:"unit"`
	Baseline  float64 `json:"baseline"`
	Candidate float64 `json:"candidate"`
	Budget    float64 `json:"budget"`
	Passed    bool    `json:"passed"`
}
type Classification struct {
	DifferenceID string    `json:"difference_id"`
	Outcome      string    `json:"outcome"`
	Rationale    string    `json:"rationale"`
	ActorID      string    `json:"actor_id"`
	CreatedAt    time.Time `json:"created_at"`
}
type Check struct {
	ID                   string           `json:"id"`
	RepositoryID         string           `json:"repository_id"`
	PullRequestID        string           `json:"pull_request_id"`
	Revision             string           `json:"revision"`
	PreviewID            string           `json:"preview_id"`
	DefinitionPath       string           `json:"definition_path"`
	DefinitionDigest     string           `json:"definition_digest"`
	DesignProposalID     string           `json:"design_proposal_id"`
	DesignVersion        int              `json:"design_version"`
	Name                 string           `json:"name"`
	Journey              string           `json:"journey"`
	Context              Context          `json:"context"`
	Status               string           `json:"status"`
	Coverage             []string         `json:"coverage"`
	AffectedRequirements []string         `json:"affected_requirements"`
	Differences          []Difference     `json:"differences"`
	Artifacts            []Artifact       `json:"artifacts"`
	Performance          []Performance    `json:"performance"`
	Classifications      []Classification `json:"classifications"`
	CreatedBy            string           `json:"created_by"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}
type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}
func token() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) file(repo, pull, id string) string {
	return filepath.Join(s.root, repo, pull, id+".json")
}
func valid(c Check) bool {
	if c.RepositoryID == "" || c.PullRequestID == "" || len(c.Revision) != 40 || c.PreviewID == "" || c.DefinitionPath == "" || len(c.DefinitionDigest) != 64 || c.DesignProposalID == "" || c.DesignVersion < 1 || strings.TrimSpace(c.Name) == "" || c.Journey == "" || len(c.Coverage) == 0 || len(c.AffectedRequirements) == 0 {
		return false
	}
	ctx := []string{c.Context.Viewport, c.Context.Theme, c.Context.Content, c.Context.Locale, c.Context.Interaction, c.Context.AssistiveTechnology}
	for _, v := range ctx {
		if strings.TrimSpace(v) == "" {
			return false
		}
	}
	if c.Status != "passed" && c.Status != "failed" {
		return false
	}
	artifactIDs := map[string]bool{}
	for _, a := range c.Artifacts {
		if a.ID == "" || artifactIDs[a.ID] || a.Name == "" || len(a.Digest) != 64 || a.SizeBytes < 0 || !strings.HasPrefix(a.URL, "/") || strings.HasPrefix(a.URL, "//") {
			return false
		}
		artifactIDs[a.ID] = true
	}
	affected := map[string]bool{}
	for _, requirement := range c.AffectedRequirements {
		if strings.TrimSpace(requirement) == "" || affected[requirement] {
			return false
		}
		affected[requirement] = true
	}
	differenceIDs := map[string]bool{}
	for _, d := range c.Differences {
		if d.ID == "" || differenceIDs[d.ID] || d.Summary == "" || !affected[d.Requirement] || (d.Kind != "visual" && d.Kind != "behavioral" && d.Kind != "accessibility" && d.Kind != "content" && d.Kind != "performance") {
			return false
		}
		differenceIDs[d.ID] = true
	}
	for _, performance := range c.Performance {
		values := []float64{performance.Baseline, performance.Candidate, performance.Budget}
		if strings.TrimSpace(performance.Metric) == "" || strings.TrimSpace(performance.Unit) == "" {
			return false
		}
		for _, value := range values {
			if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return false
			}
		}
		if performance.Passed != (performance.Candidate <= performance.Budget) {
			return false
		}
		if c.Status == "passed" && !performance.Passed {
			return false
		}
	}
	return true
}
func (s *Store) Create(c Check) (Check, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(c) {
		return Check{}, ErrInvalid
	}
	c.ID = token()
	c.CreatedAt = s.now()
	c.UpdatedAt = c.CreatedAt
	c.Classifications = []Classification{}
	if err := os.MkdirAll(filepath.Dir(s.file(c.RepositoryID, c.PullRequestID, c.ID)), 0700); err != nil {
		return Check{}, err
	}
	return c, s.write(c)
}
func (s *Store) write(c Check) error {
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.file(c.RepositoryID, c.PullRequestID, c.ID) + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e == nil {
		e = os.Rename(tmp, s.file(c.RepositoryID, c.PullRequestID, c.ID))
	}
	return e
}
func (s *Store) Get(repo, pull, id string) (Check, error) {
	b, e := os.ReadFile(s.file(repo, pull, id))
	if os.IsNotExist(e) {
		return Check{}, ErrNotFound
	}
	var c Check
	if e == nil {
		e = json.Unmarshal(b, &c)
	}
	return c, e
}
func (s *Store) List(repo, pull string) ([]Check, error) {
	entries, e := os.ReadDir(filepath.Join(s.root, repo, pull))
	if os.IsNotExist(e) {
		return []Check{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Check{}
	for _, x := range entries {
		if filepath.Ext(x.Name()) != ".json" {
			continue
		}
		c, e := s.Get(repo, pull, strings.TrimSuffix(x.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Classify(repo, pull, id, difference, outcome, rationale, actor string) (Check, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, e := s.Get(repo, pull, id)
	if e != nil {
		return c, e
	}
	if outcome != "intentional_change" && outcome != "regression" && outcome != "false_positive" || strings.TrimSpace(rationale) == "" {
		return c, ErrInvalid
	}
	found := false
	for _, d := range c.Differences {
		if d.ID == difference {
			found = true
		}
	}
	if !found {
		return c, ErrNotFound
	}
	for _, v := range c.Classifications {
		if v.DifferenceID == difference {
			return c, ErrConflict
		}
	}
	c.Classifications = append(c.Classifications, Classification{DifferenceID: difference, Outcome: outcome, Rationale: rationale, ActorID: actor, CreatedAt: s.now()})
	c.UpdatedAt = s.now()
	return c, s.write(c)
}
