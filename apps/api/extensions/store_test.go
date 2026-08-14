package extensions

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateRejectsContractsEmptyAfterNormalization(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := Registration{
		Name:                 "Review lens",
		OperatorContact:      "ops@example.test",
		Capabilities:         []string{" \t"},
		CallbackURL:          "https://example.test/events",
		ActionURL:            "https://example.test/actions",
		RequestedPermissions: []Permission{{Resource: "pull_requests", Actions: []string{"read"}}},
		SupportedEvents:      []string{" \n"},
		CredentialRotation:   RotationPolicy{IntervalDays: 30},
	}
	if _, err = store.Create("0123456789abcdef0123456789abcdef", in, time.Now()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Create error = %v, want ErrInvalid", err)
	}
}

func TestInstallationInputBindsPublicJSONContract(t *testing.T) {
	var input InstallationInput
	err := json.Unmarshal([]byte(`{"owner_type":"repository","owner_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","repository_ids":["bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"],"resource_types":["pull_requests"],"capability_decisions":[{"capability":"review","decision":"approved"}],"settings":{"label":"review"}}`), &input)
	if err != nil || input.OwnerType != "repository" || len(input.RepositoryIDs) != 1 || len(input.CapabilityDecisions) != 1 || input.Settings["label"] != "review" {
		t.Fatalf("input = %#v, error = %v", input, err)
	}
}

func TestWithInstallationsRepositoryHoldsMutationBoundary(t *testing.T) {
	store, _ := New(t.TempDir())
	installation := Installation{ID: strings.Repeat("a", 32), RepositoryIDs: []string{strings.Repeat("b", 32)}, Version: 1}
	if err := writeAtomic(filepath.Join(store.root, "installation-"+installation.ID+".json"), installation); err != nil {
		t.Fatal(err)
	}
	readFinished := make(chan struct{})
	err := store.WithInstallationsRepository([]string{installation.ID}, installation.RepositoryIDs[0], func() error {
		go func() { _, _ = store.GetInstallation(installation.ID); close(readFinished) }()
		select {
		case <-readFinished:
			t.Fatal("installation read crossed dependent write boundary")
		case <-time.After(20 * time.Millisecond):
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-readFinished:
	case <-time.After(time.Second):
		t.Fatal("installation boundary was not released")
	}
}

func TestInstallationScopesDecisionsAndRetainsLifecycleHistory(t *testing.T) {
	store, _ := New(t.TempDir())
	extension, err := store.Create(strings.Repeat("a", 32), Registration{Name: "Review lens", OperatorContact: "ops@example.test", Capabilities: []string{"review", "summarize"}, CallbackURL: "https://example.test/events", ActionURL: "https://example.test/actions", RequestedPermissions: []Permission{{Resource: "pull_requests", Actions: []string{"read", "comment"}}, {Resource: "issues", Actions: []string{"read"}}}, SupportedEvents: []string{"pull_request.opened"}, CredentialRotation: RotationPolicy{IntervalDays: 30}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	in := InstallationInput{OwnerType: "repository", OwnerID: strings.Repeat("b", 32), RepositoryIDs: []string{strings.Repeat("b", 32)}, ResourceTypes: []string{"pull_requests"}, CapabilityDecisions: []CapabilityDecision{{Capability: "review", Decision: "approved"}, {Capability: "summarize", Decision: "denied", Reason: "not needed"}}, Settings: map[string]string{"label": "extension-review"}}
	installed, err := store.CreateInstallation(extension.ID, strings.Repeat("a", 32), in)
	if err != nil || len(installed.EffectiveAccess) != 1 || installed.EffectiveAccess[0].Resource != "pull_requests" {
		t.Fatalf("installation=%#v error=%v", installed, err)
	}
	if _, err = store.ChangeInstallation(installed.ID, strings.Repeat("a", 32), "suspend", installed.Version, nil, nil); err != nil {
		t.Fatal(err)
	}
	suspended, _ := store.GetInstallation(installed.ID)
	if suspended.Status != "suspended" || len(suspended.Events) != 2 {
		t.Fatalf("suspended=%#v", suspended)
	}
	if _, err = store.ChangeInstallation(installed.ID, strings.Repeat("a", 32), "resume", 1, nil, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale error=%v", err)
	}
	if _, err = store.CreateInstallation(extension.ID, strings.Repeat("a", 32), InstallationInput{OwnerType: "repository", OwnerID: strings.Repeat("b", 32), RepositoryIDs: []string{strings.Repeat("b", 32)}, ResourceTypes: []string{"pull_requests"}, CapabilityDecisions: in.CapabilityDecisions, Settings: map[string]string{"api_token": "nope"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("secret setting error=%v", err)
	}
}

func TestInstallationWithAllCapabilitiesDeniedHasNoEffectiveAccess(t *testing.T) {
	store, _ := New(t.TempDir())
	extension, err := store.Create(strings.Repeat("a", 32), Registration{Name: "Review lens", OperatorContact: "ops@example.test", Capabilities: []string{"review"}, CallbackURL: "https://example.test/events", ActionURL: "https://example.test/actions", RequestedPermissions: []Permission{{Resource: "pull_requests", Actions: []string{"read"}}}, SupportedEvents: []string{"pull_request.opened"}, CredentialRotation: RotationPolicy{IntervalDays: 30}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	installed, err := store.CreateInstallation(extension.ID, strings.Repeat("a", 32), InstallationInput{OwnerType: "repository", OwnerID: strings.Repeat("b", 32), RepositoryIDs: []string{strings.Repeat("b", 32)}, ResourceTypes: []string{"pull_requests"}, CapabilityDecisions: []CapabilityDecision{{Capability: "review", Decision: "denied"}}, Settings: map[string]string{}})
	if err != nil || len(installed.EffectiveAccess) != 0 {
		t.Fatalf("installation = %#v, error = %v", installed, err)
	}
}

func TestInstallationTransferSynchronizesRepositoryScope(t *testing.T) {
	store, _ := New(t.TempDir())
	extension, _ := store.Create(strings.Repeat("a", 32), Registration{Name: "Review lens", OperatorContact: "ops@example.test", Capabilities: []string{"review"}, CallbackURL: "https://example.test/events", ActionURL: "https://example.test/actions", RequestedPermissions: []Permission{{Resource: "pull_requests", Actions: []string{"read"}}}, SupportedEvents: []string{"pull_request.opened"}, CredentialRotation: RotationPolicy{IntervalDays: 30}}, time.Now())
	input := InstallationInput{OwnerType: "repository", OwnerID: strings.Repeat("b", 32), RepositoryIDs: []string{strings.Repeat("b", 32)}, ResourceTypes: []string{"pull_requests"}, CapabilityDecisions: []CapabilityDecision{{Capability: "review", Decision: "approved"}}, Settings: map[string]string{}}
	installed, _ := store.CreateInstallation(extension.ID, strings.Repeat("a", 32), input)
	newOwner := strings.Repeat("c", 32)
	transferred, err := store.ChangeInstallation(installed.ID, strings.Repeat("a", 32), "transfer", installed.Version, &InstallationInput{OwnerType: "repository", OwnerID: newOwner}, nil)
	if err != nil || transferred.OwnerID != newOwner || len(transferred.RepositoryIDs) != 1 || transferred.RepositoryIDs[0] != newOwner {
		t.Fatalf("installation = %#v, error = %v", transferred, err)
	}
}

func TestContractUpdateRequiresInstallationOwnerConsent(t *testing.T) {
	store, _ := New(t.TempDir())
	owner, repository := strings.Repeat("a", 32), strings.Repeat("b", 32)
	original := Registration{Name: "Review lens", OperatorContact: "ops@example.test", Capabilities: []string{"review"}, CallbackURL: "https://example.test/events", ActionURL: "https://example.test/actions", RequestedPermissions: []Permission{{Resource: "pull_requests", Actions: []string{"read"}}}, SupportedEvents: []string{"pull_request.opened"}, CredentialRotation: RotationPolicy{IntervalDays: 30}}
	extension, _ := store.Create(owner, original, time.Now())
	installation, _ := store.CreateInstallation(extension.ID, owner, InstallationInput{OwnerType: "repository", OwnerID: repository, RepositoryIDs: []string{repository}, ResourceTypes: []string{"pull_requests"}, CapabilityDecisions: []CapabilityDecision{{Capability: "review", Decision: "approved"}}, Settings: map[string]string{}})
	updatedInput := original
	updatedInput.Capabilities = []string{"review", "deploy"}
	updatedInput.RequestedPermissions = []Permission{{Resource: "pull_requests", Actions: []string{"read"}}, {Resource: "deployments", Actions: []string{"write"}}}
	updated, err := store.UpdateContract(extension.ID, owner, 1, updatedInput, time.Now())
	if err != nil || updated.ContractVersion != 2 {
		t.Fatalf("extension=%#v error=%v", updated, err)
	}
	retained, _ := store.GetInstallation(installation.ID)
	if retained.ContractVersion != 1 || len(retained.EffectiveAccess) != 1 {
		t.Fatalf("authority changed without consent: %#v", retained)
	}
	if _, err = store.ChangeInstallation(installation.ID, owner, "upgrade", installation.Version, &InstallationInput{OwnerType: "repository", OwnerID: repository, RepositoryIDs: []string{repository}, ResourceTypes: []string{"pull_requests", "deployments"}, CapabilityDecisions: []CapabilityDecision{{Capability: "review", Decision: "approved"}, {Capability: "deploy", Decision: "denied"}}, Settings: map[string]string{}}, nil); err != nil {
		t.Fatal(err)
	}
	consented, _ := store.GetInstallation(installation.ID)
	if consented.ContractVersion != 2 {
		t.Fatalf("contract version=%d", consented.ContractVersion)
	}
}

func TestRotateDerivedCredentialAtomicallyPreservesSuccessor(t *testing.T) {
	store, _ := New(t.TempDir())
	owner, repository := strings.Repeat("a", 32), strings.Repeat("b", 32)
	extension, _ := store.Create(owner, Registration{Name: "Review lens", OperatorContact: "ops@example.test", Capabilities: []string{"review"}, CallbackURL: "https://example.test/events", ActionURL: "https://example.test/actions", RequestedPermissions: []Permission{{Resource: "pull_requests", Actions: []string{"write"}}}, SupportedEvents: []string{"pull_request.opened"}, CredentialRotation: RotationPolicy{IntervalDays: 30}}, time.Now())
	installation, _ := store.CreateInstallation(extension.ID, owner, InstallationInput{OwnerType: "repository", OwnerID: repository, RepositoryIDs: []string{repository}, ResourceTypes: []string{"pull_requests"}, CapabilityDecisions: []CapabilityDecision{{Capability: "review", Decision: "approved"}}, Settings: map[string]string{}})
	oldID, newID := strings.Repeat("c", 32), strings.Repeat("d", 32)
	installation, _ = store.RecordDerivedCredential(installation.ID, oldID, installation.Version)
	rotated, err := store.RotateDerivedCredential(installation.ID, newID, installation.Version, false)
	if err != nil || len(rotated.DerivedCredentialIDs) != 1 || rotated.DerivedCredentialIDs[0] != newID {
		t.Fatalf("rotation=%#v error=%v", rotated, err)
	}
	if _, err = store.RotateDerivedCredential(installation.ID, strings.Repeat("e", 32), installation.Version, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale retry error=%v", err)
	}
}
