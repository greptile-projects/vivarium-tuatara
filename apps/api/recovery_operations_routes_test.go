package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoveryoperations"
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
