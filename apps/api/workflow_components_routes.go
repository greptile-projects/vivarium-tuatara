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
	"github.com/greptile-projects/vivarium-tuatara/apps/api/federation"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workflowcomponents"
)

func registerWorkflowComponentRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, packageStore *packages.Store, peers *federation.Store, components *workflowcomponents.Store) {
	mux.HandleFunc("GET /workflow-components", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		items, err := components.List()
		if err != nil {
			writeAPIError(w, 500, "workflow_components_unavailable", "components could not be read")
			return
		}
		type projection struct {
			workflowcomponents.Component
			Diagnostics []string `json:"diagnostics"`
		}
		visible := []projection{}
		for _, item := range items {
			if workflowComponentReadable(catalog, item.Source.RepositoryID, actor.UserID) {
				visible = append(visible, projection{Component: item, Diagnostics: workflowComponentDiagnostics(catalog, packageStore, peers, item)})
			}
		}
		writeJSON(w, 200, map[string]any{"components": visible, "authority_note": "components carry contracts and evidence only; consumers grant only pull-reviewed local mappings"})
	})
	mux.HandleFunc("POST /repositories/{id}/workflow-components", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "workflow_component_publish_forbidden", "only the repository owner may attest a component")
			return
		}
		var in struct {
			Revision       string `json:"revision"`
			Path           string `json:"path"`
			PackageName    string `json:"package_name"`
			PackageVersion string `json:"package_version"`
			Boundary       string `json:"boundary"`
			PeerID         string `json:"peer_id"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "exact source and package provenance are required")
			return
		}
		d, src, err := readComponentDefinition(git, r.PathValue("id"), in.Revision, in.Path)
		if err != nil {
			writeAPIError(w, 422, "invalid_workflow_component", err.Error())
			return
		}
		pkg, err := packageStore.Get(in.PackageName, in.PackageVersion)
		if err != nil || pkg.RepositoryID != r.PathValue("id") || pkg.SourceCommit != in.Revision || pkg.Lifecycle != "active" {
			writeAPIError(w, 422, "invalid_component_package", "an active package from the same repository and exact source commit is required")
			return
		}
		if pkg.PublisherID != actor.UserID {
			writeAPIError(w, 422, "component_publisher_changed", "the package attestor and current component publisher must be the same owner")
			return
		}
		if in.Boundary == "federation" {
			if peers == nil {
				writeAPIError(w, 503, "component_peer_unavailable", "federation trust resolver is unavailable")
				return
			}
			peer, e := peers.Get(in.PeerID)
			if e != nil || peer.Status != "trusted" {
				writeAPIError(w, 422, "component_peer_untrusted", "federated provenance requires a currently trusted peer")
				return
			}
		}
		src.PackageName, src.PackageVersion, src.PackageSHA256, src.Boundary, src.PeerID = pkg.Name, pkg.Version, pkg.SHA256, in.Boundary, in.PeerID
		out, err := components.Publish(d, src, actor.UserID)
		if errors.Is(err, workflowcomponents.ErrInvalid) {
			writeAPIError(w, 422, "invalid_workflow_component", "typed contracts, capabilities, data terms, passing tests, compatibility, and support are required")
			return
		}
		if errors.Is(err, workflowcomponents.ErrConflict) {
			writeAPIError(w, 409, "workflow_component_exists", "this immutable version already has different evidence")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "workflow_component_publish_failed", "component could not be persisted")
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("GET /repositories/{id}/workflow-component-installations", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := components.ListInstallations(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "component_installations_unavailable", "installations could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"installations": out})
	})
	mux.HandleFunc("POST /repositories/{id}/workflow-component-installations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Name            string                       `json:"name"`
			ComponentID     string                       `json:"component_id"`
			PullID          string                       `json:"pull_id"`
			ExpectedVersion int                          `json:"expected_version"`
			Mappings        []workflowcomponents.Mapping `json:"mappings"`
			Configuration   map[string]any               `json:"configuration"`
			AcceptedDataUse []string                     `json:"accepted_data_use"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a component, ordinary pull, mappings, and configuration are required")
			return
		}
		p, err := pulls.Get(r.PathValue("id"), in.PullID)
		if err != nil || p.Status != pullrequests.Open {
			writeAPIError(w, 422, "invalid_component_pull", "installation must be reviewed through a current open pull")
			return
		}
		c, err := components.Get(in.ComponentID)
		if err != nil {
			writeAPIError(w, 404, "workflow_component_not_found", "component not found")
			return
		}
		if !workflowComponentReadable(catalog, c.Source.RepositoryID, actor.UserID) {
			writeAPIError(w, 404, "workflow_component_not_found", "component not found")
			return
		}
		pkg, err := packageStore.Get(c.Source.PackageName, c.Source.PackageVersion)
		if err != nil || pkg.SHA256 != c.Source.PackageSHA256 || pkg.Lifecycle != "active" {
			writeAPIError(w, 409, "component_trust_changed", "publisher package is unavailable, replaced, or no longer trusted")
			return
		}
		out, err := components.Install(r.PathValue("id"), in.Name, actor.UserID, p.ID, p.SourceCommitID, c, in.Mappings, in.Configuration, in.AcceptedDataUse, in.ExpectedVersion)
		if errors.Is(err, workflowcomponents.ErrConflict) {
			writeAPIError(w, 409, "stale_component_installation", "installation version changed")
			return
		}
		if errors.Is(err, workflowcomponents.ErrInvalid) {
			writeAPIError(w, 422, "invalid_component_installation", "map every requested capability locally, accept exact data terms, and omit credentials")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "component_installation_failed", "installation could not be persisted")
			return
		}
		writeJSON(w, 201, out)
	})
}

func workflowComponentReadable(catalog *repositories.Store, repositoryID, actorID string) bool {
	repo, err := catalog.GetByID(repositoryID)
	if err != nil {
		return false
	}
	if repo.Visibility == repositories.Public || repo.OwnerID == actorID {
		return true
	}
	ok, err := catalog.HasCollaborator(actorID, repositoryID)
	return err == nil && ok
}

func workflowComponentDiagnostics(catalog *repositories.Store, packageStore *packages.Store, peers *federation.Store, c workflowcomponents.Component) []string {
	out := []string{}
	repo, e := catalog.GetByID(c.Source.RepositoryID)
	if e != nil {
		out = append(out, "publisher repository unavailable")
	} else if repo.OwnerID != c.Attestation.PublisherID {
		out = append(out, "publisher ownership changed since attestation")
	}
	pkg, e := packageStore.Get(c.Source.PackageName, c.Source.PackageVersion)
	if e != nil {
		out = append(out, "pinned package unavailable")
	} else if pkg.PublisherID != c.Attestation.PublisherID {
		out = append(out, "package publisher differs from component attestor")
	} else if pkg.Lifecycle != "active" {
		out = append(out, "pinned package is "+pkg.Lifecycle+": "+pkg.LifecycleWarning)
	}
	if c.Source.Boundary == "federation" && peers == nil {
		out = append(out, "publishing peer unavailable")
	} else if c.Source.Boundary == "federation" {
		peer, e := peers.Get(c.Source.PeerID)
		if e != nil {
			out = append(out, "publishing peer unavailable")
		} else if peer.Status != "trusted" {
			out = append(out, "publishing peer trust is "+peer.Status)
		}
	}
	if c.Definition.Compatibility.Breaking {
		out = append(out, "version declares a breaking upgrade; migration: "+c.Definition.Compatibility.Migration)
	}
	return out
}

func readComponentDefinition(git *storage.Store, repo, revision, path string) (workflowcomponents.Definition, workflowcomponents.Source, error) {
	var d workflowcomponents.Definition
	var src workflowcomponents.Source
	if git == nil || !exactCommit.MatchString(revision) || path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return d, src, errors.New("source must name an exact commit and repository-relative JSON path")
	}
	r, e := git.Open(repo)
	if e != nil {
		return d, src, errors.New("repository source is inaccessible")
	}
	branches, e := exec.Command("git", "--git-dir="+r.Path(), "for-each-ref", "--format=%(refname)", "refs/heads").Output()
	if e != nil {
		return d, src, errors.New("repository branches could not be resolved")
	}
	reachable := false
	for _, b := range strings.Fields(string(branches)) {
		if !strings.HasPrefix(b, "refs/heads/vivarium-security/") && exec.Command("git", "--git-dir="+r.Path(), "merge-base", "--is-ancestor", revision, b).Run() == nil {
			reachable = true
			break
		}
	}
	if !reachable {
		return d, src, errors.New("source commit must remain reachable from a visible branch")
	}
	body, e := exec.Command("git", "--git-dir="+r.Path(), "show", revision+":"+path).Output()
	if e != nil || len(body) > 256*1024 || json.Unmarshal(body, &d) != nil {
		return d, src, errors.New("component definition must be a readable JSON blob no larger than 256 KiB")
	}
	sum := sha256.Sum256(body)
	src = workflowcomponents.Source{RepositoryID: repo, Revision: revision, Path: path, SHA256: hex.EncodeToString(sum[:])}
	return d, src, nil
}
