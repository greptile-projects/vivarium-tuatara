import type { Metadata } from "next";
import { AccessibilityCommitmentsWorkspace } from "@/components/accessibility-commitments-workspace";
import { AccessibilityReportsWorkspace } from "@/components/accessibility-reports-workspace";
import { AccessibilityAssessmentsWorkspace } from "@/components/accessibility-assessments-workspace";

export const metadata: Metadata = { title: "Accessibility commitments" };
export default async function AccessibilityPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <div className="space-y-10"><AccessibilityCommitmentsWorkspace repositoryID={id} /><AccessibilityReportsWorkspace repositoryID={id} /><AccessibilityAssessmentsWorkspace repositoryID={id} /></div>;
}
