// Package outcomevalidations retains pre-commitment learning linked to roadmap outcomes.
package outcomevalidations

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("outcome validation not found")
var ErrInvalid = errors.New("invalid outcome validation")
var ErrConflict = errors.New("outcome validation changed")

const maxFindings = 250

type Measure struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Target    string   `json:"target"`
	SourceIDs []string `json:"source_ids"`
}
type Invitation struct {
	ID            string     `json:"id"`
	ParticipantID string     `json:"participant_id"`
	Activity      string     `json:"activity"`
	Revision      string     `json:"revision"`
	ExpiresAt     time.Time  `json:"expires_at"`
	Status        string     `json:"status"`
	InvitedBy     string     `json:"invited_by"`
	RespondedAt   *time.Time `json:"responded_at,omitempty"`
}
type Finding struct {
	ID                 string    `json:"id"`
	InvitationID       string    `json:"invitation_id"`
	Body               string    `json:"body"`
	AccessibilityNeeds []string  `json:"accessibility_needs"`
	Dissent            string    `json:"dissent"`
	Acceptance         string    `json:"acceptance"`
	EvidenceQuality    string    `json:"evidence_quality"`
	ActorID            string    `json:"actor_id"`
	CreatedAt          time.Time `json:"created_at"`
}
type Conclusion struct {
	Outcome   string    `json:"outcome"`
	Reason    string    `json:"reason"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Validation struct {
	ID                 string       `json:"id"`
	RepositoryID       string       `json:"repository_id"`
	RoadmapVersion     int          `json:"roadmap_version"`
	ItemID             string       `json:"item_id"`
	OpportunityID      string       `json:"opportunity_id"`
	OpportunityVersion int          `json:"opportunity_version"`
	Kind               string       `json:"kind"`
	Title              string       `json:"title"`
	Question           string       `json:"question"`
	Revision           string       `json:"revision"`
	Measures           []Measure    `json:"measures"`
	CreatedBy          string       `json:"created_by"`
	CreatedAt          time.Time    `json:"created_at"`
	Version            int          `json:"version"`
	Invitations        []Invitation `json:"invitations"`
	Findings           []Finding    `json:"findings"`
	Conclusions        []Conclusion `json:"conclusions"`
}
type Draft struct {
	RoadmapVersion int       `json:"roadmap_version"`
	ItemID         string    `json:"item_id"`
	Kind           string    `json:"kind"`
	Title          string    `json:"title"`
	Question       string    `json:"question"`
	Revision       string    `json:"revision"`
	Measures       []Measure `json:"measures"`
}
type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	return &Store{root: root, now: time.Now}, nil
}
func ident() string             { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func text(v string, n int) bool { l := len(strings.TrimSpace(v)); return l > 0 && l <= n }
func one(v string, xs ...string) bool {
	for _, x := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func validDraft(d Draft) bool {
	if d.RoadmapVersion < 1 || !text(d.ItemID, 200) || !one(d.Kind, "technical_decision", "prototype", "documentation_concept", "product_experiment") || !text(d.Title, 300) || !text(d.Question, 5000) || !text(d.Revision, 500) || len(d.Measures) == 0 || len(d.Measures) > 30 {
		return false
	}
	for _, m := range d.Measures {
		if !text(m.Name, 300) || !one(m.Kind, "success", "guardrail") || !text(m.Target, 1000) || len(m.SourceIDs) == 0 {
			return false
		}
	}
	return true
}
func (s *Store) path(repo, id string) string { return filepath.Join(s.root, repo+"-"+id+".json") }
func (s *Store) lock() (*os.File, error) {
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e == nil {
		e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	}
	return f, e
}
func (s *Store) read(repo, id string) (Validation, error) {
	b, e := os.ReadFile(s.path(repo, id))
	if errors.Is(e, os.ErrNotExist) {
		return Validation{}, ErrNotFound
	}
	var v Validation
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *Store) write(v Validation) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.root, ".validation-")
	if e != nil {
		return e
	}
	n := f.Name()
	defer os.Remove(n)
	_ = f.Chmod(0600)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(n, s.path(v.RepositoryID, v.ID))
	}
	return e
}
func (s *Store) Create(repo, actor, opportunity string, opportunityVersion int, d Draft) (Validation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !text(repo, 200) || !text(actor, 200) || !text(opportunity, 200) || opportunityVersion < 1 || !validDraft(d) {
		return Validation{}, ErrInvalid
	}
	f, e := s.lock()
	if e != nil {
		return Validation{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	now := s.now().UTC()
	v := Validation{ID: ident(), RepositoryID: repo, RoadmapVersion: d.RoadmapVersion, ItemID: d.ItemID, OpportunityID: opportunity, OpportunityVersion: opportunityVersion, Kind: d.Kind, Title: d.Title, Question: d.Question, Revision: d.Revision, Measures: d.Measures, CreatedBy: actor, CreatedAt: now, Version: 1, Invitations: []Invitation{}, Findings: []Finding{}, Conclusions: []Conclusion{}}
	e = s.write(v)
	return v, e
}
func (s *Store) Get(repo, id string) (Validation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo, id)
}

// GuestAccess reports whether a named participant currently has accepted,
// revision-exact access. Repository collaborators are authorized separately.
func (s *Store) GuestAccess(repo, id, participant string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return false, err
	}
	now := s.now().UTC()
	for _, invitation := range v.Invitations {
		if invitation.ParticipantID == participant && invitation.Status == "accepted" && invitation.Revision == v.Revision && invitation.ExpiresAt.After(now) {
			return true, nil
		}
	}
	return false, nil
}
func (s *Store) List(repo string) ([]Validation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Validation{}
	for _, x := range es {
		if x.IsDir() || !strings.HasPrefix(x.Name(), repo+"-") || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		b, e := os.ReadFile(filepath.Join(s.root, x.Name()))
		var v Validation
		if e == nil {
			e = json.Unmarshal(b, &v)
		}
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) mutate(repo, id string, expected int, fn func(*Validation) error) (Validation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.lock()
	if e != nil {
		return Validation{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	v, e := s.read(repo, id)
	if e != nil {
		return v, e
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	if e = fn(&v); e != nil {
		return v, e
	}
	v.Version++
	e = s.write(v)
	return v, e
}
func (s *Store) Invite(repo, id, actor, participant, activity, revision string, expires time.Time, expected int) (Validation, error) {
	if !text(participant, 200) || !one(activity, "preview", "research") || !text(revision, 500) || !expires.After(s.now().UTC()) {
		return Validation{}, ErrInvalid
	}
	return s.mutate(repo, id, expected, func(v *Validation) error {
		if revision != v.Revision {
			return ErrInvalid
		}
		v.Invitations = append(v.Invitations, Invitation{ID: ident(), ParticipantID: participant, Activity: activity, Revision: revision, ExpiresAt: expires.UTC(), Status: "invited", InvitedBy: actor})
		return nil
	})
}
func (s *Store) Consent(repo, id, invite, actor, status string, expected int) (Validation, error) {
	if !one(status, "accepted", "declined", "withdrawn") {
		return Validation{}, ErrInvalid
	}
	return s.mutate(repo, id, expected, func(v *Validation) error {
		for i := range v.Invitations {
			p := &v.Invitations[i]
			initialResponse := p.Status == "invited" && (status == "accepted" || status == "declined")
			withdrawal := p.Status == "accepted" && status == "withdrawn"
			if p.ID == invite && p.ParticipantID == actor && (initialResponse || withdrawal) && p.ExpiresAt.After(s.now().UTC()) {
				now := s.now().UTC()
				p.Status = status
				p.RespondedAt = &now
				return nil
			}
		}
		return ErrInvalid
	})
}
func (s *Store) Find(repo, id, invite, actor string, expected int, f Finding) (Validation, error) {
	if !text(f.Body, 5000) || len(f.Dissent) > 5000 || len(f.AccessibilityNeeds) > 20 || !one(f.Acceptance, "accept", "reject", "uncertain") || !one(f.EvidenceQuality, "valid", "insufficient", "invalid") {
		return Validation{}, ErrInvalid
	}
	accessibilityBytes := 0
	for _, need := range f.AccessibilityNeeds {
		if !text(need, 500) {
			return Validation{}, ErrInvalid
		}
		accessibilityBytes += len(need)
	}
	if accessibilityBytes > 5000 {
		return Validation{}, ErrInvalid
	}
	return s.mutate(repo, id, expected, func(v *Validation) error {
		if len(v.Findings) >= maxFindings {
			return ErrInvalid
		}
		ok := false
		for _, p := range v.Invitations {
			ok = ok || (p.ID == invite && p.ParticipantID == actor && p.Status == "accepted" && p.Revision == v.Revision && p.ExpiresAt.After(s.now().UTC()))
		}
		if !ok {
			return ErrInvalid
		}
		f.ID = ident()
		f.InvitationID = invite
		f.ActorID = actor
		f.CreatedAt = s.now().UTC()
		v.Findings = append(v.Findings, f)
		return nil
	})
}
func (s *Store) Conclude(repo, id, actor, outcome, reason string, expected int) (Validation, error) {
	if !one(outcome, "validated", "revise", "defer", "reject") || !text(reason, 5000) {
		return Validation{}, ErrInvalid
	}
	return s.mutate(repo, id, expected, func(v *Validation) error {
		v.Conclusions = append(v.Conclusions, Conclusion{Outcome: outcome, Reason: reason, ActorID: actor, CreatedAt: s.now().UTC()})
		return nil
	})
}
