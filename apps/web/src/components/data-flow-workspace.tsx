"use client";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { useAuth } from "@/components/auth";
import { Badge, Button, Card } from "@/components/ui";
import { api } from "@/lib/api";

type Ref = { commitment_id: string; version: number; data_use_ids: string[] };
type Node = {
  id: string;
  kind: string;
  name: string;
  resource_id?: string;
  interface?: string;
  accessible: boolean;
  uncertainty?: string;
};
type Edge = {
  id: string;
  from: string;
  to: string;
  operation: string;
  data_categories: string[];
  purpose: string;
  retained_copy: boolean;
  commitment_refs: Ref[];
};
type Finding = {
  id: string;
  kind: string;
  severity: string;
  summary: string;
  node_ids?: string[];
  edge_ids?: string[];
  citations: {
    path: string;
    start_line?: number;
    end_line?: number;
    claim: string;
  }[];
  uncertainty?: string;
  added_by_type: string;
  added_by: string;
};
type Flow = {
  id: string;
  current_version: number;
  revisions: {
    version: number;
    code_revision: string;
    title: string;
    entry_points: string[];
    nodes: Node[];
    edges: Edge[];
    commitment_refs: Ref[];
    rationale: string;
  }[];
  analyses: {
    id: string;
    map_version: number;
    status: string;
    findings: Finding[];
    created_by_type: string;
    created_by: string;
  }[];
  diagnostics: {
    kind: string;
    severity: string;
    message: string;
    resource_id?: string;
  }[];
};
const lines = (v: FormDataEntryValue | null) =>
  String(v ?? "")
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
const csv = (v: string) =>
  v
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
const refs = (v: string): Ref[] => {
  const [commitment_id, version, uses] = v.split("|");
  return commitment_id
    ? [
        {
          commitment_id,
          version: Number(version),
          data_use_ids: csv(uses ?? ""),
        },
      ]
    : [];
};
export function DataFlowWorkspace({ repositoryID }: { repositoryID: string }) {
  const { token } = useAuth();
  const [flows, setFlows] = useState<Flow[]>([]);
  const [selected, setSelected] = useState<Flow>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const out = await api<{ data_flows: Flow[] }>(
        `/repositories/${repositoryID}/data-flows`,
        {},
        token,
      );
      setFlows(out.data_flows);
      setSelected(
        (x) => out.data_flows.find((f) => f.id === x?.id) ?? out.data_flows[0],
      );
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Data flows could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function publish(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    setError("");
    const d = new FormData(e.currentTarget);
    const commitmentRefs = refs(String(d.get("commitment")));
    const nodes = lines(d.get("nodes")).map((line) => {
      const [id, kind, name, resource_id, iface, accessible, uncertainty] =
        line.split("|");
      return {
        id,
        kind,
        name,
        resource_id,
        interface: iface,
        accessible: accessible !== "no",
        uncertainty,
      };
    });
    const edges = lines(d.get("edges")).map((line) => {
      const [id, from, to, operation, categories, purpose, retained] =
        line.split("|");
      return {
        id,
        from,
        to,
        operation,
        data_categories: csv(categories),
        purpose,
        retained_copy: retained === "yes",
        commitment_refs: commitmentRefs,
      };
    });
    const revision = {
      code_revision: d.get("revision"),
      title: d.get("title"),
      entry_points: csv(String(d.get("entry_points"))),
      nodes,
      edges,
      commitment_refs: commitmentRefs,
      rationale: d.get("rationale"),
    };
    const path = selected
      ? `/repositories/${repositoryID}/data-flows/${selected.id}/revisions`
      : `/repositories/${repositoryID}/data-flows`;
    try {
      const out = await api<Flow>(
        path,
        {
          method: "POST",
          body: JSON.stringify({
            revision,
            ...(selected ? { expected_version: selected.current_version } : {}),
          }),
        },
        token,
      );
      setSelected(out);
      setFlows((v) => [out, ...v.filter((x) => x.id !== out.id)]);
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Data-flow declaration could not be published.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function analyze(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    setBusy(true);
    setError("");
    const d = new FormData(e.currentTarget),
      r = selected.revisions.at(-1)!;
    const [path, start, end, claim] = String(d.get("citation")).split("|");
    try {
      const out = await api<Flow>(
        `/repositories/${repositoryID}/data-flows/${selected.id}/analyses`,
        {
          method: "POST",
          body: JSON.stringify({
            map_version: r.version,
            code_revision: r.code_revision,
            status: "completed",
            bounds: lines(d.get("bounds")),
            findings: [
              {
                kind: d.get("kind"),
                severity: d.get("severity"),
                summary: d.get("summary"),
                node_ids: csv(String(d.get("node_ids"))),
                edge_ids: csv(String(d.get("edge_ids"))),
                citations: [
                  {
                    path,
                    start_line: Number(start) || undefined,
                    end_line: Number(end) || undefined,
                    claim,
                  },
                ],
                uncertainty: d.get("uncertainty"),
              },
            ],
          }),
        },
        token,
      );
      setSelected(out);
      setFlows((v) => [out, ...v.filter((x) => x.id !== out.id)]);
      e.currentTarget.reset();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Finding could not be retained.",
      );
    } finally {
      setBusy(false);
    }
  }
  const current = selected?.revisions.at(-1);
  return (
    <section className="space-y-5 border-t pt-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold">
            Permission-aware data-flow map
          </h2>
          <p className="mt-1 max-w-3xl text-sm text-[var(--muted)]">
            Trace governed data from an exact interaction and code revision
            through interfaces, packages, retained stores, extensions, releases,
            environments, audiences, and external recipients. Evidence records
            references and claims—not restricted payloads.
          </p>
        </div>
        {selected && (
          <Button
            type="button"
            variant="secondary"
            onClick={() => setSelected(undefined)}
          >
            New map
          </Button>
        )}
      </header>
      <Card className="p-5">
        <h3 className="font-semibold">
          {selected
            ? "Publish a complete successor"
            : "Declare an observed path"}
        </h3>
        <form onSubmit={publish} className="mt-4 grid gap-3">
          <div className="grid gap-3 md:grid-cols-3">
            <Field n="title" l="Map title" v={current?.title} />
            <Field
              n="revision"
              l="Exact visible commit SHA"
              v={current?.code_revision}
            />
            <Field
              n="entry_points"
              l="Entry node IDs"
              v={current?.entry_points.join(", ")}
            />
          </div>
          <Field
            n="commitment"
            l="Applicable commitment: ID | version | data-use IDs"
            v={current?.commitment_refs
              .map(
                (x) =>
                  `${x.commitment_id}|${x.version}|${x.data_use_ids.join(",")}`,
              )
              .join("\n")}
          />
          <Area
            n="nodes"
            l="Nodes, one per line: ID | interaction/interface/package/store/extension/release/environment/audience/external_recipient | name | resource ID | interface | yes/no accessible | uncertainty"
            v={current?.nodes
              .map((x) =>
                [
                  x.id,
                  x.kind,
                  x.name,
                  x.resource_id,
                  x.interface,
                  x.accessible ? "yes" : "no",
                  x.uncertainty,
                ].join("|"),
              )
              .join("\n")}
          />
          <Area
            n="edges"
            l="Flows, one per line: ID | from | to | operation | data categories | purpose | yes/no retained copy"
            v={current?.edges
              .map((x) =>
                [
                  x.id,
                  x.from,
                  x.to,
                  x.operation,
                  x.data_categories.join(","),
                  x.purpose,
                  x.retained_copy ? "yes" : "no",
                ].join("|"),
              )
              .join("\n")}
          />
          <Field
            n="rationale"
            l="Declaration rationale"
            v={current?.rationale}
          />
          <div>
            <Button disabled={busy}>
              {busy
                ? "Publishing…"
                : selected
                  ? `Publish version ${selected.current_version + 1}`
                  : "Publish map"}
            </Button>
          </div>
        </form>
      </Card>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      {selected && current && (
        <>
          <Card className="p-5">
            <div className="flex flex-wrap gap-2">
              <h3 className="mr-2 font-semibold">{current.title}</h3>
              <Badge>v{current.version}</Badge>
              <Badge>{current.code_revision.slice(0, 12)}</Badge>
              <Badge
                tone={
                  selected.diagnostics.some((x) => x.severity === "blocking")
                    ? "danger"
                    : selected.diagnostics.length
                      ? "warning"
                      : "success"
                }
              >
                {selected.diagnostics.length} diagnostics
              </Badge>
            </div>
            <div className="mt-4 grid gap-3 md:grid-cols-3">
              {current.nodes.map((n) => (
                <article className="rounded-lg border p-3 text-sm" key={n.id}>
                  <Badge tone={n.accessible ? "neutral" : "warning"}>
                    {n.kind.replaceAll("_", " ")}
                  </Badge>
                  <p className="mt-2 font-semibold">{n.name}</p>
                  <p className="text-xs text-[var(--muted)]">
                    {n.interface || n.resource_id || n.id}
                  </p>
                  {n.uncertainty && (
                    <p className="mt-2 text-xs">Uncertain: {n.uncertainty}</p>
                  )}
                </article>
              ))}
            </div>
            {current.edges.map((e) => (
              <p
                className="mt-3 rounded-lg bg-[var(--surface-2)] p-3 text-sm"
                key={e.id}
              >
                <b>
                  {e.from} → {e.to}
                </b>{" "}
                · {e.operation} · {e.data_categories.join(", ")}
                {e.retained_copy && " · retained copy"}
              </p>
            ))}
            {selected.diagnostics.map((d, i) => (
              <p className="mt-3 text-sm" key={`${d.kind}-${i}`}>
                <Badge tone={d.severity === "blocking" ? "danger" : "warning"}>
                  {d.kind.replaceAll("_", " ")}
                </Badge>{" "}
                {d.message}
              </p>
            ))}
          </Card>
          <Card className="p-5">
            <h3 className="font-semibold">Add bounded analysis</h3>
            <p className="mt-1 text-xs text-[var(--muted)]">
              Humans and repository-bound read-only agents may retain code
              citations, differences, and uncertainty. Do not paste governed
              data values.
            </p>
            <form onSubmit={analyze} className="mt-4 grid gap-3 md:grid-cols-2">
              <Select
                n="kind"
                l="Finding"
                options={[
                  "confirmed",
                  "undeclared_flow",
                  "declared_observed_difference",
                  "inaccessible_dependency",
                  "uncertainty",
                ]}
              />
              <Select
                n="severity"
                l="Severity"
                options={["warning", "blocking", "info"]}
              />
              <Field
                n="node_ids"
                l="Node IDs (comma separated)"
                required={false}
              />
              <Field
                n="edge_ids"
                l="Edge IDs (comma separated)"
                required={false}
              />
              <Area n="summary" l="Finding summary (no restricted values)" />
              <Area n="bounds" l="Analysis bounds, one per line" />
              <Field
                n="citation"
                l="Citation: path | start line | end line | claim"
              />
              <Field n="uncertainty" l="Uncertainty" required={false} />
              <div>
                <Button disabled={busy}>Retain cited finding</Button>
              </div>
            </form>
            {selected.analyses.flatMap((a) =>
              a.findings.map((f) => (
                <article
                  className="mt-4 rounded-lg border p-3 text-sm"
                  key={f.id}
                >
                  <Badge
                    tone={
                      f.severity === "blocking"
                        ? "danger"
                        : f.severity === "warning"
                          ? "warning"
                          : "neutral"
                    }
                  >
                    {f.kind.replaceAll("_", " ")}
                  </Badge>{" "}
                  <b>{f.summary}</b>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    {f.added_by_type} {f.added_by} ·{" "}
                    {f.citations
                      .map((c) => `${c.path}:${c.start_line ?? "?"}`)
                      .join(", ")}
                  </p>
                </article>
              )),
            )}
          </Card>
        </>
      )}
      <div>
        <h3 className="font-semibold">Declared maps</h3>
        {flows.map((f) => (
          <button
            type="button"
            className="mt-2 w-full rounded-xl border bg-white p-3 text-left hover:border-[var(--brand)]"
            onClick={() => setSelected(f)}
            key={f.id}
          >
            <b>{f.revisions.at(-1)?.title}</b>
            <span className="block text-xs text-[var(--muted)]">
              v{f.current_version} · {f.revisions.at(-1)?.nodes.length} nodes ·{" "}
              {f.diagnostics.length} diagnostics
            </span>
          </button>
        ))}
      </div>
    </section>
  );
}
function Field({
  n,
  l,
  v,
  required = true,
}: {
  n: string;
  l: string;
  v?: string;
  required?: boolean;
}) {
  return (
    <label className="text-xs font-semibold">
      {l}
      <input
        name={n}
        required={required}
        defaultValue={v}
        className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal"
      />
    </label>
  );
}
function Area({ n, l, v }: { n: string; l: string; v?: string }) {
  return (
    <label className="text-xs font-semibold">
      {l}
      <textarea
        name={n}
        required
        defaultValue={v}
        rows={3}
        className="mt-1 w-full rounded-lg border p-3 font-normal"
      />
    </label>
  );
}
function Select({
  n,
  l,
  options,
}: {
  n: string;
  l: string;
  options: string[];
}) {
  return (
    <label className="text-xs font-semibold">
      {l}
      <select
        name={n}
        className="mt-1 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
      >
        {options.map((x) => (
          <option key={x}>{x}</option>
        ))}
      </select>
    </label>
  );
}
