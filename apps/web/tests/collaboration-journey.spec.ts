import { expect, test, type Page } from "@playwright/test";
import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import { execFile } from "node:child_process";

const run = promisify(execFile);

async function createAccount(page: Page, displayName: string, handle: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(displayName);
  await page.getByLabel("Handle").fill(handle);
  await page.getByRole("button", { name: "Create account and continue" }).click();
  await expect(page.getByRole("heading", { name: "Create a repository" })).toBeVisible();
}

async function issueGitToken(page: Page, name: string) {
  await page.goto("/settings");
  await page.getByLabel("Name", { exact: true }).fill(name);
  await page.getByRole("button", { name: "Create token" }).click();
  const token = await page.getByText("Copy this token now").locator("..").locator("code").textContent();
  expect(token).toBeTruthy();
  return token!;
}

async function git(cwd: string, ...args: string[]) {
  const { stdout } = await run("git", args, { cwd, env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } });
  return stdout.trim();
}

test("an unknown contributor and an agent take a forked change through verified merge", async ({ browser }) => {
  test.setTimeout(180_000);
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() =>
    run("docker", ["pull", "alpine:3.22"]),
  );
  const suffix = Date.now().toString(36);
  const maintainerContext = await browser.newContext();
  const newcomerContext = await browser.newContext();
  const maintainer = await maintainerContext.newPage();
  const newcomer = await newcomerContext.newPage();

  await createAccount(maintainer, "Journey Maintainer", `maintainer-${suffix}`);
  const maintainerToken = await issueGitToken(maintainer, "Journey Git");
  await maintainer.goto("/repositories");
  await maintainer.getByLabel("Repository name").fill(`welcome-${suffix}`);
  await maintainer.getByRole("button", { name: "Create repository" }).click();
  await maintainer.getByRole("link", { name: new RegExp(`welcome-${suffix}`) }).click();
  await expect(maintainer).toHaveURL(/\/repositories\/[a-f0-9]{32}$/);
  const repositoryID = new URL(maintainer.url()).pathname.split("/").pop()!;
  const remote = `http://git:${maintainerToken}@localhost:3000/git/${repositoryID}.git`;

  const maintainerCopy = await mkdtemp(join(tmpdir(), "vivarium-maintainer-"));
  await git(tmpdir(), "clone", remote, maintainerCopy);
  await git(maintainerCopy, "config", "user.name", "Journey Maintainer");
  await git(maintainerCopy, "config", "user.email", "maintainer@example.com");
  await writeFile(join(maintainerCopy, "README.md"), "# Welcome\n");
  await mkdir(join(maintainerCopy, ".vivarium"));
  await writeFile(join(maintainerCopy, ".vivarium", "checks.json"), JSON.stringify({
    version: 1,
    checks: [{
      name: "greeting verification",
      image: "alpine:3.22",
      command: "echo 'checking for delegated verification'; printf 'expected agent verified marker\\n' > \"$VIVARIUM_OUTPUT/diagnosis.txt\"; grep -qx 'agent verified' greeting.txt",
    }],
  }, null, 2) + "\n");
  await git(maintainerCopy, "add", "README.md", ".vivarium/checks.json");
  await git(maintainerCopy, "commit", "-m", "Start project");
  await git(maintainerCopy, "push", "origin", "main");

  await maintainer.goto(`/repositories/${repositoryID}`);
  await maintainer.getByLabel("Visibility").selectOption("public");
  await expect(maintainer.getByText("public", { exact: true }).first()).toBeVisible();
  await maintainer.getByLabel("Required check names").fill("greeting verification");
  const requirementSaved = maintainer.waitForResponse((response) =>
    response.request().method() === "PUT" && response.url().includes(`/repositories/${repositoryID}/branches/main/required-checks`),
  );
  await maintainer.getByRole("button", { name: "Save requirements" }).click();
  expect((await requirementSaved).status()).toBe(200);

  await createAccount(newcomer, "Journey Newcomer", `newcomer-${suffix}`);
  const newcomerToken = await issueGitToken(newcomer, "Journey Git");
  await newcomer.goto(`/repositories/${repositoryID}`);
  await expect(newcomer.getByRole("heading", { name: `welcome-${suffix}` })).toBeVisible();
  await expect(newcomer.getByText("public", { exact: true }).first()).toBeVisible();
  await newcomer.getByRole("button", { name: "Create fork" }).click();
  await newcomer.waitForURL((url) =>
    url.pathname.startsWith("/repositories/") && !url.pathname.endsWith(repositoryID),
  );
  const forkID = new URL(newcomer.url()).pathname.split("/").pop()!;
  expect(forkID).not.toBe(repositoryID);
  await expect(newcomer.getByText("fork", { exact: true }).first()).toBeVisible();

  await writeFile(join(maintainerCopy, "CONTRIBUTING.md"), "Please keep examples welcoming.\n");
  await git(maintainerCopy, "add", "CONTRIBUTING.md");
  await git(maintainerCopy, "commit", "-m", "Document contribution guidance");
  await git(maintainerCopy, "push", "origin", "main");
  await newcomer.getByRole("button", { name: "Sync main" }).click();
  await expect(newcomer.getByText(/Updated to [a-f0-9]{7}\./)).toBeVisible();

  const newcomerCopy = await mkdtemp(join(tmpdir(), "vivarium-newcomer-"));
  await git(tmpdir(), "clone", `http://git:${newcomerToken}@localhost:3000/git/${forkID}.git`, newcomerCopy);
  await git(newcomerCopy, "config", "user.name", "Journey Newcomer");
  await git(newcomerCopy, "config", "user.email", "newcomer@example.com");
  await git(newcomerCopy, "switch", "-c", "greeting");
  await writeFile(join(newcomerCopy, "greeting.txt"), "hello developers and agents\n");
  await git(newcomerCopy, "add", "greeting.txt");
  await git(newcomerCopy, "commit", "-m", "Add greeting");
  await git(newcomerCopy, "push", "origin", "greeting");

  await newcomer.goto("/pulls");
  await newcomer.getByRole("button", { name: "New pull request" }).click();
  await newcomer.getByLabel("Candidate branch").selectOption("greeting");
  await newcomer.getByLabel("Target branch").selectOption("main");
  await newcomer.getByLabel("Title").fill("Add a greeting");
  await newcomer.getByLabel("Purpose and feedback needed").fill("Implements the agreed welcome.");
  await newcomer.getByRole("button", { name: "Open pull request" }).click();
  await expect(newcomer).toHaveURL(new RegExp(`/pulls/${repositoryID}/[a-f0-9]{32}$`));
  const pullRequestID = new URL(newcomer.url()).pathname.split("/").pop()!;
  const pullRequestURL = newcomer.url();
  await expect(newcomer.getByText(`@newcomer-${suffix}`, { exact: true }).first()).toBeVisible();
  await expect(newcomer.getByText(new RegExp(`welcome-${suffix}:greeting.*welcome-${suffix}:main`))).toBeVisible();
  await newcomer.getByRole("button", { name: "Allow maintainer edits" }).click();
  await expect(newcomer.getByRole("button", { name: "Disable maintainer edits" })).toBeVisible();

  await maintainer.goto(newcomer.url());
  await maintainer.getByLabel("Add review feedback").fill("Please include a friendly verified example.");
  await maintainer.getByRole("button", { name: "Comment" }).click();
  await expect(maintainer.getByText("Please include a friendly verified example.")).toBeVisible();

  const checks = maintainer.locator("#checks");
  await expect(checks.getByText("failed", { exact: true })).toBeVisible({ timeout: 60_000 });
  await checks.getByText("greeting verification", { exact: true }).click();
  await expect(checks.getByText(/checking for delegated verification/)).toBeVisible();
  await expect(checks.getByText(/diagnosis\.txt/)).toBeVisible();
  await expect(maintainer.getByText("failed", { exact: true }).last()).toBeVisible();
  await expect(maintainer.getByRole("button", { name: "Merge into main" })).toBeDisabled();
  await checks.getByRole("button", { name: "Repair with agent" }).click();
  await expect(maintainer).toHaveURL(new RegExp(`/pulls/${repositoryID}/[a-f0-9]{32}/sessions/[a-f0-9]{32}$`));
  await expect(maintainer.getByRole("heading", { name: "Session timeline" })).toBeVisible();
  await expect(maintainer.getByText("Change session opened")).toBeVisible();
  await expect(maintainer.getByText("Automated failure")).toBeVisible();
  await expect(maintainer.getByRole("heading", { name: "greeting verification" })).toBeVisible();
  await expect(maintainer.getByText(/checking for delegated verification/).last()).toBeVisible();
  await expect(maintainer.getByText(/diagnosis\.txt/).last()).toBeVisible();
  await maintainer.reload();
  await expect(maintainer.getByText(`@maintainer-${suffix}`, { exact: true })).toBeVisible();
  await maintainer.getByLabel("Instructions").fill("Verify the greeting behavior and add focused coverage.");
  await maintainer.getByLabel("Repository context").fill("greeting.txt");
  await expect(maintainer.getByLabel("Working branch")).toHaveValue("greeting");
  const launchResponse = maintainer.waitForResponse((response) =>
    response.request().method() === "POST" && /\/sessions\/[a-f0-9]{32}\/runs$/.test(new URL(response.url()).pathname),
  );
  await maintainer.getByRole("button", { name: "Launch agent run" }).click();
  const launched: { run: { id: string }; credential: { token: string } } = await (await launchResponse).json();
  await expect(maintainer.getByText("Run launched · copy its credential now")).toBeVisible();
  await expect(maintainer.getByText("Agent run launched")).toBeVisible();
  const runCard = maintainer.locator("section").filter({ hasText: "Verify the greeting behavior and add focused coverage." });
  const interventionPattern = /\/sessions\/[a-f0-9]{32}\/runs\/[a-f0-9]{32}\/interventions$/;
  await maintainer.route(interventionPattern, async (route) => {
    await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { code: "temporarily_unavailable", message: "Intervention storage is temporarily unavailable." } }) });
  }, { times: 1 });
  await runCard.getByLabel("Guidance message").fill("Keep the regression focused on the public behavior.");
  await runCard.getByRole("button", { name: "Send to agent" }).click();
  await expect(maintainer.getByText(/Your draft is still available to retry\./)).toBeVisible();
  await expect(runCard.getByLabel("Guidance message")).toHaveValue("Keep the regression focused on the public behavior.");
  await maintainer.unroute(interventionPattern);
  await runCard.getByRole("button", { name: "Send to agent" }).click();
  await expect(maintainer.locator("ol").getByText("Follow-up guidance", { exact: true })).toBeVisible();
  await runCard.getByRole("button", { name: "Pause run" }).click();
  await expect(runCard.getByText("paused", { exact: true })).toBeVisible();
  await expect(maintainer.getByText("Run paused")).toBeVisible();
  await runCard.getByRole("button", { name: "Resume run" }).click();
  await expect(runCard.getByText("launched", { exact: true })).toBeVisible();
  await expect(maintainer.getByText("Run resumed")).toBeVisible();

  const sessionURL = maintainer.url();
  await maintainer.getByRole("link", { name: "← Back to pull request" }).click();
  await maintainer.getByRole("button", { name: "Approve" }).click();
  await expect(maintainer.getByText(`@maintainer-${suffix}`, { exact: true }).first()).toBeVisible();
  await expect(maintainer.getByRole("button", { name: "Merge into main" })).toBeDisabled();

  const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-agent-"));
  const agentRemote = `http://git:${launched.credential.token}@localhost:3000/git/${forkID}.git`;
  await git(tmpdir(), "clone", agentRemote, agentCopy);
  await git(agentCopy, "config", "user.name", "Vivarium Agent");
  await git(agentCopy, "config", "user.email", "agent@users.vivarium");
  await git(agentCopy, "switch", "-c", "agent-greeting", "origin/greeting");
  await writeFile(join(agentCopy, "greeting.txt"), "hello developers and agents\nagent verified\n");
  await git(agentCopy, "add", "greeting.txt");
  await git(agentCopy, "commit", "-m", "Verify delegated greeting");
  await git(agentCopy, "push", "origin", "HEAD:refs/heads/greeting");
  const agentCommit = await git(agentCopy, "rev-parse", "HEAD");
  const runBase = `/api/repositories/${repositoryID}/pulls/${pullRequestID}/sessions/${new URL(sessionURL).pathname.split("/").pop()}/runs/${launched.run.id}`;
  const agentHeaders = { Authorization: `Bearer ${launched.credential.token}` };
  const progress = await maintainer.request.post(`${runBase}/events`, {
    headers: agentHeaders,
    data: { kind: "branch.updated", state: "working", message: "Published the redirected greeting revision.", branch: "greeting", commit_id: agentCommit },
  });
  expect(progress.status()).toBe(201);
  const invalidCompletion = await maintainer.request.post(`${runBase}/completion`, {
    headers: agentHeaders,
    data: { summary: "Invalid check evidence must not move review state.", commit_id: agentCommit, checks: [{ name: "greeting check", status: "unknown" }] },
  });
  expect(invalidCompletion.status()).toBe(400);
  const completion = await maintainer.request.post(`${runBase}/completion`, {
    headers: agentHeaders,
    data: { summary: "Applied the maintainer's direction and verified the delegated greeting.", commit_id: agentCommit, checks: [{ name: "greeting check", status: "passed", details: "Expected content is present." }], unresolved_concerns: [] },
  });
  expect(completion.status()).toBe(201);

  await maintainer.goto(sessionURL);
  const handoff = maintainer.locator("#outcome");
  await expect(handoff.getByRole("heading", { name: "Review handoff" })).toBeVisible();
  await expect(handoff.getByText("Applied the maintainer's direction and verified the delegated greeting.")).toBeVisible();
  await expect(handoff.getByText("greeting check", { exact: true })).toBeVisible();
  await expect(handoff.getByRole("link", { name: "greeting.txt" })).toBeVisible();
  await expect(maintainer.getByText("Work published for review")).toBeVisible();
  await maintainer.reload();
  await expect(maintainer.getByRole("heading", { name: "Review handoff" })).toBeVisible();
  await maintainer.getByRole("link", { name: new RegExp(`Review revision ${agentCommit.slice(0, 7)}`) }).click();
  await expect(maintainer.getByText("0 / 1 required approvals")).toBeVisible();
  await expect(maintainer.getByRole("button", { name: "Merge into main" })).toBeDisabled();
  const repairedChecks = maintainer.locator("#checks");
  await expect(repairedChecks.getByText("succeeded", { exact: true })).toBeVisible({ timeout: 60_000 });
  await expect(repairedChecks.getByText("greeting verification", { exact: true })).toHaveCount(2);
  await expect(repairedChecks.getByText("failed", { exact: true })).toBeVisible();
  await expect(maintainer.getByText("passed", { exact: true }).last()).toBeVisible();
  await maintainer.getByRole("button", { name: "Approve" }).click();
  await expect(maintainer.getByRole("button", { name: "Merge into main" })).toBeEnabled();
  await maintainer.getByRole("button", { name: "Merge into main" }).click();
  await expect(maintainer.getByText("Merged", { exact: true })).toBeVisible();
  await expect(maintainer.getByText(`@maintainer-${suffix}`, { exact: true }).first()).toBeVisible();

  await git(maintainerCopy, "pull", "--ff-only");
  expect(await readFile(join(maintainerCopy, "greeting.txt"), "utf8")).toBe("hello developers and agents\nagent verified\n");
  expect(await readFile(join(maintainerCopy, "CONTRIBUTING.md"), "utf8")).toBe("Please keep examples welcoming.\n");

  await newcomer.goto(pullRequestURL);
  await expect(newcomer.getByText("Merged", { exact: true })).toBeVisible();
  await expect(newcomer.getByText(`@newcomer-${suffix}`, { exact: true }).first()).toBeVisible();
  await expect(newcomer.getByText(`@maintainer-${suffix}`, { exact: true }).first()).toBeVisible();
  const newcomerSession = await newcomer.evaluate(() => localStorage.getItem("vivarium.access-token"));
  const durablePullResponse = await newcomer.request.get(
    `/api/repositories/${repositoryID}/pulls/${pullRequestID}`,
    { headers: { Authorization: `Bearer ${newcomerSession}` } },
  );
  expect(durablePullResponse.status()).toBe(200);
  const durablePull = await durablePullResponse.json();
  expect(durablePull.source_repository_id).toBe(forkID);
  expect(durablePull.repository_id).toBe(repositoryID);
  expect(durablePull.author_id).not.toBe(durablePull.merged_by);
  expect(durablePull.merge_commit_id).toMatch(/^[a-f0-9]{40}$/);
  await newcomer.reload();
  await expect(newcomer.getByText("Merged", { exact: true })).toBeVisible();

  await maintainerContext.close();
  await newcomerContext.close();
});

test("a protected queue lands parallel human and agent changes without stale evidence", async ({ browser }) => {
  test.setTimeout(240_000);
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() =>
    run("docker", ["pull", "alpine:3.22"]),
  );
  const suffix = Date.now().toString(36);
  const maintainerContext = await browser.newContext();
  const contributorContext = await browser.newContext();
  const maintainer = await maintainerContext.newPage();
  const contributor = await contributorContext.newPage();

  await createAccount(maintainer, "Queue Maintainer", `queue-maintainer-${suffix}`);
  const maintainerGitToken = await issueGitToken(maintainer, "Queue Git");
  const maintainerAPIToken = await maintainer.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(maintainerAPIToken).toBeTruthy();
  const maintainerHeaders = { Authorization: `Bearer ${maintainerAPIToken}` };
  const maintainerUserResponse = await maintainer.request.get("/api/user", { headers: maintainerHeaders });
  expect(maintainerUserResponse.status()).toBe(200);
  const maintainerUser: { id: string } = await maintainerUserResponse.json();

  await maintainer.goto("/repositories");
  await maintainer.getByLabel("Repository name").fill(`queue-${suffix}`);
  await maintainer.getByRole("button", { name: "Create repository" }).click();
  await maintainer.getByRole("link", { name: new RegExp(`queue-${suffix}`) }).click();
  await expect(maintainer).toHaveURL(/\/repositories\/[a-f0-9]{32}$/);
  const repositoryID = new URL(maintainer.url()).pathname.split("/").pop()!;
  const remote = `http://git:${maintainerGitToken}@localhost:3000/git/${repositoryID}.git`;
  const maintainerCopy = await mkdtemp(join(tmpdir(), "vivarium-queue-maintainer-"));
  await git(tmpdir(), "clone", remote, maintainerCopy);
  await git(maintainerCopy, "config", "user.name", "Queue Maintainer");
  await git(maintainerCopy, "config", "user.email", "queue-maintainer@example.com");
  await writeFile(join(maintainerCopy, "README.md"), "# Continuous integration\n");
  await writeFile(join(maintainerCopy, "shared.txt"), "base\n");
  await mkdir(join(maintainerCopy, ".vivarium"));
  await writeFile(join(maintainerCopy, ".vivarium", "checks.json"), JSON.stringify({
    version: 1,
    checks: [{
      name: "integration safety",
      image: "alpine:3.22",
      command: "sleep 5; test -f README.md && test -f shared.txt",
    }],
  }, null, 2) + "\n");
  await git(maintainerCopy, "add", ".");
  await git(maintainerCopy, "commit", "-m", "Protect continuous integration");
  await git(maintainerCopy, "push", "origin", "main");

  await createAccount(contributor, "Queue Contributor", `queue-contributor-${suffix}`);
  const contributorGitToken = await issueGitToken(contributor, "Queue Git");
  const contributorAPIToken = await contributor.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(contributorAPIToken).toBeTruthy();
  const contributorHeaders = { Authorization: `Bearer ${contributorAPIToken}` };
  const contributorUserResponse = await contributor.request.get("/api/user", { headers: contributorHeaders });
  expect(contributorUserResponse.status()).toBe(200);
  const contributorUser: { id: string } = await contributorUserResponse.json();
  const collaborationID = (await contributor.getByTestId("collaboration-id").textContent())!;
  await maintainer.goto(`/repositories/${repositoryID}`);
  await maintainer.getByLabel("Collaboration ID").fill(collaborationID);
  await maintainer.getByRole("button", { name: "Add", exact: true }).click();
  await expect(maintainer.getByText(`@queue-contributor-${suffix}`)).toBeVisible();

  const contributorCopy = await mkdtemp(join(tmpdir(), "vivarium-queue-contributor-"));
  await git(tmpdir(), "clone", `http://git:${contributorGitToken}@localhost:3000/git/${repositoryID}.git`, contributorCopy);
  await git(contributorCopy, "config", "user.name", "Queue Contributor");
  await git(contributorCopy, "config", "user.email", "queue-contributor@example.com");
  await git(contributorCopy, "switch", "-c", "human-first");
  await writeFile(join(contributorCopy, "shared.txt"), "first compatible decision\n");
  await git(contributorCopy, "add", "shared.txt");
  await git(contributorCopy, "commit", "-m", "Choose the first shared value");
  await git(contributorCopy, "push", "origin", "human-first");
  await git(contributorCopy, "switch", "-c", "human-conflict", "origin/main");
  await writeFile(join(contributorCopy, "shared.txt"), "conflicting decision\n");
  await git(contributorCopy, "add", "shared.txt");
  await git(contributorCopy, "commit", "-m", "Choose a conflicting shared value");
  await git(contributorCopy, "push", "origin", "human-conflict");
  await git(contributorCopy, "switch", "-c", "agent-compatible", "origin/main");
  await writeFile(join(contributorCopy, "agent-draft.txt"), "ready for delegated completion\n");
  await git(contributorCopy, "add", "agent-draft.txt");
  await git(contributorCopy, "commit", "-m", "Prepare delegated change");
  await git(contributorCopy, "push", "origin", "agent-compatible");

  async function postJSON(page: Page, path: string, headers: Record<string, string>, data?: unknown) {
    const response = await page.request.post(`/api${path}`, { headers, data });
    expect(response.status(), `POST ${path}: ${await response.text()}`).toBeGreaterThanOrEqual(200);
    expect(response.status(), `POST ${path}`).toBeLessThan(300);
    return response.json();
  }
  async function createPull(title: string, sourceBranch: string) {
    return postJSON(contributor, `/repositories/${repositoryID}/pulls`, contributorHeaders, {
      title,
      body: "Independently reviewed parallel queue work.",
      source_branch: sourceBranch,
      target_branch: "main",
    }) as Promise<{ id: string; source_commit_id: string }>;
  }
  const firstPull = await createPull("Land the first compatible change", "human-first");
  const conflictPull = await createPull("Isolate the conflicting change", "human-conflict");
  const agentPull = await createPull("Land the delegated compatible change", "agent-compatible");

  const session = await postJSON(
    contributor,
    `/repositories/${repositoryID}/pulls/${agentPull.id}/sessions`,
    contributorHeaders,
  ) as { id: string; source_commit_id: string };
  const launched = await postJSON(
    contributor,
    `/repositories/${repositoryID}/pulls/${agentPull.id}/sessions/${session.id}/runs`,
    contributorHeaders,
    {
      instructions: "Complete the independent agent contribution without touching the shared decision.",
      source_commit_id: session.source_commit_id,
      context_paths: ["README.md", "agent-draft.txt"],
      working_branch: "agent-compatible",
      expires_in: 3600,
    },
  ) as { run: { id: string; agent_id: string }; credential: { token: string } };
  const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-queue-agent-"));
  await git(tmpdir(), "clone", `http://git:${launched.credential.token}@localhost:3000/git/${repositoryID}.git`, agentCopy);
  await git(agentCopy, "config", "user.name", "Vivarium Queue Agent");
  await git(agentCopy, "config", "user.email", "queue-agent@users.vivarium");
  await git(agentCopy, "switch", "-c", "agent-work", "origin/agent-compatible");
  await writeFile(join(agentCopy, "agent-result.txt"), "compatible delegated result\n");
  await git(agentCopy, "add", "agent-result.txt");
  await git(agentCopy, "commit", "-m", "Complete compatible delegated work");
  await git(agentCopy, "push", "origin", "HEAD:refs/heads/agent-compatible");
  const agentCommit = await git(agentCopy, "rev-parse", "HEAD");
  const agentHeaders = { Authorization: `Bearer ${launched.credential.token}` };
  await postJSON(
    contributor,
    `/repositories/${repositoryID}/pulls/${agentPull.id}/sessions/${session.id}/runs/${launched.run.id}/events`,
    agentHeaders,
    { kind: "branch.updated", state: "working", message: "Published the compatible queue contribution.", branch: "agent-compatible", commit_id: agentCommit },
  );
  await postJSON(
    contributor,
    `/repositories/${repositoryID}/pulls/${agentPull.id}/sessions/${session.id}/runs/${launched.run.id}/completion`,
    agentHeaders,
    { summary: "Completed an independently reviewable compatible change.", commit_id: agentCommit, checks: [{ name: "agent scope", status: "passed", details: "The shared file is unchanged." }], unresolved_concerns: [] },
  );

  for (const pull of [firstPull, conflictPull, agentPull]) {
    await postJSON(maintainer, `/repositories/${repositoryID}/pulls/${pull.id}/reviews`, maintainerHeaders, { decision: "approved" });
  }
  await maintainer.goto(`/repositories/${repositoryID}`);
  await maintainer.getByLabel("Required check names").fill("integration safety");
  await maintainer.getByRole("button", { name: "Save requirements" }).click();
  await maintainer.getByLabel("Require queue").check();
  await maintainer.getByLabel("Concurrent candidates").fill("3");
  await maintainer.getByLabel("On candidate failure").selectOption("remove");
  await maintainer.getByRole("button", { name: "Save queue policy" }).click();

  for (const pull of [firstPull, conflictPull, agentPull]) {
    await expect.poll(async () => {
      const response = await maintainer.request.get(
        `/api/repositories/${repositoryID}/pulls/${pull.id}/merge-readiness`,
        { headers: maintainerHeaders },
      );
      if (response.status() !== 200) return false;
      return (await response.json()).can_enqueue;
    }, { timeout: 60_000 }).toBe(true);
  }
  for (const pull of [firstPull, conflictPull, agentPull]) {
    await postJSON(maintainer, `/repositories/${repositoryID}/pulls/${pull.id}/queue`, maintainerHeaders);
  }

  await maintainer.goto(`/repositories/${repositoryID}/queue/main`);
  await expect(maintainer.getByRole("heading", { name: "main queue" })).toBeVisible();
  await expect(maintainer.getByRole("link", { name: "Land the first compatible change" })).toBeVisible();
  await expect(maintainer.getByRole("link", { name: "Isolate the conflicting change" })).toBeVisible();
  await expect(maintainer.getByRole("link", { name: "Land the delegated compatible change" })).toBeVisible();

  type DurablePull = {
    status: string;
    queued_at?: string;
    queued_by?: string;
    merged_by?: string;
    merge_commit_id?: string;
    integration_candidates: Array<{
      base_commit_id: string;
      commit_id: string;
      superseded_at?: string;
      superseded_reason?: string;
    }>;
    queue_actions: Array<{ action: string; actor_id: string }>;
  };
  async function durablePull(id: string) {
    const response = await maintainer.request.get(`/api/repositories/${repositoryID}/pulls/${id}`, { headers: maintainerHeaders });
    expect(response.status()).toBe(200);
    return response.json() as Promise<DurablePull>;
  }
  await expect.poll(async () => {
    const [first, conflict, agent] = await Promise.all([
      durablePull(firstPull.id), durablePull(conflictPull.id), durablePull(agentPull.id),
    ]);
    return `${first.status}:${conflict.status}:${Boolean(conflict.queued_at)}:${agent.status}`;
  }, { timeout: 120_000, intervals: [1000, 2000, 5000] }).toBe("merged:open:false:merged");

  const firstResult = await durablePull(firstPull.id);
  const conflictResult = await durablePull(conflictPull.id);
  const agentResult = await durablePull(agentPull.id);
  expect(firstResult.merged_by).toBe(maintainerUser.id);
  expect(agentResult.merged_by).toBe(maintainerUser.id);
  expect(firstResult.merge_commit_id).toMatch(/^[a-f0-9]{40}$/);
  expect(agentResult.merge_commit_id).toMatch(/^[a-f0-9]{40}$/);
  expect(conflictResult.integration_candidates).toHaveLength(1);
  expect(conflictResult.integration_candidates[0].superseded_reason).toBe("target_changed");
  expect(conflictResult.integration_candidates[0].superseded_at).toBeTruthy();
  expect(agentResult.integration_candidates).toHaveLength(2);
  expect(agentResult.integration_candidates[0].superseded_reason).toBe("target_changed");
  expect(agentResult.integration_candidates[1].base_commit_id).toBe(firstResult.merge_commit_id);
  for (const result of [firstResult, conflictResult, agentResult]) {
    expect(result.queue_actions[0]).toMatchObject({ action: "enqueued", actor_id: maintainerUser.id });
  }

  const candidateResponse = await maintainer.request.get(
    `/api/repositories/${repositoryID}/pulls/${conflictPull.id}/candidates`,
    { headers: maintainerHeaders },
  );
  expect(candidateResponse.status()).toBe(200);
  const candidateEvidence: { candidates: Array<{ state: string; checks: Array<{ state: string; commit_id: string }> }> } = await candidateResponse.json();
  expect(candidateEvidence.candidates).toHaveLength(1);
  expect(candidateEvidence.candidates[0].state).toBe("superseded");
  expect(candidateEvidence.candidates[0].checks).toEqual([
    expect.objectContaining({ state: "succeeded", commit_id: conflictResult.integration_candidates[0].commit_id }),
  ]);

  const timelineResponse = await contributor.request.get(
    `/api/repositories/${repositoryID}/pulls/${agentPull.id}/sessions/${session.id}/events?limit=100`,
    { headers: contributorHeaders },
  );
  expect(timelineResponse.status()).toBe(200);
  const timeline: { events: Array<{ kind: string; actor_id: string; agent_id?: string; initiator_id?: string; run_id?: string }> } = await timelineResponse.json();
  expect(timeline.events).toEqual(expect.arrayContaining([
    expect.objectContaining({ kind: "run.completed", agent_id: launched.run.agent_id, initiator_id: contributorUser.id, run_id: launched.run.id }),
  ]));

  const activityResponse = await maintainer.request.get("/api/activity?limit=100", { headers: maintainerHeaders });
  expect(activityResponse.status()).toBe(200);
  const activity: { events: Array<{ kind: string; actor_id: string; resource_id: string }> } = await activityResponse.json();
  for (const pull of [firstPull, conflictPull, agentPull]) {
    expect(activity.events).toEqual(expect.arrayContaining([
      expect.objectContaining({ kind: "integration_queue.enqueued", actor_id: maintainerUser.id, resource_id: pull.id }),
    ]));
  }
  for (const pull of [firstPull, agentPull]) {
    expect(activity.events).toEqual(expect.arrayContaining([
      expect.objectContaining({ kind: "pull_request.merged", actor_id: maintainerUser.id, resource_id: pull.id }),
    ]));
  }

  await git(maintainerCopy, "pull", "--ff-only");
  expect(await readFile(join(maintainerCopy, "shared.txt"), "utf8")).toBe("first compatible decision\n");
  expect(await readFile(join(maintainerCopy, "agent-result.txt"), "utf8")).toBe("compatible delegated result\n");
  await expect(readFile(join(maintainerCopy, "agent-draft.txt"), "utf8")).resolves.toBe("ready for delegated completion\n");

  await maintainerContext.close();
  await contributorContext.close();
});

test("a proposal plan and incident response preserve collaboration through verified delivery", async ({ browser }) => {
	test.setTimeout(360_000);
	await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() =>
		run("docker", ["pull", "alpine:3.22"]),
	);
  const suffix = Date.now().toString(36);
  const maintainerContext = await browser.newContext();
  const contributorContext = await browser.newContext();
  const maintainer = await maintainerContext.newPage();
  const contributor = await contributorContext.newPage();

	await createAccount(maintainer, "Plan Maintainer", `plan-maintainer-${suffix}`);
	const maintainerToken = await issueGitToken(maintainer, "Plan Git");
	const maintainerAPIToken = await maintainer.evaluate(() => localStorage.getItem("vivarium.access-token"));
	expect(maintainerAPIToken).toBeTruthy();
	const maintainerHeaders = { Authorization: `Bearer ${maintainerAPIToken}` };
	const maintainerUser = await (await maintainer.request.get("/api/user", { headers: maintainerHeaders })).json() as { id: string };
  await maintainer.goto("/repositories");
	await maintainer.getByLabel("Repository name").fill(`plan-${suffix}`);
  await maintainer.getByRole("button", { name: "Create repository" }).click();
	await maintainer.getByRole("link", { name: new RegExp(`plan-${suffix}`) }).click();
  await expect(maintainer).toHaveURL(/\/repositories\/[a-f0-9]{32}$/);
  const repositoryID = new URL(maintainer.url()).pathname.split("/").pop()!;

	const maintainerCopy = await mkdtemp(join(tmpdir(), "vivarium-plan-maintainer-"));
  await git(tmpdir(), "clone", `http://git:${maintainerToken}@localhost:3000/git/${repositoryID}.git`, maintainerCopy);
	await git(maintainerCopy, "config", "user.name", "Plan Maintainer");
	await git(maintainerCopy, "config", "user.email", "plan-maintainer@example.com");
	await writeFile(join(maintainerCopy, "README.md"), "# Coordinated delivery\n");
	await mkdir(join(maintainerCopy, ".vivarium"));
	await writeFile(join(maintainerCopy, ".vivarium", "checks.json"), JSON.stringify({
		version: 1,
		checks: [{ name: "plan verification", image: "alpine:3.22", command: "test -f README.md" }],
	}, null, 2) + "\n");
	await writeFile(join(maintainerCopy, ".vivarium", "release.json"), JSON.stringify({
		version: 1,
		steps: [{ name: "package service", image: "alpine:3.22", command: "cp rollout-state.txt \"$VIVARIUM_OUTPUT/app.txt\"" }],
	}, null, 2) + "\n");
	await writeFile(join(maintainerCopy, ".vivarium", "deployment.json"), JSON.stringify({
		version: 1,
		stages: [{ name: "production health", signals: [{ name: "service responds", command: "grep -qx healthy \"$VIVARIUM_ARTIFACT\"" }] }],
	}, null, 2) + "\n");
	await writeFile(join(maintainerCopy, "rollout-state.txt"), "healthy\n");
	await git(maintainerCopy, "add", ".");
	await git(maintainerCopy, "commit", "-m", "Start coordinated project");
  await git(maintainerCopy, "push", "origin", "main");
	const baseCommit = await git(maintainerCopy, "rev-parse", "HEAD");
	await maintainer.goto(`/repositories/${repositoryID}`);
	await maintainer.getByLabel("Required check names").fill("plan verification");
	await maintainer.getByRole("button", { name: "Save requirements" }).click();

	await createAccount(contributor, "Plan Contributor", `plan-contributor-${suffix}`);
	const contributorToken = await issueGitToken(contributor, "Plan Git");
	const contributorAPIToken = await contributor.evaluate(() => localStorage.getItem("vivarium.access-token"));
	expect(contributorAPIToken).toBeTruthy();
	const contributorHeaders = { Authorization: `Bearer ${contributorAPIToken}` };
	const contributorUser = await (await contributor.request.get("/api/user", { headers: contributorHeaders })).json() as { id: string };
  const collaborationID = (await contributor.getByTestId("collaboration-id").textContent())!;
  await maintainer.goto(`/repositories/${repositoryID}`);
  await maintainer.getByLabel("Collaboration ID").fill(collaborationID);
  await maintainer.getByRole("button", { name: "Add", exact: true }).click();
	await expect(maintainer.getByText(`@plan-contributor-${suffix}`)).toBeVisible();

  await contributor.goto("/proposals");
  await contributor.getByRole("button", { name: "New proposal" }).click();
	await contributor.getByLabel("Title").fill("Ship a coordinated feature");
	await contributor.getByLabel("Context", { exact: true }).fill("Deliver the human foundation before the delegated follow-up.");
  await contributor.getByRole("button", { name: "Publish proposal" }).click();
  await expect(contributor).toHaveURL(new RegExp(`/proposals/${repositoryID}/[a-f0-9]{32}$`));
  const proposalID = new URL(contributor.url()).pathname.split("/").pop()!;
	await contributor.getByLabel("Add to the conversation").fill("Keep both outcomes independently reviewable and verified.");
	await contributor.getByRole("button", { name: "Comment" }).click();
	await expect(contributor.getByText("Keep both outcomes independently reviewable and verified.", { exact: true })).toBeVisible();

	async function postJSON(page: Page, path: string, headers: Record<string, string>, data: unknown) {
		const response = await page.request.post(`/api${path}`, { headers, data });
		expect(response.status(), `POST ${path}: ${await response.text()}`).toBeGreaterThanOrEqual(200);
		expect(response.status(), `POST ${path}`).toBeLessThan(300);
		return response.json();
	}
	async function putJSON(page: Page, path: string, headers: Record<string, string>, data: unknown) {
		const response = await page.request.put(`/api${path}`, { headers, data });
		expect(response.status(), `PUT ${path}: ${await response.text()}`).toBeGreaterThanOrEqual(200);
		expect(response.status(), `PUT ${path}`).toBeLessThan(300);
		return response.json();
	}
	async function getJSON(page: Page, path: string, headers: Record<string, string>) {
		const response = await page.request.get(`/api${path}`, { headers });
		expect(response.status(), `GET ${path}: ${await response.text()}`).toBe(200);
		return response.json();
	}
	async function eventually<T>(read: () => Promise<T>, ready: (value: T) => boolean, label: string) {
		await expect.poll(async () => ready(await read()), { message: label, timeout: 60_000, intervals: [250, 500, 1000] }).toBe(true);
		return read();
	}
	const commentsResponse = await contributor.request.get(`/api/repositories/${repositoryID}/proposals/${proposalID}/comments`, { headers: contributorHeaders });
	const comments = await commentsResponse.json() as { comments: Array<{ id: string }> };
	const humanTask = await postJSON(contributor, `/repositories/${repositoryID}/proposals/${proposalID}/tasks`, contributorHeaders, {
		title: "Build the human foundation", outcome: "The foundation is merged first.", discussion_comment_ids: [comments.comments[0].id],
	}) as { id: string };
	const agentTask = await postJSON(contributor, `/repositories/${repositoryID}/proposals/${proposalID}/tasks`, contributorHeaders, {
		title: "Add the delegated follow-up", outcome: "Agent work builds on the merged foundation.", dependency_ids: [humanTask.id], discussion_comment_ids: [comments.comments[0].id],
	}) as { id: string; ready: boolean };
	expect(agentTask.ready).toBe(false);
	const blockedAssignment = await maintainer.request.put(`/api/repositories/${repositoryID}/proposals/${proposalID}/tasks/${agentTask.id}/assignment`, {
		headers: maintainerHeaders,
		data: { assignee_type: "agent", mandate: "Do not start before the foundation.", repository_id: repositoryID, base_revision: baseCommit },
	});
	expect(blockedAssignment.status()).toBeGreaterThanOrEqual(400);
	await putJSON(maintainer, `/repositories/${repositoryID}/proposals/${proposalID}/tasks/${humanTask.id}/assignment`, maintainerHeaders, {
		assignee_type: "human", assignee_id: contributorUser.id, mandate: "Publish the independently reviewable foundation.", repository_id: repositoryID, base_revision: baseCommit,
	});

	await contributor.reload();
	await expect(contributor.getByRole("heading", { name: "What can start now" })).toBeVisible();
	await expect(contributor.getByText("Blocked by Build the human foundation")).toBeVisible();
	await expect(contributor.getByText("owned by human", { exact: true })).toBeVisible();

	const contributorCopy = await mkdtemp(join(tmpdir(), "vivarium-plan-contributor-"));
  await git(tmpdir(), "clone", `http://git:${contributorToken}@localhost:3000/git/${repositoryID}.git`, contributorCopy);
	await git(contributorCopy, "config", "user.name", "Plan Contributor");
	await git(contributorCopy, "config", "user.email", "plan-contributor@example.com");
	await git(contributorCopy, "switch", "-c", "human-foundation");
	await writeFile(join(contributorCopy, "foundation.txt"), "human foundation\n");
	await git(contributorCopy, "add", "foundation.txt");
	await git(contributorCopy, "commit", "-m", "Build human foundation");
	await git(contributorCopy, "push", "origin", "human-foundation");
	const humanPull = await postJSON(contributor, `/repositories/${repositoryID}/proposals/${proposalID}/tasks/${humanTask.id}/contributions`, contributorHeaders, {
		title: "Build the human foundation", body: "Implements the first agreed outcome.", source_branch: "human-foundation", target_branch: "main",
	}) as { id: string };
	await maintainer.goto(`/pulls/${repositoryID}/${humanPull.id}`);
	await expect(maintainer.locator("#checks").getByText("succeeded", { exact: true }).last()).toBeVisible({ timeout: 60_000 });
	await maintainer.getByRole("button", { name: "Approve" }).click();
	await expect(maintainer.getByRole("button", { name: "Merge into main" })).toBeEnabled();
	await maintainer.getByRole("button", { name: "Merge into main" }).click();
	await expect(maintainer.getByText("Merged", { exact: true })).toBeVisible();
	const humanResult = await (await maintainer.request.get(`/api/repositories/${repositoryID}/pulls/${humanPull.id}`, { headers: maintainerHeaders })).json() as { merge_commit_id: string };

	const readyTasks = await (await maintainer.request.get(`/api/repositories/${repositoryID}/proposals/${proposalID}/tasks`, { headers: maintainerHeaders })).json() as { tasks: Array<{ id: string; ready: boolean; status: string }> };
	expect(readyTasks.tasks.find((task) => task.id === humanTask.id)).toMatchObject({ status: "completed" });
	expect(readyTasks.tasks.find((task) => task.id === agentTask.id)).toMatchObject({ ready: true });
	const assignedAgentTask = await putJSON(maintainer, `/repositories/${repositoryID}/proposals/${proposalID}/tasks/${agentTask.id}/assignment`, maintainerHeaders, {
		assignee_type: "agent", mandate: "Add the delegated result on the merged foundation.", repository_id: repositoryID, base_revision: humanResult.merge_commit_id,
	}) as { assignment: { id: string; assignee_id: string } };
	const launched = await postJSON(maintainer, `/repositories/${repositoryID}/proposals/${proposalID}/tasks/${agentTask.id}/sessions`, maintainerHeaders, {
		expected_assignment_id: assignedAgentTask.assignment.id, context_paths: ["foundation.txt"], expires_in: 3600,
	}) as { session: { id: string }; run: { id: string; working_branch: string; agent_id: string }; credential: { token: string } };
	const taskRunBase = `/repositories/${repositoryID}/proposals/${proposalID}/tasks/${agentTask.id}/sessions/${launched.session.id}/runs/${launched.run.id}`;
	await postJSON(contributor, `${taskRunBase}/interventions`, contributorHeaders, { kind: "run.guidance", message: "Preserve the human foundation and add a separate result." });

	const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-plan-agent-"));
	await git(tmpdir(), "clone", `http://git:${launched.credential.token}@localhost:3000/git/${repositoryID}.git`, agentCopy);
	await git(agentCopy, "config", "user.name", "Vivarium Plan Agent");
	await git(agentCopy, "config", "user.email", "plan-agent@users.vivarium");
	await git(agentCopy, "switch", "-c", "agent-work", `origin/${launched.run.working_branch}`);
	await writeFile(join(agentCopy, "agent-result.txt"), "guided delegated result\n");
	await writeFile(join(agentCopy, "rollout-state.txt"), "unhealthy\n");
	await git(agentCopy, "add", "agent-result.txt", "rollout-state.txt");
	await git(agentCopy, "commit", "-m", "Add delegated follow-up");
	await git(agentCopy, "push", "origin", `HEAD:refs/heads/${launched.run.working_branch}`);
	const agentCommit = await git(agentCopy, "rev-parse", "HEAD");
	const agentHeaders = { Authorization: `Bearer ${launched.credential.token}` };
	await postJSON(maintainer, `${taskRunBase}/events`, agentHeaders, { kind: "branch.updated", state: "working", message: "Published the guided follow-up.", branch: launched.run.working_branch, commit_id: agentCommit });
	await postJSON(maintainer, `${taskRunBase}/completion`, agentHeaders, {
		summary: "Preserved the human foundation and added the delegated result.", commit_id: agentCommit,
		checks: [{ name: "agent scope", status: "passed", details: "The result is a separate file." }], unresolved_concerns: [],
	});
	const agentPull = await postJSON(maintainer, `/repositories/${repositoryID}/proposals/${proposalID}/tasks/${agentTask.id}/contributions`, maintainerHeaders, {
		title: "Add the delegated follow-up", body: "Carries planning and execution evidence into review.", source_branch: launched.run.working_branch, target_branch: "main", session_id: launched.session.id, run_id: launched.run.id,
	}) as { id: string };

	await contributor.goto(`/pulls/${repositoryID}/${agentPull.id}`);
	await expect(contributor.getByText("Connected proposal task", { exact: true })).toBeVisible();
	await expect(contributor.getByText(new RegExp(`session ${launched.session.id} · run ${launched.run.id}`))).toBeVisible();
	await contributor.getByRole("button", { name: "Approve" }).click();
	await maintainer.goto(`/pulls/${repositoryID}/${agentPull.id}`);
	await expect(maintainer.locator("#checks").getByText("succeeded", { exact: true })).toBeVisible({ timeout: 60_000 });
	await expect(maintainer.getByRole("button", { name: "Merge into main" })).toBeEnabled();
	await maintainer.getByRole("button", { name: "Merge into main" }).click();
	await expect(maintainer.getByText("Merged", { exact: true })).toBeVisible();

	await contributor.goto(`/proposals/${repositoryID}/${proposalID}`);
	await expect(contributor.getByText("merged", { exact: true })).toHaveCount(2);
	await contributor.getByRole("button", { name: "Close proposal" }).click();
	await expect(contributor.getByText("closed", { exact: true }).first()).toBeVisible();
	const historyResponse = await contributor.request.get(`/api/repositories/${repositoryID}/proposals/${proposalID}/tasks/${agentTask.id}/history`, { headers: contributorHeaders });
	const history = await historyResponse.json() as { history: Array<{ action: string; actor_id: string }> };
	expect(history.history).toEqual(expect.arrayContaining([
		expect.objectContaining({ action: "assigned", actor_id: maintainerUser.id }),
		expect.objectContaining({ action: "contribution_published", actor_id: maintainerUser.id }),
		expect.objectContaining({ action: "contribution_merged", actor_id: maintainerUser.id }),
	]));
	const timelineResponse = await contributor.request.get(`/api/repositories/${repositoryID}/proposals/${proposalID}/tasks/${agentTask.id}/sessions/${launched.session.id}/events?limit=100`, { headers: contributorHeaders });
	const timeline = await timelineResponse.json() as { events: Array<{ kind: string; agent_id?: string; initiator_id?: string }> };
	expect(timeline.events).toEqual(expect.arrayContaining([
		expect.objectContaining({ kind: "run.guidance", actor_id: contributorUser.id }),
		expect.objectContaining({ kind: "run.completed", agent_id: assignedAgentTask.assignment.assignee_id, initiator_id: maintainerUser.id }),
	]));
	await git(maintainerCopy, "pull", "--ff-only");
	expect(await readFile(join(maintainerCopy, "foundation.txt"), "utf8")).toBe("human foundation\n");
	expect(await readFile(join(maintainerCopy, "agent-result.txt"), "utf8")).toBe("guided delegated result\n");

	// The same public collaboration boundary now carries both merged authors'
	// work through release, failure, restoration, delegated repair, and delivery.
	type Release = { id: string; commit_id: string; inclusions: { pull_request_ids: string[]; contributor_ids: string[] } };
	type Build = { id: string; state: string; commit_id: string; artifacts: Array<{ id: string; sha256: string }> };
	type Deployment = { id: string; release_id: string; state: string; commit_id: string; artifact_sha256: string; recovery_kind?: string; recovery_of?: string; restores_deployment_id?: string; approvals: Array<{ actor_id: string }>; evidence: Array<{ state: string; signal: string }>; events: Array<{ kind: string; actor_id?: string }> };
	const createRelease = (page: Page, headers: Record<string, string>, version: string, notes: string, commitID: string, previousReleaseID?: string) =>
		postJSON(page, `/repositories/${repositoryID}/releases`, headers, { version, notes, commit_id: commitID, previous_release_id: previousReleaseID }) as Promise<Release>;
	const buildRelease = async (page: Page, headers: Record<string, string>, release: Release) => {
		await postJSON(page, `/repositories/${repositoryID}/releases/${release.id}/builds`, headers, {});
		const result = await eventually(
			() => getJSON(page, `/repositories/${repositoryID}/releases/${release.id}/builds`, headers) as Promise<{ builds: Build[] }>,
			(value) => value.builds.length > 0 && value.builds.every((build) => build.state === "succeeded"),
			`release ${release.id} builds become verified`,
		);
		return result.builds[0];
	};
	const listDeployments = (page: Page, headers: Record<string, string>) =>
		getJSON(page, `/repositories/${repositoryID}/deployments`, headers) as Promise<{ deployments: Deployment[] }>;
	const waitForDeployment = async (page: Page, headers: Record<string, string>, id: string, state: string) => {
		const result = await eventually(
			() => listDeployments(page, headers),
			(value) => value.deployments.some((deployment) => deployment.id === id && (deployment.state === state || ["failed", "canceled"].includes(deployment.state))),
			`deployment ${id} becomes ${state}`,
		);
		const deployment = result.deployments.find((item) => item.id === id)!;
		expect(deployment.state, JSON.stringify(deployment.events, null, 2)).toBe(state);
		return deployment;
	};

	const baselineRelease = await createRelease(maintainer, maintainerHeaders, `v1.0.0-${suffix}`, "Known-good service before the coordinated work.", baseCommit);
	const baselineBuild = await buildRelease(maintainer, maintainerHeaders, baselineRelease);
	const environment = await postJSON(maintainer, `/repositories/${repositoryID}/environments`, maintainerHeaders, {
		name: "production", position: 1, image: "alpine:3.22", command: "test -r \"$VIVARIUM_ARTIFACT\"", timeout_seconds: 30,
		required_approvals: 1, concurrency: 1, configuration: { REGION: "test" }, credentials: { DEPLOY_TOKEN: `secret-${suffix}` },
	}) as { id: string; credential_names: string[] };
	expect(environment.credential_names).toEqual(["DEPLOY_TOKEN"]);
	const baselineDeployment = await postJSON(maintainer, `/repositories/${repositoryID}/deployments`, maintainerHeaders, {
		environment_id: environment.id, release_id: baselineRelease.id, build_id: baselineBuild.id, artifact_id: baselineBuild.artifacts[0].id,
	}) as Deployment;
	expect(baselineDeployment.state).toBe("pending_approval");
	await postJSON(contributor, `/repositories/${repositoryID}/deployments/${baselineDeployment.id}/approvals`, contributorHeaders, {});
	const knownGood = await waitForDeployment(maintainer, maintainerHeaders, baselineDeployment.id, "succeeded");
	expect(knownGood.approvals).toEqual([expect.objectContaining({ actor_id: contributorUser.id })]);
	expect(knownGood.evidence).toEqual([expect.objectContaining({ state: "passed", signal: "service responds" })]);

	const deliveredCommit = await git(maintainerCopy, "rev-parse", "HEAD");
	const unhealthyRelease = await createRelease(contributor, contributorHeaders, `v1.1.0-${suffix}`, "Deliver the merged human foundation and delegated follow-up.", deliveredCommit, baselineRelease.id);
	expect(unhealthyRelease.inclusions.pull_request_ids).toEqual(expect.arrayContaining([humanPull.id, agentPull.id]));
	expect(unhealthyRelease.inclusions.contributor_ids).toEqual(expect.arrayContaining([maintainerUser.id, contributorUser.id]));
	const unhealthyBuild = await buildRelease(contributor, contributorHeaders, unhealthyRelease);
	const failedRequest = await postJSON(contributor, `/repositories/${repositoryID}/deployments`, contributorHeaders, {
		environment_id: environment.id, release_id: unhealthyRelease.id, build_id: unhealthyBuild.id, artifact_id: unhealthyBuild.artifacts[0].id,
	}) as Deployment;
	await postJSON(maintainer, `/repositories/${repositoryID}/deployments/${failedRequest.id}/approvals`, maintainerHeaders, {});
	const failed = await waitForDeployment(contributor, contributorHeaders, failedRequest.id, "failed");
	expect(failed.evidence).toEqual([expect.objectContaining({ state: "failed", signal: "service responds" })]);
	expect(failed.events).toEqual(expect.arrayContaining([
		expect.objectContaining({ kind: "promotion.requested", actor_id: contributorUser.id }),
		expect.objectContaining({ kind: "promotion.approved", actor_id: maintainerUser.id }),
		expect.objectContaining({ kind: "deployment.failed" }),
	]));

	// Declare the retained failed signal through the browser, then use only the
	// public incident and deployment APIs for the agent and governed response.
	await contributor.goto("/incidents");
	await contributor.getByRole("button", { name: "Declare incident" }).click();
	await contributor.getByLabel("Title").fill("Production service health regression");
	await contributor.getByLabel("Current impact").fill("The newly promoted service artifact fails its production health contract.");
	await contributor.getByLabel("Severity").selectOption("sev1");
	await contributor.getByLabel("Deployment health signal").selectOption(`${repositoryID}:${failed.id}`);
	await contributor.getByRole("button", { name: "Declare and take command" }).click();
	await expect(contributor).toHaveURL(/\/incidents\/[a-f0-9]{32}$/);
	const incidentID = new URL(contributor.url()).pathname.split("/").pop()!;
	await expect(contributor.getByText(`Declared from verified signal production health/service responds`)).toBeVisible();

	type IncidentEvidence = { kind: string; repository_id: string; resource_id: string; label: string; query?: string; window_start?: string; window_end?: string; captured_at: string };
	type Incident = {
		id: string; version: number; status: string;
		timeline: Array<{ id: string; kind: string; actor_id: string; message: string; audience: string; acknowledged_by?: string[]; evidence?: IncidentEvidence[] }>;
		investigations: Array<{ id: string; agent_id: string; state: string }>;
		actions: Array<{ id: string; status: string; attempts: Array<{ outcome: string; resource_id?: string }> }>;
		commitments: Array<{ proposal_id: string; task_id: string; progress: { state: string; pull_request_id?: string; release_ids?: string[]; deployment_ids?: string[] } }>;
	};
	const windowStart = new Date(Date.now() - 10 * 60_000).toISOString();
	const windowEnd = new Date(Date.now() + 60_000).toISOString();
	let incident = await postJSON(contributor, `/incidents/${incidentID}/findings`, contributorHeaders, {
		operation_id: crypto.randomUUID().replaceAll("-", ""), kind: "hypothesis",
		message: "The failed production signal begins with the unhealthy artifact revision.", audience: "participants",
		evidence: [{ kind: "health_signal", repository_id: repositoryID, resource_id: failed.id, query: "production health/service responds", window_start: windowStart, window_end: windowEnd }],
	}) as Incident;
	const diagnosticEntry = incident.timeline.at(-1)!;
	expect(diagnosticEntry.evidence).toEqual([expect.objectContaining({ kind: "health_signal", resource_id: failed.id })]);

	const investigationLaunch = await postJSON(contributor, `/incidents/${incidentID}/investigations`, contributorHeaders, {
		mandate: "Determine whether the failed signal is consistent with the promoted revision and report uncertainty.",
		evidence: diagnosticEntry.evidence, revisions: [{ repository_id: repositoryID, commit_id: deliveredCommit }], expires_in: 3600,
	}) as { incident: Incident; investigation: { id: string; agent_id: string; access: string[] }; credential: { token: string; scopes: string[] } };
	expect(investigationLaunch.credential.scopes).toEqual(["incidents:investigate"]);
	expect(investigationLaunch.investigation.access).toEqual([
		"selected incident evidence", "selected repository revisions", "incident investigation timeline:write",
	]);
	const investigationHeaders = { Authorization: `Bearer ${investigationLaunch.credential.token}` };
	const forbiddenProduction = await contributor.request.post(`/api/repositories/${repositoryID}/deployments/${failed.id}/recoveries`, {
		headers: investigationHeaders, data: { action: "rollback" },
	});
	expect(forbiddenProduction.status()).toBe(401);
	await postJSON(contributor, `/incidents/${incidentID}/investigations/${investigationLaunch.investigation.id}/events`, investigationHeaders, {
		kind: "tool_action", tool: "health_signal.inspect", message: "Compared the frozen signal window with the exact promoted revision.",
	});
	incident = await postJSON(contributor, `/incidents/${incidentID}/investigations/${investigationLaunch.investigation.id}/events`, investigationHeaders, {
		kind: "finding", message: "The unhealthy artifact deterministically fails the declared service response contract.",
	}) as Incident;
	expect(incident.timeline).toEqual(expect.arrayContaining([
		expect.objectContaining({ kind: "agent_finding", actor_id: investigationLaunch.investigation.agent_id }),
	]));

	incident = await postJSON(contributor, `/incidents/${incidentID}/actions`, contributorHeaders, {
		operation_id: crypto.randomUUID().replaceAll("-", ""), kind: "restore_release", repository_id: repositoryID,
		deployment_id: failed.id, rationale: "Restore the last attested healthy artifact while corrective work is reviewed.",
		evidence: diagnosticEntry.evidence, health_criteria: [{ stage: "production health", signal: "service responds" }],
	}) as Incident;
	const mitigation = incident.actions.at(-1)!;
	incident = await postJSON(maintainer, `/incidents/${incidentID}/actions/${mitigation.id}/decisions`, maintainerHeaders, {
		decision: "approve", message: "The exact failed signal and known-good artifact support a bounded rollback.",
	}) as Incident;
	expect(incident.actions.at(-1)!.status).toBe("approved");
	const mitigationOperation = crypto.randomUUID().replaceAll("-", "");
	await postJSON(contributor, `/incidents/${incidentID}/actions/${mitigation.id}/attempts`, contributorHeaders, {
		operation_id: mitigationOperation, outcome: "pending", message: "Governed rollback reserved before environment mutation.",
	});
	const rollbackResult = await postJSON(contributor, `/repositories/${repositoryID}/deployments/${failed.id}/recoveries`, contributorHeaders, { action: "rollback" }) as { deployment: Deployment };
	await postJSON(contributor, `/incidents/${incidentID}/actions/${mitigation.id}/attempts`, contributorHeaders, {
		operation_id: mitigationOperation, outcome: "started", resource_id: rollbackResult.deployment.id, message: "Approval-gated rollback requested.",
	});

	await contributor.goto(`/repositories/${repositoryID}/releases/${unhealthyRelease.id}`);
	await expect(contributor.getByRole("heading", { name: `v1.1.0-${suffix}` })).toBeVisible();
	await expect(contributor.getByLabel("Health evidence").getByText("failed", { exact: true })).toBeVisible();
	const rollbackSet = await listDeployments(contributor, contributorHeaders);
	const rollback = rollbackSet.deployments.find((deployment) => deployment.recovery_of === failed.id)!;
	expect(rollback).toMatchObject({ recovery_kind: "rollback", restores_deployment_id: knownGood.id, state: "pending_approval" });
	await postJSON(maintainer, `/repositories/${repositoryID}/deployments/${rollback.id}/approvals`, maintainerHeaders, {});
	const restored = await waitForDeployment(contributor, contributorHeaders, rollback.id, "succeeded");
	expect(restored.artifact_sha256).toBe(knownGood.artifact_sha256);
	incident = await postJSON(contributor, `/incidents/${incidentID}/actions/${mitigation.id}/attempts`, contributorHeaders, {
		operation_id: crypto.randomUUID().replaceAll("-", ""), outcome: "recovered", resource_id: restored.id,
		message: "The restored deployment passed the incident's declared production health criterion.",
	}) as Incident;
	expect(incident.actions.at(-1)!.status).toBe("recovered");
	incident = await postJSON(contributor, `/incidents/${incidentID}/updates`, contributorHeaders, {
		operation_id: crypto.randomUUID().replaceAll("-", ""), message: "Service health is restored; corrective work will proceed through normal review.", audience: "public",
	}) as Incident;
	const recoveryUpdate = incident.timeline.at(-1)!;
	incident = await postJSON(maintainer, `/incidents/${incidentID}/timeline/${recoveryUpdate.id}/acknowledgements`, maintainerHeaders, {}) as Incident;
	expect(incident.timeline.at(-1)!.acknowledged_by).toContain(maintainerUser.id);

	await contributor.reload();
	const repairResponsePromise = contributor.waitForResponse((response) => response.request().method() === "POST" && response.url().endsWith(`/deployments/${failed.id}/recoveries`));
	await contributor.getByRole("button", { name: "Open repair session" }).click();
	const repairResponse = await repairResponsePromise;
	expect(repairResponse.status()).toBe(201);
	await expect(contributor).toHaveURL(new RegExp(`/pulls/${repositoryID}/[a-f0-9]{32}/sessions/[a-f0-9]{32}$`));
	const repairParts = new URL(contributor.url()).pathname.split("/");
	const repairPullID = repairParts[3], repairSessionID = repairParts[5];
	const repair = {
		pull_request: await getJSON(contributor, `/repositories/${repositoryID}/pulls/${repairPullID}`, contributorHeaders) as { id: string; source_branch: string; source_commit_id: string },
		session: await getJSON(contributor, `/repositories/${repositoryID}/pulls/${repairPullID}/sessions/${repairSessionID}`, contributorHeaders) as { id: string; source_commit_id: string; deployment_evidence: { deployment_id: string; release_id: string; artifact_sha256: string; state: string; evidence: Array<{ state: string }>; events: Array<{ kind: string }> } },
	};
	expect(repair.session.deployment_evidence).toMatchObject({ deployment_id: failed.id, release_id: unhealthyRelease.id, artifact_sha256: unhealthyBuild.artifacts[0].sha256, state: "failed" });
	expect(repair.session.deployment_evidence.evidence).toEqual([expect.objectContaining({ state: "failed" })]);

	const repairRun = await postJSON(contributor, `/repositories/${repositoryID}/pulls/${repair.pull_request.id}/sessions/${repair.session.id}/runs`, contributorHeaders, {
		instructions: "Restore the service health contract using the attached failed rollout evidence.", source_commit_id: repair.session.source_commit_id,
		context_paths: ["rollout-state.txt", ".vivarium/deployment.json"], working_branch: repair.pull_request.source_branch, expires_in: 3600,
	}) as { run: { id: string }; credential: { token: string } };
	const recoveryCopy = await mkdtemp(join(tmpdir(), "vivarium-delivery-repair-"));
	await git(tmpdir(), "clone", `http://git:${repairRun.credential.token}@localhost:3000/git/${repositoryID}.git`, recoveryCopy);
	await git(recoveryCopy, "config", "user.name", "Vivarium Recovery Agent");
	await git(recoveryCopy, "config", "user.email", "recovery-agent@users.vivarium");
	await git(recoveryCopy, "switch", "-c", "repair-work", `origin/${repair.pull_request.source_branch}`);
	await writeFile(join(recoveryCopy, "rollout-state.txt"), "healthy\n");
	await git(recoveryCopy, "add", "rollout-state.txt");
	await git(recoveryCopy, "commit", "-m", "Restore service health contract");
	await git(recoveryCopy, "push", "origin", `HEAD:refs/heads/${repair.pull_request.source_branch}`);
	const repairCommit = await git(recoveryCopy, "rev-parse", "HEAD");
	const repairAgentHeaders = { Authorization: `Bearer ${repairRun.credential.token}` };
	const repairRunBase = `/repositories/${repositoryID}/pulls/${repair.pull_request.id}/sessions/${repair.session.id}/runs/${repairRun.run.id}`;
	await postJSON(contributor, `${repairRunBase}/events`, repairAgentHeaders, { kind: "branch.updated", state: "working", message: "Published the evidence-driven health repair.", branch: repair.pull_request.source_branch, commit_id: repairCommit });
	await postJSON(contributor, `${repairRunBase}/completion`, repairAgentHeaders, {
		summary: "Restored the health contract identified by the failed production signal.", commit_id: repairCommit,
		checks: [{ name: "rollout contract", status: "passed", details: "The artifact again reports healthy." }], unresolved_concerns: [],
	});

	await maintainer.goto(`/pulls/${repositoryID}/${repair.pull_request.id}`);
	await expect(maintainer.locator("#checks").getByText("succeeded", { exact: true }).last()).toBeVisible({ timeout: 60_000 });
	await maintainer.getByRole("button", { name: "Approve" }).click();
	await expect(maintainer.getByRole("button", { name: "Merge into main" })).toBeEnabled();
	await maintainer.getByRole("button", { name: "Merge into main" }).click();
	await expect(maintainer.getByText("Merged", { exact: true })).toBeVisible();
	const repairedPull = await getJSON(maintainer, `/repositories/${repositoryID}/pulls/${repair.pull_request.id}`, maintainerHeaders) as { merge_commit_id: string; author_id: string; merged_by: string };
	expect(repairedPull).toMatchObject({ author_id: contributorUser.id, merged_by: maintainerUser.id });

	const correctedRelease = await createRelease(maintainer, maintainerHeaders, `v1.1.1-${suffix}`, "Repair the failed rollout using its retained evidence.", repairedPull.merge_commit_id, unhealthyRelease.id);
	expect(correctedRelease.inclusions.pull_request_ids).toContain(repair.pull_request.id);
	const correctedBuild = await buildRelease(maintainer, maintainerHeaders, correctedRelease);
	const correctedRequest = await postJSON(maintainer, `/repositories/${repositoryID}/deployments`, maintainerHeaders, {
		environment_id: environment.id, release_id: correctedRelease.id, build_id: correctedBuild.id, artifact_id: correctedBuild.artifacts[0].id,
	}) as Deployment;
	await postJSON(contributor, `/repositories/${repositoryID}/deployments/${correctedRequest.id}/approvals`, contributorHeaders, {});
	const corrected = await waitForDeployment(maintainer, maintainerHeaders, correctedRequest.id, "succeeded");
	expect(corrected).toMatchObject({ commit_id: repairedPull.merge_commit_id, artifact_sha256: correctedBuild.artifacts[0].sha256 });
	expect(corrected.events).toEqual(expect.arrayContaining([
		expect.objectContaining({ kind: "promotion.requested", actor_id: maintainerUser.id }),
		expect.objectContaining({ kind: "promotion.approved", actor_id: contributorUser.id }),
		expect.objectContaining({ kind: "deployment.succeeded" }),
	]));
	await maintainer.goto(`/repositories/${repositoryID}/releases/${correctedRelease.id}`);
	await expect(maintainer.getByRole("heading", { name: `v1.1.1-${suffix}` })).toBeVisible();
	await expect(maintainer.getByLabel("Health evidence").getByText("passed", { exact: true })).toBeVisible();

	// Publish the review in the incident workspace, then carry one accountable
	// corrective task through the ordinary proposal, pull, release, and
	// production paths. Incident reads must derive every later projection.
	await contributor.goto(`/incidents/${incidentID}`);
	await contributor.getByPlaceholder("Who or what was affected, how severely, and for how long?").fill("The unhealthy release failed its production health signal; rollback restored service within the response window.");
	await contributor.getByPlaceholder("Reviewable sequence of detection, decisions, mitigation, and recovery").fill("Signal failed; incident declared; bounded agent investigation confirmed the contract failure; independent approval authorized rollback; health recovered.");
	await contributor.getByPlaceholder("One contributing factor per line").fill("The release check did not exercise the packaged health state\nThe production signal was the first end-to-end contract check");
	await contributor.getByPlaceholder("What did the team learn, including uncertainty?").fill("Verify the packaged rollout state before promotion while retaining production health as the authoritative recovery criterion.");
	await contributor.getByRole("button", { name: "Resolve and publish review" }).click();
	await expect(contributor.getByText("The unhealthy release failed its production health signal; rollback restored service within the response window.")).toBeVisible();

	await git(maintainerCopy, "pull", "--ff-only");
	const correctiveBase = await git(maintainerCopy, "rev-parse", "HEAD");
	incident = await getJSON(contributor, `/incidents/${incidentID}`, contributorHeaders) as Incident;
	expect(incident.status).toBe("resolved");
	const commitmentResult = await postJSON(contributor, `/incidents/${incidentID}/commitments`, contributorHeaders, {
		operation_id: crypto.randomUUID().replaceAll("-", ""), repository_id: repositoryID,
		proposal_title: "Prevent unhealthy artifact promotion", proposal_body: "Carry the incident's health-contract learning into a reviewed pre-promotion guard.",
		task_title: "Add packaged health regression coverage", outcome: "Repository verification documents and checks the packaged health invariant.",
		assignee_id: contributorUser.id, base_revision: correctiveBase, due_at: new Date(Date.now() + 24 * 60 * 60_000).toISOString(),
	}) as Incident;
	const commitment = commitmentResult.commitments.at(-1)!;
	expect(commitment.progress.state).toBe("assigned");
	await contributor.reload();
	await expect(contributor.getByRole("link", { name: "Corrective proposal →" })).toBeVisible();
	await expect(contributor.getByText("assigned", { exact: true })).toBeVisible();

	await git(contributorCopy, "fetch", "origin", "main");
	await git(contributorCopy, "switch", "-C", "incident-health-regression", "origin/main");
	await writeFile(join(contributorCopy, "incident-regression.md"), "Release packages must preserve the healthy rollout-state contract before promotion.\n");
	await git(contributorCopy, "add", "incident-regression.md");
	await git(contributorCopy, "commit", "-m", "Document incident health regression");
	await git(contributorCopy, "push", "origin", "incident-health-regression");
	const correctivePull = await postJSON(contributor, `/repositories/${repositoryID}/proposals/${commitment.proposal_id}/tasks/${commitment.task_id}/contributions`, contributorHeaders, {
		title: "Add packaged health regression coverage", body: "Implements the incident review's corrective commitment.", source_branch: "incident-health-regression", target_branch: "main",
	}) as { id: string };
	await maintainer.goto(`/pulls/${repositoryID}/${correctivePull.id}`);
	await expect(maintainer.locator("#checks").getByText("succeeded", { exact: true }).last()).toBeVisible({ timeout: 60_000 });
	await maintainer.getByRole("button", { name: "Approve" }).click();
	await expect(maintainer.getByRole("button", { name: "Merge into main" })).toBeEnabled();
	await maintainer.getByRole("button", { name: "Merge into main" }).click();
	await expect(maintainer.getByText("Merged", { exact: true })).toBeVisible();
	const correctivePullResult = await getJSON(maintainer, `/repositories/${repositoryID}/pulls/${correctivePull.id}`, maintainerHeaders) as { merge_commit_id: string };

	const followUpRelease = await createRelease(contributor, contributorHeaders, `v1.1.2-${suffix}`, "Deliver the incident-linked preventive improvement.", correctivePullResult.merge_commit_id, correctedRelease.id);
	const followUpBuild = await buildRelease(contributor, contributorHeaders, followUpRelease);
	const followUpRequest = await postJSON(contributor, `/repositories/${repositoryID}/deployments`, contributorHeaders, {
		environment_id: environment.id, release_id: followUpRelease.id, build_id: followUpBuild.id, artifact_id: followUpBuild.artifacts[0].id,
	}) as Deployment;
	await postJSON(maintainer, `/repositories/${repositoryID}/deployments/${followUpRequest.id}/approvals`, maintainerHeaders, {});
	await waitForDeployment(contributor, contributorHeaders, followUpRequest.id, "succeeded");
	incident = await getJSON(maintainer, `/incidents/${incidentID}`, maintainerHeaders) as Incident;
	const completed = incident.commitments.find((item) => item.task_id === commitment.task_id)!;
	expect(completed.progress).toMatchObject({ state: "completed", pull_request_id: correctivePull.id });
	expect(completed.progress.release_ids).toContain(followUpRelease.id);
	expect(completed.progress.deployment_ids).toContain(followUpRequest.id);
	expect(incident.timeline).toEqual(expect.arrayContaining([
		expect.objectContaining({ kind: "declared", actor_id: contributorUser.id }),
		expect.objectContaining({ kind: "agent_finding", actor_id: investigationLaunch.investigation.agent_id }),
		expect.objectContaining({ kind: "mitigation_recovered", actor_id: contributorUser.id }),
		expect.objectContaining({ kind: "incident_resolved", actor_id: contributorUser.id }),
	]));

  await maintainerContext.close();
  await contributorContext.close();
});
