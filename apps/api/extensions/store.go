// Package extensions persists external collaborator identities and contracts.
package extensions

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("extension not found")
var ErrInvalid = errors.New("invalid extension")

type Endpoint struct {
	URL        string    `json:"url"`
	VerifiedAt time.Time `json:"verified_at"`
}
type Permission struct {
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}
type RotationPolicy struct {
	IntervalDays int `json:"interval_days"`
	OverlapHours int `json:"overlap_hours"`
}
type AuthorityItem struct {
	Resource         string   `json:"resource"`
	RequestedActions []string `json:"requested_actions"`
	EffectiveActions []string `json:"effective_actions"`
	Decision         string   `json:"decision"`
	Reason           string   `json:"reason"`
}
type AuthorityPreview struct {
	Installed bool            `json:"installed"`
	Items     []AuthorityItem `json:"items"`
	Summary   string          `json:"summary"`
}

// Extension is a platform principal separate from its human owner and operator.
type Extension struct {
	ID                   string           `json:"id"`
	PrincipalType        string           `json:"principal_type"`
	Name                 string           `json:"name"`
	Description          string           `json:"description"`
	OwnerID              string           `json:"owner_id"`
	OperatorContact      string           `json:"operator_contact"`
	Capabilities         []string         `json:"capabilities"`
	CallbackEndpoint     Endpoint         `json:"callback_endpoint"`
	ActionEndpoint       Endpoint         `json:"action_endpoint"`
	RequestedPermissions []Permission     `json:"requested_permissions"`
	SupportedEvents      []string         `json:"supported_events"`
	CredentialRotation   RotationPolicy   `json:"credential_rotation"`
	AuthorityPreview     AuthorityPreview `json:"authority_preview"`
	Status               string           `json:"status"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

type Registration struct {
	Name, Description, OperatorContact string
	Capabilities                       []string
	CallbackURL, ActionURL             string
	RequestedPermissions               []Permission
	SupportedEvents                    []string
	CredentialRotation                 RotationPolicy
}
type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("extension storage root is empty")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Store) Create(owner string, in Registration, verified time.Time) (Extension, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Extension{}, err
	}
	defer unlock()
	in.Capabilities = clean(in.Capabilities)
	in.SupportedEvents = clean(in.SupportedEvents)
	if err = validate(owner, in); err != nil {
		return Extension{}, err
	}
	id, err := newID()
	if err != nil {
		return Extension{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	items := make([]AuthorityItem, len(in.RequestedPermissions))
	for i, p := range in.RequestedPermissions {
		items[i] = AuthorityItem{Resource: p.Resource, RequestedActions: p.Actions, EffectiveActions: []string{}, Decision: "not_installed", Reason: "registration declares requested authority; a resource owner must approve a future installation"}
	}
	v := Extension{ID: id, PrincipalType: "extension", Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), OwnerID: owner, OperatorContact: strings.TrimSpace(in.OperatorContact), Capabilities: in.Capabilities, CallbackEndpoint: Endpoint{URL: in.CallbackURL, VerifiedAt: verified}, ActionEndpoint: Endpoint{URL: in.ActionURL, VerifiedAt: verified}, RequestedPermissions: in.RequestedPermissions, SupportedEvents: in.SupportedEvents, CredentialRotation: in.CredentialRotation, AuthorityPreview: AuthorityPreview{Installed: false, Items: items, Summary: "No collaborative context or resource authority is granted before an owner-approved installation."}, Status: "registered", CreatedAt: now, UpdatedAt: now}
	if err = writeAtomic(filepath.Join(s.root, id+".json"), v); err != nil {
		return Extension{}, err
	}
	return v, nil
}
func (s *Store) Get(id string) (Extension, error) {
	if len(id) != 32 {
		return Extension{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var v Extension
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *Store) List(owner string) ([]Extension, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Extension{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var v Extension
		b, e := os.ReadFile(filepath.Join(s.root, entry.Name()))
		if e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		if v.OwnerID == owner {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func validate(owner string, in Registration) error {
	if owner == "" || len(strings.TrimSpace(in.Name)) < 2 || len(strings.TrimSpace(in.Name)) > 100 || len(strings.TrimSpace(in.OperatorContact)) < 3 || len(in.Capabilities) == 0 || len(in.SupportedEvents) == 0 || len(in.RequestedPermissions) == 0 || in.CredentialRotation.IntervalDays < 1 || in.CredentialRotation.IntervalDays > 365 || in.CredentialRotation.OverlapHours < 0 || in.CredentialRotation.OverlapHours > 168 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, p := range in.RequestedPermissions {
		if p.Resource == "" || len(p.Actions) == 0 || seen[p.Resource] {
			return ErrInvalid
		}
		seen[p.Resource] = true
		for _, a := range p.Actions {
			if strings.TrimSpace(a) == "" {
				return ErrInvalid
			}
		}
	}
	return nil
}
func clean(v []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, x := range v {
		x = strings.TrimSpace(x)
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
func newID() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
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
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
func writeAtomic(name string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(name), ".extension-*")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
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
		e = os.Rename(tmp.Name(), name)
	}
	if e == nil {
		directory, openErr := os.Open(filepath.Dir(name))
		if openErr != nil {
			e = openErr
		} else {
			e = directory.Sync()
			if closeErr := directory.Close(); e == nil {
				e = closeErr
			}
		}
	}
	return e
}
