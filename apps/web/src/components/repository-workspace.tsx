"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, type Repository } from "@/lib/api";
import { useAuth } from "./auth";
import { Icons } from "./icons";
import { Badge, Button, Card } from "./ui";

export function RepositoryWorkspace({ compact = false }: { compact?: boolean }) {
  const { token, user } = useAuth();
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    if (!token) return;
    try {
      const found: Repository[] = [];
      let after: string | null = null;
      do {
        const query = after ? `?limit=100&after=${encodeURIComponent(after)}` : "?limit=100";
        const result: { repositories: Repository[]; next_cursor: string | null } = await api(`/repositories${query}`, {}, token);
        found.push(...result.repositories);
        after = result.next_cursor;
      } while (after);
      setRepositories(found);
    }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Repositories could not be loaded."); }
    finally { setLoading(false); }
  }, [token]);
  useEffect(() => { void Promise.resolve().then(load); }, [load]);

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setCreating(true); setError("");
    const form = event.currentTarget;
    const name = String(new FormData(form).get("name") ?? "");
    try { const repository = await api<Repository>("/repositories", { method: "POST", body: JSON.stringify({ name }) }, token); setRepositories((items) => [...items, repository]); form.reset(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Repository could not be created."); }
    finally { setCreating(false); }
  }

  return <div className={compact ? "grid gap-6 xl:grid-cols-[1.1fr_.9fr]" : "space-y-7"}>
    <Card className="p-6 sm:p-7"><div className="flex items-start gap-4"><span className="grid size-11 shrink-0 place-items-center rounded-xl bg-[var(--brand-soft)] text-[var(--brand)]"><Icons.Plus /></span><div><h2 className="text-lg font-semibold">Create a repository</h2><p className="mt-1 text-sm leading-6 text-[var(--muted)]">Give an idea a durable home, then add collaborators and candidate branches.</p></div></div><form onSubmit={create} className="mt-6 flex flex-col gap-3 sm:flex-row"><label className="sr-only" htmlFor="repository-name">Repository name</label><input id="repository-name" name="name" required maxLength={100} pattern="[A-Za-z0-9._-]+" placeholder="project-name" className="min-h-11 flex-1 rounded-lg border border-[var(--line-strong)] bg-white px-3 font-mono text-sm outline-none focus:border-[var(--brand)]"/><Button type="submit" disabled={creating}>{creating ? "Creating…" : "Create repository"}</Button></form><p className="mt-2 text-xs text-[var(--muted)]">Private by default · starts on an unborn <code>main</code> branch</p>{error && <p role="alert" className="mt-4 rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}</Card>
    <section aria-labelledby="repo-list-heading"><div className="mb-3"><h2 id="repo-list-heading" className="text-lg font-semibold">{compact ? "Your starting points" : "Your repositories"}</h2><p className="mt-1 text-sm text-[var(--muted)]">Spaces you own and can build in.</p></div><Card className="overflow-hidden">{loading ? <p className="p-6 text-sm text-[var(--muted)]">Loading repositories…</p> : repositories.length === 0 ? <div className="p-8 text-center"><h3 className="font-semibold">No repositories yet</h3><p className="mt-2 text-sm text-[var(--muted)]">Create one above to begin your first collaboration.</p></div> : <div className="divide-y divide-[var(--line)]">{repositories.map((repository) => <article key={repository.id} className="flex items-center gap-3 p-4 sm:p-5"><span className="grid size-9 shrink-0 place-items-center rounded-lg border border-[var(--line)] text-[var(--brand)]"><Icons.Code size={17}/></span><div className="min-w-0 flex-1"><div className="flex items-center gap-2"><h3 className="truncate font-mono text-sm font-semibold">{repository.owner_id === user?.id ? user.handle : "shared"}/{repository.name}</h3><Badge>{repository.visibility}</Badge></div><p className="mt-1 text-xs text-[var(--muted)]">{repository.default_branch} · created {new Date(repository.created_at).toLocaleDateString()}</p></div><code className="hidden max-w-52 truncate text-xs text-[var(--muted)] sm:block">{repository.git_remote}</code></article>)}</div>}</Card></section>
  </div>;
}
