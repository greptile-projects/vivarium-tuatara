import type { Metadata } from "next";
import { IssuesWorkspace } from "@/components/issues-workspace";

export const metadata: Metadata = { title: "Issues" };
export default function IssuesPage() { return <IssuesWorkspace />; }
