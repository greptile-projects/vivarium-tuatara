import type { Metadata } from "next";
import { RepositoryBrowser } from "@/components/repository-browser";

export const metadata: Metadata = { title: "Repository" };

export default async function RepositoryPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <RepositoryBrowser id={id} />;
}
