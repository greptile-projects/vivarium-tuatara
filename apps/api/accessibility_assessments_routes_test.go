package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityreports"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
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
