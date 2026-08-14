import type { Metadata } from "next";
import { LocalePlansWorkspace } from "@/components/locale-plans-workspace";
import { LocalizationDeliveryWorkspace } from "@/components/localization-delivery-workspace";

export const metadata: Metadata = { title: "Locale plans" };
export default async function LocalePlansPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <div className="space-y-10"><LocalePlansWorkspace repositoryID={id} /><LocalizationDeliveryWorkspace repositoryID={id} /></div>;
}
