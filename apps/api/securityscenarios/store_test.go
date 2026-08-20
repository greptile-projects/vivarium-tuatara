package securityscenarios

import "testing"

func sample() Scenario {
	return Scenario{ThreatModelID: "model", ThreatModelVersion: 1, AbusePathID: "path", Title: "Reject replay", AttackerPreconditions: []string{"synthetic expired session"}, BoundedCapabilities: []string{"loopback HTTP requests"}, Fixtures: []Fixture{{ID: "session", Description: "generated session", Path: "testdata/session.json", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", DataClass: "synthetic", Generator: "fixed seed"}}, Actions: []string{"submit the session twice"}, Containment: []Criterion{{ID: "c", Description: "second use is denied", Observable: "status", Expected: "409"}}, Detection: []Criterion{{ID: "d", Description: "replay is counted", Observable: "audit event", Expected: "replay_denied"}}, Recovery: []Criterion{{ID: "r", Description: "new session works", Observable: "status", Expected: "200"}}, MitigationIDs: []string{"nonce"}, CommitID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", CheckPath: ".vivarium/security-checks.json", CheckSHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", Command: "bun test replay", Isolation: "workspace", MaxCostUnits: 5}
}

func TestScenarioReviewAndAttempts(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create("repo", "agent", "agent", sample())
	if err != nil {
		t.Fatal(err)
	}
	a := Attempt{Revision: v.CommitID, ExecutionKind: "workspace", Commands: []Command{{OutcomeID: "out", SHA256: "digest"}}, Coverage: Coverage{AbuseAttempted: true, ContainmentIDs: []string{"c"}}, CostUnits: 1, Result: "failed", Provenance: []string{"workspace:out"}}
	if _, err = s.AddAttempt("repo", v.ID, "agent", a); err != ErrInvalid {
		t.Fatalf("unreviewed attempt = %v", err)
	}
	v, err = s.Review("repo", v.ID, "owner", "approved", "safe and bounded")
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.AddAttempt("repo", v.ID, "agent", a)
	if err != nil || len(v.Attempts) != 1 {
		t.Fatalf("attempt = %#v, %v", v.Attempts, err)
	}
	if _, err = s.Review("repo", v.ID, "owner", "approved", "again"); err != ErrConflict {
		t.Fatalf("second review = %v", err)
	}
}

func TestUnsafeAndNonReproducibleRemainExplicit(t *testing.T) {
	s, _ := New(t.TempDir())
	v, _ := s.Create("repo", "owner", "human", sample())
	v, _ = s.Review("repo", v.ID, "owner", "approved", "bounded")
	a := Attempt{Revision: v.CommitID, ExecutionKind: "workspace", Result: "unsafe", UnsafeReasons: []string{"dependency would require production access"}, Provenance: []string{"workspace:w"}}
	if _, err := s.AddAttempt("repo", v.ID, "owner", a); err != nil {
		t.Fatal(err)
	}
	a.Result, a.UnsafeReasons, a.NonReproducibleReasons = "not_reproducible", nil, []string{"dependency inaccessible"}
	if _, err := s.AddAttempt("repo", v.ID, "owner", a); err != nil {
		t.Fatal(err)
	}
	a.Result, a.NonReproducibleReasons = "unsafe", nil
	if _, err := s.AddAttempt("repo", v.ID, "owner", a); err != ErrInvalid {
		t.Fatalf("silent unsafe result accepted: %v", err)
	}
}
