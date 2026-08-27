package main

import (
	"errors"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/adoptioncampaigns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	docscollections "github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/durableschemas"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancebundles"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancegraphs"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancepolicies"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"log"
	"net/http"
	"strconv"
)

type adoptionCampaignInput struct {
	RequestID       string                     `json:"request_id"`
	ExpectedVersion int                        `json:"expected_version"`
	Revision        adoptioncampaigns.Revision `json:"revision"`
}

func registerAdoptionCampaignRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *adoptioncampaigns.Store, releaseStore *releases.Store, bundles *provenancebundles.Store, graphs *provenancegraphs.Store, policies *provenancepolicies.Store, packageStore *packages.Store, pulls *pullrequests.Store, decisionStore *decisions.Store, documentationStore *docscollections.Store, apiStore *apicontracts.Store, schemaStore *durableschemas.Store) {
	store.ConfigureProjection(func(c adoptioncampaigns.Campaign) adoptioncampaigns.Projection {
		p := adoptioncampaigns.Projection{}
		if len(c.Revisions) == 0 {
			return p
		}
		current := c.Revisions[len(c.Revisions)-1]
		repository, repositoryErr := catalog.GetByID(c.RepositoryID)
		for _, ownerID := range current.OwnerIDs {
			if repositoryErr != nil || !repository.HasParticipant(ownerID) {
				p.MissingOwners = append(p.MissingOwners, ownerID)
			}
		}
		for _, link := range current.Links {
			if !adoptionLinkCurrent(c.RepositoryID, link, pulls, decisionStore, documentationStore, packageStore, apiStore, schemaStore) {
				p.InvalidLinks = append(p.InvalidLinks, link.ID)
			}
		}
		xs, err := releaseStore.List(c.RepositoryID)
		if err != nil {
			return p
		}
		for _, release := range xs {
			if release.ID != current.ReleaseID && release.CreatedAt.After(c.CreatedAt) && release.Status != "withdrawn" {
				p.ReleaseSuperseded = true
				break
			}
		}
		return p
	})
	mux.HandleFunc("GET /repositories/{id}/adoption-campaigns", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		xs, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "adoption_campaigns_unavailable", "adoption campaigns could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"adoption_campaigns": xs})
	})
	mux.HandleFunc("GET /repositories/{id}/adoption-campaigns/{campaign_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := store.Get(r.PathValue("campaign_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "adoption_campaign_not_found", "adoption campaign not found")
			return
		}
		writeJSON(w, 200, v)
	})
	publish := func(w http.ResponseWriter, r *http.Request, revise bool) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in adoptionCampaignInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete adoption campaign revision is required")
			return
		}
		rel, e := releaseStore.Get(r.PathValue("id"), in.Revision.ReleaseID)
		if e != nil || rel.CommitID != in.Revision.ReleaseRevision {
			writeAPIError(w, 422, "adoption_release_invalid", "the campaign must bind an exact repository release")
			return
		}
		bundle, e := bundles.Get(in.Revision.AttestationID)
		if e != nil || bundle.Claim.RepositoryID != r.PathValue("id") || bundle.Claim.ReleaseID != rel.ID || bundle.Claim.Revision != rel.CommitID {
			writeAPIError(w, 422, "adoption_attestation_invalid", "the campaign must bind the release's exact signed provenance attestation")
			return
		}
		if !adoptionBundleCurrent(bundle, graphs, policies, packageStore) {
			writeAPIError(w, 422, "adoption_attestation_not_current", "the campaign requires current blocking-free release provenance")
			return
		}
		var out adoptioncampaigns.Campaign
		e = catalog.WithCurrentParticipants(append([]string{actor.UserID}, in.Revision.OwnerIDs...), r.PathValue("id"), func() error {
			return bundles.WithCurrent(in.Revision.AttestationID, func(locked provenancebundles.Bundle) error {
				if locked.Claim.RepositoryID != r.PathValue("id") || locked.Claim.ReleaseID != rel.ID || locked.Claim.Revision != rel.CommitID {
					return errAdoptionAttestationNotCurrent
				}
				return policies.WithCurrent(locked.Claim.PolicyID, func(policy provenancepolicies.Policy) error {
					if policy.CurrentVersion != locked.Claim.PolicyVersion || !adoptionBundleMaterialsCurrent(locked, graphs, packageStore) {
						return errAdoptionAttestationNotCurrent
					}
					var x error
					if revise {
						out, x = store.Revise(r.PathValue("id"), r.PathValue("campaign_id"), in.ExpectedVersion, actor.UserID, in.RequestID, in.Revision)
					} else {
						out, x = store.Create(r.PathValue("id"), actor.UserID, in.RequestID, in.Revision)
					}
					return x
				})
			})
		})
		if errors.Is(e, errAdoptionAttestationNotCurrent) {
			writeAPIError(w, 422, "adoption_attestation_not_current", "the campaign requires current blocking-free release provenance")
			return
		}
		writeAdoptionCampaign(w, out, e, map[bool]int{true: 200, false: 201}[revise])
	}
	mux.HandleFunc("POST /repositories/{id}/adoption-campaigns", func(w http.ResponseWriter, r *http.Request) { publish(w, r, false) })
	mux.HandleFunc("POST /repositories/{id}/adoption-campaigns/{campaign_id}/revisions", func(w http.ResponseWriter, r *http.Request) { publish(w, r, true) })
}

var errAdoptionAttestationNotCurrent = errors.New("adoption attestation not current")

func adoptionLinkCurrent(repo string, link adoptioncampaigns.Link, pulls *pullrequests.Store, decisionStore *decisions.Store, documentationStore *docscollections.Store, packageStore *packages.Store, apiStore *apicontracts.Store, schemaStore *durableschemas.Store) bool {
	version, versionErr := strconv.Atoi(link.Revision)
	switch link.Kind {
	case "change":
		if pulls == nil {
			return false
		}
		v, err := pulls.Get(repo, link.ResourceID)
		return err == nil && link.Revision != "" && v.SourceCommitID == link.Revision
	case "decision":
		if decisionStore == nil {
			return false
		}
		v, err := decisionStore.Get(link.ResourceID)
		return err == nil && v.RepositoryID == repo && versionErr == nil && v.Version == version
	case "documentation":
		if documentationStore == nil {
			return false
		}
		xs, err := documentationStore.List(repo, link.ResourceID)
		if err != nil {
			return false
		}
		for _, v := range xs {
			if v.ID == link.Revision || (versionErr == nil && v.Version == version) {
				return true
			}
		}
		return false
	case "package":
		if packageStore == nil {
			return false
		}
		v, err := packageStore.Get(link.ResourceID, link.Revision)
		return err == nil && v.RepositoryID == repo && v.Lifecycle == "active"
	case "api":
		if apiStore == nil {
			return false
		}
		v, err := apiStore.Get(link.ResourceID)
		return err == nil && v.RepositoryID == repo && versionErr == nil && v.CurrentVersion == version
	case "schema":
		if schemaStore == nil {
			return false
		}
		v, err := schemaStore.Get(repo, link.ResourceID)
		return err == nil && versionErr == nil && v.CurrentVersion == version
	case "compatibility":
		if apiStore == nil || schemaStore == nil {
			return false
		}
		if v, err := apiStore.Get(link.ResourceID); err == nil && v.RepositoryID == repo && versionErr == nil && v.CurrentVersion == version {
			return true
		}
		v, err := schemaStore.Get(repo, link.ResourceID)
		return err == nil && versionErr == nil && v.CurrentVersion == version
	default:
		return false
	}
}

func adoptionBundleCurrent(bundle provenancebundles.Bundle, graphs *provenancegraphs.Store, policies *provenancepolicies.Store, packageStore *packages.Store) bool {
	for _, notice := range bundle.Notices {
		if notice.Severity == "blocking" {
			return false
		}
	}
	if policies == nil {
		return false
	}
	policy, err := policies.Get(bundle.Claim.PolicyID)
	if err != nil || policy.CurrentVersion != bundle.Claim.PolicyVersion {
		return false
	}
	return adoptionBundleMaterialsCurrent(bundle, graphs, packageStore)
}

func adoptionBundleMaterialsCurrent(bundle provenancebundles.Bundle, graphs *provenancegraphs.Store, packageStore *packages.Store) bool {
	for _, notice := range bundle.Notices {
		if notice.Severity == "blocking" {
			return false
		}
	}
	if graphs == nil || packageStore == nil {
		return false
	}
	graph, err := graphs.Get(bundle.Claim.GraphID)
	if err != nil || graph.AnalysisDigest != bundle.Claim.GraphDigest {
		return false
	}
	for _, artifact := range bundle.Claim.Artifacts {
		version, err := packageStore.Get(artifact.Name, artifact.Version)
		if err != nil || version.SHA256 != artifact.SHA256 || version.Lifecycle != "active" {
			return false
		}
	}
	return bundleVerificationValid(bundle)
}
func writeAdoptionCampaign(w http.ResponseWriter, v adoptioncampaigns.Campaign, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, adoptioncampaigns.ErrNotFound):
		writeAPIError(w, 404, "adoption_campaign_not_found", "adoption campaign not found")
	case errors.Is(e, adoptioncampaigns.ErrConflict):
		writeAPIError(w, 409, "adoption_campaign_conflict", "the campaign changed or this request identity was reused with different content")
	case errors.Is(e, adoptioncampaigns.ErrInvalid):
		writeAPIError(w, 400, "invalid_adoption_campaign", "release, attestation, audiences, versions, coverage, deadline, measures, support, rollback, owners, and links are required")
	case errors.Is(e, repositories.ErrInvalidCollaborator), errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 403, "adoption_campaign_forbidden", "all accountable owners must be current repository participants")
	default:
		log.Printf("adoption campaign storage: %v", e)
		writeAPIError(w, 500, "adoption_campaigns_unavailable", "adoption campaign could not be persisted")
	}
}
