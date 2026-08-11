"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, type ContributionMatch, type ContributorPathway, type ContributorPathwayResponse, type Repository } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

const blank = {
  goals: "", prerequisites: "", conduct: "", security: "", setup: "", commands: "",
  communication: "", review: "", categories: "human_or_agent | Good first changes | Bounded improvements with clear verification",
  requirements: "ownership | Project ownership |",
};

export function ContributorPathwayWorkspace({ repositoryId }: { repositoryId: string }) {
  const { token, user, loading: authLoading } = useAuth();
  const [repository, setRepository] = useState<Repository | null>(null);
  const [data, setData] = useState<ContributorPathwayResponse | null>(null);
  const [form, setForm] = useState(blank);
  const [editing, setEditing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    if (authLoading) return;
    setLoading(true); setError("");
    try {
      const repo = await api<Repository>(`/repositories/${repositoryId}`, {}, token); setRepository(repo);
      try { setData(await api<ContributorPathwayResponse>(`/repositories/${repositoryId}/contributor-pathway`, {}, token)); }
      catch (reason) { if (!(reason instanceof Error) || !reason.message.includes("has not been published")) throw reason; setData(null); }
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Contributor guidance could not be loaded."); }
    finally { setLoading(false); }
  }, [authLoading, repositoryId, token]);
  useEffect(() => { void Promise.resolve().then(load); }, [load]);

  function beginEdit() {
    const p = data?.pathway;
    setForm(p ? {
      goals:p.goals, prerequisites:p.prerequisites.join("\n"), conduct:p.conduct, security:p.security,
      setup:p.setup.summary, commands:p.setup.verification_commands.join("\n"), communication:p.communication, review:p.review_policy,
      categories:p.work_categories.map((v) => `${v.audience} | ${v.name} | ${v.description}`).join("\n"),
      requirements:p.requirements.map((v) => `${v.kind} | ${v.label} | ${v.path || v.resource_id || ""}${v.revision ? ` | ${v.revision}` : ""}`).join("\n"),
    } : blank); setEditing(true);
  }
  async function publish(event: FormEvent) {
    event.preventDefault(); setPending(true); setError("");
    try {
      const categories = rows(form.categories).map(([audience, name, description]) => ({ audience, name, description }));
      const requirements = rows(form.requirements).map(([kind, label, target, revision]) => ({ kind, label, ...(kind === "documentation" ? { path:target } : kind === "ownership" ? {} : { resource_id:target }), ...(revision ? { revision } : {}) }));
      await api(`/repositories/${repositoryId}/contributor-pathway`, { method:"PUT", body:JSON.stringify({ expected_version:data?.pathway.version ?? 0, pathway:{ goals:form.goals, prerequisites:lines(form.prerequisites), conduct:form.conduct, security:form.security, setup:{ summary:form.setup, verification_commands:lines(form.commands) }, communication:form.communication, review_policy:form.review, work_categories:categories, requirements } }) }, token);
      setEditing(false); await load();
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Contributor guidance could not be published."); }
    finally { setPending(false); }
  }
  async function acknowledge() {
    if (!data) return; setPending(true); setError("");
    try { await api(`/repositories/${repositoryId}/contributor-pathway/acknowledgements`, { method:"POST", body:JSON.stringify({ version:data.pathway.version }) }, token); await load(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Acknowledgement could not be saved."); }
    finally { setPending(false); }
  }
  if (loading) return <Card className="p-8 text-sm text-[var(--muted)]">Loading contributor guidance…</Card>;
  const owner = repository?.owner_id === user?.id;
  const acknowledged = !!data?.acknowledgements.some((v) => v.version === data.pathway.version && v.actor_id === user?.id);
  return <div className="space-y-6">
    <header className="flex flex-wrap items-end justify-between gap-4"><div><Link href={`/repositories/${repositoryId}`} className="text-sm text-[var(--muted)] hover:text-[var(--brand)]">{repository?.name ?? "Repository"}</Link><h1 className="mt-2 text-2xl font-semibold">How to contribute</h1><p className="mt-2 max-w-2xl text-sm text-[var(--muted)]">Decide whether this project fits, then arrive with its shared expectations in hand.</p></div>{owner && !editing && <Button onClick={beginEdit}>{data ? "Publish a new version" : "Publish pathway"}</Button>}</header>
    {error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}
    {editing ? <Editor form={form} setForm={setForm} publish={publish} pending={pending} cancel={() => setEditing(false)} /> : data ? <Pathway pathway={data.pathway} /> : <Card className="p-8"><h2 className="font-semibold">No contributor pathway published</h2><p className="mt-2 text-sm text-[var(--muted)]">Maintainers have not yet gathered the project&apos;s participation expectations here.</p></Card>}
    {data && <Card className="flex flex-wrap items-center justify-between gap-4 p-5"><div><p className="font-semibold">Revision {data.pathway.version}</p><p className="text-sm text-[var(--muted)]">Published {new Date(data.pathway.published_at).toLocaleString()} · {data.acknowledgement_count} acknowledgement(s)</p></div>{token && !acknowledged && <Button disabled={pending} onClick={acknowledge}>I understand these expectations</Button>}{acknowledged && <Badge tone="success">Acknowledged</Badge>}</Card>}
    {data && data.history.length > 1 && <Card className="p-5"><h2 className="font-semibold">Version history</h2><ol className="mt-3 space-y-2 text-sm">{[...data.history].reverse().map((v) => <li key={v.id}>Revision {v.version} · {new Date(v.published_at).toLocaleString()} · by <code>{v.published_by.slice(0, 8)}</code></li>)}</ol></Card>}
    <OpportunityMatcher repositoryId={repositoryId} token={token} userId={user?.id} />
  </div>;
}

function OpportunityMatcher({ repositoryId, token, userId }: { repositoryId:string; token:string | null; userId?:string }) {
  const [skills,setSkills]=useState(""); const [interests,setInterests]=useState(""); const [minutes,setMinutes]=useState(240); const [risk,setRisk]=useState("medium"); const [agent,setAgent]=useState(true);
  const [matches,setMatches]=useState<ContributionMatch[]>([]); const [pending,setPending]=useState(false); const [error,setError]=useState("");
  const find=async(e:FormEvent)=>{e.preventDefault();setPending(true);setError("");try{const v=await api<{matches:ContributionMatch[]}>(`/repositories/${repositoryId}/contribution-opportunity-matches`,{method:"POST",body:JSON.stringify({skills:csv(skills),interests:csv(interests),available_minutes:minutes,maximum_risk:risk,agent_assistance:agent})},token);setMatches(v.matches)}catch(reason){setError(reason instanceof Error?reason.message:"Matches could not be loaded.")}finally{setPending(false)}};
  const claim=async(m:ContributionMatch)=>{setPending(true);setError("");try{await api(`/repositories/${repositoryId}/contribution-opportunities/${m.opportunity.id}/claim`,{method:"POST",body:JSON.stringify({expected_version:m.opportunity.version,hours:72,note:"Reserved from contributor matching"})},token); await rematch()}catch(reason){setError(reason instanceof Error?reason.message:"Opportunity could not be reserved.")}finally{setPending(false)}};
  const release=async(m:ContributionMatch)=>{setPending(true);setError("");try{await api(`/repositories/${repositoryId}/contribution-opportunities/${m.opportunity.id}/release`,{method:"POST",body:JSON.stringify({expected_version:m.opportunity.version})},token);await rematch()}catch(reason){setError(reason instanceof Error?reason.message:"Opportunity could not be released.")}finally{setPending(false)}};
  const launch=async(m:ContributionMatch)=>{setPending(true);setError("");try{const result=await api<{workspace:{id:string}}>(`/repositories/${repositoryId}/contribution-opportunities/${m.opportunity.id}/launch`,{method:"POST",body:JSON.stringify({expected_version:m.opportunity.version,fork_name:`${m.opportunity.title.toLowerCase().replace(/[^a-z0-9]+/g,"-").replace(/^-|-$/g,"").slice(0,48)||"contribution"}-fork`,sample_attachment_ids:[]})},token);window.location.assign(`/workspaces/${result.workspace.id}`)}catch(reason){setError(reason instanceof Error?reason.message:"Contribution workspace could not be launched.")}finally{setPending(false)}};
  const rematch=async()=>{const v=await api<{matches:ContributionMatch[]}>(`/repositories/${repositoryId}/contribution-opportunity-matches`,{method:"POST",body:JSON.stringify({skills:csv(skills),interests:csv(interests),available_minutes:minutes,maximum_risk:risk,agent_assistance:agent})},token);setMatches(v.matches)};
  return <section className="space-y-4"><div><h2 className="text-xl font-semibold">Find ready work</h2><p className="mt-1 text-sm text-[var(--muted)]">Describe what fits. Reservations prevent duplicate effort but grant no repository access.</p></div>
    <Card className="p-5"><form onSubmit={find} className="grid gap-4 md:grid-cols-2"><label className="text-sm font-semibold">Skills<input value={skills} onChange={e=>setSkills(e.target.value)} placeholder="Go, testing, documentation" className="mt-1 w-full rounded-lg border border-[var(--line-strong)] p-3 font-normal"/></label><label className="text-sm font-semibold">Interests<input value={interests} onChange={e=>setInterests(e.target.value)} placeholder="accessibility, API design" className="mt-1 w-full rounded-lg border border-[var(--line-strong)] p-3 font-normal"/></label><label className="text-sm font-semibold">Available minutes<input type="number" min={15} max={10080} value={minutes} onChange={e=>setMinutes(Number(e.target.value))} className="mt-1 w-full rounded-lg border border-[var(--line-strong)] p-3 font-normal"/></label><label className="text-sm font-semibold">Maximum risk<select value={risk} onChange={e=>setRisk(e.target.value)} className="mt-1 w-full rounded-lg border border-[var(--line-strong)] p-3 font-normal"><option>low</option><option>medium</option><option>high</option></select></label><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={agent} onChange={e=>setAgent(e.target.checked)}/>Show work with agent assistance</label><div><Button disabled={pending} type="submit">{pending?"Matching…":"Suggest work"}</Button></div></form></Card>
    {error&&<p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}
    <div className="space-y-3">{matches.map(m=><Card key={m.opportunity.id} className="p-5"><div className="flex flex-wrap items-start justify-between gap-3"><div><div className="flex flex-wrap gap-2"><h3 className="font-semibold">{m.opportunity.title}</h3><Badge tone={m.ready?"success":"warning"}>{m.ready?"ready":"not ready"}</Badge><Badge tone="info">{m.score} match</Badge></div><p className="mt-2 text-sm">{m.opportunity.expected_outcome}</p><p className="mt-2 text-xs text-[var(--muted)]">{m.opportunity.estimated_minutes} min · {m.opportunity.risk} risk · revision <code>{m.opportunity.revision.slice(0,12)}</code> · from {m.opportunity.source.kind}</p></div><div className="flex flex-wrap gap-2">{token&&m.ready&&!m.opportunity.claim&&<Button disabled={pending} onClick={()=>claim(m)}>Reserve for 72 hours</Button>}{m.opportunity.claim?.actor_id===userId&&<><Button disabled={pending} onClick={()=>launch(m)}>Fork and launch workspace</Button><Button variant="secondary" disabled={pending} onClick={()=>release(m)}>Release</Button></>}</div></div><div className="mt-4 grid gap-3 md:grid-cols-2"><div><p className="text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">Why suggested</p><ul className="mt-1 list-disc pl-5 text-sm">{m.reasons.map(x=><li key={x}>{x}</li>)}</ul></div><div><p className="text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">Scope and gaps</p><p className="mt-1 text-sm">{m.opportunity.scope}</p>{m.gaps.length>0&&<ul className="mt-1 list-disc pl-5 text-sm text-[var(--muted)]">{m.gaps.map(x=><li key={x}>{x}</li>)}</ul>}</div></div>{m.opportunity.mentors.length>0&&<p className="mt-3 text-xs text-[var(--muted)]">{m.opportunity.mentors.length} mentor(s) available</p>}</Card>)}</div>
  </section>;
}
function csv(value:string){return value.split(",").map(v=>v.trim()).filter(Boolean)}

function Pathway({ pathway:p }: { pathway: ContributorPathway }) { return <div className="grid gap-5 lg:grid-cols-2">
  <Section title="Project goals"><p>{p.goals}</p></Section><Section title="Prerequisites"><List values={p.prerequisites}/></Section>
  <Section title="Conduct"><p>{p.conduct}</p></Section><Section title="Security guidance"><p>{p.security}</p></Section>
  <Section title="Supported setup"><p>{p.setup.summary}</p><div className="mt-3 space-y-1">{p.setup.verification_commands.map((v) => <code key={v} className="block rounded bg-[var(--surface)] p-2 text-xs">{v}</code>)}</div></Section>
  <Section title="Communication expectations"><p>{p.communication}</p></Section><Section title="Review policy"><p>{p.review_policy}</p></Section>
  <Section title="Ways to help"><div className="space-y-3">{p.work_categories.map((v) => <div key={`${v.audience}-${v.name}`}><div className="flex gap-2"><strong>{v.name}</strong><Badge tone="info">{v.audience.replaceAll("_", " ")}</Badge></div><p className="mt-1 text-sm text-[var(--muted)]">{v.description}</p></div>)}</div></Section>
  <Section title="Current project references"><div className="space-y-3">{p.requirements.map((v, i) => <div key={`${v.kind}-${v.label}-${i}`} className="flex items-start justify-between gap-3"><div><strong>{v.label}</strong><p className="text-xs text-[var(--muted)]">{v.kind.replaceAll("_", " ")} · {v.path || v.resource_id || "current repository owner"}</p>{v.status !== "current" && <p className="mt-1 text-xs text-[var(--danger)]">{v.status_detail}</p>}</div><Badge tone={v.status === "current" ? "success" : v.status === "stale" ? "warning" : "danger"}>{v.status ?? "unknown"}</Badge></div>)}</div></Section>
  </div>; }
function Section({ title, children }: { title:string; children:React.ReactNode }) { return <Card className="p-5"><h2 className="mb-3 text-lg font-semibold">{title}</h2><div className="whitespace-pre-wrap text-sm leading-6">{children}</div></Card>; }
function List({ values }: { values:string[] }) { return <ul className="list-disc space-y-1 pl-5">{values.map((v) => <li key={v}>{v}</li>)}</ul>; }
function Editor({ form, setForm, publish, pending, cancel }: { form:typeof blank; setForm:React.Dispatch<React.SetStateAction<typeof blank>>; publish:(e:FormEvent)=>void; pending:boolean; cancel:()=>void }) { const field=(key:keyof typeof blank,label:string,help?:string)=><label className="block"><span className="text-sm font-semibold">{label}</span>{help&&<span className="ml-2 text-xs text-[var(--muted)]">{help}</span>}<textarea required={key !== "commands" && key !== "requirements"} rows={key === "goals" ? 4 : 3} value={form[key]} onChange={(e)=>setForm((v)=>({...v,[key]:e.target.value}))} className="mt-1 w-full rounded-lg border border-[var(--line-strong)] bg-white p-3 text-sm"/></label>; return <Card className="p-6"><form onSubmit={publish} className="grid gap-5 lg:grid-cols-2">{field("goals","Project goals")}{field("prerequisites","Prerequisites","one per line")}{field("conduct","Conduct expectations")}{field("security","Security guidance")}{field("setup","Supported setup")}{field("commands","Verification commands","one per line; may be empty")}{field("communication","Communication expectations")}{field("review","Review policy")}{field("categories","Ways to help","audience | name | description")}{field("requirements","Project references","kind | label | resource ID or documentation path | optional revision")}<div className="flex gap-3 lg:col-span-2"><Button disabled={pending} type="submit">{pending?"Publishing…":"Publish version"}</Button><Button type="button" variant="secondary" onClick={cancel}>Cancel</Button></div></form></Card>; }
function lines(value:string) { return value.split("\n").map((v)=>v.trim()).filter(Boolean); }
function rows(value:string) { return lines(value).map((line)=>line.split("|").map((v)=>v.trim())); }
