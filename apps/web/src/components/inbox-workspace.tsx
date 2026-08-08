"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AccessGate, useAuth } from "@/components/auth";
import { Icons } from "@/components/icons";
import { Avatar, Badge, Button, Card } from "@/components/ui";
import { api, type InboxItem, type User } from "@/lib/api";

type Filter = "all" | InboxItem["category"];
const filters: { value: Filter; label: string }[] = [
  { value: "all", label: "All" }, { value: "review", label: "Review" },
  { value: "response", label: "Response" }, { value: "awareness", label: "Awareness" },
];
const categoryCopy = {
  review: "A decision is waiting on you",
  response: "The conversation needs your response",
  awareness: "A relevant outcome is ready to acknowledge",
};

function href(item: InboxItem) {
  if (item.resource_type === "proposal") return `/proposals/${item.repository_id}/${item.resource_id}`;
  if (item.resource_type === "pull_request") return `/pulls/${item.repository_id}/${item.resource_id}`;
  if (item.resource_type === "incident") return `/incidents/${item.resource_id}`;
  return `/repositories/${item.repository_id}`;
}
function initials(user?: User) { return (user?.display_name ?? "Unknown collaborator").split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase(); }
function relativeTime(value: string) {
  const seconds = Math.round((new Date(value).getTime() - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  if (Math.abs(seconds) < 60) return formatter.format(seconds, "second");
  const minutes = Math.round(seconds / 60); if (Math.abs(minutes) < 60) return formatter.format(minutes, "minute");
  const hours = Math.round(minutes / 60); if (Math.abs(hours) < 24) return formatter.format(hours, "hour");
  return formatter.format(Math.round(hours / 24), "day");
}

export function InboxWorkspace() {
  const { token } = useAuth();
  const [items, setItems] = useState<InboxItem[]>([]);
  const [people, setPeople] = useState<Record<string, User>>({});
  const [filter, setFilter] = useState<Filter>("all");
  const [loading, setLoading] = useState(true);
  const [clearing, setClearing] = useState<string | null>(null);
  const [error, setError] = useState("");
  const generation = useRef(0);
  const load = useCallback(async () => {
    const current = ++generation.current;
    if (!token) { setItems([]); setLoading(false); return; }
    setLoading(true); setError("");
    try {
      const loaded: InboxItem[] = []; let cursor: string | null = null;
      do {
        const result: { items: InboxItem[]; next_cursor: string | null } = await api(`/inbox?limit=100${cursor ? `&after=${cursor}` : ""}`, {}, token);
        loaded.push(...result.items); cursor = result.next_cursor;
      } while (cursor);
      const ids = [...new Set(loaded.map((item) => item.actor_id))];
      const resolved = await Promise.all(ids.map(async (id) => [id, await api<User>(`/users/${id}`)] as const));
      if (current === generation.current) { setItems(loaded); setPeople(Object.fromEntries(resolved)); }
    } catch (reason) { if (current === generation.current) setError(reason instanceof Error ? reason.message : "Inbox could not be loaded."); }
    finally { if (current === generation.current) setLoading(false); }
  }, [token]);
  useEffect(() => { void Promise.resolve().then(load); return () => { generation.current += 1; }; }, [load]);
  const visible = useMemo(() => filter === "all" ? items : items.filter((item) => item.category === filter), [filter, items]);
  async function clear(item: InboxItem) {
    if (!token || clearing) return;
    setClearing(item.id); setError("");
    try { await api(`/inbox/${item.id}`, { method: "DELETE" }, token); setItems((current) => current.filter((candidate) => candidate.id !== item.id)); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "The item could not be cleared."); }
    finally { setClearing(null); }
  }
  return <AccessGate><div className="space-y-7">
    <header className="flex items-start gap-4"><span className="grid size-11 place-items-center rounded-xl bg-[var(--brand-soft)] text-[var(--brand)]"><Icons.Bell /></span><div><p className="text-xs font-bold uppercase tracking-[.16em] text-[var(--brand)]">Needs your attention</p><h1 className="mt-1 text-3xl font-semibold tracking-tight">Inbox</h1><p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--muted)]">Decisions, responses, and outcomes collected from the work you share.</p></div></header>
    <div className="flex flex-wrap gap-2" role="group" aria-label="Filter inbox"><span className="sr-only" aria-live="polite">{visible.length} items shown</span>{filters.map(({ value, label }) => { const count = value === "all" ? items.length : items.filter((item) => item.category === value).length; return <button key={value} type="button" onClick={() => setFilter(value)} aria-pressed={filter === value} className={`rounded-full border px-3 py-1.5 text-sm font-semibold transition ${filter === value ? "border-[var(--brand)] bg-[var(--brand-soft)] text-[var(--brand-strong)]" : "border-[var(--line)] bg-white text-[var(--muted)] hover:border-[var(--line-strong)]"}`}>{label} <span className="ml-1 tabular-nums">{count}</span></button>; })}</div>
    {error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}
    {loading ? <Card className="p-8 text-sm text-[var(--muted)]">Loading your inbox…</Card> : !visible.length ? <Card className="p-10 text-center"><h2 className="font-semibold">{items.length ? `No ${filter} items` : "You’re all caught up"}</h2><p className="mt-2 text-sm text-[var(--muted)]">{items.length ? "Choose another classification to see the rest of your inbox." : "New review requests, responses, and relevant outcomes will appear here."}</p></Card> : <Card className="divide-y divide-[var(--line)] overflow-hidden">{visible.map((item) => { const actor = people[item.actor_id]; return <article key={item.id} className="flex flex-col gap-4 p-4 sm:flex-row sm:items-center sm:p-5"><Avatar size="sm" initials={initials(actor)} label={actor?.display_name ?? "Unknown collaborator"}/><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><Badge tone={item.category === "review" ? "warning" : item.category === "response" ? "info" : "neutral"}>{item.category}</Badge><span className="text-xs text-[var(--muted)]">{categoryCopy[item.category]}</span></div><Link href={href(item)} className="mt-2 block truncate font-semibold hover:text-[var(--brand)] hover:underline">{item.resource_title}</Link><p className="mt-1 text-sm text-[var(--muted)]"><span className="font-medium text-[var(--ink)]">{actor?.display_name ?? "A collaborator"}</span> · {item.repository_name} · <time dateTime={item.created_at}>{relativeTime(item.created_at)}</time></p></div><div className="flex shrink-0 items-center gap-2"><Link href={href(item)} className="inline-flex min-h-9 items-center rounded-lg bg-[var(--brand)] px-3.5 text-sm font-semibold text-white hover:bg-[var(--brand-strong)]">{item.action}</Link><Button variant="quiet" disabled={clearing === item.id} onClick={() => void clear(item)} aria-label={`Clear ${item.resource_title}`}>{clearing === item.id ? "Clearing…" : "Clear"}</Button></div></article>; })}</Card>}
  </div></AccessGate>;
}
