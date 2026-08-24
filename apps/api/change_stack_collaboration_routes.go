package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changestacks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerChangeStackCollaborationRoutes(mux *http.ServeMux, catalog *repositories.Store, orgs *organizations.Store, credentials *auth.Store, stacks *changestacks.Store) {
	loadMember := func(repo, stackID, memberID string) (changestacks.Stack, changestacks.Member, error) {
		v, err := stacks.Get(repo, stackID)
		if err != nil {
			return v, changestacks.Member{}, err
		}
		for _, m := range v.Members {
			if m.ID == memberID {
				return v, m, nil
			}
		}
		return v, changestacks.Member{}, changestacks.ErrNotFound
	}
	actorID := func(c auth.Credential) string {
		if c.AgentID != "" {
			return c.AgentID
		}
		return c.UserID
	}

	mux.HandleFunc("POST /repositories/{id}/change-stacks/{stack_id}/members/{member_id}/assignments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "change_stack_assignment_forbidden", "a human source-repository participant must assign stack work")
			return
		}
		_, member, err := loadMember(r.PathValue("id"), r.PathValue("stack_id"), r.PathValue("member_id"))
		if err != nil {
			writeAPIError(w, 404, "change_stack_member_not_found", "stack member not found")
			return
		}
		source := member.SourceRepositoryID
		if source == "" {
			source = r.PathValue("id")
		}
		var in struct {
			PrincipalType string `json:"principal_type"`
			PrincipalID   string `json:"principal_id"`
			AccessGrantID string `json:"access_grant_id"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a human or approved-agent assignment is required")
			return
		}
		assignment := changestacks.Assignment{PrincipalType: in.PrincipalType, PrincipalID: in.PrincipalID, AccessGrantID: in.AccessGrantID, AssignedBy: actor.UserID}
		var out changestacks.Stack
		var created changestacks.Assignment
		persist := func() error {
			var e error
			out, created, e = stacks.Assign(r.PathValue("id"), r.PathValue("stack_id"), member.ID, assignment)
			return e
		}
		if in.PrincipalType == "human" {
			err = catalog.WithCurrentParticipants([]string{actor.UserID, in.PrincipalID}, source, persist)
		} else if in.PrincipalType == "agent" {
			repo, repoErr := catalog.GetByID(source)
			if repoErr != nil || orgs == nil {
				err = changestacks.ErrInvalid
			} else {
				assignment.OperatorID = actor.UserID
				err = orgs.WithCurrentAgentGrant(repo.OrganizationID, in.AccessGrantID, in.PrincipalID, source, func() error { return catalog.WithCurrentParticipant(actor.UserID, source, persist) })
			}
		} else {
			err = changestacks.ErrInvalid
		}
		if err != nil {
			writeAPIError(w, 422, "change_stack_assignment_invalid", "assignee and assigner must retain ordinary source access; agents require a current repository grant")
			return
		}
		_ = out
		writeJSON(w, 201, map[string]any{"assignment": created})
	})

	mux.HandleFunc("POST /repositories/{id}/change-stacks/{stack_id}/members/{member_id}/work-launches", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		v, member, err := loadMember(r.PathValue("id"), r.PathValue("stack_id"), r.PathValue("member_id"))
		if err != nil {
			writeAPIError(w, 404, "change_stack_member_not_found", "stack member not found")
			return
		}
		var in struct {
			RequestID    string `json:"request_id"`
			Kind         string `json:"kind"`
			AssignmentID string `json:"assignment_id"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a caller-stable scoped work launch is required")
			return
		}
		var assignment *changestacks.Assignment
		for i := range v.Assignments {
			if v.Assignments[i].ID == in.AssignmentID && v.Assignments[i].MemberID == member.ID {
				assignment = &v.Assignments[i]
			}
		}
		if assignment == nil || assignment.PrincipalID != actorID(actor) {
			writeAPIError(w, 403, "change_stack_work_forbidden", "only the currently assigned principal may open this layer")
			return
		}
		source := member.SourceRepositoryID
		if source == "" {
			source = r.PathValue("id")
		}
		if !changeStackAgentMemberScope(actor, source, member.SourceBranch) {
			writeAPIError(w, 403, "change_stack_agent_scope_mismatch", "agent authority must remain bound to this exact source repository and branch")
			return
		}
		clean, _ := json.Marshal(in)
		digest := sha256.Sum256(clean)
		launch := changestacks.WorkLaunch{RequestID: in.RequestID, RequestDigest: hex.EncodeToString(digest[:]), MemberID: member.ID, Kind: in.Kind, AssignmentID: in.AssignmentID, OpenedBy: actorID(actor)}
		var out changestacks.Stack
		var opened changestacks.WorkLaunch
		persist := func() error {
			var e error
			out, opened, e = stacks.OpenWork(r.PathValue("id"), r.PathValue("stack_id"), launch)
			return e
		}
		if actor.AgentID != "" {
			err = orgs.WithCurrentAgentGrant(actor.OrganizationID, actor.AccessGrantID, actor.AgentID, source, persist)
		} else {
			err = catalog.WithCurrentParticipant(actor.UserID, source, persist)
		}
		if err != nil {
			writeAPIError(w, 422, "change_stack_work_invalid", "work kind, assignment, source authority, or request identity is no longer valid")
			return
		}
		w.Header().Set("Location", "/repositories/"+r.PathValue("id")+"/change-stacks/"+r.PathValue("stack_id")+"/work-launches/"+opened.ID)
		opened.CurrentUpstream = true
		_ = out
		writeJSON(w, 201, map[string]any{"work_launch": opened})
	})

	mux.HandleFunc("GET /repositories/{id}/change-stacks/{stack_id}/work-launches/{launch_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := stacks.Get(r.PathValue("id"), r.PathValue("stack_id"))
		if e != nil {
			writeAPIError(w, 404, "change_stack_work_not_found", "work launch not found")
			return
		}
		for _, launch := range v.WorkLaunches {
			if launch.ID == r.PathValue("launch_id") {
				memberSource := ""
				for _, m := range v.Members {
					if m.ID == launch.MemberID {
						memberSource = m.SourceRepositoryID
					}
				}
				if memberSource == "" {
					memberSource = v.RepositoryID
				}
				allowed, _ := catalog.HasCollaborator(actor.UserID, memberSource)
				repo, _ := catalog.GetByID(memberSource)
				if actor.RepositoryID == memberSource || actor.UserID == repo.OwnerID || allowed {
					current := map[string]string{}
					for _, member := range v.Members {
						current[member.ID] = member.Revision
					}
					launch.CurrentUpstream, launch.ChangedUpstream = stackUpstreamCurrent(launch.UpstreamRevisions, current)
					writeJSON(w, 200, launch)
					return
				}
				break
			}
		}
		writeAPIError(w, 404, "change_stack_work_not_found", "work launch is outside the caller's source-repository boundary")
	})

	mux.HandleFunc("POST /repositories/{id}/change-stacks/{stack_id}/timeline", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var in changestacks.TimelineEvent
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a revision-bound stack event is required")
			return
		}
		in.ActorID = actorID(actor)
		if actor.AgentID != "" {
			in.ActorType = "agent"
		} else {
			in.ActorType = "human"
		}
		v, member, e := loadMember(r.PathValue("id"), r.PathValue("stack_id"), in.MemberID)
		if e != nil {
			writeAPIError(w, 404, "change_stack_member_not_found", "stack member not found")
			return
		}
		assigned := false
		for _, a := range v.Assignments {
			if a.MemberID == member.ID && a.Status == "active" && a.PrincipalID == in.ActorID {
				assigned = true
			}
		}
		for _, prior := range v.Timeline {
			if prior.RequestID == in.RequestID && prior.ActorID == in.ActorID {
				assigned = true
			}
		}
		source := member.SourceRepositoryID
		if source == "" {
			source = r.PathValue("id")
		}
		if !changeStackAgentMemberScope(actor, source, member.SourceBranch) {
			writeAPIError(w, 403, "change_stack_agent_scope_mismatch", "agent authority must remain bound to this exact source repository and branch")
			return
		}
		if !assigned || (actor.AgentID != "" && in.Kind == "restack_proposal") {
			writeAPIError(w, 403, "change_stack_timeline_forbidden", "only the assigned principal may publish layer events; agents cannot propose restacks")
			return
		}
		if in.Kind == "handoff" && in.FromPrincipalID != in.ActorID {
			writeAPIError(w, 422, "change_stack_handoff_sender_mismatch", "handoff sender must be the authenticated assigned principal")
			return
		}
		clean := in
		clean.ID = ""
		clean.RequestDigest = ""
		clean.ActorID = ""
		clean.ActorType = ""
		clean.CreatedAt = clean.CreatedAt.UTC()
		clean.UpstreamRevisions = nil
		clean.CurrentUpstream = false
		clean.ChangedUpstream = nil
		b, _ := json.Marshal(clean)
		d := sha256.Sum256(b)
		in.RequestDigest = hex.EncodeToString(d[:])
		if in.Revision == "" {
			in.Revision = member.Revision
		}
		var out changestacks.Stack
		var event changestacks.TimelineEvent
		persist := func() error {
			var x error
			out, event, x = stacks.AppendTimeline(r.PathValue("id"), r.PathValue("stack_id"), in)
			return x
		}
		if actor.AgentID != "" {
			e = orgs.WithCurrentAgentGrant(actor.OrganizationID, actor.AccessGrantID, actor.AgentID, source, persist)
		} else {
			e = catalog.WithCurrentParticipant(actor.UserID, source, persist)
		}
		if errors.Is(e, changestacks.ErrInvalid) || e != nil {
			writeAPIError(w, 422, "change_stack_timeline_invalid", "event must bind the current layer, upstream snapshot, work context, and any referenced restack")
			return
		}
		_ = out
		event.CurrentUpstream = true
		writeJSON(w, 201, map[string]any{"event": event})
	})
}

func changeStackAgentMemberScope(actor auth.Credential, sourceRepositoryID, sourceBranch string) bool {
	return actor.AgentID == "" || (actor.RepositoryID == sourceRepositoryID && (actor.GitWriteBranch == "" || canonicalStackBranch(actor.GitWriteBranch) == canonicalStackBranch(sourceBranch)))
}
