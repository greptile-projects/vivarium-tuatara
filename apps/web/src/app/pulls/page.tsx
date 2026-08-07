import type { Metadata } from "next";
import { Icons } from "@/components/icons";
import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Pull requests" };

export default function PullRequestsPage() {
  return <SectionPlaceholder eyebrow="Review contributions" title="Pull requests" description="Review requests will be discoverable here once the contribution workflow is connected to the API." icon={<Icons.GitPull />} />;
}
