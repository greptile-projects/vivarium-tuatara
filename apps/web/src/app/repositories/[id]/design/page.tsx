import type { Metadata } from "next";
import { DesignProposalWorkspace } from "@/components/design-proposal-workspace";

export const metadata: Metadata = { title: "Product design proposals" };
export default async function DesignPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <DesignProposalWorkspace repositoryID={id} />;
}
