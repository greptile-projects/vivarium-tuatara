"use client";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
type Report = {
  id: string;
  reporter_id?: string;
  target: {
    kind: string;
    resource_id: string;
    revision: string;
    location?: string;
  };
  access_needs: string[];
  expected_outcome: string;
  steps: string[];
  reporter_environment: {
    browser: string;
    browser_version?: string;
    device?: string;
    operating_system?: string;
    assistive_technology: string;
    assistive_technology_version?: string;
    input_mode?: string;
  };
  evidence: {
    kind: string;
    description: string;
    content_ref: string;
    redacted: boolean;
  }[];
  attempts: {
    id: string;
    runner_id: string;
    boundary: string;
    outcome: string;
    notes: string;
    created_at: string;
  }[];
};
const val = (f: FormData, n: string) => String(f.get(n) ?? "").trim();
export function AccessibilityReportsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token } = useAuth(),
    [reports, setReports] = useState<Report[]>([]),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    if (!token) return;
    try {
      setReports(
        (
          await api<{ reports: Report[] }>(
            `/repositories/${repositoryID}/accessibility-reports`,
            {},
            token,
          )
        ).reports,
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Reports could not be loaded.");
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function report(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    const form = e.currentTarget;
    setBusy(true);
    setError("");
    const f = new FormData(form);
    try {
      await api(
        `/repositories/${repositoryID}/accessibility-reports`,
        {
          method: "POST",
          body: JSON.stringify({
            target: {
              kind: val(f, "kind"),
              resource_id: val(f, "resource"),
              revision: val(f, "revision"),
              location: val(f, "location"),
            },
            access_needs: val(f, "needs")
              .split(",")
              .map((x) => x.trim())
              .filter(Boolean),
            expected_outcome: val(f, "expected"),
            steps: val(f, "steps")
              .split("\n")
              .map((x) => x.trim())
              .filter(Boolean),
            reporter_environment: {
              browser: val(f, "browser"),
              browser_version: val(f, "browser_version"),
              device: val(f, "device"),
              operating_system: val(f, "os"),
              assistive_technology: val(f, "at"),
              assistive_technology_version: val(f, "at_version"),
              input_mode: val(f, "input"),
            },
            evidence: [
              {
                kind: val(f, "evidence_kind"),
                description: val(f, "evidence_description"),
                content_ref: val(f, "evidence_ref"),
                redacted: true,
              },
            ],
            consent: {
              share_identity: f.get("identity") === "on",
              share_device_details: f.get("device_consent") === "on",
            },
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Report could not be submitted.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function attempt(e: FormEvent<HTMLFormElement>, id: string) {
    e.preventDefault();
    if (!token) return;
    const f = new FormData(e.currentTarget);
    try {
      await api(
        `/repositories/${repositoryID}/accessibility-reports/${id}/attempts`,
        {
          method: "POST",
          body: JSON.stringify({
            boundary: val(f, "boundary"),
            environment: {
              browser: val(f, "browser"),
              browser_version: val(f, "browser_version"),
              device: val(f, "device"),
              operating_system: val(f, "os"),
              assistive_technology: val(f, "at"),
              assistive_technology_version: val(f, "at_version"),
            },
            outcome: val(f, "outcome"),
            notes: val(f, "notes"),
            evidence: [],
          }),
        },
        token,
      );
      await load();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Attempt could not be retained.",
      );
    }
  }
  return (
    <section className="space-y-4">
      <header>
        <h2 className="text-xl font-semibold">
          Barrier evidence and reproduction
        </h2>
        <p className="mt-1 max-w-3xl text-sm text-[var(--muted)]">
          Share what blocked the experience—not private medical context.
          Evidence must be redacted; identity and sensitive device details stay
          hidden unless you consent.
        </p>
      </header>
      <Card className="p-5">
        <form onSubmit={report} className="grid gap-3 md:grid-cols-3">
          <Select
            n="kind"
            l="Target"
            options={["release", "page", "documentation_journey", "preview"]}
          />
          <Field n="resource" l="Resource ID" />
          <Field n="revision" l="Exact revision" />
          <Field n="location" l="Page or journey location" />
          <Field n="needs" l="Access needs (comma separated)" />
          <Field n="expected" l="Expected outcome" />
          <Area n="steps" l="Interaction steps (one per line)" />
          <Field n="browser" l="Browser" />
          <Field n="browser_version" l="Browser version" />
          <Field n="device" l="Device description" />
          <Field n="os" l="Operating system" />
          <Field n="at" l="Assistive technology" />
          <Field n="at_version" l="Assistive technology version" />
          <Field n="input" l="Input mode" />
          <Select
            n="evidence_kind"
            l="Redacted evidence type"
            options={[
              "screenshot",
              "recording",
              "accessibility_tree",
              "speech_output",
              "input_trace",
            ]}
          />
          <Field n="evidence_description" l="Redacted evidence description" />
          <Field n="evidence_ref" l="Safe evidence reference" />
          <label className="text-sm">
            <input name="identity" type="checkbox" /> Share my identity with
            maintainers
          </label>
          <label className="text-sm">
            <input name="device_consent" type="checkbox" /> Share detailed
            device settings
          </label>
          <div>
            <Button disabled={busy}>
              {busy ? "Submitting…" : "Report barrier"}
            </Button>
          </div>
        </form>
      </Card>
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}
        </p>
      )}
      {reports.map((x) => (
        <Card className="p-5" key={x.id}>
          <div className="flex flex-wrap gap-2">
            <h3 className="font-semibold">
              {x.target.kind.replaceAll("_", " ")} · {x.target.resource_id}
            </h3>
            <Badge>{x.target.revision}</Badge>
          </div>
          <p className="mt-2 text-sm">Expected: {x.expected_outcome}</p>
          <p className="text-xs text-[var(--muted)]">
            Reporter: {x.reporter_id || "identity withheld"} ·{" "}
            {x.reporter_environment.browser} /{" "}
            {x.reporter_environment.assistive_technology}
          </p>
          <ol className="mt-3 list-decimal pl-5 text-sm">
            {x.steps.map((s, i) => (
              <li key={i}>{s}</li>
            ))}
          </ol>
          <div className="mt-3 space-y-2">
            <h4 className="text-sm font-semibold">Redacted evidence</h4>
            {x.evidence.map((artifact, i) => (
              <div
                className="rounded-lg border p-3 text-sm"
                key={`${artifact.content_ref}-${i}`}
              >
                <Badge>{artifact.kind.replaceAll("_", " ")}</Badge>
                <p className="mt-1">{artifact.description}</p>
                <code className="mt-1 block break-all text-xs text-[var(--muted)]">
                  {artifact.content_ref}
                </code>
              </div>
            ))}
          </div>
          <form
            onSubmit={(e) => attempt(e, x.id)}
            className="mt-4 grid gap-2 rounded-lg border p-3 md:grid-cols-4"
          >
            <Select
              n="boundary"
              l="Bounded run"
              options={["workspace", "preview"]}
            />
            <Select
              n="outcome"
              l="Result"
              options={[
                "reproducible",
                "intermittent",
                "environment_specific",
                "unconfirmed",
              ]}
            />
            <Field n="browser" l="Browser" />
            <Field n="browser_version" l="Browser version" />
            <Field n="device" l="Device" />
            <Field n="os" l="Operating system" />
            <Field n="at" l="Assistive technology" />
            <Field n="at_version" l="AT version" />
            <Field n="notes" l="Observed result" />
            <div>
              <Button variant="secondary">Retain attempt</Button>
            </div>
          </form>
          <div className="mt-3 space-y-1">
            {(x.attempts ?? []).map((a) => (
              <p className="text-xs" key={a.id}>
                <Badge>{a.outcome.replaceAll("_", " ")}</Badge> {a.boundary} ·{" "}
                {a.notes} · {a.runner_id}
              </p>
            ))}
          </div>
        </Card>
      ))}
    </section>
  );
}
function Field({ n, l }: { n: string; l: string }) {
  return (
    <label className="text-xs font-semibold">
      {l}
      <input
        name={n}
        required
        className="mt-1 min-h-10 w-full rounded-lg border px-3 font-normal"
      />
    </label>
  );
}
function Area({ n, l }: { n: string; l: string }) {
  return (
    <label className="text-xs font-semibold">
      {l}
      <textarea
        name={n}
        required
        rows={3}
        className="mt-1 w-full rounded-lg border p-3 font-normal"
      />
    </label>
  );
}
function Select({
  n,
  l,
  options,
}: {
  n: string;
  l: string;
  options: string[];
}) {
  return (
    <label className="text-xs font-semibold">
      {l}
      <select
        name={n}
        className="mt-1 min-h-10 w-full rounded-lg border bg-white px-3 font-normal"
      >
        {options.map((x) => (
          <option key={x}>{x}</option>
        ))}
      </select>
    </label>
  );
}
