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
	bad := trial
	bad.Commands = []string{"curl -H 'Authorization: Bearer secret'"}
	if _, err = s.CreateTrial(x.ID, bad, viewer, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("credential-shaped command accepted: %v", err)
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

func errorsIs(got, want error) bool { return got == want }
