package main

import (
	"testing"

	packageversions "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
)

func TestVerifiedAdoptionUpdateRequiresExactInstalledProviderRelease(t *testing.T) {
	versions := []packageversions.Version{{Name: "relay", Version: "2.1.0", RepositoryID: "provider", ReleaseID: "release", SourceCommit: "1111111111111111111111111111111111111111"}}
	inventory := packageversions.Inventory{Entries: []packageversions.InventoryEntry{{Name: "relay", Version: "2.1.0", Direct: true, State: "resolved"}}}
	name, version, ok := exactAdoptionPackage(versions, inventory, "provider", "release", "1111111111111111111111111111111111111111")
	if !ok || name != "relay" || version != "2.1.0" {
		t.Fatalf("exact provider package was not proven: %q %q %v", name, version, ok)
	}
	inventory.Entries[0].Version = "2.0.0"
	if _, _, ok = exactAdoptionPackage(versions, inventory, "provider", "release", "1111111111111111111111111111111111111111"); ok {
		t.Fatal("unrelated installed package version was accepted as the upstream update")
	}
	if _, _, ok = exactAdoptionPackage(versions, packageversions.Inventory{Entries: []packageversions.InventoryEntry{{Name: "relay", Version: "2.1.0", Direct: false, State: "resolved"}}}, "provider", "release", "1111111111111111111111111111111111111111"); ok {
		t.Fatal("transitive package evidence was accepted for a direct adoption")
	}
}

func TestVerifiedAdoptionUpdateMustCoverEveryLocalPatchPath(t *testing.T) {
	local := []pullrequests.FileChange{{Path: "src/workaround.go"}, {Path: "tests/workaround_test.go"}}
	update := []pullrequests.FileChange{{Path: "src/workaround.go"}, {Path: "go.mod"}}
	if paths, ok := adoptionPatchCoverage(local, update); ok || len(paths) != 0 {
		t.Fatalf("partial local-patch replacement was accepted: %v %v", paths, ok)
	}
	update = append(update, pullrequests.FileChange{Path: "tests/workaround_test.go"})
	paths, ok := adoptionPatchCoverage(local, update)
	if !ok || len(paths) != 2 {
		t.Fatalf("complete local-patch coverage was rejected: %v %v", paths, ok)
	}
}
