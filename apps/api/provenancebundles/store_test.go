package provenancebundles

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
)

func testBundle(request string) Bundle {
	return Bundle{RequestID: request, Claim: Claim{Schema: "https://vivarium.dev/provenance-bundle/v1", RepositoryID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReleaseID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ReleaseVersion: "v1", Revision: "cccccccccccccccccccccccccccccccccccccccc", GraphID: "graph", GraphDigest: "digest", AssessmentID: "assessment", AssessmentVersion: 1, PolicyID: "policy", PolicyVersion: 1, Audience: "public", Artifacts: []Artifact{{ID: "artifact", Name: "sdk", Version: "1.0.0", SHA256: "abcd"}}, Verification: []string{"hash the artifact with SHA-256"}, PublishedBy: "owner"}}
}

func TestWithCurrentSerializesBlockingNotice(t *testing.T) {
	s, _ := New(t.TempDir())
	b, err := s.Create(testBundle("publish-locked"))
	if err != nil {
		t.Fatal(err)
	}
	entered, release, noticeDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		_ = s.WithCurrent(b.ID, func(current Bundle) error {
			if len(current.Notices) != 0 {
				t.Errorf("notices = %#v", current.Notices)
			}
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = s.AddNotice(b.ID, "owner", 0, Notice{RequestID: "blocked", Kind: "attestation_revoked", Severity: "blocking", Summary: "revoked", Evidence: "evidence"})
		close(noticeDone)
	}()
	select {
	case <-noticeDone:
		t.Fatal("notice crossed the locked publication boundary")
	default:
	}
	close(release)
	wg.Wait()
}

func TestSignedClaimSurvivesAppendOnlyTrustNotices(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(testBundle("publish-1"))
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(b.Payload)
	signature, _ := base64.RawURLEncoding.DecodeString(b.Signature)
	publicKey, _ := base64.RawURLEncoding.DecodeString(b.PublicKey)
	digest := sha256.Sum256(payload)
	if hex.EncodeToString(digest[:]) != b.PayloadSHA256 || !ed25519.Verify(publicKey, digest[:], signature) {
		t.Fatal("bundle signature does not verify")
	}
	updated, err := s.AddNotice(b.ID, "owner", 0, Notice{RequestID: "notice-1", Kind: "attestation_revoked", Severity: "blocking", Summary: "upstream signer revoked its claim", Evidence: "attestation upstream-1", RemediationID: "repair-1"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Payload != b.Payload || updated.Signature != b.Signature || len(updated.Notices) != 1 {
		t.Fatalf("original claim was rewritten: %#v", updated)
	}
	retry, err := s.AddNotice(b.ID, "owner", 1, Notice{RequestID: "notice-1", Kind: "attestation_revoked", Severity: "blocking", Summary: "upstream signer revoked its claim", Evidence: "attestation upstream-1", RemediationID: "repair-1"})
	if err != nil || len(retry.Notices) != 1 {
		t.Fatalf("notice retry = %#v, %v", retry, err)
	}
}

func TestPublicationRequestIsRetryStableAndCannotChangeClaim(t *testing.T) {
	s, _ := New(t.TempDir())
	first, err := s.Create(testBundle("stable"))
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.Create(testBundle("stable"))
	if err != nil || retry.ID != first.ID {
		t.Fatalf("retry = %#v, %v", retry, err)
	}
	changed := testBundle("stable")
	changed.Claim.Omissions = []string{"private dependency"}
	if _, err = s.Create(changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed retry error = %v", err)
	}
}
