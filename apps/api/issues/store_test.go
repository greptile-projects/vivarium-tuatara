package issues

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCommittedWriteReportsDirectoryDurabilityUncertainty(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("directory sync failed")
	store.directorySync = func(string) error { return injected }
	created, err := store.Create(Issue{RepositoryID: "repository", ReporterID: "reporter", Title: "Failure", ExpectedBehavior: "works", ObservedBehavior: "fails", Severity: "medium", Environment: "Linux", ReproductionSteps: []string{"run"}, Visibility: "repository"})
	if !errors.Is(err, ErrDurabilityUncertain) || !errors.Is(err, injected) || created.ID == "" {
		t.Fatalf("create = %#v, %v", created, err)
	}
	store.directorySync = syncDirectory
	reloaded, err := store.Get("repository", created.ID)
	if err != nil || reloaded.ID != created.ID {
		t.Fatalf("visible committed issue = %#v, %v", reloaded, err)
	}
}

func TestReproductionAttemptIsRetainedAsImmutableEvidence(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	issue, err := store.Create(Issue{RepositoryID: "repository", ReporterID: "reporter", Title: "Failure", ExpectedBehavior: "works", ObservedBehavior: "fails", Severity: "medium", Environment: "Linux", ReproductionSteps: []string{"run"}, Visibility: "repository"})
	if err != nil {
		t.Fatal(err)
	}
	attempt := ReproductionAttempt{WorkspaceID: "workspace", CommitID: strings.Repeat("a", 40), DefinitionSHA256: strings.Repeat("b", 64), EnvironmentDefinition: json.RawMessage(`{"version":1,"image":"alpine"}`), Commands: []ReproductionCommand{{Name: "reproduce", OutcomeID: "outcome", CommandSHA256: strings.Repeat("c", 64), ExitCode: 1, Log: "observed failure"}}, ObservedResult: "process exited with the reported failure", Result: "reproduced"}
	updated, err := store.AddReproductionAttempt(issue.RepositoryID, issue.ID, "maintainer", attempt)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.ReproductionAttempts) != 1 || updated.ReproductionAttempts[0].ID == "" || updated.ReproductionAttempts[0].ReproducedBy != "maintainer" || updated.ReproductionAttempts[0].Commands[0].Log != "observed failure" {
		t.Fatalf("attempt = %#v", updated.ReproductionAttempts)
	}
	if attempt.ID != "" || attempt.ReproducedBy != "" {
		t.Fatal("caller-owned evidence was mutated")
	}
	reloaded, err := store.Get(issue.RepositoryID, issue.ID)
	if err != nil || len(reloaded.ReproductionAttempts) != 1 {
		t.Fatalf("reloaded = %#v, %v", reloaded, err)
	}
}

func TestRepairVerificationRetainsReporterDissentAndOverride(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	issue, err := store.Create(Issue{RepositoryID: "repository", ReporterID: "reporter", Title: "Failure", ExpectedBehavior: "works", ObservedBehavior: "fails", Severity: "medium", Environment: "Linux", ReproductionSteps: []string{"run"}, Visibility: "repository"})
	if err != nil {
		t.Fatal(err)
	}
	verification := RepairVerification{ID: NewEvidenceID(), PullRequestID: strings.Repeat("1", 32), CandidateCommitID: strings.Repeat("a", 40), ReproductionAttemptID: strings.Repeat("2", 32), DefinitionSHA256: strings.Repeat("b", 64), InputSHA256s: []string{strings.Repeat("c", 64)}, AcceptanceCriteria: []string{"upload completes"}, Decisions: []ResolutionDecision{{ID: NewEvidenceID(), Kind: "rejected", ActorID: "reporter", CommitID: strings.Repeat("a", 40), Rationale: "the original behavior remains"}}}
	updated, err := store.Mutate(issue.RepositoryID, issue.ID, "owner", issue.Version, func(v *Issue) error {
		verification.Decisions = append(verification.Decisions, ResolutionDecision{ID: NewEvidenceID(), Kind: "maintainer_override", ActorID: "owner", CommitID: verification.CandidateCommitID, Rationale: "release is blocked by separate evidence"})
		v.RepairVerifications = append(v.RepairVerifications, verification)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	decisions := updated.RepairVerifications[0].Decisions
	if len(decisions) != 2 || decisions[0].Kind != "rejected" || decisions[1].Kind != "maintainer_override" {
		t.Fatalf("decisions = %#v", decisions)
	}
	reloaded, err := store.Get(issue.RepositoryID, issue.ID)
	if err != nil || len(reloaded.RepairVerifications) != 1 || len(reloaded.RepairVerifications[0].InputSHA256s) != 1 {
		t.Fatalf("reloaded = %#v, %v", reloaded, err)
	}
}
