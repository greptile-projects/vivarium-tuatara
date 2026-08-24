package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/historyremediations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	packageversions "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityfindings"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
)

func registerHistoryRemediationRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *historyremediations.Store, findings *securityfindings.Store, incidentStore *incidents.Store, support *supportthreads.Store, releaseStore *releases.Store, packageStore *packageversions.Store, environments *deployments.Store, checks *checkruns.Store) {
	actorID := func(c auth.Credential) string {
		if c.AgentID != "" {
			return c.AgentID
		}
		return c.UserID
	}
	public := historyRemediationPublic
	mux.HandleFunc("GET /repositories/{id}/history-remediations", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		xs, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "history_remediations_unavailable", "history remediations could not be read")
			return
		}
		out := []historyremediations.Remediation{}
		for _, v := range xs {
			if historyRemediationCanSee(v, actorID(c)) {
				out = append(out, public(v))
			}
		}
		writeJSON(w, 200, map[string]any{"history_remediations": out})
	})
	mux.HandleFunc("GET /repositories/{id}/history-remediations/{remediation_id}", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := store.Get(r.PathValue("id"), r.PathValue("remediation_id"))
		if e != nil || !historyRemediationCanSee(v, actorID(c)) {
			writeAPIError(w, 404, "history_remediation_not_found", "history remediation not found")
			return
		}
		writeJSON(w, 200, public(v))
	})
	mux.HandleFunc("POST /repositories/{id}/history-remediations", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in historyremediations.Remediation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a history remediation is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		clean := in
		clean.ID = ""
		clean.RequestDigest = ""
		clean.CreatedBy = ""
		clean.CreatedAt = clean.CreatedAt.UTC()
		clean.Authority = ""
		b, _ := json.Marshal(clean)
		sum := sha256.Sum256(b)
		digest := hex.EncodeToString(sum[:])
		if existing, found, reconcileErr := store.Reconcile(in.RepositoryID, in.RequestID, digest); found {
			if !historyRemediationCanSee(existing, actorID(c)) {
				writeAPIError(w, 404, "history_remediation_not_found", "history remediation not found")
				return
			}
			writeJSON(w, 200, public(existing))
			return
		} else if errors.Is(reconcileErr, historyremediations.ErrConflict) {
			writeAPIError(w, 409, "history_remediation_request_conflict", "request_id was already used for a different remediation")
			return
		} else if reconcileErr != nil {
			writeAPIError(w, 500, "history_remediation_unavailable", "history remediation could not be reconciled")
			return
		}
		if c.AgentID != "" || (c.RepositoryID != "" && c.RepositoryID != in.RepositoryID) {
			writeAPIError(w, 403, "history_remediation_credential_forbidden", "history remediation requires an unbounded human maintainer credential or one bound to the source repository")
			return
		}
		for _, scope := range in.Scopes {
			if c.RepositoryID != "" && scope.RepositoryID != c.RepositoryID {
				writeAPIError(w, 403, "history_remediation_credential_forbidden", "a repository-bound credential cannot claim scope in another repository")
				return
			}
		}
		if !historyRemediationSourceExists(in, findings, incidentStore, support) {
			writeAPIError(w, 422, "history_remediation_source_missing", "the selected source must resolve in its authoritative store")
			return
		}
		// Every named participant and affected repository is resolved before restricted state is retained.
		people := append(append([]string{}, in.AudienceIDs...), in.OwnerIDs...)
		for _, a := range in.RequiredApprovals {
			people = append(people, a.ApproverIDs...)
		}
		for _, id := range people {
			member, _ := catalog.HasCollaborator(id, in.RepositoryID)
			repo, _ := catalog.GetByID(in.RepositoryID)
			if repo.ID == "" || (repo.OwnerID != id && !member) {
				writeAPIError(w, 422, "history_remediation_participant_invalid", "audience, owners, and approvers must be current source-repository participants")
				return
			}
		}
		repositoryIDs := []string{}
		for _, scope := range in.Scopes {
			repositoryIDs = append(repositoryIDs, scope.RepositoryID)
			repo, e := catalog.GetByID(scope.RepositoryID)
			if e != nil {
				writeAPIError(w, 422, "history_remediation_scope_unavailable", "every affected repository must resolve")
				return
			}
			allowed, _ := catalog.HasCollaborator(c.UserID, scope.RepositoryID)
			if repo.OwnerID != c.UserID && !allowed {
				writeAPIError(w, 403, "history_remediation_scope_forbidden", "the creator must maintain every affected repository")
				return
			}
			if !historyRemediationScopeExists(scope, git, catalog, releaseStore, packageStore, environments, checks) {
				writeAPIError(w, 422, "history_remediation_object_missing", "every scoped object, revision, ref, release, package artifact, and environment must resolve together")
				return
			}
		}
		participants := append([]string{c.UserID}, people...)
		var out historyremediations.Remediation
		e := catalog.WithCurrentParticipantsAndMaintainerAccess(participants, in.RepositoryID, c.UserID, repositoryIDs, func() error {
			var createErr error
			out, createErr = store.Create(in, actorID(c), digest)
			return createErr
		})
		if errors.Is(e, historyremediations.ErrConflict) {
			writeAPIError(w, 409, "history_remediation_request_conflict", "request_id was already used for a different remediation")
			return
		}
		if errors.Is(e, historyremediations.ErrInvalid) {
			writeAPIError(w, 422, "history_remediation_invalid", "source, payload-free content description, exact scope, discovery digests, audience, owners, and approvals are required")
			return
		}
		if errors.Is(e, repositories.ErrInvalidCollaborator) || errors.Is(e, repositories.ErrNotFound) {
			writeAPIError(w, 409, "history_remediation_authority_changed", "repository or participant authority changed before publication")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "history_remediation_unavailable", "history remediation could not be opened")
			return
		}
		writeJSON(w, 201, public(out))
	})
	mux.HandleFunc("POST /repositories/{id}/history-remediations/{remediation_id}/exposure-findings", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		v, err := store.Get(r.PathValue("id"), r.PathValue("remediation_id"))
		if err != nil || !historyRemediationCanSee(v, actorID(c)) {
			writeAPIError(w, 404, "history_remediation_not_found", "history remediation not found")
			return
		}
		if c.AgentID != "" && c.RepositoryID != v.RepositoryID {
			writeAPIError(w, 403, "history_remediation_agent_forbidden", "read-only agents must be bound to the remediation repository")
			return
		}
		var in struct {
			ExpectedVersion int                                 `json:"expected_version"`
			Finding         historyremediations.ExposureFinding `json:"finding"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exposure finding is required")
			return
		}
		out, err := store.AddExposureFinding(v.RepositoryID, v.ID, in.ExpectedVersion, in.Finding, actorID(c))
		switch {
		case errors.Is(err, historyremediations.ErrVersionConflict):
			writeAPIError(w, 409, "history_remediation_version_conflict", "the remediation changed; reload before adding the finding")
		case errors.Is(err, historyremediations.ErrConflict):
			writeAPIError(w, 409, "history_remediation_finding_conflict", "request_id was already used for a different finding")
		case errors.Is(err, historyremediations.ErrInvalid):
			writeAPIError(w, 422, "history_remediation_finding_invalid", "a permitted copy kind, affected object, state, digest citation, and bounded safe analysis are required")
		case err != nil:
			writeAPIError(w, 500, "history_remediation_unavailable", "the exposure finding could not be retained")
		default:
			writeJSON(w, 201, public(out))
		}
	})
}

func historyRemediationPublic(v historyremediations.Remediation) historyremediations.Remediation {
	v.RequestDigest = ""
	for i := range v.ExposureMap {
		if v.ExposureMap[i].Restricted {
			v.ExposureMap[i].ResourceID = "restricted-copy"
			v.ExposureMap[i].RepositoryID = ""
			v.ExposureMap[i].CitationResourceID = "restricted-citation"
			v.ExposureMap[i].Note = ""
			v.ExposureMap[i].Uncertainty = "Details are restricted by the independently controlled system."
		}
	}
	return v
}

func historyRemediationCanSee(v historyremediations.Remediation, actor string) bool {
	if v.CreatedBy == actor {
		return true
	}
	for _, ids := range [][]string{v.AudienceIDs, v.OwnerIDs} {
		for _, id := range ids {
			if id == actor {
				return true
			}
		}
	}
	for _, approval := range v.RequiredApprovals {
		for _, id := range approval.ApproverIDs {
			if id == actor {
				return true
			}
		}
	}
	return false
}

func historyRemediationSourceExists(v historyremediations.Remediation, findings *securityfindings.Store, incidentStore *incidents.Store, support *supportthreads.Store) bool {
	switch v.Source.Kind {
	case "security_finding":
		x, err := findings.Get(v.RepositoryID, v.Source.ResourceID)
		return err == nil && (v.Source.Revision == "" || v.Source.Revision == x.CandidateCommitID)
	case "privacy_incident":
		x, err := incidentStore.Get(v.Source.ResourceID)
		if err != nil {
			return false
		}
		for _, scope := range x.Scopes {
			if scope.RepositoryID == v.RepositoryID {
				return true
			}
		}
		return false
	case "support_case":
		_, err := support.Get(v.RepositoryID, v.Source.ResourceID)
		return err == nil
	case "selected_object":
		for _, scope := range v.Scopes {
			if scope.RepositoryID == v.RepositoryID && scope.ObjectID == v.Source.ResourceID && (v.Source.Revision == "" || v.Source.Revision == scope.Revision) {
				return true
			}
		}
		return false
	}
	return false
}

func historyRemediationScopeExists(scope historyremediations.Scope, git *storage.Store, catalog *repositories.Store, releaseStore *releases.Store, packageStore *packageversions.Store, environments *deployments.Store, checks *checkruns.Store) bool {
	if scope.Revision != "" && (len(scope.Revision) != 40 || !catalog.HasCommit(scope.RepositoryID, scope.Revision)) {
		return false
	}
	switch scope.Kind {
	case "git_object":
		repo, err := git.Open(scope.RepositoryID)
		if err != nil || len(scope.ObjectID) != 40 {
			return false
		}
		if _, err = repo.ReadObject(storage.ObjectID(scope.ObjectID)); err != nil {
			return false
		}
		if scope.Ref != "" {
			out, refErr := exec.Command("git", "--git-dir="+repo.Path(), "rev-parse", "--verify", scope.Ref).Output()
			resolved := strings.TrimSpace(string(out))
			if refErr != nil || resolved != scope.ObjectID || (scope.Revision != "" && resolved != scope.Revision) {
				return false
			}
		}
		return scope.ReleaseID == "" && scope.Package == "" && scope.ArtifactDigest == "" && scope.EnvironmentID == ""
	case "release":
		x, err := releaseStore.Get(scope.RepositoryID, scope.ReleaseID)
		return err == nil && scope.ObjectID == x.ID && (scope.Revision == "" || scope.Revision == x.CommitID) && scope.Ref == "" && scope.Package == "" && scope.ArtifactDigest == "" && scope.EnvironmentID == ""
	case "package":
		parts := strings.Split(scope.Package, "@")
		if len(parts) != 2 {
			return false
		}
		x, err := packageStore.Get(parts[0], parts[1])
		return err == nil && x.RepositoryID == scope.RepositoryID && scope.ObjectID == x.ArtifactID && scope.ArtifactDigest == x.SHA256 && (scope.Revision == "" || scope.Revision == x.SourceCommit) && scope.Ref == "" && scope.ReleaseID == "" && scope.EnvironmentID == ""
	case "environment":
		x, err := environments.GetEnvironment(scope.RepositoryID, scope.EnvironmentID)
		return err == nil && scope.ObjectID == x.ID && scope.Revision == "" && scope.Ref == "" && scope.ReleaseID == "" && scope.Package == "" && scope.ArtifactDigest == ""
	case "check_artifact":
		parts := strings.Split(scope.ObjectID, "/")
		if len(parts) != 3 || len(scope.ArtifactDigest) != 64 {
			return false
		}
		run, err := checks.Get(scope.RepositoryID, parts[0], parts[1])
		if err != nil {
			return false
		}
		for _, artifact := range run.Artifacts {
			if artifact.ID == parts[2] && artifact.SHA256 == scope.ArtifactDigest && (scope.Revision == "" || scope.Revision == run.CommitID) {
				return true
			}
		}
		return false
	}
	return false
}
