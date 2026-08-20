import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey checks a cross-resource public projection */

const run = promisify(execFile);
async function git(cwd: string, ...args: string[]) {
  return (await run("git", args, { cwd, env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } })).stdout.trim();
}
async function account(page: Page, suffix: string, name: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(name);
  await page.getByLabel("Handle").fill(`retirement-${name.toLowerCase()}-${suffix}`);
  await page.getByRole("button", { name: "Create account and continue" }).click();
  await expect(page.getByRole("heading", { name: "Create a repository" })).toBeVisible();
  const token = await page.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const user = await (await page.request.get("/api/user", { headers })).json();
  return { headers, user };
}
async function json(page: Page, method: "get" | "post", path: string, headers: Record<string, string>, data?: unknown) {
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

test("collaborators retire a released capability through governed cleanup", async ({ browser }) => {
  test.setTimeout(240_000);
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copies: string[] = [];
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const consumerPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, suffix, "Provider");
    const consumer = await account(consumerPage, suffix, "Consumer");
    const providerRepo = await json(ownerPage, "post", "/repositories", owner.headers, { name: `legacy-provider-${suffix}` });
    const consumerRepo = await json(consumerPage, "post", "/repositories", consumer.headers, { name: `independent-consumer-${suffix}` });
    await json(ownerPage, "post", `/repositories/${providerRepo.id}/collaborators`, owner.headers, { user_id: consumer.user.id });
    await json(consumerPage, "post", `/repositories/${consumerRepo.id}/collaborators`, consumer.headers, { user_id: owner.user.id });
    const credential = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "capability retirement journey", scopes: ["git:read", "git:write"], expires_in: 3600 });
    const consumerCredential = await json(consumerPage, "post", "/auth/credentials", consumer.headers, { kind: "git", name: "consumer retirement journey", scopes: ["git:read", "git:write"], expires_in: 3600 });
    const providerCopy = await mkdtemp(join(tmpdir(), "vivarium-capability-provider-")); copies.push(providerCopy);
    const consumerCopy = await mkdtemp(join(tmpdir(), "vivarium-capability-consumer-")); copies.push(consumerCopy);
    await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${providerRepo.id}.git`, providerCopy);
    await git(tmpdir(), "clone", `http://git:${consumerCredential.token}@localhost:3000/git/${consumerRepo.id}.git`, consumerCopy);
    for (const copy of copies) { await git(copy, "config", "user.name", "Capability Journey"); await git(copy, "config", "user.email", "capability@example.test"); }

    const surfaces = [
      ["code", "legacy/code.txt"], ["flags", "legacy/flag.txt"], ["data", "legacy/schema.sql"],
      ["credentials", "legacy/credential-policy.txt"], ["telemetry", "legacy/metric.txt"],
      ["documentation", "legacy/README.md"], ["policy_exceptions", "legacy/exception.txt"],
    ];
    await mkdir(join(providerCopy, "legacy"));
    for (const [, path] of surfaces) await writeFile(join(providerCopy, path), `obsolete ${path}\n`);
    await git(providerCopy, "add", "."); await git(providerCopy, "commit", "-m", "Release legacy search v1"); await git(providerCopy, "push", "origin", "main");
    const providerRevision = await git(providerCopy, "rev-parse", "HEAD");
    await mkdir(join(consumerCopy, ".vivarium"));
    await writeFile(join(consumerCopy, ".vivarium", "workspace.json"), JSON.stringify({ version: 1, image: "alpine:3.22", tools: [{ name: "sh", version: "3.22" }], dependencies: ["sh"], setup: [], experiments: [], resources: { cpus: 1, memory_mb: 128, storage_mb: 128, setup_seconds: 30 } }));
    await writeFile(join(consumerCopy, "consumer.txt"), "legacy and replacement coexist\n");
    await git(consumerCopy, "add", "."); await git(consumerCopy, "commit", "-m", "Declare legacy capability use"); await git(consumerCopy, "push", "origin", "main");
    const consumerRevision = await git(consumerCopy, "rev-parse", "HEAD");

    const release = await json(ownerPage, "post", `/repositories/${providerRepo.id}/releases`, owner.headers, { version: "v1.0.0", notes: "Legacy search release", commit_id: providerRevision });
    const observed = new Date().toISOString();
    let capability = await json(ownerPage, "post", `/repositories/${providerRepo.id}/capabilities`, owner.headers, { revision: {
      name: "legacy search", summary: "The released v1 search contract and every obsolete surface", commit_id: providerRevision, release_id: release.id,
      owner_ids: [owner.user.id], items: surfaces.map(([kind, path]) => ({ kind: kind === "code" ? "symbol" : kind === "flags" ? "flag" : kind === "data" ? "schema" : kind === "credentials" ? "configuration" : kind === "telemetry" ? "journey" : kind === "documentation" ? "documentation" : "interface", name: path, path, revision: providerRevision })),
      consumers: [{ name: "independent checkout", repository_id: consumerRepo.id, owner_ids: [consumer.user.id], environment: "production", revision: consumerRevision, discovery: "declared", evidence_state: "current", evidence_reference: "aggregate search calls", last_observed_at: observed, compatibility_promise: "migration governed by this retirement contract" }], unknown_use: false,
    } });
    capability = capability.capability ?? capability;
    const now = Date.now();
    capability = await json(ownerPage, "post", `/repositories/${providerRepo.id}/capabilities/${capability.id}/retirement-plans`, owner.headers, {
      rationale: "Replace legacy search without breaking independently owned consumers", replacements: [{ name: "search v2", reference: "capability:search-v2", migration_guide: "docs/search-v2.md", supported: true }],
      audiences: [{ name: "independent checkout", owner_ids: [consumer.user.id], impact: "v1 requests stop after the observed zero-use window" }],
      stages: [{ name: "disable", starts_at: new Date(now + 60_000).toISOString(), behavior: "disable old writes while rollback remains available", exit_criteria: ["zero old use"] }, { name: "remove", starts_at: new Date(now + 120_000).toISOString(), behavior: "remove obsolete surfaces", exit_criteria: ["cleanup proof complete"] }],
      deadline: new Date(now + 3_600_000).toISOString(), approval_due_at: new Date(now + 600_000).toISOString(), success_criteria: ["v2 journey passes", "old use remains zero"], rollback_criteria: ["consumer regression", "unexpected dependent"], communication: { channels: ["repository inbox"], notice_days: 1, updates: "after every stage", escalation: "pause and contact the affected owner" }, required_owner_ids: [consumer.user.id],
    });
    let plan = capability.retirement_plans[0];
    expect(plan.blockers).toEqual(expect.arrayContaining([expect.objectContaining({ kind: "owner_approval_required" })]));
    capability = await json(ownerPage, "post", `/repositories/${providerRepo.id}/capabilities/${capability.id}/retirement-plans/${plan.id}/events`, owner.headers, { expected_version: 0, event: { type: "assessment", summary: "Provider mapped code, schema, configuration, documentation, collection, and rollback boundaries.", evidence: [providerRevision, release.id] } });
    plan = capability.retirement_plans[0];
    capability = await json(consumerPage, "post", `/repositories/${providerRepo.id}/capabilities/${capability.id}/retirement-plans/${plan.id}/events`, consumer.headers, { expected_version: 1, event: { type: "approval", summary: "Independent owner accepts v2 and the staged disable window.", owner_id: consumer.user.id, decision: "approved" } });
    plan = capability.retirement_plans[0];

    const workBase = `/repositories/${providerRepo.id}/capabilities/${capability.id}/retirement-plans/${plan.id}/work`;
    const humanWork = await json(consumerPage, "post", workBase, consumer.headers, { expected_version: 0, repository_id: consumerRepo.id, title: "Migrate checkout to search v2", completion_criteria: "Checkout no longer calls v1.", assignee_type: "human", assignee_id: consumer.user.id, mandate: "Change only the consumer-owned checkout integration.", base_revision: consumerRevision, work: { audience_index: 0, dependency_ids: [], old_contract: "search v1 request", replacement_contract: "search v2 request", acceptance_criteria: ["dual mode passes"], documentation_changes: ["update checkout runbook"], rollout_stage: "dual support" } });
    const agentWork = await json(consumerPage, "post", workBase, consumer.headers, { expected_version: 1, repository_id: consumerRepo.id, title: "Remove the bounded v1 adapter", completion_criteria: "The old adapter and collection hook are absent.", assignee_type: "agent", assignee_id: "", mandate: "Edit only the assigned adapter after human migration merges.", base_revision: consumerRevision, work: { audience_index: 0, dependency_ids: [humanWork.retirement_work.id], old_contract: "search v1 adapter", replacement_contract: "search v2 direct client", acceptance_criteria: ["replacement-only journey passes"], documentation_changes: ["remove v1 examples"], rollout_stage: "replacement only" } });
    expect(agentWork.task.assignment.assignee_type).toBe("agent");

    plan = agentWork.capability.retirement_plans[0];
    const commands = ["test -f consumer.txt", "grep -q coexist consumer.txt", "grep -q migrated consumer.txt", "test ! -f missing-rollback-marker", "grep -q 'legacy and replacement' consumer.txt"];
    const checks = ["old_only", "dual_support", "replacement", "rollback", "journey"].map((stage, index) => ({ id: stage, stage, journey: stage === "journey" ? "checkout-search" : undefined, repository_id: consumerRepo.id, revision: consumerRevision, command: commands[index], paths: ["consumer.txt"], expectation: `${stage} behavior is safe` }));
    const cleanup = surfaces.map(([kind, path], index) => ({ id: `cleanup-${index}`, kind, repository_id: providerRepo.id, path, revision: providerRevision, previous_blob: "server-derived", expectation: "removed" }));
    const created = await json(ownerPage, "post", `/repositories/${providerRepo.id}/capabilities/${capability.id}/retirement-plans/${plan.id}/candidates`, owner.headers, { environment: "bounded coexistence", checks, cleanup_requirements: cleanup });
    const candidate = created.candidate;
    const workspace = await json(consumerPage, "post", "/workspaces", consumer.headers, { repository_id: consumerRepo.id, commit_id: consumerRevision, source: { kind: "repository" } });
    await expect.poll(async () => (await json(consumerPage, "get", `/workspaces/${workspace.id}`, consumer.headers)).state, { timeout: 60_000 }).toBe("running");
    const evidenceBase = `/repositories/${providerRepo.id}/capabilities/${capability.id}/retirement-plans/${plan.id}/candidates/${candidate.id}/evidence`;
    await json(consumerPage, "post", `/workspaces/${workspace.id}/commands`, consumer.headers, { command: commands[2], timeout_seconds: 30 });
    const failed = await json(consumerPage, "post", evidenceBase, consumer.headers, { check_id: "replacement", workspace_id: workspace.id });
    expect(failed.retirement_plans[0].candidates[0].checks.find((x: any) => x.id === "replacement").status).toBe("failed");
    const replacement = await consumerPage.request.put(`/api/workspaces/${workspace.id}/file`, { headers: consumer.headers, data: { path: "consumer.txt", content: "legacy and replacement coexist migrated\n", expected_sha256: createHash("sha256").update("legacy and replacement coexist\n").digest("hex") } });
    expect(replacement.status(), await replacement.text()).toBe(200);
    for (const check of checks) {
      await json(consumerPage, "post", `/workspaces/${workspace.id}/commands`, consumer.headers, { command: check.command, timeout_seconds: 30 });
      await json(consumerPage, "post", evidenceBase, consumer.headers, { check_id: check.id, workspace_id: workspace.id });
    }
    const usageBase = `/repositories/${providerRepo.id}/capabilities/${capability.id}/retirement-plans/${plan.id}/candidates/${candidate.id}/usage-observations`;
    const window = { consumer_index: 0, state: "measured", window_starts_at: new Date(now - 60_000).toISOString(), window_ends_at: new Date(now).toISOString(), artifact_digest: "sha256:aggregate-window", summary: "Bounded aggregate production window" };
    let usage = await json(consumerPage, "post", usageBase, consumer.headers, { ...window, old_behavior_uses: 3, total_uses: 100 });
    expect(usage.retirement_plans[0].candidates[0].removal_ready).toBe(false);
    usage = await json(consumerPage, "post", usageBase, consumer.headers, { ...window, old_behavior_uses: 0, total_uses: 120, summary: "Owner confirms zero v1 calls after migration" });
    expect(usage.retirement_plans[0].candidates[0].removal_ready).toBe(true);

    const executionBase = `/repositories/${providerRepo.id}/capabilities/${capability.id}/retirement-plans/${plan.id}`;
    let opened = await json(ownerPage, "post", `${executionBase}/candidates/${candidate.id}/removal-executions`, owner.headers); let execution = opened.execution;
    const report = async (extra: Record<string, unknown>) => { const out = await json(ownerPage, "post", `${executionBase}/removal-executions/${execution.id}/reports`, owner.headers, { expected_version: execution.version, report: extra }); execution = out.retirement_plans[0].executions.find((x: any) => x.id === execution.id); return out; };
    const delivery = [{ kind: "deployment", resource_id: "staged-disable", revision: providerRevision, status: "succeeded" }];
    await report({ stage_index: 0, stage: "disable", action: "advance", remaining_use: 1, health: "degraded", control: "disable flag", rollback_boundary: "compatibility restore remains available", next_action: "restore v1", unexpected_consumers: ["hidden batch exporter"], delivery });
    await report({ stage_index: 0, stage: "disable", action: "restore", remaining_use: 0, health: "healthy", control: "v1 compatibility restored", rollback_boundary: "before destructive removal", next_action: "inventory hidden exporter before retry", compatibility_restored: true, delivery });
    expect(execution.status).toBe("restored");
    opened = await json(ownerPage, "post", `${executionBase}/candidates/${candidate.id}/removal-executions`, owner.headers); execution = opened.execution;
    await report({ stage_index: 0, stage: "disable", action: "advance", remaining_use: 0, health: "healthy", control: "v1 disabled after exporter correction", rollback_boundary: "compatibility package retained", next_action: "observe replacement journey", delivery });
    await report({ stage_index: 1, stage: "remove", action: "advance", remaining_use: 0, health: "degraded", control: "removal paused", rollback_boundary: "code not yet deleted", next_action: "correct late checkout regression", unexpected_consumers: ["late checkout regression"], delivery });
    await report({ stage_index: 1, stage: "remove", action: "resume", remaining_use: 0, health: "healthy", control: "late regression corrected", rollback_boundary: "final cleanup pending", next_action: "deliver cleanup", delivery });
    for (const [, path] of surfaces) await rm(join(providerCopy, path));
    await git(providerCopy, "add", "-A"); await git(providerCopy, "commit", "-m", "Remove legacy search surfaces"); await git(providerCopy, "push", "origin", "main");
    const cleanupRevision = await git(providerCopy, "rev-parse", "HEAD");
    await report({ stage_index: 1, stage: "remove", action: "advance", remaining_use: 0, health: "healthy", control: "obsolete behavior removed", rollback_boundary: "point of no return recorded", next_action: "verify exact cleanup", delivery: [{ kind: "merge_queue", resource_id: "legacy-cleanup", revision: cleanupRevision, status: "succeeded" }] });
    expect(execution.status).toBe("awaiting_verification");
    const proofs = surfaces.map(([kind, path], index) => { const id = `cleanup-${index}`; const digest = createHash("sha256").update(id).update("\0").update(path).update("\0").update("absent").update("\0").digest("hex"); return { kind, requirement_ids: [id], repository_id: providerRepo.id, revision: cleanupRevision, paths: [path], digest, summary: `${path} is absent at the delivered revision` }; });
    const completed = await json(ownerPage, "post", `${executionBase}/removal-executions/${execution.id}/completion`, owner.headers, { expected_version: execution.version, proofs });
    expect(completed.retirement_plans[0].executions.at(-1).status).toBe("completed");

    await writeFile(join(consumerCopy, "consumer.txt"), "replacement only after retirement\n"); await git(consumerCopy, "add", "."); await git(consumerCopy, "commit", "-m", "Move consumer after retained measurement"); await git(consumerCopy, "push", "origin", "main");
    const movedConsumer = await git(consumerCopy, "rev-parse", "HEAD");
    await json(ownerPage, "post", `/repositories/${providerRepo.id}/capabilities/${capability.id}/revisions`, owner.headers, { expected_version: 1, revision: {
      name: "legacy search", summary: "Post-retirement inventory retains the moved consumer signal", commit_id: providerRevision, release_id: release.id, owner_ids: [owner.user.id],
      items: surfaces.map(([kind, path]) => ({ kind: kind === "code" ? "symbol" : kind === "flags" ? "flag" : kind === "data" ? "schema" : kind === "credentials" ? "configuration" : kind === "telemetry" ? "journey" : kind === "documentation" ? "documentation" : "interface", name: path, path, revision: providerRevision })),
      consumers: [{ name: "independent checkout", repository_id: consumerRepo.id, owner_ids: [consumer.user.id], environment: "production", revision: movedConsumer, discovery: "declared", evidence_state: "current", evidence_reference: "post-retirement aggregate search calls", last_observed_at: new Date().toISOString(), compatibility_promise: "migration governed by this retirement contract" }], unknown_use: false,
    } });
    const stale = await json(ownerPage, "get", `/repositories/${providerRepo.id}/capabilities/${capability.id}`, owner.headers);
    expect(stale.retirement_plans[0].candidates[0].blockers).toEqual(expect.arrayContaining([expect.objectContaining({ kind: "usage_revision_stale" })]));

    await ownerPage.goto(`/repositories/${providerRepo.id}/capabilities`);
    await expect(ownerPage.getByRole("heading", { name: "Know the footprint before removal" })).toBeVisible();
    await expect(ownerPage.getByText("Decision and evidence trail")).toBeVisible();
    await expect(ownerPage.getByText("Provider mapped code, schema, configuration, documentation, collection, and rollback boundaries.")).toBeVisible();
    await expect(ownerPage.getByText("hidden batch exporter")).toBeVisible();
    await expect(ownerPage.getByText("Unexpected consumers: late checkout regression", { exact: true })).toBeVisible();
    await expect(ownerPage.getByText("Verified product removal")).toBeVisible();
    await expect(ownerPage.getByText(/usage revision stale/i)).toBeVisible();
    await expect(ownerPage.getByText("legacy/metric.txt is absent at the delivered revision")).toBeVisible();
    await rejected(await consumerPage.request.post(`/api${executionBase}/candidates/${candidate.id}/removal-executions`, { headers: consumer.headers }), 403, "removal_execution_forbidden");
  } finally {
    await Promise.all(copies.map((copy) => rm(copy, { recursive: true, force: true })));
  }
});
