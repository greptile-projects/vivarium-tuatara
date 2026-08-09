import { AccessGate } from "@/components/auth";
import { DevelopmentWorkspaces } from "@/components/development-workspaces";
export default function Page(){return <AccessGate><div className="space-y-6"><header><p className="font-mono text-xs font-semibold uppercase tracking-widest text-[var(--brand)]">Reproducible development</p><h1 className="mt-2 text-3xl font-semibold">Workspaces</h1><p className="mt-2 text-[var(--muted)]">Return to exact, evidence-backed project foundations shared through repository context.</p></header><DevelopmentWorkspaces/></div></AccessGate>}
