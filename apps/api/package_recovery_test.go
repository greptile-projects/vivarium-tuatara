package main

import (
	"strings"
	"testing"

	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
)

func TestPromotionDependencyPolicyFailsClosed(t *testing.T) {
	if err := verifyPromotionDependencies(nil, strings.Repeat("a", 32), strings.Repeat("b", 40)); err == nil {
		t.Fatal("missing package store passed promotion policy")
	}
	store, err := packages.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repositoryID, commitID := strings.Repeat("a", 32), strings.Repeat("b", 40)
	if err := verifyPromotionDependencies(store, repositoryID, commitID); err == nil {
		t.Fatal("missing inventory passed promotion policy")
	}
	_, err = store.RecordInventory(packages.Inventory{RepositoryID: repositoryID, CommitID: commitID, RecordedBy: strings.Repeat("c", 32), Entries: []packages.InventoryEntry{{Name: "missing-kit", Version: "1.0.0", State: "resolved", Paths: []string{"missing-kit"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyPromotionDependencies(store, repositoryID, commitID); err == nil {
		t.Fatal("missing package metadata passed promotion policy")
	}
}

func TestRenamedRecoveryReplacementRewritesManifestAndLock(t *testing.T) {
	manifest := packages.InventoryConfig{Version: 1,
		Dependencies: []packages.ManifestDependency{{Name: "unsafe-kit", Constraint: "^1.0.0"}, {Name: "safe-kit", Constraint: "^1.5.0"}, {Name: "other", Constraint: "^3.0.0"}},
		Lock:         []packages.LockEntry{{Name: "unsafe-kit", Version: "1.0.0"}, {Name: "safe-kit", Version: "1.5.0"}, {Name: "other", Version: "3.1.0"}},
	}
	result := replacePackageInManifest(manifest, "unsafe-kit", "safe-kit", "2.0.0")
	if len(result.Dependencies) != 2 || result.Dependencies[0].Name != "safe-kit" || result.Dependencies[0].Constraint != "^2.0.0" || len(result.Lock) != 2 || result.Lock[0].Name != "safe-kit" || result.Lock[0].Version != "2.0.0" {
		t.Fatalf("manifest = %#v", result)
	}
}
