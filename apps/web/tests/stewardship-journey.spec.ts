import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey connects several public workflow projections */

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
  const user = await (await page.request.get("/api/user", { headers })).json() as { id: string };
  return { headers, user };
}

async function json(page: Page, method: "get" | "post" | "put", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const text = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${text}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${text}`).toBeLessThan(300);
  return text ? JSON.parse(text) : undefined;
}

test("maintainers carry a proactive steward finding through governed delivery and revoke it", async ({ browser }) => {
  test.setTimeout(300_000);
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copies: string[] = [];
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const operatorPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, "Stewardship Maintainer", `steward-owner-${suffix}`);
    const operator = await account(operatorPage, "Agent Operator", `steward-operator-${suffix}`);

    await ownerPage.goto("/organizations");
    await ownerPage.getByLabel("Name", { exact: true }).fill(`Runtime Stewards ${suffix}`);
    await ownerPage.getByLabel("URL slug").fill(`runtime-stewards-${suffix}`);
    await ownerPage.getByLabel("Purpose").fill("Keep the runtime healthy with bounded proactive agent help.");
    await ownerPage.getByRole("button", { name: "Create organization" }).click();
    let organizations = await json(ownerPage, "get", "/organizations", owner.headers) as any;
    const organization = organizations.organizations.find((item: any) => item.slug === `runtime-stewards-${suffix}`);
    await json(ownerPage, "post", `/organizations/${organization.id}/invitations`, owner.headers, { user_id: operator.user.id });
    organizations = await json(operatorPage, "get", "/organizations", operator.headers) as any;
    const invitation = organizations.organizations.find((item: any) => item.id === organization.id).invitations[0];
    await json(operatorPage, "post", `/organizations/${organization.id}/invitations/${invitation.id}/accept`, operator.headers);

    await ownerPage.goto(`/organizations/${organization.id}`);
    await ownerPage.getByPlaceholder("new-repository").fill(`runtime-${suffix}`);
    await ownerPage.getByRole("button", { name: "Create here" }).click();
    await expect(ownerPage.getByRole("link", { name: `runtime-${suffix}` })).toBeVisible();
    let portfolio = await json(ownerPage, "get", `/organizations/${organization.id}/portfolio`, owner.headers) as any;
    const repository = portfolio.repositories.find((item: any) => item.name === `runtime-${suffix}`);
    await ownerPage.getByPlaceholder("Agent name").fill("Runtime Steward");
    await ownerPage.getByPlaceholder("agent-slug").fill(`runtime-steward-${suffix}`);
    await ownerPage.getByPlaceholder("inspect checks, summarize failures").fill("inspect trusted evidence, edit bounded task branches, report verification");
    await ownerPage.getByPlaceholder("Operator collaboration ID").fill(operator.user.id);
    await ownerPage.getByRole("button", { name: "Register approved agent" }).click();
    await expect(ownerPage.getByRole("heading", { name: "Runtime Steward" })).toBeVisible();
    portfolio = await json(ownerPage, "get", `/organizations/${organization.id}/portfolio`, owner.headers) as any;
    const agent = portfolio.organization.agents.find((item: any) => item.name === "Runtime Steward");

    const expiresAt = new Date(Date.now() + 3_600_000).toISOString();
    const requestState = await json(operatorPage, "post", `/organizations/${organization.id}/access-requests`, operator.headers, {
      principal_type: "agent", principal_id: agent.id, role: "contributor",
      resources: [{ kind: "repository", id: repository.id }], exceptions: [],
      reason: "Implement only promoted runtime stewardship tasks.", expires_at: expiresAt,
    }) as any;
    const accessRequest = requestState.access_requests.at(-1);
    await json(ownerPage, "post", `/organizations/${organization.id}/access-requests/${accessRequest.id}/decision`, owner.headers, { decision: "approve" });

    const ownerGit = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "stewardship baseline", scopes: ["git:read", "git:write"], expires_in: 3600 }) as any;
    const baseline = await mkdtemp(join(tmpdir(), "vivarium-steward-baseline-")); copies.push(baseline);
    await git(tmpdir(), "clone", `http://git:${ownerGit.token}@localhost:3000/git/${repository.id}.git`, baseline);
    await git(baseline, "config", "user.name", "Stewardship Maintainer"); await git(baseline, "config", "user.email", "maintainer@example.test");
    await mkdir(join(baseline, ".vivarium"));
    await writeFile(join(baseline, ".vivarium", "checks.json"), JSON.stringify({ version: 1, checks: [{ name: "runtime verification", image: "alpine:3.22", command: "grep -qx 'timeout=30' runtime.conf" }] }));
    await writeFile(join(baseline, "runtime.conf"), "timeout=10\n");
    await git(baseline, "add", "."); await git(baseline, "commit", "-m", "Establish runtime baseline"); await git(baseline, "push", "origin", "main");
    const baseCommit = await git(baseline, "rev-parse", "HEAD");

    const mandate = await json(ownerPage, "post", `/organizations/${organization.id}/stewardship-mandates`, owner.headers, {
      title: "Improve runtime reliability", desired_outcomes: ["Reduce timeout-related failures"],
      repositories: [{ repository_id: repository.id, branches: ["main"] }], trusted_signals: ["usage regression"],
      exclusions: ["No dependency or deployment changes"], budget: { max_agent_minutes: 30, max_actions: 8 },
      starts_at: new Date().toISOString(), expires_at: expiresAt, agent_id: agent.id,
      allowed_actions: ["inspect evidence", "edit task branch", "run verification"], required_human_decisions: ["priority", "review", "merge", "release"],
      opportunity_policies: [{ evidence_type: "usage", minimum_severity: "medium", mode: "approval_required", max_agent_minutes: 15 }],
      reason: "Convert newly relevant usage evidence into reviewable reliability work.",
    }) as any;
    await json(operatorPage, "post", `/organizations/${organization.id}/stewardship-mandates/${mandate.id}/accept`, operator.headers, { expected_version: mandate.version });

    const evaluation = await json(operatorPage, "post", `/organizations/${organization.id}/stewardship-mandates/${mandate.id}/evaluations`, operator.headers, { findings: [
      { repository_id: repository.id, signal: "usage regression", evidence_type: "usage", evidence_id: "trace-timeouts", evidence_revision: "window-2026-08-10", dedupe_key: "runtime-timeout", title: "Increase the bounded runtime timeout", summary: "Recent project traces show otherwise healthy work ending at the legacy ten-second limit.", severity: "high", expected_value: "Reduce avoidable failures without widening network or dependency scope.", confidence: 0.94, affected_owner_ids: [owner.user.id], affected_revisions: [baseCommit], in_scope_reason: "The accepted reliability outcome and main branch scope cover this configuration.", citations: [{ kind: "usage", resource_id: "trace-timeouts", revision: "window-2026-08-10", label: "Timeout trace cohort" }] },
      { repository_id: repository.id, signal: "usage regression", evidence_type: "usage", evidence_id: "format-sample", evidence_revision: "window-2026-08-10", dedupe_key: "format-comments", title: "Reformat runtime comments", summary: "A small comment style inconsistency was observed.", severity: "low", expected_value: "Minor readability improvement.", confidence: 0.61, affected_owner_ids: [owner.user.id], affected_revisions: [baseCommit], in_scope_reason: "The sample came from the scoped repository.", citations: [{ kind: "usage", resource_id: "format-sample", revision: "window-2026-08-10", label: "Formatting sample" }] },
    ] }) as any;
    expect(evaluation.items).toHaveLength(2);

    await ownerPage.goto(`/organizations/${organization.id}`);
    const primary = ownerPage.getByRole("article").filter({ hasText: "Increase the bounded runtime timeout" });
    await primary.getByPlaceholder("Discuss or challenge this finding").fill("Prioritize this because the trace cohort is current and the change remains easy to verify.");
    await primary.getByRole("button", { name: "Comment" }).click();
    await expect(primary.getByText(/Prioritize this because/)).toBeVisible();
    const secondary = ownerPage.getByRole("article").filter({ hasText: "Reformat runtime comments" });
    await secondary.getByRole("button", { name: "Dismiss" }).click();
    await expect(secondary.getByText("dismissed", { exact: true })).toBeVisible();
    await primary.getByRole("button", { name: "Approve follow-up" }).click();
    await expect(primary.getByText("Promote into accountable work")).toBeVisible();

    const queue = await json(ownerPage, "get", `/organizations/${organization.id}/stewardship-mandates/${mandate.id}/opportunities`, owner.headers) as any;
    const opportunity = queue.items.find((item: any) => item.title === "Increase the bounded runtime timeout");
    const promoted = await json(operatorPage, "post", `/organizations/${organization.id}/stewardship-mandates/${mandate.id}/opportunities/${opportunity.id}/promotion`, operator.headers, {
      expected_version: opportunity.version, title: "Raise the verified runtime timeout", body: "Implement the prioritized trace-backed improvement under ordinary review.", base_revision: baseCommit, agent_minutes: 12,
      tasks: [{ title: "Raise and verify the timeout", owner_type: "agent", owner_id: agent.id, completion_criteria: "runtime.conf sets timeout=30 and runtime verification passes", risk: "A broader timeout could mask slow work; this change is limited to the named setting.", verification_plan: "Run the repository runtime verification check and inspect the exact diff.", depends_on_previous: false }],
    }) as any;
    const task = promoted.tasks[0];
    const authority = (await json(operatorPage, "get", `/organizations/${organization.id}`, operator.headers) as any).stewardship_mandates.find((item: any) => item.id === mandate.id);
    const linkedOpportunity = authority.opportunities.find((item: any) => item.id === opportunity.id);
    expect(authority).toMatchObject({ status: "active", acceptance: { operator_id: operator.user.id, version: authority.version } });
    expect(task).toMatchObject({ assignment: { assignee_id: agent.id, access: { base_revision: baseCommit } }, reasoning: { mandate_id: mandate.id, opportunity_id: opportunity.id, revision: baseCommit } });
    expect(linkedOpportunity).toMatchObject({ status: "promoted", work: { proposal_id: promoted.proposal.id, base_revision: baseCommit, task_ids: [task.id] } });
    const launched = await json(operatorPage, "post", `/repositories/${repository.id}/proposals/${promoted.proposal.id}/tasks/${task.id}/sessions`, operator.headers, { expected_assignment_id: task.assignment.id, context_paths: ["runtime.conf"], expires_in: 3600 }) as any;
    const runPath = `/repositories/${repository.id}/proposals/${promoted.proposal.id}/tasks/${task.id}/sessions/${launched.session.id}/runs/${launched.run.id}`;
    await json(ownerPage, "post", `${runPath}/interventions`, owner.headers, { kind: "run.guidance", message: "Keep the change to the single timeout value and report the exact verification command." });

    const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-steward-agent-")); copies.push(agentCopy);
    await git(tmpdir(), "clone", `http://git:${launched.credential.token}@localhost:3000/git/${repository.id}.git`, agentCopy);
    await git(agentCopy, "config", "user.name", "Runtime Steward"); await git(agentCopy, "config", "user.email", "runtime-steward@agents.vivarium");
    await git(agentCopy, "switch", "-c", "steward-timeout", `origin/${launched.run.working_branch}`);
    await writeFile(join(agentCopy, "runtime.conf"), "timeout=30\n");
    await git(agentCopy, "add", "runtime.conf"); await git(agentCopy, "commit", "-m", "Raise bounded runtime timeout"); await git(agentCopy, "push", "origin", `HEAD:refs/heads/${launched.run.working_branch}`);
    const agentCommit = await git(agentCopy, "rev-parse", "HEAD");
    const agentHeaders = { Authorization: `Bearer ${launched.credential.token}` };
    await json(operatorPage, "post", `${runPath}/completion`, agentHeaders, { summary: "Raised only the configured timeout and verified its exact value.", commit_id: agentCommit, checks: [{ name: "runtime verification", status: "passed", details: "The focused command passed." }], commands: [{ command: "grep -qx 'timeout=30' runtime.conf", exit_code: 0, summary: "Confirmed the bounded value." }], completion_criteria: [{ criterion: "runtime.conf sets timeout=30 and runtime verification passes", status: "met", evidence: "Focused grep succeeded at the result commit." }], unresolved_concerns: ["Maintainers should continue watching long-running work latency."] });
    const pull = await json(operatorPage, "post", `/repositories/${repository.id}/proposals/${promoted.proposal.id}/tasks/${task.id}/contributions`, operator.headers, { title: "Raise the verified runtime timeout", body: "Implements the prioritized stewardship opportunity.", source_branch: launched.run.working_branch, target_branch: "main", session_id: launched.session.id, run_id: launched.run.id }) as any;

    await ownerPage.goto(`/pulls/${repository.id}/${pull.id}`);
    await expect(ownerPage.getByRole("heading", { name: "Agent review handoff" })).toBeVisible();
    await expect(ownerPage.getByText("Timeout trace cohort")).toBeVisible();
    await expect(ownerPage.getByText("grep -qx 'timeout=30' runtime.conf")).toBeVisible();
    await expect(ownerPage.locator("#checks").getByText("succeeded", { exact: true })).toBeVisible({ timeout: 60_000 });
    await ownerPage.getByRole("button", { name: "Approve" }).click();
    await ownerPage.getByRole("button", { name: "Merge into main" }).click();
    await expect(ownerPage.getByText("Merged", { exact: true })).toBeVisible();
    const merged = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}`, owner.headers) as any;
    const release = await json(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: "v1.1.0", notes: "Deliver the trace-backed runtime reliability improvement.", commit_id: merged.merge_commit_id }) as any;
    expect(release.inclusions.pull_request_ids).toContain(pull.id);
    await json(ownerPage, "post", `/organizations/${organization.id}/stewardship-mandates/${mandate.id}/outcomes`, owner.headers, { idempotency_key: crypto.randomUUID(), kind: "release", status: "succeeded", summary: `Released ${release.version} (${release.id}) from verified pull ${pull.id}.`, opportunity_id: opportunity.id, goal: "Reduce timeout-related failures", goal_progress: 1, agent_minutes: 9, actions: 1 });

    const current = (await json(ownerPage, "get", `/organizations/${organization.id}`, owner.headers) as any).stewardship_mandates.find((item: any) => item.id === mandate.id);
    await json(ownerPage, "post", `/organizations/${organization.id}/stewardship-mandates/${mandate.id}/revoke`, owner.headers, { expected_version: current.version });
    const report = await json(ownerPage, "get", `/organizations/${organization.id}/stewardship-mandates/${mandate.id}/report`, owner.headers) as any;
    expect(report).toMatchObject({ status: "revoked", used_agent_minutes: 21, used_actions: 4 });
    expect(report.opportunity_dispositions).toEqual(expect.objectContaining({ promoted: 1, dismissed: 1 }));
    expect(report.outcomes).toEqual(expect.arrayContaining([expect.objectContaining({ opportunity_id: opportunity.id, status: "succeeded", summary: expect.stringContaining(release.id) })]));

    await ownerPage.goto(`/organizations/${organization.id}`);
    await expect(ownerPage.getByText("revoked", { exact: true }).first()).toBeVisible();
    await ownerPage.getByText(/Learning and outcomes · 1 records/).click();
    await expect(ownerPage.getByText(/Disposition promoted 1 · dismissed 1/)).toBeVisible();
    await expect(ownerPage.getByText(/resource use 21 minutes \/ 4 actions/)).toBeVisible();
  } finally {
    await Promise.all(copies.map((path) => rm(path, { recursive: true, force: true })));
  }
});
