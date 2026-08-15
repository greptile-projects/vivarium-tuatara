package serviceobjectives

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReliabilityEvidenceRetainsExactMappingHistoryAndDerivesAttainment(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, 8, 14, 20, 0, 0, 0, time.UTC) }
	contract, err := s.Create("repo", "owner", completeRevision())
	if err != nil {
		t.Fatal(err)
	}
	mapping := SignalMappingRevision{ContractVersion: 1, ObjectiveID: "availability", InstrumentationRevision: "otel-v1", Calculation: "ratio", Unit: "percent", Rationale: "Connect release telemetry", Sources: []SignalSource{
		{Kind: "metric", Name: "Successful requests", Reference: "metrics://requests/success", Visibility: "public", Sanitization: "aggregate counts only"},
		{Kind: "log", Name: "Failure classes", Reference: "logs://service/failures", Visibility: "participants", Sanitization: "messages and user fields removed"},
		{Kind: "trace", Name: "Critical journey", Reference: "traces://journey/checkout", Visibility: "public", Sanitization: "span names and duration only"},
	}}
	contract, err = s.PublishMapping(contract.ID, "owner", mapping)
	if err != nil {
		t.Fatal(err)
	}
	mappingID := contract.SignalMappings[0].ID
	observation := Observation{MappingID: mappingID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: s.now().Add(-time.Hour), WindowEnd: s.now(), GoodEvents: 995, TotalEvents: 1000, Uncertainty: .2, Summary: "Release window aggregate", Gaps: []EvidenceGap{{Kind: "support_delay", Detail: "Support reports can arrive late."}}, Software: []SoftwareReference{{Kind: "release", ID: "release-1", Revision: "abc123", Label: "v1.2.0"}, {Kind: "deployment", ID: "deploy-1", Revision: "abc123", Label: "production"}, {Kind: "commit", ID: "abc123", Revision: "abc123", Label: "checkout fix"}, {Kind: "pull_request", ID: "42", Revision: "abc123", Label: "Reduce retries"}, {Kind: "package", ID: "web", Revision: "1.2.0", Label: "web package"}, {Kind: "dependent_service", ID: "payments", Revision: "api-v3", Label: "Payments"}}}
	contract, err = s.RecordObservation(contract.ID, "owner", observation)
	if err != nil {
		t.Fatal(err)
	}
	got := contract.Observations[0]
	if got.Attainment == nil || *got.Attainment != 99.5 || got.TargetMet == nil || *got.TargetMet || got.ErrorBudgetConsumed == nil || *got.ErrorBudgetConsumed != 500 {
		t.Fatalf("derived observation = %#v", got)
	}
	mapping.InstrumentationRevision = "otel-v2"
	mapping.Rationale = "Replace counter cardinality"
	contract, err = s.ReviseMapping(contract.ID, mappingID, 1, "owner", mapping)
	if err != nil {
		t.Fatal(err)
	}
	observation.MappingVersion = 2
	observation.WindowStart = observation.WindowEnd
	observation.WindowEnd = observation.WindowEnd.Add(time.Hour)
	contract, err = s.RecordObservation(contract.ID, "owner", observation)
	if err != nil {
		t.Fatal(err)
	}
	if contract.Observations[1].ComparableToPrevious || contract.Observations[1].ComparisonReason == "" {
		t.Fatalf("instrumentation change was silently compared: %#v", contract.Observations[1])
	}
	public := s.ProjectForReader(contract, false)
	if public.SignalMappings[0].Revisions[0].Sources[1].Reference != "restricted" {
		t.Fatalf("restricted source leaked: %#v", public.SignalMappings[0])
	}
	if len(public.Observations[0].Software) != 6 || public.Observations[0].Gaps[0].Kind != "support_delay" {
		t.Fatalf("public provenance missing: %#v", public.Observations[0])
	}
}

func TestReliabilityDeliveryPolicyPreservesHumanAuthorityAndExceptions(t *testing.T) {
	s, _ := New(t.TempDir())
	contract, err := s.Create("repo", "owner", completeRevision())
	if err != nil {
		t.Fatal(err)
	}
	policy := DeliveryPolicy{ContractVersion: 1, ObjectiveIDs: []string{"availability"}, Branches: []string{"main"}, Services: []string{"checkout"}, EnvironmentIDs: []string{"production"}, JourneyIDs: []string{"buy"}, RiskClasses: []string{"availability"}, MaximumBudgetConsumed: 90, MaximumPredictedIncrease: 10, RequireCurrentEvidence: true, RequireDependencies: true, RequiredOwnerIDs: []string{"owner"}, MinimumAcknowledgements: 1, OnMissingEvidence: "block", OnBudgetExhausted: "pause", OnRegression: "slow", OnDependencyFailure: "rollback", Rationale: "Protect current purchasers."}
	contract, err = s.PublishDeliveryPolicy(contract.ID, "owner", 1, policy)
	if err != nil {
		t.Fatal(err)
	}
	policy = contract.DeliveryPolicies[0]
	duplicatePolicy := policy
	duplicatePolicy.ObjectiveIDs = []string{"availability", "availability"}
	if _, err = s.PublishDeliveryPolicy(contract.ID, "owner", 1, duplicatePolicy); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate policy objectives = %v", err)
	}
	missing, err := s.EvaluateReliability("repo", "pull_request", "pull", strings.Repeat("a", 40), "main", "checkout", "production", []string{"buy"}, []string{"availability"})
	if err != nil || len(missing) != 1 || missing[0].Effect != "block" {
		t.Fatalf("missing = %#v, %v", missing, err)
	}
	consumed := 100.0
	forged := ReliabilityImpact{PolicyID: policy.ID, PolicyVersion: 1, Kind: "pull_request", ResourceID: "pull", Revision: strings.Repeat("a", 40), Branch: "main", Service: "checkout", EnvironmentID: "production", JourneyIDs: []string{"buy"}, RiskClasses: []string{"availability"}, ObjectiveImpacts: []ObjectiveImpact{{ObjectiveID: "availability", ObservationID: "forged-window", ObservedBudgetConsumed: &consumed, Confidence: "high"}}, Summary: "Caller-selected evidence."}
	if _, err = s.RecordReliabilityImpact(contract.ID, "maintainer", forged); !errors.Is(err, ErrInvalid) {
		t.Fatalf("forged observation = %v", err)
	}
	contract, observationID := deliveryObservation(t, s, contract, "availability", "pull", strings.Repeat("a", 40), 800, 1000)
	impact := ReliabilityImpact{PolicyID: policy.ID, PolicyVersion: 1, Kind: "pull_request", ResourceID: "pull", Revision: strings.Repeat("a", 40), Branch: "main", Service: "checkout", EnvironmentID: "production", JourneyIDs: []string{"buy"}, RiskClasses: []string{"availability"}, ObjectiveImpacts: []ObjectiveImpact{{ObjectiveID: "availability", ObservationID: observationID, PredictedBudgetIncrease: 12, ObservedBudgetConsumed: &consumed, Confidence: "high"}}, DependencyFailures: []string{"payments objective failed"}, Summary: "Canary predicts and observes lost reliability."}
	contract, err = s.RecordReliabilityImpact(contract.ID, "maintainer", impact)
	if err != nil {
		t.Fatal(err)
	}
	impact = contract.ReliabilityImpacts[0]
	if impact.ObjectiveImpacts[0].ObservedBudgetConsumed == nil || *impact.ObjectiveImpacts[0].ObservedBudgetConsumed == consumed {
		t.Fatalf("caller budget was not replaced by trusted observation: %#v", impact.ObjectiveImpacts[0])
	}
	blocked, _ := s.EvaluateReliability("repo", "pull_request", "pull", impact.Revision, "main", "checkout", "production", []string{"buy"}, []string{"availability"})
	if len(blocked) != 1 || blocked[0].Effect != "rollback" || blocked[0].State != "blocked" || blocked[0].AuthorityNote == "" {
		t.Fatalf("blocked = %#v", blocked)
	}
	if _, err = s.AcknowledgeReliabilityImpact(contract.ID, impact.ID, "maintainer", "not an owner"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-owner acknowledgement = %v", err)
	}
	contract, err = s.AcknowledgeReliabilityImpact(contract.ID, impact.ID, "owner", "Current user impact warrants a hold.")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = s.ExceptReliabilityImpact(contract.ID, impact.ID, "owner", DeliveryException{Reason: "Bounded emergency repair", ExpiresAt: time.Now().UTC().Add(time.Hour), FollowUp: "review tomorrow"})
	if err != nil {
		t.Fatal(err)
	}
	excepted, _ := s.EvaluateReliability("repo", "pull_request", "pull", impact.Revision, "main", "checkout", "production", []string{"buy"}, []string{"availability"})
	if excepted[0].State != "excepted" || excepted[0].ActiveException == nil || excepted[0].Effect != "none" {
		t.Fatalf("excepted = %#v", excepted)
	}
}

func TestImprovementRetainsHarmAndDerivesGovernedBudgetRestoration(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 15, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	c, err := s.Create("repo", "owner", completeRevision())
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.PublishMapping(c.ID, "owner", SignalMappingRevision{ContractVersion: 1, ObjectiveID: "availability", InstrumentationRevision: "otel-v1", Sources: []SignalSource{{Kind: "metric", Name: "success", Reference: "metrics://success", Visibility: "public", Sanitization: "aggregates only"}}, Calculation: "ratio", Unit: "percent", Rationale: "release comparison"})
	if err != nil {
		t.Fatal(err)
	}
	mapping := c.SignalMappings[0]
	c, err = s.RecordObservation(c.ID, "owner", Observation{MappingID: mapping.ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: now.Add(-2 * time.Hour), WindowEnd: now.Add(-time.Hour), GoodEvents: 990, TotalEvents: 1000, Uncertainty: 1, Summary: "affected users", Software: []SoftwareReference{{Kind: "deployment", ID: "bad", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Label: "degraded rollout"}}})
	if err != nil {
		t.Fatal(err)
	}
	harm := c.Observations[0]
	policy := DeliveryPolicy{ContractVersion: 1, ObjectiveIDs: []string{"availability"}, RequiredOwnerIDs: []string{"owner"}, MaximumBudgetConsumed: 90, MaximumPredictedIncrease: 10, OnMissingEvidence: "block", OnBudgetExhausted: "pause", OnRegression: "slow", OnDependencyFailure: "rollback", Rationale: "protect users"}
	c, err = s.PublishDeliveryPolicy(c.ID, "owner", 1, policy)
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.RecordReliabilityImpact(c.ID, "owner", ReliabilityImpact{PolicyID: c.DeliveryPolicies[0].ID, PolicyVersion: 1, Kind: "deployment", ResourceID: "bad", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Summary: "budget depleted", ObjectiveImpacts: []ObjectiveImpact{{ObjectiveID: "availability", ObservationID: harm.ID, PredictedBudgetIncrease: 10, Confidence: "high"}}})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.RecordObservation(c.ID, "owner", Observation{MappingID: mapping.ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: now.Add(-time.Hour), WindowEnd: now, GoodEvents: 995, TotalEvents: 1000, Uncertainty: 1, Summary: "unrelated baseline", Software: []SoftwareReference{{Kind: "deployment", ID: "unrelated", Revision: "ffffffffffffffffffffffffffffffffffffffff", Label: "unrelated"}}})
	if err != nil {
		t.Fatal(err)
	}
	unrelatedBaseline := c.Observations[1]
	template := Improvement{ContractVersion: 1, ObjectiveID: "availability", ImpactID: c.ReliabilityImpacts[0].ID, BaselineObservationIDs: []string{harm.ID}, AffectedObservationIDs: []string{harm.ID}, AffectedRevisions: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, DependencyContext: []string{"payments owner"}, EvidenceIDs: []string{harm.ID}, AcceptanceCriteria: []string{"restore target attainment"}, ProposalID: "pending", TaskIDs: []string{"pending"}}
	wrongBaseline := template
	wrongBaseline.BaselineObservationIDs = []string{unrelatedBaseline.ID}
	if err = s.ValidateImprovement(c.ID, wrongBaseline); !errors.Is(err, ErrInvalid) {
		t.Fatalf("impact-unbound baseline = %v", err)
	}
	c, reservation, err := s.ReserveImprovement(c.ID, "owner", template)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.Status != "pending" || reservation.ProposalID != "" {
		t.Fatalf("reservation = %#v", reservation)
	}
	c, err = s.CompleteImprovement(c.ID, reservation.ID, "owner", "proposal", []string{"task"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * time.Minute)
	c, err = s.RecordObservation(c.ID, "owner", Observation{MappingID: mapping.ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: now.Add(-30 * time.Minute), WindowEnd: now, GoodEvents: 980, TotalEvents: 1000, Uncertainty: 1, Summary: "unlinked comparison", Software: []SoftwareReference{{Kind: "deployment", ID: "other", Revision: "cccccccccccccccccccccccccccccccccccccccc", Label: "other rollout"}}})
	if err != nil {
		t.Fatal(err)
	}
	unlinked := c.Observations[2]
	now = now.Add(30 * time.Minute)
	c, err = s.RecordObservation(c.ID, "owner", Observation{MappingID: mapping.ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: now.Add(-time.Hour), WindowEnd: now, GoodEvents: 1000, TotalEvents: 1000, Uncertainty: 1, Summary: "recovered users", Software: []SoftwareReference{{Kind: "release", ID: "release", Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Label: "repair"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.VerifyImprovement(c.ID, "owner", RolloutVerification{ImprovementID: c.Improvements[0].ID, Kind: "release", ResourceID: "release", Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", BaselineObservationID: unlinked.ID, CurrentObservationID: c.Observations[3].ID, Decision: "restore_budget", Rationale: "unlinked comparison"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unlinked baseline = %v", err)
	}
	c, err = s.VerifyImprovement(c.ID, "owner", RolloutVerification{ImprovementID: c.Improvements[0].ID, Kind: "release", ResourceID: "release", Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", BaselineObservationID: harm.ID, CurrentObservationID: c.Observations[3].ID, Decision: "restore_budget", Rationale: "target recovered in governed rollout"})
	if err != nil {
		t.Fatal(err)
	}
	got := c.RolloutVerifications[0]
	if !got.Improved || !got.BudgetRestored || got.BudgetBefore == nil || got.BudgetAfter == nil || *got.BudgetAfter >= *got.BudgetBefore || c.Observations[0].Summary != "affected users" {
		t.Fatalf("verification/history = %#v %#v", got, c.Observations[0])
	}
	if _, err = s.VerifyImprovement(c.ID, "owner", RolloutVerification{ImprovementID: c.Improvements[0].ID, Kind: "release", ResourceID: "release", Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", BaselineObservationID: harm.ID, CurrentObservationID: harm.ID, Decision: "restore_budget", Rationale: "forged recovery"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("forged restoration = %v", err)
	}
}

func TestPredictedOnlyImpactCannotAuthorizeImprovement(t *testing.T) {
	s, _ := New(t.TempDir())
	c, err := s.Create("repo", "owner", completeRevision())
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.PublishMapping(c.ID, "owner", SignalMappingRevision{ContractVersion: 1, ObjectiveID: "availability", InstrumentationRevision: "otel", Sources: []SignalSource{{Kind: "metric", Name: "success", Reference: "metrics://success", Visibility: "public", Sanitization: "aggregate"}}, Calculation: "ratio", Unit: "percent", Rationale: "trusted"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	c, err = s.RecordObservation(c.ID, "owner", Observation{MappingID: c.SignalMappings[0].ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: now.Add(-time.Hour), WindowEnd: now, GoodEvents: 1000, TotalEvents: 1000, Uncertainty: 1, Summary: "healthy", Software: []SoftwareReference{{Kind: "deployment", ID: "healthy", Revision: "dddddddddddddddddddddddddddddddddddddddd", Label: "healthy"}}})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.PublishDeliveryPolicy(c.ID, "owner", 1, DeliveryPolicy{ContractVersion: 1, ObjectiveIDs: []string{"availability"}, RequiredOwnerIDs: []string{"owner"}, MaximumBudgetConsumed: 90, MaximumPredictedIncrease: 10, OnMissingEvidence: "block", OnBudgetExhausted: "pause", OnRegression: "slow", OnDependencyFailure: "rollback", Rationale: "protect users"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.RecordReliabilityImpact(c.ID, "owner", ReliabilityImpact{PolicyID: c.DeliveryPolicies[0].ID, PolicyVersion: 1, Kind: "pull_request", ResourceID: "future", Revision: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Summary: "prediction", ObjectiveImpacts: []ObjectiveImpact{{ObjectiveID: "availability", PredictedBudgetIncrease: 20, Confidence: "medium"}}})
	if err != nil {
		t.Fatal(err)
	}
	in := Improvement{ContractVersion: 1, ObjectiveID: "availability", ImpactID: c.ReliabilityImpacts[0].ID, BaselineObservationIDs: []string{c.Observations[0].ID}, AffectedObservationIDs: []string{c.Observations[0].ID}, AffectedRevisions: []string{"dddddddddddddddddddddddddddddddddddddddd"}, EvidenceIDs: []string{c.Observations[0].ID}, AcceptanceCriteria: []string{"retain target"}, ProposalID: "pending", TaskIDs: []string{"pending"}}
	if err = s.ValidateImprovement(c.ID, in); !errors.Is(err, ErrInvalid) {
		t.Fatalf("predicted-only authorization = %v", err)
	}
}

func TestReliabilityDeliveryIsolatesScopesAndRequiresCompleteObjectives(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := completeRevision()
	revision.Objectives = append(revision.Objectives, Objective{ID: "latency", Name: "Checkout latency", IndicatorID: "success", WindowID: "month", Target: 95, Comparator: "at_least", JourneyIDs: []string{"buy"}, OwnerIDs: []string{"owner"}})
	revision.ErrorBudgets = append(revision.ErrorBudgets, ErrorBudget{ObjectiveID: "latency", AllowedFailure: 5, Unit: "percent", BurnPolicy: "Slow rollout"})
	contract, err := s.Create("repo", "owner", revision)
	if err != nil {
		t.Fatal(err)
	}
	policy := DeliveryPolicy{ContractVersion: 1, ObjectiveIDs: []string{"availability", "latency"}, Branches: []string{"main"}, Services: []string{"checkout"}, EnvironmentIDs: []string{"production"}, JourneyIDs: []string{"buy"}, RiskClasses: []string{"availability"}, MaximumBudgetConsumed: 90, MaximumPredictedIncrease: 10, RequireCurrentEvidence: true, RequiredOwnerIDs: []string{"owner"}, OnMissingEvidence: "block", OnBudgetExhausted: "pause", OnRegression: "block", OnDependencyFailure: "pause", Rationale: "Protect checkout."}
	contract, err = s.PublishDeliveryPolicy(contract.ID, "owner", 1, policy)
	if err != nil {
		t.Fatal(err)
	}
	policy = contract.DeliveryPolicies[0]
	partial := ReliabilityImpact{PolicyID: policy.ID, PolicyVersion: 1, Kind: "pull_request", ResourceID: "pull", Revision: strings.Repeat("b", 40), Branch: "main", Service: "checkout", EnvironmentID: "production", JourneyIDs: []string{"buy"}, RiskClasses: []string{"availability"}, ObjectiveImpacts: []ObjectiveImpact{{ObjectiveID: "availability", ObservationID: "one", Confidence: "high"}}, Summary: "Partial evidence."}
	if _, err = s.RecordReliabilityImpact(contract.ID, "owner", partial); !errors.Is(err, ErrInvalid) {
		t.Fatalf("partial objective coverage = %v", err)
	}
	partial.ObjectiveImpacts = append(partial.ObjectiveImpacts, partial.ObjectiveImpacts[0])
	if _, err = s.RecordReliabilityImpact(contract.ID, "owner", partial); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate objective coverage = %v", err)
	}
	contract, availabilityObservation := deliveryObservation(t, s, contract, "availability", "pull", strings.Repeat("b", 40), 980, 1000)
	contract, latencyObservation := deliveryObservation(t, s, contract, "latency", "pull", strings.Repeat("b", 40), 990, 1000)
	target := partial
	target.ObjectiveImpacts = []ObjectiveImpact{{ObjectiveID: "availability", ObservationID: availabilityObservation, PredictedBudgetIncrease: 20, Confidence: "high"}, {ObjectiveID: "latency", ObservationID: latencyObservation, Confidence: "high"}}
	contract, err = s.RecordReliabilityImpact(contract.ID, "owner", target)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Now().UTC().Add(time.Minute) }
	foreign := target
	foreign.Service, foreign.EnvironmentID, foreign.JourneyIDs, foreign.RiskClasses = "billing", "staging", []string{"refund"}, []string{"latency"}
	foreign.ObjectiveImpacts[0].PredictedBudgetIncrease = 0
	contract, err = s.RecordReliabilityImpact(contract.ID, "owner", foreign)
	if err != nil {
		t.Fatal(err)
	}
	evaluations, _ := s.EvaluateReliability("repo", "pull_request", "pull", target.Revision, "main", "checkout", "production", []string{"buy"}, []string{"availability"})
	if len(evaluations) != 1 || evaluations[0].Effect != "pause" {
		t.Fatalf("foreign evidence replaced target: %#v", evaluations)
	}
	duplicateRisks, _ := s.EvaluateReliability("repo", "pull_request", "pull", target.Revision, "main", "checkout", "production", []string{"buy"}, []string{"availability", "availability"})
	if len(duplicateRisks) != 1 || duplicateRisks[0].ImpactID != evaluations[0].ImpactID || duplicateRisks[0].Effect != "pause" {
		t.Fatalf("duplicate risks hid matching impact: %#v", duplicateRisks)
	}
	omitted, _ := s.EvaluateReliability("repo", "pull_request", "pull", target.Revision, "main", "", "", nil, []string{"availability"})
	if len(omitted) != 0 {
		t.Fatalf("omitted restricted scope matched: %#v", omitted)
	}
}

func deliveryObservation(t *testing.T, s *Store, contract Contract, objectiveID, resourceID, revision string, good, total float64) (Contract, string) {
	t.Helper()
	contract, err := s.PublishMapping(contract.ID, "owner", SignalMappingRevision{ContractVersion: 1, ObjectiveID: objectiveID, InstrumentationRevision: "delivery-" + objectiveID, Calculation: "ratio", Unit: "percent", Rationale: "Ground delivery readiness", Sources: []SignalSource{{Kind: "metric", Name: objectiveID, Reference: "metrics://" + objectiveID, Visibility: "participants", Sanitization: "aggregate counts"}}})
	if err != nil {
		t.Fatal(err)
	}
	mapping := contract.SignalMappings[len(contract.SignalMappings)-1]
	now := s.now()
	contract, err = s.RecordObservation(contract.ID, "owner", Observation{MappingID: mapping.ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: objectiveID, WindowStart: now.Add(-time.Hour), WindowEnd: now, GoodEvents: good, TotalEvents: total, Summary: "Trusted delivery window", Uncertainty: 1, Software: []SoftwareReference{{Kind: "pull_request", ID: resourceID, Revision: revision, Label: "candidate"}}})
	if err != nil {
		t.Fatal(err)
	}
	return contract, contract.Observations[len(contract.Observations)-1].ID
}

func TestReliabilityEvidenceRejectsCredentials(t *testing.T) {
	s, _ := New(t.TempDir())
	contract, _ := s.Create("repo", "owner", completeRevision())
	for _, reference := range []string{"https://metrics/?api_key=leak", "https://metrics/?access_token=leak", "https://metrics/?access_token%3dleak", "https://metrics/?access_token%3dleak%zz", "https://metrics/?access%5ftoken%253dleak", "https://metrics/?token=leak", "Authorization: Basic leak", "Cookie: session=leak", "X-API-Key: leak"} {
		_, err := s.PublishMapping(contract.ID, "owner", SignalMappingRevision{ContractVersion: 1, ObjectiveID: "availability", InstrumentationRevision: "v1", Calculation: "ratio", Unit: "percent", Rationale: "connect", Sources: []SignalSource{{Kind: "metric", Name: "requests", Reference: reference, Visibility: "public", Sanitization: "aggregate"}}})
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("credential-bearing reference %q error = %v", reference, err)
		}
	}
	contract.SignalMappings = []SignalMapping{{ID: "legacy", CurrentVersion: 1, Revisions: []SignalMappingRevision{{Version: 1, Sources: []SignalSource{{Kind: "metric", Name: "requests", Reference: "https://metrics/?access_token%3dlegacy%zz", Visibility: "public", Sanitization: "aggregate"}}}}}}
	if got := s.ProjectForReader(contract, false).SignalMappings[0].Revisions[0].Sources[0].Reference; got != "redacted_unsafe_reference" {
		t.Fatalf("legacy credential projection = %q", got)
	}
}

func TestReliabilityInvestigationRetainsEvidenceDisputeOwnerInputAndStaleness(t *testing.T) {
	s, _ := New(t.TempDir())
	s.ConfigureInvestigationProvenance(func(_ Contract, in Investigation) bool {
		return in.Trigger.ID == "42" && in.Trigger.Revision == "abc123"
	})
	c, _ := s.Create("repo", "owner", completeRevision())
	c, _ = s.PublishMapping(c.ID, "owner", SignalMappingRevision{ContractVersion: 1, ObjectiveID: "availability", InstrumentationRevision: "v1", Calculation: "ratio", Unit: "percent", Rationale: "aggregate", Sources: []SignalSource{{Kind: "metric", Name: "success", Reference: "metrics://success", Visibility: "public", Sanitization: "aggregate"}}})
	now := time.Now().UTC()
	c, _ = s.RecordObservation(c.ID, "owner", Observation{MappingID: c.SignalMappings[0].ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: now.Add(-2 * time.Hour), WindowEnd: now.Add(-time.Hour), GoodEvents: 999, TotalEvents: 1000, Summary: "baseline", Uncertainty: 1})
	c, _ = s.RecordObservation(c.ID, "owner", Observation{MappingID: c.SignalMappings[0].ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: now.Add(-time.Hour), WindowEnd: now, GoodEvents: 990, TotalEvents: 1000, Summary: "affected", Uncertainty: 3, Software: []SoftwareReference{{Kind: "pull_request", ID: "42", Revision: "abc123", Label: "retry change"}}})
	c, err := s.OpenInvestigation(c.ID, "agent:reader", Investigation{ContractVersion: 1, ObjectiveID: "availability", Title: "Checkout may be degrading", Trigger: InvestigationTrigger{Kind: "pull_request", ID: "42", Revision: "abc123"}, BaselineObservationIDs: []string{c.Observations[0].ID}, AffectedObservationIDs: []string{c.Observations[1].ID}, JourneyIDs: []string{"buy"}, Evidence: []InvestigationEvidence{{Kind: "code", ResourceID: "diff-42", Revision: "abc123", Label: "retry change", Visibility: "public"}, {Kind: "trace", ResourceID: "trace-window", Revision: "window-2", Label: "sanitized critical path", Visibility: "participants"}}})
	if err != nil {
		t.Fatal(err)
	}
	x := c.Investigations[0]
	c, err = s.MutateInvestigation(c.ID, x.ID, "agent:reader", "agent", "finding", x.Version, InvestigationFinding{Kind: "hypothesis", Statement: "Retries correlate with lower completion.", Uncertainty: "Dependency saturation was not observed.", Confidence: "medium", CitationIDs: []string{c.Observations[1].ID, "diff-42"}}, InvestigationResponse{}, InputRequest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	x = c.Investigations[0]
	if _, err = s.MutateInvestigation(c.ID, x.ID, "agent:reader", "agent", "outcome", x.Version, InvestigationFinding{}, InvestigationResponse{}, InputRequest{}, &InvestigationOutcome{Kind: "incident", ResourceID: "incident-1", Summary: "contain"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("read-only agent outcome error = %v", err)
	}
	c, err = s.MutateInvestigation(c.ID, x.ID, "owner", "human", "response", x.Version, InvestigationFinding{}, InvestigationResponse{FindingID: x.Findings[0].ID, Kind: "dispute", Body: "The deployment preceded this pull."}, InputRequest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	x = c.Investigations[0]
	c, err = s.MutateInvestigation(c.ID, x.ID, "owner", "human", "request", x.Version, InvestigationFinding{}, InvestigationResponse{}, InputRequest{OwnerID: "owner", DependencyID: "payments", Question: "Did payment latency change?"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	x = c.Investigations[0]
	c, err = s.MutateInvestigation(c.ID, x.ID, "owner", "human", "reply", x.Version, InvestigationFinding{}, InvestigationResponse{}, InputRequest{ID: x.InputRequests[0].ID, Response: "No regional latency change."}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := completeRevision()
	r.Summary = "successor"
	c, err = s.Revise(c.ID, 1, "owner", r)
	if err != nil {
		t.Fatal(err)
	}
	x = c.Investigations[0]
	if !x.Inconclusive || len(x.StaleEvidenceIDs) == 0 || x.Responses[0].Kind != "dispute" || x.InputRequests[0].Status != "answered" {
		t.Fatalf("investigation projection=%#v", x)
	}
	public := s.ProjectForReader(c, false)
	if len(public.Investigations) != 0 {
		t.Fatalf("investigation leaked to anonymous reader: %#v", public.Investigations)
	}
}

func TestReliabilityInvestigationFailsClosedWithoutProvenanceResolver(t *testing.T) {
	s, _ := New(t.TempDir())
	c, _ := s.Create("repo", "owner", completeRevision())
	_, err := s.OpenInvestigation(c.ID, "owner", Investigation{ContractVersion: 1, ObjectiveID: "availability", Title: "Unverified", Trigger: InvestigationTrigger{Kind: "pull_request", ID: "invented", Revision: "invented-revision"}, JourneyIDs: []string{"buy"}, Evidence: []InvestigationEvidence{{Kind: "pull_request", ResourceID: "invented", Revision: "invented-revision", Label: "unverified", Visibility: "public"}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("unconfigured provenance error = %v", err)
	}
}

func TestReliabilityInvestigationTriggerCannotCrossObjective(t *testing.T) {
	r := completeRevision()
	r.Objectives = append(r.Objectives, Objective{ID: "latency", Name: "Latency", IndicatorID: "success", WindowID: "month", Target: 99, Comparator: "at_least", JourneyIDs: []string{"buy"}, OwnerIDs: []string{"owner"}})
	r.ErrorBudgets = append(r.ErrorBudgets, ErrorBudget{ObjectiveID: "latency", AllowedFailure: 1, Unit: "percent", BurnPolicy: "Investigate"})
	v := Contract{CurrentVersion: 1, Revisions: []Revision{r}, Observations: []Observation{{ID: "latency-window", ContractVersion: 1, ObjectiveID: "latency", Software: []SoftwareReference{{Kind: "deployment", ID: "deploy-latency", Revision: "abc123", Label: "latency only"}}}}}
	x := Investigation{ContractVersion: 1, ObjectiveID: "availability", Title: "Availability", Trigger: InvestigationTrigger{Kind: "deployment", ID: "deploy-latency", Revision: "abc123"}, JourneyIDs: []string{"buy"}, Evidence: []InvestigationEvidence{{Kind: "deployment", ResourceID: "deploy-latency", Revision: "abc123", Label: "deployment", Visibility: "participants"}}}
	if validInvestigation(v, x) {
		t.Fatal("cross-objective deployment resolved the availability trigger")
	}
}

func TestNativeUnitObservationUsesDirectionalBudget(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := completeRevision()
	revision.Indicators[0].Calculation, revision.Indicators[0].Unit = "count", "requests"
	revision.Objectives[0].Target, revision.Objectives[0].Comparator = 10, "at_most"
	revision.ErrorBudgets[0].AllowedFailure, revision.ErrorBudgets[0].Unit = 2, "requests"
	contract, err := s.Create("repo", "owner", revision)
	if err != nil {
		t.Fatal(err)
	}
	contract, err = s.PublishMapping(contract.ID, "owner", SignalMappingRevision{ContractVersion: 1, ObjectiveID: "availability", InstrumentationRevision: "counter-v1", Calculation: "count", Unit: "requests", Rationale: "Count failed requests", Sources: []SignalSource{{Kind: "metric", Name: "Failed requests", Reference: "metrics://failed", Visibility: "public", Sanitization: "aggregate count"}}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	contract, err = s.RecordObservation(contract.ID, "owner", Observation{MappingID: contract.SignalMappings[0].ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: now.Add(-time.Hour), WindowEnd: now, GoodEvents: 15, TotalEvents: 1000, Summary: "Failure count", Uncertainty: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := contract.Observations[0]
	if got.Attainment == nil || *got.Attainment != 15 || got.TargetMet == nil || *got.TargetMet || got.ErrorBudgetConsumed == nil || *got.ErrorBudgetConsumed != 250 {
		t.Fatalf("native-unit projection = %#v", got)
	}
}

func completeRevision() Revision {
	return Revision{Title: "Checkout reliability", Summary: "People can complete checkout reliably.", Scopes: []Scope{{Kind: "environment", ResourceID: "production", Name: "Production checkout"}}, Indicators: []Indicator{{ID: "success", Name: "Successful checkout", Description: "Eligible checkouts that complete", Signal: "checkout.completed", Calculation: "ratio", Unit: "percent", GoodEvent: "completed", TotalEvent: "started"}}, Windows: []Window{{ID: "month", Name: "Rolling month", Duration: "720h", Rolling: true}}, Journeys: []Journey{{ID: "buy", Name: "Buy", Description: "Complete payment", OwnerIDs: []string{"owner"}}}, Objectives: []Objective{{ID: "availability", Name: "Checkout availability", IndicatorID: "success", WindowID: "month", Target: 99.9, Comparator: "at_least", JourneyIDs: []string{"buy"}, OwnerIDs: []string{"owner"}}}, Dependencies: []Dependency{{ID: "payments", Name: "Payments", Kind: "service", OwnerIDs: []string{"owner"}, ObjectiveIDs: []string{"availability"}}}, ErrorBudgets: []ErrorBudget{{ObjectiveID: "availability", AllowedFailure: .1, Unit: "percent", BurnPolicy: "Pause rollout"}}, Severities: []Severity{{Level: "warning", BudgetConsumedPercent: 50, Response: "Investigate", OwnerIDs: []string{"owner"}}, {Level: "critical", BudgetConsumedPercent: 100, Response: "Contain", OwnerIDs: []string{"owner"}}}, OwnerIDs: []string{"owner"}, CommitmentLinks: []CommitmentLink{{Kind: "performance", ID: "perf", Version: 2}, {Kind: "privacy", ID: "data", Version: 1}}, ExceptionPolicy: ExceptionPolicy{MaximumDuration: "168h", ApprovalOwnerIDs: []string{"owner"}, FollowUpRequired: true}, Rationale: "Initial shared contract"}
}

func TestVersioningAndExplicitDiagnostics(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	created, err := s.Create("repo", "owner", completeRevision())
	if err != nil || created.CurrentVersion != 1 || len(created.Diagnostics) != 0 {
		t.Fatalf("create = %#v, %v", created, err)
	}
	r := completeRevision()
	r.Indicators[0].Signal = ""
	r.Indicators[0].Calculation = "histogram_magic"
	r.Objectives[0].OwnerIDs = nil
	r.Objectives = append(r.Objectives, Objective{ID: "conflict", Name: "Conflicting target", IndicatorID: "success", WindowID: "month", Target: 99, Comparator: "at_least", JourneyIDs: []string{"buy"}, OwnerIDs: []string{"owner"}})
	r.ErrorBudgets = append(r.ErrorBudgets, ErrorBudget{ObjectiveID: "conflict", AllowedFailure: 1, Unit: "percent", BurnPolicy: "Investigate"})
	r.Exceptions = []Exception{{ID: "temporary", ObjectiveIDs: []string{"availability"}, Reason: "Migration", ApprovedBy: "owner", ExpiresAt: now.Add(48 * time.Hour), FollowUp: "proposal/1"}}
	updated, err := s.Revise(created.ID, 1, "owner", r)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"missing_signal": false, "unsupported_calculation": false, "missing_ownership": false, "conflicting_target": false, "expiring_exception": false}
	for _, d := range updated.Diagnostics {
		want[d.Kind] = true
	}
	for kind, found := range want {
		if !found {
			t.Errorf("missing %s: %#v", kind, updated.Diagnostics)
		}
	}
	if _, err = s.Revise(created.ID, 1, "owner", completeRevision()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale error = %v", err)
	}
	got, _ := s.Get(created.ID)
	if len(got.Revisions) != 2 {
		t.Fatalf("history = %#v", got.Revisions)
	}
}

func TestRejectsBrokenReferences(t *testing.T) {
	s, _ := New(t.TempDir())
	cases := []func(*Revision){func(r *Revision) { r.Objectives[0].IndicatorID = "missing" }, func(r *Revision) { r.ErrorBudgets = nil }, func(r *Revision) { r.Severities[1].BudgetConsumedPercent = 25 }, func(r *Revision) { r.CommitmentLinks[0].Kind = "operations" }, func(r *Revision) { r.Scopes[0].ResourceID = "" }, func(r *Revision) { r.Windows[0].Duration = "0s" }, func(r *Revision) { r.Windows[0].Duration = "-1s" }, func(r *Revision) { r.ExceptionPolicy.MaximumDuration = "0s" }, func(r *Revision) { r.ExceptionPolicy.MaximumDuration = "-1s" }}
	for i, mutate := range cases {
		r := completeRevision()
		mutate(&r)
		if _, err := s.Create("repo", "owner", r); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d error = %v", i, err)
		}
	}
}
