package main

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/adoptionworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/federation"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incubators"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	packageversions "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/roadmaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

type adoptionCreateInput struct {
	adoptionworkspaces.Workspace
	Invitations []adoptionworkspaces.Invitation `json:"invitations"`
}
type adoptionConsentInput struct {
	Decision        string `json:"decision"`
	ExpectedVersion int    `json:"expected_version"`
}
type adoptionTrialInput struct {
	adoptionworkspaces.TrialDefinition
	ExpectedVersion int `json:"expected_version"`
}
type adoptionTrialAttemptInput struct {
	adoptionworkspaces.TrialAttempt
	ExpectedVersion int `json:"expected_version"`
}
type adoptionPlanInput struct {
	adoptionworkspaces.AdoptionPlan
	ExpectedVersion int `json:"expected_version"`
}
type adoptionDeliveryInput struct {
	adoptionworkspaces.AdoptionDelivery
	ExpectedVersion int `json:"expected_version"`
}
type adoptionFindingInput struct {
	adoptionworkspaces.SharedFinding
	ExpectedVersion int `json:"expected_version"`
}
type adoptionFindingConsentInput struct {
	Decision        string `json:"decision"`
	ExpectedVersion int    `json:"expected_version"`
}
type adoptionContributionInput struct {
	adoptionworkspaces.UpstreamContribution
	ExpectedVersion int `json:"expected_version"`
}
type adoptionUpdateInput struct {
	adoptionworkspaces.VerifiedUpdate
	ExpectedVersion int `json:"expected_version"`
}

func storeCanReadRepository(catalog *repositories.Store, viewer adoptionworkspaces.Viewer, id string) bool {
	repository, err := catalog.GetByID(id)
	if err != nil {
		return false
	}
	if repository.Visibility == repositories.Public {
		return true
	}
	if viewer.PrincipalType == "agent" {
		return viewer.RepositoryID == id
	}
	if repository.OwnerID == viewer.PrincipalID {
		return true
	}
	ok, _ := catalog.HasCollaborator(viewer.PrincipalID, id)
	return ok
}

func exactAdoptionPackage(versions []packageversions.Version, inventory packageversions.Inventory, providerRepositoryID, providerReleaseID, providerRevision string) (string, string, bool) {
	name, version := "", ""
	for _, candidate := range versions {
		if candidate.RepositoryID != providerRepositoryID || candidate.ReleaseID != providerReleaseID || candidate.SourceCommit != providerRevision {
			continue
		}
		if name != "" && (name != candidate.Name || version != candidate.Version) {
			return "", "", false
		}
		name, version = candidate.Name, candidate.Version
	}
	if name == "" {
		return "", "", false
	}
	for _, entry := range inventory.Entries {
		if entry.Direct && entry.Name == name && entry.Version == version && entry.State == "resolved" {
			return name, version, true
		}
	}
	return "", "", false
}

func adoptionPatchCoverage(local, update []pullrequests.FileChange) ([]string, bool) {
	if len(local) == 0 {
		return nil, false
	}
	updated := map[string]bool{}
	for _, change := range update {
		updated[change.Path] = true
	}
	paths := make([]string, 0, len(local))
	for _, change := range local {
		if !updated[change.Path] {
			return nil, false
		}
		paths = append(paths, change.Path)
	}
	return paths, true
}

func registerAdoptionWorkspaceRoutes(mux *http.ServeMux, credentials *auth.Store, identities *users.Store, catalog *repositories.Store, orgs *organizations.Store, incubatorStore *incubators.Store, federationStore *federation.Store, roadmapStore *roadmaps.Store, supportStore *supportthreads.Store, decisionStore *decisions.Store, packageStore *packageversions.Store, apiStore *apicontracts.Store, releaseStore *releases.Store, buildStore *checkruns.Store, pullStore *pullrequests.Store, issueStore *issues.Store, deploymentStore *deployments.Store, store *adoptionworkspaces.Store) {
	authn := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool) {
		a, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return a, false
		}
		if a.UserID == "" && a.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return a, false
		}
		return a, true
	}
	canReadRepository := func(actor auth.Credential, id string) bool {
		repo, e := catalog.GetByID(id)
		if e != nil {
			return false
		}
		if repo.Visibility == repositories.Public || repo.OwnerID == actor.UserID {
			return true
		}
		if actor.AgentID != "" {
			return actor.RepositoryID == id
		}
		ok, _ := catalog.HasCollaborator(actor.UserID, id)
		return ok
	}
	viewer := func(actor auth.Credential) adoptionworkspaces.Viewer {
		if actor.AgentID != "" {
			return adoptionworkspaces.Viewer{PrincipalType: "agent", PrincipalID: actor.AgentID, OrganizationID: actor.OrganizationID, RepositoryID: actor.RepositoryID}
		}
		return adoptionworkspaces.Viewer{PrincipalType: "human", PrincipalID: actor.UserID}
	}
	store.ConfigureRepositoryAccess(func(v adoptionworkspaces.Viewer, id string) bool {
		repo, e := catalog.GetByID(id)
		if e != nil {
			return false
		}
		if repo.Visibility == repositories.Public {
			return true
		}
		if v.PrincipalType == "agent" {
			return v.RepositoryID == id
		}
		if repo.OwnerID == v.PrincipalID {
			return true
		}
		ok, _ := catalog.HasCollaborator(v.PrincipalID, id)
		return ok
	})
	store.ConfigurePlanTargetProjection(func(v adoptionworkspaces.Viewer, work adoptionworkspaces.AdoptionWork) adoptionworkspaces.AdoptionWork {
		repository, err := catalog.GetByID(work.RepositoryID)
		if err != nil {
			work.EffectiveAccess, work.OwnerStatus, work.Authority = "stale_target", "stale_target", "no_authority_granted"
			return work
		}
		if v.PrincipalType == "agent" {
			if v.RepositoryID == repository.ID {
				work.EffectiveAccess = "collaborator"
			} else if repository.Visibility == repositories.Public {
				work.EffectiveAccess = "read_only"
			} else {
				work.EffectiveAccess = "inaccessible"
			}
		} else if repository.OwnerID == v.PrincipalID {
			work.EffectiveAccess = "owner"
		} else if collaborator, _ := catalog.HasCollaborator(v.PrincipalID, repository.ID); collaborator {
			work.EffectiveAccess = "collaborator"
		} else if repository.Visibility == repositories.Public {
			work.EffectiveAccess = "read_only"
		} else {
			work.EffectiveAccess = "inaccessible"
		}
		work.OwnerStatus = "stale"
		if work.OwnerType == "human" && repository.HasParticipant(work.OwnerID) {
			work.OwnerStatus = "current"
		}
		if work.OwnerType == "agent" {
			if organization, organizationErr := orgs.Get(repository.OrganizationID); organizationErr == nil {
				for _, agent := range organization.Agents {
					if agent.ID == work.OwnerID {
						work.OwnerStatus = "current"
					}
				}
			}
		}
		if work.EffectiveAccess == "inaccessible" {
			work.RepositoryID, work.EnvironmentID, work.OwnerID = "restricted", "", "restricted"
			work.Paths = nil
		}
		work.Authority = "no_authority_granted"
		return work
	})
	store.ConfigureDeliveryProjection(func(v adoptionworkspaces.Viewer, delivery adoptionworkspaces.AdoptionDelivery) adoptionworkspaces.AdoptionDelivery {
		if !storeCanReadRepository(catalog, v, delivery.ConsumerRepositoryID) {
			delivery.ConsumerRepositoryID, delivery.PullRequestID, delivery.PullRevision, delivery.MergeRevision, delivery.ReleaseID, delivery.ReleaseRevision, delivery.DeploymentID, delivery.EnvironmentID = "restricted", "", "", "", "", "", "", ""
			delivery.CheckRunIDs, delivery.ApprovalIDs, delivery.Rollout, delivery.Health = nil, nil, nil, nil
			delivery.State = "access_revoked"
		}
		if !storeCanReadRepository(catalog, v, delivery.ProviderRepositoryID) {
			delivery.ProviderRepositoryID, delivery.ProviderRevision = "restricted", ""
			delivery.State = "access_revoked"
		}
		delivery.Authority = "no_authority_granted"
		return delivery
	})
	store.ConfigureEnvironmentResolver(func(repositoryID, environmentID string) bool {
		if deploymentStore == nil {
			return false
		}
		_, err := deploymentStore.GetEnvironment(repositoryID, environmentID)
		return err == nil
	})
	resolveSource := func(source adoptionworkspaces.Source, actor auth.Credential) adoptionworkspaces.Source {
		source.Resolution, source.Detail = "inaccessible", "Starting context is outside this collaborator's current read boundary"
		if source.Kind == "federated_repository" {
			if federationStore == nil {
				return source
			}
			cache, e := federationStore.RepositoryCache(source.ResourceID)
			if e != nil {
				source.Resolution, source.Detail = "missing", "Federated repository has not been resolved"
			} else if cache.Status != "current" || cache.Snapshot == nil || !cache.SignatureVerified {
				source.Resolution, source.Detail = "stale", "Federated repository evidence is unavailable or stale"
			} else {
				source.Resolution, source.Detail = "resolved", "Current signed federated repository snapshot"
			}
			return source
		}
		if !canReadRepository(actor, source.RepositoryID) {
			return source
		}
		exists := strings.TrimSpace(source.ResourceID) != ""
		switch source.Kind {
		case "roadmap_outcome":
			exists = false
			if roadmapStore != nil {
				if x, e := roadmapStore.Get(source.RepositoryID); e == nil {
					for _, r := range x.Revisions {
						for _, item := range r.Items {
							exists = exists || item.ID == source.ResourceID || item.OpportunityID == source.ResourceID
						}
					}
				}
			}
		case "support_gap":
			if supportStore != nil {
				x, e := supportStore.Get(source.RepositoryID, source.ResourceID)
				exists = e == nil && x.RepositoryID == source.RepositoryID
			}
		case "incubator":
			exists = false
			if incubatorStore != nil {
				_, e := incubatorStore.Get(source.ResourceID, actor.UserID)
				exists = e == nil
			}
		case "decision":
			if decisionStore != nil {
				x, e := decisionStore.Get(source.ResourceID)
				exists = e == nil && x.RepositoryID == source.RepositoryID
			}
		case "package":
			parts := strings.SplitN(source.ResourceID, "@", 2)
			exists = false
			if packageStore != nil && len(parts) == 2 {
				x, e := packageStore.Get(parts[0], parts[1])
				exists = e == nil && x.RepositoryID == source.RepositoryID
			}
		case "api":
			if apiStore != nil {
				x, e := apiStore.Get(source.ResourceID)
				exists = e == nil && x.RepositoryID == source.RepositoryID
			}
		}
		if exists {
			source.Resolution, source.Detail = "resolved", "Starting context resolved inside the creator's current read boundary"
		} else {
			source.Resolution, source.Detail = "missing", "Starting context does not resolve"
		}
		return source
	}
	projectEvidence := func(in *adoptionCreateInput, actor auth.Credential) {
		for i := range in.Candidates {
			c := &in.Candidates[i]
			for j := range c.Evidence {
				e := &c.Evidence[j]
				e.Resolution = "missing"
				e.Detail = "Evidence reference does not resolve"
				if e.RepositoryID != "" {
					if canReadRepository(actor, e.RepositoryID) {
						e.Resolution, e.Detail = "resolved", "Repository evidence admitted within current read access"
					} else {
						e.Resolution, e.Detail = "inaccessible", "Repository evidence is outside the creator's read boundary"
						e.RepositoryID = ""
						e.Reference = "Restricted evidence"
						e.Summary = "Restricted evidence"
					}
					continue
				}
				u, err := url.ParseRequestURI(e.Reference)
				if err == nil && u.Scheme == "https" && u.Host != "" {
					e.Resolution, e.Detail = "resolved", "Public HTTPS evidence"
				}
			}
		}
	}
	mux.HandleFunc("POST /adoption-workspaces", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_creator_required", "a human collaborator must open an adoption workspace")
			return
		}
		var in adoptionCreateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "complete adoption requirements, candidates, and invitations are required")
			return
		}
		for _, v := range in.Invitations {
			if v.PrincipalType == "human" {
				if _, e := identities.Get(v.PrincipalID); e != nil {
					writeAPIError(w, 422, "invalid_invitee", "human invitees must exist")
					return
				}
			} else if v.PrincipalType == "agent" {
				org, e := orgs.Get(v.OrganizationID)
				approved := false
				if e == nil {
					for _, a := range org.Agents {
						approved = approved || a.ID == v.PrincipalID
					}
				}
				if !approved {
					writeAPIError(w, 422, "unapproved_agent", "agents must be approved by the selected organization")
					return
				}
			}
		}
		in.Source = resolveSource(in.Source, actor)
		projectEvidence(&in, actor)
		out, e := store.Create(in.Workspace, actor.UserID, in.Invitations)
		writeAdoptionWorkspace(w, out, e, 201)
	})
	mux.HandleFunc("GET /adoption-workspaces", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		out, e := store.List(viewer(actor))
		if e != nil {
			writeAdoptionWorkspace(w, adoptionworkspaces.Workspace{}, e, 500)
			return
		}
		writeJSON(w, 200, map[string]any{"adoption_workspaces": out})
	})
	mux.HandleFunc("GET /adoption-workspaces/invitations/pending", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		out, e := store.Pending(viewer(actor))
		if e != nil {
			writeAdoptionWorkspace(w, adoptionworkspaces.Workspace{}, e, 500)
			return
		}
		writeJSON(w, 200, map[string]any{"invitations": out})
	})
	mux.HandleFunc("GET /adoption-workspaces/{workspace_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		out, e := store.Get(r.PathValue("workspace_id"), viewer(actor))
		writeAdoptionWorkspace(w, out, e, 200)
	})
	mux.HandleFunc("POST /adoption-workspaces/{workspace_id}/invitations/{invitation_id}/consent", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_consent_required", "agents are admitted only under their existing organization approval")
			return
		}
		var in adoptionConsentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a decision and expected version are required")
			return
		}
		out, e := store.Consent(r.PathValue("workspace_id"), r.PathValue("invitation_id"), actor.UserID, in.Decision, in.ExpectedVersion)
		writeAdoptionWorkspace(w, out, e, 200)
	})
	mux.HandleFunc("POST /adoption-workspaces/{workspace_id}/trials", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in adoptionTrialInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete bounded trial and expected version are required")
			return
		}
		in.Source.Resolution = "inaccessible"
		if canReadRepository(actor, in.Source.RepositoryID) {
			switch in.Source.Kind {
			case "attested_release":
				if releaseStore != nil && buildStore != nil {
					if rel, e := releaseStore.Get(in.Source.RepositoryID, in.Source.ResourceID); e == nil && rel.CommitID == in.Source.Revision {
						runs, runErr := buildStore.List(rel.RepositoryID, rel.ID)
						verified := runErr == nil && len(runs) > 0
						for _, run := range runs {
							verified = verified && run.State == "succeeded"
						}
						if verified {
							in.Source.Resolution = "resolved"
							in.Source.Attestation = "verified repository release " + rel.ID
						}
					}
				}
			case "exact_revision":
				// Exact revisions remain repository-scoped and must be immutable SHA-1 identities.
				if len(in.Source.Revision) == 40 && in.Source.ResourceID == in.Source.Revision && catalog.HasCommit(in.Source.RepositoryID, in.Source.Revision) {
					in.Source.Resolution = "resolved"
				}
			}
		}
		out, e := store.CreateTrial(r.PathValue("workspace_id"), in.TrialDefinition, viewer(actor), in.ExpectedVersion)
		writeAdoptionWorkspace(w, out, e, 201)
	})
	mux.HandleFunc("POST /adoption-workspaces/{workspace_id}/trials/{trial_id}/attempts", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in adoptionTrialAttemptInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "trial evidence and expected version are required")
			return
		}
		out, e := store.RecordTrialAttempt(r.PathValue("workspace_id"), r.PathValue("trial_id"), in.TrialAttempt, viewer(actor), in.ExpectedVersion)
		writeAdoptionWorkspace(w, out, e, 201)
	})
	mux.HandleFunc("POST /adoption-workspaces/{workspace_id}/plans", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_adoption_agreement_required", "a consented human adopter or provider participant must record the agreement")
			return
		}
		var in adoptionPlanInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete adoption agreement and expected version are required")
			return
		}
		targetIDs := make([]string, len(in.Work))
		for i := range in.Work {
			targetIDs[i] = in.Work[i].RepositoryID
		}
		var created adoptionworkspaces.Workspace
		e := catalog.WithCurrentRepositories(targetIDs, func(targets []repositories.Repository) error {
			for i := range in.Work {
				work, repository := &in.Work[i], targets[i]
				if work.Kind == "upstream_fork" && repository.UpstreamRepositoryID == "" {
					return adoptionworkspaces.ErrInvalid
				}
				if work.Kind == "environment" {
					if deploymentStore == nil {
						return adoptionworkspaces.ErrInvalid
					}
					if _, environmentErr := deploymentStore.GetEnvironment(repository.ID, work.EnvironmentID); environmentErr != nil {
						return adoptionworkspaces.ErrInvalid
					}
				}
				if work.OwnerType == "human" {
					if _, ownerErr := identities.Get(work.OwnerID); ownerErr != nil {
						return adoptionworkspaces.ErrInvalid
					}
					if !repository.HasParticipant(work.OwnerID) {
						return adoptionworkspaces.ErrInvalid
					}
				} else {
					organization, organizationErr := orgs.Get(repository.OrganizationID)
					approved := false
					if organizationErr == nil {
						for _, agent := range organization.Agents {
							approved = approved || agent.ID == work.OwnerID
						}
					}
					if !approved {
						return adoptionworkspaces.ErrInvalid
					}
				}
				switch {
				case repository.OwnerID == actor.UserID:
					work.EffectiveAccess = "owner"
				case repository.HasParticipant(actor.UserID):
					work.EffectiveAccess = "collaborator"
				case repository.Visibility == repositories.Public:
					work.EffectiveAccess = "read_only"
				default:
					work.EffectiveAccess = "inaccessible"
				}
				work.OwnerStatus = "current"
			}
			var createErr error
			created, createErr = store.CreatePlan(r.PathValue("workspace_id"), in.AdoptionPlan, viewer(actor), in.ExpectedVersion)
			return createErr
		})
		if errors.Is(e, repositories.ErrNotFound) || errors.Is(e, repositories.ErrInvalidCollaborator) {
			e = adoptionworkspaces.ErrInvalid
		}
		if e == nil {
			created, e = store.Get(created.ID, viewer(actor))
		}
		writeAdoptionWorkspace(w, created, e, 201)
	})
	mux.HandleFunc("POST /adoption-workspaces/{workspace_id}/deliveries", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_delivery_attestation_required", "a human consumer participant must retain adoption delivery evidence")
			return
		}
		var in adoptionDeliveryInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "exact ordinary delivery records, attestations, and expected version are required")
			return
		}
		consumer, err := catalog.GetByID(in.ConsumerRepositoryID)
		if err != nil || (actor.RepositoryID != "" && actor.RepositoryID != in.ConsumerRepositoryID) || !consumer.HasParticipant(actor.UserID) || pullStore == nil || releaseStore == nil || buildStore == nil || deploymentStore == nil {
			writeAPIError(w, 403, "consumer_delivery_forbidden", "current consumer repository participation and available ordinary delivery records are required")
			return
		}
		workspace, err := store.Get(r.PathValue("workspace_id"), viewer(actor))
		if err != nil {
			writeAdoptionWorkspace(w, adoptionworkspaces.Workspace{}, err, 200)
			return
		}
		providerRevision, providerRepository := "", ""
		for _, plan := range workspace.Plans {
			if plan.ID != in.PlanID {
				continue
			}
			for _, trial := range workspace.Trials {
				if trial.ID == plan.TrialID {
					providerRevision, providerRepository = trial.Source.Revision, trial.Source.RepositoryID
				}
			}
		}
		pull, pullErr := pullStore.Get(in.ConsumerRepositoryID, in.PullRequestID)
		release, releaseErr := releaseStore.Get(in.ConsumerRepositoryID, in.ReleaseID)
		promotion, promotionErr := deploymentStore.GetPromotion(in.ConsumerRepositoryID, in.DeploymentID)
		if pullErr != nil || releaseErr != nil || promotionErr != nil || pull.Status != "merged" || pull.MergeCommitID == nil || promotion.ReleaseID != release.ID || promotion.CommitID != release.CommitID || promotion.EnvironmentID == "" || providerRevision == "" || providerRepository == "" {
			writeAPIError(w, 422, "invalid_adoption_delivery", "pull, release, deployment, and provider revisions must resolve through ordinary platform records")
			return
		}
		included := false
		for _, evidence := range release.Inclusions.PullEvidence {
			included = included || evidence.PullRequestID == pull.ID && evidence.SourceCommitID == pull.SourceCommitID
		}
		reviews, reviewErr := pullStore.ListReviews(in.ConsumerRepositoryID, pull.ID)
		approvalIDs := []string{}
		if reviewErr == nil {
			for _, review := range reviews {
				if review.Decision == pullrequests.Approved && !review.Stale && review.ReviewedCommitID == pull.SourceCommitID {
					approvalIDs = append(approvalIDs, review.ID)
				}
			}
		}
		runs, runErr := buildStore.List(in.ConsumerRepositoryID, pull.ID)
		checkIDs, checksPassed := []string{}, runErr == nil
		for _, run := range runs {
			if run.CommitID != pull.SourceCommitID {
				continue
			}
			checkIDs = append(checkIDs, run.ID)
			checksPassed = checksPassed && run.State == "succeeded"
		}
		checksPassed = checksPassed && len(checkIDs) > 0
		for _, approval := range promotion.Approvals {
			approvalIDs = append(approvalIDs, approval.ActorID)
		}
		if !included || len(approvalIDs) == len(promotion.Approvals) || !checksPassed {
			writeAPIError(w, 422, "adoption_delivery_not_ready", "the exact pull must be merged, approved, checked, included in the release, and governed by deployment approvals")
			return
		}
		for _, attestation := range in.Attestations {
			if attestation.AttestedBy != actor.UserID {
				writeAPIError(w, 422, "invalid_attestation_owner", "delivery attestations must be authored by the authenticated human")
				return
			}
		}
		in.PullRevision, in.MergeRevision, in.ReleaseRevision = pull.SourceCommitID, *pull.MergeCommitID, release.CommitID
		in.ProviderRevision, in.ProviderRepositoryID, in.EnvironmentID = providerRevision, providerRepository, promotion.EnvironmentID
		in.CheckRunIDs, in.ApprovalIDs, in.Authority = checkIDs, approvalIDs, "no_authority_granted"
		requestedRestores := in.RestoresDeliveryID
		in.RestoresDeliveryID, in.RecoveryOfDeploymentID = "", ""
		in.Rollout, in.Health = []string{}, []string{}
		for _, stage := range promotion.Rollout.Stages {
			in.Rollout = append(in.Rollout, "staged rollout: "+stage.Name)
		}
		for _, event := range promotion.Events {
			in.Rollout = append(in.Rollout, event.Kind+": "+event.State)
		}
		for _, evidence := range promotion.Evidence {
			in.Health = append(in.Health, evidence.Stage+" / "+evidence.Signal+": "+evidence.State)
		}
		if len(in.Rollout) == 0 {
			in.Rollout = []string{"deployment state: " + promotion.State}
		}
		if len(in.Health) == 0 {
			in.Health = []string{"deployment health: " + promotion.State}
		}
		in.PauseReasons = nil
		unmet := []string{}
		for _, attestation := range in.Attestations {
			if !attestation.Satisfied {
				unmet = append(unmet, "unmet "+attestation.Kind+" attestation")
			}
		}
		switch promotion.State {
		case "succeeded":
			if len(unmet) > 0 {
				in.State, in.PauseReasons = "paused", unmet
			} else if promotion.RecoveryOf != "" {
				in.State = "restored"
				in.RestoresDeliveryID, in.RecoveryOfDeploymentID = requestedRestores, promotion.RecoveryOf
			} else {
				in.State = "operating"
			}
		case "failed", "paused", "canceled":
			in.State = "paused"
			in.PauseReasons = []string{"deployment " + promotion.State}
		default:
			writeAPIError(w, 422, "adoption_rollout_incomplete", "the staged deployment must finish or enter a retained safe pause")
			return
		}
		out, createErr := store.CreateDelivery(r.PathValue("workspace_id"), in.AdoptionDelivery, viewer(actor), in.ExpectedVersion)
		writeAdoptionWorkspace(w, out, createErr, 201)
	})
	mux.HandleFunc("POST /adoption-workspaces/{workspace_id}/shared-findings", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in adoptionFindingInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_shared_finding", "redacted finding evidence and expected version are required")
			return
		}
		out, err := store.ShareFinding(r.PathValue("workspace_id"), in.SharedFinding, viewer(actor), in.ExpectedVersion)
		writeAdoptionWorkspace(w, out, err, 201)
	})
	mux.HandleFunc("POST /adoption-workspaces/{workspace_id}/shared-findings/{finding_id}/consent", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_provider_consent_required", "a consented human provider maintainer must decide disclosure")
			return
		}
		var in adoptionFindingConsentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_finding_consent", "decision and expected version are required")
			return
		}
		out, err := store.ConsentFinding(r.PathValue("workspace_id"), r.PathValue("finding_id"), actor.UserID, in.Decision, in.ExpectedVersion)
		writeAdoptionWorkspace(w, out, err, 200)
	})
	mux.HandleFunc("POST /adoption-workspaces/{workspace_id}/upstream-contributions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in adoptionContributionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_upstream_contribution", "an ordinary issue or pull and expected version are required")
			return
		}
		principal := actor.UserID
		if actor.AgentID != "" {
			principal = actor.AgentID
		}
		workspace, err := store.Get(r.PathValue("workspace_id"), viewer(actor))
		if err != nil {
			writeAdoptionWorkspace(w, adoptionworkspaces.Workspace{}, err, 200)
			return
		}
		var finding *adoptionworkspaces.SharedFinding
		for i := range workspace.SharedFindings {
			if workspace.SharedFindings[i].ID == in.FindingID {
				finding = &workspace.SharedFindings[i]
			}
		}
		if finding == nil {
			writeAPIError(w, 422, "invalid_upstream_contribution", "the shared finding must resolve in this workspace")
			return
		}
		providerRepositoryID := ""
		for _, trial := range workspace.Trials {
			if trial.ID == finding.TrialID {
				providerRepositoryID = trial.Source.RepositoryID
			}
		}
		for _, delivery := range workspace.Deliveries {
			if delivery.ID == finding.DeliveryID {
				providerRepositoryID = delivery.ProviderRepositoryID
			}
		}
		localTarget := false
		for _, delivery := range workspace.Deliveries {
			localTarget = localTarget || delivery.ConsumerRepositoryID == in.TargetRepositoryID
		}
		for _, plan := range workspace.Plans {
			for _, work := range plan.Work {
				localTarget = localTarget || work.RepositoryID == in.TargetRepositoryID && work.Kind != "upstream_fork"
			}
		}
		if !canReadRepository(actor, in.TargetRepositoryID) || in.Kind == "local_pull" && !localTarget || in.Kind != "local_pull" && in.TargetRepositoryID != providerRepositoryID {
			writeAPIError(w, 422, "invalid_contribution_target", "the contribution target must resolve to this finding's provider or planned consumer boundary")
			return
		}
		if in.Kind == "issue" {
			if issueStore == nil {
				writeAPIError(w, 503, "issues_unavailable", "ordinary issues could not be resolved")
				return
			}
			issue, issueErr := issueStore.Get(in.TargetRepositoryID, in.ResourceID)
			if issueErr != nil || issue.ReporterID != principal {
				writeAPIError(w, 422, "invalid_upstream_issue", "the ordinary issue must exist and retain authenticated authorship")
				return
			}
			in.Status, in.Revision, in.SourceRepositoryID = "open", "", ""
			if issue.Status != "open" {
				in.Status = "closed"
			}
		} else {
			if pullStore == nil {
				writeAPIError(w, 503, "pulls_unavailable", "ordinary pulls could not be resolved")
				return
			}
			pull, pullErr := pullStore.Get(in.TargetRepositoryID, in.ResourceID)
			if pullErr != nil || pull.AuthorID != actor.UserID {
				writeAPIError(w, 422, "invalid_upstream_pull", "the ordinary pull must exist and retain authenticated authorship")
				return
			}
			source, sourceErr := catalog.GetByID(pull.SourceRepositoryID)
			if sourceErr != nil || in.Kind == "fork_pull" && source.UpstreamRepositoryID != pull.RepositoryID || in.Kind == "local_pull" && pull.SourceRepositoryID != pull.RepositoryID || in.Kind == "federated_pull" && pull.FederatedContributionID == "" {
				writeAPIError(w, 422, "invalid_upstream_pull", "pull topology does not match its declared local, fork, or federated kind")
				return
			}
			in.SourceRepositoryID, in.Revision, in.Status = pull.SourceRepositoryID, pull.SourceCommitID, pull.Status
			if in.Kind == "local_pull" {
				in.Status = "local_only"
			}
		}
		in.AuthoredBy, in.AuthoredByType, in.Authority = "", "", ""
		out, createErr := store.RecordContribution(r.PathValue("workspace_id"), in.UpstreamContribution, viewer(actor), in.ExpectedVersion)
		writeAdoptionWorkspace(w, out, createErr, 201)
	})
	mux.HandleFunc("POST /adoption-workspaces/{workspace_id}/verified-updates", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_update_verification_required", "a human adopter must verify replacement of a consumer patch")
			return
		}
		var in adoptionUpdateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_verified_update", "exact provider and consumer delivery records are required")
			return
		}
		if releaseStore == nil || pullStore == nil || buildStore == nil || deploymentStore == nil || packageStore == nil {
			writeAPIError(w, 503, "adoption_update_evidence_unavailable", "ordinary release, pull, check, and deployment records are required")
			return
		}
		providerRelease, pe := releaseStore.Get(in.ProviderRepositoryID, in.ProviderReleaseID)
		consumerPull, ce := pullStore.Get(in.ConsumerRepositoryID, in.ConsumerPullRequestID)
		consumerRelease, re := releaseStore.Get(in.ConsumerRepositoryID, in.ConsumerReleaseID)
		promotion, de := deploymentStore.GetPromotion(in.ConsumerRepositoryID, in.ConsumerDeploymentID)
		providerIncludes, consumerIncludes := false, false
		workspace, we := store.Get(r.PathValue("workspace_id"), viewer(actor))
		contributionResource, contributionRevision, contributionFindingID := "", "", ""
		if we == nil {
			for _, c := range workspace.Contributions {
				if c.ID == in.ContributionID && c.Status == "merged" && c.TargetRepositoryID == in.ProviderRepositoryID {
					contributionResource, contributionRevision, contributionFindingID = c.ResourceID, c.Revision, c.FindingID
				}
			}
		}
		findingProvenance := func(findingID string) string {
			for _, finding := range workspace.SharedFindings {
				if finding.ID == findingID {
					return finding.TrialID + ":" + finding.AttemptID + ":" + finding.DeliveryID
				}
			}
			return ""
		}
		contributionProvenance := findingProvenance(contributionFindingID)
		for _, evidence := range providerRelease.Inclusions.PullEvidence {
			providerIncludes = providerIncludes || evidence.PullRequestID == contributionResource && evidence.SourceCommitID == contributionRevision
		}
		for _, evidence := range consumerRelease.Inclusions.PullEvidence {
			consumerIncludes = consumerIncludes || evidence.PullRequestID == consumerPull.ID && evidence.SourceCommitID == consumerPull.SourceCommitID
		}
		runs, be := buildStore.List(in.ConsumerRepositoryID, in.ConsumerPullRequestID)
		checkIDs, passed := []string{}, be == nil
		for _, run := range runs {
			if run.CommitID == consumerPull.SourceCommitID {
				checkIDs = append(checkIDs, run.ID)
				passed = passed && run.State == "succeeded"
			}
		}
		consumer, catalogErr := catalog.GetByID(in.ConsumerRepositoryID)
		packageName, packageVersion, packageProven := "", "", false
		versions, versionErr := packageStore.List()
		inventory, inventoryErr := packageStore.GetInventory(in.ConsumerRepositoryID, consumerRelease.CommitID)
		if versionErr == nil && inventoryErr == nil {
			packageName, packageVersion, packageProven = exactAdoptionPackage(versions, inventory, in.ProviderRepositoryID, providerRelease.ID, providerRelease.CommitID)
		}
		replacedPaths, localPatchProven := []string{}, in.ReplacesContributionID == ""
		if in.ReplacesContributionID != "" {
			localPullID, localRepositoryID := "", ""
			for _, contribution := range workspace.Contributions {
				if contribution.ID == in.ReplacesContributionID && contribution.Kind == "local_pull" && contributionProvenance != "" && findingProvenance(contribution.FindingID) == contributionProvenance {
					localPullID, localRepositoryID = contribution.ResourceID, contribution.TargetRepositoryID
				}
			}
			localChanges, localErr := pullStore.Changes(localRepositoryID, localPullID)
			consumerChanges, updateErr := pullStore.Changes(in.ConsumerRepositoryID, consumerPull.ID)
			if localErr == nil && updateErr == nil && localRepositoryID == in.ConsumerRepositoryID {
				replacedPaths, localPatchProven = adoptionPatchCoverage(localChanges, consumerChanges)
			}
		}
		if pe != nil || ce != nil || re != nil || de != nil || we != nil || catalogErr != nil || versionErr != nil || inventoryErr != nil || !consumer.HasParticipant(actor.UserID) || consumerPull.Status != pullrequests.Merged || consumerPull.MergeCommitID == nil || !providerIncludes || !consumerIncludes || promotion.State != "succeeded" || promotion.ReleaseID != consumerRelease.ID || promotion.CommitID != consumerRelease.CommitID || !passed || len(checkIDs) == 0 || !packageProven || !localPatchProven {
			writeAPIError(w, 422, "unverified_adoption_update", "accepted provider release and exact checked consumer rollout must resolve through ordinary records")
			return
		}
		in.ProviderReleaseRevision, in.ConsumerPullRevision, in.ConsumerReleaseRevision, in.CheckRunIDs, in.State = providerRelease.CommitID, consumerPull.SourceCommitID, consumerRelease.CommitID, checkIDs, "verified"
		in.VerificationKind, in.PackageName, in.PackageVersion, in.ReplacedPaths = "exact_package_inventory", packageName, packageVersion, replacedPaths
		out, createErr := store.RecordVerifiedUpdate(r.PathValue("workspace_id"), in.VerifiedUpdate, viewer(actor), in.ExpectedVersion)
		writeAdoptionWorkspace(w, out, createErr, 201)
	})
}

func writeAdoptionWorkspace(w http.ResponseWriter, x adoptionworkspaces.Workspace, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, x)
	case errors.Is(e, adoptionworkspaces.ErrNotFound):
		writeAPIError(w, 404, "adoption_workspace_not_found", "adoption workspace not found")
	case errors.Is(e, adoptionworkspaces.ErrConflict):
		writeAPIError(w, 409, "adoption_workspace_changed", "adoption workspace changed; refresh before responding")
	case errors.Is(e, adoptionworkspaces.ErrInvalid):
		writeAPIError(w, 422, "invalid_adoption_workspace", "adoption requirements, evidence, permissions, or versions are invalid")
	case errors.Is(e, adoptionworkspaces.ErrForbidden):
		writeAPIError(w, 403, "adoption_plan_forbidden", "only consented human adopters and provider participants may record an adoption agreement")
	default:
		log.Printf("adoption workspace storage: %v", e)
		writeAPIError(w, 500, "adoption_workspace_unavailable", "adoption workspace could not be persisted")
	}
}
