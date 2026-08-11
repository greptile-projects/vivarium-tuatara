// Package contributoropportunities retains bounded work advertised to newcomers
// and coordination-only claims that never confer repository authority.
package contributoropportunities

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("contribution opportunity not found")
	ErrInvalid  = errors.New("invalid contribution opportunity")
	ErrConflict = errors.New("contribution opportunity changed")
	ErrClaimed  = errors.New("contribution opportunity is already claimed")
)

// BeginLaunch atomically revalidates and consumes a live exact-version claim
// immediately before the caller creates contributor resources.
func (s *Store) BeginLaunch(repositoryID, id, actor string, expected int) (Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.read(repositoryID)
	if err != nil {
		return Opportunity{}, err
	}
	for _, v := range items {
		if v.ID != id {
			continue
		}
		if v.Version != expected {
			return Opportunity{}, ErrConflict
		}
		now := s.now().UTC()
		v.Claim = activeClaim(v.Claim, now)
		if v.Status != "open" || v.Claim == nil || v.Claim.ActorID != actor {
			return Opportunity{}, ErrClaimed
		}
		v.Status = "in_progress"
		v.Version++
		v.UpdatedAt = now
		if err := s.write(v); err != nil {
			return Opportunity{}, err
		}
		return v, nil
	}
	return Opportunity{}, ErrNotFound
}

// AbortLaunch compensates an admission only when the first resource creation
// failed. Restoring the prior exact version makes the original request safely
// retryable; callers must not use this after any fork or workspace is retained.
func (s *Store) AbortLaunch(repositoryID, id, actor string, launchVersion int) (Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.read(repositoryID)
	if err != nil {
		return Opportunity{}, err
	}
	for _, v := range items {
		if v.ID != id {
			continue
		}
		if v.Version != launchVersion || v.Status != "in_progress" || v.Claim == nil || v.Claim.ActorID != actor {
			return Opportunity{}, ErrConflict
		}
		v.Status = "open"
		v.Version--
		if err := s.write(v); err != nil {
			return Opportunity{}, err
		}
		return v, nil
	}
	return Opportunity{}, ErrNotFound
}

type Source struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
}
type Mentor struct {
	UserID string `json:"user_id"`
	Note   string `json:"note,omitempty"`
}
type Opportunity struct {
	ID               string      `json:"id"`
	RepositoryID     string      `json:"repository_id"`
	Version          int         `json:"version"`
	Source           Source      `json:"source"`
	Title            string      `json:"title"`
	ExpectedOutcome  string      `json:"expected_outcome"`
	Scope            string      `json:"scope"`
	RequiredSkills   []string    `json:"required_skills"`
	Interests        []string    `json:"interests"`
	DependencyIDs    []string    `json:"dependency_ids"`
	Risk             string      `json:"risk"`
	EstimatedMinutes int         `json:"estimated_minutes"`
	AgentAssistance  bool        `json:"agent_assistance"`
	Mentors          []Mentor    `json:"mentors"`
	Revision         string      `json:"revision"`
	Status           string      `json:"status"`
	PublishedBy      string      `json:"published_by"`
	PublishedAt      time.Time   `json:"published_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	Claim            *Claim      `json:"claim,omitempty"`
	Completion       *Completion `json:"completion,omitempty"`
}
type Claim struct {
	ID         string     `json:"id"`
	ActorID    string     `json:"actor_id"`
	Note       string     `json:"note,omitempty"`
	ClaimedAt  time.Time  `json:"claimed_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	ReleasedBy string     `json:"released_by,omitempty"`
}
type SupportEffort struct {
	SetupAttempts        int `json:"setup_attempts"`
	MentorGuidanceItems  int `json:"mentor_guidance_items"`
	AgentAssistanceItems int `json:"agent_assistance_items"`
}
type Readiness struct {
	ReadyForNext      bool     `json:"ready_for_next"`
	SkillsRecognized  []string `json:"skills_recognized"`
	NextOpportunityID string   `json:"next_opportunity_id,omitempty"`
	Note              string   `json:"note"`
}
type Completion struct {
	ContributorID  string        `json:"contributor_id"`
	PullRequestID  string        `json:"pull_request_id"`
	ReleaseID      string        `json:"release_id"`
	ReleaseVersion string        `json:"release_version"`
	MergeCommitID  string        `json:"merge_commit_id"`
	Credit         []string      `json:"credit"`
	Feedback       string        `json:"feedback"`
	SupportEffort  SupportEffort `json:"support_effort"`
	Readiness      Readiness     `json:"readiness"`
	RecordedBy     string        `json:"recorded_by"`
	RecordedAt     time.Time     `json:"recorded_at"`
}
type Profile struct {
	Skills           []string `json:"skills"`
	Interests        []string `json:"interests"`
	AvailableMinutes int      `json:"available_minutes"`
	MaximumRisk      string   `json:"maximum_risk"`
	AgentAssistance  bool     `json:"agent_assistance"`
}
type Match struct {
	Opportunity Opportunity `json:"opportunity"`
	Score       int         `json:"score"`
	Reasons     []string    `json:"reasons"`
	Gaps        []string    `json:"gaps"`
	Ready       bool        `json:"ready"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

// WithInProgressVersion prevents opportunity publication or state changes
// from interleaving with an admitted guided pull publication.
func (s *Store) WithInProgressVersion(repositoryID, id string, expectedVersion int, fn func(Opportunity) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.read(repositoryID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID != id {
			continue
		}
		if item.Version != expectedVersion || item.Status != "in_progress" {
			return ErrConflict
		}
		return fn(item)
	}
	return ErrNotFound
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: time.Now}, nil
}

func (s *Store) Publish(v Opportunity, expected int) (Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.read(v.RepositoryID)
	if err != nil {
		return Opportunity{}, err
	}
	var prior *Opportunity
	if v.ID != "" {
		for i := range items {
			if items[i].ID == v.ID {
				prior = &items[i]
				break
			}
		}
		if prior == nil {
			return Opportunity{}, ErrNotFound
		}
		if prior.Version != expected {
			return Opportunity{}, ErrConflict
		}
	} else if expected != 0 {
		return Opportunity{}, ErrConflict
	}
	if prior != nil && prior.Completion != nil {
		return Opportunity{}, ErrInvalid
	}
	if !valid(v) {
		return Opportunity{}, ErrInvalid
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	if prior == nil {
		v.ID = newID()
		v.Version = 1
		v.PublishedAt = now
	} else {
		v.ID = prior.ID
		v.Version = prior.Version + 1
		v.PublishedAt = prior.PublishedAt
		// Claims bind to one exact opportunity version. Republishing creates a
		// new agreement about the work, so it must be explicitly claimed again.
		v.Claim = nil
	}
	v.UpdatedAt = now
	if v.Status == "" {
		v.Status = "open"
	}
	if err = s.write(v); err != nil {
		return Opportunity{}, err
	}
	return v, nil
}
func (s *Store) List(repositoryID string) ([]Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.read(repositoryID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	for i := range items {
		items[i].Claim = activeClaim(items[i].Claim, now)
	}
	return items, nil
}
func (s *Store) Get(repositoryID, id string) (Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.read(repositoryID)
	if err != nil {
		return Opportunity{}, err
	}
	for _, v := range items {
		if v.ID == id {
			v.Claim = activeClaim(v.Claim, s.now())
			return v, nil
		}
	}
	return Opportunity{}, ErrNotFound
}
func (s *Store) Claim(repositoryID, id, actor, note string, ttl time.Duration, expected int) (Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.read(repositoryID)
	if err != nil {
		return Opportunity{}, err
	}
	for _, v := range items {
		if v.ID != id {
			continue
		}
		if v.Version != expected {
			return Opportunity{}, ErrConflict
		}
		now := s.now().UTC()
		v.Claim = activeClaim(v.Claim, now)
		if v.Status != "open" {
			return Opportunity{}, ErrInvalid
		}
		if v.Claim != nil {
			return Opportunity{}, ErrClaimed
		}
		if ttl < time.Hour || ttl > 14*24*time.Hour {
			return Opportunity{}, ErrInvalid
		}
		v.Claim = &Claim{ID: newID(), ActorID: actor, Note: strings.TrimSpace(note), ClaimedAt: now, ExpiresAt: now.Add(ttl)}
		v.Version++
		v.UpdatedAt = now
		if err = s.write(v); err != nil {
			return Opportunity{}, err
		}
		return v, nil
	}
	return Opportunity{}, ErrNotFound
}
func (s *Store) Release(repositoryID, id, actor string, owner bool, expected int) (Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.read(repositoryID)
	if err != nil {
		return Opportunity{}, err
	}
	for _, v := range items {
		if v.ID != id {
			continue
		}
		if v.Version != expected {
			return Opportunity{}, ErrConflict
		}
		now := s.now().UTC()
		v.Claim = activeClaim(v.Claim, now)
		if v.Claim == nil {
			return Opportunity{}, ErrInvalid
		}
		if !owner && v.Claim.ActorID != actor {
			return Opportunity{}, ErrClaimed
		}
		v.Claim.ReleasedAt = &now
		v.Claim.ReleasedBy = actor
		v.Version++
		v.UpdatedAt = now
		if err = s.write(v); err != nil {
			return Opportunity{}, err
		}
		return v, nil
	}
	return Opportunity{}, ErrNotFound
}

// Complete closes an in-progress reservation with delivery evidence. The
// caller validates the ordinary pull and release before entering this atomic
// exact-version transition.
func (s *Store) Complete(repositoryID, id string, expected int, completion Completion) (Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	completion = normalizeCompletion(completion)
	items, err := s.read(repositoryID)
	if err != nil {
		return Opportunity{}, err
	}
	for _, v := range items {
		if v.ID != id {
			continue
		}
		if v.Completion != nil {
			if sameCompletion(*v.Completion, completion) {
				return v, nil
			}
			return Opportunity{}, ErrConflict
		}
		if v.Version != expected {
			return Opportunity{}, ErrConflict
		}
		if v.Status != "in_progress" || v.Claim == nil || v.Claim.ActorID != completion.ContributorID || !validCompletion(completion) {
			return Opportunity{}, ErrInvalid
		}
		completion.RecordedAt = s.now().UTC().Truncate(time.Microsecond)
		v.Status, v.Completion, v.Version, v.UpdatedAt = "completed", &completion, v.Version+1, completion.RecordedAt
		if err = s.write(v); err != nil {
			return Opportunity{}, err
		}
		return v, nil
	}
	return Opportunity{}, ErrNotFound
}

func validCompletion(v Completion) bool {
	return validID(v.ContributorID) && validID(v.PullRequestID) && validID(v.ReleaseID) && validID(v.RecordedBy) &&
		len(v.MergeCommitID) == 40 && strings.TrimSpace(v.ReleaseVersion) != "" && strings.TrimSpace(v.Feedback) != "" && len(v.Feedback) <= 4000 &&
		strings.TrimSpace(v.Readiness.Note) != "" && len(v.Readiness.Note) <= 2000 && len(v.Credit) > 0 && len(v.Credit) <= 20 && len(v.Readiness.SkillsRecognized) <= 20 &&
		v.SupportEffort.SetupAttempts >= 0 && v.SupportEffort.MentorGuidanceItems >= 0 && v.SupportEffort.AgentAssistanceItems >= 0 &&
		(v.Readiness.NextOpportunityID == "" || validID(v.Readiness.NextOpportunityID))
}
func sameCompletion(a, b Completion) bool {
	return a.ContributorID == b.ContributorID && a.PullRequestID == b.PullRequestID && a.ReleaseID == b.ReleaseID && a.ReleaseVersion == b.ReleaseVersion && a.MergeCommitID == b.MergeCommitID &&
		slices.Equal(a.Credit, b.Credit) && a.Feedback == b.Feedback && a.SupportEffort == b.SupportEffort &&
		a.Readiness.ReadyForNext == b.Readiness.ReadyForNext && slices.Equal(a.Readiness.SkillsRecognized, b.Readiness.SkillsRecognized) &&
		a.Readiness.NextOpportunityID == b.Readiness.NextOpportunityID && a.Readiness.Note == b.Readiness.Note && a.RecordedBy == b.RecordedBy
}
func normalizeCompletion(v Completion) Completion {
	v.ReleaseVersion = strings.TrimSpace(v.ReleaseVersion)
	v.Feedback = strings.TrimSpace(v.Feedback)
	v.Readiness.Note = strings.TrimSpace(v.Readiness.Note)
	for i := range v.Credit {
		v.Credit[i] = strings.TrimSpace(v.Credit[i])
	}
	for i := range v.Readiness.SkillsRecognized {
		v.Readiness.SkillsRecognized[i] = strings.TrimSpace(v.Readiness.SkillsRecognized[i])
	}
	return v
}
func MatchAll(items []Opportunity, p Profile, now time.Time) []Match {
	out := []Match{}
	completed := map[string]bool{}
	for _, v := range items {
		completed[v.ID] = v.Status == "completed"
	}
	for _, v := range items {
		v.Claim = activeClaim(v.Claim, now)
		if v.Status != "open" {
			continue
		}
		m := Match{Opportunity: v, Ready: v.Claim == nil, Reasons: []string{}, Gaps: []string{}}
		for _, dependency := range v.DependencyIDs {
			if !completed[dependency] {
				m.Gaps = append(m.Gaps, "Waiting on dependency "+dependency+".")
				m.Ready = false
			}
		}
		skills := set(p.Skills)
		interests := set(p.Interests)
		matched := 0
		for _, x := range v.RequiredSkills {
			if skills[strings.ToLower(x)] {
				matched++
			} else {
				m.Gaps = append(m.Gaps, "Missing skill: "+x)
			}
		}
		if len(v.RequiredSkills) == 0 || matched == len(v.RequiredSkills) {
			m.Score += 40
			m.Reasons = append(m.Reasons, "Your skills cover the stated requirements.")
		} else {
			m.Score += 20 * matched / max(1, len(v.RequiredSkills))
		}
		for _, x := range v.Interests {
			if interests[strings.ToLower(x)] {
				m.Score += 15
				m.Reasons = append(m.Reasons, "Matches your interest in "+x+".")
				break
			}
		}
		if p.AvailableMinutes >= v.EstimatedMinutes {
			m.Score += 20
			m.Reasons = append(m.Reasons, "Fits within your available time.")
		} else {
			m.Gaps = append(m.Gaps, "Needs more time than you listed.")
			m.Ready = false
		}
		if riskRank(v.Risk) <= riskRank(p.MaximumRisk) {
			m.Score += 15
			m.Reasons = append(m.Reasons, "Risk is within your comfort level.")
		} else {
			m.Gaps = append(m.Gaps, "Risk exceeds your preference.")
			m.Ready = false
		}
		if p.AgentAssistance && v.AgentAssistance {
			m.Score += 10
			m.Reasons = append(m.Reasons, "Agent assistance is available.")
		}
		if v.Claim != nil {
			m.Gaps = append(m.Gaps, "Already reserved until "+v.Claim.ExpiresAt.Format(time.RFC3339))
			m.Ready = false
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Ready != out[j].Ready {
			return out[i].Ready
		}
		return out[i].Score > out[j].Score
	})
	return out
}
func activeClaim(c *Claim, now time.Time) *Claim {
	if c == nil || c.ReleasedAt != nil || !c.ExpiresAt.After(now) {
		return nil
	}
	return c
}
func valid(v Opportunity) bool {
	if !(validID(v.RepositoryID) && validID(v.PublishedBy) && strings.TrimSpace(v.Title) != "" && strings.TrimSpace(v.ExpectedOutcome) != "" && strings.TrimSpace(v.Scope) != "" && validID(v.Source.ID) && one(v.Source.Kind, "issue", "proposal", "stewardship", "task") && one(v.Risk, "low", "medium", "high") && v.EstimatedMinutes >= 15 && v.EstimatedMinutes <= 10080 && strings.TrimSpace(v.Revision) != "" && one(defaultStatus(v.Status), "open", "in_progress", "paused", "completed") && len(v.RequiredSkills) <= 20 && len(v.Interests) <= 20 && len(v.DependencyIDs) <= 50 && len(v.Mentors) <= 20) {
		return false
	}
	for _, mentor := range v.Mentors {
		if !validID(mentor.UserID) {
			return false
		}
	}
	return true
}
func defaultStatus(v string) string {
	if v == "" {
		return "open"
	}
	return v
}
func one(v string, x ...string) bool {
	for _, a := range x {
		if v == a {
			return true
		}
	}
	return false
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil
}
func set(v []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range v {
		m[strings.ToLower(strings.TrimSpace(x))] = true
	}
	return m
}
func riskRank(v string) int {
	switch v {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	}
	return 0
}
func (s *Store) read(repo string) ([]Opportunity, error) {
	dir := filepath.Join(s.root, repo)
	es, e := os.ReadDir(dir)
	if errors.Is(e, os.ErrNotExist) {
		return []Opportunity{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Opportunity{}
	for _, x := range es {
		if x.IsDir() {
			continue
		}
		var v Opportunity
		if b, e := os.ReadFile(filepath.Join(dir, x.Name())); e == nil && json.Unmarshal(b, &v) == nil {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishedAt.Before(out[j].PublishedAt) })
	return out, nil
}
func (s *Store) write(v Opportunity) error {
	dir := filepath.Join(s.root, v.RepositoryID)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := filepath.Join(dir, "."+v.ID+".tmp")
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, filepath.Join(dir, v.ID+".json"))
}
func newID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
