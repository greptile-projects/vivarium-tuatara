// Package impacts persists revision-exact, collaborative prospective change assessments.
package impacts

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

var ErrNotFound = errors.New("impact assessment not found")
var ErrInvalid = errors.New("invalid impact assessment")
var ErrConflict = errors.New("impact assessment version conflict")

type Source struct {
	Kind          string `json:"kind"`
	Path          string `json:"path,omitempty"`
	StartLine     int    `json:"start_line,omitempty"`
	EndLine       int    `json:"end_line,omitempty"`
	ExplanationID string `json:"explanation_id,omitempty"`
	EntryID       string `json:"entry_id,omitempty"`
	Diff          string `json:"diff,omitempty"`
}
type Evidence struct {
	Kind         string `json:"kind"`
	RepositoryID string `json:"repository_id"`
	Revision     string `json:"revision"`
	Path         string `json:"path,omitempty"`
	Line         int    `json:"line,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Label        string `json:"label"`
	OwnerID      string `json:"owner_id,omitempty"`
	State        string `json:"state,omitempty"`
	Verification string `json:"verification,omitempty"`
}
type Item struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Summary   string     `json:"summary"`
	Status    string     `json:"status"`
	Evidence  []Evidence `json:"evidence,omitempty"`
	AddedBy   string     `json:"added_by"`
	CreatedAt time.Time  `json:"created_at"`
}
type Participant struct {
	UserID    string    `json:"user_id"`
	InvitedBy string    `json:"invited_by,omitempty"`
	JoinedAt  time.Time `json:"joined_at"`
}
type AcknowledgementRequest struct {
	ID              string     `json:"id"`
	RepositoryID    string     `json:"repository_id"`
	OwnerID         string     `json:"owner_id"`
	RequestedBy     string     `json:"requested_by"`
	Note            string     `json:"note,omitempty"`
	RequestedAt     time.Time  `json:"requested_at"`
	AcknowledgedBy  string     `json:"acknowledged_by,omitempty"`
	Acknowledgement string     `json:"acknowledgement,omitempty"`
	AcknowledgedAt  *time.Time `json:"acknowledged_at,omitempty"`
}
type Assessment struct {
	ID                      string                   `json:"id"`
	RepositoryID            string                   `json:"repository_id"`
	Revision                string                   `json:"revision"`
	Title                   string                   `json:"title"`
	Source                  Source                   `json:"source"`
	CreatedBy               string                   `json:"created_by"`
	Participants            []Participant            `json:"participants"`
	Items                   []Item                   `json:"items"`
	AcknowledgementRequests []AcknowledgementRequest `json:"acknowledgement_requests"`
	AnalysisStatus          string                   `json:"analysis_status"`
	AnalysisReason          string                   `json:"analysis_reason,omitempty"`
	Version                 int                      `json:"version"`
	CreatedAt               time.Time                `json:"created_at"`
	UpdatedAt               time.Time                `json:"updated_at"`
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
func (s *Store) Create(v Assessment) (Assessment, error) {
	return s.mutate("", func(value *Assessment) error {
		if value.RepositoryID == "" || len(value.Revision) != 40 || strings.TrimSpace(value.Title) == "" || value.CreatedBy == "" || len(value.Items) == 0 {
			return ErrInvalid
		}
		value.ID = randomID()
		value.Title = strings.TrimSpace(value.Title)
		value.Version = 1
		value.CreatedAt = s.now()
		value.UpdatedAt = value.CreatedAt
		value.Participants = []Participant{{UserID: value.CreatedBy, JoinedAt: value.CreatedAt}}
		return nil
	}, &v)
}
func (s *Store) Update(id string, version int, fn func(*Assessment) error) (Assessment, error) {
	return s.mutate(id, func(v *Assessment) error {
		if v.Version != version {
			return ErrConflict
		}
		if err := fn(v); err != nil {
			return err
		}
		v.Version++
		v.UpdatedAt = s.now()
		return nil
	}, nil)
}
func (s *Store) AddParticipant(id string, version int, actor, user string) (Assessment, error) {
	return s.Update(id, version, func(v *Assessment) error {
		if !participant(v, actor) || user == "" {
			return ErrNotFound
		}
		for _, p := range v.Participants {
			if p.UserID == user {
				return nil
			}
		}
		v.Participants = append(v.Participants, Participant{UserID: user, InvitedBy: actor, JoinedAt: s.now()})
		return nil
	})
}
func (s *Store) AddItem(id string, version int, item Item) (Assessment, error) {
	return s.Update(id, version, func(v *Assessment) error {
		if !participant(v, item.AddedBy) || !validKind(item.Kind) || strings.TrimSpace(item.Summary) == "" || len(item.Summary) > 2000 {
			return ErrInvalid
		}
		item.ID = randomID()
		item.Summary = strings.TrimSpace(item.Summary)
		if item.Status == "" {
			item.Status = "unknown"
		}
		if item.Status != "candidate" && item.Status != "accepted_risk" && item.Status != "unknown" && item.Status != "verification_required" {
			return ErrInvalid
		}
		item.CreatedAt = s.now()
		v.Items = append(v.Items, item)
		return nil
	})
}
func (s *Store) Request(id string, version int, x AcknowledgementRequest) (Assessment, error) {
	return s.Update(id, version, func(v *Assessment) error {
		if !participant(v, x.RequestedBy) || x.RepositoryID == "" || x.OwnerID == "" {
			return ErrInvalid
		}
		for _, old := range v.AcknowledgementRequests {
			if old.RepositoryID == x.RepositoryID && old.OwnerID == x.OwnerID {
				return ErrConflict
			}
		}
		x.ID = randomID()
		x.Note = strings.TrimSpace(x.Note)
		x.RequestedAt = s.now()
		v.AcknowledgementRequests = append(v.AcknowledgementRequests, x)
		return nil
	})
}
func (s *Store) Acknowledge(id string, version int, requestID, actor, note string) (Assessment, error) {
	return s.Update(id, version, func(v *Assessment) error {
		for i := range v.AcknowledgementRequests {
			x := &v.AcknowledgementRequests[i]
			if x.ID == requestID && x.OwnerID == actor && x.AcknowledgedBy == "" {
				now := s.now()
				x.AcknowledgedBy = actor
				x.Acknowledgement = strings.TrimSpace(note)
				x.AcknowledgedAt = &now
				return nil
			}
		}
		return ErrNotFound
	})
}
func participant(v *Assessment, user string) bool {
	for _, p := range v.Participants {
		if p.UserID == user {
			return true
		}
	}
	return false
}
func IsParticipant(v Assessment, user string) bool { return participant(&v, user) }
func validKind(v string) bool {
	return map[string]bool{"reference": true, "test": true, "owner": true, "package": true, "interface": true, "consumer": true, "release": true, "environment": true, "risk": true, "unknown": true}[v]
}
func (s *Store) Get(id string) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v Assessment
	if err := read(filepath.Join(s.root, id+".json"), &v); err != nil {
		return v, err
	}
	return v, nil
}
func (s *Store) List(repositoryID string) ([]Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := filepath.Glob(filepath.Join(s.root, "*.json"))
	if err != nil {
		return nil, err
	}
	out := []Assessment{}
	for _, f := range files {
		var v Assessment
		if read(f, &v) == nil && v.RepositoryID == repositoryID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) mutate(id string, fn func(*Assessment) error, created *Assessment) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Assessment{}, err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Assessment{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	var v Assessment
	if created != nil {
		v = *created
	} else if err = read(filepath.Join(s.root, id+".json"), &v); err != nil {
		return v, err
	}
	if err = fn(&v); err != nil {
		return v, err
	}
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return v, err
	}
	tmp, err := os.CreateTemp(s.root, ".impact-*")
	if err != nil {
		return v, err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(body)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(s.root, v.ID+".json"))
	}
	return v, err
}
func read(path string, v any) error {
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}
func randomID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
