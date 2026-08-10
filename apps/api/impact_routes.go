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
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
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

func registerImpactRoutes(mux *http.ServeMux, gitStore *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *impacts.Store, explanationStore *explanations.Store, proposalStore *proposals.Store, relationStore *relationships.Store, releaseStore *releases.Store, packageStore *packages.Store, deploymentStore *deployments.Store) {
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
		conclusionRevisionChanged := false
		switch in.Source.Kind {
		case "selected_code":
			if in.Source.Path == "" || in.Source.StartLine < 1 || in.Source.EndLine < in.Source.StartLine {
				err = impacts.ErrInvalid
			} else {
				query = selectedCodeQuery(repo.Path(), string(revision), in.Source)
			}
		case "investigation_conclusion":
			query = ""
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
						if entry.Revision != string(revision) {
							conclusionRevisionChanged = true
						} else {
							query = entry.Body
						}
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
		if conclusionRevisionChanged {
			writeAPIError(w, 409, "conclusion_revision_changed", "the requested ref no longer resolves to the conclusion revision")
			return
		}
		if err != nil || query == "" {
			writeAPIError(w, 400, "invalid_source", "the selected source is unavailable or contains no analyzable behavior")
			return
		}
		items, status, reason := deriveImpact(repo.Path(), r.PathValue("id"), string(revision), query, catalog, relationStore, releaseStore, packageStore, deploymentStore, actor.UserID)
		assessment := impacts.Assessment{RepositoryID: r.PathValue("id"), Revision: string(revision), Ref: strings.TrimSpace(in.Ref), Title: in.Title, Source: in.Source, CreatedBy: actor.UserID, Items: items, AnalysisStatus: status, AnalysisReason: reason}
		assessment.ContextState = "current"
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
				v = withImpactContext(gitStore, catalog, v)
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
		writeJSON(w, 200, projectImpact(withImpactContext(gitStore, catalog, v), actor.UserID, catalog))
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
		if err != nil || !visibleRepository(target, actor.UserID, catalog) || !assessmentAffectsRepository(v, target.ID) {
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
		if _, _, allowed := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !allowed {
			return
		}
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
	mux.HandleFunc("POST /repositories/{id}/impact-assessments/{assessment_id}/implementation", func(w http.ResponseWriter, r *http.Request) {
		actor, v, ok := mutate(w, r)
		if !ok {
			return
		}
		if proposalStore == nil || !impacts.IsParticipant(v, actor.UserID) {
			writeAPIError(w, 404, "assessment_not_found", "impact assessment not found")
			return
		}
		var in struct {
			Version int      `json:"version"`
			Title   string   `json:"title"`
			Body    string   `json:"body"`
			ItemIDs []string `json:"item_ids"`
			Tasks   []struct {
				Title             string `json:"title"`
				Outcome           string `json:"outcome"`
				AssigneeType      string `json:"assignee_type"`
				AssigneeID        string `json:"assignee_id"`
				DependsOnPrevious bool   `json:"depends_on_previous"`
			} `json:"tasks"`
		}
		if decodeJSON(r, &in) != nil || len(in.ItemIDs) == 0 || len(in.Tasks) == 0 {
			writeAPIError(w, 400, "invalid_implementation", "current version, cited impact items, and ordered owned tasks are required")
			return
		}
		if in.Version != v.Version {
			if proposal, tasks, recovered := recoverImpactImplementation(proposalStore, v, in.Version, in.Title, in.Body, in.ItemIDs, in.Tasks); recovered {
				writeJSON(w, 200, map[string]any{"assessment": projectImpact(withImpactContext(gitStore, catalog, v), actor.UserID, catalog), "proposal": proposal, "tasks": tasks, "recovered": true})
				return
			}
			writeAPIError(w, 400, "invalid_implementation", "current version, cited impact items, and ordered owned tasks are required")
			return
		}
		if withImpactContext(gitStore, catalog, v).ContextState != "current" {
			writeAPIError(w, 409, "assessment_context_changed", "the selected ref moved; rerun the assessment instead of rewriting its evidence")
			return
		}
		selected := map[string]bool{}
		originItems := []proposals.ReasoningItem{}
		for _, id := range in.ItemIDs {
			selected[id] = true
		}
		for _, item := range v.Items {
			if selected[item.ID] {
				originItems = append(originItems, proposals.ReasoningItem{ID: item.ID, Kind: item.Kind, Summary: item.Summary, Status: item.Status})
				delete(selected, item.ID)
			}
		}
		if len(selected) != 0 {
			writeAPIError(w, 400, "invalid_implementation", "every cited item must belong to the frozen assessment")
			return
		}
		acks := []proposals.ReasoningAcknowledgement{}
		for _, value := range v.AcknowledgementRequests {
			acks = append(acks, proposals.ReasoningAcknowledgement{RequestID: value.ID, RepositoryID: value.RepositoryID, OwnerID: value.OwnerID, AcknowledgedBy: value.AcknowledgedBy, Note: value.Acknowledgement})
		}
		origin := proposals.ReasoningOrigin{AssessmentID: v.ID, AssessmentVersion: v.Version, Revision: v.Revision, SelectedItemIDs: append([]string(nil), in.ItemIDs...), Items: originItems, Acknowledgements: acks, AnalysisStatus: v.AnalysisStatus}
		if v.Source.Kind == "investigation_conclusion" {
			origin.ExplanationID, origin.ConclusionEntryID = v.Source.ExplanationID, v.Source.EntryID
		}
		tasks := make([]proposals.ImplementationTaskInput, 0, len(in.Tasks))
		participants := []string{actor.UserID}
		for _, item := range in.Tasks {
			tasks = append(tasks, proposals.ImplementationTaskInput{Title: item.Title, Outcome: item.Outcome, AssigneeType: item.AssigneeType, AssigneeID: item.AssigneeID, DependsOnPrevious: item.DependsOnPrevious})
			if item.AssigneeType == "human" {
				participants = append(participants, item.AssigneeID)
			}
		}
		var proposal proposals.Proposal
		var created []proposals.Task
		var next impacts.Assessment
		var createUncertain bool
		var linkErr error
		repository, openErr := gitStore.Open(v.RepositoryID)
		if openErr != nil {
			writeAPIError(w, 409, "assessment_context_changed", "the selected ref is no longer available")
			return
		}
		publish := func() error {
			var createErr error
			proposal, created, createErr = proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: v.RepositoryID, ActorID: actor.UserID, Title: in.Title, Body: in.Body, Origin: origin, Tasks: tasks})
			if createErr != nil && !errors.Is(createErr, proposals.ErrDurabilityUncertain) {
				return createErr
			}
			createUncertain = errors.Is(createErr, proposals.ErrDurabilityUncertain)
			ids := make([]string, len(created))
			for i := range created {
				ids[i] = created[i].ID
			}
			next, linkErr = store.LinkImplementation(v.ID, v.Version, actor.UserID, proposal.ID, ids)
			return nil
		}
		err := catalog.WithCurrentParticipants(participants, v.RepositoryID, func() error {
			selectedRef := v.Ref
			if selectedRef == "" {
				meta, metaErr := catalog.GetByID(v.RepositoryID)
				if metaErr != nil {
					return metaErr
				}
				selectedRef = meta.DefaultBranch
			}
			name := impactReferenceName(repository, selectedRef)
			if name == "" {
				return publish() // an exact object ID is immutable
			}
			return repository.WithReferenceTarget(name, v.Revision, publish)
		})
		if err != nil {
			if errors.Is(err, storage.ErrReferenceExists) || errors.Is(err, storage.ErrReferenceNotFound) {
				writeAPIError(w, 409, "assessment_context_changed", "the selected ref moved; rerun the assessment instead of rewriting its evidence")
				return
			}
			writeAPIError(w, 400, "invalid_implementation", "human owners must be current participants and the plan must be valid")
			return
		}
		if linkErr != nil {
			next, _ = store.Get(v.ID)
		}
		if createUncertain || linkErr != nil {
			recovery := "the implementation is linked; confirm it through the returned assessment or a fresh assessment read"
			if linkErr != nil {
				recovery = "reload the assessment; if implementation is absent, resubmit using the freshly read version"
			}
			writeJSON(w, 202, map[string]any{"assessment": projectImpact(next, actor.UserID, catalog), "proposal": proposal, "tasks": created, "recovery": recovery})
			return
		}
		writeJSON(w, 201, map[string]any{"assessment": projectImpact(next, actor.UserID, catalog), "proposal": proposal, "tasks": created})
	})
}

func recoverImpactImplementation(proposalStore *proposals.Store, assessment impacts.Assessment, requestedVersion int, title, body string, itemIDs []string, requested []struct {
	Title             string `json:"title"`
	Outcome           string `json:"outcome"`
	AssigneeType      string `json:"assignee_type"`
	AssigneeID        string `json:"assignee_id"`
	DependsOnPrevious bool   `json:"depends_on_previous"`
}) (proposals.Proposal, []proposals.Task, bool) {
	if assessment.Implementation == nil || requestedVersion+1 != assessment.Version || assessment.Implementation.ProposalID == "" {
		return proposals.Proposal{}, nil, false
	}
	proposal, err := proposalStore.Get(assessment.RepositoryID, assessment.Implementation.ProposalID)
	if err != nil || proposal.Reasoning == nil || proposal.Reasoning.AssessmentID != assessment.ID || proposal.Reasoning.AssessmentVersion != requestedVersion || proposal.Title != strings.TrimSpace(title) || proposal.Body != strings.TrimSpace(body) || !sameStrings(proposal.Reasoning.SelectedItemIDs, itemIDs) {
		return proposals.Proposal{}, nil, false
	}
	tasks, err := proposalStore.ListTasks(assessment.RepositoryID, proposal.ID)
	if err != nil || len(tasks) != len(requested) {
		return proposals.Proposal{}, nil, false
	}
	for index := range tasks {
		input, task := requested[index], tasks[index]
		if task.Title != strings.TrimSpace(input.Title) || task.Outcome != strings.TrimSpace(input.Outcome) || task.Assignment == nil || task.Assignment.AssigneeType != input.AssigneeType || (input.AssigneeType == "human" && task.Assignment.AssigneeID != input.AssigneeID) {
			return proposals.Proposal{}, nil, false
		}
		expectedDependencies := []string{}
		if input.DependsOnPrevious && index > 0 {
			expectedDependencies = []string{tasks[index-1].ID}
		}
		if !sameStrings(task.DependencyIDs, expectedDependencies) {
			return proposals.Proposal{}, nil, false
		}
	}
	return proposal, tasks, true
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
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
	incomplete := map[string]bool{}
	add := func(kind, summary, status string, e ...impacts.Evidence) {
		items = append(items, impacts.Item{ID: "derived-" + kind + "-" + string(rune(len(items)+97)), Kind: kind, Summary: summary, Status: status, Evidence: e, AddedBy: "vivarium-impact-agent-v1"})
	}
	files, treeErr := exec.Command("git", "--git-dir="+gitDir, "ls-tree", "-r", "--name-only", revision).Output()
	if treeErr != nil {
		incomplete["the revision file list could not be read"] = true
	}
	scanned := 0
	for _, path := range strings.Split(strings.TrimSpace(string(files)), "\n") {
		if scanned >= 300 || !sourcePath(path) {
			continue
		}
		body, err := exec.Command("git", "--git-dir="+gitDir, "show", revision+":"+path).Output()
		if err != nil {
			incomplete["one or more source files could not be read"] = true
			continue
		}
		if len(body) > 512<<10 {
			incomplete["one or more source files exceeded the 512 KiB analysis limit"] = true
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
		if scan.Err() != nil {
			incomplete["one or more source lines exceeded the lexical scanner limit"] = true
		}
		if len(items) >= 80 {
			incomplete["lexical analysis reached the 80-item result limit"] = true
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
	if scanned >= 300 {
		incomplete["bounded lexical analysis reached the 300-file limit"] = true
	}
	reasons := make([]string, 0, len(incomplete))
	for value := range incomplete {
		reasons = append(reasons, value)
	}
	sort.Strings(reasons)
	reason := strings.Join(reasons, "; ")
	if len(reasons) > 0 {
		status = "incomplete"
		reason += "; record remaining unknowns explicitly"
	}
	return items, status, reason
}

func assessmentAffectsRepository(v impacts.Assessment, repositoryID string) bool {
	for _, item := range v.Items {
		if item.Kind != "consumer" {
			continue
		}
		for _, evidence := range item.Evidence {
			if evidence.Kind == "consumer" && evidence.RepositoryID == repositoryID && evidence.OwnerID != "" {
				return true
			}
		}
	}
	return false
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
	if v.ContextState == "" {
		v.ContextState = "current"
	}
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

func withImpactContext(gitStore *storage.Store, catalog *repositories.Store, value impacts.Assessment) impacts.Assessment {
	value.ContextState = "changed"
	repo, err := gitStore.Open(value.RepositoryID)
	if err != nil {
		return value
	}
	ref := value.Ref
	if ref == "" {
		meta, metaErr := catalog.GetByID(value.RepositoryID)
		if metaErr != nil {
			return value
		}
		ref = meta.DefaultBranch
	}
	resolved, err := resolveRevision(repo, ref)
	if err == nil && string(resolved) == value.Revision {
		value.ContextState = "current"
	}
	return value
}

func impactReferenceName(repo *storage.Repository, ref string) string {
	ref = strings.TrimSpace(ref)
	if len(ref) == 40 && regexp.MustCompile(`^[0-9a-fA-F]{40}$`).MatchString(ref) {
		return ""
	}
	candidates := []string{ref, "refs/heads/" + ref, "refs/tags/" + ref}
	for _, name := range candidates {
		if name == "" {
			continue
		}
		value, err := repo.ReadReference(name)
		if err == nil {
			if value.Symbolic {
				return value.Target
			}
			return name
		}
	}
	return ref
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
