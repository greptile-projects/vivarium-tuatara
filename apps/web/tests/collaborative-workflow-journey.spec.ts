import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey intentionally joins independently versioned public records */
const run = promisify(execFile);
async function git(cwd: string, ...args: string[]) {
  return (await run("git", args, { cwd, env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } })).stdout.trim();
}
async function authenticatedGit(token: string, cwd: string, ...args: string[]) {
  return (await run("git", ["-c", "credential.helper=", ...args], { cwd, env: { ...process.env, GIT_ASKPASS: join(__dirname, "git-askpass.sh"), GIT_TERMINAL_PROMPT: "0", VIVARIUM_GIT_TOKEN: token } })).stdout.trim();
}
async function account(page: Page, name: string, suffix: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(name);
  await page.getByLabel("Handle").fill(`${name.toLowerCase().replaceAll(" ", "-")}-${suffix}`);
  await page.getByRole("button", { name: "Create account and continue" }).click();
  await expect(page.getByRole("heading", { name: "Create a repository" })).toBeVisible();
  const token = await page.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  return { headers, user: await (await page.request.get("/api/user", { headers })).json() as any };
}
async function json(page: Page, method: "get" | "post" | "patch", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}
async function rejected(response: APIResponse, status: number) {
  expect(response.status(), await response.text()).toBe(status);
  return response.json();
}

test("an accepted issue becomes a bounded, inspectable human-agent delivery run", async ({ browser }) => {
  test.setTimeout(240_000);
  const suffix = Date.now().toString(36);
  const ownerPage = await (await browser.newContext()).newPage();
  const reviewerPage = await (await browser.newContext()).newPage();
  const owner = await account(ownerPage, "Workflow Owner", suffix);
  const reviewer = await account(reviewerPage, "Human Reviewer", suffix);
  const repository = await json(ownerPage, "post", "/repositories", owner.headers, { name: `repair-loop-${suffix}` });
  await json(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: reviewer.user.id });
  const credential = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "workflow journey", scopes: ["git:read", "git:write"], expires_in: 3600 });
  const copy = await mkdtemp(join(tmpdir(), "vivarium-workflow-"));
  try {
    await authenticatedGit(credential.token, tmpdir(), "clone", `http://localhost:3000/git/${repository.id}.git`, copy);
    await git(copy, "config", "user.name", "Workflow Owner");
    await git(copy, "config", "user.email", "workflow@example.test");
    await mkdir(join(copy, ".vivarium"));
    await writeFile(join(copy, "repair-agent.md"), "Prepare one issue-scoped patch; never merge, release, deploy, or read secrets.\n");
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Review bounded repair agent"); await authenticatedGit(credential.token, copy, "push", "origin", "main");
    let revision = await git(copy, "rev-parse", "HEAD");
    const project = await json(ownerPage, "post", `/repositories/${repository.id}/agent-projects`, owner.headers, { revision: {
      title: "Bounded repair preparer", purpose: "Prepare a patch for one accepted issue", owner_ids: [owner.user.id],
      sources: [{ id: "prompt", kind: "prompt", repository_id: repository.id, revision, path: "repair-agent.md", purpose: "Reviewed repair boundary" }],
      tools: [{ name: "git", purpose: "Prepare a candidate patch", actions: ["read", "write"], boundary: "one issue branch; no merge" }],
      models: [{ provider: "Vivarium", name: "repair", version: "1", purpose: "bounded repair" }], supported_tasks: ["issue repair"], expected_outputs: ["candidate revision"], prohibited_actions: ["merge", "release", "deploy", "secrets"], memory_policy: "execution only", data_use_terms: "selected issue and repository paths only", budget: { max_cost_usd: 2, max_tokens: 4000, max_tool_actions: 4, max_runtime_seconds: 120 }, escalations: [{ trigger: "uncertainty", owner_ids: [owner.user.id], action: "stop" }], deployment_boundaries: [{ environment: "repository", repository_access: "issue branch", network_access: "none", data_classes: ["repository"], approval_required: true }], change_summary: "Initial reviewed boundary"
    }});
    const step = (id: string, name: string, needs: string[], invocation: any, outputs: string[] = [], approval = "") => ({ id, name, needs, invocation, outputs, retries: 2, timeout_seconds: 120, budget_actions: 2, owner_ids: [owner.user.id], completion: [`${name} retained`], ...(approval ? { approval } : {}) });
    const definition = {
      name: "Accepted issue delivery", outcome: "An accepted issue reaches a reviewed protected deployment", description: "Keep agent work and consequential actions independently governed", owner_ids: [owner.user.id],
      triggers: [{ id: "accepted", kind: "repository_event", event: "issue.accepted", inputs: [{ name: "issue_id", type: "string", required: true, source: "event.issue_id" }] }],
      steps: [
        step("prepare", "Prepare bounded repair", [], { kind: "agent", agent_id: project.id, authority: ["repository:read", "branch:write"] }, ["revision"]),
        step("pull", "Open pull request", ["prepare"], { kind: "platform_action", action: "update_project", authority: ["pulls:write"] }, ["pull_id"]),
        step("review", "Wait for human review and checks", ["pull"], { kind: "platform_action", action: "request_review", authority: ["reviews:write", "checks:read"] }, ["evidence"]),
        step("queue", "Enter merge queue", ["review"], { kind: "platform_action", action: "merge", authority: ["merge-queue:write"] }, ["merge_revision"], "merge"),
        step("release", "Build exact release", ["queue"], { kind: "platform_action", action: "release", authority: ["releases:write"] }, ["release_id"], "release"),
        step("deploy", "Request protected deployment", ["release"], { kind: "platform_action", action: "change_infrastructure", authority: ["deployments:request"] }, ["deployment_id"], "infrastructure_change"),
      ], outputs: ["deployment_id"], completion: ["protected deployment request retained"], budget_actions: 10,
    };
    await writeFile(join(copy, ".vivarium", "workflow.json"), JSON.stringify(definition, null, 2));
    await git(copy, "add", ".vivarium/workflow.json"); await git(copy, "commit", "-m", "Author accepted issue workflow"); await authenticatedGit(credential.token, copy, "push", "origin", "main");
    revision = await git(copy, "rev-parse", "HEAD");
    const preview = await json(ownerPage, "post", `/repositories/${repository.id}/collaboration-workflows/preview`, owner.headers, { revision, path: ".vivarium/workflow.json" });
    expect(preview).toMatchObject({ activatable: true, subscriptions: ["repository_event:issue.accepted"] });
    const workflow = await json(ownerPage, "post", `/repositories/${repository.id}/collaboration-workflows`, owner.headers, { activation_id: `accepted-${suffix}`, revision, path: ".vivarium/workflow.json" });
    const issue = await json(reviewerPage, "post", `/repositories/${repository.id}/issues`, reviewer.headers, { title: "Retry loses collaborator redirect", expected_behavior: "The redirected run resumes once", observed_behavior: "The repair step is duplicated", severity: "high", environment: "workflow engine", reproduction_steps: ["interrupt a claimed step", "resume the run"], visibility: "repository" });
    const accepted = await json(ownerPage, "patch", `/repositories/${repository.id}/issues/${issue.id}`, owner.headers, { status: "triaged", expected_version: issue.version, message: "Accepted for bounded repair" });
    expect(accepted.status).toBe("triaged");
    const activity = await json(ownerPage, "get", "/activity?limit=100", owner.headers);
    const delivery = activity.events.find((event: any) => event.kind === "issue.accepted" && event.resource_id === issue.id);
    expect(delivery).toMatchObject({ actor_id: owner.user.id, resource_revision: revision });
    let execution = await json(ownerPage, "post", `/repositories/${repository.id}/collaboration-workflows/${workflow.id}/executions`, owner.headers, { delivery_id: delivery.id, workflow_version: 1 });
    const duplicate = await json(ownerPage, "post", `/repositories/${repository.id}/collaboration-workflows/${workflow.id}/executions`, owner.headers, { delivery_id: delivery.id, workflow_version: 1 });
    expect(duplicate.id).toBe(execution.id);
    await rejected(await ownerPage.request.post(`/api/repositories/${repository.id}/collaboration-workflows/${workflow.id}/executions`, { headers: owner.headers, data: { delivery_id: delivery.id, workflow_version: 2 } }), 409);
    const claimPath = (id: string) => `/repositories/${repository.id}/collaboration-workflows/${workflow.id}/executions/${execution.id}/steps/${id}/claim`;
    const completePath = (id: string) => `/repositories/${repository.id}/collaboration-workflows/${workflow.id}/executions/${execution.id}/steps/${id}/complete`;
    let lease = await json(ownerPage, "post", claimPath("prepare"), owner.headers, { expected_version: execution.version });
    execution = await json(ownerPage, "post", completePath("prepare"), owner.headers, { token: lease.token, actions: 1, failure_code: "engine_interrupted", logs: [{ time: new Date().toISOString(), level: "error", message: "runner restarted after durable checkpoint" }], agent_session: { id: `repair-${suffix}`, agent_id: project.id, status: "interrupted" }, cost_units: 0.3, provenance: [`issue:${issue.id}`, `workflow:${workflow.id}@1`] });
    execution = await json(reviewerPage, "post", `/repositories/${repository.id}/collaboration-workflows/${workflow.id}/executions/${execution.id}/interventions`, reviewer.headers, { expected_version: execution.version, kind: "retry", step_id: "prepare", reason: "Resume from the retained issue boundary" });
    const expiredToken = lease.token;
    lease = await json(ownerPage, "post", claimPath("prepare"), owner.headers, { expected_version: execution.version });
    await rejected(await ownerPage.request.post(`/api${completePath("prepare")}`, { data: { token: expiredToken, actions: 1, outputs: { revision } } }), 401);
    await rejected(await ownerPage.request.post(`/api${completePath("prepare")}`, { data: { token: lease.token, actions: 3, outputs: { revision } } }), 409);
    execution = await json(ownerPage, "post", completePath("prepare"), owner.headers, { token: lease.token, actions: 1, outputs: { revision }, logs: [{ time: new Date().toISOString(), level: "info", message: "bounded candidate prepared" }], agent_session: { id: `repair-${suffix}`, agent_id: project.id, status: "succeeded" }, cost_units: 0.7, provenance: [`issue:${issue.id}`, `commit:${revision}`] });
    for (const [id, outputs] of [["pull", { pull_id: `pull-${suffix}` }], ["review", { evidence: "human approval and required checks passed" }]] as const) {
      lease = await json(ownerPage, "post", claimPath(id), owner.headers, { expected_version: execution.version });
      execution = await json(ownerPage, "post", completePath(id), owner.headers, { token: lease.token, actions: 1, outputs, cost_units: 0.1, provenance: [`issue:${issue.id}`] });
    }
    await rejected(await reviewerPage.request.post(`/api/repositories/${repository.id}/collaboration-workflows/${workflow.id}/executions/${execution.id}/interventions`, { headers: reviewer.headers, data: { expected_version: execution.version, kind: "approve", step_id: "queue", reason: "reviewer cannot grant merge authority" } }), 409);
    for (const [id, outputs] of [["queue", { merge_revision: revision }], ["release", { release_id: `release-${suffix}` }], ["deploy", { deployment_id: `protected-${suffix}` }]] as const) {
      execution = await json(ownerPage, "post", `/repositories/${repository.id}/collaboration-workflows/${workflow.id}/executions/${execution.id}/interventions`, owner.headers, { expected_version: execution.version, kind: "approve", step_id: id, reason: `Resource owner approved ${id}` });
      lease = await json(ownerPage, "post", claimPath(id), owner.headers, { expected_version: execution.version });
      execution = await json(ownerPage, "post", completePath(id), owner.headers, { token: lease.token, actions: 1, outputs, cost_units: 0.1, provenance: [`issue:${issue.id}`, `commit:${revision}`] });
    }
    expect(execution).toMatchObject({ status: "succeeded", workflow_version: 1, trigger: { inputs: { issue_id: issue.id }, resource_revisions: { issue_id: revision } }, actions_used: 7 });
    expect(execution.steps[0].attempts).toHaveLength(2);
    expect(execution.interventions).toEqual(expect.arrayContaining([expect.objectContaining({ kind: "retry", actor_id: reviewer.user.id }), expect.objectContaining({ kind: "approve", actor_id: owner.user.id })]));
    expect(execution.action_receipts).toHaveLength(6);
    await ownerPage.goto(`/repositories/${repository.id}/workflows`);
    await expect(ownerPage.getByRole("heading", { name: "Collaboration workflows" })).toBeVisible();
    await expect(ownerPage.getByText("Accepted issue delivery", { exact: true })).toBeVisible();
    await expect(ownerPage.getByText("succeeded", { exact: true }).first()).toBeVisible();
    await ownerPage.getByText(/Attempt 1 · interrupted/).click();
    await expect(ownerPage.getByText("runner restarted after durable checkpoint")).toBeVisible();
  } finally {
    const revoked = await ownerPage.request.delete(`/api/auth/credentials/${credential.id}`, { headers: owner.headers });
    expect(revoked.status(), await revoked.text()).toBe(204);
    await rm(copy, { recursive: true, force: true });
  }
});
