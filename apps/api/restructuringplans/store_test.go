package restructuringplans

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func resolvedCreate(s *Store, p Plan, actor, digest string) (Plan, error) {
	owners := map[string][]string{}
	for _, item := range p.Inventory {
		owners[item.ID] = append([]string(nil), item.OwnerIDs...)
	}
	return s.CreateResolved(p, actor, digest, owners)
}

func TestCreateRejectsCallerControlledInventoryOwners(t *testing.T) {
	s, _ := New(t.TempDir())
	if _, err := s.Create(completePlan(), "owner", "digest"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unresolved create err = %v", err)
	}
	p := completePlan()
	owners := map[string][]string{}
	for _, item := range p.Inventory {
		owners[item.ID] = append([]string(nil), item.OwnerIDs...)
	}
	owners[p.Inventory[0].ID] = []string{"different-authoritative-owner"}
	if _, err := s.CreateResolved(p, "owner", "digest", owners); !errors.Is(err, ErrInvalid) {
		t.Fatalf("mismatched resolved owners err = %v", err)
	}
}

func TestDependentMigrationKeepsFailuresVisibleAndOwnerControlled(t *testing.T) {
	s, _ := New(t.TempDir())
	p, _ := resolvedCreate(s, completePlan(), "owner", "digest")
	now := time.Now().UTC()
	m := DependentMigration{RequestID: "dependent-1", Kind: "clone", ResourceID: "developer-clone", OwnerID: "clone-owner", Audience: "public", State: "unmigrated", CompatibilityWindow: CompatibilityWindow{StartsAt: now, EndsAt: now.Add(7 * 24 * time.Hour)}, NextAction: "fetch the replacement, compare the signed tip, then change origin", ReplacementRemotes: []ReplacementRemote{{DestinationID: "core", RemoteURL: "https://example.test/core.git", Ref: "refs/heads/main"}}, Mappings: []DependencyLinkMapping{{From: "old/module", To: "core/module", Kind: "dependency"}}, Synchronization: []string{"git fetch replacement", "verify the expected tip", "git remote set-url origin https://example.test/core.git"}}
	p, err := s.AddDependentMigration("source", p.ID, "coordinator", p.Version, m)
	if err != nil {
		t.Fatal(err)
	}
	got := p.DependentMigrations[0]
	if got.State != "unmigrated" || got.OwnerID != "clone-owner" || len(got.ReplacementRemotes) != 1 {
		t.Fatalf("migration=%#v", got)
	}
	if _, err = s.AddDependentEvent("source", p.ID, got.ID, "coordinator", p.Version, DependentEvent{RequestID: "false-adoption", State: "adopted", NextAction: "done"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("coordinator adoption err=%v", err)
	}
	p, err = s.AddDependentEvent("source", p.ID, got.ID, "clone-owner", p.Version, DependentEvent{RequestID: "credentials", State: "stale_credentials", Evidence: "old token rejected", NextAction: "owner must issue a repository-scoped credential"})
	if err != nil {
		t.Fatal(err)
	}
	if p.DependentMigrations[0].State != "stale_credentials" || len(p.DependentMigrations[0].Events) != 1 {
		t.Fatalf("event=%#v", p.DependentMigrations[0])
	}
}

func TestDependentMigrationRejectsUnsafeOrIncompleteEntryPoint(t *testing.T) {
	s, _ := New(t.TempDir())
	p, _ := resolvedCreate(s, completePlan(), "owner", "digest")
	now := time.Now().UTC()
	base := DependentMigration{RequestID: "x", Kind: "fork", ResourceID: "fork", OwnerID: "owner", Audience: "public", State: "planned", CompatibilityWindow: CompatibilityWindow{StartsAt: now, EndsAt: now.Add(time.Hour)}, NextAction: "update remote", Synchronization: []string{"fetch first"}}
	if _, err := s.AddDependentMigration("source", p.ID, "owner", p.Version, base); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing replacement accepted: %v", err)
	}
	base.ReplacementRemotes = []ReplacementRemote{{DestinationID: "missing", RemoteURL: "https://example.test/new.git"}}
	if _, err := s.AddDependentMigration("source", p.ID, "owner", p.Version, base); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown destination accepted: %v", err)
	}
}

func TestCollaborationMappingRetainsIntentAndRequiresEveryAuthorApproval(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rev := "0123456789012345678901234567890123456789"
	plan := validPlanForTest(rev)
	created, err := resolvedCreate(store, plan, "author-a", "digest")
	if err != nil {
		t.Fatal(err)
	}
	mapping := CollaborationMapping{RequestID: "map-1", InventoryItemID: "i-0", Kind: "branch", SourceRepositoryID: "source", SourceResourceID: "resource-0", SourceRevision: rev, Disposition: "divide", Snapshot: CollaborationSnapshot{AuthorshipIDs: []string{"author-a", "author-b"}, DiscussionIDs: []string{"comment-1"}, ReviewIDs: []string{"review-1"}, DependencyIDs: []string{"issue-1"}, AcceptanceCriteria: []string{"behavior retained"}}, Destinations: []CollaborationDestination{{DestinationID: "core", ResourceID: "branch-a", Revision: rev, OwnerIDs: []string{"author-a"}, AcceptanceCriteria: []string{"checks pass"}}, {DestinationID: "destination-2", ResourceID: "branch-b", Revision: rev, OwnerIDs: []string{"author-b"}, DependencyIDs: []string{"branch-a"}, AcceptanceCriteria: []string{"connected check passes"}}}}
	created, err = store.AddCollaborationMapping("source", created.ID, "author-a", created.Version, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if got := created.CollaborationMappings[0]; got.State != "proposed" || len(got.Snapshot.DiscussionIDs) != 1 || len(got.Destinations) != 2 {
		t.Fatalf("mapping = %#v", got)
	}
	if retry, retryErr := store.AddCollaborationMapping("source", created.ID, "author-a", 1, mapping); retryErr != nil || retry.Version != created.Version {
		t.Fatalf("exact retry = %#v, %v", retry, retryErr)
	}
	created, err = store.DecideCollaborationMapping("source", created.ID, created.CollaborationMappings[0].ID, "author-a", created.Version, MappingDecision{RequestID: "decision-a", Decision: "approve", SourceRevision: rev})
	if err != nil {
		t.Fatal(err)
	}
	if created.CollaborationMappings[0].State != "proposed" {
		t.Fatal("one author silently approved for all authors")
	}
	created, err = store.DecideCollaborationMapping("source", created.ID, created.CollaborationMappings[0].ID, "author-b", created.Version, MappingDecision{RequestID: "decision-b", Decision: "approve", SourceRevision: rev})
	if err != nil {
		t.Fatal(err)
	}
	if created.CollaborationMappings[0].State != "approved" {
		t.Fatal("complete approval did not approve mapping")
	}
}

func TestCollaborationMappingRejectsStaleAndUnownedApproval(t *testing.T) {
	store, _ := New(t.TempDir())
	rev := "0123456789012345678901234567890123456789"
	created, _ := resolvedCreate(store, validPlanForTest(rev), "author-a", "digest")
	m := CollaborationMapping{RequestID: "map", InventoryItemID: "i-0", Kind: "branch", SourceRepositoryID: "source", SourceResourceID: "resource-0", SourceRevision: rev, Disposition: "move", Snapshot: CollaborationSnapshot{AuthorshipIDs: []string{"author-a", "author-b"}, AcceptanceCriteria: []string{"preserve intent"}}, Destinations: []CollaborationDestination{{DestinationID: "core", ResourceID: "new-main", Revision: rev, OwnerIDs: []string{"owner"}, AcceptanceCriteria: []string{"pass"}}}}
	created, err := store.AddCollaborationMapping("source", created.ID, "author-a", created.Version, m)
	if err != nil {
		t.Fatal(err)
	}
	id := created.CollaborationMappings[0].ID
	if _, err = store.DecideCollaborationMapping("source", created.ID, id, "outsider", created.Version, MappingDecision{RequestID: "x", Decision: "approve", SourceRevision: rev}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("outsider err=%v", err)
	}
	if _, err = store.DecideCollaborationMapping("source", created.ID, id, "author-a", created.Version, MappingDecision{RequestID: "y", Decision: "approve", SourceRevision: strings.Repeat("b", 40)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("stale err=%v", err)
	}
}

func TestCollaborationMappingRejectsOmittedInventoriedOwner(t *testing.T) {
	store, _ := New(t.TempDir())
	rev := "0123456789012345678901234567890123456789"
	created, _ := resolvedCreate(store, validPlanForTest(rev), "author-a", "digest")
	m := CollaborationMapping{RequestID: "omitted", InventoryItemID: "i-0", Kind: "branch", SourceRepositoryID: "source", SourceResourceID: "resource-0", SourceRevision: rev, Disposition: "move", Snapshot: CollaborationSnapshot{AuthorshipIDs: []string{"author-a"}, AcceptanceCriteria: []string{"preserve intent"}}, Destinations: []CollaborationDestination{{DestinationID: "core", ResourceID: "new-main", Revision: rev, OwnerIDs: []string{"owner"}, AcceptanceCriteria: []string{"pass"}}}}
	if _, err := store.AddCollaborationMapping("source", created.ID, "author-a", created.Version, m); !errors.Is(err, ErrInvalid) {
		t.Fatalf("omitted inventoried owner err = %v", err)
	}
}

func TestTerminalCollaborationMappingsRejectApprovals(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		embargoed, inaccessible bool
		disposition, state      string
	}{{"embargoed", true, false, "move", "blocked"}, {"inaccessible", false, true, "move", "blocked"}, {"archived", false, false, "archive", "archived"}} {
		t.Run(tc.name, func(t *testing.T) {
			store, _ := New(t.TempDir())
			rev := "0123456789012345678901234567890123456789"
			plan := validPlanForTest(rev)
			if tc.inaccessible {
				plan.Inventory[0].State = "inaccessible"
			}
			created, _ := resolvedCreate(store, plan, "author-a", "digest")
			m := CollaborationMapping{RequestID: "terminal", InventoryItemID: "i-0", Kind: "branch", SourceRepositoryID: "source", SourceResourceID: "resource-0", SourceRevision: rev, Disposition: tc.disposition, Embargoed: tc.embargoed, Snapshot: CollaborationSnapshot{AuthorshipIDs: []string{"author-a", "author-b"}, AcceptanceCriteria: []string{"preserve intent"}}}
			if tc.disposition != "archive" {
				m.Destinations = []CollaborationDestination{{DestinationID: "core", ResourceID: "new-main", Revision: rev, OwnerIDs: []string{"owner"}, AcceptanceCriteria: []string{"pass"}}}
			}
			created, err := store.AddCollaborationMapping("source", created.ID, "author-a", created.Version, m)
			if err != nil {
				t.Fatal(err)
			}
			if created.CollaborationMappings[0].State != tc.state {
				t.Fatalf("state=%s", created.CollaborationMappings[0].State)
			}
			if _, err = store.DecideCollaborationMapping("source", created.ID, created.CollaborationMappings[0].ID, "author-a", created.Version, MappingDecision{RequestID: "approval", Decision: "approve", SourceRevision: rev}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("terminal approval err=%v", err)
			}
			got, _ := store.Get("source", created.ID)
			if got.CollaborationMappings[0].State != tc.state {
				t.Fatalf("terminal state changed to %s", got.CollaborationMappings[0].State)
			}
		})
	}
}

func validPlanForTest(rev string) Plan {
	p := completePlan()
	p.Destinations = append(p.Destinations, Destination{ID: "destination-2", Name: "two", OwnerIDs: []string{"owner"}, Visibility: "private", DefaultBranch: "main"})
	p.Inventory[0].OwnerIDs = []string{"author-a", "author-b"}
	return p
}

func completePlan() Plan {
	rev := "0123456789012345678901234567890123456789"
	kinds := []string{"ref", "pull_request", "issue", "task", "release", "package", "documentation", "policy", "workspace", "automation", "consumer", "federated_relationship"}
	items := make([]InventoryItem, 0, len(kinds))
	for i, k := range kinds {
		items = append(items, InventoryItem{ID: fmt.Sprintf("i-%d", i), Kind: k, RepositoryID: "source", ResourceID: fmt.Sprintf("resource-%d", i), Revision: rev, OwnerIDs: []string{"owner"}, DestinationIDs: []string{"core"}, Disposition: "move", State: "resolved", Summary: "reviewed impact", Citation: "source record at exact revision"})
	}
	return Plan{RequestID: "request-1", RepositoryID: "source", Title: "Extract core", Intent: "Give the shared core an independent boundary.", Sources: []SourceRepository{{RepositoryID: "source", Revision: rev, Role: "primary"}}, Destinations: []Destination{{ID: "core", Name: "core", OwnerIDs: []string{"owner"}, Visibility: "public", DefaultBranch: "main"}}, Mappings: []ContentMapping{{ID: "map", SourceRepositoryID: "source", SourcePath: "src/core", DestinationID: "core", DestinationPath: "src", HistoryMode: "full", RetainIdentity: true, Disposition: "move"}}, Inventory: items, Deadline: time.Now().UTC().Add(24 * time.Hour), SuccessCriteria: []string{"owners agree on authority"}, RollbackLimits: []string{"before destination publication"}}
}
func TestIndependentStoresSerializeFindingCAS(t *testing.T) {
	root := t.TempDir()
	first, _ := New(root)
	second, _ := New(root)
	p, err := resolvedCreate(first, completePlan(), "owner", "digest")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, s := range []*Store{first, second} {
		wg.Add(1)
		go func(i int, s *Store) {
			defer wg.Done()
			<-start
			_, e := s.AddFinding("source", p.ID, fmt.Sprintf("actor-%d", i), "human", Finding{RequestID: fmt.Sprintf("race-%d", i), Version: 1, InventoryItemIDs: []string{"i-0"}, Body: "concurrent cited finding", Citations: []string{"citation"}})
			results <- e
		}(i, s)
	}
	close(start)
	wg.Wait()
	close(results)
	successes, versions := 0, 0
	for e := range results {
		if e == nil {
			successes++
		}
		if errors.Is(e, ErrVersion) {
			versions++
		}
	}
	if successes != 1 || versions != 1 {
		t.Fatalf("successes=%d version conflicts=%d", successes, versions)
	}
	out, err := first.Get("source", p.ID)
	if err != nil || len(out.Findings) != 1 || out.Version != 2 {
		t.Fatalf("persisted = %#v %v", out, err)
	}
}
func TestCreateRetryAndCitedAgentFinding(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	p := completePlan()
	one, e := resolvedCreate(s, p, "owner", "digest")
	if e != nil {
		t.Fatal(e)
	}
	two, e := resolvedCreate(s, p, "owner", "digest")
	if e != nil || two.ID != one.ID {
		t.Fatalf("retry = %#v %v", two, e)
	}
	if _, e = resolvedCreate(s, p, "owner", "changed"); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed retry = %v", e)
	}
	out, e := s.AddFinding("source", one.ID, "agent-1", "read_only_agent", Finding{RequestID: "finding-1", Version: 1, InventoryItemIDs: []string{"i-0"}, Body: "The release workflow still names the old path.", Citations: []string{"policy.yml@0123456789012345678901234567890123456789"}})
	if e != nil {
		t.Fatal(e)
	}
	if out.Version != 2 || len(out.Findings) != 1 || out.Findings[0].ActorKind != "read_only_agent" {
		t.Fatalf("finding not retained: %#v", out)
	}
	retry, e := s.AddFinding("source", one.ID, "agent-1", "read_only_agent", Finding{RequestID: "finding-1", Version: 1, InventoryItemIDs: []string{"i-0"}, Body: "The release workflow still names the old path.", Citations: []string{"policy.yml@0123456789012345678901234567890123456789"}})
	if e != nil || retry.Version != 2 || len(retry.Findings) != 1 {
		t.Fatalf("finding retry = %#v %v", retry, e)
	}
	if _, e = s.AddFinding("source", one.ID, "agent-1", "read_only_agent", Finding{RequestID: "finding-2", Version: 1, InventoryItemIDs: []string{"i-0"}, Body: "stale", Citations: []string{"citation"}}); !errors.Is(e, ErrVersion) {
		t.Fatalf("stale finding = %v", e)
	}
}
func TestRequiresCompleteExplicitInventory(t *testing.T) {
	s, _ := New(t.TempDir())
	p := completePlan()
	p.Inventory = p.Inventory[:len(p.Inventory)-1]
	if _, e := resolvedCreate(s, p, "owner", "digest"); !errors.Is(e, ErrInvalid) {
		t.Fatalf("incomplete inventory = %v", e)
	}
	p = completePlan()
	p.Inventory[0].State = "hidden"
	if _, e := resolvedCreate(s, p, "owner", "digest"); !errors.Is(e, ErrInvalid) {
		t.Fatalf("hidden state = %v", e)
	}
}

func TestPlanRejectsCandidatePathTraversal(t *testing.T) {
	s, _ := New(t.TempDir())
	for _, mutate := range []func(*Plan){
		func(p *Plan) { p.Destinations[0].ID = "../../outside"; p.Mappings[0].DestinationID = "../../outside" },
		func(p *Plan) { p.Mappings[0].DestinationPath = "../outside" },
		func(p *Plan) { p.Destinations[0].DefaultBranch = "main..escape" },
	} {
		p := completePlan()
		mutate(&p)
		if _, err := resolvedCreate(s, p, "owner", randomID()); !errors.Is(err, ErrInvalid) {
			t.Fatalf("unsafe plan accepted: %#v, %v", p, err)
		}
	}
}

func TestCandidateCreationSerializesAssemblyAndRegistration(t *testing.T) {
	root := t.TempDir()
	first, _ := New(root)
	second, _ := New(root)
	plan, err := resolvedCreate(first, completePlan(), "owner", "plan-digest")
	if err != nil {
		t.Fatal(err)
	}
	assemble := func(id, request, digest string) func(Plan) (CandidateSet, error) {
		return func(Plan) (CandidateSet, error) {
			return CandidateSet{ID: id, RequestID: request, RequestDigest: digest, Repositories: []CandidateRepository{{ID: "core", DestinationID: "core"}}}, nil
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, s := range []*Store{first, second} {
		go func(i int, s *Store) {
			<-start
			_, e := s.CreateCandidateSet("source", plan.ID, "owner", 1, fmt.Sprintf("request-%d", i), fmt.Sprintf("digest-%d", i), assemble(fmt.Sprintf("candidate-%d", i), fmt.Sprintf("request-%d", i), fmt.Sprintf("digest-%d", i)))
			results <- e
		}(i, s)
	}
	close(start)
	a, b := <-results, <-results
	if !((a == nil && errors.Is(b, ErrVersion)) || (b == nil && errors.Is(a, ErrVersion))) {
		t.Fatalf("distinct concurrent results = %v, %v", a, b)
	}
	current, err := first.Get("source", plan.ID)
	if err != nil || len(current.CandidateSets) != 1 {
		t.Fatalf("registered candidates = %#v, %v", current.CandidateSets, err)
	}
	calls := 0
	exact := func(Plan) (CandidateSet, error) {
		calls++
		return CandidateSet{}, errors.New("exact retry assembled again")
	}
	if _, err = second.CreateCandidateSet("source", plan.ID, "owner", 1, current.CandidateSets[0].RequestID, current.CandidateSets[0].RequestDigest, exact); err != nil || calls != 0 {
		t.Fatalf("exact retry = %v, assembly calls %d", err, calls)
	}
}
