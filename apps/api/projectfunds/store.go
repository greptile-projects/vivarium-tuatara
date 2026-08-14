// Package projectfunds persists governed repository funds and their append-only ledgers.
package projectfunds

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

var ErrNotFound = errors.New("project fund not found")
var ErrInvalid = errors.New("invalid project fund")
var ErrConflict = errors.New("project fund conflict")
var ErrForbidden = errors.New("project fund action forbidden")

type Limit struct {
	Period string `json:"period"`
	Amount int64  `json:"amount"`
}
type ApprovalRule struct {
	MinimumAmount     int64    `json:"minimum_amount"`
	RequiredApprovals int      `json:"required_approvals"`
	EligibleApprovers []string `json:"eligible_approvers"`
}
type Terms struct {
	Name               string         `json:"name"`
	Purpose            string         `json:"purpose"`
	Stewards           []string       `json:"stewards"`
	FundingSources     []string       `json:"accepted_funding_sources"`
	Unit               string         `json:"unit"`
	Precision          int            `json:"precision"`
	SpendingLimits     []Limit        `json:"spending_limits"`
	ApprovalRules      []ApprovalRule `json:"approval_rules"`
	EligibleRecipients []string       `json:"eligible_recipients"`
	RefundPolicy       string         `json:"refund_policy"`
	LedgerVisibility   string         `json:"ledger_visibility"`
}
type Balances struct {
	Available int64 `json:"available"`
	Reserved  int64 `json:"reserved"`
	Spent     int64 `json:"spent"`
	Refunded  int64 `json:"refunded"`
	Disputed  int64 `json:"disputed"`
	Pending   int64 `json:"pending"`
}
type Entry struct {
	ID                string    `json:"id"`
	Kind              string    `json:"kind"`
	Amount            int64     `json:"amount"`
	SpendableDelta    int64     `json:"spendable_delta"`
	Status            string    `json:"status"`
	Source            string    `json:"source"`
	ExternalReference string    `json:"external_reference"`
	IdempotencyKey    string    `json:"idempotency_key"`
	ContributorID     string    `json:"contributor_id"`
	ActorID           string    `json:"actor_id"`
	Note              string    `json:"note"`
	CreatedAt         time.Time `json:"created_at"`
}
type Fund struct {
	ID            string    `json:"id"`
	RepositoryID  string    `json:"repository_id"`
	Version       int       `json:"version"`
	Terms         Terms     `json:"terms"`
	Balances      Balances  `json:"balances"`
	Ledger        []Entry   `json:"ledger"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	AuthorityNote string    `json:"authority_note"`
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
func (s *Store) Create(repositoryID, actor string, terms Terms) (Fund, error) {
	var out Fund
	err := s.lock(func() error {
		if !validTerms(terms) {
			return ErrInvalid
		}
		now := s.now()
		out = Fund{ID: randomID(), RepositoryID: repositoryID, Version: 1, Terms: terms, CreatedBy: actor, CreatedAt: now, UpdatedAt: now, AuthorityNote: "Fund stewardship and balances grant no repository, Git, review, merge, deployment, or identity authority."}
		return s.write(out)
	})
	return out, err
}
func (s *Store) List(repositoryID string) ([]Fund, error) {
	var out []Fund
	err := s.lock(func() error {
		es, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, x := range es {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			f, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
			if e != nil {
				return e
			}
			if f.RepositoryID == repositoryID {
				out = append(out, f)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
		return nil
	})
	return out, err
}
func (s *Store) Get(id string) (Fund, error) {
	var f Fund
	err := s.lock(func() error { var e error; f, e = s.read(id); return e })
	return f, err
}

// Commit records intent as pending. Only a named steward can reconcile verified completion into value.
func (s *Store) Commit(id, contributor, source, external string, amount int64, key, note string) (Fund, error) {
	var out Fund
	err := s.lock(func() error {
		f, e := s.read(id)
		if e != nil {
			return e
		}
		if amount <= 0 || !contains(f.Terms.FundingSources, source) || strings.TrimSpace(key) == "" || strings.TrimSpace(external) == "" {
			return ErrInvalid
		}
		for _, v := range f.Ledger {
			if v.IdempotencyKey == key {
				return ErrConflict
			}
		}
		now := s.now()
		f.Version++
		f.UpdatedAt = now
		f.Ledger = append(f.Ledger, Entry{ID: randomID(), Kind: "commitment", Amount: amount, Status: "pending", Source: source, ExternalReference: external, IdempotencyKey: key, ContributorID: contributor, ActorID: contributor, Note: note, CreatedAt: now})
		f.Balances = derive(f.Ledger)
		out = f
		return s.write(f)
	})
	return out, err
}
func (s *Store) Reconcile(id, entryID, steward, status string, completed int64, note string, expected int) (Fund, error) {
	var out Fund
	err := s.lock(func() error {
		f, e := s.read(id)
		if e != nil {
			return e
		}
		if f.Version != expected {
			return ErrConflict
		}
		if !contains(f.Terms.Stewards, steward) {
			return ErrForbidden
		}
		idx := -1
		for i := range f.Ledger {
			if f.Ledger[i].ID == entryID && f.Ledger[i].Kind == "commitment" {
				idx = i
				break
			}
		}
		if idx < 0 {
			return ErrNotFound
		}
		original := f.Ledger[idx]
		if original.Status != "pending" {
			return ErrConflict
		}
		if !contains([]string{"settled", "partial", "failed", "revoked"}, status) || completed < 0 || completed > original.Amount || (status == "settled" && completed != original.Amount) || (status == "partial" && (completed == 0 || completed == original.Amount)) || ((status == "failed" || status == "revoked") && completed != 0) {
			return ErrInvalid
		}
		now := s.now()
		f.Ledger[idx].Status = status
		f.Ledger = append(f.Ledger, Entry{ID: randomID(), Kind: "transfer_reconciliation", Amount: completed, SpendableDelta: completed, Status: status, Source: original.Source, ExternalReference: original.ExternalReference, ContributorID: original.ContributorID, ActorID: steward, Note: note, CreatedAt: now})
		f.Version++
		f.UpdatedAt = now
		f.Balances = derive(f.Ledger)
		out = f
		return s.write(f)
	})
	return out, err
}
func derive(es []Entry) Balances {
	var b Balances
	for _, e := range es {
		if e.Kind == "commitment" && e.Status == "pending" {
			b.Pending += e.Amount
		}
		if e.Kind == "transfer_reconciliation" {
			b.Available += e.SpendableDelta
		}
	}
	return b
}
func validTerms(t Terms) bool {
	return strings.TrimSpace(t.Name) != "" && strings.TrimSpace(t.Purpose) != "" && len(t.Stewards) > 0 && len(t.FundingSources) > 0 && strings.TrimSpace(t.Unit) != "" && t.Precision >= 0 && t.Precision <= 8 && len(t.SpendingLimits) > 0 && len(t.ApprovalRules) > 0 && len(t.EligibleRecipients) > 0 && strings.TrimSpace(t.RefundPolicy) != "" && contains([]string{"public", "participants"}, t.LedgerVisibility) && allPositive(t.SpendingLimits, t.ApprovalRules)
}
func allPositive(ls []Limit, rs []ApprovalRule) bool {
	for _, v := range ls {
		if strings.TrimSpace(v.Period) == "" || v.Amount <= 0 {
			return false
		}
	}
	for _, v := range rs {
		if v.MinimumAmount < 0 || v.RequiredApprovals < 1 || len(v.EligibleApprovers) < v.RequiredApprovals {
			return false
		}
	}
	return true
}
func contains(vs []string, v string) bool {
	for _, x := range vs {
		if x == v {
			return true
		}
	}
	return false
}
func randomID() string                 { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Fund, error) {
	var f Fund
	b, e := os.ReadFile(s.path(id))
	if os.IsNotExist(e) {
		return f, ErrNotFound
	}
	if e != nil {
		return f, e
	}
	e = json.Unmarshal(b, &f)
	return f, e
}
func (s *Store) write(f Fund) error {
	b, e := json.MarshalIndent(f, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, "fund-*.tmp")
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
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, s.path(f.ID))
	}
	return e
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lf, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lf.Close()
	if e = syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
	return fn()
}
