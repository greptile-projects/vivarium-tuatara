package main

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/datacommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/dataflows"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/dataobservations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/extensions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerDataObservationRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, credentials *auth.Store, commitments *datacommitments.Store, flows *dataflows.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, extensionStore *extensions.Store, organizationStore *organizations.Store, proposalStore *proposals.Store, observations *dataobservations.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return actor, false, false
		}
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return actor, false, false
		}
		participant := false
		if actor.AgentID == "" {
			repo, _ := repos.GetByID(r.PathValue("id"))
			participant = repo.OwnerID == actor.UserID
			if !participant {
				participant, _ = repos.HasCollaborator(actor.UserID, r.PathValue("id"))
			}
		}
		return actor, participant, true
	}
	mux.HandleFunc("GET /repositories/{id}/data-observations", func(w http.ResponseWriter, r *http.Request) {
		_, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "data_observation_forbidden", "runtime data observations are private to current repository participants")
			return
		}
		v, e := observations.List(r.PathValue("id"))
		if e != nil {
			writeDataObservationError(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"data_observations": v})
	})
	mux.HandleFunc("GET /repositories/{id}/data-observations/{observation_id}", func(w http.ResponseWriter, r *http.Request) {
		_, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 404, "data_observation_not_found", "data observation not found")
			return
		}
		v, e := observations.Get(r.PathValue("id"), r.PathValue("observation_id"))
		if e != nil {
			writeDataObservationError(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/data-observations", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant && actor.AgentID == "" {
			writeAPIError(w, 403, "data_observation_forbidden", "only participants and repository-bound read-only agents may submit permitted sanitized signals")
			return
		}
		var in dataobservations.Observation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded sanitized production observation is required")
			return
		}
		actorType, actorID := "human", actor.UserID
		if actor.AgentID != "" {
			actorType, actorID = "agent", actor.AgentID
		}
		var v dataobservations.Observation
		create := func(installation *extensions.Installation) error {
			owners, valid := resolveDataObservationScope(r.PathValue("id"), in.Scope, commitments, flows, releaseStore, deploymentStore, installation)
			if !valid {
				return dataobservations.ErrInvalid
			}
			in.OwnerIDs = owners
			var createErr error
			v, createErr = observations.Create(r.PathValue("id"), actorType, actorID, in)
			return createErr
		}
		var e error
		if in.Scope.ExtensionInstallationID == "" {
			e = create(nil)
		} else {
			e = extensionStore.WithActiveInstallationRepository(in.Scope.ExtensionInstallationID, r.PathValue("id"), func(installation extensions.Installation) error {
				return create(&installation)
			})
		}
		if errors.Is(e, extensions.ErrInvalid) || errors.Is(e, extensions.ErrNotFound) || errors.Is(e, dataobservations.ErrInvalid) {
			writeAPIError(w, 422, "invalid_data_observation_scope", "flow, commitment, release, environment, deployment, and extension scope must resolve together")
			return
		}
		if e != nil {
			writeDataObservationError(w, e)
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/data-observations/{observation_id}/actions", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "data_observation_action_forbidden", "only current human collaborators may contain or coordinate a data-use gap")
			return
		}
		var in struct {
			ExpectedVersion int                     `json:"expected_version"`
			Action          dataobservations.Action `json:"action"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a governed action are required")
			return
		}
		for _, id := range in.Action.ParticipantIDs {
			repo, _ := repos.GetByID(r.PathValue("id"))
			member, _ := repos.HasCollaborator(id, r.PathValue("id"))
			if id != repo.OwnerID && !member {
				writeAPIError(w, 422, "invalid_notification_participant", "notified participants must currently belong to the repository")
				return
			}
		}
		v, e := observations.AddAction(r.PathValue("id"), r.PathValue("observation_id"), actor.UserID, in.ExpectedVersion, in.Action)
		if e != nil {
			writeDataObservationError(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/data-observations/{observation_id}/repair", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "data_observation_repair_forbidden", "only current human collaborators may delegate repair")
			return
		}
		var in struct {
			ExpectedVersion  int    `json:"expected_version"`
			Title            string `json:"title"`
			Body             string `json:"body"`
			TaskTitle        string `json:"task_title"`
			Outcome          string `json:"outcome"`
			VerificationPlan string `json:"verification_plan"`
			AssigneeType     string `json:"assignee_type"`
			AssigneeID       string `json:"assignee_id"`
		}
		if decodeJSON(r, &in) != nil || (in.AssigneeType != "human" && in.AssigneeType != "agent") {
			writeAPIError(w, 422, "invalid_data_observation_repair", "current evidence and a human- or agent-owned repair are required")
			return
		}
		v, e := observations.Get(r.PathValue("id"), r.PathValue("observation_id"))
		if e != nil {
			writeDataObservationError(w, e)
			return
		}
		if v.Version != in.ExpectedVersion || v.Repair != nil {
			writeAPIError(w, 409, "data_observation_conflict", "the observation changed; reload before delegating repair")
			return
		}
		if in.AssigneeType == "human" {
			if in.AssigneeID == "" {
				in.AssigneeID = actor.UserID
			}
			repo, _ := repos.GetByID(r.PathValue("id"))
			member, _ := repos.HasCollaborator(in.AssigneeID, r.PathValue("id"))
			if in.AssigneeID != repo.OwnerID && !member {
				writeAPIError(w, 422, "invalid_data_observation_assignee", "human assignee must currently participate in the repository")
				return
			}
		}
		if in.AssigneeType == "agent" && (in.AssigneeID == "" || organizationStore == nil) {
			writeAPIError(w, 422, "invalid_data_observation_assignee", "an existing approved agent identity is required")
			return
		}
		if !accessibilityRevisionIsVisible(git, r.PathValue("id"), v.Scope.Revision) {
			writeAPIError(w, 409, "data_observation_context_changed", "the responsible release revision is no longer available")
			return
		}
		items := make([]proposals.ReasoningItem, len(v.Evidence))
		for i, evidence := range v.Evidence {
			items[i] = proposals.ReasoningItem{ID: evidence.Digest, Kind: v.SignalKind, Summary: "Sanitized " + evidence.Kind + " evidence (" + evidence.Digest + ")", Status: "confirmed"}
		}
		origin := proposals.ReasoningOrigin{DataObservationID: v.ID, DataObservationVersion: v.Version, Revision: v.Scope.Revision, Items: items, AnalysisStatus: "production_data_gap"}
		var p proposals.Proposal
		var tasks []proposals.Task
		publish := func() error {
			var publishErr error
			p, tasks, publishErr = proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: v.RepositoryID, ActorID: actor.UserID, Title: in.Title, Body: in.Body, Origin: origin, Tasks: []proposals.ImplementationTaskInput{{Title: in.TaskTitle, Outcome: in.Outcome, Risk: v.Severity + " production data-use gap", VerificationPlan: in.VerificationPlan, AssigneeType: in.AssigneeType, AssigneeID: strings.TrimSpace(in.AssigneeID)}}})
			return publishErr
		}
		if in.AssigneeType == "agent" {
			e = organizationStore.WithCurrentAgentOperator(in.AssigneeID, actor.UserID, func(organizationID string) error {
				return repos.WithCurrentDeliveryAuthority([]string{actor.UserID}, v.RepositoryID, organizationID, publish)
			})
			if errors.Is(e, organizations.ErrNotFound) || errors.Is(e, organizations.ErrInvalid) || errors.Is(e, repositories.ErrNotFound) || errors.Is(e, repositories.ErrInvalidCollaborator) {
				writeAPIError(w, 409, "data_observation_assignment_authority_changed", "the approved-agent operator or repository authority changed before repair publication")
				return
			}
		} else {
			e = repos.WithCurrentParticipants([]string{actor.UserID, in.AssigneeID}, v.RepositoryID, publish)
			if errors.Is(e, repositories.ErrNotFound) || errors.Is(e, repositories.ErrInvalidCollaborator) {
				writeAPIError(w, 409, "data_observation_assignment_authority_changed", "the collaborator or assignee authority changed before repair publication")
				return
			}
		}
		if e != nil && !errors.Is(e, proposals.ErrDurabilityUncertain) {
			writeProposalError(w, e)
			return
		}
		updated, e := observations.LinkRepair(v.RepositoryID, v.ID, actor.UserID, v.Version, dataobservations.Repair{ProposalID: p.ID, TaskID: tasks[0].ID, AssigneeType: in.AssigneeType, AssigneeID: tasks[0].Assignment.AssigneeID})
		if e != nil {
			writeDataObservationError(w, e)
			return
		}
		writeJSON(w, 201, map[string]any{"data_observation": updated, "proposal": p, "task": tasks[0]})
	})
}

func resolveDataObservationScope(repo string, s dataobservations.Scope, commitments *datacommitments.Store, flows *dataflows.Store, releasesStore *releases.Store, deploymentsStore *deployments.Store, installation *extensions.Installation) ([]string, bool) {
	f, e := flows.Get(repo, s.DataFlowID)
	if e != nil || s.DataFlowVersion < 1 || s.DataFlowVersion > len(f.Revisions) || f.Revisions[s.DataFlowVersion-1].CodeRevision != s.Revision {
		return nil, false
	}
	if !dataFlowRevisionReferencesUse(f.Revisions[s.DataFlowVersion-1], s.CommitmentID, s.CommitmentVersion, s.DataUseID) {
		return nil, false
	}
	c, e := commitments.Get(s.CommitmentID)
	if e != nil || c.RepositoryID != repo || s.CommitmentVersion < 1 || s.CommitmentVersion > len(c.Revisions) {
		return nil, false
	}
	var owners []string
	found := false
	for _, u := range c.Revisions[s.CommitmentVersion-1].DataUses {
		if u.ID == s.DataUseID {
			owners = append([]string(nil), u.OwnerIDs...)
			found = true
		}
	}
	if !found || len(owners) == 0 {
		return nil, false
	}
	rel, e := releasesStore.Get(repo, s.ReleaseID)
	if e != nil || rel.CommitID != s.Revision {
		return nil, false
	}
	env, e := deploymentsStore.GetEnvironment(repo, s.EnvironmentID)
	if e != nil || env.RepositoryID != repo {
		return nil, false
	}
	dep, e := deploymentsStore.GetPromotion(repo, s.DeploymentID)
	if e != nil || dep.EnvironmentID != s.EnvironmentID || dep.ReleaseID != s.ReleaseID || dep.CommitID != s.Revision {
		return nil, false
	}
	if s.ExtensionInstallationID != "" {
		if installation == nil || installation.ID != s.ExtensionInstallationID || !dataObservationInstallationActive(repo, *installation, nil) {
			return nil, false
		}
	}
	return owners, true
}

func dataObservationInstallationActive(repo string, installation extensions.Installation, err error) bool {
	return err == nil && installation.Status == "active" && slices.Contains(installation.RepositoryIDs, repo)
}

func dataFlowRevisionReferencesUse(revision dataflows.Revision, commitmentID string, commitmentVersion int, dataUseID string) bool {
	for _, ref := range revision.CommitmentRefs {
		if ref.CommitmentID == commitmentID && ref.Version == commitmentVersion && slices.Contains(ref.DataUseIDs, dataUseID) {
			return true
		}
	}
	return false
}
func writeDataObservationError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, dataobservations.ErrNotFound):
		writeAPIError(w, 404, "data_observation_not_found", "data observation not found")
	case errors.Is(e, dataobservations.ErrConflict):
		writeAPIError(w, 409, "data_observation_conflict", "the observation changed; reload and retry")
	case errors.Is(e, dataobservations.ErrInvalid):
		writeAPIError(w, 422, "invalid_data_observation", "sanitized evidence or governed action is invalid")
	default:
		writeAPIError(w, 500, "data_observations_unavailable", "data-use evidence could not be persisted")
	}
}
