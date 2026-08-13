import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this proof joins the complete public governance workflow */

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
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}
async function tallyWhenClosed(page: Page, proposalID: string, headers: Record<string, string>) {
  let tally: any;
  await expect.poll(async () => {
    const response = await page.request.post(`/api/governance/proposals/${proposalID}/tally`, { headers, data: {} });
    const body = await response.text();
    if (response.status() === 409) {
      expect(JSON.parse(body)).toMatchObject({ error: { code: "voting_closed" } });
      return false;
    }
    expect(response.status(), `POST governance tally: ${body}`).toBe(200);
    tally = JSON.parse(body);
    return true;
  }, { timeout: 40_000 }).toBe(true);
  return tally;
}

test("a community governs delivery and renews stewardship without inheriting repository authority", async ({ browser }) => {
  test.setTimeout(300_000);
  const dockerAvailable = await run("docker", ["info"]).then(() => true, () => false);
  test.skip(!dockerAvailable, "the governance journey requires isolated required-check execution");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copies: string[] = [];
  const contexts = [];
  try {
    const suffix = Date.now().toString(36);
    const ownerContext = await browser.newContext(); contexts.push(ownerContext);
    const contributorContext = await browser.newContext(); contexts.push(contributorContext);
    const successorContext = await browser.newContext(); contexts.push(successorContext);
    const ownerPage = await ownerContext.newPage(), contributorPage = await contributorContext.newPage(), successorPage = await successorContext.newPage();
    const owner = await account(ownerPage, "Founding Maintainer", `governance-owner-${suffix}`);
    const contributor = await account(contributorPage, "Proven Contributor", `governance-contributor-${suffix}`);
    const successor = await account(successorPage, "Successor Maintainer", `governance-successor-${suffix}`);

    const repository = await json(ownerPage, "post", "/repositories", owner.headers, { name: `community-runtime-${suffix}` }) as any;
    await json(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: contributor.user.id });
    const unboundCredential = await json(successorPage, "post", "/auth/credentials", successor.headers, { kind: "git", name: "standing is not access", scopes: ["git:read", "git:write"], expires_in: 3600 }) as any;
    await expect(run("git", ["ls-remote", `http://git:${unboundCredential.token}@localhost:3000/git/${repository.id}.git`], { env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } })).rejects.toThrow();
    await json(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: successor.user.id });
    const credential = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "governance baseline", scopes: ["git:read", "git:write"], expires_in: 3600 }) as any;
    const copy = await mkdtemp(join(tmpdir(), "vivarium-governance-")); copies.push(copy);
    await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${repository.id}.git`, copy);
    await git(copy, "config", "user.name", "Founding Maintainer"); await git(copy, "config", "user.email", "founder@example.test");
    await mkdir(join(copy, ".vivarium"));
    await writeFile(join(copy, "runtime.conf"), "mode=legacy\n");
    await writeFile(join(copy, ".vivarium", "checks.json"), JSON.stringify({ version: 1, checks: [{ name: "community policy", image: "alpine:3.22", command: "grep -qx 'mode=community' runtime.conf && test -f stewardship.md" }] }, null, 2) + "\n");
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Establish governed runtime baseline"); await git(copy, "push", "origin", "main");
    const base = await git(copy, "rev-parse", "HEAD");
    await json(ownerPage, "put", `/repositories/${repository.id}/branches/main/required-checks`, owner.headers, { checks: ["community policy"] });

    const charter = { title: "Community stewardship charter", summary: "Contributors decide direction while repository controls remain independent.", roles: [{ name: "maintainer", description: "Deliberate and renew community stewardship", eligibility: ["repository_owner", "repository_collaborator"] }], decision_classes: [{ name: "community initiative", description: "Approve delivery and leadership transitions", eligible_roles: ["maintainer"], participation: 3, quorum: 2, approval: "majority", protected_resources: ["branch:main", "release:stable"] }], procedures: { terms: "One year with attributable renewal", removal: "Suspension with an appeal retained", succession: "Governed election and explicit handoff", amendments: "A fresh charter revision and approval" } };
    await json(ownerPage, "post", `/repositories/${repository.id}/charter/revisions`, owner.headers, { expected_version: 0, charter });
    await json(ownerPage, "post", `/repositories/${repository.id}/charter/revisions/1/approvals`, owner.headers, { version: 1, decision: "approved", reason: "Adopt transparent community governance." });
    await json(ownerPage, "post", `/repositories/${repository.id}/charter/revisions/1/activate`, owner.headers, { version: 1 });

    async function invite(principalID: string, evidence: any[]) {
      const view = await json(ownerPage, "get", `/repositories/${repository.id}/charter`, owner.headers) as any;
      const created = await json(ownerPage, "post", `/repositories/${repository.id}/charter/standing`, owner.headers, { expected_version: view.charter.standings.length, charter_version: 1, principal_type: "human", principal_id: principalID, role: "maintainer", responsibilities: "Evaluate evidence, record dissent, and preserve independent resource controls.", evidence, expires_at: new Date(Date.now() + 86400000).toISOString() }) as any;
      return created.charter.standings.at(-1);
    }
    const ownerStanding = await invite(owner.user.id, [{ kind: "ownership", resource_id: repository.id, summary: "Founding repository stewardship" }]);
    const contributorStanding = await invite(contributor.user.id, [{ kind: "contribution", resource_id: "pull-proven-42", summary: "Sustained reviewed runtime contributions" }]);
    const successorStanding = await invite(successor.user.id, [{ kind: "support", resource_id: "support-rotation-7", summary: "Documented community support and incident response" }]);
    await json(ownerPage, "post", `/repositories/${repository.id}/charter/standing/${ownerStanding.id}/actions`, owner.headers, { action: "accept", reason: "Accept the charter responsibilities." });
    await json(contributorPage, "post", `/repositories/${repository.id}/charter/standing/${contributorStanding.id}/actions`, contributor.headers, { action: "accept", reason: "Accept standing earned through reviewed contribution." });
    await json(successorPage, "post", `/repositories/${repository.id}/charter/standing/${successorStanding.id}/actions`, successor.headers, { action: "accept", reason: "Accept bounded governance standing." });
    async function proposal(page: Page, headers: Record<string, string>, title: string) {
      const closes = new Date(Date.now() + 30_000).toISOString();
      return json(page, "post", "/governance/proposals", headers, { scope_type: "repository", scope_id: repository.id, source: { kind: "initiative", resource_id: `initiative-${title}`, label: title }, title, summary: "Evaluate a bounded community change from retained project evidence.", scope: "The runtime and stewardship workflow only.", alternatives: [{ id: "adopt", title: "Adopt", summary: "Proceed through ordinary repository controls.", effects: ["Create a reviewed implementation"] }, { id: "defer", title: "Defer", summary: "Retain the current implementation.", effects: ["No repository change"] }], evidence: [{ kind: "usage", resource_id: "runtime-trace-2026-08", label: "Exact runtime usage trace" }], affected_resources: [{ kind: "branch", resource_id: "main", label: "Protected main branch" }], disclosure_requirements: ["Disclose operational risk and conflicts"], implementation_effects: ["Human and agent tasks require review and checks"], rule: { decision_class: "community initiative", opens_at: new Date(Date.now() - 100).toISOString(), closes_at: closes } });
    }
    let failed = await proposal(ownerPage, owner.headers, "Quorum recovery rehearsal") as any;
    failed = await json(ownerPage, "post", `/governance/proposals/${failed.id}/ballots`, owner.headers, { choice: "adopt", reason: "Availability should not rewrite quorum." });
    failed = await tallyWhenClosed(ownerPage, failed.id, owner.headers);
    expect(failed.tally).toMatchObject({ status: "not_accepted", quorum_met: false });

    let initiative = await proposal(contributorPage, contributor.headers, "Community runtime initiative") as any;
    initiative = await json(contributorPage, "post", `/governance/proposals/${initiative.id}/analysis`, contributor.headers, { actor_type: "human", body: "The trace supports adoption, but the rollout needs a reversible limit.", position: "support", citations: [{ kind: "usage", resource_id: "runtime-trace-2026-08", label: "Runtime trace" }] });
    initiative = await json(ownerPage, "post", `/governance/proposals/${initiative.id}/analysis`, owner.headers, { actor_type: "human", body: "Recorded dissent: agent-authored work must not acquire review or merge authority.", position: "oppose", citations: [{ kind: "policy", resource_id: "branch-main", label: "Independent main-branch policy" }] });
    await json(ownerPage, "post", `/governance/proposals/${initiative.id}/ballots`, owner.headers, { choice: "adopt", reason: "Approve only with ordinary controls." });
    await json(contributorPage, "post", `/governance/proposals/${initiative.id}/ballots`, contributor.headers, { choice: "adopt", reason: "The evidence and bounded path are sufficient." });
    await json(successorPage, "post", `/governance/proposals/${initiative.id}/ballots`, successor.headers, { choice: "recuse", reason: "I helped prepare the cited support rotation." });
    initiative = await tallyWhenClosed(ownerPage, initiative.id, owner.headers);
    expect(initiative.tally).toMatchObject({ status: "accepted", quorum_met: true, recusals: 1, contested: false });
    expect(initiative.analyses).toEqual(expect.arrayContaining([expect.objectContaining({ position: "oppose", body: expect.stringContaining("Recorded dissent") })]));

    initiative = await json(ownerPage, "post", `/governance/proposals/${initiative.id}/implementation`, owner.headers, { repository_id: repository.id, revision: base, scope: "Adopt community mode and document bounded stewardship.", cost: "Two small reviewed tasks", title: "Deliver the community runtime initiative", body: "The decision receipt is a mandate, not repository authority.", assumptions: ["The exact usage trace remains representative"], protected_effects: ["main and stable release remain owner controlled"], tasks: [{ title: "Adopt community runtime mode", outcome: "Runtime uses the accepted mode", risk: "Configuration regression", verification_plan: "Required community policy check", assignee_type: "human", assignee_id: contributor.user.id, depends_on_previous: false }, { title: "Document stewardship safeguards", outcome: "Operational authority boundaries are explicit", risk: "Readers confuse standing with access", verification_plan: "Required check and independent review", assignee_type: "agent", depends_on_previous: true }] }) as any;
    const implementationID = initiative.implementation.steps[0].resource_id;
    let tasks = await json(ownerPage, "get", `/repositories/${repository.id}/proposals/${implementationID}/tasks`, owner.headers) as any;
    const [humanTask, agentTask] = tasks.tasks;
    const contributorCredential = await json(contributorPage, "post", "/auth/credentials", contributor.headers, { kind: "git", name: "human governed task", scopes: ["git:read", "git:write"], expires_in: 3600 }) as any;
    const humanCopy = await mkdtemp(join(tmpdir(), "vivarium-governance-human-")); copies.push(humanCopy);
    await git(tmpdir(), "clone", `http://git:${contributorCredential.token}@localhost:3000/git/${repository.id}.git`, humanCopy);
    await git(humanCopy, "config", "user.name", "Proven Contributor"); await git(humanCopy, "config", "user.email", "contributor@example.test"); await git(humanCopy, "switch", "-c", "community-runtime");
    await writeFile(join(humanCopy, "runtime.conf"), "mode=community\n"); await git(humanCopy, "add", "runtime.conf"); await git(humanCopy, "commit", "-m", "Adopt community runtime mode"); await git(humanCopy, "push", "origin", "community-runtime");
    const humanPull = await json(contributorPage, "post", `/repositories/${repository.id}/proposals/${implementationID}/tasks/${humanTask.id}/contributions`, contributor.headers, { title: "Adopt community runtime mode", body: "Human-owned implementation of the accepted result.", source_branch: "community-runtime", target_branch: "main" }) as any;
    await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${humanPull.id}/reviews`, owner.headers, { decision: "approved" });
    await expect.poll(async () => (await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${humanPull.id}/merge-readiness`, owner.headers)).blockers.length, { timeout: 60_000 }).toBe(1);
    await writeFile(join(humanCopy, "stewardship.md"), "# Stewardship\n\nGovernance standing grants voice, not Git, review, merge, release, or credential authority.\n"); await git(humanCopy, "add", "stewardship.md"); await git(humanCopy, "commit", "-m", "Document stewardship boundary"); await git(humanCopy, "push", "origin", "community-runtime");
    await json(contributorPage, "post", `/repositories/${repository.id}/pulls/${humanPull.id}/synchronize`, contributor.headers, {});
    await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${humanPull.id}/reviews`, owner.headers, { decision: "approved" });
    await expect.poll(async () => (await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${humanPull.id}/merge-readiness`, owner.headers)).blockers.length, { timeout: 60_000 }).toBe(0);
    const humanMerge = await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${humanPull.id}/merge`, owner.headers, {}) as any;

    tasks = await json(ownerPage, "get", `/repositories/${repository.id}/proposals/${implementationID}/tasks`, owner.headers) as any;
    const readyAgent = tasks.tasks.find((x: any) => x.id === agentTask.id);
    const launched = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${implementationID}/tasks/${agentTask.id}/sessions`, owner.headers, { expected_assignment_id: readyAgent.assignment.id, context_paths: ["runtime.conf"], expires_in: 3600 }) as any;
    const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-governance-agent-")); copies.push(agentCopy);
    await git(tmpdir(), "clone", `http://git:${launched.credential.token}@localhost:3000/git/${repository.id}.git`, agentCopy); await git(agentCopy, "config", "user.name", "Governance Delivery Agent"); await git(agentCopy, "config", "user.email", "agent@agents.vivarium"); await git(agentCopy, "switch", launched.run.working_branch); await git(agentCopy, "fetch", `http://git:${credential.token}@localhost:3000/git/${repository.id}.git`, humanMerge.merge_commit_id); await git(agentCopy, "merge", "FETCH_HEAD", "--no-edit");
    await writeFile(join(agentCopy, "stewardship.md"), "# Stewardship\n\nGovernance standing grants voice, not Git, review, merge, release, or credential authority. Emergency recovery is bounded, reviewed, and attributable.\n"); await git(agentCopy, "add", "stewardship.md"); await git(agentCopy, "commit", "-m", "Document emergency recovery safeguards"); await git(agentCopy, "push", "origin", launched.run.working_branch);
    const agentCommit = await git(agentCopy, "rev-parse", "HEAD");
    const runPath = `/repositories/${repository.id}/proposals/${implementationID}/tasks/${agentTask.id}/sessions/${launched.session.id}/runs/${launched.run.id}`;
    await json(ownerPage, "post", `${runPath}/completion`, { Authorization: `Bearer ${launched.credential.token}` }, { summary: "Documented the charter's recovery boundary.", commit_id: agentCommit, checks: [{ name: "authority wording", status: "passed", details: "Operational authority remains separate." }], unresolved_concerns: [] });
    const agentPull = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${implementationID}/tasks/${agentTask.id}/contributions`, owner.headers, { title: "Document stewardship safeguards", body: "Agent-owned task routed through independent review.", source_branch: launched.run.working_branch, target_branch: "main", session_id: launched.session.id, run_id: launched.run.id }) as any;
    await json(contributorPage, "post", `/repositories/${repository.id}/pulls/${agentPull.id}/reviews`, contributor.headers, { decision: "approved" });
    await expect.poll(async () => (await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${agentPull.id}/merge-readiness`, owner.headers)).blockers.length, { timeout: 60_000 }).toBe(0);
    const merged = await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${agentPull.id}/merge`, owner.headers, {}) as any;
    const release = await json(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: "v1.0.0", notes: "Community-approved delivery with independent policy enforcement.", commit_id: merged.merge_commit_id }) as any;

    await json(ownerPage, "post", `/repositories/${repository.id}/charter/standing/${contributorStanding.id}/actions`, owner.headers, { action: "suspend", reason: "Review a disclosed conflict without deleting contribution history." });
    await json(contributorPage, "post", `/repositories/${repository.id}/charter/standing/${contributorStanding.id}/actions`, contributor.headers, { action: "appeal", reason: "The conflict was disclosed and recused from the affected ballot." });
    await json(ownerPage, "post", `/repositories/${repository.id}/charter/standing/${contributorStanding.id}/actions`, owner.headers, { action: "reinstate", reason: "Appeal reviewed; independent repository access was never derived from standing." });

    let election = await proposal(ownerPage, owner.headers, "Successor maintainer election") as any;
    await json(ownerPage, "post", `/governance/proposals/${election.id}/ballots`, owner.headers, { choice: "adopt", reason: "Transfer governance stewardship transparently." });
    await json(contributorPage, "post", `/governance/proposals/${election.id}/ballots`, contributor.headers, { choice: "adopt", reason: "The nominee has proven support work." });
    election = await tallyWhenClosed(ownerPage, election.id, owner.headers);
    const charterView = await json(ownerPage, "get", `/repositories/${repository.id}/charter`, owner.headers) as any;
    const continuity = await json(ownerPage, "post", `/repositories/${repository.id}/charter/continuity`, owner.headers, { expected_version: charterView.continuity.length, charter_version: 1, action: { kind: "election", role: "maintainer", from_standing_id: ownerStanding.id, to_standing_id: successorStanding.id, governance_proposal_id: election.id, reason: "Elect and hand off community stewardship.", resources: ["branch:main", "release:stable"], expires_at: new Date(Date.now() + 86400000).toISOString(), review_at: new Date(Date.now() + 43200000).toISOString() } }) as any;
    const electionAction = continuity.charter.continuity.at(-1);
    await json(ownerPage, "post", `/repositories/${repository.id}/charter/continuity/${electionAction.id}/actions`, owner.headers, { action: "approve", reason: "Accepted tally and active endpoints verified." });
    await json(ownerPage, "post", `/repositories/${repository.id}/charter/continuity/${electionAction.id}/actions`, owner.headers, { action: "complete", reason: "Governance handoff complete; resource access was approved separately." });

    let emergency = await proposal(ownerPage, owner.headers, "Emergency availability recovery") as any;
    await json(ownerPage, "post", `/governance/proposals/${emergency.id}/ballots`, owner.headers, { choice: "adopt", reason: "Authorize bounded recovery without rewriting the charter." });
    await json(successorPage, "post", `/governance/proposals/${emergency.id}/ballots`, successor.headers, { choice: "adopt", reason: "Keep the project available under explicit review." });
    emergency = await tallyWhenClosed(successorPage, emergency.id, successor.headers);
    const beforeEmergency = await json(successorPage, "get", `/repositories/${repository.id}/charter`, successor.headers) as any;
    const recovery = await json(ownerPage, "post", `/repositories/${repository.id}/charter/continuity`, owner.headers, { expected_version: beforeEmergency.continuity.length, charter_version: 1, action: { kind: "emergency", role: "maintainer", governance_proposal_id: emergency.id, reason: "Time-bound availability recovery while ordinary stewardship is unavailable.", resources: ["branch:main"], expires_at: new Date(Date.now() + 3600000).toISOString(), review_at: new Date(Date.now() + 1800000).toISOString() } }) as any;
    const recoveryAction = recovery.charter.continuity.at(-1);
    await json(ownerPage, "post", `/repositories/${repository.id}/charter/continuity/${recoveryAction.id}/actions`, owner.headers, { action: "approve", reason: "Narrow recovery approved from the accepted result." });
    await json(ownerPage, "post", `/repositories/${repository.id}/charter/continuity/${recoveryAction.id}/actions`, owner.headers, { action: "relinquish", reason: "Availability restored; relinquish emergency governance before expiry." });

    await successorPage.goto(`/repositories/${repository.id}/charter`);
    await expect(successorPage.getByRole("heading", { name: "Project charter" })).toBeVisible();
    await expect(successorPage.getByText("Community stewardship charter")).toBeVisible();
    await expect(successorPage.getByText(/completed/).first()).toBeVisible();
    await expect(successorPage.getByText(/relinquished/).first()).toBeVisible();
    await successorPage.goto("/governance");
    await expect(successorPage.getByRole("heading", { name: "Governance" })).toBeVisible();
    await expect(successorPage.getByText("Community runtime initiative")).toBeVisible();
    expect(release.commit_id).toBe(merged.merge_commit_id);
    expect(election.tally.verification_sha256).toBe(electionAction.governance_tally_sha256);
    expect((await json(successorPage, "get", `/repositories/${repository.id}/charter`, successor.headers)).charter.active_version).toBe(1);
  } finally {
    await Promise.all(copies.map(path => rm(path, { recursive: true, force: true })));
    await Promise.all(contexts.map(context => context.close()));
  }
});
