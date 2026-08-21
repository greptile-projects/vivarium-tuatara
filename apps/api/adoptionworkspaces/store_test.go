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
	if _, err = s.Get(x.ID, "maintainer"); !errorsIs(err, ErrNotFound) {
		t.Fatalf("pending invite exposed workspace: %v", err)
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
	if _, err = s.Get(x.ID, "maintainer"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(x.ID, "reader"); err != nil {
		t.Fatal(err)
	}
}

func TestAgentCannotReceiveWriteRole(t *testing.T) {
	s, _ := New(t.TempDir())
	_, err := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "agent", PrincipalID: "agent", OrganizationID: "org", Role: "provider_maintainer"}})
	if !errorsIs(err, ErrInvalid) {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func errorsIs(got, want error) bool { return got == want }
