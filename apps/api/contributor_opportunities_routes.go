package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributoropportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerContributorOpportunityRoutes(mux *http.ServeMux, repos *repositories.Store, opportunities *contributoropportunities.Store, issueStore *issues.Store, proposalStore *proposals.Store, credentials *auth.Store) {
	read := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		return actor, ok
	}
	mux.HandleFunc("GET /repositories/{id}/contribution-opportunities", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := read(w, r); !ok {
			return
		}
		items, err := opportunities.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "opportunities_read_failed", "contribution opportunities could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"opportunities": items})
	})
	mux.HandleFunc("POST /repositories/{id}/contribution-opportunity-matches", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := read(w, r); !ok {
			return
		}
		var p contributoropportunities.Profile
		if decodeJSON(r, &p) != nil || p.AvailableMinutes < 15 || p.AvailableMinutes > 10080 || p.MaximumRisk != "low" && p.MaximumRisk != "medium" && p.MaximumRisk != "high" {
			writeAPIError(w, 422, "invalid_match_profile", "skills, interests, available minutes, and maximum risk must describe realistic constraints")
			return
		}
		items, err := opportunities.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "opportunities_read_failed", "contribution opportunities could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"matches": contributoropportunities.MatchAll(items, p, time.Now())})
	})
	mux.HandleFunc("PUT /repositories/{id}/contribution-opportunities/{opportunity}", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "owner_required", "only the repository owner can publish contribution opportunities")
			return
		}
		var input struct {
			ExpectedVersion int                                  `json:"expected_version"`
			Opportunity     contributoropportunities.Opportunity `json:"opportunity"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		input.Opportunity.RepositoryID = r.PathValue("id")
		input.Opportunity.PublishedBy = actor.UserID
		pathID := r.PathValue("opportunity")
		if pathID != "new" {
			input.Opportunity.ID = pathID
		} else {
			input.Opportunity.ID = ""
		}
		// A published opportunity must remain grounded in a live collaboration record.
		sourceOK := false
		switch input.Opportunity.Source.Kind {
		case "issue":
			if issueStore != nil {
				v, e := issueStore.Get(r.PathValue("id"), input.Opportunity.Source.ID)
				sourceOK = e == nil && v.Triage.Classification != ""
			}
		case "proposal":
			if proposalStore != nil {
				_, e := proposalStore.Get(r.PathValue("id"), input.Opportunity.Source.ID)
				sourceOK = e == nil
			}
		case "task":
			if proposalStore != nil && input.Opportunity.Source.ParentID != "" {
				_, e := proposalStore.GetTask(r.PathValue("id"), input.Opportunity.Source.ParentID, input.Opportunity.Source.ID)
				sourceOK = e == nil
			}
		case "stewardship":
			sourceOK = input.Opportunity.Source.ID != ""
		}
		if !sourceOK {
			writeAPIError(w, 422, "invalid_opportunity_source", "the source must resolve to a triaged issue, proposal, planned task, or stewardship finding")
			return
		}
		v, err := opportunities.Publish(input.Opportunity, input.ExpectedVersion)
		if errors.Is(err, contributoropportunities.ErrConflict) {
			writeAPIError(w, 409, "opportunity_changed", "contribution opportunity changed")
			return
		}
		if errors.Is(err, contributoropportunities.ErrInvalid) || errors.Is(err, contributoropportunities.ErrNotFound) {
			writeAPIError(w, 422, "invalid_opportunity", "bounded outcome, skills, scope, risk, revision, source, and estimate are required")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "opportunity_write_failed", "contribution opportunity could not be retained")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/contribution-opportunities/{opportunity}/claim", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		if _, _, ok = authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		var input struct {
			ExpectedVersion int    `json:"expected_version"`
			Hours           int    `json:"hours"`
			Note            string `json:"note"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, err := opportunities.Claim(r.PathValue("id"), r.PathValue("opportunity"), actor.UserID, input.Note, time.Duration(input.Hours)*time.Hour, input.ExpectedVersion)
		opportunityResult(w, v, err, true)
	})
	mux.HandleFunc("POST /repositories/{id}/contribution-opportunities/{opportunity}/release", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		if _, _, ok = authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		repo, err := repos.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		var input struct {
			ExpectedVersion int `json:"expected_version"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, err := opportunities.Release(repo.ID, r.PathValue("opportunity"), actor.UserID, actor.UserID == repo.OwnerID, input.ExpectedVersion)
		opportunityResult(w, v, err, false)
	})
}
func opportunityResult(w http.ResponseWriter, v contributoropportunities.Opportunity, err error, created bool) {
	if errors.Is(err, contributoropportunities.ErrConflict) {
		writeAPIError(w, 409, "opportunity_changed", "contribution opportunity changed")
		return
	}
	if errors.Is(err, contributoropportunities.ErrClaimed) {
		writeAPIError(w, 409, "opportunity_claimed", "another contributor currently holds this opportunity")
		return
	}
	if errors.Is(err, contributoropportunities.ErrInvalid) {
		writeAPIError(w, 422, "invalid_opportunity_claim", "claim duration or opportunity state is invalid")
		return
	}
	if errors.Is(err, contributoropportunities.ErrNotFound) {
		writeAPIError(w, 404, "opportunity_not_found", "contribution opportunity not found")
		return
	}
	if err != nil {
		writeAPIError(w, 500, "opportunity_write_failed", "contribution opportunity could not be updated")
		return
	}
	status := 200
	if created {
		status = 201
	}
	writeJSON(w, status, v)
}
