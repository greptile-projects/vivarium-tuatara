import type { Metadata } from "next";
import { InterfaceSystemWorkspace } from "@/components/interface-system-workspace";

export const metadata: Metadata = { title: "Interface system" };
export default async function InterfaceSystemPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <InterfaceSystemWorkspace repositoryID={id} />;
}
