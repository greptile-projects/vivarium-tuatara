package proposals

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	repositoryID = "0123456789abcdef0123456789abcdef"
	authorID     = "abcdefabcdefabcdefabcdefabcdefab"
	commenterID  = "11111111111111111111111111111111"
)

func TestProposalAndConversationPersistWithAttribution(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	created, err := store.Create(repositoryID, authorID, "An idea", "Shared context")
	if err != nil {
		t.Fatal(err)
	}
	comment, err := store.AddComment(repositoryID, created.ID, commenterID, "Useful feedback")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.Update(repositoryID, created.ID, Patch{Title: pointer("A refined idea"), Status: pointer(Closed)})
	if err != nil {
		t.Fatal(err)
	}
	if updated.AuthorID != authorID || updated.Status != Closed || updated.ClosedAt == nil {
		t.Fatalf("updated = %#v", updated)
	}

	reopened, _ := New(root)
	got, err := reopened.Get(repositoryID, created.ID)
	if err != nil || got.ID != updated.ID || got.Title != updated.Title || got.Status != Closed || got.ClosedAt == nil || !got.ClosedAt.Equal(*updated.ClosedAt) {
		t.Fatalf("reopened = %#v, %v", got, err)
	}
	comments, err := reopened.ListComments(repositoryID, created.ID)
	if err != nil || len(comments) != 1 || comments[0] != comment || comments[0].AuthorID != commenterID {
		t.Fatalf("comments = %#v, %v", comments, err)
	}
	if _, err := reopened.Update(repositoryID, created.ID, Patch{Status: pointer(Closed)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("second close: %v", err)
	}
}

func TestCorrectiveWorkPublishesAtomicallyAndDeduplicatesRetry(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	input := CorrectiveWorkInput{IncidentID: strings.Repeat("2", 32), OperationID: strings.Repeat("3", 32), RepositoryID: repositoryID, ActorID: authorID, ProposalTitle: "Prevent recurrence", ProposalBody: "Bound the failure mode.", TaskTitle: "Add saturation guard", Outcome: "Load tests prove bounded concurrency.", AssigneeID: commenterID, BaseRevision: strings.Repeat("4", 40), DueAt: time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)}
	proposal, task, err := store.CreateCorrectiveWork(input)
	if err != nil || task.Assignment == nil || task.Assignment.AssigneeID != commenterID {
		t.Fatalf("created = %#v %#v, %v", proposal, task, err)
	}
	reopened, _ := New(root)
	retryProposal, retryTask, err := reopened.CreateCorrectiveWork(input)
	if err != nil || retryProposal.ID != proposal.ID || retryTask.ID != task.ID {
		t.Fatalf("retry = %#v %#v, %v", retryProposal, retryTask, err)
	}
	items, err := reopened.List(repositoryID)
	if err != nil || len(items) != 1 {
		t.Fatalf("proposals = %#v, %v", items, err)
	}
	input.Outcome = "Changed operation content"
	if _, _, err := reopened.CreateCorrectiveWork(input); !errors.Is(err, ErrCorrectiveConflict) {
		t.Fatalf("changed retry = %v", err)
	}
}

func TestConcurrentCommentsAreNotLostAcrossStores(t *testing.T) {
	root := t.TempDir()
	first, _ := New(root)
	second, _ := New(root)
	proposal, _ := first.Create(repositoryID, authorID, "Discuss", "")
	stores := []*Store{first, second}
	var wg sync.WaitGroup
	for i, store := range stores {
		wg.Add(1)
		go func(store *Store, body string) {
			defer wg.Done()
			if _, err := store.AddComment(repositoryID, proposal.ID, commenterID, body); err != nil {
				t.Errorf("comment: %v", err)
			}
		}(store, []string{"first", "second"}[i])
	}
	wg.Wait()
	comments, err := first.ListComments(repositoryID, proposal.ID)
	if err != nil || len(comments) != 2 {
		t.Fatalf("comments = %#v, %v", comments, err)
	}
}

func TestProposalTasksDeriveReadinessAndRetainHistory(t *testing.T) {
	store, _ := New(t.TempDir())
	proposal, _ := store.Create(repositoryID, authorID, "Ship onboarding", "Discuss the path")
	comment, _ := store.AddComment(repositoryID, proposal.ID, commenterID, "Start with the API")
	first, err := store.CreateTask(repositoryID, proposal.ID, authorID, "Define contract", "A documented task API", nil, []string{comment.ID})
	if err != nil || !first.Ready || len(first.DependencyIDs) != 0 {
		t.Fatalf("first = %#v, %v", first, err)
	}
	second, err := store.CreateTask(repositoryID, proposal.ID, commenterID, "Build UI", "Collaborators can manage the plan", []string{first.ID}, nil)
	if err != nil || second.Ready || len(second.BlockedBy) != 1 {
		t.Fatalf("second = %#v, %v", second, err)
	}
	completed := TaskCompleted
	if _, err := store.UpdateTask(repositoryID, proposal.ID, first.ID, commenterID, TaskPatch{Status: &completed}); err != nil {
		t.Fatal(err)
	}
	tasks, err := store.ListTasks(repositoryID, proposal.ID)
	if err != nil || len(tasks) != 2 || !tasks[1].Ready || len(tasks[1].BlockedBy) != 0 {
		t.Fatalf("tasks = %#v, %v", tasks, err)
	}
	position, started := 0, TaskInProgress
	moved, err := store.UpdateTask(repositoryID, proposal.ID, second.ID, authorID, TaskPatch{Position: &position, Status: &started})
	if err != nil || moved.Position != 0 || moved.Status != TaskInProgress {
		t.Fatalf("moved = %#v, %v", moved, err)
	}
	history, err := store.ListTaskChanges(repositoryID, proposal.ID, first.ID)
	if err != nil || len(history) != 2 || history[0].ActorID != authorID || history[1].ActorID != commenterID || history[1].Action != "status_changed" || history[1].Task.Status != TaskCompleted {
		t.Fatalf("history = %#v, %v", history, err)
	}
	secondHistory, err := store.ListTaskChanges(repositoryID, proposal.ID, second.ID)
	if err != nil || len(secondHistory) != 2 || secondHistory[1].Action != "status_changed" || secondHistory[1].Task.Status != TaskInProgress || secondHistory[1].Task.Position != 0 {
		t.Fatalf("combined update history = %#v, %v", secondHistory, err)
	}
	reopened, _ := New(store.root)
	persisted, err := reopened.ListTasks(repositoryID, proposal.ID)
	if err != nil || len(persisted) != 2 || persisted[0].ID != second.ID {
		t.Fatalf("persisted = %#v, %v", persisted, err)
	}
}

func TestTaskContributionCannotReopenTerminalTaskOrClosedProposal(t *testing.T) {
	contribution := TaskContribution{PullRequestID: commenterID, SourceCommitID: strings.Repeat("a", 40), CommitIDs: []string{strings.Repeat("a", 40)}, Status: "review"}
	for _, status := range []string{TaskCompleted, TaskCancelled} {
		store, _ := New(t.TempDir())
		proposal, _ := store.Create(repositoryID, authorID, "Intent", "Context")
		task, _ := store.CreateTask(repositoryID, proposal.ID, authorID, "Work", "Outcome", nil, nil)
		if _, err := store.UpdateTask(repositoryID, proposal.ID, task.ID, authorID, TaskPatch{Status: &status}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.LinkTaskContribution(repositoryID, proposal.ID, task.ID, authorID, contribution); !errors.Is(err, ErrInvalid) {
			t.Fatalf("status %s link: %v", status, err)
		}
	}
	store, _ := New(t.TempDir())
	proposal, _ := store.Create(repositoryID, authorID, "Intent", "Context")
	task, _ := store.CreateTask(repositoryID, proposal.ID, authorID, "Work", "Outcome", nil, nil)
	closed := Closed
	if _, err := store.Update(repositoryID, proposal.ID, Patch{Status: &closed}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LinkTaskContribution(repositoryID, proposal.ID, task.ID, authorID, contribution); !errors.Is(err, ErrInvalid) {
		t.Fatalf("closed proposal link: %v", err)
	}
}

func TestProposalTasksRejectInvalidGraphAndDiscussionLinks(t *testing.T) {
	store, _ := New(t.TempDir())
	proposal, _ := store.Create(repositoryID, authorID, "Plan", "")
	first, _ := store.CreateTask(repositoryID, proposal.ID, authorID, "First", "First result", nil, nil)
	second, _ := store.CreateTask(repositoryID, proposal.ID, authorID, "Second", "Second result", []string{first.ID}, nil)
	deps := []string{second.ID}
	if _, err := store.UpdateTask(repositoryID, proposal.ID, first.ID, authorID, TaskPatch{DependencyIDs: &deps}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cycle error = %v", err)
	}
	unknown := []string{"22222222222222222222222222222222"}
	if _, err := store.UpdateTask(repositoryID, proposal.ID, first.ID, authorID, TaskPatch{DiscussionCommentIDs: &unknown}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("comment error = %v", err)
	}
}

func TestTaskAssignmentClaimsAreAtomicAndAttributable(t *testing.T) {
	root := t.TempDir()
	first, _ := New(root)
	second, _ := New(root)
	proposal, _ := first.Create(repositoryID, authorID, "Plan", "")
	task, _ := first.CreateTask(repositoryID, proposal.ID, authorID, "Build", "A bounded result", nil, nil)
	base := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	assigned, err := first.AssignTask(repositoryID, proposal.ID, task.ID, authorID, TaskAssignmentInput{AssigneeType: "human", AssigneeID: commenterID, Mandate: "Deliver the result", RepositoryID: repositoryID, BaseRevision: base})
	if err != nil || assigned.Assignment == nil || assigned.Assignment.AssigneeID != commenterID || len(assigned.Assignment.Access.Scopes) != 0 {
		t.Fatalf("assigned = %#v, %v", assigned, err)
	}
	if _, err := second.AssignTask(repositoryID, proposal.ID, task.ID, commenterID, TaskAssignmentInput{AssigneeType: "agent", Mandate: "Competing claim", RepositoryID: repositoryID, BaseRevision: base}); !errors.Is(err, ErrTaskAssignmentConflict) {
		t.Fatalf("concurrent claim = %v", err)
	}
	reassigned, err := second.AssignTask(repositoryID, proposal.ID, task.ID, commenterID, TaskAssignmentInput{AssigneeType: "agent", Mandate: "Delegated result", RepositoryID: repositoryID, BaseRevision: base, ExpectedAssignmentID: assigned.Assignment.ID})
	if err != nil || reassigned.Assignment == nil || reassigned.Assignment.AssigneeType != "agent" || len(reassigned.Assignment.Access.Scopes) != 2 {
		t.Fatalf("reassigned = %#v, %v", reassigned, err)
	}
	if _, err := first.RevokeTaskAssignment(repositoryID, proposal.ID, task.ID, authorID, assigned.Assignment.ID); !errors.Is(err, ErrTaskAssignmentConflict) {
		t.Fatalf("stale revoke = %v", err)
	}
	revoked, err := first.RevokeTaskAssignment(repositoryID, proposal.ID, task.ID, authorID, reassigned.Assignment.ID)
	if err != nil || revoked.Assignment != nil {
		t.Fatalf("revoked = %#v, %v", revoked, err)
	}
	history, _ := first.ListTaskChanges(repositoryID, proposal.ID, task.ID)
	if len(history) != 4 || history[1].Action != "assigned" || history[2].Action != "reassigned" || history[3].Action != "assignment_revoked" {
		t.Fatalf("history = %#v", history)
	}
}

func TestPlanRevisionObsoletesWorkAndRebaseCreatesFreshBoundary(t *testing.T) {
	store, _ := New(t.TempDir())
	proposal, _ := store.Create(repositoryID, authorID, "Parallel plan", "")
	first, _ := store.CreateTask(repositoryID, proposal.ID, authorID, "Foundation", "Initial outcome", nil, nil)
	second, _ := store.CreateTask(repositoryID, proposal.ID, authorID, "Dependent", "Uses foundation", []string{first.ID}, nil)
	assigned, err := store.AssignTask(repositoryID, proposal.ID, first.ID, authorID, TaskAssignmentInput{AssigneeType: "agent", Mandate: "Build foundation", RepositoryID: repositoryID, BaseRevision: strings.Repeat("a", 40)})
	if err != nil {
		t.Fatal(err)
	}
	unchangedTitle, unchangedOutcome := first.Title, first.Outcome
	unchangedDependencies, unchangedDiscussion := cloneStrings(first.DependencyIDs), cloneStrings(first.DiscussionCommentIDs)
	unchanged, err := store.UpdateTask(repositoryID, proposal.ID, first.ID, commenterID, TaskPatch{Title: &unchangedTitle, Outcome: &unchangedOutcome, DependencyIDs: &unchangedDependencies, DiscussionCommentIDs: &unchangedDiscussion})
	if err != nil || unchanged.ContextRevision != 1 || unchanged.ContextState != "current" {
		t.Fatalf("unchanged full edit = %#v, %v", unchanged, err)
	}
	if err := store.WithStartableAgentTask(repositoryID, proposal.ID, first.ID, assigned.Assignment.ID, func(Proposal, Task, []Task, []Comment) error { return nil }); err != nil {
		t.Fatalf("unchanged edit blocked start = %v", err)
	}
	revisedOutcome := "Revised outcome"
	revised, err := store.UpdateTask(repositoryID, proposal.ID, first.ID, commenterID, TaskPatch{Outcome: &revisedOutcome})
	if err != nil || revised.ContextRevision != 2 || revised.ContextState != "changed" {
		t.Fatalf("revised = %#v, %v", revised, err)
	}
	if err := store.WithStartableAgentTask(repositoryID, proposal.ID, first.ID, assigned.Assignment.ID, func(Proposal, Task, []Task, []Comment) error { return nil }); !errors.Is(err, ErrTaskAssignmentConflict) {
		t.Fatalf("stale start = %v", err)
	}
	rebased, err := store.RebaseTaskAssignment(repositoryID, proposal.ID, first.ID, commenterID, TaskRebaseInput{BaseRevision: strings.Repeat("b", 40), ExpectedAssignmentID: assigned.Assignment.ID})
	if err != nil || rebased.ContextState != "current" || rebased.Assignment.ID == assigned.Assignment.ID || rebased.Assignment.ContextRevision != 2 || rebased.Assignment.Access.BaseRevision != strings.Repeat("b", 40) {
		t.Fatalf("rebased = %#v, %v", rebased, err)
	}

	contribution := TaskContribution{PullRequestID: commenterID, SourceCommitID: strings.Repeat("c", 40), CommitIDs: []string{strings.Repeat("c", 40)}, Status: "review"}
	if _, err := store.LinkTaskContribution(repositoryID, proposal.ID, first.ID, authorID, contribution); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateTaskContribution(repositoryID, proposal.ID, first.ID, authorID, commenterID, "merged"); err != nil {
		t.Fatal(err)
	}
	tasks, _ := store.ListTasks(repositoryID, proposal.ID)
	if !tasks[1].Ready {
		t.Fatalf("dependent not ready after current merge: %#v", tasks[1])
	}
	revisedTitle := "Foundation v2"
	obsolete, err := store.UpdateTask(repositoryID, proposal.ID, first.ID, commenterID, TaskPatch{Title: &revisedTitle})
	if err != nil || obsolete.ContextState != "obsolete" {
		t.Fatalf("obsolete = %#v, %v", obsolete, err)
	}
	tasks, _ = store.ListTasks(repositoryID, proposal.ID)
	if tasks[1].Ready || len(tasks[1].BlockedBy) != 1 {
		t.Fatalf("dependent trusted obsolete result: %#v", tasks[1])
	}
	todo := TaskTodo
	reset, err := store.UpdateTask(repositoryID, proposal.ID, first.ID, authorID, TaskPatch{Status: &todo})
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := store.RebaseTaskAssignment(repositoryID, proposal.ID, first.ID, authorID, TaskRebaseInput{BaseRevision: strings.Repeat("d", 40), ExpectedAssignmentID: reset.Assignment.ID})
	if err != nil || replacement.ContextState != "obsolete" || replacement.Assignment.ContextRevision != replacement.ContextRevision {
		t.Fatalf("replacement boundary = %#v, %v", replacement, err)
	}
	if err := store.WithStartableAgentTask(repositoryID, proposal.ID, first.ID, replacement.Assignment.ID, func(Proposal, Task, []Task, []Comment) error { return nil }); err != nil {
		t.Fatalf("replacement start = %v", err)
	}
	history, _ := store.ListTaskChanges(repositoryID, proposal.ID, first.ID)
	foundPublished := false
	for _, change := range history {
		foundPublished = foundPublished || change.Action == "contribution_published"
	}
	if !foundPublished || history[len(history)-1].Task.ContextState != "obsolete" {
		t.Fatalf("history = %#v", history)
	}
	_ = second
}

func TestRebaseObsoleteReviewRestoresReplacementPublication(t *testing.T) {
	store, _ := New(t.TempDir())
	proposal, _ := store.Create(repositoryID, authorID, "Plan", "")
	task, _ := store.CreateTask(repositoryID, proposal.ID, authorID, "Build", "First result", nil, nil)
	assigned, _ := store.AssignTask(repositoryID, proposal.ID, task.ID, authorID, TaskAssignmentInput{AssigneeType: "human", AssigneeID: commenterID, Mandate: "Build", RepositoryID: repositoryID, BaseRevision: strings.Repeat("a", 40)})
	first := TaskContribution{PullRequestID: authorID, SourceCommitID: strings.Repeat("b", 40), CommitIDs: []string{strings.Repeat("b", 40)}, Status: "review"}
	if _, err := store.LinkTaskContribution(repositoryID, proposal.ID, task.ID, commenterID, first); err != nil {
		t.Fatal(err)
	}
	outcome := "Replacement result"
	obsolete, err := store.UpdateTask(repositoryID, proposal.ID, task.ID, authorID, TaskPatch{Outcome: &outcome})
	if err != nil || obsolete.Status != TaskInProgress || obsolete.ContextState != "obsolete" {
		t.Fatalf("obsolete review = %#v, %v", obsolete, err)
	}
	rebased, err := store.RebaseTaskAssignment(repositoryID, proposal.ID, task.ID, authorID, TaskRebaseInput{BaseRevision: strings.Repeat("c", 40), ExpectedAssignmentID: assigned.Assignment.ID})
	if err != nil || rebased.Status != TaskTodo || rebased.Assignment.ContextRevision != rebased.ContextRevision {
		t.Fatalf("rebased = %#v, %v", rebased, err)
	}
	replacement := TaskContribution{PullRequestID: commenterID, SourceCommitID: strings.Repeat("d", 40), CommitIDs: []string{strings.Repeat("d", 40)}, Status: "review"}
	linked, err := store.LinkTaskContribution(repositoryID, proposal.ID, task.ID, commenterID, replacement)
	if err != nil || linked.Contribution.PullRequestID != commenterID || linked.Contributions[0].Status != "superseded" {
		t.Fatalf("replacement = %#v, %v", linked, err)
	}
}

func TestClosedProposalRejectsAssignmentRevocation(t *testing.T) {
	store, _ := New(t.TempDir())
	proposal, _ := store.Create(repositoryID, authorID, "Plan", "")
	task, _ := store.CreateTask(repositoryID, proposal.ID, authorID, "Build", "A bounded result", nil, nil)
	assigned, err := store.AssignTask(repositoryID, proposal.ID, task.ID, authorID, TaskAssignmentInput{AssigneeType: "human", AssigneeID: commenterID, Mandate: "Deliver", RepositoryID: repositoryID, BaseRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	closed := Closed
	if _, err := store.Update(repositoryID, proposal.ID, Patch{Status: &closed}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RevokeTaskAssignment(repositoryID, proposal.ID, task.ID, authorID, assigned.Assignment.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("revoke closed proposal = %v", err)
	}
	persisted, _ := store.GetTask(repositoryID, proposal.ID, task.ID)
	history, _ := store.ListTaskChanges(repositoryID, proposal.ID, task.ID)
	if persisted.Assignment == nil || persisted.Assignment.ID != assigned.Assignment.ID || len(history) != 2 || history[1].Action != "assigned" {
		t.Fatalf("task = %#v, history = %#v", persisted, history)
	}
}

func TestTaskStartBoundaryExcludesAssignmentRevocation(t *testing.T) {
	root := t.TempDir()
	starter, _ := New(root)
	revoker, _ := New(root)
	repositoryID := "11111111111111111111111111111111"
	authorID := "22222222222222222222222222222222"
	proposal, _ := starter.Create(repositoryID, authorID, "Plan", "Shared intent")
	task, _ := starter.CreateTask(repositoryID, proposal.ID, authorID, "Build", "Working result", nil, nil)
	assigned, _ := starter.AssignTask(repositoryID, proposal.ID, task.ID, authorID, TaskAssignmentInput{AssigneeType: "agent", Mandate: "Build it", RepositoryID: repositoryID, BaseRevision: strings.Repeat("a", 40)})

	entered := make(chan struct{})
	release := make(chan struct{})
	startDone := make(chan error, 1)
	go func() {
		startDone <- starter.WithStartableAgentTask(repositoryID, proposal.ID, task.ID, assigned.Assignment.ID, func(Proposal, Task, []Task, []Comment) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	revokeDone := make(chan error, 1)
	go func() {
		_, err := revoker.RevokeTaskAssignment(repositoryID, proposal.ID, task.ID, authorID, assigned.Assignment.ID)
		revokeDone <- err
	}()
	select {
	case err := <-revokeDone:
		t.Fatalf("revocation crossed active start boundary: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-startDone; err != nil {
		t.Fatal(err)
	}
	if err := <-revokeDone; err != nil {
		t.Fatal(err)
	}
}

func TestGetPreservesCorruptRecordError(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	proposal, err := store.Create(repositoryID, authorID, "An idea", "Context")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, proposal.ID+".json"), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(repositoryID, proposal.ID); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("Get error = %v, want preserved corruption error", err)
	}
	if _, err := store.Get(repositoryID, "22222222222222222222222222222222"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing Get error = %v", err)
	}
}

func TestMutationsReconcilePostRenameDirectorySyncFailure(t *testing.T) {
	store, _ := New(t.TempDir())
	failReads := false
	store.directorySync = func(string) error {
		failReads = true
		return errors.New("injected directory sync failure")
	}
	store.readFile = func(path string) ([]byte, error) {
		if failReads {
			return nil, errors.New("injected verification read failure")
		}
		return os.ReadFile(path)
	}
	proposal, err := store.Create(repositoryID, authorID, "Durable idea", "Context")
	if !errors.Is(err, ErrDurabilityUncertain) || proposal.ID == "" {
		t.Fatalf("committed create result = %#v, %v", proposal, err)
	}
	failReads = false
	listed, err := store.List(repositoryID)
	if err != nil || len(listed) != 1 || listed[0].ID != proposal.ID {
		t.Fatalf("proposals after create = %#v, %v", listed, err)
	}
	comment, err := store.AddComment(repositoryID, proposal.ID, commenterID, "Feedback")
	if !errors.Is(err, ErrDurabilityUncertain) || comment.ID == "" {
		t.Fatalf("committed comment result = %#v, %v", comment, err)
	}
	failReads = false
	comments, err := store.ListComments(repositoryID, proposal.ID)
	if err != nil || len(comments) != 1 || comments[0].ID != comment.ID {
		t.Fatalf("comments after append = %#v, %v", comments, err)
	}
	updated, err := store.Update(repositoryID, proposal.ID, Patch{Status: pointer(Closed)})
	if !errors.Is(err, ErrDurabilityUncertain) || updated.Status != Closed {
		t.Fatalf("committed update = %#v, %v", updated, err)
	}
}

func pointer(value string) *string { return &value }
