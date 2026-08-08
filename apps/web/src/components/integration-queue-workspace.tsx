"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { AccessGate, useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
import { api, type IntegrationQueueEntry, type Repository } from "@/lib/api";

export function IntegrationQueueWorkspace({
  repositoryID,
  branch,
}: {
  repositoryID: string;
  branch: string;
}) {
  const { token, user } = useAuth();
  const [repository, setRepository] = useState<Repository | null>(null);
  const [entries, setEntries] = useState<IntegrationQueueEntry[]>([]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState("");
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const [repo, queue] = await Promise.all([
        api<Repository>(`/repositories/${repositoryID}`, {}, token),
        api<{ entries: IntegrationQueueEntry[] }>(
          `/repositories/${repositoryID}/branches/${encodeURIComponent(branch)}/queue`,
          {},
          token,
        ),
      ]);
      setRepository(repo);
      setEntries(queue.entries);
      setError("");
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "The integration queue could not be loaded.",
      );
    }
  }, [branch, repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
    const timer = window.setInterval(load, 5000);
    return () => window.clearInterval(timer);
  }, [load]);
  async function operate(
    entry: IntegrationQueueEntry,
    action: string,
    position = 0,
  ) {
    if (!token) return;
    setPending(entry.pull_request.id);
    setError("");
    try {
      await api(
        `/repositories/${repositoryID}/pulls/${entry.pull_request.id}/queue`,
        { method: "PATCH", body: JSON.stringify({ action, position }) },
        token,
      );
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "The queue intervention failed.",
      );
    } finally {
      setPending("");
    }
  }
  const owner = repository?.owner_id === user?.id;
  return (
    <AccessGate>
      <div className="space-y-6">
        <header>
          <Link
            href={`/repositories/${repositoryID}?ref=${encodeURIComponent(branch)}`}
            className="text-sm text-[var(--muted)] hover:text-[var(--brand)]"
          >
            {repository?.name ?? "Repository"}
          </Link>
          <p className="mt-3 text-xs font-bold uppercase tracking-[.16em] text-[var(--brand)]">
            Branch integration
          </p>
          <h1 className="mt-1 text-3xl font-semibold">{branch} queue</h1>
          <p className="mt-2 text-sm text-[var(--muted)]">
            Shared order, prospective results, blockers, and the next available
            intervention.
          </p>
        </header>
        {error && (
          <p
            role="alert"
            className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
          >
            {error}
          </p>
        )}
        {!entries.length ? (
          <Card className="p-8 text-center">
            <h2 className="font-semibold">No changes are waiting</h2>
            <p className="mt-2 text-sm text-[var(--muted)]">
              Ready pull requests can be admitted from their review page.
            </p>
          </Card>
        ) : (
          <ol className="space-y-4">
            {entries.map((entry) => (
              <li key={entry.pull_request.id}>
                <Card className="p-5">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <p className="text-xs font-semibold text-[var(--muted)]">
                        Position {entry.position}
                      </p>
                      <Link
                        href={`/pulls/${repositoryID}/${entry.pull_request.id}`}
                        className="mt-1 block text-lg font-semibold hover:text-[var(--brand)]"
                      >
                        {entry.pull_request.title}
                      </Link>
                      <p className="mt-1 text-xs text-[var(--muted)]">
                        <code>{entry.pull_request.source_branch}</code> →{" "}
                        <code>{branch}</code>
                      </p>
                    </div>
                    <Badge
                      tone={
                        entry.state === "passed"
                          ? "success"
                          : entry.state === "failed"
                            ? "danger"
                            : entry.state === "paused"
                              ? "neutral"
                              : "warning"
                      }
                    >
                      {entry.state}
                    </Badge>
                  </div>
                  {entry.candidate && (
                    <div className="mt-4 rounded-lg bg-[var(--canvas)] p-3 text-xs">
                      <p>
                        Candidate{" "}
                        <code>{entry.candidate.commit_id.slice(0, 10)}</code>{" "}
                        against{" "}
                        <code>
                          {entry.candidate.base_commit_id.slice(0, 10)}
                        </code>
                      </p>
                      <p className="mt-1 text-[var(--muted)]">
                        Attempt{" "}
                        {(entry.pull_request.queue_actions?.filter(
                          (item) => item.action === "retry",
                        ).length ?? 0) + 1}{" "}
                        · {entry.candidate.checks.length} retained check run(s)
                      </p>
                    </div>
                  )}
                  {entry.blockers.map((blocker) => (
                    <p
                      key={blocker.code}
                      className="mt-3 text-sm text-[var(--danger)]"
                    >
                      {blocker.message}
                    </p>
                  ))}
                  <p className="mt-3 text-sm">
                    <span className="font-semibold">Next:</span>{" "}
                    {entry.next_action}
                  </p>
                  {owner && (
                    <div className="mt-4 flex flex-wrap gap-2">
                      <Button
                        variant="secondary"
                        disabled={
                          pending === entry.pull_request.id ||
                          entry.position === 1
                        }
                        onClick={() =>
                          void operate(
                            entry,
                            "reprioritize",
                            entry.position - 1,
                          )
                        }
                      >
                        Move up
                      </Button>
                      <Button
                        variant="secondary"
                        disabled={pending === entry.pull_request.id}
                        onClick={() =>
                          void operate(
                            entry,
                            entry.state === "paused" ? "resume" : "pause",
                          )
                        }
                      >
                        {entry.state === "paused" ? "Resume" : "Pause"}
                      </Button>
                      <Button
                        variant="secondary"
                        disabled={pending === entry.pull_request.id}
                        onClick={() => void operate(entry, "retry")}
                      >
                        Retry
                      </Button>
                      <Button
                        variant="quiet"
                        disabled={pending === entry.pull_request.id}
                        onClick={() => void operate(entry, "remove")}
                      >
                        Remove
                      </Button>
                    </div>
                  )}
                  {entry.pull_request.queue_actions &&
                    entry.pull_request.queue_actions.length > 0 && (
                      <details className="mt-4 text-xs text-[var(--muted)]">
                        <summary className="cursor-pointer font-semibold">
                          Intervention history (
                          {entry.pull_request.queue_actions.length})
                        </summary>
                        <ul className="mt-2 space-y-1">
                          {[...entry.pull_request.queue_actions]
                            .reverse()
                            .map((item, index) => (
                              <li key={`${item.created_at}-${index}`}>
                                {item.action} ·{" "}
                                {new Date(item.created_at).toLocaleString()} ·{" "}
                                <code>{item.actor_id.slice(0, 8)}</code>
                              </li>
                            ))}
                        </ul>
                      </details>
                    )}
                </Card>
              </li>
            ))}
          </ol>
        )}
      </div>
    </AccessGate>
  );
}
