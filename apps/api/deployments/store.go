// Package deployments persists governed release environments and promotions.
package deployments

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
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

const ConfigPath = ".vivarium/deployment.json"

func ParseRolloutDefinition(body []byte) (RolloutDefinition, error) {
	var value RolloutDefinition
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || !validRollout(value) {
		return value, ErrInvalid
	}
	return value, nil
}

var (
	ErrNotFound = errors.New("deployment resource not found")
	ErrInvalid  = errors.New("invalid deployment resource")
	ErrBlocked  = errors.New("deployment transition blocked")
)

type Environment struct {
	ID                string            `json:"id"`
	RepositoryID      string            `json:"repository_id"`
	Name              string            `json:"name"`
	Position          int               `json:"position"`
	Image             string            `json:"image"`
	Command           string            `json:"command"`
	TimeoutSeconds    int               `json:"timeout_seconds"`
	Configuration     map[string]string `json:"configuration"`
	CredentialNames   []string          `json:"credential_names"`
	RequiredApprovals int               `json:"required_approvals"`
	Concurrency       int               `json:"concurrency"`
	CreatedBy         string            `json:"created_by"`
	UpdatedBy         string            `json:"updated_by"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	Credentials       map[string]string `json:"-"`
}

// RolloutDefinition is frozen from .vivarium/deployment.json at the release
// commit. It therefore remains reviewable even when the repository advances.
type RolloutDefinition struct {
	Version int            `json:"version"`
	Stages  []RolloutStage `json:"stages"`
}
type RolloutStage struct {
	Name               string         `json:"name"`
	ObservationSeconds int            `json:"observation_seconds"`
	Signals            []HealthSignal `json:"signals"`
}
type HealthSignal struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}
type SignalEvidence struct {
	Stage     string    `json:"stage"`
	Signal    string    `json:"signal"`
	State     string    `json:"state"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Approval struct {
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Event struct {
	Sequence  int       `json:"sequence"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id,omitempty"`
	State     string    `json:"state,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Promotion struct {
	ID                   string            `json:"id"`
	RepositoryID         string            `json:"repository_id"`
	EnvironmentID        string            `json:"environment_id"`
	ReleaseID            string            `json:"release_id"`
	BuildID              string            `json:"build_id"`
	ArtifactID           string            `json:"artifact_id"`
	ArtifactSHA256       string            `json:"artifact_sha256"`
	CommitID             string            `json:"commit_id"`
	Rollout              RolloutDefinition `json:"rollout"`
	CurrentStage         int               `json:"current_stage"`
	Evidence             []SignalEvidence  `json:"evidence"`
	State                string            `json:"state"`
	InitiatedBy          string            `json:"initiated_by"`
	Approvals            []Approval        `json:"approvals"`
	Events               []Event           `json:"events"`
	CreatedAt            time.Time         `json:"created_at"`
	StartedAt            *time.Time        `json:"started_at,omitempty"`
	CompletedAt          *time.Time        `json:"completed_at,omitempty"`
	ExecutionOwner       string            `json:"execution_owner,omitempty"`
	LeaseExpiresAt       *time.Time        `json:"lease_expires_at,omitempty"`
	RecoveryOf           string            `json:"recovery_of,omitempty"`
	RecoveryKind         string            `json:"recovery_kind,omitempty"`
	RestoresDeploymentID string            `json:"restores_deployment_id,omitempty"`
}

type Store struct {
	root string
	key  []byte
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(root, ".credential-key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err == nil {
			err = os.WriteFile(keyPath, key, 0600)
		}
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("deployment credential key unavailable")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: abs, key: key, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}

func (s *Store) PutEnvironment(value Environment) (Environment, error) {
	if !validID(value.RepositoryID) || !validID(value.UpdatedBy) || strings.TrimSpace(value.Name) == "" || len(value.Name) > 100 || value.Position < 1 || !validImage(value.Image) || strings.TrimSpace(value.Command) == "" || len(value.Command) > 4000 || value.TimeoutSeconds < 1 || value.TimeoutSeconds > 3600 || value.RequiredApprovals < 0 || value.RequiredApprovals > 20 || value.Concurrency < 1 || value.Concurrency > 20 || len(value.Configuration) > 50 || len(value.Credentials) > 50 {
		return Environment{}, ErrInvalid
	}
	for k, v := range value.Configuration {
		if !validName(k) || len(v) > 4000 {
			return Environment{}, ErrInvalid
		}
	}
	for k, v := range value.Credentials {
		if !validName(k) || v == "" || len(v) > 16000 {
			return Environment{}, ErrInvalid
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Environment{}, err
	}
	defer unlock()
	now := s.now()
	existing, getErr := s.getEnvironment(value.RepositoryID, value.ID)
	if value.ID == "" {
		value.ID, err = newID()
		if err != nil {
			return Environment{}, err
		}
		value.CreatedAt = now
		value.CreatedBy = value.UpdatedBy
	} else if getErr != nil {
		return Environment{}, ErrNotFound
	} else {
		value.CreatedAt = existing.CreatedAt
		value.CreatedBy = existing.CreatedBy
		if value.Credentials == nil {
			value.Credentials = existing.Credentials
		}
	}
	value.Name = strings.TrimSpace(value.Name)
	value.UpdatedAt = now
	value.CredentialNames = keys(value.Credentials)
	environments, err := s.listEnvironments(value.RepositoryID)
	if err != nil {
		return Environment{}, err
	}
	for _, env := range environments {
		if env.ID != value.ID && (strings.EqualFold(env.Name, value.Name) || env.Position == value.Position) {
			return Environment{}, ErrInvalid
		}
	}
	return public(value), s.write(filepath.Join(s.root, value.RepositoryID, "environments", value.ID+".json"), value)
}

func (s *Store) GetEnvironment(repo, id string) (Environment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, err := s.getEnvironment(repo, id)
	return public(value), err
}

// ExecutionEnvironment is reserved for the in-process executor. HTTP reads
// use GetEnvironment, which always removes protected values.
func (s *Store) ExecutionEnvironment(repo, id string) (Environment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getEnvironment(repo, id)
}
func (s *Store) getEnvironment(repo, id string) (Environment, error) {
	var v Environment
	if !validID(repo) || !validID(id) {
		return v, ErrNotFound
	}
	err := s.read(filepath.Join(s.root, repo, "environments", id+".json"), &v)
	if err != nil {
		return v, err
	}
	return v, nil
}
func (s *Store) ListEnvironments(repo string) ([]Environment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.listEnvironments(repo)
	for i := range values {
		values[i] = public(values[i])
	}
	return values, err
}
func (s *Store) listEnvironments(repo string) ([]Environment, error) {
	var values []Environment
	if !validID(repo) {
		return nil, ErrNotFound
	}
	err := s.list(filepath.Join(s.root, repo, "environments"), &values)
	sort.Slice(values, func(i, j int) bool { return values[i].Position < values[j].Position })
	return values, err
}

func (s *Store) CreatePromotion(value Promotion) (Promotion, error) {
	legacyDefinition := value.CommitID == "" && value.Rollout.Version == 0 && len(value.Rollout.Stages) == 0
	if !validID(value.RepositoryID) || !validID(value.EnvironmentID) || !validID(value.ReleaseID) || !validID(value.BuildID) || !validID(value.ArtifactID) || !validID(value.InitiatedBy) || len(value.ArtifactSHA256) != 64 || (!legacyDefinition && (len(value.CommitID) != 40 || !validRollout(value.Rollout))) || (value.RecoveryOf != "" && (!validID(value.RecoveryOf) || value.RecoveryKind != "rollback" || !validID(value.RestoresDeploymentID))) {
		return Promotion{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Promotion{}, err
	}
	defer unlock()
	env, err := s.getEnvironment(value.RepositoryID, value.EnvironmentID)
	if err != nil {
		return Promotion{}, err
	}
	promotions, err := s.listPromotions(value.RepositoryID)
	if err != nil {
		return Promotion{}, err
	}
	active := 0
	for _, p := range promotions {
		if p.EnvironmentID == env.ID && (p.State == "pending_approval" || p.State == "queued" || p.State == "running") {
			active++
		}
	}
	if active >= env.Concurrency {
		return Promotion{}, ErrBlocked
	}
	if env.Position > 1 {
		ok := false
		for _, p := range promotions {
			if p.ReleaseID == value.ReleaseID && p.BuildID == value.BuildID && p.ArtifactID == value.ArtifactID && p.ArtifactSHA256 == value.ArtifactSHA256 && p.State == "succeeded" {
				prior, e := s.getEnvironment(value.RepositoryID, p.EnvironmentID)
				if e == nil && prior.Position == env.Position-1 {
					ok = true
				}
			}
		}
		if !ok {
			return Promotion{}, ErrBlocked
		}
	}
	value.ID, err = newID()
	if err != nil {
		return Promotion{}, err
	}
	value.State = "pending_approval"
	value.CreatedAt = s.now()
	value.Approvals = []Approval{}
	value.Events = []Event{{Sequence: 1, Kind: "promotion.requested", ActorID: value.InitiatedBy, State: value.State, CreatedAt: value.CreatedAt}}
	if env.RequiredApprovals == 0 {
		value.State = "queued"
		value.Events = append(value.Events, Event{Sequence: 2, Kind: "promotion.queued", ActorID: value.InitiatedBy, State: value.State, CreatedAt: value.CreatedAt})
	}
	return value, s.write(filepath.Join(s.root, value.RepositoryID, "promotions", value.ID+".json"), value)
}

// RollbackTarget derives the newest successful deployment to the same
// environment that predates an unhealthy deployment.
func (s *Store) RollbackTarget(repo, id string) (Promotion, Promotion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	failed, err := s.getPromotion(repo, id)
	if err != nil {
		return Promotion{}, Promotion{}, err
	}
	if failed.State != "failed" && failed.State != "canceled" {
		return failed, Promotion{}, ErrBlocked
	}
	items, err := s.listPromotions(repo)
	if err != nil {
		return failed, Promotion{}, err
	}
	var target Promotion
	for _, candidate := range items {
		if candidate.EnvironmentID == failed.EnvironmentID && candidate.State == "succeeded" && candidate.CreatedAt.Before(failed.CreatedAt) && (target.ID == "" || candidate.CreatedAt.After(target.CreatedAt)) {
			target = candidate
		}
	}
	if target.ID == "" {
		return failed, target, ErrBlocked
	}
	return failed, target, nil
}

// Control records a participant decision without discarding execution or
// health evidence. expectedState gives callers compare-and-swap semantics.
func (s *Store) Control(repo, id, actor, action, expectedState, reason string) (Promotion, error) {
	if !validID(actor) || len(reason) > 1000 {
		return Promotion{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Promotion{}, err
	}
	defer unlock()
	p, err := s.getPromotion(repo, id)
	if err != nil {
		return p, err
	}
	if expectedState != "" && p.State != expectedState {
		return p, ErrBlocked
	}
	next := ""
	switch action {
	case "pause":
		if p.State == "running" {
			next = "paused"
		}
	case "resume":
		if p.State == "paused" {
			next = "running"
		}
	case "cancel":
		if p.State == "pending_approval" || p.State == "queued" || p.State == "running" || p.State == "paused" {
			next = "canceled"
		}
	case "mark_unsuccessful":
		if p.State == "running" || p.State == "paused" || p.State == "succeeded" {
			next = "failed"
		}
	}
	if next == "" {
		return p, ErrBlocked
	}
	now := s.now()
	p.State = next
	if next == "failed" || next == "canceled" {
		p.CompletedAt = &now
		p.LeaseExpiresAt = nil
	}
	p.Events = append(p.Events, Event{Sequence: len(p.Events) + 1, Kind: "deployment." + action, ActorID: actor, State: next, Message: strings.TrimSpace(reason), CreatedAt: now})
	return p, s.write(filepath.Join(s.root, repo, "promotions", id+".json"), p)
}

func (s *Store) RecordStage(repo, id, owner string, stage int, evidence SignalEvidence) (Promotion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Promotion{}, err
	}
	defer unlock()
	p, err := s.getPromotion(repo, id)
	if err != nil {
		return p, err
	}
	if (p.State != "running" && p.State != "paused") || p.ExecutionOwner != owner || stage < 0 || stage >= len(p.Rollout.Stages) {
		return p, ErrBlocked
	}
	p.CurrentStage = stage
	evidence.CreatedAt = s.now()
	p.Evidence = append(p.Evidence, evidence)
	p.Events = append(p.Events, Event{Sequence: len(p.Events) + 1, Kind: "rollout.signal_" + evidence.State, State: p.State, Message: evidence.Stage + " / " + evidence.Signal + ": " + evidence.Message, CreatedAt: evidence.CreatedAt})
	return p, s.write(filepath.Join(s.root, repo, "promotions", id+".json"), p)
}
func (s *Store) Approve(repo, id, actor string) (Promotion, error) {
	if !validID(actor) {
		return Promotion{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Promotion{}, err
	}
	defer unlock()
	p, err := s.getPromotion(repo, id)
	if err != nil {
		return p, err
	}
	env, err := s.getEnvironment(repo, p.EnvironmentID)
	if err != nil {
		return p, err
	}
	if p.State != "pending_approval" {
		return p, ErrBlocked
	}
	for _, a := range p.Approvals {
		if a.ActorID == actor {
			return p, ErrBlocked
		}
	}
	if actor == p.InitiatedBy {
		return p, ErrBlocked
	}
	now := s.now()
	p.Approvals = append(p.Approvals, Approval{ActorID: actor, CreatedAt: now})
	p.Events = append(p.Events, Event{Sequence: len(p.Events) + 1, Kind: "promotion.approved", ActorID: actor, State: p.State, CreatedAt: now})
	if len(p.Approvals) >= env.RequiredApprovals {
		p.State = "queued"
		p.Events = append(p.Events, Event{Sequence: len(p.Events) + 1, Kind: "promotion.queued", ActorID: actor, State: p.State, CreatedAt: now})
	}
	return p, s.write(filepath.Join(s.root, repo, "promotions", id+".json"), p)
}
func (s *Store) Transition(repo, id, state, message string) (Promotion, error) {
	return s.transition(repo, id, state, message, "", nil)
}

// Claim atomically owns queued work for one bounded execution generation.
func (s *Store) Claim(repo, id, owner string, leaseExpires time.Time) (Promotion, error) {
	if !validID(owner) || !leaseExpires.After(s.now()) {
		return Promotion{}, ErrInvalid
	}
	return s.transition(repo, id, "running", "Execution claimed for exact artifact verification.", owner, &leaseExpires)
}

// Reject terminalizes setup work that cannot be claimed, without allowing a
// caller to overwrite running or completed execution evidence.
func (s *Store) Reject(repo, id, message string) (Promotion, error) {
	return s.transition(repo, id, "failed", message, "", nil)
}

// Renew extends only the current owner's live execution lease.
func (s *Store) Renew(repo, id, owner string, leaseExpires time.Time) (Promotion, error) {
	if !validID(owner) || !leaseExpires.After(s.now()) {
		return Promotion{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Promotion{}, err
	}
	defer unlock()
	p, err := s.getPromotion(repo, id)
	if err != nil {
		return p, err
	}
	if (p.State != "running" && p.State != "paused") || p.ExecutionOwner != owner {
		return p, ErrBlocked
	}
	p.LeaseExpiresAt = &leaseExpires
	return p, s.write(filepath.Join(s.root, repo, "promotions", id+".json"), p)
}

// Complete compare-and-swaps the terminal result against its execution owner.
func (s *Store) Complete(repo, id, owner, state, message string) (Promotion, error) {
	return s.transition(repo, id, state, message, owner, nil)
}

func (s *Store) transition(repo, id, state, message, owner string, lease *time.Time) (Promotion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Promotion{}, err
	}
	defer unlock()
	p, err := s.getPromotion(repo, id)
	if err != nil {
		return p, err
	}
	valid := p.State == "queued" && (state == "running" || state == "failed") || p.State == "running" && (state == "succeeded" || state == "failed") || p.State == "paused" && state == "failed"
	if !valid {
		return p, ErrBlocked
	}
	if (p.State == "running" || p.State == "paused") && owner != "" && p.ExecutionOwner != owner {
		return p, ErrBlocked
	}
	env, _ := s.getEnvironment(repo, p.EnvironmentID)
	if state == "running" {
		active := 0
		all, _ := s.listPromotions(repo)
		for _, x := range all {
			if x.EnvironmentID == p.EnvironmentID && x.State == "running" {
				active++
			}
		}
		if active >= env.Concurrency {
			return p, ErrBlocked
		}
	}
	now := s.now()
	p.State = state
	if state == "running" {
		p.StartedAt = &now
		p.ExecutionOwner = owner
		p.LeaseExpiresAt = lease
	} else {
		p.CompletedAt = &now
		p.LeaseExpiresAt = nil
	}
	p.Events = append(p.Events, Event{Sequence: len(p.Events) + 1, Kind: "deployment." + state, State: state, Message: message, CreatedAt: now})
	return p, s.write(filepath.Join(s.root, repo, "promotions", id+".json"), p)
}
func (s *Store) GetPromotion(repo, id string) (Promotion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getPromotion(repo, id)
}
func (s *Store) getPromotion(repo, id string) (Promotion, error) {
	var p Promotion
	if !validID(repo) || !validID(id) {
		return p, ErrNotFound
	}
	err := s.read(filepath.Join(s.root, repo, "promotions", id+".json"), &p)
	return p, err
}
func (s *Store) ListPromotions(repo string) ([]Promotion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listPromotions(repo)
}

// Nonterminal returns durable queued and interrupted work for recovery.
func (s *Store) Nonterminal() ([]Promotion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var result []Promotion
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) {
			continue
		}
		items, err := s.listPromotions(entry.Name())
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.State == "queued" || item.State == "running" || item.State == "paused" {
				result = append(result, item)
			}
		}
	}
	return result, nil
}
func (s *Store) listPromotions(repo string) ([]Promotion, error) {
	var p []Promotion
	if !validID(repo) {
		return nil, ErrNotFound
	}
	err := s.list(filepath.Join(s.root, repo, "promotions"), &p)
	sort.Slice(p, func(i, j int) bool { return p[i].CreatedAt.Before(p[j].CreatedAt) })
	return p, err
}

func (s *Store) write(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if env, ok := v.(Environment); ok {
		raw, _ := json.Marshal(env.Credentials)
		block, _ := aes.NewCipher(s.key)
		gcm, _ := cipher.NewGCM(block)
		nonce := make([]byte, gcm.NonceSize())
		rand.Read(nonce)
		sealed := gcm.Seal(nonce, nonce, raw, nil)
		type alias Environment
		body, _ = json.Marshal(struct {
			alias
			EncryptedCredentials string `json:"encrypted_credentials"`
		}{alias: alias(env), EncryptedCredentials: hex.EncodeToString(sealed)})
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	tmp.Chmod(0600)
	_, err = tmp.Write(body)
	if err == nil {
		err = tmp.Sync()
	}
	tmp.Close()
	if err == nil {
		err = os.Rename(name, path)
	}
	return err
}
func (s *Store) read(path string, v any) error {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if env, ok := v.(*Environment); ok {
		type alias Environment
		var wire struct {
			alias
			EncryptedCredentials string `json:"encrypted_credentials"`
		}
		if json.Unmarshal(body, &wire) != nil {
			return ErrNotFound
		}
		*env = Environment(wire.alias)
		sealed, e := hex.DecodeString(wire.EncryptedCredentials)
		if e != nil {
			return e
		}
		block, _ := aes.NewCipher(s.key)
		gcm, _ := cipher.NewGCM(block)
		if len(sealed) < gcm.NonceSize() {
			return ErrInvalid
		}
		raw, e := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
		if e != nil {
			return e
		}
		return json.Unmarshal(raw, &env.Credentials)
	}
	if json.Unmarshal(body, v) != nil {
		return ErrNotFound
	}
	return nil
}
func (s *Store) list(dir string, out any) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	switch values := out.(type) {
	case *[]Environment:
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				var v Environment
				if err = s.read(filepath.Join(dir, e.Name()), &v); err != nil {
					return err
				}
				*values = append(*values, v)
			}
		}
	case *[]Promotion:
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				var v Promotion
				if err = s.read(filepath.Join(dir, e.Name()), &v); err != nil {
					return err
				}
				*values = append(*values, v)
			}
		}
	}
	return nil
}
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
func public(v Environment) Environment {
	v.CredentialNames = keys(v.Credentials)
	v.Credentials = nil
	return v
}
func keys(v map[string]string) []string {
	r := make([]string, 0, len(v))
	for k := range v {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}
func validName(v string) bool {
	if v == "" || len(v) > 100 {
		return false
	}
	for _, r := range v {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func validImage(image string) bool {
	if image == "" || len(image) > 200 || strings.ContainsAny(image, " \t\r\n@") {
		return false
	}
	for _, r := range image {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("./:_-", r)) {
			return false
		}
	}
	return true
}
func validRollout(v RolloutDefinition) bool {
	if v.Version != 1 || len(v.Stages) == 0 || len(v.Stages) > 20 {
		return false
	}
	for _, stage := range v.Stages {
		if strings.TrimSpace(stage.Name) == "" || len(stage.Name) > 100 || stage.ObservationSeconds < 0 || stage.ObservationSeconds > 3600 || len(stage.Signals) == 0 || len(stage.Signals) > 20 {
			return false
		}
		for _, signal := range stage.Signals {
			if strings.TrimSpace(signal.Name) == "" || len(signal.Name) > 100 || strings.TrimSpace(signal.Command) == "" || len(signal.Command) > 4000 {
				return false
			}
		}
	}
	return true
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil && v == strings.ToLower(v)
}
func newID() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
