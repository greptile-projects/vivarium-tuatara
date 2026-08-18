"use client";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
type Ref = {
  kind: string;
  resource_id?: string;
  revision?: string;
  label: string;
};
type Policy = {
  data_categories: string[];
  privacy: string;
  security: string;
  retention_hours: number;
  sample_percent: number;
  max_cost_cents: number;
  max_load_percent: number;
};
type Probe = {
  id: string;
  kind: string;
  purpose: string;
  status: string;
  requested_by: string;
  audience_user_ids: string[];
  requested_policy: Policy;
  approved_policy?: Policy;
  expires_at: string;
  decision_reason?: string;
  actions: {
    id: string;
    outcome: string;
    provenance: string;
    gaps: string[];
    transformations: string[];
    artifacts: {
      kind: string;
      digest: string;
      size_bytes: number;
      reference: string;
      redaction: string;
    }[];
  }[];
};
type Citation = {
  id: string;
  kind: string;
  resource_id?: string;
  revision?: string;
  path?: string;
  symbol?: string;
  line_start?: number;
  line_end?: number;
  label: string;
  evidence_id?: string;
  accessible: boolean;
  blocked_reason?: string;
};
type Claim = {
  id: string;
  kind: string;
  statement: string;
  uncertainty: string;
  confidence: string;
  citation_ids: string[];
  status: string;
  created_by: string;
  agent_investigation_id?: string;
  responses: { id: string; actor_id: string; kind: string; message: string }[];
};
type AgentInvestigation = {
  id: string;
  agent_id: string;
  mandate: string;
  citation_ids: string[];
  state: string;
  guidance: { id: string; message?: string }[];
};
type OwnerRequest = {
  id: string;
  owner_type: string;
  owner_id: string;
  question: string;
  citation_ids: string[];
  status: string;
  response?: string;
};
type RepairWork = {
  id: string;
  scenario_id: string;
  cause_claim_id: string;
  affected_revision: string;
  acceptance_criteria: string[];
  regression_criteria: string[];
  proposal_id: string;
  task_id: string;
  assignee_type: string;
  assignee_id: string;
  pull_request_id?: string;
  pull_revision?: string;
  scenario_check_run_ids: string[];
  required_check_run_ids: string[];
  release_id?: string;
  deployment_id?: string;
  validation_status: string;
  validation_summary?: string;
  validation_signal_names: string[];
  reopened_diagnosis: boolean;
};
type Replay = {
  id: string;
  parent_scenario_id?: string;
  title: string;
  objective: string;
  commit_id: string;
  evidence_citation_ids: string[];
  dependencies: string[];
  commands: { name: string; sha256: string; purpose: string }[];
  invariants: {
    name: string;
    command_name: string;
    expected_exit_code: number;
    description: string;
  }[];
  production_differences: string[];
  unsafe_side_effects: string[];
  gaps: string[];
  status: string;
  attempts: {
    id: string;
    workspace_id: string;
    result: string;
    cost_cents: number;
    production_differences: string[];
    gaps: string[];
    invariants: { name: string; passed: boolean }[];
  }[];
};
type W = {
  id: string;
  version: number;
  title: string;
  summary: string;
  trigger: Ref;
  release: Ref;
  environment: Ref;
  time_start: string;
  time_end: string;
  user_journey: string;
  owner_ids: string[];
  severity: string;
  audience: string;
  status: string;
  source: Ref;
  packages: Ref[];
  configuration: Ref;
  infrastructure: Ref;
  permitted_evidence: {
    id: string;
    kind: string;
    label: string;
    visibility: string;
    available: boolean;
    unavailable_reason?: string;
  }[];
  unavailable_context: string[];
  hypotheses: {
    id: string;
    statement: string;
    status: string;
    created_by: string;
  }[];
  history: {
    id: string;
    kind: string;
    actor_id: string;
    message?: string;
    created_at: string;
  }[];
  probes: Probe[];
  citations: Citation[];
  claims: Claim[];
  owner_requests: OwnerRequest[];
  agent_investigations: AgentInvestigation[];
  replay_scenarios: Replay[];
  repair_work: RepairWork[];
};
const field = "rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-2 text-sm";
const val = (f: FormData, n: string) => String(f.get(n) || "").trim();
const list = (v: string) =>
  v
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
const normalized = (w: W): W => ({
  ...w,
  citations: w.citations ?? [],
  claims: w.claims ?? [],
  owner_requests: w.owner_requests ?? [],
  agent_investigations: w.agent_investigations ?? [],
  replay_scenarios: w.replay_scenarios ?? [],
  repair_work: w.repair_work ?? [],
});
export function DebuggingWorkspace({ repositoryID }: { repositoryID: string }) {
  const { token, user } = useAuth();
  const [items, setItems] = useState<W[]>([]),
    [selected, setSelected] = useState<W>(),
    [error, setError] = useState("");
  const load = useCallback(async () => {
    try {
      const x = await api<{ debugging_workspaces: W[] }>(`/repositories/${repositoryID}/debugging-workspaces`, {}, token),
        values = x.debugging_workspaces.map(normalized);
      setItems(values);
      setSelected((s) => values.find((v) => v.id === s?.id) ?? values[0]);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Debugging workspaces could not be loaded");
    }
  }, [repositoryID, token]);
  useEffect(() => {
    // This effect synchronizes repository and identity changes with the API-backed workspace list.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load();
  }, [load]);
  useEffect(() => {
    if (!token) return;
    const timer = window.setInterval(() => void load(), 5000);
    return () => window.clearInterval(timer);
  }, [load, token]);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    const f = new FormData(e.currentTarget),
      releaseRevision = val(f, "release_revision"),
      evidenceRef = val(f, "evidence_reference"),
      infraID = val(f, "infrastructure_id"),
      packageID = val(f, "package_id");
    const evidenceLabel = val(f, "evidence_label");
    try {
      const out = await api<W>(
        `/repositories/${repositoryID}/debugging-workspaces`,
        {
          method: "POST",
          body: JSON.stringify({
            title: val(f, "title"),
            summary: val(f, "summary"),
            trigger: {
              kind: val(f, "trigger_kind"),
              resource_id: val(f, "trigger_id") || undefined,
              label: val(f, "trigger_label"),
            },
            release: {
              kind: "release",
              resource_id: val(f, "release_id"),
              revision: releaseRevision,
              label: val(f, "release_label"),
            },
            environment: {
              kind: "environment",
              resource_id: val(f, "environment_id"),
              label: val(f, "environment_label"),
            },
            time_start: new Date(val(f, "time_start")).toISOString(),
            time_end: new Date(val(f, "time_end")).toISOString(),
            user_journey: val(f, "journey"),
            owner_ids: list(val(f, "owners")),
            severity: val(f, "severity"),
            audience: val(f, "audience"),
            access_user_ids: list(val(f, "access")),
            source: {
              kind: "commit",
              resource_id: repositoryID,
              revision: releaseRevision,
              label: "Released source",
            },
            packages: packageID
              ? [
                  {
                    kind: "package",
                    resource_id: packageID,
                    revision: val(f, "package_version"),
                    label: val(f, "package_label"),
                  },
                ]
              : [],
            configuration: {
              kind: "configuration",
              revision: releaseRevision,
              label: "Configuration at release",
            },
            infrastructure: {
              kind: "infrastructure",
              resource_id: infraID || undefined,
              revision: infraID ? val(f, "infrastructure_version") : undefined,
              label: infraID ? "Infrastructure definition" : "Unavailable",
            },
            permitted_evidence: evidenceLabel
              ? [
                  {
                    kind: val(f, "evidence_kind"),
                    reference: evidenceRef,
                    label: evidenceLabel,
                    visibility: val(f, "evidence_visibility"),
                    sanitization: val(f, "sanitization"),
                    available: Boolean(evidenceRef),
                    unavailable_reason: evidenceRef ? undefined : val(f, "evidence_gap"),
                  },
                ]
              : [],
            unavailable_context: list(val(f, "unavailable")),
          }),
        },
        token,
      );
      setItems((x) => [out, ...x]);
      setSelected(out);
      setError("");
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Workspace could not be opened");
    }
  }
  async function event(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      const out = await api<W>(
        `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/events`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            kind: val(f, "kind"),
            value: val(f, "value"),
            message: val(f, "message"),
          }),
        },
        token,
      );
      setSelected(out);
      setItems((xs) => xs.map((x) => (x.id === out.id ? out : x)));
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "History could not be appended");
    }
  }
  const update = (value: W) => {
    const out = normalized(value);
    setSelected(out);
    setItems((xs) => xs.map((x) => (x.id === out.id ? out : x)));
    setError("");
  };
  const policy = (f: FormData): Policy => ({
    data_categories: list(val(f, "categories")),
    privacy: val(f, "privacy"),
    security: val(f, "security"),
    retention_hours: Number(val(f, "retention")),
    sample_percent: Number(val(f, "sampling")),
    max_cost_cents: Number(val(f, "cost")),
    max_load_percent: Number(val(f, "load")),
  });
  async function requestProbe(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget),
      kind = val(f, "probe_kind");
    try {
      update(
        await api<W>(
          `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/probes`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: selected.version,
              probe: {
                kind,
                purpose: val(f, "purpose"),
                definition_path: kind === "dynamic_diagnostic" ? val(f, "definition_path") : undefined,
                definition_revision: kind === "dynamic_diagnostic" ? selected.source.revision : undefined,
                audience_user_ids: list(val(f, "probe_audience")),
                requested_policy: policy(f),
                expires_at: new Date(val(f, "probe_expiry")).toISOString(),
              },
            }),
          },
          token,
        ),
      );
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Probe could not be requested");
    }
  }
  async function decideProbe(e: FormEvent<HTMLFormElement>, probe: Probe) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      update(
        await api<W>(
          `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/probes/${probe.id}/decision`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: selected.version,
              decision: val(f, "decision"),
              reason: val(f, "reason"),
              policy: policy(f),
              expires_at: new Date(val(f, "approval_expiry")).toISOString(),
            }),
          },
          token,
        ),
      );
    } catch (x) {
      setError(x instanceof Error ? x.message : "Probe decision could not be retained");
    }
  }
  async function reportProbe(e: FormEvent<HTMLFormElement>, probe: Probe) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget),
      outcome = val(f, "outcome"),
      now = new Date().toISOString(),
      ref = val(f, "artifact_reference");
    try {
      update(
        await api<W>(
          `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/probes/${probe.id}/actions`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: selected.version,
              action: {
                outcome,
                started_at: new Date(val(f, "started_at")).toISOString(),
                finished_at: now,
                provenance: val(f, "provenance"),
                transformations: list(val(f, "transformations")),
                gaps: list(val(f, "gaps")),
                artifacts: ref
                  ? [
                      {
                        kind: val(f, "artifact_kind"),
                        digest: val(f, "artifact_digest"),
                        size_bytes: Number(val(f, "artifact_size")),
                        reference: ref,
                        redaction: val(f, "redaction"),
                      },
                    ]
                  : [],
              },
            }),
          },
          token,
        ),
      );
    } catch (x) {
      setError(x instanceof Error ? x.message : "Probe outcome could not be retained");
    }
  }
  async function revokeProbe(e: FormEvent<HTMLFormElement>, probe: Probe) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      update(
        await api<W>(
          `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/probes/${probe.id}/revoke`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: selected.version,
              reason: val(f, "revoke_reason"),
            }),
          },
          token,
        ),
      );
    } catch (x) {
      setError(x instanceof Error ? x.message : "Probe consent could not be revoked");
    }
  }
  async function publishClaim(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget),
      kind = val(f, "citation_kind"),
      resource = val(f, "citation_resource"),
      evidence = val(f, "citation_evidence");
    const citation = {
      kind,
      label: val(f, "citation_label"),
      resource_id: resource || undefined,
      evidence_id: evidence || undefined,
      revision: val(f, "citation_revision") || undefined,
      path: val(f, "citation_path") || undefined,
      symbol: val(f, "citation_symbol") || undefined,
      line_start: Number(val(f, "line_start")) || undefined,
      line_end: Number(val(f, "line_end")) || undefined,
    };
    try {
      update(
        await api<W>(
          `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/claims`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: selected.version,
              claim: {
                kind: val(f, "claim_kind"),
                statement: val(f, "statement"),
                uncertainty: val(f, "uncertainty"),
                confidence: val(f, "confidence"),
              },
              citations: [citation],
            }),
          },
          token,
        ),
      );
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Cited claim could not be published");
    }
  }
  async function respondClaim(e: FormEvent<HTMLFormElement>, claim: Claim) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      update(
        await api<W>(
          `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/claims/${claim.id}/responses`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: selected.version,
              kind: val(f, "response_kind"),
              message: val(f, "response_message"),
              citation_ids: list(val(f, "response_citations")),
            }),
          },
          token,
        ),
      );
    } catch (x) {
      setError(x instanceof Error ? x.message : "Claim response could not be retained");
    }
  }
  async function requestOwner(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      update(
        await api<W>(
          `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/owner-requests`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: selected.version,
              request: {
                owner_type: val(f, "owner_type"),
                owner_id: val(f, "owner_id"),
                question: val(f, "owner_question"),
                citation_ids: list(val(f, "owner_citations")),
              },
            }),
          },
          token,
        ),
      );
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Owner input could not be requested");
    }
  }
  async function startAgent(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      const out = await api<{ debugging_workspace: W }>(
        `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/agent-investigations`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            mandate: val(f, "agent_mandate"),
            citation_ids: list(val(f, "agent_citations")),
            expires_in: Number(val(f, "agent_expiry")),
          }),
        },
        token,
      );
      update(out.debugging_workspace);
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Read-only investigation could not be started");
    }
  }
  async function controlAgent(e: FormEvent<HTMLFormElement>, agent: AgentInvestigation) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      update(
        await api<W>(
          `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/agent-investigations/${agent.id}/controls`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: selected.version,
              action: val(f, "agent_action"),
              message: val(f, "agent_guidance"),
            }),
          },
          token,
        ),
      );
    } catch (x) {
      setError(x instanceof Error ? x.message : "Agent control could not be applied");
    }
  }
  async function createReplay(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget),
      command = val(f, "replay_command");
    try {
      const out = await api<{ debugging_workspace: W }>(
        `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/replay-scenarios`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            scenario: {
              parent_scenario_id: val(f, "parent_scenario") || undefined,
              title: val(f, "replay_title"),
              objective: val(f, "replay_objective"),
              evidence_citation_ids: list(val(f, "replay_citations")),
              inputs: [
                {
                  name: val(f, "input_name"),
                  kind: val(f, "input_kind"),
                  schema: val(f, "input_schema"),
                  sha256: val(f, "input_digest"),
                  sanitization: val(f, "input_sanitization"),
                },
              ],
              dependencies: list(val(f, "replay_dependencies")),
              commands: [
                {
                  name: val(f, "command_name"),
                  sha256: command,
                  purpose: val(f, "command_purpose"),
                },
              ],
              invariants: [
                {
                  name: val(f, "invariant_name"),
                  command_name: val(f, "command_name"),
                  expected_exit_code: Number(val(f, "expected_exit")),
                  description: val(f, "invariant_description"),
                },
              ],
              production_differences: list(val(f, "production_differences")),
              unsafe_side_effects: list(val(f, "unsafe_side_effects")),
              gaps: list(val(f, "replay_gaps")),
            },
          }),
        },
        token,
      );
      update(out.debugging_workspace);
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Replay scenario could not be retained");
    }
  }
  async function launchReplay(replay: Replay) {
    if (!token || !selected) return;
    try {
      const workspace = await api<{ id: string }>(
        "/workspaces",
        {
          method: "POST",
          body: JSON.stringify({
            repository_id: repositoryID,
            commit_id: replay.commit_id,
            source: {
              kind: "debugging_reproduction",
              debugging_workspace_id: selected.id,
              replay_scenario_id: replay.id,
            },
          }),
        },
        token,
      );
      window.location.assign(`/workspaces/${workspace.id}`);
    } catch (x) {
      setError(x instanceof Error ? x.message : "Isolated replay workspace could not be launched");
    }
  }
  async function retainReplay(e: FormEvent<HTMLFormElement>, replay: Replay) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      const out = await api<{ debugging_workspace: W }>(
        `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/replay-scenarios/${replay.id}/attempts`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            attempt: {
              workspace_id: val(f, "attempt_workspace"),
              command_outcome_ids: list(val(f, "attempt_outcomes")),
              cost_cents: Number(val(f, "attempt_cost")),
              production_differences: list(val(f, "attempt_differences")),
              gaps: list(val(f, "attempt_gaps")),
              traces: [],
            },
          }),
        },
        token,
      );
      update(out.debugging_workspace);
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Replay evidence could not be retained");
    }
  }
  async function createRepair(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      const out = await api<{ debugging_workspace: W }>(
        `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/repair-work`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            scenario_id: val(f, "repair_scenario"),
            cause_claim_id: val(f, "repair_cause"),
            title: val(f, "repair_title"),
            acceptance_criteria: list(val(f, "repair_acceptance")),
            regression_criteria: list(val(f, "repair_regression")),
            assignee_type: val(f, "repair_owner_type"),
            assignee_id: val(f, "repair_owner"),
          }),
        },
        token,
      );
      update(out.debugging_workspace);
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Repair work could not be created");
    }
  }
  async function validateRepair(e: FormEvent<HTMLFormElement>, work: RepairWork) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      const out = await api<{ debugging_workspace: W }>(
        `/repositories/${repositoryID}/debugging-workspaces/${selected.id}/repair-work/${work.id}/validation`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            pull_request_id: val(f, "repair_pull"),
            check_run_ids: list(val(f, "repair_checks")),
            release_id: val(f, "repair_release"),
            deployment_id: val(f, "repair_deployment"),
            signal_names: list(val(f, "repair_signals")),
            outcome: val(f, "repair_outcome"),
            summary: val(f, "repair_summary"),
            action: val(f, "repair_action"),
          }),
        },
        token,
      );
      update(out.debugging_workspace);
    } catch (x) {
      setError(x instanceof Error ? x.message : "Repair validation could not be retained");
    }
  }
  return (
    <main id="main-content" className="mx-auto max-w-7xl px-6 py-8">
      <p className="font-mono text-xs font-semibold uppercase tracking-[.16em] text-[var(--brand)]">Runtime collaboration</p>
      <h1 className="mt-2 text-3xl font-semibold">Production debugging</h1>
      <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">Establish the exact running behavior and released code under investigation. This shared context grants no observability, deployment, Git, or environment authority.</p>
      {error && <p className="mt-4 text-sm text-[var(--danger)]">{error}</p>}
      {token && (
        <Card className="mt-6 p-5">
          <h2 className="font-semibold">Open a debugging workspace</h2>
          <form onSubmit={create} className="mt-4 grid gap-3 md:grid-cols-3">
            <I n="title" p="Workspace title" />
            <I n="summary" p="Observed behavior and impact" />
            <S n="trigger_kind" a={["issue", "incident", "support_thread", "deployment", "service_objective", "trace", "manual_observation"]} />
            <I n="trigger_id" p="Trigger ID (optional for trace/manual)" req={false} />
            <I n="trigger_label" p="Trigger label" />
            <I n="release_id" p="Affected release ID" />
            <I n="release_revision" p="Exact 40-character release commit" />
            <I n="release_label" p="Release label" />
            <I n="environment_id" p="Environment ID" />
            <I n="environment_label" p="Environment label" />
            <I n="time_start" type="datetime-local" p="Window start" />
            <I n="time_end" type="datetime-local" p="Window end" />
            <I n="journey" p="Affected user journey" />
            <I n="owners" p="Owner IDs, comma separated" />
            <S n="severity" a={["low", "medium", "high", "critical"]} />
            <S n="audience" a={["repository", "restricted"]} />
            <I n="access" p="Restricted reader IDs" req={false} />
            <I n="package_id" p="Affected package ID" req={false} />
            <I n="package_version" p="Exact package version" req={false} />
            <I n="package_label" p="Package label" req={false} />
            <I n="infrastructure_id" p="Infrastructure definition ID" req={false} />
            <I n="infrastructure_version" p="Infrastructure version" req={false} />
            <I n="evidence_label" p="Permitted evidence label" req={false} />
            <S n="evidence_kind" a={["observation", "trace", "log", "metric", "report", "link", "profile", "snapshot"]} />
            <I n="evidence_reference" p="Sanitized evidence reference" req={false} />
            <S n="evidence_visibility" a={["repository", "restricted"]} />
            <I n="sanitization" p="Redaction/sanitization applied" req={false} />
            <I n="evidence_gap" p="Why evidence is unavailable" req={false} />
            <I n="unavailable" p="Other unavailable context, comma separated" req={false} />
            <div>
              <Button>Open workspace</Button>
            </div>
          </form>
        </Card>
      )}
      <div className="mt-6 grid gap-6 lg:grid-cols-[.7fr_1.3fr]">
        <Card className="p-4">
          <h2 className="font-semibold">Shared workspaces</h2>
          {items.length === 0 && <p className="mt-3 text-sm text-[var(--muted)]">No visible debugging workspace yet.</p>}
          {items.map((x) => (
            <button key={x.id} onClick={() => setSelected(x)} className="mt-3 block w-full rounded-lg border p-3 text-left">
              <div className="flex gap-2">
                <Badge tone={x.severity === "critical" ? "danger" : "neutral"}>{x.severity}</Badge>
                <Badge>{x.status}</Badge>
              </div>
              <strong className="mt-2 block text-sm">{x.title}</strong>
              <span className="text-xs text-[var(--muted)]">
                {x.trigger.kind.replaceAll("_", " ")} · {x.user_journey}
              </span>
            </button>
          ))}
        </Card>
        {selected && (
          <Card className="p-5">
            <div className="flex flex-wrap gap-2">
              <Badge>{selected.audience}</Badge>
              <Badge>{selected.status}</Badge>
              <Badge tone={selected.severity === "critical" ? "danger" : "warning"}>{selected.severity}</Badge>
            </div>
            <h2 className="mt-3 text-xl font-semibold">{selected.title}</h2>
            <p className="mt-2 text-sm">{selected.summary}</p>
            <dl className="mt-4 grid gap-3 text-sm sm:grid-cols-2">
              <D k="Running release" v={`${selected.release.label} · ${selected.release.revision?.slice(0, 12)}`} />
              <D k="Environment" v={selected.environment.label} />
              <D k="Observed window" v={`${new Date(selected.time_start).toLocaleString()} — ${new Date(selected.time_end).toLocaleString()}`} />
              <D k="Journey" v={selected.user_journey} />
              <D k="Owners" v={selected.owner_ids.join(", ")} />
              <D k="Source and configuration" v={selected.configuration.revision?.slice(0, 12) || "unavailable"} />
              <D k="Packages" v={selected.packages.map((x) => `${x.label} ${x.revision}`).join(", ") || "none declared"} />
              <D k="Infrastructure" v={selected.infrastructure.resource_id ? `${selected.infrastructure.label} v${selected.infrastructure.revision}` : "unavailable"} />
            </dl>
            <h3 className="mt-5 font-semibold">Permitted evidence</h3>
            {selected.permitted_evidence.map((x) => (
              <p key={x.id} className="mt-2 rounded-lg bg-[var(--surface-soft)] p-3 text-sm">
                {x.kind}: {x.label} · {x.available ? x.visibility : `unavailable — ${x.unavailable_reason}`}
              </p>
            ))}
            {selected.unavailable_context.map((x) => (
              <p key={x} className="mt-2 text-sm text-[var(--warning)]">
                Unavailable: {x}
              </p>
            ))}
            <section aria-labelledby="diagnosis-heading">
              <div className="mt-6 flex items-center gap-2">
                <h3 id="diagnosis-heading" className="font-semibold">
                  Live, cited diagnosis
                </h3>
                <Badge tone="success">refreshes every 5s</Badge>
              </div>
              <p className="mt-1 text-xs text-[var(--muted)]">Every claim points to server-resolved runtime evidence and revision-exact project context. Status changes remain attributable.</p>
              {selected.claims.length === 0 && <p className="mt-3 text-sm text-[var(--muted)]">No cited explanation has been published.</p>}
              {selected.claims.map((c) => (
                <article key={c.id} className="mt-3 rounded-lg border p-3 text-sm">
                  <div className="flex flex-wrap gap-2">
                    <Badge>{c.kind}</Badge>
                    <Badge tone={c.status === "supported" ? "success" : c.status === "disputed" ? "danger" : c.status === "blocked" || c.status === "stale" ? "warning" : "neutral"}>{c.status}</Badge>
                    <Badge>{c.confidence} confidence</Badge>
                  </div>
                  <p className="mt-2 font-medium">{c.statement}</p>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    Uncertainty: {c.uncertainty} · by {c.created_by}
                  </p>
                  <ul className="mt-2 text-xs">
                    {c.citation_ids.map((id) => {
                      const ref = selected.citations.find((x) => x.id === id);
                      return (
                        <li key={id}>
                          {ref?.kind}: {ref?.label}
                          {ref && !ref.accessible ? ` — blocked: ${ref.blocked_reason}` : ""} <span className="font-mono">{ref?.revision?.slice(0, 12)}</span>
                        </li>
                      );
                    })}
                  </ul>
                  {c.responses.map((r) => (
                    <p key={r.id} className="mt-2 border-t pt-2 text-xs">
                      <strong>{r.kind}</strong> by {r.actor_id}: {r.message}
                    </p>
                  ))}
                  {user && (
                    <form onSubmit={(e) => respondClaim(e, c)} className="mt-3 grid gap-2 sm:grid-cols-3">
                      <S n="response_kind" a={["support", "dispute", "mark_stale"]} />
                      <I n="response_message" p="Reason another collaborator can inspect" />
                      <I n="response_citations" p="Additional citation IDs" req={false} />
                      <div>
                        <Button>Respond to claim</Button>
                      </div>
                    </form>
                  )}
                </article>
              ))}
              {user && (
                <form onSubmit={publishClaim} className="mt-4 grid gap-2 sm:grid-cols-3">
                  <S n="claim_kind" a={["hypothesis", "query", "finding", "uncertainty"]} />
                  <I n="statement" p="Challengeable claim or exact query" />
                  <I n="uncertainty" p="What remains unknown" />
                  <S n="confidence" a={["low", "medium", "high"]} />
                  <S n="citation_kind" a={["runtime_evidence", "symbol", "commit", "dependency", "configuration", "infrastructure", "deployment", "known_issue"]} />
                  <I n="citation_label" p="What this citation demonstrates" />
                  <I n="citation_evidence" p="Permitted evidence ID" req={false} />
                  <I n="citation_resource" p="Resource or artifact digest" req={false} />
                  <I n="citation_revision" p="Exact commit/version" req={false} />
                  <I n="citation_path" p="Source path" req={false} />
                  <I n="citation_symbol" p="Symbol" req={false} />
                  <I n="line_start" p="Start line" type="number" req={false} />
                  <I n="line_end" p="End line" type="number" req={false} />
                  <div>
                    <Button>Publish cited claim</Button>
                  </div>
                </form>
              )}
              <h3 className="mt-6 font-semibold">Owner input</h3>
              {selected.owner_requests.map((x) => (
                <p key={x.id} className="mt-2 rounded-lg bg-[var(--surface-soft)] p-3 text-sm">
                  <Badge>{x.owner_type}</Badge> {x.question} — {x.status}
                  {x.response && `: ${x.response}`}
                </p>
              ))}
              {user && (
                <form onSubmit={requestOwner} className="mt-3 grid gap-2 sm:grid-cols-3">
                  <S n="owner_type" a={["code", "service", "privacy", "security"]} />
                  <I n="owner_id" p="Responsible owner ID" />
                  <I n="owner_question" p="Specific input requested" />
                  <I n="owner_citations" p="Citation IDs, comma separated" />
                  <div>
                    <Button>Request owner input</Button>
                  </div>
                </form>
              )}
              <h3 className="mt-6 font-semibold">Read-only agent investigations</h3>
              {selected.agent_investigations.map((a) => (
                <div key={a.id} className="mt-2 rounded-lg border p-3 text-sm">
                  <Badge>{a.state}</Badge>
                  <p className="mt-2">{a.mandate}</p>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    Agent {a.agent_id} · citations {a.citation_ids.join(", ")}
                  </p>
                  {user && (
                    <form onSubmit={(e) => controlAgent(e, a)} className="mt-3 grid gap-2 sm:grid-cols-3">
                      <S n="agent_action" a={["guide", "pause", "resume", "revoke"]} />
                      <I n="agent_guidance" p="Guidance or control rationale" req={false} />
                      <div>
                        <Button>Apply control</Button>
                      </div>
                    </form>
                  )}
                </div>
              ))}
              {user && selected.citations.length > 0 && (
                <form onSubmit={startAgent} className="mt-3 grid gap-2 sm:grid-cols-3">
                  <I n="agent_mandate" p="Bounded diagnostic mandate" />
                  <I n="agent_citations" p="Permitted citation IDs" />
                  <I n="agent_expiry" p="Lifetime in seconds (300–86400)" type="number" />
                  <div>
                    <Button>Start read-only investigation</Button>
                  </div>
                </form>
              )}
            </section>
            <section aria-labelledby="replay-heading">
              <h3 id="replay-heading" className="mt-6 font-semibold">
                Privacy-bounded replay
              </h3>
              <p className="mt-1 text-xs text-[var(--muted)]">Minimize selected evidence into synthetic inputs and repository-defined commands. Two distinct passing isolated workspaces are required before the behavior is labeled reproduced.</p>
              {selected.replay_scenarios.map((r) => (
                <article key={r.id} className="mt-3 rounded-lg border p-3 text-sm">
                  <div className="flex flex-wrap gap-2">
                    <Badge tone={r.status === "reproduced" ? "success" : r.status === "nondeterministic" || r.status === "blocked" || r.status === "unsafe_side_effects" ? "warning" : "neutral"}>{r.status.replaceAll("_", " ")}</Badge>
                    {r.parent_scenario_id && <Badge>refinement</Badge>}
                  </div>
                  <p className="mt-2 font-medium">{r.title}</p>
                  <p className="mt-1">{r.objective}</p>
                  <p className="mt-2 text-xs text-[var(--muted)]">
                    Revision <code>{r.commit_id.slice(0, 12)}</code> · {r.dependencies.length} dependencies · {r.production_differences.join(", ")}
                  </p>
                  {r.gaps.map((x) => (
                    <p key={x} className="mt-1 text-xs text-[var(--warning)]">
                      Gap: {x}
                    </p>
                  ))}
                  {r.unsafe_side_effects.map((x) => (
                    <p key={x} className="mt-1 text-xs text-[var(--danger)]">
                      Unsafe side effect: {x}
                    </p>
                  ))}
                  <div className="mt-3 flex flex-wrap gap-2">{r.unsafe_side_effects.length === 0 && <Button onClick={() => launchReplay(r)}>Launch isolated workspace</Button>}</div>
                  {r.attempts.map((a) => (
                    <p key={a.id} className="mt-2 border-t pt-2 text-xs">
                      <strong>{a.result.replaceAll("_", " ")}</strong> in workspace {a.workspace_id} · {a.cost_cents}¢ · {a.production_differences.join(", ")}
                      {a.gaps.length ? ` · gaps: ${a.gaps.join(", ")}` : ""}
                    </p>
                  ))}
                  {user && (
                    <form onSubmit={(e) => retainReplay(e, r)} className="mt-3 grid gap-2 sm:grid-cols-2">
                      <I n="attempt_workspace" p="Completed replay workspace ID" />
                      <I n="attempt_outcomes" p="Command outcome IDs" />
                      <I n="attempt_cost" p="Attempt cost in cents" type="number" />
                      <I n="attempt_differences" p="Differences from production" />
                      <I n="attempt_gaps" p="Missing dependencies or gaps" req={false} />
                      <div>
                        <Button>Retain attempt evidence</Button>
                      </div>
                    </form>
                  )}
                </article>
              ))}
              {user && selected.citations.length > 0 && (
                <form onSubmit={createReplay} className="mt-4 grid gap-2 sm:grid-cols-3">
                  <I n="replay_title" p="Minimized scenario title" />
                  <I n="replay_objective" p="Behavior to demonstrate" />
                  <I n="parent_scenario" p="Parent scenario ID for refinement" req={false} />
                  <I n="replay_citations" p="Permitted citation IDs" />
                  <I n="input_name" p="Synthetic input name" />
                  <S n="input_kind" a={["synthetic", "privacy_preserving"]} />
                  <I n="input_schema" p="Bounded generated shape, no values" />
                  <I n="input_digest" p="Synthetic input SHA-256" />
                  <I n="input_sanitization" p="Privacy transformations" />
                  <I n="replay_dependencies" p="Isolated dependencies" req={false} />
                  <I n="command_name" p="Repository experiment name" />
                  <I n="replay_command" p="Repository command SHA-256" />
                  <I n="command_purpose" p="Command purpose" />
                  <I n="invariant_name" p="Invariant name" />
                  <I n="expected_exit" p="Expected exit code" type="number" />
                  <I n="invariant_description" p="Observable invariant" />
                  <I n="production_differences" p="Differences from production" />
                  <I n="unsafe_side_effects" p="Unsafe effects to remove" req={false} />
                  <I n="replay_gaps" p="Missing or irreducible conditions" req={false} />
                  <div>
                    <Button>Retain replay scenario</Button>
                  </div>
                </form>
              )}
            </section>
            <section aria-labelledby="repair-heading">
              <h3 id="repair-heading" className="mt-6 font-semibold">
                Governed repair and real-world validation
              </h3>
              <p className="mt-1 text-xs text-[var(--muted)]">Freeze the reproduced scenario, supported cause, affected revision, and criteria into ordinary owned work. Delivery still requires linked review, scenario and required checks, integration, release, and staged deployment.</p>
              {selected.repair_work.map((work) => (
                <article key={work.id} className="mt-3 rounded-lg border p-3 text-sm">
                  <div className="flex flex-wrap gap-2">
                    <Badge tone={work.validation_status === "validated" ? "success" : work.validation_status === "failed" ? "warning" : "neutral"}>{work.validation_status.replaceAll("_", " ")}</Badge>
                    <Badge>{work.assignee_type} owned</Badge>
                    {work.reopened_diagnosis && <Badge tone="warning">diagnosis reopened</Badge>}
                  </div>
                  <p className="mt-2">
                    Affected revision <code>{work.affected_revision.slice(0, 12)}</code> · proposal {work.proposal_id} · task {work.task_id}
                  </p>
                  <p className="mt-1 text-xs">
                    Acceptance: {work.acceptance_criteria.join(", ")} · regression: {work.regression_criteria.join(", ")}
                  </p>
                  {work.deployment_id && (
                    <p className="mt-1 text-xs">
                      Pull {work.pull_request_id} · release {work.release_id} · deployment {work.deployment_id} · signals {work.validation_signal_names.join(", ")}
                    </p>
                  )}
                  {user && work.validation_status !== "validated" && (
                    <form onSubmit={(e) => validateRepair(e, work)} className="mt-3 grid gap-2 sm:grid-cols-3">
                      <I n="repair_pull" p="Linked merged pull ID" />
                      <I n="repair_checks" p="Scenario and required check run IDs" />
                      <I n="repair_release" p="Integrated release ID" />
                      <I n="repair_deployment" p="Staged deployment ID" />
                      <I n="repair_signals" p="Passing production signal names" />
                      <S n="repair_outcome" a={["validated", "failed"]} />
                      <S n="repair_action" a={["none", "pause", "restore", "reopen"]} />
                      <I n="repair_summary" p="Observed behavior and causal validation" />
                      <div>
                        <Button>Validate delivered repair</Button>
                      </div>
                    </form>
                  )}
                </article>
              ))}
              {user && selected.replay_scenarios.some((x) => x.status === "reproduced") && selected.claims.some((x) => x.kind === "finding" && x.status === "supported") && (
                <form onSubmit={createRepair} className="mt-4 grid gap-2 sm:grid-cols-3">
                  <I n="repair_title" p="Repair title" />
                  <I n="repair_scenario" p="Reproduced scenario ID" />
                  <I n="repair_cause" p="Supported cause claim ID" />
                  <I n="repair_acceptance" p="Acceptance criteria, comma separated" />
                  <I n="repair_regression" p="Regression criteria, comma separated" />
                  <S n="repair_owner_type" a={["human", "agent"]} />
                  <I n="repair_owner" p="Human or approved-agent owner ID" />
                  <div>
                    <Button>Create governed repair</Button>
                  </div>
                </form>
              )}
            </section>
            {user && (
              <>
                <h3 className="mt-6 font-semibold">Request a governed probe</h3>
                <p className="mt-1 text-xs text-[var(--muted)]">Preview categories, privacy/security transformations, retention, sampling, cost, load, audience, and expiry. An affected environment owner must approve before collection.</p>
                <form onSubmit={requestProbe} className="mt-3 grid gap-2 sm:grid-cols-3">
                  <S n="probe_kind" a={["logs", "traces", "profile", "state_snapshot", "dynamic_diagnostic"]} />
                  <I n="purpose" p="Bounded diagnostic purpose" />
                  <I n="definition_path" p="Repository diagnostic definition path" req={false} />
                  <I n="probe_audience" p="Audience user IDs" />
                  <I n="categories" p="Data categories, comma separated" />
                  <S n="privacy" a={["hash_user_identifiers", "remove_user_identifiers", "remove_user_data"]} />
                  <S n="security" a={["detect_secrets", "redact_secrets", "drop_secret_bearing_records"]} />
                  <I n="retention" p="Retention hours (1–720)" type="number" />
                  <I n="sampling" p="Sampling percent (1–100)" type="number" />
                  <I n="cost" p="Maximum cost in cents" type="number" />
                  <I n="load" p="Maximum service load percent" type="number" />
                  <I n="probe_expiry" p="Probe expiry" type="datetime-local" />
                  <div>
                    <Button>Request probe</Button>
                  </div>
                </form>
              </>
            )}
            <h3 className="mt-6 font-semibold">Scoped probes</h3>
            {selected.probes.length === 0 && <p className="mt-2 text-sm text-[var(--muted)]">No probe is visible to this audience.</p>}
            {selected.probes.map((p) => (
              <div key={p.id} className="mt-3 rounded-lg border p-3 text-sm">
                <div className="flex flex-wrap gap-2">
                  <Badge>{p.kind.replaceAll("_", " ")}</Badge>
                  <Badge tone={p.status === "completed" ? "success" : p.status === "partial" || p.status === "overloaded" ? "warning" : "neutral"}>{p.status}</Badge>
                </div>
                <p className="mt-2 font-medium">{p.purpose}</p>
                <p className="mt-1 text-xs text-[var(--muted)]">
                  Audience {p.audience_user_ids.join(", ")} · expires {new Date(p.expires_at).toLocaleString()}
                </p>
                <p className="mt-1 text-xs">
                  Requested: {p.requested_policy.data_categories.join(", ")} · {p.requested_policy.sample_percent}% sample · {p.requested_policy.max_load_percent}% load · {p.requested_policy.max_cost_cents}¢ · {p.requested_policy.retention_hours}h retention
                </p>
                {p.decision_reason && <p className="mt-1 text-xs">Decision: {p.decision_reason}</p>}
                {p.status === "pending" && selected.owner_ids.includes(user?.id || "") && (
                  <form onSubmit={(e) => decideProbe(e, p)} className="mt-3 grid gap-2 sm:grid-cols-3">
                    <S n="decision" a={["approved", "denied"]} />
                    <I n="reason" p="Owner decision rationale" />
                    <I n="categories" p="Approved categories" />
                    <S n="privacy" a={["hash_user_identifiers", "remove_user_identifiers", "remove_user_data"]} />
                    <S n="security" a={["detect_secrets", "redact_secrets", "drop_secret_bearing_records"]} />
                    <I n="retention" p="Approved retention hours" type="number" />
                    <I n="sampling" p="Approved sampling percent" type="number" />
                    <I n="cost" p="Approved maximum cost cents" type="number" />
                    <I n="load" p="Approved maximum load percent" type="number" />
                    <I n="approval_expiry" p="Approved expiry" type="datetime-local" />
                    <div>
                      <Button>Record decision</Button>
                    </div>
                  </form>
                )}
                {p.status === "approved" && p.requested_by === user?.id && (
                  <form onSubmit={(e) => reportProbe(e, p)} className="mt-3 grid gap-2 sm:grid-cols-3">
                    <S n="outcome" a={["complete", "partial", "overloaded", "denied"]} />
                    <I n="started_at" p="Collection start" type="datetime-local" />
                    <I n="provenance" p="Collector/run provenance" />
                    <I n="transformations" p="Applied transformations" />
                    <I n="gaps" p="Gaps (required unless complete)" req={false} />
                    <S n="artifact_kind" a={["log", "trace", "profile", "snapshot", "diagnostic"]} />
                    <I n="artifact_reference" p="Sanitized artifact reference" req={false} />
                    <I n="artifact_digest" p="SHA-256 digest" req={false} />
                    <I n="artifact_size" p="Artifact bytes" type="number" req={false} />
                    <I n="redaction" p="Artifact redaction" req={false} />
                    <div>
                      <Button>Retain outcome</Button>
                    </div>
                  </form>
                )}
                {(p.status === "pending" || p.status === "approved") && selected.owner_ids.includes(user?.id || "") && (
                  <form onSubmit={(e) => revokeProbe(e, p)} className="mt-3 flex gap-2">
                    <I n="revoke_reason" p="Consent revocation reason" />
                    <Button>Revoke probe</Button>
                  </form>
                )}
                {p.actions.map((a) => (
                  <div key={a.id} className="mt-2 border-t pt-2 text-xs">
                    <p>
                      <strong>{a.outcome}</strong> · {a.provenance}
                      {a.gaps.length > 0 ? ` · gaps: ${a.gaps.join(", ")}` : " · no declared gaps"}
                    </p>
                    <p className="mt-1 text-[var(--muted)]">Transformations: {a.transformations.join(", ")}</p>
                    {a.artifacts.map((artifact) => (
                      <p key={`${a.id}-${artifact.digest}`} className="mt-1 text-[var(--muted)]">
                        {artifact.kind} · {artifact.size_bytes} bytes · redaction: {artifact.redaction} · digest <code>{artifact.digest.slice(0, 12)}</code>
                      </p>
                    ))}
                  </div>
                ))}
              </div>
            ))}
            {user && (
              <form onSubmit={event} className="mt-5 grid gap-2 sm:grid-cols-3">
                <S n="kind" a={["hypothesis", "status"]} />
                <I n="value" p="Hypothesis or status" />
                <I n="message" p="Attributable note" req={false} />
                <div>
                  <Button>Append history</Button>
                </div>
              </form>
            )}
            <h3 className="mt-5 font-semibold">Hypotheses and history</h3>
            {selected.hypotheses.map((x) => (
              <p key={x.id} className="mt-2 text-sm">
                <Badge>{x.status}</Badge> {x.statement} <span className="text-[var(--muted)]">— {x.created_by}</span>
              </p>
            ))}
            {[...selected.history].reverse().map((x) => (
              <p key={x.id} className="mt-2 border-t pt-2 text-xs text-[var(--muted)]">
                {x.kind} by {x.actor_id} · {new Date(x.created_at).toLocaleString()} {x.message}
              </p>
            ))}
          </Card>
        )}
      </div>
    </main>
  );
}
function I({ n, p, req = true, type = "text" }: { n: string; p: string; req?: boolean; type?: string }) {
  return <input className={field} name={n} type={type} required={req} placeholder={p} aria-label={p} />;
}
function S({ n, a }: { n: string; a: string[] }) {
  return (
    <select className={field} name={n} aria-label={n}>
      {a.map((x) => (
        <option key={x}>{x}</option>
      ))}
    </select>
  );
}
function D({ k, v }: { k: string; v: string }) {
  return (
    <div>
      <dt className="text-xs font-semibold uppercase text-[var(--muted)]">{k}</dt>
      <dd className="mt-1 break-all">{v}</dd>
    </div>
  );
}
