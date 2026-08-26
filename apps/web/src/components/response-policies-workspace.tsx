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

type Diagnostic = {
  kind: string;
  severity: string;
  message: string;
  resource_id?: string;
  rule_id?: string;
  attributed_to: string;
};
type Revision = {
  title: string;
  summary: string;
  resources: Array<{
    id: string;
    kind: string;
    name: string;
    owner_team_ids: string[];
  }>;
  teams: Array<{
    id: string;
    name: string;
    member_ids: string[];
    skills: string[];
    contact: string;
  }>;
  rules: Array<{
    id: string;
    resource_ids: string[];
    signal_class: string;
    severity: string;
    accountable_team_id: string;
    required_skills: string[];
    acknowledge_seconds: number;
    resolve_seconds: number;
    expected_actions: string[];
    escalations: Array<{
      after_seconds: number;
      team_id: string;
      audience_ids: string[];
      expected_action: string;
    }>;
    communication_audience_ids: string[];
    incident_criteria: string[];
    authority: {
      required_access: string[];
      permitted_actions: string[];
      prohibited_actions: string[];
    };
  }>;
  exceptions: Array<{
    id: string;
    rule_id: string;
    reason: string;
    follow_up_id: string;
    expires_at: string;
  }>;
  change_reason: string;
  created_by?: string;
  created_at?: string;
};
type Policy = {
  id: string;
  current_version: number;
  revisions: Revision[];
  diagnostics: Diagnostic[];
  updated_at: string;
};
const template = (owner = ""): Revision => ({
  title: "Urgent response coverage",
  summary: "Accountability before an alert arrives",
  resources: [
    {
      id: "repository",
      kind: "repository",
      name: "Repository",
      owner_team_ids: ["maintainers"],
    },
  ],
  teams: [
    {
      id: "maintainers",
      name: "Maintainers",
      member_ids: owner ? [owner] : [],
      skills: ["service operations"],
      contact: "#maintainers",
    },
  ],
  rules: [
    {
      id: "reliability-critical",
      resource_ids: ["repository"],
      signal_class: "reliability",
      severity: "critical",
      accountable_team_id: "maintainers",
      required_skills: ["service operations"],
      acknowledge_seconds: 300,
      resolve_seconds: 3600,
      expected_actions: ["Assess impact and preserve evidence"],
      escalations: [],
      communication_audience_ids: ["repository owners", "support"],
      incident_criteria: ["Confirmed user impact exceeds five minutes"],
      authority: {
        required_access: ["repository:read"],
        permitted_actions: ["investigate and coordinate"],
        prohibited_actions: [
          "deploy, disclose, or spend without separate authority",
        ],
      },
    },
  ],
  exceptions: [],
  change_reason: "Initial coverage contract",
});

export function ResponsePoliciesWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { user } = useAuth();
  return (
    <ScopedResponsePoliciesWorkspace
      key={`${repositoryID}:${user?.id ?? "unauthenticated"}`}
      repositoryID={repositoryID}
    />
  );
}

function ScopedResponsePoliciesWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth();
  const [items, setItems] = useState<Policy[]>([]),
    [selected, setSelected] = useState<Policy>(),
    [draft, setDraft] = useState(""),
    [error, setError] = useState(""),
    [publishing, setPublishing] = useState(false);
  const publishingRef = useRef(false);
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const out = await api<{ response_policies: Policy[] }>(
        `/repositories/${repositoryID}/response-policies`,
        {},
        token,
      );
      setItems(out.response_policies);
      setSelected(out.response_policies[0]);
      setDraft(JSON.stringify(template(user?.id), null, 2));
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Response coverage could not be loaded",
      );
    }
  }, [repositoryID, token, user]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function publish(e: FormEvent) {
    e.preventDefault();
    if (!token || publishingRef.current) return;
    publishingRef.current = true;
    setPublishing(true);
    try {
      const out = await api<Policy>(
        selected
          ? `/repositories/${repositoryID}/response-policies/${selected.id}/revisions`
          : `/repositories/${repositoryID}/response-policies`,
        {
          method: "POST",
          body: JSON.stringify({
            request_id: crypto.randomUUID(),
            expected_version: selected?.current_version,
            revision: JSON.parse(draft),
          }),
        },
        token,
      );
      setItems((v) => [out, ...v.filter((x) => x.id !== out.id)]);
      setSelected(out);
      setDraft(JSON.stringify(out.revisions.at(-1), null, 2));
      setError("");
    } catch (x) {
      setError(
        x instanceof Error
          ? x.message
          : "Response policy could not be published",
      );
    } finally {
      publishingRef.current = false;
      setPublishing(false);
    }
  }
  const current = selected?.revisions.at(-1);
  return (
    <main className="mx-auto w-full max-w-6xl space-y-7 px-6 py-8">
      <header>
        <p className="text-sm font-semibold text-[var(--accent)]">
          On-call coordination
        </p>
        <h1 className="mt-2 text-3xl font-semibold">Response coverage</h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Define who responds to each urgent condition, their skills, targets,
          escalation, audiences, incident threshold, and exact authority
          boundary before a signal arrives.
        </p>
      </header>
      <div className="grid gap-6 lg:grid-cols-[.8fr_1.2fr]">
        <section className="space-y-3">
          {items.map((x) => (
            <button
              key={x.id}
              className="w-full rounded-xl border p-4 text-left"
              disabled={publishing}
              onClick={() => {
                setSelected(x);
                setDraft(JSON.stringify(x.revisions.at(-1), null, 2));
              }}
            >
              <Badge>v{x.current_version}</Badge>
              <strong className="ml-2">{x.revisions.at(-1)?.title}</strong>
              <p className="mt-2 text-xs text-[var(--muted)]">
                {x.diagnostics.length} visible coverage gap(s)
              </p>
            </button>
          ))}
          {current && (
            <Card className="p-5">
              <h2 className="font-semibold">Current responsibility map</h2>
              {current.rules.map((rule) => (
                <article key={rule.id} className="mt-4 border-l-2 pl-3">
                  <div className="flex gap-2">
                    <Badge
                      tone={rule.severity === "critical" ? "danger" : "warning"}
                    >
                      {rule.severity}
                    </Badge>
                    <Badge>{rule.signal_class}</Badge>
                  </div>
                  <p className="mt-2 font-medium">
                    {rule.accountable_team_id} · acknowledge{" "}
                    {rule.acknowledge_seconds}s · resolve {rule.resolve_seconds}
                    s
                  </p>
                  <p className="mt-1 text-sm">
                    {rule.expected_actions.join(" · ")}
                  </p>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    Authority permits{" "}
                    {rule.authority.permitted_actions.join(", ") ||
                      "nothing declared"}
                    ; prohibits{" "}
                    {rule.authority.prohibited_actions.join(", ") ||
                      "nothing declared"}
                    .
                  </p>
                </article>
              ))}
              {selected!.diagnostics.map((d, i) => (
                <p key={`${d.kind}-${i}`} className="mt-3 text-sm">
                  <Badge
                    tone={d.severity === "blocking" ? "danger" : "warning"}
                  >
                    {d.kind}
                  </Badge>{" "}
                  {d.message}{" "}
                  <span className="text-[var(--muted)]">
                    — {d.attributed_to}
                  </span>
                </p>
              ))}
            </Card>
          )}
        </section>
        <Card className="p-5">
          <div className="flex items-center justify-between">
            <h2 className="font-semibold">
              {selected
                ? `Publish successor v${selected.current_version + 1}`
                : "Define a policy"}
            </h2>
            <Button
                type="button"
                variant="secondary"
                disabled={publishing}
              onClick={() => {
                setSelected(undefined);
                setDraft(JSON.stringify(template(user?.id), null, 2));
              }}
            >
              New policy
            </Button>
          </div>
          <form onSubmit={publish}>
            <textarea
              aria-label="Response policy JSON"
              rows={34}
              className="mt-3 w-full rounded-lg border bg-slate-950 p-4 font-mono text-xs text-slate-100"
              value={draft}
              disabled={publishing}
              onChange={(e) => setDraft(e.target.value)}
            />
            <Button className="mt-3" disabled={publishing}>
              {publishing ? "Publishing…" : "Publish immutable coverage"}
            </Button>
          </form>
          {error && (
            <p role="alert" className="mt-3 text-sm text-[var(--danger)]">
              {error}
            </p>
          )}
          <p className="mt-4 text-xs text-[var(--muted)]">
            This policy coordinates response. It grants no repository, secret,
            deployment, disclosure, incident, spending, or operational
            authority.
          </p>
        </Card>
      </div>
    </main>
  );
}
