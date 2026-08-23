"use client";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Investigation = {
  id: string;
  version: number;
  title: string;
  expected_behavior: string;
  regressed_behavior: string;
  known_good: { revision: string; label: string };
  known_bad: { revision: string; label: string };
  affected_environments: string[];
  severity: string;
  owner_ids: string[];
  acceptance_criteria: string[];
  comparable: boolean;
  diagnostics: string[];
  status: string;
  evidence: {
    id: string;
    label: string;
    available: boolean;
    stale: boolean;
    diagnostic?: string;
  }[];
  history: {
    id: string;
    kind: string;
    actor_id: string;
    message: string;
    from?: string;
    to?: string;
    created_at: string;
  }[];
  scenarios: Scenario[];
  attempts: Attempt[];
  searches: Search[];
  responses: ResponsePlan[];
};
type ResponsePlan = {
  id: string;
  search_id: string;
  options: { kind: string; summary: string; benefits: string[]; risks: string[]; constraints: string[]; affected_release_ids: string[]; affected_pull_ids: string[]; backport_targets: string[] }[];
  selected_kind?: string;
  rationale?: string;
  proposal_id?: string;
  task_ids: string[];
};
type SearchCandidate = {
  kind: string;
  repository_id: string;
  revision: string;
  parents: string[];
  merge: boolean;
  classification: string;
  attempt_ids: string[];
  excluded: boolean;
  exclusion?: string;
  selected: boolean;
  subject?: string;
  changed_paths: string[];
  owner_ids: string[];
  pull_request_ids: string[];
};
type Search = {
  id: string;
  version: number;
  state: string;
  scenario_id: string;
  candidates: SearchCandidate[];
  culprit_ranges: {
    working_revision: string;
    regressed_revision: string;
    remaining: number;
    confidence: number;
    ambiguity?: string;
  }[];
  hypotheses: {
    id: string;
    actor_id: string;
    claim: string;
    candidate_revisions: string[];
    evidence_ids: string[];
    attempt_ids: string[];
    confidence: string;
  }[];
  decisions: {
    id: string;
    actor_id: string;
    kind: string;
    revision?: string;
    reason: string;
  }[];
};
type ScenarioEnvironment = {
  image: string;
  working_directory: string;
  setup_command?: string;
  command: string;
  timeout_seconds: number;
  cpus: number;
  memory_mb: number;
  storage_mb: number;
};
type Scenario = {
  id: string;
  name: string;
  environment: ScenarioEnvironment;
  acceptance_criteria: string[];
  environment_variants: {
    revision: string;
    environment: ScenarioEnvironment;
  }[];
};
type Attempt = {
  id: string;
  scenario_id: string;
  target_kind: string;
  target_id?: string;
  revision?: string;
  dependencies: {
    name: string;
    repository_id: string;
    revision: string;
    path: string;
    archive_sha256?: string;
  }[];
  classification: string;
  diagnostic?: string;
  cost_compute_seconds: number;
  requested_by: string;
  runs: {
    run_id?: string;
    state: string;
    exit_code?: number;
    failure?: string;
    output?: string;
    logs?: string;
    duration_ms: number;
    artifacts: { path: string; sha256: string; size: number }[];
  }[];
};
const field =
  "rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-2 text-sm";
const value = (f: FormData, n: string) => String(f.get(n) || "").trim();
const list = (v: string) =>
  v
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
const normalized = (investigation: Investigation): Investigation => ({
  ...investigation,
  evidence: investigation.evidence ?? [],
  diagnostics: investigation.diagnostics ?? [],
  history: investigation.history ?? [],
  acceptance_criteria: investigation.acceptance_criteria ?? [],
  affected_environments: investigation.affected_environments ?? [],
  scenarios: investigation.scenarios ?? [],
  attempts: investigation.attempts ?? [],
  searches: investigation.searches ?? [],
  responses: investigation.responses ?? [],
});
export function RegressionInvestigationsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token } = useAuth();
  const [items, setItems] = useState<Investigation[]>([]);
  const [selected, setSelected] = useState<Investigation>();
  const [error, setError] = useState("");
  const createRequestID = useRef(crypto.randomUUID());
  const attemptRequestIDs = useRef<Record<string, string>>({});
  const searchRequestID = useRef(crypto.randomUUID());
  const responseRequestID = useRef(crypto.randomUUID());
  const load = useCallback(async () => {
    try {
      const x = await api<{ regression_investigations: Investigation[] }>(
        `/repositories/${repositoryID}/regression-investigations`,
        {},
        token,
      );
      const investigations = x.regression_investigations.map(normalized);
      setItems(investigations);
      setSelected(
        (s) => investigations.find((v) => v.id === s?.id) ?? investigations[0],
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Investigations could not be loaded",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    // Repository and identity changes must refresh this API-backed workspace list.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load();
  }, [load]);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    const f = new FormData(e.currentTarget),
      good = value(f, "good"),
      bad = value(f, "bad"),
      sourceKind = value(f, "source_kind"),
      evidenceID = value(f, "evidence_id");
    try {
      const out = await api<Investigation>(
        `/repositories/${repositoryID}/regression-investigations`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: createRequestID.current,
            title: value(f, "title"),
            source: {
              kind: sourceKind,
              resource_id: value(f, "source_id"),
              revision: value(f, "source_revision"),
              label: value(f, "source_label"),
            },
            expected_behavior: value(f, "expected"),
            regressed_behavior: value(f, "regressed"),
            known_good: {
              kind: "commit",
              revision: good,
              label: value(f, "good_label") || good.slice(0, 12),
            },
            known_bad: {
              kind: "commit",
              revision: bad,
              label: value(f, "bad_label") || bad.slice(0, 12),
            },
            affected_environments: list(value(f, "environments")),
            severity: value(f, "severity"),
            owner_ids: list(value(f, "owners")),
            acceptance_criteria: list(value(f, "criteria")),
            evidence: evidenceID
              ? [
                  {
                    kind: value(f, "evidence_kind"),
                    resource_id: evidenceID,
                    revision: value(f, "evidence_revision"),
                    label: value(f, "evidence_label"),
                    visibility: "repository",
                  },
                ]
              : [],
          }),
        },
        token,
      );
      setItems((v) => (v.some((x) => x.id === out.id) ? v : [out, ...v]));
      setSelected(out);
      e.currentTarget.reset();
      createRequestID.current = crypto.randomUUID();
      setError("");
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Investigation could not be opened",
      );
    }
  }
  async function append(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      const out = await api<Investigation>(
        `/repositories/${repositoryID}/regression-investigations/${selected.id}/events`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            kind: value(f, "kind"),
            message: value(f, "message"),
            value: value(f, "event_value"),
          }),
        },
        token,
      );
      setSelected(out);
      setItems((v) => v.map((x) => (x.id === out.id ? out : x)));
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Update failed");
    }
  }
  function replace(out: Investigation) {
    const next = normalized(out);
    setSelected(next);
    setItems((v) => v.map((x) => (x.id === next.id ? next : x)));
  }
  async function defineScenario(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      const variantText = value(f, "environment_variants");
      const out = await api<Investigation>(
        `/repositories/${repositoryID}/regression-investigations/${selected.id}/scenarios`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            scenario: {
              name: value(f, "scenario_name"),
              environment: {
                image: value(f, "image"),
                working_directory: value(f, "working_directory") || ".",
                setup_command: value(f, "setup_command"),
                command: value(f, "command"),
                timeout_seconds: Number(value(f, "timeout")),
                cpus: Number(value(f, "cpus")),
                memory_mb: Number(value(f, "memory")),
                storage_mb: Number(value(f, "storage")),
              },
              environment_variants: variantText ? JSON.parse(variantText) : [],
              inputs: [],
              acceptance_criteria: list(value(f, "scenario_criteria")),
            },
          }),
        },
        token,
      );
      replace(out);
      e.currentTarget.reset();
      setError("");
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Scenario could not be defined",
      );
    }
  }
  async function runScenario(
    e: FormEvent<HTMLFormElement>,
    scenario: Scenario,
  ) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget),
      dependencies: {
        name: string;
        repository_id: string;
        revision: string;
      }[] = [];
    for (const item of list(value(f, "dependencies"))) {
      const [name, repository_id, revision] = item.split("@");
      if (name && repository_id && revision)
        dependencies.push({ name, repository_id, revision });
    }
    const requestID =
      attemptRequestIDs.current[scenario.id] ?? crypto.randomUUID();
    attemptRequestIDs.current[scenario.id] = requestID;
    try {
      const out = await api<Investigation>(
        `/repositories/${repositoryID}/regression-investigations/${selected.id}/scenarios/${scenario.id}/attempts`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            request_id: requestID,
            target_kind: value(f, "target_kind"),
            target_id: value(f, "target_id"),
            revision: value(f, "revision"),
            dependencies,
            repeats: Number(value(f, "repeats")),
          }),
        },
        token,
      );
      replace(out);
      delete attemptRequestIDs.current[scenario.id];
      setError("");
    } catch (x) {
      setError(x instanceof Error ? x.message : "Historical attempt failed");
    }
  }
  async function scheduleSearch(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      const out = await api<Investigation>(
        `/repositories/${repositoryID}/regression-investigations/${selected.id}/searches`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            request_id: searchRequestID.current,
            scenario_id: value(f, "search_scenario"),
          }),
        },
        token,
      );
      replace(out);
      searchRequestID.current = crypto.randomUUID();
      setError("");
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Search could not be scheduled",
      );
    }
  }
  async function guideSearch(
    e: FormEvent<HTMLFormElement>,
    search: Search,
    revision?: string,
  ) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget),
      kind = value(f, "kind");
    try {
      const out = await api<Investigation>(
        `/repositories/${repositoryID}/regression-investigations/${selected.id}/searches/${search.id}/guidance`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            expected_search_version: search.version,
            kind,
            revision: revision || value(f, "revision"),
            classification: value(f, "classification"),
            reason: value(f, "reason"),
            claim: value(f, "claim"),
            confidence: value(f, "confidence"),
            evidence_ids: list(value(f, "evidence_ids")),
            attempt_ids: list(value(f, "attempt_ids")),
            candidate_revisions: list(value(f, "candidate_revisions")),
          }),
        },
        token,
      );
      replace(out);
      setError("");
    } catch (x) {
      setError(x instanceof Error ? x.message : "Search guidance failed");
    }
  }
  async function compareResponses(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); if (!token || !selected) return;
    const f=new FormData(e.currentTarget), search=selected.searches.find((x)=>x.id===value(f,"response_search")); if(!search)return;
    const shared={benefits:list(value(f,"benefits")),risks:list(value(f,"risks")),constraints:list(value(f,"constraints")),affected_release_ids:list(value(f,"releases")),affected_pull_ids:list(value(f,"pulls")),backport_targets:list(value(f,"backports"))};
    try { const out=await api<Investigation>(`/repositories/${repositoryID}/regression-investigations/${selected.id}/responses`,{method:"POST",body:JSON.stringify({expected_version:selected.version,request_id:responseRequestID.current,search_id:search.id,scenario_id:search.scenario_id,options:["revert","containment","dependency_adjustment","forward_repair"].map((kind)=>({...shared,kind,summary:value(f,kind)}))})},token); replace(out);responseRequestID.current=crypto.randomUUID();e.currentTarget.reset();setError(""); } catch(x){setError(x instanceof Error?x.message:"Correction options could not be compared")}
  }
  async function publishResponse(e: FormEvent<HTMLFormElement>, response: ResponsePlan) {
    e.preventDefault();if(!token||!selected)return;const f=new FormData(e.currentTarget);
    try{const out=await api<Investigation>(`/repositories/${repositoryID}/regression-investigations/${selected.id}/responses/${response.id}/publish`,{method:"POST",body:JSON.stringify({expected_version:selected.version,selected_kind:value(f,"selected_kind"),rationale:value(f,"rationale"),title:value(f,"proposal_title"),work:[{title:value(f,"task_title"),outcome:value(f,"outcome"),assignee_type:value(f,"assignee_type"),assignee_id:value(f,"assignee_id"),acceptance_criteria:list(value(f,"work_criteria"))}]})},token);replace(out);setError("")}catch(x){setError(x instanceof Error?x.message:"Correction work could not be published")}
  }
  return (
    <main className="mx-auto max-w-7xl space-y-6 p-6">
      <div>
        <p className="text-xs font-semibold uppercase tracking-widest text-[var(--brand)]">
          Shared search boundary
        </p>
        <h1 className="text-2xl font-semibold">Regression investigations</h1>
        <p className="mt-1 text-sm text-[var(--muted)]">
          Agree on what changed and which history is comparable before testing
          commits.
        </p>
      </div>
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      <div className="grid gap-6 lg:grid-cols-[.8fr_1.2fr]">
        <div className="space-y-4">
          <Card className="p-5">
            <h2 className="font-semibold">Open an investigation</h2>
            <form onSubmit={create} className="mt-4 grid gap-3">
              <input
                className={field}
                name="title"
                required
                placeholder="Checkout fails after rollout"
              />
              <div className="grid grid-cols-2 gap-2">
                <select
                  className={field}
                  name="source_kind"
                  defaultValue="issue"
                >
                  <option value="issue">Issue</option>
                  <option value="support_thread">Support thread</option>
                  <option value="failed_check">Failed check</option>
                  <option value="release">Release</option>
                  <option value="deployment">Deployment</option>
                  <option value="reproduction">Reproduction</option>
                </select>
                <input
                  className={field}
                  name="source_id"
                  required
                  placeholder="Source ID"
                />
              </div>
              <input
                className={field}
                name="source_revision"
                placeholder="Source revision (when applicable)"
              />
              <input
                className={field}
                name="source_label"
                required
                placeholder="Source label"
              />
              <textarea
                className={field}
                name="expected"
                required
                placeholder="Expected behavior"
              />
              <textarea
                className={field}
                name="regressed"
                required
                placeholder="Regressed behavior"
              />
              <input
                className={field}
                name="good"
                required
                pattern="[0-9a-f]{40}"
                placeholder="Known-good commit"
              />
              <input
                className={field}
                name="bad"
                required
                pattern="[0-9a-f]{40}"
                placeholder="Known-bad commit"
              />
              <input
                className={field}
                name="environments"
                required
                placeholder="Affected environments, comma separated"
              />
              <select className={field} name="severity" defaultValue="high">
                <option>low</option>
                <option>medium</option>
                <option>high</option>
                <option>critical</option>
              </select>
              <input
                className={field}
                name="owners"
                required
                placeholder="Owner IDs, comma separated"
              />
              <input
                className={field}
                name="criteria"
                required
                placeholder="Acceptance criteria, comma separated"
              />
              <fieldset className="grid gap-2 rounded-lg border border-[var(--border)] p-3">
                <legend className="px-1 text-xs font-semibold">
                  Permitted evidence (optional)
                </legend>
                <select
                  className={field}
                  name="evidence_kind"
                  defaultValue="issue"
                >
                  <option value="issue">Issue</option>
                  <option value="support_thread">Support thread</option>
                  <option value="failed_check">Failed check</option>
                  <option value="release">Release</option>
                  <option value="deployment">Deployment</option>
                  <option value="reproduction">Reproduction</option>
                  <option value="commit">Commit</option>
                </select>
                <input
                  className={field}
                  name="evidence_id"
                  placeholder="Evidence resource ID"
                />
                <input
                  className={field}
                  name="evidence_revision"
                  placeholder="Evidence revision"
                />
                <input
                  className={field}
                  name="evidence_label"
                  placeholder="Evidence label"
                />
              </fieldset>
              <Button type="submit">Open durable boundary</Button>
            </form>
          </Card>
          <Card className="divide-y divide-[var(--border)]">
            {items.map((x) => (
              <button
                key={x.id}
                onClick={() => setSelected(x)}
                className="block w-full p-4 text-left"
              >
                <span className="font-medium">{x.title}</span>
                <span className="mt-1 flex gap-2">
                  <Badge>{x.severity}</Badge>
                  <Badge tone={x.comparable ? "success" : "danger"}>
                    {x.comparable ? "comparable" : "blocked"}
                  </Badge>
                </span>
              </button>
            ))}
          </Card>
        </div>
        {selected ? (
          <div className="space-y-4">
            <Card className="p-5">
              <div className="flex justify-between">
                <h2 className="text-lg font-semibold">{selected.title}</h2>
                <Badge>{selected.status}</Badge>
              </div>
              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                <div>
                  <p className="text-xs text-[var(--muted)]">Known good</p>
                  <code>{selected.known_good.revision.slice(0, 12)}</code>
                </div>
                <div>
                  <p className="text-xs text-[var(--muted)]">Known bad</p>
                  <code>{selected.known_bad.revision.slice(0, 12)}</code>
                </div>
              </div>
              <h3 className="mt-5 font-medium">Expected</h3>
              <p className="text-sm">{selected.expected_behavior}</p>
              <h3 className="mt-4 font-medium">Regressed</h3>
              <p className="text-sm">{selected.regressed_behavior}</p>
              <p className="mt-4 text-sm">
                <b>Environments:</b> {selected.affected_environments.join(", ")}
              </p>
              <h3 className="mt-4 font-medium">Acceptance criteria</h3>
              <ul className="list-disc pl-5 text-sm">
                {selected.acceptance_criteria.map((x) => (
                  <li key={x}>{x}</li>
                ))}
              </ul>
              <h3 className="mt-4 font-medium">Evidence</h3>
              {selected.evidence.length === 0 ? (
                <p className="text-sm text-[var(--muted)]">
                  No evidence attached.
                </p>
              ) : (
                <ul className="mt-2 space-y-2">
                  {selected.evidence.map((evidence) => (
                    <li
                      key={evidence.id}
                      className="rounded-lg border border-[var(--border)] p-3 text-sm"
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="font-medium">{evidence.label}</span>
                        <Badge
                          tone={
                            evidence.stale
                              ? "warning"
                              : evidence.available
                                ? "success"
                                : "danger"
                          }
                        >
                          {evidence.stale
                            ? "stale"
                            : evidence.available
                              ? "available"
                              : "unavailable"}
                        </Badge>
                      </div>
                      {evidence.diagnostic && (
                        <p className="mt-1 text-[var(--muted)]">
                          {evidence.diagnostic}
                        </p>
                      )}
                    </li>
                  ))}
                </ul>
              )}
              {selected.diagnostics.map((x) => (
                <p key={x} className="mt-2 text-sm text-[var(--warning)]">
                  {x}
                </p>
              ))}
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Governed regression response</h2>
              <p className="mt-1 text-sm text-[var(--muted)]">Compare containment and durable corrections against the supported culprit range before creating ordinary owned work.</p>
              <form onSubmit={compareResponses} className="mt-4 grid gap-2 sm:grid-cols-2">
                <select className={field} name="response_search" required><option value="">Evidence search…</option>{selected.searches.filter((x)=>x.culprit_ranges.length>0).map((x)=><option key={x.id} value={x.id}>{x.id.slice(0,8)} · {x.culprit_ranges.length} range(s)</option>)}</select>
                <input className={field} name="releases" placeholder="Affected release IDs, comma separated" />
                <textarea className={field} name="revert" required placeholder="Evidence-backed revert: what valid intent would be lost?" />
                <textarea className={field} name="containment" required placeholder="Configuration or rollout containment" />
                <textarea className={field} name="dependency_adjustment" required placeholder="Dependency adjustment" />
                <textarea className={field} name="forward_repair" required placeholder="Forward repair preserving original intent" />
                <input className={field} name="pulls" placeholder="Affected current pull IDs" />
                <input className={field} name="backports" placeholder="Backport targets" />
                <input className={field} name="benefits" required placeholder="Benefits, comma separated" />
                <input className={field} name="risks" required placeholder="Tradeoffs and risks, comma separated" />
                <input className={`${field} sm:col-span-2`} name="constraints" required placeholder="Repository, release, rollout, and environment constraints" />
                <Button type="submit" className="sm:col-span-2">Freeze four-way comparison</Button>
              </form>
              <div className="mt-5 space-y-4">{selected.responses.map((response)=><section key={response.id} className="rounded-lg border border-[var(--border)] p-4">
                <div className="grid gap-2 sm:grid-cols-2">{response.options.map((option)=><div key={option.kind} className="rounded-md bg-[var(--surface-muted)] p-3 text-sm"><b>{option.kind.replaceAll("_"," ")}</b><p>{option.summary}</p><p className="mt-1 text-[var(--muted)]">Risk: {option.risks.join("; ")}</p></div>)}</div>
                {response.proposal_id ? <p className="mt-3 text-sm"><Badge tone="success">published</Badge> Proposal {response.proposal_id} · tasks {response.task_ids.join(", ")}</p> : <form onSubmit={(e)=>publishResponse(e,response)} className="mt-3 grid gap-2 sm:grid-cols-2">
                  <select className={field} name="selected_kind">{response.options.map((o)=><option key={o.kind} value={o.kind}>{o.kind.replaceAll("_"," ")}</option>)}</select>
                  <input className={field} name="proposal_title" required placeholder="Correction proposal title" />
                  <textarea className={`${field} sm:col-span-2`} name="rationale" required placeholder="Why this choice best limits harm while preserving valid intent" />
                  <input className={field} name="task_title" required placeholder="Owned correction task" /><input className={field} name="outcome" required placeholder="Required outcome" />
                  <select className={field} name="assignee_type"><option value="human">Human owner</option><option value="agent">Task-scoped agent</option></select><input className={field} name="assignee_id" required placeholder="Assignee ID" />
                  <input className={`${field} sm:col-span-2`} name="work_criteria" required placeholder="Acceptance criteria, comma separated" /><Button type="submit" className="sm:col-span-2">Create ordinary governed work</Button>
                </form>}
              </section>)}</div>
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Repeatable comparison scenarios</h2>
              <p className="mt-1 text-sm text-[var(--muted)]">
                Freeze a preinstalled image, setup, command, resources, and
                criteria before selecting historical revisions.
              </p>
              <form
                onSubmit={defineScenario}
                className="mt-4 grid gap-2 sm:grid-cols-2"
              >
                <input
                  className={field}
                  name="scenario_name"
                  required
                  placeholder="Checkout historical behavior"
                />
                <input
                  className={field}
                  name="image"
                  required
                  placeholder="alpine:3.22"
                />
                <input
                  className={field}
                  name="working_directory"
                  defaultValue="."
                  placeholder="Working directory"
                />
                <input
                  className={field}
                  name="setup_command"
                  placeholder="Revision-specific setup command"
                />
                <textarea
                  className={`${field} sm:col-span-2`}
                  name="command"
                  required
                  placeholder="Exact comparison command"
                />
                <textarea
                  className={`${field} sm:col-span-2`}
                  name="environment_variants"
                  placeholder='Optional revision-specific environments as JSON: [{"revision":"…","environment":{…}}]'
                />
                <input
                  className={field}
                  name="scenario_criteria"
                  required
                  placeholder="Criteria, comma separated"
                />
                <div className="grid grid-cols-4 gap-2">
                  <input
                    className={field}
                    name="timeout"
                    type="number"
                    defaultValue="120"
                    aria-label="Timeout seconds"
                  />
                  <input
                    className={field}
                    name="cpus"
                    type="number"
                    step="0.25"
                    defaultValue="1"
                    aria-label="CPUs"
                  />
                  <input
                    className={field}
                    name="memory"
                    type="number"
                    defaultValue="512"
                    aria-label="Memory MB"
                  />
                  <input
                    className={field}
                    name="storage"
                    type="number"
                    defaultValue="64"
                    aria-label="Storage MB"
                  />
                </div>
                <Button type="submit" className="sm:col-span-2">
                  Define scenario
                </Button>
              </form>
              <div className="mt-5 space-y-4">
                {selected.scenarios.map((scenario) => (
                  <section
                    key={scenario.id}
                    className="rounded-lg border border-[var(--border)] p-4"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <h3 className="font-medium">{scenario.name}</h3>
                      <code className="text-xs">
                        {scenario.environment.image}
                      </code>
                    </div>
                    <code className="mt-2 block overflow-x-auto text-xs">
                      {scenario.environment.setup_command
                        ? `${scenario.environment.setup_command} && `
                        : ""}
                      {scenario.environment.command}
                    </code>
                    <form
                      onSubmit={(e) => runScenario(e, scenario)}
                      className="mt-3 grid gap-2 sm:grid-cols-2"
                    >
                      <select
                        className={field}
                        name="target_kind"
                        defaultValue="commit"
                      >
                        <option value="commit">Commit</option>
                        <option value="release">Attested release</option>
                      </select>
                      <input
                        className={field}
                        name="target_id"
                        placeholder="Release ID (for release targets)"
                      />
                      <input
                        className={field}
                        name="revision"
                        pattern="[0-9a-f]{40}"
                        placeholder="Exact commit (optional for release)"
                      />
                      <input
                        className={field}
                        name="dependencies"
                        placeholder="Name@RepositoryID@40-char revision, …"
                      />
                      <input
                        className={field}
                        name="repeats"
                        type="number"
                        min="1"
                        max="5"
                        defaultValue="2"
                        aria-label="Repeat count"
                      />
                      <Button type="submit">Run isolated comparison</Button>
                    </form>
                    <div className="mt-3 space-y-2">
                      {selected.attempts
                        .filter(
                          (attempt) => attempt.scenario_id === scenario.id,
                        )
                        .map((attempt) => (
                          <details
                            key={attempt.id}
                            className="rounded-md bg-[var(--surface-muted)] p-3 text-sm"
                          >
                            <summary className="cursor-pointer">
                              <Badge
                                tone={
                                  attempt.classification === "passed"
                                    ? "success"
                                    : attempt.classification === "flaky"
                                      ? "warning"
                                      : "danger"
                                }
                              >
                                {attempt.classification.replaceAll("_", " ")}
                              </Badge>{" "}
                              <code className="ml-2">
                                {attempt.revision?.slice(0, 12) ||
                                  attempt.target_id}
                              </code>{" "}
                              · {attempt.runs.length} run(s) ·{" "}
                              {attempt.cost_compute_seconds.toFixed(2)}{" "}
                              compute-seconds
                            </summary>
                            {attempt.diagnostic && (
                              <p className="mt-2 text-[var(--muted)]">
                                {attempt.diagnostic}
                              </p>
                            )}
                            {attempt.dependencies.length > 0 && (
                              <p className="mt-2">
                                <b>Dependencies:</b>{" "}
                                {attempt.dependencies
                                  .map(
                                    (dependency) =>
                                      `${dependency.name}@${dependency.repository_id}:${dependency.revision.slice(0, 12)} → ${dependency.path}`,
                                  )
                                  .join(", ")}
                              </p>
                            )}
                            {attempt.runs.map((run, index) => (
                              <div
                                key={run.run_id || index}
                                className="mt-3 border-t border-[var(--border)] pt-2"
                              >
                                <b>Run {index + 1}:</b> {run.state} (
                                {run.duration_ms} ms)
                                {run.failure && <p>{run.failure}</p>}
                                {run.logs && (
                                  <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap text-xs">
                                    {run.logs}
                                  </pre>
                                )}
                                {run.artifacts.length > 0 && (
                                  <p>
                                    {run.artifacts.length} digest-addressed
                                    artifact(s)
                                  </p>
                                )}
                              </div>
                            ))}
                          </details>
                        ))}
                    </div>
                  </section>
                ))}
              </div>
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Evidence-driven graph search</h2>
              {selected.searches.length === 0 ? (
                <form onSubmit={scheduleSearch} className="mt-3 flex gap-2">
                  <select
                    className={field}
                    name="search_scenario"
                    required
                    defaultValue=""
                  >
                    <option value="" disabled>
                      Select a frozen scenario
                    </option>
                    {selected.scenarios.map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name}
                      </option>
                    ))}
                  </select>
                  <Button type="submit">Schedule search</Button>
                </form>
              ) : (
                selected.searches.map((search) => (
                  <section key={search.id} className="mt-4 space-y-4">
                    <div className="flex items-center justify-between">
                      <div>
                        <Badge
                          tone={
                            search.state === "isolated" ? "success" : "warning"
                          }
                        >
                          {search.state}
                        </Badge>
                        <span className="ml-2 text-sm text-[var(--muted)]">
                          {
                            search.candidates.filter(
                              (c) => !c.excluded && c.classification === "",
                            ).length
                          }{" "}
                          candidates remain
                        </span>
                      </div>
                      <span className="text-sm">
                        {search.culprit_ranges.length} competing range(s)
                      </span>
                    </div>
                    <div className="space-y-2">
                      {search.culprit_ranges.map((range, i) => (
                        <div
                          key={`${range.working_revision}-${range.regressed_revision}-${i}`}
                          className="rounded-lg border border-[var(--border)] p-3 text-sm"
                        >
                          <code>
                            {range.working_revision.slice(0, 8)} →{" "}
                            {range.regressed_revision.slice(0, 8)}
                          </code>
                          <span className="ml-2">
                            {Math.round(range.confidence * 100)}% confidence ·{" "}
                            {range.remaining} untested
                          </span>
                          {range.ambiguity && (
                            <p className="mt-1 text-[var(--warning)]">
                              {range.ambiguity}
                            </p>
                          )}
                        </div>
                      ))}
                    </div>
                    <div className="space-y-2">
                      {search.candidates.map((candidate) => (
                        <details
                          key={`${candidate.repository_id}-${candidate.revision}`}
                          open={candidate.selected}
                          className={`rounded-lg border p-3 ${candidate.selected ? "border-[var(--brand)]" : "border-[var(--border)]"}`}
                        >
                          <summary className="cursor-pointer text-sm">
                            <Badge
                              tone={
                                candidate.classification === "working"
                                  ? "success"
                                  : candidate.classification === "regressed"
                                    ? "danger"
                                    : candidate.excluded ||
                                        candidate.classification === "flaky"
                                      ? "warning"
                                      : undefined
                              }
                            >
                              {candidate.excluded
                                ? "excluded"
                                : candidate.classification || "untested"}
                            </Badge>{" "}
                            <code className="ml-2">
                              {candidate.revision.slice(0, 12)}
                            </code>
                            {candidate.merge && " · merge"}
                            {candidate.selected && " · recommended next"}
                            <span className="ml-2">{candidate.subject}</span>
                          </summary>
                          <div className="mt-2 text-xs text-[var(--muted)]">
                            {candidate.changed_paths.join(", ") ||
                              "No direct path diff"}
                            {candidate.owner_ids.length > 0 &&
                              ` · owners ${candidate.owner_ids.join(", ")}`}
                            {candidate.pull_request_ids.length > 0 &&
                              ` · pulls ${candidate.pull_request_ids.join(", ")}`}
                          </div>
                          {candidate.exclusion && (
                            <p className="mt-2 text-sm">
                              Excluded: {candidate.exclusion}
                            </p>
                          )}
                          <form
                            onSubmit={(e) =>
                              guideSearch(e, search, candidate.revision)
                            }
                            className="mt-3 flex flex-wrap gap-2"
                          >
                            <input type="hidden" name="kind" value="classify" />
                            <select
                              name="classification"
                              className={field}
                              defaultValue="working"
                            >
                              <option value="working">Working</option>
                              <option value="regressed">Regressed</option>
                              <option value="flaky">Flaky</option>
                              <option value="invalid">Invalid trial</option>
                            </select>
                            <input
                              name="reason"
                              className={field}
                              required
                              placeholder="Evidence-backed rationale"
                            />
                            <Button type="submit">Record guidance</Button>
                          </form>
                        </details>
                      ))}
                    </div>
                    <form
                      onSubmit={(e) => guideSearch(e, search)}
                      className="grid gap-2 rounded-lg bg-[var(--surface-muted)] p-3 sm:grid-cols-2"
                    >
                      <input type="hidden" name="kind" value="hypothesis" />
                      <textarea
                        name="claim"
                        className={`${field} sm:col-span-2`}
                        required
                        placeholder="Causal claim explaining why the transition introduced the behavior"
                      />
                      <input
                        name="candidate_revisions"
                        className={field}
                        required
                        placeholder="Cited candidate commits, comma separated"
                      />
                      <input
                        name="attempt_ids"
                        className={field}
                        placeholder="Cited attempt IDs, comma separated"
                      />
                      <input
                        name="evidence_ids"
                        className={field}
                        placeholder="Cited evidence IDs, comma separated"
                      />
                      <select
                        name="confidence"
                        className={field}
                        defaultValue="medium"
                      >
                        <option>low</option>
                        <option>medium</option>
                        <option>high</option>
                      </select>
                      <input
                        name="reason"
                        className={field}
                        required
                        placeholder="Decision rationale"
                      />
                      <Button type="submit">Append cited hypothesis</Button>
                    </form>
                    {search.hypotheses.map((h) => (
                      <blockquote
                        key={h.id}
                        className="border-l-2 border-[var(--brand)] pl-3 text-sm"
                      >
                        <b>
                          {h.confidence} confidence · {h.actor_id}
                        </b>
                        <p>{h.claim}</p>
                        <code>
                          {h.candidate_revisions
                            .map((r) => r.slice(0, 8))
                            .join(", ")}
                        </code>
                      </blockquote>
                    ))}
                  </section>
                ))
              )}
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Discussion and boundary history</h2>
              <form onSubmit={append} className="mt-3 grid gap-2">
                <select name="kind" className={field}>
                  <option value="discussion">Discussion</option>
                  <option value="hypothesis">Hypothesis</option>
                  <option value="scope_change">Scope change</option>
                  <option value="status_change">Status change</option>
                </select>
                <textarea
                  name="message"
                  className={field}
                  required
                  placeholder="Attributable context"
                />
                <input
                  name="event_value"
                  className={field}
                  placeholder="New environments or status when changing scope/status"
                />
                <Button type="submit">Append</Button>
              </form>
              <ol className="mt-4 space-y-3">
                {[...selected.history].reverse().map((x) => (
                  <li
                    key={x.id}
                    className="border-l-2 border-[var(--border)] pl-3 text-sm"
                  >
                    <b>{x.kind.replaceAll("_", " ")}</b> · {x.actor_id}
                    <p>{x.message}</p>
                  </li>
                ))}
              </ol>
            </Card>
          </div>
        ) : (
          <Card className="p-8 text-sm text-[var(--muted)]">
            Open or select an investigation.
          </Card>
        )}
      </div>
    </main>
  );
}
