"use client";

import { useCallback, useEffect, useState } from "react";
import { api, type PullPreview } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

export function PullRequestPreviews({
  repositoryID,
  pullRequestID,
  participant,
  owner,
}: {
  repositoryID: string;
  pullRequestID: string;
  participant: boolean;
  owner: boolean;
}) {
  const { token } = useAuth();
  const [items, setItems] = useState<PullPreview[]>([]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [audience, setAudience] = useState({
    source_kind: "user",
    user_id: "",
    source_id: "",
    role: "view",
    expires_at: "",
  });
  const base = `/repositories/${repositoryID}/pulls/${pullRequestID}/previews`;
  const load = useCallback(async () => {
    try {
      setItems(
        (await api<{ previews: PullPreview[] }>(base, {}, token)).previews,
      );
      setError("");
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Previews could not be loaded.",
      );
    }
  }, [base, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
    const timer = window.setInterval(() => void load(), 3000);
    return () => window.clearInterval(timer);
  }, [load]);
  async function launch() {
    setPending(true);
    try {
      await api(base, { method: "POST" }, token);
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Preview could not be launched.",
      );
    } finally {
      setPending(false);
    }
  }
  async function invite(previewID: string) {
    setPending(true);
    try {
      await api(
        `${base}/${previewID}/invitations`,
        {
          method: "POST",
          body: JSON.stringify({
            ...audience,
            expires_at: new Date(audience.expires_at).toISOString(),
          }),
        },
        token,
      );
      setAudience({ ...audience, user_id: "", source_id: "" });
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Audience could not be invited.",
      );
    } finally {
      setPending(false);
    }
  }
  async function revoke(previewID: string, invitationID: string) {
    setPending(true);
    try {
      await api(
        `${base}/${previewID}/invitations/${invitationID}`,
        { method: "DELETE" },
        token,
      );
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Invitation could not be revoked.",
      );
    } finally {
      setPending(false);
    }
  }
  return (
    <section id="previews" className="scroll-mt-24 space-y-3">
      <div className="flex items-baseline justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Change previews</h2>
          <p className="text-xs text-[var(--muted)]">
            Isolated experiences pinned to one review revision
          </p>
        </div>
        {participant && (
          <Button disabled={pending} onClick={() => void launch()}>
            {pending ? "Launching…" : "Launch preview"}
          </Button>
        )}
      </div>
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      {items.length === 0 ? (
        <Card className="p-5 text-sm text-[var(--muted)]">
          No preview has been published. Add a version 1{" "}
          <code>.vivarium/preview.json</code> with an explicit access policy to
          opt in.
        </Card>
      ) : (
        items.map((item) => (
          <Card key={item.id} className="p-5">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p className="font-semibold">
                  Preview <code>{item.id.slice(0, 8)}</code>
                </p>
                <p className="mt-1 text-xs text-[var(--muted)]">
                  revision <code>{item.revision.slice(0, 12)}</code> · created
                  by {item.creator_id} ·{" "}
                  {new Date(item.created_at).toLocaleString()}
                </p>
              </div>
              <Badge
                tone={
                  item.stale
                    ? "warning"
                    : item.state === "succeeded"
                      ? "success"
                      : item.state === "failed"
                        ? "danger"
                        : "neutral"
                }
              >
                {item.stale ? "stale" : item.state}
              </Badge>
            </div>
            <p className="mt-3 text-xs">
              Definition <code>{item.definition_sha256.slice(0, 12)}</code> ·{" "}
              {item.definition.resources.cpus} CPU ·{" "}
              {item.definition.resources.memory_mb} MiB memory ·{" "}
              {item.definition.resources.storage_mb} MiB output
            </p>
            <div className="mt-3 rounded-lg border border-[var(--line)] p-3 text-xs">
              <p className="font-semibold">Safe audience boundary</p>
              <p className="mt-1">
                Named, expiring guests · network{" "}
                {item.definition.access.network} · data limited to built preview
                artifacts · actions {item.definition.access.actions.join(", ")}.
              </p>
              <p className="mt-1 text-[var(--muted)]">
                An invitation grants no repository, credential, workspace,
                environment, deployment, or production authority. Build logs and
                private services remain participant-only.
              </p>
            </div>
            <div className="mt-4 flex gap-3 text-sm">
              <a
                className="font-semibold text-[var(--accent)]"
                href={`/api${item.url}`}
                target="_blank"
                rel="noreferrer"
              >
                Open exact preview
              </a>
              <a
                className="text-[var(--muted)] underline"
                href={`/api${base}/${item.id}/events`}
                target="_blank"
                rel="noreferrer"
              >
                Build logs
              </a>
            </div>
            {owner && (
              <div className="mt-4 space-y-3 border-t border-[var(--line)] pt-4">
                <p className="text-sm font-semibold">Preview audience</p>
                <div className="grid gap-2 sm:grid-cols-4">
                  <select
                    className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm"
                    value={audience.source_kind}
                    onChange={(e) =>
                      setAudience({ ...audience, source_kind: e.target.value })
                    }
                  >
                    <option value="user">Named user</option>
                    <option value="issue">Issue participants</option>
                    <option value="decision">Decision participants</option>
                    <option value="proposal">Proposal participants</option>
                  </select>
                  <input
                    className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm sm:col-span-2"
                    value={
                      audience.source_kind === "user"
                        ? audience.user_id
                        : audience.source_id
                    }
                    onChange={(e) =>
                      setAudience({
                        ...audience,
                        [audience.source_kind === "user"
                          ? "user_id"
                          : "source_id"]: e.target.value,
                      })
                    }
                    placeholder={
                      audience.source_kind === "user"
                        ? "User ID"
                        : "Resource ID"
                    }
                  />
                  <select
                    className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm"
                    value={audience.role}
                    onChange={(e) =>
                      setAudience({ ...audience, role: e.target.value })
                    }
                  >
                    {item.definition.access.actions.map((action) => (
                      <option key={action} value={action}>
                        {action}
                      </option>
                    ))}
                  </select>
                </div>
                <label className="block text-xs">
                  Expires at
                  <input
                    className="ml-2 rounded-lg border border-[var(--line)] bg-transparent p-2"
                    type="datetime-local"
                    value={audience.expires_at}
                    onChange={(e) =>
                      setAudience({ ...audience, expires_at: e.target.value })
                    }
                  />
                </label>
                <Button
                  disabled={pending || !audience.expires_at}
                  variant="secondary"
                  onClick={() => void invite(item.id)}
                >
                  Invite audience
                </Button>
                <ul className="space-y-2 text-xs">
                  {item.invitations?.map((inv) => (
                    <li
                      key={inv.id}
                      className="flex items-center justify-between gap-2"
                    >
                      <span>
                        {inv.user_id} · {inv.role} ·{" "}
                        {inv.revoked_at
                          ? "revoked"
                          : `expires ${new Date(inv.expires_at).toLocaleString()}`}
                      </span>
                      {!inv.revoked_at && (
                        <Button
                          variant="quiet"
                          onClick={() => void revoke(item.id, inv.id)}
                        >
                          Revoke
                        </Button>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            )}
            {item.stale && (
              <p className="mt-3 text-xs text-[var(--warning)]">
                The pull request has moved. This retained URL still represents
                the revision guests evaluated.
              </p>
            )}
          </Card>
        ))
      )}
    </section>
  );
}
