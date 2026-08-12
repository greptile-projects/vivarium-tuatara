// Package governance retains charter-bound community proposals, deliberation, ballots, and tallies.
package governance

import (
	"crypto/rand"
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

var ErrNotFound = errors.New("governed proposal not found")
var ErrInvalid = errors.New("invalid governed proposal")
var ErrConflict = errors.New("governed proposal changed")
var ErrDuplicateBallot = errors.New("duplicate ballot")
var ErrFinalized = errors.New("tally already finalized")
var ErrClosed = errors.New("voting is closed")
var ErrIneligible = errors.New("actor is not eligible")
var ErrNotAccepted = errors.New("governed proposal was not accepted")
var ErrMaterialChange = errors.New("implementation materially changed")

type Reference struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision,omitempty"`
	Label      string `json:"label"`
	URL        string `json:"url,omitempty"`
}
type Alternative struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Effects []string `json:"effects"`
}
type Rule struct {
	DecisionClass string    `json:"decision_class"`
	EligibleRoles []string  `json:"eligible_roles"`
	Quorum        int       `json:"quorum"`
	Threshold     string    `json:"threshold"`
	SecretBallot  bool      `json:"secret_ballot"`
	OpensAt       time.Time `json:"opens_at"`
	ClosesAt      time.Time `json:"closes_at"`
}
type Elector struct {
	UserID   string   `json:"user_id"`
	Roles    []string `json:"roles"`
	Eligible bool     `json:"eligible"`
	Reason   string   `json:"reason,omitempty"`
}
type Analysis struct {
	ID        string      `json:"id"`
	ActorType string      `json:"actor_type"`
	ActorID   string      `json:"actor_id"`
	Body      string      `json:"body"`
	Position  string      `json:"position"`
	Citations []Reference `json:"citations"`
	CreatedAt time.Time   `json:"created_at"`
}
type Ballot struct {
	ID                string    `json:"id"`
	ActorID           string    `json:"actor_id,omitempty"`
	Choice            string    `json:"choice"`
	Reason            string    `json:"reason,omitempty"`
	Receipt           string    `json:"receipt"`
	CastAt            time.Time `json:"cast_at"`
	EligibleAtTally   bool      `json:"eligible_at_tally"`
	EligibilityReason string    `json:"eligibility_reason,omitempty"`
}
type Tally struct {
	Status             string         `json:"status"`
	Eligible           int            `json:"eligible"`
	Participating      int            `json:"participating"`
	Abstentions        int            `json:"abstentions"`
	Recusals           int            `json:"recusals"`
	Counts             map[string]int `json:"counts"`
	QuorumMet          bool           `json:"quorum_met"`
	ThresholdMet       bool           `json:"threshold_met"`
	Result             string         `json:"result,omitempty"`
	Contested          bool           `json:"contested"`
	ContestReasons     []string       `json:"contest_reasons"`
	ComputedAt         time.Time      `json:"computed_at"`
	VerificationSHA256 string         `json:"verification_sha256"`
}
type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}
type DecisionReceipt struct {
	ID                  string      `json:"id"`
	ProposalID          string      `json:"proposal_id"`
	CharterVersion      int         `json:"charter_version"`
	DecisionClass       string      `json:"decision_class"`
	Result              Alternative `json:"result"`
	TallySHA256         string      `json:"tally_sha256"`
	AuthorizationSHA256 string      `json:"authorization_sha256"`
	IssuedAt            time.Time   `json:"issued_at"`
}
type ImplementationStep struct {
	ID               string    `json:"id"`
	Kind             string    `json:"kind"`
	ResourceID       string    `json:"resource_id,omitempty"`
	Status           string    `json:"status"`
	Summary          string    `json:"summary"`
	RequiredApproval string    `json:"required_approval,omitempty"`
	Blocker          string    `json:"blocker,omitempty"`
	ActorID          string    `json:"actor_id"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type Implementation struct {
	Receipt          DecisionReceipt      `json:"receipt"`
	Kind             string               `json:"kind"`
	RepositoryID     string               `json:"repository_id"`
	Scope            string               `json:"scope"`
	Cost             string               `json:"cost"`
	Assumptions      []string             `json:"assumptions"`
	ProtectedEffects []string             `json:"protected_effects"`
	Steps            []ImplementationStep `json:"steps"`
	CreatedBy        string               `json:"created_by"`
	CreatedAt        time.Time            `json:"created_at"`
}
type Proposal struct {
	ID                     string          `json:"id"`
	ScopeType              string          `json:"scope_type"`
	ScopeID                string          `json:"scope_id"`
	CharterVersion         int             `json:"charter_version"`
	Source                 Reference       `json:"source"`
	Title                  string          `json:"title"`
	Summary                string          `json:"summary"`
	Scope                  string          `json:"scope"`
	Alternatives           []Alternative   `json:"alternatives"`
	Evidence               []Reference     `json:"evidence"`
	AffectedResources      []Reference     `json:"affected_resources"`
	DisclosureRequirements []string        `json:"disclosure_requirements"`
	ImplementationEffects  []string        `json:"implementation_effects"`
	Rule                   Rule            `json:"rule"`
	Electorate             []Elector       `json:"electorate"`
	Analyses               []Analysis      `json:"analyses"`
	Ballots                []Ballot        `json:"ballots"`
	Tally                  *Tally          `json:"tally,omitempty"`
	Implementation         *Implementation `json:"implementation,omitempty"`
	Status                 string          `json:"status"`
	CreatedBy              string          `json:"created_by"`
	CreatedAt              time.Time       `json:"created_at"`
	Events                 []Event         `json:"events"`
}

// BeginImplementation freezes the accepted mandate before ordinary resource
// owners route it through their existing controls. Exact retries converge;
// changed scope, cost, assumptions, or protected effects require amendment.
func (s *Store) BeginImplementation(pid, actor string, in Implementation) (Proposal, error) {
	return s.change(pid, func(p *Proposal) error {
		if p.Tally == nil || p.Tally.Status != "accepted" || p.Tally.Contested || p.Tally.Result == "" {
			return ErrNotAccepted
		}
		var result Alternative
		for _, a := range p.Alternatives {
			if a.ID == p.Tally.Result {
				result = a
			}
		}
		if actor == "" || in.Kind != "task_plan" || in.RepositoryID == "" || strings.TrimSpace(in.Scope) == "" || strings.TrimSpace(in.Cost) == "" || len(in.Assumptions) == 0 || len(in.ProtectedEffects) == 0 || result.ID == "" {
			return ErrInvalid
		}
		snapshot := struct {
			ProposalID                    string
			CharterVersion                int
			DecisionClass                 string
			Result                        Alternative
			Tally                         string
			Kind                          string
			RepositoryID                  string
			Scope                         string
			Cost                          string
			Assumptions, ProtectedEffects []string
		}{p.ID, p.CharterVersion, p.Rule.DecisionClass, result, p.Tally.VerificationSHA256, in.Kind, in.RepositoryID, strings.TrimSpace(in.Scope), strings.TrimSpace(in.Cost), in.Assumptions, in.ProtectedEffects}
		encoded, _ := json.Marshal(snapshot)
		sum := sha256.Sum256(encoded)
		authorization := hex.EncodeToString(sum[:])
		if p.Implementation != nil {
			if p.Implementation.Receipt.AuthorizationSHA256 != authorization {
				return ErrMaterialChange
			}
			return nil
		}
		now := s.now().UTC()
		in.Scope, in.Cost = snapshot.Scope, snapshot.Cost
		in.Assumptions = append([]string(nil), in.Assumptions...)
		in.ProtectedEffects = append([]string(nil), in.ProtectedEffects...)
		in.Receipt = DecisionReceipt{ID: id(), ProposalID: p.ID, CharterVersion: p.CharterVersion, DecisionClass: p.Rule.DecisionClass, Result: result, TallySHA256: p.Tally.VerificationSHA256, AuthorizationSHA256: authorization, IssuedAt: now}
		in.CreatedBy, in.CreatedAt = actor, now
		in.Steps = []ImplementationStep{{ID: id(), Kind: "resource_owner_handoff", Status: "pending", Summary: "Community mandate awaits ordinary repository proposal and task-plan publication", RequiredApproval: "repository owner publication; later review, integration, release, environment, extension, and agent controls remain independent", ActorID: actor, UpdatedAt: now}}
		p.Implementation = &in
		p.Events = append(p.Events, Event{ID: id(), Kind: "implementation.receipt_issued", ActorID: actor, Summary: "Issued immutable decision receipt without granting operational authority", CreatedAt: now})
		return nil
	})
}

func (s *Store) LinkImplementation(pid, actor, resourceID string) (Proposal, error) {
	return s.change(pid, func(p *Proposal) error {
		if p.Implementation == nil || resourceID == "" {
			return ErrInvalid
		}
		step := &p.Implementation.Steps[0]
		if step.ResourceID != "" && step.ResourceID != resourceID {
			return ErrConflict
		}
		if step.ResourceID == resourceID {
			return nil
		}
		now := s.now().UTC()
		step.ResourceID, step.Status, step.Blocker, step.ActorID, step.UpdatedAt = resourceID, "in_progress", "", actor, now
		step.Summary = "Ordinary repository proposal and task plan published; implementation remains subject to its resource controls"
		p.Events = append(p.Events, Event{ID: id(), Kind: "implementation.resource_linked", ActorID: actor, Summary: "Linked owner-approved implementation resource", CreatedAt: now})
		return nil
	})
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: time.Now}, nil
}
func (s *Store) Create(in Proposal) (Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return in, e
	}
	defer unlock()
	now := s.now().UTC()
	in.ID = id()
	in.CreatedAt = now
	in.Status = "open"
	in.Analyses = []Analysis{}
	in.Ballots = []Ballot{}
	in.Events = []Event{{ID: id(), Kind: "proposal.opened", ActorID: in.CreatedBy, Summary: "Opened a charter-bound community proposal", CreatedAt: now}}
	if !valid(in, now) {
		return in, ErrInvalid
	}
	return in, s.write(in)
}
func (s *Store) Get(id string) (Proposal, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.read(id) }
func (s *Store) List() ([]Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Proposal{}
	for _, x := range entries {
		if strings.HasSuffix(x.Name(), ".json") {
			p, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
			if e == nil {
				out = append(out, p)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Analyze(pid, actorType, actorID, body, position string, citations []Reference) (Proposal, error) {
	return s.change(pid, func(p *Proposal) error {
		if (actorType != "human" && actorType != "agent") || actorID == "" || strings.TrimSpace(body) == "" || len(citations) == 0 {
			return ErrInvalid
		}
		for _, c := range citations {
			if c.Kind == "" || c.ResourceID == "" || c.Label == "" {
				return ErrInvalid
			}
		}
		now := s.now().UTC()
		p.Analyses = append(p.Analyses, Analysis{ID: id(), ActorType: actorType, ActorID: actorID, Body: strings.TrimSpace(body), Position: position, Citations: citations, CreatedAt: now})
		p.Events = append(p.Events, Event{ID: id(), Kind: "analysis.added", ActorID: actorID, Summary: "Added cited " + actorType + " analysis", CreatedAt: now})
		return nil
	})
}
func (s *Store) Cast(pid, actor, choice, reason string, eligible bool, eligibilityReason string) (Proposal, error) {
	return s.change(pid, func(p *Proposal) error {
		now := s.now().UTC()
		if now.Before(p.Rule.OpensAt) || !now.Before(p.Rule.ClosesAt) {
			return ErrClosed
		}
		if !eligible {
			return ErrIneligible
		}
		if choice != "abstain" && choice != "recuse" && !alternative(p.Alternatives, choice) {
			return ErrInvalid
		}
		for _, b := range p.Ballots {
			if b.ActorID == actor {
				return ErrDuplicateBallot
			}
		}
		sum := sha256.Sum256([]byte(p.ID + "\x00" + actor + "\x00" + choice + "\x00" + now.Format(time.RFC3339Nano)))
		receipt := hex.EncodeToString(sum[:])
		p.Ballots = append(p.Ballots, Ballot{ID: id(), ActorID: actor, Choice: choice, Reason: strings.TrimSpace(reason), Receipt: receipt, CastAt: now, EligibleAtTally: true, EligibilityReason: eligibilityReason})
		p.Events = append(p.Events, Event{ID: id(), Kind: "ballot.cast", ActorID: actor, Summary: "Cast a ballot", CreatedAt: now})
		return nil
	})
}
func (s *Store) Finalize(pid, actor string, current []Elector, contest []string) (Proposal, error) {
	return s.change(pid, func(p *Proposal) error {
		now := s.now().UTC()
		if now.Before(p.Rule.ClosesAt) {
			return ErrClosed
		}
		if p.Status == "closed" || p.Tally != nil {
			return ErrFinalized
		}
		live := map[string]bool{}
		for _, e := range current {
			if e.Eligible {
				live[e.UserID] = true
			}
		}
		counts := map[string]int{}
		part, abstain, recuse := 0, 0, 0
		receipts := []string{}
		for i := range p.Ballots {
			b := &p.Ballots[i]
			b.EligibleAtTally = live[b.ActorID]
			if !b.EligibleAtTally {
				b.EligibilityReason = "eligibility changed before tally"
				continue
			}
			part++
			receipts = append(receipts, b.Receipt)
			if b.Choice == "abstain" {
				abstain++
			} else if b.Choice == "recuse" {
				recuse++
			} else {
				counts[b.Choice]++
			}
		}
		voting := part - abstain - recuse
		winner, max, ties := "", 0, false
		for choice, n := range counts {
			if n > max {
				winner, max, ties = choice, n, false
			} else if n == max && n > 0 {
				ties = true
			}
		}
		quorum := part >= p.Rule.Quorum
		threshold := false
		if voting > 0 && !ties {
			switch p.Rule.Threshold {
			case "consensus":
				threshold = max == voting
			case "supermajority":
				threshold = max*3 >= voting*2
			default:
				threshold = max*2 > voting
			}
		}
		if !quorum || !threshold {
			winner = ""
		}
		sort.Strings(receipts)
		verification, _ := json.Marshal(struct {
			Proposal string
			Charter  int
			Electors []Elector
			Receipts []string
			Counts   map[string]int
		}{p.ID, p.CharterVersion, current, receipts, counts})
		digest := sha256.Sum256(verification)
		p.Tally = &Tally{Status: map[bool]string{true: "accepted", false: "not_accepted"}[winner != ""], Eligible: len(live), Participating: part, Abstentions: abstain, Recusals: recuse, Counts: counts, QuorumMet: quorum, ThresholdMet: threshold, Result: winner, Contested: len(contest) > 0, ContestReasons: contest, ComputedAt: now, VerificationSHA256: hex.EncodeToString(digest[:])}
		p.Status = "closed"
		p.Events = append(p.Events, Event{ID: id(), Kind: "tally.finalized", ActorID: actor, Summary: "Finalized the deterministic tally", CreatedAt: now})
		return nil
	})
}
func (s *Store) change(pid string, fn func(*Proposal) error) (Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Proposal{}, e
	}
	defer unlock()
	p, e := s.read(pid)
	if e != nil {
		return p, e
	}
	if e = fn(&p); e != nil {
		return p, e
	}
	return p, s.write(p)
}
func valid(p Proposal, now time.Time) bool {
	return p.ScopeType != "" && p.ScopeID != "" && p.CharterVersion > 0 && p.Source.Kind != "" && p.Source.ResourceID != "" && p.Title != "" && p.Summary != "" && p.Scope != "" && len(p.Alternatives) >= 2 && len(p.Evidence) > 0 && len(p.AffectedResources) > 0 && len(p.DisclosureRequirements) > 0 && len(p.ImplementationEffects) > 0 && p.Rule.DecisionClass != "" && len(p.Rule.EligibleRoles) > 0 && p.Rule.Quorum > 0 && (p.Rule.Threshold == "majority" || p.Rule.Threshold == "supermajority" || p.Rule.Threshold == "consensus") && p.Rule.OpensAt.Before(p.Rule.ClosesAt) && p.Rule.ClosesAt.After(now) && p.CreatedBy != ""
}
func alternative(a []Alternative, id string) bool {
	for _, x := range a {
		if x.ID == id {
			return true
		}
	}
	return false
}
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Proposal, error) {
	if id == "" || strings.ContainsAny(id, "/\\.") {
		return Proposal{}, ErrNotFound
	}
	b, e := os.ReadFile(s.path(id))
	if errors.Is(e, os.ErrNotExist) {
		return Proposal{}, ErrNotFound
	}
	if e != nil {
		return Proposal{}, e
	}
	var p Proposal
	if json.Unmarshal(b, &p) != nil || p.ID != id {
		return Proposal{}, ErrNotFound
	}
	return p, nil
}
func (s *Store) write(p Proposal) error {
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.root, p.ID+"-*.tmp")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(tmp, s.path(p.ID))
	}
	if e != nil {
		return e
	}
	d, e := os.Open(s.root)
	if e != nil {
		return e
	}
	e = d.Sync()
	ce = d.Close()
	if e == nil {
		e = ce
	}
	return e
}
func (s *Store) lock() (func(), error) {
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		f.Close()
		return nil, e
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
func id() string { b := make([]byte, 16); rand.Read(b); return hex.EncodeToString(b) }
