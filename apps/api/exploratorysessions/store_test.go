package exploratorysessions

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func example(now time.Time) Session {
	return Session{Title: "Checkout edge exploration", Source: Source{Kind: "pull_preview", ResourceID: "pull-1", Revision: strings.Repeat("a", 40), Label: "Checkout preview"}, Access: []string{"owner", "tester"}, Limits: Limits{ExpiresAt: now.Add(time.Hour), MaxCostCents: 500, MaxAgentActions: 2, AllowedActions: []string{"navigate", "input", "screenshot", "trace", "command", "observe", "guide", "pause", "resume", "reproduce", "classify", "discard", "close"}, TestData: []string{"synthetic"}}, Charters: []Charter{
		{ID: "payment", Title: "Payment interruption", Risk: "high", Mission: "Interrupt checkout at every boundary", AssigneeType: "agent", AssigneeID: "agent-1", AllowedActions: []string{"navigate", "input", "screenshot", "observe"}, Coverage: []string{"checkout", "recovery"}, Uncertainty: "Gateway timing remains unknown"},
		{ID: "tester", Title: "Reproduce findings", Risk: "high", Mission: "Independently reproduce candidate findings", AssigneeType: "human", AssigneeID: "tester", AllowedActions: []string{"observe", "reproduce"}, Coverage: []string{"checkout", "recovery"}, Uncertainty: "Reproduction may vary"},
		{ID: "owner", Title: "Control and decide", Risk: "high", Mission: "Control the session and decide findings", AssigneeType: "human", AssigneeID: "owner", AllowedActions: []string{"pause", "resume", "classify", "discard", "close"}, Coverage: []string{"session", "findings"}, Uncertainty: "Further evidence may change decisions"},
	}}
}

func TestTimelineIsBoundedCASAndAttributable(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	v, err := s.Create("repo", "owner", example(now))
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("b", 64)
	v, err = s.Append(v.ID, "agent-1", EventInput{ExpectedVersion: 1, Kind: "observation", CharterID: "payment", FindingID: "duplicate-submit", Summary: "Retry duplicates the submission", Route: "/checkout", Inputs: []string{"synthetic decline"}, Coverage: []string{"retry"}, Uncertainty: "Observed once", Artifacts: []Artifact{{Kind: "screenshot", SHA256: digest, MediaType: "image/png", Description: "Duplicate submit state"}}, ActorType: "agent", ActorID: "agent-1"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 2 || v.Events[0].ActorID != "agent-1" || v.Events[0].Route != "/checkout" {
		t.Fatalf("unexpected event: %+v", v)
	}
	if _, err = s.Append(v.ID, "owner", EventInput{ExpectedVersion: 1, Kind: "guide", Summary: "Try keyboard retry"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err = s.Append(v.ID, "other-agent", EventInput{ExpectedVersion: 2, Kind: "observation", CharterID: "payment", Summary: "Unapproved action", ActorType: "agent", ActorID: "other-agent"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected scoped agent rejection, got %v", err)
	}
	if _, err = s.Append(v.ID, "agent-1", EventInput{ExpectedVersion: 2, Kind: "observation", CharterID: "payment", Summary: "Attempt undelegated command", Command: "curl /admin", ActorType: "agent", ActorID: "agent-1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected action-scope rejection, got %v", err)
	}
	if _, err = s.Append(v.ID, "agent-1", EventInput{ExpectedVersion: 2, Kind: "pause", CharterID: "payment", Summary: "Attempt undelegated control", ActorType: "agent", ActorID: "agent-1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected lifecycle-scope rejection, got %v", err)
	}
	if _, err = s.Append(v.ID, "agent-1", EventInput{ExpectedVersion: 2, Kind: "observation", CharterID: "payment", FindingID: "smuggled-decision", Summary: "Attempt classification through observation", Classification: "bug", ActorType: "agent", ActorID: "agent-1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected observation classification rejection, got %v", err)
	}
	v, err = s.Append(v.ID, "tester", EventInput{ExpectedVersion: 2, Kind: "reproduce", CharterID: "tester", FindingID: "duplicate-submit", Summary: "Reproduced with keyboard", ReproducesEventID: v.Events[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Append(v.ID, "owner", EventInput{ExpectedVersion: 3, Kind: "classify", CharterID: "owner", FindingID: "duplicate-submit", Summary: "Confirmed candidate defect", Classification: "bug"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Events[2].Classification != "bug" || v.Events[2].ActorType != "human" {
		t.Fatalf("classification not retained: %+v", v.Events[2])
	}
}

func TestConfirmedFindingRepairIsFrozenAndRetrySafe(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	v, err := s.Create("repo", "owner", example(now))
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Append(v.ID, "tester", EventInput{ExpectedVersion: v.Version, Kind: "observation", CharterID: "tester", FindingID: "double-charge", Summary: "A retry submits payment twice"})
	if err != nil {
		t.Fatal(err)
	}
	observationID := v.Events[len(v.Events)-1].ID
	v, err = s.Append(v.ID, "tester", EventInput{ExpectedVersion: v.Version, Kind: "reproduce", CharterID: "tester", FindingID: "double-charge", Summary: "Reproduced with one synthetic card and one retry", ReproducesEventID: observationID})
	if err != nil {
		t.Fatal(err)
	}
	reproductionID := v.Events[len(v.Events)-1].ID
	v, err = s.Append(v.ID, "owner", EventInput{ExpectedVersion: v.Version, Kind: "classify", CharterID: "owner", FindingID: "double-charge", Summary: "Confirmed defect", Classification: "bug"})
	if err != nil {
		t.Fatal(err)
	}
	in := RepairInput{ExpectedVersion: v.Version, FindingID: "double-charge", EvidenceEventIDs: []string{observationID, reproductionID}, ReproductionEventID: reproductionID, AcceptanceCriteria: []string{"One retry produces one charge", "The regression command fails on the affected revision"}, AssigneeType: "human", AssigneeID: "tester", QualityPlanID: "plan", QualityPlanVersion: 2, RequirementIDs: []string{"checkout"}}
	v, repair, err := s.ReserveRepair(v.ID, "owner", in)
	if err != nil {
		t.Fatal(err)
	}
	if repair.State != "pending" || repair.AffectedRevision != example(now).Source.Revision || v.Version != 5 {
		t.Fatalf("unexpected reservation: %+v", repair)
	}
	_, retried, err := s.ReserveRepair(v.ID, "owner", in)
	if err != nil || retried.RecoveryID != repair.RecoveryID {
		t.Fatalf("exact retry must converge: %+v %v", retried, err)
	}
	v, err = s.FinalizeRepair(v.ID, repair.RecoveryID, "issue", "proposal", "task")
	if err != nil || v.Repairs[0].State != "linked" || v.Repairs[0].IssueID != "issue" {
		t.Fatalf("repair was not linked: %+v %v", v.Repairs, err)
	}
	if _, err = s.FinalizeRepair(v.ID, repair.RecoveryID, "issue", "proposal", "task"); err != nil {
		t.Fatalf("finalization retry must converge: %v", err)
	}
	v, err = s.LinkCoverage(v.ID, repair.FindingID, "scenario", "pull", strings.Repeat("b", 40), v.Version)
	if err != nil || v.Repairs[0].ScenarioID != "scenario" || v.Repairs[0].PullRequestID != "pull" {
		t.Fatalf("lasting coverage was not linked: %+v %v", v.Repairs, err)
	}
	if _, err = s.LinkCoverage(v.ID, repair.FindingID, "different", "pull", strings.Repeat("b", 40), v.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("different coverage must not replace retained proof: %v", err)
	}
}

func TestOnlyConfirmedReproducedBugsEnterRepair(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return now }
	v, _ := s.Create("repo", "owner", example(now))
	v, _ = s.Append(v.ID, "tester", EventInput{ExpectedVersion: v.Version, Kind: "observation", CharterID: "tester", FindingID: "environment-only", Summary: "Observed only in one browser"})
	observationID := v.Events[0].ID
	v, _ = s.Append(v.ID, "owner", EventInput{ExpectedVersion: v.Version, Kind: "classify", CharterID: "owner", FindingID: "environment-only", Summary: "Environment-specific", Classification: "risk"})
	_, _, err := s.ReserveRepair(v.ID, "owner", RepairInput{ExpectedVersion: v.Version, FindingID: "environment-only", EvidenceEventIDs: []string{observationID}, ReproductionEventID: observationID, AcceptanceCriteria: []string{"Document environment boundary"}, AssigneeType: "human", AssigneeID: "owner"})
	if !errors.Is(err, ErrFindingNotConfirmed) {
		t.Fatalf("non-bug resolution entered repair: %v", err)
	}
}

func TestSupersedingFindingDecisionsBlockPendingRepair(t *testing.T) {
	for _, classification := range []string{"flaky", "environment_specific", "not_reproducible", "discarded"} {
		t.Run(classification, func(t *testing.T) {
			now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
			s, _ := New(t.TempDir())
			s.now = func() time.Time { return now }
			v, _ := s.Create("repo", "owner", example(now))
			v, _ = s.Append(v.ID, "tester", EventInput{ExpectedVersion: v.Version, Kind: "observation", CharterID: "tester", FindingID: "finding", Summary: "Observed failure"})
			observationID := v.Events[len(v.Events)-1].ID
			v, _ = s.Append(v.ID, "tester", EventInput{ExpectedVersion: v.Version, Kind: "reproduce", CharterID: "tester", FindingID: "finding", Summary: "Reproduced failure", ReproducesEventID: observationID})
			reproductionID := v.Events[len(v.Events)-1].ID
			v, _ = s.Append(v.ID, "owner", EventInput{ExpectedVersion: v.Version, Kind: "classify", CharterID: "owner", FindingID: "finding", Summary: "Initially confirmed", Classification: "bug"})
			kind := "classify"
			if classification == "discarded" {
				kind = "discard"
			}
			v, _ = s.Append(v.ID, "owner", EventInput{ExpectedVersion: v.Version, Kind: kind, CharterID: "owner", FindingID: "finding", Summary: "Superseded after more evidence", Classification: classification})
			_, _, err := s.ReserveRepair(v.ID, "owner", RepairInput{ExpectedVersion: v.Version, FindingID: "finding", EvidenceEventIDs: []string{observationID, reproductionID}, ReproductionEventID: reproductionID, AcceptanceCriteria: []string{"Failure is absent"}, AssigneeType: "human", AssigneeID: "owner"})
			if !errors.Is(err, ErrFindingNotConfirmed) {
				t.Fatalf("superseded %s finding entered repair: %v", classification, err)
			}
		})
	}
}

func TestSupersedingDecisionBlocksRepairFinalization(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return now }
	v, _ := s.Create("repo", "owner", example(now))
	v, _ = s.Append(v.ID, "tester", EventInput{ExpectedVersion: v.Version, Kind: "observation", CharterID: "tester", FindingID: "finding", Summary: "Observed failure"})
	observationID := v.Events[len(v.Events)-1].ID
	v, _ = s.Append(v.ID, "tester", EventInput{ExpectedVersion: v.Version, Kind: "reproduce", CharterID: "tester", FindingID: "finding", Summary: "Reproduced failure", ReproducesEventID: observationID})
	reproductionID := v.Events[len(v.Events)-1].ID
	v, _ = s.Append(v.ID, "owner", EventInput{ExpectedVersion: v.Version, Kind: "classify", CharterID: "owner", FindingID: "finding", Summary: "Initially confirmed", Classification: "bug"})
	v, repair, _ := s.ReserveRepair(v.ID, "owner", RepairInput{ExpectedVersion: v.Version, FindingID: "finding", EvidenceEventIDs: []string{observationID, reproductionID}, ReproductionEventID: reproductionID, AcceptanceCriteria: []string{"Failure is absent"}, AssigneeType: "human", AssigneeID: "owner"})
	v, _ = s.Append(v.ID, "owner", EventInput{ExpectedVersion: v.Version, Kind: "classify", CharterID: "owner", FindingID: "finding", Summary: "Superseded after more evidence", Classification: "flaky"})
	if _, err := s.FinalizeRepair(v.ID, repair.RecoveryID, "issue", "proposal", "task"); !errors.Is(err, ErrFindingNotConfirmed) {
		t.Fatalf("superseded finding finalized repair: %v", err)
	}
	stored, _ := s.Get(v.ID)
	if stored.Repairs[0].State != "pending" || stored.Repairs[0].IssueID != "" || stored.Repairs[0].ProposalID != "" || stored.Repairs[0].TaskID != "" {
		t.Fatalf("failed finalization attached work: %+v", stored.Repairs[0])
	}
}

func TestCoverageLinkChecksVersionAndActiveStateUnderLock(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return now }
	v, _ := s.Create("repo", "owner", example(now))
	v, _ = s.Append(v.ID, "tester", EventInput{ExpectedVersion: v.Version, Kind: "observation", CharterID: "tester", FindingID: "finding", Summary: "Observed failure"})
	observationID := v.Events[len(v.Events)-1].ID
	v, _ = s.Append(v.ID, "tester", EventInput{ExpectedVersion: v.Version, Kind: "reproduce", CharterID: "tester", FindingID: "finding", Summary: "Reproduced failure", ReproducesEventID: observationID})
	reproductionID := v.Events[len(v.Events)-1].ID
	v, _ = s.Append(v.ID, "owner", EventInput{ExpectedVersion: v.Version, Kind: "classify", CharterID: "owner", FindingID: "finding", Summary: "Confirmed", Classification: "bug"})
	v, repair, _ := s.ReserveRepair(v.ID, "owner", RepairInput{ExpectedVersion: v.Version, FindingID: "finding", EvidenceEventIDs: []string{observationID, reproductionID}, ReproductionEventID: reproductionID, AcceptanceCriteria: []string{"Failure is absent"}, AssigneeType: "human", AssigneeID: "owner"})
	v, _ = s.FinalizeRepair(v.ID, repair.RecoveryID, "issue", "proposal", "task")
	capturedVersion := v.Version
	v, err := s.Append(v.ID, "owner", EventInput{ExpectedVersion: v.Version, Kind: "close", CharterID: "owner", Summary: "Exploration complete"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.LinkCoverage(v.ID, repair.FindingID, "scenario", "pull", strings.Repeat("c", 40), capturedVersion); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale coverage mutated a closed session: %v", err)
	}
	stored, _ := s.Get(v.ID)
	if stored.Version != v.Version || stored.Repairs[0].ScenarioID != "" {
		t.Fatalf("stale coverage changed retained state: %+v", stored)
	}
}

func TestTimelineReferencesMustResolveWithinSession(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	first, err := s.Create("repo", "owner", example(now))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create("repo", "owner", example(now))
	if err != nil {
		t.Fatal(err)
	}
	first, err = s.Append(first.ID, "tester", EventInput{ExpectedVersion: 1, Kind: "observation", CharterID: "tester", FindingID: "finding-1", Summary: "Observed failure"})
	if err != nil {
		t.Fatal(err)
	}
	second, err = s.Append(second.ID, "tester", EventInput{ExpectedVersion: 1, Kind: "observation", CharterID: "tester", FindingID: "finding-2", Summary: "Other failure"})
	if err != nil {
		t.Fatal(err)
	}
	invalid := []EventInput{
		{ExpectedVersion: 2, Kind: "classify", CharterID: "owner", FindingID: "missing", Summary: "Classify missing", Classification: "bug"},
		{ExpectedVersion: 2, Kind: "discard", CharterID: "owner", FindingID: "missing", Summary: "Discard missing", Classification: "discarded"},
		{ExpectedVersion: 2, Kind: "reproduce", CharterID: "tester", FindingID: "finding-1", ReproducesEventID: "missing", Summary: "Reproduce missing"},
		{ExpectedVersion: 2, Kind: "reproduce", CharterID: "tester", FindingID: "finding-2", ReproducesEventID: second.Events[0].ID, Summary: "Cross-session reference"},
	}
	for _, in := range invalid {
		actor := "tester"
		if one(in.Kind, "classify", "discard") {
			actor = "owner"
		}
		if _, err = s.Append(first.ID, actor, in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected invalid reference rejection for %+v, got %v", in, err)
		}
	}
}

func TestHumanCharterAssignmentControlsActions(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	v, err := s.Create("repo", "owner", example(now))
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Append(v.ID, "tester", EventInput{ExpectedVersion: 1, Kind: "observation", CharterID: "tester", FindingID: "finding", Summary: "Observed failure"})
	if err != nil {
		t.Fatal(err)
	}
	invalid := []EventInput{
		{ExpectedVersion: 2, Kind: "pause", Summary: "Unassigned control without charter"},
		{ExpectedVersion: 2, Kind: "pause", CharterID: "owner", Summary: "Unassigned control through another human charter"},
		{ExpectedVersion: 2, Kind: "classify", CharterID: "owner", FindingID: "finding", Summary: "Unassigned finding decision", Classification: "risk"},
	}
	for _, in := range invalid {
		if _, err = s.Append(v.ID, "tester", in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected human charter rejection for %+v, got %v", in, err)
		}
	}
	v, err = s.Append(v.ID, "tester", EventInput{ExpectedVersion: 2, Kind: "guide", Summary: "Try the keyboard path"})
	if err != nil || v.Events[1].Kind != "guide" {
		t.Fatalf("explicit audience guidance should remain collaborative: %v", err)
	}
}

func TestSessionRejectsUnsafeDataAndUnboundedAuthority(t *testing.T) {
	now := time.Now().UTC()
	v := example(now)
	v.Limits.TestData = []string{"production_personal_data"}
	if ValidSession(v, now) {
		t.Fatal("production data must not be admitted")
	}
	v = example(now)
	v.Limits.ExpiresAt = now.Add(25 * time.Hour)
	if ValidSession(v, now) {
		t.Fatal("session over 24 hours must not be admitted")
	}
	v = example(now)
	v.Charters[0].AllowedActions = []string{"deploy"}
	if ValidSession(v, now) {
		t.Fatal("charter must not broaden session authority")
	}
}
