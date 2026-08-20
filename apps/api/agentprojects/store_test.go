package agentprojects

import (
	"errors"
	"testing"
)

func TestProjectVersionsIntentAndDerivesReviewBoundary(t *testing.T) {
	s, _ := New(t.TempDir())
	r := Revision{Title: "Review helper", Purpose: "Review changes", Sources: []Source{{ID: "prompt", Kind: "prompt", RepositoryID: "repo", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Path: "agents/review.md", Purpose: "operating prompt"}}, Tools: []Tool{{Name: "git", Purpose: "inspect changes", Actions: []string{"read"}, Boundary: "repository read only"}}, Models: []Model{{Provider: "openai", Name: "codex", Version: "1", Purpose: "reasoning"}}, SupportedTasks: []string{"review pull requests"}, ExpectedOutputs: []string{"cited findings"}, ProhibitedActions: []string{"read"}, MemoryPolicy: "session only", DataUseTerms: "repository content only", Guarantees: []string{"finds every bug"}, Budget: Budget{MaxCostUSD: 1, MaxTokens: 1000, MaxToolActions: 4, MaxRuntimeSeconds: 60}, Escalations: []Escalation{{Trigger: "uncertainty", Action: "stop and ask"}}, DeploymentBoundaries: []DeploymentBoundary{{Environment: "isolated", RepositoryAccess: "selected repository", NetworkAccess: "none", ApprovalRequired: true}}, ChangeSummary: "initial intent"}
	p, err := s.Create("repo", "author", r)
	if err != nil {
		t.Fatal(err)
	}
	if p.CurrentVersion != 1 || len(p.Diagnostics) != 4 || !p.EffectiveCapability.HumanEscalationRequired {
		t.Fatalf("projection = %#v", p)
	}
	r.ProhibitedActions = []string{"write"}
	r.Guarantees = nil
	r.OwnerIDs = []string{"owner"}
	r.Escalations[0].OwnerIDs = []string{"owner"}
	r.ChangeSummary = "resolve review blockers"
	p, err = s.Revise(p.ID, 1, "author", r)
	if err != nil || p.CurrentVersion != 2 || len(p.Revisions) != 2 || len(p.Diagnostics) != 0 {
		t.Fatalf("revision = %#v, %v", p, err)
	}
	if _, err = s.Revise(p.ID, 1, "author", r); err != ErrConflict {
		t.Fatalf("conflict = %v", err)
	}
}

func TestCommittedProjectReportsPostRenameDurabilityUncertaintyAsSuccess(t *testing.T) {
	s, _ := New(t.TempDir())
	s.syncDirectory = func(string) error { return errors.New("injected directory sync failure") }
	r := Revision{Title: "Review helper", Purpose: "Review changes", Sources: []Source{{ID: "prompt", Kind: "prompt", RepositoryID: "repo", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Path: "agent.md", Purpose: "prompt"}}, Models: []Model{{Provider: "openai", Name: "codex", Version: "1", Purpose: "reasoning"}}, SupportedTasks: []string{"review"}, ExpectedOutputs: []string{"findings"}, ProhibitedActions: []string{"write"}, MemoryPolicy: "session", DataUseTerms: "repository only", Budget: Budget{MaxTokens: 1, MaxRuntimeSeconds: 1}, ChangeSummary: "initial"}
	p, err := s.Create("repo", "author", r)
	if err != nil || !p.DurabilityUncertain {
		t.Fatalf("create = %#v, %v", p, err)
	}
	persisted, err := s.Get(p.ID)
	if err != nil || persisted.ID != p.ID {
		t.Fatalf("committed ledger = %#v, %v", persisted, err)
	}
}
