package localization

import (
	"strings"
	"testing"
)

func TestLocaleDeliveryDefersOneLocaleWithoutBlockingOthers(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	revision := "0123456789abcdef0123456789abcdef01234567"
	p, err := s.CreateDeliveryPolicy("repo", "owner", DeliveryPolicy{Branch: "main", LocalePlanID: "plan", LocalePlanVersion: 2, Locales: []string{"fr-CA", "ja-JP"}, Audiences: []string{"customers"}, RiskClasses: []string{"checkout"}, RequiredChecks: []string{"locale-format"}, MinimumReviews: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetLocaleDisposition("repo", "owner", LocaleDisposition{PolicyID: p.ID, Revision: revision, Locale: "fr-CA", State: "deferred", Reason: "regional review follows in the next train"}); err != nil {
		t.Fatal(err)
	}
	r, err := s.EvaluateDelivery("repo", "", "", revision, "main", []string{"customers"}, []string{"checkout"}, map[string]string{"locale-format": "passed"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Ready || r.Locales["fr-CA"] != "deferred" || r.Locales["ja-JP"] != "required" {
		t.Fatalf("readiness = %#v", r)
	}
}

func TestLocaleDeliveryCountsCurrentPreviewApproval(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	revision := "1111111111111111111111111111111111111111"
	v, err := s.Extract("repo", "pull", revision, "owner", ExtractionMap{ID: "messages", Version: 1, Name: "Messages", Include: []string{"messages.json"}, Formats: []string{"json"}}, []string{"fr-CA"}, []Unit{{Key: "welcome", Message: "Welcome", Context: "Home", Locations: []Location{{Path: "messages.json", Line: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Propose("repo", "pull", revision, v.Extractions[0].Units[0].ID, "fr-CA", "Bienvenue", "regional wording", "translator")
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Verify("repo", "pull", revision, v.WorkspaceVersion, "publish_candidate", "owner", "translator", 1, map[string]any{"locale": "fr-CA", "preview_id": "preview", "preview_url": "/preview", "locale_plan_id": "plan", "locale_plan_version": float64(1), "routes": []map[string]any{{"journey_id": "home", "route": "/fr-CA", "interface_hash": strings.Repeat("a", 64)}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Verify("repo", "pull", revision, v.WorkspaceVersion, "review", "regional", "regional_reviewer", 1, map[string]any{"candidate_id": v.VerificationCandidates[0].ID, "locale": "fr-CA", "route": "/fr-CA", "unit_ids": []string{v.Extractions[0].Units[0].ID}, "kind": "approve", "reason": "Natural in the current regional preview"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateDeliveryPolicy("repo", "owner", DeliveryPolicy{Branch: "main", LocalePlanID: "plan", LocalePlanVersion: 1, Locales: []string{"fr-CA"}, MinimumReviews: 1}); err != nil {
		t.Fatal(err)
	}
	readiness, err := s.EvaluateDelivery("repo", "pull", "", revision, "main", nil, nil, nil)
	if err != nil || !readiness.Ready || readiness.Requirements[0].Status != "passed" {
		t.Fatalf("readiness = %#v, %v", readiness, err)
	}
}

func TestScopedLocaleDeliveryDoesNotSelectAbsentContext(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := "0123456789abcdef0123456789abcdef01234567"
	_, err := s.CreateDeliveryPolicy("repo", "owner", DeliveryPolicy{Branch: "main", LocalePlanID: "plan", LocalePlanVersion: 1, Locales: []string{"fr-CA"}, Audiences: []string{"customers"}, RiskClasses: []string{"checkout"}, RequiredChecks: []string{"locale-format"}, MinimumReviews: 1})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.EvaluateDelivery("repo", "pull", "", revision, "main", nil, nil, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Ready || len(r.Requirements) != 0 {
		t.Fatalf("context-free readiness selected scoped policy: %#v", r)
	}
	r, err = s.EvaluateDelivery("repo", "pull", "", revision, "main", []string{"customers"}, []string{"checkout"}, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Ready || len(r.Requirements) != 2 {
		t.Fatalf("matching readiness did not select scoped policy: %#v", r)
	}
}

func TestPublishedLocaleFindingRetainsProvenanceAndRepair(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := "0123456789abcdef0123456789abcdef01234567"
	p, err := s.Publish("repo", "maintainer", Publication{Kind: "documentation", ResourceID: "guide", Version: "v2", Revision: revision, Locale: "ar-EG", LocalePlanID: "plan", LocalePlanVersion: 3, SourceLocale: "en", FallbackLocale: "en", FallbackState: "partial", URL: "https://example.test/ar/guide", Status: "published"})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.ReportPublished("repo", "reader", PublishedFinding{PublicationID: p.ID, Locale: "ar-EG", Category: "broken_formatting", Route: "/ar/guide", Expected: "code remains left-to-right", Observed: "code punctuation is reversed"})
	if err != nil {
		t.Fatal(err)
	}
	f, err = s.DecidePublishedFinding("repo", f.ID, "maintainer", "validated", "reproduced on the published version", &LocaleRepair{OwnerType: "agent", OwnerID: "locale-agent", WorkURL: "/repositories/repo/proposals/repair", AcceptanceCriteria: "preserve bidirectional code formatting"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Repair == nil || f.Repair.OwnerType != "agent" || f.Status != "validated" || f.PublicationID != p.ID {
		t.Fatalf("finding = %#v", f)
	}
}
