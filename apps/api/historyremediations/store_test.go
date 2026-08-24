package historyremediations

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture() Remediation {
	return Remediation{RepositoryID: "repo", RequestID: "request-1", Title: "Remove exposed signing material", Source: Source{Kind: "security_finding", ResourceID: "finding-1"}, ContentDescription: "Signing credential identified by evidence digest", Reason: "Credential entered published history", Scopes: []Scope{{RepositoryID: "repo", Kind: "commit_blob", ObjectID: "blob-1", Revision: strings.Repeat("a", 40), Ref: "refs/heads/main"}}, Evidence: []Evidence{{ID: "e-1", Kind: "scanner_match", ResourceID: "run-1", SHA256: strings.Repeat("b", 64), State: "matched", AttributedTo: "maintainer"}, {ID: "e-2", Kind: "manual_review", ResourceID: "object-2", SHA256: strings.Repeat("c", 64), State: "false_match", Note: "Different object", AttributedTo: "privacy"}}, Constraints: []Constraint{{ID: "c-1", Kind: "legal_hold", State: "unresolved", Reason: "Counsel decision required", AttributedTo: "counsel"}}, AudienceIDs: []string{"maintainer", "security"}, OwnerIDs: []string{"security"}, RequiredApprovals: []Approval{{Role: "legal", ApproverIDs: []string{"counsel"}, Required: 1}}}
}
func TestCreateIsRetryStableAndPrivateOnDisk(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	v := fixture()
	got, e := s.Create(v, "maintainer", "digest-1")
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Create(v, "maintainer", "digest-1")
	if e != nil || again.ID != got.ID {
		t.Fatalf("retry = %#v, %v", again, e)
	}
	if _, e = s.Create(v, "maintainer", "changed"); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed retry = %v", e)
	}
	info, e := os.Stat(s.path("repo", got.ID))
	if e != nil {
		t.Fatal(e)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}
func TestRejectsPayloadLikeMultilineDescriptionAndIncompleteEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	v := fixture()
	v.ContentDescription = "unsafe\npayload"
	if _, e := s.Create(v, "maintainer", "d"); !errors.Is(e, ErrInvalid) {
		t.Fatalf("multiline description = %v", e)
	}
	v = fixture()
	v.Evidence[0].SHA256 = "not-a-digest"
	if _, e := s.Create(v, "maintainer", "d"); !errors.Is(e, ErrInvalid) {
		t.Fatalf("weak evidence = %v", e)
	}
	for name, mutate := range map[string]func(*Remediation){
		"evidence note credential":      func(v *Remediation) { v.Evidence[0].Note = "Authorization: Bearer abcdefghijklmnop" },
		"constraint reason credential":  func(v *Remediation) { v.Constraints[0].Reason = "api_key=abcdefghijklmnop" },
		"unbounded evidence note":       func(v *Remediation) { v.Evidence[0].Note = strings.Repeat("x", 301) },
		"bare JWT in evidence note":     func(v *Remediation) { v.Evidence[0].Note = testJWT() },
		"bare JWT in constraint reason": func(v *Remediation) { v.Constraints[0].Reason = testJWT() },
		"bare JWT in root description":  func(v *Remediation) { v.ContentDescription = testJWT() },
		"AWS access key in evidence":    func(v *Remediation) { v.Evidence[0].Note = "AKIAIOSFODNN7EXAMPLE" },
	} {
		t.Run(name, func(t *testing.T) {
			v := fixture()
			mutate(&v)
			if _, err := s.Create(v, "maintainer", name); !errors.Is(err, ErrInvalid) {
				t.Fatalf("unsafe payload = %v", err)
			}
		})
	}
}

func testJWT() string {
	return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJzZW5zaXRpdmUtaGlzdG9yeSJ9.dGVzdC1zaWduYXR1cmUtZG8tbm90LXVzZQ"
}

func TestReconcilePrecedesMutableValidation(t *testing.T) {
	s, _ := New(t.TempDir())
	v := fixture()
	created, err := s.Create(v, "maintainer", "digest")
	if err != nil {
		t.Fatal(err)
	}
	reconciled, found, err := s.Reconcile(v.RepositoryID, v.RequestID, "digest")
	if err != nil || !found || reconciled.ID != created.ID {
		t.Fatalf("reconcile = %#v, %v, %v", reconciled, found, err)
	}
	if _, _, err = s.Reconcile(v.RepositoryID, v.RequestID, "changed"); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed reconcile = %v", err)
	}
}

func TestExposureMapIsCASVersionedRetryStableAndBoundToAffectedObjects(t *testing.T) {
	s, _ := New(t.TempDir())
	v := fixture()
	created, err := s.Create(v, "maintainer", "digest")
	if err != nil {
		t.Fatal(err)
	}
	finding := ExposureFinding{RequestID: "map-1", CopyKind: "active_clone", ResourceID: "clone-7", ObjectIDs: []string{"blob-1"}, DerivedKinds: []string{"credential"}, State: "suspected", IndependentlyControlled: true, Restricted: true, CitationKind: "owner_attestation", CitationResourceID: "attestation-1", CitationSHA256: strings.Repeat("d", 64), Uncertainty: "Owner has not completed an object scan"}
	updated, err := s.AddExposureFinding("repo", created.ID, 1, finding, "readonly-agent")
	if err != nil || updated.Version != 2 || len(updated.ExposureMap) != 1 || updated.ExposureMap[0].AttributedTo != "readonly-agent" {
		t.Fatalf("append = %#v, %v", updated, err)
	}
	retry, err := s.AddExposureFinding("repo", created.ID, 1, finding, "readonly-agent")
	if err != nil || retry.Version != 2 {
		t.Fatalf("retry = %#v, %v", retry, err)
	}
	finding.RequestID = "map-2"
	if _, err = s.AddExposureFinding("repo", created.ID, 1, finding, "maintainer"); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale append = %v", err)
	}
	finding.ObjectIDs = []string{"not-in-remediation"}
	if _, err = s.AddExposureFinding("repo", created.ID, 2, finding, "maintainer"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign object = %v", err)
	}
	finding.ObjectIDs = []string{"blob-1"}
	finding.Note = testJWT()
	if _, err = s.AddExposureFinding("repo", created.ID, 2, finding, "maintainer"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("credential prose = %v", err)
	}
	finding.Note = ""
	finding.Uncertainty = "Observed identifier ASIAIOSFODNN7EXAMPLE"
	if _, err = s.AddExposureFinding("repo", created.ID, 2, finding, "maintainer"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("AWS credential prose = %v", err)
	}
}

func TestFailedExposureReplacementPreservesLiveRecord(t *testing.T) {
	s, _ := New(t.TempDir())
	created, err := s.Create(fixture(), "maintainer", "digest")
	if err != nil {
		t.Fatal(err)
	}
	path := s.path("repo", created.ID)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s.beforeReplace = func() error { return errors.New("injected replacement failure") }
	finding := ExposureFinding{RequestID: "map-failure", CopyKind: "backup", ResourceID: "backup-1", ObjectIDs: []string{"blob-1"}, State: "unverifiable", CitationKind: "inventory", CitationResourceID: "inventory-1", CitationSHA256: strings.Repeat("e", 64)}
	if _, err = s.AddExposureFinding("repo", created.ID, 1, finding, "maintainer"); err == nil {
		t.Fatal("replacement unexpectedly succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("failed replacement changed live remediation")
	}
	if _, err = s.Get("repo", created.ID); err != nil {
		t.Fatalf("live remediation became unreadable: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".history-remediation-*"))
	if len(matches) != 0 {
		t.Fatalf("temporary replacements remain: %v", matches)
	}
}

func TestRewriteCandidateAndRehearsalAreCASVersionedAndRetryStable(t *testing.T) {
	s, _ := New(t.TempDir())
	created, err := s.Create(fixture(), "maintainer", "digest")
	if err != nil {
		t.Fatal(err)
	}
	candidate := RewriteCandidate{RequestID: "candidate-1", Rules: []RewriteRule{{ID: "rule", AffectedObjectID: "blob-1", Action: "remove", Reason: "Remove affected blob"}}, SelectedRefs: []RewriteRef{{Name: "refs/heads/main", ExpectedTip: strings.Repeat("a", 40)}}, CandidateRefs: []CandidateRef{{Name: "refs/heads/main", OldTip: strings.Repeat("a", 40), NewTip: strings.Repeat("b", 40)}}, CommitMap: []CommitMapping{{OldCommitID: strings.Repeat("a", 40), NewCommitID: strings.Repeat("b", 40), Changed: true}}, ObjectMap: []ObjectMapping{{OldObjectID: "blob-1", Action: "remove"}}, RollbackLimit: "Old copies can restore the lineage.", CollaboratorActions: []string{"replace local branches"}}
	updated, err := s.AddRewriteCandidate("repo", created.ID, 1, candidate, "maintainer")
	if err != nil || updated.Version != 2 || len(updated.RewriteCandidates) != 1 {
		t.Fatalf("candidate = %#v, %v", updated, err)
	}
	retry, err := s.AddRewriteCandidate("repo", created.ID, 1, candidate, "maintainer")
	if err != nil || retry.Version != 2 {
		t.Fatalf("retry = %#v, %v", retry, err)
	}
	kinds := []string{"repository_integrity", "build", "check", "release", "dependency", "clone", "fetch"}
	run := Rehearsal{RequestID: "run-1"}
	for _, kind := range kinds {
		run.Scenarios = append(run.Scenarios, RehearsalScenario{ID: kind, Kind: kind, Expectation: "usable", TimeoutSeconds: 10})
		run.Outcomes = append(run.Outcomes, RehearsalOutcome{ScenarioID: kind, Kind: kind, RefName: "refs/heads/main", State: "passed"})
	}
	finished, err := s.AddRehearsal("repo", created.ID, updated.RewriteCandidates[0].ID, 2, run, "maintainer")
	if err != nil || finished.Version != 3 || finished.RewriteCandidates[0].Rehearsals[0].State != "passed" {
		t.Fatalf("rehearsal = %#v, %v", finished, err)
	}
}
