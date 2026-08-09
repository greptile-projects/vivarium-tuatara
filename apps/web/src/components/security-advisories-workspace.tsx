"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, type Repository, type SecurityAdvisory, type User } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

const field = "mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 outline-none focus:border-[var(--brand)]";
const errorMessage = (reason: unknown) => reason instanceof Error ? reason.message : "The private security operation failed.";
const stamp = (value: string) => new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));

export function SecurityAdvisoriesWorkspace({ advisoryId }: { advisoryId?: string }) {
  const { token, user, loading } = useAuth();
  const [items, setItems] = useState<SecurityAdvisory[]>([]);
  const [advisory, setAdvisory] = useState<SecurityAdvisory>();
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [people, setPeople] = useState<Record<string, User>>({});
  const [showReport, setShowReport] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const repos = await api<{ repositories: Repository[] }>("/repositories?limit=100", {}, token);
      setRepositories(repos.repositories);
      if (advisoryId) {
        const found = await api<SecurityAdvisory>(`/security-advisories/${advisoryId}`, {}, token);
        setAdvisory(found);
        const ids = [...new Set([found.reporter_id, ...found.response_team, ...found.messages.map((x) => x.actor_id), ...found.access_log.map((x) => x.actor_id)])];
        const users = await Promise.all(ids.map(async (id) => [id, await api<User>(`/users/${id}`, {}, token).catch(() => undefined)] as const));
        setPeople(Object.fromEntries(users.filter((x): x is readonly [string, User] => Boolean(x[1]))));
      } else {
        const found = await api<{ security_advisories: SecurityAdvisory[] }>("/security-advisories?limit=100", {}, token);
        setItems(found.security_advisories);
      }
    } catch (reason) { setError(errorMessage(reason)); }
  }, [advisoryId, token]);
  useEffect(() => { void Promise.resolve().then(load); }, [load]);
  const mutate = async (path: string, body: object, method = "POST") => {
    if (!token) return;
    setPending(true); setError("");
    try { setAdvisory(await api<SecurityAdvisory>(path, { method, body: JSON.stringify(body) }, token)); }
    catch (reason) { setError(errorMessage(reason)); }
    finally { setPending(false); }
  };
  async function report(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!token) return;
    const data = new FormData(event.currentTarget);
    const repositoryId = String(data.get("repository_id"));
    setPending(true); setError("");
    try {
      const created = await api<SecurityAdvisory>("/security-advisories", { method: "POST", body: JSON.stringify({
        title: data.get("title"), description: data.get("description"), contact: data.get("contact"),
        affected_repositories: [{ repository_id: repositoryId, versions: String(data.get("versions")).split(",").map((x) => x.trim()).filter(Boolean) }],
        evidence: [{ label: data.get("evidence_label"), description: data.get("evidence_description") }],
      }) }, token);
      window.location.assign(`/security/${created.id}`);
    } catch (reason) { setError(errorMessage(reason)); setPending(false); }
  }
  if (loading) return <Card className="p-8 text-sm text-[var(--muted)]">Opening protected workspace…</Card>;
  if (!user) return <Card className="p-8 text-center"><h1 className="text-2xl font-semibold">Report a vulnerability privately</h1><p className="mt-2 text-sm text-[var(--muted)]">Sign in so maintainers can respond through a protected, attributable channel.</p><Link href="/?access=signin" className="mt-5 inline-flex rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-semibold text-white">Sign in</Link></Card>;
  const repositoryName = (id: string) => repositories.find((x) => x.id === id)?.name ?? id;
  const actorName = (id: string) => people[id]?.display_name || people[id]?.handle || id;
  const isMaintainer = advisory?.affected_repositories.some((x) => repositories.find((repo) => repo.id === x.repository_id)?.owner_id === user.id);
  if (advisoryId && advisory) return <div className="space-y-6">
    <header><Link href="/security" className="text-sm font-semibold text-[var(--brand-strong)]">← Private security reports</Link><div className="mt-4 flex flex-wrap items-center gap-2"><Badge tone={advisory.severity === "critical" || advisory.severity === "high" ? "danger" : "warning"}>{advisory.severity}</Badge><Badge>{advisory.embargo_state}</Badge></div><h1 className="mt-3 text-3xl font-semibold">{advisory.title}</h1><p className="mt-2 max-w-3xl whitespace-pre-wrap text-sm text-[var(--muted)]">{advisory.description}</p></header>
    {error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}
    <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_22rem]">
      <div className="space-y-5"><Card className="p-5"><h2 className="font-semibold">Affected versions</h2>{advisory.affected_repositories.map((x) => <div key={x.repository_id} className="mt-3 rounded-lg bg-[var(--canvas)] p-3 text-sm"><span className="font-semibold">{repositoryName(x.repository_id)}</span><span className="ml-2 text-[var(--muted)]">{x.versions.join(", ")}</span></div>)}<h3 className="mt-5 font-semibold">Reporter evidence</h3>{advisory.evidence.map((x, index) => <div key={`${x.label}-${index}`} className="mt-3"><p className="text-sm font-semibold">{x.label}</p><p className="mt-1 whitespace-pre-wrap text-sm text-[var(--muted)]">{x.description}</p></div>)}</Card>
      <Card className="p-5"><h2 className="font-semibold">Protected conversation</h2><div className="mt-4 space-y-3">{advisory.messages.length === 0 && <p className="text-sm text-[var(--muted)]">No messages yet.</p>}{advisory.messages.map((message) => <div key={message.id} className="rounded-lg bg-[var(--canvas)] p-3"><p className="text-xs font-semibold">{actorName(message.actor_id)} · {stamp(message.created_at)}</p><p className="mt-2 whitespace-pre-wrap text-sm">{message.body}</p></div>)}</div><form className="mt-4" onSubmit={(event) => { event.preventDefault(); const form = event.currentTarget; const body = new FormData(form).get("body"); void mutate(`/security-advisories/${advisory.id}/messages`, { body }).then(() => form.reset()); }}><textarea className={`${field} py-3`} name="body" rows={3} required maxLength={20000} aria-label="Message"/><Button className="mt-3" disabled={pending}>Send privately</Button></form></Card></div>
      <aside className="space-y-5"><Card className="p-5"><h2 className="font-semibold">Safe contact</h2><p className="mt-2 break-words text-sm text-[var(--muted)]">{advisory.contact}</p><p className="mt-4 text-xs text-[var(--muted)]">Reporter: {actorName(advisory.reporter_id)}</p></Card>
      {isMaintainer && <Card className="p-5"><h2 className="font-semibold">Maintainer triage</h2><form className="mt-3 space-y-3" onSubmit={(event) => { event.preventDefault(); const data = new FormData(event.currentTarget); void mutate(`/security-advisories/${advisory.id}`, { expected_version: advisory.version, severity: data.get("severity"), embargo_state: data.get("embargo_state") }, "PATCH"); }}><select className={field} name="severity" defaultValue={advisory.severity === "untriaged" ? "high" : advisory.severity}><option value="low">Low</option><option value="moderate">Moderate</option><option value="high">High</option><option value="critical">Critical</option></select><select className={field} name="embargo_state" defaultValue={advisory.embargo_state}><option value="reported">Reported</option><option value="triaging">Triaging</option><option value="embargoed">Embargoed</option><option value="coordinating">Coordinating</option></select><Button disabled={pending}>Update triage</Button></form><form className="mt-6" onSubmit={(event) => { event.preventDefault(); const form = event.currentTarget; const userId = new FormData(form).get("user_id"); void mutate(`/security-advisories/${advisory.id}/responders`, { user_id: userId }).then(() => form.reset()); }}><label className="text-sm font-semibold">Invite responder by collaboration ID<input className={field} name="user_id" required minLength={32} maxLength={32}/></label><Button className="mt-3" disabled={pending}>Invite to response team</Button></form></Card>}
      <Card className="p-5"><h2 className="font-semibold">Access audit</h2><div className="mt-3 max-h-80 space-y-2 overflow-auto">{[...advisory.access_log].reverse().map((event) => <p key={event.id} className="text-xs text-[var(--muted)]"><span className="font-semibold text-[var(--ink)]">{actorName(event.actor_id)}</span> {event.action.replaceAll("_", " ")} · {stamp(event.created_at)}</p>)}</div></Card></aside>
    </div></div>;
  return <div className="space-y-7"><header className="flex flex-wrap items-end justify-between gap-4"><div><p className="font-mono text-xs font-semibold uppercase tracking-[.16em] text-[var(--danger)]">Protected collaboration</p><h1 className="mt-2 text-3xl font-semibold">Private security reports</h1><p className="mt-2 max-w-2xl text-sm text-[var(--muted)]">Share suspected vulnerabilities and coordinate an embargoed response without publishing repository activity or notifications.</p></div><Button onClick={() => setShowReport((x) => !x)}>{showReport ? "Cancel" : "Report vulnerability"}</Button></header>
  {error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}
  {showReport && <Card className="p-6"><h2 className="text-lg font-semibold">Protected report</h2><form onSubmit={report} className="mt-5 grid gap-4"><label className="text-sm font-semibold">Title<input className={field} name="title" required maxLength={200}/></label><label className="text-sm font-semibold">Suspected vulnerability<textarea className={`${field} py-3`} name="description" rows={5} required maxLength={20000}/></label><label className="text-sm font-semibold">Affected repository<select className={field} name="repository_id" required><option value="">Select a repository</option>{repositories.map((repo) => <option key={repo.id} value={repo.id}>{repo.name}</option>)}</select></label><label className="text-sm font-semibold">Affected versions<input className={field} name="versions" required placeholder="1.4.x, 2.0.0"/></label><label className="text-sm font-semibold">Evidence label<input className={field} name="evidence_label" required placeholder="Reproduction notes"/></label><label className="text-sm font-semibold">Evidence (avoid live secrets)<textarea className={`${field} py-3`} name="evidence_description" rows={4} required maxLength={10000}/></label><label className="text-sm font-semibold">Safe contact channel<input className={field} name="contact" required maxLength={500} placeholder="Encrypted email, Signal handle, or monitored address"/></label><p className="text-xs text-[var(--muted)]">Only you, affected repository owners, and responders they explicitly invite can discover this report.</p><Button disabled={pending}>Submit protected report</Button></form></Card>}
  <div className="grid gap-3">{items.length === 0 && <Card className="p-8 text-center text-sm text-[var(--muted)]">No private reports are available to you.</Card>}{items.map((item) => <Link key={item.id} href={`/security/${item.id}`}><Card className="p-5 transition hover:border-[var(--line-strong)]"><div className="flex flex-wrap items-center gap-2"><Badge tone={item.severity === "critical" || item.severity === "high" ? "danger" : "warning"}>{item.severity}</Badge><Badge>{item.embargo_state}</Badge></div><h2 className="mt-3 font-semibold">{item.title}</h2><p className="mt-1 text-sm text-[var(--muted)]">{item.affected_repositories.map((x) => repositoryName(x.repository_id)).join(", ")} · updated {stamp(item.updated_at)}</p></Card></Link>)}</div></div>;
}
