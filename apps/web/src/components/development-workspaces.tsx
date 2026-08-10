"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import { api, APIError, DevelopmentWorkspace, WorkspaceConsumption, WorkspacePolicy } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
import { WorkspaceIDE } from "./workspace-ide";
import { WorkspaceCheckpoints } from "./workspace-checkpoints";

const short = (value: string) => value.slice(0, 8);
export function DevelopmentWorkspaces({
  workspaceID,
}: {
  workspaceID?: string;
}) {
  const { token, user } = useAuth();
  const [items, setItems] = useState<DevelopmentWorkspace[]>([]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const controlsLifecycle = (item: DevelopmentWorkspace) =>
    item.control?.principal_kind === "human" &&
    item.control.principal_id === user?.id &&
    item.control.mode === "execute" &&
    item.control.scopes.includes("lifecycle") &&
    new Date(item.control.expires_at) > new Date();
  useEffect(() => {
    if (!token) return;
    let active = true;
    const request = workspaceID
      ? api<DevelopmentWorkspace>(`/workspaces/${workspaceID}`, {}, token).then(
          (value) => [value],
        )
      : api<{ items: DevelopmentWorkspace[] }>("/workspaces", {}, token).then(
          (value) => value.items,
        );
    void request
      .then((value) => {
        if (active) {
          setError("");
          setItems(value);
        }
      })
      .catch((e) => {
        if (active)
          setError(
            e instanceof APIError
              ? e.message
              : "Workspaces could not be loaded.",
          );
      });
    return () => {
      active = false;
    };
  }, [token, workspaceID]);
  async function transition(
    item: DevelopmentWorkspace,
    target: "suspend" | "resume",
  ) {
    if (!token) return;
    setPending(true);
    try {
      const updated = await api<DevelopmentWorkspace>(
        `/workspaces/${item.id}/${target}`,
        {
          method: "POST",
          body: JSON.stringify({ foundation: item.definition_sha256 }),
        },
        token,
      );
      setItems((current) =>
        current.map((value) => (value.id === updated.id ? updated : value)),
      );
    } catch (e) {
      setError(e instanceof APIError ? e.message : "Lifecycle update failed.");
    } finally {
      setPending(false);
    }
  }
  async function takeLifecycleControl(item: DevelopmentWorkspace) {
    if (!token || !user) return;
    setPending(true);
    try {
      const updated = await api<DevelopmentWorkspace>(
        `/workspaces/${item.id}/control`,
        {
          method: "PUT",
          body: JSON.stringify({
            expected_version: item.control.version,
            principal_kind: "human",
            principal_id: user.id,
            mode: "execute",
            scopes: ["files", "commands", "lifecycle"],
            expires_in: 900,
          }),
        },
        token,
      );
      setItems((current) =>
        current.map((value) => (value.id === updated.id ? updated : value)),
      );
    } catch (e) {
      setError(e instanceof APIError ? e.message : "Control takeover failed.");
    } finally {
      setPending(false);
    }
  }
  return (
    <div className="space-y-4">
      {error && (
        <Card className="border-[var(--danger)] p-4 text-sm text-[var(--danger)]">
          {error}
        </Card>
      )}
      {!workspaceID && (
        <div className="flex justify-end">
          <Link
            href="/workspaces/new"
            className="inline-flex min-h-9 items-center rounded-lg bg-[var(--brand)] px-4 text-sm font-semibold text-white"
          >
            Launch workspace
          </Link>
        </div>
      )}
      {items.map((item) => (
        <Card key={item.id} className="p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div className="flex items-center gap-2">
                <Badge
                  tone={
                    item.state === "running"
                      ? "success"
                      : item.state === "failed"
                        ? "danger"
                        : "warning"
                  }
                >
                  {item.state}
                </Badge>
                <span className="font-mono text-xs text-[var(--muted)]">
                  {short(item.id)}
                </span>
              </div>
              <h2 className="mt-3 font-semibold">
                Revision {short(item.commit_id)}
              </h2>
              <p className="mt-1 text-sm text-[var(--muted)]">
                From {item.source.kind.replaceAll("_", " ")} · created{" "}
                {new Date(item.created_at).toLocaleString()}
              </p>
            </div>
            {!workspaceID && (
              <Link
                className="text-sm font-semibold text-[var(--brand)]"
                href={`/workspaces/${item.id}`}
              >
                Inspect →
              </Link>
            )}
          </div>
          <dl className="mt-5 grid gap-3 text-sm sm:grid-cols-3">
            <div>
              <dt className="text-xs font-semibold text-[var(--muted)]">
                Foundation
              </dt>
              <dd className="mt-1 font-mono">
                sha256:{short(item.definition_sha256)}
              </dd>
            </div>
            <div>
              <dt className="text-xs font-semibold text-[var(--muted)]">
                Runtime
              </dt>
              <dd className="mt-1">{item.definition.image}</dd>
            </div>
            <div>
              <dt className="text-xs font-semibold text-[var(--muted)]">
                Resources
              </dt>
              <dd className="mt-1">
                {item.definition.resources.cpus} CPU ·{" "}
                {item.definition.resources.memory_mb} MB
              </dd>
            </div>
          </dl>
          {workspaceID && (
            <>
              <p className="mt-5 text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">
                Setup evidence
              </p>
              <ol className="mt-2 space-y-2">
                {item.setup_evidence.map((step, index) => (
                  <li
                    key={index}
                    className="rounded-lg bg-[var(--canvas)] p-3 text-xs"
                  >
                    <Badge
                      tone={step.state === "passed" ? "success" : "danger"}
                    >
                      {step.state}
                    </Badge>
                    <code className="ml-2">{step.command}</code>
                    {step.output && (
                      <pre className="mt-2 overflow-auto whitespace-pre-wrap text-[var(--muted)]">
                        {step.output}
                      </pre>
                    )}
                  </li>
                ))}
              </ol>
              <p className="mt-5 text-xs text-[var(--muted)]">
                Effective access: {item.effective_access.role} ·{" "}
                {item.effective_access.scopes.join(", ")}
              </p>
              <WorkspaceGovernance workspace={item} onWorkspace={(updated) => setItems((current) => current.map((value) => value.id === updated.id ? updated : value))} />
              {!controlsLifecycle(item) && (
                <Button
                  className="mt-4"
                  disabled={pending}
                  onClick={() => void takeLifecycleControl(item)}
                >
                  Take lifecycle control
                </Button>
              )}
              {item.state === "running" && (
                <Button
                  className="mt-4"
                  disabled={pending || !controlsLifecycle(item)}
                  onClick={() => void transition(item, "suspend")}
                >
                  Suspend
                </Button>
              )}
              {item.state === "suspended" && (
                <Button
                  className="mt-4"
                  disabled={pending || !controlsLifecycle(item)}
                  onClick={() => void transition(item, "resume")}
                >
                  Resume exact foundation
                </Button>
              )}
            </>
          )}
          {workspaceID && item.state === "running" && (
            <><WorkspaceCheckpoints workspace={item} onWorkspace={(updated) => setItems([updated])} /><WorkspaceIDE workspace={item} onWorkspace={(updated) => setItems([updated])} /></>
          )}
        </Card>
      ))}
    </div>
  );
}

function WorkspaceGovernance({workspace,onWorkspace}:{workspace:DevelopmentWorkspace;onWorkspace:(workspace:DevelopmentWorkspace)=>void}) {
  const {token}=useAuth(); const [usage,setUsage]=useState<WorkspaceConsumption>(); const [currentPolicy,setCurrentPolicy]=useState<WorkspacePolicy>(workspace.policy); const [error,setError]=useState(""); const [pending,setPending]=useState(false);
  const owner=workspace.effective_access.role==="owner";
  useEffect(()=>{if(!token||!owner)return;void Promise.all([api<{items:WorkspaceConsumption[]}>(`/repositories/${workspace.repository_id}/workspace-usage`,{},token),api<WorkspacePolicy>(`/repositories/${workspace.repository_id}/workspace-policy`,{},token)]).then(([v,p])=>{setUsage(v.items.find(x=>x.workspace_id===workspace.id));setCurrentPolicy(p)}).catch(()=>{});},[token,owner,workspace.id,workspace.repository_id]);
  async function savePolicy(event:FormEvent<HTMLFormElement>){event.preventDefault();if(!token)return;setPending(true);const data=new FormData(event.currentTarget);const input:Omit<WorkspacePolicy,"updated_by"|"updated_at">={version:currentPolicy.version,max_cpus:Number(data.get("max_cpus")),max_memory_mb:Number(data.get("max_memory_mb")),max_storage_mb:Number(data.get("max_storage_mb")),network:"none",idle_minutes:Number(data.get("idle_minutes")),max_runtime_hours:Number(data.get("max_runtime_hours")),retention_hours:Number(data.get("retention_hours")),sharing:String(data.get("sharing")) as WorkspacePolicy["sharing"],agent_execution:data.get("agent_execution")==="on"};try{setCurrentPolicy(await api<WorkspacePolicy>(`/repositories/${workspace.repository_id}/workspace-policy`,{method:"PUT",body:JSON.stringify({...input,expected_version:currentPolicy.version})},token));}catch(e){setError(e instanceof APIError?e.message:"Policy could not be saved.");}finally{setPending(false)}}
  async function act(kind:"stop"|"expiry"){if(!token)return;setPending(true);try{const body=kind==="expiry"?{expires_at:new Date(Date.now()+24*60*60*1000).toISOString(),reason:"Owner-announced expiry; export unpublished work before this time."}:{reason:"Stopped by repository owner"};onWorkspace(await api<DevelopmentWorkspace>(`/workspaces/${workspace.id}/${kind}`,{method:"POST",body:JSON.stringify(body)},token));}catch(e){setError(e instanceof APIError?e.message:"Workspace governance update failed.")}finally{setPending(false)}}
  return <section className="mt-5 rounded-lg border border-[var(--line)] p-4"><h3 className="font-semibold">Governance and consumption</h3><p className="mt-1 text-xs text-[var(--muted)]">Launch policy {workspace.policy_scope} v{workspace.policy_version} · {workspace.policy.sharing} sharing · network {workspace.policy.network} · idle after {workspace.policy.idle_minutes} minutes</p>{workspace.rebuild_required&&<p className="mt-2 text-sm text-[var(--warning)]">Rebuild required: {workspace.rebuild_reasons.join("; ")}</p>}{workspace.expires_at&&<p className="mt-2 text-sm">Expiry: {new Date(workspace.expires_at).toLocaleString()}{workspace.expiry_announced_at?" (announced)":""}</p>}{workspace.stop_reason&&<p className="mt-2 text-sm text-[var(--danger)]">{workspace.stop_reason}</p>}{usage&&<dl className="mt-3 grid gap-2 text-xs sm:grid-cols-3"><div><dt>CPU consumed</dt><dd>{Math.round(usage.cpu_seconds)} seconds</dd></div><div><dt>Memory reserved</dt><dd>{usage.memory_mb_hours.toFixed(1)} MB-hours</dd></div><div><dt>Storage reserved</dt><dd>{usage.storage_mb_hours.toFixed(1)} MB-hours</dd></div></dl>}{owner&&<><form onSubmit={savePolicy} className="mt-4 grid gap-2 sm:grid-cols-3">{[["max_cpus","Max CPUs",currentPolicy.max_cpus],["max_memory_mb","Memory MB",currentPolicy.max_memory_mb],["max_storage_mb","Storage MB",currentPolicy.max_storage_mb],["idle_minutes","Idle minutes",currentPolicy.idle_minutes],["max_runtime_hours","Runtime hours",currentPolicy.max_runtime_hours],["retention_hours","Retention hours",currentPolicy.retention_hours]].map(([name,label,value])=><label className="text-xs font-semibold" key={String(name)}>{label}<input name={String(name)} type="number" step={name==="max_cpus"?"0.25":"1"} defaultValue={value} className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-normal"/></label>)}<label className="text-xs font-semibold">Sharing<select name="sharing" defaultValue={currentPolicy.sharing} className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-normal"><option value="private">Creator only</option><option value="repository">Repository collaborators</option><option value="organization">Organization</option></select></label><label className="flex items-center gap-2 text-xs font-semibold"><input name="agent_execution" type="checkbox" defaultChecked={currentPolicy.agent_execution}/> Allow approved agents</label><Button disabled={pending}>Save repository policy</Button></form><div className="mt-3 flex gap-2"><Button disabled={pending||!!workspace.expiry_announced_at} onClick={()=>void act("expiry")}>Announce 24-hour expiry</Button><Button disabled={pending||workspace.state==="stopped"||workspace.state==="expired"} onClick={()=>void act("stop")}>Stop compute</Button></div></>}{error&&<p role="alert" className="mt-2 text-sm text-[var(--danger)]">{error}</p>}</section>
}

export function WorkspaceLauncher() {
  const { token } = useAuth();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    const data = new FormData(event.currentTarget);
    const kind = String(data.get("kind"));
    const source: Record<string, string> = { kind };
    for (const key of [
      "proposal_id",
      "task_id",
      "pull_request_id",
      "incident_id",
      "repair_id",
    ]) {
      const value = String(data.get(key) || "").trim();
      if (value) source[key] = value;
    }
    try {
      const created = await api<DevelopmentWorkspace>(
        "/workspaces",
        {
          method: "POST",
          body: JSON.stringify({
            repository_id: String(data.get("repository_id")),
            commit_id: String(data.get("commit_id")),
            source,
          }),
        },
        token,
      );
      location.href = `/workspaces/${created.id}`;
    } catch (e) {
      setError(
        e instanceof APIError ? e.message : "Workspace could not be launched.",
      );
      setPending(false);
    }
  }
  return (
    <Card className="p-6">
      <form onSubmit={submit} className="grid gap-4">
        <label className="text-sm font-semibold">
          Repository ID
          <input
            required
            name="repository_id"
            className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-mono font-normal"
          />
        </label>
        <label className="text-sm font-semibold">
          Exact commit
          <input
            required
            name="commit_id"
            className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-mono font-normal"
          />
        </label>
        <label className="text-sm font-semibold">
          Shared context
          <select
            name="kind"
            className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-normal"
          >
            <option value="repository">Repository</option>
            <option value="proposal_task">Proposal task</option>
            <option value="pull_request">Pull request</option>
            <option value="incident_repair">Incident repair</option>
          </select>
        </label>
        <div className="grid gap-3 sm:grid-cols-2">
          {[
            "proposal_id",
            "task_id",
            "pull_request_id",
            "incident_id",
            "repair_id",
          ].map((name) => (
            <label key={name} className="text-xs font-semibold capitalize">
              {name.replaceAll("_", " ")}
              <input
                name={name}
                className="mt-1 w-full rounded-lg border border-[var(--line)] p-2 font-mono font-normal"
              />
            </label>
          ))}
        </div>
        {error && (
          <p role="alert" className="text-sm text-[var(--danger)]">
            {error}
          </p>
        )}
        <Button disabled={pending}>
          {pending
            ? "Provisioning exact revision…"
            : "Launch isolated workspace"}
        </Button>
      </form>
    </Card>
  );
}
