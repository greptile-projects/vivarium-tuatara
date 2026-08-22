// Package repositories connects application ownership metadata to durable Git storage.
package repositories

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

var (
	ErrNotFound            = errors.New("repository not found")
	ErrNameTaken           = errors.New("repository name is already in use")
	ErrInvalidName         = errors.New("invalid repository name")
	ErrVisibility          = errors.New("invalid repository visibility")
	ErrInvalidCollaborator = errors.New("invalid repository collaborator")
	ErrInvalidBranch       = errors.New("invalid repository branch")
	ErrForkDiverged        = errors.New("fork branch has diverged from upstream")
	ErrBranchChanged       = errors.New("repository branch changed")
)

const (
	Private = "private"
	Public  = "public"
)

type Repository struct {
	ID                    string    `json:"id"`
	OwnerID               string    `json:"owner_id"`
	OrganizationID        string    `json:"organization_id,omitempty"`
	Name                  string    `json:"name"`
	Visibility            string    `json:"visibility"`
	DefaultBranch         string    `json:"default_branch"`
	GitRemote             string    `json:"git_remote"`
	CreatedAt             time.Time `json:"created_at"`
	UpstreamRepositoryID  string    `json:"upstream_repository_id,omitempty"`
	FederatedUpstream     string    `json:"federated_upstream,omitempty"`
	FederatedBranch       string    `json:"federated_branch,omitempty"`
	collaboratorIDs       string
	organizationMemberIDs string
	requiredChecks        string
	integrationPolicies   string
}

// CreateFederatedFork publishes an independently owned repository from an
// already verified transfer repository and retains only remote lineage.
func (s *Store) CreateFederatedFork(ownerID, reference, branch, name string, source *storage.Repository, revision string) (Repository, error) {
	name, err := validateName(name)
	if err != nil || source == nil || reference == "" || branch == "" || len(revision) != 40 {
		if err != nil {
			return Repository{}, err
		}
		return Repository{}, ErrInvalidBranch
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Repository{}, err
	}
	defer unlock()
	all, err := s.loadActive()
	if err != nil {
		return Repository{}, err
	}
	for _, repository := range all {
		if repository.OwnerID == ownerID && strings.EqualFold(repository.Name, name) {
			return Repository{}, ErrNameTaken
		}
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return Repository{}, err
	}
	id := hex.EncodeToString(idBytes)
	if _, err := source.ReadCommit(storage.ObjectID(revision)); err != nil {
		return Repository{}, ErrInvalidBranch
	}
	created, err := s.git.Create(id)
	if err != nil {
		return Repository{}, err
	}
	if err = created.ImportCommit(source, storage.ObjectID(revision)); err != nil {
		_ = s.git.Delete(id)
		return Repository{}, err
	}
	if err = created.CreateReference(storage.Reference{Name: "refs/heads/" + branch, Target: revision}); err != nil {
		_ = s.git.Delete(id)
		return Repository{}, err
	}
	repository := Repository{ID: id, OwnerID: ownerID, Name: name, Visibility: Private, DefaultBranch: branch, GitRemote: "/git/" + id + ".git", CreatedAt: s.now().Truncate(time.Microsecond), FederatedUpstream: reference, FederatedBranch: branch, requiredChecks: "[]"}
	if err = s.write(repository); err != nil {
		_ = s.git.Delete(id)
		return Repository{}, err
	}
	return repository, nil
}

func (s *Store) SynchronizeFederatedFork(ownerID, id, branch string, source *storage.Repository, revision string) (ForkSynchronization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return ForkSynchronization{}, err
	}
	defer unlock()
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID || repository.FederatedUpstream == "" || repository.FederatedBranch != branch {
		return ForkSynchronization{}, ErrNotFound
	}
	fork, err := s.git.Open(id)
	if err != nil {
		return ForkSynchronization{}, err
	}
	current, err := fork.ReadReference("refs/heads/" + branch)
	if err != nil || current.Symbolic {
		return ForkSynchronization{}, ErrInvalidBranch
	}
	if current.Target == revision {
		return ForkSynchronization{Branch: branch, PreviousCommitID: revision, CommitID: revision}, nil
	}
	if err = fork.ImportCommit(source, storage.ObjectID(revision)); err != nil {
		return ForkSynchronization{}, err
	}
	ancestry, err := fork.ListCommitAncestry(storage.ObjectID(revision))
	if err != nil {
		return ForkSynchronization{}, err
	}
	found := false
	for _, commit := range ancestry {
		if string(commit.ID) == current.Target {
			found = true
			break
		}
	}
	if !found {
		return ForkSynchronization{}, ErrForkDiverged
	}
	if err = fork.UpdateReferenceIfTarget(storage.Reference{Name: "refs/heads/" + branch, Target: revision}, current.Target); err != nil {
		return ForkSynchronization{}, ErrBranchChanged
	}
	return ForkSynchronization{Branch: branch, PreviousCommitID: current.Target, CommitID: revision}, nil
}

// SetOrganization associates an existing repository with an accountable group
// without replacing its catalog or Git identity. The current user custodian is
// retained for compatibility with owner-governed workflows.
func (s *Store) SetOrganization(ownerID, id, organizationID string, memberIDs []string) (Repository, error) {
	if !validID(organizationID) {
		return Repository{}, ErrInvalidName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Repository{}, err
	}
	defer unlock()
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID {
		return Repository{}, ErrNotFound
	}
	if repository.OrganizationID != "" && repository.OrganizationID != organizationID {
		return Repository{}, ErrNameTaken
	}
	repository.OrganizationID = organizationID
	ids := collaboratorIDs(repository)
	projected := organizationCollaboratorIDs(repository)
	for _, memberID := range memberIDs {
		if !validID(memberID) || memberID == ownerID {
			continue
		}
		if slices.Contains(ids, memberID) {
			continue
		}
		ids = append(ids, memberID)
		projected = append(projected, memberID)
	}
	sort.Strings(ids)
	sort.Strings(projected)
	repository.collaboratorIDs = strings.Join(ids, ",")
	repository.organizationMemberIDs = strings.Join(projected, ",")
	if err := s.write(repository); err != nil {
		return Repository{}, err
	}
	return repository, nil
}

// RemoveOrganizationMember revokes only access projected from organization
// membership. A collaborator grant that predated the transfer remains intact.
func (s *Store) RemoveOrganizationMember(ownerID, id, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID {
		return ErrNotFound
	}
	projected := organizationCollaboratorIDs(repository)
	if !slices.Contains(projected, userID) {
		return nil
	}
	projected = slices.DeleteFunc(projected, func(v string) bool { return v == userID })
	ids := slices.DeleteFunc(collaboratorIDs(repository), func(v string) bool { return v == userID })
	repository.organizationMemberIDs, repository.collaboratorIDs = strings.Join(projected, ","), strings.Join(ids, ",")
	return s.write(repository)
}

func (s *Store) ListOrganization(organizationID string) ([]Repository, error) {
	all, err := s.loadActive()
	if err != nil {
		return nil, err
	}
	result := []Repository{}
	for _, repository := range all {
		if repository.OrganizationID == organizationID {
			result = append(result, repository)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

// WithOrganization holds the catalog mutation boundary while verifying that
// an exact repository still belongs to an organization and publishing a
// coordinated record in another store. Callers must acquire their own store
// lock first, matching organization-transfer lock ordering.
func (s *Store) WithOrganization(id, organizationID string, publish func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	repository, err := s.read(id)
	if err != nil || repository.OrganizationID != organizationID {
		return ErrNotFound
	}
	return publish()
}

type BranchCheckRequirements struct {
	Branch string   `json:"branch"`
	Checks []string `json:"checks"`
}

type IntegrationQueuePolicy struct {
	Branch            string   `json:"branch"`
	Enabled           bool     `json:"enabled"`
	Concurrency       int      `json:"concurrency"`
	FailureBehavior   string   `json:"failure_behavior"`
	RequiredChecks    []string `json:"required_checks"`
	RequiredApprovals int      `json:"required_approvals"`
}

const (
	QueueFailurePause  = "pause"
	QueueFailureRemove = "remove"
)

type Collaborator struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

const Contributor = "contributor"

type gitStore interface {
	Create(string) (*storage.Repository, error)
	Open(string) (*storage.Repository, error)
	Delete(string) error
}

type forkGitStore interface {
	Fork(string, string) (*storage.Repository, error)
}

type ForkSynchronization struct {
	Branch               string `json:"branch"`
	PreviousCommitID     string `json:"previous_commit_id,omitempty"`
	CommitID             string `json:"commit_id"`
	UpstreamRepositoryID string `json:"upstream_repository_id"`
}

type Store struct {
	root                           string
	git                            gitStore
	mu                             sync.Mutex
	now                            func() time.Time
	remove                         func(string) error
	rename                         func(string, string) error
	directorySync                  func(string) error
	afterCreateForkAuthorization   func()
	afterSynchronizeAuthorization  func()
	afterContributionAuthorization func()
	afterParticipantAuthorization  func()
	afterReadAuthorization         func()
}

// WithCurrentReadAccess runs fn while holding the catalog mutation lock after
// proving that actorID can still read every named repository. Visibility and
// collaborator changes therefore commit wholly before or after publication of
// evidence derived from those repositories.
func (s *Store) WithCurrentReadAccess(actorID string, repositoryIDs []string, fn func() error) error {
	if (actorID != "" && !validID(actorID)) || len(repositoryIDs) == 0 || fn == nil {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	seen := map[string]bool{}
	for _, id := range repositoryIDs {
		if !validID(id) || seen[id] {
			continue
		}
		seen[id] = true
		repository, readErr := s.read(id)
		if readErr != nil {
			return ErrNotFound
		}
		if _, openErr := s.git.Open(id); openErr != nil {
			return ErrNotFound
		}
		if repository.Visibility != Public && repository.OwnerID != actorID && !slices.Contains(collaboratorIDs(repository), actorID) {
			return ErrNotFound
		}
	}
	if s.afterReadAuthorization != nil {
		s.afterReadAuthorization()
	}
	return fn()
}

// WithCurrentOwners holds repository existence and ownership stable while a
// dependent record that claims owner authority is committed. Repositories have
// one canonical owner, so every claimed owner must resolve to that identity.
func (s *Store) WithCurrentOwners(ownerIDs, repositoryIDs []string, fn func() error) error {
	if len(ownerIDs) == 0 || len(repositoryIDs) == 0 || fn == nil {
		return ErrNotFound
	}
	for _, ownerID := range ownerIDs {
		if !validID(ownerID) {
			return ErrNotFound
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	seen := map[string]bool{}
	for _, id := range repositoryIDs {
		if !validID(id) || seen[id] {
			return ErrNotFound
		}
		seen[id] = true
		repository, readErr := s.read(id)
		if readErr != nil {
			return ErrNotFound
		}
		for _, ownerID := range ownerIDs {
			if repository.OwnerID != ownerID {
				return ErrNotFound
			}
		}
		if _, openErr := s.git.Open(id); openErr != nil {
			return ErrNotFound
		}
	}
	return fn()
}

// WithCurrentParticipant runs fn while holding the catalog mutation lock after
// proving userID is a current owner or contributor. Access removal therefore
// commits wholly before or after the dependent mutation performed by fn.
func (s *Store) WithCurrentParticipant(userID, repositoryID string, fn func() error) error {
	return s.WithCurrentParticipants([]string{userID}, repositoryID, fn)
}

// WithCurrentParticipants holds one catalog boundary while proving every user
// still participates in the repository. It is used when a collaboration
// mutation names both its actor and another authority-bearing participant.
func (s *Store) WithCurrentParticipants(userIDs []string, repositoryID string, fn func() error) error {
	if len(userIDs) == 0 {
		return ErrInvalidCollaborator
	}
	return s.WithCurrentDeliveryAuthority(userIDs, repositoryID, "", fn)
}

// WithCurrentParticipantsAndReadAccess holds one catalog boundary while
// proving provider ownership roles and the publisher's read access to every
// repository whose existence a dependent record would disclose.
func (s *Store) WithCurrentParticipantsAndReadAccess(userIDs []string, repositoryID, readerID string, readableRepositoryIDs []string, fn func() error) error {
	if !validID(repositoryID) || !validID(readerID) || len(userIDs) == 0 || fn == nil {
		return ErrInvalidCollaborator
	}
	for _, id := range append(append([]string{}, userIDs...), readableRepositoryIDs...) {
		if !validID(id) {
			return ErrInvalidCollaborator
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	provider, err := s.read(repositoryID)
	if err != nil {
		return ErrNotFound
	}
	if _, err = s.git.Open(repositoryID); err != nil {
		return ErrNotFound
	}
	for _, userID := range userIDs {
		if provider.OwnerID != userID && !slices.Contains(collaboratorIDs(provider), userID) {
			return ErrInvalidCollaborator
		}
	}
	seen := map[string]bool{}
	for _, id := range readableRepositoryIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		repository, readErr := s.read(id)
		if readErr != nil {
			return ErrNotFound
		}
		if _, openErr := s.git.Open(id); openErr != nil {
			return ErrNotFound
		}
		if repository.Visibility != Public && repository.OwnerID != readerID && !slices.Contains(collaboratorIDs(repository), readerID) {
			return ErrNotFound
		}
	}
	if s.afterParticipantAuthorization != nil {
		s.afterParticipantAuthorization()
	}
	return fn()
}

// WithCurrentDeliveryAuthority additionally freezes an optional organization
// association while a dependent delivery mutation commits.
func (s *Store) WithCurrentDeliveryAuthority(userIDs []string, repositoryID, organizationID string, fn func() error) error {
	if !validID(repositoryID) || fn == nil {
		return ErrInvalidCollaborator
	}
	for _, userID := range userIDs {
		if !validID(userID) {
			return ErrInvalidCollaborator
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	repository, err := s.read(repositoryID)
	if err != nil {
		return ErrNotFound
	}
	if _, err := s.git.Open(repositoryID); err != nil {
		return ErrNotFound
	}
	if organizationID != "" && repository.OrganizationID != organizationID {
		return ErrInvalidCollaborator
	}
	for _, userID := range userIDs {
		if repository.OwnerID != userID && !slices.Contains(collaboratorIDs(repository), userID) {
			return ErrInvalidCollaborator
		}
	}
	if s.afterParticipantAuthorization != nil {
		s.afterParticipantAuthorization()
	}
	return fn()
}

// WithIncidentAuthorization holds the catalog mutation lock while proving the
// actor and every named role holder still participate in at least one affected
// repository. Access revocation therefore commits wholly before or after the
// incident mutation performed by fn.
func (s *Store) WithIncidentAuthorization(actorID string, repositoryIDs, roleUserIDs []string, fn func() error) error {
	return s.withIncidentAuthorization(actorID, repositoryIDs, roleUserIDs, false, fn)
}

// WithIncidentDeclarationAuthorization additionally requires the declarer to
// participate in every affected repository.
func (s *Store) WithIncidentDeclarationAuthorization(actorID string, repositoryIDs, roleUserIDs []string, fn func() error) error {
	return s.withIncidentAuthorization(actorID, repositoryIDs, roleUserIDs, true, fn)
}

func (s *Store) withIncidentAuthorization(actorID string, repositoryIDs, roleUserIDs []string, requireAll bool, fn func() error) error {
	if !validID(actorID) || len(repositoryIDs) == 0 || fn == nil {
		return ErrInvalidCollaborator
	}
	for _, id := range append(append([]string{}, repositoryIDs...), roleUserIDs...) {
		if !validID(id) {
			return ErrInvalidCollaborator
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	repositories := make([]Repository, 0, len(repositoryIDs))
	for _, id := range repositoryIDs {
		repository, readErr := s.read(id)
		if readErr != nil {
			return ErrNotFound
		}
		if _, openErr := s.git.Open(id); openErr != nil {
			return ErrNotFound
		}
		repositories = append(repositories, repository)
	}
	participationCount := func(userID string) int {
		count := 0
		for _, repository := range repositories {
			if repository.OwnerID == userID || slices.Contains(collaboratorIDs(repository), userID) {
				count++
			}
		}
		return count
	}
	actorCount := participationCount(actorID)
	if actorCount == 0 || (requireAll && actorCount != len(repositories)) {
		return ErrInvalidCollaborator
	}
	for _, userID := range roleUserIDs {
		if participationCount(userID) == 0 {
			return ErrInvalidCollaborator
		}
	}
	if s.afterParticipantAuthorization != nil {
		s.afterParticipantAuthorization()
	}
	return fn()
}

// WithContributionAuthorization runs fn while holding the catalog's
// cross-process mutation lock after proving the actor may contribute from the
// named source into the target. Collaborator revocation and visibility changes
// therefore commit wholly before or after source-object import and pull
// revision publication performed by fn.
func (s *Store) WithContributionAuthorization(actorID, targetID, sourceID string, fn func() error) error {
	if !validID(actorID) || !validID(targetID) || !validID(sourceID) || fn == nil {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	target, err := s.read(targetID)
	if err != nil {
		return ErrNotFound
	}
	if _, err := s.git.Open(targetID); err != nil {
		return ErrNotFound
	}
	participant := target.OwnerID == actorID || slices.Contains(collaboratorIDs(target), actorID)
	if target.Visibility != Public && !participant {
		return ErrNotFound
	}
	source, err := s.read(sourceID)
	if err != nil {
		return ErrNotFound
	}
	if _, err := s.git.Open(sourceID); err != nil {
		return ErrNotFound
	}
	if sourceID == targetID {
		if !participant {
			return ErrNotFound
		}
	} else if source.OwnerID != actorID || source.UpstreamRepositoryID != targetID {
		return ErrNotFound
	}
	if s.afterContributionAuthorization != nil {
		s.afterContributionAuthorization()
	}
	return fn()
}

func New(root string, git *storage.Store) (*Store, error) {
	if root == "" || git == nil {
		return nil, errors.New("repository catalog requires metadata and Git storage")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create repository catalog: %w", err)
	}
	return &Store{root: abs, git: git, now: func() time.Time { return time.Now().UTC() }, remove: os.Remove, rename: os.Rename, directorySync: syncDirectory}, nil
}

func (s *Store) Create(ownerID, name string) (Repository, error) {
	name, err := validateName(name)
	if err != nil {
		return Repository{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Repository{}, err
	}
	defer unlock()
	all, err := s.loadActive()
	if err != nil {
		return Repository{}, err
	}
	for _, repository := range all {
		if repository.OwnerID == ownerID && strings.EqualFold(repository.Name, name) {
			return Repository{}, ErrNameTaken
		}
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return Repository{}, err
	}
	id := hex.EncodeToString(idBytes)
	if _, err := s.git.Create(id); err != nil {
		return Repository{}, fmt.Errorf("create Git repository: %w", err)
	}
	repository := Repository{ID: id, OwnerID: ownerID, Name: name, Visibility: Private, DefaultBranch: "main", GitRemote: "/git/" + id + ".git", CreatedAt: s.now().Truncate(time.Microsecond), requiredChecks: "[]", integrationPolicies: "[]"}
	if err := s.write(repository); err != nil {
		if persisted, readErr := s.read(id); readErr == nil && persisted == repository {
			return repository, nil
		}
		if deleteErr := s.git.Delete(id); deleteErr != nil {
			return Repository{}, fmt.Errorf("publish repository metadata: %v; rollback Git repository: %w", err, deleteErr)
		}
		return Repository{}, err
	}
	return repository, nil
}

// CreateFork creates an independently owned repository while retaining the
// immutable catalog identity of the source repository as upstream lineage.
func (s *Store) CreateFork(ownerID, sourceID, name string) (Repository, error) {
	name, err := validateName(name)
	if err != nil {
		return Repository{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Repository{}, err
	}
	defer unlock()
	source, err := s.read(sourceID)
	if err != nil {
		return Repository{}, ErrNotFound
	}
	if source.Visibility != Public && source.OwnerID != ownerID && !slices.Contains(collaboratorIDs(source), ownerID) {
		return Repository{}, ErrNotFound
	}
	if s.afterCreateForkAuthorization != nil {
		s.afterCreateForkAuthorization()
	}
	if _, err := s.git.Open(source.ID); err != nil {
		if errors.Is(err, storage.ErrRepositoryNotFound) {
			return Repository{}, ErrNotFound
		}
		return Repository{}, err
	}
	all, err := s.loadActive()
	if err != nil {
		return Repository{}, err
	}
	for _, repository := range all {
		if repository.OwnerID == ownerID && strings.EqualFold(repository.Name, name) {
			return Repository{}, ErrNameTaken
		}
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return Repository{}, err
	}
	id := hex.EncodeToString(idBytes)
	forker, ok := s.git.(forkGitStore)
	if !ok {
		return Repository{}, errors.New("Git storage does not support forks")
	}
	if _, err := forker.Fork(source.ID, id); err != nil {
		return Repository{}, fmt.Errorf("fork Git repository: %w", err)
	}
	repository := Repository{ID: id, OwnerID: ownerID, Name: name, Visibility: Private, DefaultBranch: source.DefaultBranch, GitRemote: "/git/" + id + ".git", CreatedAt: s.now().Truncate(time.Microsecond), UpstreamRepositoryID: source.ID, requiredChecks: "[]"}
	if err := s.write(repository); err != nil {
		if persisted, readErr := s.read(id); readErr == nil && persisted == repository {
			return repository, nil
		}
		if deleteErr := s.git.Delete(id); deleteErr != nil {
			return Repository{}, fmt.Errorf("publish fork metadata: %v; rollback Git repository: %w", err, deleteErr)
		}
		return Repository{}, err
	}
	return repository, nil
}

// SynchronizeFork fast-forwards one fork branch to the same named branch in
// its recorded upstream repository. Divergent independent work is preserved.
func (s *Store) SynchronizeFork(ownerID, id, branch string) (ForkSynchronization, error) {
	if branch == "" || strings.HasPrefix(branch, "refs/") {
		return ForkSynchronization{}, ErrInvalidBranch
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return ForkSynchronization{}, err
	}
	defer unlock()
	repository, err := s.Get(ownerID, id)
	if err != nil {
		return ForkSynchronization{}, err
	}
	if repository.UpstreamRepositoryID == "" {
		return ForkSynchronization{}, ErrNotFound
	}
	upstream, err := s.GetByID(repository.UpstreamRepositoryID)
	if err != nil {
		return ForkSynchronization{}, err
	}
	if upstream.Visibility != Public && upstream.OwnerID != ownerID && !slices.Contains(collaboratorIDs(upstream), ownerID) {
		return ForkSynchronization{}, ErrNotFound
	}
	if s.afterSynchronizeAuthorization != nil {
		s.afterSynchronizeAuthorization()
	}
	upstreamGit, err := s.git.Open(upstream.ID)
	if err != nil {
		return ForkSynchronization{}, err
	}
	forkGit, err := s.git.Open(repository.ID)
	if err != nil {
		return ForkSynchronization{}, err
	}
	name := "refs/heads/" + branch
	upstreamRef, err := upstreamGit.ReadReference(name)
	if err != nil || upstreamRef.Symbolic {
		if errors.Is(err, storage.ErrInvalidReference) || errors.Is(err, storage.ErrReferenceNotFound) {
			return ForkSynchronization{}, ErrInvalidBranch
		}
		if err != nil {
			return ForkSynchronization{}, err
		}
		return ForkSynchronization{}, ErrInvalidBranch
	}
	if _, err := upstreamGit.ReadCommit(storage.ObjectID(upstreamRef.Target)); err != nil {
		return ForkSynchronization{}, err
	}
	current, readErr := forkGit.ReadReference(name)
	if readErr != nil && !errors.Is(readErr, storage.ErrReferenceNotFound) && !errors.Is(readErr, storage.ErrInvalidReference) {
		return ForkSynchronization{}, readErr
	}
	if errors.Is(readErr, storage.ErrInvalidReference) {
		return ForkSynchronization{}, ErrInvalidBranch
	}
	previous := ""
	if readErr == nil {
		if current.Symbolic {
			return ForkSynchronization{}, ErrInvalidBranch
		}
		previous = current.Target
		if previous == upstreamRef.Target {
			return ForkSynchronization{Branch: branch, PreviousCommitID: previous, CommitID: upstreamRef.Target, UpstreamRepositoryID: upstream.ID}, nil
		}
	}
	if err := forkGit.ImportCommit(upstreamGit, storage.ObjectID(upstreamRef.Target)); err != nil {
		return ForkSynchronization{}, err
	}
	if previous != "" {
		ancestry, err := forkGit.ListCommitAncestry(storage.ObjectID(upstreamRef.Target))
		if err != nil {
			return ForkSynchronization{}, err
		}
		found := false
		for _, commit := range ancestry {
			if string(commit.ID) == previous {
				found = true
				break
			}
		}
		if !found {
			return ForkSynchronization{}, ErrForkDiverged
		}
		if err := forkGit.UpdateReferenceIfTarget(storage.Reference{Name: name, Target: upstreamRef.Target}, previous); err != nil {
			if errors.Is(err, storage.ErrReferenceExists) || errors.Is(err, storage.ErrReferenceLocked) {
				return ForkSynchronization{}, ErrBranchChanged
			}
			return ForkSynchronization{}, err
		}
	} else if err := forkGit.CreateReference(storage.Reference{Name: name, Target: upstreamRef.Target}); err != nil {
		if errors.Is(err, storage.ErrReferenceExists) || errors.Is(err, storage.ErrReferenceLocked) {
			return ForkSynchronization{}, ErrBranchChanged
		}
		return ForkSynchronization{}, err
	}
	return ForkSynchronization{Branch: branch, PreviousCommitID: previous, CommitID: upstreamRef.Target, UpstreamRepositoryID: upstream.ID}, nil
}

// GetByID resolves an active repository without applying an actor policy. It
// is intended for the shared HTTP authorization layer, not direct API use.
func (s *Store) GetByID(id string) (Repository, error) {
	repository, err := s.read(id)
	if err != nil {
		return Repository{}, ErrNotFound
	}
	if _, err := s.git.Open(id); err != nil {
		if errors.Is(err, storage.ErrRepositoryNotFound) {
			return Repository{}, ErrNotFound
		}
		return Repository{}, fmt.Errorf("open Git repository: %w", err)
	}
	return repository, nil
}

// HasCommit verifies an immutable repository object without exposing its contents.
func (s *Store) HasCommit(id, revision string) bool {
	if len(revision) != 40 {
		return false
	}
	repository, err := s.git.Open(id)
	if err != nil {
		return false
	}
	_, err = repository.ReadCommit(storage.ObjectID(revision))
	return err == nil
}

func (s *Store) SetVisibility(ownerID, id, visibility string) (Repository, error) {
	if visibility != Private && visibility != Public {
		return Repository{}, ErrVisibility
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Repository{}, err
	}
	defer unlock()
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID {
		return Repository{}, ErrNotFound
	}
	if _, err := s.git.Open(id); err != nil {
		if errors.Is(err, storage.ErrRepositoryNotFound) {
			return Repository{}, ErrNotFound
		}
		return Repository{}, err
	}
	if repository.Visibility == visibility {
		return repository, nil
	}
	repository.Visibility = visibility
	if err := s.write(repository); err != nil {
		if persisted, readErr := s.read(id); readErr == nil && persisted == repository {
			return repository, nil
		}
		return Repository{}, err
	}
	return repository, nil
}

// SetRequiredChecks replaces the owner-managed quality gate for one target branch.
func (s *Store) SetRequiredChecks(ownerID, id, branch string, checks []string) ([]string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" || strings.HasPrefix(branch, "refs/") || strings.ContainsAny(branch, " ~^:?*[\\\r\n") || strings.HasSuffix(branch, ".") || strings.Contains(branch, "..") {
		return nil, ErrInvalidName
	}
	cleaned, seen := make([]string, 0, len(checks)), map[string]bool{}
	if len(checks) > 20 {
		return nil, ErrInvalidName
	}
	for _, check := range checks {
		check = strings.TrimSpace(check)
		if check == "" || len([]rune(check)) > 100 || seen[check] {
			return nil, ErrInvalidName
		}
		seen[check], cleaned = true, append(cleaned, check)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return nil, err
	}
	defer unlock()
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID {
		return nil, ErrNotFound
	}
	policies := decodeRequirements(repository.requiredChecks)
	updated := make([]BranchCheckRequirements, 0, len(policies)+1)
	for _, policy := range policies {
		if policy.Branch != branch {
			updated = append(updated, policy)
		}
	}
	if len(cleaned) > 0 {
		updated = append(updated, BranchCheckRequirements{Branch: branch, Checks: cleaned})
	}
	sort.Slice(updated, func(i, j int) bool { return updated[i].Branch < updated[j].Branch })
	body, _ := json.Marshal(updated)
	repository.requiredChecks = string(body)
	if err := s.write(repository); err != nil {
		return nil, err
	}
	return append([]string(nil), cleaned...), nil
}

func (s *Store) RequiredChecks(id, branch string) ([]string, error) {
	repository, err := s.read(id)
	if err != nil {
		return nil, ErrNotFound
	}
	for _, policy := range decodeRequirements(repository.requiredChecks) {
		if policy.Branch == branch {
			return append([]string(nil), policy.Checks...), nil
		}
	}
	return []string{}, nil
}

// SetIntegrationQueuePolicy makes ordered admission mandatory for one target
// branch. Review and required-check rules remain the admission criteria.
func (s *Store) SetIntegrationQueuePolicy(ownerID, id, branch string, enabled bool, concurrency int, failureBehavior string) (IntegrationQueuePolicy, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" || strings.HasPrefix(branch, "refs/") || strings.ContainsAny(branch, " ~^:?*[\\\r\n") || strings.Contains(branch, "..") || concurrency < 1 || concurrency > 10 || (failureBehavior != QueueFailurePause && failureBehavior != QueueFailureRemove) {
		return IntegrationQueuePolicy{}, ErrInvalidName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return IntegrationQueuePolicy{}, err
	}
	defer unlock()
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID {
		return IntegrationQueuePolicy{}, ErrNotFound
	}
	policies := decodeIntegrationPolicies(repository.integrationPolicies)
	updated := make([]IntegrationQueuePolicy, 0, len(policies)+1)
	for _, policy := range policies {
		if policy.Branch != branch {
			updated = append(updated, policy)
		}
	}
	if enabled {
		updated = append(updated, IntegrationQueuePolicy{Branch: branch, Enabled: true, Concurrency: concurrency, FailureBehavior: failureBehavior})
	}
	sort.Slice(updated, func(i, j int) bool { return updated[i].Branch < updated[j].Branch })
	body, _ := json.Marshal(updated)
	repository.integrationPolicies = string(body)
	if err := s.write(repository); err != nil {
		return IntegrationQueuePolicy{}, err
	}
	return s.IntegrationQueuePolicy(id, branch)
}

func (s *Store) IntegrationQueuePolicy(id, branch string) (IntegrationQueuePolicy, error) {
	repository, err := s.read(id)
	if err != nil {
		return IntegrationQueuePolicy{}, ErrNotFound
	}
	checks, err := s.RequiredChecks(id, branch)
	if err != nil {
		return IntegrationQueuePolicy{}, err
	}
	policy := IntegrationQueuePolicy{Branch: branch, Concurrency: 1, FailureBehavior: QueueFailurePause, RequiredChecks: checks, RequiredApprovals: 1}
	for _, candidate := range decodeIntegrationPolicies(repository.integrationPolicies) {
		if candidate.Branch == branch {
			policy.Enabled, policy.Concurrency, policy.FailureBehavior = true, candidate.Concurrency, candidate.FailureBehavior
			break
		}
	}
	return policy, nil
}

func decodeIntegrationPolicies(encoded string) []IntegrationQueuePolicy {
	if encoded == "" {
		return []IntegrationQueuePolicy{}
	}
	var policies []IntegrationQueuePolicy
	if json.Unmarshal([]byte(encoded), &policies) != nil {
		return []IntegrationQueuePolicy{}
	}
	if policies == nil {
		return []IntegrationQueuePolicy{}
	}
	return policies
}

// LockRequiredChecks prevents a branch quality policy from changing while a
// merge revalidates its evidence and advances the target reference.
func (s *Store) LockRequiredChecks() (func(), error) { return s.lockRoot() }

func decodeRequirements(encoded string) []BranchCheckRequirements {
	if encoded == "" {
		return []BranchCheckRequirements{}
	}
	var policies []BranchCheckRequirements
	if json.Unmarshal([]byte(encoded), &policies) != nil {
		return []BranchCheckRequirements{}
	}
	return policies
}

func (s *Store) AddCollaborator(ownerID, id, userID string) (Collaborator, error) {
	if !validID(userID) {
		return Collaborator{}, ErrInvalidCollaborator
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Collaborator{}, err
	}
	defer unlock()
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID {
		return Collaborator{}, ErrNotFound
	}
	if userID == ownerID {
		return Collaborator{}, ErrInvalidCollaborator
	}
	ids := collaboratorIDs(repository)
	for _, existing := range ids {
		if existing == userID {
			projected := organizationCollaboratorIDs(repository)
			if slices.Contains(projected, userID) {
				repository.organizationMemberIDs = strings.Join(slices.DeleteFunc(projected, func(v string) bool { return v == userID }), ",")
				if err := s.write(repository); err != nil {
					return Collaborator{}, err
				}
			}
			return Collaborator{UserID: userID, Role: Contributor}, nil
		}
	}
	ids = append(ids, userID)
	sort.Strings(ids)
	repository.collaboratorIDs = strings.Join(ids, ",")
	if err := s.write(repository); err != nil {
		if persisted, readErr := s.read(id); readErr == nil && persisted == repository {
			return Collaborator{UserID: userID, Role: Contributor}, nil
		}
		return Collaborator{}, err
	}
	return Collaborator{UserID: userID, Role: Contributor}, nil
}

func (s *Store) ListCollaborators(ownerID, id string) ([]Collaborator, error) {
	repository, err := s.Get(ownerID, id)
	if err != nil {
		return nil, err
	}
	ids := collaboratorIDs(repository)
	result := make([]Collaborator, len(ids))
	for i, userID := range ids {
		result[i] = Collaborator{UserID: userID, Role: Contributor}
	}
	return result, nil
}

func (s *Store) RemoveCollaborator(ownerID, id, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID {
		return ErrNotFound
	}
	ids := collaboratorIDs(repository)
	for i, existing := range ids {
		if existing == userID {
			ids = append(ids[:i], ids[i+1:]...)
			repository.collaboratorIDs = strings.Join(ids, ",")
			if err := s.write(repository); err != nil {
				// A directory-sync failure after rename leaves publication
				// uncertain. Reconcile the exact requested state so DELETE does
				// not report failure after access was visibly revoked.
				if persisted, readErr := s.read(id); readErr == nil && persisted == repository {
					return nil
				}
				return err
			}
			return nil
		}
	}
	return nil
}

func (s *Store) HasCollaborator(userID, id string) (bool, error) {
	repository, err := s.GetByID(id)
	if err != nil {
		return false, err
	}
	for _, collaboratorID := range collaboratorIDs(repository) {
		if collaboratorID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) Get(ownerID, id string) (Repository, error) {
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID {
		return Repository{}, ErrNotFound
	}
	if _, err := s.git.Open(id); err != nil {
		if errors.Is(err, storage.ErrRepositoryNotFound) {
			return Repository{}, ErrNotFound
		}
		return Repository{}, fmt.Errorf("open Git repository: %w", err)
	}
	return repository, nil
}

func (s *Store) List(ownerID string) ([]Repository, error) {
	all, err := s.loadActive()
	if err != nil {
		return nil, err
	}
	result := []Repository{}
	for _, repository := range all {
		if repository.OwnerID == ownerID {
			result = append(result, repository)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

// ListAccessible returns repositories the user owns or currently contributes
// to, ordered as one stable collection for cursor pagination.
func (s *Store) ListAccessible(userID string) ([]Repository, error) {
	all, err := s.loadActive()
	if err != nil {
		return nil, err
	}
	result := []Repository{}
	for _, repository := range all {
		if repository.OwnerID == userID || slices.Contains(collaboratorIDs(repository), userID) {
			result = append(result, repository)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (s *Store) Delete(ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID {
		return ErrNotFound
	}
	if err := s.git.Delete(id); err != nil {
		// Git storage retains a stable tombstone after post-detach cleanup
		// failures. Preserve ownership metadata so an authenticated retry can
		// invoke Delete again and finish that cleanup.
		return fmt.Errorf("delete Git repository: %w", err)
	}
	if err := s.remove(s.path(id)); err != nil {
		return fmt.Errorf("delete repository metadata: %w", err)
	}
	return s.directorySync(s.root)
}

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 || name == "." || name == ".." || strings.ContainsAny(name, "\x00/\\\r\n") {
		return "", ErrInvalidName
	}
	return name, nil
}

func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }

func (s *Store) read(id string) (Repository, error) {
	if !validID(id) {
		return Repository{}, ErrNotFound
	}
	data, err := os.ReadFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return Repository{}, ErrNotFound
	}
	if err != nil {
		return Repository{}, err
	}
	var repository Repository
	var record struct {
		ID                    string                    `json:"id"`
		OwnerID               string                    `json:"owner_id"`
		OrganizationID        string                    `json:"organization_id,omitempty"`
		Name                  string                    `json:"name"`
		Visibility            string                    `json:"visibility"`
		DefaultBranch         string                    `json:"default_branch"`
		GitRemote             string                    `json:"git_remote"`
		CreatedAt             time.Time                 `json:"created_at"`
		UpstreamRepositoryID  string                    `json:"upstream_repository_id,omitempty"`
		CollaboratorIDs       []string                  `json:"collaborator_ids,omitempty"`
		OrganizationMemberIDs []string                  `json:"organization_member_ids,omitempty"`
		RequiredChecks        []BranchCheckRequirements `json:"required_checks,omitempty"`
		IntegrationQueues     []IntegrationQueuePolicy  `json:"integration_queues,omitempty"`
	}
	if json.Unmarshal(data, &record) != nil {
		return Repository{}, fmt.Errorf("corrupt repository metadata %s", id)
	}
	if record.RequiredChecks == nil {
		record.RequiredChecks = []BranchCheckRequirements{}
	}
	if record.IntegrationQueues == nil {
		record.IntegrationQueues = []IntegrationQueuePolicy{}
	}
	seenBranches := map[string]bool{}
	for _, policy := range record.RequiredChecks {
		if seenBranches[policy.Branch] || !validRequiredPolicy(policy) {
			return Repository{}, fmt.Errorf("corrupt repository metadata %s", id)
		}
		seenBranches[policy.Branch] = true
	}
	requirements, _ := json.Marshal(record.RequiredChecks)
	seenQueues := map[string]bool{}
	for _, policy := range record.IntegrationQueues {
		if seenQueues[policy.Branch] || policy.Branch == "" || !policy.Enabled || policy.Concurrency < 1 || policy.Concurrency > 10 || (policy.FailureBehavior != QueueFailurePause && policy.FailureBehavior != QueueFailureRemove) {
			return Repository{}, fmt.Errorf("corrupt repository metadata %s", id)
		}
		seenQueues[policy.Branch] = true
	}
	integrationPolicies, _ := json.Marshal(record.IntegrationQueues)
	repository = Repository{ID: record.ID, OwnerID: record.OwnerID, OrganizationID: record.OrganizationID, Name: record.Name, Visibility: record.Visibility, DefaultBranch: record.DefaultBranch, GitRemote: record.GitRemote, CreatedAt: record.CreatedAt, UpstreamRepositoryID: record.UpstreamRepositoryID, collaboratorIDs: strings.Join(record.CollaboratorIDs, ","), organizationMemberIDs: strings.Join(record.OrganizationMemberIDs, ","), requiredChecks: string(requirements), integrationPolicies: string(integrationPolicies)}
	if repository.ID != id || !validID(repository.OwnerID) || repository.GitRemote != "/git/"+id+".git" || repository.DefaultBranch != "main" {
		return Repository{}, fmt.Errorf("corrupt repository metadata %s", id)
	}
	if repository.UpstreamRepositoryID != "" && (!validID(repository.UpstreamRepositoryID) || repository.UpstreamRepositoryID == repository.ID) {
		return Repository{}, fmt.Errorf("corrupt repository metadata %s", id)
	}
	if repository.OrganizationID != "" && !validID(repository.OrganizationID) {
		return Repository{}, fmt.Errorf("corrupt repository metadata %s", id)
	}
	seen := map[string]bool{}
	for _, collaboratorID := range collaboratorIDs(repository) {
		if !validID(collaboratorID) || collaboratorID == repository.OwnerID || seen[collaboratorID] {
			return Repository{}, fmt.Errorf("corrupt repository metadata %s", id)
		}
		seen[collaboratorID] = true
	}
	for _, memberID := range organizationCollaboratorIDs(repository) {
		if !seen[memberID] {
			return Repository{}, fmt.Errorf("corrupt repository metadata %s", id)
		}
	}
	// Records created before visibility existed are private by default.
	if repository.Visibility == "" {
		repository.Visibility = Private
	}
	if repository.Visibility != Private && repository.Visibility != Public {
		return Repository{}, fmt.Errorf("corrupt repository metadata %s", id)
	}
	return repository, nil
}

func (s *Store) loadAll() ([]Repository, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	result := []Repository{}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok || !validID(id) {
			continue
		}
		repository, err := s.read(id)
		if err != nil {
			return nil, err
		}
		result = append(result, repository)
	}
	return result, nil
}

// loadActive reconciles the catalog with the Git lifecycle boundary. A Git
// repository is atomically detached before its metadata is removed, so a
// retained record after an interrupted cleanup represents a completed delete,
// not an active remote. The record remains available to a later Delete retry.
func (s *Store) loadActive() ([]Repository, error) {
	all, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	active := make([]Repository, 0, len(all))
	for _, repository := range all {
		if _, err := s.git.Open(repository.ID); err != nil {
			if errors.Is(err, storage.ErrRepositoryNotFound) {
				continue
			}
			return nil, fmt.Errorf("open Git repository %s: %w", repository.ID, err)
		}
		active = append(active, repository)
	}
	return active, nil
}

func (s *Store) write(repository Repository) error {
	record := struct {
		ID                    string                    `json:"id"`
		OwnerID               string                    `json:"owner_id"`
		OrganizationID        string                    `json:"organization_id,omitempty"`
		Name                  string                    `json:"name"`
		Visibility            string                    `json:"visibility"`
		DefaultBranch         string                    `json:"default_branch"`
		GitRemote             string                    `json:"git_remote"`
		CreatedAt             time.Time                 `json:"created_at"`
		UpstreamRepositoryID  string                    `json:"upstream_repository_id,omitempty"`
		CollaboratorIDs       []string                  `json:"collaborator_ids,omitempty"`
		OrganizationMemberIDs []string                  `json:"organization_member_ids,omitempty"`
		RequiredChecks        []BranchCheckRequirements `json:"required_checks,omitempty"`
		IntegrationQueues     []IntegrationQueuePolicy  `json:"integration_queues,omitempty"`
	}{repository.ID, repository.OwnerID, repository.OrganizationID, repository.Name, repository.Visibility, repository.DefaultBranch, repository.GitRemote, repository.CreatedAt, repository.UpstreamRepositoryID, collaboratorIDs(repository), organizationCollaboratorIDs(repository), decodeRequirements(repository.requiredChecks), decodeIntegrationPolicies(repository.integrationPolicies)}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.root, ".writing-")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err == nil {
		_, err = temp.Write(append(data, '\n'))
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := s.rename(tempPath, s.path(repository.ID)); err != nil {
		return err
	}
	return syncDirectory(s.root)
}

func collaboratorIDs(repository Repository) []string {
	if repository.collaboratorIDs == "" {
		return nil
	}
	return strings.Split(repository.collaboratorIDs, ",")
}

func organizationCollaboratorIDs(repository Repository) []string {
	if repository.organizationMemberIDs == "" {
		return []string{}
	}
	return strings.Split(repository.organizationMemberIDs, ",")
}

func (s *Store) lockRoot() (func(), error) {
	file, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}

func validRequiredPolicy(policy BranchCheckRequirements) bool {
	if policy.Branch == "" || strings.HasPrefix(policy.Branch, "refs/") || strings.ContainsAny(policy.Branch, " ~^:?*[\\\r\n") || strings.HasSuffix(policy.Branch, ".") || strings.Contains(policy.Branch, "..") || len(policy.Checks) == 0 || len(policy.Checks) > 20 {
		return false
	}
	seen := map[string]bool{}
	for _, check := range policy.Checks {
		if check == "" || check != strings.TrimSpace(check) || len([]rune(check)) > 100 || seen[check] {
			return false
		}
		seen[check] = true
	}
	return true
}

func validID(id string) bool {
	if len(id) != 32 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
