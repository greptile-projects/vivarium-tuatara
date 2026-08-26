import { expect, test, type APIResponse, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

const run = promisify(execFile);
async function git(token: string, cwd: string, ...args: string[]) {
  return (await run("git", ["-c", "credential.helper=", ...args], { cwd, env: { ...process.env, GIT_ASKPASS: join(__dirname, "git-askpass.sh"), GIT_TERMINAL_PROMPT: "0", VIVARIUM_GIT_TOKEN: token } })).stdout.trim();
}
async function account(page: Page, suffix: string, role: string) {
  await page.goto("/");
  await page.getByLabel("Display name").fill(`${role} responder`);
  await page.getByLabel("Handle").fill(`response-${role.toLowerCase()}-${suffix}`);
  await page.getByRole("button", { name: "Create account and continue" }).click();
  await expect(page.getByRole("heading", { name: "Create a repository" })).toBeVisible();
  const token = await page.evaluate(() => localStorage.getItem("vivarium.access-token"));
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  return { token: token!, headers, user: await (await page.request.get("/api/user", { headers })).json() };
}
async function json(page: Page, method: "get" | "post" | "put" | "delete", path: string, headers: Record<string, string>, data?: unknown) {
  const response = await page.request[method](`/api${path}`, { headers, data });
  const body = await response.text();
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeGreaterThanOrEqual(200);
  expect(response.status(), `${method.toUpperCase()} ${path}: ${body}`).toBeLessThan(300);
  return body ? JSON.parse(body) : undefined;
}
async function rejected(response: APIResponse, status: number, code: string) {
  const body = await response.text(); expect(response.status(), body).toBe(status); expect(JSON.parse(body)).toMatchObject({ error: { code } });
}

test("a released service signal becomes an owned response, handoff, incident, and reviewed improvement", async ({ browser }) => {
  test.setTimeout(180_000);
  const copy = await mkdtemp(join(tmpdir(), "vivarium-response-"));
  try {
    const suffix = Date.now().toString(36);
    const ownerPage = await (await browser.newContext()).newPage();
    const primaryPage = await (await browser.newContext()).newPage();
    const backupPage = await (await browser.newContext()).newPage();
    const dependencyPage = await (await browser.newContext()).newPage();
    const owner = await account(ownerPage, suffix, "Owner"), primary = await account(primaryPage, suffix, "Primary"), backup = await account(backupPage, suffix, "Backup"), dependency = await account(dependencyPage, suffix, "Dependency");
    const repository = await json(ownerPage, "post", "/repositories", owner.headers, { name: `released-checkout-${suffix}` });
    for (const participant of [primary, backup, dependency]) await json(ownerPage, "post", `/repositories/${repository.id}/collaborators`, owner.headers, { user_id: participant.user.id });
    const credential = await json(ownerPage, "post", "/auth/credentials", owner.headers, { kind: "git", name: "response journey", scopes: ["git:read", "git:write"], expires_in: 3600 });
    await git(credential.token, tmpdir(), "clone", `http://localhost:3000/git/${repository.id}.git`, copy);
    await git("", copy, "config", "user.name", "Service Owner"); await git("", copy, "config", "user.email", "owner@example.test");
    await writeFile(join(copy, "service.txt"), "checkout release v1\n");
    await writeFile(join(copy, "RUNBOOK.md"), "# Checkout response\n\nConfirm error rate and ask the dependency owner.\n");
    await git("", copy, "add", "."); await git("", copy, "commit", "-m", "Release checkout with its response runbook"); await git(credential.token, copy, "push", "origin", "main");
    const revision = await git("", copy, "rev-parse", "HEAD");
    const release = await json(ownerPage, "post", `/repositories/${repository.id}/releases`, owner.headers, { version: `v1-${suffix}`, notes: "Released checkout service", commit_id: revision });
    const now = Date.now(), iso = (offset = 0) => new Date(now + offset).toISOString();
    const authority = { required_access: ["repository:read", "environment:operate"], permitted_actions: ["investigate", "request dependency evidence", "propose mitigation"], prohibited_actions: ["deploy without environment authority", "read secrets", "expand agent access"] };
    const policyRevision = { title: "Checkout response coverage", summary: "Accountable coverage for the released checkout service and payment dependency", resources: [{ id: "checkout", kind: "service", name: "checkout", owner_team_ids: ["service"] }, { id: "payments", kind: "dependency", name: "payments", owner_team_ids: ["dependency"] }], teams: [{ id: "service", name: "Checkout responders", member_ids: [owner.user.id, primary.user.id, backup.user.id], skills: ["service operations", "checkout"], contact: "response workspace" }, { id: "dependency", name: "Payment owner", member_ids: [dependency.user.id], skills: ["payment operations"], contact: "response workspace" }], rules: [{ id: "checkout-high", resource_ids: ["checkout"], signal_class: "reliability", severity: "high", accountable_team_id: "service", required_skills: ["service operations"], acknowledge_seconds: 1, resolve_seconds: 3600, expected_actions: ["acknowledge", "inspect exact release", "coordinate dependency", "request authorized mitigation"], escalations: [{ after_seconds: 1, team_id: "service", audience_ids: [owner.user.id], expected_action: "activate declared backup" }], communication_audience_ids: [owner.user.id, dependency.user.id], incident_criteria: ["recurrence with material user impact"], authority }, { id: "checkout-critical", resource_ids: ["checkout"], signal_class: "reliability", severity: "critical", accountable_team_id: "service", required_skills: ["service operations"], acknowledge_seconds: 60, resolve_seconds: 3600, expected_actions: ["declare incident", "mitigate through protected environment"], escalations: [{ after_seconds: 60, team_id: "service", audience_ids: [owner.user.id], expected_action: "incident coordination" }], communication_audience_ids: [owner.user.id, dependency.user.id], incident_criteria: ["more than 100 affected users"], authority }, { id: "dependency-noise", resource_ids: ["payments"], signal_class: "dependency", severity: "medium", accountable_team_id: "dependency", required_skills: ["payment operations"], acknowledge_seconds: 60, resolve_seconds: 600, expected_actions: ["validate dependency evidence"], escalations: [{ after_seconds: 60, team_id: "dependency", audience_ids: [owner.user.id], expected_action: "correct noisy signal" }], communication_audience_ids: [owner.user.id], incident_criteria: ["confirmed checkout impact"], authority }], exceptions: [], change_reason: "Released service coverage" };
    const policy = await json(ownerPage, "post", `/repositories/${repository.id}/response-policies`, owner.headers, { request_id: `policy-${suffix}`, revision: policyRevision });
    const availability = [{ weekdays: ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"], start_local: "00:00", end_local: "23:59" }];
    let rotation = await json(ownerPage, "post", `/repositories/${repository.id}/response-rotations`, owner.headers, { request_id: `rotation-${suffix}`, revision: { name: "Checkout primary and backup", policy_id: policy.id, team_id: "service", time_zone: "UTC", handoff_window_minutes: 30, responders: [{ user_id: primary.user.id, qualifications: ["service operations"], availability, max_shifts_per_week: 7 }, { user_id: backup.user.id, qualifications: ["service operations"], availability, max_shifts_per_week: 7 }], absence_rules: [{ kind: "unplanned", notice_hours: 0, action: "offer exact context to backup" }], shifts: [{ id: "active", starts_at: iso(-3600_000), ends_at: iso(3600_000), primary_user_id: primary.user.id, backup_user_ids: [backup.user.id], required_qualifications: ["service operations"] }], change_reason: "Continuous released-service duty" } });
    rotation = await json(primaryPage, "post", `/repositories/${repository.id}/response-rotations/${rotation.id}/duty-events`, primary.headers, { request_id: `duty-${suffix}`, expected_version: rotation.event_version, kind: "acknowledge", shift_id: "active", context: [] });
    const evidence = [{ kind: "release_health", resource_id: release.id, revision, digest: "a".repeat(64), summary: "Checkout error rate crossed the released threshold", accessible_to: [owner.user.id, primary.user.id, backup.user.id, dependency.user.id], available: true }];
    const signal = (severity: string, request: string, correlation: string, affected = 42, extra: object = {}) => json(ownerPage, "post", `/repositories/${repository.id}/response-alerts`, owner.headers, { request_id: request, signal: { signal_class: "reliability", severity, resource_ids: ["checkout"], affected_user_count: affected, affected_user_groups: ["checkout teams"], summary: severity === "critical" ? "Checkout failure recurred severely" : "Checkout error rate is rising", uncertainty: "sampled release health and dependency saturation", occurred_at: iso(), source_revision: revision, correlation_key: correlation, evidence, ...extra } });
    let alert = await signal("high", `alert-${suffix}`, "checkout-window"); expect(alert.routing).toContainEqual(expect.objectContaining({ recipient_id: primary.user.id, status: "delivered" }));
    const duplicate = await signal("high", `duplicate-${suffix}`, "checkout-window"); expect(duplicate.id).toBe(alert.id); expect(duplicate.event_count).toBe(2);
    await expect.poll(async () => (await json(ownerPage, "get", `/repositories/${repository.id}/response-outcomes`, owner.headers)).missed_acknowledgements, { timeout: 10_000 }).toBeGreaterThan(0);
    alert = await json(primaryPage, "post", `/repositories/${repository.id}/response-alerts/${alert.id}/events`, primary.headers, { request_id: `ack-${suffix}`, kind: "acknowledge", reason: "Primary owns the exact released-service response" });
    const context = [{ kind: "release", resource_id: release.id, revision, summary: "Exact unhealthy checkout release", window_from: iso(-300_000), window_to: iso() }, { kind: "dependency", resource_id: "payments", revision: "health-window-7", summary: "Payment saturation overlaps checkout errors", window_from: iso(-300_000), window_to: iso() }, { kind: "runbook", resource_id: "RUNBOOK.md", revision, summary: "Released response instructions", window_from: iso(-300_000), window_to: iso() }];
    alert = await json(primaryPage, "post", `/repositories/${repository.id}/response-alerts/${alert.id}/workspace`, primary.headers, { request_id: `classify-${suffix}`, kind: "classify", message: "Release-bound failure is actionable", classification: "actionable" });
    alert = await json(primaryPage, "post", `/repositories/${repository.id}/response-alerts/${alert.id}/workspace`, primary.headers, { request_id: `invite-${suffix}`, kind: "invite", message: "Payment owner, inspect the frozen dependency window", target_user_id: dependency.user.id });
    alert = await json(primaryPage, "post", `/repositories/${repository.id}/response-alerts/${alert.id}/workspace`, primary.headers, { request_id: `invite-operator-${suffix}`, kind: "invite", message: "Service owner, retain the separately authorized mitigation", target_user_id: owner.user.id });
    await rejected(await primaryPage.request.post(`/api/repositories/${repository.id}/response-alerts/${alert.id}/workspace`, { headers: primary.headers, data: { request_id: `budget-${suffix}`, kind: "delegate_agent", message: "Investigate without production access", agent_id: "response-reader", permitted_tools: ["read_context", "inspect_dependencies", "read_runbook"], budget: 101, context } }), 400, "invalid_response_alert");
    alert = await json(primaryPage, "post", `/repositories/${repository.id}/response-alerts/${alert.id}/workspace`, primary.headers, { request_id: `agent-${suffix}`, kind: "delegate_agent", message: "Compare the release, runbook, and dependency evidence; report only", agent_id: "response-reader", permitted_tools: ["read_context", "inspect_dependencies", "read_runbook"], budget: 20, context });
    alert = await json(dependencyPage, "post", `/repositories/${repository.id}/response-alerts/${alert.id}/workspace`, dependency.headers, { request_id: `dependency-${suffix}`, kind: "observe", message: "Payment retry noise amplified checkout latency; no credential or mutation is needed", context: [context[1]] });
    alert = await json(ownerPage, "post", `/repositories/${repository.id}/response-alerts/${alert.id}/workspace`, owner.headers, { request_id: `mitigation-${suffix}`, kind: "action", message: "Repository owner used existing environment authority to reduce retries; the response workspace granted none", context: [context[0]] });
    rotation = await json(primaryPage, "post", `/repositories/${repository.id}/response-rotations/${rotation.id}/duty-events`, primary.headers, { request_id: `handoff-${suffix}`, expected_version: rotation.event_version, kind: "delegate", shift_id: "active", to_user_id: backup.user.id, reason: "Shift ends while verification continues", context: [{ kind: "active_alert", resource_id: alert.id, revision: String(alert.workspace.version), summary: "Acknowledged; dependency owner found retry noise; authorized mitigation applied; verify recovery" }] });
    rotation = await json(backupPage, "post", `/repositories/${repository.id}/response-rotations/${rotation.id}/duty-events/${rotation.events.at(-1).id}/accept`, backup.headers, { expected_version: rotation.event_version });
    expect(rotation.effective_owner_by_shift.active).toBe(backup.user.id); expect(rotation.events.at(-1).context[0].resource_id).toBe(alert.id);
    await rejected(await primaryPage.request.post(`/api/repositories/${repository.id}/response-alerts/${alert.id}/events`, { headers: primary.headers, data: { request_id: `former-owner-${suffix}`, kind: "resolve", reason: "Former primary no longer owns the accepted handoff" } }), 403, "response_alert_forbidden");
    alert = await json(backupPage, "post", `/repositories/${repository.id}/response-alerts/${alert.id}/events`, backup.headers, { request_id: `resolve-${suffix}`, kind: "resolve", reason: "Accepted backup verified that mitigation restored checkout health" });
    const recurrence = await signal("critical", `recurrence-${suffix}`, "checkout-recurrence", 240); expect(recurrence.id).not.toBe(alert.id);
    const incidentAlert = await json(backupPage, "post", `/repositories/${repository.id}/response-alerts/${recurrence.id}/workspace`, backup.headers, { request_id: `incident-${suffix}`, kind: "promote_incident", message: "Severe recurrence meets the frozen incident criterion" });
    expect(incidentAlert.workspace.incident_id).toBeTruthy();
    const incident = await json(ownerPage, "get", `/incidents/${incidentAlert.workspace.incident_id}`, owner.headers); expect(incident.summary).toContain(recurrence.id);
    const outcome = await json(ownerPage, "post", `/repositories/${repository.id}/response-alerts/${recurrence.id}/outcomes`, owner.headers, { request_id: "b".repeat(32), classification: "incident_candidate", user_outcome: "Checkout recovered after bounded retry mitigation", user_outcome_consent: true, agent_cost: 20, correction_kind: "signal", rationale: "The severe recurrence exposed dependency noise and a missing retry/runbook instruction", work: { kind: "documentation", title: "Improve checkout signal and runbook", outcome: "Tune dependency noise and document bounded retry mitigation", assignee_type: "human", assignee_id: backup.user.id, due_at: iso(7 * 86400_000) } });
    const review = outcome.outcome_reviews.at(-1); expect(review.proposal_id).toBeTruthy(); expect(review.task_id).toBeTruthy();
    await git("", copy, "switch", "-c", "improve-response"); await writeFile(join(copy, "RUNBOOK.md"), "# Checkout response\n\nConfirm the revision-exact error rate, separate payment retry noise, ask the dependency owner, and request mitigation from an authorized environment operator.\n"); await git("", copy, "add", "RUNBOOK.md"); await git("", copy, "commit", "-m", "Improve checkout response signal and runbook"); await git(credential.token, copy, "push", "origin", "improve-response");
    const pull = await json(backupPage, "post", `/repositories/${repository.id}/proposals/${review.proposal_id}/tasks/${review.task_id}/contributions`, backup.headers, { title: "Improve checkout signal and runbook", body: `Reviewed follow-up from incident ${incident.id}`, source_branch: "improve-response", target_branch: "main" });
    await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/reviews`, owner.headers, { decision: "approved" }); await json(ownerPage, "post", `/repositories/${repository.id}/pulls/${pull.id}/merge`, owner.headers, {});
    const noisy = await json(ownerPage, "post", `/repositories/${repository.id}/response-alerts`, owner.headers, { request_id: `noise-${suffix}`, signal: { signal_class: "dependency", severity: "medium", resource_ids: ["payments"], affected_user_count: 0, affected_user_groups: [], summary: "Payment health sample was noisy", uncertainty: "single incomplete sample", occurred_at: iso(), source_revision: revision, correlation_key: "payment-noise", evidence } });
    await json(dependencyPage, "post", `/repositories/${repository.id}/response-alerts/${noisy.id}/workspace`, dependency.headers, { request_id: `false-positive-${suffix}`, kind: "classify", message: "Dependency owner confirms sampling noise", classification: "false_positive" });
    await json(ownerPage, "delete", `/repositories/${repository.id}/collaborators/${dependency.user.id}`, owner.headers);
    const failed = await json(ownerPage, "post", `/repositories/${repository.id}/response-alerts`, owner.headers, { request_id: `revoked-${suffix}`, signal: { signal_class: "dependency", severity: "medium", resource_ids: ["payments"], affected_user_count: 0, affected_user_groups: [], summary: "Revoked dependency owner cannot receive delivery", uncertainty: "delivery path unavailable", occurred_at: iso(), source_revision: revision, correlation_key: "revoked-owner", evidence } });
    expect(failed.routing).toEqual([]); expect(failed.diagnostics).toContain("delivery_failed");
    const missedBefore = (await json(ownerPage, "get", `/repositories/${repository.id}/response-outcomes`, owner.headers)).missed_acknowledgements;
    const missed = await signal("high", `missed-${suffix}`, "absent-primary"); expect(missed.routing).toContainEqual(expect.objectContaining({ recipient_id: backup.user.id }));
    await expect.poll(async () => { const target = await json(ownerPage, "get", `/repositories/${repository.id}/response-alerts/${missed.id}`, owner.headers); return Date.now() > Date.parse(target.acknowledge_by) && !target.events.some((event: { kind: string }) => event.kind === "acknowledge"); }, { timeout: 10_000 }).toBe(true);
    expect((await json(ownerPage, "get", `/repositories/${repository.id}/response-outcomes`, owner.headers)).missed_acknowledgements).toBe(missedBefore + 1);
    await ownerPage.goto(`/repositories/${repository.id}/response-policies`); await expect(ownerPage.getByRole("heading", { name: "Response coverage" })).toBeVisible(); await expect(ownerPage.getByText("Checkout response coverage", { exact: true })).toBeVisible(); await expect(ownerPage.getByRole("heading", { name: "Response outcomes" })).toBeVisible();
    const report = await json(ownerPage, "get", `/repositories/${repository.id}/response-outcomes`, owner.headers); expect(report).toMatchObject({ deduplicated_events: 1, incidents: 1, agent_cost: 20 }); expect(report.missed_acknowledgements).toBeGreaterThan(0);
  } finally { await rm(copy, { recursive: true, force: true }); }
});
