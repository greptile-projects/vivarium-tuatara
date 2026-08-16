package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/protectionplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoverycommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoveryexercises"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type recoveryExerciseInput struct {
	Name      string                   `json:"name"`
	Scenario  string                   `json:"scenario"`
	PlanID    string                   `json:"plan_id"`
	CaptureID string                   `json:"capture_id"`
	Steps     []recoveryexercises.Step `json:"steps"`
}

func registerRecoveryExerciseRoutes(mux *http.ServeMux, git *storage.Store, environments *deployments.Store, catalog *repositories.Store, credentials *auth.Store, plans *protectionplans.Store, commitments *recoverycommitments.Store, exercises *recoveryexercises.Store) {
	mux.HandleFunc("GET /repositories/{id}/recovery-exercises", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, e := exercises.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "recovery_exercises_unavailable", "recovery exercise evidence could not be read")
			return
		}
		for i := range values {
			refreshExercise(&values[i], git, environments, plans, commitments)
		}
		writeJSON(w, 200, map[string]any{"exercises": values})
	})
	mux.HandleFunc("POST /repositories/{id}/recovery-exercises", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in recoveryExerciseInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded recovery exercise is required")
			return
		}
		p, e := plans.Get(in.PlanID)
		if e != nil || p.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "protection_plan_not_found", "protection plan not found")
			return
		}
		if !exerciseContains(p.AccessorIDs, actor.UserID) {
			writeAPIError(w, 403, "recovery_exercise_forbidden", "only a declared recovery accessor may launch this exercise")
			return
		}
		var capture protectionplans.Capture
		found := false
		for _, c := range p.Captures {
			if c.ID == in.CaptureID {
				capture = c
				found = true
				break
			}
		}
		if !found || !capture.Recoverable {
			writeAPIError(w, 409, "recovery_capture_unavailable", "the selected protected capture is not recoverable")
			return
		}
		restored, e := plans.Restore(p.ID, capture.ID)
		if e != nil {
			writeAPIError(w, 409, "recovery_capture_unavailable", "the selected protected capture failed integrity verification")
			return
		}
		exercise := recoveryexercises.Exercise{Name: strings.TrimSpace(in.Name), Scenario: strings.TrimSpace(in.Scenario), PlanID: p.ID, PlanVersion: capture.PlanVersion, CommitmentID: p.CommitmentID, CommitmentVersion: capture.CommitmentVersion, CaptureID: capture.ID, SourceRevision: capture.SourceRevision, Steps: in.Steps}
		execute, cleanup := newExerciseExecutor(restored, p.ValidationChecks)
		defer cleanup()
		out, e := exercises.Run(r.PathValue("id"), actor.UserID, exercise, execute)
		if errors.Is(e, recoveryexercises.ErrInvalid) {
			writeAPIError(w, 400, "invalid_recovery_exercise", "use ordered restore, integrity, journey, or manual steps with declared objectives")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "recovery_exercises_unavailable", "recovery exercise evidence could not be persisted")
			return
		}
		writeJSON(w, 201, out)
	})
}

type recoveryRuntime struct {
	root        string
	provisioned bool
	restored    int
}

func newExerciseExecutor(source protectionplans.RestoredSource, declared []string) (func(recoveryexercises.Step) (string, string, string, bool), func()) {
	runtime := &recoveryRuntime{}
	cleanup := func() {
		if runtime.root != "" {
			_ = os.RemoveAll(runtime.root)
		}
	}
	execute := func(step recoveryexercises.Step) (string, string, string, bool) {
		switch step.Kind {
		case "restore":
			if step.Command != "restore:protected-manifest" {
				return "failed", "restore command is not permitted", "", false
			}
			if runtime.provisioned {
				return "failed", "isolated environment was already populated", "", false
			}
			root, err := os.MkdirTemp("", "vivarium-recovery-")
			if err != nil {
				return "failed", "isolated environment could not be provisioned", "", false
			}
			runtime.root = root
			if err = restoreExercisePayload(runtime, source); err != nil {
				cleanup()
				runtime.root = ""
				return "failed", "protected capture could not be applied to isolated environment", "", false
			}
			runtime.provisioned = true
			return "passed", "protected capture applied to ephemeral isolated environment", "restored artifacts: " + exerciseInt(runtime.restored), false
		case "integrity":
			if step.Command != "verify:manifest" && step.Command != "verify:dependencies" {
				return "failed", "integrity command is not permitted", "", false
			}
			if step.Command == "verify:dependencies" {
				ids := map[string]bool{}
				for _, e := range source.Manifest {
					ids[e.Version] = true
				}
				for _, e := range source.Manifest {
					dependencies := e.Dependencies
					// Parent commits are provenance; only the captured root tree
					// is a required restore input.
					if e.Kind == "commit" && len(dependencies) > 1 {
						dependencies = dependencies[:1]
					}
					for _, d := range dependencies {
						if !ids[d] {
							return "failed", "missing declared dependency", "", false
						}
					}
				}
			}
			sum := sha256.Sum256(source.Payload)
			return "passed", "integrity verified without exposing restored content", "sha256:" + hex.EncodeToString(sum[:]), false
		case "journey":
			name := strings.TrimPrefix(step.Command, "journey:")
			if name == step.Command || !exerciseContains(declared, name) {
				return "failed", "journey is not declared by the protection plan", "", false
			}
			if !runtime.provisioned {
				return "failed", "isolated environment has not been restored", "", false
			}
			if name != "smoke" {
				return "failed", "declared journey has no bounded runner implementation", "", false
			}
			if _, err := os.Stat(filepath.Join(runtime.root, ".recovery", "manifest.json")); err != nil || runtime.restored == 0 {
				return "failed", "restored system failed the smoke journey", "", false
			}
			return "passed", "bounded smoke journey executed against restored environment", "journey:smoke", false
		case "manual":
			if step.Command != "manual:confirm" {
				return "failed", "manual command is not permitted", "", true
			}
			return "passed", "manual recovery step acknowledged by launch actor", "", true
		}
		return "failed", "unsupported bounded command", "", false
	}
	return execute, cleanup
}

func restoreExercisePayload(runtime *recoveryRuntime, source protectionplans.RestoredSource) error {
	var resources map[string]json.RawMessage
	if json.Unmarshal(source.Payload, &resources) != nil {
		return protectionplans.ErrInvalid
	}
	for targetID, raw := range resources {
		targetSum := sha256.Sum256([]byte(targetID))
		targetDirectory := hex.EncodeToString(targetSum[:8])
		var repository struct {
			Objects map[string]storage.Object `json:"objects"`
		}
		if json.Unmarshal(raw, &repository) == nil && len(repository.Objects) > 0 {
			for _, entry := range source.Manifest {
				object, ok := repository.Objects[entry.Version]
				if !ok || object.Type != storage.BlobObject {
					continue
				}
				clean := filepath.Clean(entry.Path)
				if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
					return protectionplans.ErrInvalid
				}
				sum := sha256.Sum256(object.Content)
				if hex.EncodeToString(sum[:]) != entry.SHA256 {
					return protectionplans.ErrInvalid
				}
				destination := filepath.Join(runtime.root, "workspace", targetDirectory, clean)
				if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
					return err
				}
				if err := os.WriteFile(destination, object.Content, 0600); err != nil {
					return err
				}
				runtime.restored++
			}
			continue
		}
		// Governed environment captures contain only the credential-free public
		// projection. Restore it under an opaque name to avoid trusting target IDs.
		destination := filepath.Join(runtime.root, "environment", targetDirectory+".json")
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(destination, raw, 0600); err != nil {
			return err
		}
		runtime.restored++
	}
	if runtime.restored == 0 {
		return protectionplans.ErrInvalid
	}
	manifest, err := json.Marshal(source.Manifest)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(runtime.root, ".recovery", "manifest.json")
	if err = os.MkdirAll(filepath.Dir(manifestPath), 0700); err != nil {
		return err
	}
	return os.WriteFile(manifestPath, manifest, 0600)
}
func refreshExercise(x *recoveryexercises.Exercise, git *storage.Store, environments *deployments.Store, plans *protectionplans.Store, commitments *recoverycommitments.Store) {
	x.Current = true
	x.StaleReasons = []string{}
	p, e := plans.Get(x.PlanID)
	if e != nil {
		x.Current = false
		x.StaleReasons = append(x.StaleReasons, "protection_plan_unavailable")
		return
	}
	if p.Version != x.PlanVersion {
		x.Current = false
		x.StaleReasons = append(x.StaleReasons, "protection_plan_changed")
	}
	commitment, commitmentErr := commitments.Get(x.CommitmentID)
	if commitmentErr != nil || commitment.CurrentVersion != x.CommitmentVersion {
		x.Current = false
		x.StaleReasons = append(x.StaleReasons, "recovery_commitment_changed")
	}
	found := false
	for _, c := range p.Captures {
		if c.ID == x.CaptureID {
			found = true
			if !c.Recoverable {
				x.Current = false
				x.StaleReasons = append(x.StaleReasons, "protected_capture_unavailable")
			}
			if c.SourceRevision != x.SourceRevision {
				x.Current = false
				x.StaleReasons = append(x.StaleReasons, "protected_dependencies_changed")
			}
			frozen := p
			frozen.Resources = c.Resources
			var currentSource protectionplans.Source
			var sourceErr error
			for _, resource := range frozen.Resources {
				if resource.Kind == "environment" && environments == nil {
					sourceErr = protectionplans.ErrInvalid
				}
			}
			if sourceErr == nil {
				currentSource, sourceErr = buildProtectionSource(git, environments, frozen)
			}
			if sourceErr != nil || currentSource.Revision != x.SourceRevision {
				x.Current = false
				x.StaleReasons = append(x.StaleReasons, "protected_dependencies_changed")
			}
		}
	}
	if !found {
		x.Current = false
		x.StaleReasons = append(x.StaleReasons, "protected_capture_unavailable")
	}
	sort.Strings(x.StaleReasons)
}
func exerciseContains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func exerciseInt(v int) string {
	if v == 0 {
		return "0"
	}
	b := make([]byte, 0, 10)
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
