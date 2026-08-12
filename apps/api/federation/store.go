// Package federation publishes a signed instance identity and retains explicit
// trust in independently administered peers. Federated identities are
// attribution references; they never authenticate as local principals.
package federation

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

var (
	ErrNotFound = errors.New("federation peer not found")
	ErrConflict = errors.New("federation peer changed")
	ErrInvalid  = errors.New("invalid federation identity")
)

type Endpoint struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}
type Key struct {
	ID        string     `json:"id"`
	Algorithm string     `json:"algorithm"`
	PublicKey string     `json:"public_key"`
	CreatedAt time.Time  `json:"created_at"`
	RetiredAt *time.Time `json:"retired_at,omitempty"`
}
type Document struct {
	Protocol          string     `json:"protocol"`
	InstanceID        string     `json:"instance_id"`
	Name              string     `json:"name"`
	Version           int        `json:"version"`
	IssuedAt          time.Time  `json:"issued_at"`
	Endpoints         []Endpoint `json:"endpoints"`
	Capabilities      []string   `json:"capabilities"`
	Operators         []string   `json:"operators"`
	Keys              []Key      `json:"keys"`
	SigningKeyID      string     `json:"signing_key_id"`
	RotationSignature string     `json:"rotation_signature,omitempty"`
	Signature         string     `json:"signature"`
}
type Peer struct {
	InstanceID     string     `json:"instance_id"`
	DiscoveryURL   string     `json:"discovery_url"`
	Status         string     `json:"status"`
	TrustVersion   int        `json:"trust_version"`
	Document       Document   `json:"document"`
	FirstSeenAt    time.Time  `json:"first_seen_at"`
	LastCheckedAt  time.Time  `json:"last_checked_at"`
	LastVerifiedAt time.Time  `json:"last_verified_at"`
	ChangedAt      *time.Time `json:"changed_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
}

// RepositorySnapshot is a bounded, read-only projection of a repository owned
// by another instance. It intentionally contains no Git objects, credentials,
// private discussion, or local authorization claims.
type RepositorySnapshot struct {
	Reference           string                  `json:"reference"`
	InstanceID          string                  `json:"instance_id"`
	RepositoryID        string                  `json:"repository_id"`
	Name                string                  `json:"name"`
	Visibility          string                  `json:"visibility"`
	DefaultBranch       string                  `json:"default_branch"`
	Revision            string                  `json:"revision"`
	Branches            []RepositoryBranch      `json:"branches"`
	Releases            []RepositoryRelease     `json:"releases"`
	ContributorGuidance *RepositoryGuidance     `json:"contributor_guidance,omitempty"`
	Issues              []RepositoryIssue       `json:"issues"`
	Opportunities       []RepositoryOpportunity `json:"opportunities"`
	Capabilities        []string                `json:"capabilities"`
	AuthoritativeURL    string                  `json:"authoritative_url"`
	GeneratedAt         time.Time               `json:"generated_at"`
}
type RepositoryBranch struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
}
type RepositoryRelease struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Notes       string `json:"notes"`
	Revision    string `json:"revision"`
	PublishedAt string `json:"published_at"`
}
type RepositoryGuidance struct {
	Version       int      `json:"version"`
	Goals         string   `json:"goals"`
	Prerequisites []string `json:"prerequisites"`
	SetupSummary  string   `json:"setup_summary"`
	Communication string   `json:"communication"`
	ReviewPolicy  string   `json:"review_policy"`
}
type RepositoryIssue struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Severity  string    `json:"severity"`
	Status    string    `json:"status"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}
type RepositoryOpportunity struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	ExpectedOutcome  string   `json:"expected_outcome"`
	Scope            string   `json:"scope"`
	Risk             string   `json:"risk"`
	Revision         string   `json:"revision"`
	Status           string   `json:"status"`
	Version          int      `json:"version"`
	EstimatedMinutes int      `json:"estimated_minutes"`
	RequiredSkills   []string `json:"required_skills"`
	AgentAssistance  bool     `json:"agent_assistance"`
}
type SignedRepositorySnapshot struct {
	Snapshot        RepositorySnapshot `json:"snapshot"`
	DocumentVersion int                `json:"identity_document_version"`
	SigningKeyID    string             `json:"signing_key_id"`
	Signature       string             `json:"signature"`
}
type RepositoryCache struct {
	Reference               string              `json:"reference"`
	PeerID                  string              `json:"peer_id"`
	RepositoryID            string              `json:"repository_id"`
	Status                  string              `json:"status"`
	Snapshot                *RepositorySnapshot `json:"snapshot,omitempty"`
	RemoteRevision          string              `json:"remote_revision,omitempty"`
	SignatureVerified       bool                `json:"signature_verified"`
	IdentityDocumentVersion int                 `json:"identity_document_version,omitempty"`
	FetchedAt               time.Time           `json:"fetched_at,omitempty"`
	LastAttemptAt           time.Time           `json:"last_attempt_at"`
	StaleSince              *time.Time          `json:"stale_since,omitempty"`
	LastError               string              `json:"last_error,omitempty"`
}
type RepositoryFollow struct {
	UserID           string    `json:"user_id"`
	Reference        string    `json:"reference"`
	FollowedAt       time.Time `json:"followed_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	LastSeenRevision string    `json:"last_seen_revision,omitempty"`
}
type persistedIdentity struct {
	Document   Document `json:"document"`
	PrivateKey string   `json:"private_key"`
}
type Store struct {
	root, name, publicURL string
	operators             []string
	mu                    sync.Mutex
	now                   func() time.Time
}

// CollaborationEvent is an immutable, signed cross-instance claim about one
// federated contribution. Imported claims retain their remote actor and
// verification boundary; they are never converted into local users or grants.
type CollaborationEvent struct {
	ID               string          `json:"id"`
	ContributionID   string          `json:"contribution_id"`
	Sequence         int64           `json:"sequence"`
	Kind             string          `json:"kind"`
	Actor            string          `json:"actor"`
	Revision         string          `json:"revision,omitempty"`
	Body             string          `json:"body,omitempty"`
	Decision         string          `json:"decision,omitempty"`
	State            string          `json:"state,omitempty"`
	Evidence         json.RawMessage `json:"evidence,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	OriginInstanceID string          `json:"origin_instance_id"`
	DocumentVersion  int             `json:"document_version"`
	SigningKeyID     string          `json:"signing_key_id"`
	Signature        string          `json:"signature"`
	Verification     string          `json:"verification"`
	Stale            bool            `json:"stale"`
}

func validCollaborationEvent(v CollaborationEvent) bool {
	if v.ID == "" || v.ContributionID == "" || v.Sequence < 1 || v.Actor == "" || v.OriginInstanceID == "" || v.CreatedAt.IsZero() {
		return false
	}
	switch v.Kind {
	case "comment", "review", "revision", "checks", "preview", "closure":
	default:
		return false
	}
	if len(v.Body) > 20000 || len(v.Evidence) > 256<<10 {
		return false
	}
	if v.Kind == "comment" && strings.TrimSpace(v.Body) == "" {
		return false
	}
	if v.Kind == "review" && v.Decision != "approved" && v.Decision != "changes_requested" && v.Decision != "withdrawn" {
		return false
	}
	if (v.Kind == "revision" || v.Kind == "review" || v.Kind == "checks" || v.Kind == "preview") && len(v.Revision) != 40 {
		return false
	}
	if v.Kind == "closure" && v.State != "open" && v.State != "closed" {
		return false
	}
	return true
}

// AppendCollaborationEvent idempotently retains a verified event. The same
// origin/ID may be retried only with byte-for-byte identical signed content.
func (s *Store) AppendCollaborationEvent(v CollaborationEvent) (CollaborationEvent, error) {
	if !validCollaborationEvent(v) {
		return CollaborationEvent{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return CollaborationEvent{}, err
	}
	defer unlock()
	dir := filepath.Join(s.root, "collaboration", v.ContributionID)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return CollaborationEvent{}, err
	}
	path := filepath.Join(dir, v.OriginInstanceID+"-"+v.ID+".json")
	if raw, readErr := os.ReadFile(path); readErr == nil {
		var prior CollaborationEvent
		if json.Unmarshal(raw, &prior) != nil {
			return CollaborationEvent{}, ErrInvalid
		}
		a, _ := json.Marshal(prior)
		b, _ := json.Marshal(v)
		if string(a) != string(b) {
			return CollaborationEvent{}, ErrConflict
		}
		return prior, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return CollaborationEvent{}, readErr
	}
	if err = writeJSON(path, v); err != nil {
		return CollaborationEvent{}, err
	}
	return v, nil
}

func (s *Store) ListCollaborationEvents(contributionID, currentRevision string) ([]CollaborationEvent, error) {
	dir := filepath.Join(s.root, "collaboration", contributionID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []CollaborationEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []CollaborationEvent{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			return nil, e
		}
		var v CollaborationEvent
		if json.Unmarshal(raw, &v) != nil || !validCollaborationEvent(v) {
			return nil, ErrInvalid
		}
		v.Stale = v.Revision != "" && currentRevision != "" && v.Revision != currentRevision
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func New(root, name, publicURL string, operators []string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("federation storage root is empty")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create federation storage: %w", err)
	}
	s := &Store{root: root, name: strings.TrimSpace(name), publicURL: strings.TrimRight(strings.TrimSpace(publicURL), "/"), operators: clean(operators), now: func() time.Time { return time.Now().UTC() }}
	if s.name == "" {
		s.name = "Vivarium"
	}
	if s.publicURL == "" {
		s.publicURL = "http://127.0.0.1:8080"
	}
	if _, err := s.Identity(); err != nil {
		return nil, err
	}
	return s, nil
}
func clean(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
func (s *Store) Identity() (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Document{}, err
	}
	defer unlock()
	p := filepath.Join(s.root, "identity.json")
	raw, err := os.ReadFile(p)
	if err == nil {
		var v persistedIdentity
		if json.Unmarshal(raw, &v) == nil {
			return v.Document, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Document{}, err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Document{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	sum := sha256.Sum256(pub)
	id := "ed25519:" + hex.EncodeToString(sum[:8])
	instanceSum := sha256.Sum256(pub)
	d := Document{Protocol: "vivarium-federation/v1", InstanceID: hex.EncodeToString(instanceSum[:16]), Name: s.name, Version: 1, IssuedAt: now, Endpoints: []Endpoint{{Kind: "api", URL: s.publicURL}, {Kind: "actors", URL: s.publicURL + "/federation/actors/{type}/{id}"}, {Kind: "repositories", URL: s.publicURL + "/federation/repositories/{id}"}, {Kind: "contributions", URL: s.publicURL + "/federation/contributions"}}, Capabilities: []string{"identity.v1", "actor-resolution.v1", "signed-attribution.v1", "repository-discovery.v1", "repository-contribution.v1"}, Operators: s.operators, Keys: []Key{{ID: id, Algorithm: "Ed25519", PublicKey: base64.RawURLEncoding.EncodeToString(pub), CreatedAt: now}}, SigningKeyID: id}
	d.Signature = sign(d, priv)
	v := persistedIdentity{Document: d, PrivateKey: base64.RawURLEncoding.EncodeToString(priv)}
	if err = writeJSON(p, v); err != nil {
		return Document{}, err
	}
	return d, nil
}

// SignPayload signs a bounded protocol payload with the instance identity.
func (s *Store) SignPayload(payload []byte) (int, string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v persistedIdentity
	b, err := os.ReadFile(filepath.Join(s.root, "identity.json"))
	if err != nil {
		return 0, "", "", err
	}
	if json.Unmarshal(b, &v) != nil {
		return 0, "", "", ErrInvalid
	}
	private, err := base64.RawURLEncoding.DecodeString(v.PrivateKey)
	if err != nil || len(private) != ed25519.PrivateKeySize {
		return 0, "", "", ErrInvalid
	}
	return v.Document.Version, v.Document.SigningKeyID, base64.RawURLEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(private), payload)), nil
}

func VerifyPayload(payload []byte, version int, keyID, signature string, document Document) error {
	if version != document.Version || keyID != document.SigningKeyID {
		return ErrInvalid
	}
	var public []byte
	for _, key := range document.Keys {
		if key.ID == keyID && key.RetiredAt == nil {
			public, _ = base64.RawURLEncoding.DecodeString(key.PublicKey)
		}
	}
	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || len(public) != ed25519.PublicKeySize || !ed25519.Verify(ed25519.PublicKey(public), payload, sig) {
		return ErrInvalid
	}
	return nil
}
func (s *Store) IsOperator(id string) bool {
	for _, v := range s.operators {
		if v == id {
			return true
		}
	}
	return false
}
func (s *Store) Rotate() (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Document{}, err
	}
	defer unlock()
	path := filepath.Join(s.root, "identity.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	var v persistedIdentity
	if err = json.Unmarshal(b, &v); err != nil {
		return Document{}, err
	}
	previousPrivate, err := base64.RawURLEncoding.DecodeString(v.PrivateKey)
	if err != nil || len(previousPrivate) != ed25519.PrivateKeySize {
		return Document{}, ErrInvalid
	}
	now := s.now().Truncate(time.Microsecond)
	for i := range v.Document.Keys {
		if v.Document.Keys[i].ID == v.Document.SigningKeyID {
			v.Document.Keys[i].RetiredAt = &now
		}
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Document{}, err
	}
	sum := sha256.Sum256(pub)
	id := "ed25519:" + hex.EncodeToString(sum[:8])
	v.Document.Keys = append(v.Document.Keys, Key{ID: id, Algorithm: "Ed25519", PublicKey: base64.RawURLEncoding.EncodeToString(pub), CreatedAt: now})
	v.Document.Version++
	v.Document.IssuedAt = now
	v.Document.SigningKeyID = id
	v.Document.RotationSignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(previousPrivate), unsigned(v.Document)))
	v.Document.Signature = sign(v.Document, priv)
	v.PrivateKey = base64.RawURLEncoding.EncodeToString(priv)
	if err = writeJSON(path, v); err != nil {
		return Document{}, err
	}
	return v.Document, nil
}
func unsigned(d Document) []byte {
	d.Signature = ""
	d.RotationSignature = ""
	b, _ := json.Marshal(d)
	return b
}
func sign(d Document, k ed25519.PrivateKey) string {
	return base64.RawURLEncoding.EncodeToString(ed25519.Sign(k, unsigned(d)))
}
func repositoryPayload(snapshot RepositorySnapshot, documentVersion int, keyID string) []byte {
	b, _ := json.Marshal(struct {
		Snapshot        RepositorySnapshot `json:"snapshot"`
		DocumentVersion int                `json:"identity_document_version"`
		SigningKeyID    string             `json:"signing_key_id"`
	}{snapshot, documentVersion, keyID})
	return b
}
func (s *Store) SignRepository(snapshot RepositorySnapshot) (SignedRepositorySnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(filepath.Join(s.root, "identity.json"))
	if err != nil {
		return SignedRepositorySnapshot{}, err
	}
	var v persistedIdentity
	if json.Unmarshal(b, &v) != nil {
		return SignedRepositorySnapshot{}, ErrInvalid
	}
	private, err := base64.RawURLEncoding.DecodeString(v.PrivateKey)
	if err != nil || len(private) != ed25519.PrivateKeySize {
		return SignedRepositorySnapshot{}, ErrInvalid
	}
	snapshot.InstanceID, snapshot.Reference = v.Document.InstanceID, v.Document.InstanceID+":"+snapshot.RepositoryID
	snapshot.GeneratedAt = s.now().UTC().Truncate(time.Microsecond)
	result := SignedRepositorySnapshot{Snapshot: snapshot, DocumentVersion: v.Document.Version, SigningKeyID: v.Document.SigningKeyID}
	result.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(private), repositoryPayload(snapshot, result.DocumentVersion, result.SigningKeyID)))
	return result, nil
}
func VerifyRepository(signed SignedRepositorySnapshot, document Document) error {
	if err := Verify(document); err != nil || signed.DocumentVersion != document.Version || signed.SigningKeyID != document.SigningKeyID || signed.Snapshot.InstanceID != document.InstanceID || signed.Snapshot.Reference != document.InstanceID+":"+signed.Snapshot.RepositoryID || signed.Snapshot.Revision == "" {
		return ErrInvalid
	}
	var public []byte
	for _, key := range document.Keys {
		if key.ID == signed.SigningKeyID && key.RetiredAt == nil {
			public, _ = base64.RawURLEncoding.DecodeString(key.PublicKey)
		}
	}
	sig, err := base64.RawURLEncoding.DecodeString(signed.Signature)
	if err != nil || len(public) != ed25519.PublicKeySize || !ed25519.Verify(ed25519.PublicKey(public), repositoryPayload(signed.Snapshot, signed.DocumentVersion, signed.SigningKeyID), sig) {
		return ErrInvalid
	}
	return nil
}

func referenceParts(reference string) (string, string, error) {
	peer, repository, ok := strings.Cut(strings.TrimSpace(reference), ":")
	if !ok || !validInstanceID(peer) || repository == "" || strings.ContainsAny(repository, "/\\") {
		return "", "", ErrInvalid
	}
	return peer, repository, nil
}
func (s *Store) repositoryCachePath(reference string) (string, error) {
	peer, repository, err := referenceParts(reference)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, "repositories", peer, repository+".json"), nil
}
func (s *Store) PutRepositoryCache(cache RepositoryCache) (RepositoryCache, error) {
	path, err := s.repositoryCachePath(cache.Reference)
	if err != nil {
		return RepositoryCache{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return RepositoryCache{}, err
	}
	defer unlock()
	if err = writeJSON(path, cache); err != nil {
		return RepositoryCache{}, err
	}
	return cache, nil
}
func (s *Store) RepositoryCache(reference string) (RepositoryCache, error) {
	path, err := s.repositoryCachePath(reference)
	if err != nil {
		return RepositoryCache{}, err
	}
	var v RepositoryCache
	if readJSON(path, &v) != nil || v.Reference != reference {
		return RepositoryCache{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) Follow(userID, reference string, follow bool) (RepositoryFollow, error) {
	if strings.TrimSpace(userID) == "" {
		return RepositoryFollow{}, ErrInvalid
	}
	if _, _, err := referenceParts(reference); err != nil {
		return RepositoryFollow{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return RepositoryFollow{}, err
	}
	defer unlock()
	path := filepath.Join(s.root, "follows", userID, strings.ReplaceAll(reference, ":", "-")+".json")
	if !follow {
		if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return RepositoryFollow{}, err
		}
		return RepositoryFollow{UserID: userID, Reference: reference}, nil
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	v := RepositoryFollow{UserID: userID, Reference: reference, FollowedAt: now, UpdatedAt: now}
	_ = readJSON(path, &v)
	v.UpdatedAt = now
	v.UserID, v.Reference = userID, reference
	if err = writeJSON(path, v); err != nil {
		return RepositoryFollow{}, err
	}
	return v, nil
}
func (s *Store) ListFollows(userID string) ([]RepositoryFollow, error) {
	files, err := filepath.Glob(filepath.Join(s.root, "follows", userID, "*.json"))
	if err != nil {
		return nil, err
	}
	out := []RepositoryFollow{}
	for _, path := range files {
		var v RepositoryFollow
		if readJSON(path, &v) == nil && v.UserID == userID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FollowedAt.Before(out[j].FollowedAt) })
	return out, nil
}
func Verify(d Document) error {
	if d.Protocol != "vivarium-federation/v1" || !validInstanceID(d.InstanceID) || d.Version < 1 || d.SigningKeyID == "" {
		return ErrInvalid
	}
	var key *Key
	for i := range d.Keys {
		if d.Keys[i].ID == d.SigningKeyID {
			key = &d.Keys[i]
			break
		}
	}
	if key == nil || key.Algorithm != "Ed25519" || key.RetiredAt != nil {
		return ErrInvalid
	}
	pub, e := base64.RawURLEncoding.DecodeString(key.PublicKey)
	if e != nil || len(pub) != ed25519.PublicKeySize {
		return ErrInvalid
	}
	if len(d.Keys) == 0 {
		return ErrInvalid
	}
	rootPublic, e := base64.RawURLEncoding.DecodeString(d.Keys[0].PublicKey)
	if e != nil || len(rootPublic) != ed25519.PublicKeySize {
		return ErrInvalid
	}
	rootSum := sha256.Sum256(rootPublic)
	if d.InstanceID != hex.EncodeToString(rootSum[:16]) {
		return ErrInvalid
	}
	sig, e := base64.RawURLEncoding.DecodeString(d.Signature)
	if e != nil || !ed25519.Verify(pub, unsigned(d), sig) {
		return ErrInvalid
	}
	return nil
}
func validInstanceID(id string) bool {
	if len(id) != 32 || id != strings.ToLower(id) {
		return false
	}
	b, e := hex.DecodeString(id)
	return e == nil && len(b) == 16
}
func verifyContinuity(previous, next Document) error {
	if len(previous.Keys) == 0 || len(next.Keys) == 0 || previous.Keys[0].ID != next.Keys[0].ID || previous.Keys[0].PublicKey != next.Keys[0].PublicKey {
		return ErrInvalid
	}
	sig, e := base64.RawURLEncoding.DecodeString(next.RotationSignature)
	if e != nil {
		return ErrInvalid
	}
	for _, key := range previous.Keys {
		if key.RetiredAt != nil {
			continue
		}
		raw, x := base64.RawURLEncoding.DecodeString(key.PublicKey)
		if x == nil && len(raw) == ed25519.PublicKeySize && ed25519.Verify(ed25519.PublicKey(raw), unsigned(next), sig) {
			return nil
		}
	}
	return ErrInvalid
}
func (s *Store) Upsert(url string, d Document) (Peer, error) {
	if err := Verify(d); err != nil {
		return Peer{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Peer{}, err
	}
	defer unlock()
	now := s.now().Truncate(time.Microsecond)
	p, err := s.readPeer(d.InstanceID)
	if errors.Is(err, ErrNotFound) {
		p = Peer{InstanceID: d.InstanceID, DiscoveryURL: url, Status: "trusted", TrustVersion: 1, Document: d, FirstSeenAt: now, LastCheckedAt: now, LastVerifiedAt: now}
	} else if err != nil {
		return Peer{}, err
	} else {
		p.LastCheckedAt = now
		if p.Status == "revoked" {
			return Peer{}, ErrConflict
		}
		if d.Version < p.Document.Version {
			return Peer{}, ErrConflict
		}
		if d.Version == p.Document.Version && d.Signature != p.Document.Signature {
			return Peer{}, ErrConflict
		}
		if d.Version > p.Document.Version {
			if verifyContinuity(p.Document, d) != nil {
				return Peer{}, ErrConflict
			}
			p.Status = "changed"
			p.ChangedAt = &now
		}
		p.Document = d
		p.DiscoveryURL = url
		p.LastVerifiedAt = now
		p.LastError = ""
		p.TrustVersion++
	}
	peerPath, err := s.peerPath(p.InstanceID)
	if err != nil {
		return Peer{}, err
	}
	if err = writeJSON(peerPath, p); err != nil {
		return Peer{}, err
	}
	return p, nil
}
func (s *Store) RecordFailure(id, msg string) (Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Peer{}, e
	}
	defer unlock()
	p, e := s.readPeer(id)
	if e != nil {
		return Peer{}, e
	}
	p.LastCheckedAt = s.now().Truncate(time.Microsecond)
	p.LastError = msg
	if p.Status != "revoked" {
		p.Status = "unreachable"
	}
	p.TrustVersion++
	peerPath, pathErr := s.peerPath(id)
	if pathErr != nil {
		return Peer{}, pathErr
	}
	e = writeJSON(peerPath, p)
	return p, e
}
func (s *Store) Decide(id string, version int, action string) (Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Peer{}, e
	}
	defer unlock()
	p, e := s.readPeer(id)
	if e != nil {
		return Peer{}, e
	}
	if p.TrustVersion != version {
		return Peer{}, ErrConflict
	}
	now := s.now().Truncate(time.Microsecond)
	switch action {
	case "trust":
		p.Status = "trusted"
		p.ChangedAt = nil
		p.LastError = ""
	case "revoke":
		p.Status = "revoked"
		p.RevokedAt = &now
	default:
		return Peer{}, ErrInvalid
	}
	p.TrustVersion++
	peerPath, pathErr := s.peerPath(id)
	if pathErr != nil {
		return Peer{}, pathErr
	}
	e = writeJSON(peerPath, p)
	return p, e
}
func (s *Store) Get(id string) (Peer, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.readPeer(id) }
func (s *Store) List() ([]Peer, error) {
	files, e := filepath.Glob(filepath.Join(s.root, "peers", "*.json"))
	if e != nil {
		return nil, e
	}
	out := []Peer{}
	for _, f := range files {
		var p Peer
		b, e := os.ReadFile(f)
		if e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &p); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstanceID < out[j].InstanceID })
	return out, nil
}
func (s *Store) readPeer(id string) (Peer, error) {
	peerPath, e := s.peerPath(id)
	if e != nil {
		return Peer{}, ErrNotFound
	}
	b, e := os.ReadFile(peerPath)
	if errors.Is(e, os.ErrNotExist) {
		return Peer{}, ErrNotFound
	}
	if e != nil {
		return Peer{}, e
	}
	var p Peer
	if e = json.Unmarshal(b, &p); e != nil {
		return Peer{}, e
	}
	return p, nil
}
func (s *Store) peerPath(id string) (string, error) {
	if !validInstanceID(id) {
		return "", ErrNotFound
	}
	return filepath.Join(s.root, "peers", id+".json"), nil
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
func writeJSON(p string, v any) error {
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(p), ".tmp-")
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
	if c := tmp.Close(); e == nil {
		e = c
	}
	if e == nil {
		e = os.Rename(tmp.Name(), p)
	}
	return e
}
func readJSON(p string, v any) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
