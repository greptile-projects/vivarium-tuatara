package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"html"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	docscollections "github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func documentationReviewPages(repository *storage.Repository, revision, root string) ([]docscollections.ReviewPage, error) {
	commit, err := repository.ReadCommit(storage.ObjectID(revision))
	if err != nil {
		return nil, err
	}
	entries, err := repository.WalkTree(commit.Tree)
	if err != nil {
		return nil, err
	}
	pages := []docscollections.ReviewPage{}
	for _, entry := range entries {
		if entry.Type != storage.BlobObject || !(entry.Path == root || strings.HasPrefix(entry.Path, root+"/")) || !(strings.HasSuffix(entry.Path, ".md") || strings.HasSuffix(entry.Path, ".mdx") || strings.HasSuffix(entry.Path, ".adoc")) {
			continue
		}
		blob, truncated, _, readErr := repository.ReadBlobPreview(entry.ID, 1<<20)
		if readErr != nil || truncated {
			return nil, errors.New("documentation page is unreadable or too large")
		}
		body := string(blob.Content)
		title := strings.TrimSuffix(path.Base(entry.Path), path.Ext(entry.Path))
		for _, line := range strings.Split(body, "\n") {
			if x := strings.TrimSpace(strings.TrimLeft(line, "#")); strings.HasPrefix(strings.TrimSpace(line), "#") && x != "" {
				title = x
				break
			}
		}
		hash := sha256.Sum256(blob.Content)
		slug := strings.TrimSuffix(strings.TrimPrefix(entry.Path, root+"/"), path.Ext(entry.Path))
		if entry.Path == root {
			slug = "index"
		}
		pages = append(pages, docscollections.ReviewPage{Path: entry.Path, Slug: slug, Title: title, SourceObjectID: string(entry.ID), SourceSHA256: hex.EncodeToString(hash[:]), RenderedHTML: "<article><pre>" + html.EscapeString(body) + "</pre></article>", Status: "current"})
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Path < pages[j].Path })
	return pages, nil
}

func registerDocumentationReviewRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, pulls *pullrequests.Store, docs *docscollections.Store, checks *checkruns.Store, credentials *auth.Store) {
	load := func(w http.ResponseWriter, r *http.Request) (auth.Credential, pullrequests.PullRequest, docscollections.PullReview, bool) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return actor, pullrequests.PullRequest{}, docscollections.PullReview{}, false
		}
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return actor, pull, docscollections.PullReview{}, false
		}
		review, err := docs.GetPullReview(pull.RepositoryID, pull.ID)
		if err != nil {
			writeAPIError(w, 404, "documentation_review_not_found", "documentation review not found")
			return actor, pull, review, false
		}
		repo, repoErr := repos.GetByID(pull.RepositoryID)
		collaborator, _ := repos.HasCollaborator(actor.UserID, pull.RepositoryID)
		_, invited := docscollections.ActiveInvitation(review, actor.UserID, time.Now().UTC())
		if repoErr != nil || actor.UserID != repo.OwnerID && !collaborator && !invited {
			writeAPIError(w, 404, "documentation_review_not_found", "documentation review not found")
			return actor, pull, review, false
		}
		return actor, pull, review, true
	}
	refresh := func(review docscollections.PullReview, pull pullrequests.PullRequest) docscollections.PullReview {
		if review.Revision == pull.SourceCommitID {
			return review
		}
		repository, err := git.Open(pull.RepositoryID)
		if err != nil {
			return review
		}
		current, err := documentationReviewPages(repository, pull.SourceCommitID, review.RootPath)
		if err != nil {
			return review
		}
		base, err := documentationReviewPages(repository, pull.TargetCommitID, review.RootPath)
		if err != nil {
			return review
		}
		baseHashes := map[string]string{}
		for _, p := range base {
			baseHashes[p.Path] = p.SourceSHA256
		}
		hashes := map[string]string{}
		for _, p := range current {
			hashes[p.Path] = p.SourceSHA256
		}
		persisted := map[string]bool{}
		for i := range review.Pages {
			persisted[review.Pages[i].Path] = true
			review.Pages[i].Status = "stale"
			if hashes[review.Pages[i].Path] == review.Pages[i].SourceSHA256 {
				review.Pages[i].Status = "current"
			}
		}
		// Synchronization may introduce a new changed page after the original
		// snapshot. Add it to the current projection without rewriting retained
		// evidence for the original content.
		for _, p := range current {
			if !persisted[p.Path] && baseHashes[p.Path] != p.SourceSHA256 {
				review.Pages = append(review.Pages, p)
			}
		}
		sort.Slice(review.Pages, func(i, j int) bool { return review.Pages[i].Path < review.Pages[j].Path })
		for i := range review.Entries {
			review.Entries[i].Stale = hashes[review.Entries[i].Path] != review.Entries[i].SourceSHA256
		}
		for i := range review.Decisions {
			review.Decisions[i].Stale = hashes[review.Decisions[i].Path] != review.Decisions[i].SourceSHA256
		}
		return review
	}
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/documentation-review", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		if pull.Status != pullrequests.Open {
			writeAPIError(w, 409, "documentation_review_closed", "pull request is not open")
			return
		}
		var in struct {
			CollectionID string `json:"collection_id"`
			RootPath     string `json:"root_path"`
			Gaps         []struct {
				Path   string `json:"path"`
				Area   string `json:"area"`
				Detail string `json:"detail"`
			} `json:"gaps"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_documentation_review", "review input is invalid")
			return
		}
		root := strings.Trim(strings.TrimSpace(in.RootPath), "/")
		affected := []string{}
		collectionID := in.CollectionID
		if collectionID != "" {
			collection, e := docs.Current(pull.RepositoryID, collectionID)
			if e != nil {
				writeAPIError(w, 422, "invalid_documentation_review", "collection is unavailable")
				return
			}
			root = collection.RootPath
			for _, v := range collection.SupportedVersions {
				affected = append(affected, v.Label)
			}
		}
		if root == "" {
			root = "docs"
		}
		repository, e := git.Open(pull.RepositoryID)
		if e != nil {
			writeAPIError(w, 422, "invalid_documentation_review", "repository source is unavailable")
			return
		}
		candidate, e := documentationReviewPages(repository, pull.SourceCommitID, root)
		if e != nil {
			writeAPIError(w, 422, "invalid_documentation_review", e.Error())
			return
		}
		base, _ := documentationReviewPages(repository, pull.TargetCommitID, root)
		baseBy := map[string]docscollections.ReviewPage{}
		for _, p := range base {
			baseBy[p.Path] = p
		}
		changed := []docscollections.ReviewPage{}
		nav := []docscollections.NavigationChange{}
		for _, p := range candidate {
			old, found := baseBy[p.Path]
			if !found || old.SourceSHA256 != p.SourceSHA256 {
				changed = append(changed, p)
			}
			if !found {
				nav = append(nav, docscollections.NavigationChange{Kind: "added", Path: p.Path, After: p.Title})
			} else if old.Title != p.Title {
				nav = append(nav, docscollections.NavigationChange{Kind: "renamed", Path: p.Path, Before: old.Title, After: p.Title})
			}
			delete(baseBy, p.Path)
		}
		for _, p := range baseBy {
			nav = append(nav, docscollections.NavigationChange{Kind: "removed", Path: p.Path, Before: p.Title})
		}
		if len(changed) == 0 && len(nav) == 0 {
			writeAPIError(w, 422, "invalid_documentation_review", "pull request has no documentation changes in the selected root")
			return
		}
		evidence := []docscollections.ReviewCheck{}
		if checks != nil {
			runs, _ := checks.List(pull.RepositoryID, pull.ID)
			for _, run := range runs {
				if run.CommitID != pull.SourceCommitID || run.Definition.Documentation == nil {
					continue
				}
				d := run.Definition.Documentation
				ids := []string{}
				for _, a := range run.Artifacts {
					ids = append(ids, a.ID)
				}
				evidence = append(evidence, docscollections.ReviewCheck{RunID: run.ID, Name: run.Definition.Name, State: run.State, Version: d.Version, Revision: d.Revision, Selectors: d.Selectors, ArtifactIDs: ids})
				if d.Version != "" {
					affected = append(affected, d.Version)
				}
			}
		}
		gaps := []docscollections.ReviewGap{}
		for _, g := range in.Gaps {
			if !docscollections.ValidReviewArea(g.Area) || strings.TrimSpace(g.Detail) == "" {
				writeAPIError(w, 422, "invalid_documentation_review", "gaps require a valid area and detail")
				return
			}
			gaps = append(gaps, docscollections.NewReviewGap(g.Path, g.Area, g.Detail, actor.UserID, time.Now()))
		}
		sort.Strings(affected)
		affected = uniqueReviewStrings(affected)
		created, e := docs.CreatePullReview(docscollections.PullReview{RepositoryID: pull.RepositoryID, PullRequestID: pull.ID, Revision: pull.SourceCommitID, BaseRevision: pull.TargetCommitID, CollectionID: collectionID, RootPath: root, Pages: changed, NavigationChanges: nav, Checks: evidence, AffectedVersions: affected, Gaps: gaps, Entries: []docscollections.ReviewEntry{}, Decisions: []docscollections.ReviewDecision{}, Invitations: []docscollections.ReviewInvitation{}})
		if errors.Is(e, docscollections.ErrConflict) {
			writeAPIError(w, 409, "documentation_review_exists", "documentation review already exists")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "documentation_review_write_failed", "documentation review could not be retained")
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/documentation-review", func(w http.ResponseWriter, r *http.Request) {
		_, pull, review, ok := load(w, r)
		if !ok {
			return
		}
		writeJSON(w, 200, refresh(review, pull))
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/documentation-review/entries", func(w http.ResponseWriter, r *http.Request) {
		actor, pull, review, ok := load(w, r)
		if !ok {
			return
		}
		var in struct {
			Kind string `json:"kind"`
			Path string `json:"path"`
			Area string `json:"area"`
			Body string `json:"body"`
		}
		if decodeJSON(r, &in) != nil || !reviewOneOf(in.Kind, "comment", "change_request", "preview_feedback") || docscollections.ValidateReviewMutation(in.Path, in.Area, in.Body) != nil {
			writeAPIError(w, 422, "invalid_documentation_review_entry", "exact page, area, kind, and bounded body are required")
			return
		}
		repository, _ := repos.GetByID(pull.RepositoryID)
		collaborator, _ := repos.HasCollaborator(actor.UserID, pull.RepositoryID)
		inv, invited := docscollections.ActiveInvitation(review, actor.UserID, time.Now())
		invitationOnly := actor.UserID != repository.OwnerID && !collaborator && invited
		if invitationOnly && (inv.Role == "view" || !reviewContains(inv.Areas, in.Area)) {
			writeAPIError(w, 403, "documentation_review_forbidden", "invitation does not permit this feedback area")
			return
		}
		sha := ""
		for _, p := range refresh(review, pull).Pages {
			if p.Path == in.Path && p.Status == "current" {
				sha = p.SourceSHA256
			}
		}
		if sha == "" {
			writeAPIError(w, 409, "documentation_review_stale", "page content changed; refresh the documentation review")
			return
		}
		updated, e := docs.UpdatePullReview(pull.RepositoryID, pull.ID, func(v *docscollections.PullReview) error {
			v.Entries = append(v.Entries, docscollections.NewReviewEntry(in.Kind, in.Path, in.Area, sha, in.Body, actor.UserID, time.Now()))
			return nil
		})
		if e != nil {
			writeAPIError(w, 500, "documentation_review_write_failed", "feedback could not be retained")
			return
		}
		writeJSON(w, 201, updated)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/documentation-review/decisions", func(w http.ResponseWriter, r *http.Request) {
		actor, pull, review, ok := load(w, r)
		if !ok {
			return
		}
		var in struct {
			Path    string `json:"path"`
			Area    string `json:"area"`
			Outcome string `json:"outcome"`
			Body    string `json:"body"`
		}
		if decodeJSON(r, &in) != nil || !reviewOneOf(in.Outcome, "approved", "changes_requested") || docscollections.ValidateReviewMutation(in.Path, in.Area, in.Outcome) != nil {
			writeAPIError(w, 422, "invalid_documentation_review_decision", "exact page, area, and decision are required")
			return
		}
		repository, _ := repos.GetByID(pull.RepositoryID)
		collaborator, _ := repos.HasCollaborator(actor.UserID, pull.RepositoryID)
		inv, invited := docscollections.ActiveInvitation(review, actor.UserID, time.Now())
		invitationOnly := actor.UserID != repository.OwnerID && !collaborator && invited
		if invitationOnly && (inv.Role != "review" || !reviewContains(inv.Areas, in.Area)) {
			writeAPIError(w, 403, "documentation_review_forbidden", "invitation does not permit decisions in this area")
			return
		}
		sha := ""
		for _, p := range refresh(review, pull).Pages {
			if p.Path == in.Path && p.Status == "current" {
				sha = p.SourceSHA256
			}
		}
		if sha == "" {
			writeAPIError(w, 409, "documentation_review_stale", "page content changed; refresh the documentation review")
			return
		}
		updated, e := docs.UpdatePullReview(pull.RepositoryID, pull.ID, func(v *docscollections.PullReview) error {
			v.Decisions = append(v.Decisions, docscollections.NewReviewDecision(in.Path, in.Area, sha, in.Outcome, in.Body, actor.UserID, time.Now()))
			return nil
		})
		if e != nil {
			writeAPIError(w, 500, "documentation_review_write_failed", "decision could not be retained")
			return
		}
		writeJSON(w, 201, updated)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/documentation-review/invitations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, review, ok := load(w, r)
		if !ok {
			return
		}
		repo, _ := repos.GetByID(review.RepositoryID)
		if actor.UserID != repo.OwnerID {
			writeAPIError(w, 403, "owner_required", "only the repository owner can invite documentation stakeholders")
			return
		}
		var in struct {
			UserID    string    `json:"user_id"`
			Role      string    `json:"role"`
			Areas     []string  `json:"areas"`
			ExpiresAt time.Time `json:"expires_at"`
		}
		if decodeJSON(r, &in) != nil || !reviewOneOf(in.Role, "view", "feedback", "review") || !in.ExpiresAt.After(time.Now()) || len(in.Areas) == 0 {
			writeAPIError(w, 422, "invalid_documentation_review_invitation", "user, role, areas, and future expiry are required")
			return
		}
		for _, a := range in.Areas {
			if !docscollections.ValidReviewArea(a) {
				writeAPIError(w, 422, "invalid_documentation_review_invitation", "invitation area is invalid")
				return
			}
		}
		updated, e := docs.UpdatePullReview(review.RepositoryID, review.PullRequestID, func(v *docscollections.PullReview) error {
			v.Invitations = append(v.Invitations, docscollections.NewReviewInvitation(in.UserID, in.Role, in.Areas, in.ExpiresAt, actor.UserID))
			return nil
		})
		if e != nil {
			writeAPIError(w, 500, "documentation_review_write_failed", "invitation could not be retained")
			return
		}
		writeJSON(w, 201, updated)
	})
}

func reviewContains(items []string, want string) bool {
	for _, v := range items {
		if v == want {
			return true
		}
	}
	return false
}
func reviewOneOf(value string, allowed ...string) bool { return reviewContains(allowed, value) }
func uniqueReviewStrings(items []string) []string {
	out := []string{}
	for _, v := range items {
		if v != "" && (len(out) == 0 || out[len(out)-1] != v) {
			out = append(out, v)
		}
	}
	return out
}
