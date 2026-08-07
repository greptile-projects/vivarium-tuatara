"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { api, type Credential, type User } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

export function AccountSettings() {
  const { user, token, setSession, signOut } = useAuth();
  const router = useRouter();
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [issued, setIssued] = useState<Credential | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [canManage, setCanManage] = useState(true);

  const load = useCallback(async () => {
    if (!token) return;
    try { const result = await api<{ credentials: Credential[] }>("/auth/credentials?limit=100", {}, token); setCredentials(result.credentials); }
    catch { setCanManage(false); }
  }, [token]);
  useEffect(() => { void Promise.resolve().then(load); }, [load]);

  async function updateProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!user || !token) return; setError(""); setMessage("");
    const data = new FormData(event.currentTarget);
    try { const updated = await api<User>(`/users/${user.id}`, { method: "PATCH", body: JSON.stringify({ display_name: data.get("display_name"), handle: data.get("handle") }) }, token); setSession(token, updated); setMessage("Profile updated."); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Profile could not be updated."); }
  }

  async function createCredential(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(""); setIssued(null);
    const form = event.currentTarget; const data = new FormData(form); const kind = String(data.get("kind"));
    const scopes = kind === "git" ? ["git:read", "git:write"] : ["profile:write", "repositories:read", "repositories:write"];
    const expires_in = kind === "git" ? 30 * 86400 : 90 * 86400;
    try { const created = await api<Credential>("/auth/credentials", { method: "POST", body: JSON.stringify({ kind, name: data.get("name"), scopes, expires_in }) }, token); setIssued(created); await load(); form.reset(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Credential could not be created."); }
  }

  async function revoke(id: string) {
    setError("");
    try { await api<void>(`/auth/credentials/${id}`, { method: "DELETE" }, token); setCredentials((items) => items.map((item) => item.id === id ? { ...item, revoked_at: new Date().toISOString() } : item)); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Credential could not be revoked."); }
  }

  return <div className="grid gap-7 xl:grid-cols-2">
    <Card className="p-6"><h2 className="text-lg font-semibold">Profile</h2><p className="mt-1 text-sm text-[var(--muted)]">The identity collaborators see and attribution keeps.</p><form onSubmit={updateProfile} className="mt-5 space-y-4"><label className="block text-sm font-semibold">Display name<input name="display_name" defaultValue={user?.display_name} required className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] px-3 font-normal"/></label><label className="block text-sm font-semibold">Handle<input name="handle" defaultValue={user?.handle} required pattern="[A-Za-z0-9-]{1,39}" className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] px-3 font-mono font-normal"/></label><Button type="submit">Save profile</Button></form></Card>
    <Card className="p-6"><h2 className="text-lg font-semibold">Create access</h2><p className="mt-1 text-sm leading-6 text-[var(--muted)]">Issue an API token for integrations or a Git token for stock clients.</p>{canManage ? <form onSubmit={createCredential} className="mt-5 space-y-4"><label className="block text-sm font-semibold">Name<input name="name" required placeholder="Work laptop" className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] px-3 font-normal"/></label><label className="block text-sm font-semibold">Kind<select name="kind" className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal"><option value="git">Git · clone and push</option><option value="api">API · profile and repositories</option></select></label><Button type="submit">Create token</Button></form> : <p className="mt-5 rounded-lg bg-[var(--warning-soft)] p-3 text-sm text-[var(--warning)]">Credential management requires a session token. Sign in with the session issued at account creation.</p>}{issued?.token && <div className="mt-5 rounded-lg border border-[var(--brand)] bg-[var(--brand-soft)] p-4"><p className="text-sm font-semibold">Copy this token now</p><p className="mt-1 text-xs text-[var(--muted)]">It won’t be shown again.</p><code className="mt-3 block break-all rounded bg-white p-3 text-xs select-all">{issued.token}</code></div>}</Card>
    <Card className="p-6 xl:col-span-2"><div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-start"><div><h2 className="text-lg font-semibold">Active access</h2><p className="mt-1 text-sm text-[var(--muted)]">Review issued credentials and revoke access you no longer recognize.</p></div><Button variant="secondary" onClick={async () => { await signOut(); router.push("/"); }}>Sign out this browser</Button></div>{error && <p role="alert" className="mt-4 rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}{message && <p role="status" className="mt-4 rounded-lg bg-[var(--brand-soft)] p-3 text-sm text-[var(--brand-strong)]">{message}</p>}{canManage && <div className="mt-5 divide-y divide-[var(--line)] border-y border-[var(--line)]">{credentials.map((credential) => <div key={credential.id} className="flex flex-col gap-3 py-4 sm:flex-row sm:items-center"><div className="min-w-0 flex-1"><div className="flex items-center gap-2"><p className="truncate text-sm font-semibold">{credential.name}</p><Badge tone={credential.revoked_at ? "neutral" : "success"}>{credential.revoked_at ? "Revoked" : credential.kind}</Badge></div><p className="mt-1 text-xs text-[var(--muted)]">Expires {new Date(credential.expires_at).toLocaleDateString()} · {credential.scopes.join(", ")}</p></div>{!credential.revoked_at && <Button variant="quiet" onClick={() => void revoke(credential.id)}>Revoke</Button>}</div>)}</div>}</Card>
  </div>;
}
