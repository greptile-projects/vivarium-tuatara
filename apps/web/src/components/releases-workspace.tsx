"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  api,
  type Branch,
  type ReleaseAttestation,
  type ReleaseBuild,
  type ReleaseCandidate,
  type Repository,
  type User,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
import { DeploymentWorkspace } from "./deployment-workspace";

export function ReleasesWorkspace({ repositoryID }: { repositoryID: string }) {
  const { token, user, loading: authLoading } = useAuth();
  const [repository, setRepository] = useState<Repository | null>(null);
  const [participant, setParticipant] = useState(false);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [releases, setReleases] = useState<ReleaseCandidate[]>([]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const load = useCallback(async () => {
    if (authLoading) return;
    try {
      const [repo, branchSet, releaseSet] = await Promise.all([
        api<Repository>(`/repositories/${repositoryID}`, {}, token),
        api<{ branches: Branch[] }>(
          `/repositories/${repositoryID}/branches`,
          {},
          token,
        ),
        api<{ releases: ReleaseCandidate[] }>(
          `/repositories/${repositoryID}/releases`,
          {},
          token,
        ),
      ]);
      setRepository(repo);
      setBranches(branchSet.branches);
      setReleases(releaseSet.releases);
      setError("");
      if (token) {
        let after: string | null = null;
        let found = false;
        do {
          const page: {
            repositories: Repository[];
            next_cursor: string | null;
          } = await api(
            `/repositories?limit=100${after ? `&after=${encodeURIComponent(after)}` : ""}`,
            {},
            token,
          );
          found ||= page.repositories.some((item) => item.id === repositoryID);
          after = page.next_cursor;
        } while (after && !found);
        setParticipant(found);
      } else setParticipant(false);
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Release candidates could not be loaded.",
      );
    }
  }, [authLoading, repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    setError("");
    const form = event.currentTarget;
    const data = new FormData(form);
    try {
      const created = await api<ReleaseCandidate>(
        `/repositories/${repositoryID}/releases`,
        {
          method: "POST",
          body: JSON.stringify({
            version: data.get("version"),
            notes: data.get("notes"),
            commit_id: data.get("commit_id"),
            previous_release_id: data.get("previous_release_id") || undefined,
          }),
        },
        token,
      );
      form.reset();
      setReleases((current) => [...current, created]);
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Release candidate could not be created.",
      );
    } finally {
      setPending(false);
    }
  }
  const canCreate = !!token && !!user && !!repository && participant;
  return (
    <div className="space-y-6">
      <div>
        <Link
          href={`/repositories/${repositoryID}`}
          className="text-sm font-semibold text-[var(--brand)] hover:underline"
        >
          ← Repository
        </Link>
        <p className="mt-5 font-mono text-xs font-semibold uppercase tracking-[.14em] text-[var(--brand)]">
          Release delivery
        </p>
        <h1 className="mt-1 text-3xl font-semibold">Release candidates</h1>
        <p className="mt-2 max-w-2xl text-sm text-[var(--muted)]">
          Freeze an exact repository state and the collaboration included since
          a prior release before build or promotion begins.
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
      {canCreate && branches.length > 0 && (
        <Card className="p-5">
          <h2 className="text-lg font-semibold">Define a candidate</h2>
          <form onSubmit={create} className="mt-4 grid gap-4 md:grid-cols-2">
            <label className="text-xs font-semibold">
              Version
              <input
                name="version"
                required
                maxLength={100}
                placeholder="v1.0.0"
                className="mt-2 min-h-10 w-full rounded-lg border border-[var(--line-strong)] px-3 font-mono text-sm font-normal"
              />
            </label>
            <label className="text-xs font-semibold">
              Exact repository state
              <select
                name="commit_id"
                required
                className="mt-2 min-h-10 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-mono text-sm font-normal"
              >
                {branches.map((branch) => (
                  <option key={branch.name} value={branch.commit_id}>
                    {branch.name} · {branch.commit_id.slice(0, 12)}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-xs font-semibold md:col-span-2">
              Included since
              <select
                name="previous_release_id"
                className="mt-2 min-h-10 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm font-normal"
              >
                <option value="">Beginning of history</option>
                {[...releases].reverse().map((release) => (
                  <option key={release.id} value={release.id}>
                    {release.version} · {release.commit_id.slice(0, 12)}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-xs font-semibold md:col-span-2">
              Release notes
              <textarea
                name="notes"
                required
                maxLength={10000}
                rows={6}
                placeholder="What is changing, why, and what should participants know?"
                className="mt-2 w-full rounded-lg border border-[var(--line-strong)] p-3 text-sm font-normal"
              />
            </label>
            <div>
              <Button type="submit" disabled={pending}>
                {pending ? "Freezing…" : "Create release candidate"}
              </Button>
            </div>
          </form>
        </Card>
      )}
      <div className="grid gap-3">
        {releases.length === 0 ? (
          <Card className="p-8 text-center text-sm text-[var(--muted)]">
            No release candidates yet.
          </Card>
        ) : (
          [...releases].reverse().map((release) => (
            <Link
              key={release.id}
              href={`/repositories/${repositoryID}/releases/${release.id}`}
            >
              <Card className="p-5 transition hover:border-[var(--brand)]">
                <div className="flex items-center gap-3">
                  <h2 className="text-lg font-semibold">{release.version}</h2>
                  <Badge tone="warning">Candidate</Badge>
                </div>
                <p className="mt-2 line-clamp-2 text-sm text-[var(--muted)]">
                  {release.notes}
                </p>
                <code className="mt-3 block text-xs text-[var(--muted)]">
                  {release.commit_id}
                </code>
              </Card>
            </Link>
          ))
        )}
      </div>
    </div>
  );
}

export function ReleaseDetail({
  repositoryID,
  releaseID,
}: {
  repositoryID: string;
  releaseID: string;
}) {
  const { token, loading } = useAuth();
  const [release, setRelease] = useState<ReleaseCandidate | null>(null);
  const [builds, setBuilds] = useState<ReleaseBuild[]>([]);
  const [attestations, setAttestations] = useState<
    Record<string, ReleaseAttestation>
  >({});
  const [people, setPeople] = useState<Record<string, User>>({});
  const [error, setError] = useState("");
  const [identityWarning, setIdentityWarning] = useState("");
  const [building, setBuilding] = useState(false);
  useEffect(() => {
    if (loading) return;
    let active = true;
    api<ReleaseCandidate>(
      `/repositories/${repositoryID}/releases/${releaseID}`,
      {},
      token,
    )
      .then(async (item) => {
        if (!active) return;
        setRelease(item);
        setError("");
        const buildSet = await api<{ builds: ReleaseBuild[] }>(
          `/repositories/${repositoryID}/releases/${releaseID}/builds`,
          {},
          token,
        );
        if (!active) return;
        setBuilds(buildSet.builds);
        const evidence = await Promise.all(
          buildSet.builds.map((build) =>
            api<ReleaseAttestation>(
              `/repositories/${repositoryID}/releases/${releaseID}/builds/${build.id}/attestation`,
              {},
              token,
            ),
          ),
        );
        if (!active) return;
        setAttestations(
          Object.fromEntries(evidence.map((item) => [item.build_id, item])),
        );
        const identityIDs = [
          ...new Set([item.created_by, ...item.inclusions.contributor_ids]),
        ];
        const results = await Promise.allSettled(
          identityIDs.map((id) => api<User>(`/users/${id}`)),
        );
        if (!active) return;
        const users = results.flatMap((result) =>
          result.status === "fulfilled" ? [result.value] : [],
        );
        setPeople(Object.fromEntries(users.map((user) => [user.id, user])));
        setIdentityWarning(
          results.some((result) => result.status === "rejected")
            ? "Some contributor profiles are temporarily unavailable; stable IDs remain visible."
            : "",
        );
      })
      .catch((reason) => {
        if (active)
          setError(
            reason instanceof Error
              ? reason.message
              : "Release candidate could not be loaded.",
          );
      });
    return () => {
      active = false;
    };
  }, [loading, releaseID, repositoryID, token]);
  useEffect(() => {
    if (
      !builds.some((build) =>
        ["queued", "running", "cleanup_pending"].includes(build.state),
      )
    )
      return;
    const timer = window.setInterval(async () => {
      try {
        const buildSet = await api<{ builds: ReleaseBuild[] }>(
          `/repositories/${repositoryID}/releases/${releaseID}/builds`,
          {},
          token,
        );
        setBuilds(buildSet.builds);
        const evidence = await Promise.all(
          buildSet.builds.map((build) =>
            api<ReleaseAttestation>(
              `/repositories/${repositoryID}/releases/${releaseID}/builds/${build.id}/attestation`,
              {},
              token,
            ),
          ),
        );
        setAttestations(
          Object.fromEntries(evidence.map((item) => [item.build_id, item])),
        );
      } catch {
        /* The main error surface remains stable during transient polling failures. */
      }
    }, 1500);
    return () => window.clearInterval(timer);
  }, [builds, releaseID, repositoryID, token]);
  async function startBuild() {
    if (!token) return;
    setBuilding(true);
    setError("");
    try {
      const result = await api<{ builds: ReleaseBuild[] }>(
        `/repositories/${repositoryID}/releases/${releaseID}/builds`,
        { method: "POST" },
        token,
      );
      setBuilds(result.builds);
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Release build could not be started.",
      );
    } finally {
      setBuilding(false);
    }
  }
  async function rerun(buildID: string) {
    if (!token) return;
    try {
      const run = await api<ReleaseBuild>(
        `/repositories/${repositoryID}/releases/${releaseID}/builds/${buildID}/rerun`,
        { method: "POST" },
        token,
      );
      setBuilds((current) =>
        current.map((item) => (item.id === run.id ? run : item)),
      );
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Release build could not be rerun.",
      );
    }
  }
  if (error)
    return (
      <p
        role="alert"
        className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
      >
        {error}
      </p>
    );
  if (!release)
    return (
      <Card className="p-8 text-sm text-[var(--muted)]">
        Loading immutable release state…
      </Card>
    );
  const rows = [
    {
      label: "Pull requests",
      ids: release.inclusions.pull_request_ids,
      href: (id: string) => `/pulls/${repositoryID}/${id}`,
    },
    {
      label: "Proposals",
      ids: release.inclusions.proposal_ids,
      href: (id: string) => `/proposals/${repositoryID}/${id}`,
    },
    { label: "Plan tasks", ids: release.inclusions.task_ids },
  ];
  return (
    <div className="space-y-6">
      <div>
        <Link
          href={`/repositories/${repositoryID}/releases`}
          className="text-sm font-semibold text-[var(--brand)] hover:underline"
        >
          ← Release candidates
        </Link>
        <div className="mt-5 flex items-center gap-3">
          <h1 className="text-3xl font-semibold">{release.version}</h1>
          <Badge tone="warning">Candidate</Badge>
        </div>
        <p className="mt-2 text-sm text-[var(--muted)]">
          Defined by <Identity id={release.created_by} people={people} /> on{" "}
          {new Date(release.created_at).toLocaleString()}
        </p>
      </div>
      {identityWarning && (
        <p
          role="status"
          className="rounded-lg bg-[var(--warning-soft)] p-3 text-sm text-[var(--warning)]"
        >
          {identityWarning}
        </p>
      )}
      <Card className="p-5">
        <p className="font-mono text-xs font-semibold uppercase tracking-[.14em] text-[var(--brand)]">
          Exact source
        </p>
        <Link
          href={`/repositories/${repositoryID}?ref=${release.commit_id}`}
          className="mt-3 block break-all font-mono text-sm font-semibold text-[var(--brand)] hover:underline"
        >
          {release.commit_id}
        </Link>
        {release.previous_commit_id && (
          <p className="mt-3 text-xs text-[var(--muted)]">
            Changes after <code>{release.previous_commit_id}</code>
          </p>
        )}
      </Card>
      <Card className="p-5">
        <div className="flex items-center justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold">Builds and attestations</h2>
            <p className="mt-1 text-sm text-[var(--muted)]">
              Isolated steps declared by this exact commit retain checksummed
              output and every attempt.
            </p>
          </div>
          {token && (
            <Button onClick={startBuild} disabled={building}>
              {building ? "Starting…" : "Run release build"}
            </Button>
          )}
        </div>
        <div className="mt-4 space-y-3">
          {builds.length === 0 ? (
            <p className="text-sm text-[var(--muted)]">
              No build evidence yet. Add <code>.vivarium/release.json</code> to
              the source commit to define release steps.
            </p>
          ) : (
            builds.map((build) => {
              const attestation = attestations[build.id];
              return (
                <div
                  key={build.id}
                  className="rounded-lg border border-[var(--line)] p-4"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="font-semibold">{build.definition.name}</h3>
                    <Badge
                      tone={
                        build.state === "succeeded"
                          ? "success"
                          : build.state === "failed"
                            ? "danger"
                            : "warning"
                      }
                    >
                      {build.state}
                    </Badge>
                    {token &&
                      !["queued", "running", "cleanup_pending"].includes(
                        build.state,
                      ) && (
                        <Button
                          variant="secondary"
                          onClick={() => rerun(build.id)}
                        >
                          Rerun
                        </Button>
                      )}
                  </div>
                  <p className="mt-2 break-all font-mono text-xs">
                    {build.definition.command}
                  </p>
                  <p className="mt-2 text-xs text-[var(--muted)]">
                    Dependency: {build.definition.image} ·{" "}
                    {build.attempts.length} attempt
                    {build.attempts.length === 1 ? "" : "s"}
                  </p>
                  {build.failure && (
                    <p className="mt-2 text-sm text-[var(--danger)]">
                      {build.failure}
                    </p>
                  )}
                  <div className="mt-3 space-y-2">
                    {(attestation?.artifacts ?? build.artifacts).map(
                      (artifact) => (
                        <div key={artifact.id} className="text-xs">
                          <span className="font-semibold">{artifact.path}</span>{" "}
                          · {artifact.size} bytes
                          <br />
                          <code className="break-all text-[var(--muted)]">
                            sha256:{artifact.sha256}
                          </code>
                        </div>
                      ),
                    )}
                  </div>
                  {attestation && (
                    <details className="mt-3">
                      <summary className="cursor-pointer text-sm font-semibold text-[var(--brand)]">
                        Machine-readable attestation
                      </summary>
                      <pre className="mt-2 overflow-x-auto rounded-lg bg-[var(--canvas)] p-3 text-xs">
                        {JSON.stringify(attestation, null, 2)}
                      </pre>
                    </details>
                  )}
                </div>
              );
            })
          )}
        </div>
      </Card>
      <DeploymentWorkspace
        repositoryID={repositoryID}
        releaseID={releaseID}
        builds={builds}
      />
      <Card className="p-5">
        <h2 className="text-lg font-semibold">Release notes</h2>
        <p className="mt-3 whitespace-pre-wrap text-sm leading-6">
          {release.notes}
        </p>
      </Card>
      <div className="grid gap-4 md:grid-cols-2">
        {rows.map((row) => (
          <Card key={row.label} className="p-5">
            <h2 className="font-semibold">
              {row.label}{" "}
              <span className="text-[var(--muted)]">({row.ids.length})</span>
            </h2>
            <div className="mt-3 space-y-2">
              {row.ids.length === 0 ? (
                <p className="text-sm text-[var(--muted)]">
                  None in this range.
                </p>
              ) : (
                row.ids.map((id) =>
                  row.href ? (
                    <Link
                      key={id}
                      href={row.href(id)}
                      className="block truncate font-mono text-xs text-[var(--brand)] hover:underline"
                    >
                      {id}
                    </Link>
                  ) : (
                    <code key={id} className="block truncate text-xs">
                      {id}
                    </code>
                  ),
                )
              )}
            </div>
          </Card>
        ))}
        <Card className="p-5">
          <h2 className="font-semibold">
            Contributors{" "}
            <span className="text-[var(--muted)]">
              ({release.inclusions.contributor_ids.length})
            </span>
          </h2>
          <div className="mt-3 space-y-2">
            {release.inclusions.contributor_ids.map((id) => (
              <p key={id} className="text-sm">
                <Identity id={id} people={people} />
              </p>
            ))}
          </div>
        </Card>
      </div>
    </div>
  );
}
function Identity({
  id,
  people,
}: {
  id: string;
  people: Record<string, User>;
}) {
  return people[id] ? <span>@{people[id].handle}</span> : <code>{id}</code>;
}
