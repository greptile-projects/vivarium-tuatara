package main

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributoropportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	productfeedback "github.com/greptile-projects/vivarium-tuatara/apps/api/feedback"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/governance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incubators"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/outcomevalidations"
	packageversions "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/projectfunds"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/roadmaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

type incubatorCreateInput struct {
	incubators.Incubator
	Invitations []incubators.Invitation `json:"invitations"`
}
type incubatorEventInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Kind            string `json:"kind"`
	DecisionKind    string `json:"decision_kind"`
	Body            string `json:"body"`
	Visibility      string `json:"visibility"`
}
type incubatorConsentInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Decision        string `json:"decision"`
}
type incubatorAlternativeInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Sources         []incubators.ResearchSource `json:"sources"`
	Alternative     incubators.Alternative      `json:"alternative"`
}
type incubatorExperimentInput struct {
	ExpectedVersion int                             `json:"expected_version"`
	Experiment      incubators.ExperimentDefinition `json:"experiment"`
}
type incubatorExperimentResultInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Result          incubators.ExperimentResult `json:"result"`
}
type incubatorResearchNoteInput struct {
	ExpectedVersion int                     `json:"expected_version"`
	Note            incubators.ResearchNote `json:"note"`
}
type incubatorBootstrapInput struct {
	ExpectedVersion int                            `json:"expected_version"`
	AlternativeID   string                         `json:"alternative_id"`
	Resources       []incubators.BootstrapResource `json:"resources"`
}
type incubatorBootstrapDecisionInput struct {
	ExpectedVersion int    `json:"expected_version"`
	PlanVersion     int    `json:"plan_version"`
	Decision        string `json:"decision"`
}
type incubatorBootstrapActionInput struct {
	ExpectedVersion int    `json:"expected_version"`
	PlanVersion     int    `json:"plan_version"`
	Action          string `json:"action"`
}
type incubatorDeliveryInput struct {
	ExpectedVersion int                     `json:"expected_version"`
	Plan            incubators.DeliveryPlan `json:"plan"`
}
type incubatorDeliveryReportInput struct {
	ExpectedVersion int                       `json:"expected_version"`
	PlanVersion     int                       `json:"plan_version"`
	Report          incubators.DeliveryReport `json:"report"`
}
type incubatorReadinessInput struct {
	ExpectedVersion int                        `json:"expected_version"`
	Readiness       incubators.LaunchReadiness `json:"readiness"`
}
type incubatorReadinessDecisionInput struct {
	ExpectedVersion  int                          `json:"expected_version"`
	ReadinessVersion int                          `json:"readiness_version"`
	Decision         incubators.ReadinessDecision `json:"decision"`
}
type incubatorLaunchInput struct {
	ExpectedVersion int                      `json:"expected_version"`
	Launch          incubators.ProjectLaunch `json:"launch"`
}
type incubatorLaunchObservationInput struct {
	ExpectedVersion int                          `json:"expected_version"`
	LaunchVersion   int                          `json:"launch_version"`
	Observation     incubators.LaunchObservation `json:"observation"`
}
type incubatorStewardshipWorkInput struct {
	ExpectedVersion int                        `json:"expected_version"`
	LaunchVersion   int                        `json:"launch_version"`
	Work            incubators.StewardshipWork `json:"work"`
}
type incubatorStewardshipTransitionInput struct {
	ExpectedVersion int                              `json:"expected_version"`
	LaunchVersion   int                              `json:"launch_version"`
	Transition      incubators.StewardshipTransition `json:"transition"`
}

func registerIncubatorRoutes(mux *http.ServeMux, git *storage.Store, credentials *auth.Store, identities *users.Store, catalog *repositories.Store, orgs *organizations.Store, store *incubators.Store, feedback *productfeedback.Store, support *supportthreads.Store, proposals *governance.Store, workspaceStore *workspaces.Store, pullStore *pullrequests.Store, previewStore *previews.Store, checkStore *checkruns.Store, releaseStore *releases.Store, documentationStore *docscollections.Store, packageStore *packageversions.Store, apiStore *apicontracts.Store, opportunityStore *contributoropportunities.Store, deploymentStore *deployments.Store, roadmapStore *roadmaps.Store, objectiveStore *serviceobjectives.Store, fundStore *projectfunds.Store, outcomeStore *outcomevalidations.Store) {
	store.ConfigureLaunchResolvers(func(a incubators.LaunchArtifact) bool {
		if releaseStore == nil || documentationStore == nil || packageStore == nil || apiStore == nil || opportunityStore == nil || deploymentStore == nil {
			return false
		}
		switch a.Kind {
		case "release":
			x, e := releaseStore.Get(a.RepositoryID, a.ResourceID)
			return e == nil && (a.Revision == "" || x.CommitID == a.Revision)
		case "documentation":
			x, e := documentationStore.Current(a.RepositoryID, a.ResourceID)
			return e == nil && (a.Revision == "" || x.SourceRevision == a.Revision)
		case "package":
			parts := strings.SplitN(a.ResourceID, "@", 2)
			if len(parts) != 2 {
				return false
			}
			x, e := packageStore.Get(parts[0], parts[1])
			return e == nil && x.RepositoryID == a.RepositoryID && (a.Revision == "" || x.SourceCommit == a.Revision)
		case "api_contract":
			x, e := apiStore.Get(a.ResourceID)
			if e != nil || x.RepositoryID != a.RepositoryID || len(x.Revisions) == 0 {
				return false
			}
			current := x.Revisions[len(x.Revisions)-1]
			return a.Revision == "" || current.Source.CommitID == a.Revision
		case "contributor_opportunity":
			x, e := opportunityStore.Get(a.RepositoryID, a.ResourceID)
			return e == nil && (a.Revision == "" || x.Revision == a.Revision)
		case "environment":
			x, e := deploymentStore.GetEnvironment(a.RepositoryID, a.ResourceID)
			return e == nil && x.RepositoryID == a.RepositoryID && a.Revision == ""
		}
		return false
	}, func(o incubators.LaunchObservation) bool {
		if feedback == nil || support == nil || objectiveStore == nil || fundStore == nil || outcomeStore == nil {
			return false
		}
		switch o.Kind {
		case "adoption", "feedback":
			x, e := feedback.Get(o.ResourceID)
			return e == nil && x.RepositoryID == o.RepositoryID
		case "support":
			x, e := support.Get(o.RepositoryID, o.ResourceID)
			return e == nil && x.RepositoryID == o.RepositoryID
		case "reliability":
			x, e := objectiveStore.Get(o.ResourceID)
			return e == nil && x.RepositoryID == o.RepositoryID
		case "cost":
			x, e := fundStore.Get(o.ResourceID)
			return e == nil && x.RepositoryID == o.RepositoryID
		case "success_measure":
			x, e := outcomeStore.Get(o.RepositoryID, o.ResourceID)
			return e == nil && x.RepositoryID == o.RepositoryID
		}
		return false
	}, func(w incubators.StewardshipWork) bool {
		if roadmapStore == nil || proposals == nil {
			return false
		}
		if w.Kind == "roadmap_revision" {
			x, e := roadmapStore.Get(w.RepositoryID)
			if e != nil {
				return false
			}
			for _, revision := range x.Revisions {
				if strconv.Itoa(revision.Version) == w.ResourceID {
					return true
				}
			}
			return false
		}
		x, e := proposals.Get(w.ResourceID)
		return e == nil && x.ScopeType == "repository" && x.ScopeID == w.RepositoryID
	}, func(t incubators.StewardshipTransition) bool {
		if orgs == nil || catalog == nil {
			return false
		}
		if t.Disposition == "organization_initiative" {
			parts := strings.SplitN(t.TargetResourceID, ":", 2)
			if len(parts) != 2 {
				return false
			}
			organization, e := orgs.Get(parts[0])
			if e != nil {
				return false
			}
			for _, initiative := range organization.Initiatives {
				if initiative.ID == parts[1] {
					return true
				}
			}
			return false
		}
		if t.Disposition == "merged" {
			_, e := catalog.GetByID(t.TargetResourceID)
			return e == nil
		}
		return true
	}, func(a incubators.LaunchArtifact, resolutionID string) bool {
		if proposals == nil {
			return false
		}
		resolution, e := proposals.Get(resolutionID)
		return e == nil && resolution.Status == "closed" && resolution.ScopeType == "repository" && resolution.ScopeID == a.RepositoryID
	}, func(e incubators.ReadinessEvidence, decisions []incubators.ReadinessDecision) bool {
		for i := len(decisions) - 1; i >= 0; i-- {
			decision := decisions[i]
			if decision.Dimension != e.Dimension {
				continue
			}
			if decision.Kind == "accepted" {
				return e.Status == "current"
			}
			if decision.Kind == "exception" && proposals != nil {
				followUp, err := proposals.Get(decision.FollowUpWork)
				return err == nil && followUp.Status == "closed"
			}
			return false
		}
		return false
	})
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
	resolve := func(source incubators.Source, actor auth.Credential) incubators.Source {
		if source.Kind == "new_idea" {
			source.Resolution = "resolved"
			source.Detail = "Original idea supplied by the creator"
			return source
		}
		source.Resolution = "inaccessible"
		source.Detail = "Source context is unavailable to this collaborator"
		repo, err := catalog.GetByID(source.RepositoryID)
		if err != nil {
			return source
		}
		allowed := actor.UserID == repo.OwnerID
		if !allowed {
			allowed, _ = catalog.HasCollaborator(actor.UserID, repo.ID)
		}
		if repo.Visibility == repositories.Public {
			allowed = true
		}
		if !allowed {
			return source
		}
		exists := false
		switch source.Kind {
		case "feedback":
			if feedback != nil {
				x, e := feedback.Get(source.ResourceID)
				exists = e == nil && x.RepositoryID == repo.ID
			}
		case "support_gap":
			if support != nil {
				_, e := support.Get(repo.ID, source.ResourceID)
				exists = e == nil
			}
		case "governed_proposal":
			if proposals != nil {
				x, e := proposals.Get(source.ResourceID)
				exists = e == nil && x.ScopeType == "repository" && x.ScopeID == repo.ID
			}
		}
		if exists {
			source.Resolution = "resolved"
			source.Detail = "Source context resolved for the creator"
		} else {
			source.Resolution = "missing"
			source.Detail = "Source context does not resolve in the selected repository"
		}
		return source
	}
	actorIdentity := func(actor auth.Credential) (string, string) {
		if actor.AgentID != "" {
			return "agent", actor.AgentID
		}
		return "human", actor.UserID
	}
	resolveResearch := func(source incubators.ResearchSource, actor auth.Credential) incubators.ResearchSource {
		// Admission redacts failed exact-code selectors before persistence. Preserve
		// that opaque classification on later projections instead of trying to
		// resolve the deliberately removed repository coordinates again.
		if source.ID != "" && source.Kind == "code" && source.RepositoryID == "" && map[string]bool{"missing": true, "inaccessible": true}[source.Resolution] {
			return source
		}
		source.Resolution, source.Detail = "inaccessible", "Evidence is outside this researcher's permitted boundary"
		switch source.Kind {
		case "public":
			u, e := url.ParseRequestURI(source.URL)
			if e == nil && u.Scheme == "https" && u.Host != "" {
				source.Resolution, source.Detail = "resolved", "Permitted public evidence"
			} else {
				source.Resolution, source.Detail = "missing", "Public evidence must use an absolute HTTPS URL"
			}
			return source
		case "organization":
			org, e := orgs.Get(source.OrganizationID)
			allowed := e == nil && organizations.HasRole(org, actor.UserID, "")
			if actor.AgentID != "" && e == nil {
				allowed = false
				for _, a := range org.Agents {
					if a.ID == actor.AgentID {
						allowed = true
					}
				}
			}
			if allowed && strings.TrimSpace(source.ResourceID) != "" {
				source.Resolution, source.Detail = "resolved", "Organization evidence admitted within current membership"
			}
			return source
		default:
			repo, e := catalog.GetByID(source.RepositoryID)
			if e != nil {
				source.Resolution, source.Detail = "missing", "Repository evidence does not resolve"
				return source
			}
			allowed := repo.Visibility == repositories.Public || repo.OwnerID == actor.UserID
			if !allowed && actor.UserID != "" {
				allowed, _ = catalog.HasCollaborator(actor.UserID, repo.ID)
			}
			if actor.AgentID != "" {
				allowed = actor.RepositoryID == repo.ID
			}
			if allowed && source.Kind == "code" {
				codePath := strings.TrimSpace(source.Path)
				if git == nil || len(source.Revision) != 40 || path.Clean(codePath) != codePath || strings.HasPrefix(codePath, "/") || strings.HasPrefix(codePath, "../") {
					source.Resolution, source.Detail = "missing", "Exact code evidence does not resolve"
					source.RepositoryID, source.ResourceID, source.Revision, source.Path = "", "", "", ""
					return source
				}
				gr, openErr := git.Open(repo.ID)
				commit, commitErr := storage.Commit{}, openErr
				if openErr == nil {
					commit, commitErr = gr.ReadCommit(storage.ObjectID(strings.ToLower(source.Revision)))
				}
				if commitErr != nil {
					source.Resolution, source.Detail = "missing", "Exact code evidence does not resolve"
					source.RepositoryID, source.ResourceID, source.Revision, source.Path = "", "", "", ""
					return source
				}
				visible, visibleErr := revisionReachableFromVisibleBranch(gr, commit.ID)
				if visibleErr != nil || !visible {
					source.Resolution, source.Detail = "inaccessible", "Exact code evidence is not reachable from a visible branch"
					source.RepositoryID, source.ResourceID, source.Revision, source.Path = "", "", "", ""
					return source
				}
				entry, pathErr := resolvePath(gr, commit.Tree, codePath)
				if pathErr != nil || entry.Type != storage.BlobObject {
					source.Resolution, source.Detail = "missing", "Exact code evidence path does not resolve"
					source.RepositoryID, source.ResourceID, source.Revision, source.Path = "", "", "", ""
					return source
				}
				source.Revision = strings.ToLower(source.Revision)
				source.Resolution, source.Detail = "resolved", "Exact code file admitted from a visible repository branch"
				return source
			}
			if allowed && strings.TrimSpace(source.ResourceID) != "" {
				source.Resolution, source.Detail = "resolved", "Repository decision, prototype, package, API, or code reference admitted within current read access"
			}
			return source
		}
	}
	projectResearch := func(x incubators.Incubator, actor auth.Credential) incubators.Incubator {
		for i := range x.ResearchSources {
			projected := resolveResearch(x.ResearchSources[i], actor)
			if projected.Resolution == "inaccessible" {
				projected.Label = "Restricted evidence"
				projected.URL, projected.OrganizationID, projected.RepositoryID, projected.ResourceID, projected.Revision, projected.Path = "", "", "", "", "", ""
				projected.Detail = "Evidence remains outside this viewer's current permitted boundary"
			}
			x.ResearchSources[i] = projected
		}
		return x
	}
	mux.HandleFunc("POST /incubators", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_creator_required", "a human collaborator must open an incubator")
			return
		}
		var in incubatorCreateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "complete incubator intent and invitations are required")
			return
		}
		for _, id := range in.SponsorIDs {
			if _, e := identities.Get(id); e != nil {
				writeAPIError(w, 422, "invalid_sponsor", "sponsors must be existing human identities")
				return
			}
		}
		for _, invite := range in.Invitations {
			if invite.PrincipalType == "human" {
				if _, e := identities.Get(invite.PrincipalID); e != nil {
					writeAPIError(w, 422, "invalid_invitee", "human invitees must exist")
					return
				}
			} else if invite.PrincipalType == "agent" {
				org, e := orgs.Get(invite.OrganizationID)
				found := false
				if e == nil {
					for _, a := range org.Agents {
						if a.ID == invite.PrincipalID {
							found = true
						}
					}
				}
				if !found {
					writeAPIError(w, 422, "unapproved_agent", "agent invitees must be approved organization agents")
					return
				}
			}
		}
		in.Source = resolve(in.Source, actor)
		out, e := store.Create(in.Incubator, actor.UserID, in.Invitations)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("GET /incubators", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		viewer := actor.UserID
		if actor.AgentID != "" {
			viewer = actor.AgentID
		}
		all, e := store.List(viewer)
		if e != nil {
			writeIncubator(w, incubators.Incubator{}, e, 500)
			return
		}
		for i := range all {
			all[i] = projectResearch(all[i], actor)
		}
		writeJSON(w, 200, map[string]any{"incubators": all})
	})
	mux.HandleFunc("GET /incubators/{incubator_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		viewer := actor.UserID
		if actor.AgentID != "" {
			viewer = actor.AgentID
		}
		out, e := store.Get(r.PathValue("incubator_id"), viewer)
		writeIncubator(w, projectResearch(out, actor), e, 200)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in incubatorEventInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributable event and expected version are required")
			return
		}
		typ, id := "human", actor.UserID
		if actor.AgentID != "" {
			typ, id = "agent", actor.AgentID
		}
		out, e := store.AddEvent(r.PathValue("incubator_id"), typ, id, in.ExpectedVersion, incubators.Event{Kind: in.Kind, DecisionKind: in.DecisionKind, Body: in.Body, Visibility: in.Visibility})
		writeIncubator(w, projectResearch(out, actor), e, 200)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/invitations/{invitation_id}/consent", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_consent_required", "agent participation is governed by its existing approval")
			return
		}
		var in incubatorConsentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an invitation decision and expected version are required")
			return
		}
		out, e := store.Consent(r.PathValue("incubator_id"), r.PathValue("invitation_id"), actor.UserID, in.Decision, in.ExpectedVersion)
		writeIncubator(w, projectResearch(out, actor), e, 200)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/alternatives", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in incubatorAlternativeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete comparison and expected version are required")
			return
		}
		for i := range in.Sources {
			in.Sources[i] = resolveResearch(in.Sources[i], actor)
		}
		typ, id := actorIdentity(actor)
		out, e := store.AddAlternative(r.PathValue("incubator_id"), typ, id, in.ExpectedVersion, in.Sources, in.Alternative)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/experiments", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in incubatorExperimentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a reproducible experiment and expected version are required")
			return
		}
		typ, id := actorIdentity(actor)
		out, e := store.AddExperiment(r.PathValue("incubator_id"), typ, id, in.ExpectedVersion, in.Experiment)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/experiments/{experiment_id}/results", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in incubatorExperimentResultInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a derived experiment result and expected version are required")
			return
		}
		typ, id := actorIdentity(actor)
		out, e := store.AddExperimentResult(r.PathValue("incubator_id"), r.PathValue("experiment_id"), typ, id, in.ExpectedVersion, in.Result)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/research-notes", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in incubatorResearchNoteInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributable research note and expected version are required")
			return
		}
		typ, id := actorIdentity(actor)
		out, e := store.AddResearchNote(r.PathValue("incubator_id"), typ, id, in.ExpectedVersion, in.Note)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/bootstrap-previews", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_owner_required", "a human owner must reserve a project boundary")
			return
		}
		var in incubatorBootstrapInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete bootstrap preview is required")
			return
		}
		visible, e := store.Get(r.PathValue("incubator_id"), actor.UserID)
		if e != nil {
			writeIncubator(w, incubators.Incubator{}, e, 404)
			return
		}
		eligible := map[string]bool{visible.CreatedBy: true}
		for _, invitation := range visible.Invitations {
			if invitation.PrincipalType == "human" && invitation.Status == "accepted" {
				eligible[invitation.PrincipalID] = true
			}
		}
		for _, resource := range in.Resources {
			for _, owner := range resource.OwnerIDs {
				if !eligible[owner] {
					writeAPIError(w, 422, "invalid_owner", "resource owners must be consenting human incubator participants")
					return
				}
				if _, e := identities.Get(owner); e != nil {
					writeAPIError(w, 422, "invalid_owner", "resource owners must be current human identities")
					return
				}
			}
			if resource.Mode != "connect" {
				continue
			}
			switch resource.Kind {
			case "organization":
				org, e := orgs.Get(resource.ResourceID)
				ownersCurrent := e == nil
				for _, owner := range resource.OwnerIDs {
					ownersCurrent = ownersCurrent && organizations.HasRole(org, owner, "owner")
				}
				if !ownersCurrent {
					writeAPIError(w, 422, "inaccessible_resource", "connected resources must exist within the owner's current authority")
					return
				}
			case "repository":
				repo, e := catalog.GetByID(resource.ResourceID)
				ownersCurrent := e == nil
				for _, owner := range resource.OwnerIDs {
					ownersCurrent = ownersCurrent && repo.OwnerID == owner
				}
				if !ownersCurrent {
					writeAPIError(w, 422, "inaccessible_resource", "connected resources must exist within the owner's current authority")
					return
				}
			}
		}
		out, e := store.PreviewBootstrap(r.PathValue("incubator_id"), actor.UserID, in.ExpectedVersion, in.AlternativeID, in.Resources)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/bootstrap-plans/{plan_id}/decisions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_owner_required", "only human resource owners approve activation")
			return
		}
		var in incubatorBootstrapDecisionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an owner decision and exact versions are required")
			return
		}
		out, e := store.DecideBootstrap(r.PathValue("incubator_id"), r.PathValue("plan_id"), actor.UserID, in.Decision, in.ExpectedVersion, in.PlanVersion)
		writeIncubator(w, projectResearch(out, actor), e, 200)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/bootstrap-plans/{plan_id}/actions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_owner_required", "only a human resource owner may activate or roll back")
			return
		}
		var in incubatorBootstrapActionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an activation or rollback action and exact versions are required")
			return
		}
		current, e := store.Get(r.PathValue("incubator_id"), actor.UserID)
		if e != nil {
			writeIncubator(w, incubators.Incubator{}, e, 404)
			return
		}
		var organizationID, repositoryID string
		var organizationOwners, repositoryOwners []string
		for _, plan := range current.BootstrapPlans {
			if plan.ID == r.PathValue("plan_id") {
				for _, resource := range plan.Resources {
					if resource.Mode == "connect" {
						if resource.Kind == "organization" {
							organizationID = resource.ResourceID
							organizationOwners = append([]string{}, resource.OwnerIDs...)
						}
						if resource.Kind == "repository" {
							repositoryID = resource.ResourceID
							repositoryOwners = append([]string{}, resource.OwnerIDs...)
						}
					}
				}
			}
		}
		var out incubators.Incubator
		var finishErr error
		finish := func() error {
			out, finishErr = store.FinishBootstrap(r.PathValue("incubator_id"), r.PathValue("plan_id"), actor.UserID, in.Action, in.ExpectedVersion, in.PlanVersion)
			return finishErr
		}
		withRepository := func() error {
			if in.Action == "activate" && repositoryID != "" {
				return catalog.WithCurrentOwners(repositoryOwners, []string{repositoryID}, finish)
			}
			return finish()
		}
		var authorityErr error
		if in.Action == "activate" && organizationID != "" {
			authorityErr = orgs.WithCurrentOwners(organizationID, organizationOwners, withRepository)
		} else {
			authorityErr = withRepository()
		}
		if finishErr != nil {
			writeIncubator(w, projectResearch(out, actor), finishErr, 200)
			return
		}
		if authorityErr != nil {
			writeAPIError(w, 409, "bootstrap_authority_changed", "a connected resource is missing or no longer controlled by the activating owner")
			return
		}
		writeIncubator(w, projectResearch(out, actor), nil, 200)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/delivery-plans", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_owner_required", "a human participant must create the delivery plan")
			return
		}
		var in incubatorDeliveryInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an ordered representative journey is required")
			return
		}
		for _, item := range in.Plan.WorkItems {
			repo, e := catalog.GetByID(item.RepositoryID)
			allowed := e == nil && (repo.Visibility == repositories.Public || repo.OwnerID == actor.UserID)
			if !allowed && e == nil {
				allowed, _ = catalog.HasCollaborator(actor.UserID, repo.ID)
			}
			gr, openErr := git.Open(item.RepositoryID)
			_, commitErr := storage.Commit{}, openErr
			if openErr == nil {
				_, commitErr = gr.ReadCommit(storage.ObjectID(strings.ToLower(item.BaseRevision)))
			}
			if !allowed || commitErr != nil {
				writeAPIError(w, 422, "invalid_work_scope", "every work item must use a readable bootstrapped repository and exact commit")
				return
			}
		}
		out, e := store.CreateDeliveryPlan(r.PathValue("incubator_id"), actor.UserID, in.ExpectedVersion, in.Plan)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/delivery-plans/{delivery_plan_id}/reports", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in incubatorDeliveryReportInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributable delivery report and exact versions are required")
			return
		}
		resolved := false
		switch in.Report.Kind {
		case "workspace":
			x, e := workspaceStore.Get(in.Report.ResourceID)
			resolved = e == nil && x.RepositoryID == in.Report.RepositoryID && x.CommitID == in.Report.Revision
		case "pull_request":
			x, e := pullStore.Get(in.Report.RepositoryID, in.Report.ResourceID)
			resolved = e == nil && x.SourceCommitID == in.Report.Revision
		case "preview", "check", "review":
			parts := strings.Split(in.Report.ResourceID, ":")
			if len(parts) == 2 {
				switch in.Report.Kind {
				case "preview":
					x, e := previewStore.Get(in.Report.RepositoryID, parts[0], parts[1])
					resolved = e == nil && x.Revision == in.Report.Revision
				case "check":
					x, e := checkStore.Get(in.Report.RepositoryID, parts[0], parts[1])
					resolved = e == nil && x.CommitID == in.Report.Revision
				case "review":
					reviews, e := pullStore.ListReviews(in.Report.RepositoryID, parts[0])
					if e == nil {
						for _, review := range reviews {
							if review.ID == parts[1] && review.ReviewedCommitID == in.Report.Revision && !review.Stale {
								resolved = true
							}
						}
					}
				}
			}
		case "target_user_feedback":
			x, e := feedback.Get(in.Report.ResourceID)
			resolved = e == nil && x.RepositoryID == in.Report.RepositoryID
		default:
			resolved = true
		}
		if !resolved {
			writeAPIError(w, 422, "unresolved_delivery_evidence", "linked delivery evidence must resolve in its repository-owned store at the reported revision")
			return
		}
		typ, id := actorIdentity(actor)
		out, e := store.AddDeliveryReport(r.PathValue("incubator_id"), r.PathValue("delivery_plan_id"), typ, id, in.ExpectedVersion, in.PlanVersion, in.Report)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/launch-readiness", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_owner_required", "a human participant must declare launch readiness")
			return
		}
		var in incubatorReadinessInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete launch readiness view is required")
			return
		}
		out, e := store.CreateLaunchReadiness(r.PathValue("incubator_id"), actor.UserID, in.ExpectedVersion, in.Readiness)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/launch-readiness/{readiness_id}/decisions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_owner_required", "only the named human owner may accept evidence or grant an exception")
			return
		}
		var in incubatorReadinessDecisionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact readiness decision is required")
			return
		}
		if in.Decision.Kind == "exception" {
			if proposals == nil {
				writeAPIError(w, 503, "follow_up_unavailable", "governed follow-up work cannot be resolved")
				return
			}
			followUp, e := proposals.Get(in.Decision.FollowUpWork)
			if e != nil {
				writeAPIError(w, 422, "follow_up_missing", "an exception must connect to existing governed follow-up work")
				return
			}
			incubator, e := store.Get(r.PathValue("incubator_id"), actor.UserID)
			if e != nil {
				writeIncubator(w, incubators.Incubator{}, e, 200)
				return
			}
			admittedRepositories := map[string]bool{}
			for _, readiness := range incubator.LaunchReadiness {
				if readiness.ID != r.PathValue("readiness_id") {
					continue
				}
				for _, delivery := range incubator.DeliveryPlans {
					if delivery.ID == readiness.DeliveryPlanID && delivery.BootstrapPlanID == readiness.BootstrapPlanID {
						for _, item := range delivery.WorkItems {
							admittedRepositories[item.RepositoryID] = true
						}
					}
				}
			}
			inScope := followUp.ScopeType == "repository" && admittedRepositories[followUp.ScopeID]
			if followUp.ScopeType == "organization" {
				for repositoryID := range admittedRepositories {
					repository, repositoryErr := catalog.GetByID(repositoryID)
					if repositoryErr == nil && repository.OrganizationID == followUp.ScopeID {
						inScope = true
					}
				}
			}
			if !inScope {
				writeAPIError(w, 422, "follow_up_scope_mismatch", "exception follow-up work must govern a repository admitted by this readiness delivery boundary")
				return
			}
		}
		out, e := store.DecideLaunchReadiness(r.PathValue("incubator_id"), r.PathValue("readiness_id"), actor.UserID, in.ExpectedVersion, in.ReadinessVersion, in.Decision)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/launches", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_owner_required", "a human collaborator must publish the first project launch")
			return
		}
		var in incubatorLaunchInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact ready launch manifest is required")
			return
		}
		out, e := store.PublishLaunch(r.PathValue("incubator_id"), actor.UserID, in.ExpectedVersion, in.Launch)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/launches/{launch_id}/observations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in incubatorLaunchObservationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact operational observation is required")
			return
		}
		typ, id := actorIdentity(actor)
		out, e := store.AddLaunchObservation(r.PathValue("incubator_id"), r.PathValue("launch_id"), typ, id, in.ExpectedVersion, in.LaunchVersion, in.Observation)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/launches/{launch_id}/work", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in incubatorStewardshipWorkInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "connected roadmap or proposal work is required")
			return
		}
		typ, id := actorIdentity(actor)
		out, e := store.AddStewardshipWork(r.PathValue("incubator_id"), r.PathValue("launch_id"), typ, id, in.ExpectedVersion, in.LaunchVersion, in.Work)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/launches/{launch_id}/transition", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_owner_required", "a human collaborator must decide project stewardship")
			return
		}
		var in incubatorStewardshipTransitionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact stewardship disposition is required")
			return
		}
		out, e := store.TransitionStewardship(r.PathValue("incubator_id"), r.PathValue("launch_id"), actor.UserID, in.ExpectedVersion, in.LaunchVersion, in.Transition)
		writeIncubator(w, projectResearch(out, actor), e, 201)
	})
}

func writeIncubator(w http.ResponseWriter, x incubators.Incubator, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, x)
	case errors.Is(e, incubators.ErrNotFound):
		writeAPIError(w, 404, "incubator_not_found", "incubator not found")
	case errors.Is(e, incubators.ErrConflict):
		writeAPIError(w, 409, "incubator_changed", "incubator changed; refresh before appending")
	case errors.Is(e, incubators.ErrInvalid):
		writeAPIError(w, 422, "invalid_incubator", "incubator intent, attribution, consent, or version is invalid")
	default:
		log.Printf("incubator storage: %v", e)
		writeAPIError(w, 500, "incubator_unavailable", "incubator could not be persisted")
	}
}
