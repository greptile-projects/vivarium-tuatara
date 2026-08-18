"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Revision = {
  version: number;
  title: string;
  user_goal: string;
  source: { kind: string; resource_id: string; summary: string };
  journeys: { name: string; actor: string; goal: string; steps: string[] }[];
  states: { name: string; description: string; content: string }[];
  content: string[];
  constraints: string[];
  alternatives: string[];
  success_measures: string[];
  affected_components: string[];
  component_contracts: string[];
  breakpoints: string[];
  acceptance_criteria: string[];
  evidence: {
    kind: string;
    resource_id: string;
    summary: string;
    accessible: boolean;
    gap?: string;
  }[];
  artifacts: {
    id: string;
    kind: string;
    title: string;
    description: string;
    content: string;
    interactions: string[];
    audience: string[];
    author_id: string;
    license: string;
    source: string;
    transformations: string[];
  }[];
  uncertainty: string[];
  created_by: string;
};
type Proposal = {
  id: string;
  owner_ids: string[];
  current_version: number;
  revisions: Revision[];
  comments: {
    id: string;
    revision: number;
    body: string;
    kind: string;
    author_id: string;
  }[];
  acknowledgements: {
    revision: number;
    owner_id: string;
    status: string;
    note: string;
  }[];
  implementation?: {
    design_version: number;
    base_revision: string;
    proposal_id: string;
    task_ids: string[];
    mappings: {
      requirement: string;
      code_paths: string[];
      rendered_surfaces: string[];
    }[];
    deviations: {
      id: string;
      requirement: string;
      reason: string;
      impact: string;
      status: string;
    }[];
  };
};
const value = (f: FormData, n: string) => String(f.get(n) ?? "").trim();
const list = (v: string) =>
  v
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
const Field = ({ name, label }: { name: string; label: string }) => (
  <label className="grid gap-1 text-sm font-semibold">
    {label}
    <input
      required
      name={name}
      className="rounded-lg border border-[var(--line-strong)] bg-white px-3 py-2 font-normal"
    />
  </label>
);
const Area = ({ name, label }: { name: string; label: string }) => (
  <label className="grid gap-1 text-sm font-semibold">
    {label}
    <textarea
      required
      name={name}
      rows={3}
      className="rounded-lg border border-[var(--line-strong)] bg-white px-3 py-2 font-normal"
    />
  </label>
);

export function DesignProposalWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth(),
    [items, setItems] = useState<Proposal[]>([]),
    [selected, setSelected] = useState<Proposal>(),
    [error, setError] = useState("");
  const load = useCallback(async () => {
    try {
      const out = await api<{ design_proposals: Proposal[] }>(
        `/repositories/${repositoryID}/design-proposals`,
        {},
        token,
      );
      setItems(out.design_proposals);
      setSelected(
        (old) =>
          out.design_proposals.find((x) => x.id === old?.id) ??
          out.design_proposals[0],
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Design proposals could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget),
      owners = value(f, "owners")
        .split(",")
        .map((x) => x.trim())
        .filter(Boolean);
    const revision = {
      title: value(f, "title"),
      user_goal: value(f, "goal"),
      source: {
        kind: value(f, "source_kind"),
        resource_id: value(f, "source_id"),
        summary: value(f, "source_summary"),
      },
      journeys: [
        {
          name: value(f, "journey"),
          actor: value(f, "actor"),
          goal: value(f, "journey_goal"),
          steps: list(value(f, "steps")),
        },
      ],
      states: [
        {
          name: value(f, "state"),
          description: value(f, "state_description"),
          content: value(f, "state_content"),
        },
      ],
      content: list(value(f, "content")),
      constraints: list(value(f, "constraints")),
      alternatives: list(value(f, "alternatives")),
      success_measures: list(value(f, "measures")),
      affected_components: list(value(f, "components")),
      component_contracts: list(value(f, "contracts")),
      breakpoints: list(value(f, "breakpoints")),
      acceptance_criteria: list(value(f, "acceptance")),
      evidence: [],
      artifacts: [
        {
          id: "concept-1",
          kind: value(f, "artifact_kind"),
          title: value(f, "artifact_title"),
          description: value(f, "artifact_description"),
          content: value(f, "artifact_content"),
          interactions: list(value(f, "interactions")),
          audience: owners,
          author_id: user?.id ?? "",
          license: value(f, "asset_license"),
          source: value(f, "asset_source"),
          transformations: list(value(f, "transformations")),
        },
      ],
      uncertainty: list(value(f, "uncertainty")),
    };
    try {
      const out = await api<Proposal>(
        `/repositories/${repositoryID}/design-proposals`,
        {
          method: "POST",
          body: JSON.stringify({ owner_ids: owners, revision }),
        },
        token,
      );
      setSelected(out);
      await load();
      e.currentTarget.reset();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Proposal could not be opened.",
      );
    }
  }
  async function comment(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!selected) return;
    const f = new FormData(e.currentTarget);
    try {
      setSelected(
        await api(
          `/repositories/${repositoryID}/design-proposals/${selected.id}/comments`,
          {
            method: "POST",
            body: JSON.stringify({
              revision: selected.current_version,
              kind: value(f, "kind"),
              body: value(f, "body"),
              evidence: [],
            }),
          },
          token,
        ),
      );
      e.currentTarget.reset();
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Response could not be recorded.",
      );
    }
  }
  async function acknowledge(status: string) {
    if (!selected) return;
    try {
      setSelected(
        await api(
          `/repositories/${repositoryID}/design-proposals/${selected.id}/acknowledgements`,
          {
            method: "POST",
            body: JSON.stringify({
              revision: selected.current_version,
              status,
              note:
                status === "acknowledged"
                  ? "Behavior is ready for implementation planning."
                  : "Resolve the recorded concerns before implementation.",
            }),
          },
          token,
        ),
      );
      await load();
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Acknowledgement could not be recorded.",
      );
    }
  }
  const current = selected?.revisions.at(-1);
  return (
    <main id="main-content" className="space-y-6">
      <header>
        <p className="text-sm font-semibold text-[var(--brand)]">
          Behavior before code
        </p>
        <h1 className="text-3xl font-semibold">Product design proposals</h1>
        <p className="mt-2 max-w-3xl text-[var(--muted)]">
          Make journeys, states, language, constraints, alternatives, and
          measures challengeable before implementation choices harden.
        </p>
      </header>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      <div className="grid gap-6 xl:grid-cols-[22rem_1fr]">
        <div className="space-y-4">
          <Card className="p-5">
            <h2 className="font-semibold">Open proposal</h2>
            <form onSubmit={create} className="mt-4 grid gap-3">
              <Field name="title" label="Proposal title" />
              <Area name="goal" label="User goal" />
              <div className="grid grid-cols-2 gap-2">
                <Field name="source_kind" label="Source kind" />
                <Field name="source_id" label="Source ID" />
              </div>
              <Area name="source_summary" label="Why this source matters" />
              <Field name="owners" label="Owner IDs, comma separated" />
              <Field name="journey" label="Journey name" />
              <Field name="actor" label="Affected person" />
              <Area name="journey_goal" label="Journey outcome" />
              <Area name="steps" label="Journey steps, one per line" />
              <Field name="state" label="State name" />
              <Area name="state_description" label="State behavior" />
              <Area name="state_content" label="Exact product content" />
              {[
                ["content", "Content rules"],
                ["constraints", "Constraints"],
                ["alternatives", "Alternatives considered"],
                ["measures", "Success measures"],
                ["components", "Affected components"],
                ["uncertainty", "Uncertainty and gaps"],
              ].map(([n, l]) => (
                <Area key={n} name={n} label={`${l}, one per line`} />
              ))}
              <Field name="artifact_title" label="Wireframe title" />
              <label className="grid gap-1 text-sm font-semibold">
                Artifact kind
                <select name="artifact_kind" className="rounded-lg border border-[var(--line-strong)] bg-white px-3 py-2 font-normal">
                  <option value="wireframe">Wireframe</option>
                  <option value="prototype">Prototype</option>
                </select>
              </label>
              <Area name="artifact_description" label="Wireframe description" />
              <Area name="artifact_content" label="Layout / screen content" />
              <Area
                name="interactions"
                label="Prototype interactions, one per line"
              />
              <Area name="contracts" label="Component contracts, one per line" />
              <Area name="breakpoints" label="Responsive breakpoints, one per line" />
              <Area name="acceptance" label="Acceptance criteria, one per line" />
              <Field name="asset_license" label="Asset license" />
              <Field name="asset_source" label="Asset source" />
              <Area name="transformations" label="Asset transformations, one per line" />
              <Button type="submit">Open design proposal</Button>
            </form>
          </Card>
        </div>
        <div className="space-y-4">
          <Card className="p-5">
            <h2 className="font-semibold">Proposals</h2>
            <div className="mt-3 flex flex-wrap gap-2">
              {items.map((x) => (
                <button
                  key={x.id}
                  onClick={() => setSelected(x)}
                  className="rounded-full border px-3 py-1 text-sm"
                >
                  {x.revisions.at(-1)?.title}
                </button>
              ))}
            </div>
          </Card>
          {current && selected && (
            <>
              <Card className="p-6">
                <div className="flex flex-wrap gap-2">
                  <Badge tone="info">revision {current.version}</Badge>
                  <Badge tone="neutral">{current.source.kind}</Badge>
                </div>
                <h2 className="mt-3 text-2xl font-semibold">{current.title}</h2>
                <p className="mt-2">{current.user_goal}</p>
                <h3 className="mt-5 font-semibold">Journey</h3>
                {current.journeys.map((j) => (
                  <div
                    key={j.name}
                    className="mt-2 rounded-lg bg-[var(--surface)] p-4"
                  >
                    <strong>
                      {j.actor}: {j.goal}
                    </strong>
                    <ol className="mt-2 list-decimal pl-5 text-sm">
                      {j.steps.map((s) => (
                        <li key={s}>{s}</li>
                      ))}
                    </ol>
                  </div>
                ))}
                <div className="mt-5 grid gap-4 md:grid-cols-2">
                  <Summary title="Constraints" values={current.constraints} />
                  <Summary title="Alternatives" values={current.alternatives} />
                  <Summary title="Measures" values={current.success_measures} />
                  <Summary title="Uncertainty" values={current.uncertainty} />
                </div>
                <h3 className="mt-5 font-semibold">
                  Wireframes and prototypes
                </h3>
                {current.artifacts.map((a) => (
                  <div key={a.id} className="mt-2 rounded-xl border p-4">
                    <Badge tone="neutral">{a.kind}</Badge>
                    <h4 className="mt-2 font-semibold">{a.title}</h4>
                    <p className="text-sm text-[var(--muted)]">
                      {a.description}
                    </p>
                    <pre className="mt-3 whitespace-pre-wrap rounded-lg bg-[var(--surface)] p-4 text-sm">
                      {a.content}
                    </pre>
                  </div>
                ))}
              </Card>
              <Card className="p-6">
                <h2 className="font-semibold">Revision discussion</h2>
                <form onSubmit={comment} className="mt-3 grid gap-3">
                  <select name="kind" className="rounded-lg border p-2">
                    <option value="comment">Comment</option>
                    <option value="question">Question</option>
                    <option value="dissent">Dissent</option>
                  </select>
                  <Area name="body" label="Response" />
                  <Button type="submit">
                    Record against revision {selected.current_version}
                  </Button>
                </form>
                <div className="mt-4 space-y-2">
                  {selected.comments.map((c) => (
                    <div
                      key={c.id}
                      className="rounded-lg bg-[var(--surface)] p-3 text-sm"
                    >
                      <Badge
                        tone={c.kind === "dissent" ? "warning" : "neutral"}
                      >
                        {c.kind}
                      </Badge>
                      <p className="mt-2">{c.body}</p>
                      <span className="text-xs text-[var(--muted)]">
                        {c.author_id} · revision {c.revision}
                      </span>
                    </div>
                  ))}
                </div>
                {selected.owner_ids.includes(user?.id ?? "") && (
                  <div className="mt-5 flex gap-2">
                    <Button onClick={() => acknowledge("acknowledged")}>
                      Acknowledge behavior
                    </Button>
                    <Button
                      variant="secondary"
                      onClick={() => acknowledge("changes_requested")}
                    >
                      Request changes
                    </Button>
                  </div>
                )}
              </Card>
            </>
          )}
        </div>
      </div>
    </main>
  );
}
function Summary({ title, values }: { title: string; values: string[] }) {
  return (
    <section>
      <h3 className="font-semibold">{title}</h3>
      <ul className="mt-2 list-disc pl-5 text-sm">
        {values.map((v) => (
          <li key={v}>{v}</li>
        ))}
      </ul>
    </section>
  );
}
