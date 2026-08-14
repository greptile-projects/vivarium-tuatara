package accessibilityreports

import "testing"

func TestReportPrivacyAndClassifiedAttempts(t *testing.T) {
	s, _ := New(t.TempDir())
	in := Report{Target: Target{Kind: "preview", ResourceID: "preview-1", Revision: "abc123"}, AccessNeeds: []string{"consistent focus order"}, ExpectedOutcome: "focus reaches Save", Steps: []string{"Press Tab"}, ReporterEnvironment: Environment{Browser: "Firefox", BrowserVersion: "128", Device: "personal switch", OperatingSystem: "Linux", AssistiveTechnology: "Orca", AssistiveTechnologyVersion: "47", InputMode: "switch"}, Evidence: []Artifact{{Kind: "accessibility_tree", Description: "redacted tree", ContentRef: "artifact://tree-1", Redacted: true}}, Consent: Consent{ShareIdentity: false, ShareDeviceDetails: false}}
	created, err := s.Create("repo", "reporter", in)
	if err != nil {
		t.Fatal(err)
	}
	projected := Project(created, "maintainer", true)
	if projected.ReporterID != "" || projected.ReporterEnvironment.Device != "" || projected.ReporterEnvironment.Browser != "Firefox" {
		t.Fatalf("unsafe projection: %+v", projected)
	}
	got, err := s.AddAttempt(created.ID, "maintainer", Attempt{Boundary: "preview", Environment: Environment{Browser: "Firefox", Device: "desktop", OperatingSystem: "Linux", AssistiveTechnology: "Orca"}, Outcome: "environment_specific", Notes: "only with screen reader", Evidence: []Artifact{{Kind: "speech_output", Description: "redacted utterance", ContentRef: "artifact://speech-1", Redacted: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Attempts) != 1 || got.Attempts[0].Revision != "abc123" || got.Attempts[0].Outcome != "environment_specific" {
		t.Fatalf("attempt = %+v", got.Attempts)
	}
}

func TestRejectsUnredactedEvidenceAndUnclassifiedAttempt(t *testing.T) {
	s, _ := New(t.TempDir())
	base := Report{Target: Target{Kind: "page", ResourceID: "settings", Revision: "abc"}, AccessNeeds: []string{"keyboard"}, ExpectedOutcome: "save", Steps: []string{"tab"}, Evidence: []Artifact{{Kind: "screenshot", Description: "raw", ContentRef: "artifact://raw", Redacted: false}}}
	if _, err := s.Create("repo", "reporter", base); err != ErrInvalid {
		t.Fatalf("create error = %v", err)
	}
	base.Evidence[0].Redacted = true
	created, _ := s.Create("repo", "reporter", base)
	if _, err := s.AddAttempt(created.ID, "runner", Attempt{Boundary: "workspace", Environment: Environment{Browser: "Chrome", Device: "desktop", AssistiveTechnology: "keyboard"}, Outcome: "fixed"}); err != ErrInvalid {
		t.Fatalf("attempt error = %v", err)
	}
}
