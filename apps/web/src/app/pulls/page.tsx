import type { Metadata } from "next";
import { PullRequestsWorkspace } from "@/components/pull-requests-workspace";

export const metadata: Metadata = { title: "Pull requests" };

export default function PullRequestsPage() {
  return <PullRequestsWorkspace />;
}
