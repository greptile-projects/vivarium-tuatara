// Package historyremediations retains restricted, payload-free history-repair boundaries.
package historyremediations

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("history remediation not found")
var ErrInvalid = errors.New("invalid history remediation")
var ErrConflict = errors.New("history remediation request changed")
var ErrVersionConflict = errors.New("history remediation version changed")

var credentialPattern = regexp.MustCompile(`(?i)(authorization\s*:|bearer\s+[a-z0-9._-]{12,}|-----begin [a-z ]*private key-----|(?:api[_-]?key|password|passwd|secret|token)\s*[:=]\s*[^\s]{8,}|(?:ghp|github_pat|glpat-|xox[baprs]-|sk-)[a-z0-9_-]{12,}|\b(?:AKIA|ASIA)[A-Z0-9]{16}\b|\beyJ[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\b)`)

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

// ExposureFinding is a payload-free observation about one place an affected
// object (or data derived from it) may still be reachable or in use.
type ExposureFinding struct {
	ID                      string    `json:"id"`
	RequestID               string    `json:"request_id"`
	CopyKind                string    `json:"copy_kind"`
	ResourceID              string    `json:"resource_id"`
	RepositoryID            string    `json:"repository_id,omitempty"`
	ObjectIDs               []string  `json:"object_ids"`
	DerivedKinds            []string  `json:"derived_kinds,omitempty"`
	State                   string    `json:"state"`
	IndependentlyControlled bool      `json:"independently_controlled,omitempty"`
	Restricted              bool      `json:"restricted,omitempty"`
	CitationKind            string    `json:"citation_kind"`
	CitationResourceID      string    `json:"citation_resource_id"`
	CitationSHA256          string    `json:"citation_sha256"`
	Note                    string    `json:"note,omitempty"`
	Uncertainty             string    `json:"uncertainty,omitempty"`
	AttributedTo            string    `json:"attributed_to"`
	CreatedAt               time.Time `json:"created_at"`
}
type Remediation struct {
	ID                 string            `json:"id"`
	RepositoryID       string            `json:"repository_id"`
	RequestID          string            `json:"request_id"`
	RequestDigest      string            `json:"request_digest,omitempty"`
	Title              string            `json:"title"`
	Source             Source            `json:"source"`
	ContentDescription string            `json:"content_description"`
	Reason             string            `json:"reason"`
	Scopes             []Scope           `json:"scopes"`
	Evidence           []Evidence        `json:"discovery_evidence"`
	Constraints        []Constraint      `json:"constraints"`
	AudienceIDs        []string          `json:"audience_ids"`
	OwnerIDs           []string          `json:"owner_ids"`
	RequiredApprovals  []Approval        `json:"required_approvals"`
	CreatedBy          string            `json:"created_by"`
	CreatedAt          time.Time         `json:"created_at"`
	Authority          string            `json:"authority"`
	Version            int               `json:"version"`
	ExposureMap        []ExposureFinding `json:"exposure_map"`
}
type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	beforeReplace func() error
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
	encoded, err := json.Marshal(v)
	if err != nil || credentialPattern.Match(encoded) {
		return false
	}
	for _, x := range v.Scopes {
		if x.RepositoryID == "" || x.Kind == "" || x.ObjectID == "" {
			return false
		}
	}
	for _, x := range v.Evidence {
		if x.Kind == "" || x.ResourceID == "" || len(x.SHA256) != 64 || len(x.Note) > 300 || strings.ContainsAny(x.Note, "\r\n") || !map[string]bool{"matched": true, "false_match": true, "inaccessible": true}[x.State] || x.AttributedTo == "" {
			return false
		}
	}
	for _, x := range v.Constraints {
		if !map[string]bool{"legal_hold": true, "retention_commitment": true, "continuity_commitment": true, "inaccessible_resource": true, "false_match": true}[x.Kind] || x.State == "" || x.Reason == "" || len(x.Reason) > 500 || strings.ContainsAny(x.Reason, "\r\n") || x.AttributedTo == "" {
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
	v.Version = 1
	v.ExposureMap = []ExposureFinding{}
	if err := os.MkdirAll(filepath.Dir(s.path(v.RepositoryID, v.ID)), 0700); err != nil {
		return Remediation{}, err
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	if err := atomicWrite(s.path(v.RepositoryID, v.ID), b, s.beforeReplace); err != nil {
		return Remediation{}, err
	}
	return v, nil
}

func validFinding(v ExposureFinding, remediation Remediation) bool {
	kinds := map[string]bool{"branch": true, "tag": true, "pull_request": true, "fork": true, "federated_contribution": true, "workspace": true, "checkpoint": true, "cache": true, "package": true, "release_artifact": true, "documentation": true, "deployment": true, "backup": true, "active_clone": true}
	states := map[string]bool{"confirmed": true, "suspected": true, "unreachable": true, "independently_controlled": true, "unverifiable": true}
	if !kinds[v.CopyKind] || !states[v.State] || v.RequestID == "" || v.ResourceID == "" || len(v.ObjectIDs) == 0 || v.CitationKind == "" || v.CitationResourceID == "" || len(v.CitationSHA256) != 64 || len(v.Note) > 300 || len(v.Uncertainty) > 300 || strings.ContainsAny(v.Note+v.Uncertainty, "\r\n") {
		return false
	}
	allowed := map[string]bool{}
	for _, scope := range remediation.Scopes {
		allowed[scope.ObjectID] = true
	}
	for _, id := range v.ObjectIDs {
		if !allowed[id] {
			return false
		}
	}
	for _, kind := range v.DerivedKinds {
		if !map[string]bool{"credential": true, "personal_data": true, "confidential_data": true, "generated_artifact": true}[kind] {
			return false
		}
	}
	b, err := json.Marshal(v)
	return err == nil && !credentialPattern.Match(b)
}

// AddExposureFinding appends under compare-and-swap and reconciles retries.
func (s *Store) AddExposureFinding(repo, id string, expected int, in ExposureFinding, actor string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(err) {
		return Remediation{}, ErrNotFound
	}
	if err != nil {
		return Remediation{}, err
	}
	var v Remediation
	if err = json.Unmarshal(b, &v); err != nil {
		return Remediation{}, err
	}
	if v.Version == 0 {
		v.Version = 1
	}
	for _, existing := range v.ExposureMap {
		if existing.RequestID == in.RequestID {
			cleanA, cleanB := existing, in
			cleanA.ID = ""
			cleanA.AttributedTo = ""
			cleanA.CreatedAt = time.Time{}
			cleanB.ID = ""
			cleanB.AttributedTo = ""
			cleanB.CreatedAt = time.Time{}
			a, _ := json.Marshal(cleanA)
			z, _ := json.Marshal(cleanB)
			if string(a) != string(z) {
				return Remediation{}, ErrConflict
			}
			return v, nil
		}
	}
	if expected != v.Version {
		return Remediation{}, ErrVersionConflict
	}
	if !validFinding(in, v) {
		return Remediation{}, ErrInvalid
	}
	in.ID = randomID()
	in.AttributedTo = actor
	in.CreatedAt = s.now()
	v.ExposureMap = append(v.ExposureMap, in)
	v.Version++
	out, _ := json.MarshalIndent(v, "", "  ")
	if err = atomicWrite(s.path(repo, id), out, s.beforeReplace); err != nil {
		return Remediation{}, err
	}
	return v, nil
}

func atomicWrite(path string, data []byte, beforeReplace func() error) (err error) {
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, ".history-remediation-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(0600); err != nil {
		return err
	}
	if _, err = temporary.Write(data); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if beforeReplace != nil {
		if err = beforeReplace(); err != nil {
			return err
		}
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		return err
	}
	directory, openErr := os.Open(dir)
	if openErr != nil {
		return openErr
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

// Reconcile returns a previously committed request before callers consult
// mutable resource and participant state. Authentication remains route-owned.
func (s *Store) Reconcile(repo, requestID, digest string) (Remediation, bool, error) {
	if strings.TrimSpace(requestID) == "" || strings.TrimSpace(digest) == "" {
		return Remediation{}, false, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.list(repo)
	if err != nil {
		return Remediation{}, false, err
	}
	for _, value := range values {
		if value.RequestID != requestID {
			continue
		}
		if value.RequestDigest != digest {
			return Remediation{}, false, ErrConflict
		}
		return value, true, nil
	}
	return Remediation{}, false, nil
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
