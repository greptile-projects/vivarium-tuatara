import type { Metadata } from "next";
import { ProposalsWorkspace } from "@/components/proposals-workspace";

export const metadata: Metadata = { title: "Proposals" };

export default function ProposalsPage() {
  return <ProposalsWorkspace />;
}
