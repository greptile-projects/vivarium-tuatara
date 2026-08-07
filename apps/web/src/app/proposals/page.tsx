import type { Metadata } from "next";
import { Icons } from "@/components/icons";
import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Proposals" };

export default function ProposalsPage() {
  return <SectionPlaceholder eyebrow="Shape work together" title="Proposals" description="Proposal discovery and discussion will live here, using the same status, card, and action patterns as the rest of the application." icon={<Icons.Spark />} />;
}
