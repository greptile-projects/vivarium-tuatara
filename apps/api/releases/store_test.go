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

func TestCandidateFreezesIncludedPullRevisionAndPaths(t *testing.T) {
	store, _ := New(t.TempDir())
	repositoryID, actorID := "0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789"
	pullID, source := "11111111111111111111111111111111", "2222222222222222222222222222222222222222"
	created, err := store.Create(Candidate{RepositoryID: repositoryID, Version: "v2", Notes: "Frozen pull evidence", CommitID: "0123456789abcdef0123456789abcdef01234567", CreatedBy: actorID, Inclusions: Inclusion{PullRequestIDs: []string{pullID}, PullEvidence: []PullEvidence{{PullRequestID: pullID, SourceCommitID: source, ChangedPaths: []string{"ui/settings.tsx"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(repositoryID, created.ID)
	if err != nil || len(got.Inclusions.PullEvidence) != 1 || got.Inclusions.PullEvidence[0].SourceCommitID != source || got.Inclusions.PullEvidence[0].ChangedPaths[0] != "ui/settings.tsx" {
		t.Fatalf("frozen evidence = %#v, %v", got.Inclusions.PullEvidence, err)
	}
}
