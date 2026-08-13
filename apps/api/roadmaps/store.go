// Package roadmaps retains versioned, accountable product direction.
package roadmaps

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("roadmap not found")
var ErrInvalid = errors.New("invalid roadmap")
var ErrConflict = errors.New("roadmap changed")

type OpportunityDecision struct {
	OpportunityID         string   `json:"opportunity_id"`
	Version               int      `json:"opportunity_version"`
	Outcome               string   `json:"outcome"`
	Reason                string   `json:"reason"`
	GoalFit               string   `json:"goal_fit"`
	Capacity              string   `json:"capacity"`
	Dependencies          []string `json:"dependencies"`
	Risks                 []string `json:"risks"`
	GovernanceDecisionIDs []string `json:"governance_decision_ids"`
	CommitmentIDs         []string `json:"commitment_ids"`
}
type Item struct {
	ID              string   `json:"id"`
	OpportunityID   string   `json:"opportunity_id"`
	Title           string   `json:"title"`
	OwnerIDs        []string `json:"owner_ids"`
	TargetHorizon   string   `json:"target_horizon"`
	SuccessMeasures []string `json:"success_measures"`
	DependsOn       []string `json:"depends_on"`
	Position        int      `json:"position"`
	Status          string   `json:"status"`
}
type Revision struct {
	Version        int                   `json:"version"`
	Goals          []string              `json:"goals"`
	Capacity       string                `json:"capacity"`
	Decisions      []OpportunityDecision `json:"decisions"`
	Items          []Item                `json:"items"`
	ChangeReason   string                `json:"change_reason"`
	ReplanTriggers []string              `json:"replan_triggers"`
	CreatedBy      string                `json:"created_by"`
	CreatedAt      time.Time             `json:"created_at"`
}
type Scenario struct {
	ID        string    `json:"id"`
	Revision  Revision  `json:"revision"`
	Rationale string    `json:"rationale"`
	ActorID   string    `json:"actor_id"`
	ActorType string    `json:"actor_type"`
	CreatedAt time.Time `json:"created_at"`
}
type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	ActorID   string    `json:"actor_id"`
	ActorType string    `json:"actor_type"`
	CreatedAt time.Time `json:"created_at"`
}
type DeliveryEvidence struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Summary        string    `json:"summary"`
	ResourceKind   string    `json:"resource_kind"`
	ResourceID     string    `json:"resource_id"`
	ActorID        string    `json:"actor_id"`
	MeasureIndexes []int     `json:"measure_indexes,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
type Implementation struct {
	RoadmapVersion int                `json:"roadmap_version"`
	ItemID         string             `json:"item_id"`
	OpportunityID  string             `json:"opportunity_id"`
	ProposalID     string             `json:"proposal_id"`
	Revision       string             `json:"revision"`
	CreatedBy      string             `json:"created_by"`
	TaskIDs        []string           `json:"task_ids"`
	Evidence       []DeliveryEvidence `json:"evidence"`
	OutcomeState   string             `json:"outcome_state"`
	RevisitReason  string             `json:"revisit_reason,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}
type Roadmap struct {
	RepositoryID    string           `json:"repository_id"`
	Version         int              `json:"version"`
	Revisions       []Revision       `json:"revisions"`
	Scenarios       []Scenario       `json:"scenarios"`
	Comments        []Comment        `json:"comments"`
	Implementations []Implementation `json:"implementations"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

func (s *Store) LinkImplementation(repo, actor string, expected, roadmapVersion int, itemID, opportunityID, proposalID, revision string, taskIDs []string) (Roadmap, error) {
	if actor == "" || roadmapVersion < 1 || itemID == "" || opportunityID == "" || proposalID == "" || len(revision) != 40 || len(taskIDs) == 0 {
		return Roadmap{}, ErrInvalid
	}
	return s.mutate(repo, expected, func(v *Roadmap) error {
		for _, x := range v.Implementations {
			if x.RoadmapVersion == roadmapVersion && x.ItemID == itemID {
				if x.ProposalID == proposalID {
					return nil
				}
				return ErrConflict
			}
		}
		var item *Item
		for _, r := range v.Revisions {
			if r.Version == roadmapVersion {
				for i := range r.Items {
					if r.Items[i].ID == itemID && r.Items[i].OpportunityID == opportunityID {
						item = &r.Items[i]
					}
				}
			}
		}
		if item == nil {
			return ErrInvalid
		}
		now := s.now().UTC()
		v.Implementations = append(v.Implementations, Implementation{RoadmapVersion: roadmapVersion, ItemID: itemID, OpportunityID: opportunityID, ProposalID: proposalID, Revision: revision, TaskIDs: append([]string(nil), taskIDs...), OutcomeState: "delivering", CreatedBy: actor, CreatedAt: now, UpdatedAt: now})
		return nil
	})
}

func (s *Store) ReportOutcome(repo, actor string, expected int, proposalID string, evidence DeliveryEvidence) (Roadmap, error) {
	allowed := one(evidence.Kind, "delivery", "measure_met", "measure_failed", "assumption_changed", "need_unresolved", "policy_conflict", "decision_revisit")
	if actor == "" || !allowed || !text(evidence.Summary, 2000) || evidence.ResourceKind == "" || evidence.ResourceID == "" {
		return Roadmap{}, ErrInvalid
	}
	return s.mutate(repo, expected, func(v *Roadmap) error {
		idx := -1
		for i := range v.Implementations {
			if v.Implementations[i].ProposalID == proposalID {
				idx = i
			}
		}
		if idx < 0 {
			return ErrNotFound
		}
		var item *Item
		for _, r := range v.Revisions {
			if r.Version == v.Implementations[idx].RoadmapVersion {
				for i := range r.Items {
					if r.Items[i].ID == v.Implementations[idx].ItemID {
						item = &r.Items[i]
					}
				}
			}
		}
		if item == nil {
			return ErrInvalid
		}
		seen := map[int]bool{}
		for _, n := range evidence.MeasureIndexes {
			if n < 0 || n >= len(item.SuccessMeasures) || seen[n] {
				return ErrInvalid
			}
			seen[n] = true
		}
		if evidence.Kind == "measure_met" || evidence.Kind == "measure_failed" {
			if len(seen) == 0 {
				return ErrInvalid
			}
		}
		now := s.now().UTC()
		evidence.ID = id()
		evidence.ActorID = actor
		evidence.CreatedAt = now
		v.Implementations[idx].Evidence = append(v.Implementations[idx].Evidence, evidence)
		v.Implementations[idx].UpdatedAt = now
		blocking := evidence.Kind == "measure_failed" || evidence.Kind == "assumption_changed" || evidence.Kind == "need_unresolved" || evidence.Kind == "policy_conflict" || evidence.Kind == "decision_revisit"
		if blocking {
			v.Implementations[idx].OutcomeState = "revisit_required"
			v.Implementations[idx].RevisitReason = evidence.Kind + ": " + evidence.Summary
			return nil
		}
		met := map[int]bool{}
		for _, e := range v.Implementations[idx].Evidence {
			if e.Kind == "measure_met" {
				for _, n := range e.MeasureIndexes {
					met[n] = true
				}
			}
		}
		if len(met) == len(item.SuccessMeasures) && v.Implementations[idx].OutcomeState != "revisit_required" {
			v.Implementations[idx].OutcomeState = "achieved"
		}
		return nil
	})
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
func id() string                { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func text(v string, n int) bool { x := len(strings.TrimSpace(v)); return x > 0 && x <= n }
func one(v string, xs ...string) bool {
	for _, x := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func valid(r Revision, replan bool) bool {
	if len(r.Goals) == 0 || len(r.Goals) > 30 || !text(r.Capacity, 5000) || len(r.Decisions) == 0 || len(r.Decisions) > 200 || len(r.Items) > 100 || replan && !text(r.ChangeReason, 5000) {
		return false
	}
	seen := map[string]bool{}
	accepted := map[string]bool{}
	for _, d := range r.Decisions {
		if !text(d.OpportunityID, 200) || d.Version < 1 || seen[d.OpportunityID] || !one(d.Outcome, "accepted", "rejected", "deferred") || !text(d.Reason, 5000) || !text(d.GoalFit, 2000) || !text(d.Capacity, 2000) {
			return false
		}
		seen[d.OpportunityID] = true
		accepted[d.OpportunityID] = d.Outcome == "accepted"
	}
	ids := map[string]bool{}
	for i, x := range r.Items {
		if !text(x.ID, 200) || ids[x.ID] || !accepted[x.OpportunityID] || !text(x.Title, 300) || len(x.OwnerIDs) == 0 || !text(x.TargetHorizon, 300) || len(x.SuccessMeasures) == 0 || x.Position != i+1 || !one(x.Status, "planned", "in_progress", "at_risk", "delivered", "cancelled") {
			return false
		}
		ids[x.ID] = true
	}
	for _, x := range r.Items {
		for _, d := range x.DependsOn {
			if !ids[d] || d == x.ID {
				return false
			}
		}
	}
	return true
}
func (s *Store) path(repo string) string { return filepath.Join(s.root, repo+".json") }
func (s *Store) lock() (*os.File, error) {
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e == nil {
		e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	}
	return f, e
}
func (s *Store) read(repo string) (Roadmap, error) {
	b, e := os.ReadFile(s.path(repo))
	if errors.Is(e, os.ErrNotExist) {
		return Roadmap{}, ErrNotFound
	}
	var v Roadmap
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *Store) write(v Roadmap) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.root, ".roadmap-")
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
		e = os.Rename(n, s.path(v.RepositoryID))
	}
	return e
}
func (s *Store) Get(repo string) (Roadmap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo)
}
func (s *Store) Publish(repo, actor string, expected int, r Revision) (Roadmap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.lock()
	if e != nil {
		return Roadmap{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	v, e := s.read(repo)
	creating := errors.Is(e, ErrNotFound)
	if e != nil && !creating {
		return Roadmap{}, e
	}
	if creating {
		if expected != 0 || !valid(r, false) {
			return Roadmap{}, map[bool]error{true: ErrInvalid, false: ErrConflict}[expected == 0]
		}
		v = Roadmap{RepositoryID: repo, Scenarios: []Scenario{}, Comments: []Comment{}}
	} else if v.Version != expected {
		return Roadmap{}, ErrConflict
	} else if !valid(r, true) {
		return Roadmap{}, ErrInvalid
	}
	now := s.now().UTC()
	v.Version++
	r.Version = v.Version
	r.CreatedBy = actor
	r.CreatedAt = now
	v.Revisions = append(v.Revisions, r)
	v.UpdatedAt = now
	e = s.write(v)
	return v, e
}
func (s *Store) mutate(repo string, expected int, fn func(*Roadmap) error) (Roadmap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.lock()
	if e != nil {
		return Roadmap{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	v, e := s.read(repo)
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
	v.UpdatedAt = s.now().UTC()
	e = s.write(v)
	return v, e
}
func (s *Store) Propose(repo, actor, actorType string, expected int, r Revision, reason string) (Roadmap, error) {
	if !valid(r, false) || !text(reason, 5000) || !one(actorType, "human", "agent") {
		return Roadmap{}, ErrInvalid
	}
	return s.mutate(repo, expected, func(v *Roadmap) error {
		r.Version = 0
		r.CreatedBy = ""
		r.CreatedAt = time.Time{}
		v.Scenarios = append(v.Scenarios, Scenario{ID: id(), Revision: r, Rationale: reason, ActorID: actor, ActorType: actorType, CreatedAt: s.now().UTC()})
		return nil
	})
}
func (s *Store) Comment(repo, actor, actorType string, expected int, body string) (Roadmap, error) {
	if !text(body, 5000) || !one(actorType, "human", "agent") {
		return Roadmap{}, ErrInvalid
	}
	return s.mutate(repo, expected, func(v *Roadmap) error {
		v.Comments = append(v.Comments, Comment{ID: id(), Body: body, ActorID: actor, ActorType: actorType, CreatedAt: s.now().UTC()})
		return nil
	})
}
func (s *Store) List() ([]Roadmap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Roadmap{}
	for _, x := range es {
		if x.IsDir() || filepath.Ext(x.Name()) != ".json" {
			continue
		}
		v, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
