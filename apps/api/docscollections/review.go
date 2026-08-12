package docscollections

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ReviewPage struct {
	Path           string `json:"path"`
	Slug           string `json:"slug"`
	Title          string `json:"title"`
	SourceObjectID string `json:"source_object_id"`
	SourceSHA256   string `json:"source_sha256"`
	RenderedHTML   string `json:"rendered_html"`
	Status         string `json:"status"`
}
type NavigationChange struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}
type ReviewCheck struct {
	RunID       string   `json:"run_id"`
	Name        string   `json:"name"`
	State       string   `json:"state"`
	Version     string   `json:"version"`
	Revision    string   `json:"revision"`
	Selectors   []string `json:"selectors"`
	ArtifactIDs []string `json:"artifact_ids,omitempty"`
}
type ReviewGap struct {
	ID           string    `json:"id"`
	Path         string    `json:"path,omitempty"`
	Area         string    `json:"area"`
	Detail       string    `json:"detail"`
	IdentifiedBy string    `json:"identified_by"`
	CreatedAt    time.Time `json:"created_at"`
}
type ReviewEntry struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	Path         string    `json:"path"`
	Area         string    `json:"area"`
	SourceSHA256 string    `json:"source_sha256"`
	Body         string    `json:"body"`
	ActorID      string    `json:"actor_id"`
	Stale        bool      `json:"stale"`
	CreatedAt    time.Time `json:"created_at"`
}
type ReviewDecision struct {
	ID           string    `json:"id"`
	Path         string    `json:"path"`
	Area         string    `json:"area"`
	SourceSHA256 string    `json:"source_sha256"`
	Outcome      string    `json:"outcome"`
	Body         string    `json:"body,omitempty"`
	ActorID      string    `json:"actor_id"`
	Stale        bool      `json:"stale"`
	CreatedAt    time.Time `json:"created_at"`
}
type ReviewInvitation struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Role      string     `json:"role"`
	Areas     []string   `json:"areas"`
	ExpiresAt time.Time  `json:"expires_at"`
	InvitedBy string     `json:"invited_by"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
type PullReview struct {
	RepositoryID      string             `json:"repository_id"`
	PullRequestID     string             `json:"pull_request_id"`
	Revision          string             `json:"revision"`
	BaseRevision      string             `json:"base_revision"`
	CollectionID      string             `json:"collection_id,omitempty"`
	RootPath          string             `json:"root_path"`
	Pages             []ReviewPage       `json:"pages"`
	NavigationChanges []NavigationChange `json:"navigation_changes"`
	Checks            []ReviewCheck      `json:"verified_examples"`
	AffectedVersions  []string           `json:"affected_versions"`
	Gaps              []ReviewGap        `json:"gaps"`
	Entries           []ReviewEntry      `json:"entries"`
	Decisions         []ReviewDecision   `json:"decisions"`
	Invitations       []ReviewInvitation `json:"invitations"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

func reviewID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
func (s *Store) reviewPath(repo, pull string) string {
	return filepath.Join(s.root, repo, "pull-reviews", pull+".json")
}
func (s *Store) GetPullReview(repo, pull string) (PullReview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v PullReview
	if !validID(repo) || !validID(pull) || readJSON(s.reviewPath(repo, pull), &v) != nil {
		return v, ErrNotFound
	}
	return v, nil
}
func (s *Store) SavePullReview(v PullReview) (PullReview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(v.RepositoryID) || !validID(v.PullRequestID) || len(v.Revision) != 40 || len(v.BaseRevision) != 40 {
		return v, ErrInvalid
	}
	now := s.now().UTC()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if err := os.MkdirAll(filepath.Dir(s.reviewPath(v.RepositoryID, v.PullRequestID)), 0700); err != nil {
		return v, err
	}
	if err := writeJSON(s.reviewPath(v.RepositoryID, v.PullRequestID), v); err != nil {
		return v, err
	}
	return v, nil
}

// CreatePullReview publishes the first review for a pull atomically across
// Store instances and API processes. Callers must not use a separate read as
// an existence guard because that would permit two successful creators.
func (s *Store) CreatePullReview(v PullReview) (PullReview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(v.RepositoryID) || !validID(v.PullRequestID) || len(v.Revision) != 40 || len(v.BaseRevision) != 40 {
		return v, ErrInvalid
	}
	unlock, err := s.lock()
	if err != nil {
		return v, err
	}
	defer unlock()
	name := s.reviewPath(v.RepositoryID, v.PullRequestID)
	if _, err = os.Stat(name); err == nil {
		return v, ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return v, err
	}
	now := s.now().UTC()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if err = os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return v, err
	}
	if err = writeJSON(name, v); err != nil {
		return v, err
	}
	return v, nil
}
func (s *Store) UpdatePullReview(repo, pull string, mutate func(*PullReview) error) (PullReview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullReview{}, err
	}
	defer unlock()
	var v PullReview
	if readJSON(s.reviewPath(repo, pull), &v) != nil {
		return v, ErrNotFound
	}
	if err = mutate(&v); err != nil {
		return v, err
	}
	v.UpdatedAt = s.now().UTC()
	if err = writeJSON(s.reviewPath(repo, pull), v); err != nil {
		return v, err
	}
	return v, nil
}
func NewReviewEntry(kind, path, area, sha, body, actor string, now time.Time) ReviewEntry {
	return ReviewEntry{ID: reviewID(), Kind: kind, Path: path, Area: area, SourceSHA256: sha, Body: strings.TrimSpace(body), ActorID: actor, CreatedAt: now.UTC()}
}
func NewReviewDecision(path, area, sha, outcome, body, actor string, now time.Time) ReviewDecision {
	return ReviewDecision{ID: reviewID(), Path: path, Area: area, SourceSHA256: sha, Outcome: outcome, Body: strings.TrimSpace(body), ActorID: actor, CreatedAt: now.UTC()}
}
func NewReviewGap(path, area, detail, actor string, now time.Time) ReviewGap {
	return ReviewGap{ID: reviewID(), Path: path, Area: area, Detail: strings.TrimSpace(detail), IdentifiedBy: actor, CreatedAt: now.UTC()}
}
func NewReviewInvitation(user, role string, areas []string, expires time.Time, actor string) ReviewInvitation {
	sort.Strings(areas)
	return ReviewInvitation{ID: reviewID(), UserID: user, Role: role, Areas: areas, ExpiresAt: expires.UTC(), InvitedBy: actor}
}
func ValidReviewArea(v string) bool {
	return v == "technical" || v == "audience" || v == "navigation" || v == "examples" || v == "versions"
}
func ValidateReviewMutation(path, area, body string) error {
	if !cleanPath(path) || !ValidReviewArea(area) || strings.TrimSpace(body) == "" || len(body) > 10000 {
		return ErrInvalid
	}
	return nil
}
func ActiveInvitation(v PullReview, user string, now time.Time) (ReviewInvitation, bool) {
	for _, x := range v.Invitations {
		if x.UserID == user && x.RevokedAt == nil && x.ExpiresAt.After(now) {
			return x, true
		}
	}
	return ReviewInvitation{}, false
}

var ErrReviewForbidden = errors.New("documentation review action forbidden")
