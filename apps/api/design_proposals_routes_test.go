package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
)

func TestDesignArtifactProjectionHonorsExplicitAudience(t *testing.T) {
	v := designproposals.Proposal{Revisions: []designproposals.Revision{{Artifacts: []designproposals.Artifact{{ID: "private-research", Description: "private interview", Content: "participant details", Interactions: []string{"open transcript"}, Audience: []string{"invited-user"}}}}}}
	redactDesignArtifacts(&v, "developer")
	a := v.Revisions[0].Artifacts[0]
	if a.Content != "" || len(a.Interactions) != 0 || a.Description == "private interview" {
		t.Fatalf("restricted artifact leaked: %#v", a)
	}

	visible := designproposals.Proposal{Revisions: []designproposals.Revision{{Artifacts: []designproposals.Artifact{{Content: "interactive flow", Interactions: []string{"continue"}, Audience: []string{"invited-user"}}}}}}
	redactDesignArtifacts(&visible, "invited-user")
	if visible.Revisions[0].Artifacts[0].Content != "interactive flow" {
		t.Fatalf("explicit audience lost artifact: %#v", visible)
	}

	anonymous := designproposals.Proposal{Revisions: []designproposals.Revision{{Artifacts: []designproposals.Artifact{{Content: "secret", Audience: []string{""}}}}}}
	redactDesignArtifacts(&anonymous, "")
	if anonymous.Revisions[0].Artifacts[0].Content != "" {
		t.Fatalf("empty audience authorized anonymous reader: %#v", anonymous)
	}
}

func TestDesignEvidenceRequiresAuthoritativeRepositorySource(t *testing.T) {
	issueStore, err := issues.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	missing := designproposals.Revision{Source: designproposals.Source{Kind: "issue", ResourceID: "missing"}, Evidence: []designproposals.Evidence{{Kind: "issue", ResourceID: "missing", Accessible: true}}}
	normalizeDesignEvidence("repo", &missing, issueStore, nil, nil, nil, nil)
	if missing.Evidence[0].Accessible || missing.Evidence[0].Gap == "" {
		t.Fatalf("missing issue remained accessible: %#v", missing.Evidence[0])
	}
	created, err := issueStore.Create(issues.Issue{RepositoryID: "repo", ReporterID: "reporter", Title: "Unexpected", ExpectedBehavior: "works", ObservedBehavior: "fails", Severity: "medium", Environment: "test", ReproductionSteps: []string{"run"}, Visibility: "repository"})
	if err != nil {
		t.Fatal(err)
	}
	visible := designproposals.Revision{Source: designproposals.Source{Kind: "issue", ResourceID: created.ID}, Evidence: []designproposals.Evidence{{Kind: "issue", ResourceID: created.ID}}}
	normalizeDesignEvidence("repo", &visible, issueStore, nil, nil, nil, nil)
	if !visible.Evidence[0].Accessible {
		t.Fatalf("repository issue was not resolved: %#v", visible.Evidence[0])
	}
	crossRepository := designproposals.Revision{Source: visible.Source, Evidence: []designproposals.Evidence{{Kind: "issue", ResourceID: created.ID}}}
	normalizeDesignEvidence("other", &crossRepository, issueStore, nil, nil, nil, nil)
	if crossRepository.Evidence[0].Accessible {
		t.Fatalf("cross-repository issue became accessible: %#v", crossRepository.Evidence[0])
	}
}
