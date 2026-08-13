package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	productfeedback "github.com/greptile-projects/vivarium-tuatara/apps/api/feedback"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productopportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/roadmaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type roadmapMutation struct {
	ExpectedVersion int               `json:"expected_version"`
	Revision        roadmaps.Revision `json:"revision"`
	Rationale       string            `json:"rationale"`
	Body            string            `json:"body"`
}

type roadmapTaskInput struct {
	Title             string `json:"title"`
	AssigneeType      string `json:"assignee_type"`
	AssigneeID        string `json:"assignee_id"`
	MeasureIndexes    []int  `json:"measure_indexes"`
	DependsOnPrevious bool   `json:"depends_on_previous"`
}
type roadmapImplementationInput struct {
	ExpectedVersion int                `json:"expected_version"`
	RoadmapVersion  int                `json:"roadmap_version"`
	ItemID          string             `json:"item_id"`
	Title           string             `json:"title"`
	Body            string             `json:"body"`
	Tasks           []roadmapTaskInput `json:"tasks"`
}
type roadmapOutcomeInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Kind            string `json:"kind"`
	Summary         string `json:"summary"`
	ResourceKind    string `json:"resource_kind"`
	ResourceID      string `json:"resource_id"`
	MeasureIndexes  []int  `json:"measure_indexes"`
}
type roadmapLearningInput struct {
	ExpectedVersion   int      `json:"expected_version"`
	OpportunityID     string   `json:"opportunity_id"`
	Kind              string   `json:"kind"`
	Summary           string   `json:"summary"`
	Rationale         string   `json:"rationale"`
	FeedbackIDs       []string `json:"feedback_ids"`
	ResourceKind      string   `json:"resource_kind"`
	ResourceID        string   `json:"resource_id"`
	UpdateID          string   `json:"update_id"`
	FeedbackID        string   `json:"feedback_id"`
	Assessment        string   `json:"assessment"`
	FollowUp          string   `json:"follow_up"`
	LeaveConversation bool     `json:"leave_conversation"`
	Promised          []string `json:"promised"`
	Observed          []string `json:"observed"`
	Lessons           []string `json:"lessons"`
	Dissent           []string `json:"dissent"`
	Disposition       string   `json:"disposition"`
	ResultingWorkIDs  []string `json:"resulting_work_ids"`
}

func registerRoadmapRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, credentials *auth.Store, store *roadmaps.Store, opportunities *productopportunities.Store, feedbackStore *productfeedback.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, checkStore *checkruns.Store, releaseStore *releases.Store, deploymentStore *deployments.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request) (auth.Credential, repositories.Repository, bool, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return actor, repositories.Repository{}, false, false
		}
		repo, e := repos.GetByID(r.PathValue("id"))
		if e != nil {
			return actor, repo, false, false
		}
		// Public reads intentionally do not authenticate in the shared read helper.
		// Recover an optional presented identity so mutations remain attributable.
		_, cookieErr := r.Cookie("vivarium_session")
		if actor.UserID == "" && actor.AgentID == "" && (r.Header.Get("Authorization") != "" || cookieErr == nil) {
			var authenticated bool
			actor, authenticated, ok = authenticateOptionalRequest(w, r, credentials, "repositories:read", false)
			if !ok || !authenticated {
				return auth.Credential{}, repo, false, false
			}
		}
		participant := actor.UserID == repo.OwnerID
		if !participant {
			participant, _ = repos.HasCollaborator(actor.UserID, repo.ID)
		}
		return actor, repo, participant, true
	}
	validate := func(repo string, r roadmaps.Revision) bool {
		for _, d := range r.Decisions {
			x, e := opportunities.Get(repo, d.OpportunityID)
			found := false
			if e == nil {
				for _, revision := range x.Revisions {
					found = found || revision.Version == d.Version
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	requireIdentity := func(w http.ResponseWriter, actor auth.Credential) bool {
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /repositories/{id}/roadmap", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		v, e := store.Get(repo.ID)
		if e == nil && !participant {
			visible := map[string]bool{}
			if actor.UserID != "" && feedbackStore != nil {
				for _, f := range v.LearningUpdates {
					for _, fid := range f.FeedbackIDs {
						if x, err := feedbackStore.Get(fid); err == nil && x.ReporterID == actor.UserID {
							visible[f.ID] = true
						}
					}
				}
			}
			updates := v.LearningUpdates[:0]
			for _, x := range v.LearningUpdates {
				if visible[x.ID] {
					x.FeedbackIDs = nil
					updates = append(updates, x)
				}
			}
			v.LearningUpdates = updates
			responses := v.LearningResponses[:0]
			for _, x := range v.LearningResponses {
				if x.ActorID == actor.UserID && visible[x.UpdateID] {
					responses = append(responses, x)
				}
			}
			v.LearningResponses = responses
			v.LearningReviews = nil
		}
		writeRoadmap(w, v, e, 200)
	})
	mux.HandleFunc("PUT /repositories/{id}/roadmap", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant || actor.AgentID != "" {
			writeAPIError(w, 403, "roadmap_commit_forbidden", "only repository maintainers may commit project resources")
			return
		}
		var in roadmapMutation
		if decodeJSON(r, &in) != nil || !validate(repo.ID, in.Revision) {
			writeAPIError(w, 400, "invalid_roadmap_evidence", "every opportunity must exist at the exact compared version")
			return
		}
		v, e := store.Publish(repo.ID, actor.UserID, in.ExpectedVersion, in.Revision)
		writeRoadmap(w, v, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/roadmap/scenarios", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, _, ok := authorize(w, r)
		if !ok || !requireIdentity(w, actor) {
			return
		}
		var in roadmapMutation
		if decodeJSON(r, &in) != nil || !validate(repo.ID, in.Revision) {
			writeAPIError(w, 400, "invalid_roadmap_scenario", "scenario opportunity versions must be exact")
			return
		}
		kind, id := "human", actor.UserID
		if actor.AgentID != "" {
			kind, id = "agent", actor.AgentID
		}
		v, e := store.Propose(repo.ID, id, kind, in.ExpectedVersion, in.Revision, in.Rationale)
		writeRoadmap(w, v, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/roadmap/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, _, ok := authorize(w, r)
		if !ok || !requireIdentity(w, actor) {
			return
		}
		var in roadmapMutation
		if decodeJSON(r, &in) != nil {
			return
		}
		kind, id := "human", actor.UserID
		if actor.AgentID != "" {
			kind, id = "agent", actor.AgentID
		}
		v, e := store.Comment(repo.ID, id, kind, in.ExpectedVersion, in.Body)
		writeRoadmap(w, v, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/roadmap/implementations", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant || actor.AgentID != "" {
			writeAPIError(w, 403, "roadmap_implementation_forbidden", "only repository participants may plan accepted outcomes")
			return
		}
		var in roadmapImplementationInput
		if decodeJSON(r, &in) != nil || len(in.Tasks) == 0 || len(in.Tasks) > 20 || proposalStore == nil || git == nil {
			writeAPIError(w, 400, "invalid_implementation", "an accepted roadmap item and ordered tasks are required")
			return
		}
		v, e := store.Get(repo.ID)
		if e != nil || v.Version != in.ExpectedVersion {
			writeRoadmap(w, v, roadmaps.ErrConflict, 0)
			return
		}
		var item *roadmaps.Item
		for _, rev := range v.Revisions {
			if rev.Version == in.RoadmapVersion {
				for i := range rev.Items {
					if rev.Items[i].ID == in.ItemID {
						item = &rev.Items[i]
					}
				}
			}
		}
		if item == nil {
			writeAPIError(w, 409, "roadmap_item_changed", "implementation requires the exact accepted roadmap item")
			return
		}
		bare, e := git.Open(repo.ID)
		if e != nil {
			writeAPIError(w, 503, "repository_unavailable", "repository context is unavailable")
			return
		}
		out, e := exec.Command("git", "--git-dir="+bare.Path(), "rev-parse", "refs/heads/"+repo.DefaultBranch).Output()
		revision := strings.TrimSpace(string(out))
		if e != nil || len(revision) != 40 {
			writeAPIError(w, 409, "implementation_base_unavailable", "the default branch has no exact implementation base")
			return
		}
		covered := make([]bool, len(item.SuccessMeasures))
		tasks := make([]proposals.ImplementationTaskInput, 0, len(in.Tasks))
		participants := []string{actor.UserID}
		reasoning := []proposals.ReasoningItem{{ID: "need", Kind: "product_need", Summary: "Opportunity " + item.OpportunityID + " earned roadmap priority", Status: "required"}}
		for i, m := range item.SuccessMeasures {
			reasoning = append(reasoning, proposals.ReasoningItem{ID: fmt.Sprintf("measure-%d", i), Kind: "roadmap_success_measure", Summary: m, Status: "required"})
		}
		for _, t := range in.Tasks {
			measures := []string{}
			for _, n := range t.MeasureIndexes {
				if n < 0 || n >= len(covered) {
					writeAPIError(w, 400, "invalid_measure_coverage", "task measure coverage is outside the accepted outcome")
					return
				}
				covered[n] = true
				measures = append(measures, item.SuccessMeasures[n])
			}
			if len(measures) == 0 {
				writeAPIError(w, 400, "invalid_measure_coverage", "every task must trace to a success measure")
				return
			}
			tasks = append(tasks, proposals.ImplementationTaskInput{Title: t.Title, Outcome: "Advance: " + item.Title, VerificationPlan: "Measure: " + strings.Join(measures, "; "), Risk: "Changed assumptions, unresolved needs, conflicts, or failed measures require decision revisit.", AssigneeType: t.AssigneeType, AssigneeID: t.AssigneeID, DependsOnPrevious: t.DependsOnPrevious})
			if t.AssigneeType == "human" {
				participants = append(participants, t.AssigneeID)
			}
		}
		for _, x := range covered {
			if !x {
				writeAPIError(w, 400, "incomplete_measure_coverage", "the plan must cover every roadmap success measure")
				return
			}
		}
		origin := proposals.ReasoningOrigin{RoadmapItemID: item.ID, RoadmapVersion: in.RoadmapVersion, OpportunityID: item.OpportunityID, Revision: revision, Items: reasoning, AnalysisStatus: "accepted_roadmap_outcome"}
		for _, x := range reasoning {
			origin.SelectedItemIDs = append(origin.SelectedItemIDs, x.ID)
		}
		var p proposals.Proposal
		var made []proposals.Task
		create := func() error {
			p, made, e = proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: repo.ID, ActorID: actor.UserID, Title: in.Title, Body: in.Body, Origin: origin, Tasks: tasks})
			return e
		}
		e = repos.WithCurrentParticipants(participants, repo.ID, func() error { return bare.WithReferenceTarget("refs/heads/"+repo.DefaultBranch, revision, create) })
		if e != nil {
			writeAPIError(w, 422, "implementation_invalid", "owners and the exact implementation plan must remain valid")
			return
		}
		ids := []string{}
		for _, t := range made {
			ids = append(ids, t.ID)
		}
		updated, e := store.LinkImplementation(repo.ID, actor.UserID, in.ExpectedVersion, in.RoadmapVersion, item.ID, item.OpportunityID, p.ID, revision, ids)
		// Proposal creation commits before the cross-store roadmap link. Reconcile
		// a concurrent roadmap mutation when this frozen historical item is still
		// valid so otherwise valid work is not stranded.
		if errors.Is(e, roadmaps.ErrConflict) {
			if latest, getErr := store.Get(repo.ID); getErr == nil {
				updated, e = store.LinkImplementation(repo.ID, actor.UserID, latest.Version, in.RoadmapVersion, item.ID, item.OpportunityID, p.ID, revision, ids)
			}
		}
		if e != nil {
			w.Header().Set("Vivarium-Recovery-Implementation", "pending")
			writeJSON(w, 202, map[string]any{"roadmap": updated, "proposal": p, "tasks": made, "recovery_pending": true})
			return
		}
		writeJSON(w, 201, map[string]any{"roadmap": updated, "proposal": p, "tasks": made})
	})
	mux.HandleFunc("POST /repositories/{id}/roadmap/implementations/{proposal_id}/outcomes", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant || actor.AgentID != "" {
			writeAPIError(w, 403, "roadmap_outcome_forbidden", "only repository participants may assess roadmap outcomes")
			return
		}
		var in roadmapOutcomeInput
		if decodeJSON(r, &in) != nil || !validDecisionDeliveryResource(repo.ID, r.PathValue("proposal_id"), mapRoadmapEvidenceKind(in.Kind), in.ResourceKind, in.ResourceID, proposalStore, pullStore, checkStore, releaseStore, deploymentStore) {
			writeAPIError(w, 422, "roadmap_evidence_invalid", "outcome evidence must be retained delivery or measurement evidence linked to this implementation")
			return
		}
		v, e := store.ReportOutcome(repo.ID, actor.UserID, in.ExpectedVersion, r.PathValue("proposal_id"), roadmaps.DeliveryEvidence{Kind: in.Kind, Summary: in.Summary, ResourceKind: in.ResourceKind, ResourceID: in.ResourceID, MeasureIndexes: in.MeasureIndexes})
		writeRoadmap(w, v, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/roadmap/learning-updates", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant || actor.AgentID != "" {
			writeAPIError(w, 403, "learning_update_forbidden", "only repository participants may publish stakeholder updates")
			return
		}
		var in roadmapLearningInput
		if decodeJSON(r, &in) != nil {
			return
		}
		for _, fid := range in.FeedbackIDs {
			x, e := feedbackStore.Get(fid)
			if e != nil || x.RepositoryID != repo.ID {
				writeAPIError(w, 422, "learning_audience_invalid", "updates may cite only feedback in this repository")
				return
			}
		}
		v, e := store.PublishLearningUpdate(repo.ID, actor.UserID, in.ExpectedVersion, roadmaps.LearningUpdate{OpportunityID: in.OpportunityID, Kind: in.Kind, Summary: in.Summary, Rationale: in.Rationale, FeedbackIDs: in.FeedbackIDs, ResourceKind: in.ResourceKind, ResourceID: in.ResourceID})
		writeRoadmap(w, v, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/roadmap/learning-responses", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, _, ok := authorize(w, r)
		if !ok || !requireIdentity(w, actor) {
			return
		}
		var in roadmapLearningInput
		if decodeJSON(r, &in) != nil {
			return
		}
		f, e := feedbackStore.Get(in.FeedbackID)
		if e != nil || f.RepositoryID != repo.ID || f.ReporterID != actor.UserID {
			writeAPIError(w, 403, "learning_response_forbidden", "only the cited feedback reporter may respond")
			return
		}
		v, e := store.RespondToLearning(repo.ID, actor.UserID, in.ExpectedVersion, roadmaps.LearningResponse{UpdateID: in.UpdateID, FeedbackID: in.FeedbackID, Assessment: in.Assessment, FollowUp: in.FollowUp, LeaveConversation: in.LeaveConversation})
		writeRoadmap(w, v, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/roadmap/learning-reviews", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant || actor.AgentID != "" {
			writeAPIError(w, 403, "learning_review_forbidden", "only repository participants may retain product lessons")
			return
		}
		var in roadmapLearningInput
		if decodeJSON(r, &in) != nil {
			return
		}
		v, e := store.RecordLearningReview(repo.ID, actor.UserID, in.ExpectedVersion, roadmaps.LearningReview{OpportunityID: in.OpportunityID, Promised: in.Promised, Observed: in.Observed, Lessons: in.Lessons, Dissent: in.Dissent, Disposition: in.Disposition, Rationale: in.Rationale, ResultingWorkIDs: in.ResultingWorkIDs})
		writeRoadmap(w, v, e, 201)
	})
}
func mapRoadmapEvidenceKind(kind string) string {
	if kind == "measure_met" {
		return "coverage"
	}
	return "failed_measure"
}
func writeRoadmap(w http.ResponseWriter, v roadmaps.Roadmap, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, roadmaps.ErrNotFound):
		writeAPIError(w, 404, "roadmap_not_found", "roadmap not found")
	case errors.Is(e, roadmaps.ErrConflict):
		writeAPIError(w, 409, "roadmap_changed", "roadmap changed; refresh and submit an attributed replan")
	case errors.Is(e, roadmaps.ErrInvalid):
		writeAPIError(w, 400, "invalid_roadmap", "complete comparisons, accountable outcomes, sequencing, and an explicit replan reason are required")
	default:
		log.Printf("roadmap storage: %v", e)
		writeAPIError(w, 500, "roadmap_unavailable", "roadmap could not be persisted")
	}
}
