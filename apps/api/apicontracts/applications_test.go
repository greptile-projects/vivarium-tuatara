package apicontracts

import (
	"testing"
	"time"
)

func TestApplicationApprovalCredentialSandboxAndOwnershipRecovery(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	app, err := s.CreateApplication("repo", "contract", "consumer", "demo", "https://consumer.example", 2, []string{"sandbox"}, []string{"read.widgets"})
	if err != nil {
		t.Fatal(err)
	}
	if app.Status != "pending" || len(app.Credentials) != 0 {
		t.Fatalf("unexpected request: %#v", app)
	}
	app, err = s.DecideApplication(app.ID, "producer", "approved", "bounded proof", []string{"read.widgets"}, now.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	app, issued, err := s.IssueApplicationCredential(app.ID, "consumer", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Secret == "" || app.Credentials[0].Hash != "" {
		t.Fatal("secret was not issued once or hash leaked")
	}
	if _, err = s.AuthenticateApplication(app.ID, issued.Secret); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	app, issued2, err := s.IssueApplicationCredential(app.ID, "consumer", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateApplication(app.ID, issued.Secret); err != ErrNotFound {
		t.Fatalf("predecessor remained active: %v", err)
	}
	if _, err = s.AuthenticateApplication(app.ID, issued2.Secret); err != nil {
		t.Fatal(err)
	}
	app, err = s.TransferApplication(app.ID, "consumer", "successor")
	if err != nil {
		t.Fatal(err)
	}
	if app.Status != "pending" || app.OwnerID != "successor" || app.Credentials[1].RevokedAt == nil {
		t.Fatalf("unsafe transfer: %#v", app)
	}
	if _, err = s.AuthenticateApplication(app.ID, issued2.Secret); err != ErrNotFound {
		t.Fatalf("credential survived ownership change: %v", err)
	}
}

func TestApplicationDecisionCannotWidenCapabilities(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	app, _ := s.CreateApplication("r", "c", "u", "n", "", 1, []string{"sandbox"}, []string{"read"})
	if _, err := s.DecideApplication(app.ID, "p", "approved", "why", []string{"write"}, now.Add(time.Hour)); err != ErrInvalid {
		t.Fatalf("widened approval: %v", err)
	}
}

func TestApplicationCredentialEnforcesAndResetsRequestWindow(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	app, _ := s.CreateApplication("r", "c", "u", "n", "", 1, []string{"sandbox"}, []string{"read"})
	app, _ = s.DecideApplication(app.ID, "p", "approved", "why", []string{"read"}, now.Add(24*time.Hour))
	_, issued, _ := s.IssueApplicationCredential(app.ID, "u", 24*time.Hour)
	for i := 0; i < 2; i++ {
		if _, err := s.AuthenticateApplicationRequest(app.ID, issued.Secret, 2, 3600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.AuthenticateApplicationRequest(app.ID, issued.Secret, 2, 3600); err != ErrQuotaExceeded {
		t.Fatalf("quota not enforced: %v", err)
	}
	_, replacement, err := s.IssueApplicationCredential(app.ID, "u", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateApplicationRequest(app.ID, replacement.Secret, 2, 3600); err != ErrQuotaExceeded {
		t.Fatalf("rotation reset application quota: %v", err)
	}
	now = now.Add(time.Hour)
	if _, err := s.AuthenticateApplicationRequest(app.ID, replacement.Secret, 2, 3600); err != nil {
		t.Fatalf("window did not reset: %v", err)
	}
}
