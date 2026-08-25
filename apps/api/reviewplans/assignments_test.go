package reviewplans

import (
	"errors"
	"fmt"
	"sync"
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

func TestReviewAssignmentAreaConflictIsScopedToExactPlan(t *testing.T) {
	store, _ := New(t.TempDir())
	first := Assignment{RequestID: "request-plan-a", RepositoryID: "repo", PullRequestID: "pull", PlanID: "plan-a", PlanVersion: 1, AreaID: "security", PrincipalType: "human", PrincipalID: "one", AssignedBy: "owner"}
	if _, err := store.CreateAssignment(first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.RequestID, second.PlanID, second.PlanVersion, second.PrincipalID = "request-plan-b", "plan-b", 2, "two"
	if _, err := store.CreateAssignment(second); err != nil {
		t.Fatalf("new plan assignment = %v", err)
	}
}

func TestIndependentStoresSerializeAssignmentMutations(t *testing.T) {
	root := t.TempDir()
	first, _ := New(root)
	second, _ := New(root)
	stores := []*Store{first, second}
	var wg sync.WaitGroup
	errorsSeen := make(chan error, 2)
	for i, store := range stores {
		wg.Add(1)
		go func(index int, current *Store) {
			defer wg.Done()
			_, err := current.CreateAssignment(Assignment{RequestID: fmt.Sprintf("request-%d-stable", index), RepositoryID: "repo", PullRequestID: "pull", PlanID: "plan", PlanVersion: 1, AreaID: fmt.Sprintf("area-%d", index), PrincipalType: "human", PrincipalID: fmt.Sprintf("reviewer-%d", index), AssignedBy: "owner"})
			errorsSeen <- err
		}(i, store)
	}
	wg.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	values, err := first.ListAssignments("repo", "pull")
	if err != nil || len(values) != 2 {
		t.Fatalf("assignments = %#v, %v", values, err)
	}
}

func TestAcceptedReviewerMayDeclineAndRequireReassignment(t *testing.T) {
	store, _ := New(t.TempDir())
	created, _ := store.CreateAssignment(Assignment{RequestID: "request-decline", RepositoryID: "repo", PullRequestID: "pull", PlanID: "plan", PlanVersion: 1, AreaID: "api", PrincipalType: "human", PrincipalID: "reviewer", AssignedBy: "owner"})
	accepted, _ := store.Transition("repo", "pull", created.ID, "reviewer", "accept", "", nil)
	declined, err := store.Transition("repo", "pull", accepted.ID, "reviewer", "decline", "cannot complete", nil)
	if err != nil || declined.Status != "declined" || declined.ActionRequired == "" {
		t.Fatalf("decline = %#v, %v", declined, err)
	}
}

func TestAcceptanceRejectsCurrentOverloadInsideMutationLock(t *testing.T) {
	store, _ := New(t.TempDir())
	var first Assignment
	for index, area := range []string{"api", "security", "privacy"} {
		created, err := store.CreateAssignment(Assignment{RequestID: fmt.Sprintf("overload-%d", index), RepositoryID: "repo", PullRequestID: "pull", PlanID: "plan", PlanVersion: 1, AreaID: area, PrincipalType: "agent", PrincipalID: "agent", AgentGrantID: "grant", AssignedBy: "owner"})
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			first = created
		}
	}
	if _, err := store.Transition("repo", "pull", first.ID, "agent", "accept", "", nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("overloaded acceptance error = %v", err)
	}
	values, _ := store.ListAssignments("repo", "pull")
	if values[0].Status != "invited" {
		t.Fatalf("overloaded assignment mutated: %#v", values[0])
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
