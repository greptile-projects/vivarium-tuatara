import type { Metadata } from "next";
import { CodeNavigationWorkspace } from "@/components/code-navigation-workspace";

export const metadata: Metadata = { title: "Code navigation" };
export default async function CodePage({ params }: { params: Promise<{ id: string }> }) { const { id } = await params; return <CodeNavigationWorkspace id={id} />; }
