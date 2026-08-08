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

test("humans and an agent verify, repair, and merge one attributed change", async ({ browser }) => {
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
  await maintainer.getByLabel("Required check names").fill("greeting verification");
  const requirementSaved = maintainer.waitForResponse((response) =>
    response.request().method() === "PUT" && response.url().includes(`/repositories/${repositoryID}/branches/main/required-checks`),
  );
  await maintainer.getByRole("button", { name: "Save requirements" }).click();
  expect((await requirementSaved).status()).toBe(200);

  await createAccount(newcomer, "Journey Newcomer", `newcomer-${suffix}`);
  const newcomerToken = await issueGitToken(newcomer, "Journey Git");
  const collaborationID = (await newcomer.getByTestId("collaboration-id").textContent())!;
  await maintainer.goto(`/repositories/${repositoryID}`);
  await maintainer.getByLabel("Collaboration ID").fill(collaborationID);
  await maintainer.getByRole("button", { name: "Add", exact: true }).click();
  await expect(maintainer.getByText(`@newcomer-${suffix}`)).toBeVisible();

  await newcomer.goto("/proposals");
  await newcomer.getByRole("button", { name: "New proposal" }).click();
  await newcomer.getByLabel("Title").fill("Add a greeting");
  await newcomer.getByLabel("Context", { exact: true }).fill("Welcome developers and agents.");
  await newcomer.getByRole("button", { name: "Publish proposal" }).click();
  await expect(newcomer).toHaveURL(new RegExp(`/proposals/${repositoryID}/[a-f0-9]{32}$`));
  const proposalID = new URL(newcomer.url()).pathname.split("/").pop()!;
  await expect(newcomer.getByText(`@newcomer-${suffix}`, { exact: true }).first()).toBeVisible();

  await maintainer.goto(newcomer.url());
  await maintainer.getByLabel("Add to the conversation").fill("Please include a friendly example.");
  await maintainer.getByRole("button", { name: "Comment" }).click();
  await expect(maintainer.getByText("Please include a friendly example.")).toBeVisible();

  const newcomerCopy = await mkdtemp(join(tmpdir(), "vivarium-newcomer-"));
  await git(tmpdir(), "clone", `http://git:${newcomerToken}@localhost:3000/git/${repositoryID}.git`, newcomerCopy);
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
  await newcomer.getByLabel("Linked proposal").selectOption(proposalID);
  await newcomer.getByLabel("Title").fill("Add a greeting");
  await newcomer.getByLabel("Purpose and feedback needed").fill("Implements the agreed welcome.");
  await newcomer.getByRole("button", { name: "Open pull request" }).click();
  await expect(newcomer).toHaveURL(new RegExp(`/pulls/${repositoryID}/[a-f0-9]{32}$`));
  const pullRequestID = new URL(newcomer.url()).pathname.split("/").pop()!;
  await expect(newcomer.getByText(`@newcomer-${suffix}`, { exact: true }).first()).toBeVisible();

  await maintainer.goto(newcomer.url());
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
  const agentRemote = `http://git:${launched.credential.token}@localhost:3000/git/${repositoryID}.git`;
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
  await newcomer.goto(`/proposals/${repositoryID}/${proposalID}`);
  await expect(newcomer.getByText("closed", { exact: true }).first()).toBeVisible();

  await newcomer.goto("/inbox");
  const mergeItem = newcomer.getByRole("article").filter({ hasText: "Review merge outcome" });
  await expect(mergeItem).toBeVisible();
  await expect(mergeItem.getByText("awareness", { exact: true })).toBeVisible();
  await mergeItem.getByRole("button", { name: "Clear Add a greeting" }).click();
  await expect(mergeItem).toBeHidden();

  await maintainerContext.close();
  await newcomerContext.close();
});
