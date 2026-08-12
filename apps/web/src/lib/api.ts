export type User = {
  id: string;
  handle: string;
  display_name: string;
  created_at: string;
  updated_at: string;
};
export type ContributorPathway = {
  id: string;
  repository_id: string;
  version: number;
  goals: string;
  prerequisites: string[];
  conduct: string;
  security: string;
  setup: { summary: string; workspace_path?: string; verification_commands: string[] };
  communication: string;
  review_policy: string;
  work_categories: { name: string; description: string; audience: "human" | "agent" | "human_or_agent" }[];
  requirements: { kind: "documentation" | "ownership" | "release" | "issue" | "proposal" | "workspace_definition"; label: string; resource_id?: string; path?: string; revision?: string; status?: "current" | "stale" | "inaccessible"; status_detail?: string }[];
  published_by: string;
  published_at: string;
};
export type ContributorPathwayResponse = {
  pathway: ContributorPathway;
  history: ContributorPathway[];
  acknowledgements: { id: string; version: number; actor_id: string; acknowledged_at: string }[];
  acknowledgement_count: number;
};
export type ContributionOpportunity = {
  id: string; repository_id: string; version: number; title: string; expected_outcome: string; scope: string;
  source: { kind: "issue" | "proposal" | "stewardship" | "task"; id: string; parent_id?: string };
  required_skills: string[]; interests: string[]; dependency_ids: string[]; risk: "low" | "medium" | "high";
  estimated_minutes: number; agent_assistance: boolean; mentors: { user_id: string; note?: string }[];
  revision: string; status: "open" | "in_progress" | "paused" | "completed"; claim?: { id: string; actor_id: string; note?: string; claimed_at: string; expires_at: string };
  completion?: { contributor_id:string; pull_request_id:string; release_id:string; release_version:string; merge_commit_id:string; credit:string[]; feedback:string; support_effort:{ setup_attempts:number; mentor_guidance_items:number; agent_assistance_items:number }; readiness:{ ready_for_next:boolean; skills_recognized:string[]; next_opportunity_id?:string; note:string }; recorded_by:string; recorded_at:string };
};
export type ContributionMatch = { opportunity: ContributionOpportunity; score: number; reasons: string[]; gaps: string[]; ready: boolean };
export type DeliveryTeam = {
  id: string;
  repository_id: string;
  name: string;
  purpose: string;
  organizer_id: string;
  outcome: {
    kind:
      | "proposal"
      | "initiative"
      | "decision"
      | "incident_follow_up"
      | "planned_outcome";
    resource_id: string;
    title: string;
  };
  overall_budget?: { unit: "minutes" | "credits" | "usd"; limit: number };
  deadline?: string;
  escalation_path: string;
  version: number;
  participants: {
    id: string;
    principal_type: "human" | "agent";
    principal_id: string;
    role: string;
    responsibility: string;
    why: string;
    budget?: { unit: string; limit: number };
    deadline?: string;
    escalation: string;
    required_access: { repository_id: string; level: "read" | "write" }[];
    access_preview: {
      repository_id: string;
      required: string;
      effective: string;
      source: string;
      sufficient: boolean;
    }[];
    status: "pending" | "accepted" | "declined";
    can_respond: boolean;
    invited_by: string;
    invited_at: string;
    responded_by?: string;
    responded_at?: string;
  }[];
  plan?: {
    revision: number;
    proposed_by: string;
    updated_at: string;
    streams: {
      id: string;
      title: string;
      owner_participant_id: string;
      inputs: {
        name: string;
        source_stream_id?: string;
        repository_id?: string;
        revision?: string;
        artifact: string;
      }[];
      expected_artifacts: string[];
      dependency_ids: string[];
      acceptance_criteria: string[];
      repository_scope: {
        repository_id: string;
        reference: string;
        revision: string;
        paths: string[];
      }[];
      integration_order: number;
      budget?: { unit: string; limit: number };
      assumptions: string[];
      contexts: {
        id: string;
        kind: "change_session" | "investigation" | "experiment" | "workspace";
        resource_id: string;
        parent_id?: string;
        repository_id: string;
        revision: string;
        attached_by: string;
        attached_at: string;
      }[];
    }[];
    acceptances: {
      participant_id: string;
      revision: number;
      status: "pending" | "accepted" | "declined";
      required: boolean;
      responded_by?: string;
      responded_at?: string;
      can_respond: boolean;
    }[];
    blockers: {
      kind:
        | "replan_acceptance"
        | "budget_exceeded"
        | "overlapping_scope"
        | "duplicate_artifact"
        | "unavailable_access"
        | "owner_unavailable"
        | "charter_changed";
      stream_ids: string[];
      owner_participant_ids: string[];
      summary: string;
    }[];
  };
  plan_history: { revision: number; proposed_by: string; updated_at: string }[];
  timeline: {
    id: string;
    stream_id: string;
    kind:
      | "finding"
      | "question"
      | "checkpoint"
      | "artifact"
      | "decision"
      | "uncertainty";
    body: string;
    citations: {
      kind: string;
      resource_id: string;
      repository_id: string;
      revision: string;
      label: string;
    }[];
    author_id: string;
    author_type: "human" | "agent";
    created_by: string;
    plan_revision: number;
    created_at: string;
  }[];
  handoffs: {
    id: string;
    stream_id: string;
    from_participant_id: string;
    to_participant_id: string;
    input_entry_ids: string[];
    inputs: {
      kind: string;
      resource_id: string;
      repository_id: string;
      revision: string;
      label: string;
    }[];
    acceptance_criteria: string[];
    residual_uncertainty: string[];
    requested_by: string;
    requested_at: string;
    plan_revision: number;
    status: "pending" | "accepted";
    accepted_by?: string;
    accepted_at?: string;
    verification_entry_ids: string[];
    acceptance_note?: string;
  }[];
  stream_statuses: {
    stream_id: string;
    status:
      | "queued"
      | "running"
      | "paused"
      | "blocked"
      | "completed"
      | "failed"
      | "canceled";
    summary: string;
    progress_percent: number;
    revision: string;
    resource_use?: { unit: string; consumed: number };
    active_control?: {
      participant_id: string;
      principal_id: string;
      principal_type: "human" | "agent";
      since: string;
    };
    blockers: { kind: string; summary: string; recovery: string }[];
    questions: {
      id: string;
      body: string;
      ask_of: string;
      urgency: "normal" | "urgent";
    }[];
    predicted_next_action: string;
    updated_by: string;
    updated_at: string;
  }[];
  interventions: {
    id: string;
    scope: "stream" | "team";
    stream_id?: string;
    action: "guide" | "pause" | "resume" | "cancel" | "reassign" | "narrow";
    guidance: string;
    actor_id: string;
    principal_id: string;
    plan_revision: number;
    created_at: string;
  }[];
  integrations: {
    id: string;
    plan_revision: number;
    base_revision: string;
    prepared_by: string;
    prepared_at: string;
    published_by?: string;
    published_at?: string;
    blockers: {
      kind: string;
      stream_ids: string[];
      paths?: string[];
      criteria?: string[];
      summary: string;
    }[];
    contributions: {
      stream_id: string;
      repository_id: string;
      source_kind: "branch" | "checkpoint";
      workspace_id?: string;
      checkpoint_id?: string;
      branch: string;
      commit_id: string;
      changed_paths: string[];
      acceptance_evidence: Record<string, string[]>;
      authors: string[];
      agent_actions: string[];
      decisions: string[];
      cost?: { unit: string; consumed: number };
      residual_risks: string[];
      integration_order: number;
    }[];
    pull_requests: {
      stream_id: string;
      repository_id: string;
      pull_request_id: string;
      order: number;
    }[];
  }[];
  events: {
    id: string;
    kind: string;
    actor_id: string;
    summary: string;
    version: number;
    created_at: string;
  }[];
  created_at: string;
  updated_at: string;
};
export type Repository = {
  id: string;
  owner_id: string;
  organization_id?: string;
  name: string;
  visibility: "private" | "public";
  default_branch: string;
  git_remote: string;
  created_at: string;
  upstream_repository_id?: string;
};
export type TechnicalDecision = {
  id: string;
  repository_id: string;
  source: {
    kind:
      | "repository"
      | "proposal"
      | "investigation"
      | "incident"
      | "evolution_plan"
      | "stewardship_opportunity";
    resource_id?: string;
  };
  status: "pending" | "published";
  scope: {
    question: string;
    constraints: string[];
    success_measures: string[];
    deadline?: string;
    affected_resources: {
      kind: string;
      repository_id?: string;
      resource_id?: string;
      label: string;
    }[];
    participants: { user_id: string; added_by: string; added_at: string }[];
    owner_id: string;
  };
  created_by: string;
  version: number;
  alternatives: DecisionAlternative[];
  findings: DecisionFinding[];
  experiments: DecisionExperiment[];
  approval_requests: DecisionApprovalRequest[];
  commitments: DecisionCommitment[];
  implementations: {
    commitment_version: number;
    proposal_id: string;
    task_ids: string[];
    revision: string;
    created_by: string;
    created_at: string;
    observations: {
      id: string;
      kind: string;
      summary: string;
      resource_kind: string;
      resource_id: string;
      actor_id: string;
      created_at: string;
    }[];
  }[];
  history: {
    id: string;
    kind:
      | "scope_created"
      | "scope_changed"
      | "discussion"
      | "alternative_proposed"
      | "research_finding"
      | "approval_requested"
      | "approval_approved"
      | "approval_rejected"
      | "decision_published"
      | "decision_reopened";
    actor_id: string;
    version: number;
    summary: string;
    body?: string;
    created_at: string;
  }[];
  created_at: string;
  updated_at: string;
};
export type DecisionApprovalRequest = {
  id: string;
  kind: "affected_owner" | "policy";
  repository_id?: string;
  policy_id?: string;
  policy_rule?: string;
  approver_id: string;
  reason: string;
  exception_reason?: string;
  exception_expires_at?: string;
  status: "pending" | "approved" | "rejected" | "superseded";
  requested_by: string;
  requested_at: string;
  decided_by?: string;
  decision_note?: string;
  decided_at?: string;
};
export type DecisionCommitment = {
  version: number;
  decision_version: number;
  status: "published" | "reopened";
  selected_alternative_id: string;
  rejected_alternative_ids: string[];
  rationale: string;
  accepted_tradeoffs: string[];
  dissent_finding_ids: string[];
  conditions: string[];
  review_date: string;
  evidence: DecisionEvidence[];
  approvals: DecisionApprovalRequest[];
  exceptions: {
    approval_request_id: string;
    policy_id: string;
    policy_rule: string;
    reason: string;
    expires_at: string;
  }[];
  published_by: string;
  published_at: string;
  reopened_at?: string;
  reopen_reason?: string;
};
export type DecisionExperiment = {
  id: string;
  alternative_id: string;
  workspace_id: string;
  revision: string;
  definition_sha256: string;
  default_branch_revision: string;
  default_definition_sha256: string;
  commands: string[];
  launched_by: string;
  launched_at: string;
  version: number;
  invalidated: boolean;
  invalidation_reasons: string[];
  evidence: {
    id: string;
    checkpoint_ids: string[];
    command_ids: string[];
    measurements: { name: string; value: number; unit: string }[];
    artifacts: { label: string; path: string; sha256: string; size: number }[];
    cpu_seconds: number;
    memory_mb_hours: number;
    storage_mb_hours: number;
    notes?: string;
    recorded_by: string;
    recorded_at: string;
  }[];
};
export type DecisionEvidence = {
  kind: "code" | "dependency" | "release" | "incident" | "usage";
  repository_id?: string;
  resource_id: string;
  revision: string;
  path?: string;
  start_line?: number;
  end_line?: number;
  label: string;
  captured_at?: string;
};
export type DecisionAlternative = {
  id: string;
  title: string;
  summary: string;
  assumptions: string[];
  tradeoffs: string[];
  risks: string[];
  compatibility_impact: string;
  cost: string;
  expected_outcomes: string[];
  evidence: DecisionEvidence[];
  criteria: {
    criterion: string;
    outcome: string;
    evidence: DecisionEvidence[];
  }[];
  evidence_status: {
    missing_kinds: string[];
    stale: DecisionEvidence[];
    missing_criteria: string[];
  };
  proposed_by: string;
  version: number;
  superseded_by?: string;
  created_at: string;
  updated_at: string;
};
export type DecisionFinding = {
  id: string;
  alternative_id: string;
  body: string;
  position: "support" | "oppose" | "neutral";
  uncertainty: string;
  citations: DecisionEvidence[];
  actor_id: string;
  supersedes_id?: string;
  superseded: boolean;
  created_at: string;
};
export type CodeNavigationResult = {
  repository_id: string;
  revision: string;
  query: string;
  results: {
    kind: "definition" | "reference" | "caller" | "test";
    path: string;
    line: number;
    preview: string;
    commit_id?: string;
    commit_summary?: string;
  }[];
  ownership: { kind: "repository_owner" | "collaborator"; id: string }[];
  dependencies: {
    id: string;
    provider_repository_id: string;
    interface_name: string;
    constraint: string;
    commit_id: string;
  }[];
  analysis: {
    status: "complete" | "incomplete";
    reason: string;
    files_scanned: number;
    bytes_scanned: number;
    result_limit: number;
    method: string;
  };
};
export type ExplanationCitation = {
  kind: string;
  revision: string;
  path?: string;
  start_line?: number;
  end_line?: number;
  commit_id?: string;
  resource_id?: string;
  label: string;
  stale?: boolean;
};
export type ExplanationClaim = {
  id: string;
  text: string;
  basis: "evidence" | "inference" | "uncertainty";
  confidence: "high" | "medium" | "low";
  citations: ExplanationCitation[];
};
export type ExplanationEntry = {
  id: string;
  kind:
    | "code_reference"
    | "query"
    | "runtime_observation"
    | "hypothesis"
    | "agent_finding"
    | "conclusion"
    | "challenge";
  body: string;
  actor_id: string;
  revision: string;
  citations?: ExplanationCitation[];
  resource_id?: string;
  supersedes_id?: string;
  created_at: string;
};
export type ExplanationConversation = {
  id: string;
  repository_id: string;
  revision: string;
  context: {
    kind:
      | "repository"
      | "file"
      | "proposal"
      | "task"
      | "pull_request"
      | "incident"
      | "workspace";
    resource_id?: string;
    path?: string;
  };
  question: string;
  asked_by: string;
  agent: string;
  answer: string;
  claims: ExplanationClaim[];
  participants: { user_id: string; invited_by?: string; joined_at: string }[];
  entries: ExplanationEntry[];
  analysis_status: "complete" | "incomplete";
  analysis_reason?: string;
  created_at: string;
  updated_at: string;
};
export type ImpactEvidence = {
  kind: string;
  repository_id: string;
  revision: string;
  path?: string;
  line?: number;
  resource_id?: string;
  label: string;
  owner_id?: string;
  state?: string;
  verification?: string;
};
export type ImpactAssessment = {
  id: string;
  repository_id: string;
  revision: string;
  title: string;
  source: {
    kind: "selected_code" | "investigation_conclusion" | "proposed_diff";
    path?: string;
    start_line?: number;
    end_line?: number;
    explanation_id?: string;
    entry_id?: string;
    diff?: string;
  };
  created_by: string;
  participants: { user_id: string; invited_by?: string; joined_at: string }[];
  items: {
    id: string;
    kind:
      | "reference"
      | "test"
      | "owner"
      | "package"
      | "interface"
      | "consumer"
      | "release"
      | "environment"
      | "risk"
      | "unknown";
    summary: string;
    status: "candidate" | "accepted_risk" | "unknown" | "verification_required";
    evidence?: ImpactEvidence[];
    added_by: string;
    created_at?: string;
  }[];
  acknowledgement_requests: {
    id: string;
    repository_id: string;
    owner_id: string;
    requested_by: string;
    note?: string;
    requested_at: string;
    acknowledged_by?: string;
    acknowledgement?: string;
    acknowledged_at?: string;
  }[];
  analysis_status: "complete" | "incomplete";
  analysis_reason?: string;
  version: number;
  created_at: string;
  updated_at: string;
  context_state: "current" | "changed";
  implementation?: {
    proposal_id: string;
    task_ids: string[];
    created_by: string;
    created_at: string;
  };
};
export type DevelopmentWorkspace = {
  id: string;
  repository_id: string;
  commit_id: string;
  creator_id: string;
  state:
    "provisioning" | "running" | "suspended" | "failed" | "stopped" | "expired";
  definition_sha256: string;
  created_at: string;
  updated_at: string;
  source: {
    kind:
      | "repository"
      | "proposal_task"
      | "pull_request"
      | "incident_repair"
      | "decision_experiment";
    repository_id: string;
    proposal_id?: string;
    task_id?: string;
    pull_request_id?: string;
    incident_id?: string;
    repair_id?: string;
    decision_id?: string;
    alternative_id?: string;
    default_branch_revision?: string;
    default_definition_sha256?: string;
    upstream_repository_id?: string;
    opportunity_id?: string;
  };
  contributor_context?: {
    opportunity_id: string; opportunity_version: number; upstream_repository_id: string; pathway_version: number;
    guidance: string; prerequisites: string[]; acceptance_criteria: string[]; evidence_kind: string; evidence_id: string;
    evidence_parent_id?: string; sample_attachment_ids?: string[]; diagnostics: string[]; mentor_ids: string[]; agent_assistance: boolean;
    help: ContributionHelp;
  };
  definition: {
    version: number;
    image: string;
    tools: { name: string; version: string }[];
    dependencies: string[];
    setup: string[];
    experiments?: { name: string; command: string }[];
    resources: {
      cpus: number;
      memory_mb: number;
      storage_mb: number;
      setup_seconds: number;
    };
  };
  effective_access: { role: string; scopes: string[] };
  setup_evidence: {
    command: string;
    state: string;
    exit_code: number;
    output?: string;
    started_at: string;
    completed_at: string;
  }[];
  events: {
    id?: string;
    kind: string;
    actor_id: string;
    role?: "observation" | "instruction" | "authorship" | "execution";
    detail?: string;
    created_at: string;
  }[];
  command_outcomes: {
    id: string;
    command_sha256: string;
    directory: string;
    exit_code: number;
    output?: string;
    actor_id: string;
    started_at: string;
    completed_at: string;
  }[];
  changes: {
    path: string;
    sha256: string;
    size: number;
    actor_id: string;
    created_at: string;
  }[];
  presence: {
    actor_id: string;
    focus: "workspace" | "file" | "terminal" | "command" | "preview";
    path?: string;
    joined_at: string;
    seen_at: string;
  }[];
  control: {
    version: number;
    principal_kind: "human" | "approved_agent";
    principal_id: string;
    mode: "observe" | "guide" | "edit" | "execute";
    scopes: ("files" | "commands" | "lifecycle")[];
    granted_by: string;
    granted_at: string;
    expires_at: string;
  };
  messages: {
    id: string;
    actor_id: string;
    body: string;
    created_at: string;
  }[];
  head_checkpoint_id?: string;
  policy: WorkspacePolicy;
  policy_scope: string;
  policy_version: number;
  last_activity_at: string;
  expires_at?: string;
  expiry_announced_at?: string;
  stopped_at?: string;
  stopped_by?: string;
  stop_reason?: string;
  rebuild_required: boolean;
  rebuild_reasons: string[];
};
export type ContributionHelpEntry = {id:string;kind:string;actor_id:string;agent_id?:string;action?:string;body:string;reply_to?:string;status:string;decision_owner:"contributor"|"maintainer";due_at?:string;created_at:string;resolved_at?:string};
export type ContributionHelp = {version:number;state:"active"|"reassignment_requested"|"exited";state_reason?:string;entries:ContributionHelpEntry[];mentor_availability:{mentor_id:string;status:string;response_hours?:number;note?:string;updated_at:string}[]};
export type WorkspacePolicy = {
  version: number;
  max_cpus: number;
  max_memory_mb: number;
  max_storage_mb: number;
  network: "none";
  idle_minutes: number;
  max_runtime_hours: number;
  retention_hours: number;
  sharing: "private" | "repository" | "organization";
  agent_execution: boolean;
  updated_by?: string;
  updated_at?: string;
};
export type WorkspaceConsumption = {
  workspace_id: string;
  repository_id: string;
  creator_id: string;
  state: string;
  cpu_seconds: number;
  memory_mb_hours: number;
  storage_mb_hours: number;
  measured_at: string;
};
export type WorkspaceCheckpoint = {
  id: string;
  workspace_id: string;
  repository_id: string;
  base_commit_id: string;
  definition_sha256: string;
  parent_checkpoint_id?: string;
  title: string;
  description?: string;
  reproducibility: { dependencies: string[]; notes?: string };
  created_by: string;
  created_at: string;
  files: {
    path: string;
    operation: "add" | "modify" | "delete";
    mode?: number;
    size?: number;
    sha256?: string;
  }[];
  contributor_ids: string[];
  commands: {
    id: string;
    sha256: string;
    exit_code: number;
    actor_id: string;
  }[];
  publication?: {
    branch: string;
    commit_id: string;
    pull_request_id?: string;
    task_id?: string;
    session_id?: string;
    contributor_ids: string[];
    command_ids: string[];
    link_pending?: boolean;
    published_by: string;
    published_at: string;
  };
};
export type CheckpointAnalysis = {
  checkpoint_id: string;
  preflight_token: string;
  base_diverged: boolean;
  repository_head?: string;
  conflicts: string[];
  missing_dependencies: string[];
  reproducible: boolean;
  reasons: string[];
};
export type ContributionPublicationFinding = { code: string; message: string; fix: string };
export type ContributionPublicationAssessment = {
  ready: boolean;
  project_requirements: ContributionPublicationFinding[];
  coaching_needs: ContributionPublicationFinding[];
  acceptance_criteria: string[];
};
export type Organization = {
  id: string;
  name: string;
  slug: string;
  description?: string;
  created_by: string;
  created_at: string;
  members: { user_id: string; role: "owner" | "member"; joined_at: string }[];
  invitations: {
    id: string;
    user_id: string;
    invited_by: string;
    created_at: string;
  }[];
  transfers: {
    id: string;
    repository_id: string;
    from_owner_id: string;
    requested_by: string;
    status: "pending" | "accepted";
    requested_at: string;
    accepted_by?: string;
    accepted_at?: string;
  }[];
  teams: OrganizationTeam[];
  agents: OrganizationAgent[];
  access_grants: OrganizationAccessGrant[];
  access_requests: OrganizationAccessRequest[];
  policies: OrganizationPolicy[];
  policy_exceptions: OrganizationPolicyException[];
  initiatives: OrganizationInitiative[];
  stewardship_mandates: OrganizationStewardshipMandate[];
  events: OrganizationEvent[];
};
export type OrganizationStewardshipMandateRevision = {
  version: number;
  desired_outcomes: string[];
  repositories: { repository_id: string; branches: string[] }[];
  trusted_signals: string[];
  exclusions: string[];
  budget: { max_agent_minutes: number; max_actions: number };
  starts_at: string;
  expires_at: string;
  agent_id: string;
  allowed_actions: string[];
  required_human_decisions: string[];
  opportunity_policies: {
    evidence_type: string;
    minimum_severity: string;
    mode: "approval_required" | "auto_start";
    max_agent_minutes: number;
  }[];
  reason: string;
  created_by: string;
  created_at: string;
};
export type OrganizationStewardshipMandate = {
  id: string;
  title: string;
  version: number;
  status: "pending_acceptance" | "active" | "paused" | "expired" | "revoked";
  revisions: OrganizationStewardshipMandateRevision[];
  acceptance?: { version: number; operator_id: string; accepted_at: string };
  paused_by?: string;
  paused_at?: string;
  revoked_by?: string;
  revoked_at?: string;
  opportunities: OrganizationStewardshipOpportunity[];
  used_agent_minutes: number;
  used_actions: number;
  tuning: {
    version: number;
    priority_evidence: string[];
    ignored_evidence: string[];
    minimum_confidence: number;
    updated_by?: string;
    updated_at?: string;
  };
  outcomes: {
    id: string;
    idempotency_key: string;
    opportunity_id?: string;
    kind:
      | "implementation"
      | "verification"
      | "release"
      | "resource"
      | "false_positive"
      | "goal"
      | "automation";
    status:
      | "succeeded"
      | "failed"
      | "partial"
      | "inactive"
      | "revoked_access"
      | "anomalous";
    summary: string;
    goal?: string;
    goal_progress?: number;
    agent_minutes?: number;
    actions?: number;
    consecutive_failures?: number;
    recorded_by: string;
    recorded_at: string;
  }[];
  notices: {
    id: string;
    kind: string;
    summary: string;
    action: string;
    created_at: string;
  }[];
};
export type OrganizationStewardshipOpportunity = {
  id: string;
  mandate_version: number;
  repository_id: string;
  evidence_type:
    | "repository"
    | "dependency"
    | "check"
    | "release"
    | "incident"
    | "security"
    | "usage";
  evidence_id: string;
  evidence_revision: string;
  title: string;
  summary: string;
  severity: "critical" | "high" | "medium" | "low";
  expected_value: string;
  confidence: number;
  affected_owner_ids: string[];
  affected_revisions: string[];
  in_scope_reason: string;
  citations: {
    kind: string;
    resource_id: string;
    revision: string;
    label: string;
    url?: string;
    stale: boolean;
  }[];
  status:
    | "open"
    | "dismissed"
    | "snoozed"
    | "incorrect"
    | "promoting"
    | "promoted"
    | "accepted";
  rank: number;
  snoozed_until?: string;
  decision_reason?: string;
  admission: "approval_required" | "auto_start_eligible";
  max_agent_minutes: number;
  blockers: string[];
  approval?: {
    decision: string;
    actor_id: string;
    reason: string;
    opportunity_version: number;
    created_at: string;
  };
  work?: {
    proposal_id: string;
    task_ids: string[];
    base_revision: string;
    created_by: string;
    created_at: string;
  };
  version: number;
  evaluated_by: string;
  evaluated_at: string;
  updated_by: string;
  updated_at: string;
  comments: {
    id: string;
    actor_id: string;
    body: string;
    created_at: string;
  }[];
};
export type OrganizationStewardshipPreview = {
  mandate_id: string;
  version: number;
  status: string;
  access_grants: OrganizationAccessGrant[];
  effective_policies: Record<string, { rules: OrganizationPolicyRules }>;
  implicit_authority: string[];
  notice: string;
};
export type OrganizationPolicyRules = {
  repository_visibility?: "public" | "private";
  minimum_reviews?: number;
  required_checks?: string[];
  integration?: "direct" | "queue";
  release_provenance?: "attested";
  dependency_use?: "active-only" | "approved-only";
  promotion_approvals?: number;
  agent_authority?: "explicit-grants" | "disabled";
};
export type OrganizationPolicy = {
  id: string;
  name: string;
  description?: string;
  version: number;
  status: "draft" | "active" | "superseded";
  targets: { kind: "organization" | "team" | "repository"; id?: string }[];
  rules: OrganizationPolicyRules;
  created_by: string;
  created_at: string;
  activated_by?: string;
  activated_at?: string;
  applies_to_new_work: boolean;
};
export type OrganizationPolicyException = {
  id: string;
  policy_id: string;
  repository_id: string;
  rule: string;
  requested_value: string;
  reason: string;
  requester_id: string;
  expires_at: string;
  status: "pending" | "approved" | "denied";
  created_at: string;
  decided_by?: string;
  decided_at?: string;
};
export type OrganizationResourceScope = {
  kind: "repository" | "package" | "environment" | "collaboration";
  id: string;
};
export type OrganizationAccessGrant = {
  id: string;
  principal_type: "team" | "agent";
  principal_id: string;
  role: "viewer" | "contributor" | "maintainer" | "operator";
  resources: OrganizationResourceScope[];
  exceptions: { resource: OrganizationResourceScope; reason: string }[];
  reason: string;
  expires_at?: string;
  version: number;
  granted_by: string;
  granted_at: string;
  revoked_by?: string;
  revoked_at?: string;
  derived_credentials: {
    id: string;
    operator_id: string;
    created_at: string;
  }[];
};
export type OrganizationAccessRequest = {
  id: string;
  requester_id: string;
  principal_type: "team" | "agent";
  principal_id: string;
  role: string;
  resources: OrganizationResourceScope[];
  exceptions: { resource: OrganizationResourceScope; reason: string }[];
  reason: string;
  expires_at?: string;
  status: "pending" | "approved" | "denied";
  created_at: string;
  decided_by?: string;
  decided_at?: string;
  grant_id?: string;
};
export type OrganizationTeam = {
  id: string;
  name: string;
  slug: string;
  description?: string;
  parent_id?: string;
  visibility: "public" | "organization";
  version: number;
  created_by: string;
  created_at: string;
  members: {
    user_id: string;
    role: "member" | "maintainer";
    added_by: string;
    added_at: string;
  }[];
  responsibilities: {
    id: string;
    repository_id: string;
    area: string;
    description?: string;
    added_by: string;
    added_at: string;
  }[];
};
export type OrganizationAgent = {
  id: string;
  name: string;
  slug: string;
  description?: string;
  visibility: "public" | "organization";
  capabilities: string[];
  operator_ids: string[];
  team_ids: string[];
  version: number;
  registered_by: string;
  registered_at: string;
};
export type OrganizationEvent = {
  id: string;
  action: string;
  actor_id: string;
  target_id?: string;
  created_at: string;
  details?: Record<string, unknown>;
};
export type OrganizationInitiativeSource = {
  kind: "proposal" | "evolution" | "incident" | "security";
  repository_id?: string;
  id: string;
};
export type OrganizationInitiativeOwner = {
  type: "human" | "team" | "agent";
  id: string;
};
export type OrganizationInitiativeItem = {
  id: string;
  title: string;
  repository_id: string;
  contribution?: OrganizationInitiativeSource;
  owner: OrganizationInitiativeOwner;
  dependency_ids: string[];
  status: "todo" | "in_progress" | "completed";
  position: number;
  blocked?: boolean;
  blocker_ids?: string[];
  ownership_state?: "accountable" | "reassignment_required";
  reassignment_note?: string;
};
export type OrganizationInitiative = {
  id: string;
  title: string;
  description?: string;
  source: OrganizationInitiativeSource;
  status: "active" | "completed";
  version: number;
  work_items: OrganizationInitiativeItem[];
  policy_exceptions?: OrganizationPolicyException[];
  upcoming_releases?: ReleaseCandidate[];
  created_by: string;
  created_at: string;
  updated_at: string;
};
export type OrganizationDirectory = {
  organization_id: string;
  name: string;
  slug: string;
  teams: {
    team: OrganizationTeam;
    effective_members: {
      user_id: string;
      role: string;
      reason: string;
      source_team_id: string;
    }[];
  }[];
  agents: OrganizationAgent[];
  events?: OrganizationEvent[];
};
export type OrganizationPortfolio = {
  organization: Organization;
  repositories: Repository[];
  packages: PackageVersion[];
  active_proposals: Proposal[];
  active_pulls: PullRequest[];
  releases: ReleaseCandidate[];
  active_incidents: Incident[];
  initiatives: OrganizationInitiative[];
};
export type ReleaseCandidate = {
  id: string;
  repository_id: string;
  version: string;
  notes: string;
  commit_id: string;
  previous_release_id?: string;
  previous_commit_id?: string;
  status: "candidate";
  created_by: string;
  created_at: string;
  inclusions: {
    pull_request_ids: string[];
    proposal_ids: string[];
    task_ids: string[];
    contributor_ids: string[];
  };
};
export type PackageVersion = {
  id: string;
  name: string;
  version: string;
  repository_id: string;
  release_id: string;
  source_commit: string;
  build_id: string;
  build_attestation: {
    step: string;
    image: string;
    command: string;
    attempt: number;
    state: string;
  };
  artifact_id: string;
  artifact_path: string;
  content_type: string;
  size: number;
  sha256: string;
  platform: { os?: string; architecture?: string; runtime?: string };
  dependencies: { name: string; constraint: string }[];
  summary?: string;
  documentation?: string;
  license?: string;
  support?: string;
  publisher_id: string;
  visibility: "public" | "private";
  lifecycle: "active" | "deprecated" | "quarantined" | "yanked";
  lifecycle_warning?: string;
  lifecycle_reason?: string;
  replacement?: { name: string; version: string };
  lifecycle_changed_by?: string;
  lifecycle_changed_at?: string;
  published_at: string;
};
export type DependencyInventory = {
  inventory: {
    id: string;
    repository_id: string;
    commit_id: string;
    recorded_by: string;
    recorded_at: string;
    entries: {
      name: string;
      version: string;
      constraint?: string;
      direct: boolean;
      paths: string[];
      package_id?: string;
      license?: string;
      support?: string;
      state: "resolved" | "stale" | "unresolved";
      provenance_gaps?: string[];
    }[];
  };
  current: boolean;
  releases: ReleaseCandidate[];
  builds: { id: string; state: string; artifact_id?: string }[];
  deployments: {
    id: string;
    environment_id: string;
    release_id: string;
    artifact_id: string;
    state: string;
    current: boolean;
  }[];
  remediation?: {
    status: string;
    proposal_id: string;
    task_id: string;
    replacement_version: string;
  };
};
export type PackageUpdate = {
  id: string;
  repository_id: string;
  package_name: string;
  from_version: string;
  to_version: string;
  base_commit: string;
  proposal_id: string;
  task_id: string;
  manifest: {
    version: number;
    dependencies: { name: string; constraint: string }[];
    lock: { name: string; version: string }[];
  };
  release_notes: string;
  compatibility_evidence: {
    step: string;
    image: string;
    command: string;
    attempt: number;
    state: string;
  };
  affected_dependency_paths: string[];
  created_by: string;
  created_at: string;
};
export type PackageUpdatePolicy = {
  repository_id: string;
  package_name: string;
  strategy: "patch" | "minor" | "major";
  action: "proposal";
  updated_by: string;
  updated_at: string;
};
export type InterfacePublication = {
  id: string;
  repository_id: string;
  name: string;
  version: string;
  release_id: string;
  commit_id: string;
  published_by: string;
  published_at: string;
  stale: boolean;
  stale_reason?: string;
};
export type DependencyRelationship = {
  id: string;
  repository_id: string;
  commit_id: string;
  release_id?: string;
  environment_id?: string;
  provider_repository_id: string;
  interface_name: string;
  constraint: string;
  declared_by: string;
  declared_at: string;
  resolved_interface_id?: string;
  resolved_version?: string;
  state: "resolved" | "stale" | "unresolved";
  reason?: string;
};
export type RelationshipGraph = {
  root_repository_id: string;
  repositories: Pick<Repository, "id" | "name" | "owner_id" | "visibility">[];
  interfaces: InterfacePublication[];
  dependencies: DependencyRelationship[];
};
export type EvolutionPlan = {
  id: string;
  repository_id: string;
  interface_name: string;
  predecessor: InterfacePublication;
  source_kind: "proposal" | "pull_request";
  source_id: string;
  candidate_commit_id?: string;
  candidate_description: string;
  changes: {
    kind: string;
    summary: string;
    classification: "compatible" | "conditional" | "breaking" | "unknown";
  }[];
  impacts: {
    repository_id: string;
    owner_id: string;
    dependency_id: string;
    commit_id: string;
    constraint: string;
    state: string;
  }[];
  strategy: string;
  sequencing: string;
  exceptions?: string;
  created_by: string;
  version: number;
  created_at: string;
  updated_at: string;
  findings: {
    id: string;
    actor_id: string;
    repository_ids: string[];
    finding: string;
    uncertainty?: string;
    created_at: string;
  }[];
  acknowledgements: {
    actor_id: string;
    repository_id: string;
    note?: string;
    created_at: string;
  }[];
  analyses: {
    id: string;
    agent_id: string;
    initiator_id: string;
    mandate: string;
    repository_ids: string[];
    created_at: string;
  }[];
  migration_tasks: {
    id: string;
    repository_id: string;
    proposal_id: string;
    task_id: string;
    target_version: string;
    dependency_ids: string[];
    created_by: string;
    created_at: string;
    status?: ProposalTask["status"] | "unavailable";
    ready: boolean;
    assignment_id?: string;
    assignee_type?: "human" | "agent";
    assignee_id?: string;
    base_revision?: string;
    branch?: string;
    pull_request_id?: string;
    contribution_status?: "review" | "merged" | "closed" | "superseded";
  }[];
  contract_candidates: {
    id: string;
    combination_hash: string;
    synthetic_commit: string;
    revisions: {
      role: "provider" | "consumer";
      repository_id: string;
      pull_request_id: string;
      source_repository_id: string;
      commit_id: string;
    }[];
    check_run_ids: string[];
    requested_by: string;
    created_at: string;
    superseded_at?: string;
    superseded_by?: string;
  }[];
  rollout?: {
    candidate_id: string;
    state?: "blocked" | "active" | "paused" | "completed";
    next_action?: string;
    configured_by: string;
    configured_at: string;
    approvals: {
      repository_id: string;
      actor_id: string;
      created_at: string;
    }[];
    phases: {
      id: string;
      name: string;
      repository_ids: string[];
      migration_task_ids: Record<string, string>;
      environment_ids?: Record<string, string>;
      state?:
        "blocked" | "awaiting_approval" | "ready" | "paused" | "completed";
      next_action?: string;
    }[];
    outcomes: {
      phase_id: string;
      repository_id: string;
      pull_request_id?: string;
      release_id?: string;
      deployment_id?: string;
      state: string;
    }[];
  };
};
export type ReleaseArtifact = {
  id: string;
  attempt: number;
  path: string;
  size: number;
  sha256: string;
  content_type: string;
  created_at: string;
};
export type ReleaseBuild = {
  id: string;
  commit_id: string;
  state:
    | "queued"
    | "running"
    | "cleanup_pending"
    | "succeeded"
    | "failed"
    | "canceled";
  definition: {
    name: string;
    image: string;
    command: string;
    working_directory?: string;
  };
  failure?: string;
  requested_by?: string;
  created_at: string;
  completed_at?: string;
  attempts: {
    number: number;
    state: string;
    actor_id?: string;
    exit_code?: number;
    failure?: string;
  }[];
  artifacts: ReleaseArtifact[];
};
export type ReleaseAttestation = {
  version: number;
  release_id: string;
  repository_id: string;
  source_commit: string;
  build_id: string;
  step: string;
  command: string;
  dependencies: string[];
  actor_id: string;
  verification: {
    state: string;
    exit_code?: number;
    failure?: string;
    attempts: ReleaseBuild["attempts"];
  };
  artifacts: ReleaseArtifact[];
  created_at: string;
  completed_at?: string;
};
export type DeploymentEnvironment = {
  id: string;
  repository_id: string;
  name: string;
  position: number;
  image: string;
  command: string;
  timeout_seconds: number;
  configuration: Record<string, string>;
  credential_names: string[];
  required_approvals: number;
  concurrency: number;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
};
export type Deployment = {
  id: string;
  repository_id: string;
  environment_id: string;
  release_id: string;
  build_id: string;
  artifact_id: string;
  artifact_sha256: string;
  commit_id: string;
  state:
    | "pending_approval"
    | "queued"
    | "running"
    | "paused"
    | "canceled"
    | "succeeded"
    | "failed";
  rollout: {
    version: number;
    stages: {
      name: string;
      observation_seconds: number;
      signals: { name: string; command: string }[];
    }[];
  };
  current_stage: number;
  evidence: {
    stage: string;
    signal: string;
    state: "passed" | "failed";
    message?: string;
    created_at: string;
  }[];
  initiated_by: string;
  approvals: { actor_id: string; created_at: string }[];
  events: {
    sequence: number;
    kind: string;
    actor_id?: string;
    state?: string;
    message?: string;
    created_at: string;
  }[];
  created_at: string;
  started_at?: string;
  completed_at?: string;
  recovery_of?: string;
  recovery_kind?: "rollback";
  restores_deployment_id?: string;
};
export type Incident = {
  id: string;
  title: string;
  summary: string;
  severity: "sev1" | "sev2" | "sev3" | "sev4";
  status: "investigating" | "identified" | "monitoring" | "resolved";
  scopes: { repository_id: string; environment_ids: string[] }[];
  roles: { name: string; user_id: string }[];
  source?: {
    repository_id: string;
    deployment_id: string;
    stage?: string;
    signal?: string;
  };
  declared_by: string;
  review?: {
    impact: string;
    timeline: string;
    contributing_factors: string[];
    conclusions: string;
    published_by: string;
    published_at: string;
  };
  commitments: {
    id: string;
    repository_id: string;
    proposal_id: string;
    task_id: string;
    assignee_id: string;
    due_at: string;
    created_by: string;
    created_at: string;
    progress: {
      state:
        | "committed"
        | "assigned"
        | "review"
        | "completed"
        | "overdue"
        | "invalidated";
      pull_request_id?: string;
      check_states?: string[];
      release_ids?: string[];
      deployment_ids?: string[];
    };
  }[];
  timeline: {
    id: string;
    kind: string;
    actor_id: string;
    message: string;
    audience: "participants" | "public";
    created_at: string;
    acknowledged_by?: string[];
    evidence?: IncidentEvidence[];
    investigation_id?: string;
  }[];
  investigations: IncidentInvestigation[];
  actions: IncidentAction[];
  version: number;
  created_at: string;
  updated_at: string;
  resolved_at?: string;
};
export type Issue = {
  id: string;
  repository_id: string;
  release_id?: string;
  affected_version?: string;
  title: string;
  expected_behavior: string;
  observed_behavior: string;
  severity: "low" | "medium" | "high" | "critical";
  environment: string;
  reproduction_steps: string[];
  visibility: "public" | "repository";
  status: "open" | "triaged" | "in_progress" | "resolved" | "closed";
  reporter_id: string;
  attachments: {
    id: string;
    kind: "log" | "screenshot" | "trace" | "sample";
    name: string;
    media_type: string;
    size: number;
    data: string;
    created_at: string;
  }[];
  discussion: { id: string; author_id: string; body: string; created_at: string }[];
  history: { id: string; kind: string; actor_id: string; from?: string; to?: string; message?: string; created_at: string }[];
  reproduction_attempts: {
    id: string; workspace_id: string; commit_id: string; release_id?: string; definition_sha256: string;
    environment_definition: { image: string; tools: { name: string; version: string }[]; dependencies: string[]; resources: { cpus: number; memory_mb: number; storage_mb: number; setup_seconds: number } };
    inputs: { attachment_id: string; name: string; sha256: string; size: number }[];
    commands: { name: string; outcome_id: string; command_sha256: string; exit_code: number; log?: string; started_at: string; completed_at: string }[];
    artifacts: { name: string; media_type: string; sha256: string; size: number; data?: string }[];
    observed_result: string; result: "reproduced" | "not_reproduced" | "inconclusive"; reproduced_by: string; created_at: string;
  }[];
  triage: { classification?: "bug" | "regression" | "performance" | "compatibility" | "documentation" | "support" | "unknown"; priority?: "low" | "normal" | "high" | "urgent"; assignee_id?: string; suspected_revision?: string; suspected_owner_ids?: string[]; updated_by?: string; updated_at?: string };
  links: { id: string; kind: "code" | "dependency" | "release" | "deployment" | "incident" | "proposal" | "pull_request" | "issue"; repository_id?: string; resource_id: string; revision?: string; label: string; added_by: string; created_at: string }[];
  evidence_requests: { id: string; body: string; requested_from: string; requested_by: string; state: "open" | "answered"; response?: string; responded_by?: string; created_at: string; updated_at: string }[];
  findings: { id: string; kind: "hypothesis" | "finding" | "uncertainty"; statement: string; actor_id: string; investigation_id?: string; citation_ids: string[]; supersedes_id?: string; challenges?: { id: string; actor_id: string; body: string; created_at: string }[]; created_at: string }[];
  investigations: { id: string; agent_id: string; initiator_id: string; mandate: string; reproduction_attempt_id: string; link_ids: string[]; state: string; created_at: string; updated_at: string }[];
  implementation?: { proposal_id: string; task_id: string; reproduction_attempt_id: string; finding_ids: string[]; affected_revision: string; acceptance_criteria: string[]; created_by: string; created_at: string };
  repair_verifications?: { id:string; pull_request_id:string; candidate_commit_id:string; reproduction_attempt_id:string; definition_sha256:string; input_sha256s:string[]; acceptance_criteria:string[]; required_run_ids:string[]; reproduction_run_ids:string[]; requested_by:string; preview_allowed:boolean; preview_workspace_id?:string; decisions:{id:string;kind:"confirmed"|"rejected"|"maintainer_override";actor_id:string;commit_id:string;rationale:string;created_at:string}[]; created_at:string }[];
  delivery_resolution?: { id:string; repair_verification_id:string; release_id:string; release_version:string; release_commit_id:string; deployment_id:string; environment_id:string; artifact_sha256:string; reporter_decision_id:string; recorded_by:string; created_at:string };
  duplicate_of?: string;
  version: number;
  created_at: string;
  updated_at: string;
};
export type SecurityAdvisory = {
  id: string;
  title: string;
  description: string;
  affected_repositories: { repository_id: string; versions: string[] }[];
  evidence: {
    id?: string;
    kind?:
      "commit" | "dependency" | "build" | "artifact" | "release" | "deployment";
    repository_id?: string;
    commit_id?: string;
    release_id?: string;
    build_id?: string;
    artifact_id?: string;
    deployment_id?: string;
    dependency?: string;
    label: string;
    description: string;
    captured_at?: string;
  }[];
  contact: string;
  reporter_id: string;
  response_team: string[];
  severity: "untriaged" | "low" | "moderate" | "high" | "critical";
  embargo_state:
    "reported" | "triaging" | "embargoed" | "coordinating" | "disclosed";
  messages: {
    id: string;
    actor_id: string;
    body: string;
    created_at: string;
  }[];
  access_log: {
    id: string;
    actor_id: string;
    action: string;
    detail?: string;
    created_at: string;
  }[];
  findings: {
    id: string;
    kind: "hypothesis" | "conclusion" | "uncertainty";
    actor_id: string;
    statement: string;
    evidence_ids: string[];
    investigation_id?: string;
    created_at: string;
  }[];
  impact_matrix: {
    repository_id: string;
    version_line: string;
    environment: string;
    state: "confirmed" | "suspected" | "unaffected" | "fixed";
    evidence_ids: string[];
    rationale: string;
    actor_id: string;
    updated_at: string;
  }[];
  investigations: {
    id: string;
    agent_id: string;
    initiator_id: string;
    mandate: string;
    state: string;
    evidence: SecurityAdvisory["evidence"];
    created_at: string;
    updated_at: string;
  }[];
  repair_tasks: {
    id: string;
    repository_id: string;
    version_line: string;
    title: string;
    mandate: string;
    base_commit_id: string;
    assignee_id: string;
    assignee_kind: "human" | "agent";
    dependency_task_ids: string[];
    status: "open" | "review";
    created_by: string;
    created_at: string;
  }[];
  repair_sessions: {
    id: string;
    task_id: string;
    repository_id: string;
    initiator_id: string;
    worker_id: string;
    branch: string;
    base_commit_id: string;
    commit_id?: string;
    state: "active" | "completed" | "revoked";
    comments: {
      id: string;
      actor_id: string;
      body: string;
      created_at: string;
    }[];
    reviews: {
      id: string;
      actor_id: string;
      decision: "approve" | "request_changes";
      body: string;
      commit_id: string;
      created_at: string;
    }[];
    created_at: string;
    updated_at: string;
  }[];
  security_reproductions: {
    id: string;
    repository_id: string;
    version_line: string;
    definition: {
      name: string;
      image: string;
      command: string;
      working_directory?: string;
      timeout_seconds?: number;
    };
    created_by: string;
    created_at: string;
  }[];
  repair_verifications: {
    id: string;
    task_id: string;
    session_id: string;
    repository_id: string;
    version_line: string;
    candidate_commit_id: string;
    required_run_ids: string[];
    reproduction_run_ids: string[];
    requested_by: string;
    approvals: { id: string; actor_id: string; created_at: string }[];
    created_at: string;
  }[];
  release_attestations: {
    id: string;
    verification_id: string;
    repository_id: string;
    version_line: string;
    release_id: string;
    release_commit_id: string;
    artifact_ids: string[];
    artifact_sha256: string[];
    actor_id: string;
    created_at: string;
  }[];
  disclosure?: {
    id: string;
    state: "ready" | "scheduled" | "publishing" | "paused" | "published";
    public_title: string;
    redacted_summary: string;
    upgrade_guidance: string;
    credits: string[];
    affected_versions: { repository_id: string; versions: string[] }[];
    fixed_versions: {
      repository_id: string;
      version_line: string;
      release_id: string;
      commit_id: string;
      branch: string;
      artifact_ids: string[];
      artifact_sha256: string[];
    }[];
    scheduled_at?: string;
    remaining: string[];
    failure?: string;
    created_by: string;
    created_at: string;
    published_at?: string;
  };
  version: number;
  created_at: string;
  updated_at: string;
};
export type IncidentAction = {
  id: string;
  operation_id: string;
  kind: "pause_rollout" | "restore_release" | "emergency_repair";
  repository_id: string;
  deployment_id: string;
  rationale: string;
  status:
    "proposed" | "approved" | "rejected" | "executing" | "failed" | "recovered";
  proposed_by: string;
  evidence: IncidentEvidence[];
  health_criteria: { stage: string; signal: string }[];
  decisions: {
    actor_id: string;
    decision: "approve" | "reject";
    message: string;
    override?: boolean;
    created_at: string;
  }[];
  attempts: {
    id: string;
    actor_id: string;
    outcome: "pending" | "started" | "failed" | "recovered";
    resource_id?: string;
    message: string;
    created_at: string;
  }[];
  created_at: string;
  updated_at: string;
};
export type IncidentInvestigation = {
  id: string;
  agent_id: string;
  initiator_id: string;
  mandate: string;
  state: "running" | "paused" | "cancelled";
  evidence: IncidentEvidence[];
  revisions: { repository_id: string; commit_id: string; label: string }[];
  access: string[];
  created_at: string;
  updated_at: string;
};
export type IncidentEvidence = {
  kind:
    | "log"
    | "health_signal"
    | "deployment"
    | "release"
    | "commit"
    | "pull_request"
    | "incident";
  repository_id: string;
  resource_id: string;
  label: string;
  query?: string;
  window_start?: string;
  window_end?: string;
  captured_at: string;
};
export type ForkSynchronization = {
  branch: string;
  previous_commit_id?: string;
  commit_id: string;
  upstream_repository_id: string;
};
export type Collaborator = {
  user_id: string;
  role: "contributor";
  created_at: string;
};
export type Credential = {
  id: string;
  user_id: string;
  kind: "session" | "api" | "git";
  name: string;
  scopes: string[];
  created_at: string;
  expires_at: string;
  last_used_at?: string;
  revoked_at?: string;
  repository_id?: string;
  git_write_branch?: string;
  pull_request_id?: string;
  token?: string;
};
export type Branch = { name: string; commit_id: string };
export type Commit = {
  id: string;
  tree_id: string;
  parent_ids: string[];
  message: string;
  author: string;
  authored_at: string | null;
};
export type TreeEntry = {
  name: string;
  mode: string;
  id: string;
  type: "blob" | "tree" | "commit";
};
export type Proposal = {
  id: string;
  repository_id: string;
  author_id: string;
  title: string;
  body: string;
  status: "open" | "closed";
  created_at: string;
  updated_at: string;
  closed_at?: string;
  reasoning?: ReasoningOrigin;
};
export type ProposalComment = {
  id: string;
  proposal_id: string;
  author_id: string;
  body: string;
  created_at: string;
};
export type ProposalTask = {
  id: string;
  proposal_id: string;
  title: string;
  outcome: string;
  risk?: string;
  verification_plan?: string;
  status: "todo" | "in_progress" | "completed" | "cancelled";
  position: number;
  dependency_ids: string[];
  discussion_comment_ids: string[];
  ready: boolean;
  blocked_by: string[];
  context_revision: number;
  context_state: "current" | "changed" | "obsolete";
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
  assignment?: {
    id: string;
    assignee_type: "human" | "agent";
    assignee_id: string;
    mandate: string;
    access: {
      repository_id: string;
      base_revision: string;
      scopes: string[];
      branch: string;
    };
    assigned_by: string;
    assigned_at: string;
    context_revision: number;
  };
  contribution?: {
    pull_request_id: string;
    session_id?: string;
    run_id?: string;
    source_commit_id: string;
    commit_ids: string[];
    status: "review" | "merged" | "closed" | "superseded";
    context_revision: number;
  };
  reasoning?: ReasoningOrigin;
};
export type ReasoningOrigin = {
  assessment_id: string;
  assessment_version: number;
  revision: string;
  explanation_id?: string;
  conclusion_entry_id?: string;
  selected_item_ids: string[];
  items: { id: string; kind: string; summary: string; status: string }[];
  acknowledgements: {
    request_id: string;
    repository_id: string;
    owner_id: string;
    acknowledged_by: string;
    note?: string;
  }[];
  analysis_status: string;
  organization_id?: string;
  mandate_id?: string;
  opportunity_id?: string;
};
export type ProposalTaskChange = {
  id: string;
  task_id: string;
  actor_id: string;
  action:
    | "created"
    | "updated"
    | "status_changed"
    | "reordered"
    | "assigned"
    | "reassigned"
    | "rebased"
    | "assignment_revoked"
    | "contribution_published"
    | "contribution_merged"
    | "contribution_closed"
    | "contribution_superseded";
  task: ProposalTask;
  created_at: string;
};
export type PullRequest = {
  id: string;
  repository_id: string;
  source_repository_id: string;
  author_id: string;
  title: string;
  body: string;
  source_branch: string;
  target_branch: string;
  source_commit_id: string;
  target_commit_id: string;
  proposal_id: string | null;
  task_id?: string;
  task_session_id?: string;
  task_run_id?: string;
  task_evidence?: {
    base_revision: string;
    assignment_id: string;
    agent_id: string;
    initiator_id: string;
    mandate: string;
    organization_id?: string;
    mandate_id?: string;
    opportunity_id?: string;
    evidence_revision?: string;
    reasoning: { id: string; kind: string; summary: string; status: string }[];
    completion_criteria: string;
    outcome: {
      summary: string;
      commit_id: string;
      commits: string[];
      changed_files: { path: string; status: string }[];
      checks: { name: string; status: string; details?: string }[];
      commands: { command: string; exit_code: number; summary?: string }[];
      completion_criteria: {
        criterion: string;
        status: "met" | "partial" | "not_met";
        evidence: string;
      }[];
      unresolved_concerns: string[];
      completed_at: string;
    };
  };
  workspace_id?: string;
  workspace_checkpoint_id?: string;
  workspace_contributor_ids?: string[];
  workspace_command_ids?: string[];
  contribution_evidence?: {
    opportunity_id: string; opportunity_version: number; pathway_version: number; upstream_revision: string;
    setup_evidence: { command: string; state: string; exit_code: number }[];
    mentor_guidance_ids: string[]; agent_assistance_ids: string[];
    acceptance_criteria: string[]; satisfied_criteria: string[];
    project_requirements: ContributionPublicationFinding[]; coaching_needs: ContributionPublicationFinding[];
  };
  status: "open" | "closed" | "merged";
  maintainer_edits_allowed: boolean;
  created_at: string;
  updated_at: string;
  merged_at?: string | null;
  merged_by?: string | null;
  merge_commit_id?: string | null;
  closed_at?: string | null;
  closed_by?: string | null;
  queued_at?: string | null;
};
export type PullRequestCommit = {
  id: string;
  tree_id: string;
  parent_ids: string[];
  headers: { name: string; value: string }[];
  message: string;
};
export type PullPreview = {
  id: string; repository_id: string; pull_request_id: string; revision: string; creator_id: string;
  definition_sha256: string; build_run_id: string; state: string; stale: boolean; url: string; created_at: string; updated_at: string;
  invitations: { id:string; user_id:string; role:"view"|"test"|"feedback"; source_kind:"user"|"issue"|"decision"|"proposal"; source_id?:string; expires_at:string; revoked_at?:string }[];
	findings: PreviewFinding[];
  definition: { version: number; image: string; build: string; working_directory?: string; output_path: string; environment?: Record<string,string>; access:{network:"none";data:"preview_artifacts";identity:"named_users";actions:("view"|"test"|"feedback")[]}; resources: { cpus: number; memory_mb: number; storage_mb: number; timeout_seconds: number } };
};
export type PreviewFinding = {
  id:string; preview_id:string; revision:string; route:string; title:string; description:string;
  classification:"bug"|"usability"|"accessibility"|"content"|"performance"|"question"|"other";
  severity:"blocking"|"major"|"minor"|"note"; status:"open"|"resolved"; duplicate_of?:string;
  reproduction_steps:string[]; author_id:string; version:number; created_at:string; updated_at:string;
  evidence:{id:string;kind:"screenshot"|"recording"|"console"|"trace"|"annotation";name:string;media_type:string;size:number;data?:string;redacted:boolean}[];
  comments:{id:string;author_id:string;body:string;created_at:string}[];
  repair?: { session_id:string; acceptance_criteria:string[]; created_by:string; created_at:string; published_commit_id?:string; preview_attempt_id?:string };
};
export type FileChange = {
  path: string;
  status: "added" | "modified" | "deleted";
  old_id: string | null;
  new_id: string | null;
  old_mode: string | null;
  new_mode: string | null;
};
export type PullRequestComment = {
  id: string;
  pull_request_id: string;
  author_id: string;
  body: string;
  created_at: string;
};
export type PullRequestReview = {
  id: string;
  pull_request_id: string;
  reviewer_id: string;
  decision: "approved" | "changes_requested" | "withdrawn";
  reviewed_commit_id: string;
  stale: boolean;
  created_at: string;
  updated_at: string;
};
export type PullRequestBranchState = {
  branch: string;
  snapshot_commit_id: string;
  current_commit_id: string | null;
  state: "current" | "advanced" | "rewritten" | "missing" | "unavailable";
};
export type MergeReadiness = {
  mergeable: boolean;
  can_merge: boolean;
  required_approvals: number;
  approvals: number;
  evaluated_commit_id: string;
  required_checks: {
    name: string;
    status: "missing" | "pending" | "failed" | "cancelled" | "stale" | "passed";
    commit_id?: string;
    run_id?: string;
  }[];
  source: PullRequestBranchState;
  target: PullRequestBranchState;
  has_conflicts: boolean;
  blockers: { code: string; message: string }[];
  preview_acceptance?: {
    revision: string;
    policy_version: number;
    applicable: { id: string; paths: string[]; risk_classes: string[]; scenarios: { name: string; role: "owner" | "contributor" | "author" | "stakeholder"; blocking: boolean }[] }[];
    decisions: { id: string; revision: string; policy_version: number; idempotency_key: string; requirement_id: string; scenario: string; role: string; outcome: "accepted" | "rejected" | "overridden"; rationale?: string; actor_id: string; created_at: string }[];
    stale_decisions: { id: string; revision: string; policy_version: number; idempotency_key: string; requirement_id: string; scenario: string; role: string; outcome: string; rationale?: string; actor_id: string; created_at: string }[];
    findings: { id: string; preview_id: string; revision: string; title: string; severity: string; status: string; author_id: string }[];
    missing: { requirement_id: string; scenario: string; role: string }[];
    blocking: boolean;
  };
  integration_queue?: {
    branch: string;
    enabled: boolean;
    concurrency: number;
    failure_behavior: "pause" | "remove";
    required_checks: string[];
    required_approvals: number;
  };
  can_enqueue: boolean;
};
export type CheckAttempt = {
  number: number;
  state: string;
  started_at: string;
  completed_at?: string;
  exit_code?: number;
  failure?: string;
  actor_id?: string;
};
export type CheckArtifact = {
  id: string;
  attempt: number;
  path: string;
  size: number;
  sha256: string;
  content_type: string;
  created_at: string;
};
export type CheckRun = {
  id: string;
  repository_id: string;
  pull_request_id: string;
  commit_id: string;
  definition: { name: string; image: string; command: string; documentation?: { check: string; collection_id: string; version: string; revision: string; source: "source" | "package" | "release"; selectors: string[]; dependency_paths: string[]; dependency_sha256: string } };
  state: string;
  failure?: string;
  created_at: string;
  attempts: CheckAttempt[];
  artifacts: CheckArtifact[];
};
export type IntegrationCandidate = {
  id: string;
  source_commit_id: string;
  base_commit_id: string;
  commit_id: string;
  required_checks: string[];
  check_definitions: CheckRun["definition"][];
  created_at: string;
  state: "pending" | "verifying" | "passed" | "failed" | "superseded";
  checks: CheckRun[];
};
export type IntegrationQueueEntry = {
  position: number;
  pull_request: PullRequest & {
    queued_by?: string;
    queue_paused?: boolean;
    queue_actions?: { action: string; actor_id: string; created_at: string }[];
  };
  candidate?: IntegrationCandidate;
  state: string;
  blockers: { code: string; message: string }[];
  next_action: string;
};
export type CheckEvent = {
  sequence: number;
  attempt: number;
  kind: "status" | "command" | "log" | "artifact" | "control";
  timestamp: string;
  state?: string;
  stream?: string;
  message?: string;
  actor_id?: string;
  artifact?: CheckArtifact;
};
export type ChangeSession = {
  id: string;
  repository_id: string;
  pull_request_id: string;
  initiator_id: string;
  source_commit_id: string;
  preview_evidence?: { preview_id:string; finding_id:string; revision:string; route:string; title:string; description?:string; classification:string; severity:string; author_id:string; reproduction_steps:string[]; acceptance_criteria:string[]; evidence:{id:string;kind:string;name:string;media_type:string;size:number;data?:string;redacted:boolean}[]; discussion:{id:string;author_id:string;body:string;created_at:string}[] };
  check_evidence?: {
    run_id: string;
    definition: {
      name: string;
      image: string;
      command: string;
      working_directory?: string;
      environment?: Record<string, string>;
      timeout_seconds?: number;
    };
    events: {
      sequence: number;
      attempt: number;
      kind: string;
      state?: string;
      stream?: string;
      message?: string;
      exit_code?: number;
    }[];
    artifacts: CheckArtifact[];
  };
  deployment_evidence?: {
    deployment_id: string;
    release_id: string;
    environment_id: string;
    artifact_id: string;
    artifact_sha256: string;
    commit_id: string;
    state: string;
    release_version: string;
    release_notes: string;
    current_stage: number;
    evidence: {
      stage: string;
      signal: string;
      state: string;
      message?: string;
    }[];
    events: {
      sequence: number;
      kind: string;
      actor_id?: string;
      state?: string;
      message?: string;
    }[];
  };
  state: "open";
  created_at: string;
  updated_at: string;
};
export type ChangeSessionEvent = {
  id: string;
  session_id: string;
  kind:
    | "session.opened"
    | "run.launched"
    | "run.status"
    | "agent.message"
    | "agent.question"
    | "tool.action"
    | "artifact.produced"
    | "run.failed"
    | "branch.updated"
    | "run.guidance"
    | "question.answered"
    | "run.paused"
    | "run.resumed"
    | "run.canceled"
    | "run.completed";
  actor_id: string;
  state: string;
  run_id?: string;
  initiator_id?: string;
  agent_id?: string;
  revision_id?: string;
  message?: string;
  tool?: string;
  artifact?: string;
  branch?: string;
  commit_id?: string;
  created_at: string;
};
export type AgentRun = {
  id: string;
  session_id: string;
  initiator_id: string;
  agent_id: string;
  instructions: string;
  source_commit_id: string;
  context_paths: string[];
  working_branch: string;
  credential_id: string;
  credential_expires_at: string;
  access_revoked_at?: string;
  state: "launched" | "paused" | "canceled" | "completed";
  outcome?: {
    summary: string;
    commit_id: string;
    commits: string[];
    changed_files: { path: string; status: "added" | "modified" | "deleted" }[];
    checks: {
      name: string;
      status: "passed" | "failed" | "skipped";
      details?: string;
    }[];
    commands: { command: string; exit_code: number; summary?: string }[];
    completion_criteria: {
      criterion: string;
      status: "met" | "partial" | "not_met";
      evidence: string;
    }[];
    unresolved_concerns: string[];
    completed_at: string;
  };
  created_at: string;
  updated_at: string;
};
export type ActivityEvent = {
  id: string;
  kind:
    | "proposal.created"
    | "proposal.updated"
    | "proposal.closed"
    | "proposal.commented"
    | "pull_request.created"
    | "pull_request.synchronized"
    | "pull_request.commented"
    | "pull_request.merged"
    | "review.approved"
    | "review.changes_requested"
    | "review.withdrawn"
    | "mention.created"
    | "access.granted"
    | "access.revoked"
    | "deployment.pause"
    | "deployment.resume"
    | "deployment.cancel"
    | "deployment.mark_unsuccessful"
    | "incident.declared";
  actor_id: string;
  repository_id: string;
  repository_name: string;
  resource_type:
    "proposal" | "pull_request" | "repository" | "deployment" | "incident";
  resource_id: string;
  resource_title: string;
  target_user_id: string | null;
  created_at: string;
};
export type InboxItem = ActivityEvent & {
  category: "review" | "response" | "awareness";
  action: string;
};

export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
    public code?: string,
  ) {
    super(message);
  }
}

export type APIResponse<T> = {
  data: T;
  status: number;
  headers: Headers;
};

export async function apiResponse<T>(
  path: string,
  init: RequestInit = {},
  token?: string | null,
): Promise<APIResponse<T>> {
  const headers = new Headers(init.headers);
  if (init.body) headers.set("Content-Type", "application/json");
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(`/api${path}`, {
    ...init,
    headers,
    cache: "no-store",
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as {
      error?: { code?: string; message?: string };
    } | null;
    throw new APIError(
      body?.error?.message ?? "The request could not be completed.",
      response.status,
      body?.error?.code,
    );
  }
  const data =
    response.status === 204 ? (undefined as T) : ((await response.json()) as T);
  return { data, status: response.status, headers: response.headers };
}

export async function api<T>(
  path: string,
  init: RequestInit = {},
  token?: string | null,
): Promise<T> {
  return (await apiResponse<T>(path, init, token)).data;
}
export type DocumentationDiagnostic = { code:string; severity:"error"|"warning"; page_path?:string; version_label?:string; detail:string };
export type DocumentationCollection = { id:string; repository_id:string; collection_id:string; version:number; name:string; description:string; root_path:string; source_ref:string; source_revision:string; audience:"public"|"repository"|"maintainers"; owners:{actor_id:string;role:string}[]; supported_versions:{label:string;source_ref:string;release_id?:string;revision?:string;status?:string;status_detail?:string}[]; navigation:{label:string;slug:string;position:number}[]; rendering:{format:string;syntax_highlighting:boolean;table_of_contents:boolean}; publication_policy:{review_required:boolean;source_branch:string;publish_on_merge:boolean}; pages:{path:string;slug:string;title:string;navigation_title?:string;position:number;source_object_id:string;source_sha256:string;authors:string[];links:{kind:string;label:string;resource_id?:string;symbol?:string;path?:string}[];status?:string;status_detail?:string}[]; published_by:string;published_at:string;diagnostics:DocumentationDiagnostic[] };
export type DocumentationReference={path?:string;start_line?:number;end_line?:number;revision:string;resource_kind?:string;resource_id?:string;label:string};
export type DocumentationTask={id:string;repository_id:string;title:string;path:string;branch:string;base_revision:string;source:{kind:string;resource_id:string;revision:string;label:string};created_by:string;created_at:string;version:number;drafts:{id:string;version:number;body:string;rendered_html:string;author_id:string;references:DocumentationReference[];created_at:string}[];entries:{id:string;kind:string;body:string;actor_id:string;agent_id?:string;draft_version:number;references?:DocumentationReference[];uncertain?:boolean;created_at:string}[];workspace_id?:string;published_collection_id?:string};
