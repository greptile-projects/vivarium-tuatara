package changestacks

import "testing"

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
