package apicontracts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// OperationalObservation is aggregate, payload-free evidence about one
// application using one exact published contract revision.
type OperationalObservation struct {
	ID              string    `json:"id"`
	ApplicationID   string    `json:"application_id"`
	ContractID      string    `json:"contract_id"`
	ContractVersion int       `json:"contract_version"`
	Environment     string    `json:"environment"`
	ReleaseID       string    `json:"release_id"`
	WindowStartedAt time.Time `json:"window_started_at"`
	WindowEndedAt   time.Time `json:"window_ended_at"`
	Requests        int64     `json:"requests"`
	Available       int64     `json:"available"`
	LatencyP95MS    int64     `json:"latency_p95_ms"`
	QuotaRejected   int64     `json:"quota_rejected"`
	Errors          int64     `json:"errors"`
	SchemaValid     int64     `json:"schema_valid"`
	UsageUnits      int64     `json:"usage_units"`
	ErrorCodes      []string  `json:"error_codes"`
	Sanitization    string    `json:"sanitization"`
	Visibility      string    `json:"visibility"`
	RecordedBy      string    `json:"recorded_by"`
	CreatedAt       time.Time `json:"created_at"`
}

type InvestigationFinding struct {
	ID             string    `json:"id"`
	ActorType      string    `json:"actor_type"`
	ActorID        string    `json:"actor_id"`
	Classification string    `json:"classification"`
	Summary        string    `json:"summary"`
	EvidenceIDs    []string  `json:"evidence_ids"`
	Confidence     string    `json:"confidence"`
	Uncertainty    string    `json:"uncertainty"`
	CreatedAt      time.Time `json:"created_at"`
}

type SandboxReproduction struct {
	ID              string    `json:"id"`
	ObservationID   string    `json:"observation_id"`
	OperationID     string    `json:"operation_id"`
	Failure         string    `json:"failure,omitempty"`
	ResultStatus    int       `json:"result_status"`
	ResultCode      string    `json:"result_code"`
	SyntheticOnly   bool      `json:"synthetic_only"`
	PayloadRetained bool      `json:"payload_retained"`
	ActorID         string    `json:"actor_id"`
	CreatedAt       time.Time `json:"created_at"`
}

type InvestigationHandoff struct {
	Kind               string    `json:"kind"`
	RepositoryID       string    `json:"repository_id"`
	ResourceID         string    `json:"resource_id"`
	FindingID          string    `json:"finding_id"`
	IntegrationWorkID  string    `json:"integration_work_id,omitempty"`
	AcceptanceCriteria []string  `json:"acceptance_criteria"`
	CreatedBy          string    `json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
}

type APIInvestigation struct {
	ID              string                 `json:"id"`
	ApplicationID   string                 `json:"application_id"`
	ContractID      string                 `json:"contract_id"`
	ContractVersion int                    `json:"contract_version"`
	ObservationIDs  []string               `json:"observation_ids"`
	Title           string                 `json:"title"`
	OpenedBy        string                 `json:"opened_by"`
	InvitedAgentIDs []string               `json:"invited_agent_ids"`
	Findings        []InvestigationFinding `json:"findings"`
	Reproductions   []SandboxReproduction  `json:"reproductions"`
	Handoff         *InvestigationHandoff  `json:"handoff,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
}

func (s *Store) AddOperationalObservation(app Application, actor string, v OperationalObservation) (OperationalObservation, error) {
	var out OperationalObservation
	err := s.lock(func() error {
		if app.Status != "approved" || v.ApplicationID != "" && v.ApplicationID != app.ID || v.ContractVersion != 0 && v.ContractVersion != app.ContractVersion || !slices.Contains(app.Environments, v.Environment) || v.ReleaseID == "" || len(v.ReleaseID) > 160 || v.WindowStartedAt.IsZero() || !v.WindowEndedAt.After(v.WindowStartedAt) || v.WindowEndedAt.After(s.now().Add(time.Minute)) || v.Requests < 0 || v.Available < 0 || v.Available > v.Requests || v.Errors < 0 || v.Errors > v.Requests || v.SchemaValid < 0 || v.SchemaValid > v.Requests || v.QuotaRejected < 0 || v.QuotaRejected > v.Requests || v.LatencyP95MS < 0 || v.UsageUnits < 0 || strings.TrimSpace(v.Sanitization) == "" || len(v.Sanitization) > 1000 || len(v.ErrorCodes) > 32 || slices.ContainsFunc(v.ErrorCodes, func(x string) bool { return strings.TrimSpace(x) == "" || len(x) > 120 }) || !map[string]bool{"shared": true, "producer_only": true, "consumer_only": true}[v.Visibility] || unsafeOperationalText(v.Sanitization+" "+strings.Join(v.ErrorCodes, " ")) {
			return ErrInvalid
		}
		v.ID, v.ApplicationID, v.ContractID, v.ContractVersion = randomID(), app.ID, app.ContractID, app.ContractVersion
		v.RecordedBy, v.CreatedAt = actor, s.now()
		out = v
		return s.writeOperational("observations", v.ID, v)
	})
	return out, err
}

func (s *Store) ListOperationalObservations(applicationID string) ([]OperationalObservation, error) {
	out := []OperationalObservation{}
	err := s.readOperationalDir("observations", func(b []byte) error {
		var v OperationalObservation
		if e := json.Unmarshal(b, &v); e != nil {
			return e
		}
		if v.ApplicationID == applicationID {
			out = append(out, v)
		}
		return nil
	})
	return out, err
}

func (s *Store) CreateAPIInvestigation(app Application, actor, title string, evidenceIDs []string) (APIInvestigation, error) {
	var out APIInvestigation
	err := s.lock(func() error {
		if strings.TrimSpace(title) == "" || len(evidenceIDs) == 0 || len(evidenceIDs) > 32 {
			return ErrInvalid
		}
		observations, e := s.listOperationalObservationsUnlocked(app.ID)
		if e != nil {
			return e
		}
		for _, id := range evidenceIDs {
			if !slices.ContainsFunc(observations, func(v OperationalObservation) bool { return v.ID == id && v.ContractVersion == app.ContractVersion }) {
				return ErrInvalid
			}
		}
		out = APIInvestigation{ID: randomID(), ApplicationID: app.ID, ContractID: app.ContractID, ContractVersion: app.ContractVersion, ObservationIDs: slices.Clone(evidenceIDs), Title: strings.TrimSpace(title), OpenedBy: actor, InvitedAgentIDs: []string{}, Findings: []InvestigationFinding{}, Reproductions: []SandboxReproduction{}, CreatedAt: s.now()}
		return s.writeOperational("investigations", out.ID, out)
	})
	return out, err
}

func (s *Store) ListAPIInvestigations(applicationID string) ([]APIInvestigation, error) {
	out := []APIInvestigation{}
	err := s.readOperationalDir("investigations", func(b []byte) error {
		var v APIInvestigation
		if e := json.Unmarshal(b, &v); e != nil {
			return e
		}
		if v.ApplicationID == applicationID {
			out = append(out, v)
		}
		return nil
	})
	return out, err
}
func (s *Store) GetAPIInvestigation(id string) (APIInvestigation, error) {
	var v APIInvestigation
	b, e := os.ReadFile(filepath.Join(s.root, "investigations", id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}

func (s *Store) UpdateAPIInvestigation(id string, mutate func(*APIInvestigation) error) (APIInvestigation, error) {
	var out APIInvestigation
	err := s.lock(func() error {
		v, e := s.GetAPIInvestigation(id)
		if e != nil {
			return e
		}
		if e = mutate(&v); e != nil {
			return e
		}
		out = v
		return s.writeOperational("investigations", id, v)
	})
	return out, err
}

func unsafeOperationalText(v string) bool {
	v = strings.ToLower(v)
	for _, x := range []string{"authorization:", "bearer ", "password=", "token=", "secret=", "private key", "vva_", "request body", "response body"} {
		if strings.Contains(v, x) {
			return true
		}
	}
	return false
}
func (s *Store) listOperationalObservationsUnlocked(id string) ([]OperationalObservation, error) {
	out := []OperationalObservation{}
	err := s.readOperationalDir("observations", func(b []byte) error {
		var v OperationalObservation
		if e := json.Unmarshal(b, &v); e != nil {
			return e
		}
		if v.ApplicationID == id {
			out = append(out, v)
		}
		return nil
	})
	return out, err
}
func (s *Store) readOperationalDir(dir string, fn func([]byte) error) error {
	entries, e := os.ReadDir(filepath.Join(s.root, dir))
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
		b, e := os.ReadFile(filepath.Join(s.root, dir, x.Name()))
		if e != nil {
			return e
		}
		if e = fn(b); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) writeOperational(dir, id string, v any) error {
	path := filepath.Join(s.root, dir)
	if e := os.MkdirAll(path, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(path, ".record-")
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
		e = os.Rename(name, filepath.Join(path, id+".json"))
	}
	return e
}

var ErrAlreadyHandedOff = errors.New("investigation already handed off")

func NewOperationalID() string { return randomID() }
