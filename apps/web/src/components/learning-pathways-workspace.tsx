"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { api, type LearningAssessment, type LearningAssessmentAttempt, type LearningPathway, type Repository } from "@/lib/api";
import { useAuth } from "./auth";

const lines = (v: string) =>
  v
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
const badge = (status?: string) =>
  status === "current"
    ? "border-emerald-200 bg-emerald-50 text-emerald-800"
    : status === "unsupported"
      ? "border-violet-200 bg-violet-50 text-violet-800"
      : "border-amber-200 bg-amber-50 text-amber-800";

export function LearningPathwaysWorkspace({
  repositoryId,
}: {
  repositoryId: string;
}) {
  const { token } = useAuth();
  const [items, setItems] = useState<LearningPathway[]>([]);
  const [selected, setSelected] = useState<LearningPathway | null>(null);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(false);
  const [requestId, setRequestId] = useState(() => crypto.randomUUID());
  const [form, setForm] = useState({
    slug: "contributor",
    role: "Contributor",
    outcome: "Contribute safely to this project",
    prerequisites: "Basic Git",
    objectives: "Understand the project architecture",
    revisions: "",
    minutes: "120",
    accessibility: "Keyboard-accessible tools",
    locales: "en-US",
    evidence: "Passing exercise evidence",
    modules: "[]",
    mentors: "[]",
    environments: "[]",
  });
  const load = useCallback(async () => {
    try {
      const d = await api<{ pathways: LearningPathway[] }>(
        `/repositories/${repositoryId}/learning-pathways`,
        {},
        token,
      );
      setItems(d.pathways);
      if (!selected && d.pathways[0]) setSelected(d.pathways[0]);
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Could not load learning pathways",
      );
    }
  }, [repositoryId, token, selected]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function publish() {
    try {
      const modules = JSON.parse(form.modules),
        mentors = JSON.parse(form.mentors),
        environments = JSON.parse(form.environments);
      const current = items.find((x) => x.slug === form.slug);
      const published = await api<LearningPathway>(
        `/repositories/${repositoryId}/learning-pathways/${form.slug}`,
        {
          method: "PUT",
          body: JSON.stringify({
            request_id: requestId,
            expected_version: current?.version ?? 0,
            pathway: {
              role: form.role,
              outcome: form.outcome,
              prerequisites: lines(form.prerequisites),
              objectives: lines(form.objectives),
              supported_revisions: lines(form.revisions),
              expected_minutes: Number(form.minutes),
              accessibility_needs: lines(form.accessibility),
              locales: lines(form.locales),
              completion_evidence: lines(form.evidence),
              modules,
              mentors,
              environments,
            },
          }),
        },
        token,
      );
      setEditing(false);
      setRequestId(crypto.randomUUID());
      setItems((xs) => [
        published,
        ...xs.filter((x) => x.slug !== published.slug),
      ]);
      setSelected(published);
      setError("");
      try {
        const d = await api<{ pathways: LearningPathway[] }>(
          `/repositories/${repositoryId}/learning-pathways`,
          {},
          token,
        );
        setItems(d.pathways);
        setSelected(
          d.pathways.find((x) => x.slug === published.slug) ?? published,
        );
      } catch {
        setError(
          "Pathway published, but the pathway list could not be refreshed.",
        );
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not publish pathway");
    }
  }
  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[.18em] text-[var(--brand-strong)]">
            Project learning
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-tight">
            Learn for real project work
          </h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--muted)]">
            Versioned roles, ordered practice, exact project material, and
            visible support gaps—maintained beside the software.
          </p>
        </div>
        {token && (
          <button
            onClick={() => setEditing(!editing)}
            className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-semibold text-white"
          >
            {editing ? "Cancel" : "Publish pathway"}
          </button>
        )}
      </header>
      {error && (
        <p
          role="alert"
          className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-800"
        >
          {error}
        </p>
      )}
      {editing && (
        <section className="rounded-xl border border-[var(--line)] bg-white p-5">
          <h2 className="font-semibold">New immutable revision</h2>
          <div className="mt-4 grid gap-3 md:grid-cols-2">
            {(
              [
                ["slug", "Stable slug"],
                ["role", "Project role"],
                ["outcome", "Practical outcome"],
                ["minutes", "Expected minutes"],
                ["revisions", "Supported 40-character revisions"],
                ["locales", "Locales"],
                ["prerequisites", "Prerequisites"],
                ["objectives", "Objectives"],
                ["accessibility", "Accessibility needs"],
                ["evidence", "Completion evidence"],
              ] as const
            ).map(([key, label]) => (
              <label
                key={key}
                className="text-xs font-semibold text-[var(--muted)]"
              >
                {label}
                <textarea
                  value={form[key]}
                  onChange={(e) => setForm({ ...form, [key]: e.target.value })}
                  className="mt-1 min-h-10 w-full rounded-lg border border-[var(--line)] p-2 text-sm text-[var(--ink)]"
                />
              </label>
            ))}
          </div>
          <p className="mt-4 text-xs text-[var(--muted)]">
            Modules, mentors, and environments use the public API object shapes,
            making exact links and support ownership fully inspectable.
          </p>
          {(
            [
              ["modules", "Ordered modules JSON"],
              ["mentors", "Mentors JSON"],
              ["environments", "Learner environments JSON"],
            ] as const
          ).map(([key, label]) => (
            <label
              key={key}
              className="mt-3 block text-xs font-semibold text-[var(--muted)]"
            >
              {label}
              <textarea
                value={form[key]}
                onChange={(e) => setForm({ ...form, [key]: e.target.value })}
                className="mt-1 min-h-28 w-full rounded-lg border border-[var(--line)] p-2 font-mono text-xs text-[var(--ink)]"
              />
            </label>
          ))}
          <button
            onClick={publish}
            className="mt-4 rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-semibold text-white"
          >
            Publish revision
          </button>
        </section>
      )}
      <div className="grid gap-5 lg:grid-cols-[18rem_1fr]">
        <aside className="space-y-2">
          {items.map((x) => (
            <button
              key={x.slug}
              onClick={() => setSelected(x)}
              className={`w-full rounded-xl border p-4 text-left ${selected?.slug === x.slug ? "border-[var(--brand)] bg-white" : "border-[var(--line)] bg-[var(--surface)]"}`}
            >
              <span className="text-sm font-semibold">{x.role}</span>
              <span className="mt-1 block text-xs text-[var(--muted)]">
                v{x.version} · {x.expected_minutes} min
              </span>
            </button>
          ))}
          {!items.length && !editing && (
            <p className="rounded-xl border border-dashed border-[var(--line)] p-5 text-sm text-[var(--muted)]">
              No learning pathway has been published.
            </p>
          )}
        </aside>
        {selected && (
          <Pathway
            pathway={selected}
            repositoryId={repositoryId}
            token={token}
            onError={setError}
          />
        )}
      </div>
    </div>
  );
}
function Pathway({
  pathway: p,
  repositoryId,
  token,
  onError,
}: {
  pathway: LearningPathway;
  repositoryId: string;
  token: string | null;
  onError: (v: string) => void;
}) {
  const [attempt, setAttempt] = useState<{
    id: string;
    state: string;
    commit_id: string;
    learning_context: {
      instructions: string;
      acceptance_criteria: string[];
      starter_commands: string[];
      reproducibility_sha256: string;
    };
  } | null>(null);
  const [launching, setLaunching] = useState("");
  const launchRequests = useRef(new Map<string, string>());
  async function launch(moduleId: string, exerciseId: string) {
    setLaunching(exerciseId);
    try {
      const key = `${p.slug}:${p.version}:${moduleId}:${exerciseId}`;
      const requestId = launchRequests.current.get(key) ?? crypto.randomUUID();
      launchRequests.current.set(key, requestId);
      const value = await api<typeof attempt>(
        `/repositories/${repositoryId}/learning-pathways/${p.slug}/modules/${moduleId}/attempts`,
        {
          method: "POST",
          body: JSON.stringify({
            pathway_version: p.version,
            exercise_id: exerciseId,
            request_id: requestId,
          }),
        },
        token,
      );
      setAttempt(value);
      onError("");
    } catch (e) {
      onError(
        e instanceof Error ? e.message : "Exercise could not be launched",
      );
    } finally {
      setLaunching("");
    }
  }
  return (
    <main className="space-y-5">
      <section className="rounded-xl border border-[var(--line)] bg-white p-6">
        <div className="flex flex-wrap justify-between gap-3">
          <div>
            <p className="text-xs text-[var(--muted)]">
              {p.slug} · revision {p.version}
            </p>
            <h2 className="mt-1 text-2xl font-semibold">{p.outcome}</h2>
          </div>
          <span className="text-sm font-semibold">
            {p.expected_minutes} minutes
          </span>
        </div>
        <div className="mt-5 grid gap-4 sm:grid-cols-3">
          <Summary title="Prerequisites" values={p.prerequisites} />
          <Summary title="Objectives" values={p.objectives} />
          <Summary title="Completion evidence" values={p.completion_evidence} />
        </div>
        <p className="mt-4 text-xs text-[var(--muted)]">
          Supported revisions: {p.supported_revisions.join(", ")} · Locales:{" "}
          {p.locales.join(", ")}
        </p>
      </section>
      {attempt && (
        <section className="rounded-xl border border-emerald-200 bg-emerald-50 p-5">
          <p className="text-xs font-semibold uppercase tracking-wide text-emerald-800">
            Bounded practice workspace · {attempt.state}
          </p>
          <h3 className="mt-1 font-semibold">
            Exact revision {attempt.commit_id.slice(0, 12)}
          </h3>
          <p className="mt-2 text-sm">
            {attempt.learning_context.instructions}
          </p>
          <p className="mt-3 text-xs">
            Acceptance:{" "}
            {attempt.learning_context.acceptance_criteria.join(" · ")}
          </p>
          <p className="mt-2 font-mono text-[10px] text-emerald-900">
            Reproduce {attempt.learning_context.reproducibility_sha256}
          </p>
          <a
            href={`/workspaces/${attempt.id}`}
            className="mt-3 inline-block text-sm font-semibold text-[var(--brand)]"
          >
            Open workspace →
          </a>
        </section>
      )}
      {p.modules.map((m, index) => (
        <section
          key={m.id}
          className="rounded-xl border border-[var(--line)] bg-white p-6"
        >
          <p className="text-xs font-semibold text-[var(--brand-strong)]">
            Module {index + 1} · {m.estimated_minutes} min
          </p>
          <h3 className="mt-1 text-xl font-semibold">{m.title}</h3>
          <p className="mt-2 text-sm text-[var(--muted)]">
            Why it matters: {m.why_it_matters}
          </p>
          <div className="mt-4 space-y-2">
            {m.materials.map((x, i) => (
              <div
                key={i}
                className="rounded-lg border border-[var(--line)] p-3"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-semibold">{x.label}</span>
                  <span
                    className={`rounded-full border px-2 py-0.5 text-[10px] font-semibold ${badge(x.status)}`}
                  >
                    {x.status?.replace("_", " ")}
                  </span>
                  <span className="text-xs text-[var(--muted)]">{x.kind}</span>
                </div>
                {x.status_detail && (
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    {x.status_detail}
                  </p>
                )}
              </div>
            ))}
          </div>
          {m.exercises.map((x, i) => (
            <div key={i} className="mt-4 rounded-lg bg-[var(--canvas)] p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="text-sm font-semibold">Exercise · {x.title}</p>
                  <p className="mt-1 text-sm text-[var(--muted)]">
                    {x.instructions}
                  </p>
                </div>
                {token && x.id && x.revision && (
                  <button
                    disabled={launching === x.id}
                    onClick={() => void launch(m.id, x.id!)}
                    className="rounded-lg bg-[var(--brand)] px-3 py-2 text-xs font-semibold text-white disabled:opacity-50"
                  >
                    {launching === x.id ? "Preparing…" : "Launch practice"}
                  </button>
                )}
              </div>
              {x.kind && (
                <p className="mt-2 text-xs">
                  {x.kind} · exact revision {x.revision?.slice(0, 12)}
                </p>
              )}
              <p className="mt-2 text-xs">
                Evidence: {x.completion_evidence.join(", ")}
              </p>
            </div>
          ))}
        </section>
      ))}
      <section className="grid gap-4 md:grid-cols-2">
        <div className="rounded-xl border border-[var(--line)] bg-white p-5">
          <h3 className="font-semibold">Mentors</h3>
          {p.mentors.map((x) => (
            <p key={x.user_id} className="mt-3 text-sm">
              {x.responsibility}{" "}
              <span
                className={`ml-2 rounded-full border px-2 py-0.5 text-[10px] ${badge(x.status)}`}
              >
                {x.status}
              </span>
            </p>
          ))}
        </div>
        <div className="rounded-xl border border-[var(--line)] bg-white p-5">
          <h3 className="font-semibold">Learner environments</h3>
          {p.environments.map((x) => (
            <p key={x.name} className="mt-3 text-sm">
              {x.name}{" "}
              <span
                className={`ml-2 rounded-full border px-2 py-0.5 text-[10px] ${badge(x.status)}`}
              >
                {x.status?.replace("_", " ")}
              </span>
            </p>
          ))}
        </div>
      </section>
      <AssessmentPanel pathway={p} repositoryId={repositoryId} token={token} onError={onError} />
    </main>
  );
}
function AssessmentPanel({ pathway, repositoryId, token, onError }: { pathway: LearningPathway; repositoryId: string; token: string | null; onError: (v: string) => void }) {
  const { user } = useAuth();
  const [assessment, setAssessment] = useState<LearningAssessment | null>(null);
  const [attempts, setAttempts] = useState<LearningAssessmentAttempt[]>([]);
  const [owner, setOwner] = useState(false);
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({ slug: `${pathway.slug}-practical`, title: "Practical project assessment", instructions: "Complete the project task in your bounded workspace and cite your evidence.", revision: pathway.supported_revisions[0] ?? "", criteria: '[{"id":"implementation","label":"Implementation","description":"The work satisfies the public project requirement.","weight":1,"required":true}]', protected: "[]", checks: "", retries: "2", cooldown: "0", accommodations: "Extra time" });
  const load = useCallback(async () => {
    try {
      const [list, repository] = await Promise.all([api<{ assessments: LearningAssessment[] }>(`/repositories/${repositoryId}/learning-assessments`, {}, token), api<Repository>(`/repositories/${repositoryId}`, {}, token)]);
      setOwner(repository.owner_id === user?.id);
      const found = list.assessments.find((x) => x.pathway_slug === pathway.slug && x.pathway_version === pathway.version) ?? null;
      setAssessment(found);
      if (found) {
        const detail = await api<{ assessment: LearningAssessment; attempts: LearningAssessmentAttempt[] }>(`/repositories/${repositoryId}/learning-assessments/${found.slug}`, {}, token);
        setAssessment(detail.assessment); setAttempts(detail.attempts);
      } else setAttempts([]);
    } catch (e) { onError(e instanceof Error ? e.message : "Could not load practical assessments"); }
  }, [repositoryId, token, pathway.slug, pathway.version, onError, user]);
  useEffect(() => { void Promise.resolve().then(load); }, [load]);
  function toggleEditor() {
    if (editing) { setEditing(false); return; }
    if (assessment) setForm({ slug: assessment.slug, title: assessment.title, instructions: assessment.instructions, revision: assessment.project_revision, criteria: JSON.stringify(assessment.criteria, null, 2), protected: JSON.stringify(assessment.protected_cases ?? [], null, 2), checks: assessment.required_checks.join("\n"), retries: String(assessment.retry_policy.maximum_attempts), cooldown: String(assessment.retry_policy.cooldown_hours), accommodations: assessment.accommodation_options.join("\n") });
    setEditing(true);
  }
  async function publish() {
    try {
      const value = await api<LearningAssessment>(`/repositories/${repositoryId}/learning-assessments/${form.slug}`, { method: "PUT", body: JSON.stringify({ request_id: crypto.randomUUID(), expected_version: assessment?.slug === form.slug ? assessment.version : 0, assessment: { pathway_slug: pathway.slug, pathway_version: pathway.version, project_revision: form.revision, title: form.title, instructions: form.instructions, criteria: JSON.parse(form.criteria), protected_cases: JSON.parse(form.protected), required_checks: lines(form.checks), retry_policy: { maximum_attempts: Number(form.retries), cooldown_hours: Number(form.cooldown) }, accommodation_options: lines(form.accommodations) } }) }, token);
      setAssessment(value); setEditing(false); onError(""); await load();
    } catch (e) { onError(e instanceof Error ? e.message : "Could not publish assessment"); }
  }
  return <section className="rounded-xl border border-[var(--line)] bg-white p-6">
    <div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-xs font-semibold uppercase tracking-wide text-[var(--brand-strong)]">Demonstrated competence</p><h3 className="mt-1 text-xl font-semibold">Practical assessment</h3><p className="mt-1 text-sm text-[var(--muted)]">Public rubric decisions and reproducible evidence stay visible; protected cases remain concealed.</p></div>{owner && <button onClick={toggleEditor} className="rounded-lg bg-[var(--brand)] px-3 py-2 text-xs font-semibold text-white">{editing ? "Cancel" : assessment ? "Revise assessment" : "Define assessment"}</button>}</div>
    {editing && <div className="mt-5 grid gap-3 md:grid-cols-2">{([ ["slug","Stable assessment slug"], ["title","Title"], ["instructions","Learner instructions"], ["revision","Exact project revision"], ["checks","Required repository checks"], ["retries","Maximum attempts"], ["cooldown","Retry cooldown hours"], ["accommodations","Permitted accommodations"], ["criteria","Public criteria JSON"], ["protected","Protected cases JSON"] ] as const).map(([key,label]) => <label key={key} className="text-xs font-semibold text-[var(--muted)]">{label}<textarea value={form[key]} onChange={(e)=>setForm({...form,[key]:e.target.value})} className="mt-1 min-h-16 w-full rounded-lg border border-[var(--line)] p-2 text-sm text-[var(--ink)]" /></label>)}<button onClick={() => void publish()} className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-semibold text-white">Publish immutable assessment</button></div>}
    {assessment && <div className="mt-5"><div className="flex flex-wrap gap-2 text-xs"><span className="rounded-full border px-2 py-1">revision {assessment.version}</span><span className="rounded-full border px-2 py-1">project {assessment.project_revision.slice(0,12)}</span><span className="rounded-full border px-2 py-1">{assessment.retry_policy.maximum_attempts} attempts</span></div><p className="mt-3 text-sm">{assessment.instructions}</p><div className="mt-4 grid gap-3 md:grid-cols-2">{assessment.criteria.map((c)=><div key={c.id} className="rounded-lg bg-[var(--canvas)] p-3"><p className="text-sm font-semibold">{c.label}</p><p className="mt-1 text-xs text-[var(--muted)]">{c.description}</p></div>)}</div>{assessment.protected_cases?.length ? <p className="mt-3 text-xs text-[var(--muted)]">{assessment.protected_cases.length} protected case{assessment.protected_cases.length === 1 ? "" : "s"} · answer material is not shown to learners.</p> : null}<div className="mt-5 space-y-3">{attempts.map((a)=><article key={a.id} className="rounded-lg border border-[var(--line)] p-4"><div className="flex justify-between gap-3"><p className="text-sm font-semibold">Attempt {a.attempt_number} · {a.status.replaceAll("_"," ")}</p><span className="text-xs text-[var(--muted)]">{a.project_revision.slice(0,12)}</span></div>{a.blockers.length>0&&<p className="mt-2 text-xs text-amber-800">Completion blocked: {a.blockers.map(x=>x.replaceAll("_"," ")).join(" · ")}</p>}{a.reviews.map((r,i)=><div key={i} className="mt-3 border-t border-[var(--line)] pt-3 text-sm"><p>{r.feedback}</p>{r.uncertainty&&<p className="mt-1 text-xs text-[var(--muted)]">Uncertainty: {r.uncertainty}</p>}</div>)}{a.accommodation&&<p className="mt-2 text-xs">Accommodation: {a.accommodation}</p>}{a.appeals.length>0&&<p className="mt-2 text-xs text-[var(--brand-strong)]">Appeal recorded and attributable.</p>}</article>)}</div></div>}
  </section>;
}
function Summary({ title, values }: { title: string; values: string[] }) {
  return (
    <div>
      <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">
        {title}
      </h3>
      <ul className="mt-2 space-y-1 text-sm">
        {values.map((x) => (
          <li key={x}>• {x}</li>
        ))}
      </ul>
    </div>
  );
}
