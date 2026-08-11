package workspaces

import (
	"errors"
	"testing"
)

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

func TestContributionAgentActionRetainsExactAuthority(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: "owner", Source: Source{Kind: "repository"}, Definition: Definition{Version: 1, Image: "alpine"}, ContributorContext: &ContributorContext{Help: ContributionHelp{Version: 1, State: "active"}}}, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.AddContributionHelp(w.ID, "owner", ContributionHelpEntry{Kind: "agent_action", Action: "edit", AgentID: "agent", Body: "Adjusted the bounded file.", DecisionOwner: "contributor"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.ContributorContext.Help.Entries[0].Action; got != "edit" {
		t.Fatalf("action = %q", got)
	}
	if got := w.Events[len(w.Events)-1].Detail; got != w.ContributorContext.Help.Entries[0].ID+":edit" {
		t.Fatalf("event detail = %q", got)
	}
}

func TestContributionHumanEntryRejectsAgentAuthorityMetadata(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: "owner", Source: Source{Kind: "repository"}, Definition: Definition{Version: 1, Image: "alpine"}, ContributorContext: &ContributorContext{Help: ContributionHelp{Version: 1, State: "active"}}}, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddContributionHelp(w.ID, "owner", ContributionHelpEntry{Kind: "question", Action: "edit", AgentID: "spoofed-agent", Body: "Human question", DecisionOwner: "contributor"}, 1)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("spoofed agent metadata = %v", err)
	}
	retained, err := s.Get(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(retained.ContributorContext.Help.Entries) != 0 {
		t.Fatalf("spoofed entry retained: %#v", retained.ContributorContext.Help.Entries)
	}
}
