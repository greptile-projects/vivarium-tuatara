package debugworkspaces

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceRetainsExactContextAndCASHistory(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	v, err := s.Create(Workspace{RepositoryID: "11111111111111111111111111111111", Title: "Intermittent checkout failure", Summary: "Users receive a timeout after payment authorization.", Trigger: Reference{Kind: "manual_observation", Label: "operator report"}, Release: Reference{Kind: "release", ResourceID: "22222222222222222222222222222222", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Label: "v1.4.2"}, Environment: Reference{Kind: "environment", ResourceID: "33333333333333333333333333333333", Label: "production"}, TimeStart: start, TimeEnd: start.Add(time.Hour), UserJourney: "complete checkout", OwnerIDs: []string{"44444444444444444444444444444444"}, Severity: "high", Audience: "restricted", AccessUserIDs: []string{"55555555555555555555555555555555"}, Source: Reference{Kind: "commit", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Label: "released source"}, Configuration: Reference{Kind: "configuration", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Label: "release config"}, Infrastructure: Reference{Kind: "infrastructure", Label: "unavailable"}, Evidence: []Evidence{{Kind: "trace", Label: "sample trace", Visibility: "restricted", Sanitization: "user identifiers removed", Available: false, UnavailableReason: "trace access denied"}}}, "44444444444444444444444444444444")
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 1 || v.Evidence[0].ID == "" || len(v.History) != 1 || len(v.UnavailableContext) != 0 {
		t.Fatalf("unexpected workspace: %+v", v)
	}
	v, err = s.Update(v.RepositoryID, v.ID, "55555555555555555555555555555555", "hypothesis", "queue saturation delayed authorization callbacks", "initial correlation", 1)
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 2 || len(v.Hypotheses) != 1 || v.Hypotheses[0].CreatedBy != "55555555555555555555555555555555" {
		t.Fatalf("history was not attributable: %+v", v)
	}
	_, err = s.Update(v.RepositoryID, v.ID, "55555555555555555555555555555555", "status", "resolved", "", 1)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("wanted conflict, got %v", err)
	}
}

func TestProbeLifecycleNarrowsAuthorityAndRetainsPartialEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	owner, requester := "44444444444444444444444444444444", "55555555555555555555555555555555"
	v, err := s.Create(Workspace{RepositoryID: "11111111111111111111111111111111", Title: "x", Summary: "x", Trigger: Reference{Kind: "trace", Label: "x"}, Release: Reference{ResourceID: "22222222222222222222222222222222", Revision: strings.Repeat("a", 40)}, Environment: Reference{ResourceID: "33333333333333333333333333333333"}, TimeStart: now.Add(-time.Hour), TimeEnd: now, UserJourney: "x", OwnerIDs: []string{owner}, Severity: "high", Audience: "repository", Source: Reference{Revision: strings.Repeat("a", 40)}}, owner)
	if err != nil {
		t.Fatal(err)
	}
	requested := Probe{Kind: "dynamic_diagnostic", Purpose: "inspect queue depth", DefinitionPath: ".vivarium/diagnostics/queue.json", DefinitionRevision: v.Source.Revision, AudienceUserIDs: []string{requester, owner}, ExpiresAt: now.Add(time.Hour), RequestedPolicy: ProbePolicy{DataCategories: []string{"aggregate_state", "request_metadata"}, Privacy: "hash_user_identifiers", Security: "redact_secrets", RetentionHours: 24, SamplePercent: 50, MaxCostCents: 500, MaxLoadPercent: 10}}
	v, err = s.RequestProbe(v.RepositoryID, v.ID, requester, requested, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if v.Probes[0].Status != "pending" || v.Probes[0].ApprovedPolicy != nil {
		t.Fatalf("request = %#v", v.Probes[0])
	}
	approved := ProbePolicy{DataCategories: []string{"aggregate_state"}, Privacy: "remove_user_identifiers", Security: "drop_secret_bearing_records", RetentionHours: 12, SamplePercent: 10, MaxCostCents: 100, MaxLoadPercent: 5}
	weakened := approved
	weakened.Security = "detect_secrets"
	if _, err = s.DecideProbe(v.RepositoryID, v.ID, v.Probes[0].ID, owner, "approved", "unsafe weakening", weakened, now.Add(30*time.Minute), v.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("weakened approval = %v", err)
	}
	widened := requested.RequestedPolicy
	widened.MaxLoadPercent++
	if _, err = s.DecideProbe(v.RepositoryID, v.ID, v.Probes[0].ID, owner, "approved", "safe bounded capture", widened, now.Add(30*time.Minute), v.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("widened approval = %v", err)
	}
	v, err = s.DecideProbe(v.RepositoryID, v.ID, v.Probes[0].ID, owner, "approved", "safe bounded capture", approved, now.Add(30*time.Minute), v.Version)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	action := ProbeAction{Outcome: "partial", StartedAt: now.Add(-30 * time.Second), FinishedAt: now, Provenance: "collector run run-1 in the approved environment", Transformations: []string{"secret filtering", "identifier hashing"}, Gaps: []string{"two overloaded instances stopped capture"}, Artifacts: []ProbeArtifact{{Kind: "diagnostic", Digest: strings.Repeat("b", 64), SizeBytes: 120, Reference: "artifact://run-1/queue", Redaction: "credentials removed and user IDs hashed"}}}
	v, err = s.ReportProbe(v.RepositoryID, v.ID, v.Probes[0].ID, requester, action, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if v.Probes[0].Status != "partial" || len(v.Probes[0].Actions) != 1 || len(v.Probes[0].Actions[0].Gaps) != 1 {
		t.Fatalf("partial result = %#v", v.Probes[0])
	}
}

func TestProbeExpiryAndCompleteGapFailClosed(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	owner := "44444444444444444444444444444444"
	v, _ := s.Create(Workspace{RepositoryID: "11111111111111111111111111111111", Title: "x", Summary: "x", Trigger: Reference{Kind: "trace", Label: "x"}, Release: Reference{ResourceID: "22222222222222222222222222222222", Revision: strings.Repeat("a", 40)}, Environment: Reference{ResourceID: "33333333333333333333333333333333"}, TimeStart: now.Add(-time.Hour), TimeEnd: now, UserJourney: "x", OwnerIDs: []string{owner}, Severity: "low", Audience: "repository", Source: Reference{Revision: strings.Repeat("a", 40)}}, owner)
	policy := ProbePolicy{DataCategories: []string{"application_logs"}, Privacy: "remove_user_data", Security: "redact_secrets", RetentionHours: 1, SamplePercent: 5, MaxCostCents: 10, MaxLoadPercent: 2}
	v, _ = s.RequestProbe(v.RepositoryID, v.ID, owner, Probe{Kind: "logs", Purpose: "inspect failures", AudienceUserIDs: []string{owner}, RequestedPolicy: policy, ExpiresAt: now.Add(10 * time.Minute)}, v.Version)
	v, _ = s.DecideProbe(v.RepositoryID, v.ID, v.Probes[0].ID, owner, "approved", "bounded", policy, now.Add(5*time.Minute), v.Version)
	bad := ProbeAction{Outcome: "complete", StartedAt: now, FinishedAt: now, Provenance: "collector", Transformations: []string{"redacted"}, Gaps: []string{"missing shard"}}
	if _, err := s.ReportProbe(v.RepositoryID, v.ID, v.Probes[0].ID, owner, bad, v.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("complete gap = %v", err)
	}
	now = now.Add(6 * time.Minute)
	if _, err := s.ReportProbe(v.RepositoryID, v.ID, v.Probes[0].ID, owner, ProbeAction{Outcome: "denied", StartedAt: now, FinishedAt: now, Provenance: "collector", Transformations: []string{"redacted"}, Gaps: []string{"expired"}}, v.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expired report = %v", err)
	}
}

func TestWorkspaceRejectsUnboundedOrUnexplainedEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	start := time.Now().UTC()
	base := Workspace{RepositoryID: "11111111111111111111111111111111", Title: "x", Summary: "x", Trigger: Reference{Kind: "trace", Label: "x"}, Release: Reference{ResourceID: "22222222222222222222222222222222", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Environment: Reference{ResourceID: "33333333333333333333333333333333"}, TimeStart: start, TimeEnd: start.Add(32 * 24 * time.Hour), UserJourney: "x", OwnerIDs: []string{"44444444444444444444444444444444"}, Severity: "low", Audience: "repository", Source: Reference{Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Evidence: []Evidence{{Kind: "log", Label: "missing", Visibility: "repository", Sanitization: "redacted", Available: false}}}
	if _, err := s.Create(base, "44444444444444444444444444444444"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wanted invalid, got %v", err)
	}
}

func TestCitedDiagnosisIsChallengeableAndRevokedAgentFailsClosed(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	owner := "44444444444444444444444444444444"
	v, err := s.Create(Workspace{RepositoryID: "11111111111111111111111111111111", Title: "checkout path", Summary: "timeouts", Trigger: Reference{Kind: "trace", Label: "trace"}, Release: Reference{ResourceID: "22222222222222222222222222222222", Revision: strings.Repeat("a", 40)}, Environment: Reference{ResourceID: "33333333333333333333333333333333"}, TimeStart: now.Add(-time.Hour), TimeEnd: now, UserJourney: "checkout", OwnerIDs: []string{owner}, Severity: "high", Audience: "repository", Source: Reference{Revision: strings.Repeat("a", 40)}}, owner)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.AddClaim(v.RepositoryID, v.ID, owner, []Citation{{Kind: "runtime_evidence", Label: "redacted trace", Accessible: false, BlockedReason: "privacy audience excludes this reader"}}, Claim{Kind: "hypothesis", Statement: "retry path exhausted the pool", Uncertainty: "one shard is inaccessible", Confidence: "medium"}, v.Version)
	if err != nil || v.Claims[0].Status != "blocked" {
		t.Fatalf("blocked claim = %#v, %v", v.Claims, err)
	}
	v, err = s.RespondClaim(v.RepositoryID, v.ID, v.Claims[0].ID, owner, "dispute", "the stack points at the non-retry path", nil, v.Version)
	if err != nil || v.Claims[0].Status != "disputed" {
		t.Fatalf("dispute = %#v, %v", v.Claims[0], err)
	}
	agentID, credentialID := "55555555555555555555555555555555", "66666666666666666666666666666666"
	v, investigation, err := s.StartAgent(v.RepositoryID, v.ID, owner, agentID, credentialID, "inspect only the cited trace", []string{v.Citations[0].ID}, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{strings.Repeat("x", 8001), "response included secret=should-never-persist"} {
		_, claimErr := s.AgentClaim(v.RepositoryID, v.ID, investigation.ID, credentialID, Claim{Kind: "finding", Statement: statement, Uncertainty: "bounded uncertainty", Confidence: "low", CitationIDs: []string{v.Citations[0].ID}}, v.Version)
		if !errors.Is(claimErr, ErrInvalid) {
			t.Fatalf("unsafe agent statement was accepted: %v", claimErr)
		}
	}
	reopened, reopenErr := New(s.root)
	if reopenErr != nil {
		t.Fatal(reopenErr)
	}
	persisted, reopenErr := reopened.Get(v.RepositoryID, v.ID)
	if reopenErr != nil || len(persisted.Claims) != 1 {
		t.Fatalf("rejected agent text reached durable state: %#v, %v", persisted.Claims, reopenErr)
	}
	v, _, err = s.ControlAgent(v.RepositoryID, v.ID, investigation.ID, owner, "revoke", "evidence consent changed", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AgentClaim(v.RepositoryID, v.ID, investigation.ID, credentialID, Claim{Kind: "finding", Statement: "late output", Uncertainty: "revoked", Confidence: "low", CitationIDs: []string{v.Citations[0].ID}}, v.Version)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoked agent publication = %v", err)
	}
}
