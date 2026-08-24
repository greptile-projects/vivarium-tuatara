import type { Metadata } from "next";
import { HistoryRemediationWorkspace } from "@/components/history-remediation-workspace";
export const metadata: Metadata = { title: "Sensitive history remediation" };
export default async function Page({ params }: { params: Promise<{ id: string }> }) { const { id } = await params; return <HistoryRemediationWorkspace repositoryID={id} />; }
