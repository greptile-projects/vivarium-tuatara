import type { Metadata } from "next";
import { AdoptionWorkspace } from "@/components/adoption-workspace";

export const metadata: Metadata = { title: "Software adoption" };
export default function AdoptionPage() { return <AdoptionWorkspace />; }
