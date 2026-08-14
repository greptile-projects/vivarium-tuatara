import type { Metadata } from "next";
import { LocalePlansWorkspace } from "@/components/locale-plans-workspace";

export const metadata: Metadata = { title: "Locale plans" };
export default async function LocalePlansPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <LocalePlansWorkspace repositoryID={id} />;
}
