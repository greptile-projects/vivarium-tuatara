package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"path"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerIssueRoutes(mux *http.ServeMux, repos *repositories.Store, store *issues.Store, releaseStore *releases.Store, workspaceStore *workspaces.Store, credentials *auth.Store, activity *activities.Store) {
	const issueBodyLimit = 15 << 20
	require := func(w http.ResponseWriter, r *http.Request, scope string) (auth.Credential, bool) {
		actor, ok := authenticateRequest(w, r, credentials, scope, false)
		if !ok {
			return actor, false
		}
		repo, err := repos.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return actor, false
		}
		collaborator, err := repos.HasCollaborator(actor.UserID, repo.ID)
		if repo.OwnerID != actor.UserID && (err != nil || !collaborator) {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return actor, false
		}
		return actor, true
	}
	visible := func(actor string, v issues.Issue) bool {
		if v.Visibility == "public" {
			return true
		}
		repo, err := repos.GetByID(v.RepositoryID)
		if err != nil {
			return false
		}
		if repo.OwnerID == actor {
			return true
		}
		ok, _ := repos.HasCollaborator(actor, v.RepositoryID)
		return ok
	}
	mux.HandleFunc("GET /repositories/{id}/issue-templates", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := require(w, r, "repositories:read"); !ok {
			return
		}
		writeJSON(w, 200, map[string]any{"templates": []map[string]any{
			{"id": "bug", "name": "Unexpected behavior", "description": "A reproducible product or code failure.", "fields": []string{"expected_behavior", "observed_behavior", "severity", "environment", "reproduction_steps"}},
			{"id": "regression", "name": "Released regression", "description": "Behavior that changed in a released version.", "fields": []string{"affected_version", "expected_behavior", "observed_behavior", "severity", "environment", "reproduction_steps"}},
		}})
	})
	mux.HandleFunc("GET /repositories/{id}/issue-suggestions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if len(query) < 3 {
			writeJSON(w, 200, map[string]any{"issues": []issues.Issue{}})
			return
		}
		all, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "issue_read_failed", "issues could not be read")
			return
		}
		terms := strings.Fields(strings.ToLower(query))
		type ranked struct {
			v issues.Issue
			n int
		}
		matches := []ranked{}
		for _, v := range all {
			if !visible(actor.UserID, v) {
				continue
			}
			text := strings.ToLower(v.Title + " " + v.ObservedBehavior)
			score := 0
			for _, term := range terms {
				if strings.Contains(text, term) {
					score++
				}
			}
			if score > 0 {
				v.Attachments = nil
				v.Discussion = nil
				matches = append(matches, ranked{v, score})
			}
		}
		sort.Slice(matches, func(i, j int) bool { return matches[i].n > matches[j].n })
		out := []issues.Issue{}
		for i := 0; i < len(matches) && i < 5; i++ {
			out = append(out, matches[i].v)
		}
		writeJSON(w, 200, map[string]any{"issues": out})
	})
	mux.HandleFunc("GET /repositories/{id}/issues", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		all, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "issue_read_failed", "issues could not be read")
			return
		}
		out := all[:0]
		for _, v := range all {
			if visible(actor.UserID, v) {
				out = append(out, v)
			}
		}
		writeJSON(w, 200, map[string]any{"issues": out})
	})
	mux.HandleFunc("POST /repositories/{id}/issues", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input issues.Issue
		if err := decodeIssueJSON(r, &input, issueBodyLimit); err != nil {
			if errors.Is(err, errIssueBodyTooLarge) {
				writeAPIError(w, 413, "issue_body_too_large", "issue request exceeds the 15 MiB aggregate limit")
				return
			}
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		input.RepositoryID, input.ReporterID = r.PathValue("id"), actor.UserID
		input.Discussion = nil
		if input.ReleaseID == "" && input.AffectedVersion != "" {
			writeAPIError(w, 422, "invalid_affected_release", "affected_version is server-derived and requires release_id")
			return
		}
		if input.ReleaseID != "" {
			if releaseStore == nil {
				writeAPIError(w, 503, "releases_unavailable", "releases could not be read")
				return
			}
			release, err := releaseStore.Get(input.RepositoryID, input.ReleaseID)
			if err != nil || input.AffectedVersion != "" && input.AffectedVersion != release.Version {
				writeAPIError(w, 422, "invalid_affected_release", "release_id must name the affected repository release")
				return
			}
			input.AffectedVersion = release.Version
		}
		for i := range input.Attachments {
			raw, err := base64.StdEncoding.DecodeString(input.Attachments[i].Data)
			if err != nil || len(raw) > 1<<20 || input.Attachments[i].Size != 0 && input.Attachments[i].Size != len(raw) {
				writeAPIError(w, 422, "invalid_issue_attachment", "attachment must be valid base64 and at most 1 MiB")
				return
			}
			input.Attachments[i].Size = len(raw)
		}
		created, err := store.Create(input)
		if err != nil && !errors.Is(err, issues.ErrDurabilityUncertain) {
			writeIssueError(w, err)
			return
		}
		recordActivity(activity, repos, activities.Event{Kind: "issue.opened", ActorID: actor.UserID, RepositoryID: created.RepositoryID, ResourceType: "issue", ResourceID: created.ID, ResourceTitle: created.Title})
		w.Header().Set("Location", "/repositories/"+created.RepositoryID+"/issues/"+created.ID)
		if errors.Is(err, issues.ErrDurabilityUncertain) {
			w.Header().Set("Vivarium-Durability", "uncertain")
			writeJSON(w, 202, created)
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/issues/{issue_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		v, err := store.Get(r.PathValue("id"), r.PathValue("issue_id"))
		if err != nil || !visible(actor.UserID, v) {
			writeAPIError(w, 404, "issue_not_found", "issue not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/issues/{issue_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Body string `json:"body"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, err := store.AddComment(r.PathValue("id"), r.PathValue("issue_id"), actor.UserID, input.Body)
		if err != nil && !errors.Is(err, issues.ErrDurabilityUncertain) {
			writeIssueError(w, err)
			return
		}
		if errors.Is(err, issues.ErrDurabilityUncertain) {
			w.Header().Set("Vivarium-Durability", "uncertain")
			writeJSON(w, 202, v)
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("PATCH /repositories/{id}/issues/{issue_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Status          string `json:"status"`
			ExpectedVersion int    `json:"expected_version"`
			Message         string `json:"message"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		repo, err := repos.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		v, err := store.UpdateStatus(r.PathValue("id"), r.PathValue("issue_id"), actor.UserID, input.Status, input.ExpectedVersion, input.Message, repo.OwnerID == actor.UserID)
		if err != nil && !errors.Is(err, issues.ErrDurabilityUncertain) {
			writeIssueError(w, err)
			return
		}
		if errors.Is(err, issues.ErrDurabilityUncertain) {
			w.Header().Set("Vivarium-Durability", "uncertain")
			writeJSON(w, 202, v)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/issues/{issue_id}/reproduction-attempts", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			WorkspaceID        string   `json:"workspace_id"`
			InputAttachmentIDs []string `json:"input_attachment_ids"`
			CommandOutcomeIDs  []string `json:"command_outcome_ids"`
			ObservedResult     string   `json:"observed_result"`
			Result             string   `json:"result"`
			ArtifactPaths      []string `json:"artifact_paths"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		issue, err := store.Get(r.PathValue("id"), r.PathValue("issue_id"))
		if err != nil {
			writeIssueError(w, err)
			return
		}
		if workspaceStore == nil {
			writeAPIError(w, 503, "reproduction_workspace_unavailable", "workspace evidence is unavailable")
			return
		}
		workspace, err := workspaceStore.Get(strings.TrimSpace(input.WorkspaceID))
		if err != nil || workspace.RepositoryID != issue.RepositoryID || workspace.Source.Kind != "issue_reproduction" || workspace.Source.IssueID != issue.ID || workspace.State == "provisioning" {
			writeAPIError(w, 422, "reproduction_workspace_invalid", "workspace must be a completed issue-reproduction workspace for this issue")
			return
		}
		declared := map[string]string{}
		for _, command := range workspace.Definition.Experiments {
			sum := sha256.Sum256([]byte(command.Command))
			declared[hex.EncodeToString(sum[:])] = command.Name
		}
		outcomes := map[string]workspaces.CommandOutcome{}
		for _, outcome := range workspace.Commands {
			outcomes[outcome.ID] = outcome
		}
		attempt := issues.ReproductionAttempt{WorkspaceID: workspace.ID, CommitID: workspace.CommitID, ReleaseID: workspace.Source.ReleaseID, DefinitionSHA256: workspace.DefinitionSHA256, ObservedResult: strings.TrimSpace(input.ObservedResult), Result: input.Result}
		attempt.EnvironmentDefinition, _ = json.Marshal(workspace.Definition)
		seen := map[string]bool{}
		for _, id := range input.CommandOutcomeIDs {
			outcome, found := outcomes[id]
			name, allowed := declared[outcome.CommandSHA256]
			if !found || !allowed || seen[id] {
				writeAPIError(w, 422, "reproduction_commands_invalid", "every outcome must be unique and match a repository-defined reproduction command")
				return
			}
			seen[id] = true
			attempt.Commands = append(attempt.Commands, issues.ReproductionCommand{Name: name, OutcomeID: id, CommandSHA256: outcome.CommandSHA256, ExitCode: outcome.ExitCode, Log: outcome.Output, StartedAt: outcome.StartedAt, CompletedAt: outcome.CompletedAt})
		}
		attachments := map[string]issues.Attachment{}
		for _, attachment := range issue.Attachments {
			attachments[attachment.ID] = attachment
		}
		staged := map[string]bool{}
		for _, id := range workspace.ReproductionInputAttachmentIDs {
			staged[id] = true
		}
		for _, id := range input.InputAttachmentIDs {
			attachment, found := attachments[id]
			if !found || !staged[id] || reproductionSecretLike(attachment.Name, attachment.Data) {
				writeAPIError(w, 422, "reproduction_input_invalid", "inputs must be sanitized attachments from this issue")
				return
			}
			raw, _ := base64.StdEncoding.DecodeString(attachment.Data)
			sum := sha256.Sum256(raw)
			attempt.Inputs = append(attempt.Inputs, issues.ReproductionInput{AttachmentID: id, Name: attachment.Name, SHA256: hex.EncodeToString(sum[:]), Size: len(raw)})
		}
		total := 0
		for _, artifactPath := range input.ArtifactPaths {
			clean, valid := cleanReproductionArtifactPath(artifactPath)
			if !valid {
				writeAPIError(w, 422, "reproduction_artifact_invalid", "artifact paths must stay inside the workspace")
				return
			}
			raw, decodeErr := readReproductionArtifact(workspace.ID, clean)
			total += len(raw)
			encoded := base64.StdEncoding.EncodeToString(raw)
			if decodeErr != nil || len(raw) > 4<<20 || total > 16<<20 || reproductionSecretLike(clean, encoded) {
				writeAPIError(w, 422, "reproduction_artifact_invalid", "artifacts must be sanitized, checksummable, and within the 16 MiB attempt limit")
				return
			}
			sum := sha256.Sum256(raw)
			attempt.Artifacts = append(attempt.Artifacts, issues.ReproductionArtifact{Name: clean, MediaType: "application/octet-stream", SHA256: hex.EncodeToString(sum[:]), Size: len(raw), Data: encoded})
		}
		updated, err := store.AddReproductionAttempt(issue.RepositoryID, issue.ID, actor.UserID, attempt)
		if err != nil && !errors.Is(err, issues.ErrDurabilityUncertain) {
			writeIssueError(w, err)
			return
		}
		if errors.Is(err, issues.ErrDurabilityUncertain) {
			w.Header().Set("Vivarium-Durability", "uncertain")
			writeJSON(w, 202, updated)
			return
		}
		writeJSON(w, 201, updated)
	})
}

func cleanReproductionArtifactPath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "/workspace/") {
		value = strings.TrimPrefix(value, "/workspace/")
	} else if strings.HasPrefix(value, "/") {
		return "", false
	}
	if value == "" || strings.Contains(value, "\\") {
		return "", false
	}
	for _, component := range strings.Split(value, "/") {
		if component == ".." {
			return "", false
		}
	}
	clean := path.Clean(value)
	return clean, clean != "." && !strings.HasPrefix(clean, "/")
}

const reproductionArtifactReadScript = `
set -f
target=/workspace
old_ifs=$IFS
IFS=/
for component in $1; do
  [ -n "$component" ] || continue
  target=$target/$component
  [ ! -L "$target" ] || exit 42
done
IFS=$old_ifs
exec 3<"$target" || exit 43
resolved=$(readlink -f /proc/self/fd/3) || exit 44
case "$resolved" in
  /workspace/*) ;;
  *) exit 45 ;;
esac
[ -f /proc/self/fd/3 ] || exit 46
exec cat <&3
`

func readReproductionArtifact(workspaceID, relativePath string) ([]byte, error) {
	return exec.Command("docker", "exec", "vivarium-workspace-"+workspaceID, "sh", "-c", reproductionArtifactReadScript, "reproduction-artifact-read", relativePath).Output()
}

var errIssueBodyTooLarge = errors.New("issue body too large")

func decodeIssueJSON(r *http.Request, destination any, limit int64) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return errIssueBodyTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func writeIssueError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, issues.ErrNotFound):
		writeAPIError(w, 404, "issue_not_found", "issue not found")
	case errors.Is(err, issues.ErrConflict):
		writeAPIError(w, 409, "issue_changed", "issue changed; reload and retry")
	case errors.Is(err, issues.ErrForbidden):
		writeAPIError(w, 403, "issue_status_forbidden", "only the repository owner can change a terminal issue status")
	case errors.Is(err, issues.ErrInvalid):
		writeAPIError(w, 422, "invalid_issue", "issue fields or attachments are invalid")
	default:
		writeAPIError(w, 500, "issue_write_failed", "issue could not be saved")
	}
}
