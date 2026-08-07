import type { Metadata } from "next";
import { AccessGate } from "@/components/auth";
import { AccountSettings } from "@/components/account-settings";

export const metadata: Metadata = { title: "Settings" };

export default function SettingsPage() {
  return <AccessGate><div className="space-y-7"><section><p className="font-mono text-xs font-semibold uppercase tracking-[.16em] text-[var(--brand)]">Your account</p><h1 className="mt-2 text-3xl font-semibold tracking-[-.035em]">Profile & access</h1><p className="mt-2 text-[var(--muted)]">Keep your identity current and control every way into your work.</p></section><AccountSettings /></div></AccessGate>;
}
