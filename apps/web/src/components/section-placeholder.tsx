import type { ReactNode } from "react";
import { Card } from "./ui";

export function SectionPlaceholder({ eyebrow, title, description, icon }: { eyebrow: string; title: string; description: string; icon: ReactNode }) {
  return (
    <div className="space-y-7">
      <header>
        <p className="mb-2 font-mono text-xs font-semibold uppercase tracking-[0.16em] text-[var(--brand)]">{eyebrow}</p>
        <h1 className="text-3xl font-semibold tracking-[-0.035em] sm:text-4xl">{title}</h1>
      </header>
      <Card className="grid min-h-72 place-items-center p-8 text-center">
        <div className="max-w-md">
          <span className="mx-auto grid size-12 place-items-center rounded-xl bg-[var(--brand-soft)] text-[var(--brand)]">{icon}</span>
          <h2 className="mt-5 text-lg font-semibold">This workspace is ready</h2>
          <p className="mt-2 text-sm leading-6 text-[var(--muted)]">{description}</p>
        </div>
      </Card>
    </div>
  );
}
