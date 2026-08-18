package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/debugworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/infrastructure"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
)

type debugMutationInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Kind            string `json:"kind"`
	Value           string `json:"value"`
	Message         string `json:"message"`
}
type debugProbeDecisionInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Decision        string                      `json:"decision"`
	Reason          string                      `json:"reason"`
	Policy          debugworkspaces.ProbePolicy `json:"policy"`
	ExpiresAt       time.Time                   `json:"expires_at"`
}
type debugProbeActionInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Action          debugworkspaces.ProbeAction `json:"action"`
}
type debugProbeRevokeInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Reason          string `json:"reason"`
}

func registerDebugWorkspaceRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, workspaces *debugworkspaces.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, issueStore *issues.Store, incidentStore *incidents.Store, supportStore *supportthreads.Store, objectiveStore *serviceobjectives.Store, packageStore *packages.Store, infrastructureStore *infrastructure.Store) {
	actorID := func(c auth.Credential) string {
		if c.AgentID != "" {
			return c.AgentID
		}
		return c.UserID
	}
	canRead := func(v debugworkspaces.Workspace, actor string) bool {
		if v.Audience != "restricted" || actor == v.CreatedBy {
			return true
		}
		for _, id := range v.AccessUserIDs {
			if id == actor {
				return true
			}
		}
		return false
	}
	project := func(v debugworkspaces.Workspace, actor string) debugworkspaces.Workspace {
		privileged := actor == v.CreatedBy
		for _, id := range v.OwnerIDs {
			if id == actor {
				privileged = true
			}
		}
		for _, id := range v.AccessUserIDs {
			if id == actor {
				privileged = true
			}
		}
		if !privileged {
			for i := range v.Evidence {
				if v.Evidence[i].Visibility == "restricted" {
					v.Evidence[i].Reference = ""
					v.Evidence[i].Label = "Restricted evidence"
					v.Evidence[i].Sanitization = ""
					v.Evidence[i].Available = false
					v.Evidence[i].UnavailableReason = "restricted evidence is unavailable to this reader"
				}
			}
		}
		visible := []debugworkspaces.Probe{}
		for _, p := range v.Probes {
			if p.Status == "approved" && !time.Now().UTC().Before(p.ExpiresAt) {
				p.Status = "expired"
			}
			if privileged || p.RequestedBy == actor {
				visible = append(visible, p)
				continue
			}
			for _, id := range p.AudienceUserIDs {
				if id == actor {
					visible = append(visible, p)
					break
				}
			}
		}
		v.Probes = visible
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/debugging-workspaces", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, err := workspaces.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "debugging_workspaces_unavailable", "debugging workspaces could not be read")
			return
		}
		out := []debugworkspaces.Workspace{}
		for _, v := range values {
			if canRead(v, actorID(c)) {
				out = append(out, project(v, actorID(c)))
			}
		}
		writeJSON(w, 200, map[string]any{"debugging_workspaces": out})
	})
	mux.HandleFunc("GET /repositories/{id}/debugging-workspaces/{workspace_id}", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, err := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if err != nil || !canRead(v, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
			return
		}
		writeJSON(w, 200, project(v, actorID(c)))
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in debugworkspaces.Workspace
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete revision-exact debugging workspace is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		if err := validateDebugContext(&in, releaseStore, deploymentStore, issueStore, incidentStore, supportStore, objectiveStore, packageStore, infrastructureStore); err != nil {
			writeAPIError(w, 422, "debugging_context_unavailable", err.Error())
			return
		}
		out, err := workspaces.Create(in, actorID(c))
		writeDebugWorkspace(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/events", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if err != nil || !canRead(current, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
			return
		}
		var in debugMutationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an expected version and attributable event are required")
			return
		}
		out, err := workspaces.Update(current.RepositoryID, current.ID, actorID(c), in.Kind, in.Value, in.Message, in.ExpectedVersion)
		writeDebugWorkspace(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/probes", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if err != nil || !canRead(current, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
			return
		}
		var in struct {
			ExpectedVersion int                   `json:"expected_version"`
			Probe           debugworkspaces.Probe `json:"probe"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded probe preview is required")
			return
		}
		out, err := workspaces.RequestProbe(current.RepositoryID, current.ID, actorID(c), in.Probe, in.ExpectedVersion)
		writeDebugWorkspace(w, project(out, actorID(c)), err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/probes/{probe_id}/decision", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in debugProbeDecisionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an owner decision is required")
			return
		}
		out, err := workspaces.DecideProbe(r.PathValue("id"), r.PathValue("workspace_id"), r.PathValue("probe_id"), actorID(c), in.Decision, in.Reason, in.Policy, in.ExpiresAt, in.ExpectedVersion)
		writeDebugWorkspace(w, project(out, actorID(c)), err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/probes/{probe_id}/actions", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in debugProbeActionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a retained collection outcome is required")
			return
		}
		out, err := workspaces.ReportProbe(r.PathValue("id"), r.PathValue("workspace_id"), r.PathValue("probe_id"), actorID(c), in.Action, in.ExpectedVersion)
		writeDebugWorkspace(w, project(out, actorID(c)), err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/probes/{probe_id}/revoke", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in debugProbeRevokeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a revocation reason is required")
			return
		}
		out, err := workspaces.RevokeProbe(r.PathValue("id"), r.PathValue("workspace_id"), r.PathValue("probe_id"), actorID(c), in.Reason, in.ExpectedVersion)
		writeDebugWorkspace(w, project(out, actorID(c)), err, 201)
	})
}

func validateDebugContext(v *debugworkspaces.Workspace, releasesStore *releases.Store, deploymentStore *deployments.Store, issueStore *issues.Store, incidentStore *incidents.Store, supportStore *supportthreads.Store, objectiveStore *serviceobjectives.Store, packageStore *packages.Store, infrastructureStore *infrastructure.Store) error {
	rel, err := releasesStore.Get(v.RepositoryID, v.Release.ResourceID)
	if err != nil || rel.CommitID != v.Release.Revision {
		return errors.New("affected release and exact source revision are unavailable")
	}
	if v.Source.Revision != rel.CommitID {
		return errors.New("source revision must equal the affected release commit")
	}
	if _, err = deploymentStore.GetEnvironment(v.RepositoryID, v.Environment.ResourceID); err != nil {
		return errors.New("affected environment is unavailable")
	}
	switch v.Trigger.Kind {
	case "issue":
		var x issues.Issue
		x, err = issueStore.Get(v.RepositoryID, v.Trigger.ResourceID)
		if err == nil {
			v.Trigger.Revision = strconv.Itoa(x.Version)
		}
	case "incident":
		var x incidents.Incident
		x, err = incidentStore.Get(v.Trigger.ResourceID)
		if err == nil {
			found := false
			for _, s := range x.Scopes {
				if s.RepositoryID == v.RepositoryID {
					found = true
				}
			}
			if !found {
				err = incidents.ErrNotFound
			} else {
				v.Trigger.Revision = strconv.Itoa(x.Version)
			}
		}
	case "support_thread":
		var x supportthreads.Thread
		x, err = supportStore.Get(v.RepositoryID, v.Trigger.ResourceID)
		if err == nil {
			v.Trigger.Revision = strconv.Itoa(x.Version)
		}
	case "deployment":
		var x deployments.Promotion
		x, err = deploymentStore.GetPromotion(v.RepositoryID, v.Trigger.ResourceID)
		if err == nil && (x.ReleaseID != v.Release.ResourceID || x.EnvironmentID != v.Environment.ResourceID || x.CommitID != rel.CommitID) {
			err = deployments.ErrNotFound
		}
		if err == nil {
			v.Trigger.Revision = x.CommitID
		}
	case "service_objective":
		var x serviceobjectives.Contract
		x, err = objectiveStore.Get(v.Trigger.ResourceID)
		if err == nil && x.RepositoryID != v.RepositoryID {
			err = serviceobjectives.ErrNotFound
		}
		if err == nil {
			v.Trigger.Revision = strconv.Itoa(x.CurrentVersion)
		}
	case "trace", "manual_observation":
		err = nil
		if v.Trigger.Revision == "" {
			v.Trigger.Revision = rel.CommitID
		}
	}
	if err != nil {
		return errors.New("trigger context is unavailable in this repository")
	}
	versions, err := packageStore.ListRepository(v.RepositoryID)
	if err != nil {
		return errors.New("package context is unavailable")
	}
	for _, ref := range v.Packages {
		found := false
		for _, p := range versions {
			if p.ID == ref.ResourceID && p.SourceCommit == rel.CommitID && ref.Revision == p.Version {
				found = true
			}
		}
		if !found {
			return errors.New("a package revision is unavailable at the affected release")
		}
	}
	if v.Configuration.Revision != "" && v.Configuration.Revision != rel.CommitID {
		return errors.New("configuration revision must resolve at the affected release commit")
	}
	if v.Infrastructure.ResourceID != "" {
		d, e := infrastructureStore.Get(v.Infrastructure.ResourceID, true)
		if e != nil || d.RepositoryID != v.RepositoryID {
			return errors.New("infrastructure context is unavailable")
		}
		wanted, e := strconv.Atoi(v.Infrastructure.Revision)
		if e != nil || wanted < 1 || wanted > d.CurrentVersion {
			return errors.New("infrastructure revision is unavailable")
		}
	}
	return nil
}
func writeDebugWorkspace(w http.ResponseWriter, v debugworkspaces.Workspace, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, debugworkspaces.ErrInvalid):
		writeAPIError(w, 422, "invalid_debugging_workspace", "debugging workspace context is incomplete or invalid")
	case errors.Is(err, debugworkspaces.ErrConflict):
		writeAPIError(w, 409, "debugging_workspace_changed", "debugging workspace changed; refresh before appending history")
	case errors.Is(err, debugworkspaces.ErrNotFound):
		writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
	case errors.Is(err, debugworkspaces.ErrForbidden):
		writeAPIError(w, 403, "debugging_probe_forbidden", "the affected environment owner or probe requester must perform this action")
	default:
		writeAPIError(w, 500, "debugging_workspace_unavailable", "debugging workspace could not be persisted")
	}
}
