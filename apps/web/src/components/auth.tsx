"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { createContext, useContext, useEffect, useState, type FormEvent, type ReactNode } from "react";
import { api, type User } from "@/lib/api";
import { Icons } from "./icons";
import { Button, Card } from "./ui";

type AuthState = { user: User | null; token: string | null; loading: boolean; setSession: (token: string, user: User) => void; signOut: () => Promise<void> };
const AuthContext = createContext<AuthState | null>(null);
const storageKey = "vivarium.access-token";

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(null);
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    void Promise.resolve().then(async () => {
      const stored = window.localStorage.getItem(storageKey);
      if (!stored) { setLoading(false); return; }
      try { const current = await api<User>("/user", {}, stored); setToken(stored); setUser(current); }
      catch { window.localStorage.removeItem(storageKey); }
      finally { setLoading(false); }
    });
  }, []);

  function setSession(nextToken: string, nextUser: User) {
    window.localStorage.setItem(storageKey, nextToken);
    setToken(nextToken); setUser(nextUser); setLoading(false);
  }
  async function signOut() {
    try { if (token) await api<void>("/auth/session", { method: "DELETE" }, token); } finally {
      window.localStorage.removeItem(storageKey); setToken(null); setUser(null);
    }
  }
  return <AuthContext value={{ user, token, loading, setSession, signOut }}>{children}</AuthContext>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used within AuthProvider");
  return value;
}

export function AccountControl() {
  const { user, loading } = useAuth();
  if (loading) return <span className="size-9 animate-pulse rounded-full bg-black/[.06]" />;
  if (!user) return <Link href="/?access=signin" className="inline-flex min-h-9 items-center rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm font-semibold">Sign in</Link>;
  const initials = user.display_name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase();
  return <Link href="/settings" aria-label={`Account settings for ${user.display_name}`} className="grid size-9 place-items-center rounded-full bg-[#dce5de] text-xs font-bold text-[var(--brand-strong)] ring-1 ring-black/[.08]">{initials}</Link>;
}

export function AccessGate({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return <Card className="p-8"><p className="text-sm text-[var(--muted)]">Loading your workspace…</p></Card>;
  if (!user) return <Card className="p-8 text-center"><span className="mx-auto grid size-12 place-items-center rounded-xl bg-[var(--brand-soft)] text-[var(--brand)]"><Icons.Code /></span><h1 className="mt-4 text-2xl font-semibold">Your workspace is one step away</h1><p className="mx-auto mt-2 max-w-lg text-sm leading-6 text-[var(--muted)]">Create an account or sign in to find repositories and begin collaborating.</p><Link href="/?access=signin" className="mt-5 inline-flex min-h-10 items-center rounded-lg bg-[var(--brand)] px-4 text-sm font-semibold text-white">Get started</Link></Card>;
  return children;
}

export function WelcomeAccess() {
  const { setSession } = useAuth();
  const searchParams = useSearchParams();
  const [selectedMode, setSelectedMode] = useState<"create" | "signin" | null>(null);
  const mode = selectedMode ?? (searchParams.get("access") === "signin" ? "signin" : "create");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setPending(true); setError("");
    const data = new FormData(event.currentTarget);
    try {
      if (mode === "create") {
        const result = await api<{ user: User; credential: { token: string } }>("/users", { method: "POST", body: JSON.stringify({ display_name: data.get("display_name"), handle: data.get("handle") }) });
        setSession(result.credential.token, result.user);
      } else {
        const token = String(data.get("token") ?? "").trim();
        const user = await api<User>("/user", {}, token);
        setSession(token, user);
      }
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Unable to continue."); }
    finally { setPending(false); }
  }

  return <div className="grid min-h-[calc(100vh-9rem)] items-center gap-10 lg:grid-cols-[1.05fr_.95fr]">
    <section><p className="font-mono text-xs font-semibold uppercase tracking-[.16em] text-[var(--brand)]">Build in good company</p><h1 className="mt-4 max-w-2xl text-4xl font-semibold tracking-[-.045em] sm:text-6xl">From first idea to shared code.</h1><p className="mt-5 max-w-xl text-lg leading-8 text-[var(--muted)]">Create your identity, open a repository, and invite collaboration into the work from the beginning.</p><div className="mt-8 grid max-w-xl gap-3 sm:grid-cols-3">{[["1", "Join"], ["2", "Create a space"], ["3", "Build together"]].map(([number, label]) => <div key={number} className="flex items-center gap-3 rounded-xl border border-[var(--line)] bg-white/70 p-3 text-sm font-semibold"><span className="grid size-7 place-items-center rounded-full bg-[var(--brand-soft)] font-mono text-xs text-[var(--brand)]">{number}</span>{label}</div>)}</div></section>
    <Card className="p-6 sm:p-8"><div className="flex rounded-lg bg-[var(--canvas)] p-1" role="tablist" aria-label="Account access"><button type="button" role="tab" aria-selected={mode === "create"} onClick={() => { setSelectedMode("create"); setError(""); }} className={`flex-1 rounded-md px-3 py-2 text-sm font-semibold ${mode === "create" ? "bg-white shadow-sm" : "text-[var(--muted)]"}`}>Create account</button><button type="button" role="tab" aria-selected={mode === "signin"} onClick={() => { setSelectedMode("signin"); setError(""); }} className={`flex-1 rounded-md px-3 py-2 text-sm font-semibold ${mode === "signin" ? "bg-white shadow-sm" : "text-[var(--muted)]"}`}>Sign in</button></div>
      <div className="mt-6"><h2 className="text-xl font-semibold">{mode === "create" ? "Make your place in Vivarium" : "Welcome back"}</h2><p className="mt-1 text-sm leading-6 text-[var(--muted)]">{mode === "create" ? "You’ll go straight to your first collaborative workspace." : "Use a session or API token issued to your account."}</p></div>
      <form onSubmit={submit} className="mt-6 space-y-4">{mode === "create" ? <><Field label="Display name" name="display_name" placeholder="Avery Morgan" autoComplete="name" /><Field label="Handle" name="handle" placeholder="avery" autoComplete="username" pattern="[A-Za-z0-9-]{1,39}" hint="Letters, numbers, and hyphens." /></> : <Field label="Access token" name="token" type="password" autoComplete="current-password" placeholder="Paste your token" hint="Tokens are stored only in this browser." />}{error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}<Button type="submit" disabled={pending} className="w-full">{pending ? "Connecting…" : mode === "create" ? "Create account and continue" : "Sign in"}<Icons.Arrow size={16}/></Button></form>
    </Card>
  </div>;
}

function Field({ label, hint, ...props }: React.InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: string }) {
  return <label className="block text-sm font-semibold">{label}<input required className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal outline-none transition focus:border-[var(--brand)]" {...props}/>{hint && <span className="mt-1.5 block text-xs font-normal text-[var(--muted)]">{hint}</span>}</label>;
}
