// Package supportthreads persists contextual developer support questions.
package supportthreads

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("support thread not found")
var ErrInvalid = errors.New("invalid support thread")
var ErrConflict = errors.New("support thread changed")
var ErrForbidden = errors.New("support thread transition forbidden")

type Target struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Label      string `json:"label"`
	Version    string `json:"version,omitempty"`
}
type Environment struct {
	OperatingSystem string   `json:"operating_system,omitempty"`
	Runtime         string   `json:"runtime,omitempty"`
	Dependencies    []string `json:"dependencies,omitempty"`
	Deployment      string   `json:"deployment,omitempty"`
	Details         string   `json:"details,omitempty"`
}
type ContactPreferences struct {
	ReplyInThread          bool   `json:"reply_in_thread"`
	Email                  string `json:"email,omitempty"`
	AllowMaintainerContact bool   `json:"allow_maintainer_contact"`
}
type Attachment struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	MediaType string    `json:"media_type"`
	Size      int       `json:"size"`
	Data      string    `json:"data,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type HistoryEntry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Reply struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
type Notification struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Kind      string    `json:"kind"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
type Diagnostic struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}
type Related struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Score  int    `json:"score"`
}
type Escalation struct {
	ID                 string    `json:"id"`
	Classification     string    `json:"classification"`
	ResourceKind       string    `json:"resource_kind"`
	ResourceID         string    `json:"resource_id"`
	ResourceURL        string    `json:"resource_url"`
	AffectedVersion    string    `json:"affected_version"`
	AcceptanceCriteria []string  `json:"acceptance_criteria"`
	Reproduction       []string  `json:"reproduction"`
	CreatedBy          string    `json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
	Status             string    `json:"status"`
	RequestedVersion   int       `json:"requested_version"`
	BaseRevision       string    `json:"base_revision"`
}
type Thread struct {
	ID                 string             `json:"id"`
	RepositoryID       string             `json:"repository_id"`
	AuthorID           string             `json:"author_id"`
	Title              string             `json:"title"`
	Body               string             `json:"body"`
	Target             Target             `json:"target"`
	Environment        Environment        `json:"environment"`
	Goal               string             `json:"goal"`
	AttemptedSteps     []string           `json:"attempted_steps"`
	Urgency            string             `json:"urgency"`
	Audience           string             `json:"audience"`
	Status             string             `json:"status"`
	ContactPreferences ContactPreferences `json:"contact_preferences"`
	Attachments        []Attachment       `json:"attachments"`
	History            []HistoryEntry     `json:"history"`
	Replies            []Reply            `json:"replies"`
	Notifications      []Notification     `json:"notifications,omitempty"`
	Diagnostics        []Diagnostic       `json:"diagnostics"`
	Related            []Related          `json:"related,omitempty"`
	Escalations        []Escalation       `json:"escalations,omitempty"`
	Version            int                `json:"version"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

// AddReply appends discussion under the same compare-and-swap boundary as
// status, escalation, and resolution changes. Only the asker and current
// repository participants may add context to a thread.
func (s *Store) AddReply(repo, id, actor, body string, expected int, maintainer bool) (Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Thread{}, err
	}
	body = strings.TrimSpace(body)
	if v.Version != expected {
		return Thread{}, ErrConflict
	}
	if actor != v.AuthorID && !maintainer {
		return Thread{}, ErrForbidden
	}
	if v.Status == "closed" || body == "" || len(body) > 10_000 {
		return Thread{}, ErrInvalid
	}
	now := s.now()
	v.Replies = append(v.Replies, Reply{ID: randomID(), ActorID: actor, Body: body, CreatedAt: now})
	if actor != v.AuthorID {
		v.Notifications = append(v.Notifications, Notification{ID: randomID(), UserID: v.AuthorID, Kind: "support_reply", Message: "A repository participant replied to your support question.", CreatedAt: now})
	}
	v.History = append(v.History, HistoryEntry{ID: randomID(), Kind: "replied", ActorID: actor, CreatedAt: now})
	v.Version++
	v.UpdatedAt = now
	v.Diagnostics = diagnostics(v)
	return v, s.write(v)
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
func (s *Store) Create(v Thread) (Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(v) {
		return Thread{}, ErrInvalid
	}
	now := s.now()
	v.ID = randomID()
	v.Status = "open"
	v.Version = 1
	v.CreatedAt = now
	v.UpdatedAt = now
	for i := range v.Attachments {
		v.Attachments[i].ID = randomID()
		v.Attachments[i].CreatedAt = now
	}
	v.History = []HistoryEntry{{ID: randomID(), Kind: "opened", ActorID: v.AuthorID, To: "open", CreatedAt: now}}
	v.Diagnostics = diagnostics(v)
	return v, s.write(v)
}
func (s *Store) Get(repo, id string) (Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(repo, id)
	if e == nil {
		v.Diagnostics = diagnostics(v)
	}
	return v, e
}
func (s *Store) List(repo string) ([]Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, e := os.ReadDir(filepath.Join(s.root, repo))
	if os.IsNotExist(e) {
		return []Thread{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Thread{}
	for _, x := range entries {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		b, e := os.ReadFile(filepath.Join(s.root, repo, x.Name()))
		if e != nil {
			return nil, e
		}
		var v Thread
		if json.Unmarshal(b, &v) != nil {
			return nil, ErrInvalid
		}
		v.Diagnostics = diagnostics(v)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) UpdateStatus(repo, id, actor, status, message string, expected int, maintainer bool) (Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(repo, id)
	if e != nil {
		return Thread{}, e
	}
	if v.Version != expected {
		return Thread{}, ErrConflict
	}
	allowed := map[string]bool{"open": true, "needs_context": true, "answered": true, "closed": true}
	if !allowed[status] {
		return Thread{}, ErrInvalid
	}
	if !maintainer && actor != v.AuthorID {
		return Thread{}, ErrForbidden
	}
	if !maintainer && status != "closed" && status != "open" {
		return Thread{}, ErrForbidden
	}
	from := v.Status
	v.Status = status
	v.Version++
	v.UpdatedAt = s.now()
	v.History = append(v.History, HistoryEntry{ID: randomID(), Kind: "status_changed", ActorID: actor, From: from, To: status, Message: strings.TrimSpace(message), CreatedAt: v.UpdatedAt})
	v.Diagnostics = diagnostics(v)
	return v, s.write(v)
}

// Escalate holds the support mutation boundary while ordinary governed work is
// created. The callback receives the immutable, privacy-safe context that may
// cross into the target; attachments and contact details never leave the thread.
func (s *Store) Escalate(repo, id, actor string, expected int, classification, resourceKind, baseRevision string, criteria []string, publish func(Thread, string, string) (string, string, error)) (Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Thread{}, err
	}
	classes := map[string]bool{"defect": true, "documentation_gap": true, "missing_example": true, "compatibility_problem": true, "product_opportunity": true}
	kinds := map[string]bool{"issue": true, "documentation_task": true, "proposal": true, "ordered_work": true}
	clean := make([]string, 0, len(criteria))
	for _, item := range criteria {
		if item = strings.TrimSpace(item); item != "" {
			clean = append(clean, item)
		}
	}
	if !classes[classification] || !kinds[resourceKind] || len(baseRevision) != 40 || len(clean) == 0 || len(clean) > 20 || publish == nil || v.Status == "closed" {
		return Thread{}, ErrInvalid
	}
	pending := -1
	for i := len(v.Escalations) - 1; i >= 0; i-- {
		candidate := v.Escalations[i]
		sameContent := candidate.Classification == classification && candidate.ResourceKind == resourceKind && candidate.CreatedBy == actor && slices.Equal(candidate.AcceptanceCriteria, clean)
		matchingRequest := sameContent && (candidate.Status == "published" || candidate.RequestedVersion == expected || candidate.Status == "pending" && expected == v.Version)
		if matchingRequest && candidate.Status == "published" {
			return v, nil
		}
		if candidate.Status == "pending" {
			if matchingRequest {
				pending = i
				break
			}
			return Thread{}, ErrConflict
		}
	}
	if pending < 0 {
		if v.Version != expected {
			return Thread{}, ErrConflict
		}
		now := s.now()
		v.Escalations = append(v.Escalations, Escalation{ID: randomID(), Classification: classification, ResourceKind: resourceKind, AffectedVersion: v.Target.Version, AcceptanceCriteria: clean, Reproduction: append([]string(nil), v.AttemptedSteps...), CreatedBy: actor, CreatedAt: now, Status: "pending", RequestedVersion: expected, BaseRevision: baseRevision})
		pending = len(v.Escalations) - 1
		v.Version++
		v.UpdatedAt = now
		if err := s.write(v); err != nil {
			return Thread{}, err
		}
	}
	resourceID, resourceURL, err := publish(v, v.Escalations[pending].ID, v.Escalations[pending].BaseRevision)
	if err != nil {
		return Thread{}, err
	}
	if strings.TrimSpace(resourceID) == "" || strings.TrimSpace(resourceURL) == "" {
		return Thread{}, ErrInvalid
	}
	now := s.now()
	v.Escalations[pending].ResourceID = resourceID
	v.Escalations[pending].ResourceURL = resourceURL
	v.Escalations[pending].Status = "published"
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, HistoryEntry{ID: randomID(), Kind: "escalated", ActorID: actor, Message: classification + " → " + resourceKind + ":" + resourceID, CreatedAt: now})
	v.Diagnostics = diagnostics(v)
	return v, s.write(v)
}

// Resolve serializes publication with all thread mutations. The publication callback
// returns compensation for its external write; a failed close invokes it before the
// error is returned, so the solution and source thread cannot diverge.
func (s *Store) Resolve(repo, id, actor, message string, expected int, maintainer bool, publish func() (func() error, error)) (Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Thread{}, err
	}
	if v.Version != expected {
		return Thread{}, ErrConflict
	}
	if !maintainer && actor != v.AuthorID {
		return Thread{}, ErrForbidden
	}
	if publish == nil {
		return Thread{}, ErrInvalid
	}
	compensate, err := publish()
	if err != nil {
		return Thread{}, err
	}
	if v.Status == "closed" {
		return v, nil
	}
	now := s.now()
	from := v.Status
	v.Status = "closed"
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, HistoryEntry{ID: randomID(), Kind: "resolved", ActorID: actor, From: from, To: "closed", Message: strings.TrimSpace(message), CreatedAt: now})
	v.Diagnostics = diagnostics(v)
	if err := s.write(v); err != nil {
		if compensate != nil {
			return Thread{}, errors.Join(err, compensate())
		}
		return Thread{}, err
	}
	return v, nil
}
func valid(v Thread) bool {
	kinds := map[string]bool{"repository": true, "package": true, "release": true, "api": true, "documented_journey": true, "error": true}
	urg := map[string]bool{"low": true, "normal": true, "high": true, "urgent": true}
	aud := map[string]bool{"public": true, "maintainers": true}
	if v.RepositoryID == "" || v.AuthorID == "" || strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.Body) == "" || !kinds[v.Target.Kind] || strings.TrimSpace(v.Target.Label) == "" || !urg[v.Urgency] || !aud[v.Audience] {
		return false
	}
	if !v.ContactPreferences.ReplyInThread && !v.ContactPreferences.AllowMaintainerContact {
		return false
	}
	for _, a := range v.Attachments {
		if !map[string]bool{"log": true, "configuration": true, "sample_code": true}[a.Kind] || a.Name == "" || a.MediaType == "" || a.Size <= 0 || a.Size > 1<<20 || a.Data == "" {
			return false
		}
	}
	return true
}
func diagnostics(v Thread) []Diagnostic {
	out := []Diagnostic{}
	add := func(k, m string) { out = append(out, Diagnostic{k, m}) }
	if strings.TrimSpace(v.Target.Version) == "" {
		add("missing_version", "Name the exact software version, release, or revision involved.")
	}
	if v.Environment.OperatingSystem == "" && v.Environment.Runtime == "" && v.Environment.Details == "" {
		add("missing_environment", "Describe the operating system, runtime, deployment, or other relevant environment.")
	}
	if strings.TrimSpace(v.Goal) == "" {
		add("missing_goal", "Explain the outcome you are trying to achieve.")
	}
	if len(v.AttemptedSteps) == 0 {
		add("missing_attempted_steps", "List what you already tried and what happened.")
	}
	return out
}
func (s *Store) read(repo, id string) (Thread, error) {
	var v Thread
	b, e := os.ReadFile(filepath.Join(s.root, repo, id+".json"))
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
func (s *Store) write(v Thread) error {
	d := filepath.Join(s.root, v.RepositoryID)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(d, ".support-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(d, v.ID+".json"))
}
func randomID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
