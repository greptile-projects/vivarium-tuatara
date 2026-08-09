import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- the journey intentionally selects cross-workflow public response fields */

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

async function json(page: Page, method: "get" | "post" | "put" | "patch" | "delete", path: string, headers: Record<string, string>, data?: unknown) {
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

test("an organization governs a human-agent portfolio without losing published work", async ({ browser }) => {
  test.setTimeout(300_000);
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copies: string[] = [];
  const temporaryCopy = async (prefix: string) => { const path = await mkdtemp(join(tmpdir(), prefix)); copies.push(path); return path; };
  try {
    const suffix = Date.now().toString(36);
    const packageName = `organization-runtime-${suffix}`;
    const ownerPage = await (await browser.newContext()).newPage();
    const developerPage = await (await browser.newContext()).newPage();
    const approverPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, "Organization Owner", `org-owner-${suffix}`);
    const developer = await account(developerPage, "Portfolio Developer", `org-developer-${suffix}`);
    const approver = await account(approverPage, "Delivery Approver", `org-approver-${suffix}`);

    await ownerPage.goto("/organizations");
    await ownerPage.getByLabel("Name", { exact: true }).fill(`Acme Systems ${suffix}`);
    await ownerPage.getByLabel("URL slug").fill(`acme-${suffix}`);
    await ownerPage.getByLabel("Purpose").fill("Coordinate human and agent delivery across one accountable portfolio.");
    await ownerPage.getByRole("button", { name: "Create organization" }).click();
    await expect(ownerPage.getByText(`Acme Systems ${suffix}`, { exact: true })).toBeVisible();
    const organizations = await json(ownerPage, "get", "/organizations", owner.headers) as any;
    const organization = organizations.organizations.find((item: any) => item.slug === `acme-${suffix}`);

    await ownerPage.getByRole("link", { name: "Open portfolio →" }).click();
    await ownerPage.getByPlaceholder("collaboration ID", { exact: true }).fill(developer.user.id);
    await ownerPage.getByRole("button", { name: "Invite" }).click();
    await developerPage.goto("/organizations");
    await developerPage.getByRole("button", { name: "Accept invitation" }).click();
    await expect(developerPage.getByRole("link", { name: "Open portfolio →" })).toBeVisible();
    await json(ownerPage, "post", `/organizations/${organization.id}/invitations`, owner.headers, { user_id: approver.user.id });
    const approverOrganizations = await json(approverPage, "get", "/organizations", approver.headers) as any;
    const approverInvitation = approverOrganizations.organizations.find((item: any) => item.id === organization.id).invitations[0];
    await json(approverPage, "post", `/organizations/${organization.id}/invitations/${approverInvitation.id}/accept`, approver.headers);

    await ownerPage.reload();
    for (const name of [`service-${suffix}`, `console-${suffix}`]) {
      await ownerPage.getByPlaceholder("new-repository").fill(name);
      await ownerPage.getByRole("button", { name: "Create here" }).click();
      await expect(ownerPage.getByRole("link", { name })).toBeVisible();
    }
    let portfolio = await json(ownerPage, "get", `/organizations/${organization.id}/portfolio`, owner.headers) as any;
    const service = portfolio.repositories.find((item: any) => item.name === `service-${suffix}`);
    const consoleRepository = portfolio.repositories.find((item: any) => item.name === `console-${suffix}`);

    for (const [name, slug] of [["Platform", `platform-${suffix}`], ["Delivery", `delivery-${suffix}`]]) {
      await ownerPage.getByPlaceholder("Team name").fill(name);
      await ownerPage.getByPlaceholder("team-slug").fill(slug);
      await ownerPage.getByRole("button", { name: "Create team" }).click();
      await expect(ownerPage.getByRole("heading", { name })).toBeVisible();
    }
    portfolio = await json(ownerPage, "get", `/organizations/${organization.id}/portfolio`, owner.headers) as any;
    const platform = portfolio.organization.teams.find((item: any) => item.name === "Platform");
    const delivery = portfolio.organization.teams.find((item: any) => item.name === "Delivery");
    await json(ownerPage, "put", `/organizations/${organization.id}/teams/${platform.id}/members`, owner.headers, { user_id: developer.user.id, role: "maintainer", expected_version: platform.version });
    await json(ownerPage, "put", `/organizations/${organization.id}/teams/${delivery.id}/members`, owner.headers, { user_id: developer.user.id, role: "member", expected_version: delivery.version });
    const refreshed = await json(ownerPage, "get", `/organizations/${organization.id}/portfolio`, owner.headers) as any;
    const platformVersion = refreshed.organization.teams.find((item: any) => item.id === platform.id).version;
    const deliveryVersion = refreshed.organization.teams.find((item: any) => item.id === delivery.id).version;
    await json(ownerPage, "post", `/organizations/${organization.id}/teams/${platform.id}/responsibilities`, owner.headers, { repository_id: service.id, area: "runtime", description: "Own service changes and operations.", expected_version: platformVersion });
    await json(ownerPage, "post", `/organizations/${organization.id}/teams/${delivery.id}/responsibilities`, owner.headers, { repository_id: consoleRepository.id, area: "release", description: "Own console delivery.", expected_version: deliveryVersion });

    await ownerPage.reload();
    await ownerPage.getByPlaceholder("Agent name").fill("Release Agent");
    await ownerPage.getByPlaceholder("agent-slug").fill(`release-agent-${suffix}`);
    await ownerPage.getByPlaceholder("inspect checks, summarize failures").fill("edit release manifests, publish bounded branches");
    await ownerPage.getByPlaceholder("Operator collaboration ID").fill(developer.user.id);
    await ownerPage.getByRole("button", { name: "Register approved agent" }).click();
    await expect(ownerPage.getByRole("heading", { name: "Release Agent" })).toBeVisible();
    portfolio = await json(ownerPage, "get", `/organizations/${organization.id}/portfolio`, owner.headers) as any;
    const agent = portfolio.organization.agents.find((item: any) => item.name === "Release Agent");

    await ownerPage.getByText("Draft a versioned policy").click();
    await ownerPage.getByPlaceholder("Policy name").fill("Shared delivery baseline");
    await ownerPage.getByLabel("Minimum reviews").fill("1");
    await ownerPage.getByPlaceholder("Required checks, comma separated").fill("portfolio verification");
    await ownerPage.locator('select[name="integration"]').selectOption("direct");
    await ownerPage.locator('select[name="release_provenance"]').selectOption("attested");
    await ownerPage.getByLabel("Promotion approvals").fill("1");
    await ownerPage.locator('select[name="agent_authority"]').selectOption("explicit-grants");
    await ownerPage.getByRole("button", { name: "Save draft" }).click();
    await expect(ownerPage.getByText("Shared delivery baseline", { exact: true })).toBeVisible();
    await ownerPage.getByRole("button", { name: "Activate baseline" }).click();
    await ownerPage.getByLabel("Preview repository policy").selectOption(service.id);
    await expect(ownerPage.getByText(/minimum_reviews: 1/)).toBeVisible();
    await expect(ownerPage.getByText(/release_provenance: attested/)).toBeVisible();

    portfolio = await json(ownerPage, "get", `/organizations/${organization.id}/portfolio`, owner.headers) as any;
    const policy = portfolio.organization.policies.find((item: any) => item.name === "Shared delivery baseline");
    const expiry = new Date(Date.now() + 3_600_000).toISOString();
    const policyException = await json(developerPage, "post", `/organizations/${organization.id}/policy-exceptions`, developer.headers, { policy_id: policy.id, repository_id: service.id, rule: "promotion_approvals", requested_value: "0", reason: "Temporary preview environment has no second approver.", expires_at: expiry }) as any;
    await json(ownerPage, "post", `/organizations/${organization.id}/policy-exceptions/${policyException.id}/decision`, owner.headers, { decision: "approve" });

    const request = await json(developerPage, "post", `/organizations/${organization.id}/access-requests`, developer.headers, { principal_type: "agent", principal_id: agent.id, role: "contributor", resources: [{ kind: "repository", id: consoleRepository.id }], exceptions: [], reason: "Publish the console half of the initiative.", expires_at: expiry }) as any;
    const accessRequest = request.access_requests.at(-1);
    const approved = await json(ownerPage, "post", `/organizations/${organization.id}/access-requests/${accessRequest.id}/decision`, owner.headers, { decision: "approve" }) as any;
    const grant = approved.access_grants.at(-1);
    const issued = await json(developerPage, "post", `/organizations/${organization.id}/access-grants/${grant.id}/credentials`, developer.headers, { agent_id: agent.id, repository_id: consoleRepository.id, expires_in: 3600 }) as any;

    const ownerGit = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "organization baseline", scopes: ["git:read", "git:write"], expires_in: 3600 }) as any;
    const developerGit = await json(developerPage, "post", "/auth/credentials", developer.headers, { kind: "git", name: "portfolio work", scopes: ["git:read", "git:write"], expires_in: 3600 }) as any;
    const ownerCopies = new Map<string, string>();
    for (const repository of [service, consoleRepository]) {
      const copy = await temporaryCopy(`vivarium-org-owner-${repository.name}-`); ownerCopies.set(repository.id, copy);
      await git(tmpdir(), "clone", `http://git:${ownerGit.token}@localhost:3000/git/${repository.id}.git`, copy);
      await git(copy, "config", "user.name", "Organization Owner"); await git(copy, "config", "user.email", "owner@example.test");
      await mkdir(join(copy, ".vivarium"));
      await writeFile(join(copy, ".vivarium", "release.json"), JSON.stringify({ version: 1, steps: [{ name: "package", image: "alpine:3.22", command: "cp artifact.txt \"$VIVARIUM_OUTPUT/app.txt\"" }] }));
      if (repository.id === consoleRepository.id) {
        await writeFile(join(copy, ".vivarium", "checks.json"), JSON.stringify({ version: 1, checks: [{ name: "portfolio verification", image: "alpine:3.22", command: "sleep 2; test -r console.txt" }] }));
        await writeFile(join(copy, ".vivarium", "packages.json"), JSON.stringify({ version: 1, dependencies: [{ name: packageName, constraint: "^1.0.0" }], lock: [{ name: packageName, version: "1.0.0" }] }));
        await writeFile(join(copy, ".vivarium", "deployment.json"), JSON.stringify({ version: 1, stages: [{ name: "portfolio health", signals: [{ name: "console artifact is readable", command: "test -r \"$VIVARIUM_ARTIFACT\"" }] }] }));
      }
      await writeFile(join(copy, "artifact.txt"), `${repository.name} baseline\n`);
      await git(copy, "add", "."); await git(copy, "commit", "-m", "Establish governed baseline"); await git(copy, "push", "origin", "main");
    }
    await json(ownerPage, "put", `/repositories/${consoleRepository.id}/branches/main/required-checks`, owner.headers, { checks: ["portfolio verification"] });

    const proposal = await json(developerPage, "post", `/repositories/${service.id}/proposals`, developer.headers, { title: "Ship coordinated portfolio update", body: "Human service work precedes the bounded agent console change." }) as any;
    const serviceWorkID = crypto.randomUUID().replaceAll("-", "");
    const consoleWorkID = crypto.randomUUID().replaceAll("-", "");
    await json(developerPage, "post", `/organizations/${organization.id}/initiatives`, developer.headers, { title: "Ship portfolio update", description: "Coordinate both repository changes through shared governance.", source: { kind: "proposal", repository_id: service.id, id: proposal.id }, work_items: [{ id: serviceWorkID, title: "Human service change", repository_id: service.id, owner: { type: "human", id: developer.user.id }, dependency_ids: [], status: "in_progress" }, { id: consoleWorkID, title: "Agent console change", repository_id: consoleRepository.id, owner: { type: "agent", id: agent.id }, dependency_ids: [serviceWorkID], status: "todo" }] });

    const humanCopy = await temporaryCopy("vivarium-org-human-");
    await git(tmpdir(), "clone", `http://git:${developerGit.token}@localhost:3000/git/${service.id}.git`, humanCopy);
    await git(humanCopy, "config", "user.name", "Portfolio Developer"); await git(humanCopy, "config", "user.email", "developer@example.test"); await git(humanCopy, "switch", "-c", "human-service");
    await writeFile(join(humanCopy, "service.txt"), "human-authored portfolio change\n"); await git(humanCopy, "add", "service.txt"); await git(humanCopy, "commit", "-m", "Implement service contribution"); await git(humanCopy, "push", "origin", "human-service");
    const humanPull = await json(developerPage, "post", `/repositories/${service.id}/pulls`, developer.headers, { title: "Implement service contribution", body: "Human-authored half of the organization initiative.", source_branch: "human-service", target_branch: "main", proposal_id: proposal.id }) as any;

    const agentCopy = await temporaryCopy("vivarium-org-agent-");
    await git(tmpdir(), "clone", `http://git:${issued.token}@localhost:3000/git/${consoleRepository.id}.git`, agentCopy);
    await git(agentCopy, "config", "user.name", "Acme Release Agent"); await git(agentCopy, "config", "user.email", "release-agent@agents.vivarium"); await git(agentCopy, "switch", "-c", "agent-console");
    await writeFile(join(agentCopy, "console.txt"), "approved-agent portfolio change\n"); await git(agentCopy, "add", "console.txt"); await git(agentCopy, "commit", "-m", "Implement console contribution"); await git(agentCopy, "push", "origin", "agent-console");
    const agentPull = await json(developerPage, "post", `/repositories/${consoleRepository.id}/pulls`, developer.headers, { title: "Implement console contribution", body: `Agent-authored under approved identity ${agent.id} and grant ${grant.id}.`, source_branch: "agent-console", target_branch: "main" }) as any;

    await json(ownerPage, "delete", `/organizations/${organization.id}/members/${developer.user.id}`, owner.headers);
    const deniedPush = await git(agentCopy, "push", "origin", "agent-console").then(() => "allowed", (error: any) => String(error.stderr));
    expect(deniedPush).toContain("Authentication failed");
    const retainedHuman = await json(ownerPage, "get", `/repositories/${service.id}/pulls/${humanPull.id}`, owner.headers) as any;
    const retainedAgent = await json(ownerPage, "get", `/repositories/${consoleRepository.id}/pulls/${agentPull.id}`, owner.headers) as any;
    expect(retainedHuman.author_id).toBe(developer.user.id);
    expect(retainedAgent.body).toContain(agent.id);

    async function approveAndMerge(repositoryID: string, pullID: string, verifyPolicy = false) {
      if (verifyPolicy) {
        const blocked = await ownerPage.request.post(`/api/repositories/${repositoryID}/pulls/${pullID}/merge`, { headers: owner.headers, data: {} });
        expect(blocked.status()).toBe(409);
        const readiness = await json(ownerPage, "get", `/repositories/${repositoryID}/pulls/${pullID}/merge-readiness`, owner.headers) as any;
        expect(readiness.blockers).toEqual(expect.arrayContaining([
          expect.objectContaining({ code: "approval_required" }),
          expect.objectContaining({ code: "required_check_pending", message: expect.stringContaining("portfolio verification") }),
        ]));
        await eventually(() => json(ownerPage, "get", `/repositories/${repositoryID}/pulls/${pullID}/checks`, owner.headers) as Promise<any>, (value) => value.check_runs?.some((check: any) => check.definition.name === "portfolio verification" && check.state === "succeeded"), "portfolio verification passes");
      }
      await json(ownerPage, "post", `/repositories/${repositoryID}/pulls/${pullID}/reviews`, owner.headers, { decision: "approved" });
      return json(ownerPage, "post", `/repositories/${repositoryID}/pulls/${pullID}/merge`, owner.headers, {}) as Promise<any>;
    }
    const humanMerge = await approveAndMerge(service.id, humanPull.id);
    const agentMerge = await approveAndMerge(consoleRepository.id, agentPull.id, true);
    const serviceRelease = await json(ownerPage, "post", `/repositories/${service.id}/releases`, owner.headers, { version: "v1.0.0", notes: "Runtime dependency for the coordinated console delivery.", commit_id: humanMerge.merge_commit_id }) as any;
    await json(ownerPage, "post", `/repositories/${service.id}/releases/${serviceRelease.id}/builds`, owner.headers, {});
    const serviceBuilds = await eventually(() => json(ownerPage, "get", `/repositories/${service.id}/releases/${serviceRelease.id}/builds`, owner.headers) as Promise<any>, (value) => value.builds?.some((build: any) => build.state === "succeeded"), "organization runtime builds");
    const serviceBuild = serviceBuilds.builds.find((item: any) => item.state === "succeeded");
    await json(ownerPage, "post", `/repositories/${service.id}/releases/${serviceRelease.id}/packages`, owner.headers, { name: packageName, version: "1.0.0", build_id: serviceBuild.id, artifact_id: serviceBuild.artifacts[0].id, platform: { os: "linux", architecture: "amd64" }, summary: "Organization runtime", documentation: "Internal coordinated runtime.", license: "MIT", support: "platform@example.test", visibility: "private", dependencies: [] });
    await json(ownerPage, "post", `/repositories/${consoleRepository.id}/dependency-inventories`, owner.headers, { commit_id: agentMerge.merge_commit_id });
    const initiative = (await json(ownerPage, "get", `/organizations/${organization.id}/portfolio`, owner.headers) as any).initiatives[0];
    expect(initiative.work_items[0]).toMatchObject({ ownership_state: "reassignment_required" });
    expect(initiative.policy_exceptions).toEqual(expect.arrayContaining([expect.objectContaining({ id: policyException.id, status: "approved" })]));

    const release = await json(ownerPage, "post", `/repositories/${consoleRepository.id}/releases`, owner.headers, { version: "v1.0.0", notes: "Governed human-agent portfolio delivery.", commit_id: agentMerge.merge_commit_id }) as any;
    const environment = await json(ownerPage, "post", `/repositories/${consoleRepository.id}/environments`, owner.headers, { name: "production", position: 1, image: "alpine:3.22", command: "test -r \"$VIVARIUM_ARTIFACT\"", timeout_seconds: 30, configuration: {}, credentials: {}, required_approvals: 1, concurrency: 1 }) as any;
    const unattested = await ownerPage.request.post(`/api/repositories/${consoleRepository.id}/deployments`, { headers: owner.headers, data: { environment_id: environment.id, release_id: release.id, build_id: "0".repeat(32), artifact_id: "1".repeat(32) } });
    expect(unattested.status()).toBe(422);
    expect(await unattested.json()).toMatchObject({ error: { code: "unverified_build" } });
    await json(ownerPage, "post", `/repositories/${consoleRepository.id}/releases/${release.id}/builds`, owner.headers, {});
    const builds = await eventually(() => json(ownerPage, "get", `/repositories/${consoleRepository.id}/releases/${release.id}/builds`, owner.headers) as Promise<any>, (value) => value.builds?.some((build: any) => build.state === "succeeded"), "organization release builds");
    const build = builds.builds.find((item: any) => item.state === "succeeded");
    const deployment = await json(ownerPage, "post", `/repositories/${consoleRepository.id}/deployments`, owner.headers, { environment_id: environment.id, release_id: release.id, build_id: build.id, artifact_id: build.artifacts[0].id }) as any;
    expect(deployment.state).toBe("pending_approval");
    const selfApproval = await ownerPage.request.post(`/api/repositories/${consoleRepository.id}/deployments/${deployment.id}/approvals`, { headers: owner.headers, data: {} });
    expect(selfApproval.status()).toBe(409);
    await json(approverPage, "post", `/repositories/${consoleRepository.id}/deployments/${deployment.id}/approvals`, approver.headers, {});
    await eventually(() => json(ownerPage, "get", `/repositories/${consoleRepository.id}/deployments`, owner.headers) as Promise<any>, (value) => value.deployments?.some((item: any) => item.id === deployment.id && item.state === "succeeded"), "organization change deploys");

    await ownerPage.goto(`/organizations/${organization.id}`);
    await expect(ownerPage.getByText("Ship portfolio update", { exact: true })).toBeVisible();
    await expect(ownerPage.getByText("reassign", { exact: true }).first()).toBeVisible();
    await expect(ownerPage.getByText("Decision needed: 1 active or pending policy exception(s).")).toBeVisible();
    await expect(ownerPage.getByRole("link", { name: /v1\.0\.0 · Governed human-agent/ })).toBeVisible();
    expect(humanMerge.merge_commit_id).toMatch(/^[a-f0-9]{40}$/);
  } finally {
    await Promise.all(copies.map((path) => rm(path, { recursive: true, force: true })));
  }
});
