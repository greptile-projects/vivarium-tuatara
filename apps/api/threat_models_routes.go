package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/durableschemas"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/infrastructure"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productexperiments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/threatmodels"
)

type threatModelSources struct {
	designs        *designproposals.Store
	pulls          *pullrequests.Store
	apis           *apicontracts.Store
	schemas        *durableschemas.Store
	infrastructure *infrastructure.Store
	experiments    *productexperiments.Store
}

func (s threatModelSources) current(repo string, revision threatmodels.Revision) (threatmodels.CurrentSource, error) {
	current := threatmodels.CurrentSource{DependencyRevisions: map[string]string{}, PermittedEvidenceIDs: map[string]bool{}}
	source := revision.Source
	var snapshot any
	switch source.Kind {
	case "design_proposal":
		v, e := s.designs.Get(repo, source.ResourceID)
		if e != nil {
			return current, e
		}
		current.Revision = strconv.Itoa(v.CurrentVersion)
		snapshot = v
	case "pull_request":
		v, e := s.pulls.Get(repo, source.ResourceID)
		if e != nil {
			return current, e
		}
		current.Revision = v.SourceCommitID
		snapshot = v
	case "api_evolution":
		v, e := s.apis.Get(source.ResourceID)
		if e != nil || v.RepositoryID != repo {
			return current, threatmodels.ErrNotFound
		}
		current.Revision = strconv.Itoa(v.CurrentVersion)
		snapshot = v
	case "schema_evolution":
		v, e := s.schemas.Get(repo, source.ResourceID)
		if e != nil {
			return current, e
		}
		current.Revision = strconv.Itoa(v.CurrentVersion)
		snapshot = v
	case "infrastructure_plan":
		v, e := s.infrastructure.GetPlan(source.ResourceID)
		if e != nil || v.RepositoryID != repo {
			return current, threatmodels.ErrNotFound
		}
		current.Revision = v.SourceRevision
		snapshot = v
	case "product_experiment":
		v, e := s.experiments.Get(source.ResourceID)
		if e != nil || v.RepositoryID != repo {
			return current, threatmodels.ErrNotFound
		}
		current.Revision = strconv.Itoa(v.CurrentVersion)
		snapshot = v
	default:
		return current, threatmodels.ErrInvalid
	}
	encoded, e := json.Marshal(snapshot)
	if e != nil {
		return current, e
	}
	digest := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(digest[:])
	current.ArchitectureDigest = fingerprint
	current.TrustBoundaryDigest = fingerprint
	for _, dependency := range revision.Dependencies {
		current.DependencyRevisions[dependency.ID] = fingerprint
	}
	// Only evidence that resolves through the same currently authorized source
	// can expose metadata. Other citation kinds retain a gap until a dedicated
	// reader-aware resolver for that governed resource exists.
	for _, evidence := range revision.Evidence {
		if evidence.Accessible && evidence.ResourceID == source.ResourceID && evidence.Revision == source.Revision {
			current.PermittedEvidenceIDs[evidence.ID] = true
		}
	}
	return current, nil
}

type threatModelInput struct {
	ExpectedVersion int                   `json:"expected_version"`
	Revision        threatmodels.Revision `json:"revision"`
}
type threatEventInput struct {
	ExpectedVersion int                `json:"expected_version"`
	Event           threatmodels.Event `json:"event"`
}
type threatAcknowledgementInput struct {
	ModelVersion    int                          `json:"model_version"`
	Acknowledgement threatmodels.Acknowledgement `json:"acknowledgement"`
}

func registerThreatModelRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *threatmodels.Store, sources threatModelSources) {
	readCurrent := func(repo string, revision threatmodels.Revision) threatmodels.CurrentSource {
		v, _ := sources.current(repo, revision)
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/threat-models", func(w http.ResponseWriter, r *http.Request) {
		repo := r.PathValue("id")
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, repo); !ok {
			return
		}
		values, e := store.List(repo, func(revision threatmodels.Revision) (threatmodels.CurrentSource, error) {
			return sources.current(repo, revision)
		})
		if e != nil {
			writeAPIError(w, 500, "threat_models_unavailable", "threat models could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"threat_models": values})
	})
	mux.HandleFunc("GET /repositories/{id}/threat-models/{model_id}", func(w http.ResponseWriter, r *http.Request) {
		repo := r.PathValue("id")
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, repo); !ok {
			return
		}
		raw, e := store.Get(repo, r.PathValue("model_id"), threatmodels.CurrentSource{})
		if e != nil {
			writeThreatModel(w, raw, e, 200)
			return
		}
		writeJSON(w, 200, func() threatmodels.Model {
			revision := raw.Revisions[len(raw.Revisions)-1]
			return func() threatmodels.Model { v, _ := store.Get(repo, raw.ID, readCurrent(repo, revision)); return v }()
		}())
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok || actor.AgentID != "" {
				if ok {
					writeAPIError(w, 403, "threat_model_agent_read_only", "agents may contribute findings but cannot publish the model")
				}
				return
			}
			var in threatModelInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete threat model revision is required")
				return
			}
			current, e := sources.current(r.PathValue("id"), in.Revision)
			if e != nil || current.Revision != in.Revision.Source.Revision {
				writeAPIError(w, 400, "invalid_threat_model_source", "the exact visible source revision is required")
				return
			}
			// Freshness inputs are snapshots of authoritative source state, never
			// caller assertions. Dependencies share that snapshot until their
			// source model exposes a narrower repository-owned revision mapping.
			in.Revision.ArchitectureDigest = current.ArchitectureDigest
			in.Revision.TrustBoundaryDigest = current.TrustBoundaryDigest
			for i := range in.Revision.Dependencies {
				in.Revision.Dependencies[i].Revision = current.DependencyRevisions[in.Revision.Dependencies[i].ID]
			}
			var out threatmodels.Model
			participants := append([]string{actor.UserID}, in.Revision.OwnerIDs...)
			for _, path := range in.Revision.AbusePaths {
				participants = append(participants, path.OwnerIDs...)
			}
			for _, mitigation := range in.Revision.Mitigations {
				participants = append(participants, mitigation.OwnerIDs...)
			}
			e = catalog.WithCurrentParticipants(participants, r.PathValue("id"), func() error {
				if revise {
					out, e = store.Revise(r.PathValue("id"), r.PathValue("model_id"), in.ExpectedVersion, actor.UserID, in.Revision)
				} else {
					out, e = store.Create(r.PathValue("id"), actor.UserID, in.Revision)
				}
				return e
			})
			if e == nil {
				out, _ = store.Get(r.PathValue("id"), out.ID, current)
			}
			status := 201
			if revise {
				status = 200
			}
			writeThreatModel(w, out, e, status)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/threat-models", publish(false))
	mux.HandleFunc("POST /repositories/{id}/threat-models/{model_id}/revisions", publish(true))
	mux.HandleFunc("POST /repositories/{id}/threat-models/{model_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authorizeThreatModelContributor(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		var in threatEventInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a cited threat-model event is required")
			return
		}
		actorID, actorType := actor.UserID, "human"
		if actor.AgentID != "" {
			model, modelErr := store.Get(r.PathValue("id"), r.PathValue("model_id"), threatmodels.CurrentSource{})
			if modelErr != nil {
				writeThreatModel(w, model, modelErr, 200)
				return
			}
			if !sources.agentAuthorized(actor, model.Revisions[len(model.Revisions)-1]) {
				writeAPIError(w, 403, "threat_model_agent_task_mismatch", "agent evidence requires the exact source pull task and branch credential")
				return
			}
			actorID, actorType = actor.AgentID, "agent"
		}
		out, e := store.AddEvent(r.PathValue("id"), r.PathValue("model_id"), in.ExpectedVersion, actorID, actorType, in.Event)
		writeThreatModel(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/threat-models/{model_id}/acknowledgements", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok || actor.AgentID != "" {
			if ok {
				writeAPIError(w, 403, "threat_model_acknowledgement_forbidden", "only a requested human owner may acknowledge")
			}
			return
		}
		var in threatAcknowledgementInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an acknowledgement is required")
			return
		}
		out, e := store.Acknowledge(r.PathValue("id"), r.PathValue("model_id"), in.ModelVersion, actor.UserID, in.Acknowledgement)
		writeThreatModel(w, out, e, 201)
	})
}

func (s threatModelSources) agentAuthorized(actor auth.Credential, revision threatmodels.Revision) bool {
	if revision.Source.Kind != "pull_request" {
		return false
	}
	pull, err := s.pulls.Get(actor.RepositoryID, revision.Source.ResourceID)
	if err != nil {
		return false
	}
	return agentMatchesPullTask(actor, pull, revision)
}

func agentMatchesPullTask(actor auth.Credential, pull pullrequests.PullRequest, revision threatmodels.Revision) bool {
	if pull.SourceCommitID != revision.Source.Revision || pull.TaskID == nil {
		return false
	}
	expectedBranch := "refs/heads/" + pull.SourceBranch
	return actor.GitWriteBranch == expectedBranch && strings.HasPrefix(pull.SourceBranch, "agent/tasks/"+*pull.TaskID+"-")
}
func authorizeThreatModelContributor(w http.ResponseWriter, r *http.Request, catalog *repositories.Store, credentials *auth.Store, repo string) (auth.Credential, bool) {
	actor, authenticated, e := authenticateOptionalCredential(r, credentials, "repositories:write")
	if e == nil && authenticated && actor.AgentID != "" {
		writeAPIError(w, 403, "threat_model_agent_scope_invalid", "agent evidence requires an exact repository-bound task credential")
		return auth.Credential{}, false
	}
	if errors.Is(e, auth.ErrNotFound) {
		actor, authenticated, e = authenticateOptionalCredential(r, credentials, "git:write")
	}
	if e != nil || !authenticated {
		writeAuthenticationRequired(w, false)
		return auth.Credential{}, false
	}
	if actor.AgentID != "" {
		if actor.Kind != auth.Git || actor.RepositoryID != repo || actor.GitWriteBranch == "" {
			writeAPIError(w, 403, "threat_model_agent_scope_invalid", "agent evidence requires an exact repository-bound task credential")
			return auth.Credential{}, false
		}
		return actor, true
	}
	repository, e := catalog.GetByID(repo)
	if e != nil {
		writeRepositoryError(w, e)
		return auth.Credential{}, false
	}
	collaborator, e := catalog.HasCollaborator(actor.UserID, repo)
	if e != nil {
		writeRepositoryError(w, e)
		return auth.Credential{}, false
	}
	if actor.UserID != repository.OwnerID && !collaborator {
		writeAPIError(w, 404, "repository_not_found", "repository not found")
		return auth.Credential{}, false
	}
	return actor, true
}
func writeThreatModel(w http.ResponseWriter, v threatmodels.Model, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, threatmodels.ErrNotFound):
		writeAPIError(w, 404, "threat_model_not_found", "threat model not found")
	case errors.Is(e, threatmodels.ErrConflict):
		writeAPIError(w, 409, "threat_model_conflict", "the threat model changed; reload before contributing")
	case errors.Is(e, threatmodels.ErrInvalid):
		writeAPIError(w, 400, "invalid_threat_model", "the threat model graph, permitted citations, revision, or acknowledgement is invalid")
	case errors.Is(e, repositories.ErrInvalidCollaborator), errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 403, "threat_model_owner_forbidden", "all affected owners must be current repository participants")
	default:
		log.Printf("threat model storage: %v", e)
		writeAPIError(w, 500, "threat_models_unavailable", "threat model could not be persisted")
	}
}
