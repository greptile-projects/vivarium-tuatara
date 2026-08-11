package contributoropportunities

import (
	"errors"
	"testing"
	"time"
)

const repo = "0123456789abcdef0123456789abcdef"
const owner = "abcdef0123456789abcdef0123456789"
const source = "11111111111111111111111111111111"
const newcomer = "22222222222222222222222222222222"

func sample() Opportunity {
	return Opportunity{RepositoryID: repo, PublishedBy: owner, Source: Source{Kind: "issue", ID: source}, Title: "Add parser coverage", ExpectedOutcome: "Malformed inputs have regression tests.", Scope: "Parser tests only; no production refactor.", RequiredSkills: []string{"Go", "testing"}, Interests: []string{"reliability"}, Risk: "low", EstimatedMinutes: 120, AgentAssistance: true, Mentors: []Mentor{{UserID: owner}}, Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: "open"}
}
func TestMatchClaimExpiryAndRelease(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	v, err := s.Publish(sample(), 0)
	if err != nil {
		t.Fatal(err)
	}
	m := MatchAll([]Opportunity{v}, Profile{Skills: []string{"Go", "testing"}, Interests: []string{"reliability"}, AvailableMinutes: 180, MaximumRisk: "low", AgentAssistance: true}, now)
	if len(m) != 1 || !m[0].Ready || m[0].Score != 100 || len(m[0].Reasons) < 4 {
		t.Fatalf("match = %#v", m)
	}
	v, err = s.Claim(repo, v.ID, newcomer, "Starting with tests", 48*time.Hour, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(repo, v.ID, owner, "duplicate", time.Hour, v.Version); !errors.Is(err, ErrClaimed) {
		t.Fatalf("claim error = %v", err)
	}
	if _, err = s.Release(repo, v.ID, owner, false, v.Version); !errors.Is(err, ErrClaimed) {
		t.Fatalf("release error = %v", err)
	}
	v, err = s.Release(repo, v.ID, newcomer, false, v.Version)
	if err != nil || v.Claim.ReleasedAt == nil {
		t.Fatalf("release = %#v, %v", v, err)
	}
	v, err = s.Claim(repo, v.ID, newcomer, "again", time.Hour, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	got, err := s.Get(repo, v.ID)
	if err != nil || got.Claim != nil {
		t.Fatalf("expired claim = %#v, %v", got.Claim, err)
	}
}

func TestBeginLaunchRevalidatesAndConsumesClaim(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	v, err := s.Publish(sample(), 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Claim(repo, v.ID, newcomer, "launching", time.Hour, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if _, err = s.BeginLaunch(repo, v.ID, newcomer, v.Version); !errors.Is(err, ErrClaimed) {
		t.Fatalf("expired launch error = %v", err)
	}
	now = now.Add(-time.Minute)
	v, err = s.BeginLaunch(repo, v.ID, newcomer, v.Version)
	if err != nil || v.Status != "in_progress" {
		t.Fatalf("launch = %#v, %v", v, err)
	}
	if _, err = s.BeginLaunch(repo, v.ID, newcomer, v.Version); !errors.Is(err, ErrClaimed) {
		t.Fatalf("duplicate launch error = %v", err)
	}
}
func TestMatchExplainsConstraints(t *testing.T) {
	v := sample()
	m := MatchAll([]Opportunity{v}, Profile{Skills: []string{"Go"}, AvailableMinutes: 30, MaximumRisk: "low"}, time.Now())[0]
	if m.Ready || len(m.Gaps) < 2 {
		t.Fatalf("match = %#v", m)
	}
}

func TestRepublishClearsExactVersionClaim(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	v, err := s.Publish(sample(), 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Claim(repo, v.ID, newcomer, "starting", time.Hour, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	replacement := sample()
	replacement.ID = v.ID
	replacement.Scope = "A newly bounded replacement scope."
	v, err = s.Publish(replacement, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if v.Claim != nil {
		t.Fatalf("republished claim = %#v", v.Claim)
	}
	if _, err = s.Claim(repo, v.ID, owner, "new version", time.Hour, v.Version); err != nil {
		t.Fatalf("claim replacement: %v", err)
	}
}

func TestListProjectsOnlyActiveClaims(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	v, err := s.Publish(sample(), 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Claim(repo, v.ID, newcomer, "starting", time.Hour, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	items, err := s.List(repo)
	if err != nil || len(items) != 1 || items[0].Claim != nil {
		t.Fatalf("expired list = %#v, %v", items, err)
	}
	v, err = s.Claim(repo, v.ID, newcomer, "again", time.Hour, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Release(repo, v.ID, newcomer, false, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	items, err = s.List(repo)
	if err != nil || items[0].Claim != nil {
		t.Fatalf("released list = %#v, %v", items, err)
	}
}
