package auth

import (
	"strings"
	"testing"
	"time"
)

func TestExpireAtRetainsCredentialThroughOverlap(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	issued, err := s.Issue(strings.Repeat("a", 32), API, "extension", []string{"extensions:contribute"}, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	deadline := now.Add(2 * time.Hour)
	if _, err = s.ExpireAt(issued.UserID, issued.ID, deadline); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(issued.Token, "extensions:contribute"); err != nil {
		t.Fatalf("credential rejected during overlap: %v", err)
	}
	now = deadline
	if _, err = s.Authenticate(issued.Token, "extensions:contribute"); err == nil {
		t.Fatal("credential remained usable after overlap")
	}
}
