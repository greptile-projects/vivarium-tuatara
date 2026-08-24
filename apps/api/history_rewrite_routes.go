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
			ExpectedVersion int                                  `json:"expected_version"`
			Candidate       historyremediations.RewriteCandidate `json:"candidate"`
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
		assembled, err := assembleHistoryCandidate(repo.Path(), v, in.Candidate)
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
			ExpectedVersion int                           `json:"expected_version"`
			Rehearsal       historyremediations.Rehearsal `json:"rehearsal"`
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
		run, e := runHistoryRehearsal(repo.Path(), *candidate, in.Rehearsal)
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

func runHistoryRehearsal(gitDir string, c historyremediations.RewriteCandidate, in historyremediations.Rehearsal) (historyremediations.Rehearsal, error) {
	if in.RequestID == "" || len(in.Scenarios) < 7 {
		return in, errors.New("request_id and all seven rehearsal scenario kinds are required")
	}
	tips := map[string]string{}
	for _, r := range c.CandidateRefs {
		tips[r.Name] = r.NewTip
	}
	if len(tips) == 0 {
		return in, errors.New("candidate has no replacement tips")
	}
	first := ""
	for _, r := range c.CandidateRefs {
		first = r.NewTip
		break
	}
	work, err := os.MkdirTemp("", "history-rehearsal-*")
	if err != nil {
		return in, err
	}
	defer os.RemoveAll(work)
	index := filepath.Join(work, "index")
	cmd := exec.Command("git", "--git-dir="+gitDir, "--work-tree="+work, "checkout", "--force", first, "--", ".")
	cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+index)
	if b, e := cmd.CombinedOutput(); e != nil {
		return in, fmt.Errorf("candidate checkout failed: %s", strings.TrimSpace(string(b)))
	}
	started := time.Now()
	seen := map[string]bool{}
	for _, scenario := range in.Scenarios {
		seen[scenario.Kind] = true
		outcome := historyremediations.RehearsalOutcome{ScenarioID: scenario.ID, Kind: scenario.Kind, State: "failed", ExitCode: 1}
		timeout := scenario.TimeoutSeconds
		if timeout < 1 || timeout > 600 {
			return in, errors.New("scenario timeout must be between 1 and 600 seconds")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		var run *exec.Cmd
		switch scenario.Kind {
		case "repository_integrity":
			run = exec.CommandContext(ctx, "git", "--git-dir="+gitDir, "fsck", "--no-dangling", first)
		case "fetch":
			bare := filepath.Join(work, "representative-"+scenario.Kind+".git")
			if e := exec.Command("git", "init", "--bare", bare).Run(); e != nil {
				cancel()
				return in, e
			}
			run = exec.CommandContext(ctx, "git", "--git-dir="+bare, "fetch", "--no-tags", gitDir, first)
		case "clone":
			bare := filepath.Join(work, "representative-clone-source.git")
			if e := exec.Command("git", "init", "--bare", bare).Run(); e != nil {
				cancel()
				return in, e
			}
			if b, e := exec.Command("git", "--git-dir="+bare, "fetch", "--no-tags", gitDir, first).CombinedOutput(); e != nil {
				cancel()
				return in, fmt.Errorf("representative clone preparation failed: %s", strings.TrimSpace(string(b)))
			}
			if e := exec.Command("git", "--git-dir="+bare, "update-ref", "refs/heads/candidate", "FETCH_HEAD").Run(); e != nil {
				cancel()
				return in, e
			}
			run = exec.CommandContext(ctx, "git", "clone", "--no-local", "--branch", "candidate", bare, filepath.Join(work, "representative-clone"))
		default:
			if strings.TrimSpace(scenario.Command) == "" {
				outcome.State = "unsupported"
				outcome.Output = "No revision-appropriate command was supplied."
				in.Outcomes = append(in.Outcomes, outcome)
				cancel()
				continue
			}
			run = exec.CommandContext(ctx, "sh", "-c", scenario.Command)
			run.Dir = work
			run.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
		}
		b, e := run.CombinedOutput()
		cancel()
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
	for _, kind := range []string{"repository_integrity", "build", "check", "release", "dependency", "clone", "fetch"} {
		if !seen[kind] {
			return in, errors.New("all required rehearsal scenario kinds must be present")
		}
	}
	in.ComputeSeconds = int64(time.Since(started).Seconds())
	return in, nil
}
