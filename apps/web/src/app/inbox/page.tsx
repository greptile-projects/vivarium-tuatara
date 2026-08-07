import type { Metadata } from "next";
import { InboxWorkspace } from "@/components/inbox-workspace";

export const metadata: Metadata = { title: "Inbox" };

export default function InboxPage() {
  return <InboxWorkspace />;
}
