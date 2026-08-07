// Package checkruns persists and executes repository-defined verification for exact commits.
package checkruns

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("check run not found")

const ConfigPath = ".vivarium/checks.json"

type Definition struct {
	Name             string            `json:"name"`
	Command          string            `json:"command"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
	Environment      map[string]string `json:"environment,omitempty"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty"`
}

type Config struct {
	Version int          `json:"version"`
	Checks  []Definition `json:"checks"`
}

type Run struct {
	ID            string     `json:"id"`
	RepositoryID  string     `json:"repository_id"`
	PullRequestID string     `json:"pull_request_id"`
	CommitID      string     `json:"commit_id"`
	Definition    Definition `json:"definition"`
	State         string     `json:"state"`
	ExitCode      *int       `json:"exit_code,omitempty"`
	Failure       string     `json:"failure,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("check run storage root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	return &Store{root: abs, now: func() time.Time { return time.Now().UTC() }}, nil
}

func ParseConfig(data []byte) (Config, error) {
	var c Config
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil || c.Version != 1 || len(c.Checks) == 0 || len(c.Checks) > 20 {
		return Config{}, errors.New("invalid check configuration")
	}
	seen := map[string]bool{}
	for i := range c.Checks {
		x := &c.Checks[i]
		if strings.TrimSpace(x.Name) == "" || len(x.Name) > 100 || strings.TrimSpace(x.Command) == "" || len(x.Command) > 4000 || seen[x.Name] || x.TimeoutSeconds < 0 || x.TimeoutSeconds > 3600 {
			return Config{}, errors.New("invalid check definition")
		}
		if x.WorkingDirectory == "" {
			x.WorkingDirectory = "."
		}
		clean := filepath.Clean(x.WorkingDirectory)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return Config{}, errors.New("invalid check working directory")
		}
		x.WorkingDirectory = clean
		if x.TimeoutSeconds == 0 {
			x.TimeoutSeconds = 600
		}
		for k, v := range x.Environment {
			if k == "" || strings.Contains(k, "=") || k == "PATH" || k == "HOME" || strings.HasPrefix(k, "GIT_") || len(k)+len(v) > 4096 {
				return Config{}, errors.New("invalid check environment")
			}
		}
		seen[x.Name] = true
	}
	return c, nil
}

func (s *Store) Create(repositoryID, pullRequestID, commitID string, definitions []Definition) ([]Run, error) {
	now := s.now().Truncate(time.Microsecond)
	runs := make([]Run, 0, len(definitions))
	for _, definition := range definitions {
		id, err := newID()
		if err != nil {
			return nil, err
		}
		runs = append(runs, Run{ID: id, RepositoryID: repositoryID, PullRequestID: pullRequestID, CommitID: commitID, Definition: definition, State: "queued", CreatedAt: now})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.root, repositoryID, pullRequestID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		var existing Run
		if json.Unmarshal(body, &existing) != nil {
			return nil, errors.New("invalid check run record")
		}
		if existing.CommitID == commitID {
			return []Run{}, nil
		}
	}
	for _, run := range runs {
		if err := s.write(dir, run); err != nil {
			return nil, err
		}
	}
	return runs, nil
}

func (s *Store) List(repositoryID, pullRequestID string) ([]Run, error) {
	dir := filepath.Join(s.root, repositoryID, pullRequestID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Run{}, nil
	}
	if err != nil {
		return nil, err
	}
	runs := make([]Run, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var r Run
		if json.Unmarshal(b, &r) != nil {
			return nil, errors.New("invalid check run record")
		}
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].CreatedAt.Equal(runs[j].CreatedAt) {
			return runs[i].ID < runs[j].ID
		}
		return runs[i].CreatedAt.Before(runs[j].CreatedAt)
	})
	return runs, nil
}

func (s *Store) Update(run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(filepath.Join(s.root, run.RepositoryID, run.PullRequestID), run)
}

func (s *Store) write(dir string, run Run) error {
	b, err := json.Marshal(run)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".check-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(b)
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
	if err := os.Rename(name, filepath.Join(dir, run.ID+".json")); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = directory.Sync()
	closeErr = directory.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Execute runs a queued check in a disposable exact-commit snapshot. The child
// receives only a minimal environment and never a repository credential.
func (s *Store) Execute(run Run, repositoryPath string) {
	now := s.now().Truncate(time.Microsecond)
	run.State = "running"
	run.StartedAt = &now
	if s.Update(run) != nil {
		return
	}
	workspace, err := os.MkdirTemp("", "vivarium-check-*")
	if err == nil {
		defer os.RemoveAll(workspace)
		archive := exec.Command("git", "--git-dir="+repositoryPath, "archive", run.CommitID)
		extract := exec.Command("tar", "-x", "-C", workspace)
		pipe, p := extract.StdinPipe()
		if p == nil {
			if p = extract.Start(); p == nil {
				archive.Stdout = pipe
				p = archive.Run()
				_ = pipe.Close()
				if p == nil {
					p = extract.Wait()
				}
			}
		}
		err = p
	}
	exit := 0
	if err == nil {
		dir := filepath.Join(workspace, run.Definition.WorkingDirectory)
		info, e := os.Stat(dir)
		if e != nil || !info.IsDir() {
			err = errors.New("working directory does not exist")
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(run.Definition.TimeoutSeconds)*time.Second)
			cmd := exec.CommandContext(ctx, "sh", "-c", run.Definition.Command)
			cmd.Dir = dir
			cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + workspace, "CI=true", "VIVARIUM_COMMIT_SHA=" + run.CommitID}
			for k, v := range run.Definition.Environment {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
			err = cmd.Run()
			cancel()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				err = errors.New("check timed out")
			}
			if ee := new(exec.ExitError); errors.As(err, &ee) {
				exit = ee.ExitCode()
			}
		}
	}
	done := s.now().Truncate(time.Microsecond)
	run.CompletedAt = &done
	run.ExitCode = &exit
	if err != nil {
		run.State = "failed"
		run.Failure = err.Error()
	} else {
		run.State = "succeeded"
	}
	_ = s.Update(run)
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
