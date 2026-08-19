import type { Metadata } from "next";
import { QualityPlansWorkspace } from "@/components/quality-plans-workspace";

export const metadata: Metadata = { title: "Quality plans" };
export default async function QualityPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <QualityPlansWorkspace repositoryID={id} />;
}
