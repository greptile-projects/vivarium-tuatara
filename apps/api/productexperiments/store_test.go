package productexperiments

import (
	"errors"
	"testing"
	"time"
)

func plan(signalStatus string) (Revision, []Signal) {
	return Revision{Hypothesis: "A clearer action increases completed work", Variants: []Variant{{Key: "control", Name: "Current", Description: "existing", Control: true}, {Key: "treatment", Name: "Clearer", Description: "new"}}, Audience: Audience{Description: "active collaborators", Eligibility: []string{"repository_collaborator"}}, Metrics: []Metric{{Name: "completion", Kind: "success", Direction: "increase", Threshold: 5, SignalID: "completed", SignalVersion: 1}, {Name: "errors", Kind: "guardrail", Direction: "below", Threshold: 2, SignalID: "errors", SignalVersion: 1}}, MinimumEvidence: 100, DurationDays: 14, Owners: []string{"owner"}, StopConditions: []string{"errors exceed 2%"}, Assumptions: []string{"traffic is stable"}, Rationale: "initial contract"}, []Signal{{ID: "completed", Name: "Completed", Version: 1, Event: "task.completed", Unit: "percent", Privacy: "aggregate", Status: signalStatus}, {ID: "errors", Name: "Errors", Version: 1, Event: "task.failed", Unit: "percent", Privacy: "aggregate", Status: "available"}}
}
func TestPlanDiagnosticsAndVersionBoundApproval(t *testing.T) {
	s, _ := New(t.TempDir())
	revision, signals := plan("planned")
	v, err := s.Create("repo", "alice", Source{Kind: "proposal", ResourceID: "p1", Label: "Clarify action"}, revision, signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Diagnostics) != 1 || v.Diagnostics[0].Kind != "missing_instrumentation" {
		t.Fatalf("diagnostics = %#v", v.Diagnostics)
	}
	v, _ = s.Approve(v.ID, "bob", "approve", "safe", 1)
	revision.Rationale = "audience assumption changed"
	signals[0].Status = "available"
	v, err = s.Revise(v.ID, 1, "alice", revision, signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Diagnostics) != 1 || v.Diagnostics[0].Kind != "changed_assumptions" || v.Diagnostics[0].AttributedTo != "bob" {
		t.Fatalf("diagnostics = %#v", v.Diagnostics)
	}
}
func TestOutcomeRequiresThresholdAndRetiresExperimentResources(t *testing.T) {
	s, _ := New(t.TempDir())
	s.ConfigureOutcomeEvidence(func(_, _, _, _, _ string, evidence TaskEvidence) bool {
		return evidence.Kind == "pull_request" && evidence.PullRequestID == "p1" && evidence.ResourceID == "p1"
	})
	revision, signals := plan("available")
	revision.MinimumEvidence = 2
	v, _ := s.Create("repo", "alice", Source{Kind: "proposal", ResourceID: "p1", Label: "test"}, revision, signals)
	v.RunAttempts = []RunAttempt{{ID: "run", ExperimentVersion: 1, Status: "running", Version: 1}}
	if err := s.write(v); err != nil {
		t.Fatal(err)
	}
	analysis := Analysis{RunID: "run", RunVersion: 1, SegmentEffects: []SegmentEffect{{Segment: "new users", Exposures: map[string]int{"control": 1, "treatment": 1}, MetricValues: map[string]float64{"completion": 8}, Uncertainty: map[string]float64{"completion": 1.2}}}, GuardrailOutcomes: []string{"errors stayed below 2%"}, Interpretation: "Treatment improves completion.", InterpretedByType: "agent", InterpretedByID: "forged-agent", Uncertainty: "95% interval overlaps a small neutral effect", Exclusions: []string{"staff traffic"}, Dissent: []string{"Bob prefers another week"}}
	if _, err := s.Analyze(v.ID, "alice", analysis); !errors.Is(err, ErrConflict) {
		t.Fatalf("premature analysis = %v", err)
	}
	raw, _ := s.read(v.ID)
	raw.RunAttempts[0].Observations = []RunObservation{{Exposures: map[string]int{"control": 1, "treatment": 1}}}
	if err := s.write(raw); err != nil {
		t.Fatal(err)
	}
	v, err := s.Analyze(v.ID, "alice", analysis)
	if err != nil || len(v.Analyses) != 1 || v.Analyses[0].ThresholdReason != "minimum_evidence_reached" {
		t.Fatalf("analysis=%#v %v", v.Analyses, err)
	}
	if v.Analyses[0].InterpretedByType != "human" || v.Analyses[0].InterpretedByID != "alice" {
		t.Fatalf("forged interpreter retained: %#v", v.Analyses[0])
	}
	tasks := []OutcomeTask{{Kind: "rollout", Title: "Roll out treatment"}, {Kind: "remove_variants", Title: "Delete control path"}, {Kind: "remove_targeting", Title: "Delete targeting flag"}, {Kind: "revoke_credentials", Title: "Revoke experiment credential"}, {Kind: "stop_collection", Title: "Retire experiment event"}, {Kind: "release", Title: "Ship cleanup"}}
	v, err = s.DecideOutcome(v.ID, "alice", 0, OutcomeDecision{AnalysisID: v.Analyses[0].ID, Decision: "adopt_variant", VariantKey: "treatment", Rationale: "Threshold met without guardrail harm", Tasks: tasks})
	if err != nil || len(v.OutcomeDecisions) != 1 || v.RunAttempts[0].Status != "stopped" {
		t.Fatalf("decision=%#v %v", v.OutcomeDecisions, err)
	}
	d := v.OutcomeDecisions[0]
	for _, task := range d.Tasks {
		if !task.Required {
			continue
		}
		v, err = s.CompleteOutcomeTask(v.ID, d.ID, task.ID, "alice", TaskEvidence{PullRequestID: "p1", Kind: "pull_request", ResourceID: "p1"}, d.Version)
		if err != nil {
			t.Fatal(err)
		}
		d = v.OutcomeDecisions[0]
	}
	if !d.CleanedUp {
		t.Fatalf("cleanup=%#v", d)
	}
	if len(v.Analyses) != 1 || len(v.RunAttempts[0].Observations) != 1 {
		t.Fatal("aggregated outcome evidence was discarded")
	}
}

func TestApprovedAgentAnalysisRetainsAgentAndOperatorAttribution(t *testing.T) {
	s, _ := New(t.TempDir())
	revision, signals := plan("available")
	v, _ := s.Create("repo", "alice", Source{Kind: "proposal", ResourceID: "p1", Label: "test"}, revision, signals)
	v.RunAttempts = []RunAttempt{{ID: "run", ExperimentVersion: 1, Status: "stopped", Version: 1}}
	if err := s.write(v); err != nil {
		t.Fatal(err)
	}
	analysis := Analysis{RunID: "run", RunVersion: 1, SegmentEffects: []SegmentEffect{{Segment: "new users", Exposures: map[string]int{"control": 1, "treatment": 1}}}, GuardrailOutcomes: []string{"errors stayed below 2%"}, Interpretation: "Treatment improves completion.", Uncertainty: "small neutral effect remains possible"}
	v, err := s.AnalyzeAsAgent(v.ID, "alice", "approved-agent", analysis)
	if err != nil || v.Analyses[0].InterpretedByType != "agent" || v.Analyses[0].InterpretedByID != "approved-agent" || v.Analyses[0].CreatedBy != "alice" {
		t.Fatalf("agent analysis attribution=%#v %v", v.Analyses, err)
	}
}
func TestFollowUpOutcomeAllowsRelaunchOnlyAfterCleanup(t *testing.T) {
	s, _ := New(t.TempDir())
	s.ConfigureDeploymentHealth(func(string, []string) (bool, error) { return true, nil })
	revision, signals := plan("available")
	v, _ := s.Create("repo", "alice", Source{Kind: "proposal", ResourceID: "p", Label: "test"}, revision, signals)
	v.AudienceContracts = []AudienceContract{{ID: "contract", ExperimentVersion: 1, VariantKeys: []string{"control", "treatment"}, Allocation: []Allocation{{VariantKey: "control", BasisPoints: 5000}, {VariantKey: "treatment", BasisPoints: 5000}}}}
	v.OutcomeDecisions = []OutcomeDecision{{Decision: "extend_test", ExperimentVersion: 1, CleanedUp: false}}
	if err := s.write(v); err != nil {
		t.Fatal(err)
	}
	allocation := []RunAllocation{{VariantKey: "control", BasisPoints: 5000}, {VariantKey: "treatment", BasisPoints: 5000}}
	if _, err := s.Launch(v.ID, "alice", "contract", []string{"deployment"}, []string{"production"}, allocation); !errors.Is(err, ErrConflict) {
		t.Fatalf("unclean relaunch = %v", err)
	}
	raw, _ := s.read(v.ID)
	raw.OutcomeDecisions[0].CleanedUp = true
	if err := s.write(raw); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Launch(v.ID, "alice", "contract", []string{"deployment"}, []string{"production"}, allocation); !errors.Is(err, ErrConflict) {
		t.Fatalf("retired plan relaunch = %v", err)
	}
	raw, _ = s.read(v.ID)
	raw.CurrentVersion = 2
	revision.Version, revision.CreatedBy, revision.CreatedAt = 2, "alice", s.now()
	raw.Revisions = append(raw.Revisions, revision)
	raw.AudienceContracts = append(raw.AudienceContracts, AudienceContract{ID: "successor", ExperimentVersion: 2, VariantKeys: []string{"control", "treatment"}, Allocation: []Allocation{{VariantKey: "control", BasisPoints: 5000}, {VariantKey: "treatment", BasisPoints: 5000}}})
	if err := s.write(raw); err != nil {
		t.Fatal(err)
	}
	if launched, err := s.Launch(v.ID, "alice", "successor", []string{"deployment"}, []string{"production"}, allocation); err != nil || len(launched.RunAttempts) != 1 {
		t.Fatalf("successor follow-up relaunch = %#v, %v", launched.RunAttempts, err)
	}
}
func TestOverlapRequiresSharedAudienceAndSignal(t *testing.T) {
	s, _ := New(t.TempDir())
	revision, signals := plan("available")
	a, _ := s.Create("repo", "alice", Source{Kind: "issue", ResourceID: "i1", Label: "one"}, revision, signals)
	b, _ := s.Create("repo", "bob", Source{Kind: "release", ResourceID: "r1", Label: "two"}, revision, signals)
	if !Overlaps(a, b) {
		t.Fatal("expected overlap")
	}
	revision.Audience.Eligibility = []string{"organization_member"}
	c, _ := s.Create("repo", "carol", Source{Kind: "preview", ResourceID: "v1", Label: "three"}, revision, signals)
	if Overlaps(a, c) {
		t.Fatal("unexpected overlap")
	}
}
func TestWorkLinksFreezeReviewEvidenceAtPlanVersion(t *testing.T) {
	s, _ := New(t.TempDir())
	revision, signals := plan("available")
	v, _ := s.Create("repo", "alice", Source{Kind: "proposal", ResourceID: "p1", Label: "test"}, revision, signals)
	work := WorkLink{VariantKeys: []string{"control", "treatment"}, OwnerType: "agent", OwnerID: "agent-1", ProposalID: "p1", TaskID: "t1", SessionID: "s1", WorkspaceID: "w1", PullRequestID: "pull-1", CommitID: "0123456789012345678901234567890123456789", EventDefinitions: []string{"task.completed@1"}, ExposureRules: []string{"repository collaborators, 50/50"}, Privacy: "aggregate", RemovalPlan: "Delete the flag and event after the decision.", CheckNames: []string{"experiment/assignment", "experiment/fallback"}}
	v, err := s.LinkWork(v.ID, "alice", 1, work)
	if err != nil || len(v.Work) != 1 || v.Work[0].ExperimentVersion != 1 || v.Work[0].LinkedBy != "alice" {
		t.Fatalf("work = %#v, %v", v.Work, err)
	}
	if _, err = s.LinkWork(v.ID, "alice", 1, work); err != nil {
		t.Fatalf("idempotent link: %v", err)
	}
	changed := work
	changed.EventDefinitions = []string{"task.failed@9"}
	if _, err = s.LinkWork(v.ID, "alice", 1, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed evidence = %v", err)
	}
	if _, err = s.LinkWork(v.ID, "mallory", 1, work); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed retry actor = %v", err)
	}
	revision.Rationale = "successor plan"
	if _, err = s.Revise(v.ID, 1, "alice", revision, signals); err != nil {
		t.Fatalf("revise = %v", err)
	}
	if retried, retryErr := s.LinkWork(v.ID, "alice", 1, work); retryErr != nil || len(retried.Work) != 1 {
		t.Fatalf("historical exact retry = %#v, %v", retried.Work, retryErr)
	}
	if replayed, exact, replayErr := s.ExistingWorkReplay(v.ID, "alice", 1, work); replayErr != nil || !exact || len(replayed.Work) != 1 {
		t.Fatalf("external-state-independent replay = %#v, %t, %v", replayed.Work, exact, replayErr)
	}
	if _, exact, replayErr := s.ExistingWorkReplay(v.ID, "mallory", 1, work); exact || !errors.Is(replayErr, ErrConflict) {
		t.Fatalf("changed replay actor = %t, %v", exact, replayErr)
	}
	newWork := work
	newWork.PullRequestID = "pull-2"
	if _, exact, replayErr := s.ExistingWorkReplay(v.ID, "alice", 2, newWork); replayErr != nil || exact {
		t.Fatalf("new work classified as replay = %t, %v", exact, replayErr)
	}
	work.CommitID = "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	if _, err = s.LinkWork(v.ID, "alice", 1, work); !errors.Is(err, ErrConflict) {
		t.Fatalf("moved pull = %v", err)
	}
}

func TestAudienceContractRequiresExactWorkAndKeepsAssignmentStableAndRedacted(t *testing.T) {
	s, _ := New(t.TempDir())
	revision, signals := plan("available")
	v, _ := s.Create("repo", "alice", Source{Kind: "release", ResourceID: "release-1", Label: "one"}, revision, signals)
	commit := "0123456789012345678901234567890123456789"
	work := WorkLink{VariantKeys: []string{"control", "treatment"}, OwnerType: "human", OwnerID: "alice", ProposalID: "p1", TaskID: "t1", PullRequestID: "pull-1", CommitID: commit, EventDefinitions: []string{"task.completed@1"}, ExposureRules: []string{"approved contract"}, Privacy: "consented", RemovalPlan: "remove", CheckNames: []string{"experiment/verified"}}
	v, _ = s.LinkWork(v.ID, "alice", 1, work)
	contract := AudienceContract{ReleaseID: "release-1", ReleaseCommitID: commit, VariantKeys: []string{"control", "treatment"}, Eligibility: []string{"repository_collaborator"}, Regions: []string{"EU"}, OrganizationIDs: []string{"org-1"}, RandomizationUnit: "user", MutualExclusionGroup: "onboarding", Allocation: []Allocation{{VariantKey: "control", BasisPoints: 4000}, {VariantKey: "treatment", BasisPoints: 4000}}, Consent: "explicit", DataFields: []string{"assignment", "metric"}, RetentionDays: 30}
	v, err := s.ApproveAudience(v.ID, "alice", 1, contract)
	if err != nil || len(v.AudienceContracts) != 1 || v.AudienceContracts[0].RandomizationSalt == "" {
		t.Fatalf("contract = %#v, %v", v.AudienceContracts, err)
	}
	context := AssignmentContext{Eligibility: []string{"repository_collaborator"}, Region: "EU", OrganizationID: "org-1"}
	_, denied, err := s.Assign(v.ID, v.AudienceContracts[0].ID, "sensitive-user-id", context)
	if err != nil || denied.Eligible || denied.Reason != "consent_required" || denied.SubjectDigest == "sensitive-user-id" {
		t.Fatalf("denied = %#v, %v", denied, err)
	}
	_, outside, err := s.Assign(v.ID, v.AudienceContracts[0].ID, "outside", AssignmentContext{Eligibility: []string{"repository_collaborator"}, Region: "US", OrganizationID: "org-1", Consented: true})
	if err != nil || outside.Eligible || outside.Reason != "audience_ineligible" {
		t.Fatalf("outside audience = %#v, %v", outside, err)
	}
	context.Consented = true
	_, repeat, err := s.Assign(v.ID, v.AudienceContracts[0].ID, "sensitive-user-id", context)
	if err != nil || repeat.ID != denied.ID || repeat.VariantKey != denied.VariantKey {
		t.Fatalf("stable = %#v, %v", repeat, err)
	}
	if _, err = s.ApproveAudience(v.ID, "alice", 1, contract); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting allocation = %v", err)
	}
	stale := revision
	stale.Rationale = "changed"
	v, _ = s.Revise(v.ID, 1, "alice", stale, signals)
	if _, _, err = s.Assign(v.ID, v.AudienceContracts[0].ID, "other", context); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale assignment = %v", err)
	}
}

func TestAudienceContractRejectsBiasedOrUnauthorizedCollection(t *testing.T) {
	s, _ := New(t.TempDir())
	revision, signals := plan("available")
	v, _ := s.Create("repo", "alice", Source{Kind: "release", ResourceID: "r", Label: "r"}, revision, signals)
	commit := "0123456789012345678901234567890123456789"
	work := WorkLink{VariantKeys: []string{"control", "treatment"}, OwnerType: "human", OwnerID: "alice", ProposalID: "p", TaskID: "t", PullRequestID: "pull", CommitID: commit, EventDefinitions: []string{"e@1"}, ExposureRules: []string{"rule"}, Privacy: "aggregate", RemovalPlan: "remove", CheckNames: []string{"check"}}
	v, _ = s.LinkWork(v.ID, "alice", 1, work)
	bad := AudienceContract{ReleaseID: "r", ReleaseCommitID: commit, VariantKeys: []string{"control", "treatment"}, Eligibility: []string{"collaborator"}, RandomizationUnit: "user", MutualExclusionGroup: "g", Allocation: []Allocation{{VariantKey: "control", BasisPoints: 9000}, {VariantKey: "treatment", BasisPoints: 9000}}, Consent: "none", DataFields: []string{"email"}, RetentionDays: 30}
	if _, err := s.ApproveAudience(v.ID, "alice", 1, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("biased/private contract = %v", err)
	}
}

func TestMutualExclusionSpansRepositoryExperiments(t *testing.T) {
	s, _ := New(t.TempDir())
	revision, signals := plan("available")
	commit := "0123456789012345678901234567890123456789"
	create := func(label string) Experiment {
		v, _ := s.Create("repo", "alice", Source{Kind: "release", ResourceID: label, Label: label}, revision, signals)
		work := WorkLink{VariantKeys: []string{"control", "treatment"}, OwnerType: "human", OwnerID: "alice", ProposalID: "p", TaskID: "t", PullRequestID: "pull-" + label, CommitID: commit, EventDefinitions: []string{"e@1"}, ExposureRules: []string{"rule"}, Privacy: "aggregate", RemovalPlan: "remove", CheckNames: []string{"check"}}
		v, _ = s.LinkWork(v.ID, "alice", 1, work)
		contract := AudienceContract{ReleaseID: label, ReleaseCommitID: commit, VariantKeys: []string{"control", "treatment"}, Eligibility: []string{"member"}, RandomizationUnit: "user", MutualExclusionGroup: "onboarding", Allocation: []Allocation{{VariantKey: "control", BasisPoints: 5000}, {VariantKey: "treatment", BasisPoints: 5000}}, Consent: "none", DataFields: []string{"assignment"}, RetentionDays: 30}
		v, _ = s.ApproveAudience(v.ID, "alice", 1, contract)
		return v
	}
	a, b := create("r1"), create("r2")
	context := AssignmentContext{Eligibility: []string{"member"}, Consented: true}
	_, first, err := s.Assign(a.ID, a.AudienceContracts[0].ID, "subject", context)
	if err != nil || !first.Eligible {
		t.Fatalf("first=%#v %v", first, err)
	}
	_, second, err := s.Assign(b.ID, b.AudienceContracts[0].ID, "subject", context)
	if err != nil || second.Eligible || second.Reason != "mutually_excluded" {
		t.Fatalf("second=%#v %v", second, err)
	}
	_, firstAfter, err := s.Assign(a.ID, a.AudienceContracts[0].ID, "after-prune", context)
	if err != nil || !firstAfter.Eligible {
		t.Fatalf("first after prune=%#v %v", firstAfter, err)
	}
	raw, _ := s.read(a.ID)
	raw.AssignmentAudit = nil
	if err = s.write(raw); err != nil {
		t.Fatal(err)
	}
	_, blocked, err := s.Assign(b.ID, b.AudienceContracts[0].ID, "after-prune", context)
	if err != nil || blocked.Eligible || blocked.Reason != "mutually_excluded" {
		t.Fatalf("membership after audit prune=%#v %v", blocked, err)
	}
}

func TestScheduledCleanupRemovesInactiveAuditAtRest(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().Add(-48 * time.Hour)
	s.now = func() time.Time { return now }
	revision, signals := plan("available")
	v, _ := s.Create("repo", "alice", Source{Kind: "release", ResourceID: "r", Label: "r"}, revision, signals)
	commit := "0123456789012345678901234567890123456789"
	work := WorkLink{VariantKeys: []string{"control", "treatment"}, OwnerType: "human", OwnerID: "alice", ProposalID: "p", TaskID: "t", PullRequestID: "pull", CommitID: commit, EventDefinitions: []string{"e@1"}, ExposureRules: []string{"rule"}, Privacy: "aggregate", RemovalPlan: "remove", CheckNames: []string{"check"}}
	v, _ = s.LinkWork(v.ID, "alice", 1, work)
	contract := AudienceContract{ReleaseID: "r", ReleaseCommitID: commit, VariantKeys: []string{"control", "treatment"}, Eligibility: []string{"member"}, RandomizationUnit: "user", MutualExclusionGroup: "g", Allocation: []Allocation{{VariantKey: "control", BasisPoints: 5000}, {VariantKey: "treatment", BasisPoints: 5000}}, Consent: "none", DataFields: []string{"assignment"}, RetentionDays: 1}
	v, _ = s.ApproveAudience(v.ID, "alice", 1, contract)
	v, _, _ = s.Assign(v.ID, v.AudienceContracts[0].ID, "subject", AssignmentContext{Eligibility: []string{"member"}})
	s.now = func() time.Time { return time.Now() }
	s.scheduleCleanupAt(time.Now().Add(20 * time.Millisecond))
	deadline := time.Now().Add(time.Second)
	for {
		raw, _ := s.read(v.ID)
		if len(raw.AssignmentAudit) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("inactive audit remains at rest: %#v", raw.AssignmentAudit)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMutuallyExcludedReceiptSchedulesInactiveCleanup(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().Add(-48 * time.Hour)
	s.now = func() time.Time { return now }
	revision, signals := plan("available")
	commit := "0123456789012345678901234567890123456789"
	create := func(label string) Experiment {
		v, _ := s.Create("repo", "alice", Source{Kind: "release", ResourceID: label, Label: label}, revision, signals)
		work := WorkLink{VariantKeys: []string{"control", "treatment"}, OwnerType: "human", OwnerID: "alice", ProposalID: "p", TaskID: "t", PullRequestID: "pull-" + label, CommitID: commit, EventDefinitions: []string{"e@1"}, ExposureRules: []string{"rule"}, Privacy: "aggregate", RemovalPlan: "remove", CheckNames: []string{"check"}}
		v, _ = s.LinkWork(v.ID, "alice", 1, work)
		contract := AudienceContract{ReleaseID: label, ReleaseCommitID: commit, VariantKeys: []string{"control", "treatment"}, Eligibility: []string{"member"}, RandomizationUnit: "user", MutualExclusionGroup: "g", Allocation: []Allocation{{VariantKey: "control", BasisPoints: 5000}, {VariantKey: "treatment", BasisPoints: 5000}}, Consent: "none", DataFields: []string{"assignment"}, RetentionDays: 1}
		v, _ = s.ApproveAudience(v.ID, "alice", 1, contract)
		return v
	}
	a, b := create("a"), create("b")
	context := AssignmentContext{Eligibility: []string{"member"}}
	_, first, err := s.Assign(a.ID, a.AudienceContracts[0].ID, "subject", context)
	if err != nil || !first.Eligible {
		t.Fatalf("first=%#v %v", first, err)
	}
	_, blocked, err := s.Assign(b.ID, b.AudienceContracts[0].ID, "subject", context)
	if err != nil || blocked.Eligible || blocked.Reason != "mutually_excluded" {
		t.Fatalf("blocked=%#v %v", blocked, err)
	}
	s.now = func() time.Time { return time.Now() }
	s.scheduleCleanupAt(time.Now().Add(20 * time.Millisecond))
	deadline := time.Now().Add(time.Second)
	for {
		raw, _ := s.read(b.ID)
		if len(raw.AssignmentAudit) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("excluded audit remains at rest: %#v", raw.AssignmentAudit)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAssignmentRetentionPrunesReadsAndPersistence(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	revision, signals := plan("available")
	v, _ := s.Create("repo", "alice", Source{Kind: "release", ResourceID: "r", Label: "r"}, revision, signals)
	commit := "0123456789012345678901234567890123456789"
	work := WorkLink{VariantKeys: []string{"control", "treatment"}, OwnerType: "human", OwnerID: "alice", ProposalID: "p", TaskID: "t", PullRequestID: "pull", CommitID: commit, EventDefinitions: []string{"e@1"}, ExposureRules: []string{"rule"}, Privacy: "aggregate", RemovalPlan: "remove", CheckNames: []string{"check"}}
	v, _ = s.LinkWork(v.ID, "alice", 1, work)
	contract := AudienceContract{ReleaseID: "r", ReleaseCommitID: commit, VariantKeys: []string{"control", "treatment"}, Eligibility: []string{"member"}, RandomizationUnit: "user", MutualExclusionGroup: "g", Allocation: []Allocation{{VariantKey: "control", BasisPoints: 5000}, {VariantKey: "treatment", BasisPoints: 5000}}, Consent: "none", DataFields: []string{"assignment"}, RetentionDays: 1}
	v, _ = s.ApproveAudience(v.ID, "alice", 1, contract)
	v, _, _ = s.Assign(v.ID, v.AudienceContracts[0].ID, "subject", AssignmentContext{Eligibility: []string{"member"}})
	if len(v.AssignmentAudit) != 1 {
		t.Fatal("missing receipt")
	}
	now = now.Add(24 * time.Hour)
	v, err := s.Get(v.ID)
	if err != nil || len(v.AssignmentAudit) != 0 {
		t.Fatalf("get=%#v %v", v.AssignmentAudit, err)
	}
	raw, err := s.read(v.ID)
	if err != nil || len(raw.AssignmentAudit) != 0 {
		t.Fatalf("persisted=%#v %v", raw.AssignmentAudit, err)
	}
}

func TestRunStagesContainmentAndStableAssignments(t *testing.T) {
	s, _ := New(t.TempDir())
	s.ConfigureDeploymentHealth(func(string, []string) (bool, error) { return true, nil })
	revision, signals := plan("available")
	v, _ := s.Create("repo", "alice", Source{Kind: "release", ResourceID: "r", Label: "r"}, revision, signals)
	commit := "0123456789012345678901234567890123456789"
	v, _ = s.LinkWork(v.ID, "alice", 1, WorkLink{VariantKeys: []string{"control", "treatment"}, OwnerType: "human", OwnerID: "alice", ProposalID: "p", TaskID: "t", PullRequestID: "pull", CommitID: commit, EventDefinitions: []string{"e@1"}, ExposureRules: []string{"rule"}, Privacy: "aggregate", RemovalPlan: "remove", CheckNames: []string{"check"}})
	v, _ = s.ApproveAudience(v.ID, "alice", 1, AudienceContract{ReleaseID: "r", ReleaseCommitID: commit, VariantKeys: []string{"control", "treatment"}, Eligibility: []string{"member"}, RandomizationUnit: "user", MutualExclusionGroup: "g", Allocation: []Allocation{{VariantKey: "control", BasisPoints: 5000}, {VariantKey: "treatment", BasisPoints: 5000}}, Consent: "explicit", DataFields: []string{"assignment", "metric"}, RetentionDays: 30})
	contract := v.AudienceContracts[0]
	v, err := s.Launch(v.ID, "alice", contract.ID, []string{"deployment-1"}, []string{"production"}, []RunAllocation{{VariantKey: "control", BasisPoints: 500}, {VariantKey: "treatment", BasisPoints: 500}})
	if err != nil || v.RunAttempts[0].Status != "running" {
		t.Fatalf("launch = %#v %v", v.RunAttempts, err)
	}
	run := v.RunAttempts[0]
	v, err = s.Stage(v.ID, run.ID, "bob", 1, []RunAllocation{{VariantKey: "control", BasisPoints: 1000}, {VariantKey: "treatment", BasisPoints: 1000}}, "healthy canary")
	if err != nil || v.RunAttempts[0].Version != 2 {
		t.Fatalf("stage = %#v %v", v.RunAttempts, err)
	}
	_, prior, err := s.Assign(v.ID, contract.ID, "prior", AssignmentContext{Eligibility: []string{"member"}, Consented: true})
	if err != nil || !prior.Eligible {
		t.Fatalf("prior assignment = %#v %v", prior, err)
	}
	v, err = s.Observe(v.ID, run.ID, "bob", RunObservation{IdempotencyKey: "sample-1", StageVersion: 2, Exposures: map[string]int{"control": 50, "treatment": 50}, MetricValues: map[string]float64{"errors": 3}, MetricSamples: map[string]int{"errors": 100}, Uncertainty: map[string]float64{"errors": 0.2}, Cost: 4.5, InstrumentationOK: true, ConsentCurrent: true, DeploymentHealthy: true, SampleBalanced: true})
	if err != nil || v.RunAttempts[0].Status != "contained" || v.RunAttempts[0].ContainmentReason != "guardrail_breach:errors" {
		t.Fatalf("containment = %#v %v", v.RunAttempts, err)
	}
	if retried, retryErr := s.Observe(v.ID, run.ID, "bob", RunObservation{IdempotencyKey: "sample-1", StageVersion: 2, Exposures: map[string]int{"control": 50, "treatment": 50}, MetricValues: map[string]float64{"errors": 3}, MetricSamples: map[string]int{"errors": 100}, Uncertainty: map[string]float64{"errors": 0.2}, Cost: 4.5, InstrumentationOK: true, ConsentCurrent: true, DeploymentHealthy: true, SampleBalanced: true}); retryErr != nil || len(retried.RunAttempts[0].Observations) != 1 {
		t.Fatalf("contained retry = %#v %v", retried.RunAttempts, retryErr)
	}
	_, repeat, _ := s.Assign(v.ID, contract.ID, "prior", AssignmentContext{Eligibility: []string{"member"}, Consented: true})
	_, blocked, _ := s.Assign(v.ID, contract.ID, "new", AssignmentContext{Eligibility: []string{"member"}, Consented: true})
	if repeat.ID != prior.ID || blocked.Eligible || blocked.Reason != "experiment_contained" {
		t.Fatalf("stable=%#v blocked=%#v", repeat, blocked)
	}
}

func TestRunRejectsAllocationAboveApprovedCapAndContainsQualityLoss(t *testing.T) {
	s, _ := New(t.TempDir())
	s.ConfigureDeploymentHealth(func(string, []string) (bool, error) { return true, nil })
	revision, signals := plan("available")
	v, _ := s.Create("repo", "alice", Source{Kind: "release", ResourceID: "r", Label: "r"}, revision, signals)
	commit := "0123456789012345678901234567890123456789"
	v, _ = s.LinkWork(v.ID, "alice", 1, WorkLink{VariantKeys: []string{"control", "treatment"}, OwnerType: "human", OwnerID: "alice", ProposalID: "p", TaskID: "t", PullRequestID: "pull", CommitID: commit, EventDefinitions: []string{"e@1"}, ExposureRules: []string{"rule"}, Privacy: "aggregate", RemovalPlan: "remove", CheckNames: []string{"check"}})
	v, _ = s.ApproveAudience(v.ID, "alice", 1, AudienceContract{ReleaseID: "r", ReleaseCommitID: commit, VariantKeys: []string{"control", "treatment"}, Eligibility: []string{"member"}, RandomizationUnit: "user", MutualExclusionGroup: "g", Allocation: []Allocation{{VariantKey: "control", BasisPoints: 1000}, {VariantKey: "treatment", BasisPoints: 1000}}, Consent: "none", DataFields: []string{"assignment"}, RetentionDays: 30})
	c := v.AudienceContracts[0]
	if _, err := s.Launch(v.ID, "alice", c.ID, []string{"d"}, []string{"prod"}, []RunAllocation{{VariantKey: "control", BasisPoints: 1001}, {VariantKey: "treatment", BasisPoints: 1000}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("over cap = %v", err)
	}
	v, _ = s.Launch(v.ID, "alice", c.ID, []string{"d"}, []string{"prod"}, []RunAllocation{{VariantKey: "control", BasisPoints: 500}, {VariantKey: "treatment", BasisPoints: 500}})
	run := v.RunAttempts[0]
	v, err := s.Observe(v.ID, run.ID, "alice", RunObservation{IdempotencyKey: "quality", StageVersion: 1, InstrumentationOK: false, ConsentCurrent: true, DeploymentHealthy: true, SampleBalanced: true})
	if err != nil || v.RunAttempts[0].ContainmentReason != "instrumentation_loss" {
		t.Fatalf("quality containment=%#v %v", v.RunAttempts, err)
	}
}

func TestRunUsesLaunchRevisionGuardrail(t *testing.T) {
	s, _ := New(t.TempDir())
	s.ConfigureDeploymentHealth(func(string, []string) (bool, error) { return true, nil })
	revision, signals := plan("available")
	v, _ := s.Create("repo", "alice", Source{Kind: "release", ResourceID: "r", Label: "r"}, revision, signals)
	commit := "0123456789012345678901234567890123456789"
	v, _ = s.LinkWork(v.ID, "alice", 1, WorkLink{VariantKeys: []string{"control", "treatment"}, OwnerType: "human", OwnerID: "alice", ProposalID: "p", TaskID: "t", PullRequestID: "pull", CommitID: commit, EventDefinitions: []string{"e@1"}, ExposureRules: []string{"rule"}, Privacy: "aggregate", RemovalPlan: "remove", CheckNames: []string{"check"}})
	v, _ = s.ApproveAudience(v.ID, "alice", 1, AudienceContract{ReleaseID: "r", ReleaseCommitID: commit, VariantKeys: []string{"control", "treatment"}, Eligibility: []string{"member"}, RandomizationUnit: "user", MutualExclusionGroup: "g", Allocation: []Allocation{{VariantKey: "control", BasisPoints: 1000}, {VariantKey: "treatment", BasisPoints: 1000}}, Consent: "none", DataFields: []string{"assignment"}, RetentionDays: 30})
	v, _ = s.Launch(v.ID, "alice", v.AudienceContracts[0].ID, []string{"d"}, []string{"prod"}, []RunAllocation{{VariantKey: "control", BasisPoints: 500}, {VariantKey: "treatment", BasisPoints: 500}})
	revision.Metrics[1].Threshold = 10
	v, _ = s.Revise(v.ID, 1, "alice", revision, signals)
	v, err := s.Observe(v.ID, v.RunAttempts[0].ID, "alice", RunObservation{IdempotencyKey: "launch-guardrail", StageVersion: 1, MetricValues: map[string]float64{"errors": 3}, InstrumentationOK: true, ConsentCurrent: true, DeploymentHealthy: true, SampleBalanced: true})
	if err != nil || v.RunAttempts[0].ContainmentReason != "guardrail_breach:errors" {
		t.Fatalf("launch guardrail = %#v %v", v.RunAttempts, err)
	}
}

func TestAssignmentContainsFailedOrUnreadableDeployment(t *testing.T) {
	for _, test := range []struct {
		name    string
		healthy bool
		err     error
		reason  string
	}{{"failed", false, nil, "deployment_failure"}, {"unreadable", false, errors.New("offline"), "deployment_health_unavailable"}} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := New(t.TempDir())
			s.ConfigureDeploymentHealth(func(string, []string) (bool, error) { return test.healthy, test.err })
			revision, signals := plan("available")
			v, _ := s.Create("repo", "alice", Source{Kind: "release", ResourceID: "r", Label: "r"}, revision, signals)
			commit := "0123456789012345678901234567890123456789"
			v, _ = s.LinkWork(v.ID, "alice", 1, WorkLink{VariantKeys: []string{"control", "treatment"}, OwnerType: "human", OwnerID: "alice", ProposalID: "p", TaskID: "t", PullRequestID: "pull", CommitID: commit, EventDefinitions: []string{"e@1"}, ExposureRules: []string{"rule"}, Privacy: "aggregate", RemovalPlan: "remove", CheckNames: []string{"check"}})
			v, _ = s.ApproveAudience(v.ID, "alice", 1, AudienceContract{ReleaseID: "r", ReleaseCommitID: commit, VariantKeys: []string{"control", "treatment"}, Eligibility: []string{"member"}, RandomizationUnit: "user", MutualExclusionGroup: "g", Allocation: []Allocation{{VariantKey: "control", BasisPoints: 1000}, {VariantKey: "treatment", BasisPoints: 1000}}, Consent: "none", DataFields: []string{"assignment"}, RetentionDays: 30})
			c := v.AudienceContracts[0]
			v, _ = s.Launch(v.ID, "alice", c.ID, []string{"d"}, []string{"prod"}, []RunAllocation{{VariantKey: "control", BasisPoints: 500}, {VariantKey: "treatment", BasisPoints: 500}})
			v, receipt, err := s.Assign(v.ID, c.ID, "new", AssignmentContext{Eligibility: []string{"member"}})
			if err != nil || receipt.Eligible || receipt.Reason != "experiment_contained" || v.RunAttempts[0].ContainmentReason != test.reason {
				t.Fatalf("assignment=%#v run=%#v err=%v", receipt, v.RunAttempts, err)
			}
		})
	}
}

func TestPlanRejectsUnsupportedMetricDirection(t *testing.T) {
	s, _ := New(t.TempDir())
	revision, signals := plan("available")
	revision.Metrics[1].Direction = "sideways"
	if _, err := s.Create("repo", "alice", Source{Kind: "release", ResourceID: "r", Label: "r"}, revision, signals); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid direction = %v", err)
	}
}
