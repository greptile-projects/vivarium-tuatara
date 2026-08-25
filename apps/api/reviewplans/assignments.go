package reviewplans

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

var ErrConflict = errors.New("review assignment conflict")

type MatchEvidence struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

type ReviewerSuggestion struct {
	PrincipalType   string          `json:"principal_type"`
	PrincipalID     string          `json:"principal_id"`
	AreaIDs         []string        `json:"area_ids"`
	Eligible        bool            `json:"eligible"`
	Availability    string          `json:"availability"`
	Conflict        string          `json:"conflict,omitempty"`
	ActiveLoad      int             `json:"active_load"`
	Evidence        []MatchEvidence `json:"evidence"`
	MissingEvidence []string        `json:"missing_evidence,omitempty"`
	AgentGrantID    string          `json:"agent_grant_id,omitempty"`
}

type AssignmentEvent struct {
	Action  string    `json:"action"`
	ActorID string    `json:"actor_id"`
	Reason  string    `json:"reason,omitempty"`
	At      time.Time `json:"at"`
}

type Assignment struct {
	ID             string            `json:"id"`
	RequestID      string            `json:"request_id"`
	RepositoryID   string            `json:"repository_id"`
	PullRequestID  string            `json:"pull_request_id"`
	PlanID         string            `json:"plan_id"`
	PlanVersion    int               `json:"plan_version"`
	AreaID         string            `json:"area_id"`
	PrincipalType  string            `json:"principal_type"`
	PrincipalID    string            `json:"principal_id"`
	AgentGrantID   string            `json:"agent_grant_id,omitempty"`
	Status         string            `json:"status"`
	Deadline       *time.Time        `json:"deadline,omitempty"`
	EscalationPath string            `json:"escalation_path,omitempty"`
	AssignedBy     string            `json:"assigned_by"`
	ReplacesID     string            `json:"replaces_id,omitempty"`
	Events         []AssignmentEvent `json:"events"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	ActionRequired string            `json:"action_required,omitempty"`
	Authority      string            `json:"authority"`
}

func (s *Store) ListAssignments(repo, pull string) ([]Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readAssignments(repo, pull)
}

func (s *Store) CreateAssignment(value Assignment) (Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validRequestID(value.RequestID) || value.RepositoryID == "" || value.PullRequestID == "" || value.PlanID == "" || value.PlanVersion < 1 || value.AreaID == "" || !slices.Contains([]string{"human", "agent"}, value.PrincipalType) || value.PrincipalID == "" || value.AssignedBy == "" || (value.PrincipalType == "agent" && value.AgentGrantID == "") {
		return Assignment{}, ErrInvalid
	}
	unlock, err := s.lockAssignments(value.RepositoryID, value.PullRequestID)
	if err != nil {
		return Assignment{}, err
	}
	defer unlock()
	values, err := s.readAssignments(value.RepositoryID, value.PullRequestID)
	if err != nil {
		return Assignment{}, err
	}
	for _, existing := range values {
		if existing.RequestID == value.RequestID {
			if existing.PlanID == value.PlanID && existing.AreaID == value.AreaID && existing.PrincipalType == value.PrincipalType && existing.PrincipalID == value.PrincipalID && existing.AgentGrantID == value.AgentGrantID && existing.EscalationPath == value.EscalationPath && timesEqual(existing.Deadline, value.Deadline) {
				return existing, nil
			}
			return Assignment{}, ErrConflict
		}
		if existing.PlanID == value.PlanID && existing.AreaID == value.AreaID && (existing.Status == "invited" || existing.Status == "accepted") {
			return Assignment{}, ErrConflict
		}
	}
	now := s.now()
	value.ID, value.Status, value.CreatedAt, value.UpdatedAt = newID(), "invited", now, now
	value.Events = []AssignmentEvent{{Action: "invited", ActorID: value.AssignedBy, At: now}}
	value.Authority = "Acceptance assigns only this exact review-plan area. It grants no repository, merge, secret, governance, policy, or operational authority."
	values = append(values, value)
	return value, s.writeAssignments(value.RepositoryID, value.PullRequestID, values)
}

func (s *Store) Transition(repo, pull, id, actor, action, reason string, replacement *Assignment) (Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockAssignments(repo, pull)
	if err != nil {
		return Assignment{}, err
	}
	defer unlock()
	values, err := s.readAssignments(repo, pull)
	if err != nil {
		return Assignment{}, err
	}
	index := slices.IndexFunc(values, func(v Assignment) bool { return v.ID == id })
	if index < 0 {
		return Assignment{}, ErrNotFound
	}
	v := &values[index]
	allowed := map[string][]string{"invited": {"accept", "decline", "recuse", "unavailable", "release", "replace"}, "accepted": {"decline", "recuse", "unavailable", "release", "replace"}}
	if !slices.Contains(allowed[v.Status], action) {
		return Assignment{}, ErrConflict
	}
	statuses := map[string]string{"accept": "accepted", "decline": "declined", "recuse": "recused", "unavailable": "unavailable", "release": "released", "replace": "replaced"}
	v.Status = statuses[action]
	v.UpdatedAt = s.now()
	v.Events = append(v.Events, AssignmentEvent{Action: action, ActorID: actor, Reason: strings.TrimSpace(reason), At: v.UpdatedAt})
	if action != "accept" {
		v.ActionRequired = "Assign another eligible reviewer to this area."
	}
	if action == "replace" {
		if replacement == nil || replacement.PrincipalID == "" || replacement.PrincipalID == v.PrincipalID || replacement.RequestID == "" {
			return Assignment{}, ErrInvalid
		}
		for _, current := range values {
			if current.PlanID == v.PlanID && current.AreaID == v.AreaID && current.ID != v.ID && (current.Status == "invited" || current.Status == "accepted") {
				return Assignment{}, ErrConflict
			}
		}
		n := *replacement
		n.ID, n.RepositoryID, n.PullRequestID, n.PlanID, n.PlanVersion, n.AreaID, n.Status, n.AssignedBy, n.ReplacesID, n.CreatedAt, n.UpdatedAt = newID(), repo, pull, v.PlanID, v.PlanVersion, v.AreaID, "invited", actor, v.ID, v.UpdatedAt, v.UpdatedAt
		n.Events = []AssignmentEvent{{Action: "invited", ActorID: actor, Reason: strings.TrimSpace(reason), At: n.CreatedAt}}
		n.Authority = v.Authority
		values = append(values, n)
	}
	if err = s.writeAssignments(repo, pull, values); err != nil {
		return Assignment{}, err
	}
	return *v, nil
}

func timesEqual(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
}
func (s *Store) assignmentPath(repo, pull string) string {
	return filepath.Join(s.root, repo, pull+"-assignments.json")
}
func (s *Store) lockAssignments(repo, pull string) (func(), error) {
	directory := filepath.Dir(s.assignmentPath(repo, pull))
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(directory, "."+pull+"-assignments.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		_ = lock.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	}, nil
}
func (s *Store) readAssignments(repo, pull string) ([]Assignment, error) {
	data, err := os.ReadFile(s.assignmentPath(repo, pull))
	if errors.Is(err, os.ErrNotExist) {
		return []Assignment{}, nil
	}
	if err != nil {
		return nil, err
	}
	var v []Assignment
	if json.Unmarshal(data, &v) != nil {
		return nil, ErrInvalid
	}
	return v, nil
}
func (s *Store) writeAssignments(repo, pull string, v []Assignment) error {
	if err := os.MkdirAll(filepath.Dir(s.assignmentPath(repo, pull)), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	target := s.assignmentPath(repo, pull)
	tmp, err := s.createTemp(filepath.Dir(target), ".review-assignment-*.tmp")
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
