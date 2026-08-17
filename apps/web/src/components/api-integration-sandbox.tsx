"use client";
import Link from "next/link";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useAuth } from "@/components/auth";
import { Badge, Button, Card } from "@/components/ui";
type Contract = {
  id: string;
  current_version: number;
  revisions: {
    version: number;
    version_label: string;
    operations: { id: string; method: string; path: string }[];
    environments: { id: string; name: string; availability: string }[];
  }[];
};
type App = {
  id: string;
  contract_id: string;
  contract_version: number;
  owner_id: string;
  name: string;
  environments: string[];
  requested_capabilities: string[];
  approved_capabilities: string[];
  status: string;
  decision_reason?: string;
  credentials: { prefix: string; expires_at: string; revoked_at?: string }[];
  events: { type: string; actor_id: string; detail: string; at: string }[];
};
type IntegrationWork = {
  id: string;
  application_id: string;
  title: string;
  kind: "task" | "session" | "workspace";
  owner_type: "human" | "agent";
  owner_id: string;
  consumer_repository_id: string;
  consumer_revision: string;
  contract_version: number;
  preload: {
    definition_path: string;
    sandbox_operations: string[];
    synthetic_only: boolean;
    credentials_included: boolean;
  };
  candidates: { id: string; evidence: { status: string }[] }[];
};
const field =
  "min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm";
export function APIIntegrationSandbox({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth();
  const [contracts, setContracts] = useState<Contract[]>([]),
    [contractID, setContractID] = useState(""),
    [apps, setApps] = useState<App[]>([]),
    [issued, setIssued] = useState<{ applicationID: string; secret: string }>(),
    [work, setWork] = useState<Record<string, IntegrationWork[]>>({}),
    [inspections, setInspections] = useState<Record<string, unknown>>({}),
    [error, setError] = useState("");
  const contract = contracts.find((x) => x.id === contractID),
    revision = contract?.revisions.at(-1);
  const load = useCallback(async () => {
    try {
      const c = await api<{ contracts: Contract[] }>(
        `/repositories/${repositoryID}/api-contracts`,
        {},
        token,
      );
      setContracts(c.contracts);
      const id = contractID || c.contracts[0]?.id || "";
      setContractID(id);
      if (token && id) {
        const a = await api<{ applications: App[] }>(
          `/repositories/${repositoryID}/api-contracts/${id}/applications`,
          {},
          token,
        );
        setApps(a.applications);
        const entries = await Promise.all(
          a.applications.map(async (application) => {
            const response = await api<{ integration_work: IntegrationWork[] }>(
              `/repositories/${repositoryID}/api-contracts/${id}/applications/${application.id}/integration-work`,
              {},
              token,
            );
            return [application.id, response.integration_work] as const;
          }),
        );
        setWork(Object.fromEntries(entries));
      }
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Integration workspace unavailable",
      );
    }
  }, [contractID, repositoryID, token]);
  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !revision) return;
    const f = new FormData(e.currentTarget);
    try {
      await api(
        `/repositories/${repositoryID}/api-contracts/${contractID}/applications`,
        {
          method: "POST",
          body: JSON.stringify({
            name: f.get("name"),
            project_url: f.get("project_url"),
            contract_version: revision.version,
            environments: f.getAll("environment"),
            capabilities: f.getAll("capability"),
          }),
        },
        token,
      );
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Request failed");
    }
  }
  async function act(
    app: App,
    action: "approved" | "denied" | "credential" | "revoke" | "exposure",
  ) {
    if (!token) return;
    try {
      if (action === "approved" || action === "denied") {
        const expiry = new Date();
        expiry.setUTCDate(expiry.getUTCDate() + 7);
        await api(
          `/repositories/${repositoryID}/api-contracts/${contractID}/applications/${app.id}/decision`,
          {
            method: "POST",
            body: JSON.stringify({
              status: action,
              reason:
                action === "approved"
                  ? "Approved for bounded synthetic integration"
                  : "Requested authority denied",
              capabilities: app.requested_capabilities,
              expires_at: expiry.toISOString(),
            }),
          },
          token,
        );
      } else if (action === "credential") {
        const out = await api<{ credential: { secret: string } }>(
          `/repositories/${repositoryID}/api-contracts/${contractID}/applications/${app.id}/credentials`,
          { method: "POST", body: JSON.stringify({ lifetime_hours: 24 }) },
          token,
        );
        setIssued({ applicationID: app.id, secret: out.credential.secret });
      } else {
        await api(
          `/repositories/${repositoryID}/api-contracts/${contractID}/applications/${app.id}/${action}`,
          { method: "POST", body: "{}" },
          token,
        );
        setIssued((current) =>
          current?.applicationID === app.id ? undefined : current,
        );
      }
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Action failed");
    }
  }
  async function run(e: FormEvent<HTMLFormElement>, app: App) {
    e.preventDefault();
    const f = new FormData(e.currentTarget),
      response = await fetch(
        `/api/repositories/${repositoryID}/api-contracts/${contractID}/applications/${app.id}/sandbox`,
        {
          method: "POST",
          headers: {
            Authorization: `Bearer ${issued?.applicationID === app.id ? issued.secret : ""}`,
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            operation_id: f.get("operation"),
            failure: f.get("failure"),
            request: { example: "synthetic consumer input" },
          }),
        },
      );
    const body: unknown = await response.json();
    setInspections((current) => ({ ...current, [app.id]: body }));
  }
  async function createWork(e: FormEvent<HTMLFormElement>, app: App) {
    e.preventDefault();
    if (!token || !user) return;
    const f = new FormData(e.currentTarget);
    try {
      await api(
        `/repositories/${repositoryID}/api-contracts/${contractID}/applications/${app.id}/integration-work`,
        {
          method: "POST",
          body: JSON.stringify({
            title: f.get("title"),
            kind: f.get("kind"),
            owner_type: f.get("owner_type"),
            owner_id: f.get("owner_id") || user.id,
            consumer_repository_id: f.get("consumer_repository_id"),
            consumer_revision: f.get("consumer_revision"),
          }),
        },
        token,
      );
      e.currentTarget.reset();
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Work creation failed");
    }
  }
  return (
    <main id="main-content" className="space-y-6">
      <header>
        <Link
          href={`/repositories/${repositoryID}/api-contracts`}
          className="text-sm text-[var(--muted)]"
        >
          API contracts
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">Consumer integrations</h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Prove an integration against synthetic data while the producer
          controls versions, operations, expiry, and impact. Approval grants no
          repository, deployment, environment, or production-data access.
        </p>
      </header>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      {revision && (
        <Card className="p-5">
          <form onSubmit={submit} className="grid gap-4 md:grid-cols-2">
            <select
              aria-label="API contract"
              value={contractID}
              onChange={(e) => setContractID(e.target.value)}
              className={field}
            >
              {contracts.map((x) => (
                <option key={x.id} value={x.id}>
                  {x.revisions.at(-1)?.version_label}
                </option>
              ))}
            </select>
            <input
              name="name"
              required
              placeholder="Application name"
              className={field}
            />
            <input
              name="project_url"
              type="url"
              placeholder="Consumer project URL"
              className={field}
            />
            <fieldset>
              <legend className="text-xs font-semibold">
                Sandbox environments
              </legend>
              {revision.environments
                .filter((x) => x.availability !== "unavailable")
                .map((x) => (
                  <label key={x.id} className="mr-3 text-sm">
                    <input
                      required
                      type="checkbox"
                      name="environment"
                      value={x.id}
                    />{" "}
                    {x.name}
                  </label>
                ))}
            </fieldset>
            <fieldset className="md:col-span-2">
              <legend className="text-xs font-semibold">
                Narrow operation capabilities
              </legend>
              {revision.operations.map((x) => (
                <label key={x.id} className="mr-4 text-sm">
                  <input
                    required
                    type="checkbox"
                    name="capability"
                    value={x.id}
                  />{" "}
                  {x.method} {x.path}
                </label>
              ))}
            </fieldset>
            <div>
              <Button>Request producer approval</Button>
            </div>
          </form>
        </Card>
      )}
      {apps.map((app) => (
        <Card key={app.id} className="p-5">
          <div className="flex gap-2">
            <h2 className="font-semibold">{app.name}</h2>
            <Badge
              tone={
                app.status === "approved"
                  ? "success"
                  : app.status === "pending"
                    ? "warning"
                    : "neutral"
              }
            >
              {app.status}
            </Badge>
          </div>
          <p className="mt-2 text-sm">
            v{app.contract_version} · {app.requested_capabilities.join(", ")} ·{" "}
            {app.environments.join(", ")}
          </p>
          {app.decision_reason && (
            <p className="text-xs">{app.decision_reason}</p>
          )}
          <div className="mt-3 flex flex-wrap gap-2">
            {app.status === "pending" && (
              <>
                <Button type="button" onClick={() => void act(app, "approved")}>
                  Approve 7 days
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => void act(app, "denied")}
                >
                  Deny
                </Button>
              </>
            )}
            {app.status === "approved" && app.owner_id === user?.id && (
              <>
                <Button
                  type="button"
                  onClick={() => void act(app, "credential")}
                >
                  Rotate credential
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => void act(app, "revoke")}
                >
                  Revoke
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => void act(app, "exposure")}
                >
                  Report secret exposure
                </Button>
              </>
            )}
          </div>
          {issued?.applicationID === app.id &&
            app.status === "approved" &&
            app.owner_id === user?.id && (
              <div className="mt-4 rounded-lg bg-[var(--surface-soft)] p-3">
                <strong className="text-xs">
                  Shown once — store outside source control
                </strong>
                <code className="block break-all text-xs">{issued.secret}</code>
                <form onSubmit={(e) => run(e, app)} className="mt-3 flex gap-2">
                  <select name="operation" className={field}>
                    {app.approved_capabilities.map((x) => (
                      <option key={x}>{x}</option>
                    ))}
                  </select>
                  <select name="failure" className={field}>
                    <option value="">Success</option>
                    <option value="rate_limit">Rate limit</option>
                    <option value="timeout">Timeout</option>
                    <option value="server_error">Server error</option>
                  </select>
                  <Button>Send synthetic request</Button>
                </form>
                {inspections[app.id] !== undefined && (
                  <pre className="mt-3 overflow-auto text-xs">
                    {JSON.stringify(inspections[app.id], null, 2)}
                  </pre>
                )}
              </div>
            )}
          {app.status === "approved" && (
            <div className="mt-5 border-t border-[var(--line)] pt-4">
              <h3 className="text-sm font-semibold">Reviewable adoption work</h3>
              <p className="mt-1 text-xs text-[var(--muted)]">
                Freeze a consumer commit with the exact contract and synthetic
                sandbox configuration. Credentials are never preloaded.
              </p>
              <form
                onSubmit={(event) => createWork(event, app)}
                className="mt-3 grid gap-2 md:grid-cols-3"
              >
                <input name="title" required placeholder="Work title" className={field} />
                <input name="consumer_repository_id" required placeholder="Consumer repository ID" className={field} />
                <input name="consumer_revision" required placeholder="Exact commit SHA" className={field} />
                <select name="kind" aria-label="Work type" className={field}>
                  <option value="task">Task</option><option value="session">Session</option><option value="workspace">Workspace</option>
                </select>
                <select name="owner_type" aria-label="Owner type" className={field}>
                  <option value="human">Human-owned</option><option value="agent">Agent-owned</option>
                </select>
                <input name="owner_id" placeholder={`Owner ID (default ${user?.id ?? "you"})`} className={field} />
                <div><Button>Create frozen work</Button></div>
              </form>
              <ul className="mt-3 space-y-2">
                {(work[app.id] ?? []).map((item) => (
                  <li key={item.id} className="rounded-lg bg-[var(--surface-soft)] p-3 text-xs">
                    <strong>{item.title}</strong> · {item.kind} · {item.owner_type}:{item.owner_id}
                    <span className="block text-[var(--muted)]">
                      Contract v{item.contract_version} · {item.consumer_repository_id}@{item.consumer_revision.slice(0, 12)} · {item.preload.definition_path} · {item.candidates.length} linked candidate(s)
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <details className="mt-3 text-xs">
            <summary>Attributable history and recovery</summary>
            {app.events.map((x, i) => (
              <p key={x.at + i}>
                {x.type} by {x.actor_id} · {x.detail}
              </p>
            ))}
          </details>
        </Card>
      ))}
    </main>
  );
}
