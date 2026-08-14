"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Unit = {
  id: string;
  message: string;
  context: string;
  screenshot?: string;
  variables: { name: string; example: string }[];
  plural_rule?: string;
  source_locations: { path: string; line: number; component?: string }[];
  change: string;
  locale_status: Record<string, string>;
};
type Extraction = {
  id: string;
  source_revision: string;
  locales: string[];
  units: Unit[];
  removed_units: Unit[];
  map: { name: string; version: number };
  created_by: string;
  created_at: string;
};
type Claim = {
  id: string;
  unit_id: string;
  locale: string;
  assignee_id: string;
  state: string;
  previous_assignee_id?: string;
  created_by: string;
};
type Comment = {
  id: string;
  unit_id: string;
  locale: string;
  body: string;
  actor_type: string;
  actor_id: string;
};
type Suggestion = {
  id: string;
  unit_id: string;
  locale: string;
  text: string;
  rationale: string;
  uncertainty: string;
  evidence: { kind: string; reference: string; excerpt?: string }[];
  agent_id: string;
  status: string;
  locale_plan_id: string;
  locale_plan_version: number;
};
type Candidate = {
  id: string;
  locale: string;
  preview_id: string;
  preview_url: string;
  source_revision: string;
  locale_plan_id: string;
  locale_plan_version: number;
  routes: { journey_id: string; route: string; interface_hash: string }[];
};
type CheckRun = {
  id: string;
  candidate_id: string;
  results: {
    kind: string;
    route: string;
    unit_ids: string[];
    status: string;
    summary: string;
  }[];
};
type Finding = {
  id: string;
  candidate_id: string;
  locale: string;
  route: string;
  unit_ids: string[];
  category: string;
  severity: string;
  body: string;
  author_id: string;
};
type LocaleDecision = {
  id: string;
  candidate_id: string;
  locale: string;
  route: string;
  unit_ids: string[];
  kind: string;
  reason: string;
  actor_id: string;
  actor_role: string;
};
type Review = {
  current_revision: string;
  workspace_version: number;
  extractions: Extraction[];
  counts: Record<string, Record<string, number>>;
  claims: Claim[];
  comments: Comment[];
  suggestions: Suggestion[];
  verification_candidates: Candidate[];
  verification_runs: CheckRun[];
  locale_findings: Finding[];
  locale_review_decisions: LocaleDecision[];
  verification: {
    candidate_id: string;
    current: boolean;
    stale_scopes: string[];
  }[];
};

export function PullLocalizationReview({
  repositoryID,
  pullRequestID,
}: {
  repositoryID: string;
  pullRequestID: string;
}) {
  const { token, user } = useAuth(),
    [review, setReview] = useState<Review>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    try {
      setReview(
        await api<Review>(
          `/repositories/${repositoryID}/pulls/${pullRequestID}/localization`,
          {},
          token,
        ),
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Localization review could not be loaded.",
      );
    }
  }, [repositoryID, pullRequestID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function propose(e: FormEvent<HTMLFormElement>, unit: Unit) {
    e.preventDefault();
    if (!token || !review) return;
    setBusy(true);
    const f = new FormData(e.currentTarget);
    try {
      setReview(
        await api<Review>(
          `/repositories/${repositoryID}/pulls/${pullRequestID}/localization/translations`,
          {
            method: "POST",
            body: JSON.stringify({
              source_revision: review.current_revision,
              unit_id: unit.id,
              locale: f.get("locale"),
              text: f.get("text"),
              note: f.get("note"),
            }),
          },
          token,
        ),
      );
      e.currentTarget.reset();
      setError("");
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Translation could not be proposed.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function mutate(
    mutation: string,
    payload: Record<string, unknown>,
    form?: HTMLFormElement,
  ) {
    if (!token || !review) return;
    setBusy(true);
    try {
      setReview(
        await api<Review>(
          `/repositories/${repositoryID}/pulls/${pullRequestID}/localization/workspace`,
          {
            method: "POST",
            body: JSON.stringify({
              source_revision: review.current_revision,
              expected_version: review.workspace_version,
              mutation,
              payload,
            }),
          },
          token,
        ),
      );
      form?.reset();
      setError("");
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Localization workspace could not be updated.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function verify(
    mutation: string,
    payload: Record<string, unknown>,
    previewID?: string,
    form?: HTMLFormElement,
  ) {
    if (!token || !review) return;
    setBusy(true);
    const path = previewID
      ? `/repositories/${repositoryID}/pulls/${pullRequestID}/localization/previews/${previewID}/review`
      : `/repositories/${repositoryID}/pulls/${pullRequestID}/localization/verification`;
    try {
      setReview(
        await api<Review>(
          path,
          {
            method: "POST",
            body: JSON.stringify({
              source_revision: review.current_revision,
              expected_version: review.workspace_version,
              mutation,
              payload,
            }),
          },
          token,
        ),
      );
      form?.reset();
      setError("");
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Locale verification could not be updated.",
      );
    } finally {
      setBusy(false);
    }
  }
  const latest = review?.extractions.at(-1);
  return (
    <Card id="localization" className="p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="font-semibold">Localization review</h2>
          <p className="mt-1 text-xs leading-5 text-[var(--muted)]">
            Exact-revision messages with enough product context to translate
            without reverse-engineering source. Translation proposals grant no
            repository write access.
          </p>
        </div>
        {latest && (
          <Badge
            tone={
              latest.source_revision === review?.current_revision
                ? "success"
                : "warning"
            }
          >
            {latest.source_revision === review?.current_revision
              ? "current revision"
              : "stale extraction"}
          </Badge>
        )}
      </div>
      {error && (
        <p role="alert" className="mt-3 text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      {!latest ? (
        <p className="mt-4 text-sm text-[var(--muted)]">
          No repository-defined extraction has been published for this pull
          revision.
        </p>
      ) : (
        <>
          <div className="mt-4 flex flex-wrap gap-2">
            {Object.entries(review?.counts ?? {}).map(([locale, counts]) => (
              <Badge
                key={locale}
                tone={(counts.untranslated ?? 0) > 0 ? "warning" : "success"}
              >
                {locale}: {counts.added ?? 0} added · {counts.changed ?? 0}{" "}
                changed · {counts.removed ?? 0} removed · {counts.reused ?? 0}{" "}
                reused · {counts.untranslated ?? 0} untranslated
              </Badge>
            ))}
          </div>
          <p className="mt-3 text-xs text-[var(--muted)]">
            {latest.map.name} v{latest.map.version} · extracted by{" "}
            {latest.created_by.slice(0, 8)} at{" "}
            <code>{latest.source_revision.slice(0, 12)}</code>
          </p>
          <div className="mt-4 space-y-3">
            {latest.units.map((unit) => (
              <article
                key={unit.id}
                className="rounded-lg border border-[var(--line)] p-4"
              >
                <div className="flex flex-wrap gap-2">
                  <Badge
                    tone={
                      unit.change === "changed"
                        ? "warning"
                        : unit.change === "added"
                          ? "info"
                          : "neutral"
                    }
                  >
                    {unit.change}
                  </Badge>
                  {Object.entries(unit.locale_status).map(([l, s]) => (
                    <Badge
                      key={l}
                      tone={s === "proposed" ? "success" : "warning"}
                    >
                      {l} {s}
                    </Badge>
                  ))}
                </div>
                <p className="mt-3 font-semibold">{unit.message}</p>
                <p className="mt-1 text-sm text-[var(--muted)]">
                  {unit.context}
                </p>
                <p className="mt-2 text-xs">
                  Appears at{" "}
                  {unit.source_locations
                    .map(
                      (x) =>
                        `${x.path}:${x.line}${x.component ? ` (${x.component})` : ""}`,
                    )
                    .join(" · ")}
                  {unit.screenshot ? ` · Screenshot ${unit.screenshot}` : ""}
                </p>
                {(unit.variables.length > 0 || unit.plural_rule) && (
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    {unit.variables
                      .map((x) => `{${x.name}} e.g. ${x.example}`)
                      .join(" · ")}
                    {unit.plural_rule ? ` · ${unit.plural_rule} plurals` : ""}
                  </p>
                )}
                {token &&
                  latest.source_revision === review?.current_revision && (
                    <form
                      onSubmit={(e) => void propose(e, unit)}
                      className="mt-3 grid gap-2 sm:grid-cols-[8rem_1fr_auto]"
                    >
                      <select
                        name="locale"
                        aria-label="Translation locale"
                        className="rounded-lg border border-[var(--line-strong)] bg-white px-2 text-sm"
                      >
                        {latest.locales.map((l) => (
                          <option key={l}>{l}</option>
                        ))}
                      </select>
                      <div>
                        <input
                          name="text"
                          required
                          maxLength={10000}
                          aria-label="Translation"
                          placeholder="Proposed translation"
                          className="w-full rounded-lg border border-[var(--line-strong)] px-3 py-2 text-sm"
                        />
                        <input
                          name="note"
                          maxLength={2000}
                          aria-label="Translator note"
                          placeholder="Context note (optional)"
                          className="mt-2 w-full rounded-lg border border-[var(--line)] px-3 py-2 text-xs"
                        />
                      </div>
                      <Button disabled={busy}>Propose</Button>
                    </form>
                  )}
                {token && review && user && (
                  <div className="mt-4 space-y-3 border-t border-[var(--line)] pt-3">
                    {latest.locales.map((locale) => (
                      <LocaleCollaboration
                        key={locale}
                        unit={unit}
                        locale={locale}
                        review={review}
                        userID={user.id}
                        busy={busy}
                        mutate={mutate}
                      />
                    ))}
                  </div>
                )}
              </article>
            ))}
          </div>
          {review && token && (
            <VerificationWorkspace
              repositoryID={repositoryID}
              pullRequestID={pullRequestID}
              review={review}
              units={latest.units}
              locales={latest.locales}
              busy={busy}
              verify={verify}
            />
          )}
        </>
      )}
    </Card>
  );
}

function LocaleCollaboration({
  unit,
  locale,
  review,
  userID,
  busy,
  mutate,
}: {
  unit: Unit;
  locale: string;
  review: Review;
  userID: string;
  busy: boolean;
  mutate: (
    kind: string,
    payload: Record<string, unknown>,
    form?: HTMLFormElement,
  ) => Promise<void>;
}) {
  const latestClaim = [...review.claims]
    .reverse()
    .find((x) => x.unit_id === unit.id && x.locale === locale);
  const claim = latestClaim?.assignee_id ? latestClaim : undefined;
  const comments = review.comments.filter(
    (x) => x.unit_id === unit.id && x.locale === locale,
  );
  const suggestions = review.suggestions.filter(
    (x) => x.unit_id === unit.id && x.locale === locale,
  );
  return (
    <section className="rounded-md bg-[var(--surface-subtle)] p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs font-semibold uppercase tracking-wide">
          {locale} collaboration
        </p>
        {claim ? (
          <>
            <Badge tone="info">
              Claimed by {claim.assignee_id.slice(0, 8)}
            </Badge>
            {claim.assignee_id === userID && (
              <Button
                disabled={busy}
                onClick={() =>
                  void mutate("release", { unit_id: unit.id, locale })
                }
              >
                Release
              </Button>
            )}
          </>
        ) : (
          <Button
            disabled={busy}
            onClick={() =>
              void mutate("claim", {
                unit_id: unit.id,
                locale,
                assignee_id: userID,
              })
            }
          >
            Claim work
          </Button>
        )}
      </div>
      {comments.map((x) => (
        <p key={x.id} className="mt-2 text-xs">
          <strong>
            {x.actor_type} {x.actor_id.slice(0, 8)}
          </strong>
          : {x.body}
        </p>
      ))}
      <form
        className="mt-2 flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          const f = new FormData(e.currentTarget);
          void mutate(
            "comment",
            { unit_id: unit.id, locale, body: f.get("body") },
            e.currentTarget,
          );
        }}
      >
        <input
          name="body"
          required
          maxLength={4000}
          placeholder="Discuss context or terminology"
          className="min-w-0 flex-1 rounded-lg border border-[var(--line)] px-3 py-2 text-xs"
        />
        <Button disabled={busy}>Comment</Button>
      </form>
      <form
        className="mt-2 grid gap-2 sm:grid-cols-4"
        onSubmit={(e) => {
          e.preventDefault();
          const f = new FormData(e.currentTarget);
          void mutate(
            "request_suggestion",
            {
              unit_id: unit.id,
              locale,
              agent_id: f.get("agent"),
              product_context: f.get("context"),
              locale_plan_id: f.get("plan"),
              locale_plan_version: Number(f.get("version")),
              protected: false,
              embargoed: false,
            },
            e.currentTarget,
          );
        }}
      >
        <input
          name="agent"
          required
          placeholder="Approved agent ID"
          className="rounded-lg border border-[var(--line)] px-2 py-2 text-xs"
        />
        <input
          name="plan"
          required
          placeholder="Locale plan ID"
          className="rounded-lg border border-[var(--line)] px-2 py-2 text-xs"
        />
        <input
          name="version"
          type="number"
          min="1"
          required
          placeholder="Plan version"
          className="rounded-lg border border-[var(--line)] px-2 py-2 text-xs"
        />
        <input
          name="context"
          required
          placeholder="Bounded product context"
          className="rounded-lg border border-[var(--line)] px-2 py-2 text-xs"
        />
        <Button disabled={busy}>Request grounded suggestion</Button>
      </form>
      {suggestions.map((x) => (
        <div
          key={x.id}
          className="mt-3 rounded-lg border border-[var(--line)] bg-white p-3 text-xs"
        >
          <div className="flex gap-2">
            <Badge tone="info">agent suggestion</Badge>
            <Badge tone={x.uncertainty === "high" ? "warning" : "neutral"}>
              {x.uncertainty} uncertainty
            </Badge>
            <Badge tone="neutral">{x.status}</Badge>
          </div>
          <p className="mt-2 text-sm font-semibold">{x.text}</p>
          <p className="mt-1">{x.rationale}</p>
          <p className="mt-1 text-[var(--muted)]">
            Evidence:{" "}
            {x.evidence.map((e) => `${e.kind}: ${e.reference}`).join(" · ")} ·
            plan {x.locale_plan_id} v{x.locale_plan_version}
          </p>
          <div className="mt-2 flex gap-2">
            {["approve", "reject", "escalate"].map((kind) => (
              <Button
                key={kind}
                disabled={busy}
                onClick={() =>
                  void mutate("decide", {
                    unit_id: unit.id,
                    locale,
                    suggestion_id: x.id,
                    kind,
                    reason: `Human ${kind} decision`,
                  })
                }
              >
                {kind}
              </Button>
            ))}
          </div>
        </div>
      ))}
    </section>
  );
}

const verificationKinds = [
  "variables",
  "pluralization",
  "formatting",
  "terminology",
  "links",
  "layout_expansion",
  "bidirectional_text",
  "fallback_behavior",
  "localized_journey",
];
function VerificationWorkspace({
  repositoryID,
  pullRequestID,
  review,
  units,
  locales,
  busy,
  verify,
}: {
  repositoryID: string;
  pullRequestID: string;
  review: Review;
  units: Unit[];
  locales: string[];
  busy: boolean;
  verify: (
    kind: string,
    payload: Record<string, unknown>,
    previewID?: string,
    form?: HTMLFormElement,
  ) => Promise<void>;
}) {
  const unitIDs = units.map((x) => x.id);
  return (
    <section className="mt-6 border-t border-[var(--line)] pt-5">
      <h3 className="font-semibold">Localized experience verification</h3>
      <p className="mt-1 text-xs leading-5 text-[var(--muted)]">
        Inspect named-user previews at the exact candidate revision. Evidence is
        grounded to a locale, declared journey, route, interface hash, and the
        translation versions it exercised.
      </p>
      <form
        className="mt-4 grid gap-2 md:grid-cols-4"
        onSubmit={(e) => {
          e.preventDefault();
          const f = new FormData(e.currentTarget);
          void verify(
            "publish_candidate",
            {
              locale: f.get("locale"),
              preview_id: f.get("preview"),
              locale_plan_id: f.get("plan"),
              locale_plan_version: Number(f.get("version")),
              routes: [
                {
                  journey_id: f.get("journey"),
                  route: f.get("route"),
                  interface_hash: f.get("interface"),
                },
              ],
            },
            undefined,
            e.currentTarget,
          );
        }}
      >
        <select
          name="locale"
          aria-label="Candidate locale"
          className="rounded-lg border px-2"
        >
          {locales.map((x) => (
            <option key={x}>{x}</option>
          ))}
        </select>
        <input
          name="preview"
          required
          placeholder="Bounded preview ID"
          className="rounded-lg border px-2 py-2 text-xs"
        />
        <input
          name="plan"
          required
          placeholder="Locale plan ID"
          className="rounded-lg border px-2 py-2 text-xs"
        />
        <input
          name="version"
          required
          min="1"
          type="number"
          placeholder="Plan version"
          className="rounded-lg border px-2 py-2 text-xs"
        />
        <input
          name="journey"
          required
          placeholder="Declared journey ID"
          className="rounded-lg border px-2 py-2 text-xs"
        />
        <input
          name="route"
          required
          placeholder="Locale route, e.g. /ar/cart"
          className="rounded-lg border px-2 py-2 text-xs"
        />
        <input
          name="interface"
          required
          pattern="[a-fA-F0-9]{64}"
          placeholder="Interface SHA-256"
          className="rounded-lg border px-2 py-2 text-xs"
        />
        <Button disabled={busy}>Bind candidate</Button>
      </form>
      <div className="mt-4 space-y-3">
        {(review.verification_candidates ?? []).map((candidate) => {
          const projection = (review.verification ?? []).find(
              (x) => x.candidate_id === candidate.id,
            ),
            runs = (review.verification_runs ?? []).filter(
              (x) => x.candidate_id === candidate.id,
            ),
            findings = (review.locale_findings ?? []).filter(
              (x) => x.candidate_id === candidate.id,
            ),
            decisions = (review.locale_review_decisions ?? []).filter(
              (x) => x.candidate_id === candidate.id,
            );
          return (
            <article key={candidate.id} className="rounded-lg border p-4">
              <div className="flex flex-wrap items-center gap-2">
                <Badge tone={projection?.current ? "success" : "warning"}>
                  {projection?.current ? "evidence current" : "evidence stale"}
                </Badge>
                <Badge tone="info">{candidate.locale}</Badge>
                <a
                  className="text-sm font-semibold text-[var(--brand)] underline"
                    href={`/pulls/${repositoryID}/${pullRequestID}/previews/${candidate.preview_id}`}
                >
                  Open bounded preview
                </a>
              </div>
              <p className="mt-2 text-xs text-[var(--muted)]">
                revision <code>{candidate.source_revision.slice(0, 12)}</code> ·
                plan {candidate.locale_plan_id} v{candidate.locale_plan_version}{" "}
                ·{" "}
                {candidate.routes
                  .map((x) => `${x.journey_id} ${x.route}`)
                  .join(" · ")}
              </p>
              {projection && !projection.current && (
                <p className="mt-2 text-xs text-[var(--danger)]">
                  Invalidated: {projection.stale_scopes.join(" · ")}
                </p>
              )}
              <form
                className="mt-3 flex items-end gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  const f = new FormData(e.currentTarget),
                    status = String(f.get("status")),
                    route = candidate.routes[0].route;
                  void verify(
                    "record_checks",
                    {
                      candidate_id: candidate.id,
                      results: verificationKinds.map((kind) => ({
                        kind,
                        route,
                        unit_ids: unitIDs,
                        status,
                        summary: `Repository ${kind.replaceAll("_", " ")} check ${status}`,
                      })),
                    },
                    undefined,
                    e.currentTarget,
                  );
                }}
              >
                <label className="text-xs font-semibold">
                  Repository-defined suite
                  <select
                    name="status"
                    className="mt-1 block rounded-lg border px-2 py-2"
                  >
                    <option value="passed">All checks passed</option>
                    <option value="failed">Checks failed</option>
                  </select>
                </label>
                <Button disabled={busy}>Record exact-revision checks</Button>
              </form>
              {runs.map((run) => (
                <div key={run.id} className="mt-3 flex flex-wrap gap-1">
                  {run.results.map((result) => (
                    <Badge
                      key={result.kind}
                      tone={result.status === "passed" ? "success" : "danger"}
                    >
                      {result.kind.replaceAll("_", " ")} {result.status}
                    </Badge>
                  ))}
                </div>
              ))}
              <form
                className="mt-3 grid gap-2 md:grid-cols-4"
                onSubmit={(e) => {
                  e.preventDefault();
                  const f = new FormData(e.currentTarget);
                  void verify(
                    "finding",
                    {
                      candidate_id: candidate.id,
                      route: candidate.routes[0].route,
                      unit_ids: [String(f.get("unit"))],
                      category: f.get("category"),
                      severity: f.get("severity"),
                      body: f.get("body"),
                    },
                    candidate.preview_id,
                    e.currentTarget,
                  );
                }}
              >
                <select
                  name="unit"
                  aria-label="Affected translation unit"
                  className="rounded-lg border px-2"
                >
                  {units.map((x) => (
                    <option value={x.id} key={x.id}>
                      {x.message.slice(0, 40)}
                    </option>
                  ))}
                </select>
                <select
                  name="category"
                  aria-label="Finding category"
                  className="rounded-lg border px-2"
                >
                  {verificationKinds.map((x) => (
                    <option key={x}>{x}</option>
                  ))}
                </select>
                <select
                  name="severity"
                  aria-label="Finding severity"
                  className="rounded-lg border px-2"
                >
                  <option>medium</option>
                  <option>high</option>
                  <option>blocking</option>
                  <option>low</option>
                </select>
                <input
                  name="body"
                  required
                  maxLength={4000}
                  placeholder="What reads or behaves incorrectly?"
                  className="rounded-lg border px-2 py-2 text-xs"
                />
                <Button disabled={busy}>Attach route-grounded finding</Button>
              </form>
              {findings.map((x) => (
                <p
                  key={x.id}
                  className="mt-2 rounded bg-[var(--surface-subtle)] p-2 text-xs"
                >
                  <strong>
                    {x.severity} {x.category.replaceAll("_", " ")}
                  </strong>{" "}
                  at {x.route}: {x.body}
                </p>
              ))}
              <div className="mt-3 flex flex-wrap gap-2">
                {["approve", "reject"].map((kind) => (
                  <Button
                    key={kind}
                    disabled={busy}
                    onClick={() =>
                      void verify(
                        "review",
                        {
                          candidate_id: candidate.id,
                          route: candidate.routes[0].route,
                          unit_ids: unitIDs,
                          kind,
                          reason: `${kind} after inspecting ${candidate.routes[0].route} in ${candidate.locale}`,
                        },
                        candidate.preview_id,
                      )
                    }
                  >
                    {kind} affected content
                  </Button>
                ))}
              </div>
              {decisions.map((x) => (
                <p key={x.id} className="mt-2 text-xs">
                  <strong>
                    {x.kind} by {x.actor_role.replaceAll("_", " ")}{" "}
                    {x.actor_id.slice(0, 8)}
                  </strong>{" "}
                  · {x.reason}
                </p>
              ))}
            </article>
          );
        })}
      </div>
    </section>
  );
}
