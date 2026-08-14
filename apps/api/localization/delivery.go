package localization

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type DeliveryPolicy struct {
	ID                string    `json:"id"`
	RepositoryID      string    `json:"repository_id"`
	Branch            string    `json:"branch"`
	LocalePlanID      string    `json:"locale_plan_id"`
	LocalePlanVersion int       `json:"locale_plan_version"`
	Locales           []string  `json:"locales"`
	Audiences         []string  `json:"audiences"`
	RiskClasses       []string  `json:"risk_classes"`
	RequiredChecks    []string  `json:"required_checks"`
	MinimumReviews    int       `json:"minimum_reviews"`
	CreatedBy         string    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
}
type LocaleDisposition struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	PolicyID     string    `json:"policy_id"`
	ReleaseID    string    `json:"release_id,omitempty"`
	Revision     string    `json:"revision"`
	Locale       string    `json:"locale"`
	State        string    `json:"state"`
	Reason       string    `json:"reason"`
	ActorID      string    `json:"actor_id"`
	CreatedAt    time.Time `json:"created_at"`
}
type LocaleRequirement struct {
	PolicyID    string   `json:"policy_id"`
	Locale      string   `json:"locale"`
	Audiences   []string `json:"audiences,omitempty"`
	RiskClasses []string `json:"risk_classes,omitempty"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
}
type LocaleReadiness struct {
	Ready        bool                `json:"ready"`
	Revision     string              `json:"revision"`
	Requirements []LocaleRequirement `json:"requirements"`
	Locales      map[string]string   `json:"locales"`
}
type Publication struct {
	ID                string    `json:"id"`
	RepositoryID      string    `json:"repository_id"`
	Kind              string    `json:"kind"`
	ResourceID        string    `json:"resource_id"`
	ReleaseID         string    `json:"release_id,omitempty"`
	Version           string    `json:"version"`
	Revision          string    `json:"revision"`
	Locale            string    `json:"locale"`
	LocalePlanID      string    `json:"locale_plan_id"`
	LocalePlanVersion int       `json:"locale_plan_version"`
	SourceLocale      string    `json:"source_locale"`
	FallbackLocale    string    `json:"fallback_locale,omitempty"`
	FallbackState     string    `json:"fallback_state"`
	URL               string    `json:"url"`
	Status            string    `json:"status"`
	PublishedBy       string    `json:"published_by"`
	PublishedAt       time.Time `json:"published_at"`
}
type LocaleRepair struct {
	OwnerType          string    `json:"owner_type"`
	OwnerID            string    `json:"owner_id"`
	WorkURL            string    `json:"work_url"`
	AcceptanceCriteria string    `json:"acceptance_criteria"`
	CreatedBy          string    `json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
}
type PublishedFinding struct {
	ID               string        `json:"id"`
	RepositoryID     string        `json:"repository_id"`
	PublicationID    string        `json:"publication_id"`
	Locale           string        `json:"locale"`
	Category         string        `json:"category"`
	Route            string        `json:"route"`
	UnitKey          string        `json:"unit_key,omitempty"`
	Expected         string        `json:"expected"`
	Observed         string        `json:"observed"`
	EvidenceURL      string        `json:"evidence_url,omitempty"`
	ReporterID       string        `json:"reporter_id"`
	Status           string        `json:"status"`
	ValidationReason string        `json:"validation_reason,omitempty"`
	ValidatedBy      string        `json:"validated_by,omitempty"`
	Repair           *LocaleRepair `json:"repair,omitempty"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}
type deliveryData struct {
	Policies     []DeliveryPolicy    `json:"policies"`
	Dispositions []LocaleDisposition `json:"dispositions"`
	Publications []Publication       `json:"publications"`
	Findings     []PublishedFinding  `json:"findings"`
}

func (s *Store) deliveryPath(repo string) string { return filepath.Join(s.root, repo+"-delivery.json") }
func (s *Store) readDelivery(repo string) (deliveryData, error) {
	var d deliveryData
	b, err := os.ReadFile(s.deliveryPath(repo))
	if errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	if json.Unmarshal(b, &d) != nil {
		return d, ErrInvalid
	}
	return d, nil
}
func (s *Store) writeDelivery(repo string, d deliveryData) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".locale-delivery-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, s.deliveryPath(repo))
}
func validCommitID(v string) bool {
	if len(v) != 40 {
		return false
	}
	for _, c := range v {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func cleanUnique(values []string) ([]string, bool) {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return nil, false
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out, len(out) > 0
}

func (s *Store) CreateDeliveryPolicy(repo, actor string, p DeliveryPolicy) (DeliveryPolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ok bool
	p.Locales, ok = cleanUnique(p.Locales)
	if !ok {
		return p, ErrInvalid
	}
	p.RequiredChecks, _ = cleanUnique(p.RequiredChecks)
	p.Audiences, ok = cleanOptionalUnique(p.Audiences)
	if !ok {
		return p, ErrInvalid
	}
	p.RiskClasses, ok = cleanOptionalUnique(p.RiskClasses)
	if !ok {
		return p, ErrInvalid
	}
	if strings.TrimSpace(p.Branch) == "" || p.LocalePlanID == "" || p.LocalePlanVersion < 1 || p.MinimumReviews < 0 {
		return p, ErrInvalid
	}
	d, e := s.readDelivery(repo)
	if e != nil {
		return p, e
	}
	p.ID = id()
	p.RepositoryID = repo
	p.CreatedBy = actor
	p.CreatedAt = s.now()
	d.Policies = append(d.Policies, p)
	return p, s.writeDelivery(repo, d)
}
func (s *Store) Delivery(repo string) (deliveryData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readDelivery(repo)
}
func (s *Store) SetLocaleDisposition(repo, actor string, v LocaleDisposition) (LocaleDisposition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validCommitID(v.Revision) || (v.State != "staged" && v.State != "deferred" && v.State != "withdrawn") || strings.TrimSpace(v.Reason) == "" {
		return v, ErrInvalid
	}
	d, e := s.readDelivery(repo)
	if e != nil {
		return v, e
	}
	found := false
	for _, p := range d.Policies {
		if p.ID == v.PolicyID {
			for _, l := range p.Locales {
				found = found || l == v.Locale
			}
		}
	}
	if !found {
		return v, ErrInvalid
	}
	v.ID = id()
	v.RepositoryID = repo
	v.ActorID = actor
	v.CreatedAt = s.now()
	d.Dispositions = append(d.Dispositions, v)
	return v, s.writeDelivery(repo, d)
}
func (s *Store) EvaluateDelivery(repo, pullID, releaseID, revision, branch string, audiences, risks []string, checks map[string]string) (LocaleReadiness, error) {
	d, e := s.Delivery(repo)
	r := LocaleReadiness{Ready: true, Revision: revision, Requirements: []LocaleRequirement{}, Locales: map[string]string{}}
	if e != nil {
		return r, e
	}
	review, _ := s.Get(repo, pullID, revision)
	if s.resolvePlanVersions != nil && len(review.VerificationCandidates) > 0 {
		seen, planIDs := map[string]bool{}, []string{}
		for _, candidate := range review.VerificationCandidates {
			if candidate.LocalePlanID != "" && !seen[candidate.LocalePlanID] {
				seen[candidate.LocalePlanID] = true
				planIDs = append(planIDs, candidate.LocalePlanID)
			}
		}
		versions, resolveErr := s.resolvePlanVersions(repo, planIDs)
		if resolveErr != nil {
			return r, resolveErr
		}
		ApplyLocalePlanVersions(&review, versions)
	}
	for _, p := range d.Policies {
		if p.Branch != branch || !deliverySelected(p.Audiences, audiences) || !deliverySelected(p.RiskClasses, risks) {
			continue
		}
		for _, locale := range p.Locales {
			state := "required"
			for _, x := range d.Dispositions {
				if x.PolicyID == p.ID && x.Locale == locale && x.Revision == revision && (x.ReleaseID == "" || x.ReleaseID == releaseID) {
					state = x.State
				}
			}
			r.Locales[locale] = state
			if state == "deferred" || state == "withdrawn" {
				continue
			}
			for _, name := range p.RequiredChecks {
				status := checks[name]
				if status == "" {
					status = "missing"
				}
				r.Requirements = append(r.Requirements, LocaleRequirement{PolicyID: p.ID, Locale: locale, Audiences: p.Audiences, RiskClasses: p.RiskClasses, Kind: "check", Name: name, Status: status})
				if status != "passed" {
					r.Ready = false
				}
			}
			reviews := 0
			currentCandidates := map[string]bool{}
			for _, projection := range review.Verification {
				if !projection.Current {
					continue
				}
				for _, candidate := range review.VerificationCandidates {
					if candidate.ID == projection.CandidateID && candidate.Locale == locale && candidate.LocalePlanID == p.LocalePlanID && candidate.LocalePlanVersion == p.LocalePlanVersion {
						currentCandidates[candidate.ID] = true
					}
				}
			}
			for _, x := range review.LocaleReviewDecisions {
				if x.Locale == locale && x.Kind == "approve" && currentCandidates[x.CandidateID] {
					reviews++
				}
			}
			status := "passed"
			if reviews < p.MinimumReviews {
				status = "missing"
				r.Ready = false
			}
			r.Requirements = append(r.Requirements, LocaleRequirement{PolicyID: p.ID, Locale: locale, Audiences: p.Audiences, RiskClasses: p.RiskClasses, Kind: "review", Name: "regional review", Status: status})
		}
	}
	return r, nil
}
func deliverySelected(policy, context []string) bool {
	if len(policy) == 0 {
		return true
	}
	if len(context) == 0 {
		return false
	}
	for _, p := range policy {
		for _, c := range context {
			if p == c {
				return true
			}
		}
	}
	return false
}
func cleanOptionalUnique(values []string) ([]string, bool) {
	if len(values) == 0 {
		return []string{}, true
	}
	return cleanUnique(values)
}
func (s *Store) Publish(repo, actor string, p Publication) (Publication, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if (p.Kind != "application" && p.Kind != "documentation") || p.ResourceID == "" || p.Version == "" || !validCommitID(p.Revision) || p.Locale == "" || p.LocalePlanID == "" || p.LocalePlanVersion < 1 || p.SourceLocale == "" || p.URL == "" || (p.FallbackState != "complete" && p.FallbackState != "partial" && p.FallbackState != "fallback") || (p.FallbackState == "fallback" && p.FallbackLocale == "") || (p.Status != "published" && p.Status != "staged" && p.Status != "withdrawn") {
		return p, ErrInvalid
	}
	d, e := s.readDelivery(repo)
	if e != nil {
		return p, e
	}
	p.ID = id()
	p.RepositoryID = repo
	p.PublishedBy = actor
	p.PublishedAt = s.now()
	d.Publications = append(d.Publications, p)
	return p, s.writeDelivery(repo, d)
}
func (s *Store) ReportPublished(repo, actor string, f PublishedFinding) (PublishedFinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.readDelivery(repo)
	if e != nil {
		return f, e
	}
	var pub *Publication
	for i := range d.Publications {
		if d.Publications[i].ID == f.PublicationID {
			pub = &d.Publications[i]
		}
	}
	validCat := f.Category == "mistranslation" || f.Category == "cultural_mismatch" || f.Category == "broken_formatting" || f.Category == "missing_content"
	if pub == nil || f.Locale != pub.Locale || !validCat || f.Route == "" || strings.TrimSpace(f.Expected) == "" || strings.TrimSpace(f.Observed) == "" {
		return f, ErrInvalid
	}
	f.ID = id()
	f.RepositoryID = repo
	f.ReporterID = actor
	f.Status = "reported"
	f.CreatedAt = s.now()
	f.UpdatedAt = f.CreatedAt
	f.Repair = nil
	d.Findings = append(d.Findings, f)
	return f, s.writeDelivery(repo, d)
}
func (s *Store) DecidePublishedFinding(repo, id, actor, status, reason string, repair *LocaleRepair) (PublishedFinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.readDelivery(repo)
	if e != nil {
		return PublishedFinding{}, e
	}
	for i := range d.Findings {
		f := &d.Findings[i]
		if f.ID != id {
			continue
		}
		if (status != "validated" && status != "dismissed") || strings.TrimSpace(reason) == "" || f.Status != "reported" {
			return *f, ErrConflict
		}
		if repair != nil {
			if status != "validated" || (repair.OwnerType != "human" && repair.OwnerType != "agent") || repair.OwnerID == "" || !strings.HasPrefix(repair.WorkURL, "/repositories/"+repo+"/") || strings.TrimSpace(repair.AcceptanceCriteria) == "" {
				return *f, ErrInvalid
			}
			repair.CreatedBy = actor
			repair.CreatedAt = s.now()
		}
		f.Status = status
		f.ValidationReason = strings.TrimSpace(reason)
		f.ValidatedBy = actor
		f.Repair = repair
		f.UpdatedAt = s.now()
		return *f, s.writeDelivery(repo, d)
	}
	return PublishedFinding{}, ErrNotFound
}
