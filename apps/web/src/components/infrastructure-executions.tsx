"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Step = {
  id: string;
  order: number;
  resource_id: string;
  action: string;
  dependency_ids: string[];
  status: string;
  controller_id?: string;
  provider_response?: string;
  health: string;
  cost_units: number;
  blockers: string[];
  next_action: string;
  safety_point: boolean;
};
type Execution = {
  id: string;
  plan_id: string;
  reviewed_revision: string;
  merge_commit_id: string;
  candidate_digest: string;
  environment_id: string;
  environment_policy: string;
  rehearsal_id: string;
  budget_units: number;
  cost_units: number;
  status: string;
  active_controller_id: string;
  version: number;
  steps: Step[];
  credential: {
    principal_id: string;
    step_ids: string[];
    resource_ids: string[];
    actions: string[];
    expires_at: string;
  };
  blockers: string[];
  next_actions: string[];
  expected_outcomes: { resource_id: string; present: boolean; measures: string[] }[];
  assessments: { id: string; converged: boolean; reasons: string[]; recorded_at: string }[];
  monitor_runs: { id: string; permission: string; provider_status: string; recorded_at: string; findings: { id: string; kind: string; resource_id?: string; severity: string; summary: string; cause?: string }[] }[];
  drift_responses: { id: string; finding_id: string; kind: string; owner_id: string; resource_kind: string; parent_id?: string; resource_id: string; summary: string }[];
  events: {
    sequence: number;
    kind: string;
    actor_id: string;
    actor_type: string;
    step_id?: string;
    summary: string;
    created_at: string;
  }[];
};
const field = (form: FormData, name: string) =>
  String(form.get(name) ?? "").trim();

export function InfrastructureExecutions({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token } = useAuth(),
    [items, setItems] = useState<Execution[]>([]),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    try {
      const out = await api<{ executions: Execution[] }>(
        `/repositories/${repositoryID}/infrastructure-executions`,
        {},
        token ?? undefined,
      );
      setItems(out.executions);
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Executions could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    const form = new FormData(e.currentTarget);
    try {
      await api(
        `/repositories/${repositoryID}/infrastructure-executions`,
        {
          method: "POST",
          body: JSON.stringify({
            plan_id: field(form, "plan_id"),
            environment_id: field(form, "environment_id"),
            environment_policy: field(form, "environment_policy"),
            rehearsal_id: field(form, "rehearsal_id"),
            budget_units: Number(field(form, "budget_units")),
            credential_expires_at: new Date(
              field(form, "credential_expires_at"),
            ).toISOString(),
            delegations: [],
          }),
        },
        token,
      );
      e.currentTarget.reset();
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Execution could not start.");
    } finally {
      setBusy(false);
    }
  }
  async function control(x: Execution, action: string) {
    if (!token) return;
    setBusy(true);
    try {
      await api(
        `/repositories/${repositoryID}/infrastructure-executions/${x.id}/controls`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: x.version,
            action,
            summary: `${action} requested from the infrastructure workspace`,
          }),
        },
        token,
      );
      await load();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Execution could not be steered.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function report(
    e: FormEvent<HTMLFormElement>,
    x: Execution,
    step: Step,
  ) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    const form = new FormData(e.currentTarget);
    try {
      await api(
        `/repositories/${repositoryID}/infrastructure-executions/${x.id}/reports`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: x.version,
            step_id: step.id,
            report: {
              status: field(form, "status"),
              provider_response: field(form, "provider_response"),
              health: field(form, "health"),
              cost_units: Number(field(form, "cost_units")),
              blockers: field(form, "blockers")
                .split(",")
                .map((v) => v.trim())
                .filter(Boolean),
              next_action: field(form, "next_action"),
              safety_point: field(form, "safety_point") === "yes",
            },
          }),
        },
        token,
      );
      await load();
    } catch (y) {
      setError(
        y instanceof Error ? y.message : "Step evidence could not be reported.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function assess(e: FormEvent<HTMLFormElement>, x: Execution) {
    e.preventDefault(); if (!token) return; setBusy(true); const form = new FormData(e.currentTarget);
    const state = field(form, "outcome");
    try {
      await api(`/repositories/${repositoryID}/infrastructure-executions/${x.id}/assessments`, { method:"POST", body:JSON.stringify({ expected_version:x.version, outcomes:x.expected_outcomes.map((o)=>({ resource_id:o.resource_id, present:field(form, `present-${o.resource_id}`)==="yes", provider_revision:field(form, `revision-${o.resource_id}`), service:state, security:state, privacy:state, cost:state, continuity:state, measures_passed:state==="passed"?o.measures:[], summary:field(form,"summary") })), unmanaged_resources:field(form,"unmanaged").split(",").map(v=>v.trim()).filter(Boolean), failed_cleanup:field(form,"cleanup").split(",").map(v=>v.trim()).filter(Boolean) }) }, token); await load();
    } catch (z) { setError(z instanceof Error?z.message:"Convergence evidence could not be retained."); } finally { setBusy(false); }
  }
  async function monitor(e: FormEvent<HTMLFormElement>, x: Execution) {
    e.preventDefault(); if (!token) return; setBusy(true); const form=new FormData(e.currentTarget); const summary=field(form,"finding_summary");
    try { await api(`/repositories/${repositoryID}/infrastructure-executions/${x.id}/monitor-runs`,{method:"POST",body:JSON.stringify({permission:field(form,"permission"),provider_status:field(form,"provider_status"),findings:summary?[{kind:field(form,"finding_kind"),resource_id:field(form,"resource_id"),severity:field(form,"severity"),summary,cause:field(form,"cause")}]:[]})},token); await load(); } catch(z){setError(z instanceof Error?z.message:"Monitoring evidence could not be retained.");} finally{setBusy(false);}
  }
  async function respond(e: FormEvent<HTMLFormElement>, x: Execution, findingID: string) {
    e.preventDefault(); if(!token)return; setBusy(true); const form=new FormData(e.currentTarget);
    try { await api(`/repositories/${repositoryID}/infrastructure-executions/${x.id}/drift-responses`,{method:"POST",body:JSON.stringify({expected_version:x.version,response:{finding_id:findingID,kind:field(form,"response_kind"),owner_id:field(form,"owner_id"),resource_kind:field(form,"resource_kind"),parent_id:field(form,"parent_id"),resource_id:field(form,"work_id"),summary:field(form,"response_summary")}})},token); await load(); } catch(z){setError(z instanceof Error?z.message:"Drift response could not be linked.");} finally{setBusy(false);}
  }
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-lg font-semibold">Authoritative executions</h2>
        <p className="text-sm text-[var(--muted)]">
          Apply an exact merged, acknowledged, rehearsed plan under an
          established environment&apos;s policy. Records expose scoped authority
          and never reveal provider credentials.
        </p>
      </div>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      {token && (
        <Card className="p-5">
          <h3 className="font-semibold">Start governed execution</h3>
          <form onSubmit={create} className="mt-3 grid gap-3 md:grid-cols-3">
            <Input name="plan_id" label="Reviewed plan ID" />
            <Input name="rehearsal_id" label="Passing rehearsal ID" />
            <Input name="environment_id" label="Established environment ID" />
            <Input
              name="environment_policy"
              label="Satisfied environment policy"
            />
            <Input
              name="budget_units"
              label="Approved cost budget"
              type="number"
            />
            <Input
              name="credential_expires_at"
              label="Scoped credential expiry"
              type="datetime-local"
            />
            <Button disabled={busy}>Start exact merged apply</Button>
          </form>
        </Card>
      )}
      {items.length === 0 && (
        <p className="text-sm text-[var(--muted)]">
          No authoritative infrastructure execution has started.
        </p>
      )}
      {items.map((x) => (
        <Card key={x.id} className="p-5">
          <div className="flex flex-wrap gap-2">
            <Badge
              tone={
                x.status === "succeeded"
                  ? "success"
                  : x.status === "paused"
                    ? "warning"
                    : x.status === "cancelled"
                      ? "danger"
                      : "info"
              }
            >
              {x.status}
            </Badge>
            <Badge>{x.environment_id}</Badge>
            <Badge>controller {x.active_controller_id}</Badge>
            <code className="text-xs">
              merge {x.merge_commit_id.slice(0, 10)}
            </code>
          </div>
          <p className="mt-2 text-sm">
            Policy: {x.environment_policy} · cost {x.cost_units}/
            {x.budget_units} · credential expires{" "}
            {new Date(x.credential.expires_at).toLocaleString()}
          </p>
          <p className="mt-1 text-xs text-[var(--muted)]">
            Plan {x.plan_id} · reviewed {x.reviewed_revision.slice(0, 10)} ·
            candidate digest {x.candidate_digest.slice(0, 12)}
          </p>
          {token && (x.status === "running" || x.status === "paused") && (
            <div className="mt-3 flex gap-2">
              {x.status === "running" && (
                <Button
                  disabled={busy}
                  variant="secondary"
                  onClick={() => control(x, "pause")}
                >
                  Pause at safety point
                </Button>
              )}
              {x.status === "paused" && (
                <Button
                  disabled={busy}
                  variant="secondary"
                  onClick={() => control(x, "resume")}
                >
                  Resume
                </Button>
              )}
              <Button
                disabled={busy}
                variant="secondary"
                onClick={() => control(x, "cancel")}
              >
                Cancel safely
              </Button>
            </div>
          )}
          <ol className="mt-4 space-y-3">
            {x.steps.map((step) => (
              <li key={step.id} className="rounded-lg border p-4">
                <div className="flex flex-wrap gap-2">
                  <Badge>{step.order}</Badge>
                  <Badge
                    tone={
                      step.status === "succeeded"
                        ? "success"
                        : step.status === "failed"
                          ? "danger"
                          : "neutral"
                    }
                  >
                    {step.status}
                  </Badge>
                  <strong>
                    {step.action} {step.resource_id}
                  </strong>
                  <Badge
                    tone={
                      step.health === "healthy"
                        ? "success"
                        : step.health === "degraded"
                          ? "warning"
                          : "neutral"
                    }
                  >
                    {step.health}
                  </Badge>
                  {step.safety_point && <Badge>safe intervention</Badge>}
                </div>
                <p className="mt-2 text-sm">
                  Provider: {step.provider_response || "No response yet"}
                </p>
                <p className="text-xs text-[var(--muted)]">
                  Depends on {step.dependency_ids.join(", ") || "nothing"} ·
                  cost {step.cost_units} · next: {step.next_action}
                </p>
                {step.blockers.map((b) => (
                  <p key={b} className="mt-2 text-sm text-[var(--danger)]">
                    Blocked: {b}
                  </p>
                ))}
                {token &&
                  (x.status === "running" || x.status === "paused") &&
                  step.status !== "succeeded" && (
                    <form
                      onSubmit={(e) => report(e, x, step)}
                      className="mt-3 grid gap-2 md:grid-cols-3"
                    >
                      <Select
                        name="status"
                        options={
                          x.status === "paused"
                            ? ["running"]
                            : ["running", "succeeded", "failed"]
                        }
                      />
                      <Select
                        name="health"
                        options={["healthy", "degraded", "unknown"]}
                      />
                      <Input
                        name="cost_units"
                        label="Cumulative step cost"
                        type="number"
                      />
                      <Input
                        name="provider_response"
                        label="Sanitized provider response"
                      />
                      <Input
                        name="blockers"
                        label="Blockers (comma separated)"
                        required={false}
                      />
                      <Input name="next_action" label="Next action" />
                      <Select name="safety_point" options={["yes", "no"]} />
                      <Button disabled={busy}>
                        {x.status === "paused"
                          ? "Report remediation"
                          : "Report exact step"}
                      </Button>
                    </form>
                  )}
              </li>
            ))}
          </ol>
          {token && ["succeeded", "paused", "cancelled"].includes(x.status) && <form onSubmit={(e)=>assess(e,x)} className="mt-4 grid gap-2 rounded-lg border p-4 md:grid-cols-3"><h3 className="md:col-span-3 font-semibold">Verify reviewed outcomes</h3><Select name="outcome" options={["passed","failed","unknown"]}/>{x.expected_outcomes.map(o=><div key={o.resource_id} className="contents"><Input name={`revision-${o.resource_id}`} label={`${o.resource_id} provider revision`}/><Select name={`present-${o.resource_id}`} options={["yes","no"]}/></div>)}<Input name="summary" label="Sanitized verification summary"/><Input name="unmanaged" label="Unmanaged resources" required={false}/><Input name="cleanup" label="Failed cleanup" required={false}/><Button disabled={busy}>Assess convergence</Button></form>}
          {x.assessments?.map(a=><div key={a.id} className="mt-3 rounded-lg border p-3"><Badge tone={a.converged?"success":"warning"}>{a.converged?"converged":"divergent"}</Badge><p className="mt-2 text-xs text-[var(--muted)]">{a.reasons.join(" · ") || "Every frozen outcome and measure passed."}</p></div>)}
          {token && <form onSubmit={(e)=>monitor(e,x)} className="mt-4 grid gap-2 rounded-lg border p-4 md:grid-cols-3"><h3 className="md:col-span-3 font-semibold">Permission-aware drift monitor</h3><Select name="permission" options={["granted","partial","denied"]}/><Select name="provider_status" options={["available","degraded","lost","unknown"]}/><Select name="finding_kind" options={["configuration_drift","unmanaged_change","failed_cleanup","credential_expiring","provider_loss"]}/><Select name="severity" options={["low","medium","high","critical"]}/><Input name="resource_id" label="Resource ID" required={false}/><Input name="finding_summary" label="Finding (blank for no finding)" required={false}/><Input name="cause" label="Attributed cause" required={false}/><Button disabled={busy}>Record monitoring run</Button></form>}
          {x.monitor_runs?.flatMap(run=>run.findings).map(f=><div key={f.id} className="mt-3 rounded-lg border p-3"><div className="flex gap-2"><Badge tone={f.severity==="critical"||f.severity==="high"?"danger":"warning"}>{f.kind}</Badge>{f.resource_id&&<Badge>{f.resource_id}</Badge>}</div><p className="mt-2 text-sm">{f.summary}</p><p className="text-xs text-[var(--muted)]">{f.cause||"Cause unavailable"} · response must link ordinary governed work.</p>{token&&<form onSubmit={(e)=>respond(e,x,f.id)} className="mt-3 grid gap-2 md:grid-cols-3"><Select name="response_kind" options={["incident","exception","repair","adopt","restore"]}/><Select name="resource_kind" options={["issue","proposal","task","incident","pull_request"]}/><Input name="parent_id" label="Proposal ID (task only)" required={false}/><Input name="work_id" label="Existing governed work ID"/><Input name="owner_id" label="Accountable owner ID"/><Input name="response_summary" label="Response and policy boundary"/><Button disabled={busy}>Link accountable response</Button></form>}</div>)}
          {x.drift_responses?.map((response) => (
            <div key={response.id} className="mt-3 rounded-lg border p-3">
              <div className="flex flex-wrap gap-2">
                <Badge tone="success">{response.kind}</Badge>
                <Badge>{response.resource_kind.replaceAll("_", " ")}</Badge>
                <Badge>owner {response.owner_id}</Badge>
              </div>
              <p className="mt-2 text-sm">{response.summary}</p>
              <p className="text-xs text-[var(--muted)]">
                Governed work {response.resource_id} · finding {response.finding_id}
              </p>
            </div>
          ))}
        </Card>
      ))}
    </section>
  );
}
function Input({
  name,
  label,
  type = "text",
  required = true,
}: {
  name: string;
  label: string;
  type?: string;
  required?: boolean;
}) {
  return (
    <label className="text-xs font-semibold">
      {label}
      <input
        name={name}
        type={type}
        required={required}
        className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal"
      />
    </label>
  );
}
function Select({ name, options }: { name: string; options: string[] }) {
  return (
    <select
      aria-label={name.replaceAll("_", " ")}
      name={name}
      className="min-h-10 rounded-lg border bg-white px-3"
    >
      {options.map((x) => (
        <option key={x}>{x}</option>
      ))}
    </select>
  );
}
