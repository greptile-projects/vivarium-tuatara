package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/observabilitygaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
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

func registerTelemetryContractRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, credentials *auth.Store, gaps *observabilitygaps.Store, contracts *telemetrycontracts.Store, proposalStore *proposals.Store, pulls *pullrequests.Store, previewStore *previews.Store, checkStore *checkruns.Store) {
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
	mux.HandleFunc("POST /repositories/{id}/telemetry-contracts/{contract_id}/acceptance", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			RequestID       string `json:"request_id"`
			ContractVersion int    `json:"contract_version"`
			Rationale       string `json:"rationale"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "exact contract acceptance is required")
			return
		}
		contract, e := contracts.Get(r.PathValue("id"), r.PathValue("contract_id"))
		if e != nil {
			writeTelemetryContractError(w, e)
			return
		}
		rev := contract.Revisions[contract.CurrentVersion-1]
		allowed := false
		for _, id := range rev.OwnerIDs {
			allowed = allowed || id == actor.UserID
		}
		if !allowed {
			writeAPIError(w, 403, "telemetry_acceptance_forbidden", "only a declared current contract owner may accept the exact complete revision")
			return
		}
		var out telemetrycontracts.Contract
		e = repos.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error {
			var x error
			out, x = contracts.Accept(r.PathValue("id"), contract.ID, actor.UserID, in.RequestID, in.Rationale, in.ContractVersion)
			return x
		})
		if e != nil {
			writeTelemetryContractError(w, e)
			return
		}
		writeJSON(w, 201, out)
	})
	type deliveryTask struct {
		Title        string `json:"title"`
		Outcome      string `json:"outcome"`
		AssigneeType string `json:"assignee_type"`
		AssigneeID   string `json:"assignee_id"`
	}
	mux.HandleFunc("POST /repositories/{id}/telemetry-contracts/{contract_id}/deliveries", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			RequestID    string         `json:"request_id"`
			RepositoryID string         `json:"repository_id"`
			Title        string         `json:"title"`
			Body         string         `json:"body"`
			Tasks        []deliveryTask `json:"tasks"`
		}
		if decodeJSON(r, &in) != nil || len(in.Tasks) == 0 {
			writeAPIError(w, 400, "invalid_request", "a target repository and owned instrumentation tasks are required")
			return
		}
		contract, e := contracts.Get(r.PathValue("id"), r.PathValue("contract_id"))
		if e != nil || contract.Acceptance == nil || contract.Acceptance.Version != contract.CurrentVersion {
			writeAPIError(w, 422, "telemetry_contract_not_accepted", "delivery requires the accepted exact current contract revision")
			return
		}
		target, e := repos.Get(actor.UserID, in.RepositoryID)
		if e != nil {
			writeAPIError(w, 403, "telemetry_delivery_forbidden", "the contributor must have current access to the target repository")
			return
		}
		bare, e := git.Open(target.ID)
		if e != nil {
			writeAPIError(w, 503, "repository_unavailable", "target repository is unavailable")
			return
		}
		b, e := exec.Command("git", "--git-dir="+bare.Path(), "rev-parse", "refs/heads/"+target.DefaultBranch).Output()
		base := strings.TrimSpace(string(b))
		if e != nil || len(base) != 40 {
			writeAPIError(w, 409, "telemetry_delivery_base_unavailable", "target default branch has no exact base")
			return
		}
		recovering := false
		for _, retained := range contract.Deliveries {
			if retained.RequestID == in.RequestID {
				base = retained.BaseRevision
				recovering = true
				break
			}
		}
		if recovering && exec.Command("git", "--git-dir="+bare.Path(), "cat-file", "-e", base+"^{commit}").Run() != nil {
			writeAPIError(w, 409, "telemetry_delivery_base_unavailable", "the retained exact delivery base is unavailable")
			return
		}
		tasks := make([]proposals.ImplementationTaskInput, len(in.Tasks))
		items := make([]proposals.ReasoningItem, len(in.Tasks))
		humans := []string{actor.UserID}
		for i, t := range in.Tasks {
			tasks[i] = proposals.ImplementationTaskInput{Title: t.Title, Outcome: t.Outcome, Risk: "Telemetry privacy, security, access, cost, and behavior boundaries must remain intact.", VerificationPlan: "Use repository-defined checks and an isolated bounded preview to prove emission, schema, units, correlation, sampling, redaction, access, overhead, and failure behavior.", AssigneeType: t.AssigneeType, AssigneeID: t.AssigneeID}
			items[i] = proposals.ReasoningItem{ID: fmt.Sprintf("signal-work-%d", i+1), Kind: "telemetry_implementation", Summary: t.Outcome, Status: "accepted"}
			if t.AssigneeType == "human" {
				humans = append(humans, t.AssigneeID)
			}
		}
		origin := proposals.ReasoningOrigin{TelemetryContractID: contract.ID, TelemetryContractVersion: contract.CurrentVersion, Revision: base, Items: items, AnalysisStatus: "accepted_telemetry_contract"}
		for _, x := range items {
			origin.SelectedItemIDs = append(origin.SelectedItemIDs, x.ID)
		}
		proposalID, taskIDs := telemetrycontracts.DeliveryIdentities(contract.ID, target.ID, contract.CurrentVersion, len(tasks))
		for i := range tasks {
			tasks[i].ID = taskIDs[i]
		}
		var p proposals.Proposal
		var made []proposals.Task
		participants := map[string][]string{r.PathValue("id"): {actor.UserID}}
		participants[target.ID] = humans
		delivery := telemetrycontracts.Delivery{RequestID: in.RequestID, ContractVersion: contract.CurrentVersion, RepositoryID: target.ID, ProposalID: proposalID, TaskIDs: taskIDs, BaseRevision: base, CreatedBy: actor.UserID}
		publish := func() error {
			if _, x := contracts.ReserveDelivery(r.PathValue("id"), contract.ID, delivery); x != nil {
				return x
			}
			var x error
			p, made, x = proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: target.ID, ActorID: actor.UserID, ProposalID: proposalID, Title: in.Title, Body: in.Body, Origin: origin, Tasks: tasks})
			if x != nil {
				return x
			}
			_, x = contracts.FinalizeDelivery(r.PathValue("id"), contract.ID, in.RequestID)
			return x
		}
		e = repos.WithCurrentParticipantsAcross(participants, func() error {
			if recovering {
				return publish()
			}
			return bare.WithReferenceTarget("refs/heads/"+target.DefaultBranch, base, publish)
		})
		if e != nil {
			writeAPIError(w, 422, "telemetry_delivery_invalid", "assignees and ordinary target-repository authority must remain current")
			return
		}
		writeJSON(w, 201, map[string]any{"proposal": p, "tasks": made, "contract_id": contract.ID, "contract_version": contract.CurrentVersion})
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/telemetry-verifications", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		xs, e := contracts.Verifications(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeTelemetryContractError(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"telemetry_verifications": xs})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/telemetry-verifications", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return
		}
		if actor.UserID != "" {
			repository, repositoryErr := repos.GetByID(r.PathValue("id"))
			participant := repositoryErr == nil && repository.OwnerID == actor.UserID
			if !participant {
				participant, _ = repos.HasCollaborator(actor.UserID, r.PathValue("id"))
			}
			if !participant {
				writeAPIError(w, 403, "telemetry_verification_forbidden", "only current target-repository participants and repository-bound agents may publish verification evidence")
				return
			}
		}
		var in telemetrycontracts.Verification
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "complete sanitized telemetry verification metadata is required")
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil || p.SourceCommitID != in.Revision {
			writeAPIError(w, 409, "telemetry_verification_revision_changed", "evidence must bind the pull's exact current revision")
			return
		}
		preview, e := previewStore.Get(p.RepositoryID, p.ID, in.PreviewID)
		if e != nil || preview.Revision != in.Revision || preview.Definition.Access.Network != "none" {
			writeAPIError(w, 422, "invalid_telemetry_preview", "telemetry verification requires an exact-revision network-none bounded preview")
			return
		}
		for _, checkID := range in.CheckRunIDs {
			run, checkErr := checkStore.Get(p.RepositoryID, p.ID, checkID)
			if checkErr != nil || run.CommitID != in.Revision || run.State != "succeeded" {
				writeAPIError(w, 422, "invalid_telemetry_check", "every named repository-defined check must have succeeded on the exact pull revision")
				return
			}
		}
		if _, _, readable := authorizeRepositoryRead(w, r, repos, credentials, in.ContractRepositoryID); !readable {
			return
		}
		contract, e := contracts.Get(in.ContractRepositoryID, in.ContractID)
		eligible := e == nil && contract.Acceptance != nil && contract.Acceptance.Version == in.ContractVersion
		linked := false
		for _, delivery := range contract.Deliveries {
			if eligible && delivery.Status == "created" && delivery.RepositoryID == p.RepositoryID && p.ProposalID != nil && delivery.ProposalID == *p.ProposalID {
				linked = true
				break
			}
		}
		if !linked {
			writeAPIError(w, 422, "telemetry_verification_unlinked", "the pull must be an ordinary contribution from the accepted exact contract delivery")
			return
		}
		in.PullRequestID = p.ID
		kind, id := "human", actor.UserID
		if actor.AgentID != "" {
			if actor.RepositoryID != p.RepositoryID {
				writeAPIError(w, 403, "telemetry_verification_forbidden", "agent must hold a live grant for this exact repository")
				return
			}
			kind, id = "agent", actor.AgentID
		}
		var out telemetrycontracts.Verification
		persist := func() error {
			return pulls.WithSourceRevision(p.RepositoryID, p.ID, in.Revision, func(pullrequests.PullRequest) error {
				var x error
				out, x = contracts.AddVerification(p.RepositoryID, kind, id, in)
				return x
			})
		}
		if actor.UserID != "" {
			e = repos.WithCurrentParticipant(actor.UserID, p.RepositoryID, persist)
		} else {
			e = persist()
		}
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
