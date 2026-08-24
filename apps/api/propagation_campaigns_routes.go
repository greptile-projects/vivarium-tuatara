package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/propagationcampaigns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerPropagationCampaignRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, campaigns *propagationcampaigns.Store, pulls *pullrequests.Store, proposalStore *proposals.Store) {
	actorID := func(c auth.Credential) string {
		if c.AgentID != "" {
			return c.AgentID
		}
		return c.UserID
	}
	project := func(v propagationcampaigns.Campaign, c auth.Credential) propagationcampaigns.Campaign {
		principal := c.UserID
		for i := range v.Targets {
			t := &v.Targets[i]
			t.State = "pending"
			t.Diagnostic = ""
			t.Authority = "campaign records intent only; delivery requires target repository authority"
			if t.Kind == "package" {
				t.State = "unknown"
				t.Diagnostic = "package release-line equivalence requires package authority and evidence"
				continue
			}
			repo, e := catalog.GetByID(t.RepositoryID)
			if e != nil {
				t.State = "unsupported"
				t.Diagnostic = "target repository is unknown or unsupported"
				continue
			}
			collab, _ := catalog.HasCollaborator(principal, t.RepositoryID)
			if repo.OwnerID != principal && !collab {
				t.State = "inaccessible"
				t.Diagnostic = "target repository is not accessible to this collaborator"
				continue
			}
			ownersOK := true
			for _, owner := range t.OwnerIDs {
				ok, _ := catalog.HasCollaborator(owner, t.RepositoryID)
				if owner != repo.OwnerID && !ok {
					ownersOK = false
				}
			}
			if !ownersOK {
				t.State = "unknown"
				t.Diagnostic = "one or more target owners are not current repository participants"
				continue
			}
			gr, e := git.Open(t.RepositoryID)
			if e != nil {
				t.State = "unsupported"
				t.Diagnostic = "target Git history is unavailable"
				continue
			}
			ref := "refs/heads/" + strings.TrimPrefix(t.ReleaseLine, "refs/heads/")
			tip, e := exec.Command("git", "--git-dir="+gr.Path(), "rev-parse", "--verify", ref+"^{commit}").Output()
			if e != nil {
				t.State = "unknown"
				t.Diagnostic = "target release line does not resolve"
				continue
			}
			tree := func(path, rev string) string {
				b, e := exec.Command("git", "--git-dir="+path, "rev-parse", rev+"^{tree}").Output()
				if e != nil {
					return ""
				}
				return strings.TrimSpace(string(b))
			}
			sourceRepo, e := git.Open(v.Source.RepositoryID)
			if e == nil && tree(sourceRepo.Path(), v.Source.Commits[len(v.Source.Commits)-1]) == tree(gr.Path(), strings.TrimSpace(string(tip))) {
				t.State = "already_equivalent"
				t.Diagnostic = "target tip has the same Git tree as the proven source outcome"
			}
		}
		for i := range v.Assessments {
			a := &v.Assessments[i]
			for _, t := range v.Targets {
				if t.ID != a.TargetID || t.Kind != "repository" {
					continue
				}
				gr, e := git.Open(t.RepositoryID)
				if e != nil {
					a.Invalidated = true
					a.InvalidationReason = "target history is unavailable"
					continue
				}
				tip, e := gitOutput(gr.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(t.ReleaseLine, "refs/heads/")+"^{commit}")
				if e != nil || tip != a.TargetRevision {
					a.Invalidated = true
					a.InvalidationReason = "target release line moved; only this target assessment requires recomparison"
				}
			}
		}
		visibleAssessments := make([]propagationcampaigns.Assessment, 0, len(v.Assessments))
		for _, assessment := range v.Assessments {
			visible := false
			for _, target := range v.Targets {
				if target.ID == assessment.TargetID && target.State != "inaccessible" && target.State != "unsupported" {
					visible = true
				}
			}
			if visible {
				visibleAssessments = append(visibleAssessments, assessment)
			}
		}
		v.Assessments = visibleAssessments
		visibleContributions := make([]propagationcampaigns.Contribution, 0, len(v.Contributions))
		for _, contribution := range v.Contributions {
			for _, target := range v.Targets {
				if target.ID == contribution.TargetID && target.State != "inaccessible" && target.State != "unsupported" {
					visibleContributions = append(visibleContributions, contribution)
				}
			}
		}
		v.Contributions = visibleContributions
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/propagation-campaigns", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, e := campaigns.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "propagation_campaigns_unavailable", "propagation campaigns could not be read")
			return
		}
		for i := range values {
			values[i] = project(values[i], c)
		}
		writeJSON(w, 200, map[string]any{"propagation_campaigns": values})
	})
	mux.HandleFunc("GET /repositories/{id}/propagation-campaigns/{campaign_id}", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := campaigns.Get(r.PathValue("id"), r.PathValue("campaign_id"))
		if e != nil {
			writeAPIError(w, 404, "propagation_campaign_not_found", "propagation campaign not found")
			return
		}
		writeJSON(w, 200, project(v, c))
	})
	mux.HandleFunc("POST /repositories/{id}/propagation-campaigns", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in propagationcampaigns.Campaign
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a propagation campaign is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		in.Source.RepositoryID = in.RepositoryID
		for _, rev := range in.Source.Commits {
			if len(rev) != 40 || !catalog.HasCommit(in.RepositoryID, rev) {
				writeAPIError(w, 422, "propagation_source_revision_missing", "every source commit must resolve in the source repository")
				return
			}
		}
		if in.Source.Kind == "merged_pull" {
			p, e := pulls.Get(in.RepositoryID, in.Source.ResourceID)
			if e != nil || p.Status != pullrequests.Merged || p.MergeCommitID == nil || *p.MergeCommitID != in.Source.Commits[len(in.Source.Commits)-1] {
				writeAPIError(w, 422, "propagation_source_invalid", "merged pull provenance must resolve to its exact merge commit")
				return
			}
		}
		clean := in
		clean.ID = ""
		clean.RequestDigest = ""
		clean.CreatedBy = ""
		clean.CreatedAt = clean.CreatedAt.UTC()
		for i := range clean.Targets {
			clean.Targets[i].State = ""
			clean.Targets[i].Diagnostic = ""
			clean.Targets[i].Authority = ""
		}
		b, _ := json.Marshal(clean)
		sum := sha256.Sum256(b)
		out, e := campaigns.Create(in, actorID(c), hex.EncodeToString(sum[:]))
		if errors.Is(e, propagationcampaigns.ErrConflict) {
			writeAPIError(w, 409, "propagation_request_conflict", "request_id was already used for a different campaign")
			return
		}
		if errors.Is(e, propagationcampaigns.ErrInvalid) {
			writeAPIError(w, 422, "propagation_campaign_invalid", "campaign intent, criteria, deadlines, sequencing, owners, targets, and completion policy are required")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "propagation_campaign_unavailable", "propagation campaign could not be published")
			return
		}
		writeJSON(w, 201, project(out, c))
	})
	mux.HandleFunc("POST /repositories/{id}/propagation-campaigns/{campaign_id}/targets/{target_id}/assessments", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		campaign, e := campaigns.Get(r.PathValue("id"), r.PathValue("campaign_id"))
		if e != nil {
			writeAPIError(w, 404, "propagation_campaign_not_found", "propagation campaign not found")
			return
		}
		var target *propagationcampaigns.Target
		for i := range campaign.Targets {
			if campaign.Targets[i].ID == r.PathValue("target_id") {
				target = &campaign.Targets[i]
			}
		}
		if target == nil || target.Kind != "repository" {
			writeAPIError(w, 422, "propagation_target_not_assessable", "only a permitted repository target can be assessed")
			return
		}
		targetRepo, e := catalog.GetByID(target.RepositoryID)
		if e != nil {
			writeAPIError(w, 422, "propagation_target_unavailable", "target repository is unavailable")
			return
		}
		allowed, _ := catalog.HasCollaborator(c.UserID, target.RepositoryID)
		if targetRepo.OwnerID != c.UserID && !allowed {
			writeAPIError(w, 403, "propagation_target_forbidden", "target repository access is required to compare it")
			return
		}
		assessment, e := comparePropagationTarget(git, campaign, *target)
		if e != nil {
			writeAPIError(w, 422, "propagation_comparison_unavailable", "exact source and target histories could not be compared")
			return
		}
		updated, out, e := campaigns.CreateAssessment(campaign.RepositoryID, campaign.ID, actorID(c), assessment)
		if e != nil {
			writeAPIError(w, 500, "propagation_assessment_unavailable", "target assessment could not be retained")
			return
		}
		writeJSON(w, 201, map[string]any{"campaign": project(updated, c), "assessment": out})
	})
	mux.HandleFunc("POST /repositories/{id}/propagation-campaigns/{campaign_id}/assessments/{assessment_id}/entries", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                             `json:"expected_version"`
			Kind            string                          `json:"kind"`
			Body            string                          `json:"body"`
			Citations       []propagationcampaigns.Citation `json:"citations"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a cited assessment entry is required")
			return
		}
		campaign, e := campaigns.Get(r.PathValue("id"), r.PathValue("campaign_id"))
		if e != nil {
			writeAPIError(w, 404, "propagation_campaign_not_found", "propagation campaign not found")
			return
		}
		var assessment *propagationcampaigns.Assessment
		var target *propagationcampaigns.Target
		for i := range campaign.Assessments {
			if campaign.Assessments[i].ID == r.PathValue("assessment_id") {
				assessment = &campaign.Assessments[i]
			}
		}
		if assessment != nil {
			for i := range campaign.Targets {
				if campaign.Targets[i].ID == assessment.TargetID {
					target = &campaign.Targets[i]
				}
			}
		}
		if assessment == nil || target == nil {
			writeAPIError(w, 404, "propagation_assessment_not_found", "target assessment not found")
			return
		}
		targetRepo, targetErr := catalog.GetByID(target.RepositoryID)
		targetAccess, _ := catalog.HasCollaborator(c.UserID, target.RepositoryID)
		if targetErr != nil || (targetRepo.OwnerID != c.UserID && !targetAccess) {
			writeAPIError(w, 403, "propagation_target_forbidden", "current target repository access is required to contribute assessment evidence")
			return
		}
		if in.Kind == "owner_acknowledgement" {
			isOwner := false
			for _, id := range target.OwnerIDs {
				if id == c.UserID {
					isOwner = true
				}
			}
			if c.AgentID != "" || !isOwner {
				writeAPIError(w, 403, "propagation_acknowledgement_forbidden", "only a named human target owner may acknowledge assumptions")
				return
			}
		}
		for _, citation := range in.Citations {
			if citation.Revision != "" && citation.Revision != assessment.TargetRevision && citation.Revision != assessment.SourceRevision && citation.Revision != assessment.SourceBaseRevision {
				writeAPIError(w, 422, "propagation_citation_invalid", "revision citations must bind the frozen source or target comparison")
				return
			}
		}
		kind := "human"
		if c.AgentID != "" {
			kind = "read_only_agent"
		}
		updated, out, e := campaigns.AddAssessmentEntry(campaign.RepositoryID, campaign.ID, assessment.ID, actorID(c), kind, in.ExpectedVersion, propagationcampaigns.AssessmentEntry{Kind: in.Kind, Body: in.Body, Citations: in.Citations})
		if errors.Is(e, propagationcampaigns.ErrVersion) {
			writeAPIError(w, 409, "propagation_assessment_changed", "reload the assessment before appending")
			return
		}
		if errors.Is(e, propagationcampaigns.ErrInvalid) {
			writeAPIError(w, 422, "propagation_entry_invalid", "findings, risks, uncertainty, and acknowledgements require bounded citations")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "propagation_assessment_unavailable", "assessment entry could not be retained")
			return
		}
		writeJSON(w, 201, map[string]any{"campaign": project(updated, c), "assessment": out})
	})
	mux.HandleFunc("POST /repositories/{id}/propagation-campaigns/{campaign_id}/targets/{target_id}/contributions", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if c.AgentID != "" {
			writeAPIError(w, 403, "propagation_contribution_forbidden", "a human target maintainer must publish contribution work")
			return
		}
		var in struct {
			AssessmentID    string   `json:"assessment_id"`
			ExpectedVersion int      `json:"expected_version"`
			Application     string   `json:"application"`
			Deviation       string   `json:"deviation"`
			Topology        string   `json:"topology"`
			Constraints     []string `json:"constraints"`
			Tasks           []struct {
				Title        string `json:"title"`
				Outcome      string `json:"outcome"`
				AssigneeType string `json:"assignee_type"`
				AssigneeID   string `json:"assignee_id"`
			} `json:"tasks"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a target contribution plan is required")
			return
		}
		campaign, e := campaigns.Get(r.PathValue("id"), r.PathValue("campaign_id"))
		if e != nil {
			writeAPIError(w, 404, "propagation_campaign_not_found", "propagation campaign not found")
			return
		}
		var target *propagationcampaigns.Target
		var assessment *propagationcampaigns.Assessment
		for i := range campaign.Targets {
			if campaign.Targets[i].ID == r.PathValue("target_id") {
				target = &campaign.Targets[i]
			}
		}
		for i := range campaign.Assessments {
			if campaign.Assessments[i].ID == in.AssessmentID && target != nil && campaign.Assessments[i].TargetID == target.ID {
				assessment = &campaign.Assessments[i]
			}
		}
		if target == nil || target.Kind != "repository" || assessment == nil {
			writeAPIError(w, 404, "propagation_assessment_not_found", "a target assessment is required")
			return
		}
		targetRepo, e := catalog.GetByID(target.RepositoryID)
		collab, _ := catalog.HasCollaborator(c.UserID, target.RepositoryID)
		if e != nil || (targetRepo.OwnerID != c.UserID && !collab) {
			writeAPIError(w, 403, "propagation_target_forbidden", "current target repository write authority is required")
			return
		}
		currentTip, e := git.Open(target.RepositoryID)
		if e != nil {
			writeAPIError(w, 422, "propagation_target_unavailable", "target repository is unavailable")
			return
		}
		tip, e := gitOutput(currentTip.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(target.ReleaseLine, "refs/heads/")+"^{commit}")
		if e != nil || tip != assessment.TargetRevision || assessment.Version != in.ExpectedVersion {
			writeAPIError(w, 409, "propagation_assessment_changed", "reassess the current target before publishing work")
			return
		}
		if assessment.Classification == "already_satisfied" || assessment.Classification == "not_applicable" {
			writeAPIError(w, 422, "propagation_contribution_unnecessary", "this assessment does not support implementation work")
			return
		}
		if in.Application == "direct" && assessment.Classification != "directly_applicable" {
			writeAPIError(w, 422, "propagation_adaptation_required", "non-direct assessments require an explained adaptation")
			return
		}
		if !map[string]bool{"direct": true, "adapted": true}[in.Application] || (in.Application == "adapted" && strings.TrimSpace(in.Deviation) == "") || !map[string]bool{"local_branch": true, "fork": true, "federated": true}[in.Topology] {
			writeAPIError(w, 422, "propagation_contribution_invalid", "application, explained deviations, and an ordinary contribution topology are required")
			return
		}
		if len(in.Tasks) == 0 {
			writeAPIError(w, 422, "propagation_contribution_invalid", "at least one owned task is required")
			return
		}
		criteria := append([]string{}, campaign.AcceptanceCriteria...)
		criteria = append(criteria, target.AcceptanceCriteria...)
		items := []proposals.ReasoningItem{{ID: "intent", Kind: "source_intent", Summary: campaign.Intent, Status: "accepted"}, {ID: "assessment", Kind: "target_assessment", Summary: assessment.Classification, Status: "current"}}
		for _, entry := range assessment.Entries {
			items = append(items, proposals.ReasoningItem{ID: entry.ID, Kind: entry.Kind, Summary: entry.Body, Status: "retained"})
		}
		origin := proposals.ReasoningOrigin{PropagationCampaignID: campaign.ID, PropagationTargetID: target.ID, PropagationAssessmentID: assessment.ID, AssessmentVersion: assessment.Version, Revision: assessment.TargetRevision, SelectedItemIDs: []string{"intent", "assessment"}, Items: items, AnalysisStatus: assessment.Classification}
		proposalTasks := make([]proposals.ImplementationTaskInput, len(in.Tasks))
		for i, task := range in.Tasks {
			proposalTasks[i] = proposals.ImplementationTaskInput{Title: task.Title, Outcome: task.Outcome, Risk: strings.Join(in.Constraints, "\n"), VerificationPlan: "Preserve source intent and satisfy:\n- " + strings.Join(criteria, "\n- "), AssigneeType: task.AssigneeType, AssigneeID: task.AssigneeID, DependsOnPrevious: i > 0}
		}
		body := "Target-maintainer contribution for propagation campaign " + campaign.ID + ".\n\nSource rationale:\n" + campaign.Intent + "\n\nRelevant source commits (retain original authorship for direct application):\n- " + strings.Join(campaign.Source.Commits, "\n- ") + "\n\nLocal assessment: " + assessment.Classification + " at " + assessment.TargetRevision + "."
		if strings.TrimSpace(in.Deviation) != "" {
			body += "\n\nAdaptation and deviations:\n" + strings.TrimSpace(in.Deviation)
		}
		if len(in.Constraints) > 0 {
			body += "\n\nLocal constraints (restricted or embargoed context remains outside this record):\n- " + strings.Join(in.Constraints, "\n- ")
		}
		proposal, tasks, e := proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: target.RepositoryID, ActorID: c.UserID, Title: "Propagate: " + campaign.Title, Body: body, Origin: origin, Tasks: proposalTasks})
		if errors.Is(e, proposals.ErrImplementationConflict) {
			writeAPIError(w, 409, "propagation_contribution_conflict", "this target contribution was already published with different work")
			return
		}
		if e != nil && !errors.Is(e, proposals.ErrDurabilityUncertain) {
			writeAPIError(w, 422, "propagation_contribution_invalid", "ordered owned tasks and local adaptation details are required")
			return
		}
		taskIDs := make([]string, len(tasks))
		for i := range tasks {
			taskIDs[i] = tasks[i].ID
		}
		updated, contribution, linkErr := campaigns.LinkContribution(campaign.RepositoryID, campaign.ID, c.UserID, propagationcampaigns.Contribution{TargetID: target.ID, AssessmentID: assessment.ID, AssessmentVersion: assessment.Version, TargetRevision: assessment.TargetRevision, Application: in.Application, Deviation: strings.TrimSpace(in.Deviation), Topology: in.Topology, Constraints: in.Constraints, ProposalID: proposal.ID, TaskIDs: taskIDs})
		if linkErr != nil {
			writeAPIError(w, 409, "propagation_contribution_conflict", "target contribution publication could not be reconciled")
			return
		}
		writeJSON(w, 201, map[string]any{"campaign": project(updated, c), "contribution": contribution, "proposal": proposal, "tasks": tasks})
	})
}

func gitOutput(path string, args ...string) (string, error) {
	all := append([]string{"--git-dir=" + path}, args...)
	b, e := exec.Command("git", all...).Output()
	return strings.TrimSpace(string(b)), e
}
func comparePropagationTarget(gitStore *storage.Store, campaign propagationcampaigns.Campaign, target propagationcampaigns.Target) (propagationcampaigns.Assessment, error) {
	source, e := gitStore.Open(campaign.Source.RepositoryID)
	if e != nil {
		return propagationcampaigns.Assessment{}, e
	}
	destination, e := gitStore.Open(target.RepositoryID)
	if e != nil {
		return propagationcampaigns.Assessment{}, e
	}
	head := campaign.Source.Commits[len(campaign.Source.Commits)-1]
	tip, e := gitOutput(destination.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(target.ReleaseLine, "refs/heads/")+"^{commit}")
	if e != nil {
		return propagationcampaigns.Assessment{}, e
	}
	base, _ := gitOutput(source.Path(), "rev-parse", campaign.Source.Commits[0]+"^1")
	pathsText, e := gitOutput(source.Path(), "diff", "--name-only", base, head)
	if e != nil {
		return propagationcampaigns.Assessment{}, e
	}
	paths := strings.Fields(pathsText)
	sort.Strings(paths)
	before, after, divergent, missing := 0, 0, 0, 0
	for _, p := range paths {
		sb, _ := gitOutput(source.Path(), "rev-parse", base+":"+p)
		sa, _ := gitOutput(source.Path(), "rev-parse", head+":"+p)
		tb, _ := gitOutput(destination.Path(), "rev-parse", tip+":"+p)
		switch {
		case tb != "" && tb == sa:
			after++
		case tb == sb:
			before++
		case tb == "" && sa == "":
			after++
		case tb == "" && sb != "":
			missing++
		default:
			divergent++
		}
	}
	classification := "adaptation_required"
	summary := "target structure differs from the source change and needs an explicit adaptation"
	if len(paths) == 0 || after == len(paths) {
		classification = "already_satisfied"
		summary = "every changed source path already has the source outcome's exact blob identity"
	} else if before == len(paths) {
		classification = "directly_applicable"
		summary = "every changed source path retains the exact pre-change blob identity"
	} else if divergent > 0 && campaign.RepositoryID == target.RepositoryID {
		classification = "conflicting"
		summary = "the target independently changed one or more source paths"
	} else if missing == len(paths) {
		classification = "not_applicable"
		summary = "none of the source change's affected paths or prior shapes exist on the target line"
	}
	diffText, _ := gitOutput(source.Path(), "diff", "--unified=0", base, head, "--")
	symbolEvidence := propagationSymbolEvidence(diffText)
	priorFixes := []string{}
	if len(paths) > 0 {
		args := []string{"log", "-n", "12", "--format=%H %s", tip, "--"}
		args = append(args, paths...)
		if history, x := gitOutput(destination.Path(), args...); x == nil && history != "" {
			priorFixes = strings.Split(history, "\n")
		}
	}
	historyEvidence := []string{"source_base=" + base, "source_head=" + head, "target_tip=" + tip}
	status := func(match bool) string {
		if match {
			return "aligned"
		}
		return "review_required"
	}
	bySuffix := func(suffixes ...string) []string {
		var out []string
		for _, p := range paths {
			for _, s := range suffixes {
				if strings.HasSuffix(strings.ToLower(p), s) {
					out = append(out, p)
					break
				}
			}
		}
		return out
	}
	comparisons := []propagationcampaigns.Comparison{
		{Kind: "histories", Status: status(before == len(paths) || after == len(paths)), Summary: summary, Evidence: historyEvidence},
		{Kind: "symbols", Status: status(divergent == 0), Summary: "Declared symbols named by the exact source diff are retained for target review; matching names alone are not behavioral proof.", Evidence: symbolEvidence},
		{Kind: "dependencies", Status: status(len(bySuffix("go.mod", "go.sum", "package.json", "bun.lock", "packages.json")) == 0), Summary: "Dependency manifests touched by the source are called out for target-owner review.", Evidence: bySuffix("go.mod", "go.sum", "package.json", "bun.lock", "packages.json")},
		{Kind: "interfaces", Status: status(len(bySuffix(".proto", ".graphql", "openapi.json", "openapi.yaml", "openapi.yml")) == 0), Summary: "Declared interface files are compared by exact path and blob; similarity is not behavioral proof.", Evidence: bySuffix(".proto", ".graphql", "openapi.json", "openapi.yaml", "openapi.yml")},
		{Kind: "schemas", Status: status(len(bySuffix(".sql", "schema.json", "schema.yaml", "schema.yml")) == 0), Summary: "Schema-bearing changes require explicit compatibility review when present.", Evidence: bySuffix(".sql", "schema.json", "schema.yaml", "schema.yml")},
		{Kind: "prior_fixes", Status: "review_required", Summary: "Exact target commits previously touching affected paths are retained; similar commit prose is not equivalence proof.", Evidence: priorFixes},
		{Kind: "release_commitments", Status: "review_required", Summary: "The campaign release line, deadline, owners, and acceptance criteria remain the authoritative commitment boundary.", Evidence: []string{"release_line=" + target.ReleaseLine, "deadline=" + target.Deadline.UTC().Format("2006-01-02T15:04:05Z")}},
	}
	return propagationcampaigns.Assessment{TargetID: target.ID, Classification: classification, TargetRevision: tip, SourceBaseRevision: base, SourceRevision: head, ChangedPaths: paths, Comparisons: comparisons}, nil
}

func propagationSymbolEvidence(diff string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "+"))
		for _, prefix := range []string{"func ", "type ", "class ", "interface ", "export function ", "export class ", "export interface ", "def "} {
			if strings.HasPrefix(value, prefix) {
				name := strings.Fields(strings.TrimPrefix(value, prefix))
				if len(name) > 0 {
					evidence := name[0]
					if boundary := strings.IndexAny(evidence, "{(:"); boundary >= 0 {
						evidence = evidence[:boundary]
					}
					if !seen[evidence] {
						seen[evidence] = true
						out = append(out, evidence)
					}
				}
			}
		}
	}
	sort.Strings(out)
	return out
}
