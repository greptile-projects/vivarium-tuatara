package workspaces

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Policy struct {
	Version         int       `json:"version"`
	MaxCPUs         float64   `json:"max_cpus"`
	MaxMemoryMB     int       `json:"max_memory_mb"`
	MaxStorageMB    int       `json:"max_storage_mb"`
	Network         string    `json:"network"`
	IdleMinutes     int       `json:"idle_minutes"`
	MaxRuntimeHours int       `json:"max_runtime_hours"`
	RetentionHours  int       `json:"retention_hours"`
	Sharing         string    `json:"sharing"`
	AgentExecution  bool      `json:"agent_execution"`
	UpdatedBy       string    `json:"updated_by,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

func DefaultPolicy() Policy {
	return Policy{Version: 1, MaxCPUs: 8, MaxMemoryMB: 16384, MaxStorageMB: 102400, Network: "none", IdleMinutes: 60, MaxRuntimeHours: 168, RetentionHours: 720, Sharing: "repository", AgentExecution: true}
}

func ValidatePolicy(p Policy) error {
	if p.Version < 1 || p.MaxCPUs <= 0 || p.MaxCPUs > 8 || p.MaxMemoryMB < 128 || p.MaxMemoryMB > 16384 || p.MaxStorageMB < 128 || p.MaxStorageMB > 102400 || p.Network != "none" || p.IdleMinutes < 5 || p.IdleMinutes > 10080 || p.MaxRuntimeHours < 1 || p.MaxRuntimeHours > 8760 || p.RetentionHours < p.MaxRuntimeHours || p.RetentionHours > 17520 || (p.Sharing != "private" && p.Sharing != "repository" && p.Sharing != "organization") {
		return ErrInvalid
	}
	return nil
}

func Constrain(org, repo Policy) Policy {
	if org.Version == 0 {
		return repo
	}
	if repo.MaxCPUs > org.MaxCPUs {
		repo.MaxCPUs = org.MaxCPUs
	}
	if repo.MaxMemoryMB > org.MaxMemoryMB {
		repo.MaxMemoryMB = org.MaxMemoryMB
	}
	if repo.MaxStorageMB > org.MaxStorageMB {
		repo.MaxStorageMB = org.MaxStorageMB
	}
	if repo.IdleMinutes > org.IdleMinutes {
		repo.IdleMinutes = org.IdleMinutes
	}
	if repo.MaxRuntimeHours > org.MaxRuntimeHours {
		repo.MaxRuntimeHours = org.MaxRuntimeHours
	}
	if repo.RetentionHours > org.RetentionHours {
		repo.RetentionHours = org.RetentionHours
	}
	if org.Sharing == "private" || (org.Sharing == "repository" && repo.Sharing == "organization") {
		repo.Sharing = org.Sharing
	}
	if !org.AgentExecution {
		repo.AgentExecution = false
	}
	return repo
}

func (s *Store) policyPath(scope, id string) string {
	return filepath.Join(s.root, "policies", scope+"-"+id+".json")
}
func (s *Store) GetPolicy(scope, id string) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.policyPath(scope, id))
	if errors.Is(err, os.ErrNotExist) {
		return DefaultPolicy(), nil
	}
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	if json.Unmarshal(b, &p) != nil {
		return Policy{}, ErrInvalid
	}
	return p, nil
}
func (s *Store) PutPolicy(scope, id, actor string, p Policy, expected int) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.policyPath(scope, id)
	current := DefaultPolicy()
	if b, e := os.ReadFile(path); e == nil {
		if json.Unmarshal(b, &current) != nil {
			return Policy{}, ErrInvalid
		}
	}
	if current.Version != expected {
		return Policy{}, ErrConflict
	}
	p.Version = expected + 1
	p.UpdatedBy = actor
	p.UpdatedAt = s.now()
	if ValidatePolicy(p) != nil {
		return Policy{}, ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return Policy{}, err
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return Policy{}, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return Policy{}, err
	}
	if scope == "repository" || scope == "organization" {
		entries, _ := os.ReadDir(s.root)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			w, readErr := s.readName(entry.Name())
			matches := scope == "repository" && w.RepositoryID == id || scope == "organization" && w.OrganizationID == id
			if readErr != nil || !matches || w.State == "stopped" || w.State == "expired" {
				continue
			}
			w.RebuildRequired, w.RebuildReasons, w.UpdatedAt = true, []string{scope + " workspace policy changed after launch"}, s.now()
			_ = s.write(w)
		}
	}
	return p, nil
}

type Consumption struct {
	WorkspaceID    string    `json:"workspace_id"`
	RepositoryID   string    `json:"repository_id"`
	CreatorID      string    `json:"creator_id"`
	State          string    `json:"state"`
	CPUSeconds     float64   `json:"cpu_seconds"`
	MemoryMBHours  float64   `json:"memory_mb_hours"`
	StorageMBHours float64   `json:"storage_mb_hours"`
	MeasuredAt     time.Time `json:"measured_at"`
}

func Usage(w Workspace, now time.Time) Consumption {
	end := now
	if w.StoppedAt != nil {
		end = *w.StoppedAt
	}
	hours := end.Sub(w.CreatedAt).Hours()
	if hours < 0 {
		hours = 0
	}
	return Consumption{w.ID, w.RepositoryID, w.CreatorID, w.State, hours * 3600 * w.Definition.Resources.CPUs, hours * float64(w.Definition.Resources.MemoryMB), hours * float64(w.Definition.Resources.StorageMB), now}
}
