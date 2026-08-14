package dataobservations

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validObservation(now time.Time) Observation {
	return Observation{
		SignalKind: "failed_deletion", Severity: "blocking", OwnerIDs: []string{"owner"},
		Scope:    Scope{Revision: strings.Repeat("a", 40), DataFlowID: "flow", DataFlowVersion: 2, CommitmentID: "commitment", CommitmentVersion: 3, DataUseID: "account", ReleaseID: "release", EnvironmentID: "production", DeploymentID: "deployment", ExtensionInstallationID: "extension"},
		Evidence: []Evidence{{Kind: "deletion_receipt", Digest: strings.Repeat("b", 64), WindowStart: now.Add(-time.Hour), WindowEnd: now, SampleCount: 7}},
	}
}

func TestLifecycleRetainsOnlySanitizedEvidenceAndGovernedLinks(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	v, err := s.Create("repo", "agent", "scanner", validObservation(now))
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "open" || v.CreatedByType != "agent" || len(v.Evidence) != 1 || v.Evidence[0].Digest == "" {
		t.Fatalf("unexpected observation: %#v", v)
	}
	expires := now.Add(24 * time.Hour)
	v, err = s.AddAction("repo", v.ID, "collaborator", 1, Action{Kind: "governed_exception", Rationale: "Allow deletion queue drain while repair is reviewed.", ParticipantIDs: []string{"owner"}, ExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.AddAction("repo", v.ID, "collaborator", 2, Action{Kind: "contain", Rationale: "Stop new account ingestion."})
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "contained" || len(v.Actions) != 2 {
		t.Fatalf("containment not retained: %#v", v)
	}
	v, err = s.LinkRepair("repo", v.ID, "collaborator", 3, Repair{ProposalID: "proposal", TaskID: "task", AssigneeType: "agent", AssigneeID: "approved-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "repair_planned" || v.Repair == nil || v.Repair.ProposalID != "proposal" {
		t.Fatalf("repair not linked: %#v", v)
	}
	if _, err = s.AddAction("repo", v.ID, "collaborator", 3, Action{Kind: "notify", Rationale: "stale"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestRejectsPayloadLikeAndUnboundedEvidence(t *testing.T) {
	now := time.Now().UTC()
	s, _ := New(t.TempDir())
	for name, mutate := range map[string]func(*Observation){
		"unknown evidence kind": func(v *Observation) { v.Evidence[0].Kind = "raw_log" },
		"non digest":            func(v *Observation) { v.Evidence[0].Digest = "person@example.com" },
		"large window":          func(v *Observation) { v.Evidence[0].WindowStart = now.Add(-32 * 24 * time.Hour) },
		"invalid revision":      func(v *Observation) { v.Scope.Revision = strings.Repeat("z", 40) },
	} {
		t.Run(name, func(t *testing.T) {
			v := validObservation(now)
			mutate(&v)
			if _, err := s.Create("repo", "human", "actor", v); !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected invalid, got %v", err)
			}
		})
	}
}
