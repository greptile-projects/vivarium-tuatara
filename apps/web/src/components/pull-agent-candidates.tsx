"use client";

import { useEffect, useState } from "react";
import { api } from "../lib/api";
import { Badge, Card } from "./ui";

type Metrics={samples:number;task_success_rate:number;policy_adherence_rate:number;human_corrections:number;mean_uncertainty:number;mean_latency_ms:number;mean_cost:number};
type Candidate={id:string;pull_revision:string;project_version:number;stale:boolean;runs:{id:string;contaminated:boolean;nondeterministic:boolean}[];comparison:{baseline_candidate_id?:string;comparable_suites:string[];invalidated_suites:string[];candidate:Metrics;delta:Metrics;contaminated:boolean;nondeterministic:boolean}};
const pct=(v:number)=>`${Math.round(v*100)}%`;

export function PullAgentCandidates({repositoryID,pullRequestID}:{repositoryID:string;pullRequestID:string}) {
  const [items,setItems]=useState<Candidate[]>([]);const [unavailable,setUnavailable]=useState(false);
  useEffect(()=>{void api<{candidates:Candidate[]}>(`/repositories/${repositoryID}/pulls/${pullRequestID}/agent-candidates`).then(x=>setItems(x.candidates)).catch(()=>setUnavailable(true));},[repositoryID,pullRequestID]);
  if(unavailable)return <p className="text-sm text-[var(--muted)]">Agent candidate evidence is temporarily unavailable.</p>;
  if(!items.length)return null;
  return <section id="agent-candidates" className="scroll-mt-24 space-y-4" aria-label="Agent behavior candidates"><h2 className="text-lg font-semibold">Agent behavior candidates</h2>{items.map(c=><Card className="p-5" key={c.id}><div className="flex flex-wrap gap-2"><Badge tone={c.stale?"warning":"success"}>{c.stale?"stale revision":"exact revision"}</Badge><Badge>{c.pull_revision.slice(0,12)}</Badge><Badge>contract v{c.project_version}</Badge>{c.comparison.contaminated&&<Badge tone="danger">contaminated</Badge>}{c.comparison.nondeterministic&&<Badge tone="warning">nondeterministic</Badge>}</div><div className="mt-4 grid gap-3 sm:grid-cols-3"><Metric label="Task success" value={pct(c.comparison.candidate.task_success_rate)} delta={c.comparison.delta.task_success_rate}/><Metric label="Policy adherence" value={pct(c.comparison.candidate.policy_adherence_rate)} delta={c.comparison.delta.policy_adherence_rate}/><Metric label="Human corrections" value={c.comparison.candidate.human_corrections.toFixed(1)} delta={-c.comparison.delta.human_corrections}/><Metric label="Uncertainty" value={pct(c.comparison.candidate.mean_uncertainty)} delta={-c.comparison.delta.mean_uncertainty}/><Metric label="Mean latency" value={`${Math.round(c.comparison.candidate.mean_latency_ms)} ms`} delta={-c.comparison.delta.mean_latency_ms}/><Metric label="Mean cost" value={`$${c.comparison.candidate.mean_cost.toFixed(2)}`} delta={-c.comparison.delta.mean_cost}/></div><p className="mt-4 text-xs text-[var(--muted)]">{c.comparison.candidate.samples} retained attempts · {c.runs.length} isolated runs · {c.comparison.comparable_suites.length} comparable suites{c.comparison.invalidated_suites.length?` · ${c.comparison.invalidated_suites.length} changed suites excluded`:""}. Statistical limits remain attached to each run.</p></Card>)}</section>;
}
function Metric({label,value,delta}:{label:string;value:string;delta:number}){return <div className="rounded-lg border border-[var(--line)] p-3"><p className="text-xs text-[var(--muted)]">{label}</p><p className="mt-1 font-semibold">{value}</p>{delta!==0&&<p className={`text-xs ${delta>0?"text-emerald-700":"text-red-700"}`}>{delta>0?"improved":"regressed"} vs baseline</p>}</div>}
