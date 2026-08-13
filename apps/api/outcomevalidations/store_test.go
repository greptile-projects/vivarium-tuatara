package outcomevalidations

import (
	"errors"
	"testing"
	"time"
)

func TestConsentFindingAndConclusionPreserveLearning(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	v, err := s.Create("repo", "maintainer", "opportunity", 2, Draft{RoadmapVersion: 3, ItemID: "item", Kind: "prototype", Title: "Test navigation", Question: "Can readers finish?", Revision: "preview-sha", Measures: []Measure{{Name: "Completion", Kind: "success", Target: "4 of 5", SourceIDs: []string{"feedback-1"}}, {Name: "No keyboard trap", Kind: "guardrail", Target: "zero", SourceIDs: []string{"feedback-1"}}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Invite("repo", v.ID, "maintainer", "guest", "preview", "preview-sha", now.Add(time.Hour), v.Version)
	if err != nil {
		t.Fatal(err)
	}
	invite := v.Invitations[0].ID
	if _, err = s.Find("repo", v.ID, invite, "guest", v.Version, Finding{Body: "Blocked", Acceptance: "reject", EvidenceQuality: "valid"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("finding before consent = %v", err)
	}
	v, err = s.Consent("repo", v.ID, invite, "guest", "accepted", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Find("repo", v.ID, invite, "guest", v.Version, Finding{Body: "Keyboard focus disappears.", AccessibilityNeeds: []string{"visible focus"}, Dissent: "The proposed direction adds steps.", Acceptance: "reject", EvidenceQuality: "valid"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Conclude("repo", v.ID, "maintainer", "revise", "Representative participant found a blocking accessibility issue.", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Findings) != 1 || len(v.Conclusions) != 1 || v.Findings[0].AccessibilityNeeds[0] != "visible focus" {
		t.Fatalf("learning not retained: %#v", v)
	}
}

func TestInvitationMustMatchFrozenRevision(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	v, _ := s.Create("repo", "owner", "opportunity", 1, Draft{RoadmapVersion: 1, ItemID: "item", Kind: "documentation_concept", Title: "Concept", Question: "Does it help?", Revision: "r1", Measures: []Measure{{Name: "Findability", Kind: "success", Target: "80%", SourceIDs: []string{"f1"}}}})
	if _, err := s.Invite("repo", v.ID, "owner", "guest", "research", "r2", now.Add(time.Hour), v.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("revision mismatch = %v", err)
	}
}
