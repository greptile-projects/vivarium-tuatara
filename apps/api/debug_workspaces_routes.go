package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
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
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
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

func registerDebugWorkspaceRoutes(mux *http.ServeMux, gitStore *storage.Store, catalog *repositories.Store, credentials *auth.Store, workspaces *debugworkspaces.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, issueStore *issues.Store, incidentStore *incidents.Store, supportStore *supportthreads.Store, objectiveStore *serviceobjectives.Store, packageStore *packages.Store, infrastructureStore *infrastructure.Store) {
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
		visibleProbeIDs := map[string]bool{}
		for _, p := range v.Probes {
			if p.Status == "approved" && !time.Now().UTC().Before(p.ExpiresAt) {
				p.Status = "expired"
			}
			if p.RequestedBy == actor {
				visible = append(visible, p)
				visibleProbeIDs[p.ID] = true
				continue
			}
			for _, id := range p.AudienceUserIDs {
				if id == actor {
					visible = append(visible, p)
					visibleProbeIDs[p.ID] = true
					break
				}
			}
		}
		v.Probes = visible
		for i := range v.Citations {
			if v.Citations[i].Kind != "runtime_evidence" {
				continue
			}
			visibleEvidence := false
			for _, evidence := range v.Evidence {
				if evidence.ID == v.Citations[i].EvidenceID && evidence.Available {
					visibleEvidence = true
				}
			}
			if v.Citations[i].EvidenceID != "" && !visibleEvidence {
				v.Citations[i].Accessible, v.Citations[i].BlockedReason = false, "selected runtime evidence is inaccessible to this reader"
				v.Citations[i].Label, v.Citations[i].ResourceID, v.Citations[i].Revision, v.Citations[i].Path, v.Citations[i].Symbol = "Inaccessible evidence", "", "", "", ""
			}
		}
		history := make([]debugworkspaces.Event, 0, len(v.History))
		for _, event := range v.History {
			if strings.HasPrefix(event.Kind, "probe_") && !visibleProbeIDs[event.To] {
				continue
			}
			history = append(history, event)
		}
		v.History = history
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
		writeDebugWorkspace(w, project(out, actorID(c)), err, 201)
	})
	type claimInput struct {
		ExpectedVersion int                        `json:"expected_version"`
		Claim           debugworkspaces.Claim      `json:"claim"`
		Citations       []debugworkspaces.Citation `json:"citations"`
	}
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/claims", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, e := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if e != nil || !canRead(current, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
			return
		}
		var in claimInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a cited diagnostic claim is required")
			return
		}
		projected := project(current, actorID(c))
		if e = resolveDebugCitations(gitStore, issueStore, deploymentStore, packageStore, infrastructureStore, projected, in.Citations); e != nil {
			writeAPIError(w, 422, "invalid_debugging_citation", e.Error())
			return
		}
		out, e := workspaces.AddClaim(current.RepositoryID, current.ID, actorID(c), in.Citations, in.Claim, in.ExpectedVersion)
		writeDebugWorkspace(w, project(out, actorID(c)), e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/claims/{claim_id}/responses", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int      `json:"expected_version"`
			Kind            string   `json:"kind"`
			Message         string   `json:"message"`
			CitationIDs     []string `json:"citation_ids"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a supported, disputed, or stale response is required")
			return
		}
		out, e := workspaces.RespondClaim(r.PathValue("id"), r.PathValue("workspace_id"), r.PathValue("claim_id"), actorID(c), in.Kind, in.Message, in.CitationIDs, in.ExpectedVersion)
		writeDebugWorkspace(w, project(out, actorID(c)), e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/owner-requests", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                          `json:"expected_version"`
			Request         debugworkspaces.OwnerRequest `json:"request"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded owner question is required")
			return
		}
		out, e := workspaces.RequestOwner(r.PathValue("id"), r.PathValue("workspace_id"), actorID(c), in.Request, in.ExpectedVersion)
		writeDebugWorkspace(w, project(out, actorID(c)), e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/owner-requests/{request_id}/answer", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Response        string `json:"response"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an owner response is required")
			return
		}
		out, e := workspaces.AnswerOwner(r.PathValue("id"), r.PathValue("workspace_id"), r.PathValue("request_id"), actorID(c), in.Response, in.ExpectedVersion)
		writeDebugWorkspace(w, project(out, actorID(c)), e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/agent-investigations", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int      `json:"expected_version"`
			Mandate         string   `json:"mandate"`
			CitationIDs     []string `json:"citation_ids"`
			ExpiresIn       int      `json:"expires_in"`
		}
		if decodeJSON(r, &in) != nil || in.ExpiresIn < 300 || in.ExpiresIn > 86400 {
			writeAPIError(w, 422, "invalid_agent_investigation", "select citations, guidance, and a 5 minute to 24 hour expiry")
			return
		}
		issued, e := credentials.Issue(c.UserID, auth.API, "Debugging investigation", []string{"debugging:investigate"}, time.Duration(in.ExpiresIn)*time.Second)
		if e != nil {
			writeAPIError(w, 500, "debugging_agent_unavailable", "read-only access could not be issued")
			return
		}
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		out, x, e := workspaces.StartAgent(r.PathValue("id"), r.PathValue("workspace_id"), actorID(c), hex.EncodeToString(b), issued.ID, in.Mandate, in.CitationIDs, in.ExpectedVersion)
		if e != nil {
			_, _ = credentials.Revoke(c.UserID, issued.ID)
			writeDebugWorkspace(w, out, e, 201)
			return
		}
		writeJSON(w, 201, map[string]any{"debugging_workspace": project(out, actorID(c)), "agent_investigation": x, "credential": issued})
	})
	mux.HandleFunc("GET /repositories/{id}/debugging-workspaces/{workspace_id}/agent-investigations/{investigation_id}", func(w http.ResponseWriter, r *http.Request) {
		c, ok := authenticateRequest(w, r, credentials, "debugging:investigate", false)
		if !ok {
			return
		}
		v, e := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if e != nil {
			writeAPIError(w, 404, "debugging_agent_not_found", "investigation not found")
			return
		}
		for _, x := range v.AgentInvestigations {
			if x.ID == r.PathValue("investigation_id") && x.CredentialID == c.ID && x.State != "revoked" {
				if catalog.WithCurrentParticipant(x.InitiatorID, v.RepositoryID, func() error { return nil }) != nil {
					writeAPIError(w, 403, "debugging_agent_access_changed", "the investigation initiator no longer has repository access")
					return
				}
				allowed := []debugworkspaces.Citation{}
				for _, citation := range v.Citations {
					for _, id := range x.CitationIDs {
						if citation.ID == id {
							allowed = append(allowed, citation)
						}
					}
				}
				claims := []debugworkspaces.Claim{}
				for _, claim := range v.Claims {
					permitted := true
					for _, id := range claim.CitationIDs {
						found := false
						for _, allowedID := range x.CitationIDs {
							found = found || id == allowedID
						}
						permitted = permitted && found
					}
					if permitted {
						claims = append(claims, claim)
					}
				}
				writeJSON(w, 200, map[string]any{"investigation": x, "citations": allowed, "claims": claims})
				return
			}
		}
		writeAPIError(w, 404, "debugging_agent_not_found", "investigation not found")
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/agent-investigations/{investigation_id}/claims", func(w http.ResponseWriter, r *http.Request) {
		c, ok := authenticateRequest(w, r, credentials, "debugging:investigate", false)
		if !ok {
			return
		}
		current, getErr := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if getErr != nil {
			writeAPIError(w, 404, "debugging_agent_not_found", "investigation not found")
			return
		}
		initiator := ""
		for _, investigation := range current.AgentInvestigations {
			if investigation.ID == r.PathValue("investigation_id") && investigation.CredentialID == c.ID {
				initiator = investigation.InitiatorID
			}
		}
		if initiator == "" || catalog.WithCurrentParticipant(initiator, current.RepositoryID, func() error { return nil }) != nil {
			writeAPIError(w, 403, "debugging_agent_access_changed", "the investigation initiator no longer has repository access")
			return
		}
		var in struct {
			ExpectedVersion int                   `json:"expected_version"`
			Claim           debugworkspaces.Claim `json:"claim"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an uncertain cited agent claim is required")
			return
		}
		out, e := workspaces.AgentClaim(r.PathValue("id"), r.PathValue("workspace_id"), r.PathValue("investigation_id"), c.ID, in.Claim, in.ExpectedVersion)
		writeDebugWorkspace(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/agent-investigations/{investigation_id}/controls", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Action          string `json:"action"`
			Message         string `json:"message"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a guide, pause, resume, or revoke control is required")
			return
		}
		out, x, e := workspaces.ControlAgent(r.PathValue("id"), r.PathValue("workspace_id"), r.PathValue("investigation_id"), actorID(c), in.Action, in.Message, in.ExpectedVersion)
		if e == nil && in.Action == "revoke" {
			_, _ = credentials.Revoke(x.InitiatorID, x.CredentialID)
		}
		writeDebugWorkspace(w, project(out, actorID(c)), e, 201)
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

func resolveDebugCitations(gitStore *storage.Store, issueStore *issues.Store, deploymentStore *deployments.Store, packageStore *packages.Store, infrastructureStore *infrastructure.Store, v debugworkspaces.Workspace, citations []debugworkspaces.Citation) error {
	if len(citations) == 0 || len(citations) > 20 {
		return errors.New("one to twenty revision-aware citations are required")
	}
	for i := range citations {
		c := &citations[i]
		c.Label = strings.TrimSpace(c.Label)
		if c.Label == "" || len(c.Label) > 500 {
			return errors.New("every citation requires a bounded label")
		}
		c.Accessible = true
		switch c.Kind {
		case "runtime_evidence":
			found := false
			for _, e := range v.Evidence {
				if e.ID == c.EvidenceID && e.Available {
					found = true
					c.ResourceID = e.ID
					c.Revision = v.Source.Revision
				}
			}
			for _, p := range v.Probes {
				for _, a := range p.Actions {
					for _, artifact := range a.Artifacts {
						if artifact.Digest == c.ResourceID {
							found = true
							c.Revision = v.Source.Revision
						}
					}
				}
			}
			if !found {
				c.Accessible = false
				c.BlockedReason = "selected runtime evidence is unavailable to this reader"
			}
		case "symbol":
			if c.Revision != v.Source.Revision || strings.TrimSpace(c.Path) == "" || strings.TrimSpace(c.Symbol) == "" || c.LineStart < 1 || c.LineEnd < c.LineStart || c.LineEnd-c.LineStart > 200 {
				return errors.New("symbol citations must select bounded lines at the workspace source revision")
			}
			repo, e := gitStore.Open(v.RepositoryID)
			if e != nil {
				return errors.New("source repository is unavailable")
			}
			commit, e := repo.ReadCommit(storage.ObjectID(c.Revision))
			if e != nil {
				return errors.New("source revision is unavailable")
			}
			entry, e := resolvePath(repo, commit.Tree, c.Path)
			if e != nil || entry.Type != storage.BlobObject {
				return errors.New("cited source path is unavailable")
			}
			blob, e := repo.ReadObject(entry.ID)
			if e != nil || c.LineEnd > len(strings.Split(string(blob.Content), "\n")) {
				return errors.New("cited source lines are unavailable")
			}
		case "commit":
			if c.Revision != v.Source.Revision {
				return errors.New("commit citation must equal the affected source revision")
			}
		case "dependency":
			found := false
			for _, p := range v.Packages {
				if p.ResourceID == c.ResourceID && p.Revision == c.Revision {
					found = true
				}
			}
			if !found {
				return errors.New("dependency citation is not frozen in this workspace")
			}
			if _, e := packageStore.ListRepository(v.RepositoryID); e != nil {
				return errors.New("dependency inventory is unavailable")
			}
		case "configuration":
			if c.Revision != v.Configuration.Revision {
				return errors.New("configuration citation is not revision-exact")
			}
		case "infrastructure":
			if c.ResourceID != v.Infrastructure.ResourceID || c.Revision != v.Infrastructure.Revision {
				return errors.New("infrastructure citation is not frozen in this workspace")
			}
			if _, e := infrastructureStore.Get(c.ResourceID, true); e != nil {
				return errors.New("infrastructure citation is unavailable")
			}
		case "deployment":
			p, e := deploymentStore.GetPromotion(v.RepositoryID, c.ResourceID)
			if e != nil || p.CommitID != v.Source.Revision || p.EnvironmentID != v.Environment.ResourceID {
				return errors.New("deployment citation does not describe the affected release and environment")
			}
			c.Revision = p.CommitID
		case "known_issue":
			x, e := issueStore.Get(v.RepositoryID, c.ResourceID)
			if e != nil {
				return errors.New("known issue citation is unavailable")
			}
			c.Revision = strconv.Itoa(x.Version)
		default:
			return errors.New("citation kind must be runtime_evidence, symbol, commit, dependency, configuration, infrastructure, deployment, or known_issue")
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
