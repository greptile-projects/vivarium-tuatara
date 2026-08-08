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
  type Branch,
  type Credential,
  type FileChange,
  type MergeReadiness,
  type Proposal,
  type PullRequest,
  type PullRequestComment,
  type PullRequestCommit,
  type PullRequestReview,
  type Repository,
  type User,
} from "@/lib/api";
import { useAuth } from "./auth";
import { ChangeSessionsCard } from "./change-sessions";
import { PullRequestChecks } from "./pull-request-checks";
import { Icons } from "./icons";
import { Avatar, Badge, Button, Card } from "./ui";

type PullRow = PullRequest & { repository: Repository };

async function allPages<T>(path: string, key: string, token?: string | null) {
  const items: T[] = [];
  let after: string | null = null;
  do {
    const separator = path.includes("?") ? "&" : "?";
    const page = await api<Record<string, T[] | string | null>>(
      `${path}${separator}limit=100${after ? `&after=${encodeURIComponent(after)}` : ""}`,
      {},
      token,
    );
    items.push(...((page[key] as T[]) ?? []));
    after = page.next_cursor as string | null;
  } while (after);
  return items;
}

const errorMessage = (reason: unknown, fallback: string) =>
  reason instanceof Error ? reason.message : fallback;
const formatDate = (value: string) =>
  new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(new Date(value));
const initials = (person?: User) =>
  (person?.display_name ?? "Unknown user")
    .split(/\s+/)
    .map((part) => part[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
const short = (id: string) => id.slice(0, 7);
const subject = (message: string) =>
  message.split("\n", 1)[0] || "Untitled commit";

export function PullRequestsWorkspace() {
  const { token, user, loading: authLoading } = useAuth();
  const router = useRouter();
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [pulls, setPulls] = useState<PullRow[]>([]);
  const [authors, setAuthors] = useState<Record<string, User>>({});
  const [status, setStatus] = useState<"open" | "merged" | "all">("open");
  const [query, setQuery] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [selectedRepository, setSelectedRepository] = useState("");
  const [branches, setBranches] = useState<Branch[]>([]);
  const [targetBranches, setTargetBranches] = useState<Branch[]>([]);
  const [targetRepository, setTargetRepository] = useState<Repository | null>(null);
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const generation = useRef(0);

  const load = useCallback(async () => {
    if (authLoading) return;
    const current = ++generation.current;
    if (!token) {
      setRepositories([]);
      setPulls([]);
      setAuthors({});
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const repos = await allPages<Repository>(
        "/repositories",
        "repositories",
        token,
      );
      if (generation.current !== current) return;
      setRepositories(repos);
      setSelectedRepository((value) => value || repos[0]?.id || "");
      const upstreams = await Promise.all(
        [...new Set(repos.map((repo) => repo.upstream_repository_id).filter((id): id is string => Boolean(id)))].map(
          (id) => api<Repository>(`/repositories/${id}`, {}, token).catch(() => null),
        ),
      );
      const reviewRepositories = [
        ...repos,
        ...upstreams
          .filter((repo): repo is Repository => repo !== null)
          .filter((repo) => !repos.some((item) => item.id === repo.id)),
      ];
      const groups = await Promise.all(
        reviewRepositories.map(async (repository) =>
          (
            await allPages<PullRequest>(
              `/repositories/${repository.id}/pulls`,
              "pull_requests",
              token,
            )
          ).map((pull) => ({ ...pull, repository })),
        ),
      );
      if (generation.current !== current) return;
      const found = groups
        .flat()
        .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
      setPulls(found);
      const people = await Promise.all(
        [...new Set(found.map((pull) => pull.author_id))].map((id) =>
          api<User>(`/users/${id}`, {}, token).catch(() => null),
        ),
      );
      if (generation.current === current)
        setAuthors(
          Object.fromEntries(
            people
              .filter((person): person is User => Boolean(person))
              .map((person) => [person.id, person]),
          ),
        );
    } catch (reason) {
      if (generation.current === current)
        setError(errorMessage(reason, "Pull requests could not be loaded."));
    } finally {
      if (generation.current === current) setLoading(false);
    }
  }, [authLoading, token]);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  useEffect(() => {
    if (!showCreate || !selectedRepository) return;
    let active = true;
    const source = repositories.find((item) => item.id === selectedRepository);
    const targetID = source?.upstream_repository_id ?? selectedRepository;
    void Promise.all([
      api<{ branches: Branch[] }>(
        `/repositories/${selectedRepository}/branches`,
        {},
        token,
      ),
      api<Repository>(`/repositories/${targetID}`, {}, token),
      api<{ branches: Branch[] }>(`/repositories/${targetID}/branches`, {}, token),
      allPages<Proposal>(
        `/repositories/${targetID}/proposals`,
        "proposals",
        token,
      ),
    ])
      .then(([branchPage, targetRepo, targetBranchPage, proposalList]) => {
        if (active) {
          setBranches(branchPage.branches);
          setTargetRepository(targetRepo);
          setTargetBranches(targetBranchPage.branches);
          setProposals(proposalList);
        }
      })
      .catch((reason) => {
        if (active)
          setError(errorMessage(reason, "Branch context could not be loaded."));
      });
    return () => {
      active = false;
    };
  }, [repositories, selectedRepository, showCreate, token]);

  const filtered = useMemo(() => {
    const term = query.trim().toLowerCase();
    return pulls.filter(
      (pull) =>
        (status === "all" || pull.status === status) &&
        (!term ||
          pull.title.toLowerCase().includes(term) ||
          pull.body.toLowerCase().includes(term) ||
          pull.repository.name.toLowerCase().includes(term) ||
          pull.source_branch.toLowerCase().includes(term)),
    );
  }, [pulls, query, status]);

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError("");
    const data = new FormData(event.currentTarget);
    const sourceRepositoryID = String(data.get("repository_id"));
    const repositoryID = targetRepository?.id ?? sourceRepositoryID;
    try {
      const proposalID = String(data.get("proposal_id") ?? "");
      const pull = await api<PullRequest>(
        `/repositories/${repositoryID}/pulls`,
        {
          method: "POST",
          body: JSON.stringify({
            title: data.get("title"),
            body: data.get("body"),
            source_repository_id: sourceRepositoryID,
            source_branch: data.get("source_branch"),
            target_branch: data.get("target_branch"),
            ...(proposalID ? { proposal_id: proposalID } : {}),
          }),
        },
        token,
      );
      router.push(`/pulls/${repositoryID}/${pull.id}`);
    } catch (reason) {
      setError(errorMessage(reason, "Pull request could not be opened."));
    } finally {
      setPending(false);
    }
  }

  if (authLoading || loading)
    return (
      <Card className="p-8 text-sm text-[var(--muted)]">
        Gathering candidate changes…
      </Card>
    );
  if (!user)
    return (
      <Card className="p-8 text-center">
        <span className="mx-auto grid size-12 place-items-center rounded-xl bg-[var(--brand-soft)] text-[var(--brand)]">
          <Icons.GitPull />
        </span>
        <h1 className="mt-4 text-2xl font-semibold">Review work in context</h1>
        <p className="mx-auto mt-2 max-w-lg text-sm leading-6 text-[var(--muted)]">
          Sign in to find pull requests across the repositories you collaborate
          on.
        </p>
        <Link
          href="/?access=signin"
          className="mt-5 inline-flex min-h-10 items-center rounded-lg bg-[var(--brand)] px-4 text-sm font-semibold text-white"
        >
          Sign in to view pull requests
        </Link>
      </Card>
    );
  return (
    <div className="space-y-7">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="font-mono text-xs font-semibold uppercase tracking-[.16em] text-[var(--brand)]">
            Review contributions
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-[-.035em]">
            Pull requests
          </h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--muted)]">
            Understand the purpose, branch state, and exact changes before
            moving shared code forward.
          </p>
        </div>
        <Button
          disabled={!repositories.length}
          onClick={() => setShowCreate((value) => !value)}
        >
          <Icons.Plus size={16} />
          {showCreate ? "Cancel" : "New pull request"}
        </Button>
      </header>
      {showCreate && (
        <Card className="p-5 sm:p-6">
          <h2 className="text-lg font-semibold">Open reviewable work</h2>
          <p className="mt-1 text-sm text-[var(--muted)]">
            Connect a candidate branch to its target and explain the feedback
            you need.
          </p>
          <form onSubmit={create} className="mt-5 grid gap-4">
            <label className="text-sm font-semibold">
              Source repository
              <select
                name="repository_id"
                value={selectedRepository}
                onChange={(event) => {
                  setBranches([]);
                  setTargetBranches([]);
                  setTargetRepository(null);
                  setProposals([]);
                  setSelectedRepository(event.target.value);
                }}
                required
                className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal"
              >
                {repositories.map((repository) => (
                  <option key={repository.id} value={repository.id}>
                    {repository.name}
                  </option>
                ))}
              </select>
            </label>
            {targetRepository && targetRepository.id !== selectedRepository && (
              <p className="rounded-lg bg-[var(--brand-soft)] p-3 text-sm text-[var(--brand-strong)]">
                Contributing from your fork to <strong>{targetRepository.name}</strong>.
              </p>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="text-sm font-semibold">
                Candidate branch
                <select
                  name="source_branch"
                  required
                  className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-mono font-normal"
                >
                  <option value="">Choose a branch</option>
                  {branches.map((branch) => (
                    <option key={branch.name}>{branch.name}</option>
                  ))}
                </select>
              </label>
              <label className="text-sm font-semibold">
                Target branch
                <select
                  key={targetRepository?.id}
                  name="target_branch"
                  defaultValue={targetRepository?.default_branch}
                  required
                  className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-mono font-normal"
                >
                  {targetBranches.map((branch) => (
                    <option key={branch.name}>{branch.name}</option>
                  ))}
                </select>
              </label>
            </div>
            <label className="text-sm font-semibold">
              Linked proposal{" "}
              <span className="font-normal text-[var(--muted)]">
                (optional)
              </span>
              <select
                name="proposal_id"
                className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal"
              >
                <option value="">No linked proposal</option>
                {proposals.map((proposal) => (
                  <option key={proposal.id} value={proposal.id}>
                    {proposal.title}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-sm font-semibold">
              Title
              <input
                name="title"
                required
                maxLength={200}
                placeholder="Summarize the candidate change"
                className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal outline-none focus:border-[var(--brand)]"
              />
            </label>
            <label className="text-sm font-semibold">
              Purpose and feedback needed
              <textarea
                name="body"
                required
                maxLength={10000}
                rows={6}
                placeholder="What changed, why, and where should reviewers focus?"
                className="mt-2 w-full rounded-lg border border-[var(--line-strong)] bg-white p-3 font-normal leading-6 outline-none focus:border-[var(--brand)]"
              />
            </label>
            <div>
              <Button type="submit" disabled={pending || !branches.length || !targetBranches.length}>
                {pending ? "Opening…" : "Open pull request"}
              </Button>
              {(!branches.length || !targetBranches.length) && (
                <p className="mt-2 text-xs text-[var(--muted)]">
                  Both the candidate and target need a published branch before
                  opening a pull request.
                </p>
              )}
            </div>
          </form>
        </Card>
      )}
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      <section aria-labelledby="pull-list-heading">
        <div className="grid gap-3 rounded-xl border border-[var(--line)] bg-[var(--surface)] p-3 md:grid-cols-[1fr_auto]">
          <label className="relative">
            <span className="sr-only">Search pull requests</span>
            <Icons.Search
              size={16}
              className="pointer-events-none absolute left-3 top-3 text-[var(--muted)]"
            />
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search title, purpose, branch, repository…"
              className="min-h-10 w-full rounded-lg border border-[var(--line-strong)] bg-white pl-9 pr-3 text-sm outline-none focus:border-[var(--brand)]"
            />
          </label>
          <div
            className="flex rounded-lg bg-black/[.045] p-1"
            aria-label="Filter pull requests by status"
          >
            {(["open", "merged", "all"] as const).map((value) => (
              <button
                key={value}
                type="button"
                aria-pressed={status === value}
                onClick={() => setStatus(value)}
                className={`min-h-8 rounded-md px-3 text-sm font-semibold capitalize ${status === value ? "bg-white shadow-sm" : "text-[var(--muted)]"}`}
              >
                {value}
              </button>
            ))}
          </div>
        </div>
        <div className="mt-4 flex justify-between">
          <h2 id="pull-list-heading" className="text-lg font-semibold">
            Candidate work
          </h2>
          <p className="text-xs text-[var(--muted)]">{filtered.length} shown</p>
        </div>
        <Card className="mt-3 overflow-hidden">
          {!filtered.length ? (
            <div className="p-9 text-center">
              <h3 className="font-semibold">No pull requests match</h3>
              <p className="mt-2 text-sm text-[var(--muted)]">
                {pulls.length
                  ? "Try another search or status filter."
                  : "Open the first pull request when a candidate branch is ready for feedback."}
              </p>
            </div>
          ) : (
            <div className="divide-y divide-[var(--line)]">
              {filtered.map((pull) => {
                const author = authors[pull.author_id];
                return (
                  <Link
                    key={pull.id}
                    href={`/pulls/${pull.repository_id}/${pull.id}`}
                    className="group flex gap-4 p-5 transition hover:bg-[var(--brand-soft)] sm:p-6"
                  >
                    <Avatar
                      initials={initials(author)}
                      label={author?.display_name ?? "Unknown author"}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="font-semibold group-hover:text-[var(--brand-strong)] group-hover:underline">
                          {pull.title}
                        </h3>
                        <Badge
                          tone={pull.status === "open" ? "success" : "info"}
                        >
                          {pull.status}
                        </Badge>
                      </div>
                      <p className="mt-1 line-clamp-2 text-sm leading-6 text-[var(--muted)]">
                        {pull.body}
                      </p>
                      <p className="mt-2 text-xs text-[var(--muted)]">
                        <span className="font-mono font-semibold text-[var(--ink)]">
                          {pull.repository.name}
                        </span>{" "}
                        · <code>{pull.source_branch}</code> →{" "}
                        <code>{pull.target_branch}</code> ·{" "}
                        {author ? `@${author.handle}` : "unknown author"}
                      </p>
                    </div>
                    <Icons.Chevron
                      className="mt-2 shrink-0 text-[var(--muted)]"
                      size={16}
                    />
                  </Link>
                );
              })}
            </div>
          )}
        </Card>
      </section>
    </div>
  );
}

export function PullRequestDetail({
  repositoryID,
  pullRequestID,
}: {
  repositoryID: string;
  pullRequestID: string;
}) {
  const { token, user, loading: authLoading } = useAuth();
  const [repository, setRepository] = useState<Repository | null>(null);
  const [sourceRepository, setSourceRepository] = useState<Repository | null>(null);
  const [pull, setPull] = useState<PullRequest | null>(null);
  const [commits, setCommits] = useState<PullRequestCommit[]>([]);
  const [files, setFiles] = useState<FileChange[]>([]);
  const [comments, setComments] = useState<PullRequestComment[]>([]);
  const [reviews, setReviews] = useState<PullRequestReview[]>([]);
  const [readiness, setReadiness] = useState<MergeReadiness | null>(null);
  const [proposal, setProposal] = useState<Proposal | null>(null);
  const [authors, setAuthors] = useState<Record<string, User>>({});
  const [participant, setParticipant] = useState(false);
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [refreshRequired, setRefreshRequired] = useState(false);
  const [branchCredential, setBranchCredential] = useState<Credential | null>(null);
  const generation = useRef(0);

  const load = useCallback(async () => {
    if (authLoading) return false;
    const current = ++generation.current;
    const active = () => generation.current === current;
    setLoading(true);
    setError("");
    try {
      const base = `/repositories/${repositoryID}/pulls/${pullRequestID}`;
      const [repo, item, commitPage, filePage, discussion, reviewList] =
        await Promise.all([
          api<Repository>(`/repositories/${repositoryID}`, {}, token),
          api<PullRequest>(base, {}, token),
          api<{ commits: PullRequestCommit[] }>(`${base}/commits`, {}, token),
          api<{ files: FileChange[] }>(`${base}/files`, {}, token),
          allPages<PullRequestComment>(`${base}/comments`, "comments", token),
          allPages<PullRequestReview>(`${base}/reviews`, "reviews", token),
        ]);
      if (!active()) return false;
      setRepository(repo);
      setPull(item);
      setCommits(commitPage.commits);
      setFiles(filePage.files);
      setComments(discussion);
      setReviews(reviewList);
      setSourceRepository(
        item.source_repository_id === repositoryID
          ? repo
          : await api<Repository>(`/repositories/${item.source_repository_id}`, {}, token).catch(() => null),
      );
      const [linked, available] = await Promise.all([
        item.proposal_id
          ? api<Proposal>(
              `/repositories/${repositoryID}/proposals/${item.proposal_id}`,
              {},
              token,
            ).catch(() => null)
          : null,
        token
          ? allPages<Repository>("/repositories", "repositories", token)
          : [],
      ]);
      if (!active()) return false;
      const canParticipate = available.some(
        (candidate) => candidate.id === repositoryID,
      );
      setProposal(linked);
      setParticipant(canParticipate);
      const report =
        item.status === "open"
          ? await api<MergeReadiness>(`${base}/merge-readiness`, {}, token)
          : null;
      if (!active()) return false;
      setReadiness(report);
      const ids = [
        ...new Set([
          item.author_id,
          ...discussion.map((comment) => comment.author_id),
          ...reviewList.map((review) => review.reviewer_id),
          ...(item.merged_by ? [item.merged_by] : []),
        ]),
      ];
      const people = await Promise.all(
        ids.map((id) => api<User>(`/users/${id}`, {}, token).catch(() => null)),
      );
      if (!active()) return false;
      setAuthors(
        Object.fromEntries(
          people
            .filter((person): person is User => Boolean(person))
            .map((person) => [person.id, person]),
        ),
      );
      setRefreshRequired(false);
      return true;
    } catch (reason) {
      if (active())
        setError(errorMessage(reason, "Pull request could not be loaded."));
      return false;
    } finally {
      if (active()) setLoading(false);
    }
  }, [authLoading, pullRequestID, repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  async function comment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError("");
    const form = event.currentTarget;
    try {
      const body = new FormData(form).get("body");
      const created = await api<PullRequestComment>(
        `/repositories/${repositoryID}/pulls/${pullRequestID}/comments`,
        { method: "POST", body: JSON.stringify({ body }) },
        token,
      );
      setComments((items) => [...items, created]);
      if (user) setAuthors((items) => ({ ...items, [user.id]: user }));
      form.reset();
    } catch (reason) {
      setError(errorMessage(reason, "Comment could not be published."));
    } finally {
      setPending(false);
    }
  }

  async function mutate(
    path: string,
    init: RequestInit,
    failure: string,
    success: string,
  ) {
    setPending(true);
    setError("");
    setNotice("");
    try {
      await api(path, init, token);
    } catch (reason) {
      setPending(false);
      setError(errorMessage(reason, failure));
      return;
    }
    setNotice(success);
    const refreshed = await load();
    if (!refreshed) {
      setError("");
      setRefreshRequired(true);
      setNotice(
        `${success} The latest page state could not be loaded; reload before taking another action.`,
      );
    }
    setPending(false);
  }

  const submitReview = (decision: "approved" | "changes_requested") =>
    mutate(
      `/repositories/${repositoryID}/pulls/${pullRequestID}/reviews`,
      { method: "POST", body: JSON.stringify({ decision }) },
      "Review decision could not be recorded.",
      "Review decision recorded.",
    );
  const withdrawReview = (reviewID: string) =>
    mutate(
      `/repositories/${repositoryID}/pulls/${pullRequestID}/reviews/${reviewID}`,
      { method: "DELETE" },
      "Review decision could not be withdrawn.",
      "Review decision withdrawn.",
    );
  const synchronize = () =>
    mutate(
      `/repositories/${repositoryID}/pulls/${pullRequestID}/synchronize`,
      { method: "POST" },
      "The latest candidate branch revision could not be adopted.",
      "Latest candidate revision adopted.",
    );
  const merge = () =>
    mutate(
      `/repositories/${repositoryID}/pulls/${pullRequestID}/merge`,
      { method: "POST" },
      "The pull request could not be merged. Review the current blockers and try again.",
      "Pull request merged.",
    );
  const enqueue = () =>
    mutate(
      `/repositories/${repositoryID}/pulls/${pullRequestID}/queue`,
      { method: "POST" },
      "The pull request could not enter the integration queue.",
      "Pull request entered the integration queue.",
    );
  const close = () =>
    mutate(
      `/repositories/${repositoryID}/pulls/${pullRequestID}/close`,
      { method: "POST" },
      "The pull request could not be closed.",
      "Pull request closed.",
    );
  const setMaintainerEdits = (allowed: boolean) =>
    mutate(
      `/repositories/${repositoryID}/pulls/${pullRequestID}`,
      { method: "PATCH", body: JSON.stringify({ maintainer_edits_allowed: allowed }) },
      "The contribution branch policy could not be updated.",
      allowed ? "Maintainer branch updates enabled." : "Maintainer branch updates disabled.",
    );
  const issueMaintainerCredential = async () => {
    setPending(true);
    setError("");
    try {
      setBranchCredential(await api<Credential>(`/repositories/${repositoryID}/pulls/${pullRequestID}/maintainer-credential`, { method: "POST" }, token));
    } catch (reason) {
      setError(errorMessage(reason, "A branch credential could not be issued."));
    } finally { setPending(false); }
  };

  if (loading)
    return (
      <Card className="p-8 text-sm text-[var(--muted)]">
        Opening the review context…
      </Card>
    );
  if (!repository || !pull)
    return (
      <Card className="p-8">
        <h1 className="text-xl font-semibold">Pull request unavailable</h1>
        <p role="alert" className="mt-2 text-sm text-[var(--danger)]">
          {error}
        </p>
        <Link
          href="/pulls"
          className="mt-5 inline-flex text-sm font-semibold text-[var(--brand)]"
        >
          Back to pull requests
        </Link>
      </Card>
    );
  const author = authors[pull.author_id];
  const ownReview = reviews.find((review) => review.reviewer_id === user?.id);
  const isAuthor = user?.id === pull.author_id;
  const isOwner = user?.id === repository.owner_id;
  const additions = files.filter((file) => file.status === "added").length;
  const deletions = files.filter((file) => file.status === "deleted").length;
  const modifications = files.length - additions - deletions;
  return (
    <div className="space-y-6">
      <header>
        <Link
          href="/pulls"
          className="text-sm text-[var(--muted)] hover:text-[var(--brand)]"
        >
          Pull requests
        </Link>
        <p className="mt-3 font-mono text-xs font-semibold text-[var(--brand)]">
          {repository.name}
        </p>
        <div className="mt-2 flex flex-wrap items-start gap-3">
          <h1 className="max-w-4xl text-3xl font-semibold tracking-[-.035em]">
            {pull.title}
          </h1>
          <Badge tone={pull.status === "open" ? "success" : "info"}>
            {pull.status}
          </Badge>
        </div>
        <p className="mt-3 text-sm text-[var(--muted)]">
          Opened by {author ? `@${author.handle}` : "an unknown author"} on{" "}
          {formatDate(pull.created_at)} ·{" "}
          <code>{sourceRepository?.name ?? short(pull.source_repository_id)}:{pull.source_branch}</code>{" "}
          into <code>{repository.name}:{pull.target_branch}</code>
        </p>
      </header>
      {notice && (
        <p
          role="status"
          className="rounded-lg bg-[var(--brand-soft)] p-3 text-sm text-[var(--brand-strong)]"
        >
          {notice}
        </p>
      )}
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      <nav
        aria-label="Pull request sections"
        className="flex gap-1 overflow-x-auto border-b border-[var(--line)]"
      >
        <a
          href="#conversation"
          className="border-b-2 border-[var(--brand)] px-3 py-3 text-sm font-semibold"
        >
          Conversation{" "}
          <span className="text-[var(--muted)]">{comments.length}</span>
        </a>
        <a
          href="#reviews"
          className="px-3 py-3 text-sm font-semibold text-[var(--muted)] hover:text-[var(--ink)]"
        >
          Reviews {reviews.length}
        </a>
        <a
          href="#commits"
          className="px-3 py-3 text-sm font-semibold text-[var(--muted)] hover:text-[var(--ink)]"
        >
          Commits {commits.length}
        </a>
        <a
          href="#checks"
          className="px-3 py-3 text-sm font-semibold text-[var(--muted)] hover:text-[var(--ink)]"
        >
          Checks
        </a>
        <a
          href="#files"
          className="px-3 py-3 text-sm font-semibold text-[var(--muted)] hover:text-[var(--ink)]"
        >
          Files changed {files.length}
        </a>
      </nav>
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_19rem]">
        <main className="min-w-0 space-y-6">
          <section
            id="conversation"
            className="scroll-mt-24 space-y-5"
            aria-label="Pull request conversation"
          >
            <Card className="overflow-hidden">
              <div className="flex items-center gap-2 border-b border-[var(--line)] bg-[var(--surface)] px-5 py-3">
                <Avatar
                  initials={initials(author)}
                  label={author?.display_name ?? "Unknown author"}
                  size="sm"
                />
                <span className="text-sm font-semibold">
                  {author ? `@${author.handle}` : "Author"}
                </span>
                <span className="text-xs text-[var(--muted)]">
                  described the change
                </span>
              </div>
              <div className="whitespace-pre-wrap p-5 text-sm leading-7 sm:p-6">
                {pull.body}
              </div>
            </Card>
            {comments.map((item) => {
              const person = authors[item.author_id];
              return (
                <Card key={item.id} className="overflow-hidden">
                  <div className="flex items-center gap-2 border-b border-[var(--line)] bg-[var(--surface)] px-5 py-3">
                    <Avatar
                      initials={initials(person)}
                      label={person?.display_name ?? "Unknown commenter"}
                      size="sm"
                    />
                    <span className="text-sm font-semibold">
                      {person ? `@${person.handle}` : "Unknown user"}
                    </span>
                    <span className="text-xs text-[var(--muted)]">
                      commented {formatDate(item.created_at)}
                    </span>
                  </div>
                  <p className="whitespace-pre-wrap p-5 text-sm leading-7">
                    {item.body}
                  </p>
                </Card>
              );
            })}
            {participant || (isAuthor && pull.source_repository_id !== pull.repository_id && repository.visibility === "public") ? (
              <Card className="p-5">
                <form onSubmit={comment}>
                  <label className="text-sm font-semibold">
                    Add review feedback
                    <textarea
                      name="body"
                      required
                      maxLength={10000}
                    rows={5}
                    disabled={refreshRequired}
                      placeholder="Ask a question or leave focused feedback…"
                      className="mt-2 w-full rounded-lg border border-[var(--line-strong)] bg-white p-3 font-normal leading-6 outline-none focus:border-[var(--brand)]"
                    />
                  </label>
                  <Button
                    type="submit"
                    disabled={pending || refreshRequired}
                    className="mt-3"
                  >
                    {pending ? "Publishing…" : "Comment"}
                  </Button>
                </form>
              </Card>
            ) : (
              <Card className="p-5 text-sm text-[var(--muted)]">
                {user ? (
                  "Only current repository participants and the outside contribution author can join this discussion."
                ) : (
                  <>
                    <Link
                      href="/?access=signin"
                      className="font-semibold text-[var(--brand)]"
                    >
                      Sign in
                    </Link>{" "}
                    to participate if you collaborate on this repository.
                  </>
                )}
              </Card>
            )}
          </section>
          <section id="reviews" className="scroll-mt-24 space-y-3">
            <div className="flex items-baseline justify-between">
              <h2 className="text-lg font-semibold">Review decisions</h2>
              <span className="text-xs text-[var(--muted)]">
                {readiness
                  ? `${readiness.approvals} of ${readiness.required_approvals} approvals`
                  : `${reviews.length} recorded`}
              </span>
            </div>
            <Card className="overflow-hidden">
              {reviews.length ? (
                <div className="divide-y divide-[var(--line)]">
                  {reviews.map((review) => {
                    const reviewer = authors[review.reviewer_id];
                    const active =
                      review.decision !== "withdrawn" && !review.stale;
                    return (
                      <div
                        key={review.id}
                        className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between sm:px-5"
                      >
                        <div className="flex min-w-0 items-center gap-3">
                          <Avatar
                            initials={initials(reviewer)}
                            label={reviewer?.display_name ?? "Unknown reviewer"}
                            size="sm"
                          />
                          <div className="min-w-0">
                            <p className="text-sm">
                              <span className="font-semibold">
                                {reviewer
                                  ? `@${reviewer.handle}`
                                  : "Unknown reviewer"}
                              </span>{" "}
                              {review.decision === "approved"
                                ? "approved this revision"
                                : review.decision === "changes_requested"
                                  ? "requested changes"
                                  : "withdrew their decision"}
                            </p>
                            <p className="mt-1 text-xs text-[var(--muted)]">
                              <code title={review.reviewed_commit_id}>
                                {short(review.reviewed_commit_id)}
                              </code>{" "}
                              · {formatDate(review.updated_at)}
                              {review.stale
                                ? " · stale after branch movement"
                                : ""}
                            </p>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <Badge
                            tone={
                              !active
                                ? "neutral"
                                : review.decision === "approved"
                                  ? "success"
                                  : "danger"
                            }
                          >
                            {review.stale
                              ? "stale"
                              : review.decision.replace("_", " ")}
                          </Badge>
                          {review.reviewer_id === user?.id &&
                            review.decision !== "withdrawn" &&
                            pull.status === "open" && (
                              <Button
                                variant="secondary"
                                disabled={pending || refreshRequired}
                                onClick={() => void withdrawReview(review.id)}
                              >
                                Withdraw
                              </Button>
                            )}
                        </div>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <p className="p-6 text-sm text-[var(--muted)]">
                  No one has submitted a review decision yet.
                </p>
              )}
            </Card>
            {participant && pull.status === "open" && (
              <Card className="p-5">
                <h3 className="font-semibold">Leave your decision</h3>
                <p className="mt-1 text-sm leading-6 text-[var(--muted)]">
                  Your decision evaluates revision{" "}
                  <code>{short(pull.source_commit_id)}</code>. Submitting again
                  replaces your current decision.
                </p>
                <div className="mt-4 flex flex-wrap gap-2">
                  <Button
                    disabled={
                      pending ||
                      refreshRequired ||
                      readiness?.source.state !== "current"
                    }
                    onClick={() => void submitReview("approved")}
                  >
                Approve
                  </Button>
                  <Button
                    variant="secondary"
                    disabled={
                      pending ||
                      refreshRequired ||
                      readiness?.source.state !== "current"
                    }
                    onClick={() => void submitReview("changes_requested")}
                  >
                    Request changes
                  </Button>
                </div>
                {readiness?.source.state !== "current" && (
                  <p className="mt-3 text-xs text-[var(--danger)]">
                    The candidate branch changed. Its author must synchronize
                    the pull request before anyone can review the new revision.
                  </p>
                )}
                {ownReview && (
                  <p className="mt-3 text-xs text-[var(--muted)]">
                    Your current decision: {ownReview.stale ? "stale " : ""}
                    {ownReview.decision.replace("_", " ")}.
                  </p>
                )}
              </Card>
            )}
          </section>
          <PullRequestChecks
            repositoryID={repositoryID}
            pullRequestID={pullRequestID}
            participant={participant}
            sourceCommitID={pull.source_commit_id}
          />
          <section id="commits" className="scroll-mt-24">
            <div className="mb-3 flex items-baseline justify-between">
              <h2 className="text-lg font-semibold">Commits</h2>
              <span className="text-xs text-[var(--muted)]">
                source-only history
              </span>
            </div>
            <Card className="overflow-hidden">
              {commits.length ? (
                <ol className="divide-y divide-[var(--line)]">
                  {commits.map((commit) => (
                    <li
                      key={commit.id}
                      className="flex items-start justify-between gap-4 p-4 sm:px-5"
                    >
                      <div className="min-w-0">
                        <p className="truncate text-sm font-semibold">
                          {subject(commit.message)}
                        </p>
                        <p className="mt-1 text-xs text-[var(--muted)]">
                          {commit.headers.find(
                            (header) => header.name === "author",
                          )?.value ?? "Unknown author"}
                        </p>
                      </div>
                      <code
                        title={commit.id}
                        className="shrink-0 rounded-md bg-[var(--canvas)] px-2 py-1 text-xs"
                      >
                        {short(commit.id)}
                      </code>
                    </li>
                  ))}
                </ol>
              ) : (
                <p className="p-6 text-sm text-[var(--muted)]">
                  The recorded source contains no commits outside the target
                  snapshot.
                </p>
              )}
            </Card>
          </section>
          <section id="files" className="scroll-mt-24">
            <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
              <h2 className="text-lg font-semibold">Files changed</h2>
              <p className="text-xs text-[var(--muted)]">
                <span className="text-[var(--brand)]">{additions} added</span> ·{" "}
                {modifications} modified ·{" "}
                <span className="text-[var(--danger)]">
                  {deletions} deleted
                </span>
              </p>
            </div>
            <Card className="overflow-hidden">
              {files.length ? (
                <div className="divide-y divide-[var(--line)]">
                  {files.map((file) => (
                    <div
                      key={file.path}
                      className="flex min-w-0 items-center gap-3 p-4 sm:px-5"
                    >
                      <Badge
                        tone={
                          file.status === "added"
                            ? "success"
                            : file.status === "deleted"
                              ? "danger"
                              : "warning"
                        }
                      >
                        {file.status}
                      </Badge>
                      <code
                        className="min-w-0 flex-1 truncate text-sm"
                        title={file.path}
                      >
                        {file.path}
                      </code>
                      {file.old_mode !== file.new_mode && (
                        <span className="hidden text-xs text-[var(--muted)] sm:block">
                          {file.old_mode ?? "—"} → {file.new_mode ?? "—"}
                        </span>
                      )}
                    </div>
                  ))}
                </div>
              ) : (
                <p className="p-6 text-sm text-[var(--muted)]">
                  No file changes differ from the target snapshot.
                </p>
              )}
            </Card>
          </section>
        </main>
        <aside className="space-y-4">
          <ChangeSessionsCard
            repositoryID={repositoryID}
            pullRequestID={pullRequestID}
            participant={participant}
            open={pull.status === "open"}
          />
          {pull.status === "merged" ? (
            <Card className="border-[var(--brand)] p-5">
              <Badge tone="success">Merged</Badge>
              <h2 className="mt-3 font-semibold">Change landed</h2>
              <p className="mt-2 text-sm leading-6 text-[var(--muted)]">
                {pull.merge_commit_id ? (
                  <>
                    Merge commit{" "}
                    <code title={pull.merge_commit_id}>
                      {short(pull.merge_commit_id)}
                    </code>
                  </>
                ) : (
                  "The target branch includes this change."
                )}
                {pull.merged_by && (
                  <>
                    {" "}
                    by{" "}
                    {authors[pull.merged_by]
                      ? `@${authors[pull.merged_by].handle}`
                      : "the maintainer"}
                  </>
                )}
              </p>
            </Card>
          ) : (
            readiness && (
              <Card className="p-5">
                <div className="flex items-center justify-between gap-2">
                  <h2 className="font-semibold">Merge readiness</h2>
                  <Badge tone={readiness.mergeable ? "success" : "warning"}>
                    {readiness.mergeable ? "Ready" : "Blocked"}
                  </Badge>
                </div>
                <p className="mt-2 text-sm leading-6 text-[var(--muted)]">
                  {readiness.mergeable
                    ? isOwner
                      ? "All checks pass. You can merge this reviewed revision."
                      : "All checks pass. A repository owner can merge this revision."
                    : `${readiness.blockers.length} ${readiness.blockers.length === 1 ? "blocker remains" : "blockers remain"}.`}
                </p>
                {readiness.blockers.length > 0 && (
                  <ul className="mt-4 space-y-3">
                    {readiness.blockers.map((blocker) => (
                      <li
                        key={blocker.code}
                        className="flex gap-2 text-sm leading-5"
                      >
                        <span
                          aria-hidden="true"
                          className="mt-1 size-2 shrink-0 rounded-full bg-[var(--warning)]"
                        />
                        <span>{blocker.message}</span>
                      </li>
                    ))}
                  </ul>
                )}
                <div className="mt-4 border-t border-[var(--line)] pt-4 text-xs text-[var(--muted)]">
                  <p>Evaluated revision <code title={readiness.evaluated_commit_id}>{short(readiness.evaluated_commit_id)}</code></p>
                  <p>
                    {readiness.approvals} / {readiness.required_approvals}{" "}
                    required approvals
                  </p>
                  <p className="mt-1">
                    Conflicts:{" "}
                    {readiness.has_conflicts ? "detected" : "none detected"}
                  </p>
                </div>
                {readiness.required_checks.length > 0 && <div className="mt-4 space-y-2"><p className="text-xs font-semibold text-[var(--muted)]">Required checks</p>{readiness.required_checks.map((check) => <div key={check.name} className="flex items-center justify-between gap-3 text-xs"><span className="font-medium">{check.name}</span><Badge tone={check.status === "passed" ? "success" : check.status === "pending" ? "warning" : "neutral"}>{check.status}</Badge></div>)}</div>}
                {readiness.integration_queue?.enabled && <div className="mt-4 rounded-lg bg-[var(--canvas)] p-3 text-xs leading-5"><p className="font-semibold">Ordered integration required</p><p className="text-[var(--muted)]">Up to {readiness.integration_queue.concurrency} candidate{readiness.integration_queue.concurrency === 1 ? "" : "s"}; failures {readiness.integration_queue.failure_behavior === "pause" ? "pause the queue" : "remove the failed entry"}. Admission uses the approval and checks shown above.</p></div>}
                {isAuthor && readiness.source.state !== "current" && (
                  <Button
                    className="mt-4 w-full"
                    variant="secondary"
                    disabled={pending || refreshRequired}
                    onClick={() => void synchronize()}
                  >
                    {pending ? "Updating…" : "Use latest source revision"}
                  </Button>
                )}
                {isAuthor && readiness.source.state !== "current" && (
                  <p className="mt-2 text-xs leading-5 text-[var(--muted)]">
                    Adopt the live branch tip for review. Existing decisions
                    remain stale.
                  </p>
                )}
                {isOwner && (
                  <Button
                    className="mt-4 w-full"
                    disabled={pending || refreshRequired || !(readiness.integration_queue?.enabled ? readiness.can_enqueue : readiness.can_merge) || Boolean(pull.queued_at)}
                    onClick={() => void (readiness.integration_queue?.enabled ? enqueue() : merge())}
                  >
                    {pending ? "Updating…" : pull.queued_at ? "Queued for integration" : readiness.integration_queue?.enabled ? `Queue for ${pull.target_branch}` : `Merge into ${pull.target_branch}`}
                  </Button>
                )}
              </Card>
            )
          )}
          <Card className="p-5">
            <h2 className="font-semibold">Branch state</h2>
            <div className="mt-4 flex items-center gap-2 rounded-lg bg-[var(--canvas)] p-3 text-xs">
              <code className="min-w-0 truncate" title={pull.source_branch}>
                {pull.source_branch}
              </code>
              <Icons.Arrow size={14} className="shrink-0 text-[var(--muted)]" />
              <code className="min-w-0 truncate" title={pull.target_branch}>
                {pull.target_branch}
              </code>
            </div>
            <dl className="mt-4 space-y-3 text-sm">
              <div>
                <dt className="text-xs text-[var(--muted)]">Review revision</dt>
                <dd className="mt-1">
                  <code title={pull.source_commit_id}>
                    {short(pull.source_commit_id)}
                  </code>
                  {readiness && (
                          <span className="ml-2">
                            <Badge
                              tone={
                                readiness.source.state === "current"
                                  ? "success"
                                  : "warning"
                              }
                            >
                              {readiness.source.state}
                            </Badge>
                          </span>
                  )}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-[var(--muted)]">Target snapshot</dt>
                <dd className="mt-1">
                  <code title={pull.target_commit_id}>
                    {short(pull.target_commit_id)}
                  </code>
                  {readiness && (
                          <span className="ml-2">
                            <Badge
                              tone={
                                readiness.target.state === "current"
                                  ? "neutral"
                                  : "info"
                              }
                            >
                              {readiness.target.state}
                            </Badge>
                          </span>
                  )}
                </dd>
              </div>
            </dl>
            {pull.status === "open" && pull.source_repository_id !== pull.repository_id && (
              <div className="mt-4 border-t border-[var(--line)] pt-4">
                <p className="text-xs leading-5 text-[var(--muted)]">The fork owner keeps source ownership. When enabled, current target participants can receive a one-hour credential restricted to this contribution branch.</p>
                {isAuthor ? <Button className="mt-3 w-full" variant="secondary" disabled={pending || refreshRequired} onClick={() => void setMaintainerEdits(!pull.maintainer_edits_allowed)}>{pull.maintainer_edits_allowed ? "Disable maintainer edits" : "Allow maintainer edits"}</Button> : participant && pull.maintainer_edits_allowed ? <Button className="mt-3 w-full" variant="secondary" disabled={pending || refreshRequired} onClick={() => void issueMaintainerCredential()}>Issue branch credential</Button> : null}
                {branchCredential?.token && <code className="mt-3 block break-all rounded bg-[var(--canvas)] p-3 text-xs select-all">{branchCredential.token}</code>}
              </div>
            )}
          </Card>
          {pull.status === "open" && (isAuthor || isOwner) && <Button className="w-full" variant="quiet" disabled={pending || refreshRequired} onClick={() => void close()}>Close pull request</Button>}
          <Card className="p-5">
            <h2 className="font-semibold">Related context</h2>
            {proposal && pull.proposal_id ? (
              <div className="mt-3">
                <Badge
                  tone={proposal.status === "open" ? "success" : "neutral"}
                >
                  proposal {proposal.status}
                </Badge>
                <Link
                  href={`/proposals/${repositoryID}/${pull.proposal_id}`}
                  className="mt-2 block text-sm font-semibold text-[var(--brand)] hover:underline"
                >
                  {proposal.title}
                </Link>
                <p className="mt-2 line-clamp-3 text-xs leading-5 text-[var(--muted)]">
                  {proposal.body}
                </p>
              </div>
            ) : (
              <p className="mt-2 text-sm leading-6 text-[var(--muted)]">
                No proposal is linked to this change.
              </p>
            )}
          </Card>
          <Card className="p-5">
            <h2 className="font-semibold">Change summary</h2>
            <dl className="mt-3 grid grid-cols-2 gap-3 text-sm">
              <div>
                <dt className="text-xs text-[var(--muted)]">Commits</dt>
                <dd className="mt-1 font-semibold">{commits.length}</dd>
              </div>
              <div>
                <dt className="text-xs text-[var(--muted)]">Files</dt>
                <dd className="mt-1 font-semibold">{files.length}</dd>
              </div>
            </dl>
          </Card>
        </aside>
      </div>
    </div>
  );
}
