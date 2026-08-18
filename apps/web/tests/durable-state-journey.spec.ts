import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- the journey intentionally inspects cross-resource public projections */

const run = promisify(execFile);

async function git(cwd: string, ...args: string[]) {
  return (await run("git", args, { cwd, env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } })).stdout.trim();
}

async function account(page: Page, suffix: string, name: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(name);
  await page.getByLabel("Handle").fill(`durable-${name.toLowerCase()}-${suffix}`);
  await page.getByRole("button", { name: "Create account and continue" }).click();
  await expect(page.getByRole("heading", { name: "Create a repository" })).toBeVisible();
  const token = await page.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const user = await (await page.request.get("/api/user", { headers })).json() as { id: string };
  return { headers, user };
}

async function request(page: Page, method: "get" | "post" | "put" | "patch", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}

async function rejected(response: APIResponse, status: number, code: string) {
  expect(response.status(), await response.text()).toBe(status);
  await expect(response.json()).resolves.toMatchObject({ error: { code } });
}

async function eventually<T>(read: () => Promise<T>, ready: (value: T) => boolean, message: string) {
  await expect.poll(async () => ready(await read()), { message, timeout: 60_000, intervals: [250, 500, 1000] }).toBe(true);
  return read();
}

test("collaborators evolve durable state from reviewed intent through verified cleanup", async ({ browser }) => {
  test.setTimeout(300_000);
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copy = await mkdtemp(join(tmpdir(), "vivarium-durable-state-"));
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const engineerPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, suffix, "Owner");
    const engineer = await account(engineerPage, suffix, "Engineer");
    const repository = await request(ownerPage, "post", "/repositories", owner.headers, { name: `ledger-${suffix}` }) as { id: string };
    await request(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: engineer.user.id });
    const credential = await request(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "durable state journey", scopes: ["git:read", "git:write"], expires_in: 3600 }) as { token: string };
    await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${repository.id}.git`, copy);
    await git(copy, "config", "user.name", "Durable State Engineer");
    await git(copy, "config", "user.email", "durable-state@example.test");
    await mkdir(join(copy, ".vivarium"));
    await writeFile(join(copy, ".vivarium", "workspace.json"), JSON.stringify({ version: 1, image: "alpine:3.22", tools: [{ name: "sh", version: "3.22" }], dependencies: ["sh"], setup: ["test -f schema.sql"], experiments: [], resources: { cpus: 1, memory_mb: 256, storage_mb: 128, setup_seconds: 30 } }));
    await writeFile(join(copy, ".vivarium", "release.json"), JSON.stringify({ version: 1, steps: [{ name: "package", image: "alpine:3.22", command: "cp schema.sql \"$VIVARIUM_OUTPUT/schema.sql\"" }] }));
    await writeFile(join(copy, ".vivarium", "deployment.json"), JSON.stringify({ version: 1, stages: [{ name: "health", signals: [{ name: "schema readable", command: "test -r \"$VIVARIUM_ARTIFACT\"" }] }] }));
    await writeFile(join(copy, ".vivarium", "packages.json"), JSON.stringify({ version: 1, dependencies: [], lock: [] }));
    await writeFile(join(copy, "schema.sql"), "CREATE TABLE accounts (id TEXT PRIMARY KEY);\n");
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Establish durable schema baseline"); await git(copy, "push", "origin", "main");

    async function reviewedSchemaChange(branch: string, definition: string, title: string) {
      await git(copy, "switch", "-C", branch, "origin/main");
      await writeFile(join(copy, "schema.sql"), definition);
      await git(copy, "add", "schema.sql"); await git(copy, "commit", "-m", title); await git(copy, "push", "-f", "origin", branch);
      const pull = await request(engineerPage, "post", `/repositories/${repository.id}/pulls`, engineer.headers, { title, body: "Reviewed persistent-state contract.", source_branch: branch, target_branch: "main" }) as { id: string };
      await request(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/reviews`, owner.headers, { decision: "approved" });
      const merged = await request(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/merge`, owner.headers, {}) as { merge_commit_id: string };
      await git(copy, "fetch", "origin", "main");
      return { pull, commit: merged.merge_commit_id };
    }

    const v1Definition = "CREATE TABLE accounts (id TEXT PRIMARY KEY, display_name TEXT);\n";
    const v1 = await reviewedSchemaChange("schema-v1", v1Definition, "Publish accounts schema v1");
    let schema = await request(engineerPage, "post", `/repositories/${repository.id}/durable-schemas`, engineer.headers, { revision: { name: "accounts", store_kind: "database", description: "Account identity and profile state", definition: v1Definition, definition_path: "schema.sql", owner_ids: [owner.user.id], compatibility: ["old readers tolerate additive columns"], retention: "identity retained until account deletion", privacy: ["display names are personal data", "rehearsals use synthetic rows only"], links: [{ kind: "service", id: "identity-api", label: "Identity API" }], pull_request_id: v1.pull.id, reviewed_commit: v1.commit, rationale: "Publish the current reviewed contract." } }) as any;

    const v2Definition = "CREATE TABLE accounts (id TEXT PRIMARY KEY, public_name TEXT NOT NULL);\n";
    const v2 = await reviewedSchemaChange("schema-v2", v2Definition, "Propose breaking account-name schema");
    schema = await request(engineerPage, "post", `/repositories/${repository.id}/durable-schemas/${schema.id}/revisions`, engineer.headers, { expected_version: 1, revision: { name: "accounts", store_kind: "database", description: "Account identity with normalized public names", definition: v2Definition, definition_path: "schema.sql", owner_ids: [owner.user.id], compatibility: ["dual write display_name and public_name before cutover"], retention: "identity retained until account deletion", privacy: ["public_name remains personal data", "backfill logs contain aggregate counts only"], links: [{ kind: "service", id: "identity-api", label: "Identity API" }, { kind: "environment", id: "production", label: "Production" }], pull_request_id: v2.pull.id, reviewed_commit: v2.commit, rationale: "Replace display_name only after governed coexistence." } });
    schema = await request(engineerPage, "post", `/repositories/${repository.id}/durable-schemas/${schema.id}/migrations`, engineer.headers, { from_version: 1, to_version: 2, source_kind: "pull_request", source_id: v2.pull.id, summary: "Expand, dual-write, backfill, cut over, and remove display_name", operations: [{ id: "writers", kind: "write", description: "Fence old writers and dual-write both names", owner_ids: [owner.user.id], consumer_ids: ["identity-api", "profile-worker"], destructive: false, rollback_limit: "before contract" }, { id: "backfill", kind: "backfill", description: "Normalize existing names without exposing row data", owner_ids: [owner.user.id], consumer_ids: ["profile-worker"], destructive: false, rollback_limit: "idempotent until contract" }, { id: "contract", kind: "destructive", description: "Drop display_name after observation", owner_ids: [owner.user.id], consumer_ids: ["identity-api"], destructive: true, rollback_limit: "point of no return is contract phase" }], steps: [{ id: "evolve", operation_ids: ["writers", "backfill", "contract"], description: "Complete compatibility and cleanup", success_measures: ["old and new readers agree", "aggregate row counts match", "service stays healthy"], required_approver_ids: [owner.user.id] }], rollback_limits: ["traffic rollback only before contract", "physical deletion is irreversible"] });
    let migration = schema.migrations[0];
    const contract = { old_readers: ["display_name reader"], new_readers: ["public_name reader with fallback"], old_writers: ["display_name writer must be fenced"], new_writers: ["dual writer"], rollout_flags: ["public_name_reads"], idempotency: "backfill keyed by account id and safe to retry", transformations: ["trim display_name into public_name"], ownership: [owner.user.id], rollback_assumptions: ["both columns exist until contract"] };
    const humanWorkResult = await request(engineerPage, "post", `/repositories/${repository.id}/durable-schemas/${schema.id}/migrations/${migration.id}/work`, engineer.headers, { expected_version: migration.version, kind: "compatibility", step_id: "evolve", repository_id: repository.id, title: "Implement compatible readers and writers", completion_criteria: "Old and new application revisions safely coexist.", dependency_ids: [], assignee_type: "human", assignee_id: engineer.user.id, mandate: "Implement the reviewed dual-read and dual-write contract.", base_revision: v2.commit, contract }) as any;
    migration = humanWorkResult.schema.migrations[0];
    const agentWorkResult = await request(engineerPage, "post", `/repositories/${repository.id}/durable-schemas/${schema.id}/migrations/${migration.id}/work`, engineer.headers, { expected_version: migration.version, kind: "backfill", step_id: "evolve", repository_id: repository.id, title: "Build idempotent aggregate-only backfill", completion_criteria: "Synthetic rows transform once and aggregate invariants pass.", dependency_ids: [humanWorkResult.migration_work.id], assignee_type: "agent", assignee_id: "", mandate: "Write only the bounded backfill change; receive no environment or database authority.", base_revision: v2.commit, contract }) as any;
    migration = agentWorkResult.schema.migrations[0];

    const checks = [{ id: "migration", kind: "upgrade", command: "grep -q 'rehearsal-ready' schema.sql", invariant: "schema retains account identity", invariant_command: "grep -q 'id TEXT PRIMARY KEY' schema.sql", revision_inputs: ["application", "schema_from", "schema_to", "migration", "data_shape"] }];
    const rehearsalResult = await request(engineerPage, "post", `/repositories/${repository.id}/durable-schemas/${schema.id}/migrations/${migration.id}/rehearsals`, engineer.headers, { expected_version: migration.version, rehearsal: { name: "Synthetic account-name rehearsal", application_revision: v2.commit, dataset: { kind: "synthetic", description: "Ten generated accounts with empty and Unicode names", privacy_method: "generated values; no production records", digest: "sha256:synthetic-account-shape", max_bytes: 4096, row_count: 10, object_count: 0 }, dependencies: [{ name: "migration image", revision: "alpine-3.22", digest: "sha256:alpine-3.22" }], checks } }) as any;
    migration = rehearsalResult.schema.migrations[0];
    const rehearsal = rehearsalResult.rehearsal;

    async function rehearsalRun(ready: boolean) {
      const workspace = await request(engineerPage, "post", "/workspaces", engineer.headers, { repository_id: repository.id, commit_id: v2.commit, source: { kind: "repository" } }) as any;
      if (ready) await request(engineerPage, "put", `/workspaces/${workspace.id}/file`, engineer.headers, { path: "schema.sql", content: `${v2Definition}-- rehearsal-ready\n`, expected_sha256: createHash("sha256").update(v2Definition).digest("hex") });
      await request(engineerPage, "post", `/workspaces/${workspace.id}/commands`, engineer.headers, { command: checks[0].command, timeout_seconds: 30 });
      await request(engineerPage, "post", `/workspaces/${workspace.id}/commands`, engineer.headers, { command: checks[0].invariant_command, timeout_seconds: 30 });
      return request(engineerPage, "post", `/repositories/${repository.id}/durable-schemas/${schema.id}/migrations/${migration.id}/rehearsals/${rehearsal.id}/runs`, engineer.headers, { workspace_id: workspace.id, outcomes: [{ check_id: "migration" }] }) as Promise<any>;
    }
    const failedRun = await rehearsalRun(false);
    expect(failedRun.run).toMatchObject({ result: "failed", outcomes: [{ status: "failed", invariant_passed: true }] });
    await request(ownerPage, "post", `/repositories/${repository.id}/durable-schemas/${schema.id}/migrations/${migration.id}/rehearsals/${rehearsal.id}/notes`, owner.headers, { run_id: failedRun.run.id, body: "The candidate command was stale; no source rows or reusable credentials entered the workspace evidence." });
    const passingRun = await rehearsalRun(true);
    expect(passingRun.run.result).toBe("passed");
    migration = passingRun.schema.migrations[0];
    const approved = await request(ownerPage, "post", `/repositories/${repository.id}/durable-schemas/${schema.id}/migrations/${migration.id}/events`, owner.headers, { expected_version: migration.version, event: { kind: "approved", step_id: "evolve", summary: "Passing synthetic proof and privacy bounds accepted." } }) as any;
    migration = approved.migrations[0];

    const release = await request(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: "v2.0.0", notes: "Reviewed dual-compatible account schema", commit_id: v2.commit }) as any;
    await request(ownerPage, "post", `/repositories/${repository.id}/dependency-inventories`, owner.headers, { commit_id: v2.commit });
    await request(ownerPage, "post", `/repositories/${repository.id}/releases/${release.id}/builds`, owner.headers, {});
    const builds = await eventually(() => request(ownerPage, "get", `/repositories/${repository.id}/releases/${release.id}/builds`, owner.headers), (value: any) => value.builds?.some((item: any) => item.state === "succeeded"), "migration release builds");
    const build = builds.builds.find((item: any) => item.state === "succeeded");
    const environment = await request(ownerPage, "post", `/repositories/${repository.id}/environments`, owner.headers, { name: "production", position: 1, image: "alpine:3.22", command: "test -r \"$VIVARIUM_ARTIFACT\"", timeout_seconds: 30, configuration: {}, credentials: {}, required_approvals: 0, concurrency: 1 }) as any;
    const pendingDeployment = await request(ownerPage, "post", `/repositories/${repository.id}/deployments`, owner.headers, { environment_id: environment.id, release_id: release.id, build_id: build.id, artifact_id: build.artifacts[0].id }) as any;
    const deployments = await eventually(() => request(ownerPage, "get", `/repositories/${repository.id}/deployments`, owner.headers), (value: any) => value.deployments?.some((item: any) => item.id === pendingDeployment.id && item.state === "succeeded"), "migration release deploys");
    const deployment = deployments.deployments.find((item: any) => item.id === pendingDeployment.id);

    const opened = await request(ownerPage, "post", `/repositories/${repository.id}/durable-schemas/${schema.id}/migrations/${migration.id}/executions`, owner.headers, { expected_version: migration.version, execution: { environment_id: environment.id, release_id: release.id, rehearsal_id: rehearsal.id, compatibility_window: "old and new readers coexist until contract", observation_period_seconds: 1, privacy_constraints: ["aggregate counts and service metrics only"], cost_budget_units: 100, abort_reversible_until: "before contract" } }) as any;
    let execution = opened.execution;
    const controls = `/repositories/${repository.id}/durable-schemas/${schema.id}/migrations/${migration.id}/executions/${execution.id}`;
    const control = async (action: string, extra: Record<string, unknown> = {}) => {
      const result = await request(ownerPage, "post", `${controls}/controls`, owner.headers, { action, expected_version: execution.version, ...extra });
      execution = result.execution;
      return result;
    };
    const recover = async (failure: any, key: string, extra: Record<string, unknown> = {}) => {
      const result = await request(ownerPage, "post", `${controls}/recoveries`, owner.headers, { expected_version: execution.version, idempotency_key: key, kind: "retry", failure_id: failure.id, summary: `Recover ${failure.kind} from retained evidence.`, evidence: ["bounded aggregate evidence reviewed"], ...extra });
      execution = result.execution;
    };
    const completePhase = async (deploymentID = "") => {
      const phase = execution.phases[execution.current_phase].name;
      await control("report", { phase, progress_percent: 100, lag_seconds: 0, invariants: [`${phase} invariants hold`], service_health: "healthy", blockers: [], next_actions: ["advance"], cost_units: execution.current_phase + 1, deployment_id: deploymentID, summary: `${phase} controller evidence is healthy.` });
      await control("advance", { summary: `${phase} evidence accepted.` });
    };

    await control("start");
    await control("report", { phase: "expand", progress_percent: 40, lag_seconds: 1, invariants: ["new column is additive"], service_health: "degraded", blockers: ["obsolete writer lease active"], next_actions: ["fence old writer"], cost_units: 1, failure_kind: "conflicting_writes", safety_point: "before enabling dual writes", failure_evidence: ["writer generations overlap"], summary: "Concurrent old writer detected and contained." });
    await recover(execution.failures.at(-1), "fence-old-writer", { recovery_attestation: "writer lease generation advanced and obsolete credential was fenced" });
    await control("resume"); await completePhase();
    await control("start"); await completePhase(deployment.id);
    await control("start");
    await control("report", { phase: "backfill", progress_percent: 35, lag_seconds: 4, invariants: ["completed batches match"], service_health: "healthy", blockers: ["worker interrupted"], next_actions: ["resume from cursor"], cost_units: 3, failure_kind: "interrupted_backfill", safety_point: "after account id cursor 350", failure_evidence: ["checkpoint digest retained"], summary: "Backfill paused at an idempotent cursor." });
    await recover(execution.failures.at(-1), "resume-backfill"); await control("resume");
    await control("report", { phase: "backfill", progress_percent: 75, lag_seconds: 2, invariants: ["row count diverged"], service_health: "degraded", blockers: ["aggregate mismatch"], next_actions: ["rerun bounded batch"], cost_units: 4, failure_kind: "failed_invariant", safety_point: "before committing batch 8", failure_evidence: ["expected 1000 rows; observed 999"], summary: "Aggregate row-count invariant breached." });
    await recover(execution.failures.at(-1), "retry-batch-eight"); await control("resume"); await completePhase();
    await control("start"); await completePhase();
    await control("start");
    await control("report", { phase: "contract", progress_percent: 20, lag_seconds: 0, invariants: ["cleanup checksum pending"], service_health: "degraded", blockers: ["cleanup checksum mismatch"], next_actions: ["repair cleanup input"], cost_units: 5, failure_kind: "failed_invariant", safety_point: "after contract point of no return", failure_evidence: ["obsolete field checksum mismatch"], summary: "Contract invariant stopped cleanup without rewriting prior phases." });
    await rejected(await ownerPage.request.post(`/api${controls}/recoveries`, { headers: owner.headers, data: { expected_version: execution.version, idempotency_key: "late-traffic-rollback", kind: "traffic_rollback", failure_id: execution.failures.at(-1).id, summary: "Attempt traffic rollback after contract began.", evidence: ["old release remains retained"], rollback_release_id: release.id } }), 422, "migration_recovery_blocked");
    await recover(execution.failures.at(-1), "retry-contract-check"); await control("resume"); await completePhase();
    expect(execution.status).toBe("completed");

    let finalSchema = await request(ownerPage, "get", `/repositories/${repository.id}/durable-schemas/${schema.id}`, owner.headers) as any;
    migration = finalSchema.migrations[0];
    finalSchema = await request(ownerPage, "post", `/repositories/${repository.id}/durable-schemas/${schema.id}/migrations/${migration.id}/retirement-approvals`, owner.headers, { expected_version: migration.version, summary: "Observation is healthy; remove temporary compatibility machinery." });
    migration = finalSchema.migrations[0];
    const observationStarted = new Date();
    await new Promise((resolve) => setTimeout(resolve, 1200));
    const observationEnded = new Date();
    const completionResult = await request(ownerPage, "post", `/repositories/${repository.id}/durable-schemas/${schema.id}/migrations/${migration.id}/completion`, owner.headers, { expected_version: migration.version, completion: { observation_started_at: observationStarted.toISOString(), observation_ended_at: observationEnded.toISOString(), compatibility_removed: ["dual writer and display_name fallback"], obsolete_fields: ["accounts.display_name"], irreversible_decisions: ["display_name physically removed after point of no return"], environments: [{ environment_id: environment.id, current_version: 2, retained_data: ["all account identities"], changed_data: ["public names normalized"], verified_deletion: ["display_name absent from exact schema digest"], exceptions: ["none"], cost_units: 15 }] } }) as any;
    expect(completionResult.completion).toMatchObject({ approved_by: [owner.user.id], completed_by: owner.user.id });

    await ownerPage.goto(`/repositories/${repository.id}/durable-state`);
    await expect(ownerPage.getByRole("heading", { name: "Durable state" })).toBeVisible();
    await expect(ownerPage.getByText("Expand, dual-write, backfill, cut over, and remove display_name")).toBeVisible();
    await expect(ownerPage.getByText(/Contained conflicting_writes.*Concurrent old writer detected/)).toBeVisible();
    await expect(ownerPage.getByText(/Contained interrupted_backfill.*Backfill paused/)).toBeVisible();
    await expect(ownerPage.getByText(/Contained failed_invariant.*Aggregate row-count invariant breached/)).toBeVisible();
    await expect(ownerPage.getByText("Recovery retry: Recover conflicting_writes from retained evidence.")).toBeVisible();
    await expect(ownerPage.getByText("Migration cleanup verified")).toBeVisible();
    await expect(ownerPage.getByText(/display_name absent from exact schema digest/)).toBeVisible();
    await ownerPage.getByText("Open production execution", { exact: true }).click();
    const executionForm = ownerPage.locator('form:has(input[name="observation_period"])');
    await executionForm.locator('input[name="observation_period"]').fill("1.5");
    await executionForm.dispatchEvent("submit");
    await expect(ownerPage.getByText("Observation period must be a whole number from 1 to 31,536,000 seconds.")).toBeVisible();
  } finally {
    await rm(copy, { recursive: true, force: true });
  }
});
