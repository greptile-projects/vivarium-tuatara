import type { Metadata } from "next";
import { AccessGate } from "@/components/auth";
import { RepositoryWorkspace } from "@/components/repository-workspace";

export const metadata: Metadata = { title: "Repositories" };

export default function RepositoriesPage() {
  return <AccessGate><div className="space-y-7"><section><p className="font-mono text-xs font-semibold uppercase tracking-[.16em] text-[var(--brand)]">Shared workspaces</p><h1 className="mt-2 text-3xl font-semibold tracking-[-.035em]">Repositories</h1><p className="mt-2 text-[var(--muted)]">Create or locate the place where your next collaboration begins.</p></section><RepositoryWorkspace /></div></AccessGate>;
}
