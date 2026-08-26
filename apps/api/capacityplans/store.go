// Package capacityplans retains approved, revision-exact scaling delivery programs.
package capacityplans

import (
	"crypto/sha256"
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

var ErrNotFound = errors.New("capacity plan not found")
var ErrInvalid = errors.New("invalid capacity plan")
var ErrConflict = errors.New("capacity plan conflict")

type Dependency struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	OwnerID     string    `json:"owner_id"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	RequiredBy  time.Time `json:"required_by"`
}
type Phase struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Kind               string   `json:"kind"`
	OwnerID            string   `json:"owner_id"`
	Budget             float64  `json:"budget"`
	Currency           string   `json:"currency"`
	DependsOn          []string `json:"depends_on"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	DecisionPoint      string   `json:"decision_point"`
	ExitStrategy       string   `json:"exit_strategy"`
}
type Delivery struct {
	ProposalID   string    `json:"proposal_id"`
	TaskIDs      []string  `json:"task_ids"`
	BaseRevision string    `json:"base_revision"`
	CreatedAt    time.Time `json:"created_at"`
}
type Plan struct {
	RequestID        string       `json:"request_id"`
	ID               string       `json:"id"`
	RepositoryID     string       `json:"repository_id"`
	ObjectiveID      string       `json:"objective_id"`
	ObjectiveVersion int          `json:"objective_version"`
	ModelID          string       `json:"model_id,omitempty"`
	ModelVersion     int          `json:"model_version,omitempty"`
	TestID           string       `json:"test_id"`
	CandidateID      string       `json:"candidate_id"`
	Title            string       `json:"title"`
	Rationale        string       `json:"rationale"`
	Reservations     []Dependency `json:"reservations"`
	Dependencies     []Dependency `json:"dependencies"`
	Phases           []Phase      `json:"phases"`
	TotalBudget      float64      `json:"total_budget"`
	Currency         string       `json:"currency"`
	ApprovedBy       string       `json:"approved_by"`
	CreatedAt        time.Time    `json:"created_at"`
	Delivery         *Delivery    `json:"delivery,omitempty"`
}
type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}
func (s *Store) Create(repositoryID, actor, request string, p Plan) (Plan, error) {
	var out Plan
	err := s.lock(func() error {
		if request == "" || !valid(p) {
			return ErrInvalid
		}
		p.RequestID = request
		p.ID = stable(repositoryID, actor, request)
		p.RepositoryID = repositoryID
		p.ApprovedBy = actor
		if old, e := s.read(p.ID); e == nil {
			p.CreatedAt = old.CreatedAt
			p.Delivery = old.Delivery
			if digest(old) != digest(p) {
				return ErrConflict
			}
			out = old
			return nil
		}
		p.CreatedAt = s.now()
		out = p
		return s.write(p)
	})
	return out, err
}
func (s *Store) LinkDelivery(repositoryID, id string, d Delivery) (Plan, error) {
	var out Plan
	err := s.lock(func() error {
		p, e := s.read(id)
		if e != nil || p.RepositoryID != repositoryID {
			return ErrNotFound
		}
		if p.Delivery != nil {
			d.CreatedAt = p.Delivery.CreatedAt
			if digest(*p.Delivery) != digest(d) {
				return ErrConflict
			}
			out = p
			return nil
		}
		if d.ProposalID == "" || len(d.TaskIDs) != len(p.Phases) || len(d.BaseRevision) != 40 {
			return ErrInvalid
		}
		d.CreatedAt = s.now()
		p.Delivery = &d
		out = p
		return s.write(p)
	})
	return out, err
}
func (s *Store) Get(repositoryID, id string) (Plan, error) {
	var p Plan
	e := s.lock(func() error {
		var x error
		p, x = s.read(id)
		if x == nil && p.RepositoryID != repositoryID {
			return ErrNotFound
		}
		return x
	})
	return p, e
}
func (s *Store) List(repositoryID string) ([]Plan, error) {
	xs := []Plan{}
	e := s.lock(func() error {
		fs, x := os.ReadDir(s.root)
		if x != nil {
			return x
		}
		for _, f := range fs {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			var p Plan
			if x = s.decode(f.Name(), &p); x != nil {
				return x
			}
			if p.RepositoryID == repositoryID {
				xs = append(xs, p)
			}
		}
		return nil
	})
	sort.Slice(xs, func(i, j int) bool { return xs[i].CreatedAt.After(xs[j].CreatedAt) })
	return xs, e
}
func valid(p Plan) bool {
	if p.ObjectiveID == "" || p.ObjectiveVersion < 1 || p.TestID == "" || p.CandidateID == "" || strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.Rationale) == "" || len(p.Phases) == 0 || p.TotalBudget < 0 || p.Currency == "" {
		return false
	}
	ids := map[string]bool{}
	sum := 0.0
	for _, x := range p.Phases {
		if x.ID == "" || ids[x.ID] || x.Name == "" || x.OwnerID == "" || x.Budget < 0 || x.Currency != p.Currency || len(x.AcceptanceCriteria) == 0 || x.DecisionPoint == "" || x.ExitStrategy == "" {
			return false
		}
		ids[x.ID] = true
		sum += x.Budget
	}
	for _, x := range p.Phases {
		for _, d := range x.DependsOn {
			if !ids[d] || d == x.ID {
				return false
			}
		}
	}
	return sum <= p.TotalBudget
}
func stable(v ...string) string {
	h := sha256.Sum256([]byte(strings.Join(v, "\x00")))
	return hex.EncodeToString(h[:16])
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Store) read(id string) (Plan, error) {
	var p Plan
	e := s.decode(id+".json", &p)
	if os.IsNotExist(e) {
		e = ErrNotFound
	}
	return p, e
}
func (s *Store) decode(name string, v any) error {
	b, e := os.ReadFile(filepath.Join(s.root, name))
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (s *Store) write(p Plan) error {
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".capacity-plan-")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	if closeErr := tmp.Close(); e == nil {
		e = closeErr
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, p.ID+".json"))
	}
	return e
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
