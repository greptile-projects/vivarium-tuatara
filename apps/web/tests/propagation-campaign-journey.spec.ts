import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey joins independently governed repositories */
const run = promisify(execFile);
async function git(token: string, cwd: string, ...args: string[]) {
  return (await run("git", ["-c", "credential.helper=", ...args], { cwd, env: { ...process.env, GIT_ASKPASS: join(__dirname, "git-askpass.sh"), GIT_TERMINAL_PROMPT: "0", VIVARIUM_GIT_TOKEN: token } })).stdout.trim();
}
async function account(page: Page, suffix: string, role: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(`${role} maintainer`);
  await page.getByLabel("Handle").fill(`propagation-${role.toLowerCase()}-${suffix}`);
  await page.getByRole("button", { name: "Create account and continue" }).click();
  await expect(page.getByRole("heading", { name: "Create a repository" })).toBeVisible();
  const token = await page.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  return { headers, user: await (await page.request.get("/api/user", { headers })).json() };
}
async function json(page: Page, method: "get" | "post", path: string, headers: Record<string, string>, data?: unknown) {
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

test("a proven repair reaches independently governed supported users", async ({ browser }) => {
  test.setTimeout(300_000);
  test.skip(!(await run("docker", ["info"]).then(() => true, () => false)), "equivalence checks require Docker");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copies: string[] = [], credentials: { page: Page; headers: Record<string, string>; id: string }[] = [];
  try {
    const suffix = Date.now().toString(36);
    const coordinatorPage = await (await browser.newContext()).newPage(), ownerPage = await (await browser.newContext()).newPage();
    const coordinator = await account(coordinatorPage, suffix, "Coordinator"), owner = await account(ownerPage, suffix, "Consumer");
    const source = await json(coordinatorPage, "post", "/repositories", coordinator.headers, { name: `verified-repair-${suffix}` });
    const target = await json(ownerPage, "post", "/repositories", owner.headers, { name: `independent-consumer-${suffix}` });
    await json(coordinatorPage, "post", `/repositories/${source.id}/collaborators`, coordinator.headers, { user_id: owner.user.id });
    const credential = async (page: Page, headers: Record<string, string>, name: string) => {
      const value = await json(page, "post", "/auth/credentials", headers, { kind: "git", name, scopes: ["git:read", "git:write"], expires_in: 3600 });
      credentials.push({ page, headers, id: value.id }); return value;
    };
    const sourceGit = await credential(coordinatorPage, coordinator.headers, "propagation source"), targetGit = await credential(ownerPage, owner.headers, "propagation target");
    const sourceCopy = await mkdtemp(join(tmpdir(), "vivarium-propagation-source-")), targetCopy = await mkdtemp(join(tmpdir(), "vivarium-propagation-target-")); copies.push(sourceCopy, targetCopy);
    await git(sourceGit.token, tmpdir(), "clone", `http://localhost:3000/git/${source.id}.git`, sourceCopy);
    await git(targetGit.token, tmpdir(), "clone", `http://localhost:3000/git/${target.id}.git`, targetCopy);
    for (const copy of copies) { await git("", copy, "config", "user.name", "Propagation Maintainer"); await git("", copy, "config", "user.email", "maintainer@example.test"); await mkdir(join(copy, ".vivarium")); }
    const checks = { version: 1, checks: [{ name: "reject traversal", image: "alpine:3.22", command: "grep -q 'traversal=blocked' behavior.txt" }] };
    await writeFile(join(sourceCopy, ".vivarium", "checks.json"), JSON.stringify(checks)); await writeFile(join(sourceCopy, "behavior.txt"), "traversal=allowed\n");
    await git("", sourceCopy, "add", "."); await git("", sourceCopy, "commit", "-m", "Release vulnerable parser"); const vulnerable = await git("", sourceCopy, "rev-parse", "HEAD");
    await writeFile(join(sourceCopy, "behavior.txt"), "traversal=blocked\n"); await git("", sourceCopy, "add", "."); await git("", sourceCopy, "commit", "-m", "Block traversal after regression report"); const repair = await git("", sourceCopy, "rev-parse", "HEAD"); await git(sourceGit.token, sourceCopy, "push", "origin", "main");
    await git(sourceGit.token, targetCopy, "remote", "add", "upstream", `http://localhost:3000/git/${source.id}.git`); await git(sourceGit.token, targetCopy, "fetch", "upstream", "main"); await git("", targetCopy, "reset", "--hard", vulnerable);
    await writeFile(join(targetCopy, ".vivarium", "checks.json"), JSON.stringify(checks)); await writeFile(join(targetCopy, "behavior.txt"), "adapter=v2\ntraversal=allowed\n");
    await git("", targetCopy, "add", "."); await git("", targetCopy, "commit", "-m", "Release divergent v2 adapter"); await git(targetGit.token, targetCopy, "push", "origin", "main");
    await git("", targetCopy, "switch", "-c", "repair-v2"); await writeFile(join(targetCopy, "behavior.txt"), "adapter=v2\ntraversal=blocked\n"); await git("", targetCopy, "add", "."); await git("", targetCopy, "commit", "-m", "Adapt traversal repair for v2"); const targetRepair = await git("", targetCopy, "rev-parse", "HEAD"); await git(targetGit.token, targetCopy, "push", "origin", "repair-v2");

    const deadline = new Date(Date.now() + 14 * 86400_000).toISOString(), targetID = "consumer-v2", missingID = "inaccessible-consumer";
    let campaign = await json(coordinatorPage, "post", `/repositories/${source.id}/propagation-campaigns`, coordinator.headers, {
      request_id: `repair-${suffix}`, title: "Propagate traversal repair", intent: "Reject parent-directory traversal without changing valid adapter behavior", acceptance_criteria: ["Traversal is blocked", "Valid adapter behavior remains"],
      source: { kind: "regression_correction", resource_id: `regression-${suffix}`, commits: [repair], label: "Verified traversal regression repair" },
      targets: [
        { id: targetID, kind: "repository", repository_id: target.id, release_line: "repair-v2", owner_ids: [owner.user.id], deadline, acceptance_criteria: ["v2 adapter remains active"] },
        { id: missingID, kind: "repository", repository_id: "f".repeat(32), release_line: "main", owner_ids: [coordinator.user.id], deadline },
      ], completion_policy: { mode: "minimum", minimum_targets: 1, require_acceptance: true },
    });
    expect(campaign.targets.find((x: any) => x.id === missingID).state).toBe("unsupported");
    campaign = (await json(ownerPage, "post", `/repositories/${source.id}/propagation-campaigns/${campaign.id}/targets/${targetID}/assessments`, owner.headers, {})).campaign;
    let assessment = campaign.assessments[0];
    expect(["adaptation_required", "conflicting"]).toContain(assessment.classification);
    campaign = (await json(ownerPage, "post", `/repositories/${source.id}/propagation-campaigns/${campaign.id}/assessments/${assessment.id}/entries`, owner.headers, { expected_version: assessment.version, kind: "finding", body: "The v2 adapter needs a local representation change while preserving the repair outcome.", citations: [{ kind: "target_commit", reference: "adapted repair", revision: targetRepair }] })).campaign;
    assessment = campaign.assessments[0];
    const contributionResult = await json(ownerPage, "post", `/repositories/${source.id}/propagation-campaigns/${campaign.id}/targets/${targetID}/contributions`, owner.headers, { assessment_id: assessment.id, expected_version: assessment.version, application: "adapted", deviation: "Retain the v2 adapter marker while applying the same traversal rejection.", topology: "local_branch", constraints: ["v2 remains supported"], tasks: [{ title: "Adapt v2 repair", outcome: "Traversal is rejected by v2", assignee_type: "human", assignee_id: owner.user.id }, { title: "Audit consumer variants", outcome: "Independently check other adapter variants", assignee_type: "agent", assignee_id: "a".repeat(32) }] });
    campaign = contributionResult.campaign; const contribution = contributionResult.contribution, humanTask = contributionResult.tasks[0];
    const adaptation = (command: string) => [{ scenario: "reject traversal", command, environment_check: "reject traversal", coverage: ["Traversal is blocked", "Valid adapter behavior remains"] }];
    const failed = await json(ownerPage, "post", `/repositories/${source.id}/propagation-campaigns/${campaign.id}/targets/${targetID}/equivalence-proofs`, owner.headers, { request_id: `failed-${suffix}`, target_revision: targetRepair, adaptations: adaptation("false") });
    expect(failed.equivalence_proof.state).toBe("failed");
    const demonstrated = await json(ownerPage, "post", `/repositories/${source.id}/propagation-campaigns/${campaign.id}/targets/${targetID}/equivalence-proofs`, owner.headers, { request_id: `passing-${suffix}`, target_revision: targetRepair, adaptations: adaptation("grep -q 'adapter=v2' behavior.txt && grep -q 'traversal=blocked' behavior.txt") });
    let proof = demonstrated.equivalence_proof;
    const accepted = await json(ownerPage, "post", `/repositories/${source.id}/propagation-campaigns/${campaign.id}/equivalence-proofs/${proof.id}/decisions`, owner.headers, { expected_version: proof.version, decision: "accepted", rationale: "Equivalent traversal rejection and ordinary v2 governance checks pass together." }); proof = accepted.equivalence_proof;
    const pull = await json(ownerPage, "post", `/repositories/${target.id}/proposals/${contribution.proposal_id}/tasks/${humanTask.id}/contributions`, owner.headers, { title: "Adapt traversal repair for v2", body: "Preserves the verified source intent with an explicit v2 representation change.", source_branch: "repair-v2", target_branch: "main" });
    campaign = (await json(ownerPage, "post", `/repositories/${source.id}/propagation-campaigns/${campaign.id}/targets/${targetID}/delivery-paths`, owner.headers, { contribution_id: contribution.id, equivalence_proof_id: proof.id, proof_version: proof.version, pull_request_id: pull.id, supported_user_groups: ["v2 adapter users"] })).campaign;
    expect(campaign.coverage).toMatchObject({ policy_satisfied: false, delivered_targets: 0 });
    await json(ownerPage, "post", `/repositories/${source.id}/propagation-campaigns/${campaign.id}/scope-events`, owner.headers, { kind: "bounded_exception", target_id: targetID, reason: "Upstream requested representation changes before acceptance", follow_up: `pull ${pull.id}`, expires_at: new Date(Date.now() + 7 * 86400_000).toISOString() });
    await json(coordinatorPage, "post", `/repositories/${source.id}/propagation-campaigns/${campaign.id}/scope-events`, coordinator.headers, { kind: "consumer_discovered", consumer_repository_id: "e".repeat(32), supported_user_groups: ["embedded users"], reason: "Support reported another maintained consumer", follow_up: "Invite its current maintainer and assess its supported line" });
    await rejected(await coordinatorPage.request.post(`/api/repositories/${source.id}/propagation-campaigns/${campaign.id}/targets/${targetID}/delivery-paths`, { headers: coordinator.headers, data: { contribution_id: contribution.id, equivalence_proof_id: proof.id, proof_version: proof.version, pull_request_id: pull.id, supported_user_groups: ["v2 adapter users"] } }), 403, "propagation_delivery_forbidden");
    await ownerPage.goto(`/repositories/${source.id}/propagation`);
    await expect(ownerPage.getByRole("heading", { name: "Propagation campaigns" })).toBeVisible();
    await expect(ownerPage.getByText("Propagate traversal repair", { exact: true }).first()).toBeVisible();
    await ownerPage.getByText("Propagate traversal repair", { exact: true }).first().click();
    await expect(ownerPage.getByText("Users: v2 adapter users", { exact: true })).toBeVisible();
    await expect(ownerPage.getByText(/Support reported another maintained consumer/).first()).toBeVisible();
  } finally {
    for (const item of credentials) await item.page.request.delete(`/api/auth/credentials/${item.id}`, { headers: item.headers }).catch(() => undefined);
    for (const copy of copies) await rm(copy, { recursive: true, force: true });
  }
});
