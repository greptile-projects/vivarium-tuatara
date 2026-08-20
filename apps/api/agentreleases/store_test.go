package agentreleases

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func approvedRelease(candidate, revision string) Release {
	now := time.Now().UTC()
	approvals := []Approval{}
	for i, kind := range []string{"evaluation", "domain_review", "pilot_acceptance", "data_policy", "resources"} {
		approvals = append(approvals, Approval{ID: kind + "-approval", CandidateID: candidate, Kind: kind, OwnerID: "owner-" + string(rune('a'+i)), EvidenceID: kind + "-evidence", Decision: "approved", ApprovedAt: now})
	}
	return Release{OrganizationID: "org", RepositoryID: "repo", AgentID: "agent", CandidateID: candidate, CandidateRevision: revision, ProjectID: "project", ProjectVersion: 2, ContractDigest: strings.Repeat("b", 64), ModelVersions: []string{"model@2"}, ToolVersions: []string{"git@1"}, Roles: []string{"contributor"}, Approvals: approvals, PilotID: "pilot", CreatedBy: "owner"}
}

func TestRollbackMustBelongToSameRepository(t *testing.T) {
	s, _ := New(t.TempDir())
	current, _ := s.CreateRelease(approvedRelease("current", strings.Repeat("a", 40)))
	otherInput := approvedRelease("other", strings.Repeat("b", 40))
	otherInput.RepositoryID = "other-repo"
	other, _ := s.CreateRelease(otherInput)
	_, err := s.CreateDeployment(Deployment{ReleaseID: current.ID, Identity: "agent:stable", Roles: []string{"contributor"}, CredentialScopes: []string{"repository.read"}, Budget: Budget{MaxCost: 1, MaxActions: 1, MaxMinutes: 1}, RollbackReleaseID: other.ID, OperatorTerms: "Operator accepts bounded deployment.", CreatedBy: "owner"})
	if err != ErrDenied {
		t.Fatalf("cross-repository rollback = %v", err)
	}
}

func TestDeploymentCASAcrossStores(t *testing.T) {
	root := t.TempDir()
	seed, _ := New(root)
	rollback, _ := seed.CreateRelease(approvedRelease("old", strings.Repeat("a", 40)))
	current, _ := seed.CreateRelease(approvedRelease("new", strings.Repeat("b", 40)))
	d, _ := seed.CreateDeployment(Deployment{ReleaseID: current.ID, Identity: "agent:stable", Roles: []string{"contributor"}, CredentialScopes: []string{"repository.read"}, Budget: Budget{MaxCost: 1, MaxActions: 2, MaxMinutes: 1}, RollbackReleaseID: rollback.ID, OperatorTerms: "Operator accepts bounded deployment.", CreatedBy: "owner"})
	a, _ := New(root)
	b, _ := New(root)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, store := range []*Store{a, b} {
		wg.Add(1)
		go func(store *Store) {
			defer wg.Done()
			<-start
			_, err := store.Signal(d.ID, "owner", 1, Signal{Kind: "outcome", Outcome: "bounded"})
			errs <- err
		}(store)
	}
	close(start)
	wg.Wait()
	close(errs)
	successes, conflicts := 0, 0
	for err := range errs {
		if err == nil {
			successes++
		} else if err == ErrConflict {
			conflicts++
		}
	}
	got, _ := seed.GetDeployment(d.ID)
	if successes != 1 || conflicts != 1 || got.Version != 2 || len(got.Signals) != 1 {
		t.Fatalf("successes=%d conflicts=%d deployment=%+v", successes, conflicts, got)
	}
}

func TestReleaseDeploymentSignalsAndRollback(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rollback, err := s.CreateRelease(approvedRelease("candidate", strings.Repeat("a", 40)))
	if err != nil {
		t.Fatal(err)
	}
	next := approvedRelease("candidate-next", strings.Repeat("c", 40))
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
	v := approvedRelease("candidate", strings.Repeat("a", 40))
	v.Approvals = v.Approvals[:4]
	if _, err := s.CreateRelease(v); err != ErrDenied {
		t.Fatalf("missing resource approval = %v", err)
	}
}

func TestSuccessorDoesNotInheritDeploymentConsent(t *testing.T) {
	s, _ := New(t.TempDir())
	old, _ := s.CreateRelease(approvedRelease("candidate", strings.Repeat("a", 40)))
	next := approvedRelease("new", strings.Repeat("d", 40))
	current, _ := s.CreateRelease(next)
	if _, err := s.CreateDeployment(Deployment{ReleaseID: current.ID, Identity: "agent:stable", Roles: []string{"contributor"}, CredentialScopes: []string{"repository.read"}, Budget: Budget{MaxCost: 1, MaxActions: 1, MaxMinutes: 1}, RollbackReleaseID: old.ID}); err != ErrInvalid {
		t.Fatalf("successor inherited missing operator consent: %v", err)
	}
}
