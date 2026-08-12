package extensions

import "time"

type OperationalNotice struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Action   string `json:"action"`
}
type Operations struct {
	InstallationID        string              `json:"installation_id"`
	ContractVersion       int                 `json:"contract_version"`
	LatestContractVersion int                 `json:"latest_contract_version"`
	Requests              int                 `json:"requests"`
	ActionInvocations     int                 `json:"action_invocations"`
	Deliveries            int                 `json:"deliveries"`
	Failures              int                 `json:"failures"`
	DeadLetters           int                 `json:"dead_letters"`
	AverageLatencyMS      int64               `json:"average_latency_ms"`
	PayloadBytes          int64               `json:"payload_bytes"`
	PermissionUse         map[string]int      `json:"permission_use"`
	Notices               []OperationalNotice `json:"notices"`
	Credentials           []CredentialHealth  `json:"credentials"`
	GeneratedAt           time.Time           `json:"generated_at"`
}
type CredentialHealth struct {
	ID         string     `json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

func (s *Store) InspectOperations(installationID string) (Operations, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.readInstallation(installationID)
	if err != nil {
		return Operations{}, err
	}
	ext, err := s.readExtension(v.ExtensionID)
	if err != nil {
		return Operations{}, err
	}
	deliveries, err := s.readDeliveries(installationID)
	if err != nil {
		return Operations{}, err
	}
	o := Operations{InstallationID: v.ID, ContractVersion: v.ContractVersion, LatestContractVersion: ext.ContractVersion, PermissionUse: map[string]int{}, Notices: []OperationalNotice{}, GeneratedAt: s.now().Truncate(time.Microsecond)}
	var latency time.Duration
	var completed int64
	for _, d := range deliveries {
		o.Deliveries++
		o.PayloadBytes += int64(len(d.Delivery.Payload))
		o.PermissionUse["events:"+d.EventType]++
		if d.Status == "dead_letter" {
			o.DeadLetters++
		}
		for _, a := range d.Attempts {
			if a.Status == "failed" {
				o.Failures++
			}
			if a.Status == "delivered" {
				latency += a.At.Sub(d.CreatedAt)
				completed++
			}
		}
	}
	if completed > 0 {
		o.AverageLatencyMS = (latency / time.Duration(completed)).Milliseconds()
	}
	contributions, err := s.readInstallationContributions(v.ID)
	if err != nil {
		return Operations{}, err
	}
	for _, c := range contributions {
		o.Requests++
		o.PermissionUse[c.ResourceType+":write"]++
		o.ActionInvocations += len(c.Invocations)
	}
	if ext.ContractVersion > v.ContractVersion {
		o.Notices = append(o.Notices, OperationalNotice{Code: "renewed_consent_required", Severity: "warning", Message: "The operator published a newer contract; existing authority has not changed.", Action: "Review and explicitly upgrade this installation."})
	}
	if o.DeadLetters > 0 {
		o.Notices = append(o.Notices, OperationalNotice{Code: "broken_delivery_endpoint", Severity: "critical", Message: "Event deliveries exhausted retries.", Action: "Test the callback endpoint, then retry or quarantine."})
	}
	if o.Failures >= 3 {
		o.Notices = append(o.Notices, OperationalNotice{Code: "delivery_failures", Severity: "warning", Message: "Repeated delivery failures were detected.", Action: "Inspect attempt history and endpoint health."})
	}
	if o.Requests >= 80 {
		o.Notices = append(o.Notices, OperationalNotice{Code: "anomalous_consumption", Severity: "warning", Message: "Contribution consumption is approaching the hourly limit.", Action: "Inspect requests and narrow access or quarantine."})
	}
	return o, nil
}
