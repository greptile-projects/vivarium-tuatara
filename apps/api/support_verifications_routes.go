package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/knowledgeanswers"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportverifications"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

var reusableSecret = regexp.MustCompile(`(?i)(authorization\s*:|bearer\s+[a-z0-9._-]{12,}|-----begin [a-z ]*private key-----|(?:api[_-]?key|password|passwd|secret|token)\s*[:=]\s*[^\s]{8,}|(?:ghp|github_pat|sk)-[a-z0-9_-]{12,})`)

func registerSupportVerificationRoutes(mux *http.ServeMux, repos *repositories.Store, threads *supportthreads.Store, answers *knowledgeanswers.Store, workspaceStore *workspaces.Store, store *supportverifications.Store, credentials *auth.Store) {
	access := func(w http.ResponseWriter, r *http.Request) (auth.Credential, repositories.Repository, supportthreads.Thread, bool) {
		a, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return a, repositories.Repository{}, supportthreads.Thread{}, false
		}
		repo, e := repos.GetByID(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return a, repo, supportthreads.Thread{}, false
		}
		member := repo.OwnerID == a.UserID
		if !member {
			member, _ = repos.HasCollaborator(a.UserID, repo.ID)
		}
		if !member {
			writeAPIError(w, 404, "support_thread_not_found", "support thread not found")
			return a, repo, supportthreads.Thread{}, false
		}
		thread, e := threads.Get(repo.ID, r.PathValue("thread_id"))
		if e != nil {
			writeAPIError(w, 404, "support_thread_not_found", "support thread not found")
			return a, repo, thread, false
		}
		return a, repo, thread, true
	}
	project := func(v supportverifications.Attempt, thread supportthreads.Thread) supportverifications.Attempt {
		reasons := []string{}
		answer, e := answers.Get(v.RepositoryID, v.AnswerID)
		if e != nil {
			reasons = append(reasons, "answer is no longer readable")
		} else if answer.CurrentRevisionID != v.AnswerRevisionID {
			reasons = append(reasons, "answer instructions changed")
		}
		if thread.Target.Version != v.SoftwareVersion {
			reasons = append(reasons, "stated software version changed")
		}
		if environmentHash(thread.Environment) != environmentVerificationHash(v.Environment) {
			reasons = append(reasons, "declared environment or dependencies changed")
		}
		if ws, e := workspaceStore.Get(v.WorkspaceID); e != nil {
			reasons = append(reasons, "workspace is no longer readable")
		} else {
			if ws.CommitID != v.CommitID {
				reasons = append(reasons, "workspace revision changed")
			}
			if ws.DefinitionSHA256 != v.DefinitionSHA256 {
				reasons = append(reasons, "workspace dependencies changed")
			}
		}
		v.Stale = len(reasons) > 0
		v.StaleReasons = reasons
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/support-threads/{thread_id}/verification-attempts", func(w http.ResponseWriter, r *http.Request) {
		_, _, thread, ok := access(w, r)
		if !ok {
			return
		}
		all, e := store.List(thread.RepositoryID, thread.ID)
		if e != nil {
			writeAPIError(w, 500, "support_verification_unavailable", "verification attempts could not be read")
			return
		}
		for i := range all {
			all[i] = project(all[i], thread)
		}
		writeJSON(w, 200, map[string]any{"attempts": all})
	})
	mux.HandleFunc("POST /repositories/{id}/support-threads/{thread_id}/verification-attempts", func(w http.ResponseWriter, r *http.Request) {
		a, _, thread, ok := access(w, r)
		if !ok {
			return
		}
		var in struct {
			AnswerID           string   `json:"answer_id"`
			AnswerRevisionID   string   `json:"answer_revision_id"`
			WorkspaceID        string   `json:"workspace_id"`
			InputAttachmentIDs []string `json:"input_attachment_ids"`
			InputsSanitized    bool     `json:"inputs_sanitized"`
			Commands           []struct {
				Command   string `json:"command"`
				OutcomeID string `json:"outcome_id"`
			} `json:"commands"`
			Artifacts []struct {
				Name      string `json:"name"`
				MediaType string `json:"media_type"`
				Data      string `json:"data"`
				Sanitized bool   `json:"sanitized"`
			} `json:"artifacts"`
			Result    string  `json:"result"`
			Notes     string  `json:"notes"`
			CostUnits float64 `json:"cost_units"`
			RerunOf   string  `json:"rerun_of"`
		}
		if decodeJSONLimit(r, &in, 15<<20) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if !in.InputsSanitized {
			writeAPIError(w, 422, "support_inputs_invalid", "reusable inputs must be explicitly reviewed and declared sanitized")
			return
		}
		answer, e := answers.Get(thread.RepositoryID, in.AnswerID)
		if e != nil {
			writeAPIError(w, 422, "support_answer_invalid", "answer was not found")
			return
		}
		var revision *knowledgeanswers.Revision
		for i := range answer.Revisions {
			if answer.Revisions[i].ID == in.AnswerRevisionID {
				revision = &answer.Revisions[i]
			}
		}
		if revision == nil || !answerCitesThread(*revision, thread.ID) {
			writeAPIError(w, 422, "support_answer_invalid", "answer revision must cite this support thread")
			return
		}
		ws, e := workspaceStore.Get(in.WorkspaceID)
		if e != nil || ws.RepositoryID != thread.RepositoryID {
			writeAPIError(w, 422, "support_workspace_invalid", "workspace was not found for this repository")
			return
		}
		if !supportWorkspaceMatches(ws, thread.ID, answer.ID, revision.ID) {
			writeAPIError(w, 422, "support_workspace_invalid", "workspace must have been launched for this exact support thread and answer revision")
			return
		}
		if ws.Policy.Sharing == "private" {
			writeAPIError(w, 422, "support_workspace_private", "private workspace output cannot be published as repository-readable verification evidence")
			return
		}
		member := false
		repo, _ := repos.GetByID(thread.RepositoryID)
		if repo.OwnerID == a.UserID {
			member = true
		} else {
			member, _ = repos.HasCollaborator(a.UserID, repo.ID)
		}
		if !member {
			writeAPIError(w, 404, "support_workspace_invalid", "workspace was not found for this participant")
			return
		}
		selected, ok := selectInputs(thread, in.InputAttachmentIDs)
		if !ok {
			writeAPIError(w, 422, "support_inputs_invalid", "inputs must be a unique subset of the thread's sanitized attachments")
			return
		}
		commands, seconds, ok := selectCommands(ws, in.Commands)
		if !ok {
			writeAPIError(w, 422, "support_commands_invalid", "commands must exactly match distinct retained workspace outcomes")
			return
		}
		artifacts, ok := verificationArtifacts(in.Artifacts)
		if !ok {
			writeAPIError(w, 422, "support_evidence_unsafe", "artifacts must be declared sanitized, bounded, and free of credential-shaped content")
			return
		}
		if reusableSecret.MatchString(revision.Body) || reusableSecret.MatchString(in.Notes) {
			writeAPIError(w, 422, "support_evidence_unsafe", "instructions and notes must not contain credentials or secrets")
			return
		}
		inputDigest := sha256.Sum256([]byte(strings.Join(selected, "\x00")))
		instructionDigest := sha256.Sum256([]byte(revision.Body))
		v := supportverifications.Attempt{RepositoryID: thread.RepositoryID, ThreadID: thread.ID, AnswerID: answer.ID, AnswerRevisionID: revision.ID, WorkspaceID: ws.ID, CommitID: ws.CommitID, DefinitionSHA256: ws.DefinitionSHA256, SoftwareVersion: thread.Target.Version, Environment: copyVerificationEnvironment(thread.Environment), InputAttachmentIDs: append([]string(nil), in.InputAttachmentIDs...), InputSHA256: hex.EncodeToString(inputDigest[:]), Instructions: revision.Body, InstructionsSHA256: hex.EncodeToString(instructionDigest[:]), Commands: commands, Artifacts: artifacts, Cost: supportverifications.Cost{ComputeSeconds: seconds, CostUnits: in.CostUnits}, Result: in.Result, Notes: strings.TrimSpace(in.Notes), ActorID: a.UserID, RerunOf: in.RerunOf}
		if in.RerunOf != "" {
			prior, x := store.Get(thread.RepositoryID, thread.ID, in.RerunOf)
			if x != nil || prior.AnswerRevisionID != v.AnswerRevisionID || prior.InputSHA256 != v.InputSHA256 || prior.InstructionsSHA256 != v.InstructionsSHA256 || prior.WorkspaceID == v.WorkspaceID {
				writeAPIError(w, 422, "support_rerun_invalid", "reruns require a fresh workspace and the exact prior answer and sanitized inputs")
				return
			}
		}
		v, e = store.Create(v)
		if e != nil {
			writeSupportVerificationError(w, e)
			return
		}
		w.Header().Set("Location", "/repositories/"+thread.RepositoryID+"/support-threads/"+thread.ID+"/verification-attempts/"+v.ID)
		writeJSON(w, 201, project(v, thread))
	})
	mux.HandleFunc("GET /repositories/{id}/support-threads/{thread_id}/verification-attempts/{attempt_id}", func(w http.ResponseWriter, r *http.Request) {
		_, _, thread, ok := access(w, r)
		if !ok {
			return
		}
		v, e := store.Get(thread.RepositoryID, thread.ID, r.PathValue("attempt_id"))
		if e != nil {
			writeSupportVerificationError(w, e)
			return
		}
		writeJSON(w, 200, project(v, thread))
	})
}

func supportWorkspaceMatches(w workspaces.Workspace, threadID, answerID, revisionID string) bool {
	return w.Source.Kind == "support_verification" &&
		w.Source.SupportThreadID == threadID &&
		w.Source.AnswerID == answerID &&
		w.Source.AnswerRevisionID == revisionID
}

func answerCitesThread(r knowledgeanswers.Revision, id string) bool {
	for _, c := range r.Claims {
		for _, x := range c.Citations {
			if x.Kind == "support_thread" && x.ResourceID == id {
				return true
			}
		}
	}
	return false
}
func copyVerificationEnvironment(e supportthreads.Environment) supportverifications.Environment {
	return supportverifications.Environment{OperatingSystem: e.OperatingSystem, Runtime: e.Runtime, Dependencies: append([]string(nil), e.Dependencies...), Deployment: e.Deployment, Details: e.Details}
}
func environmentHash(e supportthreads.Environment) string {
	return environmentVerificationHash(copyVerificationEnvironment(e))
}
func environmentVerificationHash(e supportverifications.Environment) string {
	deps := append([]string(nil), e.Dependencies...)
	sort.Strings(deps)
	d := sha256.Sum256([]byte(strings.Join([]string{e.OperatingSystem, e.Runtime, strings.Join(deps, "\x00"), e.Deployment, e.Details}, "\x01")))
	return hex.EncodeToString(d[:])
}
func selectInputs(t supportthreads.Thread, ids []string) ([]string, bool) {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range ids {
		if seen[id] {
			return nil, false
		}
		seen[id] = true
		found := false
		for _, a := range t.Attachments {
			if a.ID == id {
				raw, err := base64.StdEncoding.DecodeString(a.Data)
				if err != nil || reusableSecret.Match(raw) {
					return nil, false
				}
				d := sha256.Sum256([]byte(a.Kind + "\x00" + a.Name + "\x00" + a.Data))
				out = append(out, hex.EncodeToString(d[:]))
				found = true
			}
		}
		if !found {
			return nil, false
		}
	}
	sort.Strings(out)
	return out, true
}
func selectCommands(w workspaces.Workspace, in []struct {
	Command   string `json:"command"`
	OutcomeID string `json:"outcome_id"`
}) ([]supportverifications.Command, int, bool) {
	seen := map[string]bool{}
	out := []supportverifications.Command{}
	seconds := 0
	for _, x := range in {
		if seen[x.OutcomeID] || reusableSecret.MatchString(x.Command) {
			return nil, 0, false
		}
		seen[x.OutcomeID] = true
		d := sha256.Sum256([]byte(x.Command))
		found := false
		for _, o := range w.Commands {
			if o.ID == x.OutcomeID && o.CommandSHA256 == hex.EncodeToString(d[:]) {
				if reusableSecret.MatchString(o.Output) {
					return nil, 0, false
				}
				out = append(out, supportverifications.Command{Command: x.Command, Directory: o.Directory, OutcomeID: o.ID, ExitCode: o.ExitCode, Output: o.Output, StartedAt: o.StartedAt, CompletedAt: o.CompletedAt})
				n := int(o.CompletedAt.Sub(o.StartedAt) / time.Second)
				if n < 1 {
					n = 1
				}
				seconds += n
				found = true
			}
		}
		if !found {
			return nil, 0, false
		}
	}
	return out, seconds, len(out) > 0
}
func verificationArtifacts(in []struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
	Sanitized bool   `json:"sanitized"`
}) ([]supportverifications.Artifact, bool) {
	if len(in) > 10 {
		return nil, false
	}
	out := []supportverifications.Artifact{}
	for _, x := range in {
		raw, e := base64.StdEncoding.DecodeString(x.Data)
		if e != nil || len(raw) < 1 || len(raw) > 1<<20 || !x.Sanitized || x.Name == "" || x.MediaType == "" || reusableSecret.Match(raw) {
			return nil, false
		}
		d := sha256.Sum256(raw)
		out = append(out, supportverifications.Artifact{Name: x.Name, MediaType: x.MediaType, Size: len(raw), SHA256: hex.EncodeToString(d[:]), Data: x.Data})
	}
	return out, true
}
func writeSupportVerificationError(w http.ResponseWriter, e error) {
	if errors.Is(e, supportverifications.ErrNotFound) {
		writeAPIError(w, 404, "support_verification_not_found", "verification attempt not found")
	} else if errors.Is(e, supportverifications.ErrInvalid) {
		writeAPIError(w, 422, "support_verification_invalid", "verification evidence is incomplete or invalid")
	} else {
		writeAPIError(w, 500, "support_verification_unavailable", "verification evidence could not be saved")
	}
}
