import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey deliberately joins several public workflow projections */

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

test("a planned human-agent workspace survives reconnection and publishes verified work", async ({ browser }) => {
  test.setTimeout(300_000);
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copies: string[] = [];
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const peerPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, "Workspace Owner", `workspace-owner-${suffix}`);
    const peer = await account(peerPage, "Workspace Peer", `workspace-peer-${suffix}`);

    await ownerPage.goto("/organizations");
    await ownerPage.getByLabel("Name", { exact: true }).fill(`Workspace Guild ${suffix}`);
    await ownerPage.getByLabel("URL slug").fill(`workspace-guild-${suffix}`);
    await ownerPage.getByLabel("Purpose").fill("Share reproducible human-agent implementation environments.");
    await ownerPage.getByRole("button", { name: "Create organization" }).click();
    const organizations = await json(ownerPage, "get", "/organizations", owner.headers) as any;
    const organization = organizations.organizations.find((item: any) => item.slug === `workspace-guild-${suffix}`);
    await ownerPage.getByRole("link", { name: "Open portfolio →" }).click();
    await ownerPage.getByPlaceholder("collaboration ID", { exact: true }).fill(peer.user.id);
    await ownerPage.getByRole("button", { name: "Invite" }).click();
    await peerPage.goto("/organizations");
    await peerPage.getByRole("button", { name: "Accept invitation" }).click();
    await ownerPage.reload();
    await ownerPage.getByPlaceholder("new-repository").fill(`shared-room-${suffix}`);
    await ownerPage.getByRole("button", { name: "Create here" }).click();
    await ownerPage.getByPlaceholder("Agent name").fill("Pairing Agent");
    await ownerPage.getByPlaceholder("agent-slug").fill(`pairing-agent-${suffix}`);
    await ownerPage.getByPlaceholder("inspect checks, summarize failures").fill("edit files, run focused checks");
    await ownerPage.getByPlaceholder("Operator collaboration ID").fill(peer.user.id);
    await ownerPage.getByRole("button", { name: "Register approved agent" }).click();
    const portfolio = await json(ownerPage, "get", `/organizations/${organization.id}/portfolio`, owner.headers) as any;
    const repository = portfolio.repositories.find((item: any) => item.name === `shared-room-${suffix}`);
    const agent = portfolio.organization.agents.find((item: any) => item.name === "Pairing Agent");

    await ownerPage.goto("/settings");
    await ownerPage.getByLabel("Name", { exact: true }).fill("Workspace Git");
    await ownerPage.getByRole("button", { name: "Create token" }).click();
    const gitToken = await ownerPage.getByText("Copy this token now").locator("..").locator("code").textContent();
    expect(gitToken).toBeTruthy();
    const copy = await mkdtemp(join(tmpdir(), "vivarium-workspace-journey-"));
    copies.push(copy);
    await git(tmpdir(), "clone", `http://git:${gitToken}@localhost:3000/git/${repository.id}.git`, copy);
    await git(copy, "config", "user.name", "Workspace Owner");
    await git(copy, "config", "user.email", "workspace-owner@example.com");
    await mkdir(join(copy, ".vivarium"));
    await writeFile(join(copy, "feature.txt"), "planned\n");
    await writeFile(join(copy, ".vivarium", "workspace.json"), JSON.stringify({
      version: 1, image: "alpine:3.22", tools: [{ name: "sh", version: "3.22" }],
      dependencies: ["sh"], setup: ["test -f feature.txt"],
      resources: { cpus: 1, memory_mb: 256, storage_mb: 128, setup_seconds: 30 },
    }, null, 2) + "\n");
    await writeFile(join(copy, ".vivarium", "checks.json"), JSON.stringify({
      version: 1,
      checks: [{ name: "workspace verification", image: "alpine:3.22", command: "grep -qx 'peer restored' feature.txt" }],
    }, null, 2) + "\n");
    await git(copy, "add", ".");
    await git(copy, "commit", "-m", "Define the shared development room");
    await git(copy, "push", "origin", "main");
    const baseCommit = await git(copy, "rev-parse", "HEAD");

    await ownerPage.goto(`/repositories/${repository.id}`);
    await ownerPage.getByLabel("Required check names").fill("workspace verification");
    await ownerPage.getByRole("button", { name: "Save requirements" }).click();
    await peerPage.goto("/proposals");
    await peerPage.getByRole("button", { name: "New proposal" }).click();
    await peerPage.getByLabel("Title").fill("Deliver from one reproducible room");
    await peerPage.getByLabel("Context", { exact: true }).fill("Pair on the exact planned revision and retain unfinished work.");
    await peerPage.getByRole("button", { name: "Publish proposal" }).click();
    await expect(peerPage).toHaveURL(new RegExp(`/proposals/${repository.id}/[a-f0-9]{32}$`));
    const proposalID = new URL(peerPage.url()).pathname.split("/").pop()!;
    const task = await json(peerPage, "post", `/repositories/${repository.id}/proposals/${proposalID}/tasks`, peer.headers, {
      title: "Implement the shared change", outcome: "The restored checkpoint passes review and integration.",
    }) as { id: string };

    await ownerPage.goto("/workspaces/new");
    await ownerPage.getByLabel("Repository ID").fill(repository.id);
    await ownerPage.getByLabel("Exact commit").fill(baseCommit);
    await ownerPage.getByLabel("Shared context").selectOption("proposal_task");
    await ownerPage.getByLabel("proposal id").fill(proposalID);
    await ownerPage.getByLabel("task id").fill(task.id);
    await ownerPage.getByRole("button", { name: "Launch isolated workspace" }).click();
    await expect(ownerPage).toHaveURL(/\/workspaces\/[a-f0-9]{32}$/);
    const workspaceID = new URL(ownerPage.url()).pathname.split("/").pop()!;
    await expect(ownerPage.getByText("passed", { exact: true }).first()).toBeVisible();

    await peerPage.goto(ownerPage.url());
    await peerPage.getByLabel("Guide collaborators").fill("Keep the first useful version small and independently reviewable.");
    await peerPage.getByRole("button", { name: "Send", exact: true }).click();
    await peerPage.getByRole("button", { name: "Take control", exact: true }).click();
    await peerPage.getByRole("button", { name: "feature.txt" }).click();
    await peerPage.getByLabel("Edit feature.txt").fill("peer restored\n");
    const peerSave = peerPage.waitForResponse((response) => response.request().method() === "PUT" && response.url().endsWith(`/api/workspaces/${workspaceID}/file`));
    await peerPage.getByRole("button", { name: "Save change" }).click();
    expect((await peerSave).status()).toBe(200);
    await peerPage.getByLabel("Command").fill("grep -qx 'peer restored' feature.txt");
    await peerPage.getByRole("button", { name: "Run", exact: true }).click();
    await expect(peerPage.getByText("exit 0", { exact: true })).toBeVisible();
    await peerPage.getByPlaceholder("Checkpoint title").fill("Peer working state");
    await peerPage.getByPlaceholder("What remains unfinished?").fill("Ready for an agent pass and owner review.");
    await peerPage.getByRole("button", { name: "Create attributed checkpoint" }).click();
    await expect(peerPage.getByText("Peer working state", { exact: true })).toBeVisible();

    let workspace = await json(ownerPage, "get", `/workspaces/${workspaceID}`, owner.headers) as any;
    workspace = await json(ownerPage, "put", `/workspaces/${workspaceID}/control`, owner.headers, {
      expected_version: workspace.control.version, principal_kind: "approved_agent", principal_id: agent.id,
      mode: "execute", scopes: ["files", "commands"], expires_in: 900,
    });
    expect(workspace.control).toMatchObject({ principal_kind: "approved_agent", principal_id: agent.id });
    await json(ownerPage, "post", `/workspaces/${workspaceID}/messages`, owner.headers, { body: "Pause the broad refactor; preserve the peer's focused version." });
    workspace = await json(ownerPage, "put", `/workspaces/${workspaceID}/control`, owner.headers, {
      expected_version: workspace.control.version, principal_kind: "human", principal_id: owner.user.id,
      mode: "execute", scopes: ["files", "commands", "lifecycle"], expires_in: 900,
    });
    await ownerPage.reload();
    await expect(ownerPage.getByText("Pause the broad refactor; preserve the peer's focused version.")).toBeVisible();
    await ownerPage.getByRole("button", { name: "Suspend" }).click();
    await expect(ownerPage.getByText("suspended", { exact: true }).first()).toBeVisible();
    await ownerPage.getByRole("button", { name: "Resume exact foundation" }).click();
    await expect(ownerPage.getByText("running", { exact: true }).first()).toBeVisible();
    await peerPage.reload();
    await expect(peerPage.getByText("Peer working state", { exact: true })).toBeVisible();

    await ownerPage.getByRole("button", { name: "feature.txt" }).click();
    await ownerPage.getByLabel("Edit feature.txt").fill("discard this experiment\n");
    const ownerSave = ownerPage.waitForResponse((response) => response.request().method() === "PUT" && response.url().endsWith(`/api/workspaces/${workspaceID}/file`));
    await ownerPage.getByRole("button", { name: "Save change" }).click();
    expect((await ownerSave).status()).toBe(200);
    await ownerPage.getByPlaceholder("Checkpoint title").fill("Owner experiment");
    await ownerPage.getByRole("button", { name: "Create attributed checkpoint" }).click();
    const peerCheckpoint = ownerPage.locator("li").filter({ hasText: "Peer working state" });
    await peerCheckpoint.getByRole("button", { name: "Inspect restore" }).click();
    await ownerPage.getByLabel("Replace conflicting live paths").check();
    const restoredResponse = ownerPage.waitForResponse((response) => response.request().method() === "POST" && response.url().includes(`/api/workspaces/${workspaceID}/checkpoints/`) && response.url().endsWith("/restore"));
    await ownerPage.getByRole("button", { name: "Restore and branch from checkpoint" }).click();
    const restoredHTTP = await restoredResponse;
    expect(restoredHTTP.status(), await restoredHTTP.text()).toBe(200);
    await ownerPage.reload();
    await ownerPage.getByRole("button", { name: "feature.txt" }).click();
    await expect(ownerPage.getByLabel("Edit feature.txt")).toHaveValue("peer restored\n");
    await peerCheckpoint.getByPlaceholder("workspace/change-name").fill("workspace/shared-change");
    await peerCheckpoint.getByRole("button", { name: "Commit checkpoint and publish" }).click();
    await peerCheckpoint.getByRole("link", { name: "Open governed review" }).click();
    await expect(ownerPage.getByText("Created in a collaborative workspace")).toBeVisible();
    const pullURL = ownerPage.url();
    await expect(ownerPage.locator("#checks").getByText("succeeded", { exact: true })).toBeVisible({ timeout: 60_000 });
    await peerPage.goto(pullURL);
    await peerPage.getByRole("button", { name: "Approve" }).click();
    await ownerPage.reload();
    await expect(ownerPage.getByRole("button", { name: "Merge into main" })).toBeEnabled();
    await ownerPage.getByRole("button", { name: "Merge into main" }).click();
    await expect(ownerPage.getByText("Merged", { exact: true })).toBeVisible();

    await git(copy, "pull", "--ff-only");
    expect(await readFile(join(copy, "feature.txt"), "utf8")).toBe("peer restored\n");
    await json(ownerPage, "post", `/workspaces/${workspaceID}/expiry`, owner.headers, {
      expires_at: new Date(Date.now() + 1_000).toISOString(), reason: "Merged work no longer needs live compute.",
    });
    await new Promise((resolve) => setTimeout(resolve, 1_100));
    const expired = await json(ownerPage, "post", `/workspaces/${workspaceID}/reconcile`, owner.headers, {});
    expect(expired.state).toBe("expired");
    const retained = await json(peerPage, "get", `/workspaces/${workspaceID}`, peer.headers) as any;
    expect(retained.state).toBe("expired");
    expect(retained.events).toEqual(expect.arrayContaining([
      expect.objectContaining({ kind: "control.changed" }),
      expect.objectContaining({ kind: "checkpoint.restored" }),
      expect.objectContaining({ kind: "expired" }),
    ]));
    expect(retained.head_checkpoint_id).toBeTruthy();
  } finally {
    await Promise.all(copies.map((copy) => rm(copy, { recursive: true, force: true })));
  }
});
