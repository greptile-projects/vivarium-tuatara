package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/federation"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestDuplicateFederatedRevisionRetryReturnsBeforeGitSideEffects(t *testing.T) {
	local, err := federation.New(t.TempDir(), "Local", "https://local.example", nil)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := federation.New(t.TempDir(), "Remote", "https://remote.example", nil)
	if err != nil {
		t.Fatal(err)
	}
	remoteDocument, err := remote.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = local.Upsert("https://remote.example", remoteDocument); err != nil {
		t.Fatal(err)
	}
	localDocument, err := local.Identity()
	if err != nil {
		t.Fatal(err)
	}
	const contributionID = "contribution-one"
	revision := strings.Repeat("b", 40)
	if err = local.BindContribution(contributionID, localDocument.InstanceID, remoteDocument.InstanceID); err != nil {
		t.Fatal(err)
	}
	if err = local.BindContributionTarget(contributionID, "repository-one", "pull-one", revision); err != nil {
		t.Fatal(err)
	}
	event := federation.CollaborationEvent{ID: "revision-one", ContributionID: contributionID, Sequence: 1, Kind: "revision", Actor: remoteDocument.InstanceID + ":agent:one", Revision: revision, CreatedAt: time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC), OriginInstanceID: remoteDocument.InstanceID, Verification: "verified"}
	version, key, signature, err := remote.SignPayload(collaborationEventBytes(event))
	if err != nil {
		t.Fatal(err)
	}
	event.DocumentVersion, event.SigningKeyID, event.Signature = version, key, signature
	if _, err = local.AppendCollaborationEvent(event); err != nil {
		t.Fatal(err)
	}
	identities, err := users.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newPlatformHandlerWithChecks(nil, identities, credentials, nil, nil, nil, nil, nil, nil, local))
	defer server.Close()
	body, _ := json.Marshal(signedCollaborationEvent{Event: event})
	response, err := http.Post(server.URL+"/federation/contributions/"+contributionID+"/events", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("duplicate revision retry status = %d", response.StatusCode)
	}
	var returned federation.CollaborationEvent
	if err = json.NewDecoder(response.Body).Decode(&returned); err != nil || !sameCollaborationEvent(returned, event) {
		t.Fatalf("returned event = %#v, %v", returned, err)
	}
	boundary, err := local.Contribution(contributionID)
	if err != nil {
		t.Fatal(err)
	}
	if boundary.SourceRevision != revision {
		t.Fatalf("contribution revision changed during retry: %q", boundary.SourceRevision)
	}
}

func TestSameCollaborationEventIgnoresLocalProjectionOnly(t *testing.T) {
	created := time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)
	event := federation.CollaborationEvent{ID: "revision-one", ContributionID: "contribution-one", Sequence: 1, Kind: "revision", Actor: "remote:agent:one", Revision: strings.Repeat("a", 40), CreatedAt: created, OriginInstanceID: strings.Repeat("b", 32), DocumentVersion: 2, SigningKeyID: "key", Signature: "signature"}
	retained := event
	retained.Verification, retained.Stale = "verified", true
	if !sameCollaborationEvent(retained, event) {
		t.Fatal("local verification and staleness projection changed immutable event identity")
	}
	changed := event
	changed.Revision = strings.Repeat("c", 40)
	if sameCollaborationEvent(retained, changed) {
		t.Fatal("different immutable event content was accepted as a retry")
	}
}

func TestVisibleFederationBranchesExcludeSecurityNamespace(t *testing.T) {
	mainRevision := strings.Repeat("1", 40)
	branches, revision := visibleFederationBranches([]storage.Reference{
		{Name: "refs/heads/main", Target: mainRevision},
		{Name: "refs/heads/vivarium-security/disclosures/CVE-2026-163/fix", Target: strings.Repeat("2", 40)},
	}, "main")
	if revision != mainRevision || len(branches) != 1 || branches[0].Name != "main" {
		t.Fatalf("public branches = %#v, revision = %q", branches, revision)
	}
}

func TestFetchFederatedRepositoryUsesSharedResponseLimit(t *testing.T) {
	valid := federation.SignedRepositorySnapshot{Snapshot: federation.RepositorySnapshot{Name: strings.Repeat("a", 3<<20)}}
	body, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	result, err := fetchFederatedRepository(federation.Peer{DiscoveryURL: server.URL}, "repository")
	if err != nil || result.Snapshot.Name != valid.Snapshot.Name {
		t.Fatalf("large valid response: %v", err)
	}
	oversized := bytesOf('x', maxFederatedRepositoryResponseBytes+1)
	largeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(oversized) }))
	defer largeServer.Close()
	if _, err = fetchFederatedRepository(federation.Peer{DiscoveryURL: largeServer.URL}, "repository"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response = %v", err)
	}
}

func bytesOf(value byte, size int) []byte {
	result := make([]byte, size)
	for i := range result {
		result[i] = value
	}
	return result
}

func TestBoundedFederationBundleBufferStopsAtLimit(t *testing.T) {
	buffer := &boundedBuffer{limit: 4}
	if n, err := buffer.Write([]byte("abcdef")); err == nil || n != 4 || buffer.String() != "abcd" {
		t.Fatalf("bounded write = %d, %q, %v", n, buffer.String(), err)
	}
	if n, err := buffer.Write([]byte("x")); err == nil || n != 0 || buffer.String() != "abcd" {
		t.Fatalf("write after limit = %d, %q, %v", n, buffer.String(), err)
	}
}

func TestFederationDialerRejectsNonPublicAddresses(t *testing.T) {
	for _, address := range []string{"127.0.0.1:443", "10.1.2.3:443", "169.254.1.1:443", "[::1]:443"} {
		_, err := safeFederationDialer(false)(context.Background(), "tcp", address)
		if err == nil || !strings.Contains(err.Error(), "not public") {
			t.Fatalf("dial %s = %v", address, err)
		}
	}
}
func TestPublicFederationIPClassification(t *testing.T) {
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicFederationIP(net.ParseIP(raw)) {
			t.Fatalf("public IP %s rejected", raw)
		}
	}
	for _, raw := range []string{"0.0.0.0", "127.0.0.1", "10.0.0.1", "100.64.0.1", "172.16.0.1", "192.0.2.1", "192.31.196.1", "192.52.193.1", "192.168.0.1", "192.175.48.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "169.254.1.1", "224.0.0.1", "240.0.0.1", "::1", "2001:db8::1", "fc00::1", "fe80::1"} {
		if publicFederationIP(net.ParseIP(raw)) {
			t.Fatalf("non-public IP %s accepted", raw)
		}
	}
}

func TestFederationDialerRejectsSpecialPurposeAddresses(t *testing.T) {
	for _, address := range []string{"100.64.0.1:443", "192.31.196.1:443", "192.52.193.1:443", "192.175.48.1:443", "198.18.0.1:443", "[2001:db8::1]:443"} {
		_, err := safeFederationDialer(false)(context.Background(), "tcp", address)
		if err == nil || !strings.Contains(err.Error(), "not public") {
			t.Fatalf("dial %s = %v", address, err)
		}
	}
}
