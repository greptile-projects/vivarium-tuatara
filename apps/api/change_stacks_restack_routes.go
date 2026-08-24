package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changestacks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

var changeStackRestackMu sync.Mutex

type restackInput struct {
	RequestID string                `json:"request_id"`
	Members   []changestacks.Member `json:"members"`
}

func registerChangeStackRestackRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, stacks *changestacks.Store, pulls *pullrequests.Store, checks *checkruns.Store) {
	mux.HandleFunc("POST /repositories/{id}/change-stacks/{stack_id}/restacks", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "change_stack_restack_agent_forbidden", "a human contributor must propose a restack")
			return
		}
		var in restackInput
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.RequestID) == "" {
			writeAPIError(w, 400, "invalid_request", "a caller-stable request_id and complete ordered member list are required")
			return
		}
		retained, err := stacks.Get(r.PathValue("id"), r.PathValue("stack_id"))
		if errors.Is(err, changestacks.ErrNotFound) {
			writeAPIError(w, 404, "change_stack_not_found", "change stack not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "change_stacks_unavailable", "change stack could not be read")
			return
		}
		clean, _ := json.Marshal(in)
		digest := sha256.Sum256(clean)
		createdAt := time.Now().UTC().Truncate(time.Second)
		proposal, view, cleanup := previewChangeStackRestack(retained, in.Members, actor.UserID, git, catalog, pulls, checks, createdAt)
		defer cleanup()
		proposal.RequestID, proposal.RequestDigest, proposal.CreatedBy, proposal.CreatedAt = in.RequestID, hex.EncodeToString(digest[:]), actor.UserID, createdAt
		_, saved, err := stacks.ProposeRestack(r.PathValue("id"), retained.ID, proposal)
		if errors.Is(err, changestacks.ErrInvalid) {
			writeAPIError(w, 409, "change_stack_restack_request_reused", "request_id was already used for a different restack")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "change_stacks_unavailable", "restack preview could not be retained")
			return
		}
		_ = view // candidates intentionally remain only in the disposable preview
		writeJSON(w, 201, saved)
	})

	mux.HandleFunc("POST /repositories/{id}/change-stacks/{stack_id}/restacks/{restack_id}/apply", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "change_stack_restack_agent_forbidden", "a human contributor must apply a restack")
			return
		}
		changeStackRestackMu.Lock()
		defer changeStackRestackMu.Unlock()
		stack, err := stacks.Get(r.PathValue("id"), r.PathValue("stack_id"))
		if err != nil {
			writeAPIError(w, 404, "change_stack_not_found", "change stack not found")
			return
		}
		var retained *changestacks.Restack
		for i := range stack.Restacks {
			if stack.Restacks[i].ID == r.PathValue("restack_id") {
				retained = &stack.Restacks[i]
				break
			}
		}
		if retained == nil {
			writeAPIError(w, 404, "change_stack_restack_not_found", "restack preview not found")
			return
		}
		if retained.Status == "applied" {
			writeJSON(w, 200, *retained)
			return
		}
		if hasBlockingStackDiagnostic(retained.Diagnostics) {
			writeAPIError(w, 409, "change_stack_restack_blocked", "resolve every preview blocker before applying the restack")
			return
		}
		requested := make([]changestacks.Member, 0, len(retained.Members))
		for _, m := range retained.Members {
			requested = append(requested, m.Member)
		}
		// Reconcile a lost response after Git committed the transaction but
		// before the stack ledger was synchronized.
		if restackBranchesAtCandidates(*retained, git, stack.RepositoryID) {
			updated, applied, applyErr := stacks.ApplyRestack(stack.RepositoryID, stack.ID, retained.ID, actor.UserID, appliedRestackMembers(*retained))
			if applyErr != nil {
				writeAPIError(w, 500, "change_stack_restack_publication_uncertain", "branch updates are committed but stack reconciliation remains unavailable")
				return
			}
			for _, m := range updated.Members {
				if m.PullRequestID != "" {
					_, _ = pulls.SynchronizeSource(updated.RepositoryID, m.PullRequestID)
				}
			}
			writeJSON(w, 200, applied)
			return
		}
		fresh, view, cleanup := previewChangeStackRestack(stack, requested, actor.UserID, git, catalog, pulls, checks, retained.CreatedAt)
		defer cleanup()
		if hasBlockingStackDiagnostic(fresh.Diagnostics) || !sameRestackCandidates(*retained, fresh) {
			writeAPIError(w, 409, "change_stack_restack_stale", "a branch, permission, target, conflict, or shared-work condition changed; create a new preview")
			return
		}
		repository, err := git.Open(stack.RepositoryID)
		if err != nil {
			writeAPIError(w, 409, "change_stack_restack_blocked", "the source repository is unavailable")
			return
		}
		seen := map[string]bool{}
		remaining := int64(256 << 20)
		for _, m := range fresh.Members {
			if err = importStackObject([]string{view}, repository.Path(), m.CandidateRevision, seen, &remaining); err != nil {
				writeAPIError(w, 409, "change_stack_rewrite_failed", "rewritten commits could not be published")
				return
			}
		}
		var transaction strings.Builder
		transaction.WriteString("start\n")
		for _, m := range fresh.Members {
			fmt.Fprintf(&transaction, "update refs/heads/%s %s %s\n", strings.TrimPrefix(m.Member.SourceBranch, "refs/heads/"), m.CandidateRevision, m.ExpectedBranchTip)
		}
		transaction.WriteString("prepare\ncommit\n")
		command := exec.Command("git", "--git-dir="+repository.Path(), "update-ref", "--stdin")
		command.Stdin = strings.NewReader(transaction.String())
		if output, updateErr := command.CombinedOutput(); updateErr != nil {
			writeAPIError(w, 409, "change_stack_concurrent_push", "a source branch changed while applying; no branch was updated: "+strings.TrimSpace(string(output)))
			return
		}
		members := appliedRestackMembers(fresh)
		updated, applied, err := stacks.ApplyRestack(stack.RepositoryID, stack.ID, retained.ID, actor.UserID, members)
		if err != nil {
			writeAPIError(w, 500, "change_stack_restack_publication_uncertain", "branches were updated but the retained stack requires reconciliation")
			return
		}
		for _, m := range updated.Members {
			if m.PullRequestID != "" {
				_, _ = pulls.SynchronizeSource(updated.RepositoryID, m.PullRequestID)
			}
		}
		writeJSON(w, 200, applied)
	})
}

func previewChangeStackRestack(stack changestacks.Stack, requested []changestacks.Member, actor string, git *storage.Store, catalog *repositories.Store, pulls *pullrequests.Store, checks *checkruns.Store, fixed time.Time) (changestacks.Restack, string, func()) {
	result := changestacks.Restack{TargetRevision: stack.TargetRevision, Members: []changestacks.RestackMember{}, Removed: []changestacks.Member{}, Diagnostics: []changestacks.Diagnostic{}}
	cleanup := func() {}
	old := map[string]changestacks.Member{}
	for _, m := range stack.Members {
		old[m.ID] = m
	}
	seenIDs, seenBranches := map[string]bool{}, map[string]bool{}
	targetRepo, err := git.Open(stack.RepositoryID)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, diag("target_unavailable", "target Git history is unavailable", true))
		return result, "", cleanup
	}
	currentTarget, err := gitOutput(targetRepo.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(stack.TargetBranch, "refs/heads/")+"^{commit}")
	if err != nil || currentTarget != stack.TargetRevision {
		result.Diagnostics = append(result.Diagnostics, diag("target_moved", "target branch moved after the stack was published", true))
		return result, "", cleanup
	}
	if len(requested) == 0 || len(requested) > 50 {
		result.Diagnostics = append(result.Diagnostics, diag("invalid_order", "a restack requires one to fifty members", true))
		return result, "", cleanup
	}
	allPulls, pullErr := pulls.List(stack.RepositoryID)
	if pullErr != nil {
		result.Diagnostics = append(result.Diagnostics, diag("pulls_unavailable", "shared branch ownership could not be checked", true))
	}
	paths := []string{targetRepo.Path()}
	for i := range requested {
		m := &requested[i]
		prior, exists := old[m.ID]
		if exists {
			// Mutable collaboration fields are inherited; the public action may
			// change order, dependencies and criteria, not forge pull identity.
			prior.DependsOn, prior.AcceptanceCriteria = append([]string(nil), m.DependsOn...), append([]string(nil), m.AcceptanceCriteria...)
			m = &prior
			requested[i] = prior
		}
		if m.ID == "" || seenIDs[m.ID] || m.SourceBranch == "" || len(m.AcceptanceCriteria) == 0 {
			result.Diagnostics = append(result.Diagnostics, diag("invalid_member", "member identities, branches, and acceptance criteria must be unique and complete", true))
			continue
		}
		seenIDs[m.ID] = true
		branchKey := m.SourceRepositoryID + "/" + m.SourceBranch
		if seenBranches[branchKey] {
			result.Diagnostics = append(result.Diagnostics, diag("shared_branch", "two stack members cannot rewrite the same branch", true))
		}
		seenBranches[branchKey] = true
		sourceID := m.SourceRepositoryID
		if sourceID == "" {
			sourceID = stack.RepositoryID
			m.SourceRepositoryID = sourceID
			requested[i].SourceRepositoryID = sourceID
		}
		if !exists && m.PullRequestID != "" {
			p, pullGetErr := pulls.Get(stack.RepositoryID, m.PullRequestID)
			if pullGetErr != nil || p.Status != pullrequests.Open || p.SourceRepositoryID != sourceID || p.SourceBranch != m.SourceBranch {
				result.Diagnostics = append(result.Diagnostics, diag("pull_mismatch", "inserted member pull does not identify its exact open source branch", true))
				continue
			}
		}
		if sourceID != stack.RepositoryID {
			result.Diagnostics = append(result.Diagnostics, diag("independently_owned_repository", "cross-repository work must be restacked by its own owner", true))
			continue
		}
		repo, getErr := catalog.GetByID(sourceID)
		allowed := getErr == nil && repo.OwnerID == actor
		if !allowed {
			allowed, _ = catalog.HasCollaborator(actor, sourceID)
		}
		if !allowed {
			result.Diagnostics = append(result.Diagnostics, diag("access_revoked", "current branch push access is required", true))
			continue
		}
		source, openErr := git.Open(sourceID)
		if openErr != nil {
			result.Diagnostics = append(result.Diagnostics, diag("branch_inaccessible", "source repository is unavailable", true))
			continue
		}
		paths = appendUniquePath(paths, source.Path())
		tip, tipErr := gitOutput(source.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(m.SourceBranch, "refs/heads/")+"^{commit}")
		if tipErr != nil {
			result.Diagnostics = append(result.Diagnostics, diag("branch_missing", "source branch does not resolve to a commit", true))
			continue
		}
		m.Revision = tip
		requested[i].Revision = tip
		for _, p := range allPulls {
			if p.Status == pullrequests.Open && p.SourceRepositoryID == sourceID && p.SourceBranch == m.SourceBranch && p.ID != m.PullRequestID {
				result.Diagnostics = append(result.Diagnostics, diag("shared_branch", "source branch is also used by pull request "+p.ID, true))
			}
		}
	}
	for _, m := range stack.Members {
		if !seenIDs[m.ID] {
			result.Removed = append(result.Removed, m)
		}
	}
	if hasBlockingStackDiagnostic(result.Diagnostics) {
		return result, "", cleanup
	}
	view, viewCleanup, err := stackRangeView(paths[0], paths[0], stack.TargetRevision, requested[0].Revision, paths...)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, diag("rewrite_failed", "source objects could not be assembled", true))
		return result, "", cleanup
	}
	cleanup = viewCleanup
	seenObjects := map[string]bool{}
	remaining := int64(256 << 20)
	for _, m := range requested {
		base := m.BaseRevision
		if prior, ok := old[m.ID]; ok {
			base = prior.BaseRevision
		}
		if base == "" {
			if merge, mergeErr := gitOutput(view, "merge-base", stack.TargetRevision, m.Revision); mergeErr == nil {
				base = merge
			}
		}
		if base == "" || importStackObject(paths, view, base, seenObjects, &remaining) != nil || importStackObject(paths, view, m.Revision, seenObjects, &remaining) != nil {
			result.Diagnostics = append(result.Diagnostics, diag("rewrite_failed", "member "+m.ID+" has no readable patch base", true))
		}
	}
	if hasBlockingStackDiagnostic(result.Diagnostics) {
		return result, view, cleanup
	}
	parent := stack.TargetRevision
	when := fixed
	if when.IsZero() {
		when = time.Now().UTC()
	}
	for i, m := range requested {
		prior, existed := old[m.ID]
		base := m.BaseRevision
		if existed {
			base = prior.BaseRevision
		}
		if base == "" {
			base, _ = gitOutput(view, "merge-base", stack.TargetRevision, m.Revision)
		}
		commits := stackCommits(view, base, m.Revision)
		candidate, rewrites, rewriteErr := rewriteStackCommits(view, parent, commits, when)
		preview := changestacks.RestackMember{Member: m, OldRevision: prior.Revision, ExpectedBranchTip: m.Revision, CandidateRevision: candidate, CandidateBase: parent, RewrittenCommits: rewrites, PublishedBranchUpdate: candidate != m.Revision, Diagnostics: []changestacks.Diagnostic{}}
		if existed {
			preview.OldPosition = prior.Position
			preview.Action = "rebased"
			if prior.Position != i+1 {
				preview.Action = "reordered"
			}
		} else {
			preview.Action = "inserted"
		}
		if rewriteErr != nil {
			preview.Diagnostics = append(preview.Diagnostics, diag("rewrite_conflict", "member "+m.ID+" conflicts with its proposed parent: "+rewriteErr.Error(), true))
			result.Diagnostics = append(result.Diagnostics, preview.Diagnostics...)
		}
		if existed && m.PullRequestID != "" {
			if reviews, e := pulls.ListReviews(stack.RepositoryID, m.PullRequestID); e == nil {
				for _, x := range reviews {
					if !x.Stale && x.ReviewedCommitID == prior.Revision {
						preview.Impact.ReviewsInvalidated++
					}
				}
			}
			if checks != nil {
				if runs, e := checks.List(stack.RepositoryID, m.PullRequestID); e == nil {
					for _, x := range runs {
						if x.CommitID == prior.Revision {
							preview.Impact.ChecksInvalidated++
						}
					}
				}
			}
		}
		result.Members = append(result.Members, preview)
		if rewriteErr == nil {
			parent = candidate
		}
	}
	return result, view, cleanup
}

func rewriteStackCommits(gitDir, onto string, commits []string, fixed time.Time) (string, map[string]string, error) {
	parent := onto
	rewritten := map[string]string{}
	work, err := os.MkdirTemp("", "vivarium-restack-work-")
	if err != nil {
		return "", rewritten, err
	}
	defer os.RemoveAll(work)
	index := filepath.Join(work, "index")
	for _, commit := range commits {
		parents, e := gitOutput(gitDir, "show", "-s", "--format=%P", commit)
		if e != nil {
			return "", rewritten, e
		}
		oldParent := strings.Fields(parents)
		if len(oldParent) == 0 {
			return "", rewritten, errors.New("root commits cannot be restacked")
		}
		cmd := exec.Command("git", "--git-dir="+gitDir, "--work-tree="+work, "read-tree", parent+"^{tree}")
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+index)
		if out, x := cmd.CombinedOutput(); x != nil {
			return "", rewritten, errors.New(strings.TrimSpace(string(out)))
		}
		patch, x := exec.Command("git", "--git-dir="+gitDir, "diff-tree", "-p", "--binary", oldParent[0], commit).Output()
		if x != nil {
			return "", rewritten, x
		}
		apply := exec.Command("git", "--git-dir="+gitDir, "--work-tree="+work, "apply", "--cached", "--3way", "--whitespace=nowarn", "-")
		apply.Env = append(os.Environ(), "GIT_INDEX_FILE="+index)
		apply.Stdin = bytes.NewReader(patch)
		if out, x := apply.CombinedOutput(); x != nil {
			return "", rewritten, errors.New(strings.TrimSpace(string(out)))
		}
		treeCmd := exec.Command("git", "--git-dir="+gitDir, "write-tree")
		treeCmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+index)
		treeOut, x := treeCmd.Output()
		if x != nil {
			return "", rewritten, x
		}
		fields, x := exec.Command("git", "--git-dir="+gitDir, "show", "-s", "--format=%an%x00%ae%x00%aI%x00%cn%x00%ce%x00%B", commit).Output()
		if x != nil {
			return "", rewritten, x
		}
		parts := bytes.SplitN(fields, []byte{0}, 6)
		if len(parts) != 6 {
			return "", rewritten, errors.New("commit attribution is unreadable")
		}
		create := exec.Command("git", "--git-dir="+gitDir, "commit-tree", strings.TrimSpace(string(treeOut)), "-p", parent)
		create.Env = append(os.Environ(), "GIT_AUTHOR_NAME="+string(parts[0]), "GIT_AUTHOR_EMAIL="+string(parts[1]), "GIT_AUTHOR_DATE="+string(parts[2]), "GIT_COMMITTER_NAME="+string(parts[3]), "GIT_COMMITTER_EMAIL="+string(parts[4]), "GIT_COMMITTER_DATE="+fixed.Format(time.RFC3339))
		create.Stdin = bytes.NewReader(parts[5])
		out, x := create.Output()
		if x != nil {
			return "", rewritten, x
		}
		parent = strings.TrimSpace(string(out))
		rewritten[commit] = parent
	}
	if len(commits) == 0 {
		return "", rewritten, errors.New("member contains no commits relative to its retained base")
	}
	return parent, rewritten, nil
}

func sameRestackCandidates(a, b changestacks.Restack) bool {
	if a.TargetRevision != b.TargetRevision || len(a.Members) != len(b.Members) {
		return false
	}
	for i := range a.Members {
		if a.Members[i].Member.ID != b.Members[i].Member.ID || a.Members[i].ExpectedBranchTip != b.Members[i].ExpectedBranchTip || a.Members[i].CandidateRevision != b.Members[i].CandidateRevision {
			return false
		}
	}
	return true
}

func appliedRestackMembers(restack changestacks.Restack) []changestacks.Member {
	members := make([]changestacks.Member, 0, len(restack.Members))
	for i, preview := range restack.Members {
		m := preview.Member
		m.Position, m.Revision, m.BaseRevision, m.ExpectedBaseRevision = i+1, preview.CandidateRevision, preview.CandidateBase, preview.CandidateBase
		m.ReviewState = "published"
		now := time.Now().UTC()
		m.PublishedAt = &now
		members = append(members, m)
	}
	return members
}

func restackBranchesAtCandidates(restack changestacks.Restack, git *storage.Store, repositoryID string) bool {
	repository, err := git.Open(repositoryID)
	if err != nil || len(restack.Members) == 0 {
		return false
	}
	for _, member := range restack.Members {
		ref, readErr := repository.ReadReference("refs/heads/" + strings.TrimPrefix(member.Member.SourceBranch, "refs/heads/"))
		if readErr != nil || ref.Symbolic || ref.Target != member.CandidateRevision {
			return false
		}
	}
	return true
}
