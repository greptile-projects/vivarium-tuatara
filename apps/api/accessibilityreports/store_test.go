package accessibilityreports

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const testRepo = "0123456789abcdef0123456789abcdef"

func sampleReport() Report {
	return Report{Target: Target{Kind: "page", ResourceID: "settings", Revision: "abc"}, AccessNeeds: []string{"keyboard"}, ExpectedOutcome: "save", Steps: []string{"tab"}, Evidence: []Artifact{{Kind: "screenshot", Description: "redacted", ContentRef: "artifact://one", Redacted: true}}}
}

func TestReportPrivacyAndClassifiedAttempts(t *testing.T) {
	s, _ := New(t.TempDir())
	in := sampleReport()
	in.ReporterEnvironment = Environment{Browser: "Firefox", BrowserVersion: "128", Device: "personal switch", OperatingSystem: "Linux", AssistiveTechnology: "Orca", AssistiveTechnologyVersion: "47", InputMode: "switch"}
	created, err := s.Create(testRepo, "reporter", in)
	if err != nil {
		t.Fatal(err)
	}
	projected := Project(created, "maintainer", true)
	if projected.ReporterID != "" || projected.ReporterEnvironment.Device != "" || projected.ReporterEnvironment.Browser != "Firefox" {
		t.Fatalf("unsafe projection: %+v", projected)
	}
	got, err := s.AddAttempt(testRepo, created.ID, "maintainer", Attempt{Boundary: "preview", Environment: Environment{Browser: "Firefox", Device: "desktop", OperatingSystem: "Linux", AssistiveTechnology: "Orca"}, Outcome: "environment_specific", Notes: "only with screen reader", Evidence: []Artifact{{Kind: "speech_output", Description: "redacted utterance", ContentRef: "artifact://speech-1", Redacted: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Attempts) != 1 || got.Attempts[0].Revision != "abc" {
		t.Fatalf("attempt = %+v", got.Attempts)
	}
}

func TestRejectsUnredactedEvidenceAndUnclassifiedAttempt(t *testing.T) {
	s, _ := New(t.TempDir())
	base := sampleReport()
	base.Evidence[0].Redacted = false
	if _, err := s.Create(testRepo, "reporter", base); err != ErrInvalid {
		t.Fatalf("create error = %v", err)
	}
	base.Evidence[0].Redacted = true
	created, _ := s.Create(testRepo, "reporter", base)
	if _, err := s.AddAttempt(testRepo, created.ID, "runner", Attempt{Boundary: "workspace", Environment: Environment{Browser: "Chrome", Device: "desktop", AssistiveTechnology: "keyboard"}, Outcome: "fixed"}); err != ErrInvalid {
		t.Fatalf("attempt error = %v", err)
	}
}

func TestReportsRequireEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	x := sampleReport()
	x.Evidence = nil
	if _, err := s.Create(testRepo, "reporter", x); err != ErrInvalid {
		t.Fatalf("omitted evidence error = %v", err)
	}
	x.Evidence = []Artifact{}
	if _, err := s.Create(testRepo, "reporter", x); err != ErrInvalid {
		t.Fatalf("empty evidence error = %v", err)
	}
}

func TestRepositoryCorruptionIsIsolated(t *testing.T) {
	root := t.TempDir()
	s, _ := New(root)
	created, err := s.Create(testRepo, "reporter", sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	other := "abcdef0123456789abcdef0123456789"
	foreign, err := s.Create(other, "reporter", sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, other, foreign.ID+".json"), []byte(`{"repository_id":`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := s.List(testRepo)
	if err != nil || len(got) != 1 || got[0].ID != created.ID {
		t.Fatalf("list = %+v, %v", got, err)
	}
}

func TestConcurrentStoresPreserveAttempts(t *testing.T) {
	root := t.TempDir()
	creator, _ := New(root)
	created, err := creator.Create(testRepo, "reporter", sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	const count = 24
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, _ := New(root)
			_, err := s.AddAttempt(testRepo, created.ID, "runner", Attempt{Boundary: "workspace", Environment: Environment{Browser: "Firefox", Device: "desktop", AssistiveTechnology: "Orca"}, Outcome: "unconfirmed"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := creator.Get(testRepo, created.ID)
	if err != nil || len(got.Attempts) != count {
		t.Fatalf("attempts = %d, %v", len(got.Attempts), err)
	}
}
