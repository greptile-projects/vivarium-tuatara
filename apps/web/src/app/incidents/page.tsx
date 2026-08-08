import type { Metadata } from "next";
import { IncidentsWorkspace } from "@/components/incidents-workspace";
export const metadata: Metadata = { title: "Incidents" };
export default function IncidentsPage() { return <IncidentsWorkspace />; }
