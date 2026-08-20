// Package assuranceassessments retains bounded independent assessment records.
package assuranceassessments

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("assurance assessment not found")
var ErrInvalid = errors.New("invalid assurance assessment")
var ErrConflict = errors.New("assurance assessment version conflict")
var ErrForbidden = errors.New("assurance assessment action forbidden")
var ErrExpired = errors.New("assurance assessment access expired")
var ErrNotStarted = errors.New("assurance assessment access has not started")

type Scope struct {
	ControlIDs     []string  `json:"control_ids"`
	SystemIDs      []string  `json:"system_ids"`
	ReleaseIDs     []string  `json:"release_ids"`
	PeriodStartsAt time.Time `json:"period_starts_at"`
	PeriodEndsAt   time.Time `json:"period_ends_at"`
}
type Assessor struct {
	UserID             string `json:"user_id"`
	Kind               string `json:"kind"`
	Organization       string `json:"organization,omitempty"`
	ConflictDisclosure string `json:"conflict_disclosure"`
	ConflictStatus     string `json:"conflict_status"`
}
type Event struct {
	ID                 string    `json:"id"`
	Kind               string    `json:"kind"`
	ActorID            string    `json:"actor_id"`
	Body               string    `json:"body"`
	ControlID          string    `json:"control_id,omitempty"`
	EvidencePackageIDs []string  `json:"evidence_package_ids,omitempty"`
	ParentID           string    `json:"parent_id,omitempty"`
	Status             string    `json:"status,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}
type Remediation struct {
	ID                             string                    `json:"id"`
	FindingEventID                 string                    `json:"finding_event_id"`
	ControlID                      string                    `json:"control_id"`
	AffectedRevision               string                    `json:"affected_revision"`
	VerifiedRevision               string                    `json:"verified_revision,omitempty"`
	Deadline                       time.Time                 `json:"deadline"`
	AcceptanceCriteria             []string                  `json:"acceptance_criteria"`
	ProposalID                     string                    `json:"proposal_id"`
	TaskIDs                        []string                  `json:"task_ids"`
	State                          string                    `json:"state"`
	Verification                   string                    `json:"verification,omitempty"`
	VerificationEvidencePackageIDs []string                  `json:"verification_evidence_package_ids,omitempty"`
	VerifiedBy                     string                    `json:"verified_by,omitempty"`
	VerifiedAt                     *time.Time                `json:"verified_at,omitempty"`
	Disposition                    string                    `json:"disposition,omitempty"`
	DispositionBy                  string                    `json:"disposition_by,omitempty"`
	CreatedBy                      string                    `json:"created_by"`
	CreatedAt                      time.Time                 `json:"created_at"`
	Verifications                  []RemediationVerification `json:"verifications"`
}
type RemediationVerification struct {
	ID                 string    `json:"id"`
	EvidencePackageIDs []string  `json:"evidence_package_ids,omitempty"`
	Summary            string    `json:"summary"`
	Disposition        string    `json:"disposition"`
	ActorID            string    `json:"actor_id"`
	ActorRole          string    `json:"actor_role"`
	CreatedAt          time.Time `json:"created_at"`
}
type Assessment struct {
	ID                 string        `json:"id"`
	RepositoryID       string        `json:"repository_id"`
	ProgramID          string        `json:"program_id"`
	ProgramVersion     int           `json:"program_version"`
	Title              string        `json:"title"`
	OwnerID            string        `json:"owner_id"`
	Assessor           Assessor      `json:"assessor"`
	Scope              Scope         `json:"scope"`
	EvidencePackageIDs []string      `json:"evidence_package_ids"`
	StartsAt           time.Time     `json:"starts_at"`
	ExpiresAt          time.Time     `json:"expires_at"`
	Status             string        `json:"status"`
	Version            int           `json:"version"`
	Events             []Event       `json:"events"`
	Remediations       []Remediation `json:"remediations"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}
type Store struct {
	root            string
	mu              sync.Mutex
	now             func() time.Time
	writeSigningKey func(*os.File, []byte) error
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }, writeSigningKey: func(file *os.File, value []byte) error {
		written, err := file.Write(value)
		if err == nil && written != len(value) {
			return io.ErrShortWrite
		}
		return err
	}}, nil
}
func (s *Store) Create(a Assessment) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !valid(a, now) {
		return Assessment{}, ErrInvalid
	}
	a.ID = id()
	a.Version = 1
	a.CreatedAt = now
	a.UpdatedAt = now
	a.Events = []Event{}
	a.Remediations = []Remediation{}
	if a.Assessor.ConflictDisclosure == "none" {
		a.Assessor.ConflictStatus = "clear"
		a.Status = "open"
	} else {
		a.Assessor.ConflictStatus = "pending"
		a.Status = "conflict_review"
	}
	return a, s.write(a)
}
func (s *Store) LinkRemediation(assessmentID string, expected int, actor string, work Remediation) (Assessment, error) {
	var out Assessment
	err := s.lock(func() error {
		a, err := s.read(assessmentID)
		if err != nil {
			return err
		}
		if a.Version != expected {
			return ErrConflict
		}
		finding, ok := findEvent(a.Events, work.FindingEventID)
		if !ok || finding.Kind != "finding" || finding.ControlID == "" || work.ControlID != finding.ControlID || len(work.AffectedRevision) != 40 || work.Deadline.IsZero() || !work.Deadline.After(s.now()) || len(work.AcceptanceCriteria) == 0 || work.ProposalID == "" || len(work.TaskIDs) == 0 {
			return ErrInvalid
		}
		for _, existing := range a.Remediations {
			if existing.FindingEventID == work.FindingEventID {
				return ErrConflict
			}
		}
		work.ID, work.State, work.CreatedBy, work.CreatedAt = id(), "delivering", actor, s.now()
		work.Verifications = []RemediationVerification{}
		a.Remediations = append(a.Remediations, work)
		a.Version++
		a.UpdatedAt = work.CreatedAt
		if err := s.write(a); err != nil {
			return err
		}
		out = a
		return nil
	})
	return out, err
}
func (s *Store) VerifyRemediation(assessmentID, remediationID string, expected int, actor, role, verification, disposition, verifiedRevision string, evidencePackageIDs []string) (Assessment, error) {
	var out Assessment
	err := s.lock(func() error {
		a, err := s.read(assessmentID)
		if err != nil {
			return err
		}
		if a.Version != expected {
			return ErrConflict
		}
		if !one(role, "owner", "assessor") || strings.TrimSpace(verification) == "" || credentialShaped(verification) || !one(disposition, "accepted", "rejected", "reopened") || (disposition == "accepted" && (len(evidencePackageIDs) == 0 || len(verifiedRevision) != 40)) || !unique(evidencePackageIDs) {
			return ErrInvalid
		}
		found := false
		now := s.now()
		for i := range a.Remediations {
			if a.Remediations[i].ID == remediationID {
				found = true
				a.Remediations[i].Verification, a.Remediations[i].VerifiedBy, a.Remediations[i].VerifiedAt, a.Remediations[i].Disposition, a.Remediations[i].DispositionBy = strings.TrimSpace(verification), actor, &now, disposition, actor
				a.Remediations[i].VerificationEvidencePackageIDs = append([]string(nil), evidencePackageIDs...)
				a.Remediations[i].VerifiedRevision = verifiedRevision
				a.Remediations[i].Verifications = append(a.Remediations[i].Verifications, RemediationVerification{ID: id(), EvidencePackageIDs: append([]string(nil), evidencePackageIDs...), Summary: strings.TrimSpace(verification), Disposition: disposition, ActorID: actor, ActorRole: role, CreatedAt: now})
				if disposition == "accepted" {
					a.Remediations[i].State = "verified"
				} else {
					a.Remediations[i].State = "open"
				}
			}
		}
		if !found {
			return ErrNotFound
		}
		a.Version++
		a.UpdatedAt = now
		if err := s.write(a); err != nil {
			return err
		}
		out = a
		return nil
	})
	return out, err
}
func findEvent(events []Event, eventID string) (Event, bool) {
	for _, e := range events {
		if e.ID == eventID {
			return e, true
		}
	}
	return Event{}, false
}

type Statement struct {
	ID               string     `json:"id"`
	RepositoryID     string     `json:"repository_id"`
	AssessmentID     string     `json:"assessment_id"`
	ReleaseID        string     `json:"release_id"`
	ReleaseRevision  string     `json:"release_revision"`
	ProgramID        string     `json:"program_id"`
	ProgramVersion   int        `json:"program_version"`
	Scope            Scope      `json:"scope"`
	ControlIDs       []string   `json:"control_ids"`
	ExceptionIDs     []string   `json:"exception_ids"`
	EvidenceDigest   string     `json:"evidence_digest"`
	Audience         []string   `json:"audience"`
	ExpiresAt        time.Time  `json:"expires_at"`
	IssuedBy         string     `json:"issued_by"`
	IssuedAt         time.Time  `json:"issued_at"`
	Payload          string     `json:"payload"`
	Signature        string     `json:"signature"`
	PublicKey        string     `json:"public_key"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevokedBy        string     `json:"revoked_by,omitempty"`
	RevocationReason string     `json:"revocation_reason,omitempty"`
}

func (s *Store) CreateStatement(v Statement) (Statement, error) {
	var out Statement
	err := s.lock(func() error {
		if v.RepositoryID == "" || v.AssessmentID == "" || v.ReleaseID == "" || len(v.ReleaseRevision) != 40 || v.ProgramID == "" || v.ProgramVersion < 1 || len(v.ControlIDs) == 0 || !unique(v.ControlIDs) || !unique(v.ExceptionIDs) || v.EvidenceDigest == "" || len(v.Audience) == 0 || !unique(v.Audience) || !v.ExpiresAt.After(s.now()) || v.ExpiresAt.After(s.now().Add(365*24*time.Hour)) {
			return ErrInvalid
		}
		v.ID, v.IssuedAt = id(), s.now()
		unsigned := v
		unsigned.Payload, unsigned.Signature, unsigned.PublicKey = "", "", ""
		raw, _ := json.Marshal(unsigned)
		sum := sha256.Sum256(raw)
		v.Payload = base64.RawURLEncoding.EncodeToString(raw)
		priv, pub, signErr := s.signingKey()
		if signErr != nil {
			return signErr
		}
		v.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, sum[:]))
		v.PublicKey = base64.RawURLEncoding.EncodeToString(pub)
		if mkdirErr := os.MkdirAll(filepath.Join(s.root, "statements"), 0700); mkdirErr != nil {
			return mkdirErr
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		if writeErr := os.WriteFile(filepath.Join(s.root, "statements", v.ID+".json"), b, 0600); writeErr != nil {
			return writeErr
		}
		out = v
		return nil
	})
	return out, err
}
func (s *Store) GetStatement(statementID string) (Statement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v Statement
	b, err := os.ReadFile(filepath.Join(s.root, "statements", statementID+".json"))
	if os.IsNotExist(err) {
		return v, ErrNotFound
	}
	if err != nil || json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) RevokeStatement(statementID, actor, reason string) (Statement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.getStatementUnlocked(statementID)
	if err != nil {
		return v, err
	}
	if strings.TrimSpace(reason) == "" || v.RevokedAt != nil {
		return v, ErrInvalid
	}
	now := s.now()
	v.RevokedAt, v.RevokedBy, v.RevocationReason = &now, actor, strings.TrimSpace(reason)
	b, _ := json.MarshalIndent(v, "", "  ")
	err = os.WriteFile(filepath.Join(s.root, "statements", v.ID+".json"), b, 0600)
	return v, err
}
func (s *Store) getStatementUnlocked(id string) (Statement, error) {
	var v Statement
	b, err := os.ReadFile(filepath.Join(s.root, "statements", id+".json"))
	if os.IsNotExist(err) {
		return v, ErrNotFound
	}
	if err != nil || json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) signingKey() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	path := filepath.Join(s.root, ".statement-signing-key")
	if b, err := os.ReadFile(path); err == nil {
		if len(b) != ed25519.PrivateKeySize {
			return nil, nil, ErrInvalid
		}
		k := ed25519.PrivateKey(b)
		return k, k.Public().(ed25519.PublicKey), nil
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.CreateTemp(s.root, ".statement-signing-key-pending-")
	if err != nil {
		return nil, nil, err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err = s.writeSigningKey(file, priv); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, nil, err
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		return nil, nil, err
	}
	directory, openErr := os.Open(s.root)
	if openErr != nil {
		return nil, nil, openErr
	}
	syncErr := directory.Sync()
	closeDirectoryErr := directory.Close()
	if syncErr != nil {
		return nil, nil, syncErr
	}
	if closeDirectoryErr != nil {
		return nil, nil, closeDirectoryErr
	}
	return priv, pub, err
}
func (s *Store) Get(id string) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}
func (s *Store) List(repo string) ([]Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Assessment{}
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	for _, x := range es {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		a, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		if a.RepositoryID == repo {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) Append(assessmentID string, expected int, actor, role string, e Event) (Assessment, error) {
	var out Assessment
	err := s.lock(func() error {
		a, err := s.read(assessmentID)
		if err != nil {
			return err
		}
		if a.Version != expected {
			return ErrConflict
		}
		now := s.now()
		if role == "assessor" && now.Before(a.StartsAt) {
			return ErrNotStarted
		}
		if role == "assessor" && !now.Before(a.ExpiresAt) {
			return ErrExpired
		}
		if a.Status == "closed" {
			return ErrForbidden
		}
		if a.Status == "conflict_review" && !(role == "owner" && e.Kind == "conflict_resolution") {
			return ErrForbidden
		}
		if !allowed(role, e.Kind) {
			return ErrForbidden
		}
		if strings.TrimSpace(e.Body) == "" || len(e.Body) > 8000 || credentialShaped(e.Body) || (e.Status != "" && !one(e.Status, "cleared", "rejected", "accepted", "contested", "open", "resolved", "pending", "upheld", "overturned", "unavailable")) {
			return ErrInvalid
		}
		if e.ControlID != "" && !contains(a.Scope.ControlIDs, e.ControlID) {
			return ErrInvalid
		}
		for _, pid := range e.EvidencePackageIDs {
			if !contains(a.EvidencePackageIDs, pid) {
				return ErrInvalid
			}
		}
		if e.ParentID != "" && !eventExists(a.Events, e.ParentID) {
			return ErrInvalid
		}
		if e.Kind == "conflict_resolution" {
			if a.Assessor.ConflictStatus != "pending" || !one(e.Status, "cleared", "rejected") {
				return ErrInvalid
			}
			a.Assessor.ConflictStatus = e.Status
			if e.Status == "cleared" {
				a.Status = "open"
			} else {
				a.Status = "closed"
			}
		}
		if e.Kind == "scope_change" {
			a.Status = "scope_changed"
		}
		if e.Kind == "scope_acknowledgement" {
			if a.Status != "scope_changed" {
				return ErrInvalid
			}
			a.Status = "open"
		}
		if e.Kind == "close" {
			a.Status = "closed"
		}
		e.ID = id()
		e.ActorID = actor
		e.CreatedAt = now
		a.Events = append(a.Events, e)
		a.Version++
		a.UpdatedAt = now
		if err := s.write(a); err != nil {
			return err
		}
		out = a
		return nil
	})
	return out, err
}
func valid(a Assessment, now time.Time) bool {
	return a.RepositoryID != "" && a.ProgramID != "" && a.ProgramVersion > 0 && a.Title != "" && a.OwnerID != "" && a.Assessor.UserID != "" && one(a.Assessor.Kind, "internal", "external") && a.Assessor.ConflictDisclosure != "" && !credentialShaped(a.Title) && !credentialShaped(a.Assessor.ConflictDisclosure) && len(a.Scope.ControlIDs) > 0 && unique(a.Scope.ControlIDs) && unique(a.Scope.SystemIDs) && unique(a.Scope.ReleaseIDs) && unique(a.EvidencePackageIDs) && !a.Scope.PeriodStartsAt.IsZero() && a.Scope.PeriodEndsAt.After(a.Scope.PeriodStartsAt) && !a.StartsAt.Before(now.Add(-time.Minute)) && a.ExpiresAt.After(a.StartsAt) && !a.ExpiresAt.After(a.StartsAt.Add(90*24*time.Hour))
}

func unique(values []string) bool {
	seen := map[string]bool{}
	for _, v := range values {
		if v == "" || seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}
func credentialShaped(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "-----begin private key-----") || strings.Contains(lower, "authorization: bearer ") || strings.Contains(lower, "ghp_") || strings.Contains(lower, "sk-")
}
func allowed(role, kind string) bool {
	if role == "owner" {
		return one(kind, "response", "sample_response", "walkthrough_response", "finding_response", "disagreement", "resolution", "appeal", "appeal_decision", "scope_change", "conflict_resolution", "close")
	}
	if role == "assessor" {
		return one(kind, "question", "sample_request", "walkthrough_request", "attestation_verification", "finding", "disagreement", "resolution", "appeal", "scope_acknowledgement")
	}
	return false
}
func one(v string, x ...string) bool {
	for _, y := range x {
		if v == y {
			return true
		}
	}
	return false
}
func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func eventExists(xs []Event, v string) bool {
	for _, x := range xs {
		if x.ID == v {
			return true
		}
	}
	return false
}
func id() string                       { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Assessment, error) {
	var a Assessment
	b, e := os.ReadFile(s.path(id))
	if os.IsNotExist(e) {
		return a, ErrNotFound
	}
	if e != nil {
		return a, e
	}
	if json.Unmarshal(b, &a) != nil {
		return a, ErrInvalid
	}
	return a, nil
}
func (s *Store) write(a Assessment) error {
	b, e := json.MarshalIndent(a, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".assessment-")
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
	if closeErr := tmp.Close(); e == nil {
		e = closeErr
	}
	if e == nil {
		e = os.Rename(name, s.path(a.ID))
	}
	if e == nil {
		if d, x := os.Open(s.root); x == nil {
			e = d.Sync()
			if closeErr := d.Close(); e == nil {
				e = closeErr
			}
		}
	}
	return e
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
