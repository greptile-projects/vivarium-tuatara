"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
} from "react";
import {
  api,
  type Proposal,
  type ProposalComment,
  type ProposalTask,
  type ProposalTaskChange,
  type Repository,
  type User,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Icons } from "./icons";
import { Avatar, Badge, Button, Card } from "./ui";

type ProposalRow = Proposal & { repository: Repository };
type StatusFilter = "open" | "closed" | "all";

async function allPages<T>(
  path: string,
  key: string,
  token?: string | null,
): Promise<T[]> {
  const items: T[] = [];
  let after: string | null = null;
  do {
    const separator = path.includes("?") ? "&" : "?";
    const query = `${path}${separator}limit=100${after ? `&after=${encodeURIComponent(after)}` : ""}`;
    const page = await api<Record<string, T[] | string | null>>(query, {}, token);
    items.push(...((page[key] as T[]) ?? []));
    after = page.next_cursor as string | null;
  } while (after);
  return items;
}

function message(reason: unknown, fallback: string) {
  return reason instanceof Error ? reason.message : fallback;
}

function date(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(new Date(value));
}

function initials(user?: User) {
  return (user?.display_name ?? "Unknown user")
    .split(/\s+/)
    .map((part) => part[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
}

export function ProposalsWorkspace() {
  const { token, user, loading: authLoading } = useAuth();
  const router = useRouter();
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [proposals, setProposals] = useState<ProposalRow[]>([]);
  const [authors, setAuthors] = useState<Record<string, User>>({});
  const [status, setStatus] = useState<StatusFilter>("open");
  const [repositoryID, setRepositoryID] = useState("all");
  const [query, setQuery] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const loadGeneration = useRef(0);

  const load = useCallback(async () => {
    if (authLoading) return;
    const generation = ++loadGeneration.current;
    const active = () => loadGeneration.current === generation;
    if (!token) {
      setRepositories([]);
      setProposals([]);
      setAuthors({});
      setError("");
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const repos = await allPages<Repository>("/repositories", "repositories", token);
      if (!active()) return;
      setRepositories(repos);
      const groups = await Promise.all(
        repos.map(async (repository) =>
          (await allPages<Proposal>(
            `/repositories/${repository.id}/proposals`,
            "proposals",
            token,
          )).map((proposal) => ({ ...proposal, repository })),
        ),
      );
      if (!active()) return;
      const found = groups.flat().sort((a, b) =>
        b.updated_at.localeCompare(a.updated_at),
      );
      setProposals(found);
      const ids = [...new Set(found.map((proposal) => proposal.author_id))];
      const people = await Promise.all(
        ids.map((id) => api<User>(`/users/${id}`, {}, token).catch(() => null)),
      );
      if (!active()) return;
      setAuthors(
        Object.fromEntries(people.filter((person): person is User => Boolean(person)).map((person) => [person.id, person])),
      );
    } catch (reason) {
      if (active())
        setError(message(reason, "Proposals could not be loaded."));
    } finally {
      if (active()) setLoading(false);
    }
  }, [authLoading, token]);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  const filtered = useMemo(() => {
    const term = query.trim().toLocaleLowerCase();
    return proposals.filter(
      (proposal) =>
        (status === "all" || proposal.status === status) &&
        (repositoryID === "all" || proposal.repository_id === repositoryID) &&
        (!term ||
          proposal.title.toLocaleLowerCase().includes(term) ||
          proposal.body.toLocaleLowerCase().includes(term) ||
          proposal.repository.name.toLocaleLowerCase().includes(term)),
    );
  }, [proposals, query, repositoryID, status]);

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError("");
    const data = new FormData(event.currentTarget);
    const repo = String(data.get("repository_id") ?? "");
    try {
      const proposal = await api<Proposal>(
        `/repositories/${repo}/proposals`,
        {
          method: "POST",
          body: JSON.stringify({ title: data.get("title"), body: data.get("body") }),
        },
        token,
      );
      router.push(`/proposals/${repo}/${proposal.id}`);
    } catch (reason) {
      setError(message(reason, "Proposal could not be created."));
    } finally {
      setPending(false);
    }
  }

  if (authLoading || loading)
    return <Card className="p-8 text-sm text-[var(--muted)]">Gathering proposal context…</Card>;
  if (!user)
    return (
      <Card className="p-8 text-center">
        <span className="mx-auto grid size-12 place-items-center rounded-xl bg-[var(--brand-soft)] text-[var(--brand)]"><Icons.Spark /></span>
        <h1 className="mt-4 text-2xl font-semibold">Find the context before the code</h1>
        <p className="mx-auto mt-2 max-w-lg text-sm leading-6 text-[var(--muted)]">Sign in to discover proposals across the repositories you collaborate on.</p>
        <Link href="/?access=signin" className="mt-5 inline-flex min-h-10 items-center rounded-lg bg-[var(--brand)] px-4 text-sm font-semibold text-white">Sign in to discover proposals</Link>
      </Card>
    );

  const openCount = proposals.filter((proposal) => proposal.status === "open").length;
  return (
    <div className="space-y-7">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="font-mono text-xs font-semibold uppercase tracking-[.16em] text-[var(--brand)]">Shape work together</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-[-.035em]">Proposals</h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--muted)]">Search the ideas already in motion, add context, or open a focused conversation before starting duplicate work.</p>
        </div>
        <Button onClick={() => setShowCreate((shown) => !shown)} disabled={!repositories.length}><Icons.Plus size={16} />{showCreate ? "Cancel" : "New proposal"}</Button>
      </header>

      {showCreate && (
        <Card className="p-5 sm:p-6">
          <h2 className="text-lg font-semibold">Start with the purpose</h2>
          <p className="mt-1 text-sm text-[var(--muted)]">Describe the problem and intended outcome so collaborators can build on existing context.</p>
          <form onSubmit={create} className="mt-5 grid gap-4">
            <label className="text-sm font-semibold">Repository<select name="repository_id" required className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal">{repositories.map((repository) => <option value={repository.id} key={repository.id}>{repository.name}</option>)}</select></label>
            <label className="text-sm font-semibold">Title<input name="title" required maxLength={200} placeholder="What should change?" className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal outline-none focus:border-[var(--brand)]" /></label>
            <label className="text-sm font-semibold">Context<textarea name="body" required maxLength={10000} rows={6} placeholder="Explain the problem, constraints, and what a good outcome looks like…" className="mt-2 w-full rounded-lg border border-[var(--line-strong)] bg-white p-3 font-normal leading-6 outline-none focus:border-[var(--brand)]" /></label>
            <div><Button type="submit" disabled={pending}>{pending ? "Publishing…" : "Publish proposal"}</Button></div>
          </form>
        </Card>
      )}

      {error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}

      <section aria-labelledby="proposal-list-heading">
        <div className="grid gap-3 rounded-xl border border-[var(--line)] bg-[var(--surface)] p-3 lg:grid-cols-[minmax(15rem,1fr)_12rem_auto]">
          <label className="relative"><span className="sr-only">Search proposals</span><Icons.Search size={16} className="pointer-events-none absolute left-3 top-3 text-[var(--muted)]" /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search titles, context, repositories…" className="min-h-10 w-full rounded-lg border border-[var(--line-strong)] bg-white pl-9 pr-3 text-sm outline-none focus:border-[var(--brand)]" /></label>
          <label><span className="sr-only">Filter by repository</span><select value={repositoryID} onChange={(event) => setRepositoryID(event.target.value)} className="min-h-10 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"><option value="all">All repositories</option>{repositories.map((repository) => <option value={repository.id} key={repository.id}>{repository.name}</option>)}</select></label>
          <div className="flex rounded-lg bg-black/[.045] p-1" aria-label="Filter proposals by status">{(["open", "closed", "all"] as const).map((value) => <button type="button" key={value} aria-pressed={status === value} onClick={() => setStatus(value)} className={`min-h-8 rounded-md px-3 text-sm font-semibold capitalize ${status === value ? "bg-white shadow-sm" : "text-[var(--muted)]"}`}>{value}</button>)}</div>
        </div>
        <div className="mt-4 flex items-baseline justify-between"><h2 id="proposal-list-heading" className="text-lg font-semibold">Shared context</h2><p className="text-xs text-[var(--muted)]">{openCount} open · {filtered.length} shown</p></div>
        <Card className="mt-3 overflow-hidden">
          {!repositories.length ? <Empty title="No collaborative spaces yet" body="Create a repository or join one to begin shaping work together." /> : !filtered.length ? <Empty title="No proposals match" body={proposals.length ? "Try another search or status filter." : "Open the first proposal to give future work a shared starting point."} /> : <div className="divide-y divide-[var(--line)]">{filtered.map((proposal) => {
            const author = authors[proposal.author_id];
            return <Link href={`/proposals/${proposal.repository_id}/${proposal.id}`} key={proposal.id} className="group flex gap-4 p-5 transition hover:bg-[var(--brand-soft)] sm:p-6">
              <Avatar initials={initials(author)} label={author?.display_name ?? "Unknown author"} />
              <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><h3 className="font-semibold group-hover:text-[var(--brand-strong)] group-hover:underline">{proposal.title}</h3><Badge tone={proposal.status === "open" ? "success" : "neutral"}>{proposal.status}</Badge></div><p className="mt-1 line-clamp-2 text-sm leading-6 text-[var(--muted)]">{proposal.body}</p><p className="mt-2 text-xs text-[var(--muted)]"><span className="font-mono font-semibold text-[var(--ink)]">{proposal.repository.name}</span> · {author ? `@${author.handle}` : "unknown author"} · updated {date(proposal.updated_at)}</p></div><Icons.Chevron className="mt-2 shrink-0 text-[var(--muted)]" size={16} />
            </Link>;
          })}</div>}
        </Card>
      </section>
    </div>
  );
}

function Empty({ title, body }: { title: string; body: string }) {
  return <div className="p-9 text-center"><h3 className="font-semibold">{title}</h3><p className="mt-2 text-sm text-[var(--muted)]">{body}</p></div>;
}

export function ProposalConversation({ repositoryID, proposalID }: { repositoryID: string; proposalID: string }) {
  const { token, user, loading: authLoading } = useAuth();
  const [repository, setRepository] = useState<Repository | null>(null);
  const [proposal, setProposal] = useState<Proposal | null>(null);
  const [comments, setComments] = useState<ProposalComment[]>([]);
  const [tasks, setTasks] = useState<ProposalTask[]>([]);
  const [taskHistory, setTaskHistory] = useState<Record<string, ProposalTaskChange[]>>({});
  const [authors, setAuthors] = useState<Record<string, User>>({});
  const [participant, setParticipant] = useState(false);
  const [editing, setEditing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [started, setStarted] = useState<Record<string, { session: { id: string }; run: { id: string; working_branch: string; state?: string }; credential?: { token?: string; expires_at: string } }>>({});
  const [error, setError] = useState("");
  const loadGeneration = useRef(0);

  const load = useCallback(async () => {
    if (authLoading) return;
    const generation = ++loadGeneration.current;
    const active = () => loadGeneration.current === generation;
    setLoading(true);
    setError("");
    try {
      const [repo, item, discussion, plan] = await Promise.all([
        api<Repository>(`/repositories/${repositoryID}`, {}, token),
        api<Proposal>(`/repositories/${repositoryID}/proposals/${proposalID}`, {}, token),
        allPages<ProposalComment>(`/repositories/${repositoryID}/proposals/${proposalID}/comments`, "comments", token),
        api<{ tasks: ProposalTask[] }>(`/repositories/${repositoryID}/proposals/${proposalID}/tasks`, {}, token),
      ]);
      if (!active()) return;
      const planTasks = (plan.tasks ?? []).map((task) => ({
        ...task,
        dependency_ids: task.dependency_ids ?? [],
        discussion_comment_ids: task.discussion_comment_ids ?? [],
        blocked_by: task.blocked_by ?? [],
      }));
      setRepository(repo); setProposal(item); setComments(discussion); setTasks(planTasks);
      if (token) {
        const available = await allPages<Repository>("/repositories", "repositories", token);
        if (!active()) return;
        const participates = available.some((candidate) => candidate.id === repositoryID);
        setParticipant(participates);
        if (participates) {
          const existing = await Promise.all(planTasks.map(async (task) => {
            if (task.assignment?.assignee_type !== "agent") return [task.id, undefined] as const;
            const sessions = await allPages<{ id: string }>(`/repositories/${repositoryID}/proposals/${proposalID}/tasks/${task.id}/sessions`, "sessions", token);
            const expectedBranch = `agent/tasks/${task.id}-${task.assignment.id.slice(0, 8)}`;
            for (const session of sessions.toReversed()) {
              const runs = (await api<{ runs: { id: string; working_branch: string; state: string }[] }>(`/repositories/${repositoryID}/proposals/${proposalID}/tasks/${task.id}/sessions/${session.id}/runs`, {}, token)).runs;
              const matching = runs.filter((run) => run.working_branch === expectedBranch);
              if (matching.length > 0) return [task.id, { session, run: matching.find((run) => run.state === "completed") ?? matching[0] }] as const;
            }
            return [task.id, undefined] as const;
          }));
          if (!active()) return;
          setStarted(Object.fromEntries(existing.filter((entry): entry is readonly [string, { session: { id: string }; run: { id: string; working_branch: string; state: string } }] => Boolean(entry[1]?.run))));
        }
      } else {
        setParticipant(false);
      }
      const histories = await Promise.all(planTasks.map(async (task) => [task.id, (await api<{ history: ProposalTaskChange[] }>(`/repositories/${repositoryID}/proposals/${proposalID}/tasks/${task.id}/history`, {}, token)).history ?? []] as const));
      if (!active()) return;
      setTaskHistory(Object.fromEntries(histories));
      const ids = [...new Set([item.author_id, ...discussion.map((comment) => comment.author_id), ...planTasks.flatMap((task) => [task.created_by, task.updated_by, task.assignment?.assigned_by, task.assignment?.assignee_type === "human" ? task.assignment.assignee_id : undefined]), ...histories.flatMap(([, changes]) => changes.map((change) => change.actor_id))].filter((id): id is string => Boolean(id)))];
      const people = await Promise.all(ids.map((id) => api<User>(`/users/${id}`, {}, token).catch(() => null)));
      if (!active()) return;
      setAuthors(Object.fromEntries(people.filter((person): person is User => Boolean(person)).map((person) => [person.id, person])));
    } catch (reason) {
      if (active()) setError(message(reason, "Proposal could not be loaded."));
    } finally {
      if (active()) setLoading(false);
    }
  }, [authLoading, proposalID, repositoryID, token]);

  useEffect(() => { void Promise.resolve().then(load); }, [load]);

  async function update(payload: { title?: FormDataEntryValue | null; body?: FormDataEntryValue | null; status?: "closed" }) {
    setPending(true); setError("");
    try {
      const updated = await api<Proposal>(`/repositories/${repositoryID}/proposals/${proposalID}`, { method: "PATCH", body: JSON.stringify(payload) }, token);
      setProposal(updated); setEditing(false);
    } catch (reason) { setError(message(reason, "Proposal could not be updated.")); }
    finally { setPending(false); }
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const data = new FormData(event.currentTarget);
    await update({ title: data.get("title"), body: data.get("body") });
  }

  async function comment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setPending(true); setError("");
    const form = event.currentTarget; const data = new FormData(form);
    try {
      const created = await api<ProposalComment>(`/repositories/${repositoryID}/proposals/${proposalID}/comments`, { method: "POST", body: JSON.stringify({ body: data.get("body") }) }, token);
      setComments((items) => [...items, created]); form.reset();
      if (user) setAuthors((people) => ({ ...people, [user.id]: user }));
    } catch (reason) { setError(message(reason, "Comment could not be published.")); }
    finally { setPending(false); }
  }

  async function createTask(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setPending(true); setError("");
    const form = event.currentTarget; const data = new FormData(form);
    try {
      await api<ProposalTask>(`/repositories/${repositoryID}/proposals/${proposalID}/tasks`, { method: "POST", body: JSON.stringify({ title: data.get("title"), outcome: data.get("outcome"), dependency_ids: data.getAll("dependency_ids"), discussion_comment_ids: data.getAll("discussion_comment_ids") }) }, token);
      form.reset(); await load();
    } catch (reason) { setError(message(reason, "Task could not be added.")); }
    finally { setPending(false); }
  }

  async function updateTask(taskID: string, payload: Record<string, unknown>) {
    setPending(true); setError("");
    try { await api<ProposalTask>(`/repositories/${repositoryID}/proposals/${proposalID}/tasks/${taskID}`, { method: "PATCH", body: JSON.stringify(payload) }, token); await load(); }
    catch (reason) { setError(message(reason, "Task could not be updated.")); }
    finally { setPending(false); }
  }

  async function editTask(event: FormEvent<HTMLFormElement>, taskID: string) {
    event.preventDefault(); const data = new FormData(event.currentTarget);
    await updateTask(taskID, { title: data.get("title"), outcome: data.get("outcome"), dependency_ids: data.getAll("dependency_ids"), discussion_comment_ids: data.getAll("discussion_comment_ids") });
  }

  async function assignTask(event: FormEvent<HTMLFormElement>, task: ProposalTask) {
	event.preventDefault(); const data = new FormData(event.currentTarget); const kind = String(data.get("assignee_type"));
	setPending(true); setError("");
	try {
		await api<ProposalTask>(`/repositories/${repositoryID}/proposals/${proposalID}/tasks/${task.id}/assignment`, { method: "PUT", body: JSON.stringify({ assignee_type: kind, assignee_id: kind === "human" ? data.get("assignee_id") : undefined, mandate: data.get("mandate"), repository_id: repositoryID, base_revision: data.get("base_revision"), expected_assignment_id: task.assignment?.id ?? "" }) }, token);
		await load();
	} catch (reason) { setError(message(reason, "Task ownership could not be changed. Reload if another collaborator claimed it.")); }
	finally { setPending(false); }
  }

  async function revokeAssignment(task: ProposalTask) {
	if (!task.assignment) return; setPending(true); setError("");
	try { await api<ProposalTask>(`/repositories/${repositoryID}/proposals/${proposalID}/tasks/${task.id}/assignment?expected_assignment_id=${task.assignment.id}`, { method: "DELETE" }, token); await load(); }
	catch (reason) { setError(message(reason, "Task ownership could not be revoked. Reload if it changed.")); }
	finally { setPending(false); }
  }

  async function rebaseTask(event: FormEvent<HTMLFormElement>, task: ProposalTask) {
	event.preventDefault(); if (!task.assignment) return; const data = new FormData(event.currentTarget); setPending(true); setError("");
	try {
		await api<ProposalTask>(`/repositories/${repositoryID}/proposals/${proposalID}/tasks/${task.id}/rebase`, { method: "POST", body: JSON.stringify({ base_revision: data.get("base_revision"), expected_assignment_id: task.assignment.id }) }, token);
		await load();
	} catch (reason) { setError(message(reason, "Task context could not be rebased. Reload if its ownership changed.")); }
	finally { setPending(false); }
  }

  async function startAgentTask(task: ProposalTask) {
	if (!task.assignment) return; setPending(true); setError("");
	try {
		const launched = await api<{ session: { id: string }; run: { id: string; working_branch: string }; credential: { token?: string; expires_at: string } }>(`/repositories/${repositoryID}/proposals/${proposalID}/tasks/${task.id}/sessions`, { method: "POST", body: JSON.stringify({ expected_assignment_id: task.assignment.id, context_paths: [], expires_in: 3600 }) }, token);
		setStarted((current) => ({ ...current, [task.id]: launched }));
	} catch (reason) { setError(message(reason, "Agent work could not be started. Reload if the assignment changed.")); }
	finally { setPending(false); }
  }

  async function publishTask(event: FormEvent<HTMLFormElement>, task: ProposalTask) {
	event.preventDefault(); setPending(true); setError(""); const form = event.currentTarget; const data = new FormData(form); const launch = started[task.id];
	try {
		const pull = await api<{ id: string }>(`/repositories/${repositoryID}/proposals/${proposalID}/tasks/${task.id}/contributions`, { method: "POST", body: JSON.stringify({ title: data.get("title"), body: data.get("body"), source_branch: data.get("source_branch"), target_branch: data.get("target_branch"), session_id: launch?.session.id, run_id: launch?.run?.id }) }, token);
		window.location.assign(`/pulls/${repositoryID}/${pull.id}`);
	} catch (reason) { setError(message(reason, "Task work could not be published for review.")); setPending(false); }
  }

  if (loading) return <Card className="p-8 text-sm text-[var(--muted)]">Opening the conversation…</Card>;
  if (error && (!proposal || !repository)) return <Card className="p-8"><h1 className="text-xl font-semibold">Proposal unavailable</h1><p role="alert" className="mt-2 text-sm text-[var(--danger)]">{error}</p><Link href="/proposals" className="mt-5 inline-flex text-sm font-semibold text-[var(--brand)]">Back to proposals</Link></Card>;
  if (!proposal || !repository) return null;
  const author = authors[proposal.author_id];
  const canEdit = participant && user?.id === proposal.author_id && proposal.status === "open";
  const canClose = participant && proposal.status === "open" && (user?.id === proposal.author_id || user?.id === repository.owner_id);
  return <div className="space-y-6">
    <header><Link href="/proposals" className="text-sm text-[var(--muted)] hover:text-[var(--brand)]">Proposals</Link><p className="mt-3 font-mono text-xs font-semibold text-[var(--brand)]">{repository.name}</p><div className="mt-2 flex flex-wrap items-start gap-3"><h1 className="max-w-4xl text-3xl font-semibold tracking-[-.035em]">{proposal.title}</h1><Badge tone={proposal.status === "open" ? "success" : "neutral"}>{proposal.status}</Badge></div><p className="mt-3 text-sm text-[var(--muted)]">Opened by {author ? `@${author.handle}` : "an unknown author"} on {date(proposal.created_at)} · updated {date(proposal.updated_at)}</p></header>
    {error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_18rem]">
      <section className="space-y-5" aria-label="Proposal conversation">
        <Card className="overflow-hidden"><div className="flex items-center justify-between border-b border-[var(--line)] bg-[var(--surface)] px-5 py-3"><div className="flex items-center gap-2"><Avatar initials={initials(author)} label={author?.display_name ?? "Unknown author"} size="sm" /><span className="text-sm font-semibold">{author ? `@${author.handle}` : "Author"}</span></div>{canEdit && !editing && <Button variant="quiet" onClick={() => setEditing(true)}>Edit</Button>}</div>{editing ? <form onSubmit={save} className="space-y-4 p-5"><label className="block text-sm font-semibold">Title<input name="title" defaultValue={proposal.title} required maxLength={200} className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] px-3 font-normal" /></label><label className="block text-sm font-semibold">Context<textarea name="body" defaultValue={proposal.body} required maxLength={10000} rows={9} className="mt-2 w-full rounded-lg border border-[var(--line-strong)] p-3 font-normal leading-6" /></label><div className="flex gap-2"><Button type="submit" disabled={pending}>{pending ? "Saving…" : "Save changes"}</Button><Button type="button" variant="secondary" onClick={() => setEditing(false)}>Cancel</Button></div></form> : <div className="whitespace-pre-wrap p-5 text-sm leading-7 sm:p-6">{proposal.body}</div>}</Card>
        <section aria-labelledby="plan-heading" className="space-y-3">
          <div><p className="font-mono text-xs font-semibold uppercase tracking-[.14em] text-[var(--brand)]">Executable plan</p><h2 id="plan-heading" className="mt-1 text-xl font-semibold">What can start now</h2><p className="mt-1 text-sm text-[var(--muted)]">Tasks are ordered for delivery. Readiness follows completed dependencies.</p></div>
          {!tasks.length ? <Card className="p-6 text-sm text-[var(--muted)]">No tasks yet. Break the agreed direction into its first concrete outcome.</Card> : tasks.map((task, index) => {
            const blockerNames = task.blocked_by.map((id) => tasks.find((candidate) => candidate.id === id)?.title ?? id);
            const linkedComments = task.discussion_comment_ids.map((id) => comments.findIndex((comment) => comment.id === id) + 1).filter(Boolean);
            return <Card key={task.id} className="p-5"><div className="flex gap-4"><span className="grid size-8 shrink-0 place-items-center rounded-full bg-[var(--brand-soft)] font-mono text-sm font-semibold text-[var(--brand)]">{index + 1}</span><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><h3 className="font-semibold">{task.title}</h3><Badge tone={task.ready ? "success" : task.status === "cancelled" ? "neutral" : "warning"}>{task.ready ? "ready" : task.status.replace("_", " ")}</Badge>{task.assignment && <Badge tone="info">owned by {task.assignment.assignee_type}</Badge>}{task.context_state !== "current" && <Badge tone="warning">{task.context_state}</Badge>}</div><p className="mt-2 text-sm leading-6"><span className="font-semibold">Outcome:</span> {task.outcome}</p>{task.reasoning&&<div className="mt-3 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3 text-xs"><p className="font-semibold">Reasoning carried into implementation</p><p className="mt-1"><Link className="text-[var(--brand)] hover:underline" href={`/repositories/${repositoryID}/impact`}>Assessment {task.reasoning.assessment_id.slice(0,8)}</Link> at <code>{task.reasoning.revision.slice(0,12)}</code></p>{task.reasoning.items.map(item=><p key={item.id} className="mt-1"><Badge tone={item.status==="verification_required"?"warning":"info"}>{item.kind}</Badge> {item.summary}</p>)}</div>}{task.context_state !== "current" && <p role="status" className="mt-3 rounded-lg border border-[var(--warning)] bg-[var(--warning-soft)] p-3 text-xs">The plan changed after this work boundary was created. Existing sessions and pull requests remain visible, but cannot represent revision {task.context_revision}. Rebase the assignment, then publish replacement work from the current plan.</p>}{blockerNames.length > 0 && <p className="mt-2 text-xs text-[var(--muted)]">Blocked by {blockerNames.join(", ")}</p>}{linkedComments.length > 0 && <p className="mt-2 text-xs text-[var(--muted)]">Motivated by discussion {linkedComments.map((number) => `#${number}`).join(", ")}</p>}
              {task.assignment && <div className="mt-4 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-4 text-xs"><p><span className="font-semibold">Accountable owner:</span> {task.assignment.assignee_type === "human" && authors[task.assignment.assignee_id] ? `@${authors[task.assignment.assignee_id].handle}` : `${task.assignment.assignee_type} ${task.assignment.assignee_id.slice(0, 8)}`}</p><p className="mt-2 whitespace-pre-wrap"><span className="font-semibold">Mandate:</span> {task.assignment.mandate}</p><p className="mt-2 break-all font-mono text-[var(--muted)]">Repository {task.assignment.access.repository_id.slice(0, 8)} · base {task.assignment.access.base_revision} · context r{task.assignment.context_revision} · {task.assignment.access.scopes.join(", ")} · {task.assignment.access.branch}</p><p className="mt-2 text-[var(--muted)]">Assigned by {authors[task.assignment.assigned_by] ? `@${authors[task.assignment.assigned_by].handle}` : task.assignment.assigned_by.slice(0, 8)} on {date(task.assignment.assigned_at)}</p>{participant && proposal.status === "open" && task.assignment.context_revision === task.context_revision && task.assignment.assignee_type === "agent" && !started[task.id] && <Button className="mt-3" disabled={pending} onClick={() => void startAgentTask(task)}>{pending ? "Starting…" : "Start agent work"}</Button>}{participant && proposal.status === "open" && !started[task.id] && <Button className="mt-3" variant="quiet" disabled={pending} onClick={() => void revokeAssignment(task)}>Revoke ownership</Button>}{started[task.id] && <div role="status" className="mt-3 rounded-lg border border-[var(--brand)] bg-[var(--brand-soft)] p-3"><p className="font-semibold">Agent workspace started from the assigned revision</p><p className="mt-1 break-all font-mono">{started[task.id].run ? `${started[task.id].run?.working_branch} · ` : ""}session {started[task.id].session.id}</p><p className="mt-2 text-[var(--muted)]">Collaborators can reconnect through the task session API.</p>{started[task.id].credential?.token && <><p className="mt-2 text-[var(--muted)]">Copy the one-time bounded credential now.</p><code className="mt-2 block break-all rounded bg-white p-2 select-all">{started[task.id].credential?.token}</code></>}</div>}</div>}
              {task.contribution && <div className="mt-3 rounded-lg border border-[var(--line)] p-3 text-xs"><Badge tone={task.contribution.status === "merged" ? "success" : task.contribution.status === "review" ? "info" : "neutral"}>{task.contribution.status}</Badge><p className="mt-2">Candidate <Link className="font-semibold text-[var(--brand)]" href={`/pulls/${repositoryID}/${task.contribution.pull_request_id}`}>pull request</Link> at <code>{task.contribution.source_commit_id.slice(0, 12)}</code></p>{task.contribution.session_id && <p className="mt-1">Execution evidence: session <code>{task.contribution.session_id.slice(0, 8)}</code> · run <code>{task.contribution.run_id?.slice(0, 8)}</code></p>}</div>}
              {task.assignment && (!task.contribution || task.context_state === "obsolete") && task.assignment.context_revision === task.context_revision && proposal.status === "open" && (task.assignment.assignee_type === "agent" ? participant && started[task.id]?.run.state === "completed" : task.assignment.assignee_id === user?.id) && <details className="mt-3"><summary className="cursor-pointer text-xs font-semibold text-[var(--brand)]">Publish work for review</summary><form onSubmit={(event) => void publishTask(event, task)} className="mt-3 grid gap-3 rounded-lg bg-[var(--surface)] p-4"><label className="text-xs font-semibold">Pull request title<input name="title" required defaultValue={task.title} className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal" /></label><label className="text-xs font-semibold">Review context<textarea name="body" required defaultValue={task.outcome} rows={3} className="mt-1 w-full rounded-lg border p-3 font-normal" /></label><label className="text-xs font-semibold">Working branch<input name="source_branch" required defaultValue={started[task.id]?.run.working_branch} className="mt-1 min-h-10 w-full rounded-lg border px-3 font-mono font-normal" /></label><label className="text-xs font-semibold">Target branch<input name="target_branch" required defaultValue="main" className="mt-1 min-h-10 w-full rounded-lg border px-3 font-mono font-normal" /></label><Button type="submit" disabled={pending}>Create connected pull request</Button></form></details>}
              {participant && proposal.status === "open" && task.ready && task.status === "todo" && <details className="mt-3"><summary className="cursor-pointer text-xs font-semibold text-[var(--brand)]">{task.assignment ? "Reassign owner" : "Claim or assign owner"}</summary><form onSubmit={(event) => void assignTask(event, task)} className="mt-3 grid gap-3 rounded-lg bg-[var(--surface)] p-4"><label className="text-xs font-semibold">Owner type<select name="assignee_type" className="mt-1 min-h-10 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal"><option value="human">Human collaborator</option><option value="agent">Available agent</option></select></label><label className="text-xs font-semibold">Human collaboration ID<input name="assignee_id" defaultValue={user?.id} pattern="[0-9a-f]{32}" className="mt-1 min-h-10 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-mono font-normal" /></label><label className="text-xs font-semibold">Mandate<textarea name="mandate" required maxLength={4000} rows={3} defaultValue={task.outcome} className="mt-1 w-full rounded-lg border border-[var(--line-strong)] bg-white p-3 font-normal" /></label><label className="text-xs font-semibold">Exact base commit<input name="base_revision" required pattern="[0-9a-fA-F]{40}" placeholder="40-character commit ID" className="mt-1 min-h-10 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-mono font-normal" /></label><p className="text-xs text-[var(--muted)]">Agent access is limited to Git read/write for this repository and the task branch created at start. Human assignment grants no new repository access.</p><div><Button type="submit" disabled={pending}>{task.assignment ? "Reassign" : "Set owner"}</Button></div></form></details>}
              {participant && proposal.status === "open" && task.assignment && task.status !== "completed" && task.status !== "cancelled" && <details className="mt-3"><summary className="cursor-pointer text-xs font-semibold text-[var(--brand)]">Rebase starting context</summary><form onSubmit={(event) => void rebaseTask(event, task)} className="mt-3 grid gap-3 rounded-lg bg-[var(--surface)] p-4"><p className="text-xs text-[var(--muted)]">This creates a new ownership boundary at the current plan revision. Existing sessions and pull requests remain attributable but become obsolete.</p><label className="text-xs font-semibold">New exact base commit<input name="base_revision" required pattern="[0-9a-fA-F]{40}" defaultValue={task.assignment.access.base_revision} className="mt-1 min-h-10 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-mono font-normal" /></label><div><Button type="submit" disabled={pending}>Rebase task</Button></div></form></details>}
              {participant && proposal.status === "open" && <div className="mt-4 flex flex-wrap gap-2"><select aria-label={`Status for ${task.title}`} value={task.status} disabled={pending} onChange={(event) => void updateTask(task.id, { status: event.target.value })} className="min-h-9 rounded-lg border border-[var(--line-strong)] bg-white px-2 text-xs"><option value="todo">To do</option><option value="in_progress">In progress</option><option value="completed">Completed</option><option value="cancelled">Cancelled</option></select><Button variant="quiet" disabled={pending || index === 0} onClick={() => void updateTask(task.id, { position: index - 1 })}>Move up</Button><Button variant="quiet" disabled={pending || index === tasks.length - 1} onClick={() => void updateTask(task.id, { position: index + 1 })}>Move down</Button></div>}
              {participant && proposal.status === "open" && <details className="mt-3"><summary className="cursor-pointer text-xs font-semibold text-[var(--brand)]">Edit task definition</summary><form onSubmit={(event) => void editTask(event, task.id)} className="mt-3 grid gap-3 rounded-lg bg-[var(--surface)] p-4"><label className="text-xs font-semibold">Task<input name="title" required maxLength={200} defaultValue={task.title} className="mt-1 min-h-10 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal" /></label><label className="text-xs font-semibold">Expected outcome<textarea name="outcome" required maxLength={2000} rows={3} defaultValue={task.outcome} className="mt-1 w-full rounded-lg border border-[var(--line-strong)] bg-white p-3 font-normal" /></label>{tasks.length > 1 && <fieldset><legend className="text-xs font-semibold">Dependencies</legend><div className="mt-2 grid gap-2 sm:grid-cols-2">{tasks.filter((candidate) => candidate.id !== task.id).map((candidate) => <label key={candidate.id} className="flex items-center gap-2 text-xs"><input type="checkbox" name="dependency_ids" value={candidate.id} defaultChecked={task.dependency_ids.includes(candidate.id)} />{candidate.title}</label>)}</div></fieldset>}{comments.length > 0 && <fieldset><legend className="text-xs font-semibold">Motivating discussion</legend><div className="mt-2 grid gap-2">{comments.map((item, commentIndex) => <label key={item.id} className="flex items-start gap-2 text-xs"><input className="mt-0.5" type="checkbox" name="discussion_comment_ids" value={item.id} defaultChecked={task.discussion_comment_ids.includes(item.id)} /><span>#{commentIndex + 1} {item.body.slice(0, 100)}</span></label>)}</div></fieldset>}<div><Button type="submit" disabled={pending}>Save task</Button></div></form></details>}
              <details className="mt-3 text-xs text-[var(--muted)]"><summary className="cursor-pointer font-semibold">Decision history ({taskHistory[task.id]?.length ?? 0})</summary><ol className="mt-2 space-y-1">{(taskHistory[task.id] ?? []).map((change) => <li key={change.id}>{authors[change.actor_id] ? `@${authors[change.actor_id].handle}` : "Unknown actor"} {change.action.replace("_", " ")} this task on {date(change.created_at)}</li>)}</ol></details>
            </div></div></Card>;
          })}
          {participant && proposal.status === "open" && <Card className="p-5"><h3 className="font-semibold">Add an actionable task</h3><form onSubmit={createTask} className="mt-4 grid gap-4"><label className="text-sm font-semibold">Task<input name="title" required maxLength={200} placeholder="A concrete piece of work" className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] px-3 font-normal" /></label><label className="text-sm font-semibold">Expected outcome<textarea name="outcome" required maxLength={2000} rows={3} placeholder="What will be true when this task succeeds?" className="mt-2 w-full rounded-lg border border-[var(--line-strong)] p-3 font-normal" /></label>{tasks.length > 0 && <fieldset><legend className="text-sm font-semibold">Depends on</legend><div className="mt-2 grid gap-2 sm:grid-cols-2">{tasks.map((task) => <label key={task.id} className="flex items-center gap-2 text-sm"><input type="checkbox" name="dependency_ids" value={task.id} />{task.title}</label>)}</div></fieldset>}{comments.length > 0 && <fieldset><legend className="text-sm font-semibold">Motivating discussion</legend><div className="mt-2 grid gap-2">{comments.map((item, index) => <label key={item.id} className="flex items-start gap-2 text-sm"><input className="mt-1" type="checkbox" name="discussion_comment_ids" value={item.id} /><span>#{index + 1} {item.body.slice(0, 100)}</span></label>)}</div></fieldset>}<div><Button type="submit" disabled={pending}>{pending ? "Adding…" : "Add task"}</Button></div></form></Card>}
        </section>
        <div className="flex items-center gap-3"><span className="h-px flex-1 bg-[var(--line)]" /><h2 className="text-sm font-semibold text-[var(--muted)]">{comments.length} {comments.length === 1 ? "comment" : "comments"}</h2><span className="h-px flex-1 bg-[var(--line)]" /></div>
        {comments.map((item) => { const person = authors[item.author_id]; return <Card key={item.id} className="overflow-hidden"><div className="flex items-center gap-2 border-b border-[var(--line)] bg-[var(--surface)] px-5 py-3"><Avatar initials={initials(person)} label={person?.display_name ?? "Unknown commenter"} size="sm" /><span className="text-sm font-semibold">{person ? `@${person.handle}` : "Unknown user"}</span><span className="text-xs text-[var(--muted)]">commented {date(item.created_at)}</span></div><p className="whitespace-pre-wrap p-5 text-sm leading-7">{item.body}</p></Card>; })}
        {participant ? <Card className="p-5"><form onSubmit={comment}><label className="text-sm font-semibold">Add to the conversation<textarea name="body" required maxLength={10000} rows={5} placeholder="Share feedback, constraints, or a useful connection…" className="mt-2 w-full rounded-lg border border-[var(--line-strong)] bg-white p-3 font-normal leading-6 outline-none focus:border-[var(--brand)]" /></label><Button type="submit" disabled={pending} className="mt-3">{pending ? "Publishing…" : "Comment"}</Button></form></Card> : <Card className="p-5 text-sm text-[var(--muted)]">{user ? "Only current repository participants can join this conversation." : <><Link href="/?access=signin" className="font-semibold text-[var(--brand)]">Sign in</Link> to participate if you collaborate on this repository.</>}</Card>}
      </section>
      <aside className="space-y-4"><Card className="p-5"><h2 className="font-semibold">Proposal details</h2><dl className="mt-4 space-y-3 text-sm"><div><dt className="text-xs text-[var(--muted)]">Repository</dt><dd className="mt-1"><Link href={`/repositories/${repository.id}`} className="font-mono font-semibold text-[var(--brand)] hover:underline">{repository.name}</Link></dd></div><div><dt className="text-xs text-[var(--muted)]">Status</dt><dd className="mt-1 capitalize">{proposal.status}</dd></div>{proposal.closed_at && <div><dt className="text-xs text-[var(--muted)]">Closed</dt><dd className="mt-1">{date(proposal.closed_at)}</dd></div>}</dl></Card>{canClose && <Card className="p-5"><h2 className="font-semibold">Close proposal</h2><p className="mt-2 text-sm leading-6 text-[var(--muted)]">Close this conversation when the direction is resolved or no longer planned. It stays readable.</p><Button variant="secondary" className="mt-4 w-full" disabled={pending} onClick={() => void update({ status: "closed" })}>{pending ? "Closing…" : "Close proposal"}</Button></Card>}</aside>
    </div>
  </div>;
}
