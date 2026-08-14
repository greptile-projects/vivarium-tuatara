import type { Metadata } from "next";
import { AccessibilityCommitmentsWorkspace } from "@/components/accessibility-commitments-workspace";

export const metadata: Metadata = { title: "Accessibility commitments" };
export default async function AccessibilityPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <AccessibilityCommitmentsWorkspace repositoryID={id} />;
}
