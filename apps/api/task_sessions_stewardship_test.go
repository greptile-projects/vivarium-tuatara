package main

import (
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
)

func TestStewardshipTaskLaunchRevalidatesRecordedAuthority(t *testing.T) {
	now := time.Now().UTC()
	operator, agent := "11111111111111111111111111111111", "22222222222222222222222222222222"
	mandateID, opportunityID, proposalID, taskID := "33333333333333333333333333333333", "44444444444444444444444444444444", "55555555555555555555555555555555", "66666666666666666666666666666666"
	base := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	task := proposals.Task{ID: taskID, Assignment: &proposals.TaskAssignment{ID: "77777777777777777777777777777777", AssigneeID: agent, Access: proposals.TaskAccess{BaseRevision: base}}, Reasoning: &proposals.ReasoningOrigin{AnalysisStatus: "stewardship_opportunity", OrganizationID: "88888888888888888888888888888888", MandateID: mandateID, OpportunityID: opportunityID, Revision: base}}
	organization := organizations.Organization{StewardshipMandates: []organizations.StewardshipMandate{{
		ID: mandateID, Version: 1, Status: "active",
		Acceptance: &organizations.MandateAcceptance{Version: 1, OperatorID: operator},
		Revisions:  []organizations.MandateRevision{{Version: 1, AgentID: agent, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}},
		Opportunities: []organizations.StewardshipOpportunity{{
			ID: opportunityID, Status: "promoted",
			Work: &organizations.OpportunityWorkLink{ProposalID: proposalID, TaskIDs: []string{taskID}, BaseRevision: base},
		}},
	}}}
	if !startableStewardshipTask(organization, task, operator, proposalID, now) {
		t.Fatal("current recorded stewardship work was not startable")
	}
	organization.StewardshipMandates[0].Status = "paused"
	if startableStewardshipTask(organization, task, operator, proposalID, now) {
		t.Fatal("paused mandate still launched")
	}
	organization.StewardshipMandates[0].Status = "active"
	if startableStewardshipTask(organization, task, "99999999999999999999999999999999", proposalID, now) {
		t.Fatal("non-operator launched steward work")
	}
	task.Assignment.AssigneeID = "99999999999999999999999999999999"
	if startableStewardshipTask(organization, task, operator, proposalID, now) {
		t.Fatal("a different agent inherited the stewardship mandate")
	}
}
