import type { Metadata } from "next";
import { PullRequestDetail } from "@/components/pull-requests-workspace";

export const metadata: Metadata = { title: "Pull request" };

export default async function PullRequestPage({ params }: { params: Promise<{ repositoryId: string; pullRequestId: string }> }) {
  const { repositoryId, pullRequestId } = await params;
  return <PullRequestDetail repositoryID={repositoryId} pullRequestID={pullRequestId} />;
}
