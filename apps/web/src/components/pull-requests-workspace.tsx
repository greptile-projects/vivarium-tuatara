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
  type ConflictAnalysis,
  type FileChange,
  type IntegrationCandidate,
  type MergeReadiness,
  type Proposal,
  type PullRequest,
  type PullRequestComment,
  type PullRequestCommit,
  type PullRequestReview,
  type ReviewPlan,
  type Repository,
  type User,
} from "@/lib/api";
import { useAuth } from "./auth";
import { ChangeSessionsCard } from "./change-sessions";
import { PullRequestChecks } from "./pull-request-checks";
import { AccessibilityAssessmentsWorkspace } from "./accessibility-assessments-workspace";
import { PullPerformanceEvaluations } from "./pull-performance-evaluations";
import { PullAgentCandidates } from "./pull-agent-candidates";
import { PullPrivacyReview } from "./pull-privacy-review";
import { PullLocalizationReview } from "./pull-localization-review";
import { PullRequestPreviews } from "./pull-request-previews";
import { PullInterfaceChecks } from "./pull-interface-checks";
import { DocumentationPullReviewCard } from "./documentation-pull-review";
import { ExtensionContributions } from "./extension-contributions";
import { PullInfrastructurePlans } from "./pull-infrastructure-plans";
import { Icons } from "./icons";
import { Avatar, Badge, Button, Card } from "./ui";

type PullRow = PullRequest & { repository: Repository };
type FederationEvent = { id:string;kind:"comment"|"review"|"revision"|"checks"|"preview"|"closure"|"agent_session";actor:string;revision?:string;body?:string;decision?:string;state?:string;evidence?:Record<string,unknown>;created_at:string;origin_instance_id:string;verification:string;stale:boolean };
type StackContext={stack_id:string;stack_title:string;outcome:string;member_id:string;position:number;revision:string;parent_revision:string;target_revision:string;review_state:"reviewable_now"|"provisional"|"blocked";individual_diff:{commit_count:number;files:string[];additions:number;deletions:number};cumulative_diff:{commit_count:number;files:string[];additions:number;deletions:number};commit_ids:string[];upstream_revisions:{member_id:string;pull_request_id?:string;revision:string;current:boolean}[];evidence:{id:string;kind:string;actor_id?:string;revision:string;state:string;summary:string;created_at:string}[];downstream_evidence_invalidated_by_upstream_change:{member_id:string;pull_request_id?:string;revision:string;current:boolean}[];acceptance_criteria:string[];authority:string};

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
  const [reviewPlans, setReviewPlans] = useState<ReviewPlan[]>([]);
  const [reviewPlanError, setReviewPlanError] = useState("");
  const [federationEvents, setFederationEvents] = useState<FederationEvent[]>([]);
  const [stackContexts, setStackContexts] = useState<StackContext[]>([]);
  const [readiness, setReadiness] = useState<MergeReadiness | null>(null);
  const [conflicts, setConflicts] = useState<ConflictAnalysis | null>(null);
  const [conflictError, setConflictError] = useState("");
  const [candidates, setCandidates] = useState<IntegrationCandidate[]>([]);
  const [candidateError, setCandidateError] = useState("");
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
      const [repo, item, commitPage, filePage, discussion, reviewList, candidatePage] =
        await Promise.all([
          api<Repository>(`/repositories/${repositoryID}`, {}, token),
          api<PullRequest>(base, {}, token),
          api<{ commits: PullRequestCommit[] }>(`${base}/commits`, {}, token),
          api<{ files: FileChange[] }>(`${base}/files`, {}, token),
          allPages<PullRequestComment>(`${base}/comments`, "comments", token),
          allPages<PullRequestReview>(`${base}/reviews`, "reviews", token),
          api<{ candidates: IntegrationCandidate[] }>(`${base}/candidates`, {}, token)
            .then((page) => ({ page, error: "" }))
            .catch(() => ({ page: { candidates: [] }, error: "Integration candidate evidence is temporarily unavailable." })),
        ]);
      if (!active()) return false;
      setRepository(repo);
      setPull(item);
      setCommits(commitPage.commits);
      setFiles(filePage.files);
      setComments(discussion);
      setReviews(reviewList);
	  const planPage=await api<{review_plans:ReviewPlan[]}>(`${base}/review-plans`,{},token)
	    .then((page)=>({page,error:""}))
	    .catch((reason)=>({page:null,error:errorMessage(reason,"Review plan history is unavailable.")}));
	  if(active()) {
	    if(planPage.page) setReviewPlans(planPage.page.review_plans);
	    setReviewPlanError(planPage.error);
	  }
	  const stackPage=await api<{stack_contexts:StackContext[]}>(`${base}/stack-context`,{},token).catch(()=>({stack_contexts:[]}));
	  if(active()) setStackContexts(stackPage.stack_contexts);
	  if (item.federated_contribution_id) {
	    const shared = await api<{events:FederationEvent[]}>(`${base}/federation-events`,{},token).catch(()=>({events:[]}));
	    if (active()) setFederationEvents(shared.events);
	  } else setFederationEvents([]);
      setCandidates(candidatePage.page.candidates);
      setCandidateError(candidatePage.error);
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
      const conflictReport = await api<ConflictAnalysis>(`${base}/conflict-analysis`, {}, token)
        .then((analysis) => ({ analysis, error: "" }))
        .catch(() => ({ analysis: null, error: "Conflict evidence is temporarily unavailable." }));
      if (!active()) return false;
      setConflicts(conflictReport.analysis);
      setConflictError(conflictReport.error);
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

  async function inspectCandidate(candidateID?: string) {
    setConflictError("");
    try {
      const query = candidateID ? `?candidate_id=${encodeURIComponent(candidateID)}` : "";
      setConflicts(await api<ConflictAnalysis>(`/repositories/${repositoryID}/pulls/${pullRequestID}/conflict-analysis${query}`, {}, token));
      document.getElementById("conflict-evidence")?.scrollIntoView({ behavior: "smooth", block: "start" });
    } catch (reason) {
      setConflictError(errorMessage(reason, "Conflict evidence could not be loaded."));
    }
  }

  async function launchConflictWorkspace() {
    if (!token || !conflicts) return;
    setPending(true);
    setConflictError("");
    try {
      const workspace = await api<{ id: string }>(
        `/repositories/${repositoryID}/pulls/${pullRequestID}/conflict-workspaces`,
        { method: "POST", body: JSON.stringify({ launch_id: `${conflicts.source.commit_id}-${conflicts.target.commit_id}`, candidate_id: conflicts.candidate_id || "" }) },
        token,
      );
      window.location.assign(`/workspaces/${workspace.id}`);
    } catch (reason) {
      setConflictError(errorMessage(reason, "Reconciliation workspace could not be launched."));
      setPending(false);
    }
  }

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
  const acknowledgeStack = (event: FormEvent<HTMLFormElement>, stackContext: StackContext) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    void mutate(
      `/repositories/${repositoryID}/pulls/${pullRequestID}/stack-context/owner-acknowledgements`,
      {method:"POST",body:JSON.stringify({stack_id:stackContext.stack_id,member_id:stackContext.member_id,decision:data.get("decision"),note:data.get("note")})},
      "Owner acknowledgement could not be recorded.",
      "Owner acknowledgement recorded.",
    );
  };
  const issueMaintainerCredential = async () => {
    setPending(true);
    setError("");
    try {
      setBranchCredential(await api<Credential>(`/repositories/${repositoryID}/pulls/${pullRequestID}/maintainer-credential`, { method: "POST" }, token));
    } catch (reason) {
      setError(errorMessage(reason, "A branch credential could not be issued."));
    } finally { setPending(false); }
  };
  const createReviewPlan = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    void mutate(
      `/repositories/${repositoryID}/pulls/${pullRequestID}/review-plans`,
      { method: "POST", body: JSON.stringify({ risk_summary: data.get("risk_summary"), completion_rule: data.get("completion_rule") }) },
      "The review plan could not be derived.",
      "A revision-exact review plan was published.",
    );
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
  const activelyQueued = Boolean(readiness?.integration_queue?.enabled && pull.queued_at);
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
        {activelyQueued && (
          <Link
            href={`/repositories/${repositoryID}/queue/${encodeURIComponent(pull.target_branch)}`}
            className="mt-3 inline-flex text-sm font-semibold text-[var(--brand)] hover:underline"
          >
            View queue order, blockers, and controls
          </Link>
        )}
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
      {stackContexts.map(stackContext=><Card key={stackContext.stack_id+stackContext.member_id} className="p-5"><div className="flex flex-wrap items-center gap-2"><Badge tone="info">Stack layer {stackContext.position}</Badge><h2 className="font-semibold">{stackContext.stack_title}</h2><Badge tone={stackContext.review_state==="reviewable_now"?"success":stackContext.review_state==="provisional"?"warning":"danger"}>{stackContext.review_state.replaceAll("_"," ")}</Badge></div><p className="mt-2 text-sm">{stackContext.outcome}</p><p className="mt-2 text-xs text-[var(--muted)]">Review this focused layer now; cumulative scope shows the complete candidate through this revision. Provisional evidence is waiting on exact upstream approval.</p><div className="mt-4 grid gap-3 sm:grid-cols-2"><div className="rounded-lg border p-3"><h3 className="text-sm font-semibold">Focused diff from declared parent</h3><p className="mt-1 font-mono text-xs">{short(stackContext.parent_revision)} → {short(stackContext.revision)}</p><p className="mt-2 text-xs">{stackContext.individual_diff.commit_count} commits · {stackContext.individual_diff.files.length} files · +{stackContext.individual_diff.additions} −{stackContext.individual_diff.deletions}</p><details className="mt-2 text-xs"><summary className="cursor-pointer font-semibold">Exact commits and files</summary><div className="mt-2 space-y-1 font-mono">{stackContext.commit_ids.map(id=><p key={id}>{id}</p>)}{stackContext.individual_diff.files.map(path=><p key={path}>{path}</p>)}</div></details></div><div className="rounded-lg border p-3"><h3 className="text-sm font-semibold">Cumulative diff from target</h3><p className="mt-1 font-mono text-xs">{short(stackContext.target_revision)} → {short(stackContext.revision)}</p><p className="mt-2 text-xs">{stackContext.cumulative_diff.commit_count} commits · {stackContext.cumulative_diff.files.length} files · +{stackContext.cumulative_diff.additions} −{stackContext.cumulative_diff.deletions}</p><p className="mt-2 text-xs text-[var(--muted)]">Inherited changes are context, not a second approval request.</p></div></div><div className="mt-4"><h3 className="text-sm font-semibold">Exact upstream dependencies</h3>{stackContext.upstream_revisions.length?<div className="mt-2 flex flex-wrap gap-2">{stackContext.upstream_revisions.map(dependency=><Badge key={dependency.member_id} tone={dependency.current?"success":"warning"}>{dependency.member_id} @ {short(dependency.revision)} · {dependency.current?"current":"moved"}</Badge>)}</div>:<p className="mt-1 text-xs text-[var(--muted)]">This is the first independently reviewable layer.</p>}</div><div className="mt-4"><h3 className="text-sm font-semibold">Layer-bound collaboration evidence</h3>{stackContext.evidence.length?<div className="mt-2 space-y-2">{stackContext.evidence.map(item=><div key={item.kind+item.id} className="rounded border p-2 text-xs"><div className="flex flex-wrap gap-2"><Badge>{item.kind.replaceAll("_"," ")}</Badge><Badge tone={item.state==="current"?"success":"warning"}>{item.state}</Badge><code>{item.revision?short(item.revision):"no revision"}</code></div><p className="mt-1">{item.summary}</p></div>)}</div>:<p className="mt-1 text-xs text-[var(--muted)]">No discussion, decisions, checks, previews, findings, or acknowledgements are retained for this revision yet.</p>}</div>{stackContext.downstream_evidence_invalidated_by_upstream_change.length>0&&<p className="mt-4 rounded-lg bg-[var(--warning-soft)] p-3 text-xs text-[var(--warning)]">Changing this layer would stale evidence on {stackContext.downstream_evidence_invalidated_by_upstream_change.map(x=>x.member_id).join(", ")}. Those layers must be re-evaluated at their replacement revisions.</p>}{isOwner&&pull.status==="open"&&<form className="mt-4 flex flex-wrap gap-2" onSubmit={event=>acknowledgeStack(event,stackContext)}><select name="decision" className="rounded-lg border bg-white px-3 py-2 text-sm"><option value="acknowledged">Acknowledge exact layer</option><option value="changes_requested">Request changes</option></select><input name="note" maxLength={2000} className="min-w-56 flex-1 rounded-lg border px-3 py-2 text-sm" placeholder="Optional revision-bound note"/><Button disabled={pending||refreshRequired}>Record owner decision</Button></form>}<p className="mt-4 text-xs text-[var(--muted)]">{stackContext.authority}</p></Card>)}
      {pull.durable_migration && <Card className="p-5"><div className="flex flex-wrap items-center gap-2"><h2 className="font-semibold">Durable-state coexistence contract</h2><Badge tone="warning">{pull.durable_migration.kind.replace("_"," ")}</Badge><Badge>step {pull.durable_migration.step_id}</Badge></div><p className="mt-2 text-sm text-[var(--muted)]">Exact review context for old and new application behavior. It grants no schema, data-store, deployment, review, or merge authority.</p><dl className="mt-4 grid gap-3 text-sm sm:grid-cols-2">{[["Old readers",pull.durable_migration.contract.old_readers],["New readers",pull.durable_migration.contract.new_readers],["Old writers",pull.durable_migration.contract.old_writers],["New writers",pull.durable_migration.contract.new_writers],["Rollout flags",pull.durable_migration.contract.rollout_flags],["Transformations",pull.durable_migration.contract.transformations],["Ownership",pull.durable_migration.contract.ownership],["Rollback assumptions",pull.durable_migration.contract.rollback_assumptions]].map(([label,values])=><div key={label as string}><dt className="font-semibold">{label}</dt><dd className="mt-1 text-[var(--muted)]">{(values as string[]).join("; ")}</dd></div>)}<div className="sm:col-span-2"><dt className="font-semibold">Idempotency and retries</dt><dd className="mt-1 text-[var(--muted)]">{pull.durable_migration.contract.idempotency}</dd></div></dl></Card>}
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
        <a href="#performance" className="px-3 py-3 text-sm font-semibold text-[var(--muted)] hover:text-[var(--ink)]">Performance</a>
        <a href="#privacy" className="px-3 py-3 text-sm font-semibold text-[var(--muted)] hover:text-[var(--ink)]">Privacy</a>
        <a
          href="#files"
          className="px-3 py-3 text-sm font-semibold text-[var(--muted)] hover:text-[var(--ink)]"
        >
          Files changed {files.length}
        </a>
      </nav>
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_19rem]">
        <main className="min-w-0 space-y-6">
          <Card className="p-5" id="review-plan">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div><h2 className="font-semibold">Review plan</h2><p className="mt-1 text-sm text-[var(--muted)]">Expertise, questions, and evidence derived before approval.</p></div>
              {(isAuthor || isOwner) && pull.status === "open" && <form className="flex flex-wrap gap-2" onSubmit={createReviewPlan}><input name="risk_summary" maxLength={2000} className="rounded-lg border px-3 py-2 text-sm" placeholder="Optional risk context"/><input name="completion_rule" maxLength={2000} className="rounded-lg border px-3 py-2 text-sm" placeholder="Optional completion rule"/><Button disabled={pending||refreshRequired}>Derive new version</Button></form>}
            </div>
            {reviewPlanError ? <p role="alert" className="mt-4 rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">Review plan history is unavailable. Retained versions may exist; retry before treating this change as unplanned. {reviewPlanError}</p> : reviewPlans.length === 0 ? <p className="mt-4 rounded-lg bg-[var(--warning-soft)] p-3 text-sm text-[var(--warning)]">No review plan exists. Approvals alone do not describe which risks and evidence were reviewed.</p> : <div className="mt-4 space-y-4">{[...reviewPlans].reverse().map((plan,index)=><details key={plan.id} open={index===0} className="rounded-lg border p-4"><summary className="cursor-pointer font-semibold">Version {plan.version} · {short(plan.source_revision)} <Badge tone={plan.stale?"warning":"success"}>{plan.stale?"stale":"current"}</Badge></summary><p className="mt-3 text-sm">{plan.risk_summary}</p><div className="mt-3 flex flex-wrap gap-2">{plan.policy_requirements.map(x=><Badge key={x}>{x}</Badge>)}{plan.affected_commitments.map(x=><Badge key={x} tone="info">{x} commitment</Badge>)}</div>{plan.diagnostics.length>0&&<div className="mt-3 space-y-2">{plan.diagnostics.map((d,i)=><p key={`${d.code}-${i}`} className="rounded bg-[var(--warning-soft)] p-2 text-xs text-[var(--warning)]"><strong>{d.code.replaceAll("_"," ")}:</strong> {d.message}{d.attributed_to?` · attributed to ${short(d.attributed_to)}`:""}</p>)}</div>}<div className="mt-4 grid gap-3">{plan.areas.map(area=><div key={area.id} className="rounded-lg border p-3"><div className="flex flex-wrap gap-2"><h3 className="text-sm font-semibold">{area.title}</h3>{area.depends_on?.map(x=><Badge key={x}>after {x}</Badge>)}</div><p className="mt-1 text-xs text-[var(--muted)]">{area.rationale}</p><p className="mt-2 text-xs font-semibold">Acceptance questions</p><ul className="mt-1 list-disc pl-5 text-xs">{area.acceptance_questions.map(x=><li key={x}>{x}</li>)}</ul><p className="mt-2 text-xs font-semibold">Required evidence</p><ul className="mt-1 list-disc pl-5 text-xs">{area.required_evidence.map(x=><li key={x.kind}>{x.description}</li>)}</ul><details className="mt-2 text-xs"><summary className="cursor-pointer">{area.paths.length} exact changed paths</summary><div className="mt-1 font-mono">{area.paths.map(x=><p key={x}>{x}</p>)}</div></details><p className="mt-2 text-xs"><strong>Complete when:</strong> {area.completion_rule}</p></div>)}</div><p className="mt-4 text-xs"><strong>Whole-plan completion:</strong> {plan.completion_rule}</p><p className="mt-2 text-xs text-[var(--muted)]">{plan.authority}</p></details>)}</div>}
          </Card>
          <PullPrivacyReview repositoryID={repositoryID} pullRequestID={pullRequestID} sourceRevision={pull.source_commit_id} targetRevision={pull.target_commit_id} participant={participant} />
          <PullLocalizationReview repositoryID={repositoryID} pullRequestID={pullRequestID} />
          <PullPerformanceEvaluations repositoryID={repositoryID} pullRequestID={pullRequestID} />
          <PullAgentCandidates repositoryID={repositoryID} pullRequestID={pullRequestID} />
          <PullInfrastructurePlans repositoryID={repositoryID} pullRequestID={pullRequestID} participant={participant} />
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
		{pull.federated_contribution_id && <Card className="p-5"><div className="flex items-center justify-between gap-3"><div><h2 className="font-semibold">Federated collaboration</h2><p className="mt-1 text-xs text-[var(--muted)]">Signed claims retain their origin and verification; revision-bound evidence becomes stale normally.</p></div><Badge tone="info">remote contribution</Badge></div><div className="mt-4 space-y-3">{federationEvents.length?federationEvents.map(event=><div key={event.origin_instance_id+event.id} className="rounded-lg border border-[var(--line)] p-3 text-sm"><div className="flex flex-wrap items-center gap-2"><Badge tone={event.verification==="verified"?"success":"warning"}>{event.verification}</Badge><b>{event.kind.replace("_"," ")}</b>{event.decision&&<Badge tone={event.decision==="approved"?"success":event.decision==="changes_requested"?"danger":"neutral"}>{event.decision.replace("_"," ")}</Badge>}{event.state&&<Badge tone={event.state==="open"?"info":"neutral"}>{event.state}</Badge>}{event.stale&&<Badge tone="warning">stale revision</Badge>}<span className="font-mono text-xs text-[var(--muted)]">{event.actor}</span></div>{event.body&&<p className="mt-2 whitespace-pre-wrap">{event.body}</p>}{event.evidence&&<details className="mt-2"><summary className="cursor-pointer font-semibold">Shared evidence</summary><pre className="mt-2 max-h-64 overflow-auto rounded-lg bg-[var(--surface-subtle)] p-3 text-xs whitespace-pre-wrap">{JSON.stringify(event.evidence,null,2)}</pre></details>}<p className="mt-2 text-xs text-[var(--muted)]">origin {event.origin_instance_id.slice(0,12)}{event.revision?` · revision ${short(event.revision)}`:""} · {formatDate(event.created_at)}</p></div>):<p className="text-sm text-[var(--muted)]">No signed cross-instance activity has arrived yet.</p>}</div>{participant&&pull.status==="open"&&<form className="mt-4 flex gap-2" onSubmit={async e=>{e.preventDefault();const form=e.currentTarget;const body=String(new FormData(form).get("body")??"");setPending(true);try{await api(`/repositories/${repositoryID}/pulls/${pullRequestID}/federation-events`,{method:"POST",body:JSON.stringify({kind:"comment",body})},token);form.reset();await load()}catch(reason){setError(errorMessage(reason,"Signed comment could not be delivered."))}finally{setPending(false)}}}><input name="body" required maxLength={20000} className="min-w-0 flex-1 rounded-lg border border-[var(--line-strong)] px-3 py-2 text-sm" placeholder="Share a signed comment across instances"/><Button disabled={pending}>Send</Button></form>}</Card>}
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
          {(candidates.length > 0 || candidateError) && (
            <section id="integration-candidates" className="scroll-mt-24 space-y-3">
              <div className="flex items-baseline justify-between gap-3"><h2 className="text-lg font-semibold">Integration candidates</h2><span className="text-xs text-[var(--muted)]">Prospective merge evidence</span></div>
              {candidateError && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{candidateError}</p>}
              {[...candidates].reverse().map((candidate) => <Card key={candidate.id} className="p-5"><div className="flex flex-wrap items-center justify-between gap-3"><div><p className="font-semibold">Candidate <code>{short(candidate.commit_id)}</code></p><p className="mt-1 text-xs text-[var(--muted)]">Source <code>{short(candidate.source_commit_id)}</code> combined with base <code>{short(candidate.base_commit_id)}</code> · {formatDate(candidate.created_at)}</p></div><Badge tone={candidate.state === "passed" ? "success" : candidate.state === "failed" ? "danger" : "warning"}>{candidate.state}</Badge></div><div className="mt-4 grid gap-2 sm:grid-cols-3"><div className="rounded-lg bg-[var(--canvas)] p-3"><p className="text-xs text-[var(--muted)]">Base</p><code className="text-xs">{candidate.base_commit_id}</code></div><div className="rounded-lg bg-[var(--canvas)] p-3"><p className="text-xs text-[var(--muted)]">Pull revision</p><code className="text-xs">{candidate.source_commit_id}</code></div><div className="rounded-lg bg-[var(--canvas)] p-3"><p className="text-xs text-[var(--muted)]">Prospective result</p><code className="text-xs">{candidate.commit_id}</code></div></div><div className="mt-4 flex flex-wrap items-center justify-between gap-3"><p className="text-xs text-[var(--muted)]">{candidate.checks.length} candidate {candidate.checks.length === 1 ? "check" : "checks"}; logs and artifacts are retained in Verification checks below.</p><Button type="button" variant="secondary" onClick={() => void inspectCandidate(candidate.id)}>Explain collisions</Button></div></Card>)}
            </section>
          )}
          <PullRequestChecks
            repositoryID={repositoryID}
            pullRequestID={pullRequestID}
            participant={participant}
            sourceCommitID={pull.source_commit_id}
          />
          <AccessibilityAssessmentsWorkspace repositoryID={repositoryID} pullRequestID={pullRequestID} revision={pull.source_commit_id} participant={participant} />
          <ExtensionContributions repositoryID={repositoryID} pullRequestID={pullRequestID} revision={pull.source_commit_id} />
          <DocumentationPullReviewCard repositoryID={repositoryID} pullRequestID={pullRequestID} participant={participant} />
          <PullRequestPreviews repositoryID={repositoryID} pullRequestID={pullRequestID} participant={participant} owner={isOwner} />
          <PullInterfaceChecks repositoryID={repositoryID} pullRequestID={pullRequestID} participant={participant} />
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
          {(conflicts || conflictError) && <Card id="conflict-evidence" className="scroll-mt-24 p-5">
            <div className="flex flex-wrap items-center justify-between gap-2"><h2 className="font-semibold">Conflict evidence</h2>{conflicts && <Badge tone={conflicts.status === "current" ? "success" : "warning"}>{conflicts.status}</Badge>}</div>
            {conflictError && <p role="alert" className="mt-3 text-sm text-[var(--danger)]">{conflictError}</p>}
            {conflicts && <>
              <p className="mt-2 text-sm leading-6 text-[var(--muted)]">Compared exact revisions from merge base <code title={conflicts.base_commit_id}>{short(conflicts.base_commit_id)}</code>. This analysis does not change either branch.</p>
              {conflicts.candidate_id && <p className="mt-2 text-xs">Retained queue candidate <code>{short(conflicts.candidate_id)}</code></p>}
              {[...conflicts.stale_reasons, ...conflicts.incomplete].map((reason) => <p key={reason} className="mt-2 text-xs text-[var(--warning)]">{reason}</p>)}
              <div className="mt-4 grid gap-3">
                {[conflicts.source, conflicts.target].map((side, index) => <div key={`${side.branch}-${side.commit_id}`} className="rounded-lg bg-[var(--canvas)] p-3 text-xs"><p className="font-semibold">{index === 0 ? "Source intention" : "Target intention"} · <code>{side.branch}</code></p><p className="mt-1 text-[var(--muted)]" title={side.commit_id}>{short(side.commit_id)}{side.current_commit_id && side.current_commit_id !== side.commit_id ? ` · now ${short(side.current_commit_id)}` : ""}</p><p className="mt-1">Owners: {side.owner_ids.map(short).join(", ") || "not identified"}</p>{side.pull_requests.map((linked) => <div key={linked.id} className="mt-2 border-t border-[var(--line)] pt-2"><Link className="font-medium text-[var(--brand)] hover:underline" href={`/pulls/${repositoryID}/${linked.id}`}>{linked.title}</Link><p className="text-[var(--muted)]">author {short(linked.author_id)}{linked.task_id ? ` · task ${short(linked.task_id)}` : ""}{linked.proposal_id ? ` · proposal ${short(linked.proposal_id)}` : ""}</p><p>{linked.discussion_ids.length} discussion item(s) · {linked.decision_ids.length} decision(s)</p>{linked.acceptance_criteria.map((criterion) => <p key={criterion} className="mt-1">Acceptance: {criterion}</p>)}</div>)}</div>)}
              </div>
              <div className="mt-4 space-y-3"><p className="text-xs font-semibold text-[var(--muted)]">Colliding files, symbols, and contracts</p>{conflicts.files.length === 0 ? <p className="text-sm text-[var(--muted)]">No overlapping textual, structural, or detected semantic changes.</p> : conflicts.files.map((file) => <div key={file.path} className="rounded-lg border border-[var(--line)] p-3 text-xs"><div className="flex flex-wrap items-center gap-2"><code className="font-semibold">{file.path}</code>{file.kinds.map((kind) => <Badge key={kind} tone={kind === "textual" ? "danger" : "warning"}>{kind}</Badge>)}</div><p className="mt-2 text-[var(--muted)]">source {file.source_change} · target {file.target_change}{file.schema_or_interface ? " · schema/interface surface" : ""}</p>{file.symbols.length > 0 && <p className="mt-1">Shared changed symbols: {file.symbols.join(", ")}</p>}</div>)}</div>
              {conflicts.affected_checks.length > 0 && <p className="mt-4 text-xs"><strong>Affected checks:</strong> {conflicts.affected_checks.map((check) => check.name).join(", ")}</p>}
              {participant && conflicts.files.length > 0 && <><Button type="button" className="mt-4" disabled={pending} onClick={() => void launchConflictWorkspace()}>Resolve together in a workspace</Button><p className="mt-2 text-xs text-[var(--muted)]">Launches from these immutable revisions with both Git histories and repository setup preloaded. Publication remains subject to each branch and repository&apos;s ordinary permissions.</p></>}
              {conflicts.candidate_id && <Button type="button" variant="secondary" className="mt-4" onClick={() => void inspectCandidate()}>Return to current revisions</Button>}
            </>}
          </Card>}
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
                {readiness.assurance_impact && readiness.assurance_impact.length > 0 && (
                  <div className="mt-4 space-y-3 border-t border-[var(--line)] pt-4 text-xs">
                    <div className="flex items-center justify-between gap-2"><p className="font-semibold">Compliance impact</p><Badge tone={readiness.assurance_impact.every((assessment) => assessment.ready) ? "success" : "warning"}>{readiness.assurance_impact.every((assessment) => assessment.ready) ? "Acknowledged" : "Review required"}</Badge></div>
                    <p className="text-[var(--muted)]">Exact candidate decisions show whether this change alters a regulated promise before merge.</p>
                    {readiness.assurance_impact.map((assessment) => <div key={assessment.id} className="space-y-2 rounded-lg bg-[var(--canvas)] p-3">
                      <p className="font-medium">Assurance program v{assessment.program_version} · <code title={assessment.candidate.revision}>{short(assessment.candidate.revision)}</code>{assessment.stale ? " · stale" : ""}</p>
                      {assessment.impacts.map((impact) => <div key={impact.control_id} className="border-t border-[var(--line)] pt-2 first:border-0 first:pt-0">
                        <div className="flex items-start justify-between gap-3"><span className="font-medium">{impact.control_title}</span><Badge tone={impact.current && impact.applicability !== "uncertain" ? "neutral" : "warning"}>{impact.applicability.replaceAll("_", " ")}</Badge></div>
                        <p className="mt-1 text-[var(--muted)]">{impact.rationale}</p>
                        <p className="mt-1">Evidence: {impact.changed_evidence_ids?.length ?? 0} · tests: {impact.tests?.length ?? 0} · notices: {impact.notices?.length ?? 0} · retention actions: {impact.retention_actions?.length ?? 0} · exceptions: {impact.exception_ids?.length ?? 0}</p>
                        <p className="text-[var(--muted)]">Owner acknowledgements {impact.acknowledged_owner_ids?.length ?? 0} / {impact.required_owner_ids?.length ?? 0}{impact.residual_risk ? ` · residual risk: ${impact.residual_risk}` : ""}</p>
                      </div>)}
                      {assessment.events.map((event) => <p key={event.id}><Badge tone={event.kind === "challenge" ? "warning" : "neutral"}>{event.actor_type} {event.kind}</Badge> {event.body}</p>)}
                    </div>)}
                  </div>
                )}
                {readiness.accessibility_readiness && readiness.accessibility_readiness.requirements.length > 0 && (
                  <div className="mt-4 space-y-3 border-t border-[var(--line)] pt-4 text-xs">
                    <div className="flex items-center justify-between gap-2"><p className="font-semibold">Accessibility delivery bar</p><Badge tone={readiness.accessibility_readiness.ready ? "success" : "warning"}>{readiness.accessibility_readiness.ready ? "Current" : "Unresolved"}</Badge></div>
                    <p className="text-[var(--muted)]">Evidence and acknowledgement apply only to <code title={readiness.accessibility_readiness.revision}>{short(readiness.accessibility_readiness.revision)}</code>.</p>
                    {readiness.accessibility_readiness.requirements.map((requirement) => <div key={`${requirement.policy_id}-${requirement.kind}-${requirement.name}`} className="flex items-start justify-between gap-3"><span><span className="font-medium">{requirement.name}</span><span className="ml-1 text-[var(--muted)]">({requirement.kind.replaceAll("_", " ")})</span></span><Badge tone={requirement.status === "passed" ? "success" : "warning"}>{requirement.status}</Badge></div>)}
                    {readiness.accessibility_readiness.dissent.map((outcome) => <p key={`${outcome.actor_id}-${outcome.created_at}`}><Badge tone="warning">rejected</Badge> {outcome.rationale}</p>)}
                    {readiness.accessibility_readiness.active_exceptions.map((exception) => <div key={exception.id} className="rounded-lg bg-[var(--canvas)] p-3"><p className="font-semibold">Active justified exception</p><p>{exception.rationale}</p><p className="mt-1 text-[var(--muted)]">Follow-up: {exception.follow_up_work}</p></div>)}
                  </div>
                )}
                {readiness.preview_acceptance && (readiness.preview_acceptance.applicable.length > 0 || readiness.preview_acceptance.decisions.length > 0 || readiness.preview_acceptance.stale_decisions.length > 0 || readiness.preview_acceptance.findings.length > 0) && (
                  <div className="mt-4 space-y-3 border-t border-[var(--line)] pt-4 text-xs">
                    <div className="flex items-center justify-between gap-2"><p className="font-semibold">Stakeholder acceptance</p><Badge tone={readiness.preview_acceptance.blocking ? "warning" : "success"}>{readiness.preview_acceptance.blocking ? "Needs attention" : "Current"}</Badge></div>
                    <p className="text-[var(--muted)]">Policy v{readiness.preview_acceptance.policy_version} evaluated against <code title={readiness.preview_acceptance.revision}>{short(readiness.preview_acceptance.revision)}</code>.</p>
                    {readiness.preview_acceptance.missing.map((item) => <p key={`${item.requirement_id}-${item.scenario}-${item.role}`}><span className="font-medium">{item.scenario}</span> needs {item.role} acceptance.</p>)}
                    {readiness.preview_acceptance.decisions.map((decision) => <p key={decision.id}><Badge tone={decision.outcome === "accepted" || decision.outcome === "overridden" ? "success" : "warning"}>{decision.outcome}</Badge> <span className="font-medium">{decision.scenario}</span> by {decision.role}{decision.rationale ? ` — ${decision.rationale}` : ""}</p>)}
                    {readiness.preview_acceptance.findings.filter((finding) => finding.revision === readiness.evaluated_commit_id).map((finding) => <p key={finding.id}><Badge tone={finding.severity === "blocking" && finding.status !== "resolved" ? "warning" : "neutral"}>{finding.status}</Badge> {finding.title}</p>)}
                    {readiness.preview_acceptance.stale_decisions.length > 0 && <p className="text-[var(--muted)]">{readiness.preview_acceptance.stale_decisions.length} earlier {readiness.preview_acceptance.stale_decisions.length === 1 ? "decision applies" : "decisions apply"} only to an older revision.</p>}
                  </div>
                )}
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
                    disabled={pending || refreshRequired || !(readiness.integration_queue?.enabled ? readiness.can_enqueue : readiness.can_merge) || activelyQueued}
                    onClick={() => void (readiness.integration_queue?.enabled ? enqueue() : merge())}
                  >
                    {pending ? "Updating…" : activelyQueued ? "Queued for integration" : readiness.integration_queue?.enabled ? `Queue for ${pull.target_branch}` : `Merge into ${pull.target_branch}`}
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
                {pull.task_id && <div className="mt-3 rounded-lg border border-[var(--line)] p-3 text-xs"><p className="font-semibold">Connected proposal task</p><p className="mt-1 font-mono text-[var(--muted)]">task {pull.task_id}</p>{proposal.reasoning&&<><p className="mt-2">Justified by {proposal.reasoning.analysis_status.replaceAll("_", " ")} <code>{proposal.reasoning.assessment_id.slice(0,8)}</code> at <code>{proposal.reasoning.revision.slice(0,12)}</code>.</p><p className="mt-1">{proposal.reasoning.items.length} frozen claim, risk, citation, or verification item(s) · {(proposal.reasoning.acknowledgements ?? []).filter(item=>item.acknowledged_by).length} owner acknowledgement(s).</p></>}{pull.task_session_id && <p className="mt-1 font-mono text-[var(--muted)]">session {pull.task_session_id} · run {pull.task_run_id}</p>}<p className="mt-2">The source snapshot, automated checks, and execution evidence all belong to this review candidate.</p></div>}
                {pull.task_evidence && <Card className="mt-3 p-4 text-xs"><div className="flex flex-wrap items-center justify-between gap-2"><h3 className="font-semibold">Agent review handoff</h3><Badge tone={pull.task_evidence.outcome.completion_criteria.length > 0 && pull.task_evidence.outcome.completion_criteria.every((item) => item.status === "met") ? "success" : "warning"}>criteria {pull.task_evidence.outcome.completion_criteria.length === 0 ? "not reported" : pull.task_evidence.outcome.completion_criteria.every((item) => item.status === "met") ? "met" : "needs review"}</Badge></div><p className="mt-2">{pull.task_evidence.outcome.summary}</p><p className="mt-2 break-all font-mono text-[var(--muted)]">base {pull.task_evidence.base_revision} · result {pull.task_evidence.outcome.commit_id}</p><p className="mt-2"><strong>Authorship:</strong> agent {pull.task_evidence.agent_id} · launched by {pull.task_evidence.initiator_id}</p><p className="mt-2"><strong>Mandate:</strong> {pull.task_evidence.mandate}</p><p className="mt-2"><strong>Recorded criterion:</strong> {pull.task_evidence.completion_criteria}</p>{pull.task_evidence.reasoning.length > 0 && <ul className="mt-2 list-disc space-y-1 pl-5">{pull.task_evidence.reasoning.map((item) => <li key={`${item.kind}-${item.id}`}>{item.kind}: {item.summary} ({item.status})</li>)}</ul>}<h4 className="mt-3 font-semibold">Commands</h4><ul className="mt-1 space-y-1">{pull.task_evidence.outcome.commands.length ? pull.task_evidence.outcome.commands.map((command, index) => <li key={`${command.command}-${index}`}><code>{command.command}</code> → exit {command.exit_code}{command.summary ? ` · ${command.summary}` : ""}</li>) : <li>No agent-reported commands.</li>}</ul><h4 className="mt-3 font-semibold">Completion criteria</h4><ul className="mt-1 space-y-1">{pull.task_evidence.outcome.completion_criteria.length ? pull.task_evidence.outcome.completion_criteria.map((item, index) => <li key={`${item.criterion}-${index}`}><strong>{item.status.replaceAll("_", " ")}:</strong> {item.criterion} · {item.evidence}</li>) : <li>No completion criteria reported.</li>}</ul><h4 className="mt-3 font-semibold">Residual risks</h4><ul className="mt-1 space-y-1">{pull.task_evidence.outcome.unresolved_concerns.length ? pull.task_evidence.outcome.unresolved_concerns.map((concern) => <li key={concern}>{concern}</li>) : <li>None reported; ordinary review and checks still apply.</li>}</ul></Card>}
              </div>
            ) : (
              <p className="mt-2 text-sm leading-6 text-[var(--muted)]">
                No proposal is linked to this change.
              </p>
            )}
            {pull.workspace_id && <div className="mt-3 rounded-lg border border-[var(--line)] p-3 text-xs"><p className="font-semibold">Created in a collaborative workspace</p><p className="mt-1 font-mono text-[var(--muted)]">checkpoint {pull.workspace_checkpoint_id} · {pull.workspace_contributor_ids?.length ?? 0} contributors · {pull.workspace_command_ids?.length ?? 0} commands</p><Link href={`/workspaces/${pull.workspace_id}`} className="mt-2 inline-block text-[var(--brand)] hover:underline">Inspect originating workspace</Link></div>}
            {pull.contribution_evidence&&<Card className="mt-3 p-4 text-xs"><div className="flex flex-wrap items-center justify-between gap-2"><h3 className="font-semibold">Guided contribution context</h3><Badge tone="success">project requirements met at publication</Badge></div><p className="mt-2">Opportunity <code>{pull.contribution_evidence.opportunity_id.slice(0,8)}</code> · pathway revision {pull.contribution_evidence.pathway_version} · base <code>{pull.contribution_evidence.upstream_revision.slice(0,12)}</code></p><p className="mt-2">{pull.contribution_evidence.setup_evidence.length} setup/verification outcome(s) · {pull.contribution_evidence.mentor_guidance_ids.length} mentor guidance item(s) · {pull.contribution_evidence.agent_assistance_ids.length} agent assistance item(s) · {pull.workspace_contributor_ids?.length??0} contributor(s)</p><h4 className="mt-3 font-semibold">Acceptance criteria</h4><ul className="mt-1 list-disc space-y-1 pl-5">{pull.contribution_evidence.acceptance_criteria.map(x=><li key={x}>{x}</li>)}</ul>{pull.contribution_evidence.coaching_needs.length>0&&<><h4 className="mt-3 font-semibold">Coaching context — non-blocking</h4><ul className="mt-1 list-disc space-y-1 pl-5">{pull.contribution_evidence.coaching_needs.map(x=><li key={x.code+x.message}>{x.message}</li>)}</ul></>}<p className="mt-3 text-[var(--muted)]">Discussion, reproductions, reviews, required checks, owner acknowledgements, integration queues, and merge permissions remain the project&apos;s ordinary governance.</p></Card>}
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
