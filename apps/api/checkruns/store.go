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
	"syscall"
	"time"
)

var ErrNotFound = errors.New("check run not found")

const ConfigPath = ".vivarium/checks.json"

type Definition struct {
	Name             string            `json:"name"`
	Image            string            `json:"image"`
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
	ID             string     `json:"id"`
	RepositoryID   string     `json:"repository_id"`
	PullRequestID  string     `json:"pull_request_id"`
	CommitID       string     `json:"commit_id"`
	Definition     Definition `json:"definition"`
	State          string     `json:"state"`
	ExitCode       *int       `json:"exit_code,omitempty"`
	Failure        string     `json:"failure,omitempty"`
	CleanupFailure string     `json:"cleanup_failure,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
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
		if !validImage(x.Image) {
			return Config{}, errors.New("invalid check image")
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

func validImage(image string) bool {
	if image == "" || len(image) > 200 || strings.ContainsAny(image, " \t\r\n@") {
		return false
	}
	for _, r := range image {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("./:_-", r)) {
			return false
		}
	}
	return true
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
	var existingCommit bool
	var resumable []Run
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
			existingCommit = true
			if nonterminal(existing.State) {
				resumable = append(resumable, existing)
			}
		}
	}
	if existingCommit {
		return resumable, nil
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

// Nonterminal returns durable work that must be relaunched after interruption.
func (s *Store) Nonterminal() ([]Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var runs []Run
	err := filepath.WalkDir(s.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var run Run
		if json.Unmarshal(body, &run) != nil {
			return errors.New("invalid check run record")
		}
		if nonterminal(run.State) {
			runs = append(runs, run)
		}
		return nil
	})
	return runs, err
}

func nonterminal(state string) bool {
	return state == "queued" || state == "running" || state == "cleanup_pending"
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

// Execute runs a queued check in a disposable exact-commit snapshot inside a
// capability-free, network-disabled container. Only preinstalled images run.
func (s *Store) Execute(run Run, repositoryPath string) {
	lockPath := filepath.Join(s.root, run.RepositoryID, run.PullRequestID, run.ID+".execution.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	containerName := "vivarium-check-" + run.ID
	if run.State == "cleanup_pending" {
		if cleanupErr := removeContainer(containerName); cleanupErr != nil {
			run.CleanupFailure = cleanupErr.Error()
			_ = s.Update(run)
			return
		}
		now := s.now().Truncate(time.Microsecond)
		run.CompletedAt = &now
		run.CleanupFailure = ""
		if run.ExitCode != nil && *run.ExitCode == 0 && run.Failure == "" {
			run.State = "succeeded"
		} else {
			run.State = "failed"
		}
		_ = s.Update(run)
		return
	}
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
			// A previous API process may have died while Docker kept the container.
			// The execution lock makes removing that abandoned tree safe before
			// relaunching this exact durable run.
			_ = removeContainer(containerName)
			args := []string{"run", "--name", containerName, "--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=128", "--memory=1g", "--cpus=2", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--mount", "type=bind,src=" + workspace + ",dst=/workspace,readonly", "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=64m", "--tmpfs", "/output:rw,nosuid,nodev,size=256m", "--workdir", "/workspace/" + run.Definition.WorkingDirectory, "--env", "HOME=/tmp", "--env", "VIVARIUM_OUTPUT=/output", "--env", "CI=true", "--env", "VIVARIUM_COMMIT_SHA=" + run.CommitID}
			for k, v := range run.Definition.Environment {
				args = append(args, "--env", k+"="+v)
			}
			args = append(args, run.Definition.Image, "sh", "-c", run.Definition.Command)
			cmd := exec.CommandContext(ctx, "docker", args...)
			err = cmd.Run()
			// CommandContext may terminate only the Docker client. Force-removing the
			// named container synchronously kills and reaps every check descendant.
			cleanupErr := removeContainer(containerName)
			cancel()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				err = errors.New("check timed out")
			}
			if cleanupErr != nil {
				run.State = "cleanup_pending"
				run.CleanupFailure = cleanupErr.Error()
				if ee := new(exec.ExitError); errors.As(err, &ee) {
					exit = ee.ExitCode()
				}
				run.ExitCode = &exit
				if err != nil {
					run.Failure = err.Error()
				}
				_ = s.Update(run)
				return
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

func removeContainer(name string) error {
	removeErr := exec.Command("docker", "rm", "--force", name).Run()
	if removeErr == nil {
		return nil
	}
	output, listErr := exec.Command("docker", "ps", "--all", "--quiet", "--filter", "name=^/"+name+"$").Output()
	if listErr == nil && len(strings.TrimSpace(string(output))) == 0 {
		return nil
	}
	return removeErr
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
