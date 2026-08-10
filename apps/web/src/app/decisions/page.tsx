import type { Metadata } from "next";
import { DecisionsWorkspace } from "@/components/decisions-workspace";
export const metadata: Metadata = { title: "Decisions" };
export default function DecisionsPage() { return <DecisionsWorkspace />; }
