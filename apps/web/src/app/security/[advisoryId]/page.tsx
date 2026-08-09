import type { Metadata } from "next";
import { SecurityAdvisoriesWorkspace } from "@/components/security-advisories-workspace";
export const metadata: Metadata = { title: "Security report" };
export default async function SecurityDetailPage({ params }: { params: Promise<{ advisoryId: string }> }) {
  const { advisoryId } = await params;
  return <SecurityAdvisoriesWorkspace advisoryId={advisoryId} />;
}
