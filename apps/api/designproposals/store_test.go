package designproposals

import "testing"

func complete() Revision {
	return Revision{Title: "Safer first run", UserGoal: "A new contributor can evaluate setup before changing files", Source: Source{Kind: "issue", ResourceID: "issue-1", Summary: "Setup confusion"}, Journeys: []Journey{{Name: "First run", Actor: "Contributor", Goal: "Understand effects", Steps: []string{"Preview", "Confirm"}}}, States: []State{{Name: "Preview", Description: "No mutation yet", Content: "Review setup"}}, Content: []string{"Use direct language"}, Constraints: []string{"Keyboard operable"}, Alternatives: []string{"Immediate setup"}, SuccessMeasures: []string{"Fewer abandoned runs"}, AffectedComponents: []string{"setup dialog"}, Artifacts: []Artifact{{ID: "wire-1", Kind: "wireframe", Title: "Preview", Description: "A review screen", Content: "[effects] [confirm]", Audience: []string{"owner"}}}, Uncertainty: []string{"Invited user sample is small"}}
}
func TestRevisionDiscussionAndAcknowledgementAreBound(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	v, e := s.Create("repo", "author", []string{"owner"}, complete())
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.Comment("repo", v.ID, "guest", Comment{Revision: 1, Kind: "dissent", Body: "This assumes expert vocabulary."})
	if e != nil || len(v.Comments) != 1 {
		t.Fatalf("comment: %#v %v", v, e)
	}
	if _, e = s.Acknowledge("repo", v.ID, "guest", Acknowledgement{Revision: 1, Status: "acknowledged"}); e != ErrInvalid {
		t.Fatalf("guest acknowledgement = %v", e)
	}
	v, e = s.Acknowledge("repo", v.ID, "owner", Acknowledgement{Revision: 1, Status: "changes_requested", Note: "Test novice language"})
	if e != nil || len(v.Acknowledgements) != 1 {
		t.Fatalf("ack: %#v %v", v, e)
	}
	if _, e = s.Revise("repo", v.ID, "author", 0, complete()); e != ErrConflict {
		t.Fatalf("stale revision = %v", e)
	}
	v, e = s.Revise("repo", v.ID, "author", 1, complete())
	if e != nil || v.CurrentVersion != 2 {
		t.Fatalf("revise: %#v %v", v, e)
	}
	if _, e = s.Acknowledge("repo", v.ID, "owner", Acknowledgement{Revision: 1, Status: "acknowledged"}); e != ErrInvalid {
		t.Fatalf("stale acknowledgement = %v", e)
	}
}
