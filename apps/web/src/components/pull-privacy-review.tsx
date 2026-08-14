"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Flow = {
  id: string;
  current_version: number;
  revisions: { version: number; code_revision: string; title: string }[];
};
type Evidence = {
  path: string;
  start_line: number;
  end_line: number;
  claim: string;
};
type Review = {
  source_revision: string;
  target_revision: string;
  changes: { kind: string; summary: string; data_categories?: string[] }[];
  requirements: {
    kind: string;
    reason: string;
    status: string;
    owner_ids?: string[];
  }[];
  comments: {
    id: string;
    kind: string;
    body: string;
    actor_type: string;
    actor_id: string;
    evidence?: Evidence[];
  }[];
  residual_risk?: string;
  accepted_by?: string;
  history?: {
    id: string;
    source_revision: string;
    changes: { kind: string; summary: string }[];
    comments: { id: string }[];
    accepted_by?: string;
  }[];
};
const short = (v: string) => v.slice(0, 12);

export function PullPrivacyReview({
  repositoryID,
  pullRequestID,
  sourceRevision,
  targetRevision,
  participant,
}: {
  repositoryID: string;
  pullRequestID: string;
  sourceRevision: string;
  targetRevision: string;
  participant: boolean;
}) {
  const { token } = useAuth();
  const [flows, setFlows] = useState<Flow[]>([]);
  const [review, setReview] = useState<Review>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const base = `/repositories/${repositoryID}/pulls/${pullRequestID}/privacy-review`;
  const load = useCallback(async () => {
    if (!token) return;
    const [f, r] = await Promise.all([
      api<{ data_flows: Flow[] }>(
        `/repositories/${repositoryID}/data-flows`,
        {},
        token,
      ),
      api<Review>(base, {}, token).catch(() => undefined),
    ]);
    setFlows(f.data_flows);
    setReview(r);
  }, [base, repositoryID, token]);
  useEffect(() => {
    void Promise.resolve()
      .then(load)
      .catch((e) =>
        setError(
          e instanceof Error
            ? e.message
            : "Privacy evidence could not be loaded.",
        ),
      );
  }, [load]);
  async function compare(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    setError("");
    const d = new FormData(e.currentTarget),
      [source_flow_id, source_flow_version] = String(
        d.get("source_choice"),
      ).split(":"),
      [target_flow_id, target_flow_version] = String(
        d.get("target_choice"),
      ).split(":");
    try {
      setReview(
        await api<Review>(
          base,
          {
            method: "POST",
            body: JSON.stringify({
              source_flow_id,
              source_flow_version: Number(source_flow_version),
              target_flow_id,
              target_flow_version: Number(target_flow_version),
            }),
          },
          token,
        ),
      );
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Privacy comparison could not be created.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function comment(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    const form = e.currentTarget,
      d = new FormData(form),
      raw = String(d.get("evidence")),
      parts = raw.split("|");
    try {
      setReview(
        await api<Review>(
          `${base}/comments`,
          {
            method: "POST",
            body: JSON.stringify({
              kind: d.get("kind"),
              body: d.get("body"),
              finding_kinds: String(d.get("finding_kinds"))
                .split(",")
                .map((x) => x.trim())
                .filter(Boolean),
              evidence: raw
                ? [
                    {
                      path: parts[0],
                      start_line: Number(parts[1]),
                      end_line: Number(parts[2]),
                      claim: parts[3],
                    },
                  ]
                : [],
            }),
          },
          token,
        ),
      );
      form.reset();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Review note could not be retained.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function accept(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !review) return;
    setBusy(true);
    const d = new FormData(e.currentTarget);
    try {
      setReview(
        await api<Review>(
          `${base}/acceptance`,
          {
            method: "POST",
            body: JSON.stringify({
              source_revision: review.source_revision,
              residual_risk: d.get("residual_risk"),
              requirement_kinds: review.requirements.map((x) => x.kind),
            }),
          },
          token,
        ),
      );
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Privacy acceptance could not be recorded.",
      );
    } finally {
      setBusy(false);
    }
  }
  const stale =
    review?.source_revision !== sourceRevision ||
    review?.target_revision !== targetRevision;
  return (
    <section id="privacy" className="scroll-mt-24">
      <Card className="p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="font-semibold">Privacy consequences</h2>
            <p className="mt-1 max-w-3xl text-xs leading-5 text-[var(--muted)]">
              Compare the exact candidate with its target before approval.
              Classifications derive from retained data flows and commitments;
              this record grants no data or repository authority.
            </p>
          </div>
          {review && (
            <Badge
              tone={stale ? "warning" : review.accepted_by ? "success" : "info"}
            >
              {stale
                ? "stale revision"
                : review.accepted_by
                  ? "acknowledged"
                  : "review open"}
            </Badge>
          )}
        </div>
      {(!review || stale) && participant && (
          <form onSubmit={compare} className="mt-4 grid gap-3 md:grid-cols-2">
            <label className="text-xs font-semibold">
              Candidate data flow
              <select
                name="source_choice"
                required
                className="mt-1 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
              >
                {flows.flatMap((f) =>
                  f.revisions
                    .filter((r) => r.code_revision === sourceRevision)
                    .map((r) => (
                      <option
                        key={f.id + r.version}
                        value={`${f.id}:${r.version}`}
                      >
                        {r.title} · v{r.version}
                      </option>
                    )),
                )}
              </select>
            </label>
            <label className="text-xs font-semibold">
              Target data flow
              <select
                name="target_choice"
                required
                className="mt-1 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
              >
                {flows.flatMap((f) =>
                  f.revisions
                    .filter((r) => r.code_revision === targetRevision)
                    .map((r) => (
                      <option
                        key={f.id + r.version}
                        value={`${f.id}:${r.version}`}
                      >
                        {r.title} · v{r.version}
                      </option>
                    )),
                )}
              </select>
            </label>
            <div>
              <Button disabled={busy}>Compare privacy impact</Button>
            </div>
          </form>
        )}
        {review && (
          <>
            <p className="mt-4 text-xs text-[var(--muted)]">
              Candidate <code>{short(review.source_revision)}</code> compared
              with target <code>{short(review.target_revision)}</code>.
            </p>
            <div className="mt-4 grid gap-3 md:grid-cols-2">
              {review.changes.length ? (
                review.changes.map((c, i) => (
                  <div
                    key={c.kind + i}
                    className="rounded-lg border p-3 text-sm"
                  >
                    <Badge tone="warning">{c.kind.replace("_", " ")}</Badge>
                    <p className="mt-2">{c.summary}</p>
                    {c.data_categories?.length ? (
                      <p className="mt-1 text-xs text-[var(--muted)]">
                        {c.data_categories.join(", ")}
                      </p>
                    ) : null}
                  </div>
                ))
              ) : (
                <p className="text-sm text-[var(--muted)]">
                  No changed handling was derived from these declarations.
                </p>
              )}
            </div>
            <div className="mt-4 space-y-2">
              {review.requirements.map((r) => (
                <div
                  key={r.kind}
                  className="flex items-start gap-2 rounded-lg bg-[var(--surface-soft)] p-3 text-sm"
                >
                  <Badge
                    tone={r.status === "acknowledged" ? "success" : "warning"}
                  >
                    {r.status}
                  </Badge>
                  <span>
                    <b>{r.kind.replaceAll("_", " ")}</b> — {r.reason}
                    {r.owner_ids?.length
                      ? ` Owners: ${r.owner_ids.join(", ")}.`
                      : ""}
                  </span>
                </div>
              ))}
            </div>
            {review.comments.map((c) => (
              <div
                key={c.id}
                className="mt-3 border-l-2 border-[var(--brand)] pl-3 text-sm"
              >
                <b>{c.kind.replace("_", " ")}</b> by {c.actor_type}{" "}
                <code>{short(c.actor_id)}</code>
                <p className="mt-1 whitespace-pre-wrap">{c.body}</p>
                {c.evidence?.map((x, i) => (
                  <p key={i} className="mt-1 text-xs text-[var(--muted)]">
                    <code>
                      {x.path}:{x.start_line}-{x.end_line}
                    </code>{" "}
                    — {x.claim}
                  </p>
                ))}
              </div>
            ))}
            {!stale && participant && (
              <form
                onSubmit={comment}
                className="mt-4 grid gap-3 rounded-lg border p-3 md:grid-cols-2"
              >
                <label className="text-xs font-semibold">
                  Review record
                  <select
                    name="kind"
                    className="mt-1 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
                  >
                    <option value="challenge">Challenge</option>
                    <option value="mitigation">Mitigation</option>
                    <option value="residual_risk">Residual risk</option>
                  </select>
                </label>
                <label className="text-xs font-semibold">
                  Affected classifications
                  <input
                    name="finding_kinds"
                    placeholder="collection, consent"
                    className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal"
                  />
                </label>
                <label className="text-xs font-semibold md:col-span-2">
                  Reasoning
                  <textarea
                    name="body"
                    required
                    rows={3}
                    className="mt-1 w-full rounded-lg border p-3 font-normal"
                  />
                </label>
                <label className="text-xs font-semibold md:col-span-2">
                  Revision evidence (optional)
                  <input
                    name="evidence"
                    placeholder="path|start line|end line|claim"
                    className="mt-1 min-h-10 w-full rounded-lg border px-3 font-mono font-normal"
                  />
                </label>
                <div>
                  <Button disabled={busy}>Record review note</Button>
                </div>
              </form>
            )}
            {!stale &&
              participant &&
              !review.accepted_by &&
              review.requirements.length > 0 && (
                <form
                  onSubmit={accept}
                  className="mt-4 flex flex-col gap-3 rounded-lg bg-[var(--surface-soft)] p-3"
                >
                  <label className="text-xs font-semibold">
                    Mitigations and residual risk
                    <textarea
                      name="residual_risk"
                      required
                      rows={3}
                      className="mt-1 w-full rounded-lg border bg-white p-3 font-normal"
                    />
                  </label>
                  <div>
                    <Button disabled={busy}>
                      Acknowledge all required actions
                    </Button>
                  </div>
                  <p className="text-xs text-[var(--muted)]">
                    Acceptance is human-only and binds this exact source
                    revision. Any synchronization makes it stale.
                  </p>
                </form>
              )}
          </>
        )}
      {stale && (
          <p
            role="status"
            className="mt-4 rounded-lg bg-[var(--warning-soft)] p-3 text-sm"
          >
            The source or target revision moved. Prior discussion remains
            evidence, but acceptance no longer applies; publish current
            data-flow evidence and compare again.
          </p>
      )}
      {review?.history?.length ? (
        <details className="mt-4 text-sm">
          <summary className="cursor-pointer font-semibold">
            Earlier revision reviews ({review.history.length})
          </summary>
          {review.history.map((item) => (
            <p key={item.id} className="mt-2 text-xs text-[var(--muted)]">
              <code>{short(item.source_revision)}</code> · {item.changes.length} classified
              change(s) · {item.comments.length} review note(s) · {item.accepted_by ? "acknowledged" : "not acknowledged"}
            </p>
          ))}
        </details>
      ) : null}
        {error && (
          <p role="alert" className="mt-3 text-sm text-[var(--danger)]">
            {error}
          </p>
        )}
      </Card>
    </section>
  );
}
