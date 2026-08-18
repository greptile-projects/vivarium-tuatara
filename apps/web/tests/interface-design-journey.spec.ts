import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey verifies one cross-resource retained trail */

const run = promisify(execFile);
const digest = (value: string) =>
  createHash("sha256").update(value).digest("hex");
async function git(cwd: string, ...args: string[]) {
  return (
    await run("git", args, {
      cwd,
      env: { ...process.env, GIT_TERMINAL_PROMPT: "0" },
    })
  ).stdout.trim();
}
async function account(page: Page, suffix: string, role: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(`${role} collaborator`);
  await page
    .getByLabel("Handle")
    .fill(`interface-${role.toLowerCase()}-${suffix}`);
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
  method: "get" | "post" | "put",
  path: string,
  headers: Record<string, string>,
  data?: unknown,
) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(
    response.status(),
    `${method.toUpperCase()} ${path}: ${body}`,
  ).toBeGreaterThanOrEqual(200);
  expect(
    response.status(),
    `${method.toUpperCase()} ${path}: ${body}`,
  ).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}
async function rejected(response: APIResponse, status: number, code: string) {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
  expect(JSON.parse(body)).toMatchObject({ error: { code } });
}
async function eventually<T>(
  read: () => Promise<T>,
  ready: (value: T) => boolean,
  label: string,
) {
  let value = await read();
  await expect
    .poll(
      async () => {
        value = await read();
        return ready(value);
      },
      { timeout: 60_000, message: label },
    )
    .toBeTruthy();
  return value;
}

test("a team turns user feedback into a governed shipped interface", async ({
  browser,
}) => {
  test.setTimeout(300_000);
  const dockerAvailable = await run("docker", ["info"]).then(
    () => true,
    () => false,
  );
  test.skip(
    !dockerAvailable,
    "the interface journey requires bounded preview, check, build, and deployment workspaces",
  );
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() =>
    run("docker", ["pull", "alpine:3.22"]),
  );
  const copy = await mkdtemp(join(tmpdir(), "vivarium-interface-design-"));
  const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-interface-agent-"));
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const developerPage = await (await browser.newContext()).newPage();
    const userPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, suffix, "Designer");
    const developer = await account(developerPage, suffix, "Developer");
    const invited = await account(userPage, suffix, "Invited-user");
    const repository = await json(
      ownerPage,
      "post",
      "/repositories",
      owner.headers,
      { name: `responsive-setup-${suffix}` },
    );
    for (const user of [developer.user, invited.user])
      await json(
        ownerPage,
        "post",
        `/repositories/${repository.id}/collaborators`,
        owner.headers,
        { user_id: user.id },
      );
    const credential = await json(
      ownerPage,
      "post",
      "/auth/credentials",
      owner.headers,
      {
        kind: "git",
        name: "interface journey",
        scopes: ["git:read", "git:write"],
        expires_in: 3600,
      },
    );
    await git(
      tmpdir(),
      "clone",
      `http://git:${credential.token}@localhost:3000/git/${repository.id}.git`,
      copy,
    );
    await git(copy, "config", "user.name", "Product Designer");
    await git(copy, "config", "user.email", "designer@example.test");
    await mkdir(join(copy, ".vivarium"));
    const previewDefinition = JSON.stringify({
      version: 1,
      image: "alpine:3.22",
      build: "mkdir -p dist && cp setup.html dist/index.html",
      output_path: "dist",
      resources: {
        cpus: 1,
        memory_mb: 256,
        storage_mb: 64,
        timeout_seconds: 30,
      },
      access: {
        network: "none",
        data: "preview_artifacts",
        identity: "named_users",
        actions: ["view", "test", "feedback"],
      },
    });
    const interfaceDefinition = JSON.stringify({
      version: 1,
      journeys: ["resume setup"],
    });
    await writeFile(join(copy, ".vivarium", "preview.json"), previewDefinition);
    await writeFile(
      join(copy, ".vivarium", "interface-checks.json"),
      interfaceDefinition,
    );
    await writeFile(
      join(copy, ".vivarium", "checks.json"),
      JSON.stringify({
        version: 1,
        checks: [
          {
            name: "responsive setup",
            image: "alpine:3.22",
            command:
              "grep -q 'aria-live=.polite.' setup.html && grep -q 'data-mobile=.stack.' setup.html",
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
            name: "package",
            image: "alpine:3.22",
            command: 'cp setup.html "$VIVARIUM_OUTPUT/setup.html"',
          },
        ],
      }),
    );
    await writeFile(
      join(copy, ".vivarium", "deployment.json"),
      JSON.stringify({
        version: 1,
        stages: [
          {
            name: "staged",
            signals: [
              {
                name: "resume setup journey",
                command: "grep -q 'Continue setup' \"$VIVARIUM_ARTIFACT\"",
              },
            ],
          },
        ],
      }),
    );
    await writeFile(
      join(copy, ".vivarium", "packages.json"),
      JSON.stringify({ version: 1, dependencies: [], lock: [] }),
    );
    await writeFile(
      join(copy, "setup.html"),
      "<main><button>Resume</button></main>\n",
    );
    await writeFile(
      join(copy, "tokens.css"),
      ":root { --space-action: 12px; }\n",
    );
    await git(copy, "add", ".");
    await git(copy, "commit", "-m", "Release initial setup journey");
    await git(copy, "push", "origin", "main");
    const base = await git(copy, "rev-parse", "HEAD");
    const baseRelease = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/releases`,
      owner.headers,
      { version: "v1.0.0", notes: "Initial setup journey", commit_id: base },
    );
    const environment = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/environments`,
      owner.headers,
      {
        name: "staged",
        position: 1,
        image: "alpine:3.22",
        command: "true",
        timeout_seconds: 30,
        configuration: {},
        credentials: {},
        required_approvals: 0,
        concurrency: 1,
      },
    );

    const feedback = await json(
      userPage,
      "post",
      `/repositories/${repository.id}/feedback`,
      invited.headers,
      {
        target: {
          kind: "release",
          resource_id: baseRelease.id,
          label: "Setup v1",
        },
        need: "Returning users cannot tell where setup resumes on a phone.",
        desired_outcome:
          "Show a clear responsive continuation with announced status.",
        frequency: "Every interrupted setup",
        impact: "People abandon setup",
        audience: "project",
        identity_visibility: "audience",
        contact_preference: "discussion",
        evidence: [
          {
            name: "Redacted mobile session",
            kind: "research_note",
            summary: "The participant missed the Resume action.",
            visibility: "audience",
            redacted: true,
          },
        ],
        links: [],
      },
    );
    const source = {
      kind: "feedback",
      resource_id: feedback.id,
      summary: feedback.need,
    };
    const artifact = (content: string) => ({
      id: "resume-prototype",
      kind: "prototype",
      title: "Responsive setup resume",
      description: "Interactive mobile and desktop flow",
      content,
      interactions: ["activate Continue setup", "status is announced"],
      audience: [owner.user.id, developer.user.id, invited.user.id],
      author_id: owner.user.id,
      license: "CC-BY-4.0",
      source: "designer prototype",
      transformations: ["agent-assisted contrast alternative compared"],
    });
    const revision = (
      states: any[],
      content: string,
      alternatives: string[],
    ) => ({
      title: "Resume interrupted setup",
      user_goal: "Return to the exact unfinished setup step",
      source,
      journeys: [
        {
          name: "resume setup",
          actor: "returning user",
          goal: "continue without losing context",
          steps: ["open setup", "hear current status", "continue"],
        },
      ],
      states,
      content: ["Continue setup", "Step 2 of 4"],
      constraints: ["WCAG 2.2 AA", "localizable without clipping"],
      alternatives,
      success_measures: ["80 percent of invited users resume without help"],
      affected_components: ["Setup progress"],
      component_contracts: ["Setup progress announces the current step"],
      breakpoints: ["stack actions below 640px"],
      acceptance_criteria: [
        "Setup progress announces the current step",
        "Mobile actions stack without clipping",
      ],
      evidence: [
        { kind: "feedback", resource_id: feedback.id, summary: feedback.need },
      ],
      artifacts: [artifact(content)],
      uncertainty: ["Long German labels need exact preview evidence"],
    });
    let design = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/design-proposals`,
      owner.headers,
      {
        owner_ids: [owner.user.id],
        revision: revision(
          [
            {
              name: "default",
              description: "Current step and action",
              content: "Step 2 of 4 · Continue setup",
            },
          ],
          "static mobile prototype",
          ["Designer progress card", "Agent-assisted higher-contrast stepper"],
        ),
      },
    );
    await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/design-proposals/${design.id}/comments`,
      owner.headers,
      {
        revision: 1,
        kind: "comment",
        body: "The grounded agent alternative improves contrast but omits the interrupted loading state.",
        evidence: [
          {
            kind: "feedback",
            resource_id: feedback.id,
            summary: "Compare against the reported need",
          },
        ],
      },
    );
    await json(
      userPage,
      "post",
      `/repositories/${repository.id}/design-proposals/${design.id}/comments`,
      invited.headers,
      {
        revision: 1,
        kind: "dissent",
        body: "The static prototype is stale: it has no loading state after I return on slow mobile.",
        evidence: [
          {
            kind: "feedback",
            resource_id: feedback.id,
            summary: "Invited-user evidence",
          },
        ],
      },
    );
    design = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/design-proposals/${design.id}/revisions`,
      owner.headers,
      {
        expected_version: 1,
        revision: revision(
          [
            {
              name: "loading",
              description: "Resume state is restored",
              content: "Restoring step 2…",
            },
            {
              name: "ready",
              description: "Current step and action",
              content: "Step 2 of 4 · Continue setup",
            },
            {
              name: "error",
              description: "Recovery remains actionable",
              content: "Could not restore · Try again",
            },
          ],
          "responsive keyboard-operable prototype",
          [
            "Designer progress card",
            "Agent-assisted contrast merged with complete states",
          ],
        ),
      },
    );
    design = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/design-proposals/${design.id}/acknowledgements`,
      owner.headers,
      {
        revision: 2,
        owner_id: owner.user.id,
        status: "acknowledged",
        note: "Responsive, localized, and accessible states are complete.",
      },
    );
    const handoff = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/design-proposals/${design.id}/implementation`,
      owner.headers,
      {
        expected_version: 2,
        title: "Implement responsive setup resume",
        body: "Carry the accepted prototype into reviewed product code.",
        tasks: [
          {
            title: "Implement accessible progress",
            assignee_type: "agent",
            assignee_id: "",
            depends_on_previous: false,
          },
          {
            title: "Polish responsive localized content",
            assignee_type: "human",
            assignee_id: developer.user.id,
            depends_on_previous: false,
          },
        ],
      },
    );
    const agentTask = handoff.tasks[0];
    const taskBase = `/repositories/${repository.id}/proposals/${handoff.proposal.id}/tasks/${agentTask.id}`;
    const launched = await json(
      ownerPage,
      "post",
      `${taskBase}/sessions`,
      owner.headers,
      {
        expected_assignment_id: agentTask.assignment.id,
        context_paths: [
          "setup.html",
          "tokens.css",
          ".vivarium/interface-checks.json",
        ],
        expires_in: 3600,
      },
    );
    await git(
      tmpdir(),
      "clone",
      `http://git:${launched.credential.token}@localhost:3000/git/${repository.id}.git`,
      agentCopy,
    );
    await git(agentCopy, "config", "user.name", "Vivarium Interface Agent");
    await git(
      agentCopy,
      "config",
      "user.email",
      "interface-agent@users.vivarium",
    );
    await git(
      agentCopy,
      "switch",
      "-c",
      "agent/setup-resume",
      `origin/${launched.run.working_branch}`,
    );
    await writeFile(
      join(agentCopy, "setup.html"),
      "<main data-mobile='stack'><p aria-live='polite'>Step 2 of 4</p><button>Continue setup</button></main>\n",
    );
    await git(agentCopy, "add", "setup.html");
    await git(agentCopy, "commit", "-m", "Implement accessible setup progress");
    await git(
      agentCopy,
      "push",
      "origin",
      `HEAD:refs/heads/${launched.run.working_branch}`,
    );
    let candidate = await git(agentCopy, "rev-parse", "HEAD");
    const agentHeaders = {
      Authorization: `Bearer ${launched.credential.token}`,
    };
    await json(
      ownerPage,
      "post",
      `${taskBase}/sessions/${launched.session.id}/runs/${launched.run.id}/completion`,
      agentHeaders,
      {
        summary: "Implemented the accessible responsive progress state.",
        commit_id: candidate,
        checks: [
          {
            name: "responsive setup",
            status: "passed",
            details: "Exact candidate passed.",
          },
        ],
        commands: [
          {
            command: "grep aria-live setup.html",
            exit_code: 0,
            summary: "Live status retained.",
          },
        ],
        completion_criteria: [
          {
            criterion: "Setup progress announces the current step",
            status: "met",
            evidence: "polite live region",
          },
          {
            criterion: "Mobile actions stack without clipping",
            status: "met",
            evidence: "mobile stack contract",
          },
        ],
        unresolved_concerns: [],
      },
    );
    const pull = await json(
      ownerPage,
      "post",
      `${taskBase}/contributions`,
      owner.headers,
      {
        title: "Ship responsive setup resume",
        body: "Agent implementation followed by human content polish.",
        source_branch: launched.run.working_branch,
        target_branch: "main",
        session_id: launched.session.id,
        run_id: launched.run.id,
      },
    );
    await git(copy, "fetch", "origin", launched.run.working_branch);
    await git(
      copy,
      "switch",
      "-C",
      "human/setup-polish",
      `origin/${launched.run.working_branch}`,
    );
    await writeFile(
      join(copy, "setup.html"),
      "<main data-mobile='stack' lang='en'><p aria-live='polite'>Step 2 of 4</p><button>Continue setup</button><p hidden>Einrichtung fortsetzen</p></main>\n",
    );
    await git(copy, "add", "setup.html");
    await git(copy, "commit", "-m", "Polish localized responsive content");
    await git(copy, "push", "origin", `HEAD:${launched.run.working_branch}`);
    candidate = await git(copy, "rev-parse", "HEAD");
    await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/pulls/${pull.id}/synchronize`,
      owner.headers,
      {},
    );
    await json(
      developerPage,
      "post",
      `/repositories/${repository.id}/design-proposals/${design.id}/implementation/reports`,
      developer.headers,
      {
        mapping: {
          requirement: "Setup progress announces the current step",
          code_paths: ["setup.html"],
          rendered_surfaces: ["setup resume"],
          evidence: [candidate],
        },
      },
    );
    let reported = await json(
      developerPage,
      "post",
      `/repositories/${repository.id}/design-proposals/${design.id}/implementation/reports`,
      developer.headers,
      {
        deviation: {
          requirement: "Mobile actions stack without clipping",
          reason: "Use an inline action at tablet width",
          impact: "The accepted breakpoint would change",
        },
      },
    );
    const deviation = reported.implementation.deviations[0];
    reported = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/design-proposals/${design.id}/implementation/deviations/${deviation.id}/decision`,
      owner.headers,
      {
        status: "rejected",
        note: "Keep the accepted mobile stacking contract.",
      },
    );
    expect(reported.implementation.deviations[0].status).toBe("rejected");

    const pullBase = `/repositories/${repository.id}/pulls/${pull.id}`;
    const stalePreview = await json(
      ownerPage,
      "post",
      `${pullBase}/previews`,
      owner.headers,
      {},
    );
    await eventually(
      () => json(ownerPage, "get", `${pullBase}/previews`, owner.headers),
      (x: any) =>
        x.previews.some(
          (p: any) => p.id === stalePreview.id && p.state === "succeeded",
        ),
      "first preview succeeds",
    );
    await writeFile(
      join(copy, "setup.html"),
      "<main data-mobile='stack' lang='en'><p aria-live='polite'>Step 2 of 4</p><button>Continue setup</button><p hidden>Einrichtung fortsetzen</p><span>Ready</span></main>\n",
    );
    await git(copy, "add", "setup.html");
    await git(copy, "commit", "-m", "Make preview revision exact");
    await git(copy, "push", "origin", `HEAD:${launched.run.working_branch}`);
    candidate = await git(copy, "rev-parse", "HEAD");
    await json(ownerPage, "post", `${pullBase}/synchronize`, owner.headers, {});
    await rejected(
      await ownerPage.request.post(`/api${pullBase}/interface-checks`, {
        headers: owner.headers,
        data: { revision: candidate, preview_id: stalePreview.id },
      }),
      422,
      "invalid_interface_preview",
    );
    const preview = await json(
      ownerPage,
      "post",
      `${pullBase}/previews`,
      owner.headers,
      {},
    );
    await eventually(
      () => json(ownerPage, "get", `${pullBase}/previews`, owner.headers),
      (x: any) =>
        x.previews.some(
          (p: any) =>
            p.id === preview.id &&
            p.state === "succeeded" &&
            p.revision === candidate &&
            !p.stale,
        ),
      "current preview succeeds",
    );
    const checkInput = (status: string, differences: any[]) => ({
      revision: candidate,
      preview_id: preview.id,
      definition_path: ".vivarium/interface-checks.json",
      definition_digest: digest(interfaceDefinition),
      design_proposal_id: design.id,
      design_version: 2,
      name: "responsive localized setup",
      journey: "resume setup",
      context: {
        viewport: "375x812",
        theme: "high contrast",
        content: "long",
        locale: "de-DE",
        interaction: "keyboard",
        assistive_technology: "screen reader",
      },
      status,
      coverage: ["Setup progress"],
      affected_requirements: [
        "Setup progress announces the current step",
        "Mobile actions stack without clipping",
      ],
      differences,
      artifacts: [
        {
          id: "recording",
          kind: "recording",
          name: "keyboard-de.webm",
          url: "/artifacts/keyboard-de.webm",
          digest: "a".repeat(64),
          size_bytes: 2048,
        },
      ],
      performance: [
        {
          metric: "interaction",
          unit: "ms",
          baseline: 80,
          candidate: 95,
          budget: 120,
          passed: true,
        },
      ],
    });
    const regression = await json(
      ownerPage,
      "post",
      `${pullBase}/interface-checks`,
      owner.headers,
      checkInput("failed", [
        {
          id: "visual-spacing",
          kind: "visual",
          summary: "Action spacing differs from accepted token",
          requirement: "Mobile actions stack without clipping",
        },
      ]),
    );
    await json(
      ownerPage,
      "post",
      `${pullBase}/interface-checks/${regression.id}/classifications`,
      owner.headers,
      {
        revision: candidate,
        difference_id: "visual-spacing",
        outcome: "regression",
        rationale:
          "This is a visual regression, not an intentional design change.",
      },
    );
    expect(
      (
        await json(
          ownerPage,
          "get",
          `${pullBase}/design-readiness`,
          owner.headers,
        )
      ).ready,
    ).toBe(false);
    const passed = await json(
      ownerPage,
      "post",
      `${pullBase}/interface-checks`,
      owner.headers,
      checkInput("passed", []),
    );
    const policy = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/design-acceptance-policies`,
      owner.headers,
      {
        name: "Setup journey acceptance",
        selectors: [{ kind: "journey", value: "resume setup" }],
        requirements: [
          { role: "design_owner", approver_ids: [owner.user.id] },
          { role: "accessibility", approver_ids: [owner.user.id] },
          { role: "localization", approver_ids: [developer.user.id] },
          { role: "invited_user", approver_ids: [invited.user.id] },
        ],
        exception_max_hours: 24,
      },
    );
    for (const [page, actor, role, rationale] of [
      [ownerPage, owner, "design_owner", "Matches the accepted prototype."],
      [
        ownerPage,
        owner,
        "accessibility",
        "Keyboard and screen-reader evidence passes.",
      ],
      [
        developerPage,
        developer,
        "localization",
        "Long German content remains available without clipping.",
      ],
      [
        userPage,
        invited,
        "invited_user",
        "I can resume the interrupted journey without help.",
      ],
    ] as const)
      await json(
        page,
        "post",
        `${pullBase}/design-acceptances`,
        actor.headers,
        {
          policy_id: policy.id,
          policy_version: 1,
          role,
          decision: "accepted",
          rationale,
        },
      );
    const readiness = await json(
      ownerPage,
      "get",
      `${pullBase}/design-readiness`,
      owner.headers,
    );
    expect(readiness.ready).toBe(true);
    expect(readiness.revision).toBe(candidate);
    await json(ownerPage, "post", `${pullBase}/reviews`, owner.headers, {
      decision: "approved",
    });
    const merged = await json(
      ownerPage,
      "post",
      `${pullBase}/merge`,
      owner.headers,
      {},
    );
    const release = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/releases`,
      owner.headers,
      {
        version: "v1.1.0",
        notes: "Responsive setup resume",
        commit_id: merged.merge_commit_id,
      },
    );
    await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/dependency-inventories`,
      owner.headers,
      { commit_id: merged.merge_commit_id },
    );
    await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/releases/${release.id}/builds`,
      owner.headers,
      {},
    );
    const builds = await eventually(
      () =>
        json(
          ownerPage,
          "get",
          `/repositories/${repository.id}/releases/${release.id}/builds`,
          owner.headers,
        ),
      (x: any) => x.builds?.some((b: any) => b.state === "succeeded"),
      "interface release build succeeds",
    );
    const build = builds.builds.find((b: any) => b.state === "succeeded");
    const pending = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/deployments`,
      owner.headers,
      {
        environment_id: environment.id,
        release_id: release.id,
        build_id: build.id,
        artifact_id: build.artifacts[0].id,
      },
    );
    const deployments = await eventually(
      () =>
        json(
          ownerPage,
          "get",
          `/repositories/${repository.id}/deployments`,
          owner.headers,
        ),
      (x: any) =>
        x.deployments?.some(
          (d: any) => d.id === pending.id && d.state === "succeeded",
        ),
      "staged interface deployment succeeds",
    );
    expect(
      deployments.deployments.find((d: any) => d.id === pending.id).state,
    ).toBe("succeeded");
    expect(
      (
        await json(
          ownerPage,
          "get",
          `/repositories/${repository.id}/releases/${release.id}/design-readiness`,
          owner.headers,
        )
      ).ready,
    ).toBe(true);

    const systemRevision = (token: string) => ({
      title: "Setup interface language",
      summary: "Responsive accessible setup patterns",
      rationale: "The delivered journey establishes the reusable contract.",
      commit_id: merged.merge_commit_id,
      release_id: release.id,
      owner_ids: [owner.user.id],
      themes: ["light", "high contrast"],
      tokens: [
        {
          name: "space-action",
          category: "spacing",
          value: token,
          theme: "light",
          description: "Action separation",
          owner_ids: [owner.user.id],
        },
      ],
      components: [
        {
          name: "Setup progress",
          description: "Announces setup position",
          usage: "Interrupted setup",
          source_path: "setup.html",
          owner_ids: [owner.user.id],
          constraints: {
            accessibility: ["polite live status"],
            localization: ["supports long labels"],
          },
          examples: [
            {
              title: "Resume",
              description: "Step two",
              properties: { state: "ready" },
            },
          ],
        },
      ],
      interaction_patterns: [],
      content_rules: [],
      responsive_rules: [
        {
          name: "Mobile stack",
          condition: "below 640px",
          behavior: "stack actions",
          owner_ids: [owner.user.id],
        },
      ],
      adoption_policy: {
        level: "required",
        supported_consumers: [repository.id],
        exceptions: [],
        migration_guidance:
          "Adopt the current spacing token and rerun exact interface evidence.",
      },
      implementations: [
        {
          consumer: repository.id,
          repository_id: repository.id,
          release_id: release.id,
          commit_id: merged.merge_commit_id,
          definition_name: "Setup progress",
          status: "current",
        },
      ],
    });
    let system = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/interface-systems`,
      owner.headers,
      { revision: systemRevision("12px") },
    );
    system = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/interface-systems/${system.id}/revisions`,
      owner.headers,
      { expected_version: 1, revision: systemRevision("16px") },
    );
    const migration = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/interface-systems/${system.id}/migration-work`,
      owner.headers,
      {
        repository_id: repository.id,
        title: "Adopt changed action spacing token",
        outcome:
          "Consumer uses space-action 16px and reruns responsive evidence.",
        documentation: true,
      },
    );
    const measured = await json(
      userPage,
      "post",
      `/repositories/${repository.id}/feedback`,
      invited.headers,
      {
        target: {
          kind: "release",
          resource_id: release.id,
          label: "Setup v1.1",
        },
        need: "The shipped continuation is now clear on mobile.",
        desired_outcome: "Keep the responsive announced progress behavior.",
        frequency: "Measured after staged release",
        impact: "Invited user completed setup without assistance",
        audience: "project",
        identity_visibility: "audience",
        contact_preference: "discussion",
        evidence: [
          {
            name: "Measured staged journey",
            kind: "outcome",
            summary: "The invited user resumed and completed setup.",
            visibility: "audience",
            redacted: true,
          },
        ],
        links: [],
      },
    );
    const repair = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/releases/${release.id}/design-repairs`,
      owner.headers,
      {
        source_kind: "feedback",
        source_id: measured.id,
        title: "Retain measured resume clarity",
        outcome:
          "Keep the delivered journey measurable while adopting the successor token.",
      },
    );
    expect(migration.interface_system_version).toBe(2);
    expect(repair.release_commit_id).toBe(merged.merge_commit_id);
    expect(passed.revision).toBe(candidate);

    await ownerPage.goto(`/repositories/${repository.id}/design`);
    await expect(
      ownerPage.getByRole("heading", { name: "Product design proposals" }),
    ).toBeVisible();
    await expect(
      ownerPage.getByText("Resume interrupted setup", { exact: true }).first(),
    ).toBeVisible();
    await expect(
      ownerPage.getByText("The static prototype is stale", { exact: false }),
    ).toBeVisible();
    await developerPage.goto(`/pulls/${repository.id}/${pull.id}`);
    await expect(
      developerPage.getByRole("heading", { name: "Interface verification" }),
    ).toBeVisible();
    await expect(
      developerPage
        .getByText("responsive localized setup", { exact: true })
        .first(),
    ).toBeVisible();
    await expect(
      developerPage.getByText("Action spacing differs from accepted token", {
        exact: true,
      }),
    ).toBeVisible();
    await ownerPage.goto(`/repositories/${repository.id}/interface-system`);
    await expect(
      ownerPage.getByRole("heading", { name: "Shared product language" }),
    ).toBeVisible();
    await expect(
      ownerPage.getByRole("textbox", { name: "Token value" }),
    ).toHaveAttribute("value", "16px");
  } finally {
    await rm(copy, { recursive: true, force: true });
    await rm(agentCopy, { recursive: true, force: true });
  }
});
