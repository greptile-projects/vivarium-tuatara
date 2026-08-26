package main

import (
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacitymodels"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacityobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacityplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacitytests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type capacityPlanInput struct {
	RequestID string             `json:"request_id"`
	Plan      capacityplans.Plan `json:"plan"`
}
type capacityDeliveryInput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Tasks []struct {
		PhaseID      string `json:"phase_id"`
		AssigneeType string `json:"assignee_type"`
		AssigneeID   string `json:"assignee_id"`
	} `json:"tasks"`
}

func registerCapacityPlanRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, objectives *capacityobjectives.Store, models *capacitymodels.Store, tests *capacitytests.Store, plans *capacityplans.Store, proposalStore *proposals.Store) {
	mux.HandleFunc("GET /repositories/{id}/capacity-plans", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		xs, e := plans.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "capacity_plans_unavailable", "capacity plans could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"capacity_plans": xs})
	})
	mux.HandleFunc("GET /repositories/{id}/capacity-plans/{plan_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		p, e := plans.Get(r.PathValue("id"), r.PathValue("plan_id"))
		if e != nil {
			writeAPIError(w, 404, "capacity_plan_not_found", "capacity plan not found")
			return
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("POST /repositories/{id}/capacity-plans", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in capacityPlanInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete phased capacity plan is required")
			return
		}
		o, e := objectives.Get(in.Plan.ObjectiveID)
		if e != nil || o.RepositoryID != r.PathValue("id") || in.Plan.ObjectiveVersion < 1 || in.Plan.ObjectiveVersion > len(o.Revisions) {
			writeAPIError(w, 422, "capacity_objective_invalid", "the exact capacity objective revision does not resolve")
			return
		}
		if in.Plan.ModelID != "" {
			m, x := models.Get(in.Plan.ModelID, c.UserID)
			if x != nil || m.RepositoryID != r.PathValue("id") || in.Plan.ModelVersion < 1 || in.Plan.ModelVersion > len(m.Revisions) {
				writeAPIError(w, 422, "capacity_model_invalid", "the exact capacity model revision does not resolve")
				return
			}
		}
		test, x := tests.Get(r.PathValue("id"), in.Plan.TestID)
		proven := false
		if x == nil && test.ObjectiveID == in.Plan.ObjectiveID && test.ObjectiveVersion == in.Plan.ObjectiveVersion && test.ModelID == in.Plan.ModelID && test.ModelVersion == in.Plan.ModelVersion {
			cmp, _ := tests.Compare(r.PathValue("id"), test.ID)
			for _, id := range cmp.ProvenCandidateIDs {
				proven = proven || id == in.Plan.CandidateID
			}
		}
		if x != nil || !proven {
			writeAPIError(w, 422, "capacity_choice_unsupported", "the selected candidate must be demonstrated by the exact bounded capacity test")
			return
		}
		owners := []string{c.UserID}
		for _, p := range in.Plan.Phases {
			owners = append(owners, p.OwnerID)
		}
		for _, d := range append(in.Plan.Reservations, in.Plan.Dependencies...) {
			owners = append(owners, d.OwnerID)
		}
		var out capacityplans.Plan
		e = catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error {
			var z error
			out, z = plans.Create(r.PathValue("id"), c.UserID, in.RequestID, in.Plan)
			return z
		})
		if e == nil {
			writeJSON(w, 201, out)
			return
		}
		if errors.Is(e, capacityplans.ErrConflict) {
			writeAPIError(w, 409, "capacity_plan_conflict", "request identity was reused with changed content")
			return
		}
		writeAPIError(w, 422, "capacity_plan_invalid", "phases require current owners, bounded budgets, order, acceptance criteria, decision points, and exit strategies")
	})
	mux.HandleFunc("POST /repositories/{id}/capacity-plans/{plan_id}/delivery", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		repo, repositoryErr := catalog.Get(c.UserID, r.PathValue("id"))
		if repositoryErr != nil {
			writeAPIError(w, 503, "repository_unavailable", "repository context is unavailable")
			return
		}
		p, e := plans.Get(repo.ID, r.PathValue("plan_id"))
		if e != nil {
			writeAPIError(w, 404, "capacity_plan_not_found", "capacity plan not found")
			return
		}
		var in capacityDeliveryInput
		if decodeJSON(r, &in) != nil || len(in.Tasks) != len(p.Phases) {
			writeAPIError(w, 400, "invalid_request", "one ordered ordinary task is required for every phase")
			return
		}
		bare, e := git.Open(repo.ID)
		if e != nil {
			writeAPIError(w, 503, "repository_unavailable", "repository context is unavailable")
			return
		}
		b, e := exec.Command("git", "--git-dir="+bare.Path(), "rev-parse", "refs/heads/"+repo.DefaultBranch).Output()
		liveRevision := strings.TrimSpace(string(b))
		if e != nil || len(liveRevision) != 40 {
			writeAPIError(w, 409, "delivery_base_unavailable", "the default branch has no exact delivery base")
			return
		}
		revision, recovering := capacityDeliveryBase(p, liveRevision)
		if recovering && exec.Command("git", "--git-dir="+bare.Path(), "cat-file", "-e", revision+"^{commit}").Run() != nil {
			writeAPIError(w, 409, "delivery_base_unavailable", "the retained delivery base is no longer available")
			return
		}
		tasks := make([]proposals.ImplementationTaskInput, len(p.Phases))
		proposalID, taskIDs := capacityplans.DeliveryIdentities(p)
		phasePositions := map[string]int{}
		for i, phase := range p.Phases {
			phasePositions[phase.ID] = i
		}
		items := make([]proposals.ReasoningItem, len(p.Phases))
		participants := []string{c.UserID}
		for i, phase := range p.Phases {
			if in.Tasks[i].PhaseID != phase.ID || in.Tasks[i].AssigneeID != phase.OwnerID {
				writeAPIError(w, 422, "capacity_delivery_invalid", "tasks must preserve phase order and accountable owner")
				return
			}
			dependencyIDs := make([]string, len(phase.DependsOn))
			for j, dependencyPhaseID := range phase.DependsOn {
				dependencyIDs[j] = taskIDs[phasePositions[dependencyPhaseID]]
			}
			tasks[i] = proposals.ImplementationTaskInput{ID: taskIDs[i], Title: phase.Name, Outcome: strings.Join(phase.AcceptanceCriteria, "; "), Risk: "Decision point: " + phase.DecisionPoint + " Exit strategy: " + phase.ExitStrategy, VerificationPlan: "Prove the phase criteria through ordinary checks, reviews, approvals, queues, release, and environment controls.", AssigneeType: in.Tasks[i].AssigneeType, AssigneeID: in.Tasks[i].AssigneeID, DependencyIDs: dependencyIDs}
			items[i] = proposals.ReasoningItem{ID: phase.ID, Kind: "capacity_phase", Summary: fmt.Sprintf("%s (%g %s)", phase.Name, phase.Budget, phase.Currency), Status: "approved"}
			if in.Tasks[i].AssigneeType == "human" {
				participants = append(participants, in.Tasks[i].AssigneeID)
			}
		}
		origin := proposals.ReasoningOrigin{CapacityPlanID: p.ID, Revision: revision, Items: items, AnalysisStatus: "approved_capacity_plan"}
		for _, x := range items {
			origin.SelectedItemIDs = append(origin.SelectedItemIDs, x.ID)
		}
		var proposal proposals.Proposal
		var made []proposals.Task
		publish := func() error {
			delivery := capacityplans.Delivery{ProposalID: proposalID, TaskIDs: taskIDs, BaseRevision: revision}
			if _, reserveErr := plans.ReserveDelivery(repo.ID, p.ID, delivery); reserveErr != nil {
				return reserveErr
			}
			var x error
			proposal, made, x = proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: repo.ID, ActorID: c.UserID, ProposalID: proposalID, Title: in.Title, Body: in.Body, Origin: origin, Tasks: tasks})
			if x != nil {
				return x
			}
			_, x = plans.FinalizeDelivery(repo.ID, p.ID, delivery)
			return x
		}
		e = catalog.WithCurrentParticipants(participants, repo.ID, func() error {
			if recovering {
				// The first attempt retained this exact base while holding the Git
				// reference lock. Recovery must finish that publication even when
				// later, independently authorized work advances the branch.
				return publish()
			}
			return bare.WithReferenceTarget("refs/heads/"+repo.DefaultBranch, revision, publish)
		})
		if e != nil {
			writeAPIError(w, 422, "capacity_delivery_invalid", "delivery owners and exact base must remain current; plan approval grants no delivery authority")
			return
		}
		writeJSON(w, 201, map[string]any{"capacity_plan": func() capacityplans.Plan { x, _ := plans.Get(repo.ID, p.ID); return x }(), "proposal": proposal, "tasks": made})
	})
}

func capacityDeliveryBase(plan capacityplans.Plan, liveRevision string) (string, bool) {
	if plan.Delivery != nil && (plan.Delivery.Status == "pending" || plan.Delivery.Status == "created") {
		return plan.Delivery.BaseRevision, true
	}
	return liveRevision, false
}
