import { expect, test, type BrowserContext, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this proof follows one permission-aware public-API trail */
const run = promisify(execFile);
async function git(cwd: string, ...args: string[]) {
  return (
    await run("git", args, {
      cwd,
      env: { ...process.env, GIT_TERMINAL_PROMPT: "0" },
    })
  ).stdout.trim();
}
async function account(page: Page, name: string, handle: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(name);
  await page.getByLabel("Handle").fill(handle);
  await page
    .getByRole("button", { name: "Create account and continue" })
    .click();
  await expect(
    page.getByRole("heading", { name: "Create a repository" }),
  ).toBeVisible();
  const token = await page.evaluate(() =>
    localStorage.getItem("vivarium.access-token"),
  );
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const user = await (await page.request.get("/api/user", { headers })).json();
  return { headers, user };
}
async function json(
  page: Page,
  method: "get" | "post" | "put" | "patch",
  path: string,
  headers: Record<string, string>,
  data?: unknown,
) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(
    response.status(),
    `${method} ${path}: ${body}`,
  ).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}
async function checksPass(
  page: Page,
  base: string,
  headers: Record<string, string>,
) {
  await expect
    .poll(
      async () => {
        const value = await json(
          page,
          "get",
          `${base}/merge-readiness`,
          headers,
        );
        return (
          value.required_checks.length > 0 &&
          value.required_checks.every((x: any) => x.status === "passed")
        );
      },
      { timeout: 60_000 },
    )
    .toBeTruthy();
}

test("code and guidance stay aligned through a reader-reported older-version repair", async ({
  browser,
}) => {
  test.setTimeout(240_000);
  const docker = await run("docker", ["info"]).then(
    () => true,
    () => false,
  );
  test.skip(!docker, "documentation checks require Docker");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() =>
    run("docker", ["pull", "alpine:3.22"]),
  );
  const copies: string[] = [];
  let ownerContext: BrowserContext | undefined;
  let contributorContext: BrowserContext | undefined;
  let readerContext: BrowserContext | undefined;
  try {
    const suffix = Date.now().toString(36);
    ownerContext = await browser.newContext();
    contributorContext = await browser.newContext();
    readerContext = await browser.newContext();
    const ownerPage = await ownerContext.newPage();
    const contributorPage = await contributorContext.newPage();
    const readerPage = await readerContext.newPage();
    const owner = await account(
        ownerPage,
        "Guide Owner",
        `guide-owner-${suffix}`,
      ),
      contributor = await account(
        contributorPage,
        "Behavior Contributor",
        `guide-contributor-${suffix}`,
      ),
      reader = await account(
        readerPage,
        "Release Reader",
        `guide-reader-${suffix}`,
      );
    const repository = await json(
      ownerPage,
      "post",
      "/repositories",
      owner.headers,
      { name: `living-guide-${suffix}` },
    );
    await json(
      ownerPage,
      "patch",
      `/repositories/${repository.id}`,
      owner.headers,
      { visibility: "public" },
    );
    await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/collaborators`,
      owner.headers,
      { user_id: contributor.user.id },
    );
    const credential = await json(
      ownerPage,
      "post",
      "/auth/credentials",
      owner.headers,
      {
        kind: "git",
        name: "living documentation journey",
        scopes: ["git:read", "git:write"],
        expires_in: 3600,
      },
    );
    const copy = await mkdtemp(join(tmpdir(), "vivarium-docs-"));
    copies.push(copy);
    await git(
      tmpdir(),
      "clone",
      `http://git:${credential.token}@localhost:3000/git/${repository.id}.git`,
      copy,
    );
    await git(copy, "config", "user.name", "Behavior Contributor");
    await git(copy, "config", "user.email", "contributor@example.test");
    await mkdir(join(copy, "docs"));
    await writeFile(
      join(copy, "behavior.sh"),
      '#!/bin/sh\nprintf "legacy\\n"\n',
    );
    await writeFile(
      join(copy, "docs", "behavior.md"),
      "# Behavior guide\n\nFor v0.9.0 run `sh behavior.sh`; it prints `legacy`.\n",
    );
    await git(copy, "add", ".");
    await git(copy, "commit", "-m", "Publish legacy behavior guidance");
    await git(copy, "push", "origin", "main");
    const legacyCommit = await git(copy, "rev-parse", "HEAD");
    const legacyRelease = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/releases`,
      owner.headers,
      {
        version: "v0.9.0",
        notes: "Legacy documented behavior",
        commit_id: legacyCommit,
      },
    );
    const collection = await json(
      ownerPage,
      "put",
      `/repositories/${repository.id}/documentation/new`,
      owner.headers,
      {
        expected_version: 0,
        collection: {
          name: "Behavior guide",
          description: "Version-tested behavior",
          root_path: "docs",
          source_ref: "main",
          audience: "public",
          owners: [{ actor_id: owner.user.id, role: "maintainer" }],
          supported_versions: [
            {
              label: "v0.9.0",
              source_ref: "main",
              release_id: legacyRelease.id,
              revision: legacyCommit,
            },
          ],
          rendering: {
            format: "markdown",
            syntax_highlighting: true,
            table_of_contents: true,
          },
          publication_policy: {
            review_required: true,
            source_branch: "main",
            publish_on_merge: true,
          },
        },
      },
    );
    const proposal = await json(
      contributorPage,
      "post",
      `/repositories/${repository.id}/proposals`,
      contributor.headers,
      {
        title: "Add friendly behavior",
        body: "Change the command output and teach the new supported behavior in the same governed pull.",
      },
    );
    await git(copy, "switch", "-c", "friendly-behavior");
    await writeFile(
      join(copy, "behavior.sh"),
      '#!/bin/sh\nprintf "friendly\\n"\n',
    );
    await writeFile(
      join(copy, "docs", "behavior.md"),
      "# Friendly behavior guide\n\nFor v1.0.0 run `sh behavior.sh`; it prints `friendly`.\n",
    );
    const config = (version: string) =>
      JSON.stringify({
        version: 1,
        checks: [
          {
            name: "behavior example",
            collection_id: collection.collection_id,
            image: "alpine:3.22",
            command: `test \"$(sh behavior.sh)\" = \"${version === "v1.0.0" ? "friendly" : "legacy-compatible"}\"`,
            selectors: ["samples", "commands", "tutorials"],
            dependency_paths: ["behavior.sh", "docs/behavior.md"],
            targets: [{ version, source: "release" }],
          },
        ],
      });
    await mkdir(join(copy, ".vivarium"));
    await writeFile(
      join(copy, ".vivarium", "documentation-checks.json"),
      config("v1.0.0"),
    );
    await git(copy, "add", ".");
    await git(copy, "commit", "-m", "Propose friendly documented behavior");
    await git(copy, "push", "-u", "origin", "friendly-behavior");
    const pull = await json(
      contributorPage,
      "post",
      `/repositories/${repository.id}/pulls`,
      contributor.headers,
      {
        title: "Ship friendly behavior with guidance",
        body: "One code-and-docs result.",
        source_branch: "friendly-behavior",
        target_branch: "main",
        proposal_id: proposal.id,
      },
    );
    let candidate = pull.source_commit_id;
    const task = await json(
      contributorPage,
      "post",
      `/repositories/${repository.id}/documentation-tasks`,
      contributor.headers,
      {
        title: "Explain friendly behavior",
        path: "docs/behavior.md",
        source: {
          kind: "proposal",
          resource_id: proposal.id,
          revision: candidate,
          label: "Behavior proposal",
        },
      },
    );
    const drafted = await json(
      contributorPage,
      "post",
      `/repositories/${repository.id}/documentation-tasks/${task.id}/drafts`,
      contributor.headers,
      {
        expected_version: task.version,
        body: "The new command emits friendly.",
        references: [
          {
            path: "behavior.sh",
            start_line: 1,
            end_line: 2,
            revision: candidate,
            label: "Exact behavior",
          },
        ],
      },
    );
    const base = `/repositories/${repository.id}/pulls/${pull.id}`;
    const session = await json(
      contributorPage,
      "post",
      `${base}/sessions`,
      contributor.headers,
    );
    const launched = await json(
      contributorPage,
      "post",
      `${base}/sessions/${session.id}/runs`,
      contributor.headers,
      {
        instructions: "Ground the guide in the exact implementation.",
        source_commit_id: session.source_commit_id,
        context_paths: ["behavior.sh", "docs/behavior.md"],
        working_branch: "friendly-behavior",
        expires_in: 3600,
      },
    );
    const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-docs-agent-"));
    copies.push(agentCopy);
    await git(
      tmpdir(),
      "clone",
      `http://git:${launched.credential.token}@localhost:3000/git/${repository.id}.git`,
      agentCopy,
    );
    await git(agentCopy, "config", "user.name", "Grounded Guide Agent");
    await git(agentCopy, "config", "user.email", "guide-agent@users.vivarium");
    await git(agentCopy, "switch", "friendly-behavior");
    await writeFile(
      join(agentCopy, "docs", "behavior.md"),
      "# Friendly behavior guide\n\nFor v1.0.0 run `sh behavior.sh`; it prints `friendly`.\n\nThis example is verified against the exact candidate revision.\n",
    );
    await git(agentCopy, "add", "docs/behavior.md");
    await git(agentCopy, "commit", "-m", "Ground friendly behavior guidance");
    await git(agentCopy, "push", "origin", "friendly-behavior");
    candidate = await git(agentCopy, "rev-parse", "HEAD");
    const completed = await json(
      contributorPage,
      "post",
      `${base}/sessions/${session.id}/runs/${launched.run.id}/completion`,
      { Authorization: `Bearer ${launched.credential.token}` },
      {
        summary:
          "The exact script proves friendly output; older release behavior remains a separate claim.",
        commit_id: candidate,
        checks: [
          {
            name: "grounded documentation review",
            status: "passed",
            details: "The cited script and guide agree at the frozen revision.",
          },
        ],
        commands: [
          {
            command: 'test "$(sh behavior.sh)" = friendly',
            exit_code: 0,
            summary: "Executed the documented example.",
          },
        ],
        completion_criteria: [],
        unresolved_concerns: [
          "The older release instruction still needs reader evidence.",
        ],
      },
    );
    expect(completed.run).toMatchObject({ agent_id: launched.run.agent_id });
    await json(
      contributorPage,
      "post",
      `/repositories/${repository.id}/documentation-tasks/${task.id}/entries`,
      contributor.headers,
      {
        expected_version: drafted.version,
        kind: "discussion",
        body: `Grounded agent ${launched.run.agent_id} executed the exact example; it retained uncertainty about the older release.`,
        references: [],
      },
    );
    await json(
      contributorPage,
      "post",
      `${base}/synchronize`,
      contributor.headers,
    );
    await json(
      ownerPage,
      "put",
      `/repositories/${repository.id}/branches/main/required-checks`,
      owner.headers,
      { checks: [`docs/behavior example [v1.0.0]`] },
    );
    await json(
      contributorPage,
      "post",
      `${base}/documentation-review`,
      contributor.headers,
      { collection_id: collection.collection_id, gaps: [] },
    );
    await contributorPage.goto(`/pulls/${repository.id}/${pull.id}`);
    await expect(
      contributorPage.getByText("Friendly behavior guide", { exact: true }),
    ).toBeVisible();
    await checksPass(ownerPage, base, owner.headers);
    await json(
      ownerPage,
      "post",
      `${base}/documentation-review/decisions`,
      owner.headers,
      {
        path: "docs/behavior.md",
        area: "technical",
        outcome: "approved",
        body: "Behavior and example agree.",
      },
    );
    await json(ownerPage, "post", `${base}/reviews`, owner.headers, {
      decision: "approved",
    });
    const merged = await json(
      ownerPage,
      "post",
      `${base}/merge`,
      owner.headers,
    );
    const release = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/releases`,
      owner.headers,
      {
        version: "v1.0.0",
        notes: "Friendly documented behavior",
        commit_id: merged.merge_commit_id,
      },
    );
    expect(release.commit_id).toBe(merged.merge_commit_id);
    const published = await json(
      ownerPage,
      "get",
      `/repositories/${repository.id}/documentation/${collection.collection_id}`,
      owner.headers,
    );
    expect(published.history).toHaveLength(2);
    const legacy = published.history[0];
    await readerPage.goto(
      `/repositories/${repository.id}/documentation/${collection.collection_id}/pages/behavior?version=${legacy.id}`,
    );
    await expect(
      readerPage.getByText("Archived publication", { exact: true }),
    ).toBeVisible();
    await readerPage.getByRole("combobox").selectOption("failed_example");
    await readerPage.getByPlaceholder("Version used (optional)").fill("v0.9.0");
    await readerPage
      .getByPlaceholder(/What did you try/)
      .fill(
        "The legacy instruction fails in the supported compatibility shell: expected legacy-compatible, observed legacy.",
      );
    await readerPage
      .getByRole("button", { name: "Report reader outcome" })
      .click();
    await expect(readerPage.getByText(/Report retained/)).toBeVisible();
    const feedback = (
      await json(
        ownerPage,
        "get",
        `/repositories/${repository.id}/documentation/${collection.collection_id}/feedback`,
        owner.headers,
      )
    ).feedback[0];
    expect(feedback).toMatchObject({
      reporter_id: reader.user.id,
      revision_id: legacy.id,
      version_label: "v0.9.0",
      kind: "failed_example",
    });
    const repair = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/documentation-tasks`,
      owner.headers,
      {
        title: "Repair v0.9.0 compatibility guidance",
        path: "docs/behavior.md",
        source: {
          kind: "release",
          resource_id: legacyRelease.id,
          revision: legacyCommit,
          label: "Reader-reproduced v0.9.0 failure",
        },
      },
    );
    await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/documentation/${collection.collection_id}/feedback/${feedback.id}/triage`,
      owner.headers,
      { kind: "documentation_task", resource_id: repair.id },
    );
    await git(copy, "switch", "main");
    await git(copy, "pull", "--ff-only");
    await git(copy, "switch", "-c", "repair-v0-guide");
    await writeFile(
      join(copy, "docs", "behavior.md"),
      "# Friendly behavior guide\n\nFor v1.0.0 run `sh behavior.sh`; it prints `friendly`.\n\nFor v0.9.0 compatibility, expect `legacy` (not `legacy-compatible`).\n",
    );
    await git(copy, "add", "docs/behavior.md");
    await git(copy, "commit", "-m", "Repair older release instruction");
    await git(copy, "push", "-u", "origin", "repair-v0-guide");
    const repairPull = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/pulls`,
      owner.headers,
      {
        title: "Correct v0.9.0 guidance",
        body: "Version-specific repair from retained reader evidence.",
        source_branch: "repair-v0-guide",
        target_branch: "main",
      },
    );
    const repairBase = `/repositories/${repository.id}/pulls/${repairPull.id}`;
    await json(
      ownerPage,
      "post",
      `${repairBase}/documentation-review`,
      owner.headers,
      { collection_id: collection.collection_id, gaps: [] },
    );
    await json(
      ownerPage,
      "post",
      `${repairBase}/documentation-review/decisions`,
      owner.headers,
      {
        path: "docs/behavior.md",
        area: "versions",
        outcome: "approved",
        body: "The older release instruction now matches the reproduced output.",
      },
    );
    await json(ownerPage, "post", `${repairBase}/reviews`, owner.headers, {
      decision: "approved",
    });
    await checksPass(ownerPage, repairBase, owner.headers);
    const repairedMerge = await json(
      ownerPage,
      "post",
      `${repairBase}/merge`,
      owner.headers,
    );
    await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/releases`,
      owner.headers,
      {
        version: "v1.0.1",
        notes: "Correct older compatibility guidance",
        commit_id: repairedMerge.merge_commit_id,
      },
    );
    const final = await json(
      ownerPage,
      "get",
      `/repositories/${repository.id}/documentation/${collection.collection_id}`,
      owner.headers,
    );
    expect(final.history).toHaveLength(3);
    expect(final.history[0].pages[0].source_sha256).toBe(
      legacy.pages[0].source_sha256,
    );
    expect(final.collection.published_pull_id).toBe(repairPull.id);
    const repairedPage = await json(
      readerPage,
      "get",
      `/repositories/${repository.id}/documentation/${collection.collection_id}/pages/behavior?version=${final.collection.id}`,
      reader.headers,
    );
    expect(repairedPage).toMatchObject({
      archived: false,
      collection: {
        id: final.collection.id,
        published_pull_id: repairPull.id,
      },
    });
    expect(repairedPage.body).toContain(
      "For v0.9.0 compatibility, expect `legacy` (not `legacy-compatible`).",
    );
    const retained = (
      await json(
        ownerPage,
        "get",
        `/repositories/${repository.id}/documentation/${collection.collection_id}/feedback`,
        owner.headers,
      )
    ).feedback[0];
    expect(retained).toMatchObject({
      triage_kind: "documentation_task",
      linked_resource_id: repair.id,
      triaged_by: owner.user.id,
    });
  } finally {
    await Promise.allSettled([
      ownerContext?.close(),
      contributorContext?.close(),
      readerContext?.close(),
      ...copies.map((x) => rm(x, { recursive: true, force: true })),
    ]);
  }
});
