"use client";

import { useCallback, useEffect, useState } from "react";
import { api, type PullPreview } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

export function PullRequestPreviews({ repositoryID, pullRequestID, participant }: { repositoryID: string; pullRequestID: string; participant: boolean }) {
  const { token } = useAuth();
  const [items, setItems] = useState<PullPreview[]>([]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const base = `/repositories/${repositoryID}/pulls/${pullRequestID}/previews`;
  const load = useCallback(async () => { try { setItems((await api<{ previews: PullPreview[] }>(base, {}, token)).previews); setError(""); } catch (reason) { setError(reason instanceof Error ? reason.message : "Previews could not be loaded."); } }, [base, token]);
  useEffect(() => { void load(); const timer=window.setInterval(()=>void load(),3000); return()=>window.clearInterval(timer); },[load]);
  async function launch() { setPending(true); try { await api(base,{method:"POST"},token); await load(); } catch(reason) { setError(reason instanceof Error ? reason.message : "Preview could not be launched."); } finally { setPending(false); } }
  return <section id="previews" className="scroll-mt-24 space-y-3">
    <div className="flex items-baseline justify-between gap-3"><div><h2 className="text-lg font-semibold">Change previews</h2><p className="text-xs text-[var(--muted)]">Isolated experiences pinned to one review revision</p></div>{participant&&<Button disabled={pending} onClick={()=>void launch()}>{pending?"Launching…":"Launch preview"}</Button>}</div>
    {error&&<p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}
    {items.length===0?<Card className="p-5 text-sm text-[var(--muted)]">No preview has been published. Add a version 1 <code>.vivarium/preview.json</code> to the candidate revision to opt in.</Card>:items.map(item=><Card key={item.id} className="p-5"><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="font-semibold">Preview <code>{item.id.slice(0,8)}</code></p><p className="mt-1 text-xs text-[var(--muted)]">revision <code>{item.revision.slice(0,12)}</code> · created by {item.creator_id} · {new Date(item.created_at).toLocaleString()}</p></div><div className="flex gap-2"><Badge tone={item.stale?"warning":item.state==="succeeded"?"success":item.state==="failed"?"danger":"neutral"}>{item.stale?"stale":item.state}</Badge></div></div><p className="mt-3 text-xs">Definition <code>{item.definition_sha256.slice(0,12)}</code> · {item.definition.resources.cpus} CPU · {item.definition.resources.memory_mb} MiB memory · {item.definition.resources.storage_mb} MiB output</p><div className="mt-4 flex gap-3 text-sm"><a className="font-semibold text-[var(--accent)]" href={item.url} target="_blank" rel="noreferrer">Open exact preview</a><a className="text-[var(--muted)] underline" href={`${base}/${item.id}/events`} target="_blank" rel="noreferrer">Build logs</a></div>{item.stale&&<p className="mt-3 text-xs text-[var(--warning)]">The pull request has moved. This retained URL still represents the revision participants evaluated.</p>}</Card>)}
  </section>;
}
