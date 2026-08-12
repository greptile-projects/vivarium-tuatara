package federation

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
)

func TestIdentityIsStableSignedAndTamperEvident(t *testing.T) {
	s, err := New(t.TempDir(), "North", "https://north.example", []string{"ops@north.example"})
	if err != nil {
		t.Fatal(err)
	}
	one, err := s.Identity()
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if one.InstanceID != two.InstanceID || one.Signature != two.Signature {
		t.Fatal("identity was not stable")
	}
	if err := Verify(one); err != nil {
		t.Fatalf("verify: %v", err)
	}
	one.Name = "Impostor"
	if err := Verify(one); err == nil {
		t.Fatal("tampered document verified")
	}
	rotated, err := s.Rotate()
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Version != two.Version+1 || rotated.SigningKeyID == two.SigningKeyID {
		t.Fatal("rotation did not publish a new version and key")
	}
	if rotated.Keys[0].RetiredAt == nil {
		t.Fatal("predecessor key was not retained as retired")
	}
	if err := Verify(rotated); err != nil {
		t.Fatalf("rotated verify: %v", err)
	}
}

func TestPeerChangesRequireTrustAndRevocationIsSticky(t *testing.T) {
	local, _ := New(t.TempDir(), "Local", "https://local.example", nil)
	remote, _ := New(t.TempDir(), "Remote", "https://remote.example", nil)
	doc, _ := remote.Identity()
	peer, err := local.Upsert("https://remote.example", doc)
	if err != nil {
		t.Fatal(err)
	}
	if peer.Status != "trusted" {
		t.Fatalf("status %q", peer.Status)
	}
	identityPath := remote.root + "/identity.json"
	remote.mu.Lock()
	var p persistedIdentity
	readJSONForTest(t, identityPath, &p)
	privateRaw, _ := base64.RawURLEncoding.DecodeString(p.PrivateKey)
	p.Document.Version++
	p.Document.Signature = sign(p.Document, ed25519.PrivateKey(privateRaw))
	if err := writeJSON(identityPath, p); err != nil {
		t.Fatal(err)
	}
	remote.mu.Unlock()
	changed, _ := remote.Identity()
	peer, err = local.Upsert("https://remote.example", changed)
	if err != nil {
		t.Fatal(err)
	}
	if peer.Status != "changed" {
		t.Fatalf("status %q", peer.Status)
	}
	peer, err = local.Decide(peer.InstanceID, peer.TrustVersion, "trust")
	if err != nil || peer.Status != "trusted" {
		t.Fatalf("trust: %#v %v", peer, err)
	}
	peer, err = local.Decide(peer.InstanceID, peer.TrustVersion, "revoke")
	if err != nil || peer.Status != "revoked" {
		t.Fatalf("revoke: %#v %v", peer, err)
	}
	if _, err = local.Upsert("https://remote.example", changed); err != ErrConflict {
		t.Fatalf("revoked peer update = %v", err)
	}
}

func readJSONForTest(t *testing.T, path string, out any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, out); err != nil {
		t.Fatal(err)
	}
}
