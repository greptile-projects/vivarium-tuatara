package charters

import (
	"errors"
	"testing"
)

func validRevision() Revision {
	return Revision{Title: "Project charter", Summary: "Who decides and how.", Roles: []Role{{Name: "maintainer", Description: "Stewards the project", Eligibility: []string{"repository owner or current maintainer"}}}, DecisionClasses: []DecisionClass{{Name: "protected change", Description: "Changes protected resources", EligibleRoles: []string{"maintainer"}, Participation: 1, Quorum: 1, Approval: "majority", ProtectedResources: []string{"branch:main"}}}, Procedures: Procedures{Terms: "Annual", Removal: "Attributed vote", Succession: "Named successor", Amendments: "New approved revision"}}
}

func TestRevisionApprovalActivationAndHistory(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Publish("repository", "repo1", "owner", 0, validRevision())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Activate("repository", "repo1", "owner", 1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("activation without approval = %v", err)
	}
	r, err = s.Approve("repository", "repo1", "owner", 1, "approved", "adopt")
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Activate("repository", "repo1", "owner", 1)
	if err != nil {
		t.Fatal(err)
	}
	if r.ActiveVersion != 1 || r.Revisions[0].Status != "active" || len(r.Approvals) != 1 {
		t.Fatalf("record = %#v", r)
	}
	r, err = s.Publish("repository", "repo1", "owner", 1, validRevision())
	if err != nil {
		t.Fatal(err)
	}
	if r.Revisions[0].Status != "active" || r.Revisions[1].Status != "draft" {
		t.Fatalf("history changed = %#v", r.Revisions)
	}
}

func TestRejectsImpossibleInternalRules(t *testing.T) {
	s, _ := New(t.TempDir())
	v := validRevision()
	v.DecisionClasses[0].Quorum = 2
	if _, err := s.Publish("repository", "repo1", "owner", 0, v); !errors.Is(err, ErrInvalid) {
		t.Fatalf("publish = %v", err)
	}
}
