import { expect, test, type Page } from "@playwright/test";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

/* eslint-disable @typescript-eslint/no-explicit-any -- one connected global-delivery evidence trail */
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
    user: await (await page.request.get("/api/user", { headers })).json(),
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

test("a product change reaches readers through attributable locale delivery and repair", async ({
  browser,
}) => {
  test.setTimeout(300_000);
  const docker = await run("docker", ["info"]).then(
    () => true,
    () => false,
  );
  test.skip(!docker, "checks and previews require Docker");
  await run("docker", ["image", "inspect", "alpine:3.22"]).catch(() =>
    run("docker", ["pull", "alpine:3.22"]),
  );
  const copy = await mkdtemp(join(tmpdir(), "vivarium-localization-"));
  try {
    const suffix = Date.now().toString(36),
      ownerPage = await (await browser.newContext()).newPage(),
      translatorPage = await (await browser.newContext()).newPage(),
      frenchPage = await (await browser.newContext()).newPage(),
      arabicPage = await (await browser.newContext()).newPage();
    const owner = await account(
        ownerPage,
        "Product Developer",
        `locale-owner-${suffix}`,
      ),
      translator = await account(
        translatorPage,
        "Human Translator",
        `translator-${suffix}`,
      ),
      french = await account(
        frenchPage,
        "French Canadian Reviewer",
        `fr-reviewer-${suffix}`,
      ),
      arabic = await account(
        arabicPage,
        "Arabic Reviewer",
        `ar-reviewer-${suffix}`,
      );
    const organization = await json(
      ownerPage,
      "post",
      "/organizations",
      owner.headers,
      { name: `Global Product ${suffix}`, slug: `global-${suffix}` },
    );
    const repository = await json(
      ownerPage,
      "post",
      `/organizations/${organization.id}/repositories`,
      owner.headers,
      { name: `welcome-${suffix}` },
    );
    for (const person of [translator, french, arabic])
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
        name: "Grounded Linguist",
        slug: `linguist-${suffix}`,
        capabilities: [
          "suggest translations from bounded terminology and source context",
        ],
        operator_ids: [owner.user.id],
        team_ids: [],
      },
    );
    const agent = group.agents.at(-1);
    group = await json(
      ownerPage,
      "post",
      `/organizations/${organization.id}/access-requests`,
      owner.headers,
      {
        principal_type: "agent",
        principal_id: agent.id,
        role: "contributor",
        resources: [{ kind: "repository", id: repository.id }],
        exceptions: [],
        reason: "Suggest only plan-grounded locale wording",
        expires_at: new Date(Date.now() + 3_600_000).toISOString(),
      },
    );
    const request = group.access_requests.at(-1);
    group = await json(
      ownerPage,
      "post",
      `/organizations/${organization.id}/access-requests/${request.id}/decision`,
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
        agent_id: agent.id,
        repository_id: repository.id,
        expires_in: 3600,
        purpose: "api_read",
      },
    );
    const agentHeaders = { Authorization: `Bearer ${issued.token}` };
    const credential = await json(
      ownerPage,
      "post",
      "/auth/credentials",
      owner.headers,
      {
        kind: "git",
        name: "localization journey",
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
    await git(copy, "config", "user.name", "Product Developer");
    await git(copy, "config", "user.email", "developer@example.test");
    await mkdir(join(copy, ".vivarium"));
    await writeFile(
      join(copy, ".vivarium", "preview.json"),
      JSON.stringify({
        version: 1,
        image: "alpine:3.22",
        build: "mkdir -p dist && cp welcome.txt dist/index.html",
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
      }),
    );
    await writeFile(
      join(copy, ".vivarium", "checks.json"),
      JSON.stringify({
        version: 1,
        checks: [
          {
            name: "locale contract",
            image: "alpine:3.22",
            command: "grep -q 'Start free trial' welcome.txt",
          },
        ],
      }),
    );
    await writeFile(join(copy, "welcome.txt"), "Start trial\n");
    await git(copy, "add", ".");
    await git(copy, "commit", "-m", "Establish welcome experience");
    await git(copy, "push", "origin", "main");
    const base = await git(copy, "rev-parse", "HEAD");
    const plan = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/locale-plans`,
      owner.headers,
      {
        revision: {
          title: "Global welcome experience",
          summary:
            "The welcome action reads naturally in French Canada and Arabic Egypt.",
          subject: {
            kind: "product",
            resource_id: "web",
            name: "Welcome experience",
          },
          locales: [
            {
              id: "fr-CA",
              language: "French",
              regions: ["CA"],
              fallback_locale: "",
              owner_ids: [translator.user.id],
              reviewer_ids: [translator.user.id],
            },
            {
              id: "ar-EG",
              language: "Arabic",
              regions: ["EG"],
              fallback_locale: "fr-CA",
              owner_ids: [translator.user.id],
              reviewer_ids: [translator.user.id],
            },
          ],
          terminology: [
            {
              id: "trial-fr",
              source: "free trial",
              locale: "fr-CA",
              preferred: "essai gratuit",
              avoid: ["test gratuit"],
              context: "primary action",
            },
            {
              id: "trial-ar",
              source: "free trial",
              locale: "ar-EG",
              preferred: "تجربة مجانية",
              avoid: [],
              context: "primary action",
            },
          ],
          formatting_requirements: [
            {
              locale: "fr-CA",
              date: "yyyy-MM-dd",
              time: "24h",
              number: "decimal comma",
              currency: "CAD",
              units: "metric",
              direction: "ltr",
            },
            {
              locale: "ar-EG",
              date: "yyyy-MM-dd",
              time: "24h",
              number: "Arabic digits",
              currency: "EGP",
              units: "metric",
              direction: "rtl",
            },
          ],
          covered_journeys: [
            {
              id: "welcome",
              name: "Start a free trial",
              locale_ids: ["fr-CA", "ar-EG"],
              owner_ids: [translator.user.id],
              required: true,
            },
          ],
          resources: [
            {
              id: "welcome-copy",
              kind: "messages",
              path: "welcome.txt",
              format: "text",
              source_revision: base,
              locale_ids: ["fr-CA", "ar-EG"],
            },
          ],
          release_thresholds: [
            {
              locale: "fr-CA",
              minimum_percent: 100,
              required_journey_ids: ["welcome"],
              require_owner_review: true,
              require_regional_review: true,
            },
            {
              locale: "ar-EG",
              minimum_percent: 100,
              required_journey_ids: ["welcome"],
              require_owner_review: true,
              require_regional_review: true,
            },
          ],
          rationale:
            "Keep linguistic ownership and global release evidence together.",
        },
      },
    );
    await git(copy, "switch", "-c", "free-trial");
    await writeFile(join(copy, "welcome.txt"), "Start trial today\n");
    await git(copy, "add", "welcome.txt");
    await git(copy, "commit", "-m", "Open localized trial experience");
    await git(copy, "push", "origin", "free-trial");
    const pull = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/pulls`,
      owner.headers,
      {
        title: "Launch the global free trial action",
        body: "Creates exact translation work for two regional experiences.",
        source_branch: "free-trial",
        target_branch: "main",
      },
    );
    const pullBase = `/repositories/${repository.id}/pulls/${pull.id}`;
    let revision = (await json(ownerPage, "get", pullBase, owner.headers))
      .source_commit_id;
    const extraction = (message: string) => ({
      source_revision: revision,
      map: {
        id: "welcome-map",
        version: 1,
        name: "Welcome messages",
        include: ["welcome.txt"],
        formats: ["text"],
      },
      locales: ["fr-CA", "ar-EG"],
      units: [
        {
          key: "welcome.start",
          message,
          context: "Primary welcome action",
          screenshot: "artifact://welcome-action",
          variables: [],
          plural_rule: "",
          source_locations: [
            { path: "welcome.txt", line: 1, component: "WelcomeAction" },
          ],
        },
      ],
    });
    let review = await json(
      ownerPage,
      "post",
      `${pullBase}/localization/extractions`,
      owner.headers,
      extraction("Start trial today"),
    );
    let unit = review.extractions.at(-1).units[0];
    review = await json(
      translatorPage,
      "post",
      `${pullBase}/localization/translations`,
      translator.headers,
      {
        source_revision: revision,
        unit_id: unit.id,
        locale: "fr-CA",
        text: "Commencer l’essai aujourd’hui",
        note: "Initial human adaptation",
      },
    );
    await writeFile(join(copy, "welcome.txt"), "Start free trial\n");
    await git(copy, "add", "welcome.txt");
    await git(copy, "commit", "-m", "Clarify the source offer");
    await git(copy, "push", "origin", "free-trial");
    const synchronized = await json(
      ownerPage,
      "post",
      `${pullBase}/synchronize`,
      owner.headers,
      {},
    );
    revision = synchronized.source_commit_id;
    const stale = await json(
      ownerPage,
      "get",
      `${pullBase}/localization`,
      owner.headers,
    );
    expect(stale.translations[0]).toMatchObject({
      proposed_by: translator.user.id,
      status: "stale",
    });
    review = await json(
      ownerPage,
      "post",
      `${pullBase}/localization/extractions`,
      owner.headers,
      extraction("Start free trial"),
    );
    unit = review.extractions.at(-1).units[0];
    review = await json(
      translatorPage,
      "post",
      `${pullBase}/localization/translations`,
      translator.headers,
      {
        source_revision: revision,
        unit_id: unit.id,
        locale: "fr-CA",
        text: "Commencer l’essai gratuit",
        note: "Uses the current preferred term",
      },
    );
    review = await json(
      translatorPage,
      "post",
      `${pullBase}/localization/workspace`,
      translator.headers,
      {
        source_revision: revision,
        expected_version: review.workspace_version,
        mutation: "request_suggestion",
        payload: {
          unit_id: unit.id,
          locale: "ar-EG",
          agent_id: agent.id,
          product_context:
            "Primary action for a free trial on the welcome route",
          locale_plan_id: plan.id,
          locale_plan_version: 1,
          protected: false,
          embargoed: false,
        },
      },
    );
    const suggestionRequest = review.suggestion_requests.at(-1);
    review = await json(
      ownerPage,
      "post",
      `${pullBase}/localization/workspace`,
      agentHeaders,
      {
        source_revision: revision,
        expected_version: review.workspace_version,
        mutation: "suggest",
        payload: {
          unit_id: unit.id,
          locale: "ar-EG",
          request_id: suggestionRequest.id,
          text: "ابدأ تجربة مجانية",
          rationale:
            "Uses the declared free-trial term in an imperative welcome action; regional directionality review remains necessary.",
          uncertainty: "medium",
          evidence: [
            { kind: "locale_plan", reference: `${plan.id}:1` },
            { kind: "terminology", reference: "trial-ar" },
            { kind: "source_context", reference: "welcome.start@current" },
          ],
        },
      },
    );
    const suggestion = review.suggestions.at(-1);
    review = await json(
      translatorPage,
      "post",
      `${pullBase}/localization/workspace`,
      translator.headers,
      {
        source_revision: revision,
        expected_version: review.workspace_version,
        mutation: "decide",
        payload: {
          unit_id: unit.id,
          locale: "ar-EG",
          suggestion_id: suggestion.id,
          kind: "approve",
          reason:
            "Terminology is current; regional layout review remains required.",
        },
      },
    );
    review = await json(
      translatorPage,
      "post",
      `${pullBase}/localization/translations`,
      translator.headers,
      {
        source_revision: revision,
        unit_id: unit.id,
        locale: "ar-EG",
        text: suggestion.text,
        note: `Adapted from grounded suggestion ${suggestion.id}`,
      },
    );
    const policy = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/localization-delivery-policies`,
      owner.headers,
      {
        branch: "main",
        locale_plan_id: plan.id,
        locale_plan_version: 1,
        locales: ["fr-CA", "ar-EG"],
        audiences: [],
        risk_classes: [],
        required_checks: ["locale contract"],
        minimum_reviews: 1,
      },
    );
    let readiness = await json(
      ownerPage,
      "get",
      `${pullBase}/localization-readiness`,
      owner.headers,
    );
    expect(readiness.ready).toBe(false);
    expect(readiness.requirements).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          locale: "fr-CA",
          kind: "review",
          status: "missing",
        }),
        expect.objectContaining({
          locale: "ar-EG",
          kind: "review",
          status: "missing",
        }),
      ]),
    );
    const preview = await json(
      ownerPage,
      "post",
      `${pullBase}/previews`,
      owner.headers,
    );
    await eventually(
      () => json(ownerPage, "get", `${pullBase}/previews`, owner.headers),
      (x: any) =>
        x.previews.some(
          (p: any) => p.id === preview.id && p.state === "succeeded",
        ),
      "localized preview succeeds",
    );
    for (const regional of [french, arabic])
      await json(
        ownerPage,
        "post",
        `${pullBase}/previews/${preview.id}/invitations`,
        owner.headers,
        {
          source_kind: "user",
          user_id: regional.user.id,
          role: "feedback",
          expires_at: new Date(Date.now() + 3_600_000).toISOString(),
        },
      );
    const candidates: Record<string, any> = {};
    for (const locale of ["fr-CA", "ar-EG"]) {
      review = await json(
        ownerPage,
        "post",
        `${pullBase}/localization/verification`,
        owner.headers,
        {
          source_revision: revision,
          expected_version: review.workspace_version,
          mutation: "publish_candidate",
          payload: {
            locale,
            preview_id: preview.id,
            locale_plan_id: plan.id,
            locale_plan_version: 1,
            routes: [
              {
                journey_id: "welcome",
                route: `/${locale}/welcome`,
                interface_hash: (locale === "fr-CA" ? "a" : "b").repeat(64),
              },
            ],
          },
        },
      );
      candidates[locale] = review.verification_candidates.at(-1);
      const results = [
        "variables",
        "pluralization",
        "formatting",
        "terminology",
        "links",
        "layout_expansion",
        "bidirectional_text",
        "fallback_behavior",
        "localized_journey",
      ].map((kind) => ({
        kind,
        route: `/${locale}/welcome`,
        unit_ids: [unit.id],
        status: "passed",
        summary: `${kind} verified on exact preview`,
        artifact: `preview://${preview.id}/${locale}/${kind}`,
      }));
      review = await json(
        ownerPage,
        "post",
        `${pullBase}/localization/verification`,
        owner.headers,
        {
          source_revision: revision,
          expected_version: review.workspace_version,
          mutation: "record_checks",
          payload: { candidate_id: candidates[locale].id, results },
        },
      );
    }
    review = await json(
      arabicPage,
      "post",
      `${pullBase}/localization/previews/${preview.id}/review`,
      arabic.headers,
      {
        source_revision: revision,
        expected_version: review.workspace_version,
        mutation: "finding",
        payload: {
          candidate_id: candidates["ar-EG"].id,
          route: "/ar-EG/welcome",
          unit_ids: [unit.id],
          category: "bidirectional_text",
          severity: "blocking",
          body: "The action is translated, but its icon and punctuation reverse in the RTL container.",
        },
      },
    );
    review = await json(
      frenchPage,
      "post",
      `${pullBase}/localization/previews/${preview.id}/review`,
      french.headers,
      {
        source_revision: revision,
        expected_version: review.workspace_version,
        mutation: "review",
        payload: {
          candidate_id: candidates["fr-CA"].id,
          route: "/fr-CA/welcome",
          unit_ids: [unit.id],
          kind: "approve",
          reason:
            "The current exact preview reads naturally for French Canada.",
        },
      },
    );
    await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/localization-dispositions`,
      owner.headers,
      {
        policy_id: policy.id,
        revision,
        locale: "ar-EG",
        state: "withdrawn",
        reason:
          "Contain the confirmed RTL layout failure while French Canada proceeds.",
      },
    );
    readiness = await eventually(
      () =>
        json(
          ownerPage,
          "get",
          `${pullBase}/localization-readiness`,
          owner.headers,
        ),
      (x: any) => x.ready,
      "locale check and regional review become current",
    );
    expect(readiness.locales).toMatchObject({
      "fr-CA": "required",
      "ar-EG": "withdrawn",
    });
    await json(
      translatorPage,
      "post",
      `${pullBase}/reviews`,
      translator.headers,
      { decision: "approved" },
    );
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
        version: "v1.0.0",
        notes:
          "French Canada staged; Arabic explicitly withdrawn after RTL review.",
        commit_id: merged.merge_commit_id,
      },
    );
    const frenchPublication = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/localized-publications`,
      owner.headers,
      {
        kind: "application",
        resource_id: "welcome",
        release_id: release.id,
        version: release.version,
        revision: release.commit_id,
        locale: "fr-CA",
        locale_plan_id: plan.id,
        locale_plan_version: 1,
        source_locale: "en",
        fallback_state: "complete",
        url: "https://example.test/fr-CA/welcome",
        status: "published",
      },
    );
    await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/localized-publications`,
      owner.headers,
      {
        kind: "application",
        resource_id: "welcome",
        release_id: release.id,
        version: release.version,
        revision: release.commit_id,
        locale: "ar-EG",
        locale_plan_id: plan.id,
        locale_plan_version: 1,
        source_locale: "en",
        fallback_locale: "fr-CA",
        fallback_state: "fallback",
        url: "https://example.test/ar-EG/welcome",
        status: "withdrawn",
      },
    );
    let finding = await json(
      frenchPage,
      "post",
      `/repositories/${repository.id}/localized-publications/${frenchPublication.id}/findings`,
      french.headers,
      {
        locale: "fr-CA",
        category: "cultural_mismatch",
        route: "/fr-CA/welcome",
        unit_key: "welcome.start",
        expected:
          "Use the familiar action wording validated by current regional readers.",
        observed:
          "The released action is grammatically correct but unusually formal for this audience.",
        evidence_url: "https://example.test/evidence/fr-ca-reader-note",
      },
    );
    await git(copy, "switch", "main");
    await git(copy, "pull", "origin", "main");
    await git(copy, "switch", "-c", "repair-fr-ca-tone");
    await writeFile(
      join(copy, "welcome.txt"),
      "Start free trial — familiar French Canadian tone\n",
    );
    await git(copy, "add", "welcome.txt");
    await git(copy, "commit", "-m", "Repair French Canadian welcome tone");
    await git(copy, "push", "origin", "repair-fr-ca-tone");
    const repairPull = await json(
      translatorPage,
      "post",
      `/repositories/${repository.id}/pulls`,
      translator.headers,
      {
        title: "Correct the French Canadian welcome tone",
        body: "Connected reader finding repair with retained regional rationale.",
        source_branch: "repair-fr-ca-tone",
        target_branch: "main",
      },
    );
    const repairRevision = repairPull.source_commit_id;
    finding = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/localization-findings/${finding.id}/decision`,
      owner.headers,
      {
        status: "validated",
        reason:
          "Regional reader feedback reproduces on the exact v1.0.0 publication.",
        repair: {
          owner_type: "human",
          owner_id: translator.user.id,
          work_url: `/repositories/${repository.id}/pulls/${repairPull.id}`,
          acceptance_criteria:
            "Use familiar French Canadian action wording and retain Arabic withdrawal until RTL is fixed.",
        },
      },
    );
    for (const locale of ["fr-CA", "ar-EG"])
      await json(
        ownerPage,
        "post",
        `/repositories/${repository.id}/localization-dispositions`,
        owner.headers,
        {
          policy_id: policy.id,
          revision: repairRevision,
          locale,
          state: "deferred",
          reason:
            locale === "fr-CA"
              ? "Publish the validated focused correction after ordinary review."
              : "Keep the independently withdrawn RTL locale contained.",
        },
      );
    await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/pulls/${repairPull.id}/reviews`,
      owner.headers,
      { decision: "approved" },
    );
    const repaired = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/pulls/${repairPull.id}/merge`,
      owner.headers,
      {},
    );
    const correction = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/releases`,
      owner.headers,
      {
        version: "v1.0.1",
        notes: "Corrected French Canadian tone; Arabic remains withdrawn.",
        commit_id: repaired.merge_commit_id,
      },
    );
    const corrected = await json(
      ownerPage,
      "post",
      `/repositories/${repository.id}/localized-publications`,
      owner.headers,
      {
        kind: "application",
        resource_id: "welcome",
        release_id: correction.id,
        version: correction.version,
        revision: correction.commit_id,
        locale: "fr-CA",
        locale_plan_id: plan.id,
        locale_plan_version: 1,
        source_locale: "en",
        fallback_state: "complete",
        url: "https://example.test/fr-CA/welcome?v=1.0.1",
        status: "published",
      },
    );
    await ownerPage.goto(`/repositories/${repository.id}/locales`);
    await expect(
      ownerPage.getByRole("heading", { name: "Locale coverage" }),
    ).toBeVisible();
    await expect(
      ownerPage.getByRole("heading", { name: "Global welcome experience" }),
    ).toBeVisible();
    await expect(
      ownerPage.getByText(/fr-CA · cultural mismatch/),
    ).toBeVisible();
    const trail = await json(
      ownerPage,
      "get",
      `/repositories/${repository.id}/localization-delivery`,
      owner.headers,
    );
    expect(trail.findings[0]).toMatchObject({
      id: finding.id,
      reporter_id: french.user.id,
      status: "validated",
      repair: {
        owner_id: translator.user.id,
        work_url: `/repositories/${repository.id}/pulls/${repairPull.id}`,
      },
    });
    expect(trail.publications).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: corrected.id,
          version: "v1.0.1",
          locale: "fr-CA",
          status: "published",
        }),
        expect.objectContaining({
          version: "v1.0.0",
          locale: "ar-EG",
          status: "withdrawn",
        }),
      ]),
    );
    expect(review.suggestions[0]).toMatchObject({
      agent_id: agent.id,
      locale_plan_id: plan.id,
      locale_plan_version: 1,
    });
    expect(review.locale_findings[0]).toMatchObject({
      category: "bidirectional_text",
      author_id: arabic.user.id,
    });
  } finally {
    await rm(copy, { recursive: true, force: true });
  }
});
