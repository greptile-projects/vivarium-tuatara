package main

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func TestLearningAssessmentEvidenceIsLearnerAndWorkProductBound(t *testing.T) {
	digest := sha256.Sum256([]byte("go test ./..."))
	w := workspaces.Workspace{CreatorID: "learner", LearningContext: &workspaces.LearningContext{Checkpoints: []workspaces.LearningCheckpoint{{ID: "cp"}}}, Commands: []workspaces.CommandOutcome{{ID: "own", ActorID: "learner", CommandSHA256: hex.EncodeToString(digest[:])}, {ID: "mentor", ActorID: "mentor"}}, Changes: []workspaces.Change{{Path: "main.go", SHA256: "changed", Size: 7, ActorID: "learner"}}}
	if !evidenceExists(w, learningassessments.Evidence{CheckpointIDs: []string{"cp"}, CommandOutcomeIDs: []string{"own"}}) {
		t.Fatal("learner evidence rejected")
	}
	if evidenceExists(w, learningassessments.Evidence{CommandOutcomeIDs: []string{"mentor"}}) {
		t.Fatal("mentor command accepted as learner evidence")
	}
	if got := learningWorkProductDigest(w, "learner"); got == "" {
		t.Fatal("learner work product was not derived")
	}
	if got := learningWorkProductDigest(w, "different-learner"); got != "" {
		t.Fatalf("shared exercise input became work product: %q", got)
	}
}
