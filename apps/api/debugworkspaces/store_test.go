package debugworkspaces

import (
	"errors"
	"testing"
	"time"
)

func TestWorkspaceRetainsExactContextAndCASHistory(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	v, err := s.Create(Workspace{RepositoryID: "11111111111111111111111111111111", Title: "Intermittent checkout failure", Summary: "Users receive a timeout after payment authorization.", Trigger: Reference{Kind: "manual_observation", Label: "operator report"}, Release: Reference{Kind: "release", ResourceID: "22222222222222222222222222222222", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Label: "v1.4.2"}, Environment: Reference{Kind: "environment", ResourceID: "33333333333333333333333333333333", Label: "production"}, TimeStart: start, TimeEnd: start.Add(time.Hour), UserJourney: "complete checkout", OwnerIDs: []string{"44444444444444444444444444444444"}, Severity: "high", Audience: "restricted", AccessUserIDs: []string{"55555555555555555555555555555555"}, Source: Reference{Kind: "commit", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Label: "released source"}, Configuration: Reference{Kind: "configuration", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Label: "release config"}, Infrastructure: Reference{Kind: "infrastructure", Label: "unavailable"}, Evidence: []Evidence{{Kind: "trace", Label: "sample trace", Visibility: "restricted", Sanitization: "user identifiers removed", Available: false, UnavailableReason: "trace access denied"}}}, "44444444444444444444444444444444")
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 1 || v.Evidence[0].ID == "" || len(v.History) != 1 || len(v.UnavailableContext) != 0 {
		t.Fatalf("unexpected workspace: %+v", v)
	}
	v, err = s.Update(v.RepositoryID, v.ID, "55555555555555555555555555555555", "hypothesis", "queue saturation delayed authorization callbacks", "initial correlation", 1)
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 2 || len(v.Hypotheses) != 1 || v.Hypotheses[0].CreatedBy != "55555555555555555555555555555555" {
		t.Fatalf("history was not attributable: %+v", v)
	}
	_, err = s.Update(v.RepositoryID, v.ID, "55555555555555555555555555555555", "status", "resolved", "", 1)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("wanted conflict, got %v", err)
	}
}

func TestWorkspaceRejectsUnboundedOrUnexplainedEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	start := time.Now().UTC()
	base := Workspace{RepositoryID: "11111111111111111111111111111111", Title: "x", Summary: "x", Trigger: Reference{Kind: "trace", Label: "x"}, Release: Reference{ResourceID: "22222222222222222222222222222222", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Environment: Reference{ResourceID: "33333333333333333333333333333333"}, TimeStart: start, TimeEnd: start.Add(32 * 24 * time.Hour), UserJourney: "x", OwnerIDs: []string{"44444444444444444444444444444444"}, Severity: "low", Audience: "repository", Source: Reference{Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Evidence: []Evidence{{Kind: "log", Label: "missing", Visibility: "repository", Sanitization: "redacted", Available: false}}}
	if _, err := s.Create(base, "44444444444444444444444444444444"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wanted invalid, got %v", err)
	}
}
