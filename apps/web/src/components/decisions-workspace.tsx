"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, type DevelopmentWorkspace, type Repository, type TechnicalDecision } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
import { Icons } from "./icons";

const sourceKinds = [
  "repository",
  "proposal",
  "investigation",
  "incident",
  "evolution_plan",
  "stewardship_opportunity",
] as const;
const lines = (value: FormDataEntryValue | null) =>
  String(value ?? "")
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
const label = (value: string) => value.replaceAll("_", " ");
const when = (value: string) =>
  new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));

export function DecisionsWorkspace({ decisionId }: { decisionId?: string }) {
  const { token, user, loading: authLoading } = useAuth();
  const router = useRouter();
  const [repos, setRepos] = useState<Repository[]>([]),
    [items, setItems] = useState<TechnicalDecision[]>([]),
    [current, setCurrent] = useState<TechnicalDecision | null>(null);
  const [loading, setLoading] = useState(true),
    [pending, setPending] = useState(false),
    [error, setError] = useState(""),
    [creating, setCreating] = useState(false),
    [editing, setEditing] = useState(false);
  const load = useCallback(async () => {
    if (authLoading) return;
    if (!token) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [rp, dp] = await Promise.all([
        api<{ repositories: Repository[] }>(
          "/repositories?limit=100",
          {},
          token,
        ),
        decisionId
          ? api<TechnicalDecision>(`/decisions/${decisionId}`, {}, token)
          : api<{ decisions: TechnicalDecision[] }>("/decisions", {}, token),
      ]);
      setRepos(rp.repositories);
      if (decisionId) setCurrent(dp as TechnicalDecision);
      else setItems((dp as { decisions: TechnicalDecision[] }).decisions);
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Decisions could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [authLoading, decisionId, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!user) return;
    setPending(true);
    setError("");
    const d = new FormData(e.currentTarget),
      kind = String(d.get("source_kind")),
      repo = String(d.get("repository_id"));
    try {
      const v = await api<TechnicalDecision>(
        `/repositories/${repo}/decisions`,
        {
          method: "POST",
          body: JSON.stringify({
            source: {
              kind,
              resource_id: kind === "repository" ? repo : d.get("source_id"),
            },
            scope: scopeFrom(d, user.id),
          }),
        },
        token,
      );
      router.push(`/decisions/${v.id}`);
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Decision could not be opened.",
      );
    } finally {
      setPending(false);
    }
  }
  async function update(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!current || !user) return;
    setPending(true);
    const d = new FormData(e.currentTarget);
    try {
      const v = await api<TechnicalDecision>(
        `/decisions/${current.id}`,
        {
          method: "PUT",
          body: JSON.stringify({
            expected_version: current.version,
            summary: d.get("summary"),
            scope: scopeFrom(d, user.id, current),
          }),
        },
        token,
      );
      setCurrent(v);
      setEditing(false);
    } catch (x) {
      setError(x instanceof Error ? x.message : "Scope could not be updated.");
    } finally {
      setPending(false);
    }
  }
  async function discuss(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!current) return;
    setPending(true);
    const form = e.currentTarget,
      d = new FormData(form);
    try {
      setCurrent(
        await api<TechnicalDecision>(
          `/decisions/${current.id}/discussion`,
          { method: "POST", body: JSON.stringify({ body: d.get("body") }) },
          token,
        ),
      );
      form.reset();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Discussion could not be added.",
      );
    } finally {
      setPending(false);
    }
  }
  async function propose(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!current) return;
    setPending(true);
    const form = e.currentTarget,
      d = new FormData(form);
    try {
      const evidence = evidenceFrom(d.get("evidence"), current.repository_id);
      const outcomes = lines(d.get("criterion_outcomes"));
      setCurrent(
        await api<TechnicalDecision>(
          `/decisions/${current.id}/alternatives`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: current.version,
              alternative: {
                title: d.get("title"),
                summary: d.get("summary"),
                assumptions: lines(d.get("assumptions")),
                tradeoffs: lines(d.get("tradeoffs")),
                risks: lines(d.get("risks")),
                compatibility_impact: d.get("compatibility_impact"),
                cost: d.get("cost"),
                expected_outcomes: lines(d.get("expected_outcomes")),
                evidence,
                criteria: current.scope.success_measures.map(
                  (criterion, i) => ({
                    criterion,
                    outcome: outcomes[i] || "Not yet demonstrated",
                    evidence,
                  }),
                ),
              },
            }),
          },
          token,
        ),
      );
      form.reset();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Alternative could not be proposed.",
      );
    } finally {
      setPending(false);
    }
  }
  async function launchExperiment(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); if (!current) return; setPending(true); setError("");
    const form = e.currentTarget, d = new FormData(form), alternativeID = String(d.get("alternative_id"));
    try {
      const workspace = await api<DevelopmentWorkspace>("/workspaces", { method: "POST", body: JSON.stringify({ repository_id: current.repository_id, commit_id: d.get("commit_id"), source: { kind: "decision_experiment", decision_id: current.id, alternative_id: alternativeID } }) }, token);
      const updated = await api<TechnicalDecision>(`/decisions/${current.id}/experiments`, { method: "POST", body: JSON.stringify({ alternative_id: alternativeID, workspace_id: workspace.id }) }, token);
      setCurrent(updated); form.reset();
    } catch (x) { setError(x instanceof Error ? x.message : "Experiment could not be launched."); } finally { setPending(false); }
  }
  async function attachEvidence(e: FormEvent<HTMLFormElement>, experimentID: string, version: number) {
    e.preventDefault(); if (!current) return; setPending(true); const form = e.currentTarget, d = new FormData(form);
    try {
      const measurements = lines(d.get("measurements")).map((row) => { const [name, value, unit] = row.split("|").map((x) => x.trim()); return { name, value: Number(value), unit }; });
      const artifacts = lines(d.get("artifacts")).map((row) => { const [label, path, sha256, size] = row.split("|").map((x) => x.trim()); return { label, path, sha256, size: Number(size) }; });
      setCurrent(await api<TechnicalDecision>(`/decisions/${current.id}/experiments/${experimentID}/evidence`, { method: "POST", body: JSON.stringify({ expected_version: version, evidence: { checkpoint_ids: lines(d.get("checkpoints")), command_ids: lines(d.get("commands")), measurements, artifacts, notes: d.get("notes") } }) }, token)); form.reset();
    } catch (x) { setError(x instanceof Error ? x.message : "Experiment evidence could not be attached."); } finally { setPending(false); }
  }
  if (authLoading || loading)
    return (
      <Card className="p-8 text-sm text-[var(--muted)]">
        Gathering decision context…
      </Card>
    );
  if (!user)
    return (
      <Card className="p-8 text-center">
        <h1 className="text-2xl font-semibold">
          Technical decisions need shared context
        </h1>
        <Link
          href="/?access=signin"
          className="mt-5 inline-flex rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-semibold text-white"
        >
          Sign in
        </Link>
      </Card>
    );
  if (decisionId && current)
    return (
      <div className="space-y-6">
        <Link
          href="/decisions"
          className="text-sm font-semibold text-[var(--brand)]"
        >
          ← All decisions
        </Link>
        <header>
          <div className="flex gap-2">
            <Badge>Pending decision</Badge>
            <Badge>{label(current.source.kind)}</Badge>
          </div>
          <h1 className="mt-3 max-w-4xl text-3xl font-semibold tracking-[-.03em]">
            {current.scope.question}
          </h1>
          <p className="mt-2 text-sm text-[var(--muted)]">
            Owned by {current.scope.owner_id} · scope version {current.version}{" "}
            · due{" "}
            {current.scope.deadline
              ? new Date(current.scope.deadline).toLocaleDateString()
              : "when ready"}
          </p>
        </header>
        {error && (
          <p
            role="alert"
            className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
          >
            {error}
          </p>
        )}
        <div className="grid gap-5 lg:grid-cols-[1fr_20rem]">
          <Card className="p-6">
            <div className="flex justify-between">
              <h2 className="text-lg font-semibold">Decision scope</h2>
              <Button onClick={() => setEditing(!editing)}>
                {editing ? "Cancel" : "Revise scope"}
              </Button>
            </div>
            {editing ? (
              <ScopeForm
                onSubmit={update}
                decision={current}
                pending={pending}
              />
            ) : (
              <ScopeView decision={current} />
            )}
          </Card>
          <Card className="p-6">
            <h2 className="font-semibold">Accountability</h2>
            <p className="mt-3 text-sm">
              <b>Owner</b>
              <br />
              {current.scope.owner_id}
            </p>
            <p className="mt-4 text-sm">
              <b>Participants</b>
              <br />
              {current.scope.participants.map((x) => x.user_id).join(", ")}
            </p>
            <p className="mt-4 text-sm">
              <b>Origin</b>
              <br />
              {label(current.source.kind)} {current.source.resource_id}
            </p>
          </Card>
        </div>
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Compare alternatives</h2>
          <p className="mt-1 text-sm text-[var(--muted)]">
            Every claim is separated from its exact, inspectable evidence. Empty
            criterion outcomes are shown as missing rather than inferred from
            prose.
          </p>
          {current.alternatives.length ? (
            <div className="mt-5 overflow-x-auto">
              <table className="w-full min-w-[48rem] text-left text-sm">
                <thead>
                  <tr className="border-b">
                    <th className="p-3">Alternative</th>
                    <th className="p-3">Assumptions / tradeoffs</th>
                    <th className="p-3">Risk / compatibility</th>
                    <th className="p-3">Cost / outcomes</th>
                    <th className="p-3">Common criteria & evidence</th>
                  </tr>
                </thead>
                <tbody>
                  {current.alternatives.map((a) => (
                    <tr key={a.id} className="border-b align-top">
                      <td className="p-3">
                        <b>{a.title}</b>
                        <p className="mt-1 text-[var(--muted)]">{a.summary}</p>
                      </td>
                      <td className="p-3">
                        {a.assumptions.join("; ")}
                        <hr className="my-2" />
                        {a.tradeoffs.join("; ")}
                      </td>
                      <td className="p-3">
                        {a.risks.join("; ")}
                        <hr className="my-2" />
                        {a.compatibility_impact}
                      </td>
                      <td className="p-3">
                        {a.cost}
                        <hr className="my-2" />
                        {a.expected_outcomes.join("; ")}
                      </td>
                      <td className="p-3 space-y-2">
                        {a.criteria.map((c) => (
                          <div key={c.criterion}>
                            <b>{c.criterion}</b>: {c.outcome}
                            <div className="text-xs text-[var(--muted)]">
                              {c.evidence
                                .map(
                                  (e) =>
                                    `${e.kind}: ${e.label} @ ${e.revision}`,
                                )
                                .join(" · ")}
                            </div>
                          </div>
                        ))}
                        {a.evidence_status?.missing_kinds?.length > 0 && (
                          <p className="rounded bg-[var(--warning-soft)] p-2 text-xs">
                            Missing evidence: {a.evidence_status.missing_kinds.join(", ")}
                          </p>
                        )}
                        {a.evidence_status?.stale?.length > 0 && (
                          <p className="rounded bg-[var(--danger-soft)] p-2 text-xs">
                            Stale evidence: {a.evidence_status.stale.map((e) => e.label).join(", ")}
                          </p>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p className="mt-4 rounded-lg bg-[var(--surface-2)] p-4 text-sm">
              No alternatives yet. A preference without an alternative and
              current evidence remains visibly unsupported.
            </p>
          )}
          <details className="mt-6">
            <summary className="cursor-pointer font-semibold">
              Propose an evidence-backed alternative
            </summary>
            <form onSubmit={propose} className="mt-4 grid gap-3 sm:grid-cols-2">
              <Field name="title" title="Title" />
              <label className="text-sm font-semibold">
                Summary
                <textarea
                  name="summary"
                  required
                  rows={3}
                  className="mt-1 w-full rounded-lg border p-2"
                />
              </label>
              <ListField name="assumptions" title="Assumptions" />
              <ListField name="tradeoffs" title="Tradeoffs" />
              <ListField name="risks" title="Risks" />
              <label className="text-sm font-semibold">
                Compatibility impact
                <textarea
                  name="compatibility_impact"
                  required
                  rows={3}
                  className="mt-1 w-full rounded-lg border p-2"
                />
              </label>
              <label className="text-sm font-semibold">
                Cost
                <textarea
                  name="cost"
                  required
                  rows={3}
                  className="mt-1 w-full rounded-lg border p-2"
                />
              </label>
              <ListField name="expected_outcomes" title="Expected outcomes" />
              <ListField
                name="criterion_outcomes"
                title={`Criterion outcomes (in this order: ${current.scope.success_measures.join("; ")})`}
              />
              <label className="text-sm font-semibold sm:col-span-2">
                Exact evidence
                <textarea
                  name="evidence"
                  required
                  rows={5}
                  placeholder="kind | resource ID | exact revision | label | code path | start line | end line"
                  className="mt-1 w-full rounded-lg border p-2 font-mono font-normal"
                />
                <span className="mt-1 block text-xs font-normal text-[var(--muted)]">
                  Kinds: code, dependency, release, incident, usage. Code
                  requires path and line range.
                </span>
              </label>
              <div>
                <Button type="submit" disabled={pending}>
                  Propose alternative
                </Button>
              </div>
            </form>
          </details>
        </Card>
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Bounded experiments</h2>
          <p className="mt-1 text-sm text-[var(--muted)]">Experiments use the exact revision&apos;s repository-defined commands in an isolated shared workspace. Checkpoints stay exploratory until someone separately publishes one through ordinary review.</p>
          <div className="mt-5 space-y-4">
            {current.experiments?.map((experiment) => (
              <article key={experiment.id} className="rounded-lg border p-4">
                <div className="flex flex-wrap gap-2"><Badge>{experiment.invalidated ? "Evidence invalidated" : "Evidence current"}</Badge><Badge>{experiment.commands.join(", ")}</Badge></div>
                <p className="mt-2 font-mono text-xs">Revision {experiment.revision}</p>
                {experiment.invalidated && <p className="mt-2 text-sm text-[var(--danger)]">{experiment.invalidation_reasons.join(" · ")}</p>}
                <Link href={`/workspaces/${experiment.workspace_id}`} className="mt-2 inline-block text-sm font-semibold text-[var(--brand)]">Open shared experiment workspace →</Link>
                {experiment.evidence.map((evidence) => <div key={evidence.id} className="mt-3 rounded bg-[var(--surface-2)] p-3 text-sm"><b>{evidence.measurements.map((x) => `${x.name}: ${x.value} ${x.unit}`).join(" · ") || "Recorded evidence"}</b><p className="mt-1 text-xs text-[var(--muted)]">{evidence.checkpoint_ids.length} checkpoints · {evidence.command_ids.length} logs · {evidence.artifacts.length} artifacts · {evidence.cpu_seconds.toFixed(1)} CPU seconds · by {evidence.recorded_by}</p>{evidence.notes && <p className="mt-2">{evidence.notes}</p>}</div>)}
                <details className="mt-3"><summary className="cursor-pointer text-sm font-semibold">Attach workspace evidence</summary><form onSubmit={(event) => attachEvidence(event, experiment.id, experiment.version)} className="mt-3 grid gap-3"><ListField name="checkpoints" title="Checkpoint IDs" /><ListField name="commands" title="Command outcome IDs (logs)" /><ListField name="measurements" title="Measurements: name | value | unit" /><ListField name="artifacts" title="Artifacts: label | path | SHA-256 | bytes" /><label className="text-sm font-semibold">Notes<textarea name="notes" rows={3} className="mt-1 w-full rounded-lg border p-2 font-normal" /></label><div><Button type="submit" disabled={pending}>Attach attributed evidence</Button></div></form></details>
              </article>
            ))}
          </div>
          <form onSubmit={launchExperiment} className="mt-6 grid gap-3 sm:grid-cols-2">
            <label className="text-sm font-semibold">Alternative<select name="alternative_id" required className="mt-1 min-h-10 w-full rounded-lg border px-3">{current.alternatives.map((a) => <option key={a.id} value={a.id}>{a.title}</option>)}</select></label>
            <Field name="commit_id" title="Exact commit SHA" />
            <div className="sm:col-span-2"><Button type="submit" disabled={pending || current.alternatives.length === 0}>Launch isolated experiment</Button></div>
          </form>
        </Card>
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Cited research and dissent</h2>
          {current.findings.length ? (
            <div className="mt-4 space-y-3">
              {current.findings.map((f) => (
                <article
                  key={f.id}
                  className={`rounded-lg border p-4 ${f.superseded ? "opacity-60" : ""}`}
                >
                  <div className="flex gap-2">
                    <Badge>{f.position}</Badge>
                    {f.superseded && <Badge>Superseded</Badge>}
                  </div>
                  <p className="mt-2 text-sm">{f.body}</p>
                  <p className="mt-2 text-xs text-[var(--muted)]">
                    Uncertainty: {f.uncertainty}
                  </p>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    {f.citations
                      .map((c) => `${c.label} @ ${c.revision}`)
                      .join(" · ")}
                  </p>
                </article>
              ))}
            </div>
          ) : (
            <p className="mt-3 text-sm text-[var(--muted)]">
              No research findings yet. Issue a bounded research credential
              through the API to let a read-only agent inspect selected options
              and append cited support, dissent, or uncertainty.
            </p>
          )}
        </Card>
        <Card className="p-6">
          <h2 className="text-lg font-semibold">History and discussion</h2>
          <div className="mt-4 space-y-4">
            {current.history.map((h) => (
              <article
                key={h.id}
                className="border-l-2 border-[var(--line-strong)] pl-4"
              >
                <p className="text-sm font-semibold">{h.summary}</p>
                {h.body && (
                  <p className="mt-1 whitespace-pre-wrap text-sm leading-6">
                    {h.body}
                  </p>
                )}
                <p className="mt-1 text-xs text-[var(--muted)]">
                  {h.actor_id} · {when(h.created_at)} · scope v{h.version}
                </p>
              </article>
            ))}
          </div>
          <form onSubmit={discuss} className="mt-6">
            <textarea
              name="body"
              required
              maxLength={4000}
              rows={4}
              placeholder="Add context, a concern, or a question…"
              className="w-full rounded-lg border border-[var(--line-strong)] p-3 text-sm"
            />
            <Button type="submit" disabled={pending} className="mt-2">
              Add to discussion
            </Button>
          </form>
        </Card>
      </div>
    );
  return (
    <div className="space-y-7">
      <header className="flex items-end justify-between">
        <div>
          <p className="font-mono text-xs font-semibold uppercase tracking-[.16em] text-[var(--brand)]">
            Choose before code hardens
          </p>
          <h1 className="mt-2 text-3xl font-semibold">Technical decisions</h1>
          <p className="mt-2 max-w-2xl text-sm text-[var(--muted)]">
            Make consequential questions, boundaries, and accountability visible
            while related work continues.
          </p>
        </div>
        <Button onClick={() => setCreating(!creating)}>
          <Icons.Plus size={16} />
          {creating ? "Cancel" : "Open decision"}
        </Button>
      </header>
      {error && <p role="alert">{error}</p>}
      {creating && (
        <Card className="p-6">
          <ScopeForm onSubmit={create} repositories={repos} pending={pending} />
        </Card>
      )}
      <div className="grid gap-3">
        {items.length ? (
          items.map((x) => (
            <Link key={x.id} href={`/decisions/${x.id}`}>
              <Card className="p-5 transition hover:border-[var(--brand)]">
                <div className="flex gap-2">
                  <Badge>Pending</Badge>
                  <Badge>{label(x.source.kind)}</Badge>
                </div>
                <h2 className="mt-3 text-lg font-semibold">
                  {x.scope.question}
                </h2>
                <p className="mt-2 text-sm text-[var(--muted)]">
                  Owner {x.scope.owner_id} · {x.scope.participants.length}{" "}
                  participant{x.scope.participants.length === 1 ? "" : "s"} ·
                  updated {when(x.updated_at)}
                </p>
              </Card>
            </Link>
          ))
        ) : (
          <Card className="p-8 text-center text-sm text-[var(--muted)]">
            No pending decisions yet. Open one before an important choice
            becomes implicit.
          </Card>
        )}
      </div>
    </div>
  );
}
function evidenceFrom(value: FormDataEntryValue | null, repositoryID: string) {
  return lines(value).map((row) => {
    const [kind, resource_id, revision, label, path, start, end] = row
      .split("|")
      .map((x) => x.trim());
    return {
      kind,
      resource_id,
      revision,
      label,
      repository_id: kind === "code" ? repositoryID : undefined,
      path: path || undefined,
      start_line: start ? Number(start) : undefined,
      end_line: end ? Number(end) : undefined,
    };
  });
}
function Field({ name, title }: { name: string; title: string }) {
  return (
    <label className="text-sm font-semibold">
      {title}
      <input
        name={name}
        required
        className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal"
      />
    </label>
  );
}
function scopeFrom(d: FormData, userID: string, current?: TechnicalDecision) {
  const ids = lines(d.get("participants"));
  if (!ids.includes(userID)) ids.unshift(userID);
  const owner = String(d.get("owner_id") || current?.scope.owner_id || userID);
  if (!ids.includes(owner)) ids.push(owner);
  return {
    question: d.get("question"),
    constraints: lines(d.get("constraints")),
    success_measures: lines(d.get("success_measures")),
    deadline: d.get("deadline")
      ? new Date(String(d.get("deadline"))).toISOString()
      : undefined,
    affected_resources: lines(d.get("resources")).map((label) => ({
      kind: "related",
      repository_id: String(
        d.get("repository_id") || current?.repository_id || "",
      ),
      label,
    })),
    participants: ids.map((user_id) => ({ user_id })),
    owner_id: owner,
  };
}
function ScopeForm({
  onSubmit,
  repositories,
  decision,
  pending,
}: {
  onSubmit: (e: FormEvent<HTMLFormElement>) => void;
  repositories?: Repository[];
  decision?: TechnicalDecision;
  pending: boolean;
}) {
  return (
    <form onSubmit={onSubmit} className="mt-5 grid gap-4">
      {repositories && (
        <>
          <label className="text-sm font-semibold">
            Repository
            <select
              name="repository_id"
              required
              className="mt-2 min-h-11 w-full rounded-lg border px-3"
            >
              {repositories.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </select>
          </label>
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="text-sm font-semibold">
              Starting context
              <select
                name="source_kind"
                className="mt-2 min-h-11 w-full rounded-lg border px-3"
              >
                {sourceKinds.map((x) => (
                  <option key={x} value={x}>
                    {label(x)}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-sm font-semibold">
              Context ID
              <input
                name="source_id"
                placeholder="Required outside repository context"
                className="mt-2 min-h-11 w-full rounded-lg border px-3"
              />
            </label>
          </div>
        </>
      )}
      <label className="text-sm font-semibold">
        Question
        <textarea
          name="question"
          required
          maxLength={2000}
          rows={3}
          defaultValue={decision?.scope.question}
          className="mt-2 w-full rounded-lg border p-3"
        />
      </label>
      <div className="grid gap-4 sm:grid-cols-2">
        <ListField
          name="constraints"
          title="Constraints"
          values={decision?.scope.constraints}
        />
        <ListField
          name="success_measures"
          title="Success measures"
          values={decision?.scope.success_measures}
        />
      </div>
      <ListField
        name="resources"
        title="Affected resources"
        values={decision?.scope.affected_resources.map((x) => x.label)}
      />
      <div className="grid gap-4 sm:grid-cols-3">
        <label className="text-sm font-semibold">
          Decision owner
          <input
            name="owner_id"
            defaultValue={decision?.scope.owner_id}
            placeholder="Defaults to you"
            className="mt-2 min-h-11 w-full rounded-lg border px-3"
          />
        </label>
        <label className="text-sm font-semibold">
          Participants
          <textarea
            name="participants"
            rows={4}
            defaultValue={decision?.scope.participants
              .map((x) => x.user_id)
              .join("\n")}
            placeholder="One user ID per line"
            className="mt-2 min-h-11 w-full rounded-lg border px-3"
          />
        </label>
        <label className="text-sm font-semibold">
          Deadline
          <input
            name="deadline"
            type="date"
            required
            defaultValue={decision?.scope.deadline?.slice(0, 10)}
            className="mt-2 min-h-11 w-full rounded-lg border px-3"
          />
        </label>
      </div>
      {decision && (
        <label className="text-sm font-semibold">
          What changed?
          <input
            name="summary"
            required
            maxLength={500}
            className="mt-2 min-h-11 w-full rounded-lg border px-3"
          />
        </label>
      )}
      <div>
        <Button type="submit" disabled={pending}>
          {pending
            ? "Saving…"
            : decision
              ? "Publish scope revision"
              : "Open pending decision"}
        </Button>
      </div>
    </form>
  );
}
function ListField({
  name,
  title,
  values,
}: {
  name: string;
  title: string;
  values?: string[];
}) {
  return (
    <label className="text-sm font-semibold">
      {title}
      <textarea
        name={name}
        required
        rows={4}
        defaultValue={values?.join("\n")}
        placeholder="One per line"
        className="mt-2 w-full rounded-lg border p-3 font-normal"
      />
    </label>
  );
}
function ScopeView({ decision: d }: { decision: TechnicalDecision }) {
  return (
    <div className="mt-5 grid gap-6 sm:grid-cols-2">
      <section>
        <h3 className="text-sm font-semibold">Constraints</h3>
        <ul className="mt-2 list-disc space-y-1 pl-5 text-sm">
          {d.scope.constraints.map((x) => (
            <li key={x}>{x}</li>
          ))}
        </ul>
      </section>
      <section>
        <h3 className="text-sm font-semibold">Success measures</h3>
        <ul className="mt-2 list-disc space-y-1 pl-5 text-sm">
          {d.scope.success_measures.map((x) => (
            <li key={x}>{x}</li>
          ))}
        </ul>
      </section>
      <section className="sm:col-span-2">
        <h3 className="text-sm font-semibold">Affected resources</h3>
        <div className="mt-2 flex flex-wrap gap-2">
          {d.scope.affected_resources.map((x, i) => (
            <Badge key={`${x.label}-${i}`}>{x.label}</Badge>
          ))}
        </div>
      </section>
    </div>
  );
}
