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
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerIssueRoutes(mux *http.ServeMux, gitStore *storage.Store, repos *repositories.Store, store *issues.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, incidentStore *incidents.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, packageStore *packages.Store, workspaceStore *workspaces.Store, credentials *auth.Store, activity *activities.Store) {
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
	mux.HandleFunc("PUT /repositories/{id}/issues/{issue_id}/triage", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion   int      `json:"expected_version"`
			Classification    string   `json:"classification"`
			Priority          string   `json:"priority"`
			AssigneeID        string   `json:"assignee_id"`
			SuspectedRevision string   `json:"suspected_revision"`
			SuspectedOwnerIDs []string `json:"suspected_owner_ids"`
			DuplicateOf       string   `json:"duplicate_of"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if in.ExpectedVersion < 1 {
			writeAPIError(w, 422, "invalid_issue_version", "expected_version is required")
			return
		}
		validClass := map[string]bool{"bug": true, "regression": true, "performance": true, "compatibility": true, "documentation": true, "support": true, "unknown": true}
		validPriority := map[string]bool{"low": true, "normal": true, "high": true, "urgent": true}
		if !validClass[in.Classification] || !validPriority[in.Priority] {
			writeAPIError(w, 422, "invalid_triage", "classification and priority are invalid")
			return
		}
		participant := func(id string) bool {
			if id == "" {
				return true
			}
			repo, err := repos.GetByID(r.PathValue("id"))
			if err != nil {
				return false
			}
			if repo.OwnerID == id {
				return true
			}
			ok, _ := repos.HasCollaborator(id, repo.ID)
			return ok
		}
		if !participant(strings.TrimSpace(in.AssigneeID)) {
			writeAPIError(w, 422, "invalid_triage_owner", "assignee must be a current repository participant")
			return
		}
		seenOwners := map[string]bool{}
		for _, id := range in.SuspectedOwnerIDs {
			if seenOwners[id] || !participant(id) {
				writeAPIError(w, 422, "invalid_triage_owner", "suspected owners must be distinct current repository participants")
				return
			}
			seenOwners[id] = true
		}
		if in.SuspectedRevision != "" && (len(in.SuspectedRevision) != 40 || strings.Trim(in.SuspectedRevision, "0123456789abcdef") != "") {
			writeAPIError(w, 422, "invalid_suspected_revision", "suspected revision must be an exact commit ID")
			return
		}
		if in.DuplicateOf != "" {
			if other, e := store.Get(r.PathValue("id"), in.DuplicateOf); e != nil || other.ID == r.PathValue("issue_id") {
				writeAPIError(w, 422, "invalid_duplicate", "duplicate must name another visible repository issue")
				return
			}
		}
		v, e := store.Mutate(r.PathValue("id"), r.PathValue("issue_id"), actor.UserID, in.ExpectedVersion, func(v *issues.Issue) error {
			v.Triage = issues.Triage{Classification: in.Classification, Priority: in.Priority, AssigneeID: strings.TrimSpace(in.AssigneeID), SuspectedRevision: strings.TrimSpace(in.SuspectedRevision), SuspectedOwnerIDs: in.SuspectedOwnerIDs, UpdatedBy: actor.UserID, UpdatedAt: time.Now().UTC()}
			v.DuplicateOf = in.DuplicateOf
			issues.AddHistory(v, "triage_updated", actor.UserID, in.Classification+" / "+in.Priority)
			return nil
		})
		writeIssueMutation(w, v, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/issues/{issue_id}/links", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Kind            string `json:"kind"`
			RepositoryID    string `json:"repository_id"`
			ResourceID      string `json:"resource_id"`
			Revision        string `json:"revision"`
			Label           string `json:"label"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if in.ExpectedVersion < 1 {
			writeAPIError(w, 422, "invalid_issue_version", "expected_version is required")
			return
		}
		kinds := map[string]bool{"code": true, "dependency": true, "release": true, "deployment": true, "incident": true, "proposal": true, "pull_request": true, "issue": true}
		if !kinds[in.Kind] || strings.TrimSpace(in.ResourceID) == "" || strings.TrimSpace(in.Label) == "" {
			writeAPIError(w, 422, "invalid_issue_link", "typed links require a resource and label")
			return
		}
		in.RepositoryID = strings.TrimSpace(in.RepositoryID)
		if in.RepositoryID == "" {
			writeAPIError(w, 422, "invalid_issue_link", "repository_id is required for issue evidence")
			return
		}
		if err := resolveIssueLink(gitStore, repos, store, releaseStore, deploymentStore, incidentStore, proposalStore, pullStore, packageStore, actor.UserID, in.Kind, in.RepositoryID, in.ResourceID, in.Revision); err != nil {
			writeAPIError(w, 422, "invalid_issue_link", "evidence target or exact revision could not be resolved")
			return
		}
		var v issues.Issue
		e := repos.WithCurrentReadAccess(actor.UserID, []string{in.RepositoryID}, func() error {
			var mutationErr error
			v, mutationErr = store.Mutate(r.PathValue("id"), r.PathValue("issue_id"), actor.UserID, in.ExpectedVersion, func(v *issues.Issue) error {
				for _, x := range v.Links {
					if x.Kind == in.Kind && x.RepositoryID == in.RepositoryID && x.ResourceID == in.ResourceID {
						return issues.ErrConflict
					}
				}
				v.Links = append(v.Links, issues.Link{ID: issues.NewID(), Kind: in.Kind, RepositoryID: in.RepositoryID, ResourceID: in.ResourceID, Revision: in.Revision, Label: strings.TrimSpace(in.Label), AddedBy: actor.UserID, CreatedAt: time.Now().UTC()})
				issues.AddHistory(v, "evidence_linked", actor.UserID, in.Kind+": "+in.Label)
				return nil
			})
			return mutationErr
		})
		if errors.Is(e, repositories.ErrNotFound) {
			writeAPIError(w, 422, "invalid_issue_link", "evidence repository is unavailable")
			return
		}
		writeIssueMutation(w, v, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/issues/{issue_id}/evidence-requests", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Body            string `json:"body"`
		}
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Body) == "" || in.ExpectedVersion < 1 {
			writeAPIError(w, 422, "invalid_evidence_request", "request text is required")
			return
		}
		v, e := store.Mutate(r.PathValue("id"), r.PathValue("issue_id"), actor.UserID, in.ExpectedVersion, func(v *issues.Issue) error {
			now := time.Now().UTC()
			v.EvidenceRequests = append(v.EvidenceRequests, issues.EvidenceRequest{ID: issues.NewID(), Body: strings.TrimSpace(in.Body), RequestedFrom: v.ReporterID, RequestedBy: actor.UserID, State: "open", CreatedAt: now, UpdatedAt: now})
			issues.AddHistory(v, "evidence_requested", actor.UserID, in.Body)
			return nil
		})
		writeIssueMutation(w, v, e, 201)
	})
	mux.HandleFunc("PUT /repositories/{id}/issues/{issue_id}/evidence-requests/{request_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Response        string `json:"response"`
		}
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Response) == "" || in.ExpectedVersion < 1 {
			writeAPIError(w, 422, "invalid_evidence_response", "response is required")
			return
		}
		v, e := store.Mutate(r.PathValue("id"), r.PathValue("issue_id"), actor.UserID, in.ExpectedVersion, func(v *issues.Issue) error {
			if actor.UserID != v.ReporterID {
				return issues.ErrForbidden
			}
			for i := range v.EvidenceRequests {
				if v.EvidenceRequests[i].ID == r.PathValue("request_id") && v.EvidenceRequests[i].State == "open" {
					v.EvidenceRequests[i].State = "answered"
					v.EvidenceRequests[i].Response = strings.TrimSpace(in.Response)
					v.EvidenceRequests[i].RespondedBy = actor.UserID
					v.EvidenceRequests[i].UpdatedAt = time.Now().UTC()
					issues.AddHistory(v, "evidence_answered", actor.UserID, in.Response)
					return nil
				}
			}
			return issues.ErrNotFound
		})
		writeIssueMutation(w, v, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/issues/{issue_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		addIssueFinding(w, r, repos, store, actor, "", 1)
	})
	mux.HandleFunc("POST /repositories/{id}/issues/{issue_id}/findings/{finding_id}/challenges", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Body            string `json:"body"`
		}
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Body) == "" || in.ExpectedVersion < 1 {
			writeAPIError(w, 422, "invalid_challenge", "challenge is required")
			return
		}
		v, e := store.Mutate(r.PathValue("id"), r.PathValue("issue_id"), actor.UserID, in.ExpectedVersion, func(v *issues.Issue) error {
			for i := range v.Findings {
				if v.Findings[i].ID == r.PathValue("finding_id") {
					v.Findings[i].Challenges = append(v.Findings[i].Challenges, issues.Challenge{ID: issues.NewID(), ActorID: actor.UserID, Body: strings.TrimSpace(in.Body), CreatedAt: time.Now().UTC()})
					issues.AddHistory(v, "finding_disputed", actor.UserID, in.Body)
					return nil
				}
			}
			return issues.ErrNotFound
		})
		writeIssueMutation(w, v, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/issues/{issue_id}/investigations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion       int      `json:"expected_version"`
			Mandate               string   `json:"mandate"`
			ReproductionAttemptID string   `json:"reproduction_attempt_id"`
			LinkIDs               []string `json:"link_ids"`
			ExpiresIn             int      `json:"expires_in"`
		}
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Mandate) == "" || in.ExpiresIn < 300 || in.ExpiresIn > 86400 || in.ExpectedVersion < 1 {
			writeAPIError(w, 422, "invalid_investigation", "select a reproduction, mandate, and 5 minute to 24 hour expiry")
			return
		}
		issued, e := credentials.Issue(actor.UserID, auth.API, "Issue investigation", []string{"issues:investigate"}, time.Duration(in.ExpiresIn)*time.Second)
		if e != nil {
			writeAPIError(w, 500, "investigation_failed", "credential could not be issued")
			return
		}
		agentID := issues.NewID()
		var investigation issues.Investigation
		v, e := store.Mutate(r.PathValue("id"), r.PathValue("issue_id"), actor.UserID, in.ExpectedVersion, func(v *issues.Issue) error {
			found := false
			for _, x := range v.ReproductionAttempts {
				found = found || x.ID == in.ReproductionAttemptID
			}
			if !found {
				return issues.ErrInvalid
			}
			allowed := map[string]bool{}
			for _, x := range v.Links {
				allowed[x.ID] = true
			}
			for _, id := range in.LinkIDs {
				if !allowed[id] {
					return issues.ErrInvalid
				}
			}
			now := time.Now().UTC()
			investigation = issues.Investigation{ID: issues.NewID(), AgentID: agentID, InitiatorID: actor.UserID, CredentialID: issued.ID, Mandate: strings.TrimSpace(in.Mandate), ReproductionAttemptID: in.ReproductionAttemptID, LinkIDs: in.LinkIDs, State: "running", CreatedAt: now, UpdatedAt: now}
			v.Investigations = append(v.Investigations, investigation)
			issues.AddHistory(v, "investigation_started", actor.UserID, investigation.ID)
			return nil
		})
		if e != nil && !errors.Is(e, issues.ErrDurabilityUncertain) {
			_, _ = credentials.Revoke(actor.UserID, issued.ID)
			writeIssueMutation(w, v, e, 201)
			return
		}
		if errors.Is(e, issues.ErrDurabilityUncertain) {
			w.Header().Set("Vivarium-Durability", "uncertain")
			writeJSON(w, 202, map[string]any{"issue": v, "investigation": investigation, "credential": issued})
			return
		}
		writeJSON(w, 201, map[string]any{"issue": v, "investigation": investigation, "credential": issued})
	})
	mux.HandleFunc("GET /repositories/{id}/issues/{issue_id}/investigations/{investigation_id}", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "issues:investigate", false)
		if !ok {
			return
		}
		v, e := store.Get(r.PathValue("id"), r.PathValue("issue_id"))
		if e != nil {
			writeAPIError(w, 404, "investigation_not_found", "investigation not found")
			return
		}
		for _, x := range v.Investigations {
			if x.ID == r.PathValue("investigation_id") && x.CredentialID == credential.ID && x.State == "running" {
				authorizationErr := repos.WithCurrentParticipant(x.InitiatorID, v.RepositoryID, func() error {
					var attempt issues.ReproductionAttempt
					for _, a := range v.ReproductionAttempts {
						if a.ID == x.ReproductionAttemptID {
							attempt = a
						}
					}
					links := []issues.Link{}
					for _, l := range v.Links {
						for _, id := range x.LinkIDs {
							if l.ID == id {
								links = append(links, l)
							}
						}
					}
					writeJSON(w, 200, map[string]any{"investigation": x, "reproduction": attempt, "links": links})
					return nil
				})
				if authorizationErr != nil {
					writeAPIError(w, 403, "investigation_access_changed", "the investigation initiator no longer has repository access")
				}
				return
			}
		}
		writeAPIError(w, 404, "investigation_not_found", "investigation not found")
	})
	mux.HandleFunc("POST /repositories/{id}/issues/{issue_id}/investigations/{investigation_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "issues:investigate", false)
		if !ok {
			return
		}
		addIssueFinding(w, r, repos, store, credential, r.PathValue("investigation_id"), 0)
	})
}

func addIssueFinding(w http.ResponseWriter, r *http.Request, catalog *repositories.Store, store *issues.Store, actor auth.Credential, investigationID string, requireExpected int) {
	var in struct {
		ExpectedVersion int      `json:"expected_version"`
		Kind            string   `json:"kind"`
		Statement       string   `json:"statement"`
		CitationIDs     []string `json:"citation_ids"`
		SupersedesID    string   `json:"supersedes_id"`
	}
	if decodeJSON(r, &in) != nil {
		writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
		return
	}
	if in.Kind != "hypothesis" && in.Kind != "finding" && in.Kind != "uncertainty" || strings.TrimSpace(in.Statement) == "" || len(in.CitationIDs) == 0 {
		writeAPIError(w, 422, "invalid_finding", "a typed statement and citations are required")
		return
	}
	if requireExpected == 1 && in.ExpectedVersion < 1 {
		writeAPIError(w, 422, "invalid_issue_version", "expected_version is required")
		return
	}
	findingActor, authorityID := actor.UserID, actor.UserID
	if investigationID != "" {
		current, err := store.Get(r.PathValue("id"), r.PathValue("issue_id"))
		if err != nil {
			writeIssueError(w, err)
			return
		}
		matched := false
		for _, x := range current.Investigations {
			if x.ID == investigationID && x.CredentialID == actor.ID {
				authorityID, matched = x.InitiatorID, true
			}
		}
		if !matched {
			writeAPIError(w, 403, "investigation_access_changed", "the investigation selection is unavailable")
			return
		}
	}
	var v issues.Issue
	e := catalog.WithCurrentParticipant(authorityID, r.PathValue("id"), func() error {
		var mutationErr error
		v, mutationErr = store.Mutate(r.PathValue("id"), r.PathValue("issue_id"), actor.UserID, in.ExpectedVersion, func(v *issues.Issue) error {
			allowed := map[string]bool{}
			for _, x := range v.Links {
				allowed[x.ID] = true
			}
			for _, x := range v.ReproductionAttempts {
				allowed[x.ID] = true
			}
			if investigationID != "" {
				ok := false
				for _, x := range v.Investigations {
					if x.ID == investigationID && x.CredentialID == actor.ID && x.State == "running" {
						ok = true
						findingActor = x.AgentID
						allowed = map[string]bool{x.ReproductionAttemptID: true}
						for _, id := range x.LinkIDs {
							allowed[id] = true
						}
					}
				}
				if !ok {
					return issues.ErrForbidden
				}
			}
			for _, id := range in.CitationIDs {
				if !allowed[id] {
					return issues.ErrInvalid
				}
			}
			if in.SupersedesID != "" {
				found := false
				for _, x := range v.Findings {
					found = found || x.ID == in.SupersedesID
				}
				if !found {
					return issues.ErrInvalid
				}
			}
			v.Findings = append(v.Findings, issues.Finding{ID: issues.NewID(), Kind: in.Kind, Statement: strings.TrimSpace(in.Statement), ActorID: findingActor, InvestigationID: investigationID, CitationIDs: in.CitationIDs, SupersedesID: in.SupersedesID, CreatedAt: time.Now().UTC()})
			issues.AddHistory(v, "finding_published", findingActor, in.Kind)
			return nil
		})
		return mutationErr
	})
	if errors.Is(e, repositories.ErrInvalidCollaborator) || errors.Is(e, repositories.ErrNotFound) {
		writeAPIError(w, 403, "investigation_access_changed", "repository authority changed before finding publication")
		return
	}
	writeIssueMutation(w, v, e, 201)
}

func writeIssueMutation(w http.ResponseWriter, v issues.Issue, err error, status int) {
	if err != nil && !errors.Is(err, issues.ErrDurabilityUncertain) {
		writeIssueError(w, err)
		return
	}
	if errors.Is(err, issues.ErrDurabilityUncertain) {
		w.Header().Set("Vivarium-Durability", "uncertain")
		writeJSON(w, 202, v)
		return
	}
	writeJSON(w, status, v)
}

func resolveIssueLink(gitStore *storage.Store, catalog *repositories.Store, issueStore *issues.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, incidentStore *incidents.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, packageStore *packages.Store, actorID, kind, repositoryID, resourceID, revision string) error {
	resourceID, revision = strings.TrimSpace(resourceID), strings.TrimSpace(revision)
	switch kind {
	case "code":
		if gitStore == nil || len(revision) != 40 {
			return issues.ErrInvalid
		}
		repository, err := gitStore.Open(repositoryID)
		if err != nil {
			return err
		}
		commit, err := repository.ReadCommit(storage.ObjectID(revision))
		if err != nil {
			return err
		}
		paths, err := repository.WalkTree(commit.Tree)
		if err != nil {
			return err
		}
		for _, entry := range paths {
			if entry.Path == resourceID && entry.Type == storage.BlobObject {
				return nil
			}
		}
	case "dependency":
		if packageStore == nil || len(revision) != 40 {
			return issues.ErrInvalid
		}
		inventory, err := packageStore.GetInventory(repositoryID, revision)
		if err != nil {
			return err
		}
		for _, entry := range inventory.Entries {
			if entry.Name == resourceID {
				return nil
			}
		}
	case "release":
		if releaseStore != nil {
			value, err := releaseStore.Get(repositoryID, resourceID)
			if err == nil && (revision == "" || value.CommitID == revision) {
				return nil
			}
		}
	case "deployment":
		if deploymentStore != nil {
			value, err := deploymentStore.GetPromotion(repositoryID, resourceID)
			if err == nil && (revision == "" || value.CommitID == revision) {
				return nil
			}
		}
	case "incident":
		participant, _ := catalog.HasCollaborator(actorID, repositoryID)
		repository, _ := catalog.GetByID(repositoryID)
		if repository.OwnerID != actorID && !participant {
			return issues.ErrForbidden
		}
		if incidentStore != nil {
			value, err := incidentStore.Get(resourceID)
			if err == nil {
				for _, scope := range value.Scopes {
					if scope.RepositoryID == repositoryID {
						return nil
					}
				}
			}
		}
	case "proposal":
		if proposalStore != nil {
			if _, err := proposalStore.Get(repositoryID, resourceID); err == nil {
				return nil
			}
		}
	case "pull_request":
		if pullStore != nil {
			value, err := pullStore.Get(repositoryID, resourceID)
			if err == nil && (revision == "" || value.SourceCommitID == revision) {
				return nil
			}
		}
	case "issue":
		if issueStore != nil {
			value, err := issueStore.Get(repositoryID, resourceID)
			participant, _ := catalog.HasCollaborator(actorID, repositoryID)
			repository, _ := catalog.GetByID(repositoryID)
			if err == nil && (value.Visibility == "public" || repository.OwnerID == actorID || participant) {
				return nil
			}
		}
	}
	return issues.ErrInvalid
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
