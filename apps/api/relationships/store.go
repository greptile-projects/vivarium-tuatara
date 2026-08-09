// Package relationships stores immutable interface publications and consumer declarations.
package relationships

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
	ErrNotFound = errors.New("relationship not found")
	ErrInvalid  = errors.New("invalid relationship")
	ErrConflict = errors.New("relationship conflict")
)

type Interface struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	ReleaseID    string    `json:"release_id"`
	CommitID     string    `json:"commit_id"`
	PublishedBy  string    `json:"published_by"`
	PublishedAt  time.Time `json:"published_at"`
}

type Dependency struct {
	ID                   string    `json:"id"`
	RepositoryID         string    `json:"repository_id"`
	CommitID             string    `json:"commit_id"`
	ReleaseID            string    `json:"release_id,omitempty"`
	EnvironmentID        string    `json:"environment_id,omitempty"`
	ProviderRepositoryID string    `json:"provider_repository_id"`
	InterfaceName        string    `json:"interface_name"`
	Constraint           string    `json:"constraint"`
	DeclaredBy           string    `json:"declared_by"`
	DeclaredAt           time.Time `json:"declared_at"`
}

// Evolution is the shared, mutable decision record for changing one published
// interface. Repository evidence is frozen when the plan is created; later
// findings and acknowledgements are attributable append-only records.
type Evolution struct {
	ID                   string                     `json:"id"`
	RepositoryID         string                     `json:"repository_id"`
	InterfaceName        string                     `json:"interface_name"`
	Predecessor          Interface                  `json:"predecessor"`
	SourceKind           string                     `json:"source_kind"`
	SourceID             string                     `json:"source_id"`
	CandidateCommitID    string                     `json:"candidate_commit_id,omitempty"`
	CandidateDescription string                     `json:"candidate_description"`
	Changes              []CompatibilityChange      `json:"changes"`
	Impacts              []ConsumerImpact           `json:"impacts"`
	Strategy             string                     `json:"strategy"`
	Sequencing           string                     `json:"sequencing"`
	Exceptions           string                     `json:"exceptions,omitempty"`
	CreatedBy            string                     `json:"created_by"`
	Version              int                        `json:"version"`
	Findings             []EvolutionFinding         `json:"findings"`
	Analyses             []EvolutionAnalysis        `json:"analyses"`
	Acknowledgements     []EvolutionAcknowledgement `json:"acknowledgements"`
	MigrationTasks       []EvolutionMigrationTask   `json:"migration_tasks"`
	CreatedAt            time.Time                  `json:"created_at"`
	UpdatedAt            time.Time                  `json:"updated_at"`
}

// EvolutionMigrationTask links the cross-repository sequence to a repository-
// owned proposal task. The proposal remains the authority for assignment,
// discussion, branch state, review, and completion.
type EvolutionMigrationTask struct {
	ID                 string    `json:"id"`
	RepositoryID       string    `json:"repository_id"`
	ProposalID         string    `json:"proposal_id"`
	TaskID             string    `json:"task_id"`
	TargetVersion      string    `json:"target_version"`
	DependencyIDs      []string  `json:"dependency_ids"`
	CreatedBy          string    `json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
	Status             string    `json:"status,omitempty"`
	Ready              bool      `json:"ready"`
	AssignmentID       string    `json:"assignment_id,omitempty"`
	AssigneeType       string    `json:"assignee_type,omitempty"`
	AssigneeID         string    `json:"assignee_id,omitempty"`
	BaseRevision       string    `json:"base_revision,omitempty"`
	Branch             string    `json:"branch,omitempty"`
	PullRequestID      string    `json:"pull_request_id,omitempty"`
	ContributionStatus string    `json:"contribution_status,omitempty"`
}
type CompatibilityChange struct {
	Kind           string `json:"kind"`
	Summary        string `json:"summary"`
	Classification string `json:"classification"`
}
type ConsumerImpact struct {
	RepositoryID string `json:"repository_id"`
	OwnerID      string `json:"owner_id"`
	DependencyID string `json:"dependency_id"`
	CommitID     string `json:"commit_id"`
	Constraint   string `json:"constraint"`
	State        string `json:"state"`
}
type EvolutionFinding struct {
	ID            string    `json:"id"`
	ActorID       string    `json:"actor_id"`
	RepositoryIDs []string  `json:"repository_ids"`
	Finding       string    `json:"finding"`
	Uncertainty   string    `json:"uncertainty,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}
type EvolutionAnalysis struct {
	ID                 string    `json:"id"`
	AgentID            string    `json:"agent_id"`
	InitiatorID        string    `json:"initiator_id"`
	CredentialID       string    `json:"-"`
	StoredCredentialID string    `json:"credential_id,omitempty"`
	Mandate            string    `json:"mandate"`
	RepositoryIDs      []string  `json:"repository_ids"`
	CreatedAt          time.Time `json:"created_at"`
}
type EvolutionAcknowledgement struct {
	ActorID      string    `json:"actor_id"`
	RepositoryID string    `json:"repository_id"`
	Note         string    `json:"note,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
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
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}

func (s *Store) CreateInterface(v Interface) (Interface, error) {
	if !validID(v.RepositoryID) || !validID(v.ReleaseID) || !validCommit(v.CommitID) || !validID(v.PublishedBy) || !validName(v.Name) || !validVersion(v.Version) {
		return v, ErrInvalid
	}
	v.Name, v.Version = strings.TrimSpace(v.Name), strings.TrimSpace(v.Version)
	return v, s.create(v.RepositoryID, "interfaces", &v.ID, &v.PublishedAt, v)
}

func (s *Store) CreateDependency(v Dependency) (Dependency, error) {
	if !validID(v.RepositoryID) || !validCommit(v.CommitID) || !optionalID(v.ReleaseID) || !optionalID(v.EnvironmentID) || !validID(v.ProviderRepositoryID) || !validID(v.DeclaredBy) || !validName(v.InterfaceName) || !validConstraint(v.Constraint) {
		return v, ErrInvalid
	}
	v.InterfaceName, v.Constraint = strings.TrimSpace(v.InterfaceName), strings.TrimSpace(v.Constraint)
	return v, s.create(v.RepositoryID, "dependencies", &v.ID, &v.DeclaredAt, v)
}

func (s *Store) create(repo, kind string, id *string, created *time.Time, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		return err
	}
	*id = hex.EncodeToString(b)
	*created = s.now()
	dir := filepath.Join(s.root, repo, kind)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	switch v := value.(type) {
	case Interface:
		v.ID, v.PublishedAt = *id, *created
		value = v
	case Dependency:
		v.ID, v.DeclaredAt = *id, *created
		value = v
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".relationship-*")
	if err != nil {
		return err
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
		return err
	}
	if err = os.Rename(name, filepath.Join(dir, *id+".json")); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	closeErr = d.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

func (s *Store) ListInterfaces(repo string) ([]Interface, error) {
	var v []Interface
	err := s.list(repo, "interfaces", &v)
	sort.Slice(v, func(i, j int) bool { return v[i].PublishedAt.Before(v[j].PublishedAt) })
	return v, err
}
func (s *Store) ListDependencies(repo string) ([]Dependency, error) {
	var v []Dependency
	err := s.list(repo, "dependencies", &v)
	sort.Slice(v, func(i, j int) bool { return v[i].DeclaredAt.Before(v[j].DeclaredAt) })
	return v, err
}
func (s *Store) ListRepositoryIDs() ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && validID(e.Name()) {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *Store) CreateEvolution(v Evolution) (Evolution, error) {
	if !validID(v.RepositoryID) || !validID(v.CreatedBy) || !validID(v.Predecessor.ID) || !validName(v.InterfaceName) || (v.SourceKind != "proposal" && v.SourceKind != "pull_request") || !validID(v.SourceID) || (v.CandidateCommitID != "" && !validCommit(v.CandidateCommitID)) || !validEvolutionText(v.CandidateDescription) || !validEvolutionText(v.Strategy) || !validEvolutionText(v.Sequencing) || len(v.Changes) == 0 || len(v.Changes) > 100 {
		return v, ErrInvalid
	}
	for _, c := range v.Changes {
		if !validChange(c) {
			return v, ErrInvalid
		}
	}
	v.ID = mustID()
	v.Version = 1
	v.CreatedAt = s.now()
	v.UpdatedAt = v.CreatedAt
	return v, s.writeEvolution(v, true)
}
func (s *Store) ListEvolutions(repo string) ([]Evolution, error) {
	var v []Evolution
	if err := s.list(repo, "evolutions", &v); err != nil {
		return nil, err
	}
	sort.Slice(v, func(i, j int) bool { return v[i].CreatedAt.Before(v[j].CreatedAt) })
	return v, nil
}
func (s *Store) GetEvolution(repo, id string) (Evolution, error) {
	var v Evolution
	if !validID(repo) || !validID(id) {
		return v, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, repo, "evolutions", id+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return v, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *Store) FindEvolutionMigrationTask(repositoryID, taskID string) (Evolution, EvolutionMigrationTask, error) {
	ids, err := s.ListRepositoryIDs()
	if err != nil {
		return Evolution{}, EvolutionMigrationTask{}, err
	}
	for _, id := range ids {
		plans, listErr := s.ListEvolutions(id)
		if listErr != nil {
			return Evolution{}, EvolutionMigrationTask{}, listErr
		}
		for _, plan := range plans {
			for _, task := range plan.MigrationTasks {
				if task.RepositoryID == repositoryID && task.TaskID == taskID {
					return plan, task, nil
				}
			}
		}
	}
	return Evolution{}, EvolutionMigrationTask{}, ErrNotFound
}
func (s *Store) UpdateEvolution(repo, id, actor string, version int, strategy, sequencing, exceptions string) (Evolution, error) {
	return s.mutateEvolution(repo, id, func(v *Evolution) error {
		if version != v.Version {
			return ErrConflict
		}
		if !validEvolutionText(strategy) || !validEvolutionText(sequencing) || len(exceptions) > 10000 {
			return ErrInvalid
		}
		v.Strategy = strings.TrimSpace(strategy)
		v.Sequencing = strings.TrimSpace(sequencing)
		v.Exceptions = strings.TrimSpace(exceptions)
		v.Version++
		v.UpdatedAt = s.now()
		return nil
	})
}
func (s *Store) AcknowledgeEvolution(repo, id, actor, consumer, note string) (Evolution, error) {
	if !validID(actor) || !validID(consumer) || len(note) > 2000 {
		return Evolution{}, ErrInvalid
	}
	return s.mutateEvolution(repo, id, func(v *Evolution) error {
		for _, a := range v.Acknowledgements {
			if a.ActorID == actor && a.RepositoryID == consumer {
				return ErrConflict
			}
		}
		v.Acknowledgements = append(v.Acknowledgements, EvolutionAcknowledgement{ActorID: actor, RepositoryID: consumer, Note: strings.TrimSpace(note), CreatedAt: s.now()})
		v.Version++
		v.UpdatedAt = s.now()
		return nil
	})
}

// CreateEvolutionMigrationTask holds the evolution CAS lock while repository
// work is published. A stale request is rejected before publish runs, so a
// losing concurrent request cannot leave an unlinked proposal behind.
func (s *Store) CreateEvolutionMigrationTask(repo, id, actor, targetRepo, targetVersion string, dependencies []string, version int, publish func() (string, string, error)) (Evolution, EvolutionMigrationTask, error) {
	if !validID(actor) || !validID(targetRepo) || !validVersion(targetVersion) || len(dependencies) > 50 || publish == nil {
		return Evolution{}, EvolutionMigrationTask{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Evolution{}, EvolutionMigrationTask{}, err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Evolution{}, EvolutionMigrationTask{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	v, err := s.GetEvolution(repo, id)
	if err != nil {
		return v, EvolutionMigrationTask{}, err
	}
	if version != v.Version {
		return v, EvolutionMigrationTask{}, ErrConflict
	}
	allowed := targetRepo == v.RepositoryID
	for _, impact := range v.Impacts {
		allowed = allowed || impact.RepositoryID == targetRepo
	}
	if !allowed {
		return v, EvolutionMigrationTask{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, dependency := range dependencies {
		if !validID(dependency) || seen[dependency] {
			return v, EvolutionMigrationTask{}, ErrInvalid
		}
		found := false
		for _, existing := range v.MigrationTasks {
			found = found || existing.ID == dependency
		}
		if !found {
			return v, EvolutionMigrationTask{}, ErrInvalid
		}
		seen[dependency] = true
	}
	proposalID, taskID, err := publish()
	if err != nil {
		return v, EvolutionMigrationTask{}, err
	}
	if !validID(proposalID) || !validID(taskID) {
		return v, EvolutionMigrationTask{}, ErrInvalid
	}
	task := EvolutionMigrationTask{ID: mustID(), RepositoryID: targetRepo, ProposalID: proposalID, TaskID: taskID, TargetVersion: strings.TrimSpace(targetVersion), DependencyIDs: append([]string(nil), dependencies...), CreatedBy: actor, CreatedAt: s.now()}
	v.MigrationTasks = append(v.MigrationTasks, task)
	v.Version++
	v.UpdatedAt = s.now()
	return v, task, s.writeEvolutionUnlocked(v, false)
}
func (s *Store) AddEvolutionFinding(repo, id, actor string, repositories []string, finding, uncertainty string) (Evolution, error) {
	if !validID(actor) || !validEvolutionText(finding) || len(uncertainty) > 5000 || len(repositories) == 0 || len(repositories) > 50 {
		return Evolution{}, ErrInvalid
	}
	for _, x := range repositories {
		if !validID(x) {
			return Evolution{}, ErrInvalid
		}
	}
	return s.mutateEvolution(repo, id, func(v *Evolution) error {
		v.Findings = append(v.Findings, EvolutionFinding{ID: mustID(), ActorID: actor, RepositoryIDs: repositories, Finding: strings.TrimSpace(finding), Uncertainty: strings.TrimSpace(uncertainty), CreatedAt: s.now()})
		v.Version++
		v.UpdatedAt = s.now()
		return nil
	})
}
func (s *Store) StartEvolutionAnalysis(repo, id, initiator, credential, mandate string, repositories []string) (Evolution, EvolutionAnalysis, error) {
	if !validID(initiator) || !validID(credential) || !validEvolutionText(mandate) || len(repositories) == 0 || len(repositories) > 50 {
		return Evolution{}, EvolutionAnalysis{}, ErrInvalid
	}
	for _, x := range repositories {
		if !validID(x) {
			return Evolution{}, EvolutionAnalysis{}, ErrInvalid
		}
	}
	var a EvolutionAnalysis
	v, e := s.mutateEvolution(repo, id, func(v *Evolution) error {
		a = EvolutionAnalysis{ID: mustID(), AgentID: mustID(), InitiatorID: initiator, CredentialID: credential, StoredCredentialID: credential, Mandate: strings.TrimSpace(mandate), RepositoryIDs: repositories, CreatedAt: s.now()}
		v.Analyses = append(v.Analyses, a)
		v.Version++
		v.UpdatedAt = s.now()
		return nil
	})
	return v, a, e
}
func (s *Store) EvolutionAnalysis(repo, id, analysis, credential string) (Evolution, EvolutionAnalysis, error) {
	v, e := s.GetEvolution(repo, id)
	if e != nil {
		return v, EvolutionAnalysis{}, e
	}
	for _, a := range v.Analyses {
		if a.ID == analysis && a.StoredCredentialID == credential {
			a.CredentialID = credential
			return v, a, nil
		}
	}
	return v, EvolutionAnalysis{}, ErrNotFound
}
func (s *Store) mutateEvolution(repo, id string, fn func(*Evolution) error) (Evolution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return Evolution{}, e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); e != nil {
		return Evolution{}, e
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	v, e := s.GetEvolution(repo, id)
	if e != nil {
		return v, e
	}
	if e = fn(&v); e != nil {
		return v, e
	}
	return v, s.writeEvolutionUnlocked(v, false)
}
func (s *Store) writeEvolution(v Evolution, exclusive bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeEvolutionUnlocked(v, exclusive)
}
func (s *Store) writeEvolutionUnlocked(v Evolution, exclusive bool) error {
	dir := filepath.Join(s.root, v.RepositoryID, "evolutions")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	body, e := json.Marshal(v)
	if e != nil {
		return e
	}
	target := filepath.Join(dir, v.ID+".json")
	if exclusive {
		if _, e = os.Stat(target); e == nil {
			return ErrConflict
		}
	}
	tmp, e := os.CreateTemp(dir, ".evolution-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0600)
	if _, e = tmp.Write(body); e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, target)
	}
	return e
}
func mustID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func validEvolutionText(v string) bool { v = strings.TrimSpace(v); return v != "" && len(v) <= 10000 }
func validChange(v CompatibilityChange) bool {
	if !validName(v.Kind) || !validEvolutionText(v.Summary) {
		return false
	}
	switch v.Classification {
	case "compatible", "conditional", "breaking", "unknown":
		return true
	}
	return false
}
func (s *Store) list(repo, kind string, target any) error {
	if !validID(repo) {
		return ErrNotFound
	}
	entries, err := os.ReadDir(filepath.Join(s.root, repo, kind))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var bodies []json.RawMessage
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(s.root, repo, kind, e.Name()))
		if readErr != nil {
			return readErr
		}
		bodies = append(bodies, body)
	}
	body, _ := json.Marshal(bodies)
	return json.Unmarshal(body, target)
}

func validName(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && len(v) <= 100 && !strings.ContainsAny(v, "\r\n")
}
func validVersion(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) < 2 || len(v) > 100 || v[0] != 'v' {
		return false
	}
	_, ok := parseVersion(v)
	return ok
}

func ValidVersion(v string) bool { return validVersion(v) }
func validConstraint(v string) bool {
	v = strings.TrimSpace(v)
	if v == "*" {
		return true
	}
	for _, p := range strings.Fields(v) {
		op := ""
		for _, prefix := range []string{"<=", ">=", "<", ">", "="} {
			if strings.HasPrefix(p, prefix) {
				op = prefix
				p = strings.TrimPrefix(p, prefix)
				break
			}
		}
		if op == "" || !validVersion(p) {
			return false
		}
	}
	return v != ""
}
func Satisfies(version, constraint string) bool {
	current, ok := parseVersion(version)
	if !ok {
		return false
	}
	if constraint == "*" {
		return true
	}
	for _, p := range strings.Fields(constraint) {
		op := p[:1]
		if strings.HasPrefix(p, ">=") || strings.HasPrefix(p, "<=") {
			op, p = p[:2], p[2:]
		} else {
			p = p[1:]
		}
		wanted, ok := parseVersion(p)
		if !ok {
			return false
		}
		cmp := compare(current, wanted)
		if (op == "=" && cmp != 0) || (op == ">" && cmp <= 0) || (op == ">=" && cmp < 0) || (op == "<" && cmp >= 0) || (op == "<=" && cmp > 0) {
			return false
		}
	}
	return true
}
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		if p == "" {
			return out, false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return out, false
			}
			out[i] = out[i]*10 + int(r-'0')
		}
	}
	return out, true
}
func compare(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil && v == strings.ToLower(v)
}
func optionalID(v string) bool { return v == "" || validID(v) }
func validCommit(v string) bool {
	if len(v) != 40 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil && v == strings.ToLower(v)
}
