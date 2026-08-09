package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerEvolutionRoutes(mux *http.ServeMux, gitStore *storage.Store, repos *repositories.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, relationStore *relationships.Store, credentials *auth.Store, builds *checkruns.Store) {
	canRead := func(actorID, id string) bool {
		repo, e := repos.GetByID(id)
		if e != nil {
			return false
		}
		if repo.Visibility == repositories.Public {
			return true
		}
		ok, _ := repos.HasCollaborator(actorID, id)
		return repo.OwnerID == actorID || ok
	}
	canReadContractCandidate := func(actorID string, candidate relationships.ContractCandidate) bool {
		for _, revision := range candidate.Revisions {
			if !canRead(actorID, revision.RepositoryID) || !canRead(actorID, revision.SourceRepositoryID) {
				return false
			}
		}
		return true
	}
	project := func(v relationships.Evolution) relationships.Evolution {
		completed := map[string]bool{}
		for i := range v.MigrationTasks {
			link := &v.MigrationTasks[i]
			task, err := proposalStore.GetTask(link.RepositoryID, link.ProposalID, link.TaskID)
			if err != nil {
				link.Status = "unavailable"
				continue
			}
			link.Status = task.Status
			completed[link.ID] = task.Status == proposals.TaskCompleted
			if task.Assignment != nil {
				link.AssignmentID, link.AssigneeType, link.AssigneeID = task.Assignment.ID, task.Assignment.AssigneeType, task.Assignment.AssigneeID
				link.BaseRevision = task.Assignment.Access.BaseRevision
				if task.Assignment.AssigneeType == "agent" {
					link.Branch = "agent/tasks/" + task.ID + "-" + task.Assignment.ID[:8]
				}
			}
			if task.Contribution != nil {
				link.PullRequestID, link.ContributionStatus = task.Contribution.PullRequestID, task.Contribution.Status
				if pull, e := pullStore.Get(link.RepositoryID, task.Contribution.PullRequestID); e == nil {
					link.Branch = pull.SourceBranch
				}
			}
		}
		for i := range v.MigrationTasks {
			ready := v.MigrationTasks[i].Status == proposals.TaskTodo
			for _, dependency := range v.MigrationTasks[i].DependencyIDs {
				ready = ready && completed[dependency]
			}
			v.MigrationTasks[i].Ready = ready
		}
		if v.Rollout != nil {
			candidatePassing := false
			for _, candidate := range v.ContractCandidates {
				if candidate.ID != v.Rollout.CandidateID || candidate.SupersededAt != nil {
					continue
				}
				candidatePassing = true
				for _, runID := range candidate.CheckRunIDs {
					run, err := builds.Get(v.RepositoryID, candidate.CombinationHash[:32], runID)
					candidatePassing = candidatePassing && err == nil && run.State == "succeeded"
				}
			}
			approved := map[string]bool{}
			for _, approval := range v.Rollout.Approvals {
				approved[approval.RepositoryID] = true
			}
			priorComplete := candidatePassing
			v.Rollout.Outcomes = []relationships.EvolutionRolloutOutcome{}
			for i := range v.Rollout.Phases {
				phase := &v.Rollout.Phases[i]
				phase.State, phase.NextAction = "blocked", "pass the frozen compatibility gate"
				allApproved := true
				complete := true
				failed := false
				for _, repositoryID := range phase.RepositoryIDs {
					allApproved = allApproved && approved[repositoryID]
					outcome := relationships.EvolutionRolloutOutcome{PhaseID: phase.ID, RepositoryID: repositoryID, State: "awaiting_merge"}
					repositoryComplete := false
					var task *relationships.EvolutionMigrationTask
					selectedTaskID := phase.MigrationTaskIDs[repositoryID]
					for j := range v.MigrationTasks {
						if v.MigrationTasks[j].RepositoryID == repositoryID && v.MigrationTasks[j].ID == selectedTaskID {
							task = &v.MigrationTasks[j]
						}
					}
					if task == nil || task.PullRequestID == "" {
						complete = false
					} else if pull, err := pullStore.Get(repositoryID, task.PullRequestID); err == nil {
						outcome.PullRequestID = pull.ID
						outcome.State = pull.Status
						repositoryComplete = pull.Status == pullrequests.Merged
						if pull.Status == pullrequests.Closed {
							failed = true
						}
					}
					if repositoryComplete {
						releases, _ := releaseStore.List(repositoryID)
						repository, _ := gitStore.Open(repositoryID)
						for _, release := range releases {
							if task != nil && task.PullRequestID != "" {
								pull, _ := pullStore.Get(repositoryID, task.PullRequestID)
								if pull.MergeCommitID != nil && repository != nil {
									ancestry, ancestryErr := repository.ListCommitAncestry(storage.ObjectID(release.CommitID))
									for _, commit := range ancestry {
										if string(commit.ID) == *pull.MergeCommitID && ancestryErr == nil {
											outcome.ReleaseID, outcome.State = release.ID, "released"
										}
									}
								}
							}
						}
						if outcome.ReleaseID == "" {
							repositoryComplete = false
						}
					}
					if environmentID := phase.EnvironmentIDs[repositoryID]; repositoryComplete && environmentID != "" {
						promotions, _ := deploymentStore.ListPromotions(repositoryID)
						deployed := false
						for _, promotion := range promotions {
							if promotion.ReleaseID == outcome.ReleaseID && promotion.EnvironmentID == environmentID {
								outcome.DeploymentID, outcome.State = promotion.ID, promotion.State
								deployed = promotion.State == "succeeded"
								failed = failed || promotion.State == "failed" || promotion.State == "canceled"
							}
						}
						repositoryComplete = repositoryComplete && deployed
					}
					complete = complete && repositoryComplete
					v.Rollout.Outcomes = append(v.Rollout.Outcomes, outcome)
				}
				switch {
				case failed:
					phase.State, phase.NextAction = "paused", "rollback or start an agent-assisted repair from the failed deployment"
				case !priorComplete:
				case !allApproved:
					phase.State, phase.NextAction = "awaiting_approval", "each repository owner approves participation"
				case complete:
					phase.State, phase.NextAction = "completed", "advance to the next phase"
				default:
					phase.State, phase.NextAction = "ready", "merge through the repository queue, then release and promote through the named environment"
				}
				priorComplete = priorComplete && complete && !failed
			}
			v.Rollout.State, v.Rollout.NextAction = "active", "complete the current phase next action"
			if !candidatePassing {
				v.Rollout.State, v.Rollout.NextAction = "blocked", "restore a passing, current compatibility candidate"
			}
			if len(v.Rollout.Phases) > 0 && v.Rollout.Phases[len(v.Rollout.Phases)-1].State == "completed" {
				v.Rollout.State, v.Rollout.NextAction = "completed", "rollout complete"
			}
			for _, p := range v.Rollout.Phases {
				if p.State == "paused" {
					v.Rollout.State, v.Rollout.NextAction = "paused", p.NextAction
				}
			}
		}
		return v
	}
	visible := func(v relationships.Evolution, actorID string) relationships.Evolution {
		impacts := v.Impacts[:0]
		for _, impact := range v.Impacts {
			if canRead(actorID, impact.RepositoryID) {
				impacts = append(impacts, impact)
			}
		}
		v.Impacts = impacts
		allowed := map[string]bool{v.RepositoryID: true}
		for _, impact := range impacts {
			allowed[impact.RepositoryID] = true
		}
		findings := v.Findings[:0]
		for _, finding := range v.Findings {
			keep := true
			for _, id := range finding.RepositoryIDs {
				keep = keep && allowed[id]
			}
			if keep {
				findings = append(findings, finding)
			}
		}
		v.Findings = findings
		acknowledgements := v.Acknowledgements[:0]
		for _, acknowledgement := range v.Acknowledgements {
			if allowed[acknowledgement.RepositoryID] {
				acknowledgements = append(acknowledgements, acknowledgement)
			}
		}
		v.Acknowledgements = acknowledgements
		analyses := v.Analyses[:0]
		for _, analysis := range v.Analyses {
			keep := true
			for _, id := range analysis.RepositoryIDs {
				keep = keep && allowed[id]
			}
			if keep {
				analysis.StoredCredentialID = ""
				analyses = append(analyses, analysis)
			}
		}
		v.Analyses = analyses
		tasks := v.MigrationTasks[:0]
		for _, task := range v.MigrationTasks {
			if canRead(actorID, task.RepositoryID) {
				tasks = append(tasks, task)
			}
		}
		v.MigrationTasks = tasks
		candidates := v.ContractCandidates[:0]
		for _, candidate := range v.ContractCandidates {
			if canReadContractCandidate(actorID, candidate) {
				candidates = append(candidates, candidate)
			}
		}
		v.ContractCandidates = candidates
		if v.Rollout != nil {
			candidateVisible := false
			for _, candidate := range candidates {
				candidateVisible = candidateVisible || candidate.ID == v.Rollout.CandidateID
			}
			if !candidateVisible {
				v.Rollout = nil
				return v
			}
			phases := v.Rollout.Phases[:0]
			for _, phase := range v.Rollout.Phases {
				ids := phase.RepositoryIDs[:0]
				environments := map[string]string{}
				tasks := map[string]string{}
				for _, id := range phase.RepositoryIDs {
					if canRead(actorID, id) {
						ids = append(ids, id)
						if x := phase.EnvironmentIDs[id]; x != "" {
							environments[id] = x
						}
						tasks[id] = phase.MigrationTaskIDs[id]
					}
				}
				if len(ids) > 0 {
					phase.RepositoryIDs, phase.EnvironmentIDs, phase.MigrationTaskIDs = ids, environments, tasks
					phases = append(phases, phase)
				}
			}
			v.Rollout.Phases = phases
			outcomes := v.Rollout.Outcomes[:0]
			for _, outcome := range v.Rollout.Outcomes {
				if canRead(actorID, outcome.RepositoryID) {
					outcomes = append(outcomes, outcome)
				}
			}
			v.Rollout.Outcomes = outcomes
			approvals := v.Rollout.Approvals[:0]
			for _, approval := range v.Rollout.Approvals {
				if canRead(actorID, approval.RepositoryID) {
					approvals = append(approvals, approval)
				}
			}
			v.Rollout.Approvals = approvals
		}
		return v
	}
	analysisPacket := func(v relationships.Evolution, a relationships.EvolutionAnalysis) relationships.Evolution {
		selected := map[string]bool{v.RepositoryID: true}
		for _, id := range a.RepositoryIDs {
			selected[id] = true
		}
		impacts := v.Impacts[:0]
		for _, impact := range v.Impacts {
			if selected[impact.RepositoryID] {
				impacts = append(impacts, impact)
			}
		}
		v.Impacts = impacts
		findings := v.Findings[:0]
		for _, finding := range v.Findings {
			keep := true
			for _, id := range finding.RepositoryIDs {
				keep = keep && selected[id]
			}
			if keep {
				findings = append(findings, finding)
			}
		}
		v.Findings = findings
		acknowledgements := v.Acknowledgements[:0]
		for _, acknowledgement := range v.Acknowledgements {
			if selected[acknowledgement.RepositoryID] {
				acknowledgements = append(acknowledgements, acknowledgement)
			}
		}
		v.Acknowledgements = acknowledgements
		tasks := v.MigrationTasks[:0]
		for _, task := range v.MigrationTasks {
			if selected[task.RepositoryID] {
				tasks = append(tasks, task)
			}
		}
		v.MigrationTasks = tasks
		candidates := v.ContractCandidates[:0]
		for _, candidate := range v.ContractCandidates {
			inScope := true
			for _, revision := range candidate.Revisions {
				inScope = inScope && selected[revision.RepositoryID] && selected[revision.SourceRepositoryID]
			}
			if inScope && canReadContractCandidate(a.InitiatorID, candidate) {
				candidates = append(candidates, candidate)
			}
		}
		v.ContractCandidates = candidates
		v.Analyses = []relationships.EvolutionAnalysis{a}
		v.Analyses[0].StoredCredentialID = ""
		return v
	}
	readActor := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return actor, false
		}
		if !authenticated {
			optional, present, authOK := authenticateOptionalRequest(w, r, credentials, "repositories:read", false)
			if !authOK {
				return actor, false
			}
			if present {
				actor = optional
			}
		}
		return actor, true
	}
	mux.HandleFunc("GET /repositories/{id}/evolutions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := readActor(w, r)
		if !ok {
			return
		}
		items, e := relationStore.ListEvolutions(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "evolution_read_failed", "evolution plans could not be read")
			return
		}
		for i := range items {
			items[i] = project(visible(items[i], actor.UserID))
		}
		writeJSON(w, 200, map[string]any{"evolutions": items})
	})
	mux.HandleFunc("GET /repositories/{id}/evolutions/{evolution_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := readActor(w, r)
		if !ok {
			return
		}
		v, e := relationStore.GetEvolution(r.PathValue("id"), r.PathValue("evolution_id"))
		if e != nil {
			writeAPIError(w, 404, "evolution_not_found", "evolution plan not found")
			return
		}
		writeJSON(w, 200, project(visible(v, actor.UserID)))
	})
	mux.HandleFunc("POST /repositories/{id}/evolutions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			InterfaceName          string                              `json:"interface_name"`
			PredecessorInterfaceID string                              `json:"predecessor_interface_id"`
			SourceKind             string                              `json:"source_kind"`
			SourceID               string                              `json:"source_id"`
			CandidateDescription   string                              `json:"candidate_description"`
			Changes                []relationships.CompatibilityChange `json:"changes"`
			Strategy               string                              `json:"strategy"`
			Sequencing             string                              `json:"sequencing"`
			Exceptions             string                              `json:"exceptions"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		var predecessor relationships.Interface
		interfaces, e := relationStore.ListInterfaces(r.PathValue("id"))
		if e == nil {
			for _, x := range interfaces {
				if x.ID == in.PredecessorInterfaceID && x.Name == strings.TrimSpace(in.InterfaceName) {
					predecessor = x
				}
			}
		}
		if predecessor.ID == "" {
			writeAPIError(w, 422, "invalid_predecessor", "predecessor must name a published interface in this repository")
			return
		}
		candidateCommit := ""
		switch in.SourceKind {
		case "proposal":
			if proposalStore == nil {
				e = errors.New("unavailable")
			} else {
				p, x := proposalStore.Get(r.PathValue("id"), in.SourceID)
				e = x
				if x == nil && p.Status != "open" {
					e = errors.New("closed")
				}
			}
		case "pull_request":
			if pullStore == nil {
				e = errors.New("unavailable")
			} else {
				p, x := pullStore.Get(r.PathValue("id"), in.SourceID)
				e = x
				if x == nil && p.Status != "open" {
					e = errors.New("closed")
				}
				candidateCommit = p.SourceCommitID
			}
		default:
			e = errors.New("kind")
		}
		if e != nil {
			writeAPIError(w, 422, "invalid_evolution_source", "source must name an open provider proposal or pull request")
			return
		}
		impacts := []relationships.ConsumerImpact{}
		ids, e := relationStore.ListRepositoryIDs()
		if e != nil {
			writeAPIError(w, 500, "evolution_create_failed", "relationship evidence could not be read")
			return
		}
		for _, id := range ids {
			if !canRead(actor.UserID, id) {
				continue
			}
			ds, x := relationStore.ListDependencies(id)
			if x != nil {
				writeAPIError(w, 500, "evolution_create_failed", "relationship evidence could not be read")
				return
			}
			repo, _ := repos.GetByID(id)
			for _, d := range ds {
				if d.ProviderRepositoryID == r.PathValue("id") && d.InterfaceName == predecessor.Name {
					if dependencyStaleReason(d, releaseStore, deploymentStore) != "" {
						continue
					}
					impacts = append(impacts, relationships.ConsumerImpact{RepositoryID: id, OwnerID: repo.OwnerID, DependencyID: d.ID, CommitID: d.CommitID, Constraint: d.Constraint, State: "affected"})
				}
			}
		}
		v, e := relationStore.CreateEvolution(relationships.Evolution{RepositoryID: r.PathValue("id"), InterfaceName: in.InterfaceName, Predecessor: predecessor, SourceKind: in.SourceKind, SourceID: in.SourceID, CandidateCommitID: candidateCommit, CandidateDescription: in.CandidateDescription, Changes: in.Changes, Impacts: impacts, Strategy: in.Strategy, Sequencing: in.Sequencing, Exceptions: in.Exceptions, CreatedBy: actor.UserID})
		if e != nil {
			writeAPIError(w, 422, "invalid_evolution", "evolution comparison and migration contract are required")
			return
		}
		w.Header().Set("Location", r.URL.Path+"/"+v.ID)
		writeJSON(w, 201, visible(v, actor.UserID))
	})
	mux.HandleFunc("PATCH /repositories/{id}/evolutions/{evolution_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Version    int    `json:"version"`
			Strategy   string `json:"strategy"`
			Sequencing string `json:"sequencing"`
			Exceptions string `json:"exceptions"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, e := relationStore.UpdateEvolution(r.PathValue("id"), r.PathValue("evolution_id"), "", in.Version, in.Strategy, in.Sequencing, in.Exceptions)
		if errors.Is(e, relationships.ErrConflict) {
			writeAPIError(w, 409, "evolution_changed", "evolution plan changed; reload before editing")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "invalid_evolution", "migration contract is invalid")
			return
		}
		writeJSON(w, 200, visible(v, actor.UserID))
	})
	mux.HandleFunc("POST /repositories/{id}/evolutions/{evolution_id}/acknowledgements", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := readActor(w, r)
		if !ok {
			return
		}
		var in struct {
			RepositoryID string `json:"repository_id"`
			Note         string `json:"note"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		participant, _ := repos.HasCollaborator(actor.UserID, in.RepositoryID)
		repo, e := repos.GetByID(in.RepositoryID)
		if e != nil || (repo.OwnerID != actor.UserID && !participant) {
			writeAPIError(w, 403, "acknowledgement_forbidden", "only a current consumer participant may acknowledge impact")
			return
		}
		v, e := relationStore.GetEvolution(r.PathValue("id"), r.PathValue("evolution_id"))
		affected := false
		for _, x := range v.Impacts {
			affected = affected || x.RepositoryID == in.RepositoryID
		}
		if e != nil || !affected {
			writeAPIError(w, 422, "consumer_not_affected", "repository is not in this plan's impact snapshot")
			return
		}
		v, e = relationStore.AcknowledgeEvolution(v.RepositoryID, v.ID, actor.UserID, in.RepositoryID, in.Note)
		if e != nil {
			writeAPIError(w, 409, "acknowledgement_exists", "this participant already acknowledged the consumer impact")
			return
		}
		writeJSON(w, 201, visible(v, actor.UserID))
	})
	mux.HandleFunc("PUT /repositories/{id}/evolutions/{evolution_id}/rollout", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Version     int                                   `json:"version"`
			CandidateID string                                `json:"candidate_id"`
			Phases      []relationships.EvolutionRolloutPhase `json:"phases"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_rollout", "version, candidate_id, and phases are required")
			return
		}
		v, err := relationStore.ConfigureEvolutionRollout(r.PathValue("id"), r.PathValue("evolution_id"), actor.UserID, in.CandidateID, in.Phases, in.Version, func(candidate relationships.ContractCandidate) bool {
			if len(candidate.CheckRunIDs) == 0 {
				return false
			}
			for _, runID := range candidate.CheckRunIDs {
				run, runErr := builds.Get(r.PathValue("id"), candidate.CombinationHash[:32], runID)
				if runErr != nil || run.State != "succeeded" {
					return false
				}
			}
			return true
		})
		if errors.Is(err, relationships.ErrConflict) {
			writeAPIError(w, 409, "evolution_changed", "evolution plan changed; reload before configuring rollout")
			return
		}
		if errors.Is(err, relationships.ErrInvalid) {
			writeAPIError(w, 422, "invalid_rollout", "rollout must use a current passing candidate and place each affected repository in at most one valid phase")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "rollout_write_failed", "rollout could not be configured")
			return
		}
		writeJSON(w, 200, project(visible(v, actor.UserID)))
	})
	mux.HandleFunc("POST /repositories/{id}/evolutions/{evolution_id}/rollout/approvals", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		var in struct {
			Version      int    `json:"version"`
			RepositoryID string `json:"repository_id"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_rollout_approval", "version and repository_id are required")
			return
		}
		repository, err := repos.GetByID(in.RepositoryID)
		if err != nil || repository.OwnerID != actor.UserID {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		v, err := relationStore.ApproveEvolutionRollout(r.PathValue("id"), r.PathValue("evolution_id"), actor.UserID, in.RepositoryID, in.Version)
		if errors.Is(err, relationships.ErrConflict) {
			writeAPIError(w, 409, "evolution_changed", "rollout changed or this repository already approved")
			return
		}
		if errors.Is(err, relationships.ErrInvalid) {
			writeAPIError(w, 422, "invalid_rollout_approval", "repository does not participate in this rollout")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "rollout_write_failed", "approval could not be recorded")
			return
		}
		writeJSON(w, 201, project(visible(v, actor.UserID)))
	})
	mux.HandleFunc("POST /repositories/{id}/evolutions/{evolution_id}/migration-tasks", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := readActor(w, r)
		if !ok {
			return
		}
		var in struct {
			Version            int      `json:"version"`
			RepositoryID       string   `json:"repository_id"`
			Title              string   `json:"title"`
			CompletionCriteria string   `json:"completion_criteria"`
			TargetVersion      string   `json:"target_version"`
			DependencyIDs      []string `json:"dependency_ids"`
			AssigneeType       string   `json:"assignee_type"`
			AssigneeID         string   `json:"assignee_id"`
			Mandate            string   `json:"mandate"`
			BaseRevision       string   `json:"base_revision"`
		}
		if decodeJSON(r, &in) != nil || (in.AssigneeType != "human" && in.AssigneeType != "agent") {
			writeAPIError(w, 400, "invalid_migration_task", "repository, task, target version, assignment, and exact base are required")
			return
		}
		if strings.TrimSpace(in.Title) == "" || strings.ContainsAny(in.Title, "\r\n") || len([]rune(in.Title)) > 200 || strings.TrimSpace(in.CompletionCriteria) == "" || len([]rune(in.CompletionCriteria)) > 2000 || strings.TrimSpace(in.Mandate) == "" || len([]rune(in.Mandate)) > 4000 {
			writeAPIError(w, 422, "invalid_migration_task", "task title, completion criteria, and mandate are required and bounded")
			return
		}
		target, err := repos.GetByID(in.RepositoryID)
		collaborator, _ := repos.HasCollaborator(actor.UserID, in.RepositoryID)
		if err != nil || (target.OwnerID != actor.UserID && !collaborator) {
			writeAPIError(w, 403, "migration_task_forbidden", "a current target-repository participant must create migration work")
			return
		}
		plan, err := relationStore.GetEvolution(r.PathValue("id"), r.PathValue("evolution_id"))
		allowed := in.RepositoryID == plan.RepositoryID
		known := map[string]bool{}
		for _, impact := range plan.Impacts {
			allowed = allowed || impact.RepositoryID == in.RepositoryID
		}
		for _, task := range plan.MigrationTasks {
			known[task.ID] = true
		}
		if err != nil {
			writeAPIError(w, 404, "evolution_not_found", "evolution plan not found")
			return
		}
		if in.Version != plan.Version {
			writeAPIError(w, 409, "evolution_changed", "evolution plan changed; reload before adding work")
			return
		}
		if !relationships.ValidVersion(in.TargetVersion) {
			writeAPIError(w, 422, "invalid_target_version", "target version must be semantic, for example v2.0.0")
			return
		}
		if !allowed {
			writeAPIError(w, 422, "invalid_migration_repository", "migration work must target the provider or a frozen affected consumer")
			return
		}
		seenDependencies := map[string]bool{}
		for _, dependency := range in.DependencyIDs {
			if !known[dependency] || seenDependencies[dependency] {
				writeAPIError(w, 422, "invalid_migration_dependency", "dependencies must name earlier migration tasks")
				return
			}
			seenDependencies[dependency] = true
		}
		repository, openErr := gitStore.Open(in.RepositoryID)
		if openErr != nil {
			writeAPIError(w, 500, "migration_task_failed", "target repository storage is unavailable")
			return
		}
		if _, readErr := repository.ReadCommit(storage.ObjectID(strings.ToLower(in.BaseRevision))); readErr != nil {
			writeAPIError(w, 422, "invalid_base_revision", "base revision must be an existing target-repository commit")
			return
		}
		if in.AssigneeType == "human" {
			participant, _ := repos.HasCollaborator(in.AssigneeID, in.RepositoryID)
			if in.AssigneeID != target.OwnerID && !participant {
				writeAPIError(w, 422, "invalid_task_assignee", "human assignee must already participate in the target repository")
				return
			}
		}
		var proposal proposals.Proposal
		var assigned proposals.Task
		plan, link, err := relationStore.CreateEvolutionMigrationTask(plan.RepositoryID, plan.ID, actor.UserID, in.RepositoryID, in.TargetVersion, in.DependencyIDs, in.Version, func() (string, string, error) {
			compensate := func(taskID, assignmentID string) {
				if proposal.ID != "" {
					_ = proposalStore.DeleteMigrationWork(in.RepositoryID, proposal.ID, taskID, assignmentID)
				}
			}
			body := "Migration work for interface evolution " + plan.ID + ".\n\nTarget version: " + strings.TrimSpace(in.TargetVersion) + "\n\nPlan strategy:\n" + plan.Strategy + "\n\nSequencing:\n" + plan.Sequencing
			var publishErr error
			proposal, publishErr = proposalStore.Create(in.RepositoryID, actor.UserID, in.Title, body)
			if publishErr != nil && !errors.Is(publishErr, proposals.ErrDurabilityUncertain) {
				compensate("", "")
				return "", "", publishErr
			}
			var task proposals.Task
			task, publishErr = proposalStore.CreateTask(in.RepositoryID, proposal.ID, actor.UserID, in.Title, in.CompletionCriteria, nil, nil)
			if publishErr != nil && !errors.Is(publishErr, proposals.ErrDurabilityUncertain) {
				compensate("", "")
				return "", "", publishErr
			}
			assigned, publishErr = proposalStore.AssignTask(in.RepositoryID, proposal.ID, task.ID, actor.UserID, proposals.TaskAssignmentInput{AssigneeType: in.AssigneeType, AssigneeID: in.AssigneeID, Mandate: in.Mandate, RepositoryID: in.RepositoryID, BaseRevision: in.BaseRevision})
			if publishErr != nil && !errors.Is(publishErr, proposals.ErrDurabilityUncertain) {
				compensate(task.ID, "")
				return "", "", publishErr
			}
			return proposal.ID, task.ID, nil
		})
		if errors.Is(err, relationships.ErrConflict) {
			writeAPIError(w, 409, "evolution_changed", "evolution plan changed; reload before adding work")
			return
		}
		if err != nil {
			if proposal.ID != "" && assigned.Assignment != nil {
				_ = proposalStore.DeleteMigrationWork(in.RepositoryID, proposal.ID, assigned.ID, assigned.Assignment.ID)
			}
			writeAPIError(w, 500, "migration_task_failed", "migration task could not be published")
			return
		}
		writeJSON(w, 201, map[string]any{"evolution": project(visible(plan, actor.UserID)), "migration_task": link, "task": assigned})
	})
	mux.HandleFunc("POST /repositories/{id}/evolutions/{evolution_id}/analyses", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Mandate       string   `json:"mandate"`
			RepositoryIDs []string `json:"repository_ids"`
			ExpiresIn     int64    `json:"expires_in"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if in.ExpiresIn == 0 {
			in.ExpiresIn = 3600
		}
		if in.ExpiresIn < 300 || in.ExpiresIn > 86400 {
			writeAPIError(w, 422, "invalid_analysis", "expiry must be between 5 minutes and 24 hours")
			return
		}
		v, e := relationStore.GetEvolution(r.PathValue("id"), r.PathValue("evolution_id"))
		if e != nil {
			writeAPIError(w, 404, "evolution_not_found", "evolution plan not found")
			return
		}
		for _, id := range in.RepositoryIDs {
			selected := id == v.RepositoryID
			for _, x := range v.Impacts {
				selected = selected || x.RepositoryID == id
			}
			if !selected || !canRead(actor.UserID, id) {
				writeAPIError(w, 422, "invalid_analysis_repository", "selected repositories must be readable members of the impact snapshot")
				return
			}
		}
		issued, e := credentials.Issue(actor.UserID, auth.API, "Evolution analysis", []string{"evolutions:analyze"}, time.Duration(in.ExpiresIn)*time.Second)
		if e != nil {
			writeAPIError(w, 500, "analysis_start_failed", "read-only analysis access could not be issued")
			return
		}
		v, a, e := relationStore.StartEvolutionAnalysis(v.RepositoryID, v.ID, actor.UserID, issued.ID, in.Mandate, in.RepositoryIDs)
		if e != nil {
			_, _ = credentials.Revoke(actor.UserID, issued.ID)
			writeAPIError(w, 422, "invalid_analysis", "analysis mandate and repositories are required")
			return
		}
		a.StoredCredentialID = ""
		writeJSON(w, 201, map[string]any{"evolution": visible(v, actor.UserID), "analysis": a, "credential": issued})
	})
	mux.HandleFunc("POST /repositories/{id}/evolutions/{evolution_id}/contract-candidates", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if pullStore == nil || builds == nil {
			writeAPIError(w, 503, "contract_verification_unavailable", "contract verification storage is unavailable")
			return
		}
		var in struct {
			ProviderPullRequestID  string            `json:"provider_pull_request_id"`
			ConsumerPullRequestIDs map[string]string `json:"consumer_pull_request_ids"`
		}
		if decodeJSON(r, &in) != nil || len(in.ConsumerPullRequestIDs) == 0 || len(in.ConsumerPullRequestIDs) > 50 {
			writeAPIError(w, 400, "invalid_contract_candidate", "one provider pull and at least one consumer pull are required")
			return
		}
		plan, err := relationStore.GetEvolution(r.PathValue("id"), r.PathValue("evolution_id"))
		if err != nil {
			writeAPIError(w, 404, "evolution_not_found", "evolution plan not found")
			return
		}
		allowed := map[string]bool{}
		for _, impact := range plan.Impacts {
			allowed[impact.RepositoryID] = true
		}
		provider, err := pullStore.Get(plan.RepositoryID, in.ProviderPullRequestID)
		if err != nil || provider.Status != pullrequests.Open || !canRead(actor.UserID, provider.SourceRepositoryID) {
			writeAPIError(w, 422, "invalid_provider_revision", "provider pull must be open and belong to the evolution repository")
			return
		}
		revisions := []relationships.ContractCandidateRevision{{Role: "provider", RepositoryID: plan.RepositoryID, PullRequestID: provider.ID, SourceRepositoryID: provider.SourceRepositoryID, CommitID: provider.SourceCommitID}}
		ids := make([]string, 0, len(in.ConsumerPullRequestIDs))
		for id := range in.ConsumerPullRequestIDs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if !allowed[id] || !canRead(actor.UserID, id) {
				writeAPIError(w, 422, "invalid_consumer_revision", "consumer pulls must belong to readable repositories in the frozen impact snapshot")
				return
			}
			pull, getErr := pullStore.Get(id, in.ConsumerPullRequestIDs[id])
			if getErr != nil || pull.Status != pullrequests.Open || !canRead(actor.UserID, pull.SourceRepositoryID) {
				writeAPIError(w, 422, "invalid_consumer_revision", "each consumer revision must name an open pull request")
				return
			}
			revisions = append(revisions, relationships.ContractCandidateRevision{Role: "consumer", RepositoryID: id, PullRequestID: pull.ID, SourceRepositoryID: pull.SourceRepositoryID, CommitID: pull.SourceCommitID})
		}
		providerRepo, err := gitStore.Open(provider.SourceRepositoryID)
		if err != nil {
			writeAPIError(w, 422, "provider_revision_unavailable", "provider source revision is unavailable")
			return
		}
		configBody, err := exec.Command("git", "--git-dir="+providerRepo.Path(), "show", provider.SourceCommitID+":.vivarium/contracts.json").Output()
		if err != nil {
			writeAPIError(w, 422, "contract_definition_missing", "provider revision must contain .vivarium/contracts.json")
			return
		}
		config, err := checkruns.ParseConfig(configBody)
		if err != nil {
			writeAPIError(w, 422, "invalid_contract_definition", "provider contract checks are invalid")
			return
		}
		synthetic, err := assembleContractCandidate(gitStore, plan.RepositoryID, revisions)
		if err != nil {
			writeAPIError(w, 500, "contract_candidate_failed", "exact repository snapshots could not be assembled")
			return
		}
		hasher := sha256.New()
		for _, revision := range revisions {
			fmt.Fprintf(hasher, "%s\x00%s\x00%s\x00", revision.RepositoryID, revision.PullRequestID, revision.CommitID)
		}
		combination := hex.EncodeToString(hasher.Sum(nil))
		for _, existing := range plan.ContractCandidates {
			if existing.CombinationHash == combination {
				writeAPIError(w, 409, "combination_already_tested", "this exact pull revision combination already has evidence")
				return
			}
		}
		candidateID := combination[:32]
		repositoryIDs := make([]string, 0, len(revisions)*2)
		for _, revision := range revisions {
			repositoryIDs = append(repositoryIDs, revision.RepositoryID, revision.SourceRepositoryID)
		}
		var publicationErr error
		err = repos.WithCurrentReadAccess(actor.UserID, repositoryIDs, func() error {
			runs, createErr := builds.CreateRequested(plan.RepositoryID, candidateID, synthetic, config.Checks, actor.UserID)
			if createErr != nil {
				publicationErr = createErr
				return nil
			}
			runIDs := make([]string, len(runs))
			for i := range runs {
				runIDs[i] = runs[i].ID
			}
			published, candidate, publishErr := relationStore.AddContractCandidate(plan.RepositoryID, plan.ID, actor.UserID, synthetic, combination, revisions, runIDs)
			if publishErr != nil {
				publicationErr = publishErr
				return nil
			}
			target, _ := gitStore.Open(published.RepositoryID)
			for _, run := range runs {
				go builds.Execute(run, target.Path())
			}
			writeJSON(w, 201, map[string]any{"evolution": project(visible(published, actor.UserID)), "candidate": candidate, "check_runs": runs})
			return nil
		})
		if err != nil {
			writeAPIError(w, 404, "contract_candidate_not_found", "contract candidate repositories are no longer readable")
			return
		}
		if errors.Is(publicationErr, relationships.ErrConflict) {
			writeAPIError(w, 409, "combination_already_tested", "this exact pull revision combination already has evidence")
			return
		}
		if publicationErr != nil {
			writeAPIError(w, 500, "contract_candidate_failed", "contract candidate and checks could not be published")
		}
	})
	mux.HandleFunc("GET /repositories/{id}/evolutions/{evolution_id}/contract-candidates/{candidate_id}/checks", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := readActor(w, r)
		if !ok {
			return
		}
		plan, err := relationStore.GetEvolution(r.PathValue("id"), r.PathValue("evolution_id"))
		if err != nil {
			writeAPIError(w, 404, "evolution_not_found", "evolution plan not found")
			return
		}
		var candidate *relationships.ContractCandidate
		for i := range plan.ContractCandidates {
			if plan.ContractCandidates[i].ID == r.PathValue("candidate_id") {
				candidate = &plan.ContractCandidates[i]
			}
		}
		if candidate == nil {
			writeAPIError(w, 404, "contract_candidate_not_found", "contract candidate not found")
			return
		}
		if !canReadContractCandidate(actor.UserID, *candidate) {
			writeAPIError(w, 404, "contract_candidate_not_found", "contract candidate not found")
			return
		}
		runs, err := builds.List(plan.RepositoryID, candidate.CombinationHash[:32])
		if err != nil {
			writeAPIError(w, 500, "contract_evidence_failed", "contract evidence could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"candidate": candidate, "check_runs": runs, "attestation": map[string]any{"combination_hash": candidate.CombinationHash, "synthetic_commit": candidate.SyntheticCommit, "revisions": candidate.Revisions, "bounded_execution": map[string]any{"network": "none", "workspace": "read-only", "credentials": "none"}}})
	})
	mux.HandleFunc("GET /repositories/{id}/evolutions/{evolution_id}/contract-candidates/{candidate_id}/checks/{check_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := readActor(w, r)
		if !ok {
			return
		}
		plan, err := relationStore.GetEvolution(r.PathValue("id"), r.PathValue("evolution_id"))
		if err != nil {
			writeAPIError(w, 404, "contract_candidate_not_found", "contract candidate not found")
			return
		}
		var candidate *relationships.ContractCandidate
		for i := range plan.ContractCandidates {
			if plan.ContractCandidates[i].ID == r.PathValue("candidate_id") {
				candidate = &plan.ContractCandidates[i]
			}
		}
		allowed := candidate != nil && canReadContractCandidate(actor.UserID, *candidate)
		if !allowed {
			writeAPIError(w, 404, "contract_candidate_not_found", "contract candidate not found")
			return
		}
		known := false
		for _, id := range candidate.CheckRunIDs {
			known = known || id == r.PathValue("check_id")
		}
		if !known {
			writeAPIError(w, 404, "contract_check_not_found", "contract check not found")
			return
		}
		events, err := builds.Events(plan.RepositoryID, candidate.CombinationHash[:32], r.PathValue("check_id"), 0)
		if err != nil {
			writeAPIError(w, 500, "contract_evidence_failed", "contract logs could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"events": events})
	})
	mux.HandleFunc("GET /repositories/{id}/evolutions/{evolution_id}/contract-candidates/{candidate_id}/checks/{check_id}/artifacts/{artifact_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := readActor(w, r)
		if !ok {
			return
		}
		plan, err := relationStore.GetEvolution(r.PathValue("id"), r.PathValue("evolution_id"))
		if err != nil {
			writeAPIError(w, 404, "contract_candidate_not_found", "contract candidate not found")
			return
		}
		var candidate *relationships.ContractCandidate
		for i := range plan.ContractCandidates {
			if plan.ContractCandidates[i].ID == r.PathValue("candidate_id") {
				candidate = &plan.ContractCandidates[i]
			}
		}
		allowed := candidate != nil && canReadContractCandidate(actor.UserID, *candidate)
		if !allowed {
			writeAPIError(w, 404, "contract_candidate_not_found", "contract candidate not found")
			return
		}
		file, artifact, err := builds.OpenArtifact(plan.RepositoryID, candidate.CombinationHash[:32], r.PathValue("check_id"), r.PathValue("artifact_id"))
		if err != nil {
			writeAPIError(w, 404, "contract_artifact_not_found", "contract artifact not found")
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", artifact.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(artifact.Path)))
		http.ServeContent(w, r, artifact.Path, artifact.CreatedAt, file)
	})
	mux.HandleFunc("GET /repositories/{id}/evolutions/{evolution_id}/analyses/{analysis_id}", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "evolutions:analyze", false)
		if !ok {
			return
		}
		v, a, e := relationStore.EvolutionAnalysis(r.PathValue("id"), r.PathValue("evolution_id"), r.PathValue("analysis_id"), credential.ID)
		if e != nil {
			writeAPIError(w, 404, "analysis_not_found", "analysis not found")
			return
		}
		packet := analysisPacket(v, a)
		a.StoredCredentialID = ""
		writeJSON(w, 200, map[string]any{"plan": packet, "analysis": a})
	})
	mux.HandleFunc("POST /repositories/{id}/evolutions/{evolution_id}/analyses/{analysis_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "evolutions:analyze", false)
		if !ok {
			return
		}
		v, a, e := relationStore.EvolutionAnalysis(r.PathValue("id"), r.PathValue("evolution_id"), r.PathValue("analysis_id"), credential.ID)
		if e != nil {
			writeAPIError(w, 404, "analysis_not_found", "analysis not found")
			return
		}
		var in struct {
			RepositoryIDs []string `json:"repository_ids"`
			Finding       string   `json:"finding"`
			Uncertainty   string   `json:"uncertainty"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		for _, id := range in.RepositoryIDs {
			allowed := false
			for _, x := range a.RepositoryIDs {
				allowed = allowed || id == x
			}
			if !allowed {
				writeAPIError(w, 422, "finding_out_of_scope", "findings may cite only selected repositories")
				return
			}
		}
		v, e = relationStore.AddEvolutionFinding(v.RepositoryID, v.ID, a.AgentID, in.RepositoryIDs, in.Finding, in.Uncertainty)
		if e != nil {
			writeAPIError(w, 422, "invalid_finding", "finding and selected repositories are required")
			return
		}
		writeJSON(w, 201, analysisPacket(v, a))
	})
}

func assembleContractCandidate(gitStore *storage.Store, providerRepositoryID string, revisions []relationships.ContractCandidateRevision) (string, error) {
	workspace, err := os.MkdirTemp("", "vivarium-contract-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workspace)
	if err = exec.Command("git", "init", "-q", workspace).Run(); err != nil {
		return "", err
	}
	for i, revision := range revisions {
		repository, openErr := gitStore.Open(revision.SourceRepositoryID)
		if openErr != nil {
			return "", openErr
		}
		destination := filepath.Join(workspace, "provider")
		if i > 0 {
			destination = filepath.Join(workspace, "consumers", revision.RepositoryID)
		}
		if err = os.MkdirAll(destination, 0700); err != nil {
			return "", err
		}
		archive := exec.Command("git", "--git-dir="+repository.Path(), "archive", revision.CommitID)
		extract := exec.Command("tar", "-x", "-C", destination)
		pipe, pipeErr := extract.StdinPipe()
		if pipeErr != nil {
			return "", pipeErr
		}
		if err = extract.Start(); err != nil {
			return "", err
		}
		archive.Stdout = pipe
		if err = archive.Run(); err != nil {
			_ = pipe.Close()
			return "", err
		}
		_ = pipe.Close()
		if err = extract.Wait(); err != nil {
			return "", err
		}
	}
	command := exec.Command("git", "-C", workspace, "add", "--all")
	if output, runErr := command.CombinedOutput(); runErr != nil {
		return "", fmt.Errorf("stage candidate: %s", output)
	}
	command = exec.Command("git", "-C", workspace, "-c", "user.name=Vivarium Contract", "-c", "user.email=contract@vivarium", "commit", "-q", "-m", "Immutable cross-repository contract candidate")
	command.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2000-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z")
	if output, runErr := command.CombinedOutput(); runErr != nil {
		return "", fmt.Errorf("commit candidate: %s", output)
	}
	commitBody, err := exec.Command("git", "-C", workspace, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(string(commitBody))
	provider, err := gitStore.Open(providerRepositoryID)
	if err != nil {
		return "", err
	}
	if output, fetchErr := exec.Command("git", "--git-dir="+provider.Path(), "fetch", "-q", workspace, commit).CombinedOutput(); fetchErr != nil {
		return "", fmt.Errorf("import candidate: %s", output)
	}
	return commit, nil
}
