// Package provenancegraphs retains revision-exact, permission-aware software origin graphs.
package provenancegraphs

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

var ErrNotFound = errors.New("provenance graph not found")
var ErrInvalid = errors.New("invalid provenance graph")
var ErrRequestConflict = errors.New("provenance graph request conflict")

type Citation struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision,omitempty"`
	Path       string `json:"path,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
}
type Node struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Label       string     `json:"label"`
	Revision    string     `json:"revision,omitempty"`
	License     string     `json:"license,omitempty"`
	Obligations []string   `json:"obligations,omitempty"`
	Citations   []Citation `json:"citations"`
	Confidence  string     `json:"confidence"`
	DeclaredBy  string     `json:"declared_by,omitempty"`
	Audience    string     `json:"audience"`
	AudienceIDs []string   `json:"audience_ids,omitempty"`
	Restricted  bool       `json:"restricted,omitempty"`
}
type Edge struct {
	ID             string   `json:"id"`
	From           string   `json:"from"`
	To             string   `json:"to"`
	Transformation string   `json:"transformation"`
	ToolNodeID     string   `json:"tool_node_id,omitempty"`
	Citation       Citation `json:"citation"`
	Confidence     string   `json:"confidence"`
	DeclaredBy     string   `json:"declared_by,omitempty"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	NodeID       string `json:"node_id,omitempty"`
	EdgeID       string `json:"edge_id,omitempty"`
	Message      string `json:"message"`
	AttributedTo string `json:"attributed_to,omitempty"`
}
type Graph struct {
	ID              string       `json:"id"`
	RequestID       string       `json:"request_id"`
	RepositoryID    string       `json:"repository_id"`
	Revision        string       `json:"revision"`
	PolicyID        string       `json:"policy_id,omitempty"`
	PolicyVersion   int          `json:"policy_version,omitempty"`
	Nodes           []Node       `json:"nodes"`
	Edges           []Edge       `json:"edges"`
	Diagnostics     []Diagnostic `json:"diagnostics"`
	AnalysisDigest  string       `json:"analysis_digest"`
	CreatedBy       string       `json:"created_by"`
	CreatedAt       time.Time    `json:"created_at"`
	Stale           bool         `json:"stale"`
	CurrentRevision string       `json:"current_revision,omitempty"`
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
func (s *Store) Create(g Graph) (Graph, error) {
	var out Graph
	err := s.lock(func() error {
		if !valid(g) {
			return ErrInvalid
		}
		values, e := s.listRaw(g.RepositoryID)
		if e != nil {
			return e
		}
		for _, v := range values {
			if v.RequestID == g.RequestID {
				if equivalent(v, g) {
					out = v
					return nil
				}
				return ErrRequestConflict
			}
		}
		g.ID = id()
		g.CreatedAt = s.now()
		out = g
		return s.write(g)
	})
	return out, err
}
func (s *Store) Get(id string) (Graph, error) {
	var out Graph
	e := s.lock(func() error { var x error; out, x = s.read(id); return x })
	return out, e
}
func (s *Store) List(repositoryID string) ([]Graph, error) {
	var out []Graph
	e := s.lock(func() error { var x error; out, x = s.listRaw(repositoryID); return x })
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, e
}
func valid(g Graph) bool {
	if g.RequestID == "" || g.RepositoryID == "" || len(g.Revision) != 40 || g.CreatedBy == "" || len(g.Nodes) == 0 {
		return false
	}
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		if n.ID == "" || ids[n.ID] || !one(n.Kind, "file", "commit", "fragment", "asset", "dependency", "build_step", "artifact", "contributor", "agent", "upstream_project", "license", "attestation", "tool") || n.Label == "" || !one(n.Confidence, "declared", "verified", "inferred", "unknown", "contradicted") || !one(n.Audience, "public", "repository", "restricted") {
			return false
		}
		ids[n.ID] = true
	}
	edgeIDs := map[string]bool{}
	for _, e := range g.Edges {
		if e.ID == "" || edgeIDs[e.ID] || !ids[e.From] || !ids[e.To] || !one(e.Transformation, "authored", "copied", "generated", "packaged", "compiled", "linked", "downloaded", "derived", "attested", "contributed") || !one(e.Confidence, "declared", "verified", "inferred", "unknown", "contradicted") {
			return false
		}
		edgeIDs[e.ID] = true
	}
	return true
}
func equivalent(a, b Graph) bool {
	a.ID = ""
	a.CreatedAt = time.Time{}
	b.ID = ""
	b.CreatedAt = time.Time{}
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
func (s *Store) read(id string) (Graph, error) {
	var g Graph
	b, e := os.ReadFile(s.path(id))
	if os.IsNotExist(e) {
		return g, ErrNotFound
	}
	if e != nil {
		return g, e
	}
	if json.Unmarshal(b, &g) != nil {
		return g, ErrInvalid
	}
	return g, nil
}
func (s *Store) listRaw(repo string) ([]Graph, error) {
	out := []Graph{}
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	for _, v := range es {
		if v.IsDir() || !strings.HasSuffix(v.Name(), ".json") {
			continue
		}
		g, x := s.read(strings.TrimSuffix(v.Name(), ".json"))
		if x != nil {
			return nil, x
		}
		if g.RepositoryID == repo {
			out = append(out, g)
		}
	}
	return out, nil
}
func (s *Store) write(g Graph) error {
	b, e := json.MarshalIndent(g, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.root, ".graph-")
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
		e = os.Rename(name, s.path(g.ID))
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
