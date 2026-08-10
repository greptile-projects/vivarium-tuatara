import type { Metadata } from "next";
import { DecisionsWorkspace } from "@/components/decisions-workspace";
export const metadata: Metadata = { title: "Technical decision" };
export default async function DecisionPage({ params }: { params: Promise<{ decisionId: string }> }) { const { decisionId } = await params; return <DecisionsWorkspace decisionId={decisionId} />; }
