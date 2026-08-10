package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/explanations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

const explanationFileLimit = 300
const explanationByteLimit = 4 << 20

type explanationInput struct {
	Question string               `json:"question"`
	Ref      string               `json:"ref"`
	Context  explanations.Context `json:"context"`
}

func registerExplanationRoutes(mux *http.ServeMux, gitStore *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *explanations.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, incidentStore *incidents.Store, workspaceStore *workspaces.Store, checkStore *checkruns.Store, relationStore *relationships.Store) {
	mux.HandleFunc("POST /repositories/{id}/explanations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		if _, _, allowed := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !allowed {
			return
		}
		var input explanationInput
		if err := decodeJSON(r, &input); err != nil {
			writeAPIError(w, 400, "invalid_request", "question, ref, and context are required")
			return
		}
		input.Question = strings.TrimSpace(input.Question)
		if input.Context.Kind == "" {
			input.Context.Kind = "repository"
		}
		if input.Question == "" || len(input.Question) > 2000 {
			writeAPIError(w, 400, "invalid_question", "question must contain between 1 and 2000 characters")
			return
		}
		repo, err := gitStore.Open(r.PathValue("id"))
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		if input.Context.Kind == "workspace" && workspaceStore != nil {
			if !explanationVisibleTo(actor.UserID, explanations.Conversation{RepositoryID: r.PathValue("id"), Context: input.Context}, catalog, workspaceStore) {
				writeAPIError(w, 404, "context_not_found", "the selected context is not available in this repository")
				return
			}
		}
		revision, err := resolveExplanationContext(repo, r.PathValue("id"), input, proposalStore, pullStore, incidentStore, workspaceStore)
		if err != nil {
			writeAPIError(w, 404, "context_not_found", "the selected context is not available in this repository")
			return
		}
		if input.Context.Kind == "file" && exec.Command("git", "--git-dir="+repo.Path(), "cat-file", "-e", string(revision)+":"+input.Context.Path).Run() != nil {
			writeAPIError(w, 404, "context_not_found", "the selected context is not available in this repository")
			return
		}
		claims, status, reason := explanationClaims(repo.Path(), r.PathValue("id"), string(revision), input, pullStore, checkStore, relationStore)
		answer := composeExplanation(claims, status)
		var conversation explanations.Conversation
		err = catalog.WithCurrentReadAccess(actor.UserID, []string{r.PathValue("id")}, func() error {
			var createErr error
			conversation, createErr = store.Create(explanations.Conversation{RepositoryID: r.PathValue("id"), Revision: string(revision), Context: input.Context, Question: input.Question, AskedBy: actor.UserID, Answer: answer, Claims: claims, AnalysisStatus: status, AnalysisReason: reason})
			return createErr
		})
		if err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				writeAPIError(w, 404, "repository_not_found", "repository not found")
			} else {
				writeAPIError(w, 500, "explanation_unavailable", "the explanation could not be retained")
			}
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Location", "/repositories/"+r.PathValue("id")+"/explanations/"+conversation.ID)
		w.WriteHeader(http.StatusCreated)
		encoder := json.NewEncoder(w)
		flush, _ := w.(http.Flusher)
		_ = encoder.Encode(map[string]any{"event": "conversation", "conversation": map[string]any{"id": conversation.ID, "revision": conversation.Revision, "asked_by": conversation.AskedBy, "created_at": conversation.CreatedAt}})
		if flush != nil {
			flush.Flush()
		}
		for _, claim := range conversation.Claims {
			_ = encoder.Encode(map[string]any{"event": "claim", "claim": claim})
			if flush != nil {
				flush.Flush()
			}
		}
		_ = encoder.Encode(map[string]any{"event": "done", "conversation": conversation})
		if flush != nil {
			flush.Flush()
		}
	})

	mux.HandleFunc("GET /repositories/{id}/explanations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		if _, _, allowed := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !allowed {
			return
		}
		items, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "explanation_unavailable", "explanations could not be read")
			return
		}
		visible := items[:0]
		for _, item := range items {
			if explanationVisibleTo(actor.UserID, item, catalog, workspaceStore) {
				visible = append(visible, item)
			}
		}
		if err := catalog.WithCurrentReadAccess(actor.UserID, []string{r.PathValue("id")}, func() error { writeJSON(w, 200, map[string]any{"conversations": visible}); return nil }); err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
		}
	})
	mux.HandleFunc("GET /repositories/{id}/explanations/{explanation_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		if _, _, allowed := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !allowed {
			return
		}
		item, err := store.Get(r.PathValue("explanation_id"))
		if err != nil || item.RepositoryID != r.PathValue("id") || !explanationVisibleTo(actor.UserID, item, catalog, workspaceStore) {
			writeAPIError(w, 404, "explanation_not_found", "explanation not found")
			return
		}
		if err := catalog.WithCurrentReadAccess(actor.UserID, []string{item.RepositoryID}, func() error { writeJSON(w, 200, item); return nil }); err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
		}
	})
}

// explanationVisibleTo retains context-specific visibility on replay. A
// missing or moved workspace fails closed because its sharing boundary can no
// longer be established from the durable authority.
func explanationVisibleTo(actorID string, conversation explanations.Conversation, catalog *repositories.Store, workspaceStore *workspaces.Store) bool {
	if conversation.Context.Kind != "workspace" {
		return true
	}
	if workspaceStore == nil {
		return false
	}
	workspace, err := workspaceStore.Get(conversation.Context.ResourceID)
	if err != nil || workspace.RepositoryID != conversation.RepositoryID {
		return false
	}
	if workspace.Policy.Sharing != "private" {
		return true
	}
	meta, err := catalog.GetByID(conversation.RepositoryID)
	return err == nil && (actorID == workspace.CreatorID || actorID == meta.OwnerID)
}

func resolveExplanationContext(repo *storage.Repository, repositoryID string, input explanationInput, proposalStore *proposals.Store, pullStore *pullrequests.Store, incidentStore *incidents.Store, workspaceStore *workspaces.Store) (storage.ObjectID, error) {
	ref := input.Ref
	switch input.Context.Kind {
	case "repository", "file":
		if input.Context.Kind == "file" && (input.Context.Path == "" || strings.HasPrefix(input.Context.Path, "/") || strings.Contains(input.Context.Path, "..")) {
			return "", explanations.ErrInvalid
		}
	case "proposal":
		if proposalStore == nil {
			return "", explanations.ErrNotFound
		}
		if _, err := proposalStore.Get(repositoryID, input.Context.ResourceID); err != nil {
			return "", err
		}
	case "task":
		if proposalStore == nil {
			return "", explanations.ErrNotFound
		}
		parts := strings.Split(input.Context.ResourceID, ":")
		if len(parts) != 2 {
			return "", explanations.ErrInvalid
		}
		task, err := proposalStore.GetTask(repositoryID, parts[0], parts[1])
		if err != nil {
			return "", err
		}
		if ref == "" && task.Assignment != nil {
			ref = task.Assignment.Access.BaseRevision
		}
	case "pull_request":
		if pullStore == nil {
			return "", explanations.ErrNotFound
		}
		pull, err := pullStore.Get(repositoryID, input.Context.ResourceID)
		if err != nil {
			return "", err
		}
		ref = pull.SourceCommitID
	case "incident":
		if incidentStore == nil {
			return "", explanations.ErrNotFound
		}
		incident, err := incidentStore.Get(input.Context.ResourceID)
		if err != nil {
			return "", err
		}
		found := false
		for _, scope := range incident.Scopes {
			if scope.RepositoryID == repositoryID {
				found = true
			}
		}
		if !found {
			return "", explanations.ErrNotFound
		}
	case "workspace":
		if workspaceStore == nil {
			return "", explanations.ErrNotFound
		}
		workspace, err := workspaceStore.Get(input.Context.ResourceID)
		if err != nil || workspace.RepositoryID != repositoryID {
			return "", explanations.ErrNotFound
		}
		ref = workspace.CommitID
	default:
		return "", explanations.ErrInvalid
	}
	return resolveRevision(repo, ref)
}

type evidenceMatch struct {
	path, text, commit, summary string
	line                        int
}

func explanationClaims(gitDir, repositoryID, revision string, input explanationInput, pullStore *pullrequests.Store, checkStore *checkruns.Store, relationStore *relationships.Store) ([]explanations.Claim, string, string) {
	terms := questionTerms(input.Question)
	pathsOut, err := exec.Command("git", "--git-dir="+gitDir, "ls-tree", "-r", "--name-only", revision).Output()
	if err != nil {
		return []explanations.Claim{{Text: "Repository evidence could not be enumerated.", Basis: "uncertainty", Confidence: "low", Citations: []explanations.Citation{{Kind: "repository", Revision: revision, Label: "frozen repository revision"}}}}, "incomplete", "Git tree enumeration failed"
	}
	paths := strings.Split(strings.TrimSpace(string(pathsOut)), "\n")
	if input.Context.Kind == "file" {
		paths = []string{input.Context.Path}
	}
	matches := []evidenceMatch{}
	files, bytesRead, skipped := 0, 0, 0
	for _, path := range paths {
		if path == "" || !sourcePath(path) {
			continue
		}
		if files >= explanationFileLimit || bytesRead >= explanationByteLimit {
			skipped++
			continue
		}
		sizeOut, e := exec.Command("git", "--git-dir="+gitDir, "cat-file", "-s", revision+":"+path).Output()
		size, p := strconv.Atoi(strings.TrimSpace(string(sizeOut)))
		if e != nil || p != nil || size > explanationByteLimit-bytesRead {
			skipped++
			continue
		}
		body, e := exec.Command("git", "--git-dir="+gitDir, "show", revision+":"+path).Output()
		if e != nil || strings.IndexByte(string(body), 0) >= 0 {
			skipped++
			continue
		}
		files++
		bytesRead += len(body)
		scanner := bufio.NewScanner(strings.NewReader(string(body)))
		scanner.Buffer(make([]byte, 1024), 1024*1024)
		line := 0
		for scanner.Scan() {
			line++
			text := strings.TrimSpace(scanner.Text())
			if text == "" || !containsTerm(text, terms) {
				continue
			}
			commit, summary := blame(gitDir, revision, path, line)
			matches = append(matches, evidenceMatch{path: path, text: truncate(text, 300), line: line, commit: commit, summary: summary})
			if len(matches) >= 8 {
				break
			}
		}
		if len(matches) >= 8 {
			break
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return strings.HasSuffix(strings.ToLower(matches[i].path), ".md") && !strings.HasSuffix(strings.ToLower(matches[j].path), ".md")
	})
	claims := []explanations.Claim{}
	for _, match := range matches {
		kind := "source"
		if strings.HasSuffix(strings.ToLower(match.path), ".md") {
			kind = "documentation"
		}
		text := fmt.Sprintf("At %s:%d, the frozen %s says: %s", match.path, match.line, kind, match.text)
		claims = append(claims, explanations.Claim{Text: text, Basis: "evidence", Confidence: "high", Citations: []explanations.Citation{{Kind: kind, Revision: revision, Path: match.path, StartLine: match.line, EndLine: match.line, CommitID: match.commit, Label: match.summary}}})
	}
	if input.Context.Kind == "pull_request" && pullStore != nil && checkStore != nil {
		if pull, e := pullStore.Get(repositoryID, input.Context.ResourceID); e == nil {
			if runs, e := checkStore.List(repositoryID, pull.ID); e == nil {
				for _, run := range runs {
					if run.CommitID == revision {
						claims = append(claims, explanations.Claim{Text: fmt.Sprintf("Check %q is %s for this exact pull revision.", run.Definition.Name, run.State), Basis: "evidence", Confidence: "high", Citations: []explanations.Citation{{Kind: "check", Revision: revision, ResourceID: run.ID, Label: run.Definition.Name + " · " + run.State}}})
						if len(claims) >= 10 {
							break
						}
					}
				}
			}
		}
	}
	if relationStore != nil {
		if deps, e := relationStore.ListDependencies(repositoryID); e == nil {
			for _, dep := range deps {
				if dep.CommitID == revision {
					claims = append(claims, explanations.Claim{Text: fmt.Sprintf("This revision declares dependency %s with constraint %s.", dep.InterfaceName, dep.Constraint), Basis: "evidence", Confidence: "high", Citations: []explanations.Citation{{Kind: "dependency", Revision: revision, ResourceID: dep.ID, Label: dep.InterfaceName + " " + dep.Constraint}}})
				}
			}
		}
	}
	status, reason := "complete", ""
	if skipped > 0 || len(matches) >= 8 {
		status, reason = "incomplete", "bounded evidence collection skipped files or stopped at the claim limit"
	}
	if len(claims) == 0 {
		claims = append(claims, explanations.Claim{Text: "I could not locate evidence that directly answers the question in the permitted, frozen revision.", Basis: "uncertainty", Confidence: "low", Citations: []explanations.Citation{{Kind: "repository", Revision: revision, Label: "frozen repository revision"}}})
		status, reason = "incomplete", "no directly matching permitted evidence was found"
	}
	if status == "incomplete" {
		claims = append(claims, explanations.Claim{Text: "Additional relevant evidence may exist outside the bounded scan; this answer should not be treated as exhaustive.", Basis: "uncertainty", Confidence: "low", Citations: []explanations.Citation{{Kind: "repository", Revision: revision, Label: "bounded analysis boundary"}}})
	}
	return claims, status, reason
}

func composeExplanation(claims []explanations.Claim, status string) string {
	parts := make([]string, 0, len(claims))
	for _, claim := range claims {
		parts = append(parts, claim.Text)
	}
	if status == "incomplete" {
		return strings.Join(parts, " ")
	}
	return strings.Join(parts, " ")
}

var explanationWord = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_.-]{2,}`)

func questionTerms(question string) []string {
	stop := map[string]bool{"what": true, "where": true, "when": true, "which": true, "does": true, "this": true, "that": true, "with": true, "from": true, "about": true, "code": true, "explain": true, "how": true, "why": true, "the": true, "and": true, "are": true}
	out := []string{}
	for _, word := range explanationWord.FindAllString(strings.ToLower(question), -1) {
		if !stop[word] {
			out = append(out, word)
		}
	}
	if len(out) == 0 {
		out = []string{strings.ToLower(strings.TrimSpace(question))}
	}
	return out
}
func containsTerm(line string, terms []string) bool {
	lower := strings.ToLower(line)
	for _, term := range terms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}
