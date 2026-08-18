"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
import { InfrastructureExecutions } from "./infrastructure-executions";

type Config = {
  name: string;
  source: string;
  sensitivity: string;
  required: boolean;
};
type Resource = {
  id: string;
  kind: string;
  name: string;
  description: string;
  owner_ids: string[];
  provider: string;
  provider_ref?: string;
  provider_access: string;
  environment_id?: string;
  release_id?: string;
  depends_on: string[];
  configuration: Config[];
  constraints: { kind: string; limit: number; unit: string; note?: string }[];
  commitments: {
    security: string[];
    privacy: string[];
    reliability: string[];
    continuity: string[];
    regions: string[];
  };
};
type Revision = {
  version: number;
  title: string;
  summary: string;
  revision: string;
  resources: Resource[];
  owner_ids: string[];
  rationale: string;
  created_by: string;
  created_at: string;
};
type Observation = {
  id: string;
  definition_version: number;
  resource_id?: string;
  provider_resource: string;
  observed_revision: string;
  environment_id?: string;
  release_id?: string;
  status: string;
  summary: string;
  visibility: string;
  managed: boolean;
  observed_at: string;
  recorded_by: string;
};
type Definition = {
  id: string;
  current_version: number;
  revisions: Revision[];
  observations: Observation[];
  diagnostics: {
    kind: string;
    severity: string;
    message: string;
    resource_id?: string;
  }[];
};
const value = (f: FormData, n: string) => String(f.get(n) ?? "").trim(),
  list = (v: string) =>
    v
      .split(",")
      .map((x) => x.trim())
      .filter(Boolean);

export function InfrastructureWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth(),
    [items, setItems] = useState<Definition[]>([]),
    [selected, setSelected] = useState<Definition>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    try {
      const out = await api<{ definitions: Definition[] }>(
        `/repositories/${repositoryID}/infrastructure`,
        {},
        token ?? undefined,
      );
      setItems(out.definitions);
      setSelected(
        (x) =>
          out.definitions.find((v) => v.id === x?.id) ?? out.definitions[0],
      );
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Infrastructure could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  const current = selected?.revisions.at(-1),
    resource = current?.resources[0];
  function revision(f: FormData): Revision {
    const owners = list(value(f, "owners")),
      secret = value(f, "config_sensitivity") === "secret_backed";
    return {
      version: 0,
      title: value(f, "title"),
      summary: value(f, "summary"),
      revision: value(f, "revision"),
      owner_ids: owners,
      rationale: value(f, "rationale"),
      created_by: "",
      created_at: "",
      resources: [
        {
          id: value(f, "resource_id"),
          kind: value(f, "kind"),
          name: value(f, "resource_name"),
          description: value(f, "description"),
          owner_ids: owners,
          provider: value(f, "provider"),
          provider_ref: value(f, "provider_ref"),
          provider_access: value(f, "provider_access"),
          environment_id: value(f, "environment_id"),
          release_id: value(f, "release_id"),
          depends_on: list(value(f, "depends_on")),
          configuration: [
            {
              name: value(f, "config_name"),
              source: secret ? "secret" : value(f, "config_source"),
              sensitivity: value(f, "config_sensitivity"),
              required: true,
            },
          ],
          constraints: [
            {
              kind: "cost",
              limit: Number(value(f, "cost_limit")),
              unit: value(f, "cost_unit"),
            },
            {
              kind: "capacity",
              limit: Number(value(f, "capacity_limit")),
              unit: value(f, "capacity_unit"),
            },
          ],
          commitments: {
            security: list(value(f, "security")),
            privacy: list(value(f, "privacy")),
            reliability: list(value(f, "reliability")),
            continuity: list(value(f, "continuity")),
            regions: list(value(f, "regions")),
          },
        },
        ...(current?.resources.slice(1) ?? []),
      ],
    };
  }
  async function publish(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    setError("");
    try {
      const path = selected
          ? `/repositories/${repositoryID}/infrastructure/${selected.id}/revisions`
          : `/repositories/${repositoryID}/infrastructure`,
        out = await api<Definition>(
          path,
          {
            method: "POST",
            body: JSON.stringify({
              revision: revision(new FormData(e.currentTarget)),
              ...(selected
                ? { expected_version: selected.current_version }
                : {}),
            }),
          },
          token,
        );
      setSelected(out);
      setItems((x) => [out, ...x.filter((v) => v.id !== out.id)]);
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Infrastructure could not be published.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function observe(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    setBusy(true);
    setError("");
    const f = new FormData(e.currentTarget);
    try {
      const out = await api<Definition>(
        `/repositories/${repositoryID}/infrastructure/${selected.id}/observations`,
        {
          method: "POST",
          body: JSON.stringify({
            definition_version: selected.current_version,
            resource_id: value(f, "observed_resource"),
            provider_resource: value(f, "provider_resource"),
            observed_revision: value(f, "observed_revision"),
            environment_id: value(f, "observed_environment"),
            release_id: value(f, "observed_release"),
            status: value(f, "status"),
            summary: value(f, "observation_summary"),
            visibility: value(f, "visibility"),
            managed: value(f, "managed") === "yes",
            observed_at: new Date(value(f, "observed_at")).toISOString(),
          }),
        },
        token,
      );
      setSelected(out);
      setItems((x) => x.map((v) => (v.id === out.id ? out : v)));
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Observation could not be published.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-6">
      <header>
        <Link
          href={`/repositories/${repositoryID}`}
          className="text-sm text-[var(--muted)] hover:text-[var(--brand)]"
        >
          Repository
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">
          Infrastructure and operational intent
        </h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Understand runtime resources, ownership, providers, configuration
          boundaries, constraints, and commitments at an exact project
          revision—without requiring cloud-console access.
        </p>
      </header>
      {token && (
        <Card className="p-5">
          <div className="flex items-center justify-between">
            <h2 className="font-semibold">
              {selected
                ? "Publish a complete successor"
                : "Define infrastructure"}
            </h2>
            {selected && (
              <Button
                type="button"
                variant="secondary"
                onClick={() => setSelected(undefined)}
              >
                New definition
              </Button>
            )}
          </div>
          <form className="mt-4 space-y-4" onSubmit={publish}>
            <Grid>
              <Field n="title" l="Definition title" v={current?.title} />
              <Field n="revision" l="Exact Git commit" v={current?.revision} />
              <Field
                n="owners"
                l="Accountable owner IDs"
                v={current?.owner_ids.join(", ") ?? user?.id}
              />
            </Grid>
            <Area n="summary" l="Scope and intent" v={current?.summary} />
            <Area n="rationale" l="Revision rationale" v={current?.rationale} />
            <Group title="Declared resource">
              <Field n="resource_id" l="Stable resource key" v={resource?.id} />
              <Select
                n="kind"
                l="Resource kind"
                v={resource?.kind ?? "service"}
                options={[
                  "environment",
                  "service",
                  "network",
                  "identity",
                  "data_store",
                  "compute",
                  "external_dependency",
                ]}
              />
              <Field n="resource_name" l="Resource name" v={resource?.name} />
              <Field n="provider" l="Provider" v={resource?.provider} />
              <Field
                n="provider_ref"
                l="Non-secret provider identity"
                v={resource?.provider_ref}
              />
              <Select
                n="provider_access"
                l="Observed-state access"
                v={resource?.provider_access ?? "participant"}
                options={["public", "participant", "inaccessible"]}
              />
              <Field
                n="environment_id"
                l="Established environment ID"
                v={resource?.environment_id}
                required={false}
              />
              <Field
                n="release_id"
                l="Exact release ID"
                v={resource?.release_id}
                required={false}
              />
              <Field
                n="depends_on"
                l="Dependency resource keys"
                v={resource?.depends_on.join(", ")}
                required={false}
              />
              <Area
                n="description"
                l="Responsibility and purpose"
                v={resource?.description}
              />
            </Group>
            <Group title="Configuration, cost, and capacity">
              <Field
                n="config_name"
                l="Configuration boundary"
                v={resource?.configuration[0]?.name}
              />
              <Select
                n="config_source"
                l="Configuration source"
                v={resource?.configuration[0]?.source ?? "environment"}
                options={["literal", "environment", "file", "provider"]}
              />
              <Select
                n="config_sensitivity"
                l="Sensitivity"
                v={resource?.configuration[0]?.sensitivity ?? "internal"}
                options={["public", "internal", "secret_backed"]}
              />
              <Field
                n="cost_limit"
                l="Cost limit"
                type="number"
                v={String(
                  resource?.constraints.find((x) => x.kind === "cost")?.limit ??
                    0,
                )}
              />
              <Field
                n="cost_unit"
                l="Cost unit"
                v={
                  resource?.constraints.find((x) => x.kind === "cost")?.unit ??
                  "USD/month"
                }
              />
              <Field
                n="capacity_limit"
                l="Capacity limit"
                type="number"
                v={String(
                  resource?.constraints.find((x) => x.kind === "capacity")
                    ?.limit ?? 0,
                )}
              />
              <Field
                n="capacity_unit"
                l="Capacity unit"
                v={
                  resource?.constraints.find((x) => x.kind === "capacity")
                    ?.unit ?? "requests/second"
                }
              />
            </Group>
            <Group title="Operational commitments">
              <Field
                n="security"
                l="Security controls"
                v={resource?.commitments.security.join(", ")}
              />
              <Field
                n="privacy"
                l="Privacy boundaries"
                v={resource?.commitments.privacy.join(", ")}
              />
              <Field
                n="reliability"
                l="Reliability promises"
                v={resource?.commitments.reliability.join(", ")}
              />
              <Field
                n="continuity"
                l="Continuity promises"
                v={resource?.commitments.continuity.join(", ")}
              />
              <Field
                n="regions"
                l="Committed regions"
                v={resource?.commitments.regions.join(", ")}
              />
            </Group>
            <Button disabled={busy}>
              {busy
                ? "Publishing…"
                : selected
                  ? `Publish version ${selected.current_version + 1}`
                  : "Publish definition"}
            </Button>
          </form>
        </Card>
      )}
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      {selected && current && (
        <>
          <Card className="p-5">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-lg font-semibold">{current.title}</h2>
              <Badge>v{selected.current_version}</Badge>
              <Badge>commit {current.revision.slice(0, 10)}</Badge>
            </div>
            <p className="mt-2 text-sm text-[var(--muted)]">
              {current.summary}
            </p>
            <div className="mt-4 grid gap-3 md:grid-cols-2">
              {current.resources.map((x) => (
                <div key={x.id} className="rounded-lg border p-4">
                  <div className="flex gap-2">
                    <Badge>{x.kind.replaceAll("_", " ")}</Badge>
                    <Badge
                      tone={
                        x.provider_access === "inaccessible"
                          ? "warning"
                          : "info"
                      }
                    >
                      {x.provider_access} provider state
                    </Badge>
                  </div>
                  <h3 className="mt-2 font-semibold">{x.name}</h3>
                  <p className="text-sm text-[var(--muted)]">
                    {x.provider} · owners {x.owner_ids.join(", ")}
                  </p>
                  <p className="mt-2 text-sm">{x.description}</p>
                  <p className="mt-2 text-xs">
                    Cost/capacity:{" "}
                    {x.constraints
                      .map((c) => `${c.limit} ${c.unit}`)
                      .join(" · ")}
                  </p>
                  <p className="mt-2 text-xs">
                    Security: {x.commitments.security.join(", ")} · Privacy:{" "}
                    {x.commitments.privacy.join(", ")} · Reliability:{" "}
                    {x.commitments.reliability.join(", ")} · Continuity:{" "}
                    {x.commitments.continuity.join(", ")} · Regions:{" "}
                    {x.commitments.regions.join(", ")}
                  </p>
                </div>
              ))}
            </div>
            {selected.diagnostics.map((x, i) => (
              <p
                key={`${x.kind}-${x.resource_id}-${i}`}
                className="mt-3 rounded-lg border p-3 text-sm"
              >
                <Badge
                  tone={
                    x.severity === "blocking"
                      ? "danger"
                      : x.severity === "warning"
                        ? "warning"
                        : "info"
                  }
                >
                  {x.kind.replaceAll("_", " ")}
                </Badge>{" "}
                {x.message}
              </p>
            ))}
            <details className="mt-4">
              <summary className="cursor-pointer font-semibold">
                Immutable revision history
              </summary>
              {selected.revisions.map((x) => (
                <p className="mt-2 text-sm" key={x.version}>
                  v{x.version} · {x.revision} · {x.rationale} · {x.created_by}
                </p>
              ))}
            </details>
          </Card>
          {token && (
            <Card className="p-5">
              <h2 className="font-semibold">
                Publish permitted observed state
              </h2>
              <p className="mt-1 text-sm text-[var(--muted)]">
                Record sanitized inventory facts only. Provider credentials and
                configuration values are rejected.
              </p>
              <form
                className="mt-4 grid gap-3 md:grid-cols-3"
                onSubmit={observe}
              >
                <Select
                  n="managed"
                  l="Declared resource"
                  v="yes"
                  options={["yes", "no"]}
                />
                <Field
                  n="observed_resource"
                  l="Resource key (blank when unmanaged)"
                  v={current.resources[0]?.id}
                  required={false}
                />
                <Field n="provider_resource" l="Provider resource identity" />
                <Field n="observed_revision" l="Observed exact revision" />
                <Field
                  n="observed_environment"
                  l="Environment ID"
                  required={false}
                />
                <Field n="observed_release" l="Release ID" required={false} />
                <Select
                  n="status"
                  l="Observed status"
                  v="healthy"
                  options={["healthy", "degraded", "unknown"]}
                />
                <Select
                  n="visibility"
                  l="Evidence visibility"
                  v="participant"
                  options={["public", "participant"]}
                />
                <Field n="observed_at" l="Observed at" type="datetime-local" />
                <Area n="observation_summary" l="Sanitized observation" />
                <Button disabled={busy}>Publish observation</Button>
              </form>
              <div className="mt-4 space-y-2">
                {selected.observations.map((x) => (
                  <p key={x.id} className="rounded-lg border p-3 text-sm">
                    <Badge
                      tone={
                        x.status === "healthy"
                          ? "success"
                          : x.status === "degraded"
                            ? "warning"
                            : "neutral"
                      }
                    >
                      {x.status}
                    </Badge>{" "}
                    {x.managed ? x.resource_id : "unmanaged"} ·{" "}
                    {x.provider_resource} @ {x.observed_revision} · {x.summary}
                  </p>
                ))}
              </div>
            </Card>
          )}
        </>
      )}
      <InfrastructureExecutions repositoryID={repositoryID} />
      <section>
        <h2 className="font-semibold">Published definitions</h2>
        {items.length === 0 && (
          <p className="mt-2 text-sm text-[var(--muted)]">
            No infrastructure definition has been published.
          </p>
        )}
        {items.map((x) => (
          <button
            type="button"
            onClick={() => setSelected(x)}
            className="mt-2 w-full rounded-xl border bg-white p-4 text-left hover:border-[var(--brand)]"
            key={x.id}
          >
            <strong>{x.revisions.at(-1)?.title}</strong>
            <span className="block text-xs text-[var(--muted)]">
              v{x.current_version} · {x.revisions.at(-1)?.resources.length}{" "}
              resources · {x.diagnostics.length} explicit state notes
            </span>
          </button>
        ))}
      </section>
    </div>
  );
}
function Grid({ children }: { children: React.ReactNode }) {
  return <div className="grid gap-3 md:grid-cols-3">{children}</div>;
}
function Group({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <fieldset className="grid gap-3 rounded-lg border p-4 md:grid-cols-3">
      <legend className="px-2 text-sm font-semibold">{title}</legend>
      {children}
    </fieldset>
  );
}
function Field({
  n,
  l,
  v,
  type = "text",
  required = true,
}: {
  n: string;
  l: string;
  v?: string;
  type?: string;
  required?: boolean;
}) {
  return (
    <label className="text-xs font-semibold">
      {l}
      <input
        key={v}
        name={n}
        type={type}
        required={required}
        defaultValue={v}
        className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal"
      />
    </label>
  );
}
function Area({ n, l, v }: { n: string; l: string; v?: string }) {
  return (
    <label className="text-xs font-semibold md:col-span-3">
      {l}
      <textarea
        key={v}
        name={n}
        required
        defaultValue={v}
        className="mt-1 w-full rounded-lg border p-3 font-normal"
      />
    </label>
  );
}
function Select({
  n,
  l,
  v,
  options,
}: {
  n: string;
  l: string;
  v: string;
  options: string[];
}) {
  return (
    <label className="text-xs font-semibold">
      {l}
      <select
        name={n}
        defaultValue={v}
        className="mt-1 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
      >
        {options.map((x) => (
          <option key={x}>{x}</option>
        ))}
      </select>
    </label>
  );
}
