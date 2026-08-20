import type { Metadata } from "next";
import { CapabilityWorkspace } from "@/components/capability-workspace";

export const metadata: Metadata = { title: "Capability inventory" };
export default async function CapabilityPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <CapabilityWorkspace repositoryID={id} />;
}
