package main

import (
	"bytes"
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/datacommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/dataflows"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type dataFlowInput struct {
	ExpectedVersion int                `json:"expected_version"`
	Revision        dataflows.Revision `json:"revision"`
}

func registerDataFlowRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, commitments *datacommitments.Store, flows *dataflows.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return actor, false, false
		}
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return actor, false, false
		}
		repo, err := catalog.GetByID(r.PathValue("id"))
		if err != nil {
			writeRepositoryError(w, err)
			return actor, false, false
		}
		participant := actor.AgentID == "" && actor.UserID == repo.OwnerID
		if !participant && actor.AgentID == "" {
			participant, _ = catalog.HasCollaborator(actor.UserID, repo.ID)
		}
		return actor, participant, true
	}
	mux.HandleFunc("GET /repositories/{id}/data-flows", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		out, err := flows.List(r.PathValue("id"))
		if err != nil {
			writeDataFlow(w, dataflows.Map{}, err, 0)
			return
		}
		includeAnalysis := dataFlowAnalysisVisible(catalog, r.PathValue("id"), actor, authenticated)
		for i := range out {
			out[i] = projectDataFlowForReader(out[i], includeAnalysis)
		}
		writeJSON(w, 200, map[string]any{"data_flows": out})
	})
	mux.HandleFunc("GET /repositories/{id}/data-flows/{flow_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		out, err := flows.Get(r.PathValue("id"), r.PathValue("flow_id"))
		if err == nil {
			out = projectDataFlowForReader(out, dataFlowAnalysisVisible(catalog, r.PathValue("id"), actor, authenticated))
		}
		writeDataFlow(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/data-flows", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "data_flow_forbidden", "only current repository participants may publish data-flow declarations")
			return
		}
		var in dataFlowInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete revision-exact data-flow declaration is required")
			return
		}
		if !accessibilityRevisionIsVisible(git, r.PathValue("id"), in.Revision.CodeRevision) || !dataFlowCommitmentsResolve(commitments, r.PathValue("id"), in.Revision.CommitmentRefs, in.Revision.Edges) {
			writeAPIError(w, 422, "invalid_data_flow_evidence", "the code revision and every data-use commitment reference must resolve in this repository")
			return
		}
		out, err := flows.Create(r.PathValue("id"), actor.UserID, in.Revision)
		writeDataFlow(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/data-flows/{flow_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "data_flow_forbidden", "only current repository participants may revise data-flow declarations")
			return
		}
		var in dataFlowInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete declaration are required")
			return
		}
		if !accessibilityRevisionIsVisible(git, r.PathValue("id"), in.Revision.CodeRevision) || !dataFlowCommitmentsResolve(commitments, r.PathValue("id"), in.Revision.CommitmentRefs, in.Revision.Edges) {
			writeAPIError(w, 422, "invalid_data_flow_evidence", "the code revision and every data-use commitment reference must resolve in this repository")
			return
		}
		out, err := flows.Revise(r.PathValue("id"), r.PathValue("flow_id"), in.ExpectedVersion, actor.UserID, in.Revision)
		writeDataFlow(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/data-flows/{flow_id}/analyses", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant && actor.AgentID == "" {
			writeAPIError(w, 403, "data_flow_analysis_forbidden", "only repository participants and repository-bound read-only agents may add bounded analysis")
			return
		}
		var in dataflows.Analysis
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "bounded cited findings are required")
			return
		}
		actorType, actorID := "human", actor.UserID
		if actor.AgentID != "" {
			actorType, actorID = "agent", actor.AgentID
		}
		if !dataFlowCitationsResolve(git, r.PathValue("id"), in.CodeRevision, in.Findings) {
			writeAPIError(w, 422, "invalid_data_flow_citation", "every finding must cite a file and line range in the analysis code revision")
			return
		}
		out, err := flows.AddAnalysis(r.PathValue("id"), r.PathValue("flow_id"), actorType, actorID, in)
		writeDataFlow(w, out, err, 201)
	})
}

func dataFlowCitationsResolve(git *storage.Store, repositoryID, revision string, findings []dataflows.Finding) bool {
	if git == nil {
		return false
	}
	repository, err := git.Open(repositoryID)
	if err != nil {
		return false
	}
	commit, err := repository.ReadCommit(storage.ObjectID(revision))
	if err != nil {
		return false
	}
	entries, err := repository.WalkTree(commit.Tree)
	if err != nil {
		return false
	}
	files := map[string]storage.TreeEntry{}
	for _, entry := range entries {
		if entry.Type == storage.BlobObject {
			files[entry.Path] = entry.TreeEntry
		}
	}
	for _, finding := range findings {
		for _, citation := range finding.Citations {
			entry, ok := files[citation.Path]
			if !ok {
				return false
			}
			object, readErr := repository.ReadObject(entry.ID)
			if readErr != nil || citation.StartLine < 1 || citation.EndLine < citation.StartLine || citation.EndLine > 1+bytes.Count(object.Content, []byte{'\n'}) {
				return false
			}
		}
	}
	return true
}

func dataFlowCommitmentsResolve(store *datacommitments.Store, repo string, refs []dataflows.CommitmentRef, edges []dataflows.Edge) bool {
	if store == nil {
		return false
	}
	all := append([]dataflows.CommitmentRef(nil), refs...)
	for _, e := range edges {
		all = append(all, e.CommitmentRefs...)
	}
	for _, ref := range all {
		if len(ref.DataUseIDs) == 0 {
			return false
		}
		c, err := store.Get(ref.CommitmentID)
		if err != nil || c.RepositoryID != repo || ref.Version < 1 || ref.Version > len(c.Revisions) {
			return false
		}
		uses := map[string]bool{}
		for _, u := range c.Revisions[ref.Version-1].DataUses {
			uses[u.ID] = true
		}
		for _, id := range ref.DataUseIDs {
			if !uses[id] {
				return false
			}
		}
	}
	return len(all) > 0
}

func dataFlowAnalysisVisible(catalog *repositories.Store, repositoryID string, actor auth.Credential, authenticated bool) bool {
	if !authenticated {
		return false
	}
	if actor.AgentID != "" {
		return actor.RepositoryID == repositoryID
	}
	repository, err := catalog.GetByID(repositoryID)
	if err != nil {
		return false
	}
	if actor.UserID == repository.OwnerID {
		return true
	}
	participant, err := catalog.HasCollaborator(actor.UserID, repositoryID)
	return err == nil && participant
}

func projectDataFlowForReader(value dataflows.Map, includeAnalysis bool) dataflows.Map {
	if includeAnalysis {
		return value
	}
	value.Analyses = []dataflows.Analysis{}
	diagnostics := make([]dataflows.Diagnostic, 0, len(value.Diagnostics))
	for _, diagnostic := range value.Diagnostics {
		switch diagnostic.Kind {
		case "stale_analysis", "undeclared_flow", "declared_observed_difference":
			continue
		default:
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	value.Diagnostics = diagnostics
	return value
}
func writeDataFlow(w http.ResponseWriter, out dataflows.Map, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, dataflows.ErrNotFound):
		writeAPIError(w, 404, "data_flow_not_found", "data-flow map not found")
	case errors.Is(err, dataflows.ErrConflict):
		writeAPIError(w, 409, "data_flow_conflict", "the data-flow declaration changed; reload before publishing")
	case errors.Is(err, dataflows.ErrInvalid):
		writeAPIError(w, 400, "invalid_data_flow", "declare a bounded revision-exact path with resolvable nodes, edges, commitments, and cited findings")
	default:
		log.Printf("data flow storage: %v", err)
		writeAPIError(w, 500, "data_flows_unavailable", "data-flow evidence could not be persisted")
	}
}
