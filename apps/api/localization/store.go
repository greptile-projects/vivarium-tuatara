// Package localization persists revision-exact localization extraction and translation work.
package localization

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("localization review not found")
var ErrInvalid = errors.New("invalid localization review")
var ErrConflict = errors.New("localization workspace version conflict")

type ExtractionMap struct {
	ID      string   `json:"id"`
	Version int      `json:"version"`
	Name    string   `json:"name"`
	Include []string `json:"include"`
	Formats []string `json:"formats"`
}
type Variable struct {
	Name    string `json:"name"`
	Example string `json:"example"`
}
type Location struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Component string `json:"component,omitempty"`
}
type Unit struct {
	ID           string            `json:"id"`
	Key          string            `json:"key"`
	Message      string            `json:"message"`
	Context      string            `json:"context"`
	Screenshot   string            `json:"screenshot,omitempty"`
	Variables    []Variable        `json:"variables"`
	PluralRule   string            `json:"plural_rule,omitempty"`
	Locations    []Location        `json:"source_locations"`
	SourceHash   string            `json:"source_hash"`
	Change       string            `json:"change"`
	LocaleStatus map[string]string `json:"locale_status"`
	Protected    bool              `json:"protected,omitempty"`
	Embargoed    bool              `json:"embargoed,omitempty"`
}
type Extraction struct {
	ID             string        `json:"id"`
	PullID         string        `json:"pull_id"`
	SourceRevision string        `json:"source_revision"`
	Map            ExtractionMap `json:"map"`
	Locales        []string      `json:"locales"`
	Units          []Unit        `json:"units"`
	Removed        []Unit        `json:"removed_units"`
	CreatedBy      string        `json:"created_by"`
	CreatedAt      time.Time     `json:"created_at"`
}
type Translation struct {
	ID         string    `json:"id"`
	UnitID     string    `json:"unit_id"`
	Locale     string    `json:"locale"`
	SourceHash string    `json:"source_hash"`
	Text       string    `json:"text"`
	Note       string    `json:"note,omitempty"`
	Status     string    `json:"status"`
	ProposedBy string    `json:"proposed_by"`
	CreatedAt  time.Time `json:"created_at"`
}
type Evidence struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
	Excerpt   string `json:"excerpt,omitempty"`
}
type Claim struct {
	ID                 string    `json:"id"`
	UnitID             string    `json:"unit_id"`
	Locale             string    `json:"locale"`
	AssigneeID         string    `json:"assignee_id"`
	State              string    `json:"state"`
	Note               string    `json:"note,omitempty"`
	PreviousAssigneeID string    `json:"previous_assignee_id,omitempty"`
	CreatedBy          string    `json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
}
type Comment struct {
	ID        string    `json:"id"`
	UnitID    string    `json:"unit_id"`
	Locale    string    `json:"locale"`
	Body      string    `json:"body"`
	ActorType string    `json:"actor_type"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type SuggestionRequest struct {
	ID                string    `json:"id"`
	UnitID            string    `json:"unit_id"`
	Locale            string    `json:"locale"`
	AgentID           string    `json:"agent_id"`
	ProductContext    string    `json:"product_context"`
	LocalePlanID      string    `json:"locale_plan_id"`
	LocalePlanVersion int       `json:"locale_plan_version"`
	Protected         bool      `json:"protected"`
	Embargoed         bool      `json:"embargoed"`
	State             string    `json:"state"`
	RequestedBy       string    `json:"requested_by"`
	CreatedAt         time.Time `json:"created_at"`
}
type Suggestion struct {
	ID                string     `json:"id"`
	RequestID         string     `json:"request_id"`
	UnitID            string     `json:"unit_id"`
	Locale            string     `json:"locale"`
	Text              string     `json:"text"`
	Rationale         string     `json:"rationale"`
	Uncertainty       string     `json:"uncertainty"`
	Evidence          []Evidence `json:"evidence"`
	AgentID           string     `json:"agent_id"`
	SourceRevision    string     `json:"source_revision"`
	SourceHash        string     `json:"source_hash"`
	LocalePlanID      string     `json:"locale_plan_id"`
	LocalePlanVersion int        `json:"locale_plan_version"`
	Status            string     `json:"status"`
	CreatedAt         time.Time  `json:"created_at"`
}
type Decision struct {
	ID            string    `json:"id"`
	UnitID        string    `json:"unit_id"`
	Locale        string    `json:"locale"`
	SuggestionID  string    `json:"suggestion_id,omitempty"`
	TranslationID string    `json:"translation_id,omitempty"`
	Kind          string    `json:"kind"`
	Reason        string    `json:"reason"`
	ActorID       string    `json:"actor_id"`
	CreatedAt     time.Time `json:"created_at"`
}
type VerificationRoute struct {
	JourneyID     string `json:"journey_id"`
	Route         string `json:"route"`
	InterfaceHash string `json:"interface_hash"`
}
type VerificationCandidate struct {
	ID                  string              `json:"id"`
	Locale              string              `json:"locale"`
	PreviewID           string              `json:"preview_id"`
	PreviewURL          string              `json:"preview_url"`
	SourceRevision      string              `json:"source_revision"`
	LocalePlanID        string              `json:"locale_plan_id"`
	LocalePlanVersion   int                 `json:"locale_plan_version"`
	Routes              []VerificationRoute `json:"routes"`
	TranslationVersions map[string]string   `json:"translation_versions"`
	CreatedBy           string              `json:"created_by"`
	CreatedAt           time.Time           `json:"created_at"`
}
type VerificationResult struct {
	Kind     string   `json:"kind"`
	Route    string   `json:"route"`
	UnitIDs  []string `json:"unit_ids"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Artifact string   `json:"artifact,omitempty"`
}
type VerificationRun struct {
	ID          string               `json:"id"`
	CandidateID string               `json:"candidate_id"`
	Results     []VerificationResult `json:"results"`
	CreatedBy   string               `json:"created_by"`
	CreatedAt   time.Time            `json:"created_at"`
}
type LocaleFinding struct {
	ID          string    `json:"id"`
	CandidateID string    `json:"candidate_id"`
	Locale      string    `json:"locale"`
	Route       string    `json:"route"`
	UnitIDs     []string  `json:"unit_ids"`
	Category    string    `json:"category"`
	Severity    string    `json:"severity"`
	Body        string    `json:"body"`
	AuthorID    string    `json:"author_id"`
	CreatedAt   time.Time `json:"created_at"`
}
type LocaleReviewDecision struct {
	ID          string    `json:"id"`
	CandidateID string    `json:"candidate_id"`
	Locale      string    `json:"locale"`
	Route       string    `json:"route"`
	UnitIDs     []string  `json:"unit_ids"`
	Kind        string    `json:"kind"`
	Reason      string    `json:"reason"`
	ActorID     string    `json:"actor_id"`
	ActorRole   string    `json:"actor_role"`
	CreatedAt   time.Time `json:"created_at"`
}
type VerificationProjection struct {
	CandidateID string   `json:"candidate_id"`
	Current     bool     `json:"current"`
	StaleScopes []string `json:"stale_scopes"`
}
type Review struct {
	RepositoryID           string                    `json:"repository_id"`
	PullID                 string                    `json:"pull_id"`
	CurrentRevision        string                    `json:"current_revision"`
	Extractions            []Extraction              `json:"extractions"`
	Translations           []Translation             `json:"translations"`
	Counts                 map[string]map[string]int `json:"counts"`
	WorkspaceVersion       int                       `json:"workspace_version"`
	Claims                 []Claim                   `json:"claims"`
	Comments               []Comment                 `json:"comments"`
	SuggestionRequests     []SuggestionRequest       `json:"suggestion_requests"`
	Suggestions            []Suggestion              `json:"suggestions"`
	Decisions              []Decision                `json:"decisions"`
	VerificationCandidates []VerificationCandidate   `json:"verification_candidates"`
	VerificationRuns       []VerificationRun         `json:"verification_runs"`
	LocaleFindings         []LocaleFinding           `json:"locale_findings"`
	LocaleReviewDecisions  []LocaleReviewDecision    `json:"locale_review_decisions"`
	Verification           []VerificationProjection  `json:"verification"`
}
type Store struct {
	root                string
	mu                  sync.Mutex
	now                 func() time.Time
	resolvePlanVersions func(repositoryID string, planIDs []string) (map[string]int, error)
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}

// ConfigureLocalePlanVersions connects delivery evaluation to the external
// locale-plan store. Production configures this once at startup; keeping the
// dependency as a resolver avoids making localization persistence own plans.
func (s *Store) ConfigureLocalePlanVersions(resolve func(string, []string) (map[string]int, error)) {
	s.resolvePlanVersions = resolve
}

func (s *Store) Extract(repo, pull, revision, actor string, m ExtractionMap, locales []string, units []Unit) (Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, pull)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Review{}, err
	}
	if len(revision) != 40 || strings.TrimSpace(m.ID) == "" || m.Version < 1 || strings.TrimSpace(m.Name) == "" || len(m.Include) == 0 || len(m.Formats) == 0 || len(locales) == 0 || len(units) == 0 {
		return Review{}, ErrInvalid
	}
	localeSet := map[string]bool{}
	for _, l := range locales {
		if strings.TrimSpace(l) == "" || localeSet[l] {
			return Review{}, ErrInvalid
		}
		localeSet[l] = true
	}
	prior := map[string]Unit{}
	if len(v.Extractions) > 0 {
		for _, u := range v.Extractions[len(v.Extractions)-1].Units {
			prior[u.ID] = u
		}
	}
	seen := map[string]bool{}
	for i := range units {
		u := &units[i]
		if strings.TrimSpace(u.Key) == "" || strings.TrimSpace(u.Message) == "" || strings.TrimSpace(u.Context) == "" || len(u.Locations) == 0 {
			return Review{}, ErrInvalid
		}
		for _, x := range u.Locations {
			if strings.TrimSpace(x.Path) == "" || x.Line < 1 {
				return Review{}, ErrInvalid
			}
		}
		u.ID = stable(m.ID, u.Key)
		if seen[u.ID] {
			return Review{}, ErrInvalid
		}
		seen[u.ID] = true
		u.SourceHash = hashUnit(*u)
		u.Change = "added"
		if old, ok := prior[u.ID]; ok {
			if old.SourceHash == u.SourceHash {
				u.Change = "reused"
			} else {
				u.Change = "changed"
			}
		}
		u.LocaleStatus = map[string]string{}
	}
	removed := []Unit{}
	for id, u := range prior {
		if !seen[id] {
			u.Change = "removed"
			removed = append(removed, u)
		}
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i].ID < removed[j].ID })
	e := Extraction{ID: id(), PullID: pull, SourceRevision: revision, Map: m, Locales: locales, Units: units, Removed: removed, CreatedBy: actor, CreatedAt: s.now()}
	v.RepositoryID = repo
	v.PullID = pull
	v.CurrentRevision = revision
	v.Extractions = append(v.Extractions, e)
	v.WorkspaceVersion++
	s.project(&v)
	if err := s.write(v); err != nil {
		return Review{}, err
	}
	return v, nil
}
func (s *Store) Propose(repo, pull, revision, unitID, locale, text, note, actor string) (Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(repo, pull)
	if e != nil {
		return v, e
	}
	if v.CurrentRevision != revision || strings.TrimSpace(text) == "" || len(v.Extractions) == 0 {
		return v, ErrInvalid
	}
	var unit *Unit
	latest := &v.Extractions[len(v.Extractions)-1]
	for i := range latest.Units {
		if latest.Units[i].ID == unitID {
			unit = &latest.Units[i]
		}
	}
	validLocale := false
	for _, l := range latest.Locales {
		validLocale = validLocale || l == locale
	}
	if unit == nil || !validLocale {
		return v, ErrInvalid
	}
	for i := range v.Translations {
		if v.Translations[i].UnitID == unitID && v.Translations[i].Locale == locale && v.Translations[i].Status == "proposed" {
			v.Translations[i].Status = "superseded"
		}
	}
	v.Translations = append(v.Translations, Translation{ID: id(), UnitID: unitID, Locale: locale, SourceHash: unit.SourceHash, Text: text, Note: note, Status: "proposed", ProposedBy: actor, CreatedAt: s.now()})
	v.WorkspaceVersion++
	s.project(&v)
	if e = s.write(v); e != nil {
		return Review{}, e
	}
	return v, nil
}

func (s *Store) Mutate(repo, pull, revision string, expected int, mutation string, actorType, actorID string, payload map[string]any) (Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, pull)
	if err != nil {
		return v, err
	}
	if v.CurrentRevision != revision || v.WorkspaceVersion != expected || actorID == "" {
		return v, ErrConflict
	}
	unitID, _ := payload["unit_id"].(string)
	locale, _ := payload["locale"].(string)
	unit := currentUnit(&v, unitID, locale)
	if unit == nil {
		return v, ErrInvalid
	}
	now := s.now()
	switch mutation {
	case "claim", "handoff", "release":
		if actorType != "user" {
			return v, ErrInvalid
		}
		assignee, _ := payload["assignee_id"].(string)
		note, _ := payload["note"].(string)
		active := activeClaim(v.Claims, unitID, locale)
		if mutation == "claim" {
			if active != nil || assignee != actorID {
				return v, ErrConflict
			}
		}
		if mutation != "claim" && (active == nil || active.AssigneeID != actorID) {
			return v, ErrConflict
		}
		if mutation == "release" {
			assignee = ""
		} else if assignee == "" {
			return v, ErrInvalid
		}
		previous := ""
		if active != nil {
			previous = active.AssigneeID
		}
		v.Claims = append(v.Claims, Claim{ID: id(), UnitID: unitID, Locale: locale, AssigneeID: assignee, State: mutation, Note: note, PreviousAssigneeID: previous, CreatedBy: actorID, CreatedAt: now})
	case "comment":
		body, _ := payload["body"].(string)
		if strings.TrimSpace(body) == "" || len(body) > 4000 {
			return v, ErrInvalid
		}
		v.Comments = append(v.Comments, Comment{ID: id(), UnitID: unitID, Locale: locale, Body: body, ActorType: actorType, ActorID: actorID, CreatedAt: now})
	case "request_suggestion":
		if actorType != "user" {
			return v, ErrInvalid
		}
		agent, _ := payload["agent_id"].(string)
		context, _ := payload["product_context"].(string)
		plan, _ := payload["locale_plan_id"].(string)
		version := number(payload["locale_plan_version"])
		protected, _ := payload["protected"].(bool)
		embargoed, _ := payload["embargoed"].(bool)
		if agent == "" || context == "" || plan == "" || version < 1 || protected || embargoed || unit.Protected || unit.Embargoed {
			return v, ErrInvalid
		}
		v.SuggestionRequests = append(v.SuggestionRequests, SuggestionRequest{ID: id(), UnitID: unitID, Locale: locale, AgentID: agent, ProductContext: context, LocalePlanID: plan, LocalePlanVersion: version, Protected: protected, Embargoed: embargoed, State: "requested", RequestedBy: actorID, CreatedAt: now})
	case "suggest":
		if actorType != "agent" {
			return v, ErrInvalid
		}
		requestID, _ := payload["request_id"].(string)
		textValue, _ := payload["text"].(string)
		rationale, _ := payload["rationale"].(string)
		uncertainty, _ := payload["uncertainty"].(string)
		request := findRequest(v.SuggestionRequests, requestID)
		if request == nil || request.AgentID != actorID || request.State != "requested" || request.UnitID != unitID || request.Locale != locale || unit.Protected || unit.Embargoed || textValue == "" || rationale == "" || !validUncertainty(uncertainty) {
			return v, ErrInvalid
		}
		evidence := decodeEvidence(payload["evidence"])
		hasSource, hasPlan := false, false
		for _, item := range evidence {
			hasSource = hasSource || item.Kind == "source_context"
			hasPlan = hasPlan || item.Kind == "locale_plan" || item.Kind == "terminology"
		}
		if len(evidence) == 0 || !hasSource || !hasPlan {
			return v, ErrInvalid
		}
		v.Suggestions = append(v.Suggestions, Suggestion{ID: id(), RequestID: request.ID, UnitID: unitID, Locale: locale, Text: textValue, Rationale: rationale, Uncertainty: uncertainty, Evidence: evidence, AgentID: actorID, SourceRevision: revision, SourceHash: unit.SourceHash, LocalePlanID: request.LocalePlanID, LocalePlanVersion: request.LocalePlanVersion, Status: "suggested", CreatedAt: now})
		request.State = "completed"
	case "decide":
		if actorType != "user" {
			return v, ErrInvalid
		}
		kind, _ := payload["kind"].(string)
		reason, _ := payload["reason"].(string)
		suggestionID, _ := payload["suggestion_id"].(string)
		translationID, _ := payload["translation_id"].(string)
		if !contains([]string{"approve", "reject", "escalate"}, kind) || reason == "" || (suggestionID == "" && translationID == "") {
			return v, ErrInvalid
		}
		if suggestionID != "" {
			found := false
			for i := range v.Suggestions {
				if v.Suggestions[i].ID == suggestionID && v.Suggestions[i].UnitID == unitID && v.Suggestions[i].Locale == locale {
					v.Suggestions[i].Status = map[string]string{"approve": "approved", "reject": "rejected", "escalate": "escalated"}[kind]
					found = true
				}
			}
			if !found {
				return v, ErrInvalid
			}
		}
		v.Decisions = append(v.Decisions, Decision{ID: id(), UnitID: unitID, Locale: locale, SuggestionID: suggestionID, TranslationID: translationID, Kind: kind, Reason: reason, ActorID: actorID, CreatedAt: now})
	default:
		return v, ErrInvalid
	}
	v.WorkspaceVersion++
	s.project(&v)
	if err = s.write(v); err != nil {
		return Review{}, err
	}
	return v, nil
}

// Verify appends locale-preview evidence under the same revision and CAS boundary
// as translation work. Preview, plan, and reviewer admission are revalidated by
// the HTTP boundary while this method validates the retained evidence graph.
func (s *Store) Verify(repo, pull, revision string, expected int, mutation, actorID, actorRole string, currentPlanVersion int, payload map[string]any) (Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, pull)
	if err != nil {
		return v, err
	}
	if v.CurrentRevision != revision || v.WorkspaceVersion != expected || actorID == "" {
		return v, ErrConflict
	}
	now := s.now()
	switch mutation {
	case "publish_candidate":
		var candidate VerificationCandidate
		if !decode(payload, &candidate) || candidate.Locale == "" || candidate.PreviewID == "" || candidate.PreviewURL == "" || candidate.LocalePlanID == "" || candidate.LocalePlanVersion < 1 || candidate.LocalePlanVersion != currentPlanVersion || len(candidate.Routes) == 0 {
			return v, ErrInvalid
		}
		if currentUnit(&v, v.Extractions[len(v.Extractions)-1].Units[0].ID, candidate.Locale) == nil {
			return v, ErrInvalid
		}
		seen := map[string]bool{}
		for _, route := range candidate.Routes {
			if route.JourneyID == "" || !validRoute(route.Route) || len(route.InterfaceHash) != 64 || seen[route.Route] {
				return v, ErrInvalid
			}
			seen[route.Route] = true
		}
		candidate.ID, candidate.SourceRevision, candidate.CreatedBy, candidate.CreatedAt = id(), revision, actorID, now
		candidate.TranslationVersions = currentTranslations(v, candidate.Locale)
		if len(candidate.TranslationVersions) != len(v.Extractions[len(v.Extractions)-1].Units) {
			return v, ErrInvalid
		}
		v.VerificationCandidates = append(v.VerificationCandidates, candidate)
	case "record_checks":
		candidateID, _ := payload["candidate_id"].(string)
		candidate := findCandidate(v.VerificationCandidates, candidateID)
		var results []VerificationResult
		if candidate == nil || candidate.LocalePlanVersion != currentPlanVersion || !candidateIsCurrent(v, candidateID) || !decode(payload["results"], &results) || !validResults(*candidate, v, results) {
			return v, ErrInvalid
		}
		v.VerificationRuns = append(v.VerificationRuns, VerificationRun{ID: id(), CandidateID: candidateID, Results: results, CreatedBy: actorID, CreatedAt: now})
	case "finding":
		var finding LocaleFinding
		if !decode(payload, &finding) {
			return v, ErrInvalid
		}
		candidate := findCandidate(v.VerificationCandidates, finding.CandidateID)
		if candidate == nil || candidate.LocalePlanVersion != currentPlanVersion || !candidateIsCurrent(v, finding.CandidateID) || candidate.Locale != finding.Locale || !candidateRoute(*candidate, finding.Route) || !validUnits(v, finding.UnitIDs) || !contains(verificationKinds(), finding.Category) || !contains([]string{"low", "medium", "high", "blocking"}, finding.Severity) || strings.TrimSpace(finding.Body) == "" || len(finding.Body) > 4000 {
			return v, ErrInvalid
		}
		finding.ID, finding.AuthorID, finding.CreatedAt = id(), actorID, now
		v.LocaleFindings = append(v.LocaleFindings, finding)
	case "review":
		var decision LocaleReviewDecision
		if !decode(payload, &decision) {
			return v, ErrInvalid
		}
		candidate := findCandidate(v.VerificationCandidates, decision.CandidateID)
		if candidate == nil || candidate.LocalePlanVersion != currentPlanVersion || !candidateIsCurrent(v, decision.CandidateID) || candidate.Locale != decision.Locale || !candidateRoute(*candidate, decision.Route) || !validUnits(v, decision.UnitIDs) || !contains([]string{"approve", "reject"}, decision.Kind) || strings.TrimSpace(decision.Reason) == "" || len(decision.Reason) > 2000 || !contains([]string{"translator", "regional_reviewer"}, actorRole) {
			return v, ErrInvalid
		}
		decision.ID, decision.ActorID, decision.ActorRole, decision.CreatedAt = id(), actorID, actorRole, now
		v.LocaleReviewDecisions = append(v.LocaleReviewDecisions, decision)
	default:
		return v, ErrInvalid
	}
	v.WorkspaceVersion++
	s.project(&v)
	if err := s.write(v); err != nil {
		return Review{}, err
	}
	return v, nil
}

func decode(value any, out any) bool {
	b, err := json.Marshal(value)
	return err == nil && json.Unmarshal(b, out) == nil
}
func validRoute(route string) bool {
	return strings.HasPrefix(route, "/") && len(route) <= 500 && !strings.ContainsAny(route, "\r\n")
}
func verificationKinds() []string {
	return []string{"variables", "pluralization", "formatting", "terminology", "links", "layout_expansion", "bidirectional_text", "fallback_behavior", "localized_journey"}
}
func findCandidate(values []VerificationCandidate, candidateID string) *VerificationCandidate {
	for i := range values {
		if values[i].ID == candidateID {
			return &values[i]
		}
	}
	return nil
}
func candidateIsCurrent(v Review, candidateID string) bool {
	for _, projection := range v.Verification {
		if projection.CandidateID == candidateID {
			return projection.Current
		}
	}
	return false
}
func candidateRoute(candidate VerificationCandidate, route string) bool {
	for _, value := range candidate.Routes {
		if value.Route == route {
			return true
		}
	}
	return false
}
func validUnits(v Review, ids []string) bool {
	if len(ids) == 0 {
		return false
	}
	known := map[string]bool{}
	for _, unit := range v.Extractions[len(v.Extractions)-1].Units {
		known[unit.ID] = true
	}
	seen := map[string]bool{}
	for _, unitID := range ids {
		if seen[unitID] || !known[unitID] {
			return false
		}
		seen[unitID] = true
	}
	return true
}
func validResults(candidate VerificationCandidate, v Review, results []VerificationResult) bool {
	if len(results) < len(verificationKinds())*len(candidate.Routes) {
		return false
	}
	seen := map[string]bool{}
	for _, result := range results {
		key := result.Route + "\x00" + result.Kind
		if !contains(verificationKinds(), result.Kind) || seen[key] || !candidateRoute(candidate, result.Route) || !validUnits(v, result.UnitIDs) || !contains([]string{"passed", "failed"}, result.Status) || strings.TrimSpace(result.Summary) == "" || len(result.Summary) > 2000 {
			return false
		}
		seen[key] = true
	}
	for _, route := range candidate.Routes {
		for _, kind := range verificationKinds() {
			if !seen[route.Route+"\x00"+kind] {
				return false
			}
		}
	}
	return true
}
func currentTranslations(v Review, locale string) map[string]string {
	out := map[string]string{}
	for _, translation := range v.Translations {
		if translation.Locale == locale && translation.Status == "proposed" {
			out[translation.UnitID] = translation.ID
		}
	}
	return out
}

func currentUnit(v *Review, unitID, locale string) *Unit {
	if len(v.Extractions) == 0 {
		return nil
	}
	x := &v.Extractions[len(v.Extractions)-1]
	valid := false
	for _, l := range x.Locales {
		valid = valid || l == locale
	}
	if !valid {
		return nil
	}
	for i := range x.Units {
		if x.Units[i].ID == unitID {
			return &x.Units[i]
		}
	}
	return nil
}
func activeClaim(values []Claim, u, l string) *Claim {
	for i := len(values) - 1; i >= 0; i-- {
		x := &values[i]
		if x.UnitID == u && x.Locale == l {
			if x.AssigneeID == "" {
				return nil
			}
			return x
		}
	}
	return nil
}
func findRequest(values []SuggestionRequest, id string) *SuggestionRequest {
	for i := range values {
		if values[i].ID == id {
			return &values[i]
		}
	}
	return nil
}
func number(v any) int {
	if x, ok := v.(float64); ok {
		return int(x)
	}
	return 0
}
func validUncertainty(v string) bool { return contains([]string{"low", "medium", "high"}, v) }
func contains(v []string, x string) bool {
	for _, y := range v {
		if x == y {
			return true
		}
	}
	return false
}
func decodeEvidence(v any) []Evidence {
	b, e := json.Marshal(v)
	if e != nil {
		return nil
	}
	var out []Evidence
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	for _, x := range out {
		if !contains([]string{"terminology", "prior_translation", "source_context", "locale_plan"}, x.Kind) || x.Reference == "" {
			return nil
		}
	}
	return out
}
func (s *Store) Get(repo, pull, current string) (Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(repo, pull)
	if e != nil {
		return v, e
	}
	v.CurrentRevision = current
	s.project(&v)
	if len(v.Extractions) > 0 && v.Extractions[len(v.Extractions)-1].SourceRevision != current {
		// Retain the earlier extraction and translation history for context, but
		// never project it as actionable work for a newer pull revision.
		v.Counts = map[string]map[string]int{}
		latest := &v.Extractions[len(v.Extractions)-1]
		for i := range latest.Units {
			for locale := range latest.Units[i].LocaleStatus {
				latest.Units[i].LocaleStatus[locale] = "stale"
			}
		}
		for i := range v.Translations {
			if v.Translations[i].Status == "proposed" {
				v.Translations[i].Status = "stale"
			}
		}
	}
	return v, nil
}
func (s *Store) project(v *Review) {
	v.Counts = map[string]map[string]int{}
	if len(v.Extractions) == 0 {
		return
	}
	latest := &v.Extractions[len(v.Extractions)-1]
	current := map[string]map[string]bool{}
	for i := range v.Translations {
		t := &v.Translations[i]
		match := false
		for _, u := range latest.Units {
			match = match || u.ID == t.UnitID && u.SourceHash == t.SourceHash
		}
		if !match && t.Status == "proposed" {
			t.Status = "superseded"
		}
		if t.Status == "proposed" {
			if current[t.UnitID] == nil {
				current[t.UnitID] = map[string]bool{}
			}
			current[t.UnitID][t.Locale] = true
		}
	}
	for i := range latest.Units {
		u := &latest.Units[i]
		u.LocaleStatus = map[string]string{}
		for _, l := range latest.Locales {
			status := "untranslated"
			if current[u.ID][l] {
				status = "proposed"
			}
			u.LocaleStatus[l] = status
			if v.Counts[l] == nil {
				v.Counts[l] = map[string]int{}
			}
			v.Counts[l][u.Change]++
			if status == "untranslated" {
				v.Counts[l][status]++
			}
		}
	}
	for _, u := range latest.Removed {
		for _, l := range latest.Locales {
			if v.Counts[l] == nil {
				v.Counts[l] = map[string]int{}
			}
			v.Counts[l][u.Change]++
		}
	}
	projectVerification(v)
}

func projectVerification(v *Review) {
	v.Verification = nil
	if len(v.Extractions) == 0 {
		return
	}
	currentRevision := v.CurrentRevision
	latestRoutes := map[string]map[string]string{}
	for _, candidate := range v.VerificationCandidates {
		if candidate.SourceRevision != currentRevision {
			continue
		}
		if latestRoutes[candidate.Locale] == nil {
			latestRoutes[candidate.Locale] = map[string]string{}
		}
		for _, route := range candidate.Routes {
			latestRoutes[candidate.Locale][route.Route] = route.InterfaceHash
		}
	}
	for _, candidate := range v.VerificationCandidates {
		stale := []string{}
		if candidate.SourceRevision != currentRevision {
			stale = append(stale, "source_revision")
		}
		current := currentTranslations(*v, candidate.Locale)
		for unitID, translationID := range candidate.TranslationVersions {
			if current[unitID] != translationID {
				stale = append(stale, "translation:"+unitID)
			}
		}
		for unitID, translationID := range current {
			if candidate.TranslationVersions[unitID] != translationID && !contains(stale, "translation:"+unitID) {
				stale = append(stale, "translation:"+unitID)
			}
		}
		for _, route := range candidate.Routes {
			if hash := latestRoutes[candidate.Locale][route.Route]; hash != "" && hash != route.InterfaceHash {
				stale = append(stale, "interface:"+route.Route)
			}
		}
		sort.Strings(stale)
		v.Verification = append(v.Verification, VerificationProjection{CandidateID: candidate.ID, Current: len(stale) == 0, StaleScopes: stale})
	}
}

// ApplyLocalePlanVersions adds the external locale-plan dependency to the
// otherwise self-contained localization projection. Callers obtain these
// versions from the locale-plan store at read time, so a plan successor makes
// old experience evidence visibly stale without rewriting retained history.
func ApplyLocalePlanVersions(v *Review, current map[string]int) {
	for i := range v.Verification {
		projection := &v.Verification[i]
		candidate := findCandidate(v.VerificationCandidates, projection.CandidateID)
		if candidate == nil || current[candidate.LocalePlanID] == candidate.LocalePlanVersion {
			continue
		}
		if !contains(projection.StaleScopes, "locale_plan") {
			projection.StaleScopes = append(projection.StaleScopes, "locale_plan")
			sort.Strings(projection.StaleScopes)
		}
		projection.Current = false
	}
}
func (s *Store) read(repo, pull string) (Review, error) {
	var v Review
	b, e := os.ReadFile(filepath.Join(s.root, repo, pull+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if e = json.Unmarshal(b, &v); e != nil {
		return v, e
	}
	if v.RepositoryID != repo || v.PullID != pull || len(v.CurrentRevision) != 40 || len(v.Extractions) == 0 {
		return Review{}, ErrInvalid
	}
	for _, extraction := range v.Extractions {
		if extraction.ID == "" || extraction.PullID != pull || len(extraction.SourceRevision) != 40 || extraction.Map.ID == "" || extraction.Map.Version < 1 || len(extraction.Locales) == 0 || len(extraction.Units) == 0 {
			return Review{}, ErrInvalid
		}
	}
	return v, nil
}
func (s *Store) write(v Review) error {
	d := filepath.Join(s.root, v.RepositoryID)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(d, ".localization-*")
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
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(d, v.PullID+".json"))
	}
	return e
}
func stable(m, k string) string {
	h := sha256.Sum256([]byte(m + "\x00" + k))
	return hex.EncodeToString(h[:16])
}
func hashUnit(u Unit) string {
	u.ID = ""
	u.SourceHash = ""
	u.Change = ""
	u.LocaleStatus = nil
	b, _ := json.Marshal(u)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
