package reviewplans

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var ErrWorkConflict = errors.New("review work conflict")

type WorkCitation struct {
	Kind         string   `json:"kind"`
	Value        string   `json:"value"`
	Label        string   `json:"label,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	CoveredPaths []string `json:"covered_paths,omitempty"`
}

type WorkEntry struct {
	ID             string         `json:"id"`
	RequestID      string         `json:"request_id"`
	RepositoryID   string         `json:"repository_id"`
	PullRequestID  string         `json:"pull_request_id"`
	PlanID         string         `json:"plan_id"`
	PlanVersion    int            `json:"plan_version"`
	AreaID         string         `json:"area_id"`
	SourceRevision string         `json:"source_revision"`
	TargetRevision string         `json:"target_revision"`
	ActorType      string         `json:"actor_type"`
	ActorID        string         `json:"actor_id"`
	Kind           string         `json:"kind"`
	Conclusion     string         `json:"conclusion,omitempty"`
	Body           string         `json:"body"`
	Uncertainty    string         `json:"uncertainty,omitempty"`
	Citations      []WorkCitation `json:"citations"`
	RecipientType  string         `json:"recipient_type,omitempty"`
	RecipientID    string         `json:"recipient_id,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	Authority      string         `json:"authority"`
}

func (s *Store) ListWork(repo, pull string) ([]WorkEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readWork(repo, pull)
}

func (s *Store) createWork(value WorkEntry) (WorkEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockAssignments(value.RepositoryID)
	if err != nil {
		return WorkEntry{}, err
	}
	defer unlock()
	return s.createWorkLocked(value)
}

// CreateAssignedWork couples the final accepted-assignment check to work
// persistence under the same repository-wide, cross-process mutation lock used
// by assignment transitions.
func (s *Store) CreateAssignedWork(value WorkEntry, assignmentID string) (WorkEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockAssignments(value.RepositoryID)
	if err != nil {
		return WorkEntry{}, err
	}
	defer unlock()
	assignments, err := s.readAssignments(value.RepositoryID, value.PullRequestID)
	if err != nil {
		return WorkEntry{}, err
	}
	accepted := slices.ContainsFunc(assignments, func(assignment Assignment) bool {
		return assignment.ID == assignmentID && assignment.PlanID == value.PlanID && assignment.PlanVersion == value.PlanVersion && assignment.AreaID == value.AreaID && assignment.PrincipalType == value.ActorType && assignment.PrincipalID == value.ActorID && assignment.Status == "accepted"
	})
	if !accepted {
		return WorkEntry{}, ErrWorkConflict
	}
	return s.createWorkLocked(value)
}

func (s *Store) createWorkLocked(value WorkEntry) (WorkEntry, error) {
	if !validRequestID(value.RequestID) || value.RepositoryID == "" || value.PullRequestID == "" || value.PlanID == "" || value.PlanVersion < 1 || value.AreaID == "" || len(value.SourceRevision) != 40 || len(value.TargetRevision) != 40 || !slices.Contains([]string{"human", "agent"}, value.ActorType) || value.ActorID == "" || !slices.Contains([]string{"progress", "finding", "uncertainty", "question", "handoff", "decision"}, value.Kind) || strings.TrimSpace(value.Body) == "" || len(value.Body) > 4000 || len(value.Citations) > 20 {
		return WorkEntry{}, ErrInvalid
	}
	if len(value.Conclusion) > 500 || len(value.Uncertainty) > 1000 || len(value.RecipientID) > 128 {
		return WorkEntry{}, ErrInvalid
	}
	if value.ActorType == "agent" && value.Kind == "decision" || value.Kind == "handoff" && (value.RecipientID == "" || !slices.Contains([]string{"human", "agent"}, value.RecipientType)) {
		return WorkEntry{}, ErrInvalid
	}
	for _, citation := range value.Citations {
		if !slices.Contains([]string{"file", "symbol", "requirement", "diff", "check", "preview", "decision"}, citation.Kind) || strings.TrimSpace(citation.Value) == "" || len(citation.Value) > 500 || len(citation.Label) > 500 || len(citation.Domain) > 100 || len(citation.CoveredPaths) > 200 {
			return WorkEntry{}, ErrInvalid
		}
		for _, path := range citation.CoveredPaths {
			if strings.TrimSpace(path) == "" || len(path) > 500 {
				return WorkEntry{}, ErrInvalid
			}
		}
	}
	values, err := s.readWork(value.RepositoryID, value.PullRequestID)
	if err != nil {
		return WorkEntry{}, err
	}
	for _, existing := range values {
		if existing.RequestID != value.RequestID {
			continue
		}
		requested := value
		existing.ID, existing.CreatedAt, existing.Authority = "", time.Time{}, ""
		requested.ID, requested.CreatedAt, requested.Authority = "", time.Time{}, ""
		a, _ := json.Marshal(existing)
		b, _ := json.Marshal(requested)
		if string(a) == string(b) {
			return values[slices.IndexFunc(values, func(v WorkEntry) bool { return v.RequestID == value.RequestID })], nil
		}
		return WorkEntry{}, ErrWorkConflict
	}
	value.ID, value.CreatedAt = newID(), s.now()
	value.Body, value.Conclusion, value.Uncertainty = strings.TrimSpace(value.Body), strings.TrimSpace(value.Conclusion), strings.TrimSpace(value.Uncertainty)
	value.Authority = "This entry coordinates review of the named exact area. It grants no repository, evidence, approval, merge, policy, disclosure, or operational authority; agent findings never satisfy a required human decision."
	values = append(values, value)
	return value, s.writeWork(value.RepositoryID, value.PullRequestID, values)
}

func (s *Store) workPath(repo, pull string) string {
	return filepath.Join(s.root, repo, pull+"-work.json")
}
func (s *Store) readWork(repo, pull string) ([]WorkEntry, error) {
	data, err := os.ReadFile(s.workPath(repo, pull))
	if errors.Is(err, os.ErrNotExist) {
		return []WorkEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	var values []WorkEntry
	if json.Unmarshal(data, &values) != nil {
		return nil, ErrInvalid
	}
	return values, nil
}
func (s *Store) writeWork(repo, pull string, values []WorkEntry) error {
	target := s.workPath(repo, pull)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := s.createTemp(filepath.Dir(target), ".review-work-*.tmp")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = s.syncFile(tmp)
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = s.rename(path, target); err != nil {
		return err
	}
	dir, err := s.openDir(filepath.Dir(target))
	if err != nil {
		return err
	}
	err = s.syncDir(dir)
	closeErr = dir.Close()
	if err != nil {
		return err
	}
	return closeErr
}
