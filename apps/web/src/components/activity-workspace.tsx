"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AccessGate, useAuth } from "@/components/auth";
import { Icons } from "@/components/icons";
import { Avatar, Badge, Card } from "@/components/ui";
import { api, type ActivityEvent, type User } from "@/lib/api";

const descriptions: Record<ActivityEvent["kind"], string> = {
  "proposal.created": "opened a proposal",
  "proposal.updated": "updated a proposal",
  "proposal.closed": "closed a proposal",
  "proposal.commented": "commented on a proposal",
  "pull_request.created": "opened a pull request",
  "pull_request.synchronized": "synchronized a pull request",
  "pull_request.commented": "commented on a pull request",
  "pull_request.merged": "merged a pull request",
  "review.approved": "approved a pull request",
  "review.changes_requested": "requested changes on a pull request",
  "review.withdrawn": "withdrew a pull request review",
  "mention.created": "mentioned a collaborator",
  "access.granted": "granted contributor access",
  "access.revoked": "revoked contributor access",
  "deployment.pause": "paused a deployment rollout",
  "deployment.resume": "resumed a deployment rollout",
  "deployment.cancel": "canceled a deployment rollout",
  "deployment.mark_unsuccessful": "marked a deployment unsuccessful",
};

function initials(user?: User) {
  return (user?.display_name ?? "Unknown collaborator").split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase();
}

function resourceHref(event: ActivityEvent) {
  if (event.resource_type === "proposal") return `/proposals/${event.repository_id}/${event.resource_id}`;
  if (event.resource_type === "pull_request") return `/pulls/${event.repository_id}/${event.resource_id}`;
  return `/repositories/${event.repository_id}`;
}

function relativeTime(value: string) {
  const seconds = Math.round((new Date(value).getTime() - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  if (Math.abs(seconds) < 60) return formatter.format(seconds, "second");
  const minutes = Math.round(seconds / 60);
  if (Math.abs(minutes) < 60) return formatter.format(minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return formatter.format(hours, "hour");
  return formatter.format(Math.round(hours / 24), "day");
}

export function ActivityWorkspace() {
  const { token } = useAuth();
  const [events, setEvents] = useState<ActivityEvent[]>([]);
  const [people, setPeople] = useState<Record<string, User>>({});
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const generation = useRef(0);
  const load = useCallback(async () => {
    const current = ++generation.current;
    if (!token) { setEvents([]); setLoading(false); return; }
    setLoading(true); setError("");
    try {
      const loaded: ActivityEvent[] = [];
      let cursor: string | null = null;
      do {
        const result: { events: ActivityEvent[]; next_cursor: string | null } = await api<{ events: ActivityEvent[]; next_cursor: string | null }>(`/activity?limit=100${cursor ? `&after=${cursor}` : ""}`, {}, token);
        loaded.push(...result.events); cursor = result.next_cursor;
      } while (cursor);
      const ids = [...new Set(loaded.flatMap((event) => [event.actor_id, ...(event.target_user_id ? [event.target_user_id] : [])]))];
      const resolved = await Promise.all(ids.map(async (id) => [id, await api<User>(`/users/${id}`)] as const));
      if (current === generation.current) { setEvents(loaded); setPeople(Object.fromEntries(resolved)); }
    } catch (reason) {
      if (current === generation.current) setError(reason instanceof Error ? reason.message : "Activity could not be loaded.");
    } finally { if (current === generation.current) setLoading(false); }
  }, [token]);
  useEffect(() => { void Promise.resolve().then(load); return () => { generation.current += 1; }; }, [load]);
  const grouped = useMemo(() => events.reduce<Record<string, ActivityEvent[]>>((groups, event) => {
    const day = new Intl.DateTimeFormat(undefined, { dateStyle: "long" }).format(new Date(event.created_at));
    (groups[day] ??= []).push(event); return groups;
  }, {}), [events]);

  return <AccessGate><div className="space-y-7">
    <header className="flex items-start gap-4"><span className="grid size-11 place-items-center rounded-xl bg-[var(--brand-soft)] text-[var(--brand)]"><Icons.Activity /></span><div><p className="text-xs font-bold uppercase tracking-[.16em] text-[var(--brand)]">Across your work</p><h1 className="mt-1 text-3xl font-semibold tracking-tight">Activity</h1><p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--muted)]">Meaningful shared-state changes from repositories you collaborate on, newest first.</p></div></header>
    {error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}
    {loading ? <Card className="p-8 text-sm text-[var(--muted)]">Loading activity…</Card> : !events.length ? <Card className="p-10 text-center"><h2 className="font-semibold">No collaboration activity yet</h2><p className="mt-2 text-sm text-[var(--muted)]">Proposal, review, merge, mention, and access changes will collect here.</p></Card> : Object.entries(grouped).map(([day, items]) => <section key={day} aria-labelledby={`day-${items[0].id}`}><h2 id={`day-${items[0].id}`} className="mb-3 text-sm font-semibold text-[var(--muted)]">{day}</h2><Card className="divide-y divide-[var(--line)] overflow-hidden">{items.map((event) => { const actor = people[event.actor_id]; const target = event.target_user_id ? people[event.target_user_id] : undefined; return <article key={event.id} className="flex gap-3 p-4 sm:p-5"><Avatar size="sm" initials={initials(actor)} label={actor?.display_name ?? "Unknown collaborator"} /><div className="min-w-0 flex-1"><p className="text-sm"><span className="font-semibold">{actor?.display_name ?? "Unknown collaborator"}</span> <span className="text-[var(--muted)]">{descriptions[event.kind]}</span>{target && event.kind.startsWith("access.") ? <> <span className="font-semibold">{target.display_name}</span></> : null}</p><Link href={resourceHref(event)} className="mt-1 block truncate font-semibold text-[var(--ink)] hover:text-[var(--brand)] hover:underline">{event.resource_title}</Link><div className="mt-2 flex flex-wrap items-center gap-2"><Badge tone={event.kind === "pull_request.merged" ? "success" : event.kind === "mention.created" ? "info" : "neutral"}>{event.repository_name}</Badge><time dateTime={event.created_at} title={new Date(event.created_at).toLocaleString()} className="text-xs text-[var(--muted)]">{relativeTime(event.created_at)}</time></div></div></article>; })}</Card></section>)}
  </div></AccessGate>;
}
