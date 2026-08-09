package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

var (
	errPackageBuildUnverified = errors.New("package build is not currently verified")
	errPackageArtifactStale   = errors.New("package artifact is not from the current attempt")
)

type packageDeploymentEvidence struct {
	deployments.Promotion
	Current bool `json:"current"`
}

func projectPackageDeployments(promotions []deployments.Promotion, releaseIDs map[string]bool) []packageDeploymentEvidence {
	latestSuccessful := map[string]deployments.Promotion{}
	for _, promotion := range promotions {
		if promotion.State != "succeeded" {
			continue
		}
		latest, found := latestSuccessful[promotion.EnvironmentID]
		if !found || promotion.CreationSequence > latest.CreationSequence || (promotion.CreationSequence == latest.CreationSequence && (promotion.CreatedAt.After(latest.CreatedAt) || (promotion.CreatedAt.Equal(latest.CreatedAt) && promotion.ID > latest.ID))) {
			latestSuccessful[promotion.EnvironmentID] = promotion
		}
	}
	result := []packageDeploymentEvidence{}
	for _, promotion := range promotions {
		if !releaseIDs[promotion.ReleaseID] {
			continue
		}
		result = append(result, packageDeploymentEvidence{Promotion: promotion, Current: promotion.State == "succeeded" && latestSuccessful[promotion.EnvironmentID].ID == promotion.ID})
	}
	return result
}

func registerPackageRoutes(mux *http.ServeMux, gitStore *storage.Store, repositoryStore *repositories.Store, proposalStore *proposals.Store, releaseStore *releases.Store, buildStore *checkruns.Store, deploymentStore *deployments.Store, packageStore *packages.Store, authStore *auth.Store, activityStore *activities.Store) {
	mux.HandleFunc("POST /repositories/{id}/releases/{release_id}/packages", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositoryStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, http.StatusForbidden, "package_publish_forbidden", "only the repository owner can publish packages")
			return
		}
		var input struct {
			Name          string                `json:"name"`
			Version       string                `json:"version"`
			BuildID       string                `json:"build_id"`
			ArtifactID    string                `json:"artifact_id"`
			Platform      packages.Platform     `json:"platform"`
			Dependencies  []packages.Dependency `json:"dependencies"`
			Visibility    string                `json:"visibility"`
			Summary       string                `json:"summary"`
			Documentation string                `json:"documentation"`
			License       string                `json:"license"`
			Support       string                `json:"support"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		release, err := releaseStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if errors.Is(err, releases.ErrNotFound) {
			writeAPIError(w, 404, "release_not_found", "release candidate not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "package_publish_failed", "release evidence could not be read")
			return
		}
		build, err := buildStore.Get(release.RepositoryID, release.ID, input.BuildID)
		if errors.Is(err, checkruns.ErrNotFound) {
			writeAPIError(w, 422, "invalid_package_build", "build_id must name release build evidence")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "package_publish_failed", "build evidence could not be read")
			return
		}
		var created packages.Version
		err = buildStore.WithCurrentArtifact(release.RepositoryID, release.ID, build.ID, input.ArtifactID, func(current checkruns.Run, artifact checkruns.Artifact, artifactFile *os.File) error {
			if current.CommitID != release.CommitID || current.State != "succeeded" {
				return errPackageBuildUnverified
			}
			latestAttempt := 0
			for _, attempt := range current.Attempts {
				if attempt.Number > latestAttempt {
					latestAttempt = attempt.Number
				}
			}
			if artifact.Attempt != latestAttempt {
				return errPackageArtifactStale
			}
			var publishErr error
			created, publishErr = packageStore.Publish(packages.Version{Name: input.Name, Version: input.Version, RepositoryID: release.RepositoryID, ReleaseID: release.ID, SourceCommit: release.CommitID, BuildID: current.ID, BuildAttestation: packages.BuildAttestation{Step: current.Definition.Name, Image: current.Definition.Image, Command: current.Definition.Command, Attempt: latestAttempt, State: current.State}, ArtifactID: artifact.ID, ArtifactPath: artifact.Path, ContentType: artifact.ContentType, Size: artifact.Size, SHA256: artifact.SHA256, Platform: input.Platform, Dependencies: input.Dependencies, Summary: strings.TrimSpace(input.Summary), Documentation: strings.TrimSpace(input.Documentation), License: strings.TrimSpace(input.License), Support: strings.TrimSpace(input.Support), PublisherID: actor.UserID, Visibility: input.Visibility}, artifactFile)
			return publishErr
		})
		switch {
		case errors.Is(err, checkruns.ErrNotFound):
			writeAPIError(w, 422, "invalid_package_artifact", "artifact_id must name output from the selected build")
		case errors.Is(err, errPackageBuildUnverified):
			writeAPIError(w, 409, "package_build_unverified", "the selected build must have succeeded for the exact release commit")
		case errors.Is(err, errPackageArtifactStale):
			writeAPIError(w, 409, "package_artifact_stale", "the selected artifact must come from the successful current build attempt")
		case errors.Is(err, packages.ErrVersionExists):
			writeAPIError(w, 409, "package_version_exists", "this package version already exists")
		case errors.Is(err, packages.ErrIdentityConflict):
			writeAPIError(w, 409, "package_identity_conflict", "this package identity belongs to another repository")
		case errors.Is(err, packages.ErrInvalid):
			writeAPIError(w, 422, "invalid_package", "package identity, version, platform, dependencies, or visibility is invalid")
		case errors.Is(err, packages.ErrChecksum):
			writeAPIError(w, 409, "package_artifact_changed", "artifact bytes no longer match their build checksum")
		case errors.Is(err, packages.ErrAlreadyPublished):
			w.Header().Set("Location", "/packages/"+created.Name+"/versions/"+created.Version)
			writeJSON(w, http.StatusOK, created)
		case errors.Is(err, packages.ErrDurabilityUncertain):
			w.Header().Set("Location", "/packages/"+created.Name+"/versions/"+created.Version)
			w.Header().Set("Vivarium-Durability", "uncertain")
			writeJSON(w, http.StatusAccepted, created)
		case err != nil:
			writeAPIError(w, 500, "package_publish_failed", "the package could not be published atomically")
		default:
			w.Header().Set("Location", "/packages/"+created.Name+"/versions/"+created.Version)
			writeJSON(w, 201, created)
		}
	})
	canRead := func(item packages.Version, actor auth.Credential, authenticated bool) bool {
		if item.Visibility == "public" {
			return true
		}
		if !authenticated {
			return false
		}
		for _, scope := range actor.Scopes {
			if scope == "packages:read" {
				return containsString(actor.PackageNames, item.Name)
			}
		}
		repository, err := repositoryStore.GetByID(item.RepositoryID)
		if err != nil {
			return false
		}
		allowed, _ := repositoryStore.HasCollaborator(actor.UserID, item.RepositoryID)
		return repository.OwnerID == actor.UserID || allowed
	}
	optionalPackageActor := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool, bool) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if header == "" || header == "Bearer" {
			return auth.Credential{}, false, true
		}
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok {
			writeAPIError(w, 401, "authentication_required", "a valid bearer credential is required")
			return auth.Credential{}, false, false
		}
		if actor, err := authStore.Authenticate(token, "packages:read"); err == nil {
			return actor, true, true
		}
		actor, err := authStore.Authenticate(token, "repositories:read")
		if err != nil {
			writeAPIError(w, 401, "authentication_required", "a valid bearer credential is required")
			return auth.Credential{}, false, false
		}
		return actor, true, true
	}
	projectInventory := func(inventory packages.Inventory) map[string]any {
		result := map[string]any{"inventory": inventory, "current": false, "releases": []releases.Candidate{}, "builds": []checkruns.Run{}, "deployments": []packageDeploymentEvidence{}}
		repository, err := repositoryStore.GetByID(inventory.RepositoryID)
		if err == nil {
			if repo, openErr := gitStore.Open(repository.ID); openErr == nil {
				if ref, readErr := repo.ReadReference("refs/heads/" + repository.DefaultBranch); readErr == nil {
					result["current"] = ref.Target == inventory.CommitID
				}
			}
		}
		releaseItems, _ := releaseStore.List(inventory.RepositoryID)
		matchingReleases := []releases.Candidate{}
		matchingBuilds := []checkruns.Run{}
		releaseIDs := map[string]bool{}
		for _, release := range releaseItems {
			if release.CommitID != inventory.CommitID {
				continue
			}
			matchingReleases = append(matchingReleases, release)
			releaseIDs[release.ID] = true
			runs, _ := buildStore.List(inventory.RepositoryID, release.ID)
			matchingBuilds = append(matchingBuilds, runs...)
		}
		matchingDeployments := []packageDeploymentEvidence{}
		if deploymentStore != nil {
			promotions, _ := deploymentStore.ListPromotions(inventory.RepositoryID)
			matchingDeployments = projectPackageDeployments(promotions, releaseIDs)
		}
		result["releases"], result["builds"], result["deployments"] = matchingReleases, matchingBuilds, matchingDeployments
		return result
	}
	mux.HandleFunc("POST /repositories/{id}/dependency-inventories", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoryStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input struct {
			CommitID string `json:"commit_id"`
		}
		if decodeJSON(r, &input) != nil || len(input.CommitID) != 40 {
			writeAPIError(w, 422, "invalid_dependency_inventory", "commit_id must identify a verified repository commit")
			return
		}
		repo, err := gitStore.Open(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "dependency_inventory_failed", "repository storage is unavailable")
			return
		}
		commit, err := repo.ReadCommit(storage.ObjectID(input.CommitID))
		if err != nil {
			writeAPIError(w, 422, "invalid_dependency_inventory", "commit_id must identify a verified repository commit")
			return
		}
		entry, err := resolvePath(repo, commit.Tree, packages.InventoryConfigPath)
		if err != nil || entry.Type != storage.BlobObject {
			writeAPIError(w, 422, "dependency_manifest_missing", "the commit must contain .vivarium/packages.json")
			return
		}
		object, err := repo.ReadObject(entry.ID)
		if err != nil || object.Size > 1<<20 {
			writeAPIError(w, 422, "invalid_dependency_manifest", "the dependency manifest must be readable and no larger than 1 MiB")
			return
		}
		var config packages.InventoryConfig
		if json.Unmarshal(object.Content, &config) != nil || config.Version != 1 || len(config.Dependencies) == 0 || len(config.Dependencies) > 500 || len(config.Lock) > 2000 {
			writeAPIError(w, 422, "invalid_dependency_manifest", "the dependency manifest and exact lock state are invalid")
			return
		}
		locked := map[string]string{}
		for _, item := range config.Lock {
			if item.Name == "" || item.Version == "" || locked[item.Name] != "" {
				writeAPIError(w, 422, "invalid_dependency_manifest", "lock entries must be unique exact package versions")
				return
			}
			locked[item.Name] = item.Version
		}
		all, err := packageStore.List()
		if err != nil {
			writeAPIError(w, 500, "dependency_inventory_failed", "package evidence is unavailable")
			return
		}
		visible := map[string]packages.Version{}
		for _, item := range all {
			if canRead(item, actor, true) {
				visible[item.Name+"@"+item.Version] = item
			}
		}
		byName := map[string]*packages.InventoryEntry{}
		var visit func(string, string, []string, bool)
		visit = func(name, constraint string, path []string, direct bool) {
			version := locked[name]
			key := name
			value := byName[key]
			if value == nil {
				value = &packages.InventoryEntry{Name: name, Version: version, Constraint: constraint, Direct: direct, State: "unresolved"}
				byName[key] = value
			}
			value.Direct = value.Direct || direct
			value.Paths = append(value.Paths, strings.Join(path, " > "))
			if version == "" {
				value.ProvenanceGaps = append(value.ProvenanceGaps, "exact lock entry missing")
				return
			}
			published, found := visible[name+"@"+version]
			if !found {
				value.ProvenanceGaps = append(value.ProvenanceGaps, "package version is missing or no longer readable")
				return
			}
			value.PackageID, value.License, value.Support = published.ID, published.License, published.Support
			if value.State != "stale" {
				value.State = "resolved"
			}
			if !versionMatches(version, constraint) {
				value.State = "stale"
				value.ProvenanceGaps = append(value.ProvenanceGaps, "locked version does not satisfy the declared constraint")
			}
			if published.License == "" {
				value.ProvenanceGaps = append(value.ProvenanceGaps, "license not declared")
			}
			if published.Support == "" {
				value.ProvenanceGaps = append(value.ProvenanceGaps, "support contact not declared")
			}
			if len(path) > 100 {
				return
			}
			for _, dependency := range published.Dependencies {
				if !containsString(path, dependency.Name) {
					visit(dependency.Name, dependency.Constraint, append(append([]string{}, path...), dependency.Name), false)
				}
			}
		}
		for _, dependency := range config.Dependencies {
			visit(strings.ToLower(strings.TrimSpace(dependency.Name)), strings.TrimSpace(dependency.Constraint), []string{dependency.Name}, true)
		}
		entries := make([]packages.InventoryEntry, 0, len(byName))
		for _, value := range byName {
			sort.Strings(value.Paths)
			value.Paths = uniquePackageStrings(value.Paths)
			value.ProvenanceGaps = uniquePackageStrings(value.ProvenanceGaps)
			entries = append(entries, *value)
		}
		created, err := packageStore.RecordInventory(packages.Inventory{RepositoryID: r.PathValue("id"), CommitID: input.CommitID, RecordedBy: actor.UserID, Entries: entries})
		if err != nil {
			writeAPIError(w, 422, "invalid_dependency_inventory", "the manifest dependency graph could not be recorded")
			return
		}
		writeJSON(w, 201, projectInventory(created))
	})
	mux.HandleFunc("GET /repositories/{id}/dependency-inventories", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoryStore, authStore, r.PathValue("id")); !ok {
			return
		}
		items, err := packageStore.ListInventories(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "dependency_inventory_failed", "dependency inventories could not be read")
			return
		}
		projected := make([]map[string]any, 0, len(items))
		for _, item := range items {
			projected = append(projected, projectInventory(item))
		}
		writeJSON(w, 200, map[string]any{"inventories": projected})
	})
	mux.HandleFunc("GET /packages/{name}/versions/{version}/consumers", func(w http.ResponseWriter, r *http.Request) {
		item, err := packageStore.Get(r.PathValue("name"), r.PathValue("version"))
		if err != nil {
			writeAPIError(w, 404, "package_not_found", "package version not found")
			return
		}
		actor, authenticated, ok := optionalPackageActor(w, r)
		if !ok {
			return
		}
		if !canRead(item, actor, authenticated) {
			writeAPIError(w, 404, "package_not_found", "package version not found")
			return
		}
		items, err := packageStore.ListConsumers(item.Name, item.Version)
		if err != nil {
			writeAPIError(w, 500, "package_consumers_failed", "package consumers could not be read")
			return
		}
		visible := []map[string]any{}
		for _, inventory := range items {
			repository, getErr := repositoryStore.GetByID(inventory.RepositoryID)
			if getErr != nil {
				continue
			}
			allowed := repository.Visibility == repositories.Public
			if authenticated {
				collaborator, _ := repositoryStore.HasCollaborator(actor.UserID, repository.ID)
				allowed = allowed || repository.OwnerID == actor.UserID || collaborator
			}
			if allowed {
				projected := projectInventory(inventory)
				updates, _ := packageStore.ListUpdates(inventory.RepositoryID)
				for _, update := range updates {
					if update.PackageName == item.Name && update.FromVersion == item.Version && (update.State == "" || update.State == "complete") {
						status := "repair_open"
						if proposalStore != nil {
							if task, taskErr := proposalStore.GetTask(inventory.RepositoryID, update.ProposalID, update.TaskID); taskErr == nil {
								status = string(task.Status)
							}
						}
						projected["remediation"] = map[string]any{"status": status, "proposal_id": update.ProposalID, "task_id": update.TaskID, "replacement_version": update.ToVersion}
						break
					}
				}
				visible = append(visible, projected)
			}
		}
		writeJSON(w, 200, map[string]any{"consumers": visible})
	})
	mux.HandleFunc("GET /packages", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := optionalPackageActor(w, r)
		if !ok {
			return
		}
		items, err := packageStore.List()
		if err != nil {
			writeAPIError(w, 500, "package_read_failed", "package catalog could not be read")
			return
		}
		query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		visible := []packages.Version{}
		for _, item := range items {
			if canRead(item, actor, authenticated) && (query == "" || strings.Contains(item.Name, query) || strings.Contains(strings.ToLower(item.Summary), query) || strings.Contains(strings.ToLower(item.Documentation), query)) {
				visible = append(visible, item)
			}
		}
		writeJSON(w, 200, map[string]any{"packages": visible})
	})
	mux.HandleFunc("GET /packages/{name}/versions", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := optionalPackageActor(w, r)
		if !ok {
			return
		}
		items, err := packageStore.List()
		if err != nil {
			writeAPIError(w, 500, "package_read_failed", "package versions could not be read")
			return
		}
		visible := []packages.Version{}
		for _, item := range items {
			if item.Name == strings.ToLower(r.PathValue("name")) && canRead(item, actor, authenticated) {
				visible = append(visible, item)
			}
		}
		writeJSON(w, 200, map[string]any{"packages": visible})
	})
	mux.HandleFunc("GET /packages/{name}/resolve", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := optionalPackageActor(w, r)
		if !ok {
			return
		}
		items, err := packageStore.List()
		if err != nil {
			writeAPIError(w, 500, "package_read_failed", "package versions could not be read")
			return
		}
		constraint := strings.TrimSpace(r.URL.Query().Get("constraint"))
		candidates := []packages.Version{}
		for _, item := range items {
			if item.Name == strings.ToLower(r.PathValue("name")) && item.Lifecycle == "active" && canRead(item, actor, authenticated) && compatiblePlatform(item.Platform, r.URL.Query().Get("os"), r.URL.Query().Get("architecture"), r.URL.Query().Get("runtime")) && versionMatches(item.Version, constraint) {
				candidates = append(candidates, item)
			}
		}
		if len(candidates) == 0 {
			writeAPIError(w, 404, "package_resolution_failed", "no authorized compatible version satisfies the constraint")
			return
		}
		sort.Slice(candidates, func(i, j int) bool { return compareVersion(candidates[i].Version, candidates[j].Version) > 0 })
		writeJSON(w, 200, candidates[0])
	})
	mux.HandleFunc("POST /repositories/{id}/package-credentials", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoryStore, authStore, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var input struct {
			Name         string   `json:"name"`
			PackageNames []string `json:"package_names"`
			ExpiresIn    int      `json:"expires_in"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if input.ExpiresIn < 60 || input.ExpiresIn > 86400 || len(input.PackageNames) == 0 {
			writeAPIError(w, 422, "invalid_package_credential", "expires_in must be 60-86400 seconds and package_names must not be empty")
			return
		}
		all, err := packageStore.List()
		if err != nil {
			writeAPIError(w, 500, "package_credential_failed", "package authorization could not be evaluated")
			return
		}
		for _, name := range input.PackageNames {
			found := false
			for _, item := range all {
				if item.Name == name && canRead(item, actor, true) {
					found = true
					break
				}
			}
			if !found {
				writeAPIError(w, 404, "package_not_found", "an authorized package identity was not found")
				return
			}
		}
		issued, err := authStore.IssuePackageBound(actor.UserID, input.Name, r.PathValue("id"), input.PackageNames, time.Duration(input.ExpiresIn)*time.Second)
		if err != nil {
			writeAPIError(w, 422, "invalid_package_credential", "package credential bounds are invalid")
			return
		}
		writeJSON(w, 201, issued)
	})
	mux.HandleFunc("PATCH /repositories/{id}/packages/{name}/versions/{version}", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositoryStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "package_lifecycle_forbidden", "only the repository owner can change package lifecycle")
			return
		}
		item, err := packageStore.Get(r.PathValue("name"), r.PathValue("version"))
		if err != nil || item.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "package_not_found", "package version not found")
			return
		}
		var input struct {
			Lifecycle   string                `json:"lifecycle"`
			Warning     string                `json:"warning"`
			Reason      string                `json:"reason"`
			Replacement *packages.Replacement `json:"replacement"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		item, err = packageStore.SetLifecycle(item.Name, item.Version, input.Lifecycle, input.Warning, input.Reason, input.Replacement, actor.UserID)
		if err != nil {
			writeAPIError(w, 422, "invalid_package_lifecycle", "deprecated, quarantined, and yanked versions require a reason and warning; a replacement must be an active published version")
			return
		}
		if item.Lifecycle != "active" && activityStore != nil {
			consumers, _ := packageStore.ListConsumers(item.Name, item.Version)
			seen := map[string]bool{}
			for _, inventory := range consumers {
				repository, getErr := repositoryStore.GetByID(inventory.RepositoryID)
				if getErr != nil || seen[repository.OwnerID] {
					continue
				}
				seen[repository.OwnerID] = true
				target := repository.OwnerID
				_ = recordActivityOnce(activityStore, repositoryStore, "package-recovery:"+item.ID+":"+item.Lifecycle+":"+target, activities.Event{Kind: "package.recovery_required", ActorID: actor.UserID, RepositoryID: repository.ID, ResourceType: "package", ResourceID: item.ID, ResourceTitle: item.Name + " " + item.Version + " is " + item.Lifecycle, TargetUserID: &target})
			}
		}
		writeJSON(w, 200, item)
	})
	mux.HandleFunc("GET /packages/{name}/versions/{version}/lifecycle", func(w http.ResponseWriter, r *http.Request) {
		item, err := packageStore.Get(r.PathValue("name"), r.PathValue("version"))
		if err != nil {
			writeAPIError(w, 404, "package_not_found", "package version not found")
			return
		}
		actor, authenticated, ok := optionalPackageActor(w, r)
		if !ok || !canRead(item, actor, authenticated) {
			if ok {
				writeAPIError(w, 404, "package_not_found", "package version not found")
			}
			return
		}
		history, err := packageStore.LifecycleHistory(item.Name, item.Version)
		if err != nil {
			writeAPIError(w, 500, "package_read_failed", "package lifecycle history could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"package": item, "history": history})
	})
	mux.HandleFunc("GET /repositories/{id}/packages", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoryStore, authStore, r.PathValue("id")); !ok {
			return
		}
		items, err := packageStore.ListRepository(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "package_read_failed", "packages could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"packages": items})
	})
	mux.HandleFunc("PUT /repositories/{id}/package-update-policies/{name}", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositoryStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "package_update_policy_forbidden", "only the repository owner can define package update policy")
			return
		}
		var input struct {
			Strategy string `json:"strategy"`
			Action   string `json:"action"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		created, err := packageStore.PutUpdatePolicy(packages.UpdatePolicy{RepositoryID: r.PathValue("id"), PackageName: r.PathValue("name"), Strategy: input.Strategy, Action: input.Action, UpdatedBy: actor.UserID})
		if err != nil {
			writeAPIError(w, 422, "invalid_package_update_policy", "strategy must be patch, minor, or major and action must be proposal")
			return
		}
		writeJSON(w, 200, created)
	})
	mux.HandleFunc("GET /repositories/{id}/package-updates", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repositoryStore, authStore, r.PathValue("id"))
		if !ok {
			return
		}
		policies, err := packageStore.ListUpdatePolicies(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "package_update_read_failed", "update policies could not be read")
			return
		}
		updates, err := packageStore.ListUpdates(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "package_update_read_failed", "updates could not be read")
			return
		}
		visible := []packages.Update{}
		for _, update := range updates {
			if update.State != "" && update.State != "complete" {
				continue
			}
			version, getErr := packageStore.Get(update.PackageName, update.ToVersion)
			if getErr == nil && canRead(version, actor, authenticated) {
				visible = append(visible, update)
			}
		}
		writeJSON(w, 200, map[string]any{"policies": policies, "updates": visible})
	})
	mux.HandleFunc("POST /repositories/{id}/package-updates/scan", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositoryStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "package_update_forbidden", "only the repository owner can open automated adoption work")
			return
		}
		if proposalStore == nil {
			writeAPIError(w, 503, "package_update_unavailable", "proposal collaboration is unavailable")
			return
		}
		catalog, err := repositoryStore.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		repo, err := gitStore.Open(catalog.ID)
		if err != nil {
			writeAPIError(w, 500, "package_update_failed", "repository storage is unavailable")
			return
		}
		ref, err := repo.ReadReference("refs/heads/" + catalog.DefaultBranch)
		if err != nil {
			writeAPIError(w, 409, "package_update_failed", "default branch has no verified dependency manifest")
			return
		}
		inventory, err := packageStore.GetInventory(catalog.ID, ref.Target)
		if err != nil {
			writeAPIError(w, 409, "package_update_failed", "record the current dependency inventory before scanning")
			return
		}
		commit, _ := repo.ReadCommit(storage.ObjectID(ref.Target))
		entry, err := resolvePath(repo, commit.Tree, packages.InventoryConfigPath)
		if err != nil {
			writeAPIError(w, 409, "package_update_failed", "current dependency manifest is unavailable")
			return
		}
		object, err := repo.ReadObject(entry.ID)
		var config packages.InventoryConfig
		if err != nil || json.Unmarshal(object.Content, &config) != nil {
			writeAPIError(w, 409, "package_update_failed", "current dependency manifest is invalid")
			return
		}
		policies, _ := packageStore.ListUpdatePolicies(catalog.ID)
		versions, err := packageStore.List()
		if err != nil {
			writeAPIError(w, 503, "package_update_failed", "package evidence is unavailable")
			return
		}
		created := []packages.Update{}
		for _, policy := range policies {
			var current packages.InventoryEntry
			found := false
			for _, value := range inventory.Entries {
				if value.Name == policy.PackageName && value.Direct && value.Version != "" {
					current, found = value, true
					break
				}
			}
			if !found {
				continue
			}
			best := packages.Version{}
			for _, candidate := range versions {
				if candidate.Name != policy.PackageName || candidate.Lifecycle != "active" || !canRead(candidate, actor, true) || compareVersion(candidate.Version, current.Version) <= 0 || !updateStrategyAllows(current.Version, candidate.Version, policy.Strategy) {
					continue
				}
				if best.Version == "" || compareVersion(candidate.Version, best.Version) > 0 {
					best = candidate
				}
			}
			if best.Version == "" {
				continue
			}
			manifest := config
			for index := range manifest.Dependencies {
				if manifest.Dependencies[index].Name == policy.PackageName {
					manifest.Dependencies[index].Constraint = "^" + best.Version
				}
			}
			for index := range manifest.Lock {
				if manifest.Lock[index].Name == policy.PackageName {
					manifest.Lock[index].Version = best.Version
				}
			}
			release, _ := releaseStore.Get(best.RepositoryID, best.ReleaseID)
			notes := release.Notes
			if strings.TrimSpace(notes) == "" {
				notes = best.Documentation
			}
			body := packageUpdateProposalBody(best, current, notes)
			update, published, createErr := packageStore.PublishUpdate(packages.Update{RepositoryID: catalog.ID, PackageName: best.Name, FromVersion: current.Version, ToVersion: best.Version, BaseCommit: ref.Target, Manifest: manifest, ReleaseNotes: notes, Compatibility: best.BuildAttestation, AffectedPaths: current.Paths, CreatedBy: actor.UserID}, func() (string, string, error) {
				proposal, proposalErr := proposalStore.Create(catalog.ID, actor.UserID, "Update "+best.Name+" to "+best.Version, body)
				if proposalErr != nil {
					return "", "", proposalErr
				}
				task, taskErr := proposalStore.CreateTask(catalog.ID, proposal.ID, actor.UserID, "Apply "+best.Name+" "+best.Version, "Update the manifest and lock exactly as proposed, investigate compatibility failures, and publish the result through ordinary review.", nil, nil)
				if taskErr != nil {
					return proposal.ID, task.ID, taskErr
				}
				return proposal.ID, task.ID, nil
			})
			if createErr != nil {
				if errors.Is(createErr, packages.ErrUpdatePending) {
					writeAPIError(w, 409, "package_update_recovery_pending", "an earlier adoption publication requires recovery before retry")
					return
				}
				if published {
					cleaned := update.ProposalID == ""
					if update.ProposalID != "" {
						cleaned = proposalStore.DeleteMigrationWork(catalog.ID, update.ProposalID, update.TaskID, "") == nil
					}
					_ = packageStore.ResolveUpdateFailure(update, cleaned)
				}
				writeAPIError(w, 500, "package_update_failed", "adoption evidence could not be recorded")
				return
			}
			created = append(created, update)
		}
		writeJSON(w, 200, map[string]any{"updates": created})
	})
	mux.HandleFunc("POST /repositories/{id}/package-recoveries", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositoryStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "package_recovery_forbidden", "only the consuming repository owner can prioritize recovery work")
			return
		}
		var input struct {
			PackageName  string `json:"package_name"`
			Version      string `json:"version"`
			AssigneeType string `json:"assignee_type"`
			AssigneeID   string `json:"assignee_id"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		unsafe, err := packageStore.Get(input.PackageName, input.Version)
		if err != nil || unsafe.Lifecycle == "active" || unsafe.Replacement == nil {
			writeAPIError(w, 422, "package_recovery_unavailable", "the exact unsafe version must name an active safe replacement")
			return
		}
		replacement, err := packageStore.Get(unsafe.Replacement.Name, unsafe.Replacement.Version)
		if err != nil || replacement.Lifecycle != "active" || !canRead(replacement, actor, true) {
			writeAPIError(w, 422, "package_recovery_unavailable", "the safe replacement is not currently installable by this repository")
			return
		}
		catalog, _ := repositoryStore.GetByID(r.PathValue("id"))
		repo, err := gitStore.Open(catalog.ID)
		if err != nil {
			writeAPIError(w, 500, "package_recovery_failed", "repository storage is unavailable")
			return
		}
		ref, err := repo.ReadReference("refs/heads/" + catalog.DefaultBranch)
		if err != nil {
			writeAPIError(w, 409, "package_recovery_failed", "the default branch has no dependency inventory")
			return
		}
		inventory, err := packageStore.GetInventory(catalog.ID, ref.Target)
		if err != nil {
			writeAPIError(w, 409, "package_recovery_failed", "record the current dependency inventory before opening recovery work")
			return
		}
		var exposed *packages.InventoryEntry
		for index := range inventory.Entries {
			if inventory.Entries[index].Name == unsafe.Name && inventory.Entries[index].Version == unsafe.Version {
				exposed = &inventory.Entries[index]
				break
			}
		}
		if exposed == nil {
			writeAPIError(w, 409, "package_not_exposed", "the current recorded dependency inventory does not use this version")
			return
		}
		commit, _ := repo.ReadCommit(storage.ObjectID(ref.Target))
		entry, pathErr := resolvePath(repo, commit.Tree, packages.InventoryConfigPath)
		object, readErr := repo.ReadObject(entry.ID)
		var manifest packages.InventoryConfig
		if pathErr != nil || readErr != nil || json.Unmarshal(object.Content, &manifest) != nil {
			writeAPIError(w, 409, "package_recovery_failed", "the current dependency manifest is unavailable")
			return
		}
		for index := range manifest.Lock {
			if manifest.Lock[index].Name == unsafe.Name {
				manifest.Lock[index].Version = replacement.Version
			}
		}
		if unsafe.Name == replacement.Name {
			for index := range manifest.Dependencies {
				if manifest.Dependencies[index].Name == unsafe.Name {
					manifest.Dependencies[index].Constraint = "^" + replacement.Version
				}
			}
		}
		if input.AssigneeType != "human" && input.AssigneeType != "agent" {
			writeAPIError(w, 422, "invalid_package_recovery", "assignee_type must be human or agent")
			return
		}
		if input.AssigneeType == "human" {
			allowed, _ := repositoryStore.HasCollaborator(input.AssigneeID, catalog.ID)
			if input.AssigneeID != catalog.OwnerID && !allowed {
				writeAPIError(w, 422, "invalid_package_recovery", "human assignee must be a current repository participant")
				return
			}
		}
		body := fmt.Sprintf("Priority: urgent dependency containment.\n\nReplace `%s %s` with `%s %s`.\n\nPublisher reason: %s\n\nWarning: %s\n\nAffected paths:\n- %s\n\nThe publisher receives no authority here; normal checks, review, integration, release, and deployment policy remain authoritative.", unsafe.Name, unsafe.Version, replacement.Name, replacement.Version, unsafe.LifecycleReason, unsafe.LifecycleWarning, strings.Join(exposed.Paths, "\n- "))
		update, _, err := packageStore.PublishUpdate(packages.Update{RepositoryID: catalog.ID, PackageName: unsafe.Name, FromVersion: unsafe.Version, ToVersion: replacement.Version, BaseCommit: ref.Target, Manifest: manifest, ReleaseNotes: replacement.Documentation, Compatibility: replacement.BuildAttestation, AffectedPaths: exposed.Paths, CreatedBy: actor.UserID}, func() (string, string, error) {
			proposal, createErr := proposalStore.Create(catalog.ID, actor.UserID, "Urgent: replace "+unsafe.Name+" "+unsafe.Version, body)
			if createErr != nil {
				return "", "", createErr
			}
			task, createErr := proposalStore.CreateTask(catalog.ID, proposal.ID, actor.UserID, "Contain "+unsafe.Name+" exposure", "Replace the unsafe exact version, verify compatibility, and deliver through ordinary review with no new publisher authority.", nil, nil)
			if createErr != nil {
				return proposal.ID, task.ID, createErr
			}
			assigned, assignErr := proposalStore.AssignTask(catalog.ID, proposal.ID, task.ID, actor.UserID, proposals.TaskAssignmentInput{AssigneeType: input.AssigneeType, AssigneeID: input.AssigneeID, Mandate: "Prioritize containment and verified replacement of " + unsafe.Name + " " + unsafe.Version, RepositoryID: catalog.ID, BaseRevision: ref.Target})
			_ = assigned
			return proposal.ID, task.ID, assignErr
		})
		if err != nil {
			writeAPIError(w, 500, "package_recovery_failed", "recovery work could not be published atomically")
			return
		}
		writeJSON(w, 201, update)
	})
	read := func(w http.ResponseWriter, r *http.Request) (packages.Version, bool) {
		item, err := packageStore.Get(r.PathValue("name"), r.PathValue("version"))
		if errors.Is(err, packages.ErrNotFound) {
			writeAPIError(w, 404, "package_not_found", "package version not found")
			return packages.Version{}, false
		}
		if err != nil {
			writeAPIError(w, 500, "package_read_failed", "package version could not be read")
			return packages.Version{}, false
		}
		actor, authenticated, ok := optionalPackageActor(w, r)
		if !ok {
			return packages.Version{}, false
		}
		if !canRead(item, actor, authenticated) {
			writeAPIError(w, 404, "package_not_found", "package version not found")
			return packages.Version{}, false
		}
		return item, true
	}
	mux.HandleFunc("GET /packages/{name}/versions/{version}", func(w http.ResponseWriter, r *http.Request) {
		item, ok := read(w, r)
		if ok {
			writeJSON(w, 200, item)
		}
	})
	mux.HandleFunc("GET /packages/{name}/versions/{version}/artifact", func(w http.ResponseWriter, r *http.Request) {
		item, ok := read(w, r)
		if !ok {
			return
		}
		if item.Lifecycle != "active" {
			header := strings.TrimSpace(r.Header.Get("Authorization"))
			if token, found := strings.CutPrefix(header, "Bearer "); found {
				if installActor, authErr := authStore.Authenticate(token, "packages:read"); authErr == nil && installActor.RepositoryID != "" && len(installActor.PackageNames) > 0 {
					writeAPIError(w, 409, "package_install_blocked", "repository-scoped installs reject deprecated, quarantined, and yanked versions; inspect lifecycle evidence and use the safe replacement")
					return
				}
			}
		}
		file, _, err := packageStore.OpenArtifact(item.Name, item.Version)
		if err != nil {
			writeAPIError(w, 500, "package_artifact_unavailable", "package artifact could not be read")
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", item.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(item.ArtifactPath)))
		w.Header().Set("X-Checksum-Sha256", item.SHA256)
		http.ServeContent(w, r, path.Base(item.ArtifactPath), item.PublishedAt, file)
	})
}

func packageUpdateProposalBody(version packages.Version, current packages.InventoryEntry, notes string) string {
	paths := strings.Join(current.Paths, "\n- ")
	if version.Visibility == "private" {
		return fmt.Sprintf("Adopt %s %s → %s from a verified private package release.\n\nAffected dependency paths:\n- %s\n\nPublisher release notes and compatibility attestation remain available only through the package-authorized update record. The proposed `.vivarium/packages.json` manifest and lock are attached there. Required checks, review, and integration policy remain authoritative.", version.Name, current.Version, version.Version, paths)
	}
	return fmt.Sprintf("Adopt %s %s → %s from verified release `%s`.\n\nRelease notes:\n%s\n\nCompatibility evidence: successful `%s` build, image `%s`, attempt %d.\n\nAffected dependency paths:\n- %s\n\nThe proposed `.vivarium/packages.json` manifest and lock are attached to the package update record. Required checks, review, and integration policy remain authoritative.", version.Name, current.Version, version.Version, version.ReleaseID, notes, version.BuildAttestation.Step, version.BuildAttestation.Image, version.BuildAttestation.Attempt, paths)
}

func updateStrategyAllows(from, to, strategy string) bool {
	left, lok := versionParts(from)
	right, rok := versionParts(to)
	if !lok || !rok {
		return false
	}
	if right[0] != left[0] {
		return strategy == "major"
	}
	if right[1] != left[1] {
		return strategy == "minor" || strategy == "major"
	}
	return strategy == "patch" || strategy == "minor" || strategy == "major"
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func uniquePackageStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func compatiblePlatform(platform packages.Platform, osName, architecture, runtime string) bool {
	return (osName == "" || platform.OS == "" || platform.OS == osName) &&
		(architecture == "" || platform.Architecture == "" || platform.Architecture == architecture) &&
		(runtime == "" || platform.Runtime == "" || platform.Runtime == runtime)
}

func versionParts(version string) ([3]int, bool) {
	var result [3]int
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	version = strings.SplitN(strings.SplitN(version, "+", 2)[0], "-", 2)[0]
	parts := strings.Split(version, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return result, false
	}
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return result, false
		}
		result[index] = value
	}
	return result, true
}

func prereleaseParts(version string) []string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	version = strings.SplitN(version, "+", 2)[0]
	_, prerelease, found := strings.Cut(version, "-")
	if !found || prerelease == "" {
		return nil
	}
	return strings.Split(prerelease, ".")
}

func compareVersion(left, right string) int {
	l, lok := versionParts(left)
	r, rok := versionParts(right)
	if !lok || !rok {
		return strings.Compare(left, right)
	}
	for index := range l {
		if l[index] < r[index] {
			return -1
		}
		if l[index] > r[index] {
			return 1
		}
	}
	lPre, rPre := prereleaseParts(left), prereleaseParts(right)
	if len(lPre) == 0 && len(rPre) > 0 {
		return 1
	}
	if len(lPre) > 0 && len(rPre) == 0 {
		return -1
	}
	for index := 0; index < len(lPre) && index < len(rPre); index++ {
		lNumber, lErr := strconv.Atoi(lPre[index])
		rNumber, rErr := strconv.Atoi(rPre[index])
		switch {
		case lErr == nil && rErr == nil && lNumber != rNumber:
			if lNumber < rNumber {
				return -1
			}
			return 1
		case lErr == nil && rErr != nil:
			return -1
		case lErr != nil && rErr == nil:
			return 1
		case lPre[index] != rPre[index]:
			return strings.Compare(lPre[index], rPre[index])
		}
	}
	if len(lPre) < len(rPre) {
		return -1
	}
	if len(lPre) > len(rPre) {
		return 1
	}
	return 0
}

// versionMatches intentionally supports the portable subset shared by common
// package clients: exact versions, caret/tilde ranges, and ordered bounds.
func versionMatches(version, constraint string) bool {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" || constraint == "*" {
		return len(prereleaseParts(version)) == 0
	}
	actual, ok := versionParts(version)
	if !ok {
		return version == constraint
	}
	// Stable constraints never opt into prerelease artifacts. A prerelease is
	// eligible only when the caller names a prerelease in the constraint.
	if len(prereleaseParts(version)) > 0 && len(prereleaseParts(strings.TrimLeft(constraint, "^~<>= "))) == 0 {
		return false
	}
	if strings.HasPrefix(constraint, "^") || strings.HasPrefix(constraint, "~") {
		kind := constraint[0]
		base, valid := versionParts(constraint[1:])
		if !valid || compareVersion(version, constraint[1:]) < 0 {
			return false
		}
		if kind == '~' {
			return actual[0] == base[0] && actual[1] == base[1]
		}
		if base[0] > 0 {
			return actual[0] == base[0]
		}
		if base[1] > 0 {
			return actual[0] == 0 && actual[1] == base[1]
		}
		return actual == base
	}
	for _, operator := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(constraint, operator) {
			comparison := compareVersion(version, strings.TrimSpace(strings.TrimPrefix(constraint, operator)))
			switch operator {
			case ">=":
				return comparison >= 0
			case "<=":
				return comparison <= 0
			case ">":
				return comparison > 0
			case "<":
				return comparison < 0
			default:
				return comparison == 0
			}
		}
	}
	return compareVersion(version, constraint) == 0
}
