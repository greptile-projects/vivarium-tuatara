package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributorpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerLearningPathwayRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, pathways *learningpathways.Store, issueStore *issues.Store, proposalStore *proposals.Store, packageStore *packages.Store, contributorStore *contributorpathways.Store, workspaceStore *workspaces.Store, orgs *organizations.Store, credentials *auth.Store) {
	isParticipant := func(repositoryID, userID string) bool {
		repo, e := repos.GetByID(repositoryID)
		if e != nil {
			return false
		}
		if repo.OwnerID == userID {
			return true
		}
		ok, _ := repos.HasCollaborator(userID, repositoryID)
		return ok
	}
	present := func(repositoryID, actorID string, v learningpathways.Revision) learningpathways.Revision {
		repo, _ := repos.GetByID(repositoryID)
		canReadRestricted := actorID != "" && isParticipant(repositoryID, actorID)
		gr, _ := git.Open(repositoryID)
		defaultRevision := ""
		if gr != nil {
			if ref, e := gr.ReadReference("refs/heads/" + repo.DefaultBranch); e == nil {
				defaultRevision = ref.Target
			}
		}
		for i := range v.Mentors {
			m := &v.Mentors[i]
			if !isParticipant(repositoryID, m.UserID) {
				m.Status, m.StatusDetail = "inaccessible", "The designated mentor is no longer a repository participant."
			} else {
				m.Status, m.StatusDetail = "current", "The designated mentor is a current repository participant."
			}
		}
		for i := range v.Environments {
			e := &v.Environments[i]
			if !e.Supported {
				e.Status, e.StatusDetail = "unsupported", "This learner environment is explicitly unsupported."
			} else if e.OwnerID == "" || !isParticipant(repositoryID, e.OwnerID) {
				e.Status, e.StatusDetail = "missing_owner", "No current collaborator owns support for this environment."
			} else {
				e.Status, e.StatusDetail = "current", "This environment has a current support owner."
			}
		}
		for mi := range v.Modules {
			for li := range v.Modules[mi].Materials {
				l := &v.Modules[mi].Materials[li]
				l.Status, l.StatusDetail = "current", "The exact learning material is available."
				if l.OwnerID == "" || !isParticipant(repositoryID, l.OwnerID) {
					l.Status, l.StatusDetail = "missing_owner", "The material has no current collaborator owner."
					continue
				}
				switch l.Kind {
				case "documentation", "symbol", "api":
					if gr == nil {
						l.Status, l.StatusDetail = "inaccessible", "Repository content is unavailable."
						break
					}
					c, e := gr.ReadCommit(storage.ObjectID(l.Revision))
					if e != nil {
						l.Status, l.StatusDetail = "inaccessible", "The exact revision is unavailable."
						break
					}
					entries, e := gr.WalkTree(c.Tree)
					found := false
					var content string
					for _, x := range entries {
						if x.Path == l.Path && x.Type == storage.BlobObject {
							found = true
							if l.Kind == "symbol" {
								if b, _, _, er := gr.ReadBlobPreview(x.ID, 1<<20); er == nil {
									content = string(b.Content)
								}
							}
							break
						}
					}
					if e != nil || !found {
						l.Status, l.StatusDetail = "stale", "The exact path is missing at the supported revision."
					} else if l.Kind == "symbol" && !strings.Contains(content, l.Symbol) {
						l.Status, l.StatusDetail = "stale", "The named symbol is missing at the exact revision."
					} else if defaultRevision != "" && defaultRevision != l.Revision {
						l.Status, l.StatusDetail = "stale", "The default branch has moved beyond this exact material revision."
					}
				case "decision":
					if proposalStore == nil {
						l.Status, l.StatusDetail = "inaccessible", "Decision records are unavailable."
					} else if _, e := proposalStore.Get(repositoryID, l.ResourceID); e != nil {
						l.Status, l.StatusDetail = "stale", "The linked decision is unavailable."
					}
				case "issue":
					if issueStore == nil {
						l.Status, l.StatusDetail = "inaccessible", "Issue records are unavailable."
					} else if linked, e := issueStore.Get(repositoryID, l.ResourceID); e != nil {
						l.Status, l.StatusDetail = "stale", "The linked issue is unavailable."
					} else if linked.Visibility != "public" && !canReadRestricted {
						l.Status, l.StatusDetail = "inaccessible", "The linked issue is not accessible."
						l.ResourceID, l.Label = "", "Restricted issue"
					}
				case "package":
					if packageStore == nil {
						l.Status, l.StatusDetail = "inaccessible", "Package records are unavailable."
					} else if _, e := packageStore.Get(l.ResourceID, l.PackageVersion); e != nil {
						l.Status, l.StatusDetail = "stale", "The exact package version is unavailable."
					}
				case "contributor_guidance":
					version, e := strconv.Atoi(l.ResourceID)
					if contributorStore == nil {
						l.Status, l.StatusDetail = "inaccessible", "Contributor guidance records are unavailable."
						break
					}
					history, he := contributorStore.List(repositoryID)
					if e != nil || he != nil || version < 1 || version > len(history) {
						l.Status, l.StatusDetail = "stale", "The exact contributor guidance revision is unavailable."
					} else if version != len(history) {
						l.Status, l.StatusDetail = "stale", "Newer contributor guidance has been published."
					}
				}
			}
		}
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/learning-pathways", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		slugs, e := pathways.Slugs(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "learning_pathway_read_failed", "learning pathways could not be read")
			return
		}
		out := []learningpathways.Revision{}
		actorID := ""
		if authenticated {
			actorID = actor.UserID
		}
		for _, s := range slugs {
			if v, e := pathways.Current(r.PathValue("id"), s); e == nil {
				out = append(out, present(r.PathValue("id"), actorID, v))
			}
		}
		writeJSON(w, 200, map[string]any{"pathways": out})
	})
	mux.HandleFunc("GET /repositories/{id}/learning-pathways/{slug}", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := pathways.Current(r.PathValue("id"), r.PathValue("slug"))
		if errors.Is(e, learningpathways.ErrNotFound) {
			writeAPIError(w, 404, "learning_pathway_not_found", "learning pathway not found")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "learning_pathway_read_failed", "learning pathway could not be read")
			return
		}
		history, e := pathways.List(r.PathValue("id"), r.PathValue("slug"))
		if e != nil {
			writeAPIError(w, 500, "learning_pathway_read_failed", "learning pathway could not be read")
			return
		}
		actorID := ""
		if authenticated {
			actorID = actor.UserID
		}
		projectedHistory := make([]learningpathways.Revision, 0, len(history))
		for _, historical := range history {
			projectedHistory = append(projectedHistory, present(r.PathValue("id"), actorID, historical))
		}
		writeJSON(w, 200, map[string]any{"pathway": present(r.PathValue("id"), actorID, v), "history": projectedHistory})
	})
	mux.HandleFunc("PUT /repositories/{id}/learning-pathways/{slug}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                       `json:"expected_version"`
			RequestID       string                    `json:"request_id"`
			Pathway         learningpathways.Revision `json:"pathway"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.Pathway.ID = ""
		in.Pathway.Version = 0
		in.Pathway.RepositoryID = r.PathValue("id")
		in.Pathway.Slug = r.PathValue("slug")
		in.Pathway.PublishedBy = actor.UserID
		in.Pathway.RequestID = in.RequestID
		for i := range in.Pathway.Mentors {
			in.Pathway.Mentors[i].Status, in.Pathway.Mentors[i].StatusDetail = "", ""
		}
		for i := range in.Pathway.Environments {
			in.Pathway.Environments[i].Status, in.Pathway.Environments[i].StatusDetail = "", ""
		}
		for i := range in.Pathway.Modules {
			for j := range in.Pathway.Modules[i].Materials {
				in.Pathway.Modules[i].Materials[j].Status, in.Pathway.Modules[i].Materials[j].StatusDetail = "", ""
			}
		}
		v, e := pathways.Publish(in.Pathway, in.ExpectedVersion)
		if errors.Is(e, learningpathways.ErrConflict) {
			writeAPIError(w, 409, "learning_pathway_changed", "learning pathway version changed")
			return
		}
		if errors.Is(e, learningpathways.ErrRequestChanged) {
			writeAPIError(w, 409, "learning_pathway_request_changed", "request identity was already used with different pathway content")
			return
		}
		if errors.Is(e, learningpathways.ErrInvalid) {
			writeAPIError(w, 422, "invalid_learning_pathway", "the complete pathway, ordered modules, exact materials, exercises, and evidence are required")
			return
		}
		if e != nil && !errors.Is(e, learningpathways.ErrDurabilityUncertain) {
			writeAPIError(w, 500, "learning_pathway_write_failed", "learning pathway could not be published")
			return
		}
		status := 201
		if errors.Is(e, learningpathways.ErrDurabilityUncertain) {
			status = 202
			w.Header().Set("Vivarium-Durability", "uncertain")
		}
		w.Header().Set("Location", "/repositories/"+r.PathValue("id")+"/learning-pathways/"+r.PathValue("slug"))
		writeJSON(w, status, present(r.PathValue("id"), actor.UserID, v))
	})
	// Outcomes are explicit, consented learner reports. Reads never expose a
	// private subject through aggregation, and aggregate-only cohorts smaller
	// than three remain suppressed.
	mux.HandleFunc("POST /repositories/{id}/learning-pathways/{slug}/outcomes", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		var in learningpathways.Outcome
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.ID, in.RepositoryID, in.PathwaySlug, in.ActorID = "", r.PathValue("id"), r.PathValue("slug"), actor.UserID
		history, e := pathways.List(in.RepositoryID, in.PathwaySlug)
		if e != nil || in.PathwayVersion < 1 || in.PathwayVersion > len(history) {
			writeAPIError(w, 422, "learning_outcome_revision_invalid", "the exact archived pathway revision is required")
			return
		}
		created, e := pathways.AddOutcome(in)
		if e != nil {
			writeAPIError(w, 422, "learning_outcome_invalid", "consent, visibility, category, state, and a stable request identity are required")
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/learning-pathways/{slug}/outcomes", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		items, e := pathways.Outcomes(r.PathValue("id"), r.PathValue("slug"))
		if e != nil {
			writeAPIError(w, 500, "learning_outcomes_read_failed", "learning outcomes could not be read")
			return
		}
		actorID := ""
		maintainer := false
		if authenticated {
			actorID = actor.UserID
			maintainer = isParticipant(r.PathValue("id"), actorID)
		}
		visible := []learningpathways.Outcome{}
		counts := map[string]int{}
		states := map[string]map[string]int{}
		for _, x := range items {
			if x.Visibility == "aggregate" {
				counts[x.Kind]++
				if states[x.Kind] == nil {
					states[x.Kind] = map[string]int{}
				}
				states[x.Kind][x.State]++
			}
			if x.ActorID == actorID || maintainer && x.Visibility == "maintainers" {
				visible = append(visible, x)
			}
		}
		for kind, n := range counts {
			if n < 3 {
				delete(counts, kind)
				delete(states, kind)
			}
		}
		current, _ := pathways.Current(r.PathValue("id"), r.PathValue("slug"))
		stale := 0
		for _, x := range visible {
			if x.Kind == "module_completion" && x.PathwayVersion != current.Version {
				stale++
			}
		}
		writeJSON(w, 200, map[string]any{"outcomes": visible, "aggregates": counts, "aggregate_states": states, "suppression_threshold": 3, "stale_completion_evidence": stale, "current_pathway_version": current.Version})
	})
	mux.HandleFunc("POST /repositories/{id}/learning-pathways/{slug}/findings", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in learningpathways.Finding
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.ID, in.RepositoryID, in.PathwaySlug, in.CreatedBy = "", r.PathValue("id"), r.PathValue("slug"), actor.UserID
		v, e := pathways.AddFinding(in)
		if e != nil {
			writeAPIError(w, 422, "learning_finding_unsupported", "a finding must cite consented non-private outcomes at its exact pathway revision")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/learning-pathways/{slug}/update-proposals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in learningpathways.UpdateProposal
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.ID, in.RepositoryID, in.PathwaySlug, in.ProposedBy = "", r.PathValue("id"), r.PathValue("slug"), actor.UserID
		v, e := pathways.AddProposal(in)
		if e != nil {
			writeAPIError(w, 422, "learning_update_unsupported", "the update must cite a supported finding and an exact base revision")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/learning-pathways/{slug}/update-proposals/{proposal_id}/review", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Decision  string `json:"decision"`
			Rationale string `json:"rationale"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, e := pathways.ReviewProposal(r.PathValue("id"), r.PathValue("slug"), r.PathValue("proposal_id"), actor.UserID, in.Decision, in.Rationale)
		if e != nil {
			writeAPIError(w, 422, "learning_update_review_invalid", "a pending proposal, accepted or rejected decision, and rationale are required")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /repositories/{id}/learning-pathways/{slug}/improvements", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		fs, e := pathways.Findings(r.PathValue("id"), r.PathValue("slug"))
		if e != nil {
			writeAPIError(w, 500, "learning_improvements_read_failed", "learning improvements could not be read")
			return
		}
		ps, e := pathways.Proposals(r.PathValue("id"), r.PathValue("slug"))
		if e != nil {
			writeAPIError(w, 500, "learning_improvements_read_failed", "learning improvements could not be read")
			return
		}
		current, _ := pathways.Current(r.PathValue("id"), r.PathValue("slug"))
		for i := range fs {
			fs[i].Stale = fs[i].PathwayVersion != current.Version
		}
		for i := range ps {
			ps[i].Stale = ps[i].BaseVersion != current.Version
			ps[i].RevalidationRequired = ps[i].Status == "accepted" && ps[i].MaterialRequirementChange
		}
		writeJSON(w, 200, map[string]any{"findings": fs, "update_proposals": ps, "current_pathway_version": current.Version, "history_preserved": true})
	})
	if workspaceStore == nil {
		return
	}
	mux.HandleFunc("POST /repositories/{id}/learning-pathways/{slug}/modules/{module_id}/attempts", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		var in struct {
			ExerciseID     string `json:"exercise_id"`
			PathwayVersion int    `json:"pathway_version"`
			RequestID      string `json:"request_id"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if strings.TrimSpace(in.RequestID) == "" || len(in.RequestID) > 200 {
			writeAPIError(w, 422, "learning_attempt_request_invalid", "a bounded caller-stable request_id is required")
			return
		}
		history, err := pathways.List(r.PathValue("id"), r.PathValue("slug"))
		if err != nil || in.PathwayVersion < 1 || in.PathwayVersion > len(history) {
			writeAPIError(w, 404, "learning_pathway_not_found", "exact learning pathway revision not found")
			return
		}
		pathway := history[in.PathwayVersion-1]
		var exercise *learningpathways.Exercise
		for mi := range pathway.Modules {
			if pathway.Modules[mi].ID == r.PathValue("module_id") {
				for ei := range pathway.Modules[mi].Exercises {
					if pathway.Modules[mi].Exercises[ei].ID == in.ExerciseID {
						exercise = &pathway.Modules[mi].Exercises[ei]
					}
				}
			}
		}
		if exercise == nil || exercise.Revision == "" {
			writeAPIError(w, 422, "learning_exercise_not_launchable", "module exercise must define an exact practice revision, kind, and acceptance criteria")
			return
		}
		gr, err := git.Open(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		if _, err = gr.ReadCommit(storage.ObjectID(exercise.Revision)); err != nil {
			writeAPIError(w, 422, "learning_revision_unavailable", "the exercise revision is unavailable")
			return
		}
		definitionBytes, err := exec.Command("git", "--git-dir="+gr.Path(), "show", exercise.Revision+":"+workspaces.DefinitionPath).Output()
		if err != nil {
			writeAPIError(w, 422, "workspace_definition_missing", "the exercise revision must contain .vivarium/workspace.json")
			return
		}
		definition, err := parseWorkspaceDefinition(definitionBytes)
		if err != nil {
			writeAPIError(w, 422, "workspace_definition_invalid", err.Error())
			return
		}
		policy, err := workspaceStore.GetPolicy("repository", r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "workspace_policy_unavailable", "workspace policy could not be read")
			return
		}
		repo, _ := repos.GetByID(r.PathValue("id"))
		scope := "repository"
		if repo.OrganizationID != "" {
			op, policyErr := workspaceStore.GetPolicy("organization", repo.OrganizationID)
			if policyErr != nil {
				writeAPIError(w, 500, "workspace_policy_unavailable", "workspace policy could not be read")
				return
			}
			policy = workspaces.Constrain(op, policy)
			scope = "organization+repository"
		}
		if definition.Resources.CPUs > policy.MaxCPUs || definition.Resources.MemoryMB > policy.MaxMemoryMB || definition.Resources.StorageMB > policy.MaxStorageMB {
			writeAPIError(w, 422, "workspace_policy_resources_exceeded", "exercise workspace exceeds the effective resource policy")
			return
		}
		dataPaths := []string{}
		for _, d := range exercise.Data {
			dataPaths = append(dataPaths, d.Path)
		}
		repro, _ := json.Marshal(struct {
			Revision       string
			Definition     string
			Instructions   string
			Criteria, Data []string
		}{exercise.Revision, hex.EncodeToString(learningDigest(definitionBytes)), exercise.Instructions, exercise.AcceptanceCriteria, dataPaths})
		ctx := &workspaces.LearningContext{PathwaySlug: pathway.Slug, PathwayVersion: pathway.Version, ModuleID: r.PathValue("module_id"), ExerciseID: exercise.ID, Kind: exercise.Kind, Instructions: exercise.Instructions, StarterCommands: exercise.StarterCommands, AcceptanceCriteria: exercise.AcceptanceCriteria, Data: dataPaths, Hints: exercise.Hints, ReproducibilitySHA256: hex.EncodeToString(learningDigest(repro)), Guidance: workspaces.LearningGuidance{Version: 1}}
		created, reused, err := workspaceStore.CreateLearning(workspaces.Workspace{RepositoryID: repo.ID, OrganizationID: repo.OrganizationID, CommitID: exercise.Revision, Definition: definition, Source: workspaces.Source{Kind: "learning_exercise", RepositoryID: repo.ID, LearningPathwaySlug: pathway.Slug, LearningPathwayVersion: pathway.Version, LearningModuleID: r.PathValue("module_id"), LearningExerciseID: exercise.ID, LearningRequestID: in.RequestID}, CreatorID: actor.UserID, Access: workspaces.Access{Role: "learner", Scopes: []string{"repositories:read"}}, Policy: policy, PolicyScope: scope, PolicyVersion: policy.Version, LearningContext: ctx}, definitionBytes)
		if errors.Is(err, workspaces.ErrRequestChanged) {
			writeAPIError(w, 409, "learning_attempt_request_changed", "request identity was already used with different launch inputs")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "learning_attempt_create_failed", "learning attempt could not be retained")
			return
		}
		created, _, err = workspaceStore.ReconcileLearningProvisioning(created.ID, func() ([]workspaces.SetupStep, bool) {
			if reused {
				_ = exec.Command("docker", "rm", "-f", "-v", "vivarium-workspace-"+created.ID).Run()
			}
			steps, failed := provisionWorkspace(gr.Path(), workspaceStore.RuntimePath(created.ID), created.ID, exercise.Revision, definition)
			if !failed {
				if stageErr := stageLearningData(gr.Path(), created.ID, exercise.Revision, exercise.Data); stageErr != nil {
					failed = true
					steps = append(steps, failedSetupStep("stage permitted learning data", nil, stageErr))
					_ = exec.Command("docker", "rm", "-f", "-v", "vivarium-workspace-"+created.ID).Run()
				}
			}
			return steps, failed
		})
		if err != nil {
			writeAPIError(w, 500, "learning_attempt_create_failed", "learning setup evidence could not be retained")
			return
		}
		w.Header().Set("Location", "/workspaces/"+created.ID)
		status := 201
		if reused {
			status = 200
		}
		writeJSON(w, status, created)
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/learning/hints/{index}", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, workspaceStore, repos, credentials, "repositories:read")
		if !ok {
			return
		}
		index, e := strconv.Atoi(r.PathValue("index"))
		if e != nil {
			writeAPIError(w, 422, "learning_hint_invalid", "hint index is invalid")
			return
		}
		updated, e := workspaceStore.UseLearningHint(item.ID, actor.UserID, index)
		if e != nil {
			writeAPIError(w, 422, "learning_hint_invalid", "hint is unavailable")
			return
		}
		writeJSON(w, 200, updated)
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/learning/checkpoints", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, workspaceStore, repos, credentials, "repositories:read")
		if !ok {
			return
		}
		var in struct {
			Summary           string   `json:"summary"`
			Criteria          []string `json:"criteria"`
			CommandOutcomeIDs []string `json:"command_outcome_ids"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		updated, e := workspaceStore.AddLearningCheckpoint(item.ID, actor.UserID, in.Summary, in.Criteria, in.CommandOutcomeIDs)
		if e != nil {
			writeAPIError(w, 422, "learning_checkpoint_invalid", "checkpoint must cite retained criteria and command outcomes")
			return
		}
		writeJSON(w, 201, updated)
	})
	loadHelp := func(w http.ResponseWriter, r *http.Request) (workspaces.Workspace, auth.Credential, learningpathways.Revision, bool, bool) {
		item, actor, ok := authorizeWorkspace(w, r, workspaceStore, repos, credentials, "repositories:read")
		if !ok || item.LearningContext == nil {
			if ok {
				writeAPIError(w, 404, "learning_attempt_not_found", "learning attempt not found")
			}
			return workspaces.Workspace{}, auth.Credential{}, learningpathways.Revision{}, false, false
		}
		history, err := pathways.List(item.RepositoryID, item.LearningContext.PathwaySlug)
		if err != nil || item.LearningContext.PathwayVersion < 1 || item.LearningContext.PathwayVersion > len(history) {
			writeAPIError(w, 409, "learning_context_stale", "the exact learning pathway is unavailable")
			return workspaces.Workspace{}, auth.Credential{}, learningpathways.Revision{}, false, false
		}
		pathway := history[item.LearningContext.PathwayVersion-1]
		mentor := false
		for _, m := range pathway.Mentors {
			if m.UserID == actor.UserID && isParticipant(item.RepositoryID, actor.UserID) {
				mentor = true
			}
		}
		if actor.UserID != item.CreatorID && !mentor && actor.AgentID == "" {
			writeAPIError(w, 403, "learning_guidance_forbidden", "only the learner, designated mentors, or the learner's active approved agent can use this timeline")
			return workspaces.Workspace{}, auth.Credential{}, learningpathways.Revision{}, false, false
		}
		if actor.AgentID != "" && !activeLearningGuide(item, actor.AgentID, time.Now()) {
			writeAPIError(w, 403, "learning_agent_access_inactive", "only the learner-selected active agent with live guide control can use this timeline")
			return workspaces.Workspace{}, auth.Credential{}, learningpathways.Revision{}, false, false
		}
		return item, actor, pathway, mentor, true
	}
	mux.HandleFunc("GET /workspaces/{workspace_id}/learning/guidance", func(w http.ResponseWriter, r *http.Request) {
		item, actor, _, mentor, ok := loadHelp(w, r)
		if !ok {
			return
		}
		publish := func() error { writeJSON(w, 200, item.LearningContext.Guidance); return nil }
		if actor.AgentID != "" {
			err := workspaceStore.PublishLearningGuidanceToAgent(item.ID, actor.AgentID, func(guidance workspaces.LearningGuidance) error { writeJSON(w, 200, guidance); return nil })
			if errors.Is(err, workspaces.ErrControl) {
				writeAPIError(w, 403, "learning_agent_access_inactive", "only the learner-selected active agent with live guide control can use this timeline")
			} else if err != nil {
				writeAPIError(w, 503, "learning_guidance_unavailable", "learning guidance could not be read")
			}
			return
		}
		if mentor {
			if err := repos.WithCurrentParticipant(actor.UserID, item.RepositoryID, publish); err != nil {
				writeAPIError(w, 403, "learning_mentor_access_revoked", "designated mentor access was revoked")
			}
			return
		}
		_ = publish()
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/learning/guidance", func(w http.ResponseWriter, r *http.Request) {
		item, actor, _, mentor, ok := loadHelp(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion   int                                   `json:"expected_version"`
			Kind              string                                `json:"kind"`
			Body              string                                `json:"body"`
			Citations         []workspaces.LearningEvidenceCitation `json:"citations"`
			CheckpointIDs     []string                              `json:"checkpoint_ids"`
			CommandOutcomeIDs []string                              `json:"command_outcome_ids"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.Kind, in.Body = strings.TrimSpace(in.Kind), strings.TrimSpace(in.Body)
		actorKind, agentID := "learner", ""
		if mentor {
			actorKind = "mentor"
		}
		if actor.AgentID != "" {
			actorKind, agentID = "agent", actor.AgentID
		}
		allowed := (actorKind == "learner" && in.Kind == "question") || (actorKind == "mentor" && slices.Contains([]string{"explanation", "hint", "demonstration", "direct_action"}, in.Kind)) || (actorKind == "agent" && in.Kind == "hint")
		if !allowed {
			writeAPIError(w, 403, "learning_guidance_kind_forbidden", "the selected help kind is not available to this role")
			return
		}
		if actorKind != "learner" && len(in.Citations) == 0 {
			writeAPIError(w, 422, "learning_guidance_citation_required", "guidance must cite exact project evidence")
			return
		}
		if actorKind != "learner" && (len(in.CheckpointIDs) > 0 || len(in.CommandOutcomeIDs) > 0) {
			writeAPIError(w, 422, "learning_state_learner_controlled", "only the learner can select exercise state to share")
			return
		}
		if reproductionSecretLike("guidance", stringToBase64([]byte(in.Body))) {
			writeAPIError(w, 422, "learning_guidance_sensitive", "guidance cannot contain credential-shaped material")
			return
		}
		gr, err := git.Open(item.RepositoryID)
		if err != nil {
			writeAPIError(w, 409, "learning_evidence_unavailable", "project evidence is unavailable")
			return
		}
		for _, c := range in.Citations {
			if c.Revision != item.CommitID || c.Path == "" {
				writeAPIError(w, 422, "learning_evidence_invalid", "citations must name a path at the exercise revision")
				return
			}
			if _, err := exec.Command("git", "--git-dir="+gr.Path(), "show", c.Revision+":"+c.Path).Output(); err != nil {
				writeAPIError(w, 422, "learning_evidence_invalid", "cited project evidence is unavailable")
				return
			}
		}
		if actorKind == "agent" {
			if !activeLearningGuide(item, agentID, time.Now()) {
				writeAPIError(w, 409, "learning_agent_paused", "agent guidance requires active learner approval and live guide control")
				return
			}
		}
		if in.Kind == "direct_action" && (item.Control.PrincipalKind != "human" || item.Control.PrincipalID != actor.UserID || !slices.Contains([]string{"edit", "execute"}, item.Control.Mode)) {
			writeAPIError(w, 409, "learning_mentor_control_required", "direct action requires explicit bounded mentor workspace control")
			return
		}
		var updated workspaces.Workspace
		mutation := func() (mutationErr error) {
			updated, mutationErr = workspaceStore.AddLearningGuidance(item.ID, actor.UserID, actorKind, agentID, in.Kind, in.Body, in.Citations, in.CheckpointIDs, in.CommandOutcomeIDs, in.ExpectedVersion)
			return mutationErr
		}
		if mentor {
			err = repos.WithCurrentParticipant(actor.UserID, item.RepositoryID, mutation)
		} else {
			err = mutation()
		}
		if errors.Is(err, workspaces.ErrConflict) {
			writeAPIError(w, 409, "learning_guidance_changed", "learning guidance changed since it was observed")
			return
		}
		if errors.Is(err, repositories.ErrInvalidCollaborator) || errors.Is(err, repositories.ErrNotFound) {
			writeAPIError(w, 403, "learning_mentor_access_revoked", "designated mentor access was revoked")
			return
		}
		if errors.Is(err, workspaces.ErrControl) {
			writeAPIError(w, 409, "learning_agent_paused", "agent guidance requires active learner approval and live guide control")
			return
		}
		if err != nil {
			writeAPIError(w, 422, "learning_guidance_invalid", "guidance could not be retained")
			return
		}
		writeJSON(w, 201, updated.LearningContext.Guidance)
	})
	mux.HandleFunc("PUT /workspaces/{workspace_id}/learning/agent", func(w http.ResponseWriter, r *http.Request) {
		item, actor, _, _, ok := loadHelp(w, r)
		if !ok {
			return
		}
		if actor.UserID != item.CreatorID || actor.AgentID != "" {
			writeAPIError(w, 403, "learning_learner_control_required", "only the learner can guide, pause, or revoke an agent")
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			AgentID         string `json:"agent_id"`
			State           string `json:"state"`
			Guidance        string `json:"guidance"`
		}
		if decodeJSON(r, &in) != nil || !slices.Contains([]string{"active", "paused", "revoked"}, in.State) {
			writeAPIError(w, 422, "learning_agent_control_invalid", "agent state must be active, paused, or revoked")
			return
		}
		if in.State == "active" && (strings.TrimSpace(in.Guidance) == "" || !workspaceApprovedAgent(orgs, repos, item.RepositoryID, in.AgentID)) {
			writeAPIError(w, 422, "learning_agent_not_approved", "agent must be approved for the repository organization")
			return
		}
		updated, err := workspaceStore.SetLearningAgent(item.ID, actor.UserID, in.AgentID, in.State, strings.TrimSpace(in.Guidance), in.ExpectedVersion)
		if errors.Is(err, workspaces.ErrConflict) {
			writeAPIError(w, 409, "learning_guidance_changed", "learning guidance changed since it was observed")
			return
		}
		if err != nil {
			writeAPIError(w, 422, "learning_agent_control_invalid", "agent control could not be retained")
			return
		}
		writeJSON(w, 200, updated.LearningContext.Guidance)
	})
}

func activeLearningGuide(item workspaces.Workspace, agentID string, now time.Time) bool {
	return item.LearningContext != nil && agentID != "" && item.LearningContext.Guidance.AgentID == agentID && item.LearningContext.Guidance.AgentState == "active" && item.Control.PrincipalKind == "approved_agent" && item.Control.PrincipalID == agentID && item.Control.Mode == "guide" && item.Control.ExpiresAt.After(now)
}

func learningDigest(body []byte) []byte { sum := sha256.Sum256(body); return sum[:] }
func stageLearningData(gitPath, workspaceID, revision string, data []learningpathways.ExerciseData) error {
	for _, d := range data {
		content := []byte(d.Content)
		if d.Kind == "permitted" {
			var err error
			content, err = exec.Command("git", "--git-dir="+gitPath, "show", revision+":"+d.Source).Output()
			if err != nil {
				return errors.New("permitted data source is unavailable")
			}
		}
		if reproductionSecretLike(d.Path, stringToBase64(content)) {
			return errors.New("learning data resembles credential material")
		}
		cmd := exec.Command("docker", "exec", "-i", "vivarium-workspace-"+workspaceID, "sh", "-lc", "umask 077; mkdir -p -- \"$(dirname -- \"$1\")\"; cat > \"$1\"", "sh", "/workspace/"+d.Path)
		cmd.Stdin = bytes.NewReader(content)
		if out, err := cmd.CombinedOutput(); err != nil {
			return errors.New("learning data could not be staged: " + string(out))
		}
	}
	return nil
}
func stringToBase64(body []byte) string { return base64.StdEncoding.EncodeToString(body) }
