"use client";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
type Item = {
  id: string;
  kind: string;
  repository_id: string;
  resource_id: string;
  revision: string;
  owner_ids: string[];
  destination_ids?: string[];
  disposition: string;
  state: string;
  summary: string;
  citation: string;
};
type Candidate = {
  id: string;
  authority: string;
  repositories: {
    id: string;
    default_branch: string;
    tip: string;
    object_count: number;
    size_bytes: number;
    license_paths?: string[];
    provenance_sha256: string;
    signature_state: string;
  }[];
  gaps?: {
    kind: string;
    resource_id: string;
    state: string;
    summary: string;
    required_decision: string;
  }[];
  rehearsals?: {
    id: string;
    state: string;
    cost_units: number;
    required_decisions?: string[];
    outcomes: {
      scenario_id: string;
      kind: string;
      destination_id: string;
      state: string;
      output?: string;
    }[];
  }[];
};
type Plan = {
  id: string;
  version: number;
  title: string;
  intent: string;
  sources: { repository_id: string; revision: string; role: string }[];
  destinations: {
    id: string;
    name: string;
    owner_ids: string[];
    visibility: string;
    default_branch: string;
    retained_identity?: string;
  }[];
  mappings: {
    id: string;
    source_path: string;
    destination_id?: string;
    destination_path?: string;
    history_mode: string;
    disposition: string;
    retain_identity: boolean;
  }[];
  inventory: Item[];
  deadline: string;
  success_criteria: string[];
  rollback_limits: string[];
  findings?: {
    id: string;
    body: string;
    actor_id: string;
    actor_kind: string;
    inventory_item_ids: string[];
    citations: string[];
  }[];
  candidate_sets?: Candidate[];
  collaboration_mappings?: {
    id:string; kind:string; source_resource_id:string; source_revision:string; disposition:string; state:string; blocked_reason?:string; authority:string;
    snapshot:{authorship_ids:string[];discussion_ids?:string[];review_ids?:string[];dependency_ids?:string[];acceptance_criteria:string[]};
    destinations?:{destination_id:string;resource_id:string;revision:string;owner_ids:string[];dependency_ids?:string[];acceptance_criteria:string[]}[];
    decisions?:{actor_id:string;decision:string;source_revision:string;note?:string}[];
  }[];
  dependent_migrations?: {
    id:string;kind:string;resource_id:string;owner_id:string;audience:string;state:string;next_action:string;authority:string;
    compatibility_window:{starts_at:string;ends_at:string};replacement_remotes?:{destination_id:string;remote_url:string;ref?:string}[];
    mappings?:{from:string;to:string;kind:string}[];synchronization:string[];propagation?:{actor_kind:string;repository_id:string;task_id?:string;pull_id?:string;release_id?:string};
    events?:{actor_id:string;state:string;evidence?:string;next_action:string;created_at:string}[];
  }[];
  cutover?: {state:string;source_state:string;candidate_id:string;pause_kinds:string[];cleanup_policy:string;authority:string;blockers?:string[];approvals?:{actor_id:string;destination_id:string;decision:string}[];destinations:{destination_id:string;repository_id?:string;state:string;revision?:string;health:string}[];observations?:{kind:string;resource_id:string;state:string;evidence:string;actor_id:string}[]};
  authority: string;
};
const rows = (v: FormDataEntryValue | null) =>
  String(v ?? "")
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
const ids = (v: string) =>
  v
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
export function RestructuringPlansWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token } = useAuth(),
    [items, setItems] = useState<Plan[]>([]),
    [error, setError] = useState(""),
    [saving, setSaving] = useState(false),
    request = useRef(crypto.randomUUID());
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const x = await api<{ restructuring_plans: Plan[] }>(
        `/repositories/${repositoryID}/restructuring-plans`,
        {},
        token,
      );
      setItems(x.restructuring_plans);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Plans could not be loaded.");
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setSaving(true);
    const form = e.currentTarget,
      f = new FormData(form);
    try {
      const sources = rows(f.get("sources")).map((line) => {
          const [repository_id, revision, role] = line
            .split("|")
            .map((x) => x.trim());
          return { repository_id, revision, role };
        }),
        destinations = rows(f.get("destinations")).map((line, i) => {
          const [
            name,
            owners,
            visibility,
            default_branch,
            retained_identity = "",
          ] = line.split("|").map((x) => x.trim());
          return {
            id: `destination-${i + 1}`,
            name,
            owner_ids: ids(owners),
            visibility,
            default_branch,
            retained_identity,
          };
        }),
        mappings = rows(f.get("mappings")).map((line, i) => {
          const [
            source_repository_id,
            source_path,
            destination_id,
            destination_path,
            history_mode,
            disposition,
            retain = "false",
          ] = line.split("|").map((x) => x.trim());
          return {
            id: `mapping-${i + 1}`,
            source_repository_id,
            source_path,
            destination_id,
            destination_path,
            history_mode,
            disposition,
            retain_identity: retain === "true",
          };
        }),
        inventory = rows(f.get("inventory")).map((line, i) => {
          const [
            kind,
            source_repository_id,
            resource_id,
            revision,
            owners,
            destination_ids,
            disposition,
            state,
            summary,
            citation,
          ] = line.split("|").map((x) => x.trim());
          return {
            id: `inventory-${i + 1}`,
            kind,
            repository_id: source_repository_id,
            resource_id,
            revision,
            owner_ids: ids(owners),
            destination_ids: ids(destination_ids),
            disposition,
            state,
            summary,
            citation,
          };
        });
      const out = await api<Plan>(
        `/repositories/${repositoryID}/restructuring-plans`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: request.current,
            title: String(f.get("title")),
            intent: String(f.get("intent")),
            sources,
            destinations,
            mappings,
            inventory,
            deadline: new Date(String(f.get("deadline"))).toISOString(),
            success_criteria: rows(f.get("success")),
            rollback_limits: rows(f.get("rollback")),
          }),
        },
        token,
      );
      setItems((x) => [out, ...x.filter((y) => y.id !== out.id)]);
      request.current = crypto.randomUUID();
      form.reset();
      setError("");
    } catch (x) {
      setError(x instanceof Error ? x.message : "Plan could not be opened.");
    } finally {
      setSaving(false);
    }
  }
  async function finding(e: FormEvent<HTMLFormElement>, p: Plan) {
    e.preventDefault();
    setSaving(true);
    const form = e.currentTarget,
      f = new FormData(form);
    try {
      const out = await api<Plan>(
        `/repositories/${repositoryID}/restructuring-plans/${p.id}/findings`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: crypto.randomUUID(),
            version: p.version,
            inventory_item_ids: ids(String(f.get("items"))),
            body: String(f.get("body")),
            citations: rows(f.get("citations")),
          }),
        },
        token,
      );
      setItems((x) => x.map((y) => (y.id === out.id ? out : y)));
      form.reset();
      setError("");
    } catch (x) {
      setError(x instanceof Error ? x.message : "Finding could not be added.");
    } finally {
      setSaving(false);
    }
  }
  async function assemble(e: FormEvent<HTMLFormElement>, p: Plan) {
    e.preventDefault();
    setSaving(true);
    const form = e.currentTarget,
      f = new FormData(form);
    try {
      const out = await api<Plan>(
        `/repositories/${repositoryID}/restructuring-plans/${p.id}/candidate-sets`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: crypto.randomUUID(),
            expected_version: p.version,
            cross_repository_links: rows(f.get("links")),
          }),
        },
        token,
      );
      setItems((x) => x.map((y) => (y.id === out.id ? out : y)));
      form.reset();
      setError("");
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Candidate repositories could not be assembled.",
      );
    } finally {
      setSaving(false);
    }
  }
  async function mapWork(e:FormEvent<HTMLFormElement>,p:Plan){e.preventDefault();setSaving(true);const form=e.currentTarget,f=new FormData(form);try{const destinations=rows(f.get("destinations")).map(line=>{const[destination_id,resource_id,revision,owners,dependencies,criteria]=line.split("|").map(x=>x.trim());return{destination_id,resource_id,revision,owner_ids:ids(owners),dependency_ids:ids(dependencies),acceptance_criteria:ids(criteria)}});const out=await api<Plan>(`/repositories/${repositoryID}/restructuring-plans/${p.id}/collaboration-mappings`,{method:"POST",body:JSON.stringify({expected_version:p.version,mapping:{request_id:crypto.randomUUID(),inventory_item_id:String(f.get("inventory_item_id")),kind:String(f.get("kind")),source_repository_id:String(f.get("source_repository_id")),source_resource_id:String(f.get("source_resource_id")),source_revision:String(f.get("source_revision")),disposition:String(f.get("disposition")),embargoed:f.get("embargoed")==="on",blocked_reason:String(f.get("blocked_reason")),snapshot:{authorship_ids:ids(String(f.get("authors"))),discussion_ids:ids(String(f.get("discussion"))),review_ids:ids(String(f.get("reviews"))),dependency_ids:ids(String(f.get("dependencies"))),acceptance_criteria:rows(f.get("criteria"))},destinations}})},token);setItems(x=>x.map(y=>y.id===out.id?out:y));form.reset();setError("")}catch(x){setError(x instanceof Error?x.message:"Active work mapping could not be retained.")}finally{setSaving(false)}}
  async function decide(p:Plan,m:NonNullable<Plan["collaboration_mappings"]>[number],decision:string){setSaving(true);try{const out=await api<Plan>(`/repositories/${repositoryID}/restructuring-plans/${p.id}/collaboration-mappings/${m.id}/decisions`,{method:"POST",body:JSON.stringify({expected_version:p.version,decision:{request_id:crypto.randomUUID(),decision,source_revision:m.source_revision,note:`${decision} after inspecting retained intent`}})},token);setItems(x=>x.map(y=>y.id===out.id?out:y));setError("")}catch(x){setError(x instanceof Error?x.message:"Mapping decision could not be retained.")}finally{setSaving(false)}}
  async function addDependent(e:FormEvent<HTMLFormElement>,p:Plan){e.preventDefault();setSaving(true);const form=e.currentTarget,f=new FormData(form);try{const replacements=rows(f.get("replacement_remotes")).map(line=>{const[destination_id,remote_url,ref]=line.split("|").map(x=>x.trim());return{destination_id,remote_url,ref}}),mappings=rows(f.get("machine_mappings")).map(line=>{const[kind,from,to]=line.split("|").map(x=>x.trim());return{kind,from,to}});const out=await api<Plan>(`/repositories/${repositoryID}/restructuring-plans/${p.id}/dependent-migrations`,{method:"POST",body:JSON.stringify({expected_version:p.version,migration:{request_id:crypto.randomUUID(),kind:String(f.get("dependent_kind")),resource_id:String(f.get("dependent_resource")),owner_id:String(f.get("dependent_owner")),audience:String(f.get("audience")),state:String(f.get("dependent_state")),compatibility_window:{starts_at:new Date(String(f.get("window_start"))).toISOString(),ends_at:new Date(String(f.get("window_end"))).toISOString()},next_action:String(f.get("next_action")),replacement_remotes:replacements,mappings,synchronization:rows(f.get("synchronization"))}})},token);setItems(x=>x.map(y=>y.id===out.id?out:y));form.reset();setError("")}catch(x){setError(x instanceof Error?x.message:"Dependent migration could not be retained.")}finally{setSaving(false)}}
  async function updateDependent(e:FormEvent<HTMLFormElement>,p:Plan,m:NonNullable<Plan["dependent_migrations"]>[number]){e.preventDefault();setSaving(true);const form=e.currentTarget,f=new FormData(form);try{const out=await api<Plan>(`/repositories/${repositoryID}/restructuring-plans/${p.id}/dependent-migrations/${m.id}/events`,{method:"POST",body:JSON.stringify({expected_version:p.version,event:{request_id:crypto.randomUUID(),state:String(f.get("state")),evidence:String(f.get("evidence")),next_action:String(f.get("next_action"))}})},token);setItems(x=>x.map(y=>y.id===out.id?out:y));form.reset();setError("")}catch(x){setError(x instanceof Error?x.message:"Only the dependent owner can update this state.")}finally{setSaving(false)}}
  async function startCutover(p:Plan,candidate_id:string){setSaving(true);try{const out=await api<Plan>(`/repositories/${repositoryID}/restructuring-plans/${p.id}/cutover`,{method:"POST",body:JSON.stringify({expected_version:p.version,cutover:{request_id:crypto.randomUUID(),candidate_id,pause_kinds:["git","collaboration","automation","release"],cleanup_policy:"archive"}})},token);setItems(x=>x.map(y=>y.id===out.id?out:y));setError("")}catch(x){setError(x instanceof Error?x.message:"Cutover is not ready.")}finally{setSaving(false)}}
  async function approveCutover(p:Plan,destination_id:string){setSaving(true);try{const out=await api<Plan>(`/repositories/${repositoryID}/restructuring-plans/${p.id}/cutover/approvals`,{method:"POST",body:JSON.stringify({expected_version:p.version,approval:{request_id:crypto.randomUUID(),destination_id,decision:"approve"}})},token);setItems(x=>x.map(y=>y.id===out.id?out:y))}catch(x){setError(x instanceof Error?x.message:"Approval failed.")}finally{setSaving(false)}}
  async function activateCutover(e:FormEvent<HTMLFormElement>,p:Plan){e.preventDefault();setSaving(true);const form=e.currentTarget,f=new FormData(form),repositories=Object.fromEntries(rows(f.get("repositories")).map(line=>line.split("|").map(x=>x.trim()) as [string,string]));try{const out=await api<Plan>(`/repositories/${repositoryID}/restructuring-plans/${p.id}/cutover/activate`,{method:"POST",body:JSON.stringify({expected_version:p.version,repositories})},token);setItems(x=>x.map(y=>y.id===out.id?out:y));form.reset()}catch(x){await load();setError(x instanceof Error?x.message:"Activation failed.")}finally{setSaving(false)}}
  async function observeCutover(e:FormEvent<HTMLFormElement>,p:Plan){e.preventDefault();setSaving(true);const form=e.currentTarget,f=new FormData(form);try{const out=await api<Plan>(`/repositories/${repositoryID}/restructuring-plans/${p.id}/cutover/observations`,{method:"POST",body:JSON.stringify({expected_version:p.version,observation:{request_id:crypto.randomUUID(),kind:String(f.get("kind")),resource_id:String(f.get("resource")),state:String(f.get("state")),evidence:String(f.get("evidence"))}})},token);setItems(x=>x.map(y=>y.id===out.id?out:y));form.reset()}catch(x){setError(x instanceof Error?x.message:"Observation failed.")}finally{setSaving(false)}}
  async function finishCutover(p:Plan,rollback:boolean){setSaving(true);try{const out=await api<Plan>(`/repositories/${repositoryID}/restructuring-plans/${p.id}/cutover/finish`,{method:"POST",body:JSON.stringify({expected_version:p.version,rollback})},token);setItems(x=>x.map(y=>y.id===out.id?out:y))}catch(x){setError(x instanceof Error?x.message:"Cleanup remains blocked.")}finally{setSaving(false)}}
  return (
    <main className="mx-auto max-w-6xl space-y-6 p-6">
      <div>
        <p className="text-xs font-semibold uppercase tracking-widest text-[var(--brand)]">
          Repository topology
        </p>
        <h1 className="text-3xl font-semibold">Restructuring plans</h1>
        <p className="text-sm text-[var(--muted)]">
          Make future project boundaries, retained identities, affected owners,
          explicit gaps, and rollback limits reviewable before any repository
          moves.
        </p>
      </div>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      <Card>
        <form onSubmit={create} className="grid gap-3">
          <h2 className="font-semibold">Open a plan</h2>
          <input
            name="title"
            required
            placeholder="Plan title"
            className="rounded border p-2"
          />
          <textarea
            name="intent"
            required
            placeholder="Intent"
            className="rounded border p-2"
          />
          <textarea
            name="sources"
            required
            placeholder="Sources: repository | 40-char revision | primary/contributing"
            className="rounded border p-2"
          />
          <textarea
            name="destinations"
            required
            placeholder="Destinations: name | owners | public/private/internal | default branch | retained identity"
            className="rounded border p-2"
          />
          <textarea
            name="mappings"
            required
            placeholder="Mappings: source repo | source path | destination ID | destination path | full/selected/none | move/copy/remain | retain identity true/false"
            className="rounded border p-2"
          />
          <textarea
            name="inventory"
            required
            rows={8}
            placeholder="One of every kind: kind | source repo | resource | exact revision | owners | destination IDs | move/remain/divide/unknown | resolved/inaccessible/ambiguous/shared | summary | citation"
            className="rounded border p-2"
          />
          <input
            name="deadline"
            type="datetime-local"
            required
            className="rounded border p-2"
          />
          <textarea
            name="success"
            required
            placeholder="Success criteria, one per line"
            className="rounded border p-2"
          />
          <textarea
            name="rollback"
            required
            placeholder="Rollback limits, one per line"
            className="rounded border p-2"
          />
          <Button disabled={saving}>Open review boundary</Button>
        </form>
      </Card>
      {items.map((p) => (
        <Card key={p.id}>
          <div className="flex justify-between gap-3">
            <div>
              <h2 className="text-xl font-semibold">{p.title}</h2>
              <p>{p.intent}</p>
            </div>
            <Badge>{`v${p.version}`}</Badge>
          </div>
          <p className="mt-2 text-xs text-[var(--muted)]">{p.authority}</p>
          <div className="mt-4 grid gap-4 md:grid-cols-3">
            <section>
              <h3 className="font-semibold">Sources</h3>
              {p.sources.map((x) => (
                <p className="break-all text-sm" key={x.repository_id}>
                  {x.repository_id} · {x.role}
                  <br />
                  {x.revision}
                </p>
              ))}
            </section>
            <section>
              <h3 className="font-semibold">Destinations</h3>
              {p.destinations.map((x) => (
                <p className="text-sm" key={x.id}>
                  {x.id}: {x.name} · {x.visibility} · {x.default_branch}
                  <br />
                  Owners: {x.owner_ids.join(", ")}
                </p>
              ))}
            </section>
            <section>
              <h3 className="font-semibold">Decision boundary</h3>
              <p className="text-sm">
                Deadline {new Date(p.deadline).toLocaleString()}
              </p>
              <p className="text-sm">
                Success: {p.success_criteria.join("; ")}
              </p>
              <p className="text-sm">
                Rollback: {p.rollback_limits.join("; ")}
              </p>
            </section>
          </div>
          <h3 className="mt-4 font-semibold">Affected inventory</h3>
          <div className="grid gap-2">
            {p.inventory.map((x) => (
              <div key={x.id} className="rounded border p-3 text-sm">
                <div className="flex gap-2">
                  <Badge>{x.kind.replaceAll("_", " ")}</Badge>
                  <Badge>{x.state}</Badge>
                  <Badge>{x.disposition}</Badge>
                </div>
                <p className="mt-1 font-medium">{x.resource_id}</p>
                <p>{x.summary}</p>
                <p className="break-all text-xs text-[var(--muted)]">
                  Owner {x.owner_ids.join(", ")} · {x.revision} · {x.citation}
                </p>
              </div>
            ))}
          </div>
          {p.findings?.map((x) => (
            <blockquote key={x.id} className="mt-3 border-l-2 pl-3 text-sm">
              <b>
                {x.actor_kind} {x.actor_id}
              </b>
              : {x.body}
              <br />
              <span className="text-xs">
                Citations: {x.citations.join("; ")}
              </span>
            </blockquote>
          ))}
          <h3 className="mt-4 font-semibold">Immutable candidate repositories</h3>
          {p.candidate_sets?.map((candidate) => (
            <section key={candidate.id} className="mt-2 rounded border p-3 text-sm">
              <p className="text-xs text-[var(--muted)]">{candidate.authority}</p>
              {candidate.repositories.map((repository) => (
                <div key={repository.id} className="mt-2">
                  <b>{repository.id}</b> · {repository.default_branch} · {repository.object_count} objects · {repository.size_bytes} bytes
                  <p className="break-all text-xs">Tip {repository.tip}<br />Provenance SHA-256 {repository.provenance_sha256}<br />{repository.signature_state}</p>
                  <p>License evidence: {repository.license_paths?.join(", ") || "none found"}</p>
                </div>
              ))}
              {candidate.gaps?.map((gap, index) => <p key={`${gap.kind}-${index}`} className="mt-2 text-[var(--danger)]">{gap.kind}: {gap.summary} Decision: {gap.required_decision}</p>)}
              {candidate.rehearsals?.map((rehearsal) => <div key={rehearsal.id} className="mt-2"><Badge>{rehearsal.state}</Badge> · {rehearsal.cost_units.toFixed(3)} cost units<p>{rehearsal.required_decisions?.join("; ")}</p></div>)}
            </section>
          ))}
          <form onSubmit={(e) => assemble(e, p)} className="mt-4 grid gap-2">
            <textarea name="links" placeholder="Required cross-repository links, one per line" className="rounded border p-2" />
            <Button disabled={saving}>Assemble candidates without redirecting work</Button>
          </form>
          <h3 className="mt-4 font-semibold">Active collaboration continuity</h3>
          <p className="text-sm text-[var(--muted)]">Mappings preserve exact intent and evidence. Each retained author approves independently; split work stays connected by destination dependencies.</p>
          {p.collaboration_mappings?.map(m=><section key={m.id} className="mt-2 rounded border p-3 text-sm"><div className="flex gap-2"><Badge>{m.kind.replaceAll("_"," ")}</Badge><Badge>{m.state}</Badge><Badge>{m.disposition}</Badge></div><p className="mt-1"><b>{m.source_resource_id}</b> at <code>{m.source_revision}</code></p><p>Authors: {m.snapshot.authorship_ids.join(", ")} · acceptance: {m.snapshot.acceptance_criteria.join("; ")}</p><p>Discussion {m.snapshot.discussion_ids?.join(", ")||"none"} · reviews {m.snapshot.review_ids?.join(", ")||"none"}</p>{m.destinations?.map(d=><p key={d.destination_id}><b>{d.destination_id}/{d.resource_id}</b> · owners {d.owner_ids.join(", ")} · depends on {d.dependency_ids?.join(", ")||"nothing"}</p>)}{m.blocked_reason&&<p className="text-[var(--danger)]">Blocked: {m.blocked_reason}</p>}<p className="text-xs text-[var(--muted)]">{m.authority}</p>{m.state==="proposed"&&<div className="mt-2 flex gap-2"><Button type="button" disabled={saving} onClick={()=>void decide(p,m,"approve")}>Approve exact mapping</Button><Button type="button" variant="secondary" disabled={saving} onClick={()=>void decide(p,m,"reject")}>Reject mapping</Button></div>}</section>)}
          <form onSubmit={e=>mapWork(e,p)} className="mt-3 grid gap-2"><input name="inventory_item_id" required placeholder="Inventoried source item ID" className="rounded border p-2"/><div className="grid gap-2 md:grid-cols-2"><input name="kind" required placeholder="branch/pull_request/issue/proposal/task/decision/check/session/workspace/queue" className="rounded border p-2"/><select name="disposition" className="rounded border p-2"><option value="move">Move</option><option value="divide">Divide into connected contributions</option><option value="archive">Archive explicitly</option></select><input name="source_repository_id" required placeholder="Source repository ID" className="rounded border p-2"/><input name="source_resource_id" required placeholder="Source resource ID" className="rounded border p-2"/><input name="source_revision" required placeholder="Exact 40-character source revision" className="rounded border p-2"/><input name="authors" required placeholder="All inventoried owner IDs, comma separated" className="rounded border p-2"/></div><input name="discussion" placeholder="Discussion IDs" className="rounded border p-2"/><input name="reviews" placeholder="Review IDs (retained, never silently current)" className="rounded border p-2"/><input name="dependencies" placeholder="Source dependency IDs" className="rounded border p-2"/><textarea name="criteria" required placeholder="Original acceptance criteria, one per line" className="rounded border p-2"/><textarea name="destinations" placeholder="destination ID | resource ID | exact revision | owners | dependencies | acceptance criteria (comma separated)" className="rounded border p-2"/><input name="blocked_reason" placeholder="Explicit conflict/access/embargo/archive reason" className="rounded border p-2"/><label className="text-sm"><input type="checkbox" name="embargoed"/> Context is embargoed; retain only a blocked mapping</label><Button disabled={saving}>Propose active-work mapping</Button></form>
          <h3 className="mt-4 font-semibold">Developer and dependent continuity</h3>
          <p className="text-sm text-[var(--muted)]">The machine-readable public subset is available from this plan&apos;s dependency-map endpoint. Adoption remains owned by each dependent.</p>
          {p.dependent_migrations?.map(m=><section key={m.id} className="mt-2 rounded border p-3 text-sm"><div className="flex gap-2"><Badge>{m.kind.replaceAll("_"," ")}</Badge><Badge>{m.state}</Badge><Badge>{m.audience}</Badge></div><p><b>{m.resource_id}</b> · owner {m.owner_id}</p><p>Compatibility {new Date(m.compatibility_window.starts_at).toLocaleString()} – {new Date(m.compatibility_window.ends_at).toLocaleString()}</p><p>Next: {m.next_action}</p>{m.replacement_remotes?.map(r=><p className="break-all" key={`${r.destination_id}-${r.remote_url}`}>Remote {r.destination_id}: <code>{r.remote_url}</code> {r.ref&&`(${r.ref})`}</p>)}<p>Safe sync: {m.synchronization.join(" → ")}</p><p className="text-xs text-[var(--muted)]">{m.authority}</p><form onSubmit={e=>updateDependent(e,p,m)} className="mt-2 grid gap-2 md:grid-cols-4"><select name="state" className="rounded border p-2">{["planned","in_progress","adopted","blocked","unavailable","rejected","stale_credentials","unmigrated"].map(x=><option key={x}>{x}</option>)}</select><input name="evidence" placeholder="Evidence (required for adoption)" className="rounded border p-2"/><input name="next_action" required placeholder="Current next action" className="rounded border p-2"/><Button disabled={saving}>Owner update</Button></form></section>)}
          <form onSubmit={e=>addDependent(e,p)} className="mt-3 grid gap-2"><div className="grid gap-2 md:grid-cols-2"><select name="dependent_kind" className="rounded border p-2">{["clone","fork","package","api","dependency","extension","workflow","documentation_link","deployment","federated_follower"].map(x=><option key={x}>{x}</option>)}</select><input name="dependent_resource" required placeholder="Clone, package, API, link, deployment, or follower identity" className="rounded border p-2"/><input name="dependent_owner" required placeholder="Accountable owner ID" className="rounded border p-2"/><select name="dependent_state" className="rounded border p-2">{["discovered","planned","in_progress","adopted","blocked","unavailable","rejected","stale_credentials","unmigrated"].map(x=><option key={x}>{x}</option>)}</select><select name="audience" className="rounded border p-2"><option>public</option><option>participants</option><option>owner</option></select><input name="next_action" required placeholder="Audience-appropriate next action" className="rounded border p-2"/><input name="window_start" type="datetime-local" required className="rounded border p-2"/><input name="window_end" type="datetime-local" required className="rounded border p-2"/></div><textarea name="replacement_remotes" placeholder="destination ID | explicit replacement remote | optional ref (one per line)" className="rounded border p-2"/><textarea name="machine_mappings" placeholder="dependency/link/package/api | old identity | new identity" className="rounded border p-2"/><textarea name="synchronization" required placeholder="Safe credential-free synchronization steps, one per line" className="rounded border p-2"/><Button disabled={saving}>Retain dependent migration</Button></form>
          <h3 className="mt-4 font-semibold">Authority cutover</h3>
          {!p.cutover&&p.candidate_sets?.map(c=><Button key={c.id} type="button" disabled={saving} onClick={()=>void startCutover(p,c.id)}>Stage cutover from {c.id.slice(0,8)}</Button>)}
          {p.cutover&&<section className="mt-2 rounded border p-3 text-sm"><div className="flex gap-2"><Badge>{p.cutover.state.replaceAll("_"," ")}</Badge><Badge>{p.cutover.source_state.replaceAll("_"," ")}</Badge><Badge>{p.cutover.cleanup_policy}</Badge></div><p className="mt-1">Active controls: {p.cutover.pause_kinds.join(", ")}</p>{p.cutover.blockers?.map(x=><p key={x} className="text-[var(--danger)]">Blocked: {x}</p>)}{p.cutover.destinations.map(d=><div key={d.destination_id} className="mt-2"><b>{d.destination_id}</b> · {d.state} · {d.health} {d.repository_id&&<>· <code>{d.repository_id}</code></>}<Button type="button" variant="secondary" disabled={saving||p.cutover?.state!=="awaiting_approval"} onClick={()=>void approveCutover(p,d.destination_id)}>Owner approval</Button></div>)}{["awaiting_approval","publication_blocked"].includes(p.cutover.state)&&<form onSubmit={e=>activateCutover(e,p)} className="mt-2 grid gap-2"><textarea name="repositories" required placeholder="destination ID | pre-created exact repository ID" className="rounded border p-2"/><Button disabled={saving}>{p.cutover.state==="publication_blocked"?"Retry exact blocked publication":"Publish candidate refs and activate boundary"}</Button></form>}{p.cutover.observations?.map((o,i)=><p key={`${o.kind}-${i}`}><b>{o.kind}</b> {o.resource_id}: {o.state} — {o.evidence}</p>)}{p.cutover.state==="active"&&<form onSubmit={e=>observeCutover(e,p)} className="mt-2 grid gap-2 md:grid-cols-4"><select name="kind" className="rounded border p-2">{["build","release","permission","link","consumer","contribution","git_traffic","late_write"].map(x=><option key={x}>{x}</option>)}</select><input name="resource" required placeholder="Resource" className="rounded border p-2"/><select name="state" className="rounded border p-2"><option>passed</option><option>failed</option><option>residual</option></select><input name="evidence" required placeholder="Bounded evidence" className="rounded border p-2"/><Button disabled={saving}>Report health</Button></form>}{["active","publication_blocked"].includes(p.cutover.state)&&<div className="mt-2 flex gap-2">{p.cutover.state==="active"&&<Button type="button" disabled={saving} onClick={()=>void finishCutover(p,false)}>Verify and retire compatibility</Button>}<Button type="button" variant="secondary" disabled={saving} onClick={()=>void finishCutover(p,true)}>Restore source authority without changing destination refs</Button></div>}<p className="mt-2 text-xs text-[var(--muted)]">{p.cutover.authority}</p></section>}
          <form onSubmit={(e) => finding(e, p)} className="mt-4 grid gap-2">
            <h3 className="font-semibold">Add cited impact finding</h3>
            <input
              name="items"
              required
              placeholder="Inventory item IDs, comma separated"
              className="rounded border p-2"
            />
            <textarea
              name="body"
              required
              placeholder="Impact finding"
              className="rounded border p-2"
            />
            <textarea
              name="citations"
              required
              placeholder="Citations, one per line"
              className="rounded border p-2"
            />
            <Button disabled={saving}>Add finding</Button>
          </form>
        </Card>
      ))}
      {items.length === 0 && (
        <p className="text-sm text-[var(--muted)]">
          No restructuring plans yet.
        </p>
      )}
    </main>
  );
}
