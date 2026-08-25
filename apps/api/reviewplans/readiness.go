package reviewplans

import (
	"slices"
	"strings"
	"time"
)

type StaleApproval struct {
	ReviewerID string    `json:"reviewer_id"`
	Revision   string    `json:"revision"`
	Decision   string    `json:"decision"`
	CreatedAt  time.Time `json:"created_at"`
}

type AreaReadiness struct {
	AreaID                   string         `json:"area_id"`
	Title                    string         `json:"title"`
	Required                 bool           `json:"required"`
	Owners                   []string       `json:"owners"`
	Assignments              []Assignment   `json:"assignments"`
	EvidenceInspected        []WorkCitation `json:"evidence_inspected"`
	Findings                 []WorkEntry    `json:"findings"`
	Decisions                []WorkEntry    `json:"decisions"`
	RequiredAcknowledgements []string       `json:"required_acknowledgements"`
	MissingAcknowledgements  []string       `json:"missing_acknowledgements"`
	UnresolvedGaps           []string       `json:"unresolved_gaps"`
	Complete                 bool           `json:"complete"`
}

type ReviewReadiness struct {
	PlanID         string          `json:"plan_id,omitempty"`
	PlanVersion    int             `json:"plan_version,omitempty"`
	SourceRevision string          `json:"source_revision"`
	TargetRevision string          `json:"target_revision"`
	Current        bool            `json:"current"`
	Complete       bool            `json:"complete"`
	Areas          []AreaReadiness `json:"areas"`
	UnresolvedGaps []string        `json:"unresolved_gaps"`
	StaleApprovals []StaleApproval `json:"stale_approvals"`
	Authority      string          `json:"authority"`
}

// ProjectReadiness joins the immutable review ledgers without rewriting them.
// A human decision on the exact plan area completes the review only when an
// accepted accountable assignee, every declared evidence kind, owner
// acknowledgement, and every finding disposition are also current.
func ProjectReadiness(plan *Plan, assignments []Assignment, work []WorkEntry, resolutions []FindingResolution, source, target string, stale []StaleApproval) ReviewReadiness {
	out := ReviewReadiness{SourceRevision: source, TargetRevision: target, Areas: []AreaReadiness{}, UnresolvedGaps: []string{}, StaleApprovals: stale, Authority: "Review readiness reports revision-bound coverage only; it grants no review, approval, merge, policy, exception, repository, or operational authority."}
	if plan == nil {
		out.UnresolvedGaps = append(out.UnresolvedGaps, "No review plan covers the current candidate.")
		return out
	}
	out.PlanID, out.PlanVersion = plan.ID, plan.Version
	out.Current = plan.SourceRevision == source && plan.TargetRevision == target
	for _, area := range plan.Areas {
		row := AreaReadiness{AreaID: area.ID, Title: area.Title, Required: true, Owners: append([]string(nil), area.OwnerIDs...), Assignments: []Assignment{}, EvidenceInspected: []WorkCitation{}, Findings: []WorkEntry{}, Decisions: []WorkEntry{}, RequiredAcknowledgements: append([]string(nil), area.OwnerIDs...), MissingAcknowledgements: []string{}, UnresolvedGaps: []string{}}
		accepted := false
		for _, assignment := range assignments {
			if assignment.PlanID == plan.ID && assignment.AreaID == area.ID && (assignment.Status == "invited" || assignment.Status == "accepted") {
				row.Assignments = append(row.Assignments, assignment)
				accepted = accepted || assignment.Status == "accepted"
			}
		}
		if !accepted {
			row.UnresolvedGaps = append(row.UnresolvedGaps, "An accountable reviewer has not accepted this area.")
		}
		inspected := []WorkCitation{}
		acknowledged := map[string]bool{}
		for _, entry := range work {
			if entry.PlanID != plan.ID || entry.AreaID != area.ID || entry.SourceRevision != source || entry.TargetRevision != target {
				continue
			}
			for _, citation := range entry.Citations {
				row.EvidenceInspected = append(row.EvidenceInspected, citation)
				inspected = append(inspected, citation)
			}
			if entry.Kind == "finding" {
				row.Findings = append(row.Findings, entry)
			}
			if entry.Kind == "decision" && entry.ActorType == "human" {
				row.Decisions = append(row.Decisions, entry)
				acknowledged[entry.ActorID] = true
			}
		}
		for _, required := range area.Evidence {
			if required.Required && !inspectedEvidenceKind(required.Kind, area, inspected) {
				row.UnresolvedGaps = append(row.UnresolvedGaps, "Required "+required.Kind+" evidence has not been inspected.")
			}
		}
		if len(row.Decisions) == 0 {
			row.UnresolvedGaps = append(row.UnresolvedGaps, "A current human review decision is required.")
		}
		for _, owner := range row.RequiredAcknowledgements {
			if !acknowledged[owner] {
				row.MissingAcknowledgements = append(row.MissingAcknowledgements, owner)
			}
		}
		if len(row.MissingAcknowledgements) > 0 {
			row.UnresolvedGaps = append(row.UnresolvedGaps, "Required owner acknowledgement is missing.")
		}
		for _, finding := range row.Findings {
			var latest *FindingResolution
			for _, resolution := range resolutions {
				if resolution.FindingID == finding.ID && resolution.CandidateRevision == source && slices.Contains([]string{"resolved", "supersede", "accepted_risk", "exception"}, resolution.Action) && (latest == nil || resolution.CreatedAt.After(latest.CreatedAt)) {
					copy := resolution
					latest = &copy
				}
			}
			resolved := latest != nil && slices.Contains([]string{"resolved", "supersede", "accepted_risk", "exception"}, latest.Action) && (latest.ExpiresAt == nil || latest.ExpiresAt.After(time.Now()))
			if resolved && latest.Action == "exception" {
				resolved = slices.ContainsFunc(latest.Links, func(link ResolutionLink) bool { return link.Kind == "follow_up" })
			}
			if !resolved {
				row.UnresolvedGaps = append(row.UnresolvedGaps, "Finding "+finding.ID+" has no current accountable disposition.")
			}
		}
		row.Complete = out.Current && len(row.UnresolvedGaps) == 0
		if !row.Complete {
			out.UnresolvedGaps = append(out.UnresolvedGaps, area.Title+": "+strings.Join(row.UnresolvedGaps, " "))
		}
		out.Areas = append(out.Areas, row)
	}
	out.Complete = out.Current && len(out.UnresolvedGaps) == 0
	return out
}

func inspectedEvidenceKind(required string, area Area, inspected []WorkCitation) bool {
	for _, citation := range inspected {
		if citation.Kind == required || required == "checks" && citation.Kind == "check" {
			return true
		}
		if strings.HasSuffix(required, "_evidence") && citation.Domain == strings.TrimSuffix(required, "_evidence") && citation.Domain == area.ID {
			covered := true
			for _, path := range area.Paths {
				covered = covered && slices.Contains(citation.CoveredPaths, path)
			}
			if covered && len(area.Paths) > 0 {
				return true
			}
		}
	}
	return false
}
