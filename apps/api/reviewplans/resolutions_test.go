package reviewplans

import (
	"testing"
	"time"
)

func TestFindingResolutionsAreRetryStableAndBoundExceptions(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	value := FindingResolution{RequestID: "resolution-request", RepositoryID: "repo", PullRequestID: "pull", FindingID: "finding", FindingRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CandidateRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ActorType: "human", ActorID: "owner", Action: "resolved", Rationale: "The retry now reconciles before validation.", Links: []ResolutionLink{{Kind: "commit", ResourceID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}, Evidence: []WorkCitation{{Kind: "check", Value: "run"}}}
	created, err := store.CreateFindingResolution(value)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := store.CreateFindingResolution(value)
	if err != nil || retried.ID != created.ID {
		t.Fatalf("retry = %#v, %v", retried, err)
	}
	value.Rationale = "changed"
	if _, err = store.CreateFindingResolution(value); err != ErrResolutionConflict {
		t.Fatalf("changed retry = %v", err)
	}
	expiry := now.Add(31 * 24 * time.Hour)
	value.RequestID = "exception-request"
	value.Action = "exception"
	value.ExpiresAt = &expiry
	if _, err = store.CreateFindingResolution(value); err != ErrInvalid {
		t.Fatalf("unbounded exception = %v", err)
	}
	value.RequestID = "agent-decision"
	value.Action = "accepted_risk"
	value.ActorType = "agent"
	value.ExpiresAt = nil
	if _, err = store.CreateFindingResolution(value); err != ErrInvalid {
		t.Fatalf("agent owner decision = %v", err)
	}
}
