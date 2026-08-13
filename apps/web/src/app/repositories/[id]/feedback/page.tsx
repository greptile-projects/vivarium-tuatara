import type { Metadata } from "next";
import { FeedbackWorkspace } from "@/components/feedback-workspace";
export const metadata: Metadata = { title: "Product feedback" };
export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <FeedbackWorkspace repositoryID={id} />;
}
