package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	docscollections "github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	productfeedback "github.com/greptile-projects/vivarium-tuatara/apps/api/feedback"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productexperiments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type feedbackCommentInput struct {
	Body string `json:"body"`
}

func registerFeedbackRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *productfeedback.Store, releaseStore *releases.Store, documentation *docscollections.Store, previewStore *previews.Store, issueStore *issues.Store, experiments *productexperiments.Store) {
	project := func(x productfeedback.Item, viewer string, participant bool) productfeedback.Item {
		reporterID := x.ReporterID
		identityAllowed := viewer == reporterID || (participant && x.IdentityVisibility == "maintainers") || x.IdentityVisibility == "audience"
		if !identityAllowed {
			x.ReporterID = ""
		}
		if x.ContactPreference != "direct" || (viewer != reporterID && !(participant && x.IdentityVisibility == "maintainers")) {
			x.Contact = ""
		}
		visible := x.Evidence[:0]
		for _, evidence := range x.Evidence {
			if viewer == reporterID || evidence.Visibility == "audience" || (participant && evidence.Visibility == "maintainers") {
				visible = append(visible, evidence)
			}
		}
		x.Evidence = visible
		for i := range x.Comments {
			if x.Comments[i].AuthorRole == "reporter" && !identityAllowed {
				x.Comments[i].AuthorID = ""
			}
		}
		for i := range x.History {
			if x.History[i].ActorID == reporterID && !identityAllowed {
				x.History[i].ActorID = ""
			}
		}
		return x
	}
	authorize := func(w http.ResponseWriter, r *http.Request) (auth.Credential, repositories.Repository, bool, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return actor, repositories.Repository{}, false, false
		}
		if actor.UserID == "" {
			writeAuthenticationRequired(w, false)
			return actor, repositories.Repository{}, false, false
		}
		repo, e := catalog.GetByID(r.PathValue("id"))
		if e != nil {
			return actor, repo, false, false
		}
		participant := actor.UserID == repo.OwnerID
		if !participant {
			participant, _ = catalog.HasCollaborator(actor.UserID, repo.ID)
		}
		return actor, repo, participant, true
	}
	validateLinks := func(repoID string, x productfeedback.Item) bool {
		for _, l := range x.Links {
			if l.Kind == "issue" {
				if issueStore == nil {
					return false
				}
				if _, e := issueStore.Get(repoID, l.ResourceID); e != nil {
					return false
				}
			} else {
				if experiments == nil {
					return false
				}
				v, e := experiments.Get(l.ResourceID)
				if e != nil || v.RepositoryID != repoID {
					return false
				}
			}
		}
		return true
	}
	mux.HandleFunc("POST /repositories/{id}/feedback", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, _, ok := authorize(w, r)
		if !ok {
			return
		}
		var in productfeedback.Item
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "complete feedback and consent preferences are required")
			return
		}
		in.RepositoryID = repo.ID
		in.OrganizationID = repo.OrganizationID
		if in.Audience == "organization_private" && repo.OrganizationID == "" {
			writeAPIError(w, 400, "invalid_feedback", "organization-private feedback requires an organization project")
			return
		}
		if in.Target.Kind == "release" {
			if releaseStore == nil {
				writeAPIError(w, 400, "invalid_feedback_target", "release target is unavailable")
				return
			}
			if _, e := releaseStore.Get(repo.ID, in.Target.ResourceID); e != nil {
				writeAPIError(w, 400, "invalid_feedback_target", "release does not belong to this project")
				return
			}
		}
		if in.Target.Kind == "journey" {
			if documentation == nil {
				writeAPIError(w, 400, "invalid_feedback_target", "documented journey target is unavailable")
				return
			}
			if _, e := documentation.Current(repo.ID, in.Target.ResourceID); e != nil {
				writeAPIError(w, 400, "invalid_feedback_target", "documented journey does not belong to this project")
				return
			}
		}
		if in.Target.Kind == "preview" {
			if previewStore == nil {
				writeAPIError(w, 400, "invalid_feedback_target", "preview target is unavailable")
				return
			}
			if _, e := previewStore.Find(repo.ID, in.Target.ResourceID); e != nil {
				writeAPIError(w, 400, "invalid_feedback_target", "preview does not belong to this project")
				return
			}
		}
		if !validateLinks(repo.ID, in) {
			writeAPIError(w, 400, "invalid_feedback_link", "related issues and experiments must belong to this project")
			return
		}
		out, e := store.Create(in, actor.UserID)
		writeFeedback(w, out, e, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/feedback", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		all, e := store.List(repo.ID)
		if e != nil {
			writeFeedback(w, productfeedback.Item{}, e, 500)
			return
		}
		out := []productfeedback.Item{}
		for _, x := range all {
			if x.Audience == "organization_private" && !participant && actor.UserID != x.ReporterID {
				continue
			}
			out = append(out, project(x, actor.UserID, participant))
		}
		writeJSON(w, 200, map[string]any{"feedback": out})
	})
	mux.HandleFunc("GET /repositories/{id}/feedback/{feedback_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		x, e := store.Get(r.PathValue("feedback_id"))
		if e != nil || x.RepositoryID != r.PathValue("id") || (x.Audience == "organization_private" && !participant && x.ReporterID != actor.UserID) {
			writeAPIError(w, 404, "feedback_not_found", "feedback not found")
			return
		}
		writeJSON(w, 200, project(x, actor.UserID, participant))
	})
	mux.HandleFunc("POST /repositories/{id}/feedback/{feedback_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		x, e := store.Get(r.PathValue("feedback_id"))
		if e != nil || x.RepositoryID != r.PathValue("id") || (actor.UserID != x.ReporterID && !participant) {
			writeAPIError(w, 404, "feedback_not_found", "feedback not found")
			return
		}
		var in feedbackCommentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a discussion comment is required")
			return
		}
		role := "maintainer"
		if actor.UserID == x.ReporterID {
			role = "reporter"
		}
		out, e := store.AddComment(x.ID, actor.UserID, in.Body, role)
		if e == nil {
			out = project(out, actor.UserID, participant)
		}
		writeFeedback(w, out, e, 200)
	})
}

func writeFeedback(w http.ResponseWriter, x productfeedback.Item, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, x)
	case errors.Is(e, productfeedback.ErrInvalid):
		writeAPIError(w, 400, "invalid_feedback", "feedback requires a valid target, need, outcome, frequency, impact, redacted evidence, audience, identity visibility, and contact preference")
	case errors.Is(e, productfeedback.ErrNotFound):
		writeAPIError(w, 404, "feedback_not_found", "feedback not found")
	default:
		log.Printf("feedback storage: %v", e)
		writeAPIError(w, 500, "feedback_unavailable", "feedback could not be persisted")
	}
}
