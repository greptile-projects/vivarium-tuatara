package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentprojects"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/collaborationworkflows"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type collaborationWorkflowSourceInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Revision        string `json:"revision"`
	Path            string `json:"path"`
}

var exactCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

func registerCollaborationWorkflowRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, workflows *collaborationworkflows.Store, agents *agentprojects.Store) {
	mux.HandleFunc("GET /repositories/{id}/collaboration-workflows", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := workflows.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "collaboration_workflows_unavailable", "collaboration workflows could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"workflows": out})
	})
	mux.HandleFunc("GET /repositories/{id}/collaboration-workflows/{workflow_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := workflows.Get(r.PathValue("workflow_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "collaboration_workflow_not_found", "collaboration workflow not found")
			return
		}
		writeJSON(w, 200, out)
	})
	handle := func(activate, revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in collaborationWorkflowSourceInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "exact revision and configuration path are required")
				return
			}
			definition, source, err := readWorkflowDefinition(git, r.PathValue("id"), in.Revision, in.Path)
			if err != nil {
				writeAPIError(w, 400, "invalid_workflow_source", err.Error())
				return
			}
			check := func(inv collaborationworkflows.Invocation) (bool, string) {
				switch inv.Kind {
				case "platform_action":
					if !stringIn(inv.Action, "create_issue", "comment", "request_review", "dispatch_check", "update_project", "notify") {
						return false, "platform action is not in the permitted workflow action set"
					}
				case "component":
					if !stringIn(inv.Component, "repository-checks", "review-gates", "release-readiness", "project-notifications") {
						return false, "reusable component is not available to repository workflows"
					}
				case "agent":
					if agents == nil {
						return false, "approved agent project resolver is unavailable"
					}
					project, e := agents.Get(inv.AgentID)
					if e != nil || project.RepositoryID != r.PathValue("id") || len(project.Revisions) == 0 || len(project.Diagnostics) > 0 {
						return false, "agent must resolve to a gap-free reviewed project in this repository"
					}
				case "workflow":
					target, e := workflows.Get(inv.WorkflowID)
					if e != nil || target.RepositoryID != r.PathValue("id") || target.Status != "active" {
						return false, "reusable workflow is not an active readable workflow in this repository"
					}
				}
				return true, ""
			}
			preview := workflows.Preview(r.PathValue("id"), definition, source, check)
			if !activate {
				writeJSON(w, 200, preview)
				return
			}
			owners := append([]string{}, definition.OwnerIDs...)
			for _, step := range definition.Steps {
				owners = append(owners, step.OwnerIDs...)
			}
			var out collaborationworkflows.Workflow
			err = catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error {
				if revise {
					current, e := workflows.Get(r.PathValue("workflow_id"))
					if e != nil || current.RepositoryID != r.PathValue("id") {
						return collaborationworkflows.ErrNotFound
					}
					out, e = workflows.Revise(current.ID, in.ExpectedVersion, actor.UserID, preview)
					return e
				}
				out, err = workflows.Create(r.PathValue("id"), actor.UserID, preview)
				return err
			})
			writeCollaborationWorkflow(w, out, err, revise)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/collaboration-workflows/preview", handle(false, false))
	mux.HandleFunc("POST /repositories/{id}/collaboration-workflows", handle(true, false))
	mux.HandleFunc("POST /repositories/{id}/collaboration-workflows/{workflow_id}/revisions", handle(true, true))
}

func readWorkflowDefinition(git *storage.Store, repo, revision, file string) (collaborationworkflows.Definition, collaborationworkflows.Source, error) {
	var d collaborationworkflows.Definition
	var src collaborationworkflows.Source
	if git == nil || !exactCommit.MatchString(revision) || file == "" || strings.HasPrefix(file, "/") || strings.Contains(file, "..") {
		return d, src, errors.New("source must name an exact 40-character commit and repository-relative path")
	}
	r, err := git.Open(repo)
	if err != nil {
		return d, src, errors.New("repository source is inaccessible")
	}
	branches, err := exec.Command("git", "--git-dir="+r.Path(), "for-each-ref", "--format=%(refname)", "refs/heads").Output()
	if err != nil {
		return d, src, errors.New("repository branches could not be resolved")
	}
	reachable := false
	for _, branch := range strings.Fields(string(branches)) {
		if strings.HasPrefix(branch, "refs/heads/vivarium-security/") {
			continue
		}
		if exec.Command("git", "--git-dir="+r.Path(), "merge-base", "--is-ancestor", revision, branch).Run() == nil {
			reachable = true
			break
		}
	}
	if !reachable {
		return d, src, errors.New("configuration commit must be reachable from a non-security branch")
	}
	b, err := exec.Command("git", "--git-dir="+r.Path(), "show", revision+":"+file).Output()
	if err != nil {
		return d, src, errors.New("configuration file is missing or is not a readable blob at that commit")
	}
	if len(b) > 256*1024 || json.Unmarshal(b, &d) != nil {
		return d, src, errors.New("configuration must be valid workflow JSON no larger than 256 KiB")
	}
	h := sha256.Sum256(b)
	src = collaborationworkflows.Source{Revision: revision, Path: file, SHA256: hex.EncodeToString(h[:])}
	return d, src, nil
}

func stringIn(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
func writeCollaborationWorkflow(w http.ResponseWriter, out collaborationworkflows.Workflow, err error, revised bool) {
	status := 201
	if revised {
		status = 200
	}
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, collaborationworkflows.ErrInvalid):
		writeAPIError(w, 400, "workflow_activation_blocked", "workflow diagnostics must be resolved before activation")
	case errors.Is(err, collaborationworkflows.ErrConflict):
		writeAPIError(w, 409, "collaboration_workflow_conflict", "the workflow changed; reload before activating a revision")
	case errors.Is(err, collaborationworkflows.ErrNotFound):
		writeAPIError(w, 404, "collaboration_workflow_not_found", "collaboration workflow not found")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "collaboration_workflow_forbidden", "workflow and step owners must be current repository participants")
	default:
		log.Printf("collaboration workflow storage: %v", err)
		writeAPIError(w, 500, "collaboration_workflows_unavailable", "collaboration workflow could not be persisted")
	}
}
