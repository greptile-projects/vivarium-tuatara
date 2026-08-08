package pullrequests

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type queueRequirements struct {
	policy repositories.IntegrationQueuePolicy
}

func (q queueRequirements) RequiredChecks(string, string) ([]string, error) {
	return append([]string(nil), q.policy.RequiredChecks...), nil
}
func (q queueRequirements) LockRequiredChecks() (func(), error) { return func() {}, nil }
func (q queueRequirements) IntegrationQueuePolicy(string, string) (repositories.IntegrationQueuePolicy, error) {
	return q.policy, nil
}

func TestCreateSnapshotsBranchesAndListsByRepository(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('1'))
	tree, err := repository.WriteObject(storage.TreeObject, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := writeCommit(t, repository, tree, "base")
	head := writeCommit(t, repository, tree, "head")
	if err := repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateReference(storage.Reference{Name: "refs/heads/topic", Target: string(head)}); err != nil {
		t.Fatal(err)
	}
	store, _ := New(t.TempDir(), gitStore)
	store.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	proposalID := testID('3')
	pullRequest, err := store.Create(repository.ID(), testID('2'), " Ship it ", " Why this matters. ", "topic", "main", &proposalID)
	if err != nil {
		t.Fatal(err)
	}
	if pullRequest.SourceCommitID != string(head) || pullRequest.TargetCommitID != string(base) || pullRequest.Title != "Ship it" || pullRequest.Status != Open {
		t.Fatalf("pull request = %#v", pullRequest)
	}
	got, err := store.Get(repository.ID(), pullRequest.ID)
	if err != nil || got.ID != pullRequest.ID {
		t.Fatalf("Get = %#v, %v", got, err)
	}
	listed, err := store.List(repository.ID())
	if err != nil || len(listed) != 1 || listed[0].ID != pullRequest.ID {
		t.Fatalf("List = %#v, %v", listed, err)
	}
	if _, err := store.Get(repository.ID(), testID('9')); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing Get error = %v", err)
	}
	if err := os.WriteFile(store.path(repository.ID(), pullRequest.ID), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(repository.ID(), pullRequest.ID); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("corrupt Get error = %v, want preserved storage failure", err)
	}
}

func TestWithSourceRevisionSerializesSynchronization(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('1'))
	tree, _ := repository.WriteObject(storage.TreeObject, nil)
	base := writeCommit(t, repository, tree, "base")
	head := writeCommit(t, repository, tree, "head")
	if err := repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateReference(storage.Reference{Name: "refs/heads/topic", Target: string(head)}); err != nil {
		t.Fatal(err)
	}
	store, _ := New(t.TempDir(), gitStore)
	pull, err := store.Create(repository.ID(), testID('2'), "Repair", "", "topic", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	next := writeCommit(t, repository, tree, "next")
	if err := repository.UpdateReference(storage.Reference{Name: "refs/heads/topic", Target: string(next)}); err != nil {
		t.Fatal(err)
	}

	entered, release, protectedDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		protectedDone <- store.WithSourceRevision(repository.ID(), pull.ID, string(head), func(current PullRequest) error {
			if current.SourceCommitID != string(head) {
				t.Errorf("protected revision = %s", current.SourceCommitID)
			}
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	synchronized := make(chan PullRequest, 1)
	go func() {
		updated, syncErr := store.SynchronizeSource(repository.ID(), pull.ID)
		if syncErr != nil {
			t.Errorf("SynchronizeSource: %v", syncErr)
		}
		synchronized <- updated
	}()
	select {
	case <-synchronized:
		t.Fatal("source synchronization escaped the protected revision boundary")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-protectedDone; err != nil {
		t.Fatal(err)
	}
	if updated := <-synchronized; updated.SourceCommitID != string(next) {
		t.Fatalf("synchronized revision = %s, want %s", updated.SourceCommitID, next)
	}
}

func TestCreateRejectsMissingAndNonCommitBranches(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('4'))
	blob, _ := repository.WriteObject(storage.BlobObject, []byte("not a commit"))
	if err := repository.CreateReference(storage.Reference{Name: "refs/heads/blob", Target: string(blob)}); err != nil {
		t.Fatal(err)
	}
	store, _ := New(t.TempDir(), gitStore)
	_, err := store.Create(repository.ID(), testID('5'), "Change", "", "missing", "blob", nil)
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("Create error = %v", err)
	}
	_, err = store.Create(repository.ID(), testID('5'), "Change", "", "same", "same", nil)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("same-branch error = %v", err)
	}
	if err := os.Remove(filepath.Join(repository.Path(), "objects", string(blob)[:2], string(blob)[2:])); err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(repository.ID(), testID('5'), "Change", "", "blob", "missing", nil)
	if err == nil || errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("corrupt branch error = %v, want preserved storage failure", err)
	}
}

func TestListIsolatesRepositoryRecordCorruption(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	first, _ := gitStore.Create(testID('8'))
	second, _ := gitStore.Create(testID('9'))
	for _, repository := range []*storage.Repository{first, second} {
		tree, _ := repository.WriteObject(storage.TreeObject, nil)
		base := writeCommit(t, repository, tree, "base")
		head := writeCommit(t, repository, tree, "head")
		if err := repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
			t.Fatal(err)
		}
		if err := repository.CreateReference(storage.Reference{Name: "refs/heads/topic", Target: string(head)}); err != nil {
			t.Fatal(err)
		}
	}
	store, _ := New(t.TempDir(), gitStore)
	firstPull, err := store.Create(first.ID(), testID('2'), "First", "", "topic", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	secondPull, err := store.Create(second.ID(), testID('2'), "Second", "", "topic", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path(first.ID(), firstPull.ID), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	listed, err := store.List(second.ID())
	if err != nil || len(listed) != 1 || listed[0].ID != secondPull.ID {
		t.Fatalf("healthy repository List = %#v, %v", listed, err)
	}
	if _, err := store.List(first.ID()); err == nil {
		t.Fatal("corrupt repository List succeeded")
	}
}

func TestCreateReturnsIdentityAfterUncertainDurability(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('6'))
	tree, _ := repository.WriteObject(storage.TreeObject, nil)
	base := writeCommit(t, repository, tree, "base")
	head := writeCommit(t, repository, tree, "head")
	_ = repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)})
	_ = repository.CreateReference(storage.Reference{Name: "refs/heads/topic", Target: string(head)})
	store, _ := New(t.TempDir(), gitStore)
	store.directorySync = func(string) error { return errors.New("injected sync failure") }
	pullRequest, err := store.Create(repository.ID(), testID('7'), "Change", "Purpose", "topic", "main", nil)
	if !errors.Is(err, ErrDurabilityUncertain) || pullRequest.ID == "" {
		t.Fatalf("Create = %#v, %v", pullRequest, err)
	}
	persisted, getErr := store.Get(repository.ID(), pullRequest.ID)
	if getErr != nil || persisted.ID != pullRequest.ID {
		t.Fatalf("persisted = %#v, %v", persisted, getErr)
	}
	comment, commentErr := store.AddComment(repository.ID(), pullRequest.ID, testID('7'), "Review note")
	if !errors.Is(commentErr, ErrDurabilityUncertain) || comment.ID == "" {
		t.Fatalf("AddComment = %#v, %v", comment, commentErr)
	}
	comments, listErr := store.ListComments(repository.ID(), pullRequest.ID)
	if listErr != nil || len(comments) != 1 || comments[0].ID != comment.ID {
		t.Fatalf("ListComments = %#v, %v", comments, listErr)
	}
	review, reviewErr := store.SetReview(repository.ID(), pullRequest.ID, testID('7'), Approved)
	if !errors.Is(reviewErr, ErrDurabilityUncertain) || review.ID == "" {
		t.Fatalf("SetReview = %#v, %v", review, reviewErr)
	}
	reviews, listReviewsErr := store.ListReviews(repository.ID(), pullRequest.ID)
	if listReviewsErr != nil || len(reviews) != 1 || reviews[0].ID != review.ID {
		t.Fatalf("ListReviews = %#v, %v", reviews, listReviewsErr)
	}
	withdrawn, withdrawErr := store.WithdrawReview(repository.ID(), pullRequest.ID, review.ID, testID('7'))
	if !errors.Is(withdrawErr, ErrDurabilityUncertain) || withdrawn.Decision != Withdrawn {
		t.Fatalf("WithdrawReview = %#v, %v", withdrawn, withdrawErr)
	}
}

func TestSynchronizeReturnsPersistedRevisionAfterUncertainDurability(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('6'))
	tree, _ := repository.WriteObject(storage.TreeObject, nil)
	base := writeCommit(t, repository, tree, "base")
	head := writeCommitWithParents(t, repository, tree, []storage.ObjectID{base}, "head")
	_ = repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)})
	_ = repository.CreateReference(storage.Reference{Name: "refs/heads/topic", Target: string(head)})
	store, _ := New(t.TempDir(), gitStore)
	pullRequest, err := store.Create(repository.ID(), testID('7'), "Change", "Purpose", "topic", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	revised := writeCommitWithParents(t, repository, tree, []storage.ObjectID{head}, "revised")
	_ = repository.UpdateReference(storage.Reference{Name: "refs/heads/topic", Target: string(revised)})
	store.directorySync = func(string) error { return errors.New("injected sync failure") }
	synchronized, err := store.SynchronizeSource(repository.ID(), pullRequest.ID)
	if !errors.Is(err, ErrDurabilityUncertain) || synchronized.SourceCommitID != string(revised) {
		t.Fatalf("SynchronizeSource = %#v, %v", synchronized, err)
	}
	persisted, getErr := store.Get(repository.ID(), pullRequest.ID)
	if getErr != nil || persisted.SourceCommitID != string(revised) {
		t.Fatalf("persisted synchronization = %#v, %v", persisted, getErr)
	}
}

func TestDeletedForkRetainsReviewableAndMergeableSnapshot(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	target, _ := gitStore.Create(testID('c'))
	source, _ := gitStore.Create(testID('d'))
	baseTree, err := target.WriteObject(storage.TreeObject, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := writeCommit(t, target, baseTree, "base")
	if err := target.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	if err := source.ImportCommit(target, base); err != nil {
		t.Fatal(err)
	}
	feature := writeCommitWithParents(t, source, baseTree, []storage.ObjectID{base}, "feature")
	if err := source.CreateReference(storage.Reference{Name: "refs/heads/topic", Target: string(feature)}); err != nil {
		t.Fatal(err)
	}
	store, _ := New(t.TempDir(), gitStore)
	pull, err := store.CreateFrom(target.ID(), source.ID(), testID('a'), "Outside", "Preserved snapshot", "topic", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := gitStore.Delete(source.ID()); err != nil {
		t.Fatal(err)
	}
	review, err := store.SetReview(target.ID(), pull.ID, testID('b'), Approved)
	if err != nil || review.ReviewedCommitID != string(feature) {
		t.Fatalf("SetReview = %#v, %v", review, err)
	}
	report, err := store.Readiness(target.ID(), pull.ID, true)
	if err != nil || !report.CanMerge || report.Source.State != "unavailable" || report.Source.CurrentCommitID == nil || *report.Source.CurrentCommitID != string(feature) {
		t.Fatalf("Readiness = %#v, %v", report, err)
	}
	merged, err := store.Merge(target.ID(), pull.ID, testID('f'))
	if err != nil || merged.Status != Merged || merged.MergeCommitID == nil {
		t.Fatalf("Merge = %#v, %v", merged, err)
	}
}

func TestSynchronizeSourceAfterRejectsMergeIntentBeforeCallback(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('6'))
	tree, _ := repository.WriteObject(storage.TreeObject, nil)
	base := writeCommit(t, repository, tree, "base")
	head := writeCommitWithParents(t, repository, tree, []storage.ObjectID{base}, "head")
	_ = repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)})
	_ = repository.CreateReference(storage.Reference{Name: "refs/heads/topic", Target: string(head)})
	store, _ := New(t.TempDir(), gitStore)
	pullRequest, err := store.Create(repository.ID(), testID('7'), "Change", "Purpose", "topic", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	revised := writeCommitWithParents(t, repository, tree, []storage.ObjectID{head}, "revised")
	_ = repository.UpdateReference(storage.Reference{Name: "refs/heads/topic", Target: string(revised)})
	pullRequest.mergeIntent = &mergeIntent{CommitID: string(head), MergerID: testID('8'), MergedAt: time.Now().UTC()}
	if _, err := store.write(pullRequest); err != nil {
		t.Fatal(err)
	}
	called := false
	_, err = store.SynchronizeSourceAfter(repository.ID(), pullRequest.ID, func() error { called = true; return nil })
	if !errors.Is(err, ErrNotReady) || called {
		t.Fatalf("SynchronizeSourceAfter error = %v, callback called = %v", err, called)
	}
	persisted, err := store.Get(repository.ID(), pullRequest.ID)
	if err != nil || persisted.SourceCommitID != string(head) {
		t.Fatalf("persisted pull request = %+v, %v", persisted, err)
	}
}

func TestReviewsCaptureCurrentCommitBecomeStaleAndRemainAttributable(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('a'))
	tree, _ := repository.WriteObject(storage.TreeObject, nil)
	base := writeCommit(t, repository, tree, "base")
	head := writeCommitWithParents(t, repository, tree, []storage.ObjectID{base}, "head")
	_ = repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)})
	_ = repository.CreateReference(storage.Reference{Name: "refs/heads/topic", Target: string(head)})
	store, _ := New(t.TempDir(), gitStore)
	now := time.Unix(1700000000, 0).UTC()
	store.now = func() time.Time { return now }
	pullRequest, err := store.Create(repository.ID(), testID('b'), "Change", "", "topic", "main", nil)
	if err != nil {
		t.Fatal(err)
	}

	review, err := store.SetReview(repository.ID(), pullRequest.ID, testID('c'), Approved)
	if err != nil || review.Decision != Approved || review.ReviewedCommitID != string(head) || review.Stale {
		t.Fatalf("SetReview = %#v, %v", review, err)
	}
	advanced := writeCommitWithParents(t, repository, tree, []storage.ObjectID{head}, "advanced")
	if err := repository.UpdateReference(storage.Reference{Name: "refs/heads/topic", Target: string(advanced)}); err != nil {
		t.Fatal(err)
	}
	reviews, err := store.ListReviews(repository.ID(), pullRequest.ID)
	if err != nil || len(reviews) != 1 || !reviews[0].Stale || reviews[0].ReviewedCommitID != string(head) {
		t.Fatalf("stale reviews = %#v, %v", reviews, err)
	}

	now = now.Add(time.Minute)
	if _, err := store.SetReview(repository.ID(), pullRequest.ID, testID('c'), ChangesRequested); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("review of unadopted revision error = %v", err)
	}
	synchronized, err := store.SynchronizeSource(repository.ID(), pullRequest.ID)
	if err != nil || synchronized.SourceCommitID != string(advanced) {
		t.Fatalf("SynchronizeSource = %#v, %v", synchronized, err)
	}
	reviews, err = store.ListReviews(repository.ID(), pullRequest.ID)
	if err != nil || len(reviews) != 1 || !reviews[0].Stale {
		t.Fatalf("review became fresh through synchronization = %#v, %v", reviews, err)
	}
	replaced, err := store.SetReview(repository.ID(), pullRequest.ID, testID('c'), ChangesRequested)
	if err != nil || replaced.ID != review.ID || replaced.Decision != ChangesRequested || replaced.ReviewedCommitID != string(advanced) || !replaced.CreatedAt.Equal(review.CreatedAt) || !replaced.UpdatedAt.After(review.UpdatedAt) {
		t.Fatalf("replacement = %#v, %v", replaced, err)
	}
	now = now.Add(time.Minute)
	withdrawn, err := store.WithdrawReview(repository.ID(), pullRequest.ID, review.ID, testID('c'))
	if err != nil || withdrawn.Decision != Withdrawn || withdrawn.ReviewedCommitID != string(advanced) {
		t.Fatalf("withdrawal = %#v, %v", withdrawn, err)
	}
	if _, err := store.WithdrawReview(repository.ID(), pullRequest.ID, review.ID, testID('d')); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other reviewer withdrawal error = %v", err)
	}
	reviews, err = store.ListReviews(repository.ID(), pullRequest.ID)
	if err != nil || len(reviews) != 1 || reviews[0].Decision != Withdrawn || reviews[0].Stale {
		t.Fatalf("withdrawn reviews = %#v, %v", reviews, err)
	}
	if err := repository.DeleteReference("refs/heads/topic"); err != nil {
		t.Fatal(err)
	}
	reviews, err = store.ListReviews(repository.ID(), pullRequest.ID)
	if err != nil || len(reviews) != 1 || !reviews[0].Stale || reviews[0].ReviewedCommitID != string(advanced) {
		t.Fatalf("reviews after source deletion = %#v, %v", reviews, err)
	}
}

func TestMergeReconcilesAttributedCommitAfterLaterTargetAdvance(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('e'))
	tree, _ := repository.WriteObject(storage.TreeObject, nil)
	base := writeCommit(t, repository, tree, "base")
	source := writeCommitWithParents(t, repository, tree, []storage.ObjectID{base}, "source")
	repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)})
	repository.CreateReference(storage.Reference{Name: "refs/heads/topic", Target: string(source)})
	store, _ := New(t.TempDir(), gitStore)
	pull, err := store.Create(repository.ID(), testID('a'), "Accepted change", "Shared reason", "topic", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	merger := testID('b')
	mergeContent := fmt.Sprintf("tree %s\nparent %s\nparent %s\nauthor Vivarium Author <%s@users.vivarium> 1700000000 +0000\ncommitter Vivarium Maintainer <%s@users.vivarium> 1700000010 +0000\n\nAccepted change\n\nShared reason\n\nPull-Request: %s\nAuthored-by: %s\nMerged-by: %s\n", tree, base, source, pull.AuthorID, merger, pull.ID, pull.AuthorID, merger)
	mergeCommit, _ := repository.WriteObject(storage.CommitObject, []byte(mergeContent))
	descendant := writeCommitWithParents(t, repository, tree, []storage.ObjectID{mergeCommit}, "later target work")
	repository.UpdateReference(storage.Reference{Name: "refs/heads/main", Target: string(descendant)})

	if _, err := store.Merge(repository.ID(), pull.ID, merger); !errors.Is(err, ErrNotReady) {
		t.Fatalf("forged attributed commit was reconciled: %v", err)
	}
	pull.mergeIntent = &mergeIntent{CommitID: string(mergeCommit), MergerID: merger, MergedAt: time.Unix(1700000010, 0).UTC()}
	if _, err := store.write(pull); err != nil {
		t.Fatal(err)
	}
	reconciled, err := store.Merge(repository.ID(), pull.ID, merger)
	if err != nil || reconciled.Status != Merged || reconciled.MergeCommitID == nil || *reconciled.MergeCommitID != string(mergeCommit) || reconciled.MergedBy == nil || *reconciled.MergedBy != merger || reconciled.MergedAt == nil || reconciled.MergedAt.Unix() != 1700000010 {
		t.Fatalf("reconciled = %#v, %v", reconciled, err)
	}
	main, _ := repository.ReadReference("refs/heads/main")
	if main.Target != string(descendant) {
		t.Fatalf("reconciliation moved target to %s", main.Target)
	}
}

func TestIntegrationQueueRebuildsConcurrentCandidateBeforeLanding(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('1'))
	baseTree, _ := repository.WriteObject(storage.TreeObject, nil)
	base := writeCommit(t, repository, baseTree, "base")
	oneBlob, _ := repository.WriteObject(storage.BlobObject, []byte("one\n"))
	oneTree, _ := repository.WriteObject(storage.TreeObject, testTree("one.txt", oneBlob))
	twoBlob, _ := repository.WriteObject(storage.BlobObject, []byte("two\n"))
	twoTree, _ := repository.WriteObject(storage.TreeObject, testTree("two.txt", twoBlob))
	one := writeCommitWithParents(t, repository, oneTree, []storage.ObjectID{base}, "one")
	two := writeCommitWithParents(t, repository, twoTree, []storage.ObjectID{base}, "two")
	repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)})
	repository.CreateReference(storage.Reference{Name: "refs/heads/one", Target: string(one)})
	repository.CreateReference(storage.Reference{Name: "refs/heads/two", Target: string(two)})
	store, _ := New(t.TempDir(), gitStore)
	store.ConfigureRequiredChecks(queueRequirements{repositories.IntegrationQueuePolicy{Branch: "main", Enabled: true, Concurrency: 2, FailureBehavior: repositories.QueueFailurePause, RequiredChecks: []string{}}}, nil)
	finalized := []string{}
	store.ConfigureQueueFinalizer(func(p PullRequest) error {
		finalized = append(finalized, p.ID)
		return nil
	})
	first, _ := store.Create(repository.ID(), testID('a'), "First", "", "one", "main", nil)
	second, _ := store.Create(repository.ID(), testID('b'), "Second", "", "two", "main", nil)
	actor := testID('c')
	firstTime, secondTime := time.Unix(1700000000, 0).UTC(), time.Unix(1700000001, 0).UTC()
	firstCandidate, err := store.newIntegrationCandidate(repository, first, string(base), []string{})
	if err != nil {
		t.Fatal(err)
	}
	secondCandidate, err := store.newIntegrationCandidate(repository, second, string(base), []string{})
	if err != nil {
		t.Fatal(err)
	}
	first.QueuedAt, first.QueuedBy, first.IntegrationCandidates = &firstTime, &actor, []IntegrationCandidate{firstCandidate}
	second.QueuedAt, second.QueuedBy, second.IntegrationCandidates = &secondTime, &actor, []IntegrationCandidate{secondCandidate}
	if _, err := store.write(first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.write(second); err != nil {
		t.Fatal(err)
	}
	corruptRepository := filepath.Join(store.root, testID('0'))
	if err := os.MkdirAll(corruptRepository, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptRepository, testID('d')+".json"), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.AdvanceIntegrationQueues(); err == nil {
		t.Fatal("queue scan did not report isolated corrupt repository")
	}
	first, _ = store.Get(repository.ID(), first.ID)
	second, _ = store.Get(repository.ID(), second.ID)
	if first.Status != Merged || second.Status != Merged || first.MergeCommitID == nil || second.MergeCommitID == nil {
		t.Fatalf("queue results: first=%#v second=%#v", first, second)
	}
	if len(finalized) != 2 || finalized[0] != first.ID || finalized[1] != second.ID {
		t.Fatalf("finalized queue pulls = %v", finalized)
	}
	if len(second.IntegrationCandidates) != 2 || second.IntegrationCandidates[0].SupersededAt == nil || second.IntegrationCandidates[0].SupersededReason != "target_changed" {
		t.Fatalf("second candidate history = %#v", second.IntegrationCandidates)
	}
	landed, err := repository.ReadCommit(storage.ObjectID(*second.MergeCommitID))
	if err != nil || len(landed.Parents) != 2 || string(landed.Parents[0]) != *first.MergeCommitID || string(landed.Parents[1]) != second.SourceCommitID {
		t.Fatalf("second landed commit = %#v, %v", landed, err)
	}
	main, _ := repository.ReadReference("refs/heads/main")
	if main.Target != *second.MergeCommitID {
		t.Fatalf("main = %s", main.Target)
	}
	first.QueueFinalizationPending, first.QueueFinalizedAt = true, nil
	if _, err := store.write(first); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	store.ConfigureQueueFinalizer(func(PullRequest) error {
		attempts++
		if attempts == 1 {
			return errors.New("activity unavailable")
		}
		return nil
	})
	_ = store.AdvanceIntegrationQueues()
	first, _ = store.Get(repository.ID(), first.ID)
	if !first.QueueFinalizationPending || first.QueueFinalizedAt != nil {
		t.Fatalf("failed finalization was acknowledged: %#v", first)
	}
	_ = store.AdvanceIntegrationQueues()
	first, _ = store.Get(repository.ID(), first.ID)
	if first.QueueFinalizationPending || first.QueueFinalizedAt == nil || attempts != 2 {
		t.Fatalf("finalization did not recover: pull=%#v attempts=%d", first, attempts)
	}
}

func TestCandidateLaunchRetriesAfterCheckStoreRecovers(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('1'))
	tree, _ := repository.WriteObject(storage.TreeObject, nil)
	commit := writeCommit(t, repository, tree, "candidate")
	checkRoot := t.TempDir()
	checkStore, _ := checkruns.New(checkRoot)
	store, _ := New(t.TempDir(), gitStore)
	store.checkRuns = checkStore
	pull := PullRequest{ID: testID('2'), RepositoryID: repository.ID()}
	candidate := IntegrationCandidate{CommitID: string(commit), CheckDefinitions: []checkruns.Definition{{Name: "quality", Image: "alpine:3.22", Command: "true", WorkingDirectory: ".", TimeoutSeconds: 1}}}
	if err := os.Chmod(checkRoot, 0); err != nil {
		t.Fatal(err)
	}
	store.launchCandidate(pull, candidate)
	if err := os.Chmod(checkRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store.launchCandidate(pull, candidate)
	deadline := time.Now().Add(time.Second)
	for {
		runs, err := checkStore.List(repository.ID(), pull.ID)
		if err == nil && len(runs) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("candidate checks were not created after recovery: %#v, %v", runs, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func writeCommit(t *testing.T, repository *storage.Repository, tree storage.ObjectID, message string) storage.ObjectID {
	return writeCommitWithParents(t, repository, tree, nil, message)
}

func testTree(name string, id storage.ObjectID) []byte {
	raw, _ := hex.DecodeString(string(id))
	return append(append([]byte("100644 "+name+"\x00"), raw...), []byte{}...)
}

func writeCommitWithParents(t *testing.T, repository *storage.Repository, tree storage.ObjectID, parents []storage.ObjectID, message string) storage.ObjectID {
	t.Helper()
	content := fmt.Sprintf("tree %s\n", tree)
	for _, parent := range parents {
		content += fmt.Sprintf("parent %s\n", parent)
	}
	content += fmt.Sprintf("author Test <test@example.com> 1700000000 +0000\ncommitter Test <test@example.com> 1700000000 +0000\n\n%s\n", message)
	id, err := repository.WriteObject(storage.CommitObject, []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testID(character byte) string {
	result := make([]byte, 32)
	for i := range result {
		result[i] = character
	}
	return string(result)
}
