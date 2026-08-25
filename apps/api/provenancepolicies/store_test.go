package provenancepolicies

import (
	"testing"
	"time"
)

func sample() Revision {
	return Revision{Title: "Accepted inputs", Summary: "Terms before intake", OwnerIDs: []string{"owner"}, Rules: []MaterialRule{{Kind: "source", PermittedOrigins: []string{"first_party"}, PermittedLicenses: []string{"UNKNOWN", "MIT"}, ProhibitedLicenses: []string{"MIT"}, ReviewOwnerIDs: []string{"owner"}, DistributionContexts: []string{"commercial"}}}, Exceptions: []Exception{{ID: "legacy", MaterialKinds: []string{"source"}, License: "UNKNOWN", Contexts: []string{"commercial"}, Rationale: "migration", OwnerID: "owner", ExpiresAt: time.Now().Add(time.Hour), FollowUp: "issue:1"}}}
}
func TestVersionedDiagnostics(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.Create("repository", "r", "owner", sample())
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Diagnostics) != 3 {
		t.Fatalf("diagnostics=%#v", p.Diagnostics)
	}
	p, e = s.Revise(p.ID, 1, "owner", sample())
	if e != nil || p.CurrentVersion != 2 || len(p.Revisions) != 2 {
		t.Fatalf("revision %#v %v", p, e)
	}
	if _, e = s.Revise(p.ID, 1, "owner", sample()); e != ErrConflict {
		t.Fatalf("wanted conflict: %v", e)
	}
}

func TestMissingOwnersRemainExplicit(t *testing.T) {
	s, _ := New(t.TempDir())
	r := sample()
	r.OwnerIDs = nil
	r.Rules[0].ReviewOwnerIDs = nil
	p, err := s.Create("organization", "o", "publisher", r)
	if err != nil {
		t.Fatal(err)
	}
	missing := 0
	for _, d := range p.Diagnostics {
		if d.Kind == "missing_owner" && d.AttributedTo == "publisher" {
			missing++
		}
	}
	if missing != 2 {
		t.Fatalf("missing owner diagnostics = %#v", p.Diagnostics)
	}
}
