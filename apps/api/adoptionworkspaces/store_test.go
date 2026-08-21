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

func errorsIs(got, want error) bool { return got == want }
