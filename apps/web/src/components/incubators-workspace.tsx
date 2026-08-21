"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, type Incubator } from "@/lib/api";
import { AccessGate, useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

const lines = (value: FormDataEntryValue | null) =>
  String(value ?? "")
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
export function IncubatorsWorkspace() {
  const { user, token } = useAuth();
  const [items, setItems] = useState<Incubator[]>([]);
  const [current, setCurrent] = useState<Incubator | null>(null);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const r = await api<{ incubators: Incubator[] }>(
        "/incubators",
        {},
        token,
      );
      setItems(r.incubators);
      setCurrent((selected) =>
        selected
          ? (r.incubators.find((x) => x.id === selected.id) ?? null)
          : null,
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unable to load incubators.");
    }
  }, [token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !user) return;
    setPending(true);
    setError("");
    const f = new FormData(e.currentTarget);
    const kind = String(f.get("source_kind"));
    try {
      const x = await api<Incubator>(
        "/incubators",
        {
          method: "POST",
          body: JSON.stringify({
            title: f.get("title"),
            audience: f.get("audience"),
            problem: f.get("problem"),
            desired_outcome: f.get("desired_outcome"),
            constraints: lines(f.get("constraints")),
            success_measures: lines(f.get("success_measures")),
            sponsor_ids: [user.id, ...lines(f.get("sponsors"))],
            visibility: f.get("visibility"),
            source: {
              kind,
              label: f.get("source_label"),
              repository_id:
                kind === "new_idea" ? undefined : f.get("repository_id"),
              resource_id:
                kind === "new_idea" ? undefined : f.get("resource_id"),
            },
            decision_rights: [
              {
                kind: "scope_change",
                decision: f.get("decision"),
                principal_ids: [user.id],
                rule: f.get("decision_rule"),
              },
              {
                kind: "visibility_change",
                decision: "Change incubator visibility",
                principal_ids: [user.id],
                rule: "owner",
              },
            ],
            invitations: lines(f.get("invitees")).map((v) => {
              const [id, role] = v.split("|").map((s) => s.trim());
              return {
                principal_type: "human",
                principal_id: id,
                role: role || "participant",
              };
            }),
          }),
        },
        token,
      );
      setItems((v) => [x, ...v]);
      setCurrent(x);
      setCreating(false);
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Unable to open incubator.",
      );
    } finally {
      setPending(false);
    }
  }
  async function append(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !current) return;
    setPending(true);
    const f = new FormData(e.currentTarget);
    try {
      const x = await api<Incubator>(
        `/incubators/${current.id}/events`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: current.version,
            kind: f.get("kind"),
            body: f.get("body"),
            visibility: "participants",
          }),
        },
        token,
      );
      setCurrent(x);
      setItems((v) => v.map((i) => (i.id === x.id ? x : i)));
      e.currentTarget.reset();
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Unable to append context.",
      );
    } finally {
      setPending(false);
    }
  }
  const replaceCurrent = (x: Incubator) => {
    setCurrent(x);
    setItems((v) => v.map((i) => (i.id === x.id ? x : i)));
  };
  async function compare(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !current) return;
    setPending(true);
    setError("");
    const f = new FormData(e.currentTarget);
    const sourceKind = String(f.get("evidence_kind"));
    try {
      const x = await api<Incubator>(
        `/incubators/${current.id}/alternatives`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: current.version,
            sources: [
              {
                kind: sourceKind,
                label: f.get("evidence_label"),
                url:
                  sourceKind === "public"
                    ? f.get("evidence_locator")
                    : undefined,
                organization_id:
                  sourceKind === "organization"
                    ? f.get("evidence_locator")
                    : undefined,
                repository_id: !["public", "organization"].includes(sourceKind)
                  ? f.get("evidence_repository")
                  : undefined,
                resource_id: !["public"].includes(sourceKind)
                  ? f.get("evidence_locator")
                  : undefined,
                revision: sourceKind === "code" ? f.get("revision") : undefined,
                path: sourceKind === "code" ? f.get("path") : undefined,
              },
            ],
            alternative: {
              name: f.get("name"),
              product_boundary: f.get("product_boundary"),
              architecture: f.get("architecture"),
              interfaces: lines(f.get("interfaces")),
              dependencies: lines(f.get("dependencies")),
              licenses: lines(f.get("licenses")),
              operating_costs: lines(f.get("operating_costs")),
              security_risks: lines(f.get("security_risks")),
              data_risks: lines(f.get("data_risks")),
              build_or_adopt: f.get("build_or_adopt"),
              unknowns: lines(f.get("unknowns")),
            },
          }),
        },
        token,
      );
      replaceCurrent(x);
      e.currentTarget.reset();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Unable to retain comparison.",
      );
    } finally {
      setPending(false);
    }
  }
  async function defineExperiment(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !current) return;
    setPending(true);
    const f = new FormData(e.currentTarget);
    try {
      const x = await api<Incubator>(
        `/incubators/${current.id}/experiments`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: current.version,
            experiment: {
              alternative_id: f.get("alternative_id"),
              question: f.get("question"),
              environment: f.get("environment"),
              commands: lines(f.get("commands")),
              inputs: lines(f.get("inputs")),
              expected_measures: lines(f.get("expected_measures")),
              safety_limits: lines(f.get("safety_limits")),
              source_ids: [],
            },
          }),
        },
        token,
      );
      replaceCurrent(x);
      e.currentTarget.reset();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Unable to define experiment.",
      );
    } finally {
      setPending(false);
    }
  }
  async function researchNote(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !current) return;
    setPending(true);
    const f = new FormData(e.currentTarget);
    try {
      const x = await api<Incubator>(
        `/incubators/${current.id}/research-notes`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: current.version,
            note: {
              kind: f.get("note_kind"),
              body: f.get("note_body"),
              alternative_id: f.get("note_alternative") || undefined,
              source_ids: [],
              supersedes_id: f.get("supersedes_id") || undefined,
            },
          }),
        },
        token,
      );
      replaceCurrent(x);
      e.currentTarget.reset();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Unable to retain research note.",
      );
    } finally {
      setPending(false);
    }
  }
  const bootstrapKinds = [
    "organization",
    "repository",
    "team",
    "package",
    "agent_role",
    "contributor_pathway",
    "documentation",
    "environment",
    "review_policy",
    "security_policy",
    "privacy_policy",
    "quality_policy",
    "release_policy",
  ];
  async function previewBootstrap(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !current || !user) return;
    setPending(true);
    setError("");
    const f = new FormData(e.currentTarget);
    const prefix = String(f.get("project_name"));
    const monthly = Number(f.get("monthly_cost"));
    try {
      const x = await api<Incubator>(
        `/incubators/${current.id}/bootstrap-previews`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: current.version,
            alternative_id: f.get("bootstrap_alternative"),
            resources: bootstrapKinds.map((kind) => ({
              kind,
              mode: "create",
              name: `${prefix}-${kind.replaceAll("_", "-")}`,
              owner_ids: [user.id],
              monthly_cost_estimate_cents:
                kind === "environment" ? Math.round(monthly * 100) : 0,
            })),
          }),
        },
        token,
      );
      replaceCurrent(x);
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Unable to reserve the project boundary.",
      );
    } finally {
      setPending(false);
    }
  }
  async function bootstrapDecision(
    plan: string,
    version: number,
    decision: string,
  ) {
    if (!token || !current) return;
    setPending(true);
    try {
      replaceCurrent(
        await api<Incubator>(
          `/incubators/${current.id}/bootstrap-plans/${plan}/decisions`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: current.version,
              plan_version: version,
              decision,
            }),
          },
          token,
        ),
      );
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Unable to record owner approval.",
      );
    } finally {
      setPending(false);
    }
  }
  async function bootstrapAction(
    plan: string,
    version: number,
    action: string,
  ) {
    if (!token || !current) return;
    setPending(true);
    try {
      replaceCurrent(
        await api<Incubator>(
          `/incubators/${current.id}/bootstrap-plans/${plan}/actions`,
          {
            method: "POST",
            body: JSON.stringify({
              expected_version: current.version,
              plan_version: version,
              action,
            }),
          },
          token,
        ),
      );
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Unable to update the project boundary.",
      );
    } finally {
      setPending(false);
    }
  }
  async function consent(invitation: string, decision: string) {
    if (!token || !current) return;
    try {
      const x = await api<Incubator>(
        `/incubators/${current.id}/invitations/${invitation}/consent`,
        {
          method: "POST",
          body: JSON.stringify({ expected_version: current.version, decision }),
        },
        token,
      );
      setCurrent(x);
      setItems((v) => v.map((i) => (i.id === x.id ? x : i)));
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Unable to record consent.",
      );
    }
  }
  return (
    <AccessGate>
      <div className="space-y-6">
        <header className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <p className="font-mono text-xs font-semibold uppercase tracking-widest text-[var(--brand)]">
              Before the repository
            </p>
            <h1 className="mt-2 text-3xl font-semibold">Project incubators</h1>
            <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
              Establish why a project should exist, who it serves, and who may
              shape it before implementation choices harden.
            </p>
          </div>
          <Button onClick={() => setCreating(!creating)}>
            {creating ? "Cancel" : "Open an incubator"}
          </Button>
        </header>
        {error && (
          <p
            role="alert"
            className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
          >
            {error}
          </p>
        )}
        {creating && (
          <Card className="p-5">
            <form onSubmit={create} className="grid gap-4 md:grid-cols-2">
              <Field name="title" label="Working title" />
              <Select
                name="source_kind"
                label="Starting point"
                values={[
                  "new_idea",
                  "feedback",
                  "support_gap",
                  "governed_proposal",
                ]}
              />
              <Field name="source_label" label="Source label" />
              <Field
                name="repository_id"
                label="Source repository ID"
                optional
              />
              <Field name="resource_id" label="Source resource ID" optional />
              <Select
                name="visibility"
                label="Visibility"
                values={["participants", "private", "public"]}
              />
              <Area name="audience" label="Audience" />
              <Area name="problem" label="Problem" />
              <Area name="desired_outcome" label="Desired outcome" />
              <Area
                name="constraints"
                label="Constraints"
                hint="One per line"
              />
              <Area
                name="success_measures"
                label="Success measures"
                hint="One per line"
              />
              <Area
                name="sponsors"
                label="Additional sponsor IDs"
                hint="One per line"
              />
              <Field
                name="decision"
                label="Decision right"
                placeholder="Who may change scope?"
              />
              <Select
                name="decision_rule"
                label="Decision rule"
                values={["owner", "consent", "majority", "consensus"]}
              />
              <Area
                name="invitees"
                label="Human invitations"
                hint="identity ID | role, one per line"
              />
              <Button disabled={pending} className="md:col-span-2">
                {pending ? "Opening…" : "Open collaborative home"}
              </Button>
            </form>
          </Card>
        )}
        <div className="grid gap-5 lg:grid-cols-[22rem_1fr]">
          <section className="space-y-3">
            {items.map((x) => (
              <button
                key={x.id}
                onClick={() => setCurrent(x)}
                className="w-full rounded-xl border bg-white p-4 text-left"
              >
                <div className="flex gap-2">
                  <Badge
                    tone={
                      x.source.resolution === "resolved" ? "success" : "warning"
                    }
                  >
                    {x.source.resolution}
                  </Badge>
                  <Badge>{x.visibility}</Badge>
                </div>
                <h2 className="mt-2 font-semibold">{x.title}</h2>
                <p className="mt-1 line-clamp-2 text-xs text-[var(--muted)]">
                  {x.problem}
                </p>
              </button>
            ))}
            {items.length === 0 && (
              <Card className="p-6 text-sm text-[var(--muted)]">
                No incubators are visible to you yet.
              </Card>
            )}
          </section>
          {current ? (
            <Card className="p-6">
              <div className="flex flex-wrap gap-2">
                <Badge
                  tone={
                    current.source.resolution === "resolved"
                      ? "success"
                      : "warning"
                  }
                >
                  {current.source.kind}: {current.source.resolution}
                </Badge>
                <Badge>{current.visibility}</Badge>
              </div>
              <h2 className="mt-3 text-2xl font-semibold">{current.title}</h2>
              <p className="mt-1 text-sm text-[var(--muted)]">
                {current.source.detail}
              </p>
              <dl className="mt-6 grid gap-4 sm:grid-cols-2">
                <Fact t="Audience" v={current.audience} />
                <Fact t="Problem" v={current.problem} />
                <Fact t="Desired outcome" v={current.desired_outcome} />
                <Fact t="Success" v={current.success_measures.join(" · ")} />
              </dl>
              {current.potential_duplicates.length > 0 && (
                <div className="mt-5 rounded-lg bg-[var(--warning-soft)] p-4">
                  <h3 className="font-semibold text-[var(--warning)]">
                    Potential duplicate initiatives
                  </h3>
                  {current.potential_duplicates.map((d) => (
                    <p key={d.incubator_id} className="mt-1 text-sm">
                      {d.title} — {d.reason}
                    </p>
                  ))}
                </div>
              )}
              <h3 className="mt-6 font-semibold">Participants and consent</h3>
              <div className="mt-2 space-y-2">
                {current.invitations.map((i) => (
                  <div
                    key={i.id}
                    className="flex items-center gap-2 rounded-lg border p-3 text-sm"
                  >
                    <span className="flex-1">
                      {i.principal_type} {i.principal_id} · {i.role}
                    </span>
                    <Badge>{i.status}</Badge>
                    {i.principal_id === user?.id && i.status === "pending" && (
                      <>
                        <Button
                          variant="secondary"
                          onClick={() => void consent(i.id, "declined")}
                        >
                          Decline
                        </Button>
                        <Button onClick={() => void consent(i.id, "accepted")}>
                          Accept
                        </Button>
                      </>
                    )}
                  </div>
                ))}
              </div>
              <h3 className="mt-6 font-semibold">Attributable context</h3>
              <div className="mt-2 space-y-2">
                {current.events.map((e) => (
                  <div key={e.id} className="rounded-lg border p-3">
                    <div className="flex gap-2">
                      <Badge>{e.kind}</Badge>
                      <span className="text-xs text-[var(--muted)]">
                        {e.actor_type} {e.actor_id}
                      </span>
                    </div>
                    <p className="mt-2 text-sm">{e.body}</p>
                  </div>
                ))}
              </div>
              <form
                onSubmit={append}
                className="mt-5 grid gap-3 sm:grid-cols-[11rem_1fr_auto]"
              >
                <Select
                  name="kind"
                  label="Context type"
                  values={[
                    "discussion",
                    "evidence",
                    "assumption",
                    "scope_change",
                  ]}
                />
                <Area name="body" label="Add context" />
                <Button disabled={pending} className="self-end">
                  Append
                </Button>
              </form>
              <h3 className="mt-8 font-semibold">Foundation comparisons</h3>
              <p className="mt-1 text-sm text-[var(--muted)]">
                Compare boundaries and foundations without turning a prototype
                into authoritative code or infrastructure.
              </p>
              <div className="mt-3 space-y-3">
                {(current.alternatives ?? []).map((a) => (
                  <div
                    key={a.id}
                    className={`rounded-lg border p-4 ${a.superseded ? "opacity-60" : ""}`}
                  >
                    <div className="flex gap-2">
                      <Badge>{a.build_or_adopt}</Badge>
                      {a.superseded && <Badge tone="warning">superseded</Badge>}
                    </div>
                    <h4 className="mt-2 font-semibold">{a.name}</h4>
                    <p className="mt-1 text-sm">{a.product_boundary}</p>
                    <p className="mt-1 text-xs text-[var(--muted)]">
                      Architecture: {a.architecture}
                    </p>
                    <p className="mt-1 text-xs text-[var(--muted)]">
                      Interfaces {a.interfaces.join(" · ")} · Dependencies{" "}
                      {a.dependencies.join(" · ")} · Licenses{" "}
                      {a.licenses.join(" · ")}
                    </p>
                    <p className="mt-1 text-xs text-[var(--muted)]">
                      Cost {a.operating_costs.join(" · ")} · Security{" "}
                      {a.security_risks.join(" · ")} · Data{" "}
                      {a.data_risks.join(" · ")}
                    </p>
                    <p className="mt-1 text-xs text-[var(--warning)]">
                      Unknowns: {a.unknowns.join(" · ")}
                    </p>
                  </div>
                ))}
              </div>
              <form
                onSubmit={compare}
                className="mt-4 grid gap-3 sm:grid-cols-2"
              >
                <Field name="name" label="Candidate name" />
                <Select
                  name="build_or_adopt"
                  label="Foundation choice"
                  values={["build", "adopt", "hybrid"]}
                />
                <Area name="product_boundary" label="Product boundary" />
                <Area name="architecture" label="Architecture" />
                {[
                  "interfaces",
                  "dependencies",
                  "licenses",
                  "operating_costs",
                  "security_risks",
                  "data_risks",
                  "unknowns",
                ].map((name) => (
                  <Area
                    key={name}
                    name={name}
                    label={name.replaceAll("_", " ")}
                    hint="One per line"
                  />
                ))}
                <Select
                  name="evidence_kind"
                  label="Evidence type"
                  values={[
                    "public",
                    "organization",
                    "decision",
                    "prototype",
                    "package",
                    "api_contract",
                    "code",
                  ]}
                />
                <Field name="evidence_label" label="Evidence label" />
                <Field
                  name="evidence_locator"
                  label="URL, organization, or resource ID"
                />
                <Field
                  name="evidence_repository"
                  label="Repository ID"
                  optional
                />
                <Field name="revision" label="Code revision" optional />
                <Field name="path" label="Code path" optional />
                <Button disabled={pending} className="sm:col-span-2">
                  Retain candidate
                </Button>
              </form>
              {(current.alternatives ?? []).length > 0 && (
                <>
                  <h3 className="mt-8 font-semibold">
                    Reproducible experiments
                  </h3>
                  <div className="mt-3 space-y-3">
                    {(current.experiments ?? []).map((x) => (
                      <div key={x.id} className="rounded-lg border p-4">
                        <Badge>
                          {x.results.length
                            ? x.results.at(-1)?.outcome
                            : "unrun"}
                        </Badge>
                        <p className="mt-2 text-sm font-semibold">
                          {x.question}
                        </p>
                        <p className="mt-1 font-mono text-xs break-all">
                          sha256 {x.definition_sha256}
                        </p>
                        <p className="mt-1 text-xs text-[var(--muted)]">
                          {x.authority}
                        </p>
                      </div>
                    ))}
                  </div>
                  <form
                    onSubmit={defineExperiment}
                    className="mt-4 grid gap-3 sm:grid-cols-2"
                  >
                    <label className="text-sm font-semibold">
                      Candidate
                      <select
                        name="alternative_id"
                        className="mt-2 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
                      >
                        {current.alternatives
                          .filter((x) => !x.superseded)
                          .map((x) => (
                            <option key={x.id} value={x.id}>
                              {x.name}
                            </option>
                          ))}
                      </select>
                    </label>
                    <Field name="question" label="Testable question" />
                    <Area name="environment" label="Bounded environment" />
                    <Area
                      name="commands"
                      label="Exact commands"
                      hint="One per line"
                    />
                    <Area
                      name="inputs"
                      label="Reproducible inputs"
                      hint="One per line"
                    />
                    <Area
                      name="expected_measures"
                      label="Expected measurements"
                      hint="One per line"
                    />
                    <Area
                      name="safety_limits"
                      label="Safety limits"
                      hint="One per line"
                    />
                    <Button disabled={pending} className="sm:col-span-2">
                      Define experiment
                    </Button>
                  </form>
                </>
              )}
              <h3 className="mt-8 font-semibold">
                Unknowns, measurements, and dissent
              </h3>
              <div className="mt-3 space-y-2">
                {(current.research_notes ?? []).map((n) => (
                  <div
                    key={n.id}
                    className={`rounded-lg border p-3 ${n.superseded ? "opacity-60" : ""}`}
                  >
                    <Badge>{n.kind}</Badge>
                    {n.superseded && <Badge tone="warning">superseded</Badge>}
                    <p className="mt-2 text-sm">{n.body}</p>
                    <p className="mt-1 text-xs text-[var(--muted)]">
                      {n.actor_type} {n.actor_id}
                    </p>
                  </div>
                ))}
              </div>
              <form
                onSubmit={researchNote}
                className="mt-4 grid gap-3 sm:grid-cols-2"
              >
                <Select
                  name="note_kind"
                  label="Finding type"
                  values={["unknown", "measurement", "assumption", "dissent"]}
                />
                <label className="text-sm font-semibold">
                  Candidate (optional)
                  <select
                    name="note_alternative"
                    className="mt-2 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
                  >
                    <option value="">Incubator-wide</option>
                    {(current.alternatives ?? []).map((x) => (
                      <option key={x.id} value={x.id}>
                        {x.name}
                      </option>
                    ))}
                  </select>
                </label>
                <Area name="note_body" label="Finding" />
                <Field
                  name="supersedes_id"
                  label="Superseded note ID"
                  optional
                />
                <Button disabled={pending} className="sm:col-span-2">
                  Retain finding
                </Button>
              </form>
              <h3 className="mt-8 font-semibold">Governed project boundary</h3>
              <p className="mt-1 text-sm text-[var(--muted)]">
                Preview names, owners, effective access, cost, generated
                defaults, and inherited policy before anything is activated.
              </p>
              <div className="mt-3 space-y-3">
                {(current.bootstrap_plans ?? []).map((p) => (
                  <div key={p.id} className="rounded-lg border p-4">
                    <div className="flex flex-wrap gap-2">
                      <Badge
                        tone={
                          p.status === "active"
                            ? "success"
                            : p.status === "rejected"
                              ? "warning"
                              : undefined
                        }
                      >
                        {p.status}
                      </Badge>
                      <Badge>
                        Estimated ${(p.recurring_cost_estimate_cents / 100).toFixed(2)}/month
                      </Badge>
                    </div>
                    <p className="mt-2 font-mono text-xs break-all">
                      {p.generated_from}
                    </p>
                    <div className="mt-3 grid gap-2 sm:grid-cols-2">
                      {p.resources.map((r) => (
                        <div
                          key={r.kind}
                          className="rounded border p-2 text-xs"
                        >
                          <strong>{r.kind.replaceAll("_", " ")}</strong> ·{" "}
                          {r.mode}
                          <p>{r.name}</p>
                          <p className="text-[var(--muted)]">
                            Owners {r.owner_ids.join(", ")} ·{" "}
                            {r.effective_access.join(" · ")}
                          </p>
                          <p className="text-[var(--muted)]">
                            {r.generated_content.join(" · ")} · inherits{" "}
                            {r.inherited_policies.join(" · ")}
                          </p>
                          <p className="text-[var(--muted)]">
                            {r.cost_basis} · {r.metadata_source}
                          </p>
                        </div>
                      ))}
                    </div>
                    <div className="mt-3 flex gap-2">
                      {p.status === "preview" && (
                        <>
                          <Button
                            disabled={pending}
                            onClick={() =>
                              void bootstrapDecision(
                                p.id,
                                p.version,
                                "approved",
                              )
                            }
                          >
                            Approve as owner
                          </Button>
                          <Button
                            variant="secondary"
                            disabled={pending}
                            onClick={() =>
                              void bootstrapAction(p.id, p.version, "rollback")
                            }
                          >
                            Release reservations
                          </Button>
                        </>
                      )}
                      {p.status === "approved" && (
                        <>
                          <Button
                            disabled={pending}
                            onClick={() =>
                              void bootstrapAction(p.id, p.version, "activate")
                            }
                          >
                            Activate atomically
                          </Button>
                          <Button
                            variant="secondary"
                            disabled={pending}
                            onClick={() =>
                              void bootstrapAction(p.id, p.version, "rollback")
                            }
                          >
                            Roll back
                          </Button>
                        </>
                      )}
                    </div>
                  </div>
                ))}
              </div>
              {(current.bootstrap_plans ?? []).every((p) =>
                ["rejected", "rolled_back"].includes(p.status),
              ) &&
                current.alternatives.some((a) => !a.superseded) && (
                  <form
                    onSubmit={previewBootstrap}
                    className="mt-4 grid gap-3 sm:grid-cols-2"
                  >
                    <Field
                      name="project_name"
                      label="Project identity prefix"
                    />
                    <Field
                      name="monthly_cost"
                      label="Environment cost (USD/month)"
                    />
                    <label className="text-sm font-semibold">
                      Accepted direction
                      <select
                        name="bootstrap_alternative"
                        className="mt-2 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
                      >
                        {current.alternatives
                          .filter((a) => !a.superseded)
                          .map((a) => (
                            <option key={a.id} value={a.id}>
                              {a.name}
                            </option>
                          ))}
                      </select>
                    </label>
                    <Button disabled={pending} className="self-end">
                      Preview complete boundary
                    </Button>
                  </form>
                )}
            </Card>
          ) : (
            <Card className="p-8 text-sm text-[var(--muted)]">
              Select an incubator to inspect its purpose, authority, consent,
              and discussion.
            </Card>
          )}
        </div>
      </div>
    </AccessGate>
  );
}
function Field({
  name,
  label,
  optional = false,
  placeholder,
}: {
  name: string;
  label: string;
  optional?: boolean;
  placeholder?: string;
}) {
  return (
    <label className="text-sm font-semibold">
      {label}
      <input
        required={!optional}
        name={name}
        placeholder={placeholder}
        className="mt-2 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
      />
    </label>
  );
}
function Area({
  name,
  label,
  hint,
}: {
  name: string;
  label: string;
  hint?: string;
}) {
  return (
    <label className="text-sm font-semibold">
      {label}
      <textarea
        name={name}
        rows={3}
        className="mt-2 w-full rounded-lg border bg-white p-3 font-normal"
      />
      {hint && (
        <span className="block text-xs font-normal text-[var(--muted)]">
          {hint}
        </span>
      )}
    </label>
  );
}
function Select({
  name,
  label,
  values,
}: {
  name: string;
  label: string;
  values: string[];
}) {
  return (
    <label className="text-sm font-semibold">
      {label}
      <select
        name={name}
        className="mt-2 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
      >
        {values.map((v) => (
          <option key={v}>{v}</option>
        ))}
      </select>
    </label>
  );
}
function Fact({ t, v }: { t: string; v: string }) {
  return (
    <div>
      <dt className="text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">
        {t}
      </dt>
      <dd className="mt-1 text-sm leading-6">{v}</dd>
    </div>
  );
}
