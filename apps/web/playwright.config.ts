import { defineConfig } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

function systemChromium() {
  if (process.env.CHROMIUM_PATH) return process.env.CHROMIUM_PATH;
  try {
    return execFileSync("which", ["chromium"], { encoding: "utf8" }).trim();
  } catch {
    return undefined;
  }
}

const chromiumPath = systemChromium();
const capabilityStorageRoot = mkdtempSync(
  join(tmpdir(), "vivarium-playwright-capabilities-"),
);
process.once("exit", () => {
  rmSync(capabilityStorageRoot, { recursive: true, force: true });
});

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
        'bash -lc \'root=$(mktemp -d); GIT_STORAGE_ROOT=$root/git USER_STORAGE_ROOT=$root/users AUTH_STORAGE_ROOT=$root/auth REPOSITORY_STORAGE_ROOT=$root/repositories PROPOSAL_STORAGE_ROOT=$root/proposals PULL_REQUEST_STORAGE_ROOT=$root/pulls ACTIVITY_STORAGE_ROOT=$root/activity CHANGE_SESSION_STORAGE_ROOT=$root/sessions CHECK_RUN_STORAGE_ROOT=$root/checks PREVIEW_STORAGE_ROOT=$root/previews PREVIEW_ACCEPTANCE_STORAGE_ROOT=$root/preview-acceptance RELEASE_STORAGE_ROOT=$root/releases DEPLOYMENT_STORAGE_ROOT=$root/deployments INCIDENT_STORAGE_ROOT=$root/incidents ISSUE_STORAGE_ROOT=$root/issues DEBUG_WORKSPACE_STORAGE_ROOT=$root/debugging-workspaces SUPPORT_THREAD_STORAGE_ROOT=$root/support-threads SUPPORT_VERIFICATION_STORAGE_ROOT=$root/support-verifications SUPPORT_SOLUTION_STORAGE_ROOT=$root/support-solutions KNOWLEDGE_ANSWER_STORAGE_ROOT=$root/knowledge-answers CONTRIBUTOR_PATHWAY_STORAGE_ROOT=$root/contributor-pathways CONTRIBUTION_OPPORTUNITY_STORAGE_ROOT=$root/contribution-opportunities INCUBATOR_STORAGE_ROOT=$root/incubators FEEDBACK_STORAGE_ROOT=$root/feedback INTERFACE_SYSTEM_STORAGE_ROOT=$root/interface-systems SECURITY_ADVISORY_STORAGE_ROOT=$root/security-advisories SECURITY_EXPECTATION_STORAGE_ROOT=$root/security-expectations THREAT_MODEL_STORAGE_ROOT=$root/threat-models SECURITY_SCENARIO_STORAGE_ROOT=$root/security-scenarios SECURITY_FINDING_STORAGE_ROOT=$root/security-findings SECURITY_CONFIDENCE_STORAGE_ROOT=$root/security-confidence RELATIONSHIP_STORAGE_ROOT=$root/relationships DURABLE_SCHEMA_STORAGE_ROOT=$root/durable-schemas API_CONTRACT_STORAGE_ROOT=$root/api-contracts PACKAGE_STORAGE_ROOT=$root/packages ORGANIZATION_STORAGE_ROOT=$root/organizations AGENT_EVALUATION_STORAGE_ROOT=$root/agent-evaluations AGENT_PROJECT_STORAGE_ROOT=$root/agent-projects AGENT_CANDIDATE_STORAGE_ROOT=$root/agent-candidates AGENT_PILOT_STORAGE_ROOT=$root/agent-pilots AGENT_RELEASE_STORAGE_ROOT=$root/agent-releases WORKSPACE_STORAGE_ROOT=$root/workspaces EXPLANATION_STORAGE_ROOT=$root/explanations IMPACT_STORAGE_ROOT=$root/impact-assessments DECISION_STORAGE_ROOT=$root/decisions DELIVERY_TEAM_STORAGE_ROOT=$root/delivery-teams DOCUMENTATION_STORAGE_ROOT=$root/documentation EXTENSION_STORAGE_ROOT=$root/extensions FEDERATION_STORAGE_ROOT=$root/federation CHARTER_STORAGE_ROOT=$root/charters GOVERNANCE_STORAGE_ROOT=$root/governance PERFORMANCE_GOAL_STORAGE_ROOT=$root/performance-goals PERFORMANCE_EVIDENCE_STORAGE_ROOT=$root/performance-evidence PRODUCT_EXPERIMENT_STORAGE_ROOT=$root/product-experiments PROJECT_FUND_STORAGE_ROOT=$root/project-funds ACCESSIBILITY_COMMITMENT_STORAGE_ROOT=$root/accessibility-commitments ACCESSIBILITY_REPORT_STORAGE_ROOT=$root/accessibility-reports ACCESSIBILITY_ASSESSMENT_STORAGE_ROOT=$root/accessibility-assessments ACCESSIBILITY_DELIVERY_STORAGE_ROOT=$root/accessibility-delivery DATA_COMMITMENT_STORAGE_ROOT=$root/data-commitments DATA_FLOW_STORAGE_ROOT=$root/data-flows PRIVACY_REVIEW_STORAGE_ROOT=$root/privacy-reviews PRIVACY_CHECK_STORAGE_ROOT=$root/privacy-checks DATA_OBSERVATION_STORAGE_ROOT=$root/data-observations LOCALE_PLAN_STORAGE_ROOT=$root/locale-plans LOCALIZATION_STORAGE_ROOT=$root/localization SERVICE_OBJECTIVE_STORAGE_ROOT=$root/service-objectives RECOVERY_COMMITMENT_STORAGE_ROOT=$root/recovery-commitments PROTECTION_PLAN_STORAGE_ROOT=$root/protection-plans RECOVERY_EXERCISE_STORAGE_ROOT=$root/recovery-exercises RECOVERY_OPERATION_STORAGE_ROOT=$root/recovery-operations QUALITY_PLAN_STORAGE_ROOT=$root/quality-plans TEST_SCENARIO_STORAGE_ROOT=$root/test-scenarios EXPLORATORY_SESSION_STORAGE_ROOT=$root/exploratory-sessions RELEASE_CONFIDENCE_STORAGE_ROOT=$root/release-confidence ASSURANCE_PROGRAM_STORAGE_ROOT=$root/assurance-programs ASSURANCE_EVIDENCE_STORAGE_ROOT=$root/assurance-evidence ASSURANCE_ASSESSMENT_STORAGE_ROOT=$root/assurance-assessments ASSURANCE_IMPACT_STORAGE_ROOT=$root/assurance-impacts PROJECT_FUND_TRUSTED_SOURCES="{\\"community\\":\\"rPeXjD9B7ngjZSmWCcsA9HqTZOo9O0UAybBEgBsubzY=\\"}" EXTENSION_DEVELOPMENT_ENDPOINTS=1 go run .\'',
      cwd: "../api",
      env: { CAPABILITY_STORAGE_ROOT: capabilityStorageRoot },
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
