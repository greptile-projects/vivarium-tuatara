import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this proof crosses the public support and governed-work contracts */
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
  const headers = { Authorization: `Bearer ${token}` };
  return { headers, user: await (await page.request.get("/api/user", { headers })).json() };
}
async function json(page: Page, method: "get" | "post" | "patch", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}

test("support preserves refinement, privacy, duplicate context, and accountable human-agent escalation", async ({ browser }) => {
  const suffix = Date.now().toString(36), copies: string[] = [];
  try {
    const ownerPage = await (await browser.newContext()).newPage();
    const askerPage = await (await browser.newContext()).newPage();
    const readerPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, "SDK Maintainer", `support-owner-${suffix}`);
    const asker = await account(askerPage, "Package User", `support-asker-${suffix}`);
    const reader = await account(readerPage, "Public Reader", `support-reader-${suffix}`);
    const repo = await json(ownerPage, "post", "/repositories", owner.headers, { name: `support-sdk-${suffix}` });
    await json(ownerPage, "patch", `/repositories/${repo.id}`, owner.headers, { visibility: "public" });
    const credential = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "support baseline", scopes: ["git:read", "git:write"], expires_in: 3600 });
    const copy = await mkdtemp(join(tmpdir(), "vivarium-support-")); copies.push(copy);
    await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${repo.id}.git`, copy);
    await git(copy, "config", "user.name", "SDK Maintainer"); await git(copy, "config", "user.email", "maintainer@example.test");
    await writeFile(join(copy, "README.md"), "# SDK integration\n"); await git(copy, "add", "."); await git(copy, "commit", "-m", "Establish SDK support baseline"); await git(copy, "push", "origin", "main");

    await askerPage.goto(`/support?repository=${repo.id}`);
    await askerPage.getByLabel("Target name").fill("Upload SDK");
    await askerPage.getByLabel("Exact version or revision").fill("2.1.0");
    await askerPage.getByLabel("Question title").fill("How do I retry an accepted upload chunk?");
    await askerPage.getByLabel("Question", { exact: true }).fill("The integration times out after a chunk is accepted.");
    await askerPage.getByLabel("Goal").fill("Retry without uploading the accepted chunk twice.");
    await askerPage.getByLabel(/What you tried/).fill("Retried the request\nObserved a duplicate chunk");
    await askerPage.getByLabel("Operating system").fill("Ubuntu 24.04");
    await askerPage.getByLabel("Runtime / SDK").fill("Go 1.26");
    await askerPage.getByRole("button", { name: "Ask for help" }).click();
    await expect(askerPage.getByRole("heading", { level: 2, name: "How do I retry an accepted upload chunk?" })).toBeVisible();

    await ownerPage.goto("/support");
    await ownerPage.getByRole("button", { name: /How do I retry an accepted upload chunk/ }).click();
    await ownerPage.getByLabel("Add context or reply").fill("Does the timeout happen before or after the server accepts the first chunk?");
    await ownerPage.getByRole("button", { name: "Reply in thread" }).click();
    await expect(ownerPage.getByText(/Does the timeout happen/)).toBeVisible();
    await askerPage.reload(); await askerPage.getByRole("button", { name: /How do I retry an accepted upload chunk/ }).click();
    await expect(askerPage.getByText("A repository participant replied to your support question.")).toBeVisible();
    await askerPage.getByLabel("Add context or reply").fill("It happens after the first chunk is accepted; the retry repeats that chunk.");
    await askerPage.getByRole("button", { name: "Reply in thread" }).click();
    const threads = await json(askerPage, "get", `/repositories/${repo.id}/support-threads`, asker.headers);
    const first = threads.threads.find((x: any) => x.title.startsWith("How do I retry"));
    await json(ownerPage, "patch", `/repositories/${repo.id}/support-threads/${first.id}`, owner.headers, { status: "answered", expected_version: first.version, message: "The refined sequence is ready for cited verification." });
    const duplicate = await json(readerPage, "post", `/repositories/${repo.id}/support-threads`, reader.headers, { title: "Retry accepted upload chunk", body: "The upload chunk repeats after timeout.", target: { kind: "package", label: "Upload SDK", version: "2.1.0" }, environment: { runtime: "Go 1.26" }, goal: "Avoid a duplicate chunk.", attempted_steps: ["retried after timeout"], urgency: "normal", audience: "public", contact_preferences: { reply_in_thread: true } });
    const related = await json(readerPage, "get", `/repositories/${repo.id}/support-threads/${duplicate.id}`, reader.headers);
    expect(related.related.some((x: any) => x.id === first.id)).toBe(true);

    const privateThread = await json(askerPage, "post", `/repositories/${repo.id}/support-threads`, asker.headers, { title: "Private customer upload trace", body: "The bounded trace contains a customer-only identifier.", target: { kind: "error", label: "Upload timeout", version: "2.1.0" }, environment: { runtime: "Go 1.26" }, goal: "Find the product gap.", attempted_steps: ["captured the bounded trace"], urgency: "high", audience: "maintainers", contact_preferences: { reply_in_thread: true }, attachments: [{ kind: "log", name: "private.log", media_type: "text/plain", data: btoa("customer-only evidence") }] });
    const hidden = await readerPage.request.get(`/api/repositories/${repo.id}/support-threads/${privateThread.id}`, { headers: reader.headers });
    expect(hidden.status()).toBe(404);
    const escalated = await json(ownerPage, "post", `/repositories/${repo.id}/support-threads/${privateThread.id}/escalations`, owner.headers, { classification: "compatibility_problem", resource_kind: "ordered_work", expected_version: privateThread.version, acceptance_criteria: ["document the version-bound retry", "prevent duplicate accepted chunks"], tasks: [{ title: "Document the supported retry", outcome: "Version-bound guidance is reviewable", risk: "stale guidance", verification_plan: "review the exact example", assignee_type: "human", assignee_id: owner.user.id }, { title: "Implement idempotent chunk retry", outcome: "Accepted chunks are not repeated", risk: "duplicate writes", verification_plan: "run the bounded reproduction", assignee_type: "agent" }] });
    expect(escalated.escalations[0]).toMatchObject({ status: "published", affected_version: "2.1.0" });
    const proposal = await json(ownerPage, "get", `/repositories/${repo.id}/proposals/${escalated.escalations[0].resource_id}`, owner.headers);
    expect(proposal.body).not.toContain("customer-only evidence");
    const tasks = await json(ownerPage, "get", `/repositories/${repo.id}/proposals/${proposal.id}/tasks`, owner.headers);
    expect(tasks.tasks.map((x: any) => x.assignment.assignee_type)).toEqual(["human", "agent"]);
    const staleSupportRequests: string[] = [];
    ownerPage.on("response", (response) => {
      if (response.url().includes("deleted-or-inaccessible/support-")) staleSupportRequests.push(response.url());
    });
    await ownerPage.goto("/support?repository=deleted-or-inaccessible");
    await expect(ownerPage.getByRole("button", { name: "Ask for help" })).toBeEnabled();
    await expect(ownerPage.getByRole("option", { name: repo.name })).toHaveCount(1);
    await expect(ownerPage.getByText("repository not found", { exact: false })).toHaveCount(0);
    expect(staleSupportRequests).toEqual([]);
  } finally { await Promise.all(copies.map((path) => rm(path, { recursive: true, force: true }))); }
});
