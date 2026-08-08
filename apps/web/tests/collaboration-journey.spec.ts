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

test("a linked proposal closes into a clearable merge outcome", async ({ browser }) => {
  const suffix = Date.now().toString(36);
  const maintainerContext = await browser.newContext();
  const contributorContext = await browser.newContext();
  const maintainer = await maintainerContext.newPage();
  const contributor = await contributorContext.newPage();

  await createAccount(maintainer, "Proposal Maintainer", `proposal-maintainer-${suffix}`);
  const maintainerToken = await issueGitToken(maintainer, "Proposal Git");
  await maintainer.goto("/repositories");
  await maintainer.getByLabel("Repository name").fill(`proposal-${suffix}`);
  await maintainer.getByRole("button", { name: "Create repository" }).click();
  await maintainer.getByRole("link", { name: new RegExp(`proposal-${suffix}`) }).click();
  await expect(maintainer).toHaveURL(/\/repositories\/[a-f0-9]{32}$/);
  const repositoryID = new URL(maintainer.url()).pathname.split("/").pop()!;

  const maintainerCopy = await mkdtemp(join(tmpdir(), "vivarium-proposal-maintainer-"));
  await git(tmpdir(), "clone", `http://git:${maintainerToken}@localhost:3000/git/${repositoryID}.git`, maintainerCopy);
  await git(maintainerCopy, "config", "user.name", "Proposal Maintainer");
  await git(maintainerCopy, "config", "user.email", "proposal-maintainer@example.com");
  await writeFile(join(maintainerCopy, "README.md"), "# Proposal workflow\n");
  await git(maintainerCopy, "add", "README.md");
  await git(maintainerCopy, "commit", "-m", "Start proposal project");
  await git(maintainerCopy, "push", "origin", "main");

  await createAccount(contributor, "Proposal Contributor", `proposal-contributor-${suffix}`);
  const contributorToken = await issueGitToken(contributor, "Proposal Git");
  const collaborationID = (await contributor.getByTestId("collaboration-id").textContent())!;
  await maintainer.goto(`/repositories/${repositoryID}`);
  await maintainer.getByLabel("Collaboration ID").fill(collaborationID);
  await maintainer.getByRole("button", { name: "Add", exact: true }).click();
  await expect(maintainer.getByText(`@proposal-contributor-${suffix}`)).toBeVisible();

  await contributor.goto("/proposals");
  await contributor.getByRole("button", { name: "New proposal" }).click();
  await contributor.getByLabel("Title").fill("Document the proposal outcome");
  await contributor.getByLabel("Context", { exact: true }).fill("Keep the discussion linked through merge.");
  await contributor.getByRole("button", { name: "Publish proposal" }).click();
  await expect(contributor).toHaveURL(new RegExp(`/proposals/${repositoryID}/[a-f0-9]{32}$`));
  const proposalID = new URL(contributor.url()).pathname.split("/").pop()!;

  const contributorCopy = await mkdtemp(join(tmpdir(), "vivarium-proposal-contributor-"));
  await git(tmpdir(), "clone", `http://git:${contributorToken}@localhost:3000/git/${repositoryID}.git`, contributorCopy);
  await git(contributorCopy, "config", "user.name", "Proposal Contributor");
  await git(contributorCopy, "config", "user.email", "proposal-contributor@example.com");
  await git(contributorCopy, "switch", "-c", "proposal-outcome");
  await writeFile(join(contributorCopy, "OUTCOME.md"), "The proposal became a merged change.\n");
  await git(contributorCopy, "add", "OUTCOME.md");
  await git(contributorCopy, "commit", "-m", "Document proposal outcome");
  await git(contributorCopy, "push", "origin", "proposal-outcome");

  await contributor.goto("/pulls");
  await contributor.getByRole("button", { name: "New pull request" }).click();
  await contributor.getByLabel("Candidate branch").selectOption("proposal-outcome");
  await contributor.getByLabel("Target branch").selectOption("main");
  await contributor.getByLabel("Linked proposal").selectOption(proposalID);
  await contributor.getByLabel("Title").fill("Document the proposal outcome");
  await contributor.getByLabel("Purpose and feedback needed").fill("Carries the proposal into review.");
  await contributor.getByRole("button", { name: "Open pull request" }).click();
  await expect(contributor).toHaveURL(new RegExp(`/pulls/${repositoryID}/[a-f0-9]{32}$`));
  const pullRequestURL = contributor.url();

  await maintainer.goto(pullRequestURL);
  await maintainer.getByRole("button", { name: "Approve" }).click();
  await expect(maintainer.getByRole("button", { name: "Merge into main" })).toBeEnabled();
  await maintainer.getByRole("button", { name: "Merge into main" }).click();
  await expect(maintainer.getByText("Merged", { exact: true })).toBeVisible();

  await contributor.goto(`/proposals/${repositoryID}/${proposalID}`);
  await expect(contributor.getByText("closed", { exact: true }).first()).toBeVisible();
  await contributor.goto("/inbox");
  const mergeItem = contributor.getByRole("article").filter({ hasText: "Review merge outcome" });
  await expect(mergeItem).toBeVisible();
  await expect(mergeItem.getByText("awareness", { exact: true })).toBeVisible();
  await mergeItem.getByRole("button", { name: "Clear Document the proposal outcome" }).click();
  await expect(mergeItem).toBeHidden();

  await maintainerContext.close();
  await contributorContext.close();
});
