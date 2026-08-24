"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Target = {
  id: string;
  kind: string;
  repository_id?: string;
  package?: string;
  release_line: string;
  owner_ids: string[];
  depends_on?: string[];
  deadline: string;
  state: string;
  diagnostic?: string;
  authority: string;
};
type AssessmentEntry = {
  id: string;
  kind: string;
  body: string;
  actor_id: string;
  actor_kind: string;
  citations: { kind: string; reference: string; revision?: string }[];
};
type Assessment = {
  id: string;
  target_id: string;
  version: number;
  classification: string;
  target_revision: string;
  source_revision: string;
  changed_paths: string[];
  comparisons: {
    kind: string;
    status: string;
    summary: string;
    evidence?: string[];
  }[];
  entries: AssessmentEntry[];
  invalidated: boolean;
  invalidation_reason?: string;
};
type Contribution = {
  id: string;
  target_id: string;
  assessment_id: string;
  application: string;
  deviation?: string;
  topology: string;
  proposal_id: string;
  task_ids: string[];
  authority: string;
};
type EvidenceRow = {
  name: string;
  command?: string;
  target_command?: string;
  source_command?: string;
  state: string;
  check_run_id?: string;
  logs?: string;
  artifacts?: { path: string; sha256: string; size: number }[];
  coverage?: string[];
  cost: number;
  substitute_evidence?: { reference: string }[];
};
type EquivalenceProof = {
  id: string;
  target_id: string;
  version: number;
  target_revision: string;
  source_revision: string;
  state: string;
  invalidated: boolean;
  invalidation_reasons?: string[];
  evidence_requirements: string[];
  scenarios: EvidenceRow[];
  ordinary_checks: EvidenceRow[];
  residual_differences: string[];
  owner_decisions: { owner_id: string; decision: string; rationale: string }[];
  authority: string;
};
type DeliveryPath = {
  id: string;
  target_id: string;
  pull_request_id: string;
  supported_user_groups: string[];
  review_state?: string;
  queue_state?: string;
  merge_revision?: string;
  release_version?: string;
  environment_id?: string;
  rollout_state?: string;
  observed_outcomes?: string[];
  blockers?: string[];
  next_action?: string;
  exposed: boolean;
  authority: string;
};
type ScopeEvent = {
  id: string;
  kind: string;
  target_id?: string;
  consumer_repository_id?: string;
  supported_user_groups?: string[];
  reason: string;
  follow_up: string;
  expires_at?: string;
  actor_id: string;
};
type Campaign = {
  id: string;
  title: string;
  intent: string;
  acceptance_criteria: string[];
  source: {
    kind: string;
    resource_id: string;
    commits: string[];
    label: string;
  };
  targets: Target[];
  assessments?: Assessment[];
  contributions?: Contribution[];
  equivalence_proofs?: EquivalenceProof[];
  delivery_paths?: DeliveryPath[];
  scope_events?: ScopeEvent[];
  coverage: {
    state: string;
    policy_satisfied: boolean;
    delivered_targets: number;
    total_targets: number;
    supported_user_groups: string[];
    blockers: string[];
    next_actions: string[];
  };
  completion_policy: {
    mode: string;
    minimum_targets?: number;
    require_acceptance: boolean;
  };
  created_by: string;
  created_at: string;
};
const lines = (value: FormDataEntryValue | null) =>
  String(value ?? "")
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);

export function PropagationCampaignsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token } = useAuth(),
    [items, setItems] = useState<Campaign[]>([]),
    [error, setError] = useState(""),
    [saving, setSaving] = useState(false),
    requestID = useRef(crypto.randomUUID());
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const x = await api<{ propagation_campaigns: Campaign[] }>(
        `/repositories/${repositoryID}/propagation-campaigns`,
        {},
        token,
      );
      setItems(x.propagation_campaigns);
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Campaigns could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    const form = event.currentTarget,
      f = new FormData(form);
    try {
      const targets = lines(f.get("targets")).map((line, index) => {
        const [kind, resource, release, owners, deadline, dependencies = ""] =
          line.split("|").map((x) => x.trim());
        return {
          id: `target-${index + 1}`,
          kind,
          repository_id: kind === "repository" ? resource : undefined,
          package: kind === "package" ? resource : undefined,
          release_line: release,
          owner_ids: owners
            .split(",")
            .map((x) => x.trim())
            .filter(Boolean),
          deadline: new Date(deadline).toISOString(),
          depends_on: dependencies
            .split(",")
            .map((x) => x.trim())
            .filter(Boolean),
        };
      });
      const out = await api<Campaign>(
        `/repositories/${repositoryID}/propagation-campaigns`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: requestID.current,
            title: String(f.get("title")),
            intent: String(f.get("intent")),
            acceptance_criteria: lines(f.get("criteria")),
            source: {
              kind: String(f.get("source_kind")),
              resource_id: String(f.get("source_id")),
              commits: lines(f.get("commits")),
              label: String(f.get("source_label")),
            },
            targets,
            completion_policy: {
              mode: String(f.get("policy")),
              minimum_targets: Number(f.get("minimum") || 0),
              require_acceptance: true,
            },
          }),
        },
        token,
      );
      setItems((x) => [out, ...x]);
      requestID.current = crypto.randomUUID();
      form.reset();
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Campaign could not be opened.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function assess(campaignID: string, targetID: string) {
    setSaving(true);
    try {
      await api(
        `/repositories/${repositoryID}/propagation-campaigns/${campaignID}/targets/${targetID}/assessments`,
        { method: "POST" },
        token,
      );
      await load();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Target could not be compared.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function trackDelivery(
    event: FormEvent<HTMLFormElement>,
    campaignID: string,
    targetID: string,
    contribution: Contribution,
    proof: EquivalenceProof,
  ) {
    event.preventDefault();
    setSaving(true);
    const f = new FormData(event.currentTarget);
    try {
      await api(
        `/repositories/${repositoryID}/propagation-campaigns/${campaignID}/targets/${targetID}/delivery-paths`,
        {
          method: "POST",
          body: JSON.stringify({
            contribution_id: contribution.id,
            equivalence_proof_id: proof.id,
            proof_version: proof.version,
            pull_request_id: String(f.get("pull_id")),
            supported_user_groups: lines(f.get("users")),
          }),
        },
        token,
      );
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Delivery path could not be tracked.");
    } finally {
      setSaving(false);
    }
  }
  async function discoverConsumer(event: FormEvent<HTMLFormElement>, campaignID: string) {
    event.preventDefault();
    setSaving(true);
    const form = event.currentTarget, f = new FormData(form);
    try {
      await api(`/repositories/${repositoryID}/propagation-campaigns/${campaignID}/scope-events`, {
        method: "POST",
        body: JSON.stringify({ kind: "consumer_discovered", consumer_repository_id: String(f.get("consumer")), supported_user_groups: lines(f.get("users")), reason: String(f.get("reason")), follow_up: String(f.get("follow_up")) }),
      }, token);
      form.reset();
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Consumer discovery could not be retained.");
    } finally { setSaving(false); }
  }
  async function addEntry(
    event: FormEvent<HTMLFormElement>,
    campaignID: string,
    assessment: Assessment,
  ) {
    event.preventDefault();
    setSaving(true);
    const form = event.currentTarget,
      f = new FormData(form);
    try {
      await api(
        `/repositories/${repositoryID}/propagation-campaigns/${campaignID}/assessments/${assessment.id}/entries`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: assessment.version,
            kind: String(f.get("kind")),
            body: String(f.get("body")),
            citations: [
              {
                kind: "commit",
                reference: String(f.get("reference")),
                revision: assessment.target_revision,
              },
            ],
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Assessment note could not be added.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function publishContribution(
    event: FormEvent<HTMLFormElement>,
    campaignID: string,
    targetID: string,
    assessment: Assessment,
  ) {
    event.preventDefault();
    setSaving(true);
    const form = event.currentTarget,
      f = new FormData(form);
    try {
      const tasks = lines(f.get("tasks")).map((line) => {
        const [assignee_type, assignee_id, title, outcome] = line
          .split("|")
          .map((x) => x.trim());
        return { assignee_type, assignee_id, title, outcome };
      });
      await api(
        `/repositories/${repositoryID}/propagation-campaigns/${campaignID}/targets/${targetID}/contributions`,
        {
          method: "POST",
          body: JSON.stringify({
            assessment_id: assessment.id,
            expected_version: assessment.version,
            application: String(f.get("application")),
            deviation: String(f.get("deviation")),
            topology: String(f.get("topology")),
            constraints: lines(f.get("constraints")),
            tasks,
          }),
        },
        token,
      );
      await load();
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Contribution work could not be published.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function prove(
    event: FormEvent<HTMLFormElement>,
    campaignID: string,
    targetID: string,
    targetRevision: string,
  ) {
    event.preventDefault();
    setSaving(true);
    const form = event.currentTarget,
      f = new FormData(form);
    try {
      const adaptations = lines(f.get("adaptations")).map((line) => {
        const [scenario, environment, command, coverage, residual = ""] = line
          .split("|")
          .map((x) => x.trim());
        return environment === "unsupported"
          ? {
              scenario,
              unsupported: true,
              coverage: coverage.split(",").filter(Boolean),
              substitute_evidence: [
                {
                  kind: "target_evidence",
                  reference: command,
                  revision: targetRevision,
                },
              ],
              residual_difference: residual,
            }
          : {
              scenario,
              environment_check: environment,
              command,
              coverage: coverage.split(",").filter(Boolean),
              unsupported: false,
            };
      });
      await api(
        `/repositories/${repositoryID}/propagation-campaigns/${campaignID}/targets/${targetID}/equivalence-proofs`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: crypto.randomUUID(),
            target_revision: targetRevision,
            adaptations,
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Behavioral equivalence could not be demonstrated.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function decide(
    campaignID: string,
    proof: EquivalenceProof,
    decision: string,
  ) {
    const rationale = window.prompt(`Why is this evidence ${decision}?`);
    if (!rationale) return;
    setSaving(true);
    try {
      await api(
        `/repositories/${repositoryID}/propagation-campaigns/${campaignID}/equivalence-proofs/${proof.id}/decisions`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: proof.version,
            decision,
            rationale,
          }),
        },
        token,
      );
      await load();
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Owner decision could not be retained.",
      );
    } finally {
      setSaving(false);
    }
  }
  return (
    <main className="mx-auto max-w-6xl space-y-6 p-6 sm:p-8">
      <header>
        <p className="text-sm font-semibold text-[var(--brand)]">
          Repository delivery
        </p>
        <h1 className="mt-1 text-2xl font-semibold">Propagation campaigns</h1>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-[var(--muted)]">
          Agree on the proven outcome, maintained users, order, and completion
          boundary before copying commits into release lines.
        </p>
      </header>
      <Card className="p-6">
        <h2 className="font-semibold">Open a campaign</h2>
        <form onSubmit={create} className="mt-4 grid gap-3 sm:grid-cols-2">
          <input
            name="title"
            required
            placeholder="Campaign title"
            className="rounded-lg border p-3"
          />
          <select name="source_kind" className="rounded-lg border p-3">
            <option value="merged_pull">Merged pull request</option>
            <option value="security_repair">Security repair</option>
            <option value="regression_correction">Regression correction</option>
            <option value="policy_change">Policy change</option>
            <option value="package_release">Package release</option>
            <option value="interface_evolution">Interface evolution</option>
          </select>
          <input
            name="source_id"
            required
            placeholder="Source resource ID"
            className="rounded-lg border p-3"
          />
          <input
            name="source_label"
            required
            placeholder="Source provenance label"
            className="rounded-lg border p-3"
          />
          <textarea
            name="commits"
            required
            placeholder="Exact source commits, one 40-character SHA per line"
            className="min-h-24 rounded-lg border p-3 sm:col-span-2"
          />
          <textarea
            name="intent"
            required
            placeholder="Shared outcome and intent"
            className="min-h-24 rounded-lg border p-3 sm:col-span-2"
          />
          <textarea
            name="criteria"
            required
            placeholder="Acceptance criteria, one per line"
            className="min-h-24 rounded-lg border p-3 sm:col-span-2"
          />
          <label className="text-sm sm:col-span-2">
            Targets{" "}
            <span className="text-[var(--muted)]">
              — one per line: repository|repository ID|branch|owner
              IDs|deadline|dependency target IDs (or package|package
              name|release line|owners|deadline)
            </span>
            <textarea
              name="targets"
              required
              placeholder={`repository|repo-id|release/1.x|user-id|2026-09-30|\npackage|@scope/pkg|2.x|user-id|2026-10-15|target-1`}
              className="mt-2 min-h-28 w-full rounded-lg border p-3 font-mono text-xs"
            />
          </label>
          <select name="policy" className="rounded-lg border p-3">
            <option value="all">All targets complete</option>
            <option value="ordered">All targets in dependency order</option>
            <option value="minimum">Minimum target count</option>
          </select>
          <input
            name="minimum"
            type="number"
            min="1"
            placeholder="Minimum (when selected)"
            className="rounded-lg border p-3"
          />
          <Button disabled={saving} type="submit" className="sm:col-span-2">
            {saving ? "Publishing…" : "Publish shared outcome"}
          </Button>
        </form>
        {error && (
          <p
            role="alert"
            className="mt-4 rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
          >
            {error}
          </p>
        )}
      </Card>
      <section className="space-y-4">
        {items.map((c) => (
          <Card key={c.id} className="p-6">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="font-semibold">{c.title}</h2>
              <Badge>{c.completion_policy.mode}</Badge>
            </div>
            <p className="mt-2 text-sm">{c.intent}</p>
            <p className="mt-3 text-xs text-[var(--muted)]">
              {c.source.kind.replaceAll("_", " ")} · {c.source.label} ·{" "}
              {c.source.commits.length} exact commit
              {c.source.commits.length === 1 ? "" : "s"}
            </p>
            <div className="mt-4 rounded-lg bg-[var(--surface-subtle)] p-4">
              <div className="flex flex-wrap items-center gap-2">
                <strong className="text-sm">Campaign coverage</strong>
                <Badge tone={c.coverage.state === "complete" ? "success" : c.coverage.blockers.length ? "danger" : "info"}>
                  {c.coverage.state.replaceAll("_", " ")}
                </Badge>
                <span className="text-xs">{c.coverage.delivered_targets}/{c.coverage.total_targets} targets delivered</span>
              </div>
              <p className="mt-2 text-xs">
                Supported users exposed: {c.coverage.supported_user_groups.join(", ") || "none yet"}
              </p>
              {c.coverage.blockers.map((x) => <p key={x} className="mt-1 text-xs text-[var(--danger)]">Blocked: {x}</p>)}
              {c.coverage.next_actions.map((x) => <p key={x} className="mt-1 text-xs">Next: {x}</p>)}
              {c.scope_events?.map((x) => (
                <p key={x.id} className="mt-2 text-xs"><strong>{x.kind.replaceAll("_", " ")}</strong> by {x.actor_id}: {x.reason} · {x.follow_up}</p>
              ))}
              <form onSubmit={(e) => discoverConsumer(e, c.id)} className="mt-3 grid gap-2 border-t pt-3 md:grid-cols-2">
                <input name="consumer" required placeholder="New consumer repository ID" className="rounded border p-2 text-xs" />
                <input name="reason" required placeholder="How this consumer was discovered" className="rounded border p-2 text-xs" />
                <textarea name="users" required placeholder="Affected supported user groups, one per line" className="rounded border p-2 text-xs" />
                <input name="follow_up" required placeholder="Named next action" className="rounded border p-2 text-xs" />
                <Button disabled={saving} type="submit">Record newly discovered consumer</Button>
              </form>
            </div>
            <div className="mt-4 grid gap-3 md:grid-cols-2">
              {c.targets.map((t) => {
                const assessment = c.assessments
                    ?.filter((a) => a.target_id === t.id)
                    .at(-1),
                  contribution = c.contributions?.find(
                    (x) => x.target_id === t.id,
                  ),
                  proof = c.equivalence_proofs
                    ?.filter((x) => x.target_id === t.id)
                    .at(-1),
                  delivery = c.delivery_paths?.find((x) => x.target_id === t.id);
                return (
                  <article
                    key={t.id}
                    className="rounded-lg border border-[var(--line)] p-4"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <strong className="text-sm">
                        {t.repository_id ?? t.package} · {t.release_line}
                      </strong>
                      <Badge
                        tone={
                          t.state === "already_equivalent"
                            ? "success"
                            : t.state === "inaccessible"
                              ? "danger"
                              : "info"
                        }
                      >
                        {t.state.replaceAll("_", " ")}
                      </Badge>
                    </div>
                    <p className="mt-2 text-xs text-[var(--muted)]">
                      Owners: {t.owner_ids.join(", ")} · due{" "}
                      {new Date(t.deadline).toLocaleDateString()}
                    </p>
                    {t.depends_on?.length ? (
                      <p className="mt-1 text-xs">
                        After: {t.depends_on.join(", ")}
                      </p>
                    ) : null}
                    {t.diagnostic && (
                      <p className="mt-2 text-xs">{t.diagnostic}</p>
                    )}
                    <p className="mt-2 text-xs text-[var(--muted)]">
                      {t.authority}
                    </p>
                    {t.kind === "repository" && t.state !== "inaccessible" ? (
                      <Button
                        type="button"
                        disabled={saving}
                        onClick={() => void assess(c.id, t.id)}
                        className="mt-3"
                      >
                        {assessment
                          ? "Recompare exact target"
                          : "Assess applicability"}
                      </Button>
                    ) : null}
                    {assessment ? (
                      <div className="mt-4 space-y-3 border-t pt-3">
                        <div className="flex gap-2">
                          <Badge
                            tone={
                              assessment.classification === "already_satisfied"
                                ? "success"
                                : assessment.classification === "conflicting"
                                  ? "danger"
                                  : "info"
                            }
                          >
                            {assessment.classification.replaceAll("_", " ")}
                          </Badge>
                          {assessment.invalidated ? (
                            <Badge tone="danger">stale</Badge>
                          ) : null}
                        </div>
                        {assessment.invalidation_reason ? (
                          <p className="text-xs text-[var(--danger)]">
                            {assessment.invalidation_reason}
                          </p>
                        ) : null}
                        <p className="font-mono text-[10px] text-[var(--muted)]">
                          Target {assessment.target_revision.slice(0, 12)} ·
                          source {assessment.source_revision.slice(0, 12)}
                        </p>
                        <ul className="space-y-2">
                          {assessment.comparisons.map((x) => (
                            <li key={x.kind} className="text-xs">
                              <strong>
                                {x.kind.replaceAll("_", " ")} ·{" "}
                                {x.status.replaceAll("_", " ")}
                              </strong>
                              <br />
                              {x.summary}
                            </li>
                          ))}
                        </ul>
                        {assessment.entries.map((x) => (
                          <p
                            key={x.id}
                            className="rounded bg-[var(--surface-subtle)] p-2 text-xs"
                          >
                            <strong>{x.kind.replaceAll("_", " ")}</strong> ·{" "}
                            {x.body}
                            <br />
                            <span className="text-[var(--muted)]">
                              {x.actor_kind} {x.actor_id}
                            </span>
                          </p>
                        ))}
                        <form
                          onSubmit={(e) => addEntry(e, c.id, assessment)}
                          className="grid gap-2"
                        >
                          <select
                            name="kind"
                            className="rounded border p-2 text-xs"
                          >
                            <option value="finding">Cited finding</option>
                            <option value="risk">Risk</option>
                            <option value="uncertainty">Uncertainty</option>
                            <option value="owner_acknowledgement">
                              Owner acknowledgement
                            </option>
                          </select>
                          <input
                            name="body"
                            required
                            placeholder="What still holds or changed?"
                            className="rounded border p-2 text-xs"
                          />
                          <input
                            name="reference"
                            required
                            placeholder="Citation label or path"
                            className="rounded border p-2 text-xs"
                          />
                          <Button disabled={saving} type="submit">
                            Add cited note
                          </Button>
                        </form>
                        {contribution ? (
                          <div className="rounded-lg bg-[var(--surface-subtle)] p-3 text-xs">
                            <strong>
                              {contribution.application} contribution published
                            </strong>
                            <br />
                            {contribution.task_ids.length} ordered task
                            {contribution.task_ids.length === 1
                              ? ""
                              : "s"} ·{" "}
                            {contribution.topology.replaceAll("_", " ")}
                            <br />
                            <span className="text-[var(--muted)]">
                              {contribution.authority}
                            </span>
                          </div>
                        ) : !assessment.invalidated &&
                          !["already_satisfied", "not_applicable"].includes(
                            assessment.classification,
                          ) ? (
                          <form
                            onSubmit={(e) =>
                              publishContribution(e, c.id, t.id, assessment)
                            }
                            className="grid gap-2 border-t pt-3"
                          >
                            <strong className="text-xs">
                              Publish locally owned contribution
                            </strong>
                            <select
                              name="application"
                              className="rounded border p-2 text-xs"
                            >
                              <option
                                value={
                                  assessment.classification ===
                                  "directly_applicable"
                                    ? "direct"
                                    : "adapted"
                                }
                              >
                                {assessment.classification ===
                                "directly_applicable"
                                  ? "Direct application (retain authorship)"
                                  : "Adapted application"}
                              </option>
                              <option value="adapted">
                                Adapted application
                              </option>
                            </select>
                            <select
                              name="topology"
                              className="rounded border p-2 text-xs"
                            >
                              <option value="local_branch">Local branch</option>
                              <option value="fork">Contributor fork</option>
                              <option value="federated">
                                Federated contribution
                              </option>
                            </select>
                            <textarea
                              name="deviation"
                              placeholder="Required for adaptation: explain deviations from the source change"
                              className="rounded border p-2 text-xs"
                            />
                            <textarea
                              name="constraints"
                              placeholder="Local constraints, one per line (do not paste embargoed context)"
                              className="rounded border p-2 text-xs"
                            />
                            <textarea
                              name="tasks"
                              required
                              placeholder="One per line: human|user-id|Title|Outcome or agent|agent-id|Title|Outcome"
                              className="min-h-20 rounded border p-2 font-mono text-xs"
                            />
                            <Button disabled={saving} type="submit">
                              Create ordered tasks
                            </Button>
                          </form>
                        ) : null}
                      </div>
                    ) : null}
                    {proof ? (
                      <div className="mt-4 space-y-3 border-t pt-3">
                        <div className="flex gap-2">
                          <Badge
                            tone={
                              proof.state === "accepted" ||
                              proof.state === "demonstrated"
                                ? "success"
                                : "danger"
                            }
                          >
                            {proof.state.replaceAll("_", " ")}
                          </Badge>
                          {proof.invalidated ? (
                            <Badge tone="danger">stale</Badge>
                          ) : null}
                        </div>
                        <p className="font-mono text-[10px] text-[var(--muted)]">
                          Target {proof.target_revision.slice(0, 12)} · source{" "}
                          {proof.source_revision.slice(0, 12)}
                        </p>
                        {proof.invalidation_reasons?.map((reason) => (
                          <p
                            key={reason}
                            className="text-xs text-[var(--danger)]"
                          >
                            {reason}
                          </p>
                        ))}
                        <div className="overflow-x-auto">
                          <table className="w-full text-left text-xs">
                            <thead>
                              <tr>
                                <th className="p-1">Evidence</th>
                                <th className="p-1">State</th>
                                <th className="p-1">Coverage</th>
                                <th className="p-1">Cost</th>
                              </tr>
                            </thead>
                            <tbody>
                              {[
                                ...proof.scenarios,
                                ...proof.ordinary_checks,
                              ].map((row) => (
                                <tr
                                  key={`${row.name}-${row.check_run_id ?? "substitute"}`}
                                  className="border-t"
                                >
                                  <td className="p-1">
                                    <strong>{row.name}</strong>
                                    <br />
                                    <span className="font-mono text-[10px]">
                                      {row.target_command ??
                                        row.command ??
                                        row.substitute_evidence
                                          ?.map((x) => x.reference)
                                          .join(", ")}
                                    </span>
                                  </td>
                                  <td className="p-1">{row.state}</td>
                                  <td className="p-1">
                                    {row.coverage?.join(", ") ??
                                      "ordinary target check"}
                                  </td>
                                  <td className="p-1">{row.cost.toFixed(4)}</td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                        {(proof.residual_differences ?? [])
                          .filter(Boolean)
                          .map((difference) => (
                            <p key={difference} className="text-xs">
                              Residual: {difference}
                            </p>
                          ))}
                        {proof.owner_decisions.map((decision, index) => (
                          <p
                            key={`${decision.owner_id}-${index}`}
                            className="rounded bg-[var(--surface-subtle)] p-2 text-xs"
                          >
                            <strong>{decision.decision}</strong> by{" "}
                            {decision.owner_id}: {decision.rationale}
                          </p>
                        ))}
                        {!proof.invalidated ? (
                          <div className="flex gap-2">
                            <Button
                              type="button"
                              disabled={saving}
                              onClick={() =>
                                void decide(c.id, proof, "accepted")
                              }
                            >
                              Accept evidence
                            </Button>
                            <Button
                              type="button"
                              disabled={saving}
                              onClick={() =>
                                void decide(c.id, proof, "rejected")
                              }
                            >
                              Reject
                            </Button>
                          </div>
                        ) : null}
                        <p className="text-xs text-[var(--muted)]">
                          {proof.authority}
                        </p>
                        {delivery ? (
                          <div className="rounded-lg border p-3 text-xs">
                            <div className="flex gap-2"><strong>Supported-user delivery</strong><Badge tone={delivery.exposed ? "success" : delivery.blockers?.length ? "danger" : "info"}>{delivery.rollout_state ?? delivery.queue_state ?? "tracked"}</Badge></div>
                            <p className="mt-2">Users: {delivery.supported_user_groups.join(", ")}</p>
                            <p>Review {delivery.review_state} · queue {delivery.queue_state} · release {delivery.release_version ?? "not released"} · environment {delivery.environment_id ?? "not deployed"}</p>
                            {delivery.observed_outcomes?.map((x) => <p key={x}>Observed: {x}</p>)}
                            {delivery.blockers?.map((x) => <p key={x} className="text-[var(--danger)]">Blocked: {x}</p>)}
                            <p>Next: {delivery.next_action}</p>
                            <p className="mt-1 text-[var(--muted)]">{delivery.authority}</p>
                          </div>
                        ) : contribution && proof.state === "accepted" && !proof.invalidated ? (
                          <form onSubmit={(e) => trackDelivery(e, c.id, t.id, contribution, proof)} className="grid gap-2 border-t pt-3">
                            <strong className="text-xs">Track ordinary delivery</strong>
                            <input name="pull_id" required placeholder="Task-linked pull request ID" className="rounded border p-2 text-xs" />
                            <textarea name="users" required placeholder="Supported user groups served, one per line" className="rounded border p-2 text-xs" />
                            <Button disabled={saving} type="submit">Bind delivery path</Button>
                          </form>
                        ) : null}
                      </div>
                    ) : assessment &&
                      !assessment.invalidated &&
                      t.kind === "repository" ? (
                      <form
                        onSubmit={(e) =>
                          prove(e, c.id, t.id, assessment.target_revision)
                        }
                        className="mt-4 grid gap-2 border-t pt-3"
                      >
                        <strong className="text-xs">
                          Demonstrate behavioral equivalence
                        </strong>
                        <p className="text-xs text-[var(--muted)]">
                          Map every source check to a bounded target command:
                          source check | target environment check | command |
                          criteria. For unsupported checks use: source check |
                          unsupported | substitute evidence reference | criteria
                          | residual difference.
                        </p>
                        <textarea
                          name="adaptations"
                          required
                          className="min-h-24 rounded border p-2 font-mono text-xs"
                          placeholder="parser-regression | unit | bun test parser | malformed input,stable error"
                        />
                        <Button disabled={saving} type="submit">
                          Run scenarios and ordinary checks
                        </Button>
                      </form>
                    ) : null}
                  </article>
                );
              })}
            </div>
          </Card>
        ))}
        {!items.length && !error && (
          <Card className="p-8 text-center text-sm text-[var(--muted)]">
            No propagation campaigns yet.
          </Card>
        )}
      </section>
    </main>
  );
}
