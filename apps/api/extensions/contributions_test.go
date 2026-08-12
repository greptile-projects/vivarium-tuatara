package extensions

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func testID(character string) string { return character + "0000000000000000000000000000000" }

func TestContributionIsRevisionBoundIdempotentAndAttributed(t *testing.T) {
	s, _ := New(t.TempDir())
	installation := Installation{ID: testID("1"), ExtensionID: testID("2"), ExtensionName: "Review lens", RepositoryIDs: []string{testID("3")}, EffectiveAccess: []Permission{{Resource: "pull_requests", Actions: []string{"write"}}}, Status: "active"}
	if err := writeAtomic(filepath.Join(s.root, "installation-"+installation.ID+".json"), installation); err != nil {
		t.Fatal(err)
	}
	in := ContributionInput{IdempotencyKey: "delivery-123", RepositoryID: testID("3"), ResourceType: "pull_requests", ResourceID: testID("4"), Revision: string(make([]byte, 40)), Kind: "check", State: "success", Title: "Dependency policy", Body: "All declared dependencies are supported.", Actions: []Action{{ID: "explain", Label: "Explain finding", Description: "Request details", Inputs: []ActionInput{{Name: "focus", Label: "Focus", Required: true}}, Effects: []string{"Queues an attributed explanation request; does not change the pull."}}}}
	first, err := s.PublishContribution(installation, in)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.PublishContribution(installation, in)
	if err != nil || retry.ID != first.ID || !first.Trusted || first.ExtensionID != installation.ExtensionID {
		t.Fatalf("retry=%#v err=%v", retry, err)
	}
	changed := in
	changed.Title = "Different"
	if _, err = s.PublishContribution(installation, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict=%v", err)
	}
	invoked, err := s.Invoke(in.RepositoryID, in.ResourceType, in.ResourceID, first.ID, "explain", testID("5"), map[string]string{"focus": "licenses", "undeclared": "ignored"})
	if err != nil || len(invoked.Invocations) != 1 || invoked.Invocations[0].ActorID != testID("5") || invoked.Invocations[0].Inputs["undeclared"] != "" || len(invoked.Invocations[0].PreviewedEffects) != 1 {
		t.Fatalf("invocation=%#v err=%v", invoked, err)
	}
}

func TestContributionRequiresLiveExactAuthority(t *testing.T) {
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return time.Unix(1, 0).UTC() }
	in := ContributionInput{IdempotencyKey: "delivery-123", RepositoryID: testID("3"), ResourceType: "pull_requests", ResourceID: testID("4"), Revision: string(make([]byte, 40)), Kind: "status", Title: "Status"}
	for _, installation := range []Installation{{ID: testID("1"), ExtensionID: testID("2"), RepositoryIDs: []string{testID("3")}, EffectiveAccess: []Permission{{Resource: "pull_requests", Actions: []string{"read"}}}, Status: "active"}, {ID: testID("1"), ExtensionID: testID("2"), RepositoryIDs: []string{testID("3")}, EffectiveAccess: []Permission{{Resource: "pull_requests", Actions: []string{"write"}}}, Status: "suspended"}} {
		if _, err := s.PublishContribution(installation, in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("authority accepted: %v", err)
		}
	}
}
