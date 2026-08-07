import { expect, test, type Page } from "@playwright/test";
import { mkdtemp, readFile, writeFile } from "node:fs/promises";
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

test("two users carry one attributed change from onboarding through merge", async ({ browser }) => {
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
  await git(maintainerCopy, "add", "README.md");
  await git(maintainerCopy, "commit", "-m", "Start project");
  await git(maintainerCopy, "push", "origin", "main");

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
  await expect(newcomer.getByText(`@newcomer-${suffix}`, { exact: true }).first()).toBeVisible();

  await maintainer.goto(newcomer.url());
  await maintainer.getByRole("button", { name: "Approve" }).click();
  await expect(maintainer.getByText(`@maintainer-${suffix}`, { exact: true }).first()).toBeVisible();
  await expect(maintainer.getByRole("button", { name: "Merge into main" })).toBeEnabled();
  await maintainer.getByRole("button", { name: "Merge into main" }).click();
  await expect(maintainer.getByText("Merged", { exact: true })).toBeVisible();
  await expect(maintainer.getByText(`@maintainer-${suffix}`, { exact: true }).first()).toBeVisible();

  await git(maintainerCopy, "pull", "--ff-only");
  expect(await readFile(join(maintainerCopy, "greeting.txt"), "utf8")).toBe("hello developers and agents\n");
  await newcomer.goto(`/proposals/${repositoryID}/${proposalID}`);
  await expect(newcomer.getByText("closed", { exact: true }).first()).toBeVisible();

  await maintainerContext.close();
  await newcomerContext.close();
});
