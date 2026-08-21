import type { Metadata } from "next";
import { IncubatorsWorkspace } from "@/components/incubators-workspace";
export const metadata: Metadata = { title: "Project incubators" };
export default function IncubatorsPage(){ return <IncubatorsWorkspace/>; }
