package main

import (
	"net/http"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/reviewplans"
)

type reviewPlanInput struct {
	RiskSummary    string `json:"risk_summary"`
	CompletionRule string `json:"completion_rule"`
}

func registerReviewPlanRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, plans *reviewplans.Store) {
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/review-plans", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		values, e := plans.List(p.RepositoryID, p.ID, p.SourceCommitID, p.TargetCommitID)
		if e != nil {
			writeAPIError(w, 500, "review_plans_unavailable", "review plans could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"review_plans": values})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/review-plans", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok || actor.AgentID != "" {
			return
		}
		var in reviewPlanInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "review plan input is invalid")
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil || p.Status != pullrequests.Open {
			writeAPIError(w, 404, "pull_request_not_found", "open pull request not found")
			return
		}
		repo, e := catalog.GetByID(p.RepositoryID)
		if e != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		if actor.UserID != p.AuthorID && actor.UserID != repo.OwnerID {
			writeAPIError(w, 403, "review_plan_forbidden", "only the pull author or repository owner can create a review plan")
			return
		}
		changes, e := pulls.Changes(p.RepositoryID, p.ID)
		if e != nil {
			writeAPIError(w, 422, "review_context_unavailable", "exact changed code could not be derived")
			return
		}
		paths := []string{}
		for _, c := range changes {
			paths = append(paths, c.Path)
		}
		plan := deriveReviewPlan(p, repo, actor.UserID, paths, in)
		checks, err := catalog.RequiredChecks(repo.ID, p.TargetBranch)
		if err != nil {
			writeAPIError(w, 503, "required_checks_unavailable", "target branch review policy could not be resolved")
			return
		}
		plan.PolicyRequirements = append(plan.PolicyRequirements, checks...)
		plan.PolicyRequirements = reviewplans.Normalize(plan.PolicyRequirements)
		out, e := plans.Create(plan)
		if e != nil {
			writeAPIError(w, 422, "invalid_review_plan", "review plan could not be created")
			return
		}
		w.Header().Set("Location", "/repositories/"+p.RepositoryID+"/pulls/"+p.ID+"/review-plans")
		writeJSON(w, 201, out)
	})
}

func deriveReviewPlan(p pullrequests.PullRequest, repo repositories.Repository, actor string, paths []string, in reviewPlanInput) reviewplans.Plan {
	paths = reviewplans.Normalize(paths)
	risks := []string{}
	commitments := []string{}
	policy := []string{"ordinary revision-bound approval"}
	areas := []reviewplans.Area{{ID: "change-intent", Title: "Change intent and code behavior", Rationale: "Every changed path must satisfy the pull's declared outcome.", Paths: paths, OwnerIDs: []string{repo.OwnerID}, Questions: []string{"Does the exact diff implement the declared intent without unintended behavior?", "Are compatibility and failure modes understood?"}, Evidence: []reviewplans.Evidence{{Kind: "diff", Description: "Inspect the exact source-to-target changed code.", Required: true}, {Kind: "checks", Description: "Run repository-required checks for this revision.", Required: true}}, CompletionRule: "A current human owner answers every acceptance question and inspects all required evidence."}}
	domains := map[string][]string{}
	for _, p := range paths {
		l := strings.ToLower(p)
		if strings.Contains(l, "security") || strings.Contains(l, "auth") || strings.Contains(l, "credential") {
			domains["security"] = append(domains["security"], p)
		}
		if strings.Contains(l, "accessib") || strings.HasSuffix(l, ".tsx") || strings.HasSuffix(l, ".css") {
			domains["accessibility"] = append(domains["accessibility"], p)
		}
		if strings.Contains(l, "api") || strings.Contains(l, "schema") || strings.Contains(l, "migration") {
			domains["interface"] = append(domains["interface"], p)
		}
		if strings.Contains(l, "privacy") || strings.Contains(l, "data") || strings.Contains(l, "telemetry") {
			domains["privacy"] = append(domains["privacy"], p)
		}
	}
	for _, d := range []string{"security", "privacy", "accessibility", "interface"} {
		ps := domains[d]
		if len(ps) == 0 {
			continue
		}
		risks = append(risks, d)
		commitments = append(commitments, d)
		areas = append(areas, reviewplans.Area{ID: d, Title: strings.ToUpper(d[:1]) + d[1:] + " impact", Rationale: "Changed paths intersect the project's " + d + " commitments.", Paths: reviewplans.Normalize(ps), OwnerIDs: []string{repo.OwnerID}, Questions: []string{"Does the change preserve the affected " + d + " commitments?", "Is residual risk explicit and acceptable?"}, Evidence: []reviewplans.Evidence{{Kind: d + "_evidence", Description: "Current evidence covering affected " + d + " behavior.", Required: true}}, DependsOn: []string{"change-intent"}, CompletionRule: "The accountable owner resolves both questions against current evidence."})
	}
	diagnostics := []reviewplans.Diagnostic{}
	if repo.OwnerID == "" {
		diagnostics = append(diagnostics, reviewplans.Diagnostic{Code: "missing_ownership", Message: "No current repository owner can account for review scope."})
	}
	pathAreas := map[string]int{}
	for _, a := range areas {
		for _, p := range a.Paths {
			pathAreas[p]++
		}
	}
	for p, n := range pathAreas {
		if n > 1 {
			diagnostics = append(diagnostics, reviewplans.Diagnostic{Code: "overlapping_scope", Message: p + " requires coordinated review across multiple areas.", AttributedTo: actor})
		}
	}
	risk := strings.TrimSpace(in.RiskSummary)
	if risk == "" {
		if len(risks) == 0 {
			risk = "No specialized risk was inferred; reviewers must challenge this classification."
		} else {
			risk = "Changed paths affect " + strings.Join(risks, ", ") + " concerns."
		}
	}
	rule := strings.TrimSpace(in.CompletionRule)
	if rule == "" {
		rule = "Every review area must have its questions answered and required evidence inspected for this exact source and target revision; visible diagnostics remain gaps."
	}
	return reviewplans.Plan{RepositoryID: p.RepositoryID, PullRequestID: p.ID, SourceRevision: p.SourceCommitID, TargetRevision: p.TargetCommitID, Intent: p.Body, RiskSummary: risk, ChangedPaths: paths, PolicyRequirements: policy, AffectedCommitments: reviewplans.Normalize(commitments), Areas: areas, CompletionRule: rule, Diagnostics: diagnostics, CreatedBy: actor}
}
