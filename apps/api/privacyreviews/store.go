// Package privacyreviews retains revision-grounded privacy change review for pull requests.
package privacyreviews

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("privacy review not found")
var ErrInvalid = errors.New("invalid privacy review")
var ErrConflict = errors.New("privacy review conflict")

type Evidence struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Claim     string `json:"claim"`
}
type Change struct {
	Kind           string     `json:"kind"`
	Summary        string     `json:"summary"`
	DataCategories []string   `json:"data_categories,omitempty"`
	SourceIDs      []string   `json:"source_ids"`
	Evidence       []Evidence `json:"evidence,omitempty"`
}
type Requirement struct {
	Kind       string     `json:"kind"`
	OwnerIDs   []string   `json:"owner_ids,omitempty"`
	Reason     string     `json:"reason"`
	Status     string     `json:"status"`
	RecordedBy string     `json:"recorded_by,omitempty"`
	RecordedAt *time.Time `json:"recorded_at,omitempty"`
}
type Comment struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	Body         string     `json:"body"`
	FindingKinds []string   `json:"finding_kinds"`
	Evidence     []Evidence `json:"evidence,omitempty"`
	ActorType    string     `json:"actor_type"`
	ActorID      string     `json:"actor_id"`
	CreatedAt    time.Time  `json:"created_at"`
}
type PriorReview struct {
	ID             string        `json:"id"`
	SourceRevision string        `json:"source_revision"`
	TargetRevision string        `json:"target_revision"`
	Changes        []Change      `json:"changes"`
	Requirements   []Requirement `json:"requirements"`
	Comments       []Comment     `json:"comments"`
	ResidualRisk   string        `json:"residual_risk,omitempty"`
	AcceptedBy     string        `json:"accepted_by,omitempty"`
	AcceptedAt     *time.Time    `json:"accepted_at,omitempty"`
}
type Review struct {
	ID                string        `json:"id"`
	RepositoryID      string        `json:"repository_id"`
	PullRequestID     string        `json:"pull_request_id"`
	SourceRevision    string        `json:"source_revision"`
	TargetRevision    string        `json:"target_revision"`
	SourceFlowID      string        `json:"source_flow_id"`
	SourceFlowVersion int           `json:"source_flow_version"`
	TargetFlowID      string        `json:"target_flow_id"`
	TargetFlowVersion int           `json:"target_flow_version"`
	Changes           []Change      `json:"changes"`
	Requirements      []Requirement `json:"requirements"`
	Comments          []Comment     `json:"comments"`
	ResidualRisk      string        `json:"residual_risk,omitempty"`
	AcceptedBy        string        `json:"accepted_by,omitempty"`
	AcceptedAt        *time.Time    `json:"accepted_at,omitempty"`
	History           []PriorReview `json:"history"`
	CreatedByType     string        `json:"created_by_type"`
	CreatedBy         string        `json:"created_by"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
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
func (s *Store) Create(v Review) (Review, error) {
	var out Review
	err := s.lock(func() error {
		if validate(v) != nil {
			return ErrInvalid
		}
		if old, e := s.read(v.RepositoryID, v.PullRequestID); e == nil {
			if sameBoundary(old, v) {
				return ErrConflict
			}
			v.History = append(old.History, PriorReview{ID: old.ID, SourceRevision: old.SourceRevision, TargetRevision: old.TargetRevision, Changes: old.Changes, Requirements: old.Requirements, Comments: old.Comments, ResidualRisk: old.ResidualRisk, AcceptedBy: old.AcceptedBy, AcceptedAt: old.AcceptedAt})
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		now := s.now()
		v.ID = randomID()
		v.CreatedAt = now
		v.UpdatedAt = now
		v.Comments = []Comment{}
		out = v
		return s.write(v)
	})
	return out, err
}
func (s *Store) Get(repo, pull string) (Review, error) {
	var out Review
	err := s.lock(func() error { var e error; out, e = s.read(repo, pull); return e })
	return out, err
}
func (s *Store) AddComment(repo, pull, actorType, actor string, c Comment) (Review, error) {
	var out Review
	err := s.lock(func() error {
		v, e := s.read(repo, pull)
		if e != nil {
			return e
		}
		if !validComment(c) {
			return ErrInvalid
		}
		c.ID = randomID()
		c.ActorType = actorType
		c.ActorID = actor
		c.CreatedAt = s.now()
		v.Comments = append(v.Comments, c)
		v.UpdatedAt = c.CreatedAt
		out = v
		return s.write(v)
	})
	return out, err
}
func (s *Store) Accept(repo, pull, revision, actor, risk string, kinds []string) (Review, error) {
	var out Review
	err := s.lock(func() error {
		v, e := s.read(repo, pull)
		if e != nil {
			return e
		}
		if v.SourceRevision != revision || strings.TrimSpace(risk) == "" {
			return ErrConflict
		}
		wanted := map[string]bool{}
		for _, k := range kinds {
			wanted[k] = true
		}
		now := s.now()
		for i := range v.Requirements {
			if wanted[v.Requirements[i].Kind] {
				v.Requirements[i].Status = "acknowledged"
				v.Requirements[i].RecordedBy = actor
				v.Requirements[i].RecordedAt = &now
			}
		}
		for _, r := range v.Requirements {
			if r.Status != "acknowledged" {
				return ErrInvalid
			}
		}
		v.ResidualRisk = risk
		v.AcceptedBy = actor
		v.AcceptedAt = &now
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return out, err
}
func sameBoundary(a, b Review) bool {
	return a.SourceRevision == b.SourceRevision && a.TargetRevision == b.TargetRevision && a.SourceFlowID == b.SourceFlowID && a.SourceFlowVersion == b.SourceFlowVersion && a.TargetFlowID == b.TargetFlowID && a.TargetFlowVersion == b.TargetFlowVersion
}
func validate(v Review) error {
	if v.RepositoryID == "" || v.PullRequestID == "" || len(v.SourceRevision) != 40 || len(v.TargetRevision) != 40 || v.SourceFlowID == "" || v.TargetFlowID == "" || v.SourceFlowVersion < 1 || v.TargetFlowVersion < 1 {
		return ErrInvalid
	}
	valid := map[string]bool{"collection": true, "purpose": true, "recipient": true, "retention": true, "access": true, "user_control": true}
	for _, c := range v.Changes {
		if !valid[c.Kind] || c.Summary == "" || len(c.SourceIDs) == 0 {
			return ErrInvalid
		}
	}
	req := map[string]bool{"owner_acknowledgement": true, "notice": true, "consent": true, "migration": true, "test": true, "exception": true}
	for _, r := range v.Requirements {
		if !req[r.Kind] || r.Reason == "" || r.Status != "required" {
			return ErrInvalid
		}
	}
	return nil
}
func validComment(c Comment) bool {
	if c.Kind != "challenge" && c.Kind != "mitigation" && c.Kind != "residual_risk" {
		return false
	}
	if strings.TrimSpace(c.Body) == "" || len(c.Body) > 4000 {
		return false
	}
	for _, e := range c.Evidence {
		if e.Path == "" || strings.Contains(e.Path, "://") || e.StartLine < 1 || e.EndLine < e.StartLine || e.Claim == "" {
			return false
		}
	}
	return true
}
func (s *Store) path(repo, pull string) string {
	return filepath.Join(s.root, "repo-"+hex.EncodeToString([]byte(repo)), pull+".json")
}
func (s *Store) read(repo, pull string) (Review, error) {
	if pull == "" || strings.ContainsAny(pull, "/\\") {
		return Review{}, ErrNotFound
	}
	b, e := os.ReadFile(s.path(repo, pull))
	if os.IsNotExist(e) {
		return Review{}, ErrNotFound
	}
	if e != nil {
		return Review{}, e
	}
	var v Review
	if json.Unmarshal(b, &v) != nil || v.RepositoryID != repo || v.PullRequestID != pull {
		return Review{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Review) error {
	p := s.path(v.RepositoryID, v.PullRequestID)
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".privacy-review-*")
	if e != nil {
		return e
	}
	n := f.Name()
	defer os.Remove(n)
	_ = f.Chmod(0600)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(n, p)
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
func randomID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
