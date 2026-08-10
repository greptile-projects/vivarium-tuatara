package workspaces

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ErrCheckpointConflict = errors.New("checkpoint lineage changed")

type Reproducibility struct {
	Dependencies []string `json:"dependencies"`
	Notes        string   `json:"notes,omitempty"`
}

type CheckpointFile struct {
	Path       string `json:"path"`
	Operation  string `json:"operation"`
	Mode       uint32 `json:"mode,omitempty"`
	Size       int64  `json:"size,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Patch      string `json:"patch,omitempty"`
	ContentB64 string `json:"content_base64,omitempty"`
}

type Checkpoint struct {
	ID                 string            `json:"id"`
	WorkspaceID        string            `json:"workspace_id"`
	RepositoryID       string            `json:"repository_id"`
	BaseCommitID       string            `json:"base_commit_id"`
	Definition         Definition        `json:"definition"`
	DefinitionSHA256   string            `json:"definition_sha256"`
	ParentCheckpointID string            `json:"parent_checkpoint_id,omitempty"`
	Title              string            `json:"title"`
	Description        string            `json:"description,omitempty"`
	Reproducibility    Reproducibility   `json:"reproducibility"`
	Files              []CheckpointFile  `json:"files"`
	CreatedBy          string            `json:"created_by"`
	CreatedAt          time.Time         `json:"created_at"`
	Publication        *Publication      `json:"publication,omitempty"`
	ContributorIDs     []string          `json:"contributor_ids"`
	Commands           []CommandEvidence `json:"commands"`
}

type CommandEvidence struct {
	ID       string `json:"id"`
	SHA256   string `json:"sha256"`
	ExitCode int    `json:"exit_code"`
	ActorID  string `json:"actor_id"`
}

type Publication struct {
	Branch         string    `json:"branch"`
	CommitID       string    `json:"commit_id"`
	PullRequestID  string    `json:"pull_request_id,omitempty"`
	TaskID         string    `json:"task_id,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	ContributorIDs []string  `json:"contributor_ids"`
	CommandIDs     []string  `json:"command_ids"`
	LinkPending    bool      `json:"link_pending,omitempty"`
	PublishedBy    string    `json:"published_by"`
	PublishedAt    time.Time `json:"published_at"`
}

type PublicationIntent struct {
	WorkspaceID  string      `json:"workspace_id"`
	CheckpointID string      `json:"checkpoint_id"`
	Publication  Publication `json:"publication"`
}

func (s *Store) publicationIntentPath(workspaceID, id string) string {
	return filepath.Join(s.root, "publication-intents", workspaceID, id+".json")
}
func (s *Store) SavePublicationIntent(intent PublicationIntent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Dir(s.publicationIntentPath(intent.WorkspaceID, intent.CheckpointID))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(intent, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".intent-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, s.publicationIntentPath(intent.WorkspaceID, intent.CheckpointID)); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (s *Store) GetPublicationIntent(workspaceID, id string) (PublicationIntent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.publicationIntentPath(workspaceID, id))
	if errors.Is(err, os.ErrNotExist) {
		return PublicationIntent{}, ErrNotFound
	}
	if err != nil {
		return PublicationIntent{}, err
	}
	var intent PublicationIntent
	if json.Unmarshal(b, &intent) != nil || intent.WorkspaceID != workspaceID || intent.CheckpointID != id {
		return PublicationIntent{}, ErrNotFound
	}
	return intent, nil
}
func (s *Store) ClearPublicationIntent(workspaceID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.publicationIntentPath(workspaceID, id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) ConfirmCheckpointPublicationLink(workspaceID, id, pullID string) (Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.readCheckpoint(workspaceID, id)
	if err != nil {
		return c, err
	}
	if c.Publication == nil || c.Publication.PullRequestID != pullID {
		return c, ErrCheckpointConflict
	}
	if !c.Publication.LinkPending {
		return c.Public(), nil
	}
	c.Publication.LinkPending = false
	if err = s.writeCheckpoint(c); err != nil {
		return c, err
	}
	return c.Public(), nil
}

func (c Checkpoint) Public() Checkpoint {
	for i := range c.Files {
		c.Files[i].ContentB64 = ""
		c.Files[i].Patch = ""
	}
	return c
}

func (s *Store) CreateCheckpoint(workspaceID, actor, expectedParent, title, description string, reproducibility Reproducibility, files []CheckpointFile) (Checkpoint, error) {
	control := s.controlLock(workspaceID)
	control.Lock()
	defer control.Unlock()
	return s.createCheckpoint(workspaceID, actor, expectedParent, title, description, reproducibility, files)
}

// CaptureAndCreateCheckpoint holds the same admission lock as controlled file
// mutations from the first runtime read through durable lineage publication.
func (s *Store) CaptureAndCreateCheckpoint(workspaceID, actor, expectedParent, title, description string, reproducibility Reproducibility, capture func(Workspace) ([]CheckpointFile, error)) (Checkpoint, error) {
	control := s.controlLock(workspaceID)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	w, err := s.read(workspaceID)
	s.mu.Unlock()
	if err != nil {
		return Checkpoint{}, err
	}
	files, err := capture(w)
	if err != nil {
		return Checkpoint{}, err
	}
	provenance, err := s.readProvenance(workspaceID)
	if err != nil {
		return Checkpoint{}, err
	}
	// Legacy workspace records predate the private ledger; retain their bounded
	// evidence on the first checkpoint instead of silently dropping it.
	if len(provenance.Changes) == 0 && len(provenance.Commands) == 0 && (len(w.Changes) > 0 || len(w.Commands) > 0) {
		provenance.Changes, provenance.Commands = w.Changes, w.Commands
	}
	contributors, commands := checkpointEvidence(provenance.Changes, provenance.Commands, files, actor)
	return s.createCheckpointWithEvidence(workspaceID, actor, expectedParent, title, description, reproducibility, files, contributors, commands)
}

func (s *Store) createCheckpoint(workspaceID, actor, expectedParent, title, description string, reproducibility Reproducibility, files []CheckpointFile) (Checkpoint, error) {
	return s.createCheckpointWithEvidence(workspaceID, actor, expectedParent, title, description, reproducibility, files, nil, nil)
}

func (s *Store) createCheckpointWithEvidence(workspaceID, actor, expectedParent, title, description string, reproducibility Reproducibility, files []CheckpointFile, contributors []string, commands []CommandEvidence) (Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(workspaceID)
	if err != nil {
		return Checkpoint{}, err
	}
	if strings.TrimSpace(title) == "" || len(title) > 160 || len(description) > 2000 || len(reproducibility.Notes) > 2000 || expectedParent != w.HeadCheckpointID {
		if expectedParent != w.HeadCheckpointID {
			return Checkpoint{}, ErrCheckpointConflict
		}
		return Checkpoint{}, ErrInvalid
	}
	id, err := randomID(16)
	if err != nil {
		return Checkpoint{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	c := Checkpoint{ID: id, WorkspaceID: w.ID, RepositoryID: w.RepositoryID, BaseCommitID: w.CommitID, Definition: w.Definition, DefinitionSHA256: w.DefinitionSHA256, ParentCheckpointID: w.HeadCheckpointID, Title: strings.TrimSpace(title), Description: strings.TrimSpace(description), Reproducibility: reproducibility, Files: files, CreatedBy: actor, CreatedAt: s.now(), ContributorIDs: contributors, Commands: commands}
	if err = s.writeCheckpoint(c); err != nil {
		return Checkpoint{}, err
	}
	w.HeadCheckpointID = c.ID
	w.UpdatedAt = c.CreatedAt
	w.Events = append(w.Events, Event{Kind: "checkpoint.created", ActorID: actor, Role: "authorship", Detail: c.ID, CreatedAt: c.CreatedAt})
	if err = s.write(w); err != nil {
		return Checkpoint{}, err
	}
	return c.Public(), nil
}

func checkpointEvidence(changes []Change, commandHistory []CommandOutcome, files []CheckpointFile, creator string) ([]string, []CommandEvidence) {
	contributors, paths := map[string]bool{creator: true}, map[string]bool{}
	for _, file := range files {
		paths[file.Path] = true
	}
	for _, change := range changes {
		if paths[change.Path] {
			contributors[change.ActorID] = true
		}
	}
	commands := make([]CommandEvidence, 0, len(commandHistory))
	for _, command := range commandHistory {
		contributors[command.ActorID] = true
		commands = append(commands, CommandEvidence{ID: command.ID, SHA256: command.CommandSHA256, ExitCode: command.ExitCode, ActorID: command.ActorID})
	}
	ids := make([]string, 0, len(contributors))
	for id := range contributors {
		if id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, commands
}

// ClaimCheckpointPublication serializes the unpublished check through all Git
// and pull side effects across both goroutines and API processes.
func (s *Store) ClaimCheckpointPublication(workspaceID, id string) (Checkpoint, func(), error) {
	control := s.controlLock(workspaceID)
	control.Lock()
	file, err := os.OpenFile(filepath.Join(s.root, ".checkpoint-publication-"+workspaceID+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		control.Unlock()
		return Checkpoint{}, nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		control.Unlock()
		return Checkpoint{}, nil, err
	}
	release := func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close(); control.Unlock() }
	s.mu.Lock()
	checkpoint, err := s.readCheckpoint(workspaceID, id)
	s.mu.Unlock()
	if err != nil {
		release()
		return Checkpoint{}, nil, err
	}
	if checkpoint.Publication != nil {
		release()
		return checkpoint.Public(), nil, ErrCheckpointConflict
	}
	return checkpoint, release, nil
}

func (s *Store) GetCheckpoint(workspaceID, id string) (Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.readCheckpoint(workspaceID, id)
	if err != nil {
		return c, err
	}
	return c.Public(), nil
}

func (s *Store) CheckpointSnapshot(workspaceID, id string) (Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readCheckpoint(workspaceID, id)
}

func (s *Store) RecordCheckpointPublication(workspaceID, id string, publication Publication) (Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.readCheckpoint(workspaceID, id)
	if err != nil {
		return c, err
	}
	if c.Publication != nil {
		if c.Publication.CommitID == publication.CommitID && c.Publication.PullRequestID == publication.PullRequestID {
			return c.Public(), nil
		}
		return c, ErrCheckpointConflict
	}
	c.Publication = &publication
	if err = s.writeCheckpoint(c); err != nil {
		return c, err
	}
	w, err := s.read(workspaceID)
	if err == nil {
		w.UpdatedAt = publication.PublishedAt
		w.Events = append(w.Events, Event{Kind: "checkpoint.published", ActorID: publication.PublishedBy, Role: "authorship", Detail: id, CreatedAt: publication.PublishedAt})
		err = s.write(w)
	}
	return c.Public(), err
}

func (s *Store) ListCheckpoints(workspaceID string) ([]Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.read(workspaceID); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.root, "checkpoints", workspaceID)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Checkpoint{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Checkpoint{}
	for _, entry := range entries {
		c, e := s.readCheckpoint(workspaceID, strings.TrimSuffix(entry.Name(), ".json"))
		if e == nil {
			out = append(out, c.Public())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// RecordCheckpointRestore publishes restore lineage while the caller holds the
// workspace control lock. expectedHead binds publication to the preflight head;
// CreateCheckpoint takes that same lock, so it cannot interleave with apply.
func (s *Store) RecordCheckpointRestore(workspaceID, checkpointID, expectedHead, actor string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(workspaceID)
	if err != nil {
		return w, err
	}
	if _, err = s.readCheckpoint(workspaceID, checkpointID); err != nil {
		return w, err
	}
	if w.HeadCheckpointID != expectedHead {
		return w, ErrCheckpointConflict
	}
	w.HeadCheckpointID = checkpointID
	w.UpdatedAt = s.now()
	w.Events = append(w.Events, Event{Kind: "checkpoint.restored", ActorID: actor, Role: "authorship", Detail: checkpointID, CreatedAt: w.UpdatedAt})
	err = s.write(w)
	return w, err
}

func (s *Store) checkpointPath(workspaceID, id string) string {
	return filepath.Join(s.root, "checkpoints", workspaceID, id+".json")
}
func (s *Store) readCheckpoint(workspaceID, id string) (Checkpoint, error) {
	if len(workspaceID) != 32 || len(id) != 32 {
		return Checkpoint{}, ErrNotFound
	}
	b, err := os.ReadFile(s.checkpointPath(workspaceID, id))
	if os.IsNotExist(err) {
		return Checkpoint{}, ErrNotFound
	}
	if err != nil {
		return Checkpoint{}, err
	}
	var c Checkpoint
	if json.Unmarshal(b, &c) != nil || c.WorkspaceID != workspaceID {
		return Checkpoint{}, ErrNotFound
	}
	return c, nil
}
func (s *Store) writeCheckpoint(c Checkpoint) error {
	dir := filepath.Dir(s.checkpointPath(c.WorkspaceID, c.ID))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "."+c.ID+".tmp")
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, s.checkpointPath(c.WorkspaceID, c.ID)); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func SnapshotDigest(files []CheckpointFile) string {
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f.Path))
		h.Write([]byte{0})
		h.Write([]byte(f.Operation))
		h.Write([]byte{0})
		h.Write([]byte(f.SHA256))
		h.Write([]byte{0})
		h.Write([]byte(strconv.FormatUint(uint64(f.Mode), 8)))
	}
	return hex.EncodeToString(h.Sum(nil))
}
