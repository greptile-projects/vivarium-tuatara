package main

import (
	"bytes"
	"context"
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
	"github.com/greptile-projects/vivarium-tuatara/apps/api/historyremediations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type historyRewriteCandidateInput struct {
	RequestID           string                            `json:"request_id"`
	Rules               []historyremediations.RewriteRule `json:"rules"`
	SelectedRefs        []historyremediations.RewriteRef  `json:"selected_refs"`
	RollbackLimit       string                            `json:"rollback_limit"`
	CollaboratorActions []string                          `json:"collaborator_actions"`
}
type historyRewriteRehearsalInput struct {
	RequestID string                                  `json:"request_id"`
	Scenarios []historyremediations.RehearsalScenario `json:"scenarios"`
}

func registerHistoryRewriteRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *historyremediations.Store) {
	mux.HandleFunc("POST /repositories/{id}/history-remediations/{remediation_id}/rewrite-candidates", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		v, err := store.Get(r.PathValue("id"), r.PathValue("remediation_id"))
		if err != nil || !historyRemediationCanRespond(v, c.UserID) {
			writeAPIError(w, 404, "history_remediation_not_found", "history remediation not found")
			return
		}
		var in struct {
			ExpectedVersion int                          `json:"expected_version"`
			Candidate       historyRewriteCandidateInput `json:"candidate"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an immutable rewrite candidate is required")
			return
		}
		repo, err := git.Open(v.RepositoryID)
		if err != nil {
			writeAPIError(w, 422, "history_rewrite_repository_unavailable", "repository objects could not be resolved")
			return
		}
		candidateInput := historyremediations.RewriteCandidate{RequestID: in.Candidate.RequestID, Rules: in.Candidate.Rules, SelectedRefs: in.Candidate.SelectedRefs, RollbackLimit: in.Candidate.RollbackLimit, CollaboratorActions: in.Candidate.CollaboratorActions}
		assembled, err := assembleHistoryCandidate(repo.Path(), v, candidateInput)
		if err != nil {
			writeAPIError(w, 422, "history_rewrite_candidate_invalid", err.Error())
			return
		}
		out, err := store.AddRewriteCandidate(v.RepositoryID, v.ID, in.ExpectedVersion, assembled, c.UserID)
		switch {
		case errors.Is(err, historyremediations.ErrVersionConflict):
			writeAPIError(w, 409, "history_remediation_version_conflict", "the remediation changed; reload before assembling")
		case errors.Is(err, historyremediations.ErrConflict):
			writeAPIError(w, 409, "history_rewrite_request_conflict", "request_id was reused for a different candidate")
		case err != nil:
			writeAPIError(w, 422, "history_rewrite_candidate_invalid", "the candidate could not be retained")
		default:
			writeJSON(w, 201, historyRemediationPublic(out))
		}
	})
	mux.HandleFunc("POST /repositories/{id}/history-remediations/{remediation_id}/rewrite-candidates/{candidate_id}/rehearsals", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		v, err := store.Get(r.PathValue("id"), r.PathValue("remediation_id"))
		if err != nil || !historyRemediationCanRespond(v, c.UserID) {
			writeAPIError(w, 404, "history_remediation_not_found", "history remediation not found")
			return
		}
		var in struct {
			ExpectedVersion int                          `json:"expected_version"`
			Rehearsal       historyRewriteRehearsalInput `json:"rehearsal"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded rehearsal is required")
			return
		}
		var candidate *historyremediations.RewriteCandidate
		for i := range v.RewriteCandidates {
			if v.RewriteCandidates[i].ID == r.PathValue("candidate_id") {
				candidate = &v.RewriteCandidates[i]
			}
		}
		if candidate == nil {
			writeAPIError(w, 404, "history_rewrite_candidate_not_found", "rewrite candidate not found")
			return
		}
		repo, e := git.Open(v.RepositoryID)
		if e != nil {
			writeAPIError(w, 422, "history_rewrite_repository_unavailable", "repository objects could not be resolved")
			return
		}
		rehearsalInput := historyremediations.Rehearsal{RequestID: in.Rehearsal.RequestID, Scenarios: in.Rehearsal.Scenarios}
		run, e := runHistoryRehearsal(repo.Path(), *candidate, rehearsalInput)
		if e != nil {
			writeAPIError(w, 422, "history_rewrite_rehearsal_invalid", e.Error())
			return
		}
		out, e := store.AddRehearsal(v.RepositoryID, v.ID, candidate.ID, in.ExpectedVersion, run, c.UserID)
		if errors.Is(e, historyremediations.ErrVersionConflict) {
			writeAPIError(w, 409, "history_remediation_version_conflict", "the remediation changed; reload before rehearsing")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "history_rewrite_rehearsal_invalid", "the rehearsal could not be retained")
			return
		}
		writeJSON(w, 201, historyRemediationPublic(out))
	})
	mux.HandleFunc("POST /repositories/{id}/history-remediations/{remediation_id}/rewrite-candidates/{candidate_id}/publish", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		v, err := store.Get(r.PathValue("id"), r.PathValue("remediation_id"))
		if err != nil || !historyRemediationCanPublish(v, c) {
			writeAPIError(w, 404, "history_remediation_not_found", "history remediation not found")
			return
		}
		var in struct {
			ExpectedVersion int                             `json:"expected_version"`
			Publication     historyremediations.Publication `json:"publication"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an approved publication is required")
			return
		}
		in.Publication.CandidateID = r.PathValue("candidate_id")
		var candidate *historyremediations.RewriteCandidate
		for i := range v.RewriteCandidates {
			if v.RewriteCandidates[i].ID == in.Publication.CandidateID {
				candidate = &v.RewriteCandidates[i]
			}
		}
		if candidate == nil {
			writeAPIError(w, 404, "history_rewrite_candidate_not_found", "rewrite candidate not found")
			return
		}
		passed := false
		for _, rehearsal := range candidate.Rehearsals {
			if rehearsal.State == "passed" {
				passed = true
			}
		}
		if !passed {
			writeAPIError(w, 409, "history_rewrite_rehearsal_required", "a complete passing rehearsal is required before publication")
			return
		}
		if !historyPublicationReady(v, in.ExpectedVersion, in.Publication) {
			writeAPIError(w, 422, "history_rewrite_publication_invalid", "required role approvals, pauses, and owner migration instructions are required")
			return
		}
		_, err = store.ReservePublication(v.RepositoryID, v.ID, in.ExpectedVersion, in.Publication, c.UserID)
		switch {
		case errors.Is(err, historyremediations.ErrVersionConflict):
			writeAPIError(w, 409, "history_remediation_version_conflict", "the remediation changed; reload before publishing")
			return
		case errors.Is(err, historyremediations.ErrConflict):
			writeAPIError(w, 409, "history_rewrite_already_published", "another candidate is already authoritative")
			return
		case errors.Is(err, historyremediations.ErrInvalid):
			writeAPIError(w, 422, "history_rewrite_publication_invalid", "required role approvals, enforced push pause, and owner migration instructions are required")
			return
		case err != nil:
			writeAPIError(w, 500, "history_rewrite_publication_unavailable", "publication containment could not be reserved")
			return
		}
		repo, openErr := git.Open(v.RepositoryID)
		if openErr != nil {
			writeAPIError(w, 422, "history_rewrite_repository_unavailable", "repository refs could not be resolved")
			return
		}
		if err = publishHistoryRefs(repo.Path(), candidate.CandidateRefs); err != nil {
			writeAPIError(w, 409, "history_rewrite_refs_changed", err.Error())
			return
		}
		out, err := store.CompletePublication(v.RepositoryID, v.ID, in.Publication.RequestID, candidate.ID)
		switch {
		case errors.Is(err, historyremediations.ErrConflict):
			writeAPIError(w, 409, "history_rewrite_already_published", "another candidate is already authoritative")
		case err != nil:
			writeAPIError(w, 500, "history_rewrite_publication_unavailable", "the durable publication intent remains active; retry the exact request to reconcile finalization")
		default:
			writeJSON(w, 201, historyRemediationPublic(out))
		}
	})
	mux.HandleFunc("POST /repositories/{id}/history-remediations/{remediation_id}/publication-approvals", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		v, err := store.Get(r.PathValue("id"), r.PathValue("remediation_id"))
		if err != nil || !historyRemediationCanSee(v, c.UserID) || c.AgentID != "" {
			writeAPIError(w, 404, "history_remediation_not_found", "history remediation not found")
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Role            string `json:"role"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an approval role is required")
			return
		}
		out, err := store.ApprovePublication(v.RepositoryID, v.ID, in.ExpectedVersion, in.Role, c.UserID)
		if errors.Is(err, historyremediations.ErrInvalid) {
			writeAPIError(w, 403, "history_rewrite_approval_forbidden", "only a named approver may attest their own required role")
			return
		}
		if errors.Is(err, historyremediations.ErrVersionConflict) {
			writeAPIError(w, 409, "history_remediation_version_conflict", "the remediation changed; reload before approving")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "history_rewrite_approval_unavailable", "approval could not be retained")
			return
		}
		writeJSON(w, 201, historyRemediationPublic(out))
	})
}

func historyPublicationReady(v historyremediations.Remediation, expected int, in historyremediations.Publication) bool {
	if v.Publication != nil {
		return v.Publication.RequestID == in.RequestID && v.Publication.CandidateID == in.CandidateID
	}
	if v.Version != expected || in.RequestID == "" || len(in.PausedSystems) != 1 || in.PausedSystems[0] != "pushes" || len(in.MigrationTargets) == 0 {
		return false
	}
	roles := map[string]map[string]bool{}
	for _, approval := range v.PublicationApprovals {
		if roles[approval.Role] == nil {
		}
		roles[approval.Role][approval.ApproverID] = true
	}
	for _, required := range v.RequiredApprovals {
		count := 0
		for _, id := range required.ApproverIDs {
			if roles[required.Role][id] {
				count++
			}
		}
		if count < required.Required {
			return false
		}
	}
	for _, target := range in.MigrationTargets {
		if target.ID == "" || target.ResourceID == "" || target.OwnerID == "" || target.Instructions == "" || target.ReplacementRef == "" || !map[string]bool{"local_branch": true, "fork": true, "federated_copy": true, "pull_request": true, "workspace": true, "integration": true}[target.Kind] {
			return false
		}
	}
	return true
}

func historyRemediationOwner(v historyremediations.Remediation, actor string) bool {
	for _, id := range v.OwnerIDs {
		if id == actor {
			return true
		}
	}
	return false
}

func historyRemediationCanPublish(v historyremediations.Remediation, credential auth.Credential) bool {
	return credential.AgentID == "" && historyRemediationOwner(v, credential.UserID)
}

func publishHistoryRefs(gitDir string, refs []historyremediations.CandidateRef) error {
	if len(refs) == 0 {
		return errors.New("candidate has no replacement refs")
	}
	var transaction strings.Builder
	transaction.WriteString("start\n")
	for _, ref := range refs {
		current, err := historyGitOutput(gitDir, "rev-parse", "--verify", ref.Name)
		if err != nil {
			return fmt.Errorf("%s no longer resolves", ref.Name)
		}
		if current == ref.NewTip {
			continue
		} // exact retry after a committed transaction
		if current != ref.OldTip {
			return fmt.Errorf("%s moved; assemble and rehearse a new candidate", ref.Name)
		}
		fmt.Fprintf(&transaction, "update %s %s %s\n", ref.Name, ref.NewTip, ref.OldTip)
	}
	transaction.WriteString("prepare\ncommit\n")
	cmd := exec.Command("git", "--git-dir="+gitDir, "update-ref", "--stdin")
	cmd.Stdin = strings.NewReader(transaction.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("replacement refs were not published: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func historyRemediationCanRespond(v historyremediations.Remediation, actor string) bool {
	if actor == "" {
		return false
	}
	if v.CreatedBy == actor {
		return true
	}
	for _, id := range v.OwnerIDs {
		if id == actor {
			return true
		}
	}
	return false
}

type historyRewriter struct {
	gitDir         string
	rules          map[string]historyremediations.RewriteRule
	trees, commits map[string]string
	applied        map[string]bool
	mappings       []historyremediations.CommitMapping
	broken         []string
}

func historyGitOutput(gitDir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"--git-dir=" + gitDir}, args...)...)
	b, e := cmd.Output()
	return strings.TrimSpace(string(b)), e
}
func (x *historyRewriter) tree(id string) (string, error) {
	if v, ok := x.trees[id]; ok {
		return v, nil
	}
	raw, err := exec.Command("git", "--git-dir="+x.gitDir, "ls-tree", "-z", id).Output()
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	changed := false
	for _, record := range bytes.Split(raw, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		parts := bytes.SplitN(record, []byte{'\t'}, 2)
		meta := strings.Fields(string(parts[0]))
		if len(meta) != 3 || len(parts) != 2 {
			return "", errors.New("candidate tree is malformed")
		}
		oid := meta[2]
		next := oid
		if rule, ok := x.rules[oid]; ok {
			x.applied[oid] = true
			changed = true
			if rule.Action == "remove" {
				continue
			}
			next = rule.ReplacementObjectID
		} else if meta[1] == "tree" {
			next, err = x.tree(oid)
			if err != nil {
				return "", err
			}
			changed = changed || next != oid
		}
		fmt.Fprintf(&out, "%s %s %s\t%s%c", meta[0], meta[1], next, parts[1], byte(0))
	}
	if !changed {
		x.trees[id] = id
		return id, nil
	}
	cmd := exec.Command("git", "--git-dir="+x.gitDir, "mktree", "-z")
	cmd.Stdin = &out
	b, err := cmd.Output()
	if err != nil {
		return "", err
	}
	next := strings.TrimSpace(string(b))
	x.trees[id] = next
	return next, nil
}
func stripSignature(lines []string) ([]string, bool) {
	out := []string{}
	removed := false
	skip := false
	for _, line := range lines {
		if strings.HasPrefix(line, "gpgsig ") {
			removed = true
			skip = true
			continue
		}
		if skip && strings.HasPrefix(line, " ") {
			continue
		}
		skip = false
		out = append(out, line)
	}
	return out, removed
}
func (x *historyRewriter) commit(id string) (string, error) {
	if v, ok := x.commits[id]; ok {
		return v, nil
	}
	raw, err := exec.Command("git", "--git-dir="+x.gitDir, "cat-file", "commit", id).Output()
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(string(raw), "\n\n", 2)
	headers := strings.Split(parts[0], "\n")
	changed := false
	for i, line := range headers {
		if strings.HasPrefix(line, "tree ") {
			old := strings.TrimPrefix(line, "tree ")
			next, e := x.tree(old)
			if e != nil {
				return "", e
			}
			headers[i] = "tree " + next
			changed = changed || next != old
		} else if strings.HasPrefix(line, "parent ") {
			old := strings.TrimPrefix(line, "parent ")
			next, e := x.commit(old)
			if e != nil {
				return "", e
			}
			headers[i] = "parent " + next
			changed = changed || next != old
		}
	}
	if !changed {
		x.commits[id] = id
		x.mappings = append(x.mappings, historyremediations.CommitMapping{OldCommitID: id, NewCommitID: id})
		return id, nil
	}
	headers, signed := stripSignature(headers)
	if signed {
		x.broken = append(x.broken, id)
	}
	body := strings.Join(headers, "\n") + "\n\n"
	if len(parts) > 1 {
		body += parts[1]
	}
	cmd := exec.Command("git", "--git-dir="+x.gitDir, "hash-object", "-t", "commit", "-w", "--stdin")
	cmd.Stdin = strings.NewReader(body)
	b, e := cmd.Output()
	if e != nil {
		return "", e
	}
	next := strings.TrimSpace(string(b))
	x.commits[id] = next
	x.mappings = append(x.mappings, historyremediations.CommitMapping{OldCommitID: id, NewCommitID: next, Changed: true})
	return next, nil
}
func assembleHistoryCandidate(gitDir string, v historyremediations.Remediation, in historyremediations.RewriteCandidate) (historyremediations.RewriteCandidate, error) {
	if in.RequestID == "" || len(in.Rules) == 0 || len(in.SelectedRefs) == 0 {
		return in, errors.New("request_id, rules, and selected refs are required")
	}
	// All audit projections are server-derived. Never append to request values.
	in.ID = ""
	in.CandidateRefs = nil
	in.CommitMap = nil
	in.ObjectMap = nil
	in.BrokenSignatures = nil
	in.BrokenLinks = nil
	in.Unrewritable = nil
	in.OriginalBytes = 0
	in.CandidateBytes = 0
	in.CreatedBy = ""
	in.CreatedAt = time.Time{}
	in.Rehearsals = nil
	allowed := map[string]bool{}
	for _, s := range v.Scopes {
		if s.Kind == "git_object" {
			allowed[s.ObjectID] = true
		}
	}
	rw := &historyRewriter{gitDir: gitDir, rules: map[string]historyremediations.RewriteRule{}, trees: map[string]string{}, commits: map[string]string{}, applied: map[string]bool{}}
	for _, rule := range in.Rules {
		if !allowed[rule.AffectedObjectID] {
			return in, errors.New("every rule must name a scoped Git object")
		}
		kind, e := historyGitOutput(gitDir, "cat-file", "-t", rule.AffectedObjectID)
		if e != nil || kind != "blob" {
			return in, errors.New("rewrite rules currently require affected blob objects")
		}
		if rule.Action == "replace" {
			replacementKind, e := historyGitOutput(gitDir, "cat-file", "-t", rule.ReplacementObjectID)
			if e != nil || replacementKind != "blob" {
				return in, errors.New("replacement objects must be existing blobs")
			}
		}
		rw.rules[rule.AffectedObjectID] = rule
		in.ObjectMap = append(in.ObjectMap, historyremediations.ObjectMapping{OldObjectID: rule.AffectedObjectID, NewObjectID: rule.ReplacementObjectID, Action: rule.Action})
	}
	for _, ref := range in.SelectedRefs {
		tip, e := historyGitOutput(gitDir, "rev-parse", "--verify", ref.Name+"^{commit}")
		if e != nil || tip != ref.ExpectedTip {
			return in, errors.New("selected refs must resolve to their exact expected commit")
		}
		next, e := rw.commit(tip)
		if e != nil {
			return in, errors.New("candidate history could not be assembled")
		}
		in.CandidateRefs = append(in.CandidateRefs, historyremediations.CandidateRef{Name: ref.Name, OldTip: tip, NewTip: next})
	}
	for objectID := range rw.rules {
		if !rw.applied[objectID] {
			return in, errors.New("every rewrite rule must affect the selected ref histories")
		}
	}
	sort.Slice(rw.mappings, func(i, j int) bool { return rw.mappings[i].OldCommitID < rw.mappings[j].OldCommitID })
	in.CommitMap = rw.mappings
	in.BrokenSignatures = rw.broken
	for _, m := range rw.mappings {
		if m.Changed {
			in.BrokenLinks = append(in.BrokenLinks, m.OldCommitID)
		}
	}
	for _, f := range v.ExposureMap {
		if f.IndependentlyControlled || f.State == "unreachable" || f.State == "unverifiable" {
			resourceID := f.ResourceID
			if f.Restricted {
				resourceID = "restricted-copy"
			}
			in.Unrewritable = append(in.Unrewritable, f.CopyKind+":"+resourceID)
		}
	}
	for _, scope := range v.Scopes {
		if scope.Kind != "git_object" {
			in.Unrewritable = append(in.Unrewritable, scope.Kind+":"+scope.ObjectID)
		}
	}
	in.OriginalBytes = objectBytes(gitDir, in.ObjectMap, false)
	in.CandidateBytes = objectBytes(gitDir, in.ObjectMap, true)
	if in.RollbackLimit == "" {
		in.RollbackLimit = "Rollback remains possible only while old objects and independently controlled copies remain available; candidate publication is a separate approved operation."
	}
	if len(in.CollaboratorActions) == 0 {
		in.CollaboratorActions = []string{"fetch replacement refs after publication", "rebase or recreate work rooted in changed commits", "verify local clones no longer advertise affected objects"}
	}
	return in, nil
}
func objectBytes(gitDir string, m []historyremediations.ObjectMapping, replacement bool) int64 {
	var n int64
	for _, x := range m {
		id := x.OldObjectID
		if replacement {
			id = x.NewObjectID
		}
		if id == "" {
			continue
		}
		s, e := historyGitOutput(gitDir, "cat-file", "-s", id)
		if e == nil {
			v, _ := strconv.ParseInt(s, 10, 64)
			n += v
		}
	}
	return n
}

func validRehearsalImage(image string) bool {
	if image == "" || len(image) > 200 || strings.ContainsAny(image, " \t\r\n@") {
		return false
	}
	for _, r := range image {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("./:_-", r)) {
			return false
		}
	}
	return true
}

func candidateCheckout(gitDir, tip, work string) error {
	index, err := os.CreateTemp("", "history-rehearsal-index-*")
	if err != nil {
		return err
	}
	indexPath := index.Name()
	_ = index.Close()
	_ = os.Remove(indexPath)
	defer os.Remove(indexPath)
	cmd := exec.Command("git", "--git-dir="+gitDir, "--work-tree="+work, "checkout", "--force", tip, "--", ".")
	cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+indexPath)
	if b, e := cmd.CombinedOutput(); e != nil {
		return fmt.Errorf("candidate checkout failed: %s", strings.TrimSpace(string(b)))
	}
	return nil
}

func boundedRehearsalCommand(ctx context.Context, work, containerName string, scenario historyremediations.RehearsalScenario) *exec.Cmd {
	return exec.CommandContext(ctx, "docker", "run", "--name", containerName, "--rm", "--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=128", "--memory=512m", "--cpus=1", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--mount", "type=bind,src="+work+",dst=/workspace,readonly", "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=64m,mode=1777", "--workdir", "/workspace", "--env", "HOME=/tmp", "--env", "CI=true", scenario.Image, "sh", "-c", scenario.Command)
}

func runHistoryRehearsal(gitDir string, c historyremediations.RewriteCandidate, in historyremediations.Rehearsal) (historyremediations.Rehearsal, error) {
	if in.RequestID == "" || len(in.Scenarios) < 7 || len(c.CandidateRefs) == 0 {
		return in, errors.New("request_id, candidate refs, and all seven rehearsal scenario kinds are required")
	}
	// Results and attribution are executor/store derived, never accepted from callers.
	in.ID = ""
	in.CandidateID = ""
	in.Outcomes = nil
	in.State = ""
	in.CreatedBy = ""
	in.CreatedAt = time.Time{}
	in.ComputeSeconds = 0
	seen := map[string]bool{}
	for _, scenario := range in.Scenarios {
		seen[scenario.Kind] = true
		if scenario.TimeoutSeconds < 1 || scenario.TimeoutSeconds > 600 {
			return in, errors.New("scenario timeout must be between 1 and 600 seconds")
		}
		if scenario.Command != "" && !validRehearsalImage(scenario.Image) {
			return in, errors.New("command scenarios require a bounded preinstalled container image")
		}
	}
	for _, kind := range []string{"repository_integrity", "build", "check", "release", "dependency", "clone", "fetch"} {
		if !seen[kind] {
			return in, errors.New("all required rehearsal scenario kinds must be present")
		}
	}
	started := time.Now()
	for refIndex, candidateRef := range c.CandidateRefs {
		work, err := os.MkdirTemp("", "history-rehearsal-*")
		if err != nil {
			return in, err
		}
		if err = candidateCheckout(gitDir, candidateRef.NewTip, work); err != nil {
			_ = os.RemoveAll(work)
			return in, err
		}
		for scenarioIndex, scenario := range in.Scenarios {
			outcome := historyremediations.RehearsalOutcome{ScenarioID: scenario.ID, Kind: scenario.Kind, RefName: candidateRef.Name, State: "failed", ExitCode: 1}
			if strings.TrimSpace(scenario.Command) == "" && !map[string]bool{"repository_integrity": true, "clone": true, "fetch": true}[scenario.Kind] {
				outcome.State = "unsupported"
				outcome.Output = "No revision-appropriate command and image were supplied."
				in.Outcomes = append(in.Outcomes, outcome)
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(scenario.TimeoutSeconds)*time.Second)
			var run *exec.Cmd
			containerName := ""
			switch scenario.Kind {
			case "repository_integrity":
				run = exec.CommandContext(ctx, "git", "--git-dir="+gitDir, "fsck", "--no-dangling", candidateRef.NewTip)
			case "fetch":
				bare := filepath.Join(work, fmt.Sprintf("representative-fetch-%d-%d.git", refIndex, scenarioIndex))
				if e := exec.Command("git", "init", "--bare", bare).Run(); e != nil {
					cancel()
					_ = os.RemoveAll(work)
					return in, e
				}
				run = exec.CommandContext(ctx, "git", "--git-dir="+bare, "fetch", "--no-tags", gitDir, candidateRef.NewTip)
			case "clone":
				bare := filepath.Join(work, fmt.Sprintf("representative-clone-source-%d-%d.git", refIndex, scenarioIndex))
				if e := exec.Command("git", "init", "--bare", bare).Run(); e != nil {
					cancel()
					_ = os.RemoveAll(work)
					return in, e
				}
				if b, e := exec.Command("git", "--git-dir="+bare, "fetch", "--no-tags", gitDir, candidateRef.NewTip).CombinedOutput(); e != nil {
					cancel()
					_ = os.RemoveAll(work)
					return in, fmt.Errorf("representative clone preparation failed: %s", strings.TrimSpace(string(b)))
				}
				if e := exec.Command("git", "--git-dir="+bare, "update-ref", "refs/heads/candidate", "FETCH_HEAD").Run(); e != nil {
					cancel()
					_ = os.RemoveAll(work)
					return in, e
				}
				run = exec.CommandContext(ctx, "git", "clone", "--no-local", "--branch", "candidate", bare, filepath.Join(work, fmt.Sprintf("representative-clone-%d-%d", refIndex, scenarioIndex)))
			default:
				containerName = fmt.Sprintf("vivarium-history-rehearsal-%d-%d-%d-%d", os.Getpid(), refIndex, scenarioIndex, time.Now().UnixNano())
				run = boundedRehearsalCommand(ctx, work, containerName, scenario)
			}
			b, e := run.CombinedOutput()
			cancel()
			if containerName != "" {
				_ = exec.Command("docker", "rm", "-f", containerName).Run()
			}
			if len(b) > 2000 {
				b = b[:2000]
			}
			outcome.Output = strings.TrimSpace(string(b))
			if e == nil {
				outcome.State = "passed"
				outcome.ExitCode = 0
			} else if exit, ok := e.(*exec.ExitError); ok {
				outcome.ExitCode = exit.ExitCode()
			}
			in.Outcomes = append(in.Outcomes, outcome)
		}
		_ = os.RemoveAll(work)
	}
	in.ComputeSeconds = int64(time.Since(started).Seconds())
	return in, nil
}
