package previews

import (
	"encoding/base64"
	"errors"
	"strings"
	"sync"
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

func TestFindResolvesOnlyWithinRepository(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	preview, err := store.Create("repo-a", "pull-a", strings.Repeat("a", 40), "owner", strings.Repeat("b", 64), "run", Config{Version: 1, Access: AccessPolicy{Actions: []string{"feedback"}}})
	if err != nil {
		t.Fatal(err)
	}
	if found, err := store.Find("repo-a", preview.ID); err != nil || found.ID != preview.ID {
		t.Fatalf("same-repository find = %#v, %v", found, err)
	}
	if _, err := store.Find("repo-b", preview.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign repository find error = %v", err)
	}
}

func TestRepairAttemptReservationRetainsExactPreviewAndBuildRun(t *testing.T) {
	config, digest, err := ParseConfig([]byte(`{"version":1,"image":"alpine:3.22","build":"true","output_path":"dist","resources":{"cpus":1,"memory_mb":128,"storage_mb":32,"timeout_seconds":30},"access":{"network":"none","data":"preview_artifacts","identity":"named_users","actions":["feedback"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, pull, revision := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 40)
	actor, session, finding := strings.Repeat("4", 32), strings.Repeat("5", 32), strings.Repeat("6", 32)
	first, err := store.ReserveRepairAttempt(repo, pull, revision, actor, digest, session, finding, config)
	if err != nil || first.BuildRunID != "" || first.State != "reserved" {
		t.Fatalf("reservation = %#v, %v", first, err)
	}
	second, err := store.ReserveRepairAttempt(repo, pull, revision, actor, digest, session, finding, config)
	if err != nil || second.ID != first.ID {
		t.Fatalf("retry = %#v, %v", second, err)
	}
	attached, err := store.AttachBuildRun(repo, pull, first.ID, strings.Repeat("7", 32))
	if err != nil || attached.BuildRunID != strings.Repeat("7", 32) {
		t.Fatalf("attach = %#v, %v", attached, err)
	}
	retried, err := store.ReserveRepairAttempt(repo, pull, revision, actor, digest, session, finding, config)
	if err != nil || retried.ID != first.ID || retried.BuildRunID != attached.BuildRunID {
		t.Fatalf("attached retry = %#v, %v", retried, err)
	}
	if _, err := store.AttachBuildRun(repo, pull, first.ID, strings.Repeat("8", 32)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("replacement run error = %v", err)
	}
}

func TestRepairAttemptReservationSerializesIndependentStores(t *testing.T) {
	config, digest, err := ParseConfig([]byte(`{"version":1,"image":"alpine:3.22","build":"true","output_path":"dist","resources":{"cpus":1,"memory_mb":128,"storage_mb":32,"timeout_seconds":30},"access":{"network":"none","data":"preview_artifacts","identity":"named_users","actions":["feedback"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	first, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	repo, pull, revision := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 40)
	actor, session, finding := strings.Repeat("4", 32), strings.Repeat("5", 32), strings.Repeat("6", 32)
	const requests = 32
	ids := make(chan string, requests)
	errs := make(chan error, requests)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		selected := first
		if i%2 == 1 {
			selected = second
		}
		wg.Add(1)
		go func(store *Store) {
			defer wg.Done()
			<-start
			p, reserveErr := store.ReserveRepairAttempt(repo, pull, revision, actor, digest, session, finding, config)
			if reserveErr != nil {
				errs <- reserveErr
				return
			}
			ids <- p.ID
		}(selected)
	}
	close(start)
	wg.Wait()
	close(ids)
	close(errs)
	for reserveErr := range errs {
		t.Fatal(reserveErr)
	}
	var expected string
	for id := range ids {
		if expected == "" {
			expected = id
		}
		if id != expected {
			t.Fatalf("reservation IDs = %s and %s", expected, id)
		}
	}
	list, err := first.List(repo, pull, revision)
	if err != nil || len(list) != 1 || list[0].ID != expected {
		t.Fatalf("reservations = %#v, %v", list, err)
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

func TestAudienceAdmissionExcludesCommittedRevocation(t *testing.T) {
	store, _ := New(t.TempDir())
	preview, err := store.Create("repo", "pull", strings.Repeat("a", 40), "owner", strings.Repeat("b", 64), "run", Config{Version: 1, Access: AccessPolicy{Actions: []string{"feedback"}}})
	if err != nil {
		t.Fatal(err)
	}
	preview, err = store.Invite("repo", "pull", preview.ID, "owner", "stakeholder", "feedback", "user", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	admissionDone := make(chan error, 1)
	go func() {
		admissionDone <- store.WithAudienceAdmission(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	revokeDone := make(chan error, 1)
	go func() {
		_, revokeErr := store.Revoke("repo", "pull", preview.ID, preview.Invitations[0].ID, "owner")
		revokeDone <- revokeErr
	}()
	select {
	case err := <-revokeDone:
		t.Fatalf("revocation committed inside audience admission: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-admissionDone; err != nil {
		t.Fatal(err)
	}
	if err := <-revokeDone; err != nil {
		t.Fatal(err)
	}
}
