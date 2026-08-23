package collaborationworkflows

import "testing"

func validDefinition() Definition {
	return Definition{Name: "Review follow-up", Outcome: "Every accepted change receives its required review", Description: "Coordinate review", OwnerIDs: []string{"owner"}, BudgetActions: 10, Outputs: []string{"review"}, Completion: []string{"review requested"}, Triggers: []Trigger{{ID: "pull", Kind: "repository_event", Event: "pull.opened", Inputs: []Input{{Name: "pull_id", Type: "string", Required: true, Source: "event.pull_id"}}}}, Steps: []Step{{ID: "classify", Name: "Classify", Invocation: Invocation{Kind: "platform_action", Action: "request_review", Authority: []string{"pulls:read", "reviews:write"}}, TimeoutSeconds: 60, Retries: 2, BudgetActions: 2, OwnerIDs: []string{"owner"}, Outputs: []string{"review_id"}, Completion: []string{"review request exists"}}, {ID: "notify", Name: "Notify", Needs: []string{"classify"}, Invocation: Invocation{Kind: "component", Component: "project-notifications", Authority: []string{"notifications:write"}}, TimeoutSeconds: 30, Retries: 1, BudgetActions: 1, OwnerIDs: []string{"owner"}, Completion: []string{"owners notified"}}}}
}

func TestPreviewExplainsOrderSubscriptionsAndAuthority(t *testing.T) {
	s, _ := New(t.TempDir())
	p := s.Preview("repo", validDefinition(), Source{Revision: "abc", Path: "workflow.json"}, func(Invocation) (bool, string) { return true, "" })
	if !p.Activatable || len(p.Diagnostics) != 0 || len(p.ExecutionOrder) != 2 || p.ExecutionOrder[1][0] != "notify" || p.Subscriptions[0] != "repository_event:pull.opened" || len(p.EffectiveAuthority) != 2 {
		t.Fatalf("unexpected preview: %#v", p)
	}
}

func TestPreviewBlocksCyclesLoopsPoliciesAndResources(t *testing.T) {
	s, _ := New(t.TempDir())
	d := validDefinition()
	d.Steps[0].Needs = []string{"notify"}
	d.Steps[0].Invocation.Emits = []string{"pull.opened"}
	d.Policies = []Policy{{ID: "security", AllowActions: []string{"request_review"}, DenyActions: []string{"request_review"}}}
	p := s.Preview("repo", d, Source{}, func(inv Invocation) (bool, string) {
		if inv.Kind == "component" {
			return false, "component inaccessible"
		}
		return true, ""
	})
	if p.Activatable {
		t.Fatal("invalid workflow was activatable")
	}
	kinds := map[string]bool{}
	for _, d := range p.Diagnostics {
		kinds[d.Kind] = true
	}
	for _, kind := range []string{"invalid_graph", "trigger_loop", "conflicting_policy", "inaccessible_resource"} {
		if !kinds[kind] {
			t.Fatalf("missing %s: %#v", kind, p.Diagnostics)
		}
	}
}

func TestVersionsAreImmutableAndCASBound(t *testing.T) {
	s, _ := New(t.TempDir())
	p := s.Preview("repo", validDefinition(), Source{}, func(Invocation) (bool, string) { return true, "" })
	w, e := s.Create("repo", "owner", "activation-1", p)
	if e != nil {
		t.Fatal(e)
	}
	d := validDefinition()
	d.Outcome = "A clearer shared outcome"
	p = s.Preview("repo", d, Source{}, func(Invocation) (bool, string) { return true, "" })
	if _, e = s.Revise(w.ID, 0, "owner", p); e != ErrConflict {
		t.Fatalf("expected conflict, got %v", e)
	}
	w, e = s.Revise(w.ID, 1, "owner", p)
	if e != nil || len(w.Revisions) != 2 || w.Revisions[0].Definition.Outcome == w.Revisions[1].Definition.Outcome {
		t.Fatalf("unexpected revisions: %#v %v", w, e)
	}
}

func TestCreateActivationIDIsRetrySafe(t *testing.T) {
	s, _ := New(t.TempDir())
	p := s.Preview("repo", validDefinition(), Source{Revision: "abc", Path: "workflow.json"}, func(Invocation) (bool, string) { return true, "" })
	first, e := s.Create("repo", "owner", "request-123", p)
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.Create("repo", "owner", "request-123", p)
	if e != nil || second.ID != first.ID {
		t.Fatalf("retry = %#v, %v", second, e)
	}
	all, _ := s.List("repo")
	if len(all) != 1 {
		t.Fatalf("retry created %d workflows", len(all))
	}
}

func TestReviseRejectsPersistedAndIndirectWorkflowRecursion(t *testing.T) {
	s, _ := New(t.TempDir())
	check := func(Invocation) (bool, string) { return true, "" }
	p := s.Preview("repo", validDefinition(), Source{Revision: "a"}, check)
	a, e := s.Create("repo", "owner", "a", p)
	if e != nil {
		t.Fatal(e)
	}
	bDef := validDefinition()
	bDef.Name = "B"
	bDef.Steps[0].Invocation = Invocation{Kind: "workflow", WorkflowID: a.ID, Authority: []string{"workflow:invoke"}}
	bPreview := s.Preview("repo", bDef, Source{Revision: "b"}, check)
	b, e := s.Create("repo", "owner", "b", bPreview)
	if e != nil {
		t.Fatal(e)
	}
	self := validDefinition()
	self.Steps[0].Invocation = Invocation{Kind: "workflow", WorkflowID: a.ID, Authority: []string{"workflow:invoke"}}
	if _, e = s.Revise(a.ID, 1, "owner", s.Preview("repo", self, Source{Revision: "self"}, check)); e != ErrInvalid {
		t.Fatalf("persisted self reference = %v", e)
	}
	cycle := validDefinition()
	cycle.Steps[0].Invocation = Invocation{Kind: "workflow", WorkflowID: b.ID, Authority: []string{"workflow:invoke"}}
	if _, e = s.Revise(a.ID, 1, "owner", s.Preview("repo", cycle, Source{Revision: "cycle"}, check)); e != ErrInvalid {
		t.Fatalf("indirect cycle = %v", e)
	}
}
