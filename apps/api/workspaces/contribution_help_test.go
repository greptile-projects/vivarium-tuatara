package workspaces

import "testing"

func TestContributionHelpRetainsOwnershipAndExitHistory(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CommitID: "revision", CreatorID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Source: Source{Kind: "repository"}, Definition: Definition{Version: 1, Image: "alpine:3.22"}, ContributorContext: &ContributorContext{OpportunityID: "cccccccccccccccccccccccccccccccc", Help: ContributionHelp{Version: 1, State: "active"}}}, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.AddContributionHelp(w.ID, w.CreatorID, ContributionHelpEntry{Kind: "question", Body: "Does this edge case belong in scope?", DecisionOwner: "contributor"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	entry := w.ContributorContext.Help.Entries[0]
	if entry.ActorID != w.CreatorID || entry.DecisionOwner != "contributor" || entry.Status != "open" {
		t.Fatalf("entry = %#v", entry)
	}
	if _, err = s.ResolveContributionHelp(w.ID, "mentor", entry.ID, "resolved", 1); err != ErrConflict {
		t.Fatalf("stale resolve = %v", err)
	}
	w, err = s.SetContributionState(w.ID, w.CreatorID, "exited", "scope no longer matches", 2)
	if err != nil {
		t.Fatal(err)
	}
	if w.ContributorContext.Help.State != "exited" || len(w.ContributorContext.Help.Entries) != 1 || len(w.Events) < 2 {
		t.Fatalf("exit lost help history: %#v", w.ContributorContext.Help)
	}
}
