package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changestacks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type stackRevisionRef struct {
	MemberID      string `json:"member_id"`
	PullRequestID string `json:"pull_request_id,omitempty"`
	Revision      string `json:"revision"`
	Current       bool   `json:"current"`
}
type stackEvidence struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id,omitempty"`
	Revision  string    `json:"revision"`
	State     string    `json:"state"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}
type stackPullContext struct {
	StackID               string             `json:"stack_id"`
	StackTitle            string             `json:"stack_title"`
	Outcome               string             `json:"outcome"`
	MemberID              string             `json:"member_id"`
	Position              int                `json:"position"`
	Revision              string             `json:"revision"`
	ParentRevision        string             `json:"parent_revision"`
	TargetRevision        string             `json:"target_revision"`
	ReviewState           string             `json:"review_state"`
	IndividualScope       changestacks.Scope `json:"individual_diff"`
	CumulativeScope       changestacks.Scope `json:"cumulative_diff"`
	CommitIDs             []string           `json:"commit_ids"`
	Upstream              []stackRevisionRef `json:"upstream_revisions"`
	Evidence              []stackEvidence    `json:"evidence"`
	DownstreamInvalidated []stackRevisionRef `json:"downstream_evidence_invalidated_by_upstream_change"`
	AcceptanceCriteria    []string           `json:"acceptance_criteria"`
	Authority             string             `json:"authority"`
}

func registerChangeStackRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, organizationsStore *organizations.Store, credentials *auth.Store, stacks *changestacks.Store, pulls *pullrequests.Store, checks *checkruns.Store, previewStore *previews.Store) {
	project := func(v changestacks.Stack, actor auth.Credential, authenticated bool) changestacks.Stack {
		participant := func(repositoryID string) bool {
			if !authenticated {
				return false
			}
			repo, e := catalog.GetByID(repositoryID)
			if e != nil {
				return false
			}
			ok, _ := catalog.HasCollaborator(actor.UserID, repositoryID)
			return repo.OwnerID == actor.UserID || ok
		}
		v.Diagnostics = nil
		ids := map[string]bool{}
		graph := map[string][]string{}
		revisions := map[string]string{}
		patches := map[string]string{}
		targetRepo, e := git.Open(v.RepositoryID)
		authorizedRangeRepositories := []string{}
		if e != nil {
			v.Diagnostics = append(v.Diagnostics, diag("target_unavailable", "target Git history is unavailable", true))
		}
		if e == nil {
			authorizedRangeRepositories = append(authorizedRangeRepositories, targetRepo.Path())
			if tip, x := gitOutput(targetRepo.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(v.TargetBranch, "refs/heads/")+"^{commit}"); x == nil {
				v.TargetRevision = tip
			} else {
				v.TargetRevision = ""
				v.Diagnostics = append(v.Diagnostics, diag("target_missing", "target branch does not resolve to a commit", true))
			}
		}
		for i := range v.Members {
			m := &v.Members[i]
			m.Diagnostics = nil
			m.Position = i + 1
			ids[m.ID] = true
			graph[m.ID] = append([]string(nil), m.DependsOn...)
			sourceID := m.SourceRepositoryID
			if sourceID == "" {
				sourceID = v.RepositoryID
			}
			accessible := sourceID == v.RepositoryID || participant(sourceID)
			m.Permissions = changestacks.Permission{Read: accessible, Publish: participant(v.RepositoryID) && accessible, Review: participant(v.RepositoryID), Push: participant(sourceID)}
			if !accessible {
				m.Diagnostics = append(m.Diagnostics, diag("branch_inaccessible", "source branch is outside the caller's repository access", true))
				m.Revision = ""
				continue
			}
			source, e := git.Open(sourceID)
			if e != nil {
				m.Diagnostics = append(m.Diagnostics, diag("branch_inaccessible", "source repository is unavailable", true))
				m.Revision = ""
				continue
			}
			authorizedRangeRepositories = appendUniquePath(authorizedRangeRepositories, source.Path())
			if m.PullRequestID != "" {
				p, x := pulls.Get(v.RepositoryID, m.PullRequestID)
				if x != nil || p.SourceRepositoryID != sourceID || p.SourceBranch != m.SourceBranch {
					m.Diagnostics = append(m.Diagnostics, diag("pull_mismatch", "pull request does not identify this source branch", true))
					m.Revision = ""
				} else if p.SourceCommitID != m.Revision {
					m.Diagnostics = append(m.Diagnostics, diag("revision_moved", "pull request moved after this exact stack revision was published", true))
					m.ReviewState = "stale"
				}
			}
			if m.Revision == "" || !commitExists(source.Path(), m.Revision) {
				m.Diagnostics = append(m.Diagnostics, diag("commit_missing", "published revision is missing from the source repository", true))
				continue
			}
			if prior := revisions[m.Revision]; prior != "" {
				m.Diagnostics = append(m.Diagnostics, diag("duplicate_change", "revision duplicates stack member "+prior, true))
			} else {
				revisions[m.Revision] = m.ID
			}
			base := v.TargetRevision
			baseRepositoryPath := ""
			if targetRepo != nil {
				baseRepositoryPath = targetRepo.Path()
			}
			if i > 0 {
				base = v.Members[i-1].Revision
				previousSourceID := v.Members[i-1].SourceRepositoryID
				if previousSourceID == "" {
					previousSourceID = v.RepositoryID
				}
				if previousSourceID == v.RepositoryID || participant(previousSourceID) {
					if previous, openErr := git.Open(previousSourceID); openErr == nil {
						baseRepositoryPath = previous.Path()
					}
				} else {
					baseRepositoryPath = ""
				}
			}
			m.ExpectedBaseRevision = base
			if m.BaseRevision != "" && m.BaseRevision != base {
				m.Diagnostics = append(m.Diagnostics, diag("base_mismatch", "pull request base does not match the preceding stack revision", true))
			}
			if base == "" || baseRepositoryPath == "" || !commitExists(baseRepositoryPath, base) {
				m.Diagnostics = append(m.Diagnostics, diag("base_missing", "declared parent revision is unavailable", true))
				continue
			}
			rangePath, cleanup, rangeErr := stackRangeView(source.Path(), baseRepositoryPath, base, m.Revision, authorizedRangeRepositories...)
			if rangeErr != nil {
				m.Diagnostics = append(m.Diagnostics, diag("base_missing", "declared parent objects could not be assembled", true))
				continue
			}
			if _, x := gitOutput(rangePath, "merge-base", "--is-ancestor", base, m.Revision); x != nil {
				m.Diagnostics = append(m.Diagnostics, diag("unrelated_history", "revision does not descend from its declared parent", true))
			}
			m.IndividualScope = stackScope(rangePath, base, m.Revision)
			m.CommitIDs = stackCommits(rangePath, base, m.Revision)
			m.Authors = stackAuthors(rangePath, base, m.Revision)
			diff, _ := gitOutput(rangePath, "diff", "--binary", base, m.Revision)
			cleanup()
			if targetRepo != nil {
				if cumulativePath, cumulativeCleanup, cumulativeErr := stackRangeView(source.Path(), targetRepo.Path(), v.TargetRevision, m.Revision, authorizedRangeRepositories...); cumulativeErr == nil {
					m.CumulativeScope = stackScope(cumulativePath, v.TargetRevision, m.Revision)
					cumulativeCleanup()
				}
			}
			sum := sha256.Sum256([]byte(diff))
			key := hex.EncodeToString(sum[:])
			if prior := patches[key]; key != "" && prior != "" {
				m.Diagnostics = append(m.Diagnostics, diag("duplicate_change", "change duplicates stack member "+prior, true))
			} else {
				patches[key] = m.ID
			}
		}
		for i := range v.Members {
			for _, d := range v.Members[i].DependsOn {
				if !ids[d] {
					v.Members[i].Diagnostics = append(v.Members[i].Diagnostics, diag("dependency_missing", "declared dependency "+d+" is not in this stack", true))
				}
			}
			if stackCycle(v.Members[i].ID, graph, map[string]bool{}, map[string]bool{}) {
				v.Members[i].Diagnostics = append(v.Members[i].Diagnostics, diag("dependency_cycle", "declared dependencies contain a cycle", true))
			}
		}
		for _, m := range v.Members {
			v.Diagnostics = append(v.Diagnostics, m.Diagnostics...)
		}
		visible := map[string]bool{}
		current := map[string]string{}
		for _, m := range v.Members {
			visible[m.ID] = m.Permissions.Read
			current[m.ID] = m.Revision
		}
		assignments := v.Assignments[:0]
		for _, a := range v.Assignments {
			if visible[a.MemberID] {
				assignments = append(assignments, a)
			}
		}
		v.Assignments = assignments
		launches := v.WorkLaunches[:0]
		for _, launch := range v.WorkLaunches {
			if visible[launch.MemberID] {
				launch.CurrentUpstream, launch.ChangedUpstream = stackUpstreamCurrent(launch.UpstreamRevisions, current)
				launches = append(launches, launch)
			}
		}
		v.WorkLaunches = launches
		timeline := v.Timeline[:0]
		for _, event := range v.Timeline {
			if visible[event.MemberID] {
				event.CurrentUpstream, event.ChangedUpstream = stackUpstreamCurrent(event.UpstreamRevisions, current)
				timeline = append(timeline, event)
			}
		}
		v.Timeline = timeline
		for i := range v.IntegrationCandidates {
			candidate := &v.IntegrationCandidates[i]
			if candidate.Status == "merged" {
				continue
			}
			if candidate.SupersededAt != nil {
				candidate.Status = "superseded"
				continue
			}
			pullID := ""
			for _, member := range v.Members {
				if member.ID == candidate.MemberID {
					pullID = member.PullRequestID
					break
				}
			}
			runs, runErr := checks.List(v.RepositoryID, pullID)
			if runErr != nil {
				candidate.Status = "evidence_unavailable"
				continue
			}
			candidate.CheckRunIDs = []string{}
			requiredChecks := map[string]bool{}
			for _, name := range candidate.RequiredChecks {
				requiredChecks[name] = true
			}
			for _, run := range runs {
				if run.CommitID != candidate.CandidateRevision || !requiredChecks[run.Definition.Name] {
					continue
				}
				candidate.CheckRunIDs = append(candidate.CheckRunIDs, run.ID)
			}
			candidate.Status = stackIntegrationCheckStatus(*candidate, runs)
		}
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/change-stacks", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		xs, e := stacks.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "change_stacks_unavailable", "change stacks could not be read")
			return
		}
		for i := range xs {
			xs[i] = project(xs[i], actor, authenticated)
		}
		writeJSON(w, 200, map[string]any{"change_stacks": xs})
	})
	mux.HandleFunc("GET /repositories/{id}/change-stacks/{stack_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := stacks.Get(r.PathValue("id"), r.PathValue("stack_id"))
		if errors.Is(e, changestacks.ErrNotFound) {
			writeAPIError(w, 404, "change_stack_not_found", "change stack not found")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "change_stacks_unavailable", "change stack could not be read")
			return
		}
		writeJSON(w, 200, project(v, actor, authenticated))
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/stack-context", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		items, err := stacks.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "change_stacks_unavailable", "stack review context could not be read")
			return
		}
		contexts := []stackPullContext{}
		for _, retained := range items {
			v := project(retained, actor, authenticated)
			for i := range v.Members {
				if v.Members[i].PullRequestID == r.PathValue("pull_id") {
					contexts = append(contexts, buildStackPullContext(v, i, pulls, checks, previewStore))
					break
				}
			}
		}
		writeJSON(w, 200, map[string]any{"stack_contexts": contexts})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/stack-context/owner-acknowledgements", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		repo, err := catalog.GetByID(r.PathValue("id"))
		if err != nil || actor.AgentID != "" || repo.OwnerID != actor.UserID {
			writeAPIError(w, 403, "stack_acknowledgement_forbidden", "only the current human repository owner may acknowledge a stack layer")
			return
		}
		var in struct {
			StackID  string `json:"stack_id"`
			MemberID string `json:"member_id"`
			Decision string `json:"decision"`
			Note     string `json:"note"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a stack layer acknowledgement is required")
			return
		}
		v, err := stacks.Get(r.PathValue("id"), in.StackID)
		if err != nil {
			writeAPIError(w, 404, "change_stack_not_found", "change stack not found")
			return
		}
		matched := false
		memberRevision := ""
		for _, m := range v.Members {
			if m.ID == in.MemberID && m.PullRequestID == r.PathValue("pull_id") {
				matched = true
				memberRevision = m.Revision
			}
		}
		if !matched || memberRevision == "" {
			writeAPIError(w, 422, "stack_layer_mismatch", "the pull request does not identify that stack layer")
			return
		}
		var out changestacks.Stack
		err = pulls.WithSourceRevision(r.PathValue("id"), r.PathValue("pull_id"), memberRevision, func(pullrequests.PullRequest) error {
			var acknowledgeErr error
			out, acknowledgeErr = stacks.Acknowledge(r.PathValue("id"), in.StackID, in.MemberID, actor.UserID, in.Decision, in.Note)
			return acknowledgeErr
		})
		if errors.Is(err, pullrequests.ErrSourceChanged) || errors.Is(err, pullrequests.ErrNotReady) {
			writeAPIError(w, 409, "stack_layer_stale", "the pull request moved or closed; refresh the stack before acknowledging its layer")
			return
		}
		if errors.Is(err, changestacks.ErrInvalid) {
			writeAPIError(w, 422, "stack_acknowledgement_invalid", "decision must acknowledge or request changes on the exact current layer")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "change_stacks_unavailable", "acknowledgement could not be retained")
			return
		}
		writeJSON(w, 201, project(out, actor, true))
	})
	mux.HandleFunc("POST /repositories/{id}/change-stacks", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "change_stack_agent_forbidden", "a human contributor must publish a change stack")
			return
		}
		var in changestacks.Stack
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a change stack is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		clean := in
		clean.ID, clean.RequestDigest, clean.CreatedBy, clean.Authority = "", "", "", ""
		clean.CreatedAt, clean.TargetRevision, clean.Diagnostics = time.Time{}, "", nil
		for i := range clean.Members {
			clean.Members[i].Position = 0
			clean.Members[i].Revision, clean.Members[i].BaseRevision, clean.Members[i].ExpectedBaseRevision = "", "", ""
			clean.Members[i].Authors, clean.Members[i].Diagnostics = nil, nil
			clean.Members[i].CommitIDs = nil
			in.Members[i].CommitIDs = nil
			clean.Members[i].IndividualScope, clean.Members[i].CumulativeScope = changestacks.Scope{}, changestacks.Scope{}
			clean.Members[i].Permissions, clean.Members[i].ReviewState, clean.Members[i].PublishedAt = changestacks.Permission{}, "", nil
			clean.Members[i].Acknowledgements = nil
			in.Members[i].Acknowledgements = nil
		}
		body, _ := json.Marshal(clean)
		digest := sha256.Sum256(body)
		in.RequestDigest = hex.EncodeToString(digest[:])
		target, e := git.Open(in.RepositoryID)
		if e != nil {
			writeAPIError(w, 422, "change_stack_target_missing", "target repository is unavailable")
			return
		}
		targetRevision, e := gitOutput(target.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(in.TargetBranch, "refs/heads/")+"^{commit}")
		if e != nil {
			writeAPIError(w, 422, "change_stack_target_missing", "target branch must resolve to a commit")
			return
		}
		in.TargetRevision = targetRevision
		for i := range in.Members {
			m := &in.Members[i]
			sourceID := m.SourceRepositoryID
			if sourceID == "" {
				sourceID = in.RepositoryID
				m.SourceRepositoryID = sourceID
			}
			sourceCatalog, catalogErr := catalog.GetByID(sourceID)
			allowed := catalogErr == nil && sourceCatalog.OwnerID == actor.UserID
			if !allowed {
				allowed, _ = catalog.HasCollaborator(actor.UserID, sourceID)
			}
			if !allowed {
				continue
			}
			source, openErr := git.Open(sourceID)
			if openErr != nil {
				continue
			}
			rev, resolveErr := gitOutput(source.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(m.SourceBranch, "refs/heads/")+"^{commit}")
			if resolveErr != nil {
				continue
			}
			m.Revision, m.BaseRevision = rev, targetRevision
			if i > 0 && in.Members[i-1].Revision != "" {
				m.BaseRevision = in.Members[i-1].Revision
			}
			m.ReviewState = "pending"
			if m.PullRequestID != "" {
				if p, pullErr := pulls.Get(in.RepositoryID, m.PullRequestID); pullErr == nil {
					m.Revision, m.BaseRevision, m.SourceBranch, m.SourceRepositoryID = p.SourceCommitID, p.TargetCommitID, p.SourceBranch, p.SourceRepositoryID
					now := time.Now().UTC()
					m.PublishedAt, m.ReviewState = &now, "published"
				}
			}
		}
		out, e := stacks.Create(in, actor.UserID)
		if errors.Is(e, changestacks.ErrInvalid) {
			writeAPIError(w, 422, "change_stack_invalid", "title, shared outcome, target, ordered changes, and per-change acceptance criteria are required")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "change_stacks_unavailable", "change stack could not be published")
			return
		}
		for i := range out.Members {
			m := &out.Members[i]
			if m.PullRequestID != "" || m.Revision == "" {
				continue
			}
			sourceRepo, catalogErr := catalog.GetByID(m.SourceRepositoryID)
			allowed := catalogErr == nil && sourceRepo.OwnerID == actor.UserID
			if !allowed {
				allowed, _ = catalog.HasCollaborator(actor.UserID, m.SourceRepositoryID)
			}
			if !allowed {
				continue
			}
			targetBranch := out.TargetBranch
			if i > 0 {
				targetBranch = out.Members[i-1].SourceBranch
			}
			marker := "Change stack: " + out.ID + "/" + m.ID
			var p pullrequests.PullRequest
			existingPulls, listErr := pulls.List(out.RepositoryID)
			if listErr != nil {
				writeAPIError(w, 503, "change_stack_publication_pending", "pull reconciliation is unavailable; retry with the same request_id")
				return
			}
			for _, existing := range existingPulls {
				if existing.AuthorID == actor.UserID && existing.SourceRepositoryID == m.SourceRepositoryID && existing.SourceBranch == m.SourceBranch && existing.TargetBranch == targetBranch && strings.Contains(existing.Body, marker) {
					p = existing
					break
				}
			}
			if p.ID == "" {
				var createErr error
				p, createErr = pulls.CreateFrom(out.RepositoryID, m.SourceRepositoryID, actor.UserID, m.Title, out.Outcome+"\n\nAcceptance criteria:\n- "+strings.Join(m.AcceptanceCriteria, "\n- ")+"\n\n"+marker, m.SourceBranch, targetBranch, nil)
				if createErr != nil && !errors.Is(createErr, pullrequests.ErrDurabilityUncertain) {
					writeAPIError(w, 409, "change_stack_publication_pending", "a member pull could not be published; restore its branch and retry with the same request_id")
					return
				}
			}
			now := time.Now().UTC()
			m.PullRequestID, m.Revision, m.BaseRevision, m.PublishedAt, m.ReviewState = p.ID, p.SourceCommitID, p.TargetCommitID, &now, "published"
			if updateErr := stacks.Update(out); updateErr != nil {
				writeAPIError(w, 500, "change_stack_publication_pending", "the reserved stack could not retain its pull publication; retry with the same request_id")
				return
			}
		}
		writeJSON(w, 201, project(out, actor, true))
	})
	mux.HandleFunc("POST /repositories/{id}/change-stacks/{stack_id}/integration-candidates", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner || actor.AgentID != "" {
			writeAPIError(w, 403, "stack_integration_forbidden", "only a current human repository maintainer may prepare stack integration")
			return
		}
		var in struct {
			RequestID       string `json:"request_id"`
			ThroughPosition int    `json:"through_position"`
		}
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.RequestID) == "" {
			writeAPIError(w, 400, "invalid_request", "request_id and through_position are required")
			return
		}
		retained, err := stacks.Get(r.PathValue("id"), r.PathValue("stack_id"))
		if err != nil {
			writeAPIError(w, 404, "change_stack_not_found", "change stack not found")
			return
		}
		if in.ThroughPosition < 1 || in.ThroughPosition > len(retained.Members) {
			writeAPIError(w, 422, "stack_prefix_invalid", "through_position must identify a complete stack prefix")
			return
		}
		digestBody, _ := json.Marshal(in)
		digestBytes := sha256.Sum256(digestBody)
		digest := hex.EncodeToString(digestBytes[:])
		for _, prior := range retained.IntegrationCandidates {
			if prior.RequestID == in.RequestID {
				if prior.RequestDigest != digest {
					writeAPIError(w, 409, "stack_integration_request_conflict", "request_id was already used for different input")
					return
				}
				writeJSON(w, 200, prior)
				return
			}
		}
		repository, err := git.Open(retained.RepositoryID)
		if err != nil {
			writeAPIError(w, 422, "stack_target_missing", "target repository is unavailable")
			return
		}
		target, err := gitOutput(repository.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(retained.TargetBranch, "refs/heads/")+"^{commit}")
		if err != nil {
			writeAPIError(w, 422, "stack_target_missing", "target branch is unavailable")
			return
		}
		parent := target
		revisions := map[string]string{}
		landedPosition := stackMergedPosition(retained)
		if landedPosition >= in.ThroughPosition {
			writeAPIError(w, 409, "stack_prefix_already_integrated", "the selected prefix is already integrated")
			return
		}
		for i := 0; i < landedPosition; i++ {
			revisions[retained.Members[i].ID] = retained.Members[i].Revision
		}
		var candidate changestacks.IntegrationCandidate
		for i := landedPosition; i < in.ThroughPosition; i++ {
			member := retained.Members[i]
			if member.SourceRepositoryID != "" && member.SourceRepositoryID != retained.RepositoryID {
				writeAPIError(w, 409, "stack_member_independently_owned", "cross-repository members must integrate through their ordinary owner")
				return
			}
			pull, pullErr := pulls.Get(retained.RepositoryID, member.PullRequestID)
			if pullErr != nil || pull.Status != pullrequests.Open || pull.SourceCommitID != member.Revision {
				writeAPIError(w, 409, "stack_prefix_stale", "a prefix member is missing, closed, or moved")
				return
			}
			readiness, readyErr := pulls.Readiness(retained.RepositoryID, member.PullRequestID, true)
			if readyErr != nil || !readiness.CanMerge {
				writeAPIError(w, 409, "stack_prefix_not_ready", "ordinary review, checks, preview, or policy does not permit this prefix")
				return
			}
			tree, mergeErr := stackMergeTree(repository, parent, member.Revision)
			if mergeErr != nil {
				writeAPIError(w, 409, "stack_prefix_conflict", "this member conflicts with the candidate history")
				return
			}
			stamp := "0 +0000"
			content := fmt.Sprintf("tree %s\nparent %s\nparent %s\nauthor Vivarium Stack Integration <stack@vivarium> %s\ncommitter Vivarium Stack Integration <stack@vivarium> %s\n\nVerify stack %s through member %s\n", tree, parent, member.Revision, stamp, stamp, retained.ID, member.ID)
			written, writeErr := repository.WriteObject(storage.CommitObject, []byte(content))
			if writeErr != nil {
				writeAPIError(w, 500, "stack_integration_unavailable", "candidate could not be assembled")
				return
			}
			parent = string(written)
			revisions[member.ID] = member.Revision
		}
		required, err := catalog.RequiredChecks(retained.RepositoryID, retained.TargetBranch)
		if err != nil {
			writeAPIError(w, 500, "stack_integration_unavailable", "required policy could not be resolved")
			return
		}
		definitions, err := stackRequiredDefinitions(repository, parent, required)
		if err != nil {
			writeAPIError(w, 409, "stack_candidate_policy_invalid", "candidate does not contain every required check definition")
			return
		}
		runs, err := checks.CreateRequested(retained.RepositoryID, retained.Members[in.ThroughPosition-1].PullRequestID, parent, definitions, actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "stack_integration_unavailable", "candidate checks could not be reserved")
			return
		}
		ids := make([]string, len(runs))
		for i := range runs {
			ids[i] = runs[i].ID
		}
		candidate = changestacks.IntegrationCandidate{RequestID: in.RequestID, RequestDigest: digest, TargetRevision: target, MemberRevisions: revisions, MemberID: retained.Members[in.ThroughPosition-1].ID, Position: in.ThroughPosition, ParentRevision: target, CandidateRevision: parent, RequiredChecks: required, CheckRunIDs: ids, Status: "verifying", CreatedBy: actor.UserID}
		_, candidate, err = stacks.RetainIntegration(retained.RepositoryID, retained.ID, candidate)
		if err != nil {
			writeAPIError(w, 500, "stack_integration_unavailable", "candidate could not be retained")
			return
		}
		startBoundCheckRuns(git, checks, retained.RepositoryID, retained.Members[in.ThroughPosition-1].PullRequestID, parent, definitions)
		writeJSON(w, 201, candidate)
	})
	mux.HandleFunc("POST /repositories/{id}/change-stacks/{stack_id}/integration-candidates/{candidate_id}/merge", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner || actor.AgentID != "" {
			writeAPIError(w, 403, "stack_integration_forbidden", "only a current human repository maintainer may integrate a stack")
			return
		}
		stack, err := stacks.Get(r.PathValue("id"), r.PathValue("stack_id"))
		if err != nil {
			writeAPIError(w, 404, "change_stack_not_found", "change stack not found")
			return
		}
		var candidate *changestacks.IntegrationCandidate
		for i := range stack.IntegrationCandidates {
			if stack.IntegrationCandidates[i].ID == r.PathValue("candidate_id") {
				candidate = &stack.IntegrationCandidates[i]
				break
			}
		}
		if candidate == nil {
			writeAPIError(w, 404, "stack_candidate_not_found", "integration candidate not found")
			return
		}
		repository, err := git.Open(stack.RepositoryID)
		if err != nil {
			writeAPIError(w, 409, "stack_target_missing", "target repository is unavailable")
			return
		}
		live, _ := gitOutput(repository.Path(), "rev-parse", "--verify", "refs/heads/"+strings.TrimPrefix(stack.TargetBranch, "refs/heads/")+"^{commit}")
		if candidate.Status == "merged" {
			_, completed, completeErr := stacks.CompleteIntegration(stack.RepositoryID, stack.ID, candidate.ID, actor.UserID)
			if completeErr != nil {
				writeAPIError(w, 500, "stack_integration_publication_uncertain", "target advanced but integration metadata requires retry")
				return
			}
			writeJSON(w, 200, completed)
			return
		}
		if live == candidate.CandidateRevision {
			for i := stackMergedPosition(stack); i < candidate.Position; i++ {
				m := stack.Members[i]
				if _, recordErr := pulls.RecordStackMerge(stack.RepositoryID, m.PullRequestID, candidate.MemberRevisions[m.ID], candidate.CandidateRevision, actor.UserID); recordErr != nil {
					writeAPIError(w, 500, "stack_integration_publication_uncertain", "target advanced but pull reconciliation requires retry")
					return
				}
			}
			_, completed, completeErr := stacks.CompleteIntegration(stack.RepositoryID, stack.ID, candidate.ID, actor.UserID)
			if completeErr != nil {
				writeAPIError(w, 500, "stack_integration_publication_uncertain", "target advanced but integration metadata requires retry")
				return
			}
			writeJSON(w, 200, completed)
			return
		}
		if candidate.SupersededAt != nil || live != candidate.TargetRevision {
			writeAPIError(w, 409, "stack_candidate_stale", "target moved; prepare a new exact candidate")
			return
		}
		runs, _ := checks.List(stack.RepositoryID, stack.Members[candidate.Position-1].PullRequestID)
		if stackIntegrationCheckStatus(*candidate, runs) != "passed" {
			writeAPIError(w, 409, "stack_candidate_not_ready", "cumulative verification has not passed")
			return
		}
		landedPosition := stackMergedPosition(stack)
		for i := landedPosition; i < candidate.Position; i++ {
			m := stack.Members[i]
			p, pullErr := pulls.Get(stack.RepositoryID, m.PullRequestID)
			ready, readyErr := pulls.Readiness(stack.RepositoryID, m.PullRequestID, true)
			if pullErr != nil || readyErr != nil || p.Status != pullrequests.Open || p.SourceCommitID != candidate.MemberRevisions[m.ID] || !ready.CanMerge {
				writeAPIError(w, 409, "stack_prefix_stale", "review, checks, policy, or a member revision changed")
				return
			}
		}
		if err := repository.UpdateReferenceIfTarget(storage.Reference{Name: "refs/heads/" + strings.TrimPrefix(stack.TargetBranch, "refs/heads/"), Target: candidate.CandidateRevision}, candidate.TargetRevision); err != nil {
			writeAPIError(w, 409, "stack_candidate_stale", "target changed concurrently; prepare a new candidate")
			return
		}
		for i := landedPosition; i < candidate.Position; i++ {
			m := stack.Members[i]
			if _, err := pulls.RecordStackMerge(stack.RepositoryID, m.PullRequestID, candidate.MemberRevisions[m.ID], candidate.CandidateRevision, actor.UserID); err != nil {
				writeAPIError(w, 500, "stack_integration_publication_uncertain", "target advanced but pull reconciliation requires retry")
				return
			}
		}
		_, completed, err := stacks.CompleteIntegration(stack.RepositoryID, stack.ID, candidate.ID, actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "stack_integration_publication_uncertain", "target advanced but integration metadata requires retry")
			return
		}
		writeJSON(w, 200, completed)
	})
	registerChangeStackRestackRoutes(mux, git, catalog, credentials, stacks, pulls, checks)
	registerChangeStackCollaborationRoutes(mux, catalog, organizationsStore, credentials, stacks)
}

func stackIntegrationCheckStatus(candidate changestacks.IntegrationCandidate, runs []checkruns.Run) string {
	state := "passed"
	required := map[string]bool{}
	for _, name := range candidate.RequiredChecks {
		required[name] = true
	}
	matched := map[string]bool{}
	for _, run := range runs {
		if run.CommitID != candidate.CandidateRevision || !required[run.Definition.Name] {
			continue
		}
		matched[run.Definition.Name] = true
		if run.State == "failed" || run.State == "canceled" {
			state = "failed"
		} else if run.State != "succeeded" && state != "failed" {
			state = "verifying"
		}
	}
	if len(matched) < len(required) && state != "failed" {
		state = "verifying"
	}
	return state
}

func stackMergedPosition(stack changestacks.Stack) int {
	position := 0
	for _, candidate := range stack.IntegrationCandidates {
		if candidate.Status == "merged" && candidate.Position > position {
			position = candidate.Position
		}
	}
	return position
}

func stackUpstreamCurrent(frozen, current map[string]string) (bool, []string) {
	changed := []string{}
	for id, revision := range frozen {
		if current[id] != revision {
			changed = append(changed, id)
		}
	}
	sort.Strings(changed)
	return len(changed) == 0, changed
}

func appendUniquePath(paths []string, candidate string) []string {
	for _, existing := range paths {
		if existing == candidate {
			return paths
		}
	}
	return append(paths, candidate)
}

func buildStackPullContext(v changestacks.Stack, index int, pulls *pullrequests.Store, checks *checkruns.Store, previewStore *previews.Store) stackPullContext {
	m := v.Members[index]
	ctx := stackPullContext{StackID: v.ID, StackTitle: v.Title, Outcome: v.Outcome, MemberID: m.ID, Position: m.Position, Revision: m.Revision, ParentRevision: m.ExpectedBaseRevision, TargetRevision: v.TargetRevision, IndividualScope: m.IndividualScope, CumulativeScope: m.CumulativeScope, CommitIDs: []string{}, Upstream: []stackRevisionRef{}, Evidence: []stackEvidence{}, DownstreamInvalidated: []stackRevisionRef{}, AcceptanceCriteria: m.AcceptanceCriteria, Authority: v.Authority}
	blocked := false
	for _, d := range m.Diagnostics {
		blocked = blocked || d.Blocking
	}
	provisional := false
	for i := 0; i < index; i++ {
		u := v.Members[i]
		current := u.Revision != "" && u.ReviewState != "stale" && !hasBlockingStackDiagnostic(u.Diagnostics)
		ctx.Upstream = append(ctx.Upstream, stackRevisionRef{MemberID: u.ID, PullRequestID: u.PullRequestID, Revision: u.Revision, Current: current})
		approved := false
		if u.PullRequestID != "" {
			if reviews, err := pulls.ListReviews(v.RepositoryID, u.PullRequestID); err == nil {
				for _, review := range reviews {
					if review.Decision == pullrequests.Approved && review.ReviewedCommitID == u.Revision && !review.Stale {
						approved = true
					}
				}
			}
		}
		if !current || !approved {
			provisional = true
		}
	}
	if blocked {
		ctx.ReviewState = "blocked"
	} else if provisional {
		ctx.ReviewState = "provisional"
	} else {
		ctx.ReviewState = "reviewable_now"
	}
	for i := index + 1; i < len(v.Members); i++ {
		d := v.Members[i]
		ctx.DownstreamInvalidated = append(ctx.DownstreamInvalidated, stackRevisionRef{MemberID: d.ID, PullRequestID: d.PullRequestID, Revision: d.Revision, Current: d.ReviewState != "stale" && !hasBlockingStackDiagnostic(d.Diagnostics)})
	}
	ctx.CommitIDs = append(ctx.CommitIDs, m.CommitIDs...)
	if comments, err := pulls.ListComments(v.RepositoryID, m.PullRequestID); err == nil {
		for _, x := range comments {
			state := "current"
			if x.Revision == "" {
				state = "unbound"
			} else if x.Revision != m.Revision {
				state = "stale"
			}
			ctx.Evidence = append(ctx.Evidence, stackEvidence{ID: x.ID, Kind: "discussion", ActorID: x.AuthorID, Revision: x.Revision, State: state, Summary: x.Body, CreatedAt: x.CreatedAt})
		}
	}
	if reviews, err := pulls.ListReviews(v.RepositoryID, m.PullRequestID); err == nil {
		for _, x := range reviews {
			state := "current"
			if x.Stale || x.ReviewedCommitID != m.Revision {
				state = "stale"
			}
			ctx.Evidence = append(ctx.Evidence, stackEvidence{ID: x.ID, Kind: "review_decision", ActorID: x.ReviewerID, Revision: x.ReviewedCommitID, State: state, Summary: x.Decision, CreatedAt: x.UpdatedAt})
		}
	}
	for _, x := range m.Acknowledgements {
		state := "current"
		if x.Revision != m.Revision || !sameUpstreamSnapshot(x.UpstreamRevisions, ctx.Upstream) {
			state = "stale"
		}
		ctx.Evidence = append(ctx.Evidence, stackEvidence{ID: x.ID, Kind: "owner_acknowledgement", ActorID: x.OwnerID, Revision: x.Revision, State: state, Summary: x.Decision + stackNote(x.Note), CreatedAt: x.CreatedAt})
	}
	if checks != nil {
		if runs, err := checks.List(v.RepositoryID, m.PullRequestID); err == nil {
			for _, x := range runs {
				state := "current"
				if x.CommitID != m.Revision {
					state = "stale"
				}
				ctx.Evidence = append(ctx.Evidence, stackEvidence{ID: x.ID, Kind: "check", ActorID: x.RequestedBy, Revision: x.CommitID, State: state, Summary: x.Definition.Name + ": " + x.State, CreatedAt: x.CreatedAt})
			}
		}
	}
	if previewStore != nil {
		if ps, err := previewStore.List(v.RepositoryID, m.PullRequestID, m.Revision); err == nil {
			for _, p := range ps {
				state := "current"
				if p.Stale || p.Revision != m.Revision {
					state = "stale"
				}
				ctx.Evidence = append(ctx.Evidence, stackEvidence{ID: p.ID, Kind: "preview", ActorID: p.CreatorID, Revision: p.Revision, State: state, Summary: p.State, CreatedAt: p.CreatedAt})
				for _, f := range p.Findings {
					kind := "finding"
					ctx.Evidence = append(ctx.Evidence, stackEvidence{ID: f.ID, Kind: kind, ActorID: f.AuthorID, Revision: f.Revision, State: state, Summary: f.Title + ": " + f.Status, CreatedAt: f.CreatedAt})
				}
			}
		}
	}
	sort.Slice(ctx.Evidence, func(i, j int) bool { return ctx.Evidence[i].CreatedAt.Before(ctx.Evidence[j].CreatedAt) })
	return ctx
}
func hasBlockingStackDiagnostic(xs []changestacks.Diagnostic) bool {
	for _, x := range xs {
		if x.Blocking {
			return true
		}
	}
	return false
}
func sameUpstreamSnapshot(snapshot map[string]string, refs []stackRevisionRef) bool {
	if len(snapshot) != len(refs) {
		return false
	}
	for _, r := range refs {
		if snapshot[r.MemberID] != r.Revision || !r.Current {
			return false
		}
	}
	return true
}
func stackNote(note string) string {
	if note == "" {
		return ""
	}
	return ": " + note
}

func stackRequiredDefinitions(repository *storage.Repository, commitID string, required []string) ([]checkruns.Definition, error) {
	if len(required) == 0 {
		return []checkruns.Definition{}, nil
	}
	data, err := exec.Command("git", "--git-dir="+repository.Path(), "show", commitID+":"+checkruns.ConfigPath).Output()
	if err != nil {
		return nil, err
	}
	config, err := checkruns.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	byName := map[string]checkruns.Definition{}
	for _, definition := range config.Checks {
		byName[definition.Name] = definition
	}
	out := make([]checkruns.Definition, 0, len(required))
	for _, name := range required {
		definition, ok := byName[name]
		if !ok {
			return nil, errors.New("required check missing")
		}
		out = append(out, definition)
	}
	return out, nil
}

func stackMergeTree(repository *storage.Repository, target, source string) (storage.ObjectID, error) {
	temporary, err := os.MkdirTemp("", "vivarium-stack-integration-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	objects := filepath.Join(temporary, "objects")
	if err = os.Mkdir(objects, 0700); err != nil {
		return "", err
	}
	env := append(os.Environ(), "GIT_OBJECT_DIRECTORY="+objects, "GIT_ALTERNATE_OBJECT_DIRECTORIES="+filepath.Join(repository.Path(), "objects"))
	command := exec.Command("git", "-C", repository.Path(), "merge-tree", "--write-tree", target, source)
	command.Env = env
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("calculate stack merge tree: %w: %s", err, strings.TrimSpace(string(output)))
	}
	id := storage.ObjectID(strings.TrimSpace(string(output)))
	seen := map[storage.ObjectID]bool{}
	var importTree func(storage.ObjectID) error
	importTree = func(tree storage.ObjectID) error {
		if seen[tree] {
			return nil
		}
		seen[tree] = true
		cat := exec.Command("git", "-C", repository.Path(), "cat-file", "tree", string(tree))
		cat.Env = env
		content, e := cat.Output()
		if e != nil {
			return e
		}
		written, e := repository.WriteObject(storage.TreeObject, content)
		if e != nil || written != tree {
			return fmt.Errorf("import stack merge tree %s: %v", tree, e)
		}
		list := exec.Command("git", "-C", repository.Path(), "ls-tree", "-z", string(tree))
		list.Env = env
		listed, e := list.Output()
		if e != nil {
			return e
		}
		for _, record := range strings.Split(string(listed), "\x00") {
			fields := strings.Fields(record)
			if len(fields) >= 3 && fields[1] == "tree" {
				if e = importTree(storage.ObjectID(fields[2])); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if err = importTree(id); err != nil {
		return "", err
	}
	return id, nil
}

func stackRangeView(headRepositoryPath, baseRepositoryPath, base, head string, authorizedRepositoryPaths ...string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "vivarium-change-stack-range-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if output, initErr := exec.Command("git", "init", "--bare", "--quiet", dir).CombinedOutput(); initErr != nil {
		cleanup()
		return "", func() {}, errors.New(strings.TrimSpace(string(output)))
	}
	seen := map[string]bool{}
	remaining := int64(256 << 20)
	baseSources := appendUniquePath([]string{baseRepositoryPath}, headRepositoryPath)
	headSources := appendUniquePath([]string{headRepositoryPath}, baseRepositoryPath)
	for _, repositoryPath := range authorizedRepositoryPaths {
		baseSources = appendUniquePath(baseSources, repositoryPath)
		headSources = appendUniquePath(headSources, repositoryPath)
	}
	if err = importStackObject(baseSources, dir, base, seen, &remaining); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err = importStackObject(headSources, dir, head, seen, &remaining); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return filepath.Clean(dir), cleanup, nil
}

func importStackObject(sourcePaths []string, destinationPath, id string, seen map[string]bool, remaining *int64) error {
	if seen[id] {
		return nil
	}
	if len(seen) >= 100000 {
		return errors.New("change-stack range exceeds object limit")
	}
	if commitExists(destinationPath, id) {
		seen[id] = true
		return nil
	}
	var sourcePath, typeName string
	var err error
	for _, candidate := range sourcePaths {
		if typeName, err = gitOutput(candidate, "cat-file", "-t", id); err == nil {
			sourcePath = candidate
			break
		}
	}
	if sourcePath == "" {
		return err
	}
	sizeText, err := gitOutput(sourcePath, "cat-file", "-s", id)
	if err != nil {
		return err
	}
	size, err := strconv.ParseInt(sizeText, 10, 64)
	if err != nil || size < 0 || size > storage.MaxObjectSize || size > *remaining {
		return errors.New("change-stack object exceeds size limit")
	}
	*remaining -= size
	raw, err := exec.Command("git", "--git-dir="+sourcePath, "cat-file", typeName, id).Output()
	if err != nil || int64(len(raw)) != size {
		return errors.New("change-stack object could not be read exactly")
	}
	dependencies := []string{}
	switch typeName {
	case "commit":
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "tree ") || strings.HasPrefix(line, "parent ") {
				fields := strings.Fields(line)
				if len(fields) == 2 {
					dependencies = append(dependencies, fields[1])
				}
			}
		}
	case "tree":
		listing, listErr := exec.Command("git", "--git-dir="+sourcePath, "ls-tree", "-z", id).Output()
		if listErr != nil {
			return listErr
		}
		for _, entry := range bytes.Split(listing, []byte{0}) {
			fields := bytes.Fields(entry)
			if len(fields) >= 3 {
				dependencies = append(dependencies, string(fields[2]))
			}
		}
	}
	for _, dependency := range dependencies {
		if objectErr := importStackObject(sourcePaths, destinationPath, dependency, seen, remaining); objectErr != nil {
			if _, destinationErr := gitOutput(destinationPath, "cat-file", "-e", dependency); destinationErr != nil {
				return objectErr
			}
		}
	}
	command := exec.Command("git", "--git-dir="+destinationPath, "hash-object", "-w", "-t", typeName, "--stdin")
	command.Stdin = bytes.NewReader(raw)
	written, err := command.Output()
	if err != nil || strings.TrimSpace(string(written)) != id {
		return fmt.Errorf("change-stack object identity mismatch")
	}
	seen[id] = true
	return nil
}

func diag(code, message string, blocking bool) changestacks.Diagnostic {
	return changestacks.Diagnostic{Code: code, Message: message, Blocking: blocking}
}
func commitExists(path, rev string) bool {
	_, e := gitOutput(path, "cat-file", "-e", rev+"^{commit}")
	return e == nil
}
func stackScope(path, base, head string) changestacks.Scope {
	s := changestacks.Scope{Files: []string{}}
	if base == "" || head == "" {
		return s
	}
	if n, e := gitOutput(path, "rev-list", "--count", base+".."+head); e == nil {
		s.CommitCount, _ = strconv.Atoi(n)
	}
	out, _ := gitOutput(path, "diff", "--numstat", base, head)
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		a, _ := strconv.Atoi(f[0])
		d, _ := strconv.Atoi(f[1])
		s.Additions += a
		s.Deletions += d
		s.Files = append(s.Files, f[2])
	}
	sort.Strings(s.Files)
	return s
}
func stackAuthors(path, base, head string) []string {
	out, _ := gitOutput(path, "log", "--format=%an <%ae>", base+".."+head)
	set := map[string]bool{}
	for _, x := range strings.Split(out, "\n") {
		if strings.TrimSpace(x) != "" {
			set[x] = true
		}
	}
	xs := make([]string, 0, len(set))
	for x := range set {
		xs = append(xs, x)
	}
	sort.Strings(xs)
	return xs
}
func stackCommits(path, base, head string) []string {
	out, err := gitOutput(path, "rev-list", "--reverse", base+".."+head)
	if err != nil || out == "" {
		return []string{}
	}
	return strings.Split(out, "\n")
}
func stackCycle(id string, g map[string][]string, visiting, done map[string]bool) bool {
	if visiting[id] {
		return true
	}
	if done[id] {
		return false
	}
	visiting[id] = true
	for _, d := range g[id] {
		if stackCycle(d, g, visiting, done) {
			return true
		}
	}
	delete(visiting, id)
	done[id] = true
	return false
}
