// Package privacychecks retains revision-exact, sanitized runtime privacy evidence.
package privacychecks

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid privacy check record")
var ErrNotFound = errors.New("privacy check record not found")

var allowedRules = map[string]bool{"collection": true, "consent": true, "minimization": true, "access": true, "retention": true, "export": true, "deletion": true, "telemetry": true, "recipient": true}
var safeIdentifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,127}$`)

type Policy struct {
	ID               string    `json:"id"`
	RepositoryID     string    `json:"repository_id"`
	Branch           string    `json:"branch"`
	Paths            []string  `json:"paths"`
	RequiredRules    []string  `json:"required_rules"`
	RequiredJourneys []string  `json:"required_journeys"`
	PrivacyOwnerIDs  []string  `json:"privacy_owner_ids"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
}
type Artifact struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Digest    string `json:"digest"`
	Summary   string `json:"summary"`
	SizeBytes int64  `json:"size_bytes"`
}
type Result struct {
	Rule    string `json:"rule"`
	Outcome string `json:"outcome"`
	Summary string `json:"summary"`
}
type Run struct {
	ID              string     `json:"id"`
	RepositoryID    string     `json:"repository_id"`
	PullRequestID   string     `json:"pull_request_id"`
	PreviewID       string     `json:"preview_id"`
	Revision        string     `json:"revision"`
	Journey         string     `json:"journey"`
	DataFlowID      string     `json:"data_flow_id"`
	DataFlowVersion int        `json:"data_flow_version"`
	Isolation       string     `json:"isolation"`
	ProductionData  bool       `json:"production_data"`
	Results         []Result   `json:"results"`
	Artifacts       []Artifact `json:"artifacts"`
	Coverage        []string   `json:"coverage"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
}
type Acknowledgement struct {
	PolicyID      string    `json:"policy_id"`
	Revision      string    `json:"revision"`
	PullRequestID string    `json:"pull_request_id,omitempty"`
	ActorID       string    `json:"actor_id"`
	Rationale     string    `json:"rationale"`
	CreatedAt     time.Time `json:"created_at"`
}
type Exception struct {
	ID            string    `json:"id"`
	PolicyID      string    `json:"policy_id"`
	Revision      string    `json:"revision"`
	PullRequestID string    `json:"pull_request_id,omitempty"`
	Rules         []string  `json:"rules"`
	Rationale     string    `json:"rationale"`
	FollowUpWork  string    `json:"follow_up_work"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}
type Requirement struct {
	PolicyID string `json:"policy_id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}
type Readiness struct {
	Ready            bool          `json:"ready"`
	Revision         string        `json:"revision"`
	Requirements     []Requirement `json:"requirements"`
	Runs             []Run         `json:"runs"`
	ActiveExceptions []Exception   `json:"active_exceptions"`
}

type record struct {
	Policies         []Policy          `json:"policies"`
	Runs             []Run             `json:"runs"`
	Acknowledgements []Acknowledgement `json:"acknowledgements"`
	Exceptions       []Exception       `json:"exceptions"`
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

func (s *Store) CreatePolicy(repo, actor string, p Policy) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.read(repo)
	if e != nil {
		return p, e
	}
	p.ID = id()
	p.RepositoryID = repo
	p.CreatedBy = actor
	p.CreatedAt = s.now()
	clean(&p.Paths)
	clean(&p.RequiredRules)
	clean(&p.RequiredJourneys)
	clean(&p.PrivacyOwnerIDs)
	if p.Branch == "" || len(p.RequiredRules) == 0 || len(p.PrivacyOwnerIDs) == 0 {
		return p, ErrInvalid
	}
	for _, path := range p.Paths {
		if strings.HasPrefix(path, "/") || strings.Contains(path, "..") || strings.Contains(path, "://") {
			return p, ErrInvalid
		}
	}
	for _, v := range p.RequiredRules {
		if !allowedRules[v] {
			return p, ErrInvalid
		}
	}
	for _, journey := range p.RequiredJourneys {
		if !safeIdentifier(journey) {
			return p, ErrInvalid
		}
	}
	r.Policies = append(r.Policies, p)
	return p, s.write(repo, r)
}
func (s *Store) Policies(repo string) ([]Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.read(repo)
	return r.Policies, e
}
func (s *Store) AddRun(repo, actor string, v Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.read(repo)
	if e != nil {
		return v, e
	}
	v.ID = id()
	v.RepositoryID = repo
	v.CreatedBy = actor
	v.CreatedAt = s.now()
	if !validCommit(v.Revision) || v.PullRequestID == "" || v.PreviewID == "" || !safeIdentifier(v.Journey) || v.DataFlowID == "" || v.DataFlowVersion < 1 || v.Isolation != "ephemeral_network_none" || v.ProductionData || len(v.Results) == 0 {
		return v, ErrInvalid
	}
	journeyDeclared := false
	for _, policy := range r.Policies {
		for _, journey := range policy.RequiredJourneys {
			if journey == v.Journey {
				journeyDeclared = true
			}
		}
	}
	if !journeyDeclared {
		return v, ErrInvalid
	}
	seen := map[string]bool{}
	v.Coverage = []string{}
	for i := range v.Results {
		x := &v.Results[i]
		if !allowedRules[x.Rule] || (x.Outcome != "passed" && x.Outcome != "failed") || seen[x.Rule] {
			return v, ErrInvalid
		}
		seen[x.Rule] = true
		v.Coverage = append(v.Coverage, x.Rule)
		x.Summary = fmt.Sprintf("%s %s in isolated synthetic journey", x.Rule, x.Outcome)
	}
	sort.Strings(v.Coverage)
	var totalBytes int64
	for i := range v.Artifacts {
		a := &v.Artifacts[i]
		_, digestErr := hex.DecodeString(a.Digest)
		totalBytes += a.SizeBytes
		if !map[string]bool{"log": true, "trace": true, "artifact": true}[a.Kind] || len(a.Digest) != 64 || digestErr != nil || a.SizeBytes < 0 || a.SizeBytes > 5<<20 || totalBytes > 12<<20 {
			return v, ErrInvalid
		}
		a.Name = fmt.Sprintf("%s-%d", a.Kind, i+1)
		a.Summary = fmt.Sprintf("sanitized %s metadata retained; payload omitted", a.Kind)
	}
	r.Runs = append(r.Runs, v)
	return v, s.write(repo, r)
}
func (s *Store) Acknowledge(repo, actor, policy, revision, pullRequestID, rationale string) (Acknowledgement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.read(repo)
	if e != nil {
		return Acknowledgement{}, e
	}
	found := false
	for _, p := range r.Policies {
		if p.ID == policy {
			for _, u := range p.PrivacyOwnerIDs {
				found = found || u == actor
			}
		}
	}
	v := Acknowledgement{PolicyID: policy, Revision: revision, PullRequestID: pullRequestID, ActorID: actor, Rationale: strings.TrimSpace(rationale), CreatedAt: s.now()}
	if !found || !validCommit(revision) || v.Rationale == "" {
		return v, ErrInvalid
	}
	r.Acknowledgements = append(r.Acknowledgements, v)
	return v, s.write(repo, r)
}
func (s *Store) AddException(repo, actor string, v Exception) (Exception, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.read(repo)
	if e != nil {
		return v, e
	}
	found := false
	for _, p := range r.Policies {
		found = found || p.ID == v.PolicyID
	}
	v.ID = id()
	v.CreatedBy = actor
	v.CreatedAt = s.now()
	clean(&v.Rules)
	if !found || !validCommit(v.Revision) || len(v.Rules) == 0 || strings.TrimSpace(v.Rationale) == "" || strings.TrimSpace(v.FollowUpWork) == "" || !v.ExpiresAt.After(v.CreatedAt) || v.ExpiresAt.After(v.CreatedAt.Add(90*24*time.Hour)) {
		return v, ErrInvalid
	}
	for _, x := range v.Rules {
		if !allowedRules[x] {
			return v, ErrInvalid
		}
	}
	r.Exceptions = append(r.Exceptions, v)
	return v, s.write(repo, r)
}
func (s *Store) Evaluate(repo, revision, branch, pullRequestID string, paths []string) (Readiness, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, e := s.read(repo)
	if e != nil {
		return Readiness{}, e
	}
	out := Readiness{true, revision, []Requirement{}, []Run{}, []Exception{}}
	now := s.now()
	for _, run := range x.Runs {
		if run.Revision == revision && (pullRequestID == "" || run.PullRequestID == pullRequestID) {
			out.Runs = append(out.Runs, run)
		}
	}
	for _, p := range x.Policies {
		if p.Branch != branch || !selected(p.Paths, paths) {
			continue
		}
		except := map[string]bool{}
		for _, v := range x.Exceptions {
			if v.PolicyID == p.ID && v.Revision == revision && v.PullRequestID == pullRequestID && v.ExpiresAt.After(now) {
				out.ActiveExceptions = append(out.ActiveExceptions, v)
				for _, q := range v.Rules {
					except[q] = true
				}
			}
		}
		add := func(kind, name, status, msg string) {
			out.Requirements = append(out.Requirements, Requirement{p.ID, kind, name, status, msg})
			if status != "passed" && !except[name] {
				out.Ready = false
			}
		}
		for _, rule := range p.RequiredRules {
			status := "missing"
			for _, run := range x.Runs {
				for _, z := range run.Results {
					if z.Rule == rule && (pullRequestID == "" || run.PullRequestID == pullRequestID) {
						if run.Revision != revision && status == "missing" {
							status = "stale"
						} else if run.Revision == revision && z.Outcome == "passed" {
							status = "passed"
						} else if run.Revision == revision {
							status = "failed"
						}
					}
				}
			}
			add("runtime_rule", rule, status, "sanitized synthetic evidence must pass at the exact candidate revision")
		}
		for _, journey := range p.RequiredJourneys {
			status := "missing"
			for _, run := range x.Runs {
				if run.Journey == journey && (pullRequestID == "" || run.PullRequestID == pullRequestID) {
					if run.Revision != revision && status == "missing" {
						status = "stale"
					} else if run.Revision == revision {
						status = "passed"
						for _, z := range run.Results {
							if z.Outcome != "passed" && !except[z.Rule] {
								status = "failed"
							}
						}
					}
				}
			}
			add("journey", journey, status, "the required synthetic journey must pass in an isolated exact-revision preview")
		}
		status := "missing"
		for _, a := range x.Acknowledgements {
			if a.PolicyID == p.ID && a.PullRequestID == pullRequestID {
				if a.Revision == revision {
					status = "passed"
				} else {
					status = "stale"
				}
			}
		}
		add("privacy_owner", "acknowledgement", status, "a named privacy owner must acknowledge current runtime evidence")
	}
	sort.Slice(out.Requirements, func(i, j int) bool {
		return out.Requirements[i].PolicyID+out.Requirements[i].Kind+out.Requirements[i].Name < out.Requirements[j].PolicyID+out.Requirements[j].Kind+out.Requirements[j].Name
	})
	return out, nil
}

func (s *Store) read(repo string) (record, error) {
	var r record
	b, e := os.ReadFile(filepath.Join(s.root, repo+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return record{Policies: []Policy{}, Runs: []Run{}, Acknowledgements: []Acknowledgement{}, Exceptions: []Exception{}}, nil
	}
	if e != nil {
		return r, e
	}
	e = json.Unmarshal(b, &r)
	return r, e
}
func (s *Store) write(repo string, r record) error {
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".privacy-check-")
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
	if c := tmp.Close(); e == nil {
		e = c
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, repo+".json"))
	}
	return e
}
func clean(v *[]string) {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range *v {
		x = strings.TrimSpace(x)
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	*v = out
}
func validCommit(v string) bool { return len(v) == 40 }
func selected(patterns, paths []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		for _, x := range paths {
			if p == "*" || p == x || strings.HasSuffix(p, "/**") && strings.HasPrefix(x, strings.TrimSuffix(p, "**")) {
				return true
			}
		}
	}
	return false
}

func safeIdentifier(v string) bool {
	return safeIdentifierPattern.MatchString(v) && !strings.Contains(v, "..")
}
func id() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
