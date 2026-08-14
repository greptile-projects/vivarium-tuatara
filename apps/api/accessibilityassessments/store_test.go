package accessibilityassessments

import "testing"

func TestAssessmentLifecycleInvalidatesOnlyAffectedEvidence(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Create("repo", "owner", Assessment{Revision: "abc123", PullRequestID: "pull", Checks: []Check{
		{Name: "Keyboard journey", Category: "keyboard", Outcome: "passed", JourneyID: "checkout", SourceLocations: []string{"src/checkout.tsx"}, AudienceIDs: []string{"keyboard"}, Summary: "All declared steps completed."},
		{Name: "Captions", Category: "captions", Outcome: "unevaluated", SourceLocations: []string{"src/video.tsx"}, AudienceIDs: []string{"deaf"}, Summary: "Requires a person to assess meaning.", RequiresHuman: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	a, err = s.AddFinding("repo", a.ID, "human", "specialist", Finding{Title: "Focus order changes", Detail: "The dialog returns focus to the page start.", Severity: "major", AudienceIDs: []string{"keyboard", "screen_reader"}, SourceLocations: []string{"src/dialog.tsx"}, JourneyIDs: []string{"checkout"}, Uncertainty: "Observed twice in Chromium; other browsers unevaluated.", RequiresHuman: true, Citations: []Citation{{Kind: "preview", ResourceID: "preview-1", Revision: "abc123", Location: "/checkout", EvidenceRef: "artifact://focus-trace"}}})
	if err != nil {
		t.Fatal(err)
	}
	finding := a.Findings[0]
	a, err = s.Decide("repo", a.ID, finding.ID, "owner", "accepted", "Confirmed with keyboard and screen reader.")
	if err != nil {
		t.Fatal(err)
	}
	a, err = s.Invalidate("repo", a.ID, "owner", []string{"src/dialog.tsx"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Findings[0].InvalidatedAt == nil || a.Findings[0].Decision != nil {
		t.Fatal("affected finding and acceptance were not invalidated")
	}
	if a.Checks[0].InvalidatedAt != nil || a.Checks[1].InvalidatedAt != nil {
		t.Fatal("unaffected checks were invalidated")
	}
}

func TestFindingRequiresExactCitedRevision(t *testing.T) {
	s, _ := New(t.TempDir())
	a, _ := s.Create("repo", "owner", Assessment{Revision: "abc", Checks: []Check{{Name: "Semantics", Category: "semantics", Outcome: "passed", SourceLocations: []string{"page.tsx"}, AudienceIDs: []string{"screen_reader"}, Summary: "No violations."}}})
	_, err := s.AddFinding("repo", a.ID, "agent", "agent-1", Finding{Title: "Missing name", Detail: "Button has no name.", Severity: "major", AudienceIDs: []string{"screen_reader"}, Uncertainty: "Low", Citations: []Citation{{Kind: "preview", ResourceID: "p", Revision: "old", EvidenceRef: "artifact://tree"}}})
	if err != ErrInvalid {
		t.Fatalf("got %v", err)
	}
}
