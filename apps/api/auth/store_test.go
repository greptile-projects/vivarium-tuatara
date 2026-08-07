package auth

import (
	"errors"
	"testing"
	"time"
)

func TestIssueAuthenticateInspectAndRevoke(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	userID := "0123456789abcdef0123456789abcdef"
	issued, err := store.Issue(userID, API, "automation", []string{"profile:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Token == "" {
		t.Fatalf("issued credential has no token: %#v", issued)
	}
	authenticated, err := store.Authenticate(issued.Token, "profile:write")
	if err != nil {
		t.Fatal(err)
	}
	if authenticated.UserID != userID || authenticated.LastUsedAt == nil {
		t.Fatalf("authenticated = %#v", authenticated)
	}
	listed, err := store.List(userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].LastUsedAt == nil {
		t.Fatalf("listed = %#v", listed)
	}
	if _, err := store.Revoke(userID, issued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(issued.Token, "profile:write"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("authenticate revoked error = %v", err)
	}
}

func TestCredentialKindLimitsScopesAndLifetime(t *testing.T) {
	store, _ := New(t.TempDir())
	userID := "0123456789abcdef0123456789abcdef"
	if _, err := store.Issue(userID, Git, "clone", []string{"git:read"}, 31*24*time.Hour); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long Git credential error = %v", err)
	}
	if _, err := store.Issue(userID, API, "wrong", []string{"git:write"}, time.Hour); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-kind scope error = %v", err)
	}
}
