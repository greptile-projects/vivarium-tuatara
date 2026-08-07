import type { Metadata } from "next";
import { ChangeSessionDetail } from "@/components/change-sessions";

export const metadata: Metadata = { title: "Change session" };

export default async function ChangeSessionPage({ params }: { params: Promise<{ repositoryId: string; pullRequestId: string; sessionId: string }> }) {
  const { repositoryId, pullRequestId, sessionId } = await params;
  return <ChangeSessionDetail repositoryID={repositoryId} pullRequestID={pullRequestId} sessionID={sessionId} />;
}
