package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productopportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/roadmaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestRoadmapMutationsAcceptCookieOnlyIdentity(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	repos, _ := repositories.New(t.TempDir(), git)
	credentials, _ := auth.New(t.TempDir())
	opportunities, _ := productopportunities.New(t.TempDir())
	roadmapStore, _ := roadmaps.New(t.TempDir())
	repo, err := repos.Create("0123456789abcdef0123456789abcdef", "roadmap-cookie")
	if err != nil {
		t.Fatal(err)
	}
	opportunity, err := opportunities.Create(repo.ID, "0123456789abcdef0123456789abcdef", "human", productopportunities.Revision{Title: "Review continuity", Need: "Reviewers lose context.", AffectedAudiences: []string{"reviewers"}, Severity: "high", Reach: "segment", Confidence: "medium", ExpectedValue: "Fewer abandoned reviews.", Sources: []productopportunities.Source{{Kind: "issue", ResourceID: "issue-1", Revision: "1", Label: "Lost context", Claim: "Reviewers restart work.", Relationship: "supports", Audience: "reviewers"}}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := credentials.Issue("0123456789abcdef0123456789abcdef", auth.Session, "browser session", []string{"repositories:read", "repositories:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerRoadmapRoutes(mux, nil, repos, credentials, roadmapStore, opportunities, nil, nil, nil, nil, nil)
	server := httptest.NewServer(mux)
	defer server.Close()
	revision := roadmaps.Revision{Goals: []string{"Continuous review"}, Capacity: "One team", Decisions: []roadmaps.OpportunityDecision{{OpportunityID: opportunity.ID, Version: 1, Outcome: "accepted", Reason: "High value", GoalFit: "Direct", Capacity: "Fits"}}, Items: []roadmaps.Item{{ID: "item-1", OpportunityID: opportunity.ID, Title: "Review continuity", OwnerIDs: []string{"0123456789abcdef0123456789abcdef"}, TargetHorizon: "Q4", SuccessMeasures: []string{"Fewer abandoned reviews"}, Position: 1, Status: "planned"}}}
	request := func(method, path string, body any, want int) {
		payload, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "vivarium_session", Value: session.Token})
		response, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("%s %s status = %d", method, path, response.StatusCode)
		}
	}
	path := "/repositories/" + repo.ID + "/roadmap"
	request(http.MethodPut, path, roadmapMutation{ExpectedVersion: 0, Revision: revision}, http.StatusOK)
	request(http.MethodPost, path+"/scenarios", roadmapMutation{ExpectedVersion: 1, Revision: revision, Rationale: "Alternative sequence"}, http.StatusCreated)
	request(http.MethodPost, path+"/comments", roadmapMutation{ExpectedVersion: 2, Body: "Preserve the target."}, http.StatusCreated)
	stored, _ := roadmapStore.Get(repo.ID)
	if stored.Revisions[0].CreatedBy != "0123456789abcdef0123456789abcdef" || stored.Scenarios[0].ActorID != "0123456789abcdef0123456789abcdef" || stored.Comments[0].ActorID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("cookie attribution = %#v", stored)
	}
}

func TestRoadmapOutcomeValidationSeparatesDeliveryFromMeasuredCoverage(t *testing.T) {
	if got := mapRoadmapEvidenceKind("delivery"); got == "coverage" {
		t.Fatalf("delivery mapped to measurement coverage: %q", got)
	}
	if got := mapRoadmapEvidenceKind("measure_met"); got != "coverage" {
		t.Fatalf("measure success mapping = %q", got)
	}
}
