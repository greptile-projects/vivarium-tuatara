import type { Metadata } from "next";
import { DependencyInventoryWorkspace } from "@/components/dependency-inventory-workspace";

export const metadata: Metadata = { title: "Dependency inventory" };
export default async function DependencyInventoryPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <DependencyInventoryWorkspace repositoryID={id} />;
}
