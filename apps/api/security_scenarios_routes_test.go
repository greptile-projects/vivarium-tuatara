package main

import "testing"

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
