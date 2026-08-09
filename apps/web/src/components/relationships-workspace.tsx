"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, type Branch, type DeploymentEnvironment, type RelationshipGraph, type ReleaseCandidate } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

export function RelationshipsWorkspace({ repositoryID }: { repositoryID: string }) {
  const { token, loading: authLoading } = useAuth();
  const [graph, setGraph] = useState<RelationshipGraph | null>(null);
  const [releases, setReleases] = useState<ReleaseCandidate[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [environments, setEnvironments] = useState<DeploymentEnvironment[]>([]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const load = useCallback(async () => {
    if (authLoading) return;
    try {
      const [nextGraph, releaseSet, branchSet, environmentSet] = await Promise.all([
        api<RelationshipGraph>(`/repositories/${repositoryID}/relationships`, {}, token),
        api<{ releases: ReleaseCandidate[] }>(`/repositories/${repositoryID}/releases`, {}, token),
        api<{ branches: Branch[] }>(`/repositories/${repositoryID}/branches`, {}, token),
        api<{ environments: DeploymentEnvironment[] }>(`/repositories/${repositoryID}/environments`, {}, token),
      ]);
      setGraph(nextGraph); setReleases(releaseSet.releases); setBranches(branchSet.branches); setEnvironments(environmentSet.environments); setError("");
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Relationship evidence could not be loaded."); }
  }, [authLoading, repositoryID, token]);
  useEffect(() => { void Promise.resolve().then(load); }, [load]);
  async function publish(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!token) return; setPending(true); const form=event.currentTarget; const data=new FormData(form);
    try { await api(`/repositories/${repositoryID}/interfaces`, {method:"POST",body:JSON.stringify({name:data.get("name"),release_id:data.get("release_id")})}, token); form.reset(); await load(); }
    catch(reason){setError(reason instanceof Error?reason.message:"Interface could not be published.");} finally{setPending(false)}
  }
  async function declare(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!token) return; setPending(true); const form=event.currentTarget; const data=new FormData(form);
    try { await api(`/repositories/${repositoryID}/dependencies`, {method:"POST",body:JSON.stringify({commit_id:data.get("commit_id"),release_id:data.get("release_id")||undefined,environment_id:data.get("environment_id")||undefined,provider_repository_id:data.get("provider_repository_id"),interface_name:data.get("interface_name"),constraint:data.get("constraint")})}, token); form.reset(); await load(); }
    catch(reason){setError(reason instanceof Error?reason.message:"Dependency could not be declared.");} finally{setPending(false)}
  }
  const names = Object.fromEntries((graph?.repositories ?? []).map((repository) => [repository.id, repository.name]));
  return <div className="space-y-6">
    <div><Link href={`/repositories/${repositoryID}`} className="text-sm font-semibold text-[var(--brand)] hover:underline">← Repository</Link><p className="mt-5 font-mono text-xs font-semibold uppercase tracking-[.14em] text-[var(--brand)]">Cross-repository evidence</p><h1 className="mt-1 text-3xl font-semibold">Interface dependency graph</h1><p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">Published contracts and consumer claims are pinned to exact releases and revisions. Current platform records determine whether each edge remains trustworthy.</p></div>
    {error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}
    {token && <div className="grid gap-4 lg:grid-cols-2">
      <Card className="p-5"><h2 className="font-semibold">Publish an interface</h2><p className="mt-1 text-sm text-[var(--muted)]">The selected release supplies both version and exact source revision.</p><form onSubmit={publish} className="mt-4 grid gap-3"><input name="name" required maxLength={100} placeholder="Interface name" className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 text-sm"/><select name="release_id" required className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"><option value="">Select a release</option>{releases.map((release)=><option key={release.id} value={release.id}>{release.version} · {release.commit_id.slice(0,12)}</option>)}</select><Button type="submit" disabled={pending}>Publish interface</Button></form></Card>
      <Card className="p-5"><h2 className="font-semibold">Declare a dependency</h2><p className="mt-1 text-sm text-[var(--muted)]">Constraints use semantic versions, for example <code>&gt;=v1.0.0 &lt;v2.0.0</code>.</p><form onSubmit={declare} className="mt-4 grid gap-3"><input name="provider_repository_id" required pattern="[0-9a-f]{32}" placeholder="Provider repository ID" className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 font-mono text-sm"/><div className="grid gap-3 sm:grid-cols-2"><input name="interface_name" required placeholder="Interface name" className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 text-sm"/><input name="constraint" required placeholder=">=v1.0.0 <v2.0.0" className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 font-mono text-sm"/></div><select name="commit_id" required className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"><option value="">Exact consumer revision</option>{branches.map((branch)=><option key={branch.name} value={branch.commit_id}>{branch.name} · {branch.commit_id.slice(0,12)}</option>)}</select><div className="grid gap-3 sm:grid-cols-2"><select name="release_id" className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"><option value="">No release evidence</option>{releases.map((release)=><option key={release.id} value={release.id}>{release.version}</option>)}</select><select name="environment_id" className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"><option value="">No environment evidence</option>{environments.map((environment)=><option key={environment.id} value={environment.id}>{environment.name}</option>)}</select></div><Button type="submit" disabled={pending}>Declare dependency</Button></form></Card>
    </div>}
    <section><h2 className="text-lg font-semibold">Evidence graph</h2><div className="mt-3 grid gap-3">{!graph ? <Card className="p-6 text-sm text-[var(--muted)]">Loading relationships…</Card> : graph.dependencies.length===0 ? <Card className="p-6 text-sm text-[var(--muted)]">No dependencies have been declared.</Card> : graph.dependencies.map((edge)=><Card key={edge.id} className="p-5"><div className="flex flex-wrap items-center gap-2"><Link href={`/repositories/${edge.repository_id}`} className="font-mono text-sm font-semibold text-[var(--brand)] hover:underline">{names[edge.repository_id]??edge.repository_id}</Link><span aria-hidden>→</span><Link href={`/repositories/${edge.provider_repository_id}`} className="font-mono text-sm font-semibold text-[var(--brand)] hover:underline">{names[edge.provider_repository_id]??edge.provider_repository_id}</Link><Badge tone={edge.state==="resolved"?"success":edge.state==="stale"?"warning":"danger"}>{edge.state}</Badge></div><p className="mt-2 text-sm"><strong>{edge.interface_name}</strong> <code>{edge.constraint}</code>{edge.resolved_version&&<> resolves to <code>{edge.resolved_version}</code></>}</p>{edge.reason&&<p className="mt-2 text-sm text-[var(--danger)]">{edge.reason}</p>}<p className="mt-3 break-all font-mono text-xs text-[var(--muted)]">consumer {edge.commit_id} · owner {graph.repositories.find((repository)=>repository.id===edge.repository_id)?.owner_id}</p></Card>)}</div></section>
    {(graph?.interfaces.length??0)>0&&<section><h2 className="text-lg font-semibold">Published interfaces</h2><div className="mt-3 grid gap-3 sm:grid-cols-2">{graph?.interfaces.map((item)=><Card key={item.id} className="p-4"><div className="flex items-center gap-2"><strong>{item.name}</strong><Badge tone={item.stale?"warning":"success"}>{item.stale?"stale":item.version}</Badge></div><p className="mt-2 font-mono text-xs text-[var(--muted)]">{names[item.repository_id]} · {item.commit_id}</p>{item.stale_reason&&<p className="mt-2 text-sm text-[var(--danger)]">{item.stale_reason}</p>}</Card>)}</div></section>}
  </div>;
}
