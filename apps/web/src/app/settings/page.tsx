import type { Metadata } from "next";
import { Icons } from "@/components/icons";
import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Settings" };

export default function SettingsPage() {
  return <SectionPlaceholder eyebrow="Your account" title="Settings" description="Profile and access management will use this route when onboarding is connected to the existing identity and credential APIs." icon={<Icons.Settings />} />;
}
