import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- one connected cross-surface evidence trail */
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
  return { headers, user: await (await page.request.get("/api/user", { headers })).json() as any };
}
async function json(page: Page, method: "get" | "post" | "put", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}
async function rejected(page: Page, path: string, headers: Record<string, string>, data: unknown) {
  const response = await page.request.post(`/api${path}`, { headers, data });
  expect(response.status(), `POST ${path}: ${await response.text()}`).toBeGreaterThanOrEqual(400);
  return response;
}
const profile = (summary: string, pricing: string, model = "Acme Repair 1") => ({
  summary, supported_tasks: ["repair dependency validation"], tools: ["git", "bounded checks"],
  model_provenance: model, execution_provenance: "Project-owned isolated task session",
  deployment_boundaries: ["platform"], data_use: "Exact task context only; no training",
  retention: "Deleted after 24 hours", pricing, resource_requirements: ["one CPU"],
  requested_capabilities: ["repository.read", "repository.write"], availability: "24x5",
  support: "operator@example.test", subprocessors: [], remote_execution_boundaries: ["none"],
  conflict_disclosures: ["Operator is paid per completed run"], change_summary: summary,
});

test("a project evaluates, adopts, delivers with, and replaces an unfamiliar agent", async ({ browser }) => {
  test.setTimeout(360_000);
  const docker = await run("docker", ["info"]).then(() => true, () => false);
  test.skip(!docker, "the connected journey requires the Docker-backed project check executor");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copies: string[] = [];
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const operatorPage = await (await browser.newContext()).newPage();
    const reviewerPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, "Adoption Owner", `adoption-owner-${suffix}`);
    const operator = await account(operatorPage, "Independent Operator", `agent-operator-${suffix}`);
    const reviewer = await account(reviewerPage, "Independent Reviewer", `agent-reviewer-${suffix}`);

    const organization = await json(ownerPage, "post", "/organizations", owner.headers, { name: `Agent Adoption ${suffix}`, slug: `agent-adoption-${suffix}` });
    for (const person of [operator, reviewer]) {
      await json(ownerPage, "post", `/organizations/${organization.id}/invitations`, owner.headers, { user_id: person.user.id });
      const listed = await json(person === operator ? operatorPage : reviewerPage, "get", "/organizations", person.headers);
      const invitation = listed.organizations.find((x: any) => x.id === organization.id).invitations.at(-1);
      await json(person === operator ? operatorPage : reviewerPage, "post", `/organizations/${organization.id}/invitations/${invitation.id}/accept`, person.headers);
    }
    const repository = await json(ownerPage, "post", `/organizations/${organization.id}/repositories`, owner.headers, { name: `validator-${suffix}` });
    const gitToken = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "adoption baseline", scopes: ["git:read", "git:write"], expires_in: 3600 });
    const baseline = await mkdtemp(join(tmpdir(), "vivarium-adoption-")); copies.push(baseline);
    await git(tmpdir(), "clone", `http://git:${gitToken.token}@localhost:3000/git/${repository.id}.git`, baseline);
    await git(baseline, "config", "user.name", "Adoption Owner"); await git(baseline, "config", "user.email", "owner@example.test");
    await mkdir(join(baseline, ".vivarium"));
    await writeFile(join(baseline, ".vivarium", "checks.json"), JSON.stringify({ version: 1, checks: [{ name: "validator tests", image: "alpine:3.22", command: "grep -qx 'validation=strict' validator.conf" }] }));
    await writeFile(join(baseline, "validator.conf"), "validation=permissive\n");
    await git(baseline, "add", "."); await git(baseline, "commit", "-m", "Establish validator baseline"); await git(baseline, "push", "origin", "main");
    const base = await git(baseline, "rev-parse", "HEAD");

    const publishAgent = async (name: string, slug: string, pricing: string) => {
      let state = await json(ownerPage, "post", `/organizations/${organization.id}/agents`, owner.headers, { name, slug, description: "Specialized dependency validation agent", visibility: "public", capabilities: ["repair dependency validation"], operator_ids: [owner.user.id, operator.user.id], team_ids: [] });
      const agent = state.agents.at(-1);
      state = await json(ownerPage, "put", `/organizations/${organization.id}/agents/${agent.id}/profile`, owner.headers, { expected_version: 0, profile: profile(`${name} initial disclosure`, pricing) });
      return state.agents.find((x: any) => x.id === agent.id);
    };
    const candidate = await publishAgent("Dependency Medic", `dependency-medic-${suffix}`, "$2 per run");
    const alternate = await publishAgent("Dependency Scout", `dependency-scout-${suffix}`, "$5 per run");

    const matches = await json(ownerPage, "post", `/organizations/${organization.id}/agent-matches`, owner.headers, { source_kind: "team_role", source_id: `delivery-validator-${suffix}`, repository_id: repository.id, workflow: "repair dependency validation", required_permissions: [], deployment_boundary: "platform" });
    expect(matches.matches.map((x: any) => x.agent_id)).toEqual(expect.arrayContaining([candidate.id, alternate.id]));
    expect(matches.matches.find((x: any) => x.agent_id === candidate.id)).toMatchObject({ pricing: "$2 per run", eligible: true });
    await ownerPage.goto(`/organizations/${organization.id}`);
    await expect(ownerPage.getByRole("heading", { name: "Published agent profiles" })).toBeVisible();
    await expect(ownerPage.getByRole("heading", { name: "Dependency Medic" }).first()).toBeVisible();

    const suite = await json(ownerPage, "post", `/organizations/${organization.id}/agent-evaluation-suites`, owner.headers, {
      name: "Dependency repair trial", repository_id: repository.id, expected_version: 0,
      revision: { repository_revision: base, scenarios: [{ id: "repair", title: "Tighten validation", sanitized_prompt: "Set project validation to strict.", expected_outcomes: ["strict configuration"], checks: [{ name: "public outcome", kind: "contains", expected: "strict" }], hidden_checks: [{ name: "held-back regression", kind: "canary", expected: "bypass" }] }], budget: { max_cost: 3, max_latency_ms: 5000, max_tool_actions: 3 }, prohibited_actions: ["publish", "merge", "secrets", "network"], human_review_criteria: ["minimal exact change"], change_summary: "Initial project-owned trial" },
    });
    const trial = (agentID: string, output: string, extra: any = {}) => json(ownerPage, "post", `/organizations/${organization.id}/agent-evaluation-suites/${suite.id}/runs`, owner.headers, { suite_version: 1, trial: { agent_id: agentID, agent_profile_version: 1, outputs: { repair: output }, cost: 2, latency_ms: 900, ...extra } });
    const hiddenFailure = await trial(candidate.id, "strict bypass");
    const evaluatorHiddenFailure = (await json(ownerPage, "get", `/organizations/${organization.id}/agent-evaluation-runs`, owner.headers)).runs.find((x: any) => x.id === hiddenFailure.id);
    expect(evaluatorHiddenFailure).toMatchObject({ correctness_passed: false, contaminated: true });
    expect(hiddenFailure.check_results.some((x: any) => x.hidden)).toBe(false);
    const prohibited = await trial(candidate.id, "strict", { tool_actions: [{ tool: "git", action: "publish", input_summary: "branch", output_summary: "blocked", duration_ms: 5, failed: true }] });
    expect(prohibited).toMatchObject({ policy_passed: false, authority: { publish: false, secrets: false, merge: false, environments: false, network: false } });
    const overrun = await trial(alternate.id, "strict", { cost: 9 });
    expect(overrun.budget_passed).toBe(false);
    const clean = await trial(candidate.id, "strict configuration");
    const reproduced = await trial(candidate.id, "strict configuration", { reproduces_run_id: clean.id });
    expect(reproduced.reproducible).toBe(true);
    const approved = await json(ownerPage, "post", `/organizations/${organization.id}/agent-evaluation-runs/${reproduced.id}/decisions`, owner.headers, { decision: "approved", rationale: "Independent reproduction passed public and held-back project checks within budget." });
    expect(approved.review_status).toBe("approved");

    let participation = await json(ownerPage, "post", `/organizations/${organization.id}/agent-participations`, owner.headers, { expected_version: 0, participation: { agent_id: candidate.id, agent_profile_version: 1, evaluation_run_id: reproduced.id, role: "contributor", resources: [{ kind: "repository", id: repository.id }], actions: ["repository.read", "repository.write"], budget: { max_cost: 12, max_agent_minutes: 30, max_actions: 4 }, starts_at: new Date(Date.now() - 1000).toISOString(), expires_at: new Date(Date.now() + 3_600_000).toISOString(), data_boundaries: ["repository_metadata", "repository_content"], policy_exception_ids: [], agreement_requirement: "sponsor", sponsor_id: reviewer.user.id, reevaluation_interval_days: 30 } });
    const preview = await json(ownerPage, "get", `/organizations/${organization.id}/agent-participations/${participation.id}/preview`, owner.headers);
    expect(preview).toMatchObject({ effective: false, would_create_access_grant: true, blockers: ["sponsor agreement required"] });
    participation = await json(reviewerPage, "post", `/organizations/${organization.id}/agent-participations/${participation.id}/agreement`, reviewer.headers, { expected_version: participation.version, statement: "I sponsor only the frozen validator task and budget." });
    participation = await json(ownerPage, "post", `/organizations/${organization.id}/agent-participations/${participation.id}/activate`, owner.headers, { expected_version: participation.version });
    expect(participation).toMatchObject({ status: "active", authority_identity: `agent-participation:${participation.id}` });
    await rejected(ownerPage, `/organizations/${organization.id}/access-grants/${participation.access_grant_id}/credentials`, owner.headers, { agent_id: candidate.id, repository_id: repository.id, expires_in: 3600, purpose: "environment_write" });

    const proposal = await json(ownerPage, "post", `/repositories/${repository.id}/proposals`, owner.headers, { title: "Tighten dependency validation", body: "Use the evaluated collaborator through ordinary task, session, review, checks, and merge boundaries." });
    const task = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${proposal.id}/tasks`, owner.headers, { title: "Make validation strict", outcome: "The project check proves strict validation." });
    const assignment = await json(ownerPage, "put", `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/assignment`, owner.headers, { assignee_type: "agent", assignee_id: candidate.id, mandate: "Change only validator.conf and satisfy the project check.", repository_id: repository.id, base_revision: base });
    const launched = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/sessions`, owner.headers, { expected_assignment_id: assignment.assignment.id, context_paths: ["validator.conf"], expires_in: 3600 });
    expect(launched.run.agent_id).toBe(candidate.id);
    const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-adopted-agent-")); copies.push(agentCopy);
    await git(tmpdir(), "clone", `http://git:${launched.credential.token}@localhost:3000/git/${repository.id}.git`, agentCopy);
    await git(agentCopy, "config", "user.name", "Dependency Medic"); await git(agentCopy, "config", "user.email", "medic@agents.vivarium");
    await git(agentCopy, "switch", "-c", "agent-work", `origin/${launched.run.working_branch}`);
    await writeFile(join(agentCopy, "validator.conf"), "validation=strict\n");
    await git(agentCopy, "add", "validator.conf"); await git(agentCopy, "commit", "-m", "Tighten dependency validation"); await git(agentCopy, "push", "origin", `HEAD:refs/heads/${launched.run.working_branch}`);
    const resultCommit = await git(agentCopy, "rev-parse", "HEAD");
    const sessionPath = `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/sessions/${launched.session.id}/runs/${launched.run.id}`;
    await json(ownerPage, "post", `${sessionPath}/completion`, { Authorization: `Bearer ${launched.credential.token}` }, { summary: "Changed only the bounded validator setting.", commit_id: resultCommit, checks: [{ name: "validator tests", status: "passed", details: "Exact strict setting present." }], unresolved_concerns: [] });
    const pull = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${proposal.id}/tasks/${task.id}/contributions`, owner.headers, { title: "Tighten dependency validation", body: "Evaluated-agent contribution for independent review.", source_branch: launched.run.working_branch, target_branch: "main", session_id: launched.session.id, run_id: launched.run.id });
    await reviewerPage.goto(`/pulls/${repository.id}/${pull.id}`);
    await expect(reviewerPage.getByText("Connected proposal task", { exact: true })).toBeVisible();
    await reviewerPage.getByRole("button", { name: "Approve" }).click();
    await ownerPage.goto(`/pulls/${repository.id}/${pull.id}`);
    await expect(ownerPage.locator("#checks").getByText("succeeded", { exact: true })).toBeVisible({ timeout: 60_000 });
    await ownerPage.getByRole("button", { name: "Merge into main" }).click();
    await expect(ownerPage.getByText("Merged", { exact: true })).toBeVisible();
    const merged = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}`, owner.headers);
    participation = await json(ownerPage, "post", `/organizations/${organization.id}/agent-participations/${participation.id}/outcomes`, owner.headers, { expected_version: participation.version, outcome: { kind: "accepted_contribution", repository_id: repository.id, status: "accepted", summary: "Strict dependency validation passed project checks and independent review.", attribution_id: merged.merge_commit_id, cost: 2.4, latency_ms: 1400 } });
    participation = await json(ownerPage, "post", `/organizations/${organization.id}/agent-participations/${participation.id}/controls`, owner.headers, { expected_version: participation.version, control: { action: "handoff", to_agent_id: alternate.id, scope: "dependency validation role", work_summary: "Transfer the retained suite, reviewed merge, and remaining reevaluation without credentials.", evidence_ids: [suite.id, reproduced.id, pull.id, merged.merge_commit_id] } });
    expect(participation).toMatchObject({ status: "suspended", handoffs: [expect.objectContaining({ from_agent_id: candidate.id, to_agent_id: alternate.id })] });

    const orgState = await json(ownerPage, "put", `/organizations/${organization.id}/agents/${candidate.id}/profile`, owner.headers, { expected_version: 1, profile: profile("Model and price upgrade", "$4 per run", "Acme Repair 2") });
    participation = (await json(ownerPage, "get", `/organizations/${organization.id}/agent-participations`, owner.headers)).participations.find((x: any) => x.id === participation.id);
    expect(participation.status).toBe("suspended");
    // A failed reevaluation and a material upgrade remain evidence and do not reactivate retired authority.
    const failedReevaluation = await trial(candidate.id, "strict", { agent_profile_version: 2, failure: "operator unavailable" });
    expect(failedReevaluation).toMatchObject({ correctness_passed: false, review_status: "pending" });
    expect(participation.outcomes[0]).toMatchObject({ attribution_id: merged.merge_commit_id, cost: 2.4 });
    expect(orgState.agents.find((x: any) => x.id === candidate.id).profiles).toHaveLength(2);
  } finally {
    await Promise.all(copies.map((path) => rm(path, { recursive: true, force: true })));
  }
});
