import { expect, test, type Page } from "@playwright/test";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import { execFile } from "node:child_process";

/* eslint-disable @typescript-eslint/no-explicit-any -- journey response shapes intentionally retain only the cross-workflow fields asserted below */

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

async function json(page: Page, method: "get" | "post" | "put" | "patch", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}

async function eventually<T>(read: () => Promise<T>, ready: (value: T) => boolean, message: string) {
  await expect.poll(async () => ready(await read()), { message, timeout: 60_000, intervals: [250, 500, 1000] }).toBe(true);
  return read();
}

test("an independently owned ecosystem evolves one contract through agent and human delivery", async ({ browser }) => {
  test.setTimeout(300_000);
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const suffix = Date.now().toString(36);
  const providerPage = await (await browser.newContext()).newPage();
  const consumerPage = await (await browser.newContext()).newPage();
  const reviewerPage = await (await browser.newContext()).newPage();
  const providerActor = await account(providerPage, "Evolution Provider", `evo-provider-${suffix}`);
  const consumerActor = await account(consumerPage, "Evolution Consumer", `evo-consumer-${suffix}`);
  const reviewerActor = await account(reviewerPage, "Evolution Reviewer", `evo-reviewer-${suffix}`);

  const provider = await json(providerPage, "post", "/repositories", providerActor.headers, { name: `events-${suffix}` }) as { id: string };
  const consumer = await json(consumerPage, "post", "/repositories", consumerActor.headers, { name: `calendar-${suffix}` }) as { id: string };
  await json(providerPage, "patch", `/repositories/${provider.id}`, providerActor.headers, { visibility: "public" });
  await json(consumerPage, "patch", `/repositories/${consumer.id}`, consumerActor.headers, { visibility: "public" });
  await json(providerPage, "post", `/repositories/${provider.id}/collaborators`, providerActor.headers, { user_id: reviewerActor.user.id });
  await json(consumerPage, "post", `/repositories/${consumer.id}/collaborators`, consumerActor.headers, { user_id: reviewerActor.user.id });

  const gitCredential = async (page: Page, headers: Record<string, string>, name: string) =>
    json(page, "post", "/auth/credentials", headers, { kind: "git", name, scopes: ["git:read", "git:write"], expires_in: 3600 }) as Promise<{ token: string }>;
  const providerGit = await gitCredential(providerPage, providerActor.headers, "provider git");
  const consumerGit = await gitCredential(consumerPage, consumerActor.headers, "consumer git");
  const providerCopy = await mkdtemp(join(tmpdir(), "vivarium-evolution-provider-"));
  const consumerCopy = await mkdtemp(join(tmpdir(), "vivarium-evolution-consumer-"));
  await git(tmpdir(), "clone", `http://git:${providerGit.token}@localhost:3000/git/${provider.id}.git`, providerCopy);
  await git(tmpdir(), "clone", `http://git:${consumerGit.token}@localhost:3000/git/${consumer.id}.git`, consumerCopy);
  for (const [copy, name] of [[providerCopy, "Evolution Provider"], [consumerCopy, "Evolution Consumer"]]) {
    await git(copy, "config", "user.name", name);
    await git(copy, "config", "user.email", `${name.replaceAll(" ", "-").toLowerCase()}@example.com`);
    await mkdir(join(copy, ".vivarium"));
    await writeFile(join(copy, ".vivarium", "release.json"), JSON.stringify({ version: 1, steps: [{ name: "package", image: "alpine:3.22", command: "cp artifact.txt \"$VIVARIUM_OUTPUT/app.txt\"" }] }));
    await writeFile(join(copy, ".vivarium", "deployment.json"), JSON.stringify({ version: 1, stages: [{ name: "adoption", signals: [{ name: "artifact is compatible", command: "grep -q compatible \"$VIVARIUM_ARTIFACT\"" }] }] }));
    await writeFile(join(copy, "artifact.txt"), "compatible baseline\n");
  }
  await writeFile(join(providerCopy, "interface.txt"), "events=v1\n");
  await writeFile(join(consumerCopy, "consumer.txt"), "events=v1\n");
  for (const copy of [providerCopy, consumerCopy]) {
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Publish compatible baseline"); await git(copy, "push", "origin", "main");
  }
  const providerBase = await git(providerCopy, "rev-parse", "HEAD");
  const consumerBase = await git(consumerCopy, "rev-parse", "HEAD");
  const predecessorRelease = await json(providerPage, "post", `/repositories/${provider.id}/releases`, providerActor.headers, { version: "v1.0.0", notes: "Supported events v1 contract", commit_id: providerBase }) as { id: string };

  await providerPage.goto(`/repositories/${provider.id}/relationships`);
  await providerPage.getByPlaceholder("Interface name").first().fill("events");
  await providerPage.locator('select[name="release_id"]').first().selectOption(predecessorRelease.id);
  const publication = providerPage.waitForResponse((response) => response.request().method() === "POST" && response.url().endsWith(`/repositories/${provider.id}/interfaces`));
  await providerPage.getByRole("button", { name: "Publish interface" }).click();
  expect((await publication).status()).toBe(201);
  await expect(providerPage.getByText("events", { exact: true }).last()).toBeVisible();
  const graph = await json(providerPage, "get", `/repositories/${provider.id}/relationships`, providerActor.headers) as { interfaces: Array<{ id: string }> };

  await consumerPage.goto(`/repositories/${consumer.id}/relationships`);
  await consumerPage.getByPlaceholder("Provider repository ID").fill(provider.id);
  await consumerPage.locator('input[name="interface_name"]').last().fill("events");
  await consumerPage.getByPlaceholder(">=v1.0.0 <v2.0.0").fill(">=v1.0.0 <v2.0.0");
  await consumerPage.locator('select[name="commit_id"]').selectOption(consumerBase);
  await consumerPage.getByRole("button", { name: "Declare dependency" }).click();
  await expect(consumerPage.getByText("resolved", { exact: true })).toBeVisible();

  const proposal = await json(providerPage, "post", `/repositories/${provider.id}/proposals`, providerActor.headers, { title: "Evolve events to v2", body: "Coordinate every known consumer before removing events v1." }) as { id: string };
  const evolution = await json(providerPage, "post", `/repositories/${provider.id}/evolutions`, providerActor.headers, {
    interface_name: "events", predecessor_interface_id: graph.interfaces[0].id, source_kind: "proposal", source_id: proposal.id,
    candidate_description: "events v2 replaces the legacy event shape", changes: [{ kind: "schema", summary: "consumers must adopt events v2", classification: "breaking" }],
    strategy: "test provider and consumer candidates together", sequencing: "deploy the consumer before the provider", exceptions: "none",
  }) as { id: string; version: number; impacts: Array<{ repository_id: string }> };
  expect(evolution.impacts.map((impact) => impact.repository_id)).toContain(consumer.id);
  await providerPage.goto(`/repositories/${provider.id}/relationships`);
  await expect(providerPage.getByText(`calendar-${suffix}`, { exact: true }).first()).toBeVisible();
  await expect(providerPage.getByText("events v2 replaces the legacy event shape")).toBeVisible();

  const analysis = await json(providerPage, "post", `/repositories/${provider.id}/evolutions/${evolution.id}/analyses`, providerActor.headers, { mandate: "Trace exact v1 use and migration uncertainty", repository_ids: [provider.id, consumer.id], expires_in: 3600 }) as { analysis: { id: string; agent_id: string }; credential: { token: string; scopes: string[] } };
  expect(analysis.credential.scopes).toEqual(["evolutions:analyze"]);
  const analysisHeaders = { Authorization: `Bearer ${analysis.credential.token}` };
  await json(providerPage, "post", `/repositories/${provider.id}/evolutions/${evolution.id}/analyses/${analysis.analysis.id}/findings`, analysisHeaders, { repository_ids: [consumer.id], finding: "calendar reads the v1 event marker and must migrate first", uncertainty: "runtime users outside declared repositories are not inferred" });
  const denied = await providerPage.request.patch(`/api/repositories/${consumer.id}`, { headers: analysisHeaders, data: { visibility: "private" } });
  expect(denied.status()).toBeGreaterThanOrEqual(400);

  let plan = await json(providerPage, "get", `/repositories/${provider.id}/evolutions/${evolution.id}`, providerActor.headers) as any;
  const consumerTaskResult = await json(consumerPage, "post", `/repositories/${provider.id}/evolutions/${evolution.id}/migration-tasks`, consumerActor.headers, {
    version: plan.version, repository_id: consumer.id, title: "Adopt events v2 in calendar", completion_criteria: "consumer.txt selects events=v2", target_version: "v2.0.0", dependency_ids: [], assignee_type: "human", assignee_id: consumerActor.user.id, mandate: "Migrate through an independently owned fork", base_revision: consumerBase,
  }) as any;
  const consumerTask = consumerTaskResult.migration_task;
  plan = consumerTaskResult.evolution;
  const providerTaskResult = await json(providerPage, "post", `/repositories/${provider.id}/evolutions/${evolution.id}/migration-tasks`, providerActor.headers, {
    version: plan.version, repository_id: provider.id, title: "Publish events v2", completion_criteria: "interface.txt selects events=v2", target_version: "v2.0.0", dependency_ids: [], assignee_type: "agent", mandate: "Publish the verified v2 provider candidate", base_revision: providerBase,
  }) as any;
  const providerTask = providerTaskResult.migration_task;
  const assignment = providerTaskResult.task.assignment;

  const fork = await json(consumerPage, "post", `/repositories/${consumer.id}/forks`, consumerActor.headers, { name: `calendar-migration-${suffix}` }) as { id: string };
  const forkCopy = await mkdtemp(join(tmpdir(), "vivarium-evolution-fork-"));
  await git(tmpdir(), "clone", `http://git:${consumerGit.token}@localhost:3000/git/${fork.id}.git`, forkCopy);
  await git(forkCopy, "config", "user.name", "Evolution Consumer"); await git(forkCopy, "config", "user.email", "consumer@example.com");
  await git(forkCopy, "switch", "-c", "events-v2"); await writeFile(join(forkCopy, "consumer.txt"), "events=v2\n");
  await git(forkCopy, "add", "consumer.txt"); await git(forkCopy, "commit", "-m", "Adopt events v2"); await git(forkCopy, "push", "origin", "events-v2");
  await json(consumerPage, "patch", `/repositories/${fork.id}`, consumerActor.headers, { visibility: "public" });
  const consumerPull = await json(consumerPage, "post", `/repositories/${consumer.id}/proposals/${consumerTask.proposal_id}/tasks/${consumerTask.task_id}/contributions`, consumerActor.headers, { title: "Adopt events v2", body: "Human migration from an independently owned fork.", source_repository_id: fork.id, source_branch: "events-v2", target_branch: "main" }) as { id: string };

  const launched = await json(providerPage, "post", `/repositories/${provider.id}/proposals/${providerTask.proposal_id}/tasks/${providerTask.task_id}/sessions`, providerActor.headers, { expected_assignment_id: assignment.id, context_paths: ["interface.txt"], expires_in: 3600 }) as any;
  const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-evolution-agent-"));
  await git(tmpdir(), "clone", `http://git:${launched.credential.token}@localhost:3000/git/${provider.id}.git`, agentCopy);
  await git(agentCopy, "config", "user.name", "Vivarium Evolution Agent"); await git(agentCopy, "config", "user.email", "agent@users.vivarium");
  await git(agentCopy, "switch", "-c", "agent-evolution", `origin/${launched.run.working_branch}`);
  await writeFile(join(agentCopy, "interface.txt"), "events=v2\n");
  await writeFile(join(agentCopy, ".vivarium", "contracts.json"), JSON.stringify({ version: 1, checks: [{ name: "provider-consumer contract", image: "alpine:3.22", command: "grep -qx 'events=v2' provider/interface.txt && find consumers -name consumer.txt -exec grep -qx 'events=v2' {} \\; && echo exact-compatible > \"$VIVARIUM_OUTPUT/attestation.txt\"" }] }));
  await git(agentCopy, "add", "."); await git(agentCopy, "commit", "-m", "Publish events v2 contract"); await git(agentCopy, "push", "origin", `HEAD:refs/heads/${launched.run.working_branch}`);
  const agentCommit = await git(agentCopy, "rev-parse", "HEAD");
  const runPath = `/repositories/${provider.id}/proposals/${providerTask.proposal_id}/tasks/${providerTask.task_id}/sessions/${launched.session.id}/runs/${launched.run.id}`;
  await json(providerPage, "post", `${runPath}/completion`, { Authorization: `Bearer ${launched.credential.token}` }, { summary: "Published events v2 and its exact combination check.", commit_id: agentCommit, checks: [{ name: "contract definition", status: "passed" }], unresolved_concerns: [] });
  const providerPull = await json(providerPage, "post", `/repositories/${provider.id}/proposals/${providerTask.proposal_id}/tasks/${providerTask.task_id}/contributions`, providerActor.headers, { title: "Publish events v2", body: "Agent-authored provider migration.", source_branch: launched.run.working_branch, target_branch: "main", session_id: launched.session.id, run_id: launched.run.id }) as { id: string };

  const candidateResult = await json(providerPage, "post", `/repositories/${provider.id}/evolutions/${evolution.id}/contract-candidates`, providerActor.headers, { provider_pull_request_id: providerPull.id, consumer_pull_request_ids: { [consumer.id]: consumerPull.id } }) as any;
  const candidate = candidateResult.candidate;
  const checks = await eventually(
    () => json(providerPage, "get", `/repositories/${provider.id}/evolutions/${evolution.id}/contract-candidates/${candidate.id}/checks`, providerActor.headers) as Promise<any>,
    (value) => value.check_runs?.every((check: any) => check.state === "succeeded"), "exact provider/consumer contract check succeeds",
  );
  expect(checks.check_runs[0].artifacts[0].sha256).toMatch(/^[a-f0-9]{64}$/);

  const environmentInput = { name: "production", position: 1, image: "alpine:3.22", command: "grep -q compatible \"$VIVARIUM_ARTIFACT\"", timeout_seconds: 30, configuration: {}, credentials: {}, required_approvals: 0, concurrency: 1 };
  const consumerEnvironment = await json(consumerPage, "post", `/repositories/${consumer.id}/environments`, consumerActor.headers, environmentInput) as any;
  const providerEnvironment = await json(providerPage, "post", `/repositories/${provider.id}/environments`, providerActor.headers, environmentInput) as any;

  plan = await json(providerPage, "get", `/repositories/${provider.id}/evolutions/${evolution.id}`, providerActor.headers) as any;
  plan = await json(providerPage, "put", `/repositories/${provider.id}/evolutions/${evolution.id}/rollout`, providerActor.headers, { version: plan.version, candidate_id: candidate.id, phases: [
    { name: "Consumer adoption", repository_ids: [consumer.id], migration_task_ids: { [consumer.id]: consumerTask.id }, environment_ids: { [consumer.id]: consumerEnvironment.id } },
    { name: "Provider publication", repository_ids: [provider.id], migration_task_ids: { [provider.id]: providerTask.id }, environment_ids: { [provider.id]: providerEnvironment.id } },
  ] });
  await json(consumerPage, "post", `/repositories/${provider.id}/evolutions/${evolution.id}/rollout/approvals`, consumerActor.headers, { version: plan.version, repository_id: consumer.id });
  plan = await json(providerPage, "get", `/repositories/${provider.id}/evolutions/${evolution.id}`, providerActor.headers) as any;
  await json(providerPage, "post", `/repositories/${provider.id}/evolutions/${evolution.id}/rollout/approvals`, providerActor.headers, { version: plan.version, repository_id: provider.id });

  async function approveAndMerge(page: Page, repositoryID: string, pullID: string, ownerHeaders: Record<string, string>) {
    await json(reviewerPage, "post", `/repositories/${repositoryID}/pulls/${pullID}/reviews`, reviewerActor.headers, { decision: "approved" });
    return json(page, "post", `/repositories/${repositoryID}/pulls/${pullID}/merge`, ownerHeaders, {}) as Promise<{ merge_commit_id: string }>;
  }
  async function releaseAndDeploy(page: Page, repositoryID: string, headers: Record<string, string>, environment: any, version: string, commitID: string) {
    const release = await json(page, "post", `/repositories/${repositoryID}/releases`, headers, { version, notes: "Governed ecosystem adoption", commit_id: commitID }) as any;
    await json(page, "post", `/repositories/${repositoryID}/releases/${release.id}/builds`, headers, {});
    const builds = await eventually(() => json(page, "get", `/repositories/${repositoryID}/releases/${release.id}/builds`, headers) as Promise<any>, (value) => value.builds?.every((build: any) => build.state === "succeeded"), `${repositoryID} release builds`);
    const deployment = await json(page, "post", `/repositories/${repositoryID}/deployments`, headers, { environment_id: environment.id, release_id: release.id, build_id: builds.builds[0].id, artifact_id: builds.builds[0].artifacts[0].id }) as any;
    const finishedSet = await eventually(() => json(page, "get", `/repositories/${repositoryID}/deployments`, headers) as Promise<any>, (value) => value.deployments?.some((item: any) => item.id === deployment.id && item.state === "succeeded"), `${repositoryID} deploys`);
    const finished = finishedSet.deployments.find((item: any) => item.id === deployment.id);
    return { release, environment, deployment: finished };
  }
  const consumerMerge = await approveAndMerge(consumerPage, consumer.id, consumerPull.id, consumerActor.headers);
  const consumerDelivery = await releaseAndDeploy(consumerPage, consumer.id, consumerActor.headers, consumerEnvironment, "v2.0.0", consumerMerge.merge_commit_id);
  plan = await json(providerPage, "get", `/repositories/${provider.id}/evolutions/${evolution.id}`, providerActor.headers) as any;
  expect(plan.rollout.phases[0].state).toBe("completed");
  expect(plan.rollout.phases[1].state).toBe("ready");

  const providerMerge = await approveAndMerge(providerPage, provider.id, providerPull.id, providerActor.headers);
  const providerDelivery = await releaseAndDeploy(providerPage, provider.id, providerActor.headers, providerEnvironment, "v2.0.0", providerMerge.merge_commit_id);
  plan = await eventually(() => json(providerPage, "get", `/repositories/${provider.id}/evolutions/${evolution.id}`, providerActor.headers) as Promise<any>, (value) => value.rollout?.state === "completed", "ecosystem rollout completes");
  expect(plan.rollout.outcomes).toEqual(expect.arrayContaining([
    expect.objectContaining({ repository_id: consumer.id, release_id: consumerDelivery.release.id, deployment_id: consumerDelivery.deployment.id, state: "succeeded" }),
    expect.objectContaining({ repository_id: provider.id, release_id: providerDelivery.release.id, deployment_id: providerDelivery.deployment.id, state: "succeeded" }),
  ]));
  await providerPage.goto(`/repositories/${provider.id}/relationships`);
  await expect(providerPage.getByText("completed", { exact: true }).first()).toBeVisible();
  await expect(providerPage.getByText("calendar reads the v1 event marker and must migrate first")).toBeVisible();
  await expect(providerPage.getByText("owner approved", { exact: false }).first()).toBeVisible();
});
