package main

import (
	"errors"
	"net/http"
	"os/exec"
	"path"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func registerPreviewRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, pulls *pullrequests.Store, runs *checkruns.Store, store *previews.Store, sessionStore *changesessions.Store, authStore *auth.Store, userStore *users.Store, proposalStore *proposals.Store, decisionStore *decisions.Store, issueStore *issues.Store, activityStore *activities.Store) {
	project := func(p previews.Preview) previews.Preview {
		if run, e := runs.Get(p.RepositoryID, p.PullRequestID, p.BuildRunID); e == nil {
			p.State = run.State
		}
		return p
	}
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, authStore, r.PathValue("id"))
		if !ok {
			return
		}
		participant := false
		if authenticated {
			repository, _ := catalog.GetByID(r.PathValue("id"))
			collaborator, _ := catalog.HasCollaborator(actor.UserID, r.PathValue("id"))
			participant = repository.OwnerID == actor.UserID || collaborator
		}
		pull, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, e) {
			return
		}
		list, e := store.List(pull.RepositoryID, pull.ID, pull.SourceCommitID)
		if e != nil {
			writeAPIError(w, 500, "internal_error", "previews unavailable")
			return
		}
		for i := range list {
			list[i] = project(list[i])
			if !participant {
				list[i].Invitations, list[i].AudienceEvents, list[i].Feedback, list[i].Findings = nil, nil, nil, nil
			}
		}
		writeJSON(w, 200, map[string]any{"previews": list})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/invitations", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "preview_owner_required", "only the repository owner can manage preview guests")
			return
		}
		var in struct {
			UserID     string    `json:"user_id"`
			SourceKind string    `json:"source_kind"`
			SourceID   string    `json:"source_id"`
			Role       string    `json:"role"`
			ExpiresAt  time.Time `json:"expires_at"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		p, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("preview_id"))
		if err != nil {
			writeAPIError(w, 404, "preview_not_found", "preview not found")
			return
		}
		usersToInvite, err := resolvePreviewAudience(in.UserID, in.SourceKind, in.SourceID, p.RepositoryID, userStore, proposalStore, decisionStore, issueStore)
		if err != nil {
			writeAPIError(w, 422, "preview_audience_invalid", err.Error())
			return
		}
		if !containsString(p.Definition.Access.Actions, in.Role) {
			writeAPIError(w, 422, "preview_role_restricted", "the preview definition does not allow this role")
			return
		}
		for _, userID := range usersToInvite {
			p, err = store.Invite(p.RepositoryID, p.PullRequestID, p.ID, actor.UserID, userID, in.Role, in.SourceKind, in.SourceID, in.ExpiresAt)
			if err != nil {
				writeAPIError(w, 422, "preview_invitation_invalid", "invitation must be named, bounded to 30 days, and allowed by policy")
				return
			}
			target := userID
			recordActivity(activityStore, catalog, activities.Event{Kind: "preview.invited", ActorID: actor.UserID, RepositoryID: p.RepositoryID, ResourceType: "preview", ResourceID: p.ID, ResourceTitle: "Change preview", TargetUserID: &target})
		}
		writeJSON(w, 201, previewAudience(p, time.Now().UTC()))
	})
	mux.HandleFunc("DELETE /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/invitations/{invitation_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "preview_owner_required", "only the repository owner can manage preview guests")
			return
		}
		p, err := store.Revoke(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("preview_id"), r.PathValue("invitation_id"), actor.UserID)
		if err != nil {
			writeAPIError(w, 404, "preview_invitation_not_found", "preview invitation not found")
			return
		}
		writeJSON(w, 200, previewAudience(p, time.Now().UTC()))
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/audience", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		p, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("preview_id"))
		if err != nil {
			writeAPIError(w, 404, "preview_not_found", "preview not found")
			return
		}
		_ = actor
		writeJSON(w, 200, previewAudience(p, time.Now().UTC()))
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/feedback", func(w http.ResponseWriter, r *http.Request) {
		actor, invitation, p, ok := authorizePreviewGuest(w, r, catalog, store, authStore)
		if !ok {
			return
		}
		if invitation.Role != "feedback" || !containsString(p.Definition.Access.Actions, "feedback") {
			writeAPIError(w, 403, "preview_feedback_restricted", "this invitation does not allow feedback")
			return
		}
		var in struct {
			Body string `json:"body"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if len(strings.TrimSpace(in.Body)) < 1 || utf8.RuneCountInString(in.Body) > 4000 {
			writeAPIError(w, 422, "preview_feedback_invalid", "feedback must be 1-4000 characters")
			return
		}
		if _, err := store.AddFeedback(p.RepositoryID, p.PullRequestID, p.ID, actor.UserID, invitation.ID, in.Body); err != nil {
			writeAPIError(w, 409, "preview_feedback_restricted", "the invitation is no longer active")
			return
		}
		recordActivity(activityStore, catalog, activities.Event{Kind: "preview.feedback", ActorID: actor.UserID, RepositoryID: p.RepositoryID, ResourceType: "preview", ResourceID: p.ID, ResourceTitle: "Change preview feedback"})
		writeJSON(w, 201, map[string]any{"recorded": true, "actor_id": actor.UserID, "role": invitation.Role})
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		_, invitation, p, ok := authorizePreviewGuest(w, r, catalog, store, authStore)
		if !ok {
			return
		}
		projection := project(p)
		projection.Invitations, projection.AudienceEvents, projection.Feedback = nil, nil, nil
		writeJSON(w, 200, map[string]any{"preview_id": p.ID, "revision": p.Revision, "preview": projection, "effective_role": invitation.Role, "findings": p.Findings, "evidence_policy": map[string]any{"visibility": "preview_audience", "kinds": []string{"screenshot", "recording", "console", "trace", "annotation"}, "max_item_bytes": 5 << 20, "max_total_bytes": 12 << 20, "sensitive_text": "redacted"}})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		actor, invitation, p, ok := authorizePreviewGuest(w, r, catalog, store, authStore)
		if !ok {
			return
		}
		if invitation.Role != "feedback" || !containsString(p.Definition.Access.Actions, "feedback") {
			writeAPIError(w, 403, "preview_feedback_restricted", "this invitation does not allow findings")
			return
		}
		var in struct {
			Route             string                     `json:"route"`
			Title             string                     `json:"title"`
			Description       string                     `json:"description"`
			Classification    string                     `json:"classification"`
			Severity          string                     `json:"severity"`
			DuplicateOf       string                     `json:"duplicate_of"`
			ReproductionSteps []string                   `json:"reproduction_steps"`
			Evidence          []previews.FindingEvidence `json:"evidence"`
		}
		if decodeJSONLimit(r, &in, 17<<20) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		_, finding, err := store.AddFinding(p.RepositoryID, p.PullRequestID, p.ID, actor.UserID, in.Route, in.Title, in.Description, in.Classification, in.Severity, in.DuplicateOf, in.ReproductionSteps, in.Evidence)
		if err != nil {
			writeAPIError(w, 422, "preview_finding_invalid", "finding must be revision-routed, bounded, policy-permitted, and relate only visible evidence")
			return
		}
		recordActivity(activityStore, catalog, activities.Event{Kind: "preview.finding", ActorID: actor.UserID, RepositoryID: p.RepositoryID, ResourceType: "pull_request", ResourceID: p.PullRequestID, ResourceTitle: finding.Title})
		writeJSON(w, 201, finding)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/findings/{finding_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, invitation, p, ok := authorizePreviewGuest(w, r, catalog, store, authStore)
		if !ok {
			return
		}
		if invitation.Role != "feedback" || !containsString(p.Definition.Access.Actions, "feedback") {
			writeAPIError(w, 403, "preview_feedback_restricted", "this invitation does not allow discussion")
			return
		}
		var in struct {
			Body    string `json:"body"`
			Version int    `json:"version"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		body, _ := previews.RedactSensitive(strings.TrimSpace(in.Body))
		if body == "" || utf8.RuneCountInString(body) > 4000 {
			writeAPIError(w, 422, "preview_comment_invalid", "comment must be 1-4000 characters")
			return
		}
		_, finding, err := store.MutateFinding(p.RepositoryID, p.PullRequestID, p.ID, r.PathValue("finding_id"), actor.UserID, in.Version, func(f *previews.Finding) error {
			now := time.Now().UTC()
			f.Comments = append(f.Comments, previews.FindingComment{ID: previews.NewID(), AuthorID: actor.UserID, Body: body, CreatedAt: now})
			f.Events = append(f.Events, previews.FindingEvent{ID: previews.NewID(), Kind: "commented", ActorID: actor.UserID, CreatedAt: now})
			return nil
		})
		if err != nil {
			writeAPIError(w, 409, "preview_finding_changed", "finding changed or is unavailable")
			return
		}
		writeJSON(w, 201, finding)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/findings/{finding_id}/decision", func(w http.ResponseWriter, r *http.Request) {
		actor, invitation, p, ok := authorizePreviewGuest(w, r, catalog, store, authStore)
		if !ok {
			return
		}
		if invitation.Role != "feedback" || !containsString(p.Definition.Access.Actions, "feedback") {
			writeAPIError(w, 403, "preview_feedback_restricted", "this invitation does not allow finding decisions")
			return
		}
		var in struct {
			Version        int    `json:"version"`
			Status         string `json:"status"`
			Classification string `json:"classification"`
			Severity       string `json:"severity"`
			DuplicateOf    string `json:"duplicate_of"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if in.Status == "" && in.Classification == "" && in.Severity == "" && in.DuplicateOf == "" {
			writeAPIError(w, 422, "preview_finding_decision_invalid", "a finding decision must change status, classification, severity, or duplicate relation")
			return
		}
		_, finding, err := store.MutateFinding(p.RepositoryID, p.PullRequestID, p.ID, r.PathValue("finding_id"), actor.UserID, in.Version, func(f *previews.Finding) error {
			now := time.Now().UTC()
			if in.Status != "" {
				if in.Status != "open" && in.Status != "resolved" {
					return previews.ErrInvalid
				}
				old := f.Status
				f.Status = in.Status
				kind := "resolved"
				if in.Status == "open" {
					kind = "reopened"
				}
				f.Events = append(f.Events, previews.FindingEvent{ID: previews.NewID(), Kind: kind, ActorID: actor.UserID, From: old, To: in.Status, CreatedAt: now})
			}
			if in.Classification != "" {
				if !containsString([]string{"bug", "usability", "accessibility", "content", "performance", "question", "other"}, in.Classification) {
					return previews.ErrInvalid
				}
				old := f.Classification
				f.Classification = in.Classification
				f.Events = append(f.Events, previews.FindingEvent{ID: previews.NewID(), Kind: "classified", ActorID: actor.UserID, From: old, To: in.Classification, CreatedAt: now})
			}
			if in.Severity != "" {
				if !containsString([]string{"blocking", "major", "minor", "note"}, in.Severity) {
					return previews.ErrInvalid
				}
				old := f.Severity
				f.Severity = in.Severity
				f.Events = append(f.Events, previews.FindingEvent{ID: previews.NewID(), Kind: "severity_changed", ActorID: actor.UserID, From: old, To: in.Severity, CreatedAt: now})
			}
			if in.DuplicateOf != "" {
				if in.DuplicateOf == f.ID {
					return previews.ErrInvalid
				}
				exists := false
				for _, candidate := range p.Findings {
					if candidate.ID == in.DuplicateOf {
						exists = true
					}
				}
				if !exists {
					return previews.ErrInvalid
				}
				f.DuplicateOf = in.DuplicateOf
				f.Events = append(f.Events, previews.FindingEvent{ID: previews.NewID(), Kind: "related_as_duplicate", ActorID: actor.UserID, To: in.DuplicateOf, CreatedAt: now})
			}
			return nil
		})
		if err != nil {
			writeAPIError(w, 409, "preview_finding_changed", "finding changed or decision is invalid")
			return
		}
		writeJSON(w, 200, finding)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/findings/{finding_id}/repair", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if sessionStore == nil {
			writeAPIError(w, 503, "repair_unavailable", "repair sessions unavailable")
			return
		}
		var in struct {
			Version            int      `json:"version"`
			AcceptanceCriteria []string `json:"acceptance_criteria"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if len(in.AcceptanceCriteria) == 0 || len(in.AcceptanceCriteria) > 20 {
			writeAPIError(w, 422, "preview_repair_invalid", "one to 20 acceptance criteria are required")
			return
		}
		for i := range in.AcceptanceCriteria {
			in.AcceptanceCriteria[i] = strings.TrimSpace(in.AcceptanceCriteria[i])
			if in.AcceptanceCriteria[i] == "" || utf8.RuneCountInString(in.AcceptanceCriteria[i]) > 1000 {
				writeAPIError(w, 422, "preview_repair_invalid", "acceptance criteria must be 1-1000 characters")
				return
			}
		}
		p, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("preview_id"))
		if err != nil {
			writeAPIError(w, 404, "preview_not_found", "preview not found")
			return
		}
		var source *previews.Finding
		for i := range p.Findings {
			if p.Findings[i].ID == r.PathValue("finding_id") {
				source = &p.Findings[i]
				break
			}
		}
		if source == nil {
			writeAPIError(w, 404, "preview_finding_not_found", "preview finding not found")
			return
		}
		if source.Repair != nil {
			if !slices.Equal(source.Repair.AcceptanceCriteria, in.AcceptanceCriteria) {
				writeAPIError(w, 409, "preview_finding_changed", "finding already has different repair criteria")
				return
			}
			if source.Repair.PublishedCommitID != "" && source.Repair.PreviewAttemptID == "" {
				if current, pullErr := pulls.Get(p.RepositoryID, p.PullRequestID); pullErr == nil && current.Status == pullrequests.Open && current.SourceCommitID == source.Repair.PublishedCommitID {
					if attempt, previewErr := createRepairPreviewAttempt(git, runs, store, current, actor.UserID, source.Repair.SessionID, source.ID); previewErr == nil {
						_, repaired, updateErr := store.MutateFinding(p.RepositoryID, p.PullRequestID, p.ID, source.ID, actor.UserID, source.Version, func(f *previews.Finding) error { f.Repair.PreviewAttemptID = attempt.ID; return nil })
						if updateErr == nil {
							source = &repaired
						}
					}
				}
			}
			session, sessionErr := sessionStore.Get(p.RepositoryID, p.PullRequestID, source.Repair.SessionID)
			if writeChangeSessionError(w, sessionErr) {
				return
			}
			writeJSON(w, 200, map[string]any{"finding": source, "session": session})
			return
		}
		pull, err := pulls.Get(p.RepositoryID, p.PullRequestID)
		if writePullRequestError(w, err) {
			return
		}
		if pull.Status != pullrequests.Open || pull.SourceCommitID != p.Revision {
			writeAPIError(w, 409, "preview_repair_stale", "repairs require an open pull at the finding revision")
			return
		}
		evidence := changesessions.PreviewEvidence{PreviewID: p.ID, FindingID: source.ID, Revision: source.Revision, Route: source.Route, Title: source.Title, Description: source.Description, Classification: source.Classification, Severity: source.Severity, AuthorID: source.AuthorID, ReproductionSteps: source.ReproductionSteps, AcceptanceCriteria: in.AcceptanceCriteria}
		for _, item := range source.Evidence {
			evidence.Evidence = append(evidence.Evidence, changesessions.PreviewArtifact{ID: item.ID, Kind: item.Kind, Name: item.Name, MediaType: item.MediaType, Size: item.Size, Data: item.Data, Redacted: item.Redacted})
		}
		for _, item := range source.Comments {
			evidence.Discussion = append(evidence.Discussion, changesessions.PreviewDiscussion{ID: item.ID, AuthorID: item.AuthorID, Body: item.Body, CreatedAt: item.CreatedAt})
		}
		var session changesessions.Session
		err = pulls.WithSourceRevision(pull.RepositoryID, pull.ID, p.Revision, func(current pullrequests.PullRequest) error {
			var createErr error
			session, createErr = sessionStore.FindOrCreateWithPreviewEvidence(current.RepositoryID, current.ID, actor.UserID, current.SourceCommitID, evidence)
			return createErr
		})
		if errors.Is(err, pullrequests.ErrSourceChanged) || errors.Is(err, pullrequests.ErrNotReady) {
			writeAPIError(w, 409, "preview_repair_stale", "pull request changed while repair was created")
			return
		}
		if err != nil && !errors.Is(err, changesessions.ErrDurabilityUncertain) {
			writeChangeSessionError(w, err)
			return
		}
		_, finding, linkErr := store.MutateFinding(p.RepositoryID, p.PullRequestID, p.ID, source.ID, actor.UserID, in.Version, func(f *previews.Finding) error {
			if f.Repair != nil {
				if f.Repair.SessionID == session.ID {
					return nil
				}
				return previews.ErrInvalid
			}
			now := time.Now().UTC()
			f.Repair = &previews.FindingRepair{SessionID: session.ID, AcceptanceCriteria: append([]string(nil), in.AcceptanceCriteria...), CreatedBy: actor.UserID, CreatedAt: now}
			f.Events = append(f.Events, previews.FindingEvent{ID: previews.NewID(), Kind: "repair_created", ActorID: actor.UserID, To: session.ID, CreatedAt: now})
			return nil
		})
		if linkErr != nil {
			writeAPIError(w, 409, "preview_finding_changed", "finding changed; retry to reconcile the retained repair session")
			return
		}
		w.Header().Set("Location", "/repositories/"+p.RepositoryID+"/pulls/"+p.PullRequestID+"/sessions/"+session.ID)
		writeJSON(w, 201, map[string]any{"finding": finding, "session": session})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/previews", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		pull, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, e) {
			return
		}
		if pull.Status != pullrequests.Open {
			writeAPIError(w, 409, "pull_request_not_open", "pull request is not open")
			return
		}
		repository, e := git.Open(pull.RepositoryID)
		if e != nil {
			writeAPIError(w, 503, "preview_source_unavailable", "preview source unavailable")
			return
		}
		data, e := exec.Command("git", "--git-dir="+repository.Path(), "show", pull.SourceCommitID+":"+previews.ConfigPath).Output()
		if e != nil {
			writeAPIError(w, 422, "preview_definition_missing", "candidate must contain .vivarium/preview.json")
			return
		}
		config, hash, e := previews.ParseConfig(data)
		if e != nil {
			writeAPIError(w, 422, "invalid_preview_definition", e.Error())
			return
		}
		command := previewBuildCommand(config)
		definitionName := "preview-" + hash[:12]
		createdRuns, e := runs.CreateRequested(pull.RepositoryID, pull.ID, pull.SourceCommitID, []checkruns.Definition{{Name: definitionName, Image: config.Image, Command: command, WorkingDirectory: config.WorkingDirectory, Environment: config.Environment, TimeoutSeconds: config.Resources.TimeoutSeconds, CPUs: config.Resources.CPUs, MemoryMB: config.Resources.MemoryMB, StorageMB: config.Resources.StorageMB}}, actor.UserID)
		var buildRun *checkruns.Run
		for i := range createdRuns {
			if createdRuns[i].Definition.Name == definitionName {
				buildRun = &createdRuns[i]
				break
			}
		}
		if e != nil || buildRun == nil {
			writeAPIError(w, 503, "preview_build_unavailable", "preview build unavailable")
			return
		}
		p, e := store.Create(pull.RepositoryID, pull.ID, pull.SourceCommitID, actor.UserID, hash, buildRun.ID, config)
		if e != nil {
			writeAPIError(w, 503, "preview_storage_unavailable", "preview storage unavailable")
			return
		}
		go runs.Execute(*buildRun, repository.Path())
		writeJSON(w, 201, p)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/events", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, authStore, r.PathValue("id")); !ok {
			return
		}
		p, e := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("preview_id"))
		if errors.Is(e, previews.ErrNotFound) {
			writeAPIError(w, 404, "preview_not_found", "preview not found")
			return
		}
		events, e := runs.Events(p.RepositoryID, p.PullRequestID, p.BuildRunID, 0)
		if e != nil {
			writeAPIError(w, 503, "preview_logs_unavailable", "preview logs unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"events": events})
	})
	serveContent := func(w http.ResponseWriter, r *http.Request) {
		if _, _, _, ok := authorizePreviewGuest(w, r, catalog, store, authStore); !ok {
			return
		}
		p, e := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("preview_id"))
		if e != nil {
			writeAPIError(w, 404, "preview_not_found", "preview not found")
			return
		}
		run, e := runs.Get(p.RepositoryID, p.PullRequestID, p.BuildRunID)
		if e != nil || run.State != "succeeded" {
			writeAPIError(w, 409, "preview_not_ready", "preview is not ready")
			return
		}
		requested := r.PathValue("asset")
		if requested == "" {
			requested = "index.html"
		}
		requested = path.Clean(strings.TrimPrefix(requested, "/"))
		if requested == "." || requested == ".." || strings.HasPrefix(requested, "../") {
			writeAPIError(w, 404, "preview_asset_not_found", "preview asset not found")
			return
		}
		var selected *checkruns.Artifact
		for i := range run.Artifacts {
			if run.Artifacts[i].Path == requested {
				selected = &run.Artifacts[i]
				break
			}
		}
		if selected == nil {
			writeAPIError(w, 404, "preview_asset_not_found", "preview asset not found")
			return
		}
		file, a, e := runs.OpenArtifact(p.RepositoryID, p.PullRequestID, p.BuildRunID, selected.ID)
		if e != nil {
			writeAPIError(w, 503, "preview_content_unavailable", "preview content unavailable")
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", a.ContentType)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'none'; form-action 'none'; frame-ancestors 'none'; sandbox allow-scripts")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "private, no-store")
		http.ServeContent(w, r, path.Base(a.Path), a.CreatedAt, file)
	}
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/content", serveContent)
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/previews/{preview_id}/content/{asset...}", serveContent)
}

func previewAudience(p previews.Preview, now time.Time) map[string]any {
	return map[string]any{"preview_id": p.ID, "revision": p.Revision, "policy": p.Definition.Access, "invitations": p.Invitations, "events": p.AudienceEvents, "feedback": p.Feedback, "effective_access": map[string]any{"active_invitation_count": func() int {
		n := 0
		for _, i := range p.Invitations {
			if i.RevokedAt == nil && i.ExpiresAt.After(now) {
				n++
			}
		}
		return n
	}(), "grants_repository_access": false, "grants_workspace_access": false, "grants_deployment_access": false, "grants_environment_access": false, "credentials_exposed": false, "private_services_exposed": false}}
}
func resolvePreviewAudience(userID, kind, sourceID, repositoryID string, userStore *users.Store, proposalStore *proposals.Store, decisionStore *decisions.Store, issueStore *issues.Store) ([]string, error) {
	set := map[string]bool{}
	if kind == "user" {
		if userID == "" {
			return nil, errors.New("user_id is required")
		}
		set[userID] = true
	} else if kind == "proposal" && proposalStore != nil {
		p, e := proposalStore.Get(repositoryID, sourceID)
		if e != nil {
			return nil, errors.New("proposal not found")
		}
		set[p.AuthorID] = true
		comments, e := proposalStore.ListComments(repositoryID, sourceID)
		if e != nil {
			return nil, e
		}
		for _, c := range comments {
			set[c.AuthorID] = true
		}
	} else if kind == "decision" && decisionStore != nil {
		d, e := decisionStore.Get(sourceID)
		if e != nil || d.RepositoryID != repositoryID {
			return nil, errors.New("decision not found")
		}
		for _, p := range d.Scope.Participants {
			set[p.UserID] = true
		}
	} else if kind == "issue" && issueStore != nil {
		i, e := issueStore.Get(repositoryID, sourceID)
		if e != nil {
			return nil, errors.New("issue not found")
		}
		set[i.ReporterID] = true
		for _, c := range i.Discussion {
			set[c.AuthorID] = true
		}
	} else {
		return nil, errors.New("source_kind must be user, issue, decision, or proposal")
	}
	out := []string{}
	for id := range set {
		if _, e := userStore.Get(id); e != nil {
			return nil, errors.New("audience contains an unknown user")
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}
func authorizePreviewGuest(w http.ResponseWriter, r *http.Request, catalog *repositories.Store, store *previews.Store, authStore *auth.Store) (auth.Credential, previews.Invitation, previews.Preview, bool) {
	actor, ok := authenticateRequest(w, r, authStore, "repositories:read", false)
	if !ok {
		return auth.Credential{}, previews.Invitation{}, previews.Preview{}, false
	}
	repo, e := catalog.GetByID(r.PathValue("id"))
	if e != nil {
		writeAPIError(w, 404, "repository_not_found", "repository not found")
		return actor, previews.Invitation{}, previews.Preview{}, false
	}
	collaborator, _ := catalog.HasCollaborator(actor.UserID, repo.ID)
	p, e := store.Get(repo.ID, r.PathValue("pull_id"), r.PathValue("preview_id"))
	if e != nil {
		writeAPIError(w, 404, "preview_not_found", "preview not found")
		return actor, previews.Invitation{}, previews.Preview{}, false
	}
	if actor.UserID == repo.OwnerID || collaborator {
		return actor, previews.Invitation{Role: "feedback"}, p, true
	}
	updated, inv, e := store.Enter(repo.ID, p.PullRequestID, p.ID, actor.UserID)
	if e != nil {
		writeAPIError(w, 404, "preview_not_found", "preview not found")
		return actor, previews.Invitation{}, previews.Preview{}, false
	}
	return actor, inv, updated, true
}

func createRepairPreviewAttempt(git *storage.Store, runs *checkruns.Store, store *previews.Store, pull pullrequests.PullRequest, actorID, sessionID, findingID string) (previews.Preview, error) {
	repository, err := git.Open(pull.RepositoryID)
	if err != nil {
		return previews.Preview{}, err
	}
	data, err := exec.Command("git", "--git-dir="+repository.Path(), "show", pull.SourceCommitID+":"+previews.ConfigPath).Output()
	if err != nil {
		return previews.Preview{}, err
	}
	config, hash, err := previews.ParseConfig(data)
	if err != nil {
		return previews.Preview{}, err
	}
	command := previewBuildCommand(config)
	p, err := store.ReserveRepairAttempt(pull.RepositoryID, pull.ID, pull.SourceCommitID, actorID, hash, sessionID, findingID, config)
	if err != nil {
		return previews.Preview{}, err
	}
	definitionName := "preview-repair-" + p.ID
	var run checkruns.Run
	if p.BuildRunID != "" {
		run, err = runs.Get(pull.RepositoryID, pull.ID, p.BuildRunID)
		if err != nil {
			return previews.Preview{}, err
		}
	} else {
		createdRuns, createErr := runs.CreateRequested(pull.RepositoryID, pull.ID, pull.SourceCommitID, []checkruns.Definition{{Name: definitionName, Image: config.Image, Command: command, WorkingDirectory: config.WorkingDirectory, Environment: config.Environment, TimeoutSeconds: config.Resources.TimeoutSeconds, CPUs: config.Resources.CPUs, MemoryMB: config.Resources.MemoryMB, StorageMB: config.Resources.StorageMB}}, actorID)
		if createErr != nil {
			return previews.Preview{}, createErr
		}
		if len(createdRuns) == 1 {
			run = createdRuns[0]
		} else {
			all, listErr := runs.List(pull.RepositoryID, pull.ID)
			if listErr != nil {
				return previews.Preview{}, listErr
			}
			for _, candidate := range all {
				if candidate.CommitID == pull.SourceCommitID && candidate.Definition.Name == definitionName {
					run = candidate
					break
				}
			}
		}
		if run.ID == "" {
			return previews.Preview{}, errors.New("preview build unavailable")
		}
		p, err = store.AttachBuildRun(pull.RepositoryID, pull.ID, p.ID, run.ID)
		if err != nil {
			return previews.Preview{}, err
		}
	}
	if run.State == "queued" {
		go runs.Execute(run, repository.Path())
	}
	return p, nil
}

func previewBuildCommand(config previews.Config) string {
	quotedWorking := strings.ReplaceAll(config.WorkingDirectory, "'", "'\\''")
	quotedOutput := strings.ReplaceAll(config.OutputPath, "'", "'\\''")
	// Check execution deliberately mounts the exact Git archive read-only. A
	// preview gets a disposable, storage-bounded /tmp copy for compilation; only
	// the declared output is copied into the separately bounded artifact mount.
	return "rm -rf /tmp/vivarium-preview && mkdir -p /tmp/vivarium-preview && cp -R /workspace/. /tmp/vivarium-preview/ && cd '/tmp/vivarium-preview/" + quotedWorking + "' && " + config.Build + " && test -d '" + quotedOutput + "' && cp -R '" + quotedOutput + "'/. \"$VIVARIUM_OUTPUT\"/"
}
