package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
	productfeedback "github.com/greptile-projects/vivarium-tuatara/apps/api/feedback"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/roadmaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type designProposalInput struct {
	ExpectedVersion int                      `json:"expected_version"`
	OwnerIDs        []string                 `json:"owner_ids"`
	Revision        designproposals.Revision `json:"revision"`
}

func registerDesignProposalRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *designproposals.Store, issueStore *issues.Store, feedbackStore *productfeedback.Store, roadmapStore *roadmaps.Store, assessmentStore *accessibilityassessments.Store, pullStore *pullrequests.Store, proposalStore *proposals.Store) {
	mux.HandleFunc("GET /repositories/{id}/design-proposals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "design_proposals_unavailable", "design proposals could not be read")
			return
		}
		for i := range v {
			projectDesignEvidence(&v[i], actor.UserID, designReaderIsParticipant(catalog, r.PathValue("id"), actor.UserID), issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
			redactDesignArtifacts(&v[i], actor.UserID)
		}
		writeJSON(w, 200, map[string]any{"design_proposals": v})
	})
	mux.HandleFunc("GET /repositories/{id}/design-proposals/{proposal_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := store.Get(r.PathValue("id"), r.PathValue("proposal_id"))
		projectDesignEvidence(&v, actor.UserID, designReaderIsParticipant(catalog, r.PathValue("id"), actor.UserID), issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
		redactDesignArtifacts(&v, actor.UserID)
		writeDesignProposal(w, v, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/design-proposals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in designProposalInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete design proposal is required")
			return
		}
		normalizeDesignEvidence(r.PathValue("id"), &in.Revision, issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
		owners := append([]string(nil), in.OwnerIDs...)
		if len(owners) == 0 {
			owners = []string{actor.UserID}
		}
		var out designproposals.Proposal
		var e error
		e = catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error { out, e = store.Create(r.PathValue("id"), actor.UserID, owners, in.Revision); return e })
		redactDesignArtifacts(&out, actor.UserID)
		writeDesignProposal(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/design-proposals/{proposal_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in designProposalInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete successor revision is required")
			return
		}
		normalizeDesignEvidence(r.PathValue("id"), &in.Revision, issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
		out, e := store.Revise(r.PathValue("id"), r.PathValue("proposal_id"), actor.UserID, in.ExpectedVersion, in.Revision)
		redactDesignArtifacts(&out, actor.UserID)
		writeDesignProposal(w, out, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/design-proposals/{proposal_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in designproposals.Comment
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a revision-bound comment is required")
			return
		}
		current, getErr := store.Get(r.PathValue("id"), r.PathValue("proposal_id"))
		if getErr != nil || in.Revision < 1 || in.Revision > current.CurrentVersion {
			writeDesignProposal(w, current, designproposals.ErrInvalid, 0)
			return
		}
		source := current.Revisions[in.Revision-1].Source
		sourceAccessible := designSourceExists(r.PathValue("id"), source, issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
		for i := range in.Evidence {
			in.Evidence[i].Accessible = sourceAccessible && in.Evidence[i].Kind == source.Kind && in.Evidence[i].ResourceID == source.ResourceID
			if !in.Evidence[i].Accessible && in.Evidence[i].Gap == "" {
				in.Evidence[i].Gap = "citation visibility was not established; no asset content was copied"
			}
		}
		out, e := store.Comment(r.PathValue("id"), r.PathValue("proposal_id"), actor.UserID, in)
		redactDesignArtifacts(&out, actor.UserID)
		writeDesignProposal(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/design-proposals/{proposal_id}/acknowledgements", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in designproposals.Acknowledgement
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a current-revision owner acknowledgement is required")
			return
		}
		out, e := store.Acknowledge(r.PathValue("id"), r.PathValue("proposal_id"), actor.UserID, in)
		redactDesignArtifacts(&out, actor.UserID)
		writeDesignProposal(w, out, e, 201)
	})
	type taskInput struct {
		Title             string `json:"title"`
		AssigneeType      string `json:"assignee_type"`
		AssigneeID        string `json:"assignee_id"`
		DependsOnPrevious bool   `json:"depends_on_previous"`
	}
	mux.HandleFunc("POST /repositories/{id}/design-proposals/{proposal_id}/implementation", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		repository, repositoryErr := catalog.GetByID(r.PathValue("id"))
		if repositoryErr != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		var in struct {
			ExpectedVersion int         `json:"expected_version"`
			Title           string      `json:"title"`
			Body            string      `json:"body"`
			Tasks           []taskInput `json:"tasks"`
		}
		if decodeJSON(r, &in) != nil || len(in.Tasks) == 0 {
			writeAPIError(w, 400, "invalid_request", "accepted revision and ordered owned tasks are required")
			return
		}
		design, e := store.Get(repository.ID, r.PathValue("proposal_id"))
		if e != nil {
			writeDesignProposal(w, design, e, 0)
			return
		}
		if design.CurrentVersion != in.ExpectedVersion || design.Implementation != nil {
			writeAPIError(w, 409, "design_implementation_conflict", "the design revision or implementation changed")
			return
		}
		if !designRevisionAccepted(design) {
			writeAPIError(w, 409, "design_not_accepted", "every named design owner must acknowledge the current revision")
			return
		}
		revision := design.Revisions[in.ExpectedVersion-1]
		if len(revision.ComponentContracts) == 0 || len(revision.Breakpoints) == 0 || len(revision.AcceptanceCriteria) == 0 || !designAssetsAccountable(revision.Artifacts, actor.UserID) {
			writeAPIError(w, 400, "incomplete_design_handoff", "component contracts, breakpoints, acceptance criteria, and accountable assets are required")
			return
		}
		bare, e := git.Open(repository.ID)
		if e != nil {
			writeAPIError(w, 500, "design_implementation_unavailable", "repository could not be resolved")
			return
		}
		ref, e := bare.ReadReference("refs/heads/" + repository.DefaultBranch)
		if e != nil {
			writeAPIError(w, 409, "design_base_unavailable", "the default branch has no implementation base")
			return
		}
		items := designRequirementItems(revision)
		tasks := make([]proposals.ImplementationTaskInput, len(in.Tasks))
		participants := []string{actor.UserID}
		for i, t := range in.Tasks {
			outcome := "Implement the accepted design requirements without unstated behavior: " + strings.Join(revision.AcceptanceCriteria, "; ")
			tasks[i] = proposals.ImplementationTaskInput{Title: t.Title, Outcome: outcome, Risk: "Any deliberate deviation requires design-owner approval.", VerificationPlan: "Map changed code and rendered surfaces to every applicable design requirement.", AssigneeType: t.AssigneeType, AssigneeID: t.AssigneeID, DependsOnPrevious: t.DependsOnPrevious}
			if t.AssigneeType == "human" {
				participants = append(participants, t.AssigneeID)
			}
		}
		var ordinary proposals.Proposal
		var made []proposals.Task
		e = catalog.WithCurrentParticipants(participants, repository.ID, func() error {
			var createErr error
			ordinary, made, createErr = proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: repository.ID, ActorID: actor.UserID, Title: in.Title, Body: in.Body, Origin: proposals.ReasoningOrigin{DesignProposalID: design.ID, DesignProposalVersion: revision.Version, Revision: ref.Target, SelectedItemIDs: requirementIDs(items), Items: items, AnalysisStatus: "accepted_design_handoff"}, Tasks: tasks})
			return createErr
		})
		if e != nil {
			writeAPIError(w, 400, "invalid_design_implementation", "tasks must use current participants or ordinary agent ownership")
			return
		}
		ids := make([]string, len(made))
		for i := range made {
			ids[i] = made[i].ID
		}
		out, e := store.PublishImplementation(repository.ID, design.ID, actor.UserID, in.ExpectedVersion, designproposals.Implementation{DesignVersion: revision.Version, BaseRevision: ref.Target, ProposalID: ordinary.ID, TaskIDs: ids})
		if e != nil {
			writeDesignProposal(w, out, e, 0)
			return
		}
		redactDesignArtifacts(&out, actor.UserID)
		writeJSON(w, 201, map[string]any{"design_proposal": out, "proposal": ordinary, "tasks": made})
	})
	mux.HandleFunc("POST /repositories/{id}/design-proposals/{proposal_id}/implementation/reports", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Mapping   *designproposals.RequirementMapping `json:"mapping"`
			Deviation *designproposals.Deviation          `json:"deviation"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a requirement mapping or deviation is required")
			return
		}
		out, e := store.Report(r.PathValue("id"), r.PathValue("proposal_id"), actor.UserID, in.Mapping, in.Deviation)
		redactDesignArtifacts(&out, actor.UserID)
		writeDesignProposal(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/design-proposals/{proposal_id}/implementation/deviations/{deviation_id}/decision", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Status string `json:"status"`
			Note   string `json:"note"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an owner decision is required")
			return
		}
		out, e := store.DecideDeviation(r.PathValue("id"), r.PathValue("proposal_id"), r.PathValue("deviation_id"), actor.UserID, in.Status, in.Note)
		redactDesignArtifacts(&out, actor.UserID)
		writeDesignProposal(w, out, e, 201)
	})
}

func designAssetsAccountable(v []designproposals.Artifact, actor string) bool {
	for _, a := range v {
		if a.AuthorID == "" || a.License == "" || a.Source == "" || !stringContains(a.Audience, actor) {
			return false
		}
	}
	return true
}

func stringContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func designRevisionAccepted(v designproposals.Proposal) bool {
	for _, owner := range v.OwnerIDs {
		accepted := false
		for i := len(v.Acknowledgements) - 1; i >= 0; i-- {
			a := v.Acknowledgements[i]
			if a.OwnerID == owner && a.Revision == v.CurrentVersion {
				accepted = a.Status == "acknowledged"
				break
			}
		}
		if !accepted {
			return false
		}
	}
	return true
}
func designRequirementItems(r designproposals.Revision) []proposals.ReasoningItem {
	out := []proposals.ReasoningItem{}
	add := func(kind string, values []string) {
		for i, v := range values {
			out = append(out, proposals.ReasoningItem{ID: fmt.Sprintf("%s-%d", kind, i+1), Kind: kind, Summary: v, Status: "accepted"})
		}
	}
	add("prototype", artifactSummaries(r.Artifacts))
	add("component_contract", r.ComponentContracts)
	add("content", r.Content)
	add("breakpoint", r.Breakpoints)
	for _, s := range r.States {
		add("state", []string{s.Name + ": " + s.Description + " — " + s.Content})
	}
	add("acceptance_criterion", r.AcceptanceCriteria)
	return out
}
func artifactSummaries(v []designproposals.Artifact) []string {
	out := make([]string, len(v))
	for i, a := range v {
		// Ordinary proposals, tasks, histories, and workspaces are repository-readable.
		// Keep audience-controlled artifact payloads solely behind the design
		// proposal projection; task workers resolve this immutable ID there.
		out[i] = a.ID + ": " + a.Kind + " " + a.Title + " — audience-controlled prototype; resolve through design proposal (author " + a.AuthorID + ", license " + a.License + ", source " + a.Source + ", transformations: " + strings.Join(a.Transformations, ", ") + ")"
	}
	return out
}
func requirementIDs(v []proposals.ReasoningItem) []string {
	out := make([]string, len(v))
	for i := range v {
		out[i] = v[i].ID
	}
	return out
}

// Citation metadata is retained, but caller claims never make research or assets visible.
// An explicit gap lets collaborators request access without propagating the underlying content.
func normalizeDesignEvidence(repositoryID string, r *designproposals.Revision, issueStore *issues.Store, feedbackStore *productfeedback.Store, roadmapStore *roadmaps.Store, assessmentStore *accessibilityassessments.Store, pullStore *pullrequests.Store) {
	sourceAccessible := designSourceExists(repositoryID, r.Source, issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore)
	for i := range r.Evidence {
		r.Evidence[i].Accessible = sourceAccessible && r.Evidence[i].Kind == r.Source.Kind && r.Evidence[i].ResourceID == r.Source.ResourceID
		if !r.Evidence[i].Accessible && r.Evidence[i].Gap == "" {
			r.Evidence[i].Gap = "citation visibility was not established; no evidence content was copied"
		}
	}
}

func designSourceExists(repositoryID string, source designproposals.Source, issueStore *issues.Store, feedbackStore *productfeedback.Store, roadmapStore *roadmaps.Store, assessmentStore *accessibilityassessments.Store, pullStore *pullrequests.Store) bool {
	switch source.Kind {
	case "issue":
		if issueStore == nil {
			return false
		}
		v, err := issueStore.Get(repositoryID, source.ResourceID)
		return err == nil && v.RepositoryID == repositoryID
	case "feedback":
		if feedbackStore == nil {
			return false
		}
		v, err := feedbackStore.Get(source.ResourceID)
		return err == nil && v.RepositoryID == repositoryID
	case "roadmap_outcome":
		if roadmapStore == nil {
			return false
		}
		v, err := roadmapStore.Get(repositoryID)
		if err != nil {
			return false
		}
		for _, revision := range v.Revisions {
			for _, item := range revision.Items {
				if item.ID == source.ResourceID || item.OpportunityID == source.ResourceID {
					return true
				}
			}
		}
		return false
	case "accessibility_finding":
		if assessmentStore == nil {
			return false
		}
		values, err := assessmentStore.List(repositoryID, "", "")
		if err != nil {
			return false
		}
		for _, assessment := range values {
			for _, finding := range assessment.Findings {
				if finding.ID == source.ResourceID {
					return true
				}
			}
		}
		return false
	case "pull_request":
		if pullStore == nil {
			return false
		}
		v, err := pullStore.Get(repositoryID, source.ResourceID)
		return err == nil && v.RepositoryID == repositoryID
	default:
		return false
	}
}

func designReaderIsParticipant(catalog *repositories.Store, repositoryID, actor string) bool {
	if actor == "" {
		return false
	}
	repository, err := catalog.GetByID(repositoryID)
	if err != nil {
		return false
	}
	if repository.OwnerID == actor {
		return true
	}
	ok, _ := catalog.HasCollaborator(actor, repositoryID)
	return ok
}

func projectDesignEvidence(v *designproposals.Proposal, actor string, participant bool, issueStore *issues.Store, feedbackStore *productfeedback.Store, roadmapStore *roadmaps.Store, assessmentStore *accessibilityassessments.Store, pullStore *pullrequests.Store) {
	visible := func(source designproposals.Source) bool {
		if actor == "" || !designSourceExists(v.RepositoryID, source, issueStore, feedbackStore, roadmapStore, assessmentStore, pullStore) {
			return false
		}
		switch source.Kind {
		case "issue":
			return participant
		case "feedback":
			item, err := feedbackStore.Get(source.ResourceID)
			return err == nil && (item.Audience != "organization_private" || participant || item.ReporterID == actor)
		default:
			return true
		}
	}
	project := func(evidence *designproposals.Evidence, source designproposals.Source) {
		evidence.Accessible = visible(source) && evidence.Kind == source.Kind && evidence.ResourceID == source.ResourceID
		if !evidence.Accessible && evidence.Gap == "" {
			evidence.Gap = "citation is not visible to the current reader; no evidence content was copied"
		}
	}
	for ri := range v.Revisions {
		source := v.Revisions[ri].Source
		for ei := range v.Revisions[ri].Evidence {
			project(&v.Revisions[ri].Evidence[ei], source)
		}
	}
	for ci := range v.Comments {
		revision := v.Comments[ci].Revision
		if revision < 1 || revision > len(v.Revisions) {
			continue
		}
		source := v.Revisions[revision-1].Source
		for ei := range v.Comments[ci].Evidence {
			project(&v.Comments[ci].Evidence[ei], source)
		}
	}
}

func redactDesignArtifacts(v *designproposals.Proposal, actor string) {
	for ri := range v.Revisions {
		for ai := range v.Revisions[ri].Artifacts {
			a := &v.Revisions[ri].Artifacts[ai]
			allowed := false
			for _, id := range a.Audience {
				if actor != "" && id != "" && id == actor {
					allowed = true
					break
				}
			}
			if !allowed {
				a.Content = ""
				a.Interactions = nil
				a.Description = "Restricted artifact; request explicit audience access."
			}
		}
	}
}
func writeDesignProposal(w http.ResponseWriter, v designproposals.Proposal, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, designproposals.ErrNotFound):
		writeAPIError(w, 404, "design_proposal_not_found", "design proposal not found")
	case errors.Is(e, designproposals.ErrConflict):
		writeAPIError(w, 409, "design_proposal_conflict", "the design proposal changed; reload before publishing")
	case errors.Is(e, designproposals.ErrInvalid):
		writeAPIError(w, 400, "invalid_design_proposal", "define the source, goal, journeys, states, constraints, alternatives, success measures, affected components, and revision-bound contribution")
	case errors.Is(e, repositories.ErrInvalidCollaborator), errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 403, "design_proposal_forbidden", "owners must be current repository participants")
	default:
		log.Printf("design proposal storage: %v", e)
		writeAPIError(w, 500, "design_proposals_unavailable", "design proposal could not be persisted")
	}
}
