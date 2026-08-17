"use client";
import Link from "next/link";
import {
  FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { api } from "@/lib/api";
import { useAuth } from "@/components/auth";
import { Badge, Button, Card } from "@/components/ui";

type Revision = {
  version: number;
  version_label: string;
  title: string;
  summary: string;
  source: {
    commit_id: string;
    pull_request_id: string;
    release_id?: string;
    definition_path: string;
    documentation_path: string;
  };
  operations: {
    id: string;
    method: string;
    path: string;
    summary: string;
    stability: string;
  }[];
  schemas: { id: string; name: string; definition: string }[];
  errors: {
    id: string;
    code: string;
    http_status: number;
    meaning: string;
    recovery: string;
  }[];
  authentication: { id: string; mode: string; description: string }[];
  environments: {
    id: string;
    name: string;
    base_url: string;
    availability: string;
  }[];
  limits: {
    requests: number;
    window_seconds: number;
    burst: number;
    payload_bytes: number;
    concurrency: number;
  };
  owner_ids: string[];
  stability: string;
  support_policy: {
    channels: string[];
    response_target: string;
    deprecation_notice_days: number;
    sunset_notice_days: number;
  };
  links: { kind: string; id?: string; url?: string; label: string }[];
  compatibility: {
    from_version: string;
    level: string;
    promise: string;
    breaking_changes: string[];
  };
  known_gaps: string[];
  rationale: string;
  created_by: string;
  created_at: string;
};
type Contract = {
  id: string;
  current_version: number;
  revisions: Revision[];
  diagnostics: { code: string; severity: string; detail: string }[];
};
const field =
  "mt-1 min-h-10 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm font-normal";
const split = (v: FormDataEntryValue | null) =>
  String(v ?? "")
    .split(/[,\n]/)
    .map((x) => x.trim())
    .filter(Boolean);
const json = <T,>(v: FormDataEntryValue | null, fallback: T): T => {
  try {
    return JSON.parse(String(v)) as T;
  } catch {
    return fallback;
  }
};

export function APIContractsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth();
  const [items, setItems] = useState<Contract[]>([]),
    [selected, setSelected] = useState<Contract | null>(null),
    [creatingNew, setCreatingNew] = useState(false),
    [compare, setCompare] = useState<number>(0),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const creatingNewRef = useRef(false);
  const load = useCallback(async () => {
    try {
      const out = await api<{ contracts: Contract[] }>(
        `/repositories/${repositoryID}/api-contracts`,
        {},
        token,
      );
      setItems(out.contracts);
      setSelected((old) =>
        creatingNewRef.current
          ? null
          : (out.contracts.find((x) => x.id === old?.id) ??
            out.contracts[0] ??
            null),
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Contracts could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);
  const current = selected?.revisions.at(-1);
  const previous = selected?.revisions.find((x) => x.version === compare);
  const changes = useMemo(() => {
    if (!current || !previous) return [];
    const old = new Map(previous.operations.map((x) => [x.id, x]));
    const now = new Map(current.operations.map((x) => [x.id, x]));
    return [
      ...previous.operations
        .filter((x) => !now.has(x.id))
        .map((x) => `Removed ${x.method} ${x.path}`),
      ...current.operations
        .filter((x) => !old.has(x.id))
        .map((x) => `Added ${x.method} ${x.path}`),
      ...current.operations
        .filter(
          (x) =>
            old.has(x.id) &&
            JSON.stringify(old.get(x.id)) !== JSON.stringify(x),
        )
        .map((x) => `Changed ${x.method} ${x.path}`),
    ];
  }, [current, previous]);
  async function publish(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    const f = new FormData(e.currentTarget);
    const schemaID = "response",
      authID = "bearer",
      errorID = "default-error";
    const revision = {
      version_label: String(f.get("version_label")),
      title: String(f.get("title")),
      summary: String(f.get("summary")),
      source: {
        commit_id: String(f.get("commit_id")),
        pull_request_id: String(f.get("pull_request_id")),
        release_id: String(f.get("release_id")) || undefined,
        definition_path: String(f.get("definition_path")),
        documentation_path: String(f.get("documentation_path")),
      },
      operations: json(f.get("operations"), []),
      schemas: json(f.get("schemas"), [
        {
          id: schemaID,
          name: "Response",
          kind: "object",
          definition: '{"type":"object"}',
          required_fields: [],
          description: "API response",
        },
      ]),
      errors: json(f.get("errors"), [
        {
          id: errorID,
          code: "request_failed",
          http_status: 400,
          meaning: "The request was rejected",
          recovery: "Correct the request and retry",
        },
      ]),
      authentication: json(f.get("authentication"), [
        { id: authID, mode: "bearer", description: "Bearer token", scopes: [] },
      ]),
      environments: json(f.get("environments"), []),
      limits: {
        requests: Number(f.get("requests")),
        window_seconds: Number(f.get("window")),
        burst: Number(f.get("burst")),
        payload_bytes: Number(f.get("payload")),
        concurrency: Number(f.get("concurrency")),
      },
      owner_ids: split(f.get("owners")),
      stability: String(f.get("stability")),
      support_policy: {
        channels: split(f.get("channels")),
        response_target: String(f.get("response_target")),
        deprecation_notice_days: Number(f.get("deprecation")),
        sunset_notice_days: Number(f.get("sunset")),
      },
      links: json(f.get("links"), []),
      compatibility: {
        from_version: String(f.get("from_version")),
        level: String(f.get("compatibility")),
        promise: String(f.get("promise")),
        breaking_changes: split(f.get("breaking_changes")),
      },
      known_gaps: split(f.get("known_gaps")),
      rationale: String(f.get("rationale")),
    };
    try {
      const path =
        !creatingNew && selected
          ? `/repositories/${repositoryID}/api-contracts/${selected.id}/revisions`
          : `/repositories/${repositoryID}/api-contracts`;
      const out = await api<Contract>(
        path,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: creatingNew
              ? 0
              : (selected?.current_version ?? 0),
            revision,
          }),
        },
        token,
      );
      creatingNewRef.current = false;
      setCreatingNew(false);
      setSelected(out);
      await load();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Contract could not be published.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <main id="main-content" className="space-y-6">
      <header>
        <Link
          href={`/repositories/${repositoryID}`}
          className="text-sm text-[var(--muted)] hover:text-[var(--brand)]"
        >
          Repository
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">API contracts</h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Integrate from reviewed implementation provenance, explicit interface
          definitions, and versioned guarantees—not detached documentation.
        </p>
        <Link
          href={`/repositories/${repositoryID}/api-contracts/integrations`}
          className="mt-3 inline-flex text-sm font-semibold text-[var(--brand)]"
        >
          Open consumer integration sandbox →
        </Link>
      </header>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
        <section className="space-y-4">
          {current ? (
            <>
              <Card className="p-5">
                <div className="flex flex-wrap gap-2">
                  <h2 className="mr-auto text-lg font-semibold">
                    {current.title}
                  </h2>
                  <Badge>{current.version_label}</Badge>
                  <Badge
                    tone={
                      current.stability === "stable" ? "success" : "warning"
                    }
                  >
                    {current.stability}
                  </Badge>
                </div>
                <p className="mt-2 text-sm text-[var(--muted)]">
                  {current.summary}
                </p>
                <p className="mt-3 text-xs">
                  Reviewed merge <code>{current.source.commit_id}</code> · pull{" "}
                  <code>{current.source.pull_request_id}</code>
                  {current.source.release_id ? (
                    <>
                      {" "}
                      · release <code>{current.source.release_id}</code>
                    </>
                  ) : (
                    " · unreleased"
                  )}
                </p>
                {selected!.diagnostics.map((x) => (
                  <p
                    key={x.code + x.detail}
                    className="mt-2 rounded-lg bg-[var(--surface-soft)] p-2 text-sm"
                  >
                    <strong>{x.code.replaceAll("_", " ")}:</strong> {x.detail}
                  </p>
                ))}
              </Card>
              <Card className="p-5">
                <h3 className="font-semibold">Operations and terms</h3>
                <div className="mt-3 space-y-3">
                  {current.operations.map((x) => (
                    <div key={x.id} className="rounded-lg border p-3">
                      <code>
                        {x.method} {x.path}
                      </code>
                      <Badge
                        tone={x.stability === "stable" ? "success" : "warning"}
                      >
                        {x.stability}
                      </Badge>
                      <p className="mt-1 text-sm">{x.summary}</p>
                    </div>
                  ))}
                </div>
                <p className="mt-4 text-sm">
                  <strong>Limits:</strong> {current.limits.requests} requests /{" "}
                  {current.limits.window_seconds}s ·{" "}
                  {current.limits.payload_bytes} byte payload
                </p>
                <p className="mt-2 text-sm">
                  <strong>Support:</strong>{" "}
                  {current.support_policy.channels.join(", ")} ·{" "}
                  {current.support_policy.response_target} ·{" "}
                  {current.support_policy.deprecation_notice_days} day
                  deprecation notice
                </p>
                {current.known_gaps.length > 0 && (
                  <div className="mt-3">
                    <strong className="text-sm">Known gaps</strong>
                    <ul className="list-disc pl-5 text-sm">
                      {current.known_gaps.map((x) => (
                        <li key={x}>{x}</li>
                      ))}
                    </ul>
                  </div>
                )}
              </Card>
              <Card className="p-5">
                <h3 className="font-semibold">Compare versions</h3>
                <select
                  className={field}
                  value={compare}
                  onChange={(e) => setCompare(Number(e.target.value))}
                >
                  <option value={0}>Choose an earlier version</option>
                  {selected!.revisions.slice(0, -1).map((x) => (
                    <option key={x.version} value={x.version}>
                      {x.version_label}
                    </option>
                  ))}
                </select>
                {previous && (
                  <div className="mt-3 text-sm">
                    <p>
                      <strong>
                        {previous.version_label} → {current.version_label}:
                      </strong>{" "}
                      {current.compatibility.level}.{" "}
                      {current.compatibility.promise}
                    </p>
                    {changes.length ? (
                      <ul className="mt-2 list-disc pl-5">
                        {changes.map((x) => (
                          <li key={x}>{x}</li>
                        ))}
                      </ul>
                    ) : (
                      <p className="mt-2 text-[var(--muted)]">
                        No operation-level changes.
                      </p>
                    )}
                  </div>
                )}
              </Card>
            </>
          ) : (
            <Card className="p-5 text-sm text-[var(--muted)]">
              No API contract has been published.
            </Card>
          )}
        </section>
        <aside>
          <div className="flex items-center justify-between gap-2">
            <h2 className="font-semibold">Published interfaces</h2>
            {token && (
              <Button
                onClick={() => {
                  creatingNewRef.current = true;
                  setCreatingNew(true);
                  setSelected(null);
                  setCompare(0);
                  setError("");
                }}
              >
                New contract
              </Button>
            )}
          </div>
          {items.map((x) => (
            <button
              key={x.id}
              onClick={() => {
                creatingNewRef.current = false;
                setCreatingNew(false);
                setSelected(x);
                setCompare(0);
              }}
              className="mt-2 w-full rounded-lg border bg-white p-3 text-left text-sm"
            >
              <strong>{x.revisions.at(-1)?.title}</strong>
              <span className="block text-xs text-[var(--muted)]">
                {x.revisions.at(-1)?.version_label} · {x.diagnostics.length}{" "}
                explicit state(s)
              </span>
            </button>
          ))}
        </aside>
      </div>
      {token && (
        <Card className="p-5">
          <h2 className="font-semibold">
            {!creatingNew && selected
              ? "Publish successor"
              : "Publish reviewed contract"}
          </h2>
          <form onSubmit={publish} className="mt-4 grid gap-3 md:grid-cols-2">
            <Field n="title" l="Interface title" value={current?.title} />
            <Field n="version_label" l="Version label" />
            <Field n="commit_id" l="Exact merge commit" />
            <Field n="pull_request_id" l="Reviewed pull request ID" />
            <Field n="release_id" l="Release ID (optional)" required={false} />
            <Field
              n="owners"
              l="Owner IDs"
              value={current?.owner_ids.join(",") ?? user?.id}
            />
            <Field n="definition_path" l="Definition source path" />
            <Field n="documentation_path" l="Documentation source path" />
            <Area n="summary" l="Purpose and audience" />
            <Area
              n="operations"
              l="Operations JSON"
              placeholder='[{"id":"list","method":"GET","path":"/widgets","summary":"List widgets","authentication":["bearer"],"parameters":[],"response_schema_ids":["response"],"error_ids":["default-error"],"stability":"stable","owner_ids":["USER_ID"]}]'
            />
            <Area n="schemas" l="Schemas JSON (optional)" required={false} />
            <Area n="errors" l="Errors JSON (optional)" required={false} />
            <Area
              n="authentication"
              l="Authentication JSON (optional)"
              required={false}
            />
            <Area
              n="environments"
              l="Environments JSON"
              placeholder='[{"id":"production","name":"Production","base_url":"https://api.example.com","availability":"available","regions":[]}]'
            />
            <Field
              n="requests"
              l="Requests per window"
              type="number"
              value="100"
            />
            <Field n="window" l="Window seconds" type="number" value="60" />
            <Field
              n="payload"
              l="Payload bytes"
              type="number"
              value="1048576"
            />
            <Field n="burst" l="Burst (optional)" type="number" value="0" />
            <Field
              n="concurrency"
              l="Concurrency (optional)"
              type="number"
              value="0"
            />
            <Field
              n="channels"
              l="Support channels"
              value="repository support"
            />
            <Field
              n="response_target"
              l="Support response target"
              value="two business days"
            />
            <Field
              n="deprecation"
              l="Deprecation notice days"
              type="number"
              value="90"
            />
            <Field
              n="sunset"
              l="Sunset notice days"
              type="number"
              value="180"
            />
            <label className="text-xs font-semibold">
              Stability
              <select name="stability" className={field}>
                <option>stable</option>
                <option>beta</option>
                <option>experimental</option>
                <option>deprecated</option>
              </select>
            </label>
            <label className="text-xs font-semibold">
              Compatibility
              <select name="compatibility" className={field}>
                <option
                  value={!creatingNew && selected ? "compatible" : "initial"}
                >
                  {!creatingNew && selected ? "compatible" : "initial"}
                </option>
                <option value="conditionally_compatible">
                  conditionally compatible
                </option>
                <option value="breaking">breaking</option>
              </select>
            </label>
            <Field
              n="from_version"
              l="Compared from version"
              required={false}
            />
            <Area n="promise" l="Compatibility promise" />
            <Area n="breaking_changes" l="Breaking changes" required={false} />
            <Area n="known_gaps" l="Known gaps" required={false} />
            <Area
              n="links"
              l="Source, release, documentation, data-use, support links JSON"
            />
            <Area n="rationale" l="Publication rationale" />
            <div>
              <Button disabled={busy}>
                {busy ? "Publishing…" : "Publish contract"}
              </Button>
            </div>
          </form>
        </Card>
      )}
    </main>
  );
}
function Field({
  n,
  l,
  value = "",
  type = "text",
  required = true,
}: {
  n: string;
  l: string;
  value?: string;
  type?: string;
  required?: boolean;
}) {
  return (
    <label className="text-xs font-semibold">
      {l}
      <input
        name={n}
        defaultValue={value}
        type={type}
        required={required}
        className={field}
      />
    </label>
  );
}
function Area({
  n,
  l,
  placeholder = "",
  required = true,
}: {
  n: string;
  l: string;
  placeholder?: string;
  required?: boolean;
}) {
  return (
    <label className="text-xs font-semibold md:col-span-2">
      {l}
      <textarea
        name={n}
        placeholder={placeholder}
        required={required}
        rows={3}
        className={`${field} py-2 font-mono`}
      />
    </label>
  );
}
