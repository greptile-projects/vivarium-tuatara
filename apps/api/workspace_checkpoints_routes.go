package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

const checkpointBytesLimit = 32 << 20

type checkpointAnalysis struct {
	CheckpointID        string   `json:"checkpoint_id"`
	PreflightToken      string   `json:"preflight_token"`
	BaseDiverged        bool     `json:"base_diverged"`
	RepositoryHead      string   `json:"repository_head,omitempty"`
	Conflicts           []string `json:"conflicts"`
	MissingDependencies []string `json:"missing_dependencies"`
	Reproducible        bool     `json:"reproducible"`
	Reasons             []string `json:"reasons"`
}

func registerWorkspaceCheckpointRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, pulls *pullrequests.Store, store *workspaces.Store, authStore *auth.Store, checks *checkruns.Store, sessions *changesessions.Store) {
	mux.HandleFunc("GET /workspaces/{workspace_id}/checkpoints", func(w http.ResponseWriter, r *http.Request) {
		item, _, ok := authorizeWorkspace(w, r, store, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		items, err := store.ListCheckpoints(item.ID)
		if err != nil {
			writeAPIError(w, 500, "checkpoint_list_failed", "checkpoints could not be listed")
			return
		}
		writeJSON(w, 200, map[string]any{"items": items, "head_checkpoint_id": item.HeadCheckpointID})
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/checkpoints", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, store, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Title           string                     `json:"title"`
			Description     string                     `json:"description"`
			ExpectedParent  string                     `json:"expected_parent_checkpoint_id"`
			Reproducibility workspaces.Reproducibility `json:"reproducibility"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		captureFailed := false
		created, err := store.CaptureAndCreateCheckpoint(item.ID, actor.UserID, input.ExpectedParent, input.Title, input.Description, input.Reproducibility, func(current workspaces.Workspace) ([]workspaces.CheckpointFile, error) {
			files, captureErr := captureWorkspaceCheckpoint(git, current)
			captureFailed = captureErr != nil
			return files, captureErr
		})
		if captureFailed {
			writeAPIError(w, 422, "checkpoint_not_safe", err.Error())
			return
		}
		if errors.Is(err, workspaces.ErrCheckpointConflict) {
			writeAPIError(w, 409, "checkpoint_lineage_changed", "another checkpoint or restore changed the workspace lineage")
			return
		}
		if errors.Is(err, workspaces.ErrInvalid) {
			writeAPIError(w, 422, "checkpoint_invalid", "title and reproducibility metadata must be bounded")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "checkpoint_create_failed", "checkpoint could not be saved")
			return
		}
		w.Header().Set("Location", "/workspaces/"+item.ID+"/checkpoints/"+created.ID)
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /workspaces/{workspace_id}/checkpoints/{checkpoint_id}", func(w http.ResponseWriter, r *http.Request) {
		item, _, ok := authorizeWorkspace(w, r, store, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		c, err := store.GetCheckpoint(item.ID, r.PathValue("checkpoint_id"))
		if err != nil {
			writeAPIError(w, 404, "checkpoint_not_found", "checkpoint not found")
			return
		}
		writeJSON(w, 200, c)
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/checkpoints/{checkpoint_id}/publish", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, store, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		if pulls == nil {
			writeAPIError(w, 503, "checkpoint_publication_unavailable", "pull request governance is unavailable")
			return
		}
		var input struct {
			Branch         string `json:"branch"`
			ExpectedCommit string `json:"expected_commit_id"`
			TargetBranch   string `json:"target_branch"`
			Title          string `json:"title"`
			SessionID      string `json:"session_id"`
			CreatePull     bool   `json:"create_pull_request"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		checkpoint, err := store.CheckpointSnapshot(item.ID, r.PathValue("checkpoint_id"))
		if err != nil {
			writeAPIError(w, 404, "checkpoint_not_found", "checkpoint not found")
			return
		}
		if checkpoint.Publication != nil {
			if checkpoint.Publication.LinkPending {
				pull, pullErr := pulls.Get(item.RepositoryID, checkpoint.Publication.PullRequestID)
				if pullErr == nil {
					pull, pullErr = pulls.LinkWorkspace(item.RepositoryID, pull.ID, item.ID, checkpoint.ID, checkpoint.Publication.ContributorIDs, checkpoint.Publication.CommandIDs)
				}
				if pullErr == nil {
					checkpoint, pullErr = store.ConfirmCheckpointPublicationLink(item.ID, checkpoint.ID, pull.ID)
					if pullErr == nil {
						_ = store.ClearPublicationIntent(item.ID, checkpoint.ID)
						startCheckRuns(git, checks, pull)
					}
				}
				if pullErr != nil {
					w.Header().Set("Vivarium-Recovery-Publication", "pending")
					writeJSON(w, 202, map[string]any{"checkpoint": checkpoint.Public(), "pull_request": pull})
					return
				}
			}
			writeJSON(w, 200, checkpoint.Public())
			return
		}
		input.Branch, input.TargetBranch = strings.TrimSpace(input.Branch), strings.TrimSpace(input.TargetBranch)
		if input.TargetBranch == "" {
			input.TargetBranch = "main"
		}
		if input.Branch == "" || exec.Command("git", "check-ref-format", "--branch", input.Branch).Run() != nil || input.Branch == input.TargetBranch || len(checkpoint.Files) == 0 {
			writeAPIError(w, 422, "checkpoint_publication_invalid", "a non-empty checkpoint and distinct valid branch are required")
			return
		}
		if input.SessionID != "" {
			if decoded, decodeErr := hex.DecodeString(input.SessionID); decodeErr != nil || len(decoded) != 16 || item.Source.Kind != "proposal_task" {
				writeAPIError(w, 422, "checkpoint_publication_invalid", "session_id must identify this proposal-task publication")
				return
			}
			if sessions == nil {
				writeAPIError(w, 503, "checkpoint_publication_unavailable", "task session provenance is unavailable")
				return
			}
			if validateCheckpointSession(sessions, item, input.SessionID) != nil {
				writeAPIError(w, 422, "checkpoint_publication_invalid", "session_id does not belong to the workspace proposal task")
				return
			}
		}
		repository, err := git.Open(item.RepositoryID)
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		if intent, intentErr := store.GetPublicationIntent(item.ID, checkpoint.ID); intentErr == nil {
			if intent.Publication.Branch != input.Branch {
				writeAPIError(w, 409, "checkpoint_publication_pending", "checkpoint publication recovery requires the original branch")
				return
			}
			var pull pullrequests.PullRequest
			var repairErr error
			if intent.Publication.PullRequestID != "" {
				pull, repairErr = pulls.Get(item.RepositoryID, intent.Publication.PullRequestID)
			} else {
				ref, refErr := repository.ReadReference("refs/heads/" + intent.Publication.Branch)
				if refErr != nil || ref.Target != intent.Publication.CommitID {
					repairErr = errors.New("publication branch is not durable")
				}
			}
			if repairErr == nil && intent.Publication.PullRequestID != "" && intent.Publication.LinkPending {
				pull, repairErr = pulls.LinkWorkspace(item.RepositoryID, pull.ID, item.ID, checkpoint.ID, intent.Publication.ContributorIDs, intent.Publication.CommandIDs)
				if repairErr == nil {
					intent.Publication.LinkPending = false
					repairErr = store.SavePublicationIntent(intent)
				}
			}
			if repairErr == nil {
				checkpoint, repairErr = store.RecordCheckpointPublication(item.ID, checkpoint.ID, intent.Publication)
			}
			if repairErr != nil {
				w.Header().Set("Vivarium-Recovery-Publication", "pending")
				writeJSON(w, 202, map[string]any{"checkpoint": checkpoint.Public(), "pull_request": pull})
				return
			}
			_ = store.ClearPublicationIntent(item.ID, checkpoint.ID)
			if intent.Publication.PullRequestID != "" {
				startCheckRuns(git, checks, pull)
			}
			writeJSON(w, 200, map[string]any{"checkpoint": checkpoint, "pull_request": pull})
			return
		}
		if input.CreatePull {
			if _, targetErr := repository.ReadReference("refs/heads/" + input.TargetBranch); targetErr != nil {
				writeAPIError(w, 422, "checkpoint_publication_invalid", "target_branch must identify an existing branch")
				return
			}
			if title := strings.TrimSpace(input.Title); len(title) > 200 {
				writeAPIError(w, 422, "checkpoint_publication_invalid", "title must be at most 200 characters")
				return
			}
		}
		checkpoint, releasePublication, claimErr := store.ClaimCheckpointPublication(item.ID, checkpoint.ID)
		if errors.Is(claimErr, workspaces.ErrCheckpointConflict) {
			writeJSON(w, 200, checkpoint.Public())
			return
		}
		if claimErr != nil {
			writeAPIError(w, 500, "checkpoint_publication_unavailable", "checkpoint publication could not be reserved")
			return
		}
		defer releasePublication()
		refName := "refs/heads/" + input.Branch
		current, refErr := repository.ReadReference(refName)
		if refErr == nil {
			if input.ExpectedCommit == "" || current.Target != input.ExpectedCommit || current.Target != checkpoint.BaseCommitID {
				writeAPIError(w, 409, "workspace_branch_changed", "the working branch no longer names the checkpoint base")
				return
			}
		} else if !errors.Is(refErr, storage.ErrReferenceNotFound) || (input.ExpectedCommit != "" && input.ExpectedCommit != checkpoint.BaseCommitID) {
			writeAPIError(w, 409, "workspace_branch_changed", "the branch expectation does not match repository state")
			return
		}
		commitID, err := commitCheckpoint(repository.Path(), checkpoint, actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "checkpoint_commit_failed", "checkpoint could not be committed")
			return
		}
		contributors, commandIDs := workspacePublicationEvidence(checkpoint)
		intent := workspaces.PublicationIntent{WorkspaceID: item.ID, CheckpointID: checkpoint.ID, Publication: workspaces.Publication{Branch: input.Branch, CommitID: commitID, TaskID: item.Source.TaskID, SessionID: input.SessionID, ContributorIDs: contributors, CommandIDs: commandIDs, LinkPending: input.CreatePull, PublishedBy: actor.UserID, PublishedAt: time.Now().UTC()}}
		if err = store.SavePublicationIntent(intent); err != nil {
			writeAPIError(w, 500, "checkpoint_publication_unavailable", "publication recovery could not be reserved")
			return
		}
		newRef := storage.Reference{Name: refName, Target: commitID}
		if refErr == nil {
			err = repository.UpdateReferenceIfTarget(newRef, current.Target)
		} else {
			err = repository.CreateReference(newRef)
		}
		if err != nil {
			_ = store.ClearPublicationIntent(item.ID, checkpoint.ID)
			writeAPIError(w, 409, "workspace_branch_changed", "the branch changed while publishing")
			return
		}
		var pull *pullrequests.PullRequest
		if input.CreatePull {
			title := strings.TrimSpace(input.Title)
			if title == "" {
				title = checkpoint.Title
			}
			body := workspacePullBody(item, checkpoint, contributors, commandIDs)
			var created pullrequests.PullRequest
			if item.Source.Kind == "proposal_task" {
				proposalID, taskID := item.Source.ProposalID, item.Source.TaskID
				var sessionID *string
				if input.SessionID != "" {
					sessionID = &input.SessionID
				}
				created, err = pulls.CreateTaskContribution(item.RepositoryID, actor.UserID, title, body, input.Branch, input.TargetBranch, commitID, []string{commitID}, &proposalID, &taskID, sessionID, nil)
			} else {
				created, err = pulls.Create(item.RepositoryID, actor.UserID, title, body, input.Branch, input.TargetBranch, nil)
			}
			if err != nil {
				if refErr == nil {
					_ = repository.UpdateReferenceIfTarget(storage.Reference{Name: refName, Target: current.Target}, commitID)
				} else {
					_ = repository.DeleteReferenceIfTarget(refName, commitID)
				}
				_ = store.ClearPublicationIntent(item.ID, checkpoint.ID)
				writeAPIError(w, 409, "checkpoint_pull_failed", "branch was published but pull request creation failed")
				return
			}
			intent.Publication.PullRequestID = created.ID
			if err = store.SavePublicationIntent(intent); err != nil {
				writeAPIError(w, 500, "checkpoint_link_failed", "pull request was created and publication recovery is pending")
				return
			}
			created, err = pulls.LinkWorkspace(item.RepositoryID, created.ID, item.ID, checkpoint.ID, contributors, commandIDs)
			if err != nil {
				pending, recordErr := store.RecordCheckpointPublication(item.ID, checkpoint.ID, workspaces.Publication{Branch: input.Branch, CommitID: commitID, PullRequestID: created.ID, TaskID: item.Source.TaskID, SessionID: input.SessionID, ContributorIDs: contributors, CommandIDs: commandIDs, LinkPending: true, PublishedBy: actor.UserID, PublishedAt: time.Now().UTC()})
				if recordErr != nil {
					writeAPIError(w, 500, "checkpoint_link_failed", "publication recovery could not be recorded")
					return
				}
				w.Header().Set("Vivarium-Recovery-Publication", "pending")
				writeJSON(w, 202, map[string]any{"checkpoint": pending, "pull_request": created})
				return
			}
			intent.Publication.LinkPending = false
			if err = store.SavePublicationIntent(intent); err != nil {
				writeAPIError(w, 500, "checkpoint_link_failed", "linked pull publication recovery is pending")
				return
			}
			pull = &created
		}
		pullID := ""
		if pull != nil {
			pullID = pull.ID
		}
		published, err := store.RecordCheckpointPublication(item.ID, checkpoint.ID, workspaces.Publication{Branch: input.Branch, CommitID: commitID, PullRequestID: pullID, TaskID: item.Source.TaskID, SessionID: input.SessionID, ContributorIDs: contributors, CommandIDs: commandIDs, PublishedBy: actor.UserID, PublishedAt: time.Now().UTC()})
		if err != nil {
			writeAPIError(w, 500, "checkpoint_link_failed", "Git publication succeeded but checkpoint attribution is pending")
			return
		}
		_ = store.ClearPublicationIntent(item.ID, checkpoint.ID)
		if pull != nil {
			startCheckRuns(git, checks, *pull)
		}
		writeJSON(w, 201, map[string]any{"checkpoint": published, "pull_request": pull})
	})
	mux.HandleFunc("GET /workspaces/{workspace_id}/checkpoints/{checkpoint_id}/restore", func(w http.ResponseWriter, r *http.Request) {
		item, _, ok := authorizeRunningWorkspace(w, r, store, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		a, err := analyzeCheckpointRestore(git, store, item, r.PathValue("checkpoint_id"))
		if err != nil {
			writeAPIError(w, 404, "checkpoint_not_found", "checkpoint not found")
			return
		}
		writeJSON(w, 200, a)
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/checkpoints/{checkpoint_id}/restore", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, store, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			PreflightToken string `json:"preflight_token"`
			AllowConflicts bool   `json:"allow_conflicts"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		a, err := analyzeCheckpointRestore(git, store, item, r.PathValue("checkpoint_id"))
		if err != nil {
			writeAPIError(w, 404, "checkpoint_not_found", "checkpoint not found")
			return
		}
		if input.PreflightToken == "" || input.PreflightToken != a.PreflightToken {
			writeAPIError(w, 409, "checkpoint_preflight_changed", "workspace state changed; inspect restore again")
			return
		}
		if len(a.Conflicts) > 0 && !input.AllowConflicts {
			writeAPIError(w, 409, "checkpoint_restore_conflicts", "restore has conflicting live paths")
			return
		}
		checkpoint, _ := store.CheckpointSnapshot(item.ID, a.CheckpointID)
		var updated workspaces.Workspace
		err = store.WithControl(item.ID, actor.UserID, "files", func(current workspaces.Workspace) error {
			fresh, freshErr := analyzeCheckpointRestore(git, store, current, r.PathValue("checkpoint_id"))
			if freshErr != nil {
				return freshErr
			}
			if fresh.PreflightToken != input.PreflightToken {
				return workspaces.ErrCheckpointConflict
			}
			if applyErr := applyCheckpoint(catalog, current, actor, checkpoint); applyErr != nil {
				return applyErr
			}
			var recordErr error
			updated, recordErr = store.RecordCheckpointRestore(item.ID, checkpoint.ID, current.HeadCheckpointID, actor.UserID)
			return recordErr
		})
		if errors.Is(err, workspaces.ErrControl) {
			writeAPIError(w, 409, "workspace_control_required", "live file control is held by another participant or has expired")
			return
		}
		if errors.Is(err, workspaces.ErrCheckpointConflict) {
			writeAPIError(w, 409, "checkpoint_preflight_changed", "workspace state changed; inspect restore again")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "checkpoint_restore_failed", "checkpoint could not be restored")
			return
		}
		writeJSON(w, 200, map[string]any{"workspace": updated, "analysis": a})
	})
}

func validateCheckpointSession(store *changesessions.Store, workspace workspaces.Workspace, sessionID string) error {
	if store == nil || workspace.Source.Kind != "proposal_task" {
		return errors.New("task session provenance unavailable")
	}
	session, err := store.Get(workspace.RepositoryID, workspace.Source.TaskID, sessionID)
	if err != nil {
		return err
	}
	if session.RepositoryID != workspace.RepositoryID || session.ProposalID != workspace.Source.ProposalID || session.TaskID != workspace.Source.TaskID {
		return errors.New("task session does not match workspace source")
	}
	return nil
}

func captureWorkspaceCheckpoint(git *storage.Store, w workspaces.Workspace) ([]workspaces.CheckpointFile, error) {
	repo, err := git.Open(w.RepositoryID)
	if err != nil {
		return nil, err
	}
	base, runtime, cleanup, err := checkpointTrees(repo.Path(), w)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return compareCheckpointTrees(base, runtime)
}

func checkpointTrees(gitPath string, w workspaces.Workspace) (string, string, func(), error) {
	root, err := os.MkdirTemp("", "vivarium-checkpoint-")
	if err != nil {
		return "", "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(root) }
	base := filepath.Join(root, "base")
	runtime := filepath.Join(root, "runtime")
	os.MkdirAll(base, 0700)
	os.MkdirAll(runtime, 0700)
	archive := exec.Command("git", "--git-dir="+gitPath, "archive", w.CommitID)
	extract := exec.Command("tar", "-x", "-C", base)
	pipe, e := archive.StdoutPipe()
	if e != nil {
		cleanup()
		return "", "", func() {}, e
	}
	extract.Stdin = pipe
	if e = archive.Start(); e == nil {
		e = extract.Run()
		if x := archive.Wait(); e == nil {
			e = x
		}
	}
	if e != nil {
		cleanup()
		return "", "", func() {}, e
	}
	container := "vivarium-workspace-" + w.ID
	capture := exec.Command("docker", "exec", container, "tar", "-c", "-C", "/workspace", ".")
	extractRuntime := exec.Command("tar", "-x", "-C", runtime)
	pipe, e = capture.StdoutPipe()
	if e != nil {
		cleanup()
		return "", "", func() {}, e
	}
	extractRuntime.Stdin = pipe
	if e = capture.Start(); e == nil {
		e = extractRuntime.Run()
		if x := capture.Wait(); e == nil {
			e = x
		}
	}
	if e != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("workspace runtime unavailable: %w", e)
	}
	return base, runtime, cleanup, nil
}

type diskEntry struct {
	mode os.FileMode
	data []byte
	sum  string
}

func scanCheckpointTree(root string) (map[string]diskEntry, error) {
	out := map[string]diskEntry{}
	total := 0
	err := filepath.WalkDir(root, func(name string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, e := filepath.Rel(root, name)
		if e != nil || rel == "." {
			return e
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a reproducible regular repository file", rel)
		}
		data, e := os.ReadFile(name)
		if e != nil {
			return e
		}
		total += len(data)
		if total > checkpointBytesLimit {
			return errors.New("repository changes exceed the 32 MiB checkpoint limit")
		}
		sum := sha256.Sum256(data)
		out[rel] = diskEntry{info.Mode(), data, hex.EncodeToString(sum[:])}
		return nil
	})
	return out, err
}
func compareCheckpointTrees(baseRoot, runtimeRoot string) ([]workspaces.CheckpointFile, error) {
	base, err := scanCheckpointTree(baseRoot)
	if err != nil {
		return nil, err
	}
	runtime, err := scanCheckpointTree(runtimeRoot)
	if err != nil {
		return nil, err
	}
	paths := map[string]bool{}
	for p := range base {
		paths[p] = true
	}
	for p := range runtime {
		paths[p] = true
	}
	names := []string{}
	for p := range paths {
		names = append(names, p)
	}
	sort.Strings(names)
	out := []workspaces.CheckpointFile{}
	for _, p := range names {
		b, bok := base[p]
		v, vok := runtime[p]
		if bok && vok && b.sum == v.sum && b.mode.Perm() == v.mode.Perm() {
			continue
		}
		if vok && looksCredentialPath(p) {
			return nil, fmt.Errorf("%s looks credential-bearing and was not captured", p)
		}
		f := workspaces.CheckpointFile{Path: p}
		if !vok {
			f.Operation = "delete"
		} else {
			if bok {
				f.Operation = "modify"
			} else {
				f.Operation = "add"
			}
			f.Mode = uint32(v.mode.Perm())
			f.Size = int64(len(v.data))
			f.SHA256 = v.sum
			if looksCredentialContent(v.data) {
				return nil, fmt.Errorf("%s contains credential-like material and was not captured", p)
			}
			f.ContentB64 = base64.StdEncoding.EncodeToString(v.data)
			if utf8.Valid(v.data) && bytes.IndexByte(v.data, 0) < 0 && len(v.data) <= 128*1024 {
				f.Patch = textPatch(p, b.data, v.data)
			}
		}
		out = append(out, f)
	}
	return out, nil
}
func looksCredentialPath(p string) bool {
	p = strings.ToLower(p)
	base := filepath.Base(p)
	if base == ".env" || strings.HasPrefix(base, ".env.") || base == ".npmrc" || base == ".yarnrc.yml" || base == ".pypirc" || base == ".netrc" || base == ".git-credentials" || strings.Contains(p, "credentials") || strings.Contains(p, ".ssh/") || strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") {
		return true
	}
	return false
}
func looksCredentialContent(data []byte) bool {
	s := strings.ToUpper(string(data))
	markers := []string{"-----BEGIN PRIVATE KEY-----", "-----BEGIN OPENSSH PRIVATE KEY-----", "AWS_SECRET_ACCESS_KEY=", "GITHUB_TOKEN=", "OPENAI_API_KEY=", "PASSWORD=", "CLIENT_SECRET=", "_AUTH"}
	for _, marker := range markers {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}
func textPatch(p string, old, new []byte) string {
	root, _ := os.MkdirTemp("", "vivarium-diff-")
	defer os.RemoveAll(root)
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	os.WriteFile(a, old, 0600)
	os.WriteFile(b, new, 0600)
	cmd := exec.Command("git", "diff", "--no-index", "--no-ext-diff", "--", a, b)
	out, _ := cmd.Output()
	s := string(out)
	lines := strings.Split(s, "\n")
	if len(lines) > 2 {
		lines[0] = "--- a/" + p
		lines[1] = "+++ b/" + p
	}
	if len(strings.Join(lines, "\n")) > 64*1024 {
		return "diff too large to display"
	}
	return strings.Join(lines, "\n")
}

func analyzeCheckpointRestore(git *storage.Store, store *workspaces.Store, w workspaces.Workspace, id string) (checkpointAnalysis, error) {
	c, err := store.CheckpointSnapshot(w.ID, id)
	if err != nil {
		return checkpointAnalysis{}, err
	}
	current, err := captureWorkspaceCheckpoint(git, w)
	if err != nil {
		return checkpointAnalysis{}, err
	}
	currentBy := map[string]string{}
	for _, f := range current {
		currentBy[f.Path] = fmt.Sprintf("%s:%s:%o", f.Operation, f.SHA256, f.Mode)
	}
	conflicts := []string{}
	for _, f := range c.Files {
		if v, ok := currentBy[f.Path]; ok && v != fmt.Sprintf("%s:%s:%o", f.Operation, f.SHA256, f.Mode) {
			conflicts = append(conflicts, f.Path)
		}
	}
	repo, _ := git.Open(w.RepositoryID)
	headBytes, _ := exec.Command("git", "--git-dir="+repo.Path(), "rev-parse", "refs/heads/main").Output()
	head := strings.TrimSpace(string(headBytes))
	missing := missingDependencies(c.Definition.Dependencies, c.Reproducibility.Dependencies)
	reasons := []string{}
	if len(missing) > 0 {
		reasons = append(reasons, "declared dependencies do not cover the frozen environment")
	}
	if c.DefinitionSHA256 != w.DefinitionSHA256 {
		reasons = append(reasons, "workspace environment definition differs from the checkpoint")
	}
	token := sha256.Sum256([]byte(id + "\x00" + w.HeadCheckpointID + "\x00" + workspaces.SnapshotDigest(current)))
	return checkpointAnalysis{id, hex.EncodeToString(token[:]), head != "" && head != c.BaseCommitID, head, conflicts, missing, len(reasons) == 0, reasons}, nil
}
func missingDependencies(required, declared []string) []string {
	seen := map[string]bool{}
	for _, v := range declared {
		seen[strings.TrimSpace(v)] = true
	}
	out := []string{}
	for _, v := range required {
		if !seen[v] {
			out = append(out, v)
		}
	}
	return out
}

const checkpointRestoreScript = `set -eu
root=$1
tx=$(mktemp -d "$root/.vivarium-restore.XXXXXX")
applying=0
physical_parent() {
	candidate=$1
	while [ ! -d "$candidate" ]; do candidate=$(dirname "$candidate"); done
	(cd -P -- "$candidate" && pwd -P)
}
rollback() {
	while IFS="$(printf '\t')" read -r operation mode encoded payload; do
		path=$(printf '%s' "$encoded" | base64 -d)
		target="$root/$path"
		rm -rf -- "$target"
		if [ -e "$tx/backup/$payload" ] || [ -L "$tx/backup/$payload" ]; then
			mkdir -p -- "$(dirname "$target")"
			cp -a -- "$tx/backup/$payload" "$target"
		fi
	done < "$tx/manifest"
	while read -r encoded; do
		directory=$(printf '%s' "$encoded" | base64 -d)
		rmdir -- "$directory" 2>/dev/null || true
	done < "$tx/missing-dirs"
}
finish() {
	status=$?
	if [ "$applying" = 1 ] && [ "$status" != 0 ]; then rollback || true; fi
	rm -rf -- "$tx"
	exit "$status"
}
trap finish EXIT HUP INT TERM
tar -x -C "$tx"
mkdir "$tx/backup"
: > "$tx/missing-dirs"
while IFS="$(printf '\t')" read -r operation mode encoded payload; do
	path=$(printf '%s' "$encoded" | base64 -d)
	case "$path" in ""|/*|../*|*/../*|*/..) exit 42 ;; esac
	parent=$(physical_parent "$root/$(dirname "$path")")
	case "$parent" in "$root"|"$root"/*) ;; *) exit 42 ;; esac
	target="$root/$path"
	if [ -e "$target" ] || [ -L "$target" ]; then cp -a -- "$target" "$tx/backup/$payload"; fi
done < "$tx/manifest"
applying=1
while IFS="$(printf '\t')" read -r operation mode encoded payload; do
	path=$(printf '%s' "$encoded" | base64 -d)
	target="$root/$path"
	if [ "$operation" = delete ]; then rm -rf -- "$target"; continue; fi
	parent=$(dirname "$target")
	while [ "$parent" != "$root" ]; do
		if [ ! -e "$parent" ] && [ ! -L "$parent" ]; then
			printf '%s' "$parent" | base64 | tr -d '\n' >> "$tx/missing-dirs"
			printf '\n' >> "$tx/missing-dirs"
		fi
		parent=$(dirname "$parent")
	done
	mkdir -p -- "$(dirname "$target")"
	parent=$(cd -P -- "$(dirname "$target")" && pwd -P)
	case "$parent" in "$root"|"$root"/*) ;; *) exit 42 ;; esac
	cp -- "$tx/payload/$payload" "$target.vivarium-new"
	chmod "$mode" "$target.vivarium-new"
	rm -rf -- "$target"
	mv -f -- "$target.vivarium-new" "$target"
done < "$tx/manifest"
applying=0`

func applyCheckpoint(catalog *repositories.Store, w workspaces.Workspace, actor auth.Credential, c workspaces.Checkpoint) error {
	archive, err := checkpointRestoreArchive(c)
	if err != nil {
		return err
	}
	_, err = workspaceAuthorizedExec(catalog, w, actor, true, 60*time.Second, "/workspace", bytes.NewReader(archive), "sh", "-c", checkpointRestoreScript, "sh", "/workspace")
	return err
}

func checkpointRestoreArchive(c workspaces.Checkpoint) ([]byte, error) {
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	var manifest strings.Builder
	for _, f := range c.Files {
		encoded := base64.StdEncoding.EncodeToString([]byte(f.Path))
		payloadSum := sha256.Sum256([]byte(f.Path))
		payload := hex.EncodeToString(payloadSum[:])
		fmt.Fprintf(&manifest, "%s\t%o\t%s\t%s\n", f.Operation, f.Mode, encoded, payload)
		if f.Operation == "delete" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(f.ContentB64)
		if err != nil {
			return nil, err
		}
		if err = tw.WriteHeader(&tar.Header{Name: "payload/" + payload, Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return nil, err
		}
		if _, err = tw.Write(data); err != nil {
			return nil, err
		}
	}
	manifestBytes := []byte(manifest.String())
	if err := tw.WriteHeader(&tar.Header{Name: "manifest", Mode: 0600, Size: int64(len(manifestBytes)), Typeflag: tar.TypeReg}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return archive.Bytes(), nil
}

func commitCheckpoint(gitPath string, checkpoint workspaces.Checkpoint, actor string) (string, error) {
	index, err := os.CreateTemp("", "vivarium-publish-index-")
	if err != nil {
		return "", err
	}
	indexPath := index.Name()
	_ = index.Close()
	_ = os.Remove(indexPath)
	defer os.Remove(indexPath)
	run := func(stdin []byte, args ...string) ([]byte, error) {
		cmd := exec.Command("git", append([]string{"--git-dir=" + gitPath}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+indexPath)
		if stdin != nil {
			cmd.Stdin = bytes.NewReader(stdin)
		}
		out, runErr := cmd.CombinedOutput()
		if runErr != nil {
			return nil, fmt.Errorf("git %s: %w: %s", args[0], runErr, out)
		}
		return bytes.TrimSpace(out), nil
	}
	if _, err = run(nil, "read-tree", checkpoint.BaseCommitID); err != nil {
		return "", err
	}
	for _, file := range checkpoint.Files {
		if file.Operation == "delete" {
			if _, err = run([]byte("0 0000000000000000000000000000000000000000\t"+file.Path+"\n"), "update-index", "--index-info"); err != nil {
				return "", err
			}
			continue
		}
		content, decodeErr := base64.StdEncoding.DecodeString(file.ContentB64)
		if decodeErr != nil {
			return "", decodeErr
		}
		blob, hashErr := run(content, "hash-object", "-w", "--stdin")
		if hashErr != nil {
			return "", hashErr
		}
		mode := "100644"
		if file.Mode&0111 != 0 {
			mode = "100755"
		}
		if _, err = run(nil, "update-index", "--add", "--cacheinfo", mode+","+string(blob)+","+file.Path); err != nil {
			return "", err
		}
	}
	tree, err := run(nil, "write-tree")
	if err != nil {
		return "", err
	}
	message := checkpoint.Title + "\n\nPublished from workspace " + checkpoint.WorkspaceID + " checkpoint " + checkpoint.ID
	cmd := exec.Command("git", "--git-dir="+gitPath, "commit-tree", string(tree), "-p", checkpoint.BaseCommitID, "-F", "-")
	cmd.Stdin = strings.NewReader(message)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Vivarium collaborator", "GIT_AUTHOR_EMAIL="+actor+"@users.vivarium.invalid", "GIT_COMMITTER_NAME=Vivarium", "GIT_COMMITTER_EMAIL=platform@vivarium.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("commit checkpoint: %w: %s", err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func workspacePublicationEvidence(checkpoint workspaces.Checkpoint) ([]string, []string) {
	contributors := append([]string(nil), checkpoint.ContributorIDs...)
	commands := make([]string, 0, len(checkpoint.Commands))
	for _, command := range checkpoint.Commands {
		commands = append(commands, command.ID)
	}
	return contributors, commands
}

func workspacePullBody(workspace workspaces.Workspace, checkpoint workspaces.Checkpoint, contributors, commands []string) string {
	changes := make([]string, 0, len(checkpoint.Files))
	for _, file := range checkpoint.Files {
		changes = append(changes, "- `"+file.Operation+"` `"+file.Path+"` (`"+file.SHA256+"`)")
	}
	commandLines := []string{"- No recorded commands."}
	if len(commands) > 0 {
		commandLines = commandLines[:0]
		for _, command := range checkpoint.Commands {
			commandLines = append(commandLines, "- `"+command.ID+"` digest `"+command.SHA256+"`, exit "+strconv.Itoa(command.ExitCode)+", by `"+command.ActorID+"`")
		}
	}
	body := checkpoint.Description + "\n\n## Workspace provenance\n\nWorkspace `" + workspace.ID + "`; checkpoint `" + checkpoint.ID + "`; exact base `" + checkpoint.BaseCommitID + "`. Contributors: `" + strings.Join(contributors, "`, `") + "`.\n\n## Changes\n\n" + strings.Join(changes, "\n") + "\n\n## Commands performed\n\n" + strings.Join(commandLines, "\n") + "\n\nOnly the checkpoint's inspected repository-file manifest was committed; workspace activity, outputs, and credentials were not exported."
	runes := []rune(body)
	if len(runes) > 19000 {
		body = string(runes[:19000]) + "\n\n_Evidence summary truncated; the checkpoint manifest remains authoritative._"
	}
	return body
}
