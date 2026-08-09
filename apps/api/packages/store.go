// Package packages persists immutable, checksummed package versions.
package packages

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotFound            = errors.New("package version not found")
	ErrInvalid             = errors.New("invalid package version")
	ErrVersionExists       = errors.New("package version already exists")
	ErrIdentityConflict    = errors.New("package identity belongs to another repository")
	ErrChecksum            = errors.New("package artifact checksum mismatch")
	ErrAlreadyPublished    = errors.New("matching package version is already published")
	ErrDurabilityUncertain = errors.New("package version is visible but durability is uncertain")
)

var identityPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,98}[a-z0-9])?$`)
var versionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,99}$`)

type Platform struct {
	OS           string `json:"os,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	Runtime      string `json:"runtime,omitempty"`
}

type Dependency struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint"`
}

type BuildAttestation struct {
	Step    string `json:"step"`
	Image   string `json:"image"`
	Command string `json:"command"`
	Attempt int    `json:"attempt"`
	State   string `json:"state"`
}

type Version struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Version          string           `json:"version"`
	RepositoryID     string           `json:"repository_id"`
	ReleaseID        string           `json:"release_id"`
	SourceCommit     string           `json:"source_commit"`
	BuildID          string           `json:"build_id"`
	BuildAttestation BuildAttestation `json:"build_attestation"`
	ArtifactID       string           `json:"artifact_id"`
	ArtifactPath     string           `json:"artifact_path"`
	ContentType      string           `json:"content_type"`
	Size             int64            `json:"size"`
	SHA256           string           `json:"sha256"`
	Platform         Platform         `json:"platform"`
	Dependencies     []Dependency     `json:"dependencies"`
	Summary          string           `json:"summary,omitempty"`
	Documentation    string           `json:"documentation,omitempty"`
	License          string           `json:"license,omitempty"`
	Support          string           `json:"support,omitempty"`
	PublisherID      string           `json:"publisher_id"`
	Visibility       string           `json:"visibility"`
	Lifecycle        string           `json:"lifecycle"`
	LifecycleWarning string           `json:"lifecycle_warning,omitempty"`
	PublishedAt      time.Time        `json:"published_at"`
}

type Store struct {
	root           string
	mu             sync.Mutex
	now            func() time.Time
	openDirectory  func(string) (*os.File, error)
	syncDirectory  func(*os.File) error
	closeDirectory func(*os.File) error
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("package storage root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	return &Store{root: abs, now: time.Now, openDirectory: os.Open, syncDirectory: func(directory *os.File) error { return directory.Sync() }, closeDirectory: func(directory *os.File) error { return directory.Close() }}, nil
}

// Publish copies and verifies the complete artifact before atomically exposing
// the version directory. Readers can therefore observe all metadata and bytes,
// or no package version at all.
func (s *Store) Publish(version Version, artifact io.Reader) (Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Version{}, err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Version{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	version.Name = strings.ToLower(strings.TrimSpace(version.Name))
	version.Version = strings.TrimSpace(version.Version)
	version.Visibility = strings.ToLower(strings.TrimSpace(version.Visibility))
	if !valid(version) {
		return Version{}, ErrInvalid
	}
	identityDir := filepath.Join(s.root, version.Name)
	if entries, readErr := os.ReadDir(identityDir); readErr == nil {
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			existing, getErr := s.Get(version.Name, entry.Name())
			if getErr != nil {
				return Version{}, getErr
			}
			if existing.RepositoryID != version.RepositoryID {
				return Version{}, ErrIdentityConflict
			}
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return Version{}, readErr
	}
	final := filepath.Join(identityDir, version.Version)
	if _, err = os.Stat(final); err == nil {
		existing, getErr := s.Get(version.Name, version.Version)
		if getErr != nil {
			return Version{}, getErr
		}
		if matchingPublication(existing, version) {
			return existing, ErrAlreadyPublished
		}
		return Version{}, ErrVersionExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return Version{}, err
	}
	if err = os.MkdirAll(identityDir, 0700); err != nil {
		return Version{}, err
	}
	tmp, err := os.MkdirTemp(identityDir, ".publishing-")
	if err != nil {
		return Version{}, err
	}
	defer os.RemoveAll(tmp)
	file, err := os.OpenFile(filepath.Join(tmp, "artifact"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Version{}, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(artifact, version.Size+1))
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil {
		return Version{}, copyErr
	}
	if syncErr != nil {
		return Version{}, syncErr
	}
	if closeErr != nil {
		return Version{}, closeErr
	}
	if written != version.Size || hex.EncodeToString(hash.Sum(nil)) != version.SHA256 {
		return Version{}, ErrChecksum
	}
	idBytes := make([]byte, 16)
	if _, err = rand.Read(idBytes); err != nil {
		return Version{}, err
	}
	version.ID = hex.EncodeToString(idBytes)
	version.Lifecycle = "active"
	version.PublishedAt = s.now().UTC().Truncate(time.Microsecond)
	sort.Slice(version.Dependencies, func(i, j int) bool { return version.Dependencies[i].Name < version.Dependencies[j].Name })
	body, err := json.Marshal(version)
	if err != nil {
		return Version{}, err
	}
	if err = atomicFile(filepath.Join(tmp, "metadata.json"), body); err != nil {
		return Version{}, err
	}
	dir, err := os.Open(tmp)
	if err != nil {
		return Version{}, err
	}
	err = dir.Sync()
	closeErr = dir.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return Version{}, err
	}
	if err = os.Rename(tmp, final); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Version{}, ErrVersionExists
		}
		return Version{}, err
	}
	dir, err = s.openDirectory(identityDir)
	if err != nil {
		return version, fmt.Errorf("%w: %w", ErrDurabilityUncertain, err)
	}
	err = s.syncDirectory(dir)
	closeErr = s.closeDirectory(dir)
	if err == nil {
		err = closeErr
	}
	if err != nil {
		// Rename has made a complete version visible, but its parent-directory
		// entry was not durably acknowledged. Report uncertainty to the caller.
		return version, fmt.Errorf("%w: %w", ErrDurabilityUncertain, err)
	}
	return version, nil
}

func matchingPublication(existing, requested Version) bool {
	requested.ID = existing.ID
	requested.Lifecycle = existing.Lifecycle
	requested.LifecycleWarning = existing.LifecycleWarning
	requested.PublishedAt = existing.PublishedAt
	sort.Slice(requested.Dependencies, func(i, j int) bool { return requested.Dependencies[i].Name < requested.Dependencies[j].Name })
	return reflect.DeepEqual(existing, requested)
}

func (s *Store) Get(name, version string) (Version, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	version = strings.TrimSpace(version)
	if !identityPattern.MatchString(name) || !versionPattern.MatchString(version) {
		return Version{}, ErrNotFound
	}
	body, err := os.ReadFile(filepath.Join(s.root, name, version, "metadata.json"))
	if errors.Is(err, os.ErrNotExist) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, err
	}
	var result Version
	if json.Unmarshal(body, &result) != nil || result.Name != name || result.Version != version {
		return Version{}, ErrNotFound
	}
	return result, nil
}

func (s *Store) OpenArtifact(name, version string) (*os.File, Version, error) {
	item, err := s.Get(name, version)
	if err != nil {
		return nil, Version{}, err
	}
	file, err := os.Open(filepath.Join(s.root, item.Name, item.Version, "artifact"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, Version{}, errors.New("package artifact missing")
	}
	return file, item, err
}

func (s *Store) ListRepository(repositoryID string) ([]Version, error) {
	items, err := s.List()
	if err != nil {
		return nil, err
	}
	result := []Version{}
	for _, item := range items {
		if item.RepositoryID == repositoryID {
			result = append(result, item)
		}
	}
	return result, nil
}

// List returns every package version; callers remain responsible for applying
// current repository visibility before projecting the catalog.
func (s *Store) List() ([]Version, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	result := []Version{}
	for _, identity := range entries {
		if !identity.IsDir() || strings.HasPrefix(identity.Name(), ".") || identity.Name() == "inventories" || identity.Name() == "update-policies" || identity.Name() == "updates" {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(s.root, identity.Name()))
		if err != nil {
			return nil, err
		}
		for _, entry := range versions {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			item, err := s.Get(identity.Name(), entry.Name())
			if err != nil {
				return nil, err
			}
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PublishedAt.Before(result[j].PublishedAt) })
	return result, nil
}

func (s *Store) SetLifecycle(name, version, lifecycle, warning string) (Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Version{}, err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Version{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	lifecycle, warning = strings.TrimSpace(strings.ToLower(lifecycle)), strings.TrimSpace(warning)
	if lifecycle != "active" && lifecycle != "deprecated" && lifecycle != "yanked" {
		return Version{}, ErrInvalid
	}
	if (lifecycle == "active" && warning != "") || (lifecycle != "active" && (warning == "" || len(warning) > 1000)) {
		return Version{}, ErrInvalid
	}
	item, err := s.Get(name, version)
	if err != nil {
		return Version{}, err
	}
	item.Lifecycle, item.LifecycleWarning = lifecycle, warning
	body, _ := json.Marshal(item)
	if err = atomicFile(filepath.Join(s.root, item.Name, item.Version, "metadata.json"), body); err != nil {
		return Version{}, err
	}
	return item, nil
}

func valid(v Version) bool {
	if !identityPattern.MatchString(v.Name) || !versionPattern.MatchString(v.Version) || len(v.RepositoryID) != 32 || len(v.ReleaseID) != 32 || len(v.BuildID) != 32 || len(v.ArtifactID) != 32 || len(v.PublisherID) != 32 || len(v.SourceCommit) != 40 || len(v.SHA256) != 64 || v.Size < 0 || (v.Visibility != "public" && v.Visibility != "private") {
		return false
	}
	if strings.TrimSpace(v.BuildAttestation.Step) == "" || strings.TrimSpace(v.BuildAttestation.Image) == "" || strings.TrimSpace(v.BuildAttestation.Command) == "" || v.BuildAttestation.Attempt < 1 || v.BuildAttestation.State != "succeeded" {
		return false
	}
	if len(v.Platform.OS) > 50 || len(v.Platform.Architecture) > 50 || len(v.Platform.Runtime) > 100 || len(v.Dependencies) > 100 || len(v.Summary) > 500 || len(v.Documentation) > 20000 || len(v.License) > 100 || len(v.Support) > 500 {
		return false
	}
	seen := map[string]bool{}
	for _, d := range v.Dependencies {
		name := strings.ToLower(strings.TrimSpace(d.Name))
		if !identityPattern.MatchString(name) || d.Name != name || strings.TrimSpace(d.Constraint) == "" || len(d.Constraint) > 200 || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

func atomicFile(name string, body []byte) error {
	dir := filepath.Dir(name)
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
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
	return os.Rename(tmpName, name)
}
