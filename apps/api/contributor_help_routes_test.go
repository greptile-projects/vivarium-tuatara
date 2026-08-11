package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestCurrentContributionMentorRejectsRemovedPublicCollaborator(t *testing.T) {
	git, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repos, err := repositories.New(t.TempDir(), git)
	if err != nil {
		t.Fatal(err)
	}
	owner, mentor := "0123456789abcdef0123456789abcdef", "abcdefabcdefabcdefabcdefabcdefab"
	repo, err := repos.Create(owner, "public-help")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repos.SetVisibility(owner, repo.ID, repositories.Public); err != nil {
		t.Fatal(err)
	}
	if _, err = repos.AddCollaborator(owner, repo.ID, mentor); err != nil {
		t.Fatal(err)
	}
	if !currentContributionMentor(repos, repo.ID, mentor) {
		t.Fatal("current mentor rejected")
	}
	if err = repos.RemoveCollaborator(owner, repo.ID, mentor); err != nil {
		t.Fatal(err)
	}
	if currentContributionMentor(repos, repo.ID, mentor) {
		t.Fatal("removed mentor retained authority on public repository")
	}
}
