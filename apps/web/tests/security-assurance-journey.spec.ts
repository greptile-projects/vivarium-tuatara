import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- one journey intentionally crosses every security ledger */

const run = promisify(execFile);
const sha = (value: string) => createHash("sha256").update(value).digest("hex");
async function git(cwd: string, ...args: string[]) {
  return (await run("git", args, { cwd, env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } })).stdout.trim();
}
async function account(page: Page, suffix: string, role: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(`${role} security collaborator`);
  await page.getByLabel("Handle").fill(`security-${role.toLowerCase()}-${suffix}`);
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

test("humans and a bounded agent carry anticipated abuse through sustained protection", async ({ browser }) => {
  test.setTimeout(360_000);
  const dockerAvailable = await run("docker", ["info"]).then(() => true, () => false);
  test.skip(!dockerAvailable, "security evidence, checks, builds, and staged deployments use bounded containers");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const ownerCopy = await mkdtemp(join(tmpdir(), "vivarium-security-owner-"));
  const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-security-agent-"));
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const securityPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, suffix, "Owner");
    const security = await account(securityPage, suffix, "Engineer");
    const repository = await json(ownerPage, "post", "/repositories", owner.headers, { name: `privileged-rotation-${suffix}` });
    await json(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: security.user.id });
    const credential = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "security journey", scopes: ["git:read", "git:write"], expires_in: 3600 });
    await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${repository.id}.git`, ownerCopy);
    await git(ownerCopy, "config", "user.name", "Security Owner"); await git(ownerCopy, "config", "user.email", "owner@example.test");
    await mkdir(join(ownerCopy, ".vivarium")); await mkdir(join(ownerCopy, "fixtures"));
    const command = "grep -q 'control=enforced' privileged.txt";
    const securityDefinition = JSON.stringify({ version: 1, checks: [{ abuse_path_id: "replay-approval", command, isolation: "workspace" }] });
    const fixture = '{"actor":"synthetic-operator","request":"rotate"}\n';
    await writeFile(join(ownerCopy, ".vivarium", "workspace.json"), JSON.stringify({ version: 1, image: "alpine:3.22", tools: [], dependencies: [], setup: [], experiments: [{ name: "approval replay", command }], resources: { cpus: 1, memory_mb: 256, storage_mb: 128, setup_seconds: 30 } }));
    await writeFile(join(ownerCopy, ".vivarium", "checks.json"), JSON.stringify({ version: 1, checks: [{ name: "privileged replay defense", image: "alpine:3.22", command }] }));
    await writeFile(join(ownerCopy, ".vivarium", "release.json"), JSON.stringify({ version: 1, steps: [{ name: "package", image: "alpine:3.22", command: "cp privileged.txt \"$VIVARIUM_OUTPUT/privileged.txt\"" }] }));
    await writeFile(join(ownerCopy, ".vivarium", "deployment.json"), JSON.stringify({ version: 1, stages: [{ name: "staged", signals: [{ name: "approval replay defense", command: "grep -q 'control=enforced' \"$VIVARIUM_ARTIFACT\"" }] }] }));
    await writeFile(join(ownerCopy, ".vivarium", "packages.json"), JSON.stringify({ version: 1, dependencies: [], lock: [] }));
    await writeFile(join(ownerCopy, ".vivarium", "security.json"), securityDefinition);
    await writeFile(join(ownerCopy, "fixtures", "rotation.json"), fixture);
    await writeFile(join(ownerCopy, "privileged.txt"), "control=enforced\nassumption=single-use\n");
    await git(ownerCopy, "add", "."); await git(ownerCopy, "commit", "-m", "Establish privileged rotation boundary"); await git(ownerCopy, "push", "origin", "main");
    const base = await git(ownerCopy, "rev-parse", "HEAD");
    await json(ownerPage, "put", `/repositories/${repository.id}/branches/main/required-checks`, owner.headers, { checks: ["privileged replay defense"] });

    const expectation = await json(ownerPage, "post", `/repositories/${repository.id}/security-expectations`, owner.headers, { revision: {
      title: "Privileged credential rotation", summary: "Only an approved, single-use request may rotate a service credential.",
      scopes: [{ kind: "journey", resource_id: "credential-rotation", name: "Rotate a service credential" }],
      protected_assets: [{ id: "credential", name: "Service credential", classification: "secret", protection: "Never disclose or rotate without fresh approval", owner_ids: [owner.user.id] }],
      trust_boundaries: [{ id: "operator-api", name: "Operator to privileged API", from: "operator", to: "rotation service", direction: "inbound", asset_ids: ["credential"], guarantees: ["approval is fresh and single-use"] }],
      actors: [{ id: "operator", name: "Approved operator", kind: "human", trust: "partially_trusted", capabilities: ["request rotation"] }, { id: "attacker", name: "Replay attacker", kind: "attacker", trust: "untrusted", capabilities: ["replay a captured request"] }],
      abuse_cases: [{ id: "replay", title: "Replay approval", actor_ids: ["attacker"], asset_ids: ["credential"], boundary_ids: ["operator-api"], scenario: "Replay a prior approval after it was consumed", impact: "Unauthorized credential replacement", severity: "critical", control_ids: ["nonce"], owner_ids: [owner.user.id, security.user.id] }],
      required_controls: [{ id: "nonce", name: "Single-use approval nonce", requirement: "Consume an approval nonce atomically", kind: "prevent", owner_ids: [owner.user.id, security.user.id], evidence: "Exact isolated replay scenario", status: "supported" }],
      severity_policy: [{ level: "critical", response: "Private immediate repair", release_rule: "Block delivery without exact current proof" }], commitment_links: [{ kind: "release", resource_id: "privileged-workflow", summary: "Privileged workflow delivery" }], exceptions: [], owner_ids: [owner.user.id, security.user.id], rationale: "Define the boundary before implementation"
    }});
    expect(expectation.current_version).toBe(1);

    const proposal = await json(ownerPage, "post", `/repositories/${repository.id}/proposals`, owner.headers, { title: "Implement credential rotation", body: "Build the privileged workflow under bounded agent collaboration." });
    const task = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${proposal.id}/tasks`, owner.headers, { title: "Implement privileged rotation", outcome: "A reviewable candidate preserves the named trust boundary." });
    const agentID = "a".repeat(32);
    const assignment = await json(ownerPage, "put", `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/assignment`, owner.headers, { assignee_type: "agent", assignee_id: agentID, mandate: "Implement only the privileged workflow and report security uncertainty.", repository_id: repository.id, base_revision: base });
    const launched = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/sessions`, owner.headers, { expected_assignment_id: assignment.assignment.id, context_paths: ["privileged.txt", ".vivarium/security.json", "fixtures/rotation.json"], expires_in: 3600 });
    await git(tmpdir(), "clone", `http://git:${launched.credential.token}@localhost:3000/git/${repository.id}.git`, agentCopy);
    await git(agentCopy, "config", "user.name", "Bounded Security Agent"); await git(agentCopy, "config", "user.email", "agent@agents.vivarium");
    await git(agentCopy, "switch", launched.run.working_branch);
    await writeFile(join(agentCopy, "privileged.txt"), "control=missing\nassumption=single-use\n");
    await git(agentCopy, "add", "privileged.txt"); await git(agentCopy, "commit", "-m", "Implement privileged rotation candidate"); await git(agentCopy, "push", "origin", launched.run.working_branch);
    const vulnerable = await git(agentCopy, "rev-parse", "HEAD");
    const agentHeaders = { Authorization: `Bearer ${launched.credential.token}` };
    const runPath = `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/sessions/${launched.session.id}/runs/${launched.run.id}`;
    await json(ownerPage, "post", `${runPath}/completion`, agentHeaders, { summary: "Implemented the candidate and retained replay uncertainty.", commit_id: vulnerable, checks: [{ name: "candidate structure", status: "passed" }], unresolved_concerns: ["Captured approvals may be replayable"] });
    const sourcePull = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/contributions`, owner.headers, { title: "Add privileged credential rotation", body: "Agent candidate for security modeling and independent review.", source_branch: launched.run.working_branch, target_branch: "main", session_id: launched.session.id, run_id: launched.run.id });

    const modelRevision = (revision: string, pullID = sourcePull.id) => ({ title: "Credential rotation replay analysis", source: { kind: "pull_request", resource_id: pullID, revision, summary: "Exact privileged workflow candidate" },
      entry_points: [{ id: "rotate", name: "Rotation endpoint", access: "authenticated operator", boundary: "operator-api" }], privileges: [{ id: "approve", principal: "operator", capability: "approve rotation", scope: "one request" }], data_flows: [{ id: "approval-flow", from: "operator", to: "rotation service", data: "approval nonce", protection: "authenticated and consumed" }], dependencies: [{ id: "nonce-store", name: "Nonce store", trust: "atomic consumption" }],
      abuse_paths: [{ id: "replay-approval", attacker_goal: "rotate with a consumed approval", entry_point_ids: ["rotate"], privilege_ids: ["approve"], data_flow_ids: ["approval-flow"], dependency_ids: ["nonce-store"], steps: ["capture synthetic approval", "replay after consumption"], impact: "unauthorized rotation", mitigation_ids: ["consume-once"], residual_risk: "low after atomic consumption", owner_ids: [owner.user.id, security.user.id] }],
      mitigations: [{ id: "consume-once", description: "Atomically consume every approval nonce", status: "proposed", evidence_ids: ["candidate"], owner_ids: [owner.user.id, security.user.id] }],
      evidence: [{ id: "candidate", kind: "pull_request", resource_id: pullID, revision, summary: "Visible exact candidate", accessible: true }, { id: "restricted-review", kind: "security_review", resource_id: "external-embargo", revision: "1", summary: "Restricted external note", accessible: false, gap: "not in the model audience" }],
      alternatives: [{ id: "atomic", name: "Atomic nonce consumption", description: "Consume before rotating", security_effect: "closes replay window", tradeoffs: ["one storage write"], evidence_ids: ["candidate"] }], owner_ids: [owner.user.id, security.user.id], assumptions: ["approval nonces are single-use"] });
    let model = await json(ownerPage, "post", `/repositories/${repository.id}/threat-models`, owner.headers, { revision: modelRevision(vulnerable) });
    model = await json(ownerPage, "post", `/repositories/${repository.id}/threat-models/${model.id}/events`, owner.headers, { expected_version: 1, event: { kind: "finding", body: "The bounded agent reported that the candidate checks identity but does not atomically consume the approval.", resource_ids: ["replay-approval"], evidence_ids: ["candidate"], alternative_ids: ["atomic"], requested_owner_ids: [owner.user.id, security.user.id] } });
    await json(ownerPage, "post", `/repositories/${repository.id}/threat-models/${model.id}/acknowledgements`, owner.headers, { model_version: 1, acknowledgement: { owner_id: owner.user.id, decision: "changes_requested", note: "Demonstrate the replay before delivery." } });
    await json(securityPage, "post", `/repositories/${repository.id}/threat-models/${model.id}/acknowledgements`, security.headers, { model_version: 1, acknowledgement: { owner_id: security.user.id, decision: "acknowledged", note: "The abuse path matches the affected control." } });

    const scenarioInput = (commit: string, title: string) => ({ threat_model_id: model.id, threat_model_version: 1, abuse_path_id: "replay-approval", title, attacker_preconditions: ["synthetic approval was consumed"], bounded_capabilities: ["replay one synthetic request in an isolated workspace"], fixtures: [{ id: "approval", description: "Synthetic rotation approval", path: "fixtures/rotation.json", sha256: sha(fixture), data_class: "synthetic", generator: "repository fixture" }], actions: ["submit the synthetic approval twice"], containment: [{ id: "blocked", description: "second use is rejected", observable: "control marker", expected: "control=enforced" }], detection: [{ id: "audit", description: "replay is audited", observable: "sanitized scenario report", expected: "replay attempted" }], recovery: [{ id: "unchanged", description: "credential remains unchanged", observable: "synthetic state", expected: "no second rotation" }], mitigation_ids: ["consume-once"], dependency_ids: ["nonce-store"], commit_id: commit, check_path: ".vivarium/security.json", check_sha256: sha(securityDefinition), command, isolation: "workspace", max_cost_units: 10 });
    await rejected(await ownerPage.request.post(`/api/repositories/${repository.id}/security-scenarios`, { headers: owner.headers, data: { ...scenarioInput(vulnerable, "Unsafe production replay"), bounded_capabilities: ["replay production data"] } }), 400, "security_scenario_unsafe");

    async function retainAttempt(commit: string, title: string, result: "failed" | "passed") {
      let scenario = await json(ownerPage, "post", `/repositories/${repository.id}/security-scenarios`, owner.headers, scenarioInput(commit, title));
      scenario = await json(ownerPage, "post", `/repositories/${repository.id}/security-scenarios/${scenario.id}/review`, owner.headers, { decision: "approved", note: "The fixture and authority are bounded to the exact candidate." });
      const workspace = await json(ownerPage, "post", "/workspaces", owner.headers, { repository_id: repository.id, commit_id: commit, source: { kind: "repository" } });
      const outcome = await json(ownerPage, "post", `/workspaces/${workspace.id}/commands`, owner.headers, { command, timeout_seconds: 30 });
      scenario = await json(ownerPage, "post", `/repositories/${repository.id}/security-scenarios/${scenario.id}/attempts`, owner.headers, { outcome_ids: [outcome.outcome.id], attempt: { revision: commit, execution_kind: "workspace", workspace_id: workspace.id, result, artifacts: [{ kind: "report", name: "sanitized replay report", sha256: "b".repeat(64), size: 128, sanitized: true }], coverage: { abuse_attempted: true, containment_ids: result === "passed" ? ["blocked"] : [], detection_ids: ["audit"], recovery_ids: result === "passed" ? ["unchanged"] : [], gaps: result === "failed" ? ["atomic consumption absent"] : [] }, cost_units: 1, provenance: ["exact isolated repository workspace"] } });
      return scenario;
    }
    const failedBase = await retainAttempt(vulnerable, "Replay the vulnerable candidate", "failed");
    expect(failedBase.attempts.at(-1).result).toBe("failed");

    let falsePositive = await json(securityPage, "post", `/repositories/${repository.id}/security-findings`, security.headers, { threat_model_id: model.id, threat_model_version: 1, abuse_path_id: "replay-approval", candidate_commit_id: vulnerable, title: "Synthetic audit ordering", description: "An audit timestamp initially appeared out of order.", severity: "low", audience: [owner.user.id, security.user.id], evidence: [{ id: "audit-order", kind: "sanitized_log", summary: "Synthetic timestamps only", sha256: "c".repeat(64) }], acceptance_criteria: ["Owner classifies the report"] });
    falsePositive = await json(ownerPage, "post", `/repositories/${repository.id}/security-findings/${falsePositive.id}/classification`, owner.headers, { expected_version: 1, classification: "false_positive", rationale: "Clock normalization proves no control failure.", audience: [owner.user.id, security.user.id] });
    expect(falsePositive.events.at(-1).classification).toBe("false_positive");

    let finding = await json(securityPage, "post", `/repositories/${repository.id}/security-findings`, security.headers, { threat_model_id: model.id, threat_model_version: 1, abuse_path_id: "replay-approval", candidate_commit_id: vulnerable, title: "Consumed approval can be replayed", description: "The isolated exact-candidate scenario rotates twice.", severity: "critical", audience: [owner.user.id, security.user.id], evidence: [{ id: "failed-replay", kind: "scenario_attempt", summary: "Sanitized failed replay", sha256: "d".repeat(64) }], acceptance_criteria: ["Replay is contained", "Owner-reviewed exact scenario passes"] });
    finding = await json(ownerPage, "post", `/repositories/${repository.id}/security-findings/${finding.id}/classification`, owner.headers, { expected_version: 1, classification: "confirmed", rationale: "Exact isolated evidence reproduces the modeled path.", audience: [owner.user.id, security.user.id] });
    const repair = await json(ownerPage, "post", `/repositories/${repository.id}/security-findings/${finding.id}/repair`, owner.headers, { expected_version: finding.version, assignee_type: "human", assignee_id: security.user.id, title: "Consume approvals atomically", workspace_kind: "task" });
    finding = repair.security_finding;

    await git(ownerCopy, "fetch", "origin", launched.run.working_branch); await git(ownerCopy, "switch", "-c", "repair/atomic-approval", vulnerable);
    await writeFile(join(ownerCopy, "privileged.txt"), "control=partial\nassumption=single-use\n"); await git(ownerCopy, "add", "privileged.txt"); await git(ownerCopy, "commit", "-m", "Attempt atomic approval repair"); await git(ownerCopy, "push", "origin", "repair/atomic-approval");
    const firstRepair = await git(ownerCopy, "rev-parse", "HEAD");
    const repairPull = await json(securityPage, "post", `/repositories/${repository.id}/proposals/${repair.proposal.id}/tasks/${repair.task.id}/contributions`, security.headers, { title: "Consume rotation approvals atomically", body: "Governed repair for the exact private finding.", source_branch: "repair/atomic-approval", target_branch: "main" });
    const failedRepair = await retainAttempt(firstRepair, "First atomic repair attempt", "failed");
    expect(failedRepair.attempts.at(-1).coverage.gaps).toContain("atomic consumption absent");
    await json(ownerPage, "post", `/repositories/${repository.id}/security-findings/${finding.id}/classification`, owner.headers, { expected_version: finding.version, classification: "failed_repair", rationale: "The first repair still permits replay.", audience: [owner.user.id, security.user.id] });
    finding = await json(ownerPage, "get", `/repositories/${repository.id}/security-findings`, owner.headers).then((x: any) => x.security_findings.find((f: any) => f.id === finding.id));
    finding = await json(ownerPage, "post", `/repositories/${repository.id}/security-findings/${finding.id}/classification`, owner.headers, { expected_version: finding.version, classification: "confirmed", rationale: "Continue the existing governed repair after retained failure.", audience: [owner.user.id, security.user.id] });

    await writeFile(join(ownerCopy, "privileged.txt"), "control=enforced\nassumption=single-use\n"); await git(ownerCopy, "add", "privileged.txt"); await git(ownerCopy, "commit", "-m", "Enforce atomic approval consumption"); await git(ownerCopy, "push", "origin", "repair/atomic-approval");
    const corrected = await git(ownerCopy, "rev-parse", "HEAD");
    await json(securityPage, "post", `/repositories/${repository.id}/pulls/${repairPull.id}/synchronize`, security.headers, {});
    const passing = await retainAttempt(corrected, "Atomic approval replay defense", "passed");
    finding = await json(ownerPage, "post", `/repositories/${repository.id}/security-findings/${finding.id}/protection`, owner.headers, { expected_version: finding.version, pull_request_id: repairPull.id, scenario_id: passing.id });
    expect(finding.repair.state).toBe("protected");

    await git(ownerCopy, "switch", "-C", "model-source-move", `origin/${launched.run.working_branch}`); await writeFile(join(ownerCopy, "agent-note.txt"), "model source moved after analysis\n"); await git(ownerCopy, "add", "agent-note.txt"); await git(ownerCopy, "commit", "-m", "Move modeled candidate"); await git(ownerCopy, "push", "origin", `HEAD:${launched.run.working_branch}`); await git(ownerCopy, "switch", "repair/atomic-approval");
    await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${sourcePull.id}/synchronize`, owner.headers, {});
    const stale = await json(ownerPage, "get", `/repositories/${repository.id}/threat-models/${model.id}`, owner.headers);
    expect(stale.freshness.fresh).toBe(false);
    model = await json(ownerPage, "post", `/repositories/${repository.id}/threat-models/${model.id}/revisions`, owner.headers, { expected_version: 1, revision: modelRevision(corrected, repairPull.id) });
    await json(ownerPage, "post", `/repositories/${repository.id}/threat-models/${model.id}/acknowledgements`, owner.headers, { model_version: 2, acknowledgement: { owner_id: owner.user.id, decision: "acknowledged", note: "The redesign and retained repair evidence close the replay path." } });
    await json(securityPage, "post", `/repositories/${repository.id}/threat-models/${model.id}/acknowledgements`, security.headers, { model_version: 2, acknowledgement: { owner_id: security.user.id, decision: "acknowledged", note: "Affected control owner accepts the current model." } });

    await json(ownerPage, "post", `/repositories/${repository.id}/security-requirements`, owner.headers, { expected_version: 0, requirements: [
      { id: "current-threat", title: "Current replay model", kind: "threat_coverage", threat_model_id: model.id, threat_model_version: 2, abuse_path_id: "replay-approval", owner_ids: [owner.user.id, security.user.id], selector: { branches: ["main"], assets: ["credential"], risk_classes: ["critical"], paths: ["privileged.txt"] } },
      { id: "replay-scenario", title: "Passing replay defense", kind: "security_scenario", threat_model_id: model.id, threat_model_version: 1, abuse_path_id: "replay-approval", scenario_id: passing.id, owner_ids: [owner.user.id], selector: { branches: ["main"], paths: ["privileged.txt"] } },
      { id: "control-owners", title: "Control-owner acknowledgement", kind: "control_acknowledgement", threat_model_id: model.id, threat_model_version: 2, abuse_path_id: "replay-approval", owner_ids: [owner.user.id, security.user.id], selector: { branches: ["main"], paths: ["privileged.txt"] } },
      { id: "findings", title: "Critical findings resolved", kind: "resolved_findings", owner_ids: [owner.user.id], finding_severities: ["critical"], selector: { branches: ["main"], paths: ["privileged.txt"] } }
    ] });
    const followUp = await json(ownerPage, "post", `/repositories/${repository.id}/issues`, owner.headers, { title: "No security exception", expected_behavior: "Exact replay evidence blocks delivery", observed_behavior: "An override was proposed", severity: "high", environment: "pull", reproduction_steps: ["Inspect security matrix"], visibility: "repository" });
    await rejected(await ownerPage.request.post(`/api/repositories/${repository.id}/security-exceptions`, { headers: owner.headers, data: { requirement_id: "replay-scenario", revision: corrected, rationale: "Attempt to bypass the missing defense", follow_up_kind: "issue", follow_up_id: followUp.id, scope: { branches: ["main"] }, expires_at: new Date(Date.now() + 31 * 86400000).toISOString() } }), 422, "security_confidence_invalid");
    const matrix = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${repairPull.id}/security-confidence`, owner.headers);
    expect(matrix).toMatchObject({ ready: true });
    expect(matrix.requirements.every((cell: any) => cell.state === "passed")).toBe(true);
    await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${repairPull.id}/reviews`, owner.headers, { decision: "approved" });
    const merged = await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${repairPull.id}/merge`, owner.headers, {});
    let checks = await eventually(() => json(ownerPage, "get", `/repositories/${repository.id}/pulls/${repairPull.id}/checks`, owner.headers), (x: any) => x.check_runs?.some((c: any) => c.commit_id === merged.merge_commit_id && c.state === "succeeded"), "integrated security check passes");
    const release = await json(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: "v1.0.0", notes: "Protected privileged rotation", commit_id: merged.merge_commit_id });
    await json(ownerPage, "post", `/repositories/${repository.id}/dependency-inventories`, owner.headers, { commit_id: merged.merge_commit_id });
    await json(ownerPage, "post", `/repositories/${repository.id}/releases/${release.id}/builds`, owner.headers, {});
    const builds = await eventually(() => json(ownerPage, "get", `/repositories/${repository.id}/releases/${release.id}/builds`, owner.headers), (x: any) => x.builds?.some((b: any) => b.state === "succeeded"), "security release build succeeds");
    const build = builds.builds.find((b: any) => b.state === "succeeded");
    const environment = await json(ownerPage, "post", `/repositories/${repository.id}/environments`, owner.headers, { name: "staged", position: 1, image: "alpine:3.22", command: "true", timeout_seconds: 30, configuration: {}, credentials: {}, required_approvals: 0, concurrency: 1 });
    const queued = await json(ownerPage, "post", `/repositories/${repository.id}/deployments`, owner.headers, { environment_id: environment.id, release_id: release.id, build_id: build.id, artifact_id: build.artifacts[0].id });
    const deployments = await eventually(() => json(ownerPage, "get", `/repositories/${repository.id}/deployments`, owner.headers), (x: any) => x.deployments?.some((d: any) => d.id === queued.id && d.state === "succeeded"), "staged security deployment succeeds");
    const deployment = deployments.deployments.find((d: any) => d.id === queued.id);
    const releaseMatrix = await json(ownerPage, "get", `/repositories/${repository.id}/releases/${release.id}/security-confidence`, owner.headers);
    const deploymentMatrix = await json(ownerPage, "get", `/repositories/${repository.id}/deployments/${deployment.id}/security-confidence`, owner.headers);
    expect(releaseMatrix, JSON.stringify(releaseMatrix)).toMatchObject({ ready: true }); expect(deploymentMatrix, JSON.stringify(deploymentMatrix)).toMatchObject({ ready: true });

    const sustained = await json(ownerPage, "post", `/repositories/${repository.id}/proposals`, owner.headers, { title: "Restore changed single-use assumption", body: "Private connected repair for the post-release signal." });
    const signal = await json(ownerPage, "post", `/repositories/${repository.id}/deployments/${deployment.id}/security-signals`, owner.headers, { requirement_id: "current-threat", kind: "assumption_violated", state: "confirmed", summary: "Sanitized staged telemetry shows approval retries can cross the original single-use window.", artifact_sha256: "e".repeat(64), assumption_ids: ["approval nonces are single-use"], control_ids: ["consume-once"], response_kind: "repair", response_id: sustained.id });
    expect(signal).toMatchObject({ response_kind: "repair", response_id: sustained.id, revision: merged.merge_commit_id });
    const sustainedTask = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${sustained.id}/tasks`, owner.headers, { title: "Bound approval retry windows", outcome: "Retries preserve atomic single-use consumption." });
    await json(ownerPage, "put", `/repositories/${repository.id}/proposals/${sustained.id}/tasks/${sustainedTask.id}/assignment`, owner.headers, { assignee_type: "human", assignee_id: owner.user.id, mandate: "Restore only the retry-window assumption under ordinary review.", repository_id: repository.id, base_revision: merged.merge_commit_id });
    await git(ownerCopy, "fetch", "origin", "main"); await git(ownerCopy, "switch", "-C", "repair/bounded-retry", merged.merge_commit_id); await writeFile(join(ownerCopy, "assumption.txt"), "retry_window=bounded\ncontrol=atomic-consumption\n"); await git(ownerCopy, "add", "assumption.txt"); await git(ownerCopy, "commit", "-m", "Restore control for bounded approval retries"); await git(ownerCopy, "push", "origin", "repair/bounded-retry");
    const sustainedPull = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${sustained.id}/tasks/${sustainedTask.id}/contributions`, owner.headers, { title: "Restore bounded retry control", body: "Private post-release repair connected to the sanitized assumption signal.", source_branch: "repair/bounded-retry", target_branch: "main" });
    const sustainedChecks = await eventually(() => json(ownerPage, "get", `/repositories/${repository.id}/pulls/${sustainedPull.id}/checks`, owner.headers), (x: any) => x.check_runs?.some((c: any) => c.state === "succeeded"), "connected assumption repair check passes");
    expect(sustainedChecks.check_runs.some((c: any) => c.state === "succeeded")).toBe(true);
    await json(securityPage, "post", `/repositories/${repository.id}/pulls/${sustainedPull.id}/reviews`, security.headers, { decision: "approved" });
    const sustainedMerge = await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${sustainedPull.id}/merge`, owner.headers, {});
    expect(sustainedMerge.merge_commit_id).toHaveLength(40);

    await ownerPage.goto(`/repositories/${repository.id}/security`);
    await expect(ownerPage.getByRole("heading", { name: "Security expectations" })).toBeVisible();
    await expect(ownerPage.getByText("Privileged credential rotation", { exact: true }).first()).toBeVisible();
    await expect(ownerPage.getByText("Credential rotation replay analysis", { exact: true }).first()).toBeVisible();
    await expect(ownerPage.getByText("Atomic approval replay defense", { exact: true }).first()).toBeVisible();
    const listedFindings = await json(ownerPage, "get", `/repositories/${repository.id}/security-findings`, owner.headers);
    expect(listedFindings.security_findings).toEqual(expect.arrayContaining([expect.objectContaining({ id: falsePositive.id }), expect.objectContaining({ id: finding.id, repair: expect.objectContaining({ state: "protected" }) })]));
    checks = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${repairPull.id}/checks`, owner.headers);
    expect(checks.check_runs.some((c: any) => c.commit_id === firstRepair && c.state === "failed")).toBe(true);
  } finally {
    await Promise.all([ownerCopy, agentCopy].map((path) => rm(path, { recursive: true, force: true })));
  }
});
