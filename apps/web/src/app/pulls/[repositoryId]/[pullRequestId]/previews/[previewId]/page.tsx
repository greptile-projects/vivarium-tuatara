import type { Metadata } from "next";
import { PullRequestPreviews } from "@/components/pull-request-previews";

export const metadata: Metadata = { title: "Preview feedback" };

export default async function PreviewFeedbackPage({ params }: { params: Promise<{repositoryId:string;pullRequestId:string;previewId:string}> }) {
  const {repositoryId,pullRequestId,previewId}=await params;
  return <PullRequestPreviews repositoryID={repositoryId} pullRequestID={pullRequestId} previewID={previewId} participant={false} owner={false}/>;
}
