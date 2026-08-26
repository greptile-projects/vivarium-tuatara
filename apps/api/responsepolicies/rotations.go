package responsepolicies

import (
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

type Availability struct {
	Weekdays   []string `json:"weekdays"`
	StartLocal string   `json:"start_local"`
	EndLocal   string   `json:"end_local"`
}
type AbsenceRule struct {
	Kind        string `json:"kind"`
	NoticeHours int    `json:"notice_hours"`
	Action      string `json:"action"`
}
type Responder struct {
	UserID         string         `json:"user_id"`
	Qualifications []string       `json:"qualifications"`
	Availability   []Availability `json:"availability"`
	MaxShifts      int            `json:"max_shifts_per_week"`
}
type Shift struct {
	ID                     string    `json:"id"`
	StartsAt               time.Time `json:"starts_at"`
	EndsAt                 time.Time `json:"ends_at"`
	PrimaryUserID          string    `json:"primary_user_id"`
	BackupUserIDs          []string  `json:"backup_user_ids"`
	RequiredQualifications []string  `json:"required_qualifications"`
}
type RotationRevision struct {
	RequestID      string        `json:"request_id,omitempty"`
	Version        int           `json:"version,omitempty"`
	Name           string        `json:"name"`
	PolicyID       string        `json:"policy_id"`
	TeamID         string        `json:"team_id"`
	TimeZone       string        `json:"time_zone"`
	HandoffMinutes int           `json:"handoff_window_minutes"`
	Responders     []Responder   `json:"responders"`
	AbsenceRules   []AbsenceRule `json:"absence_rules"`
	Shifts         []Shift       `json:"shifts"`
	ChangeReason   string        `json:"change_reason"`
	CreatedBy      string        `json:"created_by,omitempty"`
	CreatedAt      time.Time     `json:"created_at,omitempty"`
}
type DutyContext struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Summary    string `json:"summary"`
}
type DutyEvent struct {
	ID              string        `json:"id"`
	RequestID       string        `json:"request_id"`
	Kind            string        `json:"kind"`
	ShiftID         string        `json:"shift_id"`
	FromUserID      string        `json:"from_user_id,omitempty"`
	ToUserID        string        `json:"to_user_id,omitempty"`
	Context         []DutyContext `json:"context"`
	Reason          string        `json:"reason,omitempty"`
	Status          string        `json:"status"`
	CreatedBy       string        `json:"created_by"`
	CreatedAt       time.Time     `json:"created_at"`
	AcceptedBy      string        `json:"accepted_by,omitempty"`
	AcceptedAt      *time.Time    `json:"accepted_at,omitempty"`
	RotationVersion int           `json:"rotation_version"`
}
type DutyDiagnostic struct {
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	ShiftID    string `json:"shift_id,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	Escalation string `json:"escalation"`
}
type Rotation struct {
	ID                    string             `json:"id"`
	RepositoryID          string             `json:"repository_id"`
	RequestID             string             `json:"request_id"`
	RequestDigest         string             `json:"request_digest"`
	CurrentVersion        int                `json:"current_version"`
	EventVersion          int                `json:"event_version"`
	Revisions             []RotationRevision `json:"revisions"`
	Events                []DutyEvent        `json:"events"`
	Diagnostics           []DutyDiagnostic   `json:"diagnostics"`
	EffectiveOwnerByShift map[string]string  `json:"effective_owner_by_shift,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

func (s *Store) CreateRotation(repositoryID, actor, requestID string, revision RotationRevision) (Rotation, error) {
	var out Rotation
	err := s.lock(func() error {
		if blank(requestID) || validateRotation(revision) != nil {
			return ErrInvalid
		}
		digest := rotationDigest(revision)
		id := stableID(repositoryID, actor, "rotation\x00"+requestID)
		if old, e := s.readRotation(id); e == nil {
			if old.RequestDigest != digest {
				return ErrConflict
			}
			out = old
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		now := s.now()
		stampRotation(&revision, actor, requestID, 1, now)
		out = Rotation{ID: id, RepositoryID: repositoryID, RequestID: requestID, RequestDigest: digest, CurrentVersion: 1, Revisions: []RotationRevision{revision}, Events: []DutyEvent{}, CreatedAt: now, UpdatedAt: now}
		return s.writeRotation(out)
	})
	return out, err
}
func (s *Store) ReviseRotation(id string, expected int, actor, requestID string, revision RotationRevision) (Rotation, error) {
	var out Rotation
	err := s.lock(func() error {
		if blank(requestID) {
			return ErrInvalid
		}
		v, e := s.readRotation(id)
		if e != nil {
			return e
		}
		digest := rotationDigest(revision)
		for _, old := range v.Revisions {
			if old.RequestID == requestID {
				if rotationDigest(old) != digest {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validateRotation(revision) != nil {
			return ErrInvalid
		}
		now := s.now()
		stampRotation(&revision, actor, requestID, expected+1, now)
		v.CurrentVersion++
		v.Revisions = append(v.Revisions, revision)
		v.UpdatedAt = now
		out = v
		return s.writeRotation(v)
	})
	return out, err
}
func (s *Store) GetRotation(id string) (Rotation, error) {
	var v Rotation
	e := s.lock(func() error { var x error; v, x = s.readRotation(id); return x })
	return v, e
}
func (s *Store) ListRotations(repositoryID string) ([]Rotation, error) {
	values := []Rotation{}
	e := s.lock(func() error {
		entries, x := os.ReadDir(s.rotationRoot())
		if os.IsNotExist(x) {
			return nil
		}
		if x != nil {
			return x
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			v, x := s.readRotation(strings.TrimSuffix(entry.Name(), ".json"))
			if x != nil {
				return x
			}
			if v.RepositoryID == repositoryID {
				values = append(values, v)
			}
		}
		return nil
	})
	sort.Slice(values, func(i, j int) bool { return values[i].UpdatedAt.After(values[j].UpdatedAt) })
	return values, e
}
func (s *Store) AppendDutyEvent(id, actor, requestID, kind, shiftID, to, reason string, context []DutyContext, expected int) (Rotation, error) {
	var out Rotation
	err := s.lock(func() error {
		if blank(requestID) {
			return ErrInvalid
		}
		v, e := s.readRotation(id)
		if e != nil {
			return e
		}
		for _, old := range v.Events {
			if old.RequestID == requestID {
				probe := dutyDigest(kind, shiftID, actor, to, reason, context)
				if dutyEventDigest(old) != probe {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if v.EventVersion != expected {
			return ErrConflict
		}
		r := v.Revisions[len(v.Revisions)-1]
		shift, ok := findShift(r, shiftID)
		if !ok {
			return ErrInvalid
		}
		effective := effectiveOwner(v, shift)
		if kind == "acknowledge" {
			if actor != effective {
				return ErrInvalid
			}
			to = actor
		} else {
			if kind != "swap" && kind != "delegate" && kind != "override" {
				return ErrInvalid
			}
			if actor != effective && !rotationMember(r, actor) {
				return ErrInvalid
			}
			if !rotationMember(r, to) || to == effective || blank(reason) || len(context) == 0 {
				return ErrInvalid
			}
			for _, item := range context {
				if blank(item.Kind) || blank(item.ResourceID) || blank(item.Revision) || blank(item.Summary) {
					return ErrInvalid
				}
			}
		}
		now := s.now()
		status := "accepted"
		if kind != "acknowledge" {
			status = "pending"
		}
		event := DutyEvent{ID: stableEventID(id, requestID), RequestID: requestID, Kind: kind, ShiftID: shiftID, FromUserID: effective, ToUserID: to, Context: context, Reason: reason, Status: status, CreatedBy: actor, CreatedAt: now, RotationVersion: v.CurrentVersion}
		if kind == "acknowledge" {
			event.AcceptedBy = actor
			event.AcceptedAt = &now
		}
		v.Events = append(v.Events, event)
		v.EventVersion++
		v.UpdatedAt = now
		out = v
		return s.writeRotation(v)
	})
	return out, err
}
func (s *Store) AcceptDutyEvent(id, eventID, actor string, expected int) (Rotation, error) {
	var out Rotation
	err := s.lock(func() error {
		v, e := s.readRotation(id)
		if e != nil {
			return e
		}
		if v.EventVersion != expected {
			return ErrConflict
		}
		idx := -1
		for i := range v.Events {
			if v.Events[i].ID == eventID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return ErrNotFound
		}
		event := &v.Events[idx]
		if event.Status != "pending" || event.ToUserID != actor {
			return ErrInvalid
		}
		r := v.Revisions[len(v.Revisions)-1]
		shift, found := findShift(r, event.ShiftID)
		if !rotationMember(r, actor) || !found || event.RotationVersion != v.CurrentVersion || event.FromUserID != effectiveOwner(v, shift) {
			return ErrInvalid
		}
		now := s.now()
		event.Status = "accepted"
		event.AcceptedBy = actor
		event.AcceptedAt = &now
		v.EventVersion++
		v.UpdatedAt = now
		out = v
		return s.writeRotation(v)
	})
	return out, err
}

func ProjectRotation(v Rotation, current map[string]bool, now time.Time) Rotation {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []DutyDiagnostic{}
	counts := map[string]map[string]int{}
	v.EffectiveOwnerByShift = map[string]string{}
	location, _ := time.LoadLocation(r.TimeZone)
	shifts := append([]Shift(nil), r.Shifts...)
	sort.Slice(shifts, func(i, j int) bool { return shifts[i].StartsAt.Before(shifts[j].StartsAt) })
	for i, shift := range shifts {
		owner := effectiveOwner(v, shift)
		v.EffectiveOwnerByShift[shift.ID] = owner
		year, week := shift.StartsAt.In(location).ISOWeek()
		weekKey := fmt.Sprintf("%04d-W%02d", year, week)
		if counts[owner] == nil {
			counts[owner] = map[string]int{}
		}
		counts[owner][weekKey]++
		if !current[owner] {
			d = append(d, dutyDiag("unavailable_responder", "blocking", "The assigned responder is no longer a current repository participant.", shift.ID, owner))
		}
		resp, _ := findResponder(r, owner)
		if !responderAvailable(resp, shift, r.TimeZone) {
			d = append(d, dutyDiag("outside_availability", "blocking", "The shift falls outside the responder's declared local availability.", shift.ID, owner))
		}
		for _, q := range shift.RequiredQualifications {
			if !contains(resp.Qualifications, q) {
				d = append(d, dutyDiag("missing_qualification", "blocking", "The assigned responder lacks a required role qualification.", shift.ID, owner))
			}
		}
		if i > 0 && shift.StartsAt.Before(shifts[i-1].EndsAt) {
			d = append(d, dutyDiag("overlapping_schedule", "blocking", "Scheduled duty overlaps another shift in this rotation.", shift.ID, owner))
		}
		if i > 0 && shift.StartsAt.After(shifts[i-1].EndsAt) {
			d = append(d, dutyDiag("uncovered_interval", "blocking", "No responder owns the interval before this shift.", shift.ID, ""))
		}
		if shift.EndsAt.Before(now) && !handoffRecorded(v, shift.ID) {
			d = append(d, dutyDiag("missed_handoff", "blocking", "The shift ended without an acknowledged handoff.", shift.ID, owner))
		}
	}
	for user, weeks := range counts {
		resp, _ := findResponder(r, user)
		for week, n := range weeks {
			if resp.MaxShifts > 0 && n > resp.MaxShifts {
				d = append(d, dutyDiag("workload_exceeded", "warning", "The published schedule exceeds the responder workload limit for "+week+".", "", user))
			}
		}
	}
	v.Diagnostics = d
	return v
}
func dutyDiag(kind, severity, message, shift, user string) DutyDiagnostic {
	return DutyDiagnostic{Kind: kind, Severity: severity, Message: message, ShiftID: shift, UserID: user, Escalation: "Notify the accountable team and assign an eligible current participant."}
}
func validateRotation(r RotationRevision) error {
	if blank(r.Name) || blank(r.PolicyID) || blank(r.TeamID) || blank(r.TimeZone) || r.HandoffMinutes <= 0 || len(r.Responders) == 0 || len(r.AbsenceRules) == 0 || len(r.Shifts) == 0 || blank(r.ChangeReason) {
		return ErrInvalid
	}
	if _, e := time.LoadLocation(r.TimeZone); e != nil {
		return ErrInvalid
	}
	users := map[string]bool{}
	for _, x := range r.Responders {
		if blank(x.UserID) || users[x.UserID] || len(x.Qualifications) == 0 || len(x.Availability) == 0 || x.MaxShifts <= 0 {
			return ErrInvalid
		}
		users[x.UserID] = true
		for _, a := range x.Availability {
			if len(a.Weekdays) == 0 || blank(a.StartLocal) || blank(a.EndLocal) {
				return ErrInvalid
			}
		}
	}
	ids := map[string]bool{}
	for _, x := range r.Shifts {
		if blank(x.ID) || ids[x.ID] || !x.EndsAt.After(x.StartsAt) || !users[x.PrimaryUserID] || len(x.BackupUserIDs) == 0 || len(x.RequiredQualifications) == 0 {
			return ErrInvalid
		}
		ids[x.ID] = true
		for _, u := range x.BackupUserIDs {
			if !users[u] || u == x.PrimaryUserID {
				return ErrInvalid
			}
		}
	}
	for _, x := range r.AbsenceRules {
		if blank(x.Kind) || x.NoticeHours < 0 || blank(x.Action) {
			return ErrInvalid
		}
	}
	return nil
}
func stampRotation(r *RotationRevision, actor, request string, version int, now time.Time) {
	r.RequestID = request
	r.Version = version
	r.CreatedBy = actor
	r.CreatedAt = now
}
func rotationDigest(r RotationRevision) string {
	r.RequestID = ""
	r.Version = 0
	r.CreatedBy = ""
	r.CreatedAt = time.Time{}
	b, _ := json.Marshal(r)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func stableEventID(id, request string) string {
	sum := sha256.Sum256([]byte(id + "\x00" + request))
	return hex.EncodeToString(sum[:16])
}
func dutyDigest(kind, shift, from, to, reason string, c []DutyContext) string {
	b, _ := json.Marshal([]any{kind, shift, from, to, reason, c})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func dutyEventDigest(e DutyEvent) string {
	return dutyDigest(e.Kind, e.ShiftID, e.CreatedBy, e.ToUserID, e.Reason, e.Context)
}
func (s *Store) rotationRoot() string { return filepath.Join(s.root, "rotations") }
func (s *Store) readRotation(id string) (Rotation, error) {
	var v Rotation
	b, e := os.ReadFile(filepath.Join(s.rotationRoot(), id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) writeRotation(v Rotation) error {
	if e := os.MkdirAll(s.rotationRoot(), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.rotationRoot(), ".rotation-")
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
	if e == nil {
		e = os.Rename(name, filepath.Join(s.rotationRoot(), v.ID+".json"))
	}
	return e
}
func findShift(r RotationRevision, id string) (Shift, bool) {
	for _, x := range r.Shifts {
		if x.ID == id {
			return x, true
		}
	}
	return Shift{}, false
}
func findResponder(r RotationRevision, id string) (Responder, bool) {
	for _, x := range r.Responders {
		if x.UserID == id {
			return x, true
		}
	}
	return Responder{}, false
}
func rotationMember(r RotationRevision, id string) bool { _, ok := findResponder(r, id); return ok }
func effectiveOwner(v Rotation, shift Shift) string {
	owner := shift.PrimaryUserID
	for _, e := range v.Events {
		if e.RotationVersion == v.CurrentVersion && e.ShiftID == shift.ID && e.Status == "accepted" && (e.Kind == "swap" || e.Kind == "delegate" || e.Kind == "override") {
			owner = e.ToUserID
		}
	}
	return owner
}
func handoffRecorded(v Rotation, shift string) bool {
	for _, e := range v.Events {
		if e.RotationVersion == v.CurrentVersion && e.ShiftID == shift && e.Status == "accepted" && (e.Kind == "swap" || e.Kind == "delegate" || e.Kind == "override") {
			return true
		}
	}
	return false
}

func responderAvailable(responder Responder, shift Shift, zone string) bool {
	location, err := time.LoadLocation(zone)
	if err != nil {
		return false
	}
	start, end := shift.StartsAt.In(location), shift.EndsAt.In(location)
	if start.YearDay() != end.YearDay() {
		return false
	}
	weekday := strings.ToLower(start.Weekday().String())
	clock := func(value string) (int, bool) {
		parsed, err := time.Parse("15:04", value)
		if err != nil {
			return 0, false
		}
		return parsed.Hour()*60 + parsed.Minute(), true
	}
	startMinute, endMinute := start.Hour()*60+start.Minute(), end.Hour()*60+end.Minute()
	for _, availability := range responder.Availability {
		if !contains(availability.Weekdays, weekday) {
			continue
		}
		availableStart, okStart := clock(availability.StartLocal)
		availableEnd, okEnd := clock(availability.EndLocal)
		if okStart && okEnd && startMinute >= availableStart && endMinute <= availableEnd {
			return true
		}
	}
	return false
}
