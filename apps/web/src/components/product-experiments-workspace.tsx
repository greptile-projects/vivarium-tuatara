"use client";
import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
type Revision = {
  version: number;
  hypothesis: string;
  variants: {
    key: string;
    name: string;
    description: string;
    control: boolean;
  }[];
  target_audience: {
    description: string;
    eligibility: string[];
    exclusions: string[];
  };
  metrics: {
    name: string;
    kind: string;
    direction: string;
    threshold: number;
    signal_id: string;
    signal_version: number;
  }[];
  minimum_evidence: number;
  duration_days: number;
  owners: string[];
  stop_conditions: string[];
  assumptions: string[];
  rationale: string;
  created_by: string;
  created_at: string;
};
type Work = {
  id: string;
  experiment_version: number;
  variant_keys: string[];
  owner_type: string;
  owner_id: string;
  pull_request_id: string;
  commit_id: string;
  event_definitions: string[];
  exposure_rules: string[];
  privacy_classification: string;
  removal_plan: string;
  check_names: string[];
  linked_by: string;
};
type Experiment = {
  id: string;
  source: { kind: string; resource_id: string; label: string };
  current_version: number;
  revisions: Revision[];
  signals: {
    id: string;
    name: string;
    version: number;
    event: string;
    property?: string;
    unit: string;
    privacy: string;
    status: string;
  }[];
  comments: {
    id: string;
    body: string;
    author_id: string;
    created_at: string;
  }[];
  approvals: {
    user_id: string;
    version: number;
    decision: string;
    note?: string;
  }[];
  work: Work[];
  audience_contracts: AudienceContract[];
  assignment_audit: {
    id: string;
    subject_digest: string;
    variant_key?: string;
    eligible: boolean;
    reason: string;
  }[];
  diagnostics: {
    kind: string;
    severity: string;
    message: string;
    attributed_to: string;
    related_experiment_id?: string;
  }[];
};
type AudienceContract = {
  id: string;
  experiment_version: number;
  release_id: string;
  release_commit_id: string;
  variant_keys: string[];
  eligibility: string[];
  exclusions: string[];
  organization_ids?: string[];
  regions?: string[];
  randomization_unit: string;
  mutual_exclusion_group: string;
  allocation: { variant_key: string; basis_points: number }[];
  consent: string;
  data_fields: string[];
  retention_days: number;
  approved_by: string;
};
const value = (f: FormData, n: string) => String(f.get(n) ?? "").trim();
const lines = (v: string) =>
  v
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
export function ProductExperimentsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth();
  const [items, setItems] = useState<Experiment[]>([]),
    [selected, setSelected] = useState<Experiment>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const r = await api<{ experiments: Experiment[] }>(
        `/repositories/${repositoryID}/product-experiments`,
        {},
        token,
      );
      setItems(r.experiments);
      setSelected(
        (x) => r.experiments.find((y) => y.id === x?.id) ?? r.experiments[0],
      );
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Experiments could not be loaded.",
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
    const f = new FormData(e.currentTarget),
      metricSignal = value(f, "signal_id");
    const revision = {
      hypothesis: value(f, "hypothesis"),
      variants: [
        {
          key: "control",
          name: value(f, "control_name"),
          description: value(f, "control_description"),
          control: true,
        },
        {
          key: "treatment",
          name: value(f, "variant_name"),
          description: value(f, "variant_description"),
          control: false,
        },
      ],
      target_audience: {
        description: value(f, "audience"),
        eligibility: lines(value(f, "eligibility")),
        exclusions: lines(value(f, "exclusions")),
      },
      metrics: [
        {
          name: value(f, "success_name"),
          kind: "success",
          direction: value(f, "success_direction"),
          threshold: Number(value(f, "success_threshold")),
          signal_id: metricSignal,
          signal_version: Number(value(f, "signal_version")),
        },
        {
          name: value(f, "guardrail_name"),
          kind: "guardrail",
          direction: value(f, "guardrail_direction"),
          threshold: Number(value(f, "guardrail_threshold")),
          signal_id: value(f, "guardrail_signal_id"),
          signal_version: Number(value(f, "guardrail_signal_version")),
        },
      ],
      minimum_evidence: Number(value(f, "minimum_evidence")),
      duration_days: Number(value(f, "duration_days")),
      owners: lines(value(f, "owners")),
      stop_conditions: lines(value(f, "stop_conditions")),
      assumptions: lines(value(f, "assumptions")),
      rationale: value(f, "rationale"),
    };
    const signals = [
      {
        id: metricSignal,
        name: value(f, "signal_name"),
        version: Number(value(f, "signal_version")),
        event: value(f, "signal_event"),
        property: value(f, "signal_property"),
        unit: value(f, "signal_unit"),
        privacy: value(f, "signal_privacy"),
        status: value(f, "signal_status"),
      },
      {
        id: value(f, "guardrail_signal_id"),
        name: value(f, "guardrail_signal_name"),
        version: Number(value(f, "guardrail_signal_version")),
        event: value(f, "guardrail_signal_event"),
        unit: value(f, "guardrail_signal_unit"),
        privacy: value(f, "guardrail_signal_privacy"),
        status: value(f, "guardrail_signal_status"),
      },
    ];
    const body = selected
      ? { expected_version: selected.current_version, revision, signals }
      : {
          source: {
            kind: value(f, "source_kind"),
            resource_id: value(f, "source_id"),
            label: value(f, "source_label"),
          },
          revision,
          signals,
        };
    const path = selected
      ? `/repositories/${repositoryID}/product-experiments/${selected.id}/revisions`
      : `/repositories/${repositoryID}/product-experiments`;
    try {
      const out = await api<Experiment>(
        path,
        { method: "POST", body: JSON.stringify(body) },
        token,
      );
      setSelected(out);
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Plan could not be published.");
    } finally {
      setBusy(false);
    }
  }
  async function discuss(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!selected || !token) return;
    const f = e.currentTarget;
    try {
      await api(
        `/repositories/${repositoryID}/product-experiments/${selected.id}/comments`,
        {
          method: "POST",
          body: JSON.stringify({ body: value(new FormData(f), "body") }),
        },
        token,
      );
      f.reset();
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Comment failed.");
    }
  }
  async function decide(decision: string) {
    if (!selected || !token) return;
    try {
      await api(
        `/repositories/${repositoryID}/product-experiments/${selected.id}/approvals`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.current_version,
            decision,
          }),
        },
        token,
      );
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Decision failed.");
    }
  }
  async function linkWork(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!selected || !token) return;
    const f = new FormData(e.currentTarget);
    const work = {
      variant_keys: lines(value(f, "variant_keys")),
      owner_type: value(f, "owner_type"),
      owner_id: value(f, "owner_id"),
      proposal_id: value(f, "proposal_id"),
      task_id: value(f, "task_id"),
      session_id: value(f, "session_id"),
      workspace_id: value(f, "workspace_id"),
      pull_request_id: value(f, "pull_request_id"),
      commit_id: value(f, "commit_id"),
      event_definitions: lines(value(f, "event_definitions")),
      exposure_rules: lines(value(f, "exposure_rules")),
      privacy_classification: value(f, "privacy"),
      removal_plan: value(f, "removal_plan"),
      check_names: lines(value(f, "check_names")),
    };
    try {
      await api(
        `/repositories/${repositoryID}/product-experiments/${selected.id}/work`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.current_version,
            work,
          }),
        },
        token,
      );
      e.currentTarget.reset();
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Work could not be linked.");
    }
  }
  async function approveAudience(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!selected || !token) return;
    const f = new FormData(e.currentTarget);
    const allocation = lines(value(f, "allocation")).map((line) => {
      const [variant_key, percent] = line.split(":");
      return {
        variant_key: variant_key?.trim(),
        basis_points: Math.round(Number(percent) * 100),
      };
    });
    try {
      await api(
        `/repositories/${repositoryID}/product-experiments/${selected.id}/audience-contracts`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.current_version,
            contract: {
              release_id: value(f, "release_id"),
              release_commit_id: value(f, "release_commit_id"),
              variant_keys: allocation.map((x) => x.variant_key),
              eligibility: lines(value(f, "contract_eligibility")),
              exclusions: lines(value(f, "contract_exclusions")),
              organization_ids: lines(value(f, "organization_ids")),
              regions: lines(value(f, "regions")),
              randomization_unit: "user",
              mutual_exclusion_group: value(f, "mutual_exclusion_group"),
              allocation,
              consent: value(f, "consent"),
              data_fields: lines(value(f, "data_fields")),
              retention_days: Number(value(f, "retention_days")),
            },
          }),
        },
        token,
      );
      e.currentTarget.reset();
      await load();
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Audience contract could not be approved.",
      );
    }
  }
  const r = selected?.revisions.at(-1);
  return (
    <div className="space-y-6">
      <header>
        <Link
          href={`/repositories/${repositoryID}`}
          className="text-sm text-[var(--muted)]"
        >
          Repository
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">Product experiments</h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Agree what a change should prove, what harm stops it, and who owns the
          answer before anyone receives a variant.
        </p>
      </header>
      <Card className="p-5">
        <div className="flex justify-between">
          <h2 className="font-semibold">
            {selected
              ? `Revise plan v${selected.current_version}`
              : "Open an experiment"}
          </h2>
          {selected && (
            <Button variant="secondary" onClick={() => setSelected(undefined)}>
              New experiment
            </Button>
          )}
        </div>
        <form onSubmit={publish} className="mt-4 space-y-4">
          <fieldset className="grid gap-3 md:grid-cols-3">
            <legend className="px-2 text-sm font-semibold">
              Decision source
            </legend>
            <Select
              name="source_kind"
              label="Source"
              initial={selected?.source.kind ?? "proposal"}
              options={[
                "proposal",
                "issue",
                "decision",
                "pull_request",
                "preview",
                "release",
              ]}
            />
            <Field
              name="source_id"
              label="Resource ID"
              initial={selected?.source.resource_id}
            />
            <Field
              name="source_label"
              label="Visible label"
              initial={selected?.source.label}
            />
          </fieldset>
          <Area
            name="hypothesis"
            label="Testable hypothesis"
            initial={r?.hypothesis}
          />
          <fieldset className="grid gap-3 md:grid-cols-2">
            <legend className="px-2 text-sm font-semibold">Variants</legend>
            <Field
              name="control_name"
              label="Control name"
              initial={r?.variants[0]?.name}
            />
            <Field
              name="variant_name"
              label="Treatment name"
              initial={r?.variants[1]?.name}
            />
            <Area
              name="control_description"
              label="Current experience"
              initial={r?.variants[0]?.description}
            />
            <Area
              name="variant_description"
              label="Changed experience"
              initial={r?.variants[1]?.description}
            />
          </fieldset>
          <fieldset className="grid gap-3 md:grid-cols-3">
            <legend className="px-2 text-sm font-semibold">
              Audience and accountability
            </legend>
            <Area
              name="audience"
              label="Target audience"
              initial={r?.target_audience.description}
            />
            <Area
              name="eligibility"
              label="Permitted eligibility rules (one per line)"
              initial={r?.target_audience.eligibility.join("\n")}
            />
            <Area
              name="exclusions"
              label="Exclusions"
              initial={r?.target_audience.exclusions.join("\n")}
            />
            <Field
              name="minimum_evidence"
              label="Minimum participants"
              type="number"
              initial={String(r?.minimum_evidence ?? 100)}
            />
            <Field
              name="duration_days"
              label="Duration (days)"
              type="number"
              initial={String(r?.duration_days ?? 14)}
            />
            <Area
              name="owners"
              label="Owner IDs (one per line)"
              initial={r?.owners.join("\n") ?? user?.id}
            />
            <Area
              name="stop_conditions"
              label="Stop conditions"
              initial={r?.stop_conditions.join("\n")}
            />
            <Area
              name="assumptions"
              label="Assumptions"
              initial={r?.assumptions.join("\n")}
            />
            <Area
              name="rationale"
              label="Revision rationale"
              initial={r?.rationale}
            />
          </fieldset>
          <Metric
            prefix="success"
            title="Success metric"
            metric={r?.metrics.find((x) => x.kind === "success")}
            signal={selected?.signals[0]}
          />
          <Metric
            prefix="guardrail"
            title="Guardrail metric"
            metric={r?.metrics.find((x) => x.kind === "guardrail")}
            signal={selected?.signals[1]}
          />
          <Button disabled={busy}>
            {busy
              ? "Publishing…"
              : selected
                ? "Publish successor"
                : "Open experiment"}
          </Button>
        </form>
      </Card>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      {selected && r && (
        <Card className="p-5">
          <div className="flex flex-wrap gap-2">
            <h2 className="mr-2 text-lg font-semibold">
              {selected.source.label}
            </h2>
            <Badge>v{selected.current_version}</Badge>
            <Badge
              tone={
                selected.diagnostics.some((x) => x.severity === "blocking")
                  ? "danger"
                  : "success"
              }
            >
              {selected.diagnostics.length
                ? `${selected.diagnostics.length} explicit states`
                : "ready for approval"}
            </Badge>
          </div>
          <p className="mt-3 text-sm">{r.hypothesis}</p>
          <div className="mt-4 grid gap-2 md:grid-cols-2">
            {selected.diagnostics.map((d, i) => (
              <div className="rounded-lg border p-3" key={d.kind + i}>
                <Badge tone={d.severity === "blocking" ? "danger" : "warning"}>
                  {d.kind.replaceAll("_", " ")}
                </Badge>
                <p className="mt-2 text-sm">{d.message}</p>
                <p className="text-xs text-[var(--muted)]">
                  Attributed to {d.attributed_to}
                </p>
              </div>
            ))}
          </div>
          <div className="mt-5 flex gap-2">
            <Button onClick={() => decide("approve")}>
              Approve v{selected.current_version}
            </Button>
            <Button
              variant="secondary"
              onClick={() => decide("request_changes")}
            >
              Request changes
            </Button>
          </div>
          <p className="mt-3 text-xs text-[var(--muted)]">
            {selected.approvals
              .map((a) => `${a.user_id}: ${a.decision} v${a.version}`)
              .join(" · ") || "No decisions yet"}
          </p>
          <form onSubmit={discuss} className="mt-5 flex gap-2">
            <input
              name="body"
              required
              placeholder="Discuss assumptions, risk, or measurement…"
              className="min-h-10 flex-1 rounded-lg border px-3 text-sm"
            />
            <Button>Comment</Button>
          </form>
          {selected.comments.map((c) => (
            <p
              key={c.id}
              className="mt-2 rounded-lg bg-[var(--surface-2)] p-3 text-sm"
            >
              {c.body}
              <span className="block text-xs text-[var(--muted)]">
                {c.author_id}
              </span>
            </p>
          ))}
        </Card>
      )}
      {selected && (
        <WorkReview
          experiment={selected}
          repositoryID={repositoryID}
          onSubmit={linkWork}
        />
      )}
      {selected && (
        <AudienceGovernance experiment={selected} onSubmit={approveAudience} />
      )}
      <section>
        <h2 className="font-semibold">Repository experiments</h2>
        {items.map((x) => (
          <button
            key={x.id}
            onClick={() => setSelected(x)}
            className="mt-2 block w-full rounded-lg border bg-white p-4 text-left"
          >
            <b>{x.source.label}</b>
            <span className="block text-xs text-[var(--muted)]">
              v{x.current_version} · {x.diagnostics.length} explicit state(s)
            </span>
          </button>
        ))}
      </section>
    </div>
  );
}
function AudienceGovernance({
  experiment,
  onSubmit,
}: {
  experiment: Experiment;
  onSubmit: (e: FormEvent<HTMLFormElement>) => void;
}) {
  return (
    <Card className="p-5">
      <h2 className="font-semibold">Audience exposure and consent</h2>
      <p className="mt-1 text-sm text-[var(--muted)]">
        Owner approval freezes an exact release, deterministic allocation,
        permitted audience, and minimal collection policy before rollout.
      </p>
      <form onSubmit={onSubmit} className="mt-4 grid gap-3 md:grid-cols-3">
        <Field name="release_id" label="Exact release ID" />
        <Field name="release_commit_id" label="Released commit" />
        <Field name="mutual_exclusion_group" label="Mutually exclusive group" />
        <Area
          name="allocation"
          label="Allocation: variant:percent (one per line)"
        />
        <Area name="contract_eligibility" label="Eligibility rules" />
        <Area name="contract_exclusions" label="Exclusions" />
        <Area name="organization_ids" label="Allowed organization IDs" />
        <Area name="regions" label="Allowed regions" />
        <Select
          name="consent"
          label="Consent"
          initial="explicit"
          options={["explicit", "none"]}
        />
        <Area
          name="data_fields"
          label="Minimal data (assignment, exposure, metric, region, organization)"
        />
        <Field
          name="retention_days"
          label="Retention days"
          type="number"
          initial="30"
        />
        <div className="md:col-span-3">
          <Button>Approve released audience</Button>
        </div>
      </form>
      <div className="mt-5 space-y-3">
        {experiment.audience_contracts?.map((c) => (
          <article key={c.id} className="rounded-lg border p-4">
            <div className="flex flex-wrap gap-2">
              <Badge>plan v{c.experiment_version}</Badge>
              <Badge>{c.consent} consent</Badge>
              <Badge>{c.retention_days} day retention</Badge>
            </div>
            <p className="mt-2 text-sm">
              <b>Release:</b> {c.release_id} at{" "}
              <code>{c.release_commit_id}</code>
            </p>
            <p className="mt-1 text-sm">
              <b>Allocation:</b>{" "}
              {c.allocation
                .map((a) => `${a.variant_key} ${a.basis_points / 100}%`)
                .join(" · ")}{" "}
              · group {c.mutual_exclusion_group}
            </p>
            <p className="mt-1 text-sm">
              <b>Eligible:</b> {c.eligibility.join(", ")} · regions{" "}
              {c.regions?.join(", ") || "all"} · organizations{" "}
              {c.organization_ids?.join(", ") || "all"}
            </p>
            <p className="mt-1 text-sm">
              <b>Collected:</b> {c.data_fields.join(", ")}
            </p>
            <p className="mt-1 text-xs text-[var(--muted)]">
              Approved by {c.approved_by}; assignment audit retains only salted
              subject digests.
            </p>
          </article>
        ))}
      </div>
    </Card>
  );
}
function WorkReview({
  experiment,
  repositoryID,
  onSubmit,
}: {
  experiment: Experiment;
  repositoryID: string;
  onSubmit: (e: FormEvent<HTMLFormElement>) => void;
}) {
  return (
    <Card className="p-5">
      <h2 className="font-semibold">Revision-exact project work</h2>
      <p className="mt-1 text-sm text-[var(--muted)]">
        Link an ordinary pull after its ownership, execution provenance,
        measurement contract, disable path, and repository checks are ready for
        review.
      </p>
      <form onSubmit={onSubmit} className="mt-4 grid gap-3 md:grid-cols-3">
        <Area name="variant_keys" label="Variant keys (one per line)" />
        <Select
          name="owner_type"
          label="Task owner"
          initial="human"
          options={["human", "agent"]}
        />
        <Field name="owner_id" label="Human or approved agent ID" />
        <Field name="proposal_id" label="Proposal ID" required={false} />
        <Field name="task_id" label="Task ID" required={false} />
        <Field name="session_id" label="Session ID" required={false} />
        <Field name="workspace_id" label="Workspace ID" required={false} />
        <Field name="pull_request_id" label="Pull request ID" />
        <Field name="commit_id" label="Exact 40-character commit" />
        <Area name="event_definitions" label="Event definitions and versions" />
        <Area name="exposure_rules" label="Exposure / assignment rules" />
        <Select
          name="privacy"
          label="Privacy classification"
          initial="aggregate"
          options={["aggregate", "pseudonymous", "consented"]}
        />
        <Area name="removal_plan" label="Disable and removal plan" />
        <Area name="check_names" label="Exact-commit check names" />
        <div className="md:col-span-3">
          <Button>Link reviewed work</Button>
        </div>
      </form>
      <div className="mt-5 space-y-3">
        {experiment.work?.map((w) => (
          <article key={w.id} className="rounded-lg border p-4">
            <div className="flex flex-wrap gap-2">
              <Badge>plan v{w.experiment_version}</Badge>
              <Badge>
                {w.owner_type}: {w.owner_id}
              </Badge>
              <Badge>{w.privacy_classification}</Badge>
            </div>
            <p className="mt-2 text-sm">
              <Link
                href={`/repositories/${repositoryID}/pulls/${w.pull_request_id}`}
                className="underline"
              >
                Pull {w.pull_request_id}
              </Link>{" "}
              at <code>{w.commit_id}</code>
            </p>
            <p className="mt-2 text-sm">
              <b>Variants:</b> {w.variant_keys.join(", ")} · <b>Events:</b>{" "}
              {w.event_definitions.join(", ")}
            </p>
            <p className="mt-1 text-sm">
              <b>Exposure:</b> {w.exposure_rules.join("; ")}
            </p>
            <p className="mt-1 text-sm">
              <b>Removal:</b> {w.removal_plan}
            </p>
            <p className="mt-1 text-xs text-[var(--muted)]">
              Checks: {w.check_names.join(", ")} · linked by {w.linked_by}
            </p>
          </article>
        ))}
      </div>
    </Card>
  );
}
function Metric({
  prefix,
  title,
  metric,
  signal,
}: {
  prefix: string;
  title: string;
  metric?: Revision["metrics"][number];
  signal?: Experiment["signals"][number];
}) {
  return (
    <fieldset className="grid gap-3 rounded-lg border p-4 md:grid-cols-4">
      <legend className="px-2 text-sm font-semibold">
        {title} and permitted product signal
      </legend>
      <Field name={`${prefix}_name`} label="Metric" initial={metric?.name} />
      <Select
        name={`${prefix}_direction`}
        label="Direction"
        initial={
          metric?.direction ?? (prefix === "success" ? "increase" : "below")
        }
        options={["increase", "decrease", "above", "below"]}
      />
      <Field
        name={`${prefix}_threshold`}
        label="Threshold"
        type="number"
        initial={String(metric?.threshold ?? 0)}
      />
      <Field
        name={`${prefix === "success" ? "" : "guardrail_"}signal_id`}
        label="Signal ID"
        initial={signal?.id}
      />
      <Field
        name={`${prefix === "success" ? "" : "guardrail_"}signal_name`}
        label="Signal name"
        initial={signal?.name}
      />
      <Field
        name={`${prefix === "success" ? "" : "guardrail_"}signal_version`}
        label="Signal version"
        type="number"
        initial={String(signal?.version ?? 1)}
      />
      <Field
        name={`${prefix === "success" ? "" : "guardrail_"}signal_event`}
        label="Event"
        initial={signal?.event}
      />
      {prefix === "success" && (
        <Field
          name="signal_property"
          label="Property (optional)"
          initial={signal?.property}
          required={false}
        />
      )}
      <Field
        name={`${prefix === "success" ? "" : "guardrail_"}signal_unit`}
        label="Unit"
        initial={signal?.unit}
      />
      <Select
        name={`${prefix === "success" ? "" : "guardrail_"}signal_privacy`}
        label="Permitted data"
        initial={signal?.privacy ?? "aggregate"}
        options={["aggregate", "pseudonymous", "consented"]}
      />
      <Select
        name={`${prefix === "success" ? "" : "guardrail_"}signal_status`}
        label="Instrumentation"
        initial={signal?.status ?? "available"}
        options={["available", "planned", "retired"]}
      />
    </fieldset>
  );
}
function Field({
  name,
  label,
  initial,
  type = "text",
  required = true,
}: {
  name: string;
  label: string;
  initial?: string;
  type?: string;
  required?: boolean;
}) {
  return (
    <label className="text-xs font-semibold">
      {label}
      <input
        key={initial}
        name={name}
        type={type}
        step={type === "number" ? "any" : undefined}
        required={required}
        defaultValue={initial}
        className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal"
      />
    </label>
  );
}
function Area({
  name,
  label,
  initial,
}: {
  name: string;
  label: string;
  initial?: string;
}) {
  return (
    <label className="text-xs font-semibold">
      {label}
      <textarea
        key={initial}
        name={name}
        required
        defaultValue={initial}
        className="mt-1 min-h-20 w-full rounded-lg border p-3 font-normal"
      />
    </label>
  );
}
function Select({
  name,
  label,
  initial,
  options,
}: {
  name: string;
  label: string;
  initial: string;
  options: string[];
}) {
  return (
    <label className="text-xs font-semibold">
      {label}
      <select
        name={name}
        defaultValue={initial}
        className="mt-1 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
      >
        {options.map((x) => (
          <option key={x}>{x}</option>
        ))}
      </select>
    </label>
  );
}
