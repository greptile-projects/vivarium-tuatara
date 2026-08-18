// Package interfacesystems persists governed, versioned interface-system definitions.
package interfacesystems

import (
	"crypto/rand"
	"encoding/base64"
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

var ErrNotFound = errors.New("interface system not found")
var ErrInvalid = errors.New("invalid interface system")
var ErrConflict = errors.New("interface system version conflict")

type Constraint struct {
	Accessibility []string `json:"accessibility"`
	Localization  []string `json:"localization"`
}
type Token struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Value       string   `json:"value"`
	Theme       string   `json:"theme"`
	Description string   `json:"description"`
	OwnerIDs    []string `json:"owner_ids"`
}
type Example struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Properties  map[string]string `json:"properties"`
}
type Definition struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Usage       string     `json:"usage"`
	SourcePath  string     `json:"source_path"`
	OwnerIDs    []string   `json:"owner_ids"`
	Constraints Constraint `json:"constraints"`
	Examples    []Example  `json:"examples"`
}
type ResponsiveRule struct {
	Name      string   `json:"name"`
	Condition string   `json:"condition"`
	Behavior  string   `json:"behavior"`
	OwnerIDs  []string `json:"owner_ids"`
}
type Implementation struct {
	Consumer       string `json:"consumer"`
	RepositoryID   string `json:"repository_id,omitempty"`
	ReleaseID      string `json:"release_id,omitempty"`
	CommitID       string `json:"commit_id"`
	DefinitionName string `json:"definition_name"`
	Status         string `json:"status"`
	Notes          string `json:"notes,omitempty"`
}
type AdoptionPolicy struct {
	Level              string   `json:"level"`
	SupportedConsumers []string `json:"supported_consumers"`
	Exceptions         []string `json:"exceptions"`
	MigrationGuidance  string   `json:"migration_guidance"`
}
type Revision struct {
	Version             int              `json:"version"`
	Title               string           `json:"title"`
	Summary             string           `json:"summary"`
	Rationale           string           `json:"rationale"`
	CommitID            string           `json:"commit_id"`
	ReleaseID           string           `json:"release_id"`
	ReleaseVersion      string           `json:"release_version"`
	OwnerIDs            []string         `json:"owner_ids"`
	Themes              []string         `json:"themes"`
	Tokens              []Token          `json:"tokens"`
	Components          []Definition     `json:"components"`
	InteractionPatterns []Definition     `json:"interaction_patterns"`
	ContentRules        []Definition     `json:"content_rules"`
	ResponsiveRules     []ResponsiveRule `json:"responsive_rules"`
	AdoptionPolicy      AdoptionPolicy   `json:"adoption_policy"`
	Implementations     []Implementation `json:"implementations"`
	CreatedBy           string           `json:"created_by"`
	CreatedAt           time.Time        `json:"created_at"`
}
type Diagnostic struct {
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Definition string `json:"definition,omitempty"`
	Consumer   string `json:"consumer,omitempty"`
}
type System struct {
	ID             string       `json:"id"`
	RepositoryID   string       `json:"repository_id"`
	CurrentVersion int          `json:"current_version"`
	Revisions      []Revision   `json:"revisions"`
	Diagnostics    []Diagnostic `json:"diagnostics"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
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
func (s *Store) Create(repo, actor string, r Revision) (System, error) {
	var out System
	err := s.lock(func() error {
		if validate(r) != nil || repo == "" || actor == "" {
			return ErrInvalid
		}
		peers, err := s.listRaw(repo)
		if err != nil {
			return err
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = System{ID: randomID(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		if err = s.write(out); err != nil {
			return err
		}
		out = s.project(out, peers)
		return nil
	})
	return out, err
}
func (s *Store) Revise(id string, expected int, actor string, r Revision) (System, error) {
	var out System
	err := s.lock(func() error {
		v, err := s.read(id)
		if err != nil {
			return err
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validate(r) != nil || actor == "" {
			return ErrInvalid
		}
		peers, err := s.listRaw(v.RepositoryID)
		if err != nil {
			return err
		}
		stamp(&r, expected+1, actor, s.now())
		v.CurrentVersion = r.Version
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		if err = s.write(v); err != nil {
			return err
		}
		out = s.project(out, peers)
		return nil
	})
	return out, err
}
func (s *Store) Get(id string) (System, error) {
	var out System
	err := s.lock(func() error { var err error; out, err = s.read(id); return err })
	if err != nil {
		return System{}, err
	}
	peers, err := s.List(out.RepositoryID)
	if err != nil {
		return System{}, err
	}
	return s.project(out, peers), nil
}
func (s *Store) List(repo string) ([]System, error) {
	raw := []System{}
	err := s.lock(func() error {
		var err error
		raw, err = s.listRaw(repo)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]System, 0, len(raw))
	for _, v := range raw {
		out = append(out, s.project(v, raw))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *Store) listRaw(repo string) ([]System, error) {
	raw := []System{}
	entries, err := os.ReadDir(s.repoDir(repo))
	if os.IsNotExist(err) {
		return raw, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		value, err := s.readFile(filepath.Join(s.repoDir(repo), entry.Name()))
		if err != nil {
			return nil, err
		}
		if value.RepositoryID == repo {
			raw = append(raw, value)
		}
	}
	return raw, nil
}
func stamp(r *Revision, version int, actor string, now time.Time) {
	r.Version = version
	r.CreatedBy = actor
	r.CreatedAt = now
}
func validate(r Revision) error {
	if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Summary) == "" || strings.TrimSpace(r.Rationale) == "" || len(r.CommitID) != 40 || r.ReleaseID == "" || r.ReleaseVersion == "" || len(r.Themes) == 0 {
		return ErrInvalid
	}
	levels := map[string]bool{"experimental": true, "recommended": true, "required": true}
	if !levels[r.AdoptionPolicy.Level] || r.AdoptionPolicy.MigrationGuidance == "" {
		return ErrInvalid
	}
	if len(r.Tokens)+len(r.Components)+len(r.InteractionPatterns)+len(r.ContentRules)+len(r.ResponsiveRules) == 0 {
		return ErrInvalid
	}
	for _, x := range r.Tokens {
		if x.Name == "" || x.Category == "" || x.Value == "" || x.Theme == "" || x.Description == "" {
			return ErrInvalid
		}
	}
	for _, group := range [][]Definition{r.Components, r.InteractionPatterns, r.ContentRules} {
		for _, x := range group {
			if x.Name == "" || x.Description == "" || x.Usage == "" || x.SourcePath == "" || len(x.Examples) == 0 {
				return ErrInvalid
			}
			for _, e := range x.Examples {
				if e.Title == "" || e.Description == "" {
					return ErrInvalid
				}
			}
		}
	}
	for _, x := range r.ResponsiveRules {
		if x.Name == "" || x.Condition == "" || x.Behavior == "" {
			return ErrInvalid
		}
	}
	statuses := map[string]bool{"current": true, "stale": true, "unsupported": true, "unknown": true}
	for _, x := range r.Implementations {
		if x.Consumer == "" || x.ReleaseID == "" || len(x.CommitID) != 40 || x.DefinitionName == "" || !statuses[x.Status] {
			return ErrInvalid
		}
	}
	return nil
}
func (s *Store) project(v System, peers []System) System {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	add := func(k, sev, msg, def, consumer string) { d = append(d, Diagnostic{k, sev, msg, def, consumer}) }
	if len(r.OwnerIDs) == 0 {
		add("missing_owner", "blocking", "The interface system has no accountable owner.", "", "")
	}
	definitions := map[string]bool{}
	for _, t := range r.Tokens {
		definitions["token:"+t.Name] = true
		if len(t.OwnerIDs) == 0 {
			add("missing_owner", "warning", "A design token has no accountable owner.", t.Name, "")
		}
	}
	for kind, group := range map[string][]Definition{"component": r.Components, "interaction": r.InteractionPatterns, "content": r.ContentRules} {
		for _, x := range group {
			definitions[kind+":"+x.Name] = true
			if len(x.OwnerIDs) == 0 {
				add("missing_owner", "warning", "A reusable definition has no accountable owner.", x.Name, "")
			}
		}
	}
	for _, x := range r.ResponsiveRules {
		if len(x.OwnerIDs) == 0 {
			add("missing_owner", "warning", "A responsive rule has no accountable owner.", x.Name, "")
		}
	}
	supported := map[string]bool{}
	for _, c := range r.AdoptionPolicy.SupportedConsumers {
		supported[c] = true
	}
	for _, x := range r.Implementations {
		if !supported[x.Consumer] || x.Status == "unsupported" {
			add("unsupported_consumer", "blocking", "An implementation targets a consumer outside the adoption policy.", x.DefinitionName, x.Consumer)
		}
		if x.Status == "stale" {
			add("stale_implementation", "warning", "An implementation is not current with this interface-system revision.", x.DefinitionName, x.Consumer)
		}
		if x.Status == "unknown" {
			add("unknown_implementation", "warning", "Implementation currency has not been established.", x.DefinitionName, x.Consumer)
		}
	}
	for _, p := range peers {
		if p.ID == v.ID || len(p.Revisions) == 0 {
			continue
		}
		pr := p.Revisions[len(p.Revisions)-1]
		for _, t := range pr.Tokens {
			if definitions["token:"+t.Name] {
				add("conflicting_definition", "blocking", "Another current interface system defines the same token.", t.Name, "")
			}
		}
		for kind, group := range map[string][]Definition{"component": pr.Components, "interaction": pr.InteractionPatterns, "content": pr.ContentRules} {
			for _, x := range group {
				if definitions[kind+":"+x.Name] {
					add("conflicting_definition", "blocking", "Another current interface system defines the same reusable decision.", x.Name, "")
				}
			}
		}
	}
	v.Diagnostics = d
	return v
}
func (s *Store) repoDir(repo string) string {
	return filepath.Join(s.root, "repo-"+base64.RawURLEncoding.EncodeToString([]byte(repo)))
}
func (s *Store) read(id string) (System, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return System{}, e
	}
	for _, entry := range entries {
		if entry.IsDir() {
			v, err := s.readFile(filepath.Join(s.root, entry.Name(), id+".json"))
			if !errors.Is(err, ErrNotFound) {
				return v, err
			}
		}
	}
	return System{}, ErrNotFound
}
func (s *Store) readFile(name string) (System, error) {
	var v System
	b, e := os.ReadFile(name)
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v System) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	dir := s.repoDir(v.RepositoryID)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".interface-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
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
		e = os.Rename(name, filepath.Join(dir, v.ID+".json"))
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
func randomID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
