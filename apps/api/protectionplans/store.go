// Package protectionplans persists encrypted recovery inputs and redacted evidence.
package protectionplans

import (
	"crypto/aes"
	"crypto/cipher"
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
	"time"
)

var ErrNotFound = errors.New("protection plan not found")
var ErrInvalid = errors.New("invalid protection plan")
var ErrConflict = errors.New("protection plan conflict")

type Resource struct {
	TargetID      string `json:"target_id"`
	Kind          string `json:"kind"`
	EnvironmentID string `json:"environment_id,omitempty"`
	Revision      string `json:"revision,omitempty"`
}
type Plan struct {
	ID                string     `json:"id"`
	RepositoryID      string     `json:"repository_id"`
	CommitmentID      string     `json:"commitment_id"`
	CommitmentVersion int        `json:"commitment_version"`
	Name              string     `json:"name"`
	Mode              string     `json:"mode"`
	Resources         []Resource `json:"resources"`
	Destination       string     `json:"destination"`
	Jurisdiction      string     `json:"jurisdiction"`
	RetentionDays     int        `json:"retention_days"`
	FreshnessMinutes  int        `json:"freshness_minutes"`
	AccessorIDs       []string   `json:"accessor_ids"`
	ValidationChecks  []string   `json:"validation_checks"`
	Version           int        `json:"version"`
	CreatedBy         string     `json:"created_by"`
	UpdatedBy         string     `json:"updated_by"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	Captures          []Capture  `json:"captures"`
}
type Entry struct {
	Path         string   `json:"path"`
	Kind         string   `json:"kind"`
	Version      string   `json:"version"`
	SHA256       string   `json:"sha256"`
	Size         int64    `json:"size"`
	Dependencies []string `json:"dependencies,omitempty"`
}
type Source struct {
	Revision string  `json:"revision"`
	Entries  []Entry `json:"entries"`
	Payload  []byte  `json:"-"`
}

// RestoredSource is available only to the recovery runner. Payload is kept in
// memory and must never be serialized into exercise evidence.
type RestoredSource struct {
	Manifest []Entry
	Payload  []byte
}
type Capture struct {
	ID                string     `json:"id"`
	PlanVersion       int        `json:"plan_version"`
	CommitmentVersion int        `json:"commitment_version"`
	Resources         []Resource `json:"resources"`
	FreshnessMinutes  int        `json:"freshness_minutes"`
	SourceRevision    string     `json:"source_revision"`
	ManifestSHA256    string     `json:"manifest_sha256"`
	EntryCount        int        `json:"entry_count"`
	PlaintextBytes    int64      `json:"plaintext_bytes"`
	StoredBytes       int64      `json:"stored_bytes"`
	Location          string     `json:"location"`
	RetainUntil       time.Time  `json:"retain_until"`
	CapturedBy        string     `json:"captured_by"`
	CapturedAt        time.Time  `json:"captured_at"`
	Validation        string     `json:"validation"`
	Freshness         string     `json:"freshness"`
	ValidationChecks  []string   `json:"validation_checks"`
	CostUnits         int64      `json:"cost_units"`
	Failure           string     `json:"failure,omitempty"`
	Recoverable       bool       `json:"recoverable"`
	Ciphertext        string     `json:"ciphertext,omitempty"`
	Nonce             string     `json:"nonce,omitempty"`
}
type Store struct {
	root string
	key  []byte
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
	keyPath := filepath.Join(root, ".snapshot-key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err == nil {
			err = os.WriteFile(keyPath, key, 0600)
		}
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("protection encryption key unavailable")
	}
	return &Store{root: root, key: key, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}
func (s *Store) Create(repo, actor string, p Plan) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(p) {
		return Plan{}, ErrInvalid
	}
	now := s.now()
	p.ID = id()
	p.RepositoryID = repo
	p.Version = 1
	p.CreatedBy = actor
	p.UpdatedBy = actor
	p.CreatedAt = now
	p.UpdatedAt = now
	p.Captures = []Capture{}
	return p, s.write(p)
}
func (s *Store) Revise(idv string, expected int, actor string, p Plan) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, e := s.read(idv)
	if e != nil {
		return Plan{}, e
	}
	if old.Version != expected {
		return Plan{}, ErrConflict
	}
	if !valid(p) || p.CommitmentID != old.CommitmentID {
		return Plan{}, ErrInvalid
	}
	p.ID = old.ID
	p.RepositoryID = old.RepositoryID
	p.Version = old.Version + 1
	p.CreatedBy = old.CreatedBy
	p.CreatedAt = old.CreatedAt
	p.UpdatedBy = actor
	p.UpdatedAt = s.now()
	p.Captures = old.Captures
	return p, s.write(p)
}
func (s *Store) Capture(idv, actor string, expected int, source Source) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := s.read(idv)
	if e != nil {
		return Plan{}, e
	}
	if p.Version != expected {
		return Plan{}, ErrConflict
	}
	if source.Revision == "" || len(source.Entries) == 0 || len(source.Payload) == 0 {
		return Plan{}, ErrInvalid
	}
	manifest, _ := json.Marshal(source.Entries)
	mh := sha256.Sum256(manifest)
	plain, _ := json.Marshal(struct {
		Manifest []Entry `json:"manifest"`
		Payload  []byte  `json:"payload"`
	}{source.Entries, source.Payload})
	block, e := aes.NewCipher(s.key)
	if e != nil {
		return Plan{}, e
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		return Plan{}, e
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return Plan{}, e
	}
	sealed := gcm.Seal(nil, nonce, plain, []byte(p.ID))
	now := s.now()
	c := Capture{ID: id(), PlanVersion: p.Version, CommitmentVersion: p.CommitmentVersion, Resources: append([]Resource(nil), p.Resources...), FreshnessMinutes: p.FreshnessMinutes, SourceRevision: source.Revision, ManifestSHA256: hex.EncodeToString(mh[:]), EntryCount: len(source.Entries), PlaintextBytes: int64(len(plain)), StoredBytes: int64(len(sealed)), Location: p.Destination, RetainUntil: now.Add(time.Duration(p.RetentionDays) * 24 * time.Hour), CapturedBy: actor, CapturedAt: now, Validation: "verified", Freshness: "fresh", ValidationChecks: append([]string(nil), p.ValidationChecks...), CostUnits: int64((len(sealed) + 1023) / 1024), Recoverable: true, Ciphertext: hex.EncodeToString(sealed), Nonce: hex.EncodeToString(nonce)}
	p.Captures = append(p.Captures, c)
	p.UpdatedAt = now
	if e = s.write(p); e != nil {
		return Plan{}, e
	}
	return s.project(p), nil
}
func (s *Store) Get(idv string) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := s.read(idv)
	return s.project(p), e
}
func (s *Store) Restore(idv, captureID string) (RestoredSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := s.read(idv)
	if e != nil {
		return RestoredSource{}, e
	}
	var capture *Capture
	for i := range p.Captures {
		if p.Captures[i].ID == captureID {
			capture = &p.Captures[i]
			break
		}
	}
	if capture == nil {
		return RestoredSource{}, ErrNotFound
	}
	if s.now().After(capture.RetainUntil) {
		return RestoredSource{}, ErrInvalid
	}
	sealed, e1 := hex.DecodeString(capture.Ciphertext)
	nonce, e2 := hex.DecodeString(capture.Nonce)
	block, e3 := aes.NewCipher(s.key)
	if e1 != nil || e2 != nil || e3 != nil {
		return RestoredSource{}, ErrInvalid
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil || len(nonce) != gcm.NonceSize() {
		return RestoredSource{}, ErrInvalid
	}
	plain, e := gcm.Open(nil, nonce, sealed, []byte(p.ID))
	if e != nil {
		return RestoredSource{}, ErrInvalid
	}
	var body struct {
		Manifest []Entry `json:"manifest"`
		Payload  []byte  `json:"payload"`
	}
	if json.Unmarshal(plain, &body) != nil || len(body.Manifest) != capture.EntryCount {
		return RestoredSource{}, ErrInvalid
	}
	m, _ := json.Marshal(body.Manifest)
	sum := sha256.Sum256(m)
	if hex.EncodeToString(sum[:]) != capture.ManifestSHA256 {
		return RestoredSource{}, ErrInvalid
	}
	return RestoredSource{Manifest: body.Manifest, Payload: body.Payload}, nil
}
func (s *Store) List(repo string) ([]Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Plan{}
	for _, x := range entries {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		p, xerr := s.read(strings.TrimSuffix(x.Name(), ".json"))
		if xerr != nil {
			return nil, xerr
		}
		if p.RepositoryID == repo {
			out = append(out, s.project(p))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) project(p Plan) Plan {
	now := s.now()
	for i := range p.Captures {
		c := &p.Captures[i]
		failure := ""
		sealed, e1 := hex.DecodeString(c.Ciphertext)
		nonce, e2 := hex.DecodeString(c.Nonce)
		block, e3 := aes.NewCipher(s.key)
		if e3 != nil {
			failure = "encryption_key_unavailable"
		} else if gcm, e := cipher.NewGCM(block); e != nil {
			failure = "encryption_key_unavailable"
		} else if e1 != nil || e2 != nil || len(nonce) != gcm.NonceSize() || len(sealed) < gcm.Overhead() {
			failure = "corrupt_snapshot"
		} else if plain, e := gcm.Open(nil, nonce, sealed, []byte(p.ID)); e != nil {
			failure = "corrupt_snapshot"
		} else {
			var body struct {
				Manifest []Entry `json:"manifest"`
				Payload  []byte  `json:"payload"`
			}
			if json.Unmarshal(plain, &body) != nil {
				failure = "corrupt_snapshot"
			} else {
				m, _ := json.Marshal(body.Manifest)
				sum := sha256.Sum256(m)
				if hex.EncodeToString(sum[:]) != c.ManifestSHA256 || len(body.Manifest) != c.EntryCount {
					failure = "manifest_mismatch"
				}
			}
		}
		if now.After(c.RetainUntil) {
			failure = "retention_expired"
		}
		freshnessMinutes := c.FreshnessMinutes
		if freshnessMinutes == 0 {
			freshnessMinutes = p.FreshnessMinutes
		}
		if now.After(c.CapturedAt.Add(time.Duration(freshnessMinutes) * time.Minute)) {
			c.Freshness = "stale"
		} else {
			c.Freshness = "fresh"
		}
		c.Failure = failure
		c.Recoverable = failure == ""
		if failure != "" {
			c.Validation = "failed"
		}
		// Encryption material is persisted for recovery but never projected.
		c.Ciphertext = ""
		c.Nonce = ""
	}
	return p
}
func valid(p Plan) bool {
	if p.Name == "" || p.CommitmentID == "" || p.CommitmentVersion < 1 || (p.Mode != "snapshot" && p.Mode != "replica") || len(p.Resources) == 0 || !strings.HasPrefix(p.Destination, "vault://") || p.Jurisdiction == "" || p.RetentionDays < 1 || p.FreshnessMinutes < 1 || len(p.AccessorIDs) == 0 || len(p.ValidationChecks) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, r := range p.Resources {
		if r.TargetID == "" || seen[r.TargetID] || (r.Kind != "repository" && r.Kind != "environment") {
			return false
		}
		seen[r.TargetID] = true
	}
	return true
}
func (s *Store) read(idv string) (Plan, error) {
	var p Plan
	b, e := os.ReadFile(filepath.Join(s.root, idv+".json"))
	if os.IsNotExist(e) {
		return p, ErrNotFound
	}
	if e != nil {
		return p, e
	}
	if json.Unmarshal(b, &p) != nil {
		return p, ErrInvalid
	}
	return p, nil
}
func (s *Store) write(p Plan) error {
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".plan-")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0600)
	if _, e = tmp.Write(b); e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, p.ID+".json"))
	}
	return e
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
