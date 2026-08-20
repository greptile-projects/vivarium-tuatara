package main

import (
	"errors"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capabilities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

type capabilityInput struct {
	ExpectedVersion int                   `json:"expected_version"`
	Revision        capabilities.Revision `json:"revision"`
}

type retirementEventInput struct {
	ExpectedVersion int                          `json:"expected_version"`
	Event           capabilities.RetirementEvent `json:"event"`
}

func registerCapabilityRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, inventory *capabilities.Store, releaseStore *releases.Store, proposalStore *proposals.Store, pulls *pullrequests.Store, sessions *changesessions.Store, workspaceStore *workspaces.Store) {
	mux.HandleFunc("GET /repositories/{id}/capabilities", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		out, err := inventory.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "capabilities_unavailable", "capability inventory could not be read")
			return
		}
		out = projectCapabilityCandidateFreshness(git, r.PathValue("id"), out)
		writeJSON(w, 200, map[string]any{"capabilities": projectCapabilitiesForReader(catalog, actor.UserID, projectCapabilityWork(out, actor.UserID, proposalStore, pulls, sessions, workspaceStore))})
	})
	mux.HandleFunc("GET /repositories/{id}/capabilities/{capability_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		out, err := inventory.Get(r.PathValue("id"), r.PathValue("capability_id"))
		if err != nil {
			writeAPIError(w, 404, "capability_not_found", "capability not found")
			return
		}
		out = projectCapabilityCandidateFreshness(git, r.PathValue("id"), []capabilities.Capability{out})[0]
		writeJSON(w, 200, projectCapabilitiesForReader(catalog, actor.UserID, projectCapabilityWork([]capabilities.Capability{out}, actor.UserID, proposalStore, pulls, sessions, workspaceStore))[0])
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in capabilityInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete capability revision is required")
				return
			}
			if !capabilityProvenanceResolves(git, releaseStore, r.PathValue("id"), &in.Revision) {
				writeAPIError(w, 400, "invalid_capability_provenance", "the release, commits, and selected paths must resolve exactly")
				return
			}
			ownerIDs := append([]string{actor.UserID}, in.Revision.OwnerIDs...)
			consumerRepositories := []string{}
			for _, consumer := range in.Revision.Consumers {
				if consumer.RepositoryID != "" {
					consumerRepositories = append(consumerRepositories, consumer.RepositoryID)
				}
			}
			var out capabilities.Capability
			var err error
			err = catalog.WithCurrentParticipantsAndReadAccess(ownerIDs, r.PathValue("id"), actor.UserID, consumerRepositories, func() error {
				if !capabilityConsumerProvenanceResolves(git, in.Revision) {
					return capabilities.ErrInvalid
				}
				if revise {
					out, err = inventory.Revise(r.PathValue("id"), r.PathValue("capability_id"), in.ExpectedVersion, actor.UserID, in.Revision)
				} else {
					out, err = inventory.Create(r.PathValue("id"), actor.UserID, in.Revision)
				}
				return err
			})
			writeCapability(w, out, err, map[bool]int{true: 200, false: 201}[revise])
		}
	}
	mux.HandleFunc("POST /repositories/{id}/capabilities", publish(false))
	mux.HandleFunc("POST /repositories/{id}/capabilities/{capability_id}/revisions", publish(true))
	mux.HandleFunc("POST /repositories/{id}/capabilities/{capability_id}/retirement-plans", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "retirement_plan_forbidden", "agents may assess plans but cannot open them")
			return
		}
		var plan capabilities.RetirementPlan
		if decodeJSON(r, &plan) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete retirement contract is required")
			return
		}
		out, err := inventory.OpenRetirement(r.PathValue("id"), r.PathValue("capability_id"), actor.UserID, plan)
		if err == nil {
			out = projectCapabilitiesForReader(catalog, actor.UserID, []capabilities.Capability{out})[0]
		}
		writeCapability(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/capabilities/{capability_id}/retirement-plans/{plan_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok || !authenticated {
			return
		}
		var in retirementEventInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an expected version and cited retirement event are required")
			return
		}
		if in.Event.Type == "policy_decision" {
			repository, repositoryErr := catalog.GetByID(r.PathValue("id"))
			collaborator, collaboratorErr := catalog.HasCollaborator(actor.UserID, r.PathValue("id"))
			if actor.AgentID != "" || repositoryErr != nil || (repository.OwnerID != actor.UserID && (collaboratorErr != nil || !collaborator)) {
				writeAPIError(w, 403, "retirement_policy_forbidden", "only a human repository participant may record a bounded policy decision")
				return
			}
		}
		actorID, actorType := actor.UserID, "human"
		if actor.AgentID != "" {
			actorID, actorType = actor.AgentID, "read_only_agent"
		}
		out, err := inventory.AppendRetirementEvent(r.PathValue("id"), r.PathValue("capability_id"), r.PathValue("plan_id"), actorID, actorType, in.ExpectedVersion, in.Event)
		if err == nil {
			out = projectCapabilitiesForReader(catalog, actor.UserID, []capabilities.Capability{out})[0]
		}
		writeCapability(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/capabilities/{capability_id}/retirement-plans/{plan_id}/work", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok || !authenticated {
			return
		}
		var in struct {
			ExpectedVersion    int                         `json:"expected_version"`
			RepositoryID       string                      `json:"repository_id"`
			Title              string                      `json:"title"`
			CompletionCriteria string                      `json:"completion_criteria"`
			AssigneeType       string                      `json:"assignee_type"`
			AssigneeID         string                      `json:"assignee_id"`
			Mandate            string                      `json:"mandate"`
			BaseRevision       string                      `json:"base_revision"`
			Work               capabilities.RetirementWork `json:"work"`
		}
		if decodeJSON(r, &in) != nil || proposalStore == nil || (in.AssigneeType != "human" && in.AssigneeType != "agent") {
			writeAPIError(w, 400, "invalid_retirement_work", "ordered work, an assignment, and an exact contract are required")
			return
		}
		target, err := catalog.GetByID(in.RepositoryID)
		collaborator, _ := catalog.HasCollaborator(actor.UserID, in.RepositoryID)
		if err != nil || (target.OwnerID != actor.UserID && !collaborator) {
			writeAPIError(w, 403, "retirement_work_forbidden", "a current target-repository participant must create its migration work")
			return
		}
		if strings.TrimSpace(in.Title) == "" || strings.ContainsAny(in.Title, "\r\n") || len([]rune(in.Title)) > 200 || strings.TrimSpace(in.CompletionCriteria) == "" || len([]rune(in.CompletionCriteria)) > 2000 || strings.TrimSpace(in.Mandate) == "" || len([]rune(in.Mandate)) > 4000 {
			writeAPIError(w, 422, "invalid_retirement_work", "title, completion criteria, and mandate are required and bounded")
			return
		}
		repository, err := git.Open(in.RepositoryID)
		if err != nil {
			writeAPIError(w, 422, "invalid_retirement_repository", "target repository storage is unavailable")
			return
		}
		in.BaseRevision = strings.ToLower(in.BaseRevision)
		if _, err = repository.ReadCommit(storage.ObjectID(in.BaseRevision)); err != nil {
			writeAPIError(w, 422, "invalid_base_revision", "base revision must be an existing target-repository commit")
			return
		}
		if in.AssigneeType == "human" {
			participant, _ := catalog.HasCollaborator(in.AssigneeID, in.RepositoryID)
			if in.AssigneeID != target.OwnerID && !participant {
				writeAPIError(w, 422, "invalid_task_assignee", "human assignee must participate in the target repository")
				return
			}
		}
		in.Work.RepositoryID = in.RepositoryID
		var proposal proposals.Proposal
		var assigned proposals.Task
		out, link, err := inventory.CreateRetirementWork(r.PathValue("id"), r.PathValue("capability_id"), r.PathValue("plan_id"), actor.UserID, in.ExpectedVersion, in.Work, func() (string, string, error) {
			body := retirementWorkBody(r.PathValue("capability_id"), r.PathValue("plan_id"), in.Work)
			var publishErr error
			proposal, publishErr = proposalStore.Create(in.RepositoryID, actor.UserID, in.Title, body)
			if publishErr != nil && !errors.Is(publishErr, proposals.ErrDurabilityUncertain) {
				return "", "", publishErr
			}
			task, taskErr := proposalStore.CreateTask(in.RepositoryID, proposal.ID, actor.UserID, in.Title, in.CompletionCriteria, nil, nil)
			if taskErr != nil && !errors.Is(taskErr, proposals.ErrDurabilityUncertain) {
				_ = proposalStore.DeleteMigrationWork(in.RepositoryID, proposal.ID, "", "")
				return "", "", taskErr
			}
			assigned, publishErr = proposalStore.AssignTask(in.RepositoryID, proposal.ID, task.ID, actor.UserID, proposals.TaskAssignmentInput{AssigneeType: in.AssigneeType, AssigneeID: in.AssigneeID, Mandate: in.Mandate, RepositoryID: in.RepositoryID, BaseRevision: in.BaseRevision})
			if publishErr != nil && !errors.Is(publishErr, proposals.ErrDurabilityUncertain) {
				_ = proposalStore.DeleteMigrationWork(in.RepositoryID, proposal.ID, task.ID, "")
				return "", "", publishErr
			}
			return proposal.ID, task.ID, nil
		})
		if err != nil {
			if errors.Is(err, capabilities.ErrDurabilityUncertain) {
				out = projectCapabilitiesForReader(catalog, actor.UserID, projectCapabilityWork([]capabilities.Capability{out}, actor.UserID, proposalStore, pulls, sessions, workspaceStore))[0]
				writeJSON(w, 202, map[string]any{"capability": out, "retirement_work": link, "task": assigned, "publication_state": "durability_uncertain"})
				return
			}
			if proposal.ID != "" && assigned.Assignment != nil {
				_ = proposalStore.DeleteMigrationWork(in.RepositoryID, proposal.ID, assigned.ID, assigned.Assignment.ID)
			}
			writeCapability(w, out, err, 201)
			return
		}
		out = projectCapabilitiesForReader(catalog, actor.UserID, projectCapabilityWork([]capabilities.Capability{out}, actor.UserID, proposalStore, pulls, sessions, workspaceStore))[0]
		writeJSON(w, 201, map[string]any{"capability": out, "retirement_work": link, "task": assigned})
	})
	mux.HandleFunc("POST /repositories/{id}/capabilities/{capability_id}/retirement-plans/{plan_id}/consumer-discoveries", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok || !authenticated {
			return
		}
		var in struct {
			ExpectedVersion int                            `json:"expected_version"`
			Discovery       capabilities.ConsumerDiscovery `json:"discovery"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_consumer_discovery", "exact consumer evidence is required")
			return
		}
		target, err := catalog.GetByID(in.Discovery.RepositoryID)
		collaborator, _ := catalog.HasCollaborator(actor.UserID, in.Discovery.RepositoryID)
		if err != nil || (target.OwnerID != actor.UserID && !collaborator) {
			writeAPIError(w, 403, "consumer_discovery_forbidden", "only a current consumer-repository participant may report its use")
			return
		}
		repository, err := git.Open(in.Discovery.RepositoryID)
		if err != nil {
			writeAPIError(w, 422, "invalid_consumer_discovery", "consumer repository is unavailable")
			return
		}
		in.Discovery.Revision = strings.ToLower(in.Discovery.Revision)
		commit, err := repository.ReadCommit(storage.ObjectID(in.Discovery.Revision))
		if err != nil {
			writeAPIError(w, 422, "invalid_consumer_discovery", "consumer revision must resolve exactly")
			return
		}
		entries, err := repository.WalkTree(commit.Tree)
		existing := map[string]bool{}
		if err == nil {
			for _, entry := range entries {
				if entry.Type == storage.BlobObject {
					existing[entry.Path] = true
				}
			}
		}
		for _, candidate := range in.Discovery.Paths {
			if !existing[candidate] {
				writeAPIError(w, 422, "invalid_consumer_discovery", "every reported path must exist at the exact consumer revision")
				return
			}
		}
		out, err := inventory.ReportRetirementConsumer(r.PathValue("id"), r.PathValue("capability_id"), r.PathValue("plan_id"), actor.UserID, in.ExpectedVersion, in.Discovery)
		if err == nil {
			out = projectCapabilitiesForReader(catalog, actor.UserID, projectCapabilityWork([]capabilities.Capability{out}, actor.UserID, proposalStore, pulls, sessions, workspaceStore))[0]
		}
		writeCapability(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/capabilities/{capability_id}/retirement-plans/{plan_id}/candidates", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var candidate capabilities.MigrationCandidate
		if decodeJSON(r, &candidate) != nil {
			writeAPIError(w, 400, "invalid_migration_candidate", "an exact bounded migration candidate is required")
			return
		}
		for _, check := range candidate.Checks {
			target, err := git.Open(check.RepositoryID)
			if err != nil {
				writeAPIError(w, 422, "invalid_candidate_revision", "every check repository and revision must resolve")
				return
			}
			commit, err := target.ReadCommit(storage.ObjectID(strings.ToLower(check.Revision)))
			if err != nil {
				writeAPIError(w, 422, "invalid_candidate_revision", "every check repository and revision must resolve")
				return
			}
			entries, err := target.WalkTree(commit.Tree)
			present := map[string]bool{}
			if err == nil {
				for _, entry := range entries {
					if entry.Type == storage.BlobObject {
						present[entry.Path] = true
					}
				}
			}
			for _, sourcePath := range check.Paths {
				if !present[sourcePath] {
					writeAPIError(w, 422, "invalid_candidate_path", "every affected path must exist at the exact check revision")
					return
				}
			}
		}
		out, created, err := inventory.CreateMigrationCandidate(r.PathValue("id"), r.PathValue("capability_id"), r.PathValue("plan_id"), actor.UserID, candidate)
		if err != nil {
			writeCapability(w, out, err, 201)
			return
		}
		out = projectCapabilitiesForReader(catalog, actor.UserID, []capabilities.Capability{out})[0]
		for _, plan := range out.RetirementPlans {
			if plan.ID == r.PathValue("plan_id") {
				for _, projected := range plan.Candidates {
					if projected.ID == created.ID {
						created = projected
					}
				}
			}
		}
		writeJSON(w, 201, map[string]any{"capability": out, "candidate": created})
	})
	mux.HandleFunc("POST /repositories/{id}/capabilities/{capability_id}/retirement-plans/{plan_id}/candidates/{candidate_id}/evidence", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			CheckID     string `json:"check_id"`
			WorkspaceID string `json:"workspace_id"`
		}
		if decodeJSON(r, &in) != nil || workspaceStore == nil {
			writeAPIError(w, 400, "invalid_candidate_evidence", "a retained bounded workspace check is required")
			return
		}
		current, err := inventory.Get(r.PathValue("id"), r.PathValue("capability_id"))
		var check *capabilities.CandidateCheck
		for pi := range current.RetirementPlans {
			if current.RetirementPlans[pi].ID != r.PathValue("plan_id") {
				continue
			}
			for ci := range current.RetirementPlans[pi].Candidates {
				if current.RetirementPlans[pi].Candidates[ci].ID != r.PathValue("candidate_id") {
					continue
				}
				for xi := range current.RetirementPlans[pi].Candidates[ci].Checks {
					if current.RetirementPlans[pi].Candidates[ci].Checks[xi].ID == in.CheckID {
						check = &current.RetirementPlans[pi].Candidates[ci].Checks[xi]
					}
				}
			}
		}
		if err != nil || check == nil {
			writeAPIError(w, 404, "migration_candidate_not_found", "candidate check not found")
			return
		}
		ws, err := workspaceStore.Get(in.WorkspaceID)
		if err != nil || ws.CreatorID != actor.UserID || ws.RepositoryID != check.RepositoryID || ws.CommitID != strings.ToLower(check.Revision) {
			writeAPIError(w, 422, "invalid_candidate_workspace", "evidence must come from the caller's exact-revision bounded workspace")
			return
		}
		digest := capabilities.CommandDigest(check.Command)
		var selected *workspaces.CommandOutcome
		for i := len(ws.Commands) - 1; i >= 0; i-- {
			x := ws.Commands[i]
			if x.CommandSHA256 == digest && !x.CompletedAt.Before(x.StartedAt) && len(x.Output) <= 65500 && !reusableSecret.MatchString(x.Output) {
				selected = &x
				break
			}
		}
		if selected == nil {
			writeAPIError(w, 422, "missing_candidate_outcome", "the workspace has no safe retained outcome for the exact command")
			return
		}
		status := "passed"
		if selected.ExitCode != 0 {
			status = "failed"
		}
		duration := selected.CompletedAt.Sub(selected.StartedAt)
		cost := float64(duration.Milliseconds()) / 1000 * (ws.Definition.Resources.CPUs + float64(ws.Definition.Resources.MemoryMB)/1024)
		evidence := capabilities.CandidateEvidence{WorkspaceID: ws.ID, OutcomeID: selected.ID, Status: status, ExitCode: selected.ExitCode, SanitizedLog: selected.Output, CommandDigest: digest, DurationMS: duration.Milliseconds(), CostUnits: cost}
		for _, change := range ws.Changes {
			if reusableSecret.MatchString(change.Path) {
				writeAPIError(w, 422, "unsafe_candidate_artifact", "artifact metadata must be sanitized")
				return
			}
			evidence.Artifacts = append(evidence.Artifacts, capabilities.CandidateArtifact{Path: change.Path, Digest: change.SHA256, Size: change.Size})
		}
		out, err := inventory.AddCandidateEvidence(r.PathValue("id"), r.PathValue("capability_id"), r.PathValue("plan_id"), r.PathValue("candidate_id"), actor.UserID, in.CheckID, evidence)
		if err == nil {
			out = projectCapabilitiesForReader(catalog, actor.UserID, []capabilities.Capability{out})[0]
		}
		writeCapability(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/capabilities/{capability_id}/retirement-plans/{plan_id}/candidates/{candidate_id}/usage-observations", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok || !authenticated {
			return
		}
		var observation capabilities.UsageObservation
		if decodeJSON(r, &observation) != nil {
			writeAPIError(w, 400, "invalid_usage_observation", "a bounded usage observation is required")
			return
		}
		current, err := inventory.Get(r.PathValue("id"), r.PathValue("capability_id"))
		var consumer *capabilities.Consumer
		var audience *capabilities.Audience
		for pi := range current.RetirementPlans {
			p := &current.RetirementPlans[pi]
			if p.ID == r.PathValue("plan_id") && observation.ConsumerIndex >= 0 && observation.ConsumerIndex < len(p.Audiences) && p.CapabilityVersion > 0 && p.CapabilityVersion <= len(current.Revisions) {
				audience = &p.Audiences[observation.ConsumerIndex]
				consumer = &current.Revisions[p.CapabilityVersion-1].Consumers[observation.ConsumerIndex]
			}
		}
		if err != nil || consumer == nil || audience == nil {
			writeAPIError(w, 422, "invalid_usage_consumer", "the frozen consumer must resolve")
			return
		}
		allowed := containsString(audience.OwnerIDs, actor.UserID)
		if consumer.RepositoryID != "" {
			collaborator, _ := catalog.HasCollaborator(actor.UserID, consumer.RepositoryID)
			target, _ := catalog.GetByID(consumer.RepositoryID)
			allowed = allowed && (target.OwnerID == actor.UserID || collaborator)
		}
		if !allowed {
			writeAPIError(w, 403, "usage_observation_forbidden", "an exact affected owner with consumer access must report use")
			return
		}
		observation.RepositoryID = consumer.RepositoryID
		observation.Revision = consumer.Revision
		observation.OwnerID = actor.UserID
		out, err := inventory.AddUsageObservation(r.PathValue("id"), r.PathValue("capability_id"), r.PathValue("plan_id"), r.PathValue("candidate_id"), actor.UserID, observation)
		if err == nil {
			out = projectCapabilitiesForReader(catalog, actor.UserID, []capabilities.Capability{out})[0]
		}
		writeCapability(w, out, err, 201)
	})
}

func projectCapabilityCandidateFreshness(git *storage.Store, providerID string, values []capabilities.Capability) []capabilities.Capability {
	for vi := range values {
		if len(values[vi].Revisions) == 0 {
			continue
		}
		current := values[vi].Revisions[len(values[vi].Revisions)-1]
		consumerRevisions := map[string]string{}
		for _, consumer := range current.Consumers {
			consumerRevisions[consumer.RepositoryID] = consumer.Revision
		}
		for pi := range values[vi].RetirementPlans {
			plan := &values[vi].RetirementPlans[pi]
			for ci := range plan.Candidates {
				candidate := &plan.Candidates[ci]
				for xi := range candidate.Checks {
					check := &candidate.Checks[xi]
					target := consumerRevisions[check.RepositoryID]
					if check.RepositoryID == providerID {
						target = current.CommitID
					}
					stale := target == "" || !sameGitPaths(git, check.RepositoryID, check.Revision, target, check.Paths)
					for ei := range check.Evidence {
						check.Evidence[ei].Stale = stale
					}
				}
				capabilities.ProjectCandidate(candidate, *plan)
				for _, usage := range candidate.Usage {
					if usage.Superseded || usage.RepositoryID == "" {
						continue
					}
					if consumerRevisions[usage.RepositoryID] != usage.Revision {
						candidate.Blockers = append(candidate.Blockers, capabilities.RetirementBlocker{Kind: "usage_revision_stale", Message: "The consumer revision changed after this usage window was retained."})
						candidate.RemovalReady = false
						candidate.Status = "blocked"
					}
				}
			}
		}
	}
	return values
}

func sameGitPaths(git *storage.Store, repositoryID, fromRevision, toRevision string, paths []string) bool {
	if fromRevision == toRevision {
		return true
	}
	repo, err := git.Open(repositoryID)
	if err != nil {
		return false
	}
	ids := func(revision string) (map[string]storage.ObjectID, bool) {
		commit, e := repo.ReadCommit(storage.ObjectID(strings.ToLower(revision)))
		if e != nil {
			return nil, false
		}
		entries, e := repo.WalkTree(commit.Tree)
		if e != nil {
			return nil, false
		}
		out := map[string]storage.ObjectID{}
		for _, entry := range entries {
			if entry.Type == storage.BlobObject {
				out[entry.Path] = entry.ID
			}
		}
		return out, true
	}
	from, ok := ids(fromRevision)
	if !ok {
		return false
	}
	to, ok := ids(toRevision)
	if !ok {
		return false
	}
	for _, path := range paths {
		if from[path] == "" || from[path] != to[path] {
			return false
		}
	}
	return true
}

func retirementWorkBody(capabilityID, planID string, work capabilities.RetirementWork) string {
	return "Capability retirement " + planID + " for capability " + capabilityID + ".\n\nOld contract\n" + work.OldContract + "\n\nSupported replacement contract\n" + work.ReplacementContract + "\n\nAcceptance criteria\n- " + strings.Join(work.AcceptanceCriteria, "\n- ") + "\n\nDocumentation changes\n- " + strings.Join(work.DocumentationChanges, "\n- ") + "\n\nRollout stage: " + work.RolloutStage + "\n\nThis context grants the retiring provider no repository, Git, agent, review, merge, release, or deployment authority."
}

func projectCapabilityWork(values []capabilities.Capability, actorID string, proposalStore *proposals.Store, pulls *pullrequests.Store, sessions *changesessions.Store, workspaceStore *workspaces.Store) []capabilities.Capability {
	if proposalStore == nil {
		return values
	}
	var actorWorkspaces []workspaces.Workspace
	if workspaceStore != nil && actorID != "" {
		actorWorkspaces, _ = workspaceStore.List(actorID)
	}
	for ci := range values {
		for pi := range values[ci].RetirementPlans {
			plan := &values[ci].RetirementPlans[pi]
			completed := map[string]bool{}
			for wi := range plan.Work {
				work := &plan.Work[wi]
				if task, err := proposalStore.GetTask(work.RepositoryID, work.ProposalID, work.TaskID); err == nil {
					work.Status = task.Status
					completed[work.ID] = task.Status == proposals.TaskCompleted && task.Contribution != nil && task.Contribution.Status == "merged" && task.Contribution.ContextRevision == task.ContextRevision
					if task.Assignment != nil {
						work.AssignmentID, work.AssigneeType, work.AssigneeID, work.BaseRevision = task.Assignment.ID, task.Assignment.AssigneeType, task.Assignment.AssigneeID, task.Assignment.Access.BaseRevision
					}
					if task.Contribution != nil {
						work.PullRequestID, work.ContributionStatus = task.Contribution.PullRequestID, task.Contribution.Status
						if pulls != nil {
							if pull, pullErr := pulls.Get(work.RepositoryID, work.PullRequestID); pullErr == nil && pull.SourceRepositoryID != work.RepositoryID {
								work.ForkRepositoryID = pull.SourceRepositoryID
							}
						}
					}
				}
				if sessions != nil {
					if list, sessionErr := sessions.List(work.RepositoryID, work.TaskID); sessionErr == nil && len(list) > 0 {
						work.SessionID = list[len(list)-1].ID
					}
				}
				for _, workspace := range actorWorkspaces {
					if workspace.RepositoryID == work.RepositoryID && workspace.Source.Kind == "proposal_task" && workspace.Source.ProposalID == work.ProposalID && workspace.Source.TaskID == work.TaskID {
						work.WorkspaceID = workspace.ID
						break
					}
				}
			}
			for wi := range plan.Work {
				plan.Work[wi].Ready = plan.Work[wi].Status == proposals.TaskTodo
				for _, dependency := range plan.Work[wi].DependencyIDs {
					plan.Work[wi].Ready = plan.Work[wi].Ready && completed[dependency]
				}
			}
		}
	}
	return values
}

func projectCapabilitiesForReader(catalog *repositories.Store, actorID string, values []capabilities.Capability) []capabilities.Capability {
	canRead := func(id string) bool {
		if id == "" {
			return true
		}
		repository, err := catalog.GetByID(id)
		if err != nil {
			return false
		}
		if repository.Visibility == repositories.Public || repository.OwnerID == actorID {
			return true
		}
		allowed, err := catalog.HasCollaborator(actorID, id)
		return err == nil && allowed
	}
	for valueIndex := range values {
		restrictedCurrentConsumers := map[int]bool{}
		restrictedConsumersByVersion := map[int]map[int]bool{}
		for revisionIndex := range values[valueIndex].Revisions {
			revision := &values[valueIndex].Revisions[revisionIndex]
			restrictedConsumers := map[int]bool{}
			restrictedConsumersByVersion[revision.Version] = restrictedConsumers
			for consumerIndex := range revision.Consumers {
				consumer := &revision.Consumers[consumerIndex]
				if canRead(consumer.RepositoryID) {
					continue
				}
				restrictedConsumers[consumerIndex] = true
				if revisionIndex == len(values[valueIndex].Revisions)-1 {
					restrictedCurrentConsumers[consumerIndex] = true
				}
				*consumer = capabilities.Consumer{Name: "restricted", Environment: "restricted", Discovery: "unknown", EvidenceState: "inaccessible", CompatibilityPromise: "restricted"}
			}
		}
		for planIndex := range values[valueIndex].RetirementPlans {
			plan := &values[valueIndex].RetirementPlans[planIndex]
			restrictedPlanConsumers, boundRevisionFound := restrictedConsumersByVersion[plan.CapabilityVersion]
			if !boundRevisionFound {
				restrictedPlanConsumers = map[int]bool{}
				for consumerIndex := range plan.Audiences {
					restrictedPlanConsumers[consumerIndex] = true
				}
			}
			restrictedAudiences := map[string]bool{}
			restrictedOwners := map[string]bool{}
			for consumerIndex := range plan.Audiences {
				if !restrictedPlanConsumers[consumerIndex] {
					continue
				}
				restrictedAudiences[plan.Audiences[consumerIndex].Name] = true
				for _, ownerID := range plan.Audiences[consumerIndex].OwnerIDs {
					restrictedOwners[ownerID] = true
				}
				plan.Audiences[consumerIndex] = capabilities.Audience{Name: "restricted", OwnerIDs: []string{"restricted"}, Impact: "restricted affected audience", Commitment: "restricted", EmbargoedDependency: true}
			}
			for diagnosticIndex := range plan.FrozenDiagnostics {
				diagnostic := &plan.FrozenDiagnostics[diagnosticIndex]
				if diagnostic.ConsumerIndex != nil && restrictedPlanConsumers[*diagnostic.ConsumerIndex] {
					diagnostic.Consumer = "restricted"
				}
			}
			for blockerIndex := range plan.Blockers {
				blocker := &plan.Blockers[blockerIndex]
				if blocker.ConsumerIndex != nil && restrictedCurrentConsumers[*blocker.ConsumerIndex] {
					blocker.Audience = "restricted"
				}
				if restrictedAudiences[blocker.Audience] {
					blocker.Audience = "restricted"
				}
				if restrictedOwners[blocker.OwnerID] {
					blocker.OwnerID = "restricted"
				}
			}
			for ownerIndex := range plan.RequiredOwnerIDs {
				if restrictedOwners[plan.RequiredOwnerIDs[ownerIndex]] {
					plan.RequiredOwnerIDs[ownerIndex] = "restricted"
				}
			}
			for eventIndex := range plan.Events {
				event := &plan.Events[eventIndex]
				if restrictedOwners[event.ActorID] || restrictedOwners[event.OwnerID] {
					event.ActorID = "restricted"
					event.OwnerID = "restricted"
					event.Summary = "restricted owner response"
					event.Evidence = nil
				}
			}
			for exceptionIndex := range plan.Exceptions {
				if restrictedAudiences[plan.Exceptions[exceptionIndex].Audience] {
					plan.Exceptions[exceptionIndex].Audience = "restricted"
				}
			}
			visibleWork := plan.Work[:0]
			for _, work := range plan.Work {
				if restrictedPlanConsumers[work.AudienceIndex] || !canRead(work.RepositoryID) {
					continue
				}
				visibleWork = append(visibleWork, work)
			}
			plan.Work = visibleWork
			visibleDiscoveries := plan.DiscoveredConsumers[:0]
			for _, discovery := range plan.DiscoveredConsumers {
				if canRead(discovery.RepositoryID) {
					visibleDiscoveries = append(visibleDiscoveries, discovery)
				}
			}
			plan.DiscoveredConsumers = visibleDiscoveries
			for candidateIndex := range plan.Candidates {
				candidate := &plan.Candidates[candidateIndex]
				for consumerIndex := range candidate.Consumers {
					if !canRead(candidate.Consumers[consumerIndex].RepositoryID) {
						candidate.Consumers[consumerIndex] = capabilities.CandidateRevision{RepositoryID: "restricted"}
					}
				}
				for checkIndex := range candidate.Checks {
					check := &candidate.Checks[checkIndex]
					if canRead(check.RepositoryID) {
						continue
					}
					check.RepositoryID, check.Revision, check.Command, check.Paths, check.Expectation = "restricted", "", "", nil, "restricted check"
					for evidenceIndex := range check.Evidence {
						evidence := &check.Evidence[evidenceIndex]
						evidence.WorkspaceID = ""
						evidence.OutcomeID = ""
						evidence.SanitizedLog = ""
						evidence.Artifacts = nil
						evidence.CreatedBy = "restricted"
					}
				}
				for usageIndex := range candidate.Usage {
					usage := &candidate.Usage[usageIndex]
					if usage.ConsumerIndex >= 0 && restrictedPlanConsumers[usage.ConsumerIndex] {
						usage.RepositoryID = "restricted"
						usage.Revision = ""
						usage.Summary = "restricted usage observation"
						usage.ArtifactDigest = ""
						usage.OwnerID = "restricted"
						usage.State = "inaccessible"
					}
				}
			}
		}
		for diagnosticIndex := range values[valueIndex].Diagnostics {
			diagnostic := &values[valueIndex].Diagnostics[diagnosticIndex]
			if diagnostic.ConsumerIndex != nil && restrictedCurrentConsumers[*diagnostic.ConsumerIndex] {
				diagnostic.Consumer = "restricted"
			}
		}
	}
	return values
}

func capabilityConsumerProvenanceResolves(git *storage.Store, revision capabilities.Revision) bool {
	for _, consumer := range revision.Consumers {
		if consumer.EvidenceState != "current" {
			continue
		}
		repository, err := git.Open(consumer.RepositoryID)
		if err != nil {
			return false
		}
		if _, err = repository.ReadCommit(storage.ObjectID(strings.ToLower(consumer.Revision))); err != nil {
			return false
		}
	}
	return true
}

func capabilityProvenanceResolves(git *storage.Store, releases *releases.Store, repoID string, r *capabilities.Revision) bool {
	if git == nil || releases == nil {
		return false
	}
	release, err := releases.Get(repoID, r.ReleaseID)
	if err != nil || release.CommitID != strings.ToLower(r.CommitID) {
		return false
	}
	r.CommitID = release.CommitID
	r.ReleaseVersion = release.Version
	repo, err := git.Open(repoID)
	if err != nil {
		return false
	}
	trees := map[string]map[string]bool{}
	for i := range r.Items {
		x := &r.Items[i]
		x.Revision = strings.ToLower(x.Revision)
		if _, ok := trees[x.Revision]; !ok {
			commit, e := repo.ReadCommit(storage.ObjectID(x.Revision))
			if e != nil {
				return false
			}
			entries, e := repo.WalkTree(commit.Tree)
			if e != nil {
				return false
			}
			files := map[string]bool{}
			for _, entry := range entries {
				if entry.Type == storage.BlobObject {
					files[entry.Path] = true
				}
			}
			trees[x.Revision] = files
		}
		if x.Kind != "release" {
			if x.Path == "" || strings.HasPrefix(x.Path, "/") || path.Clean(x.Path) != x.Path || x.Path == "." || strings.HasPrefix(x.Path, "../") || !trees[x.Revision][x.Path] {
				return false
			}
		}
	}
	return true
}
func writeCapability(w http.ResponseWriter, v capabilities.Capability, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, capabilities.ErrNotFound):
		writeAPIError(w, 404, "capability_not_found", "capability not found")
	case errors.Is(err, capabilities.ErrPlanNotFound):
		writeAPIError(w, 404, "retirement_plan_not_found", "retirement plan not found")
	case errors.Is(err, capabilities.ErrConflict):
		writeAPIError(w, 409, "capability_conflict", "the capability changed; reload before publishing")
	case errors.Is(err, capabilities.ErrInvalid):
		writeAPIError(w, 400, "invalid_capability", "define exact capability evidence or a complete, bounded retirement contract")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "capability_forbidden", "only current repository participants may own or publish capabilities")
	default:
		log.Printf("capability storage: %v", err)
		writeAPIError(w, 500, "capabilities_unavailable", "capability inventory could not be persisted")
	}
}
