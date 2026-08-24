package restructuringplans

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

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
	p, err := first.Create(completePlan(), "owner", "digest")
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
	one, e := s.Create(p, "owner", "digest")
	if e != nil {
		t.Fatal(e)
	}
	two, e := s.Create(p, "owner", "digest")
	if e != nil || two.ID != one.ID {
		t.Fatalf("retry = %#v %v", two, e)
	}
	if _, e = s.Create(p, "owner", "changed"); !errors.Is(e, ErrConflict) {
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
	if _, e := s.Create(p, "owner", "digest"); !errors.Is(e, ErrInvalid) {
		t.Fatalf("incomplete inventory = %v", e)
	}
	p = completePlan()
	p.Inventory[0].State = "hidden"
	if _, e := s.Create(p, "owner", "digest"); !errors.Is(e, ErrInvalid) {
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
		if _, err := s.Create(p, "owner", randomID()); !errors.Is(err, ErrInvalid) {
			t.Fatalf("unsafe plan accepted: %#v, %v", p, err)
		}
	}
}
