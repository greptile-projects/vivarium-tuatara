package main

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	productfeedback "github.com/greptile-projects/vivarium-tuatara/apps/api/feedback"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productexperiments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productopportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type opportunityMutation struct {
	ExpectedVersion int                           `json:"expected_version"`
	Revision        productopportunities.Revision `json:"revision"`
	Body            string                        `json:"body"`
	SourceIDs       []string                      `json:"source_ids"`
	Field           string                        `json:"field"`
	To              string                        `json:"to"`
	Reason          string                        `json:"reason"`
}

func registerProductOpportunityRoutes(mux *http.ServeMux, repos *repositories.Store, credentials *auth.Store, store *productopportunities.Store, feedback *productfeedback.Store, issueStore *issues.Store, previewStore *previews.Store, experiments *productexperiments.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request) (auth.Credential, repositories.Repository, bool, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return actor, repositories.Repository{}, false, false
		}
		repo, e := repos.GetByID(r.PathValue("id"))
		if e != nil {
			return actor, repo, false, false
		}
		participant := actor.UserID == repo.OwnerID
		if !participant {
			participant, _ = repos.HasCollaborator(actor.UserID, repo.ID)
		}
		return actor, repo, participant, true
	}
	feedbackRevision := func(x productfeedback.Item) string {
		return x.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	feedbackPermitted := func(x productfeedback.Item, repo repositories.Repository, viewer string, participant bool) bool {
		return x.RepositoryID == repo.ID && (x.Audience != "organization_private" || participant || x.ReporterID == viewer)
	}
	resolveFeedbackSource := func(src productopportunities.Source) (productfeedback.Item, bool) {
		feedbackID := src.ResourceID
		if src.Kind != "feedback" {
			feedbackID = src.ParentID
		}
		x, e := feedback.Get(feedbackID)
		if e != nil {
			return productfeedback.Item{}, false
		}
		if src.Kind == "feedback" {
			return x, true
		}
		for _, evidence := range x.Evidence {
			if evidence.ID == src.ResourceID && evidence.Kind == src.Kind {
				return x, true
			}
		}
		return productfeedback.Item{}, false
	}
	sourcePermitted := func(src productopportunities.Source, x productfeedback.Item, repo repositories.Repository, viewer string, participant bool) bool {
		if !feedbackPermitted(x, repo, viewer, participant) {
			return false
		}
		if src.Kind == "feedback" {
			return true
		}
		for _, evidence := range x.Evidence {
			if evidence.ID == src.ResourceID {
				return viewer == x.ReporterID || evidence.Visibility == "audience" || (participant && evidence.Visibility == "maintainers")
			}
		}
		return false
	}
	visible := func(v productopportunities.Entry, repo repositories.Repository, viewer string, participant bool) bool {
		for _, revision := range v.Revisions {
			for _, src := range revision.Sources {
				if src.Kind != "feedback" && src.Kind != "support_signal" && src.Kind != "usage_evidence" {
					continue
				}
				x, found := resolveFeedbackSource(src)
				if !found || !sourcePermitted(src, x, repo, viewer, participant) {
					return false
				}
			}
		}
		return true
	}
	refresh := func(v productopportunities.Entry, repo repositories.Repository, participant bool) productopportunities.Entry {
		for ri := range v.Revisions {
			for si := range v.Revisions[ri].Sources {
				src := &v.Revisions[ri].Sources[si]
				src.Stale = false
				src.StaleReason = ""
				switch src.Kind {
				case "feedback", "support_signal", "usage_evidence":
					x, found := resolveFeedbackSource(*src)
					if !found || x.RepositoryID != repo.ID {
						src.Stale = true
						src.StaleReason = "source is unavailable"
					} else if src.Revision != feedbackRevision(x) {
						src.Stale = true
						src.StaleReason = "source changed"
					}
				case "issue":
					x, e := issueStore.Get(repo.ID, src.ResourceID)
					if e != nil || src.Revision != strconv.Itoa(x.Version) {
						src.Stale = true
						src.StaleReason = "issue changed or is unavailable"
					}
				case "preview_finding":
					p, e := previewStore.Find(repo.ID, src.ParentID)
					found := false
					if e == nil {
						for _, f := range p.Findings {
							if f.ID == src.ResourceID {
								found = true
								if src.Revision != strconv.Itoa(f.Version) {
									src.Stale = true
									src.StaleReason = "preview finding changed"
								}
							}
						}
					}
					if !found {
						src.Stale = true
						src.StaleReason = "preview finding is unavailable"
					}
				case "experiment_outcome":
					x, e := experiments.Get(src.ParentID)
					found := false
					if e == nil && x.RepositoryID == repo.ID {
						for _, o := range x.OutcomeDecisions {
							if o.ID == src.ResourceID {
								found = true
								if src.Revision != strconv.Itoa(o.Version) {
									src.Stale = true
									src.StaleReason = "experiment outcome changed"
								}
							}
						}
					}
					if !found {
						src.Stale = true
						src.StaleReason = "experiment outcome is unavailable"
					}
				}
			}
		}
		return v
	}
	validate := func(repo repositories.Repository, participant bool, r productopportunities.Revision) bool {
		for _, src := range r.Sources {
			switch src.Kind {
			case "feedback", "support_signal", "usage_evidence":
				x, found := resolveFeedbackSource(src)
				if !found || !sourcePermitted(src, x, repo, "", participant) || src.Revision != feedbackRevision(x) {
					return false
				}
			case "issue":
				x, e := issueStore.Get(repo.ID, src.ResourceID)
				if e != nil || src.Revision != strconv.Itoa(x.Version) {
					return false
				}
			case "preview_finding":
				p, e := previewStore.Find(repo.ID, src.ParentID)
				ok := false
				if e == nil {
					for _, f := range p.Findings {
						ok = ok || (f.ID == src.ResourceID && src.Revision == strconv.Itoa(f.Version))
					}
				}
				if !ok {
					return false
				}
			case "experiment_outcome":
				x, e := experiments.Get(src.ParentID)
				ok := false
				if e == nil && x.RepositoryID == repo.ID {
					for _, o := range x.OutcomeDecisions {
						ok = ok || (o.ID == src.ResourceID && src.Revision == strconv.Itoa(o.Version))
					}
				}
				if !ok {
					return false
				}
			}
		}
		return true
	}
	mux.HandleFunc("GET /repositories/{id}/product-opportunities", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		all, e := store.List(repo.ID)
		if e != nil {
			writeOpportunity(w, productopportunities.Entry{}, e, 500)
			return
		}
		permitted := all[:0]
		for i := range all {
			if visible(all[i], repo, actor.UserID, participant) {
				permitted = append(permitted, refresh(all[i], repo, participant))
			}
		}
		writeJSON(w, 200, map[string]any{"opportunities": permitted})
	})
	mux.HandleFunc("POST /repositories/{id}/product-opportunities", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant && actor.AgentID == "" {
			writeAPIError(w, 403, "opportunity_forbidden", "only project participants or repository read-only agents may synthesize evidence")
			return
		}
		var in productopportunities.Revision
		if decodeJSON(r, &in) != nil || !validate(repo, participant, in) {
			writeAPIError(w, 400, "invalid_opportunity_evidence", "every cited source must be current, permitted, and revision exact")
			return
		}
		kind, id := "human", actor.UserID
		if actor.AgentID != "" {
			kind, id = "agent", actor.AgentID
		}
		out, e := store.Create(repo.ID, id, kind, in)
		writeOpportunity(w, out, e, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/product-opportunities/{opportunity_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		v, e := store.Get(repo.ID, r.PathValue("opportunity_id"))
		if e == nil && !visible(v, repo, actor.UserID, participant) {
			e = productopportunities.ErrNotFound
		}
		if e == nil {
			v = refresh(v, repo, participant)
		}
		writeOpportunity(w, v, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/product-opportunities/{opportunity_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant || actor.AgentID != "" {
			writeAPIError(w, 403, "opportunity_revision_forbidden", "only project participants may revise a synthesis")
			return
		}
		var in opportunityMutation
		if decodeJSON(r, &in) != nil || !validate(repo, participant, in.Revision) {
			writeAPIError(w, 400, "invalid_opportunity_evidence", "a current evidence-backed revision is required")
			return
		}
		out, e := store.Revise(repo.ID, r.PathValue("opportunity_id"), actor.UserID, in.ExpectedVersion, in.Revision)
		writeOpportunity(w, out, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/product-opportunities/{opportunity_id}/challenges", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, _, ok := authorize(w, r)
		if !ok {
			return
		}
		var in opportunityMutation
		if decodeJSON(r, &in) != nil {
			return
		}
		actorID := actor.UserID
		if actor.AgentID != "" {
			actorID = actor.AgentID
		}
		out, e := store.Challenge(repo.ID, r.PathValue("opportunity_id"), actorID, in.ExpectedVersion, productopportunities.Challenge{Body: in.Body, SourceIDs: in.SourceIDs})
		writeOpportunity(w, out, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/product-opportunities/{opportunity_id}/corrections", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant || actor.AgentID != "" {
			writeAPIError(w, 403, "opportunity_correction_forbidden", "only project participants may correct classification")
			return
		}
		var in opportunityMutation
		if decodeJSON(r, &in) != nil {
			return
		}
		out, e := store.Correct(repo.ID, r.PathValue("opportunity_id"), actor.UserID, in.ExpectedVersion, productopportunities.Correction{Field: in.Field, To: in.To, Reason: in.Reason})
		writeOpportunity(w, out, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/product-opportunities/{opportunity_id}/detach-feedback/{feedback_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, _, ok := authorize(w, r)
		if !ok {
			return
		}
		x, e := feedback.Get(r.PathValue("feedback_id"))
		if e != nil || x.RepositoryID != repo.ID || x.ReporterID != actor.UserID {
			writeAPIError(w, 403, "feedback_detach_forbidden", "only the feedback reporter may detach this citation")
			return
		}
		var in opportunityMutation
		if decodeJSON(r, &in) != nil {
			return
		}
		out, e := store.DetachFeedback(repo.ID, r.PathValue("opportunity_id"), x.ID, actor.UserID, in.ExpectedVersion)
		writeOpportunity(w, out, e, 200)
	})
}
func writeOpportunity(w http.ResponseWriter, v productopportunities.Entry, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, productopportunities.ErrNotFound):
		writeAPIError(w, 404, "product_opportunity_not_found", "product opportunity not found")
	case errors.Is(e, productopportunities.ErrConflict):
		writeAPIError(w, 409, "product_opportunity_changed", "product opportunity changed")
	case errors.Is(e, productopportunities.ErrInvalid):
		writeAPIError(w, 400, "invalid_product_opportunity", "complete transparent synthesis fields and citations are required")
	default:
		log.Printf("product opportunity storage: %v", e)
		writeAPIError(w, 500, "product_opportunity_unavailable", "product opportunity could not be persisted")
	}
}
