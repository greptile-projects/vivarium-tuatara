package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerLearningAssessmentRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, pathways *learningpathways.Store, assessments *learningassessments.Store, workspaceStore *workspaces.Store, checks *checkruns.Store, credentials *auth.Store) {
	isOwner := func(repo, user string) bool { v, e := repos.GetByID(repo); return e == nil && v.OwnerID == user }
	project := func(v learningassessments.Definition, owner bool) learningassessments.Definition {
		if !owner {
			for i := range v.ProtectedCases {
				v.ProtectedCases[i].ID = ""
				v.ProtectedCases[i].Description = "Protected case"
				v.ProtectedCases[i].Expected = ""
			}
		}
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/learning-assessments", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		owner := authenticated && isOwner(r.PathValue("id"), actor.UserID)
		slugs, e := assessments.Slugs(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "learning_assessment_read_failed", "assessments could not be read")
			return
		}
		out := []learningassessments.Definition{}
		for _, slug := range slugs {
			if v, e := assessments.Current(r.PathValue("id"), slug); e == nil {
				out = append(out, project(v, owner))
			}
		}
		writeJSON(w, 200, map[string]any{"assessments": out})
	})
	mux.HandleFunc("GET /repositories/{id}/learning-assessments/{slug}", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := assessments.Current(r.PathValue("id"), r.PathValue("slug"))
		if e != nil {
			writeAPIError(w, 404, "learning_assessment_not_found", "assessment not found")
			return
		}
		attempts, _ := assessments.Attempts(r.PathValue("id"), r.PathValue("slug"))
		visible := attempts[:0]
		owner := authenticated && isOwner(r.PathValue("id"), actor.UserID)
		for _, a := range attempts {
			if owner || authenticated && a.LearnerID == actor.UserID {
				visible = append(visible, a)
			}
		}
		writeJSON(w, 200, map[string]any{"assessment": project(v, owner), "attempts": visible})
	})
	mux.HandleFunc("PUT /repositories/{id}/learning-assessments/{slug}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !isOwner(r.PathValue("id"), actor.UserID) {
			writeAPIError(w, 403, "repository_owner_required", "only the repository owner can publish assessments")
			return
		}
		var in struct {
			ExpectedVersion int                            `json:"expected_version"`
			RequestID       string                         `json:"request_id"`
			Assessment      learningassessments.Definition `json:"assessment"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		history, e := pathways.List(r.PathValue("id"), in.Assessment.PathwaySlug)
		if e != nil || in.Assessment.PathwayVersion < 1 || in.Assessment.PathwayVersion > len(history) || !learningAssessmentContains(history[in.Assessment.PathwayVersion-1].SupportedRevisions, in.Assessment.ProjectRevision) {
			writeAPIError(w, 422, "invalid_learning_assessment", "assessment must bind an exact supported pathway and project revision")
			return
		}
		in.Assessment.ID = ""
		in.Assessment.Version = 0
		in.Assessment.RepositoryID = r.PathValue("id")
		in.Assessment.Slug = r.PathValue("slug")
		in.Assessment.RequestID = in.RequestID
		in.Assessment.PublishedBy = actor.UserID
		v, e := assessments.Publish(in.Assessment, in.ExpectedVersion)
		if errors.Is(e, learningassessments.ErrConflict) {
			writeAPIError(w, 409, "learning_assessment_changed", "assessment changed")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "invalid_learning_assessment", "public criteria, retry policy, pathway, and exact revision are required")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/learning-assessments/{slug}/attempts", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		var in struct {
			RequestID         string                       `json:"request_id"`
			AssessmentVersion int                          `json:"assessment_version"`
			WorkspaceID       string                       `json:"workspace_id"`
			Evidence          learningassessments.Evidence `json:"evidence"`
			Accommodation     string                       `json:"accommodation"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		defs, _ := assessments.List(r.PathValue("id"), r.PathValue("slug"))
		if in.AssessmentVersion < 1 || in.AssessmentVersion > len(defs) {
			writeAPIError(w, 422, "invalid_learning_attempt", "exact assessment revision is required")
			return
		}
		d := defs[in.AssessmentVersion-1]
		ws, e := workspaceStore.Get(in.WorkspaceID)
		if e != nil || ws.CreatorID != actor.UserID || ws.RepositoryID != r.PathValue("id") || ws.CommitID != d.ProjectRevision || ws.LearningContext == nil || ws.LearningContext.PathwaySlug != d.PathwaySlug || ws.LearningContext.PathwayVersion != d.PathwayVersion {
			writeAPIError(w, 422, "invalid_learning_attempt", "workspace must be the learner's reproducible attempt for the exact pathway and project revision")
			return
		}
		if !evidenceExists(ws, in.Evidence) {
			writeAPIError(w, 422, "invalid_learning_evidence", "cited checkpoint and command evidence must exist in the workspace")
			return
		}
		if in.Accommodation != "" && !learningAssessmentContains(d.AccommodationOptions, in.Accommodation) {
			writeAPIError(w, 422, "invalid_accommodation", "accommodation is not permitted by this assessment")
			return
		}
		blockers := []string{}
		if strings.TrimSpace(in.Evidence.AuthorshipStatement) == "" {
			blockers = append(blockers, "authorship_unattested")
		}
		for _, g := range ws.LearningContext.Guidance.Entries {
			if g.ActorKind == "agent" && g.Kind != "hint" {
				blockers = append(blockers, "agent_overreach")
			}
		}
		workProduct := learningWorkProductDigest(ws, actor.UserID)
		prior, _ := assessments.Attempts(r.PathValue("id"), r.PathValue("slug"))
		for _, p := range prior {
			if workProduct != "" && p.LearnerID != actor.UserID && p.WorkProductSHA256 == workProduct {
				blockers = append(blockers, "copied_solution_signal")
			}
		}
		a, e := assessments.CreateAttempt(learningassessments.Attempt{RequestID: in.RequestID, RepositoryID: r.PathValue("id"), AssessmentSlug: d.Slug, AssessmentVersion: d.Version, WorkspaceID: ws.ID, LearnerID: actor.UserID, ProjectRevision: ws.CommitID, ReproducibilitySHA256: ws.LearningContext.ReproducibilitySHA256, WorkProductSHA256: workProduct, Evidence: in.Evidence, Accommodation: in.Accommodation, Blockers: blockers}, d.RetryPolicy.MaximumAttempts, d.RetryPolicy.CooldownHours)
		if e != nil {
			writeAPIError(w, 409, "learning_attempt_not_permitted", "retry limit or request identity prevents this attempt")
			return
		}
		writeJSON(w, 201, a)
	})
	mux.HandleFunc("POST /repositories/{id}/learning-assessments/{slug}/attempts/{attempt}/reviews", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !isOwner(r.PathValue("id"), actor.UserID) {
			writeAPIError(w, 403, "repository_owner_required", "only the repository owner can review assessments")
			return
		}
		var in struct {
			Decisions   []learningassessments.RubricDecision `json:"decisions"`
			Feedback    string                               `json:"feedback"`
			Uncertainty string                               `json:"uncertainty"`
			Outcome     string                               `json:"outcome"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		current, _ := assessments.Current(r.PathValue("id"), r.PathValue("slug"))
		definitions, _ := assessments.List(r.PathValue("id"), r.PathValue("slug"))
		a, e := assessments.UpdateAttempt(r.PathValue("id"), r.PathValue("slug"), r.PathValue("attempt"), func(a *learningassessments.Attempt) error {
			if a.AssessmentVersion < 1 || a.AssessmentVersion > len(definitions) {
				return learningassessments.ErrInvalid
			}
			bound := definitions[a.AssessmentVersion-1]
			blockers := append([]string{}, a.Blockers...)
			if a.AssessmentVersion != current.Version {
				blockers = append(blockers, "criteria_changed")
			}
			repo, _ := repos.GetByID(a.RepositoryID)
			gr, _ := git.Open(a.RepositoryID)
			if gr == nil {
				blockers = append(blockers, "project_revision_unavailable")
			} else if ref, e := gr.ReadReference("refs/heads/" + repo.DefaultBranch); e != nil || ref.Target != a.ProjectRevision {
				blockers = append(blockers, "stale_project_revision")
			}
			if len(in.Decisions) != len(bound.Criteria) {
				return learningassessments.ErrInvalid
			}
			seen := map[string]bool{}
			passed := in.Outcome == "passed"
			for _, d := range in.Decisions {
				seen[d.CriterionID] = true
				if d.Decision != "met" {
					passed = false
				}
			}
			for _, c := range bound.Criteria {
				if !seen[c.ID] {
					return learningassessments.ErrInvalid
				}
			}
			passedChecks := map[string]bool{}
			workspace, workspaceErr := workspaceStore.Get(a.WorkspaceID)
			citedCommands := map[string]bool{}
			if workspaceErr == nil {
				for _, outcome := range workspace.Commands {
					if learningAssessmentContains(a.Evidence.CommandOutcomeIDs, outcome.ID) && outcome.ActorID == a.LearnerID && outcome.ExitCode == 0 {
						citedCommands[outcome.CommandSHA256] = true
					}
				}
			}
			for _, ref := range a.Evidence.CheckRunIDs {
				parts := strings.Split(ref, "/")
				if len(parts) != 2 {
					blockers = append(blockers, "missing_repository_check")
					continue
				}
				run, e := checks.Get(a.RepositoryID, parts[0], parts[1])
				if e != nil || run.CommitID != a.ProjectRevision || run.State != "succeeded" {
					blockers = append(blockers, "repository_check_not_passing")
					continue
				}
				commandDigest := sha256.Sum256([]byte(run.Definition.Command))
				if !citedCommands[hex.EncodeToString(commandDigest[:])] {
					blockers = append(blockers, "repository_check_not_workspace_bound")
					continue
				}
				passedChecks[run.Definition.Name] = true
				for _, attempt := range run.Attempts {
					if attempt.State == "failed" {
						blockers = append(blockers, "flaky_repository_check")
					}
				}
			}
			for _, required := range bound.RequiredChecks {
				if !passedChecks[required] {
					blockers = append(blockers, "missing_repository_check")
				}
			}
			if len(blockers) > 0 {
				passed = false
			}
			outcome := "not_yet_demonstrated"
			if passed {
				outcome = "demonstrated"
			}
			a.Blockers = uniqueLearningAssessmentBlockers(blockers)
			a.Status = outcome
			a.Reviews = append(a.Reviews, learningassessments.Review{ID: randomAPIID(), ReviewerID: actor.UserID, Decisions: in.Decisions, Feedback: in.Feedback, Uncertainty: in.Uncertainty, Outcome: outcome})
			return nil
		})
		if e != nil {
			writeAPIError(w, 422, "invalid_learning_review", "every rubric criterion needs accountable judgment")
			return
		}
		writeJSON(w, 201, a)
	})
	mux.HandleFunc("POST /repositories/{id}/learning-assessments/{slug}/attempts/{attempt}/appeals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		var in struct {
			Body string `json:"body"`
		}
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Body) == "" {
			writeAPIError(w, 422, "invalid_appeal", "appeal reasoning is required")
			return
		}
		a, e := assessments.UpdateAttempt(r.PathValue("id"), r.PathValue("slug"), r.PathValue("attempt"), func(a *learningassessments.Attempt) error {
			if a.LearnerID != actor.UserID {
				return learningassessments.ErrInvalid
			}
			a.Appeals = append(a.Appeals, learningassessments.Appeal{ID: randomAPIID(), Body: strings.TrimSpace(in.Body), ActorID: actor.UserID})
			a.Status = "appealed"
			return nil
		})
		if e != nil {
			writeAPIError(w, 403, "appeal_not_permitted", "only the learner can appeal")
			return
		}
		writeJSON(w, 201, a)
	})
}
func evidenceExists(w workspaces.Workspace, e learningassessments.Evidence) bool {
	cp := map[string]bool{}
	for _, x := range w.LearningContext.Checkpoints {
		cp[x.ID] = true
	}
	co := map[string]bool{}
	for _, x := range w.Commands {
		co[x.ID] = x.ActorID == w.CreatorID
	}
	for _, x := range e.CheckpointIDs {
		if !cp[x] {
			return false
		}
	}
	for _, x := range e.CommandOutcomeIDs {
		if !co[x] {
			return false
		}
	}
	return len(e.CheckpointIDs)+len(e.CommandOutcomeIDs) > 0
}
func learningWorkProductDigest(w workspaces.Workspace, learner string) string {
	type product struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Size   int    `json:"size"`
	}
	products := []product{}
	for _, change := range w.Changes {
		if change.ActorID == learner {
			products = append(products, product{change.Path, change.SHA256, change.Size})
		}
	}
	if len(products) == 0 {
		return ""
	}
	sort.Slice(products, func(i, j int) bool { return products[i].Path < products[j].Path })
	body, _ := json.Marshal(products)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
func learningAssessmentContains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func uniqueLearningAssessmentBlockers(xs []string) []string {
	m := map[string]bool{}
	o := []string{}
	for _, x := range xs {
		if !m[x] {
			m[x] = true
			o = append(o, x)
		}
	}
	return o
}
