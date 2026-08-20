package threatmodels

import "testing"

func modelRevision() Revision {
	return Revision{Title: "Login attack surface", Source: Source{Kind: "pull_request", ResourceID: "pull-1", Revision: "abc", Summary: "Login redesign"}, ArchitectureDigest: "arch-1", TrustBoundaryDigest: "trust-1", EntryPoints: []EntryPoint{{ID: "login", Name: "Login", Access: "public", Boundary: "internet"}}, Privileges: []Privilege{{ID: "session", Principal: "user", Capability: "create session", Scope: "account"}}, DataFlows: []DataFlow{{ID: "credentials", From: "browser", To: "api", Data: "password", Protection: "TLS"}}, Dependencies: []Dependency{{ID: "identity", Name: "Identity provider", Revision: "1", Trust: "partial"}}, Mitigations: []Mitigation{{ID: "rate", Description: "Rate limit attempts", Status: "proposed", EvidenceIDs: []string{"design"}, OwnerIDs: []string{"owner"}}}, Evidence: []Evidence{{ID: "design", Kind: "source", ResourceID: "pull-1", Revision: "abc", Summary: "Visible pull diff", Accessible: true}, {ID: "restricted", Kind: "incident", ResourceID: "incident-1", Revision: "1", Summary: "Restricted evidence gap", Accessible: false, Gap: "not in audience"}}, Alternatives: []Alternative{{ID: "password", Name: "Passwords", Description: "Existing flow", SecurityEffect: "phishable", Tradeoffs: []string{"familiar"}, EvidenceIDs: []string{"design"}}, {ID: "passkey", Name: "Passkeys", Description: "Bound credential", SecurityEffect: "resists phishing", Tradeoffs: []string{"recovery"}, EvidenceIDs: []string{"design"}}}, AbusePaths: []AbusePath{{ID: "stuffing", Goal: "take over accounts", EntryPointIDs: []string{"login"}, PrivilegeIDs: []string{"session"}, DataFlowIDs: []string{"credentials"}, DependencyIDs: []string{"identity"}, Steps: []string{"submit leaked passwords"}, Impact: "account access", MitigationIDs: []string{"rate"}, ResidualRisk: "distributed attempts", OwnerIDs: []string{"owner"}}}, OwnerIDs: []string{"owner"}, Assumptions: []string{"identity provider is available"}}
}
func TestCollaborativeEventsStayRevisionBoundAndCited(t *testing.T) {
	s, _ := New(t.TempDir())
	v, e := s.Create("repo", "owner", modelRevision())
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.AddEvent("repo", v.ID, 1, "agent-1", "agent", Event{Kind: "alternative_comparison", Body: "Passkeys remove reusable secrets.", ResourceIDs: []string{"stuffing"}, EvidenceIDs: []string{"design"}, AlternativeIDs: []string{"password", "passkey"}})
	if e != nil || len(v.Events) != 1 || v.Events[0].ModelVersion != 1 {
		t.Fatalf("event=%#v err=%v", v.Events, e)
	}
	if _, e = s.AddEvent("repo", v.ID, 1, "agent-1", "agent", Event{Kind: "finding", Body: "leak", EvidenceIDs: []string{"restricted"}}); e != ErrInvalid {
		t.Fatalf("restricted evidence err=%v", e)
	}
}
func TestFreshnessDerivesAffectedSourceAndDependencyMovement(t *testing.T) {
	s, _ := New(t.TempDir())
	v, _ := s.Create("repo", "owner", modelRevision())
	fresh, e := s.Get("repo", v.ID, CurrentSource{Revision: "abc", ArchitectureDigest: "arch-1", TrustBoundaryDigest: "trust-1", DependencyRevisions: map[string]string{"identity": "1"}})
	if e != nil || !fresh.Freshness.Fresh {
		t.Fatalf("fresh=%#v err=%v", fresh.Freshness, e)
	}
	stale, _ := s.Get("repo", v.ID, CurrentSource{Revision: "def", ArchitectureDigest: "arch-2", TrustBoundaryDigest: "trust-2", DependencyRevisions: map[string]string{"identity": "2"}})
	if stale.Freshness.Fresh || len(stale.Freshness.Reasons) != 4 {
		t.Fatalf("stale=%#v", stale.Freshness)
	}
}
func TestOnlyAffectedOwnerCanAcknowledgeCurrentRevision(t *testing.T) {
	s, _ := New(t.TempDir())
	v, _ := s.Create("repo", "owner", modelRevision())
	if _, e := s.Acknowledge("repo", v.ID, 1, "other", Acknowledgement{OwnerID: "other", Decision: "acknowledged", Note: "ok"}); e != ErrInvalid {
		t.Fatalf("outsider err=%v", e)
	}
	v, e := s.Acknowledge("repo", v.ID, 1, "owner", Acknowledgement{OwnerID: "owner", Decision: "changes_requested", Note: "Prefer passkeys"})
	if e != nil || len(v.Acknowledgements) != 1 {
		t.Fatalf("ack=%#v err=%v", v.Acknowledgements, e)
	}
}
