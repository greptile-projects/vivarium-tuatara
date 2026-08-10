"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import { api, type CodeNavigationResult } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

export function CodeNavigationWorkspace({ id }: { id: string }) {
  const params = useSearchParams();
  const { token } = useAuth();
  const [query, setQuery] = useState(params.get("q") ?? "");
  const [revision, setRevision] = useState(params.get("ref") ?? "");
  const [result, setResult] = useState<CodeNavigationResult | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function search(q = query, ref = revision) {
    if (!q.trim() || !ref.trim()) return;
    setLoading(true); setError("");
    try { setResult(await api<CodeNavigationResult>(`/repositories/${id}/code-navigation?q=${encodeURIComponent(q.trim())}&ref=${encodeURIComponent(ref.trim())}`, {}, token)); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Code navigation failed."); }
    finally { setLoading(false); }
  }
  function submit(event: FormEvent) { event.preventDefault(); const url = new URL(window.location.href); url.searchParams.set("q", query); url.searchParams.set("ref", revision); window.history.replaceState(null, "", url); void search(); }

  return <div className="space-y-6">
    <header><Link href={`/repositories/${id}`} className="text-sm text-[var(--muted)] hover:text-[var(--brand)]">Repository</Link><h1 className="mt-2 text-2xl font-semibold">Code navigation</h1><p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">Find definitions, references, callers, and tests at one exact commit, with ownership, commit evidence, and declared repository dependencies kept on that same revision.</p></header>
    <Card className="p-5"><form onSubmit={submit} className="grid gap-3 md:grid-cols-[1fr_2fr_auto]"><label className="text-xs font-semibold">Symbol or text<input value={query} onChange={e=>setQuery(e.target.value)} required maxLength={200} placeholder="authorizeRepositoryRead" className="mt-1 min-h-10 w-full rounded-lg border border-[var(--line-strong)] px-3 font-mono font-normal" /></label><label className="text-xs font-semibold">Exact commit SHA<input value={revision} onChange={e=>setRevision(e.target.value)} required pattern="[0-9a-f]{40}" placeholder="40-character commit" className="mt-1 min-h-10 w-full rounded-lg border border-[var(--line-strong)] px-3 font-mono font-normal" /></label><Button className="self-end" disabled={loading}>{loading ? "Analyzing…" : "Navigate"}</Button></form>{error&&<p role="alert" className="mt-3 text-sm text-[var(--danger)]">{error}</p>}</Card>
    {result&&<><Card className="p-5"><div className="flex flex-wrap items-center gap-2"><Badge tone={result.analysis.status==="complete"?"success":"info"}>{result.analysis.status}</Badge><code className="text-xs">{result.revision}</code></div><p className="mt-2 text-sm text-[var(--muted)]">{result.analysis.method} · {result.analysis.files_scanned} files · {result.analysis.bytes_scanned.toLocaleString()} bytes. {result.analysis.reason}</p></Card>
    <section className="space-y-3"><h2 className="text-lg font-semibold">Source evidence <span className="text-sm font-normal text-[var(--muted)]">({result.results.length})</span></h2>{result.results.map((item,index)=><Card key={`${item.path}:${item.line}:${index}`} className="p-4"><div className="flex flex-wrap items-center gap-2"><Badge>{item.kind}</Badge><Link className="font-mono text-sm font-semibold text-[var(--brand)] hover:underline" href={`/repositories/${id}?ref=${result.revision}&path=${encodeURIComponent(item.path)}&line=${item.line}#L${item.line}`}>{item.path}:{item.line}</Link></div><pre className="mt-2 overflow-x-auto whitespace-pre-wrap text-xs">{item.preview}</pre>{item.commit_id&&<p className="mt-2 text-xs text-[var(--muted)]">Last changed in <code>{item.commit_id.slice(0,7)}</code>{item.commit_summary&&` · ${item.commit_summary}`}</p>}</Card>)}{result.results.length===0&&<Card className="p-5 text-sm text-[var(--muted)]">No matching source evidence at this revision.</Card>}</section>
    <div className="grid gap-5 lg:grid-cols-2"><Card className="p-5"><h2 className="font-semibold">Who knows this repository</h2><ul className="mt-3 space-y-2 text-sm">{result.ownership.map(owner=><li key={`${owner.kind}:${owner.id}`}><Badge>{owner.kind.replaceAll("_"," ")}</Badge> <code>{owner.id}</code></li>)}</ul></Card><Card className="p-5"><h2 className="font-semibold">Declared dependencies at this commit</h2>{result.dependencies.length?<ul className="mt-3 space-y-3 text-sm">{result.dependencies.map(dep=><li key={dep.id}><Link href={`/repositories/${dep.provider_repository_id}/relationships`} className="font-semibold text-[var(--brand)] hover:underline">{dep.interface_name}</Link> <code>{dep.constraint}</code></li>)}</ul>:<p className="mt-3 text-sm text-[var(--muted)]">No readable declarations are pinned to this commit.</p>}</Card></div></>}
  </div>;
}
