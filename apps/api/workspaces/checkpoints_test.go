package workspaces

import (
	"encoding/base64"
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
