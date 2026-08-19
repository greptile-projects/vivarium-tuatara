package auth

import (
	"errors"
	"os"
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

func TestTaskAgentIdentityIsCreatedAtomicallyAndRevocationPersists(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	userID := "0123456789abcdef0123456789abcdef"
	agentID := "abcdef0123456789abcdef0123456789"
	repositoryID := "fedcba9876543210fedcba9876543210"
	issued, err := store.IssueTaskAgentBound(userID, "bounded explorer", agentID, []string{"git:read", "git:write"}, time.Hour, repositoryID, "refs/heads/agent/explore")
	if err != nil {
		t.Fatal(err)
	}
	if issued.AgentID != agentID || issued.RepositoryID != repositoryID {
		t.Fatalf("issued task credential = %#v", issued.Credential)
	}
	if _, err = store.Revoke(userID, issued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Authenticate(issued.Token, "git:write"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("authenticate revoked task credential error = %v", err)
	}
	persisted, err := store.read(issued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RevokedAt == nil || persisted.AgentID != agentID {
		t.Fatalf("persisted revoked task credential = %#v", persisted)
	}
}

func TestBatchRevocationPublishesAllOrNothingAndReconciles(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	userID := "0123456789abcdef0123456789abcdef"
	first, err := store.Issue(userID, Git, "first", []string{"git:read"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Issue(userID, Git, "second", []string{"git:read"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	if err = store.RevokeBatch(userID, []string{first.ID, second.ID}); err == nil {
		t.Fatal("read-only root allowed batch publication")
	}
	if err = os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Authenticate(first.Token, "git:read"); err != nil {
		t.Fatalf("failed batch revoked first credential: %v", err)
	}
	if _, err = store.Authenticate(second.Token, "git:read"); err != nil {
		t.Fatalf("failed batch revoked second credential: %v", err)
	}
	store.afterWrite = func() error { return errors.New("post-rename uncertainty") }
	if err = store.RevokeBatch(userID, []string{first.ID, second.ID}); err != nil {
		t.Fatalf("committed batch was not reconciled: %v", err)
	}
	store.afterWrite = nil
	if _, err = store.Authenticate(first.Token, "git:read"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("first credential remained active: %v", err)
	}
	if _, err = store.Authenticate(second.Token, "git:read"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second credential remained active: %v", err)
	}
	listed, err := store.List(userID)
	if err != nil || len(listed) != 2 || listed[0].RevokedAt == nil || listed[1].RevokedAt == nil {
		t.Fatalf("batch metadata = %#v, %v", listed, err)
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

func TestOrganizationAgentCanUseRepositoryBoundReadAPI(t *testing.T) {
	store, _ := New(t.TempDir())
	issued, err := store.IssueOrganizationAgent(
		"0123456789abcdef0123456789abcdef", "discovery synthesis",
		"11111111111111111111111111111111", "22222222222222222222222222222222",
		"33333333333333333333333333333333", "44444444444444444444444444444444",
		[]string{"repositories:read"}, time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	authenticated, err := store.Authenticate(issued.Token, "repositories:read")
	if err != nil || authenticated.Kind != API || authenticated.AgentID != "33333333333333333333333333333333" || authenticated.RepositoryID != "44444444444444444444444444444444" {
		t.Fatalf("agent API credential = %#v, %v", authenticated, err)
	}
}

func TestIssueReconcilesPostRenameFailure(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.afterWrite = func() error { return errors.New("injected post-rename failure") }
	issued, err := store.Issue("0123456789abcdef0123456789abcdef", API, "committed", []string{"profile:write"}, time.Hour)
	if err != nil {
		t.Fatalf("Issue returned post-publication error: %v", err)
	}
	store.afterWrite = nil
	if _, err := store.Authenticate(issued.Token, "profile:write"); err != nil {
		t.Fatalf("authenticate reconciled credential: %v", err)
	}
}
