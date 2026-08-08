"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  api,
  type Deployment,
  type DeploymentEnvironment,
  type Incident,
  type Repository,
  type User,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
import { Icons } from "./icons";

const field =
  "mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal outline-none focus:border-[var(--brand)]";
const tone = (severity: Incident["severity"]) =>
  severity === "sev1" || severity === "sev2" ? "danger" : "warning";
const stamp = (value: string) =>
  new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
const errorMessage = (reason: unknown) =>
  reason instanceof Error ? reason.message : "The incident operation failed.";
async function pages<T>(path: string, key: string, token: string) {
  const out: T[] = [];
  let after: string | null = null;
  do {
    const page = await api<Record<string, T[] | string | null>>(
      `${path}${path.includes("?") ? "&" : "?"}limit=100${after ? `&after=${after}` : ""}`,
      {},
      token,
    );
    out.push(...((page[key] as T[]) ?? []));
    after = page.next_cursor as string | null;
  } while (after);
  return out;
}

export function IncidentsWorkspace() {
  const { token, user, loading: authLoading } = useAuth();
  const router = useRouter();
  const [incidents, setIncidents] = useState<Incident[]>([]),
    [repositories, setRepositories] = useState<Repository[]>([]),
    [environments, setEnvironments] = useState<
      Record<string, DeploymentEnvironment[]>
    >({}),
    [deployments, setDeployments] = useState<Record<string, Deployment[]>>({}),
    [show, setShow] = useState(false),
    [pending, setPending] = useState(false),
    [error, setError] = useState("");
  const load = useCallback(async () => {
    if (authLoading) return;
    if (!token) {
      setIncidents([]);
      return;
    }
    try {
      const [found, repos] = await Promise.all([
        pages<Incident>("/incidents", "incidents", token),
        pages<Repository>("/repositories", "repositories", token),
      ]);
      setIncidents(found);
      setRepositories(repos);
      const values = await Promise.all(
        repos.map(
          async (repo) =>
            [
              repo.id,
              await Promise.all([
                api<{ environments: DeploymentEnvironment[] }>(
                  `/repositories/${repo.id}/environments`,
                  {},
                  token,
                ).catch(() => ({ environments: [] })),
                api<{ deployments: Deployment[] }>(
                  `/repositories/${repo.id}/deployments`,
                  {},
                  token,
                ).catch(() => ({ deployments: [] })),
              ]),
            ] as const,
        ),
      );
      setEnvironments(
        Object.fromEntries(values.map(([id, [x]]) => [id, x.environments])),
      );
      setDeployments(
        Object.fromEntries(values.map(([id, [, x]]) => [id, x.deployments])),
      );
    } catch (reason) {
      setError(errorMessage(reason));
    }
  }, [authLoading, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    setError("");
    const data = new FormData(event.currentTarget);
    const sourceValue = String(data.get("source") ?? "");
    const [sourceRepo, sourceDeployment] = sourceValue.split(":");
    const deployment = deployments[sourceRepo]?.find(
      (x) => x.id === sourceDeployment,
    );
    const failed = deployment?.evidence.find((x) => x.state === "failed");
    const ids = [
      ...new Set([
        ...data.getAll("repository_ids").map(String),
        ...(deployment ? [sourceRepo] : []),
      ]),
    ];
    try {
      const created = await api<Incident>(
        "/incidents",
        {
          method: "POST",
          body: JSON.stringify({
            title: data.get("title"),
            summary: data.get("summary"),
            severity: data.get("severity"),
            scopes: ids.map((id) => ({
              repository_id: id,
              environment_ids: [
                ...new Set([
                  ...data.getAll(`environment_${id}`).map(String),
                  ...(deployment?.repository_id === id
                    ? [deployment.environment_id]
                    : []),
                ]),
              ],
            })),
            roles: [{ name: "incident commander", user_id: user?.id }],
            source: deployment
              ? {
                  repository_id: sourceRepo,
                  deployment_id: sourceDeployment,
                  stage: failed?.stage,
                  signal: failed?.signal,
                }
              : undefined,
          }),
        },
        token,
      );
      router.push(`/incidents/${created.id}`);
    } catch (reason) {
      setError(errorMessage(reason));
      setPending(false);
    }
  }
  if (authLoading)
    return (
      <Card className="p-8 text-sm text-[var(--muted)]">
        Loading response workspace…
      </Card>
    );
  if (!user)
    return (
      <Card className="p-8 text-center">
        <h1 className="text-2xl font-semibold">
          Coordinate incidents where the work happens
        </h1>
        <p className="mt-2 text-sm text-[var(--muted)]">
          Sign in to see incidents affecting your repositories.
        </p>
        <Link
          className="mt-5 inline-flex rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-semibold text-white"
          href="/?access=signin"
        >
          Sign in
        </Link>
      </Card>
    );
  return (
    <div className="space-y-7">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="font-mono text-xs font-semibold uppercase tracking-[.16em] text-[var(--danger)]">
            Response coordination
          </p>
          <h1 className="mt-2 text-3xl font-semibold">Incidents</h1>
          <p className="mt-2 max-w-2xl text-sm text-[var(--muted)]">
            Establish ownership, affected systems, and one attributable source
            of truth while service is at risk.
          </p>
        </div>
        <Button onClick={() => setShow((x) => !x)}>
          <Icons.Plus size={16} />
          {show ? "Cancel" : "Declare incident"}
        </Button>
      </header>
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      {show && (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Declare an incident</h2>
          <form onSubmit={create} className="mt-5 grid gap-4">
            <label className="text-sm font-semibold">
              Title
              <input className={field} name="title" required maxLength={200} />
            </label>
            <label className="text-sm font-semibold">
              Current impact
              <textarea
                className={`${field} py-3`}
                name="summary"
                rows={4}
                required
                maxLength={10000}
              />
            </label>
            <label className="text-sm font-semibold">
              Severity
              <select className={field} name="severity" defaultValue="sev2">
                <option value="sev1">SEV1 · critical</option>
                <option value="sev2">SEV2 · major</option>
                <option value="sev3">SEV3 · degraded</option>
                <option value="sev4">SEV4 · minor</option>
              </select>
            </label>
            <fieldset>
              <legend className="text-sm font-semibold">
                Affected repositories and environments
              </legend>
              <div className="mt-2 grid gap-2">
                {repositories.map((repo) => (
                  <div
                    key={repo.id}
                    className="rounded-lg border border-[var(--line)] p-3"
                  >
                    <label className="flex gap-2 text-sm font-semibold">
                      <input
                        type="checkbox"
                        name="repository_ids"
                        value={repo.id}
                      />
                      {repo.name}
                    </label>
                    <div className="ml-6 mt-2 flex flex-wrap gap-3">
                      {(environments[repo.id] ?? []).map((env) => (
                        <label key={env.id} className="text-xs">
                          <input
                            type="checkbox"
                            name={`environment_${repo.id}`}
                            value={env.id}
                            className="mr-1"
                          />
                          {env.name}
                        </label>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </fieldset>
            <label className="text-sm font-semibold">
              Deployment health signal{" "}
              <span className="font-normal text-[var(--muted)]">
                (optional)
              </span>
              <select className={field} name="source">
                <option value="">Manual declaration</option>
                {repositories.flatMap((repo) =>
                  (deployments[repo.id] ?? [])
                    .filter((run) =>
                      run.evidence.some((x) => x.state === "failed"),
                    )
                    .map((run) => (
                      <option key={run.id} value={`${repo.id}:${run.id}`}>
                        {repo.name} · failed{" "}
                        {run.evidence.find((x) => x.state === "failed")?.stage}/
                        {run.evidence.find((x) => x.state === "failed")?.signal}
                      </option>
                    )),
                )}
              </select>
            </label>
            <div>
              <Button type="submit" disabled={pending}>
                {pending ? "Declaring…" : "Declare and take command"}
              </Button>
            </div>
          </form>
        </Card>
      )}
      <Card className="overflow-hidden">
        {incidents.length ? (
          <div className="divide-y divide-[var(--line)]">
            {incidents.map((item) => (
              <Link
                className="flex items-start gap-4 p-5 hover:bg-[var(--danger-soft)]"
                href={`/incidents/${item.id}`}
                key={item.id}
              >
                <span className="mt-1 size-2 rounded-full bg-[var(--danger)]" />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap gap-2">
                    <h2 className="font-semibold">{item.title}</h2>
                    <Badge tone={tone(item.severity)}>
                      {item.severity.toUpperCase()}
                    </Badge>
                    <Badge
                      tone={item.status === "resolved" ? "success" : "neutral"}
                    >
                      {item.status}
                    </Badge>
                  </div>
                  <p className="mt-1 line-clamp-2 text-sm text-[var(--muted)]">
                    {item.summary}
                  </p>
                  <p className="mt-2 text-xs text-[var(--muted)]">
                    {item.scopes.length} repositories · updated{" "}
                    {stamp(item.updated_at)}
                  </p>
                </div>
                <Icons.Chevron size={16} />
              </Link>
            ))}
          </div>
        ) : (
          <div className="p-9 text-center">
            <h2 className="font-semibold">
              No incidents in your collaborative spaces
            </h2>
            <p className="mt-2 text-sm text-[var(--muted)]">
              When service is at risk, declare here so responders share the same
              truth.
            </p>
          </div>
        )}
      </Card>
    </div>
  );
}

export function IncidentDetail({ incidentID }: { incidentID: string }) {
  const { token, user, loading } = useAuth();
  const [incident, setIncident] = useState<Incident | null>(null),
    [repos, setRepos] = useState<Record<string, Repository>>({}),
    [people, setPeople] = useState<Record<string, User>>({}),
    [pending, setPending] = useState(false),
    [error, setError] = useState(""),
    [agentToken, setAgentToken] = useState("");
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const value = await api<Incident>(`/incidents/${incidentID}`, {}, token);
      setIncident(value);
      const repoValues = await Promise.all(
        value.scopes.map((x) =>
          api<Repository>(`/repositories/${x.repository_id}`, {}, token),
        ),
      );
      setRepos(Object.fromEntries(repoValues.map((x) => [x.id, x])));
      const ids = [
        ...new Set([
          value.declared_by,
          ...value.roles.map((x) => x.user_id),
          ...value.timeline.map((x) => x.actor_id),
        ]),
      ];
      const users = await Promise.all(
        ids.map((id) => api<User>(`/users/${id}`, {}, token).catch(() => null)),
      );
      setPeople(
        Object.fromEntries(
          users.filter((x): x is User => Boolean(x)).map((x) => [x.id, x]),
        ),
      );
    } catch (reason) {
      setError(errorMessage(reason));
    }
  }, [incidentID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function submit(path: string, body: object) {
    if (!token) return false;
    setPending(true);
    setError("");
    const isOperation = path.endsWith("/updates") || path.endsWith("/findings"),
      draft = JSON.stringify(body),
      storageKey = `vivarium.incident-operation.${incidentID}.${path.split("/").at(-1)}`;
    let operationID = "";
    if (isOperation) {
      try {
        const stored = JSON.parse(
          localStorage.getItem(storageKey) ?? "null",
        ) as { draft?: string; operation_id?: string } | null;
        if (
          stored?.draft === draft &&
          /^[0-9a-f]{32}$/.test(stored.operation_id ?? "")
        )
          operationID = stored.operation_id ?? "";
      } catch {}
      if (!operationID) operationID = crypto.randomUUID().replaceAll("-", "");
      localStorage.setItem(
        storageKey,
        JSON.stringify({ draft, operation_id: operationID }),
      );
    }
    try {
      const payload = isOperation
        ? { operation_id: operationID, ...body }
        : body;
      setIncident(
        await api<Incident>(
          path,
          { method: "POST", body: JSON.stringify(payload) },
          token,
        ),
      );
      if (isOperation) localStorage.removeItem(storageKey);
      return true;
    } catch (reason) {
      setError(errorMessage(reason));
      return false;
    } finally {
      setPending(false);
    }
  }
  async function change(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!incident || !token) return;
    const data = new FormData(event.currentTarget);
    setPending(true);
    try {
      setIncident(
        await api<Incident>(
          `/incidents/${incidentID}`,
          {
            method: "PATCH",
            body: JSON.stringify({
              expected_version: incident.version,
              severity: data.get("severity"),
              status: data.get("status"),
              roles: incident.roles,
              message: data.get("message"),
            }),
          },
          token,
        ),
      );
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setPending(false);
    }
  }
  async function finding(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    const form = event.currentTarget,
      data = new FormData(form),
      kind = String(data.get("source_kind")),
      start = String(data.get("window_start") ?? ""),
      end = String(data.get("window_end") ?? "");
    const body = {
      kind: data.get("finding_kind"),
      message: data.get("message"),
      audience: data.get("audience"),
      evidence: [
        {
          kind,
          repository_id: data.get("repository_id"),
          resource_id: data.get("resource_id"),
          query: data.get("query"),
          window_start: start ? new Date(start).toISOString() : undefined,
          window_end: end ? new Date(end).toISOString() : undefined,
        },
      ],
    };
    if (await submit(`/incidents/${incidentID}/findings`, body)) form.reset();
  }
  async function delegate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token || !incident) return;
    const form = event.currentTarget,
      data = new FormData(form),
      revisions = String(data.get("revisions") ?? "")
        .split("\n")
        .map((x) => x.trim())
        .filter(Boolean)
        .map((line) => {
          const [repository_id, commit_id] = line.split(":");
          return { repository_id, commit_id };
        }),
      selected = new Set(data.getAll("evidence_ids").map(String)),
      evidence = incident.timeline
        .filter((x) => selected.has(x.id))
        .flatMap((x) => x.evidence ?? []);
    setPending(true);
    setError("");
    try {
      const result = await api<{
        incident: Incident;
        credential: { token: string };
      }>(
        `/incidents/${incidentID}/investigations`,
        {
          method: "POST",
          body: JSON.stringify({
            mandate: data.get("mandate"),
            evidence,
            revisions,
            expires_in: 3600,
          }),
        },
        token,
      );
      setIncident(result.incident);
      setAgentToken(result.credential.token);
      form.reset();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setPending(false);
    }
  }
  async function control(id: string, action: string) {
    if (!token) return;
    const message =
      action === "guide"
        ? (prompt("Guidance for the investigation") ?? "")
        : "";
    if (action === "guide" && !message) return;
    setPending(true);
    try {
      setIncident(
        await api<Incident>(
          `/incidents/${incidentID}/investigations/${id}/controls`,
          { method: "POST", body: JSON.stringify({ action, message }) },
          token,
        ),
      );
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setPending(false);
    }
  }
  async function proposeMitigation(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!incident) return;
    const form = event.currentTarget, data = new FormData(form), selected = new Set(data.getAll("evidence_ids").map(String));
    const evidence = incident.timeline.filter((x) => selected.has(x.id)).flatMap((x) => x.evidence ?? []);
    const [stage, signal] = String(data.get("criterion")).split("/");
    if (await submit(`/incidents/${incidentID}/actions`, {kind:data.get("kind"),repository_id:data.get("repository_id"),deployment_id:data.get("deployment_id"),rationale:data.get("rationale"),evidence,health_criteria:[{stage,signal}]})) form.reset();
  }
  async function decideMitigation(id: string, decision: "approve" | "reject") {
    const message = prompt(`${decision === "approve" ? "Approval" : "Rejection"} rationale`) ?? "";
    if (!message) return;
    await submit(`/incidents/${incidentID}/actions/${id}/decisions`, {decision,message,override:false});
  }
  async function executeMitigation(action: NonNullable<Incident["actions"]>[number]) {
    if (!token) return;
    setPending(true); setError("");
    let outcome: "started" | "failed" = "started", resourceID = action.deployment_id, message = "Governed execution was accepted.";
    try {
      if (action.kind === "pause_rollout") await api(`/repositories/${action.repository_id}/deployments/${action.deployment_id}/controls`, {method:"POST",body:JSON.stringify({action:"pause",expected_state:"running",reason:action.rationale})}, token);
      else {
        const result = await api<{deployment?:{id:string};pull_request?:{id:string}}>(`/repositories/${action.repository_id}/deployments/${action.deployment_id}/recoveries`, {method:"POST",body:JSON.stringify({action:action.kind === "restore_release" ? "rollback" : "repair"})}, token);
        resourceID = result.deployment?.id ?? result.pull_request?.id ?? resourceID;
      }
    } catch (reason) { outcome="failed"; message=errorMessage(reason); }
    try { setIncident(await api<Incident>(`/incidents/${incidentID}/actions/${action.id}/attempts`, {method:"POST",body:JSON.stringify({outcome,resource_id:resourceID,message})}, token)); }
    catch (reason) { setError(errorMessage(reason)); }
    finally { setPending(false); }
  }
  async function verifyMitigation(action: NonNullable<Incident["actions"]>[number]) {
    const resourceID = prompt("Recovery deployment ID to verify against the declared health criteria") ?? "";
    if (!resourceID) return;
    await submit(`/incidents/${incidentID}/actions/${action.id}/attempts`, {outcome:"recovered",resource_id:resourceID,message:"Declared health criteria passed on the retained recovery deployment."});
  }
  const evidenceHref = (
    source: NonNullable<Incident["timeline"][number]["evidence"]>[number],
  ) =>
    source.kind === "incident"
      ? `/incidents/${source.resource_id}`
      : source.kind === "pull_request"
        ? `/pulls/${source.repository_id}/${source.resource_id}`
        : source.kind === "release"
          ? `/repositories/${source.repository_id}/releases/${source.resource_id}`
          : source.kind === "commit"
            ? `/repositories/${source.repository_id}?ref=${source.resource_id}`
            : `/repositories/${source.repository_id}/releases`;
  if (loading || (!incident && !error))
    return (
      <Card className="p-8 text-sm text-[var(--muted)]">
        Gathering the operating picture…
      </Card>
    );
  if (!user) return <Card className="p-8">Sign in to join this response.</Card>;
  if (!incident)
    return <Card className="p-8 text-[var(--danger)]">{error}</Card>;
  return (
    <div className="space-y-6">
      <Link
        href="/incidents"
        className="text-sm font-semibold text-[var(--brand)]"
      >
        ← All incidents
      </Link>
      <header>
        <div className="flex flex-wrap gap-2">
          <Badge tone={tone(incident.severity)}>
            {incident.severity.toUpperCase()}
          </Badge>
          <Badge tone={incident.status === "resolved" ? "success" : "neutral"}>
            {incident.status}
          </Badge>
        </div>
        <h1 className="mt-3 text-3xl font-semibold">{incident.title}</h1>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-[var(--muted)]">
          {incident.summary}
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
      <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <main className="space-y-5">
          <Card className="p-5">
            <h2 className="font-semibold">Mitigation decisions</h2>
            <p className="mt-1 text-xs text-[var(--muted)]">Proposals retain their evidence, independent authorization, governed execution attempts, and recovery criteria.</p>
            <form className="mt-4 grid gap-3 sm:grid-cols-2" onSubmit={proposeMitigation}>
              <select className={field} name="kind"><option value="pause_rollout">Pause rollout</option><option value="restore_release">Restore attested release</option><option value="emergency_repair">Emergency repair</option></select>
              <select className={field} name="repository_id">{incident.scopes.map((x)=><option key={x.repository_id} value={x.repository_id}>{repos[x.repository_id]?.name ?? x.repository_id}</option>)}</select>
              <input className={field} name="deployment_id" required placeholder="Affected deployment ID" />
              <input className={field} name="criterion" required placeholder="Declared health criterion: stage/signal" />
              <textarea className={`${field} py-3 sm:col-span-2`} name="rationale" required rows={3} placeholder="Expected effect, risk, and reason to act" />
              <fieldset className="sm:col-span-2"><legend className="text-xs font-semibold">Exact supporting evidence</legend>{incident.timeline.filter((x)=>x.evidence?.length).map((entry)=><label className="mt-2 flex gap-2 text-xs" key={entry.id}><input type="checkbox" name="evidence_ids" value={entry.id}/>{entry.message}</label>)}</fieldset>
              <div><Button disabled={pending}>Propose mitigation</Button></div>
            </form>
            {(incident.actions ?? []).map((action)=><div key={action.id} className="mt-4 rounded-lg border border-[var(--line)] p-4"><div className="flex flex-wrap justify-between gap-2"><b>{action.kind.replaceAll("_"," ")}</b><Badge tone={action.status === "recovered" ? "success" : action.status === "failed" || action.status === "rejected" ? "danger" : "neutral"}>{action.status}</Badge></div><p className="mt-2 text-sm">{action.rationale}</p><p className="mt-2 text-xs text-[var(--muted)]">{action.evidence.length} evidence sources · recovery requires {action.health_criteria.map((x)=>`${x.stage}/${x.signal}`).join(", ")}</p>{action.status === "proposed" && <div className="mt-3 flex gap-2"><Button onClick={()=>void decideMitigation(action.id,"approve")}>Approve</Button><Button variant="secondary" onClick={()=>void decideMitigation(action.id,"reject")}>Reject</Button></div>}{action.status === "approved" && <Button className="mt-3" onClick={()=>void executeMitigation(action)} disabled={pending}>Execute through environment policy</Button>}{(action.status === "executing" || action.status === "failed") && <Button className="mt-3" variant="secondary" onClick={()=>void verifyMitigation(action)} disabled={pending}>Verify recovery</Button>}{action.attempts.map((attempt)=><p key={attempt.id} className="mt-2 text-xs"><b>{attempt.outcome}:</b> {attempt.message}</p>)}</div>)}
          </Card>
          <Card className="p-5">
            <h2 className="font-semibold">Delegate investigation</h2>
            <p className="mt-1 text-xs text-[var(--muted)]">
              Freeze selected evidence and commits. Agent access reports
              diagnostics but cannot change production, credentials, or secrets.
            </p>
            <form className="mt-4 grid gap-3" onSubmit={delegate}>
              <label className="text-xs font-semibold">
                Mandate
                <textarea
                  className={`${field} py-3`}
                  name="mandate"
                  required
                  rows={3}
                />
              </label>
              <fieldset>
                <legend className="text-xs font-semibold">
                  Selected evidence
                </legend>
                {incident.timeline
                  .filter((x) => x.evidence?.length)
                  .map((entry) => (
                    <label className="mt-2 flex gap-2 text-xs" key={entry.id}>
                      <input
                        type="checkbox"
                        name="evidence_ids"
                        value={entry.id}
                      />
                      {entry.message}
                    </label>
                  ))}
              </fieldset>
              <label className="text-xs font-semibold">
                Repository revisions (repository-id:commit)
                <textarea
                  className={`${field} py-3 font-mono`}
                  name="revisions"
                  required
                  rows={3}
                />
              </label>
              <div>
                <Button disabled={pending}>
                  Delegate read-only investigation
                </Button>
              </div>
            </form>
            {agentToken && (
              <div className="mt-4 rounded-lg bg-[var(--warning-soft)] p-3 text-xs">
                <strong>One-time agent token</strong>
                <p className="mt-1 break-all font-mono">{agentToken}</p>
              </div>
            )}
            {(incident.investigations ?? []).map((item) => (
              <div
                className="mt-4 rounded-lg border border-[var(--line)] p-4"
                key={item.id}
              >
                <div className="flex justify-between">
                  <b>Agent {item.agent_id.slice(0, 8)}</b>
                  <Badge
                    tone={item.state === "running" ? "success" : "neutral"}
                  >
                    {item.state}
                  </Badge>
                </div>
                <p className="mt-2 text-sm">{item.mandate}</p>
                <p className="mt-2 text-xs text-[var(--muted)]">
                  {item.evidence.length} evidence sources ·{" "}
                  {item.revisions.length} revisions · no production authority
                </p>
                {item.state !== "cancelled" && (
                  <div className="mt-3 flex flex-wrap gap-2">
                    <Button
                      variant="secondary"
                      onClick={() => void control(item.id, "guide")}
                    >
                      Guide
                    </Button>
                    <Button
                      variant="secondary"
                      onClick={() =>
                        void control(
                          item.id,
                          item.state === "running" ? "pause" : "resume",
                        )
                      }
                    >
                      {item.state === "running" ? "Pause" : "Resume"}
                    </Button>
                    <Button
                      variant="secondary"
                      onClick={() => void control(item.id, "cancel")}
                    >
                      Cancel
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </Card>
          <Card className="p-5">
            <h2 className="font-semibold">Connected diagnosis</h2>
            <p className="mt-1 text-xs text-[var(--muted)]">
              Record a claim with the exact source another responder needs to
              verify it.
            </p>
            <form className="mt-3 grid gap-3 sm:grid-cols-2" onSubmit={finding}>
              <label className="text-xs font-semibold">
                Claim type
                <select className={field} name="finding_kind">
                  {["observation", "hypothesis", "query", "conclusion"].map(
                    (x) => (
                      <option key={x}>{x}</option>
                    ),
                  )}
                </select>
              </label>
              <label className="text-xs font-semibold">
                Audience
                <select className={field} name="audience">
                  <option value="participants">Participants</option>
                  <option value="public">Public</option>
                </select>
              </label>
              <label className="text-xs font-semibold">
                Affected repository
                <select className={field} name="repository_id">
                  {incident.scopes.map((x) => (
                    <option key={x.repository_id} value={x.repository_id}>
                      {repos[x.repository_id]?.name ?? x.repository_id}
                    </option>
                  ))}
                </select>
              </label>
              <label className="text-xs font-semibold">
                Evidence type
                <select className={field} name="source_kind">
                  {[
                    "log",
                    "health_signal",
                    "deployment",
                    "release",
                    "commit",
                    "pull_request",
                    "incident",
                  ].map((x) => (
                    <option key={x} value={x}>
                      {x.replaceAll("_", " ")}
                    </option>
                  ))}
                </select>
              </label>
              <label className="text-xs font-semibold sm:col-span-2">
                Source ID
                <input
                  className={field}
                  name="resource_id"
                  required
                  placeholder="Deployment, release, commit, pull request, or incident ID"
                />
              </label>
              <label className="text-xs font-semibold">
                Window start{" "}
                <span className="font-normal text-[var(--muted)]">
                  (required for logs/signals)
                </span>
                <input
                  className={field}
                  type="datetime-local"
                  name="window_start"
                />
              </label>
              <label className="text-xs font-semibold">
                Window end
                <input
                  className={field}
                  type="datetime-local"
                  name="window_end"
                />
              </label>
              <label className="text-xs font-semibold sm:col-span-2">
                Query or signal selector
                <input
                  className={field}
                  name="query"
                  placeholder="e.g. errors where service=checkout"
                />
              </label>
              <label className="text-xs font-semibold sm:col-span-2">
                Finding
                <textarea
                  className={`${field} py-3`}
                  name="message"
                  required
                  rows={3}
                  placeholder="What does this evidence show, and how certain are you?"
                />
              </label>
              <div>
                <Button disabled={pending}>Attach finding</Button>
              </div>
            </form>
          </Card>
          <Card className="p-5">
            <h2 className="font-semibold">Response update</h2>
            <form
              className="mt-3 grid gap-3"
              onSubmit={(e) => {
                e.preventDefault();
                const data = new FormData(e.currentTarget);
                void submit(`/incidents/${incidentID}/updates`, {
                  message: data.get("message"),
                  audience: data.get("audience"),
                });
                e.currentTarget.reset();
              }}
            >
              <textarea
                className={`${field} py-3`}
                name="message"
                required
                rows={4}
                placeholder="What changed, what is known, and what happens next?"
              />
              <div className="flex flex-wrap gap-3">
                <select
                  name="audience"
                  className="rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"
                >
                  <option value="participants">Participants</option>
                  <option value="public">Public update</option>
                </select>
                <Button disabled={pending}>Publish update</Button>
              </div>
            </form>
          </Card>
          <section>
            <h2 className="mb-3 text-lg font-semibold">
              Attributable timeline
            </h2>
            <ol className="space-y-3">
              {[...incident.timeline].reverse().map((entry) => (
                <li key={entry.id}>
                  <Card className="p-5">
                    <div className="flex flex-wrap justify-between gap-2">
                      <p className="text-sm font-semibold">
                        {people[entry.actor_id]?.display_name ??
                          "Unknown responder"}{" "}
                        · {entry.kind.replaceAll("_", " ")}
                      </p>
                      <Badge
                        tone={
                          entry.audience === "public" ? "success" : "neutral"
                        }
                      >
                        {entry.audience}
                      </Badge>
                    </div>
                    <p className="mt-2 whitespace-pre-wrap text-sm leading-6">
                      {entry.message}
                    </p>
                    {entry.evidence?.map((source) => (
                      <div
                        key={`${source.kind}-${source.resource_id}`}
                        className="mt-3 rounded-lg border border-[var(--line)] p-3 text-xs"
                      >
                        <div className="flex flex-wrap items-center justify-between gap-2">
                          <Badge tone="info">
                            {source.kind.replaceAll("_", " ")}
                          </Badge>
                          <Link
                            className="font-semibold text-[var(--brand)]"
                            href={evidenceHref(source)}
                          >
                            Inspect live source →
                          </Link>
                        </div>
                        <p className="mt-2 font-semibold">{source.label}</p>
                        {source.query && (
                          <p className="mt-1 font-mono text-[var(--muted)]">
                            {source.query}
                          </p>
                        )}
                        {source.window_start && (
                          <p className="mt-1 text-[var(--muted)]">
                            Window {stamp(source.window_start)} →{" "}
                            {stamp(source.window_end!)}
                          </p>
                        )}
                        <p className="mt-1 text-[var(--muted)]">
                          Captured {stamp(source.captured_at)} · historical
                          label retained
                        </p>
                      </div>
                    ))}
                    <div className="mt-3 flex items-center justify-between gap-3 text-xs text-[var(--muted)]">
                      <span>
                        {stamp(entry.created_at)} ·{" "}
                        {entry.acknowledged_by?.length ?? 0} acknowledged
                      </span>
                      {!entry.acknowledged_by?.includes(user.id) && (
                        <button
                          className="font-semibold text-[var(--brand)]"
                          disabled={pending}
                          onClick={() =>
                            void submit(
                              `/incidents/${incidentID}/timeline/${entry.id}/acknowledgements`,
                              {},
                            )
                          }
                        >
                          Acknowledge
                        </button>
                      )}
                    </div>
                  </Card>
                </li>
              ))}
            </ol>
          </section>
        </main>
        <aside className="space-y-4">
          <Card className="p-5">
            <h2 className="font-semibold">Affected systems</h2>
            {incident.scopes.map((scope) => (
              <div className="mt-3" key={scope.repository_id}>
                <p className="font-mono text-sm font-semibold">
                  {repos[scope.repository_id]?.name ?? scope.repository_id}
                </p>
                <p className="mt-1 text-xs text-[var(--muted)]">
                  {scope.environment_ids.length
                    ? `${scope.environment_ids.length} environment(s) affected`
                    : "Repository-wide impact"}
                </p>
              </div>
            ))}
            {incident.source && (
              <p className="mt-4 border-t border-[var(--line)] pt-4 text-xs text-[var(--muted)]">
                Declared from verified signal {incident.source.stage}/
                {incident.source.signal} on deployment{" "}
                {incident.source.deployment_id.slice(0, 8)}.
              </p>
            )}
          </Card>
          <Card className="p-5">
            <h2 className="font-semibold">Response roles</h2>
            <dl className="mt-3 space-y-3">
              {incident.roles.map((role) => (
                <div key={role.name}>
                  <dt className="text-xs uppercase text-[var(--muted)]">
                    {role.name}
                  </dt>
                  <dd className="text-sm font-semibold">
                    {people[role.user_id]?.display_name ?? role.user_id}
                  </dd>
                </div>
              ))}
            </dl>
          </Card>
          <Card className="p-5">
            <h2 className="font-semibold">Current state</h2>
            <form onSubmit={change} className="mt-3 grid gap-3">
              <label className="text-xs font-semibold">
                Severity
                <select
                  name="severity"
                  defaultValue={incident.severity}
                  className={field}
                >
                  {[1, 2, 3, 4].map((x) => (
                    <option key={x} value={`sev${x}`}>
                      SEV{x}
                    </option>
                  ))}
                </select>
              </label>
              <label className="text-xs font-semibold">
                Status
                <select
                  name="status"
                  defaultValue={incident.status}
                  className={field}
                >
                  <option value="investigating">Investigating</option>
                  <option value="identified">Identified</option>
                  <option value="monitoring">Monitoring</option>
                  <option value="resolved">Resolved</option>
                </select>
              </label>
              <label className="text-xs font-semibold">
                Decision note
                <textarea name="message" className={`${field} py-2`} rows={3} />
              </label>
              <Button disabled={pending}>Update state</Button>
            </form>
          </Card>
        </aside>
      </div>
    </div>
  );
}
