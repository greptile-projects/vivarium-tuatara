package organizations

import (
	"errors"
	"testing"
)

func TestRemoveMemberCleanupFailurePreventsMembershipCommit(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := "0123456789abcdef0123456789abcdef"
	member := "abcdef0123456789abcdef0123456789"
	organization, err := store.Create("Runtime", "runtime", "", owner)
	if err != nil {
		t.Fatal(err)
	}
	organization, err = store.Invite(organization.ID, owner, member)
	if err != nil {
		t.Fatal(err)
	}
	organization, err = store.AcceptInvitation(organization.ID, organization.Invitations[0].ID, member)
	if err != nil {
		t.Fatal(err)
	}
	cleanupFailure := errors.New("credential store unavailable")
	if _, err = store.RemoveMember(organization.ID, owner, member, func(Organization) error { return cleanupFailure }); !errors.Is(err, cleanupFailure) {
		t.Fatalf("RemoveMember error = %v", err)
	}
	stored, err := store.Get(organization.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !HasRole(stored, member, "member") {
		t.Fatal("cleanup failure committed member removal")
	}
	for _, event := range stored.Events {
		if event.Action == "member.removed" {
			t.Fatal("cleanup failure published removal audit")
		}
	}
}
