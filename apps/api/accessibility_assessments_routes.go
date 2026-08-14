package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilitycommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityreports"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerAccessibilityAssessmentRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, previewStore *previews.Store, reportStore *accessibilityreports.Store, commitmentStore *accessibilitycommitments.Store, proposalStore *proposals.Store, assessments *accessibilityassessments.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return actor, false, false
		}
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return actor, false, false
		}
		repo, err := catalog.GetByID(r.PathValue("id"))
		if err != nil {
			writeRepositoryError(w, err)
			return actor, false, false
		}
		participant := actor.UserID != "" && actor.AgentID == "" && actor.UserID == repo.OwnerID
		if !participant && actor.UserID != "" && actor.AgentID == "" {
			participant, _ = catalog.HasCollaborator(actor.UserID, repo.ID)
		}
		return actor, participant, true
	}
	mux.HandleFunc("POST /repositories/{id}/accessibility-assessments", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "accessibility_assessment_forbidden", "only a current repository participant may publish repository-defined checks")
			return
		}
		var in accessibilityassessments.Assessment
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a revision and complete accessibility checks are required")
			return
		}
		if in.PullRequestID != "" {
			if pulls == nil {
				writeAPIError(w, 503, "pull_requests_unavailable", "pull request evidence could not be verified")
				return
			}
			var out accessibilityassessments.Assessment
			err := pulls.WithSourceRevision(r.PathValue("id"), in.PullRequestID, in.Revision, func(_ pullrequests.PullRequest) error {
				var createErr error
				out, createErr = assessments.Create(r.PathValue("id"), actor.UserID, in)
				return createErr
			})
			if errors.Is(err, pullrequests.ErrSourceChanged) || errors.Is(err, pullrequests.ErrNotReady) {
				writeAPIError(w, 409, "accessibility_pull_changed", "the pull request changed while accessibility evidence was published")
				return
			}
			if errors.Is(err, pullrequests.ErrNotFound) || errors.Is(err, pullrequests.ErrInvalid) {
				writeAPIError(w, 400, "invalid_accessibility_pull_revision", "the pull request must exist in this repository at the assessment revision")
				return
			}
			writeAccessibilityAssessment(w, out, err, 201)
			return
		} else if !accessibilityRevisionIsVisible(git, r.PathValue("id"), in.Revision) {
			writeAPIError(w, 400, "invalid_accessibility_revision", "the assessment revision must be a commit reachable from a visible repository branch")
			return
		}
		out, err := assessments.Create(r.PathValue("id"), actor.UserID, in)
		writeAccessibilityAssessment(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/accessibility-assessments", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorize(w, r)
		if !ok {
			return
		}
		out, err := assessments.List(r.PathValue("id"), r.URL.Query().Get("revision"), r.URL.Query().Get("pull_request_id"))
		if err != nil {
			writeAccessibilityAssessment(w, accessibilityassessments.Assessment{}, err, 0)
			return
		}
		writeJSON(w, 200, map[string]any{"assessments": out})
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-assessments/{assessment_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant && actor.AgentID == "" {
			writeAPIError(w, 403, "accessibility_finding_forbidden", "only repository participants and repository-bound read-only agents may add findings")
			return
		}
		var in accessibilityassessments.Finding
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a cited accessibility finding is required")
			return
		}
		assessment, err := assessments.Get(r.PathValue("id"), r.PathValue("assessment_id"))
		if err != nil {
			writeAccessibilityAssessment(w, accessibilityassessments.Assessment{}, err, 0)
			return
		}
		kind, id := "human", actor.UserID
		if actor.AgentID != "" {
			kind, id = "agent", actor.AgentID
		}
		guardPullID := assessment.PullRequestID
		for _, citation := range in.Citations {
			if citation.Kind != "preview" || previewStore == nil {
				continue
			}
			preview, previewErr := previewStore.Find(r.PathValue("id"), citation.ResourceID)
			if previewErr != nil || (guardPullID != "" && guardPullID != preview.PullRequestID) {
				writeAPIError(w, 400, "invalid_accessibility_citation", "preview citations must resolve to the assessment's single guarded pull request")
				return
			}
			guardPullID = preview.PullRequestID
		}
		var out accessibilityassessments.Assessment
		persist := func(current *pullrequests.PullRequest) error {
			for _, citation := range in.Citations {
				if !accessibilityCitationResolves(r.PathValue("id"), assessment.Revision, citation, current, previewStore, reportStore) {
					return accessibilityassessments.ErrInvalid
				}
			}
			var addErr error
			out, addErr = assessments.AddFinding(r.PathValue("id"), r.PathValue("assessment_id"), kind, id, in)
			return addErr
		}
		if guardPullID != "" {
			if pulls == nil {
				writeAPIError(w, 503, "pull_requests_unavailable", "preview evidence freshness could not be verified")
				return
			}
			err = pulls.WithSourceRevision(r.PathValue("id"), guardPullID, assessment.Revision, func(current pullrequests.PullRequest) error { return persist(&current) })
		} else {
			err = persist(nil)
		}
		if errors.Is(err, pullrequests.ErrSourceChanged) || errors.Is(err, pullrequests.ErrNotReady) {
			writeAPIError(w, 409, "accessibility_citation_stale", "the pull request changed while cited accessibility evidence was published")
			return
		}
		writeAccessibilityAssessment(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-assessments/{assessment_id}/findings/{finding_id}/decision", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "accessibility_decision_forbidden", "only a current human repository participant may decide a finding")
			return
		}
		var in struct {
			Classification string `json:"classification"`
			Reason         string `json:"reason"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a classification and reason are required")
			return
		}
		out, err := assessments.Decide(r.PathValue("id"), r.PathValue("assessment_id"), r.PathValue("finding_id"), actor.UserID, in.Classification, in.Reason)
		writeAccessibilityAssessment(w, out, err, 200)
	})
	projectRepair := func(assessment accessibilityassessments.Assessment, finding accessibilityassessments.Finding) map[string]any {
		out := map[string]any{"assessment": assessment, "finding": finding, "proposal": nil, "task": nil, "pull_request": nil, "previews": []previews.Preview{}}
		if finding.Repair == nil || proposalStore == nil {
			return out
		}
		proposal, err := proposalStore.Get(assessment.RepositoryID, finding.Repair.ProposalID)
		if err != nil {
			return out
		}
		task, err := proposalStore.GetTask(assessment.RepositoryID, proposal.ID, finding.Repair.TaskID)
		if err != nil {
			return out
		}
		out["proposal"], out["task"] = proposal, task
		if task.Contribution != nil && pulls != nil {
			if pull, getErr := pulls.Get(assessment.RepositoryID, task.Contribution.PullRequestID); getErr == nil {
				out["pull_request"] = pull
				if previewStore != nil {
					if values, listErr := previewStore.List(assessment.RepositoryID, pull.ID, pull.SourceCommitID); listErr == nil {
						out["previews"] = values
					}
				}
			}
		}
		return out
	}
	mux.HandleFunc("GET /repositories/{id}/accessibility-assessments/{assessment_id}/findings/{finding_id}/repair", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorize(w, r)
		if !ok {
			return
		}
		assessment, err := assessments.Get(r.PathValue("id"), r.PathValue("assessment_id"))
		if err != nil {
			writeAccessibilityAssessment(w, assessment, err, 0)
			return
		}
		for _, finding := range assessment.Findings {
			if finding.ID == r.PathValue("finding_id") {
				if finding.Repair == nil {
					writeAPIError(w, 404, "accessibility_repair_not_found", "finding has no governed repair")
					return
				}
				writeJSON(w, 200, projectRepair(assessment, finding))
				return
			}
		}
		writeAPIError(w, 404, "accessibility_finding_not_found", "accessibility finding not found")
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-assessments/{assessment_id}/findings/{finding_id}/repair", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "accessibility_repair_forbidden", "only a current human repository participant may start a repair")
			return
		}
		if commitmentStore == nil || proposalStore == nil {
			writeAPIError(w, 503, "accessibility_repair_unavailable", "governed accessibility repair is unavailable")
			return
		}
		var in struct {
			CommitmentID       string   `json:"commitment_id"`
			CommitmentVersion  int      `json:"commitment_version"`
			AcceptanceCriteria []string `json:"acceptance_criteria"`
			ComponentGuidance  []string `json:"component_guidance"`
			AssigneeType       string   `json:"assignee_type"`
			AssigneeID         string   `json:"assignee_id"`
		}
		if decodeJSON(r, &in) != nil || (in.AssigneeType != "human" && in.AssigneeType != "agent") || len(in.AcceptanceCriteria) == 0 || len(in.AcceptanceCriteria) > 20 || len(in.ComponentGuidance) == 0 || len(in.ComponentGuidance) > 20 {
			writeAPIError(w, 422, "invalid_accessibility_repair", "an exact commitment, acceptance criteria, component guidance, and owner are required")
			return
		}
		for _, values := range [][]string{in.AcceptanceCriteria, in.ComponentGuidance} {
			for i := range values {
				values[i] = strings.TrimSpace(values[i])
				if values[i] == "" || len(values[i]) > 1000 {
					writeAPIError(w, 422, "invalid_accessibility_repair", "acceptance criteria and component guidance must be bounded")
					return
				}
			}
		}
		if in.AssigneeType == "human" {
			if strings.TrimSpace(in.AssigneeID) == "" {
				in.AssigneeID = actor.UserID
			}
			repo, _ := catalog.GetByID(r.PathValue("id"))
			collaborator, _ := catalog.HasCollaborator(in.AssigneeID, repo.ID)
			if repo.OwnerID != in.AssigneeID && !collaborator {
				writeAPIError(w, 422, "invalid_accessibility_assignee", "human owner must already participate in the repository")
				return
			}
		}
		commitment, err := commitmentStore.Get(strings.TrimSpace(in.CommitmentID))
		if err != nil || commitment.RepositoryID != r.PathValue("id") || in.CommitmentVersion < 1 || in.CommitmentVersion > len(commitment.Revisions) {
			writeAPIError(w, 422, "invalid_accessibility_commitment", "the selected commitment revision must belong to this repository")
			return
		}
		commitmentRevision := commitment.Revisions[in.CommitmentVersion-1]
		assessment, err := assessments.Get(r.PathValue("id"), r.PathValue("assessment_id"))
		if err != nil {
			writeAccessibilityAssessment(w, assessment, err, 0)
			return
		}
		var finding *accessibilityassessments.Finding
		for i := range assessment.Findings {
			if assessment.Findings[i].ID == r.PathValue("finding_id") {
				finding = &assessment.Findings[i]
				break
			}
		}
		if finding == nil {
			writeAPIError(w, 404, "accessibility_finding_not_found", "accessibility finding not found")
			return
		}
		if finding.InvalidatedAt != nil || finding.Decision == nil || finding.Decision.Classification != "accepted" {
			writeAPIError(w, 409, "accessibility_finding_not_confirmed", "only a current accepted finding can enter implementation")
			return
		}
		items := []proposals.ReasoningItem{{ID: finding.ID, Kind: "accessibility_finding", Summary: finding.Title + ": " + finding.Detail, Status: "accepted"}, {ID: commitment.ID + "-v" + strconv.Itoa(in.CommitmentVersion), Kind: "accessibility_commitment", Summary: commitmentRevision.Title + ": " + commitmentRevision.Summary, Status: "required"}}
		evidence := []accessibilityassessments.RepairEvidence{}
		for index, citation := range finding.Citations {
			var currentPull *pullrequests.PullRequest
			if citation.Kind == "preview" && previewStore != nil && pulls != nil {
				if preview, findErr := previewStore.Find(assessment.RepositoryID, citation.ResourceID); findErr == nil {
					if pull, getErr := pulls.Get(assessment.RepositoryID, preview.PullRequestID); getErr == nil {
						currentPull = &pull
					}
				}
			}
			if !accessibilityCitationResolves(assessment.RepositoryID, assessment.Revision, citation, currentPull, previewStore, reportStore) {
				writeAPIError(w, 409, "accessibility_evidence_changed", "the permitted reproduction evidence is no longer revision-exact")
				return
			}
			summary := accessibilityRepairEvidenceSummary(assessment.RepositoryID, citation, previewStore, reportStore)
			evidence = append(evidence, accessibilityassessments.RepairEvidence{Kind: citation.Kind, ResourceID: citation.ResourceID, EvidenceRef: citation.EvidenceRef, Summary: summary})
			items = append(items, proposals.ReasoningItem{ID: "evidence-" + strconv.Itoa(index+1), Kind: "permitted_reproduction_evidence", Summary: summary, Status: "confirmed"})
		}
		for index, guidance := range in.ComponentGuidance {
			items = append(items, proposals.ReasoningItem{ID: "component-guidance-" + strconv.Itoa(index+1), Kind: "component_guidance", Summary: guidance, Status: "required"})
		}
		criteria := strings.Join(in.AcceptanceCriteria, "\n- ")
		guidance := strings.Join(in.ComponentGuidance, "\n- ")
		origin := proposals.ReasoningOrigin{AssessmentID: assessment.ID, AssessmentVersion: 1, AccessibilityFindingID: finding.ID, AccessibilityCommitmentID: commitment.ID, AccessibilityCommitmentVersion: in.CommitmentVersion, Revision: assessment.Revision, SelectedItemIDs: []string{finding.ID}, Items: items, AnalysisStatus: "accessibility_repair"}
		proposal, tasks, createErr := proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: assessment.RepositoryID, ActorID: actor.UserID, Title: "Accessible repair: " + finding.Title, Body: "Governed repair for accepted accessibility finding " + finding.ID + ". The affected user remains the author of their evidence; this implementation does not speak for them.\n\nAcceptance criteria:\n- " + criteria + "\n\nComponent guidance:\n- " + guidance + "\n\nPull requests must document design and code changes, interaction and content tradeoffs, and attach an exact-revision preview.", Origin: origin, Tasks: []proposals.ImplementationTaskInput{{Title: "Repair " + finding.Title, Outcome: "Meet every acceptance criterion at the exact repair revision while preserving contributor authorship and ordinary review.", Risk: finding.Severity + " accessibility barrier at " + assessment.Revision, VerificationPlan: criteria + "\n\nPreview the interaction and record design, code, interaction, and content tradeoffs.", AssigneeType: in.AssigneeType, AssigneeID: strings.TrimSpace(in.AssigneeID)}}})
		if createErr != nil && !errors.Is(createErr, proposals.ErrDurabilityUncertain) {
			if errors.Is(createErr, proposals.ErrImplementationConflict) {
				writeAPIError(w, 409, "accessibility_repair_changed", "the frozen repair handoff differs from the existing work")
				return
			}
			writeAPIError(w, 500, "accessibility_repair_failed", "governed repair could not be created")
			return
		}
		repair := accessibilityassessments.Repair{ProposalID: proposal.ID, TaskID: tasks[0].ID, BaseRevision: assessment.Revision, AcceptanceCriteria: append([]string(nil), in.AcceptanceCriteria...), CommitmentID: commitment.ID, CommitmentVersion: in.CommitmentVersion, CommitmentTitle: commitmentRevision.Title, ComponentGuidance: append([]string(nil), in.ComponentGuidance...), PermittedEvidence: evidence}
		updated, attachErr := assessments.AttachRepair(assessment.RepositoryID, assessment.ID, finding.ID, actor.UserID, repair)
		if attachErr != nil {
			w.Header().Set("Location", "/proposals/"+assessment.RepositoryID+"/"+proposal.ID)
			writeJSON(w, 202, map[string]any{"assessment": assessment, "finding": finding, "proposal": proposal, "task": tasks[0], "recovery_pending": true})
			return
		}
		for _, value := range updated.Findings {
			if value.ID == finding.ID {
				w.Header().Set("Location", "/repositories/"+assessment.RepositoryID+"/accessibility-assessments/"+assessment.ID+"/findings/"+finding.ID+"/repair")
				writeJSON(w, 201, projectRepair(updated, value))
				return
			}
		}
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-assessments/{assessment_id}/invalidate", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "accessibility_invalidation_forbidden", "only a current repository participant may invalidate affected evidence")
			return
		}
		var in struct {
			SourceLocations []string `json:"source_locations"`
			JourneyIDs      []string `json:"journey_ids"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "changed source locations or journeys are required")
			return
		}
		out, err := assessments.Invalidate(r.PathValue("id"), r.PathValue("assessment_id"), actor.UserID, in.SourceLocations, in.JourneyIDs)
		writeAccessibilityAssessment(w, out, err, 200)
	})
}

func accessibilityRepairEvidenceSummary(repositoryID string, citation accessibilityassessments.Citation, previewStore *previews.Store, reportStore *accessibilityreports.Store) string {
	if citation.Kind == "preview" && previewStore != nil {
		if preview, err := previewStore.Find(repositoryID, citation.ResourceID); err == nil {
			for _, finding := range preview.Findings {
				for _, artifact := range finding.Evidence {
					if citation.EvidenceRef == "artifact://"+artifact.ID {
						return "Preview " + finding.Route + ": " + finding.Title + ". " + finding.Description + " Steps: " + strings.Join(finding.ReproductionSteps, "; ") + ". Redacted artifact: " + artifact.Name + " (" + citation.EvidenceRef + ")"
					}
				}
			}
		}
	}
	if citation.Kind == "reproduction" && reportStore != nil {
		if reports, err := reportStore.List(repositoryID); err == nil {
			for _, report := range reports {
				for _, attempt := range report.Attempts {
					if attempt.ID != citation.ResourceID {
						continue
					}
					for _, artifact := range attempt.Evidence {
						if artifact.ContentRef == citation.EvidenceRef {
							return "Expected: " + report.ExpectedOutcome + ". Steps: " + strings.Join(report.Steps, "; ") + ". Reproduction " + attempt.Outcome + ": " + attempt.Notes + ". Redacted " + artifact.Kind + ": " + artifact.Description + " (" + citation.EvidenceRef + ")"
						}
					}
				}
			}
		}
	}
	return citation.Kind + " evidence at " + citation.EvidenceRef
}

func accessibilityRevisionIsVisible(git *storage.Store, repositoryID, revision string) bool {
	if git == nil || len(revision) != 40 || revision != strings.ToLower(revision) {
		return false
	}
	repository, err := git.Open(repositoryID)
	if err != nil {
		return false
	}
	if _, err = repository.ReadCommit(storage.ObjectID(revision)); err != nil {
		return false
	}
	refs, err := repository.ListReferences()
	if err != nil {
		return false
	}
	for _, ref := range refs {
		if !strings.HasPrefix(ref.Name, "refs/heads/") || strings.HasPrefix(ref.Name, "refs/heads/vivarium-security/") {
			continue
		}
		ancestry, ancestryErr := repository.ListCommitAncestry(storage.ObjectID(ref.Target))
		if ancestryErr != nil {
			continue
		}
		for _, commit := range ancestry {
			if string(commit.ID) == revision {
				return true
			}
		}
	}
	return false
}

func accessibilityCitationResolves(repositoryID, revision string, citation accessibilityassessments.Citation, currentPull *pullrequests.PullRequest, previewStore *previews.Store, reportStore *accessibilityreports.Store) bool {
	switch citation.Kind {
	case "preview":
		if previewStore == nil {
			return false
		}
		preview, err := previewStore.Find(repositoryID, citation.ResourceID)
		if err != nil || preview.Revision != revision || currentPull == nil || currentPull.ID != preview.PullRequestID {
			return false
		}
		if !accessibilityPreviewMatchesCurrentRevision(preview, currentPull.SourceCommitID) {
			return false
		}
		return accessibilityPreviewArtifactResolves(preview, citation.EvidenceRef)
	case "reproduction":
		if reportStore == nil {
			return false
		}
		reports, err := reportStore.List(repositoryID)
		if err != nil {
			return false
		}
		for _, report := range reports {
			if report.Target.Revision != revision {
				continue
			}
			for _, attempt := range report.Attempts {
				if attempt.ID != citation.ResourceID {
					continue
				}
				for _, evidence := range attempt.Evidence {
					if evidence.ContentRef == citation.EvidenceRef && evidence.Redacted {
						return true
					}
				}
			}
		}
	}
	return false
}

func accessibilityPreviewMatchesCurrentRevision(preview previews.Preview, currentRevision string) bool {
	return currentRevision != "" && preview.Revision == currentRevision
}

func accessibilityPreviewArtifactResolves(preview previews.Preview, evidenceRef string) bool {
	for _, finding := range preview.Findings {
		for _, evidence := range finding.Evidence {
			if evidenceRef == "artifact://"+evidence.ID {
				return true
			}
		}
	}
	return false
}

func writeAccessibilityAssessment(w http.ResponseWriter, out accessibilityassessments.Assessment, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, accessibilityassessments.ErrNotFound):
		writeAPIError(w, 404, "accessibility_assessment_not_found", "accessibility assessment not found")
	case errors.Is(err, accessibilityassessments.ErrInvalid):
		writeAPIError(w, 400, "invalid_accessibility_assessment", "checks and findings must be bounded, revision-exact, cited, and use supported classifications")
	default:
		writeAPIError(w, 500, "accessibility_assessments_unavailable", "accessibility assessments could not be persisted")
	}
}
