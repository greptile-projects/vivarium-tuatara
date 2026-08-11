import type { Metadata } from "next";
import { ContributorPathwayWorkspace } from "@/components/contributor-pathway-workspace";

export const metadata: Metadata = { title: "How to contribute" };

export default async function ContributorPathwayPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <ContributorPathwayWorkspace repositoryId={id} />;
}
