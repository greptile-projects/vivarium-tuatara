# AGENTS.md

Guidance for coding agents working in this repository.

## What this is

A Bun workspace monorepo with two apps:

```
apps/web    Next.js frontend (TypeScript, React 19, Tailwind v4)
apps/api    Go HTTP API
docs/       notes on how the pieces fit together
```

`apps/web` has its own `AGENTS.md` with a rule that matters: this is a newer
Next.js than your training data, so read `node_modules/next/dist/docs/` before
writing frontend code rather than reaching for remembered APIs.

## Commands

Run these from the repo root. The runtime is **Bun**, not Node — use `bun`, not
`npm`/`yarn`.

```sh
bun install       # install workspace deps
bun dev           # web  → http://localhost:3000
bun run dev:api   # api  → http://localhost:8080/health
bun run build     # next build
bun run lint      # eslint over apps/web
bun run --cwd apps/web test:e2e # full two-user browser + stock Git journey
```

There is no root typecheck script. Typecheck the frontend the way CI does:

```sh
cd apps/web && bunx tsc --noEmit
cd apps/api && go vet ./... && go build ./...
```

## What CI checks

Two workflows gate a pull request, each scoped by path — they only run when
that app changed:

- **web** (`apps/web/**`, `package.json`, `bun.lock`) — `bun install
  --frozen-lockfile`, `bun run lint`, `bunx tsc --noEmit`.
- **api** (`apps/api/**`) — `go vet ./...` and `go build ./...` in `apps/api`.

Run the matching commands locally before pushing; the four above are the whole
gate. `bun install --frozen-lockfile` is what CI uses, so commit `bun.lock`
whenever dependencies change or the web job fails before it starts.

## Conventions

- **Frontend** — App Router, file-based routes under `apps/web/src/app`. Entry
  point is `src/app/page.tsx`; `layout.tsx` installs the persistent application
  shell from `src/components/app-shell.tsx`. Reuse the accessible visual
  primitives in `src/components/ui.tsx` and the stroke icon set in
  `src/components/icons.tsx` rather than creating route-local variants.
  Global design tokens, focus treatment, reduced-motion behavior, and base
  styles live in `globals.css`. Tailwind v4 is wired through PostCSS
  (`postcss.config.mjs`); there is no `tailwind.config` file. The shell and
  presentational pages remain Server Components unless an interaction truly
  needs client state. Navigation uses `next/link`; every page renders its
  primary content inside the shell's `#main-content` landmark. Browser API
  calls use the typed helper in `src/lib/api.ts` and same-origin `/api/*`
  requests; clone URLs use same-origin `/git/*`. Next rewrites both to
  `$API_ORIGIN` (default `http://127.0.0.1:8080`). `AuthProvider` retains the API-issued bearer token
  in browser local storage, validates it through `GET /user` at startup, and
  is the shared identity boundary for interactive account and repository
  workflows. Proposal discovery at `/proposals` aggregates the authenticated
  actor's repository catalog and provides repository, status, and text filters;
  durable conversations use `/proposals/{repository-id}/{proposal-id}` for
  attributable comments, author edits, and participant closure controls.
  Proposal detail also carries an ordered executable task plan: current
  participants define expected outcomes, same-proposal dependencies, and links
  to motivating comments; readiness is derived from completed dependencies,
  while task edits, status decisions, and reordering retain actor-stamped
  immutable history through the public proposal task APIs.
  Task definitions carry context revisions; only current merged dependency
  contributions satisfy readiness. Definition or contribution changes surface
  assigned work as changed or obsolete, notify human assignees, and require an
  explicit compare-and-swap rebase to a verified commit before replacement work
  can represent the current plan; earlier sessions and pulls remain traceable.
  Ready proposal tasks have at most one explicit human or generated-agent
  assignment. Each freezes a mandate, repository, and exact base commit;
  assignment IDs provide compare-and-swap claim, reassignment, and revocation
  semantics, and the access preview grants humans nothing while limiting future
  agent credentials to the repository and task branch. Human assignment holds
  the catalog mutation lock across participant revalidation and proposal write,
  excluding collaborator removal; closed proposals reject revocation.
  Starting an agent assignment creates one assignment-scoped change session and an
  isolated `agent/tasks/*` branch at its frozen base without creating a pull
  request. The launched mandate snapshots proposal, task, dependency,
  discussion, and repository context and reuses the public session timeline,
  guidance, pause/resume/cancel, control, and bounded Git credential surfaces.
  Task-run completion validates the exact live task branch as a new descendant
  of the frozen base, derives commits and changed files, stores structured
  evidence, and revokes the credential without creating a pull request.
  Start holds the proposal mutation lock across exact assignment revalidation
  and publication; its session and initial run share one atomic durable record,
  preventing revoked launches and stranded session-only retries.
  Rebasing preserves earlier task sessions as evidence while allowing the new
  assignment ID to start one fresh session; only runs from that assignment's
  branch can publish its replacement contribution.
  Assigned task work publishes as an ordinary pull request with stable
  proposal, task, optional session/run, exact commit, and check provenance in
  both directions. Publication means review, not completion: replacement
  attempts supersede earlier candidates, closure returns the task to todo, and
  only merge completes it; task-linked merges do not close the whole proposal.
  The pull retains durable task-state repair intent until bidirectional
  publication/close/merge projection succeeds; pending links block merge and
  reconcile on pull reads. Agent publication compare-and-swaps the live task
  branch against the completed run outcome commit.
  The connected orchestration browser journey carries a discussed two-task
  human/agent plan through dependency-gated assignment, stock Git publication,
  guidance, task-run completion, exact-revision checks, review, ordered merges,
  proposal closure, and durable attribution assertions.
  Repository release candidates at `/repositories/{id}/releases` freeze a
  verified commit, version, notes, creator, optional prior-release boundary,
  and the server-derived merged pulls, proposals, tasks, and contributors in
  that ancestry range. They remain immutable `candidate` records for later
  build and promotion workflows; the web surface lives beneath repository
  detail at `/repositories/{id}/releases`. Exact candidates opt into isolated
  builds through `.vivarium/release.json`; steps reuse the network-disabled
  check executor, retain immutable attempts, logs, and checksummed artifacts,
  and expose source/command/dependency/actor/result attestations. Reruns append
  evidence for the same frozen candidate.
  Owners define ordered delivery environments with scoped visible configuration,
  encrypted write-only credentials, independent approval thresholds, and
  concurrency limits beneath `$DEPLOYMENT_STORAGE_ROOT`. Participants promote
  one successful exact-release artifact; later environments require the same
  release, build, artifact, and checksum to have succeeded immediately before.
  Initiators cannot self-approve, and immutable deployment events retain every
  request, approval, queue, status/log, and completion transition. Execution
  reopens and SHA-256 verifies the exact build artifact before mounting it
  read-only into the owner-defined container command; decrypted values enter
  only that boundary and output is bounded and secret-redacted. Startup and a
  periodic recovery pass resume queued work; running work carries a bounded
  renewable execution-owner lease so live commands are preserved, while an
  expired lease fails closed because its external outcome is unknowable.
  Pre-execution policy failures terminalize queued work without consuming
  environment capacity indefinitely.
  Exact release commits define ordered rollout stages and isolated health
  signals in `.vivarium/deployment.json`; deployments freeze that definition,
  affected commit, live stage, and retained signal evidence. Participants use
  compare-and-swap pause, resume, cancel, and unsuccessful controls, whose
  actor-stamped decisions notify the initiator through activity and inbox.
  A pause retains the renewable execution owner and suspends rollout
  observation time; deployment and health commands keep independent bounded
  timeouts, and signal evidence that finishes during a pause remains durable
  before the executor waits for resume. Owned failed signals may terminalize a
  paused rollout; recovery scans paused work and fails an expired owner with an
  unknown-outcome record before any later resume.
  Pull request discovery at `/pulls` aggregates reviewable work across the
  authenticated actor's repository catalog and opens candidate branches
  against distinct targets or owned-fork branches against their upstream,
  with optional proposal context. Durable detail
  routes at `/pulls/{repository-id}/{pull-request-id}` expose the recorded
  branch snapshots, source-only commits, path-ordered file changes, linked
  proposal, attributable discussion, current and stale review decisions,
  source synchronization for authors, server-derived merge blockers, and
  owner-only merge controls. Completed requests retain their merge attribution.
  Owners can protect individual target branches with an integration queue,
  configure 1-10 concurrent candidates and `pause` or `remove` failure
  behavior, and inspect the existing approval and required-check admission
  rules. Protected ready pulls must be admitted instead of merged directly;
  `queued_at` supplies initial FIFO order; exact rational `queue_rank` values
  allow reprioritization to atomically update only the moved entry. Both clear
  when the source changes or the pull closes. Admission freezes an immutable synthetic two-parent
  candidate from the exact pull revision and latest eligible target, launches
  target-base-bound required-check definitions on that prospective result,
  and exposes candidate identity, base, lifecycle, logs, and artifacts. A
  partial candidate check-run publication is repaired definition-by-definition
  on reconciliation, so a durable prefix cannot strand a required run. A
  cross-process reconciler advances only a passing FIFO head by compare-and-
  swapping its frozen base to that exact candidate, records it as the durable
  pull merge, and rebuilds affected concurrent candidates after either that
  merge or an external target update. Superseded evidence is retained but
  never lands; source changes and closure clear admission, while conflicts and
  failed or cancelled checks follow `pause` or `remove` without deleting pull
  request history. The branch queue surface exposes durable order, current
  candidate state, blockers, predicted next action, and retained attempts;
  owners can pause, resume, retry, remove, or reprioritize entries, with
  attributed history and author-targeted activity/inbox outcomes. Automatic
  completion closes a linked open proposal and
  records the same attributable proposal and merge activity as direct merge.
  Pull metadata retains pending finalization until those cross-store effects
  succeed; recovery retries them with stable idempotent activity identities.
  Fork pull authors can opt current target participants into short-lived Git
  access restricted to the issuing pull request and exact contribution branch; policy removal, closure,
  or target-access revocation invalidates that access on the next request.
  Change-session agents use this same dynamic grant for cross-repository pulls:
  their bounded credential targets the fork source branch, never the upstream,
  and completion adopts the fork revision through ordinary pull synchronization.
  Cross-repository pull authors can participate in attributable discussion
  while a public upstream remains readable without receiving upstream
  membership; review, check control, and merge authority remain target-scoped.
  Pull authors and target owners can close open requests while retaining their
  snapshots and evidence; only target owners can merge.
  Owners select required verification names per target branch from repository
  detail; readiness and merge require successful durable runs for the pull
  request's exact adopted source revision and report every requirement status.
  Owners manage contributors from repository detail pages using the stable
  collaboration ID each user can copy from Settings. The Playwright journey in
  `apps/web/tests` is the connected-product regression and uses isolated
  temporary API storage plus the system Chromium and stock Git clients. It
  begins with an unknown user discovering a public repository, forking and
  synchronizing it without an upstream grant, then carries delegated work
  through a pull-scoped bounded agent credential, public progress
  and completion APIs, a stock Git push, stale-review replacement, and merge;
  keep that full handoff intact when changing agent or review workflows.
  The same journey is the verify-repair-merge regression: it installs a required
  repository-defined check, retains its failed evidence in an agent repair
  session, proves completion starts fresh exact-revision checks, and merges only
  after the repaired run passes and stale review is replaced.
  A separate queued-integration journey protects `main`, submits three approved
  parallel changes, completes one through a bounded agent run, and proves a
  passing candidate that conflicts after an earlier merge is superseded and
  removed while the compatible agent change rebuilds and lands. Keep its public
  API, browser queue, stock Git, evidence, and attribution assertions connected.
  The authenticated `/activity` workspace shows newest-first attributable
  proposal, pull request, review, merge, mention, and access changes across
  repositories the actor currently collaborates on; every event remains
  subject to current repository authorization.
  The authenticated `/inbox` workspace derives only recipient-specific work
  from those immutable events, classifies it as review, response, or awareness,
  links directly to the underlying collaboration, and persists per-user clears
  without deleting activity. Inbox reads recheck current repository access and
  resource state so revoked or completed work does not retain obsolete actions.
- **API** — a single `main.go` registering handlers on a `net/http` mux with
  Go 1.22+ method-and-path patterns (`"GET /health"`). It has no third-party
  dependencies and no `go.sum`; adding a dependency means the api workflow's
  `cache: false` line should flip to `cache-dependency-path: apps/api/go.sum`.
  The port comes from `$PORT`, defaulting to `8080`. Bare Git repository
  lifecycle storage lives in `apps/api/storage`; callers create or reopen a
  stable storage ID there before performing repository operations, and
  `Store.Delete` atomically detaches an ID when its repository is removed.
  Deletion uses a stable `.deleting-<id>` tombstone; cleanup failures retain
  that locator, block ID reuse, and are completed by retrying `Store.Delete`.
  `Repository.WriteObject` and `ReadObject` are the durable object boundary;
  they use canonical Git loose-object storage and support blob, tree, commit,
  and annotated tag objects addressed by lowercase SHA-1 object IDs.
  `Repository.ListObjects` discovers every loose object, verifies it through
  the same read boundary, and returns the complete objects ordered by ID.
  `Repository.ReadTree` exposes direct snapshot entries and `WalkTree`
  recursively returns repository paths. `Repository.ReadCommit` exposes a
  commit's tree, ordered parents, headers, and message, while
  `ListCommitAncestry` traverses the complete deduplicated parent graph in
  depth-first parent order.
  Loose references are managed through `CreateReference`, `ReadReference`,
  `UpdateReference`, `ListReferences`, and `DeleteReference`. Direct targets
  must name an existing verified object; symbolic targets may be unborn so
  `HEAD` can identify the default branch before its first commit.
  Reference reads and listings also merge stock Git `packed-refs`, with loose
  references taking precedence, so browser and interoperability reads survive
  normal `git pack-refs` maintenance. Deletion rewrites a matching packed entry
  under Git's standard lock before removing its loose override, preventing
  packed fallback from resurrecting a deleted branch.
  `Repository.Path` is reserved as an interoperability handle for stock Git
  processes; application storage writes should go through the package API.
  The integrated compatibility test builds representative merged history and
  lightweight/annotated tags through that API and requires `git fsck --full`.
  Read-only smart HTTP remotes are exposed at `/git/<storage-id>.git`. Discovery
  and upload-pack RPCs delegate to the installed stock `git` binary, support
  protocol v0 and v2, and advertise unborn `HEAD` so empty repositories retain
  their `main` default branch. Stock Git clones reproduce all advertised,
  reachable objects and check out populated repositories on `main`; empty
  clones retain an unborn local `main`. Existing clones can negotiate and
  fetch later primary-branch objects, update `origin/main`, and fast-forward
  their checkout with a stock `git pull`. `$GIT_STORAGE_ROOT` selects the
  repository root used by the API process and defaults to `repositories`.
  Smart HTTP receive-pack accepts initial, fast-forward, explicitly forced,
  and deletion pushes for every branch under `refs/heads/`; tags and other ref
  namespaces remain denied. Ordinary non-fast-forward pushes are rejected by
  stock clients unless force is requested, and receive-pack applies accepted
  updates transactionally. Stock clients discover and fetch every branch, and
  candidate-branch updates leave `main` unchanged. The end-to-end compatibility
  suite also covers the primary-branch workflow in sequence: initial push,
  clone, ordinary push and pull, forced replacement, deletion, empty clone,
  recovery push, and recovery pull.
  Durable human identities live in `apps/api/users` as atomic JSON records.
  Opaque 128-bit lowercase IDs are permanent attribution keys; handles are
  globally unique and, along with display names, are editable profile data.
  `POST /users`, `GET /users/{id}`, and `PATCH /users/{id}` expose the account
  lifecycle; authenticated consumers resolve their own identity with `GET
  /user`. `$USER_STORAGE_ROOT` selects their directory and defaults to
  `users`. Mutations take a root-wide advisory lock, so handle uniqueness and
  sparse profile patches remain atomic across API processes sharing the root.
  Account creation returns a short-lived session credential and sets the same
  secret as an HttpOnly, Secure cookie. Profile mutations require that actor's
  `profile:write` scope; identity inspection remains public. Durable credential
  records live beneath `$AUTH_STORAGE_ROOT` (default `credentials`) and contain
  only SHA-256 token hashes. Session credentials can create, list, and revoke
  scoped API and Git credentials through `/auth/credentials`; stock Git sends
  its opaque token as the HTTP Basic password. Git transport requires
  `git:read` for upload-pack and `git:write` for receive-pack. Maximum lifetimes
  are 24 hours for sessions, 90 days for API tokens, and 30 days for Git tokens.
  Account bootstrap publishes its user record only after the initial session
  is durable, so a credential failure never reserves the handle; logout only
  reports success after revocation is durable.
  Both stores reconcile exact records after uncertain post-rename failures,
  and a definitively failed user publication revokes its prepared session.
  Repository lifecycle routes are `POST /repositories`, `GET /repositories`,
  `GET /repositories/{id}`, `PATCH /repositories/{id}`, and `DELETE
  /repositories/{id}`.
  Read-only browser routes beneath `/repositories/{id}` expose branches,
  paginated revision ancestry, direct tree entries, and bounded text/binary
  blob inspection.
  Commit pages inspect at most 200 ancestry nodes per request, and blob previews
  verify content in a stream while retaining at most 512 KiB.
  They accept a branch or full commit ID through `ref`, return the resolved
  commit with content, and apply the same public/private collaborator policy
  as repository inspection.
  Repository records have an opaque stable ID, immutable owner, user-facing
  name, `main` default branch, and `/git/<id>.git` remote path. Names are unique
  per owner (case-insensitively), while IDs are shared by application records
  and bare Git storage. `$REPOSITORY_STORAGE_ROOT` selects the private metadata
  root and defaults to `repository-records`. Session and API credentials use
  `repositories:read` and `repositories:write`. Repositories are private by
  default; public reads are anonymous, while private reads, pushes, and
  administration require matching API or Git scope plus repository access.
  Unauthenticated or ungranted non-owners receive not-found where repository
  visibility must be hidden. The repository catalog includes both owned
  repositories and repositories available through current collaborator
  grants. Catalog and credential collection routes use `limit`/`after` cursor pagination (30
  by default, at most 100) and return `next_cursor`. The supported JSON contract
  and stable error envelope are documented in `docs/API.md`. Repository reads
  reconcile metadata with Git storage: records whose Git ID has already
  been detached by deletion are not active, even if metadata cleanup must be
  retried. A Git cleanup error preserves the ownership record so the owner can
  retry deletion; metadata is removed only after Git cleanup succeeds.
  Owners manage durable `contributor` grants through
  `/repositories/{id}/collaborators`. Contributors can inspect and fetch a
  private repository and use Git receive-pack for branches other than `main`;
  default-branch writes, visibility, access management, and deletion remain
  owner-only. Revocation applies on the next request.
  Any authenticated user can fork a repository they can read through
  `/repositories/{id}/forks`; the private result is independently owned and
  retains immutable `upstream_repository_id` lineage without granting source
  authority. Fork storage transfers only published reachable Git objects and
  keeps references independent. Private-source authorization, cloning, and
  fork metadata publication hold the catalog's cross-process lock so
  collaborator revocation cannot commit mid-creation. Fork owners synchronize one selected,
  same-named upstream branch through `/repositories/{id}/synchronizations`;
  only fast-forwards are allowed, exact upstream objects are imported before a
  compare-and-swap reference update, and divergence or concurrent pushes are
  preserved as conflicts. Private-upstream authorization, object import, and
  fork reference publication hold the catalog's cross-process lock so access
  revocation cannot commit mid-synchronization. Repository detail exposes fork creation, lineage,
  and selected-branch synchronization.
  Durable repository proposals live beneath `$PROPOSAL_STORAGE_ROOT` (default
  `proposals`). Owners and contributors can create and discuss proposals;
  authors can edit and close their own proposals, and owners can close any.
  Proposal and immutable comment records retain stable author user IDs. Reads
  inherit repository visibility and private collaborator access, while public
  readability does not grant participation. Proposal and comment collections
  use the shared cursor pagination contract.
  After a proposal mutation's atomic rename, a parent-directory sync failure
  returns the resource with `202` and `Vivarium-Durability: uncertain`; clients
  retain its stable ID and inspect later instead of issuing a duplicate retry.
  Durable pull requests beneath `$PULL_REQUEST_STORAGE_ROOT` (default
  `pull-requests`) are partitioned by repository ID so damaged metadata cannot
  affect another repository's collection. They connect an existing source branch to a distinct target
  branch, or an actor-owned direct fork source branch to its readable upstream.
  Creation records both repository identities, imports the exact fork revision
  without publishing a target ref, snapshots both verified commit IDs, attributes the request
  to its actor, records title/body purpose and optional same-repository
  proposal linkage, and starts it with `open` status. Owners and contributors
  can create them; reads inherit repository visibility and access. Pull request
  collections use shared cursor pagination and creation uses the same uncertain-
  durability response contract as proposals.
  Fork-source synchronization holds the catalog's cross-process lock while it
  rechecks current target access, imports a newer revision, and publishes the
  pull snapshot, so revocation cannot commit mid-adoption. If the source fork is deleted, synchronization stops but
  its verified imported snapshot remains reviewable and mergeable with source
  branch state reported as `unavailable`.
  Pull request inspection derives source-only commits and path-ordered file
  changes from the fixed target snapshot and explicitly recorded source
  revision rather than silently following live branches. Authors adopt a
  revised source-branch tip through the public synchronize endpoint; existing
  reviews remain tied to their evaluated commit and require a fresh decision.
  Candidate commits opt into automatic verification through versioned
  `.vivarium/checks.json` definitions. Pull creation and source synchronization
  create durable exact-commit runs beneath `$CHECK_RUN_STORAGE_ROOT` (default
  `check-runs`); commands execute in capability-free, network-disabled OCI
  containers using preinstalled images, disposable exported snapshots, and no
  Git credential. Snapshots are read-only; `$VIVARIUM_OUTPUT` is a bounded
  writable tmpfs. Startup, a periodic scheduler, and later same-commit triggers
  retry execution or container cleanup under a cross-process execution lock;
  cleanup must be confirmed before a run becomes terminal. The visibility-aware run
  collection is `GET /repositories/{id}/pulls/{pull_id}/checks`. Stable run
  detail retains numbered execution attempts; its sequence-ordered events API
  exposes lifecycle, bounded stdout/stderr logs, command outcomes, and artifact
  publication with `after` reconnection. Artifact bytes remain downloadable by
  stable ID and carry path, size, SHA-256, media type, attempt, and timing
  metadata. Recovery records an interrupted attempt before relaunching so a
  later success never erases prior failure evidence.
  Pull request detail renders live and historical attempts, logs, and
  authenticated artifact downloads in the shared review surface. Current
  owners and contributors can cancel active checks or rerun terminal checks;
  durable control events and collaborator-requested attempts retain stable
  actor IDs without replacing earlier evidence. Cancellation publishes a
  durable intent before interrupting the executor, and recovery honors that
  intent even if the command first reports failure. Run records are the source
  of truth for control attribution; event reads repair a missing projection
  under a cross-process projection lock. Cancellation intent remains until
  terminal-state directory durability is confirmed. If an executor completes
  while cancellation waits for its lock, the confirmed terminal result wins
  and cancellation must not contradict its attempt or evidence.
  Immutable pull request comments are attributable by stable user ID,
  readable under repository visibility rules, and writable by current owners,
  contributors, and a cross-repository author while the upstream remains
  public; comment publication uses the uncertain-durability contract.
  Each owner or contributor can maintain one pull request review decision;
  approvals and change requests snapshot the live source-branch commit,
  replacements retain the review ID, withdrawals remain explicit, and reads
  derive staleness against the current source tip. A deleted or non-commit
  source leaves durable reviews readable and marks them stale. Review
  publication uses the uncertain-durability contract.
  `GET /repositories/{id}/pulls/{pull_id}/merge-readiness` recomputes a
  read-only merge report for current participants. It requires one fresh
  approval and no fresh change request, reports live source/target branch
  state, already-merged state and Git merge conflicts, and separates global
  `mergeable` from owner-only, caller-specific `can_merge`. Conflict
  calculation redirects generated Git objects to temporary storage and must
  not mutate repository objects or references.
  Owners apply ready pull requests with `POST
  /repositories/{id}/pulls/{pull_id}/merge`. Readiness is revalidated, the
  target advances with compare-and-swap protection, and a two-parent commit
  records stable pull-request, proposal, author, and merger attribution. The
  request becomes `merged` with its commit, actor, and timestamp; a linked open
  proposal closes without losing its discussion. Retries reconcile an
  exact commit from private durable server merge intent when it is already
  present in later target ancestry, repairing metadata after publication
  failures without trusting forgeable Git trailers as authorization.
  Meaningful collaboration mutations also append immutable activity events
  beneath `$ACTIVITY_STORAGE_ROOT` (default `activity-records`). Events snapshot the
  repository name and affected resource title while retaining stable actor,
  target-user, repository, and resource IDs. `GET /activity` is authenticated,
  cursor-paginated, and filters every event by current repository access.
  Agent-native change sessions live beneath `$CHANGE_SESSION_STORAGE_ROOT`
  (default `change-sessions`), partitioned by repository and pull request.
  Current owners and contributors can open one only on an open pull request;
  creation snapshots its recorded source revision and atomically publishes an
  attributable `session.opened` timeline event. Participant-only collection,
  detail, and event endpoints are the durable reconnection boundary for future
  runs and must not expose worker internals. Detail inspection retries a
  failed creation-time directory sync: only a successful retry confirms
  durability, while another failure repeats the stable `202` resource response.
  Event reads must pass through the same reconciliation before exposing a
  timeline, including for direct API consumers.
  Confirmed sessions accept durable agent run mandates containing bounded
  instructions, the exact session revision, existing repository context paths,
  and the explicit pull-request source branch. Launch issues a one-time Git
  credential with only read/write scopes, restricted to that repository and
  branch and expiring within 24 hours; durable runs retain only its ID and
  expiry. Launches append attributable `run.launched` events, collection reads
  remain participant-only, and any current participant with repository write
  access can revoke the credential while preserving the mandate.
  Active run credentials publish status, agent messages, tool actions,
  artifacts, failures, and source-branch updates through the run event
  endpoint. The store derives immutable initiating-user, generated-agent, run,
  and exact session-revision attribution rather than trusting event bodies;
  collaborators read the same ordered durable events from the session timeline.
  Branch-update publication holds the stock-Git-compatible per-reference lock
  across tip validation and durable event append, preventing a concurrent push
  from making an accepted timeline event stale.
  Current participants control runs through durable guidance, question-answer,
  pause, resume, and cancel interventions. Credential-bound agents read the
  authoritative run state and ordered interventions from the control endpoint;
  paused runs cannot append progress, and cancellation is terminal and revokes
  their bounded Git credential. Every intervention shares the attributable
  session timeline.
  Active agents complete work only after pushing new descendant commits to the
  bounded source branch. Completion verifies the live tip under its reference
  lock, derives exact commits and changed files, stores structured summary,
  checks, and unresolved concerns, appends `run.completed`, and synchronizes
  the pull request to that revision while revoking further run access. Existing reviews then become stale and
  merge readiness applies without agent-specific exceptions.
  Failed checks on the pull request's currently adopted revision can seed a
  repair change session with an immutable snapshot of the definition, logs,
  command outcomes, and artifact identities. The bounded run control response
  carries that evidence and permits credential-scoped downloads of only those
  artifacts; completion uses ordinary pull synchronization so the repaired
  revision starts its checks automatically. Repair-session publication holds
  the pull-request mutation lock across its final source-revision comparison
  and durable session write, preventing concurrent synchronization from
  producing an obsolete workspace.
  Bounded receive-pack also checks durable run state, independently denying
  terminal credentials if auth revocation fails. Completion persists before
  pull synchronization so validation/storage failures cannot move the request;
  a synchronization failure retains retryable durable completion intent.
  Pull synchronization checks open status and merge intent under its lock
  before invoking completion, preventing blocked or concurrent merges from
  terminalizing a run whose revision cannot enter review.
- **Docs** — `docs/README.md` records decisions once they're made, not before.
  Update it when you change how the apps fit together, not for every change.

## LADDER.md

`/LADDER.md` at the repo root is **not part of this repository**. It is placed
there from outside the checkout — as a symlink or a read-only bind mount — and
is gitignored for that reason. Read it for context on what is being built, but
never edit it, `git add` it, or delete it, and do not treat its presence as
uncommitted work.
