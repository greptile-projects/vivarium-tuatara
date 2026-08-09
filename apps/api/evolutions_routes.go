package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerEvolutionRoutes(mux *http.ServeMux, repos *repositories.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, relationStore *relationships.Store, credentials *auth.Store) {
	canRead := func(actorID, id string) bool {
		repo, e := repos.GetByID(id)
		if e != nil {
			return false
		}
		if repo.Visibility == repositories.Public {
			return true
		}
		ok, _ := repos.HasCollaborator(actorID, id)
		return repo.OwnerID == actorID || ok
	}
	visible := func(v relationships.Evolution, actorID string) relationships.Evolution {
		impacts := v.Impacts[:0]
		for _, impact := range v.Impacts {
			if canRead(actorID, impact.RepositoryID) {
				impacts = append(impacts, impact)
			}
		}
		v.Impacts = impacts
		allowed := map[string]bool{v.RepositoryID: true}
		for _, impact := range impacts {
			allowed[impact.RepositoryID] = true
		}
		findings := v.Findings[:0]
		for _, finding := range v.Findings {
			keep := true
			for _, id := range finding.RepositoryIDs {
				keep = keep && allowed[id]
			}
			if keep {
				findings = append(findings, finding)
			}
		}
		v.Findings = findings
		analyses := v.Analyses[:0]
		for _, analysis := range v.Analyses {
			keep := true
			for _, id := range analysis.RepositoryIDs {
				keep = keep && allowed[id]
			}
			if keep {
				analysis.StoredCredentialID = ""
				analyses = append(analyses, analysis)
			}
		}
		v.Analyses = analyses
		return v
	}
	analysisPacket := func(v relationships.Evolution, a relationships.EvolutionAnalysis) relationships.Evolution {
		selected := map[string]bool{v.RepositoryID: true}
		for _, id := range a.RepositoryIDs {
			selected[id] = true
		}
		impacts := v.Impacts[:0]
		for _, impact := range v.Impacts {
			if selected[impact.RepositoryID] {
				impacts = append(impacts, impact)
			}
		}
		v.Impacts = impacts
		findings := v.Findings[:0]
		for _, finding := range v.Findings {
			keep := true
			for _, id := range finding.RepositoryIDs {
				keep = keep && selected[id]
			}
			if keep {
				findings = append(findings, finding)
			}
		}
		v.Findings = findings
		v.Analyses = []relationships.EvolutionAnalysis{a}
		v.Analyses[0].StoredCredentialID = ""
		return v
	}
	readActor := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return actor, false
		}
		if !authenticated {
			optional, present, authOK := authenticateOptionalRequest(w, r, credentials, "repositories:read", false)
			if !authOK {
				return actor, false
			}
			if present {
				actor = optional
			}
		}
		return actor, true
	}
	mux.HandleFunc("GET /repositories/{id}/evolutions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := readActor(w, r)
		if !ok {
			return
		}
		items, e := relationStore.ListEvolutions(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "evolution_read_failed", "evolution plans could not be read")
			return
		}
		for i := range items {
			items[i] = visible(items[i], actor.UserID)
		}
		writeJSON(w, 200, map[string]any{"evolutions": items})
	})
	mux.HandleFunc("GET /repositories/{id}/evolutions/{evolution_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := readActor(w, r)
		if !ok {
			return
		}
		v, e := relationStore.GetEvolution(r.PathValue("id"), r.PathValue("evolution_id"))
		if e != nil {
			writeAPIError(w, 404, "evolution_not_found", "evolution plan not found")
			return
		}
		writeJSON(w, 200, visible(v, actor.UserID))
	})
	mux.HandleFunc("POST /repositories/{id}/evolutions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			InterfaceName          string                              `json:"interface_name"`
			PredecessorInterfaceID string                              `json:"predecessor_interface_id"`
			SourceKind             string                              `json:"source_kind"`
			SourceID               string                              `json:"source_id"`
			CandidateDescription   string                              `json:"candidate_description"`
			Changes                []relationships.CompatibilityChange `json:"changes"`
			Strategy               string                              `json:"strategy"`
			Sequencing             string                              `json:"sequencing"`
			Exceptions             string                              `json:"exceptions"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		var predecessor relationships.Interface
		interfaces, e := relationStore.ListInterfaces(r.PathValue("id"))
		if e == nil {
			for _, x := range interfaces {
				if x.ID == in.PredecessorInterfaceID && x.Name == strings.TrimSpace(in.InterfaceName) {
					predecessor = x
				}
			}
		}
		if predecessor.ID == "" {
			writeAPIError(w, 422, "invalid_predecessor", "predecessor must name a published interface in this repository")
			return
		}
		candidateCommit := ""
		switch in.SourceKind {
		case "proposal":
			if proposalStore == nil {
				e = errors.New("unavailable")
			} else {
				p, x := proposalStore.Get(r.PathValue("id"), in.SourceID)
				e = x
				if x == nil && p.Status != "open" {
					e = errors.New("closed")
				}
			}
		case "pull_request":
			if pullStore == nil {
				e = errors.New("unavailable")
			} else {
				p, x := pullStore.Get(r.PathValue("id"), in.SourceID)
				e = x
				if x == nil && p.Status != "open" {
					e = errors.New("closed")
				}
				candidateCommit = p.SourceCommitID
			}
		default:
			e = errors.New("kind")
		}
		if e != nil {
			writeAPIError(w, 422, "invalid_evolution_source", "source must name an open provider proposal or pull request")
			return
		}
		impacts := []relationships.ConsumerImpact{}
		ids, e := relationStore.ListRepositoryIDs()
		if e != nil {
			writeAPIError(w, 500, "evolution_create_failed", "relationship evidence could not be read")
			return
		}
		for _, id := range ids {
			if !canRead(actor.UserID, id) {
				continue
			}
			ds, x := relationStore.ListDependencies(id)
			if x != nil {
				writeAPIError(w, 500, "evolution_create_failed", "relationship evidence could not be read")
				return
			}
			repo, _ := repos.GetByID(id)
			for _, d := range ds {
				if d.ProviderRepositoryID == r.PathValue("id") && d.InterfaceName == predecessor.Name {
					impacts = append(impacts, relationships.ConsumerImpact{RepositoryID: id, OwnerID: repo.OwnerID, DependencyID: d.ID, CommitID: d.CommitID, Constraint: d.Constraint, State: "affected"})
				}
			}
		}
		v, e := relationStore.CreateEvolution(relationships.Evolution{RepositoryID: r.PathValue("id"), InterfaceName: in.InterfaceName, Predecessor: predecessor, SourceKind: in.SourceKind, SourceID: in.SourceID, CandidateCommitID: candidateCommit, CandidateDescription: in.CandidateDescription, Changes: in.Changes, Impacts: impacts, Strategy: in.Strategy, Sequencing: in.Sequencing, Exceptions: in.Exceptions, CreatedBy: actor.UserID})
		if e != nil {
			writeAPIError(w, 422, "invalid_evolution", "evolution comparison and migration contract are required")
			return
		}
		w.Header().Set("Location", r.URL.Path+"/"+v.ID)
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("PATCH /repositories/{id}/evolutions/{evolution_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write"); !ok {
			return
		}
		var in struct {
			Version    int    `json:"version"`
			Strategy   string `json:"strategy"`
			Sequencing string `json:"sequencing"`
			Exceptions string `json:"exceptions"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, e := relationStore.UpdateEvolution(r.PathValue("id"), r.PathValue("evolution_id"), "", in.Version, in.Strategy, in.Sequencing, in.Exceptions)
		if errors.Is(e, relationships.ErrConflict) {
			writeAPIError(w, 409, "evolution_changed", "evolution plan changed; reload before editing")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "invalid_evolution", "migration contract is invalid")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/evolutions/{evolution_id}/acknowledgements", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := readActor(w, r)
		if !ok {
			return
		}
		var in struct {
			RepositoryID string `json:"repository_id"`
			Note         string `json:"note"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		participant, _ := repos.HasCollaborator(actor.UserID, in.RepositoryID)
		repo, e := repos.GetByID(in.RepositoryID)
		if e != nil || (repo.OwnerID != actor.UserID && !participant) {
			writeAPIError(w, 403, "acknowledgement_forbidden", "only a current consumer participant may acknowledge impact")
			return
		}
		v, e := relationStore.GetEvolution(r.PathValue("id"), r.PathValue("evolution_id"))
		affected := false
		for _, x := range v.Impacts {
			affected = affected || x.RepositoryID == in.RepositoryID
		}
		if e != nil || !affected {
			writeAPIError(w, 422, "consumer_not_affected", "repository is not in this plan's impact snapshot")
			return
		}
		v, e = relationStore.AcknowledgeEvolution(v.RepositoryID, v.ID, actor.UserID, in.RepositoryID, in.Note)
		if e != nil {
			writeAPIError(w, 409, "acknowledgement_exists", "this participant already acknowledged the consumer impact")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/evolutions/{evolution_id}/analyses", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Mandate       string   `json:"mandate"`
			RepositoryIDs []string `json:"repository_ids"`
			ExpiresIn     int64    `json:"expires_in"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if in.ExpiresIn == 0 {
			in.ExpiresIn = 3600
		}
		if in.ExpiresIn < 300 || in.ExpiresIn > 86400 {
			writeAPIError(w, 422, "invalid_analysis", "expiry must be between 5 minutes and 24 hours")
			return
		}
		v, e := relationStore.GetEvolution(r.PathValue("id"), r.PathValue("evolution_id"))
		if e != nil {
			writeAPIError(w, 404, "evolution_not_found", "evolution plan not found")
			return
		}
		for _, id := range in.RepositoryIDs {
			selected := id == v.RepositoryID
			for _, x := range v.Impacts {
				selected = selected || x.RepositoryID == id
			}
			if !selected || !canRead(actor.UserID, id) {
				writeAPIError(w, 422, "invalid_analysis_repository", "selected repositories must be readable members of the impact snapshot")
				return
			}
		}
		issued, e := credentials.Issue(actor.UserID, auth.API, "Evolution analysis", []string{"evolutions:analyze"}, time.Duration(in.ExpiresIn)*time.Second)
		if e != nil {
			writeAPIError(w, 500, "analysis_start_failed", "read-only analysis access could not be issued")
			return
		}
		v, a, e := relationStore.StartEvolutionAnalysis(v.RepositoryID, v.ID, actor.UserID, issued.ID, in.Mandate, in.RepositoryIDs)
		if e != nil {
			_, _ = credentials.Revoke(actor.UserID, issued.ID)
			writeAPIError(w, 422, "invalid_analysis", "analysis mandate and repositories are required")
			return
		}
		a.StoredCredentialID = ""
		writeJSON(w, 201, map[string]any{"evolution": visible(v, actor.UserID), "analysis": a, "credential": issued})
	})
	mux.HandleFunc("GET /repositories/{id}/evolutions/{evolution_id}/analyses/{analysis_id}", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "evolutions:analyze", false)
		if !ok {
			return
		}
		v, a, e := relationStore.EvolutionAnalysis(r.PathValue("id"), r.PathValue("evolution_id"), r.PathValue("analysis_id"), credential.ID)
		if e != nil {
			writeAPIError(w, 404, "analysis_not_found", "analysis not found")
			return
		}
		packet := analysisPacket(v, a)
		a.StoredCredentialID = ""
		writeJSON(w, 200, map[string]any{"plan": packet, "analysis": a})
	})
	mux.HandleFunc("POST /repositories/{id}/evolutions/{evolution_id}/analyses/{analysis_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "evolutions:analyze", false)
		if !ok {
			return
		}
		v, a, e := relationStore.EvolutionAnalysis(r.PathValue("id"), r.PathValue("evolution_id"), r.PathValue("analysis_id"), credential.ID)
		if e != nil {
			writeAPIError(w, 404, "analysis_not_found", "analysis not found")
			return
		}
		var in struct {
			RepositoryIDs []string `json:"repository_ids"`
			Finding       string   `json:"finding"`
			Uncertainty   string   `json:"uncertainty"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		for _, id := range in.RepositoryIDs {
			allowed := false
			for _, x := range a.RepositoryIDs {
				allowed = allowed || id == x
			}
			if !allowed {
				writeAPIError(w, 422, "finding_out_of_scope", "findings may cite only selected repositories")
				return
			}
		}
		v, e = relationStore.AddEvolutionFinding(v.RepositoryID, v.ID, a.AgentID, in.RepositoryIDs, in.Finding, in.Uncertainty)
		if e != nil {
			writeAPIError(w, 422, "invalid_finding", "finding and selected repositories are required")
			return
		}
		writeJSON(w, 201, analysisPacket(v, a))
	})
}
