"use client";

import Link from "next/link";
import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
import { api, APIError, type DependencyInventory, type PackageVersion } from "@/lib/api";
import { PackageArtifactDownload } from "./package-artifact-download";

type Catalog = { packages: PackageVersion[] };
type IssuedCredential = { token: string; repository_id: string; package_names: string[]; expires_at: string };

export function PackagesWorkspace() {
  const { token } = useAuth();
  const [items, setItems] = useState<PackageVersion[]>([]);
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [issued, setIssued] = useState<IssuedCredential | null>(null);
  const [consumers, setConsumers] = useState<Record<string, DependencyInventory[]>>({});

  useEffect(() => {
    let active = true;
    api<Catalog>("/packages", {}, token).then((result) => { if (active) setItems(result.packages ?? []); }).catch((reason: unknown) => { if (active) setError(reason instanceof APIError ? reason.message : "Packages could not be loaded."); }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [token]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return needle ? items.filter((item) => `${item.name} ${item.summary ?? ""} ${item.documentation ?? ""}`.toLowerCase().includes(needle)) : items;
  }, [items, query]);

  async function createCredential(event: FormEvent<HTMLFormElement>, packageName: string) {
    event.preventDefault();
    if (!token) return;
    const data = new FormData(event.currentTarget);
    setError(""); setIssued(null);
    try {
      setIssued(await api<IssuedCredential>(`/repositories/${data.get("repository_id")}/package-credentials`, { method: "POST", body: JSON.stringify({ name: `Install ${packageName}`, package_names: [packageName], expires_in: 3600 }) }, token));
    } catch (reason) { setError(reason instanceof APIError ? reason.message : "Credential could not be created."); }
  }

  async function loadConsumers(item: PackageVersion) {
    const key = `${item.name}@${item.version}`;
    try { const result = await api<{ consumers: DependencyInventory[] }>(`/packages/${item.name}/versions/${encodeURIComponent(item.version)}/consumers`, {}, token); setConsumers((current) => ({ ...current, [key]: result.consumers })); }
    catch (reason) { setError(reason instanceof APIError ? reason.message : "Consumers could not be loaded."); }
  }

  async function changeLifecycle(event: FormEvent<HTMLFormElement>, item: PackageVersion) {
    event.preventDefault();
    const data = new FormData(event.currentTarget); setError("");
    const replacementName = String(data.get("replacement_name") ?? "").trim();
    const replacementVersion = String(data.get("replacement_version") ?? "").trim();
    try {
      const changed = await api<PackageVersion>(`/repositories/${item.repository_id}/packages/${item.name}/versions/${encodeURIComponent(item.version)}`, { method: "PATCH", body: JSON.stringify({ lifecycle: data.get("lifecycle"), warning: data.get("warning"), reason: data.get("reason"), replacement: replacementName && replacementVersion ? { name: replacementName, version: replacementVersion } : undefined }) }, token);
      setItems((current) => current.map((value) => value.id === changed.id ? changed : value));
    } catch (reason) { setError(reason instanceof APIError ? reason.message : "Lifecycle decision could not be recorded."); }
  }

  async function openRecovery(event: FormEvent<HTMLFormElement>, item: PackageVersion) {
    event.preventDefault(); const data = new FormData(event.currentTarget); setError("");
    try {
      const update = await api<{ repository_id: string; proposal_id: string }>(`/repositories/${data.get("repository_id")}/package-recoveries`, { method: "POST", body: JSON.stringify({ package_name: item.name, version: item.version, assignee_type: data.get("assignee_type"), assignee_id: data.get("assignee_id") }) }, token);
      window.location.assign(`/proposals/${update.repository_id}/${update.proposal_id}`);
    } catch (reason) { setError(reason instanceof APIError ? reason.message : "Recovery work could not be opened."); }
  }

  return <div className="space-y-6">
    <header className="max-w-3xl"><p className="text-xs font-bold uppercase tracking-[0.16em] text-[var(--brand)]">Package registry</p><h1 className="mt-2 text-3xl font-bold tracking-tight">Choose dependencies from inspectable evidence</h1><p className="mt-3 text-sm leading-6 text-[var(--muted)]">Search public packages and private packages you can currently read. Every version connects its documentation and compatibility claims to exact source, build, artifact, and checksum provenance.</p></header>
    <Card className="p-4"><label className="text-sm font-semibold">Search packages<input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Name, purpose, or documentation" className="mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal" /></label></Card>
    {error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">{error}</p>}
    {issued && <Card className="border-[var(--brand)] p-4"><p className="font-semibold">One-hour repository package credential</p><p className="mt-1 text-xs text-[var(--muted)]">Copy it now. It resolves only {issued.package_names.join(", ")} and carries no Git, repository mutation, or publisher authority.</p><code className="mt-3 block overflow-x-auto rounded-lg bg-[#172019] p-3 text-xs text-white">{issued.token}</code></Card>}
    {loading ? <p className="text-sm text-[var(--muted)]">Loading package evidence…</p> : filtered.length === 0 ? <Card className="p-8 text-center text-sm text-[var(--muted)]">No authorized package versions match this search.</Card> : <div className="grid gap-4">{filtered.map((item) => <Card key={item.id} className="p-5">
      <div className="flex flex-wrap items-start justify-between gap-3"><div><div className="flex flex-wrap items-center gap-2"><h2 className="text-lg font-semibold">{item.name} <span className="font-mono text-sm text-[var(--muted)]">{item.version}</span></h2><Badge tone={item.lifecycle === "active" ? "success" : item.lifecycle === "yanked" ? "danger" : "warning"}>{item.lifecycle}</Badge><Badge>{item.visibility}</Badge></div><p className="mt-1 text-sm text-[var(--muted)]">{item.summary || "No summary supplied."}</p></div><PackageArtifactDownload name={item.name} version={item.version} /></div>
      {item.lifecycle_warning && <div className={`mt-4 rounded-lg p-3 text-sm ${item.lifecycle === "quarantined" || item.lifecycle === "yanked" ? "bg-[var(--danger-soft)] text-[var(--danger)]" : "bg-[var(--warning-soft)] text-[var(--warning)]"}`}><p><strong>{item.lifecycle === "quarantined" ? "Quarantined" : "Lifecycle warning"}:</strong> {item.lifecycle_warning}</p>{item.lifecycle_reason && <p className="mt-1"><strong>Publisher reason:</strong> {item.lifecycle_reason}</p>}{item.replacement && <p className="mt-1"><strong>Safe replacement:</strong> {item.replacement.name} {item.replacement.version}</p>}<p className="mt-1 text-xs">Published bytes, checksum, source, build, consumers, and historical deployments remain retained.</p></div>}
      <div className="mt-4 grid gap-3 text-xs sm:grid-cols-2 lg:grid-cols-5"><div><span className="font-semibold">Platform</span><p className="mt-1 text-[var(--muted)]">{[item.platform.os, item.platform.architecture, item.platform.runtime].filter(Boolean).join(" / ") || "portable"}</p></div><div><span className="font-semibold">License</span><p className="mt-1 text-[var(--muted)]">{item.license || "not declared"}</p></div><div><span className="font-semibold">Support</span><p className="mt-1 text-[var(--muted)]">{item.support || "not declared"}</p></div><div><span className="font-semibold">Compatibility</span><p className="mt-1 text-[var(--muted)]">{item.dependencies.length ? item.dependencies.map((value) => `${value.name} ${value.constraint}`).join(", ") : "no dependencies"}</p></div><div><span className="font-semibold">Published</span><p className="mt-1 text-[var(--muted)]">{new Date(item.published_at).toLocaleString()}</p></div></div>
      <details className="mt-4 rounded-lg border border-[var(--line)] p-4"><summary className="cursor-pointer text-sm font-semibold">Documentation and provenance</summary><div className="mt-3 space-y-3 text-sm"><p className="whitespace-pre-wrap leading-6 text-[var(--muted)]">{item.documentation || "The publisher did not supply version documentation."}</p><dl className="grid gap-2 font-mono text-xs"><div><dt className="inline font-semibold">SHA-256: </dt><dd className="inline break-all">{item.sha256}</dd></div><div><dt className="inline font-semibold">Source: </dt><dd className="inline"><Link className="text-[var(--brand)] hover:underline" href={`/repositories/${item.repository_id}/commits/${item.source_commit}`}>{item.source_commit}</Link></dd></div><div><dt className="inline font-semibold">Build: </dt><dd className="inline">{item.build_attestation.image} · {item.build_attestation.step} · attempt {item.build_attestation.attempt}</dd></div></dl></div></details>
      {token && <details className="mt-3"><summary className="cursor-pointer text-xs font-semibold text-[var(--brand)]">Create isolated install credential</summary><form onSubmit={(event) => void createCredential(event, item.name)} className="mt-3 flex flex-wrap items-end gap-3"><label className="text-xs font-semibold">Consuming repository ID<input name="repository_id" required minLength={32} maxLength={32} className="mt-1 min-h-9 rounded-lg border border-[var(--line-strong)] px-3 font-mono font-normal" /></label><Button type="submit" variant="secondary">Create one-hour token</Button></form></details>}
      {token && item.lifecycle !== "active" && item.replacement && <details className="mt-3"><summary className="cursor-pointer text-xs font-semibold text-[var(--danger)]">Open prioritized consumer repair</summary><form onSubmit={(event) => void openRecovery(event, item)} className="mt-3 grid gap-3 sm:grid-cols-2"><label className="text-xs font-semibold">Consuming repository ID<input name="repository_id" required minLength={32} maxLength={32} className="mt-1 min-h-9 w-full rounded-lg border border-[var(--line-strong)] px-3 font-mono font-normal" /></label><label className="text-xs font-semibold">Owner<select name="assignee_type" className="mt-1 min-h-9 w-full rounded-lg border border-[var(--line-strong)] px-3 font-normal"><option value="agent">Scoped agent</option><option value="human">Human participant</option></select></label><label className="text-xs font-semibold">Human collaboration ID (human only)<input name="assignee_id" className="mt-1 min-h-9 w-full rounded-lg border border-[var(--line-strong)] px-3 font-mono font-normal" /></label><div className="flex items-end"><Button type="submit">Open urgent repair</Button></div></form></details>}
      {token && <details className="mt-3"><summary className="cursor-pointer text-xs font-semibold text-[var(--muted)]">Publisher lifecycle control</summary><form onSubmit={(event) => void changeLifecycle(event, item)} className="mt-3 grid gap-3 sm:grid-cols-2"><label className="text-xs font-semibold">Decision<select name="lifecycle" defaultValue={item.lifecycle} className="mt-1 min-h-9 w-full rounded-lg border border-[var(--line-strong)] px-3 font-normal"><option value="active">Active</option><option value="deprecated">Deprecated</option><option value="quarantined">Quarantined</option><option value="yanked">Yanked</option></select></label><label className="text-xs font-semibold">Public warning<input name="warning" defaultValue={item.lifecycle_warning} className="mt-1 min-h-9 w-full rounded-lg border border-[var(--line-strong)] px-3 font-normal" /></label><label className="text-xs font-semibold sm:col-span-2">Reason<textarea name="reason" defaultValue={item.lifecycle_reason} className="mt-1 min-h-20 w-full rounded-lg border border-[var(--line-strong)] p-3 font-normal" /></label><label className="text-xs font-semibold">Replacement package<input name="replacement_name" defaultValue={item.replacement?.name} className="mt-1 min-h-9 w-full rounded-lg border border-[var(--line-strong)] px-3 font-mono font-normal" /></label><label className="text-xs font-semibold">Replacement version<input name="replacement_version" defaultValue={item.replacement?.version} className="mt-1 min-h-9 w-full rounded-lg border border-[var(--line-strong)] px-3 font-mono font-normal" /></label><div><Button type="submit" variant="secondary">Record decision</Button></div></form></details>}
      <div className="mt-3"><Button variant="secondary" onClick={() => void loadConsumers(item)}>Show exposure and remediation</Button>{consumers[`${item.name}@${item.version}`] && <div className="mt-3 grid gap-2 text-xs text-[var(--muted)]">{consumers[`${item.name}@${item.version}`].length ? consumers[`${item.name}@${item.version}`].map((consumer) => <div key={consumer.inventory.id}><Link className="text-[var(--brand)] hover:underline" href={`/repositories/${consumer.inventory.repository_id}/dependencies`}>{consumer.inventory.repository_id.slice(0, 8)} · {consumer.current ? "current source exposure" : "historical source"} · {consumer.deployments.filter((value) => value.current).length} current deployments · {consumer.deployments.filter((value) => value.state === "succeeded" && !value.current).length} retained historical deployments</Link>{consumer.remediation && <span> · repair {consumer.remediation.status} toward {consumer.remediation.replacement_version}</span>}</div>) : "No visible consumers use this exact version."}</div>}</div>
    </Card>)}</div>}
  </div>;
}
