package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func registerAssuranceAssessmentRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, people *users.Store, programs *assuranceprograms.Store, evidence *assuranceevidence.Store, assessments *assuranceassessments.Store, proposalStore *proposals.Store, releaseStore *releases.Store) {
	mux.HandleFunc("GET /repositories/{id}/assurance-assessments", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		values, err := assessments.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "assessments_unavailable", "assessments could not be read")
			return
		}
		out := values[:0]
		now := time.Now().UTC()
		for _, a := range values {
			if a.OwnerID == actor.UserID || (a.Assessor.UserID == actor.UserID && assessorWindowOpen(a, now)) {
				out = append(out, a)
			}
		}
		writeJSON(w, 200, map[string]any{"assessments": out})
	})
	mux.HandleFunc("GET /repositories/{id}/assurance-assessments/{assessment_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		a, err := assessments.Get(r.PathValue("assessment_id"))
		if err != nil || a.RepositoryID != r.PathValue("id") || !assessmentParty(a, actor.UserID) {
			writeAPIError(w, 404, "assessment_not_found", "assessment not found")
			return
		}
		if actor.UserID == a.Assessor.UserID && actor.UserID != a.OwnerID && !writeAssessorWindowError(w, a, time.Now().UTC()) {
			return
		}
		packages := []assuranceevidence.Package{}
		for _, id := range a.EvidencePackageIDs {
			p, e := evidence.GetPackage(id)
			if e == nil && p.RepositoryID == a.RepositoryID {
				packages = append(packages, p)
			}
		}
		writeJSON(w, 200, map[string]any{"assessment": a, "evidence_packages": packages, "authority_note": "assessment access grants no repository, source-system, production, review, release, deployment, or project mutation authority"})
	})
	mux.HandleFunc("POST /repositories/{id}/assurance-assessments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in assuranceassessments.Assessment
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete bounded assessment is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		in.OwnerID = actor.UserID
		program, err := programs.Get(in.ProgramID)
		if err != nil || program.RepositoryID != in.RepositoryID || in.ProgramVersion < 1 || in.ProgramVersion > len(program.Revisions) {
			writeAPIError(w, 400, "invalid_program_revision", "an exact repository assurance program revision is required")
			return
		}
		revision := program.Revisions[in.ProgramVersion-1]
		if !hasID(revision.OwnerIDs, actor.UserID) {
			writeAPIError(w, 403, "program_owner_required", "only a named program owner may open an independent assessment")
			return
		}
		if _, err = people.Get(in.Assessor.UserID); err != nil {
			writeAPIError(w, 400, "invalid_assessor", "the assessor must have an identified platform account")
			return
		}
		if !assessmentScopeValid(revision, in.Scope) {
			writeAPIError(w, 400, "invalid_assessment_scope", "controls, systems, and releases must be selected from the exact program revision")
			return
		}
		for _, id := range in.EvidencePackageIDs {
			p, e := evidence.GetPackage(id)
			if e != nil || p.RepositoryID != in.RepositoryID || p.ProgramID != in.ProgramID || p.ProgramVersion != in.ProgramVersion || !hasID(in.Scope.ControlIDs, p.ControlID) || p.PeriodStartsAt.Before(in.Scope.PeriodStartsAt) || p.PeriodEndsAt.After(in.Scope.PeriodEndsAt) {
				writeAPIError(w, 400, "invalid_assessment_evidence", "evidence must be explicitly selected from the exact program, controls, and assessment period")
				return
			}
		}
		out, err := assessments.Create(in)
		writeAssessment(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/assurance-assessments/{assessment_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		a, err := assessments.Get(r.PathValue("assessment_id"))
		if err != nil || a.RepositoryID != r.PathValue("id") || !assessmentParty(a, actor.UserID) {
			writeAPIError(w, 404, "assessment_not_found", "assessment not found")
			return
		}
		role := "assessor"
		if actor.UserID == a.OwnerID {
			role = "owner"
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
			assuranceassessments.Event
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete attributable assessment event is required")
			return
		}
		out, err := assessments.Append(a.ID, in.ExpectedVersion, actor.UserID, role, in.Event)
		writeAssessment(w, out, err, 201)
	})
	if proposalStore != nil {
		mux.HandleFunc("POST /repositories/{id}/assurance-assessments/{assessment_id}/remediations", func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			a, err := assessments.Get(r.PathValue("assessment_id"))
			if err != nil || a.RepositoryID != r.PathValue("id") {
				writeAPIError(w, 404, "assessment_not_found", "assessment not found")
				return
			}
			if actor.UserID != a.OwnerID {
				writeAPIError(w, 403, "assessment_owner_required", "only the program owner may authorize corrective work")
				return
			}
			var in struct {
				ExpectedVersion    int       `json:"expected_version"`
				FindingEventID     string    `json:"finding_event_id"`
				AffectedRevision   string    `json:"affected_revision"`
				Deadline           time.Time `json:"deadline"`
				AcceptanceCriteria []string  `json:"acceptance_criteria"`
				Title              string    `json:"title"`
				Tasks              []struct {
					Title        string `json:"title"`
					AssigneeType string `json:"assignee_type"`
					AssigneeID   string `json:"assignee_id"`
				} `json:"tasks"`
			}
			if decodeJSON(r, &in) != nil || len(in.Tasks) == 0 {
				writeAPIError(w, 400, "invalid_remediation", "ordered corrective tasks and frozen finding context are required")
				return
			}
			var finding assuranceassessments.Event
			found := false
			for _, e := range a.Events {
				if e.ID == in.FindingEventID && e.Kind == "finding" {
					finding, found = e, true
				}
			}
			if !found {
				writeAPIError(w, 400, "invalid_finding", "corrective work must originate from an assessment finding")
				return
			}
			criteria := strings.Join(in.AcceptanceCriteria, "; ")
			taskInputs := make([]proposals.ImplementationTaskInput, len(in.Tasks))
			for i, task := range in.Tasks {
				taskInputs[i] = proposals.ImplementationTaskInput{Title: task.Title, Outcome: criteria, Risk: "Unresolved assurance finding for control " + finding.ControlID + " at " + in.AffectedRevision, VerificationPlan: "Fresh evidence must satisfy: " + criteria, AssigneeType: task.AssigneeType, AssigneeID: task.AssigneeID, DependsOnPrevious: i > 0}
			}
			origin := proposals.ReasoningOrigin{AssessmentID: a.ID, AssessmentVersion: a.Version, AssuranceFindingID: finding.ID, Revision: in.AffectedRevision, SelectedItemIDs: []string{finding.ID}, Items: []proposals.ReasoningItem{{ID: finding.ID, Kind: "assurance_finding", Summary: finding.Body, Status: finding.Status}}, AnalysisStatus: "authorized_assurance_remediation"}
			p, tasks, err := proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: a.RepositoryID, ActorID: actor.UserID, Title: in.Title, Body: "Correct assurance finding " + finding.ID + " by " + in.Deadline.UTC().Format(time.RFC3339) + ".\n\nControl: " + finding.ControlID + "\nAffected revision: " + in.AffectedRevision + "\nAcceptance criteria: " + criteria, Origin: origin, Tasks: taskInputs})
			if err != nil {
				writeAPIError(w, 409, "remediation_publication_failed", "corrective work could not be published")
				return
			}
			ids := make([]string, len(tasks))
			for i := range tasks {
				ids[i] = tasks[i].ID
			}
			updated, err := assessments.LinkRemediation(a.ID, in.ExpectedVersion, actor.UserID, assuranceassessments.Remediation{FindingEventID: finding.ID, ControlID: finding.ControlID, AffectedRevision: in.AffectedRevision, Deadline: in.Deadline, AcceptanceCriteria: in.AcceptanceCriteria, ProposalID: p.ID, TaskIDs: ids})
			if err != nil {
				writeAssessment(w, updated, err, 201)
				return
			}
			writeJSON(w, 201, map[string]any{"assessment": updated, "proposal": p, "tasks": tasks, "authority_note": "delivery continues through ordinary task, session, workspace, review, release, policy, and operational controls"})
		})
		mux.HandleFunc("POST /repositories/{id}/assurance-assessments/{assessment_id}/remediations/{remediation_id}/verification", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
			if !ok {
				return
			}
			a, err := assessments.Get(r.PathValue("assessment_id"))
			if err != nil || a.RepositoryID != r.PathValue("id") || !assessmentParty(a, actor.UserID) {
				writeAPIError(w, 404, "assessment_not_found", "assessment not found")
				return
			}
			role := "assessor"
			if actor.UserID == a.OwnerID {
				role = "owner"
			} else if !writeAssessorWindowError(w, a, time.Now().UTC()) {
				return
			}
			var in struct {
				ExpectedVersion    int      `json:"expected_version"`
				Verification       string   `json:"verification"`
				Disposition        string   `json:"disposition"`
				EvidencePackageIDs []string `json:"evidence_package_ids"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_verification", "fresh verification and disposition are required")
				return
			}
			workFound := false
			for _, work := range a.Remediations {
				if work.ID == r.PathValue("remediation_id") {
					workFound = true
					for _, taskID := range work.TaskIDs {
						task, e := proposalStore.GetTask(a.RepositoryID, work.ProposalID, taskID)
						if e != nil || task.Status != proposals.TaskCompleted || task.Contribution == nil || task.Contribution.Status != "merged" {
							writeAPIError(w, 409, "remediation_incomplete", "every ordered task must be merged before verification")
							return
						}
					}
				}
			}
			if !workFound {
				writeAPIError(w, 404, "remediation_not_found", "remediation not found")
				return
			}
			if in.Disposition == "accepted" {
				for _, packageID := range in.EvidencePackageIDs {
					p, e := evidence.GetPackage(packageID)
					if e != nil || p.RepositoryID != a.RepositoryID || p.ProgramID != a.ProgramID || p.ProgramVersion != a.ProgramVersion || len(p.Gaps) > 0 || len(p.Contradictions) > 0 {
						writeAPIError(w, 409, "verification_evidence_not_current", "accepted closure requires current gap-free exact-program evidence")
						return
					}
					matched := false
					for _, work := range a.Remediations {
						if work.ID == r.PathValue("remediation_id") && p.ControlID == work.ControlID && p.CollectedAt.After(work.CreatedAt) {
							matched = true
						}
					}
					if !matched {
						writeAPIError(w, 409, "verification_evidence_not_current", "verification evidence must cover the finding control and postdate corrective work")
						return
					}
				}
			}
			out, err := assessments.VerifyRemediation(a.ID, r.PathValue("remediation_id"), in.ExpectedVersion, actor.UserID, role, in.Verification, in.Disposition, in.EvidencePackageIDs)
			writeAssessment(w, out, err, 201)
		})
	}
	if releaseStore != nil {
		mux.HandleFunc("POST /repositories/{id}/assurance-statements", func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in assuranceassessments.Statement
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_statement", "an exact assurance statement is required")
				return
			}
			a, err := assessments.Get(in.AssessmentID)
			if err != nil || a.RepositoryID != r.PathValue("id") || a.OwnerID != actor.UserID {
				writeAPIError(w, 403, "statement_owner_required", "the assessment owner must publish the statement")
				return
			}
			rel, err := releaseStore.Get(a.RepositoryID, in.ReleaseID)
			if err != nil || rel.CommitID != in.ReleaseRevision {
				writeAPIError(w, 400, "invalid_statement_release", "an exact repository release is required")
				return
			}
			program, err := programs.Get(a.ProgramID)
			if err != nil || program.CurrentVersion != a.ProgramVersion {
				writeAPIError(w, 409, "assurance_drift", "the assessment program is no longer current")
				return
			}
			for _, audienceID := range in.Audience {
				if _, e := people.Get(audienceID); e != nil {
					writeAPIError(w, 400, "invalid_statement_audience", "every statement audience member must be an identified platform user")
					return
				}
			}
			revision := program.Revisions[a.ProgramVersion-1]
			for _, exceptionID := range in.ExceptionIDs {
				found := false
				for _, exception := range revision.Exceptions {
					if exception.ID == exceptionID && intersects(exception.ControlIDs, in.ControlIDs) {
						found = true
					}
				}
				if !found {
					writeAPIError(w, 400, "invalid_statement_exception", "exceptions must belong to the exact assurance program revision")
					return
				}
			}
			for _, control := range in.ControlIDs {
				if !hasID(a.Scope.ControlIDs, control) {
					writeAPIError(w, 400, "invalid_statement_control", "statements may include only assessed controls")
					return
				}
				for _, event := range a.Events {
					if event.Kind == "finding" && event.ControlID == control {
						closed := false
						for _, work := range a.Remediations {
							if work.FindingEventID == event.ID && work.State == "verified" && work.Disposition == "accepted" {
								closed = true
							}
						}
						if !closed {
							writeAPIError(w, 409, "open_assurance_finding", "all included findings require current accepted verification")
							return
						}
					}
				}
			}
			hashes := []string{}
			for _, pid := range a.EvidencePackageIDs {
				p, e := evidence.GetPackage(pid)
				if e == nil {
					hashes = append(hashes, p.ManifestHash)
				}
			}
			for _, work := range a.Remediations {
				if hasID(in.ControlIDs, work.ControlID) {
					for _, pid := range work.VerificationEvidencePackageIDs {
						p, e := evidence.GetPackage(pid)
						if e == nil {
							hashes = append(hashes, p.ManifestHash)
						}
					}
				}
			}
			sort.Strings(hashes)
			sum := sha256.Sum256([]byte(strings.Join(hashes, "\n")))
			in.RepositoryID, in.ProgramID, in.ProgramVersion, in.Scope, in.EvidenceDigest, in.IssuedBy = a.RepositoryID, a.ProgramID, a.ProgramVersion, a.Scope, hex.EncodeToString(sum[:]), actor.UserID
			out, err := assessments.CreateStatement(in)
			if err != nil {
				writeAPIError(w, 400, "invalid_statement", "statement could not be signed")
				return
			}
			writeJSON(w, 201, map[string]any{"statement": out, "status": "current", "signature_algorithm": "Ed25519", "signature_input": "SHA-256 of decoded payload"})
		})
		mux.HandleFunc("GET /repositories/{id}/assurance-statements/{statement_id}", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
			if !ok {
				return
			}
			v, err := assessments.GetStatement(r.PathValue("statement_id"))
			if err != nil || v.RepositoryID != r.PathValue("id") || !hasID(v.Audience, actor.UserID) {
				writeAPIError(w, 404, "statement_not_found", "statement not found")
				return
			}
			status := "current"
			reason := ""
			if v.RevokedAt != nil {
				status, reason = "revoked", v.RevocationReason
			} else if !time.Now().UTC().Before(v.ExpiresAt) {
				status, reason = "expired", "statement validity period ended"
			} else if a, e := assessments.Get(v.AssessmentID); e != nil {
				status, reason = "drifted", "assessment unavailable"
			} else if p, e := programs.Get(v.ProgramID); e != nil || p.CurrentVersion != v.ProgramVersion {
				status, reason = "drifted", "assurance program changed"
			} else {
				for _, event := range a.Events {
					if event.Kind == "finding" && hasID(v.ControlIDs, event.ControlID) && event.CreatedAt.After(v.IssuedAt) {
						status, reason = "reopened", "a later finding reopened an included control"
					}
				}
				for _, work := range a.Remediations {
					if hasID(v.ControlIDs, work.ControlID) && work.State != "verified" {
						status, reason = "reopened", "a finding was reopened"
					}
				}
				if status == "current" {
					revision := p.Revisions[v.ProgramVersion-1]
					for _, exceptionID := range v.ExceptionIDs {
						for _, exception := range revision.Exceptions {
							if exception.ID == exceptionID && !time.Now().UTC().Before(exception.ExpiresAt) {
								status, reason = "drifted", "a claimed exception expired"
							}
						}
					}
				}
			}
			writeJSON(w, 200, map[string]any{"statement": v, "status": status, "status_reason": reason, "evidence_disclosure": "digest only; source evidence remains governed by its original audience"})
		})
		mux.HandleFunc("POST /repositories/{id}/assurance-statements/{statement_id}/revocation", func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			v, err := assessments.GetStatement(r.PathValue("statement_id"))
			if err != nil || v.RepositoryID != r.PathValue("id") || v.IssuedBy != actor.UserID {
				writeAPIError(w, 403, "statement_revocation_forbidden", "only the issuer may revoke this statement")
				return
			}
			var in struct {
				Reason string `json:"reason"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_revocation", "a reason is required")
				return
			}
			out, err := assessments.RevokeStatement(v.ID, actor.UserID, in.Reason)
			if err != nil {
				writeAPIError(w, 400, "invalid_revocation", "statement could not be revoked")
				return
			}
			writeJSON(w, 201, map[string]any{"statement": out, "status": "revoked"})
		})
	}
}
func assessmentParty(a assuranceassessments.Assessment, id string) bool {
	return a.OwnerID == id || a.Assessor.UserID == id
}
func assessorWindowOpen(a assuranceassessments.Assessment, now time.Time) bool {
	return !now.Before(a.StartsAt) && now.Before(a.ExpiresAt)
}
func writeAssessorWindowError(w http.ResponseWriter, a assuranceassessments.Assessment, now time.Time) bool {
	if now.Before(a.StartsAt) {
		writeAPIError(w, 403, "assessment_access_not_started", "the assessor's bounded evidence access has not started")
		return false
	}
	if !now.Before(a.ExpiresAt) {
		writeAPIError(w, 403, "assessment_access_expired", "the assessor's bounded evidence access has expired")
		return false
	}
	return true
}
func hasID(xs []string, id string) bool {
	for _, x := range xs {
		if x == id {
			return true
		}
	}
	return false
}
func intersects(left, right []string) bool {
	for _, value := range left {
		if hasID(right, value) {
			return true
		}
	}
	return false
}
func assessmentScopeValid(r assuranceprograms.Revision, s assuranceassessments.Scope) bool {
	for _, id := range s.ControlIDs {
		found := false
		for _, c := range r.Controls {
			if c.ID == id {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	for _, id := range s.SystemIDs {
		found := false
		for _, x := range r.Scopes {
			if x.ID == id && (x.Kind == "repository" || x.Kind == "data_flow" || x.Kind == "infrastructure" || x.Kind == "environment") {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	for _, id := range s.ReleaseIDs {
		found := false
		for _, x := range r.Scopes {
			if x.ID == id && x.Kind == "release" {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func writeAssessment(w http.ResponseWriter, a assuranceassessments.Assessment, err error, status int) {
	if err == nil {
		writeJSON(w, status, a)
		return
	}
	switch {
	case errors.Is(err, assuranceassessments.ErrConflict):
		writeAPIError(w, 409, "assessment_version_conflict", "the assessment changed; reload before appending")
	case errors.Is(err, assuranceassessments.ErrExpired):
		writeAPIError(w, 403, "assessment_access_expired", "bounded assessment access has expired")
	case errors.Is(err, assuranceassessments.ErrNotStarted):
		writeAPIError(w, 403, "assessment_access_not_started", "bounded assessment access has not started")
	case errors.Is(err, assuranceassessments.ErrForbidden):
		writeAPIError(w, 403, "assessment_action_forbidden", "this party cannot perform that assessment action")
	default:
		writeAPIError(w, 400, "invalid_assessment", "the assessment record is invalid")
	}
}
