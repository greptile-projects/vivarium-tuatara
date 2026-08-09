"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  api,
  type Branch,
  type Credential,
  type DeploymentEnvironment,
  type EvolutionPlan,
  type RelationshipGraph,
  type ReleaseCandidate,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

export function RelationshipsWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, loading: authLoading } = useAuth();
  const [graph, setGraph] = useState<RelationshipGraph | null>(null);
  const [releases, setReleases] = useState<ReleaseCandidate[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [environments, setEnvironments] = useState<DeploymentEnvironment[]>([]);
  const [evolutions, setEvolutions] = useState<EvolutionPlan[]>([]);
  const [analysisToken, setAnalysisToken] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const load = useCallback(async () => {
    if (authLoading) return;
    try {
      const [nextGraph, releaseSet, branchSet, environmentSet, evolutionSet] =
        await Promise.all([
          api<RelationshipGraph>(
            `/repositories/${repositoryID}/relationships`,
            {},
            token,
          ),
          api<{ releases: ReleaseCandidate[] }>(
            `/repositories/${repositoryID}/releases`,
            {},
            token,
          ),
          api<{ branches: Branch[] }>(
            `/repositories/${repositoryID}/branches`,
            {},
            token,
          ),
          api<{ environments: DeploymentEnvironment[] }>(
            `/repositories/${repositoryID}/environments`,
            {},
            token,
          ),
          api<{ evolutions: EvolutionPlan[] }>(
            `/repositories/${repositoryID}/evolutions`,
            {},
            token,
          ),
        ]);
      setGraph(nextGraph);
      setReleases(releaseSet.releases);
      setBranches(branchSet.branches);
      setEnvironments(environmentSet.environments);
      setEvolutions(
        evolutionSet.evolutions.map((plan) => ({
          ...plan,
          migration_tasks: plan.migration_tasks ?? [],
          contract_candidates: plan.contract_candidates ?? [],
        })),
      );
      setError("");
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Relationship evidence could not be loaded.",
      );
    }
  }, [authLoading, repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  async function publish(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    const form = event.currentTarget;
    const data = new FormData(form);
    try {
      await api(
        `/repositories/${repositoryID}/interfaces`,
        {
          method: "POST",
          body: JSON.stringify({
            name: data.get("name"),
            release_id: data.get("release_id"),
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Interface could not be published.",
      );
    } finally {
      setPending(false);
    }
  }
  async function declare(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    const form = event.currentTarget;
    const data = new FormData(form);
    try {
      await api(
        `/repositories/${repositoryID}/dependencies`,
        {
          method: "POST",
          body: JSON.stringify({
            commit_id: data.get("commit_id"),
            release_id: data.get("release_id") || undefined,
            environment_id: data.get("environment_id") || undefined,
            provider_repository_id: data.get("provider_repository_id"),
            interface_name: data.get("interface_name"),
            constraint: data.get("constraint"),
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Dependency could not be declared.",
      );
    } finally {
      setPending(false);
    }
  }
  async function createEvolution(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    const form = event.currentTarget;
    const data = new FormData(form);
    try {
      await api(
        `/repositories/${repositoryID}/evolutions`,
        {
          method: "POST",
          body: JSON.stringify({
            interface_name: data.get("interface_name"),
            predecessor_interface_id: data.get("predecessor_interface_id"),
            source_kind: data.get("source_kind"),
            source_id: data.get("source_id"),
            candidate_description: data.get("candidate_description"),
            changes: [
              {
                kind: data.get("change_kind"),
                summary: data.get("change_summary"),
                classification: data.get("classification"),
              },
            ],
            strategy: data.get("strategy"),
            sequencing: data.get("sequencing"),
            exceptions: data.get("exceptions"),
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Evolution plan could not be created.",
      );
    } finally {
      setPending(false);
    }
  }
  async function acknowledge(plan: EvolutionPlan, consumerID: string) {
    if (!token) return;
    try {
      await api(
        `/repositories/${repositoryID}/evolutions/${plan.id}/acknowledgements`,
        { method: "POST", body: JSON.stringify({ repository_id: consumerID }) },
        token,
      );
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Impact could not be acknowledged.",
      );
    }
  }
  async function analyze(
    event: FormEvent<HTMLFormElement>,
    plan: EvolutionPlan,
  ) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    const data = new FormData(event.currentTarget);
    try {
      const result = await api<{ credential: Credential & { token: string } }>(
        `/repositories/${repositoryID}/evolutions/${plan.id}/analyses`,
        {
          method: "POST",
          body: JSON.stringify({
            mandate: data.get("mandate"),
            repository_ids: data.getAll("repository_ids"),
            expires_in: 3600,
          }),
        },
        token,
      );
      setAnalysisToken(result.credential.token);
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Analysis could not be delegated.",
      );
    } finally {
      setPending(false);
    }
  }
  async function createMigrationTask(
    event: FormEvent<HTMLFormElement>,
    plan: EvolutionPlan,
  ) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    const form = event.currentTarget;
    const data = new FormData(form);
    try {
      await api(
        `/repositories/${repositoryID}/evolutions/${plan.id}/migration-tasks`,
        {
          method: "POST",
          body: JSON.stringify({
            version: plan.version,
            repository_id: data.get("repository_id"),
            title: data.get("title"),
            completion_criteria: data.get("completion_criteria"),
            target_version: data.get("target_version"),
            dependency_ids: data.getAll("dependency_ids"),
            assignee_type: data.get("assignee_type"),
            assignee_id: data.get("assignee_id"),
            mandate: data.get("mandate"),
            base_revision: data.get("base_revision"),
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Migration task could not be created.",
      );
    } finally {
      setPending(false);
    }
  }
  async function testCombination(
    event: FormEvent<HTMLFormElement>,
    plan: EvolutionPlan,
  ) {
    event.preventDefault();
    if (!token) return;
    setPending(true);
    const form = event.currentTarget;
    const data = new FormData(form);
    try {
      const consumers = Object.fromEntries(
        String(data.get("consumer_pulls") ?? "")
          .split(/\n/)
          .filter(Boolean)
          .map((line) => line.split("=").map((value) => value.trim()))
          .filter((parts) => parts.length === 2),
      );
      await api(
        `/repositories/${repositoryID}/evolutions/${plan.id}/contract-candidates`,
        {
          method: "POST",
          body: JSON.stringify({
            provider_pull_request_id: data.get("provider_pull_request_id"),
            consumer_pull_request_ids: consumers,
          }),
        },
        token,
      );
      form.reset();
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Contract candidate could not be assembled.",
      );
    } finally {
      setPending(false);
    }
  }
  async function configureRollout(event: FormEvent<HTMLFormElement>, plan: EvolutionPlan) {
    event.preventDefault(); if (!token) return; setPending(true);
    const form=event.currentTarget, data=new FormData(form);
    try {
      const phases=String(data.get("phases") ?? "").split(/\n/).filter(Boolean).map((line) => {
        const [name, repositories, tasks="", environments=""] = line.split("|").map((x)=>x.trim());
        return { name, repository_ids: repositories.split(",").map((x)=>x.trim()).filter(Boolean), migration_task_ids: Object.fromEntries(tasks.split(",").filter(Boolean).map((x)=>x.split("=").map((v)=>v.trim()))), environment_ids: Object.fromEntries(environments.split(",").filter(Boolean).map((x)=>x.split("=").map((v)=>v.trim()))) };
      });
      await api(`/repositories/${repositoryID}/evolutions/${plan.id}/rollout`, {method:"PUT", body:JSON.stringify({version:plan.version,candidate_id:data.get("candidate_id"),phases})}, token); form.reset(); await load();
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Rollout could not be configured."); } finally { setPending(false); }
  }
  async function approveRollout(plan: EvolutionPlan, targetRepositoryID: string) {
    if (!token) return; setPending(true); try { await api(`/repositories/${repositoryID}/evolutions/${plan.id}/rollout/approvals`, {method:"POST",body:JSON.stringify({version:plan.version,repository_id:targetRepositoryID})},token); await load(); } catch(reason) { setError(reason instanceof Error ? reason.message : "Rollout approval could not be recorded."); } finally { setPending(false); }
  }
  const names = Object.fromEntries(
    (graph?.repositories ?? []).map((repository) => [
      repository.id,
      repository.name,
    ]),
  );
  return (
    <div className="space-y-6">
      <div>
        <Link
          href={`/repositories/${repositoryID}`}
          className="text-sm font-semibold text-[var(--brand)] hover:underline"
        >
          ← Repository
        </Link>
        <p className="mt-5 font-mono text-xs font-semibold uppercase tracking-[.14em] text-[var(--brand)]">
          Cross-repository evidence
        </p>
        <h1 className="mt-1 text-3xl font-semibold">
          Interface dependency graph
        </h1>
        <p className="mt-2 max-w-3xl text-sm text-[var(--muted)]">
          Published contracts and consumer claims are pinned to exact releases
          and revisions. Current platform records determine whether each edge
          remains trustworthy.
        </p>
      </div>
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      {token && (
        <div className="grid gap-4 lg:grid-cols-2">
          <Card className="p-5">
            <h2 className="font-semibold">Publish an interface</h2>
            <p className="mt-1 text-sm text-[var(--muted)]">
              The selected release supplies both version and exact source
              revision.
            </p>
            <form onSubmit={publish} className="mt-4 grid gap-3">
              <input
                name="name"
                required
                maxLength={100}
                placeholder="Interface name"
                className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 text-sm"
              />
              <select
                name="release_id"
                required
                className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"
              >
                <option value="">Select a release</option>
                {releases.map((release) => (
                  <option key={release.id} value={release.id}>
                    {release.version} · {release.commit_id.slice(0, 12)}
                  </option>
                ))}
              </select>
              <Button type="submit" disabled={pending}>
                Publish interface
              </Button>
            </form>
          </Card>
          <Card className="p-5">
            <h2 className="font-semibold">Declare a dependency</h2>
            <p className="mt-1 text-sm text-[var(--muted)]">
              Constraints use semantic versions, for example{" "}
              <code>&gt;=v1.0.0 &lt;v2.0.0</code>.
            </p>
            <form onSubmit={declare} className="mt-4 grid gap-3">
              <input
                name="provider_repository_id"
                required
                pattern="[0-9a-f]{32}"
                placeholder="Provider repository ID"
                className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 font-mono text-sm"
              />
              <div className="grid gap-3 sm:grid-cols-2">
                <input
                  name="interface_name"
                  required
                  placeholder="Interface name"
                  className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 text-sm"
                />
                <input
                  name="constraint"
                  required
                  placeholder=">=v1.0.0 <v2.0.0"
                  className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 font-mono text-sm"
                />
              </div>
              <select
                name="commit_id"
                required
                className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"
              >
                <option value="">Exact consumer revision</option>
                {branches.map((branch) => (
                  <option key={branch.name} value={branch.commit_id}>
                    {branch.name} · {branch.commit_id.slice(0, 12)}
                  </option>
                ))}
              </select>
              <div className="grid gap-3 sm:grid-cols-2">
                <select
                  name="release_id"
                  className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"
                >
                  <option value="">No release evidence</option>
                  {releases.map((release) => (
                    <option key={release.id} value={release.id}>
                      {release.version}
                    </option>
                  ))}
                </select>
                <select
                  name="environment_id"
                  className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"
                >
                  <option value="">No environment evidence</option>
                  {environments.map((environment) => (
                    <option key={environment.id} value={environment.id}>
                      {environment.name}
                    </option>
                  ))}
                </select>
              </div>
              <Button type="submit" disabled={pending}>
                Declare dependency
              </Button>
            </form>
          </Card>
        </div>
      )}
      <section>
        <h2 className="text-lg font-semibold">Evidence graph</h2>
        <div className="mt-3 grid gap-3">
          {!graph ? (
            <Card className="p-6 text-sm text-[var(--muted)]">
              Loading relationships…
            </Card>
          ) : graph.dependencies.length === 0 ? (
            <Card className="p-6 text-sm text-[var(--muted)]">
              No dependencies have been declared.
            </Card>
          ) : (
            graph.dependencies.map((edge) => (
              <Card key={edge.id} className="p-5">
                <div className="flex flex-wrap items-center gap-2">
                  <Link
                    href={`/repositories/${edge.repository_id}`}
                    className="font-mono text-sm font-semibold text-[var(--brand)] hover:underline"
                  >
                    {names[edge.repository_id] ?? edge.repository_id}
                  </Link>
                  <span aria-hidden>→</span>
                  <Link
                    href={`/repositories/${edge.provider_repository_id}`}
                    className="font-mono text-sm font-semibold text-[var(--brand)] hover:underline"
                  >
                    {names[edge.provider_repository_id] ??
                      edge.provider_repository_id}
                  </Link>
                  <Badge
                    tone={
                      edge.state === "resolved"
                        ? "success"
                        : edge.state === "stale"
                          ? "warning"
                          : "danger"
                    }
                  >
                    {edge.state}
                  </Badge>
                </div>
                <p className="mt-2 text-sm">
                  <strong>{edge.interface_name}</strong>{" "}
                  <code>{edge.constraint}</code>
                  {edge.resolved_version && (
                    <>
                      {" "}
                      resolves to <code>{edge.resolved_version}</code>
                    </>
                  )}
                </p>
                {edge.reason && (
                  <p className="mt-2 text-sm text-[var(--danger)]">
                    {edge.reason}
                  </p>
                )}
                <p className="mt-3 break-all font-mono text-xs text-[var(--muted)]">
                  consumer {edge.commit_id} · owner{" "}
                  {
                    graph.repositories.find(
                      (repository) => repository.id === edge.repository_id,
                    )?.owner_id
                  }
                </p>
              </Card>
            ))
          )}
        </div>
      </section>
      {(graph?.interfaces.length ?? 0) > 0 && (
        <section>
          <h2 className="text-lg font-semibold">Published interfaces</h2>
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            {graph?.interfaces.map((item) => (
              <Card key={item.id} className="p-4">
                <div className="flex items-center gap-2">
                  <strong>{item.name}</strong>
                  <Badge tone={item.stale ? "warning" : "success"}>
                    {item.stale ? "stale" : item.version}
                  </Badge>
                </div>
                <p className="mt-2 font-mono text-xs text-[var(--muted)]">
                  {names[item.repository_id]} · {item.commit_id}
                </p>
                {item.stale_reason && (
                  <p className="mt-2 text-sm text-[var(--danger)]">
                    {item.stale_reason}
                  </p>
                )}
              </Card>
            ))}
          </div>
        </section>
      )}
      {token && (graph?.interfaces.length ?? 0) > 0 && (
        <Card className="p-5">
          <h2 className="font-semibold">Propose an interface evolution</h2>
          <p className="mt-1 text-sm text-[var(--muted)]">
            Compare an open provider proposal or pull request with a released
            contract and record the migration decision before merge.
          </p>
          <form onSubmit={createEvolution} className="mt-4 grid gap-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <select
                name="predecessor_interface_id"
                required
                className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"
              >
                <option value="">Released predecessor</option>
                {graph?.interfaces
                  .filter((x) => x.repository_id === repositoryID && !x.stale)
                  .map((x) => (
                    <option key={x.id} value={x.id}>
                      {x.name} · {x.version}
                    </option>
                  ))}
              </select>
              <input
                name="interface_name"
                required
                placeholder="Interface name"
                className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 text-sm"
              />
            </div>
            <div className="grid gap-3 sm:grid-cols-[10rem_1fr]">
              <select
                name="source_kind"
                className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"
              >
                <option value="proposal">Proposal</option>
                <option value="pull_request">Pull request</option>
              </select>
              <input
                name="source_id"
                required
                pattern="[0-9a-f]{32}"
                placeholder="Open source record ID"
                className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 font-mono text-sm"
              />
            </div>
            <textarea
              name="candidate_description"
              required
              placeholder="Candidate interface shape and behavioral differences"
              className="min-h-24 rounded-lg border border-[var(--line-strong)] p-3 text-sm"
            />
            <div className="grid gap-3 sm:grid-cols-3">
              <input
                name="change_kind"
                required
                placeholder="Change, e.g. removed field"
                className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 text-sm"
              />
              <input
                name="change_summary"
                required
                placeholder="Compatibility rationale"
                className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 text-sm"
              />
              <select
                name="classification"
                className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"
              >
                <option value="breaking">Breaking</option>
                <option value="conditional">Conditional</option>
                <option value="compatible">Compatible</option>
                <option value="unknown">Unknown</option>
              </select>
            </div>
            <textarea
              name="strategy"
              required
              placeholder="Migration strategy"
              className="min-h-20 rounded-lg border border-[var(--line-strong)] p-3 text-sm"
            />
            <textarea
              name="sequencing"
              required
              placeholder="Provider and consumer sequencing contract"
              className="min-h-20 rounded-lg border border-[var(--line-strong)] p-3 text-sm"
            />
            <textarea
              name="exceptions"
              placeholder="Accepted exceptions and rationale"
              className="min-h-16 rounded-lg border border-[var(--line-strong)] p-3 text-sm"
            />
            <Button type="submit" disabled={pending}>
              Create shared evolution plan
            </Button>
          </form>
        </Card>
      )}
      <section>
        <h2 className="text-lg font-semibold">Evolution decisions</h2>
        <div className="mt-3 grid gap-4">
          {evolutions.length === 0 ? (
            <Card className="p-6 text-sm text-[var(--muted)]">
              No interface evolution has been proposed.
            </Card>
          ) : (
            evolutions.map((plan) => (
              <Card key={plan.id} className="p-5">
                <div className="flex flex-wrap items-center gap-2">
                  <strong>{plan.interface_name}</strong>
                  <Badge
                    tone={
                      plan.changes.some((x) => x.classification === "breaking")
                        ? "danger"
                        : "warning"
                    }
                  >
                    {plan.changes.map((x) => x.classification).join(", ")}
                  </Badge>
                  <span className="text-xs text-[var(--muted)]">
                    {plan.predecessor.version} → candidate{" "}
                    {plan.source_kind.replace("_", " ")}
                  </span>
                </div>
                <p className="mt-3 text-sm">{plan.candidate_description}</p>
                <div className="mt-4 grid gap-3 sm:grid-cols-2">
                  <div>
                    <h3 className="text-sm font-semibold">Strategy</h3>
                    <p className="mt-1 whitespace-pre-wrap text-sm text-[var(--muted)]">
                      {plan.strategy}
                    </p>
                  </div>
                  <div>
                    <h3 className="text-sm font-semibold">Sequence</h3>
                    <p className="mt-1 whitespace-pre-wrap text-sm text-[var(--muted)]">
                      {plan.sequencing}
                    </p>
                  </div>
                </div>
                <h3 className="mt-5 text-sm font-semibold">
                  Affected consumers and owners
                </h3>
                <div className="mt-2 grid gap-2">
                  {plan.impacts.length === 0 ? (
                    <p className="text-sm text-[var(--muted)]">
                      No readable declared consumers were found.
                    </p>
                  ) : (
                    plan.impacts.map((impact) => (
                      <div
                        key={impact.dependency_id}
                        className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-[var(--line)] p-3 text-sm"
                      >
                        <span>
                          <Link
                            href={`/repositories/${impact.repository_id}`}
                            className="font-semibold text-[var(--brand)] hover:underline"
                          >
                            {names[impact.repository_id] ??
                              impact.repository_id}
                          </Link>{" "}
                          · owner <code>{impact.owner_id}</code> ·{" "}
                          <code>{impact.constraint}</code>
                        </span>
                        {token &&
                          !plan.acknowledgements.some(
                            (x) => x.repository_id === impact.repository_id,
                          ) && (
                            <Button
                              type="button"
                              onClick={() =>
                                void acknowledge(plan, impact.repository_id)
                              }
                            >
                              Acknowledge
                            </Button>
                          )}
                      </div>
                    ))
                  )}
                </div>
                {plan.findings.map((f) => (
                  <blockquote
                    key={f.id}
                    className="mt-3 border-l-2 border-[var(--brand)] pl-3 text-sm"
                  >
                    <strong>Agent finding:</strong> {f.finding}
                    {f.uncertainty && (
                      <span className="block text-[var(--muted)]">
                        Uncertainty: {f.uncertainty}
                      </span>
                    )}
                  </blockquote>
                ))}
                <h3 className="mt-5 text-sm font-semibold">
                  Compatibility matrix
                </h3>
                <div className="mt-2 grid gap-2">
                  {plan.contract_candidates.length === 0 ? (
                    <p className="text-sm text-[var(--muted)]">
                      No exact provider/consumer combination has been tested.
                    </p>
                  ) : (
                    plan.contract_candidates.map((candidate) => (
                      <div
                        key={candidate.id}
                        className="rounded-lg border border-[var(--line)] p-3 text-sm"
                      >
                        <Badge
                          tone={candidate.superseded_at ? "warning" : "info"}
                        >
                          {candidate.superseded_at ? "superseded" : "retained"}
                        </Badge>{" "}
                        <code>{candidate.combination_hash.slice(0, 12)}</code>
                        <p className="mt-2 text-xs text-[var(--muted)]">
                          {candidate.revisions
                            .map(
                              (revision) =>
                                `${revision.role} ${names[revision.repository_id] ?? revision.repository_id} @ ${revision.commit_id.slice(0, 12)}`,
                            )
                            .join(" · ")}
                        </p>
                        <a
                          href={`/api/repositories/${repositoryID}/evolutions/${plan.id}/contract-candidates/${candidate.id}/checks`}
                          className="text-xs font-semibold text-[var(--brand)] hover:underline"
                        >
                          Checks, artifacts, and attestation
                        </a>
                      </div>
                    ))
                  )}
                </div>
                {token && (
                  <form
                    onSubmit={(event) => testCombination(event, plan)}
                    className="mt-4 grid gap-3 rounded-lg bg-[var(--surface-subtle)] p-4"
                  >
                    <strong className="text-sm">
                      Test intended pull revisions together
                    </strong>
                    <p className="text-xs text-[var(--muted)]">
                      The provider defines <code>.vivarium/contracts.json</code>
                      ; execution has no network or platform credential.
                    </p>
                    <input
                      name="provider_pull_request_id"
                      required
                      pattern="[0-9a-f]{32}"
                      placeholder="Provider pull request ID"
                      className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 font-mono text-sm"
                    />
                    <textarea
                      name="consumer_pulls"
                      required
                      placeholder={
                        "consumer repository ID=pull request ID\none line per consumer"
                      }
                      className="min-h-20 rounded-lg border border-[var(--line-strong)] p-3 font-mono text-sm"
                    />
                    <Button type="submit" disabled={pending}>
                      Assemble and run contract checks
                    </Button>
                  </form>
                )}
                <h3 className="mt-5 text-sm font-semibold">
                  Ordered migration work
                </h3>
                <div className="mt-2 grid gap-2">
                  {plan.migration_tasks.length === 0 ? (
                    <p className="text-sm text-[var(--muted)]">
                      No repository has claimed migration work yet.
                    </p>
                  ) : (
                    plan.migration_tasks.map((task) => (
                      <div
                        key={task.id}
                        className="rounded-lg border border-[var(--line)] p-3 text-sm"
                      >
                        <div className="flex flex-wrap items-center gap-2">
                          <Link
                            href={`/proposals/${task.repository_id}/${task.proposal_id}`}
                            className="font-semibold text-[var(--brand)] hover:underline"
                          >
                            {names[task.repository_id] ?? task.repository_id}
                          </Link>
                          <Badge
                            tone={
                              task.status === "completed"
                                ? "success"
                                : task.ready
                                  ? "info"
                                  : "warning"
                            }
                          >
                            {task.status ?? "linked"}
                          </Badge>
                          <code>{task.target_version}</code>
                        </div>
                        <p className="mt-2 text-xs text-[var(--muted)]">
                          {task.assignee_type ?? "unassigned"}
                          {task.assignee_id && ` · ${task.assignee_id}`}
                          {task.base_revision &&
                            ` · base ${task.base_revision.slice(0, 12)}`}
                          {task.branch && ` · ${task.branch}`}
                        </p>
                        {task.pull_request_id && (
                          <Link
                            href={`/pulls/${task.repository_id}/${task.pull_request_id}`}
                            className="mt-2 inline-block text-xs font-semibold text-[var(--brand)] hover:underline"
                          >
                            Open {task.contribution_status} pull request
                          </Link>
                        )}
                      </div>
                    ))
                  )}
                </div>
                <h3 className="mt-5 text-sm font-semibold">Governed rollout</h3>
                {plan.rollout ? (
                  <div className="mt-2 grid gap-3">
                    <div className="rounded-lg border border-[var(--line)] p-3 text-sm">
                      <Badge tone={plan.rollout.state === "completed" ? "success" : plan.rollout.state === "paused" ? "danger" : "warning"}>{plan.rollout.state ?? "configured"}</Badge>
                      <p className="mt-2 font-semibold">Next: {plan.rollout.next_action}</p>
                      <p className="mt-1 text-xs text-[var(--muted)]">Compatibility gate <code>{plan.rollout.candidate_id}</code></p>
                    </div>
                    {plan.rollout.phases.map((phase, index) => (
                      <div key={phase.id} className="rounded-lg border border-[var(--line)] p-3 text-sm">
                        <div className="flex flex-wrap items-center gap-2"><strong>{index + 1}. {phase.name}</strong><Badge tone={phase.state === "completed" ? "success" : phase.state === "paused" ? "danger" : phase.state === "ready" ? "info" : "warning"}>{phase.state ?? "blocked"}</Badge></div>
                        <p className="mt-2">{phase.next_action}</p>
                        <div className="mt-2 grid gap-2">{phase.repository_ids.map((id) => { const outcome=plan.rollout?.outcomes.find((x)=>x.phase_id===phase.id && x.repository_id===id); const approved=plan.rollout?.approvals.some((x)=>x.repository_id===id); return <div key={id} className="flex flex-wrap items-center justify-between gap-2 rounded bg-[var(--surface-subtle)] p-2"><span><Link href={`/repositories/${id}`} className="font-semibold text-[var(--brand)] hover:underline">{names[id] ?? id}</Link> · {approved ? "owner approved" : "owner approval required"} · {outcome?.state ?? "awaiting work"}</span>{token && !approved && <Button type="button" disabled={pending} onClick={()=>void approveRollout(plan,id)}>Approve my repository</Button>}</div>; })}</div>
                        {phase.state === "paused" && <p className="mt-2 text-xs text-[var(--danger)]">Use the failed deployment’s governed rollback or repair action. Safe repositories and completed phases remain unchanged.</p>}
                      </div>
                    ))}
                  </div>
                ) : token ? (
                  <form onSubmit={(event)=>configureRollout(event,plan)} className="mt-2 grid gap-3 rounded-lg bg-[var(--surface-subtle)] p-4">
                    <strong className="text-sm">Define compatibility gate and rollout phases</strong>
                    <select name="candidate_id" required className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"><option value="">Passing contract candidate</option>{plan.contract_candidates.filter((x)=>!x.superseded_at).map((x)=><option key={x.id} value={x.id}>{x.combination_hash.slice(0,12)}</option>)}</select>
                    <textarea name="phases" required placeholder={"Consumers | repository-id | repository-id=migration-task-id | repository-id=environment-id\nProvider | repository-id | repository-id=migration-task-id"} className="min-h-24 rounded-lg border border-[var(--line-strong)] p-3 font-mono text-sm" />
                    <p className="text-xs text-[var(--muted)]">One phase per line: name | repository IDs | repository=migration-task selections | optional repository=environment targets. Every repository appears once; owners approve independently.</p>
                    <Button type="submit" disabled={pending}>Configure governed rollout</Button>
                  </form>
                ) : <p className="mt-2 text-sm text-[var(--muted)]">No rollout has been configured.</p>}
                {token && (
                  <form
                    onSubmit={(event) => createMigrationTask(event, plan)}
                    className="mt-5 grid gap-3 rounded-lg bg-[var(--surface-subtle)] p-4"
                  >
                    <strong className="text-sm">
                      Claim work in a repository you participate in
                    </strong>
                    <p className="text-xs text-[var(--muted)]">
                      Creating this link grants no repository access. Human
                      assignees must already participate; agents receive only a
                      task branch when started.
                    </p>
                    <select
                      name="repository_id"
                      required
                      className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"
                    >
                      <option value="">Target repository</option>
                      {[
                        repositoryID,
                        ...plan.impacts.map((x) => x.repository_id),
                      ].map((id) => (
                        <option key={id} value={id}>
                          {names[id] ?? id}
                        </option>
                      ))}
                    </select>
                    <div className="grid gap-3 sm:grid-cols-2">
                      <input
                        name="title"
                        required
                        placeholder="Migration task title"
                        className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 text-sm"
                      />
                      <input
                        name="target_version"
                        required
                        placeholder="Target version, e.g. v2.0.0"
                        className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 font-mono text-sm"
                      />
                    </div>
                    <textarea
                      name="completion_criteria"
                      required
                      placeholder="Observable completion criteria"
                      className="min-h-20 rounded-lg border border-[var(--line-strong)] p-3 text-sm"
                    />
                    <textarea
                      name="mandate"
                      required
                      placeholder="Frozen assignee mandate"
                      className="min-h-20 rounded-lg border border-[var(--line-strong)] p-3 text-sm"
                    />
                    <div className="grid gap-3 sm:grid-cols-3">
                      <select
                        name="assignee_type"
                        className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"
                      >
                        <option value="human">Human</option>
                        <option value="agent">Agent</option>
                      </select>
                      <input
                        name="assignee_id"
                        pattern="[0-9a-f]{32}"
                        placeholder="Human collaboration ID"
                        className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 font-mono text-sm"
                      />
                      <input
                        name="base_revision"
                        required
                        pattern="[0-9a-f]{40}"
                        placeholder="Exact target commit"
                        className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 font-mono text-sm"
                      />
                    </div>
                    {plan.migration_tasks.length > 0 && (
                      <fieldset>
                        <legend className="text-xs font-semibold">
                          Wait for earlier merged work
                        </legend>
                        <div className="mt-2 flex flex-wrap gap-3">
                          {plan.migration_tasks.map((task) => (
                            <label key={task.id} className="text-xs">
                              <input
                                type="checkbox"
                                name="dependency_ids"
                                value={task.id}
                                className="mr-2"
                              />
                              {names[task.repository_id] ?? task.repository_id}{" "}
                              · {task.target_version}
                            </label>
                          ))}
                        </div>
                      </fieldset>
                    )}
                    <Button type="submit" disabled={pending}>
                      Create and assign migration task
                    </Button>
                  </form>
                )}
                {token && (
                  <form
                    onSubmit={(event) => analyze(event, plan)}
                    className="mt-5 grid gap-3 rounded-lg bg-[var(--surface-subtle)] p-4"
                  >
                    <strong className="text-sm">
                      Delegate read-only impact analysis
                    </strong>
                    <input
                      name="mandate"
                      required
                      placeholder="Analysis mandate"
                      className="min-h-10 rounded-lg border border-[var(--line-strong)] px-3 text-sm"
                    />
                    <div className="flex flex-wrap gap-3">
                      {[
                        repositoryID,
                        ...plan.impacts.map((x) => x.repository_id),
                      ].map((id) => (
                        <label key={id} className="text-sm">
                          <input
                            type="checkbox"
                            name="repository_ids"
                            value={id}
                            className="mr-2"
                          />
                          {names[id] ?? id}
                        </label>
                      ))}
                    </div>
                    <Button type="submit" disabled={pending}>
                      Issue one-hour analysis access
                    </Button>
                  </form>
                )}
              </Card>
            ))
          )}
        </div>
      </section>
      {analysisToken && (
        <Card className="border-[var(--brand)] p-5">
          <h2 className="font-semibold">Copy the analysis credential now</h2>
          <p className="mt-1 text-sm text-[var(--muted)]">
            It is shown only in this browser state and grants no repository or
            Git write scope.
          </p>
          <code className="mt-3 block break-all rounded-lg bg-[var(--surface-subtle)] p-3 text-xs">
            {analysisToken}
          </code>
          <Button
            type="button"
            className="mt-3"
            onClick={() => setAnalysisToken("")}
          >
            Clear credential
          </Button>
        </Card>
      )}
    </div>
  );
}
