package interfacesystems

import "testing"

func revision(name string) Revision {
	commit := "0123456789abcdef0123456789abcdef01234567"
	definition := Definition{Name: name, Description: "Reviewed action", Usage: "Use for the primary task.", SourcePath: "src/button.tsx", Examples: []Example{{Title: "Save", Description: "Ready state", Properties: map[string]string{"state": "default"}}}, Constraints: Constraint{Accessibility: []string{"Visible focus"}, Localization: []string{"Label may expand"}}}
	return Revision{Title: "Product UI", Summary: "Shared decisions", Rationale: "Keep journeys coherent.", CommitID: commit, ReleaseID: "release", ReleaseVersion: "v1", Themes: []string{"light"}, Tokens: []Token{{Name: "color.action", Category: "color", Value: "#123456", Theme: "light", Description: "Action color"}}, Components: []Definition{definition}, InteractionPatterns: []Definition{{Name: "confirmation", Description: "Confirm risk", Usage: "Destructive actions", SourcePath: "src/dialog.tsx", Examples: definition.Examples}}, ContentRules: []Definition{{Name: "button-label", Description: "Verb first", Usage: "Actions", SourcePath: "src/content.ts", Examples: definition.Examples}}, ResponsiveRules: []ResponsiveRule{{Name: "compact", Condition: "width < 40rem", Behavior: "Stack controls"}}, AdoptionPolicy: AdoptionPolicy{Level: "recommended", SupportedConsumers: []string{"web"}, MigrationGuidance: "Adopt in the next reviewed release."}, Implementations: []Implementation{{Consumer: "web", CommitID: commit, DefinitionName: name, Status: "stale"}}}
}

func TestVersionHistoryAndExplicitDiagnostics(t *testing.T) {
	s, err := New(t.TempDir()); if err != nil { t.Fatal(err) }
	first, err := s.Create("repo", "actor", revision("Button")); if err != nil { t.Fatal(err) }
	if len(first.Diagnostics) == 0 { t.Fatal("missing owners and stale implementation must remain explicit") }
	next := revision("Button"); next.Summary = "Successor"
	updated, err := s.Revise(first.ID, 1, "actor", next); if err != nil { t.Fatal(err) }
	if updated.CurrentVersion != 2 || len(updated.Revisions) != 2 || updated.Revisions[0].Summary != "Shared decisions" { t.Fatalf("history was not retained: %#v", updated) }
	if _, err = s.Revise(first.ID, 1, "actor", next); err != ErrConflict { t.Fatalf("stale write = %v", err) }
}

func TestConflictingCurrentDefinitionsAreProjected(t *testing.T) {
	s, _ := New(t.TempDir()); a, _ := s.Create("repo", "one", revision("Button")); _, _ = s.Create("repo", "two", revision("Button"))
	out, err := s.Get(a.ID); if err != nil { t.Fatal(err) }
	found := false; for _, d := range out.Diagnostics { if d.Kind == "conflicting_definition" { found = true } }
	if !found { t.Fatal("conflicting current definitions were presented as one coherent system") }
}
