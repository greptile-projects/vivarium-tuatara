package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
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
}
