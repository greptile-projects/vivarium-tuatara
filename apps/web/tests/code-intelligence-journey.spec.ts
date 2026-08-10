import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey verifies one trail across several public workflow projections */

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

async function json(page: Page, method: "get" | "post" | "patch", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}

test("a shared code investigation becomes an owner-reviewed verified change", async ({ browser }) => {
  test.setTimeout(240_000);
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() => run("docker", ["pull", "alpine:3.22"]));
  const copies: string[] = [];
  try {
    const suffix = Date.now().toString(36);
    const developerContext = await browser.newContext();
    const developerPage = await developerContext.newPage();
    const ownerPage = await (await browser.newContext()).newPage();
    const developer = await account(developerPage, "Intelligence Developer", `intel-developer-${suffix}`);
    const affectedOwner = await account(ownerPage, "Affected Owner", `intel-owner-${suffix}`);

    const provider = await json(developerPage, "post", "/repositories", developer.headers, { name: `authorization-${suffix}` }) as { id: string };
    const consumer = await json(ownerPage, "post", "/repositories", affectedOwner.headers, { name: `dashboard-${suffix}` }) as { id: string };
    await json(developerPage, "patch", `/repositories/${provider.id}`, developer.headers, { visibility: "public" });
    await json(ownerPage, "patch", `/repositories/${consumer.id}`, affectedOwner.headers, { visibility: "public" });
    await json(developerPage, "post", `/repositories/${provider.id}/collaborators`, developer.headers, { user_id: affectedOwner.user.id });

    const credential = await json(developerPage, "post", "/auth/credentials", developer.headers, {
      kind: "git", name: "code intelligence journey", scopes: ["git:read", "git:write"], expires_in: 3600,
    }) as { token: string };
    const providerCopy = await mkdtemp(join(tmpdir(), "vivarium-intelligence-provider-"));
    copies.push(providerCopy);
    await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${provider.id}.git`, providerCopy);
    await git(providerCopy, "config", "user.name", "Intelligence Developer");
    await git(providerCopy, "config", "user.email", "developer@example.com");
    await mkdir(join(providerCopy, ".vivarium"));
    await writeFile(join(providerCopy, "authorize.go"), "package access\n\nfunc Authorize(identity string) bool {\n\treturn identity != \"\"\n}\n");
    await writeFile(join(providerCopy, "authorize_test.go"), "package access\n\nimport \"testing\"\n\nfunc TestAuthorizeRejectsBlank(t *testing.T) {\n\tif Authorize(\"\") { t.Fatal(\"blank identity was accepted\") }\n}\n");
    await writeFile(join(providerCopy, ".vivarium", "checks.json"), JSON.stringify({
      version: 1,
      checks: [{ name: "authorization behavior", image: "alpine:3.22", command: "grep -q 'TrimSpace(identity)' authorize.go && grep -q 'whitespace identity was accepted' authorize_test.go" }],
    }));
    await git(providerCopy, "add", ".");
    await git(providerCopy, "commit", "-m", "Document authorization boundary");
    await git(providerCopy, "push", "origin", "main");
    const base = await git(providerCopy, "rev-parse", "HEAD");

    const consumerCredential = await json(ownerPage, "post", "/auth/credentials", affectedOwner.headers, {
      kind: "git", name: "consumer setup", scopes: ["git:read", "git:write"], expires_in: 3600,
    }) as { token: string };
    const consumerCopy = await mkdtemp(join(tmpdir(), "vivarium-intelligence-consumer-"));
    copies.push(consumerCopy);
    await git(tmpdir(), "clone", `http://git:${consumerCredential.token}@localhost:3000/git/${consumer.id}.git`, consumerCopy);
    await git(consumerCopy, "config", "user.name", "Affected Owner");
    await git(consumerCopy, "config", "user.email", "owner@example.com");
    await writeFile(join(consumerCopy, "dashboard.go"), "package dashboard\n\n// The dashboard relies on the authorization contract.\nconst authorizationContract = \"non-empty-identity\"\n");
    await git(consumerCopy, "add", "."); await git(consumerCopy, "commit", "-m", "Consume authorization contract"); await git(consumerCopy, "push", "origin", "main");
    const consumerBase = await git(consumerCopy, "rev-parse", "HEAD");

    const release = await json(developerPage, "post", `/repositories/${provider.id}/releases`, developer.headers, { version: "v1.0.0", notes: "Non-empty identity contract", commit_id: base }) as { id: string };
    await json(developerPage, "post", `/repositories/${provider.id}/interfaces`, developer.headers, { name: "authorization", release_id: release.id });
    await json(ownerPage, "post", `/repositories/${consumer.id}/dependencies`, affectedOwner.headers, { provider_repository_id: provider.id, interface_name: "authorization", constraint: ">=v1.0.0 <v2.0.0", commit_id: consumerBase });

    // Warm the dynamic workspaces before client navigation. Next's development
    // compiler otherwise occasionally leaves a prefetched transition waiting
    // for its first route compilation rather than issuing that compilation.
    await developerPage.request.get(`/repositories/${provider.id}/explanations?ref=${base}`);
    await developerPage.request.get(`/repositories/${provider.id}/impact`);

    await developerPage.goto(`/repositories/${provider.id}/code`);
    await developerPage.getByLabel("Symbol or text").fill("Authorize");
    await developerPage.getByLabel("Exact commit SHA").fill(base);
    await developerPage.getByRole("button", { name: "Navigate" }).click();
    await expect(developerPage.getByRole("link", { name: "authorize.go:3" })).toBeVisible();
    await expect(developerPage.getByText("definition", { exact: true }).first()).toBeVisible();
    await expect(developerPage.getByText("test", { exact: true }).first()).toBeVisible();
    const investigationLink = developerPage.getByRole("link", { name: "Ask about this evidence" });
    await expect(investigationLink).toHaveAttribute("href", `/repositories/${provider.id}/explanations?ref=${base}`);
    await developerPage.goto(`/repositories/${provider.id}/explanations?ref=${base}`);
    await expect(developerPage.getByRole("heading", { name: "Code investigations" })).toBeVisible();
    const explanationResponse = await developerPage.request.post(`/api/repositories/${provider.id}/explanations`, {
      headers: { ...developer.headers, "Content-Type": "application/json" },
      data: { question: "How does Authorize fail closed, and what consumer behavior could change?", ref: base, context: { kind: "file", path: "authorize.go" } },
    });
    expect(explanationResponse.status()).toBe(201);
    const explanationEvents = (await explanationResponse.text()).trim().split("\n").map((line) => JSON.parse(line));
    const investigation = explanationEvents.findLast((event) => event.event === "done").conversation;
    expect(investigation.revision).toBe(base);
    expect(investigation.claims.some((claim: any) => claim.citations?.some((citation: any) => citation.path === "authorize.go"))).toBe(true);

    await json(developerPage, "post", `/repositories/${provider.id}/explanations/${investigation.id}/participants`, developer.headers, { user_id: affectedOwner.user.id });
    const refined = await json(ownerPage, "post", `/repositories/${provider.id}/explanations/${investigation.id}/entries`, affectedOwner.headers, {
      kind: "conclusion", body: "Trim surrounding whitespace before the non-empty check, and verify blank and whitespace-only identities for the dashboard consumer.",
    }) as any;
    const conclusion = refined.entries.findLast((entry: any) => entry.kind === "conclusion");
    expect(conclusion.actor_id).toBe(affectedOwner.user.id);

    await developerPage.goto(`/repositories/${provider.id}/impact`);
    await expect(developerPage.getByRole("heading", { name: "Prospective change impact" })).toBeVisible();
    let assessment = await json(developerPage, "post", `/repositories/${provider.id}/impact-assessments`, developer.headers, {
      title: "Normalize identities without surprising consumers", ref: base, query: "Authorize",
      source: { kind: "investigation_conclusion", explanation_id: investigation.id, entry_id: conclusion.id },
    }) as any;
    expect(assessment.source.explanation_id).toBe(investigation.id);
    expect(assessment.items.some((item: any) => item.repository_id === consumer.id || item.evidence?.some((evidence: any) => evidence.repository_id === consumer.id))).toBe(true);

    assessment = await json(developerPage, "post", `/repositories/${provider.id}/impact-assessments/${assessment.id}/acknowledgement-requests`, developer.headers, {
      version: assessment.version, repository_id: consumer.id, note: "Confirm the dashboard treats whitespace-only identities as unauthenticated.",
    }) as any;
    assessment = await json(developerPage, "post", `/repositories/${provider.id}/impact-assessments/${assessment.id}/participants`, developer.headers, { version: assessment.version, user_id: affectedOwner.user.id }) as any;
    assessment = await json(ownerPage, "post", `/repositories/${provider.id}/impact-assessments/${assessment.id}/acknowledgement-requests/${assessment.acknowledgement_requests[0].id}`, affectedOwner.headers, {
      version: assessment.version, note: "Dashboard ownership confirms whitespace-only identities must fail closed.",
    }) as any;
    expect(assessment.acknowledgement_requests[0].acknowledged_by).toBe(affectedOwner.user.id);

    const implementation = await json(developerPage, "post", `/repositories/${provider.id}/impact-assessments/${assessment.id}/implementation`, developer.headers, {
      version: assessment.version, title: "Normalize authorization identities", body: "Implements the owner-refined conclusion and acknowledged consumer impact.",
      item_ids: [assessment.items[0].id], tasks: [{ title: "Trim and verify identities", outcome: "Whitespace-only identities fail the authorization behavior check.", assignee_type: "agent", depends_on_previous: false }],
    }) as any;
    assessment = implementation.assessment;
    const proposal = implementation.proposal;
    const task = implementation.tasks[0];
    await developerPage.goto(`/proposals/${provider.id}/${proposal.id}`);
    await expect(developerPage.getByText("Normalize authorization identities", { exact: true })).toBeVisible();
    const launched = await json(developerPage, "post", `/repositories/${provider.id}/proposals/${proposal.id}/tasks/${task.id}/sessions`, developer.headers, {
      expected_assignment_id: task.assignment.id, context_paths: ["authorize.go", "authorize_test.go"], expires_in: 3600,
    }) as any;
    expect(launched.session.task_context.reasoning.assessment_id).toBe(assessment.id);

    const agentCopy = await mkdtemp(join(tmpdir(), "vivarium-intelligence-agent-"));
    copies.push(agentCopy);
    await git(tmpdir(), "clone", `http://git:${launched.credential.token}@localhost:3000/git/${provider.id}.git`, agentCopy);
    await git(agentCopy, "config", "user.name", "Vivarium Intelligence Agent"); await git(agentCopy, "config", "user.email", "agent@users.vivarium");
    await git(agentCopy, "switch", launched.run.working_branch);
    await writeFile(join(agentCopy, "authorize.go"), "package access\n\nimport \"strings\"\n\nfunc Authorize(identity string) bool {\n\treturn strings.TrimSpace(identity) != \"\"\n}\n");
    await writeFile(join(agentCopy, "authorize_test.go"), "package access\n\nimport \"testing\"\n\nfunc TestAuthorizeRejectsBlank(t *testing.T) {\n\tfor _, identity := range []string{\"\", \"   \"} {\n\t\tif Authorize(identity) { t.Fatal(\"whitespace identity was accepted\") }\n\t}\n}\n");
    await git(agentCopy, "add", "."); await git(agentCopy, "commit", "-m", "Normalize authorization identities"); await git(agentCopy, "push", "origin", launched.run.working_branch);
    const outcome = await git(agentCopy, "rev-parse", "HEAD");
    const runPath = `/repositories/${provider.id}/proposals/${proposal.id}/tasks/${task.id}/sessions/${launched.session.id}/runs/${launched.run.id}`;
    await json(developerPage, "post", `${runPath}/completion`, { Authorization: `Bearer ${launched.credential.token}` }, {
      summary: "Normalized identities and added the identified whitespace regression.", commit_id: outcome,
      checks: [{ name: "authorization behavior", status: "passed", details: "Blank and whitespace-only identities are rejected." }], unresolved_concerns: [],
    });
    const pull = await json(developerPage, "post", `/repositories/${provider.id}/proposals/${proposal.id}/tasks/${task.id}/contributions`, developer.headers, {
      title: "Normalize authorization identities", body: "Implements the shared investigation and acknowledged impact decision.", source_branch: launched.run.working_branch,
      target_branch: "main", session_id: launched.session.id, run_id: launched.run.id,
    }) as any;

    await ownerPage.goto(`/pulls/${provider.id}/${pull.id}`);
    await expect(ownerPage.getByText(new RegExp(`Justified by impact assessment ${assessment.id.slice(0, 8)}`))).toBeVisible();
    await expect(ownerPage.getByText("1 frozen claim, risk, or verification item(s) · 1 owner acknowledgement(s).")).toBeVisible();
    await expect(ownerPage.getByText("authorization behavior", { exact: true })).toBeVisible();
    await expect(ownerPage.locator("#checks").getByText("succeeded", { exact: true })).toBeVisible({ timeout: 60_000 });
    await ownerPage.getByRole("button", { name: "Approve" }).click();
    await developerPage.goto(`/pulls/${provider.id}/${pull.id}`);
    await expect(developerPage.getByRole("button", { name: "Merge into main" })).toBeEnabled();
    await developerPage.getByRole("button", { name: "Merge into main" }).click();
    await expect(developerPage.getByText("Merged", { exact: true })).toBeVisible();

    const merged = await json(developerPage, "get", `/repositories/${provider.id}/pulls/${pull.id}`, developer.headers) as any;
    expect(merged.merge_commit_id).toMatch(/^[a-f0-9]{40}$/);
    expect(merged.task_session_id).toBe(launched.session.id);
    expect(merged.proposal_id).toBe(proposal.id);
    expect(merged.task_id).toBe(task.id);
    const durableProposal = await json(developerPage, "get", `/repositories/${provider.id}/proposals/${proposal.id}`, developer.headers) as any;
    expect(durableProposal.reasoning.assessment_id).toBe(assessment.id);
    expect(durableProposal.reasoning.explanation_id).toBe(investigation.id);
    await git(providerCopy, "pull", "--ff-only");
    expect(await readFile(join(providerCopy, "authorize.go"), "utf8")).toContain("strings.TrimSpace(identity)");
  } finally {
    await Promise.all(copies.map((path) => rm(path, { recursive: true, force: true })));
  }
});
