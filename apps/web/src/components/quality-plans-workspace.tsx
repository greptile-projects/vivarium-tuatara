"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Revision = {
  version: number;
  title: string;
  summary: string;
  scopes: { kind: string; resource_id?: string; name: string; source_revision?: string }[];
  supported_environments: {
    id: string;
    name: string;
    description: string;
    supported: boolean;
  }[];
  requirements: {
    id: string;
    source_kind: string;
    source_id: string;
    title: string;
    rationale: string;
    expected_behavior: string;
    risk: string;
    test_levels: string[];
    representative_data: string;
    coverage_goal: string;
    owner_ids: string[];
    judge_ids: string[];
    environment_ids: string[];
    schedule: string;
    release_threshold: string;
    evidence_ids: string[];
    conflicts_with?: string[];
    verification?: string;
  }[];
  evidence: {
    id: string;
    kind: string;
    resource_kind: string;
    resource_id: string;
    revision?: string;
    summary: string;
    status: string;
    added_by?: string;
  }[];
  exceptions: {
    id: string;
    requirement_id: string;
    rationale: string;
    granted_by: string;
    expires_at: string;
    follow_up: string;
  }[];
  owner_ids: string[];
  review_schedule: string;
  rationale: string;
  created_by: string;
  created_at: string;
};
type Plan = {
  id: string;
  current_version: number;
  revisions: Revision[];
  diagnostics: {
    kind: string;
    severity: string;
    message: string;
    requirement_id?: string;
    attributed_to: string;
  }[];
};
type Scenario = {
  id: string;
  title: string;
  purpose: string;
  sources: {
    kind: string;
    resource_id: string;
    revision: string;
    path?: string;
    summary: string;
  }[];
  parameters: {
    name: string;
    description: string;
    type: string;
    required: boolean;
    example?: string;
  }[];
  preconditions: { id: string; description: string; operation: string }[];
  actions: {
    id: string;
    description: string;
    operation: string;
    parameters?: string[];
  }[];
  assertions: {
    id: string;
    description: string;
    matcher: string;
    expected: string;
  }[];
  fixtures: {
    id: string;
    kind: string;
    description: string;
    path: string;
    sha256: string;
    data_class: string;
    generator?: string;
    assumptions: string[];
  }[];
  environments: {
    id: string;
    description: string;
    runtime: string;
    requirements: string[];
  }[];
  cases: {
    id: string;
    name: string;
    values: Record<string, string>;
    assumptions: string[];
    expected_outcome: string;
  }[];
  implementation: {
    authored_by_type: string;
    branch: string;
    commit_id: string;
    pull_request_id?: string;
    workspace_id?: string;
    test_paths: string[];
    command: string;
    framework: string;
    generated: boolean;
    assumptions: string[];
    provenance: string[];
  };
  created_by: string;
  created_at: string;
};
const value = (f: FormData, n: string) => String(f.get(n) ?? "").trim();
const list = (v: string) =>
  v
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);

export function QualityPlansWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth(),
    [plans, setPlans] = useState<Plan[]>([]),
    [scenarios, setScenarios] = useState<Scenario[]>([]),
    [selected, setSelected] = useState<Plan>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const [out, cases] = await Promise.all([
        api<{ plans: Plan[] }>(
          `/repositories/${repositoryID}/quality-plans`,
          {},
          token,
        ),
        api<{ scenarios: Scenario[] }>(
          `/repositories/${repositoryID}/test-scenarios`,
          {},
          token,
        ),
      ]);
      setPlans(out.plans);
      setScenarios(cases.scenarios);
      setSelected(
        (old) => out.plans.find((x) => x.id === old?.id) ?? out.plans[0],
      );
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Quality assets could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  const current = selected?.revisions.at(-1),
    requirement = current?.requirements[0],
    evidence = current?.evidence[0],
    environment = current?.supported_environments[0];
  function build(
    f: FormData,
  ): Omit<Revision, "version" | "created_by" | "created_at"> {
    const requirementID = value(f, "requirement_id"),
      environmentID = value(f, "environment_id"),
      evidenceID = value(f, "evidence_id"),
      expiry = value(f, "exception_expiry"),
      priorEnvironmentID = environment?.id,
      priorEvidenceID = evidence?.id;
    const replaceEnvironment = (ids: string[]) =>
      replaceAssociation(ids, priorEnvironmentID, environmentID);
    const replaceEvidence = (ids: string[]) =>
      evidenceID
        ? replaceAssociation(ids, priorEvidenceID, evidenceID)
        : ids.filter((id) => id !== priorEvidenceID);
    return {
      title: value(f, "title"),
      summary: value(f, "summary"),
      scopes: [
        {
          kind: value(f, "scope_kind"),
          resource_id: value(f, "scope_id"),
          name: value(f, "scope_name"),
          source_revision: value(f, "scope_revision"),
        },
        ...(current?.scopes.slice(1) ?? []),
      ],
      supported_environments: [
        {
          id: environmentID,
          name: value(f, "environment_name"),
          description: value(f, "environment_description"),
          supported: true,
        },
        ...(current?.supported_environments.slice(1) ?? []),
      ],
      requirements: [
        {
          id: requirementID,
          source_kind: value(f, "source_kind"),
          source_id: value(f, "source_id"),
          title: value(f, "requirement_title"),
          rationale: value(f, "requirement_rationale"),
          expected_behavior: value(f, "expected_behavior"),
          risk: value(f, "risk"),
          test_levels: list(value(f, "test_levels")),
          representative_data: value(f, "representative_data"),
          coverage_goal: value(f, "coverage_goal"),
          owner_ids: list(value(f, "requirement_owners")),
          judge_ids: list(value(f, "judges")),
          environment_ids: current
            ? replaceEnvironment(requirement?.environment_ids ?? [])
            : [environmentID],
          schedule: value(f, "schedule"),
          release_threshold: value(f, "release_threshold"),
          evidence_ids: current
            ? replaceEvidence(requirement?.evidence_ids ?? [])
            : evidenceID
              ? [evidenceID]
              : [],
          conflicts_with: list(value(f, "conflicts_with")),
          verification: value(f, "verification"),
        },
        ...(current?.requirements
          .slice(1)
          .map((item) => ({
            ...item,
            environment_ids: replaceEnvironment(item.environment_ids),
            evidence_ids: replaceEvidence(item.evidence_ids),
          })) ?? []),
      ],
      evidence: evidenceID
        ? [
            {
              id: evidenceID,
              kind: value(f, "evidence_kind"),
              resource_kind: value(f, "evidence_resource_kind"),
              resource_id: value(f, "evidence_resource_id"),
              revision: value(f, "evidence_revision"),
              summary: value(f, "evidence_summary"),
              status: value(f, "evidence_status"),
            },
            ...(current?.evidence.slice(1) ?? []),
          ]
        : (current?.evidence.slice(1) ?? []),
      exceptions: expiry
        ? [
            {
              id: value(f, "exception_id"),
              requirement_id: requirementID,
              rationale: value(f, "exception_rationale"),
              granted_by: value(f, "exception_grantor"),
              expires_at: new Date(expiry).toISOString(),
              follow_up: value(f, "exception_follow_up"),
            },
            ...(current?.exceptions.slice(1) ?? []),
          ]
        : (current?.exceptions.slice(1) ?? []),
      owner_ids: list(value(f, "plan_owners")),
      review_schedule: value(f, "review_schedule"),
      rationale: value(f, "revision_rationale"),
    };
  }
  async function publish(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    setBusy(true);
    setError("");
    try {
      const path = selected
        ? `/repositories/${repositoryID}/quality-plans/${selected.id}/revisions`
        : `/repositories/${repositoryID}/quality-plans`;
      const out = await api<Plan>(
        path,
        {
          method: "POST",
          body: JSON.stringify({
            revision: build(new FormData(event.currentTarget)),
            ...(selected ? { expected_version: selected.current_version } : {}),
          }),
        },
        token,
      );
      setSelected(out);
      setPlans((items) => [out, ...items.filter((x) => x.id !== out.id)]);
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Quality plan could not be published.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function createScenario(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    const f = new FormData(event.currentTarget),
      parameter = value(f, "case_parameter"),
      sourceID = value(f, "scenario_source_id"),
      revision = value(f, "scenario_revision"),
      plan = selected;
    setBusy(true);
    setError("");
    try {
      const out = await api<Scenario>(
        `/repositories/${repositoryID}/test-scenarios`,
        {
          method: "POST",
          body: JSON.stringify({
            title: value(f, "scenario_title"),
            purpose: value(f, "scenario_purpose"),
            quality_plan_id: plan?.id,
            quality_plan_version: plan?.current_version,
            requirement_ids: value(f, "scenario_requirement")
              ? [value(f, "scenario_requirement")]
              : [],
            sources: [
              {
                kind: value(f, "scenario_source_kind"),
                resource_id: sourceID,
                revision,
                path: value(f, "scenario_source_path"),
                summary: value(f, "scenario_source_summary"),
              },
            ],
            parameters: [
              {
                name: parameter,
                description: value(f, "parameter_description"),
                type: "string",
                required: true,
                example: value(f, "case_value"),
              },
            ],
            preconditions: [
              {
                id: "precondition",
                description: value(f, "precondition"),
                operation: value(f, "precondition_operation"),
              },
            ],
            actions: [
              {
                id: "action",
                description: value(f, "action"),
                operation: value(f, "action_operation"),
                parameters: [parameter],
              },
            ],
            assertions: [
              {
                id: "assertion",
                description: value(f, "assertion"),
                matcher: value(f, "matcher"),
                expected: value(f, "expected"),
              },
            ],
            fixtures: [
              {
                id: "fixture",
                kind: value(f, "fixture_kind"),
                description: value(f, "fixture_description"),
                path: value(f, "fixture_path"),
                sha256: value(f, "fixture_digest"),
                data_class: value(f, "data_class"),
                generator: value(f, "generator"),
                assumptions: list(value(f, "fixture_assumptions")),
                source_ids: [sourceID],
              },
            ],
            environments: [
              {
                id: "environment",
                description: value(f, "scenario_environment"),
                runtime: value(f, "runtime"),
                requirements: list(value(f, "environment_requirements")),
              },
            ],
            cases: [
              {
                id: "case",
                name: value(f, "case_name"),
                values: { [parameter]: value(f, "case_value") },
                assumptions: list(value(f, "case_assumptions")),
                expected_outcome: value(f, "case_outcome"),
              },
            ],
            implementation: {
              authored_by_type: value(f, "authored_by_type"),
              branch: value(f, "branch"),
              commit_id: revision,
              pull_request_id: value(f, "pull_id"),
              workspace_id: value(f, "workspace_id"),
              test_paths: list(value(f, "test_paths")),
              command: value(f, "command"),
              framework: value(f, "framework"),
              generated: f.get("generated") !== null,
              assumptions: list(value(f, "implementation_assumptions")),
              provenance: list(value(f, "provenance")),
            },
          }),
        },
        token,
      );
      setScenarios((items) => [out, ...items]);
      event.currentTarget.reset();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Test scenario could not be created.",
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
        <h1 className="mt-2 text-2xl font-semibold">Quality plans</h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Name the behavior worth protecting, its source and risk, who owns and
          judges it, and the evidence a release must provide. Gaps remain
          visible even when checks pass.
        </p>
      </header>
      <Card className="p-5">
        <div className="flex justify-between">
          <h2 className="font-semibold">
            {selected
              ? "Publish a complete successor"
              : "Define quality intent"}
          </h2>
          {selected && (
            <Button
              type="button"
              variant="secondary"
              onClick={() => setSelected(undefined)}
            >
              New plan
            </Button>
          )}
        </div>
        <form
          key={selected ? `${selected.id}:${selected.current_version}` : "new"}
          className="mt-4 space-y-4"
          onSubmit={publish}
        >
          <Group title="Plan and scope">
            <Field n="title" l="Plan title" v={current?.title} />
            <Select
              n="scope_kind"
              l="Scope"
              v={current?.scopes[0]?.kind ?? "repository"}
              options={["repository", "release", "journey", "interface"]}
            />
            <Field n="scope_name" l="Scope name" v={current?.scopes[0]?.name} />
            <Field
              n="scope_id"
              l="Scope resource ID"
              v={current?.scopes[0]?.resource_id}
              required={false}
            />
            <Field
              n="scope_revision"
              l="Scope source revision"
              v={current?.scopes[0]?.source_revision}
              placeholder="Exact 40-character commit for a journey"
              required={false}
            />
            <Field
              n="plan_owners"
              l="Plan owner IDs"
              v={current?.owner_ids.join(", ") ?? user?.id}
              required={false}
            />
            <Field
              n="review_schedule"
              l="Review schedule"
              v={current?.review_schedule}
              placeholder="Before every release"
            />
          </Group>
          <Area n="summary" l="Quality promise" v={current?.summary} />
          <Area
            n="revision_rationale"
            l="Revision rationale"
            v={current?.rationale}
          />
          <Group title="Supported environment">
            <Field n="environment_id" l="Environment key" v={environment?.id} />
            <Field
              n="environment_name"
              l="Environment name"
              v={environment?.name}
            />
            <Field
              n="environment_description"
              l="Support boundary"
              v={environment?.description}
            />
          </Group>
          <Group title="Expected behavior">
            <Field n="requirement_id" l="Requirement key" v={requirement?.id} />
            <Field
              n="requirement_title"
              l="Behavior title"
              v={requirement?.title}
            />
            <Select
              n="source_kind"
              l="Requirement source"
              v={requirement?.source_kind ?? "issue"}
              options={[
                "issue",
                "decision",
                "design",
                "accessibility",
                "privacy",
                "performance",
                "reliability",
              ]}
            />
            <Field
              n="source_id"
              l="Source resource ID"
              v={requirement?.source_id}
            />
            <Select
              n="risk"
              l="Risk"
              v={requirement?.risk ?? "high"}
              options={["low", "medium", "high", "critical"]}
            />
            <Field
              n="test_levels"
              l="Test levels"
              v={requirement?.test_levels.join(", ")}
              placeholder="unit, end_to_end, manual"
            />
            <Field
              n="requirement_owners"
              l="Behavior owner IDs"
              v={requirement?.owner_ids.join(", ") ?? user?.id}
              required={false}
            />
            <Field
              n="judges"
              l="Release judge IDs"
              v={requirement?.judge_ids.join(", ") ?? user?.id}
              required={false}
            />
            <Field n="schedule" l="Test schedule" v={requirement?.schedule} />
            <Field
              n="coverage_goal"
              l="Coverage goal"
              v={requirement?.coverage_goal}
            />
            <Field
              n="release_threshold"
              l="Release threshold"
              v={requirement?.release_threshold}
            />
            <Field
              n="conflicts_with"
              l="Conflicting requirement IDs"
              v={requirement?.conflicts_with?.join(", ")}
              required={false}
            />
          </Group>
          <Area
            n="requirement_rationale"
            l="Why this behavior matters"
            v={requirement?.rationale}
          />
          <Area
            n="expected_behavior"
            l="Expected behavior"
            v={requirement?.expected_behavior}
          />
          <Area
            n="representative_data"
            l="Representative, privacy-safe data"
            v={requirement?.representative_data}
          />
          <Area
            n="verification"
            l="Observable verification method (blank remains an explicit untestable claim)"
            v={requirement?.verification}
            required={false}
          />
          <Group title="Existing evidence (optional)">
            <Field
              n="evidence_id"
              l="Evidence key"
              v={evidence?.id}
              required={false}
            />
            <Select
              n="evidence_kind"
              l="Evidence type"
              v={evidence?.kind ?? "automated"}
              options={["automated", "manual"]}
            />
            <Field
              n="evidence_resource_kind"
              l="Resource type"
              v={evidence?.resource_kind ?? "check_run"}
              required={false}
            />
            <Field
              n="evidence_resource_id"
              l="Resource ID"
              v={evidence?.resource_id}
              required={false}
            />
            <Field
              n="evidence_revision"
              l="Exact revision"
              v={evidence?.revision}
              required={false}
            />
            <Select
              n="evidence_status"
              l="Current status"
              v={evidence?.status ?? "passing"}
              options={["passing", "failing", "missing", "stale", "unknown"]}
            />
            <Field
              n="evidence_summary"
              l="What it demonstrates"
              v={evidence?.summary}
              required={false}
            />
          </Group>
          <Group title="Bounded exception (optional)">
            <Field
              n="exception_id"
              l="Exception key"
              v={current?.exceptions[0]?.id}
              required={false}
            />
            <Field
              n="exception_expiry"
              l="Expiry"
              type="datetime-local"
              v={localDate(current?.exceptions[0]?.expires_at)}
              required={false}
            />
            <Field
              n="exception_grantor"
              l="Grantor ID"
              v={current?.exceptions[0]?.granted_by ?? user?.id}
              required={false}
            />
            <Field
              n="exception_rationale"
              l="Rationale"
              v={current?.exceptions[0]?.rationale}
              required={false}
            />
            <Field
              n="exception_follow_up"
              l="Follow-up work"
              v={current?.exceptions[0]?.follow_up}
              required={false}
            />
          </Group>
          <Button disabled={busy}>
            {busy
              ? "Publishing…"
              : selected
                ? `Publish version ${selected.current_version + 1}`
                : "Create quality plan"}
          </Button>
        </form>
      </Card>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      {selected && current && (
        <Card className="p-5">
          <div className="flex flex-wrap gap-2">
            <h2 className="text-lg font-semibold">{current.title}</h2>
            <Badge>version {selected.current_version}</Badge>
            <Badge
              tone={
                selected.diagnostics.some((x) => x.severity === "blocking")
                  ? "danger"
                  : "success"
              }
            >
              {selected.diagnostics.length
                ? `${selected.diagnostics.length} explicit gap(s)`
                : "intent complete"}
            </Badge>
          </div>
          <p className="mt-2 text-sm text-[var(--muted)]">
            {current.requirements.length} behavior(s) ·{" "}
            {current.supported_environments.length} environment(s) ·{" "}
            {current.evidence.length} evidence link(s)
          </p>
          <div className="mt-4 grid gap-3 md:grid-cols-2">
            {selected.diagnostics.map((d, i) => (
              <div className="rounded-lg border p-3" key={`${d.kind}-${i}`}>
                <Badge tone={d.severity === "blocking" ? "danger" : "warning"}>
                  {d.kind.replaceAll("_", " ")}
                </Badge>
                <p className="mt-2 text-sm">{d.message}</p>
                <p className="mt-1 text-xs text-[var(--muted)]">
                  {d.requirement_id || "plan"} · attributed to {d.attributed_to}
                </p>
              </div>
            ))}
          </div>
          <details className="mt-4">
            <summary className="cursor-pointer text-sm font-semibold">
              Version history
            </summary>
            {selected.revisions.map((r) => (
              <p className="mt-2 rounded-lg border p-3 text-sm" key={r.version}>
                <strong>v{r.version}</strong> · {r.rationale} · {r.created_by} ·{" "}
                {new Date(r.created_at).toLocaleString()}
              </p>
            ))}
          </details>
        </Card>
      )}
      <Card className="p-5">
        <h2 className="font-semibold">Propose an executable scenario</h2>
        <p className="mt-1 text-sm text-[var(--muted)]">
          Commit the test and synthetic fixture on an ordinary branch first.
          This record verifies their exact revision and digest; it never copies
          fixture contents.
        </p>
        <form className="mt-4 space-y-4" onSubmit={createScenario}>
          <Group title="Rationale and revision">
            <Field n="scenario_title" l="Scenario title" />
            <Field
              n="scenario_requirement"
              l="Quality requirement key"
              v={requirement?.id}
              required={Boolean(selected)}
            />
            <Select
              n="scenario_source_kind"
              l="Source type"
              v="issue"
              options={[
                "issue",
                "reproduction",
                "design_specification",
                "api_contract",
                "documentation",
                "user_journey",
              ]}
            />
            <Field n="scenario_source_id" l="Source resource ID" />
            <Field n="scenario_revision" l="Exact 40-character branch commit" />
            <Field
              n="scenario_source_path"
              l="Source path at that commit"
              required={false}
            />
          </Group>
          <Area n="scenario_purpose" l="What this scenario proves" />
          <Area n="scenario_source_summary" l="Source rationale summary" />
          <Group title="Behavior">
            <Field n="case_parameter" l="Parameter name" />
            <Field n="parameter_description" l="Parameter meaning" />
            <Field n="case_value" l="Example case value" />
            <Field n="precondition" l="Precondition" />
            <Field n="precondition_operation" l="Precondition operation" />
            <Field n="action" l="Action" />
            <Field n="action_operation" l="Action operation" />
            <Field n="assertion" l="Assertion" />
            <Field
              n="matcher"
              l="Matcher"
              placeholder="equals, contains, count_equals"
            />
            <Field n="expected" l="Expected value" />
          </Group>
          <Group title="Safe fixture">
            <Select
              n="fixture_kind"
              l="Fixture kind"
              v="synthetic"
              options={["synthetic", "generated", "template"]}
            />
            <Select
              n="data_class"
              l="Permitted data class"
              v="synthetic"
              options={["synthetic", "anonymized", "public"]}
            />
            <Field n="fixture_path" l="Fixture path in commit" />
            <Field n="fixture_digest" l="Fixture SHA-256" />
            <Field n="fixture_description" l="Fixture purpose" />
            <Field
              n="generator"
              l="Deterministic generator command"
              required={false}
            />
            <Field
              n="fixture_assumptions"
              l="Fixture assumptions"
              placeholder="No real identity, deterministic seed"
            />
          </Group>
          <Group title="Case and environment">
            <Field n="case_name" l="Case name" />
            <Field n="case_outcome" l="Expected outcome" />
            <Field n="case_assumptions" l="Case assumptions" />
            <Field n="scenario_environment" l="Environment boundary" />
            <Field n="runtime" l="Runtime" />
            <Field n="environment_requirements" l="Environment requirements" />
          </Group>
          <Group title="Ordinary Git proposal">
            <Select
              n="authored_by_type"
              l="Author type"
              v="human"
              options={["human", "agent"]}
            />
            <Field n="branch" l="Branch name" />
            <Field n="test_paths" l="Test paths" />
            <Field n="command" l="Rerun command" />
            <Field n="framework" l="Framework" />
            <Field n="pull_id" l="Pull request ID" required={false} />
            <Field n="workspace_id" l="Workspace ID" required={false} />
            <Field
              n="implementation_assumptions"
              l="Implementation assumptions"
            />
            <Field n="provenance" l="Generation provenance" />
            <label className="flex items-center gap-2 text-xs font-semibold">
              <input name="generated" type="checkbox" /> Generated case
            </label>
          </Group>
          <Button disabled={busy}>
            {busy ? "Verifying…" : "Create immutable scenario"}
          </Button>
        </form>
      </Card>
      <section>
        <h2 className="text-lg font-semibold">Reusable scenarios</h2>
        <div className="mt-3 space-y-3">
          {scenarios.map((s) => (
            <Card className="p-4" key={s.id}>
              <div className="flex flex-wrap gap-2">
                <strong>{s.title}</strong>
                <Badge>{s.implementation.framework}</Badge>
                <Badge tone="success">
                  {s.fixtures[0]?.data_class} fixture
                </Badge>
                {s.implementation.generated && <Badge>generated</Badge>}
              </div>
              <p className="mt-2 text-sm">{s.purpose}</p>
              <p className="mt-2 font-mono text-xs">
                {s.implementation.command}
              </p>
              <p className="mt-2 text-xs text-[var(--muted)]">
                {s.cases.length} parameterized case(s) · {s.assertions.length}{" "}
                assertion(s) ·{" "}
                {s.sources
                  .map(
                    (x) =>
                      `${x.kind}:${x.resource_id}@${x.revision.slice(0, 8)}`,
                  )
                  .join(", ")}{" "}
                · by {s.created_by}
              </p>
              <details className="mt-3 text-sm">
                <summary className="cursor-pointer font-semibold">
                  Review assumptions and proof
                </summary>
                <ol className="mt-2 list-decimal space-y-1 pl-5">
                  {s.preconditions.map((x) => (
                    <li key={x.id}>{x.description}</li>
                  ))}
                  {s.actions.map((x) => (
                    <li key={x.id}>{x.description}</li>
                  ))}
                </ol>
                {s.assertions.map((x) => (
                  <p className="mt-2" key={x.id}>
                    <strong>Assert:</strong> {x.description} ({x.matcher}{" "}
                    {x.expected})
                  </p>
                ))}
                <p className="mt-2">
                  <strong>Fixture assumptions:</strong>{" "}
                  {s.fixtures.flatMap((x) => x.assumptions).join("; ")}
                </p>
                <p>
                  <strong>Implementation assumptions:</strong>{" "}
                  {s.implementation.assumptions.join("; ")}
                </p>
              </details>
            </Card>
          ))}
        </div>
      </section>
      <section>
        <h2 className="text-lg font-semibold">Repository quality plans</h2>
        <div className="mt-3 space-y-2">
          {plans.map((plan) => {
            const r = plan.revisions.at(-1)!;
            return (
              <button
                type="button"
                className="w-full rounded-xl border bg-white p-4 text-left hover:border-[var(--brand)]"
                key={plan.id}
                onClick={() => setSelected(plan)}
              >
                <span className="font-semibold">{r.title}</span>
                <span className="block text-xs text-[var(--muted)]">
                  v{plan.current_version} · {r.requirements.length} protected
                  behavior(s) · {plan.diagnostics.length} explicit state(s)
                </span>
              </button>
            );
          })}
        </div>
      </section>
    </div>
  );
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
  placeholder,
}: {
  n: string;
  l: string;
  v?: string;
  type?: string;
  required?: boolean;
  placeholder?: string;
}) {
  return (
    <label className="text-xs font-semibold">
      {l}
      <input
        name={n}
        type={type}
        required={required}
        defaultValue={v}
        placeholder={placeholder}
        className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal"
      />
    </label>
  );
}
function Area({
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
    <label className="block text-xs font-semibold">
      {l}
      <textarea
        name={n}
        required={required}
        rows={2}
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
          <option key={x} value={x}>
            {x.replaceAll("_", " ")}
          </option>
        ))}
      </select>
    </label>
  );
}
function localDate(value?: string) {
  if (!value) return undefined;
  const date = new Date(value);
  const offset = date.getTimezoneOffset();
  return new Date(date.getTime() - offset * 60000).toISOString().slice(0, 16);
}
function replaceAssociation(
  values: string[],
  previous: string | undefined,
  next: string,
) {
  return Array.from(
    new Set(values.map((value) => (value === previous ? next : value))),
  );
}
