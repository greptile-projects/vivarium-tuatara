package localization

import "testing"

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
