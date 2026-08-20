package agentreleases

import (
	"strings"
	"testing"
	"time"
)

func approvedRelease() Release {
	now := time.Now().UTC()
	approvals := []Approval{}
	for _, kind := range []string{"evaluation", "domain_review", "pilot_acceptance", "data_policy", "resources"} {
		approvals = append(approvals, Approval{Kind: kind, OwnerID: "owner", EvidenceID: kind + "-evidence", Decision: "approved", ApprovedAt: now})
	}
	return Release{OrganizationID: "org", RepositoryID: "repo", AgentID: "agent", CandidateID: "candidate", CandidateRevision: strings.Repeat("a", 40), ProjectID: "project", ProjectVersion: 2, ContractDigest: strings.Repeat("b", 64), ModelVersions: []string{"model@2"}, ToolVersions: []string{"git@1"}, Roles: []string{"contributor"}, Approvals: approvals, PilotID: "pilot", CreatedBy: "owner"}
}

func TestReleaseDeploymentSignalsAndRollback(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rollback, err := s.CreateRelease(approvedRelease())
	if err != nil {
		t.Fatal(err)
	}
	next := approvedRelease()
	next.CandidateID = "candidate-next"
	next.CandidateRevision = strings.Repeat("c", 40)
	release, err := s.CreateRelease(next)
	if err != nil {
		t.Fatal(err)
	}
	if release.Attestation == rollback.Attestation || release.Status != "attested" {
		t.Fatalf("attestation did not bind candidate: %+v", release)
	}
	d, err := s.CreateDeployment(Deployment{ReleaseID: release.ID, Identity: "agent:stable", Roles: []string{"contributor"}, CredentialScopes: []string{"repository.read"}, Budget: Budget{MaxCost: 20, MaxActions: 10, MaxMinutes: 60}, RollbackReleaseID: rollback.ID, OperatorTerms: "Human operator responds and may withdraw access.", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	d, err = s.Signal(d.ID, "owner", 1, Signal{Kind: "safety", Outcome: "Unsafe tool request was denied.", Safety: "denied"})
	if err != nil || len(d.Signals) != 1 {
		t.Fatalf("signal = %+v, %v", d, err)
	}
	d, err = s.Control(d.ID, "owner", 2, "rollback", "Regression exceeded the reviewed safety boundary.")
	if err != nil || d.Status != "rolled_back" || len(d.Events) != 2 {
		t.Fatalf("rollback = %+v, %v", d, err)
	}
}

func TestReleaseRequiresEveryIndependentApproval(t *testing.T) {
	s, _ := New(t.TempDir())
	v := approvedRelease()
	v.Approvals = v.Approvals[:4]
	if _, err := s.CreateRelease(v); err != ErrDenied {
		t.Fatalf("missing resource approval = %v", err)
	}
}

func TestSuccessorDoesNotInheritDeploymentConsent(t *testing.T) {
	s, _ := New(t.TempDir())
	old, _ := s.CreateRelease(approvedRelease())
	next := approvedRelease()
	next.CandidateID = "new"
	next.CandidateRevision = strings.Repeat("d", 40)
	current, _ := s.CreateRelease(next)
	if _, err := s.CreateDeployment(Deployment{ReleaseID: current.ID, Identity: "agent:stable", Roles: []string{"contributor"}, CredentialScopes: []string{"repository.read"}, Budget: Budget{MaxCost: 1, MaxActions: 1, MaxMinutes: 1}, RollbackReleaseID: old.ID}); err != ErrInvalid {
		t.Fatalf("successor inherited missing operator consent: %v", err)
	}
}
