"use client";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Contract = {
  old_readers: string[];
  new_readers: string[];
  old_writers: string[];
  new_writers: string[];
  rollout_flags: string[];
  idempotency: string;
  transformations: string[];
  ownership: string[];
  rollback_assumptions: string[];
};
type Work = {
  id: string;
  kind: string;
  step_id: string;
  repository_id: string;
  proposal_id: string;
  task_id: string;
  dependency_ids: string[];
  contract: Contract;
  status: string;
  ready: boolean;
  assignee_type?: string;
  assignee_id?: string;
  session_id?: string;
  workspace_id?: string;
  pull_request_id?: string;
  contribution_status?: string;
};
type Rehearsal = {
  id: string;
  name: string;
  application_revision: string;
  dataset: {
    kind: string;
    description: string;
    privacy_method: string;
    digest: string;
    max_bytes: number;
    row_count: number;
    object_count: number;
  };
  checks: {
    id: string;
    kind: string;
    command: string;
    invariant: string;
    invariant_command: string;
  }[];
  runs: {
    id: string;
    result: string;
    workspace_id: string;
    created_by: string;
    outcomes: {
      check_id: string;
      status: string;
      duration_ms: number;
      sanitized_log: string;
      rows_before: number;
      rows_after: number;
      objects_before: number;
      objects_after: number;
      invariant_passed: boolean;
      artifact_digests: string[];
      cost_units: number;
    }[];
    attestations: string[];
  }[];
  notes: { id: string; run_id: string; body: string; actor_id: string }[];
};
type ExecutionPhase = {
  name: string;
  state: string;
  progress_percent: number;
  lag_seconds: number;
  invariants: string[];
  service_health: string;
  blockers: string[];
  next_actions: string[];
  cost_units: number;
};
type Execution = {
  id: string;
  version: number;
  active_revision: number;
  environment_id: string;
  release_id: string;
  deployment_id?: string;
  rehearsal_id: string;
  controller_id: string;
  status: string;
  current_phase: number;
  compatibility_window: string;
  privacy_constraints: string[];
  cost_budget_units: number;
  throttle_percent: number;
  abort_reversible_until: string;
  observation_period_seconds: number;
  phases: ExecutionPhase[];
  step_reports: {
    phase: string;
    step_id: string;
    agent_id: string;
    progress_percent: number;
    service_health: string;
    blockers: string[];
    summary: string;
  }[];
  failures: {
    id: string;
    kind: string;
    phase: string;
    safety_point: string;
    summary: string;
    evidence: string[];
  }[];
  recoveries: {
    id: string;
    kind: string;
    failure_id: string;
    summary: string;
    evidence: string[];
    recovery_attestation?: string;
    rollback_release_id?: string;
  }[];
  events: {
    kind: string;
    phase: string;
    summary: string;
    actor_id: string;
    agent_id?: string;
  }[];
};
type Migration = {
  id: string;
  from_version: number;
  to_version: number;
  source_kind: string;
  source_id: string;
  summary: string;
  operations: {
    id: string;
    kind: string;
    description: string;
    owner_ids: string[];
    consumer_ids: string[];
    destructive: boolean;
    rollback_limit: string;
  }[];
  steps: {
    id: string;
    description: string;
    success_measures: string[];
    required_approver_ids: string[];
  }[];
  rollback_limits: string[];
  version: number;
  events: {
    kind: string;
    summary: string;
    actor_id: string;
    created_at: string;
  }[];
  work: Work[];
  rehearsals: Rehearsal[];
  executions: Execution[];
  retirement_approvals: { owner_id: string; summary: string }[];
  completion?: {
    compatibility_removed: string[];
    obsolete_fields: string[];
    irreversible_decisions: string[];
    environments: {
      environment_id: string;
      current_version: number;
      retained_data: string[];
      changed_data: string[];
      verified_deletion: string[];
      exceptions: string[];
      cost_units: number;
    }[];
    approved_by: string[];
    completed_by: string;
  };
};
type Schema = {
  id: string;
  current_version: number;
  revisions: {
    version: number;
    name: string;
    store_kind: string;
    description: string;
    definition: string;
    definition_path: string;
    owner_ids: string[];
    compatibility: string[];
    retention: string;
    privacy: string[];
    links: { kind: string; id: string; label: string }[];
    pull_request_id: string;
    reviewed_commit: string;
    created_by: string;
    created_at: string;
  }[];
  migrations: Migration[];
};
const val = (f: FormData, n: string) => String(f.get(n) ?? "").trim(),
  list = (v: string) =>
    v
      .split(",")
      .map((x) => x.trim())
      .filter(Boolean);
export function DurableStateWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth(),
    [items, setItems] = useState<Schema[]>([]),
    [selected, setSelected] = useState<Schema>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const out = await api<{ schemas: Schema[] }>(
        `/repositories/${repositoryID}/durable-schemas`,
        {},
        token,
      );
      setItems(out.schemas);
      setSelected(
        (x) => out.schemas.find((s) => s.id === x?.id) ?? out.schemas[0],
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Durable state could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function publish(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    const f = new FormData(e.currentTarget);
    try {
      await api(
        `/repositories/${repositoryID}/durable-schemas`,
        {
          method: "POST",
          body: JSON.stringify({
            revision: {
              name: val(f, "name"),
              store_kind: val(f, "kind"),
              description: val(f, "description"),
              definition: val(f, "definition"),
              definition_path: val(f, "definition_path"),
              owner_ids: list(val(f, "owners")),
              compatibility: list(val(f, "compatibility")),
              retention: val(f, "retention"),
              privacy: list(val(f, "privacy")),
              links: [
                ...(val(f, "service")
                  ? [
                      {
                        kind: "service",
                        id: val(f, "service"),
                        label: "Service",
                      },
                    ]
                  : []),
                ...(val(f, "environment")
                  ? [
                      {
                        kind: "environment",
                        id: val(f, "environment"),
                        label: "Environment",
                      },
                    ]
                  : []),
              ],
              pull_request_id: val(f, "pull"),
              reviewed_commit: val(f, "commit"),
              rationale: val(f, "rationale"),
            },
          }),
        },
        token,
      );
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Schema could not be published.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function migrate(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected || !user) return;
    setBusy(true);
    const f = new FormData(e.currentTarget),
      op = val(f, "operation_id"),
      step = val(f, "step_id"),
      owners = list(val(f, "owners")),
      consumers = list(val(f, "consumers")),
      kind = val(f, "operation_kind");
    try {
      await api(
        `/repositories/${repositoryID}/durable-schemas/${selected.id}/migrations`,
        {
          method: "POST",
          body: JSON.stringify({
            from_version: Number(val(f, "from")),
            to_version: Number(val(f, "to")),
            source_kind: val(f, "source_kind"),
            source_id: val(f, "source_id"),
            summary: val(f, "summary"),
            operations: [
              {
                id: op,
                kind,
                description: val(f, "operation"),
                owner_ids: owners,
                consumer_ids: consumers,
                destructive: kind === "destructive",
                rollback_limit: val(f, "operation_rollback"),
              },
            ],
            steps: [
              {
                id: step,
                operation_ids: [op],
                description: val(f, "step"),
                success_measures: list(val(f, "measures")),
                required_approver_ids: list(val(f, "approvers")),
              },
            ],
            rollback_limits: list(val(f, "rollback")),
          }),
        },
        token,
      );
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Migration plan could not be opened.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function createWork(
    e: FormEvent<HTMLFormElement>,
    migration: Migration,
  ) {
    e.preventDefault();
    if (!token || !selected) return;
    setBusy(true);
    const f = new FormData(e.currentTarget);
    try {
      await api(
        `/repositories/${repositoryID}/durable-schemas/${selected.id}/migrations/${migration.id}/work`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: migration.version,
            kind: val(f, "work_kind"),
            step_id: val(f, "work_step"),
            repository_id: val(f, "work_repository"),
            title: val(f, "work_title"),
            completion_criteria: val(f, "criteria"),
            dependency_ids: list(val(f, "dependencies")),
            assignee_type: val(f, "assignee_type"),
            assignee_id: val(f, "assignee_id"),
            mandate: val(f, "mandate"),
            base_revision: val(f, "base_revision"),
            contract: {
              old_readers: list(val(f, "old_readers")),
              new_readers: list(val(f, "new_readers")),
              old_writers: list(val(f, "old_writers")),
              new_writers: list(val(f, "new_writers")),
              rollout_flags: list(val(f, "rollout_flags")),
              idempotency: val(f, "idempotency"),
              transformations: list(val(f, "transformations")),
              ownership: list(val(f, "contract_ownership")),
              rollback_assumptions: list(val(f, "rollback_assumptions")),
            },
          }),
        },
        token,
      );
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Migration work could not be created.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function rehearse(e: FormEvent<HTMLFormElement>, migration: Migration) {
    e.preventDefault();
    if (!token || !selected) return;
    setBusy(true);
    const f = new FormData(e.currentTarget);
    try {
      const parsed: unknown = JSON.parse(val(f, "checks_json"));
      if (
        !Array.isArray(parsed) ||
        !parsed.length ||
        parsed.some(
          (x) =>
            !x ||
            typeof x !== "object" ||
            !("id" in x) ||
            !("kind" in x) ||
            !("command" in x) ||
            !("invariant" in x) ||
            !("invariant_command" in x),
        )
      )
        throw new Error(
          "Checks must be a non-empty JSON array with id, kind, command, invariant, and invariant_command fields.",
        );
      const checks = parsed.map((x) => ({
        ...(x as {
          id: string;
          kind: string;
          command: string;
          invariant: string;
          invariant_command: string;
        }),
        revision_inputs: [
          "application",
          "schema_from",
          "schema_to",
          "migration",
          "data_shape",
        ],
      }));
      await api(
        `/repositories/${repositoryID}/durable-schemas/${selected.id}/migrations/${migration.id}/rehearsals`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: migration.version,
            rehearsal: {
              name: val(f, "rehearsal_name"),
              application_revision: val(f, "application_revision"),
              dataset: {
                kind: val(f, "dataset_kind"),
                description: val(f, "dataset_description"),
                privacy_method: val(f, "privacy_method"),
                digest: val(f, "dataset_digest"),
                max_bytes: Number(val(f, "max_bytes")),
                row_count: Number(val(f, "row_count")),
                object_count: Number(val(f, "object_count")),
              },
              dependencies: [],
              checks,
            },
          }),
        },
        token,
      );
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Rehearsal could not be assembled.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function openExecution(
    e: FormEvent<HTMLFormElement>,
    migration: Migration,
  ) {
    e.preventDefault();
    if (!token || !selected) return;
    setBusy(true);
    const f = new FormData(e.currentTarget);
    try {
      const agent = val(f, "agent_id");
      await api(
        `/repositories/${repositoryID}/durable-schemas/${selected.id}/migrations/${migration.id}/executions`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: migration.version,
            execution: {
              environment_id: val(f, "execution_environment"),
              release_id: val(f, "execution_release"),
              rehearsal_id: val(f, "execution_rehearsal"),
              compatibility_window: val(f, "compatibility_window"),
              observation_period_seconds: Number(
                val(f, "observation_period"),
              ),
              privacy_constraints: list(val(f, "execution_privacy")),
              cost_budget_units: Number(val(f, "cost_budget")),
              abort_reversible_until: val(f, "abort_until"),
              delegations: agent
                ? [
                    {
                      phase: val(f, "agent_phase"),
                      agent_id: agent,
                      step_id: val(f, "agent_step"),
                      mandate: val(f, "agent_mandate"),
                    },
                  ]
                : [],
            },
          }),
        },
        token,
      );
      await load();
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Production execution could not be opened.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function control(
    migration: Migration,
    execution: Execution,
    action: string,
    extra: Record<string, unknown> = {},
  ) {
    if (!token || !selected) return;
    setBusy(true);
    try {
      await api(
        `/repositories/${repositoryID}/durable-schemas/${selected.id}/migrations/${migration.id}/executions/${execution.id}/controls`,
        {
          method: "POST",
          body: JSON.stringify({
            action,
            expected_version: execution.version,
            ...extra,
          }),
        },
        token,
      );
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Execution control was rejected.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function report(
    e: FormEvent<HTMLFormElement>,
    migration: Migration,
    execution: Execution,
  ) {
    e.preventDefault();
    const f = new FormData(e.currentTarget),
      phase = execution.phases[execution.current_phase];
    await control(migration, execution, "report", {
      phase: phase.name,
      progress_percent: Number(val(f, "progress")),
      lag_seconds: Number(val(f, "lag")),
      invariants: list(val(f, "invariants")),
      service_health: val(f, "health"),
      blockers: list(val(f, "blockers")),
      next_actions: list(val(f, "next_actions")),
      cost_units: Number(val(f, "execution_cost")),
      deployment_id: val(f, "deployment_id"),
      summary: val(f, "report_summary"),
    });
  }
  return (
    <main
      id="main-content"
      className="mx-auto w-full max-w-6xl space-y-6 px-6 py-8"
    >
      <div>
        <p className="text-sm font-semibold text-[var(--brand)]">
          Repository governance
        </p>
        <h1 className="text-3xl font-bold">Durable state</h1>
        <p className="mt-2 max-w-3xl text-[var(--muted)]">
          Make authoritative state changes visible and steerable while old and
          new software safely coexist.
        </p>
      </div>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      <div className="grid gap-6 lg:grid-cols-[.8fr_1.2fr]">
        <Card className="p-5">
          <h2 className="font-bold">Published schemas</h2>
          <div className="mt-4 space-y-2">
            {items.map((s) => {
              const r = s.revisions.at(-1)!;
              return (
                <button
                  key={s.id}
                  onClick={() => setSelected(s)}
                  className="w-full rounded-lg border border-[var(--line)] p-3 text-left"
                >
                  <span className="font-semibold">{r.name}</span>
                  <span className="ml-2">
                    <Badge tone="info">
                      v{s.current_version} {r.store_kind}
                    </Badge>
                  </span>
                  <p className="mt-1 text-sm text-[var(--muted)]">
                    {r.description}
                  </p>
                </button>
              );
            })}
            {!items.length && (
              <p className="text-sm text-[var(--muted)]">
                No reviewed schemas published yet.
              </p>
            )}
          </div>
        </Card>
        <Card className="p-5">
          <h2 className="font-bold">Publish reviewed schema</h2>
          <form onSubmit={publish} className="mt-4 grid gap-3 sm:grid-cols-2">
            {[
              ["name", "Schema name"],
              ["kind", "Store kind"],
              ["owners", "Owner IDs"],
              ["pull", "Merged pull ID"],
              ["commit", "Exact merge commit"],
              ["definition_path", "Definition path"],
              ["retention", "Retention"],
              ["compatibility", "Compatibility"],
              ["privacy", "Privacy constraints"],
              ["rationale", "Rationale"],
            ].map(([n, p]) => (
              <input
                key={n}
                name={n}
                required
                placeholder={p}
                className="rounded-lg border border-[var(--line)] px-3 py-2 text-sm"
              />
            ))}
            <input
              name="service"
              placeholder="Service ID (optional)"
              className="rounded-lg border border-[var(--line)] px-3 py-2 text-sm"
            />
            <input
              name="environment"
              placeholder="Environment ID (optional)"
              className="rounded-lg border border-[var(--line)] px-3 py-2 text-sm"
            />
            <textarea
              name="description"
              required
              placeholder="Purpose and stored state"
              className="rounded-lg border border-[var(--line)] px-3 py-2 text-sm"
            />
            <textarea
              name="definition"
              required
              placeholder="Exact reviewed definition"
              className="rounded-lg border border-[var(--line)] px-3 py-2 font-mono text-sm"
            />
            <Button disabled={busy} className="sm:col-span-2">
              Publish immutable version
            </Button>
          </form>
        </Card>
      </div>
      {selected && (
        <Card className="p-5">
          <h2 className="font-bold">
            Migration collaboration · {selected.revisions.at(-1)?.name}
          </h2>
          <details className="mt-4">
            <summary className="cursor-pointer font-semibold">
              Open migration plan
            </summary>
            <form onSubmit={migrate} className="mt-3 grid gap-3 sm:grid-cols-3">
              {[
                ["from", "From version"],
                ["to", "To version"],
                ["source_kind", "pull_request or decision"],
                ["source_id", "Source ID"],
                ["summary", "Plan summary"],
                ["operation_id", "Operation ID"],
                ["operation_kind", "read, write, backfill, destructive"],
                ["operation", "Operation description"],
                ["owners", "Owner IDs"],
                ["consumers", "Consumer IDs"],
                ["operation_rollback", "Operation rollback limit"],
                ["step_id", "Step ID"],
                ["step", "Step description"],
                ["measures", "Success measures"],
                ["approvers", "Required approver IDs"],
                ["rollback", "Overall rollback limits"],
              ].map(([n, p]) => (
                <input
                  key={n}
                  name={n}
                  required
                  placeholder={p}
                  className="rounded-lg border border-[var(--line)] px-3 py-2 text-sm"
                />
              ))}
              <Button disabled={busy} className="sm:col-span-3">
                Open attributable plan
              </Button>
            </form>
          </details>
          <div className="mt-6 space-y-5">
            {selected.migrations.map((m) => (
              <section
                key={m.id}
                className="rounded-lg border border-[var(--line)] p-4"
              >
                <div className="flex flex-wrap gap-2">
                  <Badge
                    tone={
                      m.operations.some((o) => o.destructive)
                        ? "danger"
                        : "warning"
                    }
                  >
                    v{m.from_version} → v{m.to_version}
                  </Badge>
                  <Badge>
                    {m.source_kind} {m.source_id}
                  </Badge>
                </div>
                <p className="mt-2 font-semibold">{m.summary}</p>
                <p className="text-sm text-[var(--muted)]">
                  Rollback: {m.rollback_limits.join("; ")} · {m.events.length}{" "}
                  history events
                </p>
                <div className="mt-3 flex flex-wrap gap-2">
                  {m.work.map((w) => (
                    <Badge key={w.id} tone={w.ready ? "success" : "warning"}>
                      {w.kind} · {w.status}
                    </Badge>
                  ))}
                  {m.rehearsals.map((r) => (
                    <Badge
                      key={r.id}
                      tone={
                        r.runs.at(-1)?.result === "passed" ? "success" : "info"
                      }
                    >
                      {r.name} · {r.runs.at(-1)?.result ?? "not run"}
                    </Badge>
                  ))}
                </div>
                {(m.executions ?? []).map((x) => {
                  const phase = x.phases[x.current_phase];
                  return (
                    <div
                      key={x.id}
                      className="mt-4 rounded-lg bg-[var(--surface-subtle)] p-4"
                    >
                      <div className="flex flex-wrap gap-2">
                        <Badge
                          tone={
                            x.status === "running"
                              ? "success"
                              : x.status === "aborted"
                                ? "danger"
                                : "warning"
                          }
                        >
                          {x.status} · {phase.name}
                        </Badge>
                        <Badge>active revision {x.active_revision}</Badge>
                        <Badge>{x.throttle_percent}% throttle</Badge>
                      </div>
                      <p className="mt-2 text-sm">
                        Controller {x.controller_id} · environment{" "}
                        {x.environment_id} · release {x.release_id}
                      </p>
                      <p className="text-xs text-[var(--muted)]">
                        Compatibility: {x.compatibility_window} · Privacy:{" "}
                        {x.privacy_constraints.join("; ")} · Reversible{" "}
                        {x.abort_reversible_until} · Observe for{" "}
                        {x.observation_period_seconds}s
                      </p>
                      <div className="mt-3 grid gap-2 sm:grid-cols-5">
                        {x.phases.map((p) => (
                          <div
                            key={p.name}
                            className="rounded border border-[var(--line)] p-2 text-xs"
                          >
                            <p className="font-semibold">
                              {p.name} · {p.state}
                            </p>
                            <p>
                              {p.progress_percent}% · lag {p.lag_seconds}s
                            </p>
                            <p>{p.service_health || "health pending"}</p>
                          </div>
                        ))}
                      </div>
                      <p className="mt-3 text-sm">
                        Invariants: {phase.invariants.join("; ") || "pending"} ·
                        Cost {x.phases.reduce((n, p) => n + p.cost_units, 0)}/
                        {x.cost_budget_units}
                      </p>
                      {x.step_reports
                        ?.filter((report) => report.phase === phase.name)
                        .map((report) => (
                          <p
                            key={`${report.agent_id}:${report.step_id}:${report.summary}`}
                            className="mt-1 text-xs text-[var(--muted)]"
                          >
                            Agent {report.agent_id} · step {report.step_id} ·{" "}
                            {report.progress_percent}% ·{" "}
                            {report.service_health || "health pending"}:{" "}
                            {report.summary}
                          </p>
                        ))}
                      {phase.blockers.length > 0 && (
                        <p className="text-sm text-[var(--danger)]">
                          Blockers: {phase.blockers.join("; ")}
                        </p>
                      )}
                      {x.failures?.map((failure) => (
                        <p
                          key={failure.id}
                          className="mt-2 text-sm text-[var(--danger)]"
                        >
                          Contained {failure.kind} in {failure.phase} at{" "}
                          {failure.safety_point}: {failure.summary}
                        </p>
                      ))}
                      {x.recoveries?.map((recovery) => (
                        <p key={recovery.id} className="mt-1 text-sm">
                          Recovery {recovery.kind}: {recovery.summary}
                        </p>
                      ))}
                      <p className="text-sm">
                        Next:{" "}
                        {phase.next_actions.join("; ") || "await evidence"}
                      </p>
                      <div className="mt-3 flex flex-wrap gap-2">
                        {x.status === "ready" && (
                          <Button
                            disabled={busy}
                            onClick={() => control(m, x, "start")}
                          >
                            Start phase
                          </Button>
                        )}
                        {x.status === "running" && (
                          <>
                            <Button
                              disabled={busy}
                              onClick={() =>
                                control(m, x, "pause", {
                                  summary: "operator paused for review",
                                })
                              }
                            >
                              Pause
                            </Button>
                            <Button
                              disabled={busy}
                              onClick={() =>
                                control(m, x, "throttle", {
                                  throttle_percent: 25,
                                  summary: "operator throttled to 25%",
                                })
                              }
                            >
                              Throttle to 25%
                            </Button>
                            <Button
                              disabled={busy}
                              onClick={() =>
                                control(m, x, "advance", {
                                  summary: "current evidence accepted",
                                })
                              }
                            >
                              Advance phase
                            </Button>
                          </>
                        )}
                        {x.status === "paused" && (
                          <Button
                            disabled={busy}
                            onClick={() =>
                              control(m, x, "resume", {
                                summary: "blocker cleared",
                              })
                            }
                          >
                            Resume
                          </Button>
                        )}
                        {!["completed", "aborted"].includes(x.status) &&
                          phase.name !== "contract" && (
                            <Button
                              disabled={busy}
                              onClick={() =>
                                control(m, x, "abort", {
                                  summary:
                                    "operator requested reversible abort",
                                })
                              }
                            >
                              Abort reversible work
                            </Button>
                          )}
                      </div>
                      {x.status === "running" && (
                        <form
                          onSubmit={(e) => report(e, m, x)}
                          className="mt-3 grid gap-2 sm:grid-cols-3"
                        >
                          {[
                            ["progress", "Progress percent"],
                            ["lag", "Lag seconds"],
                            ["health", "Service health"],
                            ["invariants", "Current invariants"],
                            ["blockers", "Blockers (optional)"],
                            ["next_actions", "Next actions"],
                            ["execution_cost", "Cumulative phase cost"],
                            [
                              "deployment_id",
                              "Successful deployment ID (optional)",
                            ],
                            ["report_summary", "Attributable update"],
                          ].map(([n, p]) => (
                            <input
                              key={n}
                              name={n}
                              required={
                                !["blockers", "deployment_id"].includes(n)
                              }
                              placeholder={p}
                              className="rounded-lg border border-[var(--line)] px-3 py-2 text-sm"
                            />
                          ))}
                          <Button disabled={busy} className="sm:col-span-3">
                            Publish live evidence
                          </Button>
                        </form>
                      )}
                    </div>
                  );
                })}
                {m.completion && (
                  <div className="mt-4 rounded-lg border border-[var(--line)] p-4">
                    <div className="flex flex-wrap gap-2">
                      <Badge tone="success">Migration cleanup verified</Badge>
                      <Badge>
                        {m.retirement_approvals.length} owner approval
                        {m.retirement_approvals.length === 1 ? "" : "s"}
                      </Badge>
                    </div>
                    <p className="mt-2 text-sm">
                      Compatibility removed:{" "}
                      {m.completion.compatibility_removed.join("; ")} · Obsolete
                      fields: {m.completion.obsolete_fields.join("; ")}
                    </p>
                    {m.completion.environments.map((environment) => (
                      <p
                        key={environment.environment_id}
                        className="mt-1 text-sm text-[var(--muted)]"
                      >
                        Environment {environment.environment_id} now uses schema
                        v{environment.current_version}; deletion verified:{" "}
                        {environment.verified_deletion.join("; ")}; cost{" "}
                        {environment.cost_units}
                      </p>
                    ))}
                  </div>
                )}
                <details className="mt-4">
                  <summary className="cursor-pointer font-semibold">
                    Open production execution
                  </summary>
                  <p className="mt-2 text-sm text-[var(--muted)]">
                    Requires all owner approvals and passing rehearsal evidence.
                    No credentials or operational authority cross into this
                    record.
                  </p>
                  <form
                    onSubmit={(e) => openExecution(e, m)}
                    className="mt-3 grid gap-3 sm:grid-cols-2"
                  >
                    {[
                      ["execution_environment", "Established environment ID"],
                      ["execution_release", "Exact release ID"],
                      ["execution_rehearsal", "Passing rehearsal ID"],
                      ["compatibility_window", "Old/new compatibility window"],
                      ["execution_privacy", "Privacy constraints"],
                      ["cost_budget", "Cost budget units"],
                      ["abort_until", "Reversible until"],
                      ["observation_period", "Observation period seconds"],
                      ["agent_id", "Delegated agent ID (optional)"],
                      ["agent_phase", "Agent phase (optional)"],
                      ["agent_step", "Agent step (optional)"],
                      ["agent_mandate", "Agent mandate (optional)"],
                    ].map(([n, p]) => (
                      <input
                        key={n}
                        name={n}
                        required={!n.startsWith("agent_")}
                        placeholder={p}
                        className="rounded-lg border border-[var(--line)] px-3 py-2 text-sm"
                      />
                    ))}
                    <Button disabled={busy} className="sm:col-span-2">
                      Open governed execution
                    </Button>
                  </form>
                </details>
                <details className="mt-3">
                  <summary className="cursor-pointer font-semibold">
                    Create repository work
                  </summary>
                  <form
                    onSubmit={(e) => createWork(e, m)}
                    className="mt-3 grid gap-2 sm:grid-cols-2"
                  >
                    {[
                      ["work_kind", "Work kind"],
                      ["work_step", "Migration step"],
                      ["work_repository", "Target repository"],
                      ["work_title", "Task title"],
                      ["criteria", "Completion criteria"],
                      ["dependencies", "Earlier work IDs"],
                      ["assignee_type", "human or agent"],
                      ["assignee_id", "Assignee ID"],
                      ["mandate", "Bounded mandate"],
                      ["base_revision", "Exact base"],
                      ["old_readers", "Old readers"],
                      ["new_readers", "New readers"],
                      ["old_writers", "Old writers"],
                      ["new_writers", "New writers"],
                      ["rollout_flags", "Rollout flags"],
                      ["idempotency", "Idempotency"],
                      ["transformations", "Transformations"],
                      ["contract_ownership", "Ownership"],
                      ["rollback_assumptions", "Rollback assumptions"],
                    ].map(([n, p]) => (
                      <input
                        key={n}
                        name={n}
                        required={n !== "dependencies"}
                        placeholder={p}
                        className="rounded-lg border border-[var(--line)] px-3 py-2 text-sm"
                      />
                    ))}
                    <Button disabled={busy} className="sm:col-span-2">
                      Create governed task
                    </Button>
                  </form>
                </details>
                <details className="mt-3">
                  <summary className="cursor-pointer font-semibold">
                    Assemble rehearsal
                  </summary>
                  <form
                    onSubmit={(e) => rehearse(e, m)}
                    className="mt-3 grid gap-2 sm:grid-cols-2"
                  >
                    {[
                      ["rehearsal_name", "Name"],
                      ["application_revision", "Exact application commit"],
                      ["dataset_kind", "synthetic or representative"],
                      ["dataset_description", "Dataset shape"],
                      ["privacy_method", "Privacy method"],
                      ["dataset_digest", "Dataset digest"],
                      ["max_bytes", "Maximum bytes"],
                      ["row_count", "Rows"],
                      ["object_count", "Objects"],
                    ].map(([n, p]) => (
                      <input
                        key={n}
                        name={n}
                        required
                        placeholder={p}
                        className="rounded-lg border border-[var(--line)] px-3 py-2 text-sm"
                      />
                    ))}
                    <textarea
                      name="checks_json"
                      required
                      className="rounded-lg border border-[var(--line)] px-3 py-2 font-mono text-sm sm:col-span-2"
                      placeholder='[{"id":"upgrade","kind":"upgrade","command":"./up","invariant":"safe","invariant_command":"./verify"}]'
                    />
                    <Button disabled={busy} className="sm:col-span-2">
                      Freeze rehearsal
                    </Button>
                  </form>
                </details>
              </section>
            ))}
          </div>
        </Card>
      )}
    </main>
  );
}
