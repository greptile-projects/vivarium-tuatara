package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changestacks"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenanceassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancegraphs"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancepolicies"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type provenanceAssessmentInput struct {
	RequestID           string                          `json:"request_id"`
	Candidate           provenanceassessments.Candidate `json:"candidate"`
	GraphID             string                          `json:"graph_id"`
	PolicyID            string                          `json:"policy_id"`
	PolicyVersion       int                             `json:"policy_version"`
	DistributionTargets []string                        `json:"distribution_targets"`
}
type provenanceAssessmentEventInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Event           provenanceassessments.Event `json:"event"`
}
type provenanceRepairInput struct {
	ExpectedVersion    int                                                `json:"expected_version"`
	RequestID          string                                             `json:"request_id"`
	FindingID          string                                             `json:"finding_id"`
	Strategy           string                                             `json:"strategy"`
	PermittedEvidence  []provenanceassessments.EvidenceReference          `json:"permitted_evidence"`
	AcceptanceCriteria []string                                           `json:"acceptance_criteria"`
	CleanRoom          bool                                               `json:"clean_room"`
	Title              string                                             `json:"title"`
	Tasks              []struct{ Title, AssigneeType, AssigneeID string } `json:"tasks"`
}

func registerProvenanceAssessmentRoutes(mux *http.ServeMux, repos *repositories.Store, credentials *auth.Store, store *provenanceassessments.Store, graphs *provenancegraphs.Store, policies *provenancepolicies.Store, pulls *pullrequests.Store, stackStore *changestacks.Store, releaseStore *releases.Store, packageStore *packages.Store, proposalStore *proposals.Store) {
	current := func(a provenanceassessments.Assessment) provenanceassessments.Current {
		return provenanceAssessmentCurrent(a, repos, graphs, policies, pulls, stackStore, releaseStore, packageStore)
	}
	mux.HandleFunc("GET /repositories/{id}/provenance-assessments", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		xs, e := store.List(r.PathValue("id"), current)
		if e != nil {
			writeAPIError(w, 500, "provenance_assessments_unavailable", "provenance assessments could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"assessments": xs})
	})
	mux.HandleFunc("POST /repositories/{id}/provenance-assessments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in provenanceAssessmentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact candidate, graph, policy, and distribution scope are required")
			return
		}
		revision, e := resolveProvenanceCandidate(r.PathValue("id"), in.Candidate, pulls, stackStore, releaseStore, packageStore)
		if e != nil || revision != in.Candidate.Revision {
			writeAPIError(w, 422, "provenance_candidate_invalid", "the candidate does not resolve to its exact current revision")
			return
		}
		g, e := graphs.Get(in.GraphID)
		if e != nil || g.RepositoryID != r.PathValue("id") || g.Revision != revision {
			writeAPIError(w, 422, "provenance_graph_invalid", "the graph must describe the exact candidate revision")
			return
		}
		p, e := policies.Get(in.PolicyID)
		if e != nil || p.ScopeKind != "repository" || p.ScopeID != r.PathValue("id") || in.PolicyVersion < 1 || in.PolicyVersion > len(p.Revisions) {
			writeAPIError(w, 422, "provenance_policy_invalid", "the exact repository policy revision does not resolve")
			return
		}
		findings := deriveProvenanceFindings(g, p.Revisions[in.PolicyVersion-1], in.DistributionTargets)
		a := provenanceassessments.Assessment{RequestID: in.RequestID, RepositoryID: r.PathValue("id"), Candidate: in.Candidate, GraphID: g.ID, GraphDigest: g.AnalysisDigest, PolicyID: p.ID, PolicyVersion: in.PolicyVersion, Findings: findings, CreatedBy: actor.UserID}
		out, e := store.Create(a)
		writeProvenanceAssessment(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/provenance-assessments/{assessment_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var in provenanceAssessmentEventInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a versioned cited assessment event is required")
			return
		}
		typ := "human"
		if actor.AgentID != "" {
			typ = "agent"
			if in.Event.Kind == "acknowledgement" || in.Event.Kind == "exception" {
				writeAPIError(w, 403, "human_provenance_owner_required", "only a current human provenance owner may acknowledge or except a finding")
				return
			}
		}
		a, e := store.Get(r.PathValue("id"), r.PathValue("assessment_id"), provenanceassessments.Current{})
		if e != nil {
			writeProvenanceAssessment(w, a, e, 200)
			return
		}
		out, e := store.AddEvent(r.PathValue("id"), a.ID, actor.UserID, typ, in.ExpectedVersion, in.Event, current(a))
		writeProvenanceAssessment(w, out, e, 200)
	})
	if proposalStore != nil {
		mux.HandleFunc("POST /repositories/{id}/provenance-assessments/{assessment_id}/repairs", func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			if actor.AgentID != "" {
				writeAPIError(w, 403, "human_provenance_owner_required", "only a current human provenance owner may authorize repair work")
				return
			}
			var in provenanceRepairInput
			if decodeJSON(r, &in) != nil || len(in.Tasks) == 0 {
				writeAPIError(w, 400, "invalid_provenance_repair", "a strategy, permitted evidence, acceptance criteria, and owned tasks are required")
				return
			}
			if strings.TrimSpace(in.RequestID) == "" || strings.TrimSpace(in.Title) == "" || len(in.AcceptanceCriteria) == 0 || !containsFold([]string{"replace", "reimplement", "remove", "obtain_permission", "isolate"}, in.Strategy) {
				writeAPIError(w, 400, "invalid_provenance_repair", "a supported repair strategy and complete work definition are required")
				return
			}
			for _, evidence := range in.PermittedEvidence {
				if evidence.Kind == "" || evidence.ResourceID == "" || evidence.Revision == "" || (evidence.Access != "repository" && evidence.Access != "restricted") || (in.CleanRoom && evidence.Access == "restricted") {
					writeAPIError(w, 400, "invalid_provenance_repair_evidence", "clean-room work may not receive restricted evidence")
					return
				}
			}
			a, err := store.Get(r.PathValue("id"), r.PathValue("assessment_id"), provenanceassessments.Current{})
			if err != nil {
				writeProvenanceAssessment(w, a, err, 200)
				return
			}
			live := current(a)
			if !live.OwnerIDs[actor.UserID] {
				writeAPIError(w, 403, "provenance_owner_required", "only a current human provenance owner may authorize repair work")
				return
			}
			projected, err := store.Get(a.RepositoryID, a.ID, live)
			if err != nil {
				writeAPIError(w, 409, "provenance_assessment_conflict", "the assessment changed; reload before authorizing work")
				return
			}
			workBytes, _ := json.Marshal(struct {
				FindingID string                                    `json:"finding_id"`
				Strategy  string                                    `json:"strategy"`
				Evidence  []provenanceassessments.EvidenceReference `json:"evidence"`
				Criteria  []string                                  `json:"criteria"`
				CleanRoom bool                                      `json:"clean_room"`
				Title     string                                    `json:"title"`
				Tasks     any                                       `json:"tasks"`
			}{in.FindingID, in.Strategy, in.PermittedEvidence, in.AcceptanceCriteria, in.CleanRoom, in.Title, in.Tasks})
			workSum := sha256.Sum256(workBytes)
			workDigest := hex.EncodeToString(workSum[:])
			for _, prior := range projected.Repairs {
				if prior.RequestID != in.RequestID {
					continue
				}
				retry := provenanceassessments.Repair{RequestID: in.RequestID, WorkDigest: workDigest, FindingID: in.FindingID, AffectedRevision: a.Candidate.Revision, Strategy: in.Strategy, PermittedEvidence: in.PermittedEvidence, AcceptanceCriteria: in.AcceptanceCriteria, CleanRoom: in.CleanRoom, ProposalID: prior.ProposalID, TaskIDs: prior.TaskIDs}
				updated, linkErr := store.LinkRepair(a.RepositoryID, a.ID, actor.UserID, in.ExpectedVersion, retry, live)
				if linkErr != nil {
					writeProvenanceAssessment(w, updated, linkErr, 200)
					return
				}
				proposal, proposalErr := proposalStore.Get(a.RepositoryID, prior.ProposalID)
				tasks, tasksErr := proposalStore.ListTasks(a.RepositoryID, prior.ProposalID)
				if proposalErr != nil || tasksErr != nil {
					writeAPIError(w, 500, "provenance_repair_unavailable", "retained ordinary repair work could not be read")
					return
				}
				writeJSON(w, 200, map[string]any{"assessment": updated, "repair": prior, "proposal": proposal, "tasks": tasks, "authority_note": "contributors continue through ordinary branches, forks, sessions, workspaces, reviews, checks, and release controls"})
				return
			}
			var finding *provenanceassessments.Finding
			for i := range projected.Findings {
				if projected.Findings[i].ID == in.FindingID {
					finding = &projected.Findings[i]
					break
				}
			}
			if finding == nil || !finding.Current {
				writeAPIError(w, 409, "provenance_finding_stale", "repair work must bind a current provenance finding")
				return
			}
			criteria := strings.Join(in.AcceptanceCriteria, "; ")
			evidenceIDs := make([]string, len(in.PermittedEvidence))
			for i, evidence := range in.PermittedEvidence {
				if evidence.Access == "restricted" {
					evidenceIDs[i] = "restricted evidence (identity withheld; separately governed access required)"
				} else {
					evidenceIDs[i] = evidence.Kind + ":" + evidence.ResourceID + "@" + evidence.Revision + " (repository)"
				}
			}
			boundary := "Only the listed evidence may be used."
			if in.CleanRoom {
				boundary = "Clean-room boundary: restricted evidence is excluded; implementers may use only the listed repository evidence."
			}
			tasks := make([]proposals.ImplementationTaskInput, len(in.Tasks))
			for i, task := range in.Tasks {
				assignee := task.AssigneeID
				if task.AssigneeType == "human" && assignee == "" {
					assignee = actor.UserID
				}
				tasks[i] = proposals.ImplementationTaskInput{Title: task.Title, Outcome: criteria, Risk: "Unresolved " + finding.Kind + " at " + a.Candidate.Revision + ". " + boundary, VerificationPlan: "Prove the acceptance criteria without rewriting original authorship: " + criteria, AssigneeType: task.AssigneeType, AssigneeID: assignee, DependsOnPrevious: i > 0}
			}
			digest := sha256.Sum256([]byte(a.ID + "\x00" + in.RequestID))
			origin := proposals.ReasoningOrigin{ProvenanceAssessmentID: a.ID, ProvenanceFindingID: finding.ID, ProvenanceRepairRequestID: hex.EncodeToString(digest[:16]), AssessmentVersion: a.Version, Revision: a.Candidate.Revision, SelectedItemIDs: []string{finding.ID}, Items: []proposals.ReasoningItem{{ID: finding.ID, Kind: "provenance_finding", Summary: finding.Summary, Status: finding.Severity}}, AnalysisStatus: "authorized_provenance_repair"}
			body := "Repair provenance finding " + finding.ID + " using strategy " + in.Strategy + ".\n\nAffected revision: " + a.Candidate.Revision + "\nPolicy: " + a.PolicyID + " revision " + fmt.Sprint(a.PolicyVersion) + "\nObligations: " + strings.Join(finding.Obligations, "; ") + "\nPermitted evidence: " + strings.Join(evidenceIDs, "; ") + "\nAcceptance criteria: " + criteria + "\n" + boundary + "\n\nThis handoff grants no Git, fork, session, workspace, review, merge, disclosure, permission, release, or distribution authority."
			p, createdTasks, err := proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: a.RepositoryID, ActorID: actor.UserID, Title: in.Title, Body: body, Origin: origin, Tasks: tasks})
			if err != nil {
				writeAPIError(w, 409, "provenance_repair_publication_failed", "repair work could not be published")
				return
			}
			ids := make([]string, len(createdTasks))
			for i := range createdTasks {
				ids[i] = createdTasks[i].ID
			}
			repair := provenanceassessments.Repair{RequestID: in.RequestID, WorkDigest: workDigest, FindingID: finding.ID, AffectedRevision: a.Candidate.Revision, Strategy: in.Strategy, PermittedEvidence: in.PermittedEvidence, AcceptanceCriteria: in.AcceptanceCriteria, CleanRoom: in.CleanRoom, ProposalID: p.ID, TaskIDs: ids}
			updated, err := store.LinkRepair(a.RepositoryID, a.ID, actor.UserID, in.ExpectedVersion, repair, live)
			if err != nil {
				writeProvenanceAssessment(w, updated, err, 201)
				return
			}
			writeJSON(w, 201, map[string]any{"assessment": updated, "repair": updated.Repairs[len(updated.Repairs)-1], "proposal": p, "tasks": createdTasks, "authority_note": "contributors continue through ordinary branches, forks, sessions, workspaces, reviews, checks, and release controls"})
		})
	}
}

func provenanceAssessmentCurrent(a provenanceassessments.Assessment, repos *repositories.Store, graphs *provenancegraphs.Store, policies *provenancepolicies.Store, pulls *pullrequests.Store, stackStore *changestacks.Store, releaseStore *releases.Store, packageStore *packages.Store) provenanceassessments.Current {
	c := provenanceassessments.Current{OwnerIDs: map[string]bool{}, DependencyRevisions: map[string]string{}, ToolRevisions: map[string]string{}, PolicyRuleDigests: map[string]string{}}
	c.CandidateRevision, _ = resolveProvenanceCandidate(a.RepositoryID, a.Candidate, pulls, stackStore, releaseStore, packageStore)
	if g, e := graphs.Get(a.GraphID); e == nil && g.RepositoryID == a.RepositoryID {
		c.GraphDigest = g.AnalysisDigest
		for _, n := range g.Nodes {
			if n.Kind == "dependency" {
				c.DependencyRevisions[n.ID] = n.Revision
			}
			if n.Kind == "tool" {
				c.ToolRevisions[n.ID] = n.Revision
			}
		}
	}
	if p, e := policies.Get(a.PolicyID); e == nil && p.ScopeKind == "repository" && p.ScopeID == a.RepositoryID {
		c.PolicyVersion = p.CurrentVersion
		if len(p.Revisions) > 0 {
			for _, rule := range p.Revisions[len(p.Revisions)-1].Rules {
				c.PolicyRuleDigests[rule.Kind] = provenanceRuleDigest(rule)
			}
		}
		if a.PolicyVersion <= len(p.Revisions) {
			for _, id := range append(append([]string{}, p.Revisions[a.PolicyVersion-1].OwnerIDs...), policyReviewOwners(p.Revisions[a.PolicyVersion-1])...) {
				c.OwnerIDs[id] = true
			}
		}
	}
	if repository, e := repos.GetByID(a.RepositoryID); e == nil {
		for id := range c.OwnerIDs {
			if !repository.HasParticipant(id) {
				delete(c.OwnerIDs, id)
			}
		}
	}
	return c
}

func resolveProvenanceCandidate(repo string, c provenanceassessments.Candidate, pulls *pullrequests.Store, stacks *changestacks.Store, releasesStore *releases.Store, packageStore *packages.Store) (string, error) {
	switch c.Kind {
	case "pull_request":
		if pulls != nil {
			v, e := pulls.Get(repo, c.ID)
			if e == nil {
				return v.SourceCommitID, nil
			}
		}
	case "release_candidate":
		if releasesStore != nil {
			v, e := releasesStore.Get(repo, c.ID)
			if e == nil {
				return v.CommitID, nil
			}
		}
	case "change_stack":
		if stacks != nil {
			v, e := stacks.Get(repo, c.ID)
			if e == nil {
				for _, x := range v.IntegrationCandidates {
					if x.CandidateRevision == c.Revision {
						return x.CandidateRevision, nil
					}
				}
			}
		}
	case "package_candidate":
		if packageStore != nil {
			xs, e := packageStore.ListRepository(repo)
			if e == nil {
				for _, v := range xs {
					if v.ID == c.ID || v.ArtifactID == c.ID {
						return v.SourceCommit, nil
					}
				}
			}
		}
	}
	return "", errors.New("candidate not found")
}
func policyReviewOwners(r provenancepolicies.Revision) []string {
	out := []string{}
	for _, x := range r.Rules {
		out = append(out, x.ReviewOwnerIDs...)
	}
	return out
}
func deriveProvenanceFindings(g provenancegraphs.Graph, p provenancepolicies.Revision, targets []string) []provenanceassessments.Finding {
	rules := map[string]provenancepolicies.MaterialRule{}
	for _, r := range p.Rules {
		rules[r.Kind] = r
	}
	nodes := map[string]provenancegraphs.Node{}
	origins := map[string][]string{}
	generated := map[string]string{}
	for _, n := range g.Nodes {
		nodes[n.ID] = n
	}
	for _, e := range g.Edges {
		if src, ok := nodes[e.From]; ok {
			if provenanceOriginTransformation(e.Transformation) {
				origins[e.To] = append(origins[e.To], src.Label)
			}
			if e.Transformation == "generated" {
				generated[e.To] = src.Label
			}
		}
	}
	out := []provenanceassessments.Finding{}
	add := func(kind, severity, material, node, summary, license, origin string, obligations, owners []string) {
		id := fmt.Sprintf("%s-%s-%d", kind, node, len(out)+1)
		f := provenanceassessments.Finding{ID: id, Kind: kind, Severity: severity, MaterialKind: material, NodeID: node, Summary: summary, License: license, Origin: origin, Obligations: obligations, DistributionTargets: append([]string{}, targets...), OwnerIDs: append([]string{}, owners...)}
		if rule, ok := rules[material]; ok {
			f.PolicyRuleDigest = provenanceRuleDigest(rule)
		}
		if n, ok := nodes[node]; ok {
			if n.Kind == "dependency" {
				f.DependencyRevision = n.Revision
			}
			if n.Kind == "tool" {
				f.ToolRevision = n.Revision
			}
		}
		out = append(out, f)
	}
	for _, n := range g.Nodes {
		material := provenanceMaterialKind(n)
		if material == "" {
			continue
		}
		rule, ok := rules[material]
		if !ok {
			add("missing_policy", "blocking", material, n.ID, "No effective policy rule covers this changed material.", n.License, "", n.Obligations, nil)
			continue
		}
		origin := strings.Join(origins[n.ID], ", ")
		if origin == "" {
			add("unattributed_material", "blocking", material, n.ID, "No attributable production origin reaches this material.", n.License, "", n.Obligations, rule.ReviewOwnerIDs)
		}
		if !containsFold(rule.PermittedLicenses, n.License) || containsFold(rule.ProhibitedLicenses, n.License) {
			add("incompatible_license", "blocking", material, n.ID, "The declared license is unknown, prohibited, or outside the permitted set.", n.License, origin, n.Obligations, rule.ReviewOwnerIDs)
		}
		if generated[n.ID] != "" {
			add("generated_output", "warning", material, n.ID, "Generated output requires its producing tool and inputs to remain attributable.", n.License, generated[n.ID], n.Obligations, rule.ReviewOwnerIDs)
		}
		for _, o := range append(append([]string{}, n.Obligations...), rule.RequiredAttribution...) {
			k := "required_notice"
			if strings.Contains(strings.ToLower(o), "source") {
				k = "source_offer"
			}
			add(k, "blocking", material, n.ID, "Distribution obligation requires an owner acknowledgement: "+o, n.License, origin, []string{o}, rule.ReviewOwnerIDs)
		}
		for _, att := range rule.ContributorAttestations {
			add("contributor_attestation", "blocking", material, n.ID, "Required contributor attestation: "+att, n.License, origin, []string{att}, rule.ReviewOwnerIDs)
		}
	}
	for _, d := range g.Diagnostics {
		if d.Severity == "blocking" {
			add(d.Kind, "blocking", "source", d.NodeID, d.Message, "", "", nil, p.OwnerIDs)
		}
	}
	return out
}
func provenanceRuleDigest(rule provenancepolicies.MaterialRule) string {
	b, _ := json.Marshal(rule)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func provenanceMaterialKind(n provenancegraphs.Node) string {
	switch n.Kind {
	case "file", "fragment", "commit":
		return "source"
	case "asset":
		return "asset"
	case "dependency":
		return "package"
	case "build_step", "tool":
		return "build_input"
	case "artifact":
		return "generated_code"
	}
	return ""
}
func containsFold(xs []string, v string) bool {
	for _, x := range xs {
		if strings.EqualFold(strings.TrimSpace(x), strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}
func writeProvenanceAssessment(w http.ResponseWriter, a provenanceassessments.Assessment, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, a)
	case errors.Is(e, provenanceassessments.ErrNotFound):
		writeAPIError(w, 404, "provenance_assessment_not_found", "provenance assessment not found")
	case errors.Is(e, provenanceassessments.ErrConflict):
		writeAPIError(w, 409, "provenance_assessment_conflict", "the assessment changed or request identity was reused")
	case errors.Is(e, provenanceassessments.ErrForbidden):
		writeAPIError(w, 403, "provenance_owner_required", "a current human provenance owner is required")
	case errors.Is(e, provenanceassessments.ErrInvalid):
		writeAPIError(w, 400, "invalid_provenance_assessment", "candidate evidence, citations, acknowledgements, and exceptions must be complete")
	default:
		writeAPIError(w, 500, "provenance_assessments_unavailable", "provenance assessment could not be persisted")
	}
}

var _ = time.Time{}
