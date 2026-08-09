// Package auth provides durable, revocable credentials for platform actors.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotFound = errors.New("credential not found")
	ErrInvalid  = errors.New("invalid credential")
)

type Kind string

const (
	Session Kind = "session"
	API     Kind = "api"
	Git     Kind = "git"
)

var allowedScopes = map[Kind]map[string]bool{
	Session: {"profile:write": true, "credentials:write": true, "repositories:read": true, "repositories:write": true},
	API:     {"profile:write": true, "repositories:read": true, "repositories:write": true, "incidents:investigate": true, "security:investigate": true},
	Git:     {"git:read": true, "git:write": true},
}

var maximumLifetime = map[Kind]time.Duration{
	Session: 24 * time.Hour,
	API:     90 * 24 * time.Hour,
	Git:     30 * 24 * time.Hour,
}

type Credential struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	Kind           Kind       `json:"kind"`
	Name           string     `json:"name"`
	Scopes         []string   `json:"scopes"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	RepositoryID   string     `json:"repository_id,omitempty"`
	GitWriteBranch string     `json:"git_write_branch,omitempty"`
	PullRequestID  string     `json:"pull_request_id,omitempty"`
	Hash           string     `json:"-"`
}

type IssuedCredential struct {
	Credential
	Token string `json:"token"`
}

type Store struct {
	root       string
	mu         sync.Mutex
	now        func() time.Time
	afterWrite func() error
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("auth storage root is empty")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create auth storage: %w", err)
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Store) Issue(userID string, kind Kind, name string, scopes []string, lifetime time.Duration) (IssuedCredential, error) {
	return s.IssueBound(userID, kind, name, scopes, lifetime, "", "")
}

// IssueBound creates a credential constrained to one repository and,
// optionally, one writable Git branch. Empty bounds retain the ordinary
// account credential behavior used by existing clients.
func (s *Store) IssueBound(userID string, kind Kind, name string, scopes []string, lifetime time.Duration, repositoryID, gitWriteBranch string) (IssuedCredential, error) {
	return s.issueBound(userID, kind, name, scopes, lifetime, repositoryID, gitWriteBranch, "")
}

// IssuePullRequestBound additionally pins a branch credential to the exact
// pull request whose source owner granted it.
func (s *Store) IssuePullRequestBound(userID string, name string, scopes []string, lifetime time.Duration, repositoryID, gitWriteBranch, pullRequestID string) (IssuedCredential, error) {
	return s.issueBound(userID, Git, name, scopes, lifetime, repositoryID, gitWriteBranch, pullRequestID)
}

func (s *Store) issueBound(userID string, kind Kind, name string, scopes []string, lifetime time.Duration, repositoryID, gitWriteBranch, pullRequestID string) (IssuedCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(userID) || allowedScopes[kind] == nil {
		return IssuedCredential{}, ErrInvalid
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 || strings.ContainsAny(name, "\x00\r\n") {
		return IssuedCredential{}, ErrInvalid
	}
	if lifetime <= 0 || lifetime > maximumLifetime[kind] {
		return IssuedCredential{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, scope := range scopes {
		if !allowedScopes[kind][scope] || seen[scope] {
			return IssuedCredential{}, ErrInvalid
		}
		seen[scope] = true
	}
	if len(scopes) == 0 {
		return IssuedCredential{}, ErrInvalid
	}
	if (repositoryID != "" && !validID(repositoryID)) || (gitWriteBranch != "" && (kind != Git || repositoryID == "" || !validBoundBranch(gitWriteBranch))) || (pullRequestID != "" && (kind != Git || gitWriteBranch == "" || !validID(pullRequestID))) {
		return IssuedCredential{}, ErrInvalid
	}
	sort.Strings(scopes)
	idBytes, secretBytes := make([]byte, 16), make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		return IssuedCredential{}, err
	}
	if _, err := rand.Read(secretBytes); err != nil {
		return IssuedCredential{}, err
	}
	id := hex.EncodeToString(idBytes)
	token := "vvr_" + id + "_" + hex.EncodeToString(secretBytes)
	hash := sha256.Sum256([]byte(token))
	now := s.now().Truncate(time.Microsecond)
	credential := Credential{ID: id, UserID: userID, Kind: kind, Name: name, Scopes: append([]string(nil), scopes...), CreatedAt: now, ExpiresAt: now.Add(lifetime), RepositoryID: repositoryID, GitWriteBranch: gitWriteBranch, PullRequestID: pullRequestID, Hash: hex.EncodeToString(hash[:])}
	if err := s.write(credential); err != nil {
		// Reconcile errors after rename so Issue never reports failure while
		// leaving the exact usable credential durably visible.
		if persisted, readErr := s.read(id); readErr == nil && sameCredential(persisted, credential) {
			return IssuedCredential{Credential: credential, Token: token}, nil
		}
		return IssuedCredential{}, err
	}
	return IssuedCredential{Credential: credential, Token: token}, nil
}

func validBoundBranch(branch string) bool {
	name, ok := strings.CutPrefix(branch, "refs/heads/")
	if !ok || name == "" || len(name) > 200 || strings.Contains(name, "..") || strings.HasSuffix(name, ".lock") {
		return false
	}
	for _, character := range name {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("._/-", character)) {
			return false
		}
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".") {
			return false
		}
	}
	return true
}

func (s *Store) Authenticate(token string, required string) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Credential{}, err
	}
	defer unlock()
	id, ok := tokenID(token)
	if !ok {
		return Credential{}, ErrNotFound
	}
	credential, err := s.read(id)
	if err != nil {
		return Credential{}, err
	}
	hash := sha256.Sum256([]byte(token))
	storedHash, decodeErr := hex.DecodeString(credential.Hash)
	if decodeErr != nil || subtle.ConstantTimeCompare(storedHash, hash[:]) != 1 || credential.RevokedAt != nil || !s.now().Before(credential.ExpiresAt) || (required != "" && !hasScope(credential.Scopes, required)) {
		return Credential{}, ErrNotFound
	}
	now := s.now().Truncate(time.Microsecond)
	credential.LastUsedAt = &now
	if err := s.write(credential); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

func (s *Store) List(userID string) ([]Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	result := []Credential{}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !validID(id) {
			continue
		}
		credential, err := s.read(id)
		if err != nil {
			return nil, err
		}
		if credential.UserID == userID {
			result = append(result, credential)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (s *Store) Revoke(userID, id string) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Credential{}, err
	}
	defer unlock()
	credential, err := s.read(id)
	if err != nil || credential.UserID != userID {
		return Credential{}, ErrNotFound
	}
	if credential.RevokedAt == nil {
		now := s.now().Truncate(time.Microsecond)
		credential.RevokedAt = &now
		if err := s.write(credential); err != nil {
			return Credential{}, err
		}
	}
	return credential, nil
}

func hasScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required {
			return true
		}
	}
	return false
}

func sameCredential(left, right Credential) bool {
	return left.ID == right.ID && left.UserID == right.UserID && left.Kind == right.Kind && left.Name == right.Name &&
		left.CreatedAt.Equal(right.CreatedAt) && left.ExpiresAt.Equal(right.ExpiresAt) && left.Hash == right.Hash &&
		slices.Equal(left.Scopes, right.Scopes) && left.RepositoryID == right.RepositoryID && left.GitWriteBranch == right.GitWriteBranch && left.PullRequestID == right.PullRequestID && left.LastUsedAt == nil && left.RevokedAt == nil
}
func validID(id string) bool {
	if len(id) != 32 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
func tokenID(token string) (string, bool) {
	parts := strings.Split(token, "_")
	if len(parts) != 3 || parts[0] != "vvr" || !validID(parts[1]) || len(parts[2]) != 64 {
		return "", false
	}
	_, err := hex.DecodeString(parts[2])
	return parts[1], err == nil
}

func (s *Store) read(id string) (Credential, error) {
	if !validID(id) {
		return Credential{}, ErrNotFound
	}
	data, err := os.ReadFile(filepath.Join(s.root, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return Credential{}, ErrNotFound
	}
	if err != nil {
		return Credential{}, err
	}
	var record credentialRecord
	if json.Unmarshal(data, &record) != nil || record.ID != id || record.Hash == "" {
		return Credential{}, fmt.Errorf("corrupt credential %s", id)
	}
	record.Credential.Hash = record.Hash
	return record.Credential, nil
}

func (s *Store) write(credential Credential) error {
	data, err := json.Marshal(credentialRecord{Credential: credential, Hash: credential.Hash})
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.root, ".credential-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, filepath.Join(s.root, credential.ID+".json")); err != nil {
		return err
	}
	if s.afterWrite != nil {
		if err := s.afterWrite(); err != nil {
			return err
		}
	}
	dir, err := os.Open(s.root)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *Store) lockRoot() (func(), error) {
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		lock.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); _ = lock.Close() }, nil
}

type credentialRecord struct {
	Credential
	Hash string `json:"hash"`
}
