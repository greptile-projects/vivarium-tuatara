// Package historyremediations retains restricted, payload-free history-repair boundaries.
package historyremediations

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

var ErrNotFound = errors.New("history remediation not found")
var ErrInvalid = errors.New("invalid history remediation")
var ErrConflict = errors.New("history remediation request changed")

type Source struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision,omitempty"`
}
type Scope struct {
	RepositoryID   string `json:"repository_id"`
	Kind           string `json:"kind"`
	ObjectID       string `json:"object_id"`
	Revision       string `json:"revision,omitempty"`
	Ref            string `json:"ref,omitempty"`
	ReleaseID      string `json:"release_id,omitempty"`
	Package        string `json:"package,omitempty"`
	ArtifactDigest string `json:"artifact_digest,omitempty"`
	EnvironmentID  string `json:"environment_id,omitempty"`
}
type Evidence struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	ResourceID   string `json:"resource_id"`
	SHA256       string `json:"sha256"`
	State        string `json:"state"`
	Note         string `json:"note,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type Constraint struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	ResourceID   string `json:"resource_id,omitempty"`
	State        string `json:"state"`
	Reason       string `json:"reason"`
	AttributedTo string `json:"attributed_to"`
}
type Approval struct {
	Role        string   `json:"role"`
	ApproverIDs []string `json:"approver_ids"`
	Required    int      `json:"required"`
}
type Remediation struct {
	ID                 string       `json:"id"`
	RepositoryID       string       `json:"repository_id"`
	RequestID          string       `json:"request_id"`
	RequestDigest      string       `json:"request_digest,omitempty"`
	Title              string       `json:"title"`
	Source             Source       `json:"source"`
	ContentDescription string       `json:"content_description"`
	Reason             string       `json:"reason"`
	Scopes             []Scope      `json:"scopes"`
	Evidence           []Evidence   `json:"discovery_evidence"`
	Constraints        []Constraint `json:"constraints"`
	AudienceIDs        []string     `json:"audience_ids"`
	OwnerIDs           []string     `json:"owner_ids"`
	RequiredApprovals  []Approval   `json:"required_approvals"`
	CreatedBy          string       `json:"created_by"`
	CreatedAt          time.Time    `json:"created_at"`
	Authority          string       `json:"authority"`
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
func randomID() string                       { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(repo, id string) string { return filepath.Join(s.root, repo, id+".json") }
func valid(v Remediation) bool {
	if strings.TrimSpace(v.RequestID) == "" || strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.ContentDescription) == "" || strings.TrimSpace(v.Reason) == "" || !map[string]bool{"security_finding": true, "privacy_incident": true, "support_case": true, "selected_object": true}[v.Source.Kind] || strings.TrimSpace(v.Source.ResourceID) == "" || len(v.Scopes) == 0 || len(v.Evidence) == 0 || len(v.AudienceIDs) == 0 || len(v.OwnerIDs) == 0 || len(v.RequiredApprovals) == 0 {
		return false
	}
	// This ledger accepts bounded descriptions and digests, never copied payloads or logs.
	if len(v.Title) > 160 || len(v.ContentDescription) > 500 || len(v.Reason) > 1000 || strings.ContainsAny(v.ContentDescription, "\r\n") {
		return false
	}
	for _, x := range v.Scopes {
		if x.RepositoryID == "" || x.Kind == "" || x.ObjectID == "" {
			return false
		}
	}
	for _, x := range v.Evidence {
		if x.Kind == "" || x.ResourceID == "" || len(x.SHA256) != 64 || !map[string]bool{"matched": true, "false_match": true, "inaccessible": true}[x.State] || x.AttributedTo == "" {
			return false
		}
	}
	for _, x := range v.Constraints {
		if !map[string]bool{"legal_hold": true, "retention_commitment": true, "continuity_commitment": true, "inaccessible_resource": true, "false_match": true}[x.Kind] || x.State == "" || x.Reason == "" || x.AttributedTo == "" {
			return false
		}
	}
	for _, x := range v.RequiredApprovals {
		if x.Role == "" || x.Required < 1 || x.Required > len(x.ApproverIDs) {
			return false
		}
	}
	return true
}
func (s *Store) Create(v Remediation, actor, digest string) (Remediation, error) {
	if !valid(v) {
		return Remediation{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	xs, _ := s.list(v.RepositoryID)
	for _, x := range xs {
		if x.RequestID == v.RequestID {
			if x.RequestDigest != digest {
				return Remediation{}, ErrConflict
			}
			return x, nil
		}
	}
	v.ID = randomID()
	v.RequestDigest = digest
	v.CreatedBy = actor
	v.CreatedAt = s.now()
	v.Authority = "coordination record only; grants no inspection, Git, object deletion, ref rewrite, package, artifact, release, environment, disclosure, or delivery authority"
	if err := os.MkdirAll(filepath.Dir(s.path(v.RepositoryID, v.ID)), 0700); err != nil {
		return Remediation{}, err
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	if err := os.WriteFile(s.path(v.RepositoryID, v.ID), b, 0600); err != nil {
		return Remediation{}, err
	}
	return v, nil
}
func (s *Store) list(repo string) ([]Remediation, error) {
	files, err := filepath.Glob(filepath.Join(s.root, repo, "*.json"))
	if err != nil {
		return nil, err
	}
	xs := []Remediation{}
	for _, p := range files {
		b, e := os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		var v Remediation
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		xs = append(xs, v)
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i].CreatedAt.After(xs[j].CreatedAt) })
	return xs, nil
}
func (s *Store) List(repo string) ([]Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(repo)
}
func (s *Store) Get(repo, id string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(e) {
		return Remediation{}, ErrNotFound
	}
	if e != nil {
		return Remediation{}, e
	}
	var v Remediation
	e = json.Unmarshal(b, &v)
	return v, e
}
