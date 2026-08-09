// Package relationships stores immutable interface publications and consumer declarations.
package relationships

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
	"syscall"
	"time"
)

var (
	ErrNotFound = errors.New("relationship not found")
	ErrInvalid  = errors.New("invalid relationship")
)

type Interface struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	ReleaseID    string    `json:"release_id"`
	CommitID     string    `json:"commit_id"`
	PublishedBy  string    `json:"published_by"`
	PublishedAt  time.Time `json:"published_at"`
}

type Dependency struct {
	ID                   string    `json:"id"`
	RepositoryID         string    `json:"repository_id"`
	CommitID             string    `json:"commit_id"`
	ReleaseID            string    `json:"release_id,omitempty"`
	EnvironmentID        string    `json:"environment_id,omitempty"`
	ProviderRepositoryID string    `json:"provider_repository_id"`
	InterfaceName        string    `json:"interface_name"`
	Constraint           string    `json:"constraint"`
	DeclaredBy           string    `json:"declared_by"`
	DeclaredAt           time.Time `json:"declared_at"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}

func (s *Store) CreateInterface(v Interface) (Interface, error) {
	if !validID(v.RepositoryID) || !validID(v.ReleaseID) || !validCommit(v.CommitID) || !validID(v.PublishedBy) || !validName(v.Name) || !validVersion(v.Version) {
		return v, ErrInvalid
	}
	v.Name, v.Version = strings.TrimSpace(v.Name), strings.TrimSpace(v.Version)
	return v, s.create(v.RepositoryID, "interfaces", &v.ID, &v.PublishedAt, v)
}

func (s *Store) CreateDependency(v Dependency) (Dependency, error) {
	if !validID(v.RepositoryID) || !validCommit(v.CommitID) || !optionalID(v.ReleaseID) || !optionalID(v.EnvironmentID) || !validID(v.ProviderRepositoryID) || !validID(v.DeclaredBy) || !validName(v.InterfaceName) || !validConstraint(v.Constraint) {
		return v, ErrInvalid
	}
	v.InterfaceName, v.Constraint = strings.TrimSpace(v.InterfaceName), strings.TrimSpace(v.Constraint)
	return v, s.create(v.RepositoryID, "dependencies", &v.ID, &v.DeclaredAt, v)
}

func (s *Store) create(repo, kind string, id *string, created *time.Time, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		return err
	}
	*id = hex.EncodeToString(b)
	*created = s.now()
	dir := filepath.Join(s.root, repo, kind)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	switch v := value.(type) {
	case Interface:
		v.ID, v.PublishedAt = *id, *created
		value = v
	case Dependency:
		v.ID, v.DeclaredAt = *id, *created
		value = v
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".relationship-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(body)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(dir, *id+".json")); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	closeErr = d.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

func (s *Store) ListInterfaces(repo string) ([]Interface, error) {
	var v []Interface
	err := s.list(repo, "interfaces", &v)
	sort.Slice(v, func(i, j int) bool { return v[i].PublishedAt.Before(v[j].PublishedAt) })
	return v, err
}
func (s *Store) ListDependencies(repo string) ([]Dependency, error) {
	var v []Dependency
	err := s.list(repo, "dependencies", &v)
	sort.Slice(v, func(i, j int) bool { return v[i].DeclaredAt.Before(v[j].DeclaredAt) })
	return v, err
}
func (s *Store) ListRepositoryIDs() ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && validID(e.Name()) {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}
func (s *Store) list(repo, kind string, target any) error {
	if !validID(repo) {
		return ErrNotFound
	}
	entries, err := os.ReadDir(filepath.Join(s.root, repo, kind))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var bodies []json.RawMessage
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(s.root, repo, kind, e.Name()))
		if readErr != nil {
			return readErr
		}
		bodies = append(bodies, body)
	}
	body, _ := json.Marshal(bodies)
	return json.Unmarshal(body, target)
}

func validName(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && len(v) <= 100 && !strings.ContainsAny(v, "\r\n")
}
func validVersion(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) < 2 || len(v) > 100 || v[0] != 'v' {
		return false
	}
	_, ok := parseVersion(v)
	return ok
}
func validConstraint(v string) bool {
	v = strings.TrimSpace(v)
	if v == "*" {
		return true
	}
	for _, p := range strings.Fields(v) {
		op := ""
		for _, prefix := range []string{"<=", ">=", "<", ">", "="} {
			if strings.HasPrefix(p, prefix) {
				op = prefix
				p = strings.TrimPrefix(p, prefix)
				break
			}
		}
		if op == "" || !validVersion(p) {
			return false
		}
	}
	return v != ""
}
func Satisfies(version, constraint string) bool {
	current, ok := parseVersion(version)
	if !ok {
		return false
	}
	if constraint == "*" {
		return true
	}
	for _, p := range strings.Fields(constraint) {
		op := p[:1]
		if strings.HasPrefix(p, ">=") || strings.HasPrefix(p, "<=") {
			op, p = p[:2], p[2:]
		} else {
			p = p[1:]
		}
		wanted, ok := parseVersion(p)
		if !ok {
			return false
		}
		cmp := compare(current, wanted)
		if (op == "=" && cmp != 0) || (op == ">" && cmp <= 0) || (op == ">=" && cmp < 0) || (op == "<" && cmp >= 0) || (op == "<=" && cmp > 0) {
			return false
		}
	}
	return true
}
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		if p == "" {
			return out, false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return out, false
			}
			out[i] = out[i]*10 + int(r-'0')
		}
	}
	return out, true
}
func compare(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil && v == strings.ToLower(v)
}
func optionalID(v string) bool { return v == "" || validID(v) }
func validCommit(v string) bool {
	if len(v) != 40 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil && v == strings.ToLower(v)
}
