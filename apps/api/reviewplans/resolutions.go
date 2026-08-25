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

var ErrResolutionConflict = errors.New("review finding resolution conflict")

type ResolutionLink struct {
	Kind        string `json:"kind"`
	ResourceID  string `json:"resource_id"`
	ContainerID string `json:"container_id,omitempty"`
	Revision    string `json:"revision,omitempty"`
	Description string `json:"description,omitempty"`
}

type FindingResolution struct {
	ID                string           `json:"id"`
	RequestID         string           `json:"request_id"`
	RepositoryID      string           `json:"repository_id"`
	PullRequestID     string           `json:"pull_request_id"`
	FindingID         string           `json:"finding_id"`
	FindingRevision   string           `json:"finding_revision"`
	CandidateRevision string           `json:"candidate_revision"`
	ActorType         string           `json:"actor_type"`
	ActorID           string           `json:"actor_id"`
	Action            string           `json:"action"`
	Classification    string           `json:"classification,omitempty"`
	Rationale         string           `json:"rationale"`
	Dissent           string           `json:"dissent,omitempty"`
	SupersedesID      string           `json:"supersedes_id,omitempty"`
	DuplicateOfID     string           `json:"duplicate_of_id,omitempty"`
	Links             []ResolutionLink `json:"links"`
	Evidence          []WorkCitation   `json:"verification_evidence"`
	ExpiresAt         *time.Time       `json:"expires_at,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	Authority         string           `json:"authority"`
}

func (s *Store) ListFindingResolutions(repo, pull string) ([]FindingResolution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readFindingResolutions(repo, pull)
}

func (s *Store) CreateFindingResolution(value FindingResolution) (FindingResolution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockAssignments(value.RepositoryID)
	if err != nil {
		return FindingResolution{}, err
	}
	defer unlock()
	if !validRequestID(value.RequestID) || value.RepositoryID == "" || value.PullRequestID == "" || value.FindingID == "" || len(value.FindingRevision) != 40 || len(value.CandidateRevision) != 40 || !slices.Contains([]string{"human", "agent"}, value.ActorType) || value.ActorID == "" || !slices.Contains([]string{"classify", "discuss", "accept", "challenge", "supersede", "defer", "resolved", "remains_applicable", "accepted_risk", "exception"}, value.Action) || strings.TrimSpace(value.Rationale) == "" || len(value.Rationale) > 4000 || len(value.Dissent) > 2000 || len(value.Classification) > 200 || len(value.Links) > 20 || len(value.Evidence) > 20 {
		return FindingResolution{}, ErrInvalid
	}
	if value.ActorType == "agent" && slices.Contains([]string{"accept", "supersede", "resolved", "accepted_risk", "exception"}, value.Action) {
		return FindingResolution{}, ErrInvalid
	}
	if slices.Contains([]string{"accepted_risk", "exception"}, value.Action) && value.ExpiresAt == nil && value.Action == "exception" {
		return FindingResolution{}, ErrInvalid
	}
	if value.ExpiresAt != nil && (value.ExpiresAt.Before(s.now()) || value.ExpiresAt.After(s.now().Add(30*24*time.Hour))) {
		return FindingResolution{}, ErrInvalid
	}
	for _, link := range value.Links {
		if !slices.Contains([]string{"commit", "task", "change_session", "workspace", "follow_up"}, link.Kind) || strings.TrimSpace(link.ResourceID) == "" || len(link.ResourceID) > 128 || len(link.ContainerID) > 128 || len(link.Revision) > 40 || len(link.Description) > 1000 {
			return FindingResolution{}, ErrInvalid
		}
	}
	values, err := s.readFindingResolutions(value.RepositoryID, value.PullRequestID)
	if err != nil {
		return FindingResolution{}, err
	}
	for _, existing := range values {
		if existing.RequestID == value.RequestID {
			requested := value
			existing.ID, existing.CreatedAt, existing.Authority = "", time.Time{}, ""
			requested.ID, requested.CreatedAt, requested.Authority = "", time.Time{}, ""
			a, _ := json.Marshal(existing)
			b, _ := json.Marshal(requested)
			if string(a) == string(b) {
				return values[slices.IndexFunc(values, func(v FindingResolution) bool { return v.RequestID == value.RequestID })], nil
			}
			return FindingResolution{}, ErrResolutionConflict
		}
	}
	value.ID, value.CreatedAt = newID(), s.now()
	value.Rationale, value.Dissent = strings.TrimSpace(value.Rationale), strings.TrimSpace(value.Dissent)
	value.Authority = "Finding decisions preserve review reasoning only. They grant no branch, agent, task, session, workspace, check, approval, merge, exception, or operational authority."
	values = append(values, value)
	return value, s.writeFindingResolutions(value.RepositoryID, value.PullRequestID, values)
}

func (s *Store) resolutionPath(repo, pull string) string {
	return filepath.Join(s.root, repo, pull+"-finding-resolutions.json")
}
func (s *Store) readFindingResolutions(repo, pull string) ([]FindingResolution, error) {
	data, err := os.ReadFile(s.resolutionPath(repo, pull))
	if errors.Is(err, os.ErrNotExist) {
		return []FindingResolution{}, nil
	}
	if err != nil {
		return nil, err
	}
	var v []FindingResolution
	if json.Unmarshal(data, &v) != nil {
		return nil, ErrInvalid
	}
	return v, nil
}
func (s *Store) writeFindingResolutions(repo, pull string, v []FindingResolution) error {
	target := s.resolutionPath(repo, pull)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := s.createTemp(filepath.Dir(target), ".finding-resolutions-*.tmp")
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
