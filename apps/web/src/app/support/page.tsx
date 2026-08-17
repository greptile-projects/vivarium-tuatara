import type { Metadata } from "next";
import { SupportWorkspace } from "@/components/support-workspace";

export const metadata: Metadata = { title: "Developer support" };
export default function SupportPage() { return <SupportWorkspace />; }
