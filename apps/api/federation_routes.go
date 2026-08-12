package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributoropportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributorpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/federation"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

const maxFederatedRepositoryResponseBytes = 16 << 20

func registerFederationRoutes(mux *http.ServeMux, store *federation.Store, userStore *users.Store, organizationStore *organizations.Store, credentials *auth.Store, gitStore *storage.Store, repositoryStore *repositories.Store, pullStore *pullrequests.Store, releaseStore *releases.Store, issueStore *issues.Store, pathwayStore *contributorpathways.Store, opportunityStore *contributoropportunities.Store) {
	mux.HandleFunc("GET /.well-known/vivarium-federation", func(w http.ResponseWriter, _ *http.Request) {
		d, err := store.Identity()
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", err.Error())
			return
		}
		writeJSON(w, 200, d)
	})
	mux.HandleFunc("GET /federation/actors/user/{id}", func(w http.ResponseWriter, r *http.Request) {
		u, err := userStore.Get(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "federated_actor_not_found", "actor not found")
			return
		}
		d, _ := store.Identity()
		writeJSON(w, 200, map[string]any{"identity": d.InstanceID + ":user:" + u.ID, "instance_id": d.InstanceID, "type": "user", "id": u.ID, "handle": u.Handle, "display_name": u.DisplayName, "profile_version": u.UpdatedAt, "identity_document_version": d.Version})
	})
	mux.HandleFunc("GET /federation/actors/agent/{id}", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.PathValue("id"), ":")
		if len(parts) != 2 || organizationStore == nil {
			writeAPIError(w, 404, "federated_actor_not_found", "actor not found")
			return
		}
		org, err := organizationStore.Get(parts[0])
		if err != nil {
			writeAPIError(w, 404, "federated_actor_not_found", "actor not found")
			return
		}
		for _, a := range org.Agents {
			if a.ID == parts[1] && a.Visibility == "public" {
				d, _ := store.Identity()
				writeJSON(w, 200, map[string]any{"identity": d.InstanceID + ":agent:" + parts[0] + ":" + a.ID, "instance_id": d.InstanceID, "type": "agent", "id": parts[0] + ":" + a.ID, "name": a.Name, "description": a.Description, "capabilities": a.Capabilities, "operators": a.OperatorIDs, "profile_version": a.Version, "identity_document_version": d.Version})
				return
			}
		}
		writeAPIError(w, 404, "federated_actor_not_found", "actor not found")
	})
	mux.HandleFunc("GET /federation/repositories/{id}", func(w http.ResponseWriter, r *http.Request) {
		if repositoryStore == nil || gitStore == nil {
			writeAPIError(w, 503, "federation_unavailable", "repository discovery is unavailable")
			return
		}
		repository, err := repositoryStore.GetByID(r.PathValue("id"))
		if err != nil || repository.Visibility != repositories.Public {
			writeAPIError(w, 404, "federated_repository_not_found", "repository is unavailable")
			return
		}
		git, err := gitStore.Open(repository.ID)
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", "repository metadata is temporarily unavailable")
			return
		}
		refs, err := git.ListReferences()
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", "repository branches are temporarily unavailable")
			return
		}
		branches, revision := visibleFederationBranches(refs, repository.DefaultBranch)
		if revision == "" {
			writeAPIError(w, 503, "federation_unavailable", "default branch revision is unavailable")
			return
		}
		document, _ := store.Identity()
		base := ""
		for _, endpoint := range document.Endpoints {
			if endpoint.Kind == "api" {
				base = strings.TrimRight(endpoint.URL, "/")
			}
		}
		snapshot := federation.RepositorySnapshot{RepositoryID: repository.ID, Name: repository.Name, Visibility: repository.Visibility, DefaultBranch: repository.DefaultBranch, Revision: revision, Branches: branches, Capabilities: []string{"metadata", "branches"}, AuthoritativeURL: base + "/federation/repositories/" + repository.ID}
		if releaseStore != nil {
			if items, e := releaseStore.List(repository.ID); e == nil {
				snapshot.Capabilities = append(snapshot.Capabilities, "releases")
				for _, v := range items {
					snapshot.Releases = append(snapshot.Releases, federation.RepositoryRelease{ID: v.ID, Version: v.Version, Notes: v.Notes, Revision: v.CommitID, PublishedAt: v.CreatedAt.Format(time.RFC3339)})
				}
			}
		}
		if pathwayStore != nil {
			if v, e := pathwayStore.Current(repository.ID); e == nil {
				snapshot.ContributorGuidance = &federation.RepositoryGuidance{Version: v.Version, Goals: v.Goals, Prerequisites: v.Prerequisites, SetupSummary: v.Setup.Summary, Communication: v.Communication, ReviewPolicy: v.ReviewPolicy}
				snapshot.Capabilities = append(snapshot.Capabilities, "contributor-guidance")
			}
		}
		if issueStore != nil {
			if items, e := issueStore.List(repository.ID); e == nil {
				snapshot.Capabilities = append(snapshot.Capabilities, "issues")
				for _, v := range items {
					if v.Visibility == "public" {
						snapshot.Issues = append(snapshot.Issues, federation.RepositoryIssue{ID: v.ID, Title: v.Title, Severity: v.Severity, Status: v.Status, Version: v.Version, UpdatedAt: v.UpdatedAt})
					}
				}
			}
		}
		if opportunityStore != nil {
			if items, e := opportunityStore.List(repository.ID); e == nil {
				snapshot.Capabilities = append(snapshot.Capabilities, "contribution-opportunities")
				for _, v := range items {
					if v.Status == "open" {
						snapshot.Opportunities = append(snapshot.Opportunities, federation.RepositoryOpportunity{ID: v.ID, Title: v.Title, ExpectedOutcome: v.ExpectedOutcome, Scope: v.Scope, Risk: v.Risk, Revision: v.Revision, Status: v.Status, Version: v.Version, EstimatedMinutes: v.EstimatedMinutes, RequiredSkills: v.RequiredSkills, AgentAssistance: v.AgentAssistance})
					}
				}
			}
		}
		signed, err := store.SignRepository(snapshot)
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", err.Error())
			return
		}
		body, err := json.Marshal(signed)
		if err != nil || len(body) > maxFederatedRepositoryResponseBytes {
			writeAPIError(w, 413, "federated_repository_too_large", "repository metadata exceeds the federation response limit")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.WriteHeader(200)
		_, _ = w.Write(body)
	})
	// Public transfers contain only the object closure reachable from one exact
	// advertised branch revision. They are not Git credentials or write APIs.
	mux.HandleFunc("GET /federation/repositories/{id}/transfers/{branch}/{revision}", func(w http.ResponseWriter, r *http.Request) {
		repository, err := repositoryStore.GetByID(r.PathValue("id"))
		if err != nil || repository.Visibility != repositories.Public {
			writeAPIError(w, 404, "federated_repository_not_found", "repository is unavailable")
			return
		}
		git, err := gitStore.Open(repository.ID)
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", "repository is unavailable")
			return
		}
		ref, err := git.ReadReference("refs/heads/" + r.PathValue("branch"))
		if err != nil || ref.Target != r.PathValue("revision") {
			writeAPIError(w, 409, "federated_revision_changed", "the advertised branch revision is no longer current")
			return
		}
		temp, err := os.CreateTemp("", "vivarium-transfer-*.bundle")
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", "transfer could not be prepared")
			return
		}
		path := temp.Name()
		_ = temp.Close()
		defer os.Remove(path)
		command := exec.Command("git", "--git-dir="+git.Path(), "bundle", "create", path, "refs/heads/"+r.PathValue("branch"))
		if output, err := command.CombinedOutput(); err != nil {
			writeAPIError(w, 503, "federation_transfer_failed", strings.TrimSpace(string(output)))
			return
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) > maxFederatedRepositoryResponseBytes {
			writeAPIError(w, 413, "federated_transfer_too_large", "transfer exceeds the federation limit")
			return
		}
		writeJSON(w, 200, map[string]any{"repository_id": repository.ID, "branch": r.PathValue("branch"), "revision": ref.Target, "bundle": base64.RawStdEncoding.EncodeToString(data)})
	})
	require := func(w http.ResponseWriter, r *http.Request) bool {
		actor, ok := authenticateRequest(w, r, credentials, "", false)
		if ok && !store.IsOperator(actor.UserID) {
			writeAPIError(w, 403, "federation_administration_forbidden", "only a configured federation operator may administer federation")
			return false
		}
		return ok
	}
	mux.HandleFunc("GET /federation/peers", func(w http.ResponseWriter, r *http.Request) {
		if !require(w, r) {
			return
		}
		p, e := store.List()
		if e != nil {
			writeAPIError(w, 503, "federation_unavailable", e.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"peers": p})
	})
	mux.HandleFunc("POST /federation/identity/rotate", func(w http.ResponseWriter, r *http.Request) {
		if !require(w, r) {
			return
		}
		d, e := store.Rotate()
		if e != nil {
			writeAPIError(w, 503, "federation_unavailable", e.Error())
			return
		}
		writeJSON(w, 200, d)
	})
	mux.HandleFunc("POST /federation/peers", func(w http.ResponseWriter, r *http.Request) {
		if !require(w, r) {
			return
		}
		var in struct {
			DiscoveryURL string `json:"discovery_url"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "discovery_url is required")
			return
		}
		d, e := fetchFederationDocument(in.DiscoveryURL)
		if e != nil {
			writeAPIError(w, 422, "peer_unreachable", e.Error())
			return
		}
		p, e := store.Upsert(in.DiscoveryURL, d)
		if e != nil {
			writeFederationError(w, e)
			return
		}
		writeJSON(w, 201, p)
	})
	mux.HandleFunc("POST /federation/peers/{id}/refresh", func(w http.ResponseWriter, r *http.Request) {
		if !require(w, r) {
			return
		}
		old, e := store.Get(r.PathValue("id"))
		if e != nil {
			writeFederationError(w, e)
			return
		}
		d, e := fetchFederationDocument(old.DiscoveryURL)
		if e != nil {
			p, _ := store.RecordFailure(old.InstanceID, e.Error())
			writeJSON(w, 202, p)
			return
		}
		if d.InstanceID != old.InstanceID {
			p, _ := store.RecordFailure(old.InstanceID, "discovery URL now identifies a different instance")
			writeJSON(w, 202, p)
			return
		}
		p, e := store.Upsert(old.DiscoveryURL, d)
		if e != nil {
			writeFederationError(w, e)
			return
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("PATCH /federation/peers/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !require(w, r) {
			return
		}
		var in struct {
			Version int    `json:"version"`
			Action  string `json:"action"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "version and action are required")
			return
		}
		p, e := store.Decide(r.PathValue("id"), in.Version, in.Action)
		if e != nil {
			writeFederationError(w, e)
			return
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("POST /federation/repositories/resolve", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticateRequest(w, r, credentials, "", false); !ok {
			return
		}
		var in struct {
			Reference string `json:"reference"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "reference is required")
			return
		}
		cache, err := refreshFederatedRepository(store, in.Reference)
		if err != nil && cache.Snapshot == nil {
			writeFederationError(w, err)
			return
		}
		status := 200
		if err != nil {
			status = 202
		}
		writeJSON(w, status, cache)
	})
	mux.HandleFunc("POST /federation/repositories/{peer}/{repository}/forks", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		reference := r.PathValue("peer") + ":" + r.PathValue("repository")
		cache, err := refreshFederatedRepository(store, reference)
		if err != nil || cache.Status != "current" || cache.Snapshot == nil {
			writeAPIError(w, 409, "federated_repository_unavailable", "a current verified repository snapshot is required")
			return
		}
		var in struct {
			Name   string `json:"name"`
			Branch string `json:"branch"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "name is required")
			return
		}
		if in.Branch == "" {
			in.Branch = cache.Snapshot.DefaultBranch
		}
		revision := ""
		for _, b := range cache.Snapshot.Branches {
			if b.Name == in.Branch {
				revision = b.Revision
			}
		}
		if revision == "" {
			writeAPIError(w, 400, "invalid_branch", "branch is not advertised")
			return
		}
		transfer, err := fetchFederatedTransfer(store, r.PathValue("peer"), r.PathValue("repository"), in.Branch, revision)
		if err != nil {
			writeAPIError(w, 422, "federated_transfer_failed", err.Error())
			return
		}
		source, cleanup, err := openTransfer(transfer.Bundle)
		if err != nil {
			writeAPIError(w, 422, "federated_transfer_invalid", err.Error())
			return
		}
		defer cleanup()
		fork, err := repositoryStore.CreateFederatedFork(actor.UserID, reference, in.Branch, in.Name, source, revision)
		if writeRepositoryError(w, err) {
			return
		}
		w.Header().Set("Location", "/repositories/"+fork.ID)
		writeJSON(w, 201, fork)
	})
	mux.HandleFunc("POST /federation/forks/{id}/synchronizations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		fork, err := repositoryStore.Get(actor.UserID, r.PathValue("id"))
		if err != nil || fork.FederatedUpstream == "" {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		cache, err := refreshFederatedRepository(store, fork.FederatedUpstream)
		if err != nil || cache.Snapshot == nil {
			writeAPIError(w, 409, "federated_repository_unavailable", "upstream could not be verified")
			return
		}
		revision := ""
		for _, b := range cache.Snapshot.Branches {
			if b.Name == fork.FederatedBranch {
				revision = b.Revision
			}
		}
		transfer, err := fetchFederatedTransfer(store, cache.PeerID, cache.RepositoryID, fork.FederatedBranch, revision)
		if err != nil {
			writeAPIError(w, 422, "federated_transfer_failed", err.Error())
			return
		}
		source, cleanup, err := openTransfer(transfer.Bundle)
		if err != nil {
			writeAPIError(w, 422, "federated_transfer_invalid", err.Error())
			return
		}
		defer cleanup()
		result, err := repositoryStore.SynchronizeFederatedFork(actor.UserID, fork.ID, fork.FederatedBranch, source, revision)
		if errors.Is(err, repositories.ErrForkDiverged) {
			writeAPIError(w, 409, "fork_diverged", "fork branch contains independent work")
			return
		}
		if writeRepositoryError(w, err) {
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("POST /federation/forks/{id}/pulls", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		fork, err := repositoryStore.Get(actor.UserID, r.PathValue("id"))
		if err != nil || fork.FederatedUpstream == "" {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		cache, err := refreshFederatedRepository(store, fork.FederatedUpstream)
		if err != nil || cache.Snapshot == nil {
			writeAPIError(w, 409, "federated_repository_unavailable", "target could not be verified")
			return
		}
		var in struct {
			Title        string `json:"title"`
			Body         string `json:"body"`
			SourceBranch string `json:"source_branch"`
			TargetBranch string `json:"target_branch"`
		}
		if decodeJSON(r, &in) != nil || in.Title == "" {
			writeAPIError(w, 400, "invalid_pull_request", "title and branches are required")
			return
		}
		if in.SourceBranch == "" {
			in.SourceBranch = fork.DefaultBranch
		}
		if in.TargetBranch == "" {
			in.TargetBranch = cache.Snapshot.DefaultBranch
		}
		sourceGit, err := gitStore.Open(fork.ID)
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", "source is unavailable")
			return
		}
		sourceRef, err := sourceGit.ReadReference("refs/heads/" + in.SourceBranch)
		if err != nil {
			writeAPIError(w, 400, "invalid_branch", "source branch is unavailable")
			return
		}
		targetRevision := ""
		for _, b := range cache.Snapshot.Branches {
			if b.Name == in.TargetBranch {
				targetRevision = b.Revision
			}
		}
		if targetRevision == "" {
			writeAPIError(w, 400, "invalid_branch", "target branch is unavailable")
			return
		}
		idBytes := make([]byte, 16)
		_, _ = rand.Read(idBytes)
		document, _ := store.Identity()
		payload := contributionPayload{ID: hex.EncodeToString(idBytes), SourceInstanceID: document.InstanceID, SourceRepositoryID: fork.ID, SourceBranch: in.SourceBranch, SourceRevision: sourceRef.Target, TargetRepositoryID: cache.RepositoryID, TargetBranch: in.TargetBranch, TargetRevision: targetRevision, Author: document.InstanceID + ":user:" + actor.UserID, Title: in.Title, Body: in.Body, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
		version, key, signature, err := store.SignPayload(contributionBytes(payload))
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", "proposal could not be signed")
			return
		}
		temp, err := os.CreateTemp("", "vivarium-contribution-*.bundle")
		if err != nil {
			return
		}
		path := temp.Name()
		temp.Close()
		defer os.Remove(path)
		cmd := exec.Command("git", "--git-dir="+sourceGit.Path(), "bundle", "create", path, "refs/heads/"+in.SourceBranch)
		if output, err := cmd.CombinedOutput(); err != nil {
			writeAPIError(w, 422, "federated_transfer_failed", string(output))
			return
		}
		data, _ := os.ReadFile(path)
		envelope := signedContribution{Payload: payload, DocumentVersion: version, SigningKeyID: key, Signature: signature, Bundle: base64.RawStdEncoding.EncodeToString(data)}
		response, err := sendContribution(store, cache.PeerID, envelope)
		if err != nil {
			writeAPIError(w, 422, "federated_proposal_failed", err.Error())
			return
		}
		writeJSON(w, 201, response)
	})
	mux.HandleFunc("POST /federation/contributions", func(w http.ResponseWriter, r *http.Request) {
		var in signedContribution
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_federated_contribution", "invalid contribution envelope")
			return
		}
		peer, err := store.Get(in.Payload.SourceInstanceID)
		if err != nil || peer.Status != "trusted" || !contains(peer.Document.Capabilities, "repository-contribution.v1") {
			writeAPIError(w, 403, "federated_source_untrusted", "source instance is not trusted for contributions")
			return
		}
		if federation.VerifyPayload(contributionBytes(in.Payload), in.DocumentVersion, in.SigningKeyID, in.Signature, peer.Document) != nil {
			writeAPIError(w, 422, "invalid_federated_signature", "contribution signature is invalid")
			return
		}
		target, err := repositoryStore.GetByID(in.Payload.TargetRepositoryID)
		if err != nil || target.Visibility != repositories.Public {
			writeAPIError(w, 404, "federated_repository_not_found", "target is unavailable")
			return
		}
		targetGit, _ := gitStore.Open(target.ID)
		targetRef, err := targetGit.ReadReference("refs/heads/" + in.Payload.TargetBranch)
		if err != nil || targetRef.Target != in.Payload.TargetRevision {
			writeAPIError(w, 409, "federated_target_changed", "target branch changed; negotiate a fresh proposal")
			return
		}
		source, cleanup, err := openTransfer(in.Bundle)
		if err != nil {
			writeAPIError(w, 422, "federated_transfer_invalid", err.Error())
			return
		}
		defer cleanup()
		authorSum := sha256.Sum256([]byte(in.Payload.Author))
		pull, err := pullStore.CreateFederated(target.ID, hex.EncodeToString(authorSum[:16]), in.Payload.Author, in.Payload.ID, in.Payload.Title, in.Payload.Body, in.Payload.SourceBranch, in.Payload.TargetBranch, in.Payload.SourceRevision, source)
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, 201, pull)
	})
	mux.HandleFunc("GET /federation/repositories/{peer}/{repository}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticateRequest(w, r, credentials, "", false); !ok {
			return
		}
		reference := r.PathValue("peer") + ":" + r.PathValue("repository")
		cache, err := store.RepositoryCache(reference)
		if err != nil {
			writeFederationError(w, err)
			return
		}
		writeJSON(w, 200, cache)
	})
	mux.HandleFunc("POST /federation/repositories/{peer}/{repository}/refresh", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticateRequest(w, r, credentials, "", false); !ok {
			return
		}
		reference := r.PathValue("peer") + ":" + r.PathValue("repository")
		cache, err := refreshFederatedRepository(store, reference)
		status := 200
		if err != nil {
			status = 202
		}
		writeJSON(w, status, cache)
	})
	mux.HandleFunc("PUT /federation/repositories/{peer}/{repository}/follow", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "", false)
		if !ok {
			return
		}
		reference := r.PathValue("peer") + ":" + r.PathValue("repository")
		var in struct {
			Follow bool `json:"follow"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "follow is required")
			return
		}
		if _, err := store.RepositoryCache(reference); err != nil {
			writeFederationError(w, err)
			return
		}
		follow, err := store.Follow(actor.UserID, reference, in.Follow)
		if err != nil {
			writeFederationError(w, err)
			return
		}
		writeJSON(w, 200, follow)
	})
	mux.HandleFunc("GET /federation/follows", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "", false)
		if !ok {
			return
		}
		follows, err := store.ListFollows(actor.UserID)
		if err != nil {
			writeFederationError(w, err)
			return
		}
		items := []map[string]any{}
		for _, follow := range follows {
			cache, e := store.RepositoryCache(follow.Reference)
			if e == nil {
				items = append(items, map[string]any{"follow": follow, "repository": cache})
			}
		}
		writeJSON(w, 200, map[string]any{"repositories": items})
	})
}

func visibleFederationBranches(refs []storage.Reference, defaultBranch string) ([]federation.RepositoryBranch, string) {
	branches := []federation.RepositoryBranch{}
	revision := ""
	for _, ref := range refs {
		if !strings.HasPrefix(ref.Name, "refs/heads/") || ref.Symbolic {
			continue
		}
		name := strings.TrimPrefix(ref.Name, "refs/heads/")
		if strings.HasPrefix(name, "vivarium-security/") {
			continue
		}
		branches = append(branches, federation.RepositoryBranch{Name: name, Revision: ref.Target})
		if name == defaultBranch {
			revision = ref.Target
		}
	}
	return branches, revision
}

func refreshFederatedRepository(store *federation.Store, reference string) (federation.RepositoryCache, error) {
	peerID, repositoryID, ok := strings.Cut(strings.TrimSpace(reference), ":")
	if !ok {
		return federation.RepositoryCache{}, federation.ErrInvalid
	}
	peer, err := store.Get(peerID)
	if err != nil {
		return federation.RepositoryCache{}, err
	}
	if peer.Status != "trusted" {
		return federation.RepositoryCache{}, federation.ErrConflict
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	old, _ := store.RepositoryCache(reference)
	cache := old
	cache.Reference, cache.PeerID, cache.RepositoryID, cache.LastAttemptAt = reference, peerID, repositoryID, now
	if !contains(peer.Document.Capabilities, "repository-discovery.v1") {
		cache.Status, cache.LastError = "unsupported", "peer does not support repository discovery"
		if cache.Snapshot != nil && cache.StaleSince == nil {
			cache.StaleSince = &now
		}
		cache, _ = store.PutRepositoryCache(cache)
		return cache, federation.ErrInvalid
	}
	signed, fetchErr := fetchFederatedRepository(peer, repositoryID)
	if fetchErr != nil {
		cache.Status, cache.LastError = "unreachable", fetchErr.Error()
		if strings.Contains(fetchErr.Error(), "404") {
			cache.Status = "inaccessible"
		}
		if cache.Snapshot != nil && cache.StaleSince == nil {
			cache.StaleSince = &now
		}
		cache, _ = store.PutRepositoryCache(cache)
		return cache, fetchErr
	}
	if err = federation.VerifyRepository(signed, peer.Document); err != nil {
		cache.Status, cache.LastError = "invalid-signature", "remote repository signature is invalid"
		if cache.Snapshot != nil && cache.StaleSince == nil {
			cache.StaleSince = &now
		}
		cache, _ = store.PutRepositoryCache(cache)
		return cache, err
	}
	cache.Status, cache.Snapshot, cache.RemoteRevision, cache.SignatureVerified, cache.IdentityDocumentVersion, cache.FetchedAt, cache.StaleSince, cache.LastError = "current", &signed.Snapshot, signed.Snapshot.Revision, true, signed.DocumentVersion, now, nil, ""
	return store.PutRepositoryCache(cache)
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func fetchFederatedRepository(peer federation.Peer, repositoryID string) (federation.SignedRepositorySnapshot, error) {
	u, err := url.Parse(peer.DiscoveryURL)
	if err != nil {
		return federation.SignedRepositorySnapshot{}, err
	}
	u.Path = "/federation/repositories/" + url.PathEscape(repositoryID)
	u.RawQuery = ""
	u.Fragment = ""
	transport := &http.Transport{Proxy: nil, DialContext: safeFederationDialer(u.Scheme == "http" && isLoopbackHost(u.Hostname()))}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(u.String())
	if err != nil {
		return federation.SignedRepositorySnapshot{}, fmt.Errorf("peer unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return federation.SignedRepositorySnapshot{}, fmt.Errorf("peer returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFederatedRepositoryResponseBytes+1))
	if err != nil {
		return federation.SignedRepositorySnapshot{}, fmt.Errorf("read repository response: %w", err)
	}
	if len(body) > maxFederatedRepositoryResponseBytes {
		return federation.SignedRepositorySnapshot{}, fmt.Errorf("peer repository response exceeds %d bytes", maxFederatedRepositoryResponseBytes)
	}
	var signed federation.SignedRepositorySnapshot
	decoder := json.NewDecoder(bytes.NewReader(body))
	if decoder.Decode(&signed) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return signed, fmt.Errorf("peer returned an invalid repository response")
	}
	return signed, nil
}

type federatedTransfer struct {
	RepositoryID string `json:"repository_id"`
	Branch       string `json:"branch"`
	Revision     string `json:"revision"`
	Bundle       string `json:"bundle"`
}
type contributionPayload struct {
	ID                 string    `json:"id"`
	SourceInstanceID   string    `json:"source_instance_id"`
	SourceRepositoryID string    `json:"source_repository_id"`
	SourceBranch       string    `json:"source_branch"`
	SourceRevision     string    `json:"source_revision"`
	TargetRepositoryID string    `json:"target_repository_id"`
	TargetBranch       string    `json:"target_branch"`
	TargetRevision     string    `json:"target_revision"`
	Author             string    `json:"author"`
	Title              string    `json:"title"`
	Body               string    `json:"body"`
	CreatedAt          time.Time `json:"created_at"`
}
type signedContribution struct {
	Payload         contributionPayload `json:"payload"`
	DocumentVersion int                 `json:"identity_document_version"`
	SigningKeyID    string              `json:"signing_key_id"`
	Signature       string              `json:"signature"`
	Bundle          string              `json:"bundle"`
}

func contributionBytes(v contributionPayload) []byte { b, _ := json.Marshal(v); return b }
func fetchFederatedTransfer(store *federation.Store, peerID, repositoryID, branch, revision string) (federatedTransfer, error) {
	peer, err := store.Get(peerID)
	if err != nil || peer.Status != "trusted" {
		return federatedTransfer{}, federation.ErrConflict
	}
	u, err := url.Parse(peer.DiscoveryURL)
	if err != nil {
		return federatedTransfer{}, err
	}
	u.Path = "/federation/repositories/" + url.PathEscape(repositoryID) + "/transfers/" + url.PathEscape(branch) + "/" + revision
	u.RawQuery = ""
	transport := &http.Transport{Proxy: nil, DialContext: safeFederationDialer(u.Scheme == "http" && isLoopbackHost(u.Hostname()))}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(u.String())
	if err != nil {
		return federatedTransfer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return federatedTransfer{}, fmt.Errorf("peer returned %s", resp.Status)
	}
	var v federatedTransfer
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxFederatedRepositoryResponseBytes*2))
	if decoder.Decode(&v) != nil || v.RepositoryID != repositoryID || v.Branch != branch || v.Revision != revision {
		return v, fmt.Errorf("peer returned an invalid transfer")
	}
	return v, nil
}
func openTransfer(encoded string) (*storage.Repository, func(), error) {
	data, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(data) > maxFederatedRepositoryResponseBytes {
		return nil, nil, fmt.Errorf("invalid transfer encoding")
	}
	file, err := os.CreateTemp("", "vivarium-transfer-*.bundle")
	if err != nil {
		return nil, nil, err
	}
	path := file.Name()
	if _, err = file.Write(data); err != nil {
		file.Close()
		os.Remove(path)
		return nil, nil, err
	}
	file.Close()
	repo, cleanup, err := storage.OpenBundle(path)
	os.Remove(path)
	return repo, cleanup, err
}
func sendContribution(store *federation.Store, peerID string, envelope signedContribution) (map[string]any, error) {
	peer, err := store.Get(peerID)
	if err != nil || peer.Status != "trusted" {
		return nil, federation.ErrConflict
	}
	u, err := url.Parse(peer.DiscoveryURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/federation/contributions"
	u.RawQuery = ""
	body, _ := json.Marshal(envelope)
	transport := &http.Transport{Proxy: nil, DialContext: safeFederationDialer(u.Scheme == "http" && isLoopbackHost(u.Hostname()))}
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Post(u.String(), "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	resultBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 201 {
		return nil, fmt.Errorf("peer rejected proposal (%s): %s", resp.Status, strings.TrimSpace(string(resultBody)))
	}
	var result map[string]any
	if json.Unmarshal(resultBody, &result) != nil {
		return nil, fmt.Errorf("peer returned invalid proposal response")
	}
	return result, nil
}

func fetchFederationDocument(raw string) (federation.Document, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return federation.Document{}, fmt.Errorf("invalid discovery URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return federation.Document{}, fmt.Errorf("discovery requires HTTPS (HTTP is allowed only for loopback development)")
	}
	u.Path = "/.well-known/vivarium-federation"
	u.RawQuery = ""
	u.Fragment = ""
	transport := &http.Transport{Proxy: nil, DialContext: safeFederationDialer(u.Scheme == "http" && isLoopbackHost(u.Hostname()))}
	c := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Get(u.String())
	if err != nil {
		return federation.Document{}, fmt.Errorf("peer unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return federation.Document{}, fmt.Errorf("peer returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return federation.Document{}, err
	}
	var d federation.Document
	if json.Unmarshal(body, &d) != nil {
		return d, fmt.Errorf("peer returned an invalid identity document")
	}
	if err = federation.Verify(d); err != nil {
		return d, fmt.Errorf("peer signature is invalid")
	}
	return d, nil
}
func safeFederationDialer(allowLoopback bool) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(resolved) == 0 {
			return nil, fmt.Errorf("discovery host has no addresses")
		}
		for _, candidate := range resolved {
			if !publicFederationIP(candidate.IP) && !(allowLoopback && candidate.IP.IsLoopback()) {
				return nil, fmt.Errorf("discovery address is not public")
			}
		}
		dialer := net.Dialer{Timeout: 5 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(resolved[0].IP.String(), port))
	}
}

var federationSpecialPurposePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"), netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"), netip.MustParsePrefix("::ffff:0:0/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"), netip.MustParsePrefix("ff00::/8"),
}

func publicFederationIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	for _, prefix := range federationSpecialPurposePrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return address.IsGlobalUnicast()
}
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func writeFederationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, federation.ErrNotFound):
		writeAPIError(w, 404, "federation_peer_not_found", err.Error())
	case errors.Is(err, federation.ErrConflict):
		writeAPIError(w, 409, "federation_conflict", err.Error())
	case errors.Is(err, federation.ErrInvalid):
		writeAPIError(w, 422, "invalid_federation_identity", err.Error())
	default:
		writeAPIError(w, 503, "federation_unavailable", err.Error())
	}
}
