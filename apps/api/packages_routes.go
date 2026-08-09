package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

var (
	errPackageBuildUnverified = errors.New("package build is not currently verified")
	errPackageArtifactStale   = errors.New("package artifact is not from the current attempt")
)

func registerPackageRoutes(mux *http.ServeMux, repositoryStore *repositories.Store, releaseStore *releases.Store, buildStore *checkruns.Store, packageStore *packages.Store, authStore *auth.Store) {
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
			Name         string                `json:"name"`
			Version      string                `json:"version"`
			BuildID      string                `json:"build_id"`
			ArtifactID   string                `json:"artifact_id"`
			Platform     packages.Platform     `json:"platform"`
			Dependencies []packages.Dependency `json:"dependencies"`
			Visibility   string                `json:"visibility"`
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
			created, publishErr = packageStore.Publish(packages.Version{Name: input.Name, Version: input.Version, RepositoryID: release.RepositoryID, ReleaseID: release.ID, SourceCommit: release.CommitID, BuildID: current.ID, BuildAttestation: packages.BuildAttestation{Step: current.Definition.Name, Image: current.Definition.Image, Command: current.Definition.Command, Attempt: latestAttempt, State: current.State}, ArtifactID: artifact.ID, ArtifactPath: artifact.Path, ContentType: artifact.ContentType, Size: artifact.Size, SHA256: artifact.SHA256, Platform: input.Platform, Dependencies: input.Dependencies, PublisherID: actor.UserID, Visibility: input.Visibility}, artifactFile)
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
		case err != nil:
			writeAPIError(w, 500, "package_publish_failed", "the package could not be published atomically")
		default:
			w.Header().Set("Location", "/packages/"+created.Name+"/versions/"+created.Version)
			writeJSON(w, 201, created)
		}
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
		if item.Visibility == "private" {
			if _, _, ok := authorizeRepositoryRead(w, r, repositoryStore, authStore, item.RepositoryID); !ok {
				return packages.Version{}, false
			}
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
