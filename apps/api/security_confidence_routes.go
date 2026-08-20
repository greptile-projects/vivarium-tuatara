package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityadvisories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityconfidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityfindings"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityscenarios"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/threatmodels"
)

func registerSecurityConfidenceRoutes(mux *http.ServeMux, catalog *repositories.Store, orgs *organizations.Store, credentials *auth.Store, confidence *securityconfidence.Store, pulls *pullrequests.Store, releaseStore *releases.Store, deploymentsStore *deployments.Store, models *threatmodels.Store, scenarios *securityscenarios.Store, findings *securityfindings.Store, issueStore *issues.Store, proposalStore *proposals.Store, incidentStore *incidents.Store, advisoryStore *securityadvisories.Store) {
	publish := func(scope string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			repo, _ := catalog.GetByID(r.PathValue("id"))
			scopeID := repo.ID
			if scope == "organization" {
				if orgs == nil || repo.OrganizationID == "" {
					writeAPIError(w, 422, "security_policy_scope_invalid", "repository has no organization")
					return
				}
				org, e := orgs.Get(repo.OrganizationID)
				if e != nil || org.CreatedBy != actor.UserID {
					writeAPIError(w, 403, "organization_owner_required", "organization security policy requires its creator")
					return
				}
				scopeID = repo.OrganizationID
			} else if !owner {
				writeAPIError(w, 403, "repository_owner_required", "repository security policy requires its owner")
				return
			}
			var in struct {
				ExpectedVersion int                              `json:"expected_version"`
				Requirements    []securityconfidence.Requirement `json:"requirements"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
				return
			}
			for _, requirement := range in.Requirements {
				for _, ownerID := range requirement.OwnerIDs {
					collaborator, _ := catalog.HasCollaborator(ownerID, repo.ID)
					if ownerID != repo.OwnerID && !collaborator {
						writeAPIError(w, 422, "security_owner_invalid", "every requirement owner must be a current repository participant")
						return
					}
				}
				if requirement.ThreatModelID != "" {
					model, err := models.Get(repo.ID, requirement.ThreatModelID, threatmodels.CurrentSource{})
					if err != nil || !threatModelHasAbusePath(model, requirement.ThreatModelVersion, requirement.AbusePathID) {
						writeAPIError(w, 422, "security_requirement_invalid", "threat requirement must resolve to an exact model revision and abuse path")
						return
					}
				}
				if requirement.ScenarioID != "" {
					scenario, err := scenarios.Get(repo.ID, requirement.ScenarioID)
					if err != nil || scenario.ThreatModelID != requirement.ThreatModelID || scenario.ThreatModelVersion != requirement.ThreatModelVersion || scenario.AbusePathID != requirement.AbusePathID {
						writeAPIError(w, 422, "security_requirement_invalid", "scenario requirement must resolve to the selected threat path")
						return
					}
				}
			}
			out, e := confidence.Publish(scope, scopeID, repo.ID, actor.UserID, in.ExpectedVersion, in.Requirements)
			writeSecurityConfidence(w, out, e, 201)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/security-requirements", publish("repository"))
	mux.HandleFunc("POST /repositories/{id}/organization-security-requirements", publish("organization"))
	mux.HandleFunc("POST /repositories/{id}/security-exceptions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in securityconfidence.Exception
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		p, e := effectiveSecurityPolicy(confidence, catalog, r.PathValue("id"))
		owned := false
		for _, q := range p.Requirements {
			owned = owned || (q.ID == in.RequirementID && containsSecurityString(q.OwnerIDs, actor.UserID))
		}
		if e != nil || !owned {
			writeAPIError(w, 403, "control_owner_required", "only a named requirement owner may create an exception")
			return
		}
		if in.FollowUpKind == "issue" {
			_, e = issueStore.Get(r.PathValue("id"), in.FollowUpID)
		} else if in.FollowUpKind == "proposal" {
			_, e = proposalStore.Get(r.PathValue("id"), in.FollowUpID)
		} else {
			e = errors.New("invalid")
		}
		if e != nil {
			writeAPIError(w, 422, "security_follow_up_invalid", "exception follow-up must resolve")
			return
		}
		out, e := confidence.AddException(r.PathValue("id"), actor.UserID, in)
		writeSecurityConfidence(w, out, e, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/security-confidence", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeSecurityConfidence(w, nil, e, 0)
			return
		}
		changes, e := pulls.Changes(p.RepositoryID, p.ID)
		paths := []string{}
		for _, c := range changes {
			paths = append(paths, c.Path)
		}
		m, e := securityMatrix(confidence, catalog, models, scenarios, findings, p.RepositoryID, actor.UserID, "pull", p.ID, p.SourceCommitID, p.TargetBranch, paths)
		writeSecurityConfidence(w, m, e, 200)
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}/security-confidence", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := releaseStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if e != nil {
			writeSecurityConfidence(w, nil, e, 0)
			return
		}
		m, e := securityMatrix(confidence, catalog, models, scenarios, findings, v.RepositoryID, actor.UserID, "release", v.ID, v.CommitID, v.TargetBranch, v.ChangedPaths)
		writeSecurityConfidence(w, m, e, 200)
	})
	mux.HandleFunc("GET /repositories/{id}/deployments/{deployment_id}/security-confidence", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		d, e := deploymentsStore.GetPromotion(r.PathValue("id"), r.PathValue("deployment_id"))
		if e != nil {
			writeSecurityConfidence(w, nil, e, 0)
			return
		}
		rel, e := releaseStore.Get(d.RepositoryID, d.ReleaseID)
		if e != nil {
			writeSecurityConfidence(w, nil, e, 0)
			return
		}
		m, e := securityMatrix(confidence, catalog, models, scenarios, findings, d.RepositoryID, actor.UserID, "deployment", d.ID, d.CommitID, rel.TargetBranch, rel.ChangedPaths)
		writeSecurityConfidence(w, m, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/deployments/{deployment_id}/security-signals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		d, e := deploymentsStore.GetPromotion(r.PathValue("id"), r.PathValue("deployment_id"))
		if e != nil {
			writeSecurityConfidence(w, nil, e, 0)
			return
		}
		var in securityconfidence.Signal
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.DeploymentID = d.ID
		in.ReleaseID = d.ReleaseID
		in.Revision = d.CommitID
		in.EnvironmentID = d.EnvironmentID
		policy, policyErr := effectiveSecurityPolicy(confidence, catalog, d.RepositoryID)
		requirementExists := false
		for _, requirement := range policy.Requirements {
			requirementExists = requirementExists || requirement.ID == in.RequirementID
		}
		if policyErr != nil || !requirementExists {
			writeAPIError(w, 422, "security_signal_invalid", "signal requirement must resolve through current security policy")
			return
		}
		if in.ResponseKind != "" {
			valid := false
			switch in.ResponseKind {
			case "private_incident":
				v, err := incidentStore.Get(in.ResponseID)
				valid = err == nil && incidentScopesRepository(v, d.RepositoryID)
			case "security_advisory":
				v, err := advisoryStore.Get(in.ResponseID)
				valid = err == nil && advisoryAffectsRepository(v, d.RepositoryID) && securityAdvisoryVisible(catalog, actor.UserID, v)
			case "repair":
				_, err := proposalStore.Get(d.RepositoryID, in.ResponseID)
				valid = err == nil
			}
			if !valid {
				writeAPIError(w, 422, "security_response_invalid", "signal response must resolve to a private incident, repository advisory, or governed repair")
				return
			}
		}
		out, e := confidence.RecordSignal(d.RepositoryID, actor.UserID, in)
		writeSecurityConfidence(w, out, e, 201)
	})
}

func effectiveSecurityPolicy(s *securityconfidence.Store, c *repositories.Store, repoID string) (securityconfidence.Policy, error) {
	if p, e := s.Current("repository", repoID); e == nil {
		return p, nil
	}
	repo, e := c.GetByID(repoID)
	if e != nil || repo.OrganizationID == "" {
		return securityconfidence.Policy{}, securityconfidence.ErrNotFound
	}
	p, e := s.Current("organization", repo.OrganizationID)
	// Organization policy is shared intent; evaluation and exceptions remain
	// repository-local and never transfer repository authority.
	p.RepositoryID = repoID
	return p, e
}
func securityMatrix(s *securityconfidence.Store, c *repositories.Store, models *threatmodels.Store, scenarios *securityscenarios.Store, findings *securityfindings.Store, repo, reader, kind, id, revision, branch string, paths []string) (securityconfidence.Matrix, error) {
	p, e := effectiveSecurityPolicy(s, c, repo)
	if e != nil {
		return securityconfidence.Matrix{}, e
	}
	ownerRepo, _ := c.GetByID(repo)
	allFindings, err := findings.List(repo, ownerRepo.OwnerID)
	if err != nil {
		return securityconfidence.Matrix{}, err
	}
	visible, err := findings.List(repo, reader)
	if err != nil {
		return securityconfidence.Matrix{}, err
	}
	visibleIDs := map[string]bool{}
	for _, f := range visible {
		visibleIDs[f.ID] = true
	}
	ev := map[string]securityconfidence.Evidence{}
	for _, q := range p.Requirements {
		x := securityconfidence.Evidence{}
		if q.ThreatModelID != "" {
			m, err := models.Get(repo, q.ThreatModelID, threatmodels.CurrentSource{})
			if err == nil && m.CurrentVersion >= q.ThreatModelVersion {
				r := m.Revisions[m.CurrentVersion-1]
				deps := map[string]string{}
				for _, d := range r.Dependencies {
					deps[d.ID] = d.Revision
				}
				m, err = models.Get(repo, q.ThreatModelID, threatmodels.CurrentSource{Revision: r.Source.Revision, ArchitectureDigest: r.ArchitectureDigest, TrustBoundaryDigest: r.TrustBoundaryDigest, DependencyRevisions: deps})
				x.ThreatRevision = m.CurrentVersion
				x.ThreatCurrent = err == nil && m.Freshness.Fresh && (r.Source.Revision == revision || !securityPathsIntersect(paths, q.Selector.Paths))
				for _, a := range m.Acknowledgements {
					if a.ModelVersion == m.CurrentVersion && a.Decision == "acknowledged" {
						x.AcknowledgedOwnerIDs = append(x.AcknowledgedOwnerIDs, a.OwnerID)
					}
				}
				for _, a := range r.AbusePaths {
					if a.ID == q.AbusePathID {
						x.ResidualRisk = a.ResidualRisk
					}
				}
			}
		}
		if q.ScenarioID != "" {
			scenario, err := scenarios.Get(repo, q.ScenarioID)
			if err == nil && securityScenarioCurrent(q, scenario, revision, paths) {
				for i := len(scenario.Attempts) - 1; i >= 0; i-- {
					a := scenario.Attempts[i]
					if a.Revision == scenario.CommitID {
						x.ScenarioAttemptID = a.ID
						x.ScenarioResult = a.Result
						break
					}
				}
			}
		}
		for _, f := range allFindings {
			classification := securityfindings.CurrentClassification(f)
			severity := len(q.FindingSeverities) == 0 || containsSecurityString(q.FindingSeverities, f.Severity)
			scoped := q.ThreatModelID == "" || f.ThreatModelID == q.ThreatModelID
			resolved := classification == "false_positive" || classification == "suspected_duplicate" || f.Repair != nil && f.Repair.State == "protected"
			if severity && scoped && !resolved {
				if visibleIDs[f.ID] {
					x.OpenFindingIDs = append(x.OpenFindingIDs, f.ID)
				} else {
					x.OpenFindingIDs = append(x.OpenFindingIDs, "restricted")
				}
			}
		}
		ev[q.ID] = x
	}
	return s.Evaluate(p, kind, id, revision, branch, paths, ev)
}
func securityPathsIntersect(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y || strings.HasPrefix(x, strings.TrimSuffix(y, "/")+"/") || strings.HasPrefix(y, strings.TrimSuffix(x, "/")+"/") {
				return true
			}
		}
	}
	return false
}
func securityScenarioCurrent(q securityconfidence.Requirement, scenario securityscenarios.Scenario, revision string, paths []string) bool {
	if scenario.Review == nil || scenario.Review.Decision != "approved" {
		return false
	}
	if scenario.CommitID == revision {
		return true
	}
	// DependencyIDs are semantic threat-model identities, not repository paths.
	// Reuse older proof only when policy freezes an explicit path scope and the
	// candidate does not intersect it; otherwise require an exact-revision rerun.
	return len(q.Selector.Paths) > 0 && !securityPathsIntersect(paths, q.Selector.Paths)
}
func containsSecurityString(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func threatModelHasAbusePath(model threatmodels.Model, version int, pathID string) bool {
	if version < 1 || version > len(model.Revisions) {
		return false
	}
	for _, path := range model.Revisions[version-1].AbusePaths {
		if path.ID == pathID {
			return true
		}
	}
	return false
}
func incidentScopesRepository(v incidents.Incident, repo string) bool {
	for _, scope := range v.Scopes {
		if scope.RepositoryID == repo {
			return true
		}
	}
	return false
}
func advisoryAffectsRepository(v securityadvisories.Advisory, repo string) bool {
	for _, affected := range v.AffectedRepositories {
		if affected.RepositoryID == repo {
			return true
		}
	}
	return false
}
func securityAdvisoryVisible(catalog *repositories.Store, actor string, v securityadvisories.Advisory) bool {
	if actor == v.ReporterID || containsSecurityString(v.ResponseTeam, actor) {
		return true
	}
	for _, affected := range v.AffectedRepositories {
		repo, err := catalog.GetByID(affected.RepositoryID)
		if err == nil && repo.OwnerID == actor {
			return true
		}
	}
	return false
}
func writeSecurityConfidence(w http.ResponseWriter, v any, e error, status int) {
	if e == nil {
		writeJSON(w, status, v)
		return
	}
	if errors.Is(e, securityconfidence.ErrNotFound) || errors.Is(e, pullrequests.ErrNotFound) || errors.Is(e, releases.ErrNotFound) || errors.Is(e, deployments.ErrNotFound) {
		writeAPIError(w, 404, "security_confidence_not_found", "security confidence target or policy was not found")
		return
	}
	if errors.Is(e, securityconfidence.ErrConflict) {
		writeAPIError(w, 409, "security_confidence_conflict", "security policy changed; reload before publishing")
		return
	}
	if errors.Is(e, securityconfidence.ErrInvalid) {
		writeAPIError(w, 422, "security_confidence_invalid", "security policy, evidence, exception, or signal is invalid")
		return
	}
	writeAPIError(w, 500, "security_confidence_unavailable", "security confidence could not be evaluated")
}
