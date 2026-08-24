package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/restructuringplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type restructuringCandidateInput struct {
	RequestID            string   `json:"request_id"`
	ExpectedVersion      int      `json:"expected_version"`
	CrossRepositoryLinks []string `json:"cross_repository_links"`
}

var restructuringCredentialPattern = regexp.MustCompile(`(?i)(authorization\s*:|bearer\s+[a-z0-9._-]{12,}|-----begin [a-z ]*private key-----|(?:api[_-]?key|password|passwd|secret|token)\s*[:=]\s*[^\s]{8,}|(?:ghp|github_pat|glpat-|xox[baprs]-|sk-)[a-z0-9_-]{12,}|\b(?:AKIA|ASIA)[A-Z0-9]{16}\b|\beyJ[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\b)`)

func registerRestructuringCandidateRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, plans *restructuringplans.Store) {
	mux.HandleFunc("POST /repositories/{id}/restructuring-plans/{plan_id}/candidate-sets", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if c.AgentID != "" {
			writeAPIError(w, 403, "restructuring_candidate_human_required", "a human collaborator must assemble candidate repositories")
			return
		}
		var in restructuringCandidateInput
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.RequestID) == "" {
			writeAPIError(w, 400, "invalid_restructuring_candidate", "request_id and current plan version are required")
			return
		}
		plan, err := plans.Get(r.PathValue("id"), r.PathValue("plan_id"))
		if err != nil {
			writeAPIError(w, 404, "restructuring_plan_not_found", "restructuring plan not found")
			return
		}
		body, _ := json.Marshal(struct {
			Request string
			Version int
			Links   []string
		}{in.RequestID, in.ExpectedVersion, in.CrossRepositoryLinks})
		sum := sha256.Sum256(body)
		digest := hex.EncodeToString(sum[:])
		candidateID := digest[:32]
		sourceIDs := make([]string, 0, len(plan.Sources))
		for _, s := range plan.Sources {
			sourceIDs = append(sourceIDs, s.RepositoryID)
		}
		var out restructuringplans.Plan
		err = catalog.WithCurrentReadAccess(c.UserID, sourceIDs, func() error {
			var createErr error
			out, createErr = plans.CreateCandidateSet(plan.RepositoryID, plan.ID, c.UserID, in.ExpectedVersion, in.RequestID, digest, func(current restructuringplans.Plan) (restructuringplans.CandidateSet, error) {
				return assembleRestructuringCandidate(git, plans, current, candidateID, digest, in)
			})
			return createErr
		})
		if err != nil {
			if errors.Is(err, restructuringplans.ErrConflict) {
				writeAPIError(w, 409, "restructuring_candidate_request_conflict", "request_id was already used for another candidate")
				return
			}
			if errors.Is(err, restructuringplans.ErrVersion) {
				writeAPIError(w, 409, "restructuring_plan_changed", "refresh the plan before assembling candidates")
				return
			}
			writeAPIError(w, 422, "restructuring_candidate_failed", err.Error())
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("POST /repositories/{id}/restructuring-plans/{plan_id}/candidate-sets/{candidate_id}/rehearsals", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                          `json:"expected_version"`
			Rehearsal       restructuringplans.Rehearsal `json:"rehearsal"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_restructuring_rehearsal", "a rehearsal is required")
			return
		}
		plan, err := plans.Get(r.PathValue("id"), r.PathValue("plan_id"))
		if err != nil {
			writeAPIError(w, 404, "restructuring_plan_not_found", "restructuring plan not found")
			return
		}
		var candidate *restructuringplans.CandidateSet
		for i := range plan.CandidateSets {
			if plan.CandidateSets[i].ID == r.PathValue("candidate_id") {
				candidate = &plan.CandidateSets[i]
			}
		}
		if candidate == nil {
			writeAPIError(w, 404, "restructuring_candidate_not_found", "candidate set not found")
			return
		}
		run, err := runRestructuringRehearsal(plans, plan, *candidate, in.Rehearsal)
		if err != nil {
			writeAPIError(w, 422, "restructuring_rehearsal_invalid", err.Error())
			return
		}
		out, err := plans.AddRehearsal(plan.RepositoryID, plan.ID, candidate.ID, c.UserID, in.ExpectedVersion, run)
		if err != nil {
			writeAPIError(w, 409, "restructuring_plan_changed", "refresh before retaining rehearsal evidence")
			return
		}
		writeJSON(w, 201, out)
	})
}

func assembleRestructuringCandidate(git *storage.Store, plans *restructuringplans.Store, plan restructuringplans.Plan, candidateID, digest string, in restructuringCandidateInput) (restructuringplans.CandidateSet, error) {
	out := restructuringplans.CandidateSet{ID: candidateID, RequestID: in.RequestID, RequestDigest: digest, CrossRepositoryLinks: in.CrossRepositoryLinks}
	for _, x := range in.CrossRepositoryLinks {
		if len(strings.TrimSpace(x)) < 1 || len(x) > 500 || restructuringCredentialPattern.MatchString(x) {
			return out, errors.New("cross-repository links must be bounded")
		}
	}
	for _, item := range plan.Inventory {
		if item.State != "resolved" || item.Disposition == "remain" || item.Disposition == "unknown" {
			out.Gaps = append(out.Gaps, restructuringplans.CandidateGap{Kind: item.Kind, ResourceID: item.ResourceID, State: item.State, Summary: item.Summary, RequiredDecision: "the affected owner must decide how this resource is retained, divided, or recreated"})
		}
	}
	if len(plan.Destinations) > 1 && len(in.CrossRepositoryLinks) == 0 {
		out.Gaps = append(out.Gaps, restructuringplans.CandidateGap{Kind: "cross_repository_link", ResourceID: "destination-set", State: "missing", Summary: "multiple destinations have no declared provenance or dependency link", RequiredDecision: "declare how the candidate projects reference and verify one another"})
	}
	historyUses := map[string]string{}
	for _, dst := range plan.Destinations {
		stage, err := os.MkdirTemp("", "restructuring-candidate-*")
		if err != nil {
			return out, err
		}
		defer os.RemoveAll(stage)
		if b, e := exec.Command("git", "init", "-q", stage).CombinedOutput(); e != nil {
			return out, fmt.Errorf("initialize candidate: %s", b)
		}
		parents := []string{}
		mappingIDs := []string{}
		sourceCommits := []string{}
		preservedTags := []string{}
		occupied := map[string]string{}
		licenses := []string{}
		for _, m := range plan.Mappings {
			if m.DestinationID != dst.ID || m.Disposition == "remain" {
				continue
			}
			var sourceRevision string
			for _, s := range plan.Sources {
				if s.RepositoryID == m.SourceRepositoryID {
					sourceRevision = s.Revision
				}
			}
			historyKey := m.SourceRepositoryID + "@" + sourceRevision + ":" + strings.Trim(m.SourcePath, "/ ")
			if prior := historyUses[historyKey]; prior != "" && prior != dst.ID {
				out.Gaps = append(out.Gaps, restructuringplans.CandidateGap{Kind: "duplicated_history", ResourceID: historyKey, State: "shared", Summary: "the same selected history is retained in destinations " + prior + " and " + dst.ID, RequiredDecision: "accept shared ancestry cost or select one authoritative history boundary"})
			} else {
				historyUses[historyKey] = dst.ID
			}
			sr, e := git.Open(m.SourceRepositoryID)
			if e != nil {
				return out, e
			}
			split := sourceRevision
			historyPath := sr.Path()
			if clean := strings.Trim(strings.TrimSpace(m.SourcePath), "/"); clean != "" && clean != "." {
				clone, ce := os.MkdirTemp("", "restructuring-source-*")
				if ce != nil {
					return out, ce
				}
				defer os.RemoveAll(clone)
				if b, ce := exec.Command("git", "clone", "-q", "--no-checkout", sr.Path(), clone).CombinedOutput(); ce != nil {
					return out, fmt.Errorf("clone source: %s", b)
				}
				if b, ce := exec.Command("git", "-C", clone, "checkout", "-q", sourceRevision).CombinedOutput(); ce != nil {
					return out, fmt.Errorf("checkout source: %s", b)
				}
				b, ce := exec.Command("git", "-C", clone, "subtree", "split", "--prefix", clean).CombinedOutput()
				if ce != nil {
					return out, fmt.Errorf("select history for %s: %s", m.ID, b)
				}
				fields := strings.Fields(string(b))
				if len(fields) == 0 {
					return out, fmt.Errorf("select history for %s produced no commit", m.ID)
				}
				split = fields[len(fields)-1]
				historyPath = clone
			}
			if m.HistoryMode != "none" {
				if b, e := exec.Command("git", "-C", stage, "fetch", "-q", historyPath, split).CombinedOutput(); e != nil {
					return out, fmt.Errorf("import history: %s", b)
				}
				parents = append(parents, split)
			}
			mappingIDs = append(mappingIDs, m.ID)
			sourceCommits = append(sourceCommits, sourceRevision)
			paths, _ := exec.Command("git", "--git-dir="+sr.Path(), "ls-tree", "-r", "--name-only", sourceRevision, m.SourcePath).Output()
			for _, p := range strings.Fields(string(paths)) {
				rel := p
				if strings.Trim(m.SourcePath, "/ ") != "" && strings.Trim(m.SourcePath, "/ ") != "." {
					rel = strings.TrimPrefix(p, strings.Trim(m.SourcePath, "/ ")+"/")
				}
				final := filepath.ToSlash(filepath.Join(m.DestinationPath, rel))
				if prior := occupied[final]; prior != "" {
					out.Gaps = append(out.Gaps, restructuringplans.CandidateGap{Kind: "path_collision", ResourceID: final, State: "blocked", Summary: "mappings " + prior + " and " + m.ID + " select the same destination path", RequiredDecision: "choose one source or a distinct destination path"})
				} else {
					occupied[final] = m.ID
				}
				base := strings.ToUpper(filepath.Base(final))
				if strings.HasPrefix(base, "LICENSE") || strings.HasPrefix(base, "COPYING") || strings.HasPrefix(base, "NOTICE") {
					licenses = append(licenses, final)
				}
			}
			dir := filepath.Join(stage, m.DestinationPath)
			if e = os.MkdirAll(dir, 0700); e != nil {
				return out, e
			}
			archive, archiveErr := exec.Command("git", "--git-dir="+sr.Path(), "archive", sourceRevision, m.SourcePath).Output()
			if archiveErr != nil {
				return out, fmt.Errorf("materialize mapping %s: %w", m.ID, archiveErr)
			}
			extract := exec.Command("tar", "-x", "--strip-components", strconv.Itoa(pathComponents(m.SourcePath)), "-C", dir)
			extract.Stdin = strings.NewReader(string(archive))
			if b, extractErr := extract.CombinedOutput(); extractErr != nil {
				return out, fmt.Errorf("materialize mapping %s: %s", m.ID, b)
			}
		}
		if len(mappingIDs) == 0 {
			continue
		}
		_ = exec.Command("git", "-C", stage, "add", "--all").Run()
		treeBody, e := exec.Command("git", "-C", stage, "write-tree").Output()
		if e != nil {
			return out, e
		}
		tree := strings.TrimSpace(string(treeBody))
		args := []string{"commit-tree", tree, "-m", "Immutable restructuring candidate for " + dst.Name}
		for _, p := range parents {
			args = append(args, "-p", p)
		}
		cmd := exec.Command("git", append([]string{"-C", stage}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Vivarium Restructuring", "GIT_AUTHOR_EMAIL=restructuring@vivarium.invalid", "GIT_COMMITTER_NAME=Vivarium Restructuring", "GIT_COMMITTER_EMAIL=restructuring@vivarium.invalid", "GIT_AUTHOR_DATE=2000-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z")
		tipBody, e := cmd.Output()
		if e != nil {
			return out, e
		}
		tip := strings.TrimSpace(string(tipBody))
		_ = exec.Command("git", "-C", stage, "update-ref", "refs/heads/"+dst.DefaultBranch, tip).Run()
		_ = exec.Command("git", "-C", stage, "symbolic-ref", "HEAD", "refs/heads/"+dst.DefaultBranch).Run()
		for _, item := range plan.Inventory {
			selected := false
			for _, destinationID := range item.DestinationIDs {
				selected = selected || destinationID == dst.ID
			}
			if item.Kind != "ref" || item.State != "resolved" || !selected || !strings.HasPrefix(item.ResourceID, "refs/tags/") {
				continue
			}
			source, openErr := git.Open(item.RepositoryID)
			if openErr != nil {
				return out, openErr
			}
			if b, fetchErr := exec.Command("git", "-C", stage, "fetch", "-q", source.Path(), item.ResourceID+":"+item.ResourceID).CombinedOutput(); fetchErr != nil {
				out.Gaps = append(out.Gaps, restructuringplans.CandidateGap{Kind: "tag", ResourceID: item.ResourceID, State: "missing", Summary: "the selected tag object could not be imported: " + strings.TrimSpace(string(b)), RequiredDecision: "repair the tag citation or explicitly recreate the tag"})
				continue
			}
			preservedTags = append(preservedTags, item.ResourceID)
		}
		final := plans.CandidatePath(plan.RepositoryID, plan.ID, candidateID, dst.ID)
		if e = os.MkdirAll(filepath.Dir(final), 0700); e != nil {
			return out, e
		}
		if _, e = os.Stat(final); errors.Is(e, os.ErrNotExist) {
			publishing, tempErr := os.MkdirTemp(filepath.Dir(final), ".candidate-publishing-")
			if tempErr != nil {
				return out, tempErr
			}
			if tempErr = os.Remove(publishing); tempErr != nil {
				return out, tempErr
			}
			if b, cloneErr := exec.Command("git", "clone", "-q", "--bare", "--no-local", stage, publishing).CombinedOutput(); cloneErr != nil {
				_ = os.RemoveAll(publishing)
				return out, fmt.Errorf("publish candidate: %s", b)
			}
			if renameErr := os.Rename(publishing, final); renameErr != nil {
				_ = os.RemoveAll(publishing)
				if _, statErr := os.Stat(final); statErr != nil {
					return out, renameErr
				}
			}
		}
		fsck, e := exec.Command("git", "--git-dir="+final, "fsck", "--full").CombinedOutput()
		if e != nil {
			return out, fmt.Errorf("candidate integrity: %s", fsck)
		}
		publishedTip, tipErr := exec.Command("git", "--git-dir="+final, "rev-parse", "--verify", "refs/heads/"+dst.DefaultBranch).Output()
		if tipErr != nil || strings.TrimSpace(string(publishedTip)) != tip {
			return out, errors.New("published candidate does not match the deterministic assembled tip")
		}
		countBody, _ := exec.Command("git", "--git-dir="+final, "rev-list", "--objects", "--all").Output()
		size := int64(0)
		_ = filepath.WalkDir(final, func(_ string, d fs.DirEntry, e error) error {
			if e == nil && !d.IsDir() {
				if i, x := d.Info(); x == nil {
					size += i.Size()
				}
			}
			return nil
		})
		sort.Strings(licenses)
		prov, _ := json.Marshal(struct {
			Plan              string
			Destination       string
			Mappings, Sources []string
			Tip, Tree         string
		}{plan.ID, dst.ID, mappingIDs, sourceCommits, tip, tree})
		h := sha256.Sum256(prov)
		sort.Strings(preservedTags)
		out.Repositories = append(out.Repositories, restructuringplans.CandidateRepository{ID: dst.ID, DestinationID: dst.ID, DefaultBranch: dst.DefaultBranch, Tip: tip, Tree: tree, ObjectCount: len(strings.Fields(string(countBody))), SizeBytes: size, Mappings: mappingIDs, SourceCommits: sourceCommits, PreservedTags: preservedTags, LicensePaths: licenses, ProvenanceSHA256: hex.EncodeToString(h[:]), SignatureState: "source commit and annotated-tag objects remain verifiable; the mapped synthetic tip is unsigned and rewritten path commits may not retain valid signatures"})
	}
	if len(out.Repositories) == 0 {
		return out, errors.New("declared mappings produced no destination repository")
	}
	return out, nil
}

func pathComponents(path string) int {
	p := strings.Trim(strings.TrimSpace(path), "/")
	if p == "" || p == "." {
		return 0
	}
	return len(strings.Split(p, "/"))
}

var restructuringScenarioKinds = []string{"repository_integrity", "clone", "fetch", "push", "build", "check", "package_resolution", "api_resolution", "documentation", "workspace", "consumer_journey"}

func restructuringRehearsalSetup(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		return output.Bytes(), err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return output.Bytes(), err
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return output.Bytes(), ctx.Err()
	}
}

func runRestructuringRehearsal(plans *restructuringplans.Store, plan restructuringplans.Plan, candidate restructuringplans.CandidateSet, in restructuringplans.Rehearsal) (restructuringplans.Rehearsal, error) {
	requiredCount := len(restructuringScenarioKinds) * len(candidate.Repositories)
	if strings.TrimSpace(in.RequestID) == "" || len(candidate.Repositories) == 0 || len(in.Scenarios) != requiredCount {
		return in, errors.New("request_id and exactly one of all eleven scenario kinds for every destination are required")
	}
	in.ID = ""
	in.Outcomes = nil
	in.State = "passed"
	in.CostUnits = 0
	in.RequiredDecisions = nil
	seen := map[string]bool{}
	required := map[string]bool{}
	repos := map[string]restructuringplans.CandidateRepository{}
	for _, x := range candidate.Repositories {
		repos[x.DestinationID] = x
		for _, kind := range restructuringScenarioKinds {
			required[x.DestinationID+"\x00"+kind] = true
		}
	}
	totalTimeout := 0
	for _, s := range in.Scenarios {
		key := s.DestinationID + "\x00" + s.Kind
		if !required[key] || seen[key] {
			return in, errors.New("scenario kinds must be unique and complete for every candidate destination")
		}
		seen[key] = true
		totalTimeout += s.TimeoutSeconds
		if repos[s.DestinationID].ID == "" || s.TimeoutSeconds < 1 || s.TimeoutSeconds > 300 {
			return in, errors.New("scenarios require a candidate destination and a timeout from 1 to 300 seconds")
		}
		encoded, _ := json.Marshal(s)
		if restructuringCredentialPattern.Match(encoded) {
			return in, errors.New("scenario definitions cannot retain credential-shaped content")
		}
		if s.Command != "" && !validRehearsalImage(s.Image) {
			return in, errors.New("commands require a bounded preinstalled image")
		}
	}
	if totalTimeout > 900 {
		return in, errors.New("aggregate rehearsal timeout cannot exceed 900 seconds")
	}
	for key := range required {
		if !seen[key] {
			return in, errors.New("every destination requires all eleven scenario kinds")
		}
	}
	overall, stopOverall := context.WithTimeout(context.Background(), 15*time.Minute)
	defer stopOverall()
	for i, s := range in.Scenarios {
		started := time.Now()
		o := restructuringplans.Outcome{ScenarioID: s.ID, Kind: s.Kind, DestinationID: s.DestinationID, State: "failed", ExitCode: 1}
		repoPath := plans.CandidatePath(plan.RepositoryID, plan.ID, candidate.ID, s.DestinationID)
		if overall.Err() != nil {
			return in, errors.New("aggregate rehearsal execution deadline exceeded")
		}
		ctx, cancel := context.WithTimeout(overall, time.Duration(s.TimeoutSeconds)*time.Second)
		tmp, tempErr := os.MkdirTemp("", "restructuring-rehearsal-*")
		if tempErr != nil {
			cancel()
			return in, tempErr
		}
		var cmd *exec.Cmd
		container := ""
		var setupOutput []byte
		var setupErr error
		setup := func(args ...string) {
			if setupErr != nil {
				return
			}
			setupOutput, setupErr = restructuringRehearsalSetup(ctx, args...)
		}
		switch s.Kind {
		case "repository_integrity":
			cmd = exec.CommandContext(ctx, "git", "--git-dir="+repoPath, "fsck", "--full")
		case "clone":
			cmd = exec.CommandContext(ctx, "git", "clone", "--no-local", repoPath, filepath.Join(tmp, "clone"))
		case "fetch":
			setup("git", "init", "--bare", filepath.Join(tmp, "fetch.git"))
			cmd = exec.CommandContext(ctx, "git", "--git-dir="+filepath.Join(tmp, "fetch.git"), "fetch", repoPath, "+refs/heads/*:refs/heads/*")
		case "push":
			sandbox := filepath.Join(tmp, "push.git")
			setup("git", "clone", "-q", "--bare", "--no-local", repoPath, sandbox)
			work := filepath.Join(tmp, "push-work")
			setup("git", "clone", "-q", "--no-local", sandbox, work)
			cmd = exec.CommandContext(ctx, "git", "-C", work, "push", "origin", "HEAD:refs/heads/rehearsal-push")
		default:
			if strings.TrimSpace(s.Command) == "" {
				o.State = "unsupported"
				o.Output = "No revision-appropriate command and preinstalled image were supplied."
				in.RequiredDecisions = append(in.RequiredDecisions, s.Kind+" for "+s.DestinationID+" needs an owner-approved command")
				in.State = "failed"
				in.Outcomes = append(in.Outcomes, o)
				cancel()
				_ = os.RemoveAll(tmp)
				continue
			}
			work := filepath.Join(tmp, "work")
			setup("git", "clone", "-q", "--no-local", repoPath, work)
			container = fmt.Sprintf("vivarium-restructuring-%d-%d", os.Getpid(), i)
			cmd = exec.CommandContext(ctx, "docker", "run", "--name", container, "--rm", "--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=128", "--memory=512m", "--cpus=1", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--mount", "type=bind,src="+work+",dst=/workspace,readonly", "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=64m,mode=1777", "--workdir", "/workspace", "--env", "HOME=/tmp", s.Image, "sh", "-c", s.Command)
		}
		b, e := setupOutput, setupErr
		if e == nil {
			b, e = cmd.CombinedOutput()
		}
		cancel()
		if container != "" {
			cleanupCtx, stopCleanup := context.WithTimeout(context.Background(), 5*time.Second)
			_ = exec.CommandContext(cleanupCtx, "docker", "rm", "-f", container).Run()
			stopCleanup()
		}
		if len(b) > 4000 {
			b = b[:4000]
		}
		o.Output = strings.TrimSpace(string(b))
		if restructuringCredentialPattern.MatchString(o.Output) {
			o.Output = "output omitted because it contained credential-shaped content"
			e = errors.New("unsafe rehearsal output")
		}
		if e == nil {
			o.State = "passed"
			o.ExitCode = 0
		} else if x, ok := e.(*exec.ExitError); ok {
			o.ExitCode = x.ExitCode()
		}
		o.DurationMS = time.Since(started).Milliseconds()
		in.CostUnits += float64(o.DurationMS) / 1000
		in.Outcomes = append(in.Outcomes, o)
		if o.State != "passed" {
			in.State = "failed"
			in.RequiredDecisions = append(in.RequiredDecisions, s.Kind+" failed for "+s.DestinationID+": "+s.Expectation)
		}
		_ = os.RemoveAll(tmp)
	}
	for _, g := range candidate.Gaps {
		in.RequiredDecisions = append(in.RequiredDecisions, g.RequiredDecision+" ("+g.Kind+": "+g.ResourceID+")")
	}
	sort.Strings(in.RequiredDecisions)
	return in, nil
}
