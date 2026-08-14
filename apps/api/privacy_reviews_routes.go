package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/datacommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/dataflows"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/privacyreviews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type privacyReviewInput struct {
	SourceFlowID      string `json:"source_flow_id"`
	SourceFlowVersion int    `json:"source_flow_version"`
	TargetFlowID      string `json:"target_flow_id"`
	TargetFlowVersion int    `json:"target_flow_version"`
}
type privacyAcceptanceInput struct {
	SourceRevision   string   `json:"source_revision"`
	ResidualRisk     string   `json:"residual_risk"`
	RequirementKinds []string `json:"requirement_kinds"`
}

func registerPrivacyReviewRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, commitments *datacommitments.Store, flows *dataflows.Store, reviews *privacyreviews.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return actor, false, false
		}
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return actor, false, false
		}
		repo, e := catalog.GetByID(r.PathValue("id"))
		if e != nil {
			writeRepositoryError(w, e)
			return actor, false, false
		}
		participant := actor.AgentID == "" && actor.UserID == repo.OwnerID
		if !participant && actor.AgentID == "" {
			participant, _ = catalog.HasCollaborator(actor.UserID, repo.ID)
		}
		return actor, participant, true
	}
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/privacy-review", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorize(w, r)
		if !ok {
			return
		}
		v, e := reviews.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e == nil {
			if p, x := pulls.Get(r.PathValue("id"), r.PathValue("pull_id")); x == nil && p.SourceCommitID != v.SourceRevision {
				v.AcceptedBy = ""
				v.AcceptedAt = nil
				for i := range v.Requirements {
					v.Requirements[i].Status = "stale"
				}
			}
		}
		writePrivacyReview(w, v, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/privacy-review", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant && actor.AgentID == "" {
			writeAPIError(w, 403, "privacy_review_forbidden", "only collaborators and repository-bound agents may compare privacy consequences")
			return
		}
		var in privacyReviewInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "exact source and target data-flow versions are required")
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeAPIError(w, 404, "pull_not_found", "pull request not found")
			return
		}
		source, se := flows.Get(r.PathValue("id"), in.SourceFlowID)
		target, te := flows.Get(r.PathValue("id"), in.TargetFlowID)
		if se != nil || te != nil || in.SourceFlowVersion < 1 || in.SourceFlowVersion > len(source.Revisions) || in.TargetFlowVersion < 1 || in.TargetFlowVersion > len(target.Revisions) {
			writeAPIError(w, 422, "invalid_privacy_evidence", "both data-flow versions must resolve in this repository")
			return
		}
		sr, tr := source.Revisions[in.SourceFlowVersion-1], target.Revisions[in.TargetFlowVersion-1]
		if sr.CodeRevision != p.SourceCommitID || tr.CodeRevision != p.TargetCommitID {
			writeAPIError(w, 409, "stale_privacy_evidence", "data flows must match the pull's current source and target revisions")
			return
		}
		changes, requirements, valid := comparePrivacy(commitments, r.PathValue("id"), sr, tr)
		if !valid {
			writeAPIError(w, 422, "invalid_privacy_evidence", "every exact commitment and data-use reference must remain readable")
			return
		}
		typ, id := "human", actor.UserID
		if actor.AgentID != "" {
			typ, id = "agent", actor.AgentID
		}
		v := privacyreviews.Review{RepositoryID: r.PathValue("id"), PullRequestID: p.ID, SourceRevision: p.SourceCommitID, TargetRevision: p.TargetCommitID, SourceFlowID: source.ID, SourceFlowVersion: in.SourceFlowVersion, TargetFlowID: target.ID, TargetFlowVersion: in.TargetFlowVersion, Changes: changes, Requirements: requirements, CreatedByType: typ, CreatedBy: id}
		out, e := reviews.Create(v)
		writePrivacyReview(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/privacy-review/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant && actor.AgentID == "" {
			writeAPIError(w, 403, "privacy_review_forbidden", "only collaborators and repository-bound agents may challenge or mitigate findings")
			return
		}
		var in privacyreviews.Comment
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded challenge, mitigation, or residual-risk record is required")
			return
		}
		v, e := reviews.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writePrivacyReview(w, v, e, 0)
			return
		}
		findings := []dataflows.Finding{{Citations: make([]dataflows.Citation, len(in.Evidence))}}
		for i, c := range in.Evidence {
			findings[0].Citations[i] = dataflows.Citation{Path: c.Path, StartLine: c.StartLine, EndLine: c.EndLine, Claim: c.Claim}
		}
		if len(in.Evidence) > 0 && !dataFlowCitationsResolve(git, r.PathValue("id"), v.SourceRevision, findings) {
			writeAPIError(w, 422, "invalid_privacy_citation", "evidence must resolve in the reviewed pull revision")
			return
		}
		typ, id := "human", actor.UserID
		if actor.AgentID != "" {
			typ, id = "agent", actor.AgentID
		}
		out, e := reviews.AddComment(r.PathValue("id"), r.PathValue("pull_id"), typ, id, in)
		writePrivacyReview(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/privacy-review/acceptance", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant || actor.AgentID != "" {
			writeAPIError(w, 403, "privacy_acceptance_forbidden", "only a current human collaborator may acknowledge privacy requirements")
			return
		}
		var in privacyAcceptanceInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "the exact revision, all requirements, and residual risk are required")
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil || p.SourceCommitID != in.SourceRevision {
			writeAPIError(w, 409, "stale_privacy_review", "the pull moved; compare the current revision before accepting")
			return
		}
		out, e := reviews.Accept(r.PathValue("id"), r.PathValue("pull_id"), in.SourceRevision, actor.UserID, in.ResidualRisk, in.RequirementKinds)
		writePrivacyReview(w, out, e, 200)
	})
}

func comparePrivacy(store *datacommitments.Store, repo string, source, target dataflows.Revision) ([]privacyreviews.Change, []privacyreviews.Requirement, bool) {
	targetEdges := map[string]dataflows.Edge{}
	for _, e := range target.Edges {
		targetEdges[edgeKey(e)] = e
	}
	changes := []privacyreviews.Change{}
	seen := map[string]bool{}
	add := func(kind, summary string, cats, ids []string) {
		key := kind + summary
		if !seen[key] {
			changes = append(changes, privacyreviews.Change{Kind: kind, Summary: summary, DataCategories: cats, SourceIDs: ids})
			seen[key] = true
		}
	}
	for _, e := range source.Edges {
		old, ok := targetEdges[edgeKey(e)]
		if !ok {
			add("collection", "New data path: "+e.Operation+" to "+e.To, e.DataCategories, []string{e.ID})
		} else {
			if !privacyStringsEqual(e.DataCategories, old.DataCategories) {
				add("collection", "Data categories changed on "+e.Operation, e.DataCategories, []string{e.ID})
			}
			if e.Purpose != old.Purpose {
				add("purpose", "Purpose changed to "+e.Purpose, e.DataCategories, []string{e.ID})
			}
			if e.RetainedCopy && !old.RetainedCopy {
				add("retention", "The path creates a newly retained copy.", e.DataCategories, []string{e.ID})
			}
		}
	}
	sourceUses, ok := usesForRefs(store, repo, source.CommitmentRefs)
	if !ok {
		return nil, nil, false
	}
	targetUses, ok := usesForRefs(store, repo, target.CommitmentRefs)
	if !ok {
		return nil, nil, false
	}
	for id, u := range sourceUses {
		old, exists := targetUses[id]
		if !exists {
			add("collection", "New committed data use: "+u.Category, []string{u.Category}, []string{id})
			continue
		}
		if !privacyStringsEqual(u.Purposes, old.Purposes) {
			add("purpose", "Purposes changed for "+u.Category, []string{u.Category}, []string{id})
		}
		if !privacyStringsEqual(u.Sharing, old.Sharing) {
			add("recipient", "Recipients changed for "+u.Category, []string{u.Category}, []string{id})
		}
		if u.Retention != old.Retention {
			add("retention", "Retention changed for "+u.Category, []string{u.Category}, []string{id})
		}
		if u.Collection != old.Collection || !privacyStringsEqual(u.Processing, old.Processing) {
			add("access", "Collection or processing access changed for "+u.Category, []string{u.Category}, []string{id})
		}
		if u.Consent != old.Consent || u.Deletion != old.Deletion {
			add("user_control", "Consent or deletion controls changed for "+u.Category, []string{u.Category}, []string{id})
		}
	}
	reqs := deriveRequirements(changes, sourceUses)
	return changes, reqs, true
}
func edgeKey(e dataflows.Edge) string { return e.From + "\x00" + e.To + "\x00" + e.Operation }
func privacyStringsEqual(a, b []string) bool {
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	return strings.Join(aa, "\x00") == strings.Join(bb, "\x00")
}
func usesForRefs(s *datacommitments.Store, repo string, refs []dataflows.CommitmentRef) (map[string]datacommitments.DataUse, bool) {
	out := map[string]datacommitments.DataUse{}
	for _, ref := range refs {
		c, e := s.Get(ref.CommitmentID)
		if e != nil || c.RepositoryID != repo || ref.Version < 1 || ref.Version > len(c.Revisions) {
			return nil, false
		}
		all := map[string]datacommitments.DataUse{}
		for _, u := range c.Revisions[ref.Version-1].DataUses {
			all[u.ID] = u
		}
		for _, id := range ref.DataUseIDs {
			u, ok := all[id]
			if !ok {
				return nil, false
			}
			out[id] = u
		}
	}
	return out, true
}
func deriveRequirements(ch []privacyreviews.Change, uses map[string]datacommitments.DataUse) []privacyreviews.Requirement {
	owners := []string{}
	for _, u := range uses {
		owners = append(owners, u.OwnerIDs...)
	}
	owners = unique(owners)
	need := map[string]string{}
	for _, c := range ch {
		need["owner_acknowledgement"] = "Accountable data owners must review the classified change."
		need["test"] = "The changed behavior needs revision-exact privacy verification."
		switch c.Kind {
		case "collection", "purpose", "recipient":
			need["notice"] = "People-facing notice must reflect new handling."
			need["consent"] = "Consent requirements must be checked before integration."
		case "retention", "access":
			need["migration"] = "Existing retained data and access paths need a migration decision."
		case "user_control":
			need["exception"] = "Any temporarily unsupported control requires an explicit exception."
		}
	}
	order := []string{"owner_acknowledgement", "notice", "consent", "migration", "test", "exception"}
	out := []privacyreviews.Requirement{}
	for _, k := range order {
		if reason := need[k]; reason != "" {
			r := privacyreviews.Requirement{Kind: k, Reason: reason, Status: "required"}
			if k == "owner_acknowledgement" {
				r.OwnerIDs = owners
			}
			out = append(out, r)
		}
	}
	return out
}
func unique(v []string) []string {
	m := map[string]bool{}
	out := []string{}
	for _, x := range v {
		if x != "" && !m[x] {
			m[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
func writePrivacyReview(w http.ResponseWriter, v privacyreviews.Review, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, privacyreviews.ErrNotFound):
		writeAPIError(w, 404, "privacy_review_not_found", "privacy review not found")
	case errors.Is(e, privacyreviews.ErrConflict):
		writeAPIError(w, 409, "privacy_review_conflict", "the pull or review boundary changed; reload before continuing")
	case errors.Is(e, privacyreviews.ErrInvalid):
		writeAPIError(w, 400, "invalid_privacy_review", "complete the revision-grounded findings, requirements, and residual-risk record")
	default:
		log.Printf("privacy review storage: %v", e)
		writeAPIError(w, 500, "privacy_reviews_unavailable", fmt.Sprintf("privacy review evidence could not be persisted"))
	}
}
