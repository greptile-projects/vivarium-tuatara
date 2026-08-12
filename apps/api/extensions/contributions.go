package extensions

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrLimit = errors.New("extension contribution limit reached")

type Action struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	Description string        `json:"description"`
	Inputs      []ActionInput `json:"inputs"`
	Effects     []string      `json:"effects"`
}
type ActionInput struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
}
type Artifact struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}
type ContributionInput struct {
	IdempotencyKey string     `json:"idempotency_key"`
	RepositoryID   string     `json:"repository_id"`
	ResourceType   string     `json:"resource_type"`
	ResourceID     string     `json:"resource_id"`
	Revision       string     `json:"revision"`
	Kind           string     `json:"kind"`
	State          string     `json:"state,omitempty"`
	Title          string     `json:"title"`
	Body           string     `json:"body,omitempty"`
	Annotations    []string   `json:"annotations,omitempty"`
	Artifacts      []Artifact `json:"artifacts,omitempty"`
	Links          []string   `json:"links,omitempty"`
	Actions        []Action   `json:"actions,omitempty"`
}
type Invocation struct {
	ID               string            `json:"id"`
	ActionID         string            `json:"action_id"`
	ActorID          string            `json:"actor_id"`
	Inputs           map[string]string `json:"inputs"`
	PreviewedEffects []string          `json:"previewed_effects"`
	Status           string            `json:"status"`
	CreatedAt        time.Time         `json:"created_at"`
}
type Contribution struct {
	ID             string `json:"id"`
	InstallationID string `json:"installation_id"`
	ExtensionID    string `json:"extension_id"`
	ExtensionName  string `json:"extension_name"`
	ContributionInput
	Trusted     bool         `json:"trusted"`
	TrustReason string       `json:"trust_reason"`
	CreatedAt   time.Time    `json:"created_at"`
	Invocations []Invocation `json:"invocations"`
}

func (s *Store) PublishContribution(installation Installation, in ContributionInput) (Contribution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Contribution{}, err
	}
	defer unlock()
	if installation.Status != "active" || !validContribution(in) || !installationAllows(installation, in.RepositoryID, in.ResourceType, "write") {
		return Contribution{}, ErrInvalid
	}
	items, err := s.readContributions(in.RepositoryID, in.ResourceType, in.ResourceID)
	if err != nil {
		return Contribution{}, err
	}
	for _, item := range items {
		if item.InstallationID == installation.ID && item.IdempotencyKey == in.IdempotencyKey {
			if sameContribution(item.ContributionInput, in) {
				return item, nil
			}
			return Contribution{}, ErrConflict
		}
	}
	now := s.now().Truncate(time.Microsecond)
	recent := 0
	for _, item := range items {
		if item.InstallationID == installation.ID && now.Sub(item.CreatedAt) < time.Hour {
			recent++
		}
	}
	if recent >= 100 || contributionWeight(in) > 20000 {
		return Contribution{}, ErrLimit
	}
	id, err := newID()
	if err != nil {
		return Contribution{}, err
	}
	v := Contribution{ID: id, InstallationID: installation.ID, ExtensionID: installation.ExtensionID, ExtensionName: installation.ExtensionName, ContributionInput: in, Trusted: true, TrustReason: "installation authority and exact resource revision were validated at publication", CreatedAt: now, Invocations: []Invocation{}}
	if err = writeAtomic(s.contributionPath(in.RepositoryID, in.ResourceType, in.ResourceID, id), v); err != nil {
		return Contribution{}, err
	}
	return v, nil
}
func (s *Store) ListContributions(repositoryID, resourceType, resourceID string) ([]Contribution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readContributions(repositoryID, resourceType, resourceID)
}
func (s *Store) Invoke(repositoryID, resourceType, resourceID, contributionID, actionID, actor string, inputs map[string]string) (Contribution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Contribution{}, err
	}
	defer unlock()
	path := s.contributionPath(repositoryID, resourceType, resourceID, contributionID)
	var v Contribution
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	installation, e := s.readInstallation(v.InstallationID)
	if e != nil || installation.Status != "active" || !installationAllows(installation, repositoryID, resourceType, "write") {
		return v, ErrInvalid
	}
	var selected *Action
	for i := range v.Actions {
		if v.Actions[i].ID == actionID {
			selected = &v.Actions[i]
			break
		}
	}
	if selected == nil {
		return v, ErrInvalid
	}
	if len(v.Invocations) >= 100 {
		return v, ErrLimit
	}
	cleanInputs := map[string]string{}
	for _, field := range selected.Inputs {
		value := strings.TrimSpace(inputs[field.Name])
		if field.Required && value == "" {
			return v, ErrInvalid
		}
		if len(value) > 1000 {
			return v, ErrInvalid
		}
		cleanInputs[field.Name] = value
	}
	id, e := newID()
	if e != nil {
		return v, e
	}
	v.Invocations = append(v.Invocations, Invocation{ID: id, ActionID: actionID, ActorID: actor, Inputs: cleanInputs, PreviewedEffects: append([]string(nil), selected.Effects...), Status: "requested", CreatedAt: s.now().Truncate(time.Microsecond)})
	if e = writeAtomic(path, v); e != nil {
		return Contribution{}, e
	}
	return v, nil
}
func (s *Store) contributionPath(repo, kind, resource, id string) string {
	return filepath.Join(s.root, "contributions", repo, kind, resource, id+".json")
}
func (s *Store) readContributions(repo, kind, resource string) ([]Contribution, error) {
	dir := filepath.Join(s.root, "contributions", repo, kind, resource)
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return []Contribution{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Contribution{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var v Contribution
		b, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			return nil, e
		}
		if json.Unmarshal(b, &v) != nil {
			return nil, ErrInvalid
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func installationAllows(v Installation, repo, resource, action string) bool {
	found := false
	for _, id := range v.RepositoryIDs {
		found = found || id == repo
	}
	if !found {
		return false
	}
	for _, p := range v.EffectiveAccess {
		if p.Resource == resource {
			for _, a := range p.Actions {
				if a == action || a == "contribute" || a == "comment" {
					return true
				}
			}
		}
	}
	return false
}
func validContribution(v ContributionInput) bool {
	if len(v.IdempotencyKey) < 8 || len(v.IdempotencyKey) > 200 || len(v.RepositoryID) != 32 || len(v.ResourceID) != 32 || len(v.Revision) != 40 || !oneOf(v.ResourceType, "pull_requests", "proposals", "issues", "releases", "deployments") || !oneOf(v.Kind, "status", "check", "annotation", "artifact", "link", "comment", "action") || strings.TrimSpace(v.Title) == "" || len(v.Title) > 200 || len(v.Body) > 10000 || len(v.Annotations) > 50 || len(v.Artifacts) > 20 || len(v.Links) > 20 || len(v.Actions) > 10 {
		return false
	}
	seen := map[string]bool{}
	for _, annotation := range v.Annotations {
		if len(annotation) > 2000 {
			return false
		}
	}
	for _, link := range v.Links {
		parsed, err := url.Parse(link)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || len(link) > 2000 {
			return false
		}
	}
	for _, artifact := range v.Artifacts {
		parsed, err := url.Parse(artifact.URL)
		if artifact.Name == "" || len(artifact.Name) > 200 || err != nil || parsed.Scheme != "https" || parsed.Host == "" || len(artifact.SHA256) != 64 {
			return false
		}
	}
	for _, a := range v.Actions {
		if a.ID == "" || len(a.ID) > 100 || a.Label == "" || len(a.Label) > 200 || len(a.Description) > 2000 || len(a.Inputs) > 20 || len(a.Effects) == 0 || len(a.Effects) > 20 || seen[a.ID] {
			return false
		}
		seen[a.ID] = true
		inputs := map[string]bool{}
		for _, input := range a.Inputs {
			if input.Name == "" || input.Label == "" || len(input.Name) > 100 || len(input.Label) > 200 || len(input.Default) > 1000 || inputs[input.Name] {
				return false
			}
			inputs[input.Name] = true
		}
		for _, effect := range a.Effects {
			if strings.TrimSpace(effect) == "" || len(effect) > 1000 {
				return false
			}
		}
	}
	return true
}
func contributionWeight(v ContributionInput) int {
	n := len(v.Title) + len(v.Body)
	for _, x := range v.Annotations {
		n += len(x)
	}
	for _, x := range v.Links {
		n += len(x)
	}
	return n
}
func sameContribution(a, b ContributionInput) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}
