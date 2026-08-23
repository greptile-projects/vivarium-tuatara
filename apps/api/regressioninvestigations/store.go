// Package regressioninvestigations persists the agreed boundary for searching a suspected regression.
package regressioninvestigations

import (
	"crypto/rand"
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

var ErrNotFound = errors.New("regression investigation not found")
var ErrInvalid = errors.New("invalid regression investigation")
var ErrConflict = errors.New("regression investigation changed")

type Reference struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision,omitempty"`
	Label      string `json:"label"`
}
type Boundary struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Revision   string `json:"revision"`
	Label      string `json:"label"`
}
type Evidence struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision,omitempty"`
	Label      string `json:"label"`
	Visibility string `json:"visibility"`
	Available  bool   `json:"available"`
	Stale      bool   `json:"stale"`
	Diagnostic string `json:"diagnostic,omitempty"`
}
type Entry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Message   string    `json:"message"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Investigation struct {
	ID                 string     `json:"id"`
	RequestID          string     `json:"request_id"`
	RequestDigest      string     `json:"request_digest"`
	RepositoryID       string     `json:"repository_id"`
	Version            int        `json:"version"`
	Title              string     `json:"title"`
	Source             Reference  `json:"source"`
	ExpectedBehavior   string     `json:"expected_behavior"`
	RegressedBehavior  string     `json:"regressed_behavior"`
	KnownGood          Boundary   `json:"known_good"`
	KnownBad           Boundary   `json:"known_bad"`
	Environments       []string   `json:"affected_environments"`
	Severity           string     `json:"severity"`
	OwnerIDs           []string   `json:"owner_ids"`
	AcceptanceCriteria []string   `json:"acceptance_criteria"`
	Evidence           []Evidence `json:"evidence"`
	Diagnostics        []string   `json:"diagnostics"`
	Comparable         bool       `json:"comparable"`
	Status             string     `json:"status"`
	History            []Entry    `json:"history"`
	CreatedBy          string     `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	syncDirectory func(*os.File) error
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }, syncDirectory: func(d *os.File) error { return d.Sync() }}, nil
}
func (s *Store) Create(v Investigation, actor string) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	if !valid(v, actor) {
		return Investigation{}, ErrInvalid
	}
	digest, e := requestDigest(v, actor)
	if e != nil {
		return Investigation{}, e
	}
	existing, e := s.list(v.RepositoryID)
	if e != nil {
		return Investigation{}, e
	}
	for _, item := range existing {
		if item.RequestID != v.RequestID {
			continue
		}
		if item.RequestDigest != digest {
			return Investigation{}, ErrConflict
		}
		return item, nil
	}
	now := s.now()
	v.ID = id()
	v.RequestDigest = digest
	v.RepositoryID = strings.TrimSpace(v.RepositoryID)
	v.Version = 1
	v.Status = "open"
	v.CreatedBy = actor
	v.CreatedAt = now
	v.UpdatedAt = now
	v.OwnerIDs = uniq(v.OwnerIDs)
	v.Environments = uniq(v.Environments)
	v.AcceptanceCriteria = uniq(v.AcceptanceCriteria)
	for i := range v.Evidence {
		v.Evidence[i].ID = id()
	}
	v.History = []Entry{{ID: id(), Kind: "opened", ActorID: actor, To: "open", Message: "Search boundary agreed", CreatedAt: now}}
	return v, s.write(v)
}
func (s *Store) Get(repo, wid string) (Investigation, error) {
	if !token(repo) || !token(wid) {
		return Investigation{}, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, repo, wid+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return Investigation{}, ErrNotFound
	}
	var v Investigation
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *Store) List(repo string) ([]Investigation, error) {
	if !token(repo) {
		return nil, ErrInvalid
	}
	return s.list(repo)
}
func (s *Store) list(repo string) ([]Investigation, error) {
	files, e := os.ReadDir(filepath.Join(s.root, repo))
	if errors.Is(e, os.ErrNotExist) {
		return []Investigation{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Investigation{}
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		v, x := s.Get(repo, strings.TrimSuffix(f.Name(), ".json"))
		if x != nil {
			return nil, x
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Append(repo, wid, actor, kind, message, value string, expected int) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	v, e := s.Get(repo, wid)
	if e != nil {
		return Investigation{}, e
	}
	if v.Version != expected {
		return Investigation{}, ErrConflict
	}
	message = strings.TrimSpace(message)
	if len(message) < 1 || len(message) > 4000 || sensitive(message) {
		return Investigation{}, ErrInvalid
	}
	now := s.now()
	entry := Entry{ID: id(), Kind: kind, ActorID: actor, Message: message, CreatedAt: now}
	switch kind {
	case "discussion", "hypothesis":
	case "scope_change":
		entry.From = strings.Join(v.Environments, ", ")
		values := split(value)
		if len(values) == 0 {
			return Investigation{}, ErrInvalid
		}
		v.Environments = values
		entry.To = strings.Join(values, ", ")
	case "status_change":
		if value != "open" && value != "bounded" && value != "paused" && value != "closed" {
			return Investigation{}, ErrInvalid
		}
		entry.From = v.Status
		entry.To = value
		v.Status = value
	default:
		return Investigation{}, ErrInvalid
	}
	v.History = append(v.History, entry)
	v.Version++
	v.UpdatedAt = now
	return v, s.write(v)
}
func valid(v Investigation, actor string) bool {
	if !(token(v.RepositoryID) && token(v.RequestID) && token(actor) && strings.TrimSpace(v.Title) != "" && len(v.Title) <= 200 && text(v.ExpectedBehavior) && text(v.RegressedBehavior) && boundary(v.KnownGood) && boundary(v.KnownBad) && len(v.Environments) > 0 && len(v.OwnerIDs) > 0 && len(v.AcceptanceCriteria) > 0 && (v.Severity == "low" || v.Severity == "medium" || v.Severity == "high" || v.Severity == "critical") && v.Source.ResourceID != "") {
		return false
	}
	for _, evidence := range v.Evidence {
		if strings.TrimSpace(evidence.Kind) == "" || strings.TrimSpace(evidence.ResourceID) == "" || !text(evidence.Label) {
			return false
		}
	}
	return true
}
func requestDigest(v Investigation, actor string) (string, error) {
	v.ID, v.RequestDigest, v.CreatedBy, v.Status = "", "", "", ""
	v.Version, v.CreatedAt, v.UpdatedAt, v.History = 0, time.Time{}, time.Time{}, nil
	v.Diagnostics, v.Comparable = nil, false
	for i := range v.Evidence {
		v.Evidence[i].ID = ""
		v.Evidence[i].Available, v.Evidence[i].Stale, v.Evidence[i].Diagnostic = false, false, ""
	}
	b, e := json.Marshal(struct {
		Actor         string        `json:"actor"`
		Investigation Investigation `json:"investigation"`
	}{actor, v})
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func boundary(v Boundary) bool {
	return (v.Kind == "commit" || v.Kind == "release") && len(v.Revision) == 40 && v.Label != ""
}
func text(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && len(v) <= 10000 && !sensitive(v)
}
func sensitive(v string) bool {
	l := strings.ToLower(v)
	return strings.Contains(l, "-----begin") || strings.Contains(l, "api_key=") || strings.Contains(l, "password=") || strings.Contains(l, "token=")
}
func token(v string) bool {
	if len(v) < 1 || len(v) > 200 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func uniq(v []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range v {
		x = strings.TrimSpace(x)
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
func split(v string) []string { return uniq(strings.Split(v, ",")) }
func id() string              { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) write(v Investigation) error {
	dir := filepath.Join(s.root, v.RepositoryID)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".investigation-")
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
		e = os.Rename(n, filepath.Join(dir, v.ID+".json"))
	}
	if e != nil {
		return e
	}
	d, e := os.Open(dir)
	if e != nil {
		return e
	}
	e = s.syncDirectory(d)
	ce = d.Close()
	if e == nil {
		e = ce
	}
	return e
}
func (s *Store) lock() (func(), error) {
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		f.Close()
		return nil, e
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
