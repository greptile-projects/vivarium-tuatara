import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey joins several public collaboration projections */

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
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const user = await (await page.request.get("/api/user", { headers })).json() as { id: string };
  return { headers, user };
}
async function json(page: Page, method: "get" | "post" | "put" | "delete", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}
test("a governed human-agent team delivers an accepted decision", async ({ browser }) => {
  test.setTimeout(300_000);
  const dockerAvailable = await run("docker", ["info"]).then(() => true, () => false);
  test.skip(!dockerAvailable, "the delivery-team journey requires the Docker-backed check executor");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copies: string[] = [];
  const copy = async (prefix: string) => { const path = await mkdtemp(join(tmpdir(), prefix)); copies.push(path); return path; };
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const developerPage = await (await browser.newContext()).newPage();
    const researchOperatorPage = await (await browser.newContext()).newPage();
    const implementationOperatorPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, "Delivery Lead", `delivery-lead-${suffix}`);
    const developer = await account(developerPage, "Delivery Developer", `delivery-dev-${suffix}`);
    const researchOperator = await account(researchOperatorPage, "Research Operator", `research-operator-${suffix}`);
    const implementationOperator = await account(implementationOperatorPage, "Implementation Operator", `implementation-operator-${suffix}`);

    const organization = await json(ownerPage, "post", "/organizations", owner.headers, { name: `Delivery Lab ${suffix}`, slug: `delivery-lab-${suffix}`, description: "Govern specialized human-agent delivery." }) as any;
    for (const member of [developer, researchOperator, implementationOperator]) {
      const invited = await json(ownerPage, "post", `/organizations/${organization.id}/invitations`, owner.headers, { user_id: member.user.id }) as any;
      const invitation = invited.invitations.at(-1);
      await json(member === developer ? developerPage : member === researchOperator ? researchOperatorPage : implementationOperatorPage, "post", `/organizations/${organization.id}/invitations/${invitation.id}/accept`, member.headers);
    }
    const repository = await json(ownerPage, "post", `/organizations/${organization.id}/repositories`, owner.headers, { name: `coordinated-runtime-${suffix}` }) as any;
    let group = await json(ownerPage, "post", `/organizations/${organization.id}/agents`, owner.headers, { name: "Evidence Agent", slug: `evidence-${suffix}`, capabilities: ["analyze behavior", "publish findings"], operator_ids: [researchOperator.user.id], team_ids: [] }) as any;
    const evidenceAgent = group.agents.find((item: any) => item.slug === `evidence-${suffix}`);
    group = await json(ownerPage, "post", `/organizations/${organization.id}/agents`, owner.headers, { name: "Implementation Agent", slug: `implementation-${suffix}`, capabilities: ["implement bounded changes", "run verification"], operator_ids: [implementationOperator.user.id], team_ids: [] }) as any;
    const implementationAgent = group.agents.find((item: any) => item.slug === `implementation-${suffix}`);
    const expiry = new Date(Date.now() + 3_600_000).toISOString();
    const grants = new Map<string, any>();
    for (const [page, operator, agent] of [[researchOperatorPage, researchOperator, evidenceAgent], [implementationOperatorPage, implementationOperator, implementationAgent]] as const) {
      const requested = await json(page, "post", `/organizations/${organization.id}/access-requests`, operator.headers, { principal_type: "agent", principal_id: agent.id, role: "contributor", resources: [{ kind: "repository", id: repository.id }], exceptions: [], reason: "Execute one chartered delivery stream.", expires_at: expiry }) as any;
      const request = requested.access_requests.at(-1);
      const approved = await json(ownerPage, "post", `/organizations/${organization.id}/access-requests/${request.id}/decision`, owner.headers, { decision: "approve" }) as any;
      grants.set(agent.id, approved.access_grants.at(-1));
    }

    const ownerGit = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "delivery baseline", scopes: ["git:read", "git:write"], expires_in: 3600 }) as any;
    const baseline = await copy("vivarium-delivery-baseline-");
    await git(tmpdir(), "clone", `http://git:${ownerGit.token}@localhost:3000/git/${repository.id}.git`, baseline);
    await git(baseline, "config", "user.name", "Delivery Lead"); await git(baseline, "config", "user.email", "lead@example.test");
    await run("mkdir", ["-p", join(baseline, ".vivarium")]);
    await writeFile(join(baseline, ".vivarium", "checks.json"), JSON.stringify({ version: 1, checks: [{ name: "coordinated verification", image: "alpine:3.22", command: "test -r README.md" }] }));
    await writeFile(join(baseline, "README.md"), "# Coordinated runtime\n");
    await git(baseline, "add", "."); await git(baseline, "commit", "-m", "Establish coordinated delivery baseline"); await git(baseline, "push", "origin", "main");
    const base = await git(baseline, "rev-parse", "HEAD");
    await json(ownerPage, "put", `/repositories/${repository.id}/branches/main/required-checks`, owner.headers, { checks: ["coordinated verification"] });

    const evidence = { kind: "usage", resource_id: "coordination-trace", revision: base, label: "Exact baseline trace", captured_at: new Date().toISOString() };
    let decision = await json(ownerPage, "post", `/repositories/${repository.id}/decisions`, owner.headers, { source: { kind: "repository" }, scope: { question: "How should the runtime expose bounded retries?", constraints: ["Retain an operator-readable fallback"], success_measures: ["All coordinated checks pass"], deadline: expiry, affected_resources: [{ kind: "repository", repository_id: repository.id, label: "Coordinated runtime" }], participants: [{ user_id: owner.user.id }, { user_id: developer.user.id }], owner_id: owner.user.id } }) as any;
    decision = await json(ownerPage, "post", `/decisions/${decision.id}/alternatives`, owner.headers, { expected_version: decision.version, alternative: { title: "Bounded retries", summary: "Cap retries and retain a human runbook.", assumptions: ["Three attempts cover transient failures"], tradeoffs: ["Some work reaches the fallback"], risks: ["Retry evidence may be incomplete"], compatibility_impact: "No API break", cost: "Three parallel streams", expected_outcomes: ["Predictable recovery"], evidence: [evidence], criteria: [{ criterion: "All coordinated checks pass", outcome: "The exact baseline supports isolated verification.", evidence: [evidence] }] } });
    const alternative = decision.alternatives[0];
    decision = await json(ownerPage, "post", `/decisions/${decision.id}/publish`, owner.headers, { expected_version: decision.version, commitment: { selected_alternative_id: alternative.id, rejected_alternative_ids: [], rationale: "Specialized evidence, implementation, and human verification can proceed in parallel.", accepted_tradeoffs: ["Fallback handling remains explicit"], dissent_finding_ids: [], conditions: ["Independent review is required"], review_date: expiry, evidence: [evidence], exceptions: [] } });
    expect(decision.status).toBe("published");

    const participant = (id: string, principal_type: string, principal_id: string, role: string, responsibility: string) => ({ id, principal_type, principal_id, role, responsibility, why: `Specialized ${role} ownership`, budget: { unit: "minutes", limit: 30 }, escalation: "Escalate consequential choices to the delivery lead", required_access: [{ repository_id: repository.id, level: "write" }] });
    let team = await json(ownerPage, "post", `/repositories/${repository.id}/delivery-teams`, owner.headers, { outcome: { kind: "decision", resource_id: decision.id, title: decision.scope.question }, charter: { name: "Bounded retry delivery team", purpose: "Deliver the accepted bounded-retry decision with visible specialization.", overall_budget: { unit: "minutes", limit: 90 }, deadline: expiry, escalation_path: "The human lead resolves disputed evidence and authority changes.", participants: [{ ...participant("lead", "human", owner.user.id, "delivery lead", "Govern consequential decisions and authority boundaries"), budget: undefined }, participant("developer", "human", developer.user.id, "delivery developer", "Verify the agent handoff and own the operator runbook"), participant("evidence-agent", "agent", evidenceAgent.id, "evidence specialist", "Analyze retry behavior and expose uncertainty"), participant("implementation-agent", "agent", implementationAgent.id, "implementation specialist", "Implement and verify bounded retry behavior")] } }) as any;
    for (const [page, actor, id] of [[ownerPage, owner, "lead"], [developerPage, developer, "developer"], [researchOperatorPage, researchOperator, "evidence-agent"], [implementationOperatorPage, implementationOperator, "implementation-agent"]] as const) {
      team = await json(page, "post", `/delivery-teams/${team.id}/participants/${id}/response`, actor.headers, { expected_version: team.version, decision: "accepted" });
    }
    const stream = (id: string, title: string, owner: string, path: string, artifact: string, order: number) => ({ id, title, owner_participant_id: owner, inputs: [{ name: "accepted decision", repository_id: repository.id, revision: base, artifact: decision.id }], expected_artifacts: [artifact], dependency_ids: [], acceptance_criteria: [`${artifact} verified`], repository_scope: [{ repository_id: repository.id, reference: "main", revision: base, paths: [path] }], integration_order: order, budget: { unit: "minutes", limit: 30 }, assumptions: ["The accepted decision remains current"] });
    team = await json(ownerPage, "put", `/delivery-teams/${team.id}/plan`, owner.headers, { expected_version: team.version, plan: { streams: [stream("research", "Bounded retry evidence", "evidence-agent", "research.md", "retry evidence", 1), stream("implementation", "Bounded retry implementation", "implementation-agent", "implementation.md", "bounded implementation", 2), stream("runbook", "Operator handoff", "developer", "runbook.md", "operator runbook", 3)] } });
    for (const [page, actor, id] of [[developerPage, developer, "developer"], [researchOperatorPage, researchOperator, "evidence-agent"], [implementationOperatorPage, implementationOperator, "implementation-agent"]] as const) {
      team = await json(page, "post", `/delivery-teams/${team.id}/plan/participants/${id}/response`, actor.headers, { expected_version: team.version, expected_plan_revision: team.plan.revision, decision: "accepted" });
    }

    team = await json(researchOperatorPage, "post", `/delivery-teams/${team.id}/timeline`, researchOperator.headers, { expected_version: team.version, entry: { stream_id: "research", kind: "uncertainty", body: "Finding: three retries appear to amplify load under burst traffic.", citations: [] } });
    const disputedFinding = team.timeline.at(-1);
    team = await json(implementationOperatorPage, "post", `/delivery-teams/${team.id}/timeline`, implementationOperator.headers, { expected_version: team.version, entry: { stream_id: "implementation", kind: "uncertainty", body: "Dispute: bounded jitter prevents the reported amplification in the implementation trace.", citations: [] } });
    team = await json(ownerPage, "post", `/delivery-teams/${team.id}/timeline`, owner.headers, { expected_version: team.version, entry: { stream_id: "research", kind: "uncertainty", body: "Resolution: retain the disputed burst case as residual risk and require the runbook fallback.", citations: [] } });

    team = await json(implementationOperatorPage, "put", `/delivery-teams/${team.id}/streams/implementation/status`, implementationOperator.headers, { expected_version: team.version, status: { status: "failed", summary: "The first implementation exceeded its retry budget.", progress_percent: 45, revision: base, resource_use: { unit: "minutes", consumed: 12 }, questions: [], blockers: [{ kind: "agent_failed", summary: "Unbounded retry loop", recovery: "Narrow the implementation to bounded jitter" }], predicted_next_action: "Await lead guidance" } });
    team = await json(ownerPage, "post", `/delivery-teams/${team.id}/interventions`, owner.headers, { expected_version: team.version, intervention: { scope: "stream", stream_id: "implementation", action: "guide", guidance: "Redirect the failing stream to bounded jitter and preserve the failed attempt as evidence." } });
    team = await json(ownerPage, "post", `/delivery-teams/${team.id}/interventions`, owner.headers, { expected_version: team.version, intervention: { scope: "stream", stream_id: "implementation", action: "resume", guidance: "Resume only the bounded implementation." } });

    team = await json(researchOperatorPage, "post", `/delivery-teams/${team.id}/handoffs`, researchOperator.headers, { expected_version: team.version, handoff: { stream_id: "research", to_participant_id: "developer", input_entry_ids: [disputedFinding.id], acceptance_criteria: ["Human verifies the residual risk is understandable"], residual_uncertainty: ["Production burst frequency remains unknown"] } });
    const handoff = team.handoffs.at(-1);
    team = await json(ownerPage, "post", `/delivery-teams/${team.id}/interventions`, owner.headers, { expected_version: team.version, intervention: { scope: "stream", stream_id: "research", action: "reassign", guidance: "Complete the explicit agent-to-human verification handoff.", new_owner_participant_id: "developer" } });
    team = await json(developerPage, "post", `/delivery-teams/${team.id}/plan/participants/developer/response`, developer.headers, { expected_version: team.version, expected_plan_revision: team.plan.revision, decision: "accepted" });
    team = await json(implementationOperatorPage, "post", `/delivery-teams/${team.id}/plan/participants/implementation-agent/response`, implementationOperator.headers, { expected_version: team.version, expected_plan_revision: team.plan.revision, decision: "accepted" });
    team = await json(developerPage, "post", `/delivery-teams/${team.id}/timeline`, developer.headers, { expected_version: team.version, entry: { stream_id: "research", kind: "uncertainty", body: "Human verification: the runbook names the disputed burst risk and a bounded fallback.", citations: [] } });
    const handoffVerification = team.timeline.at(-1);
    team = await json(developerPage, "post", `/delivery-teams/${team.id}/handoffs/${handoff.id}/accept`, developer.headers, { expected_version: team.version, verification_entry_ids: [handoffVerification.id], note: "Verified the agent finding, dispute, and residual uncertainty before accepting ownership." });

    const developerGit = await json(developerPage, "post", "/auth/credentials", developer.headers, { kind: "git", name: "delivery handoff", scopes: ["git:read", "git:write"], expires_in: 3600 }) as any;
    const issued = new Map<string, any>();
    for (const [page, actor, agent] of [[researchOperatorPage, researchOperator, evidenceAgent], [implementationOperatorPage, implementationOperator, implementationAgent]] as const) {
      issued.set(agent.id, await json(page, "post", `/organizations/${organization.id}/access-grants/${grants.get(agent.id).id}/credentials`, actor.headers, { agent_id: agent.id, repository_id: repository.id, expires_in: 3600 }));
    }
    const branches: Record<string, { branch: string; commit: string }> = {};
    for (const work of [{ stream: "research", branch: "team-research", file: "research.md", text: "Agent finding retained; human accepted the bounded fallback.\n", token: issued.get(evidenceAgent.id).token, author: "Evidence Agent" }, { stream: "implementation", branch: "team-implementation", file: "implementation.md", text: "Bounded jitter: at most three attempts.\n", token: issued.get(implementationAgent.id).token, author: "Implementation Agent" }, { stream: "runbook", branch: "team-runbook", file: "runbook.md", text: "After three attempts, stop and page the human lead.\n", token: developerGit.token, author: "Delivery Developer" }]) {
      const path = await copy(`vivarium-delivery-${work.stream}-`);
      await git(tmpdir(), "clone", `http://git:${work.token}@localhost:3000/git/${repository.id}.git`, path);
      await git(path, "config", "user.name", work.author); await git(path, "config", "user.email", `${work.stream}@example.test`); await git(path, "switch", "-c", work.branch);
      await writeFile(join(path, work.file), work.text); await git(path, "add", work.file); await git(path, "commit", "-m", `Complete ${work.stream} stream`); await git(path, "push", "origin", work.branch);
      branches[work.stream] = { branch: work.branch, commit: await git(path, "rev-parse", "HEAD") };
    }
    const entryByStream: Record<string, string> = { research: handoffVerification.id };
    for (const [page, actor, streamID, body] of [[implementationOperatorPage, implementationOperator, "implementation", "Verification: bounded jitter passes the exact coordinated check."], [developerPage, developer, "runbook", "Verification: the human-owned fallback is actionable."]] as const) {
      team = await json(page, "post", `/delivery-teams/${team.id}/timeline`, actor.headers, { expected_version: team.version, entry: { stream_id: streamID, kind: "uncertainty", body, citations: [] } });
      entryByStream[streamID] = team.timeline.at(-1).id;
    }
    for (const [page, actor, streamID, consumed] of [[developerPage, developer, "research", 18], [implementationOperatorPage, implementationOperator, "implementation", 22], [developerPage, developer, "runbook", 9]] as const) {
      team = await json(page, "put", `/delivery-teams/${team.id}/streams/${streamID}/status`, actor.headers, { expected_version: team.version, status: { status: "completed", summary: `${streamID} contribution is ready for integration.`, progress_percent: 100, revision: base, resource_use: { unit: "minutes", consumed }, questions: [], blockers: [], predicted_next_action: "Publish through ordinary review" } });
    }
    const criteria: Record<string, string> = { research: "retry evidence verified", implementation: "bounded implementation verified", runbook: "operator runbook verified" };
    team = await json(ownerPage, "post", `/delivery-teams/${team.id}/integrations`, owner.headers, { expected_version: team.version, plan_revision: team.plan.revision, base_revision: base, contributions: ["research", "implementation", "runbook"].map((streamID, index) => ({ stream_id: streamID, repository_id: repository.id, source_kind: "branch", branch: branches[streamID].branch, commit_id: branches[streamID].commit, acceptance_evidence: { [criteria[streamID]]: [entryByStream[streamID]] }, agent_actions: streamID === "runbook" ? [] : [`specialized ${streamID} execution`], decisions: ["Retain bounded fallback"], cost: { unit: "minutes", consumed: [18, 22, 9][index] }, residual_risks: streamID === "research" ? ["Production burst frequency remains unknown"] : [] })) });
    const integration = team.integrations.at(-1);
    expect(integration.blockers).toEqual([]);
    team = await json(ownerPage, "post", `/delivery-teams/${team.id}/integrations/${integration.id}/publish`, owner.headers, { expected_version: team.version, target_branch: "main" });
    expect(team.integrations.at(-1).pull_requests).toHaveLength(3);

    await json(ownerPage, "delete", `/organizations/${organization.id}/members/${researchOperator.user.id}`, owner.headers);
    const deniedPush = await git(copies.find((path) => path.includes("delivery-research"))!, "push", "origin", "team-research").then(() => "allowed", (error: any) => String(error.stderr));
    expect(deniedPush).toContain("Authentication failed");
    const retained = await json(ownerPage, "get", `/delivery-teams/${team.id}`, owner.headers) as any;
    expect(retained.timeline).toEqual(expect.arrayContaining([expect.objectContaining({ id: disputedFinding.id, author_id: evidenceAgent.id })]));
    expect(retained.handoffs[0]).toMatchObject({ status: "accepted", accepted_by: developer.user.id });
    expect(retained.interventions).toEqual(expect.arrayContaining([expect.objectContaining({ action: "guide" }), expect.objectContaining({ action: "reassign" })]));

    let lastMerge: any;
    for (const linked of retained.integrations[0].pull_requests.sort((a: any, b: any) => a.order - b.order)) {
      await expect.poll(async () => JSON.stringify(await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${linked.pull_request_id}/checks`, owner.headers)), { message: `checks pass for stream ${linked.stream_id}`, timeout: 60_000 }).toContain('"state":"succeeded"');
      await json(developerPage, "post", `/repositories/${repository.id}/pulls/${linked.pull_request_id}/reviews`, developer.headers, { decision: "approved" });
      lastMerge = await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${linked.pull_request_id}/merge`, owner.headers, {});
    }
    const release = await json(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: "v1.0.0", notes: "Accepted decision delivered by a governed mixed team.", commit_id: lastMerge.merge_commit_id }) as any;

    await ownerPage.goto("/delivery-teams");
    await expect(ownerPage.getByRole("heading", { name: "Delivery teams" })).toBeVisible();
    await expect(ownerPage.getByText("Bounded retry delivery team", { exact: true })).toBeVisible();
    await expect(ownerPage.getByText(/three retries appear to amplify load/)).toBeVisible();
    await ownerPage.getByText("Retained intervention history", { exact: true }).click();
    await expect(ownerPage.getByText(/Redirect the failing stream to bounded jitter/)).toBeVisible();
    await expect(ownerPage.getByText(/Verified the agent finding, dispute/)).toBeVisible();
    expect(release).toMatchObject({ version: "v1.0.0", commit_id: lastMerge.merge_commit_id });
    expect(retained.integrations[0].contributions.reduce((total: number, item: any) => total + item.cost.consumed, 0)).toBe(49);
  } finally {
    await Promise.all(copies.map((path) => rm(path, { recursive: true, force: true })));
  }
});
