package workspaces

import (
	"encoding/base64"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCheckpointLineageAndPublicSnapshot(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: "0123456789abcdef0123456789abcdef", CommitID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatorID: "user"}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	content := base64.StdEncoding.EncodeToString([]byte("unfinished"))
	c, err := s.CreateCheckpoint(w.ID, "user", "", "first", "", Reproducibility{}, []CheckpointFile{{Path: "work.txt", Operation: "add", SHA256: "abc", Patch: "secret diff", ContentB64: content}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Files[0].ContentB64 != "" || c.Files[0].Patch != "" {
		t.Fatal("public checkpoint exposed snapshot content")
	}
	stored, err := s.CheckpointSnapshot(w.ID, c.ID)
	if err != nil || stored.Files[0].ContentB64 != content {
		t.Fatalf("stored snapshot = %#v, %v", stored, err)
	}
	if _, err = s.CreateCheckpoint(w.ID, "user", "", "stale", "", Reproducibility{}, nil); err != ErrCheckpointConflict {
		t.Fatalf("stale lineage err = %v", err)
	}
	second, err := s.CreateCheckpoint(w.ID, "user", c.ID, "second", "", Reproducibility{}, nil)
	if err != nil || second.ParentCheckpointID != c.ID {
		t.Fatalf("second = %#v, %v", second, err)
	}
	if _, err = s.RecordCheckpointRestore(w.ID, c.ID, second.ID, "peer"); err != nil {
		t.Fatal(err)
	}
	branched, err := s.CreateCheckpoint(w.ID, "peer", c.ID, "branch", "", Reproducibility{}, nil)
	if err != nil || branched.ParentCheckpointID != c.ID {
		t.Fatalf("branch = %#v, %v", branched, err)
	}
	if _, err = s.RecordCheckpointRestore(w.ID, c.ID, second.ID, "peer"); err != ErrCheckpointConflict {
		t.Fatalf("stale restore lineage err = %v", err)
	}
}

func TestCheckpointPublicationIsBidirectionalAndIdempotent(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: strings.Repeat("0", 32), CommitID: strings.Repeat("a", 40), CreatorID: "user"}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateCheckpoint(w.ID, "user", "", "review", "", Reproducibility{}, []CheckpointFile{{Path: "work.txt", Operation: "add", SHA256: "abc"}})
	if err != nil {
		t.Fatal(err)
	}
	publication := Publication{Branch: "workspace/review", CommitID: strings.Repeat("b", 40), PullRequestID: strings.Repeat("c", 32), ContributorIDs: []string{"user"}, CommandIDs: []string{"command"}, LinkPending: true, PublishedBy: "user", PublishedAt: time.Now().UTC()}
	got, err := s.RecordCheckpointPublication(w.ID, c.ID, publication)
	if err != nil || got.Publication == nil || got.Publication.PullRequestID != publication.PullRequestID {
		t.Fatalf("publication = %#v, %v", got.Publication, err)
	}
	if _, err = s.RecordCheckpointPublication(w.ID, c.ID, publication); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	confirmed, err := s.ConfirmCheckpointPublicationLink(w.ID, c.ID, publication.PullRequestID)
	if err != nil || confirmed.Publication.LinkPending {
		t.Fatalf("confirm = %#v, %v", confirmed.Publication, err)
	}
	publication.CommitID = strings.Repeat("d", 40)
	if _, err = s.RecordCheckpointPublication(w.ID, c.ID, publication); err != ErrCheckpointConflict {
		t.Fatalf("replacement = %v", err)
	}
	updated, err := s.Get(w.ID)
	if err != nil || updated.Events[len(updated.Events)-1].Kind != "checkpoint.published" {
		t.Fatalf("events = %#v, %v", updated.Events, err)
	}
}

func TestCheckpointFreezesContributorAndCommandEvidenceAtCapture(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: strings.Repeat("0", 32), CommitID: strings.Repeat("a", 40), CreatorID: "creator"}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordChange(w.ID, Change{Path: "work.txt", ActorID: "author"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordCommand(w.ID, CommandOutcome{CommandSHA256: "before", ActorID: "runner", ExitCode: 0}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 201; i++ {
		if _, err = s.RecordChange(w.ID, Change{Path: "unrelated.txt", ActorID: "later"}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 101; i++ {
		if _, err = s.RecordCommand(w.ID, CommandOutcome{CommandSHA256: "later", ActorID: "later", ExitCode: 0}); err != nil {
			t.Fatal(err)
		}
	}
	c, err := s.CaptureAndCreateCheckpoint(w.ID, "creator", "", "frozen", "", Reproducibility{}, func(Workspace) ([]CheckpointFile, error) {
		return []CheckpointFile{{Path: "work.txt", Operation: "add"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordCommand(w.ID, CommandOutcome{CommandSHA256: "after", ActorID: "later", ExitCode: 1}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.GetCheckpoint(w.ID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Commands) != 102 || stored.Commands[0].SHA256 != "before" || strings.Join(stored.ContributorIDs, ",") != "author,creator,later,runner" {
		t.Fatalf("evidence = %#v, %#v", stored.ContributorIDs, stored.Commands)
	}
}

func TestLegacyWorkspaceSeedsLedgerBeforeFirstNewEvidence(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: strings.Repeat("0", 32), CommitID: strings.Repeat("a", 40), CreatorID: "creator"}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordChange(w.ID, Change{Path: "work.txt", ActorID: "legacy-author"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordCommand(w.ID, CommandOutcome{CommandSHA256: "legacy-command", ActorID: "legacy-runner"}); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(s.provenancePath(w.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordChange(w.ID, Change{Path: "other.txt", ActorID: "new-author"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordCommand(w.ID, CommandOutcome{CommandSHA256: "new-command", ActorID: "new-runner"}); err != nil {
		t.Fatal(err)
	}
	c, err := s.CaptureAndCreateCheckpoint(w.ID, "creator", "", "migration", "", Reproducibility{}, func(Workspace) ([]CheckpointFile, error) {
		return []CheckpointFile{{Path: "work.txt", Operation: "modify"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Commands) != 2 || c.Commands[0].SHA256 != "legacy-command" || !slices.Contains(c.ContributorIDs, "legacy-author") {
		t.Fatalf("migrated evidence = %#v, %#v", c.ContributorIDs, c.Commands)
	}
}

func TestPublicationIntentSurvivesCheckpointRecordFailure(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	intent := PublicationIntent{WorkspaceID: strings.Repeat("1", 32), CheckpointID: strings.Repeat("2", 32), Publication: Publication{Branch: "work", CommitID: strings.Repeat("a", 40), PullRequestID: strings.Repeat("3", 32)}}
	if err = s.SavePublicationIntent(intent); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPublicationIntent(intent.WorkspaceID, intent.CheckpointID)
	if err != nil || got.Publication.PullRequestID != intent.Publication.PullRequestID {
		t.Fatalf("intent = %#v, %v", got, err)
	}
	if err = s.ClearPublicationIntent(intent.WorkspaceID, intent.CheckpointID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetPublicationIntent(intent.WorkspaceID, intent.CheckpointID); err != ErrNotFound {
		t.Fatalf("cleared intent = %v", err)
	}
}

func TestCheckpointPublicationClaimRejectsConcurrentSideEffects(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: strings.Repeat("0", 32), CommitID: strings.Repeat("a", 40), CreatorID: "user"}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateCheckpoint(w.ID, "user", "", "review", "", Reproducibility{}, []CheckpointFile{{Path: "work.txt", Operation: "add"}})
	if err != nil {
		t.Fatal(err)
	}
	_, release, err := s.ClaimCheckpointPublication(w.ID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, secondRelease, claimErr := s.ClaimCheckpointPublication(w.ID, c.ID)
		if secondRelease != nil {
			secondRelease()
		}
		result <- claimErr
	}()
	select {
	case claimErr := <-result:
		t.Fatalf("claim did not wait: %v", claimErr)
	case <-time.After(30 * time.Millisecond):
	}
	publication := Publication{Branch: "one", CommitID: strings.Repeat("b", 40), PublishedBy: "user", PublishedAt: time.Now().UTC()}
	if _, err = s.RecordCheckpointPublication(w.ID, c.ID, publication); err != nil {
		t.Fatal(err)
	}
	release()
	if err = <-result; err != ErrCheckpointConflict {
		t.Fatalf("second claim = %v", err)
	}
}

func TestCheckpointCreationCannotInterleaveWithRestoreLineage(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: "0123456789abcdef0123456789abcdef", CommitID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatorID: "user"}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	target, err := s.CreateCheckpoint(w.ID, "user", "", "target", "", Reproducibility{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	admitted, release := make(chan struct{}), make(chan struct{})
	restored := make(chan error, 1)
	go func() {
		restored <- s.WithControl(w.ID, "user", "files", func(current Workspace) error {
			close(admitted)
			<-release
			_, e := s.RecordCheckpointRestore(w.ID, target.ID, current.HeadCheckpointID, "user")
			return e
		})
	}()
	<-admitted
	created := make(chan error, 1)
	go func() {
		_, e := s.CreateCheckpoint(w.ID, "peer", target.ID, "intervening", "", Reproducibility{}, nil)
		created <- e
	}()
	select {
	case err := <-created:
		t.Fatalf("checkpoint interleaved with restore: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err = <-restored; err != nil {
		t.Fatal(err)
	}
	if err = <-created; err != nil {
		t.Fatal(err)
	}
	current, err := s.Get(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.HeadCheckpointID == target.ID {
		t.Fatal("intervening checkpoint did not publish after restore")
	}
}

func TestCheckpointCaptureWaitsForAdmittedFileMutation(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: "0123456789abcdef0123456789abcdef", CommitID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatorID: "user"}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	mutationAdmitted, releaseMutation := make(chan struct{}), make(chan struct{})
	mutation := make(chan error, 1)
	go func() {
		mutation <- s.WithControl(w.ID, "user", "files", func(Workspace) error { close(mutationAdmitted); <-releaseMutation; return nil })
	}()
	<-mutationAdmitted
	captureStarted := make(chan struct{})
	created := make(chan error, 1)
	go func() {
		_, e := s.CaptureAndCreateCheckpoint(w.ID, "peer", "", "after mutation", "", Reproducibility{}, func(Workspace) ([]CheckpointFile, error) { close(captureStarted); return nil, nil })
		created <- e
	}()
	select {
	case <-captureStarted:
		t.Fatal("checkpoint captured during admitted mutation")
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseMutation)
	if err = <-mutation; err != nil {
		t.Fatal(err)
	}
	select {
	case <-captureStarted:
	case <-time.After(time.Second):
		t.Fatal("checkpoint capture did not resume")
	}
	if err = <-created; err != nil {
		t.Fatal(err)
	}
}
