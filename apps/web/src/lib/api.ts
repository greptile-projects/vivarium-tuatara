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
export type PullRequest = {
  id: string;
  repository_id: string;
  author_id: string;
  title: string;
  body: string;
  source_branch: string;
  target_branch: string;
  source_commit_id: string;
  target_commit_id: string;
  proposal_id: string | null;
  status: "open" | "merged";
  created_at: string;
  updated_at: string;
  merged_at?: string | null;
  merged_by?: string | null;
  merge_commit_id?: string | null;
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
  state: "current" | "advanced" | "rewritten" | "missing";
};
export type MergeReadiness = {
  mergeable: boolean;
  can_merge: boolean;
  required_approvals: number;
  approvals: number;
  source: PullRequestBranchState;
  target: PullRequestBranchState;
  has_conflicts: boolean;
  blockers: { code: string; message: string }[];
};
export type ChangeSession = {
  id: string;
  repository_id: string;
  pull_request_id: string;
  initiator_id: string;
  source_commit_id: string;
  state: "open";
  created_at: string;
  updated_at: string;
};
export type ChangeSessionEvent = {
  id: string;
  session_id: string;
  kind: "session.opened" | "run.launched" | "run.status" | "agent.message" | "agent.question" | "tool.action" | "artifact.produced" | "run.failed" | "branch.updated" | "run.guidance" | "question.answered" | "run.paused" | "run.resumed" | "run.canceled";
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
  state: "launched" | "paused" | "canceled";
  created_at: string;
  updated_at: string;
};
export type ActivityEvent = {
  id: string;
  kind: "proposal.created" | "proposal.updated" | "proposal.closed" | "proposal.commented" | "pull_request.created" | "pull_request.synchronized" | "pull_request.commented" | "pull_request.merged" | "review.approved" | "review.changes_requested" | "review.withdrawn" | "mention.created" | "access.granted" | "access.revoked";
  actor_id: string;
  repository_id: string;
  repository_name: string;
  resource_type: "proposal" | "pull_request" | "repository";
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
  const data = response.status === 204
    ? undefined as T
    : await response.json() as T;
  return { data, status: response.status, headers: response.headers };
}

export async function api<T>(
  path: string,
  init: RequestInit = {},
  token?: string | null,
): Promise<T> {
  return (await apiResponse<T>(path, init, token)).data;
}
