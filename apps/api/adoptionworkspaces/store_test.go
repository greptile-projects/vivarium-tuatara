package adoptionworkspaces

import "testing"

func fixture() Workspace {
	return Workspace{Title: "Adopt relay", Outcome: "Deliver events reliably", Source: Source{Kind: "package", RepositoryID: "repo", ResourceID: "relay@2.0.0", Label: "Relay package", Resolution: "resolved"}, RequiredJourneys: []string{"publish and replay"}, Environments: []string{"linux"}, Constraints: []string{"no payload retention"}, BudgetCents: 10000, Currency: "USD", Owners: []Owner{{PrincipalID: "owner", Responsibility: "decision"}}, Criteria: []Criterion{{Name: "journey fit", Requirement: "passes replay", Weight: 100, OwnerID: "owner"}}, Candidates: []Candidate{{Name: "Relay", Provider: "provider", Version: "2.0.0", SourceKind: "package", SourceReference: "relay@2.0.0", Evidence: []Evidence{{Dimension: "capabilities", Summary: "ordered events", Reference: "https://example.test/capabilities", ObservedVersion: "2.0.0", Visibility: "public", Resolution: "resolved"}, {Dimension: "support", Summary: "older policy", Reference: "https://example.test/support", ObservedVersion: "1.0.0", Visibility: "participants", Resolution: "resolved"}}}}}
}

func TestWorkspaceConsentAndEvidenceGaps(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	x, err := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "human", PrincipalID: "maintainer", Role: "provider_maintainer"}, {PrincipalType: "agent", PrincipalID: "reader", OrganizationID: "org", Role: "observer"}})
	if err != nil {
		t.Fatal(err)
	}
	if x.Candidates[0].FitStatus != "undetermined" || x.Candidates[0].Evidence[1].Resolution != "stale" {
		t.Fatalf("expected version-derived gaps, got %+v", x.Candidates[0])
	}
	maintainer := Viewer{PrincipalType: "human", PrincipalID: "maintainer"}
	if _, err = s.Get(x.ID, maintainer); !errorsIs(err, ErrNotFound) {
		t.Fatalf("pending invite exposed workspace: %v", err)
	}
	pending, err := s.Pending(maintainer)
	if err != nil || len(pending) != 1 || pending[0].WorkspaceID != x.ID {
		t.Fatalf("pending invitation not discoverable: %+v, %v", pending, err)
	}
	var invitation string
	for _, v := range x.Invitations {
		if v.PrincipalID == "maintainer" {
			invitation = v.ID
		}
	}
	x, err = s.Consent(x.ID, invitation, "maintainer", "accepted", x.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(x.ID, maintainer); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(x.ID, Viewer{PrincipalType: "agent", PrincipalID: "maintainer", OrganizationID: "org"}); !errorsIs(err, ErrNotFound) {
		t.Fatalf("human invitation authorized colliding agent: %v", err)
	}
	if _, err = s.Get(x.ID, Viewer{PrincipalType: "agent", PrincipalID: "reader", OrganizationID: "org"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(x.ID, Viewer{PrincipalType: "human", PrincipalID: "reader"}); !errorsIs(err, ErrNotFound) {
		t.Fatalf("agent invitation authorized colliding human: %v", err)
	}
}

func TestRepositoryEvidenceIsRedactedForParticipantWithoutAccess(t *testing.T) {
	s, _ := New(t.TempDir())
	s.ConfigureRepositoryAccess(func(viewer Viewer, repository string) bool {
		return viewer.PrincipalID == "owner" && repository == "private-repo"
	})
	in := fixture()
	in.Candidates[0].Evidence[0].RepositoryID = "private-repo"
	in.Candidates[0].Evidence[0].Summary = "SECRET_PRIVATE_EVIDENCE"
	x, err := s.Create(in, "owner", []Invitation{{PrincipalType: "human", PrincipalID: "participant", Role: "affected_user"}})
	if err != nil {
		t.Fatal(err)
	}
	invitation := x.Invitations[0].ID
	x, err = s.Consent(x.ID, invitation, "participant", "accepted", x.Version)
	if err != nil {
		t.Fatal(err)
	}
	got := x.Candidates[0].Evidence[0]
	if got.Resolution != "inaccessible" || got.RepositoryID != "" || got.Summary != "Restricted evidence" || got.Reference != "Restricted evidence" {
		t.Fatalf("private evidence leaked: %+v", got)
	}
}

func TestAgentCannotReceiveWriteRole(t *testing.T) {
	s, _ := New(t.TempDir())
	_, err := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "agent", PrincipalID: "agent", OrganizationID: "org", Role: "provider_maintainer"}})
	if !errorsIs(err, ErrInvalid) {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestBoundedTrialRetainsFailedEvidenceAndRejectsCredentials(t *testing.T) {
	s, _ := New(t.TempDir())
	x, err := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "agent", PrincipalID: "trial-agent", OrganizationID: "org", Role: "observer"}})
	if err != nil {
		t.Fatal(err)
	}
	viewer := Viewer{PrincipalType: "agent", PrincipalID: "trial-agent", OrganizationID: "org"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", RepositoryID: "repo", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, Packages: []string{"relay@2.0.0"}, APIs: []string{"events/v2"}, DataKind: "synthetic", DataDescription: "generated ordered events", Journeys: []string{"publish and replay"}, Policies: []string{"no payload retention"}, Setup: []string{"create an empty fixture"}, Configuration: []string{"retention=off"}, Commands: []string{"relay verify fixture.json"}, IntegrationChanges: []string{"wire the test adapter"}, MaximumCostCents: 500}
	x, err = s.CreateTrial(x.ID, trial, viewer, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	x, err = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, TrialAttempt{Status: "failed", Reproducible: true, Checks: []string{"replay check failed"}, Measurements: []string{"p95=31ms"}, CostCents: 120, Findings: []string{"duplicates after reconnect"}}, viewer, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Trials[0].Attempts) != 1 || x.Trials[0].Attempts[0].Status != "failed" {
		t.Fatalf("failed trial disappeared: %+v", x.Trials)
	}
	for _, credential := range []string{"Authorization: Bearer secret", "Authorization: Basic dXNlcjpwYXNz", "github_pat_11ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"} {
		bad := trial
		bad.Commands = []string{"curl -H '" + credential + "'"}
		if _, err = s.CreateTrial(x.ID, bad, viewer, x.Version); !errorsIs(err, ErrInvalid) {
			t.Fatalf("credential-shaped command %q accepted: %v", credential, err)
		}
	}
}

func TestTrialCostCeilingIsCumulative(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "owner", nil)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "synthetic", DataDescription: "fixture", Journeys: []string{"publish and replay"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}, MaximumCostCents: 100}
	x, err := s.CreateTrial(x.ID, trial, v, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	attempt := TrialAttempt{Status: "failed", CostCents: 60, Findings: []string{"retryable failure"}}
	x, err = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, attempt, v, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, attempt, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("cumulative cost overage accepted: %v", err)
	}
}

func TestTrialMustUseDeclaredJourneyAndBudget(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "owner", nil)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "permitted", DataDescription: "approved anonymized fixture", Journeys: []string{"undeclared journey"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}}
	if _, err := s.CreateTrial(x.ID, trial, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("undeclared journey accepted: %v", err)
	}
}

func TestTrialSourceIsRedactedAfterRepositoryAccessLoss(t *testing.T) {
	s, _ := New(t.TempDir())
	allowed := true
	s.ConfigureRepositoryAccess(func(Viewer, string) bool { return allowed })
	x, _ := s.Create(fixture(), "owner", nil)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", RepositoryID: "private", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "synthetic", DataDescription: "fixture", Journeys: []string{"publish and replay"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}}
	x, err := s.CreateTrial(x.ID, trial, v, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	allowed = false
	x, err = s.Get(x.ID, v)
	if err != nil {
		t.Fatal(err)
	}
	if x.Trials[0].Source.Resolution != "inaccessible" || x.Trials[0].Source.Revision != "" || x.Trials[0].Source.ResourceID != "" {
		t.Fatalf("restricted trial source leaked: %+v", x.Trials[0].Source)
	}
}

func TestSuccessfulTrialCreatesOwnedOrderedAdoptionAgreement(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "human", PrincipalID: "maintainer", Role: "provider_maintainer"}})
	x, _ = s.Consent(x.ID, x.Invitations[0].ID, "maintainer", "accepted", x.Version)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "synthetic", DataDescription: "fixture", Journeys: []string{"publish and replay"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}, MaximumCostCents: 100}
	x, _ = s.CreateTrial(x.ID, trial, v, x.Version)
	x, _ = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, TrialAttempt{Status: "passed", Reproducible: true, Checks: []string{"journey passed"}}, Viewer{PrincipalType: "human", PrincipalID: "maintainer"}, x.Version)
	plan := validAdoptionPlan(x)
	s.ConfigureEnvironmentResolver(func(repositoryID, environmentID string) bool {
		return repositoryID == "consumer" && environmentID == "env-live"
	})
	plan.Work[1].Kind, plan.Work[1].EnvironmentID = "environment", "env-live"
	bad := plan
	bad.Work = append([]AdoptionWork(nil), plan.Work...)
	bad.Work[1].EnvironmentID = "missing-or-cross-repository"
	if _, err := s.CreatePlan(x.ID, bad, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("unresolved environment accepted: %v", err)
	}
	x, err := s.CreatePlan(x.ID, plan, v, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Plans) != 1 || len(x.Plans[0].Work) != 2 || len(x.Plans[0].Work[1].DependencyIDs) != 1 || x.Plans[0].Work[1].DependencyIDs[0] != x.Plans[0].Work[0].ID || x.Plans[0].Work[0].Authority != "no_authority_granted" {
		t.Fatalf("agreement did not retain ordered authority-safe work: %+v", x.Plans)
	}
	s.ConfigurePlanTargetProjection(func(_ Viewer, work AdoptionWork) AdoptionWork {
		work.EffectiveAccess, work.RepositoryID, work.OwnerID = "inaccessible", "restricted", "restricted"
		return work
	})
	projected, _ := s.Get(x.ID, v)
	if projected.Plans[0].Work[0].EffectiveAccess != "inaccessible" || projected.Plans[0].Work[0].RepositoryID != "restricted" || projected.Plans[0].Work[0].OwnerID != "restricted" {
		t.Fatalf("current target projection leaked stale facts: %+v", projected.Plans[0].Work[0])
	}
}

func TestAdoptionAgreementRequiresIndependentlyReproducedPass(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "owner", nil)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "synthetic", DataDescription: "fixture", Journeys: []string{"publish and replay"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}}
	x, _ = s.CreateTrial(x.ID, trial, v, x.Version)
	x, _ = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, TrialAttempt{Status: "passed", Reproducible: true, Checks: []string{"self asserted"}}, v, x.Version)
	plan := validAdoptionPlan(x)
	if _, err := s.CreatePlan(x.ID, plan, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("unproven agreement accepted: %v", err)
	}
}

func TestDeliveryRequiresCompleteAttestationsAndSafeRestore(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "human", PrincipalID: "maintainer", Role: "provider_maintainer"}})
	x, _ = s.Consent(x.ID, x.Invitations[0].ID, "maintainer", "accepted", x.Version)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "synthetic", DataDescription: "fixture", Journeys: []string{"publish and replay"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}}
	x, _ = s.CreateTrial(x.ID, trial, v, x.Version)
	x, _ = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, TrialAttempt{Status: "passed", Reproducible: true, Checks: []string{"passed"}}, Viewer{PrincipalType: "human", PrincipalID: "maintainer"}, x.Version)
	x, _ = s.CreatePlan(x.ID, validAdoptionPlan(x), v, x.Version)
	base := AdoptionDelivery{PlanID: x.Plans[0].ID, ConsumerRepositoryID: "consumer", PullRequestID: "pull", PullRevision: "1111111111111111111111111111111111111111", MergeRevision: "2222222222222222222222222222222222222222", ReleaseID: "release", ReleaseRevision: "3333333333333333333333333333333333333333", DeploymentID: "deployment", EnvironmentID: "environment", ProviderRepositoryID: "provider", ProviderRevision: "0123456789012345678901234567890123456789", CheckRunIDs: []string{"check"}, ApprovalIDs: []string{"approval"}, Rollout: []string{"staged rollout completed"}, Health: []string{"journey healthy"}, Currency: "USD", SupportReadiness: "on-call ready", UserAcceptance: "target users accepted", State: "operating"}
	for _, kind := range []string{"policy", "rehearsal", "support", "user_acceptance", "cost"} {
		base.Attestations = append(base.Attestations, DeliveryAttestation{Kind: kind, Statement: kind + " satisfied", Satisfied: true, AttestedBy: "owner"})
	}
	bad := base
	bad.Attestations = append([]DeliveryAttestation(nil), base.Attestations...)
	bad.Attestations[0].Satisfied = false
	if _, err := s.CreateDelivery(x.ID, bad, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("unmet operating criteria accepted: %v", err)
	}
	forged := base
	forged.Attestations = append([]DeliveryAttestation(nil), base.Attestations...)
	forged.Attestations[0].AttestedBy = "unrelated-human"
	if _, err := s.CreateDelivery(x.ID, forged, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("forged attestation attribution accepted: %v", err)
	}
	paused := base
	paused.State, paused.PauseReasons = "paused", []string{"failed rollout"}
	paused.Attestations = bad.Attestations
	x, err := s.CreateDelivery(x.ID, paused, v, x.Version)
	if err != nil || x.Deliveries[0].Authority != "no_authority_granted" {
		t.Fatalf("safe pause not retained: %+v, %v", x.Deliveries, err)
	}
	restored := base
	restored.State, restored.RestoresDeliveryID = "restored", x.Deliveries[0].ID
	restored.RecoveryOfDeploymentID = x.Deliveries[0].DeploymentID
	unrelated := restored
	unrelated.RecoveryOfDeploymentID = "unrelated-failed-deployment"
	if _, err = s.CreateDelivery(x.ID, unrelated, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("recovery linked to unrelated paused delivery: %v", err)
	}
	x, err = s.CreateDelivery(x.ID, restored, v, x.Version)
	if err != nil || x.Deliveries[1].RestoresDeliveryID != x.Deliveries[0].ID {
		t.Fatalf("governed restoration not linked: %+v, %v", x.Deliveries, err)
	}
	s.ConfigureDeliveryProjection(func(_ Viewer, delivery AdoptionDelivery) AdoptionDelivery {
		delivery.State, delivery.ConsumerRepositoryID = "access_revoked", "restricted"
		return delivery
	})
	projected, _ := s.Get(x.ID, v)
	if projected.Deliveries[1].State != "access_revoked" || projected.Deliveries[1].ConsumerRepositoryID != "restricted" {
		t.Fatalf("access loss not projected: %+v", projected.Deliveries[1])
	}
}

func TestRedactedFindingRequiresProviderConsentBeforeUpstreamWork(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "human", PrincipalID: "maintainer", Role: "provider_maintainer"}, {PrincipalType: "human", PrincipalID: "user", Role: "affected_user"}})
	x, _ = s.Consent(x.ID, x.Invitations[0].ID, "maintainer", "accepted", x.Version)
	x, _ = s.Consent(x.ID, x.Invitations[1].ID, "user", "accepted", x.Version)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "synthetic", DataDescription: "fixture", Journeys: []string{"publish and replay"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}}
	x, _ = s.CreateTrial(x.ID, trial, v, x.Version)
	x, _ = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, TrialAttempt{Status: "failed", Reproducible: true, Findings: []string{"duplicate delivery"}}, v, x.Version)
	finding := SharedFinding{Kind: "reproduction", TrialID: x.Trials[0].ID, AttemptID: x.Trials[0].Attempts[0].ID, Summary: "Reconnect duplicates one synthetic event", Reproduction: []string{"disconnect after acknowledgement"}, Evidence: []string{"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Redactions: []string{"removed account and payload fields"}, Visibility: "participants", State: "pending_consent"}
	x, err := s.ShareFinding(x.ID, finding, v, x.Version)
	if err != nil || x.SharedFindings[0].ProviderStatus != "awaiting_consent" {
		t.Fatalf("finding was not retained for consent: %+v, %v", x.SharedFindings, err)
	}
	participant, _ := s.Get(x.ID, Viewer{PrincipalType: "human", PrincipalID: "user"})
	if len(participant.SharedFindings) != 0 {
		t.Fatalf("unconsented finding metadata leaked: %+v", participant.SharedFindings)
	}
	contribution := UpstreamContribution{FindingID: x.SharedFindings[0].ID, Kind: "fork_pull", TargetRepositoryID: "provider", SourceRepositoryID: "fork", ResourceID: "pull", Revision: "1111111111111111111111111111111111111111", Status: "open", Resolution: "ordinary maintainer review"}
	if _, err = s.RecordContribution(x.ID, contribution, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("unconsented upstream contribution accepted: %v", err)
	}
	x, err = s.ConsentFinding(x.ID, x.SharedFindings[0].ID, "maintainer", "accepted", x.Version)
	if err != nil || x.SharedFindings[0].State != "shared" || x.SharedFindings[0].ConsentedBy != "maintainer" {
		t.Fatalf("provider consent not retained: %+v, %v", x.SharedFindings, err)
	}
	x, err = s.RecordContribution(x.ID, contribution, v, x.Version)
	if err != nil || x.Contributions[0].Authority != "no_authority_granted" || x.Contributions[0].AuthoredBy != "owner" {
		t.Fatalf("ordinary upstream contribution not linked safely: %+v, %v", x.Contributions, err)
	}
}

func TestEmbargoAndRejectionRetainLocalPatchAndVerifiedReplacement(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "human", PrincipalID: "maintainer", Role: "provider_maintainer"}})
	x, _ = s.Consent(x.ID, x.Invitations[0].ID, "maintainer", "accepted", x.Version)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "synthetic", DataDescription: "fixture", Journeys: []string{"publish and replay"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}}
	x, _ = s.CreateTrial(x.ID, trial, v, x.Version)
	x, _ = s.ShareFinding(x.ID, SharedFinding{Kind: "compatibility_evidence", TrialID: x.Trials[0].ID, Summary: "Private compatibility gap", Redactions: []string{"consumer identity removed"}, Visibility: "provider", State: "embargoed"}, v, x.Version)
	local := UpstreamContribution{FindingID: x.SharedFindings[0].ID, Kind: "local_pull", TargetRepositoryID: "consumer", SourceRepositoryID: "consumer", ResourceID: "local-pull", Revision: "1111111111111111111111111111111111111111", Status: "local_only", Resolution: "provider unavailable; retain consumer patch"}
	x, err := s.RecordContribution(x.ID, local, v, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	upstream := UpstreamContribution{FindingID: x.SharedFindings[0].ID, Kind: "fork_pull", TargetRepositoryID: "provider", SourceRepositoryID: "fork", ResourceID: "upstream-pull", Revision: "2222222222222222222222222222222222222222", Status: "merged", Resolution: "accepted through ordinary review", AuthoredBy: "owner", AuthoredByType: "human"}
	// A later explicit share is modeled independently; the embargo remains immutable.
	x, err = s.ShareFinding(x.ID, SharedFinding{Kind: "compatibility_evidence", TrialID: x.Trials[0].ID, Summary: "Public synthetic compatibility gap", Redactions: []string{"consumer identity removed"}, Visibility: "public", State: "pending_consent"}, v, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordContribution(x.ID, upstream, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("unconsented replacement accepted: %v", err)
	}
	sharedFindingID := x.SharedFindings[1].ID
	x, err = s.ConsentFinding(x.ID, sharedFindingID, "maintainer", "accepted", x.Version)
	if err != nil {
		t.Fatal(err)
	}
	upstream.FindingID = sharedFindingID
	x, err = s.RecordContribution(x.ID, upstream, v, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	upstream = x.Contributions[1]
	update := VerifiedUpdate{ContributionID: upstream.ID, ProviderRepositoryID: "provider", ProviderReleaseID: "provider-release", ProviderReleaseRevision: "3333333333333333333333333333333333333333", ConsumerRepositoryID: "consumer", ConsumerPullRequestID: "update-pull", ConsumerPullRevision: "4444444444444444444444444444444444444444", ConsumerReleaseID: "consumer-release", ConsumerReleaseRevision: "5555555555555555555555555555555555555555", ConsumerDeploymentID: "deployment", ReplacesContributionID: x.Contributions[0].ID, VerificationKind: "exact_package_inventory", PackageName: "relay", PackageVersion: "2.1.0", ReplacedPaths: []string{"src/relay-workaround.go"}, Outcome: "provider release replaced the local patch", CheckRunIDs: []string{"check"}, State: "verified"}
	x, err = s.RecordVerifiedUpdate(x.ID, update, v, x.Version)
	if err != nil || x.VerifiedUpdates[0].Authority != "no_authority_granted" {
		t.Fatalf("verified patch replacement not retained: %+v, %v", x.VerifiedUpdates, err)
	}
}

func validAdoptionPlan(x Workspace) AdoptionPlan {
	return AdoptionPlan{CandidateID: x.Candidates[0].ID, TrialID: x.Trials[0].ID, SelectedVersion: "2.0.0", IntegrationArchitecture: "Consumer adapter calls the provider API", ConfigurationOwnership: []DecisionOwner{{Decision: "Consumer retry policy", OwnerID: "owner", Party: "adopter"}}, UpdatePolicy: "Monthly review; security updates within seven days", SupportPolicy: "Provider triages protocol defects; adopter owns local operations", ServiceBoundaries: []string{"Provider ends at the public API"}, DataBoundaries: []string{"No payload retention"}, RequiredExceptions: []string{"Temporary retry-policy exception tracked by consumer"}, ExitStrategy: "Remove the adapter and replay queued events", UnresolvedFitGaps: []string{"Windows remains unverified"}, CompatibilityPromises: []string{"Provider supports events/v2 through 2027"}, RecurringCostCents: 2500, Currency: "USD", Work: []AdoptionWork{{Position: 1, Kind: "consumer_repository", Title: "Implement adapter", RepositoryID: "consumer", Paths: []string{"src/adapter"}, OwnerType: "human", OwnerID: "owner", OwnerStatus: "current", AcceptanceCriteria: []string{"Journey passes"}, EffectiveAccess: "owner"}, {Position: 2, Kind: "documentation", Title: "Document support boundary", RepositoryID: "consumer", Paths: []string{"docs/relay.md"}, OwnerType: "human", OwnerID: "owner", OwnerStatus: "current", AcceptanceCriteria: []string{"Owners approve"}, EffectiveAccess: "owner"}}}
}

func errorsIs(got, want error) bool { return got == want }
