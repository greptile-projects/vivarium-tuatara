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
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
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
const maxFederatedContributionEnvelopeBytes = (maxFederatedRepositoryResponseBytes*4)/3 + (64 << 10)

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

type signedCollaborationEvent struct {
	Event  federation.CollaborationEvent `json:"event"`
	Bundle string                        `json:"bundle,omitempty"`
}

func collaborationEventBytes(v federation.CollaborationEvent) []byte {
	v.Signature, v.Verification, v.SigningKeyID = "", "", ""
	v.DocumentVersion = 0
	b, _ := json.Marshal(v)
	return b
}

func (w *boundedBuffer) Write(p []byte) (int, error) {
	remaining := w.limit - w.Len()
	if remaining <= 0 {
		return 0, fmt.Errorf("transfer exceeds the federation limit")
	}
	if len(p) > remaining {
		_, _ = w.Buffer.Write(p[:remaining])
		return remaining, fmt.Errorf("transfer exceeds the federation limit")
	}
	return w.Buffer.Write(p)
}

func registerFederationRoutes(mux *http.ServeMux, store *federation.Store, userStore *users.Store, organizationStore *organizations.Store, credentials *auth.Store, gitStore *storage.Store, repositoryStore *repositories.Store, pullStore *pullrequests.Store, sessionStore *changesessions.Store, releaseStore *releases.Store, issueStore *issues.Store, pathwayStore *contributorpathways.Store, opportunityStore *contributoropportunities.Store) {
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
		bundle := &boundedBuffer{limit: maxFederatedRepositoryResponseBytes}
		var stderr bytes.Buffer
		command := exec.CommandContext(r.Context(), "git", "--git-dir="+git.Path(), "bundle", "create", "-", "refs/heads/"+r.PathValue("branch"))
		command.Stdout, command.Stderr = bundle, &stderr
		if err := command.Run(); err != nil {
			if r.Context().Err() != nil {
				return
			}
			if bundle.Len() >= maxFederatedRepositoryResponseBytes {
				writeAPIError(w, 413, "federated_transfer_too_large", "transfer exceeds the federation limit")
				return
			}
			writeAPIError(w, 503, "federation_transfer_failed", strings.TrimSpace(stderr.String()))
			return
		}
		writeJSON(w, 200, map[string]any{"repository_id": repository.ID, "branch": r.PathValue("branch"), "revision": ref.Target, "bundle": base64.RawStdEncoding.EncodeToString(bundle.Bytes())})
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
		bundle := &boundedBuffer{limit: maxFederatedRepositoryResponseBytes}
		var stderr bytes.Buffer
		cmd := exec.CommandContext(r.Context(), "git", "--git-dir="+sourceGit.Path(), "bundle", "create", "-", "refs/heads/"+in.SourceBranch)
		cmd.Stdout, cmd.Stderr = bundle, &stderr
		if err := cmd.Run(); err != nil {
			if r.Context().Err() != nil {
				return
			}
			if bundle.Len() >= maxFederatedRepositoryResponseBytes {
				writeAPIError(w, 422, "federated_transfer_too_large", "contribution exceeds the federation transfer limit")
				return
			}
			writeAPIError(w, 422, "federated_transfer_failed", strings.TrimSpace(stderr.String()))
			return
		}
		envelope := signedContribution{Payload: payload, DocumentVersion: version, SigningKeyID: key, Signature: signature, Bundle: base64.RawStdEncoding.EncodeToString(bundle.Bytes())}
		response, err := sendContribution(store, cache.PeerID, envelope)
		if err != nil {
			writeAPIError(w, 422, "federated_proposal_failed", err.Error())
			return
		}
		if err = store.BindContribution(payload.ID, document.InstanceID, cache.PeerID); err != nil {
			writeAPIError(w, 503, "federation_unavailable", "contribution authority could not be retained")
			return
		}
		if err = store.BindContributionSource(payload.ID, fork.ID, in.SourceBranch, sourceRef.Target); err != nil {
			writeAPIError(w, 503, "federation_unavailable", "contribution publishing boundary could not be retained")
			return
		}
		writeJSON(w, 201, response)
	})
	mux.HandleFunc("POST /federation/contributions", func(w http.ResponseWriter, r *http.Request) {
		var in signedContribution
		if decodeJSONLimit(r, &in, maxFederatedContributionEnvelopeBytes) != nil {
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
		targetGit, err := gitStore.Open(target.ID)
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", "target repository storage is unavailable")
			return
		}
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
		document, _ := store.Identity()
		if err = store.BindContribution(in.Payload.ID, in.Payload.SourceInstanceID, document.InstanceID); err != nil {
			writeAPIError(w, 503, "federation_unavailable", "contribution authority could not be retained")
			return
		}
		if err = store.BindContributionTarget(in.Payload.ID, target.ID, pull.ID, in.Payload.SourceRevision); err != nil {
			writeAPIError(w, 503, "federation_unavailable", "contribution review boundary could not be retained")
			return
		}
		writeJSON(w, 201, pull)
	})
	// Source-instance participants may delegate only against their locally owned
	// contribution branch. Remote context is limited to the already imported
	// source revision and caller-selected repository paths.
	mux.HandleFunc("POST /federation/contributions/{contribution}/agent-sessions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		if sessionStore == nil || organizationStore == nil {
			writeAPIError(w, 503, "federated_agents_unavailable", "agent delegation is unavailable")
			return
		}
		boundary, err := store.ContributionSource(r.PathValue("contribution"))
		if err != nil {
			writeAPIError(w, 404, "federated_contribution_not_found", "federated contribution not found")
			return
		}
		repository, err := repositoryStore.Get(actor.UserID, boundary.SourceRepositoryID)
		if err != nil {
			writeAPIError(w, 404, "federated_contribution_not_found", "federated contribution not found")
			return
		}
		var in struct {
			OrganizationID string   `json:"organization_id"`
			AgentID        string   `json:"agent_id"`
			Instructions   string   `json:"instructions"`
			ContextPaths   []string `json:"context_paths"`
			ExpiresIn      int64    `json:"expires_in"`
		}
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Instructions) == "" || len(in.ContextPaths) == 0 || len(in.ContextPaths) > 50 {
			writeAPIError(w, 400, "invalid_agent_session", "approved agent, instructions, and bounded context paths are required")
			return
		}
		organization, err := organizationStore.Get(in.OrganizationID)
		if err != nil {
			writeAPIError(w, 404, "approved_agent_not_found", "approved agent not found")
			return
		}
		approved := false
		for _, agent := range organization.Agents {
			if agent.ID == in.AgentID && contains(agent.OperatorIDs, actor.UserID) {
				approved = true
				break
			}
		}
		if !approved {
			writeAPIError(w, 403, "approved_agent_operator_required", "the participant must operate the selected approved agent")
			return
		}
		git, err := gitStore.Open(repository.ID)
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", "source repository is unavailable")
			return
		}
		commit, err := git.ReadCommit(storage.ObjectID(boundary.SourceRevision))
		if err != nil {
			writeAPIError(w, 409, "federated_revision_changed", "contribution revision is unavailable")
			return
		}
		entries, err := git.WalkTree(commit.Tree)
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", "context is unavailable")
			return
		}
		available := map[string]bool{}
		for _, entry := range entries {
			available[entry.Path] = true
		}
		for _, path := range in.ContextPaths {
			if !available[path] {
				writeAPIError(w, 400, "invalid_agent_context", "context path is not present in the permitted contribution revision")
				return
			}
		}
		if in.ExpiresIn == 0 {
			in.ExpiresIn = 3600
		}
		if in.ExpiresIn < 300 || in.ExpiresIn > 86400 {
			writeAPIError(w, 400, "invalid_agent_session", "credential lifetime must be between five minutes and 24 hours")
			return
		}
		issued, err := credentials.IssuePullRequestBound(actor.UserID, "federated contribution agent", []string{"git:read", "git:write"}, time.Duration(in.ExpiresIn)*time.Second, repository.ID, "refs/heads/"+boundary.SourceBranch, boundary.ContributionID)
		if err != nil {
			writeAPIError(w, 503, "federated_agents_unavailable", "bounded credential could not be issued")
			return
		}
		session, err := sessionStore.Create(repository.ID, boundary.ContributionID, actor.UserID, boundary.SourceRevision)
		if err != nil {
			_, _ = credentials.Revoke(actor.UserID, issued.ID)
			writeChangeSessionError(w, err)
			return
		}
		run, err := sessionStore.LaunchRunForAgent(repository.ID, boundary.ContributionID, session.ID, actor.UserID, in.AgentID, strings.TrimSpace(in.Instructions), boundary.SourceRevision, in.ContextPaths, boundary.SourceBranch, issued.ID, issued.ExpiresAt)
		if err != nil {
			_, _ = credentials.Revoke(actor.UserID, issued.ID)
			writeChangeSessionError(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"session": session, "run": run, "credential": issued, "context_boundary": map[string]any{"revision": boundary.SourceRevision, "paths": in.ContextPaths, "remote_secrets": false, "remote_checks": false, "remote_repository_access": false}})
	})
	mux.HandleFunc("POST /federation/contributions/{contribution}/agent-sessions/{session}/runs/{run}/completion", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "git:write", false)
		if !ok {
			return
		}
		boundary, err := store.ContributionSource(r.PathValue("contribution"))
		if err != nil || credential.RepositoryID != boundary.SourceRepositoryID || credential.GitWriteBranch != "refs/heads/"+boundary.SourceBranch || credential.PullRequestID != boundary.ContributionID {
			writeAPIError(w, 404, "agent_run_not_found", "agent run not found")
			return
		}
		var in struct {
			Summary      string                   `json:"summary"`
			CommitID     string                   `json:"commit_id"`
			Commands     []changesessions.Command `json:"commands"`
			Checks       []changesessions.Check   `json:"evidence"`
			Concerns     []string                 `json:"residual_concerns"`
			AgentMinutes int                      `json:"agent_minutes"`
			CostUnits    int                      `json:"cost_units"`
		}
		if decodeJSONLimit(r, &in, 256<<10) != nil || in.AgentMinutes < 0 || in.CostUnits < 0 {
			writeAPIError(w, 400, "invalid_agent_completion", "completion evidence is invalid")
			return
		}
		run, _, err := sessionStore.GetRunControl(boundary.SourceRepositoryID, boundary.ContributionID, r.PathValue("session"), r.PathValue("run"), credential.ID)
		if err != nil {
			writeAPIError(w, 404, "agent_run_not_found", "agent run not found")
			return
		}
		git, err := gitStore.Open(boundary.SourceRepositoryID)
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", "source repository is unavailable")
			return
		}
		ref, err := git.ReadReference("refs/heads/" + boundary.SourceBranch)
		if err != nil || ref.Target != in.CommitID {
			writeAPIError(w, 409, "branch_tip_changed", "completion must identify the contribution branch tip")
			return
		}
		head, err := git.ListCommitAncestry(storage.ObjectID(in.CommitID))
		if err != nil {
			writeAPIError(w, 400, "invalid_agent_completion", "commit is invalid")
			return
		}
		base, err := git.ListCommitAncestry(storage.ObjectID(run.SourceCommitID))
		if err != nil {
			writeAPIError(w, 409, "federated_revision_changed", "base revision is unavailable")
			return
		}
		baseSet := map[storage.ObjectID]bool{}
		for _, c := range base {
			baseSet[c.ID] = true
		}
		commits := []string{}
		descendant := false
		for _, c := range head {
			if string(c.ID) == run.SourceCommitID {
				descendant = true
			}
			if !baseSet[c.ID] {
				commits = append(commits, string(c.ID))
			}
		}
		if !descendant || len(commits) == 0 {
			writeAPIError(w, 400, "invalid_agent_completion", "completion must publish descendant commits")
			return
		}
		changes, err := pullStore.CompareCommits(boundary.SourceRepositoryID, run.SourceCommitID, in.CommitID)
		if err != nil {
			writeAPIError(w, 400, "invalid_agent_completion", "changes could not be derived")
			return
		}
		files := make([]changesessions.ChangedFile, len(changes))
		for i, c := range changes {
			files[i] = changesessions.ChangedFile{Path: c.Path, Status: c.Status}
		}
		completed, _, err := sessionStore.CompleteRunWithEvidence(boundary.SourceRepositoryID, boundary.ContributionID, r.PathValue("session"), run.ID, credential.ID, in.Summary, in.CommitID, commits, files, in.Checks, in.Concerns, in.Commands, nil)
		if err != nil {
			writeChangeSessionError(w, err)
			return
		}
		bundle := &boundedBuffer{limit: maxFederatedRepositoryResponseBytes}
		cmd := exec.CommandContext(r.Context(), "git", "--git-dir="+git.Path(), "bundle", "create", "-", "refs/heads/"+boundary.SourceBranch)
		cmd.Stdout = bundle
		if err = cmd.Run(); err != nil {
			writeAPIError(w, 422, "federated_transfer_failed", "completed revision could not be transferred")
			return
		}
		document, _ := store.Identity()
		peerID := ""
		for _, id := range boundary.InstanceIDs {
			if id != document.InstanceID {
				peerID = id
				break
			}
		}
		revision := federation.CollaborationEvent{ID: run.ID + "-revision", ContributionID: boundary.ContributionID, Sequence: time.Now().UnixMicro(), Kind: "revision", Actor: document.InstanceID + ":agent:" + run.AgentID, Revision: in.CommitID, CreatedAt: time.Now().UTC().Truncate(time.Microsecond), OriginInstanceID: document.InstanceID, Verification: "verified"}
		version, key, signature, _ := store.SignPayload(collaborationEventBytes(revision))
		revision.DocumentVersion, revision.SigningKeyID, revision.Signature = version, key, signature
		body, _ := json.Marshal(map[string]any{"session_id": r.PathValue("session"), "run_id": run.ID, "initiator": document.InstanceID + ":user:" + run.InitiatorID, "agent": document.InstanceID + ":agent:" + run.AgentID, "summary": completed.Outcome.Summary, "commands": completed.Outcome.Commands, "evidence": completed.Outcome.Checks, "changed_files": completed.Outcome.ChangedFiles, "agent_minutes": in.AgentMinutes, "cost_units": in.CostUnits, "residual_concerns": completed.Outcome.Concerns})
		summary := federation.CollaborationEvent{ID: run.ID + "-summary", ContributionID: boundary.ContributionID, Sequence: revision.Sequence + 1, Kind: "agent_session", Actor: revision.Actor, Revision: in.CommitID, Evidence: body, CreatedAt: revision.CreatedAt, OriginInstanceID: document.InstanceID, Verification: "verified"}
		version, key, signature, _ = store.SignPayload(collaborationEventBytes(summary))
		summary.DocumentVersion, summary.SigningKeyID, summary.Signature = version, key, signature
		if err = sendCollaborationEventEnvelope(store, peerID, revision, base64.RawStdEncoding.EncodeToString(bundle.Bytes())); err != nil {
			writeAPIError(w, 502, "federated_delivery_failed", "revision delivery failed; completion remains local and retryable")
			return
		}
		if err = store.AdvanceContributionRevision(boundary.ContributionID, boundary.SourceRevision, in.CommitID); err != nil {
			writeAPIError(w, 409, "federated_revision_conflict", "contribution advanced concurrently")
			return
		}
		_, _ = credentials.Revoke(run.InitiatorID, credential.ID)
		_, _ = sessionStore.RevokeRunAccess(boundary.SourceRepositoryID, boundary.ContributionID, r.PathValue("session"), run.ID)
		_ = sendCollaborationEvent(store, peerID, summary)
		_, _ = store.AppendCollaborationEvent(revision)
		_, _ = store.AppendCollaborationEvent(summary)
		writeJSON(w, 201, map[string]any{"run": completed, "revision_event": revision, "summary_event": summary})
	})
	// Peer delivery accepts only signed immutable collaboration claims. The
	// receiving instance decides what it may retain and never treats the actor
	// string as a local principal.
	mux.HandleFunc("POST /federation/contributions/{contribution}/events", func(w http.ResponseWriter, r *http.Request) {
		var in signedCollaborationEvent
		if decodeJSONLimit(r, &in, 300<<10) != nil || in.Event.ContributionID != r.PathValue("contribution") {
			writeAPIError(w, 400, "invalid_federated_event", "invalid collaboration event")
			return
		}
		peer, err := store.Get(in.Event.OriginInstanceID)
		if err != nil || peer.Status != "trusted" || !contains(peer.Document.Capabilities, "repository-contribution.v1") {
			writeAPIError(w, 403, "federated_source_untrusted", "event origin is not trusted")
			return
		}
		if !store.ContributionAllows(in.Event.ContributionID, in.Event.OriginInstanceID) {
			writeAPIError(w, 403, "federated_contribution_forbidden", "event origin is not authorized for this contribution")
			return
		}
		if federation.VerifyPayload(collaborationEventBytes(in.Event), in.Event.DocumentVersion, in.Event.SigningKeyID, in.Event.Signature, peer.Document) != nil {
			writeAPIError(w, 422, "invalid_federated_signature", "collaboration event signature is invalid")
			return
		}
		in.Event.Verification = "verified"
		if in.Event.Kind == "revision" {
			boundary, boundaryErr := store.Contribution(in.Event.ContributionID)
			if boundaryErr != nil || boundary.TargetRepositoryID == "" || in.Bundle == "" {
				writeAPIError(w, 422, "invalid_federated_revision", "revision event requires its exact bounded transfer")
				return
			}
			source, cleanup, openErr := openTransfer(in.Bundle)
			if openErr != nil {
				writeAPIError(w, 422, "invalid_federated_revision", "revision transfer is invalid")
				return
			}
			defer cleanup()
			if _, openErr = pullStore.AdoptFederatedRevision(boundary.TargetRepositoryID, in.Event.ContributionID, boundary.SourceRevision, in.Event.Revision, source); openErr != nil {
				writeAPIError(w, 409, "federated_revision_conflict", "revision is not an authorized descendant of the current contribution")
				return
			}
			if openErr = store.AdvanceContributionRevision(in.Event.ContributionID, boundary.SourceRevision, in.Event.Revision); openErr != nil {
				writeAPIError(w, 409, "federated_revision_conflict", "contribution advanced concurrently")
				return
			}
		}
		retained, err := store.AppendCollaborationEvent(in.Event)
		if errors.Is(err, federation.ErrConflict) {
			writeAPIError(w, 409, "federated_event_conflict", "event id was already used for different content")
			return
		}
		if err != nil {
			writeAPIError(w, 422, "invalid_federated_event", "event could not be retained")
			return
		}
		writeJSON(w, 201, retained)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull}/federation-events", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoryStore, credentials, r.PathValue("id")); !ok {
			return
		}
		pull, err := pullStore.Get(r.PathValue("id"), r.PathValue("pull"))
		if err != nil || pull.FederatedContributionID == "" {
			writeAPIError(w, 404, "federated_contribution_not_found", "federated contribution not found")
			return
		}
		items, err := store.ListCollaborationEvents(pull.FederatedContributionID, pull.SourceCommitID)
		if err != nil {
			writeAPIError(w, 503, "federated_collaboration_unavailable", "collaboration history is unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"events": items})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull}/federation-events", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoryStore, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		pull, err := pullStore.Get(r.PathValue("id"), r.PathValue("pull"))
		if err != nil || pull.FederatedContributionID == "" {
			writeAPIError(w, 404, "federated_contribution_not_found", "federated contribution not found")
			return
		}
		var in struct {
			ID       string          `json:"id"`
			Sequence int64           `json:"sequence"`
			Kind     string          `json:"kind"`
			Body     string          `json:"body"`
			Decision string          `json:"decision"`
			State    string          `json:"state"`
			Revision string          `json:"revision"`
			Evidence json.RawMessage `json:"evidence"`
		}
		if decodeJSONLimit(r, &in, 280<<10) != nil {
			writeAPIError(w, 400, "invalid_federated_event", "invalid collaboration event")
			return
		}
		document, _ := store.Identity()
		if in.ID == "" {
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			in.ID = hex.EncodeToString(b)
		}
		if in.Sequence < 1 {
			in.Sequence = time.Now().UTC().UnixMicro()
		}
		if in.Revision == "" && in.Kind != "comment" && in.Kind != "closure" {
			in.Revision = pull.SourceCommitID
		}
		event := federation.CollaborationEvent{ID: in.ID, ContributionID: pull.FederatedContributionID, Sequence: in.Sequence, Kind: in.Kind, Actor: document.InstanceID + ":user:" + actor.UserID, Revision: in.Revision, Body: in.Body, Decision: in.Decision, State: in.State, Evidence: in.Evidence, CreatedAt: time.Now().UTC().Truncate(time.Microsecond), OriginInstanceID: document.InstanceID, Verification: "verified"}
		version, key, signature, signErr := store.SignPayload(collaborationEventBytes(event))
		if signErr != nil {
			writeAPIError(w, 503, "federation_unavailable", "event could not be signed")
			return
		}
		event.DocumentVersion, event.SigningKeyID, event.Signature = version, key, signature
		if _, err = store.AppendCollaborationEvent(event); err != nil {
			writeAPIError(w, 422, "invalid_federated_event", "event could not be retained")
			return
		}
		peerID := strings.SplitN(pull.FederatedAuthor, ":", 2)[0]
		if err = sendCollaborationEvent(store, peerID, event); err != nil {
			_ = store.RetainCollaborationDelivery(peerID, event, err.Error())
			w.Header().Set("Vivarium-Federation-Delivery", "pending")
			writeJSON(w, 202, map[string]any{"event": event, "delivery": "pending", "error": err.Error()})
			return
		}
		_ = store.CompleteCollaborationDelivery(peerID, event.ID)
		writeJSON(w, 201, event)
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

func sendCollaborationEvent(store *federation.Store, peerID string, event federation.CollaborationEvent) error {
	return sendCollaborationEventEnvelope(store, peerID, event, "")
}

func sendCollaborationEventEnvelope(store *federation.Store, peerID string, event federation.CollaborationEvent, bundle string) error {
	peer, err := store.Get(peerID)
	if err != nil || peer.Status != "trusted" {
		return federation.ErrConflict
	}
	u, err := url.Parse(peer.DiscoveryURL)
	if err != nil {
		return err
	}
	u.Path = "/federation/contributions/" + url.PathEscape(event.ContributionID) + "/events"
	u.RawQuery = ""
	body, _ := json.Marshal(signedCollaborationEvent{Event: event, Bundle: bundle})
	transport := &http.Transport{Proxy: nil, DialContext: safeFederationDialer(u.Scheme == "http" && isLoopbackHost(u.Hostname()))}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Post(u.String(), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("peer rejected event (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func recoverFederationDeliveries(store *federation.Store) error {
	items, err := store.PendingCollaborationDeliveries()
	if err != nil {
		return err
	}
	for _, item := range items {
		if err = sendCollaborationEvent(store, item.PeerID, item.Event); err != nil {
			_ = store.RetainCollaborationDelivery(item.PeerID, item.Event, err.Error())
			continue
		}
		if err = store.CompleteCollaborationDelivery(item.PeerID, item.Event.ID); err != nil {
			return err
		}
	}
	return nil
}

// finalizeFederatedMerge freezes the locally verified collaboration record in
// a signed receipt. Retention and outbox publication happen before network I/O,
// so accepted history never depends on the source instance remaining online.
func finalizeFederatedMerge(store *federation.Store, pull pullrequests.PullRequest) error {
	if store == nil || pull.FederatedContributionID == "" || pull.Status != pullrequests.Merged || pull.MergeCommitID == nil || pull.MergedAt == nil || pull.MergedBy == nil {
		return nil
	}
	boundary, err := store.Contribution(pull.FederatedContributionID)
	if err != nil {
		return err
	}
	events, err := store.ListCollaborationEvents(pull.FederatedContributionID, pull.SourceCommitID)
	if err != nil {
		return err
	}
	type retainedClaim struct {
		ID               string `json:"id"`
		Kind             string `json:"kind"`
		OriginInstanceID string `json:"origin_instance_id"`
		SHA256           string `json:"sha256"`
	}
	verified := make([]retainedClaim, 0, len(events))
	for _, event := range events {
		// Revisionless discussion/closure remains contribution provenance. Claims
		// bound to a superseded revision are retained in history but cannot attest
		// to the exact candidate accepted by this merge.
		if event.Kind != "receipt" && !event.Stale {
			raw, _ := json.Marshal(event)
			digest := sha256.Sum256(raw)
			verified = append(verified, retainedClaim{ID: event.ID, Kind: event.Kind, OriginInstanceID: event.OriginInstanceID, SHA256: hex.EncodeToString(digest[:])})
		}
	}
	document, err := store.Identity()
	if err != nil {
		return err
	}
	peerID := ""
	for _, id := range boundary.InstanceIDs {
		if id != document.InstanceID {
			peerID = id
			break
		}
	}
	if peerID == "" {
		return federation.ErrInvalid
	}
	evidence, err := json.Marshal(map[string]any{
		"repository_id": pull.RepositoryID, "pull_request_id": pull.ID,
		"source_instance_id": peerID, "source_actor": pull.FederatedAuthor,
		"source_revision": pull.SourceCommitID, "target_branch": pull.TargetBranch,
		"merge_commit_id": *pull.MergeCommitID, "merged_by": *pull.MergedBy,
		"merged_at": pull.MergedAt, "verified_collaboration": verified,
	})
	if err != nil {
		return err
	}
	receipt := federation.CollaborationEvent{
		ID: "merge-" + pull.ID, ContributionID: pull.FederatedContributionID,
		Sequence: pull.MergedAt.UnixMicro(), Kind: "receipt",
		Actor:    document.InstanceID + ":user:" + *pull.MergedBy,
		Revision: *pull.MergeCommitID, Evidence: evidence, CreatedAt: pull.MergedAt.UTC().Truncate(time.Microsecond),
		OriginInstanceID: document.InstanceID, Verification: "verified",
	}
	if retained, getErr := store.CollaborationEvent(receipt.ContributionID, receipt.OriginInstanceID, receipt.ID); getErr == nil {
		receipt = retained
	} else if !errors.Is(getErr, federation.ErrNotFound) {
		return getErr
	} else {
		version, key, signature, signErr := store.SignPayload(collaborationEventBytes(receipt))
		if signErr != nil {
			return signErr
		}
		receipt.DocumentVersion, receipt.SigningKeyID, receipt.Signature = version, key, signature
		if _, err = store.AppendCollaborationEvent(receipt); err != nil {
			return err
		}
	}
	// Always publish an outbox record first. A crash, outage, or trust change can
	// delay delivery, but cannot erase the upstream's accepted evidence.
	if err = store.RetainCollaborationDelivery(peerID, receipt, "delivery pending"); err != nil {
		return err
	}
	if err = sendCollaborationEvent(store, peerID, receipt); err != nil {
		_ = store.RetainCollaborationDelivery(peerID, receipt, err.Error())
		return nil
	}
	return store.CompleteCollaborationDelivery(peerID, receipt.ID)
}

func startFederationDeliveryRecovery(store *federation.Store) {
	if store == nil {
		return
	}
	recover := func() {
		if err := recoverFederationDeliveries(store); err != nil {
			log.Printf("recover federation deliveries: %v", err)
		}
	}
	recover()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			recover()
		}
	}()
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
