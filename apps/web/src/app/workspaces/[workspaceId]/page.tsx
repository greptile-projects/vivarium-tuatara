import { AccessGate } from "@/components/auth";
import { DevelopmentWorkspaces } from "@/components/development-workspaces";
export default async function Page({params}:{params:Promise<{workspaceId:string}>}){const {workspaceId}=await params;return <AccessGate><div className="space-y-6"><header><h1 className="text-3xl font-semibold">Development workspace</h1><p className="mt-2 text-[var(--muted)]">Durable lifecycle, access, setup evidence, and frozen environment definition.</p></header><DevelopmentWorkspaces workspaceID={workspaceId}/></div></AccessGate>}
