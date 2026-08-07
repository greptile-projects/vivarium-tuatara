# HTTP API contract

The JSON API is the supported application boundary for browsers, agents, and
external consumers. Durable files beneath the configured storage roots are
private implementation details. Git clients use the smart HTTP URLs returned
by repository resources.

## Conventions

- Requests and responses use JSON. Successful resource creation returns `201`
  and a `Location` header; deletion returns `204`.
- A mutation whose atomic rename is visible but whose parent-directory sync
  fails returns `202` with `Vivarium-Durability: uncertain` and the affected
  resource (including its stable ID). Clients must retain that identity and
  inspect it later rather than retrying the mutation as a new request.
- Resource `id` fields are opaque, permanent identities. Display names,
  handles, repository names, and Git remote paths are not attribution keys.
- API credentials use `Authorization: Bearer <token>`. The account bootstrap
  session is also set as the `vivarium_session` HttpOnly cookie. Git
  credentials are sent as an HTTP Basic password.
- JSON request bodies reject unknown fields, multiple values, and bodies over
  1 MiB. Validation failures do not partially mutate resources.
- JSON failures have the stable shape
  `{"error":{"code":"machine_readable_code","message":"human readable message"}}`.
  Consumers should branch on `code`, not `message`. Authentication failures
  are `401`, authorization deliberately hidden as not-found is `404`, invalid
  input is `400`, uniqueness conflicts are `409`, and unavailable durable
  storage is `500`.

Collection endpoints accept `limit` (default 30, range 1–100) and `after`.
Responses contain the resource array and `next_cursor`, which is `null` on the
last page. Pass a non-null `next_cursor` unchanged as the next request's
`after`; cursors outside the authenticated collection return
`invalid_pagination`. Collection order is oldest creation first, with opaque ID
as the deterministic tie-breaker.

## Activity

`GET /activity` returns a newest-first, cursor-paginated `events` collection
for the authenticated actor. It records meaningful proposal creation, editing,
closure, and discussion; pull request creation, synchronization, discussion,
review decisions, withdrawal, and merge; `@handle` mentions in proposal and
pull request text; and collaborator grant or revocation. Each event contains a
stable `actor_id`, `repository_id`, `resource_type`, `resource_id`, optional
`target_user_id`, timestamp, and snapshot labels for the repository and
resource. Every event requires current repository access, including targeted
mention and access events, so activity never reveals private resource metadata.

Activity is append-only beneath `ACTIVITY_STORAGE_ROOT`, which defaults to
`activity-records`. Repeating an idempotent collaborator grant or removal does not
create another event because shared state did not change.

## Inbox

`GET /inbox` returns the authenticated actor's newest-first actionable items
under `items`. Each item retains its underlying activity fields and adds a
`category` of `review`, `response`, or `awareness` plus a concrete `action`
label. The optional `category` query filters before the shared cursor
pagination contract is applied.

Inbox membership is recipient-specific and derived from current collaboration
state. Repository owners receive one review item per open pull request; a
synchronized revision replaces its older review action. Resource authors receive mentions, comments, and requested changes
that call for a response, plus approvals, merges, and owner-driven proposal
closures to acknowledge. Newly granted collaborators receive the access grant
for awareness. Revocation does not bypass current private-repository authorization
to retain inaccessible repository metadata. An actor's own events are excluded, completed work stops presenting
obsolete actions, and current repository authorization is checked on every
read.

`DELETE /inbox/{event_id}` clears one item for the authenticated actor and is
idempotent for an already-cleared item while it remains otherwise actionable.
Clearing persists per user beneath the activity storage root; it never removes
the immutable activity event or changes the underlying proposal, pull request,
review, or repository.

## Accounts

`POST /users` creates an account from `handle` and `display_name` and returns
`{"user": ..., "credential": ...}`. The credential secret appears only in
that creation response. `GET /user` resolves the currently authenticated
account without requiring a particular scope. `GET /users/{id}` is public and
resolves stable attribution. `PATCH /users/{id}` accepts a sparse `handle`
and/or `display_name` patch from that user with `profile:write`.

Handles are normalized lowercase strings of 1–39 letters, digits, or hyphens.
Display names are one line of 1–100 characters.

## Credentials

`GET /auth/credentials` is a paginated list of the current account's
credentials. `POST /auth/credentials` accepts `kind`, `name`, `scopes`, and an
`expires_in` lifetime in seconds. It returns the secret once. API credentials
may last at most 90 days and Git credentials at most 30 days. `DELETE
/auth/credentials/{id}` revokes one credential; `DELETE /auth/session` revokes
the calling session. Credential administration requires the session-only
`credentials:write` scope.

## Repositories

`POST /repositories` accepts `name` and creates a private repository owned by
the current account. `GET /repositories` paginates repositories that account
owns or can access through a current collaborator grant.
`GET /repositories/{id}` returns an owned private repository or any public
repository. `PATCH /repositories/{id}` accepts `visibility` as `private` or
`public`; `DELETE /repositories/{id}` removes the owned repository and its Git
remote.

Owners manage limited access with `GET` and `POST
/repositories/{id}/collaborators` and `DELETE
/repositories/{id}/collaborators/{user_id}`. A grant request contains an
existing `user_id`; collaborator resources contain that stable ID and
`role: "contributor"`. Granting the same user and revoking an absent grant are
idempotent. Only the owner may inspect or change grants.

Repository responses include immutable `id` and `owner_id`, user-facing
`name`, `visibility`, `default_branch`, `created_at`, and `git_remote`. Use the
returned `git_remote` relative to the API origin. Private reads and writes
require matching repository or Git credential scopes. Owners retain every
administrative power. Contributors can inspect and fetch a private repository
and can create, update, force-update, or delete non-default branches through
stock Git. They cannot update `main`, change visibility, manage access, or
delete the repository. Revocation takes effect on the next API or Git request.

Repository content reads use the same visibility and collaborator policy.
`GET /repositories/{id}/branches` lists direct branch names and commit IDs.
`GET /repositories/{id}/commits?ref=<revision>` returns the deduplicated commit
ancestry from a branch name or full lowercase commit ID through the shared
`limit`/`after` cursor contract;
each commit includes its tree, ordered parents, message, author header, and
parsed author time. `GET /repositories/{id}/tree?ref=<revision>&path=<path>`
lists the direct entries of the root or a nested directory with name, object
ID, type, and Git mode. `GET /repositories/{id}/blob` accepts the same query
and returns a text preview, total size, binary indicator, and `truncated`
flag for a file. Text previews are limited to 512 KiB. Every
revision-bearing response includes the resolved commit ID so clients can keep
branch-friendly navigation tied to exact repository state. `path` is relative
to the commit root and omitted for its top-level tree.
Commit cursor replay is capped at 200 inspected commits per request; cursors
deeper than that bound are rejected as invalid pagination. Blob previews stream
and verify the full loose object while retaining only the bounded preview.

## Proposals

Repository participants use proposals to establish shared context before or
alongside a code change. `POST /repositories/{id}/proposals` accepts `title`
and `body`; the title is a single line of at most 200 characters and the body
may contain at most 10,000 characters. The resulting resource has an opaque
`id`, immutable `repository_id` and `author_id`, `status: "open"`, and durable
creation and update timestamps. `GET /repositories/{id}/proposals` is
paginated, and `GET /repositories/{id}/proposals/{proposal_id}` inspects one.

The author can update `title` or `body` with `PATCH`; the repository owner can
also close any proposal by sending `status: "closed"`, as can its author. A
closed proposal records `closed_at` and cannot be reopened. Other contributors
cannot rewrite another person's proposal. Owners and contributors can append
feedback with `POST .../comments` using a non-empty `body`; `GET .../comments`
returns the attributable conversation in creation order with pagination.
Comments are immutable and contain stable `author_id` values, so later profile
edits do not alter conversation history.

Proposal reads follow repository visibility: public repository proposals and
comments are anonymously readable, while private reads require the owner or a
current contributor with `repositories:read`. Creation and updates require
`repositories:write`; commenting requires `repositories:read`. Public access
does not itself grant participation. Proposal records are private atomic JSON
files beneath `PROPOSAL_STORAGE_ROOT`, which defaults to `proposals`.
Proposal creates, edits, closes, and comments use the shared uncertain-
durability response when their new state is visible but crash persistence
cannot be confirmed, preserving attribution IDs without overstating storage.

## Pull requests

Repository participants open a pull request with `POST
/repositories/{id}/pulls`. The request requires `title`, `body`,
`source_branch`, and `target_branch`; `proposal_id` may link an existing
proposal in the same repository. Branch names are repository-relative (for
example, `feature`, not `refs/heads/feature`), must be different, and must both
currently identify commit objects. A missing, unborn, or non-commit branch is
rejected without creating a resource.

The created resource records immutable `repository_id` and `author_id`, its
purpose in `title` and `body`, the source and target branch names, and the exact
branch tips as `source_commit_id` and `target_commit_id`. These commit IDs do
not silently change when a branch advances. The author can explicitly adopt
the source branch's current commit as a new reviewable revision with `POST
/repositories/{id}/pulls/{pull_id}/synchronize`; this updates
`source_commit_id`, while the target snapshot remains fixed. Synchronization
requires `repositories:write`, rejects a missing or non-commit source branch,
and is unavailable after merge. A different participant receives not-found.
New pull requests have `status: "open"` and creation and update timestamps.
The linked `proposal_id` is nullable.

`GET /repositories/{id}/pulls` returns pull requests in the shared cursor-
paginated collection shape under `pull_requests`; `GET
/repositories/{id}/pulls/{pull_id}` inspects one. Reads inherit repository
visibility and collaborator access. Creation requires a current owner or
contributor with `repositories:write`; public readability alone does not
grant permission to open a pull request. Pull request metadata is stored as
private atomic JSON beneath `PULL_REQUEST_STORAGE_ROOT`, defaulting to
`pull-requests`, partitioned by repository ID so one repository's damaged
metadata cannot make another repository's collection unavailable. A create
whose rename is visible but directory durability is
uncertain returns the shared `202` response with its stable pull request ID.

`GET /repositories/{id}/pulls/{pull_id}/commits` returns `commits`, the commits
reachable from the pull request's recorded source revision but not from its
fixed target snapshot. Results follow depth-first parent order from the
source tip and expose each commit's `id`, `tree_id`, ordered `parent_ids`, raw
ordered Git `headers`, and exact `message`. The endpoint uses the fixed target
snapshot and explicitly synchronized source revision rather than silently
following current branch tips.

`GET /repositories/{id}/pulls/{pull_id}/files` compares those same two commit
trees and returns `files` in path order. Each entry has `path`, `status`
(`added`, `modified`, or `deleted`), and nullable `old_id`, `new_id`,
`old_mode`, and `new_mode`. Files, symbolic links, and gitlinks are reported;
tree container entries are omitted. A mode-only change is `modified`.

Owners and contributors append immutable pull request discussion with `POST
/repositories/{id}/pulls/{pull_id}/comments` and a non-empty `body`. `GET` on
the same collection returns attributable comments under `comments` with the
shared cursor pagination contract. Each comment records its stable `id`,
`pull_request_id`, `author_id`, body, and creation time. Reads inherit pull
request visibility, while participation requires current repository access
and `repositories:read`; making a repository public does not grant comment
permission. Comment publication uses the shared uncertain-durability response.

Current repository participants record an explicit review with `POST
/repositories/{id}/pulls/{pull_id}/reviews` and a `decision` of `approved` or
`changes_requested`. Each reviewer has one current review: posting again keeps
its stable review ID while replacing the decision and the evaluated commit.
The live source branch must match the pull request's recorded source revision;
otherwise review submission returns `409 source_branch_changed` and the pull
request author must synchronize the revision first. This prevents a decision
on an unadopted branch tip from becoming valid through later synchronization.
The resource records stable `reviewer_id`, `reviewed_commit_id`, creation and
update timestamps, and a derived `stale` flag. The evaluated commit is the
source branch tip when the decision is submitted, not merely the pull
request's recorded revision. Consequently, `GET` on the same paginated
collection reports an earlier review as stale after the source branch moves.
Synchronizing that tip makes it the pull request's reviewable revision but
does not revive the earlier decision; a reviewer must approve it explicitly.
If the source branch is deleted or no longer identifies a commit, its durable
reviews remain readable and are all reported stale.

`DELETE /repositories/{id}/pulls/{pull_id}/reviews/{review_id}` is available
only to that review's author and replaces its decision with `withdrawn`. The
review remains visible with its prior `reviewed_commit_id`, preserving who
evaluated which version without treating the old decision as active. Owners
and contributors may review with `repositories:read`; public readability does
not grant review participation. Review mutations use the shared uncertain-
durability response when publication is visible but its directory sync fails.

`GET /repositories/{id}/pulls/{pull_id}/merge-readiness` gives a current owner
or contributor a read-only answer about what remains before merge. The report
contains `mergeable`, caller-specific `can_merge`, one required current
approval and the current approval count, live `source` and `target` branch
state, `has_conflicts`, and an ordered `blockers` array whose entries have a
stable `code` and explanatory `message`. Branch state includes the branch
name, immutable pull-request `snapshot_commit_id`, nullable
`current_commit_id`, and a state of `current`, `advanced`, `rewritten`, or
`missing`.

An open request is mergeable only while its source branch still identifies the
snapshotted commit, its target exists, the source is not already reachable
from the target, at least one non-stale approval exists, no non-stale review
requests changes, and Git's three-way merge against the live target is
conflict-free. A target advance is reported but is not itself a blocker.
`can_merge` additionally requires that the inspecting participant is the
repository owner; contributors can therefore see that a request is otherwise
ready without being told they may update the maintained branch. Conflict
detection redirects Git's calculated merge objects into temporary storage, so
the endpoint never changes repository objects or references.

`POST /repositories/{id}/pulls/{pull_id}/merge` applies a ready request and is
restricted to the repository owner with `repositories:write`. A request that
is no longer ready returns `409 pull_request_not_ready`; merge revalidates live
branches and reviews and advances the target only if its tip still matches the
calculation. Success creates a two-parent merge commit, changes the request to
`status: "merged"`, and adds `merged_at`, `merged_by`, and `merge_commit_id`.
The commit retains the request title and body plus stable `Pull-Request`,
optional `Proposal`, `Authored-by`, and `Merged-by` trailers. A linked open
proposal is closed while its existing discussion remains readable. Target
advancement uses compare-and-swap protection so a concurrent push is never
overwritten. Merge retries are idempotent; if target publication became
visible before pull-request metadata could be published, a retry recognizes
the exact commit recorded in a private, durable server merge intent—even
beneath later target history—and repairs the durable request outcome. Git
trailers remain collaboration context and are never trusted as authorization
provenance.

## Change sessions

Current repository participants can turn an open pull request into a durable
agent collaboration workspace with `POST
/repositories/{id}/pulls/{pull_id}/sessions`. Creation requires
`repositories:write`, accepts no worker configuration, and snapshots the pull
request's current `source_commit_id`. The returned session contains stable
repository, pull request, initiator, and revision identities; `state: "open"`;
and creation and update timestamps. Merged pull requests retain their existing
sessions but reject new ones with `409 pull_request_closed`.

`GET /repositories/{id}/pulls/{pull_id}/sessions` discovers every session on
the request using shared cursor pagination. `GET .../sessions/{session_id}`
reconnects to one, and `GET .../sessions/{session_id}/events` returns its
oldest-first, cursor-paginated timeline. Creation atomically publishes the
session with a `session.opened` event attributed to its initiating user, so an
inspectable session never depends on process memory or private worker logs.
Session creation follows the shared uncertain-durability response contract.
Inspecting a session retries synchronization of its containing directory. If
that reconciliation succeeds, inspection returns `200` and confirms durable
reconnection; if it still fails, inspection repeats the `202` uncertainty
response with the stable session body so clients continue warning users and
withhold reliance on its timeline. Direct event collection requests enforce
the same reconciliation and return that stable uncertain session response
instead of an ordinary event page until persistence is confirmed.

Session discovery and inspection require current owner or contributor access
with `repositories:read`, including for public repositories. Durable records
live beneath `CHANGE_SESSION_STORAGE_ROOT` (default `change-sessions`) and are
partitioned by repository and pull request. Run, guidance, intervention, and
artifact events extend this public session and timeline boundary rather than
exposing execution internals.

Confirmed sessions accept bounded delegation at `POST
.../sessions/{session_id}/runs`. A current participant with
`repositories:write` supplies `instructions`, the session's exact
`source_commit_id`, one to fifty unique `context_paths` that exist in that
revision, the explicit pull-request source `working_branch`, and an optional
`expires_in` between five minutes and 24 hours (one hour by default). The pull
request must remain open. Success returns an attributable durable `run` plus a
Git `credential`; its opaque `token` is shown only in this response. The
credential has only `git:read` and `git:write`, is bound to this repository,
may update only that source branch, and expires at the recorded
`credential_expires_at`. It cannot update another branch even when the
initiator owns the repository.

`GET .../sessions/{session_id}/runs` is participant-only and cursor-paginated.
Run records retain the mandate, selected revision and paths, branch,
initiator, a stable generated `agent_id`, credential ID and expiry, but never the token. Launch atomically
adds an oldest-first `run.launched` session event. A launch whose session
publication is visible but not confirmed uses the shared `202` durability
contract and still returns its stable run and one-time credential; clients
must inspect rather than duplicate the launch. Any current participant with
`repositories:write` can revoke a run credential through `DELETE
.../runs/{run_id}/credential`; the run then records `access_revoked_at` and
the token fails on its next request. The run credential publishes execution
progress at `POST
.../runs/{run_id}/events`. The body has a `kind` of `run.status`,
`agent.message`, `tool.action`, `artifact.produced`, `run.failed`, or
`branch.updated`, plus a required human-readable `message` and `state`.
Tool actions require `tool`, artifacts require `artifact`, and branch updates
require the run's exact `branch` plus a full `commit_id`. A branch update holds
Git's standard per-reference lock while verifying that commit is the published
tip and durably appending the event, so a concurrent push cannot make the
observation stale. Each append verifies
the credential belongs to that run and is still active, then durably snapshots
the `run_id`, generated `agent_id`, initiating user, and session revision onto
the event. The event is immediately available through the ordinary
participant-only timeline; its atomic publication uses the shared uncertain-
durability response. This provides one ordered public record for status,
messages, actions, outputs, failures, and published revisions without exposing
worker logs or accepting caller-supplied attribution.

An agent can also publish `agent.question` when it needs a decision. Current
participants guide active work through `POST
.../runs/{run_id}/interventions` with a `kind` of `run.guidance`,
`question.answered`, `run.paused`, `run.resumed`, or `run.canceled` and a
`message`. Guidance and answers require non-empty messages and are accepted
while a run is active or paused. Pause and resume are strict transitions;
invalid or repeated transitions return `409 invalid_run_transition`. Every
accepted intervention atomically updates the run and appends an attributed
event to the shared timeline.

The run credential reads its authoritative control state at `GET
.../runs/{run_id}/control`. The response contains the `run` and its ordered
collaborator `interventions`, without exposing the participant-only session
timeline. A paused run receives `409 agent_run_paused` if it attempts to
publish more progress and must poll control state until resumed. Cancellation
is terminal: it records `state: "canceled"`, appends `run.canceled`, and revokes
the bounded Git credential so later progress, control reads, fetches, and
pushes fail authentication. Cancel retries tolerate an already-revoked
credential. Intervention publication follows the shared uncertain-durability
response contract.

After committing and pushing new descendant history, the active run credential
publishes its review handoff at `POST .../runs/{run_id}/completion`. The body
contains a required `summary`, the exact live source-branch `commit_id`, zero
or more `checks` (`name`, `status` of `passed`, `failed`, or `skipped`, and
optional `details`), and `unresolved_concerns`. The server holds the branch's
standard reference lock, verifies the commit is its current tip and descends
from the run's pinned revision, and derives the exact intervening commit IDs
and path-ordered changed files from Git rather than trusting agent claims.

Successful publication records the structured `outcome` on the run, appends an
attributed `run.completed` event, marks the run terminal, revokes its Git
credential, and synchronizes the pull request to the same commit. Existing reviews consequently become stale
and merge readiness evaluates the new revision through the ordinary pull
request rules. The response contains the run, event, and updated pull request;
the shared `202` contract applies if either durable publication is visible but
not confirmed. A moved branch tip, unrelated history, paused/canceled run, or
pull request advanced by another workflow is rejected without presenting the
candidate as this run's completed work.
