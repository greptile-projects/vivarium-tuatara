package localization

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractionRetainsHistoryAndSupersedesOnlyChangedUnits(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := ExtractionMap{ID: "web-messages", Version: 1, Name: "Web messages", Include: []string{"src/**"}, Formats: []string{"typescript"}}
	unit := func(key, message string) Unit {
		return Unit{Key: key, Message: message, Context: "Repository navigation", Screenshot: "preview://nav", Variables: []Variable{{Name: "name", Example: "Tuatara"}}, PluralRule: "cardinal", Locations: []Location{{Path: "src/nav.tsx", Line: 12, Component: "Nav"}}}
	}
	v, err := s.Extract("repo", "pull", "1111111111111111111111111111111111111111", "owner", m, []string{"fr", "de"}, []Unit{unit("welcome", "Welcome {name}"), unit("settings", "Settings")})
	if err != nil {
		t.Fatal(err)
	}
	welcomeID := v.Extractions[0].Units[0].ID
	settingsID := v.Extractions[0].Units[1].ID
	v, err = s.Propose("repo", "pull", v.CurrentRevision, welcomeID, "fr", "Bienvenue {name}", "Reviewed in navigation", "translator")
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Extract("repo", "pull", "2222222222222222222222222222222222222222", "owner", m, []string{"fr", "de"}, []Unit{unit("welcome", "Hello {name}"), unit("settings", "Settings"), unit("help", "Help")})
	if err != nil {
		t.Fatal(err)
	}
	latest := v.Extractions[1]
	if latest.Units[0].ID != welcomeID || latest.Units[0].Change != "changed" {
		t.Fatalf("stable changed unit = %#v", latest.Units[0])
	}
	if latest.Units[1].ID != settingsID || latest.Units[1].Change != "reused" {
		t.Fatalf("reused unit = %#v", latest.Units[1])
	}
	if v.Translations[0].Status != "superseded" {
		t.Fatalf("translation status = %q", v.Translations[0].Status)
	}
	if v.Counts["fr"]["changed"] != 1 || v.Counts["fr"]["reused"] != 1 || v.Counts["fr"]["added"] != 1 || v.Counts["fr"]["untranslated"] != 3 {
		t.Fatalf("counts = %#v", v.Counts["fr"])
	}
}

func TestTranslationRequiresCurrentKnownUnitAndLocale(t *testing.T) {
	s, _ := New(t.TempDir())
	m := ExtractionMap{ID: "docs", Version: 1, Name: "Docs", Include: []string{"docs/**"}, Formats: []string{"markdown"}}
	v, err := s.Extract("repo", "pull", "1111111111111111111111111111111111111111", "owner", m, []string{"es"}, []Unit{{Key: "intro", Message: "Start", Context: "Introduction", Locations: []Location{{Path: "docs/start.md", Line: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Propose("repo", "pull", "2222222222222222222222222222222222222222", v.Extractions[0].Units[0].ID, "es", "Inicio", "", "translator"); err != ErrInvalid {
		t.Fatalf("stale revision error = %v", err)
	}
	if _, err = s.Propose("repo", "pull", v.CurrentRevision, v.Extractions[0].Units[0].ID, "xx", "Start", "", "translator"); err != ErrInvalid {
		t.Fatalf("unknown locale error = %v", err)
	}
}

func TestExtractionPreservesUnreadableReview(t *testing.T) {
	root := t.TempDir()
	s, _ := New(root)
	if err := os.MkdirAll(filepath.Join(root, "repo"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "repo", "pull.json")
	corrupt := []byte("CORRUPT-REVIEW-MUST-NOT-BE-REPLACED{")
	if err := os.WriteFile(path, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	m := ExtractionMap{ID: "docs", Version: 1, Name: "Docs", Include: []string{"docs/**"}, Formats: []string{"markdown"}}
	_, err := s.Extract("repo", "pull", "1111111111111111111111111111111111111111", "owner", m, []string{"es"}, []Unit{{Key: "intro", Message: "Start", Context: "Introduction", Locations: []Location{{Path: "docs/start.md", Line: 1}}}})
	if err == nil {
		t.Fatal("Extract accepted corrupt persisted review")
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("corrupt review was replaced: %q", after)
	}
}

func TestGetDoesNotProjectOldRevisionAsActionable(t *testing.T) {
	s, _ := New(t.TempDir())
	m := ExtractionMap{ID: "docs", Version: 1, Name: "Docs", Include: []string{"docs/**"}, Formats: []string{"markdown"}}
	v, err := s.Extract("repo", "pull", "1111111111111111111111111111111111111111", "owner", m, []string{"es"}, []Unit{{Key: "intro", Message: "Start", Context: "Introduction", Locations: []Location{{Path: "docs/start.md", Line: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Propose("repo", "pull", v.CurrentRevision, v.Extractions[0].Units[0].ID, "es", "Inicio", "", "translator")
	if err != nil {
		t.Fatal(err)
	}

	projected, err := s.Get("repo", "pull", "2222222222222222222222222222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Counts) != 0 {
		t.Fatalf("stale extraction produced current counts: %#v", projected.Counts)
	}
	if projected.Translations[0].Status != "stale" {
		t.Fatalf("old translation status = %q", projected.Translations[0].Status)
	}
	if projected.Extractions[0].Units[0].LocaleStatus["es"] != "stale" {
		t.Fatalf("old unit status = %#v", projected.Extractions[0].Units[0].LocaleStatus)
	}

	persisted, err := s.Get("repo", "pull", "1111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Translations[0].Status != "proposed" {
		t.Fatalf("read projection mutated history: %q", persisted.Translations[0].Status)
	}
}

func TestProposalRejectsPersistedReviewWithoutExtractions(t *testing.T) {
	root := t.TempDir()
	s, _ := New(root)
	if err := os.MkdirAll(filepath.Join(root, "repo"), 0700); err != nil {
		t.Fatal(err)
	}
	review := Review{RepositoryID: "repo", PullID: "pull", CurrentRevision: "1111111111111111111111111111111111111111", Extractions: []Extraction{}, Translations: []Translation{}}
	encoded, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "repo", "pull.json")
	if err = os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Propose("repo", "pull", review.CurrentRevision, "unit", "es", "Inicio", "", "translator"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty extraction proposal error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(encoded) {
		t.Fatal("invalid review was rewritten")
	}
}

func TestCollaborativeWorkspaceKeepsAgentEvidenceAndHumanAuthorityVisible(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := "1111111111111111111111111111111111111111"
	v, err := s.Extract("repo", "pull", revision, "owner", ExtractionMap{ID: "web", Version: 1, Name: "Web", Include: []string{"src/**"}, Formats: []string{"typescript"}}, []string{"fr"}, []Unit{{Key: "welcome", Message: "Welcome", Context: "Home hero", Locations: []Location{{Path: "src/home.tsx", Line: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	unit := v.Extractions[0].Units[0].ID
	v, err = s.Mutate("repo", "pull", revision, v.WorkspaceVersion, "claim", "user", "translator", map[string]any{"unit_id": unit, "locale": "fr", "assignee_id": "translator"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Mutate("repo", "pull", revision, v.WorkspaceVersion, "comment", "user", "owner", map[string]any{"unit_id": unit, "locale": "fr", "body": "Keep the welcoming product tone."})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Mutate("repo", "pull", revision, v.WorkspaceVersion, "request_suggestion", "user", "translator", map[string]any{"unit_id": unit, "locale": "fr", "agent_id": "linguist-agent", "product_context": "Home hero for new contributors", "locale_plan_id": "plan", "locale_plan_version": float64(3), "protected": false, "embargoed": false})
	if err != nil {
		t.Fatal(err)
	}
	request := v.SuggestionRequests[0]
	v, err = s.Mutate("repo", "pull", revision, v.WorkspaceVersion, "suggest", "agent", "linguist-agent", map[string]any{"unit_id": unit, "locale": "fr", "request_id": request.ID, "text": "Bienvenue", "rationale": "Matches the approved welcoming term.", "uncertainty": "low", "evidence": []map[string]any{{"kind": "terminology", "reference": "plan:3:welcome"}, {"kind": "prior_translation", "reference": "translation:42"}, {"kind": "source_context", "reference": "unit:welcome"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Suggestions[0]; got.AgentID != "linguist-agent" || got.SourceRevision != revision || got.SourceHash == "" || len(got.Evidence) != 3 {
		t.Fatalf("suggestion provenance = %#v", got)
	}
	v, err = s.Mutate("repo", "pull", revision, v.WorkspaceVersion, "decide", "user", "reviewer", map[string]any{"unit_id": unit, "locale": "fr", "suggestion_id": v.Suggestions[0].ID, "kind": "approve", "reason": "Reviewed against current terminology"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Suggestions[0].Status != "approved" || v.Decisions[0].ActorID != "reviewer" {
		t.Fatalf("human decision = %#v / %#v", v.Suggestions[0], v.Decisions)
	}
}

func TestCollaborativeWorkspaceRejectsConcurrencyAndSensitiveSuggestionRequests(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := "1111111111111111111111111111111111111111"
	v, _ := s.Extract("repo", "pull", revision, "owner", ExtractionMap{ID: "docs", Version: 1, Name: "Docs", Include: []string{"docs/**"}, Formats: []string{"markdown"}}, []string{"es"}, []Unit{{Key: "intro", Message: "Start", Context: "Intro", Locations: []Location{{Path: "docs/a.md", Line: 1}}}})
	payload := map[string]any{"unit_id": v.Extractions[0].Units[0].ID, "locale": "es", "agent_id": "agent", "product_context": "Intro", "locale_plan_id": "plan", "locale_plan_version": float64(1), "protected": true, "embargoed": false}
	if _, err := s.Mutate("repo", "pull", revision, v.WorkspaceVersion, "request_suggestion", "user", "translator", payload); !errors.Is(err, ErrInvalid) {
		t.Fatalf("protected request error = %v", err)
	}
	if _, err := s.Mutate("repo", "pull", revision, v.WorkspaceVersion-1, "comment", "user", "translator", map[string]any{"unit_id": v.Extractions[0].Units[0].ID, "locale": "es", "body": "stale"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version error = %v", err)
	}
}

func TestSuggestionCannotRedirectRequestToAnotherProtectedUnitOrLocale(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := "1111111111111111111111111111111111111111"
	v, _ := s.Extract("repo", "pull", revision, "owner", ExtractionMap{ID: "web", Version: 1, Name: "Web", Include: []string{"src/**"}, Formats: []string{"typescript"}}, []string{"fr", "de"}, []Unit{
		{Key: "normal", Message: "Welcome", Context: "Public hero", Locations: []Location{{Path: "src/a.tsx", Line: 1}}},
		{Key: "secret", Message: "Unreleased launch", Context: "Embargoed launch", Protected: true, Locations: []Location{{Path: "src/b.tsx", Line: 1}}},
	})
	normal, secret := v.Extractions[0].Units[0].ID, v.Extractions[0].Units[1].ID
	v, err := s.Mutate("repo", "pull", revision, v.WorkspaceVersion, "request_suggestion", "user", "translator", map[string]any{"unit_id": normal, "locale": "fr", "agent_id": "agent", "product_context": "Public hero", "locale_plan_id": "plan", "locale_plan_version": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Mutate("repo", "pull", revision, v.WorkspaceVersion, "suggest", "agent", "agent", map[string]any{"unit_id": secret, "locale": "de", "request_id": v.SuggestionRequests[0].ID, "text": "Geheimer Start", "rationale": "redirected", "uncertainty": "low", "evidence": []map[string]any{{"kind": "locale_plan", "reference": "plan:1"}, {"kind": "source_context", "reference": "unit:secret"}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-scope protected suggestion error = %v", err)
	}
	stored, readErr := s.Get("repo", "pull", revision)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(stored.Suggestions) != 0 || stored.SuggestionRequests[0].State != "requested" {
		t.Fatalf("redirect persisted: %#v", stored.Suggestions)
	}
}

func TestLocaleVerificationRetainsCompleteChecksAndInvalidatesOnlyAffectedEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := "1111111111111111111111111111111111111111"
	v, err := s.Extract("repo", "pull", revision, "owner", ExtractionMap{ID: "web", Version: 1, Name: "Web", Include: []string{"src/**"}, Formats: []string{"typescript"}}, []string{"ar"}, []Unit{
		{Key: "welcome", Message: "Welcome {name}", Context: "Home", Variables: []Variable{{Name: "name", Example: "Mina"}}, Locations: []Location{{Path: "src/home.tsx", Line: 2}}},
		{Key: "items", Message: "{count} items", Context: "Cart", PluralRule: "cardinal", Locations: []Location{{Path: "src/cart.tsx", Line: 3}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	welcome, items := v.Extractions[0].Units[0].ID, v.Extractions[0].Units[1].ID
	v, _ = s.Propose("repo", "pull", revision, welcome, "ar", "مرحبًا {name}", "", "translator")
	v, _ = s.Propose("repo", "pull", revision, items, "ar", "{count} عناصر", "", "translator")
	v, err = s.Verify("repo", "pull", revision, v.WorkspaceVersion, "publish_candidate", "owner", "translator", 2, map[string]any{
		"locale": "ar", "preview_id": "preview", "preview_url": "/bounded/preview", "locale_plan_id": "plan", "locale_plan_version": float64(2),
		"routes": []map[string]any{{"journey_id": "home", "route": "/ar/home", "interface_hash": strings.Repeat("a", 64)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate := v.VerificationCandidates[0]
	results := []map[string]any{}
	for _, kind := range verificationKinds() {
		results = append(results, map[string]any{"kind": kind, "route": "/ar/home", "unit_ids": []string{welcome, items}, "status": "passed", "summary": kind + " passed"})
	}
	v, err = s.Verify("repo", "pull", revision, v.WorkspaceVersion, "record_checks", "owner", "translator", 2, map[string]any{"candidate_id": candidate.ID, "results": results})
	if err != nil || len(v.VerificationRuns) != 1 || !v.Verification[0].Current {
		t.Fatalf("verification run = %#v, %v", v.Verification, err)
	}
	v, err = s.Verify("repo", "pull", revision, v.WorkspaceVersion, "finding", "regional", "regional_reviewer", 2, map[string]any{"candidate_id": candidate.ID, "locale": "ar", "route": "/ar/home", "unit_ids": []string{welcome}, "category": "bidirectional_text", "severity": "high", "body": "The name interrupts reading order."})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Verify("repo", "pull", revision, v.WorkspaceVersion, "review", "regional", "regional_reviewer", 2, map[string]any{"candidate_id": candidate.ID, "locale": "ar", "route": "/ar/home", "unit_ids": []string{items}, "kind": "approve", "reason": "Plural forms read naturally in the cart."})
	if err != nil || v.LocaleReviewDecisions[0].ActorRole != "regional_reviewer" {
		t.Fatalf("regional review = %#v, %v", v.LocaleReviewDecisions, err)
	}
	v, err = s.Propose("repo", "pull", revision, welcome, "ar", "أهلًا {name}", "tone correction", "translator")
	if err != nil {
		t.Fatal(err)
	}
	if v.Verification[0].Current || !contains(v.Verification[0].StaleScopes, "translation:"+welcome) || contains(v.Verification[0].StaleScopes, "translation:"+items) {
		t.Fatalf("selective invalidation = %#v", v.Verification[0])
	}
}

func TestLocaleVerificationRequiresEveryDefinedExperienceCheck(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := "1111111111111111111111111111111111111111"
	v, _ := s.Extract("repo", "pull", revision, "owner", ExtractionMap{ID: "web", Version: 1, Name: "Web", Include: []string{"src/**"}, Formats: []string{"typescript"}}, []string{"fr"}, []Unit{{Key: "welcome", Message: "Welcome", Context: "Home", Locations: []Location{{Path: "src/home.tsx", Line: 2}}}})
	unit := v.Extractions[0].Units[0].ID
	v, _ = s.Propose("repo", "pull", revision, unit, "fr", "Bienvenue", "", "translator")
	v, _ = s.Verify("repo", "pull", revision, v.WorkspaceVersion, "publish_candidate", "owner", "translator", 1, map[string]any{"locale": "fr", "preview_id": "preview", "preview_url": "/bounded", "locale_plan_id": "plan", "locale_plan_version": float64(1), "routes": []map[string]any{{"journey_id": "home", "route": "/fr", "interface_hash": strings.Repeat("a", 64)}}})
	_, err := s.Verify("repo", "pull", revision, v.WorkspaceVersion, "record_checks", "owner", "translator", 1, map[string]any{"candidate_id": v.VerificationCandidates[0].ID, "results": []map[string]any{{"kind": "variables", "route": "/fr", "unit_ids": []string{unit}, "status": "passed", "summary": "passed"}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("incomplete check suite error = %v", err)
	}
}

func TestLocalePlanSuccessorStalesCandidateAndRejectsNewEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := "1111111111111111111111111111111111111111"
	v, _ := s.Extract("repo", "pull", revision, "owner", ExtractionMap{ID: "web", Version: 1, Name: "Web", Include: []string{"src/**"}, Formats: []string{"typescript"}}, []string{"fr"}, []Unit{{Key: "welcome", Message: "Welcome", Context: "Home", Locations: []Location{{Path: "src/home.tsx", Line: 2}}}})
	unit := v.Extractions[0].Units[0].ID
	v, _ = s.Propose("repo", "pull", revision, unit, "fr", "Bienvenue", "", "translator")
	v, err := s.Verify("repo", "pull", revision, v.WorkspaceVersion, "publish_candidate", "owner", "translator", 1, map[string]any{"locale": "fr", "preview_id": "preview", "preview_url": "/bounded", "locale_plan_id": "plan", "locale_plan_version": float64(1), "routes": []map[string]any{{"journey_id": "home", "route": "/fr", "interface_hash": strings.Repeat("a", 64)}}})
	if err != nil {
		t.Fatal(err)
	}
	ApplyLocalePlanVersions(&v, map[string]int{"plan": 2})
	if v.Verification[0].Current || !contains(v.Verification[0].StaleScopes, "locale_plan") {
		t.Fatalf("plan successor projection = %#v", v.Verification[0])
	}
	results := []map[string]any{}
	for _, kind := range verificationKinds() {
		results = append(results, map[string]any{"kind": kind, "route": "/fr", "unit_ids": []string{unit}, "status": "passed", "summary": "passed"})
	}
	_, err = s.Verify("repo", "pull", revision, v.WorkspaceVersion, "record_checks", "owner", "translator", 2, map[string]any{"candidate_id": v.VerificationCandidates[0].ID, "results": results})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("superseded plan check error = %v", err)
	}
}
