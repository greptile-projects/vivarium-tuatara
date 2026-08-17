import type { Metadata } from "next";
import { APIIntegrationSandbox } from "@/components/api-integration-sandbox";
export const metadata: Metadata = { title: "Consumer integrations" };
export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <APIIntegrationSandbox repositoryID={id} />;
}
