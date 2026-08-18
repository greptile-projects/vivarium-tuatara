package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/debugworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/infrastructure"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	devworkspaces "github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
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

func registerDebugWorkspaceRoutes(mux *http.ServeMux, gitStore *storage.Store, catalog *repositories.Store, credentials *auth.Store, workspaces *debugworkspaces.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, issueStore *issues.Store, incidentStore *incidents.Store, supportStore *supportthreads.Store, objectiveStore *serviceobjectives.Store, packageStore *packages.Store, infrastructureStore *infrastructure.Store, developmentWorkspaces *devworkspaces.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, checkStore *checkruns.Store) {
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
	projectAgentPacket := func(v debugworkspaces.Workspace, x debugworkspaces.AgentInvestigation) map[string]any {
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
				projected := claim
				projected.Responses = []debugworkspaces.ClaimResponse{}
				for _, response := range claim.Responses {
					responsePermitted := true
					for _, id := range response.CitationIDs {
						found := false
						for _, allowedID := range x.CitationIDs {
							found = found || id == allowedID
						}
						responsePermitted = responsePermitted && found
					}
					if responsePermitted {
						projected.Responses = append(projected.Responses, response)
					}
				}
				claims = append(claims, projected)
			}
		}
		return map[string]any{"investigation": x, "citations": allowed, "claims": claims}
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
		current, getErr := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if getErr != nil || !canRead(current, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
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
		current, getErr := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if getErr != nil || !canRead(current, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
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
		current, getErr := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if getErr != nil || !canRead(current, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
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
		current, getErr := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if getErr != nil || !canRead(current, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
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
				if !canRead(v, x.InitiatorID) || catalog.WithCurrentParticipant(x.InitiatorID, v.RepositoryID, func() error { return nil }) != nil {
					writeAPIError(w, 403, "debugging_agent_access_changed", "the investigation initiator no longer has repository access")
					return
				}
				writeJSON(w, 200, projectAgentPacket(v, x))
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
		var selected debugworkspaces.AgentInvestigation
		for _, investigation := range current.AgentInvestigations {
			if investigation.ID == r.PathValue("investigation_id") && investigation.CredentialID == c.ID {
				selected = investigation
			}
		}
		if selected.ID == "" || !canRead(current, selected.InitiatorID) || catalog.WithCurrentParticipant(selected.InitiatorID, current.RepositoryID, func() error { return nil }) != nil {
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
		if e != nil {
			writeDebugWorkspace(w, debugworkspaces.Workspace{}, e, 201)
			return
		}
		for _, investigation := range out.AgentInvestigations {
			if investigation.ID == selected.ID {
				selected = investigation
			}
		}
		writeJSON(w, 201, projectAgentPacket(out, selected))
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/agent-investigations/{investigation_id}/controls", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, getErr := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if getErr != nil || !canRead(current, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
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
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/replay-scenarios", func(w http.ResponseWriter, r *http.Request) {
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
			ExpectedVersion int                            `json:"expected_version"`
			Scenario        debugworkspaces.ReplayScenario `json:"scenario"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a minimized privacy-bounded replay scenario is required")
			return
		}
		out, scenario, err := workspaces.CreateReplay(current.RepositoryID, current.ID, actorID(c), in.Scenario, in.ExpectedVersion)
		if err != nil {
			writeDebugWorkspace(w, project(out, actorID(c)), err, 201)
			return
		}
		writeJSON(w, 201, map[string]any{"debugging_workspace": project(out, actorID(c)), "replay_scenario": scenario})
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/replay-scenarios/{scenario_id}/attempts", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := workspaces.Get(r.PathValue("id"), r.PathValue("workspace_id"))
		if err != nil || !canRead(current, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
			return
		}
		var scenario *debugworkspaces.ReplayScenario
		for i := range current.ReplayScenarios {
			if current.ReplayScenarios[i].ID == r.PathValue("scenario_id") {
				scenario = &current.ReplayScenarios[i]
			}
		}
		if scenario == nil {
			writeAPIError(w, 404, "replay_scenario_not_found", "replay scenario not found")
			return
		}
		if len(scenario.UnsafeSideEffects) > 0 {
			writeAPIError(w, 422, "replay_side_effects_unsafe", "unsafe side effects must be removed before an isolated replay can run")
			return
		}
		if developmentWorkspaces == nil {
			writeAPIError(w, 503, "replay_workspace_unavailable", "isolated workspace evidence is unavailable")
			return
		}
		var in struct {
			ExpectedVersion int                           `json:"expected_version"`
			Attempt         debugworkspaces.ReplayAttempt `json:"attempt"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "revision-exact replay evidence is required")
			return
		}
		dw, e := developmentWorkspaces.Get(strings.TrimSpace(in.Attempt.WorkspaceID))
		if e != nil || dw.RepositoryID != current.RepositoryID || dw.Source.Kind != "debugging_reproduction" || dw.Source.DebuggingWorkspaceID != current.ID || dw.Source.ReplayScenarioID != scenario.ID || dw.State == "provisioning" {
			writeAPIError(w, 422, "replay_workspace_invalid", "attempt must use this scenario's isolated revision-exact debugging workspace")
			return
		}
		meta, metaErr := catalog.GetByID(dw.RepositoryID)
		if metaErr != nil || !canReadReplayWorkspace(dw, c.UserID, meta.OwnerID) {
			writeAPIError(w, 404, "replay_workspace_not_found", "replay workspace not found")
			return
		}
		declared := map[string]devworkspaces.ExperimentCommand{}
		for _, cmd := range dw.Definition.Experiments {
			sum := sha256.Sum256([]byte(cmd.Command))
			declared[cmd.Name] = devworkspaces.ExperimentCommand{Name: cmd.Name, Command: hex.EncodeToString(sum[:])}
		}
		outcomes := map[string]devworkspaces.CommandOutcome{}
		for _, o := range dw.Commands {
			outcomes[o.ID] = o
		}
		selected := map[string]devworkspaces.CommandOutcome{}
		in.Attempt.Outputs = []string{}
		for _, oid := range in.Attempt.CommandOutcomeIDs {
			o, found := outcomes[oid]
			if !found {
				writeAPIError(w, 422, "replay_commands_invalid", "every outcome must come from the selected workspace")
				return
			}
			commandName, matched := replayCommandForOutcome(scenario.Commands, declared, o)
			if !matched {
				writeAPIError(w, 422, "replay_commands_invalid", "every outcome must match a command frozen in the replay scenario")
				return
			}
			if !selectReplayOutcome(selected, commandName, o) {
				writeAPIError(w, 422, "replay_commands_invalid", "an attempt may select only one outcome for each frozen scenario command")
				return
			}
			in.Attempt.Outputs = append(in.Attempt.Outputs, o.Output)
		}
		in.Attempt.CommitID = dw.CommitID
		in.Attempt.DefinitionSHA256 = dw.DefinitionSHA256
		environment, _ := json.Marshal(dw.Definition)
		in.Attempt.Environment = environment
		if dw.CommitID != scenario.CommitID {
			in.Attempt.Gaps = append(in.Attempt.Gaps, "workspace revision differs from the frozen affected revision")
			in.Attempt.ProductionDifferences = append(in.Attempt.ProductionDifferences, "revision changed from "+scenario.CommitID+" to "+dw.CommitID)
			out, attempt, e := workspaces.AddReplayAttempt(current.RepositoryID, current.ID, scenario.ID, actorID(c), in.Attempt, in.ExpectedVersion)
			if e != nil {
				writeDebugWorkspace(w, project(out, actorID(c)), e, 201)
				return
			}
			writeJSON(w, 201, map[string]any{"debugging_workspace": project(out, actorID(c)), "attempt": attempt})
			return
		}
		in.Attempt.Invariants = []debugworkspaces.ReplayInvariantResult{}
		for _, inv := range scenario.Invariants {
			o, found := selected[inv.CommandName]
			if !found {
				writeAPIError(w, 422, "replay_commands_invalid", "every invariant requires its repository-defined command outcome")
				return
			}
			in.Attempt.Invariants = append(in.Attempt.Invariants, debugworkspaces.ReplayInvariantResult{Name: inv.Name, OutcomeID: o.ID, ActualExitCode: o.ExitCode, Passed: o.ExitCode == inv.ExpectedExitCode})
		}
		out, attempt, e := workspaces.AddReplayAttempt(current.RepositoryID, current.ID, scenario.ID, actorID(c), in.Attempt, in.ExpectedVersion)
		if e != nil {
			writeDebugWorkspace(w, project(out, actorID(c)), e, 201)
			return
		}
		writeJSON(w, 201, map[string]any{"debugging_workspace": project(out, actorID(c)), "attempt": attempt})
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/repair-work", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		repo, err := catalog.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		current, err := workspaces.Get(repo.ID, r.PathValue("workspace_id"))
		if err != nil || !canRead(current, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
			return
		}
		var in struct {
			ExpectedVersion    int      `json:"expected_version"`
			ScenarioID         string   `json:"scenario_id"`
			CauseClaimID       string   `json:"cause_claim_id"`
			AcceptanceCriteria []string `json:"acceptance_criteria"`
			RegressionCriteria []string `json:"regression_criteria"`
			AssigneeType       string   `json:"assignee_type"`
			AssigneeID         string   `json:"assignee_id"`
			Title              string   `json:"title"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "proven diagnosis and owned repair criteria are required")
			return
		}
		provenScenario, provenCause := false, false
		for _, scenario := range current.ReplayScenarios {
			provenScenario = provenScenario || (scenario.ID == in.ScenarioID && scenario.Status == "reproduced")
		}
		for _, claim := range current.Claims {
			provenCause = provenCause || (claim.ID == in.CauseClaimID && claim.Kind == "finding" && claim.Status == "supported")
		}
		if !provenScenario || !provenCause {
			writeAPIError(w, 422, "invalid_debugging_repair", "repair requires a reproduced scenario and supported cited cause")
			return
		}
		if in.AssigneeType == "human" {
			participant, _ := catalog.HasCollaborator(in.AssigneeID, repo.ID)
			if repo.OwnerID != in.AssigneeID && !participant {
				writeAPIError(w, 422, "invalid_debugging_repair", "human owner must already participate in the repository")
				return
			}
		}
		identity := sha256.Sum256([]byte(current.ID + "\x00" + in.ScenarioID + "\x00" + in.CauseClaimID))
		workID := hex.EncodeToString(identity[:16])
		reserved, work := current, debugworkspaces.RepairWork{}
		for _, existing := range current.RepairWork {
			if existing.ID == workID {
				work = existing
			}
		}
		if work.ID == "" {
			var reserveErr error
			reserved, work, reserveErr = workspaces.CreateRepairWork(repo.ID, current.ID, actorID(c), debugworkspaces.RepairWork{ID: workID, ScenarioID: in.ScenarioID, CauseClaimID: in.CauseClaimID, AffectedRevision: current.Source.Revision, AcceptanceCriteria: in.AcceptanceCriteria, RegressionCriteria: in.RegressionCriteria, AssigneeType: in.AssigneeType, AssigneeID: in.AssigneeID}, in.ExpectedVersion)
			if reserveErr != nil {
				writeDebugWorkspace(w, project(reserved, actorID(c)), reserveErr, 201)
				return
			}
		} else if work.ValidationStatus != "publishing" || work.AffectedRevision != current.Source.Revision {
			writeAPIError(w, 409, "debugging_repair_changed", "repair work is already published")
			return
		}
		origin := proposals.ReasoningOrigin{DebuggingWorkspaceID: current.ID, DebuggingRepairWorkID: workID, DebuggingScenarioID: in.ScenarioID, DebuggingCauseClaimID: in.CauseClaimID, Revision: current.Source.Revision, SelectedItemIDs: []string{in.ScenarioID, in.CauseClaimID}, Items: []proposals.ReasoningItem{{ID: in.ScenarioID, Kind: "reproduction", Summary: "Minimized production behavior reproduced twice", Status: "confirmed"}, {ID: in.CauseClaimID, Kind: "diagnosis", Summary: "Cited cause selected for repair", Status: "supported"}}, AnalysisStatus: "debugging_repair"}
		criteria := strings.Join(append(append([]string{}, in.AcceptanceCriteria...), in.RegressionCriteria...), "\n- ")
		title := strings.TrimSpace(in.Title)
		if title == "" {
			title = "Repair diagnosed production behavior"
		}
		p, tasks, e := proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: repo.ID, ActorID: actorID(c), Title: title, Body: "Governed repair from debugging workspace " + current.ID + " at affected revision " + current.Source.Revision + ".\n\nCriteria:\n- " + criteria, Origin: origin, Tasks: []proposals.ImplementationTaskInput{{Title: title, Outcome: "Change the reproduced execution for the cited cause and validate it in the affected environment.", Risk: current.Severity + " production regression", VerificationPlan: "Rerun the frozen scenario and every ordinary required check on each pull revision, then validate production signals after staged deployment.\n- " + criteria, AssigneeType: in.AssigneeType, AssigneeID: in.AssigneeID}}})
		if e != nil {
			writeAPIError(w, 422, "invalid_debugging_repair", e.Error())
			return
		}
		out, work, e := workspaces.UpdateRepairWork(repo.ID, current.ID, work.ID, actorID(c), reserved.Version, func(x *debugworkspaces.RepairWork) error {
			x.ProposalID = p.ID
			x.TaskID = tasks[0].ID
			x.AssigneeID = tasks[0].Assignment.AssigneeID
			x.ValidationStatus = "awaiting_pull"
			return nil
		})
		if e != nil {
			writeDebugWorkspace(w, project(out, actorID(c)), e, 201)
			return
		}
		writeJSON(w, 201, map[string]any{"debugging_workspace": project(out, actorID(c)), "repair_work": work, "proposal": p, "task": tasks[0]})
	})
	mux.HandleFunc("POST /repositories/{id}/debugging-workspaces/{workspace_id}/repair-work/{work_id}/validation", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		repo, e := catalog.GetByID(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		current, e := workspaces.Get(repo.ID, r.PathValue("workspace_id"))
		if e != nil || !canRead(current, actorID(c)) {
			writeAPIError(w, 404, "debugging_workspace_not_found", "debugging workspace not found")
			return
		}
		var in struct {
			ExpectedVersion int      `json:"expected_version"`
			PullRequestID   string   `json:"pull_request_id"`
			CheckRunIDs     []string `json:"check_run_ids"`
			ReleaseID       string   `json:"release_id"`
			DeploymentID    string   `json:"deployment_id"`
			SignalNames     []string `json:"signal_names"`
			Outcome         string   `json:"outcome"`
			Summary         string   `json:"summary"`
			Action          string   `json:"action"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "governed delivery evidence is required")
			return
		}
		var work *debugworkspaces.RepairWork
		for i := range current.RepairWork {
			if current.RepairWork[i].ID == r.PathValue("work_id") {
				work = &current.RepairWork[i]
			}
		}
		if work == nil {
			writeAPIError(w, 404, "debugging_repair_not_found", "repair work not found")
			return
		}
		pull, e := pullStore.Get(repo.ID, in.PullRequestID)
		if e != nil || pull.TaskID == nil || *pull.TaskID != work.TaskID || pull.Status != "merged" || pull.MergeCommitID == nil {
			writeAPIError(w, 422, "debugging_delivery_invalid", "validation requires the linked merged pull")
			return
		}
		rel, e := releaseStore.Get(repo.ID, in.ReleaseID)
		if e != nil || !debugContains(rel.Inclusions.PullRequestIDs, pull.ID) {
			writeAPIError(w, 422, "debugging_release_invalid", "release must include the linked integrated pull")
			return
		}
		runs, e := checkStore.List(repo.ID, pull.ID)
		if e != nil {
			writeAPIError(w, 422, "debugging_checks_invalid", "check evidence unavailable")
			return
		}
		selected := debugPassingChecks(runs, in.CheckRunIDs, rel.CommitID)
		if len(selected) != len(in.CheckRunIDs) || len(selected) == 0 {
			writeAPIError(w, 422, "debugging_checks_invalid", "all selected checks must pass at the linked pull revision")
			return
		}
		requiredChecks, e := catalog.RequiredChecks(repo.ID, pull.TargetBranch)
		if e != nil {
			writeAPIError(w, 422, "debugging_required_checks_unavailable", "target-branch required-check policy is unavailable")
			return
		}
		if missing := debugMissingRequiredChecks(selected, requiredChecks); len(missing) > 0 {
			writeAPIError(w, 422, "debugging_required_checks_incomplete", "every target-branch required check must pass at the deployed revision: "+strings.Join(missing, ", "))
			return
		}
		scenarioHashes := map[string]bool{}
		for _, s := range current.ReplayScenarios {
			if s.ID == work.ScenarioID {
				for _, cmd := range s.Commands {
					scenarioHashes[cmd.SHA256] = true
				}
			}
		}
		scenarioRuns, ordinary := []string{}, []string{}
		matchedScenarioHashes := map[string]bool{}
		for id, run := range selected {
			sum := sha256.Sum256([]byte(run.Definition.Command))
			if scenarioHashes[hex.EncodeToString(sum[:])] {
				scenarioRuns = append(scenarioRuns, id)
				matchedScenarioHashes[hex.EncodeToString(sum[:])] = true
			} else {
				ordinary = append(ordinary, id)
			}
		}
		if len(matchedScenarioHashes) != len(scenarioHashes) || len(ordinary) == 0 {
			writeAPIError(w, 422, "debugging_checks_incomplete", "the frozen scenario and ordinary required checks must both pass")
			return
		}
		dep, e := deploymentStore.GetPromotion(repo.ID, in.DeploymentID)
		if e != nil || dep.ReleaseID != rel.ID || dep.CommitID != rel.CommitID {
			writeAPIError(w, 422, "debugging_deployment_invalid", "deployment must deliver the exact repair release")
			return
		}
		if in.Outcome == "validated" && dep.State != "succeeded" {
			writeAPIError(w, 422, "debugging_deployment_incomplete", "validated repair requires a succeeded staged deployment")
			return
		}
		signalStates := map[string]string{}
		for _, signal := range dep.Evidence {
			signalStates[signal.Signal] = signal.State
		}
		failedMeasure := false
		for _, name := range in.SignalNames {
			state, found := signalStates[name]
			failedMeasure = failedMeasure || state == "failed"
			if !found || (in.Outcome == "validated" && state != "passed") {
				writeAPIError(w, 422, "debugging_signals_invalid", "every production validation signal must be retained and validated signals must pass")
				return
			}
		}
		if len(in.SignalNames) == 0 || !oneOf(in.Outcome, "validated", "failed") {
			writeAPIError(w, 422, "debugging_validation_invalid", "a measured outcome and production signals are required")
			return
		}
		if !oneOf(in.Action, "none", "pause", "restore", "reopen") || (in.Outcome == "validated" && in.Action != "none") {
			writeAPIError(w, 422, "debugging_action_invalid", "action must match the measured outcome")
			return
		}
		if in.Outcome == "failed" && !failedMeasure {
			writeAPIError(w, 422, "debugging_signals_invalid", "a failed validation requires a retained failed production measure")
			return
		}
		needsAction := in.Outcome == "failed" && oneOf(in.Action, "pause", "restore")
		out, updated, e := workspaces.UpdateRepairWork(repo.ID, current.ID, work.ID, actorID(c), in.ExpectedVersion, func(x *debugworkspaces.RepairWork) error {
			x.PullRequestID = pull.ID
			x.PullRevision = rel.CommitID
			x.ScenarioCheckRunIDs = scenarioRuns
			x.RequiredCheckRunIDs = ordinary
			x.ReleaseID = rel.ID
			x.DeploymentID = dep.ID
			x.ValidationStatus = in.Outcome
			x.ValidationSummary = in.Summary
			x.ValidationSignalNames = append([]string{}, in.SignalNames...)
			x.ReopenedDiagnosis = in.Outcome == "failed" && in.Action == "reopen"
			x.RequestedAction = in.Action
			x.ActionStatus = "completed"
			if needsAction {
				x.ActionStatus = "pending"
			}
			return nil
		})
		if e != nil {
			writeDebugWorkspace(w, project(out, actorID(c)), e, 201)
			return
		}
		if !needsAction {
			writeJSON(w, 201, map[string]any{"debugging_workspace": project(out, actorID(c)), "repair_work": updated})
			return
		}
		actionDeploymentID := dep.ID
		if in.Action == "pause" {
			if dep.State != "paused" {
				var controlled deployments.Promotion
				controlled, e = deploymentStore.Control(repo.ID, dep.ID, actorID(c), "pause", dep.State, in.Summary)
				actionDeploymentID = controlled.ID
			}
		}
		if in.Action == "restore" {
			var rollback deployments.Promotion
			rollback, _, e = deploymentStore.CreateRollback(repo.ID, dep.ID, actorID(c))
			actionDeploymentID = rollback.ID
		}
		if e != nil {
			_, _, _ = workspaces.UpdateRepairWork(repo.ID, current.ID, work.ID, actorID(c), out.Version, func(x *debugworkspaces.RepairWork) error { x.ActionStatus = "failed"; return nil })
			writeAPIError(w, 409, "debugging_operational_action_failed", e.Error())
			return
		}
		final, completed, finalErr := workspaces.UpdateRepairWork(repo.ID, current.ID, work.ID, actorID(c), out.Version, func(x *debugworkspaces.RepairWork) error {
			x.ActionStatus = "completed"
			x.ActionDeploymentID = actionDeploymentID
			return nil
		})
		if finalErr != nil {
			w.Header().Set("Vivarium-Recovery-Validation", "pending")
			writeJSON(w, 202, map[string]any{"debugging_workspace": project(out, actorID(c)), "repair_work": updated, "operational_action_deployment_id": actionDeploymentID, "reconciliation_pending": true})
			return
		}
		writeJSON(w, 201, map[string]any{"debugging_workspace": project(final, actorID(c)), "repair_work": completed})
	})
}

func debugContains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func debugPassingChecks(runs []checkruns.Run, ids []string, deployedCommit string) map[string]checkruns.Run {
	selected := map[string]checkruns.Run{}
	for _, run := range runs {
		for _, id := range ids {
			if run.ID == id && run.CommitID == deployedCommit && run.State == "completed" && run.ExitCode != nil && *run.ExitCode == 0 {
				selected[id] = run
			}
		}
	}
	return selected
}
func debugMissingRequiredChecks(selected map[string]checkruns.Run, required []string) []string {
	present := map[string]bool{}
	for _, run := range selected {
		present[run.Definition.Name] = true
	}
	missing := []string{}
	for _, name := range required {
		if !present[name] {
			missing = append(missing, name)
		}
	}
	return missing
}
func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}

func canReadReplayWorkspace(workspace devworkspaces.Workspace, actor, repositoryOwner string) bool {
	return workspace.Policy.Sharing != "private" || actor == workspace.CreatorID || actor == repositoryOwner
}

func replayCommandForOutcome(commands []debugworkspaces.ReplayCommand, declared map[string]devworkspaces.ExperimentCommand, outcome devworkspaces.CommandOutcome) (string, bool) {
	for _, command := range commands {
		definition, ok := declared[command.Name]
		if ok && definition.Command == command.SHA256 && outcome.CommandSHA256 == command.SHA256 {
			return command.Name, true
		}
	}
	return "", false
}

func selectReplayOutcome(selected map[string]devworkspaces.CommandOutcome, commandName string, outcome devworkspaces.CommandOutcome) bool {
	if _, exists := selected[commandName]; exists {
		return false
	}
	selected[commandName] = outcome
	return true
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
