"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, type Repository, type TechnicalDecision } from "@/lib/api";
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
