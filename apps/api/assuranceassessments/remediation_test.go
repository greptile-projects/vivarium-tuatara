package assuranceassessments

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFindingRemediationRequiresFreshEvidenceDisposition(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	a, err := store.Create(Assessment{RepositoryID: "repo", ProgramID: "program", ProgramVersion: 1, Title: "review", OwnerID: "owner", Assessor: Assessor{UserID: "assessor", Kind: "external", ConflictDisclosure: "none"}, Scope: Scope{ControlIDs: []string{"control"}, PeriodStartsAt: now.Add(-time.Hour), PeriodEndsAt: now}, StartsAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	a, err = store.Append(a.ID, a.Version, "assessor", "assessor", Event{Kind: "finding", Body: "Encryption evidence is incomplete", ControlID: "control", Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	a, err = store.LinkRemediation(a.ID, a.Version, "owner", Remediation{FindingEventID: a.Events[0].ID, ControlID: "control", AffectedRevision: strings.Repeat("a", 40), Deadline: now.Add(24 * time.Hour), AcceptanceCriteria: []string{"fresh evidence passes"}, ProposalID: "proposal", TaskIDs: []string{"task"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.VerifyRemediation(a.ID, a.Remediations[0].ID, a.Version, "owner", "owner", "looks good", "accepted", nil); err != ErrInvalid {
		t.Fatalf("accepted without evidence: %v", err)
	}
	a, err = store.VerifyRemediation(a.ID, a.Remediations[0].ID, a.Version, "assessor", "assessor", "package is current", "accepted", []string{"package"})
	if err != nil || a.Remediations[0].State != "verified" {
		t.Fatalf("verification failed: %#v %v", a.Remediations, err)
	}
	a, err = store.VerifyRemediation(a.ID, a.Remediations[0].ID, a.Version, "owner", "owner", "later regression", "reopened", nil)
	if err != nil || a.Remediations[0].State != "open" || len(a.Remediations[0].Verifications) != 2 || a.Remediations[0].Verifications[0].Disposition != "accepted" {
		t.Fatalf("reopen failed: %#v %v", a.Remediations, err)
	}
}

func TestStatementSignaturePreservesOriginalAfterRevocation(t *testing.T) {
	store, _ := New(t.TempDir())
	now := time.Now().UTC()
	v, err := store.CreateStatement(Statement{RepositoryID: "repo", AssessmentID: "assessment", ReleaseID: "release", ReleaseRevision: strings.Repeat("a", 40), ProgramID: "program", ProgramVersion: 1, Scope: Scope{ControlIDs: []string{"control"}, PeriodStartsAt: now.Add(-time.Hour), PeriodEndsAt: now}, ControlIDs: []string{"control"}, EvidenceDigest: strings.Repeat("b", 64), Audience: []string{"consumer"}, ExpiresAt: now.Add(time.Hour), IssuedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(v.Payload)
	sig, _ := base64.RawURLEncoding.DecodeString(v.Signature)
	pub, _ := base64.RawURLEncoding.DecodeString(v.PublicKey)
	sum := sha256.Sum256(raw)
	if !ed25519.Verify(pub, sum[:], sig) {
		t.Fatal("signature did not verify")
	}
	var payload Statement
	if json.Unmarshal(raw, &payload) != nil || payload.ReleaseRevision != v.ReleaseRevision {
		t.Fatal("payload is not independently inspectable")
	}
	revoked, err := store.RevokeStatement(v.ID, "owner", "release withdrawn")
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Payload != v.Payload || revoked.Signature != v.Signature || revoked.RevokedAt == nil {
		t.Fatal("revocation rewrote the original claim")
	}
}
