package changestacks

import (
	"errors"
	"strings"
	"testing"
)

func TestStackPersistsOrderedAcceptanceBoundary(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := Stack{RequestID: "request", RequestDigest: "digest", RepositoryID: "repo", Title: "Large outcome", Outcome: "Ship it in reviewable layers", TargetBranch: "main", Members: []Member{{ID: "storage", Title: "Storage", SourceBranch: "feature/storage", AcceptanceCriteria: []string{"round trips"}}, {ID: "api", Title: "API", SourceBranch: "feature/api", DependsOn: []string{"storage"}, AcceptanceCriteria: []string{"compatible"}}}}
	created, err := s.Create(in, "alice")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("repo", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Members[0].Position != 1 || got.Members[1].Position != 2 || got.Members[1].DependsOn[0] != "storage" {
		t.Fatalf("order was not retained: %#v", got.Members)
	}
	if got.Authority == "" {
		t.Fatal("authority boundary missing")
	}
}

func TestStackWorkFreezesContextAndRejectsStaleInputs(t *testing.T) {
	s, _ := New(t.TempDir())
	created, err := s.Create(Stack{RequestID: "stack-work", RequestDigest: "digest", RepositoryID: "repo", Title: "Parallel delivery", Outcome: "Ship the shared outcome", TargetBranch: "main", Members: []Member{
		{ID: "base", Title: "Foundation", SourceRepositoryID: "repo", SourceBranch: "base", Revision: strings.Repeat("1", 40), BaseRevision: strings.Repeat("0", 40), AcceptanceCriteria: []string{"foundation passes"}},
		{ID: "layer", Title: "Dependent", SourceRepositoryID: "repo", SourceBranch: "layer", Revision: strings.Repeat("2", 40), BaseRevision: strings.Repeat("1", 40), DependsOn: []string{"base"}, AcceptanceCriteria: []string{"journey passes"}, PullRequestID: "pull-layer"},
	}}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	_, assignment, err := s.Assign("repo", created.ID, "layer", Assignment{PrincipalType: "agent", PrincipalID: "agent", OperatorID: "owner", AccessGrantID: "grant", AssignedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	_, launch, err := s.OpenWork("repo", created.ID, WorkLaunch{RequestID: "launch", RequestDigest: "launch-digest", MemberID: "layer", Kind: "shared_workspace", AssignmentID: assignment.ID, OpenedBy: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	if launch.Outcome != created.Outcome || launch.ParentRevision != strings.Repeat("1", 40) || launch.UpstreamRevisions["base"] != strings.Repeat("1", 40) || len(launch.AcceptanceCriteria) != 1 || len(launch.Evidence) != 2 {
		t.Fatalf("launch did not freeze context: %#v", launch)
	}
	_, event, err := s.AppendTimeline("repo", created.ID, TimelineEvent{RequestID: "checkpoint", RequestDigest: "event-digest", MemberID: "layer", Kind: "checkpoint", Summary: "Tests pass", Revision: strings.Repeat("2", 40), WorkLaunchID: launch.ID, ActorID: "agent", ActorType: "agent"})
	if err != nil || event.UpstreamRevisions["base"] != strings.Repeat("1", 40) {
		t.Fatalf("event %#v: %v", event, err)
	}
	_, replacement, _ := s.Assign("repo", created.ID, "layer", Assignment{PrincipalType: "human", PrincipalID: "next", AssignedBy: "owner"})
	if _, _, err = s.OpenWork("repo", created.ID, WorkLaunch{RequestID: "stale", RequestDigest: "stale-digest", MemberID: "layer", Kind: "change_session", AssignmentID: assignment.ID, OpenedBy: "agent"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("stale assignment admitted: %v", err)
	}
	if _, _, err = s.AppendTimeline("repo", created.ID, TimelineEvent{RequestID: "moved", RequestDigest: "moved-digest", MemberID: "layer", Kind: "question", Summary: "Current?", Revision: strings.Repeat("3", 40), ActorID: replacement.PrincipalID, ActorType: "human"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("moved revision admitted: %v", err)
	}
}

func TestStackRejectsMissingPerChangeCriteria(t *testing.T) {
	s, _ := New(t.TempDir())
	_, err := s.Create(Stack{RequestID: "request", RequestDigest: "digest", RepositoryID: "repo", Title: "Outcome", Outcome: "shared", TargetBranch: "main", Members: []Member{{ID: "one", Title: "One", SourceBranch: "one"}}}, "alice")
	if err != ErrInvalid {
		t.Fatalf("missing criteria error = %v", err)
	}
}

func TestStackReconcilesStablePublicationRequest(t *testing.T) {
	s, _ := New(t.TempDir())
	in := Stack{RequestID: "stable", RequestDigest: "same", RepositoryID: "repo", Title: "Outcome", Outcome: "shared", TargetBranch: "main", Members: []Member{{ID: "one", Title: "One", SourceBranch: "one", AcceptanceCriteria: []string{"passes"}}}}
	first, err := s.Create(in, "alice")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(in, "alice")
	if err != nil || second.ID != first.ID {
		t.Fatalf("retry = %#v, %v; want %s", second, err, first.ID)
	}
	in.RequestDigest = "changed"
	if _, err = s.Create(in, "alice"); err != ErrInvalid {
		t.Fatalf("changed reuse = %v", err)
	}
}

func TestOwnerAcknowledgementFreezesLayerAndUpstreamRevisions(t *testing.T) {
	s, _ := New(t.TempDir())
	in := Stack{RequestID: "ack", RequestDigest: "digest", RepositoryID: "repo", Title: "Outcome", Outcome: "shared", TargetBranch: "main", Members: []Member{{ID: "one", Title: "One", SourceBranch: "one", Revision: "1111111111111111111111111111111111111111", AcceptanceCriteria: []string{"one passes"}}, {ID: "two", Title: "Two", SourceBranch: "two", Revision: "2222222222222222222222222222222222222222", AcceptanceCriteria: []string{"two passes"}}}}
	created, err := s.Create(in, "author")
	if err != nil {
		t.Fatal(err)
	}
	acknowledged, err := s.Acknowledge("repo", created.ID, "two", "owner", "acknowledged", "Reviewed with layer one")
	if err != nil {
		t.Fatal(err)
	}
	a := acknowledged.Members[1].Acknowledgements[0]
	if a.Revision != in.Members[1].Revision || a.UpstreamRevisions["one"] != in.Members[0].Revision || a.OwnerID != "owner" {
		t.Fatalf("acknowledgement lost exact context: %#v", a)
	}
	if _, err = s.Acknowledge("repo", created.ID, "two", "owner", "approved", ""); err != ErrInvalid {
		t.Fatalf("unsupported decision = %v", err)
	}
}

func TestRestackIsRetryStableAndPreservesRevisionLineage(t *testing.T) {
	s, _ := New(t.TempDir())
	old := strings.Repeat("1", 40)
	next := strings.Repeat("2", 40)
	created, err := s.Create(Stack{RequestID: "stack", RequestDigest: "stack-digest", RepositoryID: "repo", Title: "Outcome", Outcome: "shared", TargetBranch: "main", Members: []Member{{ID: "one", Title: "One", SourceBranch: "one", Revision: old, BaseRevision: strings.Repeat("0", 40), AcceptanceCriteria: []string{"passes"}}}}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	proposal := Restack{RequestID: "restack", RequestDigest: "restack-digest", CreatedBy: "alice", Members: []RestackMember{{Member: created.Members[0], ExpectedBranchTip: old, CandidateRevision: next}}}
	_, first, err := s.ProposeRestack("repo", created.ID, proposal)
	if err != nil {
		t.Fatal(err)
	}
	_, retry, err := s.ProposeRestack("repo", created.ID, proposal)
	if err != nil || retry.ID != first.ID {
		t.Fatalf("retry = %#v, %v", retry, err)
	}
	member := created.Members[0]
	member.Revision = next
	updated, _, err := s.ApplyRestack("repo", created.ID, first.ID, "alice", []Member{member})
	if err != nil {
		t.Fatal(err)
	}
	lineage := updated.RevisionLineage["one"]
	if len(lineage) != 1 || lineage[0].Revision != old || lineage[0].SucceededBy != next || lineage[0].ChangedBy != "alice" {
		t.Fatalf("lineage = %#v", lineage)
	}
}

func TestApplyRestackRejectsDanglingAndCyclicDependencies(t *testing.T) {
	s, _ := New(t.TempDir())
	baseMembers := []Member{{ID: "one", Title: "One", SourceBranch: "one", Revision: strings.Repeat("1", 40), AcceptanceCriteria: []string{"one"}}, {ID: "two", Title: "Two", SourceBranch: "two", Revision: strings.Repeat("2", 40), DependsOn: []string{"one"}, AcceptanceCriteria: []string{"two"}}}
	created, err := s.Create(Stack{RequestID: "stack-graph", RequestDigest: "stack-graph-digest", RepositoryID: "repo", Title: "Outcome", Outcome: "shared", TargetBranch: "main", Members: baseMembers}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	_, proposal, err := s.ProposeRestack("repo", created.ID, Restack{RequestID: "restack-graph", RequestDigest: "restack-graph-digest", CreatedBy: "alice", Members: []RestackMember{{Member: baseMembers[0]}, {Member: baseMembers[1]}}})
	if err != nil {
		t.Fatal(err)
	}
	dangling := append([]Member(nil), baseMembers...)
	dangling[1].DependsOn = []string{"removed"}
	if _, _, err = s.ApplyRestack("repo", created.ID, proposal.ID, "alice", dangling); err != ErrInvalid {
		t.Fatalf("dangling dependency = %v", err)
	}
	cyclic := append([]Member(nil), baseMembers...)
	cyclic[0].DependsOn = []string{"two"}
	if _, _, err = s.ApplyRestack("repo", created.ID, proposal.ID, "alice", cyclic); err != ErrInvalid {
		t.Fatalf("cyclic dependency = %v", err)
	}
}

func TestSequentialDistinctRestackRequestIDsDoNotReconcile(t *testing.T) {
	s, _ := New(t.TempDir())
	member := Member{ID: "one", Title: "One", SourceBranch: "one", Revision: strings.Repeat("1", 40), AcceptanceCriteria: []string{"passes"}}
	created, err := s.Create(Stack{RequestID: "stack-sequential", RequestDigest: "stack-digest", RepositoryID: "repo", Title: "Outcome", Outcome: "shared", TargetBranch: "main", Members: []Member{member}}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	first := Restack{RequestID: "removed-reference", RequestDigest: "first-digest", CreatedBy: "alice", Members: []RestackMember{{Member: member}}}
	second := Restack{RequestID: "cycle-reference", RequestDigest: "second-digest", CreatedBy: "alice", Members: []RestackMember{{Member: member}}}
	_, one, err := s.ProposeRestack("repo", created.ID, first)
	if err != nil {
		t.Fatal(err)
	}
	_, two, err := s.ProposeRestack("repo", created.ID, second)
	if err != nil {
		t.Fatal(err)
	}
	if one.ID == two.ID || one.RequestID == two.RequestID {
		t.Fatalf("distinct requests reconciled: %#v %#v", one, two)
	}
}
