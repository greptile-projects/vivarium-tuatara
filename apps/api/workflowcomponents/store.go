// Package workflowcomponents retains attested collaboration components and
// repository-local, pull-reviewed installations. A component describes a
// contract; it never carries executable source or authority into a consumer.
package workflowcomponents

import (
	"crypto/rand"
	"crypto/sha256"
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

var ErrInvalid = errors.New("invalid workflow component")
var ErrNotFound = errors.New("workflow component not found")
var ErrConflict = errors.New("workflow component version conflict")

var namePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,98}[a-z0-9])?$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)

type Field struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}
type Contract struct {
	Inputs  []Field `json:"inputs"`
	Outputs []Field `json:"outputs"`
}
type Source struct {
	RepositoryID   string `json:"repository_id"`
	Revision       string `json:"revision"`
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	PackageName    string `json:"package_name"`
	PackageVersion string `json:"package_version"`
	PackageSHA256  string `json:"package_sha256"`
	Boundary       string `json:"boundary"`
	PeerID         string `json:"peer_id,omitempty"`
}
type Capability struct {
	Name     string `json:"name"`
	Reason   string `json:"reason"`
	Optional bool   `json:"optional,omitempty"`
}
type DataUse struct {
	Classification string   `json:"classification"`
	Purpose        string   `json:"purpose"`
	Retention      string   `json:"retention"`
	Destinations   []string `json:"destinations"`
}
type Test struct {
	Name          string `json:"name"`
	CommandSHA256 string `json:"command_sha256"`
	Revision      string `json:"revision"`
	Outcome       string `json:"outcome"`
}
type Compatibility struct {
	WorkflowFormat int      `json:"workflow_format"`
	Platforms      []string `json:"platforms"`
	Predecessor    string   `json:"predecessor,omitempty"`
	Breaking       bool     `json:"breaking"`
	Migration      string   `json:"migration,omitempty"`
}
type Support struct {
	MaintainerIDs []string `json:"maintainer_ids"`
	Policy        string   `json:"policy"`
	Until         string   `json:"until,omitempty"`
	Contact       string   `json:"contact"`
}
type Definition struct {
	Name                  string        `json:"name"`
	Version               string        `json:"version"`
	Summary               string        `json:"summary"`
	Contract              Contract      `json:"contract"`
	RequestedCapabilities []Capability  `json:"requested_capabilities"`
	DataUse               []DataUse     `json:"data_use"`
	Compatibility         Compatibility `json:"compatibility"`
	Tests                 []Test        `json:"tests"`
	Support               Support       `json:"support"`
}
type Attestation struct {
	PublisherID      string    `json:"publisher_id"`
	PublishedAt      time.Time `json:"published_at"`
	DefinitionSHA256 string    `json:"definition_sha256"`
	Statement        string    `json:"statement"`
}
type Component struct {
	ID          string      `json:"id"`
	Definition  Definition  `json:"definition"`
	Source      Source      `json:"source"`
	Attestation Attestation `json:"attestation"`
	PublishedAt time.Time   `json:"published_at"`
}
type Mapping struct {
	Capability      string `json:"capability"`
	LocalPermission string `json:"local_permission"`
}
type InstallationRevision struct {
	Version          int            `json:"version"`
	ComponentID      string         `json:"component_id"`
	ComponentVersion string         `json:"component_version"`
	PullID           string         `json:"pull_id"`
	PullRevision     string         `json:"pull_revision"`
	Mappings         []Mapping      `json:"mappings"`
	Configuration    map[string]any `json:"configuration"`
	AcceptedDataUse  []string       `json:"accepted_data_use"`
	InstalledBy      string         `json:"installed_by"`
	InstalledAt      time.Time      `json:"installed_at"`
	Status           string         `json:"status"`
	Diagnostics      []string       `json:"diagnostics"`
}
type Installation struct {
	ID             string                 `json:"id"`
	RepositoryID   string                 `json:"repository_id"`
	Name           string                 `json:"name"`
	CurrentVersion int                    `json:"current_version"`
	Revisions      []InstallationRevision `json:"revisions"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
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

func (s *Store) Publish(d Definition, src Source, actor string) (Component, error) {
	if !validDefinition(d) || !validSource(src) || len(actor) != 32 {
		return Component{}, ErrInvalid
	}
	body, _ := json.Marshal(d)
	sum := sha256.Sum256(body)
	id := src.RepositoryID + ":" + d.Name + "@" + d.Version
	c := Component{ID: id, Definition: d, Source: src, Attestation: Attestation{PublisherID: actor, DefinitionSHA256: hex.EncodeToString(sum[:]), Statement: "publisher attests that the exact contract, provenance, tests, data use, compatibility, and support terms describe this immutable version"}}
	err := s.lock(func() error {
		if old, e := s.readComponent(id); e == nil {
			if old.Attestation.DefinitionSHA256 == c.Attestation.DefinitionSHA256 && old.Source == c.Source {
				c = old
				return nil
			}
			return ErrConflict
		}
		c.PublishedAt = s.now()
		c.Attestation.PublishedAt = c.PublishedAt
		return atomicJSON(s.componentPath(id), c)
	})
	return c, err
}
func (s *Store) Get(id string) (Component, error) { return s.readComponent(id) }
func (s *Store) List() (out []Component, err error) {
	entries, e := os.ReadDir(filepath.Join(s.root, "components"))
	if errors.Is(e, os.ErrNotExist) {
		return []Component{}, nil
	}
	if e != nil {
		return nil, e
	}
	for _, x := range entries {
		if x.IsDir() {
			continue
		}
		var c Component
		if readJSON(filepath.Join(s.root, "components", x.Name()), &c) == nil {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishedAt.Before(out[j].PublishedAt) })
	return
}
func (s *Store) Install(repo, name, actor, pull, pullRevision string, c Component, mappings []Mapping, config map[string]any, data []string, expected int) (Installation, error) {
	if len(repo) != 32 || len(actor) != 32 || len(pull) == 0 || len(pullRevision) != 40 || !validInstall(c, mappings, config, data) {
		return Installation{}, ErrInvalid
	}
	id := repo + ":" + name
	var out Installation
	err := s.lock(func() error {
		old, e := s.readInstallation(id)
		now := s.now()
		if errors.Is(e, ErrNotFound) {
			if expected != 0 {
				return ErrConflict
			}
			old = Installation{ID: id, RepositoryID: repo, Name: name, CreatedAt: now}
		} else if e != nil {
			return e
		}
		if old.CurrentVersion != expected {
			return ErrConflict
		}
		r := InstallationRevision{Version: expected + 1, ComponentID: c.ID, ComponentVersion: c.Definition.Version, PullID: pull, PullRevision: pullRevision, Mappings: mappings, Configuration: config, AcceptedDataUse: data, InstalledBy: actor, InstalledAt: now, Status: "active", Diagnostics: []string{}}
		old.CurrentVersion = r.Version
		old.Revisions = append(old.Revisions, r)
		old.UpdatedAt = now
		out = old
		return atomicJSON(s.installationPath(id), old)
	})
	return out, err
}
func (s *Store) GetInstallation(repo, name string) (Installation, error) {
	return s.readInstallation(repo + ":" + name)
}
func (s *Store) ListInstallations(repo string) (out []Installation, err error) {
	entries, e := os.ReadDir(filepath.Join(s.root, "installations"))
	if errors.Is(e, os.ErrNotExist) {
		return []Installation{}, nil
	}
	if e != nil {
		return nil, e
	}
	for _, x := range entries {
		var v Installation
		if readJSON(filepath.Join(s.root, "installations", x.Name()), &v) == nil && v.RepositoryID == repo {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return
}
func (s *Store) Resolve(repo, ref string) (Component, Installation, bool) {
	parts := strings.Split(ref, "@")
	if len(parts) != 2 {
		return Component{}, Installation{}, false
	}
	in, e := s.GetInstallation(repo, parts[0])
	if e != nil || len(in.Revisions) == 0 {
		return Component{}, Installation{}, false
	}
	r := in.Revisions[len(in.Revisions)-1]
	if r.Status != "active" || r.ComponentVersion != parts[1] {
		return Component{}, Installation{}, false
	}
	c, e := s.Get(r.ComponentID)
	return c, in, e == nil
}

func validDefinition(d Definition) bool {
	if !namePattern.MatchString(d.Name) || !versionPattern.MatchString(d.Version) || strings.TrimSpace(d.Summary) == "" || len(d.Summary) > 500 || d.Compatibility.WorkflowFormat != 1 || len(d.Support.MaintainerIDs) == 0 || d.Support.Policy == "" || d.Support.Contact == "" || len(d.Tests) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, f := range append(append([]Field{}, d.Contract.Inputs...), d.Contract.Outputs...) {
		if f.Name == "" || seen[f.Name] || !oneOf(f.Type, "string", "number", "boolean", "object", "array") || len(f.Description) > 1000 {
			return false
		}
		seen[f.Name] = true
	}
	seen = map[string]bool{}
	for _, c := range d.RequestedCapabilities {
		if c.Name == "" || c.Reason == "" || seen[c.Name] {
			return false
		}
		seen[c.Name] = true
	}
	for _, u := range d.DataUse {
		if !oneOf(u.Classification, "none", "public", "internal", "personal", "sensitive") || u.Purpose == "" || u.Retention == "" {
			return false
		}
	}
	for _, t := range d.Tests {
		if t.Name == "" || len(t.CommandSHA256) != 64 || len(t.Revision) != 40 || t.Outcome != "passed" {
			return false
		}
	}
	if d.Compatibility.Breaking && d.Compatibility.Migration == "" {
		return false
	}
	return true
}
func validSource(s Source) bool {
	return len(s.RepositoryID) == 32 && len(s.Revision) == 40 && s.Path != "" && len(s.SHA256) == 64 && s.PackageName != "" && s.PackageVersion != "" && len(s.PackageSHA256) == 64 && oneOf(s.Boundary, "package", "federation") && (s.Boundary != "federation" || s.PeerID != "")
}
func validInstall(c Component, m []Mapping, config map[string]any, data []string) bool {
	if len(m) != len(c.Definition.RequestedCapabilities) {
		return false
	}
	req := map[string]bool{}
	for _, x := range c.Definition.RequestedCapabilities {
		req[x.Name] = true
	}
	seen := map[string]bool{}
	for _, x := range m {
		if !req[x.Capability] || seen[x.Capability] || x.LocalPermission == "" || strings.Contains(strings.ToLower(x.LocalPermission), "token") {
			return false
		}
		seen[x.Capability] = true
	}
	if raw, _ := json.Marshal(config); len(raw) > 16384 || credentialLike(string(raw)) {
		return false
	}
	accepted := map[string]bool{}
	for _, x := range data {
		accepted[x] = true
	}
	for _, x := range c.Definition.DataUse {
		if !accepted[x.Classification+":"+x.Purpose] {
			return false
		}
	}
	return true
}
func credentialLike(v string) bool {
	v = strings.ToLower(v)
	return strings.Contains(v, "bearer ") || strings.Contains(v, "private_key") || strings.Contains(v, "api_key") || strings.Contains(v, "password")
}
func oneOf(v string, x ...string) bool {
	for _, a := range x {
		if v == a {
			return true
		}
	}
	return false
}
func key(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:]) + ".json"
}
func (s *Store) componentPath(id string) string { return filepath.Join(s.root, "components", key(id)) }
func (s *Store) installationPath(id string) string {
	return filepath.Join(s.root, "installations", key(id))
}
func (s *Store) readComponent(id string) (Component, error) {
	var v Component
	if readJSON(s.componentPath(id), &v) != nil || v.ID != id {
		return Component{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) readInstallation(id string) (Installation, error) {
	var v Installation
	if readJSON(s.installationPath(id), &v) != nil || v.ID != id {
		return Installation{}, ErrNotFound
	}
	return v, nil
}
func readJSON(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func atomicJSON(path string, v any) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".staged-")
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
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(n, path)
	}
	if e == nil {
		d, x := os.Open(filepath.Dir(path))
		if x == nil {
			e = d.Sync()
			d.Close()
		}
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
func randomID() string { b := make([]byte, 16); rand.Read(b); return hex.EncodeToString(b) }
