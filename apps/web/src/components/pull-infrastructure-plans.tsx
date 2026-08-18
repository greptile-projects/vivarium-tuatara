"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Risk = {
  kind: string;
  severity: string;
  summary: string;
  mitigation: string;
};
type Resource = { id: string; name: string; kind: string; owner_ids: string[] };
type Change = {
  resource_id: string;
  action: "create" | "change" | "replace" | "destroy";
  before?: Resource;
  after?: Resource;
  dependency_ids: string[];
  order: number;
  risks: Risk[];
  rollback_limit: string;
};
type Event = {
  id: string;
  kind: string;
  actor_id: string;
  actor_type: string;
  body: string;
  resource_ids: string[];
  owner_id?: string;
  created_at: string;
};
type Check = {
  id: string;
  kind: string;
  command: string;
  resource_ids: string[];
  expectation: string;
};
type Outcome = {
  check_id: string;
  kind: string;
  status: string;
  exit_code: number;
  sanitized_log: string;
  duration_ms: number;
  actor_id: string;
};
type Run = {
  id: string;
  workspace_id: string;
  result: string;
  outcomes: Outcome[];
  artifacts: { path: string; digest: string; size: number }[];
  resource_graph: Resource[];
  attestations: string[];
  agent_actions: string[];
  created_at: string;
};
type Rehearsal = {
  id: string;
  name: string;
  scope: {
    environment_kind: string;
    environment_id: string;
    state_kind: string;
    state_description: string;
    credential_expires_at: string;
  };
  checks: Check[];
  unsupported_effects: {
    resource_id: string;
    effect: string;
    reason: string;
  }[];
  runs: Run[];
  created_at: string;
};
type Plan = {
  id: string;
  source_revision: string;
  definition_version: number;
  changes: Change[];
  policy_effects: { path: string; digest: string; effects: string[] }[];
  affected_owner_ids: string[];
  acknowledged_owner_ids: string[];
  events: Event[];
  rehearsals: Rehearsal[];
  fresh: boolean;
  stale_reasons: string[];
  created_at: string;
};

export function PullInfrastructurePlans({
  repositoryID,
  pullRequestID,
  participant,
}: {
  repositoryID: string;
  pullRequestID: string;
  participant: boolean;
}) {
  const { token, user } = useAuth();
  const [plans, setPlans] = useState<Plan[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    try {
      const out = await api<{ plans: Plan[] }>(
        `/repositories/${repositoryID}/pulls/${pullRequestID}/infrastructure-plans`,
        {},
        token,
      );
      setPlans(out.plans);
      setError("");
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Infrastructure plans could not be loaded.",
      );
    }
  }, [repositoryID, pullRequestID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function contribute(e: FormEvent<HTMLFormElement>, plan: Plan) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    setError("");
    const form = e.currentTarget,
      data = new FormData(form);
    const kind = String(data.get("kind"));
    try {
      await api(
        `/repositories/${repositoryID}/pulls/${pullRequestID}/infrastructure-plans/${plan.id}/events`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_events: plan.events.length,
            event: {
              kind,
              body: data.get("body"),
              resource_ids: data.getAll("resource_ids"),
              owner_id:
                kind === "owner_acknowledgement"
                  ? user?.id
                  : String(data.get("owner_id") ?? ""),
            },
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Review evidence could not be published.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function rehearse(e: FormEvent<HTMLFormElement>, plan: Plan) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    setError("");
    const form = e.currentTarget,
      data = new FormData(form),
      resources = plan.changes.map((x) => x.resource_id),
      kinds = [
        "provisioning",
        "connectivity",
        "access",
        "policy",
        "service_journey",
        "failure",
        "cost",
        "teardown",
        "recovery",
      ];
    try {
      await api(
        `/repositories/${repositoryID}/pulls/${pullRequestID}/infrastructure-plans/${plan.id}/rehearsals`,
        {
          method: "POST",
          body: JSON.stringify({
            name: data.get("name"),
            scope: {
              environment_kind: data.get("environment_kind"),
              environment_id: data.get("environment_id"),
              policy_approval: data.get("policy_approval"),
              credential_resource_ids: resources,
              credential_expires_at: data.get("credential_expires_at"),
              state_kind: data.get("state_kind"),
              state_description: data.get("state_description"),
            },
            checks: kinds.map((kind) => ({
              id: kind,
              kind,
              command: data.get(kind),
              resource_ids: resources,
              expectation: `${kind.replaceAll("_", " ")} meets the repository contract`,
            })),
            unsupported_effects: plan.changes
              .filter((x) => x.action === "destroy" || x.action === "replace")
              .map((x) => ({
                resource_id: x.resource_id,
                effect: `${x.action} authoritative resource`,
                reason:
                  "An ephemeral operation cannot prove destructive effects on authoritative provider state.",
              })),
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Rehearsal could not be frozen.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function publishRun(
    e: FormEvent<HTMLFormElement>,
    plan: Plan,
    rehearsal: Rehearsal,
  ) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    setError("");
    const form = e.currentTarget,
      data = new FormData(form);
    try {
      await api(
        `/repositories/${repositoryID}/pulls/${pullRequestID}/infrastructure-plans/${plan.id}/rehearsals/${rehearsal.id}/runs`,
        {
          method: "POST",
          body: JSON.stringify({
            workspace_id: data.get("workspace_id"),
            check_ids: rehearsal.checks.map((x) => x.id),
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Retained rehearsal evidence could not be published.",
      );
    } finally {
      setBusy(false);
    }
  }
  if (!plans.length && !error) return null;
  return (
    <section id="infrastructure-plan" className="scroll-mt-24 space-y-3">
      <div className="flex items-baseline justify-between">
        <h2 className="text-lg font-semibold">Infrastructure change plans</h2>
        <span className="text-xs text-[var(--muted)]">
          Plan and isolated evidence
        </span>
      </div>
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      {plans.map((plan) => (
        <Card key={plan.id} className="p-5">
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone={plan.fresh ? "success" : "warning"}>
              {plan.fresh ? "current" : "stale"}
            </Badge>
            <Badge>definition v{plan.definition_version}</Badge>
            <code className="text-xs">{plan.source_revision.slice(0, 10)}</code>
          </div>
          {!plan.fresh && (
            <p className="mt-3 rounded-lg bg-[var(--warning-soft)] p-3 text-sm">
              Prior evidence is stale:{" "}
              {plan.stale_reasons.map((x) => x.replaceAll("_", " ")).join(", ")}
              .
            </p>
          )}
          <ol className="mt-4 space-y-3">
            {plan.changes.map((change) => (
              <li
                key={change.resource_id}
                className="rounded-lg border border-[var(--line)] p-4"
              >
                <div className="flex flex-wrap gap-2">
                  <Badge>{change.order}</Badge>
                  <Badge
                    tone={
                      change.action === "destroy" || change.action === "replace"
                        ? "warning"
                        : "info"
                    }
                  >
                    {change.action}
                  </Badge>
                  <strong>
                    {change.after?.name ??
                      change.before?.name ??
                      change.resource_id}
                  </strong>
                </div>
                <p className="mt-2 text-xs text-[var(--muted)]">
                  Depends on{" "}
                  {change.dependency_ids.join(", ") || "no planned resource"}
                </p>
                <div className="mt-3 grid gap-2 sm:grid-cols-2">
                  {change.risks.map((risk, index) => (
                    <div
                      key={`${risk.kind}-${index}`}
                      className="rounded-lg bg-[var(--canvas)] p-3 text-sm"
                    >
                      <Badge
                        tone={
                          risk.severity === "high"
                            ? "danger"
                            : risk.severity === "medium"
                              ? "warning"
                              : "neutral"
                        }
                      >
                        {risk.kind} · {risk.severity}
                      </Badge>
                      <p className="mt-2">{risk.summary}</p>
                      <p className="mt-1 text-xs text-[var(--muted)]">
                        {risk.mitigation}
                      </p>
                    </div>
                  ))}
                </div>
                <p className="mt-3 text-xs">
                  <strong>Rollback limit:</strong> {change.rollback_limit}
                </p>
              </li>
            ))}
          </ol>
          <div className="mt-4">
            <h3 className="text-sm font-semibold">Expected policy effects</h3>
            {plan.policy_effects.map((effect) => (
              <p key={effect.path} className="mt-2 text-sm">
                <code>{effect.path}</code> · {effect.effects.join("; ")}
              </p>
            ))}
          </div>
          <div className="mt-4">
            <h3 className="text-sm font-semibold">
              Affected-owner acknowledgements
            </h3>
            <p className="mt-1 text-sm text-[var(--muted)]">
              {plan.acknowledged_owner_ids.length} of{" "}
              {plan.affected_owner_ids.length} current
            </p>
          </div>
          {plan.events.length > 0 && (
            <div className="mt-4 space-y-2">
              {plan.events.map((event) => (
                <p
                  key={event.id}
                  className="rounded-lg border border-[var(--line)] p-3 text-sm"
                >
                  <Badge>{event.kind.replaceAll("_", " ")}</Badge> {event.body}
                  <span className="mt-1 block text-xs text-[var(--muted)]">
                    {event.actor_type} {event.actor_id}
                  </span>
                </p>
              ))}
            </div>
          )}
          {plan.rehearsals?.map((rehearsal) => (
            <div
              key={rehearsal.id}
              className="mt-4 rounded-lg border border-[var(--line)] p-4"
            >
              <div className="flex flex-wrap gap-2">
                <Badge tone="info">isolated rehearsal</Badge>
                <strong>{rehearsal.name}</strong>
                <Badge>{rehearsal.scope.state_kind} state</Badge>
              </div>
              <p className="mt-2 text-sm">
                {rehearsal.scope.environment_kind.replaceAll("_", " ")} ·{" "}
                {rehearsal.scope.environment_id} · credentials expire{" "}
                {new Date(
                  rehearsal.scope.credential_expires_at,
                ).toLocaleString()}
              </p>
              {rehearsal.unsupported_effects.map((x) => (
                <p
                  key={x.resource_id}
                  className="mt-2 rounded bg-[var(--warning-soft)] p-2 text-sm"
                >
                  <strong>Not rehearsed: {x.effect}</strong> — {x.reason}
                </p>
              ))}
              <div className="mt-3 grid gap-2 sm:grid-cols-3">
                {rehearsal.checks.map((x) => (
                  <div
                    key={x.id}
                    className="rounded bg-[var(--canvas)] p-2 text-xs"
                  >
                    <strong>{x.kind.replaceAll("_", " ")}</strong>
                    <code className="mt-1 block break-all">{x.command}</code>
                  </div>
                ))}
              </div>
              {rehearsal.runs.map((run) => (
                <div
                  key={run.id}
                  className="mt-3 rounded bg-[var(--canvas)] p-3 text-sm"
                >
                  <Badge tone={run.result === "passed" ? "success" : "danger"}>
                    {run.result}
                  </Badge>
                  <span className="ml-2">
                    {run.outcomes.filter((x) => x.status === "passed").length}/
                    {run.outcomes.length} checks ·{" "}
                    {run.outcomes.reduce((n, x) => n + x.duration_ms, 0)} ms ·{" "}
                    {run.artifacts.length} artifacts
                  </span>
                  <p className="mt-2 text-xs text-[var(--muted)]">
                    {run.attestations.length} retained attestations ·{" "}
                    {run.resource_graph.length} graphed resources ·{" "}
                    {run.agent_actions.length} agent actions
                  </p>
                </div>
              ))}
              {token && participant && plan.fresh && (
                <form
                  onSubmit={(e) => publishRun(e, plan, rehearsal)}
                  className="mt-3 flex gap-2"
                >
                  <input
                    name="workspace_id"
                    required
                    placeholder="Exact-candidate workspace ID"
                    className="min-h-10 flex-1 rounded-lg border bg-white px-3 text-sm"
                  />
                  <Button disabled={busy}>Publish retained run</Button>
                </form>
              )}
            </div>
          ))}
          {token && participant && plan.fresh && (
            <>
              <form
                onSubmit={(e) => rehearse(e, plan)}
                className="mt-4 grid gap-3 rounded-lg bg-[var(--canvas)] p-4"
              >
                <h3 className="font-semibold">Freeze isolated rehearsal</h3>
                <div className="grid gap-3 sm:grid-cols-2">
                  <input
                    name="name"
                    required
                    placeholder="Rehearsal name"
                    className="rounded-lg border bg-white px-3 py-2"
                  />
                  <input
                    name="environment_id"
                    required
                    placeholder="Ephemeral environment identity"
                    className="rounded-lg border bg-white px-3 py-2"
                  />
                  <select
                    name="environment_kind"
                    className="rounded-lg border bg-white px-3 py-2"
                  >
                    <option value="isolated">Isolated</option>
                    <option value="policy_approved_ephemeral">
                      Policy-approved ephemeral
                    </option>
                  </select>
                  <input
                    name="policy_approval"
                    placeholder="Policy approval (required when selected)"
                    className="rounded-lg border bg-white px-3 py-2"
                  />
                  <select
                    name="state_kind"
                    className="rounded-lg border bg-white px-3 py-2"
                  >
                    <option value="synthetic">Synthetic state</option>
                    <option value="permitted">Permitted state</option>
                  </select>
                  <input
                    name="state_description"
                    required
                    placeholder="Sanitized state shape and permission"
                    className="rounded-lg border bg-white px-3 py-2"
                  />
                  <label className="text-sm">
                    Scoped credential expiry
                    <input
                      name="credential_expires_at"
                      type="datetime-local"
                      required
                      className="mt-1 w-full rounded-lg border bg-white px-3 py-2"
                    />
                  </label>
                </div>
                {[
                  "provisioning",
                  "connectivity",
                  "access",
                  "policy",
                  "service_journey",
                  "failure",
                  "cost",
                  "teardown",
                  "recovery",
                ].map((kind) => (
                  <input
                    key={kind}
                    name={kind}
                    required
                    placeholder={`${kind.replaceAll("_", " ")} check command`}
                    className="rounded-lg border bg-white px-3 py-2 font-mono text-sm"
                  />
                ))}
                <Button disabled={busy}>
                  {busy ? "Publishing…" : "Freeze rehearsal"}
                </Button>
              </form>
              <form
                onSubmit={(e) => contribute(e, plan)}
                className="mt-4 grid gap-3 rounded-lg bg-[var(--canvas)] p-4"
              >
                <select
                  name="kind"
                  className="min-h-10 rounded-lg border bg-white px-3"
                >
                  <option value="assumption">Annotate assumption</option>
                  <option value="impact">Investigate impact</option>
                  <option value="acknowledgement_request">
                    Request owner acknowledgement
                  </option>
                  {user && plan.affected_owner_ids.includes(user.id) && (
                    <option value="owner_acknowledgement">
                      Acknowledge as affected owner
                    </option>
                  )}
                </select>
                <input
                  name="owner_id"
                  placeholder="Affected owner ID when applicable"
                  className="rounded-lg border bg-white px-3 py-2"
                />
                <textarea
                  name="body"
                  required
                  maxLength={5000}
                  rows={2}
                  placeholder="Evidence or reasoning"
                  className="rounded-lg border bg-white p-3"
                />
                <Button disabled={busy}>Publish to plan</Button>
              </form>
            </>
          )}
          <p className="mt-4 text-xs text-[var(--muted)]">
            Rehearsals use bounded evidence and grant no production, provider,
            deployment, environment, review, merge, or destructive authority.
          </p>
        </Card>
      ))}
    </section>
  );
}
