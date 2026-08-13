package feedback

import "testing"

func TestFeedbackRequiresRedactedEvidenceAndRetainsHistory(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actor := "11111111111111111111111111111111"
	in := Item{RepositoryID: "22222222222222222222222222222222", Target: Target{Kind: "project", Label: "Checkout"}, Need: "Understand failed payments", DesiredOutcome: "Clear recovery guidance", Frequency: "weekly", Impact: "Customers abandon purchases", Audience: "project", IdentityVisibility: "maintainers", ContactPreference: "discussion", Evidence: []Evidence{{Name: "redacted transcript", Kind: "text", Summary: "Identifiers removed", Visibility: "maintainers", Redacted: true}}}
	out, err := s.Create(in, actor)
	if err != nil {
		t.Fatal(err)
	}
	if out.ReporterID != actor || len(out.History) != 1 || out.Evidence[0].ID == "" {
		t.Fatalf("incomplete record: %#v", out)
	}
	in.Evidence[0].Redacted = false
	if _, err = s.Create(in, actor); err != ErrInvalid {
		t.Fatalf("unredacted evidence error = %v", err)
	}
}

func TestFeedbackDiscussionIsAppendOnly(t *testing.T) {
	s, _ := New(t.TempDir())
	actor := "11111111111111111111111111111111"
	x, err := s.Create(Item{RepositoryID: "22222222222222222222222222222222", Target: Target{Kind: "journey", ResourceID: "onboarding", Label: "Onboarding"}, Need: "Need examples", DesiredOutcome: "First success sooner", Frequency: "daily", Impact: "New users stop", Audience: "project", IdentityVisibility: "reporter_only", ContactPreference: "none"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	x, err = s.AddComment(x.ID, "33333333333333333333333333333333", "Which step blocks you?", "maintainer")
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Comments) != 1 || len(x.History) != 2 || x.Comments[0].AuthorRole != "maintainer" {
		t.Fatalf("discussion = %#v", x)
	}
}

func TestFeedbackRejectsContactWithoutDirectConsent(t *testing.T) {
	s, _ := New(t.TempDir())
	in := Item{RepositoryID: "22222222222222222222222222222222", Target: Target{Kind: "project", Label: "Checkout"}, Need: "Need help", DesiredOutcome: "Recover", Frequency: "weekly", Impact: "Abandonment", Audience: "project", IdentityVisibility: "maintainers", ContactPreference: "discussion", Contact: "reporter@example.test"}
	if _, err := s.Create(in, "11111111111111111111111111111111"); err != ErrInvalid {
		t.Fatalf("non-direct contact error = %v", err)
	}
	in.ContactPreference = "direct"
	if _, err := s.Create(in, "11111111111111111111111111111111"); err != nil {
		t.Fatalf("direct contact rejected: %v", err)
	}
}
