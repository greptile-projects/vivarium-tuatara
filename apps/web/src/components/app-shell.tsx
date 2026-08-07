import Link from "next/link";
import type { ReactNode } from "react";
import { Icons } from "./icons";
import { Navigation, SettingsLink } from "./navigation";
import { Avatar, Button } from "./ui";

function Brand() {
  return <Link href="/" className="inline-flex items-center gap-2.5 rounded-md font-semibold tracking-[-0.02em]"><span className="grid size-8 place-items-center rounded-lg bg-[var(--brand)] text-sm font-bold text-white shadow-sm">V</span><span>vivarium</span></Link>;
}

export function AppShell({ children }: { children: ReactNode }) {
  return <div className="min-h-screen lg:grid lg:grid-cols-[15.5rem_minmax(0,1fr)]">
    <aside className="fixed inset-y-0 left-0 z-20 hidden w-[15.5rem] flex-col border-r border-[var(--line)] bg-[var(--surface)] px-4 py-5 lg:flex">
      <div className="px-2"><Brand /></div>
      <div className="mt-8"><Navigation /></div>
      <div className="mt-auto rounded-xl border border-[var(--line)] bg-white p-3.5"><div className="flex items-center gap-2 text-xs font-semibold text-[var(--brand-strong)]"><Icons.Spark size={15}/> Built for shared work</div><p className="mt-2 text-xs leading-5 text-[var(--muted)]">Propose an idea, shape it together, then carry its context into code.</p></div>
      <SettingsLink />
    </aside>
    <div className="min-w-0 lg:col-start-2">
      <header className="sticky top-0 z-10 flex h-16 items-center gap-3 border-b border-[var(--line)] bg-[rgb(251_252_248_/_0.88)] px-4 backdrop-blur-xl sm:px-6 lg:px-8">
        <details className="group relative lg:hidden"><summary aria-label="Open navigation" className="grid size-9 cursor-pointer list-none place-items-center rounded-lg text-[var(--muted)] hover:bg-black/[.04]"><Icons.Menu/></summary><div className="absolute left-0 top-12 w-64 rounded-xl border border-[var(--line)] bg-white p-3 shadow-xl"><div className="mb-4 px-2"><Brand/></div><Navigation/></div></details>
        <button type="button" className="flex min-h-9 min-w-0 flex-1 items-center gap-2 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-left text-sm text-[var(--muted)] shadow-sm transition hover:border-[#aeb7ae] sm:max-w-sm"><Icons.Search size={16}/><span className="truncate">Search repositories and work…</span><kbd className="ml-auto hidden rounded border border-[var(--line)] bg-[var(--canvas)] px-1.5 py-0.5 font-mono text-[10px] sm:block">⌘ K</kbd></button>
        <Button variant="secondary" className="hidden sm:inline-flex"><Icons.Plus size={16}/>Create</Button>
        <button type="button" aria-label="Notifications, 3 unread" className="relative grid size-9 place-items-center rounded-lg text-[var(--muted)] hover:bg-black/[.04] hover:text-[var(--ink)]"><Icons.Bell size={18}/><span className="absolute right-1.5 top-1.5 size-2 rounded-full bg-[var(--danger)] ring-2 ring-[var(--surface)]"/></button>
        <button type="button" aria-label="Open account menu" className="rounded-full"><Avatar initials="AM" label="Avery Morgan" /></button>
      </header>
      <main id="main-content" className="mx-auto w-full max-w-[88rem] px-4 py-8 sm:px-6 lg:px-8 lg:py-10">{children}</main>
    </div>
  </div>;
}
