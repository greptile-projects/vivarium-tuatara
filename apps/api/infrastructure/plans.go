package infrastructure

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrPlanNotFound = errors.New("infrastructure plan not found")
var ErrPlanStale = errors.New("infrastructure plan dependencies changed")

type PlanRisk struct {
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Summary    string `json:"summary"`
	Mitigation string `json:"mitigation"`
}

type PlanChange struct {
	ResourceID    string     `json:"resource_id"`
	Action        string     `json:"action"`
	Before        *Resource  `json:"before,omitempty"`
	After         *Resource  `json:"after,omitempty"`
	DependencyIDs []string   `json:"dependency_ids"`
	Order         int        `json:"order"`
	Risks         []PlanRisk `json:"risks"`
	RollbackLimit string     `json:"rollback_limit"`
}

type PolicyEffect struct {
	Path    string   `json:"path"`
	Digest  string   `json:"digest"`
	Effects []string `json:"effects"`
}

type PlanEvent struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	ActorID     string    `json:"actor_id"`
	ActorType   string    `json:"actor_type"`
	Body        string    `json:"body"`
	ResourceIDs []string  `json:"resource_ids"`
	OwnerID     string    `json:"owner_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type ChangePlan struct {
	ID                     string         `json:"id"`
	RepositoryID           string         `json:"repository_id"`
	PullRequestID          string         `json:"pull_request_id"`
	SourceRevision         string         `json:"source_revision"`
	DefinitionID           string         `json:"definition_id"`
	DefinitionVersion      int            `json:"definition_version"`
	ObservationFingerprint string         `json:"observation_fingerprint"`
	ObservationsValidUntil *time.Time     `json:"observations_valid_until,omitempty"`
	Candidate              Revision       `json:"candidate"`
	CandidatePath          string         `json:"candidate_path"`
	CandidateDigest        string         `json:"candidate_digest"`
	Changes                []PlanChange   `json:"changes"`
	PolicyEffects          []PolicyEffect `json:"policy_effects"`
	AffectedOwnerIDs       []string       `json:"affected_owner_ids"`
	Events                 []PlanEvent    `json:"events"`
	Fresh                  bool           `json:"fresh"`
	StaleReasons           []string       `json:"stale_reasons"`
	AcknowledgedOwnerIDs   []string       `json:"acknowledged_owner_ids"`
	CreatedBy              string         `json:"created_by"`
	CreatedAt              time.Time      `json:"created_at"`
}

type PlanCreation struct {
	PullRequestID   string
	Revision        string
	Definition      Definition
	Candidate       Revision
	CandidatePath   string
	CandidateDigest string
	Policies        []PolicyEffect
}

func (s *Store) CreatePlan(repo, actor string, in PlanCreation) (ChangePlan, error) {
	var out ChangePlan
	err := s.lock(func() error {
		if strings.TrimSpace(in.PullRequestID) == "" || in.Revision == "" || in.Definition.RepositoryID != repo || in.Definition.CurrentVersion < 1 || in.Candidate.Revision != in.Revision || in.CandidatePath == "" || len(in.CandidateDigest) != 64 || validateRevision(in.Candidate) != nil || len(in.Policies) == 0 {
			return ErrInvalid
		}
		for _, p := range in.Policies {
			if p.Path == "" || len(p.Digest) != 64 || len(p.Effects) == 0 || unsafe(p.Path, strings.Join(p.Effects, " ")) {
				return ErrInvalid
			}
		}
		base := in.Definition.Revisions[in.Definition.CurrentVersion-1]
		changes, owners, err := buildChanges(base.Resources, in.Candidate.Resources)
		if err != nil || len(changes) == 0 {
			return ErrInvalid
		}
		now := s.now()
		out = ChangePlan{ID: randomID(), RepositoryID: repo, PullRequestID: in.PullRequestID, SourceRevision: in.Revision, DefinitionID: in.Definition.ID, DefinitionVersion: in.Definition.CurrentVersion, ObservationFingerprint: observationFingerprint(in.Definition), ObservationsValidUntil: observationExpiry(in.Definition), Candidate: in.Candidate, CandidatePath: in.CandidatePath, CandidateDigest: in.CandidateDigest, Changes: changes, PolicyEffects: in.Policies, AffectedOwnerIDs: owners, Events: []PlanEvent{}, Fresh: true, StaleReasons: []string{}, AcknowledgedOwnerIDs: []string{}, CreatedBy: actor, CreatedAt: now}
		return s.writePlan(out)
	})
	return out, err
}

func buildChanges(before, after []Resource) ([]PlanChange, []string, error) {
	b, a := map[string]Resource{}, map[string]Resource{}
	for _, x := range before {
		b[x.ID] = x
	}
	for _, x := range after {
		a[x.ID] = x
	}
	ids := map[string]bool{}
	for id := range b {
		ids[id] = true
	}
	for id := range a {
		ids[id] = true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	changes := []PlanChange{}
	owners := map[string]bool{}
	for _, id := range ordered {
		old, had := b[id]
		next, has := a[id]
		action := ""
		switch {
		case !had:
			action = "create"
		case !has:
			action = "destroy"
		default:
			ob, _ := json.Marshal(old)
			nb, _ := json.Marshal(next)
			if string(ob) == string(nb) {
				continue
			}
			if old.Kind != next.Kind || old.Provider != next.Provider || old.ProviderRef != next.ProviderRef {
				action = "replace"
			} else {
				action = "change"
			}
		}
		var bp, ap *Resource
		deps := []string{}
		if had {
			x := old
			bp = &x
			for _, o := range old.OwnerIDs {
				owners[o] = true
			}
		}
		if has {
			x := next
			ap = &x
			deps = append(deps, next.DependsOn...)
			for _, o := range next.OwnerIDs {
				owners[o] = true
			}
		}
		risks := deriveRisks(action, old, next, had, has)
		if len(risks) == 0 {
			return nil, nil, ErrInvalid
		}
		rollback := "Recreate from the frozen prior declaration and permitted provider observation before dependent changes proceed."
		if action == "destroy" || action == "replace" {
			rollback = "Rollback may be impossible after provider deletion or data movement; preserve and verify recovery evidence before this step."
		}
		changes = append(changes, PlanChange{ResourceID: id, Action: action, Before: bp, After: ap, DependencyIDs: deps, Risks: risks, RollbackLimit: rollback})
	}
	depthFor := func(resources map[string]Resource) func(string, map[string]bool) int {
		memo := map[string]int{}
		var visit func(string, map[string]bool) int
		visit = func(id string, active map[string]bool) int {
			if v, ok := memo[id]; ok {
				return v
			}
			if active[id] {
				return 0
			}
			active[id] = true
			d := 0
			for _, dep := range resources[id].DependsOn {
				if x := visit(dep, active) + 1; x > d {
					d = x
				}
			}
			delete(active, id)
			memo[id] = d
			return d
		}
		return visit
	}
	afterDepth, beforeDepth := depthFor(a), depthFor(b)
	sort.SliceStable(changes, func(i, j int) bool {
		di, dj := afterDepth(changes[i].ResourceID, map[string]bool{}), afterDepth(changes[j].ResourceID, map[string]bool{})
		if changes[i].Action == "destroy" {
			di = -beforeDepth(changes[i].ResourceID, map[string]bool{})
		}
		if changes[j].Action == "destroy" {
			dj = -beforeDepth(changes[j].ResourceID, map[string]bool{})
		}
		if di == dj {
			return changes[i].ResourceID < changes[j].ResourceID
		}
		return di < dj
	})
	for i := range changes {
		changes[i].Order = i + 1
	}
	outOwners := make([]string, 0, len(owners))
	for id := range owners {
		outOwners = append(outOwners, id)
	}
	sort.Strings(outOwners)
	return changes, outOwners, nil
}

func deriveRisks(action string, old, next Resource, had, has bool) []PlanRisk {
	x := next
	if !has {
		x = old
	}
	out := []PlanRisk{}
	add := func(k, sev, msg, mit string) { out = append(out, PlanRisk{k, sev, msg, mit}) }
	if action == "destroy" || action == "replace" {
		add("availability", "high", "Replacement or removal can interrupt dependent services.", "Sequence dependants and verify recovery before continuing.")
	}
	for _, v := range x.Commitments.Security {
		add("security", "medium", v, "Revalidate the declared security commitment.")
	}
	for _, v := range x.Commitments.Privacy {
		add("privacy", "medium", v, "Confirm data handling with the affected owner.")
	}
	for _, v := range x.Commitments.Continuity {
		add("continuity", "medium", v, "Verify recovery and continuity assumptions.")
	}
	for _, c := range x.Constraints {
		if c.Kind == "cost" {
			add("cost", "medium", c.Note+" "+formatLimit(c), "Review the estimate and configured limit.")
		}
	}
	if x.Kind == "data_store" && (action == "destroy" || action == "replace") {
		add("data", "high", "Persistent data may be moved or deleted.", "Require a tested recovery point and explicit retention decision.")
	}
	if len(out) == 0 {
		add("availability", "low", "The resource declaration changes.", "Verify the resource and its dependants after the change.")
	}
	present := map[string]bool{}
	for _, r := range out {
		present[r.Kind] = true
	}
	defaults := []struct{ k, msg, mit string }{{"availability", "No additional availability risk is declared.", "Validate dependency health and availability assumptions."}, {"security", "No additional security risk is declared.", "Revalidate access boundaries and least privilege."}, {"privacy", "No additional privacy risk is declared.", "Confirm data categories, regions, and affected subjects."}, {"continuity", "No additional continuity risk is declared.", "Confirm recovery and teardown assumptions."}, {"cost", "No additional cost risk is declared.", "Verify the provider estimate against the declared limit."}, {"data", "No additional persistent-data risk is declared.", "Confirm whether the resource stores or transforms durable data."}}
	for _, d := range defaults {
		if !present[d.k] {
			add(d.k, "low", d.msg, d.mit)
		}
	}
	return out
}

func formatLimit(c Constraint) string { b, _ := json.Marshal(c.Limit); return string(b) + " " + c.Unit }
func observationFingerprint(v Definition) string {
	b, _ := json.Marshal(v.Observations)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func observationExpiry(v Definition) *time.Time {
	var earliest *time.Time
	for _, o := range v.Observations {
		if o.DefinitionVersion != v.CurrentVersion {
			continue
		}
		x := o.ObservedAt.Add(24 * time.Hour)
		if earliest == nil || x.Before(*earliest) {
			earliest = &x
		}
	}
	return earliest
}

func (s *Store) AddPlanEvent(id, actor, actorType string, expectedEvents int, e PlanEvent) (ChangePlan, error) {
	var out ChangePlan
	err := s.lock(func() error {
		p, er := s.readPlan(id)
		if er != nil {
			return er
		}
		return s.addPlanEventLocked(&p, actor, actorType, expectedEvents, e, &out)
	})
	return out, err
}

func (s *Store) addPlanEventLocked(p *ChangePlan, actor, actorType string, expectedEvents int, e PlanEvent, out *ChangePlan) error {
	if len(p.Events) != expectedEvents {
		return ErrConflict
	}
	if (e.Kind != "assumption" && e.Kind != "impact" && e.Kind != "acknowledgement_request" && e.Kind != "owner_acknowledgement") || strings.TrimSpace(e.Body) == "" || unsafe(e.Body) {
		return ErrInvalid
	}
	for _, rid := range e.ResourceIDs {
		found := false
		for _, c := range p.Changes {
			if c.ResourceID == rid {
				found = true
				break
			}
		}
		if !found {
			return ErrInvalid
		}
	}
	if e.Kind == "acknowledgement_request" && !contains(p.AffectedOwnerIDs, e.OwnerID) {
		return ErrInvalid
	}
	if e.Kind == "owner_acknowledgement" && (actorType != "human" || e.OwnerID != actor || !contains(p.AffectedOwnerIDs, actor)) {
		return ErrInvalid
	}
	e.ID = randomID()
	e.ActorID = actor
	e.ActorType = actorType
	e.CreatedAt = s.now()
	p.Events = append(p.Events, e)
	*out = *p
	return s.writePlan(*p)
}

// AddPlanEventCurrent checks definition and observation freshness under the
// same infrastructure-store lock that appends the event. Callers hold the
// pull source-revision lock around this method, making the append linearizable
// with both pull movement and infrastructure dependency changes.
func (s *Store) AddPlanEventCurrent(id, actor, actorType string, expectedEvents int, e PlanEvent) (ChangePlan, error) {
	var out ChangePlan
	err := s.lock(func() error {
		p, err := s.readPlan(id)
		if err != nil {
			return err
		}
		current, err := s.read(p.DefinitionID)
		if err != nil {
			return ErrPlanStale
		}
		if current.CurrentVersion != p.DefinitionVersion || observationFingerprint(current) != p.ObservationFingerprint || (p.ObservationsValidUntil != nil && s.now().After(*p.ObservationsValidUntil)) {
			return ErrPlanStale
		}
		if err := s.addPlanEventLocked(&p, actor, actorType, expectedEvents, e, &out); err != nil {
			return err
		}
		projectCurrentPlan(&out)
		return nil
	})
	return out, err
}

func projectCurrentPlan(p *ChangePlan) {
	p.Fresh = true
	p.StaleReasons = []string{}
	p.AcknowledgedOwnerIDs = []string{}
	seen := map[string]bool{}
	for _, e := range p.Events {
		if e.Kind == "owner_acknowledgement" {
			seen[e.OwnerID] = true
		}
	}
	for id := range seen {
		p.AcknowledgedOwnerIDs = append(p.AcknowledgedOwnerIDs, id)
	}
	sort.Strings(p.AcknowledgedOwnerIDs)
}

func (s *Store) ListPlans(repo, pull string) ([]ChangePlan, error) {
	out := []ChangePlan{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.planDir(repo))
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		for _, x := range entries {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			p, e := s.readPlanFile(filepath.Join(s.planDir(repo), x.Name()))
			if e != nil {
				return e
			}
			if p.PullRequestID == pull {
				out = append(out, p)
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, err
}
func (s *Store) ProjectPlan(p ChangePlan, current Definition, currentRevision string, policyCurrent func(PolicyEffect) bool) ChangePlan {
	reasons := []string{}
	if currentRevision != p.SourceRevision {
		reasons = append(reasons, "pull_source_changed")
	}
	if current.ID != p.DefinitionID || current.CurrentVersion != p.DefinitionVersion {
		reasons = append(reasons, "definition_changed")
	}
	if observationFingerprint(current) != p.ObservationFingerprint {
		reasons = append(reasons, "observed_state_changed")
	}
	if p.ObservationsValidUntil != nil && s.now().After(*p.ObservationsValidUntil) {
		reasons = append(reasons, "provider_observation_expired")
	}
	for _, x := range p.PolicyEffects {
		if !policyCurrent(x) {
			reasons = append(reasons, "policy_changed")
			break
		}
	}
	p.Fresh = len(reasons) == 0
	p.StaleReasons = reasons
	if p.Fresh {
		projectCurrentPlan(&p)
	} else {
		p.AcknowledgedOwnerIDs = []string{}
	}
	return p
}
func (s *Store) GetPlan(id string) (ChangePlan, error) {
	var p ChangePlan
	err := s.lock(func() error { var e error; p, e = s.readPlan(id); return e })
	return p, err
}
func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func (s *Store) planDir(repo string) string { return filepath.Join(s.repoDir(repo), "plans") }
func (s *Store) writePlan(p ChangePlan) error {
	d := s.planDir(p.RepositoryID)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(d, ".plan-*.tmp")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(d, p.ID+".json"))
}
func (s *Store) readPlan(id string) (ChangePlan, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return ChangePlan{}, e
	}
	for _, r := range entries {
		if !r.IsDir() {
			continue
		}
		p := filepath.Join(s.root, r.Name(), "plans", id+".json")
		v, e := s.readPlanFile(p)
		if e == nil {
			return v, nil
		}
		if !os.IsNotExist(e) {
			return ChangePlan{}, e
		}
	}
	return ChangePlan{}, ErrPlanNotFound
}
func (s *Store) readPlanFile(path string) (ChangePlan, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return ChangePlan{}, e
	}
	var p ChangePlan
	if json.Unmarshal(b, &p) != nil {
		return p, ErrInvalid
	}
	return p, nil
}
