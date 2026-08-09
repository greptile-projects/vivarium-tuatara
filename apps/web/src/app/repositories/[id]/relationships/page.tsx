import type { Metadata } from "next";
import { RelationshipsWorkspace } from "@/components/relationships-workspace";

export const metadata: Metadata = { title: "Interface relationships" };
export default async function RelationshipsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <RelationshipsWorkspace repositoryID={id} />;
}
