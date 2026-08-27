// Package adoptioncampaigns retains immutable release-adoption commitments.
package adoptioncampaigns

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("adoption campaign not found")
var ErrInvalid = errors.New("invalid adoption campaign")
var ErrConflict = errors.New("adoption campaign conflict")

type Audience struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type StartingVersion struct {
	Product     string `json:"product"`
	Constraint  string `json:"constraint"`
	UpgradePath string `json:"upgrade_path"`
	Supported   bool   `json:"supported"`
}
type Measure struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Target   string `json:"target"`
	Evidence string `json:"evidence"`
}
type Link struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	ResourceID  string `json:"resource_id"`
	Revision    string `json:"revision,omitempty"`
	Label       string `json:"label"`
	Requirement string `json:"requirement,omitempty"`
}
type Revision struct {
	RequestID        string            `json:"request_id"`
	Version          int               `json:"version"`
	ReleaseID        string            `json:"release_id"`
	ReleaseRevision  string            `json:"release_revision"`
	AttestationID    string            `json:"attestation_id"`
	Title            string            `json:"title"`
	Purpose          string            `json:"purpose"`
	Audiences        []Audience        `json:"target_audiences"`
	StartingVersions []StartingVersion `json:"supported_starting_versions"`
	DesiredCoverage  string            `json:"desired_coverage"`
	Deadline         time.Time         `json:"deadline"`
	SuccessMeasures  []Measure         `json:"success_measures"`
	SupportPolicy    string            `json:"support_policy"`
	RollbackPolicy   string            `json:"rollback_policy"`
	OwnerIDs         []string          `json:"owner_ids"`
	Links            []Link            `json:"links"`
	CreatedBy        string            `json:"created_by"`
	CreatedAt        time.Time         `json:"created_at"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	AttributedTo string `json:"attributed_to"`
	LinkID       string `json:"link_id,omitempty"`
}
type Campaign struct {
	ID             string       `json:"id"`
	RepositoryID   string       `json:"repository_id"`
	RequestID      string       `json:"request_id"`
	RequestDigest  string       `json:"request_digest"`
	CurrentVersion int          `json:"current_version"`
	Revisions      []Revision   `json:"revisions"`
	Diagnostics    []Diagnostic `json:"diagnostics"`
	Authority      string       `json:"authority"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}
type Projection struct {
	ReleaseSuperseded bool
	MissingOwners     []string
	InvalidLinks      []string
}
type Store struct {
	root    string
	mu      sync.Mutex
	now     func() time.Time
	project func(Campaign) Projection
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
func (s *Store) ConfigureProjection(fn func(Campaign) Projection) { s.project = fn }
func (s *Store) Create(repo, actor, request string, r Revision) (Campaign, error) {
	var out Campaign
	err := s.lock(func() error {
		if request == "" || validate(r) != nil {
			return ErrInvalid
		}
		d := digest(r)
		id := stable(repo, actor, request)
		if v, e := s.read(id); e == nil {
			if v.RequestDigest != d {
				return ErrConflict
			}
			out = v
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		now := s.now()
		stamp(&r, actor, request, 1, now)
		out = Campaign{ID: id, RepositoryID: repo, RequestID: request, RequestDigest: d, CurrentVersion: 1, Revisions: []Revision{r}, Authority: "Campaigns coordinate adoption only and grant no repository, release, package, API, deployment, support, rollback, disclosure, spending, or operational authority.", CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.decorate(out), err
}
func (s *Store) Revise(repo, id string, expected int, actor, request string, r Revision) (Campaign, error) {
	var out Campaign
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil || v.RepositoryID != repo {
			if e == nil {
				return ErrNotFound
			}
			return e
		}
		d := digest(r)
		for _, x := range v.Revisions {
			if x.RequestID == request {
				if digest(x) != d {
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
	return s.decorate(out), err
}
func (s *Store) Get(id string) (Campaign, error) {
	var v Campaign
	e := s.lock(func() error { var x error; v, x = s.read(id); return x })
	return s.decorate(v), e
}
func (s *Store) List(repo string) ([]Campaign, error) {
	out := []Campaign{}
	e := s.lock(func() error {
		es, x := os.ReadDir(s.root)
		if x != nil {
			return x
		}
		for _, f := range es {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			v, x := s.read(strings.TrimSuffix(f.Name(), ".json"))
			if x != nil {
				return x
			}
			if v.RepositoryID == repo {
				out = append(out, v)
			}
		}
		return nil
	})
	for i := range out {
		out[i] = s.decorate(out[i])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, e
}
func validate(r Revision) error {
	if r.ReleaseID == "" || len(r.ReleaseRevision) != 40 || r.AttestationID == "" || strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Purpose) == "" || len(r.Audiences) == 0 || len(r.StartingVersions) == 0 || strings.TrimSpace(r.DesiredCoverage) == "" || r.Deadline.IsZero() || len(r.SuccessMeasures) == 0 || strings.TrimSpace(r.SupportPolicy) == "" || strings.TrimSpace(r.RollbackPolicy) == "" || len(r.OwnerIDs) == 0 || len(r.Links) == 0 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, a := range r.Audiences {
		if a.ID == "" || a.Name == "" || a.Description == "" || seen["a:"+a.ID] {
			return ErrInvalid
		}
		seen["a:"+a.ID] = true
	}
	for _, v := range r.StartingVersions {
		if v.Product == "" || v.Constraint == "" || v.UpgradePath == "" {
			return ErrInvalid
		}
	}
	for _, m := range r.SuccessMeasures {
		if m.ID == "" || m.Name == "" || m.Target == "" || m.Evidence == "" || seen["m:"+m.ID] {
			return ErrInvalid
		}
		seen["m:"+m.ID] = true
	}
	kinds := map[string]bool{"change": true, "decision": true, "documentation": true, "package": true, "api": true, "schema": true, "compatibility": true}
	for _, l := range r.Links {
		if l.ID == "" || !kinds[l.Kind] || l.ResourceID == "" || l.Label == "" || seen["l:"+l.ID] {
			return ErrInvalid
		}
		seen["l:"+l.ID] = true
	}
	return nil
}
func stamp(r *Revision, actor, request string, v int, now time.Time) {
	r.RequestID = request
	r.Version = v
	r.CreatedBy = actor
	r.CreatedAt = now
}
func (s *Store) decorate(v Campaign) Campaign {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	ds := []Diagnostic{}
	add := func(k, m, w string) {
		ds = append(ds, Diagnostic{Kind: k, Severity: "warning", Message: m, AttributedTo: w})
	}
	p := Projection{}
	if s.project != nil {
		p = s.project(v)
	}
	if p.ReleaseSuperseded {
		add("superseded_release", "a newer repository release supersedes the campaign release", r.CreatedBy)
	}
	for _, o := range p.MissingOwners {
		add("missing_owner", "accountable owner "+o+" is no longer a repository participant", o)
	}
	for _, id := range p.InvalidLinks {
		ds = append(ds, Diagnostic{Kind: "changed_commitment", Severity: "warning", Message: "linked commitment no longer resolves at its retained identity", AttributedTo: r.CreatedBy, LinkID: id})
	}
	for _, sv := range r.StartingVersions {
		if !sv.Supported {
			add("unsupported_upgrade_path", sv.Product+" "+sv.Constraint+" is explicitly unsupported", r.CreatedBy)
		}
	}
	all, _ := s.listRaw()
	for _, x := range all {
		if x.ID == v.ID || x.RepositoryID != v.RepositoryID || len(x.Revisions) == 0 {
			continue
		}
		q := x.Revisions[len(x.Revisions)-1]
		if q.ReleaseID == r.ReleaseID && overlap(r.Audiences, q.Audiences) {
			add("conflicting_campaign", "campaign "+x.ID+" targets the same release audience", q.CreatedBy)
		}
	}
	v.Diagnostics = ds
	return v
}
func overlap(a, b []Audience) bool {
	m := map[string]bool{}
	for _, x := range a {
		m[x.ID] = true
	}
	for _, x := range b {
		if m[x.ID] {
			return true
		}
	}
	return false
}
func digest(r Revision) string {
	r.RequestID = ""
	r.Version = 0
	r.CreatedBy = ""
	r.CreatedAt = time.Time{}
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func stable(xs ...string) string {
	h := sha256.Sum256([]byte(strings.Join(xs, "\x00")))
	return hex.EncodeToString(h[:16])
}
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Campaign, error) {
	b, e := os.ReadFile(s.path(id))
	if os.IsNotExist(e) {
		return Campaign{}, ErrNotFound
	}
	var v Campaign
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *Store) write(v Campaign) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.root, ".campaign-*")
	if e != nil {
		return e
	}
	n := f.Name()
	defer os.Remove(n)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	if x := f.Close(); e == nil {
		e = x
	}
	if e == nil {
		e = os.Rename(n, s.path(v.ID))
	}
	return e
}
func (s *Store) listRaw() ([]Campaign, error) {
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Campaign{}
	for _, f := range es {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		v, x := s.read(strings.TrimSuffix(f.Name(), ".json"))
		if x != nil {
			return nil, x
		}
		out = append(out, v)
	}
	return out, nil
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
