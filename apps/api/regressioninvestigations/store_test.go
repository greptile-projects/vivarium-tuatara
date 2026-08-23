package regressioninvestigations

import (
	"errors"
	"os"
	"strings"
	"testing"
)

const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func validInvestigation() Investigation {
	return Investigation{RequestID: "request-1", RepositoryID: "repo-1", Title: "Checkout regression", Source: Reference{Kind: "issue", ResourceID: "issue-1", Label: "Reported checkout failure"}, ExpectedBehavior: "Checkout completes once.", RegressedBehavior: "Checkout submits twice.", KnownGood: Boundary{Kind: "commit", Revision: commit, Label: "last known good"}, KnownBad: Boundary{Kind: "commit", Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Label: "first known bad"}, Environments: []string{"staging"}, Severity: "high", OwnerIDs: []string{"owner-1"}, AcceptanceCriteria: []string{"The boundary reproduces the behavior difference."}, Comparable: true, Evidence: []Evidence{{Kind: "issue", ResourceID: "issue-1", Label: "report", Visibility: "repository", Available: true}}}
}

func TestStoreRetainsAttributedCASHistory(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create(validInvestigation(), "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 1 || v.Status != "open" || len(v.History) != 1 || v.Evidence[0].ID == "" {
		t.Fatalf("unexpected creation: %#v", v)
	}
	updated, err := s.Append(v.RepositoryID, v.ID, "owner-1", "hypothesis", "The serializer changed inside this boundary.", "", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.History[1].ActorID != "owner-1" || updated.History[1].Kind != "hypothesis" {
		t.Fatalf("history was not attributed: %#v", updated.History)
	}
	if _, err = s.Append(v.RepositoryID, v.ID, "actor-1", "discussion", "stale update", "", v.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestStoreRetainsScopeAndStatusChanges(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Create(validInvestigation(), "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Append(v.RepositoryID, v.ID, "owner-1", "scope_change", "Production is affected too.", "staging, production", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Append(v.RepositoryID, v.ID, "owner-1", "status_change", "The search boundary is agreed.", "bounded", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "bounded" || len(v.Environments) != 2 || v.History[1].From != "staging" || v.History[2].From != "open" {
		t.Fatalf("changes were not retained: %#v", v)
	}
}

func TestStoreRejectsIncompleteOrCredentialShapedContext(t *testing.T) {
	s, _ := New(t.TempDir())
	v := validInvestigation()
	v.AcceptanceCriteria = nil
	if _, err := s.Create(v, "actor-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want invalid, got %v", err)
	}
	v = validInvestigation()
	v.ExpectedBehavior = "token=do-not-retain"
	if _, err := s.Create(v, "actor-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want invalid sensitive context, got %v", err)
	}
}

func TestCreateRetryReconcilesPublishedRecordAfterDirectorySyncFailure(t *testing.T) {
	s, _ := New(t.TempDir())
	in := validInvestigation()
	s.syncDirectory = func(*os.File) error { return errors.New("injected directory sync failure") }
	published, err := s.Create(in, "actor-1")
	if err == nil || published.ID == "" {
		t.Fatalf("want published record and durability error, got %#v, %v", published, err)
	}
	if _, err = s.Get(in.RepositoryID, published.ID); err != nil {
		t.Fatalf("published record is not readable: %v", err)
	}
	s.syncDirectory = func(d *os.File) error { return d.Sync() }
	reconciled, err := s.Create(in, "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.ID != published.ID {
		t.Fatalf("retry duplicated create: %s != %s", reconciled.ID, published.ID)
	}
	items, err := s.List(in.RepositoryID)
	if err != nil || len(items) != 1 {
		t.Fatalf("want one retained investigation, got %#v, %v", items, err)
	}
}

func TestCreateRejectsChangedRequestIdentityReuse(t *testing.T) {
	s, _ := New(t.TempDir())
	in := validInvestigation()
	if _, err := s.Create(in, "actor-1"); err != nil {
		t.Fatal(err)
	}
	in.Title = "Different boundary"
	if _, err := s.Create(in, "actor-1"); !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestReconcileFindsPublishedRequestBeforeMutableValidation(t *testing.T) {
	s, _ := New(t.TempDir())
	in := validInvestigation()
	published, err := s.Create(in, "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	reconciled, found, err := s.Reconcile(in, "actor-1")
	if err != nil || !found || reconciled.ID != published.ID {
		t.Fatalf("reconciliation = %#v, %v, %v", reconciled, found, err)
	}
}

func TestCreateNormalizesOmittedEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	in := validInvestigation()
	in.Evidence = nil
	v, err := s.Create(in, "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	if v.Evidence == nil {
		t.Fatal("evidence must serialize as an empty collection")
	}
}

func TestScenarioAndHistoricalAttemptRetainFrozenComparisonEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Create(validInvestigation(), "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	scenario := Scenario{Name: "Checkout comparison", Environment: ScenarioEnvironment{Image: "alpine:3.22", WorkingDirectory: ".", SetupCommand: "./setup-old-revision", Command: "./compare", TimeoutSeconds: 120, CPUs: 1, MemoryMB: 256, StorageMB: 64}, EnvironmentVariants: []EnvironmentVariant{{Revision: strings.Repeat("b", 40), Environment: ScenarioEnvironment{Image: "alpine:3.22", WorkingDirectory: ".", SetupCommand: "./setup-new-revision", Command: "./compare", TimeoutSeconds: 120, CPUs: 1, MemoryMB: 256, StorageMB: 64}}}, AcceptanceCriteria: []string{"Exit zero means expected behavior"}}
	v, err = s.AddScenario(v.RepositoryID, v.ID, "owner-1", scenario, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Scenarios) != 1 || len(v.Scenarios[0].EnvironmentVariants) != 1 || v.Scenarios[0].CreatedBy != "owner-1" || v.Version != 2 {
		t.Fatalf("scenario not retained: %#v", v)
	}
	attempt := Attempt{ScenarioID: v.Scenarios[0].ID, TargetKind: "commit", Revision: commit, Dependencies: []Dependency{{Name: "runtime", RepositoryID: "dependency-repo", Revision: strings.Repeat("c", 40), Path: "dependencies/runtime"}}, Environment: v.Scenarios[0].Environment, Command: "./setup-old-revision && ./compare", Classification: "flaky", CostComputeSeconds: 2.5, Runs: []AttemptRun{{RunID: "run-1", State: "succeeded", DurationMS: 1000, Artifacts: []AttemptArtifact{}}, {RunID: "run-2", State: "failed", Failure: "exit status 1", DurationMS: 1500, Artifacts: []AttemptArtifact{{Path: "result.json", SHA256: strings.Repeat("d", 64), Size: 12}}}}}
	v, err = s.RecordAttempt(v.RepositoryID, v.ID, "agent-1", attempt, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Attempts) != 1 || v.Attempts[0].Classification != "flaky" || v.Attempts[0].RequestedBy != "agent-1" || len(v.Attempts[0].Runs) != 2 {
		t.Fatalf("attempt not retained: %#v", v.Attempts)
	}
	if _, err = s.RecordAttempt(v.RepositoryID, v.ID, "agent-1", attempt, 2); !errors.Is(err, ErrConflict) {
		t.Fatalf("want stale attempt conflict, got %v", err)
	}
}

func TestScenarioRejectsUnboundedEnvironment(t *testing.T) {
	s, _ := New(t.TempDir())
	v, _ := s.Create(validInvestigation(), "actor-1")
	scenario := Scenario{Name: "Unbounded", Environment: ScenarioEnvironment{Image: "alpine:3.22", WorkingDirectory: ".", Command: "./compare", TimeoutSeconds: 7200, CPUs: 1, MemoryMB: 256, StorageMB: 64}, AcceptanceCriteria: []string{"Compare"}}
	if _, err := s.AddScenario(v.RepositoryID, v.ID, "owner-1", scenario, v.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want invalid unbounded scenario, got %v", err)
	}
}

func TestSearchRetainsGuidanceAndRequiresCitedHypothesisEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	v, _ := s.Create(validInvestigation(), "actor-1")
	v, _ = s.AddScenario(v.RepositoryID, v.ID, "owner-1", Scenario{Name: "Comparison", Environment: ScenarioEnvironment{Image: "alpine:3.22", WorkingDirectory: ".", Command: "./compare", TimeoutSeconds: 60, CPUs: 1, MemoryMB: 128, StorageMB: 32}}, v.Version)
	search := Search{RequestID: "search-request-1", ScenarioID: v.Scenarios[0].ID, Candidates: []SearchCandidate{{Kind: "commit", RepositoryID: v.RepositoryID, Revision: commit, Classification: "working"}, {Kind: "commit", RepositoryID: v.RepositoryID, Revision: strings.Repeat("b", 40), Classification: "regressed"}}}
	v, err := s.CreateSearch(v.RepositoryID, v.ID, "agent-1", search, v.Version)
	if err != nil || len(v.Searches) != 1 || v.Searches[0].CreatedBy != "agent-1" {
		t.Fatalf("search = %#v, %v", v.Searches, err)
	}
	v, err = s.GuideSearch(v.RepositoryID, v.ID, v.Searches[0].ID, "owner-1", "classify", strings.Repeat("b", 40), "flaky", "Three isolated runs disagreed.", "", "", nil, nil, nil, v.Version, v.Searches[0].Version)
	if err != nil || v.Searches[0].Candidates[1].Classification != "flaky" {
		t.Fatalf("guidance = %#v, %v", v.Searches[0], err)
	}
	if _, err = s.GuideSearch(v.RepositoryID, v.ID, v.Searches[0].ID, "agent-1", "hypothesis", "", "", "Candidate changes serialization.", "The retained attempt supports this claim.", "medium", nil, []string{"invented"}, []string{strings.Repeat("b", 40)}, v.Version, v.Searches[0].Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invented citation accepted: %v", err)
	}
}

func TestAttemptReservationSurvivesConcurrentInvestigationChangesAndReconciles(t *testing.T) {
	s, _ := New(t.TempDir())
	v, _ := s.Create(validInvestigation(), "actor-1")
	v, _ = s.AddScenario(v.RepositoryID, v.ID, "owner-1", Scenario{Name: "Comparison", Environment: ScenarioEnvironment{Image: "alpine:3.22", WorkingDirectory: ".", Command: "./compare", TimeoutSeconds: 60, CPUs: 1, MemoryMB: 128, StorageMB: 32}}, v.Version)
	in := Attempt{RequestID: "attempt-request-1", ScenarioID: v.Scenarios[0].ID, TargetKind: "commit", Revision: commit, Environment: v.Scenarios[0].Environment, Repeats: 2}
	reservedInvestigation, reserved, existed, err := s.ReserveAttempt(v.RepositoryID, v.ID, "agent-1", in, v.Version)
	if err != nil || existed || reserved.State != "running" || len(reservedInvestigation.Attempts) != 1 {
		t.Fatalf("reservation = %#v %#v %v %v", reservedInvestigation, reserved, existed, err)
	}
	changed, err := s.Append(v.RepositoryID, v.ID, "owner-1", "discussion", "Concurrent context", "", reservedInvestigation.Version)
	if err != nil {
		t.Fatal(err)
	}
	completed := reserved
	completed.Classification = "passed"
	completed.Runs = []AttemptRun{{RunID: "run-1", State: "succeeded", Artifacts: []AttemptArtifact{}}}
	finalized, err := s.FinalizeAttempt(v.RepositoryID, v.ID, "agent-1", reserved.ID, completed)
	if err != nil || finalized.Version != changed.Version+1 || finalized.Attempts[0].State != "completed" {
		t.Fatalf("finalization = %#v %v", finalized, err)
	}
	reconciled, same, existed, err := s.ReserveAttempt(v.RepositoryID, v.ID, "agent-1", in, v.Version)
	if err != nil || !existed || same.ID != reserved.ID || reconciled.Attempts[0].Classification != "passed" {
		t.Fatalf("retry = %#v %#v %v %v", reconciled, same, existed, err)
	}
}
