import type { ButtonHTMLAttributes, HTMLAttributes, ReactNode } from "react";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "quiet";
};

export function Button({ variant = "primary", className = "", ...props }: ButtonProps) {
  const styles = {
    primary: "border-transparent bg-[var(--brand)] text-white shadow-sm hover:bg-[var(--brand-strong)]",
    secondary: "border-[var(--line-strong)] bg-white text-[var(--ink)] hover:border-[#aab4aa] hover:bg-[#f8faf7]",
    quiet: "border-transparent bg-transparent text-[var(--muted)] hover:bg-black/[.04] hover:text-[var(--ink)]",
  };
  return <button className={`inline-flex min-h-9 items-center justify-center gap-2 rounded-lg border px-3.5 text-sm font-semibold transition disabled:cursor-not-allowed disabled:opacity-45 ${styles[variant]} ${className}`} {...props} />;
}

export function Badge({ tone = "neutral", children }: { tone?: "neutral" | "success" | "warning" | "info" | "danger"; children: ReactNode }) {
  const styles = {
    neutral: "bg-[#edf0eb] text-[#59625c]",
    success: "bg-[var(--brand-soft)] text-[var(--brand-strong)]",
    warning: "bg-[var(--warning-soft)] text-[var(--warning)]",
    info: "bg-[var(--info-soft)] text-[var(--info)]",
    danger: "bg-[var(--danger-soft)] text-[var(--danger)]",
  };
  return <span className={`inline-flex w-fit items-center rounded-full px-2 py-0.5 text-xs font-semibold ${styles[tone]}`}>{children}</span>;
}

export function Card({ className = "", ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={`rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] shadow-[var(--shadow-card)] ${className}`} {...props} />;
}

export function Avatar({ initials, label, size = "md" }: { initials: string; label: string; size?: "sm" | "md" }) {
  return <span aria-label={label} role="img" className={`${size === "sm" ? "size-7 text-[10px]" : "size-9 text-xs"} inline-flex shrink-0 items-center justify-center rounded-full bg-[#dce5de] font-bold tracking-wide text-[var(--brand-strong)] ring-1 ring-inset ring-black/[.06]`}>{initials}</span>;
}
