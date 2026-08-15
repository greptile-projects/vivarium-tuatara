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
      sources: {
        kind: string;
        name: string;
        reference: string;
        visibility: string;
        sanitization: string;
      }[];
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
    observed_value?: number;
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
  investigations: {
    id: string; version: number; contract_version: number; objective_id: string;
    title: string; trigger: { kind: string; id: string; revision: string };
    baseline_observation_ids: string[]; affected_observation_ids: string[]; journey_ids: string[];
    evidence: { kind: string; resource_id: string; revision: string; label: string; visibility: string }[];
    findings: { id: string; kind: string; statement: string; uncertainty: string; confidence: string; citation_ids: string[]; created_by: string; actor_type: string }[];
    responses: { id: string; finding_id: string; kind: string; body: string; created_by: string }[];
    input_requests: { id: string; owner_id: string; dependency_id?: string; question: string; status: string; response?: string }[];
    outcome?: { kind: string; resource_id: string; summary: string };
    status: string; stale_evidence_ids: string[]; hidden_dependency_ids: string[]; inconclusive: boolean;
  }[];
  delivery_policies: { id:string; version:number; contract_version:number; objective_ids:string[]; branches:string[]; services:string[]; environment_ids:string[]; journey_ids:string[]; risk_classes:string[]; maximum_budget_consumed_percent:number; maximum_predicted_budget_increase_percent:number; required_owner_ids:string[]; minimum_acknowledgements:number; on_missing_evidence:string; on_budget_exhausted:string; on_regression:string; on_dependency_failure:string; rationale:string }[];
  reliability_impacts: { id:string; policy_id:string; kind:string; resource_id:string; revision:string; summary:string; objective_impacts:{objective_id:string;predicted_budget_increase_percent:number;observed_budget_consumed_percent?:number;confidence:string}[]; dependency_failures:string[]; owner_acknowledgements:{owner_id:string;rationale:string}[]; active_exception?:{reason:string;approved_by:string;expires_at:string;follow_up:string} }[];
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
            ...current.dependencies?.[0],
            ...next.dependencies[0],
            id: current.dependencies?.[0]?.id ?? next.dependencies[0].id,
            objective_ids: current.dependencies?.[0]?.objective_ids ?? [
              objectiveID,
            ],
          },
          ...(current.dependencies?.slice(1) ?? []),
        ]
      : (current.dependencies ?? []),
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
    [mappingID, setMappingID] = useState(""),
    [objectiveID, setObjectiveID] = useState(""),
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
  const current = selected?.revisions.at(-1),
    selectedMapping =
      mappingID === "__new"
        ? undefined
        : (selected?.signal_mappings.find((x) => x.id === mappingID) ??
          selected?.signal_mappings[0]),
    selectedMappingRevision = selectedMapping?.revisions.at(-1),
    selectedObjective =
      current?.objectives.find(
        (x) => x.id === (objectiveID || selectedMappingRevision?.objective_id),
      ) ?? current?.objectives[0],
    selectedIndicator = current?.indicators.find(
      (x) => x.id === selectedObjective?.indicator_id,
    );
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
    e.preventDefault();
    if (!token || !selected || !current) return;
    const f = new FormData(e.currentTarget),
      existing = selectedMapping,
      revision = {
        contract_version: current.version,
        objective_id: selectedObjective?.id,
        instrumentation_revision: value(f, "instrumentation_revision"),
        calculation: selectedIndicator?.calculation,
        unit: selectedIndicator?.unit,
        rationale: value(f, "mapping_rationale"),
        sources: value(f, "sources")
          .split(";")
          .map((entry) => {
            const [kind, name, reference, visibility, sanitization] = entry
              .split("|")
              .map((x) => x.trim());
            return { kind, name, reference, visibility, sanitization };
          }),
      };
    const path = existing
      ? `/repositories/${repositoryID}/service-objectives/${selected.id}/signal-mappings/${existing.id}/revisions`
      : `/repositories/${repositoryID}/service-objectives/${selected.id}/signal-mappings`;
    try {
      const out = await api<Contract>(
        path,
        {
          method: "POST",
          body: JSON.stringify({
            revision,
            ...(existing ? { expected_version: existing.current_version } : {}),
          }),
        },
        token,
      );
      setSelected(out);
      setMappingID(existing?.id ?? out.signal_mappings.at(-1)?.id ?? "");
      setItems((xs) => [out, ...xs.filter((x) => x.id !== out.id)]);
      setError("");
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Signal mapping could not be published.",
      );
    }
  }
  async function recordObservation(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (
      !token ||
      !selected ||
      !current ||
      !selectedMapping ||
      !selectedMappingRevision
    )
      return;
    const f = new FormData(e.currentTarget),
      software = value(f, "software")
        .split(";")
        .filter(Boolean)
        .map((entry) => {
          const [kind, id, revision, label] = entry
            .split("|")
            .map((x) => x.trim());
          return { kind, id, revision, label };
        }),
      gaps = value(f, "gaps")
        .split(";")
        .filter(Boolean)
        .map((entry) => {
          const [kind, detail] = entry.split("|").map((x) => x.trim());
          return { kind, detail };
        }),
      observedValue = value(f, "observed_value");
    const observation = {
      mapping_id: selectedMapping.id,
      mapping_version: selectedMapping.current_version,
      contract_version: selectedMappingRevision.contract_version,
      objective_id: selectedMappingRevision.objective_id,
      window_start: new Date(value(f, "window_start")).toISOString(),
      window_end: new Date(value(f, "window_end")).toISOString(),
      good_events: Number(value(f, "good_events")),
      total_events: Number(value(f, "total_events")),
      ...(observedValue ? { observed_value: Number(observedValue) } : {}),
      uncertainty: Number(value(f, "uncertainty")),
      summary: value(f, "evidence_summary"),
      software,
      gaps,
    };
    try {
      const out = await api<Contract>(
        `/repositories/${repositoryID}/service-objectives/${selected.id}/observations`,
        { method: "POST", body: JSON.stringify({ observation }) },
        token,
      );
      setSelected(out);
      setItems((xs) => [out, ...xs.filter((x) => x.id !== out.id)]);
      setError("");
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Reliability observation could not be recorded.",
      );
    }
  }
  async function openInvestigation(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); if(!token||!selected||!current||!selectedObjective)return; const f=new FormData(e.currentTarget);
    const parseEvidence=(raw:string)=>raw.split(";").filter(Boolean).map((entry)=>{const [kind,resource_id,revision,label,visibility]=entry.split("|").map((x)=>x.trim());return {kind,resource_id,revision,label,visibility}});
    try{const out=await api<Contract>(`/repositories/${repositoryID}/service-objectives/${selected.id}/investigations`,{method:"POST",body:JSON.stringify({contract_version:current.version,objective_id:selectedObjective.id,title:value(f,"investigation_title"),trigger:{kind:value(f,"trigger_kind"),id:value(f,"trigger_id"),revision:value(f,"trigger_revision")},baseline_observation_ids:list(value(f,"baseline_ids")),affected_observation_ids:list(value(f,"affected_ids")),journey_ids:selectedObjective.journey_ids,evidence:parseEvidence(value(f,"investigation_evidence"))})},token);setSelected(out);setItems((xs)=>[out,...xs.filter((x)=>x.id!==out.id)]);setError("");e.currentTarget.reset()}catch(x){setError(x instanceof Error?x.message:"Investigation could not be opened.")}
  }
  async function addFinding(e: FormEvent<HTMLFormElement>, investigation: Contract["investigations"][number]) {
    e.preventDefault();if(!token||!selected)return;const f=new FormData(e.currentTarget);try{const out=await api<Contract>(`/repositories/${repositoryID}/service-objectives/${selected.id}/investigations/${investigation.id}/findings`,{method:"POST",body:JSON.stringify({expected_version:investigation.version,finding:{kind:value(f,"kind"),statement:value(f,"statement"),uncertainty:value(f,"finding_uncertainty"),confidence:value(f,"confidence"),citation_ids:list(value(f,"citations"))}})},token);setSelected(out);setItems((xs)=>[out,...xs.filter((x)=>x.id!==out.id)]);setError("")}catch(x){setError(x instanceof Error?x.message:"Finding could not be recorded.")}
  }
  async function mutateInvestigation(e: FormEvent<HTMLFormElement>, investigation: Contract["investigations"][number], action: "responses"|"input-requests"|"input-responses"|"outcomes", payload: (f:FormData)=>object) {
    e.preventDefault();if(!token||!selected)return;const f=new FormData(e.currentTarget);try{const out=await api<Contract>(`/repositories/${repositoryID}/service-objectives/${selected.id}/investigations/${investigation.id}/${action}`,{method:"POST",body:JSON.stringify({expected_version:investigation.version,...payload(f)})},token);setSelected(out);setItems((xs)=>[out,...xs.filter((x)=>x.id!==out.id)]);setError("")}catch(x){setError(x instanceof Error?x.message:"Investigation contribution could not be recorded.")}
  }
  async function publishDeliveryPolicy(e:FormEvent<HTMLFormElement>){e.preventDefault();if(!token||!selected||!current)return;const f=new FormData(e.currentTarget),action=(n:string)=>value(f,n);try{const out=await api<Contract>(`/repositories/${repositoryID}/service-objectives/${selected.id}/delivery-policies`,{method:"POST",body:JSON.stringify({expected_version:selected.current_version,policy:{contract_version:current.version,objective_ids:list(value(f,"policy_objectives")),branches:list(value(f,"policy_branches")),services:list(value(f,"policy_services")),environment_ids:list(value(f,"policy_environments")),journey_ids:list(value(f,"policy_journeys")),risk_classes:list(value(f,"policy_risks")),maximum_budget_consumed_percent:Number(value(f,"maximum_budget")),maximum_predicted_budget_increase_percent:Number(value(f,"maximum_predicted")),require_current_evidence:true,require_dependencies:true,required_owner_ids:list(value(f,"policy_owners")),minimum_acknowledgements:Number(value(f,"minimum_acknowledgements")),on_missing_evidence:action("missing_action"),on_budget_exhausted:action("budget_action"),on_regression:action("regression_action"),on_dependency_failure:action("dependency_action"),rationale:value(f,"policy_rationale")}})},token);setSelected(out);setItems((xs)=>[out,...xs.filter((x)=>x.id!==out.id)]);setError("")}catch(x){setError(x instanceof Error?x.message:"Reliability delivery policy could not be published.")}}
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
            v={current?.dependencies?.[0]?.name}
            required={false}
          />
          <Select
            n="dependency_kind"
            l="Dependency kind"
            v={current?.dependencies?.[0]?.kind ?? "service"}
            options={["repository", "service", "external"]}
          />
          <Field
            n="dependency_id"
            l="Dependency resource ID"
            v={current?.dependencies?.[0]?.resource_id}
            required={false}
          />
          <Field
            n="dependency_owners"
            l="Dependency owner IDs"
            v={current?.dependencies?.[0]?.owner_ids.join(", ")}
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
      {selected && current && (
        <Card className="p-5">
          <h2 className="font-semibold">Operational evidence mapping</h2>
          <p className="mt-1 text-sm text-[var(--muted)]">
            Connect sanitized observability and delivery sources to a selected
            objective. Prior instrumentation stays visible.
          </p>
          <div className="mt-4 grid gap-4 md:grid-cols-2">
            <label className="text-sm font-medium">
              Objective
              <select
                value={selectedObjective?.id ?? ""}
                onChange={(e) => {
                  const id = e.target.value;
                  setObjectiveID(id);
                  setMappingID(
                    selected.signal_mappings.find(
                      (m) => m.revisions.at(-1)?.objective_id === id,
                    )?.id ?? "__new",
                  );
                }}
                className="mt-1 w-full rounded-lg border bg-transparent px-3 py-2 font-normal"
              >
                {current.objectives.map((x) => (
                  <option key={x.id} value={x.id}>
                    {x.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-sm font-medium">
              Signal mapping
              <select
                value={selectedMapping?.id ?? ""}
                onChange={(e) => {
                  const id = e.target.value;
                  setMappingID(id || "__new");
                  const objective = selected.signal_mappings
                    .find((m) => m.id === id)
                    ?.revisions.at(-1)?.objective_id;
                  if (objective) setObjectiveID(objective);
                }}
                className="mt-1 w-full rounded-lg border bg-transparent px-3 py-2 font-normal"
              >
                <option value="">New mapping</option>
                {selected.signal_mappings.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.revisions.at(-1)?.instrumentation_revision} · v
                    {m.current_version}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <form
            key={`${selectedObjective?.id}-${selectedMapping?.id ?? "new"}`}
            onSubmit={publishMapping}
            className="mt-4 grid gap-4 md:grid-cols-2"
          >
            <Field
              n="instrumentation_revision"
              l="Instrumentation revision"
              v={selectedMappingRevision?.instrumentation_revision}
            />
            <Area
              n="mapping_rationale"
              l="Mapping rationale"
              v={selectedMappingRevision?.rationale}
            />
            <div className="md:col-span-2">
              <Area
                n="sources"
                l="Sources: kind | name | sanitized reference | public/participants | sanitization (semicolon separated)"
                v={selectedMappingRevision?.sources
                  .map(
                    (x) =>
                      `${x.kind} | ${x.name} | ${x.reference} | ${x.visibility} | ${x.sanitization}`,
                  )
                  .join("; ")}
              />
            </div>
            <div className="md:col-span-2">
              <Button>
                {selectedMapping
                  ? `Publish mapping v${selectedMapping.current_version + 1}`
                  : "Publish signal mapping"}
              </Button>
            </div>
          </form>
          {selectedMapping && (
            <form
              key={`observation-${selectedMapping.id}`}
              onSubmit={recordObservation}
              className="mt-6 grid gap-4 border-t pt-5 md:grid-cols-2"
            >
              <Field n="window_start" l="Window start" type="datetime-local" />
              <Field n="window_end" l="Window end" type="datetime-local" />
              <Field n="good_events" l="Good events" type="number" step="any" />
              <Field
                n="total_events"
                l="Total events"
                type="number"
                step="any"
              />
              <Field
                n="observed_value"
                l="Native observed value (latency/custom)"
                type="number"
                step="any"
                required={false}
              />
              <Field
                n="uncertainty"
                l="Uncertainty %"
                type="number"
                step="any"
                v="0"
              />
              <Area n="evidence_summary" l="Sanitized evidence summary" />
              <div className="md:col-span-2">
                <Area
                  n="software"
                  l="Delivered software: kind | id | exact revision | label (semicolon separated)"
                />
              </div>
              <div className="md:col-span-2">
                <Area
                  n="gaps"
                  l="Evidence gaps: kind | detail (semicolon separated)"
                />
              </div>
              <div className="md:col-span-2">
                <Button>Record evidence window</Button>
              </div>
            </form>
          )}
        </Card>
      )}
      {selected && current && <Card className="p-5"><h2 className="font-semibold">Reliability delivery policy</h2><p className="mt-1 text-sm text-[var(--muted)]">Apply exact objectives and error-budget evidence to pulls, integration queues, releases, and staged deployments. Effects coordinate delivery; they never grant merge or environment authority.</p><form onSubmit={publishDeliveryPolicy} className="mt-4 grid gap-4 md:grid-cols-2"><Field n="policy_objectives" l="Objective IDs" v={current.objectives.map((x)=>x.id).join(", ")}/><Field n="policy_owners" l="Required reliability owner IDs" v={current.owner_ids.join(", ")}/><Field n="policy_branches" l="Branches" v="main"/><Field n="policy_services" l="Services" required={false}/><Field n="policy_environments" l="Environment IDs" required={false}/><Field n="policy_journeys" l="Journey IDs" v={current.user_journeys.map((x)=>x.id).join(", ")}/><Field n="policy_risks" l="Risk classes" required={false}/><Field n="minimum_acknowledgements" l="Minimum owner acknowledgements" type="number" v="1"/><Field n="maximum_budget" l="Maximum budget consumed %" type="number" step="any" v="100"/><Field n="maximum_predicted" l="Maximum predicted increase %" type="number" step="any" v="10"/><Select n="missing_action" l="Missing evidence" v="block" options={["warn","slow","block"]}/><Select n="budget_action" l="Exhausted budget" v="pause" options={["block","slow","pause","rollback"]}/><Select n="regression_action" l="Regression" v="slow" options={["warn","slow","block","pause"]}/><Select n="dependency_action" l="Dependency failure" v="pause" options={["warn","slow","block","pause","rollback"]}/><div className="md:col-span-2"><Area n="policy_rationale" l="Policy rationale and user impact"/><Button>Publish delivery policy</Button></div></form>{[...(selected.delivery_policies??[])].reverse().map((p)=><div key={p.id} className="mt-4 border-t pt-3 text-sm"><strong>Policy v{p.version}</strong> · branches {p.branches.join(", ")||"all"} · services {p.services.join(", ")||"all"} · environments {p.environment_ids.join(", ")||"all"}<p className="mt-1 text-xs text-[var(--muted)]">Budget {p.maximum_budget_consumed_percent}% · predicted increase {p.maximum_predicted_budget_increase_percent}% · {p.minimum_acknowledgements} owner acknowledgement(s). Missing {p.on_missing_evidence}; regression {p.on_regression}; exhausted {p.on_budget_exhausted}; dependency {p.on_dependency_failure}.</p></div>)}{[...(selected.reliability_impacts??[])].reverse().map((x)=><div key={x.id} className="mt-3 rounded-lg border p-3 text-sm"><div className="flex flex-wrap gap-2"><Badge>{x.kind.replaceAll("_"," ")}</Badge><Badge>{x.revision}</Badge>{x.active_exception&&<Badge tone="warning">exception until {new Date(x.active_exception.expires_at).toLocaleString()}</Badge>}</div><p className="mt-2">{x.summary}</p><p className="mt-1 text-xs text-[var(--muted)]">Predicted/observed impact {x.objective_impacts.map((i)=>`${i.objective_id}: +${i.predicted_budget_increase_percent}% predicted, ${i.observed_budget_consumed_percent??"—"}% consumed`).join(" · ")} · acknowledgements {x.owner_acknowledgements.map((a)=>a.owner_id).join(", ")||"missing"}</p>{x.dependency_failures.map((d)=><p key={d} className="mt-1 text-xs text-[var(--danger)]">Dependency failure: {d}</p>)}</div>)}</Card>}
      {selected && current && selectedObjective && (
        <section className="space-y-3">
          <Card className="p-5">
            <h2 className="font-semibold">Open a reliability investigation</h2>
            <p className="mt-1 text-sm text-[var(--muted)]">Freeze the change or budget event, baseline and affected windows, journeys, and permitted operational or code evidence before drawing a conclusion.</p>
            <form onSubmit={openInvestigation} className="mt-4 grid gap-4 md:grid-cols-2">
              <Field n="investigation_title" l="Investigation title" />
              <Select n="trigger_kind" l="Started from" v="objective" options={["objective","pull_request","deployment","budget_consumption"]}/>
              <Field n="trigger_id" l="Trigger resource ID" />
              <Field n="trigger_revision" l="Exact trigger revision" />
              <Field n="baseline_ids" l="Baseline observation IDs (comma separated)" />
              <Field n="affected_ids" l="Affected observation IDs (comma separated)" />
              <div className="md:col-span-2"><Area n="investigation_evidence" l="Evidence: kind | resource ID | exact revision | label | public/participants (semicolon separated)" /></div>
              <div className="md:col-span-2"><Button>Open revision-bound investigation</Button></div>
            </form>
          </Card>
          {[...(selected.investigations ?? [])].reverse().map((x)=>(
            <Card key={x.id} className="p-5">
              <div className="flex flex-wrap items-center gap-2"><h3 className="font-semibold">{x.title}</h3><Badge>{x.trigger.kind.replaceAll("_"," ")} @ {x.trigger.revision}</Badge>{x.inconclusive&&<Badge tone="warning">inconclusive</Badge>}{x.stale_evidence_ids.length>0&&<Badge tone="danger">stale evidence</Badge>}</div>
              <p className="mt-2 text-xs text-[var(--muted)]">Contract v{x.contract_version} · investigation v{x.version} · journeys {x.journey_ids.join(", ")}</p>
              {x.hidden_dependency_ids.map((id)=><p key={id} className="mt-2 text-sm text-[var(--warning)]">Hidden dependency ownership: {id}</p>)}
              {x.findings.map((finding)=><div key={finding.id} className="mt-3 border-l-2 pl-3 text-sm"><strong>{finding.kind}</strong> · {finding.confidence} confidence · {finding.statement}<p className="text-xs text-[var(--muted)]">Uncertainty: {finding.uncertainty} · citations {finding.citation_ids.join(", ")} · {finding.actor_type} {finding.created_by}</p>{x.responses.filter((r)=>r.finding_id===finding.id).map((r)=><p key={r.id} className="mt-1 text-xs"><strong>{r.kind}:</strong> {r.body} — {r.created_by}</p>)}<form className="mt-2 flex gap-2" onSubmit={(e)=>mutateInvestigation(e,x,"responses",(f)=>({response:{finding_id:finding.id,kind:value(f,"response_kind"),body:value(f,"response_body")}}))}><select name="response_kind" className="rounded border bg-transparent px-2"><option value="confirm">Confirm</option><option value="dispute">Dispute</option></select><input name="response_body" required aria-label="Response rationale" className="min-w-0 flex-1 rounded border bg-transparent px-2"/><Button>Respond</Button></form></div>)}
              {x.input_requests.map((q)=><div key={q.id} className="mt-3 text-sm"><strong>Owner input · {q.status}:</strong> {q.question}{q.response?` — ${q.response}`:""}{q.status==="requested"&&<form className="mt-2 flex gap-2" onSubmit={(e)=>mutateInvestigation(e,x,"input-responses",(f)=>({request:{id:q.id,response:value(f,"owner_response")}}))}><input name="owner_response" required aria-label="Owner response" className="min-w-0 flex-1 rounded border bg-transparent px-2"/><Button>Answer</Button></form>}</div>)}
              <form onSubmit={(e)=>addFinding(e,x)} className="mt-4 grid gap-3 border-t pt-4 md:grid-cols-2">
                <Select n="kind" l="Evidence entry" v="hypothesis" options={["hypothesis","comparison","uncertainty","conclusion"]}/><Select n="confidence" l="Confidence" v="low" options={["low","medium","high"]}/><Area n="statement" l="Cited statement"/><Area n="finding_uncertainty" l="What remains uncertain"/><div className="md:col-span-2"><Field n="citations" l="Observation or evidence IDs (comma separated)"/><Button>Record cited finding</Button></div>
              </form>
              <form onSubmit={(e)=>mutateInvestigation(e,x,"input-requests",(f)=>({request:{owner_id:value(f,"owner_id"),dependency_id:value(f,"dependency_id")||undefined,question:value(f,"question")}}))} className="mt-4 grid gap-3 border-t pt-4 md:grid-cols-3"><Field n="owner_id" l="Service/dependency owner ID"/><Field n="dependency_id" l="Dependency ID (optional)" required={false}/><Area n="question" l="Input requested"/><div className="md:col-span-3"><Button>Request owner input</Button></div></form>
              <form onSubmit={(e)=>mutateInvestigation(e,x,"outcomes",(f)=>({outcome:{kind:value(f,"outcome_kind"),resource_id:value(f,"outcome_id"),summary:value(f,"outcome_summary")}}))} className="mt-4 grid gap-3 border-t pt-4 md:grid-cols-3"><Select n="outcome_kind" l="Resulting work" v="issue" options={["issue","incident","decision","planned_improvement"]}/><Field n="outcome_id" l="Existing resource ID"/><Area n="outcome_summary" l="Why it follows"/><div className="md:col-span-3"><Button>Link resulting work</Button></div></form>
            </Card>
          ))}
        </section>
      )}
      {selected?.observations.length ? (
        <section>
          <h2 className="text-lg font-semibold">Attainment history</h2>
          <div className="mt-3 space-y-3">
            {[...selected.observations].reverse().map((o) => (
              <Card key={o.id} className="p-4">
                <div className="flex flex-wrap gap-2">
                  <strong>
                    {o.attainment?.toFixed(3) ?? "Unavailable"}% attainment
                  </strong>
                  <Badge tone={o.target_met ? "success" : "danger"}>
                    {o.target_met ? "target met" : "target missed"}
                  </Badge>
                  <Badge>
                    {o.error_budget_consumed_percent?.toFixed(1) ?? "—"}% budget
                    consumed
                  </Badge>
                  {!o.comparable_to_previous && (
                    <Badge tone="warning">incomparable window</Badge>
                  )}
                </div>
                <p className="mt-2 text-sm">{o.summary}</p>
                <p className="mt-1 text-xs text-[var(--muted)]">
                  {new Date(o.window_start).toLocaleString()} –{" "}
                  {new Date(o.window_end).toLocaleString()} · uncertainty{" "}
                  {o.uncertainty}% · mapping v{o.mapping_version} · recorded by{" "}
                  {o.recorded_by}
                </p>
                {o.comparison_reason && (
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    {o.comparison_reason}
                  </p>
                )}
                <p className="mt-2 text-xs text-[var(--muted)]">
                  Provenance:{" "}
                  {o.software
                    .map((x) => `${x.kind} ${x.label} @ ${x.revision}`)
                    .join(" · ") || "No delivered-software references"}
                </p>
                {o.gaps.map((x) => (
                  <p
                    key={`${x.kind}-${x.detail}`}
                    className="mt-1 text-xs text-[var(--warning)]"
                  >
                    Gap: {x.kind.replaceAll("_", " ")} — {x.detail}
                  </p>
                ))}
              </Card>
            ))}
          </div>
        </section>
      ) : null}
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
                onClick={() => {
                  setSelected(x);
                  setMappingID("");
                  setObjectiveID("");
                }}
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
