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
var ErrConflict = errors.New("extension installation changed")

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
type CapabilityDecision struct {
	Capability string `json:"capability"`
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
}
type InstallationEvent struct {
	Type    string            `json:"type"`
	ActorID string            `json:"actor_id"`
	At      time.Time         `json:"at"`
	Details map[string]string `json:"details,omitempty"`
}
type Installation struct {
	ID                   string               `json:"id"`
	ExtensionID          string               `json:"extension_id"`
	ExtensionName        string               `json:"extension_name"`
	ExtensionVerifiedAt  time.Time            `json:"extension_verified_at"`
	OwnerType            string               `json:"owner_type"`
	OwnerID              string               `json:"owner_id"`
	RepositoryIDs        []string             `json:"repository_ids"`
	ResourceTypes        []string             `json:"resource_types"`
	CapabilityDecisions  []CapabilityDecision `json:"capability_decisions"`
	Settings             map[string]string    `json:"settings"`
	EffectiveAccess      []Permission         `json:"effective_access"`
	AuthorityEffectiveAt map[string]time.Time `json:"authority_effective_at"`
	DerivedCredentialIDs []string             `json:"derived_credential_ids"`
	Status               string               `json:"status"`
	Version              int                  `json:"version"`
	CreatedBy            string               `json:"created_by"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
	Events               []InstallationEvent  `json:"events"`
}
type InstallationInput struct {
	OwnerType           string               `json:"owner_type"`
	OwnerID             string               `json:"owner_id"`
	RepositoryIDs       []string             `json:"repository_ids"`
	ResourceTypes       []string             `json:"resource_types"`
	CapabilityDecisions []CapabilityDecision `json:"capability_decisions"`
	Settings            map[string]string    `json:"settings"`
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
	s := &Store{root: root, now: func() time.Time { return time.Now().UTC() }}
	if err := s.migrateLegacyAuthority(); err != nil {
		return nil, err
	}
	return s, nil
}

// Legacy installations were already authorized no later than their durable
// creation time. Persist that conservative boundary before delivery recovery
// is enabled, rather than treating a missing post-upgrade field as no access.
func (s *Store) migrateLegacyAuthority() error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "installation-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.root, entry.Name())
		var v Installation
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if json.Unmarshal(b, &v) != nil {
			return ErrInvalid
		}
		if len(v.AuthorityEffectiveAt) == 0 && v.Status == "active" {
			v.AuthorityEffectiveAt = authorityBoundaries(nil, v.RepositoryIDs, v.EffectiveAccess, v.CreatedAt, true)
			if err = writeAtomic(path, v); err != nil {
				return err
			}
		}
	}
	return nil
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

func (s *Store) CreateInstallation(extensionID, actor string, in InstallationInput) (Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Installation{}, err
	}
	defer unlock()
	extension, err := s.readExtension(extensionID)
	if err != nil {
		return Installation{}, err
	}
	if !validInstallation(in, extension) {
		return Installation{}, ErrInvalid
	}
	id, err := newID()
	if err != nil {
		return Installation{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	v := Installation{ID: id, ExtensionID: extension.ID, ExtensionName: extension.Name, ExtensionVerifiedAt: minTime(extension.CallbackEndpoint.VerifiedAt, extension.ActionEndpoint.VerifiedAt), OwnerType: in.OwnerType, OwnerID: in.OwnerID, RepositoryIDs: clean(in.RepositoryIDs), ResourceTypes: clean(in.ResourceTypes), CapabilityDecisions: in.CapabilityDecisions, Settings: copySettings(in.Settings), EffectiveAccess: effective(extension, in), DerivedCredentialIDs: []string{}, Status: "active", Version: 1, CreatedBy: actor, CreatedAt: now, UpdatedAt: now, Events: []InstallationEvent{{Type: "installation.created", ActorID: actor, At: now}}}
	v.AuthorityEffectiveAt = authorityBoundaries(nil, v.RepositoryIDs, v.EffectiveAccess, now, false)
	if err = writeAtomic(filepath.Join(s.root, "installation-"+id+".json"), v); err != nil {
		return Installation{}, err
	}
	return v, nil
}
func (s *Store) GetInstallation(id string) (Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readInstallation(id)
}

// RecordDerivedCredential attaches only a credential minted for this exact
// installation, allowing later suspension/removal to revoke it independently.
func (s *Store) RecordDerivedCredential(id, credentialID string, expected int) (Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Installation{}, err
	}
	defer unlock()
	v, err := s.readInstallation(id)
	if err != nil {
		return v, err
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	if len(credentialID) != 32 || v.Status != "active" {
		return v, ErrInvalid
	}
	for _, x := range v.DerivedCredentialIDs {
		if x == credentialID {
			return v, nil
		}
	}
	v.DerivedCredentialIDs = append(v.DerivedCredentialIDs, credentialID)
	v.Version++
	v.UpdatedAt = s.now().Truncate(time.Microsecond)
	if err = writeAtomic(filepath.Join(s.root, "installation-"+id+".json"), v); err != nil {
		return Installation{}, err
	}
	return v, nil
}
func (s *Store) ListInstallations(actor string) ([]Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Installation{}
	for _, x := range entries {
		if !strings.HasPrefix(x.Name(), "installation-") || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		var v Installation
		b, e := os.ReadFile(filepath.Join(s.root, x.Name()))
		if e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		if actor == "" || v.OwnerID == actor {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) ChangeInstallation(id, actor, action string, expected int, in *InstallationInput, revoke func(string) error) (Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Installation{}, err
	}
	defer unlock()
	v, err := s.readInstallation(id)
	if err != nil {
		return v, err
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	now := s.now().Truncate(time.Microsecond)
	switch action {
	case "suspend", "remove":
		for _, cid := range v.DerivedCredentialIDs {
			if revoke != nil {
				if err = revoke(cid); err != nil {
					return v, err
				}
			}
		}
		v.DerivedCredentialIDs = []string{}
		if action == "suspend" {
			v.Status = "suspended"
		} else {
			v.Status = "removed"
		}
	case "resume":
		if v.Status != "suspended" {
			return v, ErrInvalid
		}
		v.Status = "active"
		v.AuthorityEffectiveAt = authorityBoundaries(v.AuthorityEffectiveAt, v.RepositoryIDs, v.EffectiveAccess, now, true)
	case "upgrade":
		if in == nil {
			return v, ErrInvalid
		}
		ext, e := s.readExtension(v.ExtensionID)
		if e != nil {
			return v, e
		}
		if !validInstallation(*in, ext) {
			return v, ErrInvalid
		}
		v.RepositoryIDs = clean(in.RepositoryIDs)
		v.ResourceTypes = clean(in.ResourceTypes)
		v.CapabilityDecisions = in.CapabilityDecisions
		v.Settings = copySettings(in.Settings)
		v.EffectiveAccess = effective(ext, *in)
		v.AuthorityEffectiveAt = authorityBoundaries(v.AuthorityEffectiveAt, v.RepositoryIDs, v.EffectiveAccess, now, false)
		v.ExtensionName = ext.Name
		v.ExtensionVerifiedAt = minTime(ext.CallbackEndpoint.VerifiedAt, ext.ActionEndpoint.VerifiedAt)
	case "transfer":
		if in == nil || !((in.OwnerType == "repository" || in.OwnerType == "organization") && len(in.OwnerID) == 32) {
			return v, ErrInvalid
		}
		v.OwnerType, v.OwnerID = in.OwnerType, in.OwnerID
		if in.OwnerType == "repository" {
			v.RepositoryIDs = []string{in.OwnerID}
		} else {
			if len(in.RepositoryIDs) == 0 {
				return v, ErrInvalid
			}
			v.RepositoryIDs = clean(in.RepositoryIDs)
		}
		v.AuthorityEffectiveAt = authorityBoundaries(nil, v.RepositoryIDs, v.EffectiveAccess, now, true)
	default:
		return v, ErrInvalid
	}
	v.Version++
	v.UpdatedAt = now
	v.Events = append(v.Events, InstallationEvent{Type: "installation." + action, ActorID: actor, At: now})
	if err = writeAtomic(filepath.Join(s.root, "installation-"+id+".json"), v); err != nil {
		return Installation{}, err
	}
	return v, nil
}

func authorityBoundaries(existing map[string]time.Time, repositories []string, access []Permission, now time.Time, reset bool) map[string]time.Time {
	out := map[string]time.Time{}
	for _, repositoryID := range repositories {
		for _, permission := range access {
			if !contains(permission.Actions, "read") {
				continue
			}
			key := repositoryID + ":" + permission.Resource
			if !reset && !existing[key].IsZero() {
				out[key] = existing[key]
			} else {
				out[key] = now
			}
		}
	}
	return out
}
func (s *Store) readExtension(id string) (Extension, error) {
	var v Extension
	if len(id) != 32 {
		return v, ErrNotFound
	}
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
func (s *Store) readInstallation(id string) (Installation, error) {
	var v Installation
	if len(id) != 32 {
		return v, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, "installation-"+id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func validInstallation(in InstallationInput, e Extension) bool {
	if (in.OwnerType != "repository" && in.OwnerType != "organization") || len(in.OwnerID) != 32 || len(in.RepositoryIDs) == 0 || len(in.ResourceTypes) == 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, c := range e.Capabilities {
		allowed[c] = true
	}
	seen := map[string]bool{}
	for _, d := range in.CapabilityDecisions {
		if !allowed[d.Capability] || seen[d.Capability] || (d.Decision != "approved" && d.Decision != "denied") {
			return false
		}
		seen[d.Capability] = true
	}
	if len(seen) != len(allowed) {
		return false
	}
	for k, v := range in.Settings {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" || strings.Contains(strings.ToLower(k), "secret") || strings.Contains(strings.ToLower(k), "token") || strings.Contains(strings.ToLower(k), "password") {
			return false
		}
	}
	return true
}
func effective(e Extension, in InstallationInput) []Permission {
	approved := false
	for _, d := range in.CapabilityDecisions {
		approved = approved || d.Decision == "approved"
	}
	out := []Permission{}
	if !approved {
		return out
	}
	for _, p := range e.RequestedPermissions {
		for _, r := range in.ResourceTypes {
			if p.Resource == r {
				out = append(out, p)
				break
			}
		}
	}
	return out
}
func copySettings(v map[string]string) map[string]string {
	out := map[string]string{}
	for k, x := range v {
		out[strings.TrimSpace(k)] = strings.TrimSpace(x)
	}
	return out
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
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
	if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
		return e
	}
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
