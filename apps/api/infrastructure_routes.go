package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/infrastructure"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type infrastructureInput struct {
	ExpectedVersion int                     `json:"expected_version"`
	Revision        infrastructure.Revision `json:"revision"`
}

func registerInfrastructureRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, definitions *infrastructure.Store, pulls *pullrequests.Store, releaseStore *releases.Store, deploymentStore *deployments.Store) {
	mux.HandleFunc("GET /repositories/{id}/infrastructure", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, err := definitions.List(r.PathValue("id"), infrastructureParticipant(catalog, r.PathValue("id"), actor, authenticated))
		if err != nil {
			writeAPIError(w, 500, "infrastructure_unavailable", "infrastructure definitions could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"definitions": values})
	})
	mux.HandleFunc("GET /repositories/{id}/infrastructure/{definition_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		out, err := definitions.Get(r.PathValue("definition_id"), infrastructureParticipant(catalog, r.PathValue("id"), actor, authenticated))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_not_found", "infrastructure definition not found")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in infrastructureInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete infrastructure revision is required")
			return
		}
		var out infrastructure.Definition
		err := catalog.WithCurrentParticipants(infrastructureOwners(actor.UserID, in.Revision), r.PathValue("id"), func() error {
			if !infrastructureRevisionResolves(git, r.PathValue("id"), in.Revision, releaseStore, deploymentStore) {
				return infrastructure.ErrInvalid
			}
			var e error
			out, e = definitions.Create(r.PathValue("id"), actor.UserID, in.Revision)
			return e
		})
		writeInfrastructure(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure/{definition_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := definitions.Get(r.PathValue("definition_id"), true)
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_not_found", "infrastructure definition not found")
			return
		}
		var in infrastructureInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete infrastructure revision are required")
			return
		}
		var out infrastructure.Definition
		err = catalog.WithCurrentParticipants(infrastructureOwners(actor.UserID, in.Revision), current.RepositoryID, func() error {
			if !infrastructureRevisionResolves(git, current.RepositoryID, in.Revision, releaseStore, deploymentStore) {
				return infrastructure.ErrInvalid
			}
			var e error
			out, e = definitions.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
			return e
		})
		writeInfrastructure(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure/{definition_id}/observations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := definitions.Get(r.PathValue("definition_id"), true)
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_not_found", "infrastructure definition not found")
			return
		}
		var in infrastructure.Observation
		if decodeJSON(r, &in) != nil || !infrastructureLinksResolve(r.PathValue("id"), in.EnvironmentID, in.ReleaseID, releaseStore, deploymentStore) {
			writeAPIError(w, 400, "invalid_infrastructure_observation", "observation must be sanitized and bind exact definition, provider, environment, and release revisions")
			return
		}
		out, err := definitions.Observe(current.ID, actor.UserID, in)
		writeInfrastructure(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/infrastructure-plans", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if err != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		items, err := definitions.ListPlans(pull.RepositoryID, pull.ID)
		if err != nil {
			writeAPIError(w, 500, "infrastructure_plan_unavailable", "infrastructure plans could not be read")
			return
		}
		projected := make([]infrastructure.ChangePlan, 0, len(items))
		for _, p := range items {
			current, e := definitions.Get(p.DefinitionID, true)
			if e != nil {
				current = infrastructure.Definition{}
			}
			projected = append(projected, definitions.ProjectPlan(p, current, pull.SourceCommitID, func(x infrastructure.PolicyEffect) bool {
				return infrastructurePolicyCurrent(git, pull.SourceRepositoryID, p.SourceRevision, x)
			}))
		}
		writeJSON(w, 200, map[string]any{"plans": projected})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/infrastructure-plans", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			DefinitionID   string                        `json:"definition_id"`
			SourceRevision string                        `json:"source_revision"`
			Candidate      infrastructure.Revision       `json:"candidate"`
			PolicyEffects  []infrastructure.PolicyEffect `json:"policy_effects"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact infrastructure candidate and policy effects are required")
			return
		}
		current, err := definitions.Get(in.DefinitionID, true)
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_not_found", "infrastructure definition not found")
			return
		}
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if err != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		for _, p := range in.PolicyEffects {
			if !infrastructurePolicyCurrent(git, pull.SourceRepositoryID, in.SourceRevision, p) {
				writeAPIError(w, 422, "infrastructure_policy_invalid", "each policy effect must cite an exact candidate-revision file digest")
				return
			}
		}
		var out infrastructure.ChangePlan
		err = catalog.WithCurrentParticipants(infrastructureOwners(actor.UserID, in.Candidate), r.PathValue("id"), func() error {
			return pulls.WithSourceRevision(r.PathValue("id"), r.PathValue("pull_id"), in.SourceRevision, func(p pullrequests.PullRequest) error {
				var e error
				out, e = definitions.CreatePlan(p.RepositoryID, actor.UserID, infrastructure.PlanCreation{PullRequestID: p.ID, Revision: p.SourceCommitID, Definition: current, Candidate: in.Candidate, Policies: in.PolicyEffects})
				return e
			})
		})
		writeInfrastructurePlan(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/infrastructure-plans/{plan_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok || !authenticated {
			return
		}
		if actor.AgentID == "" {
			if !infrastructureParticipant(catalog, r.PathValue("id"), actor, true) {
				writeAPIError(w, 403, "infrastructure_plan_forbidden", "only current participants and repository-bound read-only agents may contribute")
				return
			}
		}
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if err != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		plan, err := definitions.GetPlan(r.PathValue("plan_id"))
		if err != nil || plan.RepositoryID != pull.RepositoryID || plan.PullRequestID != pull.ID {
			writeAPIError(w, 404, "infrastructure_plan_not_found", "infrastructure plan not found")
			return
		}
		current, _ := definitions.Get(plan.DefinitionID, true)
		projected := definitions.ProjectPlan(plan, current, pull.SourceCommitID, func(x infrastructure.PolicyEffect) bool {
			return infrastructurePolicyCurrent(git, pull.SourceRepositoryID, plan.SourceRevision, x)
		})
		if !projected.Fresh {
			writeAPIError(w, 409, "infrastructure_plan_stale", "source, provider, policy, or observed state changed; create a new plan")
			return
		}
		var in struct {
			ExpectedEvents int                      `json:"expected_events"`
			Event          infrastructure.PlanEvent `json:"event"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an event and expected event count are required")
			return
		}
		actorID, actorType := actor.UserID, "human"
		if actor.AgentID != "" {
			actorID, actorType = actor.AgentID, "agent"
		}
		out, err := definitions.AddPlanEvent(plan.ID, actorID, actorType, in.ExpectedEvents, in.Event)
		if err == nil {
			out = definitions.ProjectPlan(out, current, pull.SourceCommitID, func(x infrastructure.PolicyEffect) bool {
				return infrastructurePolicyCurrent(git, pull.SourceRepositoryID, plan.SourceRevision, x)
			})
		}
		writeInfrastructurePlan(w, out, err, 201)
	})
}

func infrastructurePolicyCurrent(git *storage.Store, repoID, revision string, policy infrastructure.PolicyEffect) bool {
	repo, err := git.Open(repoID)
	if err != nil {
		return false
	}
	commit, err := repo.ReadCommit(storage.ObjectID(revision))
	if err != nil {
		return false
	}
	entries, err := repo.WalkTree(commit.Tree)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Path != policy.Path || entry.Type != storage.BlobObject {
			continue
		}
		blob, truncated, binary, err := repo.ReadBlobPreview(entry.ID, 1<<20)
		if err != nil || truncated || binary {
			return false
		}
		sum := sha256.Sum256(blob.Content)
		return hex.EncodeToString(sum[:]) == policy.Digest
	}
	return false
}

func writeInfrastructurePlan(w http.ResponseWriter, v infrastructure.ChangePlan, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, infrastructure.ErrConflict):
		writeAPIError(w, 409, "infrastructure_plan_conflict", "the plan discussion changed; reload before contributing")
	case errors.Is(err, infrastructure.ErrInvalid):
		writeAPIError(w, 400, "invalid_infrastructure_plan", "the plan must compare exact resources, policy effects, risks, dependencies, affected owners, and rollback limits")
	case errors.Is(err, pullrequests.ErrSourceChanged), errors.Is(err, pullrequests.ErrNotReady):
		writeAPIError(w, 409, "infrastructure_plan_stale", "the pull source changed or is no longer open")
	default:
		log.Printf("infrastructure plan storage: %v", err)
		writeAPIError(w, 500, "infrastructure_plan_unavailable", "infrastructure plan could not be persisted")
	}
}

func infrastructureParticipant(catalog *repositories.Store, repo string, actor auth.Credential, authenticated bool) bool {
	if !authenticated {
		return false
	}
	if actor.AgentID != "" {
		return true
	}
	v, e := catalog.GetByID(repo)
	if e != nil {
		return false
	}
	if v.OwnerID == actor.UserID {
		return true
	}
	ok, e := catalog.HasCollaborator(actor.UserID, repo)
	return e == nil && ok
}
func infrastructureOwners(actor string, r infrastructure.Revision) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(x string) {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	add(actor)
	for _, x := range r.OwnerIDs {
		add(x)
	}
	for _, resource := range r.Resources {
		for _, x := range resource.OwnerIDs {
			add(x)
		}
	}
	return out
}
func infrastructureRevisionResolves(git *storage.Store, repo string, r infrastructure.Revision, releases *releases.Store, deployments *deployments.Store) bool {
	if git == nil {
		return false
	}
	gr, e := git.Open(repo)
	if e != nil {
		return false
	}
	if _, e = gr.ReadCommit(storage.ObjectID(r.Revision)); e != nil {
		return false
	}
	for _, x := range r.Resources {
		if !infrastructureLinksResolve(repo, x.EnvironmentID, x.ReleaseID, releases, deployments) {
			return false
		}
	}
	return true
}
func infrastructureLinksResolve(repo, environmentID, releaseID string, releases *releases.Store, deployments *deployments.Store) bool {
	if environmentID != "" {
		if deployments == nil {
			return false
		}
		if _, e := deployments.GetEnvironment(repo, environmentID); e != nil {
			return false
		}
	}
	if releaseID != "" {
		if releases == nil {
			return false
		}
		if _, e := releases.Get(repo, releaseID); e != nil {
			return false
		}
	}
	return true
}
func writeInfrastructure(w http.ResponseWriter, v infrastructure.Definition, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, infrastructure.ErrConflict):
		writeAPIError(w, 409, "infrastructure_conflict", "the definition changed; reload before publishing another revision")
	case errors.Is(err, infrastructure.ErrInvalid):
		writeAPIError(w, 400, "invalid_infrastructure", "publish an exact commit with complete resources, owners, providers, boundaries, constraints, commitments, and sanitized observations")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "infrastructure_forbidden", "every declared owner must be a current repository participant")
	default:
		log.Printf("infrastructure storage: %v", err)
		writeAPIError(w, 500, "infrastructure_unavailable", "infrastructure definition could not be persisted")
	}
}
