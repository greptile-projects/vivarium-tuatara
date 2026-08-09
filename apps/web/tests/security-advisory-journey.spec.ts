import {
  expect,
  test,
  type APIRequestContext,
  type BrowserContext,
} from "@playwright/test";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const apiOrigin = "http://127.0.0.1:8080";
const run = promisify(execFile);

type Account = { user: { id: string }; credential: { token: string } };
type Advisory = {
  id: string;
  version: number;
  evidence: Array<{ id: string }>;
  findings: Array<{ investigation_id?: string; actor_id: string }>;
  repair_tasks: Array<{ id: string }>;
  repair_sessions: Array<{ id: string }>;
  repair_verifications: Array<{ id: string }>;
  disclosure?: {
    state: string;
    remaining: string[];
    fixed_versions: Array<{ branch: string; sha256: string[] }>;
  };
};

async function json<T>(response: {
  ok(): boolean;
  status(): number;
  text(): Promise<string>;
  json(): Promise<unknown>;
}) {
  const body = await response.text();
  expect(response.ok(), body).toBeTruthy();
  return JSON.parse(body) as T;
}

async function account(
  request: APIRequestContext,
  name: string,
  handle: string,
) {
  return json<Account>(
    await request.post(`${apiOrigin}/users`, {
      data: { display_name: name, handle },
    }),
  );
}

const auth = (actor: Account) => ({
  Authorization: `Bearer ${actor.credential.token}`,
});

async function git(cwd: string, ...args: string[]) {
  return (
    await run("git", args, {
      cwd,
      env: { ...process.env, GIT_TERMINAL_PROMPT: "0" },
    })
  ).stdout.trim();
}

async function signIn(context: BrowserContext, token: string) {
  await context.addInitScript(
    (value) => localStorage.setItem("vivarium.access-token", value),
    token,
  );
}

test("a confidential report reaches verified public fixes without leaking its embargo", async ({
  browser,
  request,
}) => {
  test.setTimeout(240_000);
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() =>
    run("docker", ["pull", "alpine:3.22"]),
  );
  const suffix = Date.now().toString(36);
  const owner = await account(
    request,
    "Security Maintainer",
    `security-owner-${suffix}`,
  );
  const reporter = await account(
    request,
    "External Researcher",
    `security-reporter-${suffix}`,
  );
  const worker = await account(
    request,
    "Repair Contributor",
    `security-worker-${suffix}`,
  );

  const repository = await json<{ id: string }>(
    await request.post(`${apiOrigin}/repositories`, {
      headers: auth(owner),
      data: { name: `parser-${suffix}` },
    }),
  );
  await json(
    await request.patch(`${apiOrigin}/repositories/${repository.id}`, {
      headers: auth(owner),
      data: { visibility: "public" },
    }),
  );
  await json(
    await request.post(
      `${apiOrigin}/repositories/${repository.id}/collaborators`,
      {
        headers: auth(owner),
        data: { user_id: worker.user.id },
      },
    ),
  );
  const gitCredential = await json<{ token: string }>(
    await request.post(`${apiOrigin}/auth/credentials`, {
      headers: auth(owner),
      data: {
        kind: "git",
        name: "Security journey Git",
        scopes: ["git:read", "git:write"],
        expires_in: 3600,
      },
    }),
  );
  const copy = await mkdtemp(join(tmpdir(), "vivarium-security-journey-"));
  const remote = `http://git:${gitCredential.token}@127.0.0.1:8080/git/${repository.id}.git`;
  await git(tmpdir(), "clone", remote, copy);
  await git(copy, "config", "user.name", "Security Maintainer");
  await git(copy, "config", "user.email", "maintainer@example.test");
  await mkdir(join(copy, ".vivarium"));
  await writeFile(
    join(copy, ".vivarium", "checks.json"),
    JSON.stringify({
      version: 1,
      checks: [
        {
          name: "security quality",
          image: "alpine:3.22",
          command: "test -f parser.txt && grep -qx fixed parser.txt",
        },
      ],
    }),
  );
  await writeFile(
    join(copy, ".vivarium", "release.json"),
    JSON.stringify({
      version: 1,
      steps: [
        {
          name: "package parser",
          image: "alpine:3.22",
          command: 'cp parser.txt "$VIVARIUM_OUTPUT/parser.txt"',
        },
      ],
    }),
  );
  await writeFile(join(copy, "parser.txt"), "vulnerable\n");
  await git(copy, "add", ".");
  await git(copy, "commit", "-m", "Add supported parser line");
  await git(copy, "push", "origin", "main");
  const baseCommit = await git(copy, "rev-parse", "HEAD");
  await json(
    await request.put(
      `${apiOrigin}/repositories/${repository.id}/branches/main/required-checks`,
      {
        headers: auth(owner),
        data: { checks: ["security quality"] },
      },
    ),
  );

  // The external researcher uses the protected browser workflow despite having
  // no repository membership. Nothing appears in anonymous disclosure reads.
  const reporterContext = await browser.newContext();
  await signIn(reporterContext, reporter.credential.token);
  const reporterPage = await reporterContext.newPage();
  await reporterPage.goto("/security");
  await reporterPage
    .getByRole("button", { name: "Report vulnerability" })
    .click();
  await reporterPage.getByLabel("Title").fill("Parser boundary bypass");
  await reporterPage
    .getByLabel("Suspected vulnerability")
    .fill(
      "A crafted parser document can escape the expected validation boundary.",
    );
  await reporterPage.getByLabel("Or public repository ID").fill(repository.id);
  await reporterPage.getByLabel("Affected versions").fill("1.x");
  await reporterPage.getByLabel("Evidence label").fill("Private reproduction");
  await reporterPage
    .getByLabel("Evidence (avoid live secrets)")
    .fill("A bounded malformed input demonstrates the boundary escape.");
  await reporterPage
    .getByLabel("Safe contact channel")
    .fill("security-reporter@example.test");
  await reporterPage
    .getByRole("button", { name: "Submit protected report" })
    .click();
  await expect(reporterPage).toHaveURL(/\/security\/[a-f0-9]{32}$/);
  const advisoryID = new URL(reporterPage.url()).pathname.split("/").pop()!;
  await expect(
    reporterPage.getByRole("heading", { name: "Parser boundary bypass" }),
  ).toBeVisible();
  expect(
    (
      await request.get(`${apiOrigin}/security-advisories/public/${advisoryID}`)
    ).status(),
  ).toBe(404);
  expect(
    (
      await request.get(`${apiOrigin}/activity`, { headers: auth(reporter) })
    ).status(),
  ).toBe(200);
  const activity = await (
    await request.get(`${apiOrigin}/activity`, { headers: auth(reporter) })
  ).text();
  expect(activity).not.toContain(advisoryID);

  let advisory = await json<Advisory>(
    await request.get(`${apiOrigin}/security-advisories/${advisoryID}`, {
      headers: auth(owner),
    }),
  );
  advisory = await json<Advisory>(
    await request.patch(`${apiOrigin}/security-advisories/${advisoryID}`, {
      headers: auth(owner),
      data: {
        expected_version: advisory.version,
        severity: "high",
        embargo_state: "coordinating",
      },
    }),
  );
  await json(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/responders`,
      {
        headers: auth(owner),
        data: { user_id: worker.user.id },
      },
    ),
  );
  advisory = await json<Advisory>(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/evidence`,
      {
        headers: auth(owner),
        data: {
          kind: "commit",
          repository_id: repository.id,
          commit_id: baseCommit,
          label: "Affected parser revision",
          description: "The supported line contains the vulnerable parser.",
        },
      },
    ),
  );
  const evidenceID = advisory.evidence.at(-1)!.id;
  const investigation = await json<{
    investigation: { id: string; agent_id: string };
    credential: { token: string; scopes: string[] };
  }>(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/investigations`,
      {
        headers: auth(owner),
        data: {
          mandate: "Assess exploitability from the frozen affected revision.",
          evidence_ids: [evidenceID],
          expires_in: 300,
        },
      },
    ),
  );
  expect(investigation.credential.scopes).toEqual(["security:investigate"]);
  const agentHeaders = {
    Authorization: `Bearer ${investigation.credential.token}`,
  };
  expect(
    (
      await request.patch(`${apiOrigin}/repositories/${repository.id}`, {
        headers: agentHeaders,
        data: { visibility: "private" },
      })
    ).status(),
  ).toBe(401);
  await json(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/investigations/${investigation.investigation.id}/findings`,
      {
        headers: agentHeaders,
        data: {
          kind: "conclusion",
          statement:
            "The frozen revision is affected and the supported line requires replacement.",
          evidence_ids: [evidenceID],
        },
      },
    ),
  );

  const task = await json<{ repair_task: { id: string } }>(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/repair-tasks`,
      {
        headers: auth(owner),
        data: {
          repository_id: repository.id,
          version_line: "1.x",
          title: "Repair parser 1.x",
          mandate: "Restore parser boundary validation.",
          base_commit_id: baseCommit,
          assignee_id: worker.user.id,
          assignee_kind: "agent",
          dependency_task_ids: [],
        },
      },
    ),
  );
  await json(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/reproductions`,
      {
        headers: auth(owner),
        data: {
          repository_id: repository.id,
          version_line: "1.x",
          definition: {
            name: "boundary remains closed",
            image: "alpine:3.22",
            command: "grep -qx fixed parser.txt",
            working_directory: ".",
            timeout_seconds: 30,
          },
        },
      },
    ),
  );
  const session = await json<{
    repair_session: { id: string; branch: string };
    credential: { token: string };
  }>(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/repair-tasks/${task.repair_task.id}/sessions`,
      {
        headers: auth(worker),
        data: { expires_in: 600 },
      },
    ),
  );
  const repairCopy = await mkdtemp(join(tmpdir(), "vivarium-security-repair-"));
  const repairRemote = `http://git:${session.credential.token}@127.0.0.1:8080/git/${repository.id}.git`;
  await git(tmpdir(), "clone", repairRemote, repairCopy);
  await git(repairCopy, "config", "user.name", "Repair Agent");
  await git(repairCopy, "config", "user.email", "agent@example.test");
  await writeFile(join(repairCopy, "parser.txt"), "fixed\n");
  await git(repairCopy, "add", "parser.txt");
  await git(repairCopy, "commit", "-m", "Close parser boundary");
  const candidateCommit = await git(repairCopy, "rev-parse", "HEAD");
  await git(
    repairCopy,
    "push",
    "origin",
    `HEAD:${session.repair_session.branch.replace("refs/heads/", "")}`,
  );
  await json(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/repair-sessions/${session.repair_session.id}/complete`,
      {
        headers: auth(worker),
        data: { commit_id: candidateCommit },
      },
    ),
  );
  await json(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/repair-sessions/${session.repair_session.id}/reviews`,
      {
        headers: auth(owner),
        data: {
          decision: "approve",
          commit_id: candidateCommit,
          body: "The exact candidate closes the reported boundary.",
        },
      },
    ),
  );
  const verification = await json<{ verification: { id: string } }>(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/repair-sessions/${session.repair_session.id}/verifications`,
      {
        headers: auth(owner),
        data: {},
      },
    ),
  );
  await expect
    .poll(
      async () => {
        const response = await request.get(
          `${apiOrigin}/security-advisories/${advisoryID}/verifications/${verification.verification.id}`,
          { headers: auth(owner) },
        );
        const body = (await response.json()) as {
          state: string;
          runs: Array<{ state: string }>;
        };
        return `${body.runs.length}:${body.state}:${body.runs
          .map((run) => run.state)
          .sort()
          .join(",")}`;
      },
      { timeout: 60_000 },
    )
    .toBe("2:awaiting_approval:succeeded,succeeded");
  await json(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/verifications/${verification.verification.id}/approvals`,
      {
        headers: auth(owner),
        data: {},
      },
    ),
  );

  const release = await json<{ id: string }>(
    await request.post(`${apiOrigin}/repositories/${repository.id}/releases`, {
      headers: auth(owner),
      data: {
        version: `1.0.1-${suffix}`,
        notes: "Ships the privately verified parser boundary repair.",
        commit_id: candidateCommit,
      },
    }),
  );
  await json(
    await request.post(
      `${apiOrigin}/repositories/${repository.id}/releases/${release.id}/builds`,
      { headers: auth(owner), data: {} },
    ),
  );
  await expect
    .poll(
      async () => {
        const response = await request.get(
          `${apiOrigin}/repositories/${repository.id}/releases/${release.id}/builds`,
          { headers: auth(owner) },
        );
        const body = (await response.json()) as {
          builds: Array<{ state: string }>;
        };
        return body.builds[0]?.state;
      },
      { timeout: 60_000 },
    )
    .toBe("succeeded");
  await json(
    await request.post(
      `${apiOrigin}/security-advisories/${advisoryID}/verifications/${verification.verification.id}/release-attestations`,
      {
        headers: auth(owner),
        data: { release_id: release.id },
      },
    ),
  );

  // The owner prepares and publishes through the browser. Public readers see
  // only the redacted packet, while the reporter receives an upgrade notice.
  const ownerContext = await browser.newContext();
  await signIn(ownerContext, owner.credential.token);
  const ownerPage = await ownerContext.newPage();
  await ownerPage.goto(`/security/${advisoryID}`);
  await ownerPage
    .getByPlaceholder("Public advisory title")
    .fill("Parser security update");
  await ownerPage
    .getByPlaceholder("Public, exploit-safe advisory summary")
    .fill("A parser boundary issue affected the supported 1.x line.");
  await ownerPage
    .getByPlaceholder("Exact versions and exposure-reduction steps")
    .fill(
      `Upgrade to ${`1.0.1-${suffix}`} and verify the published artifact checksum.`,
    );
  await ownerPage
    .getByPlaceholder("Public credits, comma separated")
    .fill("External Researcher");
  await ownerPage.getByRole("button", { name: "Prepare disclosure" }).click();
  await expect(ownerPage.getByText("ready", { exact: true })).toBeVisible();
  expect(
    (
      await request.get(`${apiOrigin}/security-advisories/public/${advisoryID}`)
    ).status(),
  ).toBe(404);
  await ownerPage.getByRole("button", { name: "Publish now" }).click();
  await expect(ownerPage.getByText("published", { exact: true })).toBeVisible();

  const publicResponse = await request.get(
    `${apiOrigin}/security-advisories/public/${advisoryID}`,
  );
  const publicBody = await json<{
    disclosure: { fixed_versions: Array<{ branch: string }> };
    [key: string]: unknown;
  }>(publicResponse);
  expect(JSON.stringify(publicBody)).toContain("Parser security update");
  expect(JSON.stringify(publicBody)).toContain("External Researcher");
  for (const secret of [
    "crafted parser document",
    "security-reporter@example.test",
    "frozen revision is affected",
    "boundary remains closed",
  ]) {
    expect(JSON.stringify(publicBody)).not.toContain(secret);
  }
  const inbox = await json<{
    items: Array<{ resource_id: string; kind: string }>;
  }>(await request.get(`${apiOrigin}/inbox`, { headers: auth(worker) }));
  expect(inbox.items).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ resource_id: advisoryID }),
    ]),
  );
  const branches = await json<{ branches: Array<{ name: string }> }>(
    await request.get(`${apiOrigin}/repositories/${repository.id}/branches`),
  );
  expect(branches.branches.map((branch) => branch.name)).toContain(
    publicBody.disclosure.fixed_versions[0].branch,
  );
  expect(
    branches.branches.some((branch) =>
      branch.name.startsWith("vivarium-security/"),
    ),
  ).toBeFalsy();

  await reporterContext.close();
  await ownerContext.close();
});
