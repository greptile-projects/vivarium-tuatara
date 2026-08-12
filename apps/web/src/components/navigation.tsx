"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Icons } from "./icons";

const navigation = [
  { label: "Home", href: "/", icon: Icons.Home },
  { label: "Repositories", href: "/repositories", icon: Icons.Code },
  { label: "Workspaces", href: "/workspaces", icon: Icons.Code },
  { label: "Organizations", href: "/organizations", icon: Icons.Home },
  { label: "Extensions", href: "/extensions", icon: Icons.Spark },
  { label: "Federation", href: "/federation", icon: Icons.Activity },
  { label: "Packages", href: "/packages", icon: Icons.Code },
  { label: "Proposals", href: "/proposals", icon: Icons.Spark },
  { label: "Decisions", href: "/decisions", icon: Icons.Activity },
  { label: "Governance", href: "/governance", icon: Icons.Activity },
  { label: "Delivery teams", href: "/delivery-teams", icon: Icons.Spark },
  { label: "Pull requests", href: "/pulls", icon: Icons.GitPull },
  { label: "Issues", href: "/issues", icon: Icons.Activity },
  { label: "Incidents", href: "/incidents", icon: Icons.Activity },
  { label: "Security", href: "/security", icon: Icons.Code },
  { label: "Inbox", href: "/inbox", icon: Icons.Bell },
  { label: "Activity", href: "/activity", icon: Icons.Activity },
];

export function Navigation() {
  const pathname = usePathname();

  return (
    <nav aria-label="Primary" className="space-y-1">
      {navigation.map(({ label, href, icon: Icon }) => {
        const current = href === "/" ? pathname === href : pathname.startsWith(`${href}/`) || pathname === href;

        return (
          <Link
            key={href}
            href={href}
            aria-current={current ? "page" : undefined}
            className={`group flex min-h-10 items-center gap-3 rounded-lg px-3 text-sm font-medium transition ${current ? "bg-[var(--brand-soft)] text-[var(--brand-strong)]" : "text-[var(--muted)] hover:bg-black/[.035] hover:text-[var(--ink)]"}`}
          >
            <Icon size={17} />
            <span className="flex-1">{label}</span>
          </Link>
        );
      })}
    </nav>
  );
}

export function SettingsLink() {
  const pathname = usePathname();
  const current = pathname === "/settings" || pathname.startsWith("/settings/");

  return (
    <Link
      href="/settings"
      aria-current={current ? "page" : undefined}
      className={`mt-3 flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition ${current ? "bg-[var(--brand-soft)] font-medium text-[var(--brand-strong)]" : "text-[var(--muted)] hover:bg-black/[.035] hover:text-[var(--ink)]"}`}
    >
      <Icons.Settings size={17} />
      Settings
    </Link>
  );
}
