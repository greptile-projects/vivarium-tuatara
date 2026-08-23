"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import {
  api,
  APIError,
  DevelopmentWorkspace,
  WorkspaceConsumption,
  WorkspacePolicy,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
import { WorkspaceIDE } from "./workspace-ide";
import { WorkspaceCheckpoints } from "./workspace-checkpoints";
import { ContributionHelp } from "./contribution-help";

const short = (value: string) => value.slice(0, 8);
export function DevelopmentWorkspaces({
  workspaceID,
}: {
  workspaceID?: string;
}) {
  const { token, user } = useAuth();
  const [items, setItems] = useState<DevelopmentWorkspace[]>([]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const controlsLifecycle = (item: DevelopmentWorkspace) =>
    item.control?.principal_kind === "human" &&
    item.control.principal_id === user?.id &&
    item.control.mode === "execute" &&
    item.control.scopes.includes("lifecycle") &&
    new Date(item.control.expires_at) > new Date();
  useEffect(() => {
    if (!token) return;
    let active = true;
    const request = workspaceID
      ? api<DevelopmentWorkspace>(`/workspaces/${workspaceID}`, {}, token).then(
          (value) => [value],
        )
      : api<{ items: DevelopmentWorkspace[] }>("/workspaces", {}, token).then(
          (value) => value.items,
        );
    void request
      .then((value) => {
        if (active) {
          setError("");
          setItems(value);
        }
      })
      .catch((e) => {
        if (active)
          setError(
            e instanceof APIError
              ? e.message
              : "Workspaces could not be loaded.",
          );
      });
    return () => {
      active = false;
    };
  }, [token, workspaceID]);
  async function transition(
    item: DevelopmentWorkspace,
    target: "suspend" | "resume",
  ) {
    if (!token) return;
    setPending(true);
    try {
      const updated = await api<DevelopmentWorkspace>(
        `/workspaces/${item.id}/${target}`,
        {
          method: "POST",
          body: JSON.stringify({ foundation: item.definition_sha256 }),
        },
        token,
      );
      setItems((current) =>
        current.map((value) => (value.id === updated.id ? updated : value)),
      );
    } catch (e) {
      setError(e instanceof APIError ? e.message : "Lifecycle update failed.");
    } finally {
      setPending(false);
    }
  }
  async function takeLifecycleControl(item: DevelopmentWorkspace) {
    if (!token || !user) return;
    setPending(true);
    try {
      const updated = await api<DevelopmentWorkspace>(
        `/workspaces/${item.id}/control`,
        {
          method: "PUT",
          body: JSON.stringify({
            expected_version: item.control.version,
            principal_kind: "human",
            principal_id: user.id,
            mode: "execute",
            scopes: ["files", "commands", "lifecycle"],
            expires_in: 900,
          }),
        },
        token,
      );
      setItems((current) =>
        current.map((value) => (value.id === updated.id ? updated : value)),
      );
    } catch (e) {
      setError(e instanceof APIError ? e.message : "Control takeover failed.");
    } finally {
      setPending(false);
    }
  }
  return (
    <div className="space-y-4">
      {error && (
        <Card className="border-[var(--danger)] p-4 text-sm text-[var(--danger)]">
          {error}
        </Card>
      )}
      {!workspaceID && (
        <div className="flex justify-end">
          <Link
            href="/workspaces/new"
            className="inline-flex min-h-9 items-center rounded-lg bg-[var(--brand)] px-4 text-sm font-semibold text-white"
          >
            Launch workspace
          </Link>
        </div>
      )}
      {items.map((item) => (
        <Card key={item.id} className="p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div className="flex items-center gap-2">
                <Badge
                  tone={
                    item.state === "running"
                      ? "success"
                      : item.state === "failed"
                        ? "danger"
                        : "warning"
                  }
                >
                  {item.state}
                </Badge>
                <span className="font-mono text-xs text-[var(--muted)]">
                  {short(item.id)}
                </span>
              </div>
              <h2 className="mt-3 font-semibold">
                Revision {short(item.commit_id)}
              </h2>
              <p className="mt-1 text-sm text-[var(--muted)]">
                From {item.source.kind.replaceAll("_", " ")} · created{" "}
                {new Date(item.created_at).toLocaleString()}
              </p>
            </div>
            {!workspaceID && (
              <Link
                className="text-sm font-semibold text-[var(--brand)]"
                href={`/workspaces/${item.id}`}
              >
                Inspect →
              </Link>
            )}
          </div>
          <dl className="mt-5 grid gap-3 text-sm sm:grid-cols-3">
            <div>
              <dt className="text-xs font-semibold text-[var(--muted)]">
                Foundation
              </dt>
              <dd className="mt-1 font-mono">
                sha256:{short(item.definition_sha256)}
              </dd>
            </div>
            <div>
              <dt className="text-xs font-semibold text-[var(--muted)]">
                Runtime
              </dt>
              <dd className="mt-1">{item.definition.image}</dd>
            </div>
            <div>
              <dt className="text-xs font-semibold text-[var(--muted)]">
                Resources
              </dt>
              <dd className="mt-1">
                {item.definition.resources.cpus} CPU ·{" "}
                {item.definition.resources.memory_mb} MB
              </dd>
            </div>
          </dl>
          {workspaceID && (
            <>
              {item.contributor_context && (
                <div className="mt-5 rounded-xl border border-[var(--line)] p-4">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <p className="text-xs font-semibold uppercase tracking-wide text-[var(--brand)]">
                        Contribution starting point
                      </p>
                      <h3 className="mt-1 font-semibold">
                        Opportunity context · pathway revision{" "}
                        {item.contributor_context.pathway_version}
                      </h3>
                    </div>
                    <Link
                      className="text-sm font-semibold text-[var(--brand)]"
                      href={`/repositories/${item.repository_id}/explanations?ref=${item.commit_id}&kind=workspace&resource_id=${item.id}`}
                    >
                      Ask a revision-grounded agent →
                    </Link>
                  </div>
                  <p className="mt-3 text-sm">
                    {item.contributor_context.guidance}
                  </p>
                  <p className="mt-4 text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">
                    Acceptance criteria
                  </p>
                  <ul className="mt-2 list-disc pl-5 text-sm">
                    {item.contributor_context.acceptance_criteria.map(
                      (criterion) => (
                        <li key={criterion}>{criterion}</li>
                      ),
                    )}
                  </ul>
                  <p className="mt-4 text-xs text-[var(--muted)]">
                    Evidence: {item.contributor_context.evidence_kind}{" "}
                    <code>{item.contributor_context.evidence_id}</code> ·
                    upstream remains read-only from this independently owned
                    fork.
                  </p>
                  {item.contributor_context.diagnostics.length > 0 && (
                    <div className="mt-3 rounded-lg bg-[var(--warning-soft)] p-3 text-sm text-[var(--warning)]">
                      {item.contributor_context.diagnostics.map(
                        (diagnostic) => (
                          <p key={diagnostic}>{diagnostic}</p>
                        ),
                      )}
                    </div>
                  )}
                </div>
              )}
              {item.contributor_context && (
                <ContributionHelp
                  workspace={item}
                  onWorkspace={(updated) => setItems([updated])}
                />
              )}
              {item.conflict_context && (
                <ConflictReconciliation
                  workspace={item}
                  onWorkspace={(updated) => setItems([updated])}
                />
              )}
              {item.conflict_context && (
                <ConflictMeaning
                  workspace={item}
                  onWorkspace={(updated) => setItems([updated])}
                />
              )}
              {item.conflict_context && (
                <ConflictVerification
                  workspace={item}
                  onWorkspace={(updated) => setItems([updated])}
                />
              )}
              <p className="mt-5 text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">
                Setup evidence
              </p>
              <ol className="mt-2 space-y-2">
                {item.setup_evidence.map((step, index) => (
                  <li
                    key={index}
                    className="rounded-lg bg-[var(--canvas)] p-3 text-xs"
                  >
                    <Badge
                      tone={step.state === "passed" ? "success" : "danger"}
                    >
                      {step.state}
                    </Badge>
                    <code className="ml-2">{step.command}</code>
                    {step.output && (
                      <pre className="mt-2 overflow-auto whitespace-pre-wrap text-[var(--muted)]">
                        {step.output}
                      </pre>
                    )}
                  </li>
                ))}
              </ol>
              <p className="mt-5 text-xs text-[var(--muted)]">
                Effective access: {item.effective_access.role} ·{" "}
                {item.effective_access.scopes.join(", ")}
              </p>
              <WorkspaceGovernance
                workspace={item}
                onWorkspace={(updated) =>
                  setItems((current) =>
                    current.map((value) =>
                      value.id === updated.id ? updated : value,
                    ),
                  )
                }
              />
              {!controlsLifecycle(item) && (
                <Button
                  className="mt-4"
                  disabled={pending}
                  onClick={() => void takeLifecycleControl(item)}
                >
                  Take lifecycle control
                </Button>
              )}
              {item.state === "running" && (
                <Button
                  className="mt-4"
                  disabled={pending || !controlsLifecycle(item)}
                  onClick={() => void transition(item, "suspend")}
                >
                  Suspend
                </Button>
              )}
              {item.state === "suspended" && (
                <Button
                  className="mt-4"
                  disabled={pending || !controlsLifecycle(item)}
                  onClick={() => void transition(item, "resume")}
                >
                  Resume exact foundation
                </Button>
              )}
            </>
          )}
          {workspaceID && item.state === "running" && (
            <>
              <WorkspaceCheckpoints
                workspace={item}
                onWorkspace={(updated) => setItems([updated])}
              />
              <WorkspaceIDE
                workspace={item}
                onWorkspace={(updated) => setItems([updated])}
              />
            </>
          )}
        </Card>
      ))}
    </div>
  );
}

function ConflictReconciliation({
  workspace,
  onWorkspace,
}: {
  workspace: DevelopmentWorkspace;
  onWorkspace: (workspace: DevelopmentWorkspace) => void;
}) {
  const { token, user } = useAuth();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const context = workspace.conflict_context!;
  async function invite(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    setError("");
    const form = event.currentTarget,
      data = new FormData(form),
      kind = String(data.get("kind")),
      principal = String(data.get("principal")),
      scopes = String(data.get("scopes"))
        .split(",")
        .map((x) => x.trim())
        .filter((x): x is "files" | "commands" | "lifecycle" =>
          ["files", "commands", "lifecycle"].includes(x),
        );
    try {
      let updated = await api<DevelopmentWorkspace>(
        `/workspaces/${workspace.id}/conflict-invitations`,
        {
          method: "POST",
          body: JSON.stringify({
            principal_kind: kind,
            principal_id: principal,
            role: String(data.get("role")),
          }),
        },
        token,
      );
      if (kind === "approved_agent") {
        updated = await api<DevelopmentWorkspace>(
          `/workspaces/${workspace.id}/control`,
          {
            method: "PUT",
            body: JSON.stringify({
              expected_version: updated.control.version,
              principal_kind: kind,
              principal_id: principal,
              mode: scopes.includes("commands") ? "execute" : "edit",
              scopes,
              expires_in: 900,
            }),
          },
          token,
        );
      }
      onWorkspace(updated);
      form.reset();
    } catch (reason) {
      setError(
        reason instanceof APIError
          ? reason.message
          : "Invitation could not be saved.",
      );
    } finally {
      setPending(false);
    }
  }
  async function respond(status: "accepted" | "declined") {
    if (!token) return;
    setPending(true);
    try {
      onWorkspace(
        await api<DevelopmentWorkspace>(
          `/workspaces/${workspace.id}/conflict-invitations/respond`,
          { method: "POST", body: JSON.stringify({ status }) },
          token,
        ),
      );
    } catch (reason) {
      setError(
        reason instanceof APIError
          ? reason.message
          : "Invitation response could not be saved.",
      );
    } finally {
      setPending(false);
    }
  }
  const mine = workspace.participants?.find(
    (x) =>
      x.principal_kind === "human" &&
      x.principal_id === user?.id &&
      x.status === "pending",
  );
  return (
    <section className="mt-5 rounded-xl border border-[var(--brand)] p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <p className="text-xs font-semibold uppercase tracking-wide text-[var(--brand)]">
            Conflict reconciliation
          </p>
          <h3 className="mt-1 font-semibold">
            Both immutable histories are here
          </h3>
        </div>
        <Badge tone="info">base {short(context.base_commit_id)}</Badge>
      </div>
      <div className="mt-4 grid gap-3 sm:grid-cols-2">
        {[context.source, context.target].map((side, index) => (
          <div
            key={side.commit_id}
            className="rounded-lg bg-[var(--canvas)] p-3 text-sm"
          >
            <b>
              {index === 0 ? "Source" : "Target"} · {side.branch}
            </b>
            <p className="mt-1 font-mono text-xs">{short(side.commit_id)}</p>
            <p className="mt-1 text-xs text-[var(--muted)]">
              Affected owners:{" "}
              {side.owner_ids.map(short).join(", ") || "not identified"}
            </p>
          </div>
        ))}
      </div>
      <p className="mt-3 text-sm">
        The checkout starts at <code>conflict/target</code>;{" "}
        <code>conflict/source</code> and the complete ancestry are preloaded
        locally. {context.files.length} overlapping file(s) and{" "}
        {context.affected_checks.length} affected check(s) are retained as
        launch evidence.
      </p>
      {context.incomplete.map((x) => (
        <p key={x} className="mt-2 text-xs text-[var(--warning)]">
          Incomplete evidence: {x}
        </p>
      ))}
      <div className="mt-4">
        <p className="text-xs font-semibold uppercase text-[var(--muted)]">
          Publication boundaries
        </p>
        {context.publication_targets.map((x) => (
          <p key={`${x.repository_id}-${x.branch}`} className="mt-1 text-xs">
            <code>{x.branch}</code> · {x.authority}
          </p>
        ))}
      </div>
      {mine && (
        <div className="mt-4 rounded-lg bg-[var(--warning-soft)] p-3 text-sm">
          <p>You were invited to reconcile this overlap.</p>
          <div className="mt-2 flex gap-2">
            <Button disabled={pending} onClick={() => void respond("accepted")}>
              Accept
            </Button>
            <Button
              variant="secondary"
              disabled={pending}
              onClick={() => void respond("declined")}
            >
              Decline
            </Button>
          </div>
        </div>
      )}
      <div className="mt-4 space-y-2">
        {workspace.participants?.map((x) => (
          <p key={`${x.principal_kind}-${x.principal_id}`} className="text-xs">
            <Badge tone={x.status === "accepted" ? "success" : "warning"}>
              {x.status}
            </Badge>{" "}
            <code>{short(x.principal_id)}</code> ·{" "}
            {x.principal_kind.replace("approved_", "")} · {x.role}
          </p>
        ))}
      </div>
      {workspace.creator_id === user?.id && (
        <form onSubmit={invite} className="mt-4 grid gap-2 sm:grid-cols-2">
          <label className="text-xs font-semibold">
            Participant type
            <select
              name="kind"
              className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-normal"
            >
              <option value="human">Affected owner</option>
              <option value="approved_agent">Approved agent</option>
            </select>
          </label>
          <label className="text-xs font-semibold">
            Principal ID
            <input
              name="principal"
              required
              className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-normal"
            />
          </label>
          <label className="text-xs font-semibold">
            Role
            <input
              name="role"
              defaultValue="resolution contributor"
              required
              className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-normal"
            />
          </label>
          <label className="text-xs font-semibold">
            Agent control scopes
            <input
              name="scopes"
              defaultValue="files, commands"
              className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-normal"
            />
            <span className="mt-1 block font-normal text-[var(--muted)]">
              Applied only to approved agents; files, commands, lifecycle.
            </span>
          </label>
          <Button disabled={pending}>Invite participant</Button>
        </form>
      )}
      {error && (
        <p role="alert" className="mt-2 text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
    </section>
  );
}

type ComparisonSide = {
  revision: string;
  content: string;
  sha256: string;
  missing: boolean;
};
type Comparison = {
  path: string;
  base: ComparisonSide;
  source: ComparisonSide;
  target: ComparisonSide;
  proposed: ComparisonSide;
};

function ConflictVerification({
  workspace,
  onWorkspace,
}: {
  workspace: DevelopmentWorkspace;
  onWorkspace: (workspace: DevelopmentWorkspace) => void;
}) {
  const { token, user } = useAuth();
  const context = workspace.conflict_context!;
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const path = context.files[0]?.path ?? "affected/path";
  const owner =
    context.source.owner_ids[0] ?? context.target.owner_ids[0] ?? "owner-id";
  const kinds = [
    "reproduction",
    "contract",
    "schema",
    "preview_acceptance",
    "conflict_test",
  ];
  const requiredChecks =
    context.affected_checks.length > 0 ? context.affected_checks : ["required"];
  const example = [
    ...requiredChecks.map((name) => ({
      kind: "required_check",
      name,
      origin: "both",
      command: "replace with the repository-defined command",
      exact_criteria: [`${name} remains satisfied`],
      coverage: [path],
      owner_ids: [owner],
      artifacts: [],
      cost: 0,
    })),
    ...kinds.map((kind) => ({
      kind,
      name: kind.replaceAll("_", " "),
      origin: "both",
      command: "replace with the repository-defined command",
      exact_criteria: [`${kind} remains satisfied`],
      coverage: [path],
      owner_ids: [owner],
      artifacts: [],
      cost: 0,
    })),
  ];
  async function checkpoint(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    const form = event.currentTarget,
      data = new FormData(form);
    setPending(true);
    try {
      const criteria = JSON.parse(String(data.get("criteria")));
      onWorkspace(
        await api<DevelopmentWorkspace>(
          `/workspaces/${workspace.id}/conflict-checkpoints`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: context.version,
              criteria,
            }),
          },
          token,
        ),
      );
      setError("");
    } catch (reason) {
      setError(
        reason instanceof APIError
          ? reason.message
          : "Checkpoint criteria must be valid JSON.",
      );
    } finally {
      setPending(false);
    }
  }
  async function decide(
    checkpointID: string,
    criterionID: string,
    decision: "accepted" | "rejected",
  ) {
    if (!token) return;
    const rationale = window.prompt(
      `${decision === "accepted" ? "Acceptance" : "Rejection"} rationale`,
    );
    if (!rationale) return;
    setPending(true);
    try {
      onWorkspace(
        await api<DevelopmentWorkspace>(
          `/workspaces/${workspace.id}/conflict-checkpoints/${checkpointID}/criteria/${criterionID}/decision`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: context.version,
              decision,
              rationale,
            }),
          },
          token,
        ),
      );
      setError("");
    } catch (reason) {
      setError(
        reason instanceof APIError
          ? reason.message
          : "Decision could not be retained.",
      );
    } finally {
      setPending(false);
    }
  }
  return (
    <section className="mt-5 rounded-xl border border-[var(--line)] p-4">
      <h3 className="font-semibold">Outcome verification checkpoints</h3>
      <p className="mt-1 text-sm text-[var(--muted)]">
        Assemble an immutable two-parent candidate and evaluate the promises
        accepted from both contributions. Commands execute only with live
        workspace control.
      </p>
      {(context.checkpoints ?? []).map((checkpoint) => (
        <article
          key={checkpoint.id}
          className="mt-3 rounded-lg bg-[var(--canvas)] p-3 text-sm"
        >
          <p className="font-mono text-xs">
            candidate {short(checkpoint.candidate_commit_id)} · tree{" "}
            {short(checkpoint.candidate_tree_id)}
          </p>
          <p className="mt-1 text-xs text-[var(--muted)]">
            source {short(checkpoint.source_revision)} · target{" "}
            {short(checkpoint.target_revision)} ·{" "}
            {new Date(checkpoint.created_at).toLocaleString()}
          </p>
          <ul className="mt-2 space-y-2">
            {checkpoint.criteria.map((criterion) => {
              const decision = checkpoint.decisions.find(
                (value) =>
                  value.criterion_id === criterion.id &&
                  value.owner_id === user?.id,
              );
              return (
                <li
                  key={criterion.id}
                  className="rounded border border-[var(--line)] p-2"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge
                      tone={criterion.state === "passed" ? "success" : "danger"}
                    >
                      {criterion.state}
                    </Badge>
                    <strong>{criterion.name}</strong>
                    <span className="text-xs text-[var(--muted)]">
                      {criterion.kind.replaceAll("_", " ")} · {criterion.origin}{" "}
                      · cost {criterion.cost.toFixed(4)}
                    </span>
                    {criterion.invalidated_by?.length ? (
                      <Badge tone="warning">
                        stale: {criterion.invalidated_by.join(", ")}
                      </Badge>
                    ) : null}
                  </div>
                  <code className="mt-2 block text-xs">
                    {criterion.command}
                  </code>
                  <p className="mt-1 text-xs">
                    Criteria: {criterion.exact_criteria.join("; ")} · Coverage:{" "}
                    {criterion.coverage.join(", ")}
                  </p>
                  {criterion.logs && (
                    <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap text-xs text-[var(--muted)]">
                      {criterion.logs}
                    </pre>
                  )}
                  {criterion.artifacts.map((artifact) => (
                    <p key={artifact.sha256} className="mt-1 font-mono text-xs">
                      {artifact.path} · sha256:{short(artifact.sha256)} ·{" "}
                      {artifact.size} bytes
                    </p>
                  ))}
                  {decision ? (
                    <p className="mt-2 text-xs">
                      Your decision: {decision.decision} — {decision.rationale}
                    </p>
                  ) : !criterion.invalidated_by?.length ? (
                    criterion.owner_ids.includes(user?.id ?? "") && (
                      <div className="mt-2 flex gap-2">
                        <Button
                          disabled={pending}
                          onClick={() =>
                            void decide(checkpoint.id, criterion.id, "accepted")
                          }
                        >
                          Accept behavior
                        </Button>
                        <Button
                          disabled={pending}
                          onClick={() =>
                            void decide(checkpoint.id, criterion.id, "rejected")
                          }
                        >
                          Reject behavior
                        </Button>
                      </div>
                    )
                  ) : null}
                </li>
              );
            })}
          </ul>
        </article>
      ))}
      <form className="mt-4 grid gap-2" onSubmit={checkpoint}>
        <textarea
          className="input min-h-48 font-mono text-xs"
          name="criteria"
          defaultValue={JSON.stringify(example, null, 2)}
          aria-label="Checkpoint criteria JSON"
          required
        />
        <Button disabled={pending}>Assemble and run checkpoint</Button>
      </form>
      {error && (
        <p role="alert" className="mt-2 text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
    </section>
  );
}

function ConflictMeaning({
  workspace,
  onWorkspace,
}: {
  workspace: DevelopmentWorkspace;
  onWorkspace: (workspace: DevelopmentWorkspace) => void;
}) {
  const { token } = useAuth(),
    context = workspace.conflict_context!,
    [comparison, setComparison] = useState<Comparison>(),
    [pending, setPending] = useState(false),
    [error, setError] = useState("");
  const [path, setPath] = useState(context.files[0]?.path || "");
  async function compare(next = path) {
    if (!token || !next) return;
    setPending(true);
    try {
      setComparison(
        await api<Comparison>(
          `/workspaces/${workspace.id}/conflict-comparison?path=${encodeURIComponent(next)}`,
          {},
          token,
        ),
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof APIError ? e.message : "Comparison could not be loaded.",
      );
    } finally {
      setPending(false);
    }
  }
  useEffect(() => {
    if (!token || !path) return;
    let active = true;
    void api<Comparison>(
      `/workspaces/${workspace.id}/conflict-comparison?path=${encodeURIComponent(path)}`,
      {},
      token,
    )
      .then((value) => {
        if (active) setComparison(value);
      })
      .catch((reason) => {
        if (active)
          setError(
            reason instanceof APIError
              ? reason.message
              : "Comparison could not be loaded.",
          );
      });
    return () => {
      active = false;
    };
  }, [token, workspace.id, path]);
  const citation = (side: "base" | "source" | "target" | "proposed") => ({
    side,
    revision:
      side === "proposed"
        ? `workspace:${workspace.id}`
        : side === "base"
          ? context.base_commit_id
          : context[side].commit_id,
    path,
  });
  async function question(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    const form = event.currentTarget,
      data = new FormData(form),
      side = String(data.get("side")) as
        "base" | "source" | "target" | "proposed";
    setPending(true);
    try {
      onWorkspace(
        await api<DevelopmentWorkspace>(
          `/workspaces/${workspace.id}/conflict-questions`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: context.version,
              body: String(data.get("body")),
              uncertainty: String(data.get("uncertainty")),
              citations: [citation(side)],
            }),
          },
          token,
        ),
      );
      form.reset();
    } catch (e) {
      setError(
        e instanceof APIError ? e.message : "Question could not be retained.",
      );
    } finally {
      setPending(false);
    }
  }
  async function answer(event: FormEvent<HTMLFormElement>, id: string) {
    event.preventDefault();
    if (!token) return;
    const form = event.currentTarget,
      data = new FormData(form),
      side = String(data.get("side")) as
        "base" | "source" | "target" | "proposed";
    setPending(true);
    try {
      onWorkspace(
        await api<DevelopmentWorkspace>(
          `/workspaces/${workspace.id}/conflict-questions/${id}/answer`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: context.version,
              body: String(data.get("body")),
              uncertainty: String(data.get("uncertainty")),
              citations: [citation(side)],
            }),
          },
          token,
        ),
      );
      form.reset();
    } catch (e) {
      setError(
        e instanceof APIError ? e.message : "Answer could not be retained.",
      );
    } finally {
      setPending(false);
    }
  }
  async function propose(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token || !comparison) return;
    const form = event.currentTarget,
      data = new FormData(form),
      side = String(data.get("side")) as
        "base" | "source" | "target" | "proposed";
    setPending(true);
    try {
      onWorkspace(
        await api<DevelopmentWorkspace>(
          `/workspaces/${workspace.id}/conflict-resolutions`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: context.version,
              path,
              summary: String(data.get("summary")),
              proposed_content: String(data.get("content")),
              expected_sha256: comparison.proposed.sha256,
              uncertainty: String(data.get("uncertainty")),
              preservation: [
                {
                  kind: String(data.get("kind")),
                  reference: String(data.get("reference")),
                  disposition: String(data.get("disposition")),
                  rationale: String(data.get("rationale")),
                  citations: [citation(side)],
                },
              ],
            }),
          },
          token,
        ),
      );
      form.reset();
    } catch (e) {
      setError(
        e instanceof APIError ? e.message : "Resolution could not be proposed.",
      );
    } finally {
      setPending(false);
    }
  }
  async function act(id: string, action: "apply" | "undo") {
    if (!token) return;
    setPending(true);
    try {
      const updated = await api<DevelopmentWorkspace>(
        `/workspaces/${workspace.id}/conflict-resolutions/${id}/${action}`,
        {
          method: "POST",
          body: JSON.stringify({ expected_version: context.version }),
        },
        token,
      );
      onWorkspace(updated);
      await compare();
    } catch (e) {
      setError(e instanceof APIError ? e.message : "Resolution action failed.");
    } finally {
      setPending(false);
    }
  }
  const field =
    "mt-1 w-full rounded-lg border border-[var(--line)] bg-white p-2 font-normal";
  return (
    <section className="mt-5 rounded-xl border border-[var(--line)] p-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-xs font-semibold uppercase tracking-wide text-[var(--brand)]">
            Meaning ledger · v{context.version}
          </p>
          <h3 className="mt-1 font-semibold">
            Compare intent, ask, and resolve
          </h3>
          <p className="mt-1 text-xs text-[var(--muted)]">
            Every claim cites frozen evidence. Applying changes requires the
            visible file-control lease and grants no merge, secret, or
            environment authority.
          </p>
        </div>
        <label className="text-xs font-semibold">
          Overlapping file
          <select
            className={field}
            value={path}
            onChange={(e) => setPath(e.target.value)}
          >
            {context.files.map((file) => (
              <option key={file.path}>{file.path}</option>
            ))}
          </select>
        </label>
      </div>
      {comparison && (
        <div className="mt-4 grid gap-2 lg:grid-cols-4">
          {(["base", "source", "target", "proposed"] as const).map((side) => (
            <div
              key={side}
              className="min-w-0 rounded-lg bg-[var(--canvas)] p-3"
            >
              <p className="text-xs font-semibold uppercase">
                {side} · {comparison[side].revision.slice(0, 12)}
              </p>
              <pre className="mt-2 max-h-56 overflow-auto whitespace-pre-wrap text-xs">
                {comparison[side].missing
                  ? "File absent"
                  : comparison[side].content}
              </pre>
            </div>
          ))}
        </div>
      )}
      <div className="mt-5 grid gap-4 lg:grid-cols-2">
        <form
          onSubmit={question}
          className="rounded-lg border border-[var(--line)] p-3"
        >
          <h4 className="font-semibold">Ask a revision-grounded question</h4>
          <textarea
            name="body"
            required
            className={field}
            placeholder="Which behavior is intentional here?"
          />
          <input
            name="uncertainty"
            required
            className={field}
            placeholder="What remains uncertain?"
          />
          <select name="side" className={field}>
            <option>source</option>
            <option>target</option>
            <option>base</option>
            <option>proposed</option>
          </select>
          <Button className="mt-2" disabled={pending}>
            Retain question
          </Button>
        </form>
        <form
          onSubmit={propose}
          className="rounded-lg border border-[var(--line)] p-3"
        >
          <h4 className="font-semibold">Propose a bounded resolution</h4>
          <input
            name="summary"
            required
            className={field}
            placeholder="How this combines both outcomes"
          />
          <textarea
            name="content"
            required
            defaultValue={comparison?.proposed.content}
            className={`${field} min-h-32 font-mono text-xs`}
          />
          <div className="grid grid-cols-2 gap-2">
            <select name="kind" className={field}>
              <option value="acceptance_criterion">Acceptance criterion</option>
              <option value="design_decision">Design decision</option>
              <option value="migration">Migration</option>
              <option value="user_behavior">User behavior</option>
            </select>
            <select name="disposition" className={field}>
              <option value="preserved">Preserved</option>
              <option value="intentionally_changed">
                Intentionally changed
              </option>
            </select>
          </div>
          <input
            name="reference"
            required
            className={field}
            placeholder="Named criterion or decision"
          />
          <input
            name="rationale"
            required
            className={field}
            placeholder="Why this edit preserves or changes it"
          />
          <input
            name="uncertainty"
            required
            className={field}
            placeholder="Residual uncertainty"
          />
          <select name="side" className={field}>
            <option>source</option>
            <option>target</option>
            <option>base</option>
            <option>proposed</option>
          </select>
          <Button
            className="mt-2"
            disabled={pending || !comparison?.proposed.sha256}
          >
            Propose without applying
          </Button>
        </form>
      </div>
      <div className="mt-5 space-y-3">
        {context.questions.map((q) => (
          <article
            key={q.id}
            className="rounded-lg bg-[var(--canvas)] p-3 text-sm"
          >
            <p>
              <b>Question:</b> {q.body}
            </p>
            <p className="mt-1 text-xs text-[var(--muted)]">
              {q.authorship.agent_id
                ? `Agent ${short(q.authorship.agent_id)} operated by ${short(q.authorship.actor_id)}`
                : `Human ${short(q.authorship.actor_id)}`}{" "}
              · uncertainty: {q.uncertainty}
            </p>
            {q.answer ? (
              <p className="mt-2">
                <b>Answer:</b> {q.answer.body}{" "}
                <span className="text-xs text-[var(--muted)]">
                  ({q.answer.uncertainty})
                </span>
              </p>
            ) : (
              <form
                className="mt-2 grid gap-2 sm:grid-cols-4"
                onSubmit={(e) => void answer(e, q.id)}
              >
                <input
                  name="body"
                  required
                  className={field}
                  placeholder="Grounded answer"
                />
                <input
                  name="uncertainty"
                  required
                  className={field}
                  placeholder="Remaining uncertainty"
                />
                <select name="side" className={field}>
                  <option>source</option>
                  <option>target</option>
                  <option>base</option>
                  <option>proposed</option>
                </select>
                <Button disabled={pending}>Answer</Button>
              </form>
            )}
          </article>
        ))}
        {context.resolutions.map((r) => (
          <article
            key={r.id}
            className="rounded-lg border border-[var(--line)] p-3 text-sm"
          >
            <div className="flex justify-between gap-3">
              <div>
                <Badge
                  tone={
                    r.state === "applied"
                      ? "success"
                      : r.state === "undone"
                        ? "warning"
                        : "info"
                  }
                >
                  {r.state}
                </Badge>
                <b className="ml-2">{r.summary}</b>
                <p className="mt-2 text-xs">
                  {r.preservation
                    .map(
                      (x) =>
                        `${x.kind.replaceAll("_", " ")}: ${x.reference} — ${x.disposition}`,
                    )
                    .join("; ")}
                </p>
                <p className="mt-1 text-xs text-[var(--muted)]">
                  Uncertainty: {r.uncertainty} ·{" "}
                  {r.authorship.agent_id
                    ? `agent ${short(r.authorship.agent_id)} / operator ${short(r.authorship.actor_id)}`
                    : `human ${short(r.authorship.actor_id)}`}
                </p>
              </div>
              {r.state !== "undone" && (
                <Button
                  disabled={pending}
                  onClick={() =>
                    void act(
                      r.id,
                      r.state === "proposed" || r.state === "applying"
                        ? "apply"
                        : "undo",
                    )
                  }
                >
                  {r.state === "proposed"
                    ? "Apply with control"
                    : r.state === "applying"
                      ? "Reconcile apply"
                      : r.state === "undoing"
                        ? "Reconcile undo"
                        : "Undo edit"}
                </Button>
              )}
            </div>
          </article>
        ))}
      </div>
      {error && (
        <p role="alert" className="mt-3 text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
    </section>
  );
}

function WorkspaceGovernance({
  workspace,
  onWorkspace,
}: {
  workspace: DevelopmentWorkspace;
  onWorkspace: (workspace: DevelopmentWorkspace) => void;
}) {
  const { token } = useAuth();
  const [usage, setUsage] = useState<WorkspaceConsumption>();
  const [currentPolicy, setCurrentPolicy] = useState<WorkspacePolicy>(
    workspace.policy,
  );
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const owner = workspace.effective_access.role === "owner";
  useEffect(() => {
    if (!token || !owner) return;
    void Promise.all([
      api<{ items: WorkspaceConsumption[] }>(
        `/repositories/${workspace.repository_id}/workspace-usage`,
        {},
        token,
      ),
      api<WorkspacePolicy>(
        `/repositories/${workspace.repository_id}/workspace-policy`,
        {},
        token,
      ),
    ])
      .then(([v, p]) => {
        setUsage(v.items.find((x) => x.workspace_id === workspace.id));
        setCurrentPolicy(p);
      })
      .catch(() => {});
  }, [token, owner, workspace.id, workspace.repository_id]);
  async function savePolicy(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    const data = new FormData(event.currentTarget);
    const input: Omit<WorkspacePolicy, "updated_by" | "updated_at"> = {
      version: currentPolicy.version,
      max_cpus: Number(data.get("max_cpus")),
      max_memory_mb: Number(data.get("max_memory_mb")),
      max_storage_mb: Number(data.get("max_storage_mb")),
      network: "none",
      idle_minutes: Number(data.get("idle_minutes")),
      max_runtime_hours: Number(data.get("max_runtime_hours")),
      retention_hours: Number(data.get("retention_hours")),
      sharing: String(data.get("sharing")) as WorkspacePolicy["sharing"],
      agent_execution: data.get("agent_execution") === "on",
    };
    try {
      setCurrentPolicy(
        await api<WorkspacePolicy>(
          `/repositories/${workspace.repository_id}/workspace-policy`,
          {
            method: "PUT",
            body: JSON.stringify({
              ...input,
              expected_version: currentPolicy.version,
            }),
          },
          token,
        ),
      );
    } catch (e) {
      setError(
        e instanceof APIError ? e.message : "Policy could not be saved.",
      );
    } finally {
      setPending(false);
    }
  }
  async function act(kind: "stop" | "expiry") {
    if (!token) return;
    setPending(true);
    try {
      const body =
        kind === "expiry"
          ? {
              expires_at: new Date(
                Date.now() + 24 * 60 * 60 * 1000,
              ).toISOString(),
              reason:
                "Owner-announced expiry; export unpublished work before this time.",
            }
          : { reason: "Stopped by repository owner" };
      onWorkspace(
        await api<DevelopmentWorkspace>(
          `/workspaces/${workspace.id}/${kind}`,
          { method: "POST", body: JSON.stringify(body) },
          token,
        ),
      );
    } catch (e) {
      setError(
        e instanceof APIError
          ? e.message
          : "Workspace governance update failed.",
      );
    } finally {
      setPending(false);
    }
  }
  return (
    <section className="mt-5 rounded-lg border border-[var(--line)] p-4">
      <h3 className="font-semibold">Governance and consumption</h3>
      <p className="mt-1 text-xs text-[var(--muted)]">
        Launch policy {workspace.policy_scope} v{workspace.policy_version} ·{" "}
        {workspace.policy.sharing} sharing · network {workspace.policy.network}{" "}
        · idle after {workspace.policy.idle_minutes} minutes
      </p>
      {workspace.rebuild_required && (
        <p className="mt-2 text-sm text-[var(--warning)]">
          Rebuild required: {workspace.rebuild_reasons.join("; ")}
        </p>
      )}
      {workspace.expires_at && (
        <p className="mt-2 text-sm">
          Expiry: {new Date(workspace.expires_at).toLocaleString()}
          {workspace.expiry_announced_at ? " (announced)" : ""}
        </p>
      )}
      {workspace.stop_reason && (
        <p className="mt-2 text-sm text-[var(--danger)]">
          {workspace.stop_reason}
        </p>
      )}
      {usage && (
        <dl className="mt-3 grid gap-2 text-xs sm:grid-cols-3">
          <div>
            <dt>CPU consumed</dt>
            <dd>{Math.round(usage.cpu_seconds)} seconds</dd>
          </div>
          <div>
            <dt>Memory reserved</dt>
            <dd>{usage.memory_mb_hours.toFixed(1)} MB-hours</dd>
          </div>
          <div>
            <dt>Storage reserved</dt>
            <dd>{usage.storage_mb_hours.toFixed(1)} MB-hours</dd>
          </div>
        </dl>
      )}
      {owner && (
        <>
          <form
            onSubmit={savePolicy}
            className="mt-4 grid gap-2 sm:grid-cols-3"
          >
            {[
              ["max_cpus", "Max CPUs", currentPolicy.max_cpus],
              ["max_memory_mb", "Memory MB", currentPolicy.max_memory_mb],
              ["max_storage_mb", "Storage MB", currentPolicy.max_storage_mb],
              ["idle_minutes", "Idle minutes", currentPolicy.idle_minutes],
              [
                "max_runtime_hours",
                "Runtime hours",
                currentPolicy.max_runtime_hours,
              ],
              [
                "retention_hours",
                "Retention hours",
                currentPolicy.retention_hours,
              ],
            ].map(([name, label, value]) => (
              <label className="text-xs font-semibold" key={String(name)}>
                {label}
                <input
                  name={String(name)}
                  type="number"
                  step={name === "max_cpus" ? "0.25" : "1"}
                  defaultValue={value}
                  className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-normal"
                />
              </label>
            ))}
            <label className="text-xs font-semibold">
              Sharing
              <select
                name="sharing"
                defaultValue={currentPolicy.sharing}
                className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-normal"
              >
                <option value="private">Creator only</option>
                <option value="repository">Repository collaborators</option>
                <option value="organization">Organization</option>
              </select>
            </label>
            <label className="flex items-center gap-2 text-xs font-semibold">
              <input
                name="agent_execution"
                type="checkbox"
                defaultChecked={currentPolicy.agent_execution}
              />{" "}
              Allow approved agents
            </label>
            <Button disabled={pending}>Save repository policy</Button>
          </form>
          <div className="mt-3 flex gap-2">
            <Button
              disabled={pending || !!workspace.expiry_announced_at}
              onClick={() => void act("expiry")}
            >
              Announce 24-hour expiry
            </Button>
            <Button
              disabled={
                pending ||
                workspace.state === "stopped" ||
                workspace.state === "expired"
              }
              onClick={() => void act("stop")}
            >
              Stop compute
            </Button>
          </div>
        </>
      )}
      {error && (
        <p role="alert" className="mt-2 text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
    </section>
  );
}

export function WorkspaceLauncher() {
  const { token } = useAuth();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    const data = new FormData(event.currentTarget);
    const kind = String(data.get("kind"));
    const source: Record<string, string> = { kind };
    for (const key of [
      "proposal_id",
      "task_id",
      "pull_request_id",
      "incident_id",
      "repair_id",
    ]) {
      const value = String(data.get(key) || "").trim();
      if (value) source[key] = value;
    }
    try {
      const created = await api<DevelopmentWorkspace>(
        "/workspaces",
        {
          method: "POST",
          body: JSON.stringify({
            repository_id: String(data.get("repository_id")),
            commit_id: String(data.get("commit_id")),
            source,
          }),
        },
        token,
      );
      location.href = `/workspaces/${created.id}`;
    } catch (e) {
      setError(
        e instanceof APIError ? e.message : "Workspace could not be launched.",
      );
      setPending(false);
    }
  }
  return (
    <Card className="p-6">
      <form onSubmit={submit} className="grid gap-4">
        <label className="text-sm font-semibold">
          Repository ID
          <input
            required
            name="repository_id"
            className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-mono font-normal"
          />
        </label>
        <label className="text-sm font-semibold">
          Exact commit
          <input
            required
            name="commit_id"
            className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-mono font-normal"
          />
        </label>
        <label className="text-sm font-semibold">
          Shared context
          <select
            name="kind"
            className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-normal"
          >
            <option value="repository">Repository</option>
            <option value="proposal_task">Proposal task</option>
            <option value="pull_request">Pull request</option>
            <option value="incident_repair">Incident repair</option>
          </select>
        </label>
        <div className="grid gap-3 sm:grid-cols-2">
          {[
            "proposal_id",
            "task_id",
            "pull_request_id",
            "incident_id",
            "repair_id",
          ].map((name) => (
            <label key={name} className="text-xs font-semibold capitalize">
              {name.replaceAll("_", " ")}
              <input
                name={name}
                className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-mono font-normal"
              />
            </label>
          ))}
        </div>
        {error && (
          <p role="alert" className="text-sm text-[var(--danger)]">
            {error}
          </p>
        )}
        <Button disabled={pending}>
          {pending
            ? "Provisioning exact revision…"
            : "Launch isolated workspace"}
        </Button>
      </form>
    </Card>
  );
}
