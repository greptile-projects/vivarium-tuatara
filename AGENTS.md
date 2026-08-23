# AGENTS.md

Guidance for coding agents working in this repository.

Regression investigations at `/repositories/{id}/regression-investigations` and the repository
`/regressions` workspace turn a suspected regression from an issue, support thread, failed check,
release, deployment, or demonstrated reproduction into a durable shared search boundary beneath
`$REGRESSION_INVESTIGATION_STORAGE_ROOT` (`regression-investigations`). Creation freezes expected and
regressed behavior, exact known-good and known-bad commits or releases, affected environments,
severity, current owners, acceptance criteria, and permitted evidence. Both commits must exist and
known-good must be an ancestor of known-bad; evidence availability is server-resolved and missing or
stale evidence remains explicit. Caller-stable request identities reconcile ambiguous post-publication
create retries before mutable source/owner/history validation and reject changed reuse. Read projection
rechecks the retained source predicate, downgrades evidence whose check or reproduction state moved, and
clears projected stale/unavailable state when that same authoritative source recovers; retained creation facts
remain immutable. CAS history
retains attributed discussion, hypotheses, scope changes, and status without granting Git, testing,
workspace, environment, deployment, evidence, or debugging authority.
Bounded regression scenarios freeze synthetic/privacy-preserving inputs, an exact preinstalled image,
revision-appropriate setup and comparison commands, resources, and criteria. Attempts resolve exact commits,
attested releases, and named dependency repository/revision pairs before digesting and materializing bounded
dependency archives at deterministic read-only workspace paths and running one to five clean, networkless
isolated executions. They retain outputs, logs, digest-addressed artifacts, cost, actor, environment, and
provenance, and classify passed, failed, flaky, incompatible setup, missing dependency, unsafe fixture, and
untestable revision distinctly; non-executable states are explicit gaps rather than behavioral evidence.
Caller-stable attempt reservations precede check creation; retries reuse deterministic run identities and
finalization survives unrelated investigation CAS changes, so completed execution cannot become orphaned.
Overlapping exact retries expose the retained running reservation while any reused check is nonterminal and
cannot finalize it as failed. Incompatible setup derives only from structured executor-originated setup
failures, including a pre-run preinstalled-image inspection; command-controlled stderr and exit codes never
alter a completed behavioral classification.
Evidence-driven searches retain the complete ancestry-path commit graph, merge parents, selected readable
dependency revisions, and caller-stable scheduling identity. Read projection joins completed scenario attempts
to candidates and derives recommended trials, competing working-to-regressed ranges, confidence, direct diffs,
commit authors, and linked pulls. Collaborator CAS guidance may classify, exclude, or restore trials with a
reason; causal hypotheses must cite retained evidence or attempts and exact candidate revisions. Missing rewritten
commits, flaky evidence, regressed merges, and multiple supported ranges remain explicit ambiguity and cannot
produce an unsupported single-commit verdict. Search records grant no Git, pull, execution, package, or agent authority.

Conflict reconciliation resolution checkpoints in the workspace ledger assemble an immutable unreferenced
two-parent candidate and retain all affected required checks, reproductions, contract/schema scenarios,
preview acceptance, and repository conflict tests with exact commands, criteria, coverage, logs, artifacts,
costs, and affected-owner decisions. Each criterion runs from a clean reset of that candidate; successor
checkpoints append selective source/target/dependency/policy staleness without deleting historical proof or
granting publication authority. Required-check definitions are frozen from the exact target revision;
required-check checkpoint criteria run through the same disposable read-only container executor and full image,
resource, timeout, input, output, environment, and candidate-identity contract as ordinary check runs;
dependency-manifest and effective-policy revisions are server-derived, and persistence atomically rechecks the
initiator's live command-control version and running workspace state.
Dependency freshness fails closed on Git/tree/blob read errors; only a successful candidate tree lookup that omits
`.vivarium/packages.json` is treated as authoritative manifest absence.

Agent intent at `/repositories/{id}/agent-projects` and the repository `/agents` workspace retains
immutable reviewed prompts, instructions, tools, models, knowledge, memory/data terms, tasks, outputs,
prohibitions, budgets, owners, escalation, and deployment boundaries beneath `$AGENT_PROJECT_STORAGE_ROOT`
(`agent-projects`). Sources resolve to exact repository files and 40-character commits; publishers must
read every dependency and named owners must be current participants. Commits must remain reachable from a
non-`vivarium-security/` branch, and reads recheck and redact inaccessible or hidden sources across every
historical revision. Source contents are resolved only after repository read authorization. Ledger publication
fsyncs staged content before rename and its directory entry afterward; a post-rename sync failure returns the
committed project with a persisted `durability_uncertain` recovery state instead of falsely reporting an
uncommitted mutation. The marker is cleared only after the conservative canonical copy is directory-synced.
Reads derive effective capability,
provenance, history, and attributable missing-owner, conflicting-instruction, inaccessible-dependency,
and unsupported-guarantee diagnostics. Definitions grant no agent, Git, tool, model, network, deployment,
data, or repository authority.

Assurance programs at `/repositories/{id}/assurance-programs` and the repository
`/assurance` workspace retain immutable selections of regulatory, contractual, and
organization requirements beneath `$ASSURANCE_PROGRAM_STORAGE_ROOT` (`assurance-programs`).
Each revision makes applicability, interpretation, inheritance, owners, review periods,
exceptions, control objectives, evidence criteria, and exact resource mappings inspectable.
Derived diagnostics keep missing owners, unsupported claims, inherited duties, conflicting
interpretations, and expiring exceptions explicit and attributable. Named owners and exception
grantors must be current repository participants; records grant no linked-system authority.
Assurance scope admission resolves data flows, infrastructure definitions, environments, and
releases through their repository-owned stores. Policy and procedure scopes are policy-as-code or
runbook files: `resource_id` must equal `path`, and that path must exist at the exact 40-character
`revision` in the containing repository.

Continuous assurance evidence at `/repositories/{id}/assurance-evidence` binds owner-defined
queries and manual, daily, weekly, monthly, or quarterly schedules to an exact assurance-program
revision, control, and assessment period beneath `$ASSURANCE_EVIDENCE_STORAGE_ROOT`
(`assurance-evidence`). Queries cover review, check, access, dependency, build, release,
deployment, incident, continuity, security, privacy, and governance records with exact optional
resource/revision selectors and freshness limits. Immutable packages derive coverage, explicit
missing/inaccessible/stale gaps, contradictions, source and manifest SHA-256 digests, and an
actor/time attestation. Definition audiences are rechecked on reads; restricted sources retain only
an opaque gap, and credential-shaped prose is rejected. Package requests select query IDs only;
resource identity, revision, occurrence, provenance, outcome, and transformations are derived from
repository-owned pull, check/build, access, dependency, release, deployment, incident, continuity,
privacy, and governance stores, while unavailable resolvers fail closed. These records prove provenance and grant no
source-system, repository, review, release, deployment, incident, or governance authority.

Independent assessments at `/repositories/{id}/assurance-assessments` bind a named program owner,
an exact assurance-program revision, selected controls and admitted system/release scopes, an exact
period, explicitly selected evidence packages, and an identified internal or external platform user
beneath `$ASSURANCE_ASSESSMENT_STORAGE_ROOT` (`assurance-assessments`). Invitations expire within 90
days and disclose conflicts before evidence work begins. The owner and assessor append CAS-versioned,
attributable questions, sample and walkthrough requests, attestation verification, findings,
responses, disagreements, resolutions, appeals, and scope-change acknowledgements according to
distinct roles. Reads expose only the selected packages and already-sanitized evidence sources;
expired or conflicted assessor access fails closed. Assessments grant no repository, source-system,
production, review, release, deployment, evidence-collection, or project mutation authority.
When correction produces a release after the originally selected candidate, the owner may propose
that exact repository release only if it descends from a selected candidate release; the assessor
must explicitly acknowledge it before the delivered release can be signed.

Assessment findings can seed ordered ordinary proposal tasks through
`/{assessment_id}/remediations`, freezing the exact finding/control, affected revision, deadline,
acceptance criteria, and human or agent ownership. Progress derives from ordinary task and merged-pull
records; accepted closure requires fresh gap-free post-work evidence and an owner or assessor
disposition. The affected revision must exist in the repository; ordered merged contributions descend
from it, accepted evidence names the final merged revision, and a claimed release descends from and
includes every corrective task and resolves through a release scope explicitly selected by the assessment.
Exact-release `/assurance-statements` retain an immutable Ed25519-signed human-readable claim over scope,
period, controls, exceptions, and evidence digest for an explicit audience. Reads project revocation,
expiry, program drift, and reopened work separately without rewriting the signature or exposing evidence.
Neither record grants task, agent, workspace, Git, policy, release, operational, or evidence authority.
The connected `assurance-journey.spec.ts` browser/API/stock-Git journey covers rejected exception
authority, missing evidence, bounded auditor access, a contested finding, ordered human and agent
correction, assurance impact and ordinary review gates, fresh exact-release evidence, acknowledged
delivered scope, signed publication, successor-program drift, and explicit revocation. Playwright
isolates all assurance stores.

Pre-merge assurance impact at `/repositories/{id}/pulls/{pull_id}/assurance-impact` binds an exact
pull commit to an exact assurance-program revision beneath `$ASSURANCE_IMPACT_STORAGE_ROOT`
(`assurance-impacts`). The server derives affected controls and paths from the Git diff and control
mappings. Decisions retain applicability, changed evidence, tests, notices, retention actions,
exceptions, mitigations, residual risk, and current control-owner acknowledgements. Humans and
exact pull-task read-only agents append cited analysis and challenges; only named human control
owners acknowledge. Pull movement invalidates the candidate, while program successors invalidate
only changed controls. Once a repository has a current assurance program, missing, uncertain,
stale, or unacknowledged impact blocks merge and integration-queue readiness. Historical
assessments remain immutable. Records grant no Git, review, merge, release, evidence, policy, or
linked-system authority.

Security confidence at pull, release, and deployment `/security-confidence` freezes repository or
organization requirements by branch, component, asset, risk class, and path beneath
`$SECURITY_CONFIDENCE_STORAGE_ROOT` (`security-confidence`). Matrices derive current threat-model
coverage, reviewed exact-command security-scenario outcomes, current control-owner acknowledgements,
residual risk, and audience-safe unresolved finding gaps. Pull merge and integration-queue readiness
consume the same matrix; affected paths invalidate only intersecting evidence. Named requirement-owner
exceptions bind the exact revision and narrow selector, expire within 30 days, retain attribution and
an existing follow-up issue or proposal, and grant no delivery authority. Exact-release deployment
signals retain sanitized digests for violated assumptions and failed controls and may connect a
repository-scoped private incident, advisory, or governed repair without granting production,
disclosure, Git, review, merge, queue, release, deployment, environment, or agent authority.
Component, asset, and risk-class requirement labels are admitted only with explicit repository paths;
delivery targets do not otherwise provide authoritative values for those governance dimensions.
The connected `security-assurance-journey.spec.ts` browser/API/Git journey retains privileged-workflow
expectations, bounded agent uncertainty, affected-owner threat decisions, inaccessible evidence, unsafe and
failed replay attempts, a false positive, stale analysis, governed repair, rejected exception, exact security
confidence through review/integration/release/staged deployment, and a sanitized changed-assumption signal
linked to private follow-up work. Repair scenarios may evaluate exact descendants of their frozen modeled
commit; merge and release targets reuse threat/scenario proof only when every explicitly selected path has
identical Git content, so unrelated history shape does not invalidate proof and changed protected blobs do.

Release confidence at pull and release `/quality-confidence` freezes versioned requirements by
branch, journey, risk, locale, platform, release, and path beneath `$RELEASE_CONFIDENCE_STORAGE_ROOT`
(`release-confidence`). Exact scenario, closed exploratory-session, and check-run attempts retain
passes, failures, flakes, gaps, quarantines, affected-path invalidation, and narrow owner overrides;
scenario outcomes derive from exact-command retained checks, exploratory success requires a chartered
signoff event before closure, and every attempt is bound to an exact pull or release target;
overrides expire within 30 days and require follow-up work. Pull merge and integration-queue
readiness consume the same matrix. Post-release sampled scenario signals remain exact-release
evidence and grant no test, environment, Git, merge, queue, release, or deployment authority.
The connected `quality-engineering-journey.spec.ts` browser/API/Git journey retains cross-platform
product intent, reviewed human and agent scenarios, unsafe-fixture rejection, bounded human-agent
pull-preview exploration, stale-result invalidation, a minimized edge-case repair, failed-first and
flaky evidence, matrix-gap and override containment, reviewed merge/release gates, and exact-release
post-release confirmation. Playwright isolates quality-plan, scenario, exploration, and confidence stores.

Collaborative exploration at `/repositories/{id}/quality` and the public
`/repositories/{id}/exploratory-sessions` API retains exact-revision sessions from pull previews,
release candidates, issues, or quality plans beneath `$EXPLORATORY_SESSION_STORAGE_ROOT`
(`exploratory-sessions`). Explicit participant audiences, maximum-24-hour expiry, privacy-safe test
data, cost/action budgets, and narrowing human or approved-agent risk charters bound each session.
CAS timeline events retain attributable routes, sanitized inputs, commands, coverage, uncertainty,
digest-addressed artifacts, guidance, pause/resume, reproduction, classification, and discard history.
Agent control and finding decisions require distinct charter actions, and finding/event references
must resolve within that session's prior timeline.
Confirmed reproduced bugs may reserve a retry-safe governed repair handoff that freezes the candidate,
permitted timeline evidence, minimized reproduction, acceptance criteria, assignee, and quality-plan intent
before creating a linked issue and ordinary human/agent proposal task. Lasting coverage links back only from
a reusable scenario implemented by that exact task pull and commit with matching issue and plan requirements;
flaky, duplicate, environment-specific, and non-reproducible decisions remain explicit classifications.
A pending repair reservation freezes further classification/discard events for its finding until
deterministic issue/task publication is linked, preventing concurrent supersession from orphaning
actionable work; linked history remains immutable while later finding decisions may append normally.
Human actions also require an exact charter assignee/action match; audience membership alone admits
only collaborative guidance.
Candidate movement marks evidence stale without rewriting it. These records grant no preview,
workspace, runtime, environment, Git, deployment, data, or general agent authority.

Production debugging at `/repositories/{id}/debugging` and the public
`/repositories/{id}/debugging-workspaces` API retains revision-exact, audience-controlled starting context
from issues, incidents, support threads, deployments, service objectives, traces, or manual observations beneath
`$DEBUG_WORKSPACE_STORAGE_ROOT` (`debugging-workspaces`). Records freeze the affected release/environment/window,
journey, owners, severity, exact source/package/configuration/infrastructure revisions, permitted sanitized
evidence, explicit gaps, access, attributable hypotheses, status, and compare-and-swap history. They grant no
observability, runtime, environment, deployment, Git, or data authority.
Participants may request explicit-audience, maximum-24-hour probes for logs, traces, profiles, state snapshots,
or exact-source repository-defined diagnostics. Requests preview closed data categories, privacy/security
transformations, retention, sampling, cost, and load; an affected-environment owner may only deny or narrow them.
Only the requester can report against a live approval, retaining sanitized digest metadata, provenance, timing,
transformations, and gaps. Owner revocation, expiry, denied access, overload, and partial capture stop collection,
and evidence with gaps cannot be reported as complete. A probe remains bounded authority, not a general production,
observability, credential, environment, deployment, Git, or data grant.
Probe privacy controls form the closed order `hash_user_identifiers` < `remove_user_identifiers` <
`remove_user_data`; security controls form `detect_secrets` < `redact_secrets` <
`drop_secret_bearing_records`. Approval accepts equal or stronger controls only. Workspace creator, owner,
and access roles never bypass a probe's explicit audience for probe records or lifecycle history.
Workspace diagnosis retains server-resolved citations to selected visible runtime evidence, exact source symbols,
commits, packages/dependencies, configuration, infrastructure, deployments, and known issues. Attributable
hypotheses, queries, findings, uncertainty, support, disputes, and stale markers keep inaccessible evidence
explicit, while code, service, privacy, and security owner questions remain distinct. Purpose-only diagnostic
agents receive only selected citation metadata through a maximum-24-hour credential; collaborators may guide,
pause, resume, or revoke them, and revocation or initiator access loss stops publication. This grants no secret,
observability, deployment, environment, Git, or mutation authority.
Every diagnosis mutation rechecks the restricted workspace audience before parsing or issuing credentials, and
agent-authored claims use the same statement-size and credential-shaped-content rejection as human claims.
Both agent investigation reads and successful claim writes project only the investigation's selected citations
and claims wholly supported by that citation set. Nested claim responses that cite anything outside the selected
set are omitted with their prose, and a write response never returns the enclosing workspace.
Permitted debugging citations can seed immutable minimized replay scenarios at the affected commit. Scenarios retain
only digest-addressed synthetic or privacy-preserving input shapes, sanitization, repository experiment-command
hashes, invariants, dependencies, production differences, unsafe effects, gaps, and parent refinements. Attempts
must resolve through a linked `debugging_reproduction` bounded workspace, and retain its exact environment,
definition digest, command outputs, sanitized trace metadata, derived invariant results, cost, differences, and
gaps. One passing run is only demonstrated; two distinct passing workspaces derive reproduced, mixed results derive
nondeterministic, and unsafe, missing, revision-changed, or irreducible conditions never become reproduced.
Reproduced debugging scenarios and supported cited causes can seed ordinary human- or agent-owned repair proposal tasks.
The handoff freezes the debugging workspace, scenario, cause, affected revision, acceptance criteria, and regression
criteria. Validation resolves the linked merged pull, exact-revision scenario and ordinary check runs, including release,
staged deployment, and named production signals. Failed signals use existing participant-only pause/rollback controls or
reopen diagnosis; debugging and task identities gain no review, merge, release, deployment, or environment authority.
Repair publication reserves its workspace identity before ordinary task creation. Production success requires a
succeeded staged deployment and scenario plus ordinary checks from its exact release commit, not merely the pull source;
every check required by the pull target branch must be selected and passing, so callers cannot omit failed policy gates;
failed-measure actions persist intent first, rollback creation is idempotent for the exact
failed/known-good pair, and post-action write races remain explicitly pending for retry-safe reconciliation.
The connected `debugging-journey.spec.ts` browser/API/Git journey retains denied and privacy-redacted probes,
noisy evidence, disputed diagnosis, revoked agent access, failed and refined replay, a failed first repair,
reviewed integration checks, release, staged deployment signals, and the final validated user outcome.
The connected `interface-design-journey.spec.ts` browser/API/Git journey retains invited-user feedback,
designer and agent-assisted alternatives, a stale and missing-state prototype, human/agent implementation,
exact responsive/localized/accessibility previews, visual-regression recovery, a rejected deviation, governed
acceptances, staged delivery, measured outcome feedback, a changed token, and downstream migration work.

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

The connected `capability-retirement-journey.spec.ts` browser/API/Git journey proves the
capability workflow from released inventory and independent-owner acknowledgement through
human/agent migration, failed-first coexistence evidence, rollback and late-regression
containment, exact delivery, post-retirement staleness, and category-complete cleanup.

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

- **Repository-reviewed collaboration workflows** — `/repositories/{id}/collaboration-workflows`
  and the repository `/workflows` workspace resolve definitions only from an exact JSON blob at a
  40-character commit reachable from a non-`vivarium-security/` branch. Definitions retain a shared
  outcome, owners, typed repository-event/schedule/manual triggers and inputs, conditions, ordered or
  parallel dependency steps, outputs, retries, timeouts, action budgets, owners, and completion
  criteria. Step invocations select a closed permitted platform action, reusable component, active
  same-repository workflow, or gap-free reviewed same-repository agent project and declare their
  authority explicitly. Preview derives subscriptions, execution stages, effective principals and
  grants, the exact source digest, and attributable blockers. Missing owners, invalid/cyclic graphs,
  self/event trigger loops, inaccessible resources, over-budget steps, and conflicting policies block
  activation. Creates require a caller-stable activation ID and derive the workflow identity from
  it, so retrying an ambiguous post-rename durability failure reconciles the published record instead
  of duplicating active automation. Successful activation retains immutable CAS-versioned revisions beneath
  `$COLLABORATION_WORKFLOW_STORAGE_ROOT` (`collaboration-workflows`); records explain coordination but
  grant no repository, event, action, agent, component, runtime, secret, review, release, deployment,
  or linked-workflow authority.
  Runtime `/executions` bind a caller-stable event ID to one workflow revision, source, authenticated
  trigger actor, exact reachable resource revisions, and declared non-secret inputs. The durable run
  schedules only dependency-ready owner steps; a claim mints a random, single-step, timeout-bounded
  capability carrying only reviewed authority and resolved declared inputs. Completion accepts only
  declared non-credential outputs and accounts both step and run action budgets. One active run per
  workflow, a 60-start hourly ceiling, CAS claims, retry limits, token expiry, cancellation, and
  deterministic duplicate-event reconciliation bound retries and outage recovery. Runs and token
  digests persist in the workflow store; plaintext step tokens are returned only on claim. Execution
  does not confer underlying action authority, and resource/participant access is rechecked at runtime.
  Pull triggers require a `pull_id` revision entry that the server derives from that repository pull's
  current source commit; missing, invented, or mismatched provenance fails closed. Exact completion
  retries reconcile through a retained request digest, while terminal failure atomically revokes every
  sibling lease so durable terminal records never advertise unusable live capabilities.
  Repository-event execution requests accept only an immutable activity delivery ID and workflow
  version. Pull creation, synchronization, and merge activity retains its exact source revision; the
  runtime derives trigger kind/name, occurrence time, actor, pull input, and revision from that server
  record and rejects stale or unsupported deliveries. Participants cannot supply trigger metadata.
  Repository-owner issue triage idempotently emits `issue.accepted` at the then-current configured default-
  branch revision before returning even when the status commit reports uncertain durability; a repository
  without that commit cannot enter triage. Any later retry reconciles the retained snapshot regardless of
  subsequent issue versions. Dispatch rejects the delivery if the issue leaves triage, its snapshot and
  activity disagree, or the exact commit becomes unreachable; ordinary default-branch movement does not
  rewrite or strand an accepted event.
  Execution reads are public projections: capability and completion digests are never returned, and
  restricted artifact entries are omitted entirely. Each step retains append-only attempts with declared inputs,
  redacted outputs and logs, digest-addressed artifacts, agent-session references, costs, timing,
  failures, and provenance. Derived next actions explain dependency, approval, requested-input, manual,
  retry, and optional-work waits. Versioned collaborator interventions pause, resume, cancel, retry,
  approve, provide non-secret input, skip only reviewed optional steps, or take over only declared manual
  steps; every accepted change retains actor, reason, step, time, and resulting version. Private terminal
  input and credential-shaped content are rejected, and these controls grant no underlying action authority.
  Repository workflow governance adds versioned requirements for independent review, named simulation
  cases, resource-owner acknowledgement, separation of duties, bounded approval, and the closed merge,
  release, infrastructure-change, protected-evidence, and spending action classes. Exact-source candidates
  retain expected event effects, permission differences, cost, conflicts, decisions, and expiring exceptions;
  policy drift blocks activation. Runs expose expiring owner approval requests and immutable action receipts.
  Owner disablement, anomaly/authority stops, and selection of a prior immutable revision block new starts
  and claims without deleting completed effects, attempts, executions, receipts, or revision history.
  The connected `collaborative-workflow-journey.spec.ts` browser/API/Git journey retains accepted-issue
  provenance, a reviewed bounded agent, duplicate and stale dispatch containment, an interrupted and
  redirected attempt, revoked lease, budget breach, owner-only approvals, receipts, costs, and visible history.

- **Attested workflow components** — `/workflow-components`, repository
  `/workflow-component-installations`, and the `/repositories/{id}/workflows` workspace expose
  immutable reusable automation contracts beneath `$WORKFLOW_COMPONENT_STORAGE_ROOT`
  (`workflow-components`). An owner publishes one semantic version from an exact JSON blob at a
  visible 40-character commit and an active package built from that same commit. The attestation
  freezes typed inputs/outputs, requested capabilities, data classification/purpose/retention/
  destinations, compatibility and breaking-migration terms, passing test digests, support policy,
  package digest, publisher, and optional currently trusted federation peer. Catalog reads recheck
  repository visibility, publisher ownership, package lifecycle and identity, and peer trust, keeping
  changed, revoked, unreachable, quarantined, and breaking-version diagnostics visible.
  A consumer pins or updates an exact component version only through a current ordinary open pull.
  The server derives component identity, mappings, configuration, and data acceptance from the pull
  source commit's canonical `.vivarium/workflow-components/{local_name}.json`; request prose cannot
  claim an unrelated pull as review. Every requested capability must map once to a named local
  permission, every data-use term must be accepted exactly, and bounded configuration rejects credential-shaped content. Installation
  successors retain earlier pins and pull revisions, so component replacement does not rewrite
  workflow execution history. Workflow activation resolves `local-name@semantic-version` only from
  the repository's current installation, rechecks current publisher ownership, active package digest/
  publisher/source, and trusted federation peer, and admits only its mapped local permissions. Components and
  installations grant no source, package, federation, Git, review, merge, workflow, runtime, secret,
  repository, or publisher authority.

- **Collaborative software adoption** — `/adoption-workspaces` and the web `/adoption`
  workspace retain shared software-fit evaluations beneath `$ADOPTION_WORKSPACE_STORAGE_ROOT`
  (`adoption-workspaces`). A human collaborator starts from a roadmap outcome, support gap,
  incubator, decision, package, API, or signed federated-repository snapshot and records required
  journeys, environments, constraints, budget, accountable owners, weighted criteria, and exact
  candidate versions. Provider maintainers and affected users explicitly consent before reading;
  already-approved organization agents are observer-only and receive no mutation authority.
  Pending humans discover only consent coordinates and invitation metadata. Reads match typed
  principal and organization identity and recheck repository evidence access for every viewer,
  replacing out-of-bound evidence with an opaque gap.
  Candidate comparisons cover capabilities, provenance, support, security, data use,
  compatibility, and known gaps with inspectable references. Missing, inaccessible, and
  version-stale evidence remains an explicit gap rather than proof of fit. Workspaces grant no
  repository, package, API, procurement, agent, Git, environment, deployment, or provider-roadmap
  authority.
  CAS-versioned bounded trials resolve an existing attested release or exact readable commit, scope
  packages/APIs and synthetic or permitted data, and select only the adopter's declared journeys.
  Definitions and immutable attempts retain setup, configuration, commands, integration changes,
  checks, previews, measurements, digest-addressed artifacts, costs, findings, and user feedback.
  Human participants and invited approved agents can contribute; failed, blocked, and
  non-reproducible attempts remain visible, while credential-shaped content is rejected and trial
  records grant no execution, production-data, repository, package, API, environment, or deployment
  authority.
  A passing attempt independently reproduced by a different consented human can seed a versioned adoption agreement authored by
  the human adopter or a consented human provider/user participant. It freezes the selected version,
  architecture, configuration decision owners, update/support policy, service/data boundaries,
  exceptions, exit strategy, unresolved gaps, compatibility promises, and recurring cost. Strictly
  ordered human/agent work spans consumer repositories, environments, documentation, and existing
  permitted upstream forks; environments resolve inside their repository, and a catalog lock holds
  repository existence, visibility, ownership, and collaborator facts through publication. Reads
  re-resolve work targets and owners, redact inaccessible details, and project deletion or removed
  ownership as stale instead of preserving false current facts. Every work item remains
  `no_authority_granted`, so the agreement creates
  coordination and accountability without bypassing target repository, secret, review, environment,
  deployment, or provider-roadmap controls.
  Delivery snapshots bind a plan to an already-merged consumer pull, current exact-commit approvals
  and checks, its including release, and a finished staged deployment. Provider, pull, merge, release,
  environment, rollout, health, cost, support, and user-acceptance provenance remains revision-exact.
  Unmet attestations and failed/paused rollout remain a safe pause; successful recovery links to the
  paused delivery. Reads recheck both repositories and redact inaccessible provenance. Delivery
  snapshots grant no review, merge, release, deployment, pause, restoration, agent, or provider
  authority. Repository-bound credentials must match the consumer repository, and attestation
  attribution is fixed to the authenticated human; historical checks from other pull revisions do not
  affect current readiness. Restorations bind the selected paused delivery's deployment ID to the
  recovery promotion's server-derived `recovery_of` target.
  Adoption findings retain one of six closed knowledge kinds, exact trial/attempt or delivery
  provenance, explicit redactions, provider/participant/public visibility, and pending-consent or
  embargoed state. A consented human provider maintainer alone accepts wider disclosure; rejection
  keeps the record local. Accepted findings can link to ordinary issues or local, upstream-fork, or
  federated pulls only after repository topology and authenticated authorship resolve. Those links
  grant no issue, Git, review, check, merge, release, or maintainer authority. Embargoed, rejected,
  or unavailable-provider findings retain local-pull resolution. Verified updates require the
  merged upstream contribution in an exact provider release and a separate merged, checked consumer
  update in an exact consumer release with a succeeded deployment; an optional replaced local pull
  must belong to the same workspace and provenance. The provider release must publish one unambiguous
  package version that the consumer release's exact-commit inventory records as a resolved direct
  dependency, and the consumer update must change every path changed by the selected local pull.
  The connected `adoption-journey.spec.ts` browser/API/stock-Git/package-client journey covers an
  unsuitable candidate, inaccessible evidence, explicit human consent, agent and target-user
  reproduction, credential-leak rejection, a denied shared-credential exception, exact reviewed
  integration, a failed version-regression rollout, provider rejection and unavailability, a
  consumer-authored fork contribution, provider review/release, and a verified exact-package
  consumer update with retained costs, support boundaries, authorship, and user outcome. Playwright
  isolates the adoption-workspace ledger.

- **Project incubators** — `/incubators` and the web `/incubators` workspace provide a
  collaborative home before repository creation beneath `$INCUBATOR_STORAGE_ROOT`
  (`incubators`). Human collaborators open an incubator from feedback, a support gap, a
  repository governed proposal, or a new idea and freeze its audience, problem, desired
  outcome, constraints, success measures, human sponsors, decision rights, visibility, and
  invitations. Source resolution is server-derived and remains explicitly `resolved`,
  `missing`, or `inaccessible`; inaccessible records do not disclose whether a private source
  exists. Existing visible initiatives with the same title or problem are reported as potential
  duplicates rather than silently merged. Human invitees must explicitly accept before shaping
  the work; pending and declined invitations expose no participants-only incubator context, while
  the direct consent endpoint remains the narrow invitation-response path. Approved organization
  agents retain their existing identity and approval boundary.
  Compare-and-swap events keep discussion, evidence, assumptions, scope and visibility changes,
  and consent attributable. Scope changes bind only to the typed `scope_change` decision right;
  Visibility changes likewise bind to the typed `visibility_change` right. Owner, majority, and
  consensus/consent rules derive from distinct attributable support events tagged with the exact
  decision kind, so support for one authority cannot satisfy another.
  Writes publish a conservative `durability_uncertain` marker before clearing it after a
  directory-synced canonical copy, so a post-rename sync failure is returned as committed state.
  Incubators grant no repository, organization, Git, agent, tool,
  deployment, or implementation authority.
  Participants compare immutable product/technical alternatives beneath each incubator, explicitly
  covering product boundary, architecture, interfaces, dependencies, licenses, operating cost,
  security/data risk, build/adopt posture, and unknowns. Evidence references resolve at admission as
  permitted public, organization, decision, prototype, package, API-contract, or exact code context;
  code evidence must be a blob at a commit reachable from a non-`vivarium-security/` branch, and failed
  selectors are redacted before persistence;
  missing and inaccessible evidence stays explicit. Human and already-approved invited agents can
  define digest-addressed reproducible experiments and append derived outcomes, measurements, artifact
  digests, unknowns, assumptions, dissent, and supersession. Experiment authority is always
  `research_only_no_code_or_infrastructure_authority`; these records do not create repositories,
  commits, packages, environments, infrastructure, or implementation authority.
  An accepted non-superseded direction can reserve a complete governed project boundary through
  `/incubators/{id}/bootstrap-previews`. The manifest covers organization, repositories, teams,
  packages, agent roles, contributor pathway, documentation, environments, and review/security/
  privacy/quality/release baselines, exposing create/connect intent, current human owners, effective
  access, explicitly unverified recurring-cost estimates, generated defaults, inherited policy, and exact incubator/alternative
  provenance before activation. Connected organizations and repositories resolve inside the
  requester's current ownership boundary. Every distinct consenting human resource owner approves an
  exact plan version; activation is one canonical ledger transition, while previewed, rejected, and
  approved reservations can be atomically released. The boundary reserves identities and context but
  grants none of the underlying organization, Git, credential, package, environment, policy, review,
  release, or deployment authority.
  Caller-supplied access/default/policy claims are discarded in favor of the server template and
  retained template source; unsupported connected kinds fail closed. Activation refreshes those
  derived fields and holds every declared connected-organization/repository owner's current ownership stable through the
  canonical boundary write, so deletion or transfer cannot race activation.
  Once active, an incubator can retain an ordered representative delivery slice with exactly one
  code, tests, documentation, infrastructure, and interface work item, each bound to a readable
  repository commit and a temporary human-agent participant. Dependencies are admitted as earlier
  one-based positions and frozen to generated work-item IDs. Attributable workspace, pull, preview,
  target-user feedback, agent action, handoff, cost, deviation, check, and review reports connect
  ordinary execution back to the incubator without granting any linked authority.

- **Incubator launch readiness** — `/{incubator_id}/launch-readiness` retains a complete,
  versioned public-life assessment only after an active project boundary and running delivery slice.
  Thirteen closed expectations bind evidence and a current human participant owner. Only that owner
  accepts current evidence or records a maximum-30-day exception with connected follow-up work;
  failed user validation cannot be excepted, and any exception narrows a public declaration to a
  limited audience. Missing maintainers, unsafe defaults, unsupported promises, stale evidence, and
  expired exceptions remain explicit blockers. Readiness grants no release, package, deployment,
  governance, budget, repository, or follow-up-work authority.

- **Incubator launch and stewardship** — `/{incubator_id}/launches` binds a ready assessment's
  effective audience to the first release, documentation, contributor opportunity, governed
  environment, and package or API contract inside its delivery boundary. Attributable adoption,
  support, reliability, cost, success-measure, and feedback observations connect existing evidence
  to roadmap revisions or ordinary human/agent work. A human participant records graduation,
  continued experiment, merge, or archive as a terminal disposition; archive requires explicit
  resolution of both resources and obligations. These records grant no publication, repository,
  roadmap, task, environment, organization, release, package, or deployment authority.
  All artifact, observation, work, graduation, and merge references resolve through their owning
  stores before mutation; missing resolvers fail closed. One incubator has one first launch, and
  archive closure maps every frozen artifact to a closed repository-scoped governance proposal and
  rechecks every accepted or exception-backed readiness obligation; copied identifiers are not proof.
  A resolution proposal also requires an accepted uncontested tally and an exact affected-resource
  match on artifact kind, identity, and revision; closed, rejected, or repository-only matches fail.
  The connected `incubator-journey.spec.ts` browser/API/stock-Git journey carries a shared need from
  consented human and approved-agent research through alternative rejection, governed bootstrap,
  revision-exact delivery and invited-user validation, complete readiness, first launch, adoption-led
  work, and continuing stewardship. Playwright isolates `$INCUBATOR_STORAGE_ROOT`; duplicate ideas,
  unavailable owners, a budget-rejected bootstrap, failed delivery evidence, and an incomplete launch
  remain contained without granting research, Git, bootstrap, review, publication, or roadmap authority.

- **Capability inventory** — `/repositories/{id}/capabilities` and the repository
  `/capabilities` workspace retain immutable, exact-release capability revisions beneath
  `$CAPABILITY_STORAGE_ROOT` (`capabilities`). A revision selects interfaces, symbols,
  flags, packages, schemas, configuration, documentation, journeys, and releases by exact
  Git revision and repository path, and maps accountable owners, permitted consumer
  repositories, environments, revision-bound usage evidence, and compatibility promises.
  Publication proves the collaborator can still read every named consumer repository while
  holding the repository mutation boundary. Unknown or dynamically discovered consumers and
  stale, inaccessible, or absent evidence remain explicit diagnostics and never imply zero
  use. Human participants can open a retirement plan against an exact inventory version with
  supported replacements, affected audiences and stop-working impact, ordered compatibility
  stages, deadlines, success/rollback criteria, communication, and every consumer-owner approval.
  Append-only cited assessments and challenges admit humans and repository-bound read-only agents,
  while only exact human owners acknowledge. Inventory drift, evidence gaps, conflicting commitments,
  embargoed dependencies, bounded exceptions, and unresponsive owners remain attributable blockers;
  human deferrals expire within 30 days and retain follow-up work. Every plan projects deadlines
  through the durable store clock and redacts its frozen audiences against its bound inventory
  revision; regenerated inventory blockers retain their current diagnostic consumer index, so later
  consumer reordering or renaming cannot expose restricted current names either. Restricted-owner event
  identities, prose, and citations are redacted on reads and retirement mutation responses through the
  same projection. A current participant in the provider or each affected consumer repository can publish ordered human-
  or approved-agent-owned migration tasks into that repository's ordinary proposal workflow. Each link
  freezes the exact old and supported replacement contract, acceptance criteria, documentation changes,
  rollout stage, exact base, and earlier contribution dependencies; agent sessions wait for those earlier
  contributions to merge. Task, session, workspace, fork, pull, review, and merge authority stays with the
  target repository, while visible plan reads derive its current progress. Current participants in a newly
  discovered consumer can report exact revision/path evidence back to the frozen plan for reassessment
  without enrolling their repository into provider authority. A post-rename capability directory-sync
  failure is durability-uncertain committed state: linked target-repository work is preserved for
  reconciliation, never destructively compensated. Dependency completion is derived before privacy
  filtering and revalidated beneath the ordinary proposal start lock through agent-session publication.
  Plans also retain immutable exact-provider/release, consumer, schema, and configuration coexistence
  candidates. Mandatory old-only, dual-support, replacement, rollback, and journey checks derive logs,
  artifacts, duration, and resource cost from exact-command bounded-workspace outcomes. New proof
  supersedes rather than rewrites history; one retained outcome can satisfy only one matrix check, and
  candidate publication resolves a check repository only after authorizing the reader. Reads compare selected Git blobs so only affected checks
  become stale. Per-consumer usage windows remain measured, unmeasured, or inaccessible; readiness
  requires current passing checks plus owner-acknowledged measured zero old-behavior use for every frozen
  audience. Privacy projection retains restricted blockers without exposing consumer evidence or identity.
  Once those gates and exact owner approvals remain current, a human capability owner can start a
  controlled removal execution against the ready candidate. Ordered stage reports retain ordinary merge-queue,
  release, schema/infrastructure migration, documentation, and deployment references alongside remaining use,
  health, controller, rollback boundary, and next action. Degraded health, residual use, or an unexpected
  consumer pauses advancement; an explicit compatibility restore preserves the failed attempt. Completion
  requires revision- and path-bound proof that obsolete code, flags, data, credentials, telemetry,
  documentation, and policy exceptions were each removed. Execution records retain provenance and expose
  delivery state but grant none of the linked systems' authority. If ownership changes during active removal,
  a current human capability owner must explicitly transfer the CAS-versioned controller and retain the prior
  controller, successor, actor, reason, and time. Final cleanup proof names the provider repository, must use a
  revision retained as succeeded delivery for that execution. Each migration candidate freezes a
  category-complete obsolete-artifact inventory with provider path, pre-removal revision/blob, and an expected
  removed or replaced outcome; only paths selected by the frozen capability revision are admitted and every
  selected path must be covered, while duplicate category/path requirements are rejected. Completion maps proofs one-to-one to those requirement IDs and verifies absence
  or the declared changed blob at the delivered revision; unrelated retained files cannot satisfy cleanup.
  These records provide removal
  context only and grant no consumer-repository, Git,
  release, environment, migration, review, merge, deployment, or retirement authority.

- **Durable state** — `/repositories/{id}/durable-schemas` and the repository
  `/durable-state` workspace retain immutable database, queue, index, object-store,
  event-log, cache, and other persistent-store schema revisions beneath
  `$DURABLE_SCHEMA_STORAGE_ROOT` (`durable-schemas`). Every revision cites a path whose
  exact contents resolve at the merge commit of a merged pull and declares owners, compatibility, retention,
  privacy, and service/environment links. Migration plans originate from an
  existing repository pull or affected technical decision, compare exact schema
  versions, classify reads, writes, backfills, and destructive work, and freeze
  affected consumers, ordered measures, required participant approvals, rollback
  limits, and compare-and-swap attributed history; approval events are step-bound
  and accepted only from that step's declared approvers. These records expose review
  context only and grant no deployment, environment, Git, or data-store authority.
  Migration work beneath `/{schema_id}/migrations/{migration_id}/work` links ordered
  schema-change, compatibility, backfill, verification, and cleanup steps to ordinary
  repository proposal tasks. Each task freezes an explicit old/new reader and writer
  contract, rollout flags, idempotency, transformations, ownership, and rollback
  assumptions; agent sessions wait for merged cross-repository dependencies. Work is
  visible only where the reader can also read its target repository, and no schema
  definition, privacy metadata, data sample, or authority crosses that link implicitly.
  Migration rehearsals freeze an exact application commit, schema and migration versions,
  dependency fingerprints, privacy-bounded dataset shape, and repository checks for upgrades,
  dual reads/writes, backfills, validation, rollback, and failure injection. Bounded-workspace
  runs retain sanitized outcomes, counts, invariants, performance, artifacts, costs, and
  attestations; command status, exit code, log, duration, invariant result, and overall result
  derive from one unambiguous exact-command workspace outcome. Attributable notes support failure investigation. Checks declare their revision
  inputs so successor candidates invalidate only affected proof. Rehearsals grant no production,
  deployment, environment, workspace, or data-store authority.
  Run publication rejects caller-supplied counts, artifact digests, costs, and attestations until
  those values have platform-retained provenance; it emits only exact workspace/command attestations.
  Every check freezes distinct work and invariant commands. Both retained outcomes must start and
  finish after rehearsal creation, and both must succeed before the invariant or run can pass.
  Production execution beneath `/{migration_id}/executions` begins only after every declared step
  approval and a passing rehearsal, freezes an exact existing release and established environment,
  and sequences expand, deploy, backfill, cutover, and contract. CAS controls and reports expose
  active revision, controller, compatibility/privacy/cost bounds, progress, lag, invariants, service
  health, blockers, and next actions. Advancing requires complete healthy unblocked evidence;
  deployment evidence resolves through the existing promotion boundary. Agents may report only an
  exact phase and step delegated to their authenticated identity; step evidence remains separate and
  cannot populate controller-owned phase readiness. Every current-phase delegation's latest report
  must be complete, healthy, invariant-backed, and unblocked before human-controlled advancement.
  Failed invariants, service regressions, revoked approvals, conflicting writes, capacity exhaustion,
  and interrupted backfills append evidence and pause at the reported safety point. Participants may
  append an idempotent retry, attested recovery-point restore, compatibility-window release rollback,
  or a link to ordinary assigned migration repair work; failed attempts are never rewritten. Each
  execution declares its observation period. After success and that full period, every candidate-schema
  owner must approve retirement before completion freezes removed compatibility machinery, obsolete
  fields, irreversible decisions, retained/changed/deleted data, exceptions, cost, and current schema
  version for every established environment. The observation starts strictly after the latest relevant
  final-phase completion, and each reported environment requires its own completed, deployment-linked
  execution. A failure-caused pause cannot resume until its latest failure has a matching recovery record.
  Failed invariants and interrupted backfills may use an evidence-backed idempotent retry; service
  regressions, capacity exhaustion, and conflicting writes additionally require an explicit remediation
  attestation (or an attested restore or compatibility-window rollback). Opening repair work alone remains paused.
  Execution records carry no commands, credentials, database,
  deployment, environment, or destructive authority.
  The connected `durable-state-journey.spec.ts` browser/API/Git journey proves reviewed
  breaking revisions, human and agent migration work, failed and passing privacy-bounded
  rehearsal, governed delivery, old-writer fencing, interrupted-backfill and invariant
  recovery, contract-phase rollback rejection, and observation-gated verified cleanup.
  Playwright isolates these records with `DURABLE_SCHEMA_STORAGE_ROOT` like its other stores.

- **Infrastructure intent** — `/repositories/{id}/infrastructure` and the repository
  `/infrastructure` workspace retain immutable, exact-commit definitions of environments,
  services, networks, identities, data stores, compute, and external dependencies beneath
  `$INFRASTRUCTURE_STORAGE_ROOT` (`infrastructure`). Resources name current participant owners,
  providers, non-secret provider identities, configuration boundaries, cost/capacity limits,
  dependencies, established release/environment links, and security, privacy, reliability,
  continuity, and regional commitments. Append-only sanitized observations bind an exact
  definition version and observed provider revision. Public reads redact participant-only
  provider identities, revisions, and summaries; credential-shaped content is rejected.
  Projections keep unmanaged resources, inaccessible providers, observations older than 24
  hours or tied to predecessor definitions, conflicting provider ownership, secret-backed
  boundaries, and missing current observations explicit. Definitions and observations are
  inventory/review context only and grant no provider, credential, repository, deployment,
  environment, or infrastructure authority.
  Pull infrastructure plans freeze an exact open-pull source revision, current definition version,
  permitted observation fingerprint, candidate resources, and exact candidate-commit policy-file
  digests. Immutable comparisons derive create/change/replace/destroy actions in dependency order,
  affected owners, availability/security/privacy/continuity/cost/data risks, policy effects, and
  rollback limits. Append-only CAS events admit participant and repository-bound read-only-agent
  assumptions and impact analysis, but only an exact affected human owner can acknowledge. Reads
  re-resolve the pull, definition, observations, and policy blobs; drift marks a plan stale and removes
  its acknowledgements from the current projection. Plans grant no operational or review authority.
  Current plans retain isolated or policy-approved ephemeral rehearsals with expiring provider scope
  limited to changed resources and synthetic or explicitly permitted state. Repository checks cover
  provisioning, connectivity, access, policy, service journeys, failure, cost, teardown, and recovery.
  Runs derive sanitized outcomes, timing, artifacts, resource graphs, attestations, and agent actions
  from the collaborator's exact-candidate bounded workspace. Destructive effects stay explicitly
  unsupported; rehearsals grant no production, provider, deployment, review, merge, or environment authority.
  Authoritative applies beneath `/infrastructure-executions` admit only an exact merged plan whose current
  inputs, affected-owner acknowledgements, latest passing rehearsal, established environment, environment
  policy, and reviewed cost limits remain satisfied. Executions freeze reviewed and merge revisions, candidate
  digest, dependency-ordered steps, human controller, budget, and short-lived resource/step/action scope.
  Sanitized CAS reports retain provider responses, health, cost, blockers, next actions, and safety points;
  degraded or blocked work pauses. Controllers steer only at safety points, while agents report only explicitly
  delegated non-destructive steps and gain no secret, approval, provider, or unrelated-resource authority. One
  running or paused controller is admitted per environment across all plans. Execution creation derives persisted
  owner acknowledgement events and rejects unresolved or cyclic changed-resource dependencies. Paused reports may
  record remediation and clear blockers but preserve step state; only an explicit resume can advance or complete.
  Passing rehearsal evidence must use the authoritative environment ID, or be a `policy_approved_ephemeral`
  rehearsal whose frozen policy approval exactly equals the execution's established environment-policy reference;
  isolated preview evidence alone cannot admit production.
  Completed, paused, and cancelled applies retain version-bound convergence assessments against frozen resource
  presence and service, security, privacy, cost, and continuity measures. Only a succeeded apply with complete
  passing observations, no unmanaged resources, and no failed cleanup projects convergence. Participant-scoped
  monitor runs retain granted, partial, or denied provider visibility and sanitized configuration drift, unmanaged
  change, cleanup, credential-expiry, and provider-loss findings with available cause attribution. Owners link
  findings to ordinary incident, exception, repair, reviewed adoption, or declared-state restoration work; these
  append-only links neither rewrite external observations nor grant provider, environment, policy, or review authority.
  The connected `infrastructure-journey.spec.ts` browser/API/Git journey proves exact application-plus-infrastructure
  planning, owner and scoped-agent collaboration, isolated rehearsal recovery, protected execution, convergence,
  drift detection, and reviewed repair while retaining rejected stale, destructive, over-budget, failed-provider,
  partial, revoked-credential, and failed-cleanup paths.

- **Developer support** — `/support` and
  `/repositories/{id}/support-threads` retain contextual questions separately
  from unexpected-behavior issues. Threads name a repository, package,
  release, API, documented journey, or error; keep version/environment/goal/
  attempted-step gaps explicit; and accept only bounded logs, configuration,
  or sample code. Audience controls visibility, contact email is projected only
  to the author and current repository participants, and related answers/issues
  expose metadata but never candidate evidence. Records default beneath
  `$SUPPORT_THREAD_STORAGE_ROOT` (`support-threads`).
  The web accepts `?repository={id}` for public package-user entry. The asker
  and current participants append bounded, CAS-guarded in-thread replies;
  participant replies create asker-only notifications, while closed threads
  reject further discussion and replies grant no repository authority.
  Exact cited answer revisions can be exercised through ordinary bounded workspaces with source kind
  `support_verification`; immutable attempts live beneath `$SUPPORT_VERIFICATION_STORAGE_ROOT`
  (`support-verifications`). Attempts freeze the stated version, declared environment, sanitized-input,
  instruction, commit, and workspace-definition digests plus exact command outcomes, bounded sanitized
  artifacts, cost, and result. Reruns require a fresh workspace with the same answer revision and inputs;
  reads derive stale provenance, and credential-shaped reusable evidence is rejected. Attempt creation
  revalidates the workspace's exact support source; private workspaces cannot publish repository-readable
  verification output.
  Passing, current attempts can become immutable reusable solutions beneath
  `$SUPPORT_SOLUTION_STORAGE_ROOT` (`support-solutions`). The asker or a current participant freezes the
  exact thread, answer revision, attempt, instructions, versions, limitations, audience, project links,
  and contributor credits. Public publication requires public repository, thread, and answer evidence;
  documentation, package, release, and contributor-guidance links resolve within the repository. Search
  omits merged duplicates, while version-guarded duplicate merge, archival, and revalidation requests append
  attributed lifecycle events and notifications without rewriting the published solution or its evidence.
  Publication holds the support-thread mutation boundary through idempotent solution persistence and closure,
  and compensates the exact evidence-bound solution created by the current request if the thread close cannot
  persist; idempotently returned prior solutions are never armed for deletion;
  terminal merged or archived records cannot be revived, and search admits only published or revalidation-needed
  records.
  Current repository collaborators can classify an unresolved thread and escalate it through
  `/repositories/{id}/support-threads/{thread_id}/escalations` into an ordinary issue, documentation task,
  proposal, or dependency-ordered human/agent plan. The handoff freezes the affected version, user goal,
  permitted reproduction, and acceptance criteria but never copies attachments or contact details. Human tasks
  gain no access and agent work remains behind the existing task-scoped launch, review, check, and merge boundaries.
  Creation first persists a pending escalation identity; issue and documentation creation reuse that identity and
  proposal creation reuses the exact support origin, so finalization failures reconcile without duplicate work.
  Published records retain their initiating thread version, allowing a lost-response retry to return the exact
  governed resource without invoking publication again; the same exact request after a thread refresh also returns
  that record. Pending records freeze the initial default-branch base so proposal and documentation reconciliation
  cannot drift when the branch advances.

- **Project knowledge** — `/repositories/{id}/knowledge` retains proposed and superseding
  guidance separately from conversational explanations and support questions. Every claim cites
  exact participant-visible source/symbol/documentation, package, release, answered support thread,
  or known issue evidence and names applicable versions; source citations must be reachable from a
  non-security branch. Scoped repository agents must declare uncertainty on every claim and cannot
  review or verify answers. Current human participants comment, request clarification, endorse, or
  challenge the current immutable revision, while only the repository owner marks it verified,
  context-missing, or retired. Audience is rechecked on reads and restricted evidence never becomes
  part of a public answer implicitly. Records default beneath `$KNOWLEDGE_ANSWER_STORAGE_ROOT`
  (`knowledge-answers`).

- **Interface systems** — `/repositories/{id}/interface-systems` and the repository
  `/interface-system` workspace retain versioned reusable visual and interaction
  decisions beneath `$INTERFACE_SYSTEM_STORAGE_ROOT` (`interface-systems`). Each
  revision is bound server-side to an exact repository release commit and to
  source paths present in that snapshot, and includes design tokens, themes,
  components, interaction patterns, content rules, responsive behavior, rendered
  example descriptions, accessibility/localization constraints, owners, adoption
  policy, and consumer implementation evidence. Publishing is participant-only
  and compare-and-swap guarded; repository read visibility controls discovery.
  Public projections preserve immutable history and explicitly diagnose conflicting
  current definitions, missing owners, unsupported consumers, stale implementations,
  and unknown implementation currency rather than selecting a false canonical
  system. These records document reviewed product intent and provenance but grant
  no Git, release, review, merge, deployment, or consumer-repository authority.

- **Security expectations** — `/repositories/{id}/security-expectations` and the repository
  `/security` workspace retain immutable versions of intended security properties beneath
  `$SECURITY_EXPECTATION_STORAGE_ROOT` (`security-expectations`). Repository, service, interface,
  package, extension, environment, and journey scopes connect protected assets, trust boundaries,
  human/agent/service/external/attacker capabilities, abuse cases, required controls, owners,
  severity response and release rules, and expiring exceptions. Links identify related design,
  privacy, infrastructure, API, quality, and release commitments without copying their contents or
  authority. Publications validate the complete reference graph and current participant ownership;
  reads keep missing owners, explicitly contradictory boundaries, unsupported guarantees, and
  expired or seven-day-expiring exceptions visible and attributable. These records document intent
  only and grant no Git, review, merge, release, deployment, environment, linked-resource, or
  control-execution authority.

- **Executable security scenarios** — `/repositories/{id}/security-scenarios` translates an exact
  threat-model abuse path into an immutable owner-reviewed attempt/defense specification. Scenarios
  freeze bounded attacker capabilities, safe fixture digests, candidate-defined checks, actions, and
  observable containment, detection, and recovery. Evidence resolves from an exact-candidate isolated
  workspace or current successful preview; workspace commands derive from retained outcomes, and
  sanitized logs, artifact metadata, coverage, gaps, costs, and provenance remain append-only.
  Destructive, secret-bearing, production/user-data, hidden-fixture, stale, and over-budget work is
  rejected, while unsafe and non-reproducible outcomes require explicit reasons. Records beneath
  `$SECURITY_SCENARIO_STORAGE_ROOT` (`security-scenarios`) grant no execution, workspace, preview,
  environment, Git, review, merge, release, deployment, secret, or data authority.

- **Governed security findings** — `/repositories/{id}/security-findings` retains audience-controlled,
  revision-exact findings linked to threat-model paths beneath `$SECURITY_FINDING_STORAGE_ROOT`
  (`security-findings`). Reporters retain authorship and bounded permitted evidence, while only the
  repository owner can classify or change the audience. Confirmed findings can seed ordinary assigned
  proposal tasks whose reasoning freezes the threat, candidate, evidence, and acceptance criteria;
  requested task, change-session, or shared-workspace context is launched through those existing
  authorization boundaries. Lasting protection requires the exact task pull, an owner-reviewed passing
  security scenario on its source commit, and retained failed abuse evidence on the affected base.
  Duplicate, false-positive, accepted-risk, embargoed, and failed-repair decisions remain append-only.
  Finding records grant no Git, task execution, workspace, agent, secret, review, merge, deployment, or
  environment authority.

- **Change threat models** — `/repositories/{id}/threat-models` and the repository `/security`
  workspace retain immutable analysis revisions beneath `$THREAT_MODEL_STORAGE_ROOT`
  (`threat-models`). Each model is server-bound to an exact design proposal, pull, API contract,
  durable schema, infrastructure plan, or product experiment revision and maps entry points,
  privileges, data flows, dependencies, attacker goals, abuse paths, mitigations, residual risk,
  alternatives, assumptions, permitted citations, and affected owners. Humans and exact
  exact source-pull task/branch-bound agents append revision-bound cited findings, challenges, comparisons, and
  acknowledgement requests; only a current named human owner can acknowledge. Inaccessible
  citations project only an explicit gap and cannot support a contribution. Reader projections
  reauthorize citation metadata through the currently visible governed source; evidence without a
  current resolver is redacted even when it was accessible at publication. Authorization keys bind
  the complete citation identity across each retained revision, and events whose citations become
  inaccessible project a restricted placeholder instead of contributor prose. Publications replace
  caller freshness claims with a fingerprint of the authoritative source snapshot; reads re-resolve
  that fingerprint so source, architecture, trust-boundary, or dependency movement projects
  staleness without rewriting history or exposing restricted metadata. These records grant no source, Git, review, merge,
  release, deployment, environment, infrastructure, experiment, or general agent authority.

- **Interface verification** — Pull evidence beneath
  `/repositories/{id}/pulls/{pull_id}/interface-checks` binds the exact candidate,
  successful bounded preview, candidate-resolved repository check definition, and
  accepted implemented design revision. Checks retain viewport, theme, content,
  locale, interaction, assistive-technology, difference, recording/artifact,
  coverage, performance, and affected-requirement evidence. Collaborator
  classifications are attributable and revision-bound; reads separate stale
  evidence after candidate movement or an accepted design successor. Cross-store
  publication holds both design- and pull-revision guards through persistence.
  Records beneath `$INTERFACE_CHECK_STORAGE_ROOT`
  grant no execution, Git, design approval, review, merge, deployment, or
  environment authority.

- **Design acceptance and evolution** — Repository and organization policies at
  `/design-acceptance-policies` require named design-owner, accessibility,
  content, localization, or invited-user decisions for matching components,
  journeys, paths, and risk classes. Acceptances and bounded owner exceptions
  freeze the exact pull revision and policy version. `/design-readiness` is
  projected into ordinary merge readiness and separately for releases; unresolved
  differences, regressions, obsolete implementation evidence, stale previews,
  and exceptions expiring within seven days remain explicit. Successful interface
  reruns supersede older attempts for the same revision, definition, and journey
  without suppressing independent definitions that share a journey. Releases
  freeze each included pull's source revision and changed paths, so later pull
  synchronization cannot rewrite release readiness. Interface-system
  migration work and post-release feedback/regression repairs create ordinary
  repository proposal tasks and grant design participants no review, merge,
  release, deployment, organization, or repository authority. Records default
  beneath `$DESIGN_GOVERNANCE_STORAGE_ROOT` (`design-governance`).

- **Product design proposals** — `/repositories/{id}/design-proposals` and the repository
  `/design` workspace retain product behavior before implementation. Each immutable revision
  names its feedback, issue, roadmap outcome, accessibility finding, or pull-request source and
  defines the user goal, journeys, states, content, constraints, alternatives, success measures,
  affected components, uncertainty, evidence citations, and explicit-audience wireframes or
  prototypes beneath `$DESIGN_PROPOSAL_STORAGE_ROOT` (`design-proposals`). Participant discussion,
  questions, dissent, and grounded evidence remain revision-bound; inaccessible citations retain
  gaps without copying asset content. Only named current participant owners acknowledge or request
  changes on the current revision. These records grant no Git, review, implementation, release,
  deployment, environment, research, or private-asset authority.
  Once every named owner acknowledges the current revision, its implementation endpoint freezes the
  default-branch commit and creates ordinary dependency-ordered human/agent proposal tasks. Frozen task
  reasoning carries audience-safe immutable prototype references, component contracts, asset provenance, content,
  breakpoints, states, and acceptance criteria into task workspaces and pull contributions. Exact artifact payloads
  remain behind the design proposal's reader-specific projection. Implementation reports map code paths
  and rendered surfaces to requirements; deliberate deviations remain pending until a named design owner
  approves or rejects them. This ledger grants no workspace, Git, agent, review, or merge authority.

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
  External extensions at `/extensions` retain a distinct `extension` principal,
  human owner, operator contact, declared capabilities/events, verified callback
  and action endpoints, requested resource actions, and credential rotation
  policy beneath `$EXTENSION_STORAGE_ROOT` (default `extensions`). Registration
  performs a live challenge against both endpoints and previews zero effective
  authority: it issues no credential and cannot act as its owner, a user, or an
  approved agent. Repository and organization owners install verified
  extensions through version-guarded records naming exact repositories and
  resource types, per-capability decisions, non-secret settings, effective
  access, and actor history. Lifecycle changes revalidate current ownership;
  suspension and removal revoke only credentials derived from that installation.
  Reverified contract updates remain pending until renewed owner consent.
  Operations project attributed requests/actions, delivery health and latency,
  consumption, permission use, credential health, history, and notices;
  rotation, narrowing, pause, quarantine, and removal preserve prior evidence.
  Rotation durably shortens predecessor expiry to the configured overlap
  deadline; hourly notices use the rolling hour while totals remain lifetime.
  Rotation atomically publishes its successor and, at zero overlap, detaches
  predecessors at the installation authority boundary before best-effort auth
  retirement; positive overlap failures detach the affected predecessor.
  Active installations subscribe to permitted project events through durable
  v1 Ed25519-signed envelopes with stable event/delivery identities, monotonic
  per-installation sequence and repository ordering keys. Owner delivery
  surfaces expose redacted payloads, attempts, retry, replay, and dead letters;
  duplicate source events are idempotent and inactive, unsubscribed, or
  resource-inaccessible installations receive no delivery.
  Federation at `/federation` publishes a stable Ed25519-signed
  `/.well-known/vivarium-federation` instance document and retains explicit
  local peer trust beneath `$FEDERATION_STORAGE_ROOT` (default `federation`).
  `$FEDERATION_PUBLIC_URL`, `$FEDERATION_INSTANCE_NAME`, and comma-separated
  local user IDs in `$FEDERATION_OPERATORS` define its public projection and
  exclusive administrative authority (an empty list fails closed). Discovery requires
  HTTPS/public addresses except HTTP loopback development; address safety is
  rechecked at dial time. Signed, predecessor-authorized version/key changes, outages, and
  revocation remain explicit. Instance-qualified user and public approved-agent
  cards are attribution references only: they never authenticate locally, and
  actor resolution provides no membership enumeration.
  Trusted peers may also advertise `repository-discovery.v1`. Public remote

  Trusted peers advertising `repository-contribution.v1` exchange only bounded
  exact-revision Git bundles and signed proposal envelopes. Local federated
  forks retain instance-qualified upstream lineage while using ordinary local
  Git credentials; synchronization is selected-branch fast-forward only.
  Remote proposal negotiation freezes both tips and creates ordinary target
  review without granting either instance credentials or repository authority.
  Federated pull collaboration exchanges immutable signed, origin-preserving
  comment, review/requested-change, revision, bounded check/preview evidence,
  and closure events keyed by the contribution identity. Delivery is
  idempotent with explicit pending/conflict recovery; revision movement derives
  stale evidence normally. Imported claims remain verified remote evidence and
  never become local users, required checks, credentials, review authority, or
  merge permission. Accepted federated pulls use the ordinary atomic local
  merge boundary, retain their reachable Git objects and verified collaboration
  evidence, and publish a locally retained signed receipt through a durable
  outbox; peer deletion, revocation, or outage cannot undo accepted history or
  remain authorization. Source-instance participants may delegate a federated
  contribution to an approved agent they operate. Delegation freezes the
  current contribution revision and selected existing paths, issues only a
  short-lived local credential bound to the fork, source branch, and
  contribution identity, and completion transfers only descendant commits plus
  a signed redacted `agent_session` summary. Guidance and revocation stay
  local; remote secrets, checks, credentials, review, and merge authority never
  enter the mandate.
  repository projections are bounded signed metadata carrying exact revisions,
  branches, releases, contributor guidance, public issues, and open contribution
  opportunities. Home-instance resolution verifies the retained peer document
  and caches only permitted metadata; follow state is local, while unsupported
  capabilities, visibility loss, invalid signatures, stale caches, and outages
  remain explicit and never imply local repository control.
  Local sample endpoint verification is available only when
  `$EXTENSION_DEVELOPMENT_ENDPOINTS=1`, and then only for HTTP loopback hosts;
  production/default verification remains HTTPS and public-address-only. The
  connected extension browser journey covers signed pull delivery, replay,
  revision-bound annotations/artifacts/actions, renewed capability consent,
  ordinary review/merge, and uninstall with retained evidence.
  Installation owners mint short-lived `extensions:contribute` credentials
  derived from the exact installation. Extensions publish append-only,
  attributed pull evidence at the current revision with idempotency, scope,
  rate, and payload-budget enforcement. Declared web actions preview inputs and
  effects and retain the invoking collaborator, but only create a request:
  extension output never becomes a privileged check, comment, merge, release,
  deployment, environment change, or embargo bypass.
  Portfolio initiatives retain an existing proposal, evolution, incident, or
  authorized security source plus ordered cross-repository contributions,
  dependencies, and accountable team/human/approved-agent ownership. Portfolio
  reads derive blockers, policy exceptions, release candidates, and actionable
  reassignment when live membership, agent operation, or stewardship changes.
  Approved agents retain append-only operator-published profile revisions covering tasks, tools, model
  and execution provenance, project-data use/retention, subprocessors and remote boundaries,
  pricing/resources, requested capabilities, availability, support, and change history. Platform-derived
  stable-identity/operator evidence is separate from claims, and profiles grant no credential, installation,
  user identity, or project authority. Authenticated organization agent matching accepts bounded work
  references and explains deterministic workflow fit from live grants, effective policy, deployment,
  cost, availability, evidence freshness, conflicts, verified evaluations, and comparable attributed
  outcomes without copying source content or exposing private evidence publicly. Approved evaluation
  runs become authority only through `/organizations/{id}/agent-participations`, which freeze the
  evaluated profile/run, role, resources, actions, budgets, schedule, data boundaries, exceptions,
  and required operator agreement or human sponsor. Preview is non-effective; activation links a
  distinct `agent-participation:{id}` identity to the existing revocable access-grant boundary.
  Revocation retires that grant and derived credentials. Evaluation, sponsorship, financial limits,
  policy exceptions, and governance standing never confer access independently. Participation-derived
  credentials require explicit `repository.read`/`repository.write` actions and compatible
  `repository_metadata`/`repository_content` boundaries; write is not inferred from role, derivations
  atomically consume the action cap, and lifetime is capped by the agent-minute budget. Pending
  sponsor-required participation supports owner-only, version-guarded reassignment to a current
  member, holds membership across that write, and retains both identities in history. Failed
  activation rollback revokes the provisional grant and all credentials derived from it. Active evaluated-agent
  participations retain privacy-bounded task outcomes, reviewer corrections, verification failures,
  reversions, security/policy violations, accepted contributions, cost, and responsiveness alongside
  periodic reevaluation and material-profile consent. Material model, data-use, requested-capability,
  or price changes suspend active trust pending renewed owner consent; anomaly evidence creates
  actionable notices. Versioned narrowing, suspension, structured replacement handoff, and revocation
  preserve commits and evidence while retiring linked grants and credentials. Deployment compatibility
  uses exact closed profile values (`platform`, `operator_managed`, `customer_managed`, or
  `external_service`); free-form execution prose is explanatory only. An omitted structured boundary
  is missing evidence and fails closed only when a search explicitly requests that boundary. Nested teams retain
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

  Repository and organization owners publish immutable governance charter revisions beneath
  `$CHARTER_STORAGE_ROOT` (default `charters`). Charters explicitly name roles and eligibility,
  decision classes, participation/quorum/approval rules, protected branch/release/environment/
  security/agent resources, terms, removal, succession, and amendment procedures. Approval,
  activation, and expiring exceptions remain separately attributed. Activation recomputes a
  live relationship preview against ownership, collaborators, teams, policies, required checks,
  and agent authority; impossible eligibility or resource rules fail closed, while an amendment
  never rewrites an earlier active revision or completed decision. Role eligibility uses closed
  identity sources (`repository_owner`, `repository_collaborator`, `organization_owner`,
  `organization_member`, `team_maintainer`, `approved_agent`) and is resolved separately for
  every decision class. Repository scope accepts only repository owner/collaborator sources;
  organization scope accepts only organization owner/member, team maintainer, and approved-agent
  sources. Exceptions can name only a class/resource in the current active revision.
  Active charters admit time-bounded human governance standing from closed contribution,
  review, support, ownership, or membership evidence. Invitations bind an exact charter role,
  responsibilities, term, and revision; participants control acceptance, decline, conflict
  recusal, and appeals, while scope owners control suspension, reinstatement, and revocation.
  Reads derive expiry and lost local identity/membership and project nominations plus operational
  access separately. Standing and votes never mint credentials or grant code, secret, merge,
  deployment, or repository authority.
  Charter continuity records bind nomination, election, recall, succession, and emergency
  recovery to an active revision, governed proposal, exact role/resources, review deadline,
  and automatic expiry. They expose unresolved handoffs without minting access; resource roles
  and derived credentials remain separately approved and revoked at their owning boundary.
  Governed community proposals at `/governance` freeze an active charter decision class and
  may originate from technical decisions, initiatives, policy exceptions, funding/resource
  requests, leadership nominations, or charter amendments. They retain public scope,
  alternatives, cited evidence, affected resources, disclosures, discussion/voting deadlines,
  implementation effects, and the declared electorate/quorum/threshold. Eligibility is
  revalidated for every human ballot and again at tally; duplicate ballots, abstentions,
  recusals, changed eligibility, missed deadlines, and contests remain deterministic evidence.
  Approved agents may add cited analysis but never vote. Secret ballots reveal only each
  voter's own ballot/receipt plus aggregate results and a verification digest; other ballots and
  ballot audit events are omitted. Ballot reasons remain retained dissent, the first tally is
  immutable, and every elector must hold active, unexpired standing for the exact charter
  revision and role; source membership alone never suffices. Charter standing validation and
  proposal, ballot, or tally persistence share the charter mutation admission boundary, so a
  concurrent suspension or other standing mutation cannot commit between authorization and the governance write.
  `$GOVERNANCE_STORAGE_ROOT` defaults to `governance`.
  The connected governance browser journey carries a proven contributor from charter standing
  through evidence-backed initiative deliberation, retained dissent and recusal, ordered human-
  and agent-owned task delivery, required checks, independent review, integration, and release.
  It also proves failed quorum, standing appeal/reinstatement, successor election and attributable
  handoff, and relinquished emergency recovery without deriving Git authority from standing or a
  vote. Playwright isolates charter and governance records beneath `$CHARTER_STORAGE_ROOT` and
  `$GOVERNANCE_STORAGE_ROOT` with its other temporary API stores.
  Accepted uncontested results can issue one immutable decision receipt and a repository-owner-
  published ordinary proposal/task plan. The receipt binds charter, result, tally, scope, cost,
  assumptions, and protected effects; it grants no operational authority and materially changed
  bounds require a new or amended decision.
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
  Accessibility commitments at `/repositories/{id}/accessibility` retain immutable
  repository, documented-journey, component, and release contract revisions beneath
  `$ACCESSIBILITY_COMMITMENT_STORAGE_ROOT` (default `accessibility-commitments`).
  They define standards, assistive technology and environment support, audiences,
  required scenarios, severity response, owners, and expiring exceptions while
  projecting missing coverage, conflicts, unsupported environments, and exception
  expiry explicitly; commitment records grant no repository authority or proof of
  conformance.
  The same workspace accepts privacy-bounded lived barrier reports beneath
  `$ACCESSIBILITY_REPORT_STORAGE_ROOT` (default `accessibility-reports`). Reports freeze a release,
  page, documentation journey, or preview revision with functional access needs, steps, expected
  behavior, and explicitly redacted screenshots, recordings, accessibility trees, speech output,
  or input traces. Reporter identity and detailed device settings are projected only by independent
  consent. Current repository participants retain append-only bounded workspace/preview attempts as
  reproducible, intermittent, environment-specific, or unconfirmed; reports grant no execution or
  repository authority.
  Revision-exact accessibility assessments beneath `$ACCESSIBILITY_ASSESSMENT_STORAGE_ROOT`
  (default `accessibility-assessments`) combine repository-defined semantics, keyboard, focus,
  contrast, motion, captions, and journey checks with cited human or repository-bound read-only
  agent findings. The accessibility workspace and matching pull expose coverage, severity,
  audiences, sources, uncertainty, duplicates, false-positive/acceptance decisions, and remaining
  human evaluation. Source/journey changes invalidate only intersecting evidence and decisions;
  agents cannot make acceptance decisions and assessment records grant no operational authority.
  Unattached assessment revisions must be branch-reachable, pull-bound revisions must match the
  live pull source, and every finding citation resolves an associated non-stale preview artifact or
  redacted reproduction-attempt artifact before persistence.
  Branch visibility excludes `vivarium-security/*`; preview freshness is recomputed from the
  authoritative current pull source rather than trusting a stored stale flag.
  Pull-bound assessment creation and preview-cited finding persistence hold
  `pullrequests.WithSourceRevision` across the cross-store write; concurrent synchronization returns
  a conflict, and one finding cannot span preview artifacts from different pulls.
  Accepted current findings can create one finding-specific ordinary proposal/task repair with an
  exact commitment revision, bounded acceptance criteria, collaborator-authored component guidance,
  and only the already-permitted redacted reproduction references frozen into task reasoning. Human
  and agent work continues through existing assignment, workspace/session, contribution, review, and
  merge boundaries. Accessibility task publication requires explicit design/code change and
  interaction/content tradeoff sections; the finding repair projection reports the connected task,
  pull, and only current-source-revision previews without exposing reporter identity or unconsented
  device context.
  Repair creation reserves a stable finding-side recovery identity before proposal publication;
  exact retries finish a pending link after process failure or concurrent post-reservation
  invalidation, and GET retains pending state instead of losing the work association. Proposal
  directory-sync uncertainty propagates through both linked and recovery HTTP responses.
  Accessibility delivery policies beneath `$ACCESSIBILITY_DELIVERY_STORAGE_ROOT` (default
  `accessibility-delivery`) select branches, paths, journeys, or risk classes and require exact-
  revision automated checks, scenario coverage, and minimum acknowledgements from named reviewer or
  participant roles. Pull merge and release readiness distinguish missing, stale, unevaluated,
  failed, and unresolved-barrier evidence. Evaluation invitations require an existing bounded guest
  invitation to the same exact preview and expiry; confirmations and rejections retain independent
  rationale and satisfy only their exact pull or release context. Any live rejection blocks delivery
  even when the role's confirmation minimum is met.
  Release candidates freeze their default-branch and release-range changed-path context; missing
  selection context fails closed. Owner overrides are expiring, preserve dissent, and require
  concrete follow-up work.
  The connected accessibility browser journey carries a privacy-bounded released-journey report
  through safe reproduction, false-positive correction, specialist judgment, assignment-scoped
  agent repair, exact-candidate automation and assistive-technology preview confirmation, stale
  acknowledgement recovery, expiring exception, independent review, merge, release, and retained
  regression evidence. Release readiness re-evaluates its merge revision and keeps pull-candidate
  evidence explicitly stale rather than promoting it implicitly.
  Product experiments at `/repositories/{id}/experiments` retain append-only hypothesis-plan
  revisions sourced from proposals, issues, decisions, pulls, previews, or releases. Success and
  guardrail metrics bind exact permitted product-signal versions; audience eligibility,
  instrumentation gaps, overlapping contracts, and approvals invalidated by changed assumptions
  remain explicit. Discussion and approval are version-bound and grant no rollout, collection,
  release, or deployment authority. `$PRODUCT_EXPERIMENT_STORAGE_ROOT` defaults to
  `product-experiments`. Experiment work links freeze a current plan version to an ordinary
  pull's exact commit and optional proposal/task/session/workspace provenance. They expose
  variant keys, versioned events, exposure rules, privacy classification, removal plan, and
  exact-commit repository checks without granting the experiment or its human/agent owner any
  new authority.
  Repository-owner audience contracts bind reviewed variants to an exact release and freeze
  eligibility, region/organization bounds, deterministic user randomization, mutual exclusion,
  allocation, consent, minimal data, and retention. Assignment retains only salted subject digests;
  stale, conflicting, biased, unauthorized, or release-mismatched admission fails before rollout.
  Live attempts bind the contract to successful exact-release deployments in established environments.
  Compare-and-swap allocation stages stay within its cap; idempotent observations retain exposure,
  samples, uncertainty, cost, consent, data quality, and operational health. Pause, stop, guardrail
  breach, deployment failure, instrumentation loss, sample imbalance, or revoked consent prevents new
  assignment without changing stable prior receipts or discarding attempt evidence. Runs retain their
  launch-time plan revision for guardrail evaluation, and assignment rechecks authoritative deployment
  state under the experiment mutation boundary, failing closed when that state is unhealthy or unreadable.
  Threshold- or stop-bound analyses freeze segment effects, uncertainty, exclusions, guardrails,
  attributed human/agent interpretation, and dissent to an exact plan/run revision. Versioned
  adopt/control/extend/inconclusive outcomes create rollout, rollback, or follow-up work plus
  mandatory retirement of variants, targeting, credentials, and collection; cleanup completes only
  when every required task has a durable evidence link, while aggregate evidence and provenance remain.
  Agent experiment interpretation is admitted only through a current operator of an
  organization-approved agent and retains the human operator as the authenticated writer.
  Permitted data handling at `/repositories/{id}/data` retains complete, immutable commitment
  revisions beneath `$DATA_COMMITMENT_STORAGE_ROOT` (default `data-commitments`). Repository
  participants define categories, subjects, purposes, collection, processing, sharing, retention,
  residency, deletion, consent, and accountable owners across repository, release, extension,
  experiment, and environment scopes. Each commitment links an applicable policy and user-facing
  notice; reads derive attributed ownership gaps, unsupported guarantees, declared conflicts, and
  expiring or expired exceptions. Commitments document permission but grant no repository,
  extension, experiment, deployment, or data-access authority.

  Repository-defined data-flow maps on the same `/repositories/{id}/data` workspace retain exact
  visible code revisions and commitment/data-use versions beneath `$DATA_FLOW_STORAGE_ROOT`
  (default `data-flows`). Typed interaction, interface, package, store, extension, release,
  environment, audience, and external-recipient nodes connect through purpose/category/retained-copy
  edges. Current participants publish declarations; participants and repository-bound read-only
  agents add only bounded repository citations, findings, and uncertainty. Projections expose stale
  analysis, inaccessible dependencies, undeclared flows, and declared/observed differences without
  accepting or projecting restricted payloads, and grant no data or repository authority.
  Anonymous and nonparticipant public-repository readers receive declarations only; bounded
  analyses, citations, attribution, and analysis-derived diagnostics remain limited to current
  participants and repository-bound read-only agents.

  Permitted production-derived observations beneath `$DATA_OBSERVATION_STORAGE_ROOT` (default
  `data-observations`) privately connect digest-addressed aggregate/audit metadata to an exact data
  flow, commitment/use, release, environment, deployment, optional extension installation, and
  derived accountable owners. Closed findings cover undeclared flow, excessive retention, failed
  deletion, consent mismatch, and unexpected recipient; raw values, subject identifiers, payloads,
  and caller-authored evidence summaries are not accepted. Current human collaborators can contain
  use, notify participants, retain a private incident or expiring governed exception, and delegate
  ordinary human- or approved-agent-owned proposal tasks carrying only permitted evidence. These
  records grant no data, extension, environment, review, release, deployment, or repository authority.
  An extension-scoped observation requires that installation to remain active for the repository;
  suspension, quarantine, or removal blocks new signals without erasing retained evidence.

  Runtime privacy policies beneath `$PRIVACY_CHECK_STORAGE_ROOT` (default `privacy-checks`) select
  branches/paths, required collection, consent, minimization, access, retention, export, deletion,
  telemetry, and recipient rules, synthetic journeys, and named current human privacy owners.
  Evidence binds the exact pull revision, matching data-flow version, and existing network-isolated
  preview; production personal data and artifact payloads are rejected, while bounded sanitized
  log/trace/artifact metadata, digests, coverage, and failures remain on the pull. Current runtime
  result summaries and artifact display text are deterministic server-generated projections rather
  than retained caller prose; journey, coverage, and other retained labels use bounded identifiers.
  Runs may name only owner-published policy journeys, and coverage is derived from validated rule
  results rather than retained from callers.
  Current evidence and privacy-owner acknowledgement govern merge and release readiness. Owner exceptions
  are exact-rule scoped, expire within 90 days, retain rationale, and require follow-up work; they
  cannot waive the owner acknowledgement. Pull runs, acknowledgements, and exceptions bind the
  pull identity as well as its revision; release evaluation alone uses the explicit revision-wide
  context.

  Product feedback at `/repositories/{id}/feedback` is a distinct needs channel for a project,

  Governed project funds at `/repositories/{id}/funds` retain named stewards, accepted sources,
  fixed-point currency or credit units, spending and approval rules, eligible recipients, refund
  policy, and public or participant-bounded append-only ledgers beneath `$PROJECT_FUND_STORAGE_ROOT`
  (default `project-funds`). `$PROJECT_FUND_TRUSTED_SOURCES` is an operator-controlled JSON object
  mapping accepted source names to base64 Ed25519 public keys; fund authors cannot define proof
  authorities. Commitments are idempotent pending evidence, and source transfer references and
  proof nonces are consumed atomically across the whole fund store; only a named steward who
  remains a project participant can reconcile verified full or partial completion into available
  value. Failed, revoked, duplicate, stale, or repeated transfers remain non-spendable, and fund
  roles grant no repository or operational authority.
  Outcome funding at `/repositories/{id}/funding` connects those governed funds to an exact
  issue, roadmap outcome, proposal, stewardship opportunity, incident follow-up, or security
  repair beneath the fund store's `outcomes/` boundary. Current participants publish complete,
  versioned contracts with scope, acceptance criteria, evidence, fixed-point budget, deadline,
  contributor eligibility, allocation method, cancellation terms, dependencies, risks,
  conflicts, and budget-balanced milestones. Authenticated permitted readers pledge to the whole
  outcome or a milestone. Scope revisions invalidate active pledges until each backer explicitly
  reconfirms; withdrawal, cancellation, insufficient or aggregate shared-fund unsettled backing,
  overlapping awards, and embargoed work remain attributable replanning diagnostics. Funding never grants task, Git,
  credential, review, acceptance, merge, deployment, or security authority.
  Eligible humans, organization teams, and current operators of organization-approved agents
  submit attributable delivery proposals against an exact funded outcome with approach,
  milestones, cost, dependencies, availability, separately requested access, and relevant work.
  Recipient acceptance is version-bound. Named fund stewards may select one or complementary
  accepted proposals only after eligibility revalidation and an explicit conflict disclosure;
  selection records a durable `delivery_reservation` under the fund mutation boundary against settled available value and
  connected planned milestone tasks. The reservation governs compensation only: selection,
  acceptance, and tasks grant no repository, secret, credential, execution, review, merge,
  environment, deployment, withdrawal, or agent authority.
  Selected delivery execution retains milestone progress, forecast, blockers, agent compute,
  and exact links to established tasks, sessions, workspaces, forks, pulls, checks, previews,
  releases, deployments, and delivery teams. Evidence-backed expenses remain pending until a
  current fund steward approves them against the selection reservation. Stewards can pause,
  resume, record revoked access, approve a bounded budget increase, replace unfinished work's
  recipient after live eligibility validation, or cancel and release only the unspent reservation.
  Overrun, fourteen-day inactivity, failed handoff, revoked access, pause, and cancellation stop
  new spending without deleting prior updates, evidence, expenses, compute, or contributions;
  none of these funding records expands the authority of linked resources or a replacement.
  Inactivity starts at selection even before a first update; paused recipients may continue
  publishing retained work and handoff evidence. A generic resume never clears revoked access,
  while a live-eligibility-validated replacement can. Approved-expense fund/outcome publication
  uses a durable roll-forward journal recovered under the fund lock after interrupted writes.
  Replacement must name a different principal; a no-op reassignment cannot clear revoked access.
  Revocation records the assigned principals, preventing a former recipient from mutating retained
  completed-task history after unfinished work moves to a live validated replacement.
  Selected milestones bind deterministic shares of the original allocation and current project
  participant reviewers. Evidence-bound acceptance, correction, rejection, partial award, dispute,
  dissent, appeal, timeout, withdrawal, payment failure/retry, and refund records move or release
  only the milestone reservation through durable fund/outcome publication. Compensation acceptance
  never supplies code review, merge, release, deployment, credential, or repository authority.
  A paid review ID is the settlement receipt shown in the funding workspace; its matching
  `milestone_award` ledger reference connects recipient credit to the frozen update, authorship,
  delivery evidence, measures, reviewer, and later payment recovery. Terminal and payment-failed
  milestone ownership remains intact when a separate unfinished human or agent task is reassigned;
  later delivery updates cannot reopen those terminal states.

  Reciprocal roadmap learning retains feedback-specific decision, preview, delivery, rejection, and
  measured-outcome updates. Reporters see only updates citing their own feedback, can validate the
  experience, add follow-up evidence, or leave future conversation. Maintainer reviews compare promises
  and observations and retain lessons, dissent, resulting work, and continue/revise/fulfilled/unsupported
  dispositions without erasing opportunity provenance.

  Accepted roadmap outcomes create ordinary ordered human/agent proposal tasks with exact opportunity
  and success-measure provenance. Linked delivery evidence does not imply value: every frozen measure
  needs retained measured success, while failed measures, changed assumptions, unresolved user needs,
  policy conflicts, or an explicit revisit keep completion blocked as `revisit_required`.
  release, documented journey, or preview. Authenticated repository readers submit need,
  desired outcome, frequency, impact, redacted evidence, related issue/experiment links, and
  explicit audience, identity, evidence, contact, and follow-up preferences beneath
  `$FEEDBACK_STORAGE_ROOT` (default `feedback`). Repository participants may discuss submissions;
  organization-private records remain limited to their reporter and current project participants,
  while API projection independently redacts reporter identity, contact, and evidence before reads.
  Product opportunities at `/repositories/{id}/opportunities` retain versioned, explicit syntheses
  of permitted feedback, issues, preview findings, support signals, usage evidence, and experiment
  outcomes beneath `$PRODUCT_OPPORTUNITY_STORAGE_ROOT` (default `product-opportunities`). Exact
  citations distinguish support, contradiction, minority need, and duplicate relationships; reads
  revalidate authoritative source revisions and expose stale or unavailable evidence. Repository
  participants can revise and correct classifications, readers can append challenges, and feedback
  reporters can detach their own citation without rewriting the original submission. Repository-
  scoped read-only agent credentials may create attributed syntheses but receive no correction or
  revision authority from that capability.
  Organization access grants mint that API credential with `purpose: api_read`; omitted purpose
  retains the grant role's established Git credential behavior.

  Repository collaborators define append-only performance contracts at
  `/repositories/{id}/performance` and the public `/repositories/{id}/performance-goals`
  API. Complete revisions name the measured surface, workloads, metric targets and baselines,
  correctness constraints, supported environments, owners, budgets, and project links. Reads
  derive attributable missing-measurement, incomparable-environment, stale-baseline, target-gap,
  and conflicting-target diagnostics beneath `$PERFORMANCE_GOAL_STORAGE_ROOT` (default
  `performance-goals`). Reproducible trials beneath `$PERFORMANCE_EVIDENCE_STORAGE_ROOT` (default
  `performance-evidence`) freeze an exact revision or matching release and retain sanitized
  workload, environment, sampling, timing variance, resources, artifacts, logs, and cost;
  comparisons fail closed when measurement conditions differ.
  Performance investigations reuse retained trials and preserve selected evidence,

  Repository owners can require current exact-revision performance evaluations by target branch,
  changed path, or declared risk class. Merge readiness fails closed on missing, stale,
  incomparable, uncertain, incorrect, or threshold-regressed evidence. Post-integration
  observations bind the same candidate evaluation to its exact release and deployment and compare
  observed trials with the candidate; pause, known-good restore, repair, and decision-revisit
  recommendations grant no merge, agent, or environment authority.
  The connected performance browser journey carries a production latency concern through a versioned goal,
  sanitized reproduction, agent diagnosis with affected-owner confirmation, exact-revision optimization,
  performance-gated review, ordered staging/production promotion, and production validation. It retains an
  uncertain noisy attempt, a correctness-blocked attempt, and a missed production target with containment
  recommendations before a successful retry. Playwright isolates performance goal and evidence records beneath
  `$PERFORMANCE_GOAL_STORAGE_ROOT` and `$PERFORMANCE_EVIDENCE_STORAGE_ROOT`.
  revision-aware code/runtime references, invited owners, cited claims, flame stacks,
  challenges, and confirmations. Newer same-context trials mark claims stale when the
  revision, workload, or environment changes. Short-lived `performance:investigate`
  credentials bind to one selected packet and may cite only its trials and references.
  Ordinary pull optimization evaluations bind a supported investigation, goal, selected baseline,
  and candidate trial to the pull's exact synchronized revision. Public pull reads derive
  confidence, metric/resource/cost changes, and correctness while retaining commands, scenarios,
  authorship, and residual risks; source movement marks earlier evidence stale and grants no
  operational authority.
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
  merge-published immutable revisions linked to the authorizing pull, stable
  page slugs, visibility-filtered search and version selection, policy-owned
  redirects, explicit archives, and exact-publication reader outcomes. Owners
  triage retained feedback into existing issues, proposals, or accountable
  human/agent documentation tasks without granting reporters authority.
  It also retains
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
  Git, workspace, review, or publication authority. Pull-scoped documentation
  reviews freeze changed rendered pages at the exact candidate commit,
  navigation differences against the target, documentation-check evidence,
  affected versions, and explicit gaps. Page/area comments, change requests,
  approvals, and bounded expiring stakeholder invitations retain the page
  content SHA-256; synchronization marks only evidence for changed pages stale
  and grants no repository or publication authority.
  Repository `.vivarium/documentation-checks.json` definitions expand selected
  link, symbol, build, sample, command, and tutorial verification across exact
  source/package/release revision matrices in the ordinary bounded check
  executor. Pull checks retain logs, artifacts, coverage/output-difference
  files, selectors, targets, and dependency digests; generated `docs/*` check
  names can be required through normal branch merge readiness, while changed
  declared paths invalidate only the checks whose evidence names them.
  Matrix target revisions may be omitted so the server freezes the exact
  candidate it archives and executes; an explicit revision must equal that
  candidate. The connected living-documentation browser journey carries a
  contributor-authored code-and-guide proposal through grounded agent evidence,
  version checks, rendered review, ordinary merge and publication, then turns
  an archived-release reader failure into a retained version-specific task,
  repair pull, review, release, and corrected immutable publication. Playwright
  isolates documentation records beneath `$DOCUMENTATION_STORAGE_ROOT`.
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
  Read-only conflict evidence at pull `.../conflict-analysis` and repository
  `/conflict-analysis?source_branch=...&target_branch=...` surfaces the exact
  merge base and sides without updating either reference. Pull analysis may pin
  a retained `candidate_id`; moved live branches make that evidence explicitly
  stale. The projection identifies overlapping files, schema/interface paths,
  declared symbols changed independently on both sides, Git textual conflicts,
  structural collisions, affected required checks, and linked pull/task/proposal/
  discussion/review/acceptance context with repository and change authors as
  participants. Keep semantic findings labeled as detector evidence rather than
  treating them as proof that Git cannot merge the text.
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

  Pull privacy review at `/repositories/{id}/pulls/{pull_id}/privacy-review` requires a candidate
  data-flow `affected_paths` scope covering every changed pull path, then compares exact source
  and target data-flow/commitment revisions and derives collection, purpose, recipient, retention,
  access, and user-control changes plus owner acknowledgement, notice, consent, migration, test,
  and exception requirements. Collaborators and repository-bound read-only agents retain cited
  challenges and mitigations; only human collaborators acknowledge all actions and residual risk.
  Pull synchronization invalidates acceptance, while successor comparison retains earlier review
  history beneath `$PRIVACY_REVIEW_STORAGE_ROOT` (default `privacy-reviews`). Review evidence grants
  no data, repository, approval, or merge authority.

  Product direction at `/repositories/{id}/roadmap` compares exact product-opportunity versions
  against goals, capacity, dependencies, risks, governance decisions, and existing commitments.
  Only human repository participants commit versioned accepted/deferred/rejected outcomes; readers
  discuss tradeoffs and agents propose non-binding scenarios. Accepted outcomes require owners,
  horizons, measures, and sequencing, while every later revision requires an attributed replan
  reason under compare-and-swap. `$ROADMAP_STORAGE_ROOT` defaults to `roadmaps`.

  Accepted roadmap items open revision-exact outcome validation at
  `/repositories/{id}/outcome-validations` as a technical decision, prototype,
  documentation concept, or product experiment. Representative success and
  guardrail measures trace to the frozen opportunity's cited evidence. Expiring
  named preview/research invitations require consent, support later withdrawal, and grant no repository
  access; findings retain accessibility needs, dissent, acceptance, and evidence
  quality. Append-only validate/revise/defer/reject conclusions never erase prior
  roadmap plans. `$OUTCOME_VALIDATION_STORAGE_ROOT` defaults to
  `outcome-validations`.

  Locale coverage at `/repositories/{id}/locales` retains complete versioned plans beneath
  `$LOCALE_PLAN_STORAGE_ROOT` (default `locale-plans`) for repository, product, documentation
  collection, and release scopes. Plans name target languages/regions, fallback, terminology,
  formatting, covered journeys, owners/reviewers, release thresholds, and translatable resources
  bound to exact existing repository commits. Reads derive missing ownership/reviewers,
  unsupported formats, conflicting preferred terminology, and coverage stale against the current
  default branch; plans document support but grant no repository or release authority.

  Quality intent at `/repositories/{id}/quality` and the public
  `/repositories/{id}/quality-plans` collection retains complete compare-and-swap revisions beneath
  `$QUALITY_PLAN_STORAGE_ROOT` (default `quality-plans`). Repository, release, journey, and interface
  scopes connect issue, decision, design, accessibility, privacy, performance, and reliability
  requirements to risks, observable expected behavior, test levels, privacy-safe representative
  data, coverage goals, supported environments, owners, release judges, schedules, thresholds, and
  existing automated or manual evidence references. Missing ownership/evidence, explicit conflicts,
  untestable claims, and exceptions expiring within seven days remain attributable diagnostics.
  Every named owner, judge, and exception grantor must remain a current repository participant;
  plans describe quality intent and grant no check, preview, release, environment, or merge authority.

  Reusable behavior assets at `/repositories/{id}/test-scenarios` retain immutable parameterized
  scenarios beneath `$TEST_SCENARIO_STORAGE_ROOT` (default `test-scenarios`). Each scenario connects
  issue, reproduction, design specification, API contract, documentation, or user-journey rationale
  to an exact repository revision and makes preconditions, actions, assertions, cases, environments,
  and assumptions explicit. Test code and synthetic/anonymized/public fixtures remain ordinary
  exact-branch Git assets: publication verifies their paths and fixture digests at the declared commit,
  and may bind the same commit to an ordinary pull or bounded workspace. Generated cases retain human
  or agent authorship, command, framework, assumptions, and provenance. Fixture contents, credentials,
  production personal data, inaccessible evidence, and operational authority are never copied into the
  scenario record implicitly.
  Publication resolves every rationale ID through its repository-owned issue, replay scenario,
  design proposal, API contract, documentation collection, or quality-plan journey store and scans
  the exact digest-matched fixture blob for credential-shaped content. Scenario citations are admitted
  only when their declared commit equals source-owned revision evidence: issue triage/reproduction/
  repair/delivery history, a design implementation base, a replay scenario, contract/documentation
  revision, or a journey scope's explicit `source_revision`.

  Pull localization reviews retain repository-defined extraction maps and exact-source snapshots
  beneath `$LOCALIZATION_STORAGE_ROOT` (default `localization`). Server-derived stable units carry
  context, screenshots, variables, plural rules, and source locations; pull reads project added,
  changed, removed, reused, and untranslated work per locale. Authenticated repository readers may
  propose translations without repository write access, while source changes supersede only affected
  proposals and preserve all attribution and history. Revision- and version-guarded collaboration
  retains locale claims/handoffs, per-unit discussion, and agent suggestion requests grounded in a
  current locale plan and bounded product context. Suggestions must cite source plus plan/terminology
  evidence and uncertainty; agents cannot decide them, and approve/reject remains with the plan's
  declared human reviewers. Protected or embargoed requests fail closed.
  Locale verification candidates bind an existing named-user preview, current locale-plan version,
  declared journey routes, interface hashes, exact source revision, and exercised translation
  versions. Complete repository-defined suites cover variables, pluralization, formatting,
  terminology, links, layout expansion, bidirectional text, fallback, and localized journeys.
  Translators and preview-invited regional reviewers retain route/unit findings and decisions;
  projections distinguish locale-plan, source, per-unit translation, and per-route interface
  invalidation; every later evidence write revalidates the bound current plan version.
  Locale delivery policies bind the current plan to a branch, audiences, risks, named checks, and
  regional-review minimums across pull merge and release readiness. Owners stage, defer, or withdraw
  one locale at an exact revision without blocking unaffected locales; a deferred or withdrawn
  locale cannot be published until a later attributed staging decision. Policy successors retire
  older plan-version requirements, while a missing successor fails closed. Published application and
  documentation records expose locale, version, revision, plan provenance, and fallback state;
  permitted readers report contextual failures, which participants validate or dismiss and may link
  to bounded human- or approved-agent-owned repair work. These records grant no repository authority.
  The connected localization browser journey carries a changed product string through stale
  translation recovery, grounded agent assistance, human linguistic decisions, exact French and
  Arabic previews, required checks and regional review, an RTL finding with per-locale withdrawal,
  ordinary review/merge, staged publication, and a reader-reported correction linked to a reviewed
  repair and successor locale publication. Playwright isolates `$LOCALE_PLAN_STORAGE_ROOT` and
  `$LOCALIZATION_STORAGE_ROOT` with its other temporary API stores.

  Reliability contracts at `/repositories/{id}/reliability` retain complete append-only service
  objective revisions beneath `$SERVICE_OBJECTIVE_STORAGE_ROOT` (default `service-objectives`).

  API contracts at `/repositories/{id}/api-contracts` retain immutable service-interface revisions
  beneath `$API_CONTRACT_STORAGE_ROOT` (default `api-contracts`). Publication requires an exact
  merged-pull commit, parses the cited definition blob as OpenAPI 3.x or Swagger 2.0 JSON with a
  paths object, and verifies an optional release against that commit. Revisions include
  operations, schemas, errors, authentication modes, environments, limits, owners, stability,
  support and compatibility terms, known gaps, and typed source/release/documentation/data-use links.
  Reads preserve comparison history and explicitly project unreleased implementations, unavailable
  environments/releases, known gaps, and documentation that trails the default branch.
  Exact-version consumer applications beneath the same storage root request only declared environments
  and operation IDs. Producer participants approve a bounded subset with expiry or deny it; owners receive
  one-time, hash-retained, rotatable sandbox secrets. Rotation, revocation, exposure reports, expiry, and
  ownership transfer fail closed, with transfers revoking credentials and resetting consent. The linked
  `/api-contracts/integrations` workspace exercises only synthetic inspected request/response examples,
  frozen quotas, and deterministic failure simulation; approval grants no repository, Git, deployment,
  environment, account, production-data, or production-endpoint authority.
  Approved applications can create application-scoped integration work in human- or agent-owned task,
  session, or workspace modes. Each record freezes the consumer repository commit and a credential-free
  preload of the exact contract definition, SDK/example links, approved operations, and synthetic sandbox
  settings. Linked producer and consumer pulls freeze their source commits and separately owned scenarios;
  immutable results retain bounded sanitized requests, responses, logs, checksum-only artifacts, coverage,
  costs, and authorship. Integration records add review provenance but no Git, check, workspace, or merge authority.
  Application operational surfaces freeze sanitized aggregate availability, p95 latency, quota, error,
  schema-conformance, and usage windows to the exact contract version, provider release, and environment.
  Visibility separates shared, producer-only, and consumer-only evidence. Shared investigations retain a closed
  evidence set, cited failure classification and uncertainty, explicit read-only agent invitations, payload-free
  sandbox reproductions, and human-only immutable issue/proposal handoffs without granting project authority.
  Client handoffs name and retain the exact affected integration-work record; its frozen consumer repository
  determines the governed-work boundary, while provider classifications reject consumer-work associations.
  Contract migrations beneath the contract bind published predecessor/candidate revisions to an existing
  interface evolution plan and discover exact-version applications plus their permitted owners and existing
  consumer work. Ordered stages derive readiness from current application access, exact all-scenario passing
  candidate attestations, bounded (at most 90-day) exceptions, acknowledgement, and latest old-version traffic.
  Revoked access, unresponsive owners, failed or missing tests, expired exceptions, and traffic above the selected
  stage threshold block retirement; migration records grant no Git, task, agent, fork, release, deployment, or
  evolution authority. Records share `$API_CONTRACT_STORAGE_ROOT` beneath `contract-migrations`.
  The connected `api-delivery-journey.spec.ts` browser/API proof carries independently owned producer and consumer
  repositories from reviewed v1 contract and narrow synthetic sandbox access through credential-free agent work,
  reviewed consumer release, sanitized shared diagnosis, breaking v2 publication, failed-then-passing conformance,
  rollout, zero-traffic evidence, and safe v1 retirement. It also proves containment and recovery for scope widening,
  exposed credentials, stale contract documentation, unavailable consumer ownership, and a bounded sunset exception.
  Playwright isolates these records with `$API_CONTRACT_STORAGE_ROOT`.
  Repository participants define repository, release, and environment scopes; user journeys;
  indicators and calculations; measurement windows and targets; dependencies; error budgets;
  severity responses and accountable owners; version-pinned product, performance, accessibility,
  privacy, and release commitment links; and bounded exception policy. Reads keep missing signals
  or ownership, unsupported calculations, conflicting targets, and expiring/expired exceptions
  explicit and attributed. Reliability contracts document expected behavior and response, but grant
  no repository, observability, incident, release, deployment, or environment authority.
  Contract-scoped signal mappings retain exact objective/contract and instrumentation versions for
  sanitized metrics, logs, traces, health checks, support reports, delivery records, Git revisions,
  packages, and dependent services. Append-only evidence windows derive attainment and error-budget
  consumption from aggregates while retaining uncertainty, gaps, and exact delivered-software
  provenance. Mapping/window changes remain explicitly incomparable; anonymous public reads redact
  participant-only source references, credential-shaped evidence is rejected before persistence and
  unsafe legacy references are always redacted. Ratio/availability signals derive percentages, count
  signals retain native counts, and percentile/custom signals require an explicit native value;
  directional native-unit error budgets use deviation from the objective target.
  Authorized repository participants and repository-bound read-only agents open investigations
  against an exact contract/objective plus objective, pull, deployment, or budget-consumption
  trigger. Investigations freeze baseline and affected observation windows, journeys, sanitized
  operational/code evidence, cited hypotheses/comparisons/uncertainty/conclusions, disputes, and
  service/dependency-owner input requests. Reads derive stale evidence, hidden dependency ownership,
  and inconclusive state; conclusions may reference an ordinary issue, incident, decision, or planned
  improvement but grant no authority in those systems. Anonymous reliability reads omit investigations.
  Pull, deployment, release, commit, operational-window, and dependency evidence resolves through its
  repository-scoped source of truth before persistence; the store fails closed without that resolver.
  Read-only agents may open investigations and add cited findings, while only human participants may
  confirm/dispute, request or answer owner input, and publish a concluding resulting-work reference.
  Git citations must be reachable from a non-security branch, and trigger observations must match the
  investigation's exact contract version and objective rather than merely sharing a resource string.
  Repository-owner reliability delivery policies bind exact objective revisions to branch, service,
  environment, journey, and risk selectors. Revision-exact pull, queue, release, and deployment
  impacts project predicted and observed budget effects, dependency failures, required owner
  acknowledgements, active bounded exceptions, and warn/slow/block/pause/rollback actions. Blocking
  and containment effects participate in ordinary pull readiness and queue admission; policies,
  acknowledgements, exceptions, and agent evidence grant no merge, release, or environment authority.
  Current reliability evidence resolves through retained same-objective, same-contract observations
  within the declared measurement-window age and exact delivered resource/revision. Budget
  consumption is derived from that trusted observation; caller values never authorize readiness.
  Reliability improvements convert exactly one cited investigation finding or depleted-budget impact
  into ordinary ordered proposal tasks while freezing objective, baseline/affected observations,
  revisions, dependencies, evidence, and acceptance criteria. Governed release/deployment verification
  derives improvement and budget restoration from a later exact-resource observation; failed measures
  retain containment, rollback, or decision-revisit outcomes, and successful restoration never rewrites
  the original impact. Improvement records grant no Git, review, merge, release, or deployment authority.
  Impact-authorized work freezes the exact trusted depleted observation in its baseline set; another
  same-objective observation cannot replace the evidence that authorized remediation. The reservation
  retains that server-derived authorization observation and its original implementation base, both of
  which remain stable across retries after default-branch movement.
  The reliability record is reserved before proposal persistence; pending records remain readable and
  exact retries reuse the reservation and proposal origin to finish cross-store linking.
  The connected reliability browser journey defines a released checkout objective and dependency,
  corrects a noisy signal, confirms post-deployment budget burn, retains missing dependency evidence
  and a rejected exception, and carries affected-owner plus read-only-agent investigation into
  contained delivery. It then publishes ordinary agent task work with attributed guidance and cost,
  reviews and deploys a failed first repair, and reviews a corrective staged deployment whose exact
  observation restores attainment without erasing prior harm. Playwright isolates
  `$SERVICE_OBJECTIVE_STORAGE_ROOT` with its other temporary API stores. Reliability reasoning IDs
  are compact opaque service-objective identifiers and remain valid when handed to ordinary proposals;
  proposal-sized identifier formatting is not an authorization boundary.

  Incident recovery operations beneath `$RECOVERY_OPERATION_STORAGE_ROOT` (default
  `recovery-operations`) freeze one verified protection capture, estimated loss, independent
  approvals, rollback, and an acyclic dependency-ordered plan. Their shared incident projection
  uses compare-and-swap transitions; rejected approval cannot be resumed, and unmet dependencies,
  failed validation, blockers, and stale writes pause safely. Creators cannot approve their own
  operation. Every frozen validation criterion declares an evidence kind and needs one unique passing
  result whose immutable resource/SHA-256 reference resolves server-side. Incident evidence must
  match the recovery repository and current step or a resource frozen by the selected capture.
  Delegated agent steps grant no repository, environment, credential,
  deployment, destructive-cutover, or return-to-service authority.
  Every mutation revalidates access to the operation's exact repository rather than accepting access
  to another repository in the incident. Revoked collaborators cannot continue, and an approved safe
  resume moves failed or blocked steps to retryable paused state without invalidating already verified
  dependencies; fresh passing evidence remains mandatory. The incident UI stores operation responses
  separately from incident state and exposes pause, resume, rollback, and service-return controls.
  Agents execute only steps assigned to their exact agent identity with explicit delegation; approvals
  remain human, while communications and recovery-wide controls require the human operation creator or
  a current human incident role.
  Recovery commitments at `/repositories/{id}/recovery` retain immutable, compare-and-swap contracts
  beneath `$RECOVERY_COMMITMENT_STORAGE_ROOT` (default `recovery-commitments`). Repository participants
  define survival targets for repositories, packages, artifacts, configuration, collaboration records,
  and deployed service data with owners, dependencies, acceptable loss, restoration time, retention,
  jurisdictions, validation criteria, exclusions, and typed service-objective, environment, incident,
  privacy-rule, and governance links. Reads keep missing ownership, impossible dependency timing,
  unprotected dependencies, and expired or soon-expiring exceptions explicit and attributed. These
  records document continuity intent and grant no repository, deployment, data, incident, privacy, or
  governance authority.
  Protection plans in that workspace bind current commitment targets to exact repository commits or
  governed environment definitions and persist encrypted snapshots beneath
  `$PROTECTION_PLAN_STORAGE_ROOT` (default `protection-plans`). The API projection exposes aggregate
  coverage, freshness inputs, checksums, validation, retention, location, cost, failures, and actors,
  never protected paths, contents, ciphertext, nonces, or environment credentials. Reads revalidate
  AES-GCM integrity, manifest checksums, retention, and source existence; corruption, key loss,
  expiry, or deleted commits/environments cannot remain recoverable. Plans and captures grant no
  routine content, restore, repository, environment, or deployment authority. Captures freeze their
  plan version, resource identities, and freshness interval; never evaluate retained captures through
  successor-plan resources or timing.
  Recovery exercises in the same workspace retain redacted evidence beneath
  `$RECOVERY_EXERCISE_STORAGE_ROOT` (default `recovery-exercises`). Only a current repository
  participant named in the plan's accessor list may launch one from an exact recoverable capture.
  The runner accepts only dependency-ordered typed restore, manifest/dependency integrity, declared
  journey, and manual-confirmation steps inside an ephemeral no-network/no-production-credentials
  environment; it never executes caller shell or writes authoritative state. Reads derive currentness
  against the frozen plan, commitment, capture, and protected source, while preserving timing,
  bounded commands, redacted logs/artifacts, gaps, manual work, achieved objectives, and actor history.
  Restore materializes checksum-verified Git blobs or credential-free governed-environment JSON with
  restrictive permissions. The registered `smoke` journey requires a restored
  `.vivarium/recovery-smoke.json` v1 contract naming a digest-pinned cached container image, restored
  script/ELF entrypoint, bounded arguments/timeout, required zero exit code, and exact stdout SHA-256.
  Application code runs with no network, a read-only source mount, dropped capabilities, no new
  privileges, and CPU/memory/process limits. README/static-only captures and unregistered journeys
  fail. The ephemeral filesystem is removed after evidence persistence.
  Failed or risky exercises carry authenticated, repository-readable investigations that correlate
  exact permitted exercise results, visible commits, releases, protection-plan configuration,
  recovery commitments, dependencies, and accountable owners. Human collaborators and
  repository-bound read-only agents may open investigations and publish citation-bound findings with
  explicit uncertainty, but cannot conclude work, restore data, or mutate protected state through
  those records. Only the current repository owner converts a finding into an ordinary ordered
  human/agent proposal task plan at the exact default-branch base; resulting sessions, workspaces,
  pulls, checks, policy changes, integration, and releases retain their existing authority boundaries.
  An improvement remains open until the owner cites a later successful, current isolated exercise
  for the same scenario, plan, commitment, ordered step contract, and originally failed or cited
  results, whose plan version or protected source changed; the fresh evidence verifies the repair
  without rewriting the original gap. Investigation code citations resolve only commits reachable
  from participant-visible non-`vivarium-security/*` branches. Verification also re-resolves the
  exact proposal reasoning and task IDs, requires every task's current contribution to be merged and
  completed at the task's exact current context revision, and proves each implementation revision
  descends from the frozen base; missing, replaced, obsolete, unfinished, or unrelated governed work
  fails closed.

  Approved-agent evaluation suites beneath `$AGENT_EVALUATION_STORAGE_ROOT` (default
  `agent-evaluations`) freeze sanitized representative scenarios, expected outcomes, public and
  protected checks, budgets, prohibited actions, and human-review criteria to an exact repository
  commit. New collaborative cases retain a non-content provenance reference to an issue, support
  thread, task, incident, decision, or sanitized prior session together with explicit synthetic or
  sanitized inputs, permitted context, rubric, uncertainty, human-judgment boundaries, license, and
  training-use terms. Personal data is not an admitted classification and training is prohibited or
  requires separate explicit consent; evaluation publication never supplies that consent. Public
  projections replace protected source identity with an opaque marker and omit protected prompts,
  inputs, context, outcomes, rubric, checks, uncertainty, judgment, and candidate outputs. Historical
  suites without the collaborative-case fields remain reproducible but successors can adopt the full
  contract. Case publication resolves each source through its repository-owned issue, support,
  proposal-task, incident, decision, or exploratory-session store under the author's current
  repository-participant boundary; the source must belong to the suite repository and its caller-supplied
  decimal revision must equal the record's current version. Prior sessions additionally require their
  explicit audience or creator. Trial evidence binds an exact published agent profile and retains bounded outputs, tool
  actions, digest-addressed artifacts, costs, latency, failures, derived checks, contamination,
  reproducibility, and human decisions. Protected check definitions never appear in suite or run
  projections. Ordinary member projections derive aggregates only from public criteria and omit
  protected contamination state; human organization-owner evaluators alone receive protected-derived
  aggregate status or decide a run, and still receive no protected definitions or result rows. An
  owner-linked agent credential remains non-human and cannot decide protected evidence, define,
  activate, control, or revoke participation, replace its sponsor, or provide sponsor/operator agreement.
  Every evaluation authority manifest disables publish, secret, merge, environment,
  and network access; a suite or favorable decision grants no organization or repository authority.
  Repeated and operator-supplied trials remain explicitly labeled.
  Pull-scoped agent candidates beneath `$AGENT_CANDIDATE_STORAGE_ROOT` (default `agent-candidates`)
  immutably assemble an exact open-pull revision, agent-project behavior-contract revision, component
  digests, and selected evaluation-suite revisions. Isolated run evidence retains bounded network,
  services, tools, cost, and latency together with per-attempt traces, actions, outputs, artifacts,
  evaluator decisions, human corrections, uncertainty, and statistical limits. Pull projections compare
  only suite digests shared with the selected baseline; changed scenario or judge evidence is listed as
  invalidated while unaffected suites remain comparable. Contamination and nondeterminism remain explicit
  and contaminated attempts are excluded from aggregates. Pull movement marks the immutable candidate stale.
  Candidate and run records grant no agent, repository, tool, service, network, merge, release, or deployment authority.
  Exact-candidate collaboration pilots beneath `$AGENT_PILOT_STORAGE_ROOT` (default `agent-pilots`)
  let the human repository owner invite current participants into a maximum-30-day, consent-based trial
  across owner-controlled repositories, roles, task kinds, and explicit `repository.read`, `draft.create`,
  `draft.update`, or `task.comment` actions. Invitees inspect effective access, consent or revoke, delegate
  scoped sessions, guide or stop work, and retain revision-bound observed/expected outcomes and corrections.
  Sessions expose cost, minutes, actions, escalations, unsafe behavior, and policy denials; ungranted or
  authoritative actions become denials rather than effects. Revoked consent, exhausted budgets, unsafe
  behavior, repository-access loss, expiry, or pull movement pauses new work without deleting legitimate
  evidence. A pilot never grants merge, publish, disclosure, release, deployment, secret, environment, or
  authoritative-resource mutation authority and cannot be converted implicitly into durable participation.
  Attested releases beneath `$AGENT_RELEASE_STORAGE_ROOT` (default `agent-releases`) require current candidate
  and accepted-pilot evidence plus approved evaluation, domain review, pilot acceptance, data-policy, and
  resource records, and bind an existing organization agent, exact contract, model/tool versions, roles, and a
  derived attestation. Deployments separately freeze identity, credential scopes, budgets, operator terms, and
  a same-agent rollback release; successors never inherit consent. Versioned signals and narrow, pause,
  rollback, private-finding, or human/agent-repair controls preserve all earlier evidence and contributions.
  Organization participation remains the source of technical authority.
  Approval actors are server-derived, candidate-bound records from five distinct current human participants;
  evaluation approvals resolve authoritative run evidence, while pilot approval and release publication require
  every invitee's current consent and latest exact-candidate feedback to explicitly be `accepted`. Rollback releases must share both
  agent and repository identity. Deployment CAS mutations hold a storage-root advisory lock across processes,
  and failed release-store initialization leaves explicit 503 routes rather than silently removing the API.
  Approval publication rechecks owners and collaborators through current repository participation rather than
  owner-only lookup. Pilot approval and release publication also recheck every invitee's live access to every
  selected repository. Fallback release routes register independently of pilot-store initialization.
  Suite selections freeze the exact scenario-ID manifest; run publication rejects unknown scenarios,
  duplicate attempt identities, negative correction counts, and samples below the declared minimum.
  Evaluator attribution is derived from the authenticated publisher rather than caller prose. Candidate and
  run files are synchronized before rename and their storage directory is synchronized before success.
  Candidate and run publications require caller-stable idempotency keys, derive deterministic identities
  within their pull/candidate scope, and reconcile an identical already-published record after an ambiguous
  post-rename failure; reuse with different content conflicts. Every scenario frozen into a selected suite
  must independently meet the run's minimum sample count, so favorable partial coverage cannot aggregate.
  A storage-root advisory lock serializes reconciliation and publication across API processes, preventing
  conflicting evidence with one retry identity from replacing an independently accepted record.
  The connected agent-adoption browser journey compares two published candidates, contains hidden-check,
  prohibited-action, budget, and operator-outage trial failures, activates only sponsored project-owned
  evaluation evidence, and carries the selected agent through an ordinary proposal task, isolated session,
  stock Git contribution, independent review, repository check, and merge. Its retained outcome, material
  profile successor, failed reevaluation, and credential-free replacement handoff keep claims, evidence,
  authority, cost, code, and human decisions linked without letting consent reactivate retired authority.
  The connected `agent-development-journey.spec.ts` browser/API/stock-Git journey carries a project-defined
  role through reviewed human and agent behavior revisions, protected exact-candidate evaluation and baseline
  comparison, intended-user pilots, independent release approvals, ordinary pull review/merge, attested release,
  bounded project work, production regression, rollback, reproduced model/contract repair, fresh consent and
  reevaluation, and a separately attested safe rollout. Leaked protected evidence, denied merge authority,
  evaluator disagreement, cost containment, model change, and revoked consent remain attributable without
  broadening credentials or rewriting the earlier release trail. Playwright isolates all four agent-development
  ledgers.

  Update it when you change how the apps fit together, not for every change.

- **Conflict reconciliation workspaces** — authorized contributors and maintainers launch one
  durable isolated workspace from pull conflict analysis through
  `/repositories/{id}/pulls/{pull_id}/conflict-workspaces`. A caller-stable launch ID reconciles
  retries. The record freezes the merge base, exact source and target revisions, overlap evidence,
  affected checks, owners, evidence gaps, and publication boundaries; the target revision's reviewed
  `.vivarium/workspace.json` provisions compute, while a local Git bundle exposes complete immutable
  ancestry as `conflict/source` and `conflict/target`. Existing workspace editing, discussion,
  presence, commands, control leases, reconnect, and checkpoints provide the shared surface.
  The compare-and-swap meaning ledger retains questions, answers, and bounded resolutions against
  frozen base/source/target evidence and the current proposed file. Every statement cites an exact
  side, revision, and overlap path; resolutions identify preserved or intentionally changed
  acceptance criteria, design decisions, migrations, or user behaviors and residual uncertainty.
  Apply and undo first persist a recoverable intent, then hold the renewable file-control lease
  across runtime inspection, editing, and finalization. Interrupted retries reconcile whether the
  intended digest is already present before completing deduplicated provenance, keeping human
  authors, agent identities, and human operators separately attributable. Finalization rechecks
  that the workspace is still running and the original principal still holds live file control;
  the live control version must equal the intent's frozen version on every retry. Lifecycle or lease
  revocation leaves the pending intent visible without claiming completion, and a newly issued lease
  for the same principal does not implicitly reauthorize it.
  Accepted checkpoints rejoin ordinary contribution governance through caller-stable publication reservations.
  Every current criterion must pass and each affected owner's latest decision must be accepted; owners can append
  withdrawal or reconsideration without rewriting history. A current destination-repository participant may
  compare-and-swap the verified source branch or create a connected resolution branch and pull. The published
  two-parent commit records both inputs, applied-resolution authors, exact commands, and decisions, then runs the
  frozen affected required checks in the ordinary pull scope. Reviews remain revision-bound and protected queues
  rebuild against the current target. Moved inputs, concurrent or superseded branches, withdrawn approval, and an
  occupied resolution branch persist as actionable publication state without overwriting either contribution.
  The connected `conflict-resolution-journey.spec.ts` browser/API/stock-Git journey carries two independently
  reviewed changes through stale and repeated queue conflict evidence, revision-exact textual and semantic
  explanation, affected-owner and bounded-agent participation, rejected agent advice, failed-first combined
  verification, participant revocation, attributed two-parent publication, fresh review, queue rebuild, and final
  merge. Keep checkpoint requests direct to the API because all six isolated criteria can exceed the frontend
  development proxy timeout; the browser still inspects the retained workspace and ordinary pull result.
  Creators may invite affected current human participants, who explicitly consent, and
  organization-approved agents with separately bounded file/command/lifecycle control. Invitations,
  evidence, or workspace access grant no repository, branch, credential, review, check, merge, or
  publication authority; each side's ordinary source permissions remain decisive.

## LADDER.md

`/LADDER.md` at the repo root is **not part of this repository**. It is placed
there from outside the checkout — as a symlink or a read-only bind mount — and
is gitignored for that reason. Read it for context on what is being built, but
never edit it, `git add` it, or delete it, and do not treat its presence as
uncommitted work.
