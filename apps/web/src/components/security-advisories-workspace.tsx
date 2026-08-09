"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  api,
  type Repository,
  type SecurityAdvisory,
  type User,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

const field =
  "mt-2 min-h-11 w-full rounded-lg border border-[var(--line-strong)] bg-white px-3 outline-none focus:border-[var(--brand)]";
const errorMessage = (reason: unknown) =>
  reason instanceof Error
    ? reason.message
    : "The private security operation failed.";
const stamp = (value: string) =>
  new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
type VerificationView = {
  state: "pending" | "failed" | "awaiting_approval" | "integration_ready";
  runs: {
    id: string;
    name: string;
    kind: "required_check" | "security_reproduction";
    state: string;
    commit_id: string;
    artifacts: { id: string; path: string; sha256: string }[];
  }[];
};
async function pages<T>(path: string, key: string, token: string) {
  const items: T[] = [];
  let after: string | null = null;
  do {
    const page = await api<Record<string, T[] | string | null>>(
      `${path}?limit=100${after ? `&after=${encodeURIComponent(after)}` : ""}`,
      {},
      token,
    );
    items.push(...((page[key] as T[]) ?? []));
    after = page.next_cursor as string | null;
  } while (after);
  return items;
}

export function SecurityAdvisoriesWorkspace({
  advisoryId,
}: {
  advisoryId?: string;
}) {
  const { token, user, loading } = useAuth();
  const [items, setItems] = useState<SecurityAdvisory[]>([]);
  const [advisory, setAdvisory] = useState<SecurityAdvisory>();
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [people, setPeople] = useState<Record<string, User>>({});
  const [showReport, setShowReport] = useState(false);
  const [investigationAccess, setInvestigationAccess] = useState<{
    token: string;
    expires_at: string;
  }>();
  const [verificationViews, setVerificationViews] = useState<
    Record<string, VerificationView>
  >({});
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    if (!token) return;
    try {
      const repos = await pages<Repository>(
        "/repositories",
        "repositories",
        token,
      );
      setRepositories(repos);
      if (advisoryId) {
        const found = await api<SecurityAdvisory>(
          `/security-advisories/${advisoryId}`,
          {},
          token,
        );
        setAdvisory(found);
        const views = await Promise.all(
          found.repair_verifications.map(
            async (verification) =>
              [
                verification.id,
                await api<VerificationView>(
                  `/security-advisories/${found.id}/verifications/${verification.id}`,
                  {},
                  token,
                ),
              ] as const,
          ),
        );
        setVerificationViews(Object.fromEntries(views));
        const ids = [
          ...new Set([
            found.reporter_id,
            ...found.response_team,
            ...found.messages.map((x) => x.actor_id),
            ...found.access_log.map((x) => x.actor_id),
          ]),
        ];
        const users = await Promise.all(
          ids.map(
            async (id) =>
              [
                id,
                await api<User>(`/users/${id}`, {}, token).catch(
                  () => undefined,
                ),
              ] as const,
          ),
        );
        setPeople(
          Object.fromEntries(
            users.filter((x): x is readonly [string, User] => Boolean(x[1])),
          ),
        );
      } else {
        setItems(
          await pages<SecurityAdvisory>(
            "/security-advisories",
            "security_advisories",
            token,
          ),
        );
      }
    } catch (reason) {
      setError(errorMessage(reason));
    }
  }, [advisoryId, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  const mutate = async (path: string, body: object, method = "POST") => {
    if (!token) return false;
    setPending(true);
    setError("");
    try {
      setAdvisory(
        await api<SecurityAdvisory>(
          path,
          { method, body: JSON.stringify(body) },
          token,
        ),
      );
      return true;
    } catch (reason) {
      setError(errorMessage(reason));
      return false;
    } finally {
      setPending(false);
    }
  };
  const protectedAction = async (path: string, body: object = {}) => {
    if (!token) return false;
    setPending(true);
    setError("");
    try {
      const result = await api<{ security_advisory: SecurityAdvisory }>(
        path,
        { method: "POST", body: JSON.stringify(body) },
        token,
      );
      setAdvisory(result.security_advisory);
      await load();
      return true;
    } catch (reason) {
      setError(errorMessage(reason));
      return false;
    } finally {
      setPending(false);
    }
  };
  const delegate = async (form: HTMLFormElement) => {
    if (!token || !advisory) return;
    const data = new FormData(form);
    setPending(true);
    setError("");
    try {
      const launched = await api<{
        security_advisory: SecurityAdvisory;
        credential: { token: string; expires_at: string };
      }>(
        `/security-advisories/${advisory.id}/investigations`,
        {
          method: "POST",
          body: JSON.stringify({
            mandate: data.get("mandate"),
            evidence_ids: data.getAll("evidence_id"),
            expires_in: 3600,
          }),
        },
        token,
      );
      setAdvisory(launched.security_advisory);
      setInvestigationAccess(launched.credential);
      form.reset();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setPending(false);
    }
  };
  async function report(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    const data = new FormData(event.currentTarget);
    const repositoryId = String(data.get("repository_id"));
    setPending(true);
    setError("");
    try {
      const created = await api<SecurityAdvisory>(
        "/security-advisories",
        {
          method: "POST",
          body: JSON.stringify({
            title: data.get("title"),
            description: data.get("description"),
            contact: data.get("contact"),
            affected_repositories: [
              {
                repository_id: repositoryId,
                versions: String(data.get("versions"))
                  .split(",")
                  .map((x) => x.trim())
                  .filter(Boolean),
              },
            ],
            evidence: [
              {
                label: data.get("evidence_label"),
                description: data.get("evidence_description"),
              },
            ],
          }),
        },
        token,
      );
      window.location.assign(`/security/${created.id}`);
    } catch (reason) {
      setError(errorMessage(reason));
      setPending(false);
    }
  }
  if (loading)
    return (
      <Card className="p-8 text-sm text-[var(--muted)]">
        Opening protected workspace…
      </Card>
    );
  if (!user)
    return (
      <Card className="p-8 text-center">
        <h1 className="text-2xl font-semibold">
          Report a vulnerability privately
        </h1>
        <p className="mt-2 text-sm text-[var(--muted)]">
          Sign in so maintainers can respond through a protected, attributable
          channel.
        </p>
        <Link
          href="/?access=signin"
          className="mt-5 inline-flex rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-semibold text-white"
        >
          Sign in
        </Link>
      </Card>
    );
  const repositoryName = (id: string) =>
    repositories.find((x) => x.id === id)?.name ?? id;
  const actorName = (id: string) =>
    people[id]?.display_name || people[id]?.handle || id;
  const isMaintainer = advisory?.affected_repositories.some(
    (x) =>
      repositories.find((repo) => repo.id === x.repository_id)?.owner_id ===
      user.id,
  );
  if (advisoryId && advisory)
    return (
      <div className="space-y-6">
        <header>
          <Link
            href="/security"
            className="text-sm font-semibold text-[var(--brand-strong)]"
          >
            ← Private security reports
          </Link>
          <div className="mt-4 flex flex-wrap items-center gap-2">
            <Badge
              tone={
                advisory.severity === "critical" || advisory.severity === "high"
                  ? "danger"
                  : "warning"
              }
            >
              {advisory.severity}
            </Badge>
            <Badge>{advisory.embargo_state}</Badge>
          </div>
          <h1 className="mt-3 text-3xl font-semibold">{advisory.title}</h1>
          <p className="mt-2 max-w-3xl whitespace-pre-wrap text-sm text-[var(--muted)]">
            {advisory.description}
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
        <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_22rem]">
          <div className="space-y-5">
            <Card className="p-5">
              <h2 className="font-semibold">Affected versions</h2>
              {advisory.affected_repositories.map((x) => (
                <div
                  key={x.repository_id}
                  className="mt-3 rounded-lg bg-[var(--canvas)] p-3 text-sm"
                >
                  <span className="font-semibold">
                    {repositoryName(x.repository_id)}
                  </span>
                  <span className="ml-2 text-[var(--muted)]">
                    {x.versions.join(", ")}
                  </span>
                </div>
              ))}
              <h3 className="mt-5 font-semibold">Protected evidence graph</h3>
              {advisory.evidence.map((x, index) => (
                <div
                  key={x.id ?? `${x.label}-${index}`}
                  className="mt-3 rounded-lg bg-[var(--canvas)] p-3"
                >
                  <div className="flex gap-2">
                    <p className="text-sm font-semibold">{x.label}</p>
                    {x.kind && <Badge>{x.kind}</Badge>}
                  </div>
                  <p className="mt-1 whitespace-pre-wrap text-sm text-[var(--muted)]">
                    {x.description}
                  </p>
                </div>
              ))}
              <form
                className="mt-5 grid gap-3 sm:grid-cols-2"
                onSubmit={(event) => {
                  event.preventDefault();
                  const form = event.currentTarget;
                  const data = new FormData(form);
                  void mutate(
                    `/security-advisories/${advisory.id}/evidence`,
                    Object.fromEntries(data),
                  ).then((saved) => {
                    if (saved) form.reset();
                  });
                }}
              >
                <select className={field} name="repository_id">
                  {advisory.affected_repositories.map((x) => (
                    <option key={x.repository_id} value={x.repository_id}>
                      {repositoryName(x.repository_id)}
                    </option>
                  ))}
                </select>
                <select className={field} name="kind">
                  <option value="commit">Commit</option>
                  <option value="dependency">Dependency</option>
                  <option value="release">Release</option>
                  <option value="build">Build</option>
                  <option value="artifact">Release artifact</option>
                  <option value="deployment">Deployment</option>
                </select>
                <input
                  className={field}
                  name="label"
                  required
                  placeholder="Evidence label"
                />
                <input
                  className={field}
                  name="commit_id"
                  placeholder="Commit SHA"
                />
                <input
                  className={field}
                  name="dependency"
                  placeholder="Dependency name/version"
                />
                <input
                  className={field}
                  name="release_id"
                  placeholder="Release ID"
                />
                <input
                  className={field}
                  name="build_id"
                  placeholder="Build ID"
                />
                <input
                  className={field}
                  name="artifact_id"
                  placeholder="Artifact ID"
                />
                <input
                  className={field}
                  name="deployment_id"
                  placeholder="Deployment ID"
                />
                <input
                  className={field}
                  name="description"
                  placeholder="Why this evidence matters"
                />
                <Button className="sm:col-span-2" disabled={pending}>
                  Connect verified evidence
                </Button>
              </form>
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Fixed-release proof</h2>
              <p className="mt-2 text-sm text-[var(--muted)]">
                Each supported line needs an exact-candidate reproduction,
                required checks, independent approval, and a checksummed release
                artifact. Commands and logs stay in this protected workspace.
              </p>
              <div className="mt-4 space-y-3">
                {advisory.affected_repositories.flatMap((scope) =>
                  scope.versions.map((line) => {
                    const tasks = advisory.repair_tasks.filter(
                      (x) =>
                        x.repository_id === scope.repository_id &&
                        x.version_line === line,
                    );
                    const verifications = advisory.repair_verifications.filter(
                      (x) =>
                        x.repository_id === scope.repository_id &&
                        x.version_line === line,
                    );
                    const approved = verifications.some(
                      (x) => x.approvals.length > 0,
                    );
                    const attested = advisory.release_attestations.some(
                      (x) =>
                        x.repository_id === scope.repository_id &&
                        x.version_line === line,
                    );
                    const reproduction = advisory.security_reproductions.some(
                      (x) =>
                        x.repository_id === scope.repository_id &&
                        x.version_line === line,
                    );
                    return (
                      <div
                        key={`${scope.repository_id}-${line}`}
                        className="rounded-lg bg-[var(--canvas)] p-3"
                      >
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="mr-auto text-sm font-semibold">
                            {repositoryName(scope.repository_id)} · {line}
                          </p>
                          <Badge tone={attested ? "success" : "warning"}>
                            {attested ? "attested" : "gap remains"}
                          </Badge>
                        </div>
                        <p className="mt-2 text-xs text-[var(--muted)]">
                          {tasks.length
                            ? `${tasks.length} repair task(s)`
                            : "No repair task"}{" "}
                          ·{" "}
                          {reproduction
                            ? "private reproduction defined"
                            : "reproduction missing"}{" "}
                          ·{" "}
                          {approved
                            ? "approved exact proof"
                            : "approval pending"}{" "}
                          ·{" "}
                          {attested
                            ? "release artifact covered"
                            : "artifact missing"}
                        </p>
                      </div>
                    );
                  }),
                )}
              </div>
              {isMaintainer && (
                <form
                  className="mt-5 grid gap-3 sm:grid-cols-2"
                  onSubmit={(event) => {
                    event.preventDefault();
                    const form = event.currentTarget;
                    const data = new FormData(form);
                    void protectedAction(
                      `/security-advisories/${advisory.id}/reproductions`,
                      {
                        repository_id: data.get("repository_id"),
                        version_line: data.get("version_line"),
                        definition: {
                          name: data.get("name"),
                          image: data.get("image"),
                          command: data.get("command"),
                          timeout_seconds: 600,
                        },
                      },
                    ).then((saved) => {
                      if (saved) form.reset();
                    });
                  }}
                >
                  <select className={field} name="repository_id" required>
                    {advisory.affected_repositories.map((x) => (
                      <option key={x.repository_id} value={x.repository_id}>
                        {repositoryName(x.repository_id)}
                      </option>
                    ))}
                  </select>
                  <input
                    className={field}
                    name="version_line"
                    required
                    placeholder="Exact supported line"
                  />
                  <input
                    className={field}
                    name="name"
                    required
                    placeholder="Private reproduction name"
                  />
                  <input
                    className={field}
                    name="image"
                    required
                    placeholder="Pinned runner image"
                  />
                  <textarea
                    className={`${field} py-3 sm:col-span-2`}
                    name="command"
                    required
                    rows={3}
                    maxLength={4000}
                    placeholder="Embargoed reproduction command"
                  />
                  <Button className="sm:col-span-2" disabled={pending}>
                    Define private reproduction
                  </Button>
                </form>
              )}
              {advisory.repair_sessions
                .filter((x) => x.state === "completed")
                .map((session) => {
                  const verification = advisory.repair_verifications.find(
                    (x) => x.session_id === session.id,
                  );
                  const view = verification
                    ? verificationViews[verification.id]
                    : undefined;
                  return (
                    <div
                      key={session.id}
                      className="mt-4 rounded-lg border border-[var(--line)] p-3 text-sm"
                    >
                      <div className="flex items-center gap-2">
                        <p className="mr-auto font-semibold">
                          Candidate {session.commit_id?.slice(0, 12)}
                        </p>
                        {view && (
                          <Badge
                            tone={
                              view.state === "integration_ready"
                                ? "success"
                                : view.state === "failed"
                                  ? "danger"
                                  : "warning"
                            }
                          >
                            {view.state.replaceAll("_", " ")}
                          </Badge>
                        )}
                      </div>
                      {!verification ? (
                        <Button
                          className="mt-3"
                          disabled={pending}
                          onClick={() =>
                            void protectedAction(
                              `/security-advisories/${advisory.id}/repair-sessions/${session.id}/verifications`,
                            )
                          }
                        >
                          Run exact proof
                        </Button>
                      ) : (
                        <>
                          <p className="mt-2 text-xs text-[var(--muted)]">
                            {verification.required_run_ids.length} required
                            check(s) ·{" "}
                            {verification.reproduction_run_ids.length} private
                            reproduction(s) · {verification.approvals.length}{" "}
                            approval(s)
                          </p>
                          {view?.runs.map((run) => (
                            <div
                              key={run.id}
                              className="mt-2 flex flex-wrap items-center gap-2 rounded bg-white p-2 text-xs"
                            >
                              <Badge
                                tone={
                                  run.state === "succeeded"
                                    ? "success"
                                    : run.state === "failed"
                                      ? "danger"
                                      : "warning"
                                }
                              >
                                {run.state}
                              </Badge>
                              <span className="font-semibold">{run.name}</span>
                              <span className="text-[var(--muted)]">
                                {run.kind.replaceAll("_", " ")}
                              </span>
                              {run.artifacts.map((artifact) => (
                                <span
                                  key={artifact.id}
                                  className="break-all font-mono text-[var(--muted)]"
                                >
                                  sha256:{artifact.sha256}
                                </span>
                              ))}
                            </div>
                          ))}
                          {isMaintainer &&
                            verification.approvals.length === 0 && (
                              <Button
                                className="mt-3"
                                disabled={
                                  pending || view?.state !== "awaiting_approval"
                                }
                                onClick={() =>
                                  void protectedAction(
                                    `/security-advisories/${advisory.id}/verifications/${verification.id}/approvals`,
                                  )
                                }
                              >
                                Approve passing proof
                              </Button>
                            )}
                          <form
                            className="mt-3 flex gap-2"
                            onSubmit={(event) => {
                              event.preventDefault();
                              const form = event.currentTarget;
                              void protectedAction(
                                `/security-advisories/${advisory.id}/verifications/${verification.id}/release-attestations`,
                                {
                                  release_id: new FormData(form).get(
                                    "release_id",
                                  ),
                                },
                              ).then((saved) => {
                                if (saved) form.reset();
                              });
                            }}
                          >
                            <input
                              className={field}
                              name="release_id"
                              required
                              placeholder="Successful release ID"
                            />
                            <Button
                              disabled={
                                pending || view?.state !== "integration_ready"
                              }
                            >
                              Attest artifact
                            </Button>
                          </form>
                        </>
                      )}
                    </div>
                  );
                })}
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Version × environment impact</h2>
              <div className="mt-3 overflow-x-auto">
                <table className="w-full text-left text-sm">
                  <thead>
                    <tr className="text-[var(--muted)]">
                      <th className="pb-2">Version</th>
                      <th>Environment</th>
                      <th>Impact</th>
                    </tr>
                  </thead>
                  <tbody>
                    {advisory.impact_matrix.map((x) => (
                      <tr
                        key={`${x.repository_id}-${x.version_line}-${x.environment}`}
                      >
                        <td className="py-2">{x.version_line}</td>
                        <td>{x.environment}</td>
                        <td>
                          <Badge
                            tone={
                              x.state === "confirmed"
                                ? "danger"
                                : x.state === "fixed" ||
                                    x.state === "unaffected"
                                  ? "success"
                                  : "warning"
                            }
                          >
                            {x.state}
                          </Badge>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <form
                className="mt-4 grid gap-3 sm:grid-cols-3"
                onSubmit={(event) => {
                  event.preventDefault();
                  const form = event.currentTarget;
                  const data = new FormData(form);
                  void mutate(
                    `/security-advisories/${advisory.id}/impact`,
                    {
                      expected_version: advisory.version,
                      repository_id: data.get("repository_id"),
                      version_line: data.get("version_line"),
                      environment: data.get("environment"),
                      state: data.get("state"),
                      rationale: data.get("rationale"),
                      evidence_ids: [],
                    },
                    "PUT",
                  ).then((saved) => {
                    if (saved) form.reset();
                  });
                }}
              >
                <select className={field} name="repository_id" required>
                  {advisory.affected_repositories.map((x) => (
                    <option key={x.repository_id} value={x.repository_id}>
                      {repositoryName(x.repository_id)}
                    </option>
                  ))}
                </select>
                <input
                  className={field}
                  name="version_line"
                  required
                  placeholder="Version line"
                />
                <input
                  className={field}
                  name="environment"
                  required
                  placeholder="Environment"
                />
                <select className={field} name="state">
                  <option value="suspected">Suspected</option>
                  <option value="confirmed">Confirmed</option>
                  <option value="unaffected">Unaffected</option>
                  <option value="fixed">Fixed</option>
                </select>
                <input
                  className={field}
                  name="rationale"
                  placeholder="Rationale"
                />
                <Button className="mt-2" disabled={pending}>
                  Record impact
                </Button>
              </form>
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Hypotheses and conclusions</h2>
              <div className="mt-3 space-y-3">
                {advisory.findings.map((x) => (
                  <div key={x.id} className="rounded-lg bg-[var(--canvas)] p-3">
                    <Badge>{x.kind}</Badge>
                    <p className="mt-2 text-sm">{x.statement}</p>
                    <p className="mt-2 text-xs text-[var(--muted)]">
                      {actorName(x.actor_id)} · {stamp(x.created_at)}
                    </p>
                  </div>
                ))}
              </div>
              <form
                className="mt-4"
                onSubmit={(event) => {
                  event.preventDefault();
                  const form = event.currentTarget;
                  const data = new FormData(form);
                  void mutate(`/security-advisories/${advisory.id}/findings`, {
                    kind: data.get("kind"),
                    statement: data.get("statement"),
                    evidence_ids: [],
                  }).then((saved) => {
                    if (saved) form.reset();
                  });
                }}
              >
                <select className={field} name="kind">
                  <option value="hypothesis">Hypothesis</option>
                  <option value="conclusion">Conclusion</option>
                  <option value="uncertainty">Uncertainty</option>
                </select>
                <textarea
                  className={`${field} py-3`}
                  name="statement"
                  required
                  rows={3}
                  maxLength={10000}
                />
                <Button className="mt-3" disabled={pending}>
                  Record finding
                </Button>
              </form>
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Delegate selected evidence</h2>
              <p className="mt-2 text-sm text-[var(--muted)]">
                A short-lived read-only agent sees only the frozen evidence you
                select. Its findings and uncertainty return only to this
                protected workspace.
              </p>
              {investigationAccess && (
                <div
                  role="status"
                  className="mt-4 rounded-lg border border-[var(--warning)] bg-[var(--warning-soft)] p-3"
                >
                  <p className="text-sm font-semibold">
                    Copy this agent credential now
                  </p>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    It is shown only in this browser session and expires{" "}
                    {stamp(investigationAccess.expires_at)}.
                  </p>
                  <input
                    className={`${field} font-mono text-xs`}
                    readOnly
                    aria-label="Investigation agent credential"
                    value={investigationAccess.token}
                  />
                  <div className="mt-2 flex gap-2">
                    <Button
                      type="button"
                      onClick={() =>
                        void navigator.clipboard.writeText(
                          investigationAccess.token,
                        )
                      }
                    >
                      Copy credential
                    </Button>
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={() => setInvestigationAccess(undefined)}
                    >
                      Clear
                    </Button>
                  </div>
                </div>
              )}
              <form
                className="mt-4"
                onSubmit={(event) => {
                  event.preventDefault();
                  void delegate(event.currentTarget);
                }}
              >
                <div className="space-y-2">
                  {advisory.evidence
                    .filter((x) => x.id)
                    .map((x) => (
                      <label key={x.id} className="flex gap-2 text-sm">
                        <input
                          type="checkbox"
                          name="evidence_id"
                          value={x.id}
                        />
                        {x.label}
                      </label>
                    ))}
                </div>
                <textarea
                  className={`${field} py-3`}
                  name="mandate"
                  required
                  rows={3}
                  maxLength={10000}
                  placeholder="Investigation mandate"
                />
                <Button
                  className="mt-3"
                  disabled={pending || !advisory.evidence.some((x) => x.id)}
                >
                  Delegate read-only investigation
                </Button>
              </form>
              {advisory.investigations.map((x) => (
                <div
                  key={x.id}
                  className="mt-3 rounded-lg bg-[var(--canvas)] p-3 text-sm"
                >
                  <Badge>{x.state}</Badge>
                  <p className="mt-2">{x.mandate}</p>
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    {x.evidence.length} frozen evidence item(s)
                  </p>
                </div>
              ))}
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Protected conversation</h2>
              <div className="mt-4 space-y-3">
                {advisory.messages.length === 0 && (
                  <p className="text-sm text-[var(--muted)]">
                    No messages yet.
                  </p>
                )}
                {advisory.messages.map((message) => (
                  <div
                    key={message.id}
                    className="rounded-lg bg-[var(--canvas)] p-3"
                  >
                    <p className="text-xs font-semibold">
                      {actorName(message.actor_id)} ·{" "}
                      {stamp(message.created_at)}
                    </p>
                    <p className="mt-2 whitespace-pre-wrap text-sm">
                      {message.body}
                    </p>
                  </div>
                ))}
              </div>
              <form
                className="mt-4"
                onSubmit={(event) => {
                  event.preventDefault();
                  const form = event.currentTarget;
                  const body = new FormData(form).get("body");
                  void mutate(`/security-advisories/${advisory.id}/messages`, {
                    body,
                  }).then((saved) => {
                    if (saved) form.reset();
                  });
                }}
              >
                <textarea
                  className={`${field} py-3`}
                  name="body"
                  rows={3}
                  required
                  maxLength={20000}
                  aria-label="Message"
                />
                <Button className="mt-3" disabled={pending}>
                  Send privately
                </Button>
              </form>
            </Card>
          </div>
          <aside className="space-y-5">
            <Card className="p-5">
              <h2 className="font-semibold">Safe contact</h2>
              <p className="mt-2 break-words text-sm text-[var(--muted)]">
                {advisory.contact}
              </p>
              <p className="mt-4 text-xs text-[var(--muted)]">
                Reporter: {actorName(advisory.reporter_id)}
              </p>
            </Card>
            {isMaintainer && (
              <>
                <Card className="p-5">
                  <h2 className="font-semibold">Coordinated disclosure</h2>
                  <p className="mt-2 text-sm text-[var(--muted)]">
                    Public knowledge remains unavailable until every supported
                    line has an attested fix, repaired branches are published,
                    and actionable notifications are durable.
                  </p>
                  {advisory.disclosure ? (
                    <div className="mt-4 rounded-lg bg-[var(--canvas)] p-3 text-sm">
                      <div className="flex items-center gap-2">
                        <Badge
                          tone={
                            advisory.disclosure.state === "published"
                              ? "success"
                              : advisory.disclosure.state === "paused"
                                ? "danger"
                                : "warning"
                          }
                        >
                          {advisory.disclosure.state}
                        </Badge>
                        <span>
                          {advisory.disclosure.remaining.length} step(s) remain
                        </span>
                      </div>
                      {advisory.disclosure.failure && (
                        <p className="mt-2 text-[var(--danger)]">
                          {advisory.disclosure.failure}
                        </p>
                      )}
                      <p className="mt-2 whitespace-pre-wrap">
                        {advisory.disclosure.upgrade_guidance}
                      </p>
                      {advisory.disclosure.state !== "published" && (
                        <Button
                          className="mt-3"
                          disabled={pending}
                          onClick={() =>
                            void mutate(
                              `/security-advisories/${advisory.id}/disclosure/publish`,
                              {},
                              "POST",
                            )
                          }
                        >
                          Publish now
                        </Button>
                      )}
                    </div>
                  ) : (
                    <form
                      className="mt-4 space-y-3"
                      onSubmit={(event) => {
                        event.preventDefault();
                        const data = new FormData(event.currentTarget);
                        void mutate(
                          `/security-advisories/${advisory.id}/disclosure`,
                          {
                            expected_version: advisory.version,
                            public_title: data.get("public_title"),
                            redacted_summary: data.get("redacted_summary"),
                            upgrade_guidance: data.get("upgrade_guidance"),
                            credits: String(data.get("credits"))
                              .split(",")
                              .map((x) => x.trim())
                              .filter(Boolean),
                            scheduled_at: data.get("scheduled_at")
                              ? new Date(String(data.get("scheduled_at"))).toISOString()
                              : null,
                          },
                          "POST",
                        );
                      }}
                    >
                      <input
                        className={field}
                        name="public_title"
                        required
                        maxLength={200}
                        placeholder="Public advisory title"
                      />
                      <textarea
                        className={`${field} py-3`}
                        name="redacted_summary"
                        required
                        rows={4}
                        maxLength={20000}
                        placeholder="Public, exploit-safe advisory summary"
                      />
                      <textarea
                        className={`${field} py-3`}
                        name="upgrade_guidance"
                        required
                        rows={4}
                        maxLength={20000}
                        placeholder="Exact versions and exposure-reduction steps"
                      />
                      <input
                        className={field}
                        name="credits"
                        placeholder="Public credits, comma separated"
                      />
                      <label className="block text-xs text-[var(--muted)]">
                        Optional publication time
                        <input
                          className={field}
                          name="scheduled_at"
                          type="datetime-local"
                        />
                      </label>
                      <Button disabled={pending}>Prepare disclosure</Button>
                    </form>
                  )}
                </Card>
                <Card className="p-5">
                  <h2 className="font-semibold">Maintainer triage</h2>
                  <form
                    className="mt-3 space-y-3"
                    onSubmit={(event) => {
                      event.preventDefault();
                      const data = new FormData(event.currentTarget);
                      void mutate(
                        `/security-advisories/${advisory.id}`,
                        {
                          expected_version: advisory.version,
                          severity: data.get("severity"),
                          embargo_state: data.get("embargo_state"),
                        },
                        "PATCH",
                      );
                    }}
                  >
                    <select
                      className={field}
                      name="severity"
                      defaultValue={
                        advisory.severity === "untriaged"
                          ? "high"
                          : advisory.severity
                      }
                    >
                      <option value="low">Low</option>
                      <option value="moderate">Moderate</option>
                      <option value="high">High</option>
                      <option value="critical">Critical</option>
                    </select>
                    <select
                      className={field}
                      name="embargo_state"
                      defaultValue={advisory.embargo_state}
                    >
                      <option value="reported">Reported</option>
                      <option value="triaging">Triaging</option>
                      <option value="embargoed">Embargoed</option>
                      <option value="coordinating">Coordinating</option>
                    </select>
                    <Button disabled={pending}>Update triage</Button>
                  </form>
                  <form
                    className="mt-6"
                    onSubmit={(event) => {
                      event.preventDefault();
                      const form = event.currentTarget;
                      const userId = new FormData(form).get("user_id");
                      void mutate(
                        `/security-advisories/${advisory.id}/responders`,
                        { user_id: userId },
                      ).then((saved) => {
                        if (saved) form.reset();
                      });
                    }}
                  >
                    <label className="text-sm font-semibold">
                      Invite responder by collaboration ID
                      <input
                        className={field}
                        name="user_id"
                        required
                        minLength={32}
                        maxLength={32}
                      />
                    </label>
                    <Button className="mt-3" disabled={pending}>
                      Invite to response team
                    </Button>
                  </form>
                </Card>
              </>
            )}
            <Card className="p-5">
              <h2 className="font-semibold">Access audit</h2>
              <div className="mt-3 max-h-80 space-y-2 overflow-auto">
                {[...advisory.access_log].reverse().map((event) => (
                  <p key={event.id} className="text-xs text-[var(--muted)]">
                    <span className="font-semibold text-[var(--ink)]">
                      {actorName(event.actor_id)}
                    </span>{" "}
                    {event.action.replaceAll("_", " ")} ·{" "}
                    {stamp(event.created_at)}
                  </p>
                ))}
              </div>
            </Card>
          </aside>
        </div>
      </div>
    );
  return (
    <div className="space-y-7">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="font-mono text-xs font-semibold uppercase tracking-[.16em] text-[var(--danger)]">
            Protected collaboration
          </p>
          <h1 className="mt-2 text-3xl font-semibold">
            Private security reports
          </h1>
          <p className="mt-2 max-w-2xl text-sm text-[var(--muted)]">
            Share suspected vulnerabilities and coordinate an embargoed response
            without publishing repository activity or notifications.
          </p>
        </div>
        <Button onClick={() => setShowReport((x) => !x)}>
          {showReport ? "Cancel" : "Report vulnerability"}
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
      {showReport && (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Protected report</h2>
          <form onSubmit={report} className="mt-5 grid gap-4">
            <label className="text-sm font-semibold">
              Title
              <input className={field} name="title" required maxLength={200} />
            </label>
            <label className="text-sm font-semibold">
              Suspected vulnerability
              <textarea
                className={`${field} py-3`}
                name="description"
                rows={5}
                required
                maxLength={20000}
              />
            </label>
            <label className="text-sm font-semibold">
              Affected repository
              <select className={field} name="repository_id" required>
                <option value="">Select a repository</option>
                {repositories.map((repo) => (
                  <option key={repo.id} value={repo.id}>
                    {repo.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-sm font-semibold">
              Affected versions
              <input
                className={field}
                name="versions"
                required
                placeholder="1.4.x, 2.0.0"
              />
            </label>
            <label className="text-sm font-semibold">
              Evidence label
              <input
                className={field}
                name="evidence_label"
                required
                placeholder="Reproduction notes"
              />
            </label>
            <label className="text-sm font-semibold">
              Evidence (avoid live secrets)
              <textarea
                className={`${field} py-3`}
                name="evidence_description"
                rows={4}
                required
                maxLength={10000}
              />
            </label>
            <label className="text-sm font-semibold">
              Safe contact channel
              <input
                className={field}
                name="contact"
                required
                maxLength={500}
                placeholder="Encrypted email, Signal handle, or monitored address"
              />
            </label>
            <p className="text-xs text-[var(--muted)]">
              Only you, affected repository owners, and responders they
              explicitly invite can discover this report.
            </p>
            <Button disabled={pending}>Submit protected report</Button>
          </form>
        </Card>
      )}
      <div className="grid gap-3">
        {items.length === 0 && (
          <Card className="p-8 text-center text-sm text-[var(--muted)]">
            No private reports are available to you.
          </Card>
        )}
        {items.map((item) => (
          <Link key={item.id} href={`/security/${item.id}`}>
            <Card className="p-5 transition hover:border-[var(--line-strong)]">
              <div className="flex flex-wrap items-center gap-2">
                <Badge
                  tone={
                    item.severity === "critical" || item.severity === "high"
                      ? "danger"
                      : "warning"
                  }
                >
                  {item.severity}
                </Badge>
                <Badge>{item.embargo_state}</Badge>
              </div>
              <h2 className="mt-3 font-semibold">{item.title}</h2>
              <p className="mt-1 text-sm text-[var(--muted)]">
                {item.affected_repositories
                  .map((x) => repositoryName(x.repository_id))
                  .join(", ")}{" "}
                · updated {stamp(item.updated_at)}
              </p>
            </Card>
          </Link>
        ))}
      </div>
    </div>
  );
}
