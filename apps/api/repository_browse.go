package main

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

const maxBlobPreviewBytes = 512 << 10

type browseCommit struct {
	ID         string     `json:"id"`
	TreeID     string     `json:"tree_id"`
	ParentIDs  []string   `json:"parent_ids"`
	Message    string     `json:"message"`
	Author     string     `json:"author"`
	AuthoredAt *time.Time `json:"authored_at"`
}

func registerRepositoryBrowseRoutes(mux *http.ServeMux, gitStore *storage.Store, catalog *repositories.Store, credentials *auth.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request) (*storage.Repository, bool) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return nil, false
		}
		repo, err := gitStore.Open(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "internal_error", "repository storage unavailable")
			return nil, false
		}
		return repo, true
	}
	mux.HandleFunc("GET /repositories/{id}/branches", func(w http.ResponseWriter, r *http.Request) {
		repo, ok := authorize(w, r)
		if !ok {
			return
		}
		refs, err := repo.ListReferences()
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		branches := make([]map[string]string, 0)
		for _, ref := range refs {
			name, found := strings.CutPrefix(ref.Name, "refs/heads/")
			if !found || ref.Symbolic {
				continue
			}
			if _, err := repo.ReadCommit(storage.ObjectID(ref.Target)); err != nil {
				writeBrowseError(w, err)
				return
			}
			branches = append(branches, map[string]string{"name": name, "commit_id": ref.Target})
		}
		writeJSON(w, 200, map[string]any{"branches": branches})
	})
	mux.HandleFunc("GET /repositories/{id}/commits", func(w http.ResponseWriter, r *http.Request) {
		repo, ok := authorize(w, r)
		if !ok {
			return
		}
		id, err := resolveRevision(repo, r.URL.Query().Get("ref"))
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		limit, after, ok := paginationParameters(r)
		if !ok {
			writeAPIError(w, http.StatusBadRequest, "invalid_pagination", "limit or after is invalid")
			return
		}
		commits, nextID, found, err := repo.ListCommitAncestryPage(id, storage.ObjectID(after), limit)
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		if !found {
			writeAPIError(w, http.StatusBadRequest, "invalid_pagination", "limit or after is invalid")
			return
		}
		result := make([]browseCommit, 0, len(commits))
		for _, commit := range commits {
			result = append(result, presentCommit(commit))
		}
		var next *string
		if nextID != nil {
			value := string(*nextID)
			next = &value
		}
		writeJSON(w, 200, map[string]any{"revision": id, "commits": result, "next_cursor": next})
	})
	mux.HandleFunc("GET /repositories/{id}/tree", func(w http.ResponseWriter, r *http.Request) {
		repo, ok := authorize(w, r)
		if !ok {
			return
		}
		id, err := resolveRevision(repo, r.URL.Query().Get("ref"))
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		commit, err := repo.ReadCommit(id)
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		treeID, err := resolveTree(repo, commit.Tree, r.URL.Query().Get("path"))
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		entries, err := repo.ReadTree(treeID)
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"revision": id, "path": r.URL.Query().Get("path"), "entries": entries})
	})
	mux.HandleFunc("GET /repositories/{id}/blob", func(w http.ResponseWriter, r *http.Request) {
		repo, ok := authorize(w, r)
		if !ok {
			return
		}
		id, err := resolveRevision(repo, r.URL.Query().Get("ref"))
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		commit, err := repo.ReadCommit(id)
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		entry, err := resolvePath(repo, commit.Tree, r.URL.Query().Get("path"))
		if err != nil || entry.Type != storage.BlobObject {
			writeBrowseError(w, storage.ErrObjectNotFound)
			return
		}
		object, err := repo.ReadObject(entry.ID)
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		binary := !utf8.Valid(object.Content) || bytes.IndexByte(object.Content, 0) >= 0
		content := ""
		truncated := false
		if !binary {
			preview := object.Content
			if len(preview) > maxBlobPreviewBytes {
				preview = preview[:maxBlobPreviewBytes]
				for len(preview) > 0 && !utf8.Valid(preview) {
					preview = preview[:len(preview)-1]
				}
				truncated = true
			}
			content = string(preview)
		}
		writeJSON(w, 200, map[string]any{"revision": id, "path": r.URL.Query().Get("path"), "size": object.Size, "is_binary": binary, "content": content, "truncated": truncated})
	})
}

func resolveRevision(repo *storage.Repository, ref string) (storage.ObjectID, error) {
	if ref == "" {
		return "", storage.ErrInvalidReference
	}
	if len(ref) == 40 && strings.IndexFunc(ref, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') }) == -1 {
		id := storage.ObjectID(ref)
		if _, err := repo.ReadCommit(id); err != nil {
			return "", err
		}
		return id, nil
	}
	reference, err := repo.ReadReference("refs/heads/" + ref)
	if err != nil || reference.Symbolic {
		return "", storage.ErrReferenceNotFound
	}
	id := storage.ObjectID(reference.Target)
	if _, err := repo.ReadCommit(id); err != nil {
		return "", err
	}
	return id, nil
}

func resolveTree(repo *storage.Repository, root storage.ObjectID, name string) (storage.ObjectID, error) {
	if name == "" {
		return root, nil
	}
	entry, err := resolvePath(repo, root, name)
	if err != nil || entry.Type != storage.TreeObject {
		return "", storage.ErrObjectNotFound
	}
	return entry.ID, nil
}

func resolvePath(repo *storage.Repository, root storage.ObjectID, name string) (storage.TreeEntry, error) {
	if name == "" || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return storage.TreeEntry{}, storage.ErrObjectNotFound
	}
	current := root
	parts := strings.Split(name, "/")
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			return storage.TreeEntry{}, storage.ErrObjectNotFound
		}
		entries, err := repo.ReadTree(current)
		if err != nil {
			return storage.TreeEntry{}, err
		}
		found := false
		for _, entry := range entries {
			if entry.Name == part {
				if index == len(parts)-1 {
					return entry, nil
				}
				if entry.Type != storage.TreeObject {
					return storage.TreeEntry{}, storage.ErrObjectNotFound
				}
				current = entry.ID
				found = true
				break
			}
		}
		if !found {
			return storage.TreeEntry{}, storage.ErrObjectNotFound
		}
	}
	return storage.TreeEntry{}, storage.ErrObjectNotFound
}

func presentCommit(commit storage.Commit) browseCommit {
	result := browseCommit{ID: string(commit.ID), TreeID: string(commit.Tree), Message: string(commit.Message), ParentIDs: make([]string, len(commit.Parents))}
	for i, parent := range commit.Parents {
		result.ParentIDs[i] = string(parent)
	}
	for _, header := range commit.Headers {
		if header.Name == "author" {
			result.Author = header.Value
			fields := strings.Fields(header.Value)
			if len(fields) >= 2 {
				if seconds, err := strconv.ParseInt(fields[len(fields)-2], 10, 64); err == nil {
					value := time.Unix(seconds, 0).UTC()
					result.AuthoredAt = &value
				}
			}
			break
		}
	}
	return result
}

func writeBrowseError(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrReferenceNotFound) || errors.Is(err, storage.ErrInvalidReference) {
		writeAPIError(w, 404, "revision_not_found", "revision not found")
	} else if errors.Is(err, storage.ErrObjectNotFound) {
		writeAPIError(w, 404, "path_not_found", "path not found")
	} else {
		writeAPIError(w, 500, "repository_unavailable", "repository content unavailable")
	}
}
