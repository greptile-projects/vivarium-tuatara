package incidents

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAddUpdateReconcilesPostPublicationRetry(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actor, repository := strings.Repeat("a", 32), strings.Repeat("b", 32)
	incident, err := store.Create(Incident{Title: "Outage", Summary: "Requests fail", Severity: "sev1", Status: "investigating", DeclaredBy: actor, Scopes: []Scope{{RepositoryID: repository}}, Roles: []Role{{Name: "commander", UserID: actor}}})
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("directory sync failed")
	store.directorySync = func(string) error { return injected }
	operation := strings.Repeat("c", 32)
	if _, err = store.AddUpdate(incident.ID, operation, actor, "Mitigation started", "participants"); !errors.Is(err, injected) {
		t.Fatalf("first update error = %v", err)
	}
	retried, err := store.AddUpdate(incident.ID, operation, actor, "Mitigation started", "participants")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range retried.Timeline {
		if entry.ID == operation {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("operation entries = %d: %#v", count, retried.Timeline)
	}
	if _, err = store.AddUpdate(incident.ID, operation, actor, "Different update", "participants"); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting reuse = %v", err)
	}
}

func TestFindingRequiresBoundedOperationalEvidence(t *testing.T) {
	store, _ := New(t.TempDir())
	actor, repository := strings.Repeat("a", 32), strings.Repeat("b", 32)
	incident, err := store.Create(Incident{Title: "Outage", Summary: "Requests fail", Severity: "sev1", Status: "investigating", DeclaredBy: actor, Scopes: []Scope{{RepositoryID: repository}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddFinding(incident.ID, strings.Repeat("c", 32), actor, "observation", "Logs show errors.", "participants", []Evidence{{Kind: "log", RepositoryID: repository, ResourceID: strings.Repeat("d", 32), Label: "deployment logs"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded log finding error = %v", err)
	}
	start, end := time.Now().Add(-time.Minute), time.Now()
	created, err := store.AddFinding(incident.ID, strings.Repeat("c", 32), actor, "observation", "Logs show errors.", "participants", []Evidence{{Kind: "log", RepositoryID: repository, ResourceID: strings.Repeat("d", 32), Label: "deployment logs", WindowStart: &start, WindowEnd: &end}})
	if err != nil || len(created.Timeline) != 2 || created.Timeline[1].Evidence[0].CapturedAt.IsZero() {
		t.Fatalf("finding = %#v, %v", created, err)
	}
	retried, err := store.AddFinding(incident.ID, strings.Repeat("c", 32), actor, "observation", "Logs show errors.", "participants", []Evidence{{Kind: "log", RepositoryID: repository, ResourceID: strings.Repeat("d", 32), Label: "deployment logs · failed", WindowStart: &start, WindowEnd: &end}})
	if err != nil || len(retried.Timeline) != 2 || retried.Timeline[1].Evidence[0].Label != "deployment logs" {
		t.Fatalf("mutable-label retry = %#v, %v", retried, err)
	}
}

func TestMitigationRequiresIndependentDecisionAndRetainsFailedAttempts(t *testing.T) {
	store, _ := New(t.TempDir())
	proposer, approver, repository, deployment := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32), strings.Repeat("d", 32)
	incident, err := store.Create(Incident{Title: "Outage", Summary: "Requests fail", Severity: "sev1", Status: "identified", DeclaredBy: proposer, Scopes: []Scope{{RepositoryID: repository}}})
	if err != nil {
		t.Fatal(err)
	}
	evidence := []Evidence{{Kind: "deployment", RepositoryID: repository, ResourceID: deployment, Label: "failed production deployment"}}
	proposalOperation := strings.Repeat("e", 32)
	incident, action, err := store.ProposeAction(incident.ID, proposalOperation, proposer, "restore_release", repository, deployment, "Restore the last attested release.", evidence, []HealthCriterion{{Stage: "steady", Signal: "availability"}})
	if err != nil || action.Status != "proposed" || len(incident.Timeline) != 2 {
		t.Fatalf("proposal = %#v, %v", action, err)
	}
	if _, _, err = store.DecideAction(incident.ID, action.ID, proposer, "approve", "I approve my proposal.", false); !errors.Is(err, ErrConflict) {
		t.Fatalf("self approval = %v", err)
	}
	incident, action, err = store.DecideAction(incident.ID, action.ID, approver, "approve", "Evidence supports rollback.", false)
	if err != nil || action.Status != "approved" || action.Decisions[0].ActorID != approver {
		t.Fatalf("approval = %#v, %v", action, err)
	}
	attemptOperation := strings.Repeat("f", 32)
	incident, action, err = store.RecordActionAttempt(incident.ID, action.ID, attemptOperation, proposer, "failed", "", "No earlier artifact passed policy.")
	if err != nil || action.Status != "failed" || len(action.Attempts) != 1 || incident.Timeline[len(incident.Timeline)-1].Kind != "mitigation_failed" {
		t.Fatalf("attempt = %#v, %v", action, err)
	}
	reopened, _ := New(store.root)
	retried, sameAction, err := reopened.ProposeAction(incident.ID, proposalOperation, proposer, "restore_release", repository, deployment, "Restore the last attested release.", evidence, []HealthCriterion{{Stage: "steady", Signal: "availability"}})
	if err != nil || sameAction.ID != action.ID || len(retried.Actions) != 1 {
		t.Fatalf("proposal retry = %#v, %#v, %v", retried.Actions, sameAction, err)
	}
	retried, sameAction, err = reopened.RecordActionAttempt(incident.ID, action.ID, attemptOperation, proposer, "failed", "", "No earlier artifact passed policy.")
	if err != nil || len(sameAction.Attempts) != 1 || len(retried.Timeline) != len(incident.Timeline) {
		t.Fatalf("attempt retry = %#v, %v", sameAction, err)
	}
}
