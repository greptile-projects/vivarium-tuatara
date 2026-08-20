import type { Metadata } from "next";
import { SecurityExpectationsWorkspace } from "@/components/security-expectations-workspace";
import { ThreatModelsWorkspace } from "@/components/threat-models-workspace";

export const metadata: Metadata = { title: "Security expectations" };
export default async function SecurityPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <div className="space-y-12"><SecurityExpectationsWorkspace repositoryID={id} /><ThreatModelsWorkspace repositoryID={id} /></div>;
}
