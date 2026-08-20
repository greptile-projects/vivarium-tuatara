import type { Metadata } from "next";
import { SecurityExpectationsWorkspace } from "@/components/security-expectations-workspace";

export const metadata: Metadata = { title: "Security expectations" };
export default async function SecurityPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <SecurityExpectationsWorkspace repositoryID={id} />;
}
