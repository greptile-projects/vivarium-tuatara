import type { Metadata } from "next";
import { IncidentDetail } from "@/components/incidents-workspace";
export const metadata: Metadata = { title: "Incident" };
export default async function IncidentPage({params}:{params:Promise<{incidentId:string}>}) { const {incidentId}=await params; return <IncidentDetail incidentID={incidentId}/>; }
