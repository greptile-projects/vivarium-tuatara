package workflowcomponents

import (
	"errors"
	"strings"
	"testing"
)

func fixture() (Definition, Source) {
	d := Definition{Name: "review-gate", Version: "1.2.0", Summary: "Run the reviewed local gate", Contract: Contract{Inputs: []Field{{Name: "pull", Type: "string", Required: true, Description: "pull id"}}, Outputs: []Field{{Name: "decision", Type: "string", Required: true, Description: "result"}}}, RequestedCapabilities: []Capability{{Name: "checks:read", Reason: "inspect local check results"}}, DataUse: []DataUse{{Classification: "internal", Purpose: "evaluate check state", Retention: "execution history", Destinations: []string{"consumer repository"}}}, Compatibility: Compatibility{WorkflowFormat: 1, Platforms: []string{"vivarium"}}, Tests: []Test{{Name: "contract", CommandSHA256: strings.Repeat("a", 64), Revision: strings.Repeat("b", 40), Outcome: "passed"}}, Support: Support{MaintainerIDs: []string{strings.Repeat("c", 32)}, Policy: "security fixes for 12 months", Contact: "repository issues"}}
	s := Source{RepositoryID: strings.Repeat("d", 32), Revision: strings.Repeat("e", 40), Path: ".vivarium/components/review.json", SHA256: strings.Repeat("f", 64), PackageName: "review-gate", PackageVersion: "1.2.0", PackageSHA256: strings.Repeat("1", 64), Boundary: "package"}
	return d, s
}

func TestImmutablePublicationAndPullReviewedInstallation(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d, source := fixture()
	actor := strings.Repeat("2", 32)
	c, err := s.Publish(d, source, actor)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.Publish(d, source, actor)
	if err != nil || retry.ID != c.ID {
		t.Fatalf("idempotent publish: %#v %v", retry, err)
	}
	d.Summary = "silently changed"
	if _, err = s.Publish(d, source, actor); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected immutable conflict, got %v", err)
	}
	mappings := []Mapping{{Capability: "checks:read", LocalPermission: "pull-checks:read"}}
	accepted := []string{"internal:evaluate check state"}
	install, err := s.Install(strings.Repeat("3", 32), "gate", actor, "pull-1", strings.Repeat("4", 40), c, mappings, map[string]any{"threshold": 2}, accepted, 0)
	if err != nil {
		t.Fatal(err)
	}
	if install.CurrentVersion != 1 || install.Revisions[0].ComponentVersion != "1.2.0" {
		t.Fatalf("unexpected install %#v", install)
	}
	resolved, _, ok := s.Resolve(strings.Repeat("3", 32), "gate@1.2.0")
	if !ok || resolved.ID != c.ID {
		t.Fatal("exact pin did not resolve")
	}
	if _, _, ok = s.Resolve(strings.Repeat("3", 32), "gate@latest"); ok {
		t.Fatal("mutable selector resolved")
	}
	install, err = s.Install(strings.Repeat("3", 32), "gate", actor, "pull-2", strings.Repeat("5", 40), c, mappings, map[string]any{"threshold": 3}, accepted, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(install.Revisions) != 2 || install.Revisions[0].PullID != "pull-1" {
		t.Fatal("update discarded installation history")
	}
}

func TestInstallationRejectsImplicitAuthorityDataAndCredentials(t *testing.T) {
	s, _ := New(t.TempDir())
	d, source := fixture()
	actor := strings.Repeat("2", 32)
	c, _ := s.Publish(d, source, actor)
	base := func(m []Mapping, config map[string]any, data []string) error {
		_, err := s.Install(strings.Repeat("3", 32), "gate", actor, "pull", strings.Repeat("4", 40), c, m, config, data, 0)
		return err
	}
	if err := base(nil, map[string]any{}, []string{"internal:evaluate check state"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing mapping accepted: %v", err)
	}
	if err := base([]Mapping{{Capability: "checks:read", LocalPermission: "admin"}}, map[string]any{}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unaccepted data use accepted: %v", err)
	}
	if err := base([]Mapping{{Capability: "checks:read", LocalPermission: "pull-checks:read"}}, map[string]any{"api_key": "secret"}, []string{"internal:evaluate check state"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("credential accepted: %v", err)
	}
}
