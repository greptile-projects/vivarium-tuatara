package protectionplans

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEncryptedCaptureProjectsProofWithoutProtectedContent(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.Create("repo", "owner", Plan{Name: "Primary source", CommitmentID: "commitment", CommitmentVersion: 2, Mode: "snapshot", Resources: []Resource{{TargetID: "source", Kind: "repository", Revision: strings.Repeat("a", 40)}}, Destination: "vault://eu-primary", Jurisdiction: "EU", RetentionDays: 30, FreshnessMinutes: 60, AccessorIDs: []string{"owner"}, ValidationChecks: []string{"decrypt", "manifest checksum"}})
	if err != nil {
		t.Fatal(err)
	}
	protected := []byte("database-password=must-never-project")
	got, err := store.Capture(plan.ID, "owner", 1, Source{Revision: "source-version", Entries: []Entry{{Path: "secret/config", Kind: "blob", Version: "object", SHA256: strings.Repeat("b", 64), Size: int64(len(protected))}}, Payload: protected})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Captures) != 1 || !got.Captures[0].Recoverable {
		t.Fatalf("capture = %#v", got.Captures)
	}
	projected, _ := json.Marshal(got)
	if strings.Contains(string(projected), "secret/config") || strings.Contains(string(projected), string(protected)) {
		t.Fatalf("protected data leaked: %s", projected)
	}
	onDisk, err := os.ReadFile(filepath.Join(root, plan.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(onDisk), string(protected)) {
		t.Fatal("plaintext persisted")
	}
}

func TestCorruptCaptureCannotRemainRecoverable(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	plan, _ := store.Create("repo", "owner", Plan{Name: "Primary", CommitmentID: "commitment", CommitmentVersion: 1, Mode: "replica", Resources: []Resource{{TargetID: "source", Kind: "repository", Revision: strings.Repeat("a", 40)}}, Destination: "vault://primary", Jurisdiction: "EU", RetentionDays: 1, FreshnessMinutes: 5, AccessorIDs: []string{"owner"}, ValidationChecks: []string{"decrypt"}})
	_, err := store.Capture(plan.ID, "owner", 1, Source{Revision: "v1", Entries: []Entry{{Path: "README", Kind: "blob", Version: "one", SHA256: strings.Repeat("a", 64), Size: 4}}, Payload: []byte("safe")})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, plan.ID+".json"))
	var persisted Plan
	_ = json.Unmarshal(raw, &persisted)
	persisted.Captures[0].Ciphertext = "00"
	raw, _ = json.Marshal(persisted)
	if err = os.WriteFile(filepath.Join(root, plan.ID+".json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Captures[0].Recoverable || got.Captures[0].Failure != "corrupt_snapshot" {
		t.Fatalf("corrupt capture = %#v", got.Captures[0])
	}
}

func TestKeyLossCannotRemainRecoverable(t *testing.T) {
	store, _ := New(t.TempDir())
	plan, _ := store.Create("repo", "owner", Plan{Name: "Primary", CommitmentID: "commitment", CommitmentVersion: 1, Mode: "snapshot", Resources: []Resource{{TargetID: "source", Kind: "repository", Revision: strings.Repeat("a", 40)}}, Destination: "vault://primary", Jurisdiction: "EU", RetentionDays: 1, FreshnessMinutes: 5, AccessorIDs: []string{"owner"}, ValidationChecks: []string{"decrypt"}})
	_, _ = store.Capture(plan.ID, "owner", 1, Source{Revision: "v1", Entries: []Entry{{Path: "README", Kind: "blob", Version: "one", SHA256: strings.Repeat("a", 64), Size: 4}}, Payload: []byte("safe")})
	store.key = nil
	got, err := store.Get(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Captures[0].Recoverable || got.Captures[0].Failure != "encryption_key_unavailable" {
		t.Fatalf("keyless capture = %#v", got.Captures[0])
	}
}

func TestRevisionDoesNotRewriteHistoricalCapturePolicy(t *testing.T) {
	store, _ := New(t.TempDir())
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	firstResource := Resource{TargetID: "source", Kind: "repository", Revision: strings.Repeat("a", 40)}
	plan, err := store.Create("repo", "owner", Plan{Name: "Primary", CommitmentID: "commitment", CommitmentVersion: 1, Mode: "snapshot", Resources: []Resource{firstResource}, Destination: "vault://primary", Jurisdiction: "EU", RetentionDays: 1, FreshnessMinutes: 10, AccessorIDs: []string{"owner"}, ValidationChecks: []string{"decrypt"}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = store.Capture(plan.ID, "owner", 1, Source{Revision: "v1", Entries: []Entry{{Path: "README", Kind: "blob", Version: "one", SHA256: strings.Repeat("a", 64), Size: 4}}, Payload: []byte("safe")})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Minute)
	secondResource := Resource{TargetID: "source", Kind: "repository", Revision: strings.Repeat("b", 40)}
	plan, err = store.Revise(plan.ID, 1, "owner", Plan{Name: "Primary", CommitmentID: "commitment", CommitmentVersion: 1, Mode: "snapshot", Resources: []Resource{secondResource}, Destination: "vault://primary", Jurisdiction: "EU", RetentionDays: 1, FreshnessMinutes: 1, AccessorIDs: []string{"owner"}, ValidationChecks: []string{"decrypt"}})
	if err != nil {
		t.Fatal(err)
	}
	capture := plan.Captures[0]
	if capture.Freshness != "fresh" || capture.FreshnessMinutes != 10 || len(capture.Resources) != 1 || capture.Resources[0].Revision != firstResource.Revision {
		t.Fatalf("historical capture changed under successor: %#v", capture)
	}
}
