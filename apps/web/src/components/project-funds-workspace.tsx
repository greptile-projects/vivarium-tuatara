"use client";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { useAuth } from "@/components/auth";
import { Badge, Button, Card } from "@/components/ui";
import { api } from "@/lib/api";
import Link from "next/link";

type Fund = {
  id: string;
  version: number;
  terms: {
    name: string;
    purpose: string;
    stewards: string[];
    accepted_funding_sources: string[];
    source_verification_keys: Record<string, string>;
    unit: string;
    precision: number;
    spending_limits: { period: string; amount: number }[];
    approval_rules: {
      minimum_amount: number;
      required_approvals: number;
      eligible_approvers: string[];
    }[];
    eligible_recipients: string[];
    refund_policy: string;
    ledger_visibility: string;
  };
  balances: Record<
    "available" | "reserved" | "spent" | "refunded" | "disputed" | "pending",
    number
  >;
  ledger: {
    id: string;
    kind: string;
    amount: number;
    status: string;
    source: string;
    contributor_id: string;
    actor_id: string;
    note: string;
    created_at: string;
  }[];
  authority_note: string;
};
export function ProjectFundsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth();
  const [funds, setFunds] = useState<Fund[]>([]);
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    try {
      setFunds(
        (
          await api<{ funds: Fund[] }>(
            `/repositories/${repositoryID}/funds`,
            {},
            token,
          )
        ).funds,
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unable to load funds");
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const d = new FormData(form);
    const csv = (n: string) =>
      String(d.get(n) || "")
        .split(",")
        .map((v) => v.trim())
        .filter(Boolean);
    const sources = csv("sources");
    try {
      await api(
        `/repositories/${repositoryID}/funds`,
        {
          method: "POST",
          body: JSON.stringify({
            name: d.get("name"),
            purpose: d.get("purpose"),
            stewards: csv("stewards"),
            accepted_funding_sources: sources,
            unit: d.get("unit"),
            precision: Number(d.get("precision")),
            spending_limits: [
              { period: d.get("period"), amount: Number(d.get("limit")) },
            ],
            approval_rules: [
              {
                minimum_amount: Number(d.get("threshold")),
                required_approvals: Number(d.get("approvals")),
                eligible_approvers: csv("approvers"),
              },
            ],
            eligible_recipients: csv("recipients"),
            refund_policy: d.get("refund"),
            ledger_visibility: d.get("visibility"),
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Unable to create fund");
    }
  }
  async function commit(e: FormEvent<HTMLFormElement>, id: string) {
    e.preventDefault();
    const form = e.currentTarget;
    const d = new FormData(form);
    try {
      await api(
        `/repositories/${repositoryID}/funds/${id}/commitments`,
        {
          method: "POST",
          body: JSON.stringify({
            amount: Number(d.get("amount")),
            source: d.get("source"),
            external_reference: d.get("reference"),
            idempotency_key: crypto.randomUUID(),
            note: d.get("note"),
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Unable to commit funds");
    }
  }
  const money = (f: Fund, n: number) =>
    (n / 10 ** f.terms.precision).toLocaleString(undefined, {
      minimumFractionDigits: f.terms.precision,
      maximumFractionDigits: f.terms.precision,
    }) + ` ${f.terms.unit}`;
  return (
    <main id="main-content" className="mx-auto max-w-6xl space-y-6 p-6">
      <div>
        <Badge>Governed resources</Badge>
        <h1 className="mt-2 text-3xl font-semibold">Project funds</h1>
        <p className="mt-2 text-sm text-[var(--muted)]">
          Inspect backing, allocation authority, and rules before promising paid
          work.
        </p>
        <Link href={`/repositories/${repositoryID}/funding`} className="mt-3 inline-block text-sm font-semibold text-[var(--brand)] hover:underline">Allocate verified backing to evaluable outcomes →</Link>
      </div>
      {user && (
        <Card className="p-5">
          <h2 className="text-lg font-semibold">Establish a fund</h2>
          <form onSubmit={create} className="mt-4 grid gap-3 md:grid-cols-3">
            <Field name="name" label="Fund name" />
            <Field name="stewards" label="Steward user IDs" value={user.id} />
            <Field
              name="sources"
              label="Accepted sources"
              value="card, invoice"
            />
            <Field name="unit" label="Currency or credit type" value="USD" />
            <Field
              name="precision"
              label="Decimal places"
              type="number"
              value="2"
            />
            <Field name="period" label="Limit period" value="monthly" />
            <Field
              name="limit"
              label="Spending limit (minor units)"
              type="number"
            />
            <Field
              name="threshold"
              label="Approval threshold"
              type="number"
              value="0"
            />
            <Field
              name="approvals"
              label="Approvals required"
              type="number"
              value="1"
            />
            <Field
              name="approvers"
              label="Eligible approvers"
              value={user.id}
            />
            <Field
              name="recipients"
              label="Eligible recipients"
              value="project_contributors"
            />
            <label className="text-xs font-semibold">
              Ledger visibility
              <select
                name="visibility"
                className="mt-1 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
              >
                <option value="public">Public</option>
                <option value="participants">Project participants</option>
              </select>
            </label>
            <div className="md:col-span-3">
              <Area name="purpose" label="Purpose and intended work" />
              <Area name="refund" label="Refund policy" />
            </div>
            <Button>Create governed fund</Button>
          </form>
        </Card>
      )}
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      {funds.map((f) => (
        <Card key={f.id} className="p-5">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-xl font-semibold">{f.terms.name}</h2>
            <Badge>{f.terms.ledger_visibility} ledger</Badge>
            <Badge>v{f.version}</Badge>
          </div>
          <p className="mt-2 text-sm">{f.terms.purpose}</p>
          <div className="mt-4 grid grid-cols-2 gap-2 md:grid-cols-6">
            {Object.entries(f.balances).map(([k, v]) => (
              <div key={k} className="rounded-lg border p-3">
                <p className="text-xs capitalize text-[var(--muted)]">{k}</p>
                <p className="font-semibold">{money(f, v)}</p>
              </div>
            ))}
          </div>
          <div className="mt-4 grid gap-3 text-sm md:grid-cols-2">
            <p>
              <strong>Stewards:</strong> {f.terms.stewards.join(", ")}
            </p>
            <p>
              <strong>Eligible recipients:</strong>{" "}
              {f.terms.eligible_recipients.join(", ")}
            </p>
            <p>
              <strong>Spending:</strong>{" "}
              {f.terms.spending_limits
                .map((x) => `${money(f, x.amount)} ${x.period}`)
                .join(", ")}
            </p>
            <p>
              <strong>Approval:</strong>{" "}
              {f.terms.approval_rules
                .map(
                  (x) =>
                    `${x.required_approvals} at ${money(f, x.minimum_amount)}`,
                )
                .join(", ")}
            </p>
            <p className="md:col-span-2">
              <strong>Refund policy:</strong> {f.terms.refund_policy}
            </p>
          </div>
          {user && (
            <form
              onSubmit={(e) => commit(e, f.id)}
              className="mt-5 grid gap-3 rounded-lg border p-4 md:grid-cols-4"
            >
              <Field
                name="amount"
                label="Commit amount (minor units)"
                type="number"
              />
              <label className="text-xs font-semibold">
                Funding source
                <select
                  name="source"
                  className="mt-1 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
                >
                  {f.terms.accepted_funding_sources.map((x) => (
                    <option key={x}>{x}</option>
                  ))}
                </select>
              </label>
              <Field name="reference" label="Transfer reference" />
              <Field name="note" label="Commitment note" />
              <Button>Commit to declared terms</Button>
            </form>
          )}
          <details className="mt-4">
            <summary className="cursor-pointer font-semibold">
              Ledger · {f.ledger.length} entries
            </summary>
            {f.ledger.map((x) => (
              <p key={x.id} className="mt-2 rounded-lg border p-3 text-sm">
                <Badge>{x.status}</Badge> {money(f, x.amount)} ·{" "}
                {x.kind.replaceAll("_", " ")} · {x.actor_id}
                <span className="block text-xs text-[var(--muted)]">
                  {x.source} · {x.note || "No note"} ·{" "}
                  {new Date(x.created_at).toLocaleString()}
                </span>
              </p>
            ))}
          </details>
          <p className="mt-4 text-xs text-[var(--muted)]">{f.authority_note}</p>
        </Card>
      ))}
    </main>
  );
}
function Field({
  name,
  label,
  value,
  type = "text",
}: {
  name: string;
  label: string;
  value?: string;
  type?: string;
}) {
  return (
    <label className="text-xs font-semibold">
      {label}
      <input
        name={name}
        required
        type={type}
        defaultValue={value}
        className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal"
      />
    </label>
  );
}
function Area({ name, label }: { name: string; label: string }) {
  return (
    <label className="block text-xs font-semibold">
      {label}
      <textarea
        name={name}
        required
        rows={2}
        className="mt-1 w-full rounded-lg border p-3 font-normal"
      />
    </label>
  );
}
