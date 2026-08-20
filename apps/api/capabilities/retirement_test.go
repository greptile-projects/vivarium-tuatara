package capabilities

import (
	"errors"
	"testing"
	"time"
)

func retirementFixture(now time.Time) Revision {
	return Revision{Name: "legacy", Summary: "legacy endpoint", CommitID: string(make([]byte, 40)), ReleaseID: "release", OwnerIDs: []string{"provider"}, Items: []Item{{Kind: "interface", Name: "v1", Path: "api.go", Revision: string(make([]byte, 40))}}, Consumers: []Consumer{{Name: "mobile", OwnerIDs: []string{"mobile-owner"}, Environment: "production", Discovery: "declared", EvidenceState: "unknown", CompatibilityPromise: "supported through 2027"}}}
}

func cleanupFixture(repo, revision, path string) []CleanupRequirement {
	out := []CleanupRequirement{}
	for _, kind := range []string{"code", "flags", "data", "credentials", "telemetry", "documentation", "policy_exceptions"} {
		out = append(out, CleanupRequirement{ID: "cleanup-" + kind, Kind: kind, RepositoryID: repo, Path: path, Revision: revision, PreviousBlob: "blob-before-" + kind, Expectation: "removed"})
	}
	return out
}

func TestRetirementWorkPreservesCommittedLinkWhenDirectorySyncIsUncertain(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	r := retirementFixture(now)
	r.Consumers[0].RepositoryID = "consumer-repository"
	v, err := s.Create("provider-repository", "provider", r)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.OpenRetirement("provider-repository", v.ID, "provider", planFixture(now))
	if err != nil {
		t.Fatal(err)
	}
	s.directorySync = func(string) error { return errors.New("injected directory sync failure") }
	work := RetirementWork{AudienceIndex: 0, RepositoryID: "consumer-repository", OldContract: "legacy", ReplacementContract: "supported", AcceptanceCriteria: []string{"tests pass"}, DocumentationChanges: []string{"update guide"}, RolloutStage: "adopt"}
	_, linked, err := s.CreateRetirementWork("provider-repository", v.ID, v.RetirementPlans[0].ID, "consumer-owner", 0, work, func() (string, string, error) { return "proposal-one", "task-one", nil })
	if !errors.Is(err, ErrDurabilityUncertain) {
		t.Fatalf("error = %v", err)
	}
	if linked.ProposalID != "proposal-one" || linked.TaskID != "task-one" {
		t.Fatalf("linked result = %#v", linked)
	}
	s.directorySync = syncCapabilityDirectory
	persisted, err := s.Get("provider-repository", v.ID)
	if err != nil || persisted.RetirementPlans[0].WorkVersion != 1 || len(persisted.RetirementPlans[0].Work) != 1 {
		t.Fatalf("persisted link = %#v, %v", persisted.RetirementPlans, err)
	}
}
func planFixture(now time.Time) RetirementPlan {
	return RetirementPlan{Rationale: "replace an unsafe protocol", Replacements: []Replacement{{Name: "v2", Reference: "capability:v2", MigrationGuide: "docs/migrate.md", Supported: true}}, Audiences: []Audience{{Name: "mobile", OwnerIDs: []string{"mobile-owner"}, Impact: "v1 requests stop working", Commitment: "supported through 2027", EmbargoedDependency: true}}, Stages: []CompatibilityStage{{Name: "warn", StartsAt: now.Add(time.Hour), Behavior: "serve with warning", ExitCriteria: []string{"owners notified"}}, {Name: "disable", StartsAt: now.Add(48 * time.Hour), Behavior: "reject v1", ExitCriteria: []string{"traffic zero"}}}, Deadline: now.Add(72 * time.Hour), ApprovalDueAt: now.Add(24 * time.Hour), SuccessCriteria: []string{"v2 success rate is stable"}, RollbackCriteria: []string{"consumer errors exceed one percent"}, Communication: CommunicationPolicy{Channels: []string{"owner inbox"}, NoticeDays: 30, Updates: "weekly", Escalation: "repository owner"}, RequiredOwnerIDs: []string{"mobile-owner"}}
}

func TestRetirementPlanRetainsBlockersAndAcknowledgements(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	v, err := s.Create("repo", "provider", retirementFixture(now))
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.OpenRetirement("repo", v.ID, "provider", planFixture(now))
	if err != nil {
		t.Fatal(err)
	}
	p := v.RetirementPlans[0]
	kinds := map[string]bool{}
	for _, b := range p.Blockers {
		kinds[b.Kind] = true
	}
	for _, k := range []string{"inventory_unknown_evidence", "owner_approval_required", "conflicting_commitment", "embargoed_dependency"} {
		if !kinds[k] {
			t.Fatalf("missing %s in %#v", k, p.Blockers)
		}
	}
	v, err = s.AppendRetirementEvent("repo", v.ID, p.ID, "agent-1", "read_only_agent", 0, RetirementEvent{Type: "challenge", Summary: "traffic remains", Evidence: []string{"usage:sha256:abc"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendRetirementEvent("repo", v.ID, p.ID, "agent-1", "read_only_agent", 1, RetirementEvent{Type: "approval", Summary: "approve", OwnerID: "agent-1", Decision: "approved"}); err != ErrInvalid {
		t.Fatalf("agent approved: %v", err)
	}
	v, err = s.AppendRetirementEvent("repo", v.ID, p.ID, "mobile-owner", "human", 1, RetirementEvent{Type: "approval", Summary: "migration understood", OwnerID: "mobile-owner", Decision: "approved"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.RetirementPlans[0].Events) != 2 {
		t.Fatalf("events %#v", v.RetirementPlans[0].Events)
	}
}

func TestRetirementChangedUsageAndBoundedDecisions(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	r := retirementFixture(now)
	v, _ := s.Create("repo", "provider", r)
	v, _ = s.OpenRetirement("repo", v.ID, "provider", planFixture(now))
	p := v.RetirementPlans[0]
	far := now.Add(31 * 24 * time.Hour)
	if _, err := s.AppendRetirementEvent("repo", v.ID, p.ID, "provider", "human", 0, RetirementEvent{Type: "policy_decision", Summary: "wait", Decision: "defer", ExpiresAt: &far, FollowUp: "issue:1"}); err != ErrInvalid {
		t.Fatalf("unbounded decision: %v", err)
	}
	r.Summary = "new usage"
	v, err := s.Revise("repo", v.ID, 1, "provider", r)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range v.RetirementPlans[0].Blockers {
		found = found || b.Kind == "changed_usage"
	}
	if !found {
		t.Fatal("changed usage did not block")
	}
}

func TestRetirementProjectionUsesStoreClock(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	v, err := s.Create("repo", "provider", retirementFixture(now))
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.OpenRetirement("repo", v.ID, "provider", planFixture(now))
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now.Add(25 * time.Hour) }
	v, err = s.Get("repo", v.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, blocker := range v.RetirementPlans[0].Blockers {
		found = found || blocker.Kind == "unresponsive_owner"
	}
	if !found {
		t.Fatalf("advanced Store clock did not make owner overdue: %#v", v.RetirementPlans[0].Blockers)
	}
}

func TestRetirementWorkIsOrderedAndNewConsumersRemainReports(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	r := retirementFixture(now)
	r.Consumers[0].RepositoryID = "consumer-repository"
	v, err := s.Create("provider-repository", "provider", r)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.OpenRetirement("provider-repository", v.ID, "provider", planFixture(now))
	if err != nil {
		t.Fatal(err)
	}
	p := v.RetirementPlans[0]
	work := RetirementWork{AudienceIndex: 0, RepositoryID: "consumer-repository", OldContract: "GET /v1 returns legacy widgets", ReplacementContract: "GET /v2 returns supported widgets", AcceptanceCriteria: []string{"consumer tests pass against v2"}, DocumentationChanges: []string{"replace v1 examples"}, RolloutStage: "adopt"}
	v, first, err := s.CreateRetirementWork("provider-repository", v.ID, p.ID, "consumer-owner", 0, work, func() (string, string, error) { return "proposal-one", "task-one", nil })
	if err != nil {
		t.Fatal(err)
	}
	work.DependencyIDs = []string{first.ID}
	v, second, err := s.CreateRetirementWork("provider-repository", v.ID, p.ID, "consumer-owner", 1, work, func() (string, string, error) { return "proposal-two", "task-two", nil })
	if err != nil || second.DependencyIDs[0] != first.ID {
		t.Fatalf("ordered work = %#v, %v", second, err)
	}
	called := false
	if _, _, err = s.CreateRetirementWork("provider-repository", v.ID, p.ID, "consumer-owner", 1, work, func() (string, string, error) { called = true; return "extra", "extra", nil }); err != ErrConflict || called {
		t.Fatalf("stale publication = %v, called=%v", err, called)
	}
	v, err = s.ReportRetirementConsumer("provider-repository", v.ID, p.ID, "new-owner", 2, ConsumerDiscovery{RepositoryID: "new-consumer", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Paths: []string{"client.go"}, Evidence: []string{"symbol:legacyClient"}, Impact: "client still calls v1"})
	if err != nil {
		t.Fatal(err)
	}
	plan := v.RetirementPlans[0]
	if len(plan.Work) != 2 || len(plan.DiscoveredConsumers) != 1 || plan.WorkVersion != 3 {
		t.Fatalf("plan coordination = %#v", plan)
	}
	found := false
	for _, blocker := range plan.Blockers {
		found = found || blocker.Kind == "new_consumer_discovered"
	}
	if !found {
		t.Fatalf("discovery did not require reassessment: %#v", plan.Blockers)
	}
}

func TestMigrationCandidateRetainsMatrixAndNeverTreatsUnknownUseAsMigrated(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	r := retirementFixture(now)
	r.Consumers[0].RepositoryID = "consumer"
	r.Consumers[0].Revision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	v, _ := s.Create("repo", "provider", r)
	v, _ = s.OpenRetirement("repo", v.ID, "provider", planFixture(now))
	checks := []CandidateCheck{}
	for _, stage := range []string{"old_only", "dual_support", "replacement", "rollback", "journey"} {
		checks = append(checks, CandidateCheck{ID: stage, Stage: stage, Journey: map[bool]string{true: "checkout"}[stage == "journey"], RepositoryID: "repo", Revision: r.CommitID, Command: "test " + stage, Paths: []string{"api.go"}, Expectation: stage + " remains supported"})
	}
	if _, _, unrelatedErr := s.CreateMigrationCandidate("repo", v.ID, v.RetirementPlans[0].ID, "provider", MigrationCandidate{Environment: "isolated synthetic compatibility lab", Checks: checks, CleanupRequirements: cleanupFixture("repo", r.CommitID, "unrelated.txt")}); unrelatedErr != ErrInvalid {
		t.Fatalf("unrelated cleanup inventory = %v", unrelatedErr)
	}
	duplicate := cleanupFixture("repo", r.CommitID, "api.go")
	duplicate = append(duplicate, CleanupRequirement{ID: "duplicate-code", Kind: "code", RepositoryID: "repo", Path: "api.go", Revision: r.CommitID, PreviousBlob: "duplicate-before", Expectation: "removed"})
	if _, _, duplicateErr := s.CreateMigrationCandidate("repo", v.ID, v.RetirementPlans[0].ID, "provider", MigrationCandidate{Environment: "isolated synthetic compatibility lab", Checks: checks, CleanupRequirements: duplicate}); duplicateErr != ErrInvalid {
		t.Fatalf("duplicate cleanup surface = %v", duplicateErr)
	}
	v, candidate, err := s.CreateMigrationCandidate("repo", v.ID, v.RetirementPlans[0].ID, "provider", MigrationCandidate{Environment: "isolated synthetic compatibility lab", Checks: checks, CleanupRequirements: cleanupFixture("repo", r.CommitID, "api.go")})
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range checks {
		v, err = s.AddCandidateEvidence("repo", v.ID, v.RetirementPlans[0].ID, candidate.ID, "provider", check.ID, CandidateEvidence{WorkspaceID: "workspace", OutcomeID: "outcome-" + check.ID, Status: "passed", CommandDigest: CommandDigest(check.Command)})
		if err != nil {
			t.Fatal(err)
		}
	}
	plan := v.RetirementPlans[0]
	if _, reuseErr := s.AddCandidateEvidence("repo", v.ID, plan.ID, candidate.ID, "provider", "dual_support", CandidateEvidence{WorkspaceID: "workspace", OutcomeID: "outcome-old_only", Status: "passed"}); reuseErr != ErrInvalid {
		t.Fatalf("shared outcome was accepted for another check: %v", reuseErr)
	}
	if plan.Candidates[0].RemovalReady {
		t.Fatal("missing usage observation was treated as migrated")
	}
	v, err = s.AddUsageObservation("repo", v.ID, plan.ID, candidate.ID, "mobile-owner", UsageObservation{ConsumerIndex: 0, State: "inaccessible", Summary: "telemetry cannot be read", WindowStartsAt: now.Add(-time.Hour), WindowEndsAt: now, OwnerID: "mobile-owner"})
	if err != nil {
		t.Fatal(err)
	}
	if v.RetirementPlans[0].Candidates[0].RemovalReady {
		t.Fatal("inaccessible use was treated as migrated")
	}
	v, err = s.AddUsageObservation("repo", v.ID, plan.ID, candidate.ID, "mobile-owner", UsageObservation{ConsumerIndex: 0, State: "measured", Summary: "no old calls in the agreed window", WindowStartsAt: now.Add(-time.Hour), WindowEndsAt: now, TotalUses: 12, OwnerID: "mobile-owner"})
	if err != nil {
		t.Fatal(err)
	}
	got := v.RetirementPlans[0].Candidates[0]
	if !got.RemovalReady || !got.Usage[0].Superseded || got.Usage[1].Superseded {
		t.Fatalf("candidate readiness = %#v", got)
	}
	_, err = s.AddCandidateEvidence("repo", v.ID, plan.ID, candidate.ID, "provider", "rollback", CandidateEvidence{WorkspaceID: "workspace-2", OutcomeID: "failed", Status: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = func() (MigrationCandidate, error) {
		current, e := s.Get("repo", v.ID)
		return current.RetirementPlans[0].Candidates[0], e
	}()
	if got.RemovalReady || !got.Checks[3].Evidence[0].Superseded {
		t.Fatalf("failed superseding proof did not block: %#v", got.Checks[3])
	}
}
