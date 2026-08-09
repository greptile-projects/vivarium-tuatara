package relationships

import "testing"

func TestCompatibilityConstraints(t *testing.T) {
	for _, test := range []struct {
		version, constraint string
		want                bool
	}{
		{"v1.4.2", ">=v1.0.0 <v2.0.0", true},
		{"v2.0.0", ">=v1.0.0 <v2.0.0", false},
		{"v3.2.1", "=v3.2.1", true},
		{"v3.2.1", "*", true},
	} {
		if got := Satisfies(test.version, test.constraint); got != test.want {
			t.Fatalf("Satisfies(%q, %q) = %v", test.version, test.constraint, got)
		}
	}
}

func TestStoreRetainsImmutableEvidence(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateInterface(Interface{RepositoryID: "11111111111111111111111111111111", Name: " events ", Version: "v1.2.3", ReleaseID: "22222222222222222222222222222222", CommitID: "3333333333333333333333333333333333333333", PublishedBy: "44444444444444444444444444444444"})
	if err != nil {
		t.Fatal(err)
	}
	values, err := store.ListInterfaces(created.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.PublishedAt.IsZero() || len(values) != 1 || values[0].ID != created.ID || values[0].Name != "events" {
		t.Fatalf("created/listed = %#v / %#v", created, values)
	}
}
