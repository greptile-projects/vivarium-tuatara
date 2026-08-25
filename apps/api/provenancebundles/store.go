// Package provenancebundles retains signed, immutable release provenance claims
// and append-only notices about trust changes discovered after publication.
package provenancebundles

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

var ErrNotFound = errors.New("provenance bundle not found")
var ErrInvalid = errors.New("invalid provenance bundle")
var ErrConflict = errors.New("provenance bundle request conflict")

type Artifact struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}
type Material struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Revision    string   `json:"revision,omitempty"`
	SHA256      string   `json:"sha256,omitempty"`
	License     string   `json:"license,omitempty"`
	Obligations []string `json:"obligations,omitempty"`
	Origin      string   `json:"origin,omitempty"`
}
type Attestation struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Subject  string `json:"subject"`
	Revision string `json:"revision,omitempty"`
	Digest   string `json:"digest,omitempty"`
	Issuer   string `json:"issuer,omitempty"`
}
type Dependency struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint"`
	PackageID  string `json:"package_id,omitempty"`
	Revision   string `json:"revision,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
}
type Claim struct {
	Schema             string        `json:"schema"`
	RepositoryID       string        `json:"repository_id"`
	ReleaseID          string        `json:"release_id"`
	ReleaseVersion     string        `json:"release_version"`
	Revision           string        `json:"revision"`
	GraphID            string        `json:"graph_id"`
	GraphDigest        string        `json:"graph_digest"`
	AssessmentID       string        `json:"assessment_id"`
	AssessmentVersion  int           `json:"assessment_version"`
	PolicyID           string        `json:"policy_id"`
	PolicyVersion      int           `json:"policy_version"`
	Audience           string        `json:"audience"`
	AudienceIDs        []string      `json:"audience_ids,omitempty"`
	Artifacts          []Artifact    `json:"artifacts"`
	Materials          []Material    `json:"materials"`
	Licenses           []string      `json:"licenses"`
	Notices            []string      `json:"notices"`
	SourceAttestations []Attestation `json:"source_attestations"`
	BuildAttestations  []Attestation `json:"build_attestations"`
	Dependencies       []Dependency  `json:"dependencies"`
	Omissions          []string      `json:"omissions"`
	Verification       []string      `json:"verification"`
	PublishedAt        time.Time     `json:"published_at"`
	PublishedBy        string        `json:"published_by"`
}
type Notice struct {
	ID                    string    `json:"id"`
	RequestID             string    `json:"request_id"`
	Kind                  string    `json:"kind"`
	Severity              string    `json:"severity"`
	Summary               string    `json:"summary"`
	Evidence              string    `json:"evidence"`
	RemediationID         string    `json:"remediation_id,omitempty"`
	PropagationCampaignID string    `json:"propagation_campaign_id,omitempty"`
	CreatedBy             string    `json:"created_by"`
	CreatedAt             time.Time `json:"created_at"`
}
type Bundle struct {
	ID            string   `json:"id"`
	RequestID     string   `json:"request_id"`
	Claim         Claim    `json:"claim"`
	Payload       string   `json:"payload"`
	PayloadSHA256 string   `json:"payload_sha256"`
	Signature     string   `json:"signature"`
	PublicKey     string   `json:"public_key"`
	Algorithm     string   `json:"algorithm"`
	Notices       []Notice `json:"notices"`
	Authority     string   `json:"authority"`
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
func (s *Store) Create(in Bundle) (Bundle, error) {
	var out Bundle
	err := s.lock(func() error {
		if !valid(in) {
			return ErrInvalid
		}
		all, e := s.listRaw()
		if e != nil {
			return e
		}
		for _, v := range all {
			if v.RequestID == in.RequestID {
				if sameClaim(v.Claim, in.Claim) {
					out = v
					return nil
				}
				return ErrConflict
			}
		}
		in.ID = id()
		in.Claim.PublishedAt = s.now()
		payload, e := json.Marshal(in.Claim)
		if e != nil {
			return e
		}
		priv, pub, e := s.key()
		if e != nil {
			return e
		}
		sum := sha256.Sum256(payload)
		in.Payload = base64.RawURLEncoding.EncodeToString(payload)
		in.PayloadSHA256 = hex.EncodeToString(sum[:])
		in.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, sum[:]))
		in.PublicKey = base64.RawURLEncoding.EncodeToString(pub)
		in.Algorithm = "Ed25519-SHA256"
		in.Authority = "A bundle is evidence for the frozen release claim only; it grants no source, package, release, distribution, remediation, propagation, or repository authority."
		out = in
		return s.write(in)
	})
	return out, err
}
func (s *Store) AddNotice(bundleID, actor string, expected int, n Notice) (Bundle, error) {
	var out Bundle
	err := s.lock(func() error {
		b, e := s.read(bundleID)
		if e != nil {
			return e
		}
		if expected != len(b.Notices) {
			return ErrConflict
		}
		if n.RequestID == "" || actor == "" || !one(n.Kind, "license_changed", "attestation_revoked", "package_quarantined", "provenance_drift", "origin_gap") || !one(n.Severity, "warning", "blocking") || strings.TrimSpace(n.Summary) == "" || strings.TrimSpace(n.Evidence) == "" || len(n.Summary) > 1000 || len(n.Evidence) > 2000 {
			return ErrInvalid
		}
		for _, x := range b.Notices {
			if x.RequestID == n.RequestID {
				if x.Kind == n.Kind && x.Severity == n.Severity && x.Summary == strings.TrimSpace(n.Summary) && x.Evidence == strings.TrimSpace(n.Evidence) && x.RemediationID == n.RemediationID && x.PropagationCampaignID == n.PropagationCampaignID && x.CreatedBy == actor {
					out = b
					return nil
				}
				return ErrConflict
			}
		}
		n.ID = id()
		n.CreatedBy = actor
		n.CreatedAt = s.now()
		b.Notices = append(b.Notices, n)
		out = b
		return s.write(b)
	})
	return out, err
}
func (s *Store) Get(id string) (Bundle, error) {
	var out Bundle
	e := s.lock(func() error { var x error; out, x = s.read(id); return x })
	return out, e
}
func (s *Store) List(repositoryID, releaseID string) ([]Bundle, error) {
	var out []Bundle
	e := s.lock(func() error {
		xs, x := s.listRaw()
		if x != nil {
			return x
		}
		for _, v := range xs {
			if (repositoryID == "" || v.Claim.RepositoryID == repositoryID) && (releaseID == "" || v.Claim.ReleaseID == releaseID) {
				out = append(out, v)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Claim.PublishedAt.After(out[j].Claim.PublishedAt) })
		return nil
	})
	return out, e
}
func valid(v Bundle) bool {
	return v.RequestID != "" && len(v.Claim.RepositoryID) == 32 && len(v.Claim.ReleaseID) == 32 && len(v.Claim.Revision) == 40 && v.Claim.GraphID != "" && v.Claim.GraphDigest != "" && v.Claim.AssessmentID != "" && v.Claim.AssessmentVersion > 0 && v.Claim.PolicyID != "" && v.Claim.PolicyVersion > 0 && v.Claim.PublishedBy != "" && one(v.Claim.Audience, "public", "repository", "restricted") && len(v.Claim.Artifacts) > 0 && len(v.Claim.Verification) > 0
}
func sameClaim(a, b Claim) bool {
	a.PublishedAt = time.Time{}
	b.PublishedAt = time.Time{}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func one(v string, xs ...string) bool {
	for _, x := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func id() string                       { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Bundle, error) {
	var v Bundle
	b, e := os.ReadFile(s.path(id))
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
func (s *Store) listRaw() ([]Bundle, error) {
	out := []Bundle{}
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	for _, v := range es {
		if v.IsDir() || !strings.HasSuffix(v.Name(), ".json") || strings.HasPrefix(v.Name(), ".") {
			continue
		}
		b, x := s.read(strings.TrimSuffix(v.Name(), ".json"))
		if x != nil {
			return nil, x
		}
		out = append(out, b)
	}
	return out, nil
}
func (s *Store) write(v Bundle) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.root, ".bundle-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	if x := f.Close(); e == nil {
		e = x
	}
	if e == nil {
		e = os.Rename(name, s.path(v.ID))
	}
	if e == nil {
		d, x := os.Open(s.root)
		if x == nil {
			e = d.Sync()
			_ = d.Close()
		} else {
			e = x
		}
	}
	return e
}
func (s *Store) key() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	p := filepath.Join(s.root, ".signing-key")
	if b, e := os.ReadFile(p); e == nil {
		if len(b) != ed25519.PrivateKeySize {
			return nil, nil, ErrInvalid
		}
		k := ed25519.PrivateKey(b)
		return k, k.Public().(ed25519.PublicKey), nil
	}
	pub, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return nil, nil, e
	}
	if e = os.WriteFile(p, priv, 0600); e != nil {
		return nil, nil, e
	}
	return priv, pub, nil
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
