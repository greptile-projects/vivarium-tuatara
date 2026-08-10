import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey intentionally joins the complete public decision workflow */

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

test("collaborators carry uncertainty through evidence, delivery, measurement, and revisit", async ({ browser }) => {
  test.setTimeout(300_000);
  const dockerAvailable = await run("docker", ["info"]).then(() => true, () => false);
  test.skip(!dockerAvailable, "the decision journey requires isolated prototype and check execution");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copies: string[] = [];
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const peerPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, "Decision Owner", `decision-owner-${suffix}`);
    const peer = await account(peerPage, "Affected Owner", `decision-peer-${suffix}`);

    const repository = await json(ownerPage, "post", "/repositories", owner.headers, { name: `adaptive-queue-${suffix}` }) as any;
    const affectedRepository = await json(peerPage, "post", "/repositories", peer.headers, { name: `traffic-gateway-${suffix}` }) as any;
    await json(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: peer.user.id });
    const credential = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "decision delivery", scopes: ["git:read", "git:write"], expires_in: 3600 }) as any;
    const copy = await mkdtemp(join(tmpdir(), "vivarium-decision-")); copies.push(copy);
    await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${repository.id}.git`, copy);
    await git(copy, "config", "user.name", "Decision Owner"); await git(copy, "config", "user.email", "owner@example.test");
    await mkdir(join(copy, ".vivarium"));
    await writeFile(join(copy, "queue.conf"), "strategy=unbounded\nlimit=0\n");
    await writeFile(join(copy, ".vivarium", "workspace.json"), JSON.stringify({ version: 1, image: "alpine:3.22", tools: [{ name: "sh", version: "3.22" }], dependencies: ["sh"], setup: ["test -f queue.conf"], experiments: [{ name: "burst", command: "grep -q '^strategy=' queue.conf" }], resources: { cpus: 1, memory_mb: 256, storage_mb: 128, setup_seconds: 30 } }, null, 2) + "\n");
    await writeFile(join(copy, ".vivarium", "checks.json"), JSON.stringify({ version: 1, checks: [{ name: "queue decision coverage", image: "alpine:3.22", command: "grep -qx 'strategy=bounded' queue.conf && grep -qx 'limit=100' queue.conf" }] }, null, 2) + "\n");
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Establish queue decision baseline"); await git(copy, "push", "origin", "main");
    const base = await git(copy, "rev-parse", "HEAD");
    await json(ownerPage, "put", `/repositories/${repository.id}/branches/main/required-checks`, owner.headers, { checks: ["queue decision coverage"] });

    let decision = await json(ownerPage, "post", `/repositories/${repository.id}/decisions`, owner.headers, { source: { kind: "repository" }, scope: { question: "How should burst traffic be bounded without losing operational visibility?", constraints: ["No downtime during adoption"], success_measures: ["p95 enqueue latency remains below 100ms"], deadline: new Date(Date.now() + 86400000).toISOString(), affected_resources: [{ kind: "repository", repository_id: repository.id, label: "Adaptive queue" }, { kind: "repository", repository_id: affectedRepository.id, label: "Traffic gateway" }], participants: [{ user_id: owner.user.id }, { user_id: peer.user.id }], owner_id: owner.user.id } }) as any;
    const evidence = { kind: "usage", resource_id: "burst-window", revision: "2026-08-10T20:00Z", label: "Production burst trace", captured_at: new Date().toISOString() };
    decision = await json(peerPage, "post", `/decisions/${decision.id}/alternatives`, peer.headers, { expected_version: decision.version, alternative: { title: "Bounded FIFO", summary: "Apply deterministic backpressure.", assumptions: ["A limit of 100 absorbs normal bursts"], tradeoffs: ["Excess work is rejected explicitly"], risks: ["Retry amplification"], compatibility_impact: "No wire change", cost: "Two implementation tasks", expected_outcomes: ["Stable latency"], evidence: [evidence], criteria: [{ criterion: "p95 enqueue latency remains below 100ms", outcome: "Prototype p95 78ms", evidence: [evidence] }] } });
    decision = await json(ownerPage, "post", `/decisions/${decision.id}/alternatives`, owner.headers, { expected_version: decision.version, alternative: { title: "Elastic workers", summary: "Scale consumers on every burst.", assumptions: ["Capacity arrives before queues grow"], tradeoffs: ["Higher idle cost"], risks: ["Slow scale-up"], compatibility_impact: "Requires runtime autoscaling", cost: "One week", expected_outcomes: ["More throughput"], evidence: [evidence], criteria: [{ criterion: "p95 enqueue latency remains below 100ms", outcome: "Prototype p95 126ms", evidence: [evidence] }] } });
    const bounded = decision.alternatives.find((item: any) => item.title === "Bounded FIFO");
    const elastic = decision.alternatives.find((item: any) => item.title === "Elastic workers");
    const research = await json(ownerPage, "post", `/decisions/${decision.id}/research-credentials`, owner.headers, { alternative_id: elastic.id, expires_in: 600 }) as any;
    decision = await json(ownerPage, "post", `/decisions/${decision.id}/findings`, { Authorization: `Bearer ${research.token}` }, { alternative_id: elastic.id, body: "The trace shows scale-up begins after the latency objective is already breached.", position: "oppose", uncertainty: "Only one production region was sampled.", citations: [evidence] });
    const dissent = decision.findings[0];

    for (const [page, headers, alternative, latency] of [[ownerPage, owner.headers, bounded, 78], [peerPage, peer.headers, bounded, 81], [ownerPage, owner.headers, elastic, 126]] as const) {
      const workspace = await json(page, "post", "/workspaces", headers, { repository_id: repository.id, commit_id: base, source: { kind: "decision_experiment", decision_id: decision.id, alternative_id: alternative.id } }) as any;
      decision = await json(page, "post", `/decisions/${decision.id}/experiments`, headers, { alternative_id: alternative.id, workspace_id: workspace.id });
      const experiment = decision.experiments.find((item: any) => item.workspace_id === workspace.id);
      decision = await json(page, "post", `/decisions/${decision.id}/experiments/${experiment.id}/evidence`, headers, { expected_version: experiment.version, evidence: { checkpoint_ids: [], command_ids: [], measurements: [{ name: "p95 enqueue latency", value: latency, unit: "ms" }], artifacts: [], notes: page === peerPage ? "Affected owner reproduced the bounded prototype." : "Exact-revision prototype run." } });
    }
    expect(decision.experiments.filter((item: any) => item.alternative_id === bounded.id)).toHaveLength(2);

    decision = await json(ownerPage, "post", `/decisions/${decision.id}/approval-requests`, owner.headers, { expected_version: decision.version, request: { kind: "affected_owner", repository_id: affectedRepository.id, approver_id: peer.user.id, reason: "Confirm the affected gateway owner accepts the reproduced prototype evidence." } });
    const approval = decision.approval_requests.at(-1);
    decision = await json(peerPage, "post", `/decisions/${decision.id}/approval-requests/${approval.id}/response`, peer.headers, { decision: "approve", note: "The reproduced result satisfies the declared threshold." });
    decision = await json(ownerPage, "post", `/decisions/${decision.id}/publish`, owner.headers, { expected_version: decision.version, commitment: { selected_alternative_id: bounded.id, rejected_alternative_ids: [elastic.id], rationale: "Two exact-revision runs met the target while the elastic prototype did not.", accepted_tradeoffs: ["Explicit overload rejection"], dissent_finding_ids: [dissent.id], conditions: ["Observe retry amplification after release"], review_date: new Date(Date.now() + 86400000).toISOString(), evidence: [evidence], exceptions: [] } });
    expect(decision.commitments[0]).toMatchObject({ status: "published", dissent_finding_ids: [dissent.id] });

    const implementation = await json(ownerPage, "post", `/decisions/${decision.id}/implementation`, owner.headers, { commitment_version: 1, title: "Deliver the evidence-backed bounded queue", body: "Preserve the chosen constraints and measurements through ordinary review.", tasks: [
      { title: "Implement bounded admission", assignee_type: "human", assignee_id: peer.user.id, constraint_indexes: [0], success_measure_indexes: [0], depends_on_previous: false },
      { title: "Add queue observability", assignee_type: "agent", constraint_indexes: [0], success_measure_indexes: [0], depends_on_previous: false },
    ] }) as any;
    const [humanTask, agentTask] = implementation.tasks;
    const peerCredential = await json(peerPage, "post", "/auth/credentials", peer.headers, { kind: "git", name: "human decision task", scopes: ["git:read", "git:write"], expires_in: 3600 }) as any;
    const humanCopy = await mkdtemp(join(tmpdir(), "vivarium-decision-human-")); copies.push(humanCopy);
    await git(tmpdir(), "clone", `http://git:${peerCredential.token}@localhost:3000/git/${repository.id}.git`, humanCopy);
    await git(humanCopy, "config", "user.name", "Affected Owner"); await git(humanCopy, "config", "user.email", "peer@example.test"); await git(humanCopy, "switch", "-c", "bounded-admission");
    await writeFile(join(humanCopy, "queue.conf"), "strategy=bounded\nlimit=100\n"); await git(humanCopy, "add", "queue.conf"); await git(humanCopy, "commit", "-m", "Implement bounded queue admission"); await git(humanCopy, "push", "origin", "bounded-admission");
    const humanPull = await json(peerPage, "post", `/repositories/${repository.id}/proposals/${implementation.proposal.id}/tasks/${humanTask.id}/contributions`, peer.headers, { title: "Implement bounded admission", body: "Human-authored implementation of the selected alternative.", source_branch: "bounded-admission", target_branch: "main" }) as any;
    await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${humanPull.id}/reviews`, owner.headers, { decision: "approved" });
    await expect.poll(async () => (await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${humanPull.id}/merge-readiness`, owner.headers)).blockers.length, { timeout: 60_000 }).toBe(0);
    await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${humanPull.id}/merge`, owner.headers, {});

    const tasks = await json(ownerPage, "get", `/repositories/${repository.id}/proposals/${implementation.proposal.id}/tasks`, owner.headers) as any;
    const readyAgent = tasks.tasks.find((item: any) => item.id === agentTask.id);
    expect(readyAgent.ready).toBe(true);
    const launched = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${implementation.proposal.id}/tasks/${agentTask.id}/sessions`, owner.headers, { expected_assignment_id: readyAgent.assignment.id, context_paths: ["queue.conf"], expires_in: 3600 }) as any;
    const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-decision-agent-")); copies.push(agentCopy);
    await git(tmpdir(), "clone", `http://git:${launched.credential.token}@localhost:3000/git/${repository.id}.git`, agentCopy); await git(agentCopy, "config", "user.name", "Decision Research Agent"); await git(agentCopy, "config", "user.email", "agent@agents.vivarium"); await git(agentCopy, "switch", launched.run.working_branch);
    await writeFile(join(agentCopy, "queue.conf"), "strategy=bounded\nlimit=100\n"); await writeFile(join(agentCopy, "queue-observability.md"), "# Measures\n\nTrack enqueue p95 and rejected work after release.\n"); await git(agentCopy, "add", "."); await git(agentCopy, "commit", "-m", "Add decision success measurement"); await git(agentCopy, "push", "origin", launched.run.working_branch);
    const agentCommit = await git(agentCopy, "rev-parse", "HEAD");
    const runPath = `/repositories/${repository.id}/proposals/${implementation.proposal.id}/tasks/${agentTask.id}/sessions/${launched.session.id}/runs/${launched.run.id}`;
    await json(ownerPage, "post", `${runPath}/completion`, { Authorization: `Bearer ${launched.credential.token}` }, { summary: "Added the declared post-release measurement plan.", commit_id: agentCommit, checks: [{ name: "measure coverage", status: "passed", details: "The accepted p95 measure is named." }], unresolved_concerns: ["The limit=100 assumption still requires production validation."] });
    const agentPull = await json(ownerPage, "post", `/repositories/${repository.id}/proposals/${implementation.proposal.id}/tasks/${agentTask.id}/contributions`, owner.headers, { title: "Add queue observability", body: "Agent-authored measurement for the accepted decision.", source_branch: launched.run.working_branch, target_branch: "main", session_id: launched.session.id, run_id: launched.run.id }) as any;
    await expect.poll(async () => (await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${agentPull.id}/merge-readiness`, owner.headers)).blockers.length, { timeout: 60_000 }).toBe(1);
    await json(peerPage, "post", `/repositories/${repository.id}/pulls/${agentPull.id}/reviews`, peer.headers, { decision: "approved" });
    await expect.poll(async () => (await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${agentPull.id}/merge-readiness`, owner.headers)).blockers.length, { timeout: 60_000 }).toBe(0);
    const agentMerge = await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${agentPull.id}/merge`, owner.headers, {}) as any;
    const release = await json(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: "v1.0.0", notes: "Evidence-backed bounded queue with retained measurement plan.", commit_id: agentMerge.merge_commit_id }) as any;
    decision = await json(peerPage, "post", `/decisions/${decision.id}/implementation/${implementation.proposal.id}/observations`, peer.headers, { kind: "failed_measure", summary: "Production p95 reached 142ms because the assumed limit of 100 amplified retries.", resource_kind: "release", resource_id: release.id });
    expect(decision).toMatchObject({ status: "pending", commitments: [expect.objectContaining({ status: "reopened", reopen_reason: expect.stringContaining("p95 reached 142ms") })] });

    await ownerPage.goto(`/decisions/${decision.id}`);
    await expect(ownerPage.getByRole("heading", { name: decision.scope.question })).toBeVisible();
    await expect(ownerPage.getByText("Affected owner reproduced the bounded prototype.")).toBeVisible();
    await expect(ownerPage.getByText(/Reopened: failed_measure: Production p95 reached 142ms/)).toBeVisible();
    await expect(ownerPage.getByText(/Two exact-revision runs met the target/).first()).toBeVisible();
    expect(decision.findings[0]).toMatchObject({ actor_id: owner.user.id, uncertainty: "Only one production region was sampled." });
    expect(decision.implementations[0]).toMatchObject({ proposal_id: implementation.proposal.id, task_ids: [humanTask.id, agentTask.id] });
  } finally {
    await Promise.all(copies.map((path) => rm(path, { recursive: true, force: true })));
  }
});
