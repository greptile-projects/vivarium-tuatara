package productopportunities

import (
	"errors"
	"testing"
)

func sampleRevision() Revision {
	return Revision{Title: "Faster review feedback", Need: "Reviewers repeatedly lose their place.", AffectedAudiences: []string{"large pull reviewers", "screen reader users"}, Severity: "high", Reach: "segment", Confidence: "medium", ExpectedValue: "Reduce abandoned reviews.", Uncertainty: []string{"support signals are self-selected"}, MinorityNeeds: []string{"keyboard-only navigation"}, Contradictions: []string{"some reviewers prefer a compact view"}, Sources: []Source{{Kind: "feedback", ResourceID: "feedback-1", Revision: "2026-08-13T20:00:00Z", Label: "Review continuity", Claim: "Context is lost between sessions.", Relationship: "supports", Audience: "large pull reviewers"}}}
}
func TestOpportunityRetainsVersionsChallengesCorrectionsAndDetachments(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create("repo", "agent-1", "agent", sampleRevision())
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 1 || v.Revisions[0].ActorType != "agent" || len(v.Revisions[0].Contradictions) != 1 {
		t.Fatalf("created = %#v", v)
	}
	v, err = s.Challenge("repo", v.ID, "reader", v.Version, Challenge{Body: "The cited sample excludes mobile users.", SourceIDs: []string{"feedback-1"}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Correct("repo", v.ID, "maintainer", v.Version, Correction{Field: "confidence", To: "low", Reason: "The sample is narrow."})
	if err != nil {
		t.Fatal(err)
	}
	if v.Revisions[0].Confidence != "low" || v.Corrections[0].From != "medium" {
		t.Fatalf("correction = %#v", v)
	}
	v, err = s.DetachFeedback("repo", v.ID, "feedback-1", "reporter", v.Version)
	if err != nil || v.Revisions[0].Sources[0].DetachedAt == nil {
		t.Fatalf("detach = %#v, %v", v, err)
	}
	r := sampleRevision()
	r.Title = "Reframed need"
	v, err = s.Revise("repo", v.ID, "maintainer", v.Version, r)
	if err != nil || v.CurrentVersion != 2 || len(v.Revisions) != 2 {
		t.Fatalf("revision = %#v, %v", v, err)
	}
	if _, err = s.Challenge("repo", v.ID, "reader", 1, Challenge{Body: "stale update"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict = %v", err)
	}
}
