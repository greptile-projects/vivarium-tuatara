package main

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerAPIContractMigrationRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, contracts *apicontracts.Store, relations *relationships.Store) {
	base := "/repositories/{id}/api-contracts/{contract_id}/migrations"
	project := func(v apicontracts.ContractMigration, actor string) apicontracts.ContractMigration {
		if !isMigrationProducer(v, actor, catalog) {
			apps := v.Applications[:0]
			for _, x := range v.Applications {
				if canActForMigrationConsumer(x, actor, catalog) {
					apps = append(apps, x)
				}
			}
			v.Applications = apps
			allowed := func(id string) bool {
				return slices.ContainsFunc(apps, func(x apicontracts.MigrationApplication) bool { return x.ApplicationID == id })
			}
			v.Acknowledgements = slices.DeleteFunc(v.Acknowledgements, func(x apicontracts.MigrationAcknowledgement) bool { return !allowed(x.ApplicationID) })
			v.Attestations = slices.DeleteFunc(v.Attestations, func(x apicontracts.MigrationAttestation) bool { return !allowed(x.ApplicationID) })
			v.Exceptions = slices.DeleteFunc(v.Exceptions, func(x apicontracts.MigrationException) bool { return !allowed(x.ApplicationID) })
		}
		apps := map[string]apicontracts.Application{}
		work := map[string][]apicontracts.IntegrationWork{}
		observations := map[string][]apicontracts.OperationalObservation{}
		for _, x := range v.Applications {
			if app, e := contracts.GetApplication(x.ApplicationID); e == nil {
				apps[x.ApplicationID] = app
			}
			work[x.ApplicationID], _ = contracts.ListIntegrationWork(x.ApplicationID)
			observations[x.ApplicationID], _ = contracts.ListOperationalObservations(x.ApplicationID)
		}
		return apicontracts.ProjectContractMigration(v, apps, work, observations, time.Now().UTC())
	}
	load := func(w http.ResponseWriter, r *http.Request) (apicontracts.ContractMigration, bool) {
		v, e := contracts.GetContractMigration(r.PathValue("migration_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") || v.ContractID != r.PathValue("contract_id") {
			writeAPIError(w, 404, "api_contract_migration_not_found", "API contract migration not found")
			return v, false
		}
		return v, true
	}
	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, e := contracts.ListContractMigrations(r.PathValue("id"), r.PathValue("contract_id"))
		if e != nil {
			writeAPIError(w, 500, "api_contract_migrations_unavailable", "API contract migrations could not be read")
			return
		}
		visible := values[:0]
		for i := range values {
			if migrationVisibleTo(values[i], actor.UserID, catalog) {
				visible = append(visible, project(values[i], actor.UserID))
			}
		}
		writeJSON(w, 200, map[string]any{"migrations": visible})
	})
	mux.HandleFunc("GET "+base+"/{migration_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, ok := load(w, r)
		if !ok {
			return
		}
		if !migrationVisibleTo(v, actor.UserID, catalog) {
			writeAPIError(w, 403, "api_contract_migration_forbidden", "Migration evidence is limited to the producer and affected consumers")
			return
		}
		writeJSON(w, 200, project(v, actor.UserID))
	})
	mux.HandleFunc("POST "+base, func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Kind        string                         `json:"kind"`
			FromVersion int                            `json:"from_version"`
			ToVersion   int                            `json:"to_version"`
			EvolutionID string                         `json:"evolution_id"`
			Changes     []apicontracts.MigrationChange `json:"changes"`
			Stages      []apicontracts.MigrationStage  `json:"stages"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "A migration policy is required")
			return
		}
		contract, e := contracts.Get(r.PathValue("contract_id"))
		if e != nil || contract.RepositoryID != r.PathValue("id") || in.FromVersion < 1 || in.FromVersion > contract.CurrentVersion || in.ToVersion > contract.CurrentVersion {
			writeAPIError(w, 422, "invalid_contract_migration_versions", "Migration versions must name published revisions of this contract")
			return
		}
		if evolution, e := relations.GetEvolution(r.PathValue("id"), in.EvolutionID); e != nil || evolution.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 422, "invalid_contract_migration_evolution", "Migration must name an existing provider evolution plan")
			return
		}
		applications, e := contracts.ListApplications(r.PathValue("id"), contract.ID)
		if e != nil {
			writeAPIError(w, 500, "applications_unavailable", "Consumer applications could not be read completely; migration was not created")
			return
		}
		linked := []apicontracts.MigrationApplication{}
		for _, app := range applications {
			if app.ContractVersion != in.FromVersion {
				continue
			}
			entry := apicontracts.MigrationApplication{ApplicationID: app.ID, OwnerID: app.OwnerID}
			work, workErr := contracts.ListIntegrationWork(app.ID)
			if workErr != nil {
				writeAPIError(w, 500, "integration_work_unavailable", "Consumer integration work could not be read completely; migration was not created")
				return
			}
			if len(work) > 0 {
				latest := work[0]
				for _, candidate := range work[1:] {
					if candidate.CreatedAt.After(latest.CreatedAt) || (candidate.CreatedAt.Equal(latest.CreatedAt) && candidate.ID > latest.ID) {
						latest = candidate
					}
				}
				entry.ConsumerRepositoryID, entry.IntegrationWorkID = latest.ConsumerRepositoryID, latest.ID
			}
			linked = append(linked, entry)
		}
		v, e := contracts.CreateContractMigration(apicontracts.ContractMigration{RepositoryID: r.PathValue("id"), ContractID: contract.ID, FromVersion: in.FromVersion, ToVersion: in.ToVersion, Kind: in.Kind, EvolutionID: in.EvolutionID, Changes: in.Changes, Stages: in.Stages, Applications: linked, CreatedBy: actor.UserID})
		writeMigration(w, project(v, actor.UserID), e, 201)
	})
	mux.HandleFunc("POST "+base+"/{migration_id}/consumer-actions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		v, ok := load(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion   int       `json:"expected_version"`
			ApplicationID     string    `json:"application_id"`
			Action            string    `json:"action"`
			Note              string    `json:"note"`
			IntegrationWorkID string    `json:"integration_work_id"`
			CandidateID       string    `json:"candidate_id"`
			Reason            string    `json:"reason"`
			ExpiresAt         time.Time `json:"expires_at"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "A consumer migration action is required")
			return
		}
		linked := slices.IndexFunc(v.Applications, func(x apicontracts.MigrationApplication) bool { return x.ApplicationID == in.ApplicationID })
		if linked < 0 || !canActForMigrationConsumer(v.Applications[linked], actor.UserID, catalog) {
			writeAPIError(w, 403, "api_contract_migration_forbidden", "Only the application owner or current consumer repository participants may respond")
			return
		}
		if in.Action == "attest" {
			if v.Applications[linked].IntegrationWorkID == "" || in.IntegrationWorkID != v.Applications[linked].IntegrationWorkID {
				writeAPIError(w, 422, "invalid_api_contract_migration", "Attestation must use the exact integration work frozen for this migration")
				return
			}
			work, e := contracts.GetIntegrationWork(in.IntegrationWorkID)
			if e != nil || work.ApplicationID != in.ApplicationID || !slices.ContainsFunc(work.Candidates, func(c apicontracts.IntegrationCandidate) bool { return c.ID == in.CandidateID }) {
				writeAPIError(w, 422, "invalid_api_contract_migration", "Attestation must name an exact candidate from this application's integration work")
				return
			}
		}
		out, e := contracts.MutateContractMigration(v.ID, in.ExpectedVersion, func(x *apicontracts.ContractMigration) error {
			now := time.Now().UTC()
			if x.State == "retired" {
				return apicontracts.ErrMigrationBlocked
			}
			switch in.Action {
			case "acknowledge":
				if strings.TrimSpace(in.Note) == "" {
					return apicontracts.ErrInvalid
				}
				x.Acknowledgements = append(x.Acknowledgements, apicontracts.MigrationAcknowledgement{ApplicationID: in.ApplicationID, ActorID: actor.UserID, Note: strings.TrimSpace(in.Note), CreatedAt: now})
			case "attest":
				x.Attestations = append(x.Attestations, apicontracts.MigrationAttestation{ApplicationID: in.ApplicationID, ActorID: actor.UserID, IntegrationWorkID: in.IntegrationWorkID, CandidateID: in.CandidateID, CreatedAt: now})
			case "exception":
				if strings.TrimSpace(in.Reason) == "" || !in.ExpiresAt.After(now) || in.ExpiresAt.After(now.Add(90*24*time.Hour)) {
					return apicontracts.ErrInvalid
				}
				x.Exceptions = append(x.Exceptions, apicontracts.MigrationException{ApplicationID: in.ApplicationID, ActorID: actor.UserID, Reason: strings.TrimSpace(in.Reason), ExpiresAt: in.ExpiresAt, CreatedAt: now})
			default:
				return apicontracts.ErrInvalid
			}
			return nil
		})
		writeMigration(w, project(out, actor.UserID), e, 200)
	})
	mux.HandleFunc("POST "+base+"/{migration_id}/stages", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		v, ok := load(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			StageID         string `json:"stage_id"`
			Retire          bool   `json:"retire"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "A migration stage decision is required")
			return
		}
		prospective := v
		prospective.CurrentStage = in.StageID
		projected := project(prospective, actor.UserID)
		activeException := slices.ContainsFunc(v.Exceptions, func(x apicontracts.MigrationException) bool { return x.ExpiresAt.After(time.Now().UTC()) })
		if in.Retire && (!projected.Readiness.Ready || activeException || v.State == "retired") {
			writeAPIError(w, 409, "api_contract_retirement_blocked", "Acknowledgement, passing migration evidence, live access, and remaining traffic policy block retirement")
			return
		}
		if !slices.ContainsFunc(v.Stages, func(x apicontracts.MigrationStage) bool { return x.ID == in.StageID }) {
			writeAPIError(w, 422, "invalid_migration_stage", "Select a defined migration stage")
			return
		}
		out, e := contracts.MutateContractMigration(v.ID, in.ExpectedVersion, func(x *apicontracts.ContractMigration) error {
			x.CurrentStage = in.StageID
			if in.Retire {
				x.State = "retired"
			} else {
				x.State = "active"
			}
			return nil
		})
		writeMigration(w, project(out, actor.UserID), e, 200)
	})
}

func migrationVisibleTo(v apicontracts.ContractMigration, actor string, catalog *repositories.Store) bool {
	if isMigrationProducer(v, actor, catalog) {
		return true
	}
	for _, x := range v.Applications {
		if canActForMigrationConsumer(x, actor, catalog) {
			return true
		}
	}
	return false
}

func isMigrationProducer(v apicontracts.ContractMigration, actor string, catalog *repositories.Store) bool {
	if actor == "" {
		return false
	}
	if ok, _ := catalog.HasCollaborator(actor, v.RepositoryID); ok {
		return true
	}
	repo, e := catalog.GetByID(v.RepositoryID)
	return e == nil && repo.OwnerID == actor
}
func canActForMigrationConsumer(x apicontracts.MigrationApplication, actor string, catalog *repositories.Store) bool {
	if x.OwnerID == actor {
		return true
	}
	if x.ConsumerRepositoryID == "" {
		return false
	}
	repo, e := catalog.GetByID(x.ConsumerRepositoryID)
	if e == nil && repo.OwnerID == actor {
		return true
	}
	ok, _ := catalog.HasCollaborator(actor, x.ConsumerRepositoryID)
	return ok
}
func writeMigration(w http.ResponseWriter, v apicontracts.ContractMigration, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, apicontracts.ErrConflict):
		writeAPIError(w, 409, "api_contract_migration_changed", "Migration changed; reload before acting")
	case errors.Is(e, apicontracts.ErrInvalid):
		writeAPIError(w, 422, "invalid_api_contract_migration", "Migration policy or evidence is invalid")
	case errors.Is(e, apicontracts.ErrMigrationBlocked):
		writeAPIError(w, 409, "api_contract_migration_closed", "Retired migrations reject further consumer actions")
	default:
		writeAPIError(w, 500, "api_contract_migration_unavailable", "Migration could not be persisted")
	}
}
