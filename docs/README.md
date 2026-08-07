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
copy clone information. The selected `ref` and repository-relative `path` live
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

Pull requests connect that context to exact repository state. An owner or
contributor opens one from an existing source branch against a different
target branch, optionally linking a repository proposal. The durable resource
records its author, purpose, branch names, and both commit IDs at creation, so
later branch movement does not silently change the review. After responding
to feedback with another source-branch push, the author explicitly
synchronizes the request to adopt that tip as its next reviewable revision;
the target snapshot remains fixed.

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
Immutable pull request comments retain stable author IDs; owners and
contributors may participate, while reads continue to follow repository
visibility.
The web pull request workspace aggregates candidate work across the signed-in
actor's repository catalog and opens requests from existing branch pairs with
optional proposal context. Its directly addressable detail pages use the
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
move review state and a failed synchronization remains safely retryable.

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
