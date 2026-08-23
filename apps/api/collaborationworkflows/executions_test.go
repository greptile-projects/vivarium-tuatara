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
	secretArtifacts := []StepArtifact{{Name: "password=abcdefghijklmnop", Kind: "report", SHA256: strings.Repeat("c", 64), Size: 12}}
	if _, err = s.CompleteStepEvidence(ex.ID, "classify", lease.Token, 1, nil, "", nil, secretArtifacts, nil, 0, nil); !errorsIs(err, ErrInvalid) {
		t.Fatalf("secret artifact name=%v", err)
	}
	secretArtifacts[0].Name, secretArtifacts[0].Kind = "sanitized report", "api_key=abcdefghijklmnop"
	if _, err = s.CompleteStepEvidence(ex.ID, "classify", lease.Token, 1, nil, "", nil, secretArtifacts, nil, 0, nil); !errorsIs(err, ErrInvalid) {
		t.Fatalf("secret artifact kind=%v", err)
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
	retryLease, err := s.ClaimStep(ex.ID, "classify", ex.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	ex, err = s.CompleteStep(ex.ID, "classify", retryLease.Token, 1, map[string]any{"review_id": "retry-review"}, "")
	if err != nil || len(ex.Steps[0].Attempts) != 2 || ex.Steps[0].Attempts[1].Status != "succeeded" {
		t.Fatalf("completed retry=%#v %v", ex.Steps[0], err)
	}
	ex, err = s.CancelExecution(ex.ID, "access_revoked")
	if err != nil || ex.Status != "cancelled" || ex.CancellationCode != "access_revoked" {
		t.Fatalf("cancel=%#v %v", ex, err)
	}
}

func TestExecutionEvidenceProjectionAndAttributedInterventions(t *testing.T) {
	s, _, event := executionFixture(t)
	d := validDefinition()
	d.Steps[0].Optional = true
	d.Steps[1].Manual = true
	d.Steps[1].Invocation.Kind = "manual"
	d.Steps[1].Invocation.Authority = nil
	p := s.Preview("repo", d, Source{Revision: strings.Repeat("c", 40)}, func(Invocation) (bool, string) { return true, "" })
	w, err := s.Create("repo", "owner", "observable", p)
	if err != nil {
		t.Fatal(err)
	}
	event.ID = "observable-event"
	ex, err := s.StartExecution(w.ID, 1, event)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimStep(ex.ID, "classify", ex.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	ex, err = s.CompleteStepEvidence(ex.ID, "classify", lease.Token, 1, map[string]any{"review_id": "review-1"}, "transient", []StepLog{{Time: time.Now().UTC(), Level: "error", Message: "bounded failure"}}, []StepArtifact{{Name: "report", Kind: "log", SHA256: strings.Repeat("d", 64), Size: 12, Restricted: true}}, &AgentSession{ID: "session-1", AgentID: "agent-1", Status: "failed"}, 2.5, []string{"runner:isolated"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Steps[0].Attempts) != 1 || ex.Steps[0].Attempts[0].CostUnits != 2.5 {
		t.Fatalf("attempt=%#v", ex.Steps[0].Attempts)
	}
	ex, err = s.Intervene(ex.ID, "collaborator", "retry", "classify", "retry retained failure", "", nil, ex.Version)
	if err != nil || ex.Steps[0].Status != "pending" || len(ex.Interventions) != 1 {
		t.Fatalf("retry=%#v %v", ex, err)
	}
	ex, err = s.Intervene(ex.ID, "collaborator", "skip", "classify", "optional result is not required", "", nil, ex.Version)
	if err != nil || ex.Steps[0].Status != "skipped" {
		t.Fatalf("skip=%#v %v", ex, err)
	}
	ex, err = s.Intervene(ex.ID, "collaborator", "take_over", "notify", "handle declared manual work", "", nil, ex.Version)
	if err != nil || ex.Steps[1].TakenOverBy != "collaborator" {
		t.Fatalf("takeover=%#v %v", ex, err)
	}
	public := PublicExecution(ex)
	if public.Steps[0].CredentialSHA256 != "" || public.Steps[0].CompletionSHA256 != "" || len(public.Steps[0].Attempts[0].Artifacts) != 0 {
		t.Fatalf("projection=%#v", public.Steps[0])
	}
	if _, err = s.Intervene(ex.ID, "collaborator", "provide_input", "notify", "unsafe", "value", "password=abcdefghijklmnop", ex.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("secret intervention=%v", err)
	}
}

func TestProvidedInputIsFrozenAcrossRetriedAttempts(t *testing.T) {
	s, _ := New(t.TempDir())
	d := validDefinition()
	d.Steps[0].RequestedInputs = []string{"decision"}
	p := s.Preview("repo", d, Source{Revision: strings.Repeat("a", 40)}, func(Invocation) (bool, string) { return true, "" })
	w, err := s.Create("repo", "owner", "frozen-input", p)
	if err != nil {
		t.Fatal(err)
	}
	event := TriggerEvent{ID: "input-event", Kind: "repository_event", Name: "pull.opened", ActorID: "owner", OccurredAt: time.Now().UTC(), Inputs: map[string]any{"pull_id": "pull-1"}, ResourceRevisions: map[string]string{"pull_id": strings.Repeat("b", 40)}}
	ex, err := s.StartExecution(w.ID, 1, event)
	if err != nil {
		t.Fatal(err)
	}
	ex, err = s.Intervene(ex.ID, "owner", "provide_input", "classify", "supply reviewed decision", "decision", "approved", ex.Version)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimStep(ex.ID, "classify", ex.Version, true)
	if err != nil {
		t.Fatal(err)
	}
	ex, err = s.CompleteStep(ex.ID, "classify", lease.Token, 1, nil, "transient")
	if err != nil {
		t.Fatal(err)
	}
	ex, err = s.Intervene(ex.ID, "owner", "retry", "classify", "retry the same reviewed input", "", nil, ex.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Intervene(ex.ID, "owner", "provide_input", "classify", "replace prior input", "decision", "rejected", ex.Version); !errorsIs(err, ErrExecutionBlocked) {
		t.Fatalf("replacement input=%v", err)
	}
	retry, err := s.ClaimStep(ex.ID, "classify", ex.Version, true)
	if err != nil || retry.Inputs["decision"] != "approved" {
		t.Fatalf("retry inputs=%#v %v", retry.Inputs, err)
	}
}

func errorsIs(err, target error) bool { return err == target }
