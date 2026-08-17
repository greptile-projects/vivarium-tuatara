// Package knowledgeanswers persists inspectable, evidence-grounded project guidance.
package knowledgeanswers

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
	"time"
)

var ErrNotFound = errors.New("knowledge answer not found")
var ErrInvalid = errors.New("invalid knowledge answer")
var ErrConflict = errors.New("knowledge answer changed")

type Citation struct {
	Kind               string   `json:"kind"`
	ResourceID         string   `json:"resource_id,omitempty"`
	Revision           string   `json:"revision,omitempty"`
	Path               string   `json:"path,omitempty"`
	Symbol             string   `json:"symbol,omitempty"`
	StartLine          int      `json:"start_line,omitempty"`
	EndLine            int      `json:"end_line,omitempty"`
	Label              string   `json:"label"`
	ApplicableVersions []string `json:"applicable_versions"`
}
type Claim struct {
	ID          string     `json:"id"`
	Text        string     `json:"text"`
	Confidence  string     `json:"confidence"`
	Uncertainty string     `json:"uncertainty,omitempty"`
	Citations   []Citation `json:"citations"`
}
type Revision struct {
	ID                   string    `json:"id"`
	Number               int       `json:"number"`
	Summary              string    `json:"summary"`
	Body                 string    `json:"body"`
	AuthorID             string    `json:"author_id"`
	AuthorType           string    `json:"author_type"`
	Claims               []Claim   `json:"claims"`
	SupersedesRevisionID string    `json:"supersedes_revision_id,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}
type Response struct {
	ID         string    `json:"id"`
	RevisionID string    `json:"revision_id"`
	Kind       string    `json:"kind"`
	Body       string    `json:"body"`
	ActorID    string    `json:"actor_id"`
	CreatedAt  time.Time `json:"created_at"`
}
type Answer struct {
	ID                string     `json:"id"`
	RepositoryID      string     `json:"repository_id"`
	Question          string     `json:"question"`
	Audience          string     `json:"audience"`
	Status            string     `json:"status"`
	CurrentRevisionID string     `json:"current_revision_id"`
	Revisions         []Revision `json:"revisions"`
	Responses         []Response `json:"responses"`
	Version           int        `json:"version"`
	CreatedAt         time.Time  `json:"created_at"`
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
func (s *Store) Create(v Answer, r Revision) (Answer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validAnswer(v) || !validRevision(r) {
		return Answer{}, ErrInvalid
	}
	now := s.now()
	v.ID = id()
	v.Status = "proposed"
	v.Version = 1
	v.CreatedAt = now
	v.UpdatedAt = now
	r.ID = id()
	r.Number = 1
	r.CreatedAt = now
	assignClaimIDs(&r)
	v.CurrentRevisionID = r.ID
	v.Revisions = []Revision{r}
	return v, s.write(v)
}
func (s *Store) Get(repo, answer string) (Answer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo, answer)
}
func (s *Store) List(repo string) ([]Answer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.root, repo)
	es, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Answer{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Answer{}
	for _, e := range es {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		v, er := s.read(repo, strings.TrimSuffix(e.Name(), ".json"))
		if er != nil {
			return nil, er
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) Revise(repo, answer string, expected int, r Revision) (Answer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, answer)
	if err != nil {
		return v, err
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	if !validRevision(r) {
		return v, ErrInvalid
	}
	r.ID = id()
	r.Number = len(v.Revisions) + 1
	r.CreatedAt = s.now()
	r.SupersedesRevisionID = v.CurrentRevisionID
	assignClaimIDs(&r)
	v.Revisions = append(v.Revisions, r)
	v.CurrentRevisionID = r.ID
	v.Status = "proposed"
	v.Version++
	v.UpdatedAt = r.CreatedAt
	return v, s.write(v)
}
func (s *Store) Respond(repo, answer, actor, revision, kind, body string, expected int) (Answer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, answer)
	if err != nil {
		return v, err
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	if revision != v.CurrentRevisionID || !map[string]bool{"comment": true, "clarification_requested": true, "endorsement": true, "challenge": true}[kind] || strings.TrimSpace(body) == "" || len(body) > 4000 {
		return v, ErrInvalid
	}
	v.Responses = append(v.Responses, Response{ID: id(), RevisionID: revision, Kind: kind, Body: strings.TrimSpace(body), ActorID: actor, CreatedAt: s.now()})
	v.Version++
	v.UpdatedAt = s.now()
	return v, s.write(v)
}
func (s *Store) SetStatus(repo, answer, actor, status string, expected int) (Answer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, answer)
	if err != nil {
		return v, err
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	if !map[string]bool{"verified": true, "needs_context": true, "retired": true}[status] {
		return v, ErrInvalid
	}
	v.Status = status
	v.Version++
	v.UpdatedAt = s.now()
	v.Responses = append(v.Responses, Response{ID: id(), RevisionID: v.CurrentRevisionID, Kind: "status_changed", Body: status, ActorID: actor, CreatedAt: v.UpdatedAt})
	return v, s.write(v)
}
func validAnswer(v Answer) bool {
	return v.RepositoryID != "" && strings.TrimSpace(v.Question) != "" && len(v.Question) <= 1000 && map[string]bool{"public": true, "participants": true}[v.Audience]
}
func validRevision(r Revision) bool {
	if strings.TrimSpace(r.Summary) == "" || strings.TrimSpace(r.Body) == "" || r.AuthorID == "" || !map[string]bool{"human": true, "agent": true}[r.AuthorType] || len(r.Claims) == 0 {
		return false
	}
	for _, c := range r.Claims {
		if strings.TrimSpace(c.Text) == "" || !map[string]bool{"high": true, "medium": true, "low": true}[c.Confidence] || len(c.Citations) == 0 || (r.AuthorType == "agent" && strings.TrimSpace(c.Uncertainty) == "") {
			return false
		}
		for _, x := range c.Citations {
			if !map[string]bool{"source": true, "symbol": true, "documentation": true, "package": true, "release": true, "support_thread": true, "known_issue": true}[x.Kind] || strings.TrimSpace(x.Label) == "" || len(x.ApplicableVersions) == 0 {
				return false
			}
		}
	}
	return true
}
func assignClaimIDs(r *Revision) {
	for i := range r.Claims {
		r.Claims[i].ID = id()
	}
}
func (s *Store) read(repo, answer string) (Answer, error) {
	var v Answer
	b, err := os.ReadFile(filepath.Join(s.root, repo, answer+".json"))
	if os.IsNotExist(err) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Answer) error {
	d := filepath.Join(s.root, v.RepositoryID)
	if err := os.MkdirAll(d, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(d, ".knowledge-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(d, v.ID+".json"))
	}
	if err == nil {
		if x, e := os.Open(d); e == nil {
			err = x.Sync()
			_ = x.Close()
		} else {
			err = e
		}
	}
	return err
}
func id() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
