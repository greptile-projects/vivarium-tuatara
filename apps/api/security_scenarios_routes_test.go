package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/threatmodels"
)

func TestSecurityAttemptCommandsMustMatchReviewedCommand(t *testing.T) {
	reviewed := "go test ./security/expected"
	if securityCommandDigestMatches(securityDigest("go test ./unrelated"), reviewed) {
		t.Fatal("unrelated workspace outcome matched reviewed security command")
	}
	if !securityCommandDigestMatches(securityDigest(reviewed), reviewed) {
		t.Fatal("exact workspace outcome did not match reviewed security command")
	}
	if securityCommandMatches("echo preview-build-only", reviewed) {
		t.Fatal("ordinary preview build matched reviewed security command")
	}
	if !securityCommandMatches(reviewed, reviewed) {
		t.Fatal("exact preview command did not match reviewed security command")
	}
}

func TestSecurityScenarioCanResolveRetainedThreatModelRevision(t *testing.T) {
	model := threatmodels.Model{CurrentVersion: 2, Revisions: []threatmodels.Revision{{Version: 1}, {Version: 2}}}
	revision, ok := retainedThreatModelRevision(model, 1)
	if !ok || revision.Version != 1 {
		t.Fatalf("retained revision = %+v, %v", revision, ok)
	}
	if _, ok = retainedThreatModelRevision(model, 3); ok {
		t.Fatal("missing future revision resolved")
	}
}
