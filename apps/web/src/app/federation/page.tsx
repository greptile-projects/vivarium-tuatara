import type { Metadata } from "next";
import { FederationWorkspace } from "@/components/federation-workspace";
export const metadata: Metadata = { title: "Federation" };
export default function FederationPage() { return <FederationWorkspace />; }
