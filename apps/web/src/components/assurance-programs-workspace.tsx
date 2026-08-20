"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Badge, Button, Card } from "@/components/ui";

type Revision = {
  version: number;
  title: string;
  summary: string;
  owner_ids: string[];
  review_period_days: number;
  requirements: {
    id: string;
    kind: string;
    authority: string;
    citation: string;
    title: string;
    summary: string;
    applicability: string;
    inherited_from?: string;
    owner_ids: string[];
    interpretation: string;
    conflicts_with: string[];
  }[];
  scopes: {
    id: string;
    kind: string;
    resource_id: string;
    revision?: string;
    path?: string;
    description: string;
  }[];
  controls: {
    id: string;
    title: string;
    objective: string;
    requirement_ids: string[];
    owner_ids: string[];
    review_period_days: number;
    mappings: { scope_id: string; purpose: string }[];
    evidence_criteria: {
      id: string;
      description: string;
      kind: string;
      resource_kind?: string;
      resource_id?: string;
      revision?: string;
    }[];
    claim: string;
  }[];
  exceptions: {
    id: string;
    requirement_ids: string[];
    control_ids: string[];
    rationale: string;
    granted_by: string;
    expires_at: string;
    follow_up: string;
  }[];
};
type Program = {
  id: string;
  current_version: number;
  revisions: Revision[];
  diagnostics: {
    kind: string;
    severity: string;
    message: string;
    requirement_id?: string;
    control_id?: string;
    attributed_to: string;
  }[];
};
const v = (f: FormData, n: string) => String(f.get(n) || "").trim();
const list = (x: string) =>
  x
    .split(",")
    .map((y) => y.trim())
    .filter(Boolean);
function Field({
  n,
  l,
  d,
  required = true,
}: {
  n: string;
  l: string;
  d?: string;
  required?: boolean;
}) {
  return (
    <label className="block text-sm font-medium">
      {l}
      <input
        name={n}
        required={required}
        defaultValue={d}
        className="mt-1 w-full rounded-lg border bg-transparent p-2.5"
      />
    </label>
  );
}

export function AssuranceProgramsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user, loading } = useAuth(),
    [programs, setPrograms] = useState<Program[]>([]),
    [selected, setSelected] = useState<Program | null>(null),
    [error, setError] = useState("");
  const load = useCallback(async () => {
    if (loading || !token) return;
    try {
      const x = await api<{ programs: Program[] }>(
        `/repositories/${repositoryID}/assurance-programs`,
        {},
        token,
      );
      setPrograms(x.programs);
      setSelected(
        (s) => x.programs.find((p) => p.id === s?.id) ?? x.programs[0] ?? null,
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Programs could not be loaded.",
      );
    }
  }, [loading, repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function publish(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget),
      req = v(f, "requirement_id"),
      scope = v(f, "scope_id"),
      control = v(f, "control_id"),
      owner = v(f, "owner") || user?.id || "",
      revision: Revision = {
        version: 0,
        title: v(f, "title"),
        summary: v(f, "summary"),
        owner_ids: list(owner),
        review_period_days: Number(v(f, "program_review")),
        requirements: [
          {
            id: req,
            kind: v(f, "requirement_kind"),
            authority: v(f, "authority"),
            citation: v(f, "citation"),
            title: v(f, "requirement_title"),
            summary: v(f, "requirement_summary"),
            applicability: v(f, "applicability"),
            inherited_from: v(f, "inherited_from"),
            owner_ids: list(v(f, "requirement_owners")),
            interpretation: v(f, "interpretation"),
            conflicts_with: list(v(f, "conflicts")),
          },
        ],
        scopes: [
          {
            id: scope,
            kind: v(f, "scope_kind"),
            resource_id: v(f, "resource_id"),
            revision: v(f, "revision"),
            path: v(f, "path"),
            description: v(f, "scope_description"),
          },
        ],
        controls: [
          {
            id: control,
            title: v(f, "control_title"),
            objective: v(f, "objective"),
            requirement_ids: [req],
            owner_ids: list(v(f, "control_owners")),
            review_period_days: Number(v(f, "control_review")),
            mappings: [{ scope_id: scope, purpose: v(f, "mapping_purpose") }],
            evidence_criteria: [
              {
                id: v(f, "evidence_id"),
                description: v(f, "evidence_description"),
                kind: v(f, "evidence_kind"),
                resource_kind: v(f, "evidence_resource_kind"),
                resource_id: v(f, "evidence_resource_id"),
                revision: v(f, "evidence_revision"),
              },
            ],
            claim: v(f, "claim"),
          },
        ],
        exceptions: v(f, "exception_id")
          ? [
              {
                id: v(f, "exception_id"),
                requirement_ids: [req],
                control_ids: [control],
                rationale: v(f, "exception_rationale"),
                granted_by: v(f, "exception_grantor"),
                expires_at: new Date(v(f, "exception_expiry")).toISOString(),
                follow_up: v(f, "exception_follow_up"),
              },
            ]
          : [],
      };
    const url = selected
      ? `/repositories/${repositoryID}/assurance-programs/${selected.id}/revisions`
      : `/repositories/${repositoryID}/assurance-programs`;
    try {
      const out = await api<Program>(
        url,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected?.current_version || 0,
            revision,
          }),
        },
        token,
      );
      setSelected(out);
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Program could not be published.",
      );
    }
  }
  const current = selected?.revisions.at(-1);
  return (
    <main id="main-content" className="space-y-6">
      <header>
        <Link
          href={`/repositories/${repositoryID}`}
          className="text-sm text-[var(--muted)]"
        >
          Repository
        </Link>
        <h1 className="mt-2 text-3xl font-semibold">Assurance program</h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Turn regulatory, contractual, and organization obligations into owned
          controls tied directly to project resources and evidence.
        </p>
      </header>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      <div className="grid gap-6 lg:grid-cols-[1fr_1.35fr]">
        <Card className="p-6">
          <h2 className="text-lg font-semibold">
            Current obligations and controls
          </h2>
          {!current ? (
            <p className="mt-3 text-sm text-[var(--muted)]">
              No assurance program has been published.
            </p>
          ) : (
            <>
              <p className="mt-2 text-sm">
                {current.title} · version {selected?.current_version}
              </p>
              <p className="mt-1 text-sm text-[var(--muted)]">
                {current.summary}
              </p>
              <div className="mt-4 space-y-3">
                {selected?.diagnostics.map((d, i) => (
                  <div key={`${d.kind}-${i}`} className="rounded-lg border p-3">
                    <Badge
                      tone={
                        d.severity === "blocking"
                          ? "danger"
                          : d.severity === "warning"
                            ? "warning"
                            : "info"
                      }
                    >
                      {d.kind.replaceAll("_", " ")}
                    </Badge>
                    <p className="mt-2 text-sm">{d.message}</p>
                    <p className="mt-1 text-xs text-[var(--muted)]">
                      {d.requirement_id || d.control_id || "program"} ·{" "}
                      {d.attributed_to}
                    </p>
                  </div>
                ))}
              </div>
              {current.requirements.map((q) => (
                <div
                  key={q.id}
                  className="mt-4 rounded-lg bg-[var(--surface)] p-4"
                >
                  <div className="flex gap-2">
                    <Badge>{q.kind}</Badge>
                    <strong>{q.title}</strong>
                  </div>
                  <p className="mt-2 text-sm">
                    {q.authority} · {q.citation}
                  </p>
                  <p className="mt-1 text-sm">
                    <strong>Applies:</strong> {q.applicability}
                  </p>
                  <p className="mt-1 text-sm">
                    <strong>Interpretation:</strong> {q.interpretation}
                  </p>
                  {q.inherited_from && (
                    <p className="mt-1 text-xs">
                      Inherited from {q.inherited_from}
                    </p>
                  )}
                </div>
              ))}
              {current.controls.map((c) => (
                <div key={c.id} className="mt-3 rounded-lg border p-4">
                  <strong>{c.title}</strong>
                  <p className="mt-1 text-sm">{c.objective}</p>
                  <p className="mt-2 text-sm">
                    <strong>Claim:</strong> {c.claim}
                  </p>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    {c.mappings
                      .map((m) => `${m.scope_id}: ${m.purpose}`)
                      .join(" · ")}
                  </p>
                </div>
              ))}
            </>
          )}
        </Card>
        <Card className="p-6">
          <h2 className="text-lg font-semibold">
            {selected ? "Publish complete successor" : "Create program"}
          </h2>
          <form onSubmit={publish} className="mt-4 grid gap-3 sm:grid-cols-2">
            <Field n="title" l="Program title" />
            <Field n="owner" l="Program owner IDs" d={user?.id} />
            <label className="text-sm font-medium">
              Program review (days)
              <input
                name="program_review"
                type="number"
                min="1"
                defaultValue="90"
                className="mt-1 w-full rounded-lg border bg-transparent p-2.5"
              />
            </label>
            <Field n="summary" l="Program summary" />
            <Field n="requirement_id" l="Requirement key" d="REQ-1" />
            <label className="text-sm font-medium">
              Requirement source
              <select
                name="requirement_kind"
                className="mt-1 w-full rounded-lg border bg-transparent p-2.5"
              >
                <option value="regulatory">Regulatory</option>
                <option value="contractual">Contractual</option>
                <option value="organization">Organization</option>
              </select>
            </label>
            <Field n="authority" l="Authority or source" />
            <Field n="citation" l="Exact citation" />
            <Field n="requirement_title" l="Obligation title" />
            <Field n="requirement_summary" l="Obligation summary" />
            <Field n="applicability" l="Applicability rule" />
            <Field n="interpretation" l="Project interpretation" />
            <Field
              n="requirement_owners"
              l="Requirement owner IDs"
              d={user?.id}
            />
            <Field n="inherited_from" l="Inherited from" required={false} />
            <Field
              n="conflicts"
              l="Conflicting requirement IDs"
              required={false}
            />
            <Field n="scope_id" l="Scope key" d="scope-1" />
            <label className="text-sm font-medium">
              Mapped resource type
              <select
                name="scope_kind"
                className="mt-1 w-full rounded-lg border bg-transparent p-2.5"
              >
                <option value="repository">Repository</option>
                <option value="policy">Policy</option>
                <option value="data_flow">Data flow</option>
                <option value="infrastructure">Infrastructure</option>
                <option value="environment">Environment</option>
                <option value="release">Release</option>
                <option value="procedure">Procedure</option>
              </select>
            </label>
            <Field n="resource_id" l="Exact resource ID" d={repositoryID} />
            <Field
              n="revision"
              l="Exact revision (optional)"
              required={false}
            />
            <Field n="path" l="Repository path (optional)" required={false} />
            <Field n="scope_description" l="Scope description" />
            <Field n="control_id" l="Control key" d="CTRL-1" />
            <Field n="control_title" l="Control title" />
            <Field n="objective" l="Control objective" />
            <Field n="control_owners" l="Control owner IDs" d={user?.id} />
            <label className="text-sm font-medium">
              Control review (days)
              <input
                name="control_review"
                type="number"
                min="1"
                defaultValue="30"
                className="mt-1 w-full rounded-lg border bg-transparent p-2.5"
              />
            </label>
            <Field n="claim" l="How the project satisfies it" />
            <Field
              n="mapping_purpose"
              l="Why this resource implements the control"
            />
            <Field n="evidence_id" l="Evidence criterion key" d="evidence-1" />
            <label className="text-sm font-medium">
              Evidence kind
              <select
                name="evidence_kind"
                className="mt-1 w-full rounded-lg border bg-transparent p-2.5"
              >
                <option value="automated">Automated</option>
                <option value="manual">Manual</option>
                <option value="attestation">Attestation</option>
                <option value="record">Record</option>
              </select>
            </label>
            <Field n="evidence_description" l="Passing evidence criteria" />
            <Field
              n="evidence_resource_kind"
              l="Evidence resource type"
              required={false}
            />
            <Field
              n="evidence_resource_id"
              l="Evidence resource ID"
              required={false}
            />
            <Field
              n="evidence_revision"
              l="Evidence revision"
              required={false}
            />
            <Field
              n="exception_id"
              l="Exception key (optional)"
              required={false}
            />
            <Field
              n="exception_rationale"
              l="Exception rationale"
              required={false}
            />
            <Field
              n="exception_grantor"
              l="Exception grantor ID"
              d={user?.id}
              required={false}
            />
            <label className="text-sm font-medium">
              Exception expiry
              <input
                name="exception_expiry"
                type="datetime-local"
                className="mt-1 w-full rounded-lg border bg-transparent p-2.5"
              />
            </label>
            <Field
              n="exception_follow_up"
              l="Exception follow-up work"
              required={false}
            />
            <div className="sm:col-span-2">
              <Button type="submit">Publish immutable revision</Button>
            </div>
          </form>
        </Card>
      </div>
    </main>
  );
}
