package supportthreads

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestContextualQuestionLifecycleAndDiagnostics(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create(Thread{RepositoryID: "repo", AuthorID: "developer", Title: "How do I retry uploads?", Body: "The client stops after a timeout.", Target: Target{Kind: "api", Label: "Upload API"}, Urgency: "high", Audience: "public", ContactPreferences: ContactPreferences{ReplyInThread: true}, Attachments: []Attachment{{Kind: "log", Name: "client.log", MediaType: "text/plain", Size: 4, Data: "bG9n"}}})
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "open" || v.Version != 1 || len(v.History) != 1 || len(v.Diagnostics) != 4 || v.Attachments[0].ID == "" {
		t.Fatalf("created = %#v", v)
	}
	if _, err = s.UpdateStatus("repo", v.ID, "outsider", "answered", "", v.Version, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-maintainer update = %v", err)
	}
	v, err = s.UpdateStatus("repo", v.ID, "maintainer", "needs_context", "Please include the runtime.", v.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "needs_context" || v.Version != 2 || len(v.History) != 2 {
		t.Fatalf("updated = %#v", v)
	}
	v, err = s.AddReply("repo", v.ID, "developer", "The runtime is Go 1.26; the timeout occurs after the first upload.", v.Version, false)
	if err != nil || len(v.Replies) != 1 || v.Replies[0].ActorID != "developer" || v.Version != 3 {
		t.Fatalf("reply = %#v, %v", v, err)
	}
	if _, err = s.AddReply("repo", v.ID, "outsider", "untrusted reply", v.Version, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("outsider reply = %v", err)
	}
	v, err = s.AddReply("repo", v.ID, "maintainer", "Confirmed; I can reproduce that sequence.", v.Version, true)
	if err != nil || len(v.Notifications) != 1 || v.Notifications[0].UserID != "developer" {
		t.Fatalf("asker notification = %#v, %v", v.Notifications, err)
	}
	if _, err = s.UpdateStatus("repo", v.ID, "maintainer", "answered", "", 1, true); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update = %v", err)
	}
	reloaded, err := s.Get("repo", v.ID)
	if err != nil || reloaded.History[1].Message != "Please include the runtime." || reloaded.Replies[0].Body == "" {
		t.Fatalf("reloaded = %#v, %v", reloaded, err)
	}
}

func TestResolveHoldsThreadMutationBoundaryAcrossPublication(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Create(Thread{RepositoryID: "repo", AuthorID: "asker", Title: "Help", Body: "Details", Target: Target{Kind: "repository", Label: "repo"}, Urgency: "normal", Audience: "public", ContactPreferences: ContactPreferences{ReplyInThread: true}})
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var resolved Thread
	var resolveErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		resolved, resolveErr = s.Resolve("repo", v.ID, "asker", "published", v.Version, false, func() (func() error, error) {
			close(entered)
			<-release
			return nil, nil
		})
	}()
	<-entered
	updateDone := make(chan error, 1)
	go func() {
		_, updateErr := s.UpdateStatus("repo", v.ID, "asker", "open", "concurrent", v.Version, false)
		updateDone <- updateErr
	}()
	close(release)
	wg.Wait()
	if resolveErr != nil || resolved.Status != "closed" || resolved.History[len(resolved.History)-1].Kind != "resolved" {
		t.Fatalf("resolved = %#v, %v", resolved, resolveErr)
	}
	if updateErr := <-updateDone; !errors.Is(updateErr, ErrConflict) {
		t.Fatalf("concurrent update = %v", updateErr)
	}
}

func TestEscalationRetryReconcilesDurablePendingIdentity(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Create(Thread{RepositoryID: "repo", AuthorID: "maintainer", Title: "Gap", Body: "Details", Target: Target{Kind: "package", Label: "sdk", Version: "3.2"}, Goal: "Make retries safe", AttemptedSteps: []string{"retry"}, Urgency: "high", Audience: "maintainers", ContactPreferences: ContactPreferences{ReplyInThread: true}})
	if err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(s.root, "repo")
	identities := map[string]bool{}
	bases := []string{}
	publishCalls := 0
	first := true
	publish := func(_ Thread, escalationID, baseRevision string) (string, string, error) {
		publishCalls++
		identities[escalationID] = true
		bases = append(bases, baseRevision)
		if first {
			first = false
			if chmodErr := os.Chmod(repoDir, 0500); chmodErr != nil {
				return "", "", chmodErr
			}
		}
		return escalationID, "/issues/" + escalationID, nil
	}
	_, err = s.Escalate("repo", v.ID, "maintainer", v.Version, "defect", "issue", strings.Repeat("a", 40), []string{"retry once"}, publish)
	if chmodErr := os.Chmod(repoDir, 0700); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	if err == nil {
		t.Fatal("forced finalization failure succeeded")
	}
	pending, readErr := s.Get("repo", v.ID)
	if readErr != nil || pending.Version != v.Version+1 || len(pending.Escalations) != 1 || pending.Escalations[0].Status != "pending" {
		t.Fatalf("pending = %#v, %v", pending, readErr)
	}
	completed, err := s.Escalate("repo", v.ID, "maintainer", v.Version, "defect", "issue", strings.Repeat("b", 40), []string{"retry once"}, publish)
	if err != nil || completed.Version != v.Version+2 || completed.Escalations[0].Status != "published" || completed.Escalations[0].ResourceID != pending.Escalations[0].ID {
		t.Fatalf("completed = %#v, %v", completed, err)
	}
	if len(identities) != 1 {
		t.Fatalf("published identities = %#v", identities)
	}
	if len(bases) != 2 || bases[0] != strings.Repeat("a", 40) || bases[1] != bases[0] {
		t.Fatalf("publication bases = %#v", bases)
	}
	replayed, err := s.Escalate("repo", v.ID, "maintainer", v.Version, "defect", "issue", strings.Repeat("c", 40), []string{"retry once"}, publish)
	if err != nil || replayed.Version != completed.Version || replayed.Escalations[0].ResourceID != completed.Escalations[0].ResourceID {
		t.Fatalf("lost-response replay = %#v, %v", replayed, err)
	}
	if publishCalls != 2 {
		t.Fatalf("lost-response replay called publish again: %d", publishCalls)
	}
	refreshed, err := s.Escalate("repo", v.ID, "maintainer", completed.Version, "defect", "issue", strings.Repeat("d", 40), []string{"retry once"}, publish)
	if err != nil || refreshed.Version != completed.Version || refreshed.Escalations[0].ResourceID != completed.Escalations[0].ResourceID || publishCalls != 2 {
		t.Fatalf("refreshed replay = %#v, calls = %d, err = %v", refreshed, publishCalls, err)
	}
}

func TestResolveCompensatesPublicationWhenThreadCloseCannotPersist(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Create(Thread{RepositoryID: "repo", AuthorID: "asker", Title: "Help", Body: "Details", Target: Target{Kind: "repository", Label: "repo"}, Urgency: "normal", Audience: "public", ContactPreferences: ContactPreferences{ReplyInThread: true}})
	if err != nil {
		t.Fatal(err)
	}
	published := false
	repoDir := filepath.Join(s.root, "repo")
	_, err = s.Resolve("repo", v.ID, "asker", "published", v.Version, false, func() (func() error, error) {
		published = true
		if chmodErr := os.Chmod(repoDir, 0500); chmodErr != nil {
			return nil, chmodErr
		}
		return func() error { published = false; return nil }, nil
	})
	if chmodErr := os.Chmod(repoDir, 0700); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	if err == nil {
		t.Fatal("forced close failure succeeded")
	}
	if published {
		t.Fatal("external publication was not compensated")
	}
	reloaded, readErr := s.Get("repo", v.ID)
	if readErr != nil || reloaded.Status != "open" || reloaded.Version != v.Version {
		t.Fatalf("thread diverged: %#v, %v", reloaded, readErr)
	}
}

func TestResolveDoesNotCompensatePreexistingPublication(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Create(Thread{RepositoryID: "repo", AuthorID: "asker", Title: "Help", Body: "Details", Target: Target{Kind: "repository", Label: "repo"}, Urgency: "normal", Audience: "public", ContactPreferences: ContactPreferences{ReplyInThread: true}})
	if err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(s.root, "repo")
	_, err = s.Resolve("repo", v.ID, "asker", "retry", v.Version, false, func() (func() error, error) {
		if chmodErr := os.Chmod(repoDir, 0500); chmodErr != nil {
			return nil, chmodErr
		}
		return nil, nil
	})
	if chmodErr := os.Chmod(repoDir, 0700); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	if err == nil {
		t.Fatal("forced retry close failure succeeded")
	}
	reloaded, readErr := s.Get("repo", v.ID)
	if readErr != nil || reloaded.Status != "open" {
		t.Fatalf("thread = %#v, %v", reloaded, readErr)
	}
}

func TestQuestionRequiresSafeTargetContactAndAttachments(t *testing.T) {
	s, _ := New(t.TempDir())
	base := Thread{RepositoryID: "repo", AuthorID: "user", Title: "Help", Body: "Details", Target: Target{Kind: "repository", Label: "repo"}, Urgency: "normal", Audience: "maintainers", ContactPreferences: ContactPreferences{ReplyInThread: true}}
	bad := base
	bad.Target.Kind = "secret"
	if _, e := s.Create(bad); !errors.Is(e, ErrInvalid) {
		t.Fatalf("unsafe target = %v", e)
	}
	bad = base
	bad.ContactPreferences = ContactPreferences{}
	if _, e := s.Create(bad); !errors.Is(e, ErrInvalid) {
		t.Fatalf("missing contact = %v", e)
	}
	bad = base
	bad.Attachments = []Attachment{{Kind: "screenshot", Name: "screen.png", MediaType: "image/png", Size: 1, Data: "eA=="}}
	if _, e := s.Create(bad); !errors.Is(e, ErrInvalid) {
		t.Fatalf("unsafe attachment = %v", e)
	}
}
