import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey verifies one cross-resource retained trail */

const run = promisify(execFile);
const digest = (value: string) => createHash("sha256").update(value).digest("hex");
async function git(cwd: string, ...args: string[]) {
  return (await run("git", args, { cwd, env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } })).stdout.trim();
}
async function account(page: Page, suffix: string, role: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(`${role} collaborator`);
  await page.getByLabel("Handle").fill(`debug-${role.toLowerCase()}-${suffix}`);
  await page.getByRole("button", { name: "Create account and continue" }).click();
  await expect(page.getByRole("heading", { name: "Create a repository" })).toBeVisible();
  const token = await page.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const user = await (await page.request.get("/api/user", { headers })).json();
  return { headers, user };
}
async function json(page: Page, method: "get" | "post" | "put", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}
async function rejected(response: APIResponse, status: number, code: string) {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
  expect(JSON.parse(body)).toMatchObject({ error: { code } });
}
async function eventually<T>(read: () => Promise<T>, ready: (value: T) => boolean, label: string) {
  let value = await read();
  await expect.poll(async () => { value = await read(); return ready(value); }, { timeout: 60_000, message: label }).toBeTruthy();
  return value;
}

test("a team turns an intermittent production failure into a verified human-agent repair", async ({ browser }) => {
  test.setTimeout(300_000);
  const dockerAvailable = await run("docker", ["info"]).then(() => true, () => false);
  test.skip(!dockerAvailable, "the debugging journey requires bounded replay, check, build, and deployment workspaces");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copy = await mkdtemp(join(tmpdir(), "vivarium-debugging-"));
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const developerPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, suffix, "Owner");
    const developer = await account(developerPage, suffix, "Developer");
    const repository = await json(ownerPage, "post", "/repositories", owner.headers, { name: `intermittent-checkout-${suffix}` });
    await json(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: developer.user.id });
    const credential = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "debugging journey", scopes: ["git:read", "git:write"], expires_in: 3600 });
    await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${repository.id}.git`, copy);
    await git(copy, "config", "user.name", "Runtime Owner"); await git(copy, "config", "user.email", "owner@example.test");
    await mkdir(join(copy, ".vivarium"));
    const replayCommand = "grep -q 'retry_guard=on' service.txt";
    await writeFile(join(copy, ".vivarium", "workspace.json"), JSON.stringify({ version: 1, image: "alpine:3.22", tools: [], dependencies: [], setup: [], experiments: [{ name: "intermittent replay", command: replayCommand }], resources: { cpus: 1, memory_mb: 256, storage_mb: 128, setup_seconds: 30 } }));
    await writeFile(join(copy, ".vivarium", "checks.json"), JSON.stringify({ version: 1, checks: [{ name: "intermittent replay", image: "alpine:3.22", command: replayCommand }, { name: "checkout regression", image: "alpine:3.22", command: "grep -q 'repair=reviewed' service.txt" }] }));
    await writeFile(join(copy, ".vivarium", "release.json"), JSON.stringify({ version: 1, steps: [{ name: "package", image: "alpine:3.22", command: "cp service.txt \"$VIVARIUM_OUTPUT/service.txt\"" }] }));
    await writeFile(join(copy, ".vivarium", "deployment.json"), JSON.stringify({ version: 1, stages: [{ name: "staged", signals: [{ name: "checkout journey", command: "grep -q 'repair=reviewed' \"$VIVARIUM_ARTIFACT\"" }] }] }));
    await writeFile(join(copy, ".vivarium", "packages.json"), JSON.stringify({ version: 1, dependencies: [], lock: [] }));
    await writeFile(join(copy, "service.txt"), "retry_guard=on\nrepair=pending\n");
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Release intermittent checkout path"); await git(copy, "push", "origin", "main");
    const affectedRevision = await git(copy, "rev-parse", "HEAD");
    await json(ownerPage, "put", `/repositories/${repository.id}/branches/main/required-checks`, owner.headers, { checks: ["intermittent replay", "checkout regression"] });
    const release = await json(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: "v1.0.0", notes: "Affected checkout release", commit_id: affectedRevision });
    const environment = await json(ownerPage, "post", `/repositories/${repository.id}/environments`, owner.headers, { name: "production", position: 1, image: "alpine:3.22", command: "true", timeout_seconds: 30, configuration: {}, credentials: {}, required_approvals: 0, concurrency: 1 });
    const issue = await json(developerPage, "post", `/repositories/${repository.id}/issues`, developer.headers, { release_id: release.id, title: "Checkout intermittently stalls", expected_behavior: "Checkout completes", observed_behavior: "Some released users time out", severity: "high", environment: "production", reproduction_steps: ["Submit a released checkout"], visibility: "repository" });

    let workspace = await json(developerPage, "post", `/repositories/${repository.id}/debugging-workspaces`, developer.headers, {
      title: "Released checkout intermittently stalls", summary: "Affected users see a timeout only in production", trigger: { kind: "issue", resource_id: issue.id, label: issue.title },
      release: { kind: "release", resource_id: release.id, revision: affectedRevision, label: "v1.0.0" }, environment: { kind: "environment", resource_id: environment.id, label: "production" },
      time_start: new Date(Date.now() - 3_600_000).toISOString(), time_end: new Date().toISOString(), user_journey: "Submit checkout", owner_ids: [owner.user.id], severity: "high", audience: "repository",
      source: { kind: "source", revision: affectedRevision, label: "released source" }, configuration: { kind: "configuration", revision: affectedRevision, label: "release configuration" },
      permitted_evidence: [{ kind: "trace", reference: "trace://checkout/sanitized", label: "Sanitized intermittent span", visibility: "repository", sanitization: "user fields removed before retention", available: true }], unavailable_context: ["one noisy shard was unavailable"],
    });
    const probePolicy = { data_categories: ["timing_spans", "request_metadata"], privacy: "remove_user_identifiers", security: "redact_secrets", retention_hours: 1, sample_percent: 5, max_cost_cents: 40, max_load_percent: 2 };
    const expiresAt = new Date(Date.now() + 3_600_000).toISOString();
    workspace = await json(developerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/probes`, developer.headers, { expected_version: workspace.version, probe: { kind: "traces", purpose: "inspect the intermittent checkout", audience_user_ids: [developer.user.id, owner.user.id], requested_policy: probePolicy, expires_at: expiresAt } });
    workspace = await json(ownerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/probes/${workspace.probes[0].id}/decision`, owner.headers, { expected_version: workspace.version, decision: "denied", reason: "request fields exceed current consent", policy: probePolicy, expires_at: expiresAt });
    workspace = await json(developerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/probes`, developer.headers, { expected_version: workspace.version, probe: { kind: "traces", purpose: "collect timing-only bounded spans", audience_user_ids: [developer.user.id, owner.user.id], requested_policy: { ...probePolicy, data_categories: ["timing_spans"], privacy: "remove_user_data", sample_percent: 2 }, expires_at: expiresAt } });
    const approvedPolicy = { ...probePolicy, data_categories: ["timing_spans"], privacy: "remove_user_data", sample_percent: 2 };
    workspace = await json(ownerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/probes/${workspace.probes[1].id}/decision`, owner.headers, { expected_version: workspace.version, decision: "approved", reason: "timing-only capture is consented and bounded", policy: approvedPolicy, expires_at: expiresAt });
    const captureTime = new Date().toISOString();
    workspace = await json(developerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/probes/${workspace.probes[1].id}/actions`, developer.headers, { expected_version: workspace.version, action: { outcome: "partial", started_at: captureTime, finished_at: captureTime, provenance: "bounded production collector checkout-1", transformations: ["removed user_id field", "redacted authorization header"], gaps: ["noisy shard exceeded sampling budget"], artifacts: [{ kind: "trace", digest: "a".repeat(64), size_bytes: 512, reference: "artifact://checkout/trace-1", redaction: "user_id removed; secret-bearing spans dropped" }] } });

    const evidenceID = workspace.permitted_evidence[0].id;
    workspace = await json(developerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/claims`, developer.headers, { expected_version: workspace.version, claim: { kind: "hypothesis", statement: "The payment provider causes the timeout", uncertainty: "the partial trace is noisy", confidence: "low" }, citations: [{ kind: "runtime_evidence", evidence_id: evidenceID, label: "bounded checkout trace" }] });
    const wrong = workspace.claims[0];
    workspace = await json(ownerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/claims/${wrong.id}/responses`, owner.headers, { expected_version: workspace.version, kind: "dispute", message: "Exact source and spans place the delay before provider dispatch", citation_ids: [workspace.citations[0].id] });
    workspace = await json(ownerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/claims`, owner.headers, { expected_version: workspace.version, claim: { kind: "finding", statement: "The released retry guard waits after queue cancellation", uncertainty: "rare scheduling remains synthetic", confidence: "high" }, citations: [{ kind: "symbol", revision: affectedRevision, path: "service.txt", symbol: "retry_guard", line_start: 1, line_end: 2, label: "released retry guard" }] });
    const finding = workspace.claims.at(-1); const findingCitation = workspace.citations.at(-1);
    workspace = await json(ownerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/claims/${finding.id}/responses`, owner.headers, { expected_version: workspace.version, kind: "support", message: "Owner confirms the exact released guard and deployment state", citation_ids: [findingCitation.id] });
    const launched = await json(developerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/agent-investigations`, developer.headers, { expected_version: workspace.version, mandate: "Correlate only the selected trace and exact released guard", citation_ids: [workspace.citations[0].id, findingCitation.id], expires_in: 600 });
    workspace = launched.debugging_workspace;
    await json(developerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/agent-investigations/${launched.agent_investigation.id}/claims`, { Authorization: `Bearer ${launched.credential.token}` }, { expected_version: workspace.version, claim: { kind: "uncertainty", statement: "Selected evidence supports the retry path but not the noisy shard", uncertainty: "sampling gap retained", confidence: "medium", citation_ids: [findingCitation.id] } });
    workspace = await json(developerPage, "get", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}`, developer.headers);
    workspace = await json(ownerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/agent-investigations/${launched.agent_investigation.id}/controls`, owner.headers, { expected_version: workspace.version, action: "revoke", message: "The bounded correlation is complete" });
    await rejected(await developerPage.request.get(`/api/repositories/${repository.id}/debugging-workspaces/${workspace.id}/agent-investigations/${launched.agent_investigation.id}`, { headers: { Authorization: `Bearer ${launched.credential.token}` } }), 401, "unauthorized");

    const scenarioResult = await json(developerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/replay-scenarios`, developer.headers, { expected_version: workspace.version, scenario: { title: "Synthetic cancelled checkout replay", objective: "Exercise the released retry guard without user data", evidence_citation_ids: [findingCitation.id], inputs: [{ name: "checkout shape", kind: "synthetic", schema: "cancelled checkout timing", sha256: "b".repeat(64), sanitization: "generated identifiers and timings" }], dependencies: ["alpine:3.22"], commands: [{ name: "intermittent replay", sha256: digest(replayCommand), purpose: "verify the retry guard" }], invariants: [{ name: "retry guard remains active", command_name: "intermittent replay", expected_exit_code: 0, description: "synthetic cancellation reaches the bounded guard" }], production_differences: ["synthetic scheduler"], unsafe_side_effects: [], gaps: [] } });
    workspace = scenarioResult.debugging_workspace; let scenario = scenarioResult.replay_scenario;
    const replay = async (pass: boolean, cost: number) => {
      const isolated = await json(developerPage, "post", "/workspaces", developer.headers, { repository_id: repository.id, commit_id: affectedRevision, source: { kind: "debugging_reproduction", debugging_workspace_id: workspace.id, replay_scenario_id: scenario.id } });
      if (!pass) await json(developerPage, "put", `/workspaces/${isolated.id}/file`, developer.headers, { path: "service.txt", content: "retry_guard=off\nrepair=pending\n", expected_sha256: digest("retry_guard=on\nrepair=pending\n") });
      const outcome = await json(developerPage, "post", `/workspaces/${isolated.id}/commands`, developer.headers, { command: replayCommand, timeout_seconds: 30 });
      const retained = await json(developerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/replay-scenarios/${scenario.id}/attempts`, developer.headers, { expected_version: workspace.version, attempt: { workspace_id: isolated.id, command_outcome_ids: [outcome.outcome.id], cost_cents: cost, production_differences: ["synthetic scheduler"], gaps: [] } });
      workspace = retained.debugging_workspace; return retained.attempt;
    };
    expect((await replay(false, 3)).result).toBe("not_reproduced");
    const refined = await json(developerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/replay-scenarios`, developer.headers, { expected_version: workspace.version, scenario: { parent_scenario_id: scenario.id, title: "Refined synthetic cancelled checkout replay", objective: "Exercise the released retry guard with deterministic synthetic scheduling", evidence_citation_ids: [findingCitation.id], inputs: [{ name: "checkout shape", kind: "synthetic", schema: "cancelled checkout timing", sha256: "c".repeat(64), sanitization: "generated identifiers and deterministic timings" }], dependencies: ["alpine:3.22"], commands: [{ name: "intermittent replay", sha256: digest(replayCommand), purpose: "verify the retry guard" }], invariants: [{ name: "retry guard remains active", command_name: "intermittent replay", expected_exit_code: 0, description: "synthetic cancellation reaches the bounded guard" }], production_differences: ["deterministic synthetic scheduler"], unsafe_side_effects: [], gaps: [] } });
    workspace = refined.debugging_workspace; scenario = refined.replay_scenario;
    expect((await replay(true, 4)).result).toBe("demonstrated"); expect((await replay(true, 4)).result).toBe("demonstrated");
    expect(workspace.replay_scenarios.find((item: any) => item.id === scenario.id).status).toBe("reproduced");

    const repairResult = await json(ownerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/repair-work`, owner.headers, { expected_version: workspace.version, scenario_id: scenario.id, cause_claim_id: finding.id, title: "Repair checkout retry cancellation", acceptance_criteria: ["released checkout completes"], regression_criteria: ["synthetic replay and ordinary check pass"], assignee_type: "human", assignee_id: developer.user.id });
    workspace = repairResult.debugging_workspace; const work = repairResult.repair_work;
    await git(copy, "switch", "-c", "repair/checkout-retry"); await writeFile(join(copy, "service.txt"), "retry_guard=off\nrepair=pending\n"); await git(copy, "add", "service.txt"); await git(copy, "commit", "-m", "Attempt checkout retry repair"); await git(copy, "push", "origin", "repair/checkout-retry");
    const pull = await json(developerPage, "post", `/repositories/${repository.id}/proposals/${work.proposal_id}/tasks/${work.task_id}/contributions`, developer.headers, { title: "Repair checkout retry cancellation", body: "Runtime-informed repair with retained replay.", source_branch: "repair/checkout-retry", target_branch: "main" });
    let checks = await eventually(() => json(developerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}/checks`, developer.headers), (x: any) => x.check_runs?.length === 2 && x.check_runs.every((c: any) => ["succeeded", "failed"].includes(c.state)), "first repair checks finish");
    expect(checks.check_runs.some((c: any) => c.state === "failed")).toBe(true);
    await writeFile(join(copy, "service.txt"), "retry_guard=on\nrepair=reviewed\n"); await git(copy, "add", "service.txt"); await git(copy, "commit", "-m", "Correct checkout retry repair"); await git(copy, "push", "origin", "repair/checkout-retry");
    const correctedRevision = await git(copy, "rev-parse", "HEAD");
    await json(developerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/synchronize`, developer.headers, {});
    checks = await eventually(() => json(developerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}/checks`, developer.headers), (x: any) => x.check_runs?.filter((c: any) => c.commit_id === correctedRevision).length === 2 && x.check_runs.filter((c: any) => c.commit_id === correctedRevision).every((c: any) => c.state === "succeeded"), "corrected repair checks pass");
    await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/reviews`, owner.headers, { decision: "approved" });
    const merged = await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/merge`, owner.headers, {});
    checks = await eventually(() => json(ownerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}/checks`, owner.headers), (x: any) => x.check_runs?.filter((c: any) => c.commit_id === merged.merge_commit_id).length === 2 && x.check_runs.filter((c: any) => c.commit_id === merged.merge_commit_id).every((c: any) => c.state === "succeeded"), "integrated repair checks pass");
    const repairRelease = await json(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: "v1.0.1", notes: "Reviewed checkout retry repair", commit_id: merged.merge_commit_id });
    await json(ownerPage, "post", `/repositories/${repository.id}/dependency-inventories`, owner.headers, { commit_id: merged.merge_commit_id });
    await json(ownerPage, "post", `/repositories/${repository.id}/releases/${repairRelease.id}/builds`, owner.headers, {});
    const builds = await eventually(() => json(ownerPage, "get", `/repositories/${repository.id}/releases/${repairRelease.id}/builds`, owner.headers), (x: any) => x.builds?.some((b: any) => b.state === "succeeded"), "repair build succeeds");
    const build = builds.builds.find((b: any) => b.state === "succeeded");
    const pending = await json(ownerPage, "post", `/repositories/${repository.id}/deployments`, owner.headers, { environment_id: environment.id, release_id: repairRelease.id, build_id: build.id, artifact_id: build.artifacts[0].id });
    const promotions = await eventually(() => json(ownerPage, "get", `/repositories/${repository.id}/deployments`, owner.headers), (x: any) => x.deployments?.some((d: any) => d.id === pending.id && d.state === "succeeded"), "staged repair deployment succeeds");
    const deployment = promotions.deployments.find((d: any) => d.id === pending.id);
    const currentRuns = checks.check_runs.filter((c: any) => c.commit_id === merged.merge_commit_id && c.state === "succeeded");
    workspace = (await json(ownerPage, "post", `/repositories/${repository.id}/debugging-workspaces/${workspace.id}/repair-work/${work.id}/validation`, owner.headers, { expected_version: workspace.version, pull_request_id: pull.id, check_run_ids: currentRuns.map((c: any) => c.id), release_id: repairRelease.id, deployment_id: deployment.id, signal_names: ["checkout journey"], outcome: "validated", summary: "Staged users complete checkout and the exact replay remains passing", action: "none" })).debugging_workspace;

    await ownerPage.goto(`/repositories/${repository.id}/debugging`);
    await expect(ownerPage.getByRole("heading", { name: "Production debugging" })).toBeVisible();
    await expect(ownerPage.getByRole("heading", { name: "Released checkout intermittently stalls" })).toBeVisible();
    await expect(ownerPage.getByText("Decision: request fields exceed current consent", { exact: true })).toBeVisible();
    await expect(ownerPage.getByText("user_id removed; secret-bearing spans dropped")).toBeVisible();
    await expect(ownerPage.getByText("The payment provider causes the timeout")).toBeVisible();
    await expect(ownerPage.getByText("disputed", { exact: true })).toBeVisible();
    await expect(ownerPage.getByText("reproduced", { exact: true })).toBeVisible();
    await expect(ownerPage.getByText("validated", { exact: true })).toBeVisible();
    expect(workspace.repair_work[0]).toMatchObject({ validation_status: "validated", pull_request_id: pull.id, release_id: repairRelease.id, deployment_id: deployment.id });
  } finally {
    await rm(copy, { recursive: true, force: true });
  }
});
