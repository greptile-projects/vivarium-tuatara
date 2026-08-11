package previews

import (
	"strings"
	"testing"
	"time"
)

func TestDefinitionAndStaleProjection(t *testing.T) {
	definition := []byte(`{"version":1,"image":"alpine:3.22","build":"mkdir -p dist && printf ok > dist/index.html","output_path":"dist","resources":{"cpus":1,"memory_mb":256,"storage_mb":64,"timeout_seconds":30},"access":{"network":"none","data":"preview_artifacts","identity":"named_users","actions":["view","test","feedback"]}}`)
	config, digest, err := ParseConfig(definition)
	if err != nil || len(digest) != 64 {
		t.Fatalf("parse = %#v, %q, %v", config, digest, err)
	}
	if config.Resources.CPUs != 1 || config.Resources.MemoryMB != 256 || config.Resources.StorageMB != 64 {
		t.Fatalf("resource contract was not retained: %#v", config.Resources)
	}
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 40), strings.Repeat("d", 32), digest, strings.Repeat("e", 32), config)
	if err != nil || created.State != "building" || created.URL == "" {
		t.Fatalf("create = %#v, %v", created, err)
	}
	store.now = func() time.Time { return time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC) }
	invited, err := store.Invite(created.RepositoryID, created.PullRequestID, created.ID, created.CreatorID, strings.Repeat("f", 32), "feedback", "issue", strings.Repeat("1", 32), store.now().Add(time.Hour))
	if err != nil || len(invited.Invitations) != 1 || invited.Invitations[0].Role != "feedback" {
		t.Fatalf("invite = %#v, %v", invited, err)
	}
	entered, invitation, err := store.Enter(created.RepositoryID, created.PullRequestID, created.ID, strings.Repeat("f", 32))
	if err != nil || invitation.ID == "" || len(entered.AudienceEvents) != 2 {
		t.Fatalf("enter = %#v, %#v, %v", entered, invitation, err)
	}
	commented, err := store.AddFeedback(created.RepositoryID, created.PullRequestID, created.ID, invitation.UserID, invitation.ID, "The checkout flow is clear.")
	if err != nil || len(commented.Feedback) != 1 || commented.Feedback[0].AuthorID != invitation.UserID {
		t.Fatalf("feedback = %#v, %v", commented, err)
	}
	revoked, err := store.Revoke(created.RepositoryID, created.PullRequestID, created.ID, invitation.ID, created.CreatorID)
	if err != nil || revoked.Invitations[0].RevokedAt == nil || len(revoked.AudienceEvents) != 4 {
		t.Fatalf("revoke = %#v, %v", revoked, err)
	}
	current, err := store.List(created.RepositoryID, created.PullRequestID, created.Revision)
	if err != nil || len(current) != 1 || current[0].Stale {
		t.Fatalf("current = %#v, %v", current, err)
	}
	moved, err := store.List(created.RepositoryID, created.PullRequestID, strings.Repeat("f", 40))
	if err != nil || len(moved) != 1 || !moved[0].Stale || moved[0].Revision != created.Revision {
		t.Fatalf("moved = %#v, %v", moved, err)
	}
}

func TestDefinitionRejectsSecretsAndUnboundedResources(t *testing.T) {
	for _, body := range []string{
		`{"version":1,"image":"alpine","build":"true","output_path":"dist","environment":{"GIT_TOKEN":"secret"},"resources":{"cpus":1,"memory_mb":128,"storage_mb":32,"timeout_seconds":30}}`,
		`{"version":1,"image":"alpine","build":"true","output_path":"dist","resources":{"cpus":3,"memory_mb":128,"storage_mb":32,"timeout_seconds":30}}`,
		`{"version":1,"image":"alpine","build":"true","output_path":"../dist","resources":{"cpus":1,"memory_mb":128,"storage_mb":32,"timeout_seconds":30}}`,
	} {
		if _, _, err := ParseConfig([]byte(body)); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
