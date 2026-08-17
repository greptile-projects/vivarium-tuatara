import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey intentionally follows one cross-owner retained trail */
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
  const headers = { Authorization: `Bearer ${token}` };
  return { headers, user: await (await page.request.get("/api/user", { headers })).json() as any };
}
async function json(page: Page, method: "get" | "post" | "patch", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}
async function rejected(page: Page, method: "post", path: string, headers: Record<string, string>, data: unknown, status?: number) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  if (status) expect(response.status(), await response.text()).toBe(status);
  else expect(response.status(), await response.text()).toBeGreaterThanOrEqual(400);
  return response;
}

test("independent teams take a reviewed API through agent adoption and safe retirement", async ({ browser }) => {
  test.setTimeout(300_000);
  const copies: string[] = [];
  try {
    const suffix = Date.now().toString(36);
    const producerPage = await (await browser.newContext()).newPage();
    const consumerPage = await (await browser.newContext()).newPage();
    const successorPage = await (await browser.newContext()).newPage();
    const reviewerPage = await (await browser.newContext()).newPage();
    const producer = await account(producerPage, "Ledger API Producer", `api-producer-${suffix}`);
    const consumer = await account(consumerPage, "Ledger API Consumer", `api-consumer-${suffix}`);
    const successor = await account(successorPage, "Consumer On Call", `api-oncall-${suffix}`);
    const reviewer = await account(reviewerPage, "Contract Reviewer", `api-reviewer-${suffix}`);
    const providerRepo = await json(producerPage, "post", "/repositories", producer.headers, { name: `ledger-api-${suffix}` });
    const consumerRepo = await json(consumerPage, "post", "/repositories", consumer.headers, { name: `ledger-client-${suffix}` });
    await json(producerPage, "patch", `/repositories/${providerRepo.id}`, producer.headers, { visibility: "public" });
    await json(consumerPage, "patch", `/repositories/${consumerRepo.id}`, consumer.headers, { visibility: "public" });
    await json(producerPage, "post", `/repositories/${providerRepo.id}/collaborators`, producer.headers, { user_id: reviewer.user.id });
    await json(consumerPage, "post", `/repositories/${consumerRepo.id}/collaborators`, consumer.headers, { user_id: successor.user.id });
    await json(consumerPage, "post", `/repositories/${consumerRepo.id}/collaborators`, consumer.headers, { user_id: reviewer.user.id });

    async function clone(page: Page, actor: any, repositoryID: string, prefix: string) {
      const credential = await json(page, "post", "/auth/credentials", actor.headers, { kind: "git", name: prefix, scopes: ["git:read", "git:write"], expires_in: 3600 });
      const copy = await mkdtemp(join(tmpdir(), prefix)); copies.push(copy);
      await git(tmpdir(), "clone", `http://git:${credential.token}@localhost:3000/git/${repositoryID}.git`, copy);
      await git(copy, "config", "user.name", actor.user.display_name);
      await git(copy, "config", "user.email", `${actor.user.handle}@example.test`);
      return copy;
    }
    const providerCopy = await clone(producerPage, producer, providerRepo.id, "vivarium-api-provider-");
    const consumerCopy = await clone(consumerPage, consumer, consumerRepo.id, "vivarium-api-consumer-");
    await writeFile(join(providerCopy, "README.md"), "# Ledger service\n");
    await git(providerCopy, "add", "."); await git(providerCopy, "commit", "-m", "Initialize ledger service"); await git(providerCopy, "push", "origin", "main");
    await writeFile(join(consumerCopy, "README.md"), "# Independent ledger client\n");
    await git(consumerCopy, "add", "."); await git(consumerCopy, "commit", "-m", "Initialize independent client"); await git(consumerCopy, "push", "origin", "main");

    const openapi = (version: number) => JSON.stringify({ openapi: "3.1.0", info: { title: "Ledger API", version: `${version}.0.0` }, paths: version === 1 ? {
      "/entries": { get: { operationId: "listEntries", responses: { "200": { description: "entries" } } } },
    } : { "/ledger-entries": { get: { operationId: "listLedgerEntries", responses: { "200": { description: "entries" } } } } } });
    async function reviewedProviderPull(branch: string, version: number) {
      await git(providerCopy, "switch", "main"); await git(providerCopy, "pull", "--ff-only"); await git(providerCopy, "switch", "-c", branch);
      await writeFile(join(providerCopy, "openapi.json"), openapi(version));
      await writeFile(join(providerCopy, "API.md"), `# Ledger API v${version}\nSynthetic sandbox; no production credentials.\n`);
      await git(providerCopy, "add", "."); await git(providerCopy, "commit", "-m", `Review ledger API v${version}`); await git(providerCopy, "push", "origin", branch);
      return json(producerPage, "post", `/repositories/${providerRepo.id}/pulls`, producer.headers, { title: `Ledger API v${version}`, body: "Reviewed implementation, interface, and data-use policy.", source_branch: branch, target_branch: "main" });
    }
    async function approveAndMerge(repositoryID: string, pull: any, ownerPage: Page, headers: Record<string, string>) {
      await json(reviewerPage, "post", `/repositories/${repositoryID}/pulls/${pull.id}/reviews`, reviewer.headers, { decision: "approved" });
      return json(ownerPage, "post", `/repositories/${repositoryID}/pulls/${pull.id}/merge`, headers, {});
    }
    const v1Pull = await reviewedProviderPull("contract-v1", 1);
    const v1Merge = await approveAndMerge(providerRepo.id, v1Pull, producerPage, producer.headers);
    const v1Release = await json(producerPage, "post", `/repositories/${providerRepo.id}/releases`, producer.headers, { version: "v1.0.0", notes: "Reviewed ledger contract and sandbox", commit_id: v1Merge.merge_commit_id });
    const revision = (version: number, pull: any, commit: string, releaseID: string) => ({
      version_label: `v${version}.0.0`, title: "Ledger API", summary: "Reviewed ledger reads with synthetic adoption support",
      source: { commit_id: commit, pull_request_id: pull.id, release_id: releaseID, definition_path: "openapi.json", documentation_path: "API.md" },
      operations: [{ id: version === 1 ? "listEntries" : "listLedgerEntries", method: "GET", path: version === 1 ? "/entries" : "/ledger-entries", summary: "List ledger entries", authentication: ["app"], parameters: [], response_schema_ids: ["entries"], error_ids: ["failed"], stability: "stable", owner_ids: [producer.user.id] }],
      schemas: [{ id: "entries", name: "Entries", kind: "object", definition: '{"type":"object"}', required_fields: [], description: "Synthetic entries" }],
      errors: [{ id: "failed", code: "request_failed", http_status: 503, meaning: "Service unavailable", recovery: "Retry the inspected synthetic request" }],
      authentication: [{ id: "app", mode: "application", description: "Scoped application secret", scopes: ["ledger.read"] }],
      environments: [{ id: "sandbox", name: "Sandbox", base_url: "https://sandbox.example.test", availability: "available", regions: ["test"] }],
      limits: { requests: 20, window_seconds: 3600, burst: 2, payload_bytes: 4096, concurrency: 1 }, owner_ids: [producer.user.id], stability: "stable",
      support_policy: { channels: ["support"], response_target: "one business day", deprecation_notice_days: 30, sunset_notice_days: 30 },
      links: [{ kind: "documentation", url: "https://example.test/ledger", label: "Reviewed API guide" }, { kind: "data_use", url: "https://example.test/data", label: "Synthetic data policy" }],
      compatibility: { from_version: version === 1 ? "none" : "v1.0.0", level: version === 1 ? "initial" : "breaking", promise: "Exact reviewed versions remain attributable", breaking_changes: version === 1 ? [] : ["GET /entries replaced by GET /ledger-entries"] }, known_gaps: [], rationale: "Enable governed independent adoption",
    });
    const contract = await json(producerPage, "post", `/repositories/${providerRepo.id}/api-contracts`, producer.headers, { expected_version: 0, revision: revision(1, v1Pull, v1Merge.merge_commit_id, v1Release.id) });

    await rejected(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications`, consumer.headers, { name: "Ledger client", contract_version: 1, environments: ["sandbox"], capabilities: ["deleteEntries"] }, 422);
    const application = await json(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications`, consumer.headers, { name: "Ledger client", project_url: `https://example.test/${consumerRepo.id}`, contract_version: 1, environments: ["sandbox"], capabilities: ["listEntries"] });
    await rejected(producerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/decision`, producer.headers, { status: "approved", reason: "Scope widening denied", capabilities: ["deleteEntries"], expires_at: new Date(Date.now() + 86_400_000).toISOString() }, 400);
    await json(producerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/decision`, producer.headers, { status: "approved", reason: "Exact read-only sandbox scope", capabilities: ["listEntries"], expires_at: new Date(Date.now() + 86_400_000).toISOString() });
    const issued = await json(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/credentials`, consumer.headers, { lifetime_hours: 1 });
    const sandboxHeaders = { Authorization: `Bearer ${issued.credential.secret}` };
    const sandbox = await json(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/sandbox`, sandboxHeaders, { operation_id: "listEntries", request: { cursor: "synthetic" } });
    expect(sandbox).toMatchObject({ response: { status: 200 }, quota: { synthetic_only: true } });

    const consumerBase = await git(consumerCopy, "rev-parse", "HEAD");
    const work = await json(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/integration-work`, consumer.headers, { consumer_repository_id: consumerRepo.id, consumer_revision: consumerBase, kind: "session", owner_type: "agent", owner_id: `consumer-agent-${suffix}`, title: "Agent builds the ledger integration" });
    expect(work.preload).toMatchObject({ synthetic_only: true, credentials_included: false, sandbox_operations: ["listEntries"] });

    const v2Pull = await reviewedProviderPull("contract-v2", 2);
    await git(providerCopy, "switch", "main");
    await writeFile(join(providerCopy, "README.md"), "# Ledger service\n\nOperational ownership clarified after v1 publication.\n");
    await git(providerCopy, "add", "README.md"); await git(providerCopy, "commit", "-m", "Clarify service ownership"); await git(providerCopy, "push", "origin", "main");
    await git(consumerCopy, "switch", "-c", "ledger-v2"); await writeFile(join(consumerCopy, "client.txt"), "GET /ledger-entries\n"); await git(consumerCopy, "add", "."); await git(consumerCopy, "commit", "-m", "Agent adopts ledger v2"); await git(consumerCopy, "push", "origin", "ledger-v2");
    const consumerPull = await json(consumerPage, "post", `/repositories/${consumerRepo.id}/pulls`, consumer.headers, { title: "Agent adopts ledger v2", body: "Credential-free integration generated from the frozen contract.", source_branch: "ledger-v2", target_branch: "main" });
    const candidateResult = await json(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/integration-work/${work.id}/candidates`, consumer.headers, { consumer_repository_id: consumerRepo.id, producer_pull_request_id: v2Pull.id, consumer_pull_request_id: consumerPull.id, scenarios: [{ name: "provider conformance", owner_side: "producer", command: "verify OpenAPI v2" }, { name: "consumer integration", owner_side: "consumer", command: "test generated client" }] });
    const failedCandidate = candidateResult.candidates.at(-1);
    for (const evidence of [{ scenario: "provider conformance", side: "producer", status: "passed" }, { scenario: "consumer integration", side: "consumer", status: "failed" }])
      await json(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/integration-work/${work.id}/candidates/${failedCandidate.id}/evidence`, consumer.headers, { ...evidence, requests: ["sanitized synthetic request"], responses: ["schema mismatch"], logs: ["bounded conformance summary"], artifacts: [{ name: "report.json", sha256: "a".repeat(64), size: 128 }], coverage_percent: 90, cost_units: 3 });

    const consumerMerge = await approveAndMerge(consumerRepo.id, consumerPull, consumerPage, consumer.headers);
    const consumerRelease = await json(consumerPage, "post", `/repositories/${consumerRepo.id}/releases`, consumer.headers, { version: "v2.0.0", notes: "Reviewed agent-built ledger integration", commit_id: consumerMerge.merge_commit_id });
    expect(consumerRelease.commit_id).toBe(consumerMerge.merge_commit_id);
    await git(providerCopy, "switch", "main"); await git(providerCopy, "pull", "--ff-only");
    const stale = await json(producerPage, "get", `/repositories/${providerRepo.id}/api-contracts/${contract.id}`, producer.headers);
    expect(stale.diagnostics).toEqual(expect.arrayContaining([expect.objectContaining({ code: "stale_documentation" })]));

    const failureWindow = await json(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/operations/observations`, consumer.headers, { environment: "sandbox", release_id: v1Release.id, window_started_at: new Date(Date.now() - 60_000).toISOString(), window_ended_at: new Date().toISOString(), requests: 10, available: 6, latency_p95_ms: 900, quota_rejected: 0, errors: 4, schema_valid: 6, usage_units: 10, error_codes: ["upstream_unavailable"], sanitization: "Aggregate counts and error class only; payloads and credentials removed", visibility: "shared" });
    const investigation = await json(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/operations/investigations`, consumer.headers, { title: "Shared ledger availability failure", observation_ids: [failureWindow.id] });
    await json(producerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/operations/investigations/${investigation.id}/reproductions`, producer.headers, { observation_id: failureWindow.id, operation_id: "listEntries", failure: "server_error" });
    const diagnosed = await json(producerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/operations/investigations/${investigation.id}/findings`, producer.headers, { classification: "service", summary: "Provider returned a deterministic availability failure", evidence_ids: [failureWindow.id], confidence: "high", uncertainty: "No production payload was retained" });
    expect(diagnosed.reproductions[0]).toMatchObject({ synthetic_only: true, payload_retained: false });

    await json(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/exposure`, consumer.headers, {});
    await rejected(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/sandbox`, sandboxHeaders, { operation_id: "listEntries", request: {} }, 401);
    await json(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/ownership`, consumer.headers, { owner_id: successor.user.id });
    await json(producerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/decision`, producer.headers, { status: "approved", reason: "On-call ownership and scope revalidated after exposure", capabilities: ["listEntries"], expires_at: new Date(Date.now() + 86_400_000).toISOString() });

    const v2Merge = await approveAndMerge(providerRepo.id, v2Pull, producerPage, producer.headers);
    const v2Release = await json(producerPage, "post", `/repositories/${providerRepo.id}/releases`, producer.headers, { version: "v2.0.0", notes: "Breaking ledger service rollout", commit_id: v2Merge.merge_commit_id });
    await json(producerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/revisions`, producer.headers, { expected_version: 1, revision: revision(2, v2Pull, v2Merge.merge_commit_id, v2Release.id) });
    const iface = await json(producerPage, "post", `/repositories/${providerRepo.id}/interfaces`, producer.headers, { name: "ledger", release_id: v1Release.id });
    const proposal = await json(producerPage, "post", `/repositories/${providerRepo.id}/proposals`, producer.headers, { title: "Migrate ledger consumers", body: "Coordinate tested migration before retiring v1." });
    const evolution = await json(producerPage, "post", `/repositories/${providerRepo.id}/evolutions`, producer.headers, { interface_name: "ledger", predecessor_interface_id: iface.id, source_kind: "proposal", source_id: proposal.id, candidate_description: "Ledger v2 removes GET /entries", changes: [{ kind: "operation", summary: "replace the list path", classification: "breaking" }], strategy: "dual-version conformance", sequencing: "consumer before provider", exceptions: "bounded sunset exceptions only" });
    let migration = await json(producerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations`, producer.headers, { kind: "new_version", from_version: 1, to_version: 2, evolution_id: evolution.id, changes: [{ kind: "removed_operation", summary: "GET /entries is removed", classification: "breaking" }], stages: [{ id: "dual", name: "Dual run", deadline: new Date(Date.now() + 86_400_000).toISOString(), required_evidence: "passing producer and consumer scenarios", observation_max_age_hours: 24, max_remaining_requests: 10 }, { id: "sunset", name: "Sunset v1", deadline: new Date(Date.now() + 172_800_000).toISOString(), required_evidence: "passing conformance and zero old traffic", observation_max_age_hours: 24, max_remaining_requests: 0 }] });
    await json(successorPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/revoke`, successor.headers, {});
    migration = await json(producerPage, "get", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}`, producer.headers);
    expect(migration.readiness.consumers[0]).toMatchObject({ access_state: "revoked" });
    await json(successorPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/ownership`, successor.headers, { owner_id: consumer.user.id });
    await json(producerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/decision`, producer.headers, { status: "approved", reason: "Consumer availability and original ownership restored", capabilities: ["listEntries"], expires_at: new Date(Date.now() + 86_400_000).toISOString() });
    await json(successorPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}/consumer-actions`, successor.headers, { expected_version: migration.version, application_id: application.id, action: "acknowledge", note: "Consumer on-call owns migration" });
    migration = await json(producerPage, "get", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}`, producer.headers);
    await json(successorPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}/consumer-actions`, successor.headers, { expected_version: migration.version, application_id: application.id, action: "attest", integration_work_id: work.id, candidate_id: failedCandidate.id });
    migration = await json(producerPage, "get", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}`, producer.headers);
    expect(migration.readiness.ready).toBe(false);

    const passingResult = await json(successorPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/integration-work/${work.id}/candidates`, successor.headers, { consumer_repository_id: consumerRepo.id, producer_pull_request_id: v2Pull.id, consumer_pull_request_id: consumerPull.id, scenarios: [{ name: "provider conformance v2", owner_side: "producer", command: "verify reviewed OpenAPI v2" }, { name: "consumer integration v2", owner_side: "consumer", command: "verify released client v2" }] });
    const passing = passingResult.candidates.at(-1);
    for (const evidence of [{ scenario: "provider conformance v2", side: "producer" }, { scenario: "consumer integration v2", side: "consumer" }])
      await json(successorPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/integration-work/${work.id}/candidates/${passing.id}/evidence`, successor.headers, { ...evidence, status: "passed", requests: ["sanitized dual-version request"], responses: ["schema conforms"], logs: ["all bounded tests passed"], artifacts: [{ name: "attestation.json", sha256: "b".repeat(64), size: 96 }], coverage_percent: 100, cost_units: 2 });
    migration = await json(producerPage, "get", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}`, producer.headers);
    await json(successorPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}/consumer-actions`, successor.headers, { expected_version: migration.version, application_id: application.id, action: "attest", integration_work_id: work.id, candidate_id: passing.id });
    migration = await json(producerPage, "get", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}`, producer.headers);
    await json(successorPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}/consumer-actions`, successor.headers, { expected_version: migration.version, application_id: application.id, action: "exception", reason: "On-call needs a brief sunset overlap", expires_at: new Date(Date.now() + 1500).toISOString() });
    migration = await json(producerPage, "get", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}`, producer.headers);
    await rejected(producerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}/stages`, producer.headers, { expected_version: migration.version, stage_id: "sunset", retire: true }, 409);
    await new Promise((resolve) => setTimeout(resolve, 1700));
    const zeroTraffic = await json(consumerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/applications/${application.id}/operations/observations`, consumer.headers, { environment: "sandbox", release_id: v1Release.id, window_started_at: new Date(Date.now() - 60_000).toISOString(), window_ended_at: new Date().toISOString(), requests: 0, available: 0, latency_p95_ms: 0, quota_rejected: 0, errors: 0, schema_valid: 0, usage_units: 0, error_codes: [], sanitization: "Aggregate zero old-version traffic; no consumer data", visibility: "shared" });
    expect(zeroTraffic.requests).toBe(0);
    migration = await json(producerPage, "get", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}`, producer.headers);
    expect(migration.readiness).toMatchObject({ ready: true });
    const retired = await json(producerPage, "post", `/repositories/${providerRepo.id}/api-contracts/${contract.id}/migrations/${migration.id}/stages`, producer.headers, { expected_version: migration.version, stage_id: "sunset", retire: true });
    expect(retired).toMatchObject({ state: "retired", current_stage: "sunset" });

    await producerPage.goto(`/repositories/${providerRepo.id}/api-contracts`);
    await expect(producerPage.getByRole("heading", { name: "API contracts" })).toBeVisible();
    await expect(producerPage.getByText("v2.0.0", { exact: true }).first()).toBeVisible();
    await producerPage.getByRole("link", { name: /Open consumer integration sandbox/ }).click();
    await expect(producerPage.getByText("Shared operational evidence", { exact: true }).first()).toBeVisible();
    await expect(producerPage.getByText("Ledger client", { exact: true })).toBeVisible();
  } finally {
    await Promise.all(copies.map((path) => rm(path, { recursive: true, force: true })));
  }
});
