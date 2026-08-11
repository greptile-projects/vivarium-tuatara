package deliveryteams

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func charter(person, role string) Charter {
	return Charter{Name: "Ship the migration", Purpose: "Deliver one governed result", EscalationPath: "Escalate scope to the accountable owner", Participants: []Participant{{ID: person + "-slot", PrincipalType: "human", PrincipalID: person, Role: role, Responsibility: "Own compatibility verification", Why: "Maintains the affected surface", Escalation: "Raise unresolved risk to the organizer", RequiredAccess: []AccessRequirement{{RepositoryID: "repo", Level: "read"}}}}}
}

func TestCharterAcceptanceAndReplacementRemainAttributable(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create("repo", Outcome{Kind: "proposal", ResourceID: "proposal", Title: "Migration"}, charter("alice", "compatibility lead"), "organizer")
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Respond(v.ID, "alice-slot", "alice", "accepted", v.Version)
	if err != nil || v.Participants[0].Status != "accepted" {
		t.Fatalf("accept: %#v %v", v.Participants, err)
	}
	next := charter("bob", "replacement lead")
	v, err = s.Update(v.ID, "organizer", v.Version, next)
	if err != nil {
		t.Fatal(err)
	}
	if v.Participants[0].Status != "pending" || len(v.Events) != 3 || v.Events[2].ActorID != "organizer" {
		t.Fatalf("replacement history: %#v", v)
	}
	if _, err = s.Update(v.ID, "alice", v.Version, next); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-organizer update = %v", err)
	}
}

func TestCharterRejectsDuplicatePrincipalAndStaleResponse(t *testing.T) {
	s, _ := New(t.TempDir())
	c := charter("alice", "lead")
	c.Participants = append(c.Participants, c.Participants[0])
	if _, err := s.Create("repo", Outcome{Kind: "planned_outcome", ResourceID: "outcome", Title: "Outcome"}, c, "organizer"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate = %v", err)
	}
	v, err := s.Create("repo", Outcome{Kind: "planned_outcome", ResourceID: "outcome", Title: "Outcome"}, charter("alice", "lead"), "organizer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Respond(v.ID, "alice-slot", "alice", "accepted", v.Version+1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale response = %v", err)
	}
}

func TestChangedInvitationTermsRequireFreshAcceptance(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Create("repo", Outcome{Kind: "planned_outcome", ResourceID: "outcome", Title: "Outcome"}, charter("alice", "lead"), "organizer")
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Respond(v.ID, "alice-slot", "alice", "accepted", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	unchanged := charter("alice", "lead")
	v, err = s.Update(v.ID, "organizer", v.Version, unchanged)
	if err != nil || v.Participants[0].Status != "accepted" {
		t.Fatalf("unchanged acceptance = %#v, %v", v.Participants[0], err)
	}
	changed := charter("alice", "delivery lead")
	v, err = s.Update(v.ID, "organizer", v.Version, changed)
	if err != nil {
		t.Fatal(err)
	}
	if v.Participants[0].Status != "pending" || v.Participants[0].RespondedBy != "" || v.Participants[0].RespondedAt != nil {
		t.Fatalf("changed acceptance = %#v", v.Participants[0])
	}
}

func TestChangedSharedCharterTermsRequireFreshAcceptance(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Create("repo", Outcome{Kind: "planned_outcome", ResourceID: "outcome", Title: "Outcome"}, charter("alice", "lead"), "organizer")
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Respond(v.ID, "alice-slot", "alice", "accepted", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	changed := charter("alice", "lead")
	changed.Purpose = "Deliver the revised governed result"
	v, err = s.Update(v.ID, "organizer", v.Version, changed)
	if err != nil {
		t.Fatal(err)
	}
	if v.Participants[0].Status != "pending" || v.Participants[0].RespondedBy != "" || v.Participants[0].RespondedAt != nil {
		t.Fatalf("shared-term acceptance = %#v", v.Participants[0])
	}
	reloaded, err := s.Get(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Participants[0].Status != "pending" {
		t.Fatalf("persisted participant = %#v", reloaded.Participants[0])
	}
	if _, err = s.Respond(v.ID, "alice-slot", "alice", "accepted", v.Version); err != nil {
		t.Fatalf("fresh acceptance: %v", err)
	}
}

func TestParallelPlanSurfacesOverlapBudgetAndRequiresOwnerAcceptance(t *testing.T) {
	s, _ := New(t.TempDir())
	c := charter("alice", "implementation lead")
	c.OverallBudget = &Budget{Unit: "minutes", Limit: 30}
	c.Participants[0].Budget = &Budget{Unit: "minutes", Limit: 20}
	c.Participants = append(c.Participants, Participant{ID: "bob-slot", PrincipalType: "human", PrincipalID: "bob", Role: "verification lead", Responsibility: "Own integration evidence", Why: "Maintains the checks", Escalation: "Raise incompatible evidence", RequiredAccess: []AccessRequirement{{RepositoryID: "repo", Level: "write"}}})
	v, err := s.Create("repo", Outcome{Kind: "planned_outcome", ResourceID: "outcome", Title: "Outcome"}, c, "organizer")
	if err != nil {
		t.Fatal(err)
	}
	v, _ = s.Respond(v.ID, "alice-slot", "alice", "accepted", v.Version)
	v, _ = s.Respond(v.ID, "bob-slot", "bob", "accepted", v.Version)
	hash := strings.Repeat("a", 40)
	plan := PlanInput{Streams: []WorkStream{
		{ID: "implementation", Title: "Implement", OwnerParticipantID: "alice-slot", ExpectedArtifacts: []string{"migration patch"}, AcceptanceCriteria: []string{"unit checks pass"}, RepositoryScope: []RevisionScope{{RepositoryID: "repo", Reference: "main", Revision: hash, Paths: []string{"src"}}}, IntegrationOrder: 1, Budget: &Budget{Unit: "minutes", Limit: 25}, Assumptions: []string{"schema v1 remains stable"}},
		{ID: "verification", Title: "Verify", OwnerParticipantID: "bob-slot", Inputs: []WorkInput{{Name: "candidate", SourceStreamID: "implementation", Artifact: "migration patch"}}, ExpectedArtifacts: []string{"migration patch"}, DependencyIDs: []string{"implementation"}, AcceptanceCriteria: []string{"integration checks pass"}, RepositoryScope: []RevisionScope{{RepositoryID: "repo", Reference: "main", Revision: hash, Paths: []string{"src/checks"}}}, IntegrationOrder: 2, Budget: &Budget{Unit: "minutes", Limit: 10}, Assumptions: []string{"implementation preserves API"}},
	}}
	v, err = s.PutPlan(v.ID, "alice", "alice", v.Version, plan)
	if err != nil {
		t.Fatal(err)
	}
	if v.Plan.Revision != 1 || len(v.Plan.Blockers) != 5 || v.Plan.Acceptances[0].Status != "accepted" || v.Plan.Acceptances[1].Status != "pending" {
		t.Fatalf("plan = %#v", v.Plan)
	}
	v, err = s.RespondPlan(v.ID, "bob-slot", "bob", "bob", "accepted", v.Version, v.Plan.Revision)
	if err != nil {
		t.Fatal(err)
	}
	for _, blocker := range v.Plan.Blockers {
		if blocker.Kind == "replan_acceptance" {
			t.Fatalf("acceptance blocker remained: %#v", v.Plan.Blockers)
		}
	}
	plan.Streams[0].Assumptions = []string{"schema v2 is now required"}
	v, err = s.PutPlan(v.ID, "bob", "bob", v.Version, plan)
	if err != nil || v.Plan.Revision != 2 || v.Plan.Acceptances[0].Status != "pending" {
		t.Fatalf("replan = %#v, %v", v.Plan, err)
	}
	v, err = s.Update(v.ID, "organizer", v.Version, c)
	if err != nil || v.Plan.Revision != 2 || len(v.PlanHistory) != 1 {
		t.Fatalf("unchanged charter moved plan = %#v, %v", v.Plan, err)
	}
	changedCharter := c
	changedCharter.Purpose = "Deliver against the changed upstream outcome"
	v, err = s.Update(v.ID, "organizer", v.Version, changedCharter)
	if err != nil || v.Plan.Revision != 3 || len(v.PlanHistory) != 2 || !slices.ContainsFunc(v.Plan.Blockers, func(b PlanBlocker) bool { return b.Kind == "charter_changed" }) {
		t.Fatalf("charter invalidation = %#v, %v", v.Plan, err)
	}
}

func TestPlanRejectsCyclesAndStaleRevisions(t *testing.T) {
	s, _ := New(t.TempDir())
	v, _ := s.Create("repo", Outcome{Kind: "planned_outcome", ResourceID: "outcome", Title: "Outcome"}, charter("alice", "lead"), "organizer")
	hash := strings.Repeat("a", 40)
	streams := []WorkStream{{ID: "one", Title: "One", OwnerParticipantID: "alice-slot", ExpectedArtifacts: []string{"one"}, DependencyIDs: []string{"two"}, AcceptanceCriteria: []string{"done"}, RepositoryScope: []RevisionScope{{RepositoryID: "repo", Reference: "main", Revision: hash, Paths: []string{"one"}}}, IntegrationOrder: 1, Assumptions: []string{"stable"}}, {ID: "two", Title: "Two", OwnerParticipantID: "alice-slot", ExpectedArtifacts: []string{"two"}, DependencyIDs: []string{"one"}, AcceptanceCriteria: []string{"done"}, RepositoryScope: []RevisionScope{{RepositoryID: "repo", Reference: "main", Revision: hash, Paths: []string{"two"}}}, IntegrationOrder: 2, Assumptions: []string{"stable"}}}
	if _, err := s.PutPlan(v.ID, "organizer", "organizer", v.Version, PlanInput{Streams: streams}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cycle = %v", err)
	}
	streams[0].DependencyIDs = nil
	streams[1].DependencyIDs = []string{"one"}
	v, err := s.PutPlan(v.ID, "organizer", "organizer", v.Version, PlanInput{Streams: streams})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RespondPlan(v.ID, "alice-slot", "alice", "alice", "accepted", v.Version-1, v.Plan.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale response = %v", err)
	}
}

func TestDeclinedStreamOwnerImmediatelyBecomesUnavailable(t *testing.T) {
	s, _ := New(t.TempDir())
	v, _ := s.Create("repo", Outcome{Kind: "planned_outcome", ResourceID: "outcome", Title: "Outcome"}, charter("alice", "lead"), "organizer")
	plan := PlanInput{Streams: []WorkStream{{ID: "work", Title: "Work", OwnerParticipantID: "alice-slot", ExpectedArtifacts: []string{"result"}, AcceptanceCriteria: []string{"verified"}, RepositoryScope: []RevisionScope{{RepositoryID: "repo", Reference: "main", Revision: strings.Repeat("a", 40), Paths: []string{"src"}}}, IntegrationOrder: 1, Assumptions: []string{"owner remains available"}}}}
	v, err := s.PutPlan(v.ID, "organizer", "organizer", v.Version, plan)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Respond(v.ID, "alice-slot", "alice", "declined", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(v.Plan.Blockers, func(blocker PlanBlocker) bool { return blocker.Kind == "owner_unavailable" }) {
		t.Fatalf("declined owner blockers = %#v", v.Plan.Blockers)
	}
	reloaded, err := s.Get(v.ID)
	if err != nil || !slices.ContainsFunc(reloaded.Plan.Blockers, func(blocker PlanBlocker) bool { return blocker.Kind == "owner_unavailable" }) {
		t.Fatalf("persisted blockers = %#v, %v", reloaded.Plan, err)
	}
}

func TestCitedTimelineAndVerifiedHandoffRetainExactContext(t *testing.T) {
	s, _ := New(t.TempDir())
	c := charter("alice", "builder")
	c.Participants = append(c.Participants, Participant{ID: "bob-slot", PrincipalType: "human", PrincipalID: "bob", Role: "reviewer", Responsibility: "Continue verified work", Why: "Owns integration", Escalation: "Raise uncertainty"})
	v, err := s.Create("repo", Outcome{Kind: "planned_outcome", ResourceID: "outcome", Title: "Outcome"}, c, "organizer")
	if err != nil {
		t.Fatal(err)
	}
	v, _ = s.Respond(v.ID, "alice-slot", "alice", "accepted", v.Version)
	v, _ = s.Respond(v.ID, "bob-slot", "bob", "accepted", v.Version)
	revision := strings.Repeat("a", 40)
	v, err = s.PutPlan(v.ID, "alice", "alice", v.Version, PlanInput{Streams: []WorkStream{{ID: "build", Title: "Build", OwnerParticipantID: "alice-slot", ExpectedArtifacts: []string{"patch"}, AcceptanceCriteria: []string{"reviewed"}, RepositoryScope: []RevisionScope{{RepositoryID: "repo", Reference: "main", Revision: revision, Paths: []string{"src"}}}, IntegrationOrder: 1, Assumptions: []string{"API stable"}}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.AttachContext(v.ID, "build", "alice", "alice", v.Version, WorkContext{Kind: "workspace", ResourceID: "workspace-1", RepositoryID: "repo", Revision: revision})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.PublishTimeline(v.ID, "alice", "alice", v.Version, TimelineInput{StreamID: "build", Kind: "finding", Body: "The boundary is reproducible", Citations: []Citation{{Kind: "workspace", ResourceID: "workspace-1", RepositoryID: "repo", Revision: revision, Label: "reproduced checkpoint"}}})
	if err != nil {
		t.Fatal(err)
	}
	finding := v.Timeline[0]
	if _, err = s.PublishTimeline(v.ID, "alice", "alice", v.Version, TimelineInput{StreamID: "build", Kind: "finding", Body: "Opaque claim", Citations: []Citation{{Kind: "workspace", ResourceID: "missing", RepositoryID: "repo", Revision: revision, Label: "missing"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unattached citation = %v", err)
	}
	v, err = s.RequestHandoff(v.ID, "alice", "alice", v.Version, HandoffInput{StreamID: "build", ToParticipantID: "bob-slot", InputEntryIDs: []string{finding.ID}, AcceptanceCriteria: []string{"reproduce the checkpoint"}, ResidualUncertainty: []string{"load behavior remains unknown"}})
	if err != nil {
		t.Fatal(err)
	}
	handoff := v.Handoffs[0]
	if len(handoff.Inputs) != 1 || handoff.Inputs[0].Revision != revision || handoff.PlanRevision != v.Plan.Revision {
		t.Fatalf("frozen handoff = %#v", handoff)
	}
	if _, err = s.AcceptHandoff(v.ID, handoff.ID, "bob", "bob", v.Version, []string{finding.ID}, "verified"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("accepted another author's unverifying entry: %v", err)
	}
	v, err = s.PublishTimeline(v.ID, "bob", "bob", v.Version, TimelineInput{StreamID: "build", Kind: "uncertainty", Body: "Reproduction completed; load remains open"})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-owner publication = %v", err)
	}
	// Transfer does not silently change stream ownership; the recipient publishes verification only after ownership is explicitly replanned.
	plan := v.Plan.Streams
	plan[0].OwnerParticipantID = "bob-slot"
	v, err = s.PutPlan(v.ID, "alice", "alice", v.Version, PlanInput{Streams: plan})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Plan.Streams[0].Contexts) != 1 || v.Plan.Streams[0].Contexts[0].ResourceID != "workspace-1" {
		t.Fatalf("replanned context = %#v", v.Plan.Streams[0].Contexts)
	}
	v, err = s.PublishTimeline(v.ID, "bob", "bob", v.Version, TimelineInput{StreamID: "build", Kind: "uncertainty", Body: "Reproduction completed; load remains open"})
	if err != nil {
		t.Fatal(err)
	}
	verification := v.Timeline[len(v.Timeline)-1]
	v, err = s.AcceptHandoff(v.ID, handoff.ID, "bob", "bob", v.Version, []string{verification.ID}, "Reproduced the frozen input and accept the remaining load uncertainty")
	if err != nil {
		t.Fatal(err)
	}
	if v.Handoffs[0].Status != "accepted" || v.Handoffs[0].AcceptedBy != "bob" {
		t.Fatalf("acceptance = %#v", v.Handoffs[0])
	}
}

func TestLiveStreamStatusAndBoundedInterventionPreserveAcceptedWork(t *testing.T) {
	s, _ := New(t.TempDir())
	c := charter("alice", "builder")
	c.Participants = append(c.Participants, Participant{ID: "bob-slot", PrincipalType: "human", PrincipalID: "bob", Role: "recovery lead", Responsibility: "Take explicit reassignment", Why: "Owns recovery", Escalation: "Escalate conflicts"})
	v, err := s.Create("repo", Outcome{Kind: "planned_outcome", ResourceID: "outcome", Title: "Outcome"}, c, "organizer")
	if err != nil {
		t.Fatal(err)
	}
	v, _ = s.Respond(v.ID, "alice-slot", "alice", "accepted", v.Version)
	v, _ = s.Respond(v.ID, "bob-slot", "bob", "accepted", v.Version)
	revision := strings.Repeat("a", 40)
	v, err = s.PutPlan(v.ID, "alice", "alice", v.Version, PlanInput{Streams: []WorkStream{{ID: "build", Title: "Build", OwnerParticipantID: "alice-slot", ExpectedArtifacts: []string{"accepted patch"}, AcceptanceCriteria: []string{"checks pass"}, RepositoryScope: []RevisionScope{{RepositoryID: "repo", Reference: "main", Revision: revision, Paths: []string{"src", "tests"}}}, IntegrationOrder: 1, Budget: &Budget{Unit: "minutes", Limit: 10}, Assumptions: []string{"base remains current"}}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.ReportStatus(v.ID, "build", "alice", "alice", v.Version, StatusInput{Status: "running", Summary: "Implementation is underway", ProgressPercent: 60, Revision: revision, ResourceUse: &ResourceUse{Unit: "minutes", Consumed: 10}, Questions: []StreamQuestion{{ID: "q1", Body: "Which compatibility edge wins?", AskOf: "organizer", Urgency: "urgent"}}, Blockers: []StreamBlocker{{Kind: "conflicting_output", Summary: "Two generated patches disagree", Recovery: "Keep the accepted patch and request a lead decision"}}, PredictedNextAction: "Compare both patches"})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.StreamStatuses[0]; got.Status != "paused" || got.PredictedNextAction != "Escalate the exhausted budget through the team charter" || !slices.ContainsFunc(got.Blockers, func(b StreamBlocker) bool { return b.Kind == "budget_exhausted" }) {
		t.Fatalf("bounded status = %#v", got)
	}
	if _, err = s.ReportStatus(v.ID, "build", "bob", "bob", v.Version, StatusInput{Status: "running", Summary: "Taking over", Revision: revision, PredictedNextAction: "Continue"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("silent owner expansion = %v", err)
	}
	if _, err = s.Intervene(v.ID, "organizer", "organizer", v.Version, InterventionInput{Scope: "stream", StreamID: "build", Action: "resume", Guidance: "Resume beyond the accepted budget"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unbounded resume = %v", err)
	}
	v, err = s.Intervene(v.ID, "organizer", "organizer", v.Version, InterventionInput{Scope: "stream", StreamID: "build", Action: "guide", Guidance: "Keep the accepted patch and isolate the disagreement"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Intervene(v.ID, "organizer", "organizer", v.Version, InterventionInput{Scope: "stream", StreamID: "build", Action: "narrow", Guidance: "Limit recovery to implementation", Paths: []string{"src"}})
	if err != nil || v.Plan.Revision != 2 || len(v.PlanHistory) != 1 || len(v.Plan.Streams[0].RepositoryScope[0].Paths) != 1 {
		t.Fatalf("narrow = %#v, %v", v.Plan, err)
	}
	v, err = s.Intervene(v.ID, "organizer", "organizer", v.Version, InterventionInput{Scope: "stream", StreamID: "build", Action: "reassign", Guidance: "Move only this stream to the recovery lead", NewOwnerParticipantID: "bob-slot"})
	if err != nil || v.Plan.Revision != 3 || v.Plan.Streams[0].OwnerParticipantID != "bob-slot" || len(v.Plan.Acceptances) != 1 || v.Plan.Acceptances[0].ParticipantID != "bob-slot" || v.Plan.Acceptances[0].Status != "pending" {
		t.Fatalf("reassign = %#v, %v", v.Plan, err)
	}
	if len(v.StreamStatuses) != 1 || len(v.Interventions) != 3 || v.StreamStatuses[0].Questions[0].ID != "q1" || v.StreamStatuses[0].ActiveControl != nil {
		t.Fatalf("accepted operational evidence discarded: %#v", v)
	}
	if _, err = s.Intervene(v.ID, "alice", "alice", v.Version, InterventionInput{Scope: "team", Action: "cancel", Guidance: "Cancel everything"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-organizer team control = %v", err)
	}
}
