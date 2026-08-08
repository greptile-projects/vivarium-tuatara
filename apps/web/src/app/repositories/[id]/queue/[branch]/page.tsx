import type { Metadata } from "next";
import { IntegrationQueueWorkspace } from "@/components/integration-queue-workspace";

export const metadata: Metadata = { title: "Integration queue" };

export default async function QueuePage({
  params,
}: {
  params: Promise<{ id: string; branch: string }>;
}) {
  const { id, branch } = await params;
  return (
    <IntegrationQueueWorkspace
      repositoryID={id}
      branch={decodeURIComponent(branch)}
    />
  );
}
