"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
import {
  RetirementContracts,
  type RetirementPlan,
} from "./retirement-contracts";

type Item = {
  kind: string;
  name: string;
  path?: string;
  symbol?: string;
  revision: string;
  notes?: string;
};
type Consumer = {
  name: string;
  repository_id?: string;
  owner_ids: string[];
  environment: string;
  revision?: string;
  discovery: string;
  evidence_state: string;
  evidence_reference?: string;
  last_observed_at?: string;
  compatibility_promise: string;
};
type Revision = {
  version: number;
  name: string;
  summary: string;
  commit_id: string;
  release_id: string;
  release_version: string;
  owner_ids: string[];
  items: Item[];
  consumers: Consumer[];
  unknown_use: boolean;
  unknown_use_reason?: string;
  created_by: string;
  created_at: string;
};
type Capability = {
  id: string;
  current_version: number;
  revisions: Revision[];
  diagnostics: {
    kind: string;
    severity: string;
    message: string;
    consumer?: string;
  }[];
  retirement_plans?: RetirementPlan[];
};
const text = (f: FormData, n: string) => String(f.get(n) ?? "").trim();
const list = (v: string) =>
  v
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
function Field({
  name,
  label,
  value = "",
  required = true,
}: {
  name: string;
  label: string;
  value?: string;
  required?: boolean;
}) {
  return (
    <label className="grid gap-1 text-xs font-semibold">
      {label}
      <input
        name={name}
        defaultValue={value}
        required={required}
        className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm font-normal"
      />
    </label>
  );
}
function Select({
  name,
  label,
  value,
  options,
  onChange,
}: {
  name: string;
  label: string;
  value: string;
  options: string[];
  onChange?: (value: string) => void;
}) {
  const props = onChange
    ? {
        value,
        onChange: (event: React.ChangeEvent<HTMLSelectElement>) =>
          onChange(event.target.value),
      }
    : { defaultValue: value };
  return (
    <label className="grid gap-1 text-xs font-semibold">
      {label}
      <select
        name={name}
        {...props}
        className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm font-normal"
      >
        {options.map((x) => (
          <option key={x}>{x}</option>
        ))}
      </select>
    </label>
  );
}

export function CapabilityWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth();
  const [values, setValues] = useState<Capability[]>([]),
    [selected, setSelected] = useState<Capability>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [itemKind, setItemKind] = useState("interface");
  const load = useCallback(async () => {
    try {
      const out = await api<{ capabilities: Capability[] }>(
        `/repositories/${repositoryID}/capabilities`,
        {},
        token,
      );
      setValues(out.capabilities);
      setSelected(
        (old) =>
          out.capabilities.find((x) => x.id === old?.id) ?? out.capabilities[0],
      );
      setItemKind(
        out.capabilities[0]?.revisions.at(-1)?.items[0]?.kind ?? "interface",
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Capabilities could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  const current = selected?.revisions.at(-1),
    item = current?.items[0],
    consumer = current?.consumers[0];
  async function publish(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    const f = new FormData(event.currentTarget);
    const unknown = f.get("unknown") === "on";
    const observed = text(f, "observed");
    const revision = {
      name: text(f, "name"),
      summary: text(f, "summary"),
      commit_id: text(f, "commit"),
      release_id: text(f, "release"),
      owner_ids: list(text(f, "owners")),
      items: [
        {
          kind: text(f, "kind"),
          name: text(f, "item_name"),
          path: text(f, "path"),
          symbol: text(f, "symbol"),
          revision: text(f, "item_revision"),
          notes: text(f, "notes"),
        },
        ...(current?.items.slice(1) ?? []),
      ],
      consumers: [
        {
          name: text(f, "consumer"),
          repository_id: text(f, "consumer_repository") || undefined,
          owner_ids: list(text(f, "consumer_owners")),
          environment: text(f, "environment"),
          revision: text(f, "consumer_revision") || undefined,
          discovery: text(f, "discovery"),
          evidence_state: text(f, "evidence"),
          evidence_reference: text(f, "evidence_reference") || undefined,
          last_observed_at: observed
            ? new Date(observed).toISOString()
            : undefined,
          compatibility_promise: text(f, "promise"),
        },
        ...(current?.consumers.slice(1) ?? []),
      ],
      unknown_use: unknown,
      unknown_use_reason: unknown ? text(f, "unknown_reason") : undefined,
    };
    try {
      await api(
        selected
          ? `/repositories/${repositoryID}/capabilities/${selected.id}/revisions`
          : `/repositories/${repositoryID}/capabilities`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected?.current_version ?? 0,
            revision,
          }),
        },
        token,
      );
      await load();
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "The capability could not be published.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-6">
      <header>
        <div className="flex gap-2">
          <Badge tone="info">Capability inventory</Badge>
          {current && <Badge>v{current.version}</Badge>}
        </div>
        <h1 className="mt-3 text-2xl font-semibold">
          Know the footprint before removal
        </h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Map what a capability includes, who relies on it, and where evidence
          remains unknown, inaccessible, stale, or dynamically discovered.
        </p>
      </header>
      {selected && current && (
        <RetirementContracts
          repositoryID={repositoryID}
          capabilityID={selected.id}
          current={current}
          plans={selected.retirement_plans ?? []}
          token={token}
          userID={user?.id}
          reload={load}
        />
      )}
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      {values.length > 1 && (
        <nav className="flex gap-2" aria-label="Capabilities">
          {values.map((x) => (
            <Button
              key={x.id}
              variant={x.id === selected?.id ? "primary" : "secondary"}
              onClick={() => {
                setSelected(x);
                setItemKind(x.revisions.at(-1)?.items[0]?.kind ?? "interface");
              }}
            >
              {x.revisions.at(-1)?.name}
            </Button>
          ))}
        </nav>
      )}
      {current && (
        <>
          <Card className="p-5">
            <h2 className="text-lg font-semibold">{current.name}</h2>
            <p className="mt-1 text-sm text-[var(--muted)]">
              {current.summary}
            </p>
            <p className="mt-3 text-xs">
              Release {current.release_version} ·{" "}
              <code>{current.commit_id.slice(0, 12)}</code>
            </p>
          </Card>
          <div className="grid gap-4 lg:grid-cols-2">
            <Card className="p-5">
              <h2 className="font-semibold">Included surface</h2>
              {current.items.map((x, i) => (
                <div
                  key={`${x.kind}-${x.name}-${i}`}
                  className="mt-3 border-t border-[var(--line)] pt-3 text-sm"
                >
                  <Badge>{x.kind}</Badge>
                  <p className="mt-2 font-semibold">{x.name}</p>
                  <p className="font-mono text-xs text-[var(--muted)]">
                    {x.path || "release"}
                    {x.symbol ? `#${x.symbol}` : ""} · {x.revision.slice(0, 12)}
                  </p>
                </div>
              ))}
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Consumers and promises</h2>
              {current.consumers.map((x, i) => (
                <div
                  key={`${x.name}-${i}`}
                  className="mt-3 border-t border-[var(--line)] pt-3 text-sm"
                >
                  <p className="font-semibold">{x.name}</p>
                  <p>
                    {x.environment} · {x.discovery} · {x.evidence_state}
                  </p>
                  <p className="mt-1 text-[var(--muted)]">
                    {x.compatibility_promise}
                  </p>
                </div>
              ))}
            </Card>
          </div>
          {(selected?.diagnostics.length ?? 0) !== 0 && (
            <Card className="p-5">
              <h2 className="font-semibold">Incomplete evidence</h2>
              <div className="mt-3 space-y-2">
                {selected?.diagnostics.map((x, i) => (
                  <p key={`${x.kind}-${i}`} className="text-sm">
                    <Badge
                      tone={x.severity === "blocking" ? "danger" : "warning"}
                    >
                      {x.kind.replaceAll("_", " ")}
                    </Badge>{" "}
                    <span className="ml-2">
                      {x.message}
                      {x.consumer ? ` (${x.consumer})` : ""}
                    </span>
                  </p>
                ))}
              </div>
            </Card>
          )}
        </>
      )}
      {user && (
        <Card className="p-5">
          <h2 className="text-lg font-semibold">
            {selected ? "Publish successor revision" : "Define a capability"}
          </h2>
          <form onSubmit={publish} className="mt-4 grid gap-4">
            <div className="grid gap-3 md:grid-cols-2">
              <Field
                name="name"
                label="Capability name"
                value={current?.name}
              />
              <Field
                name="owners"
                label="Owner IDs (comma-separated)"
                value={current?.owner_ids.join(", ") ?? user.id}
              />
              <Field
                name="commit"
                label="Released commit"
                value={current?.commit_id}
              />
              <Field
                name="release"
                label="Release ID"
                value={current?.release_id}
              />
            </div>
            <label className="grid gap-1 text-xs font-semibold">
              Summary
              <textarea
                name="summary"
                defaultValue={current?.summary}
                required
                rows={2}
                className="rounded-lg border border-[var(--line-strong)] p-3 text-sm font-normal"
              />
            </label>
            <h3 className="font-semibold">Selected footprint item</h3>
            <div className="grid gap-3 md:grid-cols-2">
              <Select
                name="kind"
                label="Kind"
                value={itemKind}
                onChange={setItemKind}
                options={[
                  "interface",
                  "symbol",
                  "flag",
                  "package",
                  "schema",
                  "configuration",
                  "documentation",
                  "journey",
                  "release",
                ]}
              />
              <Field name="item_name" label="Item name" value={item?.name} />
              <Field
                name="path"
                label="Exact repository path"
                value={item?.path}
                required={itemKind !== "release"}
              />
              <Field
                name="symbol"
                label="Symbol or selector"
                value={item?.symbol}
                required={false}
              />
              <Field
                name="item_revision"
                label="Exact item revision"
                value={item?.revision ?? current?.commit_id}
              />
              <Field
                name="notes"
                label="Scope notes"
                value={item?.notes}
                required={false}
              />
            </div>
            <h3 className="font-semibold">Consumer and usage evidence</h3>
            <div className="grid gap-3 md:grid-cols-2">
              <Field name="consumer" label="Consumer" value={consumer?.name} />
              <Field
                name="consumer_repository"
                label="Consumer repository ID"
                value={consumer?.repository_id}
                required={false}
              />
              <Field
                name="consumer_owners"
                label="Consumer owner IDs"
                value={consumer?.owner_ids.join(", ")}
              />
              <Field
                name="environment"
                label="Environment"
                value={consumer?.environment}
              />
              <Field
                name="consumer_revision"
                label="Exact consumer revision"
                value={consumer?.revision}
                required={false}
              />
              <Select
                name="discovery"
                label="Discovery"
                value={consumer?.discovery ?? "declared"}
                options={["declared", "dynamic", "unknown"]}
              />
              <Select
                name="evidence"
                label="Evidence state"
                value={consumer?.evidence_state ?? "unknown"}
                options={["current", "stale", "inaccessible", "unknown"]}
              />
              <Field
                name="evidence_reference"
                label="Usage evidence reference"
                value={consumer?.evidence_reference}
                required={false}
              />
              <Field
                name="observed"
                label="Observed at"
                value={consumer?.last_observed_at?.slice(0, 16)}
                required={false}
              />
              <Field
                name="promise"
                label="Compatibility promise"
                value={consumer?.compatibility_promise}
              />
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                name="unknown"
                defaultChecked={current?.unknown_use}
              />
              Use may exist beyond known consumers
            </label>
            <Field
              name="unknown_reason"
              label="Unknown-use boundary"
              value={current?.unknown_use_reason}
              required={false}
            />
            <Button type="submit" disabled={busy}>
              {busy ? "Publishing…" : "Publish immutable revision"}
            </Button>
          </form>
        </Card>
      )}
    </div>
  );
}
