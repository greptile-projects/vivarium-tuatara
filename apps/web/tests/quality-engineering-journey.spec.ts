import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- one connected retained trail crosses public resources */

const run = promisify(execFile);
const sha = (value: string) => createHash("sha256").update(value).digest("hex");
async function git(cwd: string, ...args: string[]) {
  return (await run("git", args, { cwd, env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } })).stdout.trim();
}
async function account(page: Page, suffix: string, role: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(`${role} collaborator`);
  await page.getByLabel("Handle").fill(`quality-${role.toLowerCase()}-${suffix}`);
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

test("a team turns cross-platform intent and exploration into sustained release quality", async ({ browser }) => {
  test.setTimeout(300_000);
  const dockerAvailable = await run("docker", ["info"]).then(() => true, () => false);
  test.skip(!dockerAvailable, "the quality journey requires bounded check and release workspaces");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copy = await mkdtemp(join(tmpdir(), "vivarium-quality-"));
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const testerPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, suffix, "Owner");
    const tester = await account(testerPage, suffix, "Tester");
    const repository = await json(ownerPage, "post", "/repositories", owner.headers, { name: `cross-platform-checkout-${suffix}` });
    await json(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: tester.user.id });
    const credential = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "quality journey", scopes: ["git:read", "git:write"], expires_in: 3600 });
    await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${repository.id}.git`, copy);
    await git(copy, "config", "user.name", "Quality Owner"); await git(copy, "config", "user.email", "quality@example.test");
    await mkdir(join(copy, ".vivarium")); await mkdir(join(copy, "fixtures")); await mkdir(join(copy, "tests"));
    const command = "grep -q 'edge=handled' checkout.txt";
    await writeFile(join(copy, ".vivarium", "checks.json"), JSON.stringify({ version: 1, checks: [{ name: "cross-platform checkout", image: "alpine:3.22", command }] }));
    await writeFile(join(copy, ".vivarium", "release.json"), JSON.stringify({ version: 1, steps: [{ name: "package", image: "alpine:3.22", command: "cp checkout.txt \"$VIVARIUM_OUTPUT/checkout.txt\"" }] }));
    await writeFile(join(copy, "checkout.txt"), "edge=pending\n");
    await writeFile(join(copy, "fixtures", "checkout.json"), '{"authorization":"Bearer unsafe-token-value-1234567890"}\n');
    await writeFile(join(copy, "tests", "checkout.spec"), "cross-platform checkout intent\n");
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Add cross-platform checkout candidate"); await git(copy, "push", "origin", "main");
    const base = await git(copy, "rev-parse", "HEAD");
    await git(copy, "switch", "-c", "repair/checkout-edge");
    await writeFile(join(copy, "checkout.txt"), "edge=first-repair\n"); await git(copy, "add", "checkout.txt"); await git(copy, "commit", "-m", "Attempt checkout edge repair"); await git(copy, "push", "origin", "repair/checkout-edge");
    const firstCandidate = await git(copy, "rev-parse", "HEAD");
    let pull = await json(testerPage, "post", `/repositories/${repository.id}/pulls`, tester.headers, { title: "Explore narrow checkout edge", body: "Candidate derived from product and design intent.", source_branch: "repair/checkout-edge", target_branch: "main" });
    await json(ownerPage, "put", `/repositories/${repository.id}/branches/main/required-checks`, owner.headers, { checks: ["cross-platform checkout"] });

    const plan = await json(ownerPage, "post", `/repositories/${repository.id}/quality-plans`, owner.headers, { revision: {
      title: "Cross-platform checkout quality", summary: "Protect keyboard and touch checkout across supported clients.",
      scopes: [{ kind: "repository", name: "checkout repository" }, { kind: "journey", resource_id: "checkout-submit", name: "Submit checkout", source_revision: firstCandidate }, { kind: "interface", resource_id: "responsive-checkout", name: "Responsive checkout" }],
      supported_environments: [{ id: "desktop", name: "Desktop Chromium", description: "Keyboard journey", supported: true }, { id: "mobile", name: "Mobile WebKit", description: "Touch journey", supported: true }],
      requirements: [{ id: "checkout", source_kind: "design", source_id: "responsive-checkout", title: "Submit once", rationale: "Product intent requires one attributable order", expected_behavior: "Keyboard and touch users submit exactly once", risk: "high", test_levels: ["end_to_end", "exploratory"], representative_data: "Synthetic order with narrow viewport", coverage_goal: "Both supported platforms and the double-submit edge", owner_ids: [owner.user.id], judge_ids: [tester.user.id], environment_ids: ["desktop", "mobile"], schedule: "Every pull and release", release_threshold: "Both platforms and exploratory sign-off pass", evidence_ids: [] }],
      evidence: [], exceptions: [], owner_ids: [owner.user.id, tester.user.id], review_schedule: "Review on every checkout change", rationale: "Connect reviewed product and design intent to delivered evidence"
    }});
    const scenarioInput = (commit: string, fixtureDigest: string, pullID = pull.id) => ({
      title: "Submit checkout once", purpose: "Protect the reviewed cross-platform journey", quality_plan_id: plan.id, quality_plan_version: 1, requirement_ids: ["checkout"],
      sources: [{ kind: "user_journey", resource_id: "checkout-submit", revision: firstCandidate, summary: "Reviewed checkout journey" }],
      parameters: [{ name: "platform", description: "Supported client", type: "enum", required: true, example: "desktop" }],
      preconditions: [{ id: "ready", description: "Synthetic checkout is ready", operation: "open checkout", parameters: ["platform"] }], actions: [{ id: "submit", description: "Submit once", operation: "activate checkout", parameters: ["platform"] }],
      assertions: [{ id: "once", description: "One order is retained", matcher: "contains", expected: "edge=handled" }],
      fixtures: [{ id: "order", kind: "synthetic", description: "Synthetic checkout", path: "fixtures/checkout.json", sha256: fixtureDigest, data_class: "synthetic", assumptions: ["No production identity"] }],
      environments: [{ id: "desktop", description: "Keyboard desktop", runtime: "chromium", requirements: ["keyboard"] }, { id: "mobile", description: "Touch mobile", runtime: "webkit", requirements: ["touch"] }],
      cases: [{ id: "desktop", name: "Desktop checkout", values: { platform: "desktop" }, assumptions: ["Keyboard available"], expected_outcome: "One order" }, { id: "mobile", name: "Mobile checkout", values: { platform: "mobile" }, assumptions: ["Narrow touch viewport"], expected_outcome: "One order" }],
      implementation: { authored_by_type: "agent", branch: pullID ? "repair/checkout-edge" : "main", commit_id: commit, pull_request_id: pullID, test_paths: ["tests/checkout.spec"], command, framework: "shell", generated: true, assumptions: ["Synthetic fixture represents the edge"], provenance: ["Quality plan checkout requirement", "Human-reviewed product and design intent"] }
    });
    await rejected(await ownerPage.request.post(`/api/repositories/${repository.id}/test-scenarios`, { headers: owner.headers, data: scenarioInput(firstCandidate, sha('{"authorization":"Bearer unsafe-token-value-1234567890"}\n')) }), 422, "test_scenario_provenance_invalid");

    const proposal = await json(ownerPage, "post", `/repositories/${repository.id}/proposals`, owner.headers, { title: "Explore checkout candidate", body: "Bound one agent to the exact candidate and synthetic data." });
    const task = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${proposal.id}/tasks`, owner.headers, { title: "Explore narrow checkout", outcome: "Retain attributable uncertainty and a minimized reproduction." });
    const agentID = "a".repeat(32);
    const assignment = await json(ownerPage, "put", `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/assignment`, owner.headers, { assignee_type: "agent", assignee_id: agentID, mandate: "Explore only the checkout edge and synthetic fixture.", repository_id: repository.id, base_revision: base });
    const launched = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/sessions`, owner.headers, { expected_assignment_id: assignment.assignment.id, context_paths: ["checkout.txt", "fixtures/checkout.json"], expires_in: 3600 });
    const agentHeaders = { Authorization: `Bearer ${launched.credential.token}` };
    expect(launched.run.agent_id).toBe(agentID);
    const sessionInput = (revision: string, title: string) => ({ title, source: { kind: "pull_preview", resource_id: pull.id, revision, label: "Exact checkout pull preview" }, access: [owner.user.id, tester.user.id], limits: { expires_at: new Date(Date.now() + 3_600_000).toISOString(), max_cost_cents: 100, max_agent_actions: 4, allowed_actions: ["navigate", "input", "observe", "reproduce", "classify", "signoff", "close", "command", "trace"], test_data: ["synthetic"] }, charters: [{ id: "tester", title: "Human cross-platform charter", risk: "high", mission: "Judge supported platforms and finding decisions", assignee_type: "human", assignee_id: tester.user.id, allowed_actions: ["navigate", "input", "observe", "reproduce", "classify", "signoff", "close"], coverage: ["desktop", "mobile"], uncertainty: "Touch timing may vary" }, { id: "agent", title: "Agent edge charter", risk: "high", mission: "Minimize only the double-submit edge", assignee_type: "agent", assignee_id: agentID, allowed_actions: ["navigate", "input", "observe", "reproduce", "command", "trace"], coverage: ["narrow viewport"], uncertainty: "Synthetic timing differs from devices" }] });
    let staleSession = await json(testerPage, "post", `/repositories/${repository.id}/exploratory-sessions`, tester.headers, sessionInput(firstCandidate, "Initial checkout exploration"));
    staleSession = await json(ownerPage, "post", `/repositories/${repository.id}/exploratory-sessions/${staleSession.id}/events`, agentHeaders, { expected_version: staleSession.version, kind: "observation", charter_id: "agent", finding_id: "double-submit", summary: "Agent observed duplicate activation at the narrow boundary", route: "/checkout", inputs: ["synthetic narrow order"], command: "test narrow viewport", coverage: ["mobile edge"], uncertainty: "Timing is synthetic" });

    const safeFixture = '{"customer":"synthetic-42","amount":12}\n';
    await writeFile(join(copy, "fixtures", "checkout.json"), safeFixture); await git(copy, "add", "fixtures/checkout.json"); await git(copy, "commit", "-m", "Replace unsafe checkout fixture"); await git(copy, "push", "origin", "repair/checkout-edge");
    const repairCandidate = await git(copy, "rev-parse", "HEAD");
    await json(testerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/synchronize`, tester.headers, {});
    staleSession = await json(testerPage, "get", `/repositories/${repository.id}/exploratory-sessions/${staleSession.id}`, tester.headers);
    expect(staleSession).toMatchObject({ stale: true });

    let session = await json(testerPage, "post", `/repositories/${repository.id}/exploratory-sessions`, tester.headers, sessionInput(repairCandidate, "Current checkout exploration"));
    session = await json(ownerPage, "post", `/repositories/${repository.id}/exploratory-sessions/${session.id}/events`, agentHeaders, { expected_version: session.version, kind: "observation", charter_id: "agent", finding_id: "double-submit", summary: "Agent found the remaining narrow double-submit edge", route: "/checkout", inputs: ["synthetic order"], coverage: ["mobile"], uncertainty: "Device scheduling remains uncertain" });
    const observed = session.events.at(-1);
    session = await json(testerPage, "post", `/repositories/${repository.id}/exploratory-sessions/${session.id}/events`, tester.headers, { expected_version: session.version, kind: "observation", charter_id: "tester", finding_id: "intermittent-animation", summary: "One animation assertion varied without changing checkout behavior", route: "/checkout", coverage: ["desktop"] });
    session = await json(testerPage, "post", `/repositories/${repository.id}/exploratory-sessions/${session.id}/events`, tester.headers, { expected_version: session.version, kind: "classify", charter_id: "tester", finding_id: "intermittent-animation", classification: "flaky", summary: "Contain the flaky animation assertion outside the release signal" });
    session = await json(testerPage, "post", `/repositories/${repository.id}/exploratory-sessions/${session.id}/events`, tester.headers, { expected_version: session.version, kind: "reproduce", charter_id: "tester", finding_id: "double-submit", reproduces_event_id: observed.id, summary: "Tester minimized the edge to one narrow synthetic order", route: "/checkout", inputs: ["width=320", "single activation"], coverage: ["mobile"] });
    const reproduction = session.events.at(-1);
    session = await json(testerPage, "post", `/repositories/${repository.id}/exploratory-sessions/${session.id}/events`, tester.headers, { expected_version: session.version, kind: "classify", charter_id: "tester", finding_id: "double-submit", classification: "bug", summary: "Confirmed cross-platform checkout bug" });
    const repaired = await json(testerPage, "post", `/repositories/${repository.id}/exploratory-sessions/${session.id}/findings/double-submit/repair`, tester.headers, { expected_version: session.version, title: "Narrow checkout submits twice", expected_behavior: "Every supported client retains one order", severity: "high", environment: "Mobile WebKit", evidence_event_ids: [observed.id, reproduction.id], reproduction_event_id: reproduction.id, acceptance_criteria: ["The minimized edge passes on desktop and mobile", "A reusable scenario remains linked"], assignee_type: "human", assignee_id: tester.user.id, quality_plan_id: plan.id, quality_plan_version: 1, requirement_ids: ["checkout"] });
    session = repaired.session;
    pull = await json(testerPage, "post", `/repositories/${repository.id}/proposals/${repaired.repair.proposal_id}/tasks/${repaired.repair.task_id}/contributions`, tester.headers, { title: "Repair narrow checkout edge", body: "The governed repair retains the minimized finding and regression criteria.", source_branch: "repair/checkout-edge", target_branch: "main" });

    let checks = await eventually(() => json(testerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}/checks`, tester.headers), (x: any) => x.check_runs?.some((c: any) => c.commit_id === repairCandidate && c.state === "failed"), "failed first repair is retained");
    await writeFile(join(copy, "checkout.txt"), "edge=handled\n"); await git(copy, "add", "checkout.txt"); await git(copy, "commit", "-m", "Correct narrow checkout edge"); await git(copy, "push", "origin", "repair/checkout-edge");
    const corrected = await git(copy, "rev-parse", "HEAD");
    await json(testerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/synchronize`, tester.headers, {});
    checks = await eventually(() => json(testerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}/checks`, tester.headers), (x: any) => x.check_runs?.some((c: any) => c.commit_id === corrected && c.state === "succeeded"), "corrected repair passes");
    const passingRun = checks.check_runs.find((c: any) => c.commit_id === corrected && c.state === "succeeded");
    const regressionInput = scenarioInput(corrected, sha(safeFixture));
    regressionInput.sources.push({ kind: "issue", resource_id: repaired.repair.issue_id, revision: repairCandidate, summary: "Confirmed minimized exploratory finding" });
    const scenario = await json(testerPage, "post", `/repositories/${repository.id}/test-scenarios`, tester.headers, regressionInput);
    const covered = await json(testerPage, "post", `/repositories/${repository.id}/exploratory-sessions/${session.id}/findings/double-submit/coverage`, tester.headers, { expected_version: session.version, scenario_id: scenario.id });
    session = covered.session;

    let deliverySession = await json(testerPage, "post", `/repositories/${repository.id}/exploratory-sessions`, tester.headers, sessionInput(corrected, "Corrected checkout sign-off"));
    await json(ownerPage, "post", `/repositories/${repository.id}/quality-requirements`, owner.headers, { expected_version: 0, requirements: [{ id: "desktop", title: "Desktop scenario", kind: "scenario", resource_id: scenario.id, owner_ids: [owner.user.id], selector: { branches: ["main"], journeys: ["checkout"], platforms: ["desktop"], paths: ["checkout.txt"] } }, { id: "mobile", title: "Mobile scenario", kind: "scenario", resource_id: scenario.id, owner_ids: [owner.user.id], selector: { branches: ["main"], journeys: ["checkout"], platforms: ["mobile"], paths: ["checkout.txt"] } }, { id: "explore", title: "Exploratory sign-off", kind: "exploratory_signoff", resource_id: deliverySession.id, owner_ids: [tester.user.id], selector: { branches: ["main"], journeys: ["checkout"] } }] });
    const attempt = (requirement: string, platform: string) => ({ requirement_id: requirement, revision: corrected, status: "passed", scenario_id: scenario.id, check_run_id: passingRun.id, pull_request_id: pull.id, target_kind: "pull", target_id: pull.id, environment: platform, journey: "checkout", risk_class: "high", platform, affected_paths: ["checkout.txt"], summary: `${platform} retained execution` });
    await json(testerPage, "post", `/repositories/${repository.id}/quality-attempts`, tester.headers, attempt("desktop", "desktop"));
    let matrix = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}/quality-confidence`, owner.headers);
    expect(matrix.ready).toBe(false);
    expect(matrix.requirements.find((cell: any) => cell.requirement.id === "mobile").state).toBe("gap");
    const followUp = await json(testerPage, "post", `/repositories/${repository.id}/issues`, tester.headers, { title: "Run missing mobile matrix", expected_behavior: "Mobile evidence is current", observed_behavior: "The first matrix retained a gap", severity: "medium", environment: "Mobile WebKit", reproduction_steps: ["Inspect pull quality matrix"], visibility: "repository" });
    await rejected(await ownerPage.request.post(`/api/repositories/${repository.id}/quality-overrides`, { headers: owner.headers, data: { requirement_id: "mobile", revision: corrected, rationale: "Ship without mobile evidence", scope: { branches: ["main"] }, expires_at: new Date(Date.now() + 31 * 86400000).toISOString(), follow_up_kind: "issue", follow_up_id: followUp.id } }), 422, "quality_confidence_invalid");
    await json(testerPage, "post", `/repositories/${repository.id}/quality-attempts`, tester.headers, attempt("mobile", "mobile"));
    deliverySession = await json(testerPage, "post", `/repositories/${repository.id}/exploratory-sessions/${deliverySession.id}/events`, tester.headers, { expected_version: deliverySession.version, kind: "signoff", charter_id: "tester", summary: "Tester signs off the corrected desktop and mobile candidate" });
    const signoff = deliverySession.events.at(-1);
    deliverySession = await json(testerPage, "post", `/repositories/${repository.id}/exploratory-sessions/${deliverySession.id}/events`, tester.headers, { expected_version: deliverySession.version, kind: "close", charter_id: "tester", summary: "Close exact corrected exploration" });
    await json(testerPage, "post", `/repositories/${repository.id}/quality-attempts`, tester.headers, { requirement_id: "explore", revision: corrected, status: "passed", exploratory_session_id: deliverySession.id, signoff_event_id: signoff.id, target_kind: "pull", target_id: pull.id, environment: "preview", journey: "checkout", risk_class: "high", summary: "Chartered human sign-off" });
    matrix = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}/quality-confidence`, owner.headers);
    expect(matrix.ready).toBe(true);
    await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/reviews`, owner.headers, { decision: "approved" });
    const merged = await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/merge`, owner.headers, {});
    checks = await eventually(() => json(ownerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}/checks`, owner.headers), (x: any) => x.check_runs?.some((c: any) => c.commit_id === merged.merge_commit_id && c.state === "succeeded"), "merge check passes");
    const release = await json(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: "v1.0.0", notes: "Cross-platform checkout quality", commit_id: merged.merge_commit_id });
    expect(release.commit_id).toBe(merged.merge_commit_id);
    const releaseRun = checks.check_runs.find((c: any) => c.commit_id === merged.merge_commit_id && c.state === "succeeded");
    const releaseScenario = await json(ownerPage, "post", `/repositories/${repository.id}/test-scenarios`, owner.headers, scenarioInput(merged.merge_commit_id, sha(safeFixture), ""));
    await json(ownerPage, "post", `/repositories/${repository.id}/quality-requirements`, owner.headers, { expected_version: 1, requirements: [{ id: "released", title: "Released checkout sample", kind: "scenario", resource_id: releaseScenario.id, owner_ids: [owner.user.id], selector: { releases: ["v1.0.0"], journeys: ["checkout"], platforms: ["mobile"], paths: ["checkout.txt"] } }] });
    await json(ownerPage, "post", `/repositories/${repository.id}/releases/${release.id}/quality-signals`, owner.headers, { requirement_id: "released", scenario_id: releaseScenario.id, check_run_id: releaseRun.id, pull_request_id: pull.id, environment: "production-mobile", journey: "checkout", risk_class: "high", platform: "mobile", affected_paths: ["checkout.txt"], summary: "Sampled released checkout retained exactly one order" });
    const releaseMatrix = await json(ownerPage, "get", `/repositories/${repository.id}/releases/${release.id}/quality-confidence`, owner.headers);
    expect(releaseMatrix).toMatchObject({ ready: true, requirements: [expect.objectContaining({ state: "passed" })] });

    await ownerPage.goto(`/repositories/${repository.id}/quality`);
    await expect(ownerPage.getByRole("heading", { name: "Quality plans", exact: true })).toBeVisible();
    await expect(ownerPage.getByText("Cross-platform checkout quality", { exact: true }).first()).toBeVisible();
    await expect(ownerPage.getByText("Submit checkout once", { exact: true }).first()).toBeVisible();
    await expect(ownerPage.getByText("Current checkout exploration", { exact: true })).toBeVisible();
    expect(session.repairs[0]).toMatchObject({ scenario_id: scenario.id, pull_request_id: pull.id, regression_commit_id: corrected });
  } finally { await rm(copy, { recursive: true, force: true }); }
});
