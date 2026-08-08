"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import {
  api,
  type Branch,
  type Collaborator,
  type Commit,
  type Repository,
  type TreeEntry,
  type User,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Icons } from "./icons";
import { Badge, Button, Card } from "./ui";

type TreeResult = { revision: string; path: string; entries: TreeEntry[] };
type BlobResult = {
  revision: string;
  path: string;
  size: number;
  is_binary: boolean;
  content: string;
  truncated: boolean;
};

export function RepositoryBrowser({ id }: { id: string }) {
  const { token, user, loading: authLoading } = useAuth();
  const router = useRouter();
  const search = useSearchParams();
  const selectedRef = search.get("ref") ?? "";
  const currentPath = search.get("path") ?? "";
  const [repository, setRepository] = useState<Repository | null>(null);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [commits, setCommits] = useState<Commit[]>([]);
  const [tree, setTree] = useState<TreeResult | null>(null);
  const [blob, setBlob] = useState<BlobResult | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const loadGeneration = useRef(0);

  const load = useCallback(async () => {
    if (authLoading) return;
    const generation = ++loadGeneration.current;
    const active = () => loadGeneration.current === generation;
    setLoading(true);
    setError("");
    setTree(null);
    setBlob(null);
    try {
      const repo = await api<Repository>(`/repositories/${id}`, {}, token);
      if (!active()) return;
      setRepository(repo);
      const revision = selectedRef || repo.default_branch;
      const branchData = await api<{ branches: Branch[] }>(
        `/repositories/${id}/branches`,
        {},
        token,
      );
      if (!active()) return;
      setBranches(branchData.branches);
      if (branchData.branches.length === 0) {
        setCommits([]);
        return;
      }
      const pinnedRevision =
        branchData.branches.find((branch) => branch.name === revision)
          ?.commit_id ?? revision;
      const history = await api<{ commits: Commit[] }>(
        `/repositories/${id}/commits?limit=20&ref=${encodeURIComponent(pinnedRevision)}`,
        {},
        token,
      );
      if (!active()) return;
      setCommits(history.commits);
      if (!currentPath) {
        const result = await api<TreeResult>(
          `/repositories/${id}/tree?ref=${encodeURIComponent(pinnedRevision)}`,
          {},
          token,
        );
        if (active()) setTree(result);
      } else {
        try {
          const result = await api<BlobResult>(
            `/repositories/${id}/blob?ref=${encodeURIComponent(pinnedRevision)}&path=${encodeURIComponent(currentPath)}`,
            {},
            token,
          );
          if (active()) setBlob(result);
        } catch {
          if (!active()) return;
          const result = await api<TreeResult>(
            `/repositories/${id}/tree?ref=${encodeURIComponent(pinnedRevision)}&path=${encodeURIComponent(currentPath)}`,
            {},
            token,
          );
          if (active()) setTree(result);
        }
      }
    } catch (reason) {
      if (active())
        setError(
          reason instanceof Error
            ? reason.message
            : "Repository could not be loaded.",
        );
    } finally {
      if (active()) setLoading(false);
    }
  }, [authLoading, currentPath, id, selectedRef, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  if (loading)
    return (
      <Card className="p-8 text-sm text-[var(--muted)]">
        Loading repository state…
      </Card>
    );
  if (error || !repository)
    return (
      <Card className="p-8">
        <h1 className="text-xl font-semibold">Repository unavailable</h1>
        <p role="alert" className="mt-2 text-sm text-[var(--danger)]">
          {error}
        </p>
        <Link
          href="/repositories"
          className="mt-5 inline-flex text-sm font-semibold text-[var(--brand)]"
        >
          Back to repositories
        </Link>
      </Card>
    );

  const revision = selectedRef || repository.default_branch;
  const resolved = tree?.revision ?? blob?.revision;
  const immutableRevision = resolved ?? commits[0]?.id ?? revision;

  const namedRevision = branches.some((branch) => branch.name === revision);
  const cloneURL =
    typeof window === "undefined"
      ? repository.git_remote
      : `${window.location.origin}${repository.git_remote}`;
  const parts = currentPath.split("/").filter(Boolean);
  const head = commits[0];
  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <Link
            href="/repositories"
            className="text-sm text-[var(--muted)] hover:text-[var(--brand)]"
          >
            Repositories
          </Link>
          <div className="mt-2 flex items-center gap-3">
            <h1 className="font-mono text-2xl font-semibold">
              {repository.name}
            </h1>
            <Badge>{repository.visibility}</Badge>
          </div>
          <p className="mt-2 text-sm text-[var(--muted)]">
            Created {new Date(repository.created_at).toLocaleDateString()} ·
            default branch <code>{repository.default_branch}</code>
          </p>
        </div>
        <div className="min-w-0 rounded-xl border border-[var(--line)] bg-white p-3">
          <p className="text-xs font-semibold uppercase tracking-wider text-[var(--muted)]">
            Clone repository
          </p>
          <code className="mt-1 block max-w-xl overflow-x-auto whitespace-nowrap text-xs">
            git clone {cloneURL}
          </code>
        </div>
      </header>
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <section className="min-w-0 space-y-3">
          <div className="flex items-center gap-2">
            <Icons.Branch size={16} />
            <label className="sr-only" htmlFor="revision">
              Branch
            </label>
            <select
              id="revision"
              value={revision}
              onChange={(event) =>
                router.push(
                  `/repositories/${id}?ref=${encodeURIComponent(event.target.value)}`,
                )
              }
              className="min-h-9 rounded-lg border border-[var(--line-strong)] bg-white px-3 font-mono text-sm font-semibold"
            >
              {!namedRevision && <option>{revision}</option>}
              {branches.map((branch) => (
                <option key={branch.name}>{branch.name}</option>
              ))}
            </select>
            {resolved && (
              <span className="text-xs text-[var(--muted)]">
                at <code title={resolved}>{resolved.slice(0, 7)}</code>
              </span>
            )}
          </div>
          <Card className="overflow-hidden">
            <div className="flex flex-wrap items-center gap-1 border-b border-[var(--line)] bg-[var(--surface)] px-4 py-3 text-sm">
              <Link
                href={href(id, immutableRevision, "")}
                className="font-semibold text-[var(--brand)]"
              >
                {repository.name}
              </Link>
              {parts.map((part, index) => {
                const target = parts.slice(0, index + 1).join("/");
                return (
                  <span key={target} className="flex items-center gap-1">
                    <Icons.Chevron size={13} />
                    <Link
                      href={href(id, immutableRevision, target)}
                      className="hover:underline"
                    >
                      {part}
                    </Link>
                  </span>
                );
              })}
            </div>
            {tree ? (
              <TreeList
                id={id}
                revision={immutableRevision}
                path={currentPath}
                entries={tree.entries}
              />
            ) : blob ? (
              <BlobView blob={blob} />
            ) : (
              <div className="p-10 text-center">
                <h2 className="font-semibold">Ready for the first commit</h2>
                <p className="mt-2 text-sm text-[var(--muted)]">
                  Clone this repository and push {repository.default_branch} to
                  make its files and history visible here.
                </p>
              </div>
            )}
          </Card>
        </section>
        <aside className="space-y-4">
          {user?.id === repository.owner_id && token && (
            <>
              <RequiredChecksPanel repositoryID={id} branch={repository.default_branch} token={token} />
              <CollaboratorPanel repositoryID={id} token={token} />
            </>
          )}
          <Card className="p-5">
            <h2 className="font-semibold">Current revision</h2>
            {head ? (
              <>
                <p className="mt-3 line-clamp-2 text-sm font-medium">
                  {subject(head.message)}
                </p>
                <p className="mt-2 text-xs text-[var(--muted)]">
                  {author(head.author)} ·{" "}
                  {head.authored_at
                    ? new Date(head.authored_at).toLocaleString()
                    : "date unavailable"}
                </p>
                <code className="mt-3 block break-all text-xs text-[var(--muted)]">
                  {head.id}
                </code>
              </>
            ) : (
              <p className="mt-2 text-sm text-[var(--muted)]">
                This branch has no commits yet.
              </p>
            )}
          </Card>
          <Card className="overflow-hidden">
            <div className="border-b border-[var(--line)] px-5 py-4">
              <h2 className="font-semibold">Commit history</h2>
              <p className="mt-1 text-xs text-[var(--muted)]">
                Ancestry from this exact revision
              </p>
            </div>
            <div className="divide-y divide-[var(--line)]">
              {commits.slice(0, 20).map((commit) => (
                <Link
                  href={href(id, commit.id, "")}
                  key={commit.id}
                  className="block p-4 hover:bg-[var(--brand-soft)]"
                >
                  <p className="truncate text-sm font-medium">
                    {subject(commit.message)}
                  </p>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    {author(commit.author)} ·{" "}
                    <code>{commit.id.slice(0, 7)}</code>
                  </p>
                </Link>
              ))}
            </div>
          </Card>
        </aside>
      </div>
    </div>
  );
}

function RequiredChecksPanel({ repositoryID, branch, token }: { repositoryID: string; branch: string; token: string }) {
  const [checks, setChecks] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const endpoint = `/repositories/${repositoryID}/branches/${encodeURIComponent(branch)}/required-checks`;
  useEffect(() => {
    let active = true;
    api<{ checks: string[] }>(endpoint, {}, token).then((result) => { if (active) setChecks(result.checks); }).catch((reason) => { if (active) setError(reason instanceof Error ? reason.message : "Required checks could not be loaded."); });
    return () => { active = false; };
  }, [endpoint, token]);
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setPending(true); setError("");
    const names = String(new FormData(event.currentTarget).get("checks") ?? "").split("\n").map((name) => name.trim()).filter(Boolean);
    try {
      const result = await api<{ checks: string[] }>(endpoint, { method: "PUT", body: JSON.stringify({ checks: names }) }, token);
      setChecks(result.checks);
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Required checks could not be saved."); }
    finally { setPending(false); }
  }
  return <Card className="p-5"><h2 className="font-semibold">Required checks</h2><p className="mt-1 text-xs leading-5 text-[var(--muted)]">One repository-defined check name per line. Pull requests into <code>{branch}</code> must pass every name on their exact revision.</p><form onSubmit={save} className="mt-4 space-y-3"><label className="sr-only" htmlFor="required-checks">Required check names</label><textarea id="required-checks" name="checks" rows={3} maxLength={2020} defaultValue={checks.join("\n")} key={checks.join("\n")} placeholder={"web\napi"} className="w-full rounded-lg border border-[var(--line-strong)] p-3 font-mono text-xs"/><Button type="submit" disabled={pending}>{pending ? "Saving…" : "Save requirements"}</Button></form>{error && <p role="alert" className="mt-3 text-sm text-[var(--danger)]">{error}</p>}</Card>;
}

function CollaboratorPanel({ repositoryID, token }: { repositoryID: string; token: string }) {
  const [collaborators, setCollaborators] = useState<Collaborator[]>([]);
  const [users, setUsers] = useState<Record<string, User>>({});
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  const load = useCallback(async () => {
    try {
      const result = await api<{ collaborators: Collaborator[] }>(`/repositories/${repositoryID}/collaborators`, {}, token);
      setCollaborators(result.collaborators);
      const resolved = await Promise.all(result.collaborators.map((item) => api<User>(`/users/${item.user_id}`)));
      setUsers(Object.fromEntries(resolved.map((item) => [item.id, item])));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Collaborators could not be loaded.");
    }
  }, [repositoryID, token]);
  useEffect(() => { void Promise.resolve().then(load); }, [load]);

  async function add(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const userID = String(new FormData(form).get("user_id") ?? "").trim();
    setPending(true); setError("");
    try {
      await api<Collaborator>(`/repositories/${repositoryID}/collaborators`, { method: "POST", body: JSON.stringify({ user_id: userID }) }, token);
      form.reset(); await load();
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Contributor could not be added."); }
    finally { setPending(false); }
  }

  async function remove(userID: string) {
    setPending(true); setError("");
    try { await api<void>(`/repositories/${repositoryID}/collaborators/${userID}`, { method: "DELETE" }, token); await load(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Contributor could not be removed."); }
    finally { setPending(false); }
  }

  return <Card className="p-5"><h2 className="font-semibold">Contributors</h2><p className="mt-1 text-xs leading-5 text-[var(--muted)]">Grant a user access to this repository using the collaboration ID from their settings.</p><form onSubmit={add} className="mt-4 flex gap-2"><label className="sr-only" htmlFor="collaborator-id">Collaboration ID</label><input id="collaborator-id" name="user_id" required pattern="[a-f0-9]{32}" placeholder="32-character ID" className="min-h-10 min-w-0 flex-1 rounded-lg border border-[var(--line-strong)] px-3 font-mono text-xs"/><Button type="submit" disabled={pending}>{pending ? "Adding…" : "Add"}</Button></form>{error && <p role="alert" className="mt-3 text-sm text-[var(--danger)]">{error}</p>}<div className="mt-4 divide-y divide-[var(--line)]">{collaborators.length === 0 ? <p className="py-2 text-sm text-[var(--muted)]">No contributors yet.</p> : collaborators.map((item) => <div key={item.user_id} className="flex items-center gap-2 py-3"><div className="min-w-0 flex-1"><p className="truncate text-sm font-semibold">{users[item.user_id] ? `@${users[item.user_id].handle}` : item.user_id}</p><p className="text-xs text-[var(--muted)]">Contributor</p></div><Button variant="quiet" disabled={pending} onClick={() => void remove(item.user_id)}>Remove</Button></div>)}</div></Card>;
}

function TreeList({
  id,
  revision,
  path,
  entries,
}: {
  id: string;
  revision: string;
  path: string;
  entries: TreeEntry[];
}) {
  if (!entries.length)
    return (
      <p className="p-8 text-center text-sm text-[var(--muted)]">
        This directory is empty.
      </p>
    );
  return (
    <div className="divide-y divide-[var(--line)]">
      {entries.map((entry) => {
        const target = [path, entry.name].filter(Boolean).join("/");
        return (
          <Link
            key={entry.name}
            href={href(id, revision, target)}
            className="flex items-center gap-3 px-4 py-3 text-sm hover:bg-[var(--brand-soft)]"
          >
            <span className="text-[var(--brand)]">
              {entry.type === "tree" ? (
                <Icons.Branch size={16} />
              ) : (
                <Icons.Code size={16} />
              )}
            </span>
            <span className="min-w-0 flex-1 truncate font-mono">
              {entry.name}
            </span>
            <span className="text-xs text-[var(--muted)]">
              {entry.type === "tree"
                ? "directory"
                : entry.mode === "100755"
                  ? "executable"
                  : "file"}
            </span>
          </Link>
        );
      })}
    </div>
  );
}

function BlobView({ blob }: { blob: BlobResult }) {
  return blob.is_binary ? (
    <div className="p-8 text-center">
      <h2 className="font-semibold">Binary file</h2>
      <p className="mt-2 text-sm text-[var(--muted)]">
        Preview unavailable · {blob.size.toLocaleString()} bytes
      </p>
    </div>
  ) : (
    <div>
      <div className="border-b border-[var(--line)] px-4 py-2 text-xs text-[var(--muted)]">
        {blob.size.toLocaleString()} bytes ·{" "}
        <code>{blob.revision.slice(0, 7)}</code>
        {blob.truncated && " · preview limited to 512 KiB"}
      </div>
      <pre className="max-h-[45rem] overflow-auto p-4 font-mono text-xs leading-6">
        <code>{blob.content}</code>
      </pre>
    </div>
  );
}
function href(id: string, revision: string, path: string) {
  return `/repositories/${id}?ref=${encodeURIComponent(revision)}${path ? `&path=${encodeURIComponent(path)}` : ""}`;
}
function subject(message: string) {
  return message.trim().split("\n")[0] || "Untitled commit";
}
function author(value: string) {
  return value.match(/^(.*?)\s+<[^>]+>/)?.[1] || "Unknown author";
}
