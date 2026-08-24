package changestacks

import "testing"

func TestStackPersistsOrderedAcceptanceBoundary(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := Stack{RepositoryID: "repo", Title: "Large outcome", Outcome: "Ship it in reviewable layers", TargetBranch: "main", Members: []Member{{ID: "storage", Title: "Storage", SourceBranch: "feature/storage", AcceptanceCriteria: []string{"round trips"}}, {ID: "api", Title: "API", SourceBranch: "feature/api", DependsOn: []string{"storage"}, AcceptanceCriteria: []string{"compatible"}}}}
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
	_, err := s.Create(Stack{RepositoryID: "repo", Title: "Outcome", Outcome: "shared", TargetBranch: "main", Members: []Member{{ID: "one", Title: "One", SourceBranch: "one"}}}, "alice")
	if err != ErrInvalid {
		t.Fatalf("missing criteria error = %v", err)
	}
}
