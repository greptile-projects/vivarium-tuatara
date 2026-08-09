export type User = {
  id: string;
  handle: string;
  display_name: string;
  created_at: string;
  updated_at: string;
};
export type Repository = {
  id: string;
  owner_id: string;
  name: string;
  visibility: "private" | "public";
  default_branch: string;
  git_remote: string;
  created_at: string;
  upstream_repository_id?: string;
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
      state: "committed" | "assigned" | "review" | "completed" | "overdue" | "invalidated";
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
export type SecurityAdvisory = {
  id: string;
  title: string;
  description: string;
  affected_repositories: { repository_id: string; versions: string[] }[];
  evidence: { id?: string; kind?: "commit" | "dependency" | "build" | "artifact" | "release" | "deployment"; repository_id?: string; commit_id?: string; release_id?: string; build_id?: string; artifact_id?: string; deployment_id?: string; dependency?: string; label: string; description: string; captured_at?: string }[];
  contact: string;
  reporter_id: string;
  response_team: string[];
  severity: "untriaged" | "low" | "moderate" | "high" | "critical";
  embargo_state: "reported" | "triaging" | "embargoed" | "coordinating";
  messages: { id: string; actor_id: string; body: string; created_at: string }[];
  access_log: { id: string; actor_id: string; action: string; detail?: string; created_at: string }[];
  findings: { id: string; kind: "hypothesis" | "conclusion" | "uncertainty"; actor_id: string; statement: string; evidence_ids: string[]; investigation_id?: string; created_at: string }[];
  impact_matrix: { repository_id: string; version_line: string; environment: string; state: "confirmed" | "suspected" | "unaffected" | "fixed"; evidence_ids: string[]; rationale: string; actor_id: string; updated_at: string }[];
  investigations: { id: string; agent_id: string; initiator_id: string; mandate: string; state: string; evidence: SecurityAdvisory["evidence"]; created_at: string; updated_at: string }[];
  repair_tasks: { id: string; repository_id: string; version_line: string; title: string; mandate: string; base_commit_id: string; assignee_id: string; assignee_kind: "human" | "agent"; dependency_task_ids: string[]; status: "open" | "review"; created_by: string; created_at: string }[];
  repair_sessions: { id: string; task_id: string; repository_id: string; initiator_id: string; worker_id: string; branch: string; base_commit_id: string; commit_id?: string; state: "active" | "completed" | "revoked"; comments: { id: string; actor_id: string; body: string; created_at: string }[]; reviews: { id: string; actor_id: string; decision: "approve" | "request_changes"; body: string; commit_id: string; created_at: string }[]; created_at: string; updated_at: string }[];
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
  status: "proposed" | "approved" | "rejected" | "executing" | "failed" | "recovered";
  proposed_by: string;
  evidence: IncidentEvidence[];
  health_criteria: { stage: string; signal: string }[];
  decisions: { actor_id: string; decision: "approve" | "reject"; message: string; override?: boolean; created_at: string }[];
  attempts: { id: string; actor_id: string; outcome: "pending" | "started" | "failed" | "recovered"; resource_id?: string; message: string; created_at: string }[];
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
  definition: { name: string; image: string; command: string };
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
