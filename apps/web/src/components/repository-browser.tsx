"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import {
  api,
  type Branch,
  type Commit,
  type Repository,
  type TreeEntry,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Icons } from "./icons";
import { Badge, Card } from "./ui";

type TreeResult = { revision: string; path: string; entries: TreeEntry[] };
type BlobResult = {
  revision: string;
  path: string;
  size: number;
  is_binary: boolean;
  content: string;
};

export function RepositoryBrowser({ id }: { id: string }) {
  const { token, loading: authLoading } = useAuth();
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

  const load = useCallback(async () => {
    if (authLoading) return;
    setLoading(true);
    setError("");
    setTree(null);
    setBlob(null);
    try {
      const repo = await api<Repository>(`/repositories/${id}`, {}, token);
      setRepository(repo);
      const revision = selectedRef || repo.default_branch;
      const branchData = await api<{ branches: Branch[] }>(
        `/repositories/${id}/branches`,
        {},
        token,
      );
      setBranches(branchData.branches);
      if (branchData.branches.length === 0) {
        setCommits([]);
        return;
      }
      const history = await api<{ commits: Commit[] }>(
        `/repositories/${id}/commits?ref=${encodeURIComponent(revision)}`,
        {},
        token,
      );
      setCommits(history.commits);
      if (!currentPath)
        setTree(
          await api<TreeResult>(
            `/repositories/${id}/tree?ref=${encodeURIComponent(revision)}`,
            {},
            token,
          ),
        );
      else {
        try {
          setBlob(
            await api<BlobResult>(
              `/repositories/${id}/blob?ref=${encodeURIComponent(revision)}&path=${encodeURIComponent(currentPath)}`,
              {},
              token,
            ),
          );
        } catch {
          setTree(
            await api<TreeResult>(
              `/repositories/${id}/tree?ref=${encodeURIComponent(revision)}&path=${encodeURIComponent(currentPath)}`,
              {},
              token,
            ),
          );
        }
      }
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Repository could not be loaded.",
      );
    } finally {
      setLoading(false);
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
              {branches.length === 0 && <option>{revision}</option>}
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
                href={href(id, revision, "")}
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
                      href={href(id, revision, target)}
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
                revision={revision}
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
