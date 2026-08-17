package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func TestSupportVerificationSanitizationAndCommandBinding(t *testing.T) {
	unsafe := []struct{ name, value string }{{"bearer", "Authorization: Bearer abcdefghijklmnopqrstuvwxyz"}, {"key", "api_key=abcdefghijklmnop"}, {"private key", "-----BEGIN PRIVATE KEY-----"}}
	for _, tc := range unsafe {
		if !reusableSecret.MatchString(tc.value) {
			t.Errorf("%s not detected", tc.name)
		}
	}
	thread := supportthreads.Thread{Attachments: []supportthreads.Attachment{{ID: "safe", Kind: "log", Name: "safe.log", Data: base64.StdEncoding.EncodeToString([]byte("bounded output"))}, {ID: "secret", Kind: "log", Name: "bad.log", Data: base64.StdEncoding.EncodeToString([]byte("token=abcdefghijklmnop"))}}}
	if _, ok := selectInputs(thread, []string{"safe"}); !ok {
		t.Fatal("safe attachment rejected")
	}
	if _, ok := selectInputs(thread, []string{"secret"}); ok {
		t.Fatal("credential-shaped attachment accepted")
	}
	now := time.Now().UTC()
	command := "go test ./..."
	digest := sha256Text(command)
	ws := workspaces.Workspace{Commands: []workspaces.CommandOutcome{{ID: "out", CommandSHA256: digest, Output: "ok", StartedAt: now, CompletedAt: now.Add(time.Second)}}}
	commands, _, ok := selectCommands(ws, []struct {
		Command   string `json:"command"`
		OutcomeID string `json:"outcome_id"`
	}{{command, "out"}})
	if !ok || len(commands) != 1 {
		t.Fatal("exact command outcome was not selected")
	}
	if _, _, ok = selectCommands(ws, []struct {
		Command   string `json:"command"`
		OutcomeID string `json:"outcome_id"`
	}{{"go test ./other", "out"}}); ok {
		t.Fatal("mismatched command was accepted")
	}
}

func TestSupportVerificationWorkspaceProvenance(t *testing.T) {
	matching := workspaces.Workspace{Source: workspaces.Source{Kind: "support_verification", SupportThreadID: "thread", AnswerID: "answer", AnswerRevisionID: "revision"}}
	if !supportWorkspaceMatches(matching, "thread", "answer", "revision") {
		t.Fatal("exact support workspace provenance was rejected")
	}
	for name, workspace := range map[string]workspaces.Workspace{
		"repository source": {Source: workspaces.Source{Kind: "repository"}},
		"other thread":      {Source: workspaces.Source{Kind: "support_verification", SupportThreadID: "other", AnswerID: "answer", AnswerRevisionID: "revision"}},
		"other answer":      {Source: workspaces.Source{Kind: "support_verification", SupportThreadID: "thread", AnswerID: "other", AnswerRevisionID: "revision"}},
		"other revision":    {Source: workspaces.Source{Kind: "support_verification", SupportThreadID: "thread", AnswerID: "answer", AnswerRevisionID: "other"}},
	} {
		if supportWorkspaceMatches(workspace, "thread", "answer", "revision") {
			t.Errorf("%s provenance was accepted", name)
		}
	}
}

func sha256Text(v string) string { d := sha256.Sum256([]byte(v)); return hex.EncodeToString(d[:]) }
