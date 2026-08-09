import type { Metadata } from "next";
import { SecurityAdvisoriesWorkspace } from "@/components/security-advisories-workspace";
export const metadata: Metadata = { title: "Private security reports" };
export default function SecurityPage() { return <SecurityAdvisoriesWorkspace />; }
