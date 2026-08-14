package accessibilityassessments

import (
	"strings"
	"testing"
)

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

func TestRepairReservationSurvivesFindingInvalidation(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := strings.Repeat("a", 40)
	a, err := s.Create("repo", "owner", Assessment{Revision: revision, Checks: []Check{{Name: "Keyboard", Category: "keyboard", Outcome: "failed", SourceLocations: []string{"form.tsx"}, AudienceIDs: []string{"keyboard"}, Summary: "Save is skipped"}}})
	if err != nil {
		t.Fatal(err)
	}
	a, err = s.AddFinding("repo", a.ID, "human", "specialist", Finding{Title: "Save skipped", Detail: "Focus skips Save", Severity: "major", AudienceIDs: []string{"keyboard"}, SourceLocations: []string{"form.tsx"}, Uncertainty: "none", Citations: []Citation{{Kind: "reproduction", ResourceID: "attempt", Revision: revision, EvidenceRef: "artifact://attempt"}}})
	if err != nil {
		t.Fatal(err)
	}
	findingID := a.Findings[0].ID
	if _, err = s.Decide("repo", a.ID, findingID, "owner", "accepted", "reproduced"); err != nil {
		t.Fatal(err)
	}
	request := Repair{BaseRevision: revision, AcceptanceCriteria: []string{"Save receives focus"}, CommitmentID: "commitment", CommitmentVersion: 1, CommitmentTitle: "Keyboard form", ComponentGuidance: []string{"Use shared focus styles"}, PermittedEvidence: []RepairEvidence{{Kind: "reproduction", ResourceID: "attempt", EvidenceRef: "artifact://attempt", Summary: "Press Tab"}}, AssigneeType: "human", AssigneeID: "collaborator"}
	_, reservation, err := s.ReserveRepair("repo", a.ID, findingID, "owner", request)
	if err != nil || reservation.State != "pending" || reservation.RecoveryID == "" {
		t.Fatalf("reservation = %+v, %v", reservation, err)
	}
	if _, err = s.Invalidate("repo", a.ID, "owner", []string{"form.tsx"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, retry, retryErr := s.ReserveRepair("repo", a.ID, findingID, "owner", request); retryErr != nil || retry.RecoveryID != reservation.RecoveryID {
		t.Fatalf("retry = %+v, %v", retry, retryErr)
	}
	linked, err := s.FinalizeRepair("repo", a.ID, findingID, reservation.RecoveryID, "proposal", "task")
	if err != nil || linked.Findings[0].Repair.State != "linked" || linked.Findings[0].Repair.ProposalID != "proposal" {
		t.Fatalf("linked = %+v, %v", linked, err)
	}
}
