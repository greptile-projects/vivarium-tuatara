import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- one connected reliability evidence trail */
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
  return {
    headers,
    user: (await (
      await page.request.get("/api/user", { headers })
    ).json()) as any,
  };
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
async function rejected(
  page: Page,
  path: string,
  headers: Record<string, string>,
  data: unknown,
) {
  const response = await page.request.post(`/api${path}`, { headers, data });
  expect(
    response.status(),
    `POST ${path}: ${await response.text()}`,
  ).toBeGreaterThanOrEqual(400);
  return response;
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

test("a team contains reliability burn and delivers a verified human-agent repair", async ({
  browser,
}) => {
  test.setTimeout(360_000);
  const docker = await run("docker", ["info"]).then(
    () => true,
    () => false,
  );
  test.skip(!docker, "release and staged-deployment evidence requires Docker");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() =>
    run("docker", ["pull", "alpine:3.22"]),
  );
  const copies: string[] = [];
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const dependencyPage = await (await browser.newContext()).newPage();
    const reviewerPage = await (await browser.newContext()).newPage();
    const owner = await account(
      ownerPage,
      "Journey Service Owner",
      `reliability-owner-${suffix}`,
    );
    const dependency = await account(
      dependencyPage,
      "Payments Owner",
      `payments-owner-${suffix}`,
    );
    const reviewer = await account(
      reviewerPage,
      "Reliability Reviewer",
      `reliability-reviewer-${suffix}`,
    );

    const organization = await json(
      ownerPage,
      "post",
      "/organizations",
      owner.headers,
      { name: `Dependable Checkout ${suffix}`, slug: `dependable-${suffix}` },
    );
    const repository = await json(
      ownerPage,
      "post",
      `/organizations/${organization.id}/repositories`,
      owner.headers,
      { name: `checkout-${suffix}` },
    );
    for (const person of [dependency, reviewer])
      await json(
        ownerPage,
        "post",
        `/repositories/${repository.id}/collaborators`,
        owner.headers,
        { user_id: person.user.id },
      );

    let group = await json(
      ownerPage,
      "post",
      `/organizations/${organization.id}/agents`,
      owner.headers,
      {
        name: "Reliability Investigator",
        slug: `reliability-investigator-${suffix}`,
        capabilities: ["analyze bounded revision-exact operational evidence"],
        operator_ids: [owner.user.id],
        team_ids: [],
      },
    );
    const investigator = group.agents.at(-1);
    group = await json(
      ownerPage,
      "post",
      `/organizations/${organization.id}/access-requests`,
      owner.headers,
      {
        principal_type: "agent",
        principal_id: investigator.id,
        role: "viewer",
        resources: [{ kind: "repository", id: repository.id }],
        exceptions: [],
        reason: "Investigate only the retained checkout reliability evidence",
        expires_at: new Date(Date.now() + 3_600_000).toISOString(),
      },
    );
    const accessRequest = group.access_requests.at(-1);
    group = await json(
      ownerPage,
      "post",
      `/organizations/${organization.id}/access-requests/${accessRequest.id}/decision`,
      owner.headers,
      { decision: "approve" },
    );
    const grant = group.access_grants.at(-1);
    const issued = await json(
      ownerPage,
      "post",
      `/organizations/${organization.id}/access-grants/${grant.id}/credentials`,
      owner.headers,
      {
        agent_id: investigator.id,
        repository_id: repository.id,
        expires_in: 3600,
        purpose: "api_read",
      },
    );
    const investigatorHeaders = { Authorization: `Bearer ${issued.token}` };

    const gitCredential = await json(
      ownerPage,
      "post",
      "/auth/credentials",
      owner.headers,
      {
        kind: "git",
        name: "reliability journey",
        scopes: ["git:read", "git:write"],
        expires_in: 3600,
      },
    );
    const copy = await mkdtemp(join(tmpdir(), "vivarium-reliability-"));
    copies.push(copy);
    await git(
      tmpdir(),
      "clone",
      `http://git:${gitCredential.token}@localhost:3000/git/${repository.id}.git`,
      copy,
    );
    await git(copy, "config", "user.name", "Journey Service Owner");
    await git(copy, "config", "user.email", "owner@example.test");
    await mkdir(join(copy, ".vivarium"));
    await writeFile(
      join(copy, ".vivarium", "checks.json"),
      JSON.stringify({
        version: 1,
        checks: [
          {
            name: "checkout contract",
            image: "alpine:3.22",
            command: "grep -q reliable service.txt",
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
            command: 'cp service.txt "$VIVARIUM_OUTPUT/service.txt"',
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
            name: "canary",
            signals: [
              {
                name: "artifact readable",
                command: 'test -r "$VIVARIUM_ARTIFACT"',
              },
            ],
          },
          {
            name: "full",
            signals: [
              {
                name: "artifact retained",
                command: 'test -r "$VIVARIUM_ARTIFACT"',
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
    await writeFile(join(copy, "service.txt"), "reliable checkout baseline\n");
    await git(copy, "add", ".");
    await git(copy, "commit", "-m", "Establish released checkout journey");
    await git(copy, "push", "origin", "main");
    const baselineRevision = await git(copy, "rev-parse", "HEAD");

    const environment = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/environments`,
      owner.headers,
      {
        name: "production",
        position: 1,
        image: "alpine:3.22",
        command: 'test -r "$VIVARIUM_ARTIFACT"',
        timeout_seconds: 30,
        configuration: {},
        credentials: {},
        required_approvals: 0,
        concurrency: 1,
      },
    );
    const releaseAndDeploy = async (
      version: string,
      notes: string,
      revision: string,
    ) => {
      const release = await json(
        ownerPage,
        "post",
        `/repositories/${repository.id}/releases`,
        owner.headers,
        { version, notes, commit_id: revision },
      );
      await json(
        ownerPage,
        "post",
        `/repositories/${repository.id}/dependency-inventories`,
        owner.headers,
        { commit_id: revision },
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
        `${version} build succeeds`,
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
        `${version} staged deployment succeeds`,
      );
      return {
        release,
        deployment: deployments.deployments.find(
          (d: any) => d.id === pending.id,
        ),
      };
    };
    const baselineDelivery = await releaseAndDeploy(
      `v1.0.0-${suffix}`,
      "Released checkout journey baseline.",
      baselineRevision,
    );

    const revision = {
      title: "Released checkout dependability",
      summary:
        "People can complete the released checkout journey without retry storms.",
      scopes: [
        {
          kind: "release",
          resource_id: baselineDelivery.release.id,
          name: "Released checkout",
        },
        {
          kind: "environment",
          resource_id: environment.id,
          name: "Production",
        },
      ],
      indicators: [
        {
          id: "success",
          name: "Successful checkout",
          description: "Eligible checkouts that complete",
          signal: "checkout.completed",
          calculation: "ratio",
          unit: "percent",
          good_event: "completed",
          total_event: "started",
        },
      ],
      measurement_windows: [
        { id: "hour", name: "Rolling hour", duration: "1h", rolling: true },
      ],
      user_journeys: [
        {
          id: "checkout",
          name: "Complete checkout",
          description: "Submit an order through payment confirmation",
          owner_ids: [owner.user.id],
        },
      ],
      objectives: [
        {
          id: "availability",
          name: "Checkout availability",
          indicator_id: "success",
          window_id: "hour",
          target: 99.9,
          comparator: "at_least",
          journey_ids: ["checkout"],
          owner_ids: [owner.user.id],
        },
      ],
      dependencies: [
        {
          id: "payments",
          name: "Payments authorization",
          kind: "service",
          owner_ids: [dependency.user.id],
          objective_ids: ["availability"],
        },
      ],
      error_budgets: [
        {
          objective_id: "availability",
          allowed_failure: 0.1,
          unit: "percent",
          burn_policy: "Pause rollout and investigate",
        },
      ],
      severity_thresholds: [
        {
          level: "warning",
          budget_consumed_percent: 50,
          response: "Investigate",
          owner_ids: [owner.user.id],
        },
        {
          level: "critical",
          budget_consumed_percent: 100,
          response: "Contain rollout",
          owner_ids: [owner.user.id],
        },
      ],
      owner_ids: [owner.user.id, dependency.user.id],
      commitment_links: [
        { kind: "release", id: baselineDelivery.release.id, version: 1 },
      ],
      exception_policy: {
        maximum_duration: "24h",
        approval_owner_ids: [owner.user.id],
        follow_up_required: true,
      },
      exceptions: [],
      rationale: "Reliability is a shared released-product commitment.",
    };
    const contract = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives`,
      owner.headers,
      { revision },
    );
    let retained = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/signal-mappings`,
      owner.headers,
      {
        revision: {
          contract_version: 1,
          objective_id: "availability",
          instrumentation_revision: baselineRevision,
          sources: [
            {
              kind: "metric",
              name: "Checkout completion",
              reference: "metrics://checkout/completion",
              visibility: "participants",
              sanitization: "hourly aggregate counts without user identifiers",
            },
            {
              kind: "trace",
              name: "Payment critical path",
              reference: "traces://checkout/payment",
              visibility: "participants",
              sanitization:
                "sampled spans with account and payment fields removed",
            },
          ],
          calculation: "ratio",
          unit: "percent",
          rationale:
            "Bind sanitized telemetry to delivered checkout revisions.",
        },
      },
    );
    const mapping = retained.signal_mappings[0];
    const observed = async (
      software: any,
      good: number,
      total: number,
      start: number,
      end: number,
      summary: string,
      uncertainty = 1,
      gaps: any[] = [],
    ) => {
      retained = await json(
        ownerPage,
        "post",
        `/repositories/${repository.id}/service-objectives/${contract.id}/observations`,
        owner.headers,
        {
          observation: {
            mapping_id: mapping.id,
            mapping_version: 1,
            contract_version: 1,
            objective_id: "availability",
            window_start: new Date(Date.now() + start).toISOString(),
            window_end: new Date(Date.now() + end).toISOString(),
            good_events: good,
            total_events: total,
            uncertainty,
            gaps,
            software: [software],
            summary,
          },
        },
      );
      return retained.observations.at(-1);
    };
    const baseline = await observed(
      {
        kind: "release",
        id: baselineDelivery.release.id,
        revision: baselineRevision,
        label: "Known-good release",
      },
      9995,
      10000,
      -3_000_000,
      -2_400_000,
      "Released journey meets its objective.",
    );

    await writeFile(
      join(copy, "service.txt"),
      "degraded checkout retry storm\n",
    );
    await git(copy, "add", "service.txt");
    await git(copy, "commit", "-m", "Deploy unbounded payment retries");
    await git(copy, "push", "origin", "main");
    const degradedRevision = await git(copy, "rev-parse", "HEAD");
    const degradedDelivery = await releaseAndDeploy(
      `v1.1.0-${suffix}`,
      "Payment retry rollout.",
      degradedRevision,
    );
    const noisy = await observed(
      {
        kind: "deployment",
        id: degradedDelivery.deployment.id,
        revision: degradedRevision,
        label: "Production rollout",
      },
      994,
      1000,
      -2_300_000,
      -1_900_000,
      "Noisy partial cohort suggests burn but cannot support a decision.",
      35,
      [
        {
          kind: "sampling",
          detail: "Canary cohort is too small and regionally skewed",
        },
      ],
    );
    expect(noisy.target_met).toBe(false);
    const burned = await observed(
      {
        kind: "deployment",
        id: degradedDelivery.deployment.id,
        revision: degradedRevision,
        label: "Production rollout",
      },
      9900,
      10000,
      -1_800_000,
      -1_200_000,
      "Corrected full cohort confirms checkout error-budget burn.",
      2,
      [],
    );
    expect(burned.error_budget_consumed_percent).toBeGreaterThan(100);

    retained = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/delivery-policies`,
      owner.headers,
      {
        expected_version: 1,
        policy: {
          contract_version: 1,
          objective_ids: ["availability"],
          branches: ["main"],
          services: ["checkout"],
          environment_ids: [environment.id],
          journey_ids: ["checkout"],
          risk_classes: ["availability"],
          maximum_budget_consumed_percent: 100,
          maximum_predicted_budget_increase_percent: 10,
          require_current_evidence: true,
          require_dependencies: true,
          required_owner_ids: [owner.user.id],
          minimum_acknowledgements: 1,
          on_missing_evidence: "block",
          on_budget_exhausted: "pause",
          on_regression: "slow",
          on_dependency_failure: "pause",
          rationale: "Contain user harm before rollout continues.",
        },
      },
    );
    const policy = retained.delivery_policies[0];
    retained = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/reliability-impacts`,
      owner.headers,
      {
        impact: {
          policy_id: policy.id,
          policy_version: 1,
          kind: "deployment",
          resource_id: degradedDelivery.deployment.id,
          revision: degradedRevision,
          branch: "main",
          service: "checkout",
          environment_id: environment.id,
          journey_ids: ["checkout"],
          risk_classes: ["availability"],
          objective_impacts: [
            {
              objective_id: "availability",
              observation_id: burned.id,
              predicted_budget_increase_percent: 25,
              confidence: "high",
            },
          ],
          dependency_failures: ["payments objective evidence is missing"],
          summary:
            "Deployment burns checkout budget while payment dependency evidence is missing.",
        },
      },
    );
    const impact = retained.reliability_impacts[0];
    let readiness = await json(
      ownerPage,
      "get",
      `/repositories/${repository.id}/reliability-readiness/deployment/${degradedDelivery.deployment.id}?revision=${degradedRevision}&branch=main&service=checkout&environment=${environment.id}&journey=checkout&risk=availability`,
      owner.headers,
    );
    expect(readiness.evaluations[0]).toMatchObject({
      state: "blocked",
      effect: "pause",
    });
    expect(readiness.evaluations[0].blockers).toEqual(
      expect.arrayContaining([
        "error budget is exhausted",
        "dependency reliability has failed",
      ]),
    );
    await rejected(
      dependencyPage,
      `/repositories/${repository.id}/service-objectives/${contract.id}/reliability-impacts/${impact.id}/exceptions`,
      dependency.headers,
      {
        exception: {
          reason: "Continue despite missing dependency evidence",
          expires_at: new Date(Date.now() + 3_600_000).toISOString(),
          follow_up: "none",
        },
      },
    );
    await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/reliability-impacts/${impact.id}/acknowledgements`,
      owner.headers,
      {
        rationale:
          "Contain production while owners investigate; this acknowledgement grants no deployment authority.",
      },
    );

    let investigationState = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/investigations`,
      owner.headers,
      {
        contract_version: 1,
        objective_id: "availability",
        title: "Explain post-deployment checkout burn",
        trigger: {
          kind: "deployment",
          id: degradedDelivery.deployment.id,
          revision: degradedRevision,
        },
        baseline_observation_ids: [baseline.id],
        affected_observation_ids: [burned.id],
        journey_ids: ["checkout"],
        evidence: [
          {
            kind: "deployment",
            resource_id: degradedDelivery.deployment.id,
            revision: degradedRevision,
            label: "Staged retry rollout",
            visibility: "participants",
          },
        {
          kind: "trace",
          resource_id: burned.id,
          revision: `observation:${burned.id}`,
            label: "Sanitized payment critical path",
            visibility: "participants",
          },
          {
          kind: "dependent_service",
          resource_id: "payments",
          revision: "contract:1",
            label: "Payments objective ownership",
            visibility: "participants",
          },
        ],
      },
    );
    const investigation = investigationState.investigations[0];
    investigationState = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/investigations/${investigation.id}/input-requests`,
      owner.headers,
      {
        expected_version: investigation.version,
        request: {
          owner_id: dependency.user.id,
          dependency_id: "payments",
          question:
            "Did payment authorization retries saturate after this exact deployment?",
        },
      },
    );
    const input = investigationState.investigations[0].input_requests[0];
    investigationState = await json(
      dependencyPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/investigations/${investigation.id}/input-responses`,
      dependency.headers,
      {
        expected_version: investigationState.investigations[0].version,
        request: {
          id: input.id,
          response:
            "Yes. Missing regional dependency evidence hid retry amplification; the bounded trace confirms saturation.",
        },
      },
    );
    investigationState = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/investigations/${investigation.id}/findings`,
      investigatorHeaders,
      {
        expected_version: investigationState.investigations[0].version,
        finding: {
          kind: "hypothesis",
          statement:
            "The deployed unbounded payment retry path amplifies dependency saturation and checkout failure.",
          uncertainty:
            "The earlier noisy cohort is excluded; conclusion uses the corrected full cohort and owner response.",
          confidence: "high",
          citation_ids: [burned.id, degradedDelivery.deployment.id, "payments"],
        },
      },
    );
    const finding = investigationState.investigations[0].findings[0];
    investigationState = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/investigations/${investigation.id}/responses`,
      owner.headers,
      {
        expected_version: investigationState.investigations[0].version,
        response: {
          finding_id: finding.id,
          kind: "confirm",
          body: "Revision-exact telemetry and the affected owner agree; keep rollout contained.",
        },
      },
    );

    retained = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/improvements`,
      owner.headers,
      {
        contract_version: 1,
        objective_id: "availability",
        impact_id: impact.id,
        title: "Restore checkout reliability",
        body: "Repair the confirmed retry amplification without bypassing ordinary review.",
        baseline_observation_ids: [burned.id],
        affected_observation_ids: [burned.id],
        affected_revisions: [degradedRevision],
        dependency_context: [
          "payments owner confirmed retry saturation and missing regional evidence",
        ],
        evidence_ids: [burned.id],
        acceptance_criteria: [
          "checkout attainment is at least 99.9 percent",
          "payment retries are bounded and dependency evidence is present",
        ],
        tasks: [
          {
            title: "Bound payment retries",
            assignee_type: "agent",
            assignee_id: "",
            depends_on_previous: false,
          },
        ],
      },
    );
    const improvement = retained.improvements[0];
    const tasks = await json(
      ownerPage,
      "get",
      `/repositories/${repository.id}/proposals/${improvement.proposal_id}/tasks`,
      owner.headers,
    );
    const task = tasks.tasks[0];
    const launched = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/proposals/${improvement.proposal_id}/tasks/${task.id}/sessions`,
      owner.headers,
      {
        expected_assignment_id: task.assignment.id,
        context_paths: ["service.txt"],
        expires_in: 3600,
      },
    );
    const taskRunBase = `/repositories/${repository.id}/proposals/${improvement.proposal_id}/tasks/${task.id}/sessions/${launched.session.id}/runs/${launched.run.id}`;
    await json(
      ownerPage,
      "post",
      `${taskRunBase}/interventions`,
      owner.headers,
      {
        kind: "run.guidance",
        message:
          "Bound retries, preserve the released journey, and report residual dependency risk.",
      },
    );
    const agentCopy = await mkdtemp(
      join(tmpdir(), "vivarium-reliability-agent-"),
    );
    copies.push(agentCopy);
    await git(
      tmpdir(),
      "clone",
      `http://git:${launched.credential.token}@localhost:3000/git/${repository.id}.git`,
      agentCopy,
    );
    await git(agentCopy, "config", "user.name", "Vivarium Reliability Agent");
    await git(
      agentCopy,
      "config",
      "user.email",
      "reliability-agent@users.vivarium",
    );
    await git(
      agentCopy,
      "switch",
      "-c",
      "agent-repair",
      `origin/${launched.run.working_branch}`,
    );
    await writeFile(
      join(agentCopy, "service.txt"),
      "reliable checkout with bounded retries but shared timeout remains\n",
    );
    await git(agentCopy, "add", "service.txt");
    await git(agentCopy, "commit", "-m", "Bound payment retries");
    await git(
      agentCopy,
      "push",
      "origin",
      `HEAD:refs/heads/${launched.run.working_branch}`,
    );
    const agentCommit = await git(agentCopy, "rev-parse", "HEAD");
    const taskAgentHeaders = {
      Authorization: `Bearer ${launched.credential.token}`,
    };
    await json(ownerPage, "post", `${taskRunBase}/events`, taskAgentHeaders, {
      kind: "tool.action",
      state: "working",
      message:
        "Changed one file; 38 seconds compute and $0.04 attributed cost.",
      tool: "git",
      branch: launched.run.working_branch,
      commit_id: agentCommit,
    });
    await json(
      ownerPage,
      "post",
      `${taskRunBase}/completion`,
      taskAgentHeaders,
      {
        summary: "Bound retries; shared timeout remains a staged-rollout risk.",
        commit_id: agentCommit,
        checks: [
          {
            name: "checkout contract",
            status: "passed",
            details: "Static contract passes.",
          },
        ],
        unresolved_concerns: ["Verify shared timeout under staged traffic."],
      },
    );
    const firstPull = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/proposals/${improvement.proposal_id}/tasks/${task.id}/contributions`,
      owner.headers,
      {
        title: "Bound payment retries",
        body: "Agent repair with retained reliability evidence, authority, cost, and residual risk.",
        source_branch: launched.run.working_branch,
        target_branch: "main",
        session_id: launched.session.id,
        run_id: launched.run.id,
      },
    );
    await json(
      reviewerPage,
      "post",
      `/repositories/${repository.id}/pulls/${firstPull.id}/reviews`,
      reviewer.headers,
      { decision: "approved" },
    );
    const firstMerged = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/pulls/${firstPull.id}/merge`,
      owner.headers,
      {},
    );
    const firstDelivery = await releaseAndDeploy(
      `v1.1.1-${suffix}`,
      "First reviewed agent reliability repair.",
      firstMerged.merge_commit_id,
    );
    const firstRepairEvidence = await observed(
      {
        kind: "deployment",
        id: firstDelivery.deployment.id,
        revision: firstMerged.merge_commit_id,
        label: "First repair staged deployment",
      },
      9970,
      10000,
      -1_100_000,
      -700_000,
      "Retry repair improves results but still misses the objective.",
      2,
      [],
    );
    retained = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/improvements/${improvement.id}/verifications`,
      owner.headers,
      {
        verification: {
          kind: "deployment",
          resource_id: firstDelivery.deployment.id,
          revision: firstMerged.merge_commit_id,
          baseline_observation_id: burned.id,
          current_observation_id: firstRepairEvidence.id,
          decision: "contain",
          rationale:
            "The reviewed first repair improved user impact but did not restore the objective; contain it in staging.",
        },
      },
    );
    expect(retained.rollout_verifications.at(-1)).toMatchObject({
      improved: false,
      budget_restored: false,
      decision: "contain",
    });

    await git(copy, "pull", "--ff-only", "origin", "main");
    await git(copy, "switch", "-c", "repair-shared-timeout");
    await writeFile(
      join(copy, "service.txt"),
      "reliable checkout with bounded retries and isolated dependency timeout\n",
    );
    await git(copy, "add", "service.txt");
    await git(copy, "commit", "-m", "Isolate payment dependency timeout");
    await git(copy, "push", "origin", "repair-shared-timeout");
    const finalPull = await json(
      dependencyPage,
      "post",
      `/repositories/${repository.id}/pulls`,
      dependency.headers,
      {
        title: "Isolate payment timeout",
        body: `Corrects the failed first repair from reliability improvement ${improvement.id}.`,
        source_branch: "repair-shared-timeout",
        target_branch: "main",
        proposal_id: improvement.proposal_id,
      },
    );
    await json(
      reviewerPage,
      "post",
      `/repositories/${repository.id}/pulls/${finalPull.id}/reviews`,
      reviewer.headers,
      { decision: "approved" },
    );
    const finalMerged = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/pulls/${finalPull.id}/merge`,
      owner.headers,
      {},
    );
    const finalDelivery = await releaseAndDeploy(
      `v1.1.2-${suffix}`,
      "Reviewed timeout isolation after contained first repair.",
      finalMerged.merge_commit_id,
    );
    const recovered = await observed(
      {
        kind: "deployment",
        id: finalDelivery.deployment.id,
        revision: finalMerged.merge_commit_id,
        label: "Recovered staged deployment",
      },
      9998,
      10000,
      -600_000,
      -100_000,
      "Staged deployment restores checkout attainment with dependency evidence present.",
      1,
      [],
    );
    retained = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/service-objectives/${contract.id}/improvements/${improvement.id}/verifications`,
      owner.headers,
      {
        verification: {
          kind: "deployment",
          resource_id: finalDelivery.deployment.id,
          revision: finalMerged.merge_commit_id,
          baseline_observation_id: burned.id,
          current_observation_id: recovered.id,
          decision: "restore_budget",
          rationale:
            "Reviewed repair restores the released journey objective in staged deployment.",
        },
      },
    );
    expect(retained.rollout_verifications.at(-1)).toMatchObject({
      improved: true,
      budget_restored: true,
      decision: "restore_budget",
    });

    await ownerPage.goto(`/repositories/${repository.id}/reliability`);
    await expect(
      ownerPage.getByRole("heading", { name: "Service objectives" }),
    ).toBeVisible();
    await expect(
      ownerPage.getByText("Released checkout dependability", { exact: true }).last(),
    ).toBeVisible();
    await expect(
      ownerPage.getByText(
        "Corrected full cohort confirms checkout error-budget burn.",
      ),
    ).toBeVisible();
    await expect(
      ownerPage.getByText(
        "Staged deployment restores checkout attainment with dependency evidence present.",
      ),
    ).toBeVisible();
    const trail = await json(
      ownerPage,
      "get",
      `/repositories/${repository.id}/service-objectives/${contract.id}`,
      owner.headers,
    );
    expect(trail.investigations[0]).toMatchObject({
      created_by: owner.user.id,
      input_requests: [
        expect.objectContaining({
          owner_id: dependency.user.id,
          status: "answered",
        }),
      ],
      findings: [
        expect.objectContaining({
          created_by: investigator.id,
          actor_type: "agent",
        }),
      ],
    });
    expect(trail.improvements[0]).toMatchObject({
      id: improvement.id,
      authorization_observation_id: burned.id,
      proposal_id: improvement.proposal_id,
      status: "linked",
    });
    expect(trail.rollout_verifications).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ decision: "contain", improved: false }),
        expect.objectContaining({
          decision: "restore_budget",
          improved: true,
          budget_restored: true,
        }),
      ]),
    );
    const timeline = await json(
      ownerPage,
      "get",
      `/repositories/${repository.id}/proposals/${improvement.proposal_id}/tasks/${task.id}/sessions/${launched.session.id}/events?limit=100`,
      owner.headers,
    );
    expect(timeline.events).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: "tool.action",
          message: expect.stringContaining("$0.04 attributed cost"),
        }),
        expect.objectContaining({
          kind: "run.completed",
          agent_id: task.assignment.assignee_id,
          initiator_id: owner.user.id,
        }),
      ]),
    );
    readiness = await json(
      ownerPage,
      "get",
      `/repositories/${repository.id}/reliability-readiness/deployment/${degradedDelivery.deployment.id}?revision=${degradedRevision}&branch=main&service=checkout&environment=${environment.id}&journey=checkout&risk=availability`,
      owner.headers,
    );
    expect(readiness.evaluations[0]).toMatchObject({ effect: "pause" });
  } finally {
    await Promise.all(
      copies.map((path) => rm(path, { recursive: true, force: true })),
    );
  }
});
