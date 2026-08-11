package previews

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFindingsFreezeRevisionRedactAndRetainCollaboration(t *testing.T) {
	config, digest, err := ParseConfig([]byte(`{"version":1,"image":"alpine:3.22","build":"true","output_path":"dist","resources":{"cpus":1,"memory_mb":128,"storage_mb":32,"timeout_seconds":30},"access":{"network":"none","data":"preview_artifacts","identity":"named_users","actions":["feedback"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.Create("repo", "pull", strings.Repeat("a", 40), "owner", digest, "run", config)
	if err != nil {
		t.Fatal(err)
	}
	secret := base64.StdEncoding.EncodeToString([]byte("{\"password\":\"json-secret\",\"nested\":{\"api_key\":\"nested-secret\"}}\nAuthorization: Bearer eyJ.test.synthetic.token"))
	updated, finding, err := store.AddFinding("repo", "pull", p.ID, "guest", "/checkout?plan=team", "Payment fails", "Expected success", "bug", "blocking", "", []string{"Open page", "token=private"}, []FindingEvidence{{Kind: "console", Name: "console.txt", MediaType: "text/plain", Data: secret}})
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ := base64.StdEncoding.DecodeString(finding.Evidence[0].Data)
	if finding.Revision != p.Revision || finding.Status != "open" || !finding.Evidence[0].Redacted || strings.Contains(string(decoded), "json-secret") || strings.Contains(string(decoded), "nested-secret") || strings.Contains(string(decoded), "eyJ.test.synthetic.token") || strings.Contains(finding.ReproductionSteps[1], "private") {
		t.Fatalf("unsafe finding = %#v, evidence %q", finding, string(decoded))
	}
	persisted, err := store.Get("repo", "pull", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	persistedEvidence, _ := base64.StdEncoding.DecodeString(persisted.Findings[0].Evidence[0].Data)
	if strings.Contains(string(persistedEvidence), "json-secret") || strings.Contains(string(persistedEvidence), "nested-secret") || strings.Contains(string(persistedEvidence), "eyJ.test.synthetic.token") {
		t.Fatalf("persisted credentials = %q", persistedEvidence)
	}
	_, related, err := store.AddFinding("repo", "pull", p.ID, "owner", "/checkout", "Same failure", "", "bug", "major", finding.ID, []string{"Retry"}, nil)
	if err != nil || related.DuplicateOf != finding.ID {
		t.Fatalf("duplicate = %#v, %v", related, err)
	}
	_, discussed, err := store.MutateFinding("repo", "pull", p.ID, finding.ID, "owner", finding.Version, func(current *Finding) error {
		current.Comments = append(current.Comments, FindingComment{ID: NewID(), AuthorID: "owner", Body: "Reproduced"})
		current.Status = "resolved"
		return nil
	})
	if err != nil || discussed.Version != 2 || discussed.Status != "resolved" {
		t.Fatalf("decision = %#v, %v", discussed, err)
	}
	if _, _, err = store.MutateFinding("repo", "pull", p.ID, finding.ID, "guest", finding.Version, func(*Finding) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("stale mutation = %v", err)
	}
	if len(updated.Findings) != 1 {
		t.Fatalf("finding was not retained: %#v", updated.Findings)
	}
}

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
	firstID := invited.Invitations[0].ID
	entered, firstInvitation, err := store.Enter(created.RepositoryID, created.PullRequestID, created.ID, strings.Repeat("f", 32))
	if err != nil || firstInvitation.ID != firstID || len(entered.AudienceEvents) != 2 {
		t.Fatalf("first entry = %#v, %#v, %v", entered, firstInvitation, err)
	}
	changedExpiry := store.now().Add(24 * time.Hour)
	reinvited, err := store.Invite(created.RepositoryID, created.PullRequestID, created.ID, created.CreatorID, strings.Repeat("f", 32), "feedback", "decision", strings.Repeat("2", 32), changedExpiry)
	if err != nil || len(reinvited.Invitations) != 2 || reinvited.Invitations[0].RevokedAt == nil || reinvited.Invitations[1].ID == firstID || reinvited.Invitations[1].SourceKind != "decision" || reinvited.Invitations[1].SourceID != strings.Repeat("2", 32) || !reinvited.Invitations[1].ExpiresAt.Equal(changedExpiry) || len(reinvited.AudienceEvents) != 4 {
		t.Fatalf("reinvite = %#v, %v", reinvited, err)
	}
	persisted, err := store.Get(created.RepositoryID, created.PullRequestID, created.ID)
	if err != nil || persisted.Invitations[0].SourceKind != "issue" || persisted.Invitations[0].RevokedAt == nil || persisted.Invitations[1].SourceKind != "decision" || !persisted.Invitations[1].ExpiresAt.Equal(changedExpiry) {
		t.Fatalf("persisted reinvite = %#v, %v", persisted, err)
	}
	entered, invitation, err := store.Enter(created.RepositoryID, created.PullRequestID, created.ID, strings.Repeat("f", 32))
	if err != nil || invitation.ID != persisted.Invitations[1].ID || len(entered.AudienceEvents) != 5 {
		t.Fatalf("enter = %#v, %#v, %v", entered, invitation, err)
	}
	enteredCount := 0
	for _, event := range entered.AudienceEvents {
		if event.Kind == "entered" {
			enteredCount++
		}
	}
	if enteredCount != 2 {
		t.Fatalf("entry audit count = %d, events %#v", enteredCount, entered.AudienceEvents)
	}
	commented, err := store.AddFeedback(created.RepositoryID, created.PullRequestID, created.ID, invitation.UserID, invitation.ID, "The checkout flow is clear.")
	if err != nil || len(commented.Feedback) != 1 || commented.Feedback[0].AuthorID != invitation.UserID {
		t.Fatalf("feedback = %#v, %v", commented, err)
	}
	commented, err = store.AddFeedback(created.RepositoryID, created.PullRequestID, created.ID, invitation.UserID, invitation.ID, strings.Repeat("é", 4000))
	if err != nil || len(commented.Feedback) != 2 {
		t.Fatalf("unicode feedback = %d records, %v", len(commented.Feedback), err)
	}
	if _, err = store.AddFeedback(created.RepositoryID, created.PullRequestID, created.ID, invitation.UserID, invitation.ID, strings.Repeat("é", 4001)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversize Unicode feedback error = %v", err)
	}
	revoked, err := store.Revoke(created.RepositoryID, created.PullRequestID, created.ID, invitation.ID, created.CreatorID)
	if err != nil || revoked.Invitations[1].RevokedAt == nil || len(revoked.AudienceEvents) != 8 {
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

func TestInviteRejectsExpiryElapsedWhileWaitingForStoreLock(t *testing.T) {
	definition := []byte(`{"version":1,"image":"alpine:3.22","build":"true","output_path":"dist","resources":{"cpus":1,"memory_mb":128,"storage_mb":32,"timeout_seconds":30},"access":{"network":"none","data":"preview_artifacts","identity":"named_users","actions":["view"]}}`)
	config, digest, err := ParseConfig(definition)
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	current := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return current }
	created, err := store.Create(strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 40), strings.Repeat("d", 32), digest, strings.Repeat("e", 32), config)
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := current.Add(time.Second)
	store.mu.Lock()
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(started)
		_, inviteErr := store.Invite(created.RepositoryID, created.PullRequestID, created.ID, created.CreatorID, strings.Repeat("f", 32), "view", "user", "", expiresAt)
		result <- inviteErr
	}()
	<-started
	current = expiresAt
	store.mu.Unlock()
	if err := <-result; !errors.Is(err, ErrInvalid) {
		t.Fatalf("contended expired invite error = %v", err)
	}
	persisted, err := store.Get(created.RepositoryID, created.PullRequestID, created.ID)
	if err != nil || len(persisted.Invitations) != 0 {
		t.Fatalf("expired invitation persisted = %#v, %v", persisted.Invitations, err)
	}
}
