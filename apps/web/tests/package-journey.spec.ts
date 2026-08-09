import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- the journey keeps only cross-workflow fields from public responses */

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

test("independent projects publish, adopt, and recover one package through reviewed agent work", async ({ browser }) => {
  test.setTimeout(300_000);
  const copies: string[] = [];
  const temporaryCopy = async (prefix: string) => { const path = await mkdtemp(join(tmpdir(), prefix)); copies.push(path); return path; };
  try {
    await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
    const suffix = Date.now().toString(36);
    const publisherPage = await (await browser.newContext()).newPage();
    const consumerPage = await (await browser.newContext()).newPage();
    const reviewerPage = await (await browser.newContext()).newPage();
    const publisherActor = await account(publisherPage, "Package Publisher", `pkg-publisher-${suffix}`);
    const consumerActor = await account(consumerPage, "Package Consumer", `pkg-consumer-${suffix}`);
    const reviewerActor = await account(reviewerPage, "Package Reviewer", `pkg-reviewer-${suffix}`);

    const publisher = await json(publisherPage, "post", "/repositories", publisherActor.headers, { name: `shared-kit-${suffix}` }) as { id: string };
    const consumer = await json(consumerPage, "post", "/repositories", consumerActor.headers, { name: `independent-app-${suffix}` }) as { id: string };
    await json(publisherPage, "patch", `/repositories/${publisher.id}`, publisherActor.headers, { visibility: "public" });
    await json(consumerPage, "patch", `/repositories/${consumer.id}`, consumerActor.headers, { visibility: "public" });
    await json(consumerPage, "post", `/repositories/${consumer.id}/collaborators`, consumerActor.headers, { user_id: reviewerActor.user.id });

    const credential = async (page: Page, headers: Record<string, string>, name: string) => json(page, "post", "/auth/credentials", headers, { kind: "git", name, scopes: ["git:read", "git:write"], expires_in: 3600 }) as Promise<{ token: string }>;
    const publisherGit = await credential(publisherPage, publisherActor.headers, "publisher git");
    const consumerGit = await credential(consumerPage, consumerActor.headers, "consumer git");
    const publisherCopy = await temporaryCopy("vivarium-package-publisher-");
    const consumerCopy = await temporaryCopy("vivarium-package-consumer-");
    await git(tmpdir(), "clone", `http://git:${publisherGit.token}@localhost:3000/git/${publisher.id}.git`, publisherCopy);
    await git(tmpdir(), "clone", `http://git:${consumerGit.token}@localhost:3000/git/${consumer.id}.git`, consumerCopy);
    for (const [copy, name] of [[publisherCopy, "Package Publisher"], [consumerCopy, "Package Consumer"]]) {
      await git(copy, "config", "user.name", name); await git(copy, "config", "user.email", `${name.replaceAll(" ", "-").toLowerCase()}@example.com`);
      await mkdir(join(copy, ".vivarium"));
      await writeFile(join(copy, ".vivarium", "release.json"), JSON.stringify({ version: 1, steps: [{ name: "package", image: "alpine:3.22", command: "cp artifact.txt \"$VIVARIUM_OUTPUT/shared-kit.txt\"" }] }));
    }
    await writeFile(join(publisherCopy, "artifact.txt"), "shared-kit 1.0.0 safe\n");
    await git(publisherCopy, "add", "."); await git(publisherCopy, "commit", "-m", "Publish shared kit 1.0.0"); await git(publisherCopy, "push", "origin", "main");

    const packageName = `shared-kit-${suffix}`;
    async function publish(version: string, content: string) {
      if (version !== "1.0.0") {
        await writeFile(join(publisherCopy, "artifact.txt"), content);
        await git(publisherCopy, "add", "artifact.txt"); await git(publisherCopy, "commit", "-m", `Publish shared kit ${version}`); await git(publisherCopy, "push", "origin", "main");
      }
      const commit = await git(publisherCopy, "rev-parse", "HEAD");
      const release = await json(publisherPage, "post", `/repositories/${publisher.id}/releases`, publisherActor.headers, { version: `v${version}`, notes: `${packageName} ${version} verified release`, commit_id: commit }) as any;
      await json(publisherPage, "post", `/repositories/${publisher.id}/releases/${release.id}/builds`, publisherActor.headers, {});
      const builds = await eventually(() => json(publisherPage, "get", `/repositories/${publisher.id}/releases/${release.id}/builds`, publisherActor.headers) as Promise<any>, (value) => value.builds?.every((build: any) => build.state === "succeeded"), `${version} package build succeeds`);
      const build = builds.builds[0];
      return json(publisherPage, "post", `/repositories/${publisher.id}/releases/${release.id}/packages`, publisherActor.headers, { name: packageName, version, build_id: build.id, artifact_id: build.artifacts[0].id, platform: { os: "linux", architecture: "amd64", runtime: "text/v1" }, summary: "Inspectable shared package", documentation: `Install ${version} with a repository-scoped package credential.`, license: "MIT", support: "publisher@example.test", visibility: "public", dependencies: [] }) as Promise<any>;
    }
    const first = await publish("1.0.0", "shared-kit 1.0.0 safe\n");
    expect(first.sha256).toMatch(/^[a-f0-9]{64}$/);

    await consumerPage.goto("/packages");
    await consumerPage.getByPlaceholder("Name, purpose, or documentation").fill(packageName);
    await expect(consumerPage.getByRole("heading", { name: new RegExp(packageName) })).toBeVisible();
    await consumerPage.getByText("Create isolated install credential").click();
    await consumerPage.getByLabel("Consuming repository ID").fill(consumer.id);
    await consumerPage.getByRole("button", { name: "Create one-hour token" }).click();
    const installToken = await consumerPage.locator("code").filter({ hasText: /vvr_/ }).textContent();
    expect(installToken).toBeTruthy();
    const installed = await run("curl", ["--fail", "--silent", "--show-error", "-H", `Authorization: Bearer ${installToken}`, `http://localhost:3000/api/packages/${packageName}/versions/1.0.0/artifact`]);
    expect(installed.stdout).toBe("shared-kit 1.0.0 safe\n");

    await writeFile(join(consumerCopy, "artifact.txt"), "consumer accepts shared-kit\n");
    await writeFile(join(consumerCopy, ".vivarium", "deployment.json"), JSON.stringify({ version: 1, stages: [{ name: "package health", signals: [{ name: "dependency is present", command: "grep -q shared-kit \"$VIVARIUM_ARTIFACT\"" }] }] }));
    const manifest = (version: string) => ({ version: 1, dependencies: [{ name: packageName, constraint: `^${version}` }], lock: [{ name: packageName, version }] });
    await writeFile(join(consumerCopy, ".vivarium", "packages.json"), JSON.stringify(manifest("1.0.0")));
    await git(consumerCopy, "add", "."); await git(consumerCopy, "commit", "-m", "Install shared kit with scoped client"); await git(consumerCopy, "push", "origin", "main");
    let consumerCommit = await git(consumerCopy, "rev-parse", "HEAD");
    await json(consumerPage, "post", `/repositories/${consumer.id}/dependency-inventories`, consumerActor.headers, { commit_id: consumerCommit });

    await publish("1.1.0", "shared-kit 1.1.0 later quarantined\n");
    await json(consumerPage, "put", `/repositories/${consumer.id}/package-update-policies/${packageName}`, consumerActor.headers, { strategy: "minor", action: "proposal" });
    const scan = await json(consumerPage, "post", `/repositories/${consumer.id}/package-updates/scan`, consumerActor.headers, {}) as any;
    expect(scan.updates).toHaveLength(1);

    async function agentUpdate(update: any, expectedBase: string, summary: string, preassigned = false) {
      const base = `/repositories/${consumer.id}/proposals/${update.proposal_id}/tasks/${update.task_id}`;
      const assigned = preassigned
        ? await json(consumerPage, "get", base, consumerActor.headers) as any
        : await json(consumerPage, "put", `${base}/assignment`, consumerActor.headers, { assignee_type: "agent", mandate: summary, repository_id: consumer.id, base_revision: expectedBase }) as any;
      const launched = await json(consumerPage, "post", `${base}/sessions`, consumerActor.headers, { expected_assignment_id: assigned.assignment.id, context_paths: [".vivarium/packages.json"], expires_in: 3600 }) as any;
      const agentCopy = await temporaryCopy("vivarium-package-agent-");
      await git(tmpdir(), "clone", `http://git:${launched.credential.token}@localhost:3000/git/${consumer.id}.git`, agentCopy);
      await git(agentCopy, "config", "user.name", "Vivarium Package Agent"); await git(agentCopy, "config", "user.email", "agent@users.vivarium");
      await git(agentCopy, "switch", "-c", `work-${update.to_version}`, `origin/${launched.run.working_branch}`);
      await writeFile(join(agentCopy, ".vivarium", "packages.json"), JSON.stringify(update.manifest));
      await git(agentCopy, "add", ".vivarium/packages.json"); await git(agentCopy, "commit", "-m", summary); await git(agentCopy, "push", "origin", `HEAD:refs/heads/${launched.run.working_branch}`);
      const commit = await git(agentCopy, "rev-parse", "HEAD");
      const runPath = `${base}/sessions/${launched.session.id}/runs/${launched.run.id}`;
      await json(consumerPage, "post", `${runPath}/completion`, { Authorization: `Bearer ${launched.credential.token}` }, { summary, commit_id: commit, checks: [{ name: "manifest matches proposed lock", status: "passed" }], unresolved_concerns: [] });
      const pull = await json(consumerPage, "post", `${base}/contributions`, consumerActor.headers, { title: summary, body: "Agent-authored exact lock update for independent review.", source_branch: launched.run.working_branch, target_branch: "main", session_id: launched.session.id, run_id: launched.run.id }) as any;
      await json(reviewerPage, "post", `/repositories/${consumer.id}/pulls/${pull.id}/reviews`, reviewerActor.headers, { decision: "approved" });
      const merged = await json(consumerPage, "post", `/repositories/${consumer.id}/pulls/${pull.id}/merge`, consumerActor.headers, {}) as any;
      return merged.merge_commit_id as string;
    }

    consumerCommit = await agentUpdate(scan.updates[0], consumerCommit, "Agent adopts shared kit 1.1.0");
    await json(consumerPage, "post", `/repositories/${consumer.id}/dependency-inventories`, consumerActor.headers, { commit_id: consumerCommit });
    const environment = await json(consumerPage, "post", `/repositories/${consumer.id}/environments`, consumerActor.headers, { name: "production", position: 1, image: "alpine:3.22", command: "grep -q shared-kit \"$VIVARIUM_ARTIFACT\"", timeout_seconds: 30, configuration: {}, credentials: {}, required_approvals: 0, concurrency: 1 }) as any;

    async function deliver(version: string, commit: string) {
      const release = await json(consumerPage, "post", `/repositories/${consumer.id}/releases`, consumerActor.headers, { version, notes: `Consumer delivery using ${packageName}`, commit_id: commit }) as any;
      await json(consumerPage, "post", `/repositories/${consumer.id}/releases/${release.id}/builds`, consumerActor.headers, {});
      const builds = await eventually(() => json(consumerPage, "get", `/repositories/${consumer.id}/releases/${release.id}/builds`, consumerActor.headers) as Promise<any>, (value) => value.builds?.every((build: any) => build.state === "succeeded"), `${version} consumer build succeeds`);
      const build = builds.builds[0];
      const deployment = await json(consumerPage, "post", `/repositories/${consumer.id}/deployments`, consumerActor.headers, { environment_id: environment.id, release_id: release.id, build_id: build.id, artifact_id: build.artifacts[0].id }) as any;
      return eventually(() => json(consumerPage, "get", `/repositories/${consumer.id}/deployments`, consumerActor.headers) as Promise<any>, (value) => value.deployments?.some((item: any) => item.id === deployment.id && item.state === "succeeded"), `${version} consumer deploys`);
    }
    await deliver("v1.1.0", consumerCommit);

    await publish("1.2.0", "shared-kit 1.2.0 safe replacement\n");
    await publisherPage.goto("/packages");
    await publisherPage.getByPlaceholder("Name, purpose, or documentation").fill(packageName);
    const unsafeCard = publisherPage.locator("div.rounded-xl").filter({ hasText: "1.1.0" }).first();
    await unsafeCard.getByText("Publisher lifecycle control").click();
    await unsafeCard.getByLabel("Decision").selectOption("quarantined");
    await unsafeCard.getByLabel("Public warning").fill("Do not install: unsafe parser behavior.");
    await unsafeCard.getByLabel("Reason").fill("Production evidence showed unsafe parsing under malformed input.");
    await unsafeCard.getByLabel("Replacement package").fill(packageName);
    await unsafeCard.getByLabel("Replacement version").fill("1.2.0");
    await unsafeCard.getByRole("button", { name: "Record decision" }).click();
    await expect(unsafeCard.getByText("Quarantined:")).toBeVisible();

    const blocked = await consumerPage.request.get(`/api/packages/${packageName}/versions/1.1.0/artifact`, { headers: { Authorization: `Bearer ${installToken}` } });
    expect(blocked.status()).toBe(409);
    const recovery = await json(consumerPage, "post", `/repositories/${consumer.id}/package-recoveries`, consumerActor.headers, { package_name: packageName, version: "1.1.0", assignee_type: "agent" }) as any;
    consumerCommit = await agentUpdate(recovery, consumerCommit, "Agent replaces quarantined shared kit with 1.2.0", true);
    await json(consumerPage, "post", `/repositories/${consumer.id}/dependency-inventories`, consumerActor.headers, { commit_id: consumerCommit });
    await deliver("v1.2.0", consumerCommit);

    await consumerPage.goto(`/repositories/${consumer.id}/dependencies`);
    await expect(consumerPage.getByText(`${packageName} 1.2.0`)).toBeVisible();
    await expect(consumerPage.getByText("current source", { exact: true })).toBeVisible();
    await publisherPage.goto("/packages");
    await publisherPage.getByPlaceholder("Name, purpose, or documentation").fill(packageName);
    const finalUnsafeCard = publisherPage.locator("div.rounded-xl").filter({ hasText: "1.1.0" }).first();
    await finalUnsafeCard.getByRole("button", { name: "Show exposure and remediation" }).click();
    await expect(finalUnsafeCard.getByText(/repair completed toward 1.2.0/)).toBeVisible();
    await expect(finalUnsafeCard.getByText(/retained historical deployments/)).toBeVisible();
  } finally {
    await Promise.all(copies.map((path) => rm(path, { recursive: true, force: true })));
  }
});
