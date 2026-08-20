import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- one intentionally connected governance trail */
const exec = promisify(execFile);
async function git(cwd: string, ...args: string[]) {
  return (await exec("git", args, { cwd, env: { ...process.env, GIT_TERMINAL_PROMPT: "0" } })).stdout.trim();
}
async function authenticatedGit(token: string, cwd: string, ...args: string[]) {
  return (await exec("git", ["-c", "credential.helper=", ...args], { cwd, env: { ...process.env, GIT_ASKPASS: join(__dirname, "git-askpass.sh"), GIT_TERMINAL_PROMPT: "0", VIVARIUM_GIT_TOKEN: token } })).stdout.trim();
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
  return { headers, user: await (await page.request.get("/api/user", { headers })).json() as any };
}
async function json(page: Page, method: "get" | "post" | "put", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}
async function rejected(page: Page, path: string, headers: Record<string, string>, data: unknown) {
  const response = await page.request.post(`/api${path}`, { headers, data });
  expect(response.status(), `POST ${path}: ${await response.text()}`).toBeGreaterThanOrEqual(400);
}

test("a team develops, releases, learns from, and safely repairs a project agent", async ({ browser }) => {
  test.setTimeout(360_000);
  const pages = await Promise.all(Array.from({ length: 6 }, async () => (await browser.newContext()).newPage()));
  const copy = await mkdtemp(join(tmpdir(), "vivarium-agent-development-"));
  let credentialToRevoke: { id: string } | undefined;
  let credentialOwnerHeaders: Record<string, string> | undefined;
  try {
    const suffix = Date.now().toString(36);
    const people = await Promise.all(pages.map((page, index) => account(page, ["Agent Owner", "Domain Owner", "Evaluator", "Data Owner", "Resource Owner", "Pilot Developer"][index], `agent-dev-${index}-${suffix}`)));
    const [owner, domain, evaluator, dataOwner, resourceOwner, developer] = people;
    const [ownerPage, domainPage, evaluatorPage, dataPage, resourcePage, developerPage] = pages;
    const organization = await json(ownerPage, "post", "/organizations", owner.headers, { name: `Agent Development ${suffix}`, slug: `agent-development-${suffix}` });
    for (let index = 1; index < people.length; index++) {
      await json(ownerPage, "post", `/organizations/${organization.id}/invitations`, owner.headers, { user_id: people[index].user.id });
      const state = await json(pages[index], "get", "/organizations", people[index].headers);
      const invitation = state.organizations.find((item: any) => item.id === organization.id).invitations.at(-1);
      await json(pages[index], "post", `/organizations/${organization.id}/invitations/${invitation.id}/accept`, people[index].headers);
    }
    const repository = await json(ownerPage, "post", `/organizations/${organization.id}/repositories`, owner.headers, { name: `triage-agent-${suffix}` });
    for (let index = 1; index < people.length; index++) await json(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: people[index].user.id });
    const pilotRepository = await json(ownerPage, "post", "/repositories", owner.headers, { name: `triage-pilot-${suffix}` });
    await json(ownerPage, "post", `/repositories/${pilotRepository.id}/collaborators`, owner.headers, { user_id: developer.user.id });
    const credential = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "agent development", scopes: ["git:read", "git:write"], expires_in: 3600 });
    credentialToRevoke = credential;
    credentialOwnerHeaders = owner.headers;
    const gitURL = `http://localhost:3000/git/${repository.id}.git`;
    await authenticatedGit(credential.token, tmpdir(), "clone", gitURL, copy);
    expect(await git(copy, "remote", "get-url", "origin")).toBe(gitURL);
    await git(copy, "config", "user.name", "Agent Owner");
    await git(copy, "config", "user.email", "owner@example.test");
    await writeFile(join(copy, "agent.md"), "Model: triage-1\nClassify incidents; draft only; escalate uncertainty.\n");
    await writeFile(join(copy, "triage.txt"), "timeout => reliability\n");
    await git(copy, "add", "."); await git(copy, "commit", "-m", "Define the incident triage collaborator"); await authenticatedGit(credential.token, copy, "push", "origin", "main");
    const base = await git(copy, "rev-parse", "HEAD");

    let orgState = await json(ownerPage, "post", `/organizations/${organization.id}/agents`, owner.headers, { name: "Triage Partner", slug: `triage-partner-${suffix}`, description: "A bounded incident triage collaborator", visibility: "public", capabilities: ["classify incident drafts"], operator_ids: [owner.user.id], team_ids: [] });
    const agent = orgState.agents.at(-1);
    orgState = await json(ownerPage, "put", `/organizations/${organization.id}/agents/${agent.id}/profile`, owner.headers, { expected_version: 0, profile: { summary: "Project-owned incident triage", supported_tasks: ["classify incident drafts"], tools: ["repository reader", "draft writer"], model_provenance: "triage-1", execution_provenance: "isolated project runner", deployment_boundaries: ["platform"], data_use: "Exact task context only; no training", retention: "Deleted after 24 hours", pricing: "$0.20 per task", resource_requirements: ["one CPU"], requested_capabilities: ["repository.read", "repository.write"], availability: "24x5", support: "owners@example.test", subprocessors: [], remote_execution_boundaries: ["none"], conflict_disclosures: ["Operator is accountable to the project owner"], change_summary: "Initial reviewed profile" } });

    const revision = (commit: string, model: string, summary: string) => ({ title: "Incident triage partner", purpose: "Classify an incident and draft a non-authoritative response", owner_ids: [owner.user.id, domain.user.id], sources: [{ id: "prompt", kind: "prompt", repository_id: repository.id, revision: commit, path: "agent.md", purpose: "Reviewed behavior contract" }, { id: "mapping", kind: "knowledge", repository_id: repository.id, revision: commit, path: "triage.txt", purpose: "Reviewed domain mapping" }], tools: [{ name: "repository", purpose: "Read selected incident context", actions: ["read"], boundary: "selected paths only" }, { name: "draft", purpose: "Draft a response", actions: ["create"], boundary: "never publish or merge" }], models: [{ provider: "Acme", name: model, version: model.endsWith("1") ? "1" : "2", purpose: "bounded classification" }], supported_tasks: ["incident classification"], expected_outputs: ["classification", "response draft", "uncertainty"], prohibited_actions: ["merge", "publish", "secrets", "production mutation"], memory_policy: "session only", data_use_terms: "project context; no training", budget: { max_cost_usd: 2, max_tokens: 4000, max_tool_actions: 4, max_runtime_seconds: 60 }, escalations: [{ trigger: "uncertainty above threshold", owner_ids: [domain.user.id], action: "stop and request human judgment" }], deployment_boundaries: [{ environment: "project pilot", repository_access: "selected read and draft", network_access: "none", data_classes: ["sanitized incident"], approval_required: true }], change_summary: summary });
    let project = await json(ownerPage, "post", `/repositories/${repository.id}/agent-projects`, owner.headers, { revision: revision(base, "triage-1", "Human-authored initial behavior") });
    await ownerPage.goto(`/repositories/${repository.id}/agents`);
    await expect(ownerPage.getByRole("heading", { name: "Incident triage partner" })).toBeVisible();

    const suite = await json(ownerPage, "post", `/organizations/${organization.id}/agent-evaluation-suites`, owner.headers, { name: "Incident triage scenarios", repository_id: repository.id, expected_version: 0, revision: { repository_revision: base, scenarios: [{ id: "timeout", title: "Timeout incident", sanitized_prompt: "Classify a sanitized timeout incident.", expected_outcomes: ["reliability draft"], checks: [{ name: "classification", kind: "contains", expected: "reliability" }], hidden_checks: [{ name: "no leaked canary", kind: "canary", expected: "SCENARIO-CANARY" }] }], budget: { max_cost: 2, max_latency_ms: 3000, max_tool_actions: 4 }, prohibited_actions: ["merge", "publish", "secrets", "network"], human_review_criteria: ["domain owner judges ambiguous classifications"], change_summary: "Protected project-owned evaluation" } });

    const makePull = async (branch: string, message: string, contents: string, author: string) => {
      await git(copy, "switch", "main"); await authenticatedGit(credential.token, copy, "pull", "--ff-only", "origin", "main"); await git(copy, "switch", "-C", branch);
      await git(copy, "config", "user.name", author); await writeFile(join(copy, "agent.md"), contents); await git(copy, "add", "agent.md"); await git(copy, "commit", "-m", message); await authenticatedGit(credential.token, copy, "push", "--force", "origin", branch);
      const commit = await git(copy, "rev-parse", "HEAD");
      const pull = await json(ownerPage, "post", `/repositories/${repository.id}/pulls`, owner.headers, { title: message, body: "Review the exact agent behavior and its bounded evidence.", source_branch: branch, target_branch: "main" });
      return { commit, pull };
    };
    const candidateFor = async (pull: any, commit: string, version: number, key: string, baselineCandidateID = "") => json(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/agent-candidates`, owner.headers, { idempotency_key: key, pull_revision: commit, project_id: project.id, project_version: version, suites: [{ suite_id: suite.id, version: 1 }], baseline_candidate_id: baselineCandidateID });
    const runCandidate = async (pull: any, candidate: any, key: string, success: boolean, extras: any = {}) => json(evaluatorPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/agent-candidates/${candidate.id}/runs`, evaluator.headers, { idempotency_key: key, suite_id: suite.id, suite_version: 1, suite_digest: candidate.suites[0].digest, isolation: "ephemeral", network: "none", permitted_services: [], max_tool_actions: 4, max_cost: 2, max_latency_ms: 3000, statistical_limits: { confidence_level: .95, minimum_samples: 2, margin_of_error: .2 }, contaminated: extras.contaminated ?? false, contamination_reasons: extras.contamination_reasons ?? [], nondeterministic: extras.nondeterministic ?? false, results: [1, 2].map((attempt) => ({ scenario_id: "timeout", attempt, task_success: success, policy_adherence: extras.policy ?? true, human_corrections: extras.corrections ?? 0, uncertainty: .1, latency_ms: 400, cost: .2, trace_digest: `${attempt}`.repeat(64), output_digest: `${success ? "a" : "b"}${attempt}`.padEnd(64, "0"), tool_actions: extras.actions ?? [], artifacts: [], evaluator_decision: extras.decision ?? (success ? "passed" : "failed") })) });

    const baselinePull = await makePull("agent/baseline", "Harden the initial triage prompt", "Model: triage-1\nClassify incidents; draft only; cite uncertainty.\n", "Agent Owner");
    project = await json(ownerPage, "post", `/repositories/${repository.id}/agent-projects/${project.id}/revisions`, owner.headers, { expected_version: 1, revision: revision(baselinePull.commit, "triage-1", "Human-authored candidate behavior") });
    const baselineCandidate = await candidateFor(baselinePull.pull, baselinePull.commit, 2, `baseline-${suffix}`);
    await runCandidate(baselinePull.pull, baselineCandidate, `leak-${suffix}`, false, { contaminated: true, contamination_reasons: ["protected scenario canary appeared in output"] });
    await runCandidate(baselinePull.pull, baselineCandidate, `prohibited-${suffix}`, false, { policy: false, actions: [{ tool: "git", action: "merge", input_digest: "1".repeat(64), output_digest: "2".repeat(64), duration_ms: 5, denied: true }] });
    const baselineRun = await runCandidate(baselinePull.pull, baselineCandidate, `baseline-run-${suffix}`, true);

    const candidatePull = await makePull("agent/candidate", "Teach the agent retry classification", "Model: triage-1\nClassify timeouts as reliability; draft only; cite uncertainty and retry context.\n", "Triage Partner Agent");
    project = await json(ownerPage, "post", `/repositories/${repository.id}/agent-projects/${project.id}/revisions`, owner.headers, { expected_version: 2, revision: revision(candidatePull.commit, "triage-1", "Agent-authored behavior change under human intent") });
    const candidate = await candidateFor(candidatePull.pull, candidatePull.commit, 3, `candidate-${suffix}`, baselineCandidate.id);
    const evaluatorDisagreement = await runCandidate(candidatePull.pull, candidate, `disagreement-${suffix}`, true, { decision: "failed", corrections: 1 });
    expect(evaluatorDisagreement.results[0]).toMatchObject({ evaluator_id: evaluator.user.id, evaluator_decision: "failed" });
    const acceptedRun = await runCandidate(candidatePull.pull, candidate, `accepted-${suffix}`, true);
    const comparison = await json(ownerPage, "get", `/repositories/${repository.id}/pulls/${candidatePull.pull.id}/agent-candidates`, owner.headers);
    expect(comparison.candidates[0].comparison).toMatchObject({ baseline_candidate_id: baselineCandidate.id, comparable_suites: [suite.id] });
    await ownerPage.goto(`/pulls/${repository.id}/${candidatePull.pull.id}`);
    await expect(ownerPage.getByText("Agent behavior candidates", { exact: true })).toBeVisible();
    await domainPage.goto(`/pulls/${repository.id}/${candidatePull.pull.id}`); await domainPage.getByRole("button", { name: "Approve" }).click();

    const createPilot = (name: string, budget: any) => json(ownerPage, "post", `/repositories/${repository.id}/pulls/${candidatePull.pull.id}/agent-candidates/${candidate.id}/pilots`, owner.headers, { repository_ids: [pilotRepository.id], roles: [name], task_kinds: ["incident"], actions: ["repository.read", "draft.create", "task.comment"], budget, starts_at: new Date(Date.now() - 1000).toISOString(), expires_at: new Date(Date.now() + 86_400_000).toISOString(), invitations: [{ participant_id: owner.user.id, role: name, repository_ids: [pilotRepository.id], task_kinds: ["incident"], actions: ["repository.read", "draft.create", "task.comment"] }] });
    let breached = (await createPilot("cost containment", { max_minutes: 5, max_actions: 1, max_cost: .5 })).pilot;
    breached = (await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${candidatePull.pull.id}/agent-candidates/${candidate.id}/pilots/${breached.id}/actions`, owner.headers, { expected_version: breached.version, kind: "consent" })).pilot;
    breached = (await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${candidatePull.pull.id}/agent-candidates/${candidate.id}/pilots/${breached.id}/actions`, owner.headers, { expected_version: breached.version, kind: "delegate", session: { repository_id: pilotRepository.id, task_kind: "incident", task_id: "cost-check", expected_outcome: "bounded response" } })).pilot;
    breached = (await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${candidatePull.pull.id}/agent-candidates/${candidate.id}/pilots/${breached.id}/actions`, owner.headers, { expected_version: breached.version, kind: "session_event", session_id: breached.sessions[0].id, event: { kind: "result", summary: "cost exceeded before effect", action: "draft.create", cost: 1, minutes: 1 } })).pilot;
    expect(breached).toMatchObject({ paused: true, pause_reason: "budget_exhausted", sessions: [expect.objectContaining({ events: [expect.objectContaining({ kind: "policy_denial" })] })] });
    expect(breached.sessions[0].events[0].cost ?? 0).toBe(0);

    let pilot = (await createPilot("intended developer", { max_minutes: 20, max_actions: 5, max_cost: 5 })).pilot;
    pilot = (await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${candidatePull.pull.id}/agent-candidates/${candidate.id}/pilots/${pilot.id}/actions`, owner.headers, { expected_version: pilot.version, kind: "consent" })).pilot;
    pilot = (await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${candidatePull.pull.id}/agent-candidates/${candidate.id}/pilots/${pilot.id}/actions`, owner.headers, { expected_version: pilot.version, kind: "delegate", session: { repository_id: pilotRepository.id, task_kind: "incident", task_id: "pilot-incident", expected_outcome: "reliability classification with bounded draft" } })).pilot;
    pilot = (await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${candidatePull.pull.id}/agent-candidates/${candidate.id}/pilots/${pilot.id}/actions`, owner.headers, { expected_version: pilot.version, kind: "session_event", session_id: pilot.sessions[0].id, event: { kind: "result", summary: "classified timeout and drafted response", action: "draft.create", cost: .4, minutes: 2 } })).pilot;
    pilot = (await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${candidatePull.pull.id}/agent-candidates/${candidate.id}/pilots/${pilot.id}/actions`, owner.headers, { expected_version: pilot.version, kind: "feedback", feedback: { candidate_revision: candidate.pull_revision, session_id: pilot.sessions[0].id, outcome: "accepted", expected_outcome: "reliability classification with bounded draft", correction: "no correction required" } })).pilot;

    const approvalActors = [[evaluatorPage, evaluator, "evaluation", acceptedRun.id], [domainPage, domain, "domain_review", "Domain mapping reviewed."], [developerPage, developer, "pilot_acceptance", pilot.id], [dataPage, dataOwner, "data_policy", "No training and session retention reviewed."], [resourcePage, resourceOwner, "resources", "Cost and action bounds reviewed."]] as const;
    const approvals = [];
    for (const [page, person, kind, evidenceID] of approvalActors) approvals.push(await json(page, "post", `/repositories/${repository.id}/agent-candidates/${candidate.id}/release-approvals`, person.headers, { kind, evidence_id: evidenceID, evidence: evidenceID }));
    const release = await json(ownerPage, "post", `/repositories/${repository.id}/agent-releases`, owner.headers, { organization_id: organization.id, agent_id: agent.id, candidate_id: candidate.id, candidate_revision: candidate.pull_revision, project_id: project.id, project_version: 3, contract_digest: candidate.contract_digest, model_versions: ["triage-1@1"], tool_versions: ["repository@1", "draft@1"], roles: ["developer collaborator"], approval_ids: approvals.map((item) => item.id), pilot_id: pilot.id });
    expect(release).toMatchObject({ status: "attested", attestation: expect.stringMatching(/^[a-f0-9]{64}$/) });
    await ownerPage.reload(); await ownerPage.getByRole("button", { name: "Merge into main" }).click(); await expect(ownerPage.getByText("Merged", { exact: true })).toBeVisible();
    let deployment = await json(ownerPage, "post", `/repositories/${repository.id}/agent-releases/${release.id}/deployments`, owner.headers, { identity: `organization-agent:${agent.id}`, roles: release.roles, credential_scopes: ["repository.read", "draft.create", "task.comment"], budget: { max_cost: 10, max_actions: 20, max_minutes: 60 }, rollback_release_id: release.id, operator_terms: "The operator accepts scoped work, signals, pause, and rollback." });
    deployment = await json(ownerPage, "post", `/repositories/${repository.id}/agent-deployments/${deployment.id}/actions`, owner.headers, { expected_version: deployment.version, kind: "signal", signal: { kind: "outcome", outcome: "production feedback: retry storm misclassified as routine timeout", corrections: 1, cost: .6, latency_ms: 700, policy: "within scope", safety: "regression" } });
    deployment = await json(ownerPage, "post", `/repositories/${repository.id}/agent-deployments/${deployment.id}/actions`, owner.headers, { expected_version: deployment.version, kind: "rollback", summary: "Rollback after the reproduced retry-storm regression." });
    expect(deployment.status).toBe("rolled_back");

    // Consent withdrawal is durable containment and cannot be reused for release acceptance.
    pilot = (await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${candidatePull.pull.id}/agent-candidates/${candidate.id}/pilots/${pilot.id}/actions`, owner.headers, { expected_version: pilot.version, kind: "revoke_consent" })).pilot;
    await rejected(ownerPage, `/repositories/${repository.id}/agent-candidates/${candidate.id}/release-approvals`, owner.headers, { kind: "pilot_acceptance", evidence_id: pilot.id });
    expect(pilot.invitations[0].revoked_at).toBeTruthy();

    // The repair is a new model/contract revision and new exact evidence; the old release remains immutable.
    await git(copy, "switch", "main"); await authenticatedGit(credential.token, copy, "pull", "--ff-only", "origin", "main");
    const repairPull = await makePull("agent/retry-repair", "Repair retry-storm classification", "Model: triage-2\nClassify retry storms as reliability incidents; draft only; cite uncertainty and retry context.\n", "Triage Partner Agent");
    project = await json(ownerPage, "post", `/repositories/${repository.id}/agent-projects/${project.id}/revisions`, owner.headers, { expected_version: 3, revision: revision(repairPull.commit, "triage-2", "Reproduced production regression repair and model change") });
    const repaired = await candidateFor(repairPull.pull, repairPull.commit, 4, `repair-${suffix}`, candidate.id);
    const repairedRun = await runCandidate(repairPull.pull, repaired, `repair-run-${suffix}`, true);
    expect(repairedRun.results.every((result: any) => result.task_success && result.policy_adherence)).toBe(true);
    const repairPilotPath = `/repositories/${repository.id}/pulls/${repairPull.pull.id}/agent-candidates/${repaired.id}/pilots`;
    let repairPilot = (await json(ownerPage, "post", repairPilotPath, owner.headers, { repository_ids: [pilotRepository.id], roles: ["intended developer"], task_kinds: ["incident"], actions: ["repository.read", "draft.create", "task.comment"], budget: { max_minutes: 20, max_actions: 5, max_cost: 5 }, starts_at: new Date(Date.now() - 1000).toISOString(), expires_at: new Date(Date.now() + 86_400_000).toISOString(), invitations: [{ participant_id: owner.user.id, role: "intended developer", repository_ids: [pilotRepository.id], task_kinds: ["incident"], actions: ["repository.read", "draft.create", "task.comment"] }] })).pilot;
    const repairActionPath = `${repairPilotPath}/${repairPilot.id}/actions`;
    repairPilot = (await json(ownerPage, "post", repairActionPath, owner.headers, { expected_version: repairPilot.version, kind: "consent" })).pilot;
    repairPilot = (await json(ownerPage, "post", repairActionPath, owner.headers, { expected_version: repairPilot.version, kind: "delegate", session: { repository_id: pilotRepository.id, task_kind: "incident", task_id: "retry-storm-repair", expected_outcome: "retry storm escalates as reliability" } })).pilot;
    repairPilot = (await json(ownerPage, "post", repairActionPath, owner.headers, { expected_version: repairPilot.version, kind: "session_event", session_id: repairPilot.sessions[0].id, event: { kind: "result", summary: "reproduced regression now escalates correctly", action: "draft.create", cost: .3, minutes: 1 } })).pilot;
    repairPilot = (await json(ownerPage, "post", repairActionPath, owner.headers, { expected_version: repairPilot.version, kind: "feedback", feedback: { candidate_revision: repaired.pull_revision, session_id: repairPilot.sessions[0].id, outcome: "accepted", expected_outcome: "retry storm escalates as reliability", correction: "production regression is repaired" } })).pilot;
    const repairEvidence = [repairedRun.id, "Repair domain mapping reviewed.", repairPilot.id, "Repair preserves data terms.", "Repair remains within resource bounds."];
    const repairApprovals = [];
    for (let index = 0; index < approvalActors.length; index++) {
      const [page, person, kind] = approvalActors[index];
      repairApprovals.push(await json(page, "post", `/repositories/${repository.id}/agent-candidates/${repaired.id}/release-approvals`, person.headers, { kind, evidence_id: repairEvidence[index], evidence: repairEvidence[index] }));
    }
    const repairRelease = await json(ownerPage, "post", `/repositories/${repository.id}/agent-releases`, owner.headers, { organization_id: organization.id, agent_id: agent.id, candidate_id: repaired.id, candidate_revision: repaired.pull_revision, project_id: project.id, project_version: 4, contract_digest: repaired.contract_digest, model_versions: ["triage-2@2"], tool_versions: ["repository@1", "draft@1"], roles: ["developer collaborator"], approval_ids: repairApprovals.map((item) => item.id), pilot_id: repairPilot.id });
    let repairedDeployment = await json(ownerPage, "post", `/repositories/${repository.id}/agent-releases/${repairRelease.id}/deployments`, owner.headers, { identity: `organization-agent:${agent.id}`, roles: repairRelease.roles, credential_scopes: ["repository.read", "draft.create", "task.comment"], budget: { max_cost: 10, max_actions: 20, max_minutes: 60 }, rollback_release_id: release.id, operator_terms: "The operator separately consents to the repaired model and rollback boundary." });
    repairedDeployment = await json(ownerPage, "post", `/repositories/${repository.id}/agent-deployments/${repairedDeployment.id}/actions`, owner.headers, { expected_version: repairedDeployment.version, kind: "signal", signal: { kind: "outcome", outcome: "scoped retry-storm work classified and escalated correctly", corrections: 0, cost: .3, latency_ms: 500, policy: "passed", safety: "passed" } });
    expect(repairedDeployment).toMatchObject({ status: "active", signals: [expect.objectContaining({ outcome: "scoped retry-storm work classified and escalated correctly" })] });
    await domainPage.goto(`/pulls/${repository.id}/${repairPull.pull.id}`); await domainPage.getByRole("button", { name: "Approve" }).click();
    await ownerPage.goto(`/pulls/${repository.id}/${repairPull.pull.id}`); await ownerPage.getByRole("button", { name: "Merge into main" }).click(); await expect(ownerPage.getByText("Merged", { exact: true })).toBeVisible();
    const releases = await json(ownerPage, "get", `/repositories/${repository.id}/agent-releases`, owner.headers);
    expect(releases.releases).toContainEqual(expect.objectContaining({ id: release.id, status: "attested", model_versions: ["triage-1@1"] }));
    expect(releases.releases).toContainEqual(expect.objectContaining({ id: repairRelease.id, status: "attested", model_versions: ["triage-2@2"] }));
    expect(baselineRun.results).toHaveLength(2);
  } finally {
    if (credentialToRevoke && credentialOwnerHeaders) {
      const revoked = await pages[0].request.delete(`/api/auth/credentials/${credentialToRevoke.id}`, { headers: credentialOwnerHeaders });
      expect(revoked.status(), await revoked.text()).toBe(204);
    }
    await rm(copy, { recursive: true, force: true });
  }
});
