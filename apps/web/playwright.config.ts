import { defineConfig } from "@playwright/test";
import { execFileSync } from "node:child_process";

function systemChromium() {
  if (process.env.CHROMIUM_PATH) return process.env.CHROMIUM_PATH;
  try {
    return execFileSync("which", ["chromium"], { encoding: "utf8" }).trim();
  } catch {
    return undefined;
  }
}

const chromiumPath = systemChromium();

export default defineConfig({
  testDir: "./tests",
  timeout: 120_000,
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: "http://localhost:3000",
    headless: true,
    launchOptions: chromiumPath ? { executablePath: chromiumPath } : undefined,
  },
  webServer: [
    {
      command:
        "bash -lc 'root=$(mktemp -d); GIT_STORAGE_ROOT=$root/git USER_STORAGE_ROOT=$root/users AUTH_STORAGE_ROOT=$root/auth REPOSITORY_STORAGE_ROOT=$root/repositories PROPOSAL_STORAGE_ROOT=$root/proposals PULL_REQUEST_STORAGE_ROOT=$root/pulls ACTIVITY_STORAGE_ROOT=$root/activity CHANGE_SESSION_STORAGE_ROOT=$root/sessions CHECK_RUN_STORAGE_ROOT=$root/checks PREVIEW_STORAGE_ROOT=$root/previews PREVIEW_ACCEPTANCE_STORAGE_ROOT=$root/preview-acceptance RELEASE_STORAGE_ROOT=$root/releases DEPLOYMENT_STORAGE_ROOT=$root/deployments INCIDENT_STORAGE_ROOT=$root/incidents ISSUE_STORAGE_ROOT=$root/issues CONTRIBUTOR_PATHWAY_STORAGE_ROOT=$root/contributor-pathways CONTRIBUTION_OPPORTUNITY_STORAGE_ROOT=$root/contribution-opportunities SECURITY_ADVISORY_STORAGE_ROOT=$root/security-advisories RELATIONSHIP_STORAGE_ROOT=$root/relationships PACKAGE_STORAGE_ROOT=$root/packages ORGANIZATION_STORAGE_ROOT=$root/organizations WORKSPACE_STORAGE_ROOT=$root/workspaces EXPLANATION_STORAGE_ROOT=$root/explanations IMPACT_STORAGE_ROOT=$root/impact-assessments DECISION_STORAGE_ROOT=$root/decisions DELIVERY_TEAM_STORAGE_ROOT=$root/delivery-teams DOCUMENTATION_STORAGE_ROOT=$root/documentation EXTENSION_STORAGE_ROOT=$root/extensions CHARTER_STORAGE_ROOT=$root/charters GOVERNANCE_STORAGE_ROOT=$root/governance EXTENSION_DEVELOPMENT_ENDPOINTS=1 go run .'",
      cwd: "../api",
      url: "http://127.0.0.1:8080/health",
      timeout: 120_000,
    },
    {
      command: "bun run dev",
      url: "http://127.0.0.1:3000",
      timeout: 120_000,
    },
  ],
});
