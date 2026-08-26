"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Diagnostic = {
  kind: string;
  severity: string;
  message: string;
  resource_id?: string;
  rule_id?: string;
  attributed_to: string;
};
type Revision = {
  title: string;
  summary: string;
  resources: Array<{
    id: string;
    kind: string;
    name: string;
    owner_team_ids: string[];
  }>;
  teams: Array<{
    id: string;
    name: string;
    member_ids: string[];
    skills: string[];
    contact: string;
  }>;
  rules: Array<{
    id: string;
    resource_ids: string[];
    signal_class: string;
    severity: string;
    accountable_team_id: string;
    required_skills: string[];
    acknowledge_seconds: number;
    resolve_seconds: number;
    expected_actions: string[];
    escalations: Array<{
      after_seconds: number;
      team_id: string;
      audience_ids: string[];
      expected_action: string;
    }>;
    communication_audience_ids: string[];
    incident_criteria: string[];
    authority: {
      required_access: string[];
      permitted_actions: string[];
      prohibited_actions: string[];
    };
  }>;
  exceptions: Array<{
    id: string;
    rule_id: string;
    reason: string;
    follow_up_id: string;
    expires_at: string;
  }>;
  change_reason: string;
  created_by?: string;
  created_at?: string;
};
type Policy = {
  id: string;
  current_version: number;
  revisions: Revision[];
  diagnostics: Diagnostic[];
  updated_at: string;
};
type Rotation = {
  id: string;
  current_version: number;
  event_version: number;
  revisions: Array<{
    name: string;
    policy_id: string;
    team_id: string;
    time_zone: string;
    handoff_window_minutes: number;
    responders: Array<{ user_id: string; qualifications: string[]; availability: Array<{ weekdays: string[]; start_local: string; end_local: string }>; max_shifts_per_week: number }>;
    absence_rules: Array<{ kind: string; notice_hours: number; action: string }>;
    shifts: Array<{ id: string; starts_at: string; ends_at: string; primary_user_id: string; backup_user_ids: string[]; required_qualifications: string[] }>;
    change_reason: string;
  }>;
  events: Array<{ id: string; kind: string; shift_id: string; from_user_id: string; to_user_id: string; status: string; reason?: string; context: Array<{ kind: string; resource_id: string; revision: string; summary: string }> }>;
  diagnostics: Array<{ kind: string; severity: string; message: string; shift_id?: string; user_id?: string; escalation: string }>;
  effective_owner_by_shift: Record<string, string>;
};
type Alert = {
  id: string; policy_id: string; policy_version: number; rule_id: string;
  signal: { signal_class: string; severity: string; summary: string; uncertainty: string; resource_ids: string[]; affected_user_count: number; affected_user_groups: string[]; evidence: Array<{ kind: string; resource_id: string; revision: string; summary: string; available: boolean }> };
  first_seen_at: string; last_seen_at: string; event_count: number; acknowledge_by: string; resolve_by: string; state: string;
  routing: Array<{ id: string; recipient_id: string; channel: string; status: string; failure?: string }>;
  diagnostics: string[]; expected_actions: string[]; permitted_actions: string[]; prohibited_actions: string[];
  workspace: { version: number; classification?: string; responder_id?: string; participant_ids: string[]; context: Array<{ kind: string; resource_id: string; revision: string; summary: string; window_from: string; window_to: string }>; timeline: Array<{ id: string; kind: string; actor_id: string; message: string; created_at: string }>; diagnostic_runs: Array<{ id: string; name: string; state: string; summary: string }>; investigations: Array<{ id: string; agent_id: string; mandate: string; state: string; permitted_tools: string[]; budget: number }>; incident_id?: string };
};
type OutcomeReport = { alert_volume: number; deduplicated_events: number; false_positives: number; handoffs: number; escalations: number; missed_acknowledgements: number; missed_resolutions: number; acknowledgement_seconds: number[]; resolution_seconds: number[]; consented_interruption_minutes: number; consented_responder_load: Record<string, number>; agent_cost: number; incidents: number; consented_user_outcomes: Array<{ alert_id: string; outcome: string }> };
const template = (owner = ""): Revision => ({
  title: "Urgent response coverage",
  summary: "Accountability before an alert arrives",
  resources: [
    {
      id: "repository",
      kind: "repository",
      name: "Repository",
      owner_team_ids: ["maintainers"],
    },
  ],
  teams: [
    {
      id: "maintainers",
      name: "Maintainers",
      member_ids: owner ? [owner] : [],
      skills: ["service operations"],
      contact: "#maintainers",
    },
  ],
  rules: [
    {
      id: "reliability-critical",
      resource_ids: ["repository"],
      signal_class: "reliability",
      severity: "critical",
      accountable_team_id: "maintainers",
      required_skills: ["service operations"],
      acknowledge_seconds: 300,
      resolve_seconds: 3600,
      expected_actions: ["Assess impact and preserve evidence"],
      escalations: [],
      communication_audience_ids: ["repository owners", "support"],
      incident_criteria: ["Confirmed user impact exceeds five minutes"],
      authority: {
        required_access: ["repository:read"],
        permitted_actions: ["investigate and coordinate"],
        prohibited_actions: [
          "deploy, disclose, or spend without separate authority",
        ],
      },
    },
  ],
  exceptions: [],
  change_reason: "Initial coverage contract",
});

export function ResponsePoliciesWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { user } = useAuth();
  return (
    <ScopedResponsePoliciesWorkspace
      key={`${repositoryID}:${user?.id ?? "unauthenticated"}`}
      repositoryID={repositoryID}
    />
  );
}

function ScopedResponsePoliciesWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth();
  const [items, setItems] = useState<Policy[]>([]),
    [selected, setSelected] = useState<Policy>(),
    [draft, setDraft] = useState(""),
    [rotations, setRotations] = useState<Rotation[]>([]),
    [alerts, setAlerts] = useState<Alert[]>([]),
    [outcomes, setOutcomes] = useState<OutcomeReport>(),
    [rotationDraft, setRotationDraft] = useState(""),
    [error, setError] = useState(""),
    [publishing, setPublishing] = useState(false);
  const publishingRef = useRef(false);
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const out = await api<{ response_policies: Policy[] }>(
        `/repositories/${repositoryID}/response-policies`,
        {},
        token,
      );
      setItems(out.response_policies);
      setSelected(out.response_policies[0]);
      setDraft(JSON.stringify(template(user?.id), null, 2));
      const rotationOut = await api<{ response_rotations: Rotation[] }>(`/repositories/${repositoryID}/response-rotations`, {}, token);
      setRotations(rotationOut.response_rotations);
      const alertOut = await api<{ response_alerts: Alert[] }>(`/repositories/${repositoryID}/response-alerts`, {}, token);
      setAlerts(alertOut.response_alerts);
      void api<OutcomeReport>(`/repositories/${repositoryID}/response-outcomes`, {}, token).then(setOutcomes).catch(() => setOutcomes(undefined));
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Response coverage could not be loaded",
      );
    }
  }, [repositoryID, token, user]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function publish(e: FormEvent) {
    e.preventDefault();
    if (!token || publishingRef.current) return;
    publishingRef.current = true;
    setPublishing(true);
    try {
      const out = await api<Policy>(
        selected
          ? `/repositories/${repositoryID}/response-policies/${selected.id}/revisions`
          : `/repositories/${repositoryID}/response-policies`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: crypto.randomUUID(),
            expected_version: selected?.current_version,
            revision: JSON.parse(draft),
          }),
        },
        token,
      );
      setItems((v) => [out, ...v.filter((x) => x.id !== out.id)]);
      setSelected(out);
      setDraft(JSON.stringify(out.revisions.at(-1), null, 2));
      setError("");
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Response policy could not be published",
      );
    } finally {
      publishingRef.current = false;
      setPublishing(false);
    }
  }
  function newRotation() {
    const policy = selected ?? items[0], revision = policy?.revisions.at(-1), member = revision?.teams[0]?.member_ids[0] ?? user?.id ?? "", backup = revision?.teams[0]?.member_ids.find((id) => id !== member) ?? "replace-with-backup-user-id";
    const start = new Date(Date.now() + 3600000), end = new Date(start.getTime() + 8 * 3600000);
    setRotationDraft(JSON.stringify({ name: "Primary response rotation", policy_id: policy?.id ?? "response-policy-id", team_id: revision?.teams[0]?.id ?? "team-id", time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC", handoff_window_minutes: 30, responders: [{ user_id: member, qualifications: ["service operations"], availability: [{ weekdays: ["monday", "tuesday", "wednesday", "thursday", "friday"], start_local: "09:00", end_local: "17:00" }], max_shifts_per_week: 5 }, { user_id: backup, qualifications: ["service operations"], availability: [{ weekdays: ["monday", "tuesday", "wednesday", "thursday", "friday"], start_local: "09:00", end_local: "17:00" }], max_shifts_per_week: 5 }], absence_rules: [{ kind: "planned", notice_hours: 24, action: "Offer duty to the first qualified backup" }], shifts: [{ id: "upcoming-primary", starts_at: start.toISOString(), ends_at: end.toISOString(), primary_user_id: member, backup_user_ids: [backup], required_qualifications: ["service operations"] }], change_reason: "Publish accountable duty" }, null, 2));
  }
  async function publishRotation(e: FormEvent) { e.preventDefault(); if (!token) return; try { const out = await api<Rotation>(`/repositories/${repositoryID}/response-rotations`, { method: "POST", body: JSON.stringify({ request_id: crypto.randomUUID(), revision: JSON.parse(rotationDraft) }) }, token); setRotations((v) => [out, ...v]); setRotationDraft(""); setError(""); } catch (x) { setError(x instanceof Error ? x.message : "Rotation could not be published"); } }
  async function duty(rotation: Rotation, body: object, eventID?: string) { if (!token) return; try { const out = await api<Rotation>(eventID ? `/repositories/${repositoryID}/response-rotations/${rotation.id}/duty-events/${eventID}/accept` : `/repositories/${repositoryID}/response-rotations/${rotation.id}/duty-events`, { method: "POST", body: JSON.stringify(body) }, token); setRotations((v) => v.map((x) => x.id === out.id ? out : x)); setError(""); } catch (x) { setError(x instanceof Error ? x.message : "Duty could not be updated"); } }
  async function actOnAlert(alert: Alert, kind: "acknowledge" | "resolve") { if (!token) return; try { const out = await api<Alert>(`/repositories/${repositoryID}/response-alerts/${alert.id}/events`, { method: "POST", body: JSON.stringify({ request_id: crypto.randomUUID(), kind, reason: kind === "resolve" ? "Response work completed" : "Accepted response responsibility" }) }, token); setAlerts((values) => values.map((value) => value.id === out.id ? out : value)); setError(""); } catch (x) { setError(x instanceof Error ? x.message : "Alert could not be updated"); } }
  async function steerAlert(alert: Alert, kind: string) { if (!token) return; const message = window.prompt(kind === "delegate_agent" ? "Bounded investigation mandate" : "Attributable reason or observation"); if (!message) return; const body: Record<string, unknown> = { request_id: crypto.randomUUID(), kind, message }; if (kind === "classify") body.classification = window.prompt("Classification: actionable, false_positive, duplicate, needs_information, or incident_candidate", "actionable"); if (kind === "invite" || kind === "reassign") body.target_user_id = window.prompt("Current repository participant ID"); if (kind === "delegate_agent") { const now = new Date().toISOString(); body.agent_id = window.prompt("Approved read-only agent ID"); body.budget = 20; body.permitted_tools = ["read_context", "compare_releases", "inspect_dependencies", "read_runbook"]; body.context = alert.workspace.context.length ? alert.workspace.context : alert.signal.evidence.map((item) => ({ kind: "evidence", resource_id: item.resource_id, revision: item.revision, summary: item.summary, window_from: now, window_to: now })); } if (kind === "diagnostic") { body.diagnostic_name = "health_snapshot"; body.diagnostic_state = "passed"; body.output_digest = window.prompt("SHA-256 diagnostic output digest"); } try { const out = await api<Alert>(`/repositories/${repositoryID}/response-alerts/${alert.id}/workspace`, { method: "POST", body: JSON.stringify(body) }, token); setAlerts((values) => values.map((value) => value.id === out.id ? out : value)); setError(""); } catch (x) { setError(x instanceof Error ? x.message : "Response workspace could not be updated"); } }
  async function reviewOutcome(alert: Alert) { if (!token) return; const classification = window.prompt("Outcome: actionable, false_positive, duplicate, needs_information, or incident_candidate", alert.workspace.classification ?? "actionable"); const rationale = window.prompt("Attributable outcome rationale"); if (!classification || !rationale) return; const routingAction = window.prompt("Routing containment: pause, activate_backup, resume, or leave blank", ""); const interruption = Number(window.prompt("Consented interruption minutes (leave 0 when not consented)", "0") || 0); try { await api(`/repositories/${repositoryID}/response-alerts/${alert.id}/outcomes`, { method: "POST", body: JSON.stringify({ request_id: crypto.randomUUID(), classification, rationale, routing_action: routingAction || "", interruption_minutes: interruption, responder_load_consent: interruption > 0, agent_cost: alert.workspace.investigations.reduce((sum, item) => sum + item.budget, 0) }) }, token); setOutcomes(await api<OutcomeReport>(`/repositories/${repositoryID}/response-outcomes`, {}, token)); setError(""); } catch (x) { setError(x instanceof Error ? x.message : "Outcome review could not be retained"); } }
  async function transfer(rotation: Rotation, shiftID: string, kind: "swap" | "delegate" | "override") { const recipient = window.prompt("Exact recipient user ID"); if (!recipient) return; const reason = window.prompt("Reason for this duty transfer"); if (!reason) return; const resource = window.prompt("Active context resource ID"); const revision = window.prompt("Exact context revision"); const summary = window.prompt("Bounded handoff summary"); if (!resource || !revision || !summary) return; await duty(rotation, { request_id: crypto.randomUUID(), expected_version: rotation.event_version, kind, shift_id: shiftID, to_user_id: recipient, reason, context: [{ kind: "active_response", resource_id: resource, revision, summary }] }); }
  const current = selected?.revisions.at(-1);
  return (
    <main className="mx-auto w-full max-w-6xl space-y-7 px-6 py-8">
      <header>
        <p className="text-sm font-semibold text-[var(--accent)]">
          On-call coordination
        </p>
        <h1 className="mt-2 text-3xl font-semibold">Response coverage</h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Define who responds to each urgent condition, their skills, targets,
          escalation, audiences, incident threshold, and exact authority
          boundary before a signal arrives.
        </p>
      </header>
      <div className="grid gap-6 lg:grid-cols-[.8fr_1.2fr]">
        <section className="space-y-3">
          {items.map((x) => (
            <button
              key={x.id}
              className="w-full rounded-xl border p-4 text-left"
              disabled={publishing}
              onClick={() => {
                setSelected(x);
                setDraft(JSON.stringify(x.revisions.at(-1), null, 2));
              }}
            >
              <Badge>v{x.current_version}</Badge>
              <strong className="ml-2">{x.revisions.at(-1)?.title}</strong>
              <p className="mt-2 text-xs text-[var(--muted)]">
                {x.diagnostics.length} visible coverage gap(s)
              </p>
            </button>
          ))}
          {current && (
            <Card className="p-5">
              <h2 className="font-semibold">Current responsibility map</h2>
              {current.rules.map((rule) => (
                <article key={rule.id} className="mt-4 border-l-2 pl-3">
                  <div className="flex gap-2">
                    <Badge
                      tone={rule.severity === "critical" ? "danger" : "warning"}
                    >
                      {rule.severity}
                    </Badge>
                    <Badge>{rule.signal_class}</Badge>
                  </div>
                  <p className="mt-2 font-medium">
                    {rule.accountable_team_id} · acknowledge{" "}
                    {rule.acknowledge_seconds}s · resolve {rule.resolve_seconds}
                    s
                  </p>
                  <p className="mt-1 text-sm">
                    {rule.expected_actions.join(" · ")}
                  </p>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    Authority permits{" "}
                    {rule.authority.permitted_actions.join(", ") ||
                      "nothing declared"}
                    ; prohibits{" "}
                    {rule.authority.prohibited_actions.join(", ") ||
                      "nothing declared"}
                    .
                  </p>
                </article>
              ))}
              {selected!.diagnostics.map((d, i) => (
                <p key={`${d.kind}-${i}`} className="mt-3 text-sm">
                  <Badge
                    tone={d.severity === "blocking" ? "danger" : "warning"}
                  >
                    {d.kind}
                  </Badge>{" "}
                  {d.message}{" "}
                  <span className="text-[var(--muted)]">
                    — {d.attributed_to}
                  </span>
                </p>
              ))}
            </Card>
          )}
        </section>
        <Card className="p-5">
          <div className="flex items-center justify-between">
            <h2 className="font-semibold">
              {selected
                ? `Publish successor v${selected.current_version + 1}`
                : "Define a policy"}
            </h2>
            <Button
                type="button"
                variant="secondary"
                disabled={publishing}
              onClick={() => {
                setSelected(undefined);
                setDraft(JSON.stringify(template(user?.id), null, 2));
              }}
            >
              New policy
            </Button>
          </div>
          <form onSubmit={publish}>
            <textarea
              aria-label="Response policy JSON"
              rows={34}
              className="mt-3 w-full rounded-lg border bg-slate-950 p-4 font-mono text-xs text-slate-100"
              value={draft}
              disabled={publishing}
              onChange={(e) => setDraft(e.target.value)}
            />
            <Button className="mt-3" disabled={publishing}>
              {publishing ? "Publishing…" : "Publish immutable coverage"}
            </Button>
          </form>
          {error && (
            <p role="alert" className="mt-3 text-sm text-[var(--danger)]">
              {error}
            </p>
          )}
          <p className="mt-4 text-xs text-[var(--muted)]">
            This policy coordinates response. It grants no repository, secret,
            deployment, disclosure, incident, spending, or operational
            authority.
          </p>
        </Card>
      </div>
      {outcomes && <section className="space-y-4" aria-labelledby="outcomes-heading"><div><h2 id="outcomes-heading" className="text-xl font-semibold">Response outcomes</h2><p className="mt-1 text-sm text-[var(--muted)]">Owner-visible, attributable learning. Responder load and user outcomes appear only with explicit consent; targets remain misses rather than normalized averages.</p></div><Card className="grid gap-3 p-5 sm:grid-cols-2 lg:grid-cols-4"><p><strong>{outcomes.alert_volume}</strong><br/><span className="text-xs text-[var(--muted)]">alerts · {outcomes.deduplicated_events} deduplicated</span></p><p><strong>{outcomes.missed_acknowledgements} / {outcomes.missed_resolutions}</strong><br/><span className="text-xs text-[var(--muted)]">missed acknowledge / resolve</span></p><p><strong>{outcomes.false_positives}</strong><br/><span className="text-xs text-[var(--muted)]">false positives · {outcomes.incidents} incidents</span></p><p><strong>{outcomes.handoffs} / {outcomes.escalations}</strong><br/><span className="text-xs text-[var(--muted)]">handoffs / escalations</span></p><p><strong>{outcomes.consented_interruption_minutes}m</strong><br/><span className="text-xs text-[var(--muted)]">consented interruptions</span></p><p><strong>{outcomes.agent_cost}</strong><br/><span className="text-xs text-[var(--muted)]">declared agent cost</span></p></Card></section>}
      <section className="space-y-4" aria-labelledby="alerts-heading">
        <div><h2 id="alerts-heading" className="text-xl font-semibold">Actionable alerts</h2><p className="mt-1 text-sm text-[var(--muted)]">Correlated signals retain exact policy, evidence, delivery, uncertainty, and deadline context. Delivery never counts as acknowledgement.</p></div>
        {alerts.length === 0 && <Card className="p-5 text-sm text-[var(--muted)]">No alerts are currently routed to you or an audience you belong to.</Card>}
        {alerts.map((alert) => <Card className="p-5" key={alert.id}><div className="flex flex-wrap items-start justify-between gap-3"><div><div className="flex gap-2"><Badge tone={alert.signal.severity === "critical" ? "danger" : "warning"}>{alert.signal.severity}</Badge><Badge>{alert.signal.signal_class}</Badge><Badge tone={alert.state === "open" ? "warning" : alert.state === "resolved" ? "success" : "neutral"}>{alert.state}</Badge></div><h3 className="mt-3 font-semibold">{alert.signal.summary}</h3><p className="mt-1 text-sm text-[var(--muted)]">{alert.event_count} correlated event(s) · policy {alert.policy_id} v{alert.policy_version} · rule {alert.rule_id}</p></div><div className="text-right text-xs"><p>Acknowledge by {new Date(alert.acknowledge_by).toLocaleString()}</p><p className="mt-1">Resolve by {new Date(alert.resolve_by).toLocaleString()}</p></div></div>
          <p className="mt-3 text-sm">Uncertainty: {alert.signal.uncertainty} · {alert.signal.affected_user_count} affected user(s) {alert.signal.affected_user_groups.join(", ")}</p>
          <div className="mt-3 grid gap-2 md:grid-cols-2">{alert.signal.evidence.map((evidence) => <div className="rounded-lg border p-3 text-xs" key={`${evidence.kind}-${evidence.resource_id}-${evidence.revision}`}><Badge tone={evidence.available ? "success" : "warning"}>{evidence.available ? "available" : "inaccessible"}</Badge><p className="mt-2 font-medium">{evidence.kind} · {evidence.resource_id}@{evidence.revision}</p><p className="mt-1 text-[var(--muted)]">{evidence.summary}</p></div>)}</div>
          {alert.diagnostics.length > 0 && <p className="mt-3 text-sm text-[var(--danger)]">Attention gaps: {alert.diagnostics.join(", ")}</p>}<p className="mt-3 text-sm">Expected: {alert.expected_actions.join(" · ")}</p><p className="mt-1 text-xs text-[var(--muted)]">Permitted: {alert.permitted_actions.join(", ")}. Prohibited: {alert.prohibited_actions.join(", ")}.</p>
          <div className="mt-4 rounded-lg border p-4"><div className="flex flex-wrap gap-2"><Badge>{alert.workspace.classification ?? "unclassified"}</Badge><Badge>owner {alert.workspace.responder_id || "unassigned"}</Badge>{alert.workspace.incident_id && <Badge tone="danger">incident {alert.workspace.incident_id}</Badge>}</div><p className="mt-2 text-xs text-[var(--muted)]">Shared with {alert.workspace.participant_ids.join(", ") || "no participant"} · {alert.workspace.context.length} exact context binding(s) · workspace v{alert.workspace.version}</p>
            <div className="mt-3 flex flex-wrap gap-2">{alert.state === "open" && <Button onClick={() => void actOnAlert(alert, "acknowledge")}>Acknowledge response</Button>}{alert.state === "acknowledged" && <Button onClick={() => void actOnAlert(alert, "resolve")}>Mark resolved</Button>}<Button variant="secondary" onClick={() => void steerAlert(alert, "classify")}>Classify</Button><Button variant="secondary" onClick={() => void steerAlert(alert, "observe")}>Record observation</Button><Button variant="secondary" onClick={() => void steerAlert(alert, "invite")}>Invite owner</Button><Button variant="secondary" onClick={() => void steerAlert(alert, "reassign")}>Reassign</Button><Button variant="secondary" onClick={() => void steerAlert(alert, "diagnostic")}>Run approved diagnostic</Button><Button variant="secondary" onClick={() => void steerAlert(alert, "delegate_agent")}>Delegate read-only investigation</Button><Button variant="quiet" onClick={() => void steerAlert(alert, "escalate")}>Escalate</Button><Button variant="quiet" onClick={() => void steerAlert(alert, "suppress")}>Suppress</Button><Button variant="quiet" onClick={() => void steerAlert(alert, "promote_incident")}>Declare incident</Button>{outcomes && <Button variant="quiet" onClick={() => void reviewOutcome(alert)}>Review outcome</Button>}</div>
            {alert.workspace.investigations.map((item) => <p className="mt-3 text-sm" key={item.id}><Badge>{item.state}</Badge> Agent {item.agent_id}: {item.mandate} · read-only tools {item.permitted_tools.join(", ")} · budget {item.budget}</p>)}
            {alert.workspace.timeline.slice().reverse().map((item) => <p className="mt-2 border-l-2 pl-3 text-sm" key={item.id}><span className="font-medium">{item.kind}</span> by {item.actor_id}: {item.message}</p>)}
            <p className="mt-3 text-xs text-[var(--muted)]">Diagnostics and agents can only read the frozen packet. Mitigation still requires the ordinary environment authority; this workspace grants no deployment, secret, disclosure, incident, or operational control.</p></div>
        </Card>)}
      </section>
      <section className="space-y-4" aria-labelledby="duty-heading">
        <div className="flex flex-wrap items-end justify-between gap-3"><div><h2 id="duty-heading" className="text-xl font-semibold">Current and upcoming duty</h2><p className="mt-1 text-sm text-[var(--muted)]">Published shifts recheck membership, qualifications, workload, gaps, and handoffs on every read.</p></div><Button variant="secondary" onClick={newRotation}>Publish a rotation</Button></div>
        {rotationDraft && <Card className="p-5"><form onSubmit={publishRotation}><textarea aria-label="Response rotation JSON" rows={24} className="w-full rounded-lg border bg-slate-950 p-4 font-mono text-xs text-slate-100" value={rotationDraft} onChange={(e) => setRotationDraft(e.target.value)}/><Button className="mt-3">Publish immutable schedule</Button></form></Card>}
        {rotations.length === 0 && !rotationDraft && <Card className="p-5 text-sm text-[var(--muted)]">No duty rotation is published. Coverage is not yet assigned to a current responder.</Card>}
        {rotations.map((rotation) => { const revision = rotation.revisions.at(-1)!; return <Card key={rotation.id} className="p-5"><div className="flex flex-wrap items-start justify-between gap-3"><div><h3 className="font-semibold">{revision.name}</h3><p className="mt-1 text-xs text-[var(--muted)]">{revision.time_zone} · {revision.handoff_window_minutes} minute handoff · schedule v{rotation.current_version}</p></div><Badge tone={rotation.diagnostics.some((d) => d.severity === "blocking") ? "danger" : "success"}>{rotation.diagnostics.length ? `${rotation.diagnostics.length} escalation(s)` : "covered"}</Badge></div>
          <div className="mt-4 grid gap-3 md:grid-cols-2">{revision.shifts.map((shift) => { const effectiveOwner = rotation.effective_owner_by_shift[shift.id] ?? shift.primary_user_id; return <article key={shift.id} className="rounded-lg border p-3"><p className="font-medium">{new Date(shift.starts_at).toLocaleString()} → {new Date(shift.ends_at).toLocaleString()}</p><p className="mt-1 text-sm">Current owner {effectiveOwner} · scheduled primary {shift.primary_user_id} · backup {shift.backup_user_ids.join(", ")}</p><p className="mt-1 text-xs text-[var(--muted)]">Requires {shift.required_qualifications.join(", ")}</p>{user?.id === effectiveOwner && <Button className="mt-3" variant="secondary" onClick={() => void duty(rotation, { request_id: crypto.randomUUID(), expected_version: rotation.event_version, kind: "acknowledge", shift_id: shift.id, context: [] })}>Acknowledge my duty</Button>} {revision.responders.some((responder) => responder.user_id === user?.id) && <span className="mt-3 inline-flex flex-wrap gap-2"><Button variant="quiet" onClick={() => void transfer(rotation, shift.id, "swap")}>Swap</Button><Button variant="quiet" onClick={() => void transfer(rotation, shift.id, "delegate")}>Delegate</Button><Button variant="quiet" onClick={() => void transfer(rotation, shift.id, "override")}>Override</Button></span>}</article>; })}</div>
          {rotation.events.map((event) => <div key={event.id} className="mt-3 rounded-lg border p-3 text-sm"><Badge tone={event.status === "accepted" ? "success" : "warning"}>{event.kind} {event.status}</Badge><span className="ml-2">{event.from_user_id} → {event.to_user_id}</span>{event.context.map((item) => <p key={`${item.kind}-${item.resource_id}-${item.revision}`} className="mt-2 text-xs">{item.kind} {item.resource_id}@{item.revision}: {item.summary}</p>)}{event.status === "pending" && event.to_user_id === user?.id && <Button className="ml-3" variant="secondary" onClick={() => void duty(rotation, { expected_version: rotation.event_version }, event.id)}>Accept exact handoff</Button>}</div>)}
          {rotation.diagnostics.map((diagnostic, index) => <p className="mt-3 text-sm" key={`${diagnostic.kind}-${index}`}><Badge tone={diagnostic.severity === "blocking" ? "danger" : "warning"}>{diagnostic.kind.replaceAll("_", " ")}</Badge> {diagnostic.message} <span className="text-[var(--muted)]">{diagnostic.escalation}</span></p>)}
          <p className="mt-4 text-xs text-[var(--muted)]">Duty coordinates response only. It grants no repository, secret, deployment, disclosure, incident, spending, governance, or operational authority.</p></Card>; })}
      </section>
    </main>
  );
}
