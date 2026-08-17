package supportthreads

import (
	"errors"
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
	if _, err = s.UpdateStatus("repo", v.ID, "maintainer", "answered", "", 1, true); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update = %v", err)
	}
	reloaded, err := s.Get("repo", v.ID)
	if err != nil || reloaded.History[1].Message != "Please include the runtime." {
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
		resolved, resolveErr = s.Resolve("repo", v.ID, "asker", "published", v.Version, false, func() error {
			close(entered)
			<-release
			return nil
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
