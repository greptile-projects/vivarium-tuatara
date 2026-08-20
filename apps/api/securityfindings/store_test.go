package securityfindings

import (
	"errors"
	"strings"
	"testing"
)

func candidate() Finding {
	return Finding{ThreatModelID: "threat", ThreatModelVersion: 2, AbusePathID: "replay", CandidateCommitID: strings.Repeat("a", 40), Title: "Replay bypass", Description: "A bounded replay reaches a protected action", Severity: "high", Audience: []string{"owner", "reporter"}, Evidence: []Evidence{{ID: "attempt", Kind: "scenario_attempt", Summary: "Sanitized failed containment", SHA256: strings.Repeat("b", 64)}}, AcceptanceCriteria: []string{"A replay is contained", "Detection remains observable"}}
}

func TestFindingClassificationAudienceAndRepairAreAppendOnly(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create("repo", "reporter", candidate())
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.List("repo", "outsider"); len(got) != 0 {
		t.Fatalf("outsider received finding: %+v", got)
	}
	v, err = s.Decide("repo", v.ID, "owner", v.Version, "confirmed", "Reproduced against the exact base", []string{"owner", "reporter"})
	if err != nil || CurrentClassification(v) != "confirmed" {
		t.Fatalf("classification failed: %+v %v", v, err)
	}
	v, err = s.LinkRepair("repo", v.ID, "owner", v.Version, Repair{ProposalID: "proposal", TaskID: "task", AssigneeType: "agent", AssigneeID: "bounded-agent"})
	if err != nil || v.Repair.State != "in_progress" {
		t.Fatalf("repair failed: %+v %v", v, err)
	}
	v, err = s.Complete("repo", v.ID, "owner", v.Version, "pull", strings.Repeat("c", 40), "scenario")
	if err != nil || v.Repair.State != "protected" || len(v.Events) != 3 {
		t.Fatalf("protection failed: %+v %v", v, err)
	}
}

func TestOnlyCurrentConfirmedFindingCanEnterRepair(t *testing.T) {
	s, _ := New(t.TempDir())
	for _, classification := range []string{"suspected_duplicate", "false_positive", "accepted_risk", "embargoed", "failed_repair"} {
		v, _ := s.Create("repo", "reporter", candidate())
		v, _ = s.Decide("repo", v.ID, "owner", v.Version, classification, "Attributable resolution", []string{"owner"})
		_, err := s.LinkRepair("repo", v.ID, "owner", v.Version, Repair{ProposalID: "p", TaskID: "t", AssigneeType: "human", AssigneeID: "owner"})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s entered repair: %v", classification, err)
		}
	}
}
