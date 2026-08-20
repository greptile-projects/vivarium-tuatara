package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestAssuranceEvidenceBindsExactControlAndProjectsAudience(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	programs, _ := assuranceprograms.New(t.TempDir())
	evidence, _ := assuranceevidence.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, programs, evidence))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "evidence-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"controlled"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	audienceMember := createTestAccount(t, server.URL, "evidence-audience-member")
	if _, err := catalog.AddCollaborator(owner.User.ID, repo.ID, audienceMember.User.ID); err != nil {
		t.Fatal(err)
	}
	revision := completeAssuranceRevision(owner.User.ID, repo.ID)
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-programs", string(payload), owner.Credential.Token, http.StatusCreated)
	var program assuranceprograms.Program
	_ = json.NewDecoder(response.Body).Decode(&program)
	response.Body.Close()
	now := time.Now().UTC()
	definition := assuranceevidence.Definition{ProgramID: program.ID, ProgramVersion: 1, ControlID: "control", Title: "Storage operation", PeriodStartsAt: now.Add(-time.Hour), PeriodEndsAt: now.Add(time.Hour), Schedule: "daily", Audience: []string{audienceMember.User.ID}, Queries: []assuranceevidence.Query{{ID: "check", Kind: "check", ResourceID: "nonexistent", Required: true, MaxAgeHours: 24}}}
	body, _ := json.Marshal(definition)
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-evidence/definitions", string(body), owner.Credential.Token, http.StatusCreated)
	var created assuranceevidence.Definition
	_ = json.NewDecoder(response.Body).Decode(&created)
	response.Body.Close()
	if created.ProgramVersion != 1 || created.OwnerID != owner.User.ID {
		t.Fatalf("definition = %#v", created)
	}
	fabricated, _ := json.Marshal(map[string]any{"sources": []assuranceevidence.Source{{QueryID: "check", Kind: "check", ResourceID: "nonexistent", Revision: "invented", OccurredAt: now, Provenance: "caller claim", Accessible: true}}, "query_ids": []string{"check"}})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-evidence/definitions/"+created.ID+"/packages", string(fabricated), owner.Credential.Token, http.StatusBadRequest).Body.Close()
	if _, err := evidence.CreatePackage(created, owner.User.ID, []assuranceevidence.Source{{QueryID: "check", Kind: "check", ResourceID: "nonexistent", OccurredAt: now, Provenance: "trusted test record", Summary: "passed", Accessible: true}}); err != nil {
		t.Fatal(err)
	}
	visible := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/assurance-evidence", "", audienceMember.Credential.Token, http.StatusOK)
	var beforeRemoval struct {
		Definitions []assuranceevidence.Definition `json:"definitions"`
		Packages    []assuranceevidence.Package    `json:"packages"`
	}
	_ = json.NewDecoder(visible.Body).Decode(&beforeRemoval)
	visible.Body.Close()
	if len(beforeRemoval.Definitions) != 1 || len(beforeRemoval.Packages) != 1 {
		t.Fatalf("current audience projection = %#v", beforeRemoval)
	}
	if err := catalog.RemoveCollaborator(owner.User.ID, repo.ID, audienceMember.User.ID); err != nil {
		t.Fatal(err)
	}
	revoked := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/assurance-evidence", "", audienceMember.Credential.Token, http.StatusOK)
	var afterRemoval struct {
		Definitions []assuranceevidence.Definition `json:"definitions"`
		Packages    []assuranceevidence.Package    `json:"packages"`
	}
	_ = json.NewDecoder(revoked.Body).Decode(&afterRemoval)
	revoked.Body.Close()
	if len(afterRemoval.Definitions) != 0 || len(afterRemoval.Packages) != 0 {
		t.Fatalf("removed audience member retained evidence: %#v", afterRemoval)
	}
	outsider := createTestAccount(t, server.URL, "evidence-outsider")
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/assurance-evidence", "", outsider.Credential.Token, http.StatusOK)
	var projection struct {
		Definitions []assuranceevidence.Definition `json:"definitions"`
		Packages    []assuranceevidence.Package    `json:"packages"`
	}
	_ = json.NewDecoder(listed.Body).Decode(&projection)
	listed.Body.Close()
	if len(projection.Definitions) != 0 || len(projection.Packages) != 0 {
		t.Fatalf("private evidence leaked: %#v", projection)
	}
}

func TestAssuranceEvidenceSourceResolverDerivesReleaseRecord(t *testing.T) {
	store, _ := releases.New(t.TempDir())
	repo, actor, commit := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 40)
	release, err := store.Create(releases.Candidate{RepositoryID: repo, Version: "v1", Notes: "reviewed release", CommitID: commit, TargetBranch: "main", CreatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	source, err := (assuranceEvidenceSources{releases: store}).resolveOne(repo, assuranceevidence.Query{ID: "release", Kind: "release", ResourceID: release.ID, Revision: commit})
	if err != nil {
		t.Fatal(err)
	}
	if source.ResourceID != release.ID || source.Revision != commit || source.OccurredAt.IsZero() || source.Provenance != "repository release ledger" {
		t.Fatalf("source = %#v", source)
	}
	_, err = (assuranceEvidenceSources{releases: store}).resolveOne(repo, assuranceevidence.Query{ID: "release", Kind: "release", ResourceID: release.ID, Revision: strings.Repeat("d", 40)})
	if err == nil {
		t.Fatal("mismatched revision resolved")
	}
}
