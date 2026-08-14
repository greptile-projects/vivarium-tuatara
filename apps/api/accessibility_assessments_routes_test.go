package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilitycommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityreports"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestAccessibilityAssessmentAPIRejectsUnknownRevisionAndCitation(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	previewStore, _ := previews.New(t.TempDir())
	reportStore, _ := accessibilityreports.New(t.TempDir())
	assessmentStore, _ := accessibilityassessments.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, nil, nil, nil, nil, nil, previewStore, reportStore, assessmentStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "assessment-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"assessment"}`, owner.Credential.Token, http.StatusCreated)
	var catalogRepository repositories.Repository
	if err := json.NewDecoder(response.Body).Decode(&catalogRepository); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	unknown := strings.Repeat("d", 40)
	payload := `{"revision":"` + unknown + `","checks":[{"name":"Semantics","category":"semantics","outcome":"passed","source_locations":["page.tsx"],"audience_ids":["screen_reader"],"summary":"No violations."}]}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+catalogRepository.ID+"/accessibility-assessments", payload, owner.Credential.Token, http.StatusBadRequest).Body.Close()

	repository, _ := git.Open(catalogRepository.ID)
	tree, _ := repository.WriteObject(storage.TreeObject, nil)
	commit, _ := repository.WriteObject(storage.CommitObject, []byte("tree "+string(tree)+"\nauthor Test <test@example.com> 0 +0000\ncommitter Test <test@example.com> 0 +0000\n\naccessible\n"))
	if err := repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	hidden, _ := repository.WriteObject(storage.CommitObject, []byte("tree "+string(tree)+"\nparent "+string(commit)+"\nauthor Test <test@example.com> 1 +0000\ncommitter Test <test@example.com> 1 +0000\n\nhidden\n"))
	if err := repository.CreateReference(storage.Reference{Name: "refs/heads/vivarium-security/incident-42", Target: string(hidden)}); err != nil {
		t.Fatal(err)
	}
	hiddenPayload := strings.Replace(payload, unknown, string(hidden), 1)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+catalogRepository.ID+"/accessibility-assessments", hiddenPayload, owner.Credential.Token, http.StatusBadRequest).Body.Close()
	payload = strings.Replace(payload, unknown, string(commit), 1)
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+catalogRepository.ID+"/accessibility-assessments", payload, owner.Credential.Token, http.StatusCreated)
	var assessment accessibilityassessments.Assessment
	if err := json.NewDecoder(createdResponse.Body).Decode(&assessment); err != nil {
		t.Fatal(err)
	}
	createdResponse.Body.Close()
	finding := `{"title":"Focus skipped","detail":"Save is skipped.","severity":"major","audience_ids":["keyboard"],"source_locations":["page.tsx"],"uncertainty":"Observed once.","citations":[{"kind":"preview","resource_id":"invented","revision":"` + string(commit) + `","evidence_ref":"artifact://invented"}]}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+catalogRepository.ID+"/accessibility-assessments/"+assessment.ID+"/findings", finding, owner.Credential.Token, http.StatusBadRequest).Body.Close()
}

func TestAcceptedAccessibilityFindingCreatesGovernedRepair(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	reportStore, _ := accessibilityreports.New(t.TempDir())
	commitmentStore, _ := accessibilitycommitments.New(t.TempDir())
	assessmentStore, _ := accessibilityassessments.New(t.TempDir())
	proposalStore, _ := proposals.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, proposalStore, nil, nil, nil, nil, reportStore, commitmentStore, assessmentStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "repair-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"accessible-repair"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	if err := json.NewDecoder(response.Body).Decode(&repo); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	revision := strings.Repeat("a", 40)
	report, err := reportStore.Create(repo.ID, "affected-user", accessibilityreports.Report{Target: accessibilityreports.Target{Kind: "page", ResourceID: "settings", Revision: revision}, AccessNeeds: []string{"keyboard"}, ExpectedOutcome: "Save receives focus", Steps: []string{"Press Tab"}, Evidence: []accessibilityreports.Artifact{{Kind: "input_trace", Description: "Redacted key sequence", ContentRef: "artifact://report", Redacted: true}}})
	if err != nil {
		t.Fatal(err)
	}
	report, err = reportStore.AddAttempt(repo.ID, report.ID, owner.User.ID, accessibilityreports.Attempt{Boundary: "workspace", Environment: accessibilityreports.Environment{Browser: "Firefox", Device: "desktop", AssistiveTechnology: "keyboard"}, Outcome: "reproducible", Notes: "Focus skips Save", Evidence: []accessibilityreports.Artifact{{Kind: "input_trace", Description: "Redacted reproduction", ContentRef: "artifact://attempt", Redacted: true}}})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := assessmentStore.Create(repo.ID, owner.User.ID, accessibilityassessments.Assessment{Revision: revision, Checks: []accessibilityassessments.Check{{Name: "Keyboard", Category: "keyboard", Outcome: "failed", SourceLocations: []string{"settings.tsx"}, AudienceIDs: []string{"keyboard"}, Summary: "Save is skipped"}}})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err = assessmentStore.AddFinding(repo.ID, assessment.ID, "human", owner.User.ID, accessibilityassessments.Finding{Title: "Save is skipped", Detail: "Keyboard focus moves past Save.", Severity: "major", AudienceIDs: []string{"keyboard"}, SourceLocations: []string{"settings.tsx"}, Uncertainty: "Confirmed in one environment", Citations: []accessibilityassessments.Citation{{Kind: "reproduction", ResourceID: report.Attempts[0].ID, Revision: revision, EvidenceRef: "artifact://attempt"}}})
	if err != nil {
		t.Fatal(err)
	}
	findingID := assessment.Findings[0].ID
	if _, err = assessmentStore.Decide(repo.ID, assessment.ID, findingID, owner.User.ID, "accepted", "The retained reproduction confirms the barrier."); err != nil {
		t.Fatal(err)
	}
	commitment, err := commitmentStore.Create(repo.ID, owner.User.ID, accessibilitycommitments.Revision{Title: "Keyboard settings", Summary: "Settings remain keyboard operable", Subject: accessibilitycommitments.Subject{Kind: "component", ResourceID: "settings-form", Name: "Settings form"}, Standards: []accessibilitycommitments.Standard{{Name: "WCAG", Version: "2.2", Level: "AA", Criteria: []string{"2.1.1"}}}, AssistiveTechnologies: []accessibilitycommitments.AssistiveTechnology{{ID: "keyboard", Name: "Keyboard", Version: "standard", Input: "keyboard", EnvironmentIDs: []string{"desktop"}}}, Audiences: []accessibilitycommitments.Audience{{ID: "keyboard", Name: "Keyboard users", AccessNeeds: []string{"keyboard input"}}}, Environments: []accessibilitycommitments.Environment{{ID: "desktop", Browser: "Firefox", BrowserVersion: "current", OS: "Linux", Device: "desktop", Supported: true}}, Scenarios: []accessibilitycommitments.Scenario{{ID: "save", Name: "Save settings", Steps: []string{"Tab to Save"}, ExpectedOutcome: "Save receives focus", StandardCriteria: []string{"2.1.1"}, AudienceIDs: []string{"keyboard"}, TechnologyIDs: []string{"keyboard"}, EnvironmentIDs: []string{"desktop"}, OwnerIDs: []string{owner.User.ID}}}, SeverityPolicy: []accessibilitycommitments.SeverityRule{{Severity: "major", Definition: "Control is unreachable", Response: "Repair before release", ResolutionDays: 7}}, OwnerIDs: []string{owner.User.ID}, Requirements: []accessibilitycommitments.Requirement{{ID: "focus-order", Statement: "Focus follows visual order"}}, Rationale: "Component contract"})
	if err != nil {
		t.Fatal(err)
	}
	reservedRequest := accessibilityassessments.Repair{BaseRevision: revision, AcceptanceCriteria: []string{"Save receives visible keyboard focus"}, CommitmentID: commitment.ID, CommitmentVersion: 1, CommitmentTitle: "Keyboard settings", ComponentGuidance: []string{"Use the shared focus-ring primitive"}, PermittedEvidence: []accessibilityassessments.RepairEvidence{{Kind: "reproduction", ResourceID: report.Attempts[0].ID, EvidenceRef: "artifact://attempt", Summary: "Expected: Save receives focus. Steps: Press Tab. Reproduction reproducible: Focus skips Save. Redacted input_trace: Redacted reproduction (artifact://attempt)"}}, AssigneeType: "human", AssigneeID: owner.User.ID}
	_, reservation, err := assessmentStore.ReserveRepair(repo.ID, assessment.ID, findingID, owner.User.ID, reservedRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = assessmentStore.Invalidate(repo.ID, assessment.ID, owner.User.ID, []string{"settings.tsx"}, nil); err != nil {
		t.Fatal(err)
	}
	body := `{"commitment_id":"` + commitment.ID + `","commitment_version":1,"acceptance_criteria":["Save receives visible keyboard focus"],"component_guidance":["Use the shared focus-ring primitive"],"assignee_type":"human","assignee_id":"` + owner.User.ID + `"}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/accessibility-assessments/"+assessment.ID+"/findings/"+findingID+"/repair", body, owner.Credential.Token, http.StatusCreated)
	var projection struct {
		Assessment accessibilityassessments.Assessment `json:"assessment"`
		Proposal   proposals.Proposal                  `json:"proposal"`
		Task       proposals.Task                      `json:"task"`
	}
	if err = json.NewDecoder(response.Body).Decode(&projection); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if projection.Task.Assignment == nil || projection.Task.Assignment.Access.BaseRevision != revision || projection.Proposal.Reasoning == nil || projection.Proposal.Reasoning.AccessibilityFindingID != findingID {
		t.Fatalf("repair did not freeze governed context: %+v", projection)
	}
	if projection.Assessment.Findings[0].Repair == nil || projection.Assessment.Findings[0].Repair.RecoveryID != reservation.RecoveryID || projection.Assessment.Findings[0].Repair.State != "linked" {
		t.Fatalf("reserved repair did not recover after invalidation: %+v", projection.Assessment.Findings[0].Repair)
	}
	stored, _ := assessmentStore.Get(repo.ID, assessment.ID)
	if stored.Findings[0].Repair == nil || stored.Findings[0].Repair.CreatedBy != owner.User.ID || len(stored.Findings[0].Repair.PermittedEvidence) != 1 {
		t.Fatalf("finding progress link missing: %+v", stored.Findings[0])
	}
	if summary := stored.Findings[0].Repair.PermittedEvidence[0].Summary; !strings.Contains(summary, "Press Tab") || strings.Contains(summary, "affected-user") {
		t.Fatalf("repair evidence was not useful and privacy bounded: %q", summary)
	}
	if stored.Findings[0].Repair.CreatedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatal("repair attribution timestamp missing")
	}
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/accessibility-assessments/"+assessment.ID+"/findings/"+findingID+"/repair", "", owner.Credential.Token, http.StatusOK)
	response.Body.Close()
}

func TestAccessibilityRevisionMustBeReachableFromBranch(t *testing.T) {
	git, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository, err := git.Create("repo")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := repository.WriteObject(storage.TreeObject, nil)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repository.WriteObject(storage.CommitObject, []byte("tree "+string(tree)+"\nauthor Test <test@example.com> 0 +0000\ncommitter Test <test@example.com> 0 +0000\n\naccessible\n"))
	if err != nil {
		t.Fatal(err)
	}
	if accessibilityRevisionIsVisible(git, "repo", string(commit)) {
		t.Fatal("unreachable commit was visible")
	}
	if err = repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	if !accessibilityRevisionIsVisible(git, "repo", string(commit)) {
		t.Fatal("reachable commit was rejected")
	}
	hidden, err := repository.WriteObject(storage.CommitObject, []byte("tree "+string(tree)+"\nparent "+string(commit)+"\nauthor Test <test@example.com> 1 +0000\ncommitter Test <test@example.com> 1 +0000\n\nhidden\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.CreateReference(storage.Reference{Name: "refs/heads/vivarium-security/incident-42", Target: string(hidden)}); err != nil {
		t.Fatal(err)
	}
	if accessibilityRevisionIsVisible(git, "repo", string(hidden)) {
		t.Fatal("hidden security commit was visible")
	}
	if accessibilityRevisionIsVisible(git, "repo", strings.Repeat("d", 40)) {
		t.Fatal("nonexistent commit was visible")
	}
}

func TestAccessibilityCitationsResolveAssociatedEvidence(t *testing.T) {
	previewStore, _ := previews.New(t.TempDir())
	repositoryID := strings.Repeat("1", 32)
	pullID := strings.Repeat("2", 32)
	config, digest, err := previews.ParseConfig([]byte(`{"version":1,"image":"alpine:3.22","build":"true","output_path":"dist","resources":{"cpus":1,"memory_mb":128,"storage_mb":32,"timeout_seconds":30},"access":{"network":"none","data":"preview_artifacts","identity":"named_users","actions":["feedback"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	revision := strings.Repeat("a", 40)
	preview, err := previewStore.Create(repositoryID, pullID, revision, "owner", digest, "run", config)
	if err != nil {
		t.Fatal(err)
	}
	_, finding, err := previewStore.AddFinding(repositoryID, pullID, preview.ID, "owner", "/settings", "Focus", "Focus skipped", "accessibility", "major", "", []string{"Tab"}, []previews.FindingEvidence{{Kind: "screenshot", Name: "focus.png", MediaType: "image/png", Data: "cmVkYWN0ZWQ="}})
	if err != nil {
		t.Fatal(err)
	}
	validPreview := accessibilityassessments.Citation{Kind: "preview", ResourceID: preview.ID, Revision: revision, EvidenceRef: "artifact://" + finding.Evidence[0].ID}
	if !accessibilityPreviewMatchesCurrentRevision(preview, revision) || !accessibilityPreviewArtifactResolves(findingPreview(t, previewStore, repositoryID, preview.ID), validPreview.EvidenceRef) {
		t.Fatal("associated preview evidence was rejected")
	}
	if accessibilityPreviewMatchesCurrentRevision(preview, strings.Repeat("b", 40)) {
		t.Fatal("stale preview matched an advanced pull revision")
	}
	fabricated := validPreview
	fabricated.EvidenceRef = "artifact://invented"
	if accessibilityPreviewArtifactResolves(findingPreview(t, previewStore, repositoryID, preview.ID), fabricated.EvidenceRef) {
		t.Fatal("fabricated preview evidence resolved")
	}

	reportStore, _ := accessibilityreports.New(t.TempDir())
	report, err := reportStore.Create(repositoryID, "reporter", accessibilityreports.Report{Target: accessibilityreports.Target{Kind: "page", ResourceID: "settings", Revision: revision}, AccessNeeds: []string{"keyboard"}, ExpectedOutcome: "focus reaches save", Steps: []string{"press Tab"}, Evidence: []accessibilityreports.Artifact{{Kind: "screenshot", Description: "redacted", ContentRef: "artifact://report", Redacted: true}}})
	if err != nil {
		t.Fatal(err)
	}
	report, err = reportStore.AddAttempt(repositoryID, report.ID, "owner", accessibilityreports.Attempt{Boundary: "preview", Environment: accessibilityreports.Environment{Browser: "Firefox", Device: "desktop", AssistiveTechnology: "Orca"}, Outcome: "reproducible", Notes: "confirmed", Evidence: []accessibilityreports.Artifact{{Kind: "input_trace", Description: "redacted", ContentRef: "artifact://attempt", Redacted: true}}})
	if err != nil {
		t.Fatal(err)
	}
	validAttempt := accessibilityassessments.Citation{Kind: "reproduction", ResourceID: report.Attempts[0].ID, Revision: revision, EvidenceRef: "artifact://attempt"}
	if !accessibilityCitationResolves(repositoryID, revision, validAttempt, nil, nil, reportStore) {
		t.Fatal("associated reproduction evidence was rejected")
	}
}

func findingPreview(t *testing.T, store *previews.Store, repositoryID, previewID string) previews.Preview {
	t.Helper()
	preview, err := store.Find(repositoryID, previewID)
	if err != nil {
		t.Fatal(err)
	}
	return preview
}
