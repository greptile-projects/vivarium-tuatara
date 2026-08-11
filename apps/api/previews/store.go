// Package previews retains exact-revision pull request preview publications.
package previews

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
)

const ConfigPath = ".vivarium/preview.json"

var ErrNotFound = errors.New("preview not found")
var ErrInvalid = errors.New("invalid preview audience")

type Resources struct {
	CPUs           float64 `json:"cpus"`
	MemoryMB       int     `json:"memory_mb"`
	StorageMB      int     `json:"storage_mb"`
	TimeoutSeconds int     `json:"timeout_seconds"`
}
type Config struct {
	Version          int               `json:"version"`
	Image            string            `json:"image"`
	Build            string            `json:"build"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
	OutputPath       string            `json:"output_path"`
	Environment      map[string]string `json:"environment,omitempty"`
	Resources        Resources         `json:"resources"`
	Access           AccessPolicy      `json:"access"`
}
type AccessPolicy struct {
	Network  string   `json:"network"`
	Data     string   `json:"data"`
	Identity string   `json:"identity"`
	Actions  []string `json:"actions"`
}
type Invitation struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Role       string     `json:"role"`
	SourceKind string     `json:"source_kind"`
	SourceID   string     `json:"source_id,omitempty"`
	InvitedBy  string     `json:"invited_by"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	RevokedBy  string     `json:"revoked_by,omitempty"`
}
type AudienceEvent struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	ActorID      string    `json:"actor_id"`
	InvitationID string    `json:"invitation_id"`
	CreatedAt    time.Time `json:"created_at"`
}
type Feedback struct {
	ID           string    `json:"id"`
	AuthorID     string    `json:"author_id"`
	InvitationID string    `json:"invitation_id"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
}
type FindingEvidence struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Size      int    `json:"size"`
	Data      string `json:"data,omitempty"`
	Redacted  bool   `json:"redacted"`
}
type FindingComment struct {
	ID        string    `json:"id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
type FindingEvent struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Finding struct {
	ID                string            `json:"id"`
	PreviewID         string            `json:"preview_id"`
	Revision          string            `json:"revision"`
	Route             string            `json:"route"`
	Title             string            `json:"title"`
	Description       string            `json:"description"`
	Classification    string            `json:"classification"`
	Severity          string            `json:"severity"`
	Status            string            `json:"status"`
	DuplicateOf       string            `json:"duplicate_of,omitempty"`
	ReproductionSteps []string          `json:"reproduction_steps"`
	Evidence          []FindingEvidence `json:"evidence"`
	Comments          []FindingComment  `json:"comments"`
	Events            []FindingEvent    `json:"events"`
	AuthorID          string            `json:"author_id"`
	Version           int               `json:"version"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}
type Preview struct {
	ID               string          `json:"id"`
	RepositoryID     string          `json:"repository_id"`
	PullRequestID    string          `json:"pull_request_id"`
	Revision         string          `json:"revision"`
	CreatorID        string          `json:"creator_id"`
	Definition       Config          `json:"definition"`
	DefinitionSHA256 string          `json:"definition_sha256"`
	BuildRunID       string          `json:"build_run_id"`
	State            string          `json:"state"`
	Stale            bool            `json:"stale"`
	URL              string          `json:"url"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	Invitations      []Invitation    `json:"invitations"`
	AudienceEvents   []AudienceEvent `json:"audience_events"`
	Feedback         []Feedback      `json:"feedback"`
	Findings         []Finding       `json:"findings"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func ParseConfig(data []byte) (Config, string, error) {
	var c Config
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || c.Version != 1 || strings.TrimSpace(c.Build) == "" || len(c.Build) > 4000 || c.Resources.CPUs <= 0 || c.Resources.CPUs > 2 || c.Resources.MemoryMB < 64 || c.Resources.MemoryMB > 2048 || c.Resources.StorageMB < 16 || c.Resources.StorageMB > 1024 || c.Resources.TimeoutSeconds < 1 || c.Resources.TimeoutSeconds > 1800 {
		return c, "", errors.New("invalid preview definition")
	}
	if c.Access.Network != "none" || c.Access.Data != "preview_artifacts" || c.Access.Identity != "named_users" || len(c.Access.Actions) == 0 || len(c.Access.Actions) > 3 {
		return c, "", errors.New("preview access must use network none, preview_artifacts data, named_users identity, and bounded actions")
	}
	seenActions := map[string]bool{}
	for _, action := range c.Access.Actions {
		if action != "view" && action != "test" && action != "feedback" || seenActions[action] {
			return c, "", errors.New("invalid preview action")
		}
		seenActions[action] = true
	}
	if c.WorkingDirectory == "" {
		c.WorkingDirectory = "."
	}
	clean := filepath.Clean(c.WorkingDirectory)
	output := filepath.Clean(c.OutputPath)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(output) || output == "." || output == ".." || strings.HasPrefix(output, ".."+string(filepath.Separator)) {
		return c, "", errors.New("invalid preview path")
	}
	c.WorkingDirectory, c.OutputPath = clean, output
	// Reuse check validation for image and scoped environment rules.
	b, _ := json.Marshal(checkruns.Config{Version: 1, Checks: []checkruns.Definition{{Name: "preview", Image: c.Image, Command: c.Build, WorkingDirectory: c.WorkingDirectory, Environment: c.Environment, TimeoutSeconds: c.Resources.TimeoutSeconds}}})
	if _, err := checkruns.ParseConfig(b); err != nil {
		return c, "", err
	}
	normalized, _ := json.Marshal(c)
	sum := sha256.Sum256(normalized)
	return c, hex.EncodeToString(sum[:]), nil
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("preview storage root required")
	}
	root, _ = filepath.Abs(root)
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}
func (s *Store) Create(repositoryID, pullID, revision, creator, hash, runID string, c Config) (Preview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	idb := make([]byte, 16)
	_, _ = rand.Read(idb)
	p := Preview{ID: hex.EncodeToString(idb), RepositoryID: repositoryID, PullRequestID: pullID, Revision: revision, CreatorID: creator, Definition: c, DefinitionSHA256: hash, BuildRunID: runID, State: "building", CreatedAt: now, UpdatedAt: now}
	p.URL = "/repositories/" + repositoryID + "/pulls/" + pullID + "/previews/" + p.ID + "/content/"
	if err := s.write(p); err != nil {
		return Preview{}, err
	}
	return p, nil
}
func (s *Store) write(p Preview) error {
	d := filepath.Join(s.root, p.RepositoryID, p.PullRequestID)
	if err := os.MkdirAll(d, 0700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(filepath.Join(d, p.ID+".json"), b, 0600)
}

func (s *Store) Invite(repo, pull, id, actor, user, role, sourceKind, sourceID string, expiresAt time.Time) (Preview, error) {
	if actor == "" || user == "" || !slicesContains([]string{"view", "test", "feedback"}, role) || !slicesContains([]string{"user", "issue", "decision", "proposal"}, sourceKind) {
		return Preview{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !expiresAt.After(now) || expiresAt.After(now.Add(30*24*time.Hour)) {
		return Preview{}, ErrInvalid
	}
	p, err := s.Get(repo, pull, id)
	if err != nil {
		return Preview{}, err
	}
	for i := range p.Invitations {
		invitation := &p.Invitations[i]
		if invitation.UserID == user && invitation.Role == role && invitation.RevokedAt == nil && invitation.ExpiresAt.After(now) {
			if invitation.SourceKind == sourceKind && invitation.SourceID == sourceID && invitation.ExpiresAt.Equal(expiresAt) {
				return p, nil
			}
			invitation.RevokedAt, invitation.RevokedBy = &now, actor
			p.AudienceEvents = append(p.AudienceEvents, AudienceEvent{ID: newID(), Kind: "replaced", ActorID: actor, InvitationID: invitation.ID, CreatedAt: now})
			break
		}
	}
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	invitation := Invitation{ID: hex.EncodeToString(raw), UserID: user, Role: role, SourceKind: sourceKind, SourceID: sourceID, InvitedBy: actor, ExpiresAt: expiresAt, CreatedAt: now}
	p.Invitations = append(p.Invitations, invitation)
	p.AudienceEvents = append(p.AudienceEvents, AudienceEvent{ID: newID(), Kind: "invited", ActorID: actor, InvitationID: invitation.ID, CreatedAt: now})
	p.UpdatedAt = now
	return p, s.write(p)
}
func (s *Store) Revoke(repo, pull, id, invitationID, actor string) (Preview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.Get(repo, pull, id)
	if err != nil {
		return Preview{}, err
	}
	now := s.now()
	for i := range p.Invitations {
		if p.Invitations[i].ID == invitationID {
			if p.Invitations[i].RevokedAt == nil {
				p.Invitations[i].RevokedAt = &now
				p.Invitations[i].RevokedBy = actor
				p.AudienceEvents = append(p.AudienceEvents, AudienceEvent{ID: newID(), Kind: "revoked", ActorID: actor, InvitationID: invitationID, CreatedAt: now})
				p.UpdatedAt = now
			}
			return p, s.write(p)
		}
	}
	return Preview{}, ErrNotFound
}
func (s *Store) Enter(repo, pull, id, user string) (Preview, Invitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.Get(repo, pull, id)
	if err != nil {
		return Preview{}, Invitation{}, err
	}
	now := s.now()
	for _, inv := range p.Invitations {
		if inv.UserID == user && inv.RevokedAt == nil && inv.ExpiresAt.After(now) {
			for _, event := range p.AudienceEvents {
				if event.Kind == "entered" && event.InvitationID == inv.ID && event.ActorID == user {
					return p, inv, nil
				}
			}
			p.AudienceEvents = append(p.AudienceEvents, AudienceEvent{ID: newID(), Kind: "entered", ActorID: user, InvitationID: inv.ID, CreatedAt: now})
			p.UpdatedAt = now
			return p, inv, s.write(p)
		}
	}
	return Preview{}, Invitation{}, ErrNotFound
}
func (s *Store) AddFeedback(repo, pull, id, user, invitationID, body string) (Preview, error) {
	if strings.TrimSpace(body) == "" || utf8.RuneCountInString(body) > 4000 {
		return Preview{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.Get(repo, pull, id)
	if err != nil {
		return Preview{}, err
	}
	now := s.now()
	valid := false
	for _, inv := range p.Invitations {
		if inv.ID == invitationID && inv.UserID == user && inv.Role == "feedback" && inv.RevokedAt == nil && inv.ExpiresAt.After(now) {
			valid = true
			break
		}
	}
	if !valid {
		return Preview{}, ErrNotFound
	}
	feedback := Feedback{ID: newID(), AuthorID: user, InvitationID: invitationID, Body: strings.TrimSpace(body), CreatedAt: now}
	p.Feedback = append(p.Feedback, feedback)
	p.AudienceEvents = append(p.AudienceEvents, AudienceEvent{ID: newID(), Kind: "feedback", ActorID: user, InvitationID: invitationID, CreatedAt: now})
	p.UpdatedAt = now
	return p, s.write(p)
}

var sensitiveValue = regexp.MustCompile(`(?i)(authorization|cookie|password|passwd|token|secret|api[-_]?key)(\s*[:=]\s*|\s+)([^\s,;]+)`)
var bearerValue = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]+`)

func RedactSensitive(value string) (string, bool) {
	clean := sensitiveValue.ReplaceAllString(value, "$1$2[REDACTED]")
	clean = bearerValue.ReplaceAllString(clean, "Bearer [REDACTED]")
	return clean, clean != value
}
func (s *Store) AddFinding(repo, pull, id, actor, route, title, description, classification, severity, duplicateOf string, steps []string, evidence []FindingEvidence) (Preview, Finding, error) {
	if actor == "" || route == "" || !strings.HasPrefix(route, "/") || utf8.RuneCountInString(route) > 2000 || strings.TrimSpace(title) == "" || utf8.RuneCountInString(title) > 200 || utf8.RuneCountInString(description) > 10000 || !slicesContains([]string{"bug", "usability", "accessibility", "content", "performance", "question", "other"}, classification) || !slicesContains([]string{"blocking", "major", "minor", "note"}, severity) || len(steps) > 30 || len(evidence) > 12 {
		return Preview{}, Finding{}, ErrInvalid
	}
	total := 0
	for i := range evidence {
		if !slicesContains([]string{"screenshot", "recording", "console", "trace", "annotation"}, evidence[i].Kind) || evidence[i].Name == "" || len(evidence[i].Name) > 200 {
			return Preview{}, Finding{}, ErrInvalid
		}
		allowedMedia := map[string][]string{"screenshot": {"image/png", "image/jpeg", "image/webp"}, "recording": {"video/webm", "video/mp4"}, "console": {"text/plain"}, "trace": {"application/json", "text/plain"}, "annotation": {"application/json", "text/plain"}}
		if !slicesContains(allowedMedia[evidence[i].Kind], evidence[i].MediaType) {
			return Preview{}, Finding{}, ErrInvalid
		}
		decoded, decodeErr := base64.StdEncoding.DecodeString(evidence[i].Data)
		if decodeErr != nil || len(decoded) == 0 || len(decoded) > 5<<20 {
			return Preview{}, Finding{}, ErrInvalid
		}
		evidence[i].Size = len(decoded)
		total += len(decoded)
		evidence[i].ID = newID()
		if evidence[i].Kind == "console" || evidence[i].Kind == "trace" || evidence[i].Kind == "annotation" {
			clean, changed := RedactSensitive(string(decoded))
			evidence[i].Data, evidence[i].Redacted = base64.StdEncoding.EncodeToString([]byte(clean)), changed
		}
	}
	if total > 12<<20 {
		return Preview{}, Finding{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.Get(repo, pull, id)
	if err != nil {
		return Preview{}, Finding{}, err
	}
	if duplicateOf != "" {
		found := false
		for _, candidate := range p.Findings {
			if candidate.ID == duplicateOf {
				found = true
			}
		}
		if !found {
			return Preview{}, Finding{}, ErrInvalid
		}
	}
	for i := range steps {
		if utf8.RuneCountInString(steps[i]) > 2000 {
			return Preview{}, Finding{}, ErrInvalid
		}
		steps[i], _ = RedactSensitive(strings.TrimSpace(steps[i]))
	}
	route, _ = RedactSensitive(route)
	title, _ = RedactSensitive(strings.TrimSpace(title))
	description, _ = RedactSensitive(strings.TrimSpace(description))
	now := s.now()
	f := Finding{ID: newID(), PreviewID: p.ID, Revision: p.Revision, Route: route, Title: title, Description: description, Classification: classification, Severity: severity, Status: "open", DuplicateOf: duplicateOf, ReproductionSteps: steps, Evidence: evidence, AuthorID: actor, Version: 1, CreatedAt: now, UpdatedAt: now}
	f.Events = append(f.Events, FindingEvent{ID: newID(), Kind: "created", ActorID: actor, CreatedAt: now})
	if duplicateOf != "" {
		f.Events = append(f.Events, FindingEvent{ID: newID(), Kind: "related_as_duplicate", ActorID: actor, To: duplicateOf, CreatedAt: now})
	}
	p.Findings = append(p.Findings, f)
	p.UpdatedAt = now
	return p, f, s.write(p)
}
func (s *Store) MutateFinding(repo, pull, id, findingID, actor string, expected int, mutate func(*Finding) error) (Preview, Finding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.Get(repo, pull, id)
	if err != nil {
		return Preview{}, Finding{}, err
	}
	for i := range p.Findings {
		if p.Findings[i].ID == findingID {
			f := &p.Findings[i]
			if expected != f.Version {
				return Preview{}, Finding{}, ErrInvalid
			}
			if err = mutate(f); err != nil {
				return Preview{}, Finding{}, err
			}
			f.Version++
			f.UpdatedAt = s.now()
			p.UpdatedAt = f.UpdatedAt
			return p, *f, s.write(p)
		}
	}
	return Preview{}, Finding{}, ErrNotFound
}
func newID() string { raw := make([]byte, 16); _, _ = rand.Read(raw); return hex.EncodeToString(raw) }
func NewID() string { return newID() }
func slicesContains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func (s *Store) Get(repo, pull, id string) (Preview, error) {
	b, e := os.ReadFile(filepath.Join(s.root, repo, pull, filepath.Base(id)+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return Preview{}, ErrNotFound
	}
	var p Preview
	if e != nil || json.Unmarshal(b, &p) != nil {
		return p, ErrNotFound
	}
	return p, nil
}
func (s *Store) List(repo, pull, currentRevision string) ([]Preview, error) {
	entries, e := os.ReadDir(filepath.Join(s.root, repo, pull))
	if errors.Is(e, os.ErrNotExist) {
		return []Preview{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Preview{}
	for _, x := range entries {
		if filepath.Ext(x.Name()) != ".json" {
			continue
		}
		p, e := s.Get(repo, pull, strings.TrimSuffix(x.Name(), ".json"))
		if e == nil {
			p.Stale = p.Revision != currentRevision
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
