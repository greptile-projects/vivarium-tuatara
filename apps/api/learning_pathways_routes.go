package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributorpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningpathways"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerLearningPathwayRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, pathways *learningpathways.Store, issueStore *issues.Store, proposalStore *proposals.Store, packageStore *packages.Store, contributorStore *contributorpathways.Store, credentials *auth.Store) {
	isParticipant := func(repositoryID, userID string) bool {
		repo, e := repos.GetByID(repositoryID)
		if e != nil {
			return false
		}
		if repo.OwnerID == userID {
			return true
		}
		ok, _ := repos.HasCollaborator(userID, repositoryID)
		return ok
	}
	present := func(repositoryID, actorID string, v learningpathways.Revision) learningpathways.Revision {
		repo, _ := repos.GetByID(repositoryID)
		canReadRestricted := actorID != "" && isParticipant(repositoryID, actorID)
		gr, _ := git.Open(repositoryID)
		defaultRevision := ""
		if gr != nil {
			if ref, e := gr.ReadReference("refs/heads/" + repo.DefaultBranch); e == nil {
				defaultRevision = ref.Target
			}
		}
		for i := range v.Mentors {
			m := &v.Mentors[i]
			if !isParticipant(repositoryID, m.UserID) {
				m.Status, m.StatusDetail = "inaccessible", "The designated mentor is no longer a repository participant."
			} else {
				m.Status, m.StatusDetail = "current", "The designated mentor is a current repository participant."
			}
		}
		for i := range v.Environments {
			e := &v.Environments[i]
			if !e.Supported {
				e.Status, e.StatusDetail = "unsupported", "This learner environment is explicitly unsupported."
			} else if e.OwnerID == "" || !isParticipant(repositoryID, e.OwnerID) {
				e.Status, e.StatusDetail = "missing_owner", "No current collaborator owns support for this environment."
			} else {
				e.Status, e.StatusDetail = "current", "This environment has a current support owner."
			}
		}
		for mi := range v.Modules {
			for li := range v.Modules[mi].Materials {
				l := &v.Modules[mi].Materials[li]
				l.Status, l.StatusDetail = "current", "The exact learning material is available."
				if l.OwnerID == "" || !isParticipant(repositoryID, l.OwnerID) {
					l.Status, l.StatusDetail = "missing_owner", "The material has no current collaborator owner."
					continue
				}
				switch l.Kind {
				case "documentation", "symbol", "api":
					if gr == nil {
						l.Status, l.StatusDetail = "inaccessible", "Repository content is unavailable."
						break
					}
					c, e := gr.ReadCommit(storage.ObjectID(l.Revision))
					if e != nil {
						l.Status, l.StatusDetail = "inaccessible", "The exact revision is unavailable."
						break
					}
					entries, e := gr.WalkTree(c.Tree)
					found := false
					var content string
					for _, x := range entries {
						if x.Path == l.Path && x.Type == storage.BlobObject {
							found = true
							if l.Kind == "symbol" {
								if b, _, _, er := gr.ReadBlobPreview(x.ID, 1<<20); er == nil {
									content = string(b.Content)
								}
							}
							break
						}
					}
					if e != nil || !found {
						l.Status, l.StatusDetail = "stale", "The exact path is missing at the supported revision."
					} else if l.Kind == "symbol" && !strings.Contains(content, l.Symbol) {
						l.Status, l.StatusDetail = "stale", "The named symbol is missing at the exact revision."
					} else if defaultRevision != "" && defaultRevision != l.Revision {
						l.Status, l.StatusDetail = "stale", "The default branch has moved beyond this exact material revision."
					}
				case "decision":
					if proposalStore == nil {
						l.Status, l.StatusDetail = "inaccessible", "Decision records are unavailable."
					} else if _, e := proposalStore.Get(repositoryID, l.ResourceID); e != nil {
						l.Status, l.StatusDetail = "stale", "The linked decision is unavailable."
					}
				case "issue":
					if issueStore == nil {
						l.Status, l.StatusDetail = "inaccessible", "Issue records are unavailable."
					} else if linked, e := issueStore.Get(repositoryID, l.ResourceID); e != nil {
						l.Status, l.StatusDetail = "stale", "The linked issue is unavailable."
					} else if linked.Visibility != "public" && !canReadRestricted {
						l.Status, l.StatusDetail = "inaccessible", "The linked issue is not accessible."
						l.ResourceID, l.Label = "", "Restricted issue"
					}
				case "package":
					if packageStore == nil {
						l.Status, l.StatusDetail = "inaccessible", "Package records are unavailable."
					} else if _, e := packageStore.Get(l.ResourceID, l.PackageVersion); e != nil {
						l.Status, l.StatusDetail = "stale", "The exact package version is unavailable."
					}
				case "contributor_guidance":
					version, e := strconv.Atoi(l.ResourceID)
					if contributorStore == nil {
						l.Status, l.StatusDetail = "inaccessible", "Contributor guidance records are unavailable."
						break
					}
					history, he := contributorStore.List(repositoryID)
					if e != nil || he != nil || version < 1 || version > len(history) {
						l.Status, l.StatusDetail = "stale", "The exact contributor guidance revision is unavailable."
					} else if version != len(history) {
						l.Status, l.StatusDetail = "stale", "Newer contributor guidance has been published."
					}
				}
			}
		}
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/learning-pathways", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		slugs, e := pathways.Slugs(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "learning_pathway_read_failed", "learning pathways could not be read")
			return
		}
		out := []learningpathways.Revision{}
		actorID := ""
		if authenticated {
			actorID = actor.UserID
		}
		for _, s := range slugs {
			if v, e := pathways.Current(r.PathValue("id"), s); e == nil {
				out = append(out, present(r.PathValue("id"), actorID, v))
			}
		}
		writeJSON(w, 200, map[string]any{"pathways": out})
	})
	mux.HandleFunc("GET /repositories/{id}/learning-pathways/{slug}", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := pathways.Current(r.PathValue("id"), r.PathValue("slug"))
		if errors.Is(e, learningpathways.ErrNotFound) {
			writeAPIError(w, 404, "learning_pathway_not_found", "learning pathway not found")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "learning_pathway_read_failed", "learning pathway could not be read")
			return
		}
		history, e := pathways.List(r.PathValue("id"), r.PathValue("slug"))
		if e != nil {
			writeAPIError(w, 500, "learning_pathway_read_failed", "learning pathway could not be read")
			return
		}
		actorID := ""
		if authenticated {
			actorID = actor.UserID
		}
		projectedHistory := make([]learningpathways.Revision, 0, len(history))
		for _, historical := range history {
			projectedHistory = append(projectedHistory, present(r.PathValue("id"), actorID, historical))
		}
		writeJSON(w, 200, map[string]any{"pathway": present(r.PathValue("id"), actorID, v), "history": projectedHistory})
	})
	mux.HandleFunc("PUT /repositories/{id}/learning-pathways/{slug}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                       `json:"expected_version"`
			RequestID       string                    `json:"request_id"`
			Pathway         learningpathways.Revision `json:"pathway"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.Pathway.ID = ""
		in.Pathway.Version = 0
		in.Pathway.RepositoryID = r.PathValue("id")
		in.Pathway.Slug = r.PathValue("slug")
		in.Pathway.PublishedBy = actor.UserID
		in.Pathway.RequestID = in.RequestID
		for i := range in.Pathway.Mentors {
			in.Pathway.Mentors[i].Status, in.Pathway.Mentors[i].StatusDetail = "", ""
		}
		for i := range in.Pathway.Environments {
			in.Pathway.Environments[i].Status, in.Pathway.Environments[i].StatusDetail = "", ""
		}
		for i := range in.Pathway.Modules {
			for j := range in.Pathway.Modules[i].Materials {
				in.Pathway.Modules[i].Materials[j].Status, in.Pathway.Modules[i].Materials[j].StatusDetail = "", ""
			}
		}
		v, e := pathways.Publish(in.Pathway, in.ExpectedVersion)
		if errors.Is(e, learningpathways.ErrConflict) {
			writeAPIError(w, 409, "learning_pathway_changed", "learning pathway version changed")
			return
		}
		if errors.Is(e, learningpathways.ErrRequestChanged) {
			writeAPIError(w, 409, "learning_pathway_request_changed", "request identity was already used with different pathway content")
			return
		}
		if errors.Is(e, learningpathways.ErrInvalid) {
			writeAPIError(w, 422, "invalid_learning_pathway", "the complete pathway, ordered modules, exact materials, exercises, and evidence are required")
			return
		}
		if e != nil && !errors.Is(e, learningpathways.ErrDurabilityUncertain) {
			writeAPIError(w, 500, "learning_pathway_write_failed", "learning pathway could not be published")
			return
		}
		status := 201
		if errors.Is(e, learningpathways.ErrDurabilityUncertain) {
			status = 202
			w.Header().Set("Vivarium-Durability", "uncertain")
		}
		w.Header().Set("Location", "/repositories/"+r.PathValue("id")+"/learning-pathways/"+r.PathValue("slug"))
		writeJSON(w, status, present(r.PathValue("id"), actor.UserID, v))
	})
}
