"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import Link from "next/link";
import {
  api,
  type KnowledgeAnswer,
  type Repository,
  type SupportSolution,
  type SupportThread,
  type SupportVerificationAttempt,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

const field =
  "mt-1 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 py-2 outline-none focus:border-[var(--brand)]";
const lines = (v: FormDataEntryValue | null) =>
  String(v || "")
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
const message = (v: unknown) =>
  v instanceof Error
    ? v.message
    : "The support request could not be completed.";
async function attachment(file: File) {
  if (file.size > 1 << 20) throw new Error(`${file.name} exceeds 1 MiB.`);
  const ext = file.name.split(".").pop()?.toLowerCase();
  const kind =
    file.name.endsWith(".log") || file.type === "text/plain"
      ? "log"
      : ext === "json" || ext === "yaml" || ext === "yml" || ext === "toml"
        ? "configuration"
        : "sample_code";
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return {
    kind,
    name: file.name,
    media_type: file.type || "text/plain",
    size: file.size,
    data: btoa(binary),
  };
}

export function SupportWorkspace() {
  const { token, user, loading } = useAuth();
  const loadSequence = useRef(0),
    selectionSequence = useRef(0);
  const [repositories, setRepositories] = useState<Repository[]>([]),
    [repoID, setRepoID] = useState(""),
    [threads, setThreads] = useState<SupportThread[]>([]),
    [selected, setSelected] = useState<SupportThread | null>(null),
    [solutions, setSolutions] = useState<SupportSolution[]>([]),
    [answers, setAnswers] = useState<KnowledgeAnswer[]>([]),
    [attempts, setAttempts] = useState<SupportVerificationAttempt[]>([]),
    [error, setError] = useState(""),
    [pending, setPending] = useState(false);
  const load = useCallback(async () => {
    if (!token) return;
    const sequence = ++loadSequence.current;
    try {
      const page = await api<{ repositories: Repository[] }>(
        "/repositories?limit=100",
        {},
        token,
      );
      if (sequence !== loadSequence.current) return;
      setRepositories(page.repositories);
      const id = repoID || page.repositories[0]?.id || "";
      if (id) {
        setRepoID(id);
        const [threadData, solutionData] = await Promise.all([
          api<{ threads: SupportThread[] }>(
            `/repositories/${id}/support-threads`,
            {},
            token,
          ),
          api<{ solutions: SupportSolution[] }>(
            `/repositories/${id}/support-solutions`,
            {},
            token,
          ),
        ]);
        if (sequence !== loadSequence.current) return;
        setThreads(threadData.threads);
        setSolutions(solutionData.solutions);
      }
    } catch (e) {
      if (sequence === loadSequence.current) setError(message(e));
    }
  }, [token, repoID]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    const sequence = loadSequence.current,
      selection = selectionSequence.current,
      repositoryID = repoID;
    setPending(true);
    setError("");
    const form = event.currentTarget,
      d = new FormData(form);
    try {
      const files = d
        .getAll("attachments")
        .filter((x): x is File => x instanceof File && x.size > 0);
      if (files.length > 10)
        throw new Error("Support threads accept at most 10 attachments.");
      const created = await api<SupportThread>(
        `/repositories/${repositoryID}/support-threads`,
        {
          method: "POST",
          body: JSON.stringify({
            title: d.get("title"),
            body: d.get("body"),
            target: {
              kind: d.get("target_kind"),
              resource_id: d.get("resource_id"),
              label: d.get("target_label"),
              version: d.get("target_version"),
            },
            environment: {
              operating_system: d.get("operating_system"),
              runtime: d.get("runtime"),
              dependencies: lines(d.get("dependencies")),
              deployment: d.get("deployment"),
              details: d.get("environment_details"),
            },
            goal: d.get("goal"),
            attempted_steps: lines(d.get("attempted_steps")),
            urgency: d.get("urgency"),
            audience: d.get("audience"),
            contact_preferences: {
              reply_in_thread: true,
              email: d.get("email"),
              allow_maintainer_contact: d.get("maintainer_contact") === "on",
            },
            attachments: await Promise.all(files.map(attachment)),
          }),
        },
        token,
      );
      if (sequence !== loadSequence.current) return;
      setThreads((x) => [created, ...x]);
      if (selection === selectionSequence.current) setSelected(created);
      form.reset();
    } catch (e) {
      if (
        sequence === loadSequence.current &&
        selection === selectionSequence.current
      )
        setError(message(e));
    } finally {
      if (sequence === loadSequence.current) setPending(false);
    }
  }
  async function inspect(v: SupportThread) {
    if (!token) return;
    const sequence = loadSequence.current,
      selection = ++selectionSequence.current;
    try {
      const [thread, proof, guidance] = await Promise.all([
        api<SupportThread>(
          `/repositories/${v.repository_id}/support-threads/${v.id}`,
          {},
          token,
        ),
        api<{ attempts: SupportVerificationAttempt[] }>(
          `/repositories/${v.repository_id}/support-threads/${v.id}/verification-attempts`,
          {},
          token,
        ).catch(() => ({ attempts: [] })),
        api<{ answers: KnowledgeAnswer[] }>(
          `/repositories/${v.repository_id}/knowledge-answers`,
          {},
          token,
        ),
      ]);
      if (
        sequence === loadSequence.current &&
        selection === selectionSequence.current
      ) {
        setSelected(thread);
        setAttempts(proof.attempts);
        setAnswers(guidance.answers);
      }
    } catch (e) {
      if (
        sequence === loadSequence.current &&
        selection === selectionSequence.current
      )
        setError(message(e));
    }
  }
  async function status(next: SupportThread["status"]) {
    if (!token || !user || !selected) return;
    const sequence = loadSequence.current,
      selection = selectionSequence.current,
      thread = selected;
    try {
      const updated = await api<SupportThread>(
        `/repositories/${thread.repository_id}/support-threads/${thread.id}`,
        {
          method: "PATCH",
          body: JSON.stringify({
            status: next,
            expected_version: thread.version,
            message: `Marked ${next.replaceAll("_", " ")} from the support workspace.`,
          }),
        },
        token,
      );
      if (sequence !== loadSequence.current) return;
      setThreads((xs) => xs.map((x) => (x.id === updated.id ? updated : x)));
      if (selection === selectionSequence.current) setSelected(updated);
    } catch (e) {
      if (
        sequence === loadSequence.current &&
        selection === selectionSequence.current
      )
        setError(message(e));
    }
  }
  async function publish(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token || !selected) return;
    const form = event.currentTarget,
      d = new FormData(form),
      attempt = attempts.find((x) => x.id === d.get("attempt_id")),
      answer = answers.find((x) => x.id === attempt?.answer_id);
    if (!answer || !attempt) return;
    setPending(true);
    try {
      const created = await api<SupportSolution>(
        `/repositories/${selected.repository_id}/support-threads/${selected.id}/solutions`,
        {
          method: "POST",
          body: JSON.stringify({
            answer_id: answer.id,
            answer_revision_id: attempt.answer_revision_id,
            verification_attempt_id: attempt.id,
            title: d.get("solution_title"),
            summary: d.get("solution_summary"),
            audience: d.get("solution_audience"),
            applicable_versions: lines(d.get("versions")),
            limitations: lines(d.get("limitations")),
            links: [{ kind: "search", label: String(d.get("solution_title")) }],
          }),
        },
        token,
      );
      setSolutions((x) => [created, ...x]);
      form.reset();
      await inspect(selected);
    } catch (e) {
      setError(message(e));
    } finally {
      setPending(false);
    }
  }
  async function escalate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token || !user || !selected) return;
    const form = event.currentTarget,
      d = new FormData(form),
      kind = String(d.get("resource_kind"));
    setPending(true);
    setError("");
    try {
      const taskTitles = lines(d.get("task_titles"));
      const criteria = lines(d.get("acceptance_criteria"));
      const updated = await api<SupportThread>(
        `/repositories/${selected.repository_id}/support-threads/${selected.id}/escalations`,
        {
          method: "POST",
          body: JSON.stringify({
            classification: d.get("classification"),
            resource_kind: kind,
            expected_version: selected.version,
            acceptance_criteria: criteria,
            documentation_path: d.get("documentation_path"),
            tasks:
              kind === "ordered_work"
                ? taskTitles.map((title, index) => ({
                    title,
                    outcome: criteria[index] || criteria[criteria.length - 1],
                    risk: "The support need remains unresolved.",
                    verification_plan: criteria[index] || criteria.join("; "),
                    assignee_type: index % 2 === 0 ? "human" : "agent",
                    assignee_id: index % 2 === 0 ? user.id : "",
                  }))
                : [],
          }),
        },
        token,
      );
      setSelected(updated);
      setThreads((items) =>
        items.map((item) => (item.id === updated.id ? updated : item)),
      );
      form.reset();
    } catch (e) {
      setError(message(e));
    } finally {
      setPending(false);
    }
  }
  async function govern(
    v: SupportSolution,
    action: "archive" | "request_revalidation",
  ) {
    if (!token) return;
    try {
      const versions =
        action === "request_revalidation"
          ? prompt("Versions requiring fresh proof (comma separated)", "")
              ?.split(",")
              .map((x) => x.trim())
              .filter(Boolean)
          : [];
      if (action === "request_revalidation" && !versions?.length) return;
      const updated = await api<SupportSolution>(
        `/repositories/${v.repository_id}/support-solutions/${v.id}/actions`,
        {
          method: "POST",
          body: JSON.stringify({
            action,
            expected_version: v.version,
            versions,
            message:
              action === "archive"
                ? "Archived as obsolete from the support workspace."
                : "Newer versions require a fresh workspace rerun.",
          }),
        },
        token,
      );
      setSolutions((xs) => xs.map((x) => (x.id === updated.id ? updated : x)));
    } catch (e) {
      setError(message(e));
    }
  }
  if (loading)
    return (
      <Card className="p-8 text-sm text-[var(--muted)]">Loading support…</Card>
    );
  if (!user)
    return (
      <Card className="p-8 text-center">
        <h1 className="text-2xl font-semibold">
          Ask for project help with usable context
        </h1>
        <Link
          href="/?access=signin"
          className="mt-5 inline-flex rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-semibold text-white"
        >
          Sign in
        </Link>
      </Card>
    );
  return (
    <div className="space-y-6">
      <header>
        <p className="text-sm font-semibold text-[var(--brand)]">
          Developer support
        </p>
        <h1 className="mt-1 text-3xl font-semibold">
          Ask where maintainers can understand the software state
        </h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Keep the goal, exact target, environment, attempts, permitted
          evidence, contact route, and status together.
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
      <Card className="p-5">
        <form onSubmit={create} className="grid gap-4 lg:grid-cols-2">
          <label className="text-sm font-semibold">
            Repository
            <select
              className={field}
              value={repoID}
              onChange={(e) => {
                loadSequence.current++;
                selectionSequence.current++;
                setRepoID(e.target.value);
                setThreads([]);
                setSolutions([]);
                setSelected(null);
                setAttempts([]);
                setAnswers([]);
                setPending(false);
              }}
              required
            >
              {repositories.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </select>
          </label>
          <label className="text-sm font-semibold">
            What is the question against?
            <select name="target_kind" className={field}>
              {[
                ["repository", "Repository"],
                ["package", "Package"],
                ["release", "Release"],
                ["api", "API"],
                ["documented_journey", "Documented journey"],
                ["error", "Error"],
              ].map(([v, l]) => (
                <option key={v} value={v}>
                  {l}
                </option>
              ))}
            </select>
          </label>
          <label className="text-sm font-semibold">
            Target name
            <input
              name="target_label"
              className={field}
              required
              placeholder="Upload API or package name"
            />
          </label>
          <label className="text-sm font-semibold">
            Target ID{" "}
            <span className="font-normal text-[var(--muted)]">
              (when known)
            </span>
            <input name="resource_id" className={field} />
          </label>
          <label className="text-sm font-semibold">
            Exact version or revision
            <input name="target_version" className={field} />
          </label>
          <label className="text-sm font-semibold">
            Urgency
            <select name="urgency" className={field} defaultValue="normal">
              <option>low</option>
              <option>normal</option>
              <option>high</option>
              <option>urgent</option>
            </select>
          </label>
          <label className="text-sm font-semibold lg:col-span-2">
            Question title
            <input name="title" className={field} required />
          </label>
          <label className="text-sm font-semibold lg:col-span-2">
            Question
            <textarea name="body" className={`${field} min-h-24`} required />
          </label>
          <label className="text-sm font-semibold lg:col-span-2">
            Goal
            <textarea
              name="goal"
              className={`${field} min-h-20`}
              placeholder="What outcome are you trying to achieve?"
            />
          </label>
          <label className="text-sm font-semibold lg:col-span-2">
            What you tried{" "}
            <span className="font-normal text-[var(--muted)]">
              (one step per line)
            </span>
            <textarea name="attempted_steps" className={`${field} min-h-24`} />
          </label>
          <label className="text-sm font-semibold">
            Operating system
            <input name="operating_system" className={field} />
          </label>
          <label className="text-sm font-semibold">
            Runtime / SDK
            <input name="runtime" className={field} />
          </label>
          <label className="text-sm font-semibold">
            Deployment context
            <input name="deployment" className={field} />
          </label>
          <label className="text-sm font-semibold">
            Dependencies{" "}
            <span className="font-normal text-[var(--muted)]">
              (one per line)
            </span>
            <textarea name="dependencies" className={field} />
          </label>
          <label className="text-sm font-semibold lg:col-span-2">
            Other environment details
            <textarea name="environment_details" className={field} />
          </label>
          <label className="text-sm font-semibold">
            Audience
            <select name="audience" className={field} defaultValue="public">
              <option value="public">Public</option>
              <option value="maintainers">Maintainers and author</option>
            </select>
          </label>
          <label className="text-sm font-semibold">
            Contact email{" "}
            <span className="font-normal text-[var(--muted)]">
              (never public)
            </span>
            <input name="email" type="email" className={field} />
          </label>
          <label className="flex items-center gap-2 text-sm lg:col-span-2">
            <input name="maintainer_contact" type="checkbox" />
            Allow maintainers to contact me outside this thread.
          </label>
          <label className="text-sm font-semibold lg:col-span-2">
            Permitted evidence{" "}
            <span className="font-normal text-[var(--muted)]">
              up to 10 logs, configuration, or sample-code files; 1 MiB each
            </span>
            <input
              name="attachments"
              type="file"
              multiple
              className={field}
              accept="text/plain,application/json,.log,.yaml,.yml,.toml,.js,.ts,.go,.py"
            />
          </label>
          <Button className="lg:col-span-2" disabled={pending || !repoID}>
            {pending ? "Opening question…" : "Ask for help"}
          </Button>
        </form>
      </Card>
      <section>
        <h2 className="text-lg font-semibold">Support threads</h2>
        <div className="mt-3 grid gap-3 lg:grid-cols-2">
          {threads.map((v) => (
            <button
              key={v.id}
              onClick={() => void inspect(v)}
              className="text-left"
            >
              <Card className="h-full p-4 hover:border-[var(--line-strong)]">
                <div className="flex gap-2">
                  <Badge>{v.status.replaceAll("_", " ")}</Badge>
                  <Badge tone={v.diagnostics.length ? "warning" : "success"}>
                    {v.diagnostics.length
                      ? `${v.diagnostics.length} context gap(s)`
                      : "context ready"}
                  </Badge>
                </div>
                <h3 className="mt-2 font-semibold">{v.title}</h3>
                <p className="mt-1 text-sm text-[var(--muted)]">
                  {v.target.label} · {v.target.version || "version missing"}
                </p>
              </Card>
            </button>
          ))}
        </div>
      </section>
      {selected && (
        <Card className="p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-xl font-semibold">{selected.title}</h2>
              <p className="mt-1 text-xs text-[var(--muted)]">
                By {selected.author_id} · {selected.audience} ·{" "}
                {selected.target.kind.replaceAll("_", " ")}{" "}
                {selected.target.label}
              </p>
            </div>
            <div className="flex gap-2">
              {(["needs_context", "answered", "closed"] as const)
                .filter((x) => x !== selected.status)
                .map((x) => (
                  <Button
                    key={x}
                    variant="secondary"
                    onClick={() => void status(x)}
                  >
                    {x.replaceAll("_", " ")}
                  </Button>
                ))}
            </div>
          </div>
          <p className="mt-4 whitespace-pre-wrap text-sm">{selected.body}</p>
          <div className="mt-4 grid gap-3 lg:grid-cols-2">
            <div className="rounded-lg bg-[var(--surface)] p-3 text-sm">
              <strong>Goal</strong>
              <p className="mt-1">{selected.goal || "Not provided"}</p>
            </div>
            <div className="rounded-lg bg-[var(--surface)] p-3 text-sm">
              <strong>Environment</strong>
              <p className="mt-1">
                {[
                  selected.environment.operating_system,
                  selected.environment.runtime,
                  selected.environment.deployment,
                  selected.environment.details,
                ]
                  .filter(Boolean)
                  .join(" · ") || "Not provided"}
              </p>
            </div>
          </div>
          {selected.diagnostics.length > 0 && (
            <div className="mt-4 rounded-lg bg-[var(--warning-soft)] p-3">
              <strong className="text-sm">Missing diagnostic context</strong>
              {selected.diagnostics.map((x) => (
                <p key={x.kind} className="mt-1 text-sm">
                  {x.message}
                </p>
              ))}
            </div>
          )}
          {selected.attachments.length > 0 && (
            <div className="mt-4">
              <strong className="text-sm">Permitted evidence</strong>
              <div className="mt-2 flex flex-wrap gap-2">
                {selected.attachments.map((a) => (
                  <a
                    key={a.id}
                    download={a.name}
                    href={`data:${a.media_type};base64,${a.data}`}
                    className="text-sm font-semibold text-[var(--brand)]"
                  >
                    {a.name}
                  </a>
                ))}
              </div>
            </div>
          )}
          {selected.related?.length ? (
            <div className="mt-4">
              <strong className="text-sm">Related answers and issues</strong>
              {selected.related.map((x) => (
                <p key={`${x.kind}-${x.id}`} className="mt-1 text-sm">
                  {x.kind.replaceAll("_", " ")}: {x.title} · {x.status}
                </p>
              ))}
            </div>
          ) : null}
          {selected.escalations?.length ? (
            <div className="mt-4 rounded-lg bg-[var(--surface)] p-3">
              <strong className="text-sm">Governed improvement progress</strong>
              {selected.escalations.map((item) => (
                <p key={item.id} className="mt-2 text-sm">
                  {item.status === "published" ? (
                    <Link
                      href={item.resource_url}
                      className="font-semibold text-[var(--brand)]"
                    >
                      {item.resource_kind.replaceAll("_", " ")}
                    </Link>
                  ) : (
                    <strong>{item.resource_kind.replaceAll("_", " ")}</strong>
                  )}{" "}
                  · {item.status} · {item.classification.replaceAll("_", " ")} ·{" "}
                  {item.affected_version || "version not supplied"}
                </p>
              ))}
            </div>
          ) : null}
          <div className="mt-4 border-t border-[var(--line)] pt-3">
            {selected.history.map((x) => (
              <p key={x.id} className="text-xs text-[var(--muted)]">
                {new Date(x.created_at).toLocaleString()} · {x.actor_id} ·{" "}
                {x.kind.replaceAll("_", " ")}
                {x.message ? ` · ${x.message}` : ""}
              </p>
            ))}
          </div>
          {selected.status !== "closed" && (
            <form
              onSubmit={escalate}
              className="mt-5 grid gap-3 border-t border-[var(--line)] pt-5 lg:grid-cols-2"
            >
              <div className="lg:col-span-2">
                <h3 className="font-semibold">Turn this gap into governed work</h3>
                <p className="mt-1 text-sm text-[var(--muted)]">
                  The user need, affected version, permitted reproduction, and
                  criteria are copied forward. Private attachments and contact
                  details stay in support; ordinary access, review, checks, and
                  merge rules still apply.
                </p>
              </div>
              <label className="text-sm font-semibold">
                Classification
                <select name="classification" className={field}>
                  <option value="defect">Defect</option>
                  <option value="documentation_gap">Documentation gap</option>
                  <option value="missing_example">Missing example</option>
                  <option value="compatibility_problem">Compatibility problem</option>
                  <option value="product_opportunity">Product opportunity</option>
                </select>
              </label>
              <label className="text-sm font-semibold">
                Governed destination
                <select name="resource_kind" className={field} defaultValue="ordered_work">
                  <option value="issue">Issue</option>
                  <option value="documentation_task">Documentation task</option>
                  <option value="proposal">Proposal</option>
                  <option value="ordered_work">Ordered human/agent work</option>
                </select>
              </label>
              <label className="text-sm font-semibold lg:col-span-2">
                Acceptance criteria, one per line
                <textarea name="acceptance_criteria" className={`${field} min-h-24`} required />
              </label>
              <label className="text-sm font-semibold">
                Documentation path (for a documentation task)
                <input name="documentation_path" className={field} defaultValue="docs/" />
              </label>
              <label className="text-sm font-semibold">
                Ordered task titles, one per line
                <textarea name="task_titles" className={field} placeholder="Human diagnosis&#10;Agent implementation" />
              </label>
              <Button disabled={pending}>Create governed improvement</Button>
            </form>
          )}
          {attempts.some((x) => x.result === "passed" && !x.stale) && (
            <form
              onSubmit={publish}
              className="mt-5 grid gap-3 border-t border-[var(--line)] pt-5 lg:grid-cols-2"
            >
              <div className="lg:col-span-2">
                <h3 className="font-semibold">
                  Publish the tested answer for the next developer
                </h3>
                <p className="mt-1 text-sm text-[var(--muted)]">
                  Publication freezes the exact answer revision and passing
                  attempt. Later lifecycle actions retain this record.
                </p>
              </div>
              <label className="text-sm font-semibold">
                Tested answer and passing attempt
                <select name="attempt_id" className={field} required>
                  {attempts
                    .filter((x) => x.result === "passed" && !x.stale)
                    .map((x) => (
                      <option key={x.id} value={x.id}>
                        {answers.find((answer) => answer.id === x.answer_id)
                          ?.question || "Tested answer"}{" "}
                        · {x.software_version} · {x.id.slice(0, 8)}
                      </option>
                    ))}
                </select>
              </label>
              <label className="text-sm font-semibold lg:col-span-2">
                Reusable title
                <input name="solution_title" className={field} required />
              </label>
              <label className="text-sm font-semibold lg:col-span-2">
                What this solves
                <textarea
                  name="solution_summary"
                  className={`${field} min-h-20`}
                  required
                />
              </label>
              <label className="text-sm font-semibold">
                Applicable versions
                <textarea
                  name="versions"
                  className={field}
                  defaultValue={selected.target.version}
                  required
                />
              </label>
              <label className="text-sm font-semibold">
                Limitations, one per line
                <textarea name="limitations" className={field} />
              </label>
              <label className="text-sm font-semibold">
                Reuse audience
                <select
                  name="solution_audience"
                  className={field}
                  defaultValue={
                    selected.audience === "public" ? "public" : "participants"
                  }
                >
                  <option value="public">Public</option>
                  <option value="participants">Repository participants</option>
                </select>
              </label>
              <Button disabled={pending}>Publish tested solution</Button>
            </form>
          )}
        </Card>
      )}
      <section>
        <h2 className="text-lg font-semibold">Reusable tested solutions</h2>
        <p className="mt-1 text-sm text-[var(--muted)]">
          Search-visible guidance keeps its applicable versions, limitations,
          proof, credits, and lifecycle history together.
        </p>
        <div className="mt-3 grid gap-3 lg:grid-cols-2">
          {solutions.map((v) => (
            <Card key={v.id} className="p-4">
              <div className="flex flex-wrap gap-2">
                <Badge tone={v.status === "published" ? "success" : "warning"}>
                  {v.status.replaceAll("_", " ")}
                </Badge>
                {v.applicable_versions.map((x) => (
                  <Badge key={x}>{x}</Badge>
                ))}
              </div>
              <h3 className="mt-2 font-semibold">{v.title}</h3>
              <p className="mt-1 text-sm">{v.summary}</p>
              {v.limitations.length > 0 && (
                <p className="mt-2 text-xs text-[var(--muted)]">
                  Limits: {v.limitations.join(" · ")}
                </p>
              )}
              <p className="mt-2 text-xs text-[var(--muted)]">
                Proof {v.verification_attempt_id.slice(0, 8)} · Credit{" "}
                {v.credits
                  .map((x) => `${x.actor_id} (${x.role.replaceAll("_", " ")})`)
                  .join(", ")}
              </p>
              {v.status !== "archived" && (
                <div className="mt-3 flex gap-2">
                  <Button
                    variant="secondary"
                    onClick={() => void govern(v, "request_revalidation")}
                  >
                    Request revalidation
                  </Button>
                  <Button
                    variant="secondary"
                    onClick={() => void govern(v, "archive")}
                  >
                    Archive
                  </Button>
                </div>
              )}
            </Card>
          ))}
        </div>
      </section>
    </div>
  );
}
