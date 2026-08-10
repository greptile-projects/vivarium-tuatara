import type { Metadata } from "next";
import { DeliveryTeamsWorkspace } from "@/components/delivery-teams-workspace";
export const metadata: Metadata = { title: "Delivery teams" };
export default function DeliveryTeamsPage(){ return <DeliveryTeamsWorkspace/>; }
