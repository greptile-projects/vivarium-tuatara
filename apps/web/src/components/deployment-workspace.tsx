"use client";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  api,
  type Deployment,
  type DeploymentEnvironment,
  type ReleaseBuild,
  type Repository,
} from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

export function DeploymentWorkspace({
  repositoryID,
  releaseID,
  builds,
}: {
  repositoryID: string;
  releaseID: string;
  builds: ReleaseBuild[];
}) {
  const { token, user } = useAuth(),
    [environments, setEnvironments] = useState<DeploymentEnvironment[]>([]),
    [deployments, setDeployments] = useState<Deployment[]>([]),
    [owner, setOwner] = useState(false),
    [recoveryNotice, setRecoveryNotice] = useState(""),
    [error, setError] = useState("");
  const load = useCallback(async () => {
    try {
      const [repo, envs, runs] = await Promise.all([
        api<Repository>(`/repositories/${repositoryID}`, {}, token),
        api<{ environments: DeploymentEnvironment[] }>(
          `/repositories/${repositoryID}/environments`,
          {},
          token,
        ),
        api<{ deployments: Deployment[] }>(
          `/repositories/${repositoryID}/deployments`,
          {},
          token,
        ),
      ]);
      setOwner(repo.owner_id === user?.id);
      setEnvironments(envs.environments);
      setDeployments(
        runs.deployments.filter((x) => x.release_id === releaseID),
      );
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Deployment state could not be loaded.",
      );
    }
  }, [releaseID, repositoryID, token, user]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  useEffect(() => {
    if (!deployments.some((x) => x.state === "queued" || x.state === "running"))
      return;
    const timer = window.setInterval(() => void load(), 1200);
    return () => clearInterval(timer);
  }, [deployments, load]);
  async function createEnvironment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    const form = event.currentTarget,
      data = new FormData(form),
      pairs = (name: string) =>
        Object.fromEntries(
          String(data.get(name) || "")
            .split("\n")
            .filter((line) => line.includes("="))
            .map((line) => [
              line.slice(0, line.indexOf("=")),
              line.slice(line.indexOf("=") + 1),
            ]),
        );
    try {
      await api(
        `/repositories/${repositoryID}/environments`,
        {
          method: "POST",
          body: JSON.stringify({
            name: data.get("name"),
            position: Number(data.get("position")),
            image: data.get("image"),
            command: data.get("command"),
            timeout_seconds: Number(data.get("timeout")),
            required_approvals: Number(data.get("approvals")),
            concurrency: Number(data.get("concurrency")),
            configuration: pairs("configuration"),
            credentials: pairs("credentials"),
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
          : "Environment could not be saved.",
      );
    }
  }
  async function promote(environmentID: string) {
    if (!token) return;
    const artifact = builds.flatMap((build) =>
      build.state === "succeeded"
        ? build.artifacts.map((item) => ({ build, item }))
        : [],
    )[0];
    if (!artifact) {
      setError("A successful build with an artifact is required.");
      return;
    }
    try {
      await api(
        `/repositories/${repositoryID}/deployments`,
        {
          method: "POST",
          body: JSON.stringify({
            environment_id: environmentID,
            release_id: releaseID,
            build_id: artifact.build.id,
            artifact_id: artifact.item.id,
          }),
        },
        token,
      );
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Promotion could not be requested.",
      );
    }
  }
  async function approve(id: string) {
    if (!token) return;
    try {
      await api(
        `/repositories/${repositoryID}/deployments/${id}/approvals`,
        { method: "POST" },
        token,
      );
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Approval could not be recorded.",
      );
    }
  }
  async function control(run: Deployment, action: "pause" | "resume" | "cancel" | "mark_unsuccessful") {
    if (!token) return;
    const reason = window.prompt("Reason for this rollout decision (optional)") ?? "";
    try {
      await api(`/repositories/${repositoryID}/deployments/${run.id}/controls`, {
        method: "POST",
        body: JSON.stringify({ action, expected_state: run.state, reason }),
      }, token);
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Rollout control could not be applied.");
    }
  }
  async function recover(run: Deployment, action: "rollback" | "repair") {
    if (!token) return;
    setError("");
    setRecoveryNotice("");
    try {
      const result = await api<{
        deployment?: Deployment;
        pull_request?: { id: string };
        session?: { id: string };
      }>(`/repositories/${repositoryID}/deployments/${run.id}/recoveries`, {
        method: "POST",
        body: JSON.stringify({ action, expected_state: run.state }),
      }, token);
      if (action === "repair" && result.pull_request && result.session) {
        window.location.assign(`/pulls/${repositoryID}/${result.pull_request.id}/sessions/${result.session.id}`);
        return;
      }
      setRecoveryNotice("Rollback requested with the last known-good artifact. Normal environment approvals still apply.");
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Recovery could not be started.");
    }
  }
  const exactArtifacts = builds.flatMap((build) =>
    build.state === "succeeded" ? build.artifacts : [],
  );
  return (
    <Card className="p-5">
      <h2 className="text-lg font-semibold">Governed environments</h2>
      <p className="mt-1 text-sm text-[var(--muted)]">
        Promote one checksummed artifact through ordered, approval-protected
        shared environments. Credential values are write-only.
      </p>
      {error && (
        <p
          role="alert"
          className="mt-3 rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      {recoveryNotice && <p role="status" className="mt-3 rounded-lg bg-[var(--brand-soft)] p-3 text-sm text-[var(--success)]">{recoveryNotice}</p>}
      {owner && (
        <details className="mt-4 rounded-lg border border-[var(--line)] p-4">
          <summary className="cursor-pointer font-semibold">
            Define environment
          </summary>
          <form
            onSubmit={createEnvironment}
            className="mt-4 grid gap-3 md:grid-cols-2"
          >
            <label className="text-xs font-semibold">
              Name
              <input
                name="name"
                required
                maxLength={100}
                className="mt-1 min-h-10 w-full rounded-lg border px-3 text-sm font-normal"
              />
            </label>
            <label className="text-xs font-semibold">
              Order
              <input
                name="position"
                type="number"
                min="1"
                required
                className="mt-1 min-h-10 w-full rounded-lg border px-3 text-sm font-normal"
              />
            </label>
            <label className="text-xs font-semibold">
              Required approvals
              <input
                name="approvals"
                type="number"
                min="0"
                max="20"
                defaultValue="1"
                required
                className="mt-1 min-h-10 w-full rounded-lg border px-3 text-sm font-normal"
              />
            </label>
            <label className="text-xs font-semibold">
              Executor image
              <input
                name="image"
                required
                defaultValue="alpine:3.22"
                className="mt-1 min-h-10 w-full rounded-lg border px-3 font-mono text-sm font-normal"
              />
            </label>
            <label className="text-xs font-semibold">
              Timeout seconds
              <input
                name="timeout"
                type="number"
                min="1"
                max="3600"
                defaultValue="600"
                required
                className="mt-1 min-h-10 w-full rounded-lg border px-3 text-sm font-normal"
              />
            </label>
            <label className="text-xs font-semibold md:col-span-2">
              Deployment command
              <textarea
                name="command"
                required
                placeholder={'deploy "$VIVARIUM_ARTIFACT"'}
                className="mt-1 w-full rounded-lg border p-3 font-mono text-xs"
              />
            </label>
            <label className="text-xs font-semibold">
              Concurrency
              <input
                name="concurrency"
                type="number"
                min="1"
                max="20"
                defaultValue="1"
                required
                className="mt-1 min-h-10 w-full rounded-lg border px-3 text-sm font-normal"
              />
            </label>
            <label className="text-xs font-semibold">
              Scoped configuration
              <textarea
                name="configuration"
                placeholder="REGION=us-east"
                className="mt-1 w-full rounded-lg border p-3 font-mono text-xs"
              />
            </label>
            <label className="text-xs font-semibold">
              Protected credentials
              <textarea
                name="credentials"
                placeholder="DEPLOY_TOKEN=write-only value"
                className="mt-1 w-full rounded-lg border p-3 font-mono text-xs"
              />
            </label>
            <div>
              <Button type="submit">Save environment</Button>
            </div>
          </form>
        </details>
      )}
      <div className="mt-4 space-y-3">
        {environments.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">
            No shared environments are defined.
          </p>
        ) : (
          environments.map((environment) => {
            const runs = deployments.filter(
              (x) => x.environment_id === environment.id,
            );
            return (
              <div
                key={environment.id}
                className="rounded-lg border border-[var(--line)] p-4"
              >
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div>
                    <h3 className="font-semibold">
                      {environment.position}. {environment.name}
                    </h3>
                    <p className="text-xs text-[var(--muted)]">
                      {environment.required_approvals} approvals ·{" "}
                      {environment.concurrency} concurrent · secrets:{" "}
                      {environment.credential_names.join(", ") || "none"}
                    </p>
                  </div>
                  {token && (
                    <Button
                      variant="secondary"
                      disabled={exactArtifacts.length === 0}
                      onClick={() => promote(environment.id)}
                    >
                      Promote exact artifact
                    </Button>
                  )}
                </div>
                {runs.map((run) => (
                  <div
                    key={run.id}
                    className="mt-3 rounded-md bg-[var(--canvas)] p-3"
                  >
                    <div className="flex items-center gap-2">
                      <Badge
                        tone={
                          run.state === "succeeded"
                            ? "success"
                            : run.state === "failed"
                              ? "danger"
                              : "warning"
                        }
                      >
                        {run.state.replace("_", " ")}
                      </Badge>
                      <code className="text-xs">
                        sha256:{run.artifact_sha256.slice(0, 12)}…
                      </code>
                      {run.state === "pending_approval" &&
                        token &&
                        run.initiated_by !== user?.id && (
                          <Button
                            variant="secondary"
                            onClick={() => approve(run.id)}
                          >
                            Approve
                          </Button>
                        )}
                    </div>
                    <p className="mt-2 text-xs">
                      Revision <code>{run.commit_id.slice(0, 12)}</code> · stage {Math.min(run.current_stage + 1, run.rollout.stages.length)} of {run.rollout.stages.length}: {run.rollout.stages[run.current_stage]?.name || "awaiting rollout"}
                    </p>
                    {token && ["pending_approval", "queued", "running", "paused", "succeeded"].includes(run.state) && (
                      <div className="mt-2 flex flex-wrap gap-2">
                        {run.state === "running" && <Button variant="secondary" onClick={() => control(run, "pause")}>Pause</Button>}
                        {run.state === "paused" && <Button variant="secondary" onClick={() => control(run, "resume")}>Resume</Button>}
                        {["pending_approval", "queued", "running", "paused"].includes(run.state) && <Button variant="secondary" onClick={() => control(run, "cancel")}>Cancel</Button>}
                        {["running", "paused", "succeeded"].includes(run.state) && <Button variant="secondary" onClick={() => control(run, "mark_unsuccessful")}>Mark unsuccessful</Button>}
                      </div>
                    )}
                    {token && ["failed", "canceled"].includes(run.state) && (
                      <div className="mt-3 rounded-lg border border-[var(--warning)] bg-[var(--warning-soft)] p-3">
                        <p className="text-xs font-semibold">Safe recovery</p>
                        <p className="mt-1 text-xs text-[var(--muted)]">Restore the newest earlier successful artifact through governed approvals, or diagnose in a source-only agent workspace that must return through review.</p>
                        <div className="mt-3 flex flex-wrap gap-2">
                          <Button variant="secondary" onClick={() => void recover(run, "rollback")}>Restore last known-good</Button>
                          <Button variant="secondary" onClick={() => void recover(run, "repair")}>Open repair session</Button>
                        </div>
                      </div>
                    )}
                    {run.evidence.length > 0 && <ul className="mt-3 space-y-1" aria-label="Health evidence">
                      {run.evidence.map((item, index) => <li key={`${item.stage}-${item.signal}-${index}`} className="text-xs">
                        <Badge tone={item.state === "passed" ? "success" : "danger"}>{item.state}</Badge>{" "}{item.stage} / {item.signal}{item.message ? ` — ${item.message}` : ""}
                      </li>)}
                    </ul>}
                    <p className="mt-2 text-xs">
                      Initiated by <code>{run.initiated_by}</code> · approvals:{" "}
                      {run.approvals.map((x) => x.actor_id).join(", ") ||
                        "none"}
                    </p>
                    <details className="mt-2">
                      <summary className="cursor-pointer text-xs font-semibold text-[var(--brand)]">
                        Status and logs
                      </summary>
                      <ol className="mt-2 space-y-1">
                        {run.events.map((event) => (
                          <li key={event.sequence} className="text-xs">
                            {new Date(event.created_at).toLocaleTimeString()} ·{" "}
                            {event.kind}
                            {event.actor_id ? (
                              <>
                                {" "}
                                by <code>{event.actor_id}</code>
                              </>
                            ) : null}
                            {event.message ? ` — ${event.message}` : ""}
                          </li>
                        ))}
                      </ol>
                    </details>
                  </div>
                ))}
              </div>
            );
          })
        )}
      </div>
    </Card>
  );
}
