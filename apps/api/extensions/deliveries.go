package extensions

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const DeliverySchemaVersion = 1

var ErrDeliveryNotFound = errors.New("extension delivery not found")

type ProjectEvent struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	RepositoryID string    `json:"repository_id"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	ActorID      string    `json:"actor_id"`
	Title        string    `json:"title"`
	OccurredAt   time.Time `json:"occurred_at"`
}

type DeliveryEnvelope struct {
	SchemaVersion  int               `json:"schema_version"`
	DeliveryID     string            `json:"delivery_id"`
	InstallationID string            `json:"installation_id"`
	EventID        string            `json:"event_id"`
	EventType      string            `json:"event_type"`
	Sequence       int64             `json:"sequence"`
	OrderingKey    string            `json:"ordering_key"`
	OccurredAt     time.Time         `json:"occurred_at"`
	Repository     map[string]string `json:"repository"`
	Resource       map[string]string `json:"resource"`
	Actor          map[string]string `json:"actor"`
}

type DeliveryAttempt struct {
	Number       int       `json:"number"`
	Status       string    `json:"status"`
	ResponseCode int       `json:"response_code,omitempty"`
	Error        string    `json:"error,omitempty"`
	At           time.Time `json:"at"`
}
type Delivery struct {
	ID             string            `json:"id"`
	InstallationID string            `json:"installation_id"`
	EventID        string            `json:"event_id"`
	EventType      string            `json:"event_type"`
	SchemaVersion  int               `json:"schema_version"`
	Sequence       int64             `json:"sequence"`
	OrderingKey    string            `json:"ordering_key"`
	Status         string            `json:"status"`
	Payload        json.RawMessage   `json:"signed_payload"`
	PayloadSHA256  string            `json:"payload_sha256"`
	Signature      string            `json:"signature"`
	Attempts       []DeliveryAttempt `json:"attempts"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}
type DeliveryInspection struct {
	Delivery
	Payload map[string]any `json:"payload"`
}

func (s *Store) DeliveryPublicKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, lockErr := s.lock()
	if lockErr != nil {
		return ""
	}
	defer unlock()
	_, public, err := s.signingKey()
	if err != nil {
		return ""
	}
	return hex.EncodeToString(public)
}

// EnqueueProjectEvent projects only the identifiers and display title already
// permitted by an active installation. Event ID makes duplicate ingestion
// idempotent; each installation has its own monotonic ordering sequence.
func (s *Store) EnqueueProjectEvent(event ProjectEvent) ([]Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if len(event.ID) != 32 || len(event.RepositoryID) != 32 || event.Type == "" {
		return nil, ErrInvalid
	}
	installations, err := s.readAllInstallations()
	if err != nil {
		return nil, err
	}
	created := []Delivery{}
	for _, installation := range installations {
		if installation.Status != "active" || !contains(installation.RepositoryIDs, event.RepositoryID) || !subscribed(s.readExtensionUnchecked(installation.ExtensionID).SupportedEvents, event.Type) || !resourcePermittedAt(installation, event.RepositoryID, event.ResourceType, event.OccurredAt) {
			continue
		}
		idSum := sha256.Sum256([]byte(installation.ID + ":" + event.ID))
		id := hex.EncodeToString(idSum[:16])
		path := filepath.Join(s.root, "delivery-"+id+".json")
		if _, statErr := os.Stat(path); statErr == nil {
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return nil, statErr
		}
		sequence, seqErr := s.nextSequence(installation.ID)
		if seqErr != nil {
			return nil, seqErr
		}
		envelope := DeliveryEnvelope{SchemaVersion: DeliverySchemaVersion, DeliveryID: id, InstallationID: installation.ID, EventID: event.ID, EventType: event.Type, Sequence: sequence, OrderingKey: "repository:" + event.RepositoryID, OccurredAt: event.OccurredAt, Repository: map[string]string{"id": event.RepositoryID}, Resource: map[string]string{"type": event.ResourceType, "id": event.ResourceID, "title": event.Title}, Actor: map[string]string{"id": event.ActorID}}
		payload, _ := json.Marshal(envelope)
		sum := sha256.Sum256(payload)
		signature, signErr := s.sign(payload)
		if signErr != nil {
			return nil, signErr
		}
		now := s.now().Truncate(time.Microsecond)
		delivery := Delivery{ID: id, InstallationID: installation.ID, EventID: event.ID, EventType: event.Type, SchemaVersion: DeliverySchemaVersion, Sequence: sequence, OrderingKey: envelope.OrderingKey, Status: "pending", Payload: payload, PayloadSHA256: hex.EncodeToString(sum[:]), Signature: hex.EncodeToString(signature), Attempts: []DeliveryAttempt{}, CreatedAt: now, UpdatedAt: now}
		if err = writeAtomic(path, delivery); err != nil {
			return nil, err
		}
		created = append(created, delivery)
	}
	return created, nil
}

func (s *Store) ListDeliveries(installationID string) ([]DeliveryInspection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readDeliveries(installationID)
}
func (s *Store) GetDelivery(installationID, id string) (DeliveryInspection, error) {
	all, err := s.ListDeliveries(installationID)
	if err != nil {
		return DeliveryInspection{}, err
	}
	for _, d := range all {
		if d.ID == id {
			return d, nil
		}
	}
	return DeliveryInspection{}, ErrDeliveryNotFound
}

func (s *Store) RecordDeliveryAttempt(installationID, id, status string, response int, message string) (Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Delivery{}, err
	}
	defer unlock()
	if status != "delivered" && status != "failed" && status != "pending" {
		return Delivery{}, ErrInvalid
	}
	path := filepath.Join(s.root, "delivery-"+id+".json")
	var d Delivery
	b, e := os.ReadFile(path)
	if e != nil {
		return d, ErrDeliveryNotFound
	}
	if json.Unmarshal(b, &d) != nil || d.InstallationID != installationID {
		return d, ErrDeliveryNotFound
	}
	now := s.now().Truncate(time.Microsecond)
	message = strings.TrimSpace(message)
	if len(message) > 240 {
		message = message[:240]
	}
	d.Attempts = append(d.Attempts, DeliveryAttempt{Number: len(d.Attempts) + 1, Status: status, ResponseCode: response, Error: message, At: now})
	d.Status = status
	failedAttempts := 0
	for _, attempt := range d.Attempts {
		if attempt.Status == "failed" {
			failedAttempts++
		}
	}
	if status == "failed" && failedAttempts >= 5 {
		d.Status = "dead_letter"
	}
	d.UpdatedAt = now
	if err = writeAtomic(path, d); err != nil {
		return Delivery{}, err
	}
	return d, nil
}

func (s *Store) ReplayDelivery(installationID, id string) (Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Delivery{}, err
	}
	defer unlock()
	path := filepath.Join(s.root, "delivery-"+id+".json")
	var original Delivery
	b, e := os.ReadFile(path)
	if e != nil {
		return Delivery{}, ErrDeliveryNotFound
	}
	if json.Unmarshal(b, &original) != nil || original.InstallationID != installationID {
		return Delivery{}, ErrDeliveryNotFound
	}
	var envelope DeliveryEnvelope
	if json.Unmarshal(original.Payload, &envelope) != nil {
		return Delivery{}, ErrInvalid
	}
	newID, err := newID()
	if err != nil {
		return Delivery{}, err
	}
	sequence, err := s.nextSequence(installationID)
	if err != nil {
		return Delivery{}, err
	}
	envelope.DeliveryID = newID
	envelope.Sequence = sequence
	payload, _ := json.Marshal(envelope)
	sum := sha256.Sum256(payload)
	sig, err := s.sign(payload)
	if err != nil {
		return Delivery{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	d := Delivery{ID: newID, InstallationID: installationID, EventID: original.EventID, EventType: original.EventType, SchemaVersion: DeliverySchemaVersion, Sequence: sequence, OrderingKey: original.OrderingKey, Status: "pending", Payload: payload, PayloadSHA256: hex.EncodeToString(sum[:]), Signature: hex.EncodeToString(sig), Attempts: []DeliveryAttempt{}, CreatedAt: now, UpdatedAt: now}
	if err = writeAtomic(filepath.Join(s.root, "delivery-"+newID+".json"), d); err != nil {
		return Delivery{}, err
	}
	return d, nil
}

func (s *Store) readDeliveries(installationID string) ([]DeliveryInspection, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []DeliveryInspection{}
	for _, x := range entries {
		if !strings.HasPrefix(x.Name(), "delivery-") || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		var d Delivery
		b, e := os.ReadFile(filepath.Join(s.root, x.Name()))
		if e != nil {
			return nil, e
		}
		if json.Unmarshal(b, &d) != nil {
			return nil, fmt.Errorf("invalid delivery record")
		}
		if d.InstallationID != installationID {
			continue
		}
		var payload map[string]any
		_ = json.Unmarshal(d.Payload, &payload)
		out = append(out, DeliveryInspection{Delivery: d, Payload: payload})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence > out[j].Sequence })
	return out, nil
}
func (s *Store) readAllInstallations() ([]Installation, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Installation{}
	for _, x := range entries {
		if strings.HasPrefix(x.Name(), "installation-") && strings.HasSuffix(x.Name(), ".json") {
			var v Installation
			b, e := os.ReadFile(filepath.Join(s.root, x.Name()))
			if e != nil {
				return nil, e
			}
			if json.Unmarshal(b, &v) != nil {
				return nil, ErrInvalid
			}
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *Store) readExtensionUnchecked(id string) Extension { v, _ := s.readExtension(id); return v }
func (s *Store) nextSequence(id string) (int64, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return 0, e
	}
	var n int64
	for _, x := range entries {
		if !strings.HasPrefix(x.Name(), "delivery-") || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		var d Delivery
		b, e := os.ReadFile(filepath.Join(s.root, x.Name()))
		if e != nil {
			return 0, e
		}
		if json.Unmarshal(b, &d) != nil {
			return 0, ErrInvalid
		}
		if d.InstallationID == id && d.Sequence > n {
			n = d.Sequence
		}
	}
	return n + 1, nil
}
func (s *Store) signingKey() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	path := filepath.Join(s.root, ".delivery-signing-key")
	b, e := os.ReadFile(path)
	if e == nil && len(b) == ed25519.PrivateKeySize {
		key := ed25519.PrivateKey(b)
		return key, key.Public().(ed25519.PublicKey), nil
	}
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return nil, nil, e
	}
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return nil, nil, e
	}
	if e = os.WriteFile(path, key, 0600); e != nil {
		return nil, nil, e
	}
	return key, pub, nil
}
func (s *Store) sign(payload []byte) ([]byte, error) {
	key, _, e := s.signingKey()
	if e != nil {
		return nil, e
	}
	return ed25519.Sign(key, payload), nil
}
func contains(v []string, x string) bool {
	for _, a := range v {
		if a == x {
			return true
		}
	}
	return false
}
func subscribed(v []string, event string) bool {
	for _, x := range v {
		if x == event || strings.HasSuffix(x, ".*") && strings.HasPrefix(event, strings.TrimSuffix(x, "*")) {
			return true
		}
	}
	return false
}
func resourcePermittedAt(i Installation, repositoryID, resource string, occurredAt time.Time) bool {
	aliases := map[string]string{"pull_request": "pull_requests", "check": "checks", "release": "releases", "deployment": "deployments", "incident": "incidents", "issue": "issues", "task": "tasks", "repository": "repositories"}
	wanted := aliases[resource]
	if wanted == "" {
		wanted = resource
	}
	for _, p := range i.EffectiveAccess {
		if p.Resource == wanted && contains(p.Actions, "read") {
			boundary := i.AuthorityEffectiveAt[repositoryID+":"+wanted]
			// Authorization and activity timestamps are microsecond-granular.
			// Equality is therefore ambiguous and must fail closed: only activity
			// durably observed after the grant boundary may be projected.
			return !boundary.IsZero() && occurredAt.After(boundary)
		}
	}
	return false
}
