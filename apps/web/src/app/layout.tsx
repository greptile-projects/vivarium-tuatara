import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import type { ReactNode } from "react";
import { AppShell } from "@/components/app-shell";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: {
    default: "Vivarium",
    template: "%s · Vivarium",
  },
  description: "A shared place to propose, build, review, and ship software.",
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full">
        <a
          href="#main-content"
          className="fixed left-3 top-3 z-50 -translate-y-20 rounded-lg bg-[var(--ink)] px-4 py-2 text-sm font-semibold text-white transition-transform focus:translate-y-0"
        >
          Skip to content
        </a>
        <AppShell>{children}</AppShell>
      </body>
    </html>
  );
}
