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
        'bash -lc \'root=$(mktemp -d); GIT_STORAGE_ROOT=$root/git USER_STORAGE_ROOT=$root/users AUTH_STORAGE_ROOT=$root/auth REPOSITORY_STORAGE_ROOT=$root/repositories PROPOSAL_STORAGE_ROOT=$root/proposals PULL_REQUEST_STORAGE_ROOT=$root/pulls ACTIVITY_STORAGE_ROOT=$root/activity CHANGE_SESSION_STORAGE_ROOT=$root/sessions CHECK_RUN_STORAGE_ROOT=$root/checks PREVIEW_STORAGE_ROOT=$root/previews PREVIEW_ACCEPTANCE_STORAGE_ROOT=$root/preview-acceptance RELEASE_STORAGE_ROOT=$root/releases DEPLOYMENT_STORAGE_ROOT=$root/deployments INCIDENT_STORAGE_ROOT=$root/incidents ISSUE_STORAGE_ROOT=$root/issues SUPPORT_THREAD_STORAGE_ROOT=$root/support-threads SUPPORT_VERIFICATION_STORAGE_ROOT=$root/support-verifications SUPPORT_SOLUTION_STORAGE_ROOT=$root/support-solutions KNOWLEDGE_ANSWER_STORAGE_ROOT=$root/knowledge-answers CONTRIBUTOR_PATHWAY_STORAGE_ROOT=$root/contributor-pathways CONTRIBUTION_OPPORTUNITY_STORAGE_ROOT=$root/contribution-opportunities SECURITY_ADVISORY_STORAGE_ROOT=$root/security-advisories RELATIONSHIP_STORAGE_ROOT=$root/relationships DURABLE_SCHEMA_STORAGE_ROOT=$root/durable-schemas API_CONTRACT_STORAGE_ROOT=$root/api-contracts PACKAGE_STORAGE_ROOT=$root/packages ORGANIZATION_STORAGE_ROOT=$root/organizations AGENT_EVALUATION_STORAGE_ROOT=$root/agent-evaluations WORKSPACE_STORAGE_ROOT=$root/workspaces EXPLANATION_STORAGE_ROOT=$root/explanations IMPACT_STORAGE_ROOT=$root/impact-assessments DECISION_STORAGE_ROOT=$root/decisions DELIVERY_TEAM_STORAGE_ROOT=$root/delivery-teams DOCUMENTATION_STORAGE_ROOT=$root/documentation EXTENSION_STORAGE_ROOT=$root/extensions FEDERATION_STORAGE_ROOT=$root/federation CHARTER_STORAGE_ROOT=$root/charters GOVERNANCE_STORAGE_ROOT=$root/governance PERFORMANCE_GOAL_STORAGE_ROOT=$root/performance-goals PERFORMANCE_EVIDENCE_STORAGE_ROOT=$root/performance-evidence PRODUCT_EXPERIMENT_STORAGE_ROOT=$root/product-experiments PROJECT_FUND_STORAGE_ROOT=$root/project-funds ACCESSIBILITY_COMMITMENT_STORAGE_ROOT=$root/accessibility-commitments ACCESSIBILITY_REPORT_STORAGE_ROOT=$root/accessibility-reports ACCESSIBILITY_ASSESSMENT_STORAGE_ROOT=$root/accessibility-assessments ACCESSIBILITY_DELIVERY_STORAGE_ROOT=$root/accessibility-delivery DATA_COMMITMENT_STORAGE_ROOT=$root/data-commitments DATA_FLOW_STORAGE_ROOT=$root/data-flows PRIVACY_REVIEW_STORAGE_ROOT=$root/privacy-reviews PRIVACY_CHECK_STORAGE_ROOT=$root/privacy-checks DATA_OBSERVATION_STORAGE_ROOT=$root/data-observations LOCALE_PLAN_STORAGE_ROOT=$root/locale-plans LOCALIZATION_STORAGE_ROOT=$root/localization SERVICE_OBJECTIVE_STORAGE_ROOT=$root/service-objectives RECOVERY_COMMITMENT_STORAGE_ROOT=$root/recovery-commitments PROTECTION_PLAN_STORAGE_ROOT=$root/protection-plans RECOVERY_EXERCISE_STORAGE_ROOT=$root/recovery-exercises RECOVERY_OPERATION_STORAGE_ROOT=$root/recovery-operations PROJECT_FUND_TRUSTED_SOURCES="{\\"community\\":\\"rPeXjD9B7ngjZSmWCcsA9HqTZOo9O0UAybBEgBsubzY=\\"}" EXTENSION_DEVELOPMENT_ENDPOINTS=1 go run .\'',
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
