import type { Metadata } from "next";
import { AdoptionWorkspace } from "@/components/adoption-workspace";
import { AdoptionDeliveryPanel } from "@/components/adoption-delivery-panel";

export const metadata: Metadata = { title: "Software adoption" };
export default function AdoptionPage() {
  return (
    <>
      <AdoptionWorkspace />
      <AdoptionDeliveryPanel />
    </>
  );
}
