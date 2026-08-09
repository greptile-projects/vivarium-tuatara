import { expect, test } from "@playwright/test";

const apiOrigin = "http://127.0.0.1:8080";

test("hands a delegated security investigation credential to its responder once", async ({ page, request, context }) => {
  const created = await request.post(`${apiOrigin}/users`, { data: { display_name: "Security Responder", handle: `security-${Date.now()}` } });
  expect(created.ok()).toBeTruthy();
  const account = await created.json() as { credential: { token: string } };
  const headers = { Authorization: `Bearer ${account.credential.token}` };

  const repositoryResponse = await request.post(`${apiOrigin}/repositories`, { headers, data: { name: "protected-parser" } });
  expect(repositoryResponse.ok()).toBeTruthy();
  const repository = await repositoryResponse.json() as { id: string };
  const advisoryResponse = await request.post(`${apiOrigin}/security-advisories`, { headers, data: {
    title: "Parser boundary bypass",
    description: "A crafted document may escape validation.",
    affected_repositories: [{ repository_id: repository.id, versions: ["1.x"] }],
    evidence: [{ label: "Minimal reproduction", description: "A bounded reproduction is available." }],
    contact: "security@example.test",
  } });
  expect(advisoryResponse.ok()).toBeTruthy();
  const advisory = await advisoryResponse.json() as { id: string };

  await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: "http://localhost:3000" });
  await page.addInitScript((token) => localStorage.setItem("vivarium.access-token", token), account.credential.token);
  await page.goto(`/security/${advisory.id}`);
  await page.getByLabel("Minimal reproduction").check();
  await page.getByPlaceholder("Investigation mandate").fill("Determine whether the selected evidence establishes exploitability.");
  await page.getByRole("button", { name: "Delegate read-only investigation" }).click();

  const handoff = page.getByRole("status");
  await expect(handoff).toContainText("Copy this agent credential now");
  await expect(handoff).toContainText("expires");
  const credential = page.getByLabel("Investigation agent credential");
  const token = await credential.inputValue();
  expect(token.length).toBeGreaterThan(20);
  await page.getByRole("button", { name: "Copy credential" }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(token);
  await page.getByRole("button", { name: "Clear" }).click();
  await expect(credential).toHaveCount(0);
  expect(await page.evaluate(() => ({ local: localStorage.getItem("investigationAccess"), session: sessionStorage.getItem("investigationAccess") }))).toEqual({ local: null, session: null });
});
