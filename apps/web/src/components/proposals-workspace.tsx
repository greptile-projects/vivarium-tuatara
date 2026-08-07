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
  const [authors, setAuthors] = useState<Record<string, User>>({});
  const [participant, setParticipant] = useState(false);
  const [editing, setEditing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const loadGeneration = useRef(0);

  const load = useCallback(async () => {
    if (authLoading) return;
    const generation = ++loadGeneration.current;
    const active = () => loadGeneration.current === generation;
    setLoading(true);
    setError("");
    try {
      const [repo, item, discussion] = await Promise.all([
        api<Repository>(`/repositories/${repositoryID}`, {}, token),
        api<Proposal>(`/repositories/${repositoryID}/proposals/${proposalID}`, {}, token),
        allPages<ProposalComment>(`/repositories/${repositoryID}/proposals/${proposalID}/comments`, "comments", token),
      ]);
      if (!active()) return;
      setRepository(repo); setProposal(item); setComments(discussion);
      if (token) {
        const available = await allPages<Repository>("/repositories", "repositories", token);
        if (!active()) return;
        setParticipant(available.some((candidate) => candidate.id === repositoryID));
      } else {
        setParticipant(false);
      }
      const ids = [...new Set([item.author_id, ...discussion.map((comment) => comment.author_id)])];
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
        <div className="flex items-center gap-3"><span className="h-px flex-1 bg-[var(--line)]" /><h2 className="text-sm font-semibold text-[var(--muted)]">{comments.length} {comments.length === 1 ? "comment" : "comments"}</h2><span className="h-px flex-1 bg-[var(--line)]" /></div>
        {comments.map((item) => { const person = authors[item.author_id]; return <Card key={item.id} className="overflow-hidden"><div className="flex items-center gap-2 border-b border-[var(--line)] bg-[var(--surface)] px-5 py-3"><Avatar initials={initials(person)} label={person?.display_name ?? "Unknown commenter"} size="sm" /><span className="text-sm font-semibold">{person ? `@${person.handle}` : "Unknown user"}</span><span className="text-xs text-[var(--muted)]">commented {date(item.created_at)}</span></div><p className="whitespace-pre-wrap p-5 text-sm leading-7">{item.body}</p></Card>; })}
        {participant ? <Card className="p-5"><form onSubmit={comment}><label className="text-sm font-semibold">Add to the conversation<textarea name="body" required maxLength={10000} rows={5} placeholder="Share feedback, constraints, or a useful connection…" className="mt-2 w-full rounded-lg border border-[var(--line-strong)] bg-white p-3 font-normal leading-6 outline-none focus:border-[var(--brand)]" /></label><Button type="submit" disabled={pending} className="mt-3">{pending ? "Publishing…" : "Comment"}</Button></form></Card> : <Card className="p-5 text-sm text-[var(--muted)]">{user ? "Only current repository participants can join this conversation." : <><Link href="/?access=signin" className="font-semibold text-[var(--brand)]">Sign in</Link> to participate if you collaborate on this repository.</>}</Card>}
      </section>
      <aside className="space-y-4"><Card className="p-5"><h2 className="font-semibold">Proposal details</h2><dl className="mt-4 space-y-3 text-sm"><div><dt className="text-xs text-[var(--muted)]">Repository</dt><dd className="mt-1"><Link href={`/repositories/${repository.id}`} className="font-mono font-semibold text-[var(--brand)] hover:underline">{repository.name}</Link></dd></div><div><dt className="text-xs text-[var(--muted)]">Status</dt><dd className="mt-1 capitalize">{proposal.status}</dd></div>{proposal.closed_at && <div><dt className="text-xs text-[var(--muted)]">Closed</dt><dd className="mt-1">{date(proposal.closed_at)}</dd></div>}</dl></Card>{canClose && <Card className="p-5"><h2 className="font-semibold">Close proposal</h2><p className="mt-2 text-sm leading-6 text-[var(--muted)]">Close this conversation when the direction is resolved or no longer planned. It stays readable.</p><Button variant="secondary" className="mt-4 w-full" disabled={pending} onClick={() => void update({ status: "closed" })}>{pending ? "Closing…" : "Close proposal"}</Button></Card>}</aside>
    </div>
  </div>;
}
