// Package responsealerts retains revision-bound urgent signals and their delivery ledger.
package responsealerts

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

	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsepolicies"
)

var ErrNotFound = errors.New("response alert not found")
var ErrInvalid = errors.New("invalid response alert")
var ErrConflict = errors.New("response alert conflict")
var ErrForbidden = errors.New("response alert forbidden")

type Evidence struct {
	Kind         string   `json:"kind"`
	ResourceID   string   `json:"resource_id"`
	Revision     string   `json:"revision"`
	Digest       string   `json:"digest"`
	URL          string   `json:"url,omitempty"`
	Summary      string   `json:"summary"`
	AccessibleTo []string `json:"accessible_to,omitempty"`
	Available    bool     `json:"available"`
}
type Signal struct {
	SignalClass        string     `json:"signal_class"`
	Severity           string     `json:"severity"`
	ResourceIDs        []string   `json:"resource_ids"`
	AffectedUserCount  int        `json:"affected_user_count"`
	AffectedUserGroups []string   `json:"affected_user_groups"`
	Summary            string     `json:"summary"`
	Uncertainty        string     `json:"uncertainty"`
	OccurredAt         time.Time  `json:"occurred_at"`
	SourceRevision     string     `json:"source_revision"`
	CorrelationKey     string     `json:"correlation_key"`
	Evidence           []Evidence `json:"evidence"`
	SuppressionKey     string     `json:"suppression_key,omitempty"`
	SuppressedUntil    *time.Time `json:"suppressed_until,omitempty"`
	MaintenanceEndsAt  *time.Time `json:"maintenance_ends_at,omitempty"`
	RateLimitSeconds   int        `json:"rate_limit_seconds,omitempty"`
}
type Delivery struct {
	ID          string    `json:"id"`
	RecipientID string    `json:"recipient_id"`
	Channel     string    `json:"channel"`
	Status      string    `json:"status"`
	AttemptedAt time.Time `json:"attempted_at"`
	Failure     string    `json:"failure,omitempty"`
}
type Event struct {
	ID        string    `json:"id"`
	RequestID string    `json:"request_id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Alert struct {
	ID                string     `json:"id"`
	RepositoryID      string     `json:"repository_id"`
	RequestID         string     `json:"request_id"`
	RequestDigest     string     `json:"request_digest"`
	PolicyID          string     `json:"policy_id"`
	PolicyVersion     int        `json:"policy_version"`
	RuleID            string     `json:"rule_id"`
	TeamID            string     `json:"team_id"`
	Signal            Signal     `json:"signal"`
	FirstSeenAt       time.Time  `json:"first_seen_at"`
	LastSeenAt        time.Time  `json:"last_seen_at"`
	EventCount        int        `json:"event_count"`
	AcknowledgeBy     time.Time  `json:"acknowledge_by"`
	ResolveBy         time.Time  `json:"resolve_by"`
	State             string     `json:"state"`
	Routing           []Delivery `json:"routing"`
	Events            []Event    `json:"events"`
	Diagnostics       []string   `json:"diagnostics"`
	ExpectedActions   []string   `json:"expected_actions"`
	PermittedActions  []string   `json:"permitted_actions"`
	ProhibitedActions []string   `json:"prohibited_actions"`
	AudienceIDs       []string   `json:"audience_ids"`
	UpdatedAt         time.Time  `json:"updated_at"`
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

func (s *Store) Create(repo, actor, request string, signal Signal, policy responsepolicies.Policy, recipients []string) (Alert, error) {
	var out Alert
	err := s.lock(func() error {
		if request == "" || validateSignal(signal) != nil || len(policy.Revisions) == 0 {
			return ErrInvalid
		}
		digest := digest(signal)
		id := stable(repo, actor, request)
		if old, e := s.read(id); e == nil {
			if old.RequestDigest != digest {
				return ErrConflict
			}
			out = old
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		rev := policy.Revisions[len(policy.Revisions)-1]
		var rule *responsepolicies.Rule
		for i := range rev.Rules {
			r := &rev.Rules[i]
			if r.SignalClass == signal.SignalClass && r.Severity == signal.Severity && intersects(r.ResourceIDs, signal.ResourceIDs) {
				if rule != nil {
					return ErrInvalid
				}
				rule = r
			}
		}
		if rule == nil {
			return ErrInvalid
		}
		now := s.now()
		state := "open"
		diagnostics := []string{}
		if signal.SuppressedUntil != nil && signal.SuppressedUntil.After(now) {
			state = "suppressed"
			diagnostics = append(diagnostics, "suppressed")
		}
		if signal.MaintenanceEndsAt != nil && signal.MaintenanceEndsAt.After(now) {
			state = "maintenance"
			diagnostics = append(diagnostics, "maintenance_window")
		}
		for _, e := range signal.Evidence {
			if !e.Available {
				diagnostics = append(diagnostics, "inaccessible_evidence")
			}
		}
		if signal.OccurredAt.Before(now.Add(-24 * time.Hour)) {
			diagnostics = append(diagnostics, "stale_signal")
		}
		routing := []Delivery{}
		if state == "open" {
			for _, recipient := range unique(recipients) {
				routing = append(routing, Delivery{ID: stable(id, recipient, "web"), RecipientID: recipient, Channel: "web", Status: "delivered", AttemptedAt: now})
			}
			if len(routing) == 0 {
				diagnostics = append(diagnostics, "delivery_failed")
			}
		}
		out = Alert{ID: id, RepositoryID: repo, RequestID: request, RequestDigest: digest, PolicyID: policy.ID, PolicyVersion: rev.Version, RuleID: rule.ID, TeamID: rule.AccountableTeamID, Signal: signal, FirstSeenAt: now, LastSeenAt: now, EventCount: 1, AcknowledgeBy: now.Add(time.Duration(rule.AcknowledgeSeconds) * time.Second), ResolveBy: now.Add(time.Duration(rule.ResolveSeconds) * time.Second), State: state, Routing: routing, Events: []Event{}, Diagnostics: diagnostics, ExpectedActions: rule.ExpectedActions, PermittedActions: rule.Authority.PermittedActions, ProhibitedActions: rule.Authority.ProhibitedActions, AudienceIDs: rule.CommunicationAudienceIDs, UpdatedAt: now}
		// Correlated events join the existing open alert without producing another page.
		values, _ := s.listUnlocked(repo)
		for _, v := range values {
			if v.Signal.CorrelationKey != "" && v.Signal.CorrelationKey == signal.CorrelationKey && v.RuleID == rule.ID && v.State == "open" && v.PolicyID == policy.ID && v.PolicyVersion == rev.Version && v.TeamID == rule.AccountableTeamID {
				for _, event := range v.Events {
					if event.Kind == "correlated" && event.RequestID == request {
						if event.Reason != digest {
							return ErrConflict
						}
						out = v
						return nil
					}
				}
				v.LastSeenAt = now
				v.EventCount++
				v.Events = append(v.Events, Event{ID: stable(v.ID, actor, request), RequestID: request, Kind: "correlated", ActorID: actor, Reason: digest, CreatedAt: now})
				if signal.RateLimitSeconds > 0 && now.Sub(v.UpdatedAt) < time.Duration(signal.RateLimitSeconds)*time.Second && !containsString(v.Diagnostics, "rate_limited") {
					v.Diagnostics = append(v.Diagnostics, "rate_limited")
				}
				v.UpdatedAt = now
				if severityRank(signal.Severity) > severityRank(v.Signal.Severity) {
					v.Signal.Severity = signal.Severity
				}
				out = v
				return s.write(v)
			}
		}
		return s.write(out)
	})
	return out, err
}

func (s *Store) Append(id, request, kind, actor, reason string, allowed bool) (Alert, error) {
	var out Alert
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		for _, x := range v.Events {
			if x.RequestID == request {
				if x.Kind != kind || x.ActorID != actor || x.Reason != reason {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if request == "" || !allowed || (kind != "acknowledge" && kind != "resolve") {
			return ErrForbidden
		}
		now := s.now()
		v.Events = append(v.Events, Event{ID: stable(id, actor, request), RequestID: request, Kind: kind, ActorID: actor, Reason: reason, CreatedAt: now})
		v.State = map[string]string{"acknowledge": "acknowledged", "resolve": "resolved"}[kind]
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return out, err
}
func (s *Store) Get(id string) (Alert, error) {
	var v Alert
	e := s.lock(func() error { var x error; v, x = s.read(id); return x })
	return v, e
}
func (s *Store) List(repo string) ([]Alert, error) {
	var v []Alert
	e := s.lock(func() error { var x error; v, x = s.listUnlocked(repo); return x })
	return v, e
}
func (s *Store) listUnlocked(repo string) ([]Alert, error) {
	out := []Alert{}
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		v, x := s.read(strings.TrimSuffix(entry.Name(), ".json"))
		if x != nil {
			return nil, x
		}
		if v.RepositoryID == repo {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func validateSignal(v Signal) error {
	valid := map[string]bool{"reliability": true, "deployment": true, "security": true, "privacy": true, "dependency": true, "workflow": true, "user_impact": true}
	sev := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	if !valid[v.SignalClass] || !sev[v.Severity] || len(v.ResourceIDs) == 0 || v.Summary == "" || v.Uncertainty == "" || v.OccurredAt.IsZero() || v.SourceRevision == "" || len(v.Evidence) == 0 || v.AffectedUserCount < 0 {
		return ErrInvalid
	}
	for _, e := range v.Evidence {
		if e.Kind == "" || e.ResourceID == "" || e.Revision == "" || e.Digest == "" || e.Summary == "" {
			return ErrInvalid
		}
	}
	return nil
}
func intersects(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
func unique(v []string) []string {
	m := map[string]bool{}
	o := []string{}
	for _, x := range v {
		if x != "" && !m[x] {
			m[x] = true
			o = append(o, x)
		}
	}
	return o
}
func severityRank(v string) int {
	return map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}[v]
}
func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func digest(v Signal) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func stable(v ...string) string {
	h := sha256.Sum256([]byte(strings.Join(v, "\x00")))
	return hex.EncodeToString(h[:16])
}
func (s *Store) read(id string) (Alert, error) {
	var v Alert
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Alert) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".alert-")
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
	if x := tmp.Close(); e == nil {
		e = x
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
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
