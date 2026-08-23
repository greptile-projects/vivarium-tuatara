package workspaces

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSuspendResumePreservesFrozenFoundation(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(Workspace{RepositoryID: "0123456789abcdef0123456789abcdef", CommitID: "0123456789012345678901234567890123456789", CreatorID: "abcdef0123456789abcdef0123456789", Source: Source{Kind: "repository"}, Definition: Definition{Version: 1, Image: "alpine:3.22"}}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	running, err := store.Complete(created.ID, []SetupStep{}, false)
	if err != nil || running.State != "running" {
		t.Fatalf("complete = %#v, %v", running, err)
	}
	collaborator := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	suspended, err := store.Transition(created.ID, collaborator, created.DefinitionSHA256, "suspended")
	if err != nil || suspended.CommitID != created.CommitID {
		t.Fatalf("suspend = %#v, %v", suspended, err)
	}
	if actor := suspended.Events[len(suspended.Events)-1].ActorID; actor != collaborator {
		t.Fatalf("suspend actor = %q, want collaborator", actor)
	}
	if _, err = store.Transition(created.ID, created.CreatorID, "different", "running"); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed foundation error = %v", err)
	}
	if _, err = store.Transition(created.ID, created.CreatorID, "", "running"); !errors.Is(err, ErrConflict) {
		t.Fatalf("missing foundation error = %v", err)
	}
	resumed, err := store.Transition(created.ID, created.CreatorID, created.DefinitionSHA256, "running")
	if err != nil || resumed.DefinitionSHA256 != created.DefinitionSHA256 || resumed.CommitID != created.CommitID {
		t.Fatalf("resume = %#v, %v", resumed, err)
	}
}

func TestWorkspaceCommandAndChangeEvidenceIsBoundedAndContentFree(t *testing.T) {
	store, _ := New(t.TempDir())
	created, err := store.Create(Workspace{RepositoryID: "repository", CommitID: "commit", CreatorID: "actor"}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for index := 0; index < 105; index++ {
		if _, err = store.RecordCommand(created.ID, CommandOutcome{CommandSHA256: strings.Repeat("a", 64), ActorID: "actor", StartedAt: now, CompletedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	secret := []byte("not-retained-source-secret")
	digest := sha256.Sum256(secret)
	updated, err := store.RecordChange(created.ID, Change{Path: "config.txt", SHA256: hex.EncodeToString(digest[:]), Size: len(secret), ActorID: "actor", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Commands) != 100 || len(updated.Changes) != 1 || updated.Changes[0].SHA256 == "" {
		t.Fatalf("unexpected evidence: %#v", updated)
	}
	body, err := os.ReadFile(filepath.Join(store.root, created.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), string(secret)) {
		t.Fatal("changed file content leaked into durable workspace evidence")
	}
}

func TestSharedPresenceDiscussionAndVersionedControlSurviveRestart(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	creator := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	peer := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	created, err := store.Create(Workspace{RepositoryID: "repository", CommitID: "commit", CreatorID: creator}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	joined, err := store.Join(created.ID, peer, "file", "main.go")
	if err != nil || len(joined.Presence) != 1 || joined.Presence[0].Path != "main.go" {
		t.Fatalf("join = %#v, %v", joined, err)
	}
	controlled, err := store.SetControl(created.ID, peer, "human", peer, "edit", []string{"files"}, 1, 300)
	if err != nil || !controlled.CanControl(peer, "files", time.Now()) || controlled.CanControl(creator, "files", time.Now()) {
		t.Fatalf("control = %#v, %v", controlled.Control, err)
	}
	if _, err = store.SetControl(created.ID, creator, "human", creator, "execute", []string{"commands"}, 1, 300); !errors.Is(err, ErrControl) {
		t.Fatalf("stale control = %v", err)
	}
	if _, err = store.AddMessage(created.ID, creator, "Please keep the test focused."); err != nil {
		t.Fatal(err)
	}
	reopened, _ := New(root)
	durable, err := reopened.Get(created.ID)
	if err != nil || len(durable.Messages) != 1 || len(durable.Presence) != 1 || durable.Control.Version != 2 {
		t.Fatalf("durable = %#v, %v", durable, err)
	}
	roles := map[string]bool{}
	for _, event := range durable.Events {
		roles[event.Role] = true
	}
	if !roles["observation"] || !roles["instruction"] {
		t.Fatalf("activity roles = %#v", roles)
	}
}

func TestConflictParticipantsRequireCreatorInvitationAndExplicitConsent(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	creator, affected, agent := "11111111111111111111111111111111", "22222222222222222222222222222222", "33333333333333333333333333333333"
	created, err := store.Create(Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: creator, ConflictContext: &ConflictContext{PullRequestID: "pull", BaseCommitID: "base"}}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Invite(created.ID, affected, "human", affected, "owner"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-creator invite = %v", err)
	}
	created, err = store.Invite(created.ID, creator, "human", affected, "source owner")
	if err != nil || created.HasParticipant(affected) {
		t.Fatalf("pending participant = %#v, %v", created.Participants, err)
	}
	created, err = store.RespondInvitation(created.ID, affected, "accepted")
	if err != nil || !created.HasParticipant(affected) {
		t.Fatalf("accepted participant = %#v, %v", created.Participants, err)
	}
	created, err = store.Invite(created.ID, creator, "approved_agent", agent, "bounded resolver")
	if err != nil || created.Participants[1].Status != "accepted" {
		t.Fatalf("agent invitation = %#v, %v", created.Participants, err)
	}
	reopened, err := New(store.root)
	if err != nil {
		t.Fatal(err)
	}
	durable, err := reopened.Get(created.ID)
	if err != nil || !durable.HasParticipant(affected) || len(durable.Participants) != 2 {
		t.Fatalf("durable participants = %#v, %v", durable.Participants, err)
	}
}

func TestConflictLaunchClaimReconcilesOneDurableWorkspace(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, reused, release, err := store.ClaimConflictLaunch("repo", "pull", "stable-launch")
	if err != nil || reused {
		t.Fatalf("initial claim reused=%v err=%v", reused, err)
	}
	created, err := store.Create(Workspace{RepositoryID: "repo", CommitID: "target", CreatorID: "creator", Source: Source{PullRequestID: "pull", ConflictLaunchID: "stable-launch"}, ConflictContext: &ConflictContext{PullRequestID: "pull", CandidateID: "candidate"}}, []byte("definition"))
	release()
	if err != nil {
		t.Fatal(err)
	}
	reconciled, reused, release, err := store.ClaimConflictLaunch("repo", "pull", "stable-launch")
	release()
	if err != nil || !reused || reconciled.ID != created.ID {
		t.Fatalf("reconciled=%#v reused=%v err=%v", reconciled, reused, err)
	}
}

func TestConflictMeaningLedgerRetainsCASAuthorshipAndUndo(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(Workspace{RepositoryID: "repo", CommitID: "target", CreatorID: "operator", ConflictContext: &ConflictContext{PullRequestID: "pull", BaseCommitID: "base", Files: []ConflictFileEvidence{{Path: "service.go"}}}}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	citation := ConflictCitation{Side: "source", Revision: "source", Path: "service.go"}
	created, err = store.AddConflictQuestion(created.ID, 1, ConflictQuestion{Body: "Which retry remains intentional?", Uncertainty: "The migration note is incomplete.", Citations: []ConflictCitation{citation}, Authorship: ConflictAuthorship{ActorID: "operator", AgentID: "agent"}})
	if err != nil || created.ConflictContext.Version != 2 || created.ConflictContext.Questions[0].Authorship.AgentID != "agent" {
		t.Fatalf("question = %#v, %v", created.ConflictContext, err)
	}
	if _, err = store.AnswerConflictQuestion(created.ID, created.ConflictContext.Questions[0].ID, 1, ConflictAnswer{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale answer = %v", err)
	}
	created, err = store.AnswerConflictQuestion(created.ID, created.ConflictContext.Questions[0].ID, 2, ConflictAnswer{Body: "Both retries remain.", Uncertainty: "Load behavior needs a check.", Citations: []ConflictCitation{citation}, Authorship: ConflictAuthorship{ActorID: "owner"}})
	if err != nil {
		t.Fatal(err)
	}
	runtimeContent := "before"
	created, err = store.AddConflictResolution(created.ID, 3, ConflictResolution{Path: "service.go", Summary: "Retain both retries", ProposedContent: "resolved", ExpectedSHA256: digestContent(runtimeContent), Preservation: []ConflictPreservation{{Kind: "user_behavior", Reference: "retry", Disposition: "preserved", Citations: []ConflictCitation{citation}}}, Authorship: ConflictAuthorship{ActorID: "operator", AgentID: "agent"}})
	if err != nil {
		t.Fatal(err)
	}
	created, err = store.Complete(created.ID, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	resolution := created.ConflictContext.Resolutions[0]
	inspect := func(Workspace, ConflictResolution) (string, string, error) {
		return runtimeContent, digestContent(runtimeContent), nil
	}
	mutate := func(_ Workspace, _ ConflictResolution, content, _ string) error {
		runtimeContent = content
		return os.Chmod(filepath.Join(store.root, "provenance"), 0500)
	}
	if _, err = store.ActConflictResolution(created.ID, resolution.ID, 4, true, "operator", ConflictAuthorship{ActorID: "operator", AgentID: "agent"}, inspect, mutate); err == nil {
		t.Fatal("apply unexpectedly finalized without durable provenance")
	}
	if chmodErr := os.Chmod(filepath.Join(store.root, "provenance"), 0700); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	pending, _ := store.Get(created.ID)
	if pending.ConflictContext.Resolutions[0].State != "applying" || runtimeContent != "resolved" {
		t.Fatalf("recoverable apply = %s / %q", pending.ConflictContext.Resolutions[0].State, runtimeContent)
	}
	created, err = store.ActConflictResolution(created.ID, resolution.ID, 4, true, "operator", ConflictAuthorship{ActorID: "different retry"}, inspect, func(Workspace, ConflictResolution, string, string) error {
		t.Fatal("reconciled apply repeated runtime edit")
		return nil
	})
	if err != nil || created.ConflictContext.Resolutions[0].State != "applied" || created.ConflictContext.Resolutions[0].PreviousContent != "before" {
		t.Fatalf("apply = %#v, %v", created.ConflictContext.Resolutions[0], err)
	}
	mutate = func(_ Workspace, _ ConflictResolution, content, _ string) error {
		runtimeContent = content
		return os.Chmod(store.root, 0500)
	}
	_, err = store.ActConflictResolution(created.ID, resolution.ID, 6, false, "operator", ConflictAuthorship{ActorID: "operator"}, inspect, mutate)
	if err == nil {
		t.Fatal("undo finalization unexpectedly survived unwritable workspace store")
	}
	if chmodErr := os.Chmod(store.root, 0700); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	pending, _ = store.Get(created.ID)
	if pending.ConflictContext.Resolutions[0].State != "undoing" || runtimeContent != "before" {
		t.Fatalf("recoverable undo = %s / %q", pending.ConflictContext.Resolutions[0].State, runtimeContent)
	}
	created, err = store.ActConflictResolution(created.ID, resolution.ID, 6, false, "operator", ConflictAuthorship{ActorID: "different retry"}, inspect, func(Workspace, ConflictResolution, string, string) error {
		t.Fatal("reconciled undo repeated runtime edit")
		return nil
	})
	if err != nil || created.ConflictContext.Resolutions[0].State != "undone" {
		t.Fatalf("undo = %#v, %v", created.ConflictContext.Resolutions[0], err)
	}
	if len(created.Changes) != 2 || created.Changes[0].SHA256 != digestContent("resolved") || created.Changes[1].SHA256 != digestContent("before") {
		t.Fatalf("resolution provenance = %#v", created.Changes)
	}
}

func TestConflictResolutionDoesNotFinalizeAfterLifecycleRevokesControl(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(Workspace{RepositoryID: "repo", CommitID: "target", CreatorID: "operator", ConflictContext: &ConflictContext{PullRequestID: "pull", Files: []ConflictFileEvidence{{Path: "conflict.txt"}}}}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	created, err = store.Complete(created.ID, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	runtimeContent := "before"
	created, err = store.AddConflictResolution(created.ID, 1, ConflictResolution{Path: "conflict.txt", Summary: "resolve", ProposedContent: "resolved", ExpectedSHA256: digestContent(runtimeContent), Authorship: ConflictAuthorship{ActorID: "operator"}})
	if err != nil {
		t.Fatal(err)
	}
	resolution := created.ConflictContext.Resolutions[0]
	inspect := func(Workspace, ConflictResolution) (string, string, error) {
		return runtimeContent, digestContent(runtimeContent), nil
	}
	mutate := func(_ Workspace, _ ConflictResolution, content, _ string) error {
		runtimeContent = content
		_, stopErr := store.Stop(created.ID, "owner", "revoked during edit", "stopped")
		return stopErr
	}
	if _, err = store.ActConflictResolution(created.ID, resolution.ID, 2, true, "operator", ConflictAuthorship{ActorID: "operator"}, inspect, mutate); !errors.Is(err, ErrControl) {
		t.Fatalf("revoked finalization = %v", err)
	}
	durable, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if durable.State != "stopped" || durable.ConflictContext.Resolutions[0].State != "applying" || durable.ConflictContext.Resolutions[0].PendingAction.PrincipalID != "operator" || len(durable.Changes) != 0 || runtimeContent != "resolved" {
		t.Fatalf("revoked action finalized: state=%s resolution=%s changes=%d runtime=%q", durable.State, durable.ConflictContext.Resolutions[0].State, len(durable.Changes), runtimeContent)
	}
	provenance, err := store.readProvenance(created.ID)
	if err != nil || len(provenance.Changes) != 0 {
		t.Fatalf("revoked provenance = %#v, %v", provenance.Changes, err)
	}
}

func TestPendingConflictResolutionRejectsFreshLeaseForSamePrincipal(t *testing.T) {
	for _, action := range []string{"apply", "undo"} {
		t.Run(action, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			created, err := store.Create(Workspace{RepositoryID: "repo", CommitID: "target", CreatorID: "operator", ConflictContext: &ConflictContext{PullRequestID: "pull", Files: []ConflictFileEvidence{{Path: "conflict.txt"}}}}, []byte("definition"))
			if err != nil {
				t.Fatal(err)
			}
			created, err = store.Complete(created.ID, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			runtimeContent := "before"
			created, err = store.AddConflictResolution(created.ID, 1, ConflictResolution{Path: "conflict.txt", Summary: "resolve", ProposedContent: "resolved", ExpectedSHA256: digestContent(runtimeContent), Authorship: ConflictAuthorship{ActorID: "operator"}})
			if err != nil {
				t.Fatal(err)
			}
			resolution := created.ConflictContext.Resolutions[0]
			inspect := func(Workspace, ConflictResolution) (string, string, error) {
				return runtimeContent, digestContent(runtimeContent), nil
			}
			mutate := func(_ Workspace, _ ConflictResolution, content, _ string) error { runtimeContent = content; return nil }
			expected, applying := 2, true
			if action == "undo" {
				created, err = store.ActConflictResolution(created.ID, resolution.ID, expected, true, "operator", ConflictAuthorship{ActorID: "operator"}, inspect, mutate)
				if err != nil {
					t.Fatal(err)
				}
				expected, applying = created.ConflictContext.Version, false
			}
			interrupted := func(_ Workspace, _ ConflictResolution, content, _ string) error {
				runtimeContent = content
				return errors.New("executor disconnected after edit")
			}
			if _, err = store.ActConflictResolution(created.ID, resolution.ID, expected, applying, "operator", ConflictAuthorship{ActorID: "operator"}, inspect, interrupted); err == nil {
				t.Fatal("interrupted action finalized")
			}
			pending, err := store.Get(created.ID)
			if err != nil {
				t.Fatal(err)
			}
			originalControlVersion := pending.ConflictContext.Resolutions[0].PendingAction.ControlVersion
			released, err := store.ReleaseControl(created.ID, "operator", pending.Control.Version)
			if err != nil {
				t.Fatal(err)
			}
			regained, err := store.SetControl(created.ID, "operator", "human", "operator", "edit", []string{"files"}, released.Control.Version, 300)
			if err != nil {
				t.Fatal(err)
			}
			if regained.Control.Version == originalControlVersion {
				t.Fatal("test did not replace the lease")
			}
			beforeChanges := len(regained.Changes)
			if _, err = store.ActConflictResolution(created.ID, resolution.ID, expected, applying, "operator", ConflictAuthorship{ActorID: "operator"}, inspect, func(Workspace, ConflictResolution, string, string) error {
				t.Fatal("stale pending action repeated runtime edit")
				return nil
			}); !errors.Is(err, ErrControl) {
				t.Fatalf("fresh same-principal lease finalized %s: %v", action, err)
			}
			durable, err := store.Get(created.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantState := map[bool]string{true: "applying", false: "undoing"}[applying]
			if durable.ConflictContext.Resolutions[0].State != wantState || len(durable.Changes) != beforeChanges {
				t.Fatalf("stale lease changed ledger: state=%s changes=%d", durable.ConflictContext.Resolutions[0].State, len(durable.Changes))
			}
		})
	}
}

func TestControlTransferWaitsForAdmittedMutationAndRejectsStaleActor(t *testing.T) {
	store, _ := New(t.TempDir())
	creator := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	peer := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	created, err := store.Create(Workspace{RepositoryID: "repository", CommitID: "commit", CreatorID: creator}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	started, finish := make(chan struct{}), make(chan struct{})
	mutationDone := make(chan error, 1)
	go func() {
		mutationDone <- store.WithControl(created.ID, creator, "commands", func(Workspace) error {
			close(started)
			<-finish
			return nil
		})
	}()
	<-started
	transferDone := make(chan error, 1)
	go func() {
		_, transferErr := store.SetControl(created.ID, peer, "human", peer, "execute", []string{"commands"}, 1, 300)
		transferDone <- transferErr
	}()
	select {
	case err := <-transferDone:
		t.Fatalf("transfer completed during admitted mutation: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(finish)
	if err := <-mutationDone; err != nil {
		t.Fatal(err)
	}
	if err := <-transferDone; err != nil {
		t.Fatal(err)
	}
	if err := store.WithControl(created.ID, creator, "commands", func(Workspace) error { return nil }); !errors.Is(err, ErrControl) {
		t.Fatalf("former controller mutation = %v", err)
	}
}

func TestControlCanBeExplicitlyReleased(t *testing.T) {
	store, _ := New(t.TempDir())
	holder := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	other := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	created, _ := store.Create(Workspace{RepositoryID: "repository", CommitID: "commit", CreatorID: holder}, []byte("definition"))
	if _, err := store.ReleaseControl(created.ID, other, 1); !errors.Is(err, ErrControl) {
		t.Fatalf("non-holder release = %v", err)
	}
	retained, err := store.Get(created.ID)
	if err != nil || retained.Control.PrincipalID != holder || retained.Control.Version != 1 {
		t.Fatalf("control after denied release = %#v, %v", retained.Control, err)
	}
	released, err := store.ReleaseControl(created.ID, holder, 1)
	if err != nil || released.Control.PrincipalID != "" || len(released.Control.Scopes) != 0 || released.Control.Version != 2 {
		t.Fatalf("release = %#v, %v", released.Control, err)
	}
	if _, err := store.SetControl(created.ID, holder, "", "", "observe", nil, 2, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty transfer = %v", err)
	}
	expiredStore, _ := New(t.TempDir())
	expired, _ := expiredStore.Create(Workspace{RepositoryID: "repository", CommitID: "commit", CreatorID: holder}, []byte("definition"))
	expiredStore.now = func() time.Time { return expired.Control.ExpiresAt.Add(time.Second) }
	if _, err := expiredStore.ReleaseControl(expired.ID, holder, 1); !errors.Is(err, ErrControl) {
		t.Fatalf("expired holder release = %v", err)
	}
}

func TestApprovedAgentUsesScopedControlWithOperatorAttribution(t *testing.T) {
	store, _ := New(t.TempDir())
	operator, agent := "11111111111111111111111111111111", "22222222222222222222222222222222"
	created, err := store.Create(Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: operator}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	controlled, err := store.SetControl(created.ID, operator, "approved_agent", agent, "execute", []string{"files", "commands", "lifecycle"}, 1, 300)
	if err != nil || !controlled.CanControl(agent, "commands", time.Now()) {
		t.Fatalf("agent control=%#v err=%v", controlled.Control, err)
	}
	if err := store.WithControl(created.ID, agent, "files", func(Workspace) error { return nil }); err != nil {
		t.Fatalf("agent file control=%v", err)
	}
	released, err := store.ReleaseControlAs(created.ID, agent, operator, controlled.Control.Version)
	if err != nil || released.Control.PrincipalID != "" || released.Control.GrantedBy != operator {
		t.Fatalf("agent release=%#v err=%v", released.Control, err)
	}
}

func TestCommandEvidenceDoesNotRetainPrivateInput(t *testing.T) {
	store, _ := New(t.TempDir())
	created, _ := store.Create(Workspace{RepositoryID: "repository", CommitID: "commit", CreatorID: "actor"}, []byte("definition"))
	secret := "export PRIVATE_TOKEN=do-not-share"
	digest := sha256.Sum256([]byte(secret))
	if _, err := store.RecordCommand(created.ID, CommandOutcome{CommandSHA256: hex.EncodeToString(digest[:]), ActorID: "actor", StartedAt: time.Now(), CompletedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(store.root, created.ID+".json"))
	if strings.Contains(string(body), secret) {
		t.Fatal("private terminal input entered the durable record")
	}
}

func TestDefinitionSnapshotIsIndependentOfCaller(t *testing.T) {
	store, _ := New(t.TempDir())
	definition := Definition{Version: 1, Image: "alpine", Tools: []Tool{{Name: "go", Version: "1.25"}}}
	created, err := store.Create(Workspace{RepositoryID: "0123456789abcdef0123456789abcdef", CommitID: "0123456789012345678901234567890123456789", CreatorID: "abcdef0123456789abcdef0123456789", Definition: definition}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	definition.Tools[0].Version = "changed"
	loaded, err := store.Get(created.ID)
	if err != nil || loaded.Definition.Tools[0].Version != "1.25" {
		t.Fatalf("loaded = %#v, %v", loaded, err)
	}
}

func TestDecisionExperimentLaunchClaimReusesRunningWorkspace(t *testing.T) {
	store, _ := New(t.TempDir())
	source := Source{Kind: "decision_experiment", DecisionID: "decision", AlternativeID: "alternative"}
	_, reused, release, err := store.ClaimDecisionExperiment("repo", "commit", "owner", "decision", "alternative")
	if err != nil || reused {
		t.Fatalf("initial claim reused = %v, %v", reused, err)
	}
	created, err := store.Create(Workspace{RepositoryID: "repo", CommitID: "commit", CreatorID: "owner", Source: source, Definition: Definition{Version: 1, Image: "alpine"}}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	created, err = store.Complete(created.ID, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	release()
	existing, reused, release, err := store.ClaimDecisionExperiment("repo", "commit", "owner", "decision", "alternative")
	defer release()
	if err != nil || !reused || existing.ID != created.ID {
		t.Fatalf("retry = %#v, %v, %v", existing, reused, err)
	}
}
