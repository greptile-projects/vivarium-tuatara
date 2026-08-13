"use client";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Feedback = {
  id: string;
  target: { kind: string; resource_id?: string; label: string };
  need: string;
  desired_outcome: string;
  frequency: string;
  impact: string;
  audience: string;
  identity_visibility: string;
  contact_preference: string;
  reporter_id?: string;
  evidence: {
    id: string;
    name: string;
    kind: string;
    summary: string;
    visibility: string;
    redacted: boolean;
  }[];
  links: { kind: string; resource_id: string }[];
  comments: {
    id: string;
    body: string;
    author_id?: string;
    author_role: string;
    created_at: string;
  }[];
  history: { id: string; kind: string; detail: string; created_at: string }[];
};
const value = (f: FormData, n: string) => String(f.get(n) ?? "").trim();
export function FeedbackWorkspace({ repositoryID }: { repositoryID: string }) {
  const { token } = useAuth();
  const [items, setItems] = useState<Feedback[]>([]),
    [selected, setSelected] = useState<Feedback>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const r = await api<{ feedback: Feedback[] }>(
        `/repositories/${repositoryID}/feedback`,
        {},
        token,
      );
      setItems(r.feedback);
      setSelected(
        (x) => r.feedback.find((y) => y.id === x?.id) ?? r.feedback[0],
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Feedback could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    setError("");
    const f = new FormData(e.currentTarget),
      evidenceSummary = value(f, "evidence_summary"),
      linkID = value(f, "link_id");
    try {
      await api(
        `/repositories/${repositoryID}/feedback`,
        {
          method: "POST",
          body: JSON.stringify({
            target: {
              kind: value(f, "target_kind"),
              resource_id: value(f, "target_id"),
              label: value(f, "target_label"),
            },
            need: value(f, "need"),
            desired_outcome: value(f, "outcome"),
            frequency: value(f, "frequency"),
            impact: value(f, "impact"),
            audience: value(f, "audience"),
            identity_visibility: value(f, "identity_visibility"),
            contact_preference: value(f, "contact_preference"),
            contact: value(f, "contact"),
            evidence: evidenceSummary
              ? [
                  {
                    name: value(f, "evidence_name"),
                    kind: "text",
                    summary: evidenceSummary,
                    visibility: value(f, "evidence_visibility"),
                    redacted: true,
                  },
                ]
              : [],
            links: linkID
              ? [{ kind: value(f, "link_kind"), resource_id: linkID }]
              : [],
          }),
        },
        token,
      );
      e.currentTarget.reset();
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Feedback could not be submitted.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function comment(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !selected) return;
    const f = new FormData(e.currentTarget);
    try {
      await api(
        `/repositories/${repositoryID}/feedback/${selected.id}/comments`,
        { method: "POST", body: JSON.stringify({ body: value(f, "body") }) },
        token,
      );
      e.currentTarget.reset();
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Comment could not be added.");
    }
  }
  return (
    <main
      id="main-content"
      className="mx-auto grid w-full max-w-6xl gap-6 px-6 py-8 lg:grid-cols-[1fr_1.2fr]"
    >
      <section>
        <p className="text-sm font-semibold text-[var(--accent)]">
          Product feedback
        </p>
        <h1 className="mt-2 text-3xl font-semibold">
          Explain the need, safely
        </h1>
        <p className="mt-2 text-sm text-[var(--muted)]">
          Share needs broader than one reproducible defect. You control the
          audience, identity, evidence, and follow-up.
        </p>
        <Card className="mt-6">
          <form className="grid gap-4" onSubmit={submit}>
            <label>
              What is this about?
              <select name="target_kind" className="input" required>
                <option value="project">Project</option>
                <option value="release">Release</option>
                <option value="journey">Documented journey</option>
                <option value="preview">Preview</option>
              </select>
            </label>
            <label>
              Label
              <input name="target_label" className="input" required />
            </label>
            <label>
              Resource ID (for release, journey, or preview)
              <input name="target_id" className="input" />
            </label>
            <label>
              What do you need?
              <textarea name="need" className="input min-h-24" required />
            </label>
            <label>
              Desired outcome
              <textarea name="outcome" className="input" required />
            </label>
            <label>
              How often does this happen?
              <input name="frequency" className="input" required />
            </label>
            <label>
              What is the impact?
              <textarea name="impact" className="input" required />
            </label>
            <div className="grid gap-3 sm:grid-cols-2">
              <label>
                Audience
                <select name="audience" className="input">
                  <option value="project">Project audience</option>
                  <option value="organization_private">
                    Organization private
                  </option>
                </select>
              </label>
              <label>
                Show my identity to
                <select name="identity_visibility" className="input">
                  <option value="maintainers">Maintainers</option>
                  <option value="reporter_only">Only me</option>
                  <option value="audience">Everyone in audience</option>
                </select>
              </label>
              <label>
                Follow-up
                <select name="contact_preference" className="input">
                  <option value="discussion">In this discussion</option>
                  <option value="none">No follow-up</option>
                  <option value="direct">Direct contact</option>
                </select>
              </label>
              <label>
                Direct contact (optional)
                <input name="contact" className="input" />
              </label>
            </div>
            <fieldset className="grid gap-3 rounded-xl border border-[var(--line)] p-4">
              <legend>Redacted evidence (optional)</legend>
              <input
                name="evidence_name"
                className="input"
                placeholder="Evidence name"
              />
              <textarea
                name="evidence_summary"
                className="input"
                placeholder="Paste only a redacted summary"
              />
              <select name="evidence_visibility" className="input">
                <option value="maintainers">Maintainers</option>
                <option value="reporter_only">Only me</option>
                <option value="audience">Everyone in audience</option>
              </select>
            </fieldset>
            <div className="grid gap-3 sm:grid-cols-2">
              <select name="link_kind" className="input">
                <option value="issue">Related issue</option>
                <option value="experiment">Related experiment</option>
              </select>
              <input
                name="link_id"
                className="input"
                placeholder="Optional resource ID"
              />
            </div>
            <Button disabled={busy}>
              {busy ? "Submitting…" : "Submit feedback"}
            </Button>
          </form>
        </Card>
      </section>
      <section>
        <h2 className="text-xl font-semibold">Feedback channel</h2>
        {error && (
          <p role="alert" className="mt-3 text-sm text-red-600">
            {error}
          </p>
        )}
        <div className="mt-4 grid gap-3">
          {items.map((x) => (
            <button
              key={x.id}
              onClick={() => setSelected(x)}
              className="text-left"
            >
              <Card>
                <div className="flex gap-2">
                  <Badge>{x.target.kind}</Badge>
                  <Badge>{x.audience.replace("_", " ")}</Badge>
                </div>
                <h3 className="mt-3 font-semibold">{x.target.label}</h3>
                <p className="mt-1 text-sm text-[var(--muted)]">{x.need}</p>
              </Card>
            </button>
          ))}
          {!items.length && (
            <Card>
              <p className="text-sm text-[var(--muted)]">
                No feedback is visible to you yet.
              </p>
            </Card>
          )}
        </div>
        {selected && (
          <Card className="mt-4">
            <h2 className="text-xl font-semibold">{selected.target.label}</h2>
            <dl className="mt-4 grid gap-3 text-sm">
              <div>
                <dt className="font-semibold">Desired outcome</dt>
                <dd>{selected.desired_outcome}</dd>
              </div>
              <div>
                <dt className="font-semibold">Frequency and impact</dt>
                <dd>
                  {selected.frequency} · {selected.impact}
                </dd>
              </div>
              <div>
                <dt className="font-semibold">Reporter</dt>
                <dd>
                  {selected.reporter_id ?? "Identity withheld"} ·{" "}
                  {selected.contact_preference}
                </dd>
              </div>
            </dl>
            {selected.evidence.map((x) => (
              <div
                className="mt-4 rounded-lg border border-[var(--line)] p-3"
                key={x.id}
              >
                <b>{x.name}</b>
                <p>{x.summary}</p>
                <small>Redacted · {x.visibility}</small>
              </div>
            ))}
            <h3 className="mt-5 font-semibold">Discussion</h3>
            {selected.comments.map((x) => (
              <p className="mt-2 text-sm" key={x.id}>
                <b>{x.author_id ?? "Reporter"}:</b> {x.body}
              </p>
            ))}
            <form className="mt-3 flex gap-2" onSubmit={comment}>
              <input
                className="input flex-1"
                name="body"
                aria-label="Discussion reply"
                required
              />
              <Button>Reply</Button>
            </form>
            <details className="mt-4">
              <summary>History</summary>
              {selected.history.map((x) => (
                <p className="mt-2 text-xs" key={x.id}>
                  {x.kind}: {x.detail}
                </p>
              ))}
            </details>
          </Card>
        )}
      </section>
    </main>
  );
}
