package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	docscollections "github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func publishMergedDocumentation(git *storage.Store, docs *docscollections.Store, pull pullrequests.PullRequest, actorID string) error {
	if pull.MergeCommitID == nil {
		return nil
	}
	gr, err := git.Open(pull.RepositoryID)
	if err != nil {
		return err
	}
	commit, err := gr.ReadCommit(storage.ObjectID(*pull.MergeCommitID))
	if err != nil {
		return err
	}
	tree, err := gr.WalkTree(commit.Tree)
	if err != nil {
		return err
	}
	byPath := map[string]storage.TreePath{}
	for _, entry := range tree {
		byPath[entry.Path] = entry
	}
	collections, err := docs.Collections(pull.RepositoryID)
	if err != nil {
		return err
	}
	for _, current := range collections {
		if !current.PublicationPolicy.PublishOnMerge || current.PublicationPolicy.SourceBranch != pull.TargetBranch || current.PublishedPullID == pull.ID {
			continue
		}
		pages := []docscollections.Page{}
		changed := false
		for _, old := range current.Pages {
			entry, ok := byPath[old.Path]
			if !ok || entry.Type != storage.BlobObject {
				continue
			}
			object, e := gr.ReadObject(entry.ID)
			if e != nil {
				return e
			}
			sum := sha256.Sum256(object.Content)
			next := old
			next.SourceObjectID = string(entry.ID)
			next.SourceSHA256 = fmt.Sprintf("%x", sum)
			next.Authors = commitAuthors(commit)
			next.Title = titleFromDocument(object.Content, path.Base(strings.TrimSuffix(old.Path, path.Ext(old.Path))))
			next.NavigationTitle = next.Title
			pages = append(pages, next)
			changed = changed || next.SourceObjectID != old.SourceObjectID
		}
		changed = changed || len(pages) != len(current.Pages)
		if !changed {
			continue
		}
		next := current
		next.ID = ""
		next.Version = 0
		next.SourceRevision = *pull.MergeCommitID
		next.Pages = pages
		next.PublishedBy = actorID
		next.PublishedPullID = pull.ID
		next.Diagnostics = nil
		for i := range next.SupportedVersions {
			if next.SupportedVersions[i].SourceRef == pull.TargetBranch && next.SupportedVersions[i].ReleaseID == "" {
				next.SupportedVersions[i].Revision = *pull.MergeCommitID
			}
		}
		if _, e := docs.Publish(next, current.Version); e != nil && !errors.Is(e, docscollections.ErrDurabilityUncertain) {
			return e
		}
	}
	return nil
}

func documentationRandomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func registerDocumentationRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, docs *docscollections.Store, releaseStore *releases.Store, issueStore *issues.Store, proposalStore *proposals.Store, credentials *auth.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool, bool) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return auth.Credential{}, false, false
		}
		if !authenticated {
			if optional, found, allowed := authenticateOptionalRequest(w, r, credentials, "repositories:read", false); !allowed {
				return auth.Credential{}, false, false
			} else if found {
				actor, authenticated = optional, true
			}
		}
		return actor, authenticated, true
	}
	present := func(repositoryID string, v docscollections.Revision) docscollections.Revision {
		v.Diagnostics = []docscollections.Diagnostic{}
		if len(v.Owners) == 0 {
			v.Diagnostics = append(v.Diagnostics, docscollections.Diagnostic{Code: "missing_owner", Severity: "error", Detail: "No maintainer or reviewer owns this collection."})
		}
		gr, gitErr := git.Open(repositoryID)
		refTargets := map[string]string{}
		resolveRef := func(name string) string {
			if target, ok := refTargets[name]; ok {
				return target
			}
			if gitErr == nil {
				if ref, err := gr.ReadReference("refs/heads/" + name); err == nil {
					refTargets[name] = ref.Target
					return ref.Target
				}
			}
			refTargets[name] = ""
			return ""
		}
		entries := []storage.TreePath{}
		if gitErr == nil {
			if currentRevision := resolveRef(v.SourceRef); currentRevision != "" {
				if commit, e := gr.ReadCommit(storage.ObjectID(currentRevision)); e == nil {
					entries, _ = gr.WalkTree(commit.Tree)
				}
			}
		}
		byPath := map[string]storage.TreePath{}
		for _, entry := range entries {
			byPath[entry.Path] = entry
		}
		for i := range v.Pages {
			page := &v.Pages[i]
			current, ok := byPath[page.Path]
			if !ok || current.Type != storage.BlobObject {
				page.Status, page.StatusDetail = "broken", "Source path is missing from the selected source branch."
				v.Diagnostics = append(v.Diagnostics, docscollections.Diagnostic{Code: "broken_source", Severity: "error", PagePath: page.Path, Detail: page.StatusDetail})
			} else if string(current.ID) != page.SourceObjectID {
				page.Status, page.StatusDetail = "stale", "Source has changed since the reviewed publication."
				v.Diagnostics = append(v.Diagnostics, docscollections.Diagnostic{Code: "stale_source", Severity: "warning", PagePath: page.Path, Detail: page.StatusDetail})
			} else {
				page.Status, page.StatusDetail = "current", "Source matches the reviewed publication."
			}
		}
		for i := range v.SupportedVersions {
			mapping := &v.SupportedVersions[i]
			mapping.Status, mapping.StatusDetail = "current", "Version mapping resolves to the reviewed source."
			if mapping.ReleaseID != "" {
				if releaseStore == nil {
					mapping.Status, mapping.StatusDetail = "stale", "Mapped release is unavailable."
				} else if candidate, e := releaseStore.Get(repositoryID, mapping.ReleaseID); e != nil {
					mapping.Status, mapping.StatusDetail = "stale", "Mapped release is unavailable."
				} else if mapping.Revision != "" && candidate.CommitID != mapping.Revision {
					mapping.Status, mapping.StatusDetail = "stale", "Release no longer matches the mapped revision."
				}
			} else if mapping.Revision != "" && resolveRef(mapping.SourceRef) != mapping.Revision {
				mapping.Status, mapping.StatusDetail = "stale", "Source ref has advanced beyond the mapped revision."
			}
			if mapping.Status == "stale" {
				v.Diagnostics = append(v.Diagnostics, docscollections.Diagnostic{Code: "stale_version_mapping", Severity: "warning", VersionLabel: mapping.Label, Detail: mapping.StatusDetail})
			}
		}
		return v
	}
	visible := func(repositoryID string, v docscollections.Revision, actor auth.Credential, authenticated bool) bool {
		if v.Audience == "public" {
			return true
		}
		if !authenticated {
			return false
		}
		repo, err := repos.GetByID(repositoryID)
		if err != nil {
			return false
		}
		if actor.UserID == repo.OwnerID {
			return true
		}
		collaborator, _ := repos.HasCollaborator(actor.UserID, repositoryID)
		return v.Audience == "repository" && collaborator
	}
	selectRevision := func(repositoryID, collectionID, selector string, actor auth.Credential, authenticated bool) (docscollections.Revision, []docscollections.Revision, error) {
		history, err := docs.List(repositoryID, collectionID)
		if err != nil {
			return docscollections.Revision{}, nil, err
		}
		for i := len(history) - 1; i >= 0; i-- {
			candidate := history[i]
			if !visible(repositoryID, candidate, actor, authenticated) {
				continue
			}
			if selector == "" || candidate.ID == selector {
				return candidate, history, nil
			}
			for _, mapping := range candidate.SupportedVersions {
				if mapping.Label == selector {
					return candidate, history, nil
				}
			}
		}
		return docscollections.Revision{}, history, docscollections.ErrNotFound
	}
	participant := func(w http.ResponseWriter, r *http.Request) (auth.Credential, *repositories.Repository, bool) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return auth.Credential{}, nil, false
		}
		repo, e := repos.GetByID(r.PathValue("id"))
		if e != nil {
			writeRepositoryError(w, e)
			return auth.Credential{}, nil, false
		}
		return actor, &repo, true
	}
	validateReferences := func(repositoryID string, task docscollections.Task, references []docscollections.Reference) error {
		gr, err := git.Open(repositoryID)
		if err != nil {
			return err
		}
		commit, err := gr.ReadCommit(storage.ObjectID(task.BaseRevision))
		if err != nil {
			return err
		}
		tree, err := gr.WalkTree(commit.Tree)
		if err != nil {
			return err
		}
		paths := map[string]storage.TreePath{}
		for _, item := range tree {
			paths[item.Path] = item
		}
		for _, reference := range references {
			if reference.Revision != task.BaseRevision || strings.TrimSpace(reference.Label) == "" || reference.StartLine < 0 || reference.EndLine < reference.StartLine {
				return docscollections.ErrInvalid
			}
			if reference.Path != "" {
				if reference.ResourceKind != "" || reference.ResourceID != "" {
					return docscollections.ErrInvalid
				}
				item, exists := paths[reference.Path]
				if !exists || item.Type != storage.BlobObject {
					return docscollections.ErrInvalid
				}
				object, readErr := gr.ReadObject(item.ID)
				if readErr != nil {
					return readErr
				}
				lineCount := len(strings.Split(strings.TrimSuffix(string(object.Content), "\n"), "\n"))
				if reference.StartLine > lineCount || reference.EndLine > lineCount {
					return docscollections.ErrInvalid
				}
			} else if reference.StartLine != 0 || reference.EndLine != 0 || reference.ResourceKind != task.Source.Kind || reference.ResourceID != task.Source.ResourceID {
				// The task's source tuple was frozen when the task was opened. Other
				// resource citations require their owning store and fail closed here.
				return docscollections.ErrInvalid
			}
		}
		return nil
	}
	mux.HandleFunc("POST /repositories/{id}/documentation-tasks", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, ok := participant(w, r)
		if !ok {
			return
		}
		var in struct {
			Title  string                     `json:"title"`
			Path   string                     `json:"path"`
			Source docscollections.TaskSource `json:"source"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		gr, e := git.Open(repo.ID)
		if e != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		if _, e = gr.ReadCommit(storage.ObjectID(in.Source.Revision)); e != nil {
			writeAPIError(w, 422, "documentation_task_revision_invalid", "source revision must be an exact repository commit")
			return
		}
		id := documentationRandomID()
		if id == "" {
			writeAPIError(w, 500, "documentation_task_create_failed", "documentation task identity could not be created")
			return
		}
		branch := "docs/tasks/" + id
		candidate := docscollections.Task{ID: id, RepositoryID: repo.ID, Title: in.Title, Path: in.Path, Branch: branch, BaseRevision: in.Source.Revision, Source: in.Source, CreatedBy: actor.UserID}
		if strings.TrimSpace(in.Title) == "" || in.Path == "" || !map[string]bool{"proposal": true, "issue": true, "pull_request": true, "release": true, "investigation": true, "stewardship_opportunity": true}[in.Source.Kind] || len(in.Source.ResourceID) != 32 || strings.TrimSpace(in.Source.Label) == "" {
			writeAPIError(w, 422, "documentation_task_invalid", "title, path, source, and exact revision are required")
			return
		}
		if e = gr.CreateReference(storage.Reference{Name: "refs/heads/" + branch, Target: candidate.BaseRevision}); e != nil {
			writeAPIError(w, 409, "documentation_task_branch_conflict", "scoped documentation branch could not be created")
			return
		}
		created, e := docs.CreateTask(candidate)
		if e != nil {
			_ = gr.DeleteReferenceIfTarget("refs/heads/"+branch, candidate.BaseRevision)
			writeAPIError(w, 500, "documentation_task_create_failed", "documentation task could not be persisted")
			return
		}
		w.Header().Set("Location", "/repositories/"+repo.ID+"/documentation-tasks/"+created.ID)
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/documentation-tasks/{taskID}", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := participant(w, r)
		if !ok {
			return
		}
		v, e := docs.GetTask(r.PathValue("id"), r.PathValue("taskID"))
		if e != nil {
			writeAPIError(w, 404, "documentation_task_not_found", "documentation task not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/documentation-tasks/{taskID}/drafts", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := participant(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                         `json:"expected_version"`
			Body            string                      `json:"body"`
			References      []docscollections.Reference `json:"references"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		body := strings.TrimSpace(in.Body)
		if body == "" || len(body) > 200000 {
			writeAPIError(w, 422, "documentation_draft_invalid", "draft body is required and bounded")
			return
		}
		current, e := docs.GetTask(r.PathValue("id"), r.PathValue("taskID"))
		if e != nil || validateReferences(r.PathValue("id"), current, in.References) != nil {
			writeAPIError(w, 422, "documentation_draft_invalid", "references must resolve at the task's exact revision")
			return
		}
		v, e := docs.UpdateTask(r.PathValue("id"), r.PathValue("taskID"), in.ExpectedVersion, func(v *docscollections.Task) error {
			rendered := "<article><pre>" + html.EscapeString(body) + "</pre></article>"
			v.Drafts = append(v.Drafts, docscollections.DraftRevision{ID: documentationRandomID(), Version: len(v.Drafts) + 1, Body: body, RenderedHTML: rendered, AuthorID: actor.UserID, References: in.References, CreatedAt: time.Now().UTC()})
			return nil
		})
		if errors.Is(e, docscollections.ErrConflict) {
			writeAPIError(w, 409, "documentation_task_changed", "documentation task changed")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "documentation_draft_invalid", "references must cite the task's exact revision")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/documentation-tasks/{taskID}/entries", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := participant(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                         `json:"expected_version"`
			Kind            string                      `json:"kind"`
			Body            string                      `json:"body"`
			AgentID         string                      `json:"agent_id"`
			Uncertain       bool                        `json:"uncertain"`
			References      []docscollections.Reference `json:"references"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		current, e := docs.GetTask(r.PathValue("id"), r.PathValue("taskID"))
		if e != nil || validateReferences(r.PathValue("id"), current, in.References) != nil {
			writeAPIError(w, 422, "documentation_entry_invalid", "references must resolve at the task's exact revision")
			return
		}
		v, e := docs.UpdateTask(r.PathValue("id"), r.PathValue("taskID"), in.ExpectedVersion, func(v *docscollections.Task) error {
			if !map[string]bool{"discussion": true, "suggestion": true, "agent_assistance": true}[in.Kind] || strings.TrimSpace(in.Body) == "" || len(in.Body) > 10000 {
				return docscollections.ErrInvalid
			}
			if in.Kind == "agent_assistance" && (actor.AgentID == "" || len(in.References) == 0) {
				return docscollections.ErrInvalid
			}
			draft := len(v.Drafts)
			v.Entries = append(v.Entries, docscollections.TaskEntry{ID: documentationRandomID(), Kind: in.Kind, Body: strings.TrimSpace(in.Body), ActorID: actor.UserID, AgentID: actor.AgentID, DraftVersion: draft, References: in.References, Uncertain: in.Uncertain, CreatedAt: time.Now().UTC()})
			return nil
		})
		if errors.Is(e, docscollections.ErrConflict) {
			writeAPIError(w, 409, "documentation_task_changed", "documentation task changed")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "documentation_entry_invalid", "agent assistance requires identity and revision-grounded sources")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /repositories/{id}/documentation", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorize(w, r)
		if !ok {
			return
		}
		items, err := docs.Collections(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "documentation_read_failed", "documentation collections could not be read")
			return
		}
		out := []docscollections.Revision{}
		for _, v := range items {
			if visible(r.PathValue("id"), v, actor, authenticated) {
				out = append(out, present(r.PathValue("id"), v))
			}
		}
		writeJSON(w, 200, map[string]any{"collections": out})
	})
	mux.HandleFunc("GET /repositories/{id}/documentation/{collectionID}", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorize(w, r)
		if !ok {
			return
		}
		v, err := docs.Current(r.PathValue("id"), r.PathValue("collectionID"))
		if err != nil || !visible(r.PathValue("id"), v, actor, authenticated) {
			writeAPIError(w, 404, "documentation_not_found", "documentation collection not found")
			return
		}
		history, err := docs.List(r.PathValue("id"), r.PathValue("collectionID"))
		if err != nil {
			writeAPIError(w, 500, "documentation_read_failed", "documentation collection could not be read")
			return
		}
		visibleHistory := []docscollections.Revision{}
		for _, revision := range history {
			if visible(r.PathValue("id"), revision, actor, authenticated) {
				visibleHistory = append(visibleHistory, revision)
			}
		}
		writeJSON(w, 200, map[string]any{"collection": present(r.PathValue("id"), v), "history": visibleHistory})
	})
	mux.HandleFunc("GET /repositories/{id}/documentation/{collectionID}/pages/{slug...}", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorize(w, r)
		if !ok {
			return
		}
		selected, history, err := selectRevision(r.PathValue("id"), r.PathValue("collectionID"), r.URL.Query().Get("version"), actor, authenticated)
		if err != nil {
			writeAPIError(w, 404, "documentation_version_not_found", "documentation version is not published or visible")
			return
		}
		slug := r.PathValue("slug")
		for _, redirect := range selected.PublicationPolicy.Redirects {
			if redirect.From == slug {
				w.Header().Set("Location", "/repositories/"+r.PathValue("id")+"/documentation/"+selected.CollectionID+"/pages/"+redirect.To)
				w.WriteHeader(http.StatusPermanentRedirect)
				return
			}
		}
		var page docscollections.Page
		found := false
		for _, p := range selected.Pages {
			if p.Slug == slug {
				page = p
				found = true
				break
			}
		}
		if !found {
			writeAPIError(w, 404, "documentation_page_not_found", "documentation page not found")
			return
		}
		gr, e := git.Open(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "documentation_source_unavailable", "published source is unavailable")
			return
		}
		object, e := gr.ReadObject(storage.ObjectID(page.SourceObjectID))
		if e != nil {
			writeAPIError(w, 500, "documentation_source_unavailable", "published source is unavailable")
			return
		}
		archived := selected.ID != history[len(history)-1].ID
		writeJSON(w, 200, map[string]any{"collection": selected, "page": page, "body": string(object.Content), "archived": archived, "canonical_path": "/repositories/" + r.PathValue("id") + "/documentation/" + selected.CollectionID + "/pages/" + page.Slug})
	})
	mux.HandleFunc("GET /repositories/{id}/documentation/search", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorize(w, r)
		if !ok {
			return
		}
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		items, _ := docs.Collections(r.PathValue("id"))
		results := []map[string]any{}
		for _, v := range items {
			if !visible(r.PathValue("id"), v, actor, authenticated) {
				continue
			}
			for _, p := range v.Pages {
				if q == "" || strings.Contains(strings.ToLower(p.Title+" "+p.Path), q) {
					results = append(results, map[string]any{"collection_id": v.CollectionID, "collection": v.Name, "version": v.Version, "page": p, "url": "/repositories/" + v.RepositoryID + "/documentation/" + v.CollectionID + "/pages/" + p.Slug})
				}
			}
		}
		writeJSON(w, 200, map[string]any{"query": q, "results": results})
	})
	mux.HandleFunc("POST /repositories/{id}/documentation/{collectionID}/feedback", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorize(w, r)
		if !ok {
			return
		}
		if !authenticated {
			writeAPIError(w, 401, "authentication_required", "sign in to report documentation outcomes")
			return
		}
		selected, _, e := selectRevision(r.PathValue("id"), r.PathValue("collectionID"), r.URL.Query().Get("version"), actor, true)
		if e != nil {
			writeAPIError(w, 404, "documentation_version_not_found", "documentation version is not published or visible")
			return
		}
		var in docscollections.Feedback
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.RepositoryID = r.PathValue("id")
		in.CollectionID = selected.CollectionID
		in.RevisionID = selected.ID
		in.ReporterID = actor.UserID
		if in.Kind != "search_miss" {
			found := false
			for _, page := range selected.Pages {
				if page.Slug == in.PageSlug {
					found = true
					break
				}
			}
			if !found {
				writeAPIError(w, 422, "invalid_documentation_feedback", "page does not exist in the selected publication")
				return
			}
		}
		created, e := docs.AddFeedback(in)
		if e != nil {
			writeAPIError(w, 422, "invalid_documentation_feedback", "kind, bounded detail, page context, and permitted evidence are required")
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/documentation/{collectionID}/feedback", func(w http.ResponseWriter, r *http.Request) {
		_, owner, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "owner_required", "only repository maintainers can triage reader outcomes")
			return
		}
		items, e := docs.Feedback(r.PathValue("id"), r.PathValue("collectionID"))
		if e != nil {
			writeAPIError(w, 500, "documentation_feedback_unavailable", "reader outcomes are unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"feedback": items})
	})
	mux.HandleFunc("POST /repositories/{id}/documentation/{collectionID}/feedback/{feedbackID}/triage", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "owner_required", "only repository maintainers can triage reader outcomes")
			return
		}
		var in struct {
			Kind       string `json:"kind"`
			ResourceID string `json:"resource_id"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		validTarget := false
		switch in.Kind {
		case "issue":
			if issueStore != nil {
				_, e := issueStore.Get(r.PathValue("id"), in.ResourceID)
				validTarget = e == nil
			}
		case "proposal":
			if proposalStore != nil {
				_, e := proposalStore.Get(r.PathValue("id"), in.ResourceID)
				validTarget = e == nil
			}
		case "documentation_task":
			_, e := docs.GetTask(r.PathValue("id"), in.ResourceID)
			validTarget = e == nil
		}
		if !validTarget {
			writeAPIError(w, 422, "invalid_documentation_triage", "link an existing issue, proposal, or documentation task in this repository")
			return
		}
		v, e := docs.TriageFeedback(r.PathValue("id"), r.PathValue("collectionID"), r.PathValue("feedbackID"), actor.UserID, in.Kind, in.ResourceID)
		if errors.Is(e, docscollections.ErrConflict) {
			writeAPIError(w, 409, "documentation_feedback_triaged", "reader outcome was already triaged")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "invalid_documentation_triage", "link an issue, proposal, or documentation task")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("PUT /repositories/{id}/documentation/{collectionID}", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "owner_required", "only the repository owner can publish documentation collections")
			return
		}
		var input struct {
			ExpectedVersion int                      `json:"expected_version"`
			Collection      docscollections.Revision `json:"collection"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		repo, _ := repos.GetByID(r.PathValue("id"))
		gr, err := git.Open(repo.ID)
		if err != nil {
			writeAPIError(w, 422, "invalid_documentation_source", "repository source is unavailable")
			return
		}
		sourceRef := input.Collection.SourceRef
		if sourceRef == "" {
			sourceRef = repo.DefaultBranch
		}
		ref, err := gr.ReadReference("refs/heads/" + sourceRef)
		if err != nil {
			writeAPIError(w, 422, "invalid_documentation_source", "source ref does not exist")
			return
		}
		commit, err := gr.ReadCommit(storage.ObjectID(ref.Target))
		if err != nil {
			writeAPIError(w, 422, "invalid_documentation_source", "source revision is unreadable")
			return
		}
		tree, err := gr.WalkTree(commit.Tree)
		if err != nil {
			writeAPIError(w, 422, "invalid_documentation_source", "source tree is unreadable")
			return
		}
		root := strings.TrimSuffix(input.Collection.RootPath, "/")
		pages := []docscollections.Page{}
		for _, entry := range tree {
			if entry.Type != storage.BlobObject || entry.Path != root && !strings.HasPrefix(entry.Path, root+"/") {
				continue
			}
			ext := strings.ToLower(path.Ext(entry.Path))
			if ext != ".md" && ext != ".mdx" && ext != ".adoc" {
				continue
			}
			object, err := gr.ReadObject(entry.ID)
			if err != nil {
				continue
			}
			rel := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(entry.Path, root), "/"), ext)
			slug := strings.TrimSuffix(rel, "/index")
			if slug == "index" || slug == "" {
				slug = "index"
			}
			title := titleFromDocument(object.Content, path.Base(rel))
			sum := sha256.Sum256(object.Content)
			pages = append(pages, docscollections.Page{Path: entry.Path, Slug: slug, Title: title, NavigationTitle: title, Position: len(pages), SourceObjectID: string(entry.ID), SourceSHA256: fmt.Sprintf("%x", sum), Authors: commitAuthors(commit)})
		}
		sort.Slice(pages, func(i, j int) bool { return pages[i].Path < pages[j].Path })
		if len(pages) == 0 {
			writeAPIError(w, 422, "invalid_documentation_source", "root path contains no supported documentation pages")
			return
		}
		links := map[string][]docscollections.Link{}
		for _, p := range input.Collection.Pages {
			links[p.Path] = p.Links
		}
		for i := range pages {
			pages[i].Position = i
			pages[i].Links = links[pages[i].Path]
		}
		input.Collection.RepositoryID = repo.ID
		input.Collection.CollectionID = r.PathValue("collectionID")
		if input.Collection.CollectionID == "new" {
			input.Collection.CollectionID = ""
		}
		input.Collection.PublishedBy = actor.UserID
		input.Collection.SourceRef = sourceRef
		input.Collection.SourceRevision = ref.Target
		for i := range input.Collection.SupportedVersions {
			mapping := &input.Collection.SupportedVersions[i]
			if mapping.ReleaseID == "" && mapping.Revision == "" {
				mappingRef, mappingErr := gr.ReadReference("refs/heads/" + mapping.SourceRef)
				if mappingErr != nil {
					writeAPIError(w, 422, "invalid_documentation_version_source", "supported version source ref does not exist")
					return
				}
				if _, mappingErr = gr.ReadCommit(storage.ObjectID(mappingRef.Target)); mappingErr != nil {
					writeAPIError(w, 422, "invalid_documentation_version_source", "supported version source revision is unreadable")
					return
				}
				mapping.Revision = mappingRef.Target
			}
		}
		input.Collection.Pages = pages
		input.Collection.ID = ""
		input.Collection.Version = 0
		created, err := docs.Publish(input.Collection, input.ExpectedVersion)
		if errors.Is(err, docscollections.ErrConflict) {
			writeAPIError(w, 409, "documentation_changed", "documentation collection version changed")
			return
		}
		if errors.Is(err, docscollections.ErrInvalid) {
			writeAPIError(w, 422, "invalid_documentation", "collection policy, version mappings, and source configuration are required")
			return
		}
		if err != nil && !errors.Is(err, docscollections.ErrDurabilityUncertain) {
			writeAPIError(w, 500, "documentation_write_failed", "documentation collection could not be published")
			return
		}
		status := http.StatusCreated
		if errors.Is(err, docscollections.ErrDurabilityUncertain) {
			status = http.StatusAccepted
			w.Header().Set("Vivarium-Durability", "uncertain")
		}
		w.Header().Set("Location", "/repositories/"+repo.ID+"/documentation/"+created.CollectionID)
		writeJSON(w, status, present(repo.ID, created))
	})
}

func titleFromDocument(content []byte, fallback string) string {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return strings.ReplaceAll(fallback, "-", " ")
}
func commitAuthors(commit storage.Commit) []string {
	out := []string{}
	for _, h := range commit.Headers {
		if h.Name == "author" {
			value := h.Value
			if at := strings.LastIndex(value, " <"); at > 0 {
				value = value[:at]
			}
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		out = append(out, "Unknown Git author")
	}
	return out
}
