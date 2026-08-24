package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/propagationcampaigns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerPropagationCampaignRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, campaigns *propagationcampaigns.Store, pulls *pullrequests.Store) {
	actorID := func(c auth.Credential) string {
		if c.AgentID != "" {
			return c.AgentID
		}
		return c.UserID
	}
	project := func(v propagationcampaigns.Campaign, c auth.Credential) propagationcampaigns.Campaign {
		principal := c.UserID
		for i := range v.Targets {
			t := &v.Targets[i]
			t.State = "pending"
			t.Diagnostic = ""
			t.Authority = "campaign records intent only; delivery requires target repository authority"
			if t.Kind == "package" {
				t.State = "unknown"
				t.Diagnostic = "package release-line equivalence requires package authority and evidence"
				continue
			}
			repo, e := catalog.GetByID(t.RepositoryID)
			if e != nil {
				t.State = "unsupported"
				t.Diagnostic = "target repository is unknown or unsupported"
				continue
			}
			collab, _ := catalog.HasCollaborator(principal, t.RepositoryID)
			if repo.OwnerID != principal && !collab {
				t.State = "inaccessible"
				t.Diagnostic = "target repository is not accessible to this collaborator"
				continue
			}
			ownersOK := true
			for _, owner := range t.OwnerIDs {
				ok, _ := catalog.HasCollaborator(owner, t.RepositoryID)
				if owner != repo.OwnerID && !ok {
					ownersOK = false
				}
			}
			if !ownersOK {
				t.State = "unknown"
				t.Diagnostic = "one or more target owners are not current repository participants"
				continue
			}
			gr, e := git.Open(t.RepositoryID)
			if e != nil {
				t.State = "unsupported"
				t.Diagnostic = "target Git history is unavailable"
				continue
			}
			ref := "refs/heads/" + strings.TrimPrefix(t.ReleaseLine, "refs/heads/")
			tip, e := exec.Command("git", "--git-dir="+gr.Path(), "rev-parse", "--verify", ref+"^{commit}").Output()
			if e != nil {
				t.State = "unknown"
				t.Diagnostic = "target release line does not resolve"
				continue
			}
			tree := func(path, rev string) string {
				b, e := exec.Command("git", "--git-dir="+path, "rev-parse", rev+"^{tree}").Output()
				if e != nil {
					return ""
				}
				return strings.TrimSpace(string(b))
			}
			sourceRepo, e := git.Open(v.Source.RepositoryID)
			if e == nil && tree(sourceRepo.Path(), v.Source.Commits[len(v.Source.Commits)-1]) == tree(gr.Path(), strings.TrimSpace(string(tip))) {
				t.State = "already_equivalent"
				t.Diagnostic = "target tip has the same Git tree as the proven source outcome"
			}
		}
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/propagation-campaigns", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, e := campaigns.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "propagation_campaigns_unavailable", "propagation campaigns could not be read")
			return
		}
		for i := range values {
			values[i] = project(values[i], c)
		}
		writeJSON(w, 200, map[string]any{"propagation_campaigns": values})
	})
	mux.HandleFunc("GET /repositories/{id}/propagation-campaigns/{campaign_id}", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := campaigns.Get(r.PathValue("id"), r.PathValue("campaign_id"))
		if e != nil {
			writeAPIError(w, 404, "propagation_campaign_not_found", "propagation campaign not found")
			return
		}
		writeJSON(w, 200, project(v, c))
	})
	mux.HandleFunc("POST /repositories/{id}/propagation-campaigns", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in propagationcampaigns.Campaign
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a propagation campaign is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		in.Source.RepositoryID = in.RepositoryID
		for _, rev := range in.Source.Commits {
			if len(rev) != 40 || !catalog.HasCommit(in.RepositoryID, rev) {
				writeAPIError(w, 422, "propagation_source_revision_missing", "every source commit must resolve in the source repository")
				return
			}
		}
		if in.Source.Kind == "merged_pull" {
			p, e := pulls.Get(in.RepositoryID, in.Source.ResourceID)
			if e != nil || p.Status != pullrequests.Merged || p.MergeCommitID == nil || *p.MergeCommitID != in.Source.Commits[len(in.Source.Commits)-1] {
				writeAPIError(w, 422, "propagation_source_invalid", "merged pull provenance must resolve to its exact merge commit")
				return
			}
		}
		clean := in
		clean.ID = ""
		clean.RequestDigest = ""
		clean.CreatedBy = ""
		clean.CreatedAt = clean.CreatedAt.UTC()
		for i := range clean.Targets {
			clean.Targets[i].State = ""
			clean.Targets[i].Diagnostic = ""
			clean.Targets[i].Authority = ""
		}
		b, _ := json.Marshal(clean)
		sum := sha256.Sum256(b)
		out, e := campaigns.Create(in, actorID(c), hex.EncodeToString(sum[:]))
		if errors.Is(e, propagationcampaigns.ErrConflict) {
			writeAPIError(w, 409, "propagation_request_conflict", "request_id was already used for a different campaign")
			return
		}
		if errors.Is(e, propagationcampaigns.ErrInvalid) {
			writeAPIError(w, 422, "propagation_campaign_invalid", "campaign intent, criteria, deadlines, sequencing, owners, targets, and completion policy are required")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "propagation_campaign_unavailable", "propagation campaign could not be published")
			return
		}
		writeJSON(w, 201, project(out, c))
	})
}
