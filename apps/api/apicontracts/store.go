// Package apicontracts persists immutable, versioned service interface contracts.
package apicontracts

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

var ErrNotFound = errors.New("api contract not found")
var ErrInvalid = errors.New("invalid api contract")
var ErrConflict = errors.New("api contract version conflict")

type Source struct {
	CommitID          string `json:"commit_id"`
	PullRequestID     string `json:"pull_request_id"`
	ReleaseID         string `json:"release_id,omitempty"`
	DefinitionPath    string `json:"definition_path"`
	DocumentationPath string `json:"documentation_path"`
}
type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required"`
	SchemaID    string `json:"schema_id"`
	Description string `json:"description"`
}
type Operation struct {
	ID                string      `json:"id"`
	Method            string      `json:"method"`
	Path              string      `json:"path"`
	Summary           string      `json:"summary"`
	Authentication    []string    `json:"authentication"`
	Parameters        []Parameter `json:"parameters"`
	RequestSchemaID   string      `json:"request_schema_id,omitempty"`
	ResponseSchemaIDs []string    `json:"response_schema_ids"`
	ErrorIDs          []string    `json:"error_ids"`
	Stability         string      `json:"stability"`
	OwnerIDs          []string    `json:"owner_ids"`
}
type Schema struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Definition     string   `json:"definition"`
	RequiredFields []string `json:"required_fields"`
	Description    string   `json:"description"`
}
type APIError struct {
	ID         string `json:"id"`
	Code       string `json:"code"`
	HTTPStatus int    `json:"http_status"`
	Meaning    string `json:"meaning"`
	Recovery   string `json:"recovery"`
	SchemaID   string `json:"schema_id,omitempty"`
}
type Authentication struct {
	ID          string   `json:"id"`
	Mode        string   `json:"mode"`
	Description string   `json:"description"`
	Scopes      []string `json:"scopes"`
}
type Environment struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	BaseURL      string   `json:"base_url"`
	Availability string   `json:"availability"`
	Regions      []string `json:"regions"`
}
type Limits struct {
	Requests      int `json:"requests"`
	WindowSeconds int `json:"window_seconds"`
	Burst         int `json:"burst"`
	PayloadBytes  int `json:"payload_bytes"`
	Concurrency   int `json:"concurrency"`
}
type SupportPolicy struct {
	Channels              []string `json:"channels"`
	ResponseTarget        string   `json:"response_target"`
	DeprecationNoticeDays int      `json:"deprecation_notice_days"`
	SunsetNoticeDays      int      `json:"sunset_notice_days"`
}
type Link struct {
	Kind  string `json:"kind"`
	ID    string `json:"id,omitempty"`
	URL   string `json:"url,omitempty"`
	Label string `json:"label"`
}
type Compatibility struct {
	FromVersion     string   `json:"from_version"`
	Level           string   `json:"level"`
	Promise         string   `json:"promise"`
	BreakingChanges []string `json:"breaking_changes"`
}
type Revision struct {
	Version        int              `json:"version"`
	VersionLabel   string           `json:"version_label"`
	Title          string           `json:"title"`
	Summary        string           `json:"summary"`
	Source         Source           `json:"source"`
	Operations     []Operation      `json:"operations"`
	Schemas        []Schema         `json:"schemas"`
	Errors         []APIError       `json:"errors"`
	Authentication []Authentication `json:"authentication"`
	Environments   []Environment    `json:"environments"`
	Limits         Limits           `json:"limits"`
	OwnerIDs       []string         `json:"owner_ids"`
	Stability      string           `json:"stability"`
	SupportPolicy  SupportPolicy    `json:"support_policy"`
	Links          []Link           `json:"links"`
	Compatibility  Compatibility    `json:"compatibility"`
	KnownGaps      []string         `json:"known_gaps"`
	Rationale      string           `json:"rationale"`
	CreatedBy      string           `json:"created_by"`
	CreatedAt      time.Time        `json:"created_at"`
}
type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}
type Contract struct {
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
func (s *Store) Create(repo, actor string, r Revision) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		if validate(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Contract{ID: randomID(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return project(out), err
}
func (s *Store) Revise(id string, expected int, actor string, r Revision) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validate(r) != nil {
			return ErrInvalid
		}
		stamp(&r, expected+1, actor, s.now())
		v.CurrentVersion = r.Version
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		return s.write(v)
	})
	return project(out), err
}
func (s *Store) Get(id string) (Contract, error) {
	var out Contract
	err := s.lock(func() error { var e error; out, e = s.read(id); return e })
	return project(out), err
}
func (s *Store) List(repo string) ([]Contract, error) {
	out := []Contract{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.repoDir(repo))
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		for _, x := range entries {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			v, e := s.readFile(filepath.Join(s.repoDir(repo), x.Name()))
			if e != nil {
				return e
			}
			if v.RepositoryID == repo {
				out = append(out, project(v))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}
func stamp(r *Revision, v int, actor string, now time.Time) {
	r.Version = v
	r.CreatedBy = actor
	r.CreatedAt = now
}
func validate(r Revision) error {
	if r.VersionLabel == "" || r.Title == "" || r.Summary == "" || r.Source.CommitID == "" || r.Source.PullRequestID == "" || r.Source.DefinitionPath == "" || r.Source.DocumentationPath == "" || len(r.Operations) == 0 || len(r.Schemas) == 0 || len(r.Errors) == 0 || len(r.Authentication) == 0 || len(r.Environments) == 0 || len(r.OwnerIDs) == 0 || r.Rationale == "" {
		return ErrInvalid
	}
	if !map[string]bool{"experimental": true, "beta": true, "stable": true, "deprecated": true}[r.Stability] || r.Limits.Requests <= 0 || r.Limits.WindowSeconds <= 0 || r.Limits.PayloadBytes <= 0 || r.SupportPolicy.DeprecationNoticeDays < 0 || r.SupportPolicy.SunsetNoticeDays < 0 || len(r.SupportPolicy.Channels) == 0 {
		return ErrInvalid
	}
	ids := map[string]bool{}
	for _, x := range r.Schemas {
		if x.ID == "" || x.Name == "" || x.Definition == "" || ids[x.ID] {
			return ErrInvalid
		}
		ids[x.ID] = true
	}
	auth := map[string]bool{}
	for _, x := range r.Authentication {
		if x.ID == "" || x.Mode == "" || x.Description == "" || auth[x.ID] {
			return ErrInvalid
		}
		auth[x.ID] = true
	}
	errs := map[string]bool{}
	for _, x := range r.Errors {
		if x.ID == "" || x.Code == "" || x.HTTPStatus < 400 || x.HTTPStatus > 599 || x.Meaning == "" || x.Recovery == "" || errs[x.ID] || (x.SchemaID != "" && !ids[x.SchemaID]) {
			return ErrInvalid
		}
		errs[x.ID] = true
	}
	ops := map[string]bool{}
	for _, x := range r.Operations {
		if x.ID == "" || x.Path == "" || x.Summary == "" || len(x.OwnerIDs) == 0 || ops[x.ID] || !map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true, "OPTIONS": true}[x.Method] || !map[string]bool{"experimental": true, "beta": true, "stable": true, "deprecated": true}[x.Stability] {
			return ErrInvalid
		}
		for _, owner := range x.OwnerIDs {
			if strings.TrimSpace(owner) == "" {
				return ErrInvalid
			}
		}
		ops[x.ID] = true
		for _, a := range x.Authentication {
			if !auth[a] {
				return ErrInvalid
			}
		}
		for _, id := range append(append([]string{x.RequestSchemaID}, x.ResponseSchemaIDs...), schemaIDs(x.Parameters)...) {
			if id != "" && !ids[id] {
				return ErrInvalid
			}
		}
		for _, id := range x.ErrorIDs {
			if !errs[id] {
				return ErrInvalid
			}
		}
	}
	for _, x := range r.Environments {
		if x.ID == "" || x.Name == "" || x.BaseURL == "" || !map[string]bool{"available": true, "limited": true, "unavailable": true, "planned": true}[x.Availability] {
			return ErrInvalid
		}
	}
	for _, x := range r.Links {
		if !map[string]bool{"source": true, "release": true, "documentation": true, "data_use": true, "support": true}[x.Kind] || x.Label == "" || (x.ID == "" && x.URL == "") {
			return ErrInvalid
		}
	}
	if !map[string]bool{"compatible": true, "conditionally_compatible": true, "breaking": true, "initial": true}[r.Compatibility.Level] || r.Compatibility.Promise == "" {
		return ErrInvalid
	}
	return nil
}
func schemaIDs(v []Parameter) []string {
	out := []string{}
	for _, x := range v {
		out = append(out, x.SchemaID)
	}
	return out
}
func project(v Contract) Contract {
	v.Diagnostics = []Diagnostic{}
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	if r.Source.ReleaseID == "" {
		v.Diagnostics = append(v.Diagnostics, Diagnostic{"unreleased_implementation", "warning", "The reviewed implementation is not tied to a release."})
	}
	if len(r.KnownGaps) > 0 {
		v.Diagnostics = append(v.Diagnostics, Diagnostic{"known_gaps", "warning", "Known contract gaps remain explicit in this version."})
	}
	for _, e := range r.Environments {
		if e.Availability != "available" {
			v.Diagnostics = append(v.Diagnostics, Diagnostic{"environment_" + e.Availability, "warning", e.Name + " is " + e.Availability + "."})
		}
	}
	return v
}
func (s *Store) repoDir(repo string) string {
	return filepath.Join(s.root, "repo-"+base64.RawURLEncoding.EncodeToString([]byte(repo)))
}
func (s *Store) read(id string) (Contract, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return Contract{}, e
	}
	for _, x := range entries {
		if x.IsDir() {
			v, e := s.readFile(filepath.Join(s.root, x.Name(), id+".json"))
			if !errors.Is(e, ErrNotFound) {
				return v, e
			}
		}
	}
	return Contract{}, ErrNotFound
}
func (s *Store) readFile(name string) (Contract, error) {
	var v Contract
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
func (s *Store) write(v Contract) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	dir := s.repoDir(v.RepositoryID)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".api-contract-")
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
