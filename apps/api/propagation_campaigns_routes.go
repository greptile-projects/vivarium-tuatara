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
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/propagationcampaigns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerPropagationCampaignRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, campaigns *propagationcampaigns.Store, pulls *pullrequests.Store, proposalStore *proposals.Store, checks *checkruns.Store, releaseStore *releases.Store, deploymentStore *deployments.Store) {
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
		for i := range v.EquivalenceProofs {
			p := &v.EquivalenceProofs[i]
			for _, target := range v.Targets {
				if target.ID != p.TargetID || target.Kind != "repository" {
					continue
				}
				repo, e := git.Open(target.RepositoryID)
				if e != nil {
					p.Invalidated = true
					p.InvalidationReasons = []string{"target history is unavailable"}
					continue
				}
				reasons := []string{}
				tip, e := gitOutput(repo.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(target.ReleaseLine, "refs/heads/")+"^{commit}")
				if e != nil || tip != p.TargetRevision {
					reasons = append(reasons, "target release line moved")
				}
				if propagationDependencyDigest(repo.Path(), p.TargetRevision) != p.DependencySHA256 {
					reasons = append(reasons, "target dependency assumptions changed")
				}
				source, e := git.Open(v.Source.RepositoryID)
				if e != nil || propagationSourceAssumptions(source.Path(), p.SourceRevision, v.AcceptanceCriteria) != p.SourceAssumptionsSHA256 {
					reasons = append(reasons, "source scenarios or acceptance assumptions changed")
				}
				p.Invalidated, p.InvalidationReasons = len(reasons) > 0, reasons
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
		visibleProofs := make([]propagationcampaigns.EquivalenceProof, 0, len(v.EquivalenceProofs))
		for _, proof := range v.EquivalenceProofs {
			for _, target := range v.Targets {
				if target.ID == proof.TargetID && target.State != "inaccessible" && target.State != "unsupported" {
					visibleProofs = append(visibleProofs, proof)
				}
			}
		}
		v.EquivalenceProofs = visibleProofs
		visibleDeliveries := make([]propagationcampaigns.DeliveryPath, 0, len(v.DeliveryPaths))
		delivered := map[string]bool{}
		groups := map[string]bool{}
		coverageBlockers, nextActions := []string{}, []string{}
		for _, retained := range v.DeliveryPaths {
			var target *propagationcampaigns.Target
			for i := range v.Targets {
				if v.Targets[i].ID == retained.TargetID {
					target = &v.Targets[i]
				}
			}
			if target == nil || target.State == "inaccessible" || target.State == "unsupported" {
				continue
			}
			d := retained
			d.ReviewState, d.QueueState, d.Blockers, d.ObservedOutcomes = "pending", "not_queued", []string{}, []string{}
			proofCurrent := propagationProofCurrent(v.EquivalenceProofs, d)
			if !proofCurrent {
				d.Blockers = append(d.Blockers, "equivalence proof is stale, rejected, or superseded")
				d.NextAction = "target owner refreshes and accepts equivalence evidence"
			}
			pull, e := pulls.Get(target.RepositoryID, d.PullRequestID)
			if e != nil {
				d.Blockers, d.NextAction = append(d.Blockers, "linked pull is unavailable"), "restore access to the ordinary target contribution"
			} else {
				reviews, _ := pulls.ListReviews(target.RepositoryID, pull.ID)
				d.ReviewState = propagationReviewState(reviews)
				if pull.Status == pullrequests.Merged && pull.MergeCommitID != nil {
					d.ReviewState, d.QueueState, d.MergeRevision = "approved", "merged", *pull.MergeCommitID
				} else if pull.QueuePaused {
					d.QueueState = "paused"
				} else if pull.QueuedAt != nil {
					d.QueueState = "queued"
				}
				switch {
				case d.ReviewState == "changes_requested":
					d.Blockers, d.NextAction = append(d.Blockers, "target review requested changes"), "target contributor addresses review"
				case d.MergeRevision == "" && d.QueueState == "paused":
					d.Blockers, d.NextAction = append(d.Blockers, "target integration queue is paused"), "target owner resolves queue blockers"
				case d.MergeRevision == "" && d.ReviewState != "approved":
					d.NextAction = "target reviewers independently review the pull"
				case d.MergeRevision == "":
					d.NextAction = "target owner queues and merges through ordinary policy"
				}
			}
			if d.MergeRevision != "" {
				releaseItems, _ := releaseStore.List(target.RepositoryID)
				for _, release := range releaseItems {
					included := false
					for _, id := range release.Inclusions.PullRequestIDs {
						included = included || id == d.PullRequestID
					}
					if included {
						d.ReleaseID, d.ReleaseVersion = release.ID, release.Version
					}
				}
				if d.ReleaseID == "" {
					d.NextAction = "target release owner publishes an ordinary release"
				}
			}
			if d.ReleaseID != "" {
				promotions, _ := deploymentStore.ListPromotions(target.RepositoryID)
				for _, p := range promotions {
					if p.ReleaseID == d.ReleaseID && (d.DeploymentID == "" || p.CreationSequence > 0) {
						d.DeploymentID, d.EnvironmentID, d.RolloutState = p.ID, p.EnvironmentID, p.State
						d.ObservedOutcomes = nil
						for _, signal := range p.Evidence {
							d.ObservedOutcomes = append(d.ObservedOutcomes, signal.Stage+": "+signal.Signal+" "+signal.State)
						}
					}
				}
				if d.DeploymentID == "" {
					d.NextAction = "target deployment owner starts an ordinary rollout"
				} else if d.RolloutState == "failed" || d.RolloutState == "paused" {
					d.Blockers, d.NextAction = append(d.Blockers, "rollout is "+d.RolloutState), "target deployment owner decides recovery for this path"
				} else if d.RolloutState == "succeeded" && proofCurrent {
					d.Exposed, d.NextAction = true, "observe supported-user outcomes"
					delivered[d.TargetID] = true
					for _, group := range d.SupportedUserGroups {
						groups[group] = true
					}
				} else {
					d.NextAction = "target deployment owner advances the governed rollout"
				}
			}
			if !proofCurrent {
				d.Exposed = false
				d.NextAction = "target owner refreshes and accepts equivalence evidence"
			}
			for _, blocker := range d.Blockers {
				coverageBlockers = append(coverageBlockers, d.TargetID+": "+blocker)
			}
			if d.NextAction != "" {
				nextActions = append(nextActions, d.TargetID+": "+d.NextAction)
			}
			visibleDeliveries = append(visibleDeliveries, d)
		}
		v.DeliveryPaths = visibleDeliveries
		for _, target := range v.Targets {
			if delivered[target.ID] {
				continue
			}
			if target.State == "inaccessible" || target.State == "unsupported" || target.State == "unknown" {
				coverageBlockers = append(coverageBlockers, target.ID+": "+target.Diagnostic)
				nextActions = append(nextActions, target.ID+": restore target visibility or support")
			}
			latestProofState := ""
			for _, proof := range v.EquivalenceProofs {
				if proof.TargetID == target.ID {
					latestProofState = proof.State
				}
			}
			if latestProofState == "rejected" {
				coverageBlockers = append(coverageBlockers, target.ID+": target owner rejected equivalence evidence")
				nextActions = append(nextActions, target.ID+": revise the adaptation or evidence")
			}
			hasDelivery := false
			for _, delivery := range v.DeliveryPaths {
				hasDelivery = hasDelivery || delivery.TargetID == target.ID
			}
			if !hasDelivery && target.State != "inaccessible" && target.State != "unsupported" {
				nextActions = append(nextActions, target.ID+": prove, accept, and bind an ordinary target contribution")
			}
			for _, dependency := range target.DependsOn {
				if !delivered[dependency] {
					coverageBlockers = append(coverageBlockers, target.ID+": waiting for dependency "+dependency)
				}
			}
		}
		for _, event := range v.ScopeEvents {
			if event.Kind == "consumer_discovered" {
				coverageBlockers = append(coverageBlockers, "new consumer "+event.ConsumerRepositoryID+": "+event.Reason)
				nextActions = append(nextActions, event.FollowUp)
			}
			if event.Kind == "bounded_exception" && time.Now().UTC().Before(event.ExpiresAt) {
				coverageBlockers = append(coverageBlockers, event.TargetID+": bounded exception until "+event.ExpiresAt.Format(time.RFC3339))
				nextActions = append(nextActions, event.FollowUp)
			}
			if event.Kind == "target_superseded" {
				coverageBlockers = append(coverageBlockers, event.TargetID+": superseded target remains unresolved by successor work")
				nextActions = append(nextActions, event.FollowUp)
			}
		}
		groupList := make([]string, 0, len(groups))
		for group := range groups {
			groupList = append(groupList, group)
		}
		sort.Strings(groupList)
		deliveredCount := len(delivered)
		policySatisfied := deliveredCount == len(v.Targets)
		if v.CompletionPolicy.Mode == "minimum" {
			policySatisfied = deliveredCount >= v.CompletionPolicy.MinimumTargets
		}
		if v.CompletionPolicy.Mode == "ordered" {
			policySatisfied = deliveredCount == len(v.Targets)
			for _, target := range v.Targets {
				if delivered[target.ID] {
					for _, dependency := range target.DependsOn {
						policySatisfied = policySatisfied && delivered[dependency]
					}
				}
			}
		}
		state := "in_progress"
		if policySatisfied && len(coverageBlockers) == 0 {
			state = "complete"
		} else if policySatisfied {
			state = "policy_satisfied_with_visible_gaps"
		} else if deliveredCount > 0 {
			state = "partial_adoption"
		}
		v.Coverage = propagationcampaigns.Coverage{State: state, PolicySatisfied: policySatisfied, DeliveredTargets: deliveredCount, TotalTargets: len(v.Targets), SupportedUserGroups: groupList, Blockers: coverageBlockers, NextActions: nextActions}
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
	mux.HandleFunc("POST /repositories/{id}/propagation-campaigns/{campaign_id}/targets/{target_id}/equivalence-proofs", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if checks == nil {
			writeAPIError(w, 503, "propagation_equivalence_unavailable", "bounded checks are unavailable")
			return
		}
		var in struct {
			RequestID      string `json:"request_id"`
			TargetRevision string `json:"target_revision"`
			Adaptations    []struct {
				Scenario           string                          `json:"scenario"`
				Command            string                          `json:"command"`
				EnvironmentCheck   string                          `json:"environment_check"`
				Coverage           []string                        `json:"coverage"`
				Unsupported        bool                            `json:"unsupported"`
				SubstituteEvidence []propagationcampaigns.Citation `json:"substitute_evidence"`
				ResidualDifference string                          `json:"residual_difference"`
			} `json:"adaptations"`
		}
		if decodeJSON(r, &in) != nil || !validPropagationRequestID(in.RequestID) {
			writeAPIError(w, 400, "invalid_request", "a stable equivalence proof request is required")
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
			writeAPIError(w, 404, "propagation_target_not_found", "repository target not found")
			return
		}
		targetRepo, e := catalog.GetByID(target.RepositoryID)
		access, _ := catalog.HasCollaborator(c.UserID, target.RepositoryID)
		if e != nil || (targetRepo.OwnerID != c.UserID && !access) {
			writeAPIError(w, 403, "propagation_target_forbidden", "current target access is required")
			return
		}
		gr, e := git.Open(target.RepositoryID)
		if e != nil || len(in.TargetRevision) != 40 || !catalog.HasCommit(target.RepositoryID, in.TargetRevision) {
			writeAPIError(w, 422, "propagation_target_revision_missing", "the exact target revision must resolve")
			return
		}
		sourceRepo, e := git.Open(campaign.Source.RepositoryID)
		if e != nil {
			writeAPIError(w, 422, "propagation_source_invalid", "source scenarios are unavailable")
			return
		}
		sourceRevision := campaign.Source.Commits[len(campaign.Source.Commits)-1]
		sourceDefs, e := propagationDefinitions(sourceRepo.Path(), sourceRevision)
		if e != nil || len(sourceDefs) == 0 {
			writeAPIError(w, 422, "propagation_scenarios_missing", "the source outcome must define reusable ordinary checks")
			return
		}
		targetDefs, e := propagationDefinitions(gr.Path(), in.TargetRevision)
		if e != nil || len(targetDefs) == 0 {
			writeAPIError(w, 422, "propagation_target_checks_missing", "the target revision must define ordinary checks")
			return
		}
		adapted := map[string]struct {
			Command, Environment string
			Coverage             []string
			Unsupported          bool
			Substitutes          []propagationcampaigns.Citation
			Residual             string
		}{}
		for _, a := range in.Adaptations {
			if _, exists := adapted[a.Scenario]; exists {
				writeAPIError(w, 422, "propagation_adaptation_invalid", "each source scenario requires one adaptation")
				return
			}
			adapted[a.Scenario] = struct {
				Command, Environment string
				Coverage             []string
				Unsupported          bool
				Substitutes          []propagationcampaigns.Citation
				Residual             string
			}{a.Command, a.EnvironmentCheck, a.Coverage, a.Unsupported, a.SubstituteEvidence, a.ResidualDifference}
		}
		covered := map[string]bool{}
		for _, a := range in.Adaptations {
			for _, criterion := range a.Coverage {
				covered[strings.TrimSpace(criterion)] = true
			}
		}
		for _, criterion := range campaign.AcceptanceCriteria {
			if !covered[criterion] {
				writeAPIError(w, 422, "propagation_coverage_incomplete", "every source acceptance criterion requires scenario coverage")
				return
			}
		}
		byTarget := map[string]checkruns.Definition{}
		for _, d := range targetDefs {
			byTarget[d.Name] = d
		}
		scope := "propagation-" + campaign.ID + "-" + target.ID + "-" + in.RequestID
		proof := propagationcampaigns.EquivalenceProof{RequestID: in.RequestID, TargetID: target.ID, TargetRevision: in.TargetRevision, SourceRevision: sourceRevision, EvidenceRequirements: append([]string{}, campaign.AcceptanceCriteria...), SourceAssumptionsSHA256: propagationSourceAssumptions(sourceRepo.Path(), sourceRevision, campaign.AcceptanceCriteria), DependencySHA256: propagationDependencyDigest(gr.Path(), in.TargetRevision), State: "demonstrated"}
		for _, d := range sourceDefs {
			a, exists := adapted[d.Name]
			if !exists || len(a.Coverage) == 0 {
				writeAPIError(w, 422, "propagation_adaptation_missing", "every source scenario requires declared target coverage")
				return
			}
			s := propagationcampaigns.EquivalenceScenario{Name: d.Name, SourceCommand: d.Command, Coverage: a.Coverage}
			if a.Unsupported {
				if len(a.Substitutes) == 0 {
					writeAPIError(w, 422, "propagation_substitute_required", "unsupported scenarios require explicit substitute evidence")
					return
				}
				s.State = "unsupported"
				s.SubstituteEvidence = a.Substitutes
				proof.ResidualDifferences = append(proof.ResidualDifferences, a.Residual)
				proof.State = "residual_differences"
			} else {
				base, ok := byTarget[a.Environment]
				if !ok || strings.TrimSpace(a.Command) == "" {
					writeAPIError(w, 422, "propagation_adaptation_invalid", "adapted scenarios require a target check environment and exact command")
					return
				}
				base.Name = "equivalence:" + d.Name
				base.Command = a.Command
				run := executePropagationCheck(checks, gr.Path(), target.RepositoryID, scope, in.TargetRevision, base, actorID(c))
				s.TargetCommand = a.Command
				s.State = run.State
				s.CheckRunID = run.ID
				s.Logs, s.Artifacts, s.Cost = propagationRunEvidence(checks, run)
				if run.State != "succeeded" {
					proof.State = "failed"
				}
			}
			proof.Scenarios = append(proof.Scenarios, s)
		}
		for _, d := range targetDefs {
			run := executePropagationCheck(checks, gr.Path(), target.RepositoryID, scope, in.TargetRevision, d, actorID(c))
			o := propagationcampaigns.OrdinaryCheck{Name: d.Name, Command: d.Command, State: run.State, CheckRunID: run.ID}
			o.Logs, o.Artifacts, o.Cost = propagationRunEvidence(checks, run)
			if run.State != "succeeded" {
				proof.State = "failed"
			}
			proof.OrdinaryChecks = append(proof.OrdinaryChecks, o)
		}
		body, _ := json.Marshal(in)
		sum := sha256.Sum256(append(body, []byte(proof.SourceAssumptionsSHA256+proof.DependencySHA256)...))
		updated, out, e := campaigns.CreateEquivalenceProof(campaign.RepositoryID, campaign.ID, actorID(c), hex.EncodeToString(sum[:]), proof)
		if errors.Is(e, propagationcampaigns.ErrConflict) {
			writeAPIError(w, 409, "propagation_equivalence_conflict", "this proof request was reused with different evidence")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "propagation_equivalence_invalid", "equivalence evidence could not be retained")
			return
		}
		writeJSON(w, 201, map[string]any{"campaign": project(updated, c), "equivalence_proof": out})
	})
	mux.HandleFunc("POST /repositories/{id}/propagation-campaigns/{campaign_id}/equivalence-proofs/{proof_id}/decisions", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if c.AgentID != "" {
			writeAPIError(w, 403, "propagation_decision_forbidden", "only a named human target owner may decide equivalence")
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Decision        string `json:"decision"`
			Rationale       string `json:"rationale"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an owner decision is required")
			return
		}
		campaign, e := campaigns.Get(r.PathValue("id"), r.PathValue("campaign_id"))
		if e != nil {
			writeAPIError(w, 404, "propagation_campaign_not_found", "propagation campaign not found")
			return
		}
		var proof *propagationcampaigns.EquivalenceProof
		for i := range campaign.EquivalenceProofs {
			if campaign.EquivalenceProofs[i].ID == r.PathValue("proof_id") {
				proof = &campaign.EquivalenceProofs[i]
			}
		}
		owner := false
		if proof != nil {
			for _, t := range campaign.Targets {
				if t.ID == proof.TargetID {
					for _, id := range t.OwnerIDs {
						if id == c.UserID {
							owner = true
						}
					}
				}
			}
		}
		if !owner {
			writeAPIError(w, 403, "propagation_decision_forbidden", "only a named human target owner may decide equivalence")
			return
		}
		var targetRepositoryID string
		for _, target := range campaign.Targets {
			if proof != nil && target.ID == proof.TargetID {
				targetRepositoryID = target.RepositoryID
			}
		}
		targetRepository, targetErr := catalog.GetByID(targetRepositoryID)
		targetAccess, _ := catalog.HasCollaborator(c.UserID, targetRepositoryID)
		if targetErr != nil || (targetRepository.OwnerID != c.UserID && !targetAccess) {
			writeAPIError(w, 403, "propagation_target_forbidden", "current target access is required to decide equivalence")
			return
		}
		updated, out, e := campaigns.DecideEquivalenceProof(campaign.RepositoryID, campaign.ID, proof.ID, c.UserID, in.Decision, in.Rationale, in.ExpectedVersion)
		if errors.Is(e, propagationcampaigns.ErrProofVersion) {
			writeAPIError(w, 409, "propagation_equivalence_changed", "reload the proof before deciding")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "propagation_decision_invalid", "an accepted or rejected decision with rationale is required")
			return
		}
		writeJSON(w, 201, map[string]any{"campaign": project(updated, c), "equivalence_proof": out})
	})
	mux.HandleFunc("POST /repositories/{id}/propagation-campaigns/{campaign_id}/targets/{target_id}/delivery-paths", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if c.AgentID != "" {
			writeAPIError(w, 403, "propagation_delivery_forbidden", "only a named human target owner may publish delivery tracking")
			return
		}
		var in propagationcampaigns.DeliveryPath
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an accepted proof, contribution pull, and supported users are required")
			return
		}
		campaign, e := campaigns.Get(r.PathValue("id"), r.PathValue("campaign_id"))
		if e != nil {
			writeAPIError(w, 404, "propagation_campaign_not_found", "propagation campaign not found")
			return
		}
		var target *propagationcampaigns.Target
		var contribution *propagationcampaigns.Contribution
		var proof *propagationcampaigns.EquivalenceProof
		for i := range campaign.Targets {
			if campaign.Targets[i].ID == r.PathValue("target_id") {
				target = &campaign.Targets[i]
			}
		}
		for i := range campaign.Contributions {
			if campaign.Contributions[i].ID == in.ContributionID {
				contribution = &campaign.Contributions[i]
			}
		}
		for i := range campaign.EquivalenceProofs {
			if campaign.EquivalenceProofs[i].ID == in.EquivalenceProofID {
				proof = &campaign.EquivalenceProofs[i]
			}
		}
		owner := false
		if target != nil {
			for _, id := range target.OwnerIDs {
				owner = owner || id == c.UserID
			}
		}
		if !owner || target == nil || target.Kind != "repository" {
			writeAPIError(w, 403, "propagation_delivery_forbidden", "current named target-owner authority is required")
			return
		}
		repo, repoErr := catalog.GetByID(target.RepositoryID)
		access, _ := catalog.HasCollaborator(c.UserID, target.RepositoryID)
		if repoErr != nil || (repo.OwnerID != c.UserID && !access) {
			writeAPIError(w, 403, "propagation_target_forbidden", "current target repository access is required")
			return
		}
		pull, pullErr := pulls.Get(target.RepositoryID, in.PullRequestID)
		taskLinked := false
		if pullErr == nil && pull.TaskID != nil && contribution != nil {
			for _, id := range contribution.TaskIDs {
				taskLinked = taskLinked || id == *pull.TaskID
			}
		}
		proofCurrent := false
		if proof != nil {
			if gr, openErr := git.Open(target.RepositoryID); openErr == nil {
				tip, tipErr := gitOutput(gr.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(target.ReleaseLine, "refs/heads/")+"^{commit}")
				if source, sourceErr := git.Open(campaign.Source.RepositoryID); tipErr == nil && sourceErr == nil {
					proofCurrent = tip == proof.TargetRevision && propagationDependencyDigest(gr.Path(), proof.TargetRevision) == proof.DependencySHA256 && propagationSourceAssumptions(source.Path(), proof.SourceRevision, campaign.AcceptanceCriteria) == proof.SourceAssumptionsSHA256
				}
			}
		}
		if contribution == nil || proof == nil || proof.Version != in.ProofVersion || proof.State != "accepted" || !proofCurrent || pullErr != nil || !taskLinked || pull.SourceCommitID != proof.TargetRevision {
			writeAPIError(w, 422, "propagation_delivery_invalid", "delivery must bind the current accepted proof to its exact ordinary task pull")
			return
		}
		in.TargetID = target.ID
		updated, out, e := campaigns.LinkDeliveryPath(campaign.RepositoryID, campaign.ID, c.UserID, in)
		if errors.Is(e, propagationcampaigns.ErrConflict) {
			writeAPIError(w, 409, "propagation_delivery_conflict", "this target already tracks different delivery work")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "propagation_delivery_invalid", "supported-user delivery tracking could not be retained")
			return
		}
		writeJSON(w, 201, map[string]any{"campaign": project(updated, c), "delivery_path": out})
	})
	mux.HandleFunc("POST /repositories/{id}/propagation-campaigns/{campaign_id}/scope-events", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if c.AgentID != "" {
			writeAPIError(w, 403, "propagation_scope_forbidden", "scope decisions require an accountable human")
			return
		}
		var in propagationcampaigns.ScopeEvent
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributable scope event is required")
			return
		}
		campaign, e := campaigns.Get(r.PathValue("id"), r.PathValue("campaign_id"))
		if e != nil {
			writeAPIError(w, 404, "propagation_campaign_not_found", "propagation campaign not found")
			return
		}
		if in.Kind != "consumer_discovered" {
			owner := false
			targetRepositoryID := ""
			for _, target := range campaign.Targets {
				if target.ID == in.TargetID {
					targetRepositoryID = target.RepositoryID
					for _, id := range target.OwnerIDs {
						owner = owner || id == c.UserID
					}
				}
			}
			targetRepository, targetErr := catalog.GetByID(targetRepositoryID)
			targetAccess, _ := catalog.HasCollaborator(c.UserID, targetRepositoryID)
			if !owner || targetErr != nil || (targetRepository.OwnerID != c.UserID && !targetAccess) {
				writeAPIError(w, 403, "propagation_scope_forbidden", "only a named target owner may supersede or except that path")
				return
			}
		}
		updated, out, e := campaigns.AddScopeEvent(campaign.RepositoryID, campaign.ID, c.UserID, in)
		if e != nil {
			writeAPIError(w, 422, "propagation_scope_invalid", "discoveries need users and follow-up; exceptions must expire within 30 days")
			return
		}
		writeJSON(w, 201, map[string]any{"campaign": project(updated, c), "scope_event": out})
	})
}

func gitOutput(path string, args ...string) (string, error) {
	all := append([]string{"--git-dir=" + path}, args...)
	b, e := exec.Command("git", all...).Output()
	return strings.TrimSpace(string(b)), e
}

func propagationReviewState(reviews []pullrequests.Review) string {
	approved := false
	for _, review := range reviews {
		if review.Decision == pullrequests.ChangesRequested {
			return "changes_requested"
		}
		approved = approved || review.Decision == pullrequests.Approved
	}
	if approved {
		return "approved"
	}
	return "pending"
}

func propagationProofCurrent(proofs []propagationcampaigns.EquivalenceProof, delivery propagationcampaigns.DeliveryPath) bool {
	for _, proof := range proofs {
		if proof.ID == delivery.EquivalenceProofID && proof.Version == delivery.ProofVersion && proof.State == "accepted" && !proof.Invalidated {
			return true
		}
	}
	return false
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

func propagationDefinitions(gitDir, revision string) ([]checkruns.Definition, error) {
	b, err := exec.Command("git", "--git-dir="+gitDir, "show", revision+":"+checkruns.ConfigPath).Output()
	if err != nil {
		return nil, err
	}
	config, err := checkruns.ParseConfig(b)
	if err != nil {
		return nil, err
	}
	return config.Checks, nil
}

func propagationSourceAssumptions(gitDir, revision string, criteria []string) string {
	b, _ := exec.Command("git", "--git-dir="+gitDir, "show", revision+":"+checkruns.ConfigPath).Output()
	payload, _ := json.Marshal(struct {
		Config   json.RawMessage `json:"config"`
		Criteria []string        `json:"criteria"`
	}{Config: b, Criteria: criteria})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func propagationDependencyDigest(gitDir, revision string) string {
	h := sha256.New()
	for _, path := range []string{"go.mod", "go.sum", "package.json", "bun.lock", ".vivarium/packages.json"} {
		b, err := exec.Command("git", "--git-dir="+gitDir, "show", revision+":"+path).Output()
		if err == nil {
			h.Write([]byte(path))
			h.Write([]byte{0})
			h.Write(b)
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func executePropagationCheck(store *checkruns.Store, gitDir, repositoryID, scope, revision string, definition checkruns.Definition, actor string) checkruns.Run {
	runs, err := store.CreateRequested(repositoryID, scope, revision, []checkruns.Definition{definition}, actor)
	if err != nil || len(runs) == 0 {
		return checkruns.Run{Definition: definition, CommitID: revision, State: "failed", Failure: "check could not be reserved"}
	}
	store.Execute(runs[0], gitDir)
	run, err := store.Get(repositoryID, scope, runs[0].ID)
	if err != nil {
		return runs[0]
	}
	return run
}

func propagationRunEvidence(store *checkruns.Store, run checkruns.Run) (string, []propagationcampaigns.Artifact, float64) {
	logs := []string{}
	events, _ := store.Events(run.RepositoryID, run.PullRequestID, run.ID, 0)
	for _, event := range events {
		if (event.Stream == "stdout" || event.Stream == "stderr" || event.Kind == "command") && event.Message != "" {
			logs = append(logs, event.Stream+": "+event.Message)
		}
	}
	artifacts := make([]propagationcampaigns.Artifact, len(run.Artifacts))
	for i, a := range run.Artifacts {
		artifacts[i] = propagationcampaigns.Artifact{Path: a.Path, SHA256: a.SHA256, Size: a.Size}
	}
	cost := 0.0
	if run.StartedAt != nil && run.CompletedAt != nil {
		cpus := run.Definition.CPUs
		if cpus <= 0 {
			cpus = 1
		}
		cost = run.CompletedAt.Sub(*run.StartedAt).Hours() * cpus
	}
	return strings.Join(logs, "\n"), artifacts, cost
}

func validPropagationRequestID(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
