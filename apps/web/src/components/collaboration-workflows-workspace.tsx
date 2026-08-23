"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "@/components/auth";
import { Badge, Button, Card } from "@/components/ui";

type Authority = {
  step_id: string;
  principal: string;
  grants: string[];
  boundary: string;
};
type Definition = {
  name: string;
  outcome: string;
  description: string;
  owner_ids: string[];
  triggers: { id: string; kind: string; event: string }[];
  steps: {
    id: string;
    name: string;
    needs?: string[];
    owner_ids: string[];
    retries: number;
    timeout_seconds: number;
    budget_actions: number;
    completion: string[];
    optional?: boolean;
    manual?: boolean;
    requested_inputs?: string[];
    approval?: string;
    invocation: {
      kind: string;
      action?: string;
      component?: string;
      agent_id?: string;
      workflow_id?: string;
    };
  }[];
  outputs: string[];
  completion: string[];
  budget_actions: number;
};
type Preview = {
  definition: Definition;
  source: { revision: string; path: string; sha256: string };
  subscriptions: string[];
  effective_authority: Authority[];
  execution_order: string[][];
  diagnostics: {
    kind: string;
    message: string;
    attributed_to: string;
    step_id?: string;
  }[];
  activatable: boolean;
};
type Workflow = {
  id: string;
  current_version: number;
  status: string;
  revisions: {
    version: number;
    definition: Definition;
    source: { revision: string; path: string; sha256: string };
    subscriptions: string[];
    effective_authority: Authority[];
    execution_order: string[][];
    activated_by: string;
    activated_at: string;
  }[];
};
type Attempt = {
  number: number;
  status: string;
  started_at: string;
  finished_at?: string;
  inputs: Record<string, unknown>;
  outputs?: Record<string, unknown>;
  logs: { time: string; level: string; message: string }[];
  artifacts: {
    name: string;
    kind: string;
    sha256: string;
    size: number;
    restricted?: boolean;
  }[];
  agent_session?: {
    id: string;
    agent_id: string;
    status: string;
    url?: string;
  };
  cost_units: number;
  actions_used: number;
  failure_code?: string;
  provenance: string[];
};
type StepRun = {
  step_id: string;
  status: string;
  attempt: number;
  started_at?: string;
  finished_at?: string;
  outputs?: Record<string, unknown>;
  actions_used: number;
  failure_code?: string;
  attempts: Attempt[];
  provided_inputs?: Record<string, unknown>;
  taken_over_by?: string;
};
type Execution = {
  id: string;
  workflow_id: string;
  workflow_version: number;
  status: string;
  version: number;
  budget_actions: number;
  actions_used: number;
  cost_units: number;
  steps: StepRun[];
  created_at: string;
  updated_at: string;
  finished_at?: string;
  trigger: {
    id: string;
    name: string;
    actor_id: string;
    occurred_at: string;
    inputs: Record<string, unknown>;
    resource_revisions: Record<string, string>;
  };
  interventions: {
    id: string;
    kind: string;
    step_id?: string;
    actor_id: string;
    reason: string;
    created_at: string;
    version: number;
  }[];
  predicted_next_actions: string[];
  paused_by?: string;
  approval_requests: {
    step_id: string;
    action_class: string;
    owner_ids: string[];
    requested_at: string;
    expires_at: string;
    approved_by?: string;
  }[];
  action_receipts: {
    id: string;
    step_id: string;
    action_class: string;
    actions: number;
    cost_units: number;
    completion_sha256: string;
    created_at: string;
  }[];
};

const short = (v: string) => v.slice(0, 8);
export function CollaborationWorkflowsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, loading } = useAuth(),
    [workflows, setWorkflows] = useState<Workflow[]>([]),
    [preview, setPreview] = useState<Preview>(),
    [selected, setSelected] = useState<Workflow>(),
    [executions, setExecutions] = useState<Execution[]>([]),
    [selectedExecution, setSelectedExecution] = useState<Execution>(),
    [input, setInput] = useState({ step: "", name: "", value: "" }),
    [source, setSource] = useState({
      revision: "",
      path: ".vivarium/workflow.json",
    }),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    if (loading || !token) return;
    try {
      const x = await api<{ workflows: Workflow[] }>(
        `/repositories/${repositoryID}/collaboration-workflows`,
        {},
        token,
      );
      setWorkflows(x.workflows);
      setSelected(
        (old) => x.workflows.find((w) => w.id === old?.id) ?? x.workflows[0],
      );
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Workflows could not be loaded",
      );
    }
  }, [loading, repositoryID, token]);
  useEffect(() => {
    if (loading || !token) return;
    let cancelled = false;
    void api<{ workflows: Workflow[] }>(
      `/repositories/${repositoryID}/collaboration-workflows`,
      {},
      token,
    )
      .then((x) => {
        if (!cancelled) {
          setWorkflows(x.workflows);
          setSelected(x.workflows[0]);
        }
      })
      .catch((e) => {
        if (!cancelled)
          setError(
            e instanceof Error ? e.message : "Workflows could not be loaded",
          );
      });
    return () => {
      cancelled = true;
    };
  }, [loading, repositoryID, token]);
  useEffect(() => {
    if (!token || !selected) return;
    let cancelled = false;
    const refresh = () =>
      api<{ executions: Execution[] }>(
        `/repositories/${repositoryID}/collaboration-workflows/${selected.id}/executions`,
        {},
        token,
      )
        .then((x) => {
          if (!cancelled) {
            setExecutions(x.executions);
            setSelectedExecution(
              (old) =>
                x.executions.find((e) => e.id === old?.id) ?? x.executions[0],
            );
          }
        })
        .catch((e) => {
          if (!cancelled)
            setError(
              e instanceof Error ? e.message : "Runs could not be loaded",
            );
        });
    void refresh();
    const timer = setInterval(() => void refresh(), 5000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [repositoryID, selected, token]);
  async function inspect(e: FormEvent) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    setError("");
    try {
      setPreview(
        await api<Preview>(
          `/repositories/${repositoryID}/collaboration-workflows/preview`,
          { method: "POST", body: JSON.stringify(source) },
          token,
        ),
      );
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Configuration could not be previewed",
      );
    } finally {
      setBusy(false);
    }
  }
  async function activate() {
    if (!token || !preview?.activatable) return;
    setBusy(true);
    try {
      const path = selected
        ? `/repositories/${repositoryID}/collaboration-workflows/${selected.id}/revisions`
        : `/repositories/${repositoryID}/collaboration-workflows`;
      await api(
        path,
        {
          method: "POST",
          body: JSON.stringify({
            ...source,
            activation_id: preview.source.sha256,
            expected_version: selected?.current_version ?? 0,
          }),
        },
        token,
      );
      setPreview(undefined);
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Workflow could not be activated",
      );
    } finally {
      setBusy(false);
    }
  }
  async function intervene(
    kind: string,
    step_id = "",
    extra: Record<string, unknown> = {},
  ) {
    if (
      !token ||
      !selected ||
      !selectedExecution ||
      selectedExecution.workflow_id !== selected.id
    )
      return;
    setBusy(true);
    setError("");
    try {
      const out = await api<Execution>(
        `/repositories/${repositoryID}/collaboration-workflows/${selected.id}/executions/${selectedExecution.id}/interventions`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selectedExecution.version,
            kind,
            step_id,
            reason: `Collaborator requested ${kind.replace("_", " ")}`,
            ...extra,
          }),
        },
        token,
      );
      setSelectedExecution(out);
      setExecutions((xs) => xs.map((x) => (x.id === out.id ? out : x)));
      if (kind === "provide_input") setInput({ step: "", name: "", value: "" });
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "The run changed or policy rejected this intervention",
      );
    } finally {
      setBusy(false);
    }
  }
  async function control(kind: "disable" | "anomaly_stop" | "rollback") {
    if (!token || !selected) return;
    setBusy(true);
    setError("");
    try {
      await api(
        `/repositories/${repositoryID}/collaboration-workflows/${selected.id}/control`,
        {
          method: "POST",
          body: JSON.stringify({
            kind,
            rollback_version:
              kind === "rollback" ? selected.current_version - 1 : 0,
          }),
        },
        token,
      );
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Workflow control failed");
    } finally {
      setBusy(false);
    }
  }
  const revision = selected?.revisions.at(-1);
  const executionRevision =
    selectedExecution && selectedExecution.workflow_id === selected?.id
      ? selected.revisions.find(
          (candidate) =>
            candidate.version === selectedExecution.workflow_version,
        )
      : undefined;
  return (
    <main className="mx-auto max-w-6xl space-y-6 p-6">
      <header>
        <p className="text-sm font-semibold text-[var(--brand)]">
          Repository collaboration
        </p>
        <h1 className="mt-1 text-3xl font-semibold">Collaboration workflows</h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Explain what will run, why, with whose authority, and toward which
          shared outcome before repository activity can trigger it.
        </p>
      </header>
      <Card className="p-5">
        <form
          onSubmit={inspect}
          className="grid gap-4 md:grid-cols-[1fr_1fr_auto]"
        >
          <label className="text-sm font-medium">
            Exact reviewed commit
            <input
              required
              pattern="[0-9a-f]{40}"
              value={source.revision}
              onChange={(e) =>
                setSource({ ...source, revision: e.target.value })
              }
              className="mt-1 w-full rounded-lg border bg-transparent p-2.5"
              placeholder="40-character commit"
            />
          </label>
          <label className="text-sm font-medium">
            Workflow configuration path
            <input
              required
              value={source.path}
              onChange={(e) => setSource({ ...source, path: e.target.value })}
              className="mt-1 w-full rounded-lg border bg-transparent p-2.5"
            />
          </label>
          <Button disabled={busy} className="self-end">
            {busy ? "Resolving…" : "Preview configuration"}
          </Button>
        </form>
        {error && (
          <p role="alert" className="mt-3 text-sm text-[var(--danger)]">
            {error}
          </p>
        )}
      </Card>
      {preview && (
        <Card className="space-y-5 p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-xl font-semibold">
                {preview.definition.name}
              </h2>
              <p className="mt-1 text-sm text-[var(--muted)]">
                Outcome: {preview.definition.outcome}
              </p>
              <p className="mt-1 text-xs text-[var(--muted)]">
                {preview.source.path} · {short(preview.source.revision)} ·
                SHA-256 {short(preview.source.sha256)}
              </p>
            </div>
            <Badge tone={preview.activatable ? "success" : "danger"}>
              {preview.activatable ? "Ready to activate" : "Activation blocked"}
            </Badge>
          </div>
          <section>
            <h3 className="font-semibold">Event subscriptions</h3>
            <div className="mt-2 flex flex-wrap gap-2">
              {preview.subscriptions.map((x) => (
                <Badge key={x}>{x}</Badge>
              ))}
            </div>
          </section>
          <section>
            <h3 className="font-semibold">Execution graph</h3>
            <ol className="mt-2 space-y-2">
              {preview.execution_order.map((level, i) => (
                <li key={i} className="rounded-lg border p-3 text-sm">
                  <span className="font-medium">Stage {i + 1}</span> ·{" "}
                  {level.join(" in parallel · ")}
                </li>
              ))}
            </ol>
          </section>
          <section>
            <h3 className="font-semibold">Effective authority</h3>
            <div className="mt-2 grid gap-2 md:grid-cols-2">
              {preview.effective_authority.map((a) => (
                <div key={a.step_id} className="rounded-lg border p-3 text-sm">
                  <p className="font-medium">
                    {a.step_id} · {a.principal}
                  </p>
                  <p>{a.grants.join(", ")}</p>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    {a.boundary}
                  </p>
                </div>
              ))}
            </div>
          </section>
          <section className="grid gap-3 md:grid-cols-2">
            <div className="rounded-lg border p-3 text-sm">
              <h3 className="font-semibold">Simulated event cases</h3>
              {preview.definition.triggers.map((trigger) => (
                <p className="mt-2" key={trigger.id}>
                  <strong>{trigger.kind}:{trigger.event}</strong> → {preview.definition.steps.map((step) => step.invocation.action ?? step.invocation.kind).join(" · ")}
                </p>
              ))}
            </div>
            <div className="rounded-lg border p-3 text-sm">
              <h3 className="font-semibold">Candidate effect</h3>
              <p className="mt-2">Maximum cost: {preview.definition.budget_actions} actions</p>
              <p className="mt-1">Permissions: {preview.effective_authority.flatMap((authority) => authority.grants).join(", ")}</p>
              <p className="mt-1">Policy conflicts: {preview.diagnostics.filter((diagnostic) => diagnostic.kind === "conflicting_policy").length}</p>
            </div>
          </section>
          {preview.diagnostics.length > 0 && (
            <section>
              <h3 className="font-semibold text-[var(--danger)]">
                Attributable blockers
              </h3>
              <ul className="mt-2 space-y-2">
                {preview.diagnostics.map((d, i) => (
                  <li
                    key={i}
                    className="rounded-lg border border-[var(--danger)] p-3 text-sm"
                  >
                    <strong>{d.kind}</strong> · {d.message}
                    <span className="block text-xs">
                      Attributed to {d.attributed_to}
                      {d.step_id ? ` · step ${d.step_id}` : ""}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          )}
          <Button
            type="button"
            disabled={!preview.activatable || busy}
            onClick={activate}
          >
            {selected
              ? `Activate revision ${selected.current_version + 1}`
              : "Activate workflow"}
          </Button>
        </Card>
      )}
      <section>
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-xl font-semibold">Active workflows</h2>
          <Button
            type="button"
            onClick={() => {
              setSelected(undefined);
              setPreview(undefined);
            }}
          >
            Define new workflow
          </Button>
        </div>
        {workflows.length === 0 ? (
          <p className="mt-2 text-sm text-[var(--muted)]">
            No recurring collaboration is active.
          </p>
        ) : (
          <div className="mt-3 grid gap-3 md:grid-cols-3">
            {workflows.map((w) => {
              const r = w.revisions.at(-1)!;
              return (
                <button
                  key={w.id}
                  onClick={() => setSelected(w)}
                  className="rounded-xl border p-4 text-left"
                >
                  <span className="font-semibold">{r.definition.name}</span>
                  <span className="mt-1 block text-sm">
                    v{w.current_version} · {w.status}
                  </span>
                  <span className="mt-2 block text-xs text-[var(--muted)]">
                    {r.definition.outcome}
                  </span>
                </button>
              );
            })}
          </div>
        )}
      </section>
      {revision && (
        <Card className="p-5">
          <h2 className="text-xl font-semibold">
            Version {revision.version} review record
          </h2>
          <p className="mt-2 text-sm">{revision.definition.description}</p>
          <p className="mt-2 text-sm">
            <strong>Completion:</strong>{" "}
            {revision.definition.completion.join(" · ")}
          </p>
          <p className="mt-1 text-sm">
            <strong>Budget:</strong> {revision.definition.budget_actions}{" "}
            platform actions
          </p>
          <p className="mt-1 text-sm">
            <strong>Source:</strong> {revision.source.path} at{" "}
            {short(revision.source.revision)}
          </p>
          <div className="mt-4 flex flex-wrap gap-2">
            <Button type="button" disabled={busy} onClick={() => void control("disable")}>Emergency disable</Button>
            <Button type="button" disabled={busy} onClick={() => void control("anomaly_stop")}>Stop anomalous effects</Button>
            {selected && selected.current_version > 1 && <Button type="button" disabled={busy} onClick={() => void control("rollback")}>Roll back to v{selected.current_version - 1}</Button>}
          </div>
        </Card>
      )}
      {revision && (
        <section className="space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="text-xl font-semibold">Live and retained runs</h2>
            <span className="text-xs text-[var(--muted)]">
              Refreshes every 5 seconds
            </span>
          </div>
          {executions.length === 0 ? (
            <p className="text-sm text-[var(--muted)]">
              No event has invoked this workflow yet.
            </p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {executions.map((e) => (
                <button
                  key={e.id}
                  onClick={() => setSelectedExecution(e)}
                  className="rounded-lg border px-3 py-2 text-left text-sm"
                >
                  <strong>{e.trigger.name}</strong>
                  <span className="block text-xs">
                    {e.status} · {new Date(e.created_at).toLocaleString()}
                  </span>
                </button>
              ))}
            </div>
          )}
          {selected && selectedExecution?.workflow_id === selected.id && (
            <Card className="space-y-5 p-5">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="text-lg font-semibold">
                    Execution {short(selectedExecution.id)}
                  </h3>
                  <p className="text-sm text-[var(--muted)]">
                    Triggered by {selectedExecution.trigger.actor_id} ·{" "}
                    {new Date(
                      selectedExecution.trigger.occurred_at,
                    ).toLocaleString()}{" "}
                    · workflow v{selectedExecution.workflow_version}
                  </p>
                  <p className="text-xs text-[var(--muted)]">
                    {selectedExecution.actions_used}/
                    {selectedExecution.budget_actions} actions ·{" "}
                    {selectedExecution.cost_units} cost units · updated{" "}
                    {new Date(
                      selectedExecution.updated_at,
                    ).toLocaleTimeString()}
                  </p>
                </div>
                <Badge
                  tone={
                    selectedExecution.status === "succeeded"
                      ? "success"
                      : selectedExecution.status === "failed"
                        ? "danger"
                        : "neutral"
                  }
                >
                  {selectedExecution.status}
                </Badge>
              </div>
              <div className="flex flex-wrap gap-2">
                {selectedExecution.status === "running" && (
                  <Button
                    disabled={busy}
                    type="button"
                    onClick={() => void intervene("pause")}
                  >
                    Pause
                  </Button>
                )}
                {selectedExecution.status === "paused" && (
                  <Button
                    disabled={busy}
                    type="button"
                    onClick={() => void intervene("resume")}
                  >
                    Resume
                  </Button>
                )}
                {["running", "paused"].includes(selectedExecution.status) && (
                  <Button
                    disabled={busy}
                    type="button"
                    onClick={() => void intervene("cancel")}
                  >
                    Cancel
                  </Button>
                )}
              </div>
              <section>
                <h4 className="font-semibold">Predicted next actions</h4>
                <ul className="mt-2 list-disc pl-5 text-sm">
                  {(selectedExecution.predicted_next_actions ?? []).map((x) => (
                    <li key={x}>{x}</li>
                  ))}
                </ul>
              </section>
              {(selectedExecution.approval_requests ?? []).length > 0 && <section>
                <h4 className="font-semibold">Approval requests</h4>
                <ul className="mt-2 space-y-2 text-sm">{selectedExecution.approval_requests.map((request) => <li className="rounded-lg border p-3" key={`${request.step_id}-${request.requested_at}`}><strong>{request.action_class}</strong> · {request.step_id}<span className="block text-xs text-[var(--muted)]">Owner {request.owner_ids.join(", ")} · expires {new Date(request.expires_at).toLocaleString()} {request.approved_by ? `· approved by ${request.approved_by}` : "· pending"}</span></li>)}</ul>
              </section>}
              {(selectedExecution.action_receipts ?? []).length > 0 && <section>
                <h4 className="font-semibold">Immutable action receipts</h4>
                <ul className="mt-2 space-y-2 text-sm">{selectedExecution.action_receipts.map((receipt) => <li className="rounded-lg border p-3" key={receipt.id}><strong>{receipt.action_class || "workflow action"}</strong> · {receipt.actions} actions · {receipt.cost_units} cost units<span className="block text-xs text-[var(--muted)]">Receipt {short(receipt.id)} · proof {short(receipt.completion_sha256)}</span></li>)}</ul>
              </section>}
              <section>
                <h4 className="font-semibold">Execution graph</h4>
                {!executionRevision ? (
                  <p role="alert" className="mt-2 text-sm text-[var(--danger)]">
                    Workflow version {selectedExecution.workflow_version} is
                    unavailable, so this retained graph cannot be rendered
                    safely. The run summary and intervention history remain
                    visible.
                  </p>
                ) : (
                  <div className="mt-2 grid gap-3 md:grid-cols-2">
                    {executionRevision.definition.steps.map((def) => {
                      const run = selectedExecution.steps.find(
                        (s) => s.step_id === def.id,
                      );
                      if (!run)
                        return (
                          <article
                            key={def.id}
                            className="rounded-lg border p-4 text-sm text-[var(--danger)]"
                          >
                            Retained status for {def.name} is unavailable.
                          </article>
                        );
                      return (
                        <article key={def.id} className="rounded-lg border p-4">
                          <div className="flex justify-between gap-2">
                            <div>
                              <h5 className="font-medium">{def.name}</h5>
                              <p className="text-xs text-[var(--muted)]">
                                Needs {def.needs?.join(", ") || "nothing"} ·
                                owner {def.owner_ids.join(", ")}
                              </p>
                            </div>
                            <Badge>{run.status}</Badge>
                          </div>
                          <p className="mt-2 text-xs">
                            {run.failure_code &&
                              `Failure: ${run.failure_code} · `}
                            {run.actions_used} actions
                            {run.taken_over_by &&
                              ` · taken over by ${run.taken_over_by}`}
                          </p>
                          <div className="mt-3 flex flex-wrap gap-2">
                            {["interrupted", "failed"].includes(run.status) && (
                              <Button
                                type="button"
                                disabled={busy}
                                onClick={() => void intervene("retry", def.id)}
                              >
                                Retry
                              </Button>
                            )}
                            {def.optional &&
                              !["succeeded", "skipped", "cancelled"].includes(
                                run.status,
                              ) && (
                                <Button
                                  type="button"
                                  disabled={busy}
                                  onClick={() => void intervene("skip", def.id)}
                                >
                                  Skip optional
                                </Button>
                              )}
                            {def.manual &&
                              ["waiting_manual", "pending"].includes(
                                run.status,
                              ) && (
                                <Button
                                  type="button"
                                  disabled={busy}
                                  onClick={() =>
                                    void intervene("take_over", def.id)
                                  }
                                >
                                  Take over
                                </Button>
                              )}
                            {run.status === "waiting_approval" && (
                              <Button
                                type="button"
                                disabled={busy}
                                onClick={() =>
                                  void intervene("approve", def.id)
                                }
                              >
                                Approve
                              </Button>
                            )}
                          </div>
                          {run.status === "waiting_input" && (
                            <form
                              className="mt-3 grid gap-2"
                              onSubmit={(e) => {
                                e.preventDefault();
                                void intervene("provide_input", def.id, {
                                  input_name: input.name,
                                  value: input.value,
                                });
                              }}
                            >
                              <select
                                required
                                value={input.step === def.id ? input.name : ""}
                                onChange={(e) =>
                                  setInput({
                                    step: def.id,
                                    name: e.target.value,
                                    value: input.value,
                                  })
                                }
                                className="rounded border bg-transparent p-2 text-sm"
                              >
                                <option value="">Requested input…</option>
                                {def.requested_inputs?.map((x) => (
                                  <option key={x}>{x}</option>
                                ))}
                              </select>
                              <input
                                required
                                type="text"
                                value={input.step === def.id ? input.value : ""}
                                onChange={(e) =>
                                  setInput({
                                    step: def.id,
                                    name: input.name,
                                    value: e.target.value,
                                  })
                                }
                                className="rounded border bg-transparent p-2 text-sm"
                                placeholder="Non-secret value (terminal input is never accepted)"
                              />
                              <Button disabled={busy}>Provide input</Button>
                            </form>
                          )}
                          {(run.attempts ?? []).map((a) => (
                            <details
                              key={a.number}
                              className="mt-3 rounded border p-2 text-xs"
                            >
                              <summary>
                                Attempt {a.number} · {a.status} · {a.cost_units}{" "}
                                cost units
                              </summary>
                              <p className="mt-2">
                                {new Date(a.started_at).toLocaleString()}
                                {a.finished_at &&
                                  ` → ${new Date(a.finished_at).toLocaleString()}`}
                              </p>
                              {a.agent_session && (
                                <p>
                                  Agent session {a.agent_session.id} ·{" "}
                                  {a.agent_session.status}
                                </p>
                              )}
                              <p>Inputs: {JSON.stringify(a.inputs)}</p>
                              <p>
                                Redacted outputs:{" "}
                                {JSON.stringify(a.outputs ?? {})}
                              </p>
                              {(a.logs ?? []).map((l, i) => (
                                <p key={i} className="mt-1 font-mono">
                                  [{l.level}] {l.message}
                                </p>
                              ))}
                              {(a.artifacts ?? []).map((x) => (
                                <p key={x.sha256}>
                                  Artifact: {x.name} · {x.kind} · {x.size} bytes
                                  · {short(x.sha256)}
                                </p>
                              ))}
                              <p className="mt-1 text-[var(--muted)]">
                                Provenance: {(a.provenance ?? []).join(" · ")}
                              </p>
                            </details>
                          ))}
                        </article>
                      );
                    })}
                  </div>
                )}
              </section>
              <section>
                <h4 className="font-semibold">Attributable interventions</h4>
                {(selectedExecution.interventions ?? []).length === 0 ? (
                  <p className="mt-1 text-sm text-[var(--muted)]">
                    No collaborator has changed this run.
                  </p>
                ) : (
                  <ol className="mt-2 space-y-1 text-sm">
                    {(selectedExecution.interventions ?? []).map((x) => (
                      <li key={x.id}>
                        {x.kind.replace("_", " ")} by {x.actor_id}
                        {x.step_id && ` on ${x.step_id}`} · {x.reason} ·{" "}
                        {new Date(x.created_at).toLocaleString()}
                      </li>
                    ))}
                  </ol>
                )}
              </section>
            </Card>
          )}
        </section>
      )}
    </main>
  );
}
