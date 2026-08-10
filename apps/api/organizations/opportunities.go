package organizations

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"
)

type OpportunityFinding struct {
	RepositoryID      string                `json:"repository_id"`
	Signal            string                `json:"signal"`
	EvidenceType      string                `json:"evidence_type"`
	EvidenceID        string                `json:"evidence_id"`
	EvidenceRevision  string                `json:"evidence_revision"`
	DedupeKey         string                `json:"dedupe_key"`
	Title             string                `json:"title"`
	Summary           string                `json:"summary"`
	Severity          string                `json:"severity"`
	ExpectedValue     string                `json:"expected_value"`
	Confidence        float64               `json:"confidence"`
	AffectedOwnerIDs  []string              `json:"affected_owner_ids"`
	AffectedRevisions []string              `json:"affected_revisions"`
	Citations         []OpportunityCitation `json:"citations"`
	InScopeReason     string                `json:"in_scope_reason"`
}

type OpportunityDecision struct {
	ExpectedVersion int        `json:"expected_version"`
	Action          string     `json:"action"`
	Rank            int        `json:"rank"`
	Until           *time.Time `json:"until"`
	Reason          string     `json:"reason"`
	Comment         string     `json:"comment"`
}

func opportunityKey(f OpportunityFinding) string {
	key := strings.TrimSpace(f.DedupeKey)
	if key == "" {
		key = f.RepositoryID + "\x00" + f.EvidenceType + "\x00" + f.EvidenceID + "\x00" + strings.ToLower(strings.TrimSpace(f.Title))
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func validateFinding(revision MandateRevision, f OpportunityFinding) bool {
	types := []string{"repository", "dependency", "check", "release", "incident", "security", "usage"}
	severities := []string{"critical", "high", "medium", "low"}
	if !slices.Contains(types, f.EvidenceType) || !slices.Contains(severities, f.Severity) || f.Confidence < 0 || f.Confidence > 1 || len(f.Citations) == 0 || len(f.Citations) > 50 || len(f.AffectedOwnerIDs) == 0 || len(f.AffectedOwnerIDs) > 100 || len(f.AffectedRevisions) == 0 || len(f.AffectedRevisions) > 100 {
		return false
	}
	inScope, trusted := false, false
	for _, scope := range revision.Repositories {
		inScope = inScope || scope.RepositoryID == f.RepositoryID
	}
	for _, signal := range revision.TrustedSignals {
		trusted = trusted || strings.EqualFold(strings.TrimSpace(signal), strings.TrimSpace(f.Signal))
	}
	if !inScope || !trusted || !validID(f.RepositoryID) || len(f.DedupeKey) > 500 {
		return false
	}
	for value, max := range map[string]int{f.EvidenceID: 300, f.EvidenceRevision: 300, f.Title: 200, f.Summary: 4000, f.ExpectedValue: 1000, f.InScopeReason: 1000} {
		if _, ok := clean(value, max); !ok {
			return false
		}
	}
	for _, id := range f.AffectedOwnerIDs {
		if !validID(id) {
			return false
		}
	}
	for _, value := range f.AffectedRevisions {
		if _, ok := clean(value, 300); !ok {
			return false
		}
	}
	for _, citation := range f.Citations {
		if _, ok := clean(citation.Kind, 50); !ok {
			return false
		}
		if _, ok := clean(citation.ResourceID, 300); !ok {
			return false
		}
		if _, ok := clean(citation.Revision, 300); !ok {
			return false
		}
		if _, ok := clean(citation.Label, 500); !ok || len(citation.URL) > 1000 || (citation.URL != "" && (!strings.HasPrefix(citation.URL, "/") || strings.HasPrefix(citation.URL, "//") || strings.Contains(citation.URL, "\\"))) {
			return false
		}
	}
	return true
}

// PublishStewardshipOpportunities is the evaluation boundary used both when a
// mandate is activated and when a trusted producer observes relevant changes.
// Repeated findings converge on one queue item and retain superseded citations
// as explicitly stale evidence.
func (s *Store) PublishStewardshipOpportunities(id, mandateID, actor string, findings []OpportunityFinding) (Organization, []StewardshipOpportunity, error) {
	if len(findings) == 0 || len(findings) > 100 {
		return Organization{}, nil, ErrInvalid
	}
	out := []StewardshipOpportunity{}
	v, err := s.mutate(id, func(v *Organization) error {
		i := mandateIndex(v, mandateID)
		if i < 0 {
			return ErrNotFound
		}
		m := &v.StewardshipMandates[i]
		now := s.now().Truncate(time.Microsecond)
		if m.Status != "active" || m.Acceptance == nil || m.Acceptance.Version != m.Version || m.Acceptance.OperatorID != actor {
			return ErrNotFound
		}
		revision := m.Revisions[len(m.Revisions)-1]
		if now.Before(revision.StartsAt) || !revision.ExpiresAt.After(now) {
			return ErrConflict
		}
		existing := map[string]bool{}
		for _, item := range m.Opportunities {
			existing[item.DedupeKey] = true
		}
		newKeys := map[string]bool{}
		for _, finding := range findings {
			if !validateFinding(revision, finding) {
				return ErrInvalid
			}
			key := opportunityKey(finding)
			if !existing[key] {
				newKeys[key] = true
			}
		}
		if len(m.Opportunities)+len(newKeys) > revision.Budget.MaxActions {
			return ErrConflict
		}
		for _, finding := range findings {
			key, found := opportunityKey(finding), -1
			for j := range m.Opportunities {
				if m.Opportunities[j].DedupeKey == key {
					found = j
					break
				}
			}
			if found < 0 {
				oid, e := newID()
				if e != nil {
					return e
				}
				m.Opportunities = append(m.Opportunities, StewardshipOpportunity{ID: oid, DedupeKey: key, Status: "open", Rank: len(m.Opportunities) + 1, Version: 1, Comments: []OpportunityComment{}})
				found = len(m.Opportunities) - 1
			}
			o := &m.Opportunities[found]
			citations := make([]OpportunityCitation, 0, len(o.Citations)+len(finding.Citations))
			for _, old := range o.Citations {
				if old.Stale || (o.EvidenceRevision != "" && o.EvidenceRevision != finding.EvidenceRevision) {
					old.Stale = true
					citations = append(citations, old)
				}
			}
			for _, citation := range finding.Citations {
				citation.Stale = false
				citations = append(citations, citation)
			}
			if o.EvaluatedAt.IsZero() {
				o.EvaluatedAt = now
			} else {
				o.Version++
			}
			o.MandateVersion, o.RepositoryID, o.EvidenceType, o.EvidenceID, o.EvidenceRevision = m.Version, finding.RepositoryID, finding.EvidenceType, finding.EvidenceID, finding.EvidenceRevision
			o.Title, o.Summary, o.Severity, o.ExpectedValue, o.Confidence = strings.TrimSpace(finding.Title), strings.TrimSpace(finding.Summary), finding.Severity, strings.TrimSpace(finding.ExpectedValue), finding.Confidence
			o.AffectedOwnerIDs, o.AffectedRevisions, o.Citations, o.InScopeReason = finding.AffectedOwnerIDs, finding.AffectedRevisions, citations, strings.TrimSpace(finding.InScopeReason)
			o.EvaluatedBy, o.UpdatedBy, o.UpdatedAt = actor, actor, now
			out = append(out, *o)
		}
		return s.event(v, "stewardship_opportunities.evaluated", actor, mandateID, map[string]any{"mandate_version": m.Version, "findings": len(findings)})
	})
	return v, out, err
}

func (s *Store) DecideStewardshipOpportunity(id, mandateID, opportunityID, actor string, decision OpportunityDecision) (Organization, StewardshipOpportunity, error) {
	var out StewardshipOpportunity
	v, err := s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "") {
			return ErrNotFound
		}
		i := mandateIndex(v, mandateID)
		if i < 0 {
			return ErrNotFound
		}
		m := &v.StewardshipMandates[i]
		j := -1
		for k := range m.Opportunities {
			if m.Opportunities[k].ID == opportunityID {
				j = k
				break
			}
		}
		if j < 0 {
			return ErrNotFound
		}
		o := &m.Opportunities[j]
		if o.Version != decision.ExpectedVersion || len(decision.Reason) > 1000 || len(decision.Comment) > 4000 {
			return ErrConflict
		}
		now := s.now().Truncate(time.Microsecond)
		switch decision.Action {
		case "rank":
			if decision.Rank < 1 || decision.Rank > 100000 {
				return ErrInvalid
			}
			order := make([]int, 0, len(m.Opportunities)-1)
			for k := range m.Opportunities {
				if k != j {
					order = append(order, k)
				}
			}
			slices.SortStableFunc(order, func(a, b int) int {
				left, right := m.Opportunities[a], m.Opportunities[b]
				if left.Rank != right.Rank {
					return left.Rank - right.Rank
				}
				if compared := right.UpdatedAt.Compare(left.UpdatedAt); compared != 0 {
					return compared
				}
				return strings.Compare(left.ID, right.ID)
			})
			position := min(decision.Rank-1, len(order))
			order = append(order, 0)
			copy(order[position+1:], order[position:])
			order[position] = j
			for rank, k := range order {
				peer := &m.Opportunities[k]
				if peer.Rank == rank+1 {
					continue
				}
				peer.Rank = rank + 1
				if k != j {
					peer.Version++
					peer.UpdatedBy, peer.UpdatedAt = actor, now
				}
			}
		case "dismiss":
			o.Status = "dismissed"
			o.DecisionReason = strings.TrimSpace(decision.Reason)
		case "incorrect":
			o.Status = "incorrect"
			o.DecisionReason = strings.TrimSpace(decision.Reason)
		case "snooze":
			if decision.Until == nil || !decision.Until.After(now) {
				return ErrInvalid
			}
			until := decision.Until.UTC()
			o.Status, o.SnoozedUntil = "snoozed", &until
			o.DecisionReason = strings.TrimSpace(decision.Reason)
		case "reopen":
			o.Status, o.SnoozedUntil, o.DecisionReason = "open", nil, ""
		case "comment":
			body, ok := clean(decision.Comment, 4000)
			if !ok {
				return ErrInvalid
			}
			cid, e := newID()
			if e != nil {
				return e
			}
			o.Comments = append(o.Comments, OpportunityComment{ID: cid, ActorID: actor, Body: body, CreatedAt: now})
		default:
			return ErrInvalid
		}
		o.Version++
		o.UpdatedBy, o.UpdatedAt = actor, now
		out = *o
		return s.event(v, "stewardship_opportunity."+decision.Action, actor, opportunityID, map[string]any{"version": o.Version})
	})
	return v, out, err
}
