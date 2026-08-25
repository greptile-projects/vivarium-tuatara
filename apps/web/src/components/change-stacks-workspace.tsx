"use client";
import Link from "next/link";
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

type Diagnostic = { code: string; message: string; blocking: boolean };
type Scope = {
  commit_count: number;
  files: string[];
  additions: number;
  deletions: number;
};
type Member = {
  id: string;
  position: number;
  title: string;
  source_repository_id?: string;
  source_branch: string;
  pull_request_id?: string;
  revision?: string;
  base_revision?: string;
  expected_base_revision?: string;
  depends_on?: string[];
  acceptance_criteria: string[];
  authors: string[];
  individual_scope: Scope;
  cumulative_scope: Scope;
  effective_permissions: {
    read: boolean;
    publish: boolean;
    review: boolean;
    push: boolean;
  };
  diagnostics: Diagnostic[];
  review_state: string;
};
type RestackMember = {
  member: Member;
  action: string;
  old_revision?: string;
  candidate_revision?: string;
  candidate_base?: string;
  impact: { reviews_invalidated: number; checks_invalidated: number };
  published_branch_update: boolean;
  diagnostics: Diagnostic[];
};
type Restack = {
  id: string;
  status: string;
  members: RestackMember[];
  removed_members?: Member[];
  diagnostics: Diagnostic[];
  authority: string;
};
type Assignment = {
  id: string;
  member_id: string;
  principal_type: "human" | "agent";
  principal_id: string;
  status: string;
};
type WorkLaunch = {
  id: string;
  member_id: string;
  kind: string;
  revision: string;
  parent_revision: string;
  current_upstream: boolean;
  changed_upstream?: string[];
  acceptance_criteria: string[];
  authority: string;
};
type TimelineEvent = {
  id: string;
  member_id: string;
  kind: string;
  summary: string;
  actor_id: string;
  actor_type: string;
  current_upstream: boolean;
  changed_upstream?: string[];
  created_at: string;
};
type IntegrationCandidate = {
  id: string;
  position: number;
  target_revision: string;
  candidate_revision: string;
  required_checks: string[];
  status: string;
  superseded_reason?: string;
  merged_by?: string;
  merged_at?: string;
};
type Stack = {
  id: string;
  title: string;
  outcome: string;
  target_branch: string;
  target_revision?: string;
  members: Member[];
  diagnostics: Diagnostic[];
  restacks?: Restack[];
  assignments?: Assignment[];
  work_launches?: WorkLaunch[];
  timeline?: TimelineEvent[];
  integration_candidates?: IntegrationCandidate[];
  created_by: string;
  created_at: string;
  authority: string;
};
const lines = (v: FormDataEntryValue | null) =>
  String(v ?? "")
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
export function ChangeStacksWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token } = useAuth(),
    [items, setItems] = useState<Stack[]>([]),
    [error, setError] = useState(""),
    [saving, setSaving] = useState(false),
    requestID = useRef(crypto.randomUUID());
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const x = await api<{ change_stacks: Stack[] }>(
        `/repositories/${repositoryID}/change-stacks`,
        {},
        token,
      );
      setItems(x.change_stacks);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Stacks could not be loaded.");
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    const form = event.currentTarget,
      f = new FormData(form);
    try {
      const members = lines(f.get("members")).map((line, index) => {
        const [id, title, branch, pull = "", dependencies = "", criteria = ""] =
          line.split("|").map((x) => x.trim());
        return {
          id: id || `change-${index + 1}`,
          title,
          source_branch: branch,
          pull_request_id: pull,
          depends_on: dependencies
            .split(",")
            .map((x) => x.trim())
            .filter(Boolean),
          acceptance_criteria: criteria
            .split(";")
            .map((x) => x.trim())
            .filter(Boolean),
        };
      });
      const out = await api<Stack>(
        `/repositories/${repositoryID}/change-stacks`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: requestID.current,
            title: String(f.get("title")),
            outcome: String(f.get("outcome")),
            target_branch: String(f.get("target")),
            members,
          }),
        },
        token,
      );
      setItems((x) => [out, ...x]);
      requestID.current = crypto.randomUUID();
      form.reset();
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Stack could not be published.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function previewRestack(
    stack: Stack,
    event: FormEvent<HTMLFormElement>,
  ) {
    event.preventDefault();
    setSaving(true);
    const f = new FormData(event.currentTarget);
    try {
      const members = lines(f.get("restack-members")).map((line) => {
        const [id, title, branch, pull = "", dependencies = "", criteria = ""] =
          line.split("|").map((x) => x.trim());
        return {
          id,
          title,
          source_branch: branch,
          pull_request_id: pull,
          depends_on: dependencies
            .split(",")
            .map((x) => x.trim())
            .filter(Boolean),
          acceptance_criteria: criteria
            .split(";")
            .map((x) => x.trim())
            .filter(Boolean),
        };
      });
      await api(
        `/repositories/${repositoryID}/change-stacks/${stack.id}/restacks`,
        {
          method: "POST",
          body: JSON.stringify({ request_id: crypto.randomUUID(), members }),
        },
        token,
      );
      await load();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Restack could not be previewed.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function applyRestack(stackID: string, restackID: string) {
    setSaving(true);
    try {
      await api(
        `/repositories/${repositoryID}/change-stacks/${stackID}/restacks/${restackID}/apply`,
        { method: "POST" },
        token,
      );
      await load();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Restack could not be applied.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function assign(
    stackID: string,
    memberID: string,
    event: FormEvent<HTMLFormElement>,
  ) {
    event.preventDefault();
    setSaving(true);
    const f = new FormData(event.currentTarget);
    try {
      await api(
        `/repositories/${repositoryID}/change-stacks/${stackID}/members/${memberID}/assignments`,
        {
          method: "POST",
          body: JSON.stringify({
            principal_type: String(f.get("principal_type")),
            principal_id: String(f.get("principal_id")),
            access_grant_id: String(f.get("access_grant_id") || ""),
          }),
        },
        token,
      );
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Layer could not be assigned.");
    } finally {
      setSaving(false);
    }
  }
  async function openWork(
    stackID: string,
    memberID: string,
    assignmentID: string,
    kind: string,
  ) {
    setSaving(true);
    try {
      await api(
        `/repositories/${repositoryID}/change-stacks/${stackID}/members/${memberID}/work-launches`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: crypto.randomUUID(),
            assignment_id: assignmentID,
            kind,
          }),
        },
        token,
      );
      await load();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Scoped work could not be opened.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function publishEvent(
    stack: Stack,
    member: Member,
    event: FormEvent<HTMLFormElement>,
  ) {
    event.preventDefault();
    setSaving(true);
    const f = new FormData(event.currentTarget);
    try {
      await api(
        `/repositories/${repositoryID}/change-stacks/${stack.id}/timeline`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: crypto.randomUUID(),
            member_id: member.id,
            revision: member.revision,
            kind: String(f.get("kind")),
            summary: String(f.get("summary")),
            work_launch_id: String(f.get("work_launch_id") || ""),
            restack_id: String(f.get("restack_id") || ""),
            from_principal_id: String(f.get("from_principal_id") || ""),
            to_principal_id: String(f.get("to_principal_id") || ""),
          }),
        },
        token,
      );
      await load();
      event.currentTarget.reset();
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Timeline event could not be published.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function prepareIntegration(stackID: string, position: number) {
    setSaving(true);
    try {
      await api(
        `/repositories/${repositoryID}/change-stacks/${stackID}/integration-candidates`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: crypto.randomUUID(),
            through_position: position,
          }),
        },
        token,
      );
      await load();
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Integration candidate could not be prepared.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function mergeIntegration(stackID: string, candidateID: string) {
    setSaving(true);
    try {
      await api(
        `/repositories/${repositoryID}/change-stacks/${stackID}/integration-candidates/${candidateID}/merge`,
        { method: "POST" },
        token,
      );
      await load();
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Verified stack prefix could not be integrated.",
      );
    } finally {
      setSaving(false);
    }
  }
  return (
    <main className="mx-auto max-w-6xl space-y-7 p-5 sm:p-8">
      <header>
        <Badge tone="info">Collaborative delivery</Badge>
        <h1 className="mt-3 text-3xl font-semibold">Change stacks</h1>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-[var(--muted)]">
          Publish exact review layers, then preview and atomically apply
          authorized restacks when upstream feedback changes the sequence.
        </p>
      </header>
      <Card className="p-6">
        <h2 className="text-lg font-semibold">Publish an ordered stack</h2>
        <form onSubmit={create} className="mt-4 grid gap-4 md:grid-cols-2">
          <label className="text-sm font-semibold">
            Stack title
            <input
              name="title"
              required
              maxLength={200}
              className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal"
            />
          </label>
          <label className="text-sm font-semibold">
            Target branch
            <input
              name="target"
              required
              defaultValue="main"
              className="mt-1 min-h-10 w-full rounded-lg border px-3 font-mono font-normal"
            />
          </label>
          <label className="text-sm font-semibold md:col-span-2">
            Shared outcome
            <textarea
              name="outcome"
              required
              rows={3}
              className="mt-1 w-full rounded-lg border p-3 font-normal"
            />
          </label>
          <label className="text-sm font-semibold md:col-span-2">
            Ordered changes{" "}
            <span className="font-normal text-[var(--muted)]">
              — one per line: id | title | branch | existing pull ID (optional)
              | dependency IDs | criteria separated by ;
            </span>
            <textarea
              name="members"
              required
              rows={6}
              placeholder="schema | Add storage contract | feature/schema | | | migration is reversible; tests pass&#10;api | Expose the endpoint | feature/api | | schema | API preserves old clients"
              className="mt-1 w-full rounded-lg border p-3 font-mono text-xs font-normal"
            />
          </label>
          <div>
            <Button disabled={saving}>
              {saving ? "Publishing…" : "Publish exact revisions"}
            </Button>
          </div>
        </form>
        {error && (
          <p
            role="alert"
            className="mt-4 rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
          >
            {error}
          </p>
        )}
      </Card>
      <section className="space-y-5" aria-label="Published change stacks">
        {items.map((stack) => (
          <Card key={stack.id} className="overflow-hidden">
            <div className="border-b p-5">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="text-xl font-semibold">{stack.title}</h2>
                <Badge>{stack.members.length} changes</Badge>
                <Badge
                  tone={
                    stack.diagnostics.some((x) => x.blocking)
                      ? "warning"
                      : "success"
                  }
                >
                  {stack.diagnostics.some((x) => x.blocking)
                    ? "needs attention"
                    : "reviewable"}
                </Badge>
              </div>
              <p className="mt-2 text-sm">{stack.outcome}</p>
              <p className="mt-2 font-mono text-xs text-[var(--muted)]">
                {stack.target_branch} @{" "}
                {stack.target_revision?.slice(0, 12) || "missing"}
              </p>
            </div>
            <ol className="divide-y">
              {stack.members.map((member) => {
                const assignment = stack.assignments?.find(
                  (a) => a.member_id === member.id && a.status === "active",
                );
                return (
                  <li key={member.id} className="p-5">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge>{member.position}</Badge>
                      <h3 className="font-semibold">{member.title}</h3>
                      <Badge
                        tone={
                          member.diagnostics.some((x) => x.blocking)
                            ? "warning"
                            : "success"
                        }
                      >
                        {member.review_state || "blocked"}
                      </Badge>
                      {assignment && (
                        <Badge tone="info">
                          {assignment.principal_type}: {assignment.principal_id}
                        </Badge>
                      )}
                      {member.pull_request_id && (
                        <Link
                          className="text-xs font-semibold text-[var(--brand)] hover:underline"
                          href={`/pulls/${repositoryID}/${member.pull_request_id}`}
                        >
                          Pull request
                        </Link>
                      )}
                    </div>
                    <p className="mt-2 font-mono text-xs text-[var(--muted)]">
                      {member.base_revision?.slice(0, 10) || "?"} →{" "}
                      {member.revision?.slice(0, 10) || "missing"} ·{" "}
                      {member.source_branch}
                    </p>
                    <div className="mt-3 grid gap-3 sm:grid-cols-2">
                      <ScopeView
                        label="This change"
                        scope={member.individual_scope}
                      />
                      <ScopeView
                        label="Cumulative outcome"
                        scope={member.cumulative_scope}
                      />
                    </div>
                    <p className="mt-3 text-xs">
                      <strong>Authors:</strong>{" "}
                      {member.authors?.join(", ") || "unavailable"}
                    </p>
                    <p className="mt-1 text-xs">
                      <strong>Depends on:</strong>{" "}
                      {member.depends_on?.join(", ") || "target"}
                    </p>
                    <p className="mt-1 text-xs">
                      <strong>Acceptance:</strong>{" "}
                      {member.acceptance_criteria.join(" · ")}
                    </p>
                    <Button className="mt-3" variant="secondary" disabled={saving} onClick={() => prepareIntegration(stack.id, member.position)}>
                      Prepare prefix through layer {member.position}
                    </Button>
                    <div className="mt-2 flex flex-wrap gap-1">
                      {Object.entries(member.effective_permissions).map(
                        ([key, value]) => (
                          <Badge key={key} tone={value ? "success" : "neutral"}>
                            {key}: {value ? "yes" : "no"}
                          </Badge>
                        ),
                      )}
                    </div>
                    {member.diagnostics.map((d) => (
                      <p
                        key={d.code + d.message}
                        className="mt-2 rounded bg-[var(--warning-soft)] p-2 text-xs text-[var(--warning)]"
                      >
                        <strong>{d.code.replaceAll("_", " ")}:</strong>{" "}
                        {d.message}
                      </p>
                    ))}
                    <form
                      onSubmit={(event) => assign(stack.id, member.id, event)}
                      className="mt-4 grid gap-2 rounded-lg border p-3 sm:grid-cols-4"
                    >
                      <select
                        name="principal_type"
                        className="rounded-lg border px-2 text-xs"
                      >
                        <option value="human">Human</option>
                        <option value="agent">Approved agent</option>
                      </select>
                      <input
                        name="principal_id"
                        required
                        placeholder="Principal ID"
                        className="rounded-lg border px-2 text-xs"
                      />
                      <input
                        name="access_grant_id"
                        placeholder="Agent grant ID"
                        className="rounded-lg border px-2 text-xs"
                      />
                      <Button disabled={saving}>Assign layer</Button>
                    </form>
                    {assignment && (
                      <div className="mt-3 flex flex-wrap gap-2">
                        <Button
                          disabled={saving}
                          onClick={() =>
                            openWork(
                              stack.id,
                              member.id,
                              assignment.id,
                              "change_session",
                            )
                          }
                        >
                          Open session
                        </Button>
                        <Button
                          disabled={saving}
                          onClick={() =>
                            openWork(
                              stack.id,
                              member.id,
                              assignment.id,
                              "shared_workspace",
                            )
                          }
                        >
                          Open shared workspace
                        </Button>
                        <Button
                          disabled={saving}
                          onClick={() =>
                            openWork(
                              stack.id,
                              member.id,
                              assignment.id,
                              "conflict_resolution_workspace",
                            )
                          }
                        >
                          Open conflict workspace
                        </Button>
                      </div>
                    )}
                    <form
                      onSubmit={(event) => publishEvent(stack, member, event)}
                      className="mt-3 grid gap-2 rounded-lg bg-[var(--surface-subtle)] p-3 sm:grid-cols-4"
                    >
                      <select
                        name="kind"
                        className="rounded-lg border px-2 text-xs"
                      >
                        <option value="checkpoint">Checkpoint</option>
                        <option value="question">Question</option>
                        <option value="handoff">Handoff</option>
                        <option value="restack_proposal">
                          Restack proposal
                        </option>
                      </select>
                      <input
                        name="summary"
                        required
                        maxLength={2000}
                        placeholder="What changed or needs attention?"
                        className="rounded-lg border px-2 text-xs sm:col-span-2"
                      />
                      <Button disabled={saving}>Add to timeline</Button>
                    </form>
                    {stack.work_launches
                      ?.filter((x) => x.member_id === member.id)
                      .map((x) => (
                        <div
                          key={x.id}
                          className="mt-3 rounded-lg border p-3 text-xs"
                        >
                          <Badge
                            tone={x.current_upstream ? "success" : "warning"}
                          >
                            {x.current_upstream
                              ? "upstream current"
                              : "upstream changed"}
                          </Badge>
                          <strong className="ml-2">
                            {x.kind.replaceAll("_", " ")}
                          </strong>
                          <p className="mt-1 font-mono">
                            {x.parent_revision.slice(0, 8)} →{" "}
                            {x.revision.slice(0, 8)}
                          </p>
                          {!x.current_upstream && (
                            <p className="mt-1 text-[var(--warning)]">
                              Changed assumptions:{" "}
                              {x.changed_upstream?.join(", ")}
                            </p>
                          )}
                        </div>
                      ))}
                  </li>
                );
              })}
            </ol>
            {Boolean(stack.timeline?.length) && (
              <div className="border-t p-5">
                <h3 className="font-semibold">Stack timeline</h3>
                {stack.timeline?.map((event) => (
                  <div key={event.id} className="mt-3 border-l-2 pl-3 text-sm">
                    <div className="flex gap-2">
                      <Badge
                        tone={event.current_upstream ? "neutral" : "warning"}
                      >
                        {event.kind.replaceAll("_", " ")}
                      </Badge>
                      <span className="text-xs text-[var(--muted)]">
                        {event.actor_type} {event.actor_id}
                      </span>
                    </div>
                    <p className="mt-1">{event.summary}</p>
                    {!event.current_upstream && (
                      <p className="text-xs text-[var(--warning)]">
                        Upstream changed: {event.changed_upstream?.join(", ")}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            )}
            <div className="border-t p-5">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h3 className="font-semibold">Verified integration</h3>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    Freeze and verify a ready prefix against the live target before advancing it.
                  </p>
                </div>
                <Button disabled={saving} onClick={() => prepareIntegration(stack.id, stack.members.length)}>
                  Prepare complete stack
                </Button>
              </div>
              {stack.integration_candidates?.map((candidate) => (
                <div key={candidate.id} className="mt-3 rounded-lg border p-3 text-xs">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge tone={candidate.status === "passed" || candidate.status === "merged" ? "success" : candidate.status === "failed" ? "danger" : "warning"}>{candidate.status}</Badge>
                    <strong>Through layer {candidate.position}</strong>
                    <code>{candidate.target_revision.slice(0, 8)} → {candidate.candidate_revision.slice(0, 8)}</code>
                  </div>
                  <p className="mt-2">{candidate.required_checks.length ? candidate.required_checks.join(" · ") : "No cumulative checks required"}</p>
                  {candidate.status === "passed" && <Button className="mt-2" disabled={saving} onClick={() => mergeIntegration(stack.id, candidate.id)}>Integrate verified prefix</Button>}
                  {candidate.merged_at && <p className="mt-2 text-[var(--muted)]">Integrated by {candidate.merged_by}</p>}
                  {candidate.superseded_reason && <p className="mt-2 text-[var(--warning)]">Superseded: {candidate.superseded_reason.replaceAll("_", " ")}</p>}
                </div>
              ))}
            </div>
            <div className="border-t p-5">
              <h3 className="font-semibold">Preview a restack</h3>
              <p className="mt-1 text-xs text-[var(--muted)]">
                Reorder lines, omit a member to remove it, add a branch to
                insert it, or push a revised branch with stock Git first.
              </p>
              <form
                className="mt-3"
                onSubmit={(event) => previewRestack(stack, event)}
              >
                <textarea
                  name="restack-members"
                  required
                  rows={Math.max(3, stack.members.length + 1)}
                  defaultValue={stack.members
                    .map((m) =>
                      [
                        m.id,
                        m.title,
                        m.source_branch,
                        m.pull_request_id ?? "",
                        m.depends_on?.join(",") ?? "",
                        m.acceptance_criteria.join(";"),
                      ].join(" | "),
                    )
                    .join("\n")}
                  className="w-full rounded-lg border p-3 font-mono text-xs"
                />
                <Button disabled={saving} className="mt-2">
                  Preview rewrites
                </Button>
              </form>
              {stack.restacks?.map((restack) => (
                <div key={restack.id} className="mt-4 rounded-lg border p-4">
                  <div className="flex items-center gap-2">
                    <Badge
                      tone={
                        restack.diagnostics.some((d) => d.blocking)
                          ? "warning"
                          : "success"
                      }
                    >
                      {restack.status}
                    </Badge>
                    <span className="text-xs">
                      {restack.members.length} branch updates ·{" "}
                      {restack.removed_members?.length ?? 0} removed
                    </span>
                  </div>
                  {restack.members.map((m) => (
                    <p key={m.member.id} className="mt-2 text-xs">
                      <strong>
                        {m.action}: {m.member.title}
                      </strong>{" "}
                      · {m.old_revision?.slice(0, 8) || "new"} →{" "}
                      {m.candidate_revision?.slice(0, 8) || "blocked"} · stales{" "}
                      {m.impact.reviews_invalidated} reviews /{" "}
                      {m.impact.checks_invalidated} checks
                    </p>
                  ))}
                  {restack.diagnostics.map((d) => (
                    <p
                      key={d.code + d.message}
                      className="mt-2 text-xs text-[var(--warning)]"
                    >
                      {d.code.replaceAll("_", " ")}: {d.message}
                    </p>
                  ))}
                  {restack.status === "previewed" &&
                    !restack.diagnostics.some((d) => d.blocking) && (
                      <Button
                        disabled={saving}
                        className="mt-3"
                        onClick={() => applyRestack(stack.id, restack.id)}
                      >
                        Apply atomic branch updates
                      </Button>
                    )}
                  <p className="mt-2 text-[10px] text-[var(--muted)]">
                    {restack.authority}
                  </p>
                </div>
              ))}
            </div>
            <p className="border-t p-4 text-xs text-[var(--muted)]">
              {stack.authority}
            </p>
          </Card>
        ))}
      </section>
    </main>
  );
}
function ScopeView({ label, scope }: { label: string; scope: Scope }) {
  return (
    <div className="rounded-lg border p-3">
      <p className="text-xs font-semibold">{label}</p>
      <p className="mt-1 text-xs text-[var(--muted)]">
        {scope?.commit_count ?? 0} commits ·{" "}
        <span className="text-[var(--brand)]">+{scope?.additions ?? 0}</span> /{" "}
        <span className="text-[var(--danger)]">-{scope?.deletions ?? 0}</span> ·{" "}
        {scope?.files?.length ?? 0} files
      </p>
      <p className="mt-1 truncate font-mono text-[10px]">
        {scope?.files?.join(", ") || "No readable scope"}
      </p>
    </div>
  );
}
