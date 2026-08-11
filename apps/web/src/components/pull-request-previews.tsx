"use client";

import { useCallback, useEffect, useState } from "react";
import { api, type PullPreview } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

export function PullRequestPreviews({
  repositoryID,
  pullRequestID,
  participant,
  owner,
}: {
  repositoryID: string;
  pullRequestID: string;
  participant: boolean;
  owner: boolean;
}) {
  const { token } = useAuth();
  const [items, setItems] = useState<PullPreview[]>([]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [audience, setAudience] = useState({
    source_kind: "user",
    user_id: "",
    source_id: "",
    role: "view",
    expires_at: "",
  });
  const [finding, setFinding] = useState({ route: "/", title: "", description: "", classification: "bug", severity: "major", steps: "", console: "", duplicate_of: "" });
  const [uploads, setUploads] = useState<File[]>([]);
  const [comments, setComments] = useState<Record<string, string>>({});
  const base = `/repositories/${repositoryID}/pulls/${pullRequestID}/previews`;
  const load = useCallback(async () => {
    try {
      setItems(
        (await api<{ previews: PullPreview[] }>(base, {}, token)).previews,
      );
      setError("");
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Previews could not be loaded.",
      );
    }
  }, [base, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
    const timer = window.setInterval(() => void load(), 3000);
    return () => window.clearInterval(timer);
  }, [load]);
  async function launch() {
    setPending(true);
    try {
      await api(base, { method: "POST" }, token);
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Preview could not be launched.",
      );
    } finally {
      setPending(false);
    }
  }
  async function invite(previewID: string) {
    setPending(true);
    try {
      await api(
        `${base}/${previewID}/invitations`,
        {
          method: "POST",
          body: JSON.stringify({
            ...audience,
            expires_at: new Date(audience.expires_at).toISOString(),
          }),
        },
        token,
      );
      setAudience({ ...audience, user_id: "", source_id: "" });
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Audience could not be invited.",
      );
    } finally {
      setPending(false);
    }
  }
  async function revoke(previewID: string, invitationID: string) {
    setPending(true);
    try {
      await api(
        `${base}/${previewID}/invitations/${invitationID}`,
        { method: "DELETE" },
        token,
      );
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Invitation could not be revoked.",
      );
    } finally {
      setPending(false);
    }
  }
  async function createFinding(previewID: string) {
    setPending(true);
    try {
      const consoleText = finding.console.trim();
      let encodedConsole = "";
      for (const byte of new TextEncoder().encode(consoleText)) encodedConsole += String.fromCharCode(byte);
      const uploadedEvidence = await Promise.all(uploads.map(async (file) => {
        const bytes = new Uint8Array(await file.arrayBuffer()); let binary = "";
        for (let offset=0; offset<bytes.length; offset+=32768) binary += String.fromCharCode(...bytes.subarray(offset,offset+32768));
        const kind = file.type.startsWith("image/") ? "screenshot" : file.type.startsWith("video/") ? "recording" : file.type === "application/json" ? "trace" : "annotation";
        return {kind,name:file.name,media_type:file.type||"text/plain",size:file.size,data:btoa(binary)};
      }));
      await api(`${base}/${previewID}/findings`, { method: "POST", body: JSON.stringify({
        route: finding.route, title: finding.title, description: finding.description,
        classification: finding.classification, severity: finding.severity,
        duplicate_of: finding.duplicate_of,
        reproduction_steps: finding.steps.split("\n").map((step) => step.trim()).filter(Boolean),
        evidence: [...uploadedEvidence, ...(consoleText ? [{ kind:"console", name:"browser-console.txt", media_type:"text/plain", size:new Blob([consoleText]).size, data:btoa(encodedConsole) }] : [])],
      }) }, token);
      setFinding({ ...finding, title:"", description:"", steps:"", console:"", duplicate_of:"" });
      setUploads([]);
      await load();
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Finding could not be recorded."); }
    finally { setPending(false); }
  }
  async function decide(previewID:string, findingID:string, version:number, decision:Record<string,string>) {
    setPending(true);
    try { await api(`${base}/${previewID}/findings/${findingID}/decision`, {method:"POST",body:JSON.stringify({version,...decision})}, token); await load(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Finding changed; reload and try again."); }
    finally { setPending(false); }
  }
  async function comment(previewID:string, findingID:string, version:number) {
    const body=comments[findingID]?.trim(); if(!body)return; setPending(true);
    try { await api(`${base}/${previewID}/findings/${findingID}/comments`, {method:"POST",body:JSON.stringify({version,body})}, token); setComments({...comments,[findingID]:""}); await load(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Comment could not be added."); }
    finally { setPending(false); }
  }
  return (
    <section id="previews" className="scroll-mt-24 space-y-3">
      <div className="flex items-baseline justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Change previews</h2>
          <p className="text-xs text-[var(--muted)]">
            Isolated experiences pinned to one review revision
          </p>
        </div>
        {participant && (
          <Button disabled={pending} onClick={() => void launch()}>
            {pending ? "Launching…" : "Launch preview"}
          </Button>
        )}
      </div>
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      {items.length === 0 ? (
        <Card className="p-5 text-sm text-[var(--muted)]">
          No preview has been published. Add a version 1{" "}
          <code>.vivarium/preview.json</code> with an explicit access policy to
          opt in.
        </Card>
      ) : (
        items.map((item) => (
          <Card key={item.id} className="p-5">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p className="font-semibold">
                  Preview <code>{item.id.slice(0, 8)}</code>
                </p>
                <p className="mt-1 text-xs text-[var(--muted)]">
                  revision <code>{item.revision.slice(0, 12)}</code> · created
                  by {item.creator_id} ·{" "}
                  {new Date(item.created_at).toLocaleString()}
                </p>
              </div>
              <Badge
                tone={
                  item.stale
                    ? "warning"
                    : item.state === "succeeded"
                      ? "success"
                      : item.state === "failed"
                        ? "danger"
                        : "neutral"
                }
              >
                {item.stale ? "stale" : item.state}
              </Badge>
            </div>
            <p className="mt-3 text-xs">
              Definition <code>{item.definition_sha256.slice(0, 12)}</code> ·{" "}
              {item.definition.resources.cpus} CPU ·{" "}
              {item.definition.resources.memory_mb} MiB memory ·{" "}
              {item.definition.resources.storage_mb} MiB output
            </p>
            <div className="mt-3 rounded-lg border border-[var(--line)] p-3 text-xs">
              <p className="font-semibold">Safe audience boundary</p>
              <p className="mt-1">
                Named, expiring guests · network{" "}
                {item.definition.access.network} · data limited to built preview
                artifacts · actions {item.definition.access.actions.join(", ")}.
              </p>
              <p className="mt-1 text-[var(--muted)]">
                An invitation grants no repository, credential, workspace,
                environment, deployment, or production authority. Build logs and
                private services remain participant-only.
              </p>
            </div>
            <div className="mt-4 flex gap-3 text-sm">
              <a
                className="font-semibold text-[var(--accent)]"
                href={`/api${item.url}`}
                target="_blank"
                rel="noreferrer"
              >
                Open exact preview
              </a>
              <a
                className="text-[var(--muted)] underline"
                href={`/api${base}/${item.id}/events`}
                target="_blank"
                rel="noreferrer"
              >
                Build logs
              </a>
            </div>
            {owner && (
              <div className="mt-4 space-y-3 border-t border-[var(--line)] pt-4">
                <p className="text-sm font-semibold">Preview audience</p>
                <div className="grid gap-2 sm:grid-cols-4">
                  <select
                    className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm"
                    value={audience.source_kind}
                    onChange={(e) =>
                      setAudience({ ...audience, source_kind: e.target.value })
                    }
                  >
                    <option value="user">Named user</option>
                    <option value="issue">Issue participants</option>
                    <option value="decision">Decision participants</option>
                    <option value="proposal">Proposal participants</option>
                  </select>
                  <input
                    className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm sm:col-span-2"
                    value={
                      audience.source_kind === "user"
                        ? audience.user_id
                        : audience.source_id
                    }
                    onChange={(e) =>
                      setAudience({
                        ...audience,
                        [audience.source_kind === "user"
                          ? "user_id"
                          : "source_id"]: e.target.value,
                      })
                    }
                    placeholder={
                      audience.source_kind === "user"
                        ? "User ID"
                        : "Resource ID"
                    }
                  />
                  <select
                    className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm"
                    value={audience.role}
                    onChange={(e) =>
                      setAudience({ ...audience, role: e.target.value })
                    }
                  >
                    {item.definition.access.actions.map((action) => (
                      <option key={action} value={action}>
                        {action}
                      </option>
                    ))}
                  </select>
                </div>
                <label className="block text-xs">
                  Expires at
                  <input
                    className="ml-2 rounded-lg border border-[var(--line)] bg-transparent p-2"
                    type="datetime-local"
                    value={audience.expires_at}
                    onChange={(e) =>
                      setAudience({ ...audience, expires_at: e.target.value })
                    }
                  />
                </label>
                <Button
                  disabled={pending || !audience.expires_at}
                  variant="secondary"
                  onClick={() => void invite(item.id)}
                >
                  Invite audience
                </Button>
                <ul className="space-y-2 text-xs">
                  {item.invitations?.map((inv) => (
                    <li
                      key={inv.id}
                      className="flex items-center justify-between gap-2"
                    >
                      <span>
                        {inv.user_id} · {inv.role} ·{" "}
                        {inv.revoked_at
                          ? "revoked"
                          : `expires ${new Date(inv.expires_at).toLocaleString()}`}
                      </span>
                      {!inv.revoked_at && (
                        <Button
                          variant="quiet"
                          onClick={() => void revoke(item.id, inv.id)}
                        >
                          Revoke
                        </Button>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            )}
            {participant && item.definition.access.actions.includes("feedback") && (
              <div className="mt-4 space-y-3 border-t border-[var(--line)] pt-4">
                <div>
                  <p className="text-sm font-semibold">Revision-exact findings</p>
                  <p className="text-xs text-[var(--muted)]">Feedback stays with this preview audience and revision. Sensitive console fields are redacted before retention.</p>
                </div>
                <div className="grid gap-2 sm:grid-cols-2">
                  <input className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm" value={finding.route} onChange={(e)=>setFinding({...finding,route:e.target.value})} placeholder="Current route, e.g. /checkout" />
                  <input className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm" value={finding.title} onChange={(e)=>setFinding({...finding,title:e.target.value})} placeholder="What did you observe?" />
                  <select className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm" value={finding.classification} onChange={(e)=>setFinding({...finding,classification:e.target.value})}>{["bug","usability","accessibility","content","performance","question","other"].map(v=><option key={v}>{v}</option>)}</select>
                  <select className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm" value={finding.severity} onChange={(e)=>setFinding({...finding,severity:e.target.value})}>{["blocking","major","minor","note"].map(v=><option key={v}>{v}</option>)}</select>
                  <textarea className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm sm:col-span-2" value={finding.description} onChange={(e)=>setFinding({...finding,description:e.target.value})} placeholder="Expected and observed behavior" />
                  <textarea className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm" value={finding.steps} onChange={(e)=>setFinding({...finding,steps:e.target.value})} placeholder="One reproduction step per line" />
                  <textarea className="rounded-lg border border-[var(--line)] bg-transparent p-2 font-mono text-xs" value={finding.console} onChange={(e)=>setFinding({...finding,console:e.target.value})} placeholder="Optional console output" />
                  <label className="rounded-lg border border-[var(--line)] p-2 text-xs">Screenshots, recordings, traces, or annotations<input className="mt-1 block w-full" type="file" multiple accept="image/png,image/jpeg,image/webp,video/webm,video/mp4,application/json,text/plain" onChange={(e)=>setUploads(Array.from(e.target.files??[]))}/></label>
                  <select className="rounded-lg border border-[var(--line)] bg-transparent p-2 text-sm" value={finding.duplicate_of} onChange={(e)=>setFinding({...finding,duplicate_of:e.target.value})}><option value="">Not a duplicate</option>{item.findings?.map(f=><option key={f.id} value={f.id}>Duplicate of {f.title}</option>)}</select>
                  <Button disabled={pending || !finding.title || !finding.route} onClick={()=>void createFinding(item.id)}>Record finding</Button>
                </div>
                <div className="space-y-3">
                  {item.findings?.map((entry)=><div key={entry.id} className="rounded-lg border border-[var(--line)] p-3 text-sm">
                    <div className="flex flex-wrap items-center justify-between gap-2"><p className="font-semibold">{entry.title}</p><Badge tone={entry.status==="resolved"?"success":entry.severity==="blocking"?"danger":"warning"}>{entry.status} · {entry.severity}</Badge></div>
                    <p className="mt-1 text-xs text-[var(--muted)]"><code>{entry.route}</code> · revision <code>{entry.revision.slice(0,12)}</code> · {entry.classification} · by {entry.author_id}</p>
                    {entry.description && <p className="mt-2 whitespace-pre-wrap">{entry.description}</p>}
                    {entry.reproduction_steps.length>0 && <ol className="mt-2 list-decimal space-y-1 pl-5">{entry.reproduction_steps.map((step,index)=><li key={`${entry.id}-step-${index}`}>{step}</li>)}</ol>}
                    {entry.evidence.length>0 && <p className="mt-2 text-xs">Evidence: {entry.evidence.map(e=>`${e.kind} ${e.name}${e.redacted?" (redacted)":""}`).join(", ")}</p>}
                    {entry.duplicate_of && <p className="mt-2 text-xs">Linked duplicate of <code>{entry.duplicate_of.slice(0,8)}</code></p>}
                    <div className="mt-2 flex gap-2"><Button variant="quiet" disabled={pending} onClick={()=>void decide(item.id,entry.id,entry.version,{status:entry.status==="open"?"resolved":"open"})}>{entry.status==="open"?"Resolve":"Reopen"}</Button></div>
                    {entry.comments.map(c=><p key={c.id} className="mt-2 rounded bg-[var(--surface-subtle)] p-2"><span className="font-semibold">{c.author_id}</span> {c.body}</p>)}
                    <div className="mt-2 flex gap-2"><input className="min-w-0 flex-1 rounded-lg border border-[var(--line)] bg-transparent p-2" value={comments[entry.id]??""} onChange={(e)=>setComments({...comments,[entry.id]:e.target.value})} placeholder="Discuss this finding"/><Button variant="secondary" disabled={pending} onClick={()=>void comment(item.id,entry.id,entry.version)}>Comment</Button></div>
                  </div>)}
                </div>
              </div>
            )}
            {item.stale && (
              <p className="mt-3 text-xs text-[var(--warning)]">
                The pull request has moved. This retained URL still represents
                the revision guests evaluated.
              </p>
            )}
          </Card>
        ))
      )}
    </section>
  );
}
