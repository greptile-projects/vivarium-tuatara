"use client";
import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, type CharterResponse } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

export function CharterWorkspace({
  kind,
  id,
}: {
  kind: "repository" | "organization";
  id: string;
}) {
  const { token, user, loading: authLoading } = useAuth();
  const [data, setData] = useState<CharterResponse | null>(null);
  const [missing, setMissing] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const base =
    kind === "repository"
      ? `/repositories/${id}/charter`
      : `/organizations/${id}/charter`;
  const back =
    kind === "repository" ? `/repositories/${id}` : `/organizations/${id}`;
  const load = useCallback(async () => {
    if (authLoading) return;
    try {
      setData(await api<CharterResponse>(base, {}, token));
      setMissing(false);
    } catch (e) {
      const m = e instanceof Error ? e.message : "Charter unavailable";
      if (m.toLowerCase().includes("no charter")) {
        setMissing(true);
      } else setError(m);
    }
  }, [authLoading, base, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function publish(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token) return;
    setPending(true);
    setError("");
    const f = new FormData(e.currentTarget);
    const lines = (n: string) =>
      String(f.get(n) || "")
        .split("\n")
        .map((x) => x.trim())
        .filter(Boolean);
    const role = String(f.get("role"));
    try {
      await api(
        base + "/revisions",
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: data?.charter.revisions.length ?? 0,
            charter: {
              title: f.get("title"),
              summary: f.get("summary"),
              roles: [
                {
                  name: role,
                  description: f.get("role_description"),
                  eligibility: lines("eligibility"),
                },
              ],
              decision_classes: [
                {
                  name: f.get("decision"),
                  description: f.get("decision_description"),
                  eligible_roles: [role],
                  participation: Number(f.get("participation")),
                  quorum: Number(f.get("quorum")),
                  approval: f.get("approval"),
                  protected_resources: lines("resources"),
                },
              ],
              procedures: {
                terms: f.get("terms"),
                removal: f.get("removal"),
                succession: f.get("succession"),
                amendments: f.get("amendments"),
              },
            },
          }),
        },
        token,
      );
      await load();
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Could not publish charter");
    } finally {
      setPending(false);
    }
  }
  async function act(path: string, body: unknown) {
    if (!token) return;
    setPending(true);
    setError("");
    try {
      await api(
        base + path,
        { method: "POST", body: JSON.stringify(body) },
        token,
      );
      await load();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Charter action failed");
    } finally {
      setPending(false);
    }
  }
  async function invite(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !data?.charter.active_version) return;
    const f = new FormData(e.currentTarget);
    setPending(true);
    setError("");
    try {
      await api(
        base + "/standing",
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: data.charter.standings.length,
            charter_version: data.charter.active_version,
            principal_type: "human",
            principal_id: f.get("principal_id"),
            role: f.get("role"),
            responsibilities: f.get("responsibilities"),
            evidence: [
              {
                kind: f.get("evidence_kind"),
                resource_id: f.get("resource_id"),
                summary: f.get("evidence_summary"),
              },
            ],
            expires_at: new Date(String(f.get("expires_at"))).toISOString(),
          }),
        },
        token,
      );
      await load();
      e.currentTarget.reset();
    } catch (x) {
      setError(x instanceof Error ? x.message : "Could not invite participant");
    } finally {
      setPending(false);
    }
  }
  async function continuity(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!token || !data?.charter.active_version) return;
    const f = new FormData(e.currentTarget);
    setPending(true);
    setError("");
    try {
      await api(
        base + "/continuity",
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: data.continuity.length,
            charter_version: data.charter.active_version,
            action: {
              kind: f.get("kind"),
              role: f.get("role"),
              from_standing_id: f.get("from"),
              to_standing_id: f.get("to"),
              governance_proposal_id: f.get("proposal"),
              reason: f.get("reason"),
              resources: String(f.get("resources"))
                .split("\n")
                .map((x) => x.trim())
                .filter(Boolean),
              review_at: new Date(String(f.get("review_at"))).toISOString(),
              expires_at: new Date(String(f.get("expires_at"))).toISOString(),
            },
          }),
        },
        token,
      );
      await load();
      e.currentTarget.reset();
    } catch (x) {
      setError(
        x instanceof Error ? x.message : "Could not record continuity action",
      );
    } finally {
      setPending(false);
    }
  }
  const current = data?.charter.revisions.at(-1);
  return (
    <div className="space-y-6">
      <header>
        <Link href={back} className="text-sm font-semibold text-[var(--brand)]">
          Back to {kind}
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">Project charter</h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          The written source of decision rights, participation rules,
          stewardship continuity, and amendment authority.
        </p>
      </header>
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      {current && data && (
        <>
          <Card className="p-6">
            <div className="flex gap-2">
              <Badge>{current.status}</Badge>
              <Badge>revision {current.version}</Badge>
            </div>
            <h2 className="mt-3 text-xl font-semibold">{current.title}</h2>
            <p className="mt-2 text-sm">{current.summary}</p>
            <div className="mt-5 grid gap-4 md:grid-cols-2">
              <section>
                <h3 className="font-semibold">Roles and eligibility</h3>
                {current.roles.map((r) => (
                  <div key={r.name} className="mt-2 text-sm">
                    <strong>{r.name}</strong> — {r.description}
                    <p className="text-xs text-[var(--muted)]">
                      {r.eligibility.join(" · ")}
                    </p>
                  </div>
                ))}
              </section>
              <section>
                <h3 className="font-semibold">Decision classes</h3>
                {current.decision_classes.map((d) => (
                  <div key={d.name} className="mt-2 text-sm">
                    <strong>{d.name}</strong> — {d.description}
                    <p className="text-xs text-[var(--muted)]">
                      {d.quorum} of {d.participation} · {d.approval} ·{" "}
                      {d.protected_resources.join(", ")}
                    </p>
                  </div>
                ))}
              </section>
            </div>
            <div className="mt-5 grid gap-3 text-sm md:grid-cols-2">
              <p>
                <strong>Terms:</strong> {current.procedures.terms}
              </p>
              <p>
                <strong>Removal:</strong> {current.procedures.removal}
              </p>
              <p>
                <strong>Succession:</strong> {current.procedures.succession}
              </p>
              <p>
                <strong>Amendments:</strong> {current.procedures.amendments}
              </p>
            </div>
          </Card>
          <Card className="p-6">
            <h2 className="font-semibold">Authority preview</h2>
            <p className="mt-1 text-sm text-[var(--muted)]">
              Eligibility is resolved separately for each decision class.
              Charter rules do not replace existing controls.
            </p>
            {Object.entries(data.preview.decision_eligibility).map(
              ([name, count]) => (
                <p key={name} className="mt-2 text-sm">
                  {name}: {count} eligible participant(s)
                </p>
              ),
            )}
            {data.preview.relationships.map((x) => (
              <p key={x} className="mt-2 text-sm">
                ✓ {x}
              </p>
            ))}
            {data.preview.blockers.map((x) => (
              <p key={x} className="mt-2 text-sm text-[var(--danger)]">
                Blocked: {x}
              </p>
            ))}
            {user && current.status === "draft" && (
              <div className="mt-4 flex gap-2">
                <Button
                  disabled={pending}
                  onClick={() =>
                    void act(`/revisions/${current.version}/approvals`, {
                      version: current.version,
                      decision: "approved",
                      reason: "Approved for activation",
                    })
                  }
                >
                  Record approval
                </Button>
                <Button
                  variant="secondary"
                  disabled={pending || !data.preview.valid}
                  onClick={() =>
                    void act(`/revisions/${current.version}/activate`, {
                      version: current.version,
                    })
                  }
                >
                  Activate
                </Button>
              </div>
            )}
          </Card>
          <Card className="p-6">
            <h2 className="font-semibold">Attributable history</h2>
            {data.charter.revisions.map((r) => (
              <p key={r.id} className="mt-2 text-sm">
                Revision {r.version} · {r.status} · {r.created_by} ·{" "}
                {new Date(r.created_at).toLocaleString()}
              </p>
            ))}
            {data.charter.approvals.map((a) => (
              <p key={a.id} className="mt-2 text-sm">
                {a.decision} revision {a.version} · {a.actor_id} · {a.reason}
              </p>
            ))}
            {data.charter.exceptions.map((x) => (
              <p key={x.id} className="mt-2 text-sm">
                Exception: {x.decision_class} / {x.resource} until{" "}
                {new Date(x.expires_at).toLocaleDateString()} · {x.reason}
              </p>
            ))}
          </Card>
        </>
      )}
      {current && data && (
        <Card className="p-6">
          <h2 className="font-semibold">Governance standing</h2>
          <p className="mt-1 text-sm text-[var(--muted)]">
            Time-bounded community voice is shown separately from operational
            authority.
          </p>
          {data.standing.length === 0 && (
            <p className="mt-3 text-sm">No participants admitted yet.</p>
          )}
          {data.standing.map((s) => (
            <section
              key={s.id}
              className="mt-4 rounded-lg border border-[var(--line)] p-4"
            >
              <div className="flex flex-wrap gap-2">
                <Badge>{s.effective_status}</Badge>
                <Badge>{s.role}</Badge>
              </div>
              <h3 className="mt-2 font-semibold">{s.principal_id}</h3>
              <p className="mt-1 text-sm">{s.responsibilities}</p>
              <p className="mt-2 text-xs text-[var(--muted)]">
                Eligible: {s.eligibility} · term ends{" "}
                {new Date(s.expires_at).toLocaleDateString()}
              </p>
              {s.evidence.map((e, i) => (
                <p key={`${e.kind}-${i}`} className="mt-2 text-sm">
                  Evidence · {e.kind} / {e.resource_id}: {e.summary}
                </p>
              ))}
              {s.conflict_of_interest && (
                <p className="mt-2 text-sm text-[var(--danger)]">
                  Conflict: {s.conflict_of_interest}
                </p>
              )}
              <p className="mt-2 text-sm">
                <strong>Operational access:</strong>{" "}
                {s.operational_access.join(", ") ||
                  "none from current project relationships"}
              </p>
              <p className="mt-1 text-xs text-[var(--muted)]">
                {s.authority_note}
              </p>
              <div className="mt-3 flex flex-wrap gap-2">
                {s.available_actions.map((a) => (
                  <Button
                    key={a}
                    variant="secondary"
                    disabled={pending}
                    onClick={() =>
                      void act(`/standing/${s.id}/actions`, {
                        action: a,
                        reason: `${a} recorded by participant`,
                        conflict_of_interest:
                          a === "recuse"
                            ? "Participant disclosed a conflict"
                            : "",
                      })
                    }
                  >
                    {a}
                  </Button>
                ))}
                {s.nomination_available && (
                  <Badge>Nominations available in a governed proposal</Badge>
                )}
              </div>
              <details className="mt-3 text-xs">
                <summary>Attributable events</summary>
                {s.events.map((e) => (
                  <p key={e.id} className="mt-1">
                    {e.kind} · {e.actor_id} · {e.reason}
                  </p>
                ))}
              </details>
            </section>
          ))}
        </Card>
      )}
      {current && data && (
        <Card className="p-6">
          <h2 className="font-semibold">Continuity and recovery</h2>
          <p className="mt-1 text-sm text-[var(--muted)]">
            Vacancies, recalls, handoffs, and emergency powers remain
            time-bounded; resource access is separately approved.
          </p>
          {data.continuity.length === 0 && (
            <p className="mt-3 text-sm">
              No unresolved handoffs or emergency powers.
            </p>
          )}
          {data.continuity.map((x) => (
            <section key={x.id} className="mt-4 rounded-lg border p-4">
              <div className="flex gap-2">
                <Badge tone={x.review_required ? "warning" : "neutral"}>
                  {x.effective_status}
                </Badge>
                <Badge>{x.kind}</Badge>
              </div>
              <p className="mt-2 text-sm">
                <strong>{x.role}</strong> · {x.reason}
              </p>
              <p className="mt-1 text-xs text-[var(--muted)]">
                Proposal {x.governance_proposal_id} · {x.resources.join(", ")} ·
                expires {new Date(x.expires_at).toLocaleString()}
              </p>
              <p className="mt-2 text-xs text-[var(--muted)]">
                {x.authority_note}
              </p>
              {x.status === "pending" && (
                <Button
                  className="mt-3"
                  variant="secondary"
                  disabled={pending}
                  onClick={() =>
                    void act(`/continuity/${x.id}/actions`, {
                      action: "approve",
                      reason:
                        "Governed result and independent resource approval verified",
                    })
                  }
                >
                  Approve handoff
                </Button>
              )}
            </section>
          ))}
        </Card>
      )}
      {token && current?.status === "active" && (
        <Card className="p-6">
          <h2 className="font-semibold">Record governed continuity</h2>
          <form
            onSubmit={continuity}
            className="mt-4 grid gap-3 md:grid-cols-2"
          >
            <select name="kind" className="rounded-lg border p-3">
              <option>nomination</option>
              <option>election</option>
              <option>recall</option>
              <option>succession</option>
              <option>emergency</option>
            </select>
            <select name="role" className="rounded-lg border p-3">
              {current.roles.map((r) => (
                <option key={r.name}>{r.name}</option>
              ))}
            </select>
            <input
              name="from"
              placeholder="Former standing ID"
              className="rounded-lg border p-3 text-sm"
            />
            <input
              name="to"
              placeholder="Successor standing ID"
              className="rounded-lg border p-3 text-sm"
            />
            <input
              name="proposal"
              required
              placeholder="Governance proposal ID"
              className="rounded-lg border p-3 text-sm"
            />
            <textarea
              name="reason"
              required
              placeholder="Vacancy, deadlock, appeal, or handoff rationale"
              className="rounded-lg border p-3 text-sm"
            />
            <textarea
              name="resources"
              required
              placeholder="Protected resources, one per line"
              className="rounded-lg border p-3 text-sm"
            />
            <input
              name="review_at"
              type="datetime-local"
              required
              className="rounded-lg border p-3"
            />
            <input
              name="expires_at"
              type="datetime-local"
              required
              className="rounded-lg border p-3"
            />
            <Button disabled={pending}>Record pending action</Button>
          </form>
        </Card>
      )}
      {token && current?.status === "active" && (
        <Card className="p-6">
          <h2 className="font-semibold">Invite or approve standing</h2>
          <form onSubmit={invite} className="mt-4 grid gap-3 md:grid-cols-2">
            <input
              name="principal_id"
              required
              placeholder="Human user ID"
              className="rounded-lg border p-3 text-sm"
            />
            <select name="role" className="rounded-lg border p-3 text-sm">
              {current.roles.map((r) => (
                <option key={r.name}>{r.name}</option>
              ))}
            </select>
            <textarea
              name="responsibilities"
              required
              placeholder="Responsibilities"
              className="rounded-lg border p-3 text-sm"
            />
            <select
              name="evidence_kind"
              className="rounded-lg border p-3 text-sm"
            >
              <option>contribution</option>
              <option>review</option>
              <option>support</option>
              <option>ownership</option>
              <option>membership</option>
            </select>
            <input
              name="resource_id"
              required
              placeholder="Evidence resource ID"
              className="rounded-lg border p-3 text-sm"
            />
            <textarea
              name="evidence_summary"
              required
              placeholder="Why this evidence establishes eligibility"
              className="rounded-lg border p-3 text-sm"
            />
            <label className="text-sm">
              Term ends
              <input
                name="expires_at"
                type="datetime-local"
                required
                className="mt-1 block w-full rounded-lg border p-2"
              />
            </label>
            <Button disabled={pending}>Invite participant</Button>
          </form>
        </Card>
      )}
      {token && (
        <Card className="p-6">
          <h2 className="font-semibold">
            {missing ? "Publish the first charter" : "Draft an amendment"}
          </h2>
          <form onSubmit={publish} className="mt-4 grid gap-3 md:grid-cols-2">
            {[
              ["title", "Charter title"],
              ["summary", "Purpose and scope"],
              ["role", "Governance role"],
              ["role_description", "Role responsibilities"],
              ["decision", "Decision class"],
              ["decision_description", "What this class decides"],
              ["resources", "Protected resources, e.g. branch:main"],
              ["terms", "Terms and renewal"],
              ["removal", "Removal procedure"],
              ["succession", "Succession procedure"],
              ["amendments", "Amendment policy"],
            ].map(([n, p]) => (
              <textarea
                key={n}
                name={n}
                required
                placeholder={p}
                className="min-h-20 rounded-lg border border-[var(--line-strong)] p-3 text-sm"
              />
            ))}
            <label className="text-sm">
              Eligibility source
              <select
                name="eligibility"
                className="mt-1 block w-full rounded-lg border p-2"
              >
                {kind === "repository" ? (
                  <>
                    <option value="repository_owner">Repository owner</option>
                    <option value="repository_collaborator">
                      Repository collaborators
                    </option>
                  </>
                ) : (
                  <>
                    <option value="organization_owner">
                      Organization owner
                    </option>
                    <option value="organization_member">
                      Organization members
                    </option>
                    <option value="team_maintainer">Team maintainers</option>
                    <option value="approved_agent">Approved agents</option>
                  </>
                )}
              </select>
            </label>
            <label className="text-sm">
              Participation
              <input
                name="participation"
                type="number"
                min="1"
                defaultValue="1"
                required
                className="mt-1 block w-full rounded-lg border p-2"
              />
            </label>
            <label className="text-sm">
              Quorum
              <input
                name="quorum"
                type="number"
                min="1"
                defaultValue="1"
                required
                className="mt-1 block w-full rounded-lg border p-2"
              />
            </label>
            <select name="approval" className="rounded-lg border p-2">
              <option>majority</option>
              <option>consensus</option>
              <option>supermajority</option>
            </select>
            <Button type="submit" disabled={pending}>
              {pending ? "Publishing…" : "Publish draft revision"}
            </Button>
          </form>
        </Card>
      )}
    </div>
  );
}
