import type { Metadata } from "next";
import { AdoptionWorkspace } from "@/components/adoption-workspace";
import { AdoptionDeliveryPanel } from "@/components/adoption-delivery-panel";
import { AdoptionUpstreamPanel } from "@/components/adoption-upstream-panel";

export const metadata: Metadata = { title: "Software adoption" };
export default function AdoptionPage() {
  return (
    <>
      <AdoptionWorkspace />
      <AdoptionDeliveryPanel />
      <AdoptionUpstreamPanel />
    </>
  );
}
