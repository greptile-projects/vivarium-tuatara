package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/acceptance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilitycommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilitydelivery"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityreports"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/adoptionworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentcandidates"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentevaluations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentpilots"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentprojects"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentreleases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceimpact"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capabilities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacitymodels"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacityobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changestacks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/charters"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/collaborationworkflows"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributoropportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributorpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/datacommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/dataflows"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/dataobservations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/debugworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deliveryteams"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/designgovernance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
	docscollections "github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/durableschemas"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/explanations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/exploratorysessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/extensions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/federation"
	productfeedback "github.com/greptile-projects/vivarium-tuatara/apps/api/feedback"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/governance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/historyremediations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/impacts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incubators"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/infrastructure"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/interfacechecks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/interfacesystems"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/knowledgeanswers"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/localeplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/localization"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/outcomevalidations"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/performanceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/performancegoals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/privacychecks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/privacyreviews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productexperiments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productopportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/projectfunds"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/propagationcampaigns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/protectionplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenanceassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancebundles"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancegraphs"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancepolicies"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/qualityplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoverycommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoveryexercises"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoveryoperations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/regressioninvestigations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releaseconfidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/restructuringplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/reviewplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/roadmaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityadvisories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityconfidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityexpectations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityfindings"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityscenarios"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportsolutions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportverifications"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/testscenarios"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/threatmodels"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workflowcomponents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

var defaultLearningAssessmentStore *learningassessments.Store

const (
	uploadPackService   = "git-upload-pack"
	receivePackService  = "git-receive-pack"
	branchNamespaceHook = `#!/bin/sh
while read -r old new ref
do
	case "$ref" in
		refs/heads/*) ;;
		*)
			echo "only branches may be updated" >&2
			exit 1
			;;
	esac
done
`
	contributorBranchHook = `#!/bin/sh
while read -r old new ref
do
	case "$ref" in
		refs/heads/main)
			echo "contributors may not update the default branch" >&2
			exit 1
			;;
		refs/heads/*) ;;
		*)
			echo "only branches may be updated" >&2
			exit 1
			;;
	esac
done
`
)

func main() {
	root := os.Getenv("GIT_STORAGE_ROOT")
	if root == "" {
		root = "repositories"
	}
	store, err := storage.New(root)
	if err != nil {
		log.Fatal(err)
	}
	userRoot := os.Getenv("USER_STORAGE_ROOT")
	if userRoot == "" {
		userRoot = "users"
	}
	userStore, err := users.New(userRoot)
	if err != nil {
		log.Fatal(err)
	}
	authRoot := os.Getenv("AUTH_STORAGE_ROOT")
	if authRoot == "" {
		authRoot = "credentials"
	}
	authStore, err := auth.New(authRoot)
	if err != nil {
		log.Fatal(err)
	}
	repositoryRoot := os.Getenv("REPOSITORY_STORAGE_ROOT")
	if repositoryRoot == "" {
		repositoryRoot = "repository-records"
	}
	repositoryStore, err := repositories.New(repositoryRoot, store)
	if err != nil {
		log.Fatal(err)
	}
	proposalRoot := os.Getenv("PROPOSAL_STORAGE_ROOT")
	if proposalRoot == "" {
		proposalRoot = "proposals"
	}
	proposalStore, err := proposals.New(proposalRoot)
	if err != nil {
		log.Fatal(err)
	}
	pullRequestRoot := os.Getenv("PULL_REQUEST_STORAGE_ROOT")
	if pullRequestRoot == "" {
		pullRequestRoot = "pull-requests"
	}
	pullRequestStore, err := pullrequests.New(pullRequestRoot, store)
	if err != nil {
		log.Fatal(err)
	}
	reviewPlanRoot := os.Getenv("REVIEW_PLAN_STORAGE_ROOT")
	if reviewPlanRoot == "" {
		reviewPlanRoot = "review-plans"
	}
	reviewPlanStore, err := reviewplans.New(reviewPlanRoot)
	if err != nil {
		log.Fatal(err)
	}
	activityRoot := os.Getenv("ACTIVITY_STORAGE_ROOT")
	if activityRoot == "" {
		activityRoot = "activity-records"
	}
	activityStore, err := activities.New(activityRoot)
	if err != nil {
		log.Fatal(err)
	}
	changeSessionRoot := os.Getenv("CHANGE_SESSION_STORAGE_ROOT")
	if changeSessionRoot == "" {
		changeSessionRoot = "change-sessions"
	}
	changeSessionStore, err := changesessions.New(changeSessionRoot)
	if err != nil {
		log.Fatal(err)
	}
	checkRunRoot := os.Getenv("CHECK_RUN_STORAGE_ROOT")
	if checkRunRoot == "" {
		checkRunRoot = "check-runs"
	}
	checkRunStore, err := checkruns.New(checkRunRoot)
	if err != nil {
		log.Fatal(err)
	}
	previewRoot := os.Getenv("PREVIEW_STORAGE_ROOT")
	if previewRoot == "" {
		previewRoot = "previews"
	}
	previewStore, err := previews.New(previewRoot)
	if err != nil {
		log.Fatal(err)
	}
	acceptanceRoot := os.Getenv("PREVIEW_ACCEPTANCE_STORAGE_ROOT")
	if acceptanceRoot == "" {
		acceptanceRoot = "preview-acceptance"
	}
	acceptanceStore, err := acceptance.New(acceptanceRoot)
	if err != nil {
		log.Fatal(err)
	}
	releaseRoot := os.Getenv("RELEASE_STORAGE_ROOT")
	if releaseRoot == "" {
		releaseRoot = "releases"
	}
	releaseStore, err := releases.New(releaseRoot)
	if err != nil {
		log.Fatal(err)
	}
	packageRoot := os.Getenv("PACKAGE_STORAGE_ROOT")
	if packageRoot == "" {
		packageRoot = "packages"
	}
	packageStore, err := packages.New(packageRoot)
	if err != nil {
		log.Fatal(err)
	}
	deploymentRoot := os.Getenv("DEPLOYMENT_STORAGE_ROOT")
	if deploymentRoot == "" {
		deploymentRoot = "deployments"
	}
	deploymentStore, err := deployments.New(deploymentRoot)
	if err != nil {
		log.Fatal(err)
	}
	relationshipRoot := os.Getenv("RELATIONSHIP_STORAGE_ROOT")
	if relationshipRoot == "" {
		relationshipRoot = "relationships"
	}
	relationshipStore, err := relationships.New(relationshipRoot)
	if err != nil {
		log.Fatal(err)
	}
	incidentRoot := os.Getenv("INCIDENT_STORAGE_ROOT")
	if incidentRoot == "" {
		incidentRoot = "incidents"
	}
	incidentStore, err := incidents.New(incidentRoot)
	if err != nil {
		log.Fatal(err)
	}
	issueRoot := os.Getenv("ISSUE_STORAGE_ROOT")
	if issueRoot == "" {
		issueRoot = "issues"
	}
	issueStore, err := issues.New(issueRoot)
	if err != nil {
		log.Fatal(err)
	}
	supportRoot := os.Getenv("SUPPORT_THREAD_STORAGE_ROOT")
	if supportRoot == "" {
		supportRoot = "support-threads"
	}
	supportThreadStore, err := supportthreads.New(supportRoot)
	if err != nil {
		log.Fatal(err)
	}
	supportVerificationRoot := os.Getenv("SUPPORT_VERIFICATION_STORAGE_ROOT")
	if supportVerificationRoot == "" {
		supportVerificationRoot = "support-verifications"
	}
	supportVerificationStore, err := supportverifications.New(supportVerificationRoot)
	if err != nil {
		log.Fatal(err)
	}
	supportSolutionRoot := os.Getenv("SUPPORT_SOLUTION_STORAGE_ROOT")
	if supportSolutionRoot == "" {
		supportSolutionRoot = "support-solutions"
	}
	supportSolutionStore, err := supportsolutions.New(supportSolutionRoot)
	if err != nil {
		log.Fatal(err)
	}
	knowledgeRoot := os.Getenv("KNOWLEDGE_ANSWER_STORAGE_ROOT")
	if knowledgeRoot == "" {
		knowledgeRoot = "knowledge-answers"
	}
	knowledgeAnswerStore, err := knowledgeanswers.New(knowledgeRoot)
	if err != nil {
		log.Fatal(err)
	}
	contributorPathwayRoot := os.Getenv("CONTRIBUTOR_PATHWAY_STORAGE_ROOT")
	if contributorPathwayRoot == "" {
		contributorPathwayRoot = "contributor-pathways"
	}
	contributorPathwayStore, err := contributorpathways.New(contributorPathwayRoot)
	if err != nil {
		log.Fatal(err)
	}
	learningPathwayRoot := os.Getenv("LEARNING_PATHWAY_STORAGE_ROOT")
	if learningPathwayRoot == "" {
		learningPathwayRoot = "learning-pathways"
	}
	learningPathwayStore, err := learningpathways.New(learningPathwayRoot)
	if err != nil {
		log.Fatal(err)
	}
	learningAssessmentRoot := os.Getenv("LEARNING_ASSESSMENT_STORAGE_ROOT")
	if learningAssessmentRoot == "" {
		learningAssessmentRoot = "learning-assessments"
	}
	learningAssessmentStore, err := learningassessments.New(learningAssessmentRoot)
	if err != nil {
		log.Fatal(err)
	}
	defaultLearningAssessmentStore = learningAssessmentStore
	contributorOpportunityRoot := os.Getenv("CONTRIBUTION_OPPORTUNITY_STORAGE_ROOT")
	if contributorOpportunityRoot == "" {
		contributorOpportunityRoot = "contribution-opportunities"
	}
	contributorOpportunityStore, err := contributoropportunities.New(contributorOpportunityRoot)
	if err != nil {
		log.Fatal(err)
	}
	documentationRoot := os.Getenv("DOCUMENTATION_STORAGE_ROOT")
	if documentationRoot == "" {
		documentationRoot = "documentation"
	}
	documentationStore, err := docscollections.New(documentationRoot)
	if err != nil {
		log.Fatal(err)
	}
	securityAdvisoryRoot := os.Getenv("SECURITY_ADVISORY_STORAGE_ROOT")
	if securityAdvisoryRoot == "" {
		securityAdvisoryRoot = "security-advisories"
	}
	securityAdvisoryStore, err := securityadvisories.New(securityAdvisoryRoot)
	if err != nil {
		log.Fatal(err)
	}
	organizationRoot := os.Getenv("ORGANIZATION_STORAGE_ROOT")
	if organizationRoot == "" {
		organizationRoot = "organizations"
	}
	organizationStore, err := organizations.New(organizationRoot)
	if err != nil {
		log.Fatal(err)
	}
	agentEvaluationRoot := os.Getenv("AGENT_EVALUATION_STORAGE_ROOT")
	if agentEvaluationRoot == "" {
		agentEvaluationRoot = "agent-evaluations"
	}
	agentEvaluationStore, err := agentevaluations.New(agentEvaluationRoot)
	if err != nil {
		log.Fatal(err)
	}
	agentCandidateRoot := os.Getenv("AGENT_CANDIDATE_STORAGE_ROOT")
	if agentCandidateRoot == "" {
		agentCandidateRoot = "agent-candidates"
	}
	agentCandidateStore, err := agentcandidates.New(agentCandidateRoot)
	if err != nil {
		log.Fatal(err)
	}
	agentProjectRoot := os.Getenv("AGENT_PROJECT_STORAGE_ROOT")
	if agentProjectRoot == "" {
		agentProjectRoot = "agent-projects"
	}
	agentProjectStore, err := agentprojects.New(agentProjectRoot)
	if err != nil {
		log.Fatal(err)
	}
	charterRoot := os.Getenv("CHARTER_STORAGE_ROOT")
	if charterRoot == "" {
		charterRoot = "charters"
	}
	charterStore, err := charters.New(charterRoot)
	if err != nil {
		log.Fatal(err)
	}
	governanceRoot := os.Getenv("GOVERNANCE_STORAGE_ROOT")
	if governanceRoot == "" {
		governanceRoot = "governance"
	}
	governanceStore, err := governance.New(governanceRoot)
	if err != nil {
		log.Fatal(err)
	}
	workspaceRoot := os.Getenv("WORKSPACE_STORAGE_ROOT")
	if workspaceRoot == "" {
		workspaceRoot = "workspaces"
	}
	workspaceStore, err := workspaces.New(workspaceRoot)
	if err != nil {
		log.Fatal(err)
	}
	explanationRoot := os.Getenv("EXPLANATION_STORAGE_ROOT")
	if explanationRoot == "" {
		explanationRoot = "explanations"
	}
	explanationStore, err := explanations.New(explanationRoot)
	if err != nil {
		log.Fatal(err)
	}
	impactRoot := os.Getenv("IMPACT_STORAGE_ROOT")
	if impactRoot == "" {
		impactRoot = "impact-assessments"
	}
	impactStore, err := impacts.New(impactRoot)
	if err != nil {
		log.Fatal(err)
	}
	decisionRoot := os.Getenv("DECISION_STORAGE_ROOT")
	if decisionRoot == "" {
		decisionRoot = "decisions"
	}
	decisionStore, err := decisions.New(decisionRoot)
	if err != nil {
		log.Fatal(err)
	}
	deliveryTeamRoot := os.Getenv("DELIVERY_TEAM_STORAGE_ROOT")
	if deliveryTeamRoot == "" {
		deliveryTeamRoot = "delivery-teams"
	}
	deliveryTeamStore, err := deliveryteams.New(deliveryTeamRoot)
	if err != nil {
		log.Fatal(err)
	}
	extensionRoot := os.Getenv("EXTENSION_STORAGE_ROOT")
	if extensionRoot == "" {
		extensionRoot = "extensions"
	}
	extensionStore, err := extensions.New(extensionRoot)
	if err != nil {
		log.Fatal(err)
	}
	federationRoot := os.Getenv("FEDERATION_STORAGE_ROOT")
	if federationRoot == "" {
		federationRoot = "federation"
	}
	federationStore, err := federation.New(federationRoot, os.Getenv("FEDERATION_INSTANCE_NAME"), os.Getenv("FEDERATION_PUBLIC_URL"), strings.Split(os.Getenv("FEDERATION_OPERATORS"), ","))
	if err != nil {
		log.Fatal(err)
	}
	performanceGoalRoot := os.Getenv("PERFORMANCE_GOAL_STORAGE_ROOT")
	if performanceGoalRoot == "" {
		performanceGoalRoot = "performance-goals"
	}
	performanceGoalStore, err := performancegoals.New(performanceGoalRoot)
	if err != nil {
		log.Fatal(err)
	}
	capacityObjectiveRoot := os.Getenv("CAPACITY_OBJECTIVE_STORAGE_ROOT")
	if capacityObjectiveRoot == "" {
		capacityObjectiveRoot = "capacity-objectives"
	}
	capacityObjectiveStore, err := capacityobjectives.New(capacityObjectiveRoot)
	if err != nil {
		log.Fatal(err)
	}
	capacityModelRoot := os.Getenv("CAPACITY_MODEL_STORAGE_ROOT")
	if capacityModelRoot == "" {
		capacityModelRoot = "capacity-models"
	}
	capacityModelStore, err := capacitymodels.New(capacityModelRoot)
	if err != nil {
		log.Fatal(err)
	}
	qualityPlanRoot := os.Getenv("QUALITY_PLAN_STORAGE_ROOT")
	if qualityPlanRoot == "" {
		qualityPlanRoot = "quality-plans"
	}
	qualityPlanStore, err := qualityplans.New(qualityPlanRoot)
	if err != nil {
		log.Fatal(err)
	}
	assuranceProgramRoot := os.Getenv("ASSURANCE_PROGRAM_STORAGE_ROOT")
	if assuranceProgramRoot == "" {
		assuranceProgramRoot = "assurance-programs"
	}
	assuranceProgramStore, err := assuranceprograms.New(assuranceProgramRoot)
	if err != nil {
		log.Fatal(err)
	}
	collaborationWorkflowRoot := os.Getenv("COLLABORATION_WORKFLOW_STORAGE_ROOT")
	if collaborationWorkflowRoot == "" {
		collaborationWorkflowRoot = "collaboration-workflows"
	}
	collaborationWorkflowStore, err := collaborationworkflows.New(collaborationWorkflowRoot)
	if err != nil {
		log.Fatal(err)
	}
	workflowComponentRoot := os.Getenv("WORKFLOW_COMPONENT_STORAGE_ROOT")
	if workflowComponentRoot == "" {
		workflowComponentRoot = "workflow-components"
	}
	workflowComponentStore, err := workflowcomponents.New(workflowComponentRoot)
	if err != nil {
		log.Fatal(err)
	}
	assuranceImpactRoot := os.Getenv("ASSURANCE_IMPACT_STORAGE_ROOT")
	if assuranceImpactRoot == "" {
		assuranceImpactRoot = "assurance-impacts"
	}
	assuranceImpactStore, err := assuranceimpact.New(assuranceImpactRoot)
	if err != nil {
		log.Fatal(err)
	}
	assuranceEvidenceRoot := os.Getenv("ASSURANCE_EVIDENCE_STORAGE_ROOT")
	if assuranceEvidenceRoot == "" {
		assuranceEvidenceRoot = "assurance-evidence"
	}
	assuranceEvidenceStore, err := assuranceevidence.New(assuranceEvidenceRoot)
	if err != nil {
		log.Fatal(err)
	}
	assuranceAssessmentRoot := os.Getenv("ASSURANCE_ASSESSMENT_STORAGE_ROOT")
	if assuranceAssessmentRoot == "" {
		assuranceAssessmentRoot = "assurance-assessments"
	}
	assuranceAssessmentStore, err := assuranceassessments.New(assuranceAssessmentRoot)
	if err != nil {
		log.Fatal(err)
	}
	securityExpectationRoot := os.Getenv("SECURITY_EXPECTATION_STORAGE_ROOT")
	if securityExpectationRoot == "" {
		securityExpectationRoot = "security-expectations"
	}
	securityExpectationStore, err := securityexpectations.New(securityExpectationRoot)
	if err != nil {
		log.Fatal(err)
	}
	threatModelRoot := os.Getenv("THREAT_MODEL_STORAGE_ROOT")
	if threatModelRoot == "" {
		threatModelRoot = "threat-models"
	}
	threatModelStore, err := threatmodels.New(threatModelRoot)
	if err != nil {
		log.Fatal(err)
	}
	securityScenarioRoot := os.Getenv("SECURITY_SCENARIO_STORAGE_ROOT")
	if securityScenarioRoot == "" {
		securityScenarioRoot = "security-scenarios"
	}
	securityScenarioStore, err := securityscenarios.New(securityScenarioRoot)
	if err != nil {
		log.Fatal(err)
	}
	testScenarioRoot := os.Getenv("TEST_SCENARIO_STORAGE_ROOT")
	if testScenarioRoot == "" {
		testScenarioRoot = "test-scenarios"
	}
	testScenarioStore, err := testscenarios.New(testScenarioRoot)
	if err != nil {
		log.Fatal(err)
	}
	exploratorySessionRoot := os.Getenv("EXPLORATORY_SESSION_STORAGE_ROOT")
	if exploratorySessionRoot == "" {
		exploratorySessionRoot = "exploratory-sessions"
	}
	exploratorySessionStore, err := exploratorysessions.New(exploratorySessionRoot)
	if err != nil {
		log.Fatal(err)
	}
	releaseConfidenceRoot := os.Getenv("RELEASE_CONFIDENCE_STORAGE_ROOT")
	if releaseConfidenceRoot == "" {
		releaseConfidenceRoot = "release-confidence"
	}
	releaseConfidenceStore, err := releaseconfidence.New(releaseConfidenceRoot)
	if err != nil {
		log.Fatal(err)
	}
	accessibilityCommitmentRoot := os.Getenv("ACCESSIBILITY_COMMITMENT_STORAGE_ROOT")
	if accessibilityCommitmentRoot == "" {
		accessibilityCommitmentRoot = "accessibility-commitments"
	}
	accessibilityCommitmentStore, err := accessibilitycommitments.New(accessibilityCommitmentRoot)
	if err != nil {
		log.Fatal(err)
	}
	dataCommitmentRoot := os.Getenv("DATA_COMMITMENT_STORAGE_ROOT")
	if dataCommitmentRoot == "" {
		dataCommitmentRoot = "data-commitments"
	}
	dataCommitmentStore, err := datacommitments.New(dataCommitmentRoot)
	if err != nil {
		log.Fatal(err)
	}
	dataFlowRoot := os.Getenv("DATA_FLOW_STORAGE_ROOT")
	if dataFlowRoot == "" {
		dataFlowRoot = "data-flows"
	}
	dataFlowStore, err := dataflows.New(dataFlowRoot)
	if err != nil {
		log.Fatal(err)
	}
	dataObservationRoot := os.Getenv("DATA_OBSERVATION_STORAGE_ROOT")
	if dataObservationRoot == "" {
		dataObservationRoot = "data-observations"
	}
	dataObservationStore, err := dataobservations.New(dataObservationRoot)
	if err != nil {
		log.Fatal(err)
	}
	localePlanRoot := os.Getenv("LOCALE_PLAN_STORAGE_ROOT")
	if localePlanRoot == "" {
		localePlanRoot = "locale-plans"
	}
	localePlanStore, err := localeplans.New(localePlanRoot)
	if err != nil {
		log.Fatal(err)
	}
	serviceObjectiveRoot := os.Getenv("SERVICE_OBJECTIVE_STORAGE_ROOT")
	if serviceObjectiveRoot == "" {
		serviceObjectiveRoot = "service-objectives"
	}
	serviceObjectiveStore, err := serviceobjectives.New(serviceObjectiveRoot)
	if err != nil {
		log.Fatal(err)
	}
	recoveryCommitmentRoot := os.Getenv("RECOVERY_COMMITMENT_STORAGE_ROOT")
	if recoveryCommitmentRoot == "" {
		recoveryCommitmentRoot = "recovery-commitments"
	}
	recoveryCommitmentStore, err := recoverycommitments.New(recoveryCommitmentRoot)
	if err != nil {
		log.Fatal(err)
	}
	apiContractRoot := os.Getenv("API_CONTRACT_STORAGE_ROOT")
	if apiContractRoot == "" {
		apiContractRoot = "api-contracts"
	}
	apiContractStore, err := apicontracts.New(apiContractRoot)
	if err != nil {
		log.Fatal(err)
	}
	durableSchemaRoot := os.Getenv("DURABLE_SCHEMA_STORAGE_ROOT")
	if durableSchemaRoot == "" {
		durableSchemaRoot = "durable-schemas"
	}
	durableSchemaStore, err := durableschemas.New(durableSchemaRoot)
	if err != nil {
		log.Fatal(err)
	}
	infrastructureRoot := os.Getenv("INFRASTRUCTURE_STORAGE_ROOT")
	if infrastructureRoot == "" {
		infrastructureRoot = "infrastructure"
	}
	infrastructureStore, err := infrastructure.New(infrastructureRoot)
	if err != nil {
		log.Fatal(err)
	}
	debugWorkspaceRoot := os.Getenv("DEBUG_WORKSPACE_STORAGE_ROOT")
	if debugWorkspaceRoot == "" {
		debugWorkspaceRoot = "debugging-workspaces"
	}
	debugWorkspaceStore, err := debugworkspaces.New(debugWorkspaceRoot)
	if err != nil {
		log.Fatal(err)
	}
	regressionRoot := os.Getenv("REGRESSION_INVESTIGATION_STORAGE_ROOT")
	if regressionRoot == "" {
		regressionRoot = "regression-investigations"
	}
	regressionInvestigationStore, err := regressioninvestigations.New(regressionRoot)
	if err != nil {
		log.Fatal(err)
	}
	propagationRoot := os.Getenv("PROPAGATION_CAMPAIGN_STORAGE_ROOT")
	if propagationRoot == "" {
		propagationRoot = "propagation-campaigns"
	}
	propagationCampaignStore, err := propagationcampaigns.New(propagationRoot)
	if err != nil {
		log.Fatal(err)
	}
	changeStackRoot := os.Getenv("CHANGE_STACK_STORAGE_ROOT")
	if changeStackRoot == "" {
		changeStackRoot = "change-stacks"
	}
	changeStackStore, err := changestacks.New(changeStackRoot)
	if err != nil {
		log.Fatal(err)
	}
	historyRemediationRoot := os.Getenv("HISTORY_REMEDIATION_STORAGE_ROOT")
	if historyRemediationRoot == "" {
		historyRemediationRoot = "history-remediations"
	}
	historyRemediationStore, err := historyremediations.New(historyRemediationRoot)
	if err != nil {
		log.Fatal(err)
	}
	restructuringPlanRoot := os.Getenv("RESTRUCTURING_PLAN_STORAGE_ROOT")
	if restructuringPlanRoot == "" {
		restructuringPlanRoot = "restructuring-plans"
	}
	restructuringPlanStore, err := restructuringplans.New(restructuringPlanRoot)
	if err != nil {
		log.Fatal(err)
	}
	interfaceSystemRoot := os.Getenv("INTERFACE_SYSTEM_STORAGE_ROOT")
	if interfaceSystemRoot == "" {
		interfaceSystemRoot = "interface-systems"
	}
	interfaceSystemStore, err := interfacesystems.New(interfaceSystemRoot)
	if err != nil {
		log.Fatal(err)
	}
	capabilityRoot := os.Getenv("CAPABILITY_STORAGE_ROOT")
	if capabilityRoot == "" {
		capabilityRoot = "capabilities"
	}
	capabilityStore, err := capabilities.New(capabilityRoot)
	if err != nil {
		log.Fatal(err)
	}
	designProposalRoot := os.Getenv("DESIGN_PROPOSAL_STORAGE_ROOT")
	if designProposalRoot == "" {
		designProposalRoot = "design-proposals"
	}
	designProposalStore, err := designproposals.New(designProposalRoot)
	if err != nil {
		log.Fatal(err)
	}
	interfaceCheckRoot := os.Getenv("INTERFACE_CHECK_STORAGE_ROOT")
	if interfaceCheckRoot == "" {
		interfaceCheckRoot = "interface-checks"
	}
	interfaceCheckStore, err := interfacechecks.New(interfaceCheckRoot)
	if err != nil {
		log.Fatal(err)
	}
	designGovernanceRoot := os.Getenv("DESIGN_GOVERNANCE_STORAGE_ROOT")
	if designGovernanceRoot == "" {
		designGovernanceRoot = "design-governance"
	}
	designGovernanceStore, err := designgovernance.New(designGovernanceRoot)
	if err != nil {
		log.Fatal(err)
	}
	protectionPlanRoot := os.Getenv("PROTECTION_PLAN_STORAGE_ROOT")
	if protectionPlanRoot == "" {
		protectionPlanRoot = "protection-plans"
	}
	protectionPlanStore, err := protectionplans.New(protectionPlanRoot)
	if err != nil {
		log.Fatal(err)
	}
	recoveryExerciseRoot := os.Getenv("RECOVERY_EXERCISE_STORAGE_ROOT")
	if recoveryExerciseRoot == "" {
		recoveryExerciseRoot = "recovery-exercises"
	}
	recoveryExerciseStore, err := recoveryexercises.New(recoveryExerciseRoot)
	if err != nil {
		log.Fatal(err)
	}
	recoveryOperationRoot := os.Getenv("RECOVERY_OPERATION_STORAGE_ROOT")
	if recoveryOperationRoot == "" {
		recoveryOperationRoot = "recovery-operations"
	}
	recoveryOperationStore, err := recoveryoperations.New(recoveryOperationRoot)
	if err != nil {
		log.Fatal(err)
	}
	localizationRoot := os.Getenv("LOCALIZATION_STORAGE_ROOT")
	if localizationRoot == "" {
		localizationRoot = "localization"
	}
	localizationStore, err := localization.New(localizationRoot)
	if err != nil {
		log.Fatal(err)
	}
	privacyReviewRoot := os.Getenv("PRIVACY_REVIEW_STORAGE_ROOT")
	if privacyReviewRoot == "" {
		privacyReviewRoot = "privacy-reviews"
	}
	privacyReviewStore, err := privacyreviews.New(privacyReviewRoot)
	if err != nil {
		log.Fatal(err)
	}
	privacyCheckRoot := os.Getenv("PRIVACY_CHECK_STORAGE_ROOT")
	if privacyCheckRoot == "" {
		privacyCheckRoot = "privacy-checks"
	}
	privacyCheckStore, err := privacychecks.New(privacyCheckRoot)
	if err != nil {
		log.Fatal(err)
	}
	accessibilityReportRoot := os.Getenv("ACCESSIBILITY_REPORT_STORAGE_ROOT")
	if accessibilityReportRoot == "" {
		accessibilityReportRoot = "accessibility-reports"
	}
	accessibilityReportStore, err := accessibilityreports.New(accessibilityReportRoot)
	if err != nil {
		log.Fatal(err)
	}
	accessibilityAssessmentRoot := os.Getenv("ACCESSIBILITY_ASSESSMENT_STORAGE_ROOT")
	if accessibilityAssessmentRoot == "" {
		accessibilityAssessmentRoot = "accessibility-assessments"
	}
	accessibilityAssessmentStore, err := accessibilityassessments.New(accessibilityAssessmentRoot)
	if err != nil {
		log.Fatal(err)
	}
	accessibilityDeliveryRoot := os.Getenv("ACCESSIBILITY_DELIVERY_STORAGE_ROOT")
	if accessibilityDeliveryRoot == "" {
		accessibilityDeliveryRoot = "accessibility-delivery"
	}
	accessibilityDeliveryStore, err := accessibilitydelivery.New(accessibilityDeliveryRoot)
	if err != nil {
		log.Fatal(err)
	}
	performanceEvidenceRoot := os.Getenv("PERFORMANCE_EVIDENCE_STORAGE_ROOT")
	if performanceEvidenceRoot == "" {
		performanceEvidenceRoot = "performance-evidence"
	}
	performanceEvidenceStore, err := performanceevidence.New(performanceEvidenceRoot)
	if err != nil {
		log.Fatal(err)
	}
	productExperimentRoot := os.Getenv("PRODUCT_EXPERIMENT_STORAGE_ROOT")
	if productExperimentRoot == "" {
		productExperimentRoot = "product-experiments"
	}
	productExperimentStore, err := productexperiments.New(productExperimentRoot)
	if err != nil {
		log.Fatal(err)
	}
	feedbackRoot := os.Getenv("FEEDBACK_STORAGE_ROOT")
	if feedbackRoot == "" {
		feedbackRoot = "feedback"
	}
	feedbackStore, err := productfeedback.New(feedbackRoot)
	if err != nil {
		log.Fatal(err)
	}
	productOpportunityRoot := os.Getenv("PRODUCT_OPPORTUNITY_STORAGE_ROOT")
	if productOpportunityRoot == "" {
		productOpportunityRoot = "product-opportunities"
	}
	productOpportunityStore, err := productopportunities.New(productOpportunityRoot)
	if err != nil {
		log.Fatal(err)
	}
	roadmapRoot := os.Getenv("ROADMAP_STORAGE_ROOT")
	if roadmapRoot == "" {
		roadmapRoot = "roadmaps"
	}
	roadmapStore, err := roadmaps.New(roadmapRoot)
	if err != nil {
		log.Fatal(err)
	}
	outcomeValidationRoot := os.Getenv("OUTCOME_VALIDATION_STORAGE_ROOT")
	if outcomeValidationRoot == "" {
		outcomeValidationRoot = "outcome-validations"
	}
	outcomeValidationStore, err := outcomevalidations.New(outcomeValidationRoot)
	if err != nil {
		log.Fatal(err)
	}
	projectFundRoot := os.Getenv("PROJECT_FUND_STORAGE_ROOT")
	if projectFundRoot == "" {
		projectFundRoot = "project-funds"
	}
	trustedFundSources := map[string]string{}
	if raw := os.Getenv("PROJECT_FUND_TRUSTED_SOURCES"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &trustedFundSources); err != nil {
			log.Fatal("invalid PROJECT_FUND_TRUSTED_SOURCES")
		}
	}
	projectFundStore, err := projectfunds.New(projectFundRoot, trustedFundSources)
	if err != nil {
		log.Fatal(err)
	}
	incubatorRoot := os.Getenv("INCUBATOR_STORAGE_ROOT")
	if incubatorRoot == "" {
		incubatorRoot = "incubators"
	}
	incubatorStore, err := incubators.New(incubatorRoot)
	if err != nil {
		log.Fatal(err)
	}
	adoptionWorkspaceRoot := os.Getenv("ADOPTION_WORKSPACE_STORAGE_ROOT")
	if adoptionWorkspaceRoot == "" {
		adoptionWorkspaceRoot = "adoption-workspaces"
	}
	adoptionWorkspaceStore, err := adoptionworkspaces.New(adoptionWorkspaceRoot)
	if err != nil {
		log.Fatal(err)
	}
	securityFindingRoot := os.Getenv("SECURITY_FINDING_STORAGE_ROOT")
	if securityFindingRoot == "" {
		securityFindingRoot = "security-findings"
	}
	securityFindingStore, err := securityfindings.New(securityFindingRoot)
	if err != nil {
		log.Fatal(err)
	}
	securityConfidenceRoot := os.Getenv("SECURITY_CONFIDENCE_STORAGE_ROOT")
	if securityConfidenceRoot == "" {
		securityConfidenceRoot = "security-confidence"
	}
	securityConfidenceStore, err := securityconfidence.New(securityConfidenceRoot)
	if err != nil {
		log.Fatal(err)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	handler := newPlatformHandlerWithChecks(store, userStore, authStore, repositoryStore, proposalStore, pullRequestStore, activityStore, changeSessionStore, checkRunStore, reviewPlanStore, previewStore, acceptanceStore, releaseStore, deploymentStore, incidentStore, securityAdvisoryStore, relationshipStore, packageStore, organizationStore, charterStore, governanceStore, workspaceStore, explanationStore, impactStore, decisionStore, deliveryTeamStore, issueStore, supportThreadStore, supportVerificationStore, supportSolutionStore, knowledgeAnswerStore, contributorPathwayStore, learningPathwayStore, contributorOpportunityStore, documentationStore, extensionStore, federationStore, performanceGoalStore, capacityObjectiveStore, capacityModelStore, performanceEvidenceStore, productExperimentStore, feedbackStore, productOpportunityStore, roadmapStore, outcomeValidationStore, projectFundStore, incubatorStore, adoptionWorkspaceStore, accessibilityCommitmentStore, accessibilityReportStore, accessibilityAssessmentStore, accessibilityDeliveryStore, dataCommitmentStore, dataFlowStore, privacyReviewStore, privacyCheckStore, dataObservationStore, localePlanStore, localizationStore, serviceObjectiveStore, recoveryCommitmentStore, protectionPlanStore, recoveryExerciseStore, recoveryOperationStore, agentEvaluationStore, agentProjectStore, agentCandidateStore, apiContractStore, durableSchemaStore, infrastructureStore, debugWorkspaceStore, interfaceSystemStore, capabilityStore, designProposalStore, interfaceCheckStore, designGovernanceStore, qualityPlanStore, assuranceProgramStore, assuranceEvidenceStore, assuranceImpactStore, assuranceAssessmentStore, testScenarioStore, exploratorySessionStore, releaseConfidenceStore, securityExpectationStore, threatModelStore, securityScenarioStore, securityFindingStore, securityConfidenceStore, collaborationWorkflowStore, workflowComponentStore, regressionInvestigationStore, propagationCampaignStore, historyRemediationStore, restructuringPlanStore, changeStackStore)
	startCheckRunRecovery(store, checkRunStore)
	startIntegrationQueueRecovery(pullRequestStore)
	startDeploymentRecovery(deploymentStore, checkRunStore)
	startWorkspaceRecovery(workspaceStore)
	startExtensionDeliveryRecovery(activityStore, extensionStore)
	log.Printf("listening on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

func startDeploymentRecovery(store *deployments.Store, builds *checkruns.Store) {
	executor := deployments.NewExecutor(store, builds)
	recover := func() {
		if err := executor.Recover(); err != nil {
			log.Printf("recover deployments: %v", err)
		}
	}
	recover()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			recover()
		}
	}()
}

func startIntegrationQueueRecovery(store *pullrequests.Store) {
	advance := func() {
		if err := store.AdvanceIntegrationQueues(); err != nil {
			log.Printf("advance integration queues: %v", err)
		}
	}
	advance()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			advance()
		}
	}()
}

func projectExtensionActivity(store *extensions.Store, event activities.Event) error {
	resourceType := event.ResourceType
	if strings.HasPrefix(event.Kind, "check.") {
		resourceType = "check"
	}
	_, err := store.EnqueueProjectEvent(extensions.ProjectEvent{ID: event.ID, Type: event.Kind, RepositoryID: event.RepositoryID, ResourceType: resourceType, ResourceID: event.ResourceID, ActorID: event.ActorID, Title: event.ResourceTitle, OccurredAt: event.CreatedAt})
	return err
}

// Recovery scans the immutable activity ledger oldest-first. Enqueue is
// idempotent, so stopping at the first failure preserves commit ordering and a
// later pass safely resumes without duplicating deliveries.
func recoverExtensionDeliveries(activityStore *activities.Store, extensionStore *extensions.Store) error {
	events, err := activityStore.List()
	if err != nil {
		return err
	}
	for i := len(events) - 1; i >= 0; i-- {
		if err = projectExtensionActivity(extensionStore, events[i]); err != nil {
			return err
		}
	}
	return nil
}

func startExtensionDeliveryRecovery(activityStore *activities.Store, extensionStore *extensions.Store) {
	if activityStore == nil || extensionStore == nil {
		return
	}
	recover := func() {
		if err := recoverExtensionDeliveries(activityStore, extensionStore); err != nil {
			log.Printf("recover extension deliveries: %v", err)
		}
	}
	recover()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			recover()
		}
	}()
}

func newHandler(store *storage.Store) http.Handler {
	return newAppHandler(store, nil)
}

func newAppHandler(store *storage.Store, userStore *users.Store) http.Handler {
	return newAuthenticatedAppHandler(store, userStore, nil)
}

func newAuthenticatedAppHandler(store *storage.Store, userStore *users.Store, authStore *auth.Store, catalogs ...*repositories.Store) http.Handler {
	var repositoryCatalog *repositories.Store
	if len(catalogs) > 0 {
		repositoryCatalog = catalogs[0]
	}
	return newPlatformHandler(store, userStore, authStore, repositoryCatalog, nil, nil, nil)
}

func newPlatformHandler(store *storage.Store, userStore *users.Store, authStore *auth.Store, repositoryCatalog *repositories.Store, proposalStore *proposals.Store, pullRequestStore *pullrequests.Store, activityStore *activities.Store, sessionStores ...*changesessions.Store) http.Handler {
	var changeSessionStore *changesessions.Store
	if len(sessionStores) > 0 {
		changeSessionStore = sessionStores[0]
	}
	return newPlatformHandlerWithChecks(store, userStore, authStore, repositoryCatalog, proposalStore, pullRequestStore, activityStore, changeSessionStore, nil)
}

func newPlatformHandlerWithChecks(store *storage.Store, userStore *users.Store, authStore *auth.Store, repositoryCatalog *repositories.Store, proposalStore *proposals.Store, pullRequestStore *pullrequests.Store, activityStore *activities.Store, changeSessionStore *changesessions.Store, checkRunStore *checkruns.Store, optionalStores ...any) http.Handler {
	if defaultLearningAssessmentStore != nil {
		optionalStores = append(optionalStores, defaultLearningAssessmentStore)
	}
	var releaseStore *releases.Store
	var deploymentStore *deployments.Store
	var incidentStore *incidents.Store
	var securityAdvisoryStore *securityadvisories.Store
	var relationshipStore *relationships.Store
	var packageStore *packages.Store
	var organizationStore *organizations.Store
	var charterStore *charters.Store
	var governanceStore *governance.Store
	var workspaceStore *workspaces.Store
	var explanationStore *explanations.Store
	var impactStore *impacts.Store
	var decisionStore *decisions.Store
	var deliveryTeamStore *deliveryteams.Store
	var issueStore *issues.Store
	var supportThreadStore *supportthreads.Store
	var supportVerificationStore *supportverifications.Store
	var supportSolutionStore *supportsolutions.Store
	var knowledgeAnswerStore *knowledgeanswers.Store
	var contributorPathwayStore *contributorpathways.Store
	var learningPathwayStore *learningpathways.Store
	var learningAssessmentStore *learningassessments.Store
	var contributorOpportunityStore *contributoropportunities.Store
	var previewStore *previews.Store
	var acceptanceStore *acceptance.Store
	var documentationStore *docscollections.Store
	var extensionStore *extensions.Store
	var federationStore *federation.Store
	var performanceGoalStore *performancegoals.Store
	var capacityObjectiveStore *capacityobjectives.Store
	var capacityModelStore *capacitymodels.Store
	var performanceEvidenceStore *performanceevidence.Store
	var productExperimentStore *productexperiments.Store
	var feedbackStore *productfeedback.Store
	var productOpportunityStore *productopportunities.Store
	var roadmapStore *roadmaps.Store
	var outcomeValidationStore *outcomevalidations.Store
	var projectFundStore *projectfunds.Store
	var incubatorStore *incubators.Store
	var adoptionWorkspaceStore *adoptionworkspaces.Store
	var accessibilityCommitmentStore *accessibilitycommitments.Store
	var accessibilityReportStore *accessibilityreports.Store
	var accessibilityAssessmentStore *accessibilityassessments.Store
	var accessibilityDeliveryStore *accessibilitydelivery.Store
	var dataCommitmentStore *datacommitments.Store
	var dataFlowStore *dataflows.Store
	var privacyReviewStore *privacyreviews.Store
	var privacyCheckStore *privacychecks.Store
	var dataObservationStore *dataobservations.Store
	var localePlanStore *localeplans.Store
	var localizationStore *localization.Store
	var serviceObjectiveStore *serviceobjectives.Store
	var recoveryCommitmentStore *recoverycommitments.Store
	var protectionPlanStore *protectionplans.Store
	var recoveryExerciseStore *recoveryexercises.Store
	var recoveryOperationStore *recoveryoperations.Store
	var agentEvaluationStore *agentevaluations.Store
	var agentProjectStore *agentprojects.Store
	var agentCandidateStore *agentcandidates.Store
	agentPilotRoot := os.Getenv("AGENT_PILOT_STORAGE_ROOT")
	if agentPilotRoot == "" {
		agentPilotRoot = "agent-pilots"
	}
	agentPilotStore, _ := agentpilots.New(agentPilotRoot)
	agentReleaseRoot := os.Getenv("AGENT_RELEASE_STORAGE_ROOT")
	if agentReleaseRoot == "" {
		agentReleaseRoot = "agent-releases"
	}
	agentReleaseStore, agentReleaseStoreErr := agentreleases.New(agentReleaseRoot)
	var apiContractStore *apicontracts.Store
	var durableSchemaStore *durableschemas.Store
	var infrastructureStore *infrastructure.Store
	var debugWorkspaceStore *debugworkspaces.Store
	var regressionInvestigationStore *regressioninvestigations.Store
	var propagationCampaignStore *propagationcampaigns.Store
	var historyRemediationStore *historyremediations.Store
	var restructuringPlanStore *restructuringplans.Store
	var changeStackStore *changestacks.Store
	var interfaceSystemStore *interfacesystems.Store
	var capabilityStore *capabilities.Store
	var designProposalStore *designproposals.Store
	var interfaceCheckStore *interfacechecks.Store
	var designGovernanceStore *designgovernance.Store
	var qualityPlanStore *qualityplans.Store
	var assuranceProgramStore *assuranceprograms.Store
	var provenancePolicyStore *provenancepolicies.Store
	var provenanceGraphStore *provenancegraphs.Store
	var provenanceAssessmentStore *provenanceassessments.Store
	var provenanceBundleStore *provenancebundles.Store
	var assuranceEvidenceStore *assuranceevidence.Store
	var assuranceImpactStore *assuranceimpact.Store
	var assuranceAssessmentStore *assuranceassessments.Store
	var testScenarioStore *testscenarios.Store
	var exploratorySessionStore *exploratorysessions.Store
	var releaseConfidenceStore *releaseconfidence.Store
	var securityExpectationStore *securityexpectations.Store
	var threatModelStore *threatmodels.Store
	var securityScenarioStore *securityscenarios.Store
	var securityFindingStore *securityfindings.Store
	var securityConfidenceStore *securityconfidence.Store
	var collaborationWorkflowStore *collaborationworkflows.Store
	var workflowComponentStore *workflowcomponents.Store
	var reviewPlanStore *reviewplans.Store
	for _, optional := range optionalStores {
		switch value := optional.(type) {
		case *releases.Store:
			releaseStore = value
		case *deployments.Store:
			deploymentStore = value
		case *incidents.Store:
			incidentStore = value
		case *securityadvisories.Store:
			securityAdvisoryStore = value
		case *relationships.Store:
			relationshipStore = value
		case *packages.Store:
			packageStore = value
		case *organizations.Store:
			organizationStore = value
		case *charters.Store:
			charterStore = value
		case *governance.Store:
			governanceStore = value
		case *workspaces.Store:
			workspaceStore = value
		case *explanations.Store:
			explanationStore = value
		case *impacts.Store:
			impactStore = value
		case *decisions.Store:
			decisionStore = value
		case *deliveryteams.Store:
			deliveryTeamStore = value
		case *issues.Store:
			issueStore = value
		case *supportthreads.Store:
			supportThreadStore = value
		case *supportverifications.Store:
			supportVerificationStore = value
		case *supportsolutions.Store:
			supportSolutionStore = value
		case *knowledgeanswers.Store:
			knowledgeAnswerStore = value
		case *contributorpathways.Store:
			contributorPathwayStore = value
		case *learningpathways.Store:
			learningPathwayStore = value
		case *learningassessments.Store:
			learningAssessmentStore = value
		case *contributoropportunities.Store:
			contributorOpportunityStore = value
		case *previews.Store:
			previewStore = value
		case *acceptance.Store:
			acceptanceStore = value
		case *docscollections.Store:
			documentationStore = value
		case *extensions.Store:
			extensionStore = value
		case *federation.Store:
			federationStore = value
		case *performancegoals.Store:
			performanceGoalStore = value
		case *capacityobjectives.Store:
			capacityObjectiveStore = value
		case *capacitymodels.Store:
			capacityModelStore = value
		case *performanceevidence.Store:
			performanceEvidenceStore = value
		case *productexperiments.Store:
			productExperimentStore = value
		case *productfeedback.Store:
			feedbackStore = value
		case *productopportunities.Store:
			productOpportunityStore = value
		case *roadmaps.Store:
			roadmapStore = value
		case *outcomevalidations.Store:
			outcomeValidationStore = value
		case *projectfunds.Store:
			projectFundStore = value
		case *incubators.Store:
			incubatorStore = value
		case *adoptionworkspaces.Store:
			adoptionWorkspaceStore = value
		case *accessibilitycommitments.Store:
			accessibilityCommitmentStore = value
		case *accessibilityreports.Store:
			accessibilityReportStore = value
		case *accessibilityassessments.Store:
			accessibilityAssessmentStore = value
		case *accessibilitydelivery.Store:
			accessibilityDeliveryStore = value
		case *datacommitments.Store:
			dataCommitmentStore = value
		case *dataflows.Store:
			dataFlowStore = value
		case *privacyreviews.Store:
			privacyReviewStore = value
		case *privacychecks.Store:
			privacyCheckStore = value
		case *dataobservations.Store:
			dataObservationStore = value
		case *localeplans.Store:
			localePlanStore = value
		case *localization.Store:
			localizationStore = value
		case *serviceobjectives.Store:
			serviceObjectiveStore = value
		case *recoverycommitments.Store:
			recoveryCommitmentStore = value
		case *protectionplans.Store:
			protectionPlanStore = value
		case *recoveryexercises.Store:
			recoveryExerciseStore = value
		case *recoveryoperations.Store:
			recoveryOperationStore = value
		case *durableschemas.Store:
			durableSchemaStore = value
		case *agentevaluations.Store:
			agentEvaluationStore = value
		case *agentprojects.Store:
			agentProjectStore = value
		case *agentcandidates.Store:
			agentCandidateStore = value
		case *apicontracts.Store:
			apiContractStore = value
		case *infrastructure.Store:
			infrastructureStore = value
		case *debugworkspaces.Store:
			debugWorkspaceStore = value
		case *regressioninvestigations.Store:
			regressionInvestigationStore = value
		case *propagationcampaigns.Store:
			propagationCampaignStore = value
		case *historyremediations.Store:
			historyRemediationStore = value
		case *restructuringplans.Store:
			restructuringPlanStore = value
		case *changestacks.Store:
			changeStackStore = value
		case *interfacesystems.Store:
			interfaceSystemStore = value
		case *capabilities.Store:
			capabilityStore = value
		case *designproposals.Store:
			designProposalStore = value
		case *interfacechecks.Store:
			interfaceCheckStore = value
		case *designgovernance.Store:
			designGovernanceStore = value
		case *qualityplans.Store:
			qualityPlanStore = value
		case *assuranceprograms.Store:
			assuranceProgramStore = value
		case *provenancepolicies.Store:
			provenancePolicyStore = value
		case *provenancegraphs.Store:
			provenanceGraphStore = value
		case *provenanceassessments.Store:
			provenanceAssessmentStore = value
		case *provenancebundles.Store:
			provenanceBundleStore = value
		case *assuranceevidence.Store:
			assuranceEvidenceStore = value
		case *assuranceimpact.Store:
			assuranceImpactStore = value
		case *assuranceassessments.Store:
			assuranceAssessmentStore = value
		case *testscenarios.Store:
			testScenarioStore = value
		case *exploratorysessions.Store:
			exploratorySessionStore = value
		case *releaseconfidence.Store:
			releaseConfidenceStore = value
		case *securityexpectations.Store:
			securityExpectationStore = value
		case *threatmodels.Store:
			threatModelStore = value
		case *securityscenarios.Store:
			securityScenarioStore = value
		case *securityfindings.Store:
			securityFindingStore = value
		case *securityconfidence.Store:
			securityConfidenceStore = value
		case *collaborationworkflows.Store:
			collaborationWorkflowStore = value
		case *workflowcomponents.Store:
			workflowComponentStore = value
		case *reviewplans.Store:
			reviewPlanStore = value
		}
	}
	mux := http.NewServeMux()
	if activityStore != nil && extensionStore != nil {
		activityStore.SetObserver(func(activities.Event) error { return recoverExtensionDeliveries(activityStore, extensionStore) })
	}
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	if userStore != nil {
		registerUserRoutes(mux, userStore, authStore)
	}
	if incubatorStore != nil && authStore != nil && userStore != nil && repositoryCatalog != nil && organizationStore != nil {
		registerIncubatorRoutes(mux, store, authStore, userStore, repositoryCatalog, organizationStore, incubatorStore, feedbackStore, supportThreadStore, governanceStore, workspaceStore, pullRequestStore, previewStore, checkRunStore, releaseStore, documentationStore, packageStore, apiContractStore, contributorOpportunityStore, deploymentStore, roadmapStore, serviceObjectiveStore, projectFundStore, outcomeValidationStore)
	}
	if adoptionWorkspaceStore != nil && authStore != nil && userStore != nil && repositoryCatalog != nil && organizationStore != nil {
		registerAdoptionWorkspaceRoutes(mux, authStore, userStore, repositoryCatalog, organizationStore, incubatorStore, federationStore, roadmapStore, supportThreadStore, decisionStore, packageStore, apiContractStore, releaseStore, checkRunStore, pullRequestStore, issueStore, deploymentStore, adoptionWorkspaceStore)
	}
	if federationStore != nil && userStore != nil && authStore != nil {
		registerFederationRoutes(mux, federationStore, userStore, organizationStore, authStore, store, repositoryCatalog, pullRequestStore, changeSessionStore, releaseStore, issueStore, contributorPathwayStore, contributorOpportunityStore)
		startFederationDeliveryRecovery(federationStore)
	}
	if authStore != nil {
		registerAuthRoutes(mux, authStore)
		if extensionStore != nil {
			registerExtensionRoutes(mux, extensionStore, authStore)
		}
	}
	if authStore != nil && repositoryCatalog != nil {
		if extensionStore != nil && organizationStore != nil {
			registerExtensionInstallationRoutes(mux, extensionStore, authStore, repositoryCatalog, organizationStore)
		}
		registerRepositoryRoutes(mux, store, repositoryCatalog, userStore, authStore, activityStore)
		registerCodeNavigationRoutes(mux, store, repositoryCatalog, authStore, relationshipStore)
		if documentationStore != nil {
			registerDocumentationRoutes(mux, store, repositoryCatalog, documentationStore, releaseStore, issueStore, proposalStore, authStore)
		}
	}
	if authStore != nil && repositoryCatalog != nil && proposalStore != nil {
		registerProposalRoutes(mux, store, repositoryCatalog, proposalStore, authStore, activityStore, userStore)
	}
	if authStore != nil && repositoryCatalog != nil && pullRequestStore != nil {
		if extensionStore != nil {
			registerExtensionContributionRoutes(mux, extensionStore, authStore, repositoryCatalog, pullRequestStore)
		}
		if acceptanceStore != nil {
			pullRequestStore.ConfigurePreviewAcceptance(acceptanceStore, previewStore)
			registerAcceptanceRoutes(mux, repositoryCatalog, pullRequestStore, acceptanceStore, previewStore, authStore)
		}
		registerPullRequestRoutes(mux, store, repositoryCatalog, proposalStore, pullRequestStore, authStore, activityStore, userStore, checkRunStore, changeSessionStore, documentationStore, durableSchemaStore, federationStore)
		if reviewPlanStore != nil {
			configureReviewReadiness(pullRequestStore, reviewPlanStore)
			registerReviewReadinessRoute(mux, repositoryCatalog, authStore, pullRequestStore, reviewPlanStore)
			registerReviewPlanRoutes(mux, repositoryCatalog, authStore, pullRequestStore, reviewPlanStore)
			registerReviewAssignmentRoutes(mux, repositoryCatalog, authStore, pullRequestStore, reviewPlanStore, organizationStore)
			registerReviewWorkRoutes(mux, store, repositoryCatalog, authStore, pullRequestStore, reviewPlanStore, organizationStore, checkRunStore, previewStore, decisionStore, proposalStore, changeSessionStore, workspaceStore)
		}
		if documentationStore != nil {
			registerDocumentationReviewRoutes(mux, store, repositoryCatalog, pullRequestStore, documentationStore, checkRunStore, authStore)
		}
		if previewStore != nil && checkRunStore != nil {
			registerPreviewRoutes(mux, store, repositoryCatalog, pullRequestStore, checkRunStore, previewStore, changeSessionStore, authStore, userStore, proposalStore, decisionStore, issueStore, activityStore)
		}
	}
	if authStore != nil && repositoryCatalog != nil && pullRequestStore != nil && changeSessionStore != nil {
		registerChangeSessionRoutes(mux, store, repositoryCatalog, pullRequestStore, changeSessionStore, authStore, activityStore, checkRunStore, previewStore)
	}
	if authStore != nil && repositoryCatalog != nil && proposalStore != nil && changeSessionStore != nil {
		registerTaskChangeSessionRoutes(mux, store, repositoryCatalog, proposalStore, pullRequestStore, changeSessionStore, authStore, relationshipStore, durableSchemaStore, capabilityStore, organizationStore)
	}
	if authStore != nil && repositoryCatalog != nil && activityStore != nil {
		registerActivityRoutes(mux, repositoryCatalog, activityStore, authStore)
		registerInboxRoutes(mux, repositoryCatalog, proposalStore, pullRequestStore, incidentStore, activityStore, authStore)
	}
	if authStore != nil && repositoryCatalog != nil && releaseStore != nil {
		registerReleaseRoutes(mux, store, repositoryCatalog, proposalStore, pullRequestStore, releaseStore, authStore, checkRunStore)
		if packageStore != nil && checkRunStore != nil {
			registerPackageRoutes(mux, store, repositoryCatalog, proposalStore, releaseStore, checkRunStore, deploymentStore, packageStore, authStore, activityStore)
		}
		if deploymentStore != nil {
			registerDeploymentRoutes(mux, store, repositoryCatalog, releaseStore, checkRunStore, deploymentStore, authStore, activityStore, pullRequestStore, changeSessionStore, packageStore)
		}
	}
	if authStore != nil && repositoryCatalog != nil && releaseStore != nil && deploymentStore != nil && relationshipStore != nil {
		registerRelationshipRoutes(mux, store, repositoryCatalog, releaseStore, deploymentStore, relationshipStore, authStore)
		registerEvolutionRoutes(mux, store, repositoryCatalog, proposalStore, pullRequestStore, releaseStore, deploymentStore, relationshipStore, authStore, checkRunStore)
	}
	if authStore != nil && repositoryCatalog != nil && incidentStore != nil {
		registerIncidentRoutes(mux, store, repositoryCatalog, incidentStore, proposalStore, deploymentStore, releaseStore, pullRequestStore, checkRunStore, authStore, activityStore)
	}
	if authStore != nil && repositoryCatalog != nil && issueStore != nil {
		registerIssueRoutes(mux, store, repositoryCatalog, issueStore, releaseStore, deploymentStore, incidentStore, proposalStore, pullRequestStore, packageStore, workspaceStore, authStore, activityStore, checkRunStore)
	}
	if authStore != nil && repositoryCatalog != nil && supportThreadStore != nil {
		registerSupportThreadRoutes(mux, store, repositoryCatalog, supportThreadStore, issueStore, proposalStore, documentationStore, authStore)
	}
	if authStore != nil && repositoryCatalog != nil && knowledgeAnswerStore != nil && supportThreadStore != nil && issueStore != nil && releaseStore != nil && packageStore != nil {
		registerKnowledgeAnswerRoutes(mux, store, repositoryCatalog, authStore, knowledgeAnswerStore, supportThreadStore, issueStore, releaseStore, packageStore)
	}
	if authStore != nil && repositoryCatalog != nil && supportThreadStore != nil && knowledgeAnswerStore != nil && workspaceStore != nil && supportVerificationStore != nil {
		registerSupportVerificationRoutes(mux, repositoryCatalog, supportThreadStore, knowledgeAnswerStore, workspaceStore, supportVerificationStore, authStore)
	}
	if authStore != nil && repositoryCatalog != nil && supportThreadStore != nil && knowledgeAnswerStore != nil && workspaceStore != nil && supportVerificationStore != nil && supportSolutionStore != nil {
		registerSupportSolutionRoutes(mux, repositoryCatalog, supportThreadStore, knowledgeAnswerStore, supportVerificationStore, workspaceStore, supportSolutionStore, releaseStore, packageStore, documentationStore, contributorPathwayStore, authStore)
	}
	if authStore != nil && repositoryCatalog != nil && contributorPathwayStore != nil {
		registerContributorPathwayRoutes(mux, store, repositoryCatalog, contributorPathwayStore, releaseStore, issueStore, proposalStore, workspaceStore, authStore)
	}
	if authStore != nil && repositoryCatalog != nil && learningPathwayStore != nil {
		registerLearningPathwayRoutes(mux, store, repositoryCatalog, learningPathwayStore, issueStore, proposalStore, packageStore, contributorPathwayStore, workspaceStore, organizationStore, authStore)
	}
	if authStore != nil && repositoryCatalog != nil && learningPathwayStore != nil && learningAssessmentStore != nil && workspaceStore != nil && checkRunStore != nil {
		registerLearningAssessmentRoutes(mux, store, repositoryCatalog, learningPathwayStore, learningAssessmentStore, workspaceStore, checkRunStore, authStore)
	}
	if authStore != nil && repositoryCatalog != nil && contributorOpportunityStore != nil {
		registerContributorOpportunityRoutes(mux, store, repositoryCatalog, contributorOpportunityStore, issueStore, proposalStore, pullRequestStore, releaseStore, learningAssessmentStore, learningPathwayStore, workspaceStore, authStore)
	}
	if authStore != nil && repositoryCatalog != nil && contributorOpportunityStore != nil && contributorPathwayStore != nil && workspaceStore != nil {
		registerContributorLaunchRoutes(mux, store, repositoryCatalog, contributorOpportunityStore, contributorPathwayStore, workspaceStore, issueStore, proposalStore, learningAssessmentStore, learningPathwayStore, authStore)
		registerContributorHelpRoutes(mux, workspaceStore, repositoryCatalog, contributorOpportunityStore, organizationStore, authStore)
		registerContributorPublicationRoutes(mux, store, repositoryCatalog, pullRequestStore, checkRunStore, contributorOpportunityStore, contributorPathwayStore, workspaceStore, authStore)
	}
	if authStore != nil && repositoryCatalog != nil && userStore != nil && securityAdvisoryStore != nil {
		registerSecurityAdvisoryRoutes(mux, store, repositoryCatalog, userStore, securityAdvisoryStore, releaseStore, checkRunStore, deploymentStore, authStore, activityStore)
	}
	if authStore != nil && repositoryCatalog != nil && userStore != nil && organizationStore != nil {
		registerOrganizationRoutes(mux, store, organizationStore, repositoryCatalog, userStore, authStore, activityStore, proposalStore, pullRequestStore, releaseStore, packageStore, incidentStore, relationshipStore, securityAdvisoryStore, agentEvaluationStore)
		if agentEvaluationStore != nil && store != nil {
			registerAgentEvaluationRoutes(mux, store, repositoryCatalog, authStore, organizationStore, agentEvaluationStore, issueStore, supportThreadStore, proposalStore, incidentStore, decisionStore, exploratorySessionStore)
		}
	}
	if authStore != nil && repositoryCatalog != nil && agentProjectStore != nil && store != nil {
		registerAgentProjectRoutes(mux, store, repositoryCatalog, authStore, agentProjectStore)
	}
	if authStore != nil && repositoryCatalog != nil && pullRequestStore != nil && agentProjectStore != nil && agentEvaluationStore != nil && agentCandidateStore != nil {
		registerAgentCandidateRoutes(mux, repositoryCatalog, authStore, pullRequestStore, agentProjectStore, agentEvaluationStore, agentCandidateStore)
		if agentPilotStore != nil {
			registerAgentPilotRoutes(mux, repositoryCatalog, authStore, pullRequestStore, agentCandidateStore, agentPilotStore)
		}
		if organizationStore != nil && agentPilotStore != nil && agentReleaseStore != nil {
			registerAgentReleaseRoutes(mux, repositoryCatalog, authStore, organizationStore, pullRequestStore, agentCandidateStore, agentPilotStore, agentReleaseStore)
		} else if agentReleaseStoreErr != nil || agentPilotStore == nil {
			registerAgentReleaseUnavailableRoutes(mux)
		}
	}
	if authStore != nil && repositoryCatalog != nil && charterStore != nil {
		registerCharterRoutes(mux, charterStore, governanceStore, repositoryCatalog, organizationStore, authStore)
		if governanceStore != nil {
			registerGovernanceRoutes(mux, store, governanceStore, charterStore, repositoryCatalog, organizationStore, proposalStore, authStore)
		}
	}
	if authStore != nil && repositoryCatalog != nil && workspaceStore != nil {
		registerWorkspaceRoutes(mux, store, repositoryCatalog, proposalStore, pullRequestStore, incidentStore, issueStore, releaseStore, workspaceStore, authStore, organizationStore, checkRunStore, changeSessionStore, supportThreadStore, knowledgeAnswerStore, debugWorkspaceStore)
		if pullRequestStore != nil {
			registerConflictWorkspaceRoutes(mux, store, repositoryCatalog, pullRequestStore, workspaceStore, authStore, organizationStore, checkRunStore)
		}
		if releaseStore != nil {
			releaseStore.ConfigureProvenanceReadiness(func(candidate releases.Candidate) (bool, error) {
				policies, err := provenancePolicyStore.List("repository", candidate.RepositoryID)
				if err != nil {
					return false, err
				}
				if len(policies) == 0 {
					return true, nil
				}
				values, err := provenanceAssessmentStore.List(candidate.RepositoryID, func(a provenanceassessments.Assessment) provenanceassessments.Current {
					return provenanceAssessmentCurrent(a, repositoryCatalog, provenanceGraphStore, provenancePolicyStore, pullRequestStore, changeStackStore, releaseStore, packageStore)
				})
				if err != nil {
					return false, err
				}
				for _, a := range values {
					if a.Candidate.Kind == "release_candidate" && a.Candidate.ID == candidate.ID && a.Candidate.Revision == candidate.CommitID && a.Ready {
						return true, nil
					}
				}
				return false, nil
			})
		}
	}
	if authStore != nil && repositoryCatalog != nil && explanationStore != nil {
		registerExplanationRoutes(mux, store, repositoryCatalog, authStore, explanationStore, proposalStore, pullRequestStore, incidentStore, workspaceStore, checkRunStore, relationshipStore)
	}
	if authStore != nil && repositoryCatalog != nil && impactStore != nil {
		registerImpactRoutes(mux, store, repositoryCatalog, authStore, impactStore, explanationStore, proposalStore, relationshipStore, releaseStore, packageStore, deploymentStore)
	}
	if authStore != nil && repositoryCatalog != nil && performanceGoalStore != nil {
		registerPerformanceGoalRoutes(mux, repositoryCatalog, authStore, performanceGoalStore)
		if performanceEvidenceStore != nil && store != nil {
			pullRequestStore.ConfigurePerformanceEvidence(performanceEvidenceStore)
			registerPerformanceEvidenceRoutes(mux, store, repositoryCatalog, authStore, performanceGoalStore, releaseStore, deploymentStore, pullRequestStore, performanceEvidenceStore)
		}
	}
	if authStore != nil && repositoryCatalog != nil && capacityObjectiveStore != nil {
		registerCapacityObjectiveRoutes(mux, repositoryCatalog, authStore, capacityObjectiveStore)
		if capacityModelStore != nil && releaseStore != nil {
			registerCapacityModelRoutes(mux, repositoryCatalog, authStore, capacityObjectiveStore, capacityModelStore, releaseStore)
		}
	}
	if authStore != nil && repositoryCatalog != nil && qualityPlanStore != nil {
		registerQualityPlanRoutes(mux, repositoryCatalog, authStore, qualityPlanStore)
	}
	if authStore != nil && repositoryCatalog != nil && collaborationWorkflowStore != nil && store != nil && agentProjectStore != nil {
		registerCollaborationWorkflowRoutes(mux, store, repositoryCatalog, authStore, collaborationWorkflowStore, workflowComponentStore, packageStore, federationStore, agentProjectStore, pullRequestStore, issueStore, activityStore)
	}
	if workflowComponentStore != nil && packageStore != nil && pullRequestStore != nil && authStore != nil && repositoryCatalog != nil {
		registerWorkflowComponentRoutes(mux, store, repositoryCatalog, authStore, pullRequestStore, packageStore, federationStore, workflowComponentStore)
	}
	if authStore != nil && repositoryCatalog != nil && assuranceProgramStore != nil {
		registerAssuranceProgramRoutes(mux, repositoryCatalog, authStore, assuranceProgramStore, assuranceScopeResources{git: store, dataFlows: dataFlowStore, infrastructure: infrastructureStore, environments: deploymentStore, releases: releaseStore})
		if assuranceEvidenceStore != nil {
			registerAssuranceEvidenceRoutes(mux, repositoryCatalog, authStore, assuranceProgramStore, assuranceEvidenceStore, assuranceEvidenceSources{pulls: pullRequestStore, checks: checkRunStore, releases: releaseStore, deployments: deploymentStore, incidents: incidentStore, privacy: privacyReviewStore, continuity: recoveryExerciseStore, access: protectionPlanStore, dependencies: packageStore, governance: governanceStore})
		}
	}
	if provenancePolicyStore == nil {
		root := os.Getenv("PROVENANCE_POLICY_STORAGE_ROOT")
		if root == "" {
			root = "provenance-policies"
		}
		provenancePolicyStore, _ = provenancepolicies.New(root)
	}
	if authStore != nil && repositoryCatalog != nil && provenancePolicyStore != nil {
		registerProvenancePolicyRoutes(mux, repositoryCatalog, organizationStore, authStore, provenancePolicyStore, contributorPathwayStore, agentProjectStore, packageStore, releaseStore)
	}
	if provenanceGraphStore == nil {
		root := os.Getenv("PROVENANCE_GRAPH_STORAGE_ROOT")
		if root == "" {
			root = "provenance-graphs"
		}
		provenanceGraphStore, _ = provenancegraphs.New(root)
	}
	if authStore != nil && repositoryCatalog != nil && store != nil && provenanceGraphStore != nil && provenancePolicyStore != nil {
		registerProvenanceGraphRoutes(mux, store, repositoryCatalog, authStore, provenanceGraphStore, provenancePolicyStore)
	}
	if provenanceAssessmentStore == nil {
		root := os.Getenv("PROVENANCE_ASSESSMENT_STORAGE_ROOT")
		if root == "" {
			root = "provenance-assessments"
		}
		provenanceAssessmentStore, _ = provenanceassessments.New(root)
	}
	if authStore != nil && repositoryCatalog != nil && provenanceAssessmentStore != nil && provenanceGraphStore != nil && provenancePolicyStore != nil {
		registerProvenanceAssessmentRoutes(mux, repositoryCatalog, authStore, provenanceAssessmentStore, provenanceGraphStore, provenancePolicyStore, pullRequestStore, changeStackStore, releaseStore, packageStore, proposalStore)
		if pullRequestStore != nil {
			pullRequestStore.ConfigureProvenanceReadiness(func(p pullrequests.PullRequest, _ []pullrequests.FileChange) (any, []pullrequests.ReadinessBlocker, error) {
				values, err := provenanceAssessmentStore.List(p.RepositoryID, func(a provenanceassessments.Assessment) provenanceassessments.Current {
					return provenanceAssessmentCurrent(a, repositoryCatalog, provenanceGraphStore, provenancePolicyStore, pullRequestStore, changeStackStore, releaseStore, packageStore)
				})
				if err != nil {
					return nil, nil, err
				}
				selected := []provenanceassessments.Assessment{}
				blockers := []pullrequests.ReadinessBlocker{}
				for _, a := range values {
					if a.Candidate.Kind == "pull_request" && a.Candidate.ID == p.ID && a.Candidate.Revision == p.SourceCommitID {
						selected = append(selected, a)
						if !a.Ready {
							blockers = append(blockers, pullrequests.ReadinessBlocker{Code: "provenance_evidence_required", Message: "current provenance assessment has unresolved or stale blocking findings"})
						}
					}
				}
				policies, policyErr := provenancePolicyStore.List("repository", p.RepositoryID)
				if policyErr != nil {
					return nil, nil, policyErr
				}
				if len(policies) > 0 && len(selected) == 0 {
					blockers = append(blockers, pullrequests.ReadinessBlocker{Code: "provenance_assessment_required", Message: "the repository provenance policy requires a current assessment for this exact pull revision"})
				}
				return selected, blockers, nil
			})
		}
	}
	if provenanceBundleStore == nil {
		root := os.Getenv("PROVENANCE_BUNDLE_STORAGE_ROOT")
		if root == "" {
			root = "provenance-bundles"
		}
		provenanceBundleStore, _ = provenancebundles.New(root)
	}
	if authStore != nil && repositoryCatalog != nil && provenanceBundleStore != nil && provenanceGraphStore != nil && provenanceAssessmentStore != nil && provenancePolicyStore != nil && releaseStore != nil && packageStore != nil {
		registerProvenanceBundleRoutes(mux, repositoryCatalog, authStore, provenanceBundleStore, provenanceGraphStore, provenanceAssessmentStore, provenancePolicyStore, releaseStore, packageStore, propagationCampaignStore)
	}
	if authStore != nil && repositoryCatalog != nil && userStore != nil && assuranceProgramStore != nil && assuranceEvidenceStore != nil && assuranceAssessmentStore != nil {
		registerAssuranceAssessmentRoutes(mux, store, repositoryCatalog, authStore, userStore, assuranceProgramStore, assuranceEvidenceStore, assuranceAssessmentStore, proposalStore, pullRequestStore, releaseStore)
	}
	if authStore != nil && repositoryCatalog != nil && assuranceProgramStore != nil && assuranceImpactStore != nil && pullRequestStore != nil {
		registerAssuranceImpactRoutes(mux, repositoryCatalog, authStore, assuranceImpactStore, assuranceProgramStore, pullRequestStore)
		pullRequestStore.ConfigureAssuranceImpact(func(p pullrequests.PullRequest, changes []pullrequests.FileChange) (any, []pullrequests.ReadinessBlocker, error) {
			values, err := assuranceImpactStore.List(p.RepositoryID, func(a assuranceimpact.Assessment) assuranceimpact.Current {
				program, e := assuranceProgramStore.Get(a.ProgramID)
				if e != nil || len(program.Revisions) == 0 {
					return assuranceimpact.Current{CandidateRevision: p.SourceCommitID}
				}
				return assuranceImpactCurrent(p.SourceCommitID, program.Revisions[len(program.Revisions)-1], changes, assuranceImpactParticipants(repositoryCatalog, p.RepositoryID, program.Revisions[len(program.Revisions)-1]))
			})
			if err != nil {
				return nil, nil, err
			}
			selected := []assuranceimpact.Assessment{}
			latest := map[string]assuranceimpact.Assessment{}
			ready := true
			for _, v := range values {
				if v.Candidate.Kind == "pull_request" && v.Candidate.ID == p.ID {
					if prior, ok := latest[v.ProgramID]; !ok || newerAssuranceAssessment(v, prior) {
						latest[v.ProgramID] = v
					}
				}
			}
			programs, programErr := assuranceProgramStore.List(p.RepositoryID)
			if programErr != nil {
				return nil, nil, programErr
			}
			for _, program := range programs {
				if v, ok := latest[program.ID]; ok {
					selected = append(selected, v)
					if !v.Ready {
						ready = false
					}
				} else {
					ready = false
				}
			}
			if len(programs) == 0 {
				return nil, nil, nil
			}
			if len(selected) == 0 {
				return selected, []pullrequests.ReadinessBlocker{{Code: "assurance_impact_missing", Message: "a current assurance program requires candidate compliance impact analysis"}}, nil
			}
			if !ready {
				return selected, []pullrequests.ReadinessBlocker{{Code: "assurance_impact_incomplete", Message: "current affected controls require applicability decisions and owner acknowledgement"}}, nil
			}
			return selected, nil, nil
		})
	}
	if authStore != nil && repositoryCatalog != nil && securityExpectationStore != nil {
		registerSecurityExpectationRoutes(mux, repositoryCatalog, authStore, securityExpectationStore)
	}
	if authStore != nil && repositoryCatalog != nil && threatModelStore != nil && designProposalStore != nil && pullRequestStore != nil && apiContractStore != nil && durableSchemaStore != nil && infrastructureStore != nil && productExperimentStore != nil {
		registerThreatModelRoutes(mux, repositoryCatalog, authStore, threatModelStore, threatModelSources{designProposalStore, pullRequestStore, apiContractStore, durableSchemaStore, infrastructureStore, productExperimentStore})
	}
	if authStore != nil && repositoryCatalog != nil && store != nil && securityScenarioStore != nil && threatModelStore != nil && workspaceStore != nil && previewStore != nil && checkRunStore != nil {
		registerSecurityScenarioRoutes(mux, store, repositoryCatalog, authStore, securityScenarioStore, threatModelStore, workspaceStore, previewStore, checkRunStore)
	}
	if authStore != nil && repositoryCatalog != nil && securityFindingStore != nil && threatModelStore != nil && securityScenarioStore != nil && proposalStore != nil && pullRequestStore != nil {
		registerSecurityFindingRoutes(mux, repositoryCatalog, authStore, securityFindingStore, threatModelStore, securityScenarioStore, proposalStore, pullRequestStore)
	}
	if authStore != nil && repositoryCatalog != nil && securityConfidenceStore != nil && pullRequestStore != nil && releaseStore != nil && deploymentStore != nil && threatModelStore != nil && securityScenarioStore != nil && securityFindingStore != nil && issueStore != nil && proposalStore != nil && incidentStore != nil && securityAdvisoryStore != nil {
		registerSecurityConfidenceRoutes(mux, store, repositoryCatalog, organizationStore, authStore, securityConfidenceStore, pullRequestStore, releaseStore, deploymentStore, threatModelStore, securityScenarioStore, securityFindingStore, issueStore, proposalStore, incidentStore, securityAdvisoryStore)
		pullRequestStore.ConfigureSecurityConfidence(func(p pullrequests.PullRequest, changes []pullrequests.FileChange) (any, []pullrequests.ReadinessBlocker, error) {
			paths := []string{}
			for _, change := range changes {
				paths = append(paths, change.Path)
			}
			matrix, matrixErr := securityMatrix(securityConfidenceStore, store, repositoryCatalog, threatModelStore, securityScenarioStore, securityFindingStore, p.RepositoryID, "", "pull", p.ID, p.SourceCommitID, p.TargetBranch, paths)
			if errors.Is(matrixErr, securityconfidence.ErrNotFound) {
				return nil, nil, nil
			}
			if matrixErr != nil {
				return nil, nil, matrixErr
			}
			blockers := []pullrequests.ReadinessBlocker{}
			if !matrix.Ready {
				blockers = append(blockers, pullrequests.ReadinessBlocker{Code: "security_confidence_incomplete", Message: "current threat coverage, scenario evidence, owner acknowledgements, or finding resolution is incomplete"})
			}
			return matrix, blockers, nil
		})
	}
	if authStore != nil && repositoryCatalog != nil && store != nil && testScenarioStore != nil && qualityPlanStore != nil {
		registerTestScenarioRoutes(mux, store, repositoryCatalog, authStore, testScenarioStore, qualityPlanStore, pullRequestStore, workspaceStore, testScenarioSources{issues: issueStore, reproductions: debugWorkspaceStore, designs: designProposalStore, contracts: apiContractStore, documentation: documentationStore})
	}
	if authStore != nil && repositoryCatalog != nil && store != nil && exploratorySessionStore != nil && pullRequestStore != nil && releaseStore != nil && issueStore != nil && qualityPlanStore != nil {
		registerExploratorySessionRoutes(mux, store, repositoryCatalog, authStore, exploratorySessionStore, pullRequestStore, releaseStore, issueStore, qualityPlanStore, proposalStore, testScenarioStore)
	}
	if authStore != nil && repositoryCatalog != nil && releaseConfidenceStore != nil && pullRequestStore != nil && releaseStore != nil && testScenarioStore != nil && exploratorySessionStore != nil && checkRunStore != nil {
		registerReleaseConfidenceRoutes(mux, repositoryCatalog, authStore, releaseConfidenceStore, pullRequestStore, releaseStore, testScenarioStore, exploratorySessionStore, checkRunStore, issueStore, proposalStore)
		pullRequestStore.ConfigureQualityConfidence(func(p pullrequests.PullRequest, changes []pullrequests.FileChange) (any, []pullrequests.ReadinessBlocker, error) {
			paths := []string{}
			for _, change := range changes {
				paths = append(paths, change.Path)
			}
			matrix, matrixErr := releaseConfidenceStore.Matrix(p.RepositoryID, releaseconfidence.Target{Kind: "pull", ID: p.ID, Revision: p.SourceCommitID, Branch: p.TargetBranch, ChangedPaths: paths})
			if errors.Is(matrixErr, releaseconfidence.ErrNotFound) {
				return nil, nil, nil
			}
			if matrixErr != nil {
				return nil, nil, matrixErr
			}
			blockers := []pullrequests.ReadinessBlocker{}
			if !matrix.Ready {
				blockers = append(blockers, pullrequests.ReadinessBlocker{Code: "quality_confidence_incomplete", Message: "current quality requirements retain failures, flakes, quarantines, or gaps"})
			}
			return matrix, blockers, nil
		})
	}
	if authStore != nil && repositoryCatalog != nil && accessibilityCommitmentStore != nil {
		registerAccessibilityCommitmentRoutes(mux, repositoryCatalog, authStore, accessibilityCommitmentStore)
	}
	if authStore != nil && repositoryCatalog != nil && localePlanStore != nil && store != nil {
		registerLocalePlanRoutes(mux, store, repositoryCatalog, authStore, localePlanStore)
	}
	if authStore != nil && repositoryCatalog != nil && serviceObjectiveStore != nil {
		if pullRequestStore != nil {
			pullRequestStore.ConfigureReliability(serviceObjectiveStore)
		}
		serviceObjectiveStore.ConfigureInvestigationProvenance(func(contract serviceobjectives.Contract, in serviceobjectives.Investigation) bool {
			return reliabilityInvestigationProvenanceResolves(store, pullRequestStore, deploymentStore, releaseStore, contract, in)
		})
		registerServiceObjectiveRoutes(mux, store, repositoryCatalog, authStore, serviceObjectiveStore, pullRequestStore, deploymentStore, releaseStore, proposalStore)
	}
	if authStore != nil && repositoryCatalog != nil && recoveryCommitmentStore != nil {
		registerRecoveryCommitmentRoutes(mux, repositoryCatalog, authStore, recoveryCommitmentStore, serviceObjectiveStore, deploymentStore, incidentStore, dataCommitmentStore, governanceStore)
	}
	if authStore != nil && repositoryCatalog != nil && interfaceSystemStore != nil && store != nil && releaseStore != nil {
		registerInterfaceSystemRoutes(mux, store, repositoryCatalog, authStore, interfaceSystemStore, releaseStore)
	}
	if authStore != nil && repositoryCatalog != nil && capabilityStore != nil && store != nil && releaseStore != nil {
		registerCapabilityRoutes(mux, store, repositoryCatalog, authStore, capabilityStore, releaseStore, proposalStore, pullRequestStore, changeSessionStore, workspaceStore)
	}
	if authStore != nil && repositoryCatalog != nil && designProposalStore != nil {
		registerDesignProposalRoutes(mux, store, repositoryCatalog, authStore, designProposalStore, issueStore, feedbackStore, roadmapStore, accessibilityAssessmentStore, pullRequestStore, proposalStore)
	}
	if authStore != nil && repositoryCatalog != nil && pullRequestStore != nil && previewStore != nil && checkRunStore != nil && designProposalStore != nil && interfaceCheckStore != nil {
		registerInterfaceCheckRoutes(mux, store, repositoryCatalog, authStore, pullRequestStore, previewStore, checkRunStore, designProposalStore, interfaceCheckStore)
	}
	if authStore != nil && repositoryCatalog != nil && organizationStore != nil && designGovernanceStore != nil && pullRequestStore != nil && releaseStore != nil && interfaceCheckStore != nil && interfaceSystemStore != nil && proposalStore != nil {
		registerDesignGovernanceRoutes(mux, repositoryCatalog, organizationStore, authStore, designGovernanceStore, pullRequestStore, releaseStore, interfaceCheckStore, interfaceSystemStore, proposalStore)
	}
	if authStore != nil && repositoryCatalog != nil && durableSchemaStore != nil && pullRequestStore != nil {
		registerDurableSchemaRoutes(mux, store, repositoryCatalog, authStore, durableSchemaStore, pullRequestStore, decisionStore, proposalStore, changeSessionStore, workspaceStore, deploymentStore, releaseStore)
	}
	if authStore != nil && repositoryCatalog != nil && infrastructureStore != nil && store != nil {
		registerInfrastructureRoutes(mux, store, repositoryCatalog, authStore, infrastructureStore, pullRequestStore, releaseStore, deploymentStore, workspaceStore, issueStore, proposalStore, incidentStore)
	}
	if authStore != nil && repositoryCatalog != nil && debugWorkspaceStore != nil && releaseStore != nil && deploymentStore != nil && issueStore != nil && incidentStore != nil && supportThreadStore != nil && serviceObjectiveStore != nil && packageStore != nil && infrastructureStore != nil {
		registerDebugWorkspaceRoutes(mux, store, repositoryCatalog, authStore, debugWorkspaceStore, releaseStore, deploymentStore, issueStore, incidentStore, supportThreadStore, serviceObjectiveStore, packageStore, infrastructureStore, workspaceStore, proposalStore, pullRequestStore, checkRunStore)
	}
	if authStore != nil && repositoryCatalog != nil && regressionInvestigationStore != nil && issueStore != nil && supportThreadStore != nil && checkRunStore != nil && releaseStore != nil && deploymentStore != nil && debugWorkspaceStore != nil && proposalStore != nil && qualityPlanStore != nil {
		registerRegressionInvestigationRoutes(mux, store, repositoryCatalog, authStore, regressionInvestigationStore, issueStore, supportThreadStore, checkRunStore, releaseStore, deploymentStore, debugWorkspaceStore, pullRequestStore, proposalStore, qualityPlanStore)
	}
	if authStore != nil && repositoryCatalog != nil && propagationCampaignStore != nil && pullRequestStore != nil && proposalStore != nil && checkRunStore != nil {
		registerPropagationCampaignRoutes(mux, store, repositoryCatalog, authStore, propagationCampaignStore, pullRequestStore, proposalStore, checkRunStore, releaseStore, deploymentStore)
	}
	if authStore != nil && repositoryCatalog != nil && historyRemediationStore != nil && store != nil && securityFindingStore != nil && incidentStore != nil && supportThreadStore != nil && releaseStore != nil && packageStore != nil && deploymentStore != nil && checkRunStore != nil {
		registerHistoryRemediationRoutes(mux, store, repositoryCatalog, authStore, historyRemediationStore, securityFindingStore, incidentStore, supportThreadStore, releaseStore, packageStore, deploymentStore, checkRunStore)
		registerHistoryRewriteRoutes(mux, store, repositoryCatalog, authStore, historyRemediationStore)
		registerHistoryContainmentRoutes(mux, store, repositoryCatalog, authStore, historyRemediationStore)
	}
	if authStore != nil && repositoryCatalog != nil && organizationStore != nil && restructuringPlanStore != nil && store != nil && pullRequestStore != nil && issueStore != nil && proposalStore != nil && releaseStore != nil && packageStore != nil && documentationStore != nil && governanceStore != nil && workspaceStore != nil && collaborationWorkflowStore != nil && relationshipStore != nil && federationStore != nil {
		registerRestructuringPlanRoutes(mux, store, repositoryCatalog, authStore, organizationStore, restructuringPlanStore, pullRequestStore, issueStore, proposalStore, releaseStore, packageStore, documentationStore, governanceStore, workspaceStore, collaborationWorkflowStore, relationshipStore, federationStore)
	}
	if authStore != nil && repositoryCatalog != nil && organizationStore != nil && store != nil && pullRequestStore != nil && changeStackStore != nil {
		registerChangeStackRoutes(mux, store, repositoryCatalog, organizationStore, authStore, changeStackStore, pullRequestStore, checkRunStore, previewStore)
	}
	if authStore != nil && userStore != nil && repositoryCatalog != nil && apiContractStore != nil && pullRequestStore != nil && releaseStore != nil {
		registerAPIContractRoutes(mux, store, repositoryCatalog, authStore, apiContractStore, pullRequestStore, releaseStore)
		registerAPIContractApplicationRoutes(mux, store, repositoryCatalog, authStore, apiContractStore, userStore, pullRequestStore, releaseStore, issueStore, proposalStore)
		if relationshipStore != nil {
			registerAPIContractMigrationRoutes(mux, repositoryCatalog, authStore, apiContractStore, relationshipStore)
		}
	}
	if authStore != nil && repositoryCatalog != nil && recoveryCommitmentStore != nil && protectionPlanStore != nil && store != nil {
		registerProtectionPlanRoutes(mux, store, repositoryCatalog, authStore, protectionPlanStore, recoveryCommitmentStore, deploymentStore)
	}
	if authStore != nil && repositoryCatalog != nil && store != nil && recoveryCommitmentStore != nil && protectionPlanStore != nil && recoveryExerciseStore != nil {
		registerRecoveryExerciseRoutes(mux, store, deploymentStore, releaseStore, repositoryCatalog, authStore, protectionPlanStore, recoveryCommitmentStore, recoveryExerciseStore, proposalStore)
	}
	if authStore != nil && repositoryCatalog != nil && incidentStore != nil && protectionPlanStore != nil && recoveryOperationStore != nil {
		registerRecoveryOperationRoutes(mux, repositoryCatalog, authStore, incidentStore, protectionPlanStore, recoveryOperationStore)
	}
	if authStore != nil && repositoryCatalog != nil && pullRequestStore != nil && localizationStore != nil {
		if localePlanStore != nil {
			localizationStore.ConfigureLocalePlanVersions(func(repositoryID string, planIDs []string) (map[string]int, error) {
				versions := map[string]int{}
				for _, planID := range planIDs {
					plan, planErr := localePlanStore.Get(planID, "")
					if errors.Is(planErr, localeplans.ErrNotFound) {
						continue
					}
					if planErr != nil {
						return nil, planErr
					}
					if plan.RepositoryID == repositoryID {
						versions[planID] = plan.CurrentVersion
					}
				}
				return versions, nil
			})
		}
		pullRequestStore.ConfigureLocalization(localizationStore)
		registerLocalizationRoutes(mux, repositoryCatalog, authStore, pullRequestStore, localePlanStore, previewStore, releaseStore, checkRunStore, localizationStore)
	}
	if authStore != nil && repositoryCatalog != nil && dataCommitmentStore != nil {
		registerDataCommitmentRoutes(mux, repositoryCatalog, authStore, dataCommitmentStore, releaseStore, extensionStore, productExperimentStore, deploymentStore)
		if dataFlowStore != nil && store != nil {
			registerDataFlowRoutes(mux, store, repositoryCatalog, authStore, dataCommitmentStore, dataFlowStore)
			if pullRequestStore != nil && privacyReviewStore != nil {
				registerPrivacyReviewRoutes(mux, store, repositoryCatalog, authStore, pullRequestStore, dataCommitmentStore, dataFlowStore, privacyReviewStore)
				if previewStore != nil && releaseStore != nil && privacyCheckStore != nil {
					pullRequestStore.ConfigurePrivacyChecks(privacyCheckStore)
					registerPrivacyCheckRoutes(mux, repositoryCatalog, authStore, pullRequestStore, releaseStore, previewStore, dataFlowStore, privacyCheckStore)
				}
			}
		}
	}
	if authStore != nil && repositoryCatalog != nil && dataObservationStore != nil && dataCommitmentStore != nil && dataFlowStore != nil && releaseStore != nil && deploymentStore != nil && extensionStore != nil && proposalStore != nil && store != nil {
		registerDataObservationRoutes(mux, store, repositoryCatalog, authStore, dataCommitmentStore, dataFlowStore, releaseStore, deploymentStore, extensionStore, organizationStore, proposalStore, dataObservationStore)
	}
	if authStore != nil && repositoryCatalog != nil && accessibilityReportStore != nil {
		registerAccessibilityReportRoutes(mux, repositoryCatalog, authStore, accessibilityReportStore)
	}
	if authStore != nil && repositoryCatalog != nil && accessibilityAssessmentStore != nil {
		registerAccessibilityAssessmentRoutes(mux, store, repositoryCatalog, authStore, pullRequestStore, previewStore, accessibilityReportStore, accessibilityCommitmentStore, proposalStore, accessibilityAssessmentStore)
	}
	if authStore != nil && repositoryCatalog != nil && accessibilityDeliveryStore != nil && accessibilityAssessmentStore != nil && pullRequestStore != nil && releaseStore != nil {
		pullRequestStore.ConfigureAccessibilityDelivery(accessibilityDeliveryStore, accessibilityAssessmentStore)
		registerAccessibilityDeliveryRoutes(mux, repositoryCatalog, authStore, pullRequestStore, releaseStore, previewStore, checkRunStore, accessibilityAssessmentStore, accessibilityDeliveryStore)
	}
	if authStore != nil && repositoryCatalog != nil && productExperimentStore != nil {
		registerProductExperimentRoutes(mux, repositoryCatalog, authStore, productExperimentStore, proposalStore, pullRequestStore, checkRunStore, releaseStore, deploymentStore, organizationStore)
	}
	if authStore != nil && repositoryCatalog != nil && feedbackStore != nil {
		registerFeedbackRoutes(mux, repositoryCatalog, authStore, feedbackStore, releaseStore, documentationStore, previewStore, issueStore, productExperimentStore)
	}
	if authStore != nil && repositoryCatalog != nil && productOpportunityStore != nil && feedbackStore != nil && issueStore != nil && previewStore != nil && productExperimentStore != nil {
		registerProductOpportunityRoutes(mux, repositoryCatalog, authStore, productOpportunityStore, feedbackStore, issueStore, previewStore, productExperimentStore)
	}
	if authStore != nil && repositoryCatalog != nil && roadmapStore != nil && productOpportunityStore != nil {
		registerRoadmapRoutes(mux, store, repositoryCatalog, authStore, roadmapStore, productOpportunityStore, feedbackStore, proposalStore, pullRequestStore, checkRunStore, releaseStore, deploymentStore)
	}
	if authStore != nil && repositoryCatalog != nil && roadmapStore != nil && productOpportunityStore != nil && outcomeValidationStore != nil {
		registerOutcomeValidationRoutes(mux, repositoryCatalog, authStore, outcomeValidationStore, roadmapStore, productOpportunityStore)
	}
	if authStore != nil && repositoryCatalog != nil && projectFundStore != nil {
		registerProjectFundRoutes(mux, repositoryCatalog, authStore, projectFundStore)
		registerOutcomeFundingRoutes(mux, repositoryCatalog, authStore, projectFundStore, organizationStore)
	}
	if authStore != nil && repositoryCatalog != nil && decisionStore != nil {
		registerDecisionRoutes(mux, store, repositoryCatalog, authStore, userStore, decisionStore, activityStore, proposalStore, explanationStore, incidentStore, relationshipStore, organizationStore, workspaceStore, pullRequestStore, checkRunStore, releaseStore, deploymentStore)
	}
	if authStore != nil && repositoryCatalog != nil && userStore != nil && deliveryTeamStore != nil {
		registerDeliveryTeamRoutes(mux, store, repositoryCatalog, authStore, userStore, deliveryTeamStore, proposalStore, decisionStore, incidentStore, organizationStore, activityStore, changeSessionStore, workspaceStore, explanationStore, pullRequestStore, checkRunStore)
	}
	mux.HandleFunc("GET /git/{remote}/info/refs", func(w http.ResponseWriter, r *http.Request) {
		service := r.URL.Query().Get("service")
		if service != uploadPackService && service != receivePackService {
			http.Error(w, "unsupported Git service", http.StatusBadRequest)
			return
		}
		required := "git:read"
		if service == receivePackService {
			required = "git:write"
			if blocked, guidance := historyRewritePushPaused(historyRemediationStore, r.PathValue("remote")); blocked {
				http.Error(w, guidance, http.StatusConflict)
				return
			}
		}
		onlyBranch := ""
		if authStore != nil {
			credential, _, ok := authorizeGitRepository(w, r, authStore, repositoryCatalog, pullRequestStore, r.PathValue("remote"), required)
			if !ok {
				return
			}
			onlyBranch = credential.GitWriteBranch
			if service == receivePackService && credential.GitWriteBranch != "" && !strings.HasPrefix(credential.GitWriteBranch, "refs/heads/vivarium-security/") && !activeRunCredential(changeSessionStore, r.PathValue("remote"), credential.ID) && !activeMaintainerCredential(pullRequestStore, repositoryCatalog, r.PathValue("remote"), credential) {
				writeAPIError(w, 401, "invalid_credential", "credential is not active")
				return
			}
		}
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-"+service+"-advertisement")
		setGitCacheHeaders(w)
		if _, err := io.WriteString(w, pktLine("# service="+service+"\n")+"0000"); err != nil {
			return
		}
		runGitService(w, r, repo, service, true, false, onlyBranch)
	})
	mux.HandleFunc("POST /git/{remote}/git-upload-pack", func(w http.ResponseWriter, r *http.Request) {
		onlyBranch := ""
		if authStore != nil {
			credential, _, ok := authorizeGitRepository(w, r, authStore, repositoryCatalog, pullRequestStore, r.PathValue("remote"), "git:read")
			if !ok {
				return
			}
			onlyBranch = credential.GitWriteBranch
		}
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
		setGitCacheHeaders(w)
		runGitService(w, r, repo, uploadPackService, false, false, onlyBranch)
	})
	mux.HandleFunc("POST /git/{remote}/git-receive-pack", func(w http.ResponseWriter, r *http.Request) {
		if blocked, guidance := historyRewritePushPaused(historyRemediationStore, r.PathValue("remote")); blocked {
			http.Error(w, guidance, http.StatusConflict)
			return
		}
		if blocked, guidance := restructuringPushPaused(restructuringPlanStore, r.PathValue("remote")); blocked {
			http.Error(w, guidance, http.StatusConflict)
			return
		}
		contributor := false
		onlyBranch := ""
		if authStore != nil {
			credential, owner, ok := authorizeGitRepository(w, r, authStore, repositoryCatalog, pullRequestStore, r.PathValue("remote"), "git:write")
			if !ok {
				return
			}
			contributor = !owner
			onlyBranch = credential.GitWriteBranch
			if onlyBranch != "" && !strings.HasPrefix(onlyBranch, "refs/heads/vivarium-security/") && !activeRunCredential(changeSessionStore, r.PathValue("remote"), credential.ID) && !activeMaintainerCredential(pullRequestStore, repositoryCatalog, r.PathValue("remote"), credential) {
				writeAPIError(w, 401, "invalid_credential", "credential is not active")
				return
			}
		}
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
		setGitCacheHeaders(w)
		runGitService(w, r, repo, receivePackService, false, contributor, onlyBranch)
	})
	return mux
}

func restructuringPushPaused(store *restructuringplans.Store, remote string) (bool, string) {
	if store == nil {
		return false, ""
	}
	repositoryID, ok := strings.CutSuffix(remote, ".git")
	if !ok {
		return false, ""
	}
	items, err := store.List(repositoryID)
	if err != nil {
		return true, "push paused: restructuring state is unavailable"
	}
	for _, p := range items {
		if p.Cutover != nil && p.Cutover.SourceState == "writes_paused" {
			return true, "push rejected: repository authority moved during restructuring; use the retained destination mapping or ask the cutover controller to roll back"
		}
	}
	return false, ""
}

func historyRewritePushPaused(store *historyremediations.Store, remote string) (bool, string) {
	if store == nil {
		return false, ""
	}
	repositoryID, ok := strings.CutSuffix(remote, ".git")
	if !ok {
		return false, ""
	}
	items, err := store.List(repositoryID)
	if err != nil {
		return true, "push paused: remediation state is unavailable; retry after a maintainer confirms migration"
	}
	for _, item := range items {
		if item.Publication == nil {
			continue
		}
		for _, system := range item.Publication.PausedSystems {
			if system == "pushes" {
				return true, "push rejected: authoritative history was rewritten; fetch the replacement refs, create a backup of local work, then rebase or reset using the remediation migration mapping before pushing"
			}
		}
	}
	return false, ""
}

func newerAssuranceAssessment(candidate, current assuranceimpact.Assessment) bool {
	return candidate.ProgramVersion > current.ProgramVersion || candidate.ProgramVersion == current.ProgramVersion && (candidate.CreatedAt.After(current.CreatedAt) || candidate.CreatedAt.Equal(current.CreatedAt) && candidate.ID > current.ID)
}

func activeMaintainerCredential(pulls *pullrequests.Store, catalog *repositories.Store, remote string, credential auth.Credential) bool {
	repositoryID, ok := strings.CutSuffix(remote, ".git")
	if !ok || pulls == nil || catalog == nil {
		return false
	}
	return pulls.AllowsMaintainerEdit(repositoryID, credential.GitWriteBranch, credential.PullRequestID, credential.UserID, func(targetID, userID string) bool {
		target, err := catalog.GetByID(targetID)
		if err != nil {
			return false
		}
		if target.OwnerID == userID {
			return true
		}
		allowed, err := catalog.HasCollaborator(userID, targetID)
		return err == nil && allowed
	})
}

func activeRunCredential(store *changesessions.Store, remote, credentialID string) bool {
	if store == nil {
		return false
	}
	repositoryID, ok := strings.CutSuffix(remote, ".git")
	if !ok {
		return false
	}
	allowed, err := store.AllowsGitWrite(repositoryID, credentialID)
	return err == nil && allowed
}

type userInput struct {
	Handle      *string `json:"handle"`
	DisplayName *string `json:"display_name"`
}

func registerUserRoutes(mux *http.ServeMux, store *users.Store, authStore *auth.Store) {
	if authStore != nil {
		mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authenticateRequest(w, r, authStore, "", false)
			if !ok {
				return
			}
			user, err := store.Get(actor.UserID)
			if writeUserError(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, user)
		})
	}
	mux.HandleFunc("POST /users", func(w http.ResponseWriter, r *http.Request) {
		var input userInput
		if err := decodeJSON(r, &input); err != nil || input.Handle == nil || input.DisplayName == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "handle and display_name are required")
			return
		}
		var issued auth.IssuedCredential
		user, err := store.CreateWithBootstrap(*input.Handle, *input.DisplayName, func(user users.User) error {
			if authStore == nil {
				return nil
			}
			var issueErr error
			issued, issueErr = authStore.Issue(user.ID, auth.Session, "web session", []string{"credentials:write", "profile:write", "repositories:read", "repositories:write"}, 24*time.Hour)
			return issueErr
		})
		if err != nil && issued.ID != "" {
			if _, revokeErr := authStore.Revoke(issued.UserID, issued.ID); revokeErr != nil {
				log.Printf("revoke credential %s after user bootstrap failure: %v", issued.ID, revokeErr)
			}
		}
		if writeUserError(w, err) {
			return
		}
		w.Header().Set("Location", "/users/"+user.ID)
		if authStore == nil {
			writeJSON(w, http.StatusCreated, user)
			return
		}
		setSessionCookie(w, issued.Token, issued.ExpiresAt)
		writeJSON(w, http.StatusCreated, map[string]any{"user": user, "credential": issued})
	})
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		user, err := store.Get(r.PathValue("id"))
		if writeUserError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, user)
	})
	mux.HandleFunc("PATCH /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		if authStore != nil {
			credential, ok := authenticateRequest(w, r, authStore, "profile:write", false)
			if !ok {
				return
			}
			if credential.UserID != r.PathValue("id") {
				writeAPIError(w, http.StatusForbidden, "forbidden", "credential belongs to another user")
				return
			}
		}
		var input userInput
		if err := decodeJSON(r, &input); err != nil || (input.Handle == nil && input.DisplayName == nil) {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "at least one of handle or display_name is required")
			return
		}
		user, err := store.Patch(r.PathValue("id"), users.ProfilePatch{
			Handle: input.Handle, DisplayName: input.DisplayName,
		})
		if writeUserError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, user)
	})
}

type repositoryInput struct {
	Name *string `json:"name"`
}

type forkInput struct {
	Name *string `json:"name"`
}

type forkSyncInput struct {
	Branch *string `json:"branch"`
}

type repositoryPatch struct {
	Visibility *string `json:"visibility"`
}

type collaboratorInput struct {
	UserID *string `json:"user_id"`
}

type requiredChecksInput struct {
	Checks []string `json:"checks"`
}

type integrationQueuePolicyInput struct {
	Enabled         *bool   `json:"enabled"`
	Concurrency     *int    `json:"concurrency"`
	FailureBehavior *string `json:"failure_behavior"`
}

type proposalInput struct {
	Title *string `json:"title"`
	Body  *string `json:"body"`
}

type proposalPatch struct {
	Title  *string `json:"title"`
	Body   *string `json:"body"`
	Status *string `json:"status"`
}

type commentInput struct {
	Body *string `json:"body"`
}

type proposalTaskInput struct {
	Title                *string  `json:"title"`
	Outcome              *string  `json:"outcome"`
	DependencyIDs        []string `json:"dependency_ids"`
	DiscussionCommentIDs []string `json:"discussion_comment_ids"`
}

type proposalTaskPatch struct {
	Title                *string   `json:"title"`
	Outcome              *string   `json:"outcome"`
	Status               *string   `json:"status"`
	Position             *int      `json:"position"`
	DependencyIDs        *[]string `json:"dependency_ids"`
	DiscussionCommentIDs *[]string `json:"discussion_comment_ids"`
}

type proposalTaskAssignmentInput struct {
	AssigneeType         string  `json:"assignee_type"`
	AssigneeID           *string `json:"assignee_id"`
	Mandate              string  `json:"mandate"`
	RepositoryID         string  `json:"repository_id"`
	BaseRevision         string  `json:"base_revision"`
	ExpectedAssignmentID string  `json:"expected_assignment_id"`
}

type proposalTaskRebaseInput struct {
	BaseRevision         string `json:"base_revision"`
	ExpectedAssignmentID string `json:"expected_assignment_id"`
}

type pullRequestInput struct {
	Title              *string `json:"title"`
	Body               *string `json:"body"`
	SourceRepositoryID *string `json:"source_repository_id"`
	SourceBranch       *string `json:"source_branch"`
	TargetBranch       *string `json:"target_branch"`
	ProposalID         *string `json:"proposal_id"`
}

type pullRequestPolicyInput struct {
	MaintainerEditsAllowed *bool `json:"maintainer_edits_allowed"`
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type reviewInput struct {
	Decision *string `json:"decision"`
}

func startCheckRuns(gitStore *storage.Store, runStore *checkruns.Store, pull pullrequests.PullRequest, requiredDocumentation ...string) error {
	return startCheckRunsWithRequiredDocumentation(gitStore, runStore, pull.RepositoryID, pull.ID, pull.SourceCommitID, nil, requiredDocumentation)
}

func startCheckRunsForCommit(gitStore *storage.Store, runStore *checkruns.Store, repositoryID, pullRequestID, commitID string, required []string) error {
	return startCheckRunsWithRequiredDocumentation(gitStore, runStore, repositoryID, pullRequestID, commitID, required, required)
}

func startCheckRunsWithRequiredDocumentation(gitStore *storage.Store, runStore *checkruns.Store, repositoryID, pullRequestID, commitID string, requiredOnly, requiredDocumentation []string) error {
	if gitStore == nil || runStore == nil {
		return errors.New("check run storage is unavailable")
	}
	if requiredOnly != nil && len(requiredOnly) == 0 {
		return nil
	}
	repository, err := gitStore.Open(repositoryID)
	if err != nil {
		return fmt.Errorf("open repository for checks: %w", err)
	}
	command := exec.Command("git", "--git-dir="+repository.Path(), "show", commitID+":"+checkruns.ConfigPath)
	data, err := command.Output()
	definitions := []checkruns.Definition{}
	if err != nil {
		// A repository opts in by versioning the configuration at the candidate commit.
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 128 {
			data = nil
		} else {
			return fmt.Errorf("read check configuration: %w", err)
		}
	}
	config := checkruns.Config{}
	if data != nil {
		config, err = checkruns.ParseConfig(data)
	}
	if data != nil && err != nil {
		log.Printf("invalid check configuration for %s: %v", commitID, err)
		runs, createErr := runStore.Create(repositoryID, pullRequestID, commitID, []checkruns.Definition{{Name: "configuration", Image: "invalid", Command: "invalid configuration", TimeoutSeconds: 1, WorkingDirectory: "."}})
		if createErr != nil {
			return fmt.Errorf("create invalid-configuration check run: %w", createErr)
		}
		if len(runs) == 1 {
			if recordErr := runStore.RecordFailure(runs[0], err.Error()); recordErr != nil {
				return fmt.Errorf("record invalid check configuration: %w", recordErr)
			}
		}
		return nil
	}
	definitions = append(definitions, config.Checks...)
	docsCommand := exec.Command("git", "--git-dir="+repository.Path(), "show", commitID+":"+checkruns.DocumentationConfigPath)
	if docsData, docsErr := docsCommand.Output(); docsErr == nil {
		_, docsDefinitions, parseErr := checkruns.ParseDocumentationConfig(docsData, commitID, func(name string) ([]byte, error) {
			return exec.Command("git", "--git-dir="+repository.Path(), "show", commitID+":"+name).Output()
		})
		if parseErr != nil {
			runs, createErr := runStore.Create(repositoryID, pullRequestID, commitID, []checkruns.Definition{{Name: "documentation/configuration", Image: "invalid", Command: "invalid configuration", TimeoutSeconds: 1, WorkingDirectory: "."}})
			if createErr != nil {
				return fmt.Errorf("create invalid documentation configuration check: %w", createErr)
			}
			if len(runs) == 1 {
				if recordErr := runStore.RecordFailure(runs[0], parseErr.Error()); recordErr != nil {
					return fmt.Errorf("record invalid documentation configuration: %w", recordErr)
				}
			}
			return nil
		}
		changed, changedErr := documentationChangedPaths(repository, commitID)
		if changedErr != nil {
			return fmt.Errorf("resolve documentation check changes: %w", changedErr)
		}
		requiredNames := map[string]bool{}
		for _, name := range requiredDocumentation {
			requiredNames[name] = true
		}
		for _, definition := range docsDefinitions {
			if documentationDefinitionSelected(definition, changed, requiredNames) {
				definitions = append(definitions, definition)
			}
		}
	} else if exit, ok := docsErr.(*exec.ExitError); !ok || exit.ExitCode() != 128 {
		return fmt.Errorf("read documentation check configuration: %w", docsErr)
	}
	if requiredOnly != nil {
		all := definitions
		definitions = make([]checkruns.Definition, 0, len(requiredOnly))
		for _, name := range requiredOnly {
			for _, definition := range all {
				if definition.Name == name {
					definitions = append(definitions, definition)
					break
				}
			}
		}
	}
	runs, err := runStore.Create(repositoryID, pullRequestID, commitID, definitions)
	if err != nil {
		return fmt.Errorf("create check runs: %w", err)
	}
	for _, run := range runs {
		go runStore.Execute(run, repository.Path())
	}
	return nil
}

func documentationChangedPaths(repository *storage.Repository, commitID string) (map[string]bool, error) {
	commit, err := repository.ReadCommit(storage.ObjectID(commitID))
	if err != nil {
		return nil, err
	}
	if len(commit.Parents) == 0 {
		return map[string]bool{checkruns.DocumentationConfigPath: true}, nil
	}
	output, err := exec.Command("git", "--git-dir="+repository.Path(), "diff", "--name-only", string(commit.Parents[0]), commitID, "--").Output()
	if err != nil {
		return nil, err
	}
	changed := map[string]bool{}
	for _, name := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if name != "" {
			changed[name] = true
		}
	}
	return changed, nil
}

func documentationDefinitionAffected(definition checkruns.Definition, changed map[string]bool) bool {
	if definition.Documentation == nil || changed[checkruns.DocumentationConfigPath] {
		return true
	}
	for _, dependency := range definition.Documentation.DependencyPaths {
		if changed[dependency] {
			return true
		}
	}
	return false
}

func documentationDefinitionSelected(definition checkruns.Definition, changed, required map[string]bool) bool {
	return documentationDefinitionAffected(definition, changed) || required[definition.Name]
}

func startBoundCheckRuns(gitStore *storage.Store, runStore *checkruns.Store, repositoryID, pullRequestID, commitID string, definitions []checkruns.Definition) {
	if gitStore == nil || runStore == nil || len(definitions) == 0 {
		return
	}
	repository, err := gitStore.Open(repositoryID)
	if err != nil {
		log.Printf("open repository for candidate checks: %v", err)
		return
	}
	runs, err := runStore.Create(repositoryID, pullRequestID, commitID, definitions)
	if err != nil {
		log.Printf("create candidate check runs: %v", err)
		return
	}
	for _, run := range runs {
		go runStore.Execute(run, repository.Path())
	}
}

func resumeCheckRuns(gitStore *storage.Store, runStore *checkruns.Store) {
	runs, err := runStore.Nonterminal()
	if err != nil {
		log.Printf("recover check runs: %v", err)
		return
	}
	for _, run := range runs {
		repository, openErr := gitStore.Open(run.RepositoryID)
		if openErr != nil {
			log.Printf("recover check repository: %v", openErr)
			continue
		}
		go runStore.Execute(run, repository.Path())
	}
}

func startCheckRunRecovery(gitStore *storage.Store, runStore *checkruns.Store) {
	resumeCheckRuns(gitStore, runStore)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			resumeCheckRuns(gitStore, runStore)
		}
	}()
}

func repairEvidence(run checkruns.Run, events []checkruns.Event) *changesessions.CheckEvidence {
	evidence := &changesessions.CheckEvidence{
		RunID:      run.ID,
		Definition: changesessions.CheckDefinition{Name: run.Definition.Name, Image: run.Definition.Image, Command: run.Definition.Command, WorkingDirectory: run.Definition.WorkingDirectory, Environment: run.Definition.Environment, TimeoutSeconds: run.Definition.TimeoutSeconds},
		Events:     make([]changesessions.CheckEvent, 0, len(events)),
		Artifacts:  make([]changesessions.CheckArtifact, 0, len(run.Artifacts)),
	}
	for _, event := range events {
		// Control projections describe collaborator actions, not the automated
		// failure. Keep execution state, command outcomes, and complete logs.
		if event.Kind == "control" {
			continue
		}
		evidence.Events = append(evidence.Events, changesessions.CheckEvent{Sequence: event.Sequence, Attempt: event.Attempt, Kind: event.Kind, State: event.State, Stream: event.Stream, Message: event.Message, ExitCode: event.ExitCode})
	}
	for _, artifact := range run.Artifacts {
		evidence.Artifacts = append(evidence.Artifacts, changesessions.CheckArtifact{ID: artifact.ID, Attempt: artifact.Attempt, Path: artifact.Path, Size: artifact.Size, SHA256: artifact.SHA256, ContentType: artifact.ContentType, CreatedAt: artifact.CreatedAt})
	}
	return evidence
}

func registerPullRequestRoutes(mux *http.ServeMux, gitStore *storage.Store, repositoriesStore *repositories.Store, proposalStore *proposals.Store, store *pullrequests.Store, authStore *auth.Store, activityStore *activities.Store, userStore *users.Store, checkRunStore *checkruns.Store, sessionStore *changesessions.Store, documentationStore *docscollections.Store, durableStore *durableschemas.Store, federationStore ...*federation.Store) {
	var federated *federation.Store
	if len(federationStore) > 0 {
		federated = federationStore[0]
	}
	store.ConfigureRequiredChecks(repositoriesStore, checkRunStore)
	reconcileTaskState := func(pull pullrequests.PullRequest) (pullrequests.PullRequest, error) {
		if pull.TaskStatePending == "" || pull.ProposalID == nil || pull.TaskID == nil || proposalStore == nil {
			return pull, nil
		}
		actorID := pull.AuthorID
		before, _ := proposalStore.ListTasks(pull.RepositoryID, *pull.ProposalID)
		var err error
		switch pull.TaskStatePending {
		case "review":
			_, err = proposalStore.LinkTaskContribution(pull.RepositoryID, *pull.ProposalID, *pull.TaskID, actorID, proposals.TaskContribution{PullRequestID: pull.ID, SessionID: valueOrEmpty(pull.TaskSessionID), RunID: valueOrEmpty(pull.TaskRunID), SourceCommitID: pull.SourceCommitID, CommitIDs: append([]string(nil), pull.TaskCommitIDs...), Status: "review"})
		case "closed":
			if pull.ClosedBy != nil {
				actorID = *pull.ClosedBy
			}
			_, err = proposalStore.UpdateTaskContribution(pull.RepositoryID, *pull.ProposalID, *pull.TaskID, actorID, pull.ID, "closed")
		case "merged":
			if pull.MergedBy != nil {
				actorID = *pull.MergedBy
			}
			_, err = proposalStore.UpdateTaskContribution(pull.RepositoryID, *pull.ProposalID, *pull.TaskID, actorID, pull.ID, "merged")
		}
		if err != nil && !errors.Is(err, proposals.ErrDurabilityUncertain) {
			return pull, err
		}
		confirmed, confirmErr := store.ConfirmTaskState(pull.RepositoryID, pull.ID, pull.TaskStatePending)
		if confirmErr != nil {
			return pull, confirmErr
		}
		after, _ := proposalStore.ListTasks(pull.RepositoryID, *pull.ProposalID)
		recordTaskTransitions(activityStore, repositoriesStore, actorID, pull.RepositoryID, *pull.ProposalID, before, after)
		return confirmed, nil
	}
	store.ConfigureQueueFinalizer(func(merged pullrequests.PullRequest) error {
		if err := finalizeQueuedMerge(merged, repositoriesStore, proposalStore, activityStore); err != nil {
			return err
		}
		if _, err := reconcileTaskState(merged); err != nil {
			return err
		}
		if documentationStore != nil && merged.MergedBy != nil {
			if err := publishMergedDocumentation(gitStore, documentationStore, merged, *merged.MergedBy); err != nil {
				return err
			}
		}
		return finalizeFederatedMerge(federated, merged)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		all, err := store.List(r.PathValue("id"))
		if writePullRequestError(w, err) {
			return
		}
		for i := range all {
			if all[i].TaskStatePending != "" {
				if repaired, repairErr := reconcileTaskState(all[i]); repairErr == nil {
					all[i] = repaired
				}
			}
			projectPullDurableMigration(&all[i], durableStore)
		}
		page, next, ok := paginate(r, all, func(p pullrequests.PullRequest) string { return p.ID })
		if !ok {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"pull_requests": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		var input pullRequestInput
		if decodeJSON(r, &input) != nil || input.Title == nil || input.Body == nil || input.SourceBranch == nil || input.TargetBranch == nil {
			writeAPIError(w, 400, "invalid_pull_request", "title, body, source_branch, and target_branch are required")
			return
		}
		target, err := repositoriesStore.GetByID(r.PathValue("id"))
		if writeRepositoryError(w, err) {
			return
		}
		sourceRepositoryID := target.ID
		if input.SourceRepositoryID != nil {
			sourceRepositoryID = *input.SourceRepositoryID
		}
		targetCollaborator, err := repositoriesStore.HasCollaborator(actor.UserID, target.ID)
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		participant := actor.UserID == target.OwnerID || targetCollaborator
		if sourceRepositoryID == target.ID {
			if !participant {
				writeAPIError(w, 404, "repository_not_found", "repository not found")
				return
			}
		} else {
			source, sourceErr := repositoriesStore.GetByID(sourceRepositoryID)
			if sourceErr != nil || source.OwnerID != actor.UserID || source.UpstreamRepositoryID != target.ID || (target.Visibility != repositories.Public && !participant) {
				writeAPIError(w, 404, "repository_not_found", "repository not found")
				return
			}
		}
		if input.ProposalID != nil {
			if proposalStore == nil {
				writeAPIError(w, 400, "invalid_pull_request", "proposal_id is invalid")
				return
			}
			if _, err := proposalStore.Get(r.PathValue("id"), *input.ProposalID); errors.Is(err, proposals.ErrNotFound) {
				writeAPIError(w, 400, "invalid_pull_request", "proposal_id is invalid")
				return
			} else if err != nil {
				log.Printf("proposal storage while creating pull request: %v", err)
				writeAPIError(w, 500, "internal_error", "proposal storage unavailable")
				return
			}
		}
		created, err := store.CreateFrom(r.PathValue("id"), sourceRepositoryID, actor.UserID, *input.Title, *input.Body, *input.SourceBranch, *input.TargetBranch, input.ProposalID)
		location := "/repositories/" + r.PathValue("id") + "/pulls/" + created.ID
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			required, _ := repositoriesStore.RequiredChecks(created.RepositoryID, created.TargetBranch)
			startCheckRuns(gitStore, checkRunStore, created, required...)
			recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "pull_request.created", ActorID: actor.UserID, RepositoryID: created.RepositoryID, ResourceType: "pull_request", ResourceID: created.ID, ResourceTitle: created.Title, ResourceRevision: created.SourceCommitID})
			recordMentions(activityStore, repositoriesStore, userStore, actor.UserID, created.RepositoryID, "pull_request", created.ID, created.Title, created.Title+"\n"+created.Body)
			w.Header().Set("Location", location)
			writeUncertainMutation(w, created)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		required, _ := repositoriesStore.RequiredChecks(created.RepositoryID, created.TargetBranch)
		startCheckRuns(gitStore, checkRunStore, created, required...)
		recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "pull_request.created", ActorID: actor.UserID, RepositoryID: created.RepositoryID, ResourceType: "pull_request", ResourceID: created.ID, ResourceTitle: created.Title, ResourceRevision: created.SourceCommitID})
		recordMentions(activityStore, repositoriesStore, userStore, actor.UserID, created.RepositoryID, "pull_request", created.ID, created.Title, created.Title+"\n"+created.Body)
		w.Header().Set("Location", location)
		writeJSON(w, 201, created)
	})
	// Task publication is intentionally a distinct command: it verifies the
	// assignment/session evidence before creating an otherwise ordinary pull.
	mux.HandleFunc("POST /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/contributions", func(w http.ResponseWriter, r *http.Request) {
		if proposalStore == nil {
			writeAPIError(w, 404, "proposal_not_found", "proposal not found")
			return
		}
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Title                string `json:"title"`
			Body                 string `json:"body"`
			SourceBranch         string `json:"source_branch"`
			TargetBranch         string `json:"target_branch"`
			SessionID            string `json:"session_id"`
			RunID                string `json:"run_id"`
			SourceRepositoryID   string `json:"source_repository_id"`
			DesignChanges        string `json:"design_changes"`
			CodeChanges          string `json:"code_changes"`
			InteractionTradeoffs string `json:"interaction_tradeoffs"`
			ContentTradeoffs     string `json:"content_tradeoffs"`
		}
		if decodeJSON(r, &input) != nil || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.SourceBranch) == "" || strings.TrimSpace(input.TargetBranch) == "" {
			writeAPIError(w, 400, "invalid_task_contribution", "title, body, source_branch, and target_branch are required")
			return
		}
		task, err := proposalStore.GetTask(r.PathValue("id"), r.PathValue("proposal_id"), r.PathValue("task_id"))
		if writeProposalError(w, err) {
			return
		}
		proposal, err := proposalStore.Get(r.PathValue("id"), r.PathValue("proposal_id"))
		if writeProposalError(w, err) {
			return
		}
		if proposal.Status != proposals.Open || task.Status != proposals.TaskTodo || task.Assignment == nil || task.Assignment.ContextRevision != task.ContextRevision || (task.Assignment.AssigneeType == "human" && task.Assignment.AssigneeID != actor.UserID) {
			writeAPIError(w, 409, "task_not_publishable", "task work must be published by its current assignee")
			return
		}
		if task.Reasoning != nil && task.Reasoning.AnalysisStatus == "accessibility_repair" {
			sections := []string{strings.TrimSpace(input.DesignChanges), strings.TrimSpace(input.CodeChanges), strings.TrimSpace(input.InteractionTradeoffs), strings.TrimSpace(input.ContentTradeoffs)}
			for _, section := range sections {
				if section == "" || len(section) > 4000 {
					writeAPIError(w, 422, "accessibility_repair_context_required", "accessible repairs must document design and code changes plus interaction and content tradeoffs")
					return
				}
			}
			input.Body = strings.TrimSpace(input.Body) + "\n\n## Design changes\n\n" + sections[0] + "\n\n## Code changes\n\n" + sections[1] + "\n\n## Interaction tradeoffs\n\n" + sections[2] + "\n\n## Content tradeoffs\n\n" + sections[3]
		}
		var sessionID, runID *string
		commits := []string{}
		expectedSourceCommit := ""
		var reviewEvidence *pullrequests.TaskReviewEvidence
		if task.Assignment.AssigneeType == "agent" {
			if sessionStore == nil || input.SessionID == "" || input.RunID == "" {
				writeAPIError(w, 409, "task_not_publishable", "completed agent session evidence is required")
				return
			}
			session, getErr := sessionStore.Get(r.PathValue("id"), task.ID, input.SessionID)
			if getErr != nil || session.ProposalID != r.PathValue("proposal_id") {
				writeAPIError(w, 409, "task_not_publishable", "completed agent session evidence is required")
				return
			}
			runs, listErr := sessionStore.ListRuns(r.PathValue("id"), task.ID, input.SessionID)
			if listErr != nil {
				writeChangeSessionError(w, listErr)
				return
			}
			found := false
			expectedBranch := "agent/tasks/" + task.ID + "-" + task.Assignment.ID[:8]
			for _, run := range runs {
				if run.ID == input.RunID && run.State == changesessions.Completed && run.Outcome != nil && run.WorkingBranch == input.SourceBranch && run.WorkingBranch == expectedBranch {
					found = true
					commits = append(commits, run.Outcome.Commits...)
					expectedSourceCommit = run.Outcome.CommitID
					reasoning := []pullrequests.ReviewReasoningItem{}
					if task.Reasoning != nil {
						for _, item := range task.Reasoning.Items {
							reasoning = append(reasoning, pullrequests.ReviewReasoningItem{ID: item.ID, Kind: item.Kind, Summary: item.Summary, Status: item.Status})
						}
					}
					reviewEvidence = &pullrequests.TaskReviewEvidence{BaseRevision: run.SourceCommitID, AssignmentID: task.Assignment.ID, AgentID: run.AgentID, InitiatorID: run.InitiatorID, Mandate: task.Assignment.Mandate, CompletionCriteria: task.Outcome, Outcome: *run.Outcome, Reasoning: reasoning}
					if task.Reasoning != nil {
						reviewEvidence.OrganizationID, reviewEvidence.MandateID, reviewEvidence.OpportunityID, reviewEvidence.EvidenceRevision = task.Reasoning.OrganizationID, task.Reasoning.MandateID, task.Reasoning.OpportunityID, task.Reasoning.Revision
					}
					break
				}
			}
			if !found {
				writeAPIError(w, 409, "task_not_publishable", "completed agent session evidence does not match the source branch")
				return
			}
			if task.Reasoning != nil && task.Reasoning.AnalysisStatus == "stewardship_opportunity" {
				criterionRecorded := false
				for _, criterion := range reviewEvidence.Outcome.Criteria {
					criterionRecorded = criterionRecorded || criterion.Criterion == task.Outcome
				}
				if len(reviewEvidence.Outcome.Commands) == 0 || !criterionRecorded {
					writeAPIError(w, 409, "stewardship_evidence_incomplete", "stewarded publication requires commands and the recorded completion criterion status")
					return
				}
			}
			sessionID, runID = &input.SessionID, &input.RunID
		}
		proposalID, taskID := r.PathValue("proposal_id"), r.PathValue("task_id")
		sourceRepositoryID := r.PathValue("id")
		if input.SourceRepositoryID != "" {
			source, sourceErr := repositoriesStore.GetByID(input.SourceRepositoryID)
			if sourceErr != nil || source.OwnerID != actor.UserID || source.UpstreamRepositoryID != r.PathValue("id") || task.Assignment.AssigneeType != "human" {
				writeAPIError(w, 404, "repository_not_found", "source repository not found")
				return
			}
			sourceRepositoryID = source.ID
		}
		created, err := store.CreateTaskContributionFromWithEvidence(r.PathValue("id"), sourceRepositoryID, actor.UserID, input.Title, input.Body, input.SourceBranch, input.TargetBranch, expectedSourceCommit, commits, &proposalID, &taskID, sessionID, runID, reviewEvidence)
		if errors.Is(err, pullrequests.ErrSourceChanged) {
			writeAPIError(w, 409, "task_not_publishable", "the source branch no longer matches the completed task work")
			return
		}
		if err != nil && !errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			writePullRequestError(w, err)
			return
		}
		created, linkErr := reconcileTaskState(created)
		startCheckRuns(gitStore, checkRunStore, created)
		recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "pull_request.created", ActorID: actor.UserID, RepositoryID: created.RepositoryID, ResourceType: "pull_request", ResourceID: created.ID, ResourceTitle: created.Title, ResourceRevision: created.SourceCommitID})
		w.Header().Set("Location", "/repositories/"+created.RepositoryID+"/pulls/"+created.ID)
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) || linkErr != nil {
			writeUncertainMutation(w, created)
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		pullRequest, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		if pullRequest.TaskStatePending != "" {
			var repairErr error
			pullRequest, repairErr = reconcileTaskState(pullRequest)
			if repairErr != nil {
				writeUncertainMutation(w, pullRequest)
				return
			}
		}
		projectPullDurableMigration(&pullRequest, durableStore)
		writeJSON(w, 200, pullRequest)
	})
	mux.HandleFunc("PATCH /repositories/{id}/pulls/{pull_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		existing, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		if existing.AuthorID != actor.UserID || existing.SourceRepositoryID == existing.RepositoryID {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		source, err := repositoriesStore.GetByID(existing.SourceRepositoryID)
		if err != nil || source.OwnerID != actor.UserID {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		var input pullRequestPolicyInput
		if decodeJSON(r, &input) != nil || input.MaintainerEditsAllowed == nil {
			writeAPIError(w, 400, "invalid_pull_request", "maintainer_edits_allowed is required")
			return
		}
		updated, err := store.UpdatePolicy(existing.RepositoryID, existing.ID, *input.MaintainerEditsAllowed)
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			writeUncertainMutation(w, updated)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, 200, updated)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/close", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		existing, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		if existing.TaskStatePending != "" {
			existing, err = reconcileTaskState(existing)
			if err != nil {
				writeUncertainMutation(w, existing)
				return
			}
		}
		target, err := repositoriesStore.GetByID(existing.RepositoryID)
		if err != nil || (actor.UserID != existing.AuthorID && actor.UserID != target.OwnerID) {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		updated, err := store.Close(existing.RepositoryID, existing.ID, actor.UserID)
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			updated, _ = reconcileTaskState(updated)
			writeUncertainMutation(w, updated)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		updated, taskErr := reconcileTaskState(updated)
		if taskErr != nil {
			writeUncertainMutation(w, updated)
			return
		}
		writeJSON(w, 200, updated)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/maintainer-credential", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		pull, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		if pull.Status != pullrequests.Open || pull.SourceRepositoryID == pull.RepositoryID || !pull.MaintainerEditsAllowed {
			writeAPIError(w, 409, "maintainer_edits_not_allowed", "the contribution owner has not allowed participant edits")
			return
		}
		source, err := repositoriesStore.GetByID(pull.SourceRepositoryID)
		if err != nil {
			writeAPIError(w, 409, "source_repository_unavailable", "the contribution repository is unavailable")
			return
		}
		issued, err := authStore.IssuePullRequestBound(actor.UserID, "Pull request participant edit", []string{"git:read", "git:write"}, time.Hour, source.ID, "refs/heads/"+pull.SourceBranch, pull.ID)
		if err != nil {
			writeAPIError(w, 500, "internal_error", "branch credential could not be issued")
			return
		}
		writeJSON(w, 201, issued)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/synchronize", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		existing, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		if existing.AuthorID != actor.UserID {
			writeAPIError(w, http.StatusNotFound, "pull_request_not_found", "pull request not found")
			return
		}
		target, targetErr := repositoriesStore.GetByID(existing.RepositoryID)
		if targetErr != nil {
			writeAPIError(w, http.StatusNotFound, "pull_request_not_found", "pull request not found")
			return
		}
		targetCollaborator, collaboratorErr := repositoriesStore.HasCollaborator(actor.UserID, existing.RepositoryID)
		if collaboratorErr != nil || (target.Visibility != repositories.Public && target.OwnerID != actor.UserID && !targetCollaborator) {
			writeAPIError(w, http.StatusNotFound, "pull_request_not_found", "pull request not found")
			return
		}
		source, sourceErr := repositoriesStore.GetByID(existing.SourceRepositoryID)
		allowedSource := sourceErr == nil && source.OwnerID == actor.UserID
		if sourceErr == nil && existing.SourceRepositoryID == existing.RepositoryID {
			collaborator, collaboratorErr := repositoriesStore.HasCollaborator(actor.UserID, existing.RepositoryID)
			allowedSource = collaboratorErr == nil && (source.OwnerID == actor.UserID || collaborator)
		}
		if !allowedSource {
			writeAPIError(w, http.StatusNotFound, "pull_request_not_found", "pull request not found")
			return
		}
		var updated pullrequests.PullRequest
		err = repositoriesStore.WithContributionAuthorization(actor.UserID, existing.RepositoryID, existing.SourceRepositoryID, func() error {
			var synchronizeErr error
			updated, synchronizeErr = store.SynchronizeSource(r.PathValue("id"), existing.ID)
			return synchronizeErr
		})
		if errors.Is(err, repositories.ErrNotFound) {
			writeAPIError(w, http.StatusNotFound, "pull_request_not_found", "pull request not found")
			return
		}
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			required, _ := repositoriesStore.RequiredChecks(updated.RepositoryID, updated.TargetBranch)
			startCheckRuns(gitStore, checkRunStore, updated, required...)
			recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "pull_request.synchronized", ActorID: actor.UserID, RepositoryID: updated.RepositoryID, ResourceType: "pull_request", ResourceID: updated.ID, ResourceTitle: updated.Title, ResourceRevision: updated.SourceCommitID})
			writeUncertainMutation(w, updated)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		required, _ := repositoriesStore.RequiredChecks(updated.RepositoryID, updated.TargetBranch)
		startCheckRuns(gitStore, checkRunStore, updated, required...)
		recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "pull_request.synchronized", ActorID: actor.UserID, RepositoryID: updated.RepositoryID, ResourceType: "pull_request", ResourceID: updated.ID, ResourceTitle: updated.Title, ResourceRevision: updated.SourceCommitID})
		writeJSON(w, http.StatusOK, updated)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/checks", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		if checkRunStore == nil {
			writeJSON(w, 200, map[string]any{"check_runs": []checkruns.Run{}})
			return
		}
		if _, err := store.Get(r.PathValue("id"), r.PathValue("pull_id")); writePullRequestError(w, err) {
			return
		}
		runs, err := checkRunStore.List(r.PathValue("id"), r.PathValue("pull_id"))
		if err != nil {
			log.Printf("check run storage: %v", err)
			writeAPIError(w, 500, "internal_error", "check run storage unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"check_runs": runs})
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/candidates", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		candidates, err := store.Candidates(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates})
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/checks/{check_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		if _, err := store.Get(r.PathValue("id"), r.PathValue("pull_id")); writePullRequestError(w, err) {
			return
		}
		run, err := checkRunStore.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("check_id"))
		if errors.Is(err, checkruns.ErrNotFound) {
			writeAPIError(w, 404, "check_run_not_found", "check run not found")
			return
		}
		if err != nil {
			log.Printf("check run storage: %v", err)
			writeAPIError(w, 500, "internal_error", "check run storage unavailable")
			return
		}
		writeJSON(w, 200, run)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/checks/{check_id}/rerun", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		pull, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		if pull.Status != pullrequests.Open {
			writeAPIError(w, 409, "pull_request_closed", "checks on a closed pull request cannot be rerun")
			return
		}
		existing, err := checkRunStore.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("check_id"))
		if errors.Is(err, checkruns.ErrNotFound) {
			writeAPIError(w, 404, "check_run_not_found", "check run not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "internal_error", "check run storage unavailable")
			return
		}
		repository, err := gitStore.Open(existing.RepositoryID)
		if err != nil {
			writeAPIError(w, 500, "internal_error", "check repository unavailable")
			return
		}
		run, err := checkRunStore.Rerun(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("check_id"), actor.UserID)
		if errors.Is(err, checkruns.ErrNotFound) {
			writeAPIError(w, 404, "check_run_not_found", "check run not found")
			return
		}
		if errors.Is(err, checkruns.ErrInvalidState) {
			writeAPIError(w, 409, "check_run_active", "an active check cannot be rerun")
			return
		}
		if err != nil {
			log.Printf("rerun check: %v", err)
			writeAPIError(w, 500, "internal_error", "check could not be rerun")
			return
		}
		go checkRunStore.Execute(run, repository.Path())
		writeJSON(w, http.StatusAccepted, run)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/checks/{check_id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		pull, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		if pull.Status != pullrequests.Open {
			writeAPIError(w, 409, "pull_request_closed", "checks on a closed pull request cannot be canceled")
			return
		}
		run, err := checkRunStore.Cancel(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("check_id"), actor.UserID)
		if errors.Is(err, checkruns.ErrNotFound) {
			writeAPIError(w, 404, "check_run_not_found", "check run not found")
			return
		}
		if errors.Is(err, checkruns.ErrInvalidState) {
			writeAPIError(w, 409, "check_run_finished", "a finished check cannot be canceled")
			return
		}
		if err != nil {
			log.Printf("cancel check: %v", err)
			writeAPIError(w, 500, "internal_error", "check could not be canceled")
			return
		}
		writeJSON(w, http.StatusOK, run)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/checks/{check_id}/events", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		if _, err := store.Get(r.PathValue("id"), r.PathValue("pull_id")); writePullRequestError(w, err) {
			return
		}
		after := int64(0)
		if value, present := r.URL.Query()["after"]; present {
			if len(value) != 1 || value[0] == "" {
				writeAPIError(w, 400, "invalid_cursor", "after is invalid")
				return
			}
			parsed, err := strconv.ParseInt(value[0], 10, 64)
			if err != nil || parsed < 0 {
				writeAPIError(w, 400, "invalid_cursor", "after is invalid")
				return
			}
			after = parsed
		}
		events, err := checkRunStore.Events(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("check_id"), after)
		if errors.Is(err, checkruns.ErrNotFound) {
			writeAPIError(w, 404, "check_run_not_found", "check run not found")
			return
		}
		if err != nil {
			log.Printf("check evidence storage: %v", err)
			writeAPIError(w, 500, "internal_error", "check evidence unavailable")
			return
		}
		next := after
		if len(events) > 0 {
			next = events[len(events)-1].Sequence
		}
		writeJSON(w, 200, map[string]any{"events": events, "next_sequence": next})
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/checks/{check_id}/artifacts/{artifact_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		if _, err := store.Get(r.PathValue("id"), r.PathValue("pull_id")); writePullRequestError(w, err) {
			return
		}
		file, artifact, err := checkRunStore.OpenArtifact(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("check_id"), r.PathValue("artifact_id"))
		if errors.Is(err, checkruns.ErrNotFound) {
			writeAPIError(w, 404, "check_artifact_not_found", "check artifact not found")
			return
		}
		if err != nil {
			log.Printf("check artifact storage: %v", err)
			writeAPIError(w, 500, "internal_error", "check artifact unavailable")
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", artifact.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(artifact.Path)))
		http.ServeContent(w, r, path.Base(artifact.Path), artifact.CreatedAt, file)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/commits", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		commits, err := store.Commits(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, 200, map[string]any{"commits": commits})
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/files", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		changes, err := store.Changes(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, 200, map[string]any{"files": changes})
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/merge-readiness", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id"))
		if !ok {
			return
		}
		if !authenticated {
			actor, authenticated, ok = authenticateOptionalRequest(w, r, authStore, "repositories:read", false)
			if !ok {
				return
			}
		}
		target, err := repositoriesStore.GetByID(r.PathValue("id"))
		if writeRepositoryError(w, err) {
			return
		}
		owner := authenticated && actor.UserID == target.OwnerID
		report, err := store.Readiness(r.PathValue("id"), r.PathValue("pull_id"), owner)
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, report)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/conflict-analysis", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		target, err := repositoriesStore.GetByID(r.PathValue("id"))
		if writeRepositoryError(w, err) {
			return
		}
		analysis, err := store.AnalyzePullConflict(r.PathValue("id"), r.PathValue("pull_id"), r.URL.Query().Get("candidate_id"), target.OwnerID)
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, analysis)
	})
	mux.HandleFunc("GET /repositories/{id}/conflict-analysis", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		target, err := repositoriesStore.GetByID(r.PathValue("id"))
		if writeRepositoryError(w, err) {
			return
		}
		sourceBranch, targetBranch := r.URL.Query().Get("source_branch"), r.URL.Query().Get("target_branch")
		if sourceBranch == "" || targetBranch == "" {
			writeAPIError(w, http.StatusBadRequest, "invalid_conflict_analysis", "source_branch and target_branch are required")
			return
		}
		analysis, err := store.AnalyzeBranches(r.PathValue("id"), sourceBranch, targetBranch, target.OwnerID)
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, analysis)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/merge", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
			return
		}
		if existing, getErr := store.Get(r.PathValue("id"), r.PathValue("pull_id")); getErr == nil && existing.TaskStatePending != "" {
			if repaired, repairErr := reconcileTaskState(existing); repairErr != nil {
				writeUncertainMutation(w, repaired)
				return
			}
		} else if getErr != nil {
			writePullRequestError(w, getErr)
			return
		}
		merged, err := store.Merge(r.PathValue("id"), r.PathValue("pull_id"), actor.UserID)
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			merged, _ = reconcileTaskState(merged)
			recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "pull_request.merged", ActorID: actor.UserID, RepositoryID: merged.RepositoryID, ResourceType: "pull_request", ResourceID: merged.ID, ResourceTitle: merged.Title, ResourceRevision: merged.SourceCommitID})
			writeUncertainMutation(w, merged)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		merged, taskErr := reconcileTaskState(merged)
		if taskErr != nil {
			writeUncertainMutation(w, merged)
			return
		}
		if merged.MergeCommitID != nil {
			required, requiredErr := repositoriesStore.RequiredChecks(merged.RepositoryID, merged.TargetBranch)
			if requiredErr != nil {
				log.Printf("resolve required checks after merge: %v", requiredErr)
				writeUncertainMutation(w, merged)
				return
			}
			integrated := merged
			integrated.SourceCommitID = *merged.MergeCommitID
			if checkErr := startCheckRuns(gitStore, checkRunStore, integrated, required...); checkErr != nil {
				log.Printf("start required checks after merge: %v", checkErr)
				writeUncertainMutation(w, merged)
				return
			}
		}
		if documentationStore != nil {
			if publicationErr := publishMergedDocumentation(gitStore, documentationStore, merged, actor.UserID); publicationErr != nil {
				log.Printf("publish merged documentation: %v", publicationErr)
				writeUncertainMutation(w, merged)
				return
			}
		}
		if federationErr := finalizeFederatedMerge(federated, merged); federationErr != nil {
			log.Printf("finalize federated merge: %v", federationErr)
			writeUncertainMutation(w, merged)
			return
		}
		if merged.ProposalID != nil && merged.TaskID == nil && proposalStore != nil {
			proposal, proposalErr := proposalStore.Get(r.PathValue("id"), *merged.ProposalID)
			closedLinkedProposal := false
			if proposalErr == nil && proposal.Status == proposals.Open {
				closed := proposals.Closed
				_, proposalErr = proposalStore.Update(r.PathValue("id"), proposal.ID, proposals.Patch{Status: &closed})
				closedLinkedProposal = proposalErr == nil || errors.Is(proposalErr, proposals.ErrDurabilityUncertain)
			}
			if errors.Is(proposalErr, proposals.ErrDurabilityUncertain) {
				if closedLinkedProposal {
					recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "proposal.closed", ActorID: actor.UserID, RepositoryID: merged.RepositoryID, ResourceType: "proposal", ResourceID: proposal.ID, ResourceTitle: proposal.Title})
				}
				recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "pull_request.merged", ActorID: actor.UserID, RepositoryID: merged.RepositoryID, ResourceType: "pull_request", ResourceID: merged.ID, ResourceTitle: merged.Title, ResourceRevision: merged.SourceCommitID})
				writeUncertainMutation(w, merged)
				return
			}
			if proposalErr != nil && !errors.Is(proposalErr, proposals.ErrDurabilityUncertain) {
				log.Printf("close linked proposal after merge: %v", proposalErr)
				writeAPIError(w, http.StatusInternalServerError, "internal_error", "linked proposal closure unavailable; retry merge")
				return
			}
			if closedLinkedProposal {
				recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "proposal.closed", ActorID: actor.UserID, RepositoryID: merged.RepositoryID, ResourceType: "proposal", ResourceID: proposal.ID, ResourceTitle: proposal.Title})
			}
		}
		recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "pull_request.merged", ActorID: actor.UserID, RepositoryID: merged.RepositoryID, ResourceType: "pull_request", ResourceID: merged.ID, ResourceTitle: merged.Title, ResourceRevision: merged.SourceCommitID})
		writeJSON(w, http.StatusOK, merged)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/queue", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
			return
		}
		queued, err := store.Enqueue(r.PathValue("id"), r.PathValue("pull_id"), actor.UserID)
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			if len(queued.IntegrationCandidates) > 0 {
				candidate := queued.IntegrationCandidates[len(queued.IntegrationCandidates)-1]
				startBoundCheckRuns(gitStore, checkRunStore, queued.RepositoryID, queued.ID, candidate.CommitID, candidate.CheckDefinitions)
			}
			target := queued.AuthorID
			recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "integration_queue.enqueued", ActorID: actor.UserID, RepositoryID: queued.RepositoryID, ResourceType: "pull_request", ResourceID: queued.ID, ResourceTitle: queued.Title, TargetUserID: &target})
			writeUncertainMutation(w, queued)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		if len(queued.IntegrationCandidates) > 0 {
			candidate := queued.IntegrationCandidates[len(queued.IntegrationCandidates)-1]
			startBoundCheckRuns(gitStore, checkRunStore, queued.RepositoryID, queued.ID, candidate.CommitID, candidate.CheckDefinitions)
		}
		target := queued.AuthorID
		recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "integration_queue.enqueued", ActorID: actor.UserID, RepositoryID: queued.RepositoryID, ResourceType: "pull_request", ResourceID: queued.ID, ResourceTitle: queued.Title, TargetUserID: &target})
		writeJSON(w, http.StatusOK, queued)
	})
	mux.HandleFunc("GET /repositories/{id}/branches/{branch}/queue", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		view, err := store.IntegrationQueue(r.PathValue("id"), r.PathValue("branch"))
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, view)
	})
	mux.HandleFunc("PATCH /repositories/{id}/pulls/{pull_id}/queue", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
			return
		}
		var input struct {
			Action   string `json:"action"`
			Position int    `json:"position"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_queue_action", "action and optional position are required")
			return
		}
		updated, err := store.OperateQueue(r.PathValue("id"), r.PathValue("pull_id"), actor.UserID, input.Action, input.Position)
		if errors.Is(err, pullrequests.ErrInvalid) {
			writeAPIError(w, http.StatusBadRequest, "invalid_queue_action", "action or position is invalid")
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		target := updated.AuthorID
		recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "integration_queue." + input.Action, ActorID: actor.UserID, RepositoryID: updated.RepositoryID, ResourceType: "pull_request", ResourceID: updated.ID, ResourceTitle: updated.Title, TargetUserID: &target})
		go func() {
			if err := store.AdvanceIntegrationQueues(); err != nil {
				log.Printf("advance integration queue after intervention: %v", err)
			}
		}()
		writeJSON(w, http.StatusOK, updated)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		all, err := store.ListComments(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		page, next, ok := paginate(r, all, func(c pullrequests.Comment) string { return c.ID })
		if !ok {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"comments": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:read", false)
		if !ok {
			return
		}
		pull, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		target, err := repositoriesStore.GetByID(r.PathValue("id"))
		if writeRepositoryError(w, err) {
			return
		}
		collaborator, collaboratorErr := repositoriesStore.HasCollaborator(actor.UserID, target.ID)
		participant := collaboratorErr == nil && (target.OwnerID == actor.UserID || collaborator)
		outsideAuthor := pull.SourceRepositoryID != pull.RepositoryID && pull.AuthorID == actor.UserID && target.Visibility == repositories.Public
		if !participant && !outsideAuthor {
			writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
			return
		}
		var input commentInput
		if decodeJSON(r, &input) != nil || input.Body == nil {
			writeAPIError(w, 400, "invalid_comment", "body is required")
			return
		}
		comment, err := store.AddComment(r.PathValue("id"), r.PathValue("pull_id"), actor.UserID, *input.Body)
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			w.Header().Set("Location", r.URL.Path+"/"+comment.ID)
			writeUncertainMutation(w, comment)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		if pull, pullErr := store.Get(r.PathValue("id"), r.PathValue("pull_id")); pullErr == nil {
			recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "pull_request.commented", ActorID: actor.UserID, RepositoryID: pull.RepositoryID, ResourceType: "pull_request", ResourceID: pull.ID, ResourceTitle: pull.Title})
			recordMentions(activityStore, repositoriesStore, userStore, actor.UserID, pull.RepositoryID, "pull_request", pull.ID, pull.Title, comment.Body)
		}
		w.Header().Set("Location", r.URL.Path+"/"+comment.ID)
		writeJSON(w, 201, comment)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/reviews", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		all, err := store.ListReviews(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		page, next, ok := paginate(r, all, func(review pullrequests.Review) string { return review.ID })
		if !ok {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"reviews": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/reviews", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var input reviewInput
		if decodeJSON(r, &input) != nil || input.Decision == nil || (*input.Decision != pullrequests.Approved && *input.Decision != pullrequests.ChangesRequested) {
			writeAPIError(w, 400, "invalid_review", "decision must be approved or changes_requested")
			return
		}
		review, err := store.SetReview(r.PathValue("id"), r.PathValue("pull_id"), actor.UserID, *input.Decision)
		location := r.URL.Path + "/" + review.ID
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			w.Header().Set("Location", location)
			writeUncertainMutation(w, review)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		if pull, pullErr := store.Get(r.PathValue("id"), r.PathValue("pull_id")); pullErr == nil {
			recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "review." + review.Decision, ActorID: actor.UserID, RepositoryID: pull.RepositoryID, ResourceType: "pull_request", ResourceID: pull.ID, ResourceTitle: pull.Title})
		}
		w.Header().Set("Location", location)
		writeJSON(w, 200, review)
	})
	mux.HandleFunc("DELETE /repositories/{id}/pulls/{pull_id}/reviews/{review_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		review, err := store.WithdrawReview(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("review_id"), actor.UserID)
		if errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			writeUncertainMutation(w, review)
			return
		}
		if writePullRequestError(w, err) {
			return
		}
		if pull, pullErr := store.Get(r.PathValue("id"), r.PathValue("pull_id")); pullErr == nil {
			recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "review.withdrawn", ActorID: actor.UserID, RepositoryID: pull.RepositoryID, ResourceType: "pull_request", ResourceID: pull.ID, ResourceTitle: pull.Title})
		}
		writeJSON(w, 200, review)
	})
}

func projectPullDurableMigration(pull *pullrequests.PullRequest, store *durableschemas.Store) {
	if store == nil || pull.TaskID == nil {
		return
	}
	schema, migration, work, err := store.FindMigrationWork(pull.RepositoryID, *pull.TaskID)
	if err != nil {
		return
	}
	pull.DurableMigration = &pullrequests.DurableMigrationReview{SchemaID: schema.ID, MigrationID: migration.ID, WorkID: work.ID, StepID: work.StepID, Kind: work.Kind, DependencyIDs: append([]string(nil), work.DependencyIDs...), Contract: work.Contract}
}

func finalizeQueuedMerge(merged pullrequests.PullRequest, repositoriesStore *repositories.Store, proposalStore *proposals.Store, activityStore *activities.Store) error {
	if merged.MergedBy == nil {
		return errors.New("queued merge attribution is missing")
	}
	actorID := *merged.MergedBy
	if merged.ProposalID != nil && merged.TaskID == nil && proposalStore != nil {
		proposal, err := proposalStore.Get(merged.RepositoryID, *merged.ProposalID)
		if err != nil {
			return err
		}
		if proposal.Status == proposals.Open {
			closed := proposals.Closed
			_, err = proposalStore.Update(merged.RepositoryID, proposal.ID, proposals.Patch{Status: &closed})
			if err != nil && !errors.Is(err, proposals.ErrDurabilityUncertain) {
				return err
			}
		}
		if err := recordActivityOnce(activityStore, repositoriesStore, "queue-proposal-closed:"+merged.RepositoryID+":"+merged.ID, activities.Event{Kind: "proposal.closed", ActorID: actorID, RepositoryID: merged.RepositoryID, ResourceType: "proposal", ResourceID: proposal.ID, ResourceTitle: proposal.Title}); err != nil {
			return err
		}
	}
	return recordActivityOnce(activityStore, repositoriesStore, "queue-pull-merged:"+merged.RepositoryID+":"+merged.ID, activities.Event{Kind: "pull_request.merged", ActorID: actorID, RepositoryID: merged.RepositoryID, ResourceType: "pull_request", ResourceID: merged.ID, ResourceTitle: merged.Title, ResourceRevision: merged.SourceCommitID})
}

func writePullRequestError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, pullrequests.ErrNotFound):
		writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
	case errors.Is(err, pullrequests.ErrInvalid):
		writeAPIError(w, 400, "invalid_pull_request", "pull request content or branches are invalid")
	case errors.Is(err, pullrequests.ErrBranchNotFound):
		writeAPIError(w, 400, "branch_not_found", "source or target branch does not identify a commit")
	case errors.Is(err, pullrequests.ErrSourceChanged):
		writeAPIError(w, http.StatusConflict, "source_branch_changed", "source branch must be synchronized before review")
	case errors.Is(err, pullrequests.ErrNotReady):
		writeAPIError(w, 409, "pull_request_not_ready", "pull request is not ready to merge")
	default:
		log.Printf("pull request storage: %v", err)
		writeAPIError(w, 500, "internal_error", "pull request storage unavailable")
	}
	return true
}

func registerChangeSessionRoutes(mux *http.ServeMux, gitStore *storage.Store, repositoriesStore *repositories.Store, pullRequestStore *pullrequests.Store, store *changesessions.Store, authStore *auth.Store, activityStore *activities.Store, checkRunStore *checkruns.Store, previewStore *previews.Store) {
	loadPull := func(w http.ResponseWriter, r *http.Request) (pullrequests.PullRequest, bool) {
		pull, err := pullRequestStore.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return pullrequests.PullRequest{}, false
		}
		return pull, true
	}
	workingRepository := func(pull pullrequests.PullRequest) string {
		return pull.SourceRepositoryID
	}
	validateRunCredential := func(w http.ResponseWriter, credential auth.Credential, pull pullrequests.PullRequest) bool {
		if credential.RepositoryID != workingRepository(pull) {
			writeAPIError(w, 404, "agent_run_not_found", "agent run not found")
			return false
		}
		if pull.SourceRepositoryID != pull.RepositoryID && !activeMaintainerCredential(pullRequestStore, repositoriesStore, pull.SourceRepositoryID+".git", credential) {
			writeAPIError(w, 401, "invalid_credential", "credential is not active")
			return false
		}
		return true
	}
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/sessions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		pull, ok := loadPull(w, r)
		if !ok {
			return
		}
		if pull.Status != pullrequests.Open {
			writeAPIError(w, http.StatusConflict, "pull_request_closed", "change sessions require an open pull request")
			return
		}
		var input struct {
			CheckRunID string `json:"check_run_id"`
		}
		if r.Body != nil && r.Body != http.NoBody && decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_change_session", "change session input is invalid")
			return
		}
		var evidence *changesessions.CheckEvidence
		if input.CheckRunID != "" {
			if checkRunStore == nil {
				writeAPIError(w, 404, "check_run_not_found", "check run not found")
				return
			}
			run, runErr := checkRunStore.Get(pull.RepositoryID, pull.ID, input.CheckRunID)
			if errors.Is(runErr, checkruns.ErrNotFound) {
				writeAPIError(w, 404, "check_run_not_found", "check run not found")
				return
			}
			if runErr != nil {
				writeAPIError(w, 500, "internal_error", "check evidence unavailable")
				return
			}
			if run.State != "failed" || run.CommitID != pull.SourceCommitID {
				writeAPIError(w, 409, "check_not_repairable", "repair sessions require a failed check on the current pull request revision")
				return
			}
			events, eventErr := checkRunStore.Events(pull.RepositoryID, pull.ID, run.ID, 0)
			if eventErr != nil {
				writeAPIError(w, 500, "internal_error", "check evidence unavailable")
				return
			}
			evidence = repairEvidence(run, events)
		}
		var session changesessions.Session
		err := pullRequestStore.WithSourceRevision(pull.RepositoryID, pull.ID, pull.SourceCommitID, func(current pullrequests.PullRequest) error {
			var createErr error
			session, createErr = store.CreateWithEvidence(current.RepositoryID, current.ID, actor.UserID, current.SourceCommitID, evidence)
			return createErr
		})
		if errors.Is(err, pullrequests.ErrSourceChanged) {
			writeAPIError(w, http.StatusConflict, "source_branch_changed", "pull request advanced while the repair session was being created")
			return
		}
		if errors.Is(err, pullrequests.ErrNotReady) {
			writeAPIError(w, http.StatusConflict, "pull_request_closed", "change sessions require an open pull request")
			return
		}
		location := r.URL.Path + "/" + session.ID
		if errors.Is(err, changesessions.ErrDurabilityUncertain) {
			w.Header().Set("Location", location)
			writeUncertainMutation(w, session)
			return
		}
		if writeChangeSessionError(w, err) {
			return
		}
		w.Header().Set("Location", location)
		writeJSON(w, http.StatusCreated, session)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/sessions", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read"); !ok {
			return
		}
		if _, ok := loadPull(w, r); !ok {
			return
		}
		all, err := store.List(r.PathValue("id"), r.PathValue("pull_id"))
		if writeChangeSessionError(w, err) {
			return
		}
		page, next, valid := paginate(r, all, func(session changesessions.Session) string { return session.ID })
		if !valid {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"sessions": page, "next_cursor": next})
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/sessions/{session_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read"); !ok {
			return
		}
		if _, ok := loadPull(w, r); !ok {
			return
		}
		session, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"))
		if errors.Is(err, changesessions.ErrDurabilityUncertain) {
			writeUncertainMutation(w, session)
			return
		}
		if writeChangeSessionError(w, err) {
			return
		}
		writeJSON(w, 200, session)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/sessions/{session_id}/events", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read"); !ok {
			return
		}
		if _, ok := loadPull(w, r); !ok {
			return
		}
		session, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"))
		if errors.Is(err, changesessions.ErrDurabilityUncertain) {
			writeUncertainMutation(w, session)
			return
		}
		if writeChangeSessionError(w, err) {
			return
		}
		all, err := store.ListEvents(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"))
		if writeChangeSessionError(w, err) {
			return
		}
		page, next, valid := paginate(r, all, func(event changesessions.Event) string { return event.ID })
		if !valid {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"events": page, "next_cursor": next})
	})
	type workEventInput struct {
		Kind     string `json:"kind"`
		State    string `json:"state"`
		Message  string `json:"message"`
		Tool     string `json:"tool"`
		Artifact string `json:"artifact"`
		Branch   string `json:"branch"`
		CommitID string `json:"commit_id"`
	}
	type completionInput struct {
		Summary            string                     `json:"summary"`
		CommitID           string                     `json:"commit_id"`
		Checks             []changesessions.Check     `json:"checks"`
		Commands           []changesessions.Command   `json:"commands"`
		CompletionCriteria []changesessions.Criterion `json:"completion_criteria"`
		UnresolvedConcerns []string                   `json:"unresolved_concerns"`
	}
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/sessions/{session_id}/runs/{run_id}/completion", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, authStore, "git:write", false)
		if !ok {
			return
		}
		var input completionInput
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_run_completion", "run completion is invalid")
			return
		}
		input.Summary, input.CommitID = strings.TrimSpace(input.Summary), strings.TrimSpace(input.CommitID)
		run, _, err := store.GetRunControl(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"), r.PathValue("run_id"), credential.ID)
		if errors.Is(err, changesessions.ErrNotFound) {
			writeAPIError(w, 404, "agent_run_not_found", "agent run not found")
			return
		}
		if writeChangeSessionError(w, err) {
			return
		}
		session, sessionErr := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"))
		if writeChangeSessionError(w, sessionErr) {
			return
		}
		if session.PreviewEvidence != nil {
			reported := map[string]bool{}
			for _, criterion := range input.CompletionCriteria {
				reported[strings.TrimSpace(criterion.Criterion)] = true
			}
			for _, criterion := range session.PreviewEvidence.AcceptanceCriteria {
				if !reported[criterion] {
					writeAPIError(w, 400, "invalid_run_completion", "preview repair completion must report every frozen acceptance criterion")
					return
				}
			}
			if len(input.Commands) == 0 {
				writeAPIError(w, 400, "invalid_run_completion", "preview repair completion must retain command evidence")
				return
			}
		}
		pull, ok := loadPull(w, r)
		if !ok {
			return
		}
		if !validateRunCredential(w, credential, pull) {
			return
		}
		if pull.Status != pullrequests.Open {
			writeAPIError(w, 409, "pull_request_closed", "completed work requires an open pull request")
			return
		}
		if pull.SourceBranch != run.WorkingBranch || (pull.SourceCommitID != run.SourceCommitID && pull.SourceCommitID != input.CommitID) {
			writeAPIError(w, 409, "run_revision_conflict", "pull request has advanced beyond this run")
			return
		}
		repository, openErr := gitStore.Open(workingRepository(pull))
		if openErr != nil {
			writeAPIError(w, 500, "internal_error", "repository storage unavailable")
			return
		}
		var completed changesessions.Run
		var event changesessions.Event
		var synchronizedPull pullrequests.PullRequest
		synchronized := false
		complete := func() error {
			headHistory, historyErr := repository.ListCommitAncestry(storage.ObjectID(input.CommitID))
			if historyErr != nil {
				return changesessions.ErrInvalid
			}
			baseHistory, baseErr := repository.ListCommitAncestry(storage.ObjectID(run.SourceCommitID))
			if baseErr != nil {
				return baseErr
			}
			baseSet := map[storage.ObjectID]bool{}
			for _, commit := range baseHistory {
				baseSet[commit.ID] = true
			}
			containsBase, commits := false, []string{}
			for _, commit := range headHistory {
				if commit.ID == storage.ObjectID(run.SourceCommitID) {
					containsBase = true
				}
				if !baseSet[commit.ID] {
					commits = append(commits, string(commit.ID))
				}
			}
			if !containsBase || len(commits) == 0 {
				return changesessions.ErrInvalid
			}
			changes, changeErr := pullRequestStore.CompareCommits(workingRepository(pull), run.SourceCommitID, input.CommitID)
			if changeErr != nil {
				return changeErr
			}
			files := make([]changesessions.ChangedFile, len(changes))
			for i, change := range changes {
				files[i] = changesessions.ChangedFile{Path: change.Path, Status: change.Status}
			}
			var completionErr error
			var syncErr error
			synchronizedPull, syncErr = pullRequestStore.SynchronizeSourceAfter(r.PathValue("id"), pull.ID, func() error {
				completed, event, completionErr = store.CompleteRunWithEvidence(r.PathValue("id"), pull.ID, run.SessionID, run.ID, credential.ID, input.Summary, input.CommitID, commits, files, input.Checks, input.UnresolvedConcerns, input.Commands, input.CompletionCriteria)
				if errors.Is(completionErr, changesessions.ErrDurabilityUncertain) {
					return nil
				}
				return completionErr
			})
			if syncErr != nil && !errors.Is(syncErr, pullrequests.ErrDurabilityUncertain) {
				return syncErr
			}
			if synchronizedPull.SourceCommitID != input.CommitID {
				return changesessions.ErrInvalid
			}
			synchronized = true
			if errors.Is(completionErr, changesessions.ErrDurabilityUncertain) {
				return completionErr
			}
			if errors.Is(syncErr, pullrequests.ErrDurabilityUncertain) {
				return pullrequests.ErrDurabilityUncertain
			}
			return nil
		}
		err = repository.WithReferenceTarget("refs/heads/"+run.WorkingBranch, input.CommitID, complete)
		if completed.ID != "" && synchronized {
			required, _ := repositoriesStore.RequiredChecks(synchronizedPull.RepositoryID, synchronizedPull.TargetBranch)
			startCheckRuns(gitStore, checkRunStore, synchronizedPull, required...)
			if _, revokeErr := authStore.Revoke(run.InitiatorID, credential.ID); revokeErr != nil && !errors.Is(revokeErr, auth.ErrNotFound) {
				writeAPIError(w, 500, "internal_error", "work was published but agent access revocation must be retried")
				return
			}
			if revoked, revokeErr := store.RevokeRunAccess(r.PathValue("id"), pull.ID, run.SessionID, run.ID); revokeErr == nil || errors.Is(revokeErr, changesessions.ErrDurabilityUncertain) {
				completed = revoked
			} else {
				writeChangeSessionError(w, revokeErr)
				return
			}
		}
		var previewAttempt *previews.Preview
		if completed.ID != "" && synchronized && previewStore != nil && checkRunStore != nil && completed.Outcome != nil {
			if session, sessionErr := store.Get(r.PathValue("id"), pull.ID, run.SessionID); sessionErr == nil && session.PreviewEvidence != nil {
				if origin, originErr := previewStore.Get(pull.RepositoryID, pull.ID, session.PreviewEvidence.PreviewID); originErr == nil {
					for _, finding := range origin.Findings {
						if finding.ID == session.PreviewEvidence.FindingID {
							_, _, _ = previewStore.MutateFinding(pull.RepositoryID, pull.ID, origin.ID, finding.ID, run.InitiatorID, finding.Version, func(f *previews.Finding) error {
								if f.Repair != nil && f.Repair.SessionID == session.ID {
									f.Repair.PublishedCommitID = completed.Outcome.CommitID
								}
								return nil
							})
							break
						}
					}
				}
				if attempt, previewErr := createRepairPreviewAttempt(gitStore, checkRunStore, previewStore, synchronizedPull, run.InitiatorID, session.ID, session.PreviewEvidence.FindingID); previewErr == nil {
					previewAttempt = &attempt
					if origin, originErr := previewStore.Get(pull.RepositoryID, pull.ID, session.PreviewEvidence.PreviewID); originErr == nil {
						for _, finding := range origin.Findings {
							if finding.ID == session.PreviewEvidence.FindingID {
								_, _, _ = previewStore.MutateFinding(pull.RepositoryID, pull.ID, origin.ID, finding.ID, run.InitiatorID, finding.Version, func(f *previews.Finding) error {
									if f.Repair != nil && f.Repair.SessionID == session.ID {
										f.Repair.PublishedCommitID = completed.Outcome.CommitID
										f.Repair.PreviewAttemptID = attempt.ID
									}
									return nil
								})
								break
							}
						}
					}
				}
			}
		}
		response := map[string]any{"run": completed, "event": event, "preview_attempt": previewAttempt, "pull_request": func() pullrequests.PullRequest {
			updated, _ := pullRequestStore.Get(r.PathValue("id"), pull.ID)
			return updated
		}()}
		if errors.Is(err, storage.ErrReferenceExists) || errors.Is(err, storage.ErrReferenceNotFound) || errors.Is(err, storage.ErrReferenceLocked) {
			writeAPIError(w, 409, "branch_tip_changed", "completion must identify the published branch tip")
			return
		}
		if errors.Is(err, changesessions.ErrRunPaused) {
			writeAPIError(w, 409, "agent_run_paused", "resume the run before publishing completion")
			return
		}
		if errors.Is(err, changesessions.ErrRunCanceled) || errors.Is(err, changesessions.ErrRunCompleted) {
			writeAPIError(w, 409, "agent_run_terminal", "agent run is already terminal")
			return
		}
		if errors.Is(err, changesessions.ErrInvalid) || errors.Is(err, storage.ErrInvalidReference) {
			writeAPIError(w, 400, "invalid_run_completion", "completion must identify new descendant commits and valid review evidence")
			return
		}
		if errors.Is(err, changesessions.ErrDurabilityUncertain) || errors.Is(err, pullrequests.ErrDurabilityUncertain) {
			if completed.ID != "" {
				recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "pull_request.synchronized", ActorID: run.InitiatorID, RepositoryID: pull.RepositoryID, ResourceType: "pull_request", ResourceID: pull.ID, ResourceTitle: pull.Title, ResourceRevision: pull.SourceCommitID})
			}
			writeUncertainMutation(w, response)
			return
		}
		if writePullRequestError(w, err) || writeChangeSessionError(w, err) {
			return
		}
		w.Header().Set("Location", strings.TrimSuffix(r.URL.Path, "/completion")+"#outcome")
		recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "pull_request.synchronized", ActorID: run.InitiatorID, RepositoryID: pull.RepositoryID, ResourceType: "pull_request", ResourceID: pull.ID, ResourceTitle: pull.Title, ResourceRevision: pull.SourceCommitID})
		writeJSON(w, http.StatusCreated, response)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/sessions/{session_id}/runs/{run_id}/events", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, authStore, "git:write", false)
		if !ok {
			return
		}
		pull, ok := loadPull(w, r)
		if !ok || !validateRunCredential(w, credential, pull) {
			return
		}
		var input workEventInput
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_agent_event", "agent event is invalid")
			return
		}
		input.Kind = strings.TrimSpace(input.Kind)
		input.State = strings.TrimSpace(input.State)
		input.Message = strings.TrimSpace(input.Message)
		input.Tool = strings.TrimSpace(input.Tool)
		input.Artifact = strings.TrimSpace(input.Artifact)
		input.Branch = strings.TrimSpace(input.Branch)
		input.CommitID = strings.TrimSpace(input.CommitID)
		var repository *storage.Repository
		if input.Kind == "branch.updated" {
			var openErr error
			repository, openErr = gitStore.Open(workingRepository(pull))
			if openErr != nil {
				writeAPIError(w, 500, "internal_error", "repository storage unavailable")
				return
			}
			if _, commitErr := repository.ReadCommit(storage.ObjectID(input.CommitID)); commitErr != nil {
				writeAPIError(w, 400, "invalid_agent_event", "branch update must identify a commit")
				return
			}
		}
		var event changesessions.Event
		appendEvent := func() error {
			var appendErr error
			event, appendErr = store.AppendWorkEvent(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"), r.PathValue("run_id"), credential.ID, input.Kind, input.State, input.Message, input.Tool, input.Artifact, input.Branch, input.CommitID)
			return appendErr
		}
		var err error
		if input.Kind == "branch.updated" {
			err = repository.WithReferenceTarget("refs/heads/"+input.Branch, input.CommitID, appendEvent)
			if errors.Is(err, storage.ErrReferenceExists) || errors.Is(err, storage.ErrReferenceNotFound) || errors.Is(err, storage.ErrReferenceLocked) {
				writeAPIError(w, 400, "invalid_agent_event", "branch update must match the published branch tip")
				return
			}
		} else {
			err = appendEvent()
		}
		if errors.Is(err, changesessions.ErrDurabilityUncertain) {
			writeUncertainMutation(w, event)
			return
		}
		if errors.Is(err, changesessions.ErrNotFound) {
			writeAPIError(w, 404, "agent_run_not_found", "agent run not found")
			return
		}
		if errors.Is(err, changesessions.ErrInvalid) {
			writeAPIError(w, 400, "invalid_agent_event", "agent event fields do not match the run mandate")
			return
		}
		if errors.Is(err, changesessions.ErrRunPaused) {
			writeAPIError(w, http.StatusConflict, "agent_run_paused", "agent run is paused; inspect control state before continuing")
			return
		}
		if errors.Is(err, changesessions.ErrRunCanceled) {
			writeAPIError(w, http.StatusConflict, "agent_run_canceled", "agent run is canceled")
			return
		}
		if errors.Is(err, changesessions.ErrRunCompleted) {
			writeAPIError(w, 409, "agent_run_completed", "agent run is already completed")
			return
		}
		if writeChangeSessionError(w, err) {
			return
		}
		w.Header().Set("Location", strings.TrimSuffix(r.URL.Path, "/runs/"+r.PathValue("run_id")+"/events")+"/events#"+event.ID)
		writeJSON(w, http.StatusCreated, event)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/sessions/{session_id}/runs/{run_id}/control", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, authStore, "git:read", false)
		if !ok {
			return
		}
		pull, ok := loadPull(w, r)
		if !ok || !validateRunCredential(w, credential, pull) {
			return
		}
		run, interventions, err := store.GetRunControl(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"), r.PathValue("run_id"), credential.ID)
		if errors.Is(err, changesessions.ErrNotFound) {
			writeAPIError(w, 404, "agent_run_not_found", "agent run not found")
			return
		}
		if writeChangeSessionError(w, err) {
			return
		}
		session, sessionErr := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"))
		if writeChangeSessionError(w, sessionErr) {
			return
		}
		writeJSON(w, 200, map[string]any{"run": run, "interventions": interventions, "check_evidence": session.CheckEvidence})
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/sessions/{session_id}/runs/{run_id}/evidence/artifacts/{artifact_id}", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, authStore, "git:read", false)
		if !ok {
			return
		}
		pull, ok := loadPull(w, r)
		if !ok || !validateRunCredential(w, credential, pull) {
			return
		}
		if _, _, err := store.GetRunControl(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"), r.PathValue("run_id"), credential.ID); writeChangeSessionError(w, err) {
			return
		}
		session, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"))
		if writeChangeSessionError(w, err) {
			return
		}
		if session.CheckEvidence == nil || checkRunStore == nil {
			writeAPIError(w, 404, "check_artifact_not_found", "check artifact not found")
			return
		}
		allowed := false
		for _, artifact := range session.CheckEvidence.Artifacts {
			if artifact.ID == r.PathValue("artifact_id") {
				allowed = true
				break
			}
		}
		if !allowed {
			writeAPIError(w, 404, "check_artifact_not_found", "check artifact not found")
			return
		}
		file, artifact, err := checkRunStore.OpenArtifact(r.PathValue("id"), r.PathValue("pull_id"), session.CheckEvidence.RunID, r.PathValue("artifact_id"))
		if err != nil {
			writeAPIError(w, 404, "check_artifact_not_found", "check artifact not found")
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", artifact.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(artifact.Path)))
		http.ServeContent(w, r, path.Base(artifact.Path), artifact.CreatedAt, file)
	})
	type runInput struct {
		Instructions   string   `json:"instructions"`
		SourceCommitID string   `json:"source_commit_id"`
		ContextPaths   []string `json:"context_paths"`
		WorkingBranch  string   `json:"working_branch"`
		ExpiresIn      int64    `json:"expires_in"`
	}
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/sessions/{session_id}/runs", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		pull, ok := loadPull(w, r)
		if !ok {
			return
		}
		if pull.Status != pullrequests.Open {
			writeAPIError(w, 409, "pull_request_closed", "agent runs require an open pull request")
			return
		}
		if pull.SourceRepositoryID != pull.RepositoryID && !pull.MaintainerEditsAllowed {
			writeAPIError(w, 409, "maintainer_edits_not_allowed", "the contribution owner has not allowed participant edits")
			return
		}
		session, err := store.Get(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"))
		if writeChangeSessionError(w, err) {
			return
		}
		var input runInput
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_agent_run", "run mandate is invalid")
			return
		}
		input.Instructions = strings.TrimSpace(input.Instructions)
		input.WorkingBranch = strings.TrimSpace(input.WorkingBranch)
		if input.ExpiresIn == 0 {
			input.ExpiresIn = 3600
		}
		if input.SourceCommitID != session.SourceCommitID || len([]rune(input.Instructions)) == 0 || len([]rune(input.Instructions)) > 10000 || len(input.ContextPaths) == 0 || len(input.ContextPaths) > 50 || !validWorkingBranch(input.WorkingBranch) || input.WorkingBranch != pull.SourceBranch || input.ExpiresIn < 300 || input.ExpiresIn > 86400 {
			writeAPIError(w, 400, "invalid_agent_run", "instructions, revision, context, branch, or lifetime is invalid")
			return
		}
		repo, openErr := gitStore.Open(workingRepository(pull))
		if openErr != nil {
			writeAPIError(w, 500, "internal_error", "repository storage unavailable")
			return
		}
		commit, commitErr := repo.ReadCommit(storage.ObjectID(input.SourceCommitID))
		if commitErr != nil {
			writeAPIError(w, 500, "internal_error", "repository revision unavailable")
			return
		}
		entries, walkErr := repo.WalkTree(commit.Tree)
		if walkErr != nil {
			writeAPIError(w, 500, "internal_error", "repository context unavailable")
			return
		}
		available := map[string]bool{}
		for _, entry := range entries {
			available[entry.Path] = true
		}
		seen := map[string]bool{}
		for i, selected := range input.ContextPaths {
			clean := path.Clean(strings.TrimSpace(selected))
			if clean == "." || clean != selected || strings.HasPrefix(clean, "../") || !available[clean] || seen[clean] {
				writeAPIError(w, 400, "invalid_agent_run", "every context path must identify a unique path in the selected revision")
				return
			}
			seen[clean] = true
			input.ContextPaths[i] = clean
		}
		var issued auth.IssuedCredential
		var issueErr error
		if pull.SourceRepositoryID != pull.RepositoryID {
			issued, issueErr = authStore.IssuePullRequestBound(actor.UserID, "Agent run in session "+session.ID, []string{"git:read", "git:write"}, time.Duration(input.ExpiresIn)*time.Second, workingRepository(pull), "refs/heads/"+input.WorkingBranch, pull.ID)
		} else {
			issued, issueErr = authStore.IssueBound(actor.UserID, auth.Git, "Agent run in session "+session.ID, []string{"git:read", "git:write"}, time.Duration(input.ExpiresIn)*time.Second, workingRepository(pull), "refs/heads/"+input.WorkingBranch)
		}
		if issueErr != nil {
			writeAPIError(w, 500, "internal_error", "agent access could not be issued")
			return
		}
		run, launchErr := store.LaunchRun(r.PathValue("id"), r.PathValue("pull_id"), session.ID, actor.UserID, input.Instructions, input.SourceCommitID, input.ContextPaths, input.WorkingBranch, issued.ID, issued.ExpiresAt)
		location := r.URL.Path + "/" + run.ID
		response := map[string]any{"run": run, "credential": issued}
		if errors.Is(launchErr, changesessions.ErrDurabilityUncertain) {
			w.Header().Set("Location", location)
			writeUncertainMutation(w, response)
			return
		}
		if launchErr != nil {
			_, _ = authStore.Revoke(actor.UserID, issued.ID)
			writeChangeSessionError(w, launchErr)
			return
		}
		w.Header().Set("Location", location)
		writeJSON(w, http.StatusCreated, response)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/sessions/{session_id}/runs", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read"); !ok {
			return
		}
		if _, ok := loadPull(w, r); !ok {
			return
		}
		all, err := store.ListRuns(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"))
		if writeChangeSessionError(w, err) {
			return
		}
		page, next, valid := paginate(r, all, func(run changesessions.Run) string { return run.ID })
		if !valid {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"runs": page, "next_cursor": next})
	})
	type interventionInput struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	}
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/sessions/{session_id}/runs/{run_id}/interventions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if _, ok := loadPull(w, r); !ok {
			return
		}
		var input interventionInput
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_run_intervention", "run intervention is invalid")
			return
		}
		input.Kind = strings.TrimSpace(input.Kind)
		input.Message = strings.TrimSpace(input.Message)
		run, event, err := store.Intervene(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"), r.PathValue("run_id"), actor.UserID, input.Kind, input.Message)
		response := map[string]any{"run": run, "event": event}
		uncertain := errors.Is(err, changesessions.ErrDurabilityUncertain)
		if errors.Is(err, changesessions.ErrNotFound) {
			writeAPIError(w, 404, "agent_run_not_found", "agent run not found")
			return
		}
		if errors.Is(err, changesessions.ErrRunCanceled) {
			writeAPIError(w, 409, "agent_run_canceled", "agent run is already canceled")
			return
		}
		if errors.Is(err, changesessions.ErrInvalid) {
			writeAPIError(w, 409, "invalid_run_transition", "intervention is invalid for the current run state")
			return
		}
		if !uncertain && writeChangeSessionError(w, err) {
			return
		}
		if input.Kind == "run.canceled" {
			if _, revokeErr := authStore.Revoke(run.InitiatorID, run.CredentialID); revokeErr != nil && !errors.Is(revokeErr, auth.ErrNotFound) {
				writeAPIError(w, 500, "internal_error", "run is canceled but agent access revocation must be retried")
				return
			}
		}
		w.Header().Set("Location", strings.TrimSuffix(r.URL.Path, "/runs/"+r.PathValue("run_id")+"/interventions")+"/events#"+event.ID)
		if uncertain {
			writeUncertainMutation(w, response)
			return
		}
		writeJSON(w, http.StatusCreated, response)
	})
	mux.HandleFunc("DELETE /repositories/{id}/pulls/{pull_id}/sessions/{session_id}/runs/{run_id}/credential", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		runs, err := store.ListRuns(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"))
		if writeChangeSessionError(w, err) {
			return
		}
		var selected *changesessions.Run
		for i := range runs {
			if runs[i].ID == r.PathValue("run_id") {
				selected = &runs[i]
				break
			}
		}
		if selected == nil {
			writeAPIError(w, 404, "agent_run_not_found", "agent run not found")
			return
		}
		if _, err := authStore.Revoke(selected.InitiatorID, selected.CredentialID); err != nil && !errors.Is(err, auth.ErrNotFound) {
			writeAPIError(w, 500, "internal_error", "agent access could not be revoked")
			return
		}
		run, err := store.RevokeRunAccess(r.PathValue("id"), r.PathValue("pull_id"), r.PathValue("session_id"), selected.ID)
		if errors.Is(err, changesessions.ErrDurabilityUncertain) {
			writeUncertainMutation(w, run)
			return
		}
		if writeChangeSessionError(w, err) {
			return
		}
		writeJSON(w, 200, run)
	})
}

func validWorkingBranch(branch string) bool {
	if branch == "" || len(branch) > 200 || strings.HasPrefix(branch, ".") || strings.HasSuffix(branch, ".") || strings.HasSuffix(branch, ".lock") || strings.Contains(branch, "..") || strings.ContainsAny(branch, " ~^:?*[\\\x00\r\n") {
		return false
	}
	for _, character := range branch {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("._/-", character)) {
			return false
		}
	}
	for _, part := range strings.Split(branch, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".") {
			return false
		}
	}
	return true
}

func writeChangeSessionError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, changesessions.ErrNotFound):
		writeAPIError(w, 404, "change_session_not_found", "change session not found")
	case errors.Is(err, changesessions.ErrInvalid):
		writeAPIError(w, 400, "invalid_change_session", "change session context is invalid")
	default:
		log.Printf("change session storage: %v", err)
		writeAPIError(w, 500, "internal_error", "change session storage unavailable")
	}
	return true
}

func registerProposalRoutes(mux *http.ServeMux, gitStore *storage.Store, repositoriesStore *repositories.Store, store *proposals.Store, authStore *auth.Store, activityStore *activities.Store, userStore *users.Store) {
	mux.HandleFunc("GET /repositories/{id}/proposals", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		all, err := store.List(r.PathValue("id"))
		if writeProposalError(w, err) {
			return
		}
		page, next, ok := paginate(r, all, func(p proposals.Proposal) string { return p.ID })
		if !ok {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"proposals": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /repositories/{id}/proposals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input proposalInput
		if decodeJSON(r, &input) != nil || input.Title == nil || input.Body == nil {
			writeAPIError(w, 400, "invalid_proposal", "title and body are required")
			return
		}
		proposal, err := store.Create(r.PathValue("id"), actor.UserID, *input.Title, *input.Body)
		if errors.Is(err, proposals.ErrDurabilityUncertain) {
			location := "/repositories/" + r.PathValue("id") + "/proposals/" + proposal.ID
			w.Header().Set("Location", location)
			writeUncertainMutation(w, proposal)
			return
		}
		if writeProposalError(w, err) {
			return
		}
		recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "proposal.created", ActorID: actor.UserID, RepositoryID: proposal.RepositoryID, ResourceType: "proposal", ResourceID: proposal.ID, ResourceTitle: proposal.Title})
		recordMentions(activityStore, repositoriesStore, userStore, actor.UserID, proposal.RepositoryID, "proposal", proposal.ID, proposal.Title, proposal.Title+"\n"+proposal.Body)
		location := "/repositories/" + r.PathValue("id") + "/proposals/" + proposal.ID
		w.Header().Set("Location", location)
		writeJSON(w, 201, proposal)
	})
	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		proposal, err := store.Get(r.PathValue("id"), r.PathValue("proposal_id"))
		if writeProposalError(w, err) {
			return
		}
		writeJSON(w, 200, proposal)
	})
	mux.HandleFunc("PATCH /repositories/{id}/proposals/{proposal_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		existing, err := store.Get(r.PathValue("id"), r.PathValue("proposal_id"))
		if writeProposalError(w, err) {
			return
		}
		var input proposalPatch
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_proposal", "proposal patch is invalid")
			return
		}
		if existing.AuthorID != actor.UserID && (!owner || input.Title != nil || input.Body != nil || input.Status == nil) {
			writeAPIError(w, 404, "proposal_not_found", "proposal not found")
			return
		}
		updated, err := store.Update(r.PathValue("id"), existing.ID, proposals.Patch{Title: input.Title, Body: input.Body, Status: input.Status})
		if errors.Is(err, proposals.ErrDurabilityUncertain) {
			writeUncertainMutation(w, updated)
			return
		}
		if writeProposalError(w, err) {
			return
		}
		kind := "proposal.updated"
		if updated.Status == proposals.Closed && existing.Status != proposals.Closed {
			kind = "proposal.closed"
		}
		recordActivity(activityStore, repositoriesStore, activities.Event{Kind: kind, ActorID: actor.UserID, RepositoryID: updated.RepositoryID, ResourceType: "proposal", ResourceID: updated.ID, ResourceTitle: updated.Title})
		if input.Body != nil {
			recordMentions(activityStore, repositoriesStore, userStore, actor.UserID, updated.RepositoryID, "proposal", updated.ID, updated.Title, *input.Body)
		}
		if input.Title != nil {
			recordMentions(activityStore, repositoriesStore, userStore, actor.UserID, updated.RepositoryID, "proposal", updated.ID, updated.Title, *input.Title)
		}
		writeJSON(w, 200, updated)
	})
	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		all, err := store.ListComments(r.PathValue("id"), r.PathValue("proposal_id"))
		if writeProposalError(w, err) {
			return
		}
		page, next, ok := paginate(r, all, func(c proposals.Comment) string { return c.ID })
		if !ok {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"comments": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /repositories/{id}/proposals/{proposal_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var input commentInput
		if decodeJSON(r, &input) != nil || input.Body == nil {
			writeAPIError(w, 400, "invalid_comment", "body is required")
			return
		}
		comment, err := store.AddComment(r.PathValue("id"), r.PathValue("proposal_id"), actor.UserID, *input.Body)
		if errors.Is(err, proposals.ErrDurabilityUncertain) {
			w.Header().Set("Location", r.URL.Path+"/"+comment.ID)
			writeUncertainMutation(w, comment)
			return
		}
		if writeProposalError(w, err) {
			return
		}
		if proposal, proposalErr := store.Get(r.PathValue("id"), r.PathValue("proposal_id")); proposalErr == nil {
			recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "proposal.commented", ActorID: actor.UserID, RepositoryID: proposal.RepositoryID, ResourceType: "proposal", ResourceID: proposal.ID, ResourceTitle: proposal.Title})
			recordMentions(activityStore, repositoriesStore, userStore, actor.UserID, proposal.RepositoryID, "proposal", proposal.ID, proposal.Title, comment.Body)
		}
		w.Header().Set("Location", r.URL.Path+"/"+comment.ID)
		writeJSON(w, 201, comment)
	})
	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}/tasks", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		tasks, err := store.ListTasks(r.PathValue("id"), r.PathValue("proposal_id"))
		if writeProposalError(w, err) {
			return
		}
		writeJSON(w, 200, map[string]any{"tasks": tasks})
	})
	mux.HandleFunc("POST /repositories/{id}/proposals/{proposal_id}/tasks", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input proposalTaskInput
		if decodeJSON(r, &input) != nil || input.Title == nil || input.Outcome == nil {
			writeAPIError(w, 400, "invalid_task", "title and outcome are required")
			return
		}
		task, err := store.CreateTask(r.PathValue("id"), r.PathValue("proposal_id"), actor.UserID, *input.Title, *input.Outcome, input.DependencyIDs, input.DiscussionCommentIDs)
		location := r.URL.Path + "/" + task.ID
		if errors.Is(err, proposals.ErrDurabilityUncertain) {
			w.Header().Set("Location", location)
			writeUncertainMutation(w, task)
			return
		}
		if writeProposalError(w, err) {
			return
		}
		w.Header().Set("Location", location)
		writeJSON(w, 201, task)
	})
	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		task, err := store.GetTask(r.PathValue("id"), r.PathValue("proposal_id"), r.PathValue("task_id"))
		if writeProposalError(w, err) {
			return
		}
		writeJSON(w, 200, task)
	})
	mux.HandleFunc("PATCH /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input proposalTaskPatch
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_task", "task patch is invalid")
			return
		}
		before, _ := store.ListTasks(r.PathValue("id"), r.PathValue("proposal_id"))
		task, err := store.UpdateTask(r.PathValue("id"), r.PathValue("proposal_id"), r.PathValue("task_id"), actor.UserID, proposals.TaskPatch{Title: input.Title, Outcome: input.Outcome, Status: input.Status, Position: input.Position, DependencyIDs: input.DependencyIDs, DiscussionCommentIDs: input.DiscussionCommentIDs})
		if errors.Is(err, proposals.ErrDurabilityUncertain) {
			after, _ := store.ListTasks(r.PathValue("id"), r.PathValue("proposal_id"))
			recordTaskTransitions(activityStore, repositoriesStore, actor.UserID, r.PathValue("id"), r.PathValue("proposal_id"), before, after)
			writeUncertainMutation(w, task)
			return
		}
		if writeProposalError(w, err) {
			return
		}
		after, _ := store.ListTasks(r.PathValue("id"), r.PathValue("proposal_id"))
		recordTaskTransitions(activityStore, repositoriesStore, actor.UserID, r.PathValue("id"), r.PathValue("proposal_id"), before, after)
		writeJSON(w, 200, task)
	})
	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/history", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoriesStore, authStore, r.PathValue("id")); !ok {
			return
		}
		changes, err := store.ListTaskChanges(r.PathValue("id"), r.PathValue("proposal_id"), r.PathValue("task_id"))
		if writeProposalError(w, err) {
			return
		}
		writeJSON(w, 200, map[string]any{"history": changes})
	})
	mux.HandleFunc("PUT /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/assignment", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input proposalTaskAssignmentInput
		if decodeJSON(r, &input) != nil || input.RepositoryID != r.PathValue("id") || (input.AssigneeType != "human" && input.AssigneeType != "agent") {
			writeAPIError(w, 400, "invalid_task_assignment", "assignee, mandate, repository, and base revision are required")
			return
		}
		assigneeID := ""
		if input.AssigneeID != nil {
			assigneeID = *input.AssigneeID
		}
		if input.AssigneeType == "human" {
			if _, err := userStore.Get(assigneeID); err != nil {
				writeAPIError(w, 400, "invalid_task_assignee", "human assignee does not exist")
				return
			}
		}
		var task proposals.Task
		assign := func() error {
			gitRepository, err := gitStore.Open(input.RepositoryID)
			if err != nil {
				return repositories.ErrNotFound
			}
			if _, err := gitRepository.ReadCommit(storage.ObjectID(strings.ToLower(input.BaseRevision))); err != nil {
				return storage.ErrObjectNotFound
			}
			task, err = store.AssignTask(r.PathValue("id"), r.PathValue("proposal_id"), r.PathValue("task_id"), actor.UserID, proposals.TaskAssignmentInput{AssigneeType: input.AssigneeType, AssigneeID: assigneeID, Mandate: input.Mandate, RepositoryID: input.RepositoryID, BaseRevision: input.BaseRevision, ExpectedAssignmentID: input.ExpectedAssignmentID})
			return err
		}
		var err error
		if input.AssigneeType == "human" {
			err = repositoriesStore.WithCurrentParticipant(assigneeID, input.RepositoryID, assign)
		} else {
			err = assign()
		}
		if errors.Is(err, repositories.ErrInvalidCollaborator) {
			writeAPIError(w, 400, "invalid_task_assignee", "human assignee must be a current repository participant")
			return
		}
		if errors.Is(err, repositories.ErrNotFound) {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		if errors.Is(err, storage.ErrObjectNotFound) || errors.Is(err, storage.ErrInvalidObject) || errors.Is(err, storage.ErrCorruptObject) {
			writeAPIError(w, 400, "invalid_base_revision", "base revision must be an existing commit")
			return
		}
		if errors.Is(err, proposals.ErrTaskAssignmentConflict) {
			writeAPIError(w, 409, "task_assignment_conflict", "task ownership changed; reload before claiming or reassigning")
			return
		}
		if errors.Is(err, proposals.ErrDurabilityUncertain) {
			writeUncertainMutation(w, task)
			return
		}
		if writeProposalError(w, err) {
			return
		}
		writeJSON(w, 200, task)
	})
	mux.HandleFunc("DELETE /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/assignment", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		expected := r.URL.Query().Get("expected_assignment_id")
		task, err := store.RevokeTaskAssignment(r.PathValue("id"), r.PathValue("proposal_id"), r.PathValue("task_id"), actor.UserID, expected)
		if errors.Is(err, proposals.ErrTaskAssignmentConflict) {
			writeAPIError(w, 409, "task_assignment_conflict", "task ownership changed; reload before revoking")
			return
		}
		if errors.Is(err, proposals.ErrDurabilityUncertain) {
			writeUncertainMutation(w, task)
			return
		}
		if writeProposalError(w, err) {
			return
		}
		writeJSON(w, 200, task)
	})
	mux.HandleFunc("POST /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/rebase", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoriesStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input proposalTaskRebaseInput
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_task_rebase", "exact base revision and expected assignment are required")
			return
		}
		gitRepository, err := gitStore.Open(r.PathValue("id"))
		if err == nil {
			_, err = gitRepository.ReadCommit(storage.ObjectID(strings.ToLower(input.BaseRevision)))
		}
		if errors.Is(err, storage.ErrObjectNotFound) || errors.Is(err, storage.ErrInvalidObject) || errors.Is(err, storage.ErrCorruptObject) {
			writeAPIError(w, 400, "invalid_base_revision", "base revision must be an existing commit")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "storage_error", "repository storage is unavailable")
			return
		}
		before, _ := store.ListTasks(r.PathValue("id"), r.PathValue("proposal_id"))
		task, err := store.RebaseTaskAssignment(r.PathValue("id"), r.PathValue("proposal_id"), r.PathValue("task_id"), actor.UserID, proposals.TaskRebaseInput{BaseRevision: input.BaseRevision, ExpectedAssignmentID: input.ExpectedAssignmentID})
		if errors.Is(err, proposals.ErrTaskAssignmentConflict) {
			writeAPIError(w, 409, "task_assignment_conflict", "task ownership or lifecycle changed; reload before rebasing")
			return
		}
		if errors.Is(err, proposals.ErrDurabilityUncertain) {
			after, _ := store.ListTasks(r.PathValue("id"), r.PathValue("proposal_id"))
			recordTaskTransitions(activityStore, repositoriesStore, actor.UserID, r.PathValue("id"), r.PathValue("proposal_id"), before, after)
			writeUncertainMutation(w, task)
			return
		}
		if writeProposalError(w, err) {
			return
		}
		after, _ := store.ListTasks(r.PathValue("id"), r.PathValue("proposal_id"))
		recordTaskTransitions(activityStore, repositoriesStore, actor.UserID, r.PathValue("id"), r.PathValue("proposal_id"), before, after)
		writeJSON(w, 200, task)
	})
}

func recordTaskTransitions(activityStore *activities.Store, repositoriesStore *repositories.Store, actorID, repositoryID, proposalID string, before, after []proposals.Task) {
	prior := make(map[string]proposals.Task, len(before))
	for _, task := range before {
		prior[task.ID] = task
	}
	for _, task := range after {
		if task.Assignment == nil || task.Assignment.AssigneeType != "human" {
			continue
		}
		old, existed := prior[task.ID]
		kind := ""
		switch {
		case existed && old.ContextState != task.ContextState && task.ContextState == "obsolete":
			kind = "task.obsolete"
		case existed && old.ContextState != task.ContextState && task.ContextState == "changed":
			kind = "task.changed"
		case existed && !old.Ready && task.Ready:
			kind = "task.ready"
		case existed && old.Ready && !task.Ready:
			kind = "task.blocked"
		}
		if kind == "" {
			continue
		}
		target := task.Assignment.AssigneeID
		recordActivity(activityStore, repositoriesStore, activities.Event{Kind: kind, ActorID: actorID, RepositoryID: repositoryID, ResourceType: "proposal", ResourceID: proposalID, ResourceTitle: task.Title, TargetUserID: &target})
	}
}

func writeUncertainMutation(w http.ResponseWriter, resource any) {
	w.Header().Set("Vivarium-Durability", "uncertain")
	writeJSON(w, http.StatusAccepted, resource)
}

func recordActivity(activityStore *activities.Store, repositoriesStore *repositories.Store, event activities.Event) {
	if activityStore == nil {
		return
	}
	repository, err := repositoriesStore.GetByID(event.RepositoryID)
	if err != nil {
		log.Printf("resolve repository for activity: %v", err)
		return
	}
	event.RepositoryName = repository.Name
	if _, err := activityStore.Append(event); err != nil {
		log.Printf("record activity: %v", err)
	}
}

func recordActivityOnce(activityStore *activities.Store, repositoriesStore *repositories.Store, key string, event activities.Event) error {
	if activityStore == nil {
		return nil
	}
	repository, err := repositoriesStore.GetByID(event.RepositoryID)
	if err != nil {
		return err
	}
	event.RepositoryName = repository.Name
	_, err = activityStore.AppendOnce(key, event)
	return err
}

func recordMentions(activityStore *activities.Store, repositoriesStore *repositories.Store, userStore *users.Store, actorID, repositoryID, resourceType, resourceID, resourceTitle, body string) {
	if activityStore == nil || userStore == nil {
		return
	}
	seen := map[string]bool{}
	for _, word := range strings.Fields(body) {
		handle := strings.Trim(strings.TrimPrefix(word, "@"), ".,;:!?()[]{}<>\"'")
		if !strings.HasPrefix(word, "@") || handle == "" {
			continue
		}
		user, err := userStore.FindByHandle(handle)
		if err != nil || user.ID == actorID || seen[user.ID] {
			continue
		}
		seen[user.ID] = true
		target := user.ID
		recordActivity(activityStore, repositoriesStore, activities.Event{Kind: "mention.created", ActorID: actorID, RepositoryID: repositoryID, ResourceType: resourceType, ResourceID: resourceID, ResourceTitle: resourceTitle, TargetUserID: &target})
	}
}

func registerActivityRoutes(mux *http.ServeMux, repositoryStore *repositories.Store, activityStore *activities.Store, authStore *auth.Store) {
	mux.HandleFunc("GET /activity", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:read", false)
		if !ok {
			return
		}
		all, err := activityStore.List()
		if err != nil {
			log.Printf("activity storage: %v", err)
			writeAPIError(w, 500, "internal_error", "activity storage unavailable")
			return
		}
		visible := make([]activities.Event, 0, len(all))
		for _, event := range all {
			repository, repoErr := repositoryStore.GetByID(event.RepositoryID)
			if repoErr != nil {
				continue
			}
			if repository.OwnerID == actor.UserID {
				visible = append(visible, event)
				continue
			}
			collaborator, collaboratorErr := repositoryStore.HasCollaborator(actor.UserID, event.RepositoryID)
			if collaboratorErr == nil && collaborator {
				visible = append(visible, event)
			}
		}
		page, next, valid := paginate(r, visible, func(event activities.Event) string { return event.ID })
		if !valid {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"events": page, "next_cursor": next})
	})
}

type inboxItem struct {
	activities.Event
	Category string `json:"category"`
	Action   string `json:"action"`
}

func registerInboxRoutes(mux *http.ServeMux, repositoryStore *repositories.Store, proposalStore *proposals.Store, pullRequestStore *pullrequests.Store, incidentStore *incidents.Store, activityStore *activities.Store, authStore *auth.Store) {
	mux.HandleFunc("GET /inbox", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:read", false)
		if !ok {
			return
		}
		category := r.URL.Query().Get("category")
		if category != "" && category != "review" && category != "response" && category != "awareness" {
			writeAPIError(w, 400, "invalid_inbox_category", "category must be review, response, or awareness")
			return
		}
		items, err := buildInbox(actor.UserID, repositoryStore, proposalStore, pullRequestStore, incidentStore, activityStore, false)
		if err != nil {
			log.Printf("inbox storage: %v", err)
			writeAPIError(w, 500, "internal_error", "inbox unavailable")
			return
		}
		if category != "" {
			filtered := items[:0]
			for _, item := range items {
				if item.Category == category {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		page, next, valid := paginate(r, items, func(item inboxItem) string { return item.ID })
		if !valid {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"items": page, "next_cursor": next})
	})

	mux.HandleFunc("DELETE /inbox/{event_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:read", false)
		if !ok {
			return
		}
		items, err := buildInbox(actor.UserID, repositoryStore, proposalStore, pullRequestStore, incidentStore, activityStore, true)
		if err != nil {
			log.Printf("inbox storage: %v", err)
			writeAPIError(w, 500, "internal_error", "inbox unavailable")
			return
		}
		found := false
		for _, item := range items {
			if item.ID == r.PathValue("event_id") {
				found = true
				break
			}
		}
		if !found {
			writeAPIError(w, 404, "inbox_item_not_found", "inbox item not found")
			return
		}
		if err := activityStore.Clear(actor.UserID, r.PathValue("event_id")); err != nil {
			log.Printf("clear inbox item: %v", err)
			writeAPIError(w, 500, "internal_error", "inbox item could not be cleared")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func buildInbox(userID string, repositoryStore *repositories.Store, proposalStore *proposals.Store, pullRequestStore *pullrequests.Store, incidentStore *incidents.Store, activityStore *activities.Store, includeCleared bool) ([]inboxItem, error) {
	events, err := activityStore.List()
	if err != nil {
		return nil, err
	}
	cleared, err := activityStore.Cleared(userID)
	if err != nil {
		return nil, err
	}
	items := make([]inboxItem, 0)
	seenReviews := make(map[string]bool)
	for _, event := range events {
		repository, err := repositoryStore.GetByID(event.RepositoryID)
		if err != nil {
			continue
		}
		if repository.OwnerID != userID {
			collaborator, err := repositoryStore.HasCollaborator(userID, event.RepositoryID)
			if err != nil || !collaborator {
				continue
			}
		}
		category, action, err := classifyInboxEvent(userID, repository.OwnerID, event, proposalStore, pullRequestStore, incidentStore)
		if err != nil {
			return nil, err
		}
		if category == "review" {
			key := event.RepositoryID + "/" + event.ResourceID
			if seenReviews[key] {
				continue
			}
			seenReviews[key] = true
		}
		// Deduplicate before applying clear state so clearing the newest review
		// action cannot reveal an obsolete event for the same pull request.
		if !includeCleared && cleared[event.ID] {
			continue
		}
		if category != "" {
			items = append(items, inboxItem{Event: event, Category: category, Action: action})
		}
	}
	return items, nil
}

func classifyInboxEvent(userID, ownerID string, event activities.Event, proposalStore *proposals.Store, pullRequestStore *pullrequests.Store, incidentStore *incidents.Store) (string, string, error) {
	if event.ActorID == userID {
		return "", "", nil
	}
	if event.Kind == "incident.commitment_assigned" && event.TargetUserID != nil && *event.TargetUserID == userID && incidentStore != nil {
		incident, err := incidentStore.Get(event.ResourceID)
		if errors.Is(err, incidents.ErrNotFound) {
			return "", "", nil
		}
		if err != nil {
			return "", "", err
		}
		for _, commitment := range incident.Commitments {
			if commitment.RepositoryID != event.RepositoryID || commitment.AssigneeID != userID {
				continue
			}
			task, err := proposalStore.GetTask(commitment.RepositoryID, commitment.ProposalID, commitment.TaskID)
			if err != nil || task.Assignment == nil || task.Assignment.AssigneeID != userID || task.ContextState != "current" || task.Status == proposals.TaskCancelled {
				return "response", "Repair invalidated incident commitment", nil
			}
			if task.Status == proposals.TaskCompleted {
				return "", "", nil
			}
			if time.Now().After(commitment.DueAt) {
				return "response", "Resolve overdue incident commitment", nil
			}
			return "response", "Complete incident corrective work", nil
		}
		return "", "", nil
	}
	if event.Kind == "mention.created" && event.TargetUserID != nil && *event.TargetUserID == userID {
		return "response", "Respond to mention", nil
	}
	if strings.HasPrefix(event.Kind, "stewardship_opportunity.") && event.TargetUserID != nil && *event.TargetUserID == userID {
		if event.Kind == "stewardship_opportunity.approve" {
			return "response", "Plan accepted stewardship follow-up", nil
		}
		return "awareness", "Review stewardship decision", nil
	}
	if strings.HasPrefix(event.Kind, "access.") && event.TargetUserID != nil && *event.TargetUserID == userID {
		return "awareness", "Review repository access", nil
	}
	if event.Kind == "security_advisory_published" && event.TargetUserID != nil && *event.TargetUserID == userID {
		return "awareness", "Upgrade affected software", nil
	}
	if event.Kind == "package.recovery_required" && event.TargetUserID != nil && *event.TargetUserID == userID {
		return "response", "Contain unsafe dependency exposure", nil
	}
	if event.ResourceType == "deployment" && event.TargetUserID != nil && *event.TargetUserID == userID {
		switch event.Kind {
		case "deployment.pause", "deployment.mark_unsuccessful":
			return "response", "Review deployment intervention", nil
		case "deployment.resume", "deployment.cancel":
			return "awareness", "Review rollout decision", nil
		}
	}
	if event.ResourceType == "pull_request" && pullRequestStore != nil {
		pull, err := pullRequestStore.Get(event.RepositoryID, event.ResourceID)
		if errors.Is(err, pullrequests.ErrNotFound) {
			return "", "", nil
		}
		if err != nil {
			return "", "", err
		}
		switch event.Kind {
		case "integration_queue.failed", "integration_queue.pause", "integration_queue.remove", "integration_queue.retry":
			if event.TargetUserID != nil && *event.TargetUserID == userID && pull.Status == pullrequests.Open {
				return "response", "Review integration queue outcome", nil
			}
		case "integration_queue.enqueued", "integration_queue.reprioritize", "integration_queue.resume":
			if event.TargetUserID != nil && *event.TargetUserID == userID && pull.Status == pullrequests.Open {
				return "awareness", "View integration queue", nil
			}
		case "pull_request.created", "pull_request.synchronized":
			if ownerID == userID && pull.Status == pullrequests.Open {
				return "review", "Review pull request", nil
			}
		case "pull_request.commented", "review.changes_requested":
			if pull.AuthorID == userID && pull.Status == pullrequests.Open {
				return "response", "Respond to feedback", nil
			}
		case "review.approved":
			if pull.AuthorID == userID {
				return "awareness", "Review approval", nil
			}
		case "pull_request.merged":
			if pull.AuthorID == userID {
				return "awareness", "Review merge outcome", nil
			}
		}
	}
	if event.ResourceType == "proposal" && proposalStore != nil {
		proposal, err := proposalStore.Get(event.RepositoryID, event.ResourceID)
		if errors.Is(err, proposals.ErrNotFound) {
			return "", "", nil
		}
		if err != nil {
			return "", "", err
		}
		if event.TargetUserID != nil && *event.TargetUserID == userID && proposal.Status == proposals.Open {
			switch event.Kind {
			case "task.ready":
				return "response", "Start ready task", nil
			case "task.blocked":
				return "awareness", "Review blocked task", nil
			case "task.changed":
				return "response", "Rebase changed task", nil
			case "task.obsolete":
				return "response", "Replace obsolete contribution", nil
			}
		}
		if event.Kind == "proposal.commented" && proposal.AuthorID == userID && proposal.Status == proposals.Open {
			return "response", "Respond to proposal feedback", nil
		}
		if event.Kind == "proposal.closed" && proposal.AuthorID == userID {
			return "awareness", "Review proposal outcome", nil
		}
	}
	return "", "", nil
}

func registerReleaseRoutes(mux *http.ServeMux, gitStore *storage.Store, repositoryStore *repositories.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, releaseStore *releases.Store, authStore *auth.Store, buildStore *checkruns.Store) {
	mux.HandleFunc("GET /repositories/{id}/releases", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoryStore, authStore, r.PathValue("id")); !ok {
			return
		}
		items, err := releaseStore.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "release_read_failed", "release candidates could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"releases": items})
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoryStore, authStore, r.PathValue("id")); !ok {
			return
		}
		item, err := releaseStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if errors.Is(err, releases.ErrNotFound) {
			writeAPIError(w, 404, "release_not_found", "release candidate not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "release_read_failed", "release candidate could not be read")
			return
		}
		writeJSON(w, 200, item)
	})
	mux.HandleFunc("POST /repositories/{id}/releases", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoryStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Version           string  `json:"version"`
			Notes             string  `json:"notes"`
			CommitID          string  `json:"commit_id"`
			PreviousReleaseID *string `json:"previous_release_id"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		repository, err := gitStore.Open(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		if _, err = repository.ReadCommit(storage.ObjectID(input.CommitID)); err != nil {
			writeAPIError(w, 422, "invalid_release_commit", "commit_id must name a verified commit in this repository")
			return
		}
		candidate := releases.Candidate{RepositoryID: r.PathValue("id"), Version: input.Version, Notes: input.Notes, CommitID: input.CommitID, PreviousReleaseID: input.PreviousReleaseID, CreatedBy: actor.UserID}
		if input.PreviousReleaseID != nil {
			previous, previousErr := releaseStore.Get(r.PathValue("id"), *input.PreviousReleaseID)
			if previousErr != nil {
				writeAPIError(w, 422, "invalid_previous_release", "previous_release_id must name a release in this repository")
				return
			}
			candidate.PreviousCommitID = &previous.CommitID
		}
		info, inspectErr := repository.Inspect()
		if inspectErr != nil {
			writeAPIError(w, 500, "release_context_unavailable", "release branch context could not be determined")
			return
		}
		candidate.TargetBranch = info.DefaultBranch
		candidate.ChangedPaths, err = deriveReleaseChangedPaths(repository, candidate.CommitID, candidate.PreviousCommitID)
		if err != nil {
			writeAPIError(w, 422, "invalid_release_range", err.Error())
			return
		}
		candidate.Inclusions, err = deriveReleaseInclusions(repository, candidate.CommitID, candidate.PreviousCommitID, proposalStore, pullStore, candidate.RepositoryID)
		if err != nil {
			writeAPIError(w, 422, "invalid_release_range", err.Error())
			return
		}
		created, err := releaseStore.Create(candidate)
		if errors.Is(err, releases.ErrVersionExists) {
			writeAPIError(w, 409, "release_version_exists", "release version already exists")
			return
		}
		if errors.Is(err, releases.ErrInvalid) {
			writeAPIError(w, 422, "invalid_release", "version, notes, or release identity is invalid")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "release_create_failed", "release candidate could not be created")
			return
		}
		w.Header().Set("Location", "/repositories/"+created.RepositoryID+"/releases/"+created.ID)
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("POST /repositories/{id}/releases/{release_id}/builds", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoryStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		candidate, err := releaseStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if errors.Is(err, releases.ErrNotFound) {
			writeAPIError(w, 404, "release_not_found", "release candidate not found")
			return
		}
		if err != nil || buildStore == nil {
			writeAPIError(w, 500, "release_build_unavailable", "release build storage unavailable")
			return
		}
		if ready, readyErr := releaseStore.ProvenanceReady(candidate); readyErr != nil || !ready {
			writeAPIError(w, 409, "release_provenance_required", "current blocking-free provenance evidence is required before release builds")
			return
		}
		repository, err := gitStore.Open(candidate.RepositoryID)
		if err != nil {
			writeAPIError(w, 500, "release_build_unavailable", "release repository unavailable")
			return
		}
		body, err := exec.Command("git", "--git-dir="+repository.Path(), "show", candidate.CommitID+":"+checkruns.ReleaseConfigPath).Output()
		if err != nil {
			writeAPIError(w, 422, "release_definition_missing", "the release commit must contain .vivarium/release.json")
			return
		}
		config, err := checkruns.ParseReleaseConfig(body)
		if err != nil {
			writeAPIError(w, 422, "invalid_release_definition", err.Error())
			return
		}
		runs, err := buildStore.CreateRequested(candidate.RepositoryID, candidate.ID, candidate.CommitID, config.Steps, actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "release_build_failed", "release build could not be created")
			return
		}
		for _, run := range runs {
			go buildStore.Execute(run, repository.Path())
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"builds": runs})
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}/builds", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoryStore, authStore, r.PathValue("id")); !ok {
			return
		}
		if _, err := releaseStore.Get(r.PathValue("id"), r.PathValue("release_id")); errors.Is(err, releases.ErrNotFound) {
			writeAPIError(w, 404, "release_not_found", "release candidate not found")
			return
		} else if err != nil || buildStore == nil {
			writeAPIError(w, 500, "release_build_unavailable", "release builds unavailable")
			return
		}
		runs, err := buildStore.List(r.PathValue("id"), r.PathValue("release_id"))
		if err != nil {
			writeAPIError(w, 500, "release_build_unavailable", "release builds unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"builds": runs})
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}/attestation", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoryStore, authStore, r.PathValue("id")); !ok {
			return
		}
		candidate, err := releaseStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if errors.Is(err, releases.ErrNotFound) {
			writeAPIError(w, 404, "release_not_found", "release candidate not found")
			return
		}
		if err != nil || buildStore == nil {
			writeAPIError(w, 500, "release_build_unavailable", "release attestation unavailable")
			return
		}
		runs, err := buildStore.List(candidate.RepositoryID, candidate.ID)
		if err != nil {
			writeAPIError(w, 500, "release_build_unavailable", "release attestation unavailable")
			return
		}
		state := "pending"
		if len(runs) == 0 {
			state = "unbuilt"
		} else {
			state = "verified"
			for _, run := range runs {
				if run.State == "failed" || run.State == "canceled" {
					state = "failed"
					break
				}
				if run.State != "succeeded" {
					state = "pending"
				}
			}
		}
		writeJSON(w, 200, map[string]any{"version": 1, "release_id": candidate.ID, "repository_id": candidate.RepositoryID, "source_commit": candidate.CommitID, "state": state, "builds": runs})
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}/builds/{build_id}/attestation", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoryStore, authStore, r.PathValue("id")); !ok {
			return
		}
		candidate, err := releaseStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if errors.Is(err, releases.ErrNotFound) {
			writeAPIError(w, 404, "release_not_found", "release candidate not found")
			return
		}
		run, err := buildStore.Get(r.PathValue("id"), r.PathValue("release_id"), r.PathValue("build_id"))
		if errors.Is(err, checkruns.ErrNotFound) {
			writeAPIError(w, 404, "release_build_not_found", "release build not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "release_build_unavailable", "release attestation unavailable")
			return
		}
		actor := candidate.CreatedBy
		if run.RequestedBy != "" {
			actor = run.RequestedBy
		}
		for i := len(run.Attempts) - 1; i >= 0; i-- {
			if run.Attempts[i].ActorID != "" {
				actor = run.Attempts[i].ActorID
				break
			}
		}
		writeJSON(w, 200, map[string]any{"version": 1, "release_id": candidate.ID, "repository_id": candidate.RepositoryID, "source_commit": run.CommitID, "build_id": run.ID, "step": run.Definition.Name, "command": run.Definition.Command, "dependencies": []string{run.Definition.Image}, "actor_id": actor, "verification": map[string]any{"state": run.State, "exit_code": run.ExitCode, "failure": run.Failure, "attempts": run.Attempts}, "artifacts": run.Artifacts, "created_at": run.CreatedAt, "completed_at": run.CompletedAt})
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}/builds/{build_id}/events", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoryStore, authStore, r.PathValue("id")); !ok {
			return
		}
		events, err := buildStore.Events(r.PathValue("id"), r.PathValue("release_id"), r.PathValue("build_id"), 0)
		if errors.Is(err, checkruns.ErrNotFound) {
			writeAPIError(w, 404, "release_build_not_found", "release build not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "release_build_unavailable", "release build logs unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"events": events})
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}/builds/{build_id}/artifacts/{artifact_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repositoryStore, authStore, r.PathValue("id")); !ok {
			return
		}
		file, artifact, err := buildStore.OpenArtifact(r.PathValue("id"), r.PathValue("release_id"), r.PathValue("build_id"), r.PathValue("artifact_id"))
		if errors.Is(err, checkruns.ErrNotFound) {
			writeAPIError(w, 404, "release_artifact_not_found", "release artifact not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "release_build_unavailable", "release artifact unavailable")
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", artifact.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(artifact.Path)))
		http.ServeContent(w, r, path.Base(artifact.Path), artifact.CreatedAt, file)
	})
	mux.HandleFunc("POST /repositories/{id}/releases/{release_id}/builds/{build_id}/rerun", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositoryStore, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		candidate, err := releaseStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if errors.Is(err, releases.ErrNotFound) {
			writeAPIError(w, 404, "release_not_found", "release candidate not found")
			return
		}
		repository, err := gitStore.Open(candidate.RepositoryID)
		if err != nil {
			writeAPIError(w, 500, "release_build_unavailable", "release repository unavailable")
			return
		}
		run, err := buildStore.Rerun(candidate.RepositoryID, candidate.ID, r.PathValue("build_id"), actor.UserID)
		if errors.Is(err, checkruns.ErrInvalidState) {
			writeAPIError(w, 409, "release_build_active", "an active release build cannot be rerun")
			return
		}
		if errors.Is(err, checkruns.ErrNotFound) {
			writeAPIError(w, 404, "release_build_not_found", "release build not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "release_build_failed", "release build could not be rerun")
			return
		}
		go buildStore.Execute(run, repository.Path())
		writeJSON(w, http.StatusAccepted, run)
	})
}

func deriveReleaseChangedPaths(repository *storage.Repository, commitID string, previousCommitID *string) ([]string, error) {
	// With no predecessor, every path in the candidate snapshot is part of the
	// first release context, not merely the paths changed by its final commit.
	args := []string{"--git-dir=" + repository.Path(), "ls-tree", "-r", "--name-only", commitID, "--"}
	if previousCommitID != nil {
		args = []string{"--git-dir=" + repository.Path(), "diff", "--name-only", *previousCommitID, commitID, "--"}
	}
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, errors.New("release changed paths could not be derived from the exact release range")
	}
	paths := []string{}
	for _, value := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if value != "" {
			paths = append(paths, value)
		}
	}
	return paths, nil
}

func deriveReleaseInclusions(repository *storage.Repository, commitID string, previousCommitID *string, proposalStore *proposals.Store, pullStore *pullrequests.Store, repositoryID string) (releases.Inclusion, error) {
	ancestry, err := repository.ListCommitAncestry(storage.ObjectID(commitID))
	if err != nil {
		return releases.Inclusion{}, errors.New("release commit history is invalid")
	}
	rangeCommits := map[string]bool{}
	for _, commit := range ancestry {
		rangeCommits[string(commit.ID)] = true
	}
	if previousCommitID != nil {
		if !rangeCommits[*previousCommitID] {
			return releases.Inclusion{}, errors.New("previous release commit is not an ancestor of commit_id")
		}
		previous, err := repository.ListCommitAncestry(storage.ObjectID(*previousCommitID))
		if err != nil {
			return releases.Inclusion{}, errors.New("previous release history is invalid")
		}
		for _, commit := range previous {
			delete(rangeCommits, string(commit.ID))
		}
	}
	result := releases.Inclusion{PullRequestIDs: []string{}, PullEvidence: []releases.PullEvidence{}, ProposalIDs: []string{}, TaskIDs: []string{}, ContributorIDs: []string{}}
	if pullStore == nil {
		return result, nil
	}
	pulls, err := pullStore.List(repositoryID)
	if err != nil {
		return result, err
	}
	proposalIDs, taskIDs, contributorIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, pull := range pulls {
		if pull.Status != pullrequests.Merged || pull.MergeCommitID == nil || !rangeCommits[*pull.MergeCommitID] {
			continue
		}
		result.PullRequestIDs = append(result.PullRequestIDs, pull.ID)
		changes, changeErr := pullStore.Changes(repositoryID, pull.ID)
		if changeErr != nil {
			return releases.Inclusion{}, changeErr
		}
		paths := make([]string, 0, len(changes))
		for _, change := range changes {
			paths = append(paths, change.Path)
		}
		result.PullEvidence = append(result.PullEvidence, releases.PullEvidence{PullRequestID: pull.ID, SourceCommitID: pull.SourceCommitID, ChangedPaths: paths})
		contributorIDs[pull.AuthorID] = true
		if pull.MergedBy != nil {
			contributorIDs[*pull.MergedBy] = true
		}
		if pull.ProposalID != nil {
			proposalIDs[*pull.ProposalID] = true
		}
		if pull.TaskID != nil {
			taskIDs[*pull.TaskID] = true
		}
	}
	if proposalStore != nil {
		for id := range proposalIDs {
			if proposal, err := proposalStore.Get(repositoryID, id); err == nil {
				contributorIDs[proposal.AuthorID] = true
				if tasks, err := proposalStore.ListTasks(repositoryID, id); err == nil {
					for _, task := range tasks {
						if taskIDs[task.ID] {
							contributorIDs[task.CreatedBy] = true
							contributorIDs[task.UpdatedBy] = true
							if task.Assignment != nil && task.Assignment.AssigneeType == "human" {
								contributorIDs[task.Assignment.AssigneeID] = true
							}
						}
					}
				}
			}
		}
	}
	for id := range proposalIDs {
		result.ProposalIDs = append(result.ProposalIDs, id)
	}
	for id := range taskIDs {
		result.TaskIDs = append(result.TaskIDs, id)
	}
	for id := range contributorIDs {
		result.ContributorIDs = append(result.ContributorIDs, id)
	}
	return result, nil
}

func authorizeRepositoryRead(w http.ResponseWriter, r *http.Request, store *repositories.Store, authStore *auth.Store, id string) (auth.Credential, bool, bool) {
	repository, err := store.GetByID(id)
	if writeRepositoryError(w, err) {
		return auth.Credential{}, false, false
	}
	presented := r.Header.Get("Authorization") != ""
	if _, cookieErr := r.Cookie("vivarium_session"); cookieErr == nil {
		presented = true
	}
	actor, authenticated, ok := auth.Credential{}, false, true
	if repository.Visibility == repositories.Public && presented {
		var authenticateErr error
		actor, authenticated, authenticateErr = authenticateOptionalCredential(r, authStore, "repositories:read")
		if authenticateErr != nil && !errors.Is(authenticateErr, auth.ErrNotFound) {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "credential storage unavailable")
			return auth.Credential{}, false, false
		}
	} else if repository.Visibility != repositories.Public {
		actor, authenticated, ok = authenticateOptionalRequest(w, r, authStore, "repositories:read", false)
	}
	if !ok {
		return auth.Credential{}, false, false
	}
	if authenticated && actor.RepositoryID != "" && actor.RepositoryID != id {
		writeAPIError(w, 404, "repository_not_found", "repository not found")
		return auth.Credential{}, false, false
	}
	if repository.Visibility == repositories.Public {
		return actor, authenticated, true
	}
	if !authenticated {
		writeAuthenticationRequired(w, false)
		return auth.Credential{}, false, false
	}
	collaborator, err := store.HasCollaborator(actor.UserID, id)
	if err != nil {
		writeRepositoryError(w, err)
		return auth.Credential{}, false, false
	}
	if actor.UserID != repository.OwnerID && !collaborator {
		writeAPIError(w, 404, "repository_not_found", "repository not found")
		return auth.Credential{}, false, false
	}
	return actor, true, true
}

func authorizeRepositoryParticipant(w http.ResponseWriter, r *http.Request, store *repositories.Store, authStore *auth.Store, id, scope string) (auth.Credential, bool, bool) {
	actor, ok := authenticateRequest(w, r, authStore, scope, false)
	if !ok {
		return auth.Credential{}, false, false
	}
	repository, err := store.GetByID(id)
	if writeRepositoryError(w, err) {
		return auth.Credential{}, false, false
	}
	owner := actor.UserID == repository.OwnerID
	collaborator, err := store.HasCollaborator(actor.UserID, id)
	if err != nil {
		writeRepositoryError(w, err)
		return auth.Credential{}, false, false
	}
	if !owner && !collaborator {
		if actor.OrganizationID != "" && actor.OrganizationID == repository.OrganizationID && actor.AccessGrantID != "" && actor.AgentID != "" && actor.RepositoryID == id {
			return actor, false, true
		}
		writeAPIError(w, 404, "repository_not_found", "repository not found")
		return auth.Credential{}, false, false
	}
	return actor, owner, true
}

func writeProposalError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, proposals.ErrNotFound) {
		writeAPIError(w, 404, "proposal_not_found", "proposal not found")
	} else if errors.Is(err, proposals.ErrInvalid) {
		writeAPIError(w, 400, "invalid_proposal", "proposal content or status is invalid")
	} else {
		log.Printf("proposal storage: %v", err)
		writeAPIError(w, 500, "internal_error", "proposal storage unavailable")
	}
	return true
}

func registerRepositoryRoutes(mux *http.ServeMux, gitStore *storage.Store, store *repositories.Store, userStore *users.Store, authStore *auth.Store, activityStore *activities.Store) {
	mux.HandleFunc("POST /repositories", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		var input repositoryInput
		if decodeJSON(r, &input) != nil || input.Name == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "name is required")
			return
		}
		repository, err := store.Create(actor.UserID, *input.Name)
		if writeRepositoryError(w, err) {
			return
		}
		w.Header().Set("Location", "/repositories/"+repository.ID)
		writeJSON(w, http.StatusCreated, repository)
	})
	mux.HandleFunc("POST /repositories/{id}/forks", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		source, err := store.GetByID(r.PathValue("id"))
		if writeRepositoryError(w, err) {
			return
		}
		if source.Visibility != repositories.Public {
			readActor, readOK := authenticateRequest(w, r, authStore, "repositories:read", false)
			if !readOK {
				return
			}
			actor = readActor
		}
		var input forkInput
		if decodeJSON(r, &input) != nil || input.Name == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "name is required")
			return
		}
		fork, err := store.CreateFork(actor.UserID, source.ID, *input.Name)
		if writeRepositoryError(w, err) {
			return
		}
		w.Header().Set("Location", "/repositories/"+fork.ID)
		writeJSON(w, http.StatusCreated, fork)
	})
	mux.HandleFunc("POST /repositories/{id}/synchronizations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		var input forkSyncInput
		if decodeJSON(r, &input) != nil || input.Branch == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "branch is required")
			return
		}
		result, err := store.SynchronizeFork(actor.UserID, r.PathValue("id"), *input.Branch)
		if errors.Is(err, repositories.ErrInvalidBranch) {
			writeAPIError(w, http.StatusBadRequest, "invalid_branch", "branch must identify an upstream branch")
			return
		}
		if errors.Is(err, repositories.ErrForkDiverged) {
			writeAPIError(w, http.StatusConflict, "fork_diverged", "fork branch contains work that is not in upstream")
			return
		}
		if errors.Is(err, repositories.ErrBranchChanged) {
			writeAPIError(w, http.StatusConflict, "branch_changed", "fork branch changed during synchronization")
			return
		}
		if writeRepositoryError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("GET /repositories", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:read", false)
		if !ok {
			return
		}
		accessible, err := store.ListAccessible(actor.UserID)
		if writeRepositoryError(w, err) {
			return
		}
		page, next, ok := paginate(r, accessible, func(repository repositories.Repository) string { return repository.ID })
		if !ok {
			writeAPIError(w, http.StatusBadRequest, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"repositories": page, "next_cursor": next})
	})
	mux.HandleFunc("GET /repositories/{id}", func(w http.ResponseWriter, r *http.Request) {
		repository, err := store.GetByID(r.PathValue("id"))
		if writeRepositoryError(w, err) {
			return
		}
		if repository.Visibility == repositories.Public {
			writeJSON(w, http.StatusOK, repository)
			return
		}
		actor, authenticated, ok := authenticateOptionalRequest(w, r, authStore, "repositories:read", false)
		if !ok {
			return
		}
		if !authenticated {
			writeAuthenticationRequired(w, false)
			return
		}
		collaborator, accessErr := store.HasCollaborator(actor.UserID, repository.ID)
		if accessErr != nil {
			writeRepositoryError(w, accessErr)
			return
		}
		if actor.UserID != repository.OwnerID && !collaborator {
			writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
			return
		}
		writeJSON(w, http.StatusOK, repository)
	})
	registerRepositoryBrowseRoutes(mux, gitStore, store, authStore)
	mux.HandleFunc("PATCH /repositories/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		var input repositoryPatch
		if decodeJSON(r, &input) != nil || input.Visibility == nil || (*input.Visibility != repositories.Private && *input.Visibility != repositories.Public) {
			writeAPIError(w, http.StatusBadRequest, "invalid_repository", "visibility must be private or public")
			return
		}
		repository, err := store.SetVisibility(actor.UserID, r.PathValue("id"), *input.Visibility)
		if writeRepositoryError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, repository)
	})
	mux.HandleFunc("GET /repositories/{id}/branches/{branch}/required-checks", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, store, authStore, r.PathValue("id")); !ok {
			return
		}
		checks, err := store.RequiredChecks(r.PathValue("id"), r.PathValue("branch"))
		if writeRepositoryError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"branch": r.PathValue("branch"), "checks": checks})
	})
	mux.HandleFunc("PUT /repositories/{id}/branches/{branch}/required-checks", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, store, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
			return
		}
		var input requiredChecksInput
		if decodeJSON(r, &input) != nil || input.Checks == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_required_checks", "checks must be an array of unique check names")
			return
		}
		checks, err := store.SetRequiredChecks(actor.UserID, r.PathValue("id"), r.PathValue("branch"), input.Checks)
		if errors.Is(err, repositories.ErrInvalidName) {
			writeAPIError(w, http.StatusBadRequest, "invalid_required_checks", "branch and checks must be valid and unique")
			return
		}
		if writeRepositoryError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"branch": r.PathValue("branch"), "checks": checks})
	})
	mux.HandleFunc("GET /repositories/{id}/branches/{branch}/integration-queue", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, store, authStore, r.PathValue("id")); !ok {
			return
		}
		policy, err := store.IntegrationQueuePolicy(r.PathValue("id"), r.PathValue("branch"))
		if writeRepositoryError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, policy)
	})
	mux.HandleFunc("PUT /repositories/{id}/branches/{branch}/integration-queue", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, store, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
			return
		}
		var input integrationQueuePolicyInput
		if decodeJSON(r, &input) != nil || input.Enabled == nil || input.Concurrency == nil || input.FailureBehavior == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_integration_queue", "enabled, concurrency, and failure_behavior are required")
			return
		}
		policy, err := store.SetIntegrationQueuePolicy(actor.UserID, r.PathValue("id"), r.PathValue("branch"), *input.Enabled, *input.Concurrency, *input.FailureBehavior)
		if errors.Is(err, repositories.ErrInvalidName) {
			writeAPIError(w, http.StatusBadRequest, "invalid_integration_queue", "branch, concurrency, or failure behavior is invalid")
			return
		}
		if writeRepositoryError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, policy)
	})
	mux.HandleFunc("DELETE /repositories/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		if writeRepositoryError(w, store.Delete(actor.UserID, r.PathValue("id"))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /repositories/{id}/collaborators", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		collaborators, err := store.ListCollaborators(actor.UserID, r.PathValue("id"))
		if writeRepositoryError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"collaborators": collaborators})
	})
	mux.HandleFunc("POST /repositories/{id}/collaborators", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		var input collaboratorInput
		if decodeJSON(r, &input) != nil || input.UserID == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_collaborator", "user_id is required")
			return
		}
		if _, err := userStore.Get(*input.UserID); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_collaborator", "user_id must identify an existing user")
			return
		}
		alreadyCollaborator, _ := store.HasCollaborator(*input.UserID, r.PathValue("id"))
		collaborator, err := store.AddCollaborator(actor.UserID, r.PathValue("id"), *input.UserID)
		if writeRepositoryError(w, err) {
			return
		}
		if !alreadyCollaborator {
			recordActivity(activityStore, store, activities.Event{Kind: "access.granted", ActorID: actor.UserID, RepositoryID: r.PathValue("id"), ResourceType: "repository", ResourceID: r.PathValue("id"), ResourceTitle: "Contributor access", TargetUserID: input.UserID})
		}
		w.Header().Set("Location", "/repositories/"+r.PathValue("id")+"/collaborators/"+collaborator.UserID)
		writeJSON(w, http.StatusCreated, collaborator)
	})
	mux.HandleFunc("DELETE /repositories/{id}/collaborators/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		wasCollaborator, _ := store.HasCollaborator(r.PathValue("user_id"), r.PathValue("id"))
		if writeRepositoryError(w, store.RemoveCollaborator(actor.UserID, r.PathValue("id"), r.PathValue("user_id"))) {
			return
		}
		target := r.PathValue("user_id")
		if wasCollaborator {
			recordActivity(activityStore, store, activities.Event{Kind: "access.revoked", ActorID: actor.UserID, RepositoryID: r.PathValue("id"), ResourceType: "repository", ResourceID: r.PathValue("id"), ResourceTitle: "Contributor access", TargetUserID: &target})
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func writeRepositoryError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "repository_not_found", "repository not found")
	case errors.Is(err, repositories.ErrNameTaken):
		writeAPIError(w, http.StatusConflict, "repository_name_taken", "repository name is already in use")
	case errors.Is(err, repositories.ErrInvalidName):
		writeAPIError(w, http.StatusBadRequest, "invalid_repository", "repository name is invalid")
	case errors.Is(err, repositories.ErrInvalidCollaborator):
		writeAPIError(w, http.StatusBadRequest, "invalid_collaborator", "repository collaborator is invalid")
	default:
		log.Printf("repository storage: %v", err)
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "repository storage unavailable")
	}
	return true
}

type credentialInput struct {
	Kind      auth.Kind `json:"kind"`
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	ExpiresIn int64     `json:"expires_in"`
}

func registerAuthRoutes(mux *http.ServeMux, store *auth.Store) {
	mux.HandleFunc("GET /auth/credentials", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, store, "credentials:write", false)
		if !ok {
			return
		}
		credentials, err := store.List(actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "internal_error", "credential storage unavailable")
			return
		}
		page, next, valid := paginate(r, credentials, func(credential auth.Credential) string { return credential.ID })
		if !valid {
			writeAPIError(w, http.StatusBadRequest, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"credentials": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /auth/credentials", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, store, "credentials:write", false)
		if !ok {
			return
		}
		var input credentialInput
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_request", "invalid credential request")
			return
		}
		if input.ExpiresIn <= 0 || input.ExpiresIn > int64((90*24*time.Hour)/time.Second) {
			writeAPIError(w, 400, "invalid_credential", "kind, name, scopes, or lifetime is invalid")
			return
		}
		issued, err := store.Issue(actor.UserID, input.Kind, input.Name, input.Scopes, time.Duration(input.ExpiresIn)*time.Second)
		if err != nil {
			writeAPIError(w, 400, "invalid_credential", "kind, name, scopes, or lifetime is invalid")
			return
		}
		writeJSON(w, 201, issued)
	})
	mux.HandleFunc("DELETE /auth/credentials/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, store, "credentials:write", false)
		if !ok {
			return
		}
		if _, err := store.Revoke(actor.UserID, r.PathValue("id")); err != nil {
			writeAPIError(w, 404, "credential_not_found", "credential not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /auth/session", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, store, "credentials:write", false)
		if !ok {
			return
		}
		if _, err := store.Revoke(actor.UserID, actor.ID); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "credential storage unavailable")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "vivarium_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		w.WriteHeader(http.StatusNoContent)
	})
}

func authenticateRequest(w http.ResponseWriter, r *http.Request, store *auth.Store, scope string, git bool) (auth.Credential, bool) {
	credential, authenticated, ok := authenticateOptionalRequest(w, r, store, scope, git)
	if !ok {
		return auth.Credential{}, false
	}
	if !authenticated {
		writeAuthenticationRequired(w, git)
		return auth.Credential{}, false
	}
	return credential, true
}

func authenticateOptionalRequest(w http.ResponseWriter, r *http.Request, store *auth.Store, scope string, git bool) (auth.Credential, bool, bool) {
	credential, authenticated, err := authenticateOptionalCredential(r, store, scope)
	if err != nil {
		if !errors.Is(err, auth.ErrNotFound) {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "credential storage unavailable")
			return auth.Credential{}, false, false
		}
		writeAuthenticationRequired(w, git)
		return auth.Credential{}, false, false
	}
	return credential, authenticated, true
}

func authenticateOptionalCredential(r *http.Request, store *auth.Store, scope string) (auth.Credential, bool, error) {
	token := ""
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		token = strings.TrimPrefix(header, "Bearer ")
	} else if _, password, ok := r.BasicAuth(); ok {
		token = password
	} else if cookie, err := r.Cookie("vivarium_session"); err == nil {
		token = cookie.Value
	}
	if token == "" {
		return auth.Credential{}, false, nil
	}
	credential, err := store.Authenticate(token, scope)
	if err != nil {
		return auth.Credential{}, false, err
	}
	return credential, true, nil
}

func writeAuthenticationRequired(w http.ResponseWriter, git bool) {
	if git {
		w.Header().Set("WWW-Authenticate", `Basic realm="vivarium-git"`)
	} else {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	writeAPIError(w, http.StatusUnauthorized, "unauthorized", "valid authentication is required")
}

func authorizeGitRepository(w http.ResponseWriter, r *http.Request, authStore *auth.Store, catalog *repositories.Store, pulls *pullrequests.Store, remote, scope string) (auth.Credential, bool, bool) {
	// Handlers without an application catalog are retained for storage-level
	// compatibility tests. Production always supplies the catalog.
	if catalog == nil {
		actor, ok := authenticateRequest(w, r, authStore, scope, true)
		return actor, true, ok
	}
	id, ok := strings.CutSuffix(remote, ".git")
	if !ok || id == "" {
		http.Error(w, "repository not found", http.StatusNotFound)
		return auth.Credential{}, false, false
	}
	repository, err := catalog.GetByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			http.Error(w, "repository not found", http.StatusNotFound)
		} else {
			http.Error(w, "repository unavailable", http.StatusInternalServerError)
		}
		return auth.Credential{}, false, false
	}
	if scope == "git:read" && repository.Visibility == repositories.Public && r.Header.Get("Authorization") == "" {
		return auth.Credential{}, false, true
	}
	actor, authenticated, valid := authenticateOptionalRequest(w, r, authStore, scope, true)
	if !valid {
		return auth.Credential{}, false, false
	}
	if !authenticated {
		writeAuthenticationRequired(w, true)
		return auth.Credential{}, false, false
	}
	if actor.RepositoryID != "" && actor.RepositoryID != id {
		http.Error(w, "repository not found", http.StatusNotFound)
		return auth.Credential{}, false, false
	}
	owner := actor.UserID == repository.OwnerID
	collaborator, accessErr := catalog.HasCollaborator(actor.UserID, id)
	if accessErr != nil {
		http.Error(w, "repository unavailable", http.StatusInternalServerError)
		return auth.Credential{}, false, false
	}
	if !owner && !collaborator {
		// Purpose-issued credentials may grant one exact, hidden branch without
		// changing repository membership. Revocation removes that access on the
		// next request; the transport hides every other repository ref.
		if actor.RepositoryID == id && strings.HasPrefix(actor.GitWriteBranch, "refs/heads/vivarium-security/") {
			return actor, false, true
		}
		if actor.GitWriteBranch != "" && actor.PullRequestID != "" && pulls != nil && pulls.AllowsMaintainerEdit(id, actor.GitWriteBranch, actor.PullRequestID, actor.UserID, func(targetID, userID string) bool {
			target, targetErr := catalog.GetByID(targetID)
			if targetErr != nil {
				return false
			}
			if target.OwnerID == userID {
				return true
			}
			ok, collaboratorErr := catalog.HasCollaborator(userID, targetID)
			return collaboratorErr == nil && ok
		}) {
			return actor, false, true
		}
		http.Error(w, "repository not found", http.StatusNotFound)
		return auth.Credential{}, false, false
	}
	return actor, owner, true
}

func setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: "vivarium_session", Value: token, Path: "/", Expires: expires, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

func decodeJSON(r *http.Request, destination any) error {
	return decodeJSONLimit(r, destination, 1<<20)
}

func decodeJSONLimit(r *http.Request, destination any, limit int64) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func paginate[T any](r *http.Request, all []T, id func(T) string) ([]T, *string, bool) {
	limit, after, ok := paginationParameters(r)
	if !ok {
		return nil, nil, false
	}
	start := 0
	if after != "" {
		start = -1
		for index, item := range all {
			if id(item) == after {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return nil, nil, false
		}
	}
	end := min(start+limit, len(all))
	page := all[start:end]
	var next *string
	if end < len(all) {
		cursor := id(all[end-1])
		next = &cursor
	}
	return page, next, true
}

func paginationParameters(r *http.Request) (int, string, bool) {
	values := r.URL.Query()
	limitValues, hasLimit := values["limit"]
	afterValues, hasAfter := values["after"]
	if len(limitValues) > 1 || len(afterValues) > 1 || (hasLimit && limitValues[0] == "") || (hasAfter && afterValues[0] == "") {
		return 0, "", false
	}
	limit := 30
	if hasLimit {
		raw := limitValues[0]
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return 0, "", false
		}
		limit = parsed
	}
	after := ""
	if hasAfter {
		after = afterValues[0]
	}
	return limit, after, true
}

func writeUserError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, users.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "user_not_found", "user not found")
	case errors.Is(err, users.ErrHandleTaken):
		writeAPIError(w, http.StatusConflict, "handle_taken", "handle is already in use")
	case errors.Is(err, users.ErrInvalidProfile):
		writeAPIError(w, http.StatusBadRequest, "invalid_profile", err.Error())
	default:
		log.Printf("user storage: %v", err)
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "user storage unavailable")
	}
	return true
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func openRemoteRepository(w http.ResponseWriter, store *storage.Store, remote string) (*storage.Repository, bool) {
	id, ok := strings.CutSuffix(remote, ".git")
	if !ok || id == "" {
		http.Error(w, "repository not found", http.StatusNotFound)
		return nil, false
	}
	repo, err := store.Open(id)
	if errors.Is(err, storage.ErrRepositoryNotFound) || errors.Is(err, storage.ErrInvalidID) {
		http.Error(w, "repository not found", http.StatusNotFound)
		return nil, false
	}
	if err != nil {
		http.Error(w, "repository unavailable", http.StatusInternalServerError)
		return nil, false
	}
	return repo, true
}

func runUploadPack(w http.ResponseWriter, r *http.Request, repo *storage.Repository, advertise bool) {
	runGitService(w, r, repo, uploadPackService, advertise, false, "")
}

func runGitService(w http.ResponseWriter, r *http.Request, repo *storage.Repository, service string, advertise, contributor bool, onlyBranch string) {
	commandName := strings.TrimPrefix(service, "git-")
	args := []string{commandName, "--stateless-rpc"}
	if onlyBranch != "" {
		// A pull-request grant exposes only its contribution branch, never the
		// rest of an independently owned private fork.
		args = append([]string{"-c", "transfer.hideRefs=refs", "-c", "transfer.hideRefs=!" + onlyBranch}, args...)
	} else {
		// Embargoed repair refs never appear through ordinary clone, fetch, or
		// push discovery, including to repository owners.
		args = append([]string{"-c", "transfer.hideRefs=refs/heads/vivarium-security/"}, args...)
	}
	if service == uploadPackService {
		// History quarantine depends on upload-pack refusing direct or merely
		// reachable SHA wants. Only objects reachable from advertised safe refs
		// may be served after a rewrite.
		args = append([]string{"-c", "uploadpack.allowAnySHA1InWant=false", "-c", "uploadpack.allowReachableSHA1InWant=false"}, args...)
	}
	var removeHooks func()
	if service == receivePackService {
		// Receive-pack applies each requested ref update transactionally. The
		// client distinguishes ordinary progress from explicit force updates,
		// while the hook keeps writes constrained to branch references.
		args = append([]string{
			"-c", "receive.denyNonFastForwards=false",
			"-c", "receive.denyDeletes=false",
			"-c", "receive.denyDeleteCurrent=ignore",
		}, args...)
		if !advertise {
			hooksPath, err := os.MkdirTemp("", "vivarium-receive-hooks-")
			if err != nil {
				log.Printf("prepare %s for repository %s: %v", service, repo.ID(), err)
				return
			}
			removeHooks = func() { _ = os.RemoveAll(hooksPath) }
			defer removeHooks()
			hook := branchNamespaceHook
			if onlyBranch != "" {
				hook = "#!/bin/sh\nwhile read -r old new ref\ndo\n  if [ \"$ref\" != \"" + onlyBranch + "\" ]; then\n    echo \"credential may only update " + onlyBranch + "\" >&2\n    exit 1\n  fi\ndone\n"
			} else if contributor {
				hook = contributorBranchHook
			}
			if err := os.WriteFile(hooksPath+"/pre-receive", []byte(hook), 0o700); err != nil {
				log.Printf("prepare %s for repository %s: %v", service, repo.ID(), err)
				return
			}
			args = append([]string{"-c", "core.hooksPath=" + hooksPath}, args...)
		}
	}
	if advertise {
		args = append(args, "--advertise-refs")
	}
	args = append(args, repo.Path())
	command := exec.CommandContext(r.Context(), "git", args...)
	// Git services can spawn pack processes. Give the invocation a dedicated
	// process group and cancel the whole group so descendants cannot outlive an
	// abandoned HTTP request.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.Stdout = w
	command.Stderr = os.Stderr
	if !advertise {
		command.Stdin = r.Body
	}
	if protocol := r.Header.Get("Git-Protocol"); protocol != "" && !strings.ContainsAny(protocol, "\x00\r\n") {
		command.Env = append(os.Environ(), "GIT_PROTOCOL="+protocol)
	}
	if err := command.Run(); err != nil {
		log.Printf("serve %s for repository %s: %v", service, repo.ID(), err)
	}
}

func pktLine(payload string) string {
	return fmt.Sprintf("%04x%s", len(payload)+4, payload)
}

func setGitCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
	w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	w.Header().Set("Pragma", "no-cache")
}
