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
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerChangeStackRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, stacks *changestacks.Store, pulls *pullrequests.Store) {
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
		if e != nil {
			v.Diagnostics = append(v.Diagnostics, diag("target_unavailable", "target Git history is unavailable", true))
		}
		if e == nil {
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
			rangePath, cleanup, rangeErr := stackRangeView(source.Path(), baseRepositoryPath, base, m.Revision)
			if rangeErr != nil {
				m.Diagnostics = append(m.Diagnostics, diag("base_missing", "declared parent objects could not be assembled", true))
				continue
			}
			if _, x := gitOutput(rangePath, "merge-base", "--is-ancestor", base, m.Revision); x != nil {
				m.Diagnostics = append(m.Diagnostics, diag("unrelated_history", "revision does not descend from its declared parent", true))
			}
			m.IndividualScope = stackScope(rangePath, base, m.Revision)
			m.Authors = stackAuthors(rangePath, base, m.Revision)
			diff, _ := gitOutput(rangePath, "diff", "--binary", base, m.Revision)
			cleanup()
			if targetRepo != nil {
				if cumulativePath, cumulativeCleanup, cumulativeErr := stackRangeView(source.Path(), targetRepo.Path(), v.TargetRevision, m.Revision); cumulativeErr == nil {
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
			clean.Members[i].IndividualScope, clean.Members[i].CumulativeScope = changestacks.Scope{}, changestacks.Scope{}
			clean.Members[i].Permissions, clean.Members[i].ReviewState, clean.Members[i].PublishedAt = changestacks.Permission{}, "", nil
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
}

func stackRangeView(headRepositoryPath, baseRepositoryPath, base, head string) (string, func(), error) {
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
	if err = importStackObject(baseRepositoryPath, dir, base, seen, &remaining); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err = importStackObject(headRepositoryPath, dir, head, seen, &remaining); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return filepath.Clean(dir), cleanup, nil
}

func importStackObject(sourcePath, destinationPath, id string, seen map[string]bool, remaining *int64) error {
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
	typeName, err := gitOutput(sourcePath, "cat-file", "-t", id)
	if err != nil {
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
		if objectErr := importStackObject(sourcePath, destinationPath, dependency, seen, remaining); objectErr != nil {
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
