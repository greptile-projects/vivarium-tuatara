// Package organizations persists accountable groups, membership invitations,
// and accepted repository stewardship changes.
package organizations

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

var (
	ErrNotFound = errors.New("organization not found")
	ErrInvalid  = errors.New("invalid organization")
	ErrConflict = errors.New("organization state changed")
)

type Member struct {
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type Invitation struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	InvitedBy string    `json:"invited_by"`
	CreatedAt time.Time `json:"created_at"`
}

type Transfer struct {
	ID           string     `json:"id"`
	RepositoryID string     `json:"repository_id"`
	FromOwnerID  string     `json:"from_owner_id"`
	RequestedBy  string     `json:"requested_by"`
	Status       string     `json:"status"`
	RequestedAt  time.Time  `json:"requested_at"`
	AcceptedBy   string     `json:"accepted_by,omitempty"`
	AcceptedAt   *time.Time `json:"accepted_at,omitempty"`
}

type Organization struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Slug        string       `json:"slug"`
	Description string       `json:"description,omitempty"`
	CreatedBy   string       `json:"created_by"`
	CreatedAt   time.Time    `json:"created_at"`
	Members     []Member     `json:"members"`
	Invitations []Invitation `json:"invitations"`
	Transfers   []Transfer   `json:"transfers"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: abs, now: func() time.Time { return time.Now().UTC() }}, nil
}

func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func clean(v string, max int) (string, bool) {
	v = strings.TrimSpace(v)
	return v, v != "" && len([]rune(v)) <= max && !strings.ContainsAny(v, "\x00\r\n")
}

func (s *Store) locked(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

func (s *Store) Create(name, slug, description, actor string) (Organization, error) {
	name, ok := clean(name, 100)
	if !ok || !validID(actor) {
		return Organization{}, ErrInvalid
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" || len(slug) > 60 {
		return Organization{}, ErrInvalid
	}
	for _, r := range slug {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
			return Organization{}, ErrInvalid
		}
	}
	if len(description) > 1000 {
		return Organization{}, ErrInvalid
	}
	var created Organization
	err := s.locked(func() error {
		items, err := s.list()
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Slug == slug {
				return ErrConflict
			}
		}
		id, err := newID()
		if err != nil {
			return err
		}
		now := s.now().Truncate(time.Microsecond)
		created = Organization{ID: id, Name: name, Slug: slug, Description: strings.TrimSpace(description), CreatedBy: actor, CreatedAt: now, Members: []Member{{UserID: actor, Role: "owner", JoinedAt: now}}, Invitations: []Invitation{}, Transfers: []Transfer{}}
		return s.write(created)
	})
	return created, err
}

func (s *Store) Get(id string) (Organization, error) {
	if !validID(id) {
		return Organization{}, ErrNotFound
	}
	var v Organization
	data, err := os.ReadFile(filepath.Join(s.root, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return v, ErrNotFound
	}
	if err != nil || json.Unmarshal(data, &v) != nil || v.ID != id {
		return v, ErrNotFound
	}
	return v, nil
}
func (s *Store) ListFor(user string) ([]Organization, error) {
	items, err := s.list()
	if err != nil {
		return nil, err
	}
	out := []Organization{}
	for _, v := range items {
		if HasRole(v, user, "") || invited(v, user) {
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *Store) list() ([]Organization, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	out := []Organization{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !validID(id) {
			continue
		}
		v, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func HasRole(v Organization, user, role string) bool {
	for _, m := range v.Members {
		if m.UserID == user && (role == "" || m.Role == role) {
			return true
		}
	}
	return false
}
func invited(v Organization, user string) bool {
	for _, i := range v.Invitations {
		if i.UserID == user {
			return true
		}
	}
	return false
}

func (s *Store) mutate(id string, fn func(*Organization) error) (Organization, error) {
	var out Organization
	err := s.locked(func() error {
		v, err := s.Get(id)
		if err != nil {
			return err
		}
		if err = fn(&v); err != nil {
			return err
		}
		if err = s.write(v); err != nil {
			return err
		}
		out = v
		return nil
	})
	return out, err
}
func (s *Store) Invite(id, actor, user string) (Organization, error) {
	if !validID(user) || actor == user {
		return Organization{}, ErrInvalid
	}
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		if HasRole(*v, user, "") {
			return ErrConflict
		}
		if invited(*v, user) {
			return nil
		}
		iid, e := newID()
		if e != nil {
			return e
		}
		v.Invitations = append(v.Invitations, Invitation{ID: iid, UserID: user, InvitedBy: actor, CreatedAt: s.now().Truncate(time.Microsecond)})
		return nil
	})
}
func (s *Store) AcceptInvitation(id, invitationID, actor string) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if HasRole(*v, actor, "") {
			return nil
		}
		for i, x := range v.Invitations {
			if x.ID == invitationID && x.UserID == actor {
				v.Invitations = append(v.Invitations[:i], v.Invitations[i+1:]...)
				v.Members = append(v.Members, Member{UserID: actor, Role: "member", JoinedAt: s.now().Truncate(time.Microsecond)})
				return nil
			}
		}
		return ErrNotFound
	})
}
func (s *Store) RemoveMember(id, actor, user string) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		for i, m := range v.Members {
			if m.UserID == user {
				if m.Role == "owner" {
					return ErrConflict
				}
				v.Members = append(v.Members[:i], v.Members[i+1:]...)
				return nil
			}
		}
		return nil
	})
}
func (s *Store) RequestTransfer(id, repositoryID, owner string) (Organization, error) {
	if !validID(repositoryID) {
		return Organization{}, ErrInvalid
	}
	return s.mutate(id, func(v *Organization) error {
		for _, t := range v.Transfers {
			if t.RepositoryID == repositoryID && t.Status == "pending" {
				if t.FromOwnerID == owner {
					return nil
				}
				return ErrConflict
			}
		}
		tid, e := newID()
		if e != nil {
			return e
		}
		v.Transfers = append(v.Transfers, Transfer{ID: tid, RepositoryID: repositoryID, FromOwnerID: owner, RequestedBy: owner, Status: "pending", RequestedAt: s.now().Truncate(time.Microsecond)})
		return nil
	})
}
func (s *Store) AcceptTransfer(id, transferID, actor string, apply func(Transfer, Organization) error) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		for i := range v.Transfers {
			t := &v.Transfers[i]
			if t.ID == transferID {
				if t.Status == "accepted" {
					return nil
				}
				if apply != nil {
					if e := apply(*t, *v); e != nil {
						return e
					}
				}
				now := s.now().Truncate(time.Microsecond)
				t.Status = "accepted"
				t.AcceptedBy = actor
				t.AcceptedAt = &now
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s *Store) write(v Organization) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".writing-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(append(data, '\n'))
	}
	if err == nil {
		err = tmp.Sync()
	}
	if e := tmp.Close(); err == nil {
		err = e
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	return err
}
