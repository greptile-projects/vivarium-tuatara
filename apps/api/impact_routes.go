package main

import (
	"bufio"
	"errors"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/explanations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/impacts"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type impactInput struct {
	Title  string         `json:"title"`
	Ref    string         `json:"ref"`
	Source impacts.Source `json:"source"`
	Query  string         `json:"query"`
}

func registerImpactRoutes(mux *http.ServeMux, gitStore *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *impacts.Store, explanationStore *explanations.Store, relationStore *relationships.Store, releaseStore *releases.Store, packageStore *packages.Store, deploymentStore *deployments.Store) {
	mux.HandleFunc("POST /repositories/{id}/impact-assessments", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		if _, _, allowed := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !allowed {
			return
		}
		var in impactInput
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Title) == "" {
			writeAPIError(w, 400, "invalid_request", "title, ref, and a selected-code, investigation-conclusion, or proposed-diff source are required")
			return
		}
		repo, err := gitStore.Open(r.PathValue("id"))
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		revision, err := resolveRevision(repo, in.Ref)
		if err != nil {
			writeBrowseError(w, err)
			return
		}
		query := strings.TrimSpace(in.Query)
		switch in.Source.Kind {
		case "selected_code":
			if in.Source.Path == "" || in.Source.StartLine < 1 || in.Source.EndLine < in.Source.StartLine {
				err = impacts.ErrInvalid
			} else {
				query = selectedCodeQuery(repo.Path(), string(revision), in.Source)
			}
		case "investigation_conclusion":
			if explanationStore == nil {
				err = impacts.ErrNotFound
				break
			}
			var conversation explanations.Conversation
			conversation, err = explanationStore.Get(in.Source.ExplanationID)
			if err == nil && (conversation.RepositoryID != r.PathValue("id") || !explanations.IsParticipant(conversation, actor.UserID)) {
				err = impacts.ErrNotFound
			}
			if err == nil {
				for _, entry := range conversation.Entries {
					if entry.ID == in.Source.EntryID && entry.Kind == "conclusion" {
						query = entry.Body
					}
				}
			}
		case "proposed_diff":
			if len(in.Source.Diff) > 20000 {
				err = impacts.ErrInvalid
			}
			if query == "" {
				query = diffQuery(in.Source.Diff)
			}
		default:
			err = impacts.ErrInvalid
		}
		if err != nil || query == "" {
			writeAPIError(w, 400, "invalid_source", "the selected source is unavailable or contains no analyzable behavior")
			return
		}
		items, status, reason := deriveImpact(repo.Path(), r.PathValue("id"), string(revision), query, catalog, relationStore, releaseStore, packageStore, deploymentStore, actor.UserID)
		assessment := impacts.Assessment{RepositoryID: r.PathValue("id"), Revision: string(revision), Title: in.Title, Source: in.Source, CreatedBy: actor.UserID, Items: items, AnalysisStatus: status, AnalysisReason: reason}
		err = catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error { var createErr error; assessment, createErr = store.Create(assessment); return createErr })
		if err != nil {
			writeAPIError(w, 403, "assessment_forbidden", "only a current repository participant may retain an assessment")
			return
		}
		writeJSON(w, 201, projectImpact(assessment, actor.UserID, catalog))
	})
	mux.HandleFunc("GET /repositories/{id}/impact-assessments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "assessment_unavailable", "assessments could not be read")
			return
		}
		visible := []impacts.Assessment{}
		for _, v := range values {
			if impacts.IsParticipant(v, actor.UserID) || impactOwnerRequest(v, actor.UserID) {
				visible = append(visible, projectImpact(v, actor.UserID, catalog))
			}
		}
		writeJSON(w, 200, map[string]any{"assessments": visible})
	})
	mux.HandleFunc("GET /repositories/{id}/impact-assessments/{assessment_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, err := store.Get(r.PathValue("assessment_id"))
		if err != nil || v.RepositoryID != r.PathValue("id") || (!impacts.IsParticipant(v, actor.UserID) && !impactOwnerRequest(v, actor.UserID)) {
			writeAPIError(w, 404, "assessment_not_found", "impact assessment not found")
			return
		}
		writeJSON(w, 200, projectImpact(v, actor.UserID, catalog))
	})
	mutate := func(w http.ResponseWriter, r *http.Request) (auth.Credential, impacts.Assessment, bool) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return actor, impacts.Assessment{}, false
		}
		v, err := store.Get(r.PathValue("assessment_id"))
		if err != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "assessment_not_found", "impact assessment not found")
			return actor, v, false
		}
		return actor, v, true
	}
	mux.HandleFunc("POST /repositories/{id}/impact-assessments/{assessment_id}/participants", func(w http.ResponseWriter, r *http.Request) {
		actor, v, ok := mutate(w, r)
		if !ok {
			return
		}
		var in struct {
			UserID  string `json:"user_id"`
			Version int    `json:"version"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "user_id and version are required")
			return
		}
		err := catalog.WithCurrentParticipants([]string{actor.UserID, in.UserID}, v.RepositoryID, func() error {
			var e error
			v, e = store.AddParticipant(v.ID, in.Version, actor.UserID, in.UserID)
			return e
		})
		writeImpactMutation(w, v, err, actor.UserID, catalog)
	})
	mux.HandleFunc("POST /repositories/{id}/impact-assessments/{assessment_id}/items", func(w http.ResponseWriter, r *http.Request) {
		actor, v, ok := mutate(w, r)
		if !ok {
			return
		}
		var in struct {
			Version int    `json:"version"`
			Kind    string `json:"kind"`
			Summary string `json:"summary"`
			Status  string `json:"status"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "impact item is invalid")
			return
		}
		var next impacts.Assessment
		err := catalog.WithCurrentParticipant(actor.UserID, v.RepositoryID, func() error {
			var updateErr error
			next, updateErr = store.AddItem(v.ID, in.Version, impacts.Item{Kind: in.Kind, Summary: in.Summary, Status: in.Status, AddedBy: actor.UserID})
			return updateErr
		})
		writeImpactMutation(w, next, err, actor.UserID, catalog)
	})
	mux.HandleFunc("POST /repositories/{id}/impact-assessments/{assessment_id}/acknowledgement-requests", func(w http.ResponseWriter, r *http.Request) {
		actor, v, ok := mutate(w, r)
		if !ok {
			return
		}
		var in struct {
			Version      int    `json:"version"`
			RepositoryID string `json:"repository_id"`
			Note         string `json:"note"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "repository_id and version are required")
			return
		}
		target, err := catalog.GetByID(in.RepositoryID)
		if err != nil || !visibleRepository(target, actor.UserID, catalog) {
			writeAPIError(w, 404, "owner_not_found", "affected owner not found")
			return
		}
		var next impacts.Assessment
		err = catalog.WithCurrentParticipant(actor.UserID, v.RepositoryID, func() error {
			var updateErr error
			next, updateErr = store.Request(v.ID, in.Version, impacts.AcknowledgementRequest{RepositoryID: target.ID, OwnerID: target.OwnerID, RequestedBy: actor.UserID, Note: in.Note})
			return updateErr
		})
		writeImpactMutation(w, next, err, actor.UserID, catalog)
	})
	mux.HandleFunc("POST /repositories/{id}/impact-assessments/{assessment_id}/acknowledgement-requests/{request_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, v, ok := mutate(w, r)
		if !ok {
			return
		}
		var in struct {
			Version int    `json:"version"`
			Note    string `json:"note"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "version is required")
			return
		}
		var request *impacts.AcknowledgementRequest
		for i := range v.AcknowledgementRequests {
			if v.AcknowledgementRequests[i].ID == r.PathValue("request_id") {
				request = &v.AcknowledgementRequests[i]
			}
		}
		if request == nil {
			writeAPIError(w, 404, "assessment_not_found", "assessment resource not found")
			return
		}
		target, targetErr := catalog.GetByID(request.RepositoryID)
		if targetErr != nil || target.OwnerID != actor.UserID || request.OwnerID != actor.UserID {
			writeAPIError(w, 404, "assessment_not_found", "assessment resource not found")
			return
		}
		next, err := store.Acknowledge(v.ID, in.Version, request.ID, actor.UserID, in.Note)
		writeImpactMutation(w, next, err, actor.UserID, catalog)
	})
}

func selectedCodeQuery(gitDir, revision string, source impacts.Source) string {
	body, err := exec.Command("git", "--git-dir="+gitDir, "show", revision+":"+source.Path).Output()
	if err != nil || len(body) > 1<<20 {
		return ""
	}
	scan := bufio.NewScanner(strings.NewReader(string(body)))
	line := 0
	parts := []string{}
	for scan.Scan() {
		line++
		if line >= source.StartLine && line <= source.EndLine {
			parts = append(parts, scan.Text())
		}
	}
	return strings.Join(parts, " ")
}
func diffQuery(diff string) string {
	re := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)
	seen := map[string]bool{}
	out := []string{}
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "-") {
			continue
		}
		for _, word := range re.FindAllString(line, -1) {
			if !seen[word] {
				seen[word] = true
				out = append(out, word)
				if len(out) == 8 {
					return strings.Join(out, " ")
				}
			}
		}
	}
	return strings.Join(out, " ")
}
func deriveImpact(gitDir, repositoryID, revision, query string, catalog *repositories.Store, relations *relationships.Store, releasesStore *releases.Store, packagesStore *packages.Store, deploymentsStore *deployments.Store, actor string) ([]impacts.Item, string, string) {
	tokens := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`).FindAllString(query, -1)
	if len(tokens) > 8 {
		tokens = tokens[:8]
	}
	items := []impacts.Item{}
	add := func(kind, summary, status string, e ...impacts.Evidence) {
		items = append(items, impacts.Item{ID: "derived-" + kind + "-" + string(rune(len(items)+97)), Kind: kind, Summary: summary, Status: status, Evidence: e, AddedBy: "vivarium-impact-agent-v1"})
	}
	files, _ := exec.Command("git", "--git-dir="+gitDir, "ls-tree", "-r", "--name-only", revision).Output()
	scanned := 0
	for _, path := range strings.Split(strings.TrimSpace(string(files)), "\n") {
		if scanned >= 300 || !sourcePath(path) {
			continue
		}
		body, err := exec.Command("git", "--git-dir="+gitDir, "show", revision+":"+path).Output()
		if err != nil || len(body) > 512<<10 {
			continue
		}
		scanned++
		scan := bufio.NewScanner(strings.NewReader(string(body)))
		line := 0
		for scan.Scan() {
			line++
			text := scan.Text()
			matched := ""
			for _, token := range tokens {
				if strings.Contains(strings.ToLower(text), strings.ToLower(token)) {
					matched = token
					break
				}
			}
			if matched != "" {
				kind := "reference"
				verification := "review call sites at the frozen revision"
				if isTestPath(path) {
					kind = "test"
					verification = "run or extend this test before implementation"
				}
				add(kind, strings.TrimSpace(text), map[bool]string{true: "verification_required", false: "candidate"}[kind == "test"], impacts.Evidence{Kind: kind, RepositoryID: repositoryID, Revision: revision, Path: path, Line: line, Label: matched, Verification: verification})
				if len(items) >= 80 {
					break
				}
			}
		}
		if len(items) >= 80 {
			break
		}
	}
	meta, _ := catalog.GetByID(repositoryID)
	add("owner", "Repository owner is affected", "candidate", impacts.Evidence{Kind: "owner", RepositoryID: repositoryID, Revision: revision, OwnerID: meta.OwnerID, Label: meta.Name})
	if relations != nil {
		ids, _ := relations.ListRepositoryIDs()
		for _, id := range ids {
			if !visibleID(id, actor, catalog) {
				continue
			}
			deps, _ := relations.ListDependencies(id)
			for _, d := range deps {
				if d.ProviderRepositoryID == repositoryID {
					target, _ := catalog.GetByID(id)
					add("consumer", "Consumer declares "+d.InterfaceName+" "+d.Constraint, "candidate", impacts.Evidence{Kind: "consumer", RepositoryID: id, Revision: d.CommitID, ResourceID: d.ID, OwnerID: target.OwnerID, Label: target.Name})
					add("interface", "Published interface "+d.InterfaceName+" may change", "unknown", impacts.Evidence{Kind: "interface", RepositoryID: repositoryID, Revision: revision, ResourceID: d.ID, Label: d.InterfaceName})
				}
			}
		}
	}
	if releasesStore != nil {
		values, _ := releasesStore.List(repositoryID)
		for _, v := range values {
			if v.CommitID == revision {
				add("release", "Release "+v.Version+" contains this exact revision", "candidate", impacts.Evidence{Kind: "release", RepositoryID: repositoryID, Revision: revision, ResourceID: v.ID, Label: v.Version})
			}
		}
	}
	if packagesStore != nil {
		values, _ := packagesStore.ListRepository(repositoryID)
		for _, v := range values {
			if v.SourceCommit == revision {
				add("package", "Package "+v.Name+"@"+v.Version+" is built from this revision", "candidate", impacts.Evidence{Kind: "package", RepositoryID: repositoryID, Revision: revision, ResourceID: v.ID, Label: v.Name + "@" + v.Version, State: v.Lifecycle})
			}
		}
	}
	if deploymentsStore != nil {
		promotions, _ := deploymentsStore.ListPromotions(repositoryID)
		for _, p := range promotions {
			if p.CommitID == revision {
				env, _ := deploymentsStore.GetEnvironment(repositoryID, p.EnvironmentID)
				add("environment", env.Name+" has "+p.State+" promotion evidence", "candidate", impacts.Evidence{Kind: "environment", RepositoryID: repositoryID, Revision: revision, ResourceID: env.ID, Label: env.Name, State: p.State})
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Kind < items[j].Kind })
	status := "complete"
	reason := ""
	if scanned >= 300 {
		status = "incomplete"
		reason = "bounded lexical analysis reached the 300-file limit; record remaining unknowns explicitly"
	}
	return items, status, reason
}
func visibleID(id, user string, c *repositories.Store) bool {
	v, e := c.GetByID(id)
	return e == nil && visibleRepository(v, user, c)
}
func visibleRepository(v repositories.Repository, user string, c *repositories.Store) bool {
	if v.Visibility == repositories.Public || v.OwnerID == user {
		return true
	}
	ok, _ := c.HasCollaborator(user, v.ID)
	return ok
}
func impactOwnerRequest(v impacts.Assessment, user string) bool {
	for _, x := range v.AcknowledgementRequests {
		if x.OwnerID == user {
			return true
		}
	}
	return false
}
func projectImpact(v impacts.Assessment, user string, c *repositories.Store) impacts.Assessment {
	if !impacts.IsParticipant(v, user) {
		// A requested owner may review the retained visible evidence and decision,
		// but an uncommitted proposed diff remains private to assessment participants.
		v.Source.Diff = ""
	}
	out := v.Items[:0]
	for _, item := range v.Items {
		evidence := item.Evidence[:0]
		for _, e := range item.Evidence {
			if visibleID(e.RepositoryID, user, c) {
				evidence = append(evidence, e)
			}
		}
		item.Evidence = evidence
		if len(evidence) > 0 || item.Kind == "risk" || item.Kind == "unknown" {
			out = append(out, item)
		}
	}
	v.Items = out
	requests := v.AcknowledgementRequests[:0]
	for _, x := range v.AcknowledgementRequests {
		if visibleID(x.RepositoryID, user, c) {
			requests = append(requests, x)
		}
	}
	v.AcknowledgementRequests = requests
	return v
}
func writeImpactMutation(w http.ResponseWriter, v impacts.Assessment, err error, user string, c *repositories.Store) {
	if err == nil {
		writeJSON(w, 200, projectImpact(v, user, c))
		return
	}
	if errors.Is(err, impacts.ErrConflict) {
		writeAPIError(w, 409, "assessment_changed", "the assessment changed; reload before updating it")
		return
	}
	if errors.Is(err, impacts.ErrNotFound) {
		writeAPIError(w, 404, "assessment_not_found", "assessment resource not found")
		return
	}
	writeAPIError(w, 400, "invalid_assessment", "the assessment update is invalid")
}
