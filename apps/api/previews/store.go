// Package previews retains exact-revision pull request preview publications.
package previews

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
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
)

const ConfigPath = ".vivarium/preview.json"

var ErrNotFound = errors.New("preview not found")

type Resources struct {
	CPUs           float64 `json:"cpus"`
	MemoryMB       int     `json:"memory_mb"`
	StorageMB      int     `json:"storage_mb"`
	TimeoutSeconds int     `json:"timeout_seconds"`
}
type Config struct {
	Version          int               `json:"version"`
	Image            string            `json:"image"`
	Build            string            `json:"build"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
	OutputPath       string            `json:"output_path"`
	Environment      map[string]string `json:"environment,omitempty"`
	Resources        Resources         `json:"resources"`
}
type Preview struct {
	ID               string    `json:"id"`
	RepositoryID     string    `json:"repository_id"`
	PullRequestID    string    `json:"pull_request_id"`
	Revision         string    `json:"revision"`
	CreatorID        string    `json:"creator_id"`
	Definition       Config    `json:"definition"`
	DefinitionSHA256 string    `json:"definition_sha256"`
	BuildRunID       string    `json:"build_run_id"`
	State            string    `json:"state"`
	Stale            bool      `json:"stale"`
	URL              string    `json:"url"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func ParseConfig(data []byte) (Config, string, error) {
	var c Config
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || c.Version != 1 || strings.TrimSpace(c.Build) == "" || len(c.Build) > 4000 || c.Resources.CPUs <= 0 || c.Resources.CPUs > 2 || c.Resources.MemoryMB < 64 || c.Resources.MemoryMB > 2048 || c.Resources.StorageMB < 16 || c.Resources.StorageMB > 1024 || c.Resources.TimeoutSeconds < 1 || c.Resources.TimeoutSeconds > 1800 {
		return c, "", errors.New("invalid preview definition")
	}
	if c.WorkingDirectory == "" {
		c.WorkingDirectory = "."
	}
	clean := filepath.Clean(c.WorkingDirectory)
	output := filepath.Clean(c.OutputPath)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(output) || output == "." || output == ".." || strings.HasPrefix(output, ".."+string(filepath.Separator)) {
		return c, "", errors.New("invalid preview path")
	}
	c.WorkingDirectory, c.OutputPath = clean, output
	// Reuse check validation for image and scoped environment rules.
	b, _ := json.Marshal(checkruns.Config{Version: 1, Checks: []checkruns.Definition{{Name: "preview", Image: c.Image, Command: c.Build, WorkingDirectory: c.WorkingDirectory, Environment: c.Environment, TimeoutSeconds: c.Resources.TimeoutSeconds}}})
	if _, err := checkruns.ParseConfig(b); err != nil {
		return c, "", err
	}
	normalized, _ := json.Marshal(c)
	sum := sha256.Sum256(normalized)
	return c, hex.EncodeToString(sum[:]), nil
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("preview storage root required")
	}
	root, _ = filepath.Abs(root)
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}
func (s *Store) Create(repositoryID, pullID, revision, creator, hash, runID string, c Config) (Preview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	idb := make([]byte, 16)
	_, _ = rand.Read(idb)
	p := Preview{ID: hex.EncodeToString(idb), RepositoryID: repositoryID, PullRequestID: pullID, Revision: revision, CreatorID: creator, Definition: c, DefinitionSHA256: hash, BuildRunID: runID, State: "building", CreatedAt: now, UpdatedAt: now}
	p.URL = "/repositories/" + repositoryID + "/pulls/" + pullID + "/previews/" + p.ID + "/content/"
	if err := s.write(p); err != nil {
		return Preview{}, err
	}
	return p, nil
}
func (s *Store) write(p Preview) error {
	d := filepath.Join(s.root, p.RepositoryID, p.PullRequestID)
	if err := os.MkdirAll(d, 0700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(filepath.Join(d, p.ID+".json"), b, 0600)
}
func (s *Store) Get(repo, pull, id string) (Preview, error) {
	b, e := os.ReadFile(filepath.Join(s.root, repo, pull, filepath.Base(id)+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return Preview{}, ErrNotFound
	}
	var p Preview
	if e != nil || json.Unmarshal(b, &p) != nil {
		return p, ErrNotFound
	}
	return p, nil
}
func (s *Store) List(repo, pull, currentRevision string) ([]Preview, error) {
	entries, e := os.ReadDir(filepath.Join(s.root, repo, pull))
	if errors.Is(e, os.ErrNotExist) {
		return []Preview{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Preview{}
	for _, x := range entries {
		if filepath.Ext(x.Name()) != ".json" {
			continue
		}
		p, e := s.Get(repo, pull, strings.TrimSuffix(x.Name(), ".json"))
		if e == nil {
			p.Stale = p.Revision != currentRevision
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
