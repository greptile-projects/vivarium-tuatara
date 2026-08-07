import type { Metadata } from "next";
import { ProposalConversation } from "@/components/proposals-workspace";

export const metadata: Metadata = { title: "Proposal" };

export default async function ProposalPage({
  params,
}: {
  params: Promise<{ repositoryId: string; proposalId: string }>;
}) {
  const { repositoryId, proposalId } = await params;
  return <ProposalConversation repositoryID={repositoryId} proposalID={proposalId} />;
}
