package packages

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const InventoryConfigPath = ".vivarium/packages.json"

var ErrInventoryInvalid = errors.New("invalid package dependency inventory")

type ManifestDependency struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint"`
}

type LockEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InventoryConfig struct {
	Version      int                  `json:"version"`
	Dependencies []ManifestDependency `json:"dependencies"`
	Lock         []LockEntry          `json:"lock"`
}

type InventoryEntry struct {
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	Constraint     string   `json:"constraint,omitempty"`
	Direct         bool     `json:"direct"`
	Paths          []string `json:"paths"`
	PackageID      string   `json:"package_id,omitempty"`
	License        string   `json:"license,omitempty"`
	Support        string   `json:"support,omitempty"`
	State          string   `json:"state"`
	ProvenanceGaps []string `json:"provenance_gaps,omitempty"`
}

type Inventory struct {
	ID           string           `json:"id"`
	RepositoryID string           `json:"repository_id"`
	CommitID     string           `json:"commit_id"`
	RecordedBy   string           `json:"recorded_by"`
	RecordedAt   time.Time        `json:"recorded_at"`
	Entries      []InventoryEntry `json:"entries"`
}

type UpdatePolicy struct {
	RepositoryID string    `json:"repository_id"`
	PackageName  string    `json:"package_name"`
	Strategy     string    `json:"strategy"`
	Action       string    `json:"action"`
	UpdatedBy    string    `json:"updated_by"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Update struct {
	ID            string           `json:"id"`
	RepositoryID  string           `json:"repository_id"`
	PackageName   string           `json:"package_name"`
	FromVersion   string           `json:"from_version"`
	ToVersion     string           `json:"to_version"`
	BaseCommit    string           `json:"base_commit"`
	ProposalID    string           `json:"proposal_id"`
	TaskID        string           `json:"task_id"`
	Manifest      InventoryConfig  `json:"manifest"`
	ReleaseNotes  string           `json:"release_notes"`
	Compatibility BuildAttestation `json:"compatibility_evidence"`
	AffectedPaths []string         `json:"affected_dependency_paths"`
	CreatedBy     string           `json:"created_by"`
	CreatedAt     time.Time        `json:"created_at"`
}

func (s *Store) PutUpdatePolicy(value UpdatePolicy) (UpdatePolicy, error) {
	value.PackageName, value.Strategy, value.Action = strings.ToLower(strings.TrimSpace(value.PackageName)), strings.ToLower(strings.TrimSpace(value.Strategy)), strings.ToLower(strings.TrimSpace(value.Action))
	if len(value.RepositoryID) != 32 || len(value.UpdatedBy) != 32 || !identityPattern.MatchString(value.PackageName) || (value.Strategy != "patch" && value.Strategy != "minor" && value.Strategy != "major") || value.Action != "proposal" {
		return UpdatePolicy{}, ErrInventoryInvalid
	}
	value.UpdatedAt = s.now().UTC().Truncate(time.Microsecond)
	body, _ := json.Marshal(value)
	dir := filepath.Join(s.root, "update-policies", value.RepositoryID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return UpdatePolicy{}, err
	}
	if err := atomicFile(filepath.Join(dir, value.PackageName+".json"), body); err != nil {
		return UpdatePolicy{}, err
	}
	return value, nil
}

func (s *Store) ListUpdatePolicies(repositoryID string) ([]UpdatePolicy, error) {
	dir := filepath.Join(s.root, "update-policies", repositoryID)
	files, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []UpdatePolicy{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []UpdatePolicy{}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(dir, file.Name()))
		if readErr != nil {
			return nil, readErr
		}
		var value UpdatePolicy
		if json.Unmarshal(body, &value) != nil {
			return nil, ErrInventoryInvalid
		}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PackageName < result[j].PackageName })
	return result, nil
}

func (s *Store) RecordUpdate(value Update) (Update, error) {
	if len(value.RepositoryID) != 32 || len(value.ProposalID) != 32 || len(value.TaskID) != 32 || len(value.BaseCommit) != 40 {
		return Update{}, ErrInventoryInvalid
	}
	items, err := s.ListUpdates(value.RepositoryID)
	if err != nil {
		return Update{}, err
	}
	for _, item := range items {
		if item.PackageName == value.PackageName && item.FromVersion == value.FromVersion && item.ToVersion == value.ToVersion && item.BaseCommit == value.BaseCommit {
			return item, nil
		}
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return Update{}, err
	}
	value.ID, value.CreatedAt = hex.EncodeToString(id), s.now().UTC().Truncate(time.Microsecond)
	body, _ := json.Marshal(value)
	dir := filepath.Join(s.root, "updates", value.RepositoryID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return Update{}, err
	}
	if err = atomicFile(filepath.Join(dir, value.ID+".json"), body); err != nil {
		return Update{}, err
	}
	return value, nil
}

// PublishUpdate serializes the cross-store collaboration callback with the
// exact base/from/to reservation. The returned value includes callback IDs on
// a later persistence failure so the caller can compensate only its own work.
func (s *Store) PublishUpdate(value Update, publish func() (string, string, error)) (Update, bool, error) {
	if len(value.RepositoryID) != 32 || len(value.BaseCommit) != 40 || !identityPattern.MatchString(value.PackageName) || !versionPattern.MatchString(value.FromVersion) || !versionPattern.MatchString(value.ToVersion) || publish == nil {
		return Update{}, false, ErrInventoryInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".update-lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Update{}, false, err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Update{}, false, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	items, err := s.ListUpdates(value.RepositoryID)
	if err != nil {
		return Update{}, false, err
	}
	for _, item := range items {
		if item.PackageName == value.PackageName && item.FromVersion == value.FromVersion && item.ToVersion == value.ToVersion && item.BaseCommit == value.BaseCommit {
			return item, false, nil
		}
	}
	value.ProposalID, value.TaskID, err = publish()
	if err != nil {
		return value, true, err
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return value, true, err
	}
	value.ID, value.CreatedAt = hex.EncodeToString(id), s.now().UTC().Truncate(time.Microsecond)
	body, err := json.Marshal(value)
	if err != nil {
		return value, true, err
	}
	dir := filepath.Join(s.root, "updates", value.RepositoryID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return value, true, err
	}
	if err = atomicFile(filepath.Join(dir, value.ID+".json"), body); err != nil {
		return value, true, err
	}
	return value, true, nil
}

func (s *Store) ListUpdates(repositoryID string) ([]Update, error) {
	dir := filepath.Join(s.root, "updates", repositoryID)
	files, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Update{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Update{}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(dir, file.Name()))
		if readErr != nil {
			return nil, readErr
		}
		var value Update
		if json.Unmarshal(body, &value) != nil {
			return nil, ErrInventoryInvalid
		}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) RecordInventory(value Inventory) (Inventory, error) {
	if len(value.RepositoryID) != 32 || len(value.CommitID) != 40 || len(value.RecordedBy) != 32 || len(value.Entries) == 0 {
		return Inventory{}, ErrInventoryInvalid
	}
	for _, entry := range value.Entries {
		if !identityPattern.MatchString(entry.Name) || (entry.Version != "" && !versionPattern.MatchString(entry.Version)) || (entry.State != "resolved" && entry.State != "stale" && entry.State != "unresolved") {
			return Inventory{}, ErrInventoryInvalid
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".inventory-lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Inventory{}, err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Inventory{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	dir := filepath.Join(s.root, "inventories", value.RepositoryID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return Inventory{}, err
	}
	if existing, getErr := s.GetInventory(value.RepositoryID, value.CommitID); getErr == nil {
		return existing, nil
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return Inventory{}, err
	}
	value.ID = hex.EncodeToString(id)
	value.RecordedAt = s.now().UTC().Truncate(time.Microsecond)
	sort.Slice(value.Entries, func(i, j int) bool { return value.Entries[i].Name < value.Entries[j].Name })
	body, err := json.Marshal(value)
	if err != nil {
		return Inventory{}, err
	}
	tmp, err := os.CreateTemp(dir, ".inventory-*")
	if err != nil {
		return Inventory{}, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(body)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return Inventory{}, err
	}
	if err = os.Rename(name, filepath.Join(dir, value.CommitID+".json")); err != nil {
		return Inventory{}, err
	}
	d, err := os.Open(dir)
	if err != nil {
		return Inventory{}, err
	}
	err = d.Sync()
	closeErr = d.Close()
	if err == nil {
		err = closeErr
	}
	return value, err
}

func (s *Store) GetInventory(repositoryID, commitID string) (Inventory, error) {
	body, err := os.ReadFile(filepath.Join(s.root, "inventories", repositoryID, commitID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return Inventory{}, ErrNotFound
	}
	if err != nil {
		return Inventory{}, err
	}
	var value Inventory
	if json.Unmarshal(body, &value) != nil || value.RepositoryID != repositoryID || value.CommitID != commitID {
		return Inventory{}, ErrNotFound
	}
	return value, nil
}

func (s *Store) ListInventories(repositoryID string) ([]Inventory, error) {
	dir := filepath.Join(s.root, "inventories", repositoryID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Inventory{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Inventory{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		value, readErr := s.GetInventory(repositoryID, strings.TrimSuffix(entry.Name(), ".json"))
		if readErr != nil {
			return nil, readErr
		}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RecordedAt.After(result[j].RecordedAt) })
	return result, nil
}

func (s *Store) ListConsumers(name, version string) ([]Inventory, error) {
	root := filepath.Join(s.root, "inventories")
	repositories, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []Inventory{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Inventory{}
	for _, repository := range repositories {
		if !repository.IsDir() {
			continue
		}
		items, readErr := s.ListInventories(repository.Name())
		if readErr != nil {
			return nil, readErr
		}
		for _, item := range items {
			for _, dependency := range item.Entries {
				if dependency.Name == name && (version == "" || dependency.Version == version) {
					result = append(result, item)
					break
				}
			}
		}
	}
	return result, nil
}
