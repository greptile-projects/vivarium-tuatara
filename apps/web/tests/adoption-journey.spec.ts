import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey joins independently versioned public resources */
const run = promisify(execFile);
async function git(cwd: string, ...args: string[]) {
  return (
    await run("git", args, {
      cwd,
      env: { ...process.env, GIT_TERMINAL_PROMPT: "0" },
    })
  ).stdout.trim();
}
async function authenticatedGit(token: string, cwd: string, ...args: string[]) {
  return (
    await run("git", ["-c", "credential.helper=", ...args], {
      cwd,
      env: {
        ...process.env,
        GIT_ASKPASS: join(__dirname, "git-askpass.sh"),
        GIT_TERMINAL_PROMPT: "0",
        VIVARIUM_GIT_TOKEN: token,
      },
    })
  ).stdout.trim();
}
async function account(page: Page, suffix: string, name: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(name);
  await page
    .getByLabel("Handle")
    .fill(`${name.toLowerCase().replaceAll(" ", "-")}-${suffix}`);
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
  return {
    headers,
    user: (await (
      await page.request.get("/api/user", { headers })
    ).json()) as any,
  };
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
    `${method.toUpperCase()} ${path}: ${body}`,
  ).toBeGreaterThanOrEqual(200);
  expect(
    response.status(),
    `${method.toUpperCase()} ${path}: ${body}`,
  ).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}
async function rejected(response: APIResponse, status: number) {
  expect(response.status(), await response.text()).toBe(status);
  return response.json();
}
async function eventually<T>(
  read: () => Promise<T>,
  ready: (value: T) => boolean,
  label: string,
) {
  await expect
    .poll(async () => ready(await read()), {
      timeout: 60_000,
      intervals: [250, 500, 1000],
      message: label,
    })
    .toBe(true);
  return read();
}

test("independent teams prove adoption and return the missing fit upstream", async ({
  browser,
}) => {
  test.setTimeout(360_000);
  const docker = await run("docker", ["info"]).then(
    () => true,
    () => false,
  );
  test.skip(
    !docker,
    "adoption delivery uses package, check, build, and deployment containers",
  );
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() =>
    run("docker", ["pull", "alpine:3.22"]),
  );
  const copies: string[] = [],
    gitCredentials: {
      page: Page;
      headers: Record<string, string>;
      id: string;
    }[] = [],
    temporary = async (prefix: string) => {
      const path = await mkdtemp(join(tmpdir(), prefix));
      copies.push(path);
      return path;
    };
  try {
    const suffix = Date.now().toString(36),
      providerPage = await (await browser.newContext()).newPage(),
      adopterPage = await (await browser.newContext()).newPage(),
      userPage = await (await browser.newContext()).newPage();
    const provider = await account(providerPage, suffix, "Provider Maintainer"),
      adopter = await account(adopterPage, suffix, "Consumer Owner"),
      targetUser = await account(userPage, suffix, "Target User");
    const providerOrg = await json(
      providerPage,
      "post",
      "/organizations",
      provider.headers,
      { name: `Provider Lab ${suffix}`, slug: `provider-lab-${suffix}` },
    );
    let org = await json(
      providerPage,
      "post",
      `/organizations/${providerOrg.id}/agents`,
      provider.headers,
      {
        name: "Fit Reproducer",
        slug: `fit-reproducer-${suffix}`,
        capabilities: ["reproduce synthetic package fit"],
        operator_ids: [provider.user.id],
        team_ids: [],
      },
    );
    const agent = org.agents.at(-1);
    const providerRepo = await json(
      providerPage,
      "post",
      `/organizations/${providerOrg.id}/repositories`,
      provider.headers,
      { name: `event-codec-${suffix}` },
    );
    const consumerRepo = await json(
      adopterPage,
      "post",
      "/repositories",
      adopter.headers,
      { name: `independent-checkout-${suffix}` },
    );
    await json(
      providerPage,
      "patch",
      `/repositories/${providerRepo.id}`,
      provider.headers,
      { visibility: "public" },
    );
    await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/collaborators`,
      adopter.headers,
      { user_id: targetUser.user.id },
    );
    const credential = async (
      page: Page,
      headers: Record<string, string>,
      name: string,
    ) =>
      json(page, "post", "/auth/credentials", headers, {
        kind: "git",
        name,
        scopes: ["git:read", "git:write"],
        expires_in: 3600,
      });
    const providerGit = await credential(
      providerPage,
      provider.headers,
      "provider adoption journey",
    );
    gitCredentials.push({
      page: providerPage,
      headers: provider.headers,
      id: providerGit.id,
    });
    const consumerGit = await credential(
      adopterPage,
      adopter.headers,
      "consumer adoption journey",
    );
    gitCredentials.push({
      page: adopterPage,
      headers: adopter.headers,
      id: consumerGit.id,
    });
    const providerCopy = await temporary("vivarium-adoption-provider-"),
      consumerCopy = await temporary("vivarium-adoption-consumer-");
    await authenticatedGit(
      providerGit.token,
      tmpdir(),
      "clone",
      `http://localhost:3000/git/${providerRepo.id}.git`,
      providerCopy,
    );
    await authenticatedGit(
      consumerGit.token,
      tmpdir(),
      "clone",
      `http://localhost:3000/git/${consumerRepo.id}.git`,
      consumerCopy,
    );
    expect(await git(providerCopy, "remote", "get-url", "origin")).toBe(
      `http://localhost:3000/git/${providerRepo.id}.git`,
    );
    expect(await git(consumerCopy, "remote", "get-url", "origin")).toBe(
      `http://localhost:3000/git/${consumerRepo.id}.git`,
    );
    for (const [copy, name] of [
      [providerCopy, "Provider Maintainer"],
      [consumerCopy, "Consumer Owner"],
    ]) {
      await git(copy, "config", "user.name", name);
      await git(
        copy,
        "config",
        "user.email",
        `${name.replaceAll(" ", "-").toLowerCase()}@example.test`,
      );
      await mkdir(join(copy, ".vivarium"));
    }
    const packageName = `event-codec-${suffix}`;
    await writeFile(
      join(providerCopy, ".vivarium", "release.json"),
      JSON.stringify({
        version: 1,
        steps: [
          {
            name: "package",
            image: "alpine:3.22",
            command: 'cp codec.txt "$VIVARIUM_OUTPUT/codec.txt"',
          },
        ],
      }),
    );
    await writeFile(
      join(providerCopy, "codec.txt"),
      "codec=1.0.0\nmode=strict\n",
    );
    await git(providerCopy, "add", ".");
    await git(providerCopy, "commit", "-m", "Release strict event codec");
    await authenticatedGit(providerGit.token, providerCopy, "push", "origin", "main");
    const providerV1 = await git(providerCopy, "rev-parse", "HEAD");
    const releaseV1 = await json(
      providerPage,
      "post",
      `/repositories/${providerRepo.id}/releases`,
      provider.headers,
      { version: "v1.0.0", notes: "Strict event codec", commit_id: providerV1 },
    );
    await json(
      providerPage,
      "post",
      `/repositories/${providerRepo.id}/releases/${releaseV1.id}/builds`,
      provider.headers,
      {},
    );
    const providerBuilds = await eventually(
      () =>
        json(
          providerPage,
          "get",
          `/repositories/${providerRepo.id}/releases/${releaseV1.id}/builds`,
          provider.headers,
        ),
      (x: any) =>
        x.builds?.length && x.builds.every((b: any) => b.state === "succeeded"),
      "provider v1 builds",
    );
    const providerPackage = await json(
      providerPage,
      "post",
      `/repositories/${providerRepo.id}/releases/${releaseV1.id}/packages`,
      provider.headers,
      {
        name: packageName,
        version: "1.0.0",
        build_id: providerBuilds.builds[0].id,
        artifact_id: providerBuilds.builds[0].artifacts[0].id,
        platform: { os: "linux", architecture: "amd64", runtime: "text/v1" },
        summary: "Strict event codec",
        documentation: "Install with a repository-scoped package token.",
        license: "MIT",
        support: "provider repository",
        visibility: "public",
        dependencies: [],
      },
    );

    // The adopter uses the package registry with a repository-scoped standard client.
    await adopterPage.goto("/packages");
    await adopterPage
      .getByPlaceholder("Name, purpose, or documentation")
      .fill(packageName);
    await adopterPage.getByText("Create isolated install credential").click();
    await adopterPage
      .getByLabel("Consuming repository ID")
      .fill(consumerRepo.id);
    await adopterPage
      .getByRole("button", { name: "Create one-hour token" })
      .click();
    const installToken = await adopterPage
      .locator("code")
      .filter({ hasText: /vvr_/ })
      .textContent();
    expect(installToken).toBeTruthy();
    const installed = await run("curl", [
      "--fail",
      "--silent",
      "--show-error",
      "-H",
      `Authorization: Bearer ${installToken}`,
      `http://localhost:3000/api/packages/${packageName}/versions/1.0.0/artifact`,
    ]);
    expect(installed.stdout).toContain("codec=1.0.0");

    const dimensions = [
      "capabilities",
      "provenance",
      "support",
      "security",
      "data_use",
      "compatibility",
      "known_gaps",
    ];
    let workspace = await json(
      adopterPage,
      "post",
      "/adoption-workspaces",
      adopter.headers,
      {
        title: "Adopt exact event decoding",
        outcome:
          "Accept partner events without losing invalid-field diagnostics.",
        source: {
          kind: "package",
          repository_id: providerRepo.id,
          resource_id: `${packageName}@1.0.0`,
          label: "Released provider package",
        },
        required_journeys: ["decode partner event"],
        environments: ["Linux production"],
        constraints: ["No shared credentials", "Preserve invalid field names"],
        budget_cents: 5000,
        currency: "USD",
        owners: [
          {
            principal_id: adopter.user.id,
            responsibility: "Adoption, cost, and production outcome",
          },
        ],
        evaluation_criteria: [
          {
            name: "Diagnostic fit",
            requirement: "Invalid field is attributable",
            weight: 60,
            owner_id: adopter.user.id,
          },
          {
            name: "Operating fit",
            requirement: "Exact release stays within budget",
            weight: 40,
            owner_id: adopter.user.id,
          },
        ],
        candidates: [
          {
            name: "Fast codec",
            provider: "Unavailable vendor",
            version: "9.0.0",
            source_kind: "package",
            source_reference: "vendor/fast-codec",
            evidence: [
              {
                dimension: "capabilities",
                summary: "Benchmark only; intended evidence is private.",
                reference: "private benchmark",
                repository_id: "0".repeat(32),
                observed_version: "8.0.0",
                visibility: "participants",
              },
            ],
          },
          {
            name: packageName,
            provider: "Provider Lab",
            version: "1.0.0",
            revision: providerV1,
            source_kind: "package",
            source_reference: `${packageName}@1.0.0`,
            evidence: dimensions.map((dimension) => ({
              dimension,
              summary: `${dimension} evidence for the exact strict codec`,
              reference: `https://example.test/${packageName}/${dimension}`,
              observed_version: "1.0.0",
              visibility: "public",
            })),
          },
        ],
        invitations: [
          {
            principal_type: "human",
            principal_id: provider.user.id,
            role: "provider_maintainer",
          },
          {
            principal_type: "human",
            principal_id: targetUser.user.id,
            role: "affected_user",
          },
          {
            principal_type: "agent",
            principal_id: agent.id,
            organization_id: providerOrg.id,
            role: "observer",
          },
        ],
      },
    );
    expect(workspace.candidates[0]).toMatchObject({
      fit_status: "undetermined",
      evidence: [
        expect.objectContaining({
          resolution: "inaccessible",
          reference: "Restricted evidence",
        }),
      ],
    });
    expect(workspace.candidates[1].fit_status).toBe("evidence_complete");
    await rejected(
      await providerPage.request.get(
        `/api/adoption-workspaces/${workspace.id}`,
        { headers: provider.headers },
      ),
      404,
    );
    let pending = await json(
      providerPage,
      "get",
      "/adoption-workspaces/invitations/pending",
      provider.headers,
    );
    workspace = await json(
      providerPage,
      "post",
      `/adoption-workspaces/${workspace.id}/invitations/${pending.invitations[0].invitation.id}/consent`,
      provider.headers,
      {
        decision: "accepted",
        expected_version: pending.invitations[0].version,
      },
    );
    pending = await json(
      userPage,
      "get",
      "/adoption-workspaces/invitations/pending",
      targetUser.headers,
    );
    workspace = await json(
      userPage,
      "post",
      `/adoption-workspaces/${workspace.id}/invitations/${pending.invitations[0].invitation.id}/consent`,
      targetUser.headers,
      {
        decision: "accepted",
        expected_version: pending.invitations[0].version,
      },
    );

    const selected = workspace.candidates[1];
    workspace = await json(
      adopterPage,
      "post",
      `/adoption-workspaces/${workspace.id}/trials`,
      adopter.headers,
      {
        expected_version: workspace.version,
        candidate_id: selected.id,
        source: {
          kind: "attested_release",
          repository_id: providerRepo.id,
          resource_id: releaseV1.id,
          revision: providerV1,
        },
        packages: [packageName],
        apis: [],
        data_kind: "synthetic",
        data_description: "Generated valid and invalid partner events",
        journeys: ["decode partner event"],
        policies: ["No production data", "No credential retention"],
        setup: ["Download through scoped package client"],
        configuration: ["strict mode"],
        commands: ["decode valid.json", "decode invalid.json"],
        integration_changes: ["temporary diagnostic adapter"],
        maximum_cost_cents: 500,
      },
    );
    const trial = workspace.trials[0];
    const leaked = await adopterPage.request.post(
      `/api/adoption-workspaces/${workspace.id}/trials/${trial.id}/attempts`,
      {
        headers: adopter.headers,
        data: {
          expected_version: workspace.version,
          status: "failed",
          reproducible: true,
          checks: ["Authorization: Bearer leaked"],
          previews: [],
          measurements: [],
          cost_cents: 5,
          findings: ["credential leaked"],
          user_feedback: [],
          artifact_digests: [],
        },
      },
    );
    await rejected(leaked, 422);
    workspace = await json(
      adopterPage,
      "post",
      `/adoption-workspaces/${workspace.id}/trials/${trial.id}/attempts`,
      adopter.headers,
      {
        expected_version: workspace.version,
        status: "failed",
        reproducible: true,
        checks: ["valid event decoded", "invalid field name missing"],
        previews: ["synthetic CLI"],
        measurements: ["42ms"],
        cost_cents: 40,
        findings: ["strict error omits the invalid field"],
        user_feedback: ["owner cannot diagnose partner payload"],
        artifact_digests: ["a".repeat(64)],
      },
    );
    const access = await json(
      providerPage,
      "post",
      `/organizations/${providerOrg.id}/access-requests`,
      provider.headers,
      {
        principal_type: "agent",
        principal_id: agent.id,
        role: "viewer",
        resources: [{ kind: "repository", id: providerRepo.id }],
        exceptions: [],
        reason: "Reproduce only the synthetic package trial.",
        expires_at: new Date(Date.now() + 3_600_000).toISOString(),
      },
    );
    org = await json(
      providerPage,
      "post",
      `/organizations/${providerOrg.id}/access-requests/${access.access_requests.at(-1).id}/decision`,
      provider.headers,
      { decision: "approve" },
    );
    const grant = org.access_grants.find(
      (x: any) => x.principal_id === agent.id,
    );
    const agentCredential = await json(
      providerPage,
      "post",
      `/organizations/${providerOrg.id}/access-grants/${grant.id}/credentials`,
      provider.headers,
      {
        agent_id: agent.id,
        repository_id: providerRepo.id,
        expires_in: 3600,
        purpose: "api_read",
      },
    );
    workspace = await json(
      providerPage,
      "post",
      `/adoption-workspaces/${workspace.id}/trials/${trial.id}/attempts`,
      { Authorization: `Bearer ${agentCredential.token}` },
      {
        expected_version: workspace.version,
        status: "passed",
        reproducible: true,
        checks: ["synthetic adapter reports invalid field"],
        previews: ["isolated agent CLI"],
        measurements: ["46ms"],
        cost_cents: 45,
        findings: ["fit depends on temporary consumer adapter"],
        user_feedback: [],
        artifact_digests: ["c".repeat(64)],
      },
    );
    workspace = await json(
      userPage,
      "post",
      `/adoption-workspaces/${workspace.id}/trials/${trial.id}/attempts`,
      targetUser.headers,
      {
        expected_version: workspace.version,
        status: "passed",
        reproducible: true,
        checks: [
          "valid event decoded",
          "temporary adapter reports invalid field",
        ],
        previews: ["target-user CLI"],
        measurements: ["48ms", "diagnosis under one minute"],
        cost_cents: 50,
        findings: ["provider gap remains behind workaround"],
        user_feedback: ["invalid field is now actionable"],
        artifact_digests: ["b".repeat(64)],
      },
    );
    const reproducedAttempt = workspace.trials[0].attempts.at(-1);

    // A denied exception remains visible; the operating boundary instead orders ordinary consumer, agent, environment, documentation, and fork work.
    await writeFile(
      join(consumerCopy, ".vivarium", "checks.json"),
      JSON.stringify({
        version: 1,
        checks: [
          {
            name: "adoption journey",
            image: "alpine:3.22",
            command: "grep -q 'diagnostic=local-workaround' integration.txt",
          },
        ],
      }),
    );
    await writeFile(
      join(consumerCopy, ".vivarium", "release.json"),
      JSON.stringify({
        version: 1,
        steps: [
          {
            name: "consumer",
            image: "alpine:3.22",
            command: 'cp integration.txt "$VIVARIUM_OUTPUT/integration.txt"',
          },
        ],
      }),
    );
    await writeFile(
      join(consumerCopy, ".vivarium", "deployment.json"),
      JSON.stringify({
        version: 1,
        stages: [
          {
            name: "staged",
            signals: [
              {
                name: "adopted event diagnostics",
                command: "grep -q 'health=ready' \"$VIVARIUM_ARTIFACT\"",
              },
            ],
          },
        ],
      }),
    );
    await writeFile(
      join(consumerCopy, ".vivarium", "packages.json"),
      JSON.stringify({
        version: 1,
        dependencies: [{ name: packageName, constraint: "=1.0.0" }],
        lock: [{ name: packageName, version: "1.0.0" }],
      }),
    );
    await writeFile(
      join(consumerCopy, "integration.txt"),
      "package=1.0.0\ndiagnostic=local-workaround\nhealth=ready\n",
    );
    await git(consumerCopy, "add", ".");
    await git(consumerCopy, "commit", "-m", "Prepare consumer adoption base");
    await authenticatedGit(consumerGit.token, consumerCopy, "push", "origin", "main");
    const environment = await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/environments`,
      adopter.headers,
      {
        name: "production",
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
    workspace = await json(
      adopterPage,
      "post",
      `/adoption-workspaces/${workspace.id}/plans`,
      adopter.headers,
      {
        expected_version: workspace.version,
        candidate_id: selected.id,
        trial_id: trial.id,
        selected_version: "1.0.0",
        integration_architecture:
          "Pinned package behind a consumer-owned diagnostic adapter",
        configuration_ownership: [
          {
            decision: "Package pin and rollout",
            owner_id: adopter.user.id,
            party: "adopter",
          },
          {
            decision: "Codec compatibility",
            owner_id: provider.user.id,
            party: "provider",
          },
        ],
        update_policy: "Exact versions after target-user trial",
        support_policy: "Consumer owns integration; provider owns codec",
        service_boundaries: ["Package executes inside consumer process"],
        data_boundaries: [
          "Synthetic trial data; partner payload stays consumer-side",
        ],
        required_exceptions: [
          "Requested shared provider credential denied; scoped tokens only",
        ],
        exit_strategy: "Remove the package and adapter",
        unresolved_fit_gaps: ["Provider diagnostics omit invalid field"],
        compatibility_promises: ["1.x strict decoding remains stable"],
        recurring_cost_cents: 250,
        currency: "USD",
        work: [
          {
            position: 1,
            kind: "consumer_repository",
            title: "Integrate pinned codec",
            repository_id: consumerRepo.id,
            paths: ["integration.txt", ".vivarium/packages.json"],
            owner_type: "human",
            owner_id: adopter.user.id,
            acceptance_criteria: ["reviewed pull passes"],
          },
          {
            position: 2,
            kind: "environment",
            title: "Stage exact release",
            repository_id: consumerRepo.id,
            environment_id: environment.id,
            paths: [".vivarium/deployment.json"],
            owner_type: "human",
            owner_id: adopter.user.id,
            acceptance_criteria: ["health passes"],
          },
          {
            position: 3,
            kind: "documentation",
            title: "Document support boundary",
            repository_id: consumerRepo.id,
            paths: ["integration.txt"],
            owner_type: "human",
            owner_id: targetUser.user.id,
            acceptance_criteria: ["target user accepts"],
          },
        ],
      },
    );
    const plan = workspace.plans[0];
    expect(plan.required_exceptions).toContain(
      "Requested shared provider credential denied; scoped tokens only",
    );
    expect(
      plan.work.every((x: any) => x.authority === "no_authority_granted"),
    ).toBe(true);

    await git(consumerCopy, "switch", "-c", "adopt-v1");
    await writeFile(
      join(consumerCopy, "integration.txt"),
      "package=1.0.0\ndiagnostic=local-workaround\nhealth=ready\nuser=accepted\n",
    );
    await git(consumerCopy, "add", ".");
    await git(
      consumerCopy,
      "commit",
      "-m",
      "Adopt exact codec with temporary workaround",
    );
    await authenticatedGit(consumerGit.token, consumerCopy, "push", "origin", "adopt-v1");
    const integrationPull = await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/pulls`,
      adopter.headers,
      {
        title: "Adopt exact codec",
        body: "Pinned integration with target-user reproduced workaround.",
        source_branch: "adopt-v1",
        target_branch: "main",
      },
    );
    await json(
      adopterPage,
      "put",
      `/repositories/${consumerRepo.id}/branches/main/required-checks`,
      adopter.headers,
      { checks: ["adoption journey"] },
    );
    await eventually(
      () =>
        json(
          adopterPage,
          "get",
          `/repositories/${consumerRepo.id}/pulls/${integrationPull.id}/checks`,
          adopter.headers,
        ),
      (x: any) => x.check_runs?.some((c: any) => c.state === "succeeded"),
      "consumer adoption checks pass",
    );
    await json(
      userPage,
      "post",
      `/repositories/${consumerRepo.id}/pulls/${integrationPull.id}/reviews`,
      targetUser.headers,
      { decision: "approved" },
    );
    const mergedV1 = await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/pulls/${integrationPull.id}/merge`,
      adopter.headers,
      {},
    );
    await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/dependency-inventories`,
      adopter.headers,
      { commit_id: mergedV1.merge_commit_id },
    );
    const consumerReleaseV1 = await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/releases`,
      adopter.headers,
      {
        version: "v1.0.0",
        notes: "First exact codec adoption",
        commit_id: mergedV1.merge_commit_id,
      },
    );
    await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/releases/${consumerReleaseV1.id}/builds`,
      adopter.headers,
      {},
    );
    const consumerBuildV1 = (
      await eventually(
        () =>
          json(
            adopterPage,
            "get",
            `/repositories/${consumerRepo.id}/releases/${consumerReleaseV1.id}/builds`,
            adopter.headers,
          ),
        (x: any) =>
          x.builds?.length &&
          x.builds.every((b: any) => b.state === "succeeded"),
        "consumer v1 builds",
      )
    ).builds[0];
    const deploymentV1Request = await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/deployments`,
      adopter.headers,
      {
        environment_id: environment.id,
        release_id: consumerReleaseV1.id,
        build_id: consumerBuildV1.id,
        artifact_id: consumerBuildV1.artifacts[0].id,
      },
    );
    const deploymentsV1 = await eventually(
      () =>
        json(
          adopterPage,
          "get",
          `/repositories/${consumerRepo.id}/deployments`,
          adopter.headers,
        ),
      (x: any) =>
        x.deployments?.some(
          (d: any) =>
            d.id === deploymentV1Request.id && d.state === "succeeded",
        ),
      "initial adoption deploys",
    );
    const deploymentV1 = deploymentsV1.deployments.find(
      (d: any) => d.id === deploymentV1Request.id,
    );
    const attestations = [
      "policy",
      "rehearsal",
      "support",
      "user_acceptance",
      "cost",
    ].map((kind) => ({
      kind,
      statement: `${kind} reviewed for the exact release`,
      satisfied: true,
      attested_by: adopter.user.id,
    }));
    workspace = await json(
      adopterPage,
      "post",
      `/adoption-workspaces/${workspace.id}/deliveries`,
      adopter.headers,
      {
        expected_version: workspace.version,
        plan_id: plan.id,
        consumer_repository_id: consumerRepo.id,
        pull_request_id: integrationPull.id,
        release_id: consumerReleaseV1.id,
        deployment_id: deploymentV1.id,
        cost_cents: 250,
        currency: "USD",
        support_readiness: "Provider and consumer repository paths tested",
        user_acceptance: "Target user diagnosed the invalid field",
        attestations,
      },
    );
    const operating = workspace.deliveries.at(-1);
    expect(operating).toMatchObject({
      state: "operating",
      provider_revision: providerV1,
      release_revision: mergedV1.merge_commit_id,
    });

    await git(consumerCopy, "switch", "main");
    await authenticatedGit(consumerGit.token, consumerCopy, "pull", "--ff-only", "origin", "main");
    await writeFile(
      join(consumerCopy, "integration.txt"),
      "package=1.0.0\ndiagnostic=local-workaround\nhealth=regressed\nuser=accepted\n",
    );
    await git(consumerCopy, "add", "integration.txt");
    await git(
      consumerCopy,
      "commit",
      "-m",
      "Retain observed version regression",
    );
    await authenticatedGit(consumerGit.token, consumerCopy, "push", "origin", "main");
    const regressedRevision = await git(consumerCopy, "rev-parse", "HEAD");
    await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/dependency-inventories`,
      adopter.headers,
      { commit_id: regressedRevision },
    );
    const regressedRelease = await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/releases`,
      adopter.headers,
      {
        version: "v1.0.1",
        notes: "Version regression retained for correction",
        commit_id: regressedRevision,
        previous_release_id: consumerReleaseV1.id,
      },
    );
    await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/releases/${regressedRelease.id}/builds`,
      adopter.headers,
      {},
    );
    const regressedBuild = (
      await eventually(
        () =>
          json(
            adopterPage,
            "get",
            `/repositories/${consumerRepo.id}/releases/${regressedRelease.id}/builds`,
            adopter.headers,
          ),
        (x: any) =>
          x.builds?.length &&
          x.builds.every((b: any) => b.state === "succeeded"),
        "regressed release builds",
      )
    ).builds[0];
    const failedRequest = await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/deployments`,
      adopter.headers,
      {
        environment_id: environment.id,
        release_id: regressedRelease.id,
        build_id: regressedBuild.id,
        artifact_id: regressedBuild.artifacts[0].id,
      },
    );
    const failedRollout = (
      await eventually(
        () =>
          json(
            adopterPage,
            "get",
            `/repositories/${consumerRepo.id}/deployments`,
            adopter.headers,
          ),
        (x: any) =>
          x.deployments?.some(
            (d: any) => d.id === failedRequest.id && d.state === "failed",
          ),
        "version regression fails rollout",
      )
    ).deployments.find((d: any) => d.id === failedRequest.id);
    expect(failedRollout.evidence).toContainEqual(
      expect.objectContaining({
        state: "failed",
        signal: "adopted event diagnostics",
      }),
    );

    // Provider rejection and outage preserve private/local paths; the consented reproduction proceeds through an ordinary fork pull.
    workspace = await json(
      adopterPage,
      "post",
      `/adoption-workspaces/${workspace.id}/shared-findings`,
      adopter.headers,
      {
        expected_version: workspace.version,
        kind: "support_question",
        trial_id: trial.id,
        attempt_id: reproducedAttempt.id,
        summary: "Can provider support inspect the omitted field?",
        reproduction: ["Decode synthetic invalid event"],
        evidence: ["Field absent"],
        redactions: ["Removed partner identifiers"],
        visibility: "provider",
        state: "pending_consent",
      },
    );
    const rejectedFinding = workspace.shared_findings.at(-1);
    workspace = await json(
      providerPage,
      "post",
      `/adoption-workspaces/${workspace.id}/shared-findings/${rejectedFinding.id}/consent`,
      provider.headers,
      { expected_version: workspace.version, decision: "rejected" },
    );
    expect(workspace.shared_findings.at(-1)).toMatchObject({
      state: "local_only",
      provider_status: "rejected",
    });
    workspace = await json(
      adopterPage,
      "post",
      `/adoption-workspaces/${workspace.id}/shared-findings`,
      adopter.headers,
      {
        expected_version: workspace.version,
        kind: "reproduction",
        trial_id: trial.id,
        attempt_id: reproducedAttempt.id,
        delivery_id: operating.id,
        summary: "Strict diagnostics omit the invalid field name",
        reproduction: ["Decode synthetic {bad_field:true}"],
        evidence: ["Target user required the local adapter"],
        redactions: ["Synthetic payload only", "No credentials"],
        visibility: "public",
        state: "pending_consent",
      },
    );
    const finding = workspace.shared_findings.at(-1);
    const unavailable = await adopterPage.request.post(
      `/api/adoption-workspaces/${workspace.id}/upstream-contributions`,
      {
        headers: adopter.headers,
        data: {
          expected_version: workspace.version,
          finding_id: finding.id,
          kind: "fork_pull",
          target_repository_id: providerRepo.id,
          resource_id: "provider-offline",
          resolution: "Provider unavailable; retain local workaround",
        },
      },
    );
    await rejected(unavailable, 422);
    workspace = await json(
      providerPage,
      "post",
      `/adoption-workspaces/${workspace.id}/shared-findings/${finding.id}/consent`,
      provider.headers,
      { expected_version: workspace.version, decision: "accepted" },
    );
    const fork = await json(
      adopterPage,
      "post",
      `/repositories/${providerRepo.id}/forks`,
      adopter.headers,
      { name: `codec-contribution-${suffix}` },
    );
    const forkCopy = await temporary("vivarium-adoption-fork-");
    await authenticatedGit(
      consumerGit.token,
      tmpdir(),
      "clone",
      `http://localhost:3000/git/${fork.id}.git`,
      forkCopy,
    );
    await git(forkCopy, "config", "user.name", "Consumer Owner");
    await git(forkCopy, "config", "user.email", "consumer@example.test");
    await git(forkCopy, "switch", "-c", "diagnostic-field");
    await writeFile(
      join(forkCopy, "codec.txt"),
      "codec=1.1.0\nmode=strict\ndiagnostic=invalid-field\n",
    );
    await git(forkCopy, "add", "codec.txt");
    await git(
      forkCopy,
      "commit",
      "-m",
      "Report invalid field in strict diagnostics",
    );
    await authenticatedGit(consumerGit.token, forkCopy, "push", "origin", "diagnostic-field");
    const upstreamPull = await json(
      adopterPage,
      "post",
      `/repositories/${providerRepo.id}/pulls`,
      adopter.headers,
      {
        title: "Report invalid field in strict diagnostics",
        body: "Consumer-authored synthetic reproduction; no consumer data or credential.",
        source_repository_id: fork.id,
        source_branch: "diagnostic-field",
        target_branch: "main",
      },
    );
    workspace = await json(
      adopterPage,
      "post",
      `/adoption-workspaces/${workspace.id}/upstream-contributions`,
      adopter.headers,
      {
        expected_version: workspace.version,
        finding_id: finding.id,
        kind: "fork_pull",
        target_repository_id: providerRepo.id,
        resource_id: upstreamPull.id,
        resolution: "Provider review pending",
      },
    );
    await json(
      providerPage,
      "post",
      `/repositories/${providerRepo.id}/pulls/${upstreamPull.id}/reviews`,
      provider.headers,
      { decision: "approved" },
    );
    const providerMerged = await json(
      providerPage,
      "post",
      `/repositories/${providerRepo.id}/pulls/${upstreamPull.id}/merge`,
      provider.headers,
      {},
    );
    await authenticatedGit(providerGit.token, providerCopy, "pull", "--ff-only", "origin", "main");
    const releaseV11 = await json(
      providerPage,
      "post",
      `/repositories/${providerRepo.id}/releases`,
      provider.headers,
      {
        version: "v1.1.0",
        notes: "Consumer-authored diagnostic improvement",
        commit_id: providerMerged.merge_commit_id,
        previous_release_id: releaseV1.id,
      },
    );
    await json(
      providerPage,
      "post",
      `/repositories/${providerRepo.id}/releases/${releaseV11.id}/builds`,
      provider.headers,
      {},
    );
    const providerBuildV11 = (
      await eventually(
        () =>
          json(
            providerPage,
            "get",
            `/repositories/${providerRepo.id}/releases/${releaseV11.id}/builds`,
            provider.headers,
          ),
        (x: any) =>
          x.builds?.length &&
          x.builds.every((b: any) => b.state === "succeeded"),
        "provider v1.1 builds",
      )
    ).builds[0];
    await json(
      providerPage,
      "post",
      `/repositories/${providerRepo.id}/releases/${releaseV11.id}/packages`,
      provider.headers,
      {
        name: packageName,
        version: "1.1.0",
        build_id: providerBuildV11.id,
        artifact_id: providerBuildV11.artifacts[0].id,
        platform: providerPackage.platform,
        summary: "Diagnostics include invalid field",
        documentation: "Upgrade and remove local workaround.",
        license: "MIT",
        support: "provider repository",
        visibility: "public",
        dependencies: [],
      },
    );

    // A regressed consumer update fails rollout, then a corrected exact update replaces every local-patch path.
    await git(consumerCopy, "switch", "main");
    await authenticatedGit(consumerGit.token, consumerCopy, "pull", "--ff-only", "origin", "main");
    await git(consumerCopy, "switch", "-c", "update-v11");
    await writeFile(
      join(consumerCopy, ".vivarium", "packages.json"),
      JSON.stringify({
        version: 1,
        dependencies: [{ name: packageName, constraint: "=1.1.0" }],
        lock: [{ name: packageName, version: "1.1.0" }],
      }),
    );
    await writeFile(
      join(consumerCopy, ".vivarium", "checks.json"),
      JSON.stringify({
        version: 1,
        checks: [
          {
            name: "adoption journey",
            image: "alpine:3.22",
            command: "grep -q 'diagnostic=provider-native' integration.txt",
          },
        ],
      }),
    );
    await writeFile(
      join(consumerCopy, "integration.txt"),
      "package=1.1.0\ndiagnostic=provider-native\nhealth=regressed\nuser=pending\n",
    );
    await git(consumerCopy, "add", ".");
    await git(
      consumerCopy,
      "commit",
      "-m",
      "Replace local adapter with provider 1.1",
    );
    await authenticatedGit(consumerGit.token, consumerCopy, "push", "origin", "update-v11");
    const updatePull = await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/pulls`,
      adopter.headers,
      {
        title: "Replace local adapter with provider 1.1",
        body: "Exact upstream replacement and package pin.",
        source_branch: "update-v11",
        target_branch: "main",
      },
    );
    // Correct the version regression on the same reviewed pull before merge.
    await writeFile(
      join(consumerCopy, "integration.txt"),
      "package=1.1.0\ndiagnostic=provider-native\nhealth=ready\nuser=accepted\n",
    );
    await git(consumerCopy, "add", "integration.txt");
    await git(consumerCopy, "commit", "-m", "Correct 1.1 rollout regression");
    await authenticatedGit(consumerGit.token, consumerCopy, "push", "origin", "update-v11");
    const correctedRevision = await git(consumerCopy, "rev-parse", "HEAD");
    await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/pulls/${updatePull.id}/synchronize`,
      adopter.headers,
      {},
    );
    await eventually(
      () =>
        json(
          adopterPage,
          "get",
          `/repositories/${consumerRepo.id}/pulls/${updatePull.id}/checks`,
          adopter.headers,
        ),
      (x: any) =>
        x.check_runs?.some(
          (c: any) =>
            c.commit_id === correctedRevision && c.state === "succeeded",
        ),
      "corrected update checks pass",
    );
    await json(
      userPage,
      "post",
      `/repositories/${consumerRepo.id}/pulls/${updatePull.id}/reviews`,
      targetUser.headers,
      { decision: "approved" },
    );
    const updateMerged = await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/pulls/${updatePull.id}/merge`,
      adopter.headers,
      {},
    );
    await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/dependency-inventories`,
      adopter.headers,
      { commit_id: updateMerged.merge_commit_id },
    );
    const consumerReleaseV11 = await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/releases`,
      adopter.headers,
      {
        version: "v1.1.0",
        notes: "Provider-native diagnostics",
        commit_id: updateMerged.merge_commit_id,
        previous_release_id: consumerReleaseV1.id,
      },
    );
    await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/releases/${consumerReleaseV11.id}/builds`,
      adopter.headers,
      {},
    );
    const consumerBuildV11 = (
      await eventually(
        () =>
          json(
            adopterPage,
            "get",
            `/repositories/${consumerRepo.id}/releases/${consumerReleaseV11.id}/builds`,
            adopter.headers,
          ),
        (x: any) =>
          x.builds?.length &&
          x.builds.every((b: any) => b.state === "succeeded"),
        "consumer v1.1 builds",
      )
    ).builds[0];
    const deploymentV11Request = await json(
      adopterPage,
      "post",
      `/repositories/${consumerRepo.id}/deployments`,
      adopter.headers,
      {
        environment_id: environment.id,
        release_id: consumerReleaseV11.id,
        build_id: consumerBuildV11.id,
        artifact_id: consumerBuildV11.artifacts[0].id,
      },
    );
    const deploymentV11 = (
      await eventually(
        () =>
          json(
            adopterPage,
            "get",
            `/repositories/${consumerRepo.id}/deployments`,
            adopter.headers,
          ),
        (x: any) =>
          x.deployments?.some(
            (d: any) =>
              d.id === deploymentV11Request.id && d.state === "succeeded",
          ),
        "corrected update deploys",
      )
    ).deployments.find((d: any) => d.id === deploymentV11Request.id);
    workspace = await json(
      adopterPage,
      "get",
      `/adoption-workspaces/${workspace.id}`,
      adopter.headers,
    );
    const durableUpstream = workspace.upstream_contributions.find(
      (x: any) => x.resource_id === upstreamPull.id,
    );
    durableUpstream.status = "merged"; // The route re-resolves current pull state on the next retained link.
    workspace = await json(
      adopterPage,
      "post",
      `/adoption-workspaces/${workspace.id}/upstream-contributions`,
      adopter.headers,
      {
        expected_version: workspace.version,
        finding_id: finding.id,
        kind: "fork_pull",
        target_repository_id: providerRepo.id,
        resource_id: upstreamPull.id,
        resolution: "Merged and released as 1.1.0",
      },
    );
    const mergedContribution = workspace.upstream_contributions.at(-1);
    workspace = await json(
      adopterPage,
      "post",
      `/adoption-workspaces/${workspace.id}/verified-updates`,
      adopter.headers,
      {
        expected_version: workspace.version,
        contribution_id: mergedContribution.id,
        provider_repository_id: providerRepo.id,
        provider_release_id: releaseV11.id,
        consumer_repository_id: consumerRepo.id,
        consumer_pull_request_id: updatePull.id,
        consumer_release_id: consumerReleaseV11.id,
        consumer_deployment_id: deploymentV11.id,
        outcome:
          "Target users diagnose invalid partner fields without the temporary adapter",
      },
    );
    expect(workspace.verified_updates.at(-1)).toMatchObject({
      state: "verified",
      package_name: packageName,
      package_version: "1.1.0",
      outcome:
        "Target users diagnose invalid partner fields without the temporary adapter",
    });
    await adopterPage.goto("/adoption");
    await expect(
      adopterPage.getByRole("heading", { name: "Software adoption" }),
    ).toBeVisible();
    await expect(
      adopterPage.getByText("Return adoption knowledge upstream", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      adopterPage.getByText(
        "Target users diagnose invalid partner fields without the temporary adapter",
        { exact: false },
      ),
    ).toBeVisible();
  } finally {
    for (const credential of gitCredentials) {
      const response = await credential.page.request.delete(
        `/api/auth/credentials/${credential.id}`,
        { headers: credential.headers },
      );
      expect(response.status(), await response.text()).toBe(204);
    }
    await Promise.all(
      copies.map((path) => rm(path, { recursive: true, force: true })),
    );
  }
});
