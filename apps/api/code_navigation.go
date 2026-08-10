package main

import (
	"bufio"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

const codeNavigationFileLimit = 500
const codeNavigationByteLimit = 8 << 20

type codeLocation struct {
	Kind          string `json:"kind"`
	Path          string `json:"path"`
	Line          int    `json:"line"`
	Preview       string `json:"preview"`
	CommitID      string `json:"commit_id,omitempty"`
	CommitSummary string `json:"commit_summary,omitempty"`
}

// Code navigation is deliberately computed from one immutable Git object. The
// bounded lexical analyzer is useful without pretending to be a language
// server; its coverage field makes omissions and unsupported syntax explicit.
func registerCodeNavigationRoutes(mux *http.ServeMux, gitStore *storage.Store, catalog *repositories.Store, credentials *auth.Store, relations *relationships.Store) {
	mux.HandleFunc("GET /repositories/{id}/code-navigation", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if query == "" || len(query) > 200 {
			writeAPIError(w, 400, "invalid_query", "q must contain between 1 and 200 characters")
			return
		}
		repo, err := gitStore.Open(r.PathValue("id"))
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		revision, err := resolveRevision(repo, r.URL.Query().Get("ref"))
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		pathsOut, err := exec.Command("git", "--git-dir="+repo.Path(), "ls-tree", "-r", "--name-only", string(revision)).Output()
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		paths := strings.Split(strings.TrimSpace(string(pathsOut)), "\n")
		matches, scanned, bytesRead, skipped := []codeLocation{}, 0, 0, 0
		needle := strings.ToLower(query)
		word := regexp.MustCompile(`(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(query) + `([^A-Za-z0-9_]|$)`)
		for _, path := range paths {
			if path == "" || scanned >= codeNavigationFileLimit || bytesRead >= codeNavigationByteLimit {
				skipped++
				continue
			}
			if !sourcePath(path) {
				continue
			}
			body, readErr := exec.Command("git", "--git-dir="+repo.Path(), "show", string(revision)+":"+path).Output()
			if readErr != nil || strings.IndexByte(string(body), 0) >= 0 {
				skipped++
				continue
			}
			scanned++
			bytesRead += len(body)
			testFile := isTestPath(path)
			scanner := bufio.NewScanner(strings.NewReader(string(body)))
			scanner.Buffer(make([]byte, 1024), 1024*1024)
			line := 0
			for scanner.Scan() {
				line++
				text := scanner.Text()
				if !strings.Contains(strings.ToLower(text), needle) {
					continue
				}
				kind := "reference"
				trimmed := strings.TrimSpace(text)
				if testFile {
					kind = "test"
				} else if isDefinition(trimmed, query) {
					kind = "definition"
				} else if word.MatchString(text) && strings.Contains(text, "(") {
					kind = "caller"
				}
				commitID, summary := blame(repo.Path(), string(revision), path, line)
				matches = append(matches, codeLocation{Kind: kind, Path: path, Line: line, Preview: truncate(trimmed, 240), CommitID: commitID, CommitSummary: summary})
				if len(matches) >= 250 {
					skipped += len(paths) - scanned
					break
				}
			}
			if len(matches) >= 250 {
				break
			}
		}
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].Kind != matches[j].Kind {
				return matches[i].Kind < matches[j].Kind
			}
			if matches[i].Path != matches[j].Path {
				return matches[i].Path < matches[j].Path
			}
			return matches[i].Line < matches[j].Line
		})
		meta, _ := catalog.GetByID(r.PathValue("id"))
		owners := []map[string]string{{"kind": "repository_owner", "id": meta.OwnerID}}
		if collaborators, listErr := catalog.ListCollaborators(meta.OwnerID, meta.ID); listErr == nil {
			for _, collaborator := range collaborators {
				owners = append(owners, map[string]string{"kind": "collaborator", "id": collaborator.UserID})
			}
		}
		dependencies := []relationships.Dependency{}
		if relations != nil {
			values, readErr := relations.ListDependencies(meta.ID)
			if readErr == nil {
				for _, value := range values {
					if value.CommitID != string(revision) {
						continue
					}
					provider, e := catalog.GetByID(value.ProviderRepositoryID)
					if e != nil {
						continue
					}
					allowed := provider.Visibility == repositories.Public
					if authenticated && !allowed {
						collaborator, _ := catalog.HasCollaborator(actor.UserID, provider.ID)
						allowed = actor.UserID == provider.OwnerID || collaborator
					}
					if allowed {
						dependencies = append(dependencies, value)
					}
				}
			}
		}
		complete := skipped == 0 && len(matches) < 250
		reason := ""
		if !complete {
			reason = "bounded analysis skipped files or stopped at the result limit"
		}
		writeJSON(w, 200, map[string]any{"repository_id": meta.ID, "revision": string(revision), "query": query, "results": matches, "ownership": owners, "dependencies": dependencies, "analysis": map[string]any{"status": map[bool]string{true: "complete", false: "incomplete"}[complete], "reason": reason, "files_scanned": scanned, "bytes_scanned": bytesRead, "result_limit": 250, "method": "revision-pinned lexical analysis"}})
	})
}

func sourcePath(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".kt", ".c", ".h", ".cpp", ".cs", ".rb", ".php", ".swift", ".sh", ".md", ".json", ".yaml", ".yml"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}
func isTestPath(path string) bool {
	p := strings.ToLower(path)
	return strings.Contains(p, "test") || strings.Contains(p, "spec") || strings.Contains(p, "__tests__")
}
func isDefinition(line, name string) bool {
	patterns := []string{"func " + name, "function " + name, "class " + name, "interface " + name, "type " + name, "const " + name, "let " + name, "var " + name, "def " + name, "fn " + name}
	for _, p := range patterns {
		if strings.Contains(line, p) {
			return true
		}
	}
	return false
}
func truncate(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return v[:n] + "…"
}
func blame(gitDir, revision, path string, line int) (string, string) {
	out, err := exec.Command("git", "--git-dir="+gitDir, "blame", "--porcelain", "-L", strconv.Itoa(line)+","+strconv.Itoa(line), revision, "--", path).Output()
	if err != nil {
		return "", ""
	}
	rows := strings.Split(string(out), "\n")
	id := ""
	if len(rows) > 0 {
		id = strings.Fields(rows[0])[0]
	}
	summary := ""
	for _, row := range rows {
		if strings.HasPrefix(row, "summary ") {
			summary = strings.TrimPrefix(row, "summary ")
			break
		}
	}
	return id, summary
}
