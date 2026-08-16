package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoveryoperations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestRecoveryIncidentEvidenceIsRepositoryAndResourceBound(t *testing.T) {
	foreign := incidents.Evidence{Kind: "deployment", RepositoryID: "other-repository", ResourceID: "production", Label: "healthy"}
	localWrongResource := incidents.Evidence{Kind: "deployment", RepositoryID: "repository", ResourceID: "unrelated", Label: "healthy"}
	local := incidents.Evidence{Kind: "deployment", RepositoryID: "repository", ResourceID: "production", Label: "healthy"}
	incident := incidents.Incident{Timeline: []incidents.Entry{{Evidence: []incidents.Evidence{foreign, localWrongResource, local}}}}
	reference := func(evidence incidents.Evidence) recoveryoperations.EvidenceReference {
		value, _ := json.Marshal(evidence)
		digest := sha256.Sum256(value)
		return recoveryoperations.EvidenceReference{Kind: "incident_evidence", ResourceID: evidence.ResourceID, SHA256: hex.EncodeToString(digest[:])}
	}
	allowed := map[string]bool{"production": true}
	if recoveryIncidentEvidenceMatches(incident, reference(foreign), "repository", allowed) {
		t.Fatal("foreign repository evidence authorized recovery")
	}
	if recoveryIncidentEvidenceMatches(incident, reference(localWrongResource), "repository", allowed) {
		t.Fatal("unrelated resource evidence authorized recovery")
	}
	if !recoveryIncidentEvidenceMatches(incident, reference(local), "repository", allowed) {
		t.Fatal("exact local recovery evidence was rejected")
	}
}

func TestRecoveryMutationRequiresExactRepositoryAccess(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	repos, _ := repositories.New(t.TempDir(), git)
	owner := "0123456789abcdef0123456789abcdef"
	actor := "abcdefabcdefabcdefabcdefabcdefab"
	recoveryRepo, _ := repos.Create(owner, "recovery-scope")
	otherRepo, _ := repos.Create(owner, "other-incident-scope")
	if _, err := repos.AddCollaborator(owner, otherRepo.ID, actor); err != nil {
		t.Fatal(err)
	}
	operations, _ := recoveryoperations.New(t.TempDir())
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	operation, err := operations.Create("incident", recoveryRepo.ID, owner, recoveryoperations.RecoveryPoint{PlanID: "plan", PlanVersion: 1, CaptureID: "capture", SourceRevision: "revision", CapturedAt: time.Now().UTC(), ManifestSHA256: digest}, recoveryoperations.Revision{Objective: "restore", RequiredApprovals: 1, ApproverIDs: []string{actor}, RollbackOption: "isolate", Steps: []recoveryoperations.Step{{ID: "restore", Name: "restore", Kind: "restore", ResourceID: "state", AssigneeType: "human", AssigneeID: actor, Status: "pending", ValidationCriteria: []recoveryoperations.ValidationCriterion{{ID: "manifest", Description: "manifest", EvidenceKind: "protection_capture"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	if recoveryOperationActorAllowed(w, repos, operations, auth.Credential{UserID: actor}, operation.ID) || w.Code != 404 {
		t.Fatalf("other-scope collaborator authorized: code=%d", w.Code)
	}
	if _, err = repos.AddCollaborator(owner, recoveryRepo.ID, actor); err != nil {
		t.Fatal(err)
	}
	if !recoveryOperationActorAllowed(httptest.NewRecorder(), repos, operations, auth.Credential{UserID: actor}, operation.ID) {
		t.Fatal("exact-scope collaborator rejected")
	}
	if recoveryOperationControllerAllowed(httptest.NewRecorder(), repos, operations, auth.Credential{UserID: actor}, incidents.Incident{}, operation.ID) {
		t.Fatal("unassigned collaborator gained recovery-wide control")
	}
	assignedIncident := incidents.Incident{Roles: []incidents.Role{{Name: "recovery commander", UserID: actor}}}
	if !recoveryOperationControllerAllowed(httptest.NewRecorder(), repos, operations, auth.Credential{UserID: actor}, assignedIncident, operation.ID) {
		t.Fatal("assigned human incident controller rejected")
	}
	agent := auth.Credential{UserID: owner, AgentID: "recovery-agent", RepositoryID: recoveryRepo.ID, AccessGrantID: "grant"}
	if recoveryOperationControllerAllowed(httptest.NewRecorder(), repos, operations, agent, assignedIncident, operation.ID) {
		t.Fatal("repository-bound agent gained recovery-wide control")
	}
	if err = repos.RemoveCollaborator(owner, recoveryRepo.ID, actor); err != nil {
		t.Fatal(err)
	}
	if recoveryOperationActorAllowed(httptest.NewRecorder(), repos, operations, auth.Credential{UserID: actor}, operation.ID) {
		t.Fatal("revoked responder retained recovery authority")
	}
}
