package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/observabilitygaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/telemetrycontracts"
)

type telemetryContractInput struct {
	RequestID       string                      `json:"request_id"`
	ExpectedVersion int                         `json:"expected_version"`
	Revision        telemetrycontracts.Revision `json:"revision"`
}
type telemetryChallengeInput struct {
	RequestID       string                        `json:"request_id"`
	ContractVersion int                           `json:"contract_version"`
	AlternativeID   string                        `json:"alternative_id"`
	Assumption      string                        `json:"assumption"`
	Rationale       string                        `json:"rationale"`
	Citations       []telemetrycontracts.Citation `json:"citations"`
}

func registerTelemetryContractRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, credentials *auth.Store, gaps *observabilitygaps.Store, contracts *telemetrycontracts.Store) {
	mux.HandleFunc("GET /repositories/{id}/telemetry-contracts", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := contracts.List(r.PathValue("id"))
		if e != nil {
			writeTelemetryContractError(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"telemetry_contracts": v})
	})
	mux.HandleFunc("GET /repositories/{id}/telemetry-contracts/{contract_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := contracts.Get(r.PathValue("id"), r.PathValue("contract_id"))
		if e != nil {
			writeTelemetryContractError(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	publish := func(w http.ResponseWriter, r *http.Request, revise bool) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in telemetryContractInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete telemetry contract revision is required")
			return
		}
		participants := append([]string{actor.UserID}, in.Revision.OwnerIDs...)
		participants = append(participants, in.Revision.ConsumerIDs...)
		var out telemetrycontracts.Contract
		e := repos.WithCurrentParticipants(participants, r.PathValue("id"), func() error {
			return gaps.WithCurrentVersion(r.PathValue("id"), in.Revision.GapID, in.Revision.GapVersion, func() error {
				var x error
				if revise {
					out, x = contracts.Revise(r.PathValue("id"), r.PathValue("contract_id"), in.ExpectedVersion, actor.UserID, in.RequestID, in.Revision)
				} else {
					out, x = contracts.Create(r.PathValue("id"), actor.UserID, in.RequestID, in.Revision)
				}
				return x
			})
		})
		if errors.Is(e, observabilitygaps.ErrConflict) || errors.Is(e, observabilitygaps.ErrNotFound) {
			writeAPIError(w, 422, "telemetry_contract_dependency_changed", "the exact observability gap dependency does not resolve or has changed")
			return
		}
		if e != nil {
			writeTelemetryContractError(w, e)
			return
		}
		if revise {
			writeJSON(w, 200, out)
		} else {
			writeJSON(w, 201, out)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/telemetry-contracts", func(w http.ResponseWriter, r *http.Request) { publish(w, r, false) })
	mux.HandleFunc("POST /repositories/{id}/telemetry-contracts/{contract_id}/revisions", func(w http.ResponseWriter, r *http.Request) { publish(w, r, true) })
	mux.HandleFunc("POST /repositories/{id}/telemetry-contracts/{contract_id}/challenges", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return
		}
		if actor.AgentID != "" && actor.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 403, "telemetry_challenge_forbidden", "read-only agent must be bound to this repository")
			return
		}
		if actor.AgentID == "" {
			repository, _ := repos.GetByID(r.PathValue("id"))
			participant := repository.OwnerID == actor.UserID
			if !participant {
				participant, _ = repos.HasCollaborator(actor.UserID, r.PathValue("id"))
			}
			if !participant {
				writeAPIError(w, 403, "telemetry_challenge_forbidden", "only current collaborators and repository-bound read-only agents may challenge a contract")
				return
			}
		}
		var in telemetryChallengeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a revision-bound cited challenge is required")
			return
		}
		if !telemetryCitationsResolve(git, r.PathValue("id"), in.Citations) {
			writeAPIError(w, 422, "telemetry_citation_invalid", "every challenge citation must resolve to an exact repository blob and its SHA-256 digest")
			return
		}
		for index := range in.Citations {
			in.Citations[index].Verified = true
		}
		kind, id := "human", actor.UserID
		if actor.AgentID != "" {
			kind, id = "agent", actor.AgentID
		}
		out, e := contracts.Challenge(r.PathValue("id"), r.PathValue("contract_id"), in.RequestID, in.ContractVersion, kind, id, in.AlternativeID, in.Assumption, in.Rationale, in.Citations)
		if e != nil {
			writeTelemetryContractError(w, e)
			return
		}
		writeJSON(w, 201, out)
	})
}

func telemetryCitationsResolve(git *storage.Store, repoID string, citations []telemetrycontracts.Citation) bool {
	repository, err := git.Open(repoID)
	if err != nil || len(citations) == 0 {
		return false
	}
	for _, citation := range citations {
		if citation.Kind != "git_blob" || citation.ResourceID == "" {
			return false
		}
		commit, err := repository.ReadCommit(storage.ObjectID(citation.Revision))
		if err != nil {
			return false
		}
		entries, err := repository.WalkTree(commit.Tree)
		if err != nil {
			return false
		}
		matched := false
		for _, entry := range entries {
			if entry.Path != citation.ResourceID || entry.Type != storage.BlobObject {
				continue
			}
			object, err := repository.ReadObject(entry.ID)
			if err != nil {
				return false
			}
			digest := sha256.Sum256(object.Content)
			matched = hex.EncodeToString(digest[:]) == citation.Digest
			break
		}
		if !matched {
			return false
		}
	}
	return true
}

func writeTelemetryContractError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, telemetrycontracts.ErrNotFound):
		writeAPIError(w, 404, "telemetry_contract_not_found", "telemetry contract not found")
	case errors.Is(e, telemetrycontracts.ErrConflict):
		writeAPIError(w, 409, "telemetry_contract_conflict", "the contract changed or request identity was reused")
	case errors.Is(e, telemetrycontracts.ErrInvalid):
		writeAPIError(w, 400, "invalid_telemetry_contract", "complete signals, impacts, ownership, consumers, alternatives, and cited review are required")
	case errors.Is(e, repositories.ErrInvalidCollaborator), errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 403, "telemetry_contract_forbidden", "owners and consumers must be current repository participants")
	default:
		writeAPIError(w, 500, "telemetry_contracts_unavailable", "telemetry contract could not be persisted")
	}
}
