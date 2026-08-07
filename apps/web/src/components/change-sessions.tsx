"use client";

import Link from "next/link";
import { type FormEvent, useCallback, useEffect, useRef, useState } from "react";
import {
  type AgentRun,
  api,
  apiResponse,
  type ChangeSession,
  type ChangeSessionEvent,
  type Credential,
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
const eventTitles: Record<ChangeSessionEvent["kind"], string> = {
  "session.opened": "Change session opened",
  "run.launched": "Agent run launched",
  "run.status": "Agent status",
  "agent.message": "Agent message",
  "agent.question": "Agent question",
  "tool.action": "Tool action",
  "artifact.produced": "Artifact produced",
  "run.failed": "Agent run failed",
  "branch.updated": "Branch updated",
  "run.guidance": "Follow-up guidance",
  "question.answered": "Question answered",
  "run.paused": "Run paused",
  "run.resumed": "Run resumed",
  "run.canceled": "Run canceled",
  "run.completed": "Work published for review",
};

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

async function runs(path: string, token: string | null) {
  const found: AgentRun[] = [];
  let after: string | null = null;
  do {
    const page: { runs: AgentRun[]; next_cursor: string | null } = await api<{ runs: AgentRun[]; next_cursor: string | null }>(`${path}?limit=100${after ? `&after=${encodeURIComponent(after)}` : ""}`, {}, token);
    found.push(...page.runs); after = page.next_cursor;
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
  const [agentRuns, setAgentRuns] = useState<AgentRun[]>([]);
  const [issued, setIssued] = useState<{ run: AgentRun; credential: Credential } | null>(null);
  const [launching, setLaunching] = useState(false);
  const [controlling, setControlling] = useState<string | null>(null);
  const [pull, setPull] = useState<PullRequest | null>(null);
  const [repository, setRepository] = useState<Repository | null>(null);
  const [initiator, setInitiator] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [interventionError, setInterventionError] = useState("");
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
      const runPage = uncertain ? [] : await runs(`${base}/sessions/${sessionID}/runs`, token);
      const person = await api<User>(`/users/${sessionItem.initiator_id}`, {}, token).catch(() => null);
      if (generation.current !== current) return;
      setRepository(repo); setPull(pullItem); setSession(sessionItem); setEvents(eventPage); setAgentRuns(runPage); setInitiator(person); setDurabilityUncertain(uncertain);
    } catch (reason) { if (generation.current === current) setError(message(reason, "The change session could not be loaded.")); }
    finally { if (generation.current === current) setLoading(false); }
  }, [authLoading, pullRequestID, repositoryID, sessionID, token]);
  useEffect(() => { void Promise.resolve().then(load); return () => { generation.current += 1; }; }, [load]);
  useEffect(() => {
    if (!token || durabilityUncertain || !session) return;
    const interval = window.setInterval(() => {
      const base = `/repositories/${repositoryID}/pulls/${pullRequestID}/sessions/${sessionID}`;
      void Promise.all([timeline(`${base}/events`, token), runs(`${base}/runs`, token)])
        .then(([nextEvents, nextRuns]) => { setEvents(nextEvents); setAgentRuns(nextRuns); }).catch(() => undefined);
    }, 5000);
    return () => window.clearInterval(interval);
  }, [durabilityUncertain, pullRequestID, repositoryID, session, sessionID, token]);

  async function launch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!session) return;
    setLaunching(true); setError(""); setIssued(null);
    const form = event.currentTarget; const data = new FormData(form);
    const contextPaths = String(data.get("context_paths") || "").split(/[\n,]/).map((item) => item.trim()).filter(Boolean);
    try {
      const response = await apiResponse<{ run: AgentRun; credential: Credential }>(`/repositories/${repositoryID}/pulls/${pullRequestID}/sessions/${sessionID}/runs`, { method: "POST", body: JSON.stringify({ instructions: data.get("instructions"), source_commit_id: data.get("source_commit_id"), context_paths: contextPaths, working_branch: data.get("working_branch"), expires_in: 3600 }) }, token);
      setIssued(response.data); setAgentRuns((current) => [...current, response.data.run]);
      setEvents((current) => [...current, { id: `pending-${response.data.run.id}`, session_id: sessionID, kind: "run.launched", actor_id: response.data.run.initiator_id, state: "launched", run_id: response.data.run.id, created_at: response.data.run.created_at }]);
      form.reset();
    } catch (reason) { setError(message(reason, "The agent run could not be launched.")); }
    finally { setLaunching(false); }
  }

  async function revoke(run: AgentRun) {
    try {
      const updated = await api<AgentRun>(`/repositories/${repositoryID}/pulls/${pullRequestID}/sessions/${sessionID}/runs/${run.id}/credential`, { method: "DELETE" }, token);
      setAgentRuns((current) => current.map((item) => item.id === updated.id ? updated : item));
    } catch (reason) { setError(message(reason, "Agent access could not be revoked.")); }
  }

  async function intervene(run: AgentRun, kind: "run.guidance" | "question.answered" | "run.paused" | "run.resumed" | "run.canceled", interventionMessage: string) {
    setControlling(run.id); setInterventionError("");
    try {
      const response = await apiResponse<{ run: AgentRun; event: ChangeSessionEvent }>(`/repositories/${repositoryID}/pulls/${pullRequestID}/sessions/${sessionID}/runs/${run.id}/interventions`, { method: "POST", body: JSON.stringify({ kind, message: interventionMessage }) }, token);
      setAgentRuns((current) => current.map((item) => item.id === run.id ? response.data.run : item));
      setEvents((current) => current.some((item) => item.id === response.data.event.id) ? current : [...current, response.data.event]);
      return true;
    } catch (reason) { setInterventionError(message(reason, "The intervention could not be published.")); return false; }
    finally { setControlling(null); }
  }

  async function guide(event: FormEvent<HTMLFormElement>, run: AgentRun) {
    event.preventDefault();
    const form = event.currentTarget; const data = new FormData(form);
    if (await intervene(run, String(data.get("kind")) as "run.guidance" | "question.answered", String(data.get("message") || ""))) form.reset();
  }

  if (authLoading || loading) return <Card className="p-8 text-sm text-[var(--muted)]">Reconnecting to the change session…</Card>;
  if (!user) return <Card className="p-8 text-center"><h1 className="text-2xl font-semibold">Reconnect to shared agent work</h1><p className="mt-2 text-sm text-[var(--muted)]">Sign in as a current repository collaborator to inspect this session.</p><Link href="/?access=signin" className="mt-5 inline-flex min-h-10 items-center rounded-lg bg-[var(--brand)] px-4 text-sm font-semibold text-white">Sign in</Link></Card>;
  if (error || !session || !pull || !repository) return <Card className="p-8"><h1 className="text-xl font-semibold">Session unavailable</h1><p role="alert" className="mt-2 text-sm text-[var(--danger)]">{error || "The session could not be found."}</p></Card>;
  return <div className="space-y-7">
    <header><Link href={`/pulls/${repositoryID}/${pullRequestID}`} className="text-sm font-semibold text-[var(--brand)]">← Back to pull request</Link><div className="mt-5 flex flex-wrap items-center gap-3"><Badge tone={durabilityUncertain ? "warning" : "success"}>{durabilityUncertain ? "durability uncertain" : session.state}</Badge><span className="font-mono text-xs text-[var(--muted)]">Session {short(session.id)}</span></div><h1 className="mt-3 text-3xl font-semibold tracking-[-.035em]">Change session for {pull.title}</h1><p className="mt-2 text-sm leading-6 text-[var(--muted)]">{durabilityUncertain ? "A visible collaboration workspace whose crash-safe persistence is not yet confirmed" : "A durable collaboration timeline"} in {repository.name}, anchored to revision <code>{short(session.source_commit_id)}</code>.</p></header>
    {durabilityUncertain && <Card className="border-[var(--warning)] bg-[var(--warning-soft)] p-5"><h2 className="font-semibold text-[var(--warning)]">Session durability remains uncertain</h2><p className="mt-2 text-sm leading-6 text-[var(--warning)]">The workspace is currently visible, but its storage directory could not be synchronized. Do not rely on its timeline until a later inspection confirms persistence.</p><Button className="mt-4" variant="secondary" onClick={() => void load()}>Retry durability check</Button></Card>}
    {interventionError && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{interventionError} Your draft is still available to retry.</p>}
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
      <main className="space-y-6"><section><div className="flex items-end justify-between gap-3"><div><h2 className="text-lg font-semibold">Session timeline</h2><p className="mt-1 text-xs text-[var(--muted)]">Live agent progress refreshes every five seconds.</p></div><Badge tone="info">{events.length} events</Badge></div>{durabilityUncertain ? <Card className="mt-3 p-6 text-sm text-[var(--muted)]">Timeline events are withheld until session durability is confirmed.</Card> : <Card className="mt-3 overflow-hidden"><ol className="divide-y divide-[var(--line)]">{events.map((event) => <li key={event.id} className="flex gap-4 p-5"><span aria-hidden="true" className={`mt-1.5 size-2 shrink-0 rounded-full ${event.kind === "run.failed" ? "bg-[var(--danger)]" : "bg-[var(--brand)]"}`}/><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><p className="text-sm font-semibold">{eventTitles[event.kind]}</p>{event.state && <Badge tone={event.kind === "run.failed" ? "warning" : "neutral"}>{event.state}</Badge>}</div><p className="mt-1 whitespace-pre-wrap text-sm text-[var(--muted)]">{event.message || (event.kind === "run.launched" ? "A collaborator authorized bounded work." : `${event.actor_id === session.initiator_id && initiator ? `@${initiator.handle}` : "A collaborator"} created this workspace.`)}</p>{(event.tool || event.artifact || event.branch || event.commit_id) && <p className="mt-2 break-all font-mono text-xs text-[var(--muted)]">{[event.tool && `tool: ${event.tool}`, event.artifact && `artifact: ${event.artifact}`, event.branch && `branch: ${event.branch}`, event.commit_id && `commit: ${short(event.commit_id)}`].filter(Boolean).join(" · ")}</p>}{event.agent_id && <p className="mt-2 font-mono text-[10px] text-[var(--muted)]">Agent {short(event.agent_id)} · authorized by {short(event.initiator_id || event.actor_id)} · revision {short(event.revision_id || session.source_commit_id)}</p>}<time className="mt-2 block text-xs text-[var(--muted)]">{formatTime(event.created_at)}</time></div></li>)}</ol></Card>}</section>
      {!durabilityUncertain && pull.status === "open" && <Card className="p-5"><h2 className="font-semibold">Delegate bounded work</h2><p className="mt-1 text-sm leading-6 text-[var(--muted)]">Define the outcome, pin this review revision, name only the paths the agent should focus on, and grant one hour of Git access to the pull request branch.</p><form className="mt-5 space-y-4" onSubmit={launch}><label className="block text-sm font-semibold">Instructions<textarea name="instructions" required maxLength={10000} rows={5} placeholder="Describe the outcome and checks that should pass." className="mt-2 w-full rounded-lg border border-[var(--line-strong)] p-3 font-normal"/></label><label className="block text-sm font-semibold">Pull request revision<select name="source_commit_id" className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-mono text-xs font-normal"><option value={session.source_commit_id}>{session.source_commit_id} · current session revision</option></select></label><label className="block text-sm font-semibold">Repository context<textarea name="context_paths" required rows={3} placeholder={"README.md\nsrc/feature.ts"} className="mt-2 w-full rounded-lg border border-[var(--line-strong)] p-3 font-mono text-xs font-normal"/><span className="mt-1 block text-xs font-normal text-[var(--muted)]">One existing file or directory path per line, resolved at the selected revision.</span></label><label className="block text-sm font-semibold">Working branch<select name="working_branch" className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-mono text-xs font-normal"><option value={pull.source_branch}>refs/heads/{pull.source_branch} · pull request source</option></select></label><Button type="submit" disabled={launching}>{launching ? "Launching…" : "Launch agent run"}</Button></form>{issued?.credential.token && <div role="status" className="mt-5 rounded-lg border border-[var(--brand)] bg-[var(--brand-soft)] p-4"><p className="text-sm font-semibold">Run launched · copy its credential now</p><p className="mt-1 text-xs text-[var(--muted)]">This secret is shown once, expires in one hour, and can only access {repository.name} and push {issued.run.working_branch}.</p><code className="mt-3 block break-all rounded bg-white p-3 text-xs select-all">{issued.credential.token}</code></div>}</Card>}
      {agentRuns.length > 0 && <section><h2 className="text-lg font-semibold">Agent runs</h2><div className="mt-3 space-y-3">{[...agentRuns].reverse().map((run) => <Card key={run.id} className="p-5"><div className="flex flex-wrap items-start justify-between gap-3"><div><div className="flex items-center gap-2"><Badge tone={run.state === "canceled" || run.access_revoked_at ? "neutral" : run.state === "paused" ? "warning" : "success"}>{run.state === "canceled" ? "canceled" : run.access_revoked_at ? "access revoked" : run.state}</Badge><code className="text-xs">{run.working_branch}</code></div><p className="mt-3 whitespace-pre-wrap text-sm leading-6">{run.instructions}</p><p className="mt-2 text-xs text-[var(--muted)]">Agent {short(run.agent_id)} · Authorized by {short(run.initiator_id)} · Revision {short(run.source_commit_id)} · Context: {run.context_paths.join(", ")} · Access expires {formatTime(run.credential_expires_at)}</p></div>{!run.access_revoked_at && run.state !== "completed" && <Button variant="quiet" onClick={() => void revoke(run)}>Revoke access</Button>}</div>{run.outcome && <div id="outcome" className="mt-5 border-t border-[var(--line)] pt-5"><h3 className="font-semibold">Review handoff</h3><p className="mt-2 whitespace-pre-wrap text-sm leading-6">{run.outcome.summary}</p><dl className="mt-4 grid gap-4 sm:grid-cols-2"><div><dt className="text-xs font-semibold text-[var(--muted)]">Commits</dt><dd className="mt-2 flex flex-wrap gap-2">{run.outcome.commits.map((commit) => <Link key={commit} href={`/repositories/${repositoryID}?ref=${commit}`} className="font-mono text-xs font-semibold text-[var(--brand)] underline" title={commit}>{short(commit)}</Link>)}</dd></div><div><dt className="text-xs font-semibold text-[var(--muted)]">Checks performed</dt><dd className="mt-2 space-y-1">{run.outcome.checks.length ? run.outcome.checks.map((check) => <p key={`${check.name}-${check.status}`} className="text-xs"><Badge tone={check.status === "passed" ? "success" : check.status === "failed" ? "warning" : "neutral"}>{check.status}</Badge><span className="ml-2 font-semibold">{check.name}</span>{check.details && <span className="ml-1 text-[var(--muted)]">— {check.details}</span>}</p>) : <p className="text-xs text-[var(--muted)]">No checks reported.</p>}</dd></div></dl><div className="mt-4"><p className="text-xs font-semibold text-[var(--muted)]">Changed files</p><ul className="mt-2 space-y-1">{run.outcome.changed_files.map((file) => <li key={file.path} className="flex items-center gap-2 text-xs"><Badge tone="neutral">{file.status}</Badge><Link href={`/repositories/${repositoryID}?ref=${run.outcome!.commit_id}&path=${encodeURIComponent(file.path)}`} className="break-all font-mono text-[var(--brand)] underline">{file.path}</Link></li>)}</ul></div><div className="mt-4"><p className="text-xs font-semibold text-[var(--muted)]">Unresolved concerns</p>{run.outcome.unresolved_concerns.length ? <ul className="mt-2 list-disc space-y-1 pl-5 text-sm">{run.outcome.unresolved_concerns.map((concern) => <li key={concern}>{concern}</li>)}</ul> : <p className="mt-2 text-sm text-[var(--muted)]">None reported.</p>}</div><Link href={`/pulls/${repositoryID}/${pullRequestID}`} className="mt-4 inline-block text-sm font-semibold text-[var(--brand)] underline">Review revision {short(run.outcome.commit_id)}</Link></div>}{run.state !== "canceled" && run.state !== "completed" && !run.access_revoked_at && <div className="mt-5 border-t border-[var(--line)] pt-5"><form className="space-y-3" onSubmit={(event) => void guide(event, run)}><div className="grid gap-3 sm:grid-cols-[11rem_1fr]"><select name="kind" aria-label="Guidance type" className="min-h-11 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"><option value="run.guidance">Follow-up guidance</option><option value="question.answered">Answer question</option></select><textarea name="message" aria-label="Guidance message" required maxLength={10000} rows={2} placeholder="Redirect the work or answer the agent clearly." className="rounded-lg border border-[var(--line-strong)] p-3 text-sm"/></div><Button type="submit" variant="secondary" disabled={controlling === run.id}>Send to agent</Button></form><div className="mt-4 flex flex-wrap gap-2">{run.state === "launched" ? <Button variant="quiet" disabled={controlling === run.id} onClick={() => void intervene(run, "run.paused", "Work paused by a collaborator.")}>Pause run</Button> : <Button variant="secondary" disabled={controlling === run.id} onClick={() => void intervene(run, "run.resumed", "Work resumed by a collaborator.")}>Resume run</Button>}<Button variant="quiet" disabled={controlling === run.id} onClick={() => void intervene(run, "run.canceled", "Run canceled by a collaborator.")}>Cancel run</Button></div></div>}</Card>)}</div></section>}</main>
      <aside><Card className="p-5"><h2 className="font-semibold">Session context</h2><dl className="mt-4 space-y-3 text-sm"><div><dt className="text-xs text-[var(--muted)]">Initiated by</dt><dd className="mt-1 font-semibold">{initiator ? `@${initiator.handle}` : short(session.initiator_id)}</dd></div><div><dt className="text-xs text-[var(--muted)]">Review revision</dt><dd className="mt-1"><code title={session.source_commit_id}>{short(session.source_commit_id)}</code></dd></div><div><dt className="text-xs text-[var(--muted)]">Created</dt><dd className="mt-1">{formatTime(session.created_at)}</dd></div></dl></Card></aside>
    </div>
  </div>;
}
