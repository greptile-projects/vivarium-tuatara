// Package responsepolicies persists immutable, versioned urgent-response coverage.
package responsepolicies

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

var ErrNotFound = errors.New("response policy not found")
var ErrInvalid = errors.New("invalid response policy")
var ErrConflict = errors.New("response policy version conflict")
var ErrCommitted = errors.New("response policy may have committed")
var ErrForbidden = errors.New("response policy action forbidden")

type Resource struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Name            string   `json:"name"`
	OwnerTeamIDs    []string `json:"owner_team_ids"`
	PrivacyClass    string   `json:"privacy_class,omitempty"`
	SecurityClass   string   `json:"security_class,omitempty"`
	ContinuityClass string   `json:"continuity_class,omitempty"`
}
type Team struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	OrganizationID string   `json:"organization_id,omitempty"`
	MemberIDs      []string `json:"member_ids"`
	Skills         []string `json:"skills"`
	Contact        string   `json:"contact"`
}
type Escalation struct {
	AfterSeconds   int      `json:"after_seconds"`
	TeamID         string   `json:"team_id"`
	AudienceIDs    []string `json:"audience_ids"`
	ExpectedAction string   `json:"expected_action"`
}
type AuthorityBoundary struct {
	RequiredAccess    []string `json:"required_access"`
	PermittedActions  []string `json:"permitted_actions"`
	ProhibitedActions []string `json:"prohibited_actions"`
	PrivacyRuleIDs    []string `json:"privacy_rule_ids"`
	SecurityRuleIDs   []string `json:"security_rule_ids"`
	ContinuityRuleIDs []string `json:"continuity_rule_ids"`
}
type Rule struct {
	ID                       string            `json:"id"`
	ResourceIDs              []string          `json:"resource_ids"`
	SignalClass              string            `json:"signal_class"`
	Severity                 string            `json:"severity"`
	AccountableTeamID        string            `json:"accountable_team_id"`
	RequiredSkills           []string          `json:"required_skills"`
	AcknowledgeSeconds       int               `json:"acknowledge_seconds"`
	ResolveSeconds           int               `json:"resolve_seconds"`
	ExpectedActions          []string          `json:"expected_actions"`
	Escalations              []Escalation      `json:"escalations"`
	CommunicationAudienceIDs []string          `json:"communication_audience_ids"`
	IncidentCriteria         []string          `json:"incident_criteria"`
	Authority                AuthorityBoundary `json:"authority"`
	AttributedTo             string            `json:"attributed_to,omitempty"`
}
type Exception struct {
	ID         string    `json:"id"`
	RuleID     string    `json:"rule_id"`
	Reason     string    `json:"reason"`
	FollowUpID string    `json:"follow_up_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	GrantedBy  string    `json:"granted_by,omitempty"`
}
type Revision struct {
	RequestID    string      `json:"request_id,omitempty"`
	Version      int         `json:"version,omitempty"`
	Title        string      `json:"title"`
	Summary      string      `json:"summary"`
	Resources    []Resource  `json:"resources"`
	Teams        []Team      `json:"teams"`
	Rules        []Rule      `json:"rules"`
	Exceptions   []Exception `json:"exceptions"`
	ChangeReason string      `json:"change_reason"`
	CreatedBy    string      `json:"created_by,omitempty"`
	CreatedAt    time.Time   `json:"created_at,omitempty"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	ResourceID   string `json:"resource_id,omitempty"`
	RuleID       string `json:"rule_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type Policy struct {
	ID             string       `json:"id"`
	RepositoryID   string       `json:"repository_id"`
	RequestID      string       `json:"request_id"`
	RequestDigest  string       `json:"request_digest"`
	CurrentVersion int          `json:"current_version"`
	Revisions      []Revision   `json:"revisions"`
	Diagnostics    []Diagnostic `json:"diagnostics"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}
type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if blank(root) {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}
func (s *Store) Create(repositoryID, actor, requestID string, r Revision) (Policy, error) {
	var out Policy
	err := s.lock(func() error {
		if blank(requestID) || validate(r) != nil {
			return ErrInvalid
		}
		digest := revisionDigest(r)
		id := stableID(repositoryID, actor, requestID)
		if old, e := s.read(id); e == nil {
			if old.RequestDigest != digest {
				return ErrConflict
			}
			out = old
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		now := s.now()
		stamp(&r, actor, requestID, 1, now)
		out = Policy{ID: id, RepositoryID: repositoryID, RequestID: requestID, RequestDigest: digest, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.project(out), err
}
func (s *Store) Revise(id string, expected int, actor, requestID string, r Revision) (Policy, error) {
	var out Policy
	err := s.lock(func() error {
		if blank(requestID) {
			return ErrInvalid
		}
		v, e := s.read(id)
		if e != nil {
			return e
		}
		digest := revisionDigest(r)
		for _, old := range v.Revisions {
			if old.RequestID == requestID {
				if revisionDigest(old) != digest {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validate(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, actor, requestID, expected+1, now)
		v.CurrentVersion++
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return s.project(out), err
}
func (s *Store) Get(id string) (Policy, error) {
	var v Policy
	e := s.lock(func() error { var x error; v, x = s.read(id); return x })
	return s.project(v), e
}
func (s *Store) List(repositoryID string) ([]Policy, error) {
	values := []Policy{}
	e := s.lock(func() error {
		entries, x := os.ReadDir(s.root)
		if x != nil {
			return x
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			v, x := s.read(strings.TrimSuffix(entry.Name(), ".json"))
			if x != nil {
				return x
			}
			if v.RepositoryID == repositoryID {
				values = append(values, s.project(v))
			}
		}
		return nil
	})
	sort.Slice(values, func(i, j int) bool { return values[i].UpdatedAt.After(values[j].UpdatedAt) })
	return values, e
}

func stamp(r *Revision, actor, request string, version int, now time.Time) {
	r.RequestID = request
	r.Version = version
	r.CreatedBy = actor
	r.CreatedAt = now
	for i := range r.Rules {
		r.Rules[i].AttributedTo = actor
	}
	for i := range r.Exceptions {
		r.Exceptions[i].GrantedBy = actor
	}
}
func (s *Store) project(v Policy) Policy {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	teams := map[string]Team{}
	for _, t := range r.Teams {
		teams[t.ID] = t
	}
	coverage := map[string]bool{}
	owners := map[string]string{}
	d := []Diagnostic{}
	for _, rule := range r.Rules {
		team := teams[rule.AccountableTeamID]
		skillSet := map[string]bool{}
		for _, x := range team.Skills {
			skillSet[x] = true
		}
		for _, skill := range rule.RequiredSkills {
			if !skillSet[skill] {
				d = append(d, diag("unavailable_skill", "blocking", "The accountable team does not declare a required response skill.", "", rule.ID, rule.AttributedTo))
			}
		}
		if rule.AcknowledgeSeconds <= 0 || rule.ResolveSeconds <= rule.AcknowledgeSeconds {
			d = append(d, diag("impossible_target", "blocking", "Response targets must allow acknowledgement before resolution.", "", rule.ID, rule.AttributedTo))
		}
		for _, x := range rule.Escalations {
			if x.AfterSeconds <= rule.AcknowledgeSeconds || x.AfterSeconds >= rule.ResolveSeconds {
				d = append(d, diag("impossible_target", "blocking", "Escalation must occur after acknowledgement and before resolution.", "", rule.ID, rule.AttributedTo))
			}
		}
		for _, resource := range rule.ResourceIDs {
			key := resource + "\x00" + rule.SignalClass + "\x00" + rule.Severity
			coverage[resource] = true
			if prior := owners[key]; prior != "" && prior != rule.AccountableTeamID {
				d = append(d, diag("conflicting_ownership", "blocking", "The same urgent condition names different accountable teams.", resource, rule.ID, rule.AttributedTo))
			} else {
				owners[key] = rule.AccountableTeamID
			}
		}
	}
	for _, resource := range r.Resources {
		if !coverage[resource.ID] {
			d = append(d, diag("uncovered_resource", "blocking", "The resource has no urgent-condition response rule.", resource.ID, "", r.CreatedBy))
		}
		for _, rule := range r.Rules {
			if contains(rule.ResourceIDs, resource.ID) && len(resource.OwnerTeamIDs) > 0 && !contains(resource.OwnerTeamIDs, rule.AccountableTeamID) {
				d = append(d, diag("conflicting_ownership", "warning", "Response accountability conflicts with declared service ownership.", resource.ID, rule.ID, rule.AttributedTo))
			}
		}
	}
	now := s.now()
	for _, x := range r.Exceptions {
		if !x.ExpiresAt.After(now) {
			d = append(d, diag("expired_exception", "blocking", "The response-policy exception has expired.", "", x.RuleID, x.GrantedBy))
		} else if x.ExpiresAt.Before(now.Add(30 * 24 * time.Hour)) {
			d = append(d, diag("expiring_exception", "warning", "The response-policy exception expires within 30 days.", "", x.RuleID, x.GrantedBy))
		}
	}
	v.Diagnostics = d
	return v
}
func diag(kind, severity, message, resource, rule, actor string) Diagnostic {
	return Diagnostic{Kind: kind, Severity: severity, Message: message, ResourceID: resource, RuleID: rule, AttributedTo: actor}
}
func validate(r Revision) error {
	if blank(r.Title) || len(r.Resources) == 0 || len(r.Teams) == 0 || len(r.Rules) == 0 || blank(r.ChangeReason) {
		return ErrInvalid
	}
	resourceIDs := map[string]bool{}
	validKinds := map[string]bool{"repository": true, "service": true, "environment": true, "user_journey": true, "dependency": true}
	for _, x := range r.Resources {
		if blank(x.ID) || resourceIDs[x.ID] || !validKinds[x.Kind] || blank(x.Name) {
			return ErrInvalid
		}
		resourceIDs[x.ID] = true
	}
	teamIDs := map[string]bool{}
	for _, x := range r.Teams {
		if blank(x.ID) || teamIDs[x.ID] || blank(x.Name) || len(x.MemberIDs) == 0 || blank(x.Contact) {
			return ErrInvalid
		}
		teamIDs[x.ID] = true
	}
	ruleIDs := map[string]bool{}
	severities := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	for _, x := range r.Rules {
		if blank(x.ID) || ruleIDs[x.ID] || len(x.ResourceIDs) == 0 || blank(x.SignalClass) || !severities[x.Severity] || !teamIDs[x.AccountableTeamID] || len(x.ExpectedActions) == 0 || len(x.CommunicationAudienceIDs) == 0 || len(x.IncidentCriteria) == 0 || len(x.Authority.RequiredAccess) == 0 || len(x.Authority.PermittedActions) == 0 || len(x.Authority.ProhibitedActions) == 0 {
			return ErrInvalid
		}
		for _, id := range x.ResourceIDs {
			if !resourceIDs[id] {
				return ErrInvalid
			}
		}
		for _, e := range x.Escalations {
			if !teamIDs[e.TeamID] || len(e.AudienceIDs) == 0 || blank(e.ExpectedAction) {
				return ErrInvalid
			}
		}
		ruleIDs[x.ID] = true
	}
	for _, x := range r.Exceptions {
		if blank(x.ID) || !ruleIDs[x.RuleID] || blank(x.Reason) || blank(x.FollowUpID) || x.ExpiresAt.IsZero() {
			return ErrInvalid
		}
	}
	return nil
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func blank(v string) bool { return strings.TrimSpace(v) == "" }
func revisionDigest(v Revision) string {
	v.RequestID = ""
	v.Version = 0
	v.CreatedBy = ""
	v.CreatedAt = time.Time{}
	for i := range v.Rules {
		v.Rules[i].AttributedTo = ""
	}
	for i := range v.Exceptions {
		v.Exceptions[i].GrantedBy = ""
	}
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func stableID(repo, actor, request string) string {
	sum := sha256.Sum256([]byte(repo + "\x00" + actor + "\x00" + request))
	return hex.EncodeToString(sum[:16])
}
func (s *Store) read(id string) (Policy, error) {
	var v Policy
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
func (s *Store) write(v Policy) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".response-")
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
	closeErr := tmp.Close()
	if e == nil {
		e = closeErr
	}
	renamed := false
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
		renamed = e == nil
	}
	if e == nil {
		dir, x := os.Open(s.root)
		if x != nil {
			return x
		}
		e = dir.Sync()
		_ = dir.Close()
	}
	if e != nil && renamed {
		return errors.Join(ErrCommitted, e)
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
