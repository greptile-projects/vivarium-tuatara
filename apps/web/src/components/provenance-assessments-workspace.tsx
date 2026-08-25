"use client";
/* eslint-disable react-hooks/set-state-in-effect -- authenticated API hydration follows the existing workspace pattern */
import { FormEvent, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { useAuth } from "@/components/auth";
import { Badge, Button, Card } from "@/components/ui";
type Finding = {
  id: string;
  kind: string;
  severity: string;
  material_kind: string;
  summary: string;
  license?: string;
  origin?: string;
  obligations?: string[];
  distribution_targets?: string[];
  current: boolean;
  resolved: boolean;
};
type Repair = {
  id: string;
  finding_id: string;
  strategy: string;
  clean_room: boolean;
  proposal_id: string;
  acceptance_criteria: string[];
};
type Assessment = {
  id: string;
  version: number;
  candidate: { kind: string; id: string; revision: string };
  policy_version: number;
  ready: boolean;
  stale: boolean;
  findings: Finding[];
  events: {
    id: string;
    kind: string;
    finding_id: string;
    actor_id: string;
    body: string;
  }[];
  repairs?: Repair[];
};
const csv = (v: FormDataEntryValue | null) =>
  String(v || "")
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
export function ProvenanceAssessmentsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token } = useAuth();
  const [items, setItems] = useState<Assessment[]>([]),
    [error, setError] = useState("");
  const base = `/repositories/${repositoryID}/provenance-assessments`;
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const x = await api<{ assessments: Assessment[] }>(base, {}, token);
      setItems(x.assessments);
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Assessments could not be loaded.",
      );
    }
  }, [base, token]);
  useEffect(() => {
    void load();
  }, [load]);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    const f = new FormData(e.currentTarget);
    try {
      await api(
        base,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: String(f.get("request_id")),
            candidate: {
              kind: String(f.get("kind")),
              id: String(f.get("candidate_id")),
              revision: String(f.get("revision")),
            },
            graph_id: String(f.get("graph_id")),
            policy_id: String(f.get("policy_id")),
            policy_version: Number(f.get("policy_version")),
            distribution_targets: csv(f.get("targets")),
          }),
        },
        token,
      );
      await load();
      e.currentTarget.reset();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Assessment could not be created.",
      );
    }
  }
  async function event(e: FormEvent<HTMLFormElement>, a: Assessment) {
    e.preventDefault();
    if (!token) return;
    const f = new FormData(e.currentTarget),
      kind = String(f.get("kind"));
    const expires = String(f.get("expires"));
    try {
      await api(
        `${base}/${a.id}/events`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: a.version,
            event: {
              request_id: String(f.get("request_id")),
              kind,
              finding_id: String(f.get("finding_id")),
              body: String(f.get("body")),
              citations:
                kind === "origin_evidence"
                  ? [
                      {
                        kind: "repository_file",
                        resource_id: String(f.get("citation")),
                        revision: a.candidate.revision,
                        summary: "Exact candidate origin evidence",
                      },
                    ]
                  : [],
              exception_expires_at: expires
                ? new Date(expires).toISOString()
                : undefined,
              follow_up: String(f.get("follow_up")),
            },
          }),
        },
        token,
      );
      await load();
      e.currentTarget.reset();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Decision could not be recorded.",
      );
    }
  }
  async function repair(e: FormEvent<HTMLFormElement>, a: Assessment) {
    e.preventDefault();
    if (!token) return;
    const f = new FormData(e.currentTarget);
    try {
      await api(
        `${base}/${a.id}/repairs`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: a.version,
            request_id: String(f.get("request_id")),
            finding_id: String(f.get("finding_id")),
            strategy: String(f.get("strategy")),
            clean_room: f.get("clean_room") === "on",
            title: String(f.get("title")),
            acceptance_criteria: csv(f.get("criteria")),
            permitted_evidence: csv(f.get("evidence")).map((resource_id) => ({
              kind: "repository_file",
              resource_id,
              revision: a.candidate.revision,
              access: "repository",
            })),
            tasks: [
              {
                title: String(f.get("task")),
                assignee_type: String(f.get("assignee_type")),
                assignee_id: String(f.get("assignee_id")),
              },
            ],
          }),
        },
        token,
      );
      await load();
      e.currentTarget.reset();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Repair work could not be authorized.",
      );
    }
  }
  return (
    <div className="mx-auto max-w-6xl space-y-6 p-6">
      <header>
        <Link
          href={`/repositories/${repositoryID}/provenance/graph`}
          className="text-sm text-[var(--brand)]"
        >
          Revision-exact graph
        </Link>
        <h1 className="mt-2 text-3xl font-semibold">
          Candidate provenance readiness
        </h1>
        <p className="mt-2 text-[var(--muted)]">
          Compare an exact pull, stack, package, or release candidate with
          effective origin and licensing policy before acceptance.
        </p>
      </header>
      {error && (
        <p role="alert" className="text-[var(--danger)]">
          {error}
        </p>
      )}
      <Card>
        <form onSubmit={create} className="grid gap-3 md:grid-cols-2">
          <input
            required
            name="request_id"
            placeholder="Stable request identity"
            className="rounded border p-2"
          />
          <select name="kind" className="rounded border p-2">
            <option value="pull_request">Pull request</option>
            <option value="change_stack">Stack integration candidate</option>
            <option value="package_candidate">Package candidate</option>
            <option value="release_candidate">Release candidate</option>
          </select>
          <input
            required
            name="candidate_id"
            placeholder="Candidate ID"
            className="rounded border p-2"
          />
          <input
            required
            name="revision"
            minLength={40}
            maxLength={40}
            placeholder="Exact candidate commit"
            className="rounded border p-2 font-mono"
          />
          <input
            required
            name="graph_id"
            placeholder="Exact provenance graph ID"
            className="rounded border p-2"
          />
          <input
            required
            name="policy_id"
            placeholder="Effective policy ID"
            className="rounded border p-2"
          />
          <input
            required
            name="policy_version"
            type="number"
            min="1"
            placeholder="Policy revision"
            className="rounded border p-2"
          />
          <input
            required
            name="targets"
            placeholder="Distribution targets, comma separated"
            className="rounded border p-2"
          />
          <Button>Assess exact candidate</Button>
        </form>
      </Card>
      {items.map((a) => (
        <Card key={a.id}>
          <div className="flex flex-wrap justify-between gap-2">
            <div>
              <h2 className="font-semibold">
                {a.candidate.kind.replaceAll("_", " ")} · {a.candidate.id}
              </h2>
              <p className="font-mono text-xs text-[var(--muted)]">
                {a.candidate.revision}
              </p>
            </div>
            <div className="flex gap-2">
              <Badge tone={a.ready ? "success" : "danger"}>
                {a.ready ? "ready" : "blocked"}
              </Badge>
              {a.stale && <Badge tone="warning">partially stale</Badge>}
            </div>
          </div>
          <div className="mt-4 space-y-2">
            {a.findings.map((f) => (
              <div key={f.id} className="rounded border p-3">
                <div className="flex flex-wrap gap-2">
                  <Badge
                    tone={f.severity === "blocking" ? "danger" : "warning"}
                  >
                    {f.kind.replaceAll("_", " ")}
                  </Badge>
                  <Badge>{f.material_kind}</Badge>
                  <Badge
                    tone={
                      f.current
                        ? f.resolved
                          ? "success"
                          : "warning"
                        : "danger"
                    }
                  >
                    {!f.current
                      ? "stale"
                      : f.resolved
                        ? "decided"
                        : "decision required"}
                  </Badge>
                </div>
                <p className="mt-2 text-sm">{f.summary}</p>
                <p className="mt-1 text-xs text-[var(--muted)]">
                  {[
                    f.origin,
                    f.license,
                    ...(f.obligations || []),
                    ...(f.distribution_targets || []),
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </p>
              </div>
            ))}
          </div>
          {(a.repairs || []).map((r) => (
            <div key={r.id} className="mt-3 rounded border p-3 text-sm">
              <Badge tone="success">{r.strategy.replaceAll("_", " ")}</Badge>{" "}
              {r.clean_room && <Badge>clean room</Badge>}
              <p className="mt-2">{r.acceptance_criteria.join(" · ")}</p>
              <Link
                className="text-[var(--brand)]"
                href={`/proposals/${repositoryID}/${r.proposal_id}`}
              >
                Open ordinary repair work
              </Link>
            </div>
          ))}
          <form
            onSubmit={(e) => repair(e, a)}
            className="mt-4 grid gap-2 md:grid-cols-2"
          >
            <input
              required
              name="request_id"
              placeholder="Stable repair identity"
              className="rounded border p-2"
            />
            <select required name="finding_id" className="rounded border p-2">
              {a.findings
                .filter((f) => f.current)
                .map((f) => (
                  <option key={f.id} value={f.id}>
                    {f.kind} · {f.material_kind}
                  </option>
                ))}
            </select>
            <select name="strategy" className="rounded border p-2">
              <option value="replace">Replace</option>
              <option value="reimplement">Reimplement</option>
              <option value="remove">Remove</option>
              <option value="obtain_permission">Obtain permission</option>
              <option value="isolate">Isolate</option>
            </select>
            <input
              required
              name="title"
              placeholder="Repair plan title"
              className="rounded border p-2"
            />
            <input
              required
              name="task"
              placeholder="First owned task"
              className="rounded border p-2"
            />
            <select name="assignee_type" className="rounded border p-2">
              <option value="human">Human-owned</option>
              <option value="agent">Agent-owned</option>
            </select>
            <input
              name="assignee_id"
              placeholder="Assignee ID (blank creates scoped agent)"
              className="rounded border p-2"
            />
            <input
              required
              name="criteria"
              placeholder="Acceptance criteria, comma separated"
              className="rounded border p-2"
            />
            <input
              name="evidence"
              placeholder="Permitted repository evidence IDs, comma separated"
              className="rounded border p-2"
            />
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" name="clean_room" /> Enforce clean-room
              evidence boundary
            </label>
            <Button>Authorize ordinary repair work</Button>
          </form>
          <form
            onSubmit={(e) => event(e, a)}
            className="mt-4 grid gap-2 md:grid-cols-2"
          >
            <input
              required
              name="request_id"
              placeholder="Stable event identity"
              className="rounded border p-2"
            />
            <select name="kind" className="rounded border p-2">
              <option value="challenge">Challenge match</option>
              <option value="origin_evidence">Supply cited origin</option>
              <option value="acknowledgement">Owner acknowledgement</option>
              <option value="exception">Bounded exception</option>
            </select>
            <select required name="finding_id" className="rounded border p-2">
              {a.findings.map((f) => (
                <option key={f.id} value={f.id}>
                  {f.kind} · {f.material_kind}
                </option>
              ))}
            </select>
            <input
              name="citation"
              placeholder="Citation resource ID (origin evidence)"
              className="rounded border p-2"
            />
            <input
              name="expires"
              type="datetime-local"
              className="rounded border p-2"
            />
            <input
              name="follow_up"
              placeholder="Exception follow-up"
              className="rounded border p-2"
            />
            <textarea
              required
              name="body"
              placeholder="Bounded analysis or decision"
              className="rounded border p-2 md:col-span-2"
            />
            <Button>Record revision-bound event</Button>
          </form>
          <p className="mt-3 text-xs text-[var(--muted)]">
            Evidence and readiness only; no Git, review, merge, package,
            release, disclosure, exception, or distribution authority is
            granted.
          </p>
        </Card>
      ))}
    </div>
  );
}
