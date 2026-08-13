"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Card } from "./ui";

type Evaluation = { id:string; revision:string; goal_id:string; investigation_id:string; baseline_trial_id:string; candidate_trial_id:string; affected_scenarios:string[]; commands:string[]; correctness_checks:{name:string;command:string;passed:boolean;summary:string}[]; residual_risks:string[]; created_by:string; confidence:number; comparisons:{metric:string;unit:string;baseline_mean:number;current_mean:number;change_percent:number;comparable:boolean;reason?:string}[]; resource_changes:Record<string,number>; cost_change_percent:number; correctness_passed:boolean; stale:boolean };

export function PullPerformanceEvaluations({repositoryID,pullRequestID}:{repositoryID:string;pullRequestID:string}) {
  const {token}=useAuth(); const [items,setItems]=useState<Evaluation[]>([]); const [unavailable,setUnavailable]=useState(false);
  useEffect(()=>{ void api<{evaluations:Evaluation[]}>(`/repositories/${repositoryID}/pulls/${pullRequestID}/performance-evaluations`,{},token).then(x=>setItems(x.evaluations)).catch(()=>setUnavailable(true)); },[repositoryID,pullRequestID,token]);
  if (unavailable) return <p className="text-sm text-[var(--muted)]">Performance evaluation evidence is temporarily unavailable.</p>;
  if (!items.length) return null;
  return <section id="performance" className="scroll-mt-24 space-y-4" aria-label="Performance evaluations">
    <h2 className="text-lg font-semibold">Performance evaluations</h2>
    {items.map(item=><Card className="p-5" key={item.id}>
      <div className="flex flex-wrap items-center gap-2"><Badge tone={item.stale?"warning":item.correctness_passed?"success":"danger"}>{item.stale?"stale revision":item.correctness_passed?"correctness passed":"correctness failed"}</Badge><span className="font-mono text-xs text-[var(--muted)]">{item.revision.slice(0,12)}</span><span className="text-sm">{(item.confidence*100).toFixed(1)}% statistical confidence</span></div>
      <div className="mt-4 grid gap-3 sm:grid-cols-2">{item.comparisons.map(value=><div className="rounded-lg bg-[var(--surface)] p-3" key={`${value.metric}:${value.unit}`}><p className="font-semibold">{value.metric}</p>{value.comparable?<p className="text-sm">{value.baseline_mean.toFixed(2)} → {value.current_mean.toFixed(2)} {value.unit} ({value.change_percent.toFixed(1)}%)</p>:<p className="text-sm text-[var(--danger)]">Not comparable: {value.reason}</p>}</div>)}</div>
      <p className="mt-4 text-sm"><strong>Resources:</strong> CPU {item.resource_changes.cpu_seconds_percent.toFixed(1)}%, memory {item.resource_changes.peak_memory_mb_percent.toFixed(1)}%, cost {item.cost_change_percent.toFixed(1)}%</p>
      <p className="mt-2 text-sm"><strong>Affected scenarios:</strong> {item.affected_scenarios.join(", ")}</p>
      <div className="mt-3 space-y-1">{item.correctness_checks.map(check=><p className="text-sm" key={check.name}>{check.passed?"✓":"✕"} <strong>{check.name}</strong> — <code>{check.command}</code>: {check.summary}</p>)}</div>
      <p className="mt-3 text-sm"><strong>Commands:</strong> {item.commands.map(x=><code className="ml-2" key={x}>{x}</code>)}</p>
      <p className="mt-2 text-sm"><strong>Residual risks:</strong> {item.residual_risks.length?item.residual_risks.join("; "):"None declared"}</p>
      <p className="mt-3 text-xs text-[var(--muted)]">Goal {item.goal_id.slice(0,8)} · diagnosis {item.investigation_id.slice(0,8)} · baseline {item.baseline_trial_id.slice(0,8)} · candidate {item.candidate_trial_id.slice(0,8)} · authored by {item.created_by}</p>
    </Card>)}
  </section>;
}
