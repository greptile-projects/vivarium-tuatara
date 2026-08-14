import { createPrivateKey, sign } from "node:crypto";
import { expect, test, type Page } from "@playwright/test";

/* eslint-disable @typescript-eslint/no-explicit-any -- this journey joins the complete public funding trail */

const sourceKey = createPrivateKey({
  key: Buffer.from("MC4CAQAwBQYDK2VwBCIEIKMRxpuP9exY91bWbhHe/EcvbolkmgQREbTdOLbETKhY", "base64"),
  format: "der",
  type: "pkcs8",
});

async function account(page: Page, name: string, handle: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(name);
  await page.getByLabel("Handle").fill(handle);
  await page.getByRole("button", { name: "Create account and continue" }).click();
  await expect(page.getByRole("heading", { name: "Create a repository" })).toBeVisible();
  const token = await page.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const user = await (await page.request.get("/api/user", { headers })).json();
  return { headers, user };
}

async function json(page: Page, method: "get" | "post", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}

function proof(reference: string, amount: number) {
  const verified = new Date(); verified.setUTCMilliseconds(123);
  const verifiedAt = verified.toISOString();
  const nonce = `community-transfer-${Date.now()}`;
  const message = ["community", reference, amount, "settled", verifiedAt, nonce].join("\n");
  return {
    source: "community", external_reference: reference, completed_amount: amount,
    status: "settled", verified_at: verifiedAt, nonce,
    signature: sign(null, Buffer.from(message), sourceKey).toString("base64"),
  };
}

test("community backing becomes paid human-agent delivery without buying project authority", async ({ browser }) => {
  const suffix = Date.now().toString(36);
  const ownerPage = await (await browser.newContext()).newPage();
  const developerPage = await (await browser.newContext()).newPage();
  const owner = await account(ownerPage, "Community Steward", `fund-steward-${suffix}`);
  const developer = await account(developerPage, "Roadmap Developer", `fund-developer-${suffix}`);

  const organization = await json(ownerPage, "post", "/organizations", owner.headers, {
    name: `Community Lab ${suffix}`, slug: `community-lab-${suffix}`, description: "Govern shared outcome funding.",
  });
  const invitationState = await json(ownerPage, "post", `/organizations/${organization.id}/invitations`, owner.headers, { user_id: developer.user.id });
  await json(developerPage, "post", `/organizations/${organization.id}/invitations/${invitationState.invitations.at(-1).id}/accept`, developer.headers);
  const repository = await json(ownerPage, "post", `/organizations/${organization.id}/repositories`, owner.headers, { name: `funded-roadmap-${suffix}` });
  const agentState = await json(ownerPage, "post", `/organizations/${organization.id}/agents`, owner.headers, {
    name: "Outcome Verifier", slug: `outcome-verifier-${suffix}`, capabilities: ["bounded implementation and verification"],
    operator_ids: [developer.user.id], team_ids: [],
  });
  const agent = agentState.agents.find((item: any) => item.slug === `outcome-verifier-${suffix}`);

  let fund = await json(ownerPage, "post", `/repositories/${repository.id}/funds`, owner.headers, {
    name: "Community roadmap fund", purpose: "Pay for measured roadmap value", stewards: [owner.user.id],
    accepted_funding_sources: ["community"], unit: "USD", precision: 2,
    spending_limits: [{ period: "monthly", amount: 500 }],
    approval_rules: [{ minimum_amount: 0, required_approvals: 1, eligible_approvers: [owner.user.id] }],
    eligible_recipients: ["contributors", "approved agents"], refund_policy: "Refund rejected or withdrawn value.", ledger_visibility: "public",
  });
  fund = await json(developerPage, "post", `/repositories/${repository.id}/funds/${fund.id}/commitments`, developer.headers, {
    amount: 300, source: "community", external_reference: `roadmap-${suffix}`, idempotency_key: `backing-${suffix}`, note: "Shared backing for a measurable result.",
  });
  const commitment = fund.ledger.find((entry: any) => entry.kind === "commitment");
  fund = await json(ownerPage, "post", `/repositories/${repository.id}/funds/${fund.id}/commitments/${commitment.id}/reconcile`, owner.headers, {
    expected_version: fund.version, status: "settled", completed_amount: 300,
    transfer_proof: proof(`roadmap-${suffix}`, 300), note: "Verified by the operator-controlled community source.",
  });
  expect(fund.balances.available).toBe(300);

  const originalTerms = {
    title: "Make contribution setup measurable", source: { kind: "roadmap_outcome", id: "guided-setup", revision: "roadmap-v1", visibility: "public" },
    scope: "Ship and measure a guided setup path.", acceptance_criteria: ["reviewed change ships", "completion reaches 70 percent"],
    evidence_requirements: ["pull, checks, preview, merge, release, deployment, and outcome measure"], budget: 180,
    deadline: new Date(Date.now() + 86_400_000).toISOString(), contributor_eligibility: ["organization members", "approved agent operators"],
    allocation_method: "maintainer_selection", cancellation_terms: "Rejected work returns to the community fund.", dependencies: [],
    risks: ["agent compute overrun"], conflicts: [], milestones: [
      { id: "implementation", title: "Implement and release", budget: 100, acceptance_criteria: ["ordinary review and merge"], evidence_requirements: ["pull through deployment"], dependencies: [] },
      { id: "measurement", title: "Measure delivered value", budget: 80, acceptance_criteria: ["completion at least 70 percent"], evidence_requirements: ["measured release outcome"], dependencies: ["implementation"] },
    ],
  };
  let outcome = await json(ownerPage, "post", `/repositories/${repository.id}/funded-outcomes`, owner.headers, { fund_id: fund.id, terms: originalTerms });
  outcome = await json(developerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/pledges`, developer.headers, {
    expected_version: outcome.version, amount: 180, idempotency_key: `pledge-${suffix}`, note: "Back both measurable milestones.",
  });
  const pledge = outcome.pledges[0];
  outcome = await json(ownerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/revisions`, owner.headers, {
    expected_version: outcome.version, reason: "Narrow setup to the first repository journey before selection.",
    terms: { ...originalTerms, scope: "Ship and measure guided setup for a contributor's first repository.", source: { ...originalTerms.source, revision: "roadmap-v2" } },
  });
  expect(outcome.pledges[0].status).toBe("reconfirmation_required");
  outcome = await json(developerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/pledges/${pledge.id}`, developer.headers, {
    expected_version: outcome.version, action: "reconfirm", reason: "The narrowed population remains measurable.",
  });

  const proposal = (applicant: any, approach: string, milestone: string, cost: number) => ({
    applicant, terms: { approach, milestones: [milestone], cost, dependencies: [], availability: "This cycle", required_access: ["separately approved task and Git access"], relevant_work: [{ kind: "proposal", id: "guided-setup", note: "Accepted roadmap result" }] },
  });
  outcome = await json(developerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-proposals`, developer.headers,
    proposal({ kind: "human", id: developer.user.id }, "Implement through ordinary review and release.", "Implement and release", 100));
  const humanProposal = outcome.delivery_proposals.at(-1);
  outcome = await json(developerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-proposals/${humanProposal.id}/accept`, developer.headers, { expected_version: outcome.version });
  outcome = await json(developerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-proposals`, developer.headers,
    proposal({ kind: "approved_agent", id: agent.id }, "Operate the approved agent against bounded evidence.", "Measure delivered value", 80));
  const agentProposal = outcome.delivery_proposals.at(-1);
  outcome = await json(developerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-proposals/${agentProposal.id}/accept`, developer.headers, { expected_version: outcome.version });
  outcome = await json(ownerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-selections`, owner.headers, {
    expected_version: outcome.version, proposal_ids: [humanProposal.id, agentProposal.id], reviewer_ids: [owner.user.id],
    conflict_disclosure: "The steward has no financial relationship with either recipient.", rationale: "Human implementation plus bounded agent measurement covers both milestones.",
  });
  const selection = outcome.delivery_selections[0];
  const humanTask = selection.tasks.find((task: any) => task.recipient_kind === "human");
  const agentTask = selection.tasks.find((task: any) => task.recipient_kind === "approved_agent");
  const resource = (kind: string, id: string, revision = "release-commit") => ({ kind, id, revision, status: "succeeded", url: `/repositories/${repository.id}` });
  const deliveryResources = [resource("task", humanTask.id), resource("session", "session-1"), resource("workspace", "workspace-1"), resource("fork", "fork-1"), resource("pull", "pull-1"), resource("check", "checks-1"), resource("preview", "preview-1"), resource("commit", "merge-commit"), resource("release", "v1.0.0"), resource("deployment", "production-1")];
  outcome = await json(developerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-selections/${selection.id}/updates`, developer.headers, {
    expected_version: outcome.version, recipient: { kind: "human", id: developer.user.id }, update: { task_id: humanTask.id, status: "completed", progress: 100, summary: "Implemented, reviewed, checked, previewed, merged, released, and deployed through ordinary authority.", blockers: [], resources: deliveryResources, evidence: deliveryResources.map((item: any) => ({ kind: item.kind === "check" || item.kind === "preview" ? item.kind : "work", summary: `${item.kind} evidence`, resource: item })), agent_minutes: 0 },
  });
  outcome = await json(ownerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-selections/${selection.id}/tasks/${humanTask.id}/reviews`, owner.headers, {
    expected_version: outcome.version, review: { decision: "accepted", rationale: "Independent project authorities completed the full delivery chain.", dissent: [], award_amount: 0, outcome_measures: [{ name: "Release deployed", status: "met", value: "production succeeded", evidence: resource("deployment", "production-1") }] },
  });

  outcome = await json(developerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-selections/${selection.id}/expenses`, developer.headers, {
    expected_version: outcome.version, recipient: { kind: "approved_agent", id: agent.id }, expense: { task_id: agentTask.id, amount: 90, category: "agent_compute", description: "Forecast compute exceeds the remaining agent allocation.", evidence: [resource("session", "agent-session-1")] },
  });
  expect(outcome.delivery_selections[0].execution.spending_blockers).toContain("budget_overrun");
  const expense = outcome.delivery_selections[0].execution.expenses[0];
  outcome = await json(ownerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-selections/${selection.id}/expenses/${expense.id}`, owner.headers, { expected_version: outcome.version, decision: "rejected", reason: "Agent compute exceeds the selected allocation." });
  outcome = await json(ownerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-selections/${selection.id}/controls`, owner.headers, { expected_version: outcome.version, action: "replace_recipient", reason: "Contain the overrun and retain the agent's attributed attempt.", amount: 0, replacement: { kind: "human", id: owner.user.id } });
  const replacementTask = outcome.delivery_selections[0].tasks.find((task: any) => task.id === agentTask.id);
  expect(replacementTask.recipient_id).toBe(owner.user.id);
  outcome = await json(ownerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-selections/${selection.id}/updates`, owner.headers, {
    expected_version: outcome.version, recipient: { kind: "human", id: owner.user.id }, update: { task_id: agentTask.id, status: "completed", progress: 100, summary: "Replacement measured the released experience while preserving the agent attempt.", blockers: [], resources: [resource("release", "v1.0.0"), resource("deployment", "production-1")], evidence: [{ kind: "acceptance", summary: "Measured completion", resource: resource("deployment", "production-1") }], agent_minutes: 0 },
  });
  const decide = async (decision: string, rationale: string, measures: any[] = []) => {
    outcome = await json(ownerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-selections/${selection.id}/tasks/${agentTask.id}/reviews`, owner.headers, { expected_version: outcome.version, review: { decision, rationale, dissent: ["The original agent operator retains attribution for the contained attempt."], award_amount: 0, outcome_measures: measures } });
  };
  await decide("disputed", "The first measurement did not explain excluded retries.");
  outcome = await json(ownerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-selections/${selection.id}/tasks/${agentTask.id}/recoveries`, owner.headers, { expected_version: outcome.version, action: "appeal", reason: "Provide the bounded cohort denominator." });
  await decide("rejected", "The corrected evidence still misses the agreed threshold.");
  outcome = await json(ownerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-selections/${selection.id}/tasks/${agentTask.id}/recoveries`, owner.headers, { expected_version: outcome.version, action: "appeal", reason: "Measure the shipped release after the full observation window." });
  await decide("accepted", "The full window demonstrates delivered value.", [{ name: "First-repository completion", status: "met", value: "74 percent", evidence: resource("deployment", "production-1") }]);
  const paidReview = outcome.delivery_selections[0].tasks.find((task: any) => task.id === agentTask.id).reviews.at(-1);
  outcome = await json(ownerPage, "post", `/repositories/${repository.id}/funded-outcomes/${outcome.id}/delivery-selections/${selection.id}/tasks/${agentTask.id}/recoveries`, owner.headers, { expected_version: outcome.version, action: "refund", reason: "Apply the declared community refund policy after a duplicate provider charge." });

  fund = await json(ownerPage, "get", `/repositories/${repository.id}/funds/${fund.id}`, owner.headers);
  expect(fund.ledger.map((entry: any) => entry.kind)).toEqual(expect.arrayContaining(["commitment", "transfer_reconciliation", "delivery_reservation", "milestone_award", "milestone_refund"]));
  expect(fund.ledger.find((entry: any) => entry.external_reference.endsWith(paidReview.id))).toMatchObject({ contributor_id: owner.user.id, actor_id: owner.user.id });
  await ownerPage.goto(`/repositories/${repository.id}/funding`);
  await expect(ownerPage.getByRole("heading", { name: "Outcome funding" })).toBeVisible();
  await ownerPage.getByText(/Attributable replanning/).click();
  await expect(ownerPage.getByText("Narrow setup to the first repository journey before selection.", { exact: false })).toBeVisible();
  await expect(ownerPage.getByText(`Settlement receipt ${paidReview.id}`, { exact: true })).toBeVisible();
  await expect(ownerPage.getByText("Forecast compute exceeds the remaining agent allocation.", { exact: true })).toBeVisible();
  await ownerPage.goto(`/repositories/${repository.id}/funds`);
  await expect(ownerPage.getByRole("heading", { name: "Project funds" })).toBeVisible();
  await ownerPage.getByText(/Ledger ·/).click();
  await expect(ownerPage.getByText("milestone refund", { exact: false })).toBeVisible();
});
