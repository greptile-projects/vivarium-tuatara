import { AccessGate } from "@/components/auth";
import { WorkspaceLauncher } from "@/components/development-workspaces";
export default function Page(){return <AccessGate><div className="mx-auto max-w-2xl space-y-6"><header><h1 className="text-3xl font-semibold">Launch a workspace</h1><p className="mt-2 text-[var(--muted)]">The selected commit must contain <code>.vivarium/workspace.json</code>. Its tools, setup, and resource bounds become the durable foundation.</p></header><WorkspaceLauncher/></div></AccessGate>}
