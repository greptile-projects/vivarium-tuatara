import type { Metadata } from "next";
import { OutcomeFundingWorkspace } from "@/components/outcome-funding-workspace";

export const metadata: Metadata = { title: "Outcome funding" };

export default async function FundingPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <OutcomeFundingWorkspace repositoryID={id} />;
}
