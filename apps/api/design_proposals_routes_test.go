package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
	productfeedback "github.com/greptile-projects/vivarium-tuatara/apps/api/feedback"
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

func TestDesignEvidenceProjectionRechecksCurrentReaderVisibility(t *testing.T) {
	feedbackStore, err := productfeedback.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repositoryID := "22222222222222222222222222222222"
	item, err := feedbackStore.Create(productfeedback.Item{RepositoryID: repositoryID, Target: productfeedback.Target{Kind: "project", Label: "Setup"}, Need: "Safer setup", DesiredOutcome: "Preview effects", Frequency: "weekly", Impact: "Abandoned setup", Audience: "organization_private", IdentityVisibility: "reporter_only", ContactPreference: "none"}, "reporter")
	if err != nil {
		t.Fatal(err)
	}
	source := designproposals.Source{Kind: "feedback", ResourceID: item.ID}
	proposal := func() designproposals.Proposal {
		return designproposals.Proposal{RepositoryID: repositoryID, Revisions: []designproposals.Revision{{Source: source, Evidence: []designproposals.Evidence{{Kind: "feedback", ResourceID: item.ID, Accessible: true}}}}}
	}
	anonymous := proposal()
	projectDesignEvidence(&anonymous, "", false, nil, feedbackStore, nil, nil, nil)
	if anonymous.Revisions[0].Evidence[0].Accessible {
		t.Fatalf("anonymous reader received restricted feedback: %#v", anonymous)
	}
	outsider := proposal()
	projectDesignEvidence(&outsider, "authenticated-outsider", false, nil, feedbackStore, nil, nil, nil)
	if outsider.Revisions[0].Evidence[0].Accessible {
		t.Fatalf("nonparticipant received organization-private feedback: %#v", outsider)
	}
	participant := proposal()
	projectDesignEvidence(&participant, "maintainer", true, nil, feedbackStore, nil, nil, nil)
	if !participant.Revisions[0].Evidence[0].Accessible {
		t.Fatalf("participant lost accessible feedback: %#v", participant)
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
