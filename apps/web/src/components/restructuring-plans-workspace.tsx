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
