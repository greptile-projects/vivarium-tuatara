package decisions

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDecisionRetainsVersionedScopeAndDiscussion(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	deadline := now.Add(24 * time.Hour)
	v, err := s.Create("repo", Source{Kind: "proposal", ResourceID: "proposal"}, Scope{Question: "Which queue?", Constraints: []string{"No downtime"}, SuccessMeasures: []string{"p95 improves"}, Deadline: &deadline, OwnerID: "owner", Participants: []Participant{{UserID: "owner"}, {UserID: "peer"}}, AffectedResources: []Resource{{Kind: "service", Label: "API"}}}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "pending" || v.Version != 1 || len(v.History) != 1 {
		t.Fatalf("created = %#v", v)
	}
	originalJoined := v.Scope.Participants[1].AddedAt
	now = now.Add(time.Hour)
	scope := v.Scope
	scope.Constraints = []string{"No downtime", "No new broker"}
	scope.Participants = append(scope.Participants, Participant{UserID: "reviewer", AddedBy: "forged", AddedAt: time.Now()})
	v, err = s.Update(v.ID, "peer", 1, scope, "Added the operating constraint")
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 2 || len(v.History) != 2 || v.Scope.Participants[1].AddedAt != originalJoined || v.Scope.Participants[2].AddedBy != "peer" || !v.Scope.Participants[2].AddedAt.Equal(now) {
		t.Fatalf("updated = %#v", v)
	}
	if _, err = s.Update(v.ID, "peer", 1, scope, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale err = %v", err)
	}
	now = now.Add(time.Hour)
	v, err = s.Discuss(v.ID, "reviewer", "We need migration evidence.")
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 2 || len(v.History) != 3 || v.History[2].Kind != "discussion" || v.History[2].Body == "" {
		t.Fatalf("discussed = %#v", v)
	}
}

func TestExperimentRetainsAttributedWorkspaceEvidence(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 10, 19, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	deadline := now.Add(24 * time.Hour)
	v, err := s.Create("repo", Source{Kind: "repository", ResourceID: "repo"}, Scope{Question: "Which parser?", Constraints: []string{"No production writes"}, SuccessMeasures: []string{"Throughput"}, Deadline: &deadline, OwnerID: "owner", Participants: []Participant{{UserID: "owner"}}, AffectedResources: []Resource{{Kind: "repository", Label: "Parser"}}}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	evidence := []Evidence{{Kind: "usage", ResourceID: "baseline", Revision: "window", Label: "baseline"}}
	v, err = s.AddAlternative(v.ID, "owner", 1, Alternative{Title: "Streaming", Summary: "Stream input", Assumptions: []string{"Input is ordered"}, Tradeoffs: []string{"More state"}, Risks: []string{"Partial reads"}, CompatibilityImpact: "None", Cost: "One day", ExpectedOutcomes: []string{"Higher throughput"}, Evidence: evidence, Criteria: []CriterionAssessment{{Criterion: "Throughput", Outcome: "Not yet demonstrated", Evidence: evidence}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.LaunchExperiment(v.ID, "owner", v.Alternatives[0].ID, "workspace", "0123456789abcdef", "definition-sha", "default-revision", "default-definition", []string{"benchmark"})
	if err != nil || len(v.Experiments) != 1 {
		t.Fatalf("launch = %#v, %v", v.Experiments, err)
	}
	experiment := v.Experiments[0]
	v, err = s.LaunchExperiment(v.ID, "owner", v.Alternatives[0].ID, "workspace", "0123456789abcdef", "definition-sha", "default-revision", "default-definition", []string{"benchmark"})
	if err != nil || len(v.Experiments) != 1 {
		t.Fatalf("idempotent launch = %#v, %v", v.Experiments, err)
	}
	artifactSHA := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	v, err = s.AttachExperimentEvidence(v.ID, experiment.ID, "owner", 1, ExperimentEvidence{CheckpointIDs: []string{"checkpoint"}, CommandIDs: []string{"command"}, Measurements: []Measurement{{Name: "throughput", Value: 1420, Unit: "requests/s"}}, Artifacts: []Artifact{{Label: "profile", Path: "artifacts/profile.pb", SHA256: artifactSHA, Size: 42}}, CPUSeconds: 12, MemoryMBHours: 0.2, StorageMBHours: 0.1, Notes: "Three bounded runs"})
	if err != nil || v.Experiments[0].Version != 2 || v.Experiments[0].Evidence[0].RecordedBy != "owner" {
		t.Fatalf("evidence = %#v, %v", v.Experiments[0], err)
	}
	if _, err = s.AttachExperimentEvidence(v.ID, experiment.ID, "owner", 1, ExperimentEvidence{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale evidence = %v", err)
	}
}

func TestDecisionRejectsUnknownParticipantAndInvalidScope(t *testing.T) {
	s, _ := New(t.TempDir())
	deadline := time.Now().Add(time.Hour)
	v, err := s.Create("repo", Source{Kind: "repository"}, Scope{Question: "Question?", Constraints: []string{"constraint"}, SuccessMeasures: []string{"measure"}, AffectedResources: []Resource{{Kind: "repository", Label: "repo"}}, Deadline: &deadline, OwnerID: "owner", Participants: []Participant{{UserID: "owner"}}}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Discuss(v.ID, "outsider", "hello"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider err = %v", err)
	}
	bad := v.Scope
	bad.OwnerID = "missing"
	if _, err = s.Update(v.ID, "owner", 1, bad, "bad"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("owner err = %v", err)
	}
}

func TestAcceptedCommitmentLinksOneImmutableImplementation(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 10, 21, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	deadline := now.Add(time.Hour)
	v, err := s.Create("repo", Source{Kind: "repository"}, Scope{Question: "Ship it?", Constraints: []string{"No downtime"}, SuccessMeasures: []string{"p95 under 100ms"}, Deadline: &deadline, AffectedResources: []Resource{{Kind: "repository", Label: "repo"}}, Participants: []Participant{{UserID: "owner"}}, OwnerID: "owner"}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	v.Status = "published"
	v.Commitments = []Commitment{{Version: 1, Status: "published"}}
	if err := s.write(v); err != nil {
		t.Fatal(err)
	}
	input := Implementation{CommitmentVersion: 1, ProposalID: "proposal", TaskIDs: []string{"task-a", "task-b"}, Revision: strings.Repeat("a", 40)}
	linked, err := s.LinkImplementation(v.ID, "owner", input)
	if err != nil || len(linked.Implementations) != 1 || linked.History[len(linked.History)-1].Kind != "implementation_created" {
		t.Fatalf("linked = %#v, %v", linked, err)
	}
	retry, err := s.LinkImplementation(v.ID, "owner", input)
	if err != nil || len(retry.Implementations) != 1 || retry.Implementations[0].ProposalID != "proposal" {
		t.Fatalf("retry = %#v, %v", retry, err)
	}
	if _, err := s.LinkImplementation(v.ID, "owner", Implementation{CommitmentVersion: 1, ProposalID: "different", TaskIDs: []string{"different"}, Revision: strings.Repeat("b", 40)}); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed retry = %v", err)
	}
	if _, err := s.ReportDelivery(v.ID, "proposal", "reviewer", DeliveryObservation{Kind: "failed_measure", Summary: "forged", ResourceKind: "deployment", ResourceID: "missing"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unverified evidence = %v", err)
	}
	reopened, err := s.ReportDelivery(v.ID, "proposal", "reviewer", DeliveryObservation{Kind: "failed_measure", Summary: "p95 reached 140ms", ResourceKind: "deployment", ResourceID: "production-42", EvidenceVerified: true})
	if err != nil || reopened.Status != "pending" || reopened.Commitments[0].Status != "reopened" || len(reopened.Implementations[0].Observations) != 1 {
		t.Fatalf("reopened = %#v, %v", reopened, err)
	}
}

func TestAlternativesCompareCommonCriteriaAndPreserveSupersededDissent(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 10, 19, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	actor := "11111111111111111111111111111111"
	deadline := now.Add(time.Hour)
	scope := Scope{Question: "Queue model?", Constraints: []string{"No downtime"}, SuccessMeasures: []string{"p95 under 100ms"}, Deadline: &deadline, AffectedResources: []Resource{{Kind: "repository", Label: "API"}}, Participants: []Participant{{UserID: actor}}, OwnerID: actor}
	v, err := s.Create("22222222222222222222222222222222", Source{Kind: "repository"}, scope, actor)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []Evidence{{Kind: "usage", ResourceID: "latency-dashboard", Revision: "window:2026-08-10T18:00Z/19:00Z", Label: "production p95"}}
	alt := Alternative{Title: "Bounded FIFO", Summary: "Bound the queue.", Assumptions: []string{"Traffic remains bursty"}, Tradeoffs: []string{"Reject overload"}, Risks: []string{"Retry storms"}, CompatibilityImpact: "No wire change", Cost: "Two engineer-days", ExpectedOutcomes: []string{"Stable tail latency"}, Evidence: evidence, Criteria: []CriterionAssessment{{Criterion: "p95 under 100ms", Outcome: "Model predicts 82ms", Evidence: evidence}}, EvidenceStatus: EvidenceStatus{MissingKinds: []string{"fabricated"}, MissingCriteria: []string{"fabricated"}}}
	v, err = s.AddAlternative(v.ID, actor, 1, alt)
	if err != nil || len(v.Alternatives) != 1 || v.Version != 2 {
		t.Fatalf("alternative = %#v, %v", v, err)
	}
	if len(v.Alternatives[0].EvidenceStatus.MissingKinds) != 4 || v.Alternatives[0].EvidenceStatus.MissingKinds[0] != "code" {
		t.Fatalf("creation evidence status = %#v", v.Alternatives[0].EvidenceStatus)
	}
	stored, _ := s.read(v.ID)
	if len(stored.Alternatives[0].EvidenceStatus.MissingKinds) != 0 || len(stored.Alternatives[0].EvidenceStatus.MissingCriteria) != 0 {
		t.Fatalf("persisted client projection = %#v", stored.Alternatives[0].EvidenceStatus)
	}
	now = now.Add(31 * 24 * time.Hour)
	v, err = s.Discuss(v.ID, actor, "Compare the evidence age.")
	if err != nil || len(v.Alternatives[0].EvidenceStatus.MissingKinds) != 4 || len(v.Alternatives[0].EvidenceStatus.Stale) != 2 {
		t.Fatalf("discussion evidence status = %#v, %v", v.Alternatives[0].EvidenceStatus, err)
	}
	v, err = s.Update(v.ID, actor, 2, scope, "Reconfirmed the scope")
	if err != nil || len(v.Alternatives[0].EvidenceStatus.MissingKinds) != 4 || len(v.Alternatives[0].EvidenceStatus.Stale) != 2 {
		t.Fatalf("update evidence status = %#v, %v", v.Alternatives[0].EvidenceStatus, err)
	}
	projected, _ := s.Get(v.ID)
	if len(projected.Alternatives[0].EvidenceStatus.MissingKinds) != 4 || projected.Alternatives[0].EvidenceStatus.MissingKinds[0] != "code" {
		t.Fatalf("evidence status = %#v", projected.Alternatives[0].EvidenceStatus)
	}
	bad := alt
	bad.Criteria = nil
	if _, err = s.AddAlternative(v.ID, actor, 3, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing common criterion = %v", err)
	}
	v, err = s.AddFinding(v.ID, actor, Finding{AlternativeID: v.Alternatives[0].ID, Body: "The model excludes retry amplification.", Position: "oppose", Uncertainty: "Retry distribution is sampled, not traced.", Citations: evidence})
	if err != nil {
		t.Fatal(err)
	}
	first := v.Findings[0].ID
	v, err = s.AddFinding(v.ID, actor, Finding{AlternativeID: v.Alternatives[0].ID, Body: "A trace confirms retry amplification.", Position: "oppose", Uncertainty: "One region only.", Citations: evidence, SupersedesID: first})
	if err != nil || !v.Findings[0].Superseded || v.Findings[1].SupersedesID != first {
		t.Fatalf("findings = %#v, %v", v.Findings, err)
	}
}

func TestGovernedCommitmentRetainsApprovalsExceptionsAndReopens(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 10, 20, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	deadline := now.Add(24 * time.Hour)
	v, err := s.Create("repo", Source{Kind: "repository", ResourceID: "repo"}, Scope{Question: "Which queue?", Constraints: []string{"No downtime"}, SuccessMeasures: []string{"Stable latency"}, Deadline: &deadline, AffectedResources: []Resource{{Kind: "repository", RepositoryID: "affected", Label: "consumer"}}, Participants: []Participant{{UserID: "owner"}, {UserID: "approver"}}, OwnerID: "owner"}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	evidence := []Evidence{{Kind: "usage", ResourceID: "latency", Revision: "window", Label: "p95"}}
	alt := func(title string) Alternative {
		return Alternative{Title: title, Summary: title, Assumptions: []string{"bursty"}, Tradeoffs: []string{"reject overload"}, Risks: []string{"retries"}, CompatibilityImpact: "none", Cost: "two days", ExpectedOutcomes: []string{"stable"}, Evidence: evidence, Criteria: []CriterionAssessment{{Criterion: "Stable latency", Outcome: "82ms", Evidence: evidence}}}
	}
	v, _ = s.AddAlternative(v.ID, "owner", v.Version, alt("FIFO"))
	v, _ = s.AddAlternative(v.ID, "owner", v.Version, alt("LIFO"))
	exceptionExpiry := now.Add(7 * 24 * time.Hour)
	v, err = s.RequestApproval(v.ID, "owner", v.Version, ApprovalRequest{Kind: "policy", PolicyID: "policy", PolicyRule: "minimum_reviews", ApproverID: "approver", Reason: "Policy requires accountable review", ExceptionReason: "Migration window", ExceptionExpiresAt: &exceptionExpiry})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(v.ID, "owner", v.Version, Commitment{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid publication = %v", err)
	}
	v, err = s.RespondApproval(v.ID, v.ApprovalRequests[0].ID, "approver", "approve", "bounded exception accepted")
	if err != nil {
		t.Fatal(err)
	}
	selected := v.Alternatives[0]
	v, err = s.AddFinding(v.ID, "owner", Finding{AlternativeID: selected.ID, Body: "Retries remain a concern.", Position: "oppose", Uncertainty: "one region", Citations: evidence})
	if err != nil {
		t.Fatal(err)
	}
	firstDissent := v.Findings[0].ID
	v, err = s.AddFinding(v.ID, "owner", Finding{AlternativeID: selected.ID, Body: "A broader trace confirms retries.", Position: "oppose", Uncertainty: "two regions", Citations: evidence, SupersedesID: firstDissent})
	if err != nil || !v.Findings[0].Superseded {
		t.Fatalf("replacement dissent = %#v, %v", v.Findings, err)
	}
	commitment := Commitment{SelectedAlternativeID: selected.ID, RejectedAlternativeIDs: []string{v.Alternatives[1].ID}, Rationale: "Best observed latency under the retained evidence.", AcceptedTradeoffs: []string{"Reject overload"}, Conditions: []string{"Monitor retries"}, ReviewDate: now.Add(30 * 24 * time.Hour), Evidence: selected.Evidence, DissentFindingIDs: []string{v.Findings[1].ID}, Exceptions: []Exception{{ApprovalRequestID: v.ApprovalRequests[0].ID, PolicyID: "policy", PolicyRule: "minimum_reviews", Reason: "Migration window", ExpiresAt: exceptionExpiry}}}
	omitted := commitment
	omitted.DissentFindingIDs = nil
	if _, publishErr := s.Publish(v.ID, "owner", v.Version, omitted); !errors.Is(publishErr, ErrInvalid) {
		t.Fatalf("omitted dissent accepted: %v", publishErr)
	}
	mismatched := commitment
	mismatched.Exceptions[0].Reason = "Data export"
	if _, publishErr := s.Publish(v.ID, "owner", v.Version, mismatched); !errors.Is(publishErr, ErrInvalid) {
		t.Fatalf("mismatched exception accepted: %v", publishErr)
	}
	commitment.Exceptions[0].Reason = "Migration window"
	duplicate := commitment
	duplicate.Exceptions = append(append([]Exception{}, commitment.Exceptions...), commitment.Exceptions[0])
	if _, publishErr := s.Publish(v.ID, "owner", v.Version, duplicate); !errors.Is(publishErr, ErrInvalid) {
		t.Fatalf("duplicate exception accepted: %v", publishErr)
	}
	v, err = s.Publish(v.ID, "owner", v.Version, commitment)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "published" || len(v.Commitments) != 1 || v.Commitments[0].Approvals[0].Status != "approved" || len(v.Commitments[0].Exceptions) != 1 {
		t.Fatalf("published = %#v", v)
	}
	v, err = s.Discuss(v.ID, "owner", "Non-material clarification")
	if err != nil || v.Status != "published" {
		t.Fatalf("discussion reopened = %s, %v", v.Status, err)
	}
	v, err = s.AddFinding(v.ID, "owner", Finding{AlternativeID: selected.ID, Body: "A new trace exposes more retries.", Position: "oppose", Uncertainty: "one region", Citations: evidence})
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "pending" || v.Commitments[0].Status != "reopened" || v.Commitments[0].ReopenedAt == nil || v.ApprovalRequests[0].Status != "superseded" {
		t.Fatalf("reopened = %#v", v)
	}
}
