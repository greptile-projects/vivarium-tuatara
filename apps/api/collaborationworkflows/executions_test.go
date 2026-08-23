package collaborationworkflows

import (
	"strings"
	"testing"
	"time"
)

func executionFixture(t *testing.T) (*Store, Workflow, TriggerEvent) {
	t.Helper()
	s, _ := New(t.TempDir())
	p := s.Preview("repo", validDefinition(), Source{Revision: strings.Repeat("a", 40), Path: "workflow.json", SHA256: "digest"}, func(Invocation) (bool, string) { return true, "" })
	w, err := s.Create("repo", "owner", "activation", p)
	if err != nil {
		t.Fatal(err)
	}
	e := TriggerEvent{ID: "delivery-1", Kind: "repository_event", Name: "pull.opened", ActorID: "owner", OccurredAt: time.Now().UTC(), Inputs: map[string]any{"pull_id": "pull-1"}, ResourceRevisions: map[string]string{"pull": strings.Repeat("b", 40)}}
	return s, w, e
}

func TestExecutionIsIdempotentRevisionBoundAndDependencyScheduled(t *testing.T) {
	s, w, event := executionFixture(t)
	ex, err := s.StartExecution(w.ID, 1, event)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.StartExecution(w.ID, 1, event)
	if err != nil || retry.ID != ex.ID {
		t.Fatalf("retry=%#v %v", retry, err)
	}
	if _, err = s.ClaimStep(ex.ID, "notify", ex.Version, true); !errorsIs(err, ErrExecutionBlocked) {
		t.Fatalf("early dependency=%v", err)
	}
	lease, err := s.ClaimStep(ex.ID, "classify", ex.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Token == "" || len(lease.Authority) != 2 || lease.Inputs["pull"] != nil {
		t.Fatalf("lease=%#v", lease)
	}
	ex, err = s.CompleteStep(ex.ID, "classify", lease.Token, 1, map[string]any{"review_id": "review-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	notify, err := s.ClaimStep(ex.ID, "notify", ex.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	ex, err = s.CompleteStep(ex.ID, "notify", notify.Token, 1, nil, "")
	if err != nil || ex.Status != "succeeded" {
		t.Fatalf("execution=%#v %v", ex, err)
	}
	retry, err = s.CompleteStep(ex.ID, "notify", notify.Token, 1, nil, "")
	if err != nil || retry.Version != ex.Version || retry.Status != "succeeded" {
		t.Fatalf("completion retry=%#v %v", retry, err)
	}
}

func TestTerminalFailureRevokesConcurrentSiblingLease(t *testing.T) {
	s, _ := New(t.TempDir())
	d := validDefinition()
	d.Steps[1].Needs = nil
	d.Steps[0].Retries = 0
	p := s.Preview("repo", d, Source{Revision: strings.Repeat("a", 40)}, func(Invocation) (bool, string) { return true, "" })
	w, _ := s.Create("repo", "owner", "parallel", p)
	event := TriggerEvent{ID: "parallel-event", Kind: "repository_event", Name: "pull.opened", ActorID: "owner", OccurredAt: time.Now().UTC(), Inputs: map[string]any{"pull_id": "pull-1"}, ResourceRevisions: map[string]string{"pull_id": strings.Repeat("b", 40)}}
	ex, err := s.StartExecution(w.ID, 1, event)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := s.ClaimStep(ex.ID, "classify", ex.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := s.ClaimStep(ex.ID, "notify", failed.Execution.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	ex, err = s.CompleteStep(ex.ID, "classify", failed.Token, 1, nil, "action_failed")
	if err != nil || ex.Status != "failed" {
		t.Fatalf("terminal=%#v %v", ex, err)
	}
	other := executionStep(&ex, "notify")
	if other.Status != "cancelled" || other.CredentialSHA256 != "" || other.CredentialExpiresAt != nil || other.FailureCode != "execution_terminal" {
		t.Fatalf("sibling lease retained: %#v", other)
	}
	if _, err = s.CompleteStep(ex.ID, "notify", sibling.Token, 0, nil, ""); !errorsIs(err, ErrExecutionBlocked) {
		t.Fatalf("revoked sibling completion=%v", err)
	}
}

func TestExecutionRejectsSecretsBudgetDuplicateMutationAndConcurrentRun(t *testing.T) {
	s, w, event := executionFixture(t)
	ex, err := s.StartExecution(w.ID, 1, event)
	if err != nil {
		t.Fatal(err)
	}
	other := event
	other.ID = "delivery-2"
	if _, err = s.StartExecution(w.ID, 1, other); !errorsIs(err, ErrExecutionBlocked) {
		t.Fatalf("concurrency=%v", err)
	}
	changed := event
	changed.Inputs = map[string]any{"pull_id": "different"}
	if _, err = s.StartExecution(w.ID, 1, changed); !errorsIs(err, ErrExecutionConflict) {
		t.Fatalf("duplicate mutation=%v", err)
	}
	lease, err := s.ClaimStep(ex.ID, "classify", ex.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteStep(ex.ID, "classify", lease.Token, 3, nil, ""); !errorsIs(err, ErrExecutionBlocked) {
		t.Fatalf("step budget=%v", err)
	}
	if _, err = s.CompleteStep(ex.ID, "classify", lease.Token, 1, map[string]any{"review_id": "token=abcdefghijklmnop"}, ""); !errorsIs(err, ErrInvalid) {
		t.Fatalf("secret output=%v", err)
	}
}

func TestExecutionCanRetryInterruptedStepAndCancel(t *testing.T) {
	s, w, event := executionFixture(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	ex, _ := s.StartExecution(w.ID, 1, event)
	lease, _ := s.ClaimStep(ex.ID, "classify", ex.Version, true)
	now = now.Add(61 * time.Second)
	if _, err := s.CompleteStep(ex.ID, "classify", lease.Token, 0, nil, ""); !errorsIs(err, ErrCredential) {
		t.Fatalf("expiry=%v", err)
	}
	ex, _ = s.GetExecution(ex.ID)
	if ex.Steps[0].Status != "interrupted" {
		t.Fatalf("step=%#v", ex.Steps[0])
	}
	if _, err := s.ClaimStep(ex.ID, "classify", ex.Version, true); err != nil {
		t.Fatal(err)
	}
	ex, err := s.CancelExecution(ex.ID, "access_revoked")
	if err != nil || ex.Status != "cancelled" || ex.CancellationCode != "access_revoked" {
		t.Fatalf("cancel=%#v %v", ex, err)
	}
}

func errorsIs(err, target error) bool { return err == target }
