import type { Metadata } from "next";
import { ReleaseDetail } from "@/components/releases-workspace";

export const metadata: Metadata = { title: "Release candidate" };
export default async function ReleasePage({ params }: { params: Promise<{ id: string; releaseId: string }> }) {
  const { id, releaseId } = await params;
  return <ReleaseDetail repositoryID={id} releaseID={releaseId} />;
}
