package workspaces

import (
	"encoding/base64"
	"testing"
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
	c, err := s.CreateCheckpoint(w.ID, "user", "", "first", "", Reproducibility{}, []CheckpointFile{{Path: "work.txt", Operation: "add", SHA256: "abc", ContentB64: content}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Files[0].ContentB64 != "" {
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
	if _, err = s.RecordCheckpointRestore(w.ID, c.ID, "peer"); err != nil {
		t.Fatal(err)
	}
	branched, err := s.CreateCheckpoint(w.ID, "peer", c.ID, "branch", "", Reproducibility{}, nil)
	if err != nil || branched.ParentCheckpointID != c.ID {
		t.Fatalf("branch = %#v, %v", branched, err)
	}
}
