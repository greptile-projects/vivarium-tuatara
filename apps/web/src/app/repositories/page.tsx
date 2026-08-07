import type { Metadata } from "next";
import { Icons } from "@/components/icons";
import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Repositories" };

export default function RepositoriesPage() {
  return <SectionPlaceholder eyebrow="Your workspaces" title="Repositories" description="Repository discovery and creation will land here in the next workflow. The shared shell, page boundary, and empty-state language are already in place." icon={<Icons.Code />} />;
}
