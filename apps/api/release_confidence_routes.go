package main

import (
	"errors"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/exploratorysessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releaseconfidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/testscenarios"
	"net/http"
)

func registerReleaseConfidenceRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, confidence *releaseconfidence.Store, pulls *pullrequests.Store, releaseStore *releases.Store, scenarios *testscenarios.Store, sessions *exploratorysessions.Store, checks *checkruns.Store, issueStore *issues.Store, proposalStore *proposals.Store) {
	mux.HandleFunc("POST /repositories/{id}/quality-requirements", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                             `json:"expected_version"`
			Requirements    []releaseconfidence.Requirement `json:"requirements"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		out, err := confidence.Publish(r.PathValue("id"), actor.UserID, in.ExpectedVersion, in.Requirements)
		writeConfidence(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/quality-attempts", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in releaseconfidence.Attempt
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		requirement, requirementErr := confidence.Requirement(r.PathValue("id"), in.RequirementID)
		if requirementErr != nil {
			writeAPIError(w, 422, "quality_evidence_invalid", "quality requirement does not resolve")
			return
		}
		if in.TargetKind == "pull" {
			target, e := pulls.Get(r.PathValue("id"), in.TargetID)
			if e != nil || target.SourceCommitID != in.Revision {
				writeAPIError(w, 422, "quality_evidence_invalid", "pull target must resolve at the attempted revision")
				return
			}
		} else if in.TargetKind == "release" {
			target, e := releaseStore.Get(r.PathValue("id"), in.TargetID)
			if e != nil || target.CommitID != in.Revision {
				writeAPIError(w, 422, "quality_evidence_invalid", "release target must resolve at the attempted revision")
				return
			}
		} else {
			writeAPIError(w, 422, "quality_evidence_invalid", "an exact pull or release target is required")
			return
		}
		// Evidence references must resolve in the repository; this record grants no execution authority.
		var scenario testscenarios.Scenario
		if in.ScenarioID != "" {
			var e error
			scenario, e = scenarios.Get(in.ScenarioID)
			if e != nil || scenario.RepositoryID != r.PathValue("id") || scenario.Implementation.CommitID != in.Revision {
				writeAPIError(w, 422, "quality_evidence_invalid", "scenario evidence must resolve at the attempted revision")
				return
			}
		}
		if in.SessionID != "" {
			x, e := sessions.Get(in.SessionID)
			signed := false
			for _, event := range x.Events {
				signed = signed || event.ID == in.SignoffEventID && event.Kind == "signoff"
			}
			if e != nil || x.RepositoryID != r.PathValue("id") || x.Source.Revision != in.Revision || x.Status != "closed" || !signed {
				writeAPIError(w, 422, "quality_evidence_invalid", "exploratory sign-off must resolve to a closed exact-revision session")
				return
			}
		}
		if in.CheckRunID != "" {
			x, e := checks.Get(r.PathValue("id"), in.PullRequestID, in.CheckRunID)
			if e != nil || x.CommitID != in.Revision || (in.TargetKind == "pull" && in.PullRequestID != in.TargetID) || (requirement.Kind == "scenario" && x.Definition.Command != scenario.Implementation.Command) {
				writeAPIError(w, 422, "quality_evidence_invalid", "test evidence must resolve to the exact retained check outcome")
				return
			}
			if x.State == "succeeded" {
				in.Status = "passed"
			} else {
				in.Status = "failed"
			}
		}
		if requirement.Kind == "exploratory_signoff" {
			in.Status = "passed"
		}
		in.OutcomeDerived = true
		out, err := confidence.RecordAttempt(r.PathValue("id"), actor.UserID, in)
		writeConfidence(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/quality-overrides", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in releaseconfidence.Override
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if in.FollowUpKind == "issue" {
			if _, err := issueStore.Get(r.PathValue("id"), in.FollowUpID); err != nil {
				writeAPIError(w, 422, "quality_follow_up_invalid", "override follow-up issue does not resolve")
				return
			}
		} else if in.FollowUpKind == "proposal" {
			if _, err := proposalStore.Get(r.PathValue("id"), in.FollowUpID); err != nil {
				writeAPIError(w, 422, "quality_follow_up_invalid", "override follow-up proposal does not resolve")
				return
			}
		} else {
			writeAPIError(w, 422, "quality_follow_up_invalid", "override follow-up must be an existing issue or proposal")
			return
		}
		out, err := confidence.Override(r.PathValue("id"), actor.UserID, in)
		writeConfidence(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/quality-confidence", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeConfidence(w, nil, e, 0)
			return
		}
		changes, e := pulls.Changes(r.PathValue("id"), p.ID)
		if e != nil {
			writeConfidence(w, nil, e, 0)
			return
		}
		paths := []string{}
		for _, c := range changes {
			paths = append(paths, c.Path)
		}
		m, e := confidence.Matrix(r.PathValue("id"), releaseconfidence.Target{Kind: "pull", ID: p.ID, Revision: p.SourceCommitID, Branch: p.TargetBranch, ChangedPaths: paths})
		writeConfidence(w, m, e, 200)
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}/quality-confidence", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		rel, e := releaseStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if e != nil {
			writeConfidence(w, nil, e, 0)
			return
		}
		m, e := confidence.Matrix(r.PathValue("id"), releaseconfidence.Target{Kind: "release", ID: rel.ID, Revision: rel.CommitID, Branch: rel.TargetBranch, Release: rel.Version, ChangedPaths: rel.ChangedPaths})
		writeConfidence(w, m, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/releases/{release_id}/quality-signals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		rel, e := releaseStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if e != nil {
			writeConfidence(w, nil, e, 0)
			return
		}
		var in releaseconfidence.Attempt
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.Revision = rel.CommitID
		in.TargetKind, in.TargetID = "release", rel.ID
		if in.Environment == "" || in.ScenarioID == "" || in.CheckRunID == "" || in.PullRequestID == "" {
			writeAPIError(w, 422, "quality_signal_invalid", "a sampled scenario, retained execution, and established environment are required")
			return
		}
		scenario, e := scenarios.Get(in.ScenarioID)
		if e != nil || scenario.RepositoryID != r.PathValue("id") || scenario.Implementation.CommitID != rel.CommitID {
			writeAPIError(w, 422, "quality_signal_invalid", "sampled scenario does not resolve")
			return
		}
		run, e := checks.Get(r.PathValue("id"), in.PullRequestID, in.CheckRunID)
		if e != nil || run.CommitID != rel.CommitID || run.Definition.Command != scenario.Implementation.Command {
			writeAPIError(w, 422, "quality_signal_invalid", "sampled scenario execution does not resolve at the release")
			return
		}
		if run.State == "succeeded" {
			in.Status = "passed"
		} else {
			in.Status = "failed"
		}
		in.OutcomeDerived = true
		in.Summary = "post-release: " + in.Summary
		out, e := confidence.RecordAttempt(r.PathValue("id"), actor.UserID, in)
		writeConfidence(w, out, e, 201)
	})
}

func writeConfidence(w http.ResponseWriter, value any, err error, status int) {
	if err == nil {
		writeJSON(w, status, value)
		return
	}
	if errors.Is(err, releaseconfidence.ErrNotFound) || errors.Is(err, pullrequests.ErrNotFound) || errors.Is(err, releases.ErrNotFound) {
		writeAPIError(w, 404, "quality_confidence_not_found", "quality confidence target was not found")
		return
	}
	if errors.Is(err, releaseconfidence.ErrConflict) {
		writeAPIError(w, 409, "quality_confidence_conflict", "quality requirements changed; reload before publishing")
		return
	}
	if errors.Is(err, releaseconfidence.ErrInvalid) {
		writeAPIError(w, 422, "quality_confidence_invalid", "quality requirements, evidence, or override are invalid")
		return
	}
	writeAPIError(w, 500, "quality_confidence_unavailable", "quality confidence could not be persisted")
}
