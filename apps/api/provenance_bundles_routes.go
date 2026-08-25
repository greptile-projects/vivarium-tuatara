package main

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/propagationcampaigns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenanceassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancebundles"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancegraphs"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancepolicies"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type bundleProjection struct {
	provenancebundles.Bundle
	Current           bool `json:"current"`
	ActionableNotices int  `json:"actionable_notices"`
	VerificationValid bool `json:"verification_valid"`
}

func registerProvenanceBundleRoutes(mux *http.ServeMux, repos *repositories.Store, credentials *auth.Store, bundles *provenancebundles.Store, graphs *provenancegraphs.Store, assessments *provenanceassessments.Store, policies *provenancepolicies.Store, releasesStore *releases.Store, packageStore *packages.Store, campaigns *propagationcampaigns.Store) {
	read := func(w http.ResponseWriter, r *http.Request, b provenancebundles.Bundle) bool {
		if b.Claim.Audience == "public" {
			return true
		}
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, b.Claim.RepositoryID)
		if !ok {
			return false
		}
		if !bundleReadableBy(b, actor.UserID) {
			writeAPIError(w, 403, "provenance_bundle_forbidden", "this bundle is restricted to its named audience")
			return false
		}
		return true
	}
	project := func(b provenancebundles.Bundle) bundleProjection {
		current := true
		if g, e := graphs.Get(b.Claim.GraphID); e != nil || g.AnalysisDigest != b.Claim.GraphDigest {
			current = false
		}
		if p, e := policies.Get(b.Claim.PolicyID); e != nil || p.CurrentVersion != b.Claim.PolicyVersion {
			current = false
		}
		for _, a := range b.Claim.Artifacts {
			if v, e := packageStore.Get(a.Name, a.Version); e != nil || v.SHA256 != a.SHA256 || v.Lifecycle != "active" {
				current = false
			}
		}
		return bundleProjection{Bundle: b, Current: current, ActionableNotices: len(b.Notices), VerificationValid: b.Signature != "" && b.PublicKey != "" && b.PayloadSHA256 != ""}
	}
	mux.HandleFunc("POST /repositories/{id}/releases/{release_id}/provenance-bundles", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "provenance_bundle_forbidden", "only the repository owner can publish a release provenance bundle")
			return
		}
		var in struct {
			RequestID    string   `json:"request_id"`
			GraphID      string   `json:"graph_id"`
			AssessmentID string   `json:"assessment_id"`
			Audience     string   `json:"audience"`
			AudienceIDs  []string `json:"audience_ids"`
			Omissions    []string `json:"omissions"`
			Verification []string `json:"verification"`
			Notices      []string `json:"notices"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		release, e := releasesStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if e != nil {
			writeAPIError(w, 404, "release_not_found", "release candidate not found")
			return
		}
		graph, e := graphs.Get(in.GraphID)
		if e != nil || graph.RepositoryID != release.RepositoryID || graph.Revision != release.CommitID || graph.Stale {
			writeAPIError(w, 422, "invalid_provenance_graph", "graph_id must name a current graph for the exact release revision")
			return
		}
		assessmentValues, e := assessments.List(release.RepositoryID, func(a provenanceassessments.Assessment) provenanceassessments.Current {
			return provenanceAssessmentCurrent(a, repos, graphs, policies, nil, nil, releasesStore, packageStore)
		})
		var assessment provenanceassessments.Assessment
		for _, candidate := range assessmentValues {
			if candidate.ID == in.AssessmentID {
				assessment = candidate
				break
			}
		}
		if e != nil || assessment.ID == "" || assessment.Candidate.Kind != "release_candidate" || assessment.Candidate.ID != release.ID || assessment.Candidate.Revision != release.CommitID || assessment.GraphID != graph.ID || !assessment.Ready {
			writeAPIError(w, 409, "release_provenance_required", "a current blocking-free assessment for this release and graph is required")
			return
		}
		versions, e := packageStore.ListRepository(release.RepositoryID)
		if e != nil {
			writeAPIError(w, 500, "provenance_bundle_failed", "release artifacts could not be read")
			return
		}
		claim := provenancebundles.Claim{Schema: "https://vivarium.dev/provenance-bundle/v1", RepositoryID: release.RepositoryID, ReleaseID: release.ID, ReleaseVersion: release.Version, Revision: release.CommitID, GraphID: graph.ID, GraphDigest: graph.AnalysisDigest, AssessmentID: assessment.ID, AssessmentVersion: assessment.Version, PolicyID: assessment.PolicyID, PolicyVersion: assessment.PolicyVersion, Audience: in.Audience, AudienceIDs: in.AudienceIDs, Omissions: cleanStrings(in.Omissions), Verification: cleanStrings(in.Verification), Notices: cleanStrings(in.Notices), PublishedBy: actor.UserID}
		for _, v := range versions {
			if v.ReleaseID != release.ID {
				continue
			}
			claim.Artifacts = append(claim.Artifacts, provenancebundles.Artifact{ID: v.ArtifactID, Name: v.Name, Version: v.Version, SHA256: v.SHA256, ContentType: v.ContentType, Size: v.Size})
			claim.BuildAttestations = append(claim.BuildAttestations, provenancebundles.Attestation{ID: v.BuildID, Kind: "build", Subject: v.Name + "@" + v.Version, Revision: v.SourceCommit, Digest: v.SHA256, Issuer: v.PublisherID})
			for _, d := range v.Dependencies {
				claim.Dependencies = append(claim.Dependencies, provenancebundles.Dependency{Name: d.Name, Constraint: d.Constraint})
			}
		}
		if len(claim.Artifacts) == 0 {
			writeAPIError(w, 409, "release_artifacts_required", "publish at least one exact release package artifact before its provenance bundle")
			return
		}
		for _, n := range graph.Nodes {
			if !nodeVisibleToBundleAudience(n, in.Audience, in.AudienceIDs) {
				continue
			}
			m := provenancebundles.Material{ID: n.ID, Kind: n.Kind, Name: n.Label, Revision: n.Revision, License: n.License, Obligations: n.Obligations}
			for _, c := range n.Citations {
				if c.SHA256 != "" {
					m.SHA256 = c.SHA256
					break
				}
			}
			claim.Materials = append(claim.Materials, m)
			if n.License != "" {
				claim.Licenses = append(claim.Licenses, n.License)
			}
			if n.Kind == "attestation" {
				claim.SourceAttestations = append(claim.SourceAttestations, provenancebundles.Attestation{ID: n.ID, Kind: "source", Subject: n.Label, Revision: n.Revision, Issuer: n.DeclaredBy})
			}
		}
		claim.Licenses = uniqueSorted(claim.Licenses)
		created, e := bundles.Create(provenancebundles.Bundle{RequestID: in.RequestID, Claim: claim})
		if errors.Is(e, provenancebundles.ErrConflict) {
			writeAPIError(w, 409, "provenance_bundle_conflict", "request_id was already used for another claim")
			return
		}
		if errors.Is(e, provenancebundles.ErrInvalid) {
			writeAPIError(w, 422, "invalid_provenance_bundle", "audience, verification instructions, and exact evidence are required")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "provenance_bundle_failed", "bundle could not be signed and retained")
			return
		}
		w.Header().Set("Location", "/provenance/bundles/"+created.ID)
		writeJSON(w, 201, project(created))
	})
	mux.HandleFunc("GET /provenance/bundles/{bundle_id}", func(w http.ResponseWriter, r *http.Request) {
		b, e := bundles.Get(r.PathValue("bundle_id"))
		if errors.Is(e, provenancebundles.ErrNotFound) {
			writeAPIError(w, 404, "provenance_bundle_not_found", "provenance bundle not found")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "provenance_bundle_unavailable", "provenance bundle could not be read")
			return
		}
		if !read(w, r, b) {
			return
		}
		writeJSON(w, 200, project(b))
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}/provenance-bundles", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		xs, e := bundles.List(r.PathValue("id"), r.PathValue("release_id"))
		if e != nil {
			writeAPIError(w, 500, "provenance_bundle_unavailable", "bundles could not be read")
			return
		}
		out := []bundleProjection{}
		for _, b := range xs {
			if !bundleReadableBy(b, actor.UserID) {
				continue
			}
			out = append(out, project(b))
		}
		writeJSON(w, 200, map[string]any{"bundles": out})
	})
	mux.HandleFunc("GET /packages/{name}/versions/{version}/provenance", func(w http.ResponseWriter, r *http.Request) {
		v, e := packageStore.Get(r.PathValue("name"), r.PathValue("version"))
		if e != nil {
			writeAPIError(w, 404, "package_not_found", "package version not found")
			return
		}
		xs, e := bundles.List(v.RepositoryID, v.ReleaseID)
		if e != nil || len(xs) == 0 {
			writeAPIError(w, 404, "provenance_bundle_not_found", "no provenance bundle covers this package version")
			return
		}
		for _, b := range xs {
			if b.Claim.Audience == "public" && bundleCoversPackage(b, v) {
				writeJSON(w, 200, project(b))
				return
			}
		}
		writeAPIError(w, 404, "provenance_bundle_not_found", "no public provenance bundle covers this package version")
	})
	mux.HandleFunc("GET /provenance/bundles/{bundle_id}/compare/{other_id}", func(w http.ResponseWriter, r *http.Request) {
		a, e := bundles.Get(r.PathValue("bundle_id"))
		if e != nil {
			writeAPIError(w, 404, "provenance_bundle_not_found", "provenance bundle not found")
			return
		}
		b, e := bundles.Get(r.PathValue("other_id"))
		if e != nil {
			writeAPIError(w, 404, "provenance_bundle_not_found", "comparison bundle not found")
			return
		}
		if !read(w, r, a) || !read(w, r, b) {
			return
		}
		writeJSON(w, 200, compareBundles(a, b))
	})
	mux.HandleFunc("POST /repositories/{id}/provenance-bundles/{bundle_id}/notices", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "provenance_notice_forbidden", "only the repository owner can publish a trust notice")
			return
		}
		b, e := bundles.Get(r.PathValue("bundle_id"))
		if e != nil || b.Claim.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "provenance_bundle_not_found", "provenance bundle not found")
			return
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
			provenancebundles.Notice
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if in.RemediationID != "" {
			found := false
			values, _ := assessments.List(b.Claim.RepositoryID, func(a provenanceassessments.Assessment) provenanceassessments.Current {
				return provenanceAssessmentCurrent(a, repos, graphs, policies, nil, nil, releasesStore, packageStore)
			})
			for _, a := range values {
				for _, repair := range a.Repairs {
					if repair.ID == in.RemediationID {
						found = true
					}
				}
			}
			if !found {
				writeAPIError(w, 422, "invalid_provenance_remediation", "remediation_id must name retained provenance repair work in this repository")
				return
			}
		}
		if in.PropagationCampaignID != "" {
			if campaigns == nil {
				writeAPIError(w, 422, "invalid_propagation_campaign", "propagation campaign storage is unavailable")
				return
			}
			if _, err := campaigns.Get(b.Claim.RepositoryID, in.PropagationCampaignID); err != nil {
				writeAPIError(w, 422, "invalid_propagation_campaign", "propagation_campaign_id must name a campaign in this repository")
				return
			}
		}
		out, e := bundles.AddNotice(b.ID, actor.UserID, in.ExpectedVersion, in.Notice)
		if errors.Is(e, provenancebundles.ErrConflict) {
			writeAPIError(w, 409, "provenance_notice_conflict", "bundle notices changed or request_id was reused")
			return
		}
		if errors.Is(e, provenancebundles.ErrInvalid) {
			writeAPIError(w, 422, "invalid_provenance_notice", "notice kind, severity, summary, and evidence are required")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "provenance_notice_failed", "notice could not be retained")
			return
		}
		writeJSON(w, 201, project(out))
	})
}
func cleanStrings(xs []string) []string {
	out := []string{}
	for _, x := range xs {
		if v := strings.TrimSpace(x); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func bundleReadableBy(b provenancebundles.Bundle, userID string) bool {
	return b.Claim.Audience != "restricted" || containsString(b.Claim.AudienceIDs, userID)
}

func nodeVisibleToBundleAudience(n provenancegraphs.Node, audience string, audienceIDs []string) bool {
	if audience == "public" {
		return n.Audience == "public" && !n.Restricted
	}
	if audience == "repository" {
		return n.Audience != "restricted" && !n.Restricted
	}
	if n.Audience != "restricted" && !n.Restricted {
		return true
	}
	if len(audienceIDs) == 0 {
		return false
	}
	for _, audienceID := range audienceIDs {
		if !containsString(n.AudienceIDs, audienceID) {
			return false
		}
	}
	return true
}

func bundleCoversPackage(b provenancebundles.Bundle, v packages.Version) bool {
	for _, artifact := range b.Claim.Artifacts {
		if artifact.Name == v.Name && artifact.Version == v.Version && artifact.SHA256 == v.SHA256 {
			return true
		}
	}
	return false
}
func uniqueSorted(xs []string) []string {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	out := []string{}
	for x := range m {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}
func compareBundles(a, b provenancebundles.Bundle) map[string]any {
	artifactKeys := func(xs []provenancebundles.Artifact) []string {
		out := []string{}
		for _, x := range xs {
			out = append(out, x.Name+"@"+x.Version+"#"+x.SHA256)
		}
		sort.Strings(out)
		return out
	}
	aArtifacts, bArtifacts := artifactKeys(a.Claim.Artifacts), artifactKeys(b.Claim.Artifacts)
	return map[string]any{"base_bundle_id": a.ID, "other_bundle_id": b.ID, "same_revision": a.Claim.Revision == b.Claim.Revision, "same_artifacts": len(bundleDifference(aArtifacts, bArtifacts)) == 0 && len(bundleDifference(bArtifacts, aArtifacts)) == 0, "added_artifacts": bundleDifference(bArtifacts, aArtifacts), "removed_artifacts": bundleDifference(aArtifacts, bArtifacts), "added_licenses": bundleDifference(b.Claim.Licenses, a.Claim.Licenses), "removed_licenses": bundleDifference(a.Claim.Licenses, b.Claim.Licenses), "added_omissions": bundleDifference(b.Claim.Omissions, a.Claim.Omissions), "base_payload_sha256": a.PayloadSHA256, "other_payload_sha256": b.PayloadSHA256}
}
func bundleDifference(a, b []string) []string {
	m := map[string]bool{}
	for _, x := range b {
		m[x] = true
	}
	out := []string{}
	for _, x := range a {
		if !m[x] {
			out = append(out, x)
		}
	}
	return out
}
