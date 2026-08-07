import type { Metadata } from "next";
import { Icons } from "@/components/icons";
import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Activity" };

export default function ActivityPage() {
  return <SectionPlaceholder eyebrow="Across your work" title="Activity" description="A chronological view of attributable collaboration will appear here as platform workflows come online." icon={<Icons.Activity />} />;
}
