package responsepolicies

import (
	"testing"
	"time"
)

func rotationFixture(now time.Time) RotationRevision {
	availability := []Availability{{Weekdays: []string{"monday", "tuesday"}, StartLocal: "09:00", EndLocal: "17:00"}}
	return RotationRevision{Name: "Platform primary", PolicyID: "policy", TeamID: "platform", TimeZone: "Europe/London", HandoffMinutes: 30, Responders: []Responder{{UserID: "alice", Qualifications: []string{"incident lead"}, Availability: availability, MaxShifts: 1}, {UserID: "bob", Qualifications: []string{"incident lead"}, Availability: availability, MaxShifts: 2}}, AbsenceRules: []AbsenceRule{{Kind: "planned", NoticeHours: 24, Action: "offer to first eligible backup"}}, Shifts: []Shift{{ID: "day-1", StartsAt: now.Add(-2 * time.Hour), EndsAt: now.Add(-time.Hour), PrimaryUserID: "alice", BackupUserIDs: []string{"bob"}, RequiredQualifications: []string{"incident lead"}}, {ID: "day-2", StartsAt: now.Add(30 * time.Minute), EndsAt: now.Add(8 * time.Hour), PrimaryUserID: "alice", BackupUserIDs: []string{"bob"}, RequiredQualifications: []string{"incident lead"}}}, ChangeReason: "publish humane coverage"}
}

func TestRotationTransferRequiresExactRecipientAcceptance(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return now }
	r, err := s.CreateRotation("repo", "alice", "create", rotationFixture(now))
	if err != nil {
		t.Fatal(err)
	}
	initial := ProjectRotation(r, map[string]bool{"alice": true, "bob": true}, now)
	overloaded := false
	for _, d := range initial.Diagnostics {
		if d.Kind == "workload_exceeded" {
			overloaded = true
		}
	}
	if !overloaded {
		t.Fatalf("initial diagnostics=%+v", initial.Diagnostics)
	}
	context := []DutyContext{{Kind: "active_alert", ResourceID: "alert-1", Revision: "rev-7", Summary: "Checkout latency is elevated"}}
	r, err = s.AppendDutyEvent(r.ID, "alice", "delegate-1", "delegate", "day-2", "bob", "planned appointment", context, r.EventVersion)
	if err != nil {
		t.Fatal(err)
	}
	if r.Events[0].Status != "pending" {
		t.Fatalf("event=%+v", r.Events[0])
	}
	if _, err = s.AcceptDutyEvent(r.ID, r.Events[0].ID, "alice", r.EventVersion); err != ErrInvalid {
		t.Fatalf("sender accepted: %v", err)
	}
	r, err = s.AcceptDutyEvent(r.ID, r.Events[0].ID, "bob", r.EventVersion)
	if err != nil {
		t.Fatal(err)
	}
	if r.Events[0].Status != "accepted" || len(r.Events[0].Context) != 1 {
		t.Fatalf("event=%+v", r.Events[0])
	}
	projected := ProjectRotation(r, map[string]bool{"alice": true, "bob": true}, now)
	kinds := map[string]bool{}
	for _, d := range projected.Diagnostics {
		kinds[d.Kind] = true
	}
	if !kinds["missed_handoff"] || !kinds["uncovered_interval"] {
		t.Fatalf("diagnostics=%+v", projected.Diagnostics)
	}
}

func TestRotationRetryAndRevokedParticipantProjection(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return now }
	revision := rotationFixture(now)
	a, _ := s.CreateRotation("repo", "alice", "same", revision)
	b, err := s.CreateRotation("repo", "alice", "same", revision)
	if err != nil || a.ID != b.ID {
		t.Fatalf("retry=%+v %v", b, err)
	}
	revision.Name = "changed"
	if _, err = s.CreateRotation("repo", "alice", "same", revision); err != ErrConflict {
		t.Fatalf("changed retry=%v", err)
	}
	projected := ProjectRotation(a, map[string]bool{"bob": true}, now)
	found := false
	for _, d := range projected.Diagnostics {
		if d.Kind == "unavailable_responder" && d.UserID == "alice" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics=%+v", projected.Diagnostics)
	}
}
