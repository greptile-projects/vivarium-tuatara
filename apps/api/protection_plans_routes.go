package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/protectionplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoverycommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type protectionPlanInput struct {
	ExpectedVersion int                  `json:"expected_version"`
	Plan            protectionplans.Plan `json:"plan"`
}

func registerProtectionPlanRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, plans *protectionplans.Store, commitments *recoverycommitments.Store, environments *deployments.Store) {
	mux.HandleFunc("GET /repositories/{id}/protection-plans", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, e := plans.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "protection_plans_unavailable", "protection evidence could not be read")
			return
		}
		refreshProtectionSources(values, git, environments)
		writeJSON(w, 200, map[string]any{"plans": values})
	})
	mux.HandleFunc("GET /repositories/{id}/protection-plans/{plan_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		p, e := plans.Get(r.PathValue("plan_id"))
		if e != nil || p.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "protection_plan_not_found", "protection plan not found")
			return
		}
		refreshProtectionSources([]protectionplans.Plan{p}, git, environments)
		writeJSON(w, 200, p)
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in protectionPlanInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete protection plan is required")
				return
			}
			if revise {
				current, e := plans.Get(r.PathValue("plan_id"))
				if e != nil || current.RepositoryID != r.PathValue("id") {
					writeAPIError(w, 404, "protection_plan_not_found", "protection plan not found")
					return
				}
			}
			owners := append([]string{actor.UserID}, in.Plan.AccessorIDs...)
			var out protectionplans.Plan
			err := catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error {
				if e := validateProtectionPlan(r.PathValue("id"), in.Plan, commitments, environments); e != nil {
					return e
				}
				if revise {
					var e error
					out, e = plans.Revise(r.PathValue("plan_id"), in.ExpectedVersion, actor.UserID, in.Plan)
					return e
				}
				var e error
				out, e = plans.Create(r.PathValue("id"), actor.UserID, in.Plan)
				return e
			})
			writeProtectionPlan(w, out, err, map[bool]int{true: 200, false: 201}[revise])
		}
	}
	mux.HandleFunc("POST /repositories/{id}/protection-plans", publish(false))
	mux.HandleFunc("POST /repositories/{id}/protection-plans/{plan_id}/revisions", publish(true))
	mux.HandleFunc("POST /repositories/{id}/protection-plans/{plan_id}/captures", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version is required")
			return
		}
		p, e := plans.Get(r.PathValue("plan_id"))
		if e != nil || p.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "protection_plan_not_found", "protection plan not found")
			return
		}
		if e = validateProtectionPlan(r.PathValue("id"), p, commitments, environments); e != nil {
			writeProtectionPlan(w, protectionplans.Plan{}, e, 200)
			return
		}
		source, e := buildProtectionSource(git, environments, p)
		if e != nil {
			writeAPIError(w, 409, "protection_capture_incomplete", "every planned source must exist and pass integrity checks before a capture is retained")
			return
		}
		out, e := plans.Capture(p.ID, actor.UserID, in.ExpectedVersion, source)
		writeProtectionPlan(w, out, e, 201)
	})
}

func validateProtectionPlan(repo string, p protectionplans.Plan, commitments *recoverycommitments.Store, environments *deployments.Store) error {
	c, e := commitments.Get(p.CommitmentID)
	if e != nil || c.RepositoryID != repo || c.CurrentVersion != p.CommitmentVersion {
		return protectionplans.ErrInvalid
	}
	rev := c.Revisions[len(c.Revisions)-1]
	targets := map[string]recoverycommitments.Target{}
	for _, t := range rev.Targets {
		targets[t.ID] = t
	}
	for _, r := range p.Resources {
		t, ok := targets[r.TargetID]
		if !ok {
			return protectionplans.ErrInvalid
		}
		if r.Kind == "repository" {
			if t.Kind != "repository" || len(r.Revision) != 40 {
				return protectionplans.ErrInvalid
			}
		} else {
			if r.EnvironmentID == "" || environments == nil {
				return protectionplans.ErrInvalid
			}
			if _, e := environments.GetEnvironment(repo, r.EnvironmentID); e != nil {
				return protectionplans.ErrInvalid
			}
		}
		allowed := false
		for _, j := range t.Jurisdictions {
			if strings.EqualFold(j, p.Jurisdiction) {
				allowed = true
			}
		}
		if !allowed {
			return protectionplans.ErrInvalid
		}
	}
	return nil
}

func buildProtectionSource(git *storage.Store, environments *deployments.Store, p protectionplans.Plan) (protectionplans.Source, error) {
	entries := []protectionplans.Entry{}
	payload := map[string]any{}
	versions := []string{}
	for _, r := range p.Resources {
		if r.Kind == "repository" {
			repo, e := git.Open(p.RepositoryID)
			if e != nil {
				return protectionplans.Source{}, e
			}
			commit, e := repo.ReadCommit(storage.ObjectID(r.Revision))
			if e != nil {
				return protectionplans.Source{}, e
			}
			walk, e := repo.WalkTree(commit.Tree)
			if e != nil {
				return protectionplans.Source{}, e
			}
			objects := map[string]any{}
			commitObject, e := repo.ReadObject(commit.ID)
			if e != nil {
				return protectionplans.Source{}, e
			}
			commitDependencies := []string{string(commit.Tree)}
			for _, parent := range commit.Parents {
				commitDependencies = append(commitDependencies, string(parent))
			}
			commitSum := sha256.Sum256(commitObject.Content)
			entries = append(entries, protectionplans.Entry{Path: "$commit", Kind: string(commitObject.Type), Version: string(commit.ID), SHA256: hex.EncodeToString(commitSum[:]), Size: commitObject.Size, Dependencies: commitDependencies})
			objects[string(commitObject.ID)] = commitObject
			root, e := repo.ReadObject(commit.Tree)
			if e != nil {
				return protectionplans.Source{}, e
			}
			rootChildren, e := repo.ReadTree(root.ID)
			if e != nil {
				return protectionplans.Source{}, e
			}
			rootDependencies := make([]string, 0, len(rootChildren))
			for _, child := range rootChildren {
				rootDependencies = append(rootDependencies, string(child.ID))
			}
			rootSum := sha256.Sum256(root.Content)
			entries = append(entries, protectionplans.Entry{Path: "$tree", Kind: string(root.Type), Version: string(root.ID), SHA256: hex.EncodeToString(rootSum[:]), Size: root.Size, Dependencies: rootDependencies})
			objects[string(root.ID)] = root
			for _, x := range walk {
				if x.Mode == "160000" {
					continue
				}
				o, e := repo.ReadObject(x.ID)
				if e != nil {
					return protectionplans.Source{}, e
				}
				sum := sha256.Sum256(o.Content)
				dependencies := []string{}
				if o.Type == storage.TreeObject {
					children, treeErr := repo.ReadTree(o.ID)
					if treeErr != nil {
						return protectionplans.Source{}, treeErr
					}
					for _, child := range children {
						dependencies = append(dependencies, string(child.ID))
					}
				}
				entries = append(entries, protectionplans.Entry{Path: x.Path, Kind: string(o.Type), Version: string(o.ID), SHA256: hex.EncodeToString(sum[:]), Size: o.Size, Dependencies: dependencies})
				objects[string(o.ID)] = o
			}
			payload[r.TargetID] = map[string]any{"commit": commit, "objects": objects}
			versions = append(versions, r.Revision)
		} else {
			env, e := environments.GetEnvironment(p.RepositoryID, r.EnvironmentID)
			if e != nil {
				return protectionplans.Source{}, e
			}
			body, _ := json.Marshal(env)
			sum := sha256.Sum256(body)
			entries = append(entries, protectionplans.Entry{Path: "environment/" + r.EnvironmentID, Kind: "environment", Version: env.UpdatedAt.Format("20060102T150405.000000Z"), SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))})
			payload[r.TargetID] = env
			versions = append(versions, env.UpdatedAt.Format("20060102T150405.000000Z"))
		}
	}
	sort.Strings(versions)
	body, e := json.Marshal(payload)
	if e != nil || len(entries) == 0 {
		return protectionplans.Source{}, protectionplans.ErrInvalid
	}
	sum := sha256.Sum256([]byte(strings.Join(versions, "\n")))
	return protectionplans.Source{Revision: hex.EncodeToString(sum[:]), Entries: entries, Payload: body}, nil
}

func refreshProtectionSources(plans []protectionplans.Plan, git *storage.Store, environments *deployments.Store) {
	for pi := range plans {
		p := &plans[pi]
		for ci := range p.Captures {
			c := &p.Captures[ci]
			resources := c.Resources
			if len(resources) == 0 {
				resources = p.Resources
			}
			for _, r := range resources {
				missing := false
				if r.Kind == "repository" {
					repo, e := git.Open(p.RepositoryID)
					if e != nil {
						missing = true
					} else if _, e = repo.ReadCommit(storage.ObjectID(r.Revision)); e != nil {
						missing = true
					}
				} else if _, e := environments.GetEnvironment(p.RepositoryID, r.EnvironmentID); e != nil {
					missing = true
				}
				if missing {
					c.Recoverable = false
					c.Validation = "failed"
					c.Failure = "source_deleted"
					break
				}
			}
		}
	}
}
func writeProtectionPlan(w http.ResponseWriter, p protectionplans.Plan, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, p)
	case errors.Is(e, protectionplans.ErrConflict):
		writeAPIError(w, 409, "protection_plan_conflict", "the plan changed; reload before continuing")
	case errors.Is(e, protectionplans.ErrInvalid):
		writeAPIError(w, 400, "invalid_protection_plan", "bind current commitment targets, permitted vault location, retention, access, validation, and exact sources")
	default:
		log.Printf("protection plan: %v", e)
		writeAPIError(w, 500, "protection_plans_unavailable", "protection evidence could not be persisted")
	}
}
