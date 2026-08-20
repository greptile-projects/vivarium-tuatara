"use client";

import { useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { Badge, Button, Card } from "./ui";

type Consumer = {
  name: string;
  repository_id?: string;
  revision?: string;
  owner_ids: string[];
  compatibility_promise: string;
};
type Revision = { consumers: Consumer[] };
export type RetirementPlan = {
  id: string;
  capability_version: number;
  rationale: string;
  replacements: { name: string; reference: string; migration_guide: string }[];
  audiences: {
    name: string;
    impact: string;
    commitment?: string;
    embargoed_dependency: boolean;
  }[];
  deadline: string;
  success_criteria: string[];
  rollback_criteria: string[];
  events: {
    version: number;
    type: string;
    actor_id: string;
    summary: string;
  }[];
  blockers: {
    kind: string;
    message: string;
    owner_id?: string;
    audience?: string;
  }[];
  status: string;
  work_version: number;
  work?: {
    id: string;
    repository_id: string;
    dependency_ids: string[];
    old_contract: string;
    replacement_contract: string;
    acceptance_criteria: string[];
    documentation_changes: string[];
    rollout_stage: string;
    status?: string;
    ready: boolean;
    assignee_type?: string;
    assignee_id?: string;
    session_id?: string;
    workspace_id?: string;
    fork_repository_id?: string;
    pull_request_id?: string;
    contribution_status?: string;
  }[];
  discovered_consumers?: {
    id: string;
    repository_id: string;
    revision: string;
    paths: string[];
    impact: string;
  }[];
};
const value = (f: FormData, n: string) => String(f.get(n) ?? "").trim();
const values = (v: string) =>
  v
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
function Field({
  name,
  label,
  value: initial = "",
  required = true,
}: {
  name: string;
  label: string;
  value?: string;
  required?: boolean;
}) {
  return (
    <label className="grid gap-1 text-xs font-semibold">
      {label}
      <input
        name={name}
        defaultValue={initial}
        required={required}
        className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm font-normal"
      />
    </label>
  );
}

export function RetirementContracts({
  repositoryID,
  capabilityID,
  current,
  plans,
  token,
  userID,
  reload,
}: {
  repositoryID: string;
  capabilityID: string;
  current: Revision;
  plans: RetirementPlan[];
  token?: string | null;
  userID?: string;
  reload: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  async function open(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const f = new FormData(event.currentTarget),
      at = (n: string) => new Date(value(f, n)).toISOString(),
      audience = current.consumers[0];
    setBusy(true);
    setError("");
    const body = {
      rationale: value(f, "rationale"),
      replacements: [
        {
          name: value(f, "replacement"),
          reference: value(f, "reference"),
          migration_guide: value(f, "guide"),
          supported: true,
        },
      ],
      audiences: [
        {
          name: audience.name,
          owner_ids: audience.owner_ids,
          impact: value(f, "impact"),
          commitment: value(f, "commitment"),
          embargoed_dependency: f.get("embargoed") === "on",
        },
        ...current.consumers
          .slice(1)
          .map((x) => ({
            name: x.name,
            owner_ids: x.owner_ids,
            impact: `${x.name} must migrate before removal`,
            commitment: x.compatibility_promise,
            embargoed_dependency: false,
          })),
      ],
      stages: [
        {
          name: "notice",
          starts_at: at("notice_at"),
          behavior: value(f, "notice_behavior"),
          exit_criteria: values(value(f, "notice_exit")),
        },
        {
          name: "removal",
          starts_at: at("deadline"),
          behavior: value(f, "removal_behavior"),
          exit_criteria: values(value(f, "removal_exit")),
        },
      ],
      deadline: at("deadline"),
      approval_due_at: at("approval_due"),
      success_criteria: values(value(f, "success")),
      rollback_criteria: values(value(f, "rollback")),
      communication: {
        channels: values(value(f, "channels")),
        notice_days: Number(value(f, "notice_days")),
        updates: value(f, "updates"),
        escalation: value(f, "escalation"),
      },
      required_owner_ids: [
        ...new Set(current.consumers.flatMap((x) => x.owner_ids)),
      ],
    };
    try {
      await api(
        `/repositories/${repositoryID}/capabilities/${capabilityID}/retirement-plans`,
        { method: "POST", body: JSON.stringify(body) },
        token,
      );
      await reload();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Retirement plan could not be opened.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function respond(
    event: FormEvent<HTMLFormElement>,
    plan: RetirementPlan,
  ) {
    event.preventDefault();
    const f = new FormData(event.currentTarget),
      kind = value(f, "kind"),
      approval = kind === "approval";
    setBusy(true);
    setError("");
    try {
      await api(
        `/repositories/${repositoryID}/capabilities/${capabilityID}/retirement-plans/${plan.id}/events`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: plan.events.length,
            event: {
              type: kind,
              summary: value(f, "summary"),
              evidence: approval ? undefined : values(value(f, "evidence")),
              owner_id: approval ? userID : undefined,
              decision: approval ? "approved" : undefined,
            },
          }),
        },
        token,
      );
      await reload();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Response could not be retained.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function createWork(event: FormEvent<HTMLFormElement>, plan: RetirementPlan) {
    event.preventDefault();
    const f = new FormData(event.currentTarget);
    setBusy(true);
    setError("");
    try {
      await api(`/repositories/${repositoryID}/capabilities/${capabilityID}/retirement-plans/${plan.id}/work`, {
        method: "POST",
        body: JSON.stringify({
          expected_version: plan.work_version,
          repository_id: value(f, "repository_id"), title: value(f, "title"),
          completion_criteria: value(f, "completion"), assignee_type: value(f, "assignee_type"),
          assignee_id: value(f, "assignee_id"), mandate: value(f, "mandate"), base_revision: value(f, "base_revision"),
          work: { audience_index: Number(value(f, "audience_index")), dependency_ids: values(value(f, "dependencies")),
            old_contract: value(f, "old_contract"), replacement_contract: value(f, "replacement_contract"),
            acceptance_criteria: values(value(f, "acceptance")), documentation_changes: values(value(f, "documentation")),
            rollout_stage: value(f, "rollout_stage") },
        }),
      }, token);
      await reload();
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Migration work could not be created."); }
    finally { setBusy(false); }
  }
  return (
    <section className="space-y-4">
      <div>
        <h2 className="text-xl font-semibold">Retirement contracts</h2>
        <p className="mt-1 text-sm text-[var(--muted)]">
          Removal stays blocked until affected owners understand the migration
          and acknowledge the exact inventory.
        </p>
      </div>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      {plans.map((plan) => (
        <Card key={plan.id} className="p-5">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="font-semibold">{plan.rationale}</h3>
            <Badge
              tone={
                plan.status === "acknowledged"
                  ? "success"
                  : plan.status === "deferred"
                    ? "warning"
                    : "danger"
              }
            >
              {plan.status}
            </Badge>
            <Badge>inventory v{plan.capability_version}</Badge>
          </div>
          <div className="mt-4 grid gap-4 md:grid-cols-2">
            <div>
              <h4 className="text-sm font-semibold">
                What stops and how to migrate
              </h4>
              {plan.audiences.map((x) => (
                <p key={x.name} className="mt-2 text-sm">
                  <strong>{x.name}:</strong> {x.impact}
                  {x.commitment ? ` · ${x.commitment}` : ""}
                  {x.embargoed_dependency ? " · restricted dependency" : ""}
                </p>
              ))}
              {plan.replacements.map((x) => (
                <p key={x.reference} className="mt-2 text-sm">
                  <strong>Replacement:</strong> {x.name} · {x.migration_guide}
                </p>
              ))}
            </div>
            <div>
              <h4 className="text-sm font-semibold">Decision boundaries</h4>
              <p className="mt-2 text-sm">
                Remove by {new Date(plan.deadline).toLocaleString()}
              </p>
              <p className="text-sm">
                Success: {plan.success_criteria.join(" · ")}
              </p>
              <p className="text-sm">
                Rollback: {plan.rollback_criteria.join(" · ")}
              </p>
            </div>
          </div>
          {plan.blockers.length > 0 && (
            <div className="mt-4 space-y-2">
              <h4 className="text-sm font-semibold">Attributable blockers</h4>
              {plan.blockers.map((x, i) => (
                <p key={`${x.kind}-${i}`} className="text-sm">
                  <Badge tone="danger">{x.kind.replaceAll("_", " ")}</Badge>{" "}
                  <span className="ml-2">
                    {x.message}
                    {x.owner_id ? ` Owner: ${x.owner_id}` : ""}
                    {x.audience ? ` Audience: ${x.audience}` : ""}
                  </span>
                </p>
              ))}
            </div>
          )}
          {(plan.work?.length ?? 0) > 0 && <div className="mt-4 space-y-2">
            <h4 className="text-sm font-semibold">Consumer-owned migration sequence</h4>
            {plan.work?.map((work) => <div key={work.id} className="rounded-lg border border-[var(--line)] p-3 text-sm">
              <div className="flex flex-wrap gap-2"><Badge>{work.rollout_stage}</Badge><Badge tone={work.contribution_status === "merged" ? "success" : work.ready ? "info" : "warning"}>{work.contribution_status ?? work.status ?? "planned"}</Badge></div>
              <p className="mt-2"><strong>Old:</strong> {work.old_contract}</p><p><strong>Replacement:</strong> {work.replacement_contract}</p>
              <p>Acceptance: {work.acceptance_criteria.join(" · ")}</p><p>Docs: {work.documentation_changes.join(" · ")}</p>
              {work.session_id && <p>Session: {work.session_id}</p>}{work.workspace_id && <p>Workspace: {work.workspace_id}</p>}{work.fork_repository_id && <p>Owned fork: {work.fork_repository_id}</p>}
              {work.pull_request_id && <p>Pull request: {work.pull_request_id}</p>}
            </div>)}
          </div>}
          {(plan.discovered_consumers?.length ?? 0) > 0 && <div className="mt-4"><h4 className="text-sm font-semibold">Newly reported consumers</h4>{plan.discovered_consumers?.map((item) => <p key={item.id} className="mt-1 text-sm">{item.repository_id} at {item.revision.slice(0, 8)} · {item.paths.join(", ")} · {item.impact}</p>)}</div>}
          {userID && current.consumers.some((consumer) => consumer.repository_id && consumer.revision) && <form onSubmit={(event) => void createWork(event, plan)} className="mt-5 grid gap-3 border-t border-[var(--line)] pt-4 md:grid-cols-2">
            <h4 className="md:col-span-2 text-sm font-semibold">Create work in an affected repository you control</h4>
            <label className="grid gap-1 text-xs font-semibold">Affected consumer<select name="audience_index" className="min-h-10 rounded-lg border px-3 font-normal">{current.consumers.map((consumer, index) => consumer.repository_id && consumer.revision ? <option key={consumer.repository_id} value={index}>{consumer.name}</option> : null)}</select></label>
            <Field name="repository_id" label="Consumer repository ID" value={current.consumers.find((consumer) => consumer.repository_id)?.repository_id} />
            <Field name="base_revision" label="Exact consumer base revision" value={current.consumers.find((consumer) => consumer.revision)?.revision} />
            <Field name="title" label="Contribution title" />
            <Field name="old_contract" label="Exact old contract" />
            <Field name="replacement_contract" label="Exact replacement contract" value={`${plan.replacements[0]?.reference ?? "supported replacement"} · ${plan.replacements[0]?.migration_guide ?? ""}`} />
            <Field name="acceptance" label="Acceptance criteria (comma-separated)" />
            <Field name="documentation" label="Documentation changes (comma-separated)" />
            <Field name="rollout_stage" label="Rollout stage" />
            <Field name="completion" label="Task completion criteria" />
            <Field name="dependencies" label="Earlier work IDs (comma-separated)" required={false} />
            <label className="grid gap-1 text-xs font-semibold">Owner type<select name="assignee_type" className="min-h-10 rounded-lg border px-3 font-normal"><option value="human">human</option><option value="agent">agent</option></select></label>
            <Field name="assignee_id" label="Human or approved-agent ID" value={userID} />
            <Field name="mandate" label="Least-privilege mandate" />
            <div className="self-end"><Button disabled={busy}>Create migration task</Button></div>
          </form>}
          {userID && (
            <form
              onSubmit={(e) => void respond(e, plan)}
              className="mt-5 grid gap-3 border-t border-[var(--line)] pt-4 md:grid-cols-2"
            >
              <label className="grid gap-1 text-xs font-semibold">
                Response
                <select
                  name="kind"
                  className="min-h-10 rounded-lg border px-3 font-normal"
                >
                  <option>assessment</option>
                  <option>challenge</option>
                  <option>approval</option>
                </select>
              </label>
              <Field name="summary" label="Assessment or acknowledgement" />
              <Field
                name="evidence"
                label="Cited evidence (required except approval)"
                required={false}
              />
              <div className="self-end">
                <Button disabled={busy}>Retain response</Button>
              </div>
            </form>
          )}
        </Card>
      ))}
      {userID && plans.length === 0 && current.consumers.length > 0 && (
        <Card className="p-5">
          <h3 className="font-semibold">
            Open an acknowledged migration contract
          </h3>
          <form onSubmit={open} className="mt-4 grid gap-3 md:grid-cols-2">
            <Field name="rationale" label="Rationale" />
            <Field name="replacement" label="Supported replacement" />
            <Field name="reference" label="Replacement reference" />
            <Field name="guide" label="Migration guide" />
            <Field
              name="impact"
              label={`What stops working for ${current.consumers[0].name}`}
            />
            <Field
              name="commitment"
              label="Conflicting commitment"
              value={current.consumers[0].compatibility_promise}
              required={false}
            />
            <Field name="notice_at" label="Notice starts (ISO date)" />
            <Field name="approval_due" label="Acknowledgement due (ISO date)" />
            <Field name="deadline" label="Removal deadline (ISO date)" />
            <Field name="notice_behavior" label="Notice-stage behavior" />
            <Field name="notice_exit" label="Notice exit criteria" />
            <Field name="removal_behavior" label="Removal-stage behavior" />
            <Field name="removal_exit" label="Removal exit criteria" />
            <Field name="success" label="Success criteria" />
            <Field name="rollback" label="Rollback criteria" />
            <Field name="channels" label="Communication channels" />
            <Field name="notice_days" label="Minimum notice days" value="30" />
            <Field name="updates" label="Update cadence" />
            <Field name="escalation" label="Unresponsive-owner escalation" />
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" name="embargoed" />
              Embargoed dependency exists
            </label>
            <div>
              <Button disabled={busy}>Open retirement plan</Button>
            </div>
          </form>
        </Card>
      )}
    </section>
  );
}
