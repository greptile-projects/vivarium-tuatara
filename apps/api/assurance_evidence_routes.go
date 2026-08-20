package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/governance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/privacyreviews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/protectionplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoveryexercises"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type assuranceEvidenceSources struct {
	pulls        *pullrequests.Store
	checks       *checkruns.Store
	releases     *releases.Store
	deployments  *deployments.Store
	incidents    *incidents.Store
	privacy      *privacyreviews.Store
	continuity   *recoveryexercises.Store
	access       *protectionplans.Store
	dependencies *packages.Store
	governance   *governance.Store
}

func registerAssuranceEvidenceRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, programs *assuranceprograms.Store, evidence *assuranceevidence.Store, sources ...assuranceEvidenceSources) {
	trusted := assuranceEvidenceSources{}
	if len(sources) > 0 {
		trusted = sources[0]
	}
	mux.HandleFunc("GET /repositories/{id}/assurance-evidence", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		definitions, e := evidence.ListDefinitions(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "assurance_evidence_unavailable", "assurance evidence could not be read")
			return
		}
		packages, e := evidence.ListPackages(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "assurance_evidence_unavailable", "assurance evidence could not be read")
			return
		}
		visible := map[string]bool{}
		filteredDefinitions := definitions[:0]
		for _, definition := range definitions {
			if evidenceAudienceAllows(definition.Audience, actor.UserID, authenticated) {
				visible[definition.ID] = true
				filteredDefinitions = append(filteredDefinitions, definition)
			}
		}
		definitions = filteredDefinitions
		filteredPackages := packages[:0]
		for _, evidencePackage := range packages {
			if visible[evidencePackage.DefinitionID] {
				filteredPackages = append(filteredPackages, evidencePackage)
			}
		}
		packages = filteredPackages
		writeJSON(w, 200, map[string]any{"definitions": definitions, "packages": packages})
	})
	mux.HandleFunc("POST /repositories/{id}/assurance-evidence/definitions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in assuranceevidence.Definition
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete evidence definition is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		in.OwnerID = actor.UserID
		program, e := programs.Get(in.ProgramID)
		if e != nil || program.RepositoryID != in.RepositoryID || in.ProgramVersion < 1 || in.ProgramVersion > len(program.Revisions) {
			writeAPIError(w, 400, "invalid_control_version", "the evidence definition must bind an exact assurance program revision")
			return
		}
		revision := program.Revisions[in.ProgramVersion-1]
		found := false
		owner := false
		for _, c := range revision.Controls {
			if c.ID == in.ControlID {
				found = true
				for _, id := range c.OwnerIDs {
					if id == actor.UserID {
						owner = true
					}
				}
			}
		}
		if !found || !owner {
			writeAPIError(w, 403, "control_owner_required", "only a named owner of the exact control version may define its evidence")
			return
		}
		for _, id := range in.Audience {
			if id == "repository" {
				continue
			}
			if e := catalog.WithCurrentParticipants([]string{id}, in.RepositoryID, func() error { return nil }); e != nil {
				writeAPIError(w, 400, "invalid_evidence_audience", "audience members must be current repository participants")
				return
			}
		}
		out, e := evidence.CreateDefinition(in)
		writeAssuranceEvidence(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/assurance-evidence/definitions/{definition_id}/packages", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		definition, e := evidence.GetDefinition(r.PathValue("definition_id"))
		if e != nil || definition.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "assurance_evidence_not_found", "evidence definition not found")
			return
		}
		if definition.OwnerID != actor.UserID {
			writeAPIError(w, 403, "control_owner_required", "only the evidence definition owner may collect a package")
			return
		}
		var in struct {
			QueryIDs []string `json:"query_ids"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "source records are required")
			return
		}
		resolved, e := trusted.resolve(definition, in.QueryIDs)
		if e != nil {
			writeAPIError(w, 400, "unresolved_assurance_source", "every selected source must resolve through its repository-owned record store")
			return
		}
		out, e := evidence.CreatePackage(definition, actor.UserID, resolved)
		writeAssuranceEvidence(w, out, e, 201)
	})
}

func (s assuranceEvidenceSources) resolve(definition assuranceevidence.Definition, ids []string) ([]assuranceevidence.Source, error) {
	selected := map[string]bool{}
	for _, id := range ids {
		if selected[id] {
			return nil, assuranceevidence.ErrInvalid
		}
		selected[id] = true
	}
	out := []assuranceevidence.Source{}
	for _, query := range definition.Queries {
		if !selected[query.ID] {
			continue
		}
		source, err := s.resolveOne(definition.RepositoryID, query)
		if err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	if len(out) != len(selected) {
		return nil, assuranceevidence.ErrInvalid
	}
	return out, nil
}

func (s assuranceEvidenceSources) resolveOne(repo string, q assuranceevidence.Query) (assuranceevidence.Source, error) {
	source := assuranceevidence.Source{QueryID: q.ID, Kind: q.Kind, ResourceID: q.ResourceID, Accessible: true, Transformations: []string{"metadata_only", "credential_and_personal_data_excluded"}}
	switch q.Kind {
	case "review":
		if s.pulls == nil {
			return source, errors.New("review source unavailable")
		}
		v, e := s.pulls.Get(repo, q.ResourceID)
		if e != nil {
			return source, e
		}
		source.Revision, source.OccurredAt, source.Provenance, source.Summary = v.SourceCommitID, v.UpdatedAt, "repository pull review ledger", v.Status
	case "check", "build":
		if s.checks == nil || q.Path == "" {
			return source, errors.New("check source unavailable")
		}
		v, e := s.checks.Get(repo, q.Path, q.ResourceID)
		if e != nil || v.CompletedAt == nil {
			return source, errors.New("completed check not found")
		}
		source.Revision, source.OccurredAt, source.Provenance, source.Summary = v.CommitID, *v.CompletedAt, "exact-command check run", v.State
	case "release":
		if s.releases == nil {
			return source, errors.New("release source unavailable")
		}
		v, e := s.releases.Get(repo, q.ResourceID)
		if e != nil {
			return source, e
		}
		source.Revision, source.OccurredAt, source.Provenance, source.Summary = v.CommitID, v.CreatedAt, "repository release ledger", v.Status
	case "deployment":
		if s.deployments == nil {
			return source, errors.New("deployment source unavailable")
		}
		v, e := s.deployments.GetPromotion(repo, q.ResourceID)
		if e != nil {
			return source, e
		}
		source.Revision, source.OccurredAt, source.Provenance, source.Summary = v.CommitID, v.CreatedAt, "protected deployment ledger", v.State
	case "incident":
		if s.incidents == nil {
			return source, errors.New("incident source unavailable")
		}
		v, e := s.incidents.Get(q.ResourceID)
		if e != nil {
			return source, e
		}
		owned := false
		for _, scope := range v.Scopes {
			if scope.RepositoryID == repo {
				owned = true
			}
		}
		if !owned {
			return source, errors.New("cross-repository incident")
		}
		source.Revision, source.OccurredAt, source.Provenance, source.Summary = strconv.Itoa(v.Version), v.UpdatedAt, "repository incident ledger", v.Status
	case "privacy":
		if s.privacy == nil || q.Path == "" {
			return source, errors.New("privacy source unavailable")
		}
		v, e := s.privacy.Get(repo, q.Path)
		if e != nil {
			return source, e
		}
		source.ResourceID, source.Revision, source.OccurredAt, source.Provenance, source.Summary = v.ID, v.SourceRevision, v.UpdatedAt, "repository privacy review ledger", v.ResidualRisk
	case "continuity":
		if s.continuity == nil {
			return source, errors.New("continuity source unavailable")
		}
		v, e := s.continuity.Get(repo, q.ResourceID)
		if e != nil {
			return source, e
		}
		source.Revision, source.OccurredAt, source.Provenance, source.Summary = v.SourceRevision, v.FinishedAt, "repository recovery exercise", v.Status
	case "access":
		if s.access == nil {
			return source, errors.New("access source unavailable")
		}
		v, e := s.access.Get(q.ResourceID)
		if e != nil || v.RepositoryID != repo {
			return source, errors.New("access plan not found")
		}
		source.Revision, source.OccurredAt, source.Provenance, source.Summary = strconv.Itoa(v.Version), v.UpdatedAt, "repository protection plan", v.Mode
	case "dependency":
		if s.dependencies == nil || q.Path == "" {
			return source, errors.New("dependency source unavailable")
		}
		v, e := s.dependencies.Get(q.ResourceID, q.Path)
		if e != nil || v.RepositoryID != repo {
			return source, errors.New("dependency record not found")
		}
		source.ResourceID, source.Revision, source.OccurredAt, source.Provenance, source.Summary = v.ID, v.SourceCommit, v.PublishedAt, "published dependency inventory", v.Version
	case "governance":
		if s.governance == nil {
			return source, errors.New("governance source unavailable")
		}
		v, e := s.governance.Get(q.ResourceID)
		if e != nil || v.ScopeType != "repository" || v.ScopeID != repo {
			return source, errors.New("governance record not found")
		}
		source.Revision, source.OccurredAt, source.Provenance, source.Summary = fmt.Sprintf("charter-%d", v.CharterVersion), v.CreatedAt, "repository governance ledger", v.Status
	default:
		return source, errors.New("trusted source resolver unavailable")
	}
	if q.ResourceID != "" && source.ResourceID != q.ResourceID {
		return source, errors.New("source identity mismatch")
	}
	if q.Revision != "" && source.Revision != q.Revision {
		return source, errors.New("source revision mismatch")
	}
	if source.OccurredAt.IsZero() {
		source.OccurredAt = time.Now().UTC()
	}
	return source, nil
}

func evidenceAudienceAllows(audience []string, userID string, authenticated bool) bool {
	for _, member := range audience {
		if member == "repository" || (authenticated && member == userID) {
			return true
		}
	}
	return false
}

func writeAssuranceEvidence(w http.ResponseWriter, v any, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, assuranceevidence.ErrNotFound):
		writeAPIError(w, 404, "assurance_evidence_not_found", "assurance evidence not found")
	case errors.Is(e, assuranceevidence.ErrInvalid):
		writeAPIError(w, 400, "invalid_assurance_evidence", "periods, schedule, queries, source provenance, and transformations must be complete and consistent")
	default:
		writeAPIError(w, 500, "assurance_evidence_unavailable", "assurance evidence could not be persisted")
	}
}
