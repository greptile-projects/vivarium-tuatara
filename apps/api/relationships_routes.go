package main

import (
	"errors"
	"net/http"
	"sort"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type relationshipRepository struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	OwnerID    string `json:"owner_id"`
	Visibility string `json:"visibility"`
}
type interfaceNode struct {
	relationships.Interface
	Stale       bool   `json:"stale"`
	StaleReason string `json:"stale_reason,omitempty"`
}
type dependencyEdge struct {
	relationships.Dependency
	ResolvedInterfaceID string `json:"resolved_interface_id,omitempty"`
	ResolvedVersion     string `json:"resolved_version,omitempty"`
	State               string `json:"state"`
	Reason              string `json:"reason,omitempty"`
}

func registerRelationshipRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, relationStore *relationships.Store, credentials *auth.Store) {
	mux.HandleFunc("GET /repositories/{id}/relationships", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if !authenticated {
			optionalActor, present, authOK := authenticateOptionalRequest(w, r, credentials, "repositories:read", false)
			if !authOK {
				return
			}
			if present {
				actor, authenticated = optionalActor, true
			}
		}
		readable := func(id string) (repositories.Repository, bool) {
			repo, err := repos.GetByID(id)
			if err != nil {
				return repo, false
			}
			if repo.Visibility == repositories.Public {
				return repo, true
			}
			if !authenticated {
				return repo, false
			}
			collaborator, _ := repos.HasCollaborator(actor.UserID, id)
			return repo, actor.UserID == repo.OwnerID || collaborator
		}
		ids, err := relationStore.ListRepositoryIDs()
		if err != nil {
			writeAPIError(w, 500, "relationship_read_failed", "relationship graph could not be read")
			return
		}
		foundRoot := false
		for _, id := range ids {
			foundRoot = foundRoot || id == r.PathValue("id")
		}
		if !foundRoot {
			ids = append(ids, r.PathValue("id"))
		}
		repositoryMap := map[string]relationshipRepository{}
		interfaces := []interfaceNode{}
		dependencies := []dependencyEdge{}
		for _, id := range ids {
			repo, allowed := readable(id)
			if !allowed {
				continue
			}
			repositoryMap[id] = relationshipRepository{ID: id, Name: repo.Name, OwnerID: repo.OwnerID, Visibility: repo.Visibility}
			values, readErr := relationStore.ListInterfaces(id)
			if readErr != nil {
				err = readErr
				break
			}
			for _, value := range values {
				node := interfaceNode{Interface: value}
				release, releaseErr := releaseStore.Get(id, value.ReleaseID)
				if releaseErr != nil || release.CommitID != value.CommitID || release.Version != value.Version {
					node.Stale = true
					node.StaleReason = "the referenced release is missing or no longer matches the publication"
				}
				interfaces = append(interfaces, node)
			}
		}
		if err != nil {
			writeAPIError(w, 500, "relationship_read_failed", "relationship graph could not be read")
			return
		}
		for _, id := range ids {
			if _, allowed := repositoryMap[id]; !allowed {
				continue
			}
			values, readErr := relationStore.ListDependencies(id)
			if readErr != nil {
				err = readErr
				break
			}
			for _, value := range values {
				edge := dependencyEdge{Dependency: value, State: "unresolved"}
				if _, allowed := repositoryMap[value.ProviderRepositoryID]; !allowed {
					continue
				}
				var match *interfaceNode
				for i := range interfaces {
					candidate := &interfaces[i]
					if candidate.RepositoryID == value.ProviderRepositoryID && candidate.Name == value.InterfaceName && !candidate.Stale && relationships.Satisfies(candidate.Version, value.Constraint) {
						match = candidate
					}
				}
				staleReason := dependencyStaleReason(value, releaseStore, deploymentStore)
				if staleReason != "" {
					edge.State = "stale"
					edge.Reason = staleReason
				} else if match == nil {
					edge.Reason = "no published interface satisfies the compatibility constraint"
				} else {
					edge.State = "resolved"
					edge.ResolvedInterfaceID = match.ID
					edge.ResolvedVersion = match.Version
				}
				dependencies = append(dependencies, edge)
			}
		}
		if err != nil {
			writeAPIError(w, 500, "relationship_read_failed", "relationship graph could not be read")
			return
		}
		connected := map[string]bool{r.PathValue("id"): true}
		for changed := true; changed; {
			changed = false
			for _, edge := range dependencies {
				if connected[edge.RepositoryID] || connected[edge.ProviderRepositoryID] {
					if !connected[edge.RepositoryID] {
						connected[edge.RepositoryID], changed = true, true
					}
					if _, visible := repositoryMap[edge.ProviderRepositoryID]; visible && !connected[edge.ProviderRepositoryID] {
						connected[edge.ProviderRepositoryID], changed = true, true
					}
				}
			}
		}
		filteredInterfaces := interfaces[:0]
		for _, item := range interfaces {
			if connected[item.RepositoryID] {
				filteredInterfaces = append(filteredInterfaces, item)
			}
		}
		interfaces = filteredInterfaces
		filteredDependencies := dependencies[:0]
		for _, item := range dependencies {
			if connected[item.RepositoryID] {
				filteredDependencies = append(filteredDependencies, item)
			}
		}
		dependencies = filteredDependencies
		repositoriesOut := make([]relationshipRepository, 0, len(connected))
		for id := range connected {
			if repo, visible := repositoryMap[id]; visible {
				repositoriesOut = append(repositoriesOut, repo)
			}
		}
		sort.Slice(repositoriesOut, func(i, j int) bool { return repositoriesOut[i].ID < repositoriesOut[j].ID })
		writeJSON(w, 200, map[string]any{"root_repository_id": r.PathValue("id"), "repositories": repositoriesOut, "interfaces": interfaces, "dependencies": dependencies})
	})
	mux.HandleFunc("POST /repositories/{id}/interfaces", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Name      string `json:"name"`
			ReleaseID string `json:"release_id"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		release, err := releaseStore.Get(r.PathValue("id"), input.ReleaseID)
		if err != nil {
			writeAPIError(w, 422, "invalid_interface_release", "release_id must name a release in this repository")
			return
		}
		created, err := relationStore.CreateInterface(relationships.Interface{RepositoryID: r.PathValue("id"), Name: input.Name, Version: release.Version, ReleaseID: release.ID, CommitID: release.CommitID, PublishedBy: actor.UserID})
		if errors.Is(err, relationships.ErrInvalid) {
			writeAPIError(w, 422, "invalid_interface", "interface name or release version is invalid")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "interface_create_failed", "interface could not be published")
			return
		}
		w.Header().Set("Location", "/repositories/"+created.RepositoryID+"/relationships")
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("POST /repositories/{id}/dependencies", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input relationships.Dependency
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		repository, err := git.Open(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		if _, err = repository.ReadCommit(storage.ObjectID(input.CommitID)); err != nil {
			writeAPIError(w, 422, "invalid_dependency_commit", "commit_id must name a verified commit in this repository")
			return
		}
		if input.ReleaseID != "" {
			release, e := releaseStore.Get(r.PathValue("id"), input.ReleaseID)
			if e != nil || release.CommitID != input.CommitID {
				writeAPIError(w, 422, "invalid_dependency_release", "release_id must match the exact consumer commit")
				return
			}
		}
		if input.EnvironmentID != "" {
			environment, e := deploymentStore.GetEnvironment(r.PathValue("id"), input.EnvironmentID)
			if e != nil || environment.RepositoryID != r.PathValue("id") {
				writeAPIError(w, 422, "invalid_dependency_environment", "environment_id must name a consumer environment")
				return
			}
		}
		input.ID = ""
		input.RepositoryID = r.PathValue("id")
		input.DeclaredBy = actor.UserID
		created, err := relationStore.CreateDependency(input)
		if errors.Is(err, relationships.ErrInvalid) {
			writeAPIError(w, 422, "invalid_dependency", "dependency identity or compatibility constraint is invalid")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "dependency_create_failed", "dependency could not be declared")
			return
		}
		w.Header().Set("Location", "/repositories/"+created.RepositoryID+"/relationships")
		writeJSON(w, 201, created)
	})
}

func dependencyStaleReason(value relationships.Dependency, releaseStore *releases.Store, deploymentStore *deployments.Store) string {
	if value.ReleaseID != "" {
		release, err := releaseStore.Get(value.RepositoryID, value.ReleaseID)
		if err != nil || release.CommitID != value.CommitID {
			return "the consumer release is missing or does not match the declared revision"
		}
	}
	if value.EnvironmentID != "" {
		if _, err := deploymentStore.GetEnvironment(value.RepositoryID, value.EnvironmentID); err != nil {
			return "the consumer environment no longer exists"
		}
		promotions, err := deploymentStore.ListPromotions(value.RepositoryID)
		if err != nil {
			return "consumer deployment evidence is unavailable"
		}
		var latest *deployments.Promotion
		for _, p := range promotions {
			if p.EnvironmentID == value.EnvironmentID {
				candidate := p
				latest = &candidate
			}
		}
		if latest == nil || latest.State != "succeeded" || latest.CommitID != value.CommitID || (value.ReleaseID != "" && latest.ReleaseID != value.ReleaseID) {
			return "the environment is not currently running the declared revision"
		}
	}
	return ""
}
