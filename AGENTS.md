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
  workflows. Organizations at `/organizations` retain owner/member identity,
  explicit invitations, and acceptance-gated repository stewardship. A
  repository keeps its user control custodian and stable catalog/Git identity
  while `organization_id` associates it with the group portfolio; organization
  membership is projected as distinguishable collaborator access so removing a
  member does not erase an independent grant that predates the transfer.
  `$ORGANIZATION_STORAGE_ROOT` defaults to `organizations`.
  Portfolio initiatives retain an existing proposal, evolution, incident, or
  authorized security source plus ordered cross-repository contributions,
  dependencies, and accountable team/human/approved-agent ownership. Portfolio
  reads derive blockers, policy exceptions, release candidates, and actionable
  reassignment when live membership, agent operation, or stewardship changes.
  Nested teams retain
  version-guarded member/maintainer roles and repository responsibility, while
  effective membership explains direct or visible-child inheritance. Approved
  agent identities expose capabilities, current member operators, visibility,
  and team associations but grant no authority by themselves. Owners approve
  explicit team/agent portfolio grants with viewer, contributor, maintainer, or
  operator roles across named repository, package, environment, and
  collaboration resources. Grants retain reasons, expiry, deny exceptions,
  requests, decisions, derived credential IDs, and revocation audit. Approved
  agent operators can mint only exact repository-bound Git credentials whose
  lifetime cannot outlast the live grant; grant revocation immediately revokes
  those credentials without touching unrelated credentials. Guided contribution
  checkpoints preflight frozen pathway acknowledgement, opportunity revision,
  setup evidence, project files, and explicitly confirmed criteria before
  committing to the contributor-owned fork and opening an ordinary upstream
  pull. Pull provenance retains mentor guidance, agent assistance, criteria,
  and contributors while coaching needs remain distinguishable from blocking
  project requirements; no upstream governance or permission is bypassed.
  Organization
  owners also publish versioned proactive stewardship mandates with desired
  outcomes, repository/branch scope, trusted signals, exclusions, bounded
  budget and schedule, approved agent, allowed actions, and required human
  decisions. Each revision requires current operator acceptance and supports
  pause, expiry, and revocation; its access preview reports only independent
  live grants and effective policy, because a mandate grants no implicit Git,
  review, merge, credential, deployment, or repository authority. Mandate
  activation requests an evidence evaluation, and trusted producers publish
  bounded evaluations after relevant repository, dependency, check, release,
  incident, security, or usage changes. Stable deduplication creates a ranked
  opportunity queue with severity, value, confidence, affected owners and
  revisions, scope rationale, and citations; newer evidence retains superseded
  citations as stale. Current collaborators discuss, rank, dismiss, snooze,
  reopen, or mark findings incorrect through compare-and-swap public surfaces.
  Mandate revisions classify opportunity evidence as maintainer-approval
  required or bounded auto-start eligible, with unlisted classes failing safe
  to approval. Promotion freezes the current default-branch revision and
  creates an ordinary linked proposal with ordered human- or approved-agent
  tasks carrying completion criteria, risk, and verification plans; it does not
  itself start compute or create a branch. Active incidents, embargoed evidence,
  conflicting work, exhausted budgets, moved bases, changed mandate policy, and
  concurrent decisions remain explicit attributed blockers or conflicts, while
  promoted assignments use the existing activity and inbox projections.
  Promoted steward agent work revalidates the live accepted mandate, operator,
  approved agent, opportunity link, and exact base before task-session launch.
  Its ordinary pull freezes server-derived changes with commands, checks,
  residual risks, criterion status, citations, and agent/initiator authorship;
  no review, queue, fork, owner-acknowledgement, or merge rule is bypassed.
  Stewardship reports retain opportunity dispositions, recommendation
  decisions, implementation, verification, release, resource, false-positive,
  and goal outcomes. Maintainers may narrow or reorder already-authorized
  evidence through versioned tuning; scope, authority, agent, action, and budget
  changes still require a freshly accepted mandate revision. Repeated failures,
  inactivity, revoked repository access, anomalous consumption, and budget
  overruns pause affected automation with actionable retained notices. Outcome
  writes use a stable idempotency key so delivery retries cannot double-charge
  usage or duplicate safety decisions, and access revocation pauses only after
  the final applicable live agent grant is gone.
  The connected stewardship browser journey carries two newly evaluated
  findings through maintainer discussion, dismissal and approval, bounded
  agent guidance and implementation, ordinary checks/review/merge, release
  outcome accounting, and final mandate revocation. Promoted opportunities use
  the durable `promoted` status required by task-session authority validation;
  empty tuning and reasoning acknowledgement projections remain valid UI state.
  Reevaluation invalidates prior approval through a distinct evaluation
  version, and promotion requires readable incident and proposal conflict
  stores before its durable reservation. Unlinked retries revalidate current
  external blockers and organization governance without double-charging their
  reservation, and final linking rejects a paused or changed mandate.
  Organization
  owners define versioned draft policies across visibility, reviews, checks,
  integration, release provenance, dependency use, promotion,
  and agent authority, targeted to the organization, a team, or a repository.
  Repository previews merge the strictest matching rules before activation;
  activation governs new decisions without rewriting active work. Responsible
  team maintainers request attributable expiring exceptions, whose approved
  projection retains both the baseline and adjusted effective value. Public
  directory reads omit organization-only structure, private-repository
  responsibility,
  hidden parent links, and audit events; pending invitees receive only their
  invitation. Responsibility publication holds the repository catalog boundary
  through its organization write, and directory reads revalidate current
  portfolio membership. Members receive the attributable event history. Proposal discovery
  at `/proposals` aggregates the authenticated
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
  Repository owners publish a successful current-attempt release artifact as an
  immutable package through `POST /repositories/{id}/releases/{release-id}/packages`.
  Package identities are globally bound to their first source repository;
  versions atomically retain the exact release, commit, build, artifact checksum,
  publisher, platform, dependencies, visibility, and active lifecycle beneath
  `$PACKAGE_STORAGE_ROOT`. Publication holds the selected check run's execution
  boundary through artifact verification and the package rename, so a rerun
  cannot supersede its attested attempt mid-publication. Public package metadata and bytes live at
  `/packages/{name}/versions/{version}`, while private versions inherit current
  source-repository read access. Pre-rename failures expose no package version
  and do not reserve a new identity; post-rename parent-directory failures are
  returned as `202` durability uncertainty with the complete package identity.
  An exact retry returns that same record, while different content at an existing
  name/version remains a conflict.
  The `/packages` catalog searches visible names, summaries, and documentation;
  identity histories and compatibility resolution expose platform selectors,
  semantic constraints, exact provenance, and retained deprecated or yanked
  warnings. Yanked versions remain inspectable but are excluded from new
  resolution. Current consuming-repository participants mint short-lived
  `packages:read` credentials frozen to that repository and an explicit set of
  currently authorized package names, allowing standard bearer-token clients
  and isolated builds to fetch only those dependencies without Git, mutation,
  publisher, or unrelated-private-package access.
  Verified commits define exact package use in `.vivarium/packages.json` through
  declared direct constraints and a resolved lock. Attributable immutable
  dependency inventories derive transitive paths from published package
  metadata and project exact source into release builds, artifacts, and
  deployments; only the newest successful promotion per environment is current,
  while superseded successes remain historical evidence. Repository and exact-version consumer reads retain stale,
  unresolved, license, support, and provenance gaps while filtering every
  package and consumer repository through current visibility.
  Consumer owners define per-package patch, minor, or major update policies on
  the dependency workspace. Scans use the current recorded default-branch lock,
  select only visible active releases, and open attributable ordinary proposals
  with one executable task plus the proposed manifest/lock, release notes,
  successful build attestation, and affected dependency paths. Update records
  recheck target-package visibility on every read and serialize proposal/task
  publication under a durable exact base/from/to reservation created before any
  collaboration work. Failed cleanup retains a recovery-pending reservation so
  exact retries cannot duplicate orphaned proposals or tasks. Published package
  recovery is append-only: owners can deprecate, quarantine, yank, or restore a
  version without replacing its artifact or provenance. Non-active decisions
  retain a reason, warning, actor, time, and optional active safe replacement;
  exact consumer owners receive targeted activity and inbox notification.
  Fresh resolution and repository-scoped installs select only active versions,
  while promotions fail closed unless the package store, exact inventory, and
  active metadata for every dependency are readable, retaining existing
  deployment exposure. Lifecycle notification delivery is a required
  post-commit outcome: failures return `202` with
  `Vivarium-Recovery-Notifications: pending`, and an exact retry reuses the
  durable decision ID to complete idempotent owner notification. Consumer
  owners open urgent human- or scoped-agent-owned
  replacement proposals at their exact default-branch revision, without
  granting the publisher any consumer authority. Subsequent human or agent work
  publishes through existing task sessions, pulls, checks, review, queues,
  releases, and deployments without granting consumer authority to publishers.
  Private-package notes and build details remain only on update records whose
  reads revalidate package access; ordinary consumer proposals contain redacted
  adoption context so consumer membership cannot disclose publisher evidence.
  The connected package browser journey proves independently owned publisher
  and consumer repositories can move from web publication and a repository-
  scoped standard-client install through agent-authored, independently reviewed
  updates, exact inventories, releases, deployments, quarantine enforcement,
  and a reviewed safe replacement. Playwright isolates package records beneath
  `$PACKAGE_STORAGE_ROOT` with its other temporary API stores.
  The connected organization browser journey assembles a two-team portfolio,
  onboards a developer and approved agent, grants exact repository-bound agent
  Git access, activates shared policy plus an expiring exception, and carries
  human- and agent-authored pulls through review, release, package inventory,
  and deployment. It deliberately removes the developer after publication to
  prove authority and derived credentials are revoked while pulls, attribution,
  initiative accountability, policy evidence, and delivery remain intact.
  Playwright isolates organization records beneath
  `$ORGANIZATION_STORAGE_ROOT` with its other temporary API stores.
  The connected technical-decision browser journey carries a shared repository
  question through affected-owner invitation, scoped agent research, retained
  dissent, separately reproduced exact-revision prototypes, approval, ordinary
  human- and agent-authored task pulls, checks, review, merge, and release. A
  linked failed success measure reopens the commitment without discarding its
  alternatives, experiments, approvals, delivery identities, permissions, or
  attribution. Playwright isolates these records beneath
  `$DECISION_STORAGE_ROOT` with its other temporary API stores.
  Reproducible development workspaces at `/workspaces` launch only for current
  repository participants and freeze an exact commit plus the versioned
  `.vivarium/workspace.json` definition. The definition declares the container
  image, tools, dependencies, setup commands, and bounded CPU, memory, storage,
  and setup time. Repository, proposal-task, pull-request, and incident-repair
  sources remain attached to the durable workspace with creator, effective
  access, setup evidence, and lifecycle events beneath `$WORKSPACE_STORAGE_ROOT`
  (default `workspaces`). Exact source and setup live in one named,
  capability-dropped container without network or credentials; `/workspace` is
  a size-limited tmpfs instead of an unbounded host bind, `/tmp` receives a
  reserved bounded share of the same declared storage budget, and the remaining
  container root is read-only. Images declaring Docker volumes are rejected
  before container creation. Setup failure or timeout force-removes that
  workload and any attached volumes. Suspend/resume uses the definition SHA-256 as a
  compare-and-swap foundation and never resolves a moving branch or reruns setup.
  Repository and organization owners additionally define versioned workspace
  policies for resource ceilings, network isolation, idle/runtime/retention,
  sharing, and approved-agent execution. Organization limits constrain local
  repository policy. Launch snapshots the effective policy; later changes mark
  active work rebuild-required. Owners inspect attributed elapsed/reserved
  consumption, announce expiry for checkpoint export, and stop or expire only
  compute and control authority while checkpoint, provenance, commit, and pull
  evidence remains retained. Startup and periodic lifecycle recovery enforce
  runtime and idle deadlines; compute teardown succeeds before terminal state
  is recorded, so failed removal remains live and retryable. Teardown and
  terminal publication hold the same per-workspace lifecycle admission lock as
  suspend/resume, preventing a successful resume from racing removed compute.
  Running workspace automation reuses that container for bounded file browsing,
  compare-and-swap editing, literal search, attributed command outcomes,
  loopback port discovery, and authenticated sandboxed previews. Durable change
  evidence retains file hashes rather than contents; no platform-managed secret
  or credential is injected into commands, snapshots, logs, or previews.
  Current participants share renewable workspace/file/terminal/command/preview
  presence, discussion, and typed observation, instruction, authorship, and
  execution history. A compare-and-swap versioned, expiring control lease names
  one current human or organization-approved agent plus exact file, command,
  and lifecycle scopes. Runtime mutations enforce the live human lease; agent
  selection grants no caller authority. Concurrent file saves still use their
  content digest, stale takeovers conflict, and durable command outcomes retain
  only a command SHA-256 rather than private terminal input.
  Empty-principal observe control lets only the current live human holder
  explicitly release a lease. Per-workspace
  control serialization covers final live-lease validation through mutation
  execution, so takeover waits for admitted work and stale actors fail before
  execution without blocking unrelated presence or discussion writes.
  Attributed workspace checkpoints retain only repository changes against the
  exact base plus the frozen environment definition and declared
  reproducibility metadata beneath `$WORKSPACE_STORAGE_ROOT/checkpoints`.
  Participant reads expose file operations, hashes, modes, sizes, and parent lineage but
  not stored file bytes; credential-like content, package-manager authentication
  files/directives, and unrelated runtime evidence fail closed. Restore requires a divergence/conflict/dependency preflight
  token and live file control, revalidates the token inside control admission,
  and moves the checkpoint head so later checkpoints explicitly branch from the
  restored record. Runtime capture through checkpoint publication shares that
  admission lock with file mutations. Restore stages and backs up every target;
  failures restore files and remove transaction-created parent directories.
  Authorized checkpoint publication creates one commit solely from the stored
  inspected manifest, compare-and-swap advances an existing base-matching
  branch or creates a new one, and can open an ordinary governed pull. Pulls
  and checkpoints retain bidirectional workspace, validated task/session,
  contributor, file-hash, and command-digest provenance frozen at checkpoint
  capture from a private append-only evidence ledger rather than bounded UI
  histories. A cross-process claim excludes duplicate publication effects,
  failed pull creation compensates its ref only by compare-and-swap, and
  post-pull link failure retains retryable publication intent. A publication
  outbox precedes Git mutation so final checkpoint-write failures reconcile the
  existing pull, and legacy histories seed the evidence ledger before their
  first post-upgrade event; normal checks, stale-review,
  protection, and queue rules apply while terminal input, outputs, credentials,
  discussion, and unpublished runtime files stay out of Git and review text.
  The connected workspace browser journey proves a proposal-task room can join
  a peer and approved agent through editing, execution, intervention,
  suspend/reconnect, explicit conflicting-checkpoint restoration, governed pull
  merge, and compute expiry without losing retained attribution. Workspace
  runtime automation must remain compatible with declared minimal images (the
  journey uses Alpine); capture reads the live tmpfs from inside the container,
  not the image layer exposed by `docker cp`.
  Living documentation at `/repositories/{id}/documentation` retains
  versioned owner-published collections backed by exact repository paths and
  commits. Collections freeze page blob/hash/authorship evidence, supported
  source or release mappings, audience, navigation, rendering, publication
  policy, and links to project resources; reads explicitly project missing
  owners, broken or changed source, and stale version mappings. Records default
  beneath `$DOCUMENTATION_STORAGE_ROOT` (`documentation`).
  Documentation tasks freeze a proposal, issue, pull, release, investigation,
  or stewardship opportunity to an exact revision and reserve a
  `docs/tasks/*` branch. They retain rendered draft revisions, grounded
  references, attributable discussion and suggestions, and identified agent
  assistance with explicit sources and uncertainty; tasks grant no additional
  Git, workspace, review, or publication authority.
  Repository `.vivarium/documentation-checks.json` definitions expand selected
  link, symbol, build, sample, command, and tutorial verification across exact
  source/package/release revision matrices in the ordinary bounded check
  executor. Pull checks retain logs, artifacts, coverage/output-difference
  files, selectors, targets, and dependency digests; generated `docs/*` check
  names can be required through normal branch merge readiness, while changed
  declared paths invalidate only the checks whose evidence names them.
  Exact-revision code navigation at `/repositories/{id}/code` and
  `GET /repositories/{id}/code-navigation` performs bounded lexical search
  across supported source files, classifies definitions, references, callers,
  and tests, and attaches per-line commit evidence. Responses resolve a branch
  to one immutable commit, expose incomplete-analysis limits, project catalog
  ownership, and include only readable dependency declarations recorded for
  that revision.
  Collaborative investigations at `/repositories/{id}/explanations` freeze repository,
  file, proposal, task, pull, incident, or workspace context to one exact
  revision and stream only a fully retained attributed conversation. Claims
  distinguish evidence, inference, and uncertainty and cite exact source lines
  or immutable check/dependency resources; bounded gaps remain explicit and
  every durable read revalidates repository access. Private workspace context
  retains its sharing boundary. Explicitly invited current participants share
  an ordered canvas of references, queries, bounded workspace observations,
  hypotheses, agent findings, challenges, supersessions, and conclusions.
  Reruns append findings at a new immutable revision and mark older citations
  stale without rewriting history; workspace attachments retain identifiers
  and revisions, never credentials or copied private output.
  `$EXPLANATION_STORAGE_ROOT` defaults to `explanations`.
  Prospective impact assessments at `/repositories/{id}/impact` freeze selected
  code, an investigation conclusion, or a proposed diff to one exact revision.
  Bounded analysis joins lexical references and tests with currently visible
  owners, packages, interfaces, consumers, releases, and environments; every
  read filters retained cross-repository evidence through current access.
  Invited participants refine the version-guarded record with accepted risks,
  unknowns, and verification needs, while affected repository owners can
  acknowledge explicit requests only when retained consumer evidence names
  their repository and they can currently read the source assessment.
  `$IMPACT_STORAGE_ROOT` defaults to `impact-assessments`.
  Assessment participants convert selected retained items into one atomic
  proposal and ordered human- or generated-agent-owned task plan. Proposal,
  task, workspace, change-session, and pull projections retain the assessment
  version, exact revision, optional investigation conclusion, selected
  claims/risks/verification needs, and owner acknowledgements. Selected-ref
  movement marks the assessment context changed and blocks new implementation;
  retained work is never rewritten. Implementation publication holds the
  selected Git reference lock through proposal/task creation and assessment
  linking. Post-persist durability uncertainty returns stable identities in a
  recoverable `202` response rather than being misreported as validation
  failure. A successful link is confirmed by rereading the assessment. An exact
  pre-link-version replay returns the existing work only when proposal text,
  selected items, ordered tasks, ownership, and dependencies match; changed
  stale payloads remain invalid. An omitted generated-agent ID permits server
  allocation, while any explicitly supplied recovery ID must match exactly.
  The connected code-intelligence browser journey carries an unfamiliar
  developer from exact symbol navigation through a grounded shared
  investigation, affected-consumer owner acknowledgement, agent-owned task,
  repository check, independent review, and merge. Playwright isolates
  explanation and impact records beneath its temporary API stores alongside
  the other journey state.
  Repository relationship graphs at `/repositories/{id}/relationships` join
  immutable versioned interface publications to exact consumer revisions,
  optional releases and environments, repository owners, and semantic-version
  constraints. Reads revalidate visible release and successful-deployment
  evidence and explicitly classify edges as resolved, stale, or unresolved;
  durable records live beneath `$RELATIONSHIP_STORAGE_ROOT`.
  Interface evolution plans in that workspace bind an open provider proposal
  or pull to an exact published predecessor, classified compatibility changes,
  and a frozen visibility-filtered consumer/owner impact snapshot. Collaborators
  retain compare-and-swap strategy, sequencing, and exceptions; current
  consumer participants acknowledge their own impacts. Short-lived
  `evolutions:analyze` credentials expose only selected readable repositories
  from that snapshot and can append attributable findings and uncertainty,
  never repository or Git mutation authority. Their read and finding responses
  retain contract candidates only when every frozen pull target and source is
  selected and remains readable by the initiator.
  Exact open provider and affected-consumer pull revisions can be assembled as
  immutable evolution contract candidates. Provider-owned
  `.vivarium/contracts.json` checks run against `provider/` and
  `consumers/<repository-id>/` in the network-disabled, read-only executor with
  no platform credential. Plans retain combination hashes, attempts, logs,
  artifacts, attestations, failures, and supersession; revision changes
  supersede only matrix rows containing that repository. Candidate projection
  and evidence reads require current access to every frozen pull target and
  source repository, preventing target access from exposing private-fork work.
  Candidate creation holds the catalog mutation lock across its final all-source
  revalidation, check/candidate publication, and response, excluding concurrent
  collaborator removal or visibility changes from that publication boundary.
  Governed evolution rollouts freeze one current contract candidate and ordered
  repository/environment phases only after every frozen compatibility check
  succeeds. Each repository explicitly selects one migration task for rollout
  projection. Current repository owners approve only their
  own participation; reads derive gate and phase readiness from exact contract
  checks, ordinary merged task pulls, ancestry-containing releases, and governed
  promotions. Closed pulls or failed/canceled promotions pause the affected
  phase and retain prior outcomes while directing recovery through established
  rollback or agent-repair controls. A store-assigned durable creation sequence
  selects the newest matching promotion as the authoritative retry outcome even
  when timestamps collide; historical failures remain evidence without
  overriding later success, and unavailable contract-check storage fails
  rollout configuration closed.
  A dedicated connected browser journey proves the full evolution loop across
  independently owned public provider and consumer repositories: browser
  publication/discovery, bounded agent analysis, a human-owned fork pull, a
  task-scoped agent pull, exact contract evidence, owner-scoped approvals, and
  ordered merge, release, build, and production deployment all project into one
  completed rollout. Playwright isolates these records beneath
  `$RELATIONSHIP_STORAGE_ROOT` with its other temporary API stores.
  Ordered evolution migration tasks are repository-owned proposal tasks linked
  to the shared plan with target versions and cross-repository dependencies.
  Only current target participants create and assign them; humans gain no new
  access, agents reuse isolated task sessions after dependencies merge, and
  local discussion, branch, pull, fork, and completion state stays authoritative.
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
  Failed or canceled deployments expose two participant recovery paths. A
  rollback derives the newest earlier successful artifact for that exact
  environment and submits it as another approval-gated promotion with durable
  failed/restored deployment provenance. A repair creates an isolated
  deterministic `agent/recovery/*` branch and ordinary pull request at the
  current default-branch tip, then freezes the unhealthy release commit,
  release notes, deployment logs, health evidence,
  artifact checksum, and source revision into its change session. Its agent
  credential remains repository/branch-bound; repaired code must complete the
  normal review, required-check, integration, build, release, and promotion
  flow and never receives environment authority.
  Repair retries reconnect the same branch, pull, and session after partial
  publication. An unpublished branch left before pull publication is
  fast-forwarded to the current default tip only when its prior tip remains an
  ancestor; divergence and concurrent changes fail closed. Rollback target derivation, unhealthy-state revalidation, and
  promotion publication occur under one deployment-store lock.
  Pull and session stores also enforce recovery uniqueness under their
  cross-process locks, so simultaneous commands converge on one deterministic
  branch, pull request, and deployment-evidence session.
  The connected orchestration browser journey continues its merged human/agent
  plan through a known-good release, immutable builds, independently approved
  promotion, retained rollout failure, governed rollback, evidence-pinned agent
  repair, ordinary re-review, and successful corrected delivery. Deployment
  execution verifies the private durable artifact, then bind-mounts an
  ephemeral owner-only copy read-only into containers running as the API host
  identity, so they can read the exact verified bytes without widening host
  access to the release.
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
  Incident response at `/incidents` provides a cross-repository shared operating
  picture for current affected-repository participants. Incidents may be declared
  manually or from verified retained deployment signal evidence, identify affected
  environments, severity, status, and named response roles, and retain an immutable
  actor-stamped timeline. Updates preserve `participants` or `public` audience
  intent and stable-user acknowledgements; compare-and-swap versions protect state
  and role decisions. Timeline update callers retain a stable operation ID across
  retries, and the web keeps the pending exact-draft identity in incident-scoped
  local storage across remounts, so lost responses and post-publication durability
  uncertainty cannot duplicate an update.
  Incident diagnosis adds retry-safe typed observations, hypotheses, queries,
  and conclusions to that timeline. Findings attach server-verified logs,
  health signals, deployments, releases, commits, pulls, or prior incidents
  from affected repositories; logs and signals retain bounded windows and
  selectors, while every source snapshots its label and capture time alongside
  the stable live-resource identity. Finding audience and reads inherit the
  incident's current-participant authorization boundary.
  `$INCIDENT_STORAGE_ROOT` defaults to `incidents`; incident mutations hold the
  repository catalog lock across current participant and role revalidation plus
  publication, so collaborator revocation cannot commit mid-mutation.
  Incident investigations freeze responder-selected evidence, verified commits,
  and a mandate behind an `incidents:investigate`-only API credential. Agents
  read that packet and stream attributable findings, tool actions, questions,
  and uncertainty into the timeline; responders exclusively guide, pause,
  resume, or cancel, without granting Git, deployment, environment, credential,
  secret, or repository mutation authority.
  Incident mitigations retain exact evidence, affected deployment, declared
  rollout health criteria, proposer, decisions, overrides, and every execution
  attempt. Pause, attested rollback, and emergency repair execute only through
  established deployment and change-workflow APIs; recovery requires every
  frozen criterion to pass on a governed-attempt-bound deployment in the
  affected environment. Proposal and attempt operation IDs deduplicate exact
  retries, and attempts are durably reserved before an environment mutation.
  Pause events embed the action/reservation identity, and emergency-repair
  recovery proves the governed repair pull's merge commit is ancestral to the
  exact recovery deployment commit.
  Resolving an incident publishes a versioned attributed review containing
  impact, a review timeline, contributing factors, and conclusions. Resolved
  incidents create corrective work as ordinary linked proposals with one
  human-assigned executable task, exact base, and due date. Incident reads
  derive pull, check, release, and deployment progress from authoritative
  workflow stores; obsolete or revoked task context invalidates the commitment,
  overdue and invalidated ownership remains actionable in inbox, and merge
  completion clears it. Commitment publication holds the target repository
  catalog lock across actor/assignee revalidation, atomically creates the
  proposal/task/assignment under the incident operation ID, and reuses those
  records if incident linking must be retried.
  The connected orchestration browser journey proves the complete incident loop
  from a failed production release signal through browser declaration, frozen
  evidence, bounded agent diagnosis, independently approved attested rollback,
  public recovery communication, review publication, and an incident-linked
  corrective task whose pull, checks, release, and successful follow-up
  deployment project back into the same permission-aware record.
  Private vulnerability coordination at `/security` is isolated from ordinary
  repository activity, inbox, incident, proposal, and search flows. An
  authenticated reporter can file against repositories they can currently
  read with affected versions, bounded evidence, and a safe contact channel.
  The web form accepts a catalog repository or the stable ID from a public
  repository URL, so an external reporter does not need a collaborator grant.
  Only the reporter, current owners of affected repositories, and an explicitly
  invited response team (at most 20 users) can discover the report. Owners set
  severity and embargo state with compare-and-swap versions and invite
  responders; all participants can communicate privately. The report retains
  its own immutable attributed mutation and detail-read audit. Unauthorized
  reads return not-found, no advisory operation publishes activity or inbox
  notifications, and `$SECURITY_ADVISORY_STORAGE_ROOT` defaults to
  `security-advisories` with owner-only filesystem permissions.
  Participants connect verified commits, dependencies, releases, builds,
  artifacts, and deployments; record attributed hypotheses, conclusions, and
  uncertainty; and compare-and-swap a version-line by environment impact
  matrix. Dependency evidence must match an exact release build's frozen image
  definition. Short-lived `security:investigate` credentials expose only selected
  frozen evidence and can publish findings only inside the embargoed report.
  Response-team repair tasks freeze affected version lines, verified bases,
  human or agent assignees, and same-advisory cross-repository dependencies.
  Sessions use hidden `vivarium-security/*` refs and exact-branch revocable Git
  access; assignment and launch both require current participation in that
  task's repository, while session control is limited to its worker, initiator,
  or explicit task creator while that creator retains repository access.
  Revocation removes the branch before the open task can
  restart. Protected commits, comments, and reviews remain advisory-only.
  Affected-repository owners define private reproductions per supported version
  line inside the advisory. Completed repair candidates run required-check
  definitions frozen from the trusted task base plus those embargoed
  reproductions against one exact commit; repair commits cannot substitute the
  executable check properties while retaining a required name. Task bases must
  be commits in the owner-controlled default-branch ancestry, excluding orphan
  and unmerged collaborator history. Sanitized status
  and artifact metadata remain response-visible while commands
  and logs never enter ordinary pull/check surfaces. Passing evidence requires
  independent repository-owner approval before it is integration-ready.
  Required checks and private reproductions reserve one exact run set, whose
  safe projection remains pending until every reserved run is readable.
  Fixed release attestations then prove the release ancestry contains that
  candidate and every exact release build succeeded with checksummed artifacts,
  allowing the advisory to derive coverage and remaining gaps across every claimed line.
  Maintainers prepare a redacted disclosure only after every affected version
  line has an attested release. Publication exposes deterministic repaired
  branches, exact releases and checksums, affected/fixed versions, credits, and
  upgrade guidance together; anonymous public advisory reads never include the
  protected report, evidence, messages, contact, commands, or logs. Durable
  publication steps are retry-safe and retain an exact remaining-work list.
  Affected repository owners, repository collaborators, and prior deployment
  initiators receive targeted
  activity/inbox notifications only after the public packet is ready.
  Before that durable transition, repair refs are staged only beneath the
  transport-hidden `vivarium-security/disclosures/*` namespace, so failed
  cleanup cannot reveal remediation. Repository branch collection and direct
  named-revision browser routes exclude the entire `vivarium-security/*`
  namespace as well. Public refs and idempotent notifications
  follow the public transition; their failures retain explicit public
  remaining work rather than falsely restoring the embargo.
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
  Candidate commits may also define `.vivarium/preview.json` version 1 with a
  bounded image, build, scoped environment, output, and resource contract.
  Collaborator launches retain exact revision and definition attestations,
  creator, lifecycle, setup logs, failures, and a sandboxed authenticated
  `index.html` URL beneath `$PREVIEW_STORAGE_ROOT` (default `previews`). Pull
  synchronization projects older previews as stale without replacing their
  retained URL or evidence. Definitions fail closed on network `none`,
  artifact-only data, named identity, and explicit view, test, or feedback
  actions. Repository owners may invite named users or current issue, decision,
  and proposal participants into attributable roles expiring within 30 days.
  Entry and revocation are audited; invitations confer no repository,
  credential, workspace, check-log, environment, deployment, private-service,
  or production authority.
  Feedback-role participants create pull-visible preview findings pinned to the
  preview revision and observed route. Findings retain classification,
  severity, reproduction steps, discussion, duplicate relationships, and
  resolve/reopen history. Screenshots, recordings, console output, traces, and
  annotations stay inside the preview audience boundary; allowlisted media and
  byte limits are enforced, and sensitive text fields are redacted before
  persistence rather than copied into broader pull comments. Non-repository
  feedback invitees use the exact `/pulls/{repository-id}/{pull-id}/previews/{preview-id}`
  workspace, whose controls derive from the live invitation instead of catalog
  participation.
  Repository owners define preview acceptance requirements per target branch,
  optionally selecting changed paths or owner-authored risk classes and naming
  required scenarios plus owner, contributor, author, or stakeholder roles.
  Stakeholder decisions require a live feedback invitation to a preview of the
  exact adopted revision; expiry, revocation, and source movement fail closed
  without granting repository access, and every readiness/merge/queue check
  revalidates that invitation. Final direct and queued Git publication shares
  the preview audience admission lock with invitation mutation. Attributable
  acceptance, rejection, and owner-only justified override decisions freeze the
  exact adopted revision and policy version. Synchronization or policy
  replacement retains older decisions as stale,
  while merge readiness and integration-queue admission block on missing
  current blocking scenarios, current rejection, or unresolved blocking preview
  findings alongside ordinary reviews and checks.
  Decision writes use caller idempotency keys plus synced atomic publication;
  rejection is sticky until an owner-only justified override. Integration queue
  landing revalidates the entire current readiness report and durably pauses an
  admitted entry when later policy, evidence, review, check, or branch changes
  invalidate it.
  Current write collaborators can convert a current-revision finding into a
  retry-safe ordinary pull change session with frozen acceptance criteria,
  redacted permitted evidence, discussion, reproduction, and authorship. The
  preview invitation grants no implementation authority; human work and agent
  credentials retain existing branch boundaries. Agent publication synchronizes
  normally, starts checks plus a new preview attempt, and back-links the repair
  commit and attempt to the original finding. The follow-up attempt reserves a
  stable repair/session preview identity before check creation; exact completion
  retries reconcile that reservation and its terminal or active build run. A
  shared preview-store lock holds the reservation scan/write and run attachment
  across API processes, while check-run creation has its own shared lock around
  definition-name scan/write so no losing process can strand a duplicate run.
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
  Git credential. Definitions may retain bounded `cpus`, `memory_mb`, and
  `storage_mb`; execution applies those Docker CPU/memory limits and uses
  storage as the output watcher and artifact collection ceiling. Omitted
  values preserve the 2 CPU, 1 GiB memory, and 256 MiB output defaults.
  Snapshots are read-only; `$VIVARIUM_OUTPUT` is a bounded
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
- Consequential technical choices live at `/decisions` as repository-authorized
  pending records sourced from repository, proposal, investigation, incident,
  evolution-plan, or stewardship-opportunity context. Each keeps a versioned
  question, constraints, success measures, deadline, affected resources,
  participants, accountable owner, and one attributable scope/discussion
  history beneath `$DECISION_STORAGE_ROOT` (default `decisions`). A decision is
  coordination context only: its pending state is discoverable by source and
  does not block proposals, pulls, tasks, or other contributions.
  Participants compare structured alternatives against every shared success
  measure with exact code, dependency, release, incident, and usage citations;
  reads identify missing evidence classes and evidence older than 30 days.
  Short-lived `decisions:research` credentials are bound to one repository,
  decision, and selected alternative. They grant repository read plus cited
  finding publication but no mutation, while retained positions and
  supersession preserve dissent.
  Alternatives can launch exact-revision `decision_experiment` workspaces whose
  named commands come from `.vivarium/workspace.json`. Decision experiment
  evidence references only that workspace's attributed command outcomes and
  checkpoints, adds measurements, checksummed artifact metadata, and
  server-derived resource consumption, and reports default-branch,
  environment, or policy drift. Experiment checkpoints remain exploratory;
  publishing one still requires the workspace's separate ordinary Git/pull
  workflow and is never implied by evidence attachment.
  Launch snapshots the then-current default branch and workspace-definition
  digest independently of the experiment's selected commit, so intentional
  historical pins begin valid and only later drift invalidates them. Identical
  decision/alternative/commit launches by one actor are serialized across
  processes and reuse the running workspace; linking that workspace to the
  decision is idempotent, making a failed second request safely retryable.
  The accountable owner turns that context into an immutable commitment only
  after resolving requested affected-repository-owner acknowledgements and
  active organization-policy approvals. Each published version freezes its
  selected and rejected alternatives, rationale, accepted tradeoffs, dissent,
  conditions, review date, exact cited evidence, approvals, and authorized
  expiring policy exceptions. Pending and rejected requests remain visible as
  governance conflicts. Scope, alternative, finding, or experiment changes
  reopen a published decision, supersede its approvals, and retain the prior
  commitment instead of rewriting it; discussion alone is non-material.
  Each accepted commitment can publish one retry-safe implementation handoff
  into an ordinary proposal with ordered human- and agent-owned tasks at the
  frozen default-branch revision. The server derives task outcomes and
  verification plans from explicit constraints and success measures and
  rejects incomplete coverage; proposal/task reasoning retains decision and
  commitment identity through scoped sessions, workspaces, linked pulls,
  checks, releases, and deployments without adding authority. Attributable
  delivery observations link coverage or drift to those ordinary resources.
  Observation admission verifies the retained review/integration pull, check,
  release inclusion, or deployment back to the exact proposal. Cross-store
  publication uncertainty returns stable proposal/task identities in a
  retryable `202` and exact retries reconcile the decision link.
  Coverage requires successful current evidence: an approval at the live pull
  revision, merged integration, a successful check at that revision, or a
  successful deployment. A candidate release is not terminal coverage, while
  deviations may cite linked failed/nonterminal resources as their evidence.
  Superseded task contributions remain historical evidence only: coverage uses
  the authoritative current contribution, and deployment coverage requires the
  successful release to include every current merged task pull.
  A deviation, changed assumption, failed measure, or incompatible work
  reopens the exact commitment with an actionable revisit reason, while
  confirmed coverage remains retained delivery evidence.
  Every current, non-superseded opposing finding must appear exactly once in
  the commitment. Each approved exception request can appear at most once.
  Policy exception approvals freeze the normalized reason and maximum expiry;
  publication cannot substitute another purpose or extend that authorization.
  Temporary outcome delivery teams at `/delivery-teams` retain a versioned
  operating charter sourced from a proposal, portfolio initiative, technical
  decision, incident follow-up, or explicitly named planned outcome. Repository
  writers invite existing humans and organization-approved agents into roles
  with reasons, responsibilities, budgets, deadlines, escalation paths, and
  exact repository access requirements. Invitees (or an approved agent's
  current operator) accept or decline through compare-and-swap responses;
  organizer-only charter changes and all responses remain actor-stamped.
  Material shared-charter or participant-invitation changes reset affected
  acceptance to pending; shared purpose, budget, deadline, escalation, name,
  or participant-composition changes require every retained participant to
  accept the revised operating contract again.
  Effective-access previews derive current repository participation and live
  organization grants on every read. A charter never mints a credential or
  grants repository authority, so missing access remains an explicit gap before
  execution. Records default beneath `$DELIVERY_TEAM_STORAGE_ROOT`
  (`delivery-teams`).
  Accepted members publish compare-and-swap execution plans whose ordered
  streams retain owners, exact-revision inputs and repository paths, artifacts,
  dependencies, acceptance criteria, assumptions, budgets, and integration
  order. Reads derive overlap, duplicate-artifact, budget, live-access, and
  material-replanning blockers without granting authority. Every affected
  human or approved-agent owner must accept a material plan revision; pending
  or declined boundaries remain visibly blocked.
  Stream owners attach existing change sessions, investigations, decision
  experiments, or workspaces at the plan's exact repository revision and
  publish findings, questions, checkpoints, artifacts, decisions, and residual
  uncertainty as retained cited timeline entries. Projection revalidates the
  viewer's independent access and omits inaccessible entries, contexts, and
  handoffs as a unit. A structured handoff freezes its input entry IDs and
  citations, plan revision, sender, named recipient, acceptance criteria, and
  declared uncertainty. It does not reassign the stream: the plan must name the
  recipient before they can publish their own verification entry, and only the
  named human or current agent operator can accept with that retained evidence.
  Team storage contains references and summaries only, never terminal input,
  credentials, hidden prompts, or copied evidence bodies.
  Each planned stream also retains an owner-published operational snapshot with
  state, progress, exact revision, bounded resource use, active control,
  blockers, questions, and predicted next action. Budget exhaustion and live
  access loss pause only the affected stream with explicit recovery. Accepted
  members may guide or control individual streams; only the organizer controls
  the whole effort or changes authority through reassignment/narrowing. Those
  authority-affecting interventions create a new plan revision and fresh owner
  acceptance while all accepted timeline, handoff, status, and intervention
  evidence remains retained.
  Accepted repository writers reconcile one exact contribution per current
  stream into durable integration manifests. Branch tips or already-published
  workspace checkpoints must descend from the shared base; server-derived
  paths expose overlaps, while incomplete streams, pending handoffs, plan
  blockers, and criteria without same-stream evidence block publication. Ready
  manifests open retry-safe ordinary pulls in declared order and trace team,
  stream, commits, authorship, agent actions, decisions, costs, and residual
  risk without bypassing review, checks, queues, release policy, or authority.
  Publication holds the delivery-team mutation admission boundary across its
  final live-readiness check, retry-safe pull recovery/linking, and manifest
  write. Recovery reuses only an open pull at the exact source, target, team,
  manifest, stream, and order; delivery identity is written atomically with a
  new pull. Existing links cannot be replaced, so competing status changes,
  inactive reviews, or unrelated branch-matching reviews cannot strand or
  commandeer contribution provenance. Publication starts the ordinary
  exact-revision repository checks only after every ordered pull is created or
  recovered and the complete link set is durable on the integration; a later
  pull or final manifest-write failure therefore starts no partial checks, and
  an exact retry reconciles existing pulls before execution. A published
  integration accepts its current or lost-response pre-publication version as
  an idempotent recovery request, recreating only missing check definitions and
  resuming nonterminal runs from its durable pull links. Repository,
  configuration, or check-store persistence failure returns a retryable `503`
  rather than claiming recovery completed. The connected
  delivery-team browser journey protects the complete accepted-decision loop:
  independently operated specialist agents and a developer accept parallel
  streams, retain a disputed finding and lead resolution, redirect a failed
  stream, verify an agent-to-human ownership handoff, publish ordered pulls,
  pass checks and independent review, merge, and release. Removing an agent
  operator revokes its derived Git credential while the charter, evidence,
  interventions, costs, authorship, handoff, pulls, and release stay retained.
- **Issues** — Unexpected-behavior reports at `/issues` retain structured
  expected/observed behavior, severity, environment, ordered reproduction,
  optional exact release, visibility, bounded allowlisted evidence,
  discussion, status, and immutable history. Duplicate suggestions are
  visibility-filtered and omit evidence/discussion. Records default beneath
  `$ISSUE_STORAGE_ROOT` (`issues`). Issue creation admits a 15 MiB encoded body
  for ten 1 MiB raw attachments; affected versions are server-derived from an
  exact release, entering or leaving terminal status is owner-only, and durability-uncertain visible
  writes return the retained identity as `202`. Issue reproduction launches the
  bounded credential-free workspace at exact source or the attested release;
  selected sanitized inputs are staged under `.vivarium/reproduction-inputs`,
  using attachment-ID-prefixed collision-proof filenames after conservative
  credential filename, assignment, authorization-header, and private-key screening,
  and immutable attempts retain only repository-declared command outcomes plus
  their environment, logs, artifacts, observed result, and disposition.
  Artifact capture rejects symlink components, then opens one file descriptor,
  validates that exact descriptor beneath `/workspace` through `/proc/self/fd`,
  and reads from it, so path replacement races cannot expose image or temporary-
  container files.
  Collaborative triage retains compare-and-swap classification, priority,
  current-participant assignment, exact suspected revision/owners, duplicate
  identity, and typed code/dependency/release/deployment/incident/existing-work
  links. Reporter evidence requests, cited human hypotheses/findings/uncertainty,
  supersession, and open challenges keep diagnosis attributable. Bounded
  `issues:investigate` credentials expose only one selected reproduction and
  named links; agent findings cite only that packet and use the generated agent
  identity without gaining repository write authority. Link creation resolves
  code and dependency revisions plus release, deployment, incident, proposal,
  pull, and issue identities against authoritative stores while holding current
  read authorization. Investigation packet reads and finding publication hold
  the repository catalog admission lock through their outcome, and a visible
  durability-uncertain launch retains its issued credential.
  A confirmed `reproduced` attempt plus undisputed cited findings can seed one
  retry-safe governed implementation proposal and human- or generated-agent-
  owned task at that exact revision. The issue freezes acceptance criteria and
  proposal/task identities, then projects the task's ordinary linked pull
  status; human assignment grants nothing and agent authority remains limited
  to the existing task-session branch and credential lifecycle. Checks,
  discussion, review, queues, and merge retain the stock pull boundaries.
  If proposal publication succeeds before the issue file can be replaced, the
  endpoint returns the stable proposal/task identities as `202` with
  `Vivarium-Recovery-Implementation: pending`; an exact retry reuses that
  proposal and completes the issue link instead of duplicating repair work.
  Recovery compares the entire frozen reasoning origin (version, revision,
  selected evidence, and item snapshots); changed evidence is a `409`, never a
  relink of new issue claims to an older task.
  Exact repair pull revisions can reserve issue-specific verification that
  reruns retained clean-environment commands and checksummed reporter inputs
  beside required checks frozen from the affected base. Pull movement makes
  proof and confirmation stale. Reporter confirmation/rejection and owner-only
  reasoned overrides append without rewriting dissent.
  A running pull-linked workspace at the same candidate revision is the only
  optional safe-preview handoff and retains normal workspace sharing controls.
  Verification input staging walks held no-follow directory descriptors so a
  candidate symlink cannot redirect API writes. A deterministic evidence key
  is durably reserved on the issue before check creation; recovery links and
  executes only that reservation's runs, preventing orphan or duplicate work.
  Delivered resolution requires current passing proof explicitly confirmed by
  the reporter, an immutable release whose commit contains that exact repair,
  and a successful promotion of the same release commit. The issue then freezes
  the release/version/commit, deployment/environment, artifact checksum,
  reporter decision, and recording actor before entering `resolved`; exact
  retries reuse the retained outcome and conflicting delivery claims fail.
  A non-reproduced attempt remains open while a maintainer requests missing
  evidence, the reporter answers beside that attempt, and collaborators retry.

- **Contributor pathways** — Repository owners publish immutable numbered
  onboarding expectations at `/repositories/{id}/contributor-pathway`; public
  repositories expose them without authentication. Live reads classify linked
  documentation, ownership, releases, issues, proposals, and workspace
  definitions as current, stale, or inaccessible without changing history.
  Authenticated readers acknowledge exact revisions; anonymous reads expose
  only a count, owners see attribution, and readers see only their own record.
  Visible writes with uncertain directory synchronization return `202` plus
  the retained identity. Records default beneath
  `$CONTRIBUTOR_PATHWAY_STORAGE_ROOT` (`contributor-pathways`), and the web
  surface is `/repositories/{id}/contribute`.
  Owners separately advertise source-grounded bounded work at
  `/repositories/{id}/contribution-opportunities`: issue, proposal, planned-task,
  and stewardship sources retain exact revision, outcome, scope, skills,
  interests, dependencies, risk, estimate, mentors, and agent-assistance
  availability. Request-scoped match profiles return explicit reasons and gaps.
  Exact-version claims are attributable, exclusive, releasable, and expire after
  one hour to 14 days; they coordinate duplicate effort but grant no repository,
  Git, workspace, task, or agent authority. Storage defaults beneath
  `$CONTRIBUTION_OPPORTUNITY_STORAGE_ROOT` (`contribution-opportunities`).
  A live claimant can launch the exact opportunity version into an independently
  owned fork and isolated shared workspace. It snapshots the current pathway,
  source evidence, criteria, prerequisites, and recorded revision; runs both
  `.vivarium/workspace.json` setup and pathway verification; and links to
  revision-grounded explanations. Only selected issue attachments that pass
  secret-like-content rejection may be staged. Preflight reports missing
  access, evidence, revisions, guidance, or definitions before fork creation;
  retained setup failures and obsolete-guidance diagnostics remain visible,
  and no upstream write authority is granted.
  Launched contribution workspaces retain a version-guarded help thread with
  designated mentors, attributable questions, advice, checkpoints, handoffs,
  explicit decision ownership, availability, and response expectations.
  Changed scope or revoked mentor access derives a reassignment/exit path
  without discarding progress. Approved-agent explanation, diagnosis, or edits
  require the matching live guide/edit control lease and grant no upstream
  authority.
  Owners close guided work only after its exact ordinary pull is merged and a
  release includes both pull and contributor. Completion retains credit,
  feedback, server-derived setup and mentor/agent support counts, recognized
  skills, and explicit next-opportunity readiness; exact retries are
  idempotent.

- **Docs** — `docs/README.md` records decisions once they're made, not before.
  Update it when you change how the apps fit together, not for every change.

## LADDER.md

`/LADDER.md` at the repo root is **not part of this repository**. It is placed
there from outside the checkout — as a symlink or a read-only bind mount — and
is gitignored for that reason. Read it for context on what is being built, but
never edit it, `git add` it, or delete it, and do not treat its presence as
uncommitted work.
