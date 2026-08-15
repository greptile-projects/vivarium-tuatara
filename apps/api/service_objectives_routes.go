package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type serviceObjectiveInput struct {
	ExpectedVersion int                        `json:"expected_version"`
	Revision        serviceobjectives.Revision `json:"revision"`
}
type signalMappingInput struct {
	ExpectedVersion int                                     `json:"expected_version"`
	Revision        serviceobjectives.SignalMappingRevision `json:"revision"`
}
type observationInput struct {
	Observation serviceobjectives.Observation `json:"observation"`
}
type investigationMutationInput struct {
	ExpectedVersion int                                     `json:"expected_version"`
	Finding         serviceobjectives.InvestigationFinding  `json:"finding"`
	Response        serviceobjectives.InvestigationResponse `json:"response"`
	Request         serviceobjectives.InputRequest          `json:"request"`
	Outcome         *serviceobjectives.InvestigationOutcome `json:"outcome"`
}
type deliveryPolicyInput struct {
	ExpectedVersion int                              `json:"expected_version"`
	Policy          serviceobjectives.DeliveryPolicy `json:"policy"`
}
type reliabilityImpactInput struct {
	Impact serviceobjectives.ReliabilityImpact `json:"impact"`
}
type reliabilityAcknowledgementInput struct {
	Rationale string `json:"rationale"`
}
type reliabilityExceptionInput struct {
	Exception serviceobjectives.DeliveryException `json:"exception"`
}
type reliabilityTaskInput struct {
	Title             string `json:"title"`
	AssigneeType      string `json:"assignee_type"`
	AssigneeID        string `json:"assignee_id"`
	DependsOnPrevious bool   `json:"depends_on_previous"`
}
type reliabilityImprovementInput struct {
	ContractVersion        int                    `json:"contract_version"`
	ObjectiveID            string                 `json:"objective_id"`
	InvestigationID        string                 `json:"investigation_id"`
	FindingID              string                 `json:"finding_id"`
	ImpactID               string                 `json:"impact_id"`
	Title                  string                 `json:"title"`
	Body                   string                 `json:"body"`
	BaselineObservationIDs []string               `json:"baseline_observation_ids"`
	AffectedObservationIDs []string               `json:"affected_observation_ids"`
	AffectedRevisions      []string               `json:"affected_revisions"`
	DependencyContext      []string               `json:"dependency_context"`
	EvidenceIDs            []string               `json:"evidence_ids"`
	AcceptanceCriteria     []string               `json:"acceptance_criteria"`
	Tasks                  []reliabilityTaskInput `json:"tasks"`
}
type reliabilityVerificationInput struct {
	Verification serviceobjectives.RolloutVerification `json:"verification"`
}

func registerServiceObjectiveRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, contracts *serviceobjectives.Store, pulls *pullrequests.Store, deploymentStore *deployments.Store, releaseStore *releases.Store, proposalStore *proposals.Store) {
	investigator := func(w http.ResponseWriter, r *http.Request) (auth.Credential, string, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return actor, "", false
		}
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return actor, "", false
		}
		typ := "human"
		if actor.AgentID != "" {
			typ = "agent"
		} else {
			repo, err := catalog.GetByID(r.PathValue("id"))
			participant := err == nil && repo.OwnerID == actor.UserID
			if !participant && err == nil {
				participant, _ = catalog.HasCollaborator(actor.UserID, repo.ID)
			}
			if !participant {
				writeAPIError(w, 403, "reliability_investigation_forbidden", "only repository participants and repository-bound read-only agents may investigate reliability")
				return actor, "", false
			}
		}
		return actor, typ, true
	}
	mux.HandleFunc("GET /repositories/{id}/service-objectives", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, err := contracts.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "service_objectives_unavailable", "service objectives could not be read")
			return
		}
		participant := serviceObjectiveReaderParticipant(r, catalog, credentials, r.PathValue("id"))
		for i := range values {
			values[i] = contracts.ProjectForReader(values[i], participant)
		}
		writeJSON(w, 200, map[string]any{"service_objectives": values})
	})
	mux.HandleFunc("GET /repositories/{id}/service-objectives/{objective_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		writeJSON(w, 200, contracts.ProjectForReader(out, serviceObjectiveReaderParticipant(r, catalog, credentials, out.RepositoryID)))
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/delivery-policies", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "reliability_policy_owner_required", "only the repository owner may publish delivery policy")
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in deliveryPolicyInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete reliability delivery policy are required")
			return
		}
		out, err := contracts.PublishDeliveryPolicy(current.ID, actor.UserID, in.ExpectedVersion, in.Policy)
		writeReliabilityDelivery(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/reliability-impacts", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in reliabilityImpactInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a revision-exact reliability impact is required")
			return
		}
		out, err := contracts.RecordReliabilityImpact(current.ID, actor.UserID, in.Impact)
		writeReliabilityDelivery(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/reliability-impacts/{impact_id}/acknowledgements", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in reliabilityAcknowledgementInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an owner rationale is required")
			return
		}
		out, err := contracts.AcknowledgeReliabilityImpact(current.ID, r.PathValue("impact_id"), actor.UserID, in.Rationale)
		writeReliabilityDelivery(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/reliability-impacts/{impact_id}/exceptions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in reliabilityExceptionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded reliability exception is required")
			return
		}
		out, err := contracts.ExceptReliabilityImpact(current.ID, r.PathValue("impact_id"), actor.UserID, in.Exception)
		writeReliabilityDelivery(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/reliability-readiness/{kind}/{resource_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		q := r.URL.Query()
		out, err := contracts.EvaluateReliability(r.PathValue("id"), r.PathValue("kind"), r.PathValue("resource_id"), q.Get("revision"), q.Get("branch"), q.Get("service"), q.Get("environment"), q["journey"], q["risk"])
		if err != nil {
			writeAPIError(w, 500, "reliability_delivery_unavailable", "reliability readiness could not be evaluated")
			return
		}
		writeJSON(w, 200, map[string]any{"evaluations": out})
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in serviceObjectiveInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete service objective revision is required")
			return
		}
		var out serviceobjectives.Contract
		err := catalog.WithCurrentParticipants(serviceObjectiveParticipants(actor.UserID, in.Revision), r.PathValue("id"), func() error {
			var e error
			out, e = contracts.Create(r.PathValue("id"), actor.UserID, in.Revision)
			return e
		})
		writeServiceObjective(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in serviceObjectiveInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete service objective revision are required")
			return
		}
		var out serviceobjectives.Contract
		err = catalog.WithCurrentParticipants(serviceObjectiveParticipants(actor.UserID, in.Revision), current.RepositoryID, func() error {
			var e error
			out, e = contracts.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
			return e
		})
		writeServiceObjective(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/signal-mappings", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in signalMappingInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete sanitized signal mapping is required")
			return
		}
		var out serviceobjectives.Contract
		err = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error {
			var e error
			out, e = contracts.PublishMapping(current.ID, actor.UserID, in.Revision)
			return e
		})
		writeReliabilityEvidence(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/signal-mappings/{mapping_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in signalMappingInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete sanitized signal mapping are required")
			return
		}
		var out serviceobjectives.Contract
		err = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error {
			var e error
			out, e = contracts.ReviseMapping(current.ID, r.PathValue("mapping_id"), in.ExpectedVersion, actor.UserID, in.Revision)
			return e
		})
		writeReliabilityEvidence(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/observations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in observationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete sanitized observation is required")
			return
		}
		var out serviceobjectives.Contract
		err = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error {
			var e error
			out, e = contracts.RecordObservation(current.ID, actor.UserID, in.Observation)
			return e
		})
		writeReliabilityEvidence(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/investigations", func(w http.ResponseWriter, r *http.Request) {
		actor, typ, ok := investigator(w, r)
		if !ok {
			return
		}
		current, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in serviceobjectives.Investigation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a revision-bound investigation is required")
			return
		}
		if !reliabilityInvestigationProvenanceResolves(git, pulls, deploymentStore, releaseStore, current, in) {
			writeAPIError(w, 422, "invalid_reliability_provenance", "the trigger and every evidence reference must resolve at its exact revision in this repository")
			return
		}
		id := actor.UserID
		if typ == "agent" {
			id = actor.AgentID
		}
		out, err := contracts.OpenInvestigation(current.ID, id, in)
		writeReliabilityInvestigation(w, out, err, 201)
	})
	for path, action := range map[string]string{"/findings": "finding", "/responses": "response", "/input-requests": "request", "/input-responses": "reply", "/outcomes": "outcome"} {
		mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/investigations/{investigation_id}"+path, func(w http.ResponseWriter, r *http.Request) {
			actor, typ, ok := investigator(w, r)
			if !ok {
				return
			}
			current, err := contracts.Get(r.PathValue("objective_id"))
			if err != nil || current.RepositoryID != r.PathValue("id") {
				writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
				return
			}
			var in investigationMutationInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "expected_version and a complete investigation entry are required")
				return
			}
			id := actor.UserID
			if typ == "agent" {
				id = actor.AgentID
			}
			if action != "finding" && typ == "agent" {
				writeAPIError(w, 403, "reliability_investigation_human_required", "read-only agents may add cited findings but only human participants may respond, request owner input, answer, or conclude")
				return
			}
			out, err := contracts.MutateInvestigation(current.ID, r.PathValue("investigation_id"), id, typ, action, in.ExpectedVersion, in.Finding, in.Response, in.Request, in.Outcome)
			writeReliabilityInvestigation(w, out, err, 201)
		})
	}
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/improvements", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in reliabilityImprovementInput
		if decodeJSON(r, &in) != nil || proposalStore == nil || len(in.Tasks) == 0 || len(in.Tasks) > 20 {
			writeAPIError(w, 400, "invalid_reliability_improvement", "a cited source, acceptance criteria, and ordered owned tasks are required")
			return
		}
		contract, err := contracts.Get(r.PathValue("objective_id"))
		if err != nil || contract.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		validation := serviceobjectives.Improvement{ContractVersion: in.ContractVersion, ObjectiveID: in.ObjectiveID, InvestigationID: in.InvestigationID, FindingID: in.FindingID, ImpactID: in.ImpactID, BaselineObservationIDs: in.BaselineObservationIDs, AffectedObservationIDs: in.AffectedObservationIDs, AffectedRevisions: in.AffectedRevisions, DependencyContext: in.DependencyContext, EvidenceIDs: in.EvidenceIDs, AcceptanceCriteria: in.AcceptanceCriteria, ProposalID: "pending", TaskIDs: []string{"pending"}}
		if contracts.ValidateImprovement(contract.ID, validation) != nil {
			writeAPIError(w, 422, "reliability_improvement_invalid", "the exact objective, finding or impact, observations, evidence, and criteria must remain valid")
			return
		}
		bare, err := git.Open(contract.RepositoryID)
		if err != nil {
			writeAPIError(w, 503, "repository_unavailable", "repository context is unavailable")
			return
		}
		repo, err := catalog.GetByID(contract.RepositoryID)
		if err != nil {
			writeAPIError(w, 503, "repository_unavailable", "repository context is unavailable")
			return
		}
		data, err := exec.Command("git", "--git-dir="+bare.Path(), "rev-parse", "refs/heads/"+repo.DefaultBranch).Output()
		revision := strings.TrimSpace(string(data))
		if err != nil || len(revision) != 40 {
			writeAPIError(w, 409, "improvement_base_unavailable", "the default branch has no exact implementation base")
			return
		}
		items := []proposals.ReasoningItem{{ID: "objective", Kind: "reliability_objective", Summary: in.ObjectiveID, Status: "required"}}
		for i, x := range in.AffectedRevisions {
			items = append(items, proposals.ReasoningItem{ID: fmt.Sprintf("revision-%d", i), Kind: "affected_revision", Summary: x, Status: "affected"})
		}
		for i, x := range in.DependencyContext {
			items = append(items, proposals.ReasoningItem{ID: fmt.Sprintf("dependency-%d", i), Kind: "dependency_context", Summary: x, Status: "required"})
		}
		for i, x := range in.EvidenceIDs {
			items = append(items, proposals.ReasoningItem{ID: fmt.Sprintf("evidence-%d", i), Kind: "reliability_evidence", Summary: x, Status: "cited"})
		}
		for i, x := range in.AcceptanceCriteria {
			items = append(items, proposals.ReasoningItem{ID: fmt.Sprintf("criterion-%d", i), Kind: "acceptance_criterion", Summary: x, Status: "required"})
		}
		origin := proposals.ReasoningOrigin{ReliabilityContractID: contract.ID, ReliabilityInvestigationID: in.InvestigationID, ReliabilityFindingID: in.FindingID, ReliabilityImpactID: in.ImpactID, Revision: revision, Items: items, AnalysisStatus: "reliability_improvement"}
		for _, x := range items {
			origin.SelectedItemIDs = append(origin.SelectedItemIDs, x.ID)
		}
		tasks, participants := []proposals.ImplementationTaskInput{}, []string{actor.UserID}
		for _, x := range in.Tasks {
			tasks = append(tasks, proposals.ImplementationTaskInput{Title: x.Title, Outcome: "Improve objective " + in.ObjectiveID + ": " + strings.Join(in.AcceptanceCriteria, "; "), VerificationPlan: "Compare rollout signals with baseline observations " + strings.Join(in.BaselineObservationIDs, ", "), Risk: "Failed measures require containment, rollback, or decision revisit.", AssigneeType: x.AssigneeType, AssigneeID: x.AssigneeID, DependsOnPrevious: x.DependsOnPrevious})
			if x.AssigneeType == "human" {
				participants = append(participants, x.AssigneeID)
			}
		}
		var p proposals.Proposal
		var made []proposals.Task
		var linked serviceobjectives.Contract
		var reservation serviceobjectives.Improvement
		err = catalog.WithCurrentParticipants(participants, contract.RepositoryID, func() error {
			return bare.WithReferenceTarget("refs/heads/"+repo.DefaultBranch, revision, func() error {
				var e error
				_, reservation, e = contracts.ReserveImprovement(contract.ID, actor.UserID, validation)
				if e != nil {
					return e
				}
				if reservation.Status == "linked" {
					linked, e = contracts.Get(contract.ID)
					return e
				}
				p, made, e = proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: contract.RepositoryID, ActorID: actor.UserID, Title: in.Title, Body: in.Body, Origin: origin, Tasks: tasks})
				if e != nil {
					return e
				}
				taskIDs := make([]string, 0, len(made))
				for _, t := range made {
					taskIDs = append(taskIDs, t.ID)
				}
				linked, e = contracts.CompleteImprovement(contract.ID, reservation.ID, actor.UserID, p.ID, taskIDs)
				return e
			})
		})
		if err != nil {
			writeAPIError(w, 422, "reliability_improvement_invalid", "authority, source evidence, assignments, or implementation base changed")
			return
		}
		writeReliabilityDelivery(w, linked, nil, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/service-objectives/{objective_id}/improvements/{improvement_id}/verifications", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "reliability_rollout_owner_required", "only the repository owner may record a governed rollout decision")
			return
		}
		contract, getErr := contracts.Get(r.PathValue("objective_id"))
		if getErr != nil || contract.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "service_objective_not_found", "service objective not found")
			return
		}
		var in reliabilityVerificationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_reliability_verification", "an exact rollout comparison and decision are required")
			return
		}
		in.Verification.ImprovementID = r.PathValue("improvement_id")
		validResource := false
		if in.Verification.Kind == "release" && releaseStore != nil {
			value, e := releaseStore.Get(contract.RepositoryID, in.Verification.ResourceID)
			validResource = e == nil && value.CommitID == in.Verification.Revision
		}
		if in.Verification.Kind == "deployment" && deploymentStore != nil {
			value, e := deploymentStore.GetPromotion(contract.RepositoryID, in.Verification.ResourceID)
			validResource = e == nil && value.CommitID == in.Verification.Revision
		}
		if !validResource {
			writeAPIError(w, 422, "reliability_rollout_unresolved", "the rollout resource must resolve at its exact repository revision")
			return
		}
		out, err := contracts.VerifyImprovement(contract.ID, actor.UserID, in.Verification)
		writeReliabilityDelivery(w, out, err, 201)
	})
}

func reliabilityInvestigationProvenanceResolves(git *storage.Store, pulls *pullrequests.Store, deploymentStore *deployments.Store, releaseStore *releases.Store, contract serviceobjectives.Contract, in serviceobjectives.Investigation) bool {
	repo := contract.RepositoryID
	resolve := func(kind, resourceID, revision string) bool {
		switch kind {
		case "pull_request":
			if pulls == nil {
				return false
			}
			v, err := pulls.Get(repo, resourceID)
			return err == nil && v.SourceCommitID == revision
		case "deployment":
			if deploymentStore == nil {
				return false
			}
			v, err := deploymentStore.GetPromotion(repo, resourceID)
			return err == nil && v.CommitID == revision
		case "release":
			if releaseStore == nil {
				return false
			}
			v, err := releaseStore.Get(repo, resourceID)
			return err == nil && v.CommitID == revision
		case "code", "commit":
			return resourceID == revision && accessibilityRevisionIsVisible(git, repo, revision)
		case "metric", "log", "trace", "health_check", "support_report":
			for _, o := range contract.Observations {
				if o.ID == resourceID && revision == "observation:"+o.ID && o.ContractVersion == in.ContractVersion && o.ObjectiveID == in.ObjectiveID {
					return true
				}
			}
			return false
		case "dependent_service":
			for _, r := range contract.Revisions {
				if r.Version == in.ContractVersion {
					for _, d := range r.Dependencies {
						if d.ID == resourceID && revision == "contract:"+strconv.Itoa(r.Version) {
							return true
						}
					}
				}
			}
			return false
		default:
			return false
		}
	}
	if in.Trigger.Kind == "pull_request" && !resolve("pull_request", in.Trigger.ID, in.Trigger.Revision) {
		return false
	}
	if in.Trigger.Kind == "deployment" && !resolve("deployment", in.Trigger.ID, in.Trigger.Revision) {
		return false
	}
	for _, e := range in.Evidence {
		if !resolve(e.Kind, e.ResourceID, e.Revision) {
			return false
		}
	}
	return true
}

func writeReliabilityInvestigation(w http.ResponseWriter, v serviceobjectives.Contract, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, serviceobjectives.ErrConflict):
		writeAPIError(w, 409, "reliability_investigation_conflict", "the investigation changed; reload before contributing")
	case errors.Is(err, serviceobjectives.ErrNotFound):
		writeAPIError(w, 404, "reliability_investigation_not_found", "reliability investigation not found")
	case errors.Is(err, serviceobjectives.ErrInvalid):
		writeAPIError(w, 400, "invalid_reliability_investigation", "the investigation must remain revision-bound, evidence-cited, uncertainty-aware, and owner-addressed")
	default:
		log.Printf("reliability investigation storage: %v", err)
		writeAPIError(w, 500, "reliability_investigation_unavailable", "reliability investigation could not be persisted")
	}
}

func serviceObjectiveReaderParticipant(r *http.Request, catalog *repositories.Store, credentials *auth.Store, repositoryID string) bool {
	actor, authenticated, err := authenticateOptionalCredential(r, credentials, "repositories:read")
	if err != nil || !authenticated {
		return false
	}
	if actor.AgentID != "" {
		return true
	}
	repo, err := catalog.GetByID(repositoryID)
	if err != nil {
		return false
	}
	if repo.OwnerID == actor.UserID {
		return true
	}
	ok, err := catalog.HasCollaborator(actor.UserID, repositoryID)
	return err == nil && ok
}
func writeReliabilityEvidence(w http.ResponseWriter, v serviceobjectives.Contract, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, serviceobjectives.ErrConflict):
		writeAPIError(w, 409, "signal_mapping_conflict", "the signal mapping changed; reload before publishing")
	case errors.Is(err, serviceobjectives.ErrMappingNotFound):
		writeAPIError(w, 404, "signal_mapping_not_found", "signal mapping not found")
	case errors.Is(err, serviceobjectives.ErrInvalid):
		writeAPIError(w, 400, "invalid_reliability_evidence", "evidence must bind an exact objective and mapping revision, sanitized sources, a measurement window, counts, uncertainty, gaps, and delivered-software provenance")
	default:
		log.Printf("reliability evidence storage: %v", err)
		writeAPIError(w, 500, "reliability_evidence_unavailable", "reliability evidence could not be persisted")
	}
}
func writeReliabilityDelivery(w http.ResponseWriter, v serviceobjectives.Contract, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, serviceobjectives.ErrConflict):
		writeAPIError(w, 409, "reliability_delivery_conflict", "reliability delivery evidence changed; reload before acting")
	case errors.Is(err, serviceobjectives.ErrNotFound), errors.Is(err, serviceobjectives.ErrPolicyNotFound):
		writeAPIError(w, 404, "reliability_delivery_not_found", "reliability delivery policy or impact not found")
	case errors.Is(err, serviceobjectives.ErrInvalid):
		writeAPIError(w, 422, "invalid_reliability_delivery", "policy and impact must remain objective-, revision-, scope-, owner-, and evidence-bound")
	default:
		log.Printf("reliability delivery storage: %v", err)
		writeAPIError(w, 500, "reliability_delivery_unavailable", "reliability delivery state could not be persisted")
	}
}
func serviceObjectiveParticipants(actor string, r serviceobjectives.Revision) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(v string) {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	add(actor)
	for _, v := range r.OwnerIDs {
		add(v)
	}
	for _, x := range r.Objectives {
		for _, v := range x.OwnerIDs {
			add(v)
		}
	}
	for _, x := range r.Journeys {
		for _, v := range x.OwnerIDs {
			add(v)
		}
	}
	for _, x := range r.Dependencies {
		for _, v := range x.OwnerIDs {
			add(v)
		}
	}
	for _, x := range r.Severities {
		for _, v := range x.OwnerIDs {
			add(v)
		}
	}
	for _, v := range r.ExceptionPolicy.ApprovalOwnerIDs {
		add(v)
	}
	for _, x := range r.Exceptions {
		add(x.ApprovedBy)
	}
	return out
}
func writeServiceObjective(w http.ResponseWriter, v serviceobjectives.Contract, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, serviceobjectives.ErrConflict):
		writeAPIError(w, 409, "service_objective_conflict", "the service objective changed; reload before publishing")
	case errors.Is(err, serviceobjectives.ErrInvalid):
		writeAPIError(w, 400, "invalid_service_objective", "the contract must completely define scope, indicators, objectives, windows, journeys, budgets, severity, ownership, exception policy, and rationale")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "service_objective_forbidden", "only current repository participants may publish service objectives")
	default:
		log.Printf("service objective storage: %v", err)
		writeAPIError(w, 500, "service_objectives_unavailable", "service objectives could not be persisted")
	}
}
