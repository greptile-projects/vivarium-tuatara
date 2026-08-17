package apicontracts

import "testing"

func validRevision() Revision {
	return Revision{VersionLabel: "1.0", Title: "Widgets API", Summary: "Manage widgets", Source: Source{CommitID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PullRequestID: "pull", DefinitionPath: "api/openapi.json", DocumentationPath: "docs/api.md"}, Operations: []Operation{{ID: "list", Method: "GET", Path: "/widgets", Summary: "List widgets", Authentication: []string{"bearer"}, ResponseSchemaIDs: []string{"widgets"}, ErrorIDs: []string{"unauthorized"}, Stability: "stable", OwnerIDs: []string{"owner"}}}, Schemas: []Schema{{ID: "widgets", Name: "Widgets", Kind: "object", Definition: `{"type":"object"}`}}, Errors: []APIError{{ID: "unauthorized", Code: "unauthorized", HTTPStatus: 401, Meaning: "Token is invalid", Recovery: "Provide a current token"}}, Authentication: []Authentication{{ID: "bearer", Mode: "bearer", Description: "OAuth bearer token"}}, Environments: []Environment{{ID: "production", Name: "Production", BaseURL: "https://api.example.test", Availability: "available"}}, Limits: Limits{Requests: 100, WindowSeconds: 60, PayloadBytes: 1048576}, OwnerIDs: []string{"owner"}, Stability: "stable", SupportPolicy: SupportPolicy{Channels: []string{"support"}, ResponseTarget: "two business days", DeprecationNoticeDays: 90, SunsetNoticeDays: 180}, Links: []Link{{Kind: "source", ID: "api/openapi.json", Label: "Definition"}, {Kind: "documentation", URL: "https://docs.example.test", Label: "Guide"}, {Kind: "data_use", URL: "https://example.test/privacy", Label: "Data use"}}, Compatibility: Compatibility{Level: "initial", Promise: "Stable operations remain backward compatible"}, Rationale: "Initial reviewed contract"}
}
func TestVersionHistoryAndDiagnostics(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	r := validRevision()
	first, e := s.Create("repo", "owner", r)
	if e != nil {
		t.Fatal(e)
	}
	if len(first.Diagnostics) != 1 || first.Diagnostics[0].Code != "unreleased_implementation" {
		t.Fatalf("diagnostics=%+v", first.Diagnostics)
	}
	r.VersionLabel = "1.1"
	r.Source.ReleaseID = "release"
	r.KnownGaps = []string{"Bulk pagination is not documented"}
	r.Compatibility = Compatibility{FromVersion: "1.0", Level: "compatible", Promise: "Existing fields remain supported"}
	second, e := s.Revise(first.ID, 1, "owner", r)
	if e != nil {
		t.Fatal(e)
	}
	if second.CurrentVersion != 2 || len(second.Revisions) != 2 || second.Revisions[0].VersionLabel != "1.0" {
		t.Fatalf("history=%+v", second)
	}
	if _, e = s.Revise(first.ID, 1, "owner", r); e != ErrConflict {
		t.Fatalf("conflict=%v", e)
	}
}
func TestRejectsDanglingDefinitions(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision()
	r.Operations[0].ResponseSchemaIDs = []string{"missing"}
	if _, e := s.Create("repo", "owner", r); e != ErrInvalid {
		t.Fatalf("error=%v", e)
	}
}

func TestRejectsOperationWithoutAccountableOwner(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision()
	r.Operations[0].OwnerIDs = nil
	if _, err := s.Create("repo", "owner", r); err != ErrInvalid {
		t.Fatalf("error=%v", err)
	}
	r = validRevision()
	r.Operations[0].OwnerIDs = []string{""}
	if _, err := s.Create("repo", "owner", r); err != ErrInvalid {
		t.Fatalf("error=%v", err)
	}
}
