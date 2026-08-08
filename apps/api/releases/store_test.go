package releases

import (
	"errors"
	"testing"
)

func TestCandidatesPersistImmutableAttributionAndUniqueVersions(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	repositoryID, actorID := "0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789"
	created, err := store.Create(Candidate{RepositoryID: repositoryID, Version: " v1.0.0 ", Notes: " First release ", CommitID: "0123456789abcdef0123456789abcdef01234567", CreatedBy: actorID, Inclusions: Inclusion{PullRequestIDs: []string{"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, ContributorIDs: []string{actorID}}})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "candidate" || created.Version != "v1.0.0" || created.Notes != "First release" || created.CreatedBy != actorID || created.Inclusions.PullRequestIDs[0] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("candidate = %#v", created)
	}
	reopened, _ := New(root)
	got, err := reopened.Get(repositoryID, created.ID)
	if err != nil || got.CommitID != created.CommitID {
		t.Fatalf("reopened = %#v, %v", got, err)
	}
	_, err = reopened.Create(Candidate{RepositoryID: repositoryID, Version: "V1.0.0", Notes: "duplicate", CommitID: "0123456789abcdef0123456789abcdef01234567", CreatedBy: actorID})
	if !errors.Is(err, ErrVersionExists) {
		t.Fatalf("duplicate error = %v", err)
	}
}
