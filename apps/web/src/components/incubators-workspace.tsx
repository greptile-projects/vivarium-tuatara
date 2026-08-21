"use client";

import { useEffect, useState, type FormEvent } from "react";
import { api, type Incubator } from "@/lib/api";
import { AccessGate, useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

const lines = (value: FormDataEntryValue | null) =>
  String(value ?? "")
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
export function IncubatorsWorkspace() {
  const { user, token } = useAuth();
  const [items, setItems] = useState<Incubator[]>([]);
  const [current, setCurrent] = useState<Incubator | null>(null);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const load = async () => {
    if (!token) return;
    try {
      const r = await api<{ incubators: Incubator[] }>(
        "/incubators",
        {},
        token,
      );
      setItems(r.incubators);
      if (current) {
        setCurrent(r.incubators.find((x) => x.id === current.id) ?? null);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unable to load incubators.");
    }
  };
  useEffect(() => {
    void load();
  }, [token]);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !user) return;
    setPending(true);
    setError("");
    const f = new FormData(e.currentTarget);
    const kind = String(f.get("source_kind"));
    try {
      const x = await api<Incubator>(
        "/incubators",
        {
          method: "POST",
          body: JSON.stringify({
            title: f.get("title"),
            audience: f.get("audience"),
            problem: f.get("problem"),
            desired_outcome: f.get("desired_outcome"),
            constraints: lines(f.get("constraints")),
            success_measures: lines(f.get("success_measures")),
            sponsor_ids: [user.id, ...lines(f.get("sponsors"))],
            visibility: f.get("visibility"),
            source: {
              kind,
              label: f.get("source_label"),
              repository_id:
                kind === "new_idea" ? undefined : f.get("repository_id"),
              resource_id:
                kind === "new_idea" ? undefined : f.get("resource_id"),
            },
            decision_rights: [
              {
                kind: "scope_change",
                decision: f.get("decision"),
                principal_ids: [user.id],
                rule: f.get("decision_rule"),
              },
            ],
            invitations: lines(f.get("invitees")).map((v) => {
              const [id, role] = v.split("|").map((s) => s.trim());
              return {
                principal_type: "human",
                principal_id: id,
                role: role || "participant",
              };
            }),
          }),
        },
        token,
      );
      setItems((v) => [x, ...v]);
      setCurrent(x);
      setCreating(false);
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Unable to open incubator.",
      );
    } finally {
      setPending(false);
    }
  }
  async function append(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !current) return;
    setPending(true);
    const f = new FormData(e.currentTarget);
    try {
      const x = await api<Incubator>(
        `/incubators/${current.id}/events`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: current.version,
            kind: f.get("kind"),
            body: f.get("body"),
            visibility: "participants",
          }),
        },
        token,
      );
      setCurrent(x);
      setItems((v) => v.map((i) => (i.id === x.id ? x : i)));
      e.currentTarget.reset();
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Unable to append context.",
      );
    } finally {
      setPending(false);
    }
  }
  async function consent(invitation: string, decision: string) {
    if (!token || !current) return;
    try {
      const x = await api<Incubator>(
        `/incubators/${current.id}/invitations/${invitation}/consent`,
        {
          method: "POST",
          body: JSON.stringify({ expected_version: current.version, decision }),
        },
        token,
      );
      setCurrent(x);
      setItems((v) => v.map((i) => (i.id === x.id ? x : i)));
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Unable to record consent.",
      );
    }
  }
  return (
    <AccessGate>
      <div className="space-y-6">
        <header className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <p className="font-mono text-xs font-semibold uppercase tracking-widest text-[var(--brand)]">
              Before the repository
            </p>
            <h1 className="mt-2 text-3xl font-semibold">Project incubators</h1>
            <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
              Establish why a project should exist, who it serves, and who may
              shape it before implementation choices harden.
            </p>
          </div>
          <Button onClick={() => setCreating(!creating)}>
            {creating ? "Cancel" : "Open an incubator"}
          </Button>
        </header>
        {error && (
          <p
            role="alert"
            className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
          >
            {error}
          </p>
        )}
        {creating && (
          <Card className="p-5">
            <form onSubmit={create} className="grid gap-4 md:grid-cols-2">
              <Field name="title" label="Working title" />
              <Select
                name="source_kind"
                label="Starting point"
                values={[
                  "new_idea",
                  "feedback",
                  "support_gap",
                  "governed_proposal",
                ]}
              />
              <Field name="source_label" label="Source label" />
              <Field
                name="repository_id"
                label="Source repository ID"
                optional
              />
              <Field name="resource_id" label="Source resource ID" optional />
              <Select
                name="visibility"
                label="Visibility"
                values={["participants", "private", "public"]}
              />
              <Area name="audience" label="Audience" />
              <Area name="problem" label="Problem" />
              <Area name="desired_outcome" label="Desired outcome" />
              <Area
                name="constraints"
                label="Constraints"
                hint="One per line"
              />
              <Area
                name="success_measures"
                label="Success measures"
                hint="One per line"
              />
              <Area
                name="sponsors"
                label="Additional sponsor IDs"
                hint="One per line"
              />
              <Field
                name="decision"
                label="Decision right"
                placeholder="Who may change scope?"
              />
              <Select
                name="decision_rule"
                label="Decision rule"
                values={["owner", "consent", "majority", "consensus"]}
              />
              <Area
                name="invitees"
                label="Human invitations"
                hint="identity ID | role, one per line"
              />
              <Button disabled={pending} className="md:col-span-2">
                {pending ? "Opening…" : "Open collaborative home"}
              </Button>
            </form>
          </Card>
        )}
        <div className="grid gap-5 lg:grid-cols-[22rem_1fr]">
          <section className="space-y-3">
            {items.map((x) => (
              <button
                key={x.id}
                onClick={() => setCurrent(x)}
                className="w-full rounded-xl border bg-white p-4 text-left"
              >
                <div className="flex gap-2">
                  <Badge
                    tone={
                      x.source.resolution === "resolved" ? "success" : "warning"
                    }
                  >
                    {x.source.resolution}
                  </Badge>
                  <Badge>{x.visibility}</Badge>
                </div>
                <h2 className="mt-2 font-semibold">{x.title}</h2>
                <p className="mt-1 line-clamp-2 text-xs text-[var(--muted)]">
                  {x.problem}
                </p>
              </button>
            ))}
            {items.length === 0 && (
              <Card className="p-6 text-sm text-[var(--muted)]">
                No incubators are visible to you yet.
              </Card>
            )}
          </section>
          {current ? (
            <Card className="p-6">
              <div className="flex flex-wrap gap-2">
                <Badge
                  tone={
                    current.source.resolution === "resolved"
                      ? "success"
                      : "warning"
                  }
                >
                  {current.source.kind}: {current.source.resolution}
                </Badge>
                <Badge>{current.visibility}</Badge>
              </div>
              <h2 className="mt-3 text-2xl font-semibold">{current.title}</h2>
              <p className="mt-1 text-sm text-[var(--muted)]">
                {current.source.detail}
              </p>
              <dl className="mt-6 grid gap-4 sm:grid-cols-2">
                <Fact t="Audience" v={current.audience} />
                <Fact t="Problem" v={current.problem} />
                <Fact t="Desired outcome" v={current.desired_outcome} />
                <Fact t="Success" v={current.success_measures.join(" · ")} />
              </dl>
              {current.potential_duplicates.length > 0 && (
                <div className="mt-5 rounded-lg bg-[var(--warning-soft)] p-4">
                  <h3 className="font-semibold text-[var(--warning)]">
                    Potential duplicate initiatives
                  </h3>
                  {current.potential_duplicates.map((d) => (
                    <p key={d.incubator_id} className="mt-1 text-sm">
                      {d.title} — {d.reason}
                    </p>
                  ))}
                </div>
              )}
              <h3 className="mt-6 font-semibold">Participants and consent</h3>
              <div className="mt-2 space-y-2">
                {current.invitations.map((i) => (
                  <div
                    key={i.id}
                    className="flex items-center gap-2 rounded-lg border p-3 text-sm"
                  >
                    <span className="flex-1">
                      {i.principal_type} {i.principal_id} · {i.role}
                    </span>
                    <Badge>{i.status}</Badge>
                    {i.principal_id === user?.id && i.status === "pending" && (
                      <>
                        <Button
                          variant="secondary"
                          onClick={() => void consent(i.id, "declined")}
                        >
                          Decline
                        </Button>
                        <Button onClick={() => void consent(i.id, "accepted")}>
                          Accept
                        </Button>
                      </>
                    )}
                  </div>
                ))}
              </div>
              <h3 className="mt-6 font-semibold">Attributable context</h3>
              <div className="mt-2 space-y-2">
                {current.events.map((e) => (
                  <div key={e.id} className="rounded-lg border p-3">
                    <div className="flex gap-2">
                      <Badge>{e.kind}</Badge>
                      <span className="text-xs text-[var(--muted)]">
                        {e.actor_type} {e.actor_id}
                      </span>
                    </div>
                    <p className="mt-2 text-sm">{e.body}</p>
                  </div>
                ))}
              </div>
              <form
                onSubmit={append}
                className="mt-5 grid gap-3 sm:grid-cols-[11rem_1fr_auto]"
              >
                <Select
                  name="kind"
                  label="Context type"
                  values={[
                    "discussion",
                    "evidence",
                    "assumption",
                    "scope_change",
                  ]}
                />
                <Area name="body" label="Add context" />
                <Button disabled={pending} className="self-end">
                  Append
                </Button>
              </form>
            </Card>
          ) : (
            <Card className="p-8 text-sm text-[var(--muted)]">
              Select an incubator to inspect its purpose, authority, consent,
              and discussion.
            </Card>
          )}
        </div>
      </div>
    </AccessGate>
  );
}
function Field({
  name,
  label,
  optional = false,
  placeholder,
}: {
  name: string;
  label: string;
  optional?: boolean;
  placeholder?: string;
}) {
  return (
    <label className="text-sm font-semibold">
      {label}
      <input
        required={!optional}
        name={name}
        placeholder={placeholder}
        className="mt-2 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
      />
    </label>
  );
}
function Area({
  name,
  label,
  hint,
}: {
  name: string;
  label: string;
  hint?: string;
}) {
  return (
    <label className="text-sm font-semibold">
      {label}
      <textarea
        name={name}
        rows={3}
        className="mt-2 w-full rounded-lg border bg-white p-3 font-normal"
      />
      {hint && (
        <span className="block text-xs font-normal text-[var(--muted)]">
          {hint}
        </span>
      )}
    </label>
  );
}
function Select({
  name,
  label,
  values,
}: {
  name: string;
  label: string;
  values: string[];
}) {
  return (
    <label className="text-sm font-semibold">
      {label}
      <select
        name={name}
        className="mt-2 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
      >
        {values.map((v) => (
          <option key={v}>{v}</option>
        ))}
      </select>
    </label>
  );
}
function Fact({ t, v }: { t: string; v: string }) {
  return (
    <div>
      <dt className="text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">
        {t}
      </dt>
      <dd className="mt-1 text-sm leading-6">{v}</dd>
    </div>
  );
}
