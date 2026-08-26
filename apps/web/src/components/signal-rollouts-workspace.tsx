"use client";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "@/components/ui";
type Scope = {
  service: string;
  audience: string;
  region: string;
  traffic_percent: number;
};
type Quality = {
  signal_health: string;
  coverage: number;
  latency_ms: number;
  missingness: number;
  sampling_bias: number;
  cardinality: number;
  storage_bytes: number;
  query_cost_cents: number;
  pipeline_loss: number;
  privacy_controls: string[];
};
type Rollout = {
  id: string;
  version: number;
  status: string;
  contract_id: string;
  contract_version: number;
  instrumentation_revision: string;
  environment_id: string;
  controller_id: string;
  scope: Scope;
  budget: {
    storage_bytes: number;
    query_cost_cents: number;
    cardinality: number;
  };
  containment_reasons: string[];
  observations: { id: string; quality: Quality; created_at: string }[];
};
const staged = {
  request_id: "",
  contract_id: "contract-id",
  contract_version: 1,
  instrumentation_revision: "exact-commit",
  deployment_id: "promotion-id",
  environment_id: "environment-id",
  scope: {
    service: "checkout-api",
    audience: "operators",
    region: "eu-west",
    traffic_percent: 5,
  },
  budget: {
    storage_bytes: 100000000,
    query_cost_cents: 1000,
    cardinality: 10000,
  },
};
const observation = {
  request_id: "",
  reason: "production calibration window",
  scope: {
    service: "checkout-api",
    audience: "operators",
    region: "eu-west",
    traffic_percent: 5,
  },
  observation: {
    scope: {
      service: "checkout-api",
      audience: "operators",
      region: "eu-west",
      traffic_percent: 5,
    },
    started_at: "2026-08-26T20:00:00Z",
    ended_at: "2026-08-26T20:05:00Z",
    quality: {
      signal_health: "healthy",
      coverage: 0.99,
      latency_ms: 250,
      missingness: 0.01,
      sampling_bias: 0.01,
      cardinality: 500,
      storage_bytes: 1000000,
      query_cost_cents: 20,
      pipeline_loss: 0.005,
      privacy_controls: ["redaction", "restricted audience"],
      malformed_payloads: 0,
      unexpected_sensitive_data: false,
      collector_available: true,
      service_regression: false,
    },
  },
};
export function SignalRolloutsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token } = useAuth();
  const [items, setItems] = useState<Rollout[]>([]),
    [selected, setSelected] = useState<Rollout>(),
    [draft, setDraft] = useState(JSON.stringify(staged, null, 2)),
    [evidence, setEvidence] = useState(JSON.stringify(observation, null, 2)),
    [error, setError] = useState("");
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const x = await api<{ signal_rollouts: Rollout[] }>(
        `/repositories/${repositoryID}/signal-rollouts`,
        {},
        token,
      );
      setItems(x.signal_rollouts);
      setSelected(
        (v) =>
          x.signal_rollouts.find((y) => y.id === v?.id) ?? x.signal_rollouts[0],
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Rollouts could not be loaded");
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(e: FormEvent) {
    e.preventDefault();
    if (!token) return;
    try {
      const body = JSON.parse(draft);
      body.request_id ||= crypto.randomUUID();
      await api(
        `/repositories/${repositoryID}/signal-rollouts`,
        { method: "POST", body: JSON.stringify(body) },
        token,
      );
      setError("");
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Rollout could not be staged");
    }
  }
  async function act(kind: string) {
    if (!token || !selected) return;
    try {
      const body =
        kind === "observe"
          ? JSON.parse(evidence)
          : { request_id: crypto.randomUUID(), reason: `operator ${kind}` };
      body.request_id ||= crypto.randomUUID();
      body.kind = kind;
      if (kind === "narrow") {
        body.scope = {
          ...selected.scope,
          traffic_percent: Math.max(1, selected.scope.traffic_percent / 2),
        };
      }
      await api(
        `/repositories/${repositoryID}/signal-rollouts/${selected.id}/events`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            event: body,
          }),
        },
        token,
      );
      setError("");
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Collection control failed");
    }
  }
  return (
    <section className="space-y-5">
      <Card className="p-5">
        <h2 className="text-xl font-semibold">Signal rollouts</h2>
        <p className="text-sm text-[var(--muted)]">
          Progressively calibrate reviewed instrumentation in protected
          environments. Unsafe quality, privacy, reliability, or budget evidence
          contains collection deterministically.
        </p>
        <form onSubmit={create}>
          <textarea
            aria-label="Signal rollout JSON"
            rows={10}
            className="mt-4 w-full rounded-lg border bg-slate-950 p-3 font-mono text-xs text-slate-100"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
          />
          <Button className="mt-2">Stage protected rollout</Button>
        </form>
        {error && (
          <p role="alert" className="mt-2 text-sm text-[var(--danger)]">
            {error}
          </p>
        )}
      </Card>
      {selected && (
        <Card className="p-5">
          <div className="flex gap-2">
            <Badge>{selected.status}</Badge>
            <Badge>v{selected.version}</Badge>
          </div>
          <h3 className="mt-3 font-semibold">
            {selected.scope.service} · {selected.scope.region} ·{" "}
            {selected.scope.audience}
          </h3>
          <p className="text-sm">
            {selected.scope.traffic_percent}% traffic · revision{" "}
            {selected.instrumentation_revision} · environment{" "}
            {selected.environment_id}
          </p>
          <p className="text-sm">Active controller: {selected.controller_id}</p>
          <div className="mt-3 flex gap-2">
            <Button onClick={() => act("pause")}>Pause</Button>
            <Button onClick={() => act("resume")}>Resume</Button>
            <Button onClick={() => act("narrow")}>Narrow</Button>
            <Button onClick={() => act("rollback")}>Roll back</Button>
          </div>
          <textarea
            aria-label="Production signal evidence JSON"
            rows={12}
            className="mt-4 w-full rounded-lg border bg-slate-950 p-3 font-mono text-xs text-slate-100"
            value={evidence}
            onChange={(e) => setEvidence(e.target.value)}
          />
          <Button className="mt-2" onClick={() => act("observe")}>
            Retain quality observation
          </Button>
          {selected.containment_reasons.length > 0 && (
            <p className="mt-3 text-sm text-[var(--danger)]">
              Contained: {selected.containment_reasons.join(", ")}
            </p>
          )}
          {selected.observations.map((o) => (
            <div className="mt-3 rounded border p-3 text-sm" key={o.id}>
              <strong>{o.quality.signal_health}</strong> · coverage{" "}
              {(o.quality.coverage * 100).toFixed(1)}% · latency{" "}
              {o.quality.latency_ms}ms · missing{" "}
              {(o.quality.missingness * 100).toFixed(1)}% · bias{" "}
              {(o.quality.sampling_bias * 100).toFixed(1)}% · cardinality{" "}
              {o.quality.cardinality} · storage {o.quality.storage_bytes}B ·
              query ${(o.quality.query_cost_cents / 100).toFixed(2)} · pipeline
              loss {(o.quality.pipeline_loss * 100).toFixed(1)}%
              <p>Privacy: {o.quality.privacy_controls.join(", ")}</p>
            </div>
          ))}
        </Card>
      )}
      <div className="grid gap-2">
        {items.map((x) => (
          <button
            className="rounded border p-3 text-left"
            key={x.id}
            onClick={() => setSelected(x)}
          >
            {x.scope.service} · {x.scope.region} · {x.status}
          </button>
        ))}
      </div>
    </section>
  );
}
