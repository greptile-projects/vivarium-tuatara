import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this connected journey inspects several public ledgers */

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

async function json(page: Page, method: "get" | "post" | "put" | "delete", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](path.startsWith("http") ? path : `/api${path}`, { headers, data });
  const text = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${text}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${text}`).toBeLessThan(300);
  return text ? JSON.parse(text) : undefined;
}

async function rejected(page: Page, method: "get" | "post", path: string, headers: Record<string, string>, data: unknown, status: number, code: string) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  expect(response.status(), `${method.toUpperCase()} ${path}: ${await response.text()}`).toBe(status);
  expect(await response.json()).toMatchObject({ error: { code } });
}

test("conflicting human and agent intent becomes one verified queued result", async ({ browser }) => {
  test.setTimeout(420_000);
  const dockerAvailable = await run("docker", ["info"]).then(() => true, () => false);
  test.skip(!dockerAvailable, "conflict checkpoints require the Docker-backed ordinary check executor");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  await run("docker", ["image", "inspect", "docker:28-cli"]).catch(() => run("docker", ["pull", "docker:28-cli"]));
  const copies: string[] = [];
  const credentials: Array<{ page: Page; headers: Record<string, string>; id: string }> = [];
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const behaviorPage = await (await browser.newContext()).newPage();
    const reliabilityPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, "Conflict Maintainer", `conflict-maintainer-${suffix}`);
    const behavior = await account(behaviorPage, "Behavior Owner", `behavior-owner-${suffix}`);
    const reliability = await account(reliabilityPage, "Reliability Owner", `reliability-owner-${suffix}`);
    async function trackedCredential(page: Page, headers: Record<string, string>, path: string, data: unknown) {
      const credential = await json(page, "post", path, headers, data);
      credentials.push({ page, headers, id: credential.id });
      return credential;
    }

    const organization = await json(ownerPage, "post", "/organizations", owner.headers, {
      name: `Conflict Guild ${suffix}`, slug: `conflict-guild-${suffix}`, description: "Resolve parallel work without losing intent.",
    });
    for (const actor of [{ page: behaviorPage, account: behavior }, { page: reliabilityPage, account: reliability }]) {
      const state = await json(ownerPage, "post", `/organizations/${organization.id}/invitations`, owner.headers, { user_id: actor.account.user.id });
      await json(actor.page, "post", `/organizations/${organization.id}/invitations/${state.invitations.at(-1).id}/accept`, actor.account.headers);
    }
    const repository = await json(ownerPage, "post", `/organizations/${organization.id}/repositories`, owner.headers, { name: `intent-runtime-${suffix}` });
    let org = await json(ownerPage, "post", `/organizations/${organization.id}/agents`, owner.headers, {
      name: "Conflict Assistant", slug: `conflict-assistant-${suffix}`, capabilities: ["compare exact revisions", "propose bounded resolutions"], operator_ids: [owner.user.id], team_ids: [],
    });
    const agent = org.agents.find((item: any) => item.slug === `conflict-assistant-${suffix}`);
    const expiry = new Date(Date.now() + 3_600_000).toISOString();
    const requestState = await json(ownerPage, "post", `/organizations/${organization.id}/access-requests`, owner.headers, {
      principal_type: "agent", principal_id: agent.id, role: "contributor", resources: [{ kind: "repository", id: repository.id }], exceptions: [], reason: "Assist only inside one reconciliation workspace.", expires_at: expiry,
    });
    org = await json(ownerPage, "post", `/organizations/${organization.id}/access-requests/${requestState.access_requests.at(-1).id}/decision`, owner.headers, { decision: "approve" });
    const grant = org.access_grants.at(-1);
    const agentCredential = await trackedCredential(ownerPage, owner.headers, `/organizations/${organization.id}/access-grants/${grant.id}/credentials`, { agent_id: agent.id, repository_id: repository.id, expires_in: 3600, purpose: "api_read" });
    const agentHeaders = { Authorization: `Bearer ${agentCredential.token}` };

    const ownerGit = await trackedCredential(ownerPage, owner.headers, "/auth/credentials", { kind: "git", name: "conflict journey", scopes: ["git:read", "git:write"], expires_in: 3600 });
    const copy = await mkdtemp(join(tmpdir(), "vivarium-conflict-owner-")); copies.push(copy);
    await git(tmpdir(), "clone", `http://git:${ownerGit.token}@localhost:3000/git/${repository.id}.git`, copy);
    await git(copy, "config", "user.name", "Conflict Maintainer"); await git(copy, "config", "user.email", "maintainer@example.test");
    await mkdir(join(copy, ".vivarium"));
    await writeFile(join(copy, "service.go"), "package service\n\nfunc Policy() string {\n\treturn \"timeout=10 retries=1\"\n}\n");
    await writeFile(join(copy, ".vivarium", "workspace.json"), JSON.stringify({ version: 1, image: "docker:28-cli", tools: [{ name: "git", version: "bundled" }], dependencies: ["git", "sh"], setup: ["test -f service.go"], resources: { cpus: 1, memory_mb: 256, storage_mb: 128, setup_seconds: 30 } }));
    await writeFile(join(copy, ".vivarium", "checks.json"), JSON.stringify({ version: 1, checks: [{ name: "repository safety", image: "alpine:3.22", command: "test -f service.go" }] }));
    await writeFile(join(copy, ".vivarium", "packages.json"), JSON.stringify({ version: 1, dependencies: [], lock: [] }));
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Establish runtime policy"); await git(copy, "push", "origin", "main");
    await json(ownerPage, "put", `/repositories/${repository.id}/branches/main/required-checks`, owner.headers, { checks: ["repository safety"] });
    await json(ownerPage, "put", `/repositories/${repository.id}/branches/main/integration-queue`, owner.headers, { enabled: true, concurrency: 2, failure_behavior: "remove" });

    const behaviorGit = await trackedCredential(behaviorPage, behavior.headers, "/auth/credentials", { kind: "git", name: "behavior work", scopes: ["git:read", "git:write"], expires_in: 3600 });
    const reliabilityGit = await trackedCredential(reliabilityPage, reliability.headers, "/auth/credentials", { kind: "git", name: "reliability work", scopes: ["git:read", "git:write"], expires_in: 3600 });
    async function contribution(prefix: string, token: string, branch: string, contents: string, author: string) {
      const path = await mkdtemp(join(tmpdir(), prefix)); copies.push(path);
      await git(tmpdir(), "clone", `http://git:${token}@localhost:3000/git/${repository.id}.git`, path);
      await git(path, "config", "user.name", author); await git(path, "config", "user.email", `${branch}@example.test`); await git(path, "switch", "-c", branch);
      await writeFile(join(path, "service.go"), contents); await git(path, "add", "service.go"); await git(path, "commit", "-m", `Define ${branch} policy`); await git(path, "push", "origin", branch);
      return path;
    }
    await contribution("vivarium-conflict-behavior-", behaviorGit.token, "behavior-timeout", "package service\n\nfunc Policy(timeoutSeconds int) string {\n\treturn \"timeout=30 retries=1\"\n}\n", "Behavior Owner");
    const reliabilityCopy = await contribution("vivarium-conflict-reliability-", reliabilityGit.token, "reliability-retries", "package service\n\nfunc Policy(maxRetries int) string {\n\treturn \"timeout=10 retries=3\"\n}\n", "Reliability Owner");
    const targetPull = await json(behaviorPage, "post", `/repositories/${repository.id}/pulls`, behavior.headers, { title: "Bound request timeout", body: "Acceptance: Policy returns timeout=30 while retaining retry behavior.", source_branch: "behavior-timeout", target_branch: "main" });
    let sourcePull = await json(reliabilityPage, "post", `/repositories/${repository.id}/pulls`, reliability.headers, { title: "Retry transient requests", body: "Acceptance: Policy returns retries=3 while retaining the timeout contract.", source_branch: "reliability-retries", target_branch: "main" });
    for (const pull of [targetPull, sourcePull]) await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/reviews`, owner.headers, { decision: "approved" });
    for (const pull of [targetPull, sourcePull]) {
      await expect.poll(async () => (await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${pull.id}/merge-readiness`, owner.headers)).can_enqueue, { timeout: 60_000 }).toBe(true);
      await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/queue`, owner.headers);
    }
    await expect.poll(async () => {
      const target = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${targetPull.id}`, owner.headers);
      const source = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${sourcePull.id}`, owner.headers);
      return `${target.status}:${Boolean(source.queued_at)}`;
    }, { timeout: 120_000, intervals: [1000, 2000, 5000] }).toBe("merged:false");
    sourcePull = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${sourcePull.id}`, owner.headers);
    const firstCandidate = sourcePull.integration_candidates.at(-1);
    const stale = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${sourcePull.id}/conflict-analysis?candidate_id=${firstCandidate.id}`, owner.headers);
    expect(stale.status).toBe("stale");

    // A concurrent source update invalidates the first evidence, and a second queue attempt retains another conflict instead of overwriting either branch.
    await writeFile(join(reliabilityCopy, "notes.md"), "Retry owner clarified that three attempts are a hard maximum.\n");
    await git(reliabilityCopy, "add", "notes.md"); await git(reliabilityCopy, "commit", "-m", "Clarify retry acceptance"); await git(reliabilityCopy, "push", "origin", "reliability-retries");
    await json(reliabilityPage, "post", `/repositories/${repository.id}/pulls/${sourcePull.id}/synchronize`, reliability.headers);
    sourcePull = await json(reliabilityPage, "post", `/repositories/${repository.id}/pulls`, reliability.headers, { title: "Retry the conflicting reliability intent", body: "Acceptance: Policy returns retries=3 while retaining the timeout contract.", source_branch: "reliability-retries", target_branch: "main" });
    await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${sourcePull.id}/reviews`, owner.headers, { decision: "approved" });
    const repeatedReadiness = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${sourcePull.id}/merge-readiness`, owner.headers);
    expect(repeatedReadiness).toMatchObject({ can_enqueue: false, has_conflicts: true });
    const repeatedQueue = await ownerPage.request.post(`/api/repositories/${repository.id}/pulls/${sourcePull.id}/queue`, { headers: owner.headers });
    expect(repeatedQueue.status()).toBe(409);
    const conflict = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${sourcePull.id}/conflict-analysis`, owner.headers);
    expect(conflict.files[0]).toMatchObject({ path: "service.go", kinds: expect.arrayContaining(["textual"]) });
    expect(conflict.semantic_incompatibilities).toEqual(expect.arrayContaining([expect.objectContaining({ symbol: "Policy", detector: "independent_symbol_overlap" })]));
    expect(conflict.source.pull_requests).toEqual(expect.arrayContaining([expect.objectContaining({ title: "Retry the conflicting reliability intent", author_id: reliability.user.id })]));
    expect(conflict.source.pull_requests.some((item: any) => item.decision_ids.length > 0)).toBe(true);

    const workspace = await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${sourcePull.id}/conflict-workspaces`, owner.headers, { launch_id: `resolve-${suffix}` });
    expect(workspace, JSON.stringify(workspace)).toMatchObject({ state: "running" });
    await json(ownerPage, "post", `/workspaces/${workspace.id}/conflict-invitations`, owner.headers, { principal_kind: "human", principal_id: behavior.user.id, role: "timeout owner" });
    await json(ownerPage, "post", `/workspaces/${workspace.id}/conflict-invitations`, owner.headers, { principal_kind: "human", principal_id: reliability.user.id, role: "retry owner" });
    let shared = await json(ownerPage, "post", `/workspaces/${workspace.id}/conflict-invitations`, owner.headers, { principal_kind: "approved_agent", principal_id: agent.id, role: "bounded resolution assistant" });
    await json(behaviorPage, "post", `/workspaces/${workspace.id}/conflict-invitations/respond`, behavior.headers, { status: "accepted" });
    await json(reliabilityPage, "post", `/workspaces/${workspace.id}/conflict-invitations/respond`, reliability.headers, { status: "accepted" });
    shared = await json(ownerPage, "get", `/workspaces/${workspace.id}`, owner.headers);
    shared = await json(ownerPage, "put", `/workspaces/${workspace.id}/control`, owner.headers, { expected_version: shared.control.version, principal_kind: "approved_agent", principal_id: agent.id, mode: "execute", scopes: ["files", "commands"], expires_in: 900 });
    const comparison = await json(ownerPage, "get", `/workspaces/${workspace.id}/conflict-comparison?path=service.go`, owner.headers);
    expect(comparison.source.revision).toBe(sourcePull.source_commit_id);
    expect(comparison.target.revision).toBe(conflict.target.commit_id);
    const citation = { side: "source", revision: sourcePull.source_commit_id, path: "service.go" };
    shared = await json(ownerPage, "post", `/workspaces/${workspace.id}/conflict-questions`, owner.headers, { expected_version: shared.conflict_context.version, body: "Must the timeout and retry limits both remain observable?", uncertainty: "The combined order is not specified.", citations: [citation] });
    shared = await json(ownerPage, "post", `/workspaces/${workspace.id}/conflict-questions/${shared.conflict_context.questions.at(-1).id}/answer`, agentHeaders, { expected_version: shared.conflict_context.version, body: "Suggestion: combine both values but omit the package declaration as unrelated boilerplate.", uncertainty: "This may not compile and requires human rejection or verification.", citations: [citation] });
    shared = await json(ownerPage, "get", `/workspaces/${workspace.id}`, owner.headers);
    const preservation = [{ kind: "acceptance_criterion", reference: "timeout=30 and retries=3", disposition: "preserved", rationale: "Retain both reviewed values.", citations: [citation] }];
    shared = await json(ownerPage, "put", `/workspaces/${workspace.id}/control`, owner.headers, { expected_version: shared.control.version, principal_kind: "human", principal_id: owner.user.id, mode: "execute", scopes: ["files", "commands"], expires_in: 900 });
    shared = await json(ownerPage, "post", `/workspaces/${workspace.id}/conflict-resolutions`, owner.headers, { expected_version: shared.conflict_context.version, path: "service.go", summary: "Reject the agent omission; keep the package and combine both reviewed limits", proposed_content: "package service\n\nfunc Policy(timeoutSeconds, maxRetries int) string {\n\treturn \"timeout=30 retries=3\"\n}\n", expected_sha256: comparison.proposed.sha256, uncertainty: "Operational latency still needs observation.", preservation });
    const acceptedResolution = shared.conflict_context.resolutions.at(-1);
    shared = await json(ownerPage, "post", `/workspaces/${workspace.id}/conflict-resolutions/${acceptedResolution.id}/apply`, owner.headers, { expected_version: shared.conflict_context.version });

    const criterion = (kind: string, name: string, command: string) => ({ kind, name, origin: "both", command, exact_criteria: [`${name} passes`], coverage: ["service.go"], owner_ids: [owner.user.id], artifacts: [], cost: 0 });
    const criteria = (conflictCommand: string) => [
      criterion("required_check", "repository safety", "test -f service.go"),
      criterion("reproduction", "timeout behavior", "grep -q 'timeout=30' service.go"),
      criterion("contract", "retry contract", "grep -q 'retries=3' service.go"),
      criterion("schema", "Go package shape", "grep -q '^package service' service.go"),
      criterion("preview_acceptance", "reviewed combined text", "grep -q 'timeout=30 retries=3' service.go"),
      criterion("conflict_test", "combined compile guard", conflictCommand),
    ];
    const checkpointEndpoint = `http://127.0.0.1:8080/workspaces/${workspace.id}/conflict-checkpoints`;
    shared = await json(ownerPage, "post", checkpointEndpoint, owner.headers, { expected_version: shared.conflict_context.version, criteria: criteria("test -f missing-combined-proof") });
    expect(shared.conflict_context.checkpoints.at(-1).criteria).toEqual(expect.arrayContaining([expect.objectContaining({ kind: "conflict_test", state: "failed" })]));
    shared = await json(ownerPage, "post", checkpointEndpoint, owner.headers, { expected_version: shared.conflict_context.version, criteria: criteria("grep -q '^func Policy' service.go") });
    const checkpoint = shared.conflict_context.checkpoints.at(-1);
    expect(checkpoint.criteria.every((item: any) => item.state === "passed")).toBe(true);
    for (const item of checkpoint.criteria) shared = await json(ownerPage, "post", `/workspaces/${workspace.id}/conflict-checkpoints/${checkpoint.id}/criteria/${item.id}/decision`, owner.headers, { expected_version: shared.conflict_context.version, decision: "accepted", rationale: "The exact combined candidate preserves both reviewed outcomes." });

    shared = await json(ownerPage, "get", `/workspaces/${workspace.id}`, owner.headers);
    const publication = await json(ownerPage, "post", `/workspaces/${workspace.id}/conflict-checkpoints/${checkpoint.id}/publications`, owner.headers, { expected_version: shared.conflict_context.version, publication_id: `publish-${suffix}`, mode: "resolution_pull", branch: `resolution/${suffix}`, title: "Integrate timeout and retry intent", body: "Publishes the exact accepted two-parent conflict result." });
    const resolutionPull = publication.pull_request;
    expect(publication.publication).toMatchObject({ status: "published", published_by: { actor_id: owner.user.id } });
    expect(publication.publication.published_commit_id).toMatch(/^[a-f0-9]{40}$/);
    await json(reliabilityPage, "post", `/repositories/${repository.id}/pulls/${resolutionPull.id}/reviews`, reliability.headers, { decision: "approved" });
    await expect.poll(async () => (await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${resolutionPull.id}/merge-readiness`, owner.headers)).can_enqueue, { timeout: 60_000 }).toBe(true);
    await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${resolutionPull.id}/queue`, owner.headers);
    await expect.poll(async () => (await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${resolutionPull.id}`, owner.headers)).status, { timeout: 120_000, intervals: [1000, 2000, 5000] }).toBe("merged");

    await ownerPage.goto(`/workspaces/${workspace.id}`);
    await expect(ownerPage.getByText("Both immutable histories are here")).toBeVisible();
    await expect(ownerPage.getByText(/Suggestion: combine both values but omit the package declaration/)).toBeVisible();
    await expect(ownerPage.getByText("Reject the agent omission; keep the package and combine both reviewed limits")).toBeVisible();
    await ownerPage.goto(`/pulls/${repository.id}/${resolutionPull.id}`);
    await expect(ownerPage.getByText("Merged", { exact: true })).toBeVisible();

    await git(copy, "pull", "--ff-only");
    expect(await readFile(join(copy, "service.go"), "utf8")).toContain("timeout=30 retries=3");
    const finalCommit = await git(copy, "rev-parse", "HEAD");
    expect((await git(copy, "rev-list", "--parents", "-n", "1", publication.publication.published_commit_id)).split(" ")).toHaveLength(3);
    const retained = await json(ownerPage, "get", `/workspaces/${workspace.id}`, owner.headers);
    expect(retained.conflict_context.questions[0].answer.authorship).toMatchObject({ actor_id: owner.user.id, agent_id: agent.id });
    expect(retained.conflict_context.resolutions).toEqual(expect.arrayContaining([expect.objectContaining({ id: acceptedResolution.id, state: "applied", authorship: { actor_id: owner.user.id } })]));
    expect(retained.conflict_context.checkpoints).toHaveLength(2);
    expect(finalCommit).toBe((await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${resolutionPull.id}`, owner.headers)).merge_commit_id);
    // Revoking an invited owner removes live workspace access without deleting the retained participant trail.
    await json(ownerPage, "delete", `/repositories/${repository.id}/collaborators/${behavior.user.id}`, owner.headers);
    await rejected(behaviorPage, "get", `/workspaces/${workspace.id}`, behavior.headers, undefined, 404, "workspace_not_found");
  } finally {
    const failures: string[] = [];
    await Promise.all(credentials.map(async (credential) => {
      try {
        const response = await credential.page.request.delete(`/api/auth/credentials/${credential.id}`, { headers: credential.headers });
        if (response.status() !== 204) failures.push(`credential ${credential.id}: HTTP ${response.status()} ${await response.text()}`);
      } catch (error) {
        failures.push(`credential ${credential.id}: ${error instanceof Error ? error.message : String(error)}`);
      }
    }));
    await Promise.all(copies.map(async (path) => {
      try {
        await rm(path, { recursive: true, force: true });
      } catch (error) {
        failures.push(`clone ${path}: ${error instanceof Error ? error.message : String(error)}`);
      }
    }));
    if (failures.length > 0) throw new Error(`Conflict journey cleanup failed:\n${failures.join("\n")}`);
  }
});
