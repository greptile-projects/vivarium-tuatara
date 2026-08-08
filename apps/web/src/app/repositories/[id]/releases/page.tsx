import type { Metadata } from "next";
import { ReleasesWorkspace } from "@/components/releases-workspace";

export const metadata: Metadata = { title: "Releases" };
export default async function ReleasesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <ReleasesWorkspace repositoryID={id} />;
}
