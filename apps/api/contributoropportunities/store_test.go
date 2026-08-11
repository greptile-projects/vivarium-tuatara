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

func TestInProgressVersionAdmissionRejectsChangedOpportunity(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Publish(sample(), 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Claim(repo, v.ID, newcomer, "launching", time.Hour, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.BeginLaunch(repo, v.ID, newcomer, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	changed := v
	changed.Status = "paused"
	if _, err = s.Publish(changed, v.Version); err != nil {
		t.Fatal(err)
	}
	called := false
	if err = s.WithInProgressVersion(repo, v.ID, v.Version, func(Opportunity) error { called = true; return nil }); !errors.Is(err, ErrConflict) || called {
		t.Fatalf("changed admission = %v, called %v", err, called)
	}
}

func TestAbortLaunchRestoresExactClaimForRetry(t *testing.T) {
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
	claimedVersion := v.Version
	launch, err := s.BeginLaunch(repo, v.ID, newcomer, claimedVersion)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := s.AbortLaunch(repo, v.ID, newcomer, launch.Version)
	if err != nil || restored.Status != "open" || restored.Version != claimedVersion || restored.Claim == nil || restored.Claim.ActorID != newcomer {
		t.Fatalf("restored launch = %#v, %v", restored, err)
	}
	if _, err = s.BeginLaunch(repo, v.ID, newcomer, claimedVersion); err != nil {
		t.Fatalf("identical retry: %v", err)
	}
	if _, err = s.AbortLaunch(repo, v.ID, owner, launch.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("wrong actor abort error = %v", err)
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

func TestCompleteRetainsDeliveredCreditAndIsExactlyRetryable(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	v, err := s.Publish(sample(), 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Claim(repo, v.ID, newcomer, "first contribution", time.Hour, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.BeginLaunch(repo, v.ID, newcomer, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	completion := Completion{
		ContributorID: newcomer, PullRequestID: "33333333333333333333333333333333", ReleaseID: "44444444444444444444444444444444", ReleaseVersion: "v1.1.0", MergeCommitID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Credit: []string{"implementation", "tests"}, Feedback: "Clear scope and responsive review follow-up.", SupportEffort: SupportEffort{SetupAttempts: 2, MentorGuidanceItems: 1, AgentAssistanceItems: 1},
		Readiness: Readiness{ReadyForNext: true, SkillsRecognized: []string{"Go", "testing"}, Note: "Ready for another low-risk parser task."}, RecordedBy: owner,
	}
	done, err := s.Complete(repo, v.ID, v.Version, completion)
	if err != nil || done.Status != "completed" || done.Completion == nil || done.Completion.RecordedAt != now || done.Completion.SupportEffort.MentorGuidanceItems != 1 {
		t.Fatalf("completion = %#v, %v", done, err)
	}
	retry, err := s.Complete(repo, v.ID, v.Version, completion)
	if err != nil || retry.Version != done.Version {
		t.Fatalf("exact retry = %#v, %v", retry, err)
	}
	mutations := map[string]func(*Completion){
		"feedback":        func(v *Completion) { v.Feedback = "Different assessment" },
		"credit":          func(v *Completion) { v.Credit = []string{"implementation"} },
		"skills":          func(v *Completion) { v.Readiness.SkillsRecognized = []string{"Go"} },
		"release version": func(v *Completion) { v.ReleaseVersion = "v1.1.1" },
		"support effort":  func(v *Completion) { v.SupportEffort.MentorGuidanceItems++ },
		"recorded by":     func(v *Completion) { v.RecordedBy = "55555555555555555555555555555555" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := completion
			changed.Credit = append([]string(nil), completion.Credit...)
			changed.Readiness.SkillsRecognized = append([]string(nil), completion.Readiness.SkillsRecognized...)
			mutate(&changed)
			if _, retryErr := s.Complete(repo, v.ID, v.Version, changed); !errors.Is(retryErr, ErrConflict) {
				t.Fatalf("changed retry = %v", retryErr)
			}
		})
	}
}
