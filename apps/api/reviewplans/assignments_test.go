package reviewplans

import (
	"errors"
	"testing"
	"time"
)

func TestReviewAssignmentLifecycleIsAreaBoundedAndRetryStable(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	in := Assignment{RequestID: "request-123", RepositoryID: "repo", PullRequestID: "pull", PlanID: "plan", PlanVersion: 2, AreaID: "security", PrincipalType: "human", PrincipalID: "reviewer", Deadline: &deadline, EscalationPath: "owner", AssignedBy: "owner"}
	created, err := store.CreateAssignment(in)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := store.CreateAssignment(in)
	if err != nil || retried.ID != created.ID {
		t.Fatalf("retry = %#v, %v", retried, err)
	}
	if created.Status != "invited" || created.Authority == "" {
		t.Fatalf("unbounded assignment: %#v", created)
	}
	accepted, err := store.Transition("repo", "pull", created.ID, "reviewer", "accept", "", nil)
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("accept = %#v, %v", accepted, err)
	}
	recused, err := store.Transition("repo", "pull", created.ID, "reviewer", "recuse", "authored related design", nil)
	if err != nil || recused.Status != "recused" || recused.ActionRequired == "" {
		t.Fatalf("recuse = %#v, %v", recused, err)
	}
}

func TestReviewAssignmentRejectsChangedRetryAndOverlappingAccountability(t *testing.T) {
	store, _ := New(t.TempDir())
	base := Assignment{RequestID: "request-123", RepositoryID: "repo", PullRequestID: "pull", PlanID: "plan", PlanVersion: 1, AreaID: "api", PrincipalType: "human", PrincipalID: "one", AssignedBy: "owner"}
	if _, err := store.CreateAssignment(base); err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.PrincipalID = "two"
	if _, err := store.CreateAssignment(changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed retry error = %v", err)
	}
	overlap := changed
	overlap.RequestID = "request-456"
	if _, err := store.CreateAssignment(overlap); !errors.Is(err, ErrConflict) {
		t.Fatalf("overlap error = %v", err)
	}
}
