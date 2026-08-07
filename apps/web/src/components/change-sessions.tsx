"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  api,
  apiResponse,
  type ChangeSession,
  type ChangeSessionEvent,
  type PullRequest,
  type Repository,
  type User,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

const formatTime = (value: string) =>
  new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(value));
const short = (id: string) => id.slice(0, 7);
const message = (reason: unknown, fallback: string) =>
  reason instanceof Error ? reason.message : fallback;

async function sessions(path: string, token: string | null) {
  const found: ChangeSession[] = [];
  let after: string | null = null;
  do {
    const page: { sessions: ChangeSession[]; next_cursor: string | null } = await api<{
      sessions: ChangeSession[];
      next_cursor: string | null;
    }>(`${path}?limit=100${after ? `&after=${encodeURIComponent(after)}` : ""}`, {}, token);
    found.push(...page.sessions);
    after = page.next_cursor;
  } while (after);
  return found;
}

async function timeline(path: string, token: string | null) {
  const found: ChangeSessionEvent[] = [];
  let after: string | null = null;
  do {
    const page: { events: ChangeSessionEvent[]; next_cursor: string | null } =
      await api<{ events: ChangeSessionEvent[]; next_cursor: string | null }>(
        `${path}?limit=100${after ? `&after=${encodeURIComponent(after)}` : ""}`,
        {},
        token,
      );
    found.push(...page.events);
    after = page.next_cursor;
  } while (after);
  return found;
}

export function ChangeSessionsCard({ repositoryID, pullRequestID, participant, open }: { repositoryID: string; pullRequestID: string; participant: boolean; open: boolean }) {
  const { token } = useAuth();
  const [items, setItems] = useState<ChangeSession[]>([]);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [uncertain, setUncertain] = useState<ChangeSession | null>(null);
  const base = `/repositories/${repositoryID}/pulls/${pullRequestID}/sessions`;

  const load = useCallback(async () => {
    if (!participant) { setItems([]); return; }
    try { setItems(await sessions(base, token)); setError(""); }
    catch (reason) { setError(message(reason, "Change sessions could not be loaded.")); }
  }, [base, participant, token]);
  useEffect(() => { void Promise.resolve().then(load); }, [load]);

  async function create() {
    setPending(true); setError(""); setUncertain(null);
    try {
      const response = await apiResponse<ChangeSession>(base, { method: "POST" }, token);
      if (response.status === 202 || response.headers.get("Vivarium-Durability") === "uncertain") {
        setUncertain(response.data);
      } else {
        setItems((current) => [...current, response.data]);
      }
    } catch (reason) { setError(message(reason, "The change session could not be started.")); }
    finally { setPending(false); }
  }

  if (!participant) return null;
  return <Card className="p-5">
    <div className="flex items-center justify-between gap-3">
      <div><p className="font-mono text-[10px] font-semibold uppercase tracking-[.14em] text-[var(--brand)]">Agent workspace</p><h2 className="mt-1 font-semibold">Change sessions</h2></div>
      <Badge tone={items.length ? "info" : "neutral"}>{items.length}</Badge>
    </div>
    <p className="mt-2 text-sm leading-6 text-[var(--muted)]">Open a durable shared workspace on this review revision and return to its timeline anytime.</p>
    {error && <p role="alert" className="mt-3 text-sm text-[var(--danger)]">{error}</p>}
    {uncertain && <div role="status" className="mt-3 rounded-lg bg-[var(--warning-soft)] p-3 text-sm leading-6 text-[var(--warning)]"><p className="font-semibold">Session created with uncertain durability</p><p>The workspace is visible, but crash-safe persistence was not confirmed. Keep its stable link and inspect it later instead of starting another session.</p><Link href={`/pulls/${repositoryID}/${pullRequestID}/sessions/${uncertain.id}`} className="mt-2 inline-block font-semibold underline">Inspect session {short(uncertain.id)}</Link></div>}
    {items.length > 0 && <ol className="mt-4 space-y-2">{[...items].reverse().map((session) => <li key={session.id}><Link href={`/pulls/${repositoryID}/${pullRequestID}/sessions/${session.id}`} className="flex items-center justify-between rounded-lg border border-[var(--line)] px-3 py-2 text-sm hover:border-[var(--line-strong)]"><span><span className="font-semibold">Session {short(session.id)}</span><span className="ml-2 text-xs text-[var(--muted)]">{formatTime(session.created_at)}</span></span><Badge tone="success">{session.state}</Badge></Link></li>)}</ol>}
    {open ? <Button className="mt-4 w-full" variant={items.length ? "secondary" : "primary"} disabled={pending} onClick={() => void create()}>{pending ? "Opening…" : "Start change session"}</Button> : <p className="mt-4 text-xs text-[var(--muted)]">Merged pull requests keep their existing sessions, but cannot start new work.</p>}
  </Card>;
}

export function ChangeSessionDetail({ repositoryID, pullRequestID, sessionID }: { repositoryID: string; pullRequestID: string; sessionID: string }) {
  const { token, user, loading: authLoading } = useAuth();
  const [session, setSession] = useState<ChangeSession | null>(null);
  const [events, setEvents] = useState<ChangeSessionEvent[]>([]);
  const [pull, setPull] = useState<PullRequest | null>(null);
  const [repository, setRepository] = useState<Repository | null>(null);
  const [initiator, setInitiator] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [durabilityUncertain, setDurabilityUncertain] = useState(false);
  const generation = useRef(0);

  const load = useCallback(async () => {
    if (authLoading) return;
    const current = ++generation.current;
    if (!token) { setLoading(false); return; }
    setLoading(true); setError("");
    const base = `/repositories/${repositoryID}/pulls/${pullRequestID}`;
    try {
      const [repo, pullItem, sessionResponse] = await Promise.all([
        api<Repository>(`/repositories/${repositoryID}`, {}, token),
        api<PullRequest>(base, {}, token),
        apiResponse<ChangeSession>(`${base}/sessions/${sessionID}`, {}, token),
      ]);
      const uncertain = sessionResponse.status === 202 || sessionResponse.headers.get("Vivarium-Durability") === "uncertain";
      const sessionItem = sessionResponse.data;
      const eventPage = uncertain ? [] : await timeline(`${base}/sessions/${sessionID}/events`, token);
      const person = await api<User>(`/users/${sessionItem.initiator_id}`, {}, token).catch(() => null);
      if (generation.current !== current) return;
      setRepository(repo); setPull(pullItem); setSession(sessionItem); setEvents(eventPage); setInitiator(person); setDurabilityUncertain(uncertain);
    } catch (reason) { if (generation.current === current) setError(message(reason, "The change session could not be loaded.")); }
    finally { if (generation.current === current) setLoading(false); }
  }, [authLoading, pullRequestID, repositoryID, sessionID, token]);
  useEffect(() => { void Promise.resolve().then(load); return () => { generation.current += 1; }; }, [load]);

  if (authLoading || loading) return <Card className="p-8 text-sm text-[var(--muted)]">Reconnecting to the change session…</Card>;
  if (!user) return <Card className="p-8 text-center"><h1 className="text-2xl font-semibold">Reconnect to shared agent work</h1><p className="mt-2 text-sm text-[var(--muted)]">Sign in as a current repository collaborator to inspect this session.</p><Link href="/?access=signin" className="mt-5 inline-flex min-h-10 items-center rounded-lg bg-[var(--brand)] px-4 text-sm font-semibold text-white">Sign in</Link></Card>;
  if (error || !session || !pull || !repository) return <Card className="p-8"><h1 className="text-xl font-semibold">Session unavailable</h1><p role="alert" className="mt-2 text-sm text-[var(--danger)]">{error || "The session could not be found."}</p></Card>;
  return <div className="space-y-7">
    <header><Link href={`/pulls/${repositoryID}/${pullRequestID}`} className="text-sm font-semibold text-[var(--brand)]">← Back to pull request</Link><div className="mt-5 flex flex-wrap items-center gap-3"><Badge tone={durabilityUncertain ? "warning" : "success"}>{durabilityUncertain ? "durability uncertain" : session.state}</Badge><span className="font-mono text-xs text-[var(--muted)]">Session {short(session.id)}</span></div><h1 className="mt-3 text-3xl font-semibold tracking-[-.035em]">Change session for {pull.title}</h1><p className="mt-2 text-sm leading-6 text-[var(--muted)]">{durabilityUncertain ? "A visible collaboration workspace whose crash-safe persistence is not yet confirmed" : "A durable collaboration timeline"} in {repository.name}, anchored to revision <code>{short(session.source_commit_id)}</code>.</p></header>
    {durabilityUncertain && <Card className="border-[var(--warning)] bg-[var(--warning-soft)] p-5"><h2 className="font-semibold text-[var(--warning)]">Session durability remains uncertain</h2><p className="mt-2 text-sm leading-6 text-[var(--warning)]">The workspace is currently visible, but its storage directory could not be synchronized. Do not rely on its timeline until a later inspection confirms persistence.</p><Button className="mt-4" variant="secondary" onClick={() => void load()}>Retry durability check</Button></Card>}
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
      <main><h2 className="text-lg font-semibold">Session timeline</h2>{durabilityUncertain ? <Card className="mt-3 p-6 text-sm text-[var(--muted)]">Timeline events are withheld until session durability is confirmed.</Card> : <Card className="mt-3 overflow-hidden"><ol className="divide-y divide-[var(--line)]">{events.map((event) => <li key={event.id} className="flex gap-4 p-5"><span aria-hidden="true" className="mt-1.5 size-2 shrink-0 rounded-full bg-[var(--brand)]"/><div><p className="text-sm font-semibold">Change session opened</p><p className="mt-1 text-sm text-[var(--muted)]">{event.actor_id === session.initiator_id && initiator ? `@${initiator.handle}` : "A collaborator"} created this workspace in the {event.state} state.</p><time className="mt-2 block text-xs text-[var(--muted)]">{formatTime(event.created_at)}</time></div></li>)}</ol></Card>}</main>
      <aside><Card className="p-5"><h2 className="font-semibold">Session context</h2><dl className="mt-4 space-y-3 text-sm"><div><dt className="text-xs text-[var(--muted)]">Initiated by</dt><dd className="mt-1 font-semibold">{initiator ? `@${initiator.handle}` : short(session.initiator_id)}</dd></div><div><dt className="text-xs text-[var(--muted)]">Review revision</dt><dd className="mt-1"><code title={session.source_commit_id}>{short(session.source_commit_id)}</code></dd></div><div><dt className="text-xs text-[var(--muted)]">Created</dt><dd className="mt-1">{formatTime(session.created_at)}</dd></div></dl></Card></aside>
    </div>
  </div>;
}
