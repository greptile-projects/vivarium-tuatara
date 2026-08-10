# Docs

Notes on how this project fits together. Mostly empty for now — things get
written down here as they're decided, not before.

## Entrypoints

- `apps/web` — Next.js frontend. Starts at `src/app/page.tsx`; routes are
  file-based under `src/app`. `bun dev` from the repo root.
- `apps/api` — Go HTTP API. Starts at `main.go`, where routes are registered on
  the mux. `bun run dev:api` from the repo root, serves on `:8080`.

The supported consumer contract, including authentication, stable error
shapes, validation, and collection pagination, is documented in [API.md](API.md).
Consumers should use that HTTP boundary rather than reading storage roots.

## Exact-revision code navigation

The repository code workspace searches one immutable commit for symbols and
text, then groups lexical definitions, references, call sites, and tests with
file/line links and last-change commit evidence. The API reports its file,
byte, and result bounds rather than presenting partial coverage as complete.
Ownership comes from the current repository catalog; declared interface
dependencies appear only when recorded for the selected commit and the
requester can currently read the provider.

## Reproducible development workspaces

Development environments are shared, revision-pinned collaboration records.
The `/workspaces` web surface and public API launch from a repository, proposal
task, pull request, or incident emergency-repair context only after current
participant authorization. Launch requires the exact commit to contain
`.vivarium/workspace.json` version 1. That file supplies the container image,
declared tools and dependencies, setup commands, and bounded CPU, memory,
storage, and setup duration.

Repository and organization owners layer versioned workspace governance over
that definition: strict organization limits constrain repository CPU, memory,
storage, network, idle, runtime, retention, sharing, and approved-agent
execution choices. Launch snapshots the effective policy; later policy changes
make active environments visibly rebuild-required instead of silently changing
their foundation. Owners can inspect creator-attributed CPU time and reserved
memory/storage, announce an expiry window for checkpoint export, or stop and
expire compute while retained checkpoints and published Git evidence remain.

The API snapshots the definition and its SHA-256, materializes only the named
Git commit into a size-limited tmpfs, reserves a bounded share of the same
storage budget for `/tmp`, and runs setup in that named, read-only-root,
network-disabled, capability-dropped container without platform credentials.
Images with declared Docker volumes are rejected before container creation so
image metadata cannot introduce writable storage outside that total budget.
Setup failure or timeout force-removes the workload instead of only terminating
its Docker client. The workspace record retains source context,
creator, launch-time effective access, commands, bounded output, exit status,
timestamps, and lifecycle history beneath `$WORKSPACE_STORAGE_ROOT` (default
`workspaces`). Current repository access is revalidated on reads and lifecycle
changes. Suspend and resume require the frozen definition hash and reuse the
same materialized commit without resolving a branch or silently rerunning setup;
a missing runtime or changed foundation fails closed.

The workspace detail surface and equivalent API expose a bounded file tree,
compare-and-swap text editing, literal search, attributed command/terminal
outcomes, loopback port discovery, and authenticated sandboxed application
previews against that same named container. Durable change evidence stores
paths, sizes, and hashes rather than file snapshots. Runtime commands receive
no platform-managed secrets, and preview ports are never published directly.

Workspace implementation is a live shared activity. Current repository
participants publish renewable presence with a workspace, file, terminal,
command, or preview focus, reconnect to the same discussion and activity, and
observe file, command, lifecycle, and control outcomes. One versioned, expiring
control lease names a current human or organization-approved agent, its mode,
and exact file, command, and lifecycle scopes. Any current participant may take
over with the last observed version; stale attempts conflict. The server
enforces the lease again at execution time. Durable activity distinguishes
observation, instruction, authorship, and execution, while terminal input is
represented only by SHA-256 and never enters the workspace record. File writes
still compare-and-swap the opened content digest across control transfers.

Unfinished repository work can be retained independently of the live runtime
as an attributed checkpoint. A checkpoint freezes the exact base commit,
workspace definition and digest, declared dependencies and reproduction notes,
plus an ordered manifest of added, modified, and deleted repository files.
Participant reads expose file operations, hashes, modes, sizes, and parent
lineage rather than stored bytes or textual patches. Credential-like paths or content,
including package-manager authentication files and directives,
non-regular filesystem state, and snapshots over 32 MiB fail closed; terminal
input, command output, presence, previews, processes, and container state are
outside the checkpoint boundary.

Restore requires `GET /workspaces/{id}/checkpoints/{checkpoint-id}/restore`
preflight. It reports live-path conflicts, a moved repository base, missing
declared dependencies, and definition drift, and returns a token bound to the
current snapshot and lineage. Applying requires the current human file-control
lease and rechecks that token inside control admission. Restore moves the
workspace checkpoint head to the selected record, so the next checkpoint forms
an explicit branch while prior descendants remain retained. Checkpoint capture
and publication share that admission boundary, so an admitted file write cannot
finish between the snapshot and lineage update. Restore stages and backs up all
targets before mutation; failure restores them and removes parent directories
introduced by the transaction.

When live paths conflict, the browser requires an explicit replacement
confirmation before sending the API's conflict override. Runtime capture
streams from inside the bounded tmpfs rather than reading the container image
layer, and image-side automation uses portable shell operations so minimal
foundations such as Alpine remain usable.

An authorized participant can publish a retained checkpoint into normal Git
governance. Publication either compare-and-swap advances an existing working
branch from the checkpoint base or creates a branch there, then creates one
commit from only the inspected checkpoint file manifest. An optional ordinary
pull retains links in both directions to the workspace and checkpoint, carries
proposal task/session context when supplied, names exact contributors, and
summarizes file hashes plus command digests frozen at checkpoint capture. A
cross-process checkpoint publication claim excludes duplicate branch/pull
effects, branch compensation is compare-and-swap safe, and post-pull linkage
failure becomes durable retryable intent rather than an orphan. An outbox
created before Git mutation also recovers a linked pull when the final
checkpoint-record write itself fails. Supplied task
sessions must belong to the same repository/proposal/task. Checkpoint capture
uses a private append-only evidence ledger independently of bounded workspace
display histories; legacy retained histories seed the ledger before the first
new event. Repository checks, stale-revision
reviews, branch protection, and the integration queue apply without a
workspace-specific bypass. Terminal input, outputs, discussion, credentials,
and unpublished runtime files remain outside the commit and generated review.

The connected workspace browser journey carries an organization-approved
agent, two human participants, and a proposal task through shared editing,
commands, intervention, suspension, reconnect, conflicting checkpoint restore,
publication, checks, review, merge, and compute expiry while retaining the full
permission-aware workspace and pull provenance.

## Unsafe package recovery

Package lifecycle decisions are attributable append-only coordination records,
not edits to published bytes. A publisher owner may deprecate, quarantine, or
yank an exact version with a reason, warning, and active safe replacement. The
catalog keeps checksum, source, build, consumer inventories, and historical
deployments visible while notifying every currently exposed repository owner.
Fresh package resolution, repository-bound downloads, and new promotions reject
non-active versions; promotion also fails closed when the exact inventory or
package evidence store is unavailable. Lifecycle changes return `202` plus a
pending-notifications header if the durable decision committed but exposed-owner
activity did not, allowing an exact idempotent retry to finish delivery. A
consumer owner can open an urgent ordinary proposal and
task for the replacement, assigning it to a current human participant or a
task-scoped agent at the consumer's exact default-branch revision; checks,
review, integration, release, and deployment authority remain consumer-owned.
The connected package browser journey exercises the whole boundary: a publisher
creates checksum-attested releases, an independent consumer installs through a
repository-scoped bearer token using `curl`, and branch-bound agent updates pass
independent review before exact-inventory releases reach production. When the
adopted version is quarantined, new scoped downloads fail while the retained
deployment remains visible; urgent agent repair then returns through ordinary
review, release, and deployment with remediation projected on the unsafe
version's consumer evidence.

## Incident response

Incidents are first-class cross-repository collaboration records rather than
deployment annotations. A current collaborator can declare service risk
manually or from retained deployment health evidence, identify all affected
repositories and environments, assign response roles, and maintain one
attributable severity, status, and timeline. Updates explicitly retain either
participant or public audience intent, and acknowledgements show which
responders have consumed each decision. The `/incidents` workspace aggregates
only incidents the signed-in actor can still reach through current repository
membership; detail routes provide the live operating picture and response
controls.
The browser writes a pending incident update's operation identity and exact
draft fingerprint to incident-scoped local storage before publication. A lost
response can therefore be retried after reload with the same durable identity;
confirmation clears it, while changed content intentionally replaces it.
Diagnosis is recorded in the same operating picture as typed observations,
hypotheses, queries, and conclusions. Each diagnostic entry attaches verified
affected-repository sources for deployment logs, health signals, deployments,
releases, commits, pull requests, or earlier incidents. Operational selections
retain an exact query and time window; every attachment snapshots a label and
capture time while keeping a path to the authoritative retained source. This
lets responders compare the historical basis for a claim with live source
state without weakening the incident's current-participant access boundary.
Authorized responders can delegate a bounded investigation by freezing a
mandate, selected diagnostic evidence, and exact verified repository commits.
The one-time agent credential has only `incidents:investigate`: it can inspect
that packet and delegation-time snapshots of selected operational resources,
then append findings, tool
actions, questions, and uncertainty to the participant timeline. Responders
retain guide, pause, resume, and cancel authority; the agent receives no Git,
deployment, environment, credential, secret, or repository mutation scope.
Responders turn that diagnosis into explicit pause, attested-restore, or
emergency-repair proposals linked to exact evidence and declared rollout health
signals. Independent approvals (or visible self-approval overrides), governed
deployment/recovery execution, failed attempts, and resulting resource IDs stay
on the incident. Recovery is recorded only after the named signals pass on a
retained recovery deployment.
The connected two-user browser journey exercises this as one continuous
signal-to-learning loop: a retained failed production signal becomes a browser-
declared incident, selected evidence and an exact revision are frozen for a
read-only agent, an independent responder approves an attested rollback, and a
public recovery update is acknowledged. The published review then creates an
accountable corrective proposal task that returns through ordinary checks,
review, release, and governed production deployment; incident reads derive all
follow-up progress from the authoritative workflow records.

## Governed interface evolution

An evolution rollout coordinates, but never replaces, repository and
environment authority. Provider maintainers select passing retained exact
contract evidence, choose the relevant migration task for each repository, and
order participating repositories into phases; each repository owner approves
only their own participation. The relationship workspace then
derives compatibility and progress from ordinary migration pulls, integration
queues, ancestry-containing releases, and governed environment promotions.
Failures pause the affected phase with links back to established rollback or
agent-repair workflows, leaving already-safe repositories and evidence intact.

The connected evolution browser journey treats this as one collaboration
capability rather than isolated endpoints. An independently owned consumer is
discovered from its exact declaration, a read-only agent records scoped impact
evidence, human migration work arrives from an owned fork, and provider work
arrives from a task-bound agent branch. The exact open pull combination must
pass the provider contract check before each owner approves their own rollout
participation; ordinary merges, releases, checksummed builds, and production
deployments then advance consumer-first and leave the completed evidence and
attribution visible in the relationship workspace.

## Collaborative code investigations

Repository participants can open revision-aware investigations from repository,
file, proposal, task, pull, incident, or workspace context in the repository's
“Ask the codebase” surface. The server resolves that context to one immutable
commit, collects only currently permitted bounded source, documentation,
history, check, and declared-dependency evidence, and persists the complete
attributed result before streaming its structured claims. Citations link to
exact paths and lines, evidence is distinct from inference and uncertainty, and
incomplete coverage stays visible. Durable conversation reads revalidate live
repository access; private workspace context also retains its narrower sharing
boundary. Explicitly invited current participants share an ordered canvas of
code references, queries, bounded workspace observations, hypotheses, agent
findings, challenges, supersessions, and conclusions. Rerunning at a new commit
preserves the earlier reasoning and marks older citations stale, so a peer can
join midway and verify what changed without trusting private agent context.
Workspace attachments retain only an authorized identity and frozen revision,
never credentials, hidden files, or copied runtime output.
`$EXPLANATION_STORAGE_ROOT` defaults to `explanations`.

## Prospective change impact

Impact assessment turns revision-pinned understanding into a reviewable
pre-implementation decision. A repository participant starts from selected
lines, a retained investigation conclusion, or a proposed diff. The bounded
analyzer records exact reference and test locations, verification guidance,
and joins them to currently readable owners, interface consumers, exact
releases, published packages, and promoted environments. Incomplete coverage
is explicit rather than silently presented as exhaustive.

Assessment participants use compare-and-swap versions to invite collaborators,
add verification needs, accepted risks, and unknowns, and request an affected
repository owner's acknowledgement. Every read revalidates visibility for each
cross-repository evidence item. A request is limited to repositories named by
retained consumer evidence, and its current owner must also retain source
repository access to read or acknowledge it. An uncommitted proposed diff
remains visible only to assessment participants. Durable records live beneath
`$IMPACT_STORAGE_ROOT` (default `impact-assessments`).

Assessment participants can carry that decision directly into implementation
by selecting retained impact items and creating one proposal with an ordered
human- and generated-agent-owned task plan. The plan freezes the assessment
version, exact commit, investigation conclusion, selected claims, risks,
verification needs, and owner acknowledgements in every task. Task work then
uses the existing scoped workspace or agent-session launch and ordinary pull
publication paths; those launches preload the frozen reasoning, while proposal
and pull review surfaces link back to it. If the selected branch moves, the
assessment is reported as changed and cannot start new work, preserving the old
trail instead of silently refreshing its evidence.

The connected code-intelligence browser journey proves this is one workflow:
an unfamiliar developer navigates exact symbol evidence, asks a grounded
question, invites an affected consumer owner to refine the conclusion, records
and acknowledges cross-repository impact, launches agent-owned implementation,
and merges only after the identified repository check and independent review.
The final pull, task session, proposal reasoning, investigation, and assessment
retain the exact revision and attributable decisions. Playwright isolates both
`$EXPLANATION_STORAGE_ROOT` and `$IMPACT_STORAGE_ROOT` with its other temporary
API stores.

## Web interface foundation

The web application uses a persistent workbench shell so account, repository,
proposal, pull-request, and review routes can grow inside one navigation and
content model. `apps/web/src/components/app-shell.tsx` owns the responsive
sidebar, mobile navigation, global search affordance, creation entry point,
notifications, account access, skip link target, and centered page boundary.
Route pages provide only their content beneath that shared shell.

The visual language is defined by semantic CSS variables in `globals.css`: an
off-white canvas, white raised surfaces, ink and muted copy, moss as the
primary action color, status-specific soft colors, consistent borders,
radii, shadows, keyboard focus rings, and reduced-motion behavior. Reusable
buttons, badges, cards, and avatars live in `src/components/ui.tsx`; common
stroke icons live in `src/components/icons.tsx`. Later workflows should extend
those modules when a pattern is genuinely shared instead of styling a second
version in a route.

Interaction states are part of the component contract: controls have visible
hover and keyboard focus treatment, buttons expose a disabled state, selected
navigation uses `aria-current`, status is communicated with text rather than
color alone, icon-only controls have accessible names, and the document offers
a skip link and semantic landmarks. Pages should supply useful empty, loading,
error, and permission-denied states using the same card, type, action, and
status vocabulary as their populated views.

Browser API calls use same-origin `/api/*` requests and clone URLs use
same-origin `/git/*`; Next.js rewrites both to `API_ORIGIN` (the local API on
port 8080 by default). Account creation returns
its one-time session token to the web client; the client retains that bearer
token in browser local storage, validates it with `GET /user` at startup, and
clears invalid or explicitly logged-out sessions. Returning users may present
an existing session or API token. Onboarding continues into owned and
collaborator repository creation and cursor-complete discovery, while settings
expose profile editing, API and Git
token issuance, one-time secret revelation, revocation, and session logout.
Settings also expose the account's immutable collaboration ID. A newcomer can
share that ID with a repository owner, who can grant or revoke contributor
access directly from the repository detail page; resolved handles make the
durable access list attributable rather than presenting opaque IDs.
Repository catalog entries link to `/repositories/[id]`, where public visitors
and authorized collaborators can select branches, follow commit-pinned history,
navigate snapshot directories, preview text files, identify binary files, and
copy clone information. Owners can change visibility on that same surface,
making a repository publicly readable and forkable without first granting the
newcomer membership. The selected `ref` and repository-relative `path` live
in the URL, while every content response exposes its resolved commit, keeping
navigation explicit when a branch moves. Branch selection is resolved once per
load and subsequent content links use that immutable commit; history is
paginated and text previews are capped at 512 KiB.
Superseded client loads cannot publish state after a newer URL selection.
Preview verification streams oversized blobs without retaining their full
contents, and later history cursors have a fixed per-request scan ceiling.

## User identity

Human collaborators have durable platform identities backed by the API's
`users` package. `POST /users` creates an account from a unique `handle` and a
`display_name`; `GET /users/{id}` inspects it; and `PATCH /users/{id}` changes
either profile field. Handles are normalized to lowercase and contain 1–39
letters, numbers, or hyphens. Display names contain 1–100 characters on one
line. Requests and responses use JSON, and create responses include a
`Location` header for the new identity.

Each account receives a random 128-bit lowercase hexadecimal ID. That ID and
`created_at` never change, so future repositories, commits, reviews, and other
meaningful actions can refer to the actor independently of profile changes.
Handles are globally unique but editable; they are collaboration labels, not
attribution keys. `updated_at` records the most recent profile write.

Records are atomically published as private JSON files beneath
`USER_STORAGE_ROOT`, which defaults to `users` relative to the API process.
The API syncs both record contents and the containing directory before
acknowledging a write, and reopening the same root restores the same identity.
All mutations hold an advisory lock in that root, making handle claims and
sparse profile merges atomic even when multiple API processes share storage.
Account creation is the credential bootstrap: its response contains the new
`user` and a short-lived `credential`, including an opaque secret shown only in
that response. It also sets the secret as a `Secure`, `HttpOnly`, `SameSite=Lax`
`vivarium_session` cookie for browser use. `GET /users/{id}` remains public so
collaborators can resolve attribution, while `PATCH /users/{id}` requires that
user's authenticated `profile:write` scope.

User creation and its initial session are one ordered application bootstrap.
The user store reserves the validated handle while the session is prepared and
publishes the identity only after credential publication succeeds. A credential
failure therefore leaves no user record or handle reservation, so registration
can be retried even when cleanup storage is unavailable. Likewise, logout
returns success only after the session revocation has been durably published.
If either store reports an error after its atomic rename, bootstrap rereads the
exact record and treats a matching publication as committed rather than
returning a misleading retry-blocking failure. If session issuance succeeded
but user publication definitively did not, the API revokes that session before
returning the registration error.

## Authentication

Credentials are typed capabilities with an owner, human-readable name, exact
scopes, expiration, last-use timestamp, and optional revocation timestamp.
Session credentials last at most 24 hours and carry `profile:write` plus
`credentials:write`. An authenticated session can call
`POST /auth/credentials` to create narrower API credentials (at most 90 days)
or Git credentials (at most 30 days). API credentials may carry
`profile:write`; Git credentials independently carry `git:read`, `git:write`,
or both. `expires_in` is expressed in seconds. A creation response is the only
place the opaque `token` is returned.

`GET /auth/credentials` inspects all of the actor's credentials as safe
metadata, including expired and revoked entries. `DELETE
/auth/credentials/{id}` revokes any owned credential immediately and is
idempotent for an already-revoked record. `DELETE /auth/session` revokes the
calling session and clears its browser cookie. Credential administration
requires `credentials:write`, which cannot be granted to long-lived API or Git
tokens.

API consumers send `Authorization: Bearer <token>`. Stock Git clients use the
Git token as an HTTP Basic password (the username is ignored), so standard Git
credential helpers can store it without custom tooling. Upload-pack discovery
and RPC require `git:read`; receive-pack discovery and RPC require
`git:write`. Public upload-pack requests may omit a credential; private
upload-pack and all receive-pack requests additionally require that the
credential actor own the repository.

Credential records are private atomic JSON files beneath `AUTH_STORAGE_ROOT`,
which defaults to `credentials`. Only SHA-256 token hashes are durable; raw
secrets cannot be recovered from inspection or storage. Authentication and
revocation updates share a root-wide advisory lock, preventing concurrent API
processes from resurrecting a revoked credential. Expiration is enforced on
every request.

## Owned repositories

Repository lifecycle is exposed through an authenticated application catalog.
`POST /repositories` accepts a `name`, creates an empty bare repository, and
returns its immutable owner and opaque repository ID together with
`default_branch: "main"` and `git_remote: "/git/<id>.git"`. The same opaque ID
is used by `GET /repositories/{id}`, durable Git storage, and the smart HTTP
remote, so profile edits and future name changes cannot change repository
identity. Names are case-insensitively unique within one owner's repositories
but may be reused by other owners.

A readable repository can become the immutable upstream of a private fork.
Fork creation transfers only published reachable content-addressed objects and
publishes a distinct bare repository with independent references,
ownership, visibility, collaborators, policy, and remote. Catalog metadata
retains `upstream_repository_id`; creating the fork grants no authority over
the source. Private-source authorization, clone transfer, and fork publication
share the catalog mutation lock, so collaborator revocation commits wholly
before or after creation. The web repository surface exposes both lineage and
fork creation.

Fork owners can synchronize a selected named branch against the same-named
upstream branch. The server snapshots the upstream tip, imports its missing
reachable content-addressed objects, proves the fork branch is an ancestor,
and compare-and-swap fast-forwards that one reference. Missing branches,
divergent work, and concurrent pushes remain explicit failures, so independent
branches are not overwritten. The web lineage panel applies this operation to
the currently selected named branch. For private upstreams, authorization,
object import, and reference publication share the catalog mutation lock;
collaborator revocation therefore commits wholly before or after a sync.
The connected browser regression proves this boundary from the newcomer’s
first public inspection through web fork creation and synchronization, stock
Git publication to the fork, cross-repository review and agent repair, required
checks, and an attributed upstream merge.

`GET /repositories` lists only the authenticated actor's repositories in
creation order using the shared cursor pagination contract. Repositories are
private by default, and their owner can use
`PATCH /repositories/{id}` to change `visibility` between `private` and
`public`. Public repositories can be inspected without authentication, while
private inspection, visibility changes, and `DELETE /repositories/{id}`
resolve ownership from the credential rather than accepting an owner from the
client; another actor receives the same not-found response as an unknown ID.
Deletion
atomically detaches the Git storage ID before removing the ownership record,
so a successfully removed API resource is no longer a usable remote. Active
catalog reads also verify that Git backing still exists. If metadata cleanup
fails after that atomic detach, list and inspection treat the repository as
deleted and its name can be reused, but the private ownership record remains
as authorization for a later delete retry. Metadata is removed only after Git
cleanup reports success; completed Git deletion is idempotent so a preceding
metadata-removal failure can also be retried safely.

Session credentials include `repositories:read` and `repositories:write`.
Long-lived API credentials can be issued with either capability for narrower
automation. Catalog metadata is stored as private atomic JSON records beneath
`REPOSITORY_STORAGE_ROOT`, defaulting to `repository-records`, while Git bytes
remain beneath `GIT_STORAGE_ROOT`. Git credentials authorize smart HTTP with
`git:read` and `git:write`, and transport applies the same visibility and owner
policy as the repository API. Anonymous and authenticated actors may fetch a
public repository. Owners can grant an existing user the `contributor` role
through the repository's collaborator collection. Contributors may inspect
and fetch private repositories and publish non-`main` candidate branches with
stock Git, while visibility, collaborator management, deletion, and
default-branch writes remain owner-only. Removing the grant immediately
removes that additional API and Git access.

Repository proposals provide durable pre-code collaboration. Owners and
contributors can create proposals and append immutable comments; proposal and
comment authorship uses stable user IDs. Proposal authors may refine and close
their proposal, and owners may close any repository proposal. Reads inherit
repository visibility and collaborator access. Records are stored atomically
beneath `PROPOSAL_STORAGE_ROOT` (default `proposals`) independently of Git
objects, so conversation remains available without a candidate branch.
The web proposal workspace aggregates proposals across every repository in the
signed-in actor's catalog and filters them by text, repository, and lifecycle
status, making existing context visible before new work is proposed. Proposal
detail routes remain directly addressable and render public conversations
without authentication when repository visibility permits, while creation,
editing, commenting, and closure controls follow the API's participant and
ownership rules.

Each proposal also owns an ordered executable plan. Current repository
participants define tasks with explicit expected outcomes, links to motivating
comments, and an acyclic dependency graph; the platform derives which pending
tasks are ready and which dependencies still block them. Collaborators can
change task status and order from the proposal detail page, while immutable
actor-stamped snapshots retain creation, edit, decision, and reorder history.
Plan storage shares the proposal's atomic record and visibility boundary, so a
public idea remains understandable without granting public mutation authority.
Ready tasks can be claimed or assigned to exactly one current human participant
or a generated available-agent identity. Each assignment freezes a mandate,
repository, and exact base commit and shows the authority boundary before work
starts: humans retain only their existing collaboration rights, while agents
are previewed with repository- and future-task-branch-scoped Git access.
Assignment IDs act as compare-and-swap versions for reassignment and revocation,
making concurrent outcomes explicit and retaining every decision in task
history.
Task definitions carry durable context revisions. Readiness requires dependency
results that are both completed and current; revising a plan or replacing a
linked contribution marks earlier assignment, session, and pull evidence as
changed or obsolete without deleting it. Human assignees receive targeted
ready, blocked, changed, and obsolete inbox events. An explicit compare-and-swap
rebase moves the existing owner and mandate to a verified new base and current
plan revision, creating a fresh assignment boundary for later work.
An assigned agent task can now start directly from that boundary without a
placeholder pull request. Start atomically names an isolated task branch at the
frozen base, opens a durable task-scoped change session, launches the mandate
with a one-time branch-bound Git credential, and snapshots proposal, dependency,
discussion, and repository context. Task sessions reuse the public timeline,
control, guidance, pause, resume, cancel, and reconnection protocol used by
pull-request sessions; publishing their candidate into review remains a later
explicit handoff.

Pull requests connect that context to exact repository state. An owner or
contributor opens one from an existing source branch, while a fork owner can
submit a branch from their independently owned direct fork to its readable
upstream. The durable resource records its author, purpose, both repository
identities, branch names, and both commit IDs at creation, so
later branch movement does not silently change the review. After responding
to feedback with another source-branch push, the author explicitly
synchronizes the request to adopt that tip as its next reviewable revision;
the target snapshot remains fixed. Adopted fork objects are imported without a
target reference, letting the existing commit, file, check, review, and merge
surfaces operate on the same exact evidence as an in-repository branch. A
deleted fork therefore ends future synchronization but does not destroy the
adopted review boundary, while revoked private-upstream access prevents any
later fork commit from being adopted or imported. The catalog's cross-process
lock spans that authorization decision, object import, and pull revision
publication, preventing revocation from committing mid-adoption.

Candidate revisions can carry their reproducible verification contract in
`.vivarium/checks.json`. Opening a pull request or explicitly adopting a new
source revision snapshots those commands into durable check runs for that exact
commit. Each command executes in a bounded, network-disabled OCI container
from a disposable exported snapshot, using a preinstalled image and no
repository credential. The snapshot is read-only; only bounded temporary and
output filesystems are writable. Each execution publishes an immutable
sequence of status changes, stdout/stderr chunks, command outcome, and artifact
metadata while it runs. Repository owners select required check names per
target branch. Readiness joins that durable policy to evidence for the pull
request's exact adopted source commit; missing, active, failed, canceled, and
other-revision results block merging and remain explicitly reported alongside
the evaluated revision. The merge transaction revalidates the same policy and
evidence.

Clients reconnect from the last sequence they
observed. Numbered attempt history retains interrupted and failed executions
against the exact commit, and artifact bytes remain downloadable by stable ID
with size and SHA-256 evidence. Interrupted execution and cleanup work is
retried at startup, periodically, and on a later same-commit trigger.
The pull-request review surface follows live state, expands every historical
attempt into its logs and artifacts, and lets current collaborators cancel or
rerun verification. Those controls append stable actor attribution while
preserving all earlier evidence, so investigation and intervention stay in the
same conversation as the change.
From a failed check on the currently adopted revision, a participant can open
a repair change session directly. That session snapshots the failing revision,
versioned definition, logs, command outcomes, and artifact identities instead
of asking the collaborator or agent to reconstruct them. Bounded agents receive
the same evidence through their control boundary and may retrieve only the
snapshotted artifacts. Their completed descendant commit follows the existing
pull synchronization path, which automatically starts the new revision's
versioned checks.
Lifecycle metadata, event streams, and artifacts live beneath
`CHECK_RUN_STORAGE_ROOT` (default `check-runs`) independently of Git and
pull-request records.

The activity layer records meaningful collaboration changes as immutable,
attributable events associated with their repository and proposal, pull
request, or access resource. The authenticated web feed combines current
repository activity including direct mention and access events, retaining stable
identity and resource keys plus snapshot labels so collaborators can
understand what changed while they were away. Records live beneath
`ACTIVITY_STORAGE_ROOT` (default `activity-records`) independently of conversation
and Git storage.
The authenticated inbox turns the subset with a recipient-specific next step
into review, response, and awareness queues. Each item links to its underlying
collaboration and can be cleared per user without deleting shared history;
current authorization and resource state prevent inaccessible or obsolete work
from remaining actionable.
Pull request metadata lives beneath `PULL_REQUEST_STORAGE_ROOT` (default
`pull-requests`), partitioned by repository ID to isolate collection reads,
and follows repository visibility and participant access.
The API derives the source-only commit set and recursive changed-file summary
from the fixed target snapshot and explicitly synchronized source revision.
Immutable pull request comments retain stable author IDs; owners, contributors,
and an outside author while the upstream remains public may participate, while
reads continue to follow repository visibility.
Fork pull requests keep two explicit authority domains. Their author can join
the attributable discussion while the upstream remains public without gaining
upstream repository membership; review, check control, and merge authority
remain governed by current upstream participation. The fork owner may
optionally allow current upstream participants to mint a short-lived Git
credential restricted to the contribution branch; the source repository's
other refs remain hidden, and opt-out, closure, or upstream access revocation
takes effect on the next Git request. Authors and upstream owners can close an
open request without deleting its durable review evidence, while only the
upstream owner can merge into the maintained branch. The resulting merge
commit records the immutable source repository, branch, and adopted commit in
addition to pull-request, author, and merger attribution, so provenance remains
inspectable after source deletion.
The web pull request workspace aggregates candidate work across the signed-in
actor's repository catalog and opens requests from existing branch pairs or
from an owned fork to its upstream, with optional proposal context. Its directly addressable detail pages use the
visibility-aware read APIs to present purpose, immutable review and target
revisions, source-only commits, path-ordered file changes, linked proposal
context, and attributable discussion. Public repositories remain inspectable
without an account, while commenting and creation require current repository
participation.
Those detail pages also complete the maintainer workflow without duplicating
server policy in the browser. Participants can approve, request changes,
replace, or withdraw their attributable decision; stale decisions remain
visible beside the exact revision they evaluated. When the candidate branch
moves, only the pull-request author is offered synchronization, with explicit
notice that prior decisions stay stale. The merge panel renders the API's
ordered readiness blockers, live branch states, approval count, conflict
result, and caller-specific permission. Only an owner with `can_merge` can
apply the change, after which the page shows the durable merge commit and
maintainer attribution.
Each owner or contributor also has one attributable current review decision.
Approvals and change requests capture the live source-branch commit being
evaluated, replacements preserve the review identity, and withdrawals remain
visible without acting as a decision. Review reads derive whether that commit
has become stale relative to the current source branch tip; deleting the
source branch leaves the durable reviews readable and marks them stale.
Synchronizing a revised source does not make an earlier decision fresh, so the
new revision must receive an explicit replacement approval before merge.

The connected web workflow is protected by a Playwright regression in
`apps/web/tests/collaboration-journey.spec.ts`. It starts isolated API and web
servers, drives two independent browser contexts through onboarding, access,
proposal, pull-request, review, and merge actions, and uses stock Git for both
candidate publication and the maintainer's final pull. Run it with
`bun run --cwd apps/web test:e2e`; it discovers Chromium from `PATH`, falls
back to Playwright's managed browser, and honors an explicit `CHROMIUM_PATH`
override. When neither an override nor system Chromium is available, the
command provisions the version-pinned Playwright Chromium in its standard
cache, making a clean checkout self-contained while keeping repeat runs
incremental.

## Git repository storage

The API's `storage` package is the boundary for durable Git repositories. A
filesystem-backed store creates repositories atomically beneath its root as
`<id>.git`, reopens them by the same stable ID, and validates their bare Git
metadata through `Repository.Inspect`. New repositories use `main` as their
unborn default branch and are compatible with stock Git commands through the
absolute path exposed by `Repository.Path`.

Repository IDs are single URL-safe components containing letters, numbers,
dots, underscores, or hyphens. Ownership and user-facing names remain an
application-layer concern; storage IDs only identify a repository boundary.
Deletion first renames `<id>.git` to the stable internal tombstone
`.deleting-<id>`, making the remote immediately unavailable. The ID cannot be
recreated while cleanup is pending, and retrying `Store.Delete` discovers and
removes that tombstone before reporting completion.

Repository content is written through `Repository.WriteObject`, which accepts
one of Git's blob, tree, commit, or tag types and canonical uncompressed
content. It computes the SHA-1 object ID over Git's `<type> <size>\0` header and
content, then atomically publishes the zlib-compressed loose object. Rewriting
the same object is idempotent and never replaces existing storage.
`Repository.ReadObject` accepts a full lowercase object ID and returns its
verified ID, type, size, and exact content. Reads reject malformed IDs, missing
objects, invalid headers, size mismatches, and content that does not hash to
the requested ID. Reads and writes are limited to 100 MiB per object so corrupt
compressed input cannot cause unbounded allocation. These files are directly
readable by stock `git cat-file`.

`Repository.ListObjects` discovers every canonical loose-object path and
returns the verified identity, type, size, and exact content of each object in
object-ID order. Enumeration fails on a corrupt object rather than silently
hiding it. For repositories populated through this storage boundary, its
result is the same complete object set reported by stock Git's
`git cat-file --batch-all-objects`.

Snapshots and history are available without requiring callers to parse raw
Git bytes. `Repository.ReadTree` returns the names, modes, object IDs, and
types of a tree's direct entries, verifying each in-repository edge.
`Repository.WalkTree` recursively returns those entries with repository-rooted
paths in depth-first tree order. Gitlinks are identified as commits but are
not required to exist locally, matching Git's submodule model.
`Repository.ReadCommit` returns a commit's snapshot tree, ordered parent IDs,
ordered headers, and exact message. `Repository.ListCommitAncestry` follows
all parent edges depth-first, in header order, and emits shared merge ancestors
only once. These graph readers reject missing, malformed, or type-mismatched
edges, while the underlying canonical objects and references remain directly
usable by `git ls-tree`, `git cat-file`, and `git log`.

Named repository state is managed through `Repository.CreateReference`,
`ReadReference`, `UpdateReference`, `ListReferences`, and `DeleteReference`.
The interface handles loose references under `refs/` plus `HEAD`, returns
direct object IDs and symbolic targets without resolving away their identity,
and lists them deterministically by name. Direct targets must identify an
existing object verified through `ReadObject`; symbolic references may point
to an unborn reference, which is how a new repository exposes `HEAD` as
`refs/heads/main` before the first commit. Reference mutations use Git-style
lock files, atomic rename, and directory syncing, and their files remain
interoperable with stock `git rev-parse`, `git show-ref`, and `git symbolic-ref`.

### Storage interface contract

Later repository and remote features use this package rather than manipulating
Git files themselves. The complete durable interface is:

| Concern | Write operations | Read operations |
| --- | --- | --- |
| Repository lifecycle | `Store.Create`, `Store.Delete` | `Store.Open`, `Repository.ID`, `Repository.Path`, `Repository.Inspect` |
| Immutable objects | `Repository.WriteObject` | `Repository.ReadObject`, `Repository.ListObjects` |
| Named state | `Repository.CreateReference`, `UpdateReference`, `DeleteReference` | `Repository.ReadReference`, `ListReferences` |
| Snapshots and history | Objects are written with `WriteObject` | `Repository.ReadTree`, `WalkTree`, `ReadCommit`, `ListCommitAncestry` |

`Repository.Path` is an interoperability handle for stock Git processes, not
an alternate application write API. New durable operations belong on the
storage interface so its atomicity, integrity checks, and error contract remain
the single repository boundary. Objects must be published before direct
references that make them reachable; symbolic references may be unborn.

Compatibility tests construct a representative repository solely through this
interface: regular, executable, symlink, and nested-tree blobs; branched and
merged commit history; branch references; lightweight and annotated tags; and
symbolic `HEAD`. After reopening and reading the repository through the same
interface, stock `git fsck --full` verifies the complete reachable graph, and
stock revision parsing verifies `HEAD` and both tag forms.

Pull-request merge readiness is derived on demand rather than persisted. It
combines immutable request snapshots with live branch tips, current reviews,
ancestry, owner authority, and a stock-Git three-way conflict calculation.
The conflict calculation uses the bare repository only as an object source and
redirects generated merge objects to a disposable directory, preserving the
storage package as the sole durable write boundary.

Maintainers may require a target branch to use coordinated integration without
changing its review or required-check bar. The branch policy records whether
the queue is enabled, candidate concurrency, and whether a failed candidate
pauses the queue or is removed. Inspection includes the existing one-approval
rule and current required-check names as admission criteria. A protected pull
can be mergeable but cannot merge directly; its exact ready revision enters
durable FIFO order, and source synchronization invalidates that admission.
Each admission freezes a synthetic two-parent prospective merge against the
latest eligible target tip and freezes required-check definitions from that
owner-controlled base before running them against the exact merged snapshot.
The API and pull workspace expose the immutable source, base, result commit,
derived lifecycle, and the existing reconnectable logs and artifact evidence.
The durable queue reconciler compare-and-swaps only a passing FIFO head from
its frozen base, records that exact candidate as the pull's merge result, and
then rebuilds affected concurrent candidates against the new target. External
target movement follows the same rebuild path. Superseded evidence remains
inspectable but cannot land; source changes and closure clear admission, while
conflicts and failed or cancelled checks deterministically pause the head or
remove it according to branch policy without deleting the pull request.
The branch queue workspace turns that reconciler into shared coordination:
participants see durable order, candidate attempts, blockers, and the next
automatic or human action. Owners can pause, resume, retry, remove, or move an
entry; each intervention retains actor and time, while queue activity gives
the pull author a direct inbox path back to the change.

The connected browser regression proves this policy under parallel load using
only the web application, public API, and stock Git. Three independently
reviewed changes enter one protected branch together, including a revision
completed by a bounded agent. After the first change lands, an already-passing
candidate that now conflicts is retained as superseded evidence and removed by
policy; the compatible agent change is rebuilt against the evolved target,
verified again, and lands next. The journey inspects durable candidate, review,
run, activity, queue-action, and merge attribution before pulling the final
branch state.

An owner applies an accepted request through its merge endpoint. The operation
rechecks readiness, materializes a two-parent merge commit through the storage
boundary, and compare-and-swaps the live target reference. The durable pull
request records the merge commit, maintainer, and time; its commit message
retains stable request, proposal, author, and merger identifiers. Linked open
proposals close after the contribution lands, while proposal and pull-request
discussion remains intact as collaboration history.

Pull requests also anchor agent-native change sessions. A current collaborator
can open a durable session on the request's exact recorded source revision,
discover it again from the pull request, and inspect a shared, attributable
timeline at a stable web route. The session and its initial event are published
together beneath `CHANGE_SESSION_STORAGE_ROOT`; neither the API nor browser
requires access to worker processes or execution logs. This public session
boundary is where later delegation, progress, intervention, and published
artifacts attach.

Delegation attaches a durable run mandate to that boundary. The initiating
collaborator writes the intended outcome, explicitly confirms the pinned pull
request revision, selects existing repository paths as context, and names a
working branch. Launch returns a one-time, expiring Git credential bound to
that repository and branch; its durable run record retains only the credential
identity and expiry. The session timeline attributes the launch, and any
current participant can revoke access without deleting the mandate. Each run
has a generated durable agent identity. While its credential remains active,
the agent can append status, messages, tool actions, artifacts, failures, and
verified source-branch updates; the store stamps every event with the
authorizing user, agent, run, and pinned revision. Collaborators see that
ordered record refresh in the same session page without needing worker logs.
The same workspace is also the run control plane. Agents can ask explicit
questions, while current collaborators can append guidance or answers and
strictly pause, resume, or cancel active work. Credential-bound workers poll a
minimal authoritative control view; paused runs cannot publish progress, and
canceling permanently closes the run and revokes its branch-bound credential.
Every intervention retains its human actor, generated agent, run, and revision
attribution in the common timeline.

Fork-backed pull requests use the same workspace without flattening ownership.
After the contribution owner enables maintainer edits, a target participant can
delegate an agent whose credential is restricted to that pull request's exact
source repository and branch. Policy removal and target-access revocation take
effect on the next request. Completion imports and adopts the fork revision for
ordinary checks and review replacement, while the agent never receives target
repository write authority.

Completed work crosses into the ordinary review flow through a credential-
bound publication. After the agent commits and pushes its authorized branch,
the API verifies the live tip under the Git reference lock, derives the exact
descendant commits and changed paths from repository objects, and durably
attaches the agent's summary, checks, and unresolved concerns to the run. The
same operation revokes further agent branch access and advances the pull request's recorded source revision, so prior
reviews become stale and existing readiness rules govern the result. The
session renders commit and file links plus the structured handoff beside its
attributed `run.completed` timeline event; collaborators review the resulting
revision through the same pull request surface used for human work.
Receive-pack independently consults durable run state for bounded credentials,
closing branch access even if credential revocation storage fails. The handoff
is persisted before pull-request synchronization so invalid evidence cannot
move review state and a failed synchronization remains safely retryable. The
pull-request lock checks open/merge-intent eligibility before terminalizing the
run and excludes a concurrent merge from entering between those operations.

The connected browser regression proves this as one developer-agent verification
loop rather than independent surfaces. A repository-defined required check
fails on the contributor's pull request with retained logs and an artifact; a
maintainer opens a repair session directly from that evidence, delegates bounded
work, redirects and pauses it, and an agent uses only its one-time credential to
push a descendant and publish the handoff through the public API. Completion
starts checks for the adopted revision, retains the earlier failed run beside
the new passing run, stales the earlier approval, and permits a fresh approval
and owner merge only after the exact revision satisfies policy. The merged
result arrives through a stock Git pull with the same attributed collaboration
history.

## Git HTTP transport

The API exposes a visibility-aware smart HTTP remote at
`/git/<storage-id>.git`. `GET .../info/refs?service=git-upload-pack` advertises
the repository and `POST .../git-upload-pack` serves the protocol-v2 `ls-refs`
exchange used by current Git clients. Both protocol v0 and v2 are passed to
stock `git upload-pack`, preserving Git's capability, symbolic `HEAD`, peeled
tag, and unborn-reference semantics. As a result, `git ls-remote` can discover
empty and populated repositories; in particular, protocol v2 advertises an
empty repository's unborn `HEAD` as `refs/heads/main`.

The API opens repositories through the storage boundary before invoking Git,
so remote IDs use the same validation and stable identity as application
storage. Set `GIT_STORAGE_ROOT` to the store directory when starting the API;
it defaults to `repositories` relative to the process working directory.

Stock `git clone` uses the same endpoint without platform-specific tooling. A
populated clone receives every object reachable from the advertised refs,
retains annotated tags and merged ancestry, and checks out `main` with the
stored file contents and executable modes. Cloning an empty repository creates
a valid working copy whose local `HEAD` remains the unborn `main` branch, ready
for its first commit.

Existing working copies remain synchronized through the same smart
HTTP endpoint. After the server publishes new objects and advances `main`, a
stock `git fetch` negotiates from the commits already present locally, receives
the missing history, and updates `origin/main` without moving the checked-out
branch. A subsequent stock `git pull --ff-only` fetches later state and
fast-forwards the local `main` branch and working tree without recloning.

Stock clients publish work through receive-pack discovery and
`POST .../git-receive-pack`. They can create the unborn `main` branch and
advance it with fast-forward commits. An explicitly forced push can replace
its history, and an explicit deletion returns the repository to an unborn
`main`; a later push can recreate it. Stock clients continue to protect
ordinary non-fast-forward pushes unless force is requested. Pushes to
any named branch under `refs/heads/` are accepted with the same create, update,
force, and delete semantics. All branches are advertised and fetchable, so a
contributor can publish and revise a candidate branch without moving `main`.
Tags and other non-branch ref namespaces remain denied. Receive-pack validates
the complete request before applying its ref transaction, so rejected requests
do not partially change named state.

End-to-end compatibility suites treat the HTTP endpoint as an opaque remote and
drive it with stock Git commands. They cover the entire primary-branch
lifecycle as well as discovery, fetch, creation, advancement, and deletion of
a candidate branch while verifying that the maintained branch remains fixed.

Assigned proposal tasks publish human or completed-agent branch work through a
task-scoped command that creates an ordinary pull request. Pulls retain stable
proposal, task, session, and run provenance; tasks retain each exact candidate
attempt. Review, checks, closure, supersession, and merge stay traceable, while
only a merge completes planned work.
The connected orchestration journey proves the complete boundary with two
browser identities and stock Git: discussion becomes a dependent human/agent
plan, blocked work remains unassignable, the human contribution verifies and
merges first, the newly ready agent receives attributable guidance and
publishes an exact completed outcome, and its connected pull verifies and
merges before the team closes the proposal. Durable task history and the
session timeline retain attribution across every handoff.

## Cross-repository interface evidence

Repositories can publish named semantic-version interfaces from immutable
release candidates and declare consumer constraints at exact verified source
revisions. A declaration may additionally pin its consumer release and
environment, allowing the platform to connect code ownership to released and
deployed evidence rather than an external inventory. The repository
relationship workspace shows every currently visible provider and consumer,
their owners, exact commits, releases, environments, constraints, and resolved
version.

Relationship state is derived on each read. A compatible current publication
is `resolved`; missing or mismatched source, release, active successful
environment deployment, or environment evidence is `stale`; and a visible provider with no
compatible publication is `unresolved` with a reason. Repository visibility is
rechecked while assembling the graph, so private provider edges and dependency
metadata never become a cross-repository discovery side channel.

Evolution decisions turn that graph into repository-owned migration work.
Current participants claim provider or affected-consumer tasks with a target
version, ordered cross-repository dependencies, completion criteria, an exact
base, and a human or agent mandate. Each link creates an ordinary local proposal
task rather than giving the provider team authority elsewhere. Its discussion,
assignment, branch, pull request, and merge status project back into the shared
plan. Humans retain existing repository or owned-fork access; agents start only
after earlier linked work merges and receive the existing task-branch credential.

Before release, provider participants can select the provider pull and one or
more affected-consumer pulls as an exact compatibility matrix row. The platform
archives those immutable source revisions beneath `provider/` and
`consumers/<repository-id>/`, imports a deterministic synthetic commit into the
provider object database without publishing a ref, and executes the provider's
`.vivarium/contracts.json` definitions. Contract checks reuse the read-only,
network-disabled check runner and receive no platform credential. Plan evidence
retains every exact combination, initiating actor, bounded logs, checksummed
artifacts, and a revision-hash attestation; a changed revision supersedes only
rows containing that repository while unrelated combinations remain current.

## Release candidates

Release definition is a durable, immutable boundary over repository history.
An owner or contributor chooses a verified commit, a repository-unique version,
release notes, and optionally an earlier release candidate. The API requires
that earlier candidate's commit to be an ancestor, then snapshots every merged
pull request whose merge commit is in the new ancestry range. Linked proposals
and plan tasks are retained with the pull set, and contributor IDs are derived
from pull authors and mergers plus linked planning attribution. This makes the
declared source, rationale, work, and people inspectable before artifact builds
or environment delivery begins.

Candidate records live beneath `RELEASE_STORAGE_ROOT` (default `releases`) and
stay in `candidate` lifecycle state. Candidate commits define ordered release
steps in `.vivarium/release.json`; those steps reuse the exact-commit,
network-disabled OCI verification executor and retain bounded logs and
immutable SHA-256 artifact evidence beneath `CHECK_RUN_STORAGE_ROOT`. Public
attestations connect source, command, image dependency, initiating actor,
attempt outcomes, and artifacts. Reruns append evidence rather than replacing
failures. Public repositories expose
candidate reads anonymously, private reads follow repository access, and only
current owners or contributors with `repositories:write` can create them. The
web release workspace is linked from repository detail and supports exact-state
creation, prior-release selection, lifecycle inspection, build controls,
machine-readable attestations and checksums, and direct links back to included
review and planning work.

An owner can turn one current successful build artifact into an immutable
package version from that same release workspace. Package publication freezes
the globally repository-bound identity and version, source commit, release and
build IDs, artifact identity and SHA-256, publisher, platform selectors,
dependency constraints, public or private visibility, and active lifecycle.
Publishers also attach a bounded summary, version documentation, and license;
these claims remain adjacent to the derived evidence rather than replacing it.
Publication holds the build execution boundary across selection, hashing, and
the atomic version rename, preventing a concurrent rerun from replacing the
attested attempt. The package store hashes output into a staging directory, so
pre-rename verification or storage failures leave no readable version or
first-publisher name reservation. Post-rename parent-directory failures return
`202` durability uncertainty together with the complete package identity; an
exact retry recovers the same stable record, while different content still
conflicts. Public package
metadata and bytes are anonymous; private packages continuously reuse the
source repository's access boundary. Durable records and copied artifacts live
beneath `PACKAGE_STORAGE_ROOT` (default `packages`), independent of later build
retention or repository visibility changes.

The `/packages` workspace and catalog API search names, summaries, and version
documentation across public packages plus private versions the current actor
can read. Identity version lists and resolution select the newest non-yanked
version satisfying an exact, caret, tilde, or ordered semantic-version
constraint and optional OS, architecture, and runtime selectors. Owners may
mark a version deprecated or yanked with a required warning; artifact bytes,
checksum, release, source, and build provenance remain unchanged, and yanked
versions are excluded from new resolution while staying inspectable.

For ordinary dependency clients and isolated builds, a current participant in
a consuming repository can mint a short-lived `packages:read` credential. The
credential freezes the consuming repository ID and an explicit allowlist of
package identities that the issuer can currently read. Registry metadata,
resolution, and artifact endpoints honor that allowlist, so the token has no
Git, repository mutation, publisher, or unrelated-private-package authority.

Repositories make actual package use reviewable with a versioned
`.vivarium/packages.json`. Its `dependencies` array declares direct semantic
constraints and its `lock` array names exact package versions. Recording an
inventory for a verified commit derives transitive paths from immutable package
metadata, attributes the snapshot to its caller, and retains unresolved or
constraint-stale entries plus missing license, support, or package provenance.
Inventory reads project exact-commit release builds, artifacts, and governed
deployments without rewriting the historical lock snapshot. A deployment is
current only when it is the newest successful promotion in its environment;
superseded successes remain labeled historical. Repository surfaces
show every authorized revision inventory; exact package versions show only
consumer repositories the viewer can currently read.

Package adoption begins from that reviewed inventory rather than an untrusted
publisher callback. A consumer owner can define a patch, minor, or major policy
for each direct package and scan the current default-branch lock for eligible,
visible, active releases. The scan preserves the exact base and proposed
`.vivarium/packages.json`, attaches immutable publisher release notes and the
successful package-build attestation, derives every affected dependency path,
and opens an attributable proposal with one executable adoption task. Exact
base/from/to retries reuse the same durable update through a cross-process
reservation persisted before proposal creation; incomplete proposal/task
publication is compensated, while failed compensation leaves a hidden recovery
marker that blocks duplicate exact retries. Reads also
revalidate current access to the target package before returning its retained
notes or attestation. Ordinary proposals redact private-package notes, release
identity, and build details because proposal authorization follows only the
consumer repository; authorized users inspect that evidence on the update
record instead. Humans or scoped task agents
can investigate and revise it through ordinary change sessions and pull
requests, so required checks, reviews, protected integration queues, later
release builds, and deployments stay consumer-controlled; the package publisher
gains no Git, repository, review, or package-publishing authority in the
consumer repository.

Release delivery is an explicit repository collaboration. Owners define a
strictly ordered set of environments with visible scoped configuration,
required independent approvals, and concurrency limits. Protected credential
values are accepted only on environment writes, encrypted with a durable
deployment-store key, and never returned; readers see only their names.
Participants select one checksummed artifact from a successful build of the
candidate's exact commit. Promotion to each later environment requires the
same release, build, artifact identity, and checksum to have succeeded in the
immediately preceding environment. Every request, independent approval, queue
transition, provision/status message, and completion is retained with actor and
time through the API and release-detail workspace. Records live beneath
`DEPLOYMENT_STORAGE_ROOT` (default `deployments`).

Terminal unhealthy delivery becomes an explicit recovery decision. A current
participant can ask the server to derive the newest earlier successful
deployment for the same environment and submit that exact release, build,
artifact, and checksum as a new governed promotion. The rollback retains both
the unhealthy deployment and the successful deployment it restores; it does
not bypass environment approval, ordering, concurrency, artifact verification,
or rollout observation.

Alternatively, a participant can open a source repair directly from the
unhealthy deployment. The server creates a deterministic isolated
`agent/recovery/*` branch at the current default-branch tip, opens an ordinary
pull request against that branch, and attaches a change session snapshot containing release version and
notes, deployment state and logs, health evidence, artifact identity and
checksum, and exact source revision. Agents launched from that workspace get
only the existing pull-branch Git credential. Completion synchronizes the pull
and therefore re-enters fresh required checks, review, protected integration,
and a newly built release before any later promotion. No repair credential can
read protected environment values or execute a deployment.
The deterministic deployment-keyed branch also makes retries reconnect a pull
or session published before a storage failure instead of duplicating repair
work. Rollback selection and promotion publication share one cross-process
store lock, so the failed deployment and known-good target cannot become stale
between derivation and the durable recovery write.
If failure leaves only the deterministic repair ref, retry fast-forwards it to
the current default tip only when the old ref remains an ancestor. Divergence
or a concurrent branch update is rejected instead of overwriting work or
opening a stale repair pull.
Pull and change-session persistence independently enforce recovery uniqueness
inside their cross-process locks. Concurrent requests for the same deployment
therefore converge on the one deterministic source branch, pull request, and
evidence-pinned session instead of splitting diagnosis across workspaces.

The connected browser regression carries the earlier human/agent proposal plan
across this complete delivery boundary. It freezes a known-good baseline, builds
and promotes exact artifacts with independent approval, releases both merged
contributions, retains a failed production health signal, restores the earlier
artifact through the same governance, delegates an evidence-pinned repair, and
requires fresh checks, review, merge, build, and approval before the corrected
release succeeds. The executor keeps artifact bytes private on the host: after
SHA-256 verification it creates a short-lived owner-only copy, bind-mounts that
copy read-only into deployment and health containers running as the API host
identity, and removes it after execution.

Each environment also defines an executor image, command, and timeout. The
worker reopens the immutable build artifact, recomputes and matches its SHA-256,
then mounts it read-only at `$VIVARIUM_ARTIFACT` in a capability-free,
read-only container. Visible configuration and decrypted protected values are
provided only through a mode-0600 environment file at this execution boundary;
retained output is bounded and credential values are redacted. A command or
verification failure becomes durable failed evidence and cannot unlock a later
environment. Recovery continuously resumes queued work and conservatively
uses a persisted, renewable execution-owner lease to distinguish live commands
from work abandoned by a prior process. Live work renews through the command
and completes through an owner compare-and-swap; only an expired lease fails
closed because its external result is unknown. Setup failures that occur before
claiming execution reject the queued record into a terminal failed state.

Incident response carries operational evidence through an attributed
post-incident review and into proposal-owned corrective commitments. Those
commitments remain visible from assignment through pull review, checks,
release, and deployment instead of ending when the incident is marked resolved.
## Protected vulnerability coordination

`securityadvisories.Store` is an isolated durable boundary for suspected
vulnerabilities. An authenticated researcher can name readable public or
private repositories, affected version expressions, bounded evidence, and a
safe return channel. Discovery is restricted to the reporter, current owners
of affected repositories, and a maintainer-invited response team capped at 20
people. Owners make versioned severity and embargo decisions; every detail
read and mutation is retained in the report's own access audit.

This subsystem intentionally has no dependency on the activity or inbox
stores. Protected titles, evidence, messages, membership, and even report
existence therefore do not enter ordinary repository collaboration feeds. The
web surface at `/security` uses only the dedicated `/security-advisories`
contract and explains that privacy boundary before submission.
The report form accepts either a repository from the actor's catalog or the
stable ID copied from a public repository URL, so an outside researcher does
not need a collaborator grant merely to file through the web application.

Participants build a protected evidence graph from verified commits,
dependency resolutions, releases, builds, release artifacts, and deployments.
Dependency links are resolved against an exact release build's frozen image
definition rather than accepting a caller-supplied package claim.
Attributable hypotheses, conclusions, and uncertainty explain what those links
mean, while a compare-and-swap version-line by environment matrix records
`confirmed`, `suspected`, `unaffected`, or `fixed` impact. Responders can freeze
selected evidence behind a short-lived `security:investigate`-only credential;
agent findings remain inside the same embargoed record.

Responders can split fixes into protected human or agent repair tasks for each
affected version line, including dependencies in another affected repository.
Each task freezes its base and mandate. Starting work creates a transport-hidden
branch and a short-lived, revocable credential that advertises only that exact
branch. Commits, discussion, reviews, and authorship stay in the advisory rather
than ordinary pull, activity, inbox, or search stores while embargoed.

Repository owners also define embargoed security reproductions for each affected
version line. A completed repair session can reserve one proof set that combines
the required-check names in current branch policy with executable definitions
frozen from the task's trusted base. It executes those definitions and the
private reproductions against the exact completed commit, so repair-controlled
configuration cannot weaken a gate while retaining its required name. The
task base must be the current default-branch tip or one of its ancestors; orphan
objects and unmerged feature commits cannot become trusted definition sources.
ordinary check API cannot discover this session-keyed evidence; advisory status
projects only names, states, candidate identity, and checksummed artifacts, not
commands or logs. A repository owner other than the repair worker must approve
a wholly passing proof before the change is marked ready for protected
integration.
Required checks and private reproductions are reserved as one exact run set;
the safe read projection remains pending until every reserved run is present,
so partial publication cannot appear approval-ready.

After integration, an owner can attach a release candidate only when its commit
contains the approved repair candidate, all exact release build steps succeeded,
and at least one checksummed artifact was produced. The immutable attestation
connects that artifact and release ancestry back to the affected version line.
The security workspace summarizes missing tasks, reproductions, approvals, and
release artifacts for every claimed line so disclosure cannot accidentally
present an unsupported line as fixed.

An affected-repository owner can then prepare one redacted disclosure packet.
Preparation is rejected until every affected repository/version line maps to an
approved, checksummed release attestation. The packet freezes public credits,
upgrade guidance, affected and fixed versions, exact release commits, artifact
checksums, and deterministic repaired branch names. Publishing is a durable,
retry-safe sequence: exact commits are staged beneath a transport-hidden repair
namespace that is also excluded from branch and named-revision browser APIs,
the advisory is made durably and anonymously readable, and only then
are public repaired refs and targeted notification records emitted for
affected repository owners, current collaborators subscribed through repository
access, and users who initiated deployments of those repositories. Any failure
retains the workflow with its remaining steps visible inside the protected
workspace. Public advisory reads remain not-found until the durable disclosure
transition, then stay available while downstream delivery retries; they never
serialize protected evidence, findings, messages, contact details, commands, or
logs.
The connected browser regression carries an external report through frozen
agent diagnosis, isolated Git repair, private reproduction and required-check
proof, independent approval, release build attestation, browser disclosure,
anonymous redaction checks, repaired branch publication, and targeted upgrade
notification as one permission-aware journey.

## Accountable organizations

Organizations provide a durable group identity over existing repository-scoped
work. Membership begins with an owner invitation and explicit acceptance. New
repositories can start inside the group; an individually stewarded repository
joins only after its current custodian requests transfer and an organization
owner accepts it. The association preserves repository and Git identity, every
historical link, and actor attribution. Existing user-owner checks continue to
name the control custodian, while accepted members receive separately tracked
collaboration access that can be removed without revoking older independent
grants.

The `/organizations` workspace exposes creation, invitations, membership,
repository creation and transfer acceptance. Its portfolio joins repositories
to packages, active proposals and pull requests, releases, and unresolved
incidents without copying those authoritative records. Durable group,
invitation, and transfer state defaults to `$ORGANIZATION_STORAGE_ROOT`
(`organizations`).

Portfolio initiatives turn existing proposals, interface evolution plans,
incidents, and authorized private security work into an organization-level
outcome map without copying their workflow state. Ordered work items connect
repository contributions and dependencies to accountable humans, teams, or
approved agents. The portfolio derives blockers, relevant policy exceptions,
and upcoming release candidates from live records. If membership, agent
operation, team identity, or repository stewardship changes, the original work
and attribution remain visible with an explicit reassignment action.

Organizations also retain nested teams, direct member and maintainer roles,
repository areas of responsibility, and approved agent identities. Every team
mutation uses its visible version as a compare-and-swap guard and appends an
actor-stamped event. Effective membership explains whether participation is
direct or inherited from a visible child team. Agent approval publishes
capabilities, current member operators, and team associations but grants no
credential or repository access by itself. Accepted members can request an
explicit role for a team they belong to or an approved agent they operate,
targeting named repositories, packages, environments, or collaboration
records. Owners approve or deny those requests; every request, decision,
grant, derived credential, and revocation is actor-stamped in organization
history. Grants retain their reason, optional expiry, and resource-specific
deny exceptions so effective authority explains both its source and its
limits. Agent operators can derive a short-lived Git credential only for an
exact repository named by a live grant, and its lifetime is capped by the
grant. Revocation walks only that grant's recorded credentials and invalidates
them immediately, preserving unrelated sessions and work. Public directory reads show only public teams
and agents and public-repository responsibility; accepted members see the full
organization-visible structure and attribution timeline.

An accepted proactive stewardship mandate turns trusted evidence into a shared,
rank-ordered opportunity backlog. Activation requests an evaluation; trusted
producers use the same public evaluation boundary after relevant repository,
dependency, check, release, incident, security, or usage changes. Findings
explain their scope and retain severity, expected value, confidence, affected
owners and revisions, and revision-pinned citations. Stable deduplication
updates one item as evidence changes, while superseded citations remain visibly
stale. The queue grants no new authority. Organization collaborators inspect,
discuss, rerank, dismiss, snooze, reopen, or mark recommendations incorrect
with compare-and-swap decisions, making proactive attention challengeable.
Each mandate can classify evidence types and severity thresholds as requiring
maintainer approval or eligible for bounded auto-start; omission always means
approval. Acceptance promotes evidence into the platform's ordinary proposal
and ordered-task model at one exact default-branch revision, retaining owners,
completion criteria, risks, verification plans, and stewardship provenance.
The admission boundary spends no compute and creates no branch. It reports
active incidents, security embargoes, duplicate work, exhausted budgets, moved
bases, changed policy/acceptance, and racing decisions before work is assigned,
then projects accepted assignments through the existing activity and inbox
surfaces.

Promoted agent work remains governed after planning. Only the mandate's
approved agent may own its agent task, and launch revalidates the exact active
mandate version, accepting operator, linked opportunity, recorded base, and
assignment before creating the ordinary isolated task branch and credential.
Pause, expiry, revision, operator change, or agent replacement stops new
execution without erasing planned work. Completion retains bounded commands,
check claims, residual concerns, and a met, partial, or not-met status with
evidence for the recorded criterion. A stewarded pull cannot publish without
that command and criterion evidence. The ordinary pull freezes those claims
beside server-derived commits and files, exact opportunity citations, mandate,
base, initiator, and agent authorship. It gains no exception from repository or
fork boundaries, owner acknowledgements, checks, review, integration queues,
stale-revision protection, or merge permission.

Long-running mandates retain a learning ledger alongside that governed work.
Collaborators can inspect recommendation dispositions and decisions,
implementation and verification outcomes, release results, resource use,
false-positive feedback, and progress against each declared goal. Maintainers
may use that history to reorder or suppress already-authorized evidence and set
a stricter confidence floor without changing authority. Any expansion of
signals, scope, actions, budget, agent, or access remains a versioned mandate
revision that the operator must accept again. Repeated failure, inactivity,
revoked repository access, anomalous consumption, or budget overrun pauses the
affected automation and leaves an attributed remediation notice rather than
silently continuing.

The connected stewardship journey proves that lifecycle as one public
collaboration record. A maintainer discusses and approves a trace-backed
finding while dismissing a lower-value recommendation; the accepted operator
then launches the exact promoted agent task, receives maintainer guidance,
publishes structured command and criterion evidence, and hands the pull through
normal checks, review, merge, and release. The retained report links the
opportunity and delivered outcome, accounts for reserved and reported use, and
remains readable after the maintainer revokes the mandate.

The access API is rooted at `/organizations/{id}/access-requests`. Decisions
are posted to `/access-requests/{request-id}/decision`; live grants appear on
the member organization and portfolio projections, revoke with an expected
version at `/access-grants/{grant-id}`, and issue agent credentials beneath
`/access-grants/{grant-id}/credentials`. Expired, revoked, and explicitly
excepted resources never authorize credential issuance.

Organization policies provide a versioned governance baseline across repository
visibility, review count, named checks, queued integration, attested release
provenance, dependency eligibility, promotion approvals, and agent authority.
Owners save policies as drafts targeted to the whole organization, a team, or a
repository and preview the merged repository impact at
`/organizations/{id}/policies/preview` before activation. Active policy reads
omit drafts and retain both the strict baseline and any effective local value.
Activation applies to new workflow decisions, so existing pulls, releases,
deployments, and credentials keep their frozen evidence rather than being
retroactively invalidated. Responsible team maintainers can request a named,
reasoned, expiring exception; owners decide it, and projections keep the
requester, decision, expiry, original baseline, and adjusted value visible.

The connected organization browser journey treats those records as one product
workflow. An owner creates a two-team, two-repository portfolio, onboards a
developer and approved agent, activates a shared review and delivery baseline,
approves a maintainer-requested exception, and issues an exact repository-bound
agent Git credential. Human and agent commits enter ordinary pull review before
the developer is removed; removal revokes the derived credential immediately
but preserves both pull requests, their authorship, the initiative map, and the
policy decision. The owner then merges the retained work and carries its exact
package inventory through an attested release and governed deployment. The
Playwright server isolates organization records beneath
`$ORGANIZATION_STORAGE_ROOT` with every other temporary journey store.
