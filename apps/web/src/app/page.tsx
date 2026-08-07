import type { Metadata } from "next";
import { Icons } from "@/components/icons";
import { Avatar, Badge, Button, Card } from "@/components/ui";

export const metadata: Metadata = { title: "Home" };

const repositories = [
  { name: "orchard", description: "Coordination tools for humans and agents working in one codebase.", language: "TypeScript", activity: "18 min ago", access: "Private" },
  { name: "field-notes", description: "Small, durable records for decisions made while building.", language: "Go", activity: "Yesterday", access: "Public" },
  { name: "seedling", description: "A compact experiment in reproducible development environments.", language: "Shell", activity: "Aug 4", access: "Private" },
];

const work = [
  { kind: "Pull request", tone: "info" as const, title: "Make review state explicit after new commits", repo: "orchard", number: "#42", meta: "2 approvals", status: "Ready to merge", statusTone: "success" as const },
  { kind: "Proposal", tone: "warning" as const, title: "Define how agents hand work back to people", repo: "orchard", number: "#18", meta: "6 comments", status: "Needs response", statusTone: "warning" as const },
  { kind: "Pull request", tone: "info" as const, title: "Preserve attribution in exported notes", repo: "field-notes", number: "#11", meta: "Changes requested", status: "Waiting on you", statusTone: "danger" as const },
];

export default function Home() {
  return <div className="space-y-9">
    <section aria-labelledby="welcome-heading" className="flex flex-col justify-between gap-5 sm:flex-row sm:items-end">
      <div><p className="mb-2 font-mono text-xs font-semibold uppercase tracking-[0.16em] text-[var(--brand)]">Friday, August 7</p><h1 id="welcome-heading" className="text-3xl font-semibold tracking-[-0.035em] text-[var(--ink)] sm:text-4xl">Good morning, Avery.</h1><p className="mt-3 max-w-2xl text-base leading-7 text-[var(--muted)]">Pick up a conversation, review a contribution, or start something worth building together.</p></div>
      <Button><Icons.Plus size={16}/>New repository</Button>
    </section>

    <section aria-labelledby="attention-heading">
      <div className="mb-3 flex items-center justify-between"><div><h2 id="attention-heading" className="text-lg font-semibold tracking-[-0.015em]">Needs your attention</h2><p className="mt-1 text-sm text-[var(--muted)]">The shortest path back into shared work.</p></div><button className="hidden items-center gap-1 text-sm font-semibold text-[var(--brand)] hover:text-[var(--brand-strong)] sm:flex">View all <Icons.Arrow size={15}/></button></div>
      <Card className="divide-y divide-[var(--line)] overflow-hidden">
        {work.map((item) => <article key={item.title} className="group flex flex-col gap-3 p-4 transition hover:bg-[#fafbf8] sm:flex-row sm:items-center sm:p-5"><div className="flex min-w-0 flex-1 items-start gap-3"><span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-lg bg-[var(--canvas)] text-[var(--muted)]">{item.kind === "Pull request" ? <Icons.GitPull size={17}/> : <Icons.Spark size={17}/>}</span><div className="min-w-0"><div className="mb-1.5 flex flex-wrap items-center gap-2"><Badge tone={item.tone}>{item.kind}</Badge><span className="font-mono text-xs text-[var(--muted)]">{item.repo} {item.number}</span></div><h3 className="font-semibold leading-6 transition group-hover:text-[var(--brand)]">{item.title}</h3><p className="mt-1 text-sm text-[var(--muted)]">{item.meta}</p></div></div><div className="ml-12 flex items-center justify-between gap-3 sm:ml-0"><Badge tone={item.statusTone}>{item.status}</Badge><button aria-label={`Open ${item.title}`} className="grid size-8 place-items-center rounded-lg text-[var(--muted)] hover:bg-black/[.05] hover:text-[var(--ink)]"><Icons.Chevron size={16}/></button></div></article>)}
      </Card>
    </section>

    <div className="grid gap-8 xl:grid-cols-[minmax(0,1.45fr)_minmax(19rem,.75fr)]">
      <section aria-labelledby="repositories-heading"><div className="mb-3 flex items-end justify-between"><div><h2 id="repositories-heading" className="text-lg font-semibold tracking-[-0.015em]">Your repositories</h2><p className="mt-1 text-sm text-[var(--muted)]">Recently active spaces you own or contribute to.</p></div><button className="text-sm font-semibold text-[var(--brand)] hover:text-[var(--brand-strong)]">View all</button></div><Card className="divide-y divide-[var(--line)] overflow-hidden">{repositories.map((repo) => <article key={repo.name} className="group p-5 transition hover:bg-[#fafbf8]"><div className="flex items-start gap-3"><span className="grid size-9 shrink-0 place-items-center rounded-lg border border-[var(--line)] bg-white text-[var(--brand)]"><Icons.Code size={17}/></span><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><h3 className="font-mono text-sm font-semibold group-hover:text-[var(--brand)]">{repo.name}</h3><Badge>{repo.access}</Badge></div><p className="mt-2 text-sm leading-6 text-[var(--muted)]">{repo.description}</p><div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-[var(--muted)]"><span className="flex items-center gap-1.5"><span className={`size-2 rounded-full ${repo.language === "Go" ? "bg-[#4d9cad]" : repo.language === "Shell" ? "bg-[#9b7a3c]" : "bg-[#5679ae]"}`}/>{repo.language}</span><span className="flex items-center gap-1.5"><Icons.Branch size={13}/>main</span><span>Updated {repo.activity}</span></div></div><Icons.Chevron className="mt-1 text-[var(--muted)]" size={16}/></div></article>)}</Card></section>

      <aside className="space-y-6" aria-label="Workspace overview">
        <Card className="overflow-hidden"><div className="border-b border-[var(--line)] p-5"><h2 className="font-semibold">This week</h2><p className="mt-1 text-sm text-[var(--muted)]">Your collaboration pulse.</p></div><dl className="grid grid-cols-3 divide-x divide-[var(--line)]"><div className="p-4"><dt className="text-xs text-[var(--muted)]">Proposed</dt><dd className="mt-1 text-2xl font-semibold tracking-tight">3</dd></div><div className="p-4"><dt className="text-xs text-[var(--muted)]">Reviewed</dt><dd className="mt-1 text-2xl font-semibold tracking-tight">7</dd></div><div className="p-4"><dt className="text-xs text-[var(--muted)]">Merged</dt><dd className="mt-1 text-2xl font-semibold tracking-tight">4</dd></div></dl></Card>
        <Card className="p-5"><div className="flex items-start justify-between gap-4"><div><h2 className="font-semibold">Recent activity</h2><p className="mt-1 text-sm text-[var(--muted)]">Across your shared spaces.</p></div><Icons.Activity className="text-[var(--brand)]" size={18}/></div><ol className="mt-5 space-y-5"><li className="flex gap-3"><Avatar initials="JL" label="Jordan Lee" size="sm"/><p className="text-sm leading-5"><strong className="font-semibold">Jordan</strong> approved <span className="font-medium text-[var(--brand)]">orchard #42</span><span className="mt-1 block text-xs text-[var(--muted)]">12 minutes ago</span></p></li><li className="flex gap-3"><Avatar initials="RK" label="Rin Kim" size="sm"/><p className="text-sm leading-5"><strong className="font-semibold">Rin</strong> replied to your proposal<span className="mt-1 block text-xs text-[var(--muted)]">1 hour ago</span></p></li><li className="flex gap-3"><span className="grid size-7 shrink-0 place-items-center rounded-full bg-[var(--brand-soft)] text-[var(--brand)]"><Icons.Branch size={13}/></span><p className="text-sm leading-5"><strong className="font-semibold">main</strong> advanced in field-notes<span className="mt-1 block text-xs text-[var(--muted)]">Yesterday</span></p></li></ol><Button variant="secondary" className="mt-5 w-full">View activity</Button></Card>
      </aside>
    </div>

    <section aria-label="Empty state example" className="sr-only"><h2>No repositories yet</h2><p>Create a repository to give your team and agents a shared place to work.</p><Button disabled>Creating repository…</Button></section>
  </div>;
}
