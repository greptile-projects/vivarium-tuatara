package securityadvisories

import (
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
)

func TestDisclosureRequiresEveryAttestedLineAndPublishesOnlyRedactedPacket(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actor := "0123456789abcdef0123456789abcdef"
	repository := "abcdef0123456789abcdef0123456789"
	v, err := store.Create(Advisory{Title: "Private root cause", Description: "restricted exploit detail", Contact: "secret@example.test", ReporterID: actor, AffectedRepositories: []AffectedRepository{{RepositoryID: repository, Versions: []string{"1.x"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PrepareDisclosure(v.ID, actor, v.Version, Disclosure{PublicTitle: "Parser security update", RedactedSummary: "Safe summary", UpgradeGuidance: "Upgrade to 1.2.3."}); err != ErrInvalid {
		t.Fatalf("unattested disclosure = %v", err)
	}
	v, err = store.update(v.ID, func(item *Advisory) error {
		item.ReleaseAttestations = append(item.ReleaseAttestations, ReleaseAttestation{ID: "11111111111111111111111111111111", RepositoryID: repository, VersionLine: "1.x", ReleaseID: "22222222222222222222222222222222", ReleaseCommitID: strings.Repeat("a", 40), ArtifactIDs: []string{"33333333333333333333333333333333"}, ArtifactSHA256: []string{strings.Repeat("b", 64)}})
		item.Version++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	scheduled := time.Now().UTC().Add(time.Hour)
	v, err = store.PrepareDisclosure(v.ID, actor, v.Version, Disclosure{PublicTitle: "Parser security update", RedactedSummary: "Safe summary", UpgradeGuidance: "Upgrade to 1.2.3.", Credits: []string{"Researcher"}, ScheduledAt: &scheduled})
	if err != nil || v.Disclosure.State != "scheduled" || len(v.Disclosure.FixedVersions) != 1 {
		t.Fatalf("disclosure = %#v, %v", v.Disclosure, err)
	}
	v, err = store.SetDisclosureState(v.ID, actor, "published", "", []string{})
	if err != nil || v.EmbargoState != "disclosed" || v.Disclosure.PublishedAt == nil {
		t.Fatalf("published = %#v, %v", v.Disclosure, err)
	}
	if strings.Contains(v.Disclosure.RedactedSummary, v.Description) || strings.Contains(v.Disclosure.UpgradeGuidance, v.Contact) {
		t.Fatal("protected fields leaked into disclosure")
	}
	v, err = store.SetDisclosureState(v.ID, actor, "published", "notifications remain unpublished", []string{"notify_affected_users"})
	if err != nil || v.EmbargoState != "disclosed" || v.Disclosure.State != "published" || len(v.Disclosure.Remaining) != 1 {
		t.Fatalf("published recovery state = %#v, %v", v.Disclosure, err)
	}
}

func TestDiagnosticEvidenceImpactAndBoundedInvestigation(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actor := "0123456789abcdef0123456789abcdef"
	repository := "abcdef0123456789abcdef0123456789"
	v, err := store.Create(Advisory{Title: "Boundary bypass", Description: "A parser boundary may be bypassed.", Contact: "security@example.test", ReporterID: actor, AffectedRepositories: []AffectedRepository{{RepositoryID: repository, Versions: []string{"1.x"}}}, Evidence: []Evidence{{Label: "Reproduction", Description: "A bounded reproduction."}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.AddEvidence(v.ID, actor, Evidence{Kind: "dependency", RepositoryID: repository, Dependency: "parser@1.4.0", Label: "Resolved dependency", Description: "Lockfile resolution."})
	if err != nil {
		t.Fatal(err)
	}
	dependency := v.Evidence[1].ID
	v, err = store.AddFinding(v.ID, actor, "hypothesis", "The vulnerable parser is reachable.", "", []string{dependency})
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.SetImpact(v.ID, actor, v.Version, Impact{RepositoryID: repository, VersionLine: "1.x", Environment: "production", State: "confirmed", EvidenceIDs: []string{dependency}, Rationale: "The production artifact contains this resolution."})
	if err != nil {
		t.Fatal(err)
	}
	credential := "11111111111111111111111111111111"
	v, investigation, err := store.StartInvestigation(v.ID, actor, credential, credential, "Determine exploitability from selected evidence.", []string{dependency})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Findings) != 1 || len(v.ImpactMatrix) != 1 || len(investigation.Evidence) != 1 {
		t.Fatalf("diagnostic record incomplete: %#v", v)
	}
	if _, _, err = store.Investigation(v.ID, investigation.ID, actor); err != ErrNotFound {
		t.Fatalf("unbound credential accessed investigation: %v", err)
	}
}

func TestRepairProofCoversExactVersionAndRequiresIndependentApproval(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	creator := "0123456789abcdef0123456789abcdef"
	approver := "11111111111111111111111111111111"
	repository := "abcdef0123456789abcdef0123456789"
	assignee := "22222222222222222222222222222222"
	v, err := store.Create(Advisory{Title: "Supported-line repair", Description: "Prove the fixed line.", Contact: "security@example.test", ReporterID: creator, AffectedRepositories: []AffectedRepository{{RepositoryID: repository, Versions: []string{"1.x", "2.x"}}}})
	if err != nil {
		t.Fatal(err)
	}
	v, reproduction, err := store.AddSecurityReproduction(v.ID, creator, SecurityReproduction{RepositoryID: repository, VersionLine: "1.x", Definition: checkruns.Definition{Name: "CVE reproduction", Image: "alpine:3.22", Command: "test fixed", TimeoutSeconds: 30}})
	if err != nil || reproduction.Definition.WorkingDirectory != "." {
		t.Fatalf("reproduction = %#v, %v", reproduction, err)
	}
	if _, _, err = store.AddSecurityReproduction(v.ID, creator, SecurityReproduction{RepositoryID: repository, VersionLine: "unsupported", Definition: checkruns.Definition{Name: "bad", Image: "alpine:3.22", Command: "true"}}); err != ErrInvalid {
		t.Fatalf("unsupported reproduction = %v", err)
	}
	v, task, err := store.AddRepairTask(v.ID, creator, RepairTask{RepositoryID: repository, VersionLine: "1.x", Title: "Repair 1.x", Mandate: "Remove the vulnerability.", BaseCommitID: strings.Repeat("a", 40), AssigneeID: assignee, AssigneeKind: "human"})
	if err != nil {
		t.Fatal(err)
	}
	credential := "33333333333333333333333333333333"
	v, session, err := store.StartRepairSession(v.ID, assignee, task.ID, credential, "refs/heads/vivarium-security/test")
	if err != nil {
		t.Fatal(err)
	}
	candidate := strings.Repeat("b", 40)
	v, session, err = store.UpdateRepairSession(v.ID, assignee, session.ID, "complete", "", "", candidate)
	if err != nil {
		t.Fatal(err)
	}
	v, session, err = store.UpdateRepairSession(v.ID, approver, session.ID, "review", "Reviewed exact repair.", "approve", candidate)
	if err != nil {
		t.Fatal(err)
	}
	v, verification, err := store.StartRepairVerification(v.ID, assignee, task.ID, session.ID, []string{"44444444444444444444444444444444"}, []string{"55555555555555555555555555555555"})
	if err != nil || verification.CandidateCommitID != candidate {
		t.Fatalf("verification = %#v, %v", verification, err)
	}
	if _, _, err = store.ApproveRepairVerification(v.ID, assignee, verification.ID); err != ErrInvalid {
		t.Fatalf("self approval = %v", err)
	}
	v, verification, err = store.ApproveRepairVerification(v.ID, approver, verification.ID)
	if err != nil || len(verification.Approvals) != 1 {
		t.Fatalf("approval = %#v, %v", verification, err)
	}
	v, attestation, err := store.AddReleaseAttestation(v.ID, approver, ReleaseAttestation{VerificationID: verification.ID, RepositoryID: repository, VersionLine: "1.x", ReleaseID: "66666666666666666666666666666666", ReleaseCommitID: strings.Repeat("c", 40), ArtifactIDs: []string{"77777777777777777777777777777777"}, ArtifactSHA256: []string{strings.Repeat("d", 64)}})
	if err != nil || len(v.ReleaseAttestations) != 1 || attestation.VersionLine != "1.x" {
		t.Fatalf("attestation = %#v, %v", attestation, err)
	}
}
