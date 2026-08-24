package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os/exec"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/historyremediations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerHistoryContainmentRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *historyremediations.Store) {
	authorizeOwner := func(w http.ResponseWriter, r *http.Request) (historyremediations.Remediation, auth.Credential, bool) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return historyremediations.Remediation{}, c, false
		}
		v, e := store.Get(r.PathValue("id"), r.PathValue("remediation_id"))
		if e != nil || !historyRemediationCanPublish(v, c) {
			writeAPIError(w, 404, "history_remediation_not_found", "history remediation not found")
			return v, c, false
		}
		return v, c, true
	}
	mux.HandleFunc("POST /repositories/{id}/history-remediations/{remediation_id}/containment-passes", func(w http.ResponseWriter, r *http.Request) {
		v, c, ok := authorizeOwner(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                                 `json:"expected_version"`
			Pass            historyremediations.ContainmentPass `json:"pass"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete containment pass is required")
			return
		}
		// Repository reachability and ordinary object-access containment are
		// always server-derived; caller evidence for those kinds is discarded.
		filtered := make([]historyremediations.ContainmentObservation, 0, len(in.Pass.Observations))
		for _, x := range in.Pass.Observations {
			if x.Kind != "repository_reachability" && x.Kind != "object_access" {
				filtered = append(filtered, x)
			}
		}
		state, summary := "failed", "authoritative replacement refs no longer match"
		if v.Publication != nil {
			for _, candidate := range v.RewriteCandidates {
				if candidate.ID != v.Publication.CandidateID {
					continue
				}
				state = "passed"
				summary = "every selected authoritative ref resolves to its replacement tip"
				repo, e := git.Open(v.RepositoryID)
				if e != nil {
					state = "unreachable"
					summary = "repository could not be rechecked"
				} else {
					for _, ref := range candidate.CandidateRefs {
						tip, e := historyGitOutput(repo.Path(), "rev-parse", "--verify", ref.Name)
						if e != nil || tip != ref.NewTip {
							state = "reintroduced"
							summary = "a selected ref moved away from its replacement tip"
						}
					}
				}
			}
		}
		digest := sha256.Sum256([]byte(v.ID + ":" + state + ":" + summary))
		filtered = append(filtered, historyremediations.ContainmentObservation{Kind: "repository_reachability", ResourceID: v.RepositoryID, State: state, EvidenceSHA256: hex.EncodeToString(digest[:]), Summary: summary})
		objectState := "passed"
		objectSummary := "affected objects are unreachable from advertised refs and direct SHA wants are disabled"
		quarantined := []string{}
		repoPath := ""
		if v.Publication != nil {
			published := map[string]bool{}
			for _, id := range v.Publication.QuarantinedObjects {
				published[id] = true
			}
			for _, scope := range v.Scopes {
				if scope.Kind == "git_object" && published[scope.ObjectID] {
					quarantined = append(quarantined, scope.ObjectID)
				}
			}
		}
		if len(quarantined) == 0 {
			objectState = "failed"
			objectSummary = "no active object quarantine was found"
		} else if repo, openErr := git.Open(v.RepositoryID); openErr != nil {
			objectState = "unreachable"
			objectSummary = "repository object reachability could not be rechecked"
		} else {
			repoPath = repo.Path()
			if reachable, checkErr := historyQuarantinedObjectsReachable(repoPath, quarantined); checkErr != nil {
				objectState = "unreachable"
				objectSummary = "repository object reachability could not be rechecked"
			} else if len(reachable) != 0 {
				objectState = "reintroduced"
				objectSummary = "an affected object is reachable from an ordinarily advertised ref"
			}
		}
		d2 := sha256.Sum256([]byte(v.ID + ":" + objectState + ":" + repoPath + ":" + strings.Join(quarantined, ",")))
		filtered = append(filtered, historyremediations.ContainmentObservation{Kind: "object_access", ResourceID: v.RepositoryID, State: objectState, EvidenceSHA256: hex.EncodeToString(d2[:]), Summary: objectSummary})
		in.Pass.Observations = filtered
		out, e := store.RecordContainmentPass(v.RepositoryID, v.ID, in.ExpectedVersion, in.Pass, c.UserID)
		switch {
		case errors.Is(e, historyremediations.ErrVersionConflict):
			writeAPIError(w, 409, "history_remediation_version_conflict", "the remediation changed; reload and recheck current containment")
		case errors.Is(e, historyremediations.ErrInvalid):
			writeAPIError(w, 422, "containment_pass_invalid", "each policy kind requires unique current digest evidence and bounded exceptions expire within 30 days")
		case e != nil:
			writeAPIError(w, 500, "containment_unavailable", "containment evidence could not be retained")
		default:
			writeJSON(w, 201, historyRemediationPublic(out))
		}
	})
	mux.HandleFunc("POST /repositories/{id}/history-remediations/{remediation_id}/migration-actions", func(w http.ResponseWriter, r *http.Request) {
		v, c, ok := authorizeOwner(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                                 `json:"expected_version"`
			Action          historyremediations.MigrationAction `json:"action"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a migration action is required")
			return
		}
		out, e := store.RecordMigration(v.RepositoryID, v.ID, in.ExpectedVersion, in.Action, c.UserID)
		if e != nil {
			writeAPIError(w, 422, "migration_action_invalid", "pull or workspace migration must preserve discussion and attribution against an exact replacement revision")
			return
		}
		writeJSON(w, 201, historyRemediationPublic(out))
	})
	mux.HandleFunc("POST /repositories/{id}/history-remediations/{remediation_id}/restorations", func(w http.ResponseWriter, r *http.Request) {
		v, c, ok := authorizeOwner(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                             `json:"expected_version"`
			Restoration     historyremediations.Restoration `json:"restoration"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a scoped restoration is required")
			return
		}
		out, e := store.Restore(v.RepositoryID, v.ID, in.ExpectedVersion, in.Restoration, c.UserID)
		if e != nil {
			writeAPIError(w, 409, "restoration_blocked", "only the latest complete passing containment evidence can restore a supported flow")
			return
		}
		writeJSON(w, 201, historyRemediationPublic(out))
	})
}

func historyQuarantinedObjectsReachable(gitDir string, quarantined []string) ([]string, error) {
	out, err := exec.Command("git", "--git-dir="+gitDir, "rev-list", "--objects", "--all").Output()
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, id := range quarantined {
		wanted[id] = true
	}
	found := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		id, _, _ := strings.Cut(line, " ")
		if wanted[id] {
			found = append(found, id)
			delete(wanted, id)
		}
	}
	return found, nil
}
