import type { Metadata } from "next";
import { PackagesWorkspace } from "@/components/packages-workspace";

export const metadata: Metadata = { title: "Packages" };

export default function PackagesPage() {
  return <PackagesWorkspace />;
}
