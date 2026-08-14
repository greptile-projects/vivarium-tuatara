"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { useAuth } from "@/components/auth";
import { Badge, Button, Card } from "@/components/ui";
import { api } from "@/lib/api";

type Terms = {
  title: string;
  source: { kind: string; id: string; revision: string; visibility: string };
  scope: string;
  acceptance_criteria: string[];
  evidence_requirements: string[];
  budget: number;
  deadline: string;
  contributor_eligibility: string[];
  allocation_method: string;
  cancellation_terms: string;
  dependencies: string[];
  risks: string[];
  conflicts: string[];
  milestones: { id: string; title: string; budget: number; acceptance_criteria: string[]; evidence_requirements: string[]; dependencies: string[] }[];
};
type Outcome = {
  id: string; fund_id: string; version: number; status: string; pledged: number;
  milestone_pledged: Record<string, number>;
  revisions: { version: number; terms: Terms; actor_id: string; reason: string; created_at: string }[];
  pledges: { id: string; backer_id: string; amount: number; milestone_id?: string; status: string; note: string }[];
  replans: { kind: string; actor_id: string; reason: string; created_at: string }[];
  diagnostics: { kind: string; message: string }[];
  authority_note: string;
};

const lines = (v: FormDataEntryValue | null) => String(v || "").split("\n").map((x) => x.trim()).filter(Boolean);

export function OutcomeFundingWorkspace({ repositoryID }: { repositoryID: string }) {
  const { token, user } = useAuth();
  const [outcomes, setOutcomes] = useState<Outcome[]>([]);
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    try { setOutcomes((await api<{ funded_outcomes: Outcome[] }>(`/repositories/${repositoryID}/funded-outcomes`, {}, token)).funded_outcomes); setError(""); }
    catch (e) { setError(e instanceof Error ? e.message : "Unable to load funded outcomes"); }
  }, [repositoryID, token]);
  useEffect(() => { void Promise.resolve().then(load); }, [load]);

  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); const form = e.currentTarget; const d = new FormData(form);
    const milestones = lines(d.get("milestones")).map((row) => { const [id, title, budget, criteria, evidence, dependencies = ""] = row.split("|").map((x) => x.trim()); return { id, title, budget: Number(budget), acceptance_criteria: criteria.split(";").filter(Boolean), evidence_requirements: evidence.split(";").filter(Boolean), dependencies: dependencies.split(";").filter(Boolean) }; });
    const terms: Terms = {
      title: String(d.get("title")), source: { kind: String(d.get("source_kind")), id: String(d.get("source_id")), revision: String(d.get("source_revision")), visibility: String(d.get("visibility")) },
      scope: String(d.get("scope")), acceptance_criteria: lines(d.get("criteria")), evidence_requirements: lines(d.get("evidence")), budget: Number(d.get("budget")), deadline: new Date(String(d.get("deadline"))).toISOString(), contributor_eligibility: lines(d.get("eligibility")), allocation_method: String(d.get("allocation")), cancellation_terms: String(d.get("cancellation")), dependencies: lines(d.get("dependencies")), risks: lines(d.get("risks")), conflicts: lines(d.get("conflicts")), milestones,
    };
    try { await api(`/repositories/${repositoryID}/funded-outcomes`, { method: "POST", body: JSON.stringify({ fund_id: d.get("fund_id"), terms }) }, token); form.reset(); await load(); }
    catch (x) { setError(x instanceof Error ? x.message : "Unable to publish funding contract"); }
  }

  async function pledge(e: FormEvent<HTMLFormElement>, outcome: Outcome) {
    e.preventDefault(); const form = e.currentTarget; const d = new FormData(form);
    try { await api(`/repositories/${repositoryID}/funded-outcomes/${outcome.id}/pledges`, { method: "POST", body: JSON.stringify({ expected_version: outcome.version, amount: Number(d.get("amount")), milestone_id: d.get("milestone_id"), idempotency_key: crypto.randomUUID(), note: d.get("note") }) }, token); form.reset(); await load(); }
    catch (x) { setError(x instanceof Error ? x.message : "Unable to record pledge"); }
  }

  async function changePledge(outcome: Outcome, pledgeID: string, action: "withdraw" | "reconfirm") {
    const reason = action === "withdraw" ? "Backing withdrawn under the declared cancellation terms." : "Backer accepts the current revised scope and evidence terms.";
    try { await api(`/repositories/${repositoryID}/funded-outcomes/${outcome.id}/pledges/${pledgeID}`, { method: "POST", body: JSON.stringify({ expected_version: outcome.version, action, reason }) }, token); await load(); }
    catch (x) { setError(x instanceof Error ? x.message : "Unable to update pledge"); }
  }

  async function cancel(e: FormEvent<HTMLFormElement>, outcome: Outcome) {
    e.preventDefault(); const form = e.currentTarget; const d = new FormData(form);
    try { await api(`/repositories/${repositoryID}/funded-outcomes/${outcome.id}/cancel`, { method: "POST", body: JSON.stringify({ expected_version: outcome.version, reason: d.get("reason") }) }, token); await load(); }
    catch (x) { setError(x instanceof Error ? x.message : "Unable to cancel funding"); }
  }

  return <main id="main-content" className="mx-auto max-w-6xl space-y-6 p-6">
    <div><Badge>Evaluable outcomes</Badge><h1 className="mt-2 text-3xl font-semibold">Outcome funding</h1><p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">Decide whether funded work is worth pursuing from its exact scope, acceptance evidence, backing, dependencies, risks, and cancellation terms.</p></div>
    {user && <Card className="p-5"><h2 className="text-lg font-semibold">Publish a funding contract</h2><p className="mt-1 text-xs text-[var(--muted)]">Amounts use the linked fund&apos;s minor unit. Lists use one item per line. Milestones use <code>id | title | amount | criterion;criterion | evidence;evidence | dependency IDs</code>.</p>
      <form onSubmit={create} className="mt-4 grid gap-3 md:grid-cols-3">
        <Field name="title" label="Outcome title"/><Field name="fund_id" label="Governed fund ID"/><Select name="source_kind" label="Outcome source" values={["issue","roadmap_outcome","proposal","stewardship_opportunity","incident_follow_up","security_repair"]}/>
        <Field name="source_id" label="Source ID"/><Field name="source_revision" label="Exact source revision"/><Select name="visibility" label="Visibility" values={["public","participants","embargoed"]}/>
        <Field name="budget" label="Budget (minor units)" type="number"/><Field name="deadline" label="Deadline" type="datetime-local"/><Select name="allocation" label="Allocation method" values={["milestone_claim","first_accepted","proportional","maintainer_selection"]}/>
        <Area name="scope" label="Bounded scope"/><Area name="criteria" label="Acceptance criteria"/><Area name="evidence" label="Required evidence"/><Area name="eligibility" label="Contributor eligibility"/><Area name="dependencies" label="Dependencies"/><Area name="risks" label="Risks"/><Area name="conflicts" label="Known conflicts"/><Area name="cancellation" label="Cancellation and return terms"/><Area name="milestones" label="Declared milestones"/>
        <Button>Publish exact terms</Button>
      </form></Card>}
    {error && <p role="alert" className="text-sm text-[var(--danger)]">{error}</p>}
    {outcomes.map((outcome) => { const revision = outcome.revisions.at(-1)!; const terms = revision.terms; return <Card key={outcome.id} className="p-5">
      <div className="flex flex-wrap items-center gap-2"><h2 className="text-xl font-semibold">{terms.title}</h2><Badge tone={outcome.status === "open" ? "success" : "neutral"}>{outcome.status}</Badge><Badge>v{outcome.version}</Badge><Badge>{terms.source.kind.replaceAll("_", " ")} · {terms.source.id}@{terms.source.revision}</Badge></div>
      <p className="mt-3">{terms.scope}</p><div className="mt-4 grid gap-4 md:grid-cols-3"><Fact title="Backing" values={[`${outcome.pledged} / ${terms.budget} minor units`, `Deadline ${new Date(terms.deadline).toLocaleString()}`, terms.allocation_method.replaceAll("_", " ")]}/><Fact title="Acceptance criteria" values={terms.acceptance_criteria}/><Fact title="Evidence required" values={terms.evidence_requirements}/><Fact title="Eligible contributors" values={terms.contributor_eligibility}/><Fact title="Dependencies" values={terms.dependencies}/><Fact title="Risks and conflicts" values={[...terms.risks, ...terms.conflicts]}/></div>
      <p className="mt-4 rounded-lg bg-[var(--surface-sunken)] p-3 text-sm"><strong>Cancellation:</strong> {terms.cancellation_terms}</p>
      {terms.milestones.length > 0 && <div className="mt-4"><h3 className="font-semibold">Milestones</h3>{terms.milestones.map((m) => <div key={m.id} className="mt-2 rounded-lg border p-3 text-sm"><strong>{m.title}</strong> · {outcome.milestone_pledged[m.id] || 0} / {m.budget}<span className="block text-xs text-[var(--muted)]">Accept: {m.acceptance_criteria.join("; ")} · Evidence: {m.evidence_requirements.join("; ")}{m.dependencies.length ? ` · After ${m.dependencies.join(", ")}` : ""}</span></div>)}</div>}
      {outcome.diagnostics.length > 0 && <div className="mt-4 flex flex-wrap gap-2">{outcome.diagnostics.map((d) => <span key={d.kind} title={d.message}><Badge tone="warning">{d.kind.replaceAll("_", " ")}</Badge></span>)}</div>}
      {outcome.pledges.length > 0 && <div className="mt-4"><h3 className="font-semibold">Backing</h3>{outcome.pledges.map((p) => <div key={p.id} className="mt-2 flex flex-wrap items-center gap-2 rounded-lg border p-3 text-sm"><Badge tone={p.status === "active" ? "success" : "warning"}>{p.status.replaceAll("_", " ")}</Badge><span>{p.amount} · {p.milestone_id || "whole outcome"} · {p.backer_id}</span>{p.backer_id === user?.id && p.status === "reconfirmation_required" && <Button type="button" variant="secondary" onClick={() => void changePledge(outcome, p.id, "reconfirm")}>Reconfirm current terms</Button>}{p.backer_id === user?.id && p.status !== "withdrawn" && <Button type="button" variant="quiet" onClick={() => void changePledge(outcome, p.id, "withdraw")}>Withdraw backing</Button>}</div>)}</div>}
      {user && outcome.status === "open" && <form onSubmit={(e) => pledge(e, outcome)} className="mt-5 grid gap-3 rounded-lg border p-4 md:grid-cols-4"><Field name="amount" label="Pledge amount" type="number"/><label className="text-xs font-semibold">Pledge target<select name="milestone_id" className="mt-1 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"><option value="">Whole outcome</option>{terms.milestones.map((m) => <option key={m.id} value={m.id}>{m.title}</option>)}</select></label><Field name="note" label="Backing rationale"/><Button>Back declared terms</Button></form>}
      {user && outcome.status === "open" && <details className="mt-4"><summary className="cursor-pointer text-sm font-semibold text-[var(--danger)]">Cancel and replan</summary><form onSubmit={(e) => cancel(e, outcome)} className="mt-2 flex flex-wrap gap-2"><Field name="reason" label="Attributed cancellation reason"/><Button variant="secondary">Apply cancellation terms</Button></form></details>}
      <details className="mt-4"><summary className="cursor-pointer font-semibold">Attributable replanning · {outcome.replans.length}</summary>{outcome.replans.map((r, i) => <p key={`${r.created_at}-${i}`} className="mt-2 text-sm"><Badge>{r.kind.replaceAll("_", " ")}</Badge> {r.reason} · {r.actor_id}</p>)}</details><p className="mt-4 text-xs text-[var(--muted)]">{outcome.authority_note}</p>
    </Card>; })}
  </main>;
}

function Fact({ title, values }: { title: string; values: string[] }) { return <div><h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">{title}</h3>{values.length ? <ul className="mt-1 list-disc pl-5 text-sm">{values.map((x, i) => <li key={`${x}-${i}`}>{x}</li>)}</ul> : <p className="mt-1 text-sm text-[var(--muted)]">None declared</p>}</div>; }
function Field({ name, label, type = "text" }: { name: string; label: string; type?: string }) { return <label className="text-xs font-semibold">{label}<input required name={name} type={type} className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal"/></label>; }
function Area({ name, label }: { name: string; label: string }) { return <label className="text-xs font-semibold">{label}<textarea required name={name} rows={3} className="mt-1 w-full rounded-lg border p-3 font-normal"/></label>; }
function Select({ name, label, values }: { name: string; label: string; values: string[] }) { return <label className="text-xs font-semibold">{label}<select name={name} className="mt-1 min-h-10 w-full rounded-lg border bg-white px-3 font-normal">{values.map((v) => <option key={v} value={v}>{v.replaceAll("_", " ")}</option>)}</select></label>; }
