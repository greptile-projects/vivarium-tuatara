import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey joins the complete public performance trail */

const run = promisify(execFile);
async function git(cwd: string, ...args: string[]) {
  return (await run("git", args, { cwd, env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } })).stdout.trim();
}
async function account(page: Page, name: string, handle: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(name);
  await page.getByLabel("Handle").fill(handle);
  await page.getByRole("button", { name: "Create account and continue" }).click();
  await expect(page.getByRole("heading", { name: "Create a repository" })).toBeVisible();
  const token = await page.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const user = await (await page.request.get("/api/user", { headers })).json() as any;
  return { headers, user };
}
async function json(page: Page, method: "get" | "post" | "put", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const text = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${text}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${text}`).toBeLessThan(300);
  return text ? JSON.parse(text) : undefined;
}
async function eventually<T>(read: () => Promise<T>, ready: (value: T) => boolean, label: string) {
  let value = await read();
  await expect.poll(async () => { value = await read(); return ready(value); }, { timeout: 60_000, message: label }).toBeTruthy();
  return value;
}

test("a team turns production slowness into a verified agent-assisted improvement", async ({ browser }) => {
  test.setTimeout(300_000);
  const dockerAvailable = await run("docker", ["info"]).then(() => true, () => false);
  test.skip(!dockerAvailable, "the performance journey requires release and rollout execution");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copies: string[] = [];
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const engineerPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, "Affected Service Owner", `perf-owner-${suffix}`);
    const engineer = await account(engineerPage, "Performance Engineer", `perf-engineer-${suffix}`);
    await ownerPage.goto("/");
    await ownerPage.getByLabel("Repository name").fill(`catalog-latency-${suffix}`);
    await ownerPage.getByRole("button", { name: "Create repository" }).click();
    const repositories = await json(ownerPage, "get", "/repositories", owner.headers) as any;
    const repository = repositories.repositories.find((item: any) => item.name === `catalog-latency-${suffix}`);
    await json(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: engineer.user.id });

    const ownerGit = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "performance baseline", scopes: ["git:read", "git:write"], expires_in: 3600 }) as any;
    const copy = await mkdtemp(join(tmpdir(), "vivarium-performance-")); copies.push(copy);
    await git(tmpdir(), "clone", `http://git:${ownerGit.token}@localhost:3000/git/${repository.id}.git`, copy);
    await git(copy, "config", "user.name", "Affected Service Owner"); await git(copy, "config", "user.email", "owner@example.test");
    await mkdir(join(copy, ".vivarium"));
    await writeFile(join(copy, ".vivarium", "release.json"), JSON.stringify({ version: 1, steps: [{ name: "package", image: "alpine:3.22", command: "cp service.txt \"$VIVARIUM_OUTPUT/service.txt\"" }] }));
    await writeFile(join(copy, ".vivarium", "deployment.json"), JSON.stringify({ version: 1, stages: [{ name: "canary", signals: [{ name: "artifact readable", command: "grep -q optimized \"$VIVARIUM_ARTIFACT\"" }] }, { name: "full", signals: [{ name: "artifact still readable", command: "grep -q optimized \"$VIVARIUM_ARTIFACT\"" }] }] }));
    await writeFile(join(copy, ".vivarium", "packages.json"), JSON.stringify({ version: 1, dependencies: [], lock: [] }));
    await writeFile(join(copy, "service.txt"), "catalog baseline\n");
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Establish catalog latency baseline"); await git(copy, "push", "origin", "main");
    const baselineRevision = await git(copy, "rev-parse", "HEAD");

    const goal = await json(engineerPage, "post", `/repositories/${repository.id}/performance-goals`, engineer.headers, { revision: {
      title: "Catalog users wait too long", summary: "Production reports show the catalog p95 above the user-impact budget.",
      subject: { kind: "user_journey", name: "Open the catalog" },
      workloads: [{ name: "catalog-100", description: "Render one hundred representative items", inputs: "sanitized-fixture-v1", warmup: 2, samples: 5 }],
      metrics: [{ name: "p95", unit: "ms", direction: "lower", maximum: 200, baseline: { value: 250, environment: "linux-amd64", measured_at: new Date().toISOString(), source: "production concern perf-2026-08" } }],
      correctness_constraints: [{ name: "complete catalog", requirement: "all one hundred items remain present", verification: "grep item-count service.txt" }],
      supported_environments: [{ name: "linux-amd64", os: "linux", architecture: "amd64", runtime: "go1.25", hardware: "shared-ci" }],
      owners: [owner.user.id], budgets: [{ kind: "agent", limit: 15, unit: "minutes" }],
      links: [{ kind: "incident", resource_id: "prod-latency-2026-08", label: "Customers report: this is too slow" }], baseline_max_age_days: 30,
      rationale: "Agree on user impact, reproducibility, and correctness before changing code.",
    } }) as any;
    const trial = (revision: string, mode: string, values: number[], cpu: number, cost: number) => ({
      goal_id: goal.id, context_kind: "incident", context_id: "prod-latency-2026-08", mode,
      source: { kind: "revision", revision }, workload: "catalog-100", inputs: "representative item fixture; no customer records",
      sanitization: ["removed account identifiers", "replaced production payloads with fixture keys"],
      environment: { name: "linux-amd64", os: "linux", architecture: "amd64", runtime: "go1.25", hardware: "shared-ci", container_image: "perf-runner@sha256:fixture" },
      sampling: { warmup: 2, samples: 5, method: "isolated wall clock" }, timings: [{ metric: "p95", unit: "ms", values }],
      resources: { cpu_seconds: cpu, peak_memory_mb: 64, read_bytes: 4096, write_bytes: 512 },
      traces: [{ kind: "profile", name: "cpu.pprof", sha256: "a".repeat(64), size: 512 }], logs: ["sanitized catalog workload complete"],
      artifacts: [{ kind: "report", name: "summary.json", sha256: "b".repeat(64), size: 128 }], cost: { amount: cost, unit: "usd" },
    });
    const baseline = await json(engineerPage, "post", `/repositories/${repository.id}/performance-trials`, engineer.headers, trial(baselineRevision, "production_capture", [248, 250, 252, 249, 251], 12, 1.2)) as any;
    expect(baseline.inputs).toBe("[sanitized production-derived workload]");
    const investigation = await json(engineerPage, "post", `/repositories/${repository.id}/performance-investigations`, engineer.headers, {
      title: "Explain catalog render latency", trial_ids: [baseline.id], invitee_ids: [owner.user.id],
      references: [{ kind: "runtime_path", id: "ignored", revision: baselineRevision, path: "service.txt", label: "Catalog hot path" }],
    }) as any;
    const access = await json(engineerPage, "post", `/repositories/${repository.id}/performance-investigations/${investigation.id}/agent-access`, engineer.headers, { expires_in: 900 }) as any;
    const agentHeaders = { Authorization: `Bearer ${access.credential.token}` };
    const agentFindingState = await json(engineerPage, "post", `/performance-investigations/${investigation.id}/findings`, agentHeaders, {
      kind: "hypothesis", body: "The selected profile attributes most time to repeated catalog materialization; cache the stable projection and preserve item completeness.",
      citation_ids: [baseline.id, `${baselineRevision}:service.txt`], confidence: "high",
      flamegraph: [{ frames: [{ name: "renderCatalog", file: "service.txt", line: 1 }, { name: "materializeItems" }], value: 78, unit: "percent" }],
    }) as any;
    const finding = agentFindingState.findings[0];
    await json(ownerPage, "post", `/repositories/${repository.id}/performance-investigations/${investigation.id}/findings/${finding.id}/confirmations`, owner.headers, { body: "I own the affected path and confirm the profile matches production symptoms; item completeness remains mandatory." });

    await git(copy, "switch", "-c", "agent/catalog-cache");
    await writeFile(join(copy, "service.txt"), "catalog optimized\nitem-count=100\nagent-assisted cache with bounded invalidation\n");
    await git(copy, "add", "service.txt"); await git(copy, "commit", "-m", "Cache stable catalog projection with agent guidance"); await git(copy, "push", "origin", "agent/catalog-cache");
    const candidateRevision = await git(copy, "rev-parse", "HEAD");
    const pull = await json(engineerPage, "post", `/repositories/${repository.id}/pulls`, engineer.headers, { title: "Reduce catalog latency", body: `Agent-assisted optimization from investigation ${investigation.id}; preserves the complete catalog constraint.`, source_branch: "agent/catalog-cache", target_branch: "main" }) as any;
    const integrationCopy = await mkdtemp(join(tmpdir(), "vivarium-performance-integration-")); copies.push(integrationCopy);
    await git(tmpdir(), "clone", `http://git:${ownerGit.token}@localhost:3000/git/${repository.id}.git`, integrationCopy);
    await git(integrationCopy, "config", "user.name", "Affected Service Owner"); await git(integrationCopy, "config", "user.email", "owner@example.test");
    await writeFile(join(integrationCopy, "integration.txt"), "Independent target-side integration context.\n");
    await git(integrationCopy, "add", "integration.txt"); await git(integrationCopy, "commit", "-m", "Advance integration context"); await git(integrationCopy, "push", "origin", "main");
    await json(ownerPage, "post", `/repositories/${repository.id}/performance-merge-policies`, owner.headers, { branch: "main", paths: ["service.txt"], risk_classes: [], goal_ids: [goal.id], maximum_regression_percent: 0, minimum_confidence: 0.95, require_correctness: true });

    const noisy = await json(engineerPage, "post", `/repositories/${repository.id}/performance-trials`, engineer.headers, trial(candidateRevision, "benchmark", [160, 340, 180, 320, 200], 10, 1.0)) as any;
    await json(engineerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/performance-evaluations`, engineer.headers, { goal_id: goal.id, investigation_id: investigation.id, baseline_trial_id: baseline.id, candidate_trial_id: noisy.id, affected_scenarios: ["Open the catalog"], commands: ["bench catalog-100 --samples 5"], correctness_checks: [{ name: "complete catalog", command: "grep -q item-count=100 service.txt", passed: true, summary: "Complete under a noisy run." }], residual_risks: ["Variance is too high to trust this attempt."] });
    let readiness = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}/merge-readiness`, owner.headers) as any;
    expect(readiness.performance_requirements).toEqual(expect.arrayContaining([expect.objectContaining({ status: "uncertain" })]));

    const stable = await json(engineerPage, "post", `/repositories/${repository.id}/performance-trials`, engineer.headers, trial(candidateRevision, "benchmark", [178, 180, 182, 179, 181], 8, 0.8)) as any;
    await json(engineerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/performance-evaluations`, engineer.headers, { goal_id: goal.id, investigation_id: investigation.id, baseline_trial_id: baseline.id, candidate_trial_id: stable.id, affected_scenarios: ["Open the catalog"], commands: ["bench catalog-100 --samples 5"], correctness_checks: [{ name: "complete catalog", command: "grep -q item-count=100 service.txt", passed: false, summary: "A fixture assertion was initially misconfigured and failed closed." }], residual_risks: ["Correctness must be rerun before review."] });
    readiness = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}/merge-readiness`, owner.headers) as any;
    expect(readiness.performance_requirements).toEqual(expect.arrayContaining([expect.objectContaining({ status: "failed", message: "correctness assumptions failed" })]));
    const evaluation = await json(engineerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/performance-evaluations`, engineer.headers, { goal_id: goal.id, investigation_id: investigation.id, baseline_trial_id: baseline.id, candidate_trial_id: stable.id, affected_scenarios: ["Open the catalog"], commands: ["bench catalog-100 --samples 5", "grep -q item-count=100 service.txt"], correctness_checks: [{ name: "complete catalog", command: "grep -q item-count=100 service.txt", passed: true, summary: "All one hundred items are retained." }], residual_risks: ["Observe cache invalidation during staged rollout."] }) as any;
    expect(evaluation.comparisons[0].change_percent).toBeLessThan(-20);

    await ownerPage.goto(`/pulls/${repository.id}/${pull.id}`);
    await expect(ownerPage.getByRole("heading", { name: "Performance evaluations" })).toBeVisible();
    await expect(ownerPage.getByText("All one hundred items are retained.")).toBeVisible();
    await ownerPage.getByRole("button", { name: "Approve" }).click();
    const merged = await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/merge`, owner.headers, {}) as any;
    expect(merged.merge_commit_id).toBeTruthy();
    const integratedRevision = merged.merge_commit_id as string;
    expect(integratedRevision).not.toBe(candidateRevision);
    const release = await json(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: "v1.1.0", notes: "Agent-assisted, exact-revision catalog latency improvement.", commit_id: integratedRevision }) as any;
    await json(ownerPage, "post", `/repositories/${repository.id}/dependency-inventories`, owner.headers, { commit_id: integratedRevision });
    await json(ownerPage, "post", `/repositories/${repository.id}/releases/${release.id}/builds`, owner.headers, {});
    const builds = await eventually(() => json(ownerPage, "get", `/repositories/${repository.id}/releases/${release.id}/builds`, owner.headers), (value: any) => value.builds?.some((item: any) => item.state === "succeeded"), "release build succeeds") as any;
    const build = builds.builds.find((item: any) => item.state === "succeeded");
    const staging = await json(ownerPage, "post", `/repositories/${repository.id}/environments`, owner.headers, { name: "staging", position: 1, image: "alpine:3.22", command: "test -r \"$VIVARIUM_ARTIFACT\"", timeout_seconds: 30, configuration: {}, credentials: {}, required_approvals: 0, concurrency: 1 }) as any;
    const production = await json(ownerPage, "post", `/repositories/${repository.id}/environments`, owner.headers, { name: "production", position: 2, image: "alpine:3.22", command: "test -r \"$VIVARIUM_ARTIFACT\"", timeout_seconds: 30, configuration: {}, credentials: {}, required_approvals: 0, concurrency: 1 }) as any;
    const promote = async (environment: any) => {
      const pending = await json(ownerPage, "post", `/repositories/${repository.id}/deployments`, owner.headers, { environment_id: environment.id, release_id: release.id, build_id: build.id, artifact_id: build.artifacts[0].id }) as any;
      const values = await eventually(() => json(ownerPage, "get", `/repositories/${repository.id}/deployments`, owner.headers), (value: any) => value.deployments?.some((item: any) => item.id === pending.id && item.state === "succeeded"), `${environment.name} promotion succeeds`) as any;
      return values.deployments.find((item: any) => item.id === pending.id);
    };
    await promote(staging);
    const deployment = await promote(production);

    const missed = await json(ownerPage, "post", `/repositories/${repository.id}/performance-trials`, owner.headers, trial(integratedRevision, "production_capture", [205, 207, 206, 208, 204], 9, 0.9)) as any;
    const contained = await json(ownerPage, "post", `/repositories/${repository.id}/performance-release-observations`, owner.headers, { release_id: release.id, deployment_id: deployment.id, goal_id: goal.id, candidate_evaluation_id: evaluation.id, observed_trial_id: missed.id, commit_id: integratedRevision, assumptions: ["Production traffic represents the declared catalog workload."] }) as any;
    expect(contained).toMatchObject({ state: "regressed", recommended_actions: ["pause_rollout", "restore_known_good", "open_repair", "revisit_decision"] });
    const recovered = await json(ownerPage, "post", `/repositories/${repository.id}/performance-trials`, owner.headers, trial(integratedRevision, "production_capture", [174, 176, 175, 173, 177], 7.8, 0.78)) as any;
    const outcome = await json(ownerPage, "post", `/repositories/${repository.id}/performance-release-observations`, owner.headers, { release_id: release.id, deployment_id: deployment.id, goal_id: goal.id, candidate_evaluation_id: evaluation.id, observed_trial_id: recovered.id, commit_id: integratedRevision, assumptions: ["The warmed production cohort now matches the declared catalog workload."] }) as any;
    expect(outcome).toMatchObject({ state: "passed", recommended_actions: [] });

    await ownerPage.goto(`/repositories/${repository.id}/performance`);
    await expect(ownerPage.getByRole("heading", { name: "Catalog users wait too long" })).toBeVisible();
    await expect(ownerPage.getByRole("heading", { name: "Explain catalog render latency" })).toBeVisible();
    await expect(ownerPage.getByText(/agent:/)).toBeVisible();
    const retained = await json(ownerPage, "get", `/repositories/${repository.id}/performance-trials`, owner.headers) as any;
    expect(retained.trials).toEqual(expect.arrayContaining([expect.objectContaining({ id: baseline.id }), expect.objectContaining({ id: noisy.id }), expect.objectContaining({ id: stable.id }), expect.objectContaining({ id: missed.id }), expect.objectContaining({ id: recovered.id })]));
  } finally {
    await Promise.all(copies.map((path) => rm(path, { recursive: true, force: true })));
  }
});
