"use client";

import { useCallback, useEffect, useState } from "react";
import {
  api,
  type CheckEvent,
  type CheckRun,
  type User,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

const terminal = (state: string) =>
  ["succeeded", "failed", "canceled"].includes(state);
const short = (value: string) => value.slice(0, 8);
const tone = (state: string) =>
  state === "succeeded"
    ? "success"
    : state === "failed"
      ? "danger"
      : state === "canceled"
        ? "neutral"
        : "warning";

export function PullRequestChecks({
  repositoryID,
  pullRequestID,
  participant,
}: {
  repositoryID: string;
  pullRequestID: string;
  participant: boolean;
}) {
  const { token } = useAuth();
  const [runs, setRuns] = useState<CheckRun[]>([]);
  const [events, setEvents] = useState<Record<string, CheckEvent[]>>({});
  const [actors, setActors] = useState<Record<string, User>>({});
  const [expanded, setExpanded] = useState<string | null>(null);
  const [pending, setPending] = useState<string | null>(null);
  const [error, setError] = useState("");
  const base = `/repositories/${repositoryID}/pulls/${pullRequestID}/checks`;

  const load = useCallback(async () => {
    try {
      const result = await api<{ check_runs: CheckRun[] }>(base, {}, token);
      setRuns(result.check_runs);
      setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Checks could not be loaded.");
    }
  }, [base, token]);

  useEffect(() => {
    void Promise.resolve().then(load);
    const timer = window.setInterval(() => void load(), 3000);
    return () => window.clearInterval(timer);
  }, [load]);

  useEffect(() => {
    if (!expanded) return;
    let active = true;
    const loadEvidence = async () => {
      try {
        const result = await api<{ events: CheckEvent[] }>(
          `${base}/${expanded}/events`,
          {},
          token,
        );
        if (!active) return;
        setEvents((current) => ({ ...current, [expanded]: result.events }));
        const ids = [...new Set(result.events.flatMap((event) => event.actor_id ? [event.actor_id] : []))];
        const people = await Promise.all(ids.map((id) => api<User>(`/users/${id}`, {}, token).catch(() => null)));
        if (active) setActors((current) => ({ ...current, ...Object.fromEntries(people.filter((person): person is User => Boolean(person)).map((person) => [person.id, person])) }));
      } catch (reason) {
        if (active) setError(reason instanceof Error ? reason.message : "Check evidence could not be loaded.");
      }
    };
    void Promise.resolve().then(loadEvidence);
    const timer = window.setInterval(() => void loadEvidence(), 2000);
    return () => { active = false; window.clearInterval(timer); };
  }, [base, expanded, token]);

  async function control(run: CheckRun, action: "cancel" | "rerun") {
    setPending(run.id);
    setError("");
    try {
      await api(`${base}/${run.id}/${action}`, { method: "POST" }, token);
      setExpanded(run.id);
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : `Check could not be ${action === "rerun" ? "rerun" : "canceled"}.`);
    } finally {
      setPending(null);
    }
  }

  async function download(run: CheckRun, artifactID: string, name: string) {
    try {
      const response = await fetch(`/api${base}/${run.id}/artifacts/${artifactID}`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!response.ok) throw new Error("Artifact could not be downloaded.");
      const url = URL.createObjectURL(await response.blob());
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = name.split("/").pop() ?? "artifact";
      anchor.click();
      URL.revokeObjectURL(url);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Artifact could not be downloaded.");
    }
  }

  return (
    <section id="checks" className="scroll-mt-24 space-y-3">
      <div className="flex items-baseline justify-between gap-3">
        <h2 className="text-lg font-semibold">Verification checks</h2>
        <span className="text-xs text-[var(--muted)]">Live · exact revision evidence</span>
      </div>
      {error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}
      <Card className="overflow-hidden">
        {runs.length === 0 ? <p className="p-6 text-sm text-[var(--muted)]">This revision does not define verification checks.</p> : (
          <div className="divide-y divide-[var(--line)]">
            {runs.map((run) => {
              const open = expanded === run.id;
              const evidence = events[run.id] ?? [];
              return <div key={run.id} className="p-4 sm:px-5">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <button type="button" onClick={() => setExpanded(open ? null : run.id)} className="min-w-0 text-left">
                    <span className="flex items-center gap-2"><Badge tone={tone(run.state)}>{run.state.replace("_", " ")}</Badge><span className="font-semibold">{run.definition.name}</span></span>
                    <span className="mt-1 block text-xs text-[var(--muted)]">Revision <code>{short(run.commit_id)}</code> · {run.attempts.length} {run.attempts.length === 1 ? "attempt" : "attempts"} · {open ? "hide details" : "inspect logs and artifacts"}</span>
                  </button>
                  {participant && <div className="flex gap-2">{terminal(run.state) ? <Button variant="secondary" disabled={pending === run.id} onClick={() => void control(run, "rerun")}>Rerun</Button> : <Button variant="quiet" disabled={pending === run.id} onClick={() => void control(run, "cancel")}>Cancel</Button>}</div>}
                </div>
                {open && <div className="mt-4 space-y-4 border-t border-[var(--line)] pt-4">
                  <ol className="space-y-2">{run.attempts.map((attempt) => <li key={attempt.number} className="text-xs"><span className="font-semibold">Attempt {attempt.number}</span> · {attempt.state}{attempt.actor_id && <> · requested by {actors[attempt.actor_id] ? `@${actors[attempt.actor_id].handle}` : short(attempt.actor_id)}</>}{attempt.failure && <span className="text-[var(--danger)]"> · {attempt.failure}</span>}</li>)}</ol>
                  {evidence.filter((event) => event.kind === "control").map((event) => <p key={event.sequence} className="text-xs text-[var(--muted)]">{event.message === "rerun" ? "Rerun requested" : "Canceled"} by {event.actor_id && actors[event.actor_id] ? `@${actors[event.actor_id].handle}` : event.actor_id ? short(event.actor_id) : "a collaborator"}</p>)}
                  <div><h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">Logs</h3><pre className="mt-2 max-h-80 overflow-auto whitespace-pre-wrap rounded-lg bg-[var(--ink)] p-4 text-xs leading-5 text-white">{evidence.filter((event) => event.kind === "log").map((event) => `[attempt ${event.attempt} · ${event.stream}] ${event.message ?? ""}`).join("") || "No log output recorded."}</pre></div>
                  <div><h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">Artifacts</h3>{run.artifacts.length ? <ul className="mt-2 space-y-2">{run.artifacts.map((artifact) => <li key={artifact.id} className="flex flex-wrap items-center justify-between gap-2 text-xs"><span><span className="font-mono">{artifact.path}</span> · attempt {artifact.attempt} · {artifact.size.toLocaleString()} bytes</span><Button variant="quiet" onClick={() => void download(run, artifact.id, artifact.path)}>Download</Button></li>)}</ul> : <p className="mt-2 text-sm text-[var(--muted)]">No artifacts published.</p>}</div>
                </div>}
              </div>;
            })}
          </div>
        )}
      </Card>
    </section>
  );
}
