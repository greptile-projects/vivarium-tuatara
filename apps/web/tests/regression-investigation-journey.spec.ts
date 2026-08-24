import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey crosses the complete public regression ledger */

const run = promisify(execFile);
async function git(cwd: string, ...args: string[]) {
  return (await run("git", args, { cwd, env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } })).stdout.trim();
}
async function account(page: Page, suffix: string, role: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(`${role} collaborator`);
  await page.getByLabel("Handle").fill(`regression-${role.toLowerCase()}-${suffix}`);
  await page.getByRole("button", { name: "Create account and continue" }).click();
  await expect(page.getByRole("heading", { name: "Create a repository" })).toBeVisible();
  const token = await page.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(token).toBeTruthy();
  const user = await (await page.request.get("/api/user", { headers: { Authorization: `Bearer ${token}` } })).json();
  return { user, headers: { Authorization: `Bearer ${token}` } };
}
async function json(page: Page, method: "get" | "post" | "put" | "delete", path: string, headers: Record<string, string>, data?: unknown) {
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

test("a reported regression becomes a challenged, governed, lasting recovery", async ({ browser }) => {
  test.setTimeout(300_000);
  test.skip(!(await run("docker", ["info"]).then(() => true, () => false)), "bounded historical attempts require Docker");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copy = await mkdtemp(join(tmpdir(), "vivarium-regression-"));
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const reporterPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, suffix, "Maintainer");
    const reporter = await account(reporterPage, suffix, "Reporter");
    const repository = await json(ownerPage, "post", "/repositories", owner.headers, { name: `merge-regression-${suffix}` });
    await json(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: reporter.user.id });
    const credential = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "regression journey", scopes: ["git:read", "git:write"], expires_in: 3600 });
    await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${repository.id}.git`, copy);
    await git(copy, "config", "user.name", "Regression Maintainer"); await git(copy, "config", "user.email", "owner@example.test");
    await mkdir(join(copy, ".vivarium"));
    await writeFile(join(copy, ".vivarium", "checks.json"), JSON.stringify({ version: 1, checks: [{ name: "preserved checkout", image: "alpine:3.22", command: "grep -q 'mode=single' behavior.txt" }] }));
    await writeFile(join(copy, "behavior.txt"), "mode=single\nintent=validate\n");
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Release single checkout"); await git(copy, "push", "origin", "main");
    const good = await git(copy, "rev-parse", "HEAD");
    await writeFile(join(copy, "notes.txt"), "suspected timeout change\n"); await git(copy, "add", "."); await git(copy, "commit", "-m", "Tune timeout");
    const falseCulprit = await git(copy, "rev-parse", "HEAD"); await git(copy, "push", "origin", "main");
    await git(copy, "switch", "-c", "validation-rework"); await writeFile(join(copy, "behavior.txt"), "mode=double\nintent=validate\n"); await git(copy, "add", "."); await git(copy, "commit", "-m", "Refactor checkout validation");
    const introducing = await git(copy, "rev-parse", "HEAD"); await git(copy, "push", "origin", "validation-rework");
    await git(copy, "switch", "main"); await writeFile(join(copy, "rollout.txt"), "cohort=all\n"); await git(copy, "add", "."); await git(copy, "commit", "-m", "Expand checkout rollout"); await git(copy, "merge", "--no-ff", "validation-rework", "-m", "Merge validation rework"); await git(copy, "push", "origin", "main");
    const bad = await git(copy, "rev-parse", "HEAD");
    const affectedRelease = await json(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: "v2.0.0", notes: "Affected checkout rollout", commit_id: bad });
    const issue = await json(reporterPage, "post", `/repositories/${repository.id}/issues`, reporter.headers, { release_id: affectedRelease.id, title: "Checkout submits twice after v2", expected_behavior: "One checkout is retained", observed_behavior: "Two checkouts are retained", severity: "high", environment: "production", reproduction_steps: ["Submit one synthetic checkout"], visibility: "repository" });

    let investigation = await json(reporterPage, "post", `/repositories/${repository.id}/regression-investigations`, reporter.headers, {
      request_id: `report-${suffix}`, title: "Checkout used to submit once", source: { kind: "issue", resource_id: issue.id, revision: bad, label: issue.title },
      expected_behavior: "One checkout is retained while validation remains enabled", regressed_behavior: "One action retains two checkouts", known_good: { kind: "commit", resource_id: repository.id, revision: good, label: "v1 behavior" }, known_bad: { kind: "release", resource_id: affectedRelease.id, revision: bad, label: "v2 rollout" },
      affected_environments: ["production"], severity: "high", owner_ids: [owner.user.id], acceptance_criteria: ["One action retains one checkout", "Validation remains enabled"], evidence: [{ kind: "issue", resource_id: issue.id, revision: bad, label: "User impact", visibility: "repository" }], status: "open",
    });
    investigation = await json(ownerPage, "post", `/repositories/${repository.id}/regression-investigations/${investigation.id}/scenarios`, owner.headers, { expected_version: investigation.version, scenario: { name: "Preserve one validated checkout", environment: { image: "alpine:3.22", working_directory: ".", command: "grep -q 'mode=single' behavior.txt && grep -q 'intent=validate' behavior.txt", timeout_seconds: 30, cpus: 1, memory_mb: 128, storage_mb: 64 }, environment_variants: [{ revision: introducing, environment: { image: "missing-history-image:0", working_directory: ".", command: "true", timeout_seconds: 30, cpus: 1, memory_mb: 128, storage_mb: 64 } }], inputs: [], acceptance_criteria: ["One action retains one checkout", "Validation remains enabled"] } });
    const scenario = investigation.scenarios[0];
    async function attempt(revision: string, requestID: string) {
      investigation = await json(ownerPage, "post", `/repositories/${repository.id}/regression-investigations/${investigation.id}/scenarios/${scenario.id}/attempts`, owner.headers, { expected_version: investigation.version, request_id: requestID, target_kind: "commit", revision, dependencies: [], repeats: 1 });
      return investigation.attempts.find((item: any) => item.request_id === requestID);
    }
    expect((await attempt(good, "known-good")).classification).toBe("passed");
    const falseAttempt = await attempt(falseCulprit, "false-culprit"); expect(falseAttempt.classification).toBe("passed");
    expect((await attempt(introducing, "unbuildable-history")).classification).toBe("incompatible_setup");
    const badAttempt = await attempt(bad, "known-bad"); expect(badAttempt.classification).toBe("failed");
    investigation = await json(ownerPage, "post", `/repositories/${repository.id}/regression-investigations/${investigation.id}/searches`, owner.headers, { expected_version: investigation.version, request_id: `search-${suffix}`, scenario_id: scenario.id, dependencies: [] });
    let search = investigation.searches[0];
    investigation = await json(reporterPage, "post", `/repositories/${repository.id}/regression-investigations/${investigation.id}/searches/${search.id}/guidance`, reporter.headers, { expected_version: investigation.version, expected_search_version: search.version, kind: "hypothesis", reason: "Initial timing correlation", claim: "The timeout tuning introduced the double submission", confidence: "medium", evidence_ids: [], attempt_ids: [falseAttempt.id], candidate_revisions: [falseCulprit] });
    search = investigation.searches[0];
    investigation = await json(ownerPage, "post", `/repositories/${repository.id}/regression-investigations/${investigation.id}/searches/${search.id}/guidance`, owner.headers, { expected_version: investigation.version, expected_search_version: search.version, kind: "classify", revision: falseCulprit, classification: "working", reason: "The preserved scenario passes, disproving the initial culprit" });
    search = investigation.searches[0];
    investigation = await json(ownerPage, "post", `/repositories/${repository.id}/regression-investigations/${investigation.id}/searches/${search.id}/guidance`, owner.headers, { expected_version: investigation.version, expected_search_version: search.version, kind: "classify", revision: introducing, classification: "flaky", reason: "Historical setup is unbuildable and an earlier midpoint signal was flaky; retain ambiguity" });
    search = investigation.searches[0];
    investigation = await json(ownerPage, "post", `/repositories/${repository.id}/regression-investigations/${investigation.id}/searches/${search.id}/guidance`, owner.headers, { expected_version: investigation.version, expected_search_version: search.version, kind: "hypothesis", reason: "Exact failed merge attempt plus user impact", claim: "The merge first exposes the validation rework under the full rollout", confidence: "high", evidence_ids: [investigation.evidence[0].id], attempt_ids: [badAttempt.id], candidate_revisions: [bad] });
    search = investigation.searches[0];
    const shared = { benefits: ["Limits duplicate checkout impact"], risks: ["May delay the rollout"], constraints: ["Preserve validation intent"], affected_release_ids: [affectedRelease.id], affected_pull_ids: [], backport_targets: ["v2.x"] };
    investigation = await json(ownerPage, "post", `/repositories/${repository.id}/regression-investigations/${investigation.id}/responses`, owner.headers, { expected_version: investigation.version, request_id: `response-${suffix}`, search_id: search.id, scenario_id: scenario.id, options: [{ ...shared, kind: "revert", summary: "Revert failed because it also removed required validation" }, { ...shared, kind: "containment", summary: "Narrow the production rollout while repair is reviewed" }, { ...shared, kind: "dependency_adjustment", summary: "Pin the unaffected checkout dependency" }, { ...shared, kind: "forward_repair", summary: "Keep validation and restore single submission" }] });
    const response = investigation.responses[0];
    investigation = await json(ownerPage, "post", `/repositories/${repository.id}/regression-investigations/${investigation.id}/responses/${response.id}/publish`, owner.headers, { expected_version: investigation.version, selected_kind: "forward_repair", rationale: "The attempted revert violated preserved validation intent; contain rollout and repair forward", title: "Recover validated checkout", work: [{ title: "Forward repair and v2 backport", outcome: "Restore one validated checkout in main and v2.x", assignee_type: "agent", assignee_id: "a".repeat(32), acceptance_criteria: ["Original scenario passes on both targets", "Retain as required regression coverage"] }] });
    expect(investigation.responses[0]).toMatchObject({ selected_kind: "forward_repair", task_ids: [expect.any(String)] });

    // Revoked evidence access stops further agent-authored reasoning without deleting its retained work.
    const taskList = await json(ownerPage, "get", `/repositories/${repository.id}/proposals/${investigation.responses[0].proposal_id}/tasks`, owner.headers);
    const responseTask = taskList.tasks.find((item: any) => item.id === investigation.responses[0].task_ids[0]);
    const launched = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${investigation.responses[0].proposal_id}/tasks/${responseTask.id}/sessions`, owner.headers, { expected_assignment_id: responseTask.assignment.id, context_paths: ["behavior.txt"], expires_in: 600 });
    await json(ownerPage, "delete", `/auth/credentials/${launched.credential.id}`, owner.headers);
    await rejected(await ownerPage.request.post(`/api/repositories/${repository.id}/regression-investigations/${investigation.id}/events`, { headers: { Authorization: `Bearer ${launched.credential.token}` }, data: { expected_version: investigation.version, kind: "discussion", message: "continue after revocation" } }), 401, "unauthorized");

    await ownerPage.goto(`/repositories/${repository.id}/regressions`);
    await expect(ownerPage.getByRole("heading", { name: "Regression investigations" })).toBeVisible();
    await expect(ownerPage.getByText("Checkout used to submit once", { exact: true }).first()).toBeVisible();
    await ownerPage.getByText("Checkout used to submit once", { exact: true }).first().click();
    await expect(ownerPage.getByText("Revert failed because it also removed required validation", { exact: true })).toBeVisible();
    await expect(ownerPage.getByText("published", { exact: true })).toBeVisible();
    await json(ownerPage, "delete", `/auth/credentials/${credential.id}`, owner.headers);
  } finally {
    await rm(copy, { recursive: true, force: true });
  }
});
