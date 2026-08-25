package reviewplans

import "testing"

func TestReviewWorkIsExactRetryStableAndAgentsCannotDecide(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := WorkEntry{RequestID: "work-request-1", RepositoryID: "repo", PullRequestID: "pull", PlanID: "plan", PlanVersion: 2, AreaID: "security", SourceRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TargetRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ActorType: "human", ActorID: "reviewer", Kind: "finding", Conclusion: "blocking", Body: "The guard is missing.", Uncertainty: "Runtime behavior remains untested.", Citations: []WorkCitation{{Kind: "file", Value: "auth.go"}}}
	created, err := store.createWork(base)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := store.createWork(base)
	if err != nil || retried.ID != created.ID {
		t.Fatalf("retry = %#v, %v", retried, err)
	}
	changed := base
	changed.Body = "Different"
	if _, err = store.createWork(changed); err != ErrWorkConflict {
		t.Fatalf("changed retry error = %v", err)
	}
	agent := base
	agent.RequestID = "work-request-2"
	agent.ActorType = "agent"
	agent.ActorID = "audit-agent"
	agent.Kind = "decision"
	if _, err = store.createWork(agent); err != ErrInvalid {
		t.Fatalf("agent decision error = %v", err)
	}
}

func TestReviewWorkRequiresCitedPublicKindsAndValidHandoff(t *testing.T) {
	store, _ := New(t.TempDir())
	base := WorkEntry{RequestID: "work-request-3", RepositoryID: "repo", PullRequestID: "pull", PlanID: "plan", PlanVersion: 1, AreaID: "api", SourceRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TargetRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ActorType: "human", ActorID: "reviewer", Kind: "handoff", Body: "Please investigate this edge.", Citations: []WorkCitation{{Kind: "secret", Value: "vault://private"}}}
	if _, err := store.createWork(base); err != ErrInvalid {
		t.Fatalf("private citation error = %v", err)
	}
	base.Citations = []WorkCitation{{Kind: "requirement", Value: "Does retry reconcile?"}}
	if _, err := store.createWork(base); err != ErrInvalid {
		t.Fatalf("recipient-less handoff error = %v", err)
	}
	base.RecipientType, base.RecipientID = "human", "reviewer-2"
	if _, err := store.createWork(base); err != nil {
		t.Fatal(err)
	}
}

func TestAssignedReviewWorkRechecksAcceptedAssignmentInsidePersistenceLock(t *testing.T) {
	store, _ := New(t.TempDir())
	assignment, err := store.CreateAssignment(Assignment{RequestID: "assignment-request", RepositoryID: "repo", PullRequestID: "pull", PlanID: "plan", PlanVersion: 1, AreaID: "api", PrincipalType: "human", PrincipalID: "reviewer", AssignedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	assignment, err = store.Transition("repo", "pull", assignment.ID, "reviewer", "accept", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	work := WorkEntry{RequestID: "assigned-work-1", RepositoryID: "repo", PullRequestID: "pull", PlanID: "plan", PlanVersion: 1, AreaID: "api", SourceRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TargetRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ActorType: "human", ActorID: "reviewer", Kind: "finding", Body: "The retry boundary is sound."}
	if _, err = store.CreateAssignedWork(work, assignment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition("repo", "pull", assignment.ID, "owner", "release", "responsibility moved", nil); err != nil {
		t.Fatal(err)
	}
	work.RequestID = "assigned-work-2"
	if _, err = store.CreateAssignedWork(work, assignment.ID); err != ErrWorkConflict {
		t.Fatalf("released assignment persisted work: %v", err)
	}
	entries, _ := store.ListWork("repo", "pull")
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
}
