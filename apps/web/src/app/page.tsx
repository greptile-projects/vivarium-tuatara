"use client";

import { AccessGate, useAuth, WelcomeAccess } from "@/components/auth";
import { RepositoryWorkspace } from "@/components/repository-workspace";

export default function Home() {
  const { user, loading } = useAuth();
  if (loading) return <p className="text-sm text-[var(--muted)]">Opening Vivarium…</p>;
  if (!user) return <WelcomeAccess />;
  return <AccessGate><div className="space-y-8"><section><p className="font-mono text-xs font-semibold uppercase tracking-[.16em] text-[var(--brand)]">Welcome to Vivarium</p><h1 className="mt-2 text-3xl font-semibold tracking-[-.035em] sm:text-4xl">Let’s build something, {user.display_name.split(" ")[0]}.</h1><p className="mt-3 max-w-2xl text-base leading-7 text-[var(--muted)]">Start with a repository—the shared space where ideas, code, and collaborators meet.</p></section><RepositoryWorkspace compact /></div></AccessGate>;
}
