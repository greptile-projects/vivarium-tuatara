import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey verifies one cross-resource public trail */

const run = promisify(execFile);
const digest = (value: string) => createHash("sha256").update(value).digest("hex");

async function git(cwd: string, ...args: string[]) {
  return (await run("git", args, { cwd, env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } })).stdout.trim();
}

async function account(page: Page, suffix: string, role: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(`${role} owner`);
  await page.getByLabel("Handle").fill(`infra-${role.toLowerCase()}-${suffix}`);
  await page.getByRole("button", { name: "Create account and continue" }).click();
  await expect(page.getByRole("heading", { name: "Create a repository" })).toBeVisible();
  const token = await page.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const user = await (await page.request.get("/api/user", { headers })).json() as { id: string };
  return { headers, user };
}

async function request(page: Page, method: "get" | "post" | "put", path: string, headers: Record<string, string>, data?: unknown) {
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

test("humans and a scoped agent evolve infrastructure from proposal through reconciled drift", async ({ browser }) => {
  test.setTimeout(300_000);
  const dockerAvailable = await run("docker", ["info"]).then(() => true, () => false);
  test.skip(!dockerAvailable, "the infrastructure journey requires bounded Docker workspaces");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copy = await mkdtemp(join(tmpdir(), "vivarium-infrastructure-"));
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const securityPage = await (await browser.newContext()).newPage();
    const servicePage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, suffix, "Platform");
    const security = await account(securityPage, suffix, "Security");
    const service = await account(servicePage, suffix, "Service");
    const repository = await request(ownerPage, "post", "/repositories", owner.headers, { name: `runtime-${suffix}` }) as { id: string };
    for (const collaborator of [security.user, service.user]) await request(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: collaborator.id });
    const environment = await request(ownerPage, "post", `/repositories/${repository.id}/environments`, owner.headers, { name: "production", position: 1, image: "alpine:3.22", command: "true", timeout_seconds: 30, configuration: {}, credentials: {}, required_approvals: 0, concurrency: 1 }) as any;
    const credential = await request(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "infrastructure journey", scopes: ["git:read", "git:write"], expires_in: 3600 }) as { id: string; token: string };
    await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${repository.id}.git`, copy);
    await git(copy, "config", "user.name", "Infrastructure Engineer"); await git(copy, "config", "user.email", "infra@example.test");
    await mkdir(join(copy, ".vivarium"));
    await writeFile(join(copy, ".vivarium", "workspace.json"), JSON.stringify({ version: 1, image: "alpine:3.22", tools: [{ name: "sh", version: "3.22" }], dependencies: ["sh"], setup: ["test -f app.txt"], resources: { cpus: 1, memory_mb: 256, storage_mb: 128, setup_seconds: 30 } }));
    await writeFile(join(copy, "app.txt"), "service=v1\n");
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Establish live service"); await git(copy, "push", "origin", "main");
    const baseline = await git(copy, "rev-parse", "HEAD");

    const commitments = { security: ["private network access only"], privacy: ["regional customer metadata"], reliability: ["99.9 percent availability"], continuity: ["restore within thirty minutes"], regions: ["eu-west"] };
    const resource = (id: string, kind: string, name: string, ownerIDs: string[], provider: string, dependsOn: string[] = []) => ({ id, kind, name, description: `${name} for the released service`, owner_ids: ownerIDs, provider, provider_ref: `${provider}-${id}`, provider_access: "participant", environment_id: environment.id, depends_on: dependsOn, configuration: [{ name: "runtime policy", source: "file", sensitivity: "internal", required: true }], constraints: [{ kind: "cost", limit: 40, unit: "credits", note: "monthly reviewed ceiling" }, { kind: "capacity", limit: 100, unit: "requests", note: "bounded test capacity" }], commitments });
    const baselineRevision = { title: "Production service infrastructure", summary: "Reviewed live resources", revision: baseline, owner_ids: [security.user.id, service.user.id], rationale: "Make operational intent project state.", resources: [resource("database", "data_store", "Primary database", [security.user.id], "provider-a"), resource("service", "service", "Ledger API", [service.user.id], "provider-a", ["database"])] };
    const definition = await request(ownerPage, "post", `/repositories/${repository.id}/infrastructure`, owner.headers, { revision: baselineRevision }) as any;

    const proposal = await request(ownerPage, "post", `/repositories/${repository.id}/proposals`, owner.headers, { title: "Analyze the infrastructure rollout", body: "A scoped agent investigates exact plan risk and may execute only the cache step." }) as any;
    const task = await request(ownerPage, "post", `/repositories/${repository.id}/proposals/${proposal.id}/tasks`, owner.headers, { title: "Bound the infrastructure analysis", outcome: "Retain exact security, cost, teardown, and recovery evidence." }) as any;
    const assignment = await request(ownerPage, "put", `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/assignment`, owner.headers, { assignee_type: "agent", assignee_id: "", mandate: "Analyze the exact plan and report only the delegated non-destructive cache step.", repository_id: repository.id, base_revision: baseline, expected_assignment_id: "" }) as any;
    const launched = await request(ownerPage, "post", `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/sessions`, owner.headers, { expected_assignment_id: assignment.assignment.id, context_paths: ["app.txt"], expires_in: 3600 }) as any;
    const agentHeaders = { Authorization: `Bearer ${launched.credential.token}` };

    await git(copy, "switch", "-c", "infra-v2");
    await writeFile(join(copy, "app.txt"), "service=v2 cache=enabled\n");
    const candidate = { title: "Harden and scale production", summary: "Application and infrastructure move together", revision: "", owner_ids: [security.user.id, service.user.id], rationale: "Add bounded caching while replacing the encrypted database provider.", resources: [resource("database", "data_store", "Primary database", [security.user.id], "provider-b"), resource("cache", "compute", "Bounded response cache", [service.user.id], "provider-a", ["database"]), resource("service", "service", "Ledger API", [service.user.id], "provider-a", ["database", "cache"])] };
    await writeFile(join(copy, "policy.txt"), "production changes require exact owner acknowledgement and passing isolation\n");
    await writeFile(join(copy, "infra.json"), JSON.stringify(candidate)); await git(copy, "add", "."); await git(copy, "commit", "-m", "Propose application and infrastructure v2"); await git(copy, "push", "origin", "infra-v2");
    const pull = await request(servicePage, "post", `/repositories/${repository.id}/pulls`, service.headers, { title: "Ship service v2 with reviewed infrastructure", body: "Application work and operational intent share one exact review.", source_branch: "infra-v2", target_branch: "main" }) as any;
    const frozenRevision = pull.source_commit_id as string;
    const candidateBody = await run("git", ["show", `${frozenRevision}:infra.json`], { cwd: copy }).then(x => x.stdout);
    const policyBody = await run("git", ["show", `${frozenRevision}:policy.txt`], { cwd: copy }).then(x => x.stdout);
    const planInput = { definition_id: definition.id, source_revision: frozenRevision, candidate_source: { path: "infra.json", digest: digest(candidateBody) }, policy_effects: [{ path: "policy.txt", digest: digest(policyBody), effects: ["security and service owners approve", "production budget remains bounded"] }] };
    const plan = await request(servicePage, "post", `/repositories/${repository.id}/pulls/${pull.id}/infrastructure-plans`, service.headers, planInput) as any;
    expect(plan.changes).toEqual(expect.arrayContaining([expect.objectContaining({ resource_id: "database", action: "replace" })]));
    await request(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/infrastructure-plans/${plan.id}/events`, service.headers, { expected_events: 0, event: { kind: "impact", body: `Scoped agent ${launched.run.agent_id} analysis: replacement is destructive; cache execution is independently bounded.`, resource_ids: ["database", "cache"] } });
    await rejected(await servicePage.request.post(`/api/repositories/${repository.id}/pulls/${pull.id}/infrastructure-plans/${plan.id}/events`, { headers: service.headers, data: { expected_events: 1, event: { kind: "owner_acknowledgement", body: "Attempt to approve security ownership.", resource_ids: ["database"], owner_id: security.user.id } } }), 400, "invalid_infrastructure_plan");
    await request(ownerPage, "post", `/repositories/${repository.id}/infrastructure/${definition.id}/observations`, owner.headers, { definition_version: 1, resource_id: "service", provider_resource: "provider-a-service", observed_revision: "provider-rev-2", environment_id: environment.id, status: "healthy", summary: "Service remains healthy before rollout", visibility: "participant", managed: true, observed_at: new Date().toISOString() });
    await rejected(await servicePage.request.post(`/api/repositories/${repository.id}/pulls/${pull.id}/infrastructure-plans/${plan.id}/events`, { headers: service.headers, data: { expected_events: 1, event: { kind: "assumption", body: "Reuse stale evidence.", resource_ids: ["service"] } } }), 409, "infrastructure_plan_stale");
    const currentPlan = await request(servicePage, "post", `/repositories/${repository.id}/pulls/${pull.id}/infrastructure-plans`, service.headers, planInput) as any;
    let eventCount = 0;
    for (const [page, actor, resourceID] of [[securityPage, security, "database"], [servicePage, service, "cache"]] as const) {
      await request(page, "post", `/repositories/${repository.id}/pulls/${pull.id}/infrastructure-plans/${currentPlan.id}/events`, actor.headers, { expected_events: eventCount++, event: { kind: "owner_acknowledgement", body: `Owner accepts the exact ${resourceID} risk and rollback boundary.`, resource_ids: [resourceID], owner_id: actor.user.id } });
    }

    const checks = ["provisioning", "connectivity", "access", "policy", "service_journey", "failure", "cost", "teardown", "recovery"].map(kind => ({ id: kind, kind, command: kind === "teardown" ? "grep -q teardown-ready app.txt" : `echo ${kind}-verified`, resource_ids: ["database", "cache", "service"], expectation: `${kind} passes in synthetic isolation` }));
    const rehearsalResult = await request(servicePage, "post", `/repositories/${repository.id}/pulls/${pull.id}/infrastructure-plans/${currentPlan.id}/rehearsals`, service.headers, { name: "Production-shaped isolated rehearsal", scope: { environment_kind: "policy_approved_ephemeral", environment_id: "ephemeral-production", policy_approval: "protected-production-v1", credential_resource_ids: ["database", "cache", "service"], credential_expires_at: new Date(Date.now() + 3_600_000).toISOString(), state_kind: "synthetic", state_description: "Generated request and recovery state; no production data" }, checks, unsupported_effects: [{ resource_id: "database", effect: "replace authoritative database", reason: "Isolation cannot prove destructive provider deletion or data movement." }] }) as any;
    const rehearsal = rehearsalResult.rehearsal;
    const workspace = await request(servicePage, "post", "/workspaces", service.headers, { repository_id: repository.id, commit_id: frozenRevision, source: { kind: "repository" } }) as any;
    for (const check of checks) await request(servicePage, "post", `/workspaces/${workspace.id}/commands`, service.headers, { command: check.command, timeout_seconds: 30 });
    const failedRun = await request(servicePage, "post", `/repositories/${repository.id}/pulls/${pull.id}/infrastructure-plans/${currentPlan.id}/rehearsals/${rehearsal.id}/runs`, service.headers, { workspace_id: workspace.id, check_ids: checks.map(x => x.id) }) as any;
    expect(failedRun.run).toMatchObject({ result: "failed", outcomes: expect.arrayContaining([expect.objectContaining({ check_id: "teardown", status: "failed" })]) });
    await request(servicePage, "put", `/workspaces/${workspace.id}/file`, service.headers, { path: "app.txt", content: "service=v2 cache=enabled teardown-ready\n", expected_sha256: digest("service=v2 cache=enabled\n") });
    await request(servicePage, "post", `/workspaces/${workspace.id}/commands`, service.headers, { command: "grep -q teardown-ready app.txt", timeout_seconds: 30 });
    const passingRun = await request(servicePage, "post", `/repositories/${repository.id}/pulls/${pull.id}/infrastructure-plans/${currentPlan.id}/rehearsals/${rehearsal.id}/runs`, service.headers, { workspace_id: workspace.id, check_ids: checks.map(x => x.id) }) as any;
    expect(passingRun.run.result).toBe("passed");

    await request(securityPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/reviews`, security.headers, { decision: "approved" });
    const merged = await request(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/merge`, owner.headers, {}) as any;
    const expires = new Date(Date.now() + 3_600_000).toISOString();
    const executionInput = { plan_id: currentPlan.id, environment_id: environment.id, environment_policy: "protected-production-v1", rehearsal_id: rehearsal.id, budget_units: 80, credential_expires_at: expires, delegations: [{ step_id: "step-cache", agent_id: launched.run.agent_id, mandate: "Report only the non-destructive cache step from sanitized provider evidence." }] };
    await rejected(await ownerPage.request.post(`/api/repositories/${repository.id}/infrastructure-executions`, { headers: owner.headers, data: { ...executionInput, budget_units: 121 } }), 409, "infrastructure_budget_exceeded");
    await rejected(await ownerPage.request.post(`/api/repositories/${repository.id}/infrastructure-executions`, { headers: owner.headers, data: { ...executionInput, delegations: [{ step_id: "step-database", agent_id: launched.run.agent_id, mandate: "Replace the database." }] } }), 400, "invalid_infrastructure_execution");
    let execution = await request(ownerPage, "post", `/repositories/${repository.id}/infrastructure-executions`, owner.headers, executionInput) as any;
    const report = async (page: Page, headers: Record<string, string>, stepID: string, evidence: any) => execution = await request(page, "post", `/repositories/${repository.id}/infrastructure-executions/${execution.id}/reports`, headers, { expected_version: execution.version, step_id: stepID, report: evidence });
    await report(ownerPage, owner.headers, "step-database", { status: "succeeded", provider_response: "Encrypted replacement restored from attested recovery point", health: "healthy", cost_units: 25, blockers: [], next_action: "provision bounded cache", safety_point: true });
    await rejected(await servicePage.request.post(`/api/repositories/${repository.id}/infrastructure-executions/${execution.id}/reports`, { headers: agentHeaders, data: { expected_version: execution.version, step_id: "step-cache", report: { status: "succeeded", provider_response: "Expired task credential attempts provider reporting", health: "healthy", cost_units: 12, blockers: [], next_action: "verify service", safety_point: true } } }), 401, "unauthorized");
    await report(ownerPage, owner.headers, "step-cache", { status: "succeeded", provider_response: "Controller retained scoped agent cache evidence without secret output", health: "healthy", cost_units: 12, blockers: [], next_action: "verify service", safety_point: true });
    await report(ownerPage, owner.headers, "step-service", { status: "failed", provider_response: "Provider returned a transient routing failure", health: "degraded", cost_units: 20, blockers: ["provider routing unavailable"], next_action: "apply reviewed route remediation", safety_point: true });
    const partialOutcomes = execution.expected_outcomes.map((x: any) => ({ resource_id: x.resource_id, present: x.present, provider_revision: "provider-partial-1", service: x.resource_id === "service" ? "failed" : "passed", security: "passed", privacy: "passed", cost: "passed", continuity: "passed", measures_passed: x.resource_id === "service" ? [] : x.measures, summary: "Partial apply evidence retained at the provider failure safety point" }));
    execution = await request(ownerPage, "post", `/repositories/${repository.id}/infrastructure-executions/${execution.id}/assessments`, owner.headers, { expected_version: execution.version, outcomes: partialOutcomes, unmanaged_resources: [], failed_cleanup: ["old route remains"] });
    expect(execution.assessments.at(-1).converged).toBe(false);
    await rejected(await ownerPage.request.post(`/api/repositories/${repository.id}/infrastructure-executions/${execution.id}/controls`, { headers: owner.headers, data: { expected_version: execution.version, action: "resume", summary: "Resume before remediation." } }), 409, "infrastructure_execution_blocked");
    await report(ownerPage, owner.headers, "step-service", { status: "running", provider_response: "Reviewed route remediation restored provider health", health: "healthy", cost_units: 20, blockers: [], next_action: "resume exact service verification", safety_point: true });
    execution = await request(ownerPage, "post", `/repositories/${repository.id}/infrastructure-executions/${execution.id}/controls`, owner.headers, { expected_version: execution.version, action: "resume", summary: "Provider remediation is retained and blockers are clear." });
    await rejected(await servicePage.request.post(`/api/repositories/${repository.id}/infrastructure-executions/${execution.id}/reports`, { headers: agentHeaders, data: { expected_version: execution.version, step_id: "step-service", report: { status: "succeeded", provider_response: "Agent attempts an undelegated service action", health: "healthy", cost_units: 20, blockers: [], next_action: "finish", safety_point: true } } }), 401, "unauthorized");
    await rejected(await ownerPage.request.post(`/api/repositories/${repository.id}/infrastructure-executions/${execution.id}/reports`, { headers: owner.headers, data: { expected_version: execution.version, step_id: "step-service", report: { status: "succeeded", provider_response: "Healthy but over budget", health: "healthy", cost_units: 50, blockers: [], next_action: "finish", safety_point: true } } }), 409, "infrastructure_execution_blocked");
    await report(ownerPage, owner.headers, "step-service", { status: "succeeded", provider_response: "Released service health and recovery journey verified", health: "healthy", cost_units: 20, blockers: [], next_action: "monitor declared outcomes", safety_point: true });
    expect(execution.status).toBe("succeeded");
    const outcomes = execution.expected_outcomes.map((x: any) => ({ resource_id: x.resource_id, present: x.present, provider_revision: "provider-release-2", service: "passed", security: "passed", privacy: "passed", cost: "passed", continuity: "passed", measures_passed: x.measures, summary: "Released service, cost, privacy, reliability, and recovery measures pass" }));
    execution = await request(ownerPage, "post", `/repositories/${repository.id}/infrastructure-executions/${execution.id}/assessments`, owner.headers, { expected_version: execution.version, outcomes, unmanaged_resources: [], failed_cleanup: [] });
    expect(execution.assessments.at(-1).converged).toBe(true);

    execution = await request(servicePage, "post", `/repositories/${repository.id}/infrastructure-executions/${execution.id}/monitor-runs`, service.headers, { permission: "granted", provider_status: "available", findings: [{ kind: "configuration_drift", resource_id: "service", severity: "high", summary: "Out-of-band route timeout differs from reviewed intent", cause: "Provider console edit by an identified operator" }] });
    const finding = execution.monitor_runs.at(-1).findings[0];
    await git(copy, "fetch", "origin", "main"); await git(copy, "switch", "-C", "repair-drift", "origin/main");
    await writeFile(join(copy, "app.txt"), "service=v2 cache=enabled route_timeout=reviewed\n"); await git(copy, "add", "app.txt"); await git(copy, "commit", "-m", "Restore reviewed route timeout"); await git(copy, "push", "origin", "repair-drift");
    const repairPull = await request(servicePage, "post", `/repositories/${repository.id}/pulls`, service.headers, { title: "Restore reviewed route timeout", body: "Repair the retained out-of-band drift through ordinary review.", source_branch: "repair-drift", target_branch: "main" }) as any;
    await request(securityPage, "post", `/repositories/${repository.id}/pulls/${repairPull.id}/reviews`, security.headers, { decision: "approved" });
    await request(ownerPage, "post", `/repositories/${repository.id}/pulls/${repairPull.id}/merge`, owner.headers, {});
    execution = await request(ownerPage, "post", `/repositories/${repository.id}/infrastructure-executions/${execution.id}/drift-responses`, owner.headers, { expected_version: execution.version, response: { finding_id: finding.id, kind: "repair", owner_id: service.user.id, resource_kind: "pull_request", resource_id: repairPull.id, summary: "Reviewed repair restores declared state without rewriting the external edit." } });
    expect(execution.drift_responses.at(-1)).toMatchObject({ finding_id: finding.id, resource_id: repairPull.id, kind: "repair" });
    const revoked = await ownerPage.request.delete(`/api/auth/credentials/${credential.id}`, { headers: owner.headers });
    expect(revoked.status()).toBe(204);
    await rejected(await ownerPage.request.get("/api/user", { headers: { Authorization: `Bearer ${credential.token}` } }), 401, "unauthorized");

    await ownerPage.goto(`/repositories/${repository.id}/infrastructure`);
    await expect(ownerPage.getByRole("heading", { name: "Infrastructure" })).toBeVisible();
    await expect(ownerPage.getByText("Authoritative executions")).toBeVisible();
    await expect(ownerPage.getByText("converged", { exact: true })).toBeVisible();
    await expect(ownerPage.getByText("Out-of-band route timeout differs from reviewed intent")).toBeVisible();
    await expect(ownerPage.getByText("Reviewed repair restores declared state without rewriting the external edit.")).toBeVisible();
    expect(merged.merge_commit_id).toBeTruthy();
  } finally {
    await rm(copy, { recursive: true, force: true });
  }
});
