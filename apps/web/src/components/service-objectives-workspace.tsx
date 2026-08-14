"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Diagnostic = {
  kind: string;
  severity: string;
  message: string;
  resource_id?: string;
  attributed_to: string;
};
type Revision = {
  version: number;
  title: string;
  summary: string;
  scopes: { kind: string; resource_id?: string; name: string }[];
  indicators: {
    id: string;
    name: string;
    description: string;
    signal?: string;
    calculation: string;
    unit: string;
    good_event: string;
    total_event: string;
  }[];
  objectives: {
    id: string;
    name: string;
    indicator_id: string;
    window_id: string;
    target: number;
    comparator: string;
    journey_ids: string[];
    owner_ids: string[];
  }[];
  measurement_windows: {
    id: string;
    name: string;
    duration: string;
    rolling: boolean;
  }[];
  user_journeys: {
    id: string;
    name: string;
    description: string;
    owner_ids: string[];
  }[];
  dependencies: {
    id: string;
    name: string;
    kind: string;
    resource_id?: string;
    owner_ids: string[];
    objective_ids: string[];
  }[];
  error_budgets: {
    objective_id: string;
    allowed_failure: number;
    unit: string;
    burn_policy: string;
  }[];
  severity_thresholds: {
    level: string;
    budget_consumed_percent: number;
    response: string;
    owner_ids: string[];
  }[];
  owner_ids: string[];
  commitment_links: { kind: string; id: string; version: number }[];
  exception_policy: {
    maximum_duration: string;
    approval_owner_ids: string[];
    follow_up_required: boolean;
  };
  exceptions: {
    id: string;
    objective_ids: string[];
    reason: string;
    approved_by: string;
    expires_at: string;
    follow_up?: string;
  }[];
  rationale: string;
  created_by: string;
  created_at: string;
};
type Contract = {
  id: string;
  current_version: number;
  revisions: Revision[];
  diagnostics: Diagnostic[];
  signal_mappings: {
    id: string;
    current_version: number;
    revisions: {
      version: number;
      contract_version: number;
      objective_id: string;
      instrumentation_revision: string;
      sources: { kind: string; name: string; reference: string; visibility: string; sanitization: string }[];
      calculation: string;
      unit: string;
      rationale: string;
      created_by: string;
      created_at: string;
    }[];
  }[];
  observations: {
    id: string;
    mapping_id: string;
    mapping_version: number;
    objective_id: string;
    window_start: string;
    window_end: string;
    attainment?: number;
    target_met?: boolean;
    error_budget_consumed_percent?: number;
    uncertainty: number;
    gaps: { kind: string; detail: string }[];
    software: { kind: string; id: string; revision: string; label: string }[];
    summary: string;
    comparable_to_previous: boolean;
    comparison_reason?: string;
    recorded_by: string;
  }[];
};
const value = (f: FormData, n: string) => String(f.get(n) ?? "").trim(),
  list = (v: string) =>
    v
      .split(",")
      .map((x) => x.trim())
      .filter(Boolean);

function buildNew(f: FormData) {
  const owners = list(value(f, "owners")),
    objective = "availability",
    journey = "critical-journey",
    indicator = "success-ratio",
    windowID = "rolling-window";
  const links = value(f, "links")
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean)
    .map((x) => {
      const [kind, id, version] = x.split(":");
      return { kind, id, version: Number(version) };
    });
  return {
    title: value(f, "title"),
    summary: value(f, "summary"),
    scopes: [
      {
        kind: value(f, "scope_kind"),
        resource_id: value(f, "scope_id") || undefined,
        name: value(f, "scope_name"),
      },
    ],
    indicators: [
      {
        id: indicator,
        name: value(f, "indicator_name"),
        description: value(f, "indicator_description"),
        signal: value(f, "signal") || undefined,
        calculation: value(f, "calculation"),
        unit: value(f, "unit"),
        good_event: value(f, "good_event"),
        total_event: value(f, "total_event"),
      },
    ],
    measurement_windows: [
      {
        id: windowID,
        name: value(f, "window_name"),
        duration: value(f, "duration"),
        rolling: value(f, "rolling") === "yes",
      },
    ],
    user_journeys: [
      {
        id: journey,
        name: value(f, "journey_name"),
        description: value(f, "journey_description"),
        owner_ids: owners,
      },
    ],
    objectives: [
      {
        id: objective,
        name: value(f, "objective_name"),
        indicator_id: indicator,
        window_id: windowID,
        target: Number(value(f, "target")),
        comparator: value(f, "comparator"),
        journey_ids: [journey],
        owner_ids: owners,
      },
    ],
    dependencies: value(f, "dependency_name")
      ? [
          {
            id: "primary-dependency",
            name: value(f, "dependency_name"),
            kind: value(f, "dependency_kind"),
            resource_id: value(f, "dependency_id") || undefined,
            owner_ids: list(value(f, "dependency_owners")),
            objective_ids: [objective],
          },
        ]
      : [],
    error_budgets: [
      {
        objective_id: objective,
        allowed_failure: Number(value(f, "budget")),
        unit: value(f, "budget_unit"),
        burn_policy: value(f, "burn_policy"),
      },
    ],
    severity_thresholds: [
      {
        level: "warning",
        budget_consumed_percent: Number(value(f, "warning")),
        response: value(f, "warning_response"),
        owner_ids: owners,
      },
      {
        level: "critical",
        budget_consumed_percent: Number(value(f, "critical")),
        response: value(f, "critical_response"),
        owner_ids: owners,
      },
    ],
    owner_ids: owners,
    commitment_links: links,
    exception_policy: {
      maximum_duration: value(f, "exception_duration"),
      approval_owner_ids: owners,
      follow_up_required: true,
    },
    exceptions: [],
    rationale: value(f, "rationale"),
  };
}

function build(f: FormData, current?: Revision) {
  const next = buildNew(f);
  if (!current) return next;
  const indicatorID = current.indicators[0].id,
    windowID = current.measurement_windows[0].id,
    journeyID = current.user_journeys[0].id,
    objectiveID = current.objectives[0].id;
  return {
    ...current,
    ...next,
    scopes: [
      { ...current.scopes[0], ...next.scopes[0] },
      ...current.scopes.slice(1),
    ],
    indicators: [
      { ...current.indicators[0], ...next.indicators[0], id: indicatorID },
      ...current.indicators.slice(1),
    ],
    measurement_windows: [
      {
        ...current.measurement_windows[0],
        ...next.measurement_windows[0],
        id: windowID,
      },
      ...current.measurement_windows.slice(1),
    ],
    user_journeys: [
      { ...current.user_journeys[0], ...next.user_journeys[0], id: journeyID },
      ...current.user_journeys.slice(1),
    ],
    objectives: [
      {
        ...current.objectives[0],
        ...next.objectives[0],
        id: objectiveID,
        indicator_id: indicatorID,
        window_id: windowID,
        journey_ids: current.objectives[0].journey_ids,
      },
      ...current.objectives.slice(1),
    ],
    dependencies: next.dependencies.length
      ? [
          {
            ...current.dependencies[0],
            ...next.dependencies[0],
            id: current.dependencies[0]?.id ?? next.dependencies[0].id,
            objective_ids: current.dependencies[0]?.objective_ids ?? [
              objectiveID,
            ],
          },
          ...current.dependencies.slice(1),
        ]
      : current.dependencies,
    error_budgets: [
      {
        ...current.error_budgets[0],
        ...next.error_budgets[0],
        objective_id: objectiveID,
      },
      ...current.error_budgets.slice(1),
    ],
    severity_thresholds: [
      ...next.severity_thresholds.map((x, i) => ({
        ...current.severity_thresholds[i],
        ...x,
      })),
      ...current.severity_thresholds.slice(next.severity_thresholds.length),
    ],
    exception_policy: { ...current.exception_policy, ...next.exception_policy },
    exceptions: current.exceptions,
  };
}

export function ServiceObjectivesWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth(),
    [items, setItems] = useState<Contract[]>([]),
    [selected, setSelected] = useState<Contract>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const out = await api<{ service_objectives: Contract[] }>(
        `/repositories/${repositoryID}/service-objectives`,
        {},
        token,
      );
      setItems(out.service_objectives);
      setSelected(
        (x) =>
          out.service_objectives.find((v) => v.id === x?.id) ??
          out.service_objectives[0],
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Service objectives could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  const current = selected?.revisions.at(-1);
  async function publish(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    setError("");
    const body = {
        revision: build(new FormData(e.currentTarget), current),
        ...(selected ? { expected_version: selected.current_version } : {}),
      },
      path = selected
        ? `/repositories/${repositoryID}/service-objectives/${selected.id}/revisions`
        : `/repositories/${repositoryID}/service-objectives`;
    try {
      const out = await api<Contract>(
        path,
        { method: "POST", body: JSON.stringify(body) },
        token,
      );
      setItems((xs) => [out, ...xs.filter((x) => x.id !== out.id)]);
      setSelected(out);
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "The reliability contract could not be published.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function publishMapping(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); if (!token || !selected || !current) return;
    const f = new FormData(e.currentTarget), existing = selected.signal_mappings[0], revision = {
      contract_version: current.version,
      objective_id: current.objectives[0].id,
      instrumentation_revision: value(f,"instrumentation_revision"),
      calculation: current.indicators[0].calculation,
      unit: current.indicators[0].unit,
      rationale: value(f,"mapping_rationale"),
      sources: value(f,"sources").split(";").map((entry) => { const [kind,name,reference,visibility,sanitization]=entry.split("|").map((x)=>x.trim()); return {kind,name,reference,visibility,sanitization}; }),
    };
    const path = existing ? `/repositories/${repositoryID}/service-objectives/${selected.id}/signal-mappings/${existing.id}/revisions` : `/repositories/${repositoryID}/service-objectives/${selected.id}/signal-mappings`;
    try { const out=await api<Contract>(path,{method:"POST",body:JSON.stringify({revision,...(existing?{expected_version:existing.current_version}:{})})},token); setSelected(out); setItems((xs)=>[out,...xs.filter((x)=>x.id!==out.id)]); setError(""); }
    catch(x){setError(x instanceof Error?x.message:"Signal mapping could not be published.");}
  }
  async function recordObservation(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); if (!token || !selected || !current || !selected.signal_mappings[0]) return;
    const f=new FormData(e.currentTarget), mapping=selected.signal_mappings[0], software=value(f,"software").split(";").filter(Boolean).map((entry)=>{const [kind,id,revision,label]=entry.split("|").map((x)=>x.trim());return {kind,id,revision,label};}), gaps=value(f,"gaps").split(";").filter(Boolean).map((entry)=>{const [kind,detail]=entry.split("|").map((x)=>x.trim());return {kind,detail};});
    const observation={mapping_id:mapping.id,mapping_version:mapping.current_version,contract_version:current.version,objective_id:current.objectives[0].id,window_start:new Date(value(f,"window_start")).toISOString(),window_end:new Date(value(f,"window_end")).toISOString(),good_events:Number(value(f,"good_events")),total_events:Number(value(f,"total_events")),uncertainty:Number(value(f,"uncertainty")),summary:value(f,"evidence_summary"),software,gaps};
    try { const out=await api<Contract>(`/repositories/${repositoryID}/service-objectives/${selected.id}/observations`,{method:"POST",body:JSON.stringify({observation})},token); setSelected(out); setItems((xs)=>[out,...xs.filter((x)=>x.id!==out.id)]); setError(""); }
    catch(x){setError(x instanceof Error?x.message:"Reliability observation could not be recorded.");}
  }
  return (
    <div className="space-y-8">
      <header>
        <p className="text-sm font-semibold text-[var(--brand)]">
          Reliability stewardship
        </p>
        <h1 className="text-3xl font-semibold">Service objectives</h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Make user-visible dependability, measurement, error-budget response,
          dependencies, and ownership a shared versioned contract—not private
          operations judgment.
        </p>
      </header>
      <Card className="p-5">
        <div className="flex justify-between gap-3">
          <h2 className="font-semibold">
            {selected
              ? "Publish a complete successor"
              : "Define a reliability contract"}
          </h2>
          {selected && (
            <Button
              type="button"
              variant="secondary"
              onClick={() => setSelected(undefined)}
            >
              New contract
            </Button>
          )}
        </div>
        <form
          key={selected?.id ?? "new"}
          onSubmit={publish}
          className="mt-4 grid gap-4 md:grid-cols-2"
        >
          <Field n="title" l="Contract title" v={current?.title} />
          <Field
            n="owners"
            l="Accountable collaborator IDs"
            v={current?.owner_ids.join(", ") ?? user?.id}
          />
          <Area n="summary" l="User reliability promise" v={current?.summary} />
          <Area n="rationale" l="Revision rationale" v={current?.rationale} />
          <Select
            n="scope_kind"
            l="Scope"
            v={current?.scopes[0]?.kind ?? "repository"}
            options={["repository", "release", "environment"]}
          />
          <Field
            n="scope_id"
            l="Release or environment ID"
            v={current?.scopes[0]?.resource_id}
            required={false}
          />
          <Field
            n="scope_name"
            l="Scoped service"
            v={current?.scopes[0]?.name ?? "Production service"}
          />
          <Field
            n="journey_name"
            l="User journey"
            v={
              current?.user_journeys[0]?.name ?? "Complete the critical journey"
            }
          />
          <Area
            n="journey_description"
            l="Dependable behavior"
            v={current?.user_journeys[0]?.description}
          />
          <Field
            n="indicator_name"
            l="Service indicator"
            v={current?.indicators[0]?.name ?? "Successful journey ratio"}
          />
          <Area
            n="indicator_description"
            l="Indicator meaning"
            v={current?.indicators[0]?.description}
          />
          <Field
            n="signal"
            l="Measurement signal (blank remains explicit)"
            v={current?.indicators[0]?.signal}
            required={false}
          />
          <Select
            n="calculation"
            l="Calculation"
            v={current?.indicators[0]?.calculation ?? "ratio"}
            options={[
              "ratio",
              "availability",
              "latency_percentile",
              "count",
              "custom",
            ]}
          />
          <Field
            n="unit"
            l="Indicator unit"
            v={current?.indicators[0]?.unit ?? "percent"}
          />
          <Field
            n="good_event"
            l="Good event definition"
            v={current?.indicators[0]?.good_event ?? "Journey succeeds"}
          />
          <Field
            n="total_event"
            l="Total event definition"
            v={current?.indicators[0]?.total_event ?? "Eligible journey starts"}
          />
          <Field
            n="objective_name"
            l="Objective"
            v={current?.objectives[0]?.name ?? "Critical journey availability"}
          />
          <Field
            n="target"
            l="Target"
            type="number"
            step="any"
            v={String(current?.objectives[0]?.target ?? 99.9)}
          />
          <Select
            n="comparator"
            l="Target comparison"
            v={current?.objectives[0]?.comparator ?? "at_least"}
            options={["at_least", "at_most"]}
          />
          <Field
            n="window_name"
            l="Measurement window"
            v={current?.measurement_windows[0]?.name ?? "Rolling 30 days"}
          />
          <Field
            n="duration"
            l="Window duration"
            v={current?.measurement_windows[0]?.duration ?? "720h"}
          />
          <Select
            n="rolling"
            l="Window behavior"
            v={
              current?.measurement_windows[0]?.rolling === false ? "no" : "yes"
            }
            options={["yes", "no"]}
          />
          <Field
            n="budget"
            l="Allowed failure"
            type="number"
            step="any"
            v={String(current?.error_budgets[0]?.allowed_failure ?? 0.1)}
          />
          <Field
            n="budget_unit"
            l="Budget unit"
            v={current?.error_budgets[0]?.unit ?? "percent"}
          />
          <Area
            n="burn_policy"
            l="Error-budget action policy"
            v={
              current?.error_budgets[0]?.burn_policy ??
              "Slow delivery and require owner review when burn is elevated."
            }
          />
          <Field
            n="warning"
            l="Warning at budget consumed %"
            type="number"
            v={String(
              current?.severity_thresholds[0]?.budget_consumed_percent ?? 50,
            )}
          />
          <Area
            n="warning_response"
            l="Warning response"
            v={
              current?.severity_thresholds[0]?.response ??
              "Objective owner investigates."
            }
          />
          <Field
            n="critical"
            l="Critical at budget consumed %"
            type="number"
            v={String(
              current?.severity_thresholds[1]?.budget_consumed_percent ?? 100,
            )}
          />
          <Area
            n="critical_response"
            l="Critical response"
            v={
              current?.severity_thresholds[1]?.response ??
              "Pause affected delivery and involve service owners."
            }
          />
          <Field
            n="dependency_name"
            l="Dependency name (optional)"
            v={current?.dependencies[0]?.name}
            required={false}
          />
          <Select
            n="dependency_kind"
            l="Dependency kind"
            v={current?.dependencies[0]?.kind ?? "service"}
            options={["repository", "service", "external"]}
          />
          <Field
            n="dependency_id"
            l="Dependency resource ID"
            v={current?.dependencies[0]?.resource_id}
            required={false}
          />
          <Field
            n="dependency_owners"
            l="Dependency owner IDs"
            v={current?.dependencies[0]?.owner_ids.join(", ")}
            required={false}
          />
          <Field
            n="links"
            l="Commitments (kind:id:version, comma separated)"
            v={current?.commitment_links
              .map((x) => `${x.kind}:${x.id}:${x.version}`)
              .join(", ")}
            required={false}
          />
          <Field
            n="exception_duration"
            l="Maximum exception duration"
            v={current?.exception_policy.maximum_duration ?? "168h"}
          />
          <div className="md:col-span-2">
            <Button disabled={busy}>
              {busy
                ? "Publishing…"
                : selected
                  ? `Publish version ${selected.current_version + 1}`
                  : "Create service objective"}
            </Button>
          </div>
        </form>
      </Card>
      {selected && current && <Card className="p-5">
        <h2 className="font-semibold">Operational evidence mapping</h2>
        <p className="mt-1 text-sm text-[var(--muted)]">Connect sanitized observability and delivery sources to {current.objectives[0]?.name}. Prior instrumentation stays visible.</p>
        <form onSubmit={publishMapping} className="mt-4 grid gap-4 md:grid-cols-2">
          <Field n="instrumentation_revision" l="Instrumentation revision" v={selected.signal_mappings[0]?.revisions.at(-1)?.instrumentation_revision}/>
          <Area n="mapping_rationale" l="Mapping rationale" v={selected.signal_mappings[0]?.revisions.at(-1)?.rationale}/>
          <div className="md:col-span-2"><Area n="sources" l="Sources: kind | name | sanitized reference | public/participants | sanitization (semicolon separated)" v={selected.signal_mappings[0]?.revisions.at(-1)?.sources.map((x)=>`${x.kind} | ${x.name} | ${x.reference} | ${x.visibility} | ${x.sanitization}`).join("; ")}/></div>
          <div className="md:col-span-2"><Button>{selected.signal_mappings[0]?`Publish mapping v${selected.signal_mappings[0].current_version+1}`:"Publish signal mapping"}</Button></div>
        </form>
        {selected.signal_mappings[0] && <form onSubmit={recordObservation} className="mt-6 grid gap-4 border-t pt-5 md:grid-cols-2">
          <Field n="window_start" l="Window start" type="datetime-local"/><Field n="window_end" l="Window end" type="datetime-local"/>
          <Field n="good_events" l="Good events" type="number" step="any"/><Field n="total_events" l="Total events" type="number" step="any"/>
          <Field n="uncertainty" l="Uncertainty %" type="number" step="any" v="0"/><Area n="evidence_summary" l="Sanitized evidence summary"/>
          <div className="md:col-span-2"><Area n="software" l="Delivered software: kind | id | exact revision | label (semicolon separated)"/></div>
          <div className="md:col-span-2"><Area n="gaps" l="Evidence gaps: kind | detail (semicolon separated)"/></div>
          <div className="md:col-span-2"><Button>Record evidence window</Button></div>
        </form>}
      </Card>}
      {selected?.observations.length ? <section><h2 className="text-lg font-semibold">Attainment history</h2><div className="mt-3 space-y-3">{[...selected.observations].reverse().map((o)=><Card key={o.id} className="p-4">
        <div className="flex flex-wrap gap-2"><strong>{o.attainment?.toFixed(3) ?? "Unavailable"}% attainment</strong><Badge tone={o.target_met?"success":"danger"}>{o.target_met?"target met":"target missed"}</Badge><Badge>{o.error_budget_consumed_percent?.toFixed(1) ?? "—"}% budget consumed</Badge>{!o.comparable_to_previous&&<Badge tone="warning">incomparable window</Badge>}</div>
        <p className="mt-2 text-sm">{o.summary}</p><p className="mt-1 text-xs text-[var(--muted)]">{new Date(o.window_start).toLocaleString()} – {new Date(o.window_end).toLocaleString()} · uncertainty {o.uncertainty}% · mapping v{o.mapping_version} · recorded by {o.recorded_by}</p>
        {o.comparison_reason&&<p className="mt-1 text-xs text-[var(--muted)]">{o.comparison_reason}</p>}
        <p className="mt-2 text-xs text-[var(--muted)]">Provenance: {o.software.map((x)=>`${x.kind} ${x.label} @ ${x.revision}`).join(" · ")||"No delivered-software references"}</p>
        {o.gaps.map((x)=><p key={`${x.kind}-${x.detail}`} className="mt-1 text-xs text-[var(--warning)]">Gap: {x.kind.replaceAll("_"," ")} — {x.detail}</p>)}
      </Card>)}</div></section>:null}
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      <section>
        <h2 className="text-lg font-semibold">Published contracts</h2>
        <div className="mt-3 space-y-3">
          {items.map((x) => {
            const r = x.revisions.at(-1)!;
            return (
              <button
                type="button"
                key={x.id}
                onClick={() => setSelected(x)}
                className="w-full rounded-xl border bg-white p-4 text-left hover:border-[var(--brand)]"
              >
                <span className="flex flex-wrap items-center gap-2">
                  <strong>{r.title}</strong>
                  <Badge>v{x.current_version}</Badge>
                  {x.diagnostics.map((d, i) => (
                    <Badge
                      key={`${d.kind}-${i}`}
                      tone={d.severity === "blocking" ? "danger" : "warning"}
                    >
                      {d.kind.replaceAll("_", " ")}
                    </Badge>
                  ))}
                </span>
                <span className="mt-2 block text-sm">{r.summary}</span>
                <span className="mt-2 block text-xs text-[var(--muted)]">
                  {r.objectives.length} objective(s) · {r.user_journeys.length}{" "}
                  journey(s) · {r.dependencies.length} dependency(ies) ·
                  authored by {r.created_by}
                </span>
                {x.diagnostics.map((d, i) => (
                  <span
                    key={i}
                    className="mt-1 block text-xs text-[var(--muted)]"
                  >
                    {d.message} Attributed to {d.attributed_to}.
                  </span>
                ))}
              </button>
            );
          })}
          {!items.length && (
            <Card className="p-4 text-sm text-[var(--muted)]">
              No shared reliability contract has been published.
            </Card>
          )}
        </div>
      </section>
    </div>
  );
}
function Field({
  n,
  l,
  v = "",
  required = true,
  type = "text",
  step,
}: {
  n: string;
  l: string;
  v?: string;
  required?: boolean;
  type?: string;
  step?: string;
}) {
  return (
    <label className="text-sm font-medium">
      {l}
      <input
        name={n}
        defaultValue={v}
        required={required}
        type={type}
        step={step}
        className="mt-1 w-full rounded-lg border bg-transparent px-3 py-2 font-normal"
      />
    </label>
  );
}
function Area({ n, l, v = "" }: { n: string; l: string; v?: string }) {
  return (
    <label className="text-sm font-medium">
      {l}
      <textarea
        name={n}
        defaultValue={v}
        required
        rows={2}
        className="mt-1 w-full rounded-lg border bg-transparent px-3 py-2 font-normal"
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
    <label className="text-sm font-medium">
      {l}
      <select
        name={n}
        defaultValue={v}
        className="mt-1 w-full rounded-lg border bg-transparent px-3 py-2 font-normal"
      >
        {options.map((x) => (
          <option key={x} value={x}>
            {x.replaceAll("_", " ")}
          </option>
        ))}
      </select>
    </label>
  );
}
