// Package documentation retains reviewed, repository-backed documentation definitions.
package docscollections

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotFound            = errors.New("documentation collection not found")
	ErrInvalid             = errors.New("invalid documentation collection")
	ErrConflict            = errors.New("documentation collection version changed")
	ErrDurabilityUncertain = errors.New("documentation mutation is visible but durability is uncertain")
)

type TaskSource struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Label      string `json:"label"`
}
type Reference struct {
	Path         string `json:"path,omitempty"`
	StartLine    int    `json:"start_line,omitempty"`
	EndLine      int    `json:"end_line,omitempty"`
	Revision     string `json:"revision"`
	ResourceKind string `json:"resource_kind,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Label        string `json:"label"`
}
type DraftRevision struct {
	ID           string      `json:"id"`
	Version      int         `json:"version"`
	Body         string      `json:"body"`
	RenderedHTML string      `json:"rendered_html"`
	AuthorID     string      `json:"author_id"`
	References   []Reference `json:"references"`
	CreatedAt    time.Time   `json:"created_at"`
}
type TaskEntry struct {
	ID           string      `json:"id"`
	Kind         string      `json:"kind"`
	Body         string      `json:"body"`
	ActorID      string      `json:"actor_id"`
	AgentID      string      `json:"agent_id,omitempty"`
	DraftVersion int         `json:"draft_version"`
	References   []Reference `json:"references,omitempty"`
	Uncertain    bool        `json:"uncertain,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
}
type Task struct {
	ID                    string          `json:"id"`
	RepositoryID          string          `json:"repository_id"`
	Title                 string          `json:"title"`
	Path                  string          `json:"path"`
	Branch                string          `json:"branch"`
	BaseRevision          string          `json:"base_revision"`
	Source                TaskSource      `json:"source"`
	CreatedBy             string          `json:"created_by"`
	CreatedAt             time.Time       `json:"created_at"`
	Version               int             `json:"version"`
	Drafts                []DraftRevision `json:"drafts"`
	Entries               []TaskEntry     `json:"entries"`
	WorkspaceID           string          `json:"workspace_id,omitempty"`
	PublishedCollectionID string          `json:"published_collection_id,omitempty"`
}

type Owner struct {
	ActorID string `json:"actor_id"`
	Role    string `json:"role"`
}
type VersionMapping struct {
	Label        string `json:"label"`
	SourceRef    string `json:"source_ref"`
	ReleaseID    string `json:"release_id,omitempty"`
	Revision     string `json:"revision,omitempty"`
	Status       string `json:"status,omitempty"`
	StatusDetail string `json:"status_detail,omitempty"`
}
type Link struct {
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	ResourceID string `json:"resource_id,omitempty"`
	Symbol     string `json:"symbol,omitempty"`
	Path       string `json:"path,omitempty"`
}
type Page struct {
	Path            string   `json:"path"`
	Slug            string   `json:"slug"`
	Title           string   `json:"title"`
	NavigationTitle string   `json:"navigation_title,omitempty"`
	Position        int      `json:"position"`
	SourceObjectID  string   `json:"source_object_id"`
	SourceSHA256    string   `json:"source_sha256"`
	Authors         []string `json:"authors"`
	Links           []Link   `json:"links"`
	Status          string   `json:"status,omitempty"`
	StatusDetail    string   `json:"status_detail,omitempty"`
}
type NavigationItem struct {
	Label    string `json:"label"`
	Slug     string `json:"slug"`
	Position int    `json:"position"`
}
type Rendering struct {
	Format             string `json:"format"`
	SyntaxHighlighting bool   `json:"syntax_highlighting"`
	TableOfContents    bool   `json:"table_of_contents"`
}
type PublicationPolicy struct {
	ReviewRequired bool       `json:"review_required"`
	SourceBranch   string     `json:"source_branch"`
	PublishOnMerge bool       `json:"publish_on_merge"`
	Redirects      []Redirect `json:"redirects,omitempty"`
}
type Redirect struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type Diagnostic struct {
	Code         string `json:"code"`
	Severity     string `json:"severity"`
	PagePath     string `json:"page_path,omitempty"`
	VersionLabel string `json:"version_label,omitempty"`
	Detail       string `json:"detail"`
}
type Revision struct {
	ID                string            `json:"id"`
	RepositoryID      string            `json:"repository_id"`
	CollectionID      string            `json:"collection_id"`
	Version           int               `json:"version"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	RootPath          string            `json:"root_path"`
	SourceRef         string            `json:"source_ref"`
	SourceRevision    string            `json:"source_revision"`
	Audience          string            `json:"audience"`
	Owners            []Owner           `json:"owners"`
	SupportedVersions []VersionMapping  `json:"supported_versions"`
	Navigation        []NavigationItem  `json:"navigation"`
	Rendering         Rendering         `json:"rendering"`
	PublicationPolicy PublicationPolicy `json:"publication_policy"`
	Pages             []Page            `json:"pages"`
	PublishedBy       string            `json:"published_by"`
	PublishedAt       time.Time         `json:"published_at"`
	Diagnostics       []Diagnostic      `json:"diagnostics,omitempty"`
	PublishedPullID   string            `json:"published_pull_id,omitempty"`
	SupersededBy      string            `json:"superseded_by,omitempty"`
}

type Feedback struct {
	ID               string      `json:"id"`
	RepositoryID     string      `json:"repository_id"`
	CollectionID     string      `json:"collection_id"`
	RevisionID       string      `json:"revision_id"`
	PageSlug         string      `json:"page_slug,omitempty"`
	Kind             string      `json:"kind"`
	Body             string      `json:"body"`
	VersionLabel     string      `json:"version_label,omitempty"`
	Query            string      `json:"query,omitempty"`
	Evidence         []Reference `json:"evidence,omitempty"`
	ReporterID       string      `json:"reporter_id"`
	CreatedAt        time.Time   `json:"created_at"`
	Status           string      `json:"status"`
	TriageKind       string      `json:"triage_kind,omitempty"`
	LinkedResourceID string      `json:"linked_resource_id,omitempty"`
	TriagedBy        string      `json:"triaged_by,omitempty"`
	TriagedAt        *time.Time  `json:"triaged_at,omitempty"`
}

type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	directorySync func(string) error
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: time.Now, directorySync: syncDir}, nil
}
func (s *Store) Publish(v Revision, expected int) (Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Revision{}, err
	}
	defer unlock()
	items := []Revision{}
	var listErr error
	if v.CollectionID != "" {
		items, listErr = s.list(v.RepositoryID, v.CollectionID)
		if listErr != nil {
			return Revision{}, listErr
		}
	}
	if len(items) != expected {
		return Revision{}, ErrConflict
	}
	if err = validate(v); err != nil {
		return Revision{}, err
	}
	id, err := randomID()
	if err != nil {
		return Revision{}, err
	}
	if v.CollectionID == "" {
		v.CollectionID, id = id, ""
		id, err = randomID()
		if err != nil {
			return Revision{}, err
		}
	}
	v.ID = id
	v.Version = len(items) + 1
	v.PublishedAt = s.now().UTC().Truncate(time.Microsecond)
	v.Diagnostics = nil
	dir := filepath.Join(s.root, v.RepositoryID, v.CollectionID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return Revision{}, err
	}
	if err = writeJSON(filepath.Join(dir, fmt.Sprintf("revision-%09d.json", v.Version)), v); err != nil {
		return Revision{}, err
	}
	if err = s.directorySync(dir); err != nil {
		return v, errors.Join(ErrDurabilityUncertain, err)
	}
	return v, nil
}
func (s *Store) List(repositoryID, collectionID string) ([]Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(repositoryID, collectionID)
}
func (s *Store) list(repositoryID, collectionID string) ([]Revision, error) {
	if !validID(repositoryID) || !validID(collectionID) {
		return nil, ErrNotFound
	}
	dir := filepath.Join(s.root, repositoryID, collectionID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Revision{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Revision{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "revision-") {
			continue
		}
		var v Revision
		if readJSON(filepath.Join(dir, e.Name()), &v) == nil && v.RepositoryID == repositoryID && v.CollectionID == collectionID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}
func (s *Store) Current(repositoryID, collectionID string) (Revision, error) {
	items, err := s.List(repositoryID, collectionID)
	if err != nil {
		return Revision{}, err
	}
	if len(items) == 0 {
		return Revision{}, ErrNotFound
	}
	return items[len(items)-1], nil
}
func (s *Store) Collections(repositoryID string) ([]Revision, error) {
	if !validID(repositoryID) {
		return nil, ErrNotFound
	}
	entries, err := os.ReadDir(filepath.Join(s.root, repositoryID))
	if errors.Is(err, os.ErrNotExist) {
		return []Revision{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Revision{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		items, x := s.List(repositoryID, e.Name())
		if x == nil && len(items) > 0 {
			out = append(out, items[len(items)-1])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) AddFeedback(v Feedback) (Feedback, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Feedback{}, err
	}
	defer unlock()
	if !validID(v.RepositoryID) || !validID(v.CollectionID) || !validID(v.RevisionID) || !validID(v.ReporterID) || !oneOf(v.Kind, "page_feedback", "failed_example", "search_miss", "version_mismatch") || len(strings.TrimSpace(v.Body)) < 1 || len(v.Body) > 4000 {
		return Feedback{}, ErrInvalid
	}
	if v.Kind != "search_miss" && strings.TrimSpace(v.PageSlug) == "" {
		return Feedback{}, ErrInvalid
	}
	if len(v.Evidence) > 20 {
		return Feedback{}, ErrInvalid
	}
	for _, evidence := range v.Evidence {
		if strings.TrimSpace(evidence.Label) == "" || len(evidence.Revision) != 40 || evidence.Path != "" && !cleanPath(evidence.Path) {
			return Feedback{}, ErrInvalid
		}
	}
	v.ID, err = randomID()
	if err != nil {
		return Feedback{}, err
	}
	v.CreatedAt = s.now().UTC()
	v.Status = "open"
	dir := filepath.Join(s.root, v.RepositoryID, v.CollectionID, "feedback")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return Feedback{}, err
	}
	if err = writeJSON(filepath.Join(dir, v.ID+".json"), v); err != nil {
		return Feedback{}, err
	}
	return v, nil
}
func (s *Store) Feedback(repositoryID, collectionID string) ([]Feedback, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.root, repositoryID, collectionID, "feedback")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Feedback{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Feedback{}
	for _, e := range entries {
		var v Feedback
		if !e.IsDir() && readJSON(filepath.Join(dir, e.Name()), &v) == nil && v.RepositoryID == repositoryID && v.CollectionID == collectionID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) TriageFeedback(repositoryID, collectionID, id, actor, kind, resource string) (Feedback, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Feedback{}, err
	}
	defer unlock()
	if !validID(repositoryID) || !validID(collectionID) || !validID(id) || !validID(actor) || !validID(resource) || !oneOf(kind, "issue", "proposal", "documentation_task") {
		return Feedback{}, ErrInvalid
	}
	name := filepath.Join(s.root, repositoryID, collectionID, "feedback", id+".json")
	var v Feedback
	if readJSON(name, &v) != nil {
		return Feedback{}, ErrNotFound
	}
	if v.Status != "open" {
		if v.TriageKind == kind && v.LinkedResourceID == resource {
			return v, nil
		}
		return Feedback{}, ErrConflict
	}
	now := s.now().UTC()
	v.Status = "triaged"
	v.TriageKind = kind
	v.LinkedResourceID = resource
	v.TriagedBy = actor
	v.TriagedAt = &now
	if err = writeJSON(name, v); err != nil {
		return Feedback{}, err
	}
	return v, nil
}

func (s *Store) CreateTask(v Task) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Task{}, err
	}
	defer unlock()
	if !validID(v.RepositoryID) || !validID(v.CreatedBy) || strings.TrimSpace(v.Title) == "" || len(v.Title) > 160 || !cleanPath(v.Path) || !cleanRef(v.Branch) || len(v.BaseRevision) != 40 || !oneOf(v.Source.Kind, "proposal", "issue", "pull_request", "release", "investigation", "stewardship_opportunity", "support_thread") || !validID(v.Source.ResourceID) || strings.TrimSpace(v.Source.Label) == "" || v.Source.Revision != v.BaseRevision {
		return Task{}, ErrInvalid
	}
	if v.ID == "" {
		v.ID, err = randomID()
		if err != nil {
			return Task{}, err
		}
	} else if !validID(v.ID) {
		return Task{}, ErrInvalid
	}
	v.CreatedAt = s.now().UTC()
	v.Version = 1
	v.Drafts = []DraftRevision{}
	v.Entries = []TaskEntry{}
	dir := filepath.Join(s.root, v.RepositoryID, "tasks")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return Task{}, err
	}
	if err = writeJSON(filepath.Join(dir, v.ID+".json"), v); err != nil {
		return Task{}, err
	}
	return v, nil
}
func (s *Store) GetTask(repositoryID, id string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(repositoryID) || !validID(id) {
		return Task{}, ErrNotFound
	}
	var v Task
	if readJSON(filepath.Join(s.root, repositoryID, "tasks", id+".json"), &v) != nil || v.RepositoryID != repositoryID {
		return Task{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) UpdateTask(repositoryID, id string, expected int, mutate func(*Task) error) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Task{}, err
	}
	defer unlock()
	var v Task
	if readJSON(filepath.Join(s.root, repositoryID, "tasks", id+".json"), &v) != nil {
		return Task{}, ErrNotFound
	}
	if v.Version != expected {
		return Task{}, ErrConflict
	}
	if err = mutate(&v); err != nil {
		return Task{}, err
	}
	v.Version++
	if err = writeJSON(filepath.Join(s.root, repositoryID, "tasks", id+".json"), v); err != nil {
		return Task{}, err
	}
	return v, nil
}
func validate(v Revision) error {
	if !validID(v.RepositoryID) || !validID(v.PublishedBy) || v.CollectionID != "" && !validID(v.CollectionID) || strings.TrimSpace(v.Name) == "" || len(v.Name) > 120 || !cleanPath(v.RootPath) || v.SourceRef == "" || len(v.SourceRevision) != 40 || !oneOf(v.Audience, "public", "repository", "maintainers") || len(v.Owners) > 50 || len(v.SupportedVersions) == 0 || len(v.Pages) == 0 || !oneOf(v.Rendering.Format, "markdown", "mdx", "asciidoc") || !cleanRef(v.PublicationPolicy.SourceBranch) {
		return ErrInvalid
	}
	for _, o := range v.Owners {
		if !validID(o.ActorID) || !oneOf(o.Role, "maintainer", "reviewer") {
			return ErrInvalid
		}
	}
	seen := map[string]bool{}
	for _, p := range v.Pages {
		if !cleanPath(p.Path) || p.Slug == "" || strings.Contains(p.Slug, "..") || strings.TrimSpace(p.Title) == "" || len(p.SourceObjectID) != 40 || len(p.SourceSHA256) != 64 || seen[p.Slug] {
			return ErrInvalid
		}
		seen[p.Slug] = true
		for _, l := range p.Links {
			if !oneOf(l.Kind, "symbol", "package", "release", "decision", "issue", "contributor_guidance") || strings.TrimSpace(l.Label) == "" {
				return ErrInvalid
			}
		}
	}
	for _, r := range v.PublicationPolicy.Redirects {
		if r.From == r.To || strings.TrimSpace(r.From) == "" || strings.TrimSpace(r.To) == "" || strings.Contains(r.From, "..") || strings.Contains(r.To, "..") || seen[r.From] || !seen[r.To] {
			return ErrInvalid
		}
	}
	return nil
}
func cleanPath(v string) bool {
	return v != "" && !strings.HasPrefix(v, "/") && !strings.Contains(v, "..") && !strings.ContainsRune(v, '\\')
}
func cleanRef(v string) bool {
	return v != "" && !strings.Contains(v, "..") && !strings.HasPrefix(v, "/")
}
func oneOf(v string, x ...string) bool {
	for _, a := range x {
		if v == a {
			return true
		}
	}
	return false
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func randomID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func writeJSON(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := name + ".tmp"
	if err = os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, name); err != nil {
		return err
	}
	return nil
}
func readJSON(name string, v any) error {
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
