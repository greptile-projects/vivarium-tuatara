package knowledgeanswers

import (
	"errors"
	"testing"
)

func cited(authorType string) Revision {
	uncertainty := ""
	if authorType == "agent" {
		uncertainty = "Only the Linux path was inspected."
	}
	return Revision{
		Summary: "Use the supported client", Body: "Call Client.Connect for release 2.4.",
		AuthorID: "author", AuthorType: authorType,
		Claims: []Claim{{
			Text: "Client.Connect is supported in 2.4.x.", Confidence: "high", Uncertainty: uncertainty,
			Citations: []Citation{{Kind: "symbol", Revision: "0123456789012345678901234567890123456789", Path: "client.go", Symbol: "Client.Connect", StartLine: 12, EndLine: 24, Label: "Client.Connect implementation", ApplicableVersions: []string{"2.4.x"}}},
		}},
	}
}

func TestLifecycleRetainsSupersededEvidenceAndReview(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create(Answer{RepositoryID: "repo", Question: "How do I connect?", Audience: "public"}, cited("human"))
	if err != nil {
		t.Fatal(err)
	}
	first := v.CurrentRevisionID
	v, err = s.Respond("repo", v.ID, "reviewer", first, "challenge", "This does not cover Windows.", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	next := cited("human")
	next.Summary = "Use the portable client"
	v, err = s.Revise("repo", v.ID, v.Version, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Revisions) != 2 || v.Revisions[1].SupersedesRevisionID != first || len(v.Responses) != 1 || v.Responses[0].RevisionID != first {
		t.Fatalf("history lost: %#v", v)
	}
	v, err = s.SetStatus("repo", v.ID, "owner", "verified", v.Version)
	if err != nil || v.Status != "verified" {
		t.Fatalf("verify = %#v, %v", v, err)
	}
	if _, err = s.Respond("repo", v.ID, "reviewer", v.CurrentRevisionID, "endorsement", "Confirmed on Windows.", v.Version-1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale response err = %v", err)
	}
}

func TestAgentRequiresUncertaintyAndEveryClaimRequiresEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	agent := cited("agent")
	agent.Claims[0].Uncertainty = ""
	if _, err := s.Create(Answer{RepositoryID: "repo", Question: "How?", Audience: "participants"}, agent); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing uncertainty err = %v", err)
	}
	human := cited("human")
	human.Claims[0].Citations = nil
	if _, err := s.Create(Answer{RepositoryID: "repo", Question: "How?", Audience: "public"}, human); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing citation err = %v", err)
	}
}

func TestReadAndMutationCannotEscapeRepository(t *testing.T) {
	s, _ := New(t.TempDir())
	left, err := s.Create(Answer{RepositoryID: "repo-a", Question: "Left?", Audience: "participants"}, cited("human"))
	if err != nil {
		t.Fatal(err)
	}
	right, err := s.Create(Answer{RepositoryID: "repo-b", Question: "Private?", Audience: "participants"}, cited("human"))
	if err != nil {
		t.Fatal(err)
	}
	escaped := "../repo-b/" + right.ID
	if _, err = s.Get("repo-a", escaped); !errors.Is(err, ErrNotFound) {
		t.Fatalf("escaped read err = %v", err)
	}
	if _, err = s.SetStatus("repo-a", escaped, "owner-a", "verified", right.Version); !errors.Is(err, ErrNotFound) {
		t.Fatalf("escaped mutation err = %v", err)
	}
	unchanged, err := s.Get("repo-b", right.ID)
	if err != nil || unchanged.Status != "proposed" {
		t.Fatalf("foreign answer changed: %#v, %v", unchanged, err)
	}
	if _, err = s.Get("repo-a", right.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign ID err = %v", err)
	}
	if _, err = s.Get("repo-a", left.ID); err != nil {
		t.Fatalf("local read: %v", err)
	}
}
