package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/threatmodels"
)

func TestThreatModelContributorRejectsGeneralAgentAndRequiresTaskCredential(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	credentials, _ := auth.New(t.TempDir())
	ownerID, agentID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	repository, _ := catalog.Create(ownerID, "threat-auth")
	general, err := credentials.IssueOrganizationAgent(ownerID, "general", strings.Repeat("3", 32), strings.Repeat("4", 32), agentID, repository.ID, []string{"repositories:read", "repositories:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	task, err := credentials.IssueTaskAgentBound(ownerID, "task", agentID, []string{"git:write"}, time.Hour, repository.ID, "refs/heads/agent/threat")
	if err != nil {
		t.Fatal(err)
	}
	human, err := credentials.Issue(ownerID, auth.API, "human", []string{"repositories:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	request := func(token string) (*httptest.ResponseRecorder, auth.Credential, bool) {
		r := httptest.NewRequest(http.MethodPost, "/repositories/"+repository.ID+"/threat-models/model/events", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		actor, ok := authorizeThreatModelContributor(w, r, catalog, credentials, repository.ID)
		return w, actor, ok
	}
	if w, _, ok := request(general.Token); ok || w.Code != http.StatusForbidden {
		t.Fatalf("general agent=%d ok=%v", w.Code, ok)
	}
	if w, actor, ok := request(task.Token); !ok || w.Code != http.StatusOK || actor.AgentID != agentID || actor.Kind != auth.Git {
		t.Fatalf("task agent=%d actor=%#v ok=%v", w.Code, actor, ok)
	}
	if w, actor, ok := request(human.Token); !ok || w.Code != http.StatusOK || actor.UserID != ownerID || actor.AgentID != "" {
		t.Fatalf("human=%d actor=%#v ok=%v", w.Code, actor, ok)
	}
}

func TestThreatModelSourceFingerprintChangesWithoutVersionMovement(t *testing.T) {
	designs, _ := designproposals.New(t.TempDir())
	revision := designproposals.Revision{Title: "Safer login", UserGoal: "Sign in safely", Source: designproposals.Source{Kind: "issue", ResourceID: "issue", Summary: "Login risk"}, Journeys: []designproposals.Journey{{Name: "Login", Actor: "User", Goal: "Sign in", Steps: []string{"Submit"}}}, States: []designproposals.State{{Name: "Ready", Description: "Form", Content: "Sign in"}}, Content: []string{"Sign in"}, Constraints: []string{"Secure"}, Alternatives: []string{"Passkey"}, SuccessMeasures: []string{"No takeover"}, AffectedComponents: []string{"login"}, Artifacts: []designproposals.Artifact{{ID: "wire", Kind: "wireframe", Title: "Login", Description: "Form", Content: "form", Audience: []string{"owner"}}}, Uncertainty: []string{"Recovery"}}
	design, err := designs.Create("repo", "owner", []string{"owner"}, revision)
	if err != nil {
		t.Fatal(err)
	}
	sources := threatModelSources{designs: designs}
	modelRevision := threatmodels.Revision{Source: threatmodels.Source{Kind: "design_proposal", ResourceID: design.ID, Revision: "1"}, Dependencies: []threatmodels.Dependency{{ID: "identity"}}}
	first, err := sources.current("repo", modelRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = designs.Comment("repo", design.ID, "owner", designproposals.Comment{Revision: 1, Kind: "dissent", Body: "Changed boundary assumption"}); err != nil {
		t.Fatal(err)
	}
	second, err := sources.current("repo", modelRevision)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != second.Revision || first.ArchitectureDigest == second.ArchitectureDigest || first.TrustBoundaryDigest == second.TrustBoundaryDigest || first.DependencyRevisions["identity"] == second.DependencyRevisions["identity"] {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
}
