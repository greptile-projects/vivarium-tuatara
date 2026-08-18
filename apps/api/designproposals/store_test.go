package designproposals

import (
	"strings"
	"testing"
)

func complete() Revision {
	return Revision{Title: "Safer first run", UserGoal: "A new contributor can evaluate setup before changing files", Source: Source{Kind: "issue", ResourceID: "issue-1", Summary: "Setup confusion"}, Journeys: []Journey{{Name: "First run", Actor: "Contributor", Goal: "Understand effects", Steps: []string{"Preview", "Confirm"}}}, States: []State{{Name: "Preview", Description: "No mutation yet", Content: "Review setup"}}, Content: []string{"Use direct language"}, Constraints: []string{"Keyboard operable"}, Alternatives: []string{"Immediate setup"}, SuccessMeasures: []string{"Fewer abandoned runs"}, AffectedComponents: []string{"setup dialog"}, Artifacts: []Artifact{{ID: "wire-1", Kind: "wireframe", Title: "Preview", Description: "A review screen", Content: "[effects] [confirm]", Audience: []string{"owner"}}}, Uncertainty: []string{"Invited user sample is small"}}
}

func TestAcceptedDesignImplementationRetainsAccountability(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create("repo", "designer", []string{"designer"}, complete())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PublishImplementation("repo", v.ID, "designer", 1, Implementation{DesignVersion: 1, BaseRevision: strings.Repeat("a", 40), ProposalID: strings.Repeat("b", 32), TaskIDs: []string{"task"}}); err != ErrInvalid {
		t.Fatalf("unaccepted design published: %v", err)
	}
	v, err = s.Acknowledge("repo", v.ID, "designer", Acknowledgement{Revision: 1, Status: "acknowledged"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.PublishImplementation("repo", v.ID, "designer", 1, Implementation{DesignVersion: 1, BaseRevision: strings.Repeat("a", 40), ProposalID: strings.Repeat("b", 32), TaskIDs: []string{"task"}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Report("repo", v.ID, "developer", &RequirementMapping{Requirement: "empty state", CodePaths: []string{"ui/empty.tsx"}, Surfaces: []string{"/inbox empty"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Report("repo", v.ID, "developer", nil, &Deviation{Requirement: "mobile breakpoint", Reason: "native control limitation", Impact: "layout differs below 320px"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.DecideDeviation("repo", v.ID, v.Implementation.Deviations[0].ID, "designer", "approved", "Accepted")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Implementation.Mappings) != 1 || v.Implementation.Deviations[0].Status != "approved" {
		t.Fatalf("accountability lost: %#v", v.Implementation)
	}
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

func TestWithCurrentVersionSerializesRevision(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create("repo", "author", []string{"owner"}, complete())
	if err != nil {
		t.Fatal(err)
	}
	entered, release, guardedDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		guardedDone <- s.WithCurrentVersion("repo", v.ID, 1, func(current Proposal) error {
			if current.CurrentVersion != 1 {
				t.Errorf("guarded version = %d", current.CurrentVersion)
			}
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	revised := make(chan error, 1)
	go func() { _, reviseErr := s.Revise("repo", v.ID, "author", 1, complete()); revised <- reviseErr }()
	select {
	case err := <-revised:
		t.Fatalf("revision did not wait for guard: %v", err)
	default:
	}
	close(release)
	if err := <-guardedDone; err != nil {
		t.Fatal(err)
	}
	if err := <-revised; err != nil {
		t.Fatal(err)
	}
	if err := s.WithCurrentVersion("repo", v.ID, 1, func(Proposal) error { return nil }); err != ErrConflict {
		t.Fatalf("stale guard = %v", err)
	}
}
