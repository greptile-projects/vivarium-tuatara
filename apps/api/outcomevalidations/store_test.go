package outcomevalidations

import (
	"errors"
	"strings"
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
	if allowed, err := s.GuestAccess("repo", v.ID, "guest"); err != nil || !allowed {
		t.Fatalf("accepted guest access = %v, %v", allowed, err)
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

func TestGuestAccessRequiresLiveAcceptance(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	draft := Draft{RoadmapVersion: 1, ItemID: "item", Kind: "prototype", Title: "Prototype", Question: "Does it work?", Revision: "r1", Measures: []Measure{{Name: "Success", Kind: "success", Target: "yes", SourceIDs: []string{"f1"}}}}
	v, _ := s.Create("repo", "owner", "opportunity", 1, draft)
	v, _ = s.Invite("repo", v.ID, "owner", "declined", "preview", "r1", now.Add(time.Hour), v.Version)
	v, _ = s.Consent("repo", v.ID, v.Invitations[0].ID, "declined", "declined", v.Version)
	if allowed, _ := s.GuestAccess("repo", v.ID, "declined"); allowed {
		t.Fatal("declined invitation granted detail access")
	}
	v, _ = s.Invite("repo", v.ID, "owner", "expired", "research", "r1", now.Add(time.Minute), v.Version)
	v, _ = s.Consent("repo", v.ID, v.Invitations[1].ID, "expired", "accepted", v.Version)
	now = now.Add(2 * time.Minute)
	if allowed, _ := s.GuestAccess("repo", v.ID, "expired"); allowed {
		t.Fatal("expired invitation granted detail access")
	}
}

func TestAcceptedParticipantCanWithdrawConsent(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	draft := Draft{RoadmapVersion: 1, ItemID: "item", Kind: "prototype", Title: "Prototype", Question: "Does it work?", Revision: "r1", Measures: []Measure{{Name: "Success", Kind: "success", Target: "yes", SourceIDs: []string{"f1"}}}}
	v, _ := s.Create("repo", "owner", "opportunity", 1, draft)
	v, _ = s.Invite("repo", v.ID, "owner", "guest", "research", "r1", now.Add(time.Hour), v.Version)
	invite := v.Invitations[0].ID
	v, _ = s.Consent("repo", v.ID, invite, "guest", "accepted", v.Version)
	v, err := s.Consent("repo", v.ID, invite, "guest", "withdrawn", v.Version)
	if err != nil || v.Invitations[0].Status != "withdrawn" {
		t.Fatalf("withdrawal = %#v, %v", v.Invitations[0], err)
	}
	if allowed, _ := s.GuestAccess("repo", v.ID, "guest"); allowed {
		t.Fatal("withdrawn consent retained guest access")
	}
	if _, err := s.Find("repo", v.ID, invite, "guest", v.Version, Finding{Body: "late finding", Acceptance: "reject", EvidenceQuality: "valid"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("finding after withdrawal = %v", err)
	}
	if _, err := s.Consent("repo", v.ID, invite, "guest", "accepted", v.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("withdrawn invitation was silently reaccepted = %v", err)
	}
}

func TestFindingBoundsPersistedGrowth(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	v, _ := s.Create("repo", "owner", "opportunity", 1, Draft{RoadmapVersion: 1, ItemID: "item", Kind: "prototype", Title: "Prototype", Question: "Does it work?", Revision: "r1", Measures: []Measure{{Name: "Success", Kind: "success", Target: "yes", SourceIDs: []string{"f1"}}}})
	v, _ = s.Invite("repo", v.ID, "owner", "guest", "research", "r1", now.Add(time.Hour), v.Version)
	v, _ = s.Consent("repo", v.ID, v.Invitations[0].ID, "guest", "accepted", v.Version)
	invite := v.Invitations[0].ID
	for name, finding := range map[string]Finding{
		"dissent":       {Body: "finding", Dissent: strings.Repeat("x", 5001), Acceptance: "reject", EvidenceQuality: "valid"},
		"accessibility": {Body: "finding", AccessibilityNeeds: []string{strings.Repeat("x", 501)}, Acceptance: "reject", EvidenceQuality: "valid"},
	} {
		if _, err := s.Find("repo", v.ID, invite, "guest", v.Version, finding); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s bound = %v", name, err)
		}
	}
	for i := 0; i < maxFindings; i++ {
		var err error
		v, err = s.Find("repo", v.ID, invite, "guest", v.Version, Finding{Body: "bounded", Acceptance: "uncertain", EvidenceQuality: "insufficient"})
		if err != nil {
			t.Fatalf("finding %d: %v", i, err)
		}
	}
	if _, err := s.Find("repo", v.ID, invite, "guest", v.Version, Finding{Body: "one too many", Acceptance: "uncertain", EvidenceQuality: "insufficient"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("finding count bound = %v", err)
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
