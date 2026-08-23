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

type Investigation = {
  id: string;
  version: number;
  title: string;
  expected_behavior: string;
  regressed_behavior: string;
  known_good: { revision: string; label: string };
  known_bad: { revision: string; label: string };
  affected_environments: string[];
  severity: string;
  owner_ids: string[];
  acceptance_criteria: string[];
  comparable: boolean;
  diagnostics: string[];
  status: string;
  evidence: {
    id: string;
    label: string;
    available: boolean;
    stale: boolean;
    diagnostic?: string;
  }[];
  history: {
    id: string;
    kind: string;
    actor_id: string;
    message: string;
    from?: string;
    to?: string;
    created_at: string;
  }[];
};
const field =
  "rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-2 text-sm";
const value = (f: FormData, n: string) => String(f.get(n) || "").trim();
const list = (v: string) =>
  v
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
export function RegressionInvestigationsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token } = useAuth();
  const [items, setItems] = useState<Investigation[]>([]);
  const [selected, setSelected] = useState<Investigation>();
  const [error, setError] = useState("");
  const createRequestID = useRef(crypto.randomUUID());
  const load = useCallback(async () => {
    try {
      const x = await api<{ regression_investigations: Investigation[] }>(
        `/repositories/${repositoryID}/regression-investigations`,
        {},
        token,
      );
      setItems(x.regression_investigations);
      setSelected(
        (s) =>
          x.regression_investigations.find((v) => v.id === s?.id) ??
          x.regression_investigations[0],
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Investigations could not be loaded",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void load();
  }, [load]);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    const f = new FormData(e.currentTarget),
      good = value(f, "good"),
      bad = value(f, "bad"),
      sourceKind = value(f, "source_kind"),
      evidenceID = value(f, "evidence_id");
    try {
      const out = await api<Investigation>(
        `/repositories/${repositoryID}/regression-investigations`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: createRequestID.current,
            title: value(f, "title"),
            source: {
              kind: sourceKind,
              resource_id: value(f, "source_id"),
              revision: value(f, "source_revision"),
              label: value(f, "source_label"),
            },
            expected_behavior: value(f, "expected"),
            regressed_behavior: value(f, "regressed"),
            known_good: {
              kind: "commit",
              revision: good,
              label: value(f, "good_label") || good.slice(0, 12),
            },
            known_bad: {
              kind: "commit",
              revision: bad,
              label: value(f, "bad_label") || bad.slice(0, 12),
            },
            affected_environments: list(value(f, "environments")),
            severity: value(f, "severity"),
            owner_ids: list(value(f, "owners")),
            acceptance_criteria: list(value(f, "criteria")),
            evidence: evidenceID
              ? [
                  {
                    kind: value(f, "evidence_kind"),
                    resource_id: evidenceID,
                    revision: value(f, "evidence_revision"),
                    label: value(f, "evidence_label"),
                    visibility: "repository",
                  },
                ]
              : [],
          }),
        },
        token,
      );
      setItems((v) => (v.some((x) => x.id === out.id) ? v : [out, ...v]));
      setSelected(out);
      e.currentTarget.reset();
      createRequestID.current = crypto.randomUUID();
      setError("");
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Investigation could not be opened",
      );
    }
  }
  async function append(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      const out = await api<Investigation>(
        `/repositories/${repositoryID}/regression-investigations/${selected.id}/events`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected.version,
            kind: value(f, "kind"),
            message: value(f, "message"),
            value: value(f, "event_value"),
          }),
        },
        token,
      );
      setSelected(out);
      setItems((v) => v.map((x) => (x.id === out.id ? out : x)));
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Update failed");
    }
  }
  return (
    <main className="mx-auto max-w-7xl space-y-6 p-6">
      <div>
        <p className="text-xs font-semibold uppercase tracking-widest text-[var(--brand)]">
          Shared search boundary
        </p>
        <h1 className="text-2xl font-semibold">Regression investigations</h1>
        <p className="mt-1 text-sm text-[var(--muted)]">
          Agree on what changed and which history is comparable before testing
          commits.
        </p>
      </div>
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      <div className="grid gap-6 lg:grid-cols-[.8fr_1.2fr]">
        <div className="space-y-4">
          <Card className="p-5">
            <h2 className="font-semibold">Open an investigation</h2>
            <form onSubmit={create} className="mt-4 grid gap-3">
              <input
                className={field}
                name="title"
                required
                placeholder="Checkout fails after rollout"
              />
              <div className="grid grid-cols-2 gap-2">
                <select
                  className={field}
                  name="source_kind"
                  defaultValue="issue"
                >
                  <option value="issue">Issue</option>
                  <option value="support_thread">Support thread</option>
                  <option value="failed_check">Failed check</option>
                  <option value="release">Release</option>
                  <option value="deployment">Deployment</option>
                  <option value="reproduction">Reproduction</option>
                </select>
                <input
                  className={field}
                  name="source_id"
                  required
                  placeholder="Source ID"
                />
              </div>
              <input
                className={field}
                name="source_revision"
                placeholder="Source revision (when applicable)"
              />
              <input
                className={field}
                name="source_label"
                required
                placeholder="Source label"
              />
              <textarea
                className={field}
                name="expected"
                required
                placeholder="Expected behavior"
              />
              <textarea
                className={field}
                name="regressed"
                required
                placeholder="Regressed behavior"
              />
              <input
                className={field}
                name="good"
                required
                pattern="[0-9a-f]{40}"
                placeholder="Known-good commit"
              />
              <input
                className={field}
                name="bad"
                required
                pattern="[0-9a-f]{40}"
                placeholder="Known-bad commit"
              />
              <input
                className={field}
                name="environments"
                required
                placeholder="Affected environments, comma separated"
              />
              <select className={field} name="severity" defaultValue="high">
                <option>low</option>
                <option>medium</option>
                <option>high</option>
                <option>critical</option>
              </select>
              <input
                className={field}
                name="owners"
                required
                placeholder="Owner IDs, comma separated"
              />
              <input
                className={field}
                name="criteria"
                required
                placeholder="Acceptance criteria, comma separated"
              />
              <fieldset className="grid gap-2 rounded-lg border border-[var(--border)] p-3">
                <legend className="px-1 text-xs font-semibold">
                  Permitted evidence (optional)
                </legend>
                <select className={field} name="evidence_kind" defaultValue="issue">
                  <option value="issue">Issue</option>
                  <option value="support_thread">Support thread</option>
                  <option value="failed_check">Failed check</option>
                  <option value="release">Release</option>
                  <option value="deployment">Deployment</option>
                  <option value="reproduction">Reproduction</option>
                  <option value="commit">Commit</option>
                </select>
                <input className={field} name="evidence_id" placeholder="Evidence resource ID" />
                <input className={field} name="evidence_revision" placeholder="Evidence revision" />
                <input className={field} name="evidence_label" placeholder="Evidence label" />
              </fieldset>
              <Button type="submit">Open durable boundary</Button>
            </form>
          </Card>
          <Card className="divide-y divide-[var(--border)]">
            {items.map((x) => (
              <button
                key={x.id}
                onClick={() => setSelected(x)}
                className="block w-full p-4 text-left"
              >
                <span className="font-medium">{x.title}</span>
                <span className="mt-1 flex gap-2">
                  <Badge>{x.severity}</Badge>
                  <Badge tone={x.comparable ? "success" : "danger"}>
                    {x.comparable ? "comparable" : "blocked"}
                  </Badge>
                </span>
              </button>
            ))}
          </Card>
        </div>
        {selected ? (
          <div className="space-y-4">
            <Card className="p-5">
              <div className="flex justify-between">
                <h2 className="text-lg font-semibold">{selected.title}</h2>
                <Badge>{selected.status}</Badge>
              </div>
              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                <div>
                  <p className="text-xs text-[var(--muted)]">Known good</p>
                  <code>{selected.known_good.revision.slice(0, 12)}</code>
                </div>
                <div>
                  <p className="text-xs text-[var(--muted)]">Known bad</p>
                  <code>{selected.known_bad.revision.slice(0, 12)}</code>
                </div>
              </div>
              <h3 className="mt-5 font-medium">Expected</h3>
              <p className="text-sm">{selected.expected_behavior}</p>
              <h3 className="mt-4 font-medium">Regressed</h3>
              <p className="text-sm">{selected.regressed_behavior}</p>
              <p className="mt-4 text-sm">
                <b>Environments:</b> {selected.affected_environments.join(", ")}
              </p>
              <h3 className="mt-4 font-medium">Acceptance criteria</h3>
              <ul className="list-disc pl-5 text-sm">
                {selected.acceptance_criteria.map((x) => (
                  <li key={x}>{x}</li>
                ))}
              </ul>
              <h3 className="mt-4 font-medium">Evidence</h3>
              {selected.evidence.length === 0 ? (
                <p className="text-sm text-[var(--muted)]">No evidence attached.</p>
              ) : (
                <ul className="mt-2 space-y-2">
                  {selected.evidence.map((evidence) => (
                    <li key={evidence.id} className="rounded-lg border border-[var(--border)] p-3 text-sm">
                      <div className="flex items-center justify-between gap-2">
                        <span className="font-medium">{evidence.label}</span>
                        <Badge tone={evidence.stale ? "warning" : evidence.available ? "success" : "danger"}>
                          {evidence.stale ? "stale" : evidence.available ? "available" : "unavailable"}
                        </Badge>
                      </div>
                      {evidence.diagnostic && <p className="mt-1 text-[var(--muted)]">{evidence.diagnostic}</p>}
                    </li>
                  ))}
                </ul>
              )}
              {selected.diagnostics.map((x) => (
                <p key={x} className="mt-2 text-sm text-[var(--warning)]">
                  {x}
                </p>
              ))}
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Discussion and boundary history</h2>
              <form onSubmit={append} className="mt-3 grid gap-2">
                <select name="kind" className={field}>
                  <option value="discussion">Discussion</option>
                  <option value="hypothesis">Hypothesis</option>
                  <option value="scope_change">Scope change</option>
                  <option value="status_change">Status change</option>
                </select>
                <textarea
                  name="message"
                  className={field}
                  required
                  placeholder="Attributable context"
                />
                <input
                  name="event_value"
                  className={field}
                  placeholder="New environments or status when changing scope/status"
                />
                <Button type="submit">Append</Button>
              </form>
              <ol className="mt-4 space-y-3">
                {[...selected.history].reverse().map((x) => (
                  <li
                    key={x.id}
                    className="border-l-2 border-[var(--border)] pl-3 text-sm"
                  >
                    <b>{x.kind.replaceAll("_", " ")}</b> · {x.actor_id}
                    <p>{x.message}</p>
                  </li>
                ))}
              </ol>
            </Card>
          </div>
        ) : (
          <Card className="p-8 text-sm text-[var(--muted)]">
            Open or select an investigation.
          </Card>
        )}
      </div>
    </main>
  );
}
