# HTTP API contract

## Threat models

`GET /repositories/{id}/threat-models` and `GET
/repositories/{id}/threat-models/{model_id}` return repository-visible immutable analysis with
server-derived `freshness`. `POST /repositories/{id}/threat-models` opens a complete model at the
exact current source revision; `POST .../{model_id}/revisions` publishes a complete compare-and-swap
successor using `expected_version`. Architecture, trust-boundary, and dependency revisions are
server-derived from the authoritative source snapshot; caller values are discarded. Inaccessible
evidence is returned only as `accessible: false`, `kind: restricted_gap`, and its explicit `gap`;
resource identity, revision, and summary are redacted from every retained revision projection.
Historically accessible evidence is reauthorized on each read and receives the same redaction when
its exact source is no longer currently permitted or has no reader-aware resolver.
Permits bind the citation ID, kind, resource, and revision, preventing a current citation from
authorizing reused historical IDs. Events with any currently unpermitted citation return
`content_restricted: true`, a fixed placeholder body, and no evidence IDs.

`POST .../{model_id}/events` accepts `expected_version` and an event of `finding`, `challenge`,
`alternative_comparison`, or `acknowledgement_request`. Every event is bound to that model version
and must cite accessible retained evidence IDs. Human participants and exact repository-bound task
agents may contribute only when their Git credential matches the model's exact source pull task,
source revision, and branch; agents cannot publish revisions or acknowledge. `POST
.../{model_id}/acknowledgements` accepts `model_version` and an owner-authored acknowledgement whose
decision is `acknowledged` or `changes_requested`. Only a named current model owner may act.

## Pull request interface verification

- `GET /repositories/{id}/pulls/{pull_id}/interface-checks` is repository-readable
  and separates evidence for the current pull revision from retained stale checks.
- `POST /repositories/{id}/pulls/{pull_id}/interface-checks` lets a participant
  publish bounded comparison evidence only when its candidate revision, successful
  preview, accepted implemented design revision, and repository check-definition
  digest all resolve exactly.
- `POST /repositories/{id}/pulls/{pull_id}/interface-checks/{check_id}/classifications`
  records one current-revision collaborator decision per difference. Outcomes are
  `intentional_change`, `regression`, or `false_positive` and require a rationale.

These records are review evidence only. Artifact URLs remain governed by their
originating bounded system and no endpoint grants execution or merge authority.

## Production debugging workspaces

- `POST /repositories/{id}/debugging-workspaces` lets a current repository collaborator open a
  durable runtime investigation from an `issue`, `incident`, `support_thread`, `deployment`,
  `service_objective`, `trace`, or `manual_observation`. The server resolves governed triggers and
  requires an existing release and environment; the source and configuration revision must equal the
  release commit. Optional package IDs must have been published from that commit, and an optional
  infrastructure definition version must exist in the same repository.
- Each workspace freezes a bounded observation window, user journey, owners, severity, repository or
  explicit restricted audience, permitted sanitized evidence, and explicit unavailable-context reasons.
  `GET` on the collection or item rechecks repository read access and hides restricted workspaces from
  identities outside their frozen access list.
- `POST .../{workspace_id}/events` compare-and-swaps the workspace version to append an attributable
  status transition or proposed hypothesis. History is never rewritten. These records coordinate
  understanding only and grant no observability, runtime, deployment, environment, Git, or data authority.
- `POST .../{workspace_id}/probes` lets a current participant request logs, traces, a runtime profile,
  state snapshot, or repository-defined dynamic diagnostic pinned to the workspace source revision. The
  request previews its explicit audience, closed data categories, privacy and security transformations,
  retention, sampling, maximum cost/load, and an expiry no more than 24 hours away. The requester is part
  of the audience; restricted-workspace probes cannot widen the workspace audience.
- `POST .../probes/{probe_id}/decision` is limited to a declared affected-environment owner and may deny
  or approve only a narrower policy and earlier expiry. `POST .../actions` accepts an outcome only from the
  requester while that approval is live, retaining timing, collector provenance, transformations, explicit
  gaps, and digest-addressed sanitized artifact metadata—not production payloads. A result with gaps cannot
  claim completion; denied access, overload, partial capture, expiry, or `POST .../revoke` stops collection
  and remains distinguishable. Probe records and their lifecycle history are projected only to their
  requester and explicit audience; workspace roles do not bypass that boundary. Approval grants no general
  observability, environment, credential, or diagnostic authority.
  Privacy controls are ordered from `hash_user_identifiers` through `remove_user_identifiers` to
  `remove_user_data`; security controls are ordered from `detect_secrets` through `redact_secrets` to
  `drop_secret_bearing_records`. Approval may retain or strengthen either control, never weaken it.
- `POST .../{workspace_id}/claims` publishes a hypothesis, exact query, finding, or uncertainty only with one
  or more server-resolved citations. Citation kinds cover selected visible runtime evidence, bounded lines and a
  symbol at the frozen source commit, that commit itself, frozen package/dependency and configuration revisions,
  infrastructure, an exact affected-environment deployment, or a current known issue. Claims project as
  `supported`, `disputed`, `stale`, or `blocked`; responses append support, dispute, or stale evidence without
  rewriting the claim. Inaccessible runtime material is returned as an opaque blocked citation.
- `POST .../owner-requests` asks a named code, service, privacy, or security owner a citation-bound question;
  only that human owner can answer it. `POST .../agent-investigations` delegates selected citations and a bounded
  mandate through a 5-minute to 24-hour `debugging:investigate` credential. The agent endpoint exposes only that
  packet and accepts uncertain, cited claims while running. Human collaborators can guide, pause, resume, or
  revoke the investigation; revocation retires the credential, and initiator repository-access loss fails closed.
  Investigation reads and claim-write responses share the same selected-citation packet projection; neither
  response exposes unselected workspace citations or claims, and nested claim responses citing any unselected
  evidence are omitted rather than returning their citation identifiers or evidence-derived prose.
  Neither citations nor the agent credential confer secret, observability, runtime, Git, deployment, or mutation
  authority.
- `POST .../{workspace_id}/replay-scenarios` minimizes selected permitted citations into an immutable scenario at
  the affected commit. Inputs are digest-only `synthetic` or `privacy_preserving` shapes with declared
  sanitization; repository experiment-command hashes, observable invariants, dependencies, production
  differences, gaps, unsafe effects, and optional parent-scenario refinement remain explicit.
- Launch `POST /workspaces` with source kind `debugging_reproduction`, the debugging workspace ID, replay scenario
  ID, and exact scenario commit. `POST .../replay-scenarios/{scenario_id}/attempts` accepts only outcomes from that
  linked isolated workspace and derives invariant results from repository-defined command exit codes. Attempts
  retain the exact environment definition, definition digest, sanitized output/trace metadata, cost, gaps, and
  production differences. One passing workspace is `demonstrated`; two distinct passing workspaces are
  `reproduced`; mixed outcomes are `nondeterministic`. Unsafe effects prevent launch, while missing dependencies,
  changed revisions, and irreducible conditions cannot be reported as reproduced.
- `POST .../{workspace_id}/repair-work` requires a scenario reproduced in two distinct workspaces and a supported
  cited finding. It freezes the affected revision, acceptance and regression criteria, and human or agent owner into
  an ordinary proposal task; the task grants no new authority. `POST .../repair-work/{work_id}/validation` accepts
  only the linked merged pull, passing scenario plus ordinary check runs at its exact revision, an including release,
  its exact staged deployment, and named passing production signals. A failed measure may invoke the existing
  participant-only pause or known-good rollback boundary, or mark the diagnosis reopened. Investigators and task
  agents gain no review, merge, release, deployment, or environment authority. Publication reserves the repair
  identity before proposal/task creation so stale CAS requests cannot orphan ordinary work. Successful validation
  requires a succeeded deployment, and both scenario and ordinary checks must have run against that deployment's exact
  release commit rather than only the pre-integration pull source. Every check required by the pull target branch must
  be included and passing; caller-selected passing checks cannot hide an omitted, pending, failed, or stale required run.
  Failed-measure actions persist an intent before execution; rollback retries reuse
  the exact existing recovery and post-action record races return an explicit reconciliation-pending response.
  Ordinary direct merges start the target branch's required checks again at the exact merge commit so a later
  release and staged deployment can satisfy this revision-exact validation boundary; canonical succeeded check
  runs and legacy completed runs both require an explicit zero exit code. If required-check policy resolution or
  scheduling fails after the Git merge, the merge response remains explicitly uncertain; retrying the idempotent
  merge boundary reconciles missing exact-commit runs before reporting success.

## Consumer API applications and sandbox

- `POST /repositories/{id}/api-contracts/{contract_id}/applications` registers the authenticated
  consumer against one exact revision, selected available environment IDs, and operation-ID capabilities.
  `GET` on the collection exposes records only to their owner and current producer participants.
- `POST .../{application_id}/decision` lets a current producer participant approve no more than the
  requested capabilities with an expiry, or deny with a reason. `POST .../credentials` returns a
  short-lived secret once to the application owner and revokes any predecessor.
- `POST .../{application_id}/sandbox` accepts that application secret and returns only synthetic inspected
  request/response data, contract quotas, and deterministic `rate_limit`, `timeout`, or `server_error`
  results. Revoke, exposure, and ownership endpoints revoke secrets; ownership transfer also resets consent.
  These credentials authenticate no other API, Git, repository, deployment, environment, or production data.
- `POST|GET .../{application_id}/integration-work` creates and lists human- or agent-owned `task`,
  `session`, or `workspace` adoption records. Creation requires current write participation in the named
  consumer repository and freezes its exact commit together with the registered contract definition,
  SDK/example links, approved operations, and synthetic sandbox configuration. The preload projection
  always reports `credentials_included: false`.
- `POST .../integration-work/{work_id}/candidates` binds an ordinary producer pull and consumer pull at
  their current exact source commits and names producer-owned conformance and consumer-owned test scenarios.
  `POST .../candidates/{candidate_id}/evidence` appends immutable scenario results with sanitized request,
  response, and log summaries, checksum-only artifact metadata, coverage, cost, and human authorship.
  Evidence is capped at 128 KiB, artifact metadata at 32 MiB per referenced artifact, and credential-shaped
  text is rejected. These records add review provenance but grant no Git, check, workspace, or merge authority.
- `POST|GET .../{application_id}/operations/observations` retains aggregate operational windows pinned
  to the application's exact contract revision, declared environment, and an existing exact provider
  release. Windows cover availability, p95 latency, quota rejection, errors, schema conformance, and
  usage with a sanitization statement and `shared`, `producer_only`, or `consumer_only` visibility.
- `POST|GET .../operations/investigations` opens a shared case from visible windows. Explicitly invited
  repository-scoped read-only agents may add evidence-cited, uncertain findings, while humans retain
  sandbox reproduction and immutable ordinary issue/proposal handoff authority. Reproductions retain only
  synthetic status/code and never a payload; handoffs grant no Git, review, task, or merge authority.
  A `client` handoff must name the exact affected `integration_work_id`; the server resolves its frozen
  consumer repository instead of inferring one from other application work. Provider-side findings reject it.
- `POST|GET /repositories/{id}/api-contracts/{contract_id}/migrations` binds published predecessor and
  candidate revisions to an existing provider evolution plan, classifies compatibility changes, discovers
  exact-version applications and their permitted owners/work repositories, and defines ordered evidence and
  traffic thresholds. Producer and affected-consumer projections remain permission-filtered.
- `POST .../migrations/{migration_id}/consumer-actions` records compare-and-swap acknowledgement, an exact
  passing integration candidate attestation, or a reasoned exception expiring within 90 days. `POST
  .../stages` advances policy stages, but retirement fails while any consumer is unacknowledged, lacks a
  passing candidate or active exception, has unavailable access, or exceeds the selected old-version traffic
  threshold. These records coordinate existing evolution and integration work without granting authority.

## Durable-schema execution recovery and retirement

- Execution `report` controls may declare `failed_invariant`, `service_regression`, `conflicting_writes`,
  `capacity_exhaustion`, or `interrupted_backfill` with a safety point and bounded evidence. The execution
  pauses while retaining all phase and attempt history. Revoking a step approval through the migration
  event API does the same and invalidates that approval for later execution admission.
- `POST .../durable-schemas/{schema_id}/migrations/{migration_id}/executions/{execution_id}/recoveries`
  appends an idempotency-keyed `retry`, attested `restore`, existing-release `traffic_rollback`, or `repair`
  linked to already assigned ordinary migration work. Recovery evidence grants no release, traffic,
  workspace, deployment, or data-store authority. A plain retry can resume failed invariants and interrupted
  backfills; service regressions, capacity exhaustion, and conflicting writes require a remediation
  attestation, attested restore, or compatibility-window rollback. A repair link alone remains paused.
- `POST .../retirement-approvals` records one immutable approval from each candidate-schema owner. `POST
  .../completion` succeeds only after production completion and the execution's declared observation period,
  and must account for retained/changed data, verified deletion, exceptions, costs, and the candidate schema
  version across every established environment as well as removed compatibility code, obsolete fields, and
  irreversible decisions. Failed executions and recovery attempts remain unchanged.
- The `/repositories/{id}/durable-state` execution form sends the required
  `observation_period_seconds`; its projection keeps contained failures, recovery actions, owner approvals,
  environment cleanup evidence, and verified deletion visible after completion.

## Approved-agent evaluation

- `GET /organizations/{id}/agent-participations` projects current bounded authority together with
  its reevaluation deadline, consented profile version, privacy-bounded delivered outcomes, structured
  handoffs, and actionable trust notices. `POST .../{participation_id}/outcomes` appends attributable
  task completion, reviewer correction, verification failure, reversion, security/policy violation,
  or accepted-contribution evidence with cost and responsiveness, but accepts no prompt, patch,
  branch, artifact content, or private work body. Failures and violations create explicit notices.
- `POST /organizations/{id}/agent-participations/{participation_id}/controls` lets organization
  owners compare-and-swap suspension, narrowing, renewed material-profile consent, reevaluation, or
  structured replacement handoff. Suspension and handoff revoke the ordinary grant and its derived
  credentials promptly; outcome history and legitimate Git commits are not deleted. Material model,
  data-use, requested-capability, or price changes suspend active participation until explicit consent.
  Reevaluation intervals are bounded to 1–365 days (90 days for older clients).

- `POST|PUT|GET /organizations/{id}/agent-evaluation-suites` lets organization collaborators
  publish compare-and-swap suite revisions bound to an exact commit in an organization repository.
  Each revision retains sanitized representative prompts, expected outcomes, public correctness and
  policy criteria, budgets, prohibited actions, and human-review criteria. Protected checks are
  persisted privately and public projections reveal only redacted result rows.
- `POST /organizations/{id}/agent-evaluation-suites/{suite_id}/runs` records an exact agent profile
  version in an isolated authority boundary with publish, secret, merge, environment, and network
  access all disabled. The server derives correctness, policy, budget, contamination, and
  reproducibility state from bounded outputs, tool actions, digest-addressed artifacts, cost,
  latency, and failure evidence. Repeated and operator-supplied trials are labeled explicitly.
- `GET /organizations/{id}/agent-evaluation-runs` returns retained revision-exact evidence, while
  `POST /organizations/{id}/agent-evaluation-runs/{run_id}/decisions` appends a human evaluator's
  approved, rejected, or needs-work decision. Ordinary member projections recompute correctness and
  policy solely from public criteria and omit protected-derived contamination state. Organization-owner
  evaluators receive protected-derived aggregate status but never protected definitions or result rows;
  only those evaluators may decide a run. Suites, runs, and decisions grant no agent access or
  repository authority. Records default beneath `$AGENT_EVALUATION_STORAGE_ROOT`
  (`agent-evaluations`).

## Runtime privacy checks

- `POST|GET /repositories/{id}/privacy-check-policies` lets repository owners define and readers
  inspect branch/path-selected runtime requirements, synthetic journeys, and named privacy owners.
- `POST /repositories/{id}/pulls/{pull_id}/privacy-check-runs` retains a current-revision run only
  when it names the pull's existing network-none preview and matching data-flow version. The run
  declares `isolation: "ephemeral_network_none"` and `production_data: false`; results cover the
  closed rule set `collection`, `consent`, `minimization`, `access`, `retention`, `export`,
  `deletion`, `telemetry`, and `recipient`. Caller-provided result summaries and artifact display
  text are replaced with deterministic server-generated descriptions before persistence. A run can
  name only an owner-published policy journey, and coverage is derived from its validated rule results
  rather than accepted from the caller. Retained labels use bounded identifier syntax. Artifact metadata is digest-addressed
  and limited to 5 MiB per declared log, trace, or artifact; payload contents are never accepted.
- `POST /repositories/{id}/privacy-check-acknowledgements` records a named privacy owner's rationale
  for an exact policy, revision, and pull context (or the revision-wide release context).
  `POST /repositories/{id}/privacy-check-exceptions` is owner-only
  and requires exact rules, rationale, follow-up work, and an expiry no more than 90 days away.
- `GET /repositories/{id}/pulls/{pull_id}/privacy-readiness` and
  `GET /repositories/{id}/releases/{release_id}/privacy-readiness` distinguish missing, stale,
  failed, and passed current evidence. Pull merge readiness includes the same `privacy_readiness`
  projection and blockers. Pull runs, acknowledgements, and exceptions cannot satisfy a sibling
  pull that happens to share its commit. Exceptions waive only their named runtime rules and do not
  waive a missing owner acknowledgement. Records default beneath `$PRIVACY_CHECK_STORAGE_ROOT`.

## Data commitments

- `GET /repositories/{id}/data-commitments` lists readable repository commitments.
- `GET /repositories/{id}/data-commitments/{commitment_id}` reads one commitment and its history.
- `POST /repositories/{id}/data-commitments` publishes a complete initial revision for a current
  repository participant with `repositories:write`.
- `POST /repositories/{id}/data-commitments/{commitment_id}/revisions` publishes a complete
  successor using `expected_version`; stale writers receive `409 data_commitment_conflict`.

Every revision requires repository/release/extension/experiment/environment scope, at least one
complete data-use declaration and accountable commitment owner, plus both an applicable `policy`
and user-facing `notice` link. Responses retain immutable revision attribution and derive
diagnostics for missing per-use ownership, unsupported guarantees, declared conflicts, and
expiring or expired exceptions.

### Data-flow evidence

- `GET /repositories/{id}/data-flows` and `GET .../data-flows/{flow_id}` return readable maps,
  immutable declaration history, bounded analyses, and derived diagnostics.
- `POST /repositories/{id}/data-flows` publishes a participant-defined map at a 40-character commit
  reachable from a visible branch. Every map and edge references exact same-repository commitment
  versions and existing data-use IDs; `affected_paths` declares its exact source coverage.
- `POST .../data-flows/{flow_id}/revisions` publishes a complete successor with
  `expected_version`; stale writers receive `409 data_flow_conflict`.
- `POST .../data-flows/{flow_id}/analyses` lets current participants and repository-bound read-only
  agents add bounded findings at the map's exact code revision. Findings use closed kinds
  (`confirmed`, `undeclared_flow`, `declared_observed_difference`, `inaccessible_dependency`, or
  `uncertainty`) and cite repository paths and line ranges. The schema accepts claims and references,
  not captured data values or artifacts.

Node kinds are `interaction`, `interface`, `package`, `store`, `extension`, `release`,
`environment`, `audience`, and `external_recipient`. An inaccessible node requires an uncertainty
statement. Diagnostics report inaccessible dependencies, uncertainty, undeclared or differing
behavior, and analyses made stale by a successor declaration; they never broaden source visibility.
Anonymous and nonparticipant readers of public repositories receive declarations and declaration
diagnostics only. Complete analyses, citations, actor attribution, and analysis-derived diagnostics
are projected only to current repository participants and repository-bound read-only agents.

### Production data-use observations

- `POST /repositories/{id}/data-observations` accepts a permitted production-derived signal from a
  current participant or repository-bound read-only agent. Closed signal kinds are `undeclared_flow`,
  `excessive_retention`, `failed_deletion`, `consent_mismatch`, and `unexpected_recipient`.
- `GET /repositories/{id}/data-observations` and `GET .../{observation_id}` are private to current
  repository participants. `POST .../{observation_id}/actions` retains containment, notification,
  private-incident coordination, or an expiring governed exception with compare-and-swap protection.
- `POST .../{observation_id}/repair` freezes current sanitized evidence into one ordinary proposal
  task assigned to a current human participant or an approved agent operated by the collaborator.

Evidence contains only a closed kind, SHA-256 digest, at-most-31-day window, and aggregate sample count.
Captured values, subject identifiers, raw payload, and caller-authored evidence summaries are not
accepted. Exact flow, commitment/use, release, environment, deployment, optional extension installation,
and derived owners must resolve together. Records grant no operational authority and default beneath
`$DATA_OBSERVATION_STORAGE_ROOT` (`data-observations`).

### Pull privacy review

- `GET /repositories/{id}/pulls/{pull_id}/privacy-review` returns the exact-revision comparison,
  required actions, attributed discussion, acceptance state, and superseded history.
- `POST .../privacy-review` compares source and target data-flow versions that match the current pull
  commits. The candidate flow must cover every changed pull path. Classifications and follow-up
  requirements are derived from flow and commitment differences rather than accepted from the client.
- `POST .../privacy-review/comments` retains collaborator or repository-bound agent challenges,
  mitigations, and residual risk with optional citations into the reviewed source revision.
- `POST .../privacy-review/acceptance` is human-collaborator-only, requires every derived action and
  a residual-risk statement, and conflicts after the pull source revision moves.

## Product feedback

`POST /repositories/{id}/feedback` accepts an authenticated repository reader's project, release,
documented-journey, or preview feedback. The body names `target`, `need`, `desired_outcome`,
`frequency`, `impact`, `audience`, `identity_visibility`, `contact_preference`, optional contact,
redacted evidence, and same-repository issue or experiment links. Evidence must explicitly set
`redacted: true` and choose `audience`, `maintainers`, or `reporter_only` visibility. Release and
related-resource ownership is validated before persistence.

`GET /repositories/{id}/feedback` and `GET .../feedback/{feedback_id}` project records for the
authenticated viewer. `organization_private` records require an organization repository and are
limited to the reporter and current project participants. Identity, direct contact, reporter
attribution in history/discussion, and every evidence item are independently redacted according to
the submission's consent. The reporter or a current participant appends discussion through `POST
.../feedback/{feedback_id}/comments`; feedback history is append-only.

## Product experiment contracts

Repository participants use `POST /repositories/{id}/product-experiments` to open a plan from a
`proposal`, `issue`, `decision`, `pull_request`, `preview`, or `release`. The request contains
`source`, a complete `revision`, and `signals`. Revisions require a hypothesis, at least two
variants, target audience, success and guardrail metrics, minimum evidence, duration, owners, stop
conditions, assumptions, and rationale. Metrics bind an exact signal ID/version; signals declare
their event, unit, privacy boundary, and `available`, `planned`, or `retired` status.

`GET /repositories/{id}/product-experiments` lists plans and live diagnostics. Successor revisions
are posted at `/{experiment_id}/revisions` with `expected_version`; discussion is appended at
`/comments`, and version-bound `approve` or `request_changes` decisions at `/approvals`. Stale
versions return `409`. Missing instrumentation, ineligible audiences, overlapping experiments,
and changed assumptions remain attributable diagnostics. Plans grant no exposure, collection,
release, or deployment authority.

Repository owners approve rollout admission at `POST
/repositories/{id}/product-experiments/{experiment_id}/audience-contracts`. It binds the current
plan and reviewed implementation to an exact release/commit and freezes eligibility, exclusions,
regions/organizations, deterministic user randomization, mutual exclusion, basis-point allocation,
consent, allowlisted minimal data, and 1–730 day retention. Stale, conflicting, over-allocated,
unknown-data, or release-mismatched contracts fail before rollout. Assignment receipts are stable
and audit only a contract-salted subject digest, variant or exclusion reason, and time.

Collaborators attach completed implementation to the current plan with `POST
/repositories/{id}/product-experiments/{experiment_id}/work`. The request freezes the plan
version, declared variant keys, human or approved-agent task owner, optional ordinary
proposal/task/session/workspace identities, ordinary pull ID, and its exact source commit. It
also retains versioned event definitions, exposure/assignment rules, privacy classification,
disable/removal plan, and repository check names. Every named check must already exist for that
exact pull commit, and every supplied execution identity must match the pull's ordinary
provenance. The link is review evidence only: existing repository permissions, reviews,
required checks, merge policy, credentials, and deployment authority remain unchanged.

Authorized repository participants launch an approved contract with `POST
.../{experiment_id}/runs`, naming successful deployment IDs for the contract's exact release and
an initial allocation no larger than its approved cap. `POST .../runs/{run_id}/stages` appends a
compare-and-swap allocation stage and reason; it never changes prior assignment receipts. Pause,
resume, and stop use `/controls`. Bounded idempotent `/observations` retain per-variant exposure,
metric values and sample counts, uncertainty, cost, instrumentation, consent, deployment health,
sample balance, and operational notes. Unsafe evidence atomically contains the attempt and rejects
new assignment while retaining earlier attempts and stable assignments. Guardrails remain frozen to
the run's launch-time plan revision. Every new assignment rechecks all launched deployments and
atomically contains the attempt if any is no longer successful or its health cannot be read.

Analysis at `POST .../{experiment_id}/analyses` freezes a plan/run revision, segment effects,
uncertainty, exclusions, guardrail outcomes, interpretation, and dissent after minimum evidence or
a stop condition. Human interpretation is attributed to the authenticated participant. An
`interpreted_by_type` of `agent` is accepted only when that participant currently operates the named
organization-approved agent for the repository; the record retains both the agent interpreter and
human mutation actor. Outcomes and cleanup remain human-authenticated decisions and ordinary work.

## Governed community proposals

`POST /repositories/{id}/charter/continuity` and the organization equivalent record a pending
charter-bound nomination, election, recall, succession, or emergency action. The request binds
the active charter version, governed proposal, declared role and protected resources,
predecessor/successor standing when required, review time, and expiry. `POST
.../continuity/{action-id}/actions` approves, completes, relinquishes, or appeals the retained
record. Reads derive expiration and review-due state. Continuity records never mint resource
access; independently owned credentials and roles must be approved and revoked separately.

`POST /governance/proposals` opens a proposal beneath an active repository or organization
charter. It supplies the source, public scope, at least two alternatives, cited evidence,
affected resources, disclosure requirements, implementation effects, and a `rule` naming the
active decision class and voting window. The server freezes the active charter version and its
authoritative eligible roles, quorum, and threshold; the opener must hold active, unexpired
standing for an eligible role under that exact revision. Repository or organization membership
alone is not governance standing.
Creation, ballot admission, and final tally hold the charter mutation boundary from exact
standing validation through governance persistence; a concurrent standing or active-revision
change therefore commits wholly before or after the governed write, never between them.

`GET /governance/proposals` lists currently visible proposals and `GET
/governance/proposals/{id}` returns one record. `POST .../{id}/analysis` appends required-citation
human or approved-agent analysis. Agent analysis requires a current operator and grants no vote.
`POST .../{id}/ballots` accepts one final alternative, `abstain`, or `recuse` choice per eligible
human. Changed charters, lost eligibility, duplicates, and out-of-window ballots fail closed.
Secret responses show only the caller's own ballot and receipt; every other ballot and ballot
audit event is omitted, including its existence, voter, choice, reason, cast time, and receipt.

After the deadline, `POST .../{id}/tally` re-resolves the exact allowed roles, excludes ballots
whose eligibility was lost, and computes participation, abstentions, recusals, counts, quorum,
threshold, and result. Optional `contest_reasons` and charter drift mark a result contested. The
tally exposes a SHA-256 digest over the proposal, charter version, current electorate, receipts,
and aggregate counts for verification. The first completed tally is immutable; later finalization
attempts conflict instead of replacing its electorate, contest evidence, result, or digest.

An accepted, uncontested result can be carried into accountable project work with `POST
/governance/proposals/{id}/implementation`. A current repository owner supplies the exact base
revision, bounded scope and cost, explicit assumptions and protected effects, and an ordinary
ordered task plan. The immutable decision receipt binds those bounds to the charter, result, and
tally. Publication creates a normal proposal and tasks; it grants no Git, review, integration,
release, environment, extension, or agent authority. Exact retries converge, while changed scope,
cost, assumptions, or protected effects return `governance_amendment_required`.

## Federated repository discovery

`GET /federation/repositories/{id}` is the authoritative public metadata
projection for a local public repository. Its response wraps a bounded snapshot
and the current identity-document version/key in an Ed25519 signature. The
snapshot includes its instance-qualified reference, exact revision, branches,
and permitted releases, contributor guidance, public issues, and open
opportunities. Embargoed `vivarium-security/*` branches are excluded. Absent
sections are distinguishable through `capabilities`. Producer and consumer
enforce the same 16 MiB aggregate response maximum; an authoritative projection
that exceeds it returns `413`, while a peer exceeding it is rejected explicitly.

Authenticated home-instance clients resolve a trusted peer reference with
`POST /federation/repositories/resolve` and `{"reference":"peer-id:repo-id"}`.
`GET /federation/repositories/{peer}/{repository}` reads the retained snapshot;
`POST .../refresh` revalidates it. A failed refresh returns `202` with explicit
status and preserves only an already-permitted stale snapshot. `PUT
.../follow` with `{"follow":true|false}` changes local follow state, and `GET
/federation/follows` returns followed repositories with current cache evidence.
These endpoints never create a local repository or confer repository authority.

## External extension registration

Authenticated developers register an external collaborator with `POST
/extensions`. The contract names the extension and operator contact, declared
capabilities, callback and action HTTPS endpoints, requested resource/action
permissions, supported event types, and a credential rotation interval plus
optional overlap. Endpoints must use HTTPS and resolve exclusively to publicly
routable addresses.

Before persistence the API sends a unique value in
`Vivarium-Extension-Challenge` to both endpoints with `GET`; each must return a
successful response that echoes the exact value in the same response header.
Verification rejects redirects, protected network ranges, and mixed public and
private DNS answers, and pins the validated addresses while dialing to prevent
DNS rebinding.
The resulting record has an independently generated ID and
`principal_type: "extension"`. It contains the verification times and an
authority preview whose effective actions are empty and whose decisions are
`not_installed`. Registration issues no bearer credential and never represents
the extension as its installer, another user, or an approved agent.

`GET /extensions` lists identities owned by the authenticated developer, while
`GET /extensions/{id}` lets an authenticated prospective installer inspect its
verified identity and requested contract. Records persist beneath
`$EXTENSION_STORAGE_ROOT` (`extensions` by default). A later scoped installation
workflow is required before any collaborative context or resource authority is
available.

### Extension installations

`POST /extensions/{id}/installations` accepts `owner_type`, `owner_id`, exact
`repository_ids`, requested `resource_types`, an approved or denied decision
for every declared capability, and optional non-secret string `settings`.
Repository installations name exactly the owned repository; organization
installations may name only repositories currently in that organization.
Secret-, token-, or password-named settings are rejected.

`GET /extension-installations` lists installations the actor currently owns;
`GET /extension-installations/{id}` returns effective access and actor history.
`POST /extension-installations/{id}/{action}` supports `upgrade`, `suspend`,
`resume`, `quarantine`, `transfer`, and `remove` with the current `version`; upgrade and
transfer also include an `installation` object. Stale writes return `409`, and
every mutation revalidates current ownership. Suspension and removal revoke
only derived credentials before publishing status, without deleting retained
events or attributed evidence.

Extension operators publish a newly endpoint-verified declaration with `POST
/extensions/{id}/contract` and the current contract `version`. Installations
remain pinned until each resource owner explicitly submits an `upgrade`.
`GET /extension-installations/{id}/operations` projects requests, actions,
delivery failures/latency, consumption, permission use, credential health,
contract drift, and notices. The `/credentials/rotate` action replaces and
retires credentials; quarantine revokes future authority without deleting evidence.
Rotation durably shortens predecessor expiry to the configured overlap deadline
(or revokes immediately for zero overlap), never extending original lifetime.
The installation atomically publishes its successor and, at zero overlap,
detaches predecessors before retiring their auth records. Positive-overlap
retirement failures detach the affected predecessor, preserving successor
continuity without retaining unintended contribution authority.

### Extension contributions and actions

An installation owner mints a one-day-or-shorter credential with `POST
/extension-installations/{id}/credentials`, supplying the current installation
`version` and `expires_in`. The returned `extensions:contribute` bearer secret
is recorded as derived from that exact installation and is revoked by its
suspension or removal.

The extension publishes to `POST /extension-contributions` with that bearer
credential and `Vivarium-Installation-ID`. A contribution names an exact
repository, supported resource type and ID, current 40-character revision,
stable `idempotency_key`, and one of `status`, `check`, `annotation`, `artifact`,
`link`, `comment`, or `action`. The API validates installation status,
credential derivation, resource scope, the live pull revision, bounded payload
and rate budgets, and declared action inputs/effects. Exact retries return the
original record; conflicting keys and stale revisions fail closed. These
records do not write privileged check, comment, merge, release, deployment,
environment, or policy stores.

Collaborators read pull contributions at `GET
/repositories/{id}/extension-contributions/pull_requests/{pull-id}` and invoke a
declared `/actions/{action-id}` child with the current `revision` and declared
inputs. The durable request retains its human actor and previewed effects but
queues no privileged operation.

## Documentation verification

Pull creation and synchronization read optional
`.vivarium/documentation-checks.json` version 1 at the exact candidate commit.
Its `checks` contain `name`, `collection_id`, bounded executor fields (`image`,
`command`, optional working directory/environment/resources), non-empty
`selectors`, `dependency_paths`, and `targets`. Each target has a display
`version`, optional exact 40-character candidate `revision`, and `source` of
`source`, `package`, or `release`. When `revision` is omitted the API freezes
the exact candidate it archives and executes; when supplied it must equal that
candidate. The API expands targets into normal check runs named
`docs/{name} [{version}]`; `definition.documentation` exposes collection,
matrix target, selectors, dependency paths, and their deterministic SHA-256.
Existing pull check event and artifact endpoints retain execution evidence, and
existing required-check configuration and merge-readiness endpoints govern a
generated documentation check name without special authority.

## Documentation collections

- `GET /repositories/{id}/documentation` lists current visible collections.
- `GET /repositories/{id}/documentation/{collection-id}` returns current
  health and immutable revision history.
- `PUT /repositories/{id}/documentation/{collection-id}` owner-publishes a
  compare-and-swap revision; use `new` with `expected_version: 0` to create.

Publication resolves `source_ref` to an exact commit and derives `.md`, `.mdx`,
and `.adoc` pages beneath `root_path`. Responses retain source object, hash, and
authorship evidence and report `missing_owner`, `broken_source`, `stale_source`,
and `stale_version_mapping` diagnostics.

Authorized repository participants open evidence-bound writing work with `POST
/repositories/{id}/documentation-tasks`. A task freezes a proposal, issue, pull
request, release, investigation, or stewardship opportunity to an exact commit
and reserves `docs/tasks/{task-id}` without granting new Git authority. Draft
and entry endpoints retain compare-and-swap rendered revisions, exact-revision
references, attributable discussion and suggestions, and identified agent
assistance that must cite sources and explicitly marks uncertainty. Ordinary
repository permissions govern all operations; collection publication remains
owner-controlled.

## Documentation pull reviews

`POST /repositories/{id}/pulls/{pull-id}/documentation-review` freezes changed
reader-facing pages from an exact candidate commit and compares navigation with
the target. `GET` returns rendered pages, documentation-check evidence and
artifacts, affected versions, declared gaps, and content-sensitive review
history. The `/entries` endpoint retains exact-page comments, change requests,
and bounded preview feedback; `/decisions` retains technical, audience,
navigation, example, or version approval at the page SHA-256.

Repository owners use `/invitations` to grant expiring `view`, `feedback`, or
`review` roles for named areas. Invitations grant no repository, Git, merge, or
publication authority. When the source advances, each retained SHA-256 is
compared with the new candidate: only evidence for changed pages becomes
`stale`, and new writes against those pages require a refreshed snapshot.

## Preview acceptance requirements

Repository owners define the acceptance gate for a target branch with `GET`
and `PUT /repositories/{id}/branches/{branch}/preview-acceptance`. A complete
replacement contains `requirements`; each requirement has a unique `id`,
optional path globs and risk-class labels, and one or more named `scenarios`.
Every scenario names the participant role (`owner`, `contributor`, `author`, or
`stakeholder`)
and whether it blocks merge. Risk-class requirements apply to the selected
target branch and cannot be evaded by omitting a caller-supplied classification;
path requirements apply only when the pull changes a matching path.

`GET /repositories/{id}/pulls/{pull-id}/preview-acceptance` derives the policy
and findings that apply to the adopted source revision. `POST .../preview-acceptance/decisions`
appends an attributable `accepted`, `rejected`, or `overridden` decision naming
the exact revision, requirement, scenario, actor's live role, and a stable
`idempotency_key`. Exact retries return the original decision; conflicting reuse
returns `409`, while post-publication directory-sync uncertainty returns the
stable decision with `202` and `Vivarium-Durability: uncertain`. Rejection and
override require a rationale; rejection remains blocking despite later ordinary
acceptance, and only a repository-owner override can clear it. A source
synchronization or policy replacement never rewrites evidence: prior decisions
move to `stale_decisions` and the new revision or policy version requires fresh
acceptance.

The ordinary merge-readiness response includes `preview_acceptance`. Blocking
scenarios without current acceptance, current rejection, and unresolved
`blocking` preview findings add readiness blockers alongside reviews and
required checks. Both direct merge and integration-queue admission recompute
this same report. The queue also rechecks it immediately before landing and
durably pauses an invalidated entry, so an older preview decision or newly
blocking finding cannot authorize a queued commit.

The `stakeholder` role requires a live `feedback` invitation to a preview of
the pull's exact current revision. Expiry, revocation, or source movement
removes decision authority immediately; retained earlier decisions become stale
evidence and grant no source, log, credential, or repository access.
Merge readiness revalidates that invitation on every read and again through the
ordinary direct-merge or queue-landing readiness boundary. Invitation
publication and final Git publication share one admission lock, so revocation
commits wholly before readiness or after the authorized merge effect.

## Change preview audiences

Version 1 `.vivarium/preview.json` requires `access` with `network: "none"`,
`data: "preview_artifacts"`, `identity: "named_users"`, and unique `actions`
selected from `view`, `test`, and `feedback`. Validation happens before build;
served content also forbids connections, forms, framing, and referrer leakage.

Only the repository owner manages guests. `POST
/repositories/{id}/pulls/{pull-id}/previews/{preview-id}/invitations` accepts a
role, an `expires_at` within 30 days, and either a `user_id` with `source_kind:
"user"` or a `source_id` whose kind is `issue`, `decision`, or `proposal`.
Resource sources expand current attributable participants into named
invitations. `DELETE .../invitations/{invitation-id}` revokes immediately, and
`GET .../audience` exposes effective access and retained audit to repository
participants. A feedback guest may submit a bounded attributable note to `POST
.../{preview-id}/feedback`.

`GET .../{preview-id}/findings` is available only through the same current
repository-participant or active-invitation admission and returns the frozen
preview revision plus its audience-scoped evidence policy. A feedback-role
participant creates a finding with `POST .../findings`, naming an absolute
preview `route`, title, classification (`bug`, `usability`, `accessibility`,
`content`, `performance`, `question`, or `other`), severity (`blocking`,
`major`, `minor`, or `note`), reproduction steps, and an optional visible
`duplicate_of`. Evidence data is base64 and limited to 12 items, 5 MiB each and
12 MiB total: PNG/JPEG/WebP screenshots, WebM/MP4 recordings, text console
output, JSON/text traces, and JSON/text annotations. The server derives byte
sizes and redacts credential-like fields in text evidence and narrative fields
before persistence.

`POST .../findings/{finding-id}/comments` appends audience-scoped discussion.
`POST .../findings/{finding-id}/decision` classifies, relates, resolves, or
reopens with the last observed `version`; stale or invalid decisions return
`409 preview_finding_changed`. All actions retain actor and time. Evidence is
never projected through public preview lists or ordinary pull comments, so an
invitation cannot implicitly broaden inaccessible material.
The findings response also includes a guest-safe `preview` projection and
`effective_role` for the exact web feedback workspace. Invitation records,
audience audit, ordinary feedback, source, and build logs are omitted from that
projection.

An active guest can fetch only that preview's static content with their account
session. Invitations are not repository participation and grant no credential,
source or build-log visibility, workspace or environment access, deployment or
production action, or private-service connectivity.

A current repository owner or collaborator with write scope converts a current
finding into guided implementation with `POST
.../{preview-id}/findings/{finding-id}/repair`, supplying the finding `version`
and one to 20 `acceptance_criteria`. The pull must still be open at the exact
preview revision. The retry-safe response links an ordinary pull change session
whose immutable `preview_evidence` freezes the observation, redacted permitted
artifacts, discussion, reproduction, authors, revision, and criteria. Preview
access alone cannot call this endpoint and the session grants no new authority;
collaborators may work normally or launch the existing repository/branch-bound
agent run. Publishing that run synchronizes the pull through the normal path,
starts checks and a fresh preview build, and back-links its commit and preview
attempt to the original finding. The repair session/finding pair reserves that
attempt identity before its check exists, so completion or recovery retries
reuse the same active or terminal build rather than duplicate provenance. The
reservation and build-run attachment share a filesystem admission lock across
API processes configured with the same preview storage root. Check-run creation
separately serializes its commit/definition-name scan and write across processes,
preventing an unlinked losing run before attachment.

## Prospective impact assessments

`POST /repositories/{id}/impact-assessments` creates a durable assessment for a
current repository participant. The request supplies `title`, `ref`, and one
source: `selected_code` (`path`, `start_line`, `end_line`),
`investigation_conclusion` (`explanation_id`, `entry_id`), or `proposed_diff`
(`diff`). An optional `query` narrows the bounded lexical analysis. The response
retains the resolved revision, completeness, and visible references, tests,
owners, interfaces, consumers, releases, packages, and environments.

Collection and detail reads return records the actor participates in or must
acknowledge. Cross-repository evidence is filtered through current visibility;
requested owners who are not participants never receive the proposed diff.
Participants mutate with the last observed `version` using nested `POST`
`/participants`, `/items`, and `/acknowledgement-requests`. The named owner uses
`POST .../acknowledgement-requests/{request-id}`. Requests are accepted only
for repositories named by derived consumer evidence; acknowledgement also
requires current read access to the source repository. Stale versions return
`409 assessment_changed`, and a moved conclusion ref returns `409
conclusion_revision_changed`.

`POST /repositories/{id}/impact-assessments/{assessment-id}/implementation`
accepts the current assessment `version`, a proposal title/body, one or more
`item_ids`, and one to 20 ordered tasks. Each task names a human or generated
agent owner and may depend on its predecessor. Human owners must still be
repository participants. The proposal, tasks, assignments, and immutable
reasoning snapshot are created atomically; retries converge on the assessment
identity. The snapshot retains the assessment version and revision, selected
claim/risk/verification items, investigation conclusion identity, analysis
status, and owner acknowledgements. A moved selected ref returns `409
assessment_context_changed`; existing history remains readable with
`context_state: changed` and must be rerun rather than rewritten.
Implementation publication holds the selected Git reference lock across atomic
proposal/task creation and assessment linking, so a concurrent stock push
cannot cross the revision-validation boundary. If proposal persistence is
visible but directory durability is uncertain, or assessment linking needs
reconciliation, the endpoint returns `202` with the stable proposal and task
identities plus a recovery instruction; it never reports persisted work as an
invalid request. When linking succeeded, clients confirm the returned stable
identity through a fresh assessment read. If linking is absent, clients reload
and resubmit with the freshly read assessment version. An exact replay carrying
the pre-link version is also recognized only when its
proposal text, selected item order, ordered task definitions, ownership, and
dependencies match the immutable linked implementation; it returns those same
identities with `recovered: true`. Any changed stale payload remains invalid.
Generated-agent creation may omit an assignee ID for server allocation, but an
explicit agent ID on recovery must exactly match the retained assignment.

Task-scoped workspaces and agent change sessions copy this reasoning snapshot
into their launch context. Ordinary task contribution endpoints continue to
publish linked pulls; proposal and pull reads expose the stable assessment,
revision, selected items, and acknowledgement trail for reviewers.

The JSON API is the supported application boundary for browsers, agents, and
external consumers. Durable files beneath the configured storage roots are
private implementation details. Git clients use the smart HTTP URLs returned
by repository resources.

## Repository code navigation

`GET /repositories/{id}/code-navigation?ref={branch-or-commit}&q={query}` uses
the repository read boundary. `ref` resolves once and `revision` is the exact
commit used for every result and blame record. Results classify definitions,
references, callers, and tests with source locations and last-change commits;
ownership lists the catalog owner and collaborators. Dependencies are included
only when declared at that commit and their provider remains readable.
`analysis.status` becomes `incomplete` whenever file, byte, or result bounds
prevent full lexical coverage, with counters and a reason returned explicitly.

## Collaborative code investigations

`POST /repositories/{id}/explanations` requires a current repository participant
and accepts `question`, `ref`, and a context with `kind` set to `repository`,
`file`, `proposal`, `task`, `pull_request`, `incident`, or `workspace`.
Resource contexts also carry `resource_id`; task IDs use
`{proposal-id}:{task-id}`, and file contexts carry `path`. Pull requests and
workspaces select their own frozen revision; other contexts resolve `ref` once.
Private workspace sharing is enforced before its revision can be used.

The response is `application/x-ndjson`: a `conversation` event is followed by
ordered `claim` events and one `done` event containing the complete retained
conversation. Every claim labels its `basis` as `evidence`, `inference`, or
`uncertainty`, reports confidence, and cites an exact revision plus a source
path and line or an immutable check/dependency resource. Bounded collection is
reported as `analysis_status: incomplete` with a reason. The complete answer is
persisted before streaming, so an interrupted client can replay it from
`GET /repositories/{id}/explanations/{explanation-id}`. Collection history is
available at `GET /repositories/{id}/explanations`; both reads revalidate
current repository access and explicit investigation membership. The opener is
the first participant. An existing participant can invite another current
repository participant with `POST .../{explanation-id}/participants`; repository
access alone does not disclose a shared investigation.

`POST .../{explanation-id}/entries` appends an attributable ordered canvas entry
of kind `code_reference`, `query`, `runtime_observation`, `hypothesis`,
`agent_finding`, `conclusion`, or `challenge`. Challenges can name
`supersedes_id`. Code references are verified against the frozen Git tree.
Runtime observations can name only a currently visible bounded workspace; the
attachment retains its identity and revision, never output, credentials, or
hidden files. `POST .../{explanation-id}/reruns` resolves `ref` to a new
immutable revision and appends findings without rewriting earlier history.
Projected citations report `stale: true` when their revision differs from the
current run.

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

## Development workspaces

`POST /workspaces` requires `repositories:write` and a current owner or
collaborator. It accepts `repository_id`, an exact 40-character `commit_id`, and
a `source` whose `kind` is `repository`, `proposal_task`, `pull_request`, or
`incident_repair` with the corresponding IDs. Task launches enforce an assigned
base when present; pull launches accept only a recorded source or target
revision; incident launches require a named emergency-repair action. The commit
must contain `.vivarium/workspace.json` version 1 with `image`, `tools`,
`dependencies`, `setup`, and `resources` (`cpus`, `memory_mb`, `storage_mb`, and
`setup_seconds`). Invalid context or definitions return `422` without launching.

Creation partitions the declared storage budget between a size-limited
`/workspace` tmpfs and 16 MiB of bounded `/tmp` scratch space; the remaining
container root is read-only. Declared storage therefore bounds
collaborator-controlled writes without exposing a writable host bind or layer.
Images declaring Docker volumes are rejected before container creation, and
cleanup removes both the named container and any attached volumes. The named
container is force-removed after setup failure or timeout. Creation
returns the durable workspace after bounded setup, including its
state, creator, exact commit, complete definition and SHA-256, source, effective
access, setup evidence, and lifecycle events. `GET /workspaces` lists launches
across the actor's current repository collaborations; `GET /workspaces/{id}` is available to current repository
participants. `POST /workspaces/{id}/suspend` and `/resume` accept
`{"foundation":"<definition_sha256>"}`. A stale hash, invalid state, or missing
materialized runtime returns `409 workspace_foundation_changed`; resume never
resolves a branch, changes the commit or definition, or reruns setup.

Workspace governance is versioned independently at `GET|PUT
/repositories/{repository_id}/workspace-policy` and, for organization owners,
`GET|PUT /organizations/{organization_id}/workspace-policy`. Updates
compare-and-swap `expected_version` and bound CPU, memory, storage, idle time,
maximum runtime, retention, sharing, network, and approved-agent execution.
Organization limits constrain repository settings; network is currently
fail-closed to `none`. Launch rejects definitions above the effective limits
and snapshots that policy into the workspace. Repository-policy changes mark
existing active environments `rebuild_required` while leaving them available
to checkpoint and export.

Repository owners inspect attributed reservations and elapsed consumption at
`GET /repositories/{repository_id}/workspace-usage`. `POST
/workspaces/{id}/expiry` announces a future expiry so collaborators can publish
or checkpoint legitimate unpublished work. Startup and a periodic lifecycle
pass autonomously expire environments after that deadline or their idle limit;
`POST .../reconcile` provides the same idempotent owner-triggered check, and
`POST .../stop` immediately removes compute. Teardown must succeed before a
terminal state is committed, and the full removal/publication interval is
serialized with suspend and resume. These terminal actions revoke control and remove the named
container but preserve the workspace record, provenance ledger, checkpoints,
published commits, and pull links. Private sharing restricts a workspace to its
creator and repository owner; repository access is still revalidated for every
other mode, and policy can disable new approved-agent control leases.
Responses contain the resource array and `next_cursor`, which is `null` on the
last page. Pass a non-null `next_cursor` unchanged as the next request's
`after`; cursors outside the authenticated collection return
`invalid_pagination`. Collection order is oldest creation first, with opaque ID
as the deterministic tie-breaker.

Running workspace automation uses the same current-participant authorization
and exact workspace identity. `GET /workspaces/{id}/files?path=...` lists one
directory, `GET .../file?path=...` reads an editable text file, and `PUT
.../file` accepts `path`, `content`, and the prior `expected_sha256`; concurrent
changes return `409 workspace_file_changed`. Saves retain only path, size, and
content digest as durable evidence, never file contents. `GET .../search?q=`
performs bounded literal search. `POST .../commands` accepts a command, relative
working directory, and 1–300 second timeout and retains its bounded attributed
outcome. `GET .../ports` discovers loopback listeners, while `GET
.../preview?port=&path=` proxies at most 1 MiB through an authenticated,
sandboxed HTML response without publishing a container port. These endpoints
reject non-running workspaces. Commands receive no platform, repository,
deployment, or environment credentials, so platform-managed secrets cannot
enter snapshots, logs, or shared previews.

`PUT /workspaces/{id}/presence` accepts a `focus` of `workspace`, `file`,
`terminal`, `command`, or `preview` plus an optional bounded `path`; clients
renew it while connected. `DELETE .../presence` records a deliberate leave.
Presence expires after 20 seconds without renewal, so abrupt disconnects do not
leave a participant online. Discussion and typed activity remain durable.
`POST .../messages` accepts a 1–4000 character `body`.

`PUT /workspaces/{id}/control` compare-and-swaps `expected_version` and names a
`human` or `approved_agent`, an `observe`, `guide`, `edit`, or `execute` mode,
a subset of `files`, `commands`, and `lifecycle`, and an `expires_in` lease of
30–3600 seconds. Humans must be current repository participants; agents must be
approved in its organization. Stale updates return `409
workspace_control_changed`. File, command, suspend, and resume routes require
the corresponding unexpired human control scope and otherwise return `409
workspace_control_required`. Selecting an agent grants no caller authority;
agent execution remains a separately authorized boundary.
An empty principal with `mode: "observe"`, empty scopes, and the current version
lets the current live human holder explicitly release control; other writers
cannot clear that lease. Final lease validation and mutation admission are
serialized with transfer: a takeover waits for already-admitted work, while a
request that loses its lease before admission is rejected.

Command outcomes omit submitted commands. They retain `command_sha256`,
directory, bounded output, exit status, actor, and times, so collaborators can
understand execution without revealing private terminal input. Activity roles
are `observation`, `instruction`, `authorship`, and `execution`.

`POST /workspaces/{id}/checkpoints` accepts a bounded `title`, `description`,
`expected_parent_checkpoint_id`, and `reproducibility` declaration containing
dependency names and notes. It snapshots only regular repository-file changes
against the workspace's exact base commit, with a 32 MiB total limit. Suspected
credential paths or contents reject the request with `422 checkpoint_not_safe`.
This includes package-manager authentication files such as changed `.npmrc`
records and npm `_authToken`/`_auth` directives regardless of filename.
Inspection covers the complete bounded snapshot rather than treating large
files as implicitly safe. Runtime capture and durable publication are one
workspace-admission operation, ordered against controlled file mutations.
`GET /workspaces/{id}/checkpoints` and `GET .../checkpoints/{checkpoint-id}`
return attribution, environment definition, parent lineage, change operations,
hashes, modes, and sizes; private stored file bytes and textual patches are
never returned.

`GET .../checkpoints/{checkpoint-id}/restore` returns base divergence, live
path conflicts, missing declared dependencies, reproducibility reasons, and a
`preflight_token`. `POST` to the same route accepts that token and optional
`allow_conflicts`. It requires current human file control and revalidates the
token after admission; changed workspace state returns `409
checkpoint_preflight_changed`, while unaccepted overlapping changes return
`409 checkpoint_restore_conflicts`. A successful restore updates the workspace
checkpoint head, making the next checkpoint a retained lineage branch.

`POST /workspaces/{id}/checkpoints/{checkpoint-id}/publish` requires current
`repositories:write` access. It accepts `branch`, optional
`expected_commit_id`, `target_branch`, `title`, `session_id`, and
`create_pull_request`. A new branch starts at the checkpoint's exact base; an
existing branch advances only when its current tip equals both the supplied
expectation and that base (clients may send the base for either branch state).
Publication claims serialize the unpublished check through Git and pull
effects, and failed pull creation compare-and-swap restores or removes the
branch without overwriting a concurrent push. A post-pull linkage failure
durably records `link_pending`, returns `202` with
`Vivarium-Recovery-Publication: pending`, and an exact retry links the existing
pull before checks start. Publication intent is durably staged outside the checkpoint record before a
branch changes and updated with the pull/link stages, so even a late checkpoint
directory failure can reconcile the existing commit and pull on retry.
The server constructs one Git commit exclusively from the checkpoint manifest
and stored bytes. With pull creation enabled, it
opens an ordinary pull (preserving proposal-task context when applicable),
starts repository-defined checks, and records bidirectional workspace,
checkpoint, task/session, contributor, and command-digest links. Task session
IDs are accepted only after loading the session beneath the same repository,
proposal, and task. Contributor and command evidence is frozen during
checkpoint capture rather than reconstructed from later bounded workspace
history. The evidence source is a private append-only workspace ledger, so the
bounded command/change arrays used by workspace reads cannot evict provenance
before capture. When a legacy workspace first records post-upgrade activity,
its retained histories seed that ledger before the new event is appended. The generated
pull body lists inspected file hashes and attributed command IDs/digests, never
terminal input, command output, credentials, presence, messages, or other
runtime state. Existing stale-review, check, review, merge, and queue rules
govern the pull.

Guided contribution work uses `GET
/workspaces/{id}/checkpoints/{checkpoint-id}/contribution-publication` as a
publication preflight. It separates fixable blocking `project_requirements`
(the frozen pathway acknowledgement and version, live opportunity revision,
setup verification, a non-empty checkpoint, and explicit acceptance-criterion
confirmation) from non-blocking `coaching_needs` such as retained diagnostics
or open help threads. `POST` to that endpoint accepts `branch`,
`target_branch`, `title`, the exact `satisfied_criteria`, and optional
`maintainer_edits_allowed`. The server commits the inspected checkpoint to the
contributor-owned fork and opens a cross-repository pull against upstream. The
pull retains the opportunity/pathway revisions, setup outcomes, mentor-guidance
and agent-assistance entry IDs, criteria, contributors, and checkpoint evidence.
Publication holds exact current-pathway and in-progress-opportunity admissions
through pull creation and checkpoint recording, so governing changes cannot
interleave after the final validation.
This provenance grants no upstream authority: ordinary discussion,
reproduction, review, required checks, owner acknowledgements, integration
queues, permissions, and merge rules apply.

`POST /repositories/{id}/contribution-opportunities/{opportunity-id}/completion`
is owner-only and accepts the opportunity's `expected_version`, exact
`pull_request_id` and `release_id`, bounded `feedback`, `credit`,
`ready_for_next`, `skills_recognized`, optional `next_opportunity_id`, and a
`readiness_note`. The pull must be a merged guided contribution for that exact
opportunity version, and the release must include both the pull and contributor.
The response retains server-derived setup, mentor-guidance, and approved-agent
support counts. Exact retries are idempotent; changed evidence conflicts.

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

`POST /repositories/{id}/forks` accepts `name` and creates a private,
independently owned repository from any source the actor can read. The source
is not modified and the actor receives no source collaborator grant. Fork
responses include immutable `upstream_repository_id` lineage and preserve the
source's published branches with independent references and administration.

The fork owner uses `POST /repositories/{id}/synchronizations` with `branch` to
synchronize from the same-named branch in its recorded upstream. The response
contains `branch`, optional `previous_commit_id`, `commit_id`, and
`upstream_repository_id`. Invalid upstream branches return `invalid_branch`,
independent history that would be overwritten returns `409 fork_diverged`, and
a concurrent fork push returns `409 branch_changed`. The server imports the
exact tip's missing content-addressed objects before compare-and-swap
fast-forwarding only the selected reference.

Federated repositories resolved through a trusted peer can be forked with
`POST /federation/repositories/{instance-id}/{repository-id}/forks`, accepting
`name` and an advertised `branch`. The private response retains
`federated_upstream` and `federated_branch`; its `git_remote` uses ordinary
local stock-Git credentials. `POST /federation/forks/{id}/synchronizations`
refreshes signed metadata and fast-forwards the selected branch. `POST
/federation/forks/{id}/pulls` accepts `title`, optional `body`, `source_branch`,
and `target_branch`, freezes both tips, and negotiates a signed idempotent
proposal. Trust, target movement, divergence, signature, and transfer failures
use explicit `federated_*` errors and publish no partial refs or credentials.

Federated pull activity is read at `GET
/repositories/{id}/pulls/{pull-id}/federation-events`. Current target-repository
participants publish a bounded event with `POST` to the same path. Supported
`kind` values are `comment`, `review`, `revision`, `checks`, `preview`,
`agent_session`, and `closure`; revision-bound events default to the pull's exact adopted source
commit. The instance signs the immutable event, retains it locally, and sends it
to `POST /federation/contributions/{contribution-id}/events`. A successful
duplicate is idempotent, different content under the same origin/event identity
is `409 federated_event_conflict`, and an unavailable peer returns `202` with
`Vivarium-Federation-Delivery: pending`. Reads retain `actor`,
`origin_instance_id`, `verification`, signature metadata, and derived `stale`.
Imported evidence never counts as a local required check, review identity,
credential, or merge permission.

Duplicate recognition occurs after live peer trust, contribution authority,
and signature verification but before revision bundle import. Consequently an
exact retry of an already adopted revision is acknowledged without advancing
Git twice, while revocation and changed signed content still fail closed.

Merging an open federated pull uses the ordinary `POST
/repositories/{id}/pulls/{pull-id}/merge` endpoint and its live ownership,
review, required-check, integration-policy, conflict, and exact-revision rules.
On success the upstream retains the reachable source objects and records a
signed `receipt` event containing the contribution/source provenance, merge
commit and maintainer, plus the verified collaboration snapshot. The receipt is
stored locally before delivery and queued durably when the source peer is
offline or no longer trusted. Merge and receipt retries are idempotent; later
peer or identity loss changes delivery/trust status, not accepted Git history.

On the source instance, `POST
/federation/contributions/{contribution-id}/agent-sessions` accepts an
`organization_id`, approved `agent_id`, bounded `instructions`, `context_paths`,
and optional `expires_in`. The caller must own the linked fork and operate that
approved agent; every path must exist at the frozen revision. Completion at the
nested `/{session-id}/runs/{run-id}/completion` endpoint accepts the branch-tip
`commit_id`, `summary`, bounded `commands`, `evidence`, `residual_concerns`,
`agent_minutes`, and `cost_units`. The server derives commits/files, transfers
the exact bounded bundle as a signed revision, and sends a signed redacted
`agent_session` event. Local guidance/control is never federated, and this
credential or evidence grants no target-instance authority.

Owners manage limited access with `GET` and `POST
/repositories/{id}/collaborators` and `DELETE
/repositories/{id}/collaborators/{user_id}`. A grant request contains an
existing `user_id`; collaborator resources contain that stable ID and
`role: "contributor"`. Granting the same user and revoking an absent grant are
idempotent. Only the owner may inspect or change grants.

Repository responses include immutable `id` and `owner_id`, user-facing
`name`, `visibility`, `default_branch`, `created_at`, and `git_remote`. Forks
also include `upstream_repository_id`. Use the returned `git_remote` relative
to the API origin. Private reads and writes
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

### Proposal plans

Current repository participants turn an open proposal into executable work with
`POST /repositories/{id}/proposals/{proposal_id}/tasks`. The request requires a
single-line `title` (at most 200 characters) and an `outcome` (at most 2,000
characters). Optional `dependency_ids` must identify other tasks in the same
proposal, and optional `discussion_comment_ids` must identify comments in that
proposal. Duplicate, missing, self, and cyclic links are rejected.

`GET .../tasks` returns the complete plan in `position` order. Each task has one
of `todo`, `in_progress`, `completed`, or `cancelled` status and includes
immutable `created_by`/`created_at` plus current `updated_by`/`updated_at`
attribution. `ready` is true only for a `todo` task whose dependencies have a
current completed result; an obsolete or closed linked contribution does not
satisfy a dependency. `blocked_by` names the unmet dependency IDs. These derived fields let
clients answer what can start without independently interpreting the graph.

Participants edit a task with `PATCH .../tasks/{task_id}`. Any supplied
`title`, `outcome`, `status`, dependency links, or discussion links replaces
that field. A zero-based `position` atomically moves the task and renumbers the
ordered plan. `GET .../tasks/{task_id}` reads one current task, while `GET
.../tasks/{task_id}/history` returns its append-only, chronological `history`.
Every history entry records the stable actor, action, timestamp, and full task
snapshot for creation, edits, status decisions, and reordering. Plan reads
inherit proposal visibility; mutations require a current owner or contributor
with `repositories:write` and use the proposal uncertain-durability contract.
Definition edits advance `context_revision`. `context_state` is `changed` when
an assignment predates that revision and `obsolete` when linked contribution
evidence does; those sessions and pull requests remain inspectable and are
never silently rewritten as current work.

A ready `todo` task gains one accountable owner through `PUT
.../tasks/{task_id}/assignment`. The body contains `assignee_type` (`human` or
`agent`), an `assignee_id` for humans, `mandate`, the proposal's
`repository_id`, an exact existing commit as `base_revision`, and an empty
`expected_assignment_id` for an initial claim. Human assignees must already be
the owner or a current contributor; assignment never grants them repository
access. An available agent receives a generated durable assignee ID and an
inspectable access preview limited to `git:read`/`git:write` on this repository,
the exact base, and the task branch that the start-work flow will create.

Reassignment repeats `PUT` with the current assignment ID in
`expected_assignment_id`. Revocation uses `DELETE
.../assignment?expected_assignment_id={assignment-id}`. These compare-and-swap
preconditions return `409 task_assignment_conflict` when another collaborator
claimed, reassigned, or revoked first, so no stale request silently replaces an
owner. Successful assignment, reassignment, and revocation append full
actor-stamped snapshots to task history and share the uncertain-durability
contract. Human participant validation and assignment publication share the
repository catalog mutation lock, so collaborator removal cannot commit between
authorization and assignment. Closed proposals reject assignment revocation and
retain their final task history unchanged.

Participants deliberately move an assigned nonterminal task onto a new commit
with `POST .../tasks/{task_id}/rebase`. Its body supplies `base_revision` and
the current `expected_assignment_id`. The commit is verified, and success
creates a new assignment ID at the current context revision while preserving
the assignee and mandate. A stale ID returns `409 task_assignment_conflict`.
This boundary leaves earlier sessions and pull requests attributable but
obsolete. Assignee-targeted activity/inbox events report transitions to ready,
blocked, changed, and obsolete as dependencies, plan revisions, and linked
contributions evolve.

An agent assignment can start before a pull request exists with `POST
.../tasks/{task_id}/sessions`. The body supplies the current
`expected_assignment_id`, optional `context_paths`, and an optional
`expires_in` from five minutes to 24 hours. The proposal and task must remain
open and ready, the context must be current, and the assignment must still
target an agent. Success creates exactly one session for the current assignment,
while sessions from earlier rebased assignments remain inspectable, plus an isolated
`refs/heads/agent/tasks/<task-id>-<assignment-prefix>` branch at the frozen
base revision, a launched run carrying the assignment mandate, and a one-time
Git credential restricted to that repository and branch. A repeated start for
the same assignment returns `409 task_session_exists`; an existing branch is
never overwritten.
Start holds the proposal mutation lock from exact assignment revalidation
through branch, credential, and session publication, so revocation, task edits,
dependency changes, and proposal closure cannot commit midway. The session and
initial run are one atomic durable record: a definitive failure revokes the
credential and removes the new branch without exposing a half-launched
workspace, while a post-publication durability uncertainty returns both stable
resources for later reconciliation.

Task sessions use `/repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/sessions`
and the same detail, `events`, `runs`, run `events`, `control`, and
`interventions` suffixes as pull-request sessions. Their durable session
snapshot adds `proposal_id`, `task_id`, and `task_context`: repository and
proposal context, the task outcome and mandate, dependency snapshots, and
linked motivating comments. Collaborators therefore reconnect, observe,
guide, answer, pause, resume, and cancel through the established protocol.
The credential-bound agent completes a task run with `POST
.../sessions/{session_id}/runs/{run_id}/completion` and the same structured
`summary`, exact `commit_id`, `checks`, and `unresolved_concerns` body used by
pull-scoped runs. Completion requires the exact live task-branch tip to be a
new descendant of the frozen assignment base, derives commits and changed
files server-side, records the attributable outcome, and terminally revokes
the bounded credential. It does not create or update a pull request.
Cancellation revokes the bounded credential, while durable run state also
denies later writes if credential revocation cannot be relied upon. Publishing
task results into review is a separate workflow; starting work does not create
an empty pull request.

An assigned human, or a participant publishing a completed assigned-agent run,
hands task work to review with `POST .../tasks/{task_id}/contributions`. The body
uses ordinary pull-request `title`, `body`, `source_branch`, and `target_branch`
fields; agent work additionally supplies its completed `session_id` and
`run_id`. The resulting pull retains `proposal_id`, `task_id`, optional
`task_session_id`, and optional `task_run_id`; the task retains every candidate
and its exact commits in the other direction. Repository-defined checks run on
the pull's exact source snapshot.

Publication records `review` and moves the task to `in_progress`, never
`completed`. A later attempt marks the prior candidate `superseded`. Closing an
unmerged pull records `closed` and returns the task to `todo`; only merge records
`merged` and completes it. Task-linked merges do not automatically close the
whole multi-task proposal. Pull records retain `task_state_pending` until the
corresponding task mutation is durable; pending pulls are not mergeable, and
pull collection/detail reads retry the exact idempotent link, close, or merge
projection before clearing that repair intent. Agent publication also requires
the live branch tip to equal the completed run's exact outcome commit.

## Automated checks

Repository owners manage the quality gate for a target branch with `GET` and
`PUT /repositories/{id}/branches/{branch}/required-checks`. The `PUT` body is
`{"checks":["web","api"]}`; names must exactly match check names in the
candidate's versioned configuration. An empty array removes the branch gate.
Participants may read the policy, but only the owner may replace it.

Owners can additionally protect a target branch with `GET` and `PUT
/repositories/{id}/branches/{branch}/integration-queue`. The complete `PUT`
body is `{"enabled":true,"concurrency":2,"failure_behavior":"pause"}`.
Concurrency is between 1 and 10; failure behavior is `pause` or `remove`.
Readable responses include the branch, configured controls, one required
approval, and the branch's live `required_checks`. Only the owner may replace
policy; anyone who can read the repository may inspect it.

A repository opts into automatic pull-request verification by committing
`.vivarium/checks.json` on the candidate branch. The file has `version: 1` and
a non-empty `checks` array (at most 20). Each check defines a unique `name`, a
preinstalled OCI `image`, a shell `command`, and optional `working_directory`, `environment`, and
`timeout_seconds` (default 600, maximum 3600):

```json
{"version":1,"checks":[{"name":"api","image":"vivarium/go:1.26","command":"go test ./...","working_directory":"apps/api","environment":{"GOFLAGS":"-mod=readonly"},"timeout_seconds":900}]}
```

Opening a pull request or explicitly synchronizing it to a new source revision
loads that file from the exact recorded source commit and creates one durable
run per definition. Execution uses an exported copy of that commit in a new
OCI container with no network, capabilities, host root, Git repository, or
credential; its root and source snapshot are read-only and CPU, memory, process
count, and lifetime are bounded. Commands may write at most 256 MiB to the
size-limited directory named by `$VIVARIUM_OUTPUT`; `$HOME` is a separate 64
MiB tmpfs. Images are never pulled by a check and must already exist on the
runner. The complete container must be confirmed removed before a run becomes
terminal; cleanup failures remain durably `cleanup_pending` for retry.
`PATH`, `HOME`, and `GIT_*` cannot be overridden. Runs progress
through `queued`, `running`, and `succeeded` or `failed`; invalid configuration
is retained as a failed configuration run. Repeating synchronization without a
new commit does not duplicate automatic runs. At API startup, durable `queued`
and interrupted `running` or `cleanup_pending` records are retried from the same
exact commit under a cross-process execution lock. Recovery repeats every 30
seconds, and a later same-commit collaboration trigger also relaunches existing
nonterminal work after a transient repository outage.

`GET /repositories/{id}/pulls/{pull_id}/checks` returns creation-ordered
`check_runs`; `GET /repositories/{id}/pulls/{pull_id}/checks/{check_id}` reads
one stable run. Each record contains its exact `commit_id`, snapshotted
definition, current lifecycle state, timestamps, retained numbered `attempts`,
and artifact metadata. Interrupted executions become failed attempts before a
new recovery attempt starts, so later success does not erase earlier evidence.

`GET /repositories/{id}/pulls/{pull_id}/checks/{check_id}/events` returns the
immutable, sequence-ordered evidence stream: queued/running/terminal status,
stdout and stderr log chunks, artifact publication, and the command outcome
with exit code and failure reason. Pass `after=<sequence>` to reconnect and
receive only newer events; `next_sequence` is the cursor for the next request.
Each output stream retains up to 10 MiB per attempt and ends with an explicit
truncation message when that bound is reached. Artifact metadata includes its
original relative path, attempt, byte size, SHA-256 digest, media type, and
publication time. Download bytes from `GET
/repositories/{id}/pulls/{pull_id}/checks/{check_id}/artifacts/{artifact_id}`.
All reads use the pull request's visibility policy. Records, evidence, and
artifact bytes live beneath `CHECK_RUN_STORAGE_ROOT`, defaulting to
`check-runs`.

Current owners and contributors with `repositories:write` can control a run
without changing its snapshotted definition or commit. `POST
/repositories/{id}/pulls/{pull_id}/checks/{check_id}/cancel` stops a
nonterminal run; `POST
/repositories/{id}/pulls/{pull_id}/checks/{check_id}/rerun` queues a finished
run for another attempt. Reruns retain every earlier attempt, event, and
artifact. Both actions append immutable `control` evidence with the actor's
stable user ID, and a collaborator-requested attempt carries that attribution.
Cancellation persists its intent before stopping the executor, so a forced
container exit cannot win the terminal-state race. Control attribution is also
stored in the run record and repaired into the event stream after a transient
event-write failure; queued reruns remain schedulable while that repair waits.
Projection repair is serialized across API processes, and cancellation intent
is retained whenever terminal record durability is uncertain so recovery never
relaunches canceled work from an older record image.
If execution reaches a terminal result before cancellation obtains the
execution lock, that durably reconfirmed result wins and the cancel endpoint
returns a state conflict without appending cancellation evidence.

## Pull requests

Repository participants open a pull request with `POST
/repositories/{id}/pulls`. The request requires `title`, `body`,
`source_branch`, and `target_branch`; `proposal_id` may link an existing
proposal in the same repository. Branch names are repository-relative (for
example, `feature`, not `refs/heads/feature`), and must both
currently identify commit objects. A missing, unborn, or non-commit branch is
rejected without creating a resource.

`source_repository_id` optionally identifies an independently owned direct
fork whose immutable `upstream_repository_id` is the target `{id}`. The actor
must own that fork and retain read access to the target; a public upstream does
not otherwise grant repository participation. When source and target are the
same repository, their branch names must differ. The server imports only the
objects reachable from the adopted fork commit into the target object database
without publishing a target reference.

The created resource records immutable target `repository_id`, immutable
`source_repository_id`, and `author_id`, its
purpose in `title` and `body`, the source and target branch names, and the exact
branch tips as `source_commit_id` and `target_commit_id`. These commit IDs do
not silently change when a branch advances. The author can explicitly adopt
the source branch's current commit as a new reviewable revision with `POST
/repositories/{id}/pulls/{pull_id}/synchronize`; this updates
`source_commit_id`, while the target snapshot remains fixed. Synchronization
requires `repositories:write`, rejects a missing or non-commit source branch,
and is unavailable after merge. For a fork contribution, synchronization reads
the live branch in the source repository and remains available to that fork
owner while they retain access to the target; private-upstream revocation takes
effect before the next synchronization and returns not-found. A different
participant receives not-found. Target/source authorization, object import,
and pull revision publication share the catalog's cross-process mutation lock,
so a revocation commits wholly before or after an authorized adoption.
New pull requests have `status: "open"` and creation and update timestamps.
The linked `proposal_id` is nullable.

For an independently owned source, only the pull author/source owner can
`PATCH /repositories/{id}/pulls/{pull_id}` with
`maintainer_edits_allowed`. When enabled, a current target owner or contributor
can `POST .../maintainer-credential` to receive a one-hour Git credential bound
to the issuing pull request, source repository, and exact contribution branch. Git advertisement and
push are restricted to that branch. Disabling the policy, closing the request,
deleting the branch or source repository, or revoking target participation
invalidates the grant on the next request; it never creates repository-wide
source access.

The pull author or target repository owner can `POST
/repositories/{id}/pulls/{pull_id}/close`. Closure records `status: "closed"`,
`closed_at`, and `closed_by`, disables the edit grant, and stops synchronization,
new reviews, checks, sessions, and merge while retaining snapshots, discussion,
reviews, and check evidence. Target contributors cannot close someone else's
contribution, and only the target owner can merge.

`GET /repositories/{id}/pulls` returns pull requests in the shared cursor-
paginated collection shape under `pull_requests`; `GET
/repositories/{id}/pulls/{pull_id}` inspects one. Reads inherit repository
visibility and collaborator access. Creation requires either a current owner
or contributor, or the owner of a direct fork targeting that readable
repository, with `repositories:write`; public readability alone does not grant
permission to submit arbitrary repository state. Pull request metadata is stored as
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

Owners, contributors, and the author of a cross-repository pull request into a
currently public upstream append immutable pull request discussion with `POST
/repositories/{id}/pulls/{pull_id}/comments` and a non-empty `body`. `GET` on
the same collection returns attributable comments under `comments` with the
shared cursor pagination contract. Each comment records its stable `id`,
`pull_request_id`, `author_id`, body, and creation time. Reads inherit pull
request visibility. Commenting requires `repositories:read` plus current
repository participation, except that the outside author may continue their
own conversation while its upstream is public. Public visibility alone does
not grant other users comment permission. Comment publication uses the shared
uncertain-durability response.

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

`GET /repositories/{id}/pulls/{pull_id}/merge-readiness` gives any actor who
can read the pull request a read-only answer about what remains before merge. The report
contains `mergeable`, caller-specific `can_merge`, one required current
approval and the current approval count, live `source` and `target` branch
state, `has_conflicts`, and an ordered `blockers` array whose entries have a
stable `code` and explanatory `message`. Branch state includes the branch
name, immutable pull-request `snapshot_commit_id`, nullable
`current_commit_id`, and a state of `current`, `advanced`, `rewritten`, or
`missing`.
If an independently owned source repository is later deleted, its already
imported adopted commit remains reviewable and mergeable. The report returns
that verified commit as `current_commit_id` with source state `unavailable`;
only synchronization requires the live fork to discover a newer revision.

The report also identifies `evaluated_commit_id` and returns every target-
branch requirement in `required_checks`, including its exact name, derived
status, and the matching run and commit IDs when evidence exists. Required
statuses are `passed`, `missing`, `pending`, `failed`, `cancelled`, or `stale`.
Only a successful run whose recorded commit equals the pull request's adopted
source revision passes. Every other status is a blocker, and merge repeats the
same evaluation while holding its mutation lock.

Readiness also returns `integration_queue` and `can_enqueue`. On a protected
target, ordinary rules still determine `mergeable`, but `can_merge` is false
and direct merge is rejected. An owner with an eligible request receives
`can_enqueue: true` and uses `POST
/repositories/{id}/pulls/{pull_id}/queue`. Admission persists `queued_at`; its
timestamp establishes initial FIFO order, while an exact rational `queue_rank`
supports atomic single-entry reprioritization without finite timestamp gaps.
Retrying admission is idempotent. Source synchronization or pull closure
clears stale admission.
Admission also creates an immutable synthetic two-parent commit: the first
parent and `base_commit_id` are the latest eligible target tip, while the
second parent and `source_commit_id` are the exact admitted pull revision.
Required check definitions are resolved from that owner-controlled target
base and frozen on the candidate; a pull's same-named configuration cannot
replace their image, command, environment, working directory, or timeout.
`GET /repositories/{id}/pulls/{pull_id}/candidates` returns these identities,
creation time, derived `pending`, `verifying`, `passed`, `failed`, or
`superseded` lifecycle,
and candidate-scoped check runs. Those bound definitions execute against the
prospective result snapshot. Run IDs use the ordinary check detail,
events, logs, artifact, cancel, and rerun routes. Candidate checks execute the
prospective result snapshot, so successful required-check evidence describes
the repository state integration would create rather than either parent.
If publishing a candidate's required runs stops after a durable prefix,
reconciliation compares run names with the frozen definitions and creates only
the missing runs before resuming execution.

The server continuously reconciles protected queues under the pull-request
mutation lock. Only the FIFO head may advance the target, and it does so with a
compare-and-swap from its recorded `base_commit_id` to its exact candidate
commit. A moved target supersedes every affected active candidate and creates
new candidates, up to the configured concurrency, before considering their
evidence. Superseded runs remain inspectable but can never authorize a target
update. A passing head becomes the pull request's durable merge result; its
success closes a linked open proposal, records attributable collaboration
activity, and immediately rebuilds later candidates against that merge.
Finalization remains durably pending until every cross-store effect succeeds;
recovery retries it with stable event identities so outages neither lose nor
duplicate activity.
Source synchronization and closure remove an entry without deleting candidate
history. Failed or cancelled head checks and candidate conflicts either leave
the entry blocking for `pause`, or clear admission and continue for `remove`.

`GET /repositories/{id}/branches/{branch}/queue` is the shared branch queue
projection. It returns durable one-based order plus each pull request, its
current candidate and retained attempt history, derived lifecycle, explicit
blockers, and a plain-language `next_action`. Owners operate an admitted entry
with `PATCH /repositories/{id}/pulls/{pull_id}/queue` and an `action` of
`pause`, `resume`, `retry`, `remove`, or `reprioritize` (the latter also takes
a one-based `position`). Retry supersedes rather than deletes failed candidate
evidence. Every admission and intervention retains actor and time on the pull
request, emits collaboration activity targeted to the pull author, and
projects relevant outcomes into that user's actionable inbox.

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
`Source-Repository`, `Source-Branch`, `Source-Commit`, `Authored-by`,
`Merged-by`, and optional `Proposal` trailers. A linked open
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

An authorized participant can turn a concrete automated failure into that
workspace by sending `{"check_run_id":"<id>"}` when creating the session. The
run must be failed and belong to the pull request's currently adopted source
revision; stale, active, successful, and canceled runs return `409
check_not_repairable`. The session durably snapshots the exact revision,
versioned check definition, execution events (including logs and command
outcomes), and artifact identities and hashes. Ordinary session inspection
therefore presents the same evidence after restart without following a later
rerun or branch movement.
The final revision comparison and session publication hold the pull-request
mutation lock, so concurrent source synchronization either waits for the
session to become durable or wins first and causes creation to return `409
source_branch_changed`; a successful repair session is never obsolete at
publication.

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

For a cross-repository pull request, delegation additionally requires the
contribution owner's current `maintainer_edits_allowed` opt-in. The credential
is bound to the independently owned source repository, contribution branch,
and pull request rather than the target repository. The agent reads context
and publishes commits in that fork; it receives no upstream repository access.
Disabling the policy, closing the pull request, deleting the source branch or
repository, or revoking the initiating participant's target access invalidates
the credential on its next Git or run API request. Completion imports the new
source snapshot into the target object database, adopts it through ordinary
pull synchronization, starts exact-revision checks, and makes existing reviews
stale without publishing an upstream branch.

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
collaborator `interventions`, plus the session's optional `check_evidence`,
without exposing the participant-only session
timeline. A paused run receives `409 agent_run_paused` if it attempts to
publish more progress and must poll control state until resumed. Cancellation
is terminal: it records `state: "canceled"`, appends `run.canceled`, and revokes
the bounded Git credential so later progress, control reads, fetches, and
pushes fail authentication. Cancel retries tolerate an already-revoked
credential. Intervention publication follows the shared uncertain-durability
response contract.

A repair run can download only the artifacts captured by its session through
`GET .../runs/{run_id}/evidence/artifacts/{artifact_id}` using its bounded Git
credential. The check evidence remains tied to the failed revision even if the
original check is rerun later.

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
request rules. The response contains the run, event, and updated pull request.
source synchronization also starts `.vivarium/checks.json` verification for
the newly adopted commit, so repaired work immediately produces fresh
exact-revision evidence. The shared `202` contract applies if either durable publication is visible but
not confirmed. A moved branch tip, unrelated history, paused/canceled run, or
pull request advanced by another workflow is rejected without presenting the
candidate as this run's completed work.

The durable terminal run state is also checked independently by receive-pack,
so a completed run cannot push again even if credential revocation storage is
temporarily unavailable. Completion is persisted before source synchronization:
invalid evidence or a failed session write cannot advance the request, while a
later synchronization failure leaves a durable handoff that the still-active
credential can safely retry. Synchronization eligibility, including absence of
a server merge intent, is checked under the pull-request lock before the
completion callback runs, so a semantically blocked request cannot terminalize
its agent run without adopting the handoff.

## Release candidates

`GET /repositories/{id}/releases` lists immutable release candidates in
creation order as `{ "releases": [...] }`. `GET
/repositories/{id}/releases/{release_id}` returns one candidate. Reads use the
repository's normal public/private visibility policy.

`POST /repositories/{id}/releases` requires an owner or contributor credential
with `repositories:write` and accepts:

```json
{
  "version": "v1.4.0",
  "notes": "Release rationale and participant guidance.",
  "commit_id": "0123456789abcdef0123456789abcdef01234567",
  "previous_release_id": "0123456789abcdef0123456789abcdef"
}
```

`previous_release_id` is optional. `commit_id` must be a verified commit in the
repository, and a selected earlier release must belong to the same repository
and be an ancestor of it. Versions are unique per repository, case
insensitively. Creation returns `201` and a `Location` header. The immutable
response has `status: "candidate"`, `created_by`, `created_at`, the exact prior
commit boundary when selected, and `inclusions` arrays for `pull_request_ids`,
`proposal_ids`, `task_ids`, and `contributor_ids`. Those inclusions are computed
from merged history by the server and cannot be supplied by the caller.

### Release builds and attestations

A candidate's exact commit opts into distributable builds with
`.vivarium/release.json`:

```json
{
  "version": 1,
  "steps": [{
    "name": "package",
    "image": "alpine:3.22",
    "command": "tar -czf \"$VIVARIUM_OUTPUT/app.tgz\" dist"
  }]
}
```

Steps use the verification definition contract (`working_directory`, bounded
`environment`, and `timeout_seconds` are optional). `POST
/repositories/{id}/releases/{release_id}/builds` requires a current owner or
contributor with `repositories:write`, snapshots that definition from the
candidate commit, and returns `202` with `{ "builds": [...] }`. Each step runs
against a read-only export of that commit in a capability-free,
network-disabled OCI container. Only `$VIVARIUM_OUTPUT` is writable and its
regular files become immutable artifacts (256 MiB total limit).

`GET /repositories/{id}/releases/{release_id}/builds` and the following
release-build routes inherit repository visibility:

- `GET .../releases/{release_id}/attestation` gives the aggregate `unbuilt`,
  `pending`, `failed`, or `verified` result and every required step for the
  frozen source commit.
- `GET .../builds/{build_id}/events` returns ordered status and bounded logs.
- `GET .../builds/{build_id}/artifacts/{artifact_id}` downloads immutable bytes
  recorded with path, size, media type, attempt, and SHA-256.
- `GET .../builds/{build_id}/attestation` returns source commit, exact command,
  container-image dependency, actor, verification attempts, and artifacts.
- `POST .../builds/{build_id}/rerun` appends a same-source attempt; earlier
  attempts, logs, failures, and artifacts remain unchanged.

### Attested package publication

Only the repository owner can publish a package from a release. `POST
/repositories/{id}/releases/{release_id}/packages` requires
`repositories:write` and accepts one artifact from the current successful build
attempt:

```json
{
  "name": "project-sdk",
  "version": "1.4.0",
  "build_id": "0123456789abcdef0123456789abcdef",
  "artifact_id": "fedcba9876543210fedcba9876543210",
  "platform": {"os": "linux", "architecture": "amd64", "runtime": "go1.24"},
  "dependencies": [{"name": "core-kit", "constraint": "^2.0.0"}],
  "summary": "Typed client for the project API",
  "documentation": "Install project-sdk and verify the registry checksum.",
  "license": "MIT",
  "visibility": "public"
}
```

Names are lowercase package identities and bind globally to the repository that
first publishes them. Versions are immutable and unique within that identity.
The server derives the release, source commit, build attestation, artifact path,
size, media type, checksum, and publisher. It holds the selected build's
execution boundary while streaming and re-hashing the artifact and atomically
exposing metadata and bytes, so a concurrent rerun cannot supersede the
attested attempt. A stale or failed build, checksum mismatch, conflict, or
interrupted copy cannot expose a partial version. Parent-directory open, sync,
or close failure after rename is returned rather than acknowledged; the
complete version may already be visible, but its durability is uncertain. That
case returns `202`, `Vivarium-Durability: uncertain`, `Location`, and the full
package record including its ID. An exact retry returns `200` with the same
record and ID; a different publication at that name/version returns `409`.
New-identity failures before rename do not reserve the name.

`GET /repositories/{id}/packages` lists versions originating in a readable
repository. `GET /packages/{name}/versions/{version}` returns immutable
provenance and lifecycle metadata, and `GET
/packages/{name}/versions/{version}/artifact` returns the bytes with
`X-Checksum-Sha256`. Public versions allow anonymous reads; private versions
continuously inherit the source repository's current read policy. Package
records live beneath `$PACKAGE_STORAGE_ROOT` (default `packages`).

`GET /packages?q={text}` searches package names, summaries, and documentation,
returning only public versions and private versions currently readable by the
caller. `GET /packages/{name}/versions` lists the visible version history.
`GET /packages/{name}/resolve?constraint=^1.2.0&os=linux&architecture=amd64`
returns the newest authorized, non-yanked compatible version. Resolution
supports exact, `^`, `~`, `>`, `>=`, `<`, and `<=` constraints; omitted
platform selectors and blank published selectors act as portable values.

Owners set known lifecycle risk with `PATCH
/repositories/{id}/packages/{name}/versions/{version}` and
`{"lifecycle":"deprecated","warning":"Move to 2.x"}` (or `yanked`). Returning
to `active` requires an empty warning. This changes discovery guidance only;
the immutable bytes and provenance remain available for verification.

`POST /repositories/{id}/package-credentials` requires current participation
and accepts `{"name":"CI install","package_names":["project-sdk"],
"expires_in":3600}`. Every name must have an authorized published version at
issuance. The returned bearer token has only `packages:read`, the consuming
repository ID, the exact sorted package allowlist, and a 60-second to 24-hour
lifetime. It works with catalog, version, resolution, and artifact reads but
cannot enumerate or download unrelated private identities and carries no
publisher or repository mutation authority.

### Exact dependency inventories

Repository commits may define `.vivarium/packages.json`:

```json
{"version":1,"dependencies":[{"name":"project-sdk","constraint":"^1.4.0"}],"lock":[{"name":"project-sdk","version":"1.4.2"},{"name":"core-kit","version":"2.3.0"}]}
```

`POST /repositories/{id}/dependency-inventories` with `{"commit_id":"..."}`
requires repository write participation and reads that file from the verified
commit. It records the caller, direct and package-derived transitive paths,
exact versions, license and support claims, resolution state, and provenance
gaps. An exact retry returns the existing immutable snapshot.

`GET /repositories/{id}/dependency-inventories` inherits repository read access
and projects whether each source revision is current plus matching releases,
builds, artifacts, and deployments. Each deployment record includes `current`,
which is true only for the newest successful promotion in that environment;
superseded successes remain explicit historical evidence. `GET
/packages/{name}/versions/{version}/consumers` returns direct and transitive
exact-version consumers, filtering every consuming repository through the
caller's current access. Package publication accepts an optional `support`
contact alongside `license`.

### Versioned interface relationships

Repository participants publish a named interface from immutable release
evidence with `POST /repositories/{id}/interfaces`:

```json
{"name":"events","release_id":"0123456789abcdef0123456789abcdef"}
```

The server derives `version` and `commit_id` from that repository's release;
callers cannot substitute either value. Semantic versions use the exact
`vMAJOR.MINOR.PATCH` form. A consumer records an immutable claim with `POST
/repositories/{id}/dependencies`:

```json
{
  "commit_id":"0123456789abcdef0123456789abcdef01234567",
  "release_id":"0123456789abcdef0123456789abcdef",
  "environment_id":"fedcba9876543210fedcba9876543210",
  "provider_repository_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "interface_name":"events",
  "constraint":">=v1.0.0 <v2.0.0"
}
```

The consumer commit must be verified. Optional release evidence must resolve
to that exact commit, and optional environment evidence must belong to the
consumer repository. Constraints support `*` or space-separated `=`, `<`,
`<=`, `>`, and `>=` comparisons.

`GET /repositories/{id}/relationships` returns `repositories`, `interfaces`,
and `dependencies` visible to the caller. It derives every dependency's
`resolved`, `stale`, or `unresolved` state on read. Resolution names the newest
visible non-stale interface satisfying the constraint. Missing or mismatched
release records, removed environments, and environments without a successful
active deployment of the declared exact revision are reported with explicit
reasons. Failed, canceled, or superseded attempts do not replace the newest
successful deployment as the environment's active evidence.
Private repository identities and edges are omitted unless the caller still
has repository access. Records live beneath `RELATIONSHIP_STORAGE_ROOT`, which
defaults to `relationships`.

An interface evolution turns that evidence into a shared pre-merge decision.
`POST /repositories/{id}/evolutions` requires provider participation and an
open proposal or pull request, one exact published predecessor, a candidate
description, classified `compatible`, `conditional`, `breaking`, or `unknown`
changes, and migration strategy/sequencing. Creation freezes every consumer
declaration and owner currently readable to the author. Collection and detail
reads continue to filter private consumer impacts under current access, while
`PATCH` compare-and-swaps `version` for contract edits.

Current participants in a frozen affected consumer use nested `POST
/acknowledgements` without gaining provider write access. Nested `POST
/analyses` delegates selected readable repositories from the frozen radius for
300-86400 seconds. Its `evolutions:analyze` credential can read that packet and
append attributable findings plus explicit uncertainty citing only the
selection; it has no repository, Git, proposal, pull, credential, or deployment
write scope. Analysis packets include contract candidates only when every
frozen pull target and source is both explicitly selected and still readable by
the initiator; this filtering applies equally to analysis reads and finding
publication responses.

`POST /repositories/{id}/evolutions/{evolution_id}/migration-tasks` adds the
next ordered unit of work using the plan's compare-and-swap `version`. The body
names a provider or frozen consumer repository, target version, earlier task
dependencies, completion criteria, exact base, mandate, and human or agent
assignee. The caller and any human assignee must already participate in the
target repository. It creates an ordinary proposal task there, whose discussion,
assignment, session, branch, pull, and merge state are projected into plan reads.
Plan authors receive no consumer permission as a side effect.

`POST /repositories/{id}/evolutions/{evolution_id}/contract-candidates`
accepts `provider_pull_request_id` and a `consumer_pull_request_ids` object keyed
by frozen affected repository ID. Every value must name an open pull and the
caller must be a provider participant who can still read every selected
consumer. The provider pull supplies `.vivarium/contracts.json` using the same
bounded definition schema as `.vivarium/checks.json`. The response freezes the
exact source repositories and commits, deterministic synthetic commit,
combination SHA-256, requester, and check IDs. Exact duplicate combinations
return `409`; a changed provider or consumer revision supersedes only earlier
rows containing that changed repository.

Nested `GET .../contract-candidates/{candidate_id}/checks` returns current run,
attempt, failure, artifact, and combination attestation projections. Add
`/{check_id}/events` for immutable bounded logs or
`/{check_id}/artifacts/{artifact_id}` for checksummed output. Every read
revalidates access to every pull target and source repository in that matrix
row, so target access cannot disclose private-fork metadata or evidence. Execution mounts the
assembled source read-only with no network and no API or Git credential.
The final access check, candidate/check publication, and creation response hold
the repository catalog mutation lock, so private-source revocation or visibility
changes linearize wholly before or after evidence publication.

`PUT /repositories/{id}/evolutions/{evolution_id}/rollout` compare-and-swaps
`version` to freeze one non-superseded contract candidate and ordered phases.
Every candidate check must already be durably readable and successful. Each
phase names repositories once, explicitly maps each repository to the one
migration task whose contribution advances it, and may map repositories to
existing environment IDs.
`POST .../rollout/approvals` accepts approval only from the named repository's
current owner; approval grants no cross-repository authority. Plan reads derive
the gate from its exact check runs and each phase from the linked pull merge,
an ancestry-containing release, and any named environment's promotion. The
projection reports ownership, retained resource IDs, readiness, and the next
ordinary queue/release/promotion action. Closed pulls and failed or canceled
promotions pause the affected phase and direct participants to the deployment's
existing rollback or repair controls; completed earlier phases are preserved.
For retries, only the newest matching release/environment promotion is
authoritative, using its store-assigned durable creation sequence even when
wall-clock timestamps collide; retained earlier failures remain evidence but
do not pause a later successful attempt. An unavailable check store fails
rollout creation closed with `422 invalid_rollout`.
Private participant filtering happens before aggregate phase and rollout state
is derived, so hidden workflow status cannot influence a reader's projection.

Unfinished cross-repository dependencies block an agent session from starting;
the eventual credential remains limited to its isolated `agent/tasks/*` branch.
Human work uses existing access and may publish its task contribution from the
target repository or an owned fork by supplying `source_repository_id`.

### Governed release environments

`GET /repositories/{id}/environments` returns ordered environment policy.
Owners create environments with `POST /repositories/{id}/environments` and
replace one with `PUT /repositories/{id}/environments/{environment_id}`:

```json
{"name":"production","position":2,"image":"alpine:3.22","command":"deploy \"$VIVARIUM_ARTIFACT\"","timeout_seconds":600,"configuration":{"REGION":"us-east"},"credentials":{"DEPLOY_TOKEN":"write-only"},"required_approvals":2,"concurrency":1}
```

Positions and names are unique. Configuration is readable to repository
readers. Credential values are encrypted at rest and never returned; responses
contain sorted `credential_names` only. Omitting `credentials` on replacement
preserves the protected values.

`POST /repositories/{id}/deployments` requires a current owner or contributor
with `repositories:write` and accepts `environment_id`, `release_id`,
`build_id`, and `artifact_id`. The build must have succeeded for the release's
exact commit and contain that checksummed artifact. A later environment accepts
only the identical release/build/artifact/checksum after success in the
immediately preceding environment. One pending or active promotion excludes a
second request for the environment.

The exact release commit must also contain `.vivarium/deployment.json`. Version
1 freezes ordered rollout stages and executable health signals into the
deployment record:

```json
{"version":1,"stages":[{"name":"canary","observation_seconds":30,"signals":[{"name":"service health","command":"wget -qO- $HEALTH_URL | grep -q ok"}]},{"name":"full rollout","observation_seconds":120,"signals":[{"name":"error budget","command":"./observe-errors"}]}]}
```

Each signal runs in the environment's isolated image with the exact artifact
and scoped values. Its pass/fail output, stage, time, and affected commit are
retained publicly with the deployment; a failure terminalizes rollout.

`POST /repositories/{id}/deployments/{deployment_id}/approvals` records one
approval from a participant other than the initiator. Reaching the environment
threshold queues execution, subject to its concurrency limit. `GET
/repositories/{id}/deployments` returns the durable history, including exact
artifact SHA-256, initiator, approvals, current state, timestamps, and ordered
request, queue, deployment-status, log, and completion events. Reads inherit
normal repository visibility; protected values never enter this history.
The executor reopens and SHA-256 verifies the artifact, mounts it read-only at
`$VIVARIUM_ARTIFACT`, and runs the environment command in its configured image
with dropped capabilities, a read-only root, and a bounded timeout. Protected
values exist only in the executor's mode-0600 environment file; logs are
bounded and secret-redacted. Verification or command failures are terminal and
do not satisfy the next environment. Queued records resume after restart;
running records retain a renewable execution-owner lease so periodic recovery
does not contradict a live command, including during slow executor cleanup.
Finalization compare-and-swaps that owner; an expired lease becomes failed with
an unknown-outcome event. Pre-execution policy failures terminalize the queued
record so they cannot consume concurrency indefinitely.

Participants with `repositories:write` may `POST
/repositories/{id}/deployments/{deployment_id}/controls` with `action` set to
`pause`, `resume`, `cancel`, or `mark_unsuccessful`, plus the currently observed
`expected_state` and an optional `reason`. State comparison prevents stale
decisions. Every intervention retains its actor and reason in deployment
history and creates recipient-specific inbox activity for the initiator.

### Failed deployment recovery

Current owners and contributors with `repositories:write` may `POST
/repositories/{id}/deployments/{deployment_id}/recoveries` for a deployment in
`failed` or `canceled` state. Include the observed state to reject stale
decisions:

```json
{"action":"rollback","expected_state":"failed"}
```

`rollback` derives the newest earlier `succeeded` deployment to the same
environment. It creates a new ordinary promotion for that exact release,
build, artifact, checksum, commit, and rollout definition, returning `202`
with `deployment` and `restores_deployment`. The new record carries
`recovery_kind: "rollback"`, `recovery_of`, and `restores_deployment_id`.
Environment concurrency, prior-environment provenance, independent approvals,
artifact verification, and health observation still apply. `409
rollback_unavailable` means no earlier known-good deployment exists;
`rollback_blocked` means current delivery policy prevents admission.

```json
{"action":"repair","expected_state":"failed"}
```

`repair` returns `201` with `pull_request` and `session` and a `Location` for
the session workspace. The server creates a deterministic, deployment-keyed
`agent/recovery/*` branch at the current default-branch tip and an ordinary open
pull against that default branch. This prevents intervening integrated work
from appearing as repair deletions. `session.deployment_evidence` immutably snapshots release
version and notes, deployment and environment IDs, source commit, artifact ID
and SHA-256, terminal state, current rollout stage, health evidence, and
ordered attributed deployment events/logs. Launching an agent uses the normal
pull-session API and issues only repository/repair-branch Git access. Agent
completion synchronizes the pull, starts exact-revision checks, and remains
subject to review, integration policy, merge, new release build, and governed
promotion; it grants no environment configuration, credential, approval, or
execution authority.
Retries locate the same branch and pull and publish or reconnect its one repair
session after a definitive or uncertain session-storage failure. Rollback
creation similarly derives the current known-good target, rechecks the
unhealthy deployment, and writes the promotion within one deployment-store
critical section.
If publication stopped after creating only the deterministic Git ref, retry
reconciles it to the latest default tip only by an ancestry check and
compare-and-swap fast-forward. A divergent or concurrently changed ref returns
`409 repair_branch_changed` and is never overwritten.
Pull and session publication enforce deployment-recovery uniqueness under
their cross-process store locks. Simultaneous identical repair commands may
return the newly created or already connected resource, but all responses name
the same pull request and change session.
## Incidents

Authenticated repository collaborators establish a durable shared operating
picture through `GET /incidents`, `POST /incidents`, and
`GET /incidents/{incident_id}`. An incident declares a title, impact summary,
`sev1` through `sev4` severity, one or more affected repository scopes with
optional environment IDs, and named response roles. The declaring actor must
currently participate in every affected repository; role holders must
participate in at least one affected repository. Reads are limited to current
participants of an affected repository, so access revocation applies without
copying repository membership into the incident record.

Declarations may be manual or carry `source` with an affected
`repository_id`, `deployment_id`, and optional `stage` and `signal`. A named
signal must match durable evidence on that deployment. This retains exact
operational provenance while leaving deployment execution state authoritative
in the deployment store. `INCIDENT_STORAGE_ROOT` selects incident storage and
defaults to `incidents`.

`POST /incidents/{incident_id}/investigations` delegates diagnosis with a
`mandate`, up to 20 previously verified `evidence` selectors, up to 20 exact
`revisions` (`repository_id` and commit `commit_id`), and an optional
`expires_in` from 300 through 86400 seconds. The response returns the updated
incident, durable investigation, and a one-time API credential carrying only
`incidents:investigate`. `GET
/incidents/{incident_id}/investigations/{investigation_id}` uses that credential
to return the frozen packet plus delegation-time snapshots of only its selected
operational resources. Later incident, pull, release, or deployment mutations
cannot silently widen that context.
The agent appends `finding`, `tool_action`, `question`, or `uncertainty` with
`POST .../events`; all become agent-attributed participant timeline entries.
Responders use `POST .../controls` with `guide`, `pause`, `resume`, or `cancel`.
Paused investigations reject agent publication, and cancellation revokes the
credential. This scope grants no repository mutation, Git, deployment,
environment, credential, approval, or secret-management API access.

`POST /incidents/{incident_id}/actions` turns diagnosis into a governed
mitigation proposal. The request names `pause_rollout`, `restore_release`, or
`emergency_repair`, an affected repository and deployment, a rationale, one to
20 verified evidence selectors, and one to 20 `health_criteria` stage/signal
pairs declared by that exact deployment. It also requires a stable
`operation_id`; an exact replay returns the original action without duplicating
timeline history, while changed reuse conflicts. `POST .../actions/{action_id}/decisions`
records an approval or rejection. A proposer cannot approve their own action
unless the caller sets `override: true`; that exception remains explicit in
the immutable decision history.

Approved work executes through the ordinary deployment control or recovery
routes, so environment membership, approval, concurrency, artifact, and change
workflow rules remain authoritative. `POST .../actions/{action_id}/attempts`
records each accepted or failed attempt with its actor and resulting deployment
or pull identity. Execution first persists a `pending` attempt under a stable
operation ID, then finalizes that same record after the environment operation;
this reservation prevents a lost audit response from making the mutation
silently repeatable. A `recovered` attempt requires a matching validated
governed attempt and must reference a retained deployment in the affected
environment on which every declared stage/signal criterion has passed;
otherwise the API returns `409 recovery_unverified`. Failed attempts remain
visible and may be followed by later governed attempts or verified recovery.
Pause controls carry the action and reserved-operation IDs in their durable
deployment event, so a historical control cannot be claimed by a later
authorization. Emergency-repair recovery additionally requires the governed
repair pull to be merged and its immutable merge commit to be in the exact
recovery deployment commit's ancestry.

`PATCH /incidents/{incident_id}` accepts `expected_version`, `severity`,
`status`, `roles`, and an optional decision `message`. Status is one of
`investigating`, `identified`, `monitoring`, or `resolved`; a stale version
returns `409 incident_changed`. Every accepted change appends an immutable,
actor-stamped timeline entry rather than replacing response history.

Resolution is a review publication, not only a status toggle. `PUT
/incidents/{incident_id}/resolution` compare-and-swaps `expected_version` and
requires an impact statement, review timeline, one to 20 contributing factors,
and conclusions. The server retains the publishing actor and time, marks the
incident resolved, and appends an immutable `incident_resolved` timeline entry.

After the review exists, `POST /incidents/{incident_id}/commitments` creates a
normal proposal and human-assigned executable task in an affected repository,
then links their stable IDs, accountable assignee, exact base revision, and due
time back to the incident. The request carries a stable `operation_id`; one
proposal-store transaction publishes the proposal, task, and assignment under
that identity, and the incident link reuses it after a partial cross-store
failure. Exact retries therefore converge without orphaning duplicate work.
The target repository's catalog lock holds current actor and assignee access
through proposal publication and incident linking. Repository participation,
branch-base verification, and later task context rules remain authoritative in
the proposal APIs rather than being copied into the incident.

Incident reads derive each commitment's current progress from that authoritative
work. They report invalidated assignments or obsolete context, overdue open
work, a published pull and its exact check states, completion by merge, and any
release candidates and deployments whose server-derived inclusions contain the
task. The assignee receives a response inbox item while the commitment remains
open; its action changes to overdue or invalidated repair when those conditions
arise and disappears after merge completion.

`POST /incidents/{incident_id}/updates` accepts a caller-generated opaque
128-bit lowercase hexadecimal `operation_id`, a `message`, and an `audience`
of `participants` or `public`. Audience is retained on the update so later
status-page and external communication surfaces can safely select explicitly
public copy. Current participants acknowledge any timeline entry with `POST
/incidents/{incident_id}/timeline/{entry_id}/acknowledgements`; acknowledgers
are retained by stable user ID and repeated acknowledgement is idempotent.
Repeating an update with the same operation ID and exact content returns the
original durable result without another timeline entry; reusing it for changed
content returns `409 incident_changed`. Clients retain the operation ID across
uncertain transport or durability failures before retrying.

`POST /incidents/{incident_id}/findings` makes diagnosis part of that same
durable timeline. It accepts an `operation_id`, a `kind` of `observation`,
`hypothesis`, `query`, or `conclusion`, a `message`, an `audience`, and one to
20 `evidence` references. Evidence kinds are `log`, `health_signal`,
`deployment`, `release`, `commit`, `pull_request`, and `incident`; every source
names an affected `repository_id` and the source's `resource_id`. Optional
`query`, `window_start`, and `window_end` retain the responder's selection.
Logs and health signals require an ordered time window; the server verifies a
retained deployment event or the exact `stage/signal` query occurred inside
that interval. Other sources are verified against their authoritative store.

The server ignores caller labels and snapshots a source label and
`captured_at` time when the finding is published. Reads therefore expose both
historical diagnostic context and stable identifiers for inspecting the live,
authoritative source. Findings inherit incident participation checks, retain
their explicit participant/public audience, and use the same idempotent
operation-ID conflict contract as updates.
## Private security advisories

Security reports are a deliberately separate collaboration boundary. They are
not repository proposals, incidents, activity events, inbox items, or search
documents. All routes require an authenticated credential with
`repositories:read`.

- `POST /security-advisories` accepts `title`, `description`, a required safe
  `contact`, one or more `affected_repositories` containing a
  `repository_id` and non-empty `versions`, and bounded `evidence` entries with
  `label` and `description`. The reporter must currently be able to read every
  named repository; this permits an outside researcher to report against a
  public repository without becoming a collaborator.
- `GET /security-advisories` returns only reports available to the caller under
  the shared `limit`/`after` cursor contract. `GET
  /security-advisories/{advisory_id}` also records a durable `viewed` access
  event.
- `PATCH /security-advisories/{advisory_id}` accepts `expected_version`,
  `severity` (`low`, `moderate`, `high`, or `critical`), and `embargo_state`
  (`reported`, `triaging`, `embargoed`, or `coordinating`). Only a current owner
  of an affected repository may make this compare-and-swap triage decision.
- `POST /security-advisories/{advisory_id}/responders` accepts a stable
  `user_id`. An affected repository owner may invite at most 20 responders.
- `POST /security-advisories/{advisory_id}/messages` accepts a bounded `body`
  from any authorized participant.
- `POST /security-advisories/{advisory_id}/evidence` connects a verified
  `commit`, `dependency`, `release`, `build`, `artifact`, or `deployment` from
  an affected repository, with a bounded label and description. Dependency
  evidence must name an exact release build and match its immutable container
  image dependency; free-form dependency claims are rejected.
- `POST /security-advisories/{advisory_id}/findings` records an attributable
  `hypothesis`, `conclusion`, or `uncertainty`. `PUT
  /security-advisories/{advisory_id}/impact` compare-and-swaps one
  repository/version-line/environment cell to `confirmed`, `suspected`,
  `unaffected`, or `fixed` with rationale and supporting evidence.
- `POST /security-advisories/{advisory_id}/investigations` accepts a mandate,
  selected `evidence_ids`, and a 300-86400 second expiry, returning a
  `security:investigate`-only credential. The nested investigation `GET`
  returns only its frozen selection; nested `POST /findings` can cite only
  delegated evidence and publishes solely into the embargoed advisory.
- `POST /security-advisories/{advisory_id}/repair-tasks` creates a human or agent
  repair for an exact affected repository, version line, verified base commit,
  response-team assignee, and optional same-advisory dependency tasks. `POST
  .../repair-tasks/{task_id}/sessions` creates one isolated
  `vivarium-security/*` branch and returns a revocable, exact-branch Git
  credential. Session comments, reviews, completion, and revocation stay inside
  the advisory; completion verifies the live tip descends from the frozen base.
  Assignment and launch require current participation in the task repository;
  response-team membership alone grants no Git access. Mutation authority is
  limited to the exact worker, session initiator, or explicit task creator while
  that creator remains a task-repository participant; unrelated collaborators
  cannot alter session history or lifecycle.
  A base is valid only when it belongs to the repository's owner-controlled
  default-branch ancestry; orphan objects and unmerged feature commits are
  rejected before they can supply trusted verification definitions.
  Revocation removes the isolated ref so an open task can start a fresh session.
- `POST /security-advisories/{advisory_id}/reproductions` lets the affected
  repository owner define an embargoed container reproduction for one exact
  affected version line. Definitions use the same bounded image, command,
  working-directory, environment, and timeout validation as repository checks,
  but remain inside the advisory authorization boundary.
- `POST .../repair-sessions/{session_id}/verifications` resolves the target
  branch's required names to executable definitions frozen at the task base,
  then reserves those checks plus every private reproduction for the task's line
  against the completed session's exact commit after a non-worker has approved
  that same commit in protected review. `GET
  .../verifications/{verification_id}` exposes response-team-safe names, states,
  commit identity, and artifact checksums without commands or logs. An exact
  candidate has one proof set.
- `POST .../verifications/{verification_id}/approvals` requires every reserved
  run to have succeeded at the exact candidate and requires an affected
  repository owner other than the repair worker. An approved proof is ready
  for the repository's protected integration policy.
- `POST .../verifications/{verification_id}/release-attestations` accepts a
  release ID only when the release commit contains the verified candidate, all
  exact release build steps succeeded, and they produced checksummed artifacts.
  The retained attestation maps those artifacts back to the supported version
  line, allowing clients to show unproved or unshipped coverage gaps.
- `POST /security-advisories/{advisory_id}/disclosure` compare-and-swaps the
  advisory version and accepts a redacted summary, actionable upgrade guidance,
  public credits, and an optional `scheduled_at`. It fails unless every claimed
  repository/version line has an exact release attestation, then freezes the
  affected/fixed matrix, release commits, artifact IDs and SHA-256 checksums,
  and deterministic repaired branch names.
- `POST /security-advisories/{advisory_id}/disclosure/publish` publishes or
  resumes a due packet. Exact commits are first staged beneath the
  transport-hidden `vivarium-security/disclosures/*` namespace. After the
  anonymous advisory is durably public, repaired branches and targeted
  owner/deployment-initiator notifications are idempotently emitted. A
  pre-publication failure returns `disclosure_paused`; a later failure returns
  `disclosure_incomplete` while retaining public availability and exact
  `remaining` work for responders.
  Repository branch listing and named-revision browse routes never expose the
  `vivarium-security/*` staging namespace, including after failed cleanup.
- `GET /security-advisories/public` and `GET
  /security-advisories/public/{advisory_id}` are anonymous and return only fully
  published disclosure projections. Private evidence, findings, messages,
  reporter contact, investigation data, reproduction definitions, and logs are
  never present.

A report is discoverable only by its reporter, current owners of affected
repositories, and its explicitly invited response team. Unauthorized detail
reads return not-found. The record retains immutable attributed report,
triage, invitation, message, and detail-access events in `access_log`; it also
retains the reporter contact channel and all protected messages. No advisory
operation emits ordinary repository activity or inbox notifications. Durable
records live beneath `$SECURITY_ADVISORY_STORAGE_ROOT`, which defaults to
`security-advisories`, with owner-only directory and record permissions.
The Git transport hides `refs/heads/vivarium-security/` from ordinary clone,
fetch, and push discovery, including owner credentials. Repair credentials
advertise only their exact branch and grant no repository membership.

## Organizations

Authenticated users create durable groups with `POST /organizations` and list
their memberships or pending invitations with `GET /organizations`. An owner
invites a stable user ID through `POST /organizations/{id}/invitations`; only
that user accepts at `POST .../invitations/{invitation_id}/accept`. Owners may
remove non-owner members with `DELETE /organizations/{id}/members/{user_id}`.

Members create a repository directly in the group with `POST
/organizations/{id}/repositories`. An existing individual repository moves
through an explicit handshake: its current custodian requests `POST
/organizations/{id}/repository-transfers`, then a group owner accepts `POST
.../repository-transfers/{transfer_id}/accept`. Acceptance sets the
repository's `organization_id` without changing its `id`, `owner_id`, Git
remote, refs, creation time, or any repository-scoped evidence. Group access is
tracked separately from independent collaborator grants. The request endpoint
returns only the pending transfer record, never the target organization's
membership roster. Every organization mutation requires `repositories:write`;
read-scoped credentials can only inspect authorized organization resources.

`GET /organizations/{id}/portfolio` returns the group's repositories,
published packages, open proposals and pulls, releases, and unresolved
incidents. It also projects durable portfolio initiatives created with `POST
/organizations/{id}/initiatives`. Each initiative names an existing proposal,
evolution plan, incident, or authorized security advisory and retains ordered
repository work items, optional contribution links, dependencies, and an
accountable organization human, team, or approved agent. Creation rejects
sources outside their authoritative store and repositories outside the current
portfolio. Proposal and evolution sources and contribution links revalidate
current owner/collaborator access on creation and every projection. Incident
sources additionally require an exact affected repository;
creation and every read revalidate that the actor remains its owner or
collaborator, so an organization record cannot disclose a private incident.
`PATCH
/organizations/{id}/initiatives/{initiative-id}/items/{item-id}` compare-and-swaps
the initiative version when changing owner or status. Portfolio reads derive
incomplete dependency blockers, current ownership health, matching active or
pending policy exceptions, and release candidates. Removed members/operators,
deleted teams/agents, and transferred repositories remain on the record as
`reassignment_required` work instead of being silently dropped. Only accepted
members can read the portfolio. Records live beneath
`$ORGANIZATION_STORAGE_ROOT`, which defaults to `organizations`.

Owners create nested responsibility groups with `POST
/organizations/{id}/teams`. A team has a unique slug, optional `parent_id`, and
`public` or `organization` visibility. `PUT .../teams/{team_id}/members` adds or
updates an accepted organization member as a `member` or `maintainer`, while
`DELETE .../members/{user_id}` removes direct membership. Both take
`expected_version`; stale concurrent commands return `409`. `POST
.../teams/{team_id}/responsibilities` associates an organization repository
and named area at the same compare-and-swap boundary. These associations
describe stewardship and do not grant repository authority.

`POST /organizations/{id}/agents` registers an approved identity with a name,
unique slug, visibility, non-empty capabilities, current member operators, and
optional teams. Approval creates no credential or access. Unauthenticated `GET
/organizations/{id}/directory` returns public teams, explained direct and
nested effective membership, public-repository responsibility, and public
agents. Members additionally receive organization-visible records and the
immutable actor-stamped event history. Private repository responsibility and
organization-only nesting are omitted from public projection.

Authenticated organization members use `POST /organizations/{id}/agent-matches` with a closed
`source_kind` (`task`, `proposal`, `issue`, `decision`, `incident`, `stewardship_mandate`, or
`team_role`), bounded `source_id`, `workflow`, optional `repository_id` and closed
`deployment_boundary` (`platform`, `operator_managed`, `customer_managed`, or `external_service`), and
exact `required_permissions`. Every candidate exposes eligibility, transparent score reasons, live
resource grants, policy conflicts, cost, availability, execution boundary, verified evaluations,
comparable attributed outcomes, and explicit missing or stale evidence and conflict disclosures.
The response retains no source body; it grants no permission and does not assign or launch an agent.
Private matching evidence never enters the unauthenticated directory projection.
Deployment compatibility uses exact structured profile disclosures; free-form execution provenance and
remote-processing explanations are displayed as context but never satisfy the boundary constraint.
When a search requests a boundary and the current profile has no structured disclosure, the candidate
remains visible but ineligible with missing evidence rather than a contradictory-boundary conflict.

Organization owners create ongoing agent responsibility contracts with `POST
/organizations/{id}/stewardship-mandates`. Each immutable revision names
desired outcomes, organization repositories and branches, trusted signals,
explicit exclusions, agent-minute/action budgets, start and expiry, one
approved agent, allowed actions, and decisions reserved for humans. The
agent's current operator accepts the exact version through `POST
.../stewardship-mandates/{mandate_id}/accept`; `PUT` publishes a new revision
and clears prior acceptance. Owner-only `pause`, `resume`, and `revoke`
commands require `expected_version`, while expiry is derived from the latest
revision's schedule. `GET .../{mandate_id}/preview` reports matching live
portfolio access grants and per-repository effective policy, plus an explicit
empty `implicit_authority` list. A mandate never creates a credential or
grants Git write, review, merge, deployment, or other repository authority;
those require the existing independently approved grant paths.

Accepting a mandate appends a `stewardship_evaluation.requested` activation
event. Trusted evidence producers use `POST
.../stewardship-mandates/{mandate_id}/evaluations` at activation and after
repository, dependency, check, release, incident, security, or usage changes.
The accepted operator submits bounded findings whose `signal` exactly matches
a trusted signal and whose repository appears in the current revision. Each
finding names its evidence identity and revision, severity (`critical`, `high`,
`medium`, or `low`), expected value, confidence from 0 through 1, affected
owner IDs and revisions, citations, and an explicit in-scope explanation. The
endpoint confers no evidence-source access: producers may submit only evidence
they can already inspect through independent authority.

The server derives a stable deduplication identity when `dedupe_key` is omitted;
a producer may supply one to converge renamed evidence. A newer evidence
revision updates that opportunity and retains earlier citations with `stale:
true`. New items consume the mandate action budget; reevaluation does not.
Accepted members inspect the rank-ordered queue at `GET
.../{mandate_id}/opportunities` and post compare-and-swap `rank`, `dismiss`,
`snooze`, `incorrect`, `reopen`, or `comment` actions to
`.../opportunities/{opportunity_id}`. Decisions and discussions retain actor,
reason, version, and time; stale concurrent decisions return `409`.

Mandate revisions may additionally define one `opportunity_policies` rule per
evidence class. A rule selects a minimum severity, `approval_required` or
`auto_start`, and a per-opportunity agent-minute ceiling; unlisted classes fail
safe to maintainer approval. Owners compare-and-swap `approve` or `reject`
through the same decision endpoint. Approval authorizes promotion only: it does
not start compute, create a branch, or grant repository authority.

`POST .../opportunities/{opportunity_id}/promotion` freezes the current default
branch SHA and atomically reserves the opportunity before creating one ordinary
proposal with one to twenty ordered, explicitly human- or approved-agent-owned
tasks. Every task carries observable completion criteria, risk, and a
verification plan; the proposal reasoning retains the organization, mandate,
opportunity, evidence, and exact base revision. Active incidents, embargoed
security evidence, same-title open work, exhausted action/agent-minute budgets,
a moved base, changed mandate policy or acceptance, and missing approval return
named blockers. Incident and proposal conflict storage must both be readable
before reservation; unavailable state returns `503` without changing the
opportunity. Reevaluation clears any earlier approval and advances a separate
evaluation version, which the approval must exactly match at reservation.
An unlinked reservation retry recomputes external blockers and revalidates the
current mandate, acceptance, policy, approval, operator, and already-consumed
budgets without charging them twice. Final linking repeats the organization
governance checks, so pausing or revising a mandate cannot authorize completion
through a stale same-actor retry.
Reservation and opportunity versions make concurrent approval
or promotion a conflict, while exact retries reconcile the same proposal and
task identities. Accepted work is linked from the opportunity and task
assignment/promotion activity feeds the established activity and inbox views.

For a promoted agent task, `POST
/repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/sessions` also
revalidates the active accepted mandate, its current operator and approved
agent, the linked opportunity, and exact recorded base. Changed governance
returns `409 stewardship_authority_changed` before branch or credential
creation. Task completion accepts bounded `commands` (`command`, `exit_code`,
optional `summary`) and `completion_criteria` (`criterion`, `status` of `met`,
`partial`, or `not_met`, and `evidence`). Stewarded publication requires a
command and the exact promoted task criterion status or returns `409
stewardship_evidence_incomplete`. Its ordinary pull exposes immutable
`task_evidence` containing the exact base, assignment, initiator and agent,
mandate and stewardship identities, cited reasoning, server-derived commits
and files, command/check claims, residual concerns, and criterion status. This
evidence grants no check, review, queue, fork, acknowledgement, or merge
exception.
An exact completion retry returns the retained outcome; a same-commit retry
whose normalized summary, commits, files, checks, commands, criteria, or
residual concerns differ returns the existing terminal-run conflict instead of
silently discarding corrected evidence.

`GET .../stewardship-mandates/{mandate_id}/report` projects opportunity
dispositions, accepted and rejected recommendations, implementation,
verification, release, resource, false-positive, automation, and goal outcomes,
current budget use, notices, and the newest progress recorded for each mandate
goal. Owners and the accepted operator append immutable outcome evidence at
`POST .../{mandate_id}/outcomes`; every outcome carries a required client-stable
`idempotency_key`, whose exact replay returns retained state without charging
resource use or creating another notice, while a changed replay conflicts.
Three consecutive failures, inactivity,
revoked access, anomalous consumption, or crossing the accepted budget pauses
active automation and retains an actionable notice; revoking an independent
agent repository grant also pauses every affected active mandate.

Owners use `PUT .../{mandate_id}/tuning` with a tuning `expected_version` to
prioritize or ignore evidence classes already listed in the accepted revision
and raise the minimum confidence threshold. Tuning never changes the mandate
version or acceptance because it can only narrow or reorder existing evidence.
Adding a signal or evidence class, repository, action, budget, agent, or any
other scope or authority still requires a new mandate revision and fresh
operator acceptance.

Successful work linking moves an opportunity from `promoting` to `promoted`.
That durable state is required by the task-session authority check and remains
the disposition reported after delivery; `accepted` describes the maintainer's
recommendation decision, not an executable opportunity state.

Pending invitees receive only organization identity and their own invitation,
not membership, teams, agents, transfers, responsibility, or events. A public
child also omits its `parent_id` when that parent is not public. Responsibility
publication holds the repository catalog boundary across a final current-group
check and organization write; directory reads independently discard links to
repositories no longer in the current portfolio.

## Technical decisions

`POST /repositories/{repository_id}/decisions` requires repository write
participation and accepts `source` plus `scope`. Source kind is `repository`,
`proposal`, `investigation`, `incident`, `evolution_plan`, or
`stewardship_opportunity`; non-repository sources require their durable
`resource_id`. Scope contains the question, constraint and success-measure
arrays, RFC 3339 deadline, affected resource references, participants,
and a participant `owner_id`. Every owner and participant ID must resolve to a
current platform identity on creation and every scope revision. Creation
returns a durable `pending` decision;
pending is informational and creates no workflow gate.

`GET /decisions` returns decisions in repositories currently accessible to the
actor and can filter by `repository_id`, `source_kind`, and `source_id`. These
source filters let related surfaces project that a decision remains pending
without coupling their mutation rules to it. `GET /decisions/{id}` revalidates
current repository participation. Participants with current repository write
access publish compare-and-swap scope revisions through `PUT /decisions/{id}`
using `expected_version`, complete replacement `scope`, and an attributable
change `summary`; stale updates return `409 decision_changed`. `POST
/decisions/{id}/discussion` appends a bounded body without advancing the scope
version. Scope creation, every revision, and discussion remain in one ordered
immutable `history`. Records default beneath `$DECISION_STORAGE_ROOT`
(`decisions`).

`POST /decisions/{id}/alternatives` requires repository write participation
and compare-and-swap `expected_version`. Its `alternative` contains a title,
summary, assumptions, tradeoffs, risks, compatibility impact, cost, expected
outcomes, exact evidence, and one assessment for every current success measure.
Evidence kind is `code`, `dependency`, `release`, `incident`, or `usage`; every
citation requires a durable resource identity, exact revision or time window,
and label; capture time is assigned by the server. Code evidence additionally requires repository, path,
and line range. Reads derive `evidence_status.missing_kinds`,
`missing_criteria`, and citations stale after 30 days.

`POST /decisions/{id}/research-credentials` accepts `alternative_id` and
`expires_in` from 60 through 86400 seconds. It issues an API credential bound
to that repository, decision, and current alternative with
`decisions:research` plus repository-bound `repositories:read`; it has no write
scope. `POST /decisions/{id}/findings` accepts that credential
plus the selected alternative, body, `support`, `oppose`, or `neutral`
position, explicit uncertainty, exact citations, and optional
`supersedes_id`. Supersession marks but retains the earlier same-alternative
finding, preserving dissent and its historical evidence.

The exact revision may define up to 20 uniquely named experiment commands in
`.vivarium/workspace.json` as `experiments: [{"name":"benchmark","command":"..."}]`.
Create an ordinary workspace with source kind `decision_experiment` and its
`decision_id` and `alternative_id`, then `POST /decisions/{id}/experiments`
with that `alternative_id` and the running `workspace_id`. The workspace must
belong to the same repository and exact decision alternative; its isolation,
resource limits, sharing, control lease, and lifecycle remain authoritative.

`POST /decisions/{id}/experiments/{experiment_id}/evidence` compare-and-swaps
`expected_version` and accepts `evidence` containing workspace checkpoint IDs,
workspace command-outcome IDs, `{name,value,unit}` measurements, checksummed
`{label,path,sha256,size}` artifact metadata, and notes. The server rejects
foreign checkpoint or command identities and derives CPU seconds, memory MB
hours, and storage MB hours from the workspace rather than trusting the
caller. Decision reads report invalidation when the repository default branch
moves, the workspace definition differs, or workspace policy marks the
environment rebuild-required. Experiment attachment has no publication or
merge effect; checkpoint publication remains a separate governed endpoint.
Experiment launch records the default-branch commit and definition digest that
were current at registration, separately from the intentionally selected
experiment revision. Reads compare against that launch baseline, so historical
pins are not immediately stale. Identical workspace creation requests for the
same actor, decision, alternative, and commit hold a cross-process claim through
provisioning and return the existing running workspace on retry; registering
that same workspace again returns its existing experiment without duplicating
history or activity.

`POST /decisions/{id}/approval-requests` is owner-only and compare-and-swaps
`expected_version`. An `affected_owner` request must name an affected repository
and its current owner. A `policy` request must cite an applicable active
organization policy rule and a current organization owner. The named approver
responds with `approve` or `reject` at
`POST /decisions/{id}/approval-requests/{request_id}/response`; pending and
rejected requests are publication conflicts and remain visible on list/detail
surfaces.

`POST /decisions/{id}/publish` is owner-only and compare-and-swaps the live
decision version. It requires one current selected alternative, explicit
rejection of every other current alternative, rationale, accepted tradeoffs,
conditions, a future review date, retained dissent finding IDs, and exact
evidence already cited by the decision. Every request must be approved.
Policy approval requests may freeze `exception_reason` and
`exception_expires_at`. Exceptions must reference an approved policy request,
match its policy/rule and normalized exception reason, and expire in the future
no later than that approved ceiling, and each approval request can authorize at
most one exception entry. The dissent list must be a duplicate-free snapshot of
every current non-superseded opposing finding. Publication appends an immutable
numbered commitment with its approval snapshot. Later material scope,
alternative, finding, experiment, or experiment-evidence changes set the live
record back to `pending`, supersede current requests, and mark the prior
commitment `reopened`; discussion does not reopen it.

`POST /decisions/{id}/implementation` requires repository write participation
and a currently published `commitment_version`. It accepts a proposal title and
body plus one to 20 ordered tasks. Each task names `human` or `agent` ownership,
an optional generated-agent ID, zero-based `constraint_indexes` and
`success_measure_indexes`, and whether it depends on its predecessor. Every
task must cover both kinds and the complete plan must cover the accepted scope.
The server freezes the current default-branch commit and derives task outcomes
and verification plans from those exact constraints and measures. The ordinary
proposal/task assignment APIs then launch sessions or workspaces and publish
pulls; no authority is added. An exact retry returns the retained plan, while
changed reuse conflicts instead of silently replacing it. Cross-store
durability uncertainty returns `202` with stable proposal/task identities,
`recovery_pending: true`, and `Vivarium-Recovery-Implementation: pending`; an
exact retry reconciles the decision link without duplicating work.

`POST /decisions/{id}/implementation/{proposal_id}/observations` appends an
attributable delivery finding with a summary and linked `resource_kind` and
`resource_id`. Kind is `coverage`, `deviation`, `assumption_changed`,
`failed_measure`, or `incompatible_work`. Coverage remains retained evidence;
every other kind marks the linked commitment reopened and the decision pending
with the finding as its explicit revisit reason. The named review/integration
pull, check run, release, or deployment must exist and derive from that exact
proposal through its task contributions and release inclusion; arbitrary or
unrelated resource identifiers are rejected before the decision store admits
the observation. A `coverage` observation additionally requires a current
approval for a review pull, a merged integration pull, a successful check at
the pull's current source commit, or a successful deployment. A release
candidate alone is not terminal coverage. Non-coverage findings may cite
linked failed or nonterminal resources because those states are the evidence
for a deviation or revisit request. Review, integration, and check coverage is
limited to each task's authoritative current contribution; superseded pulls
remain historical evidence but cannot satisfy coverage. Deployment coverage
requires its successful release to include every current merged task pull.

## Delivery teams

`POST /repositories/{id}/delivery-teams` creates a version-one operating
charter for an existing `proposal`, `initiative`, `decision`, or
`incident_follow_up`, or a named `planned_outcome`. The authenticated caller
must already have repository write participation. `charter` requires a name,
purpose, team escalation path, and at least one participant; it may also carry
an overall budget and deadline. Each participant names a unique `human` or
organization-approved `agent`, role, responsibility, reason, escalation route,
optional budget/deadline, and `required_access` entries for the source
repository at `read` or `write` level. Requested authority outside that
repository is rejected. Creation sends no credential and changes no repository
membership.

`GET /delivery-teams` lists charters visible through repository access or a
direct invitation. `GET /delivery-teams/{id}` applies the same boundary. Every
participant includes a freshly derived `access_preview` comparing required
levels with independent current owner/collaborator/public access or a live
organization agent grant. A false `sufficient` value is a visible preflight
gap, not an implicit request or grant.

`PUT /delivery-teams/{id}` is organizer-only and compare-and-swaps
`expected_version`. It replaces the declarative charter, preserves response
state only when the shared charter and that participant's invitation terms are
unchanged, starts new or materially changed invitations as pending, and appends
an attributable history event. Shared purpose, budget, deadline, escalation,
name, or participant-composition changes require every retained participant to
respond again. `POST
/delivery-teams/{id}/participants/{participant-id}/response` accepts
`expected_version` and `decision` (`accepted` or `declined`). A human invitee
responds for themself; a current operator responds for an approved agent.

`PUT /delivery-teams/{id}/plan` lets the organizer or any accepted participant
propose a compare-and-swap `plan` revision. Every stream names a charter
participant as owner and records ordered dependencies, exact inputs, expected
artifacts, acceptance criteria, assumptions, budget, integration order, and at
least one `{repository_id, reference, revision, paths}` scope. Revisions are
40-character immutable Git identities; stream IDs and integration positions
are unique and dependencies must be acyclic.

Plan reads derive blockers for overlapping paths, duplicate artifacts, team or
owner budget overruns, and an owner's missing independent write access. The
plan does not grant that access. Every stream owner must accept a material
revision; the proposer accepts their own boundary implicitly and other affected
owners receive a `replan_acceptance` blocker. Humans and current approved-agent
operators respond through `POST
/delivery-teams/{id}/plan/participants/{participant-id}/response` with both the
team version and plan revision. Declined or pending boundaries remain visible.

`PUT /delivery-teams/{id}/streams/{stream-id}/status` lets only the accepted
stream owner (or the current operator of that approved agent) replace its
compare-and-swap operational snapshot. The snapshot reports queued, running,
paused, blocked, completed, failed, or canceled state; summary and progress;
one revision already frozen in the stream scope; bounded resource consumption;
open questions; structured blockers with an explicit recovery; and the
predicted next action. Blocker kinds cover agent failure, revoked access, stale
revision, conflicting output, exhausted budget, disconnection, and dependency
failure. Crossing the accepted stream budget forces a paused snapshot and an
explicit budget escalation. Reads also project current loss of independent
write access as a pause without transferring the stream to another principal.

`POST /delivery-teams/{id}/interventions` accepts `expected_version` and a
stream- or team-scoped `intervention`. Accepted members may guide and apply
bounded stream controls; only the organizer can control the whole effort or
reassign/narrow a stream. Pause, resume, and cancel update operational state;
guidance is retained in the immutable intervention history. Reassignment names
an already accepted participant and narrowing selects only paths already in
scope. Both authority-affecting actions create a material plan revision and
fresh owner acceptance rather than silently expanding authority. Timeline,
handoff, prior plan, status, and intervention evidence remains retained.

`POST /delivery-teams/{id}/integrations` compare-and-swaps
`expected_version`, the current `plan_revision`, one shared 40-character
`base_revision`, and exactly one contribution per stream. A contribution names
an exact live branch or a matching published workspace checkpoint, plus
criterion-to-timeline evidence, decisions, and residual risks. The server
verifies ancestry, derives paths and checkpoint authors/command IDs, projects
cost, and persists blockers for overlaps, incomplete streams, pending
handoffs, plan blockers, and missing or cross-stream acceptance evidence.

`POST /delivery-teams/{id}/integrations/{integration-id}/publish` accepts
`expected_version` and optional `target_branch` (default `main`). It revalidates
each branch and the caller's independent write permission in every repository,
then finds or creates ordinary pulls in declared integration order. Each pull
retains team, manifest, stream, and order provenance plus commits, evidence,
authors, agent actions, decisions, cost, and risk. Retry reuses the source
branch pull. Ordinary exact-revision repository checks start only after every
ordered pull is created or recovered and the complete link set is durable on
the integration; failed partial publication starts no checks. No review, check,
queue, release, permission, or merge bypass is created.
An exact retry of a published integration may use the current team version or
the immediately preceding pre-publication version. It follows the retained pull
links and idempotently creates missing exact-revision check definitions or
resumes nonterminal runs, returning `200` without changing the integration.
If repository/configuration access or check-run persistence fails for any
linked pull, publication or recovery returns retryable `503` with
`delivery_checks_unavailable` rather than reporting that recovery completed.

## Contributor pathways

`GET /repositories/{id}/contributor-pathway` returns the current published
pathway, immutable revision history, and attributed acknowledgements. Public
repository reads require no credential; private repositories use the ordinary
repository read boundary. A missing publication returns
`404 contributor_pathway_not_found`.

Owners publish with `PUT /repositories/{id}/contributor-pathway`, supplying
`expected_version` and a `pathway` containing goals, prerequisite strings,
conduct and security guidance, supported setup summary and verification
commands, communication expectations, review policy, at least one work
category, and optional requirement links. Work category `audience` is `human`,
`agent`, or `human_or_agent`. Requirement kinds are `documentation`,
`ownership`, `release`, `issue`, `proposal`, and `workspace_definition`.
Documentation links name a repository-relative `path` and may freeze a
`revision`; resource links name an exact `resource_id`. A stale expected
version returns `409 contributor_pathway_changed`. Contributors and agents
cannot publish on an owner's behalf. If a revision or acknowledgement rename
is visible but parent-directory synchronization fails, the endpoint returns
the retained identity as `202` with `Vivarium-Durability: uncertain`; exact
retries therefore do not need to guess whether the mutation committed.

Each current-pathway read derives `current`, `stale`, or `inaccessible` status
and an explanation for every requirement from current repository content and
authoritative stores. Status is a projection and is not written into immutable
history. `POST /repositories/{id}/contributor-pathway/acknowledgements` requires
an authenticated reader and an exact `version`; the same actor cannot
acknowledge that version twice. Responses always include the non-identifying
`acknowledgement_count`. Repository owners receive attributed acknowledgement
records, other authenticated readers receive only their own record, and public
anonymous readers receive no acknowledgement identities. Publication and acknowledgement records live
beneath `$CONTRIBUTOR_PATHWAY_STORAGE_ROOT`, default `contributor-pathways`.

## Repository issues

Authenticated repository participants use `GET/POST /repositories/{id}/issues`
and `GET /repositories/{id}/issues/{issue-id}` to retain structured unexpected-
behavior reports. Creation requires expected and observed behavior, low through
critical severity, an environment description, ordered reproduction steps, and
public or repository-participant visibility. `release_id` optionally binds the
report to an existing immutable release and the server derives its version.

Attachments are inline base64 evidence limited to ten 1 MiB files. The API
accepts plain-text logs, PNG/JPEG/WebP screenshots, JSON or binary traces, and
text/JSON/binary sample inputs; filenames are reduced to their basename and
media types are allowlisted. Issue creation has a separate 15 MiB encoded-body
limit so all ten raw attachment boundaries remain representable; larger bodies
return `413 issue_body_too_large`. `GET /repositories/{id}/issue-templates` provides
the canonical bug and released-regression field sets. `GET
/repositories/{id}/issue-suggestions?q=...` searches only issues visible to the
caller and removes attachments and discussion from candidate summaries, so
duplicate discovery cannot disclose private evidence.

`POST /repositories/{id}/issues/{issue-id}/comments` appends attributable
discussion. `PATCH /repositories/{id}/issues/{issue-id}` compare-and-swaps
`expected_version` while moving through open, triaged, in-progress, resolved,
or closed status. Only the repository owner may resolve or close; contributors
can report, discuss, triage, and record work in progress while an issue remains
nonterminal. Leaving a resolved or closed state is also owner-only, enforced
atomically with the versioned mutation. Opening, comments, and status changes
remain in immutable actor-stamped history. Writes sync the file
before rename and its parent directory afterward. A visible rename whose
directory durability cannot be confirmed returns the retained record as `202`
with `Vivarium-Durability: uncertain`, allowing exact identity recovery rather
than an unsafe duplicate. Durable records use `$ISSUE_STORAGE_ROOT`, defaulting
to `issues`.

Issue reproduction reuses the bounded workspace executor. `POST /workspaces`
accepts source kind `issue_reproduction` with `issue_id`, an exact `commit_id`,
and, for released reports, the issue's `release_id`; released reports reject
any commit other than the attested release commit. Optional
`input_attachment_ids` are copied into `.vivarium/reproduction-inputs/` under
attachment-ID-prefixed names only after credential filenames, secret assignments,
authorization headers, and private-key material are rejected. The container stays
network-disabled, read-only-root, resource-bounded, and credential-free.

`POST /repositories/{id}/issues/{issue-id}/reproduction-attempts` freezes a
completed issue workspace into immutable evidence. It accepts selected staged
input IDs, workspace command outcome IDs, an observed result, a `reproduced`,
`not_reproduced`, or `inconclusive` disposition, and optional workspace-local
artifact paths. Every selected command must hash exactly to a named
repository-defined `experiments` command. The attempt retains the revision and
optional release, environment definition and digest, checksummed inputs, exit
codes and bounded logs, checksummed artifacts (4 MiB each, 16 MiB total), actor,
and time. Secret-like evidence fails closed; failed and inconclusive attempts
remain inspectable and can be rerun in a fresh workspace.
Artifact collection rejects absolute/traversing paths, backslashes, every
symlink component, and non-regular files. It opens one descriptor, validates
that exact descriptor resolves beneath `/workspace`, and reads from the same
descriptor, preventing links or concurrent path replacement from exporting
image or temporary-container content.

Collaborative triage remains versioned issue state rather than private labels.
`PUT /repositories/{id}/issues/{issue-id}/triage` compare-and-swaps a
classification, priority, current participant assignee, exact suspected commit,
suspected current participant owners, and optional same-repository duplicate.
`POST .../links` attaches attributable typed code, dependency, release,
deployment, incident, proposal, pull-request, or issue references. Duplicate
identity, ownership, status, comments, links, and every triage revision remain
in the issue history.

`POST .../evidence-requests` directs a retained question to the reporter; only
that reporter can answer it with `PUT .../evidence-requests/{request-id}`.
Humans publish `hypothesis`, `finding`, or `uncertainty` records with `POST
.../findings`; every claim must cite retained reproduction-attempt or linked-
evidence IDs, can supersede an earlier claim without rewriting it, and can be
openly disputed through `POST .../findings/{finding-id}/challenges`.

Maintainers delegate selected evidence with `POST .../investigations`, naming
one retained reproduction attempt, a bounded subset of linked evidence, a
mandate, and a 5-minute through 24-hour expiry. The returned API credential has
only `issues:investigate`. Its nested investigation `GET` exposes only that
frozen packet, and nested `POST /findings` accepts citations only from the
selection. Agent findings carry the generated agent identity; the initiator,
credential binding, mandate, evidence selection, and history remain retained.

Issue repairs gain revision-exact proof through `POST
/repositories/{id}/issues/{issue-id}/repair-verifications`. The candidate must
be the current commit of the issue task's ordinary pull. The server freezes the
original reproduction environment and checksummed inputs, stages those inputs
in a fresh network-disabled checkout, reruns its retained commands, and runs
required checks defined by the trusted affected revision. `GET` projects
permitted logs, criteria, pass state, and staleness. Reporter confirmation or
rejection and owner-only justified overrides append through the nested
`/decisions` endpoint without replacing dissent. Pull movement invalidates the
projection and blocks decisions against stale evidence.
When the exact candidate already has a running, currently shared pull-linked
workspace, the proof includes `preview_allowed` and `preview_workspace_id`; its
existing authenticated workspace preview remains the policy enforcement point.
Input files are created through descriptor-relative no-follow traversal, so
candidate archive symlinks cannot redirect staging. The issue durably reserves
a deterministic verification identity before creating checks; split-outcome
retries reuse and link that reservation, and execute only its recorded run IDs.

## Governed project funds

`POST /repositories/{id}/funds` lets a current repository participant establish immutable fund
terms: named stewards, accepted funding sources, fixed-point currency or credit units, spending
limits, approval thresholds and approvers, eligible recipient classes, refund policy, and `public`
or `participants` ledger visibility. `GET /repositories/{id}/funds` and
`GET /repositories/{id}/funds/{fund_id}` project the declared rules, authority disclaimer,
append-only transfer evidence, and available, reserved, spent, refunded, disputed, and pending
balances subject to repository and ledger visibility.

Authenticated repository readers commit backing through `POST
/repositories/{id}/funds/{fund_id}/commitments` with an accepted source, external transfer
reference, positive minor-unit amount, and idempotency key. Accepted source keys come only from the
operator-controlled `$PROJECT_FUND_TRUSTED_SOURCES` JSON object; fund authors select registered
source names and cannot supply trust keys. Commitments are pending and contribute no available value. A named
steward who remains a repository participant reconciles one through
`POST .../commitments/{entry_id}/reconcile`, providing the current fund version and a `settled`,
`partial`, `failed`, or `revoked` result. Spendable outcomes require the source's Ed25519 signature
over the source, original reference, completed amount, status, verification time, and nonce. Only
the cryptographically verified completed amount becomes available. Source/reference pairs and
signed proof references and nonces are single-use across every fund in the store under one atomic
admission lock, with balance projection independently deduplicating proof identities;
duplicate, stale, failed, revoked, repeated, or invalid partial transfers cannot do so. Fund roles
and balances grant no repository or operational authority.

## Outcome funding

`POST /repositories/{id}/funded-outcomes` lets a current repository participant allocate a
governed fund toward an exact `issue`, `roadmap_outcome`, `proposal`,
`stewardship_opportunity`, `incident_follow_up`, or `security_repair`. The request contains
`fund_id` and complete `terms`: a source ID, exact source revision and visibility; title and
bounded scope; acceptance criteria and evidence requirements; positive minor-unit budget and
deadline; contributor eligibility; `first_accepted`, `proportional`, `maintainer_selection`, or
`milestone_claim` allocation; cancellation terms; dependencies, risks, conflicts; and optional
milestones. When present, milestone budgets must exactly equal the outcome budget and each
milestone independently declares acceptance and evidence.

Repository readers use `GET /repositories/{id}/funded-outcomes` and `GET
/repositories/{id}/funded-outcomes/{outcome_id}`. Participant and embargoed contracts require
current repository participation. Projections show active whole/milestone backing, aggregate all
active pledges across open outcomes sharing a fund, and derive
`insufficient_funds`, `unsettled_backing`, `overlapping_award`, `embargoed_work`,
`changed_scope`, and `withdrawn_backing` diagnostics instead of presenting an ambiguous promise.

Authenticated permitted readers pledge through `POST .../{outcome_id}/pledges` with
`expected_version`, a positive amount, optional `milestone_id`, idempotency key, and rationale.
The backer alone may `withdraw` or `reconfirm` through `POST
.../{outcome_id}/pledges/{pledge_id}`. A former public backer may withdraw after visibility is
restricted, but receives only the changed pledge identity/status and cannot reconfirm or read the
current contract. Current participants publish a successor through `POST
.../{outcome_id}/revisions`; it retains the prior revision and an attributable reason, and every
active pledge becomes `reconfirmation_required`. `POST .../{outcome_id}/cancel` retains the
cancellation reason. All mutations compare-and-swap the outcome version. Funding and backing do
not by themselves reserve project-fund value or grant task, Git, credential, review, acceptance, merge,
deployment, or security authority; cryptographically settled fund value remains a separately
derived custody fact.

Eligible contributors submit a delivery offer through `POST
.../{outcome_id}/delivery-proposals`. Each request names a `human`, organization `team`, or
`approved_agent`, and declares its approach, ordered milestones, minor-unit cost, dependencies,
availability, requested access, and attributed relevant work. A human submits for themself, a
current team member submits for the team, and a current operator submits for an organization-
approved agent; permission-bounded outcomes remain participant-only. The proposed recipient must
then explicitly accept through `POST .../delivery-proposals/{proposal_id}/accept` at the current
outcome version.

A named fund steward compares accepted offers and calls `POST
.../{outcome_id}/delivery-selections` with one or several complementary proposal IDs, an explicit
conflict disclosure, comparative rationale, and the current version. Recipient eligibility is
revalidated, selected costs must fit cryptographically settled available value, and one durable
`delivery_reservation` ledger entry moves that value from available to reserved. The selection
also creates connected milestone-shaped `planned` tasks attributed to each recipient. Selection
cannot bypass the fund's spending limits or eligible-approver rule; a rule requiring more than one
approval fails closed until a multi-approval reservation workflow is established. Selection,
reservation, acceptance, requested access, and planned tasks grant no repository,
secret, credential, task-execution, review, merge, environment, deployment, fund-withdrawal, or
recipient-acceptance authority; those scopes remain independently governed.

Selected contributors report execution through `POST
.../{outcome_id}/delivery-selections/{selection_id}/updates`. The compare-and-swap request names
one connected task and retains status, percent progress, summary, blockers, forecast, agent
minutes, and bounded exact references to existing tasks, sessions, workspaces, forks, pulls,
checks, previews, delivery teams, commits, releases, or deployments. Evidence stays attributed to
the authenticated human, team member, or approved-agent operator and does not alter authority in
the referenced system.

Contributors request evidence-backed costs through `POST .../{selection_id}/expenses`; they do
not spend automatically. A current repository-participant fund steward approves or rejects a
pending request at `POST .../{selection_id}/expenses/{expense_id}`. Approval atomically moves the
amount from the selection's reserved balance to spent and retains the decision and evidence.
`POST .../{selection_id}/controls` lets a current steward `pause`, `resume`, record
`access_revoked`, `increase_budget` within live fund availability and single-steward spending
rules, `replace_recipient` for unfinished tasks after live applicant validation, or
`cancel_remaining`. Cancellation releases only unspent reserved value. Projections derive total
progress, approved and pending expense, agent compute, last activity, forecast, and blockers.
Overrun, fourteen-day inactivity, failed handoff, revoked access, pause, or cancellation stops new
expense requests and approvals while preserving legitimate work and spend. Controls and
replacement never grant repository, task, credential, Git, review, merge, deployment, or fund
authority.
The inactivity clock begins at selection when no update exists. Pause stops expenses but not
progress or handoff reporting. `resume` does not assert that revoked access was restored; that
block remains until a replacement is admitted through live eligibility validation. Approved
expense publication is journaled before either fund or outcome publication, and the journal is
rolled forward under the fund mutation lock after interruption so their durable projections
reconverge.
A replacement must differ in principal kind or identity from the currently assigned recipient;
same-principal reassignment is rejected and cannot clear revoked-access protection.
Access revocation freezes later delivery-record mutations by every then-assigned principal.
Completed tasks retain attribution after a distinct replacement, but that retained identity is
history rather than continuing update authority.

Designated reviewers decide a task through `POST
.../{selection_id}/tasks/{task_id}/reviews` with the current outcome version, an `accepted`,
`rejected`, `correction_requested`, `partial_award`, or `disputed` decision, rationale, dissent,
and measured outcomes. An award requires the latest task update to be completed with linked work
and every accepted measure met. The paid review ID is its settlement receipt: the matching
`milestone_award` ledger entry ends in that ID and attributes recipient credit to the reviewer,
frozen update, authorship, resources, and measures. It grants no authority over those resources.

Eligible recipients, reviewers, and stewards use `POST .../{task_id}/recoveries` for appeal,
withdrawal, timeout, payment failure/retry, or refund. These append evidence and move only the
task's remaining reservation or prior award. Completed, accepted, partially accepted,
payment-failed, withdrawn, timed-out, and refunded tasks retain their recipient when a steward
uses `replace_recipient`; only unfinished work moves to the distinct, live-validated replacement.
Payment retry therefore credits the originally accepted recipient.
Delivery updates against any terminal task conflict instead of replacing its status, so they cannot
disable a payment retry or make retained ownership eligible for reassignment.

## Accessibility commitments

Current repository participants publish a complete contract with `POST
/repositories/{id}/accessibility-commitments` and a successor with `POST
/repositories/{id}/accessibility-commitments/{commitment-id}/revisions`. The
successor requires `expected_version`; stale writes return `409
accessibility_commitment_conflict`. Repository readers use collection and detail
`GET` endpoints.

Each immutable revision targets a repository, documented journey, component, or
release and defines standards and criteria, supported assistive technologies,
target audiences and access needs, supported or explicitly unsupported
environments, required test scenarios, a severity/response policy, accountable
owners, testable requirements, expiring permitted exceptions, and typed links to
roadmap outcomes, documentation, previews, and release policy. Reads derive
attributed `missing_coverage`, `conflicting_requirement`,
`unsupported_environment`, `expiring_exception`, and `expired_exception`
diagnostics. Records default beneath `$ACCESSIBILITY_COMMITMENT_STORAGE_ROOT`
(`accessibility-commitments`) in repository-scoped directories so corruption in
one repository cannot hide or deny another repository's commitments; corruption
inside the requested repository fails the collection read explicitly.

## Accessibility barrier reports

Any authenticated user permitted to read a repository can submit a lived barrier with `POST
/repositories/{id}/accessibility-reports`. A report targets a `release`, `page`,
`documentation_journey`, or `preview` by resource ID and exact revision, and includes functional
access needs, an expected observable outcome, ordered interaction steps, the reporter's browser,
device, and assistive-technology environment, explicit consent, and up to twelve artifacts.
Artifacts are typed as a screenshot, recording, accessibility tree, speech output, or input trace;
each must be explicitly redacted and carry only a bounded description and safe content reference.

Repository readers use the collection and detail `GET` endpoints. Reporter identity is hidden from
other readers unless the reporter consents to share it with maintainers. Device, OS, version, and
input-mode detail is independently suppressed outside the reporter's own projection unless the
reporter consents; the functional browser and assistive-technology names remain so the scenario is
actionable without medical context.

Current repository participants append reproduction evidence with `POST
/repositories/{id}/accessibility-reports/{report-id}/attempts`. Every attempt is server-bound to the
report's frozen revision, declares a bounded `workspace` or `preview`, browser, device, and assistive
technology, and classifies the observation as `reproducible`, `intermittent`,
`environment_specific`, or `unconfirmed`. Attempts and their redacted artifacts are append-only and
grant no repository, credential, preview, or deployment authority. Records default beneath
`$ACCESSIBILITY_REPORT_STORAGE_ROOT` (`accessibility-reports`) in repository-scoped directories;
cross-process mutations use a shared lock and atomically published files so concurrent attempts are
not lost and corruption in another repository cannot deny this collection.

## Accessibility assessments

Current repository participants publish repository-defined exact-revision check evidence with
`POST /repositories/{id}/accessibility-assessments`. Checks cover `semantics`, `keyboard`, `focus`,
`contrast`, `motion`, `captions`, or a declared `journey`, report `passed`, `failed`, or
`unevaluated`, and retain audiences, source locations, a bounded summary, and whether human
evaluation remains required. An optional `pull_request_id` projects the same assessment on the
matching pull. Without a pull identity, the revision must resolve to a commit reachable from a
visible repository branch; hidden `vivarium-security/*` branches are never assessment sources.
Pull-bound evidence must match the pull's authoritative source revision.
Readers filter collection `GET` requests by `revision` and/or `pull_request_id`.

Participants and repository-bound read-only agent credentials append findings through `POST
.../{assessment-id}/findings`. Each finding is revision-bound through one or more permitted
`preview` or `reproduction` citations, and retains severity, audiences, source/journey locations,
uncertainty, duplicate identity, and human-evaluation need. Only current human participants may
record `accepted` or `false_positive` decisions through `POST .../findings/{finding-id}/decision`.
Preview citations must resolve to an exact-revision preview whose parent pull still has that source
revision, and one of its retained
artifacts. Reproduction citations must resolve to an exact-revision retained attempt and one of its
redacted artifacts; invented or cross-revision resource/evidence pairs fail before persistence.
Pull-bound assessment creation and preview-cited finding publication hold the pull mutation boundary
through final evidence persistence. Concurrent synchronization therefore returns `409` without
publishing stale assessment or citation records; all preview citations in one finding must share the
assessment's guarded pull (or one shared pull for a standalone assessment).
`POST .../{assessment-id}/invalidate` names changed source locations or journey IDs and invalidates
only intersecting checks/findings, clearing only their affected acceptance. Records default beneath
`$ACCESSIBILITY_ASSESSMENT_STORAGE_ROOT` (`accessibility-assessments`).

An accepted, current finding enters governed implementation through `POST
.../{assessment-id}/findings/{finding-id}/repair`. A current human repository participant selects an
exact accessibility commitment revision, supplies bounded acceptance criteria and component
guidance, and assigns the resulting ordinary task to an existing human participant or an agent.
The task freezes the assessment revision, accepted finding, permitted redacted citation references,
commitment context, and collaborator-authored guidance; it does not copy reporter identity or
unconsented device detail and grants no new authority. Retries converge on the finding-specific
proposal/task identity. `GET .../repair` reports the live task, linked ordinary pull request, and
only previews matching that pull's current exact source revision back to the original finding.
The finding-side repair reservation is persisted before proposal creation and exposes a stable
`recovery_id` with `state: pending`; an exact retry completes the link even if source invalidation
arrived after reservation, while a mismatched retry conflicts. Responses depending on a proposal
whose atomic rename committed but directory durability is uncertain include
`Vivarium-Durability: uncertain`, including pending-recovery responses.

## Accessibility delivery readiness

Repository owners publish immutable accessibility delivery gates with
`POST /repositories/{id}/accessibility-delivery-policies`; selectors can narrow a gate to a target
branch, changed path (with an explicit trailing `*` wildcard), declared journey, or risk class. A
gate names exact automated check runs, assessed scenario coverage, and a minimum number of named
`reviewer`, `participant`, or `accessibility_reviewer` acknowledgements. Collection `GET` lists the
retained policies.

An evaluator must first receive ordinary bounded guest access to a revision-exact preview. A
participant then creates the matching policy invitation at
`POST /repositories/{id}/accessibility-evaluations/invitations`; the user, preview revision, and
expiry must match. The invited user alone confirms or rejects at `POST
.../invitations/{invitation_id}/outcome`. Outcomes are final and bound to the exact revision plus
pull-or-release delivery context; a rejection remains attributed dissent and never represents other
access needs.

`GET /repositories/{id}/pulls/{pull_id}/accessibility-readiness` and
`GET /repositories/{id}/releases/{release_id}/accessibility-readiness` report applicable evidence
as passed, failed, pending, unevaluated, missing, or stale. Ordinary pull merge readiness includes
the same result and blocks merge/queue admission. Owners may retain a bounded exception with
rationale, expiry, and mandatory follow-up work at
`POST /repositories/{id}/accessibility-delivery-overrides`; exceptions do not remove prior dissent
or evidence. Records default beneath `$ACCESSIBILITY_DELIVERY_STORAGE_ROOT`
(`accessibility-delivery`).
Release creation freezes the repository default branch and the exact changed paths from the
previous release (or the root candidate), so path-scoped gates cannot disappear during later
readiness evaluation. Missing pull or release selection context fails closed. Any current invited
rejection blocks its role even when the numerical confirmation minimum is otherwise met; only the
explicit owner exception can permit delivery while preserving that dissent.

Publishing an accessibility-repair task through the ordinary task contribution endpoint additionally
requires structured design changes, code changes, interaction tradeoffs, and content tradeoffs. The
server appends those sections to the pull body. Contributors then attach previews through the normal
pull preview workflow; repository write, agent-session, review, check, queue, and merge policy remain
unchanged.

## Performance goals

Current repository participants create a complete contract with `POST
/repositories/{id}/performance-goals` and publish a successor with `POST
/repositories/{id}/performance-goals/{goal-id}/revisions`. Successors require
`expected_version`; a stale write returns `409 performance_goal_conflict`.
Repository readers use the collection and detail `GET` endpoints.

The `revision` object names a `repository`, `release`, `user_journey`, `api`,
`command`, or `service` subject and includes workloads, metrics with target
ranges and optional measured baselines, correctness constraints, supported
environments, owner IDs, budgets, baseline age policy, rationale, and typed
links to issues, incidents, previews, releases, or decisions. Responses retain
every immutable revision and derive attributed diagnostics named
`missing_measurement`, `incomparable_environment`, `stale_baseline`,
`target_gap`, and `conflicting_target`. Storage defaults to
`$PERFORMANCE_GOAL_STORAGE_ROOT` (`performance-goals`).

## Performance evidence

Repository collaborators publish a bounded completed trial with `POST
/repositories/{id}/performance-trials`; readers use the collection/detail endpoints and compare
two retained trials with `GET
/repositories/{id}/performance-trials/{trial-id}/compare/{baseline-id}`. Every trial freezes an
exact Git commit; a `release` source must name a release attesting that commit.

Trials retain a fixed non-sensitive production-workload marker plus its sanitization recipe (or
repository benchmark inputs), environment, warmup/sample method, raw timings with
server-derived variance, resource profiles, content-addressed trace/artifact metadata, logs, and
cost. Production captures require declared sanitization; their raw input and log entries are
replaced with non-sensitive markers before persistence and again on reads, while credential-like
benchmark logs are rejected.
Comparisons are numeric only when workload, every environment field, warmup/sample count and
method, metric, and unit match.
Records default beneath `$PERFORMANCE_EVIDENCE_STORAGE_ROOT` (`performance-evidence`).

## Performance merge and release policy

Repository owners create append-only merge gates with `POST
/repositories/{id}/performance-merge-policies`; repository readers list them with `GET` on the same
collection. Policies name a branch, optional path and risk selectors, goals, allowed regression,
minimum confidence, and correctness requirements. Ordinary merge readiness returns
`performance_missing`, `performance_failed`, or `performance_uncertain` blockers against the exact
current revision. Post-integration `POST /repositories/{id}/performance-release-observations` binds
the candidate evaluation to its exact release, deployment, commit, goal, and observed trial, then
projects passed, regressed, or uncertain state and recovery recommendations. These records grant no
merge, agent, deployment, or environment authority.

## Performance diagnosis

Repository readers list and inspect diagnoses at `GET
/repositories/{id}/performance-investigations`; collaborators create one with `POST` by selecting
retained trials, revision-aware code or runtime references, and owners to invite. Findings posted
beneath `.../{investigation-id}/findings` require selected citations and retain confidence,
optional bounded flame stacks, challenges, and confirmations. Newer same-context evidence marks a
finding stale when revision, workload, or environment changes.

`POST .../{investigation-id}/agent-access` issues a 5-minute to 24-hour
`performance:investigate` credential bound to that investigation. Its read and finding endpoints
revalidate the issuer's participation and expose only selected, sanitized trials and references.

## Performance optimization evaluation

Collaborators attach candidate evidence to an ordinary open pull with `POST
/repositories/{id}/pulls/{pull-id}/performance-evaluations`; repository readers inspect retained
revisions with `GET` on the same collection. The write binds a supported investigation and goal,
one investigation-selected baseline trial, and a candidate trial at the pull's exact synchronized
source commit. It retains affected scenarios, reproduction commands, attributable correctness
checks, and residual risks. The server derives compatible metric deltas, statistical confidence,
CPU/memory/I/O and cost changes, and aggregate correctness. Source movement preserves earlier
evidence as explicitly stale. These records are review evidence only and grant no operational
authority.
## Product opportunities

`GET /repositories/{id}/product-opportunities` and
`GET /repositories/{id}/product-opportunities/{opportunity_id}` return permitted opportunity
syntheses with all retained versions, challenges, corrections, and citation freshness. Source kinds
are `feedback`, `issue`, `preview_finding`, `support_signal`, `usage_evidence`, and
`experiment_outcome`; relationships are `supports`, `contradicts`, `minority_need`, and `duplicate`.

`POST /repositories/{id}/product-opportunities` accepts a complete synthesis revision. Repository
participants and repository-scoped read-only agent credentials may create one, but every platform
source must resolve to the repository and exact submitted revision. Participant-only mutations are
`POST .../{opportunity_id}/revisions` and `POST .../{opportunity_id}/corrections`; all authorized
readers may `POST .../{opportunity_id}/challenges`. A feedback reporter can call
`POST .../{opportunity_id}/detach-feedback/{feedback_id}`. Mutations require `expected_version` and
return `409 product_opportunity_changed` after concurrent changes.

Organization agent grants mint that synthesis credential through
`POST /organizations/{id}/access-grants/{grant_id}/credentials` with `purpose: "api_read"`;
the result is API-scoped to `repositories:read` and the named repository. Omitting `purpose`
retains the existing Git credential behavior derived from the grant role.

## Product roadmaps

`GET /repositories/{id}/roadmap` returns the versioned roadmap, non-binding scenarios, and
attributed tradeoff comments. `PUT /repositories/{id}/roadmap` is limited to human repository
participants and accepts `expected_version` plus a complete `revision`. Every opportunity decision
must cite an existing exact `opportunity_version`, choose `accepted`, `deferred`, or `rejected`, and
retain its reason, goal fit, and capacity assessment. Accepted decisions may be sequenced as roadmap
items with stable IDs, owners, target horizons, success measures, dependencies, and status.

`POST .../roadmap/scenarios` lets authorized humans and repository agents append a non-binding
alternative with a rationale; it never publishes a roadmap revision. `POST .../roadmap/comments`
retains an authorized reader's tradeoff discussion. All mutations compare `expected_version` and
return `409 roadmap_changed` after concurrent activity. Every `PUT` after initial publication must
include `change_reason`; `replan_triggers` retain why scope, commitments, ownership, sequence, or
targets changed instead of silently replacing the prior promise.

## Roadmap outcome validation

`POST /repositories/{id}/outcome-validations` freezes an accepted roadmap item and exact cited
product-opportunity revision. Human repository participants select `technical_decision`, `prototype`,
`documentation_concept`, or `product_experiment`, name an exact concept/research revision, and provide
success and guardrail measures whose `source_ids` belong to that opportunity revision.

Collaborators invite named users with `POST .../{validation_id}/invitations` to `preview` or `research`
at the exact revision and an explicit expiry. Invitees can read only a validation naming them; this
grants no repository access. They accept, decline, or withdraw prior acceptance at `POST .../consent`, and only an accepted, live,
revision-matched invitation may append a finding at `POST .../findings`. Findings retain attributable
accessibility needs, dissent, acceptance, and `valid`, `insufficient`, or `invalid` evidence quality.
Collaborators append a `validated`, `revise`, `defer`, or `reject` conclusion at `POST .../conclusions`;
conclusions never rewrite the roadmap. Mutations compare `expected_version` and return
`409 validation_changed` on concurrent activity.

`POST /repositories/{id}/roadmap/implementations` converts one exact accepted roadmap item into an
ordinary proposal with ordered human- or agent-owned tasks. Every task names the success-measure
indexes it advances and the complete plan must cover every measure; the server freezes the current
default-branch revision and retains the opportunity need and measures on the proposal and tasks.
`POST /repositories/{id}/roadmap/implementations/{proposal_id}/outcomes` accepts only retained linked
review, check, integration, release, or deployment evidence. Product experiments report through
their release/deployment-linked evidence chain. `delivery` never
means achieved; `measure_met` must cover every frozen success measure. `measure_failed`,
`assumption_changed`, `need_unresolved`, `policy_conflict`, and `decision_revisit` move the projection
to `revisit_required` with attributable evidence.

`POST .../roadmap/learning-updates` publishes a `decision`, `preview`, `delivery`, `rejection`, or
`measured_outcome` update with an exact opportunity, cited feedback IDs, audience-safe summary and
rationale, and an inspectable resource link. Non-participant roadmap reads include only updates citing
feedback reported by that viewer and omit the feedback audience list. The reporter answers through
`POST .../roadmap/learning-responses` with `improved`, `not_improved`, or `unsure`, optional follow-up
evidence, and `leave_conversation`; leaving prevents later updates citing that feedback.
`POST .../roadmap/learning-reviews` lets a human repository participant retain promised and observed
outcomes, lessons, dissent, resulting work, and a `continue`, `revise_roadmap`, `fulfilled`, or
`unsupported` disposition. All three mutations require `expected_version` and are append-only.

## Service objectives

`POST /repositories/{id}/service-objectives` publishes a complete reliability contract. A revision
defines repository, release, or environment scopes; service indicators and supported calculations;
objectives with measurement windows and user journeys; dependencies; error budgets; ordered
severity responses and owners; exception policy and time-bounded exceptions; and version-pinned
links to product, performance, accessibility, privacy, or release commitments. Current repository
participants may publish; a named owner must also remain a current participant.

`POST /repositories/{id}/service-objectives/{objective_id}/revisions` publishes a complete
successor using `expected_version`; stale writers receive `409 service_objective_conflict`.
Repository readers use collection and detail `GET` endpoints. Responses preserve every immutable
revision and derive `missing_signal`, `unsupported_calculation`, `conflicting_target`,
`missing_ownership`, `expiring_exception`, and `expired_exception` diagnostics with attribution.
The contract grants no operational authority. Storage defaults beneath
`$SERVICE_OBJECTIVE_STORAGE_ROOT` (`service-objectives`).

Signal mappings and append-only operational windows are published beneath a contract through
`.../signal-mappings`, mapping `.../revisions`, and `.../observations`. Each binds exact contract,
objective, instrumentation, window, and delivered-software revisions; projections derive attainment,
target status, budget consumption, uncertainty, gaps, and comparability.

`POST /repositories/{id}/service-objectives/{objective_id}/investigations` opens an authenticated,
revision-bound investigation. `contract_version`, `objective_id`, and one exact `objective`,
`pull_request`, `deployment`, or `budget_consumption` trigger must resolve through retained contract
or observation provenance. Baseline and affected observations must belong to that objective revision;
journeys must belong to its contract. Evidence declares a bounded kind, stable ID, exact revision,
label, and `public` or `participants` visibility.

Participants and repository-bound read-only agents append cited `hypothesis`, `comparison`,
`uncertainty`, or `conclusion` entries through `.../investigations/{investigation_id}/findings`.
Each states confidence and uncertainty and cites the closed evidence/observation set. `.../responses`
retains `confirm` or `dispute`; `.../input-requests` addresses only a frozen objective/dependency
owner, who answers through `.../input-responses`. `.../outcomes` retains an `issue`, `incident`,
`decision`, or `planned_improvement` reference without creating it or granting authority. Mutations
require `expected_version`; conflicts return `409 reliability_investigation_conflict`. Reads expose
stale evidence, dependencies without owners, and inconclusive state caused by absent/disputed
conclusions or stale evidence. Anonymous contract reads omit investigations.
Pull, deployment, release, and commit evidence is resolved against the target repository and exact
current resource revision. Metric/log/trace/health/support evidence must cite an exact retained
observation from the frozen objective; dependency evidence must cite the frozen contract dependency.
Missing, cross-repository, hidden, or revision-mismatched provenance returns
`422 invalid_reliability_provenance`, and persistence fails closed if resolution is unavailable.
Read-only agents may open an investigation and add cited findings only. Confirm/dispute responses,
owner-input requests and answers, and concluding outcomes require a human repository participant.
Commit evidence must be reachable from a non-`vivarium-security/*` branch. Pull, deployment, and
budget triggers additionally require retained provenance on the investigation's exact contract
version and objective; a canonical resource that appears only on another objective cannot authorize it.

`POST .../delivery-policies` lets the repository owner bind an exact contract revision and objectives
to branch, service, environment, journey, and risk selectors. A policy sets budget and predicted-impact
limits, dependency/current-evidence requirements, required objective owners and acknowledgement count,
and a `warn`, `slow`, `block`, `pause`, or `rollback` response for missing evidence, regression,
exhaustion, and dependency failure. `POST .../reliability-impacts` records a pull request, integration
queue, release, or deployment's exact revision with predicted and observed objective impact.
Named owners use `.../acknowledgements` or a maximum-30-day `.../exceptions`; neither conveys delivery
authority. `GET /repositories/{id}/reliability-readiness/{kind}/{resource_id}` accepts `revision`,
`branch`, `service`, `environment`, repeated `journey`, and repeated `risk` query parameters and returns
blockers, required and acknowledged owners, an active exception, configured effect, next actions, and
an explicit authority note. Pull merge readiness includes this projection and blocks merge/queue
admission for block, pause, or rollback effects.
When current evidence is required, every objective impact must cite a retained observation for the
same contract version and objective, within its declared measurement-window age, whose software
provenance names the exact delivery resource and revision. The server replaces any submitted
`observed_budget_consumed_percent` with the observation-derived value; unresolved, stale,
cross-objective, or revision-mismatched citations are rejected.

`POST .../improvements` converts exactly one investigation finding or reliability impact into
ordinary proposal work. The request freezes `contract_version`, `objective_id`, baseline and affected
observation IDs, affected revisions, dependency context, cited evidence, acceptance criteria, and one
to twenty ordered tasks (`title`, `assignee_type`, `assignee_id`, `depends_on_previous`). The default
branch revision and every human assignee remain valid through proposal publication. The response
retains the proposal and task IDs; normal assignment, checks, review, merge, and release boundaries
remain authoritative.
The impact source must contain a server-derived observation at or beyond its delivery policy's
depletion threshold, and that exact observation must be frozen in the improvement's baseline set;
predicted-only or differently baselined impacts cannot authorize work. A stable `pending` improvement is
reserved before proposal persistence, and an exact retry reuses the reservation and proposal origin
to finish the link after cross-store failure. The reservation retains the server-selected depletion
observation and original default-branch base; restoration must use that observation, and retries reuse
that base even if the branch has advanced.

`POST .../improvements/{improvement_id}/verifications` is repository-owner-only and accepts a
`release` or `deployment`, exact resource/revision, retained baseline and current observation, one of
`restore_budget`, `contain`, `rollback`, or `revisit`, and rationale. The current observation must
postdate the baseline and cite that exact rollout. `restore_budget` is accepted only when the selected
baseline belongs to the improvement's frozen baseline set and the current window meets its objective
and consumes less error budget; failed measures require one of the other
decisions. Responses retain before/after derived budget values and never rewrite the prior observation.
Reliability contract, finding, and impact references are compact opaque identifiers; their handoff to
ordinary proposal reasoning preserves those exact values and does not require proposal-ID formatting.
The connected browser journey exercises the public endpoints through noisy-signal correction, missing
dependency evidence, rejected exception, revision-exact human-agent investigation, containment, reviewed
agent repair, failed verification, corrective review, staged deployment, and restored attainment.

## Recovery protection plans

`POST /repositories/{id}/protection-plans` requires repository write access and publishes a complete
plan against the current `commitment_id` and `commitment_version`. Each resource names a commitment
`target_id` and either an exact 40-character repository `revision` or an existing governed
`environment_id`. The target must permit the plan jurisdiction. Plans also require `snapshot` or
`replica` mode, an approved `vault://` destination, positive retention and freshness bounds, current
repository-participant accessor IDs, and validation checks. Successors use
`POST .../protection-plans/{plan_id}/revisions` with `expected_version`; the commitment identity is
immutable and a changed commitment must be acknowledged by a current-version plan.

`POST .../protection-plans/{plan_id}/captures` accepts only the current plan version. The server
resolves and integrity-checks every source, walks the full Git tree or reads the credential-free
environment projection, derives per-entry checksums and a complete manifest, and AES-GCM encrypts the
manifest and payload at rest. Any missing or corrupt input rejects the whole capture; partial captures
are never published. The capture also freezes the plan version, source-resource identities, and
freshness interval used to evaluate it; successor plan policy applies only to later captures.

`GET /repositories/{id}/protection-plans` and `GET .../{plan_id}` use ordinary repository read
visibility. They return coverage counts, source and manifest checksums, encrypted byte counts,
validation checks and status, location, retention, cost units, freshness inputs, and actor history.
They never return manifest paths, source content, ciphertext, nonces, or credentials. Reads verify
decryption and the manifest checksum and mark corruption, key loss, retention expiry, or a deleted
source as `recoverable: false` with an explicit failure. These records grant no content, restore,
repository, environment, or deployment authority.

## Recovery exercises

`POST /repositories/{id}/recovery-exercises` requires repository write access and membership in the
selected protection plan's `accessor_ids`. It accepts an exercise name, failure `scenario`, exact
`plan_id` and recoverable `capture_id`, plus at most 32 dependency-ordered steps. Step kinds are
`restore`, `integrity`, `journey`, and `manual`; commands are deliberately bounded to
`restore:protected-manifest`, `verify:manifest`, `verify:dependencies`, `journey:{name}` (where the
name is declared in the protection plan's validation checks), and `manual:confirm`. Arbitrary shell
commands are rejected. The server freezes captured plan, commitment, and source versions, decrypts
only into an ephemeral no-network/no-production-credentials environment, runs steps in declared
order, and never writes restored content to repository or environment authorities.
Repository restore verifies each blob checksum before writing it with restrictive permissions;
environment restore writes only the deployment store's credential-free public projection. The
registered `smoke` journey requires exactly one `.vivarium/recovery-smoke.json` v1 contract in the
restored repository. It names a digest-pinned locally cached container `image`, relative executable
script/ELF `entrypoint`, bounded `arguments` and `timeout_seconds`, zero `expected_exit_code`, and exact
`expected_stdout_sha256`. The runner executes restored application code with no network, a read-only
source mount, dropped capabilities, no new privileges, and CPU/memory/process limits. README/static-
only captures, missing entrypoints, and mismatched outcomes fail. A declared journey without a
registered bounded implementation fails, and the filesystem is deleted after the run completes.

`GET /repositories/{id}/recovery-exercises` uses ordinary repository visibility and returns the
scenario, isolated environment identity, per-step timing, bounded command, redacted log/artifact,
gap/manual/objective results, actor, and overall duration/status. Protected paths and payloads are
never returned. `current` and `stale_reasons` are derived on every read; a successor protection plan
or commitment, unavailable capture, or changed protected source prevents retained evidence from
counting as current. Exercise records default beneath `$RECOVERY_EXERCISE_STORAGE_ROOT`
(`recovery-exercises`) and grant no restore, repository, deployment, environment, or secret access.

### Recovery investigations and governed improvements

`POST /repositories/{id}/recovery-exercises/{exercise_id}/investigations` is available to authenticated
repository readers, including repository-bound read-only agent credentials, for an exercise with
gaps. It accepts a title and bounded evidence references of type `exercise_result`, `code`, `release`,
`protection_plan`, `configuration`, `recovery_commitment`, `dependency`, or `ownership`. The server
resolves each reference against the exercise's repository, exact revision, retained plan/commitment,
or result before persistence. `POST .../investigations/{investigation_id}/findings` compare-and-swaps
the investigation version and requires a statement, explicit uncertainty and confidence, and citations
to that investigation's permitted evidence. Agent credentials can investigate and publish findings;
these endpoints grant no restore, Git-write, policy, review, merge, release, deployment, or production
authority.

`POST /repositories/{id}/recovery-exercises/{exercise_id}/improvements` is repository-owner-only. It
freezes one cited finding and the current default-branch commit into an ordinary proposal with ordered
human- or agent-owned tasks and acceptance criteria. Tasks continue through the existing assignment,
session/workspace, pull, checks, review, integration, release, and policy-approval boundaries. The
exercise retains the reciprocal proposal/task link and stays `work_open` after publication.

`POST .../improvements/{improvement_id}/verifications` is also owner-only and accepts a
`follow_up_exercise_id`. Verification requires a later successful and currently valid isolated
exercise, plus a changed protection-plan version or protected source revision. It links, rather than
rewrites, the original failure and marks the improvement `verified`. The follow-up must retain the
same scenario, plan, commitment, ordered step contract, and pass every originally failed or cited
result; stale, unrelated, weakened, or unchanged rehearsal evidence fails closed. Code citations
likewise require an exact commit reachable from a visible non-security branch. Before accepting the
follow-up, the API re-resolves the exact proposal origin and ordered task IDs, matches the frozen
acceptance criteria and implementation base, requires every task to be completed through a currently
merged contribution at the task's exact current context revision, and proves each implementation
commit descends from that base. A missing, replaced, obsolete, unfinished, closed-without-merge, or
unrelated work record cannot become `verified`.

## Quality plans

`POST /repositories/{id}/quality-plans` publishes a complete versioned quality plan for an
authorized repository participant. `POST
/repositories/{id}/quality-plans/{plan_id}/revisions` publishes a compare-and-swap successor using
`expected_version`. `GET /repositories/{id}/quality-plans` and `GET
/repositories/{id}/quality-plans/{plan_id}` follow ordinary repository read visibility.

Each revision supplies one or more repository, release, journey, or interface scopes; supported
environments; source-linked behavioral requirements; automated or manual evidence references; and
optional bounded exceptions. Requirements include their risk, expected behavior, test levels,
representative data, coverage goal, owners, release judges, schedule, threshold, and observable
verification method. Source kinds are closed to `issue`, `decision`, `design`, `accessibility`,
`privacy`, `performance`, and `reliability`. Evidence status is one of `passing`, `failing`,
`missing`, `stale`, or `unknown` and is descriptive rather than execution authority.

Mutation validates every plan owner, requirement owner/judge, and exception grantor as a current
repository participant. Responses retain attributable diagnostics for missing ownership or evidence,
explicit conflicts, untestable claims, expired exceptions, and exceptions expiring within seven
days. Storage defaults to `$QUALITY_PLAN_STORAGE_ROOT` (`quality-plans`).

## Reusable test scenarios

`POST /repositories/{id}/test-scenarios` publishes an immutable executable behavior specification.
`GET /repositories/{id}/test-scenarios` and `GET
/repositories/{id}/test-scenarios/{scenario_id}` follow repository read visibility. Publication is
participant-only and accepts exact-revision rationale; typed parameters; explicit preconditions,
actions, assertions, environments, and cases; safe fixtures; and an ordinary Git implementation
proposal with author type, branch, commit, test paths, framework, rerun command, assumptions, and
provenance.

Source kinds are `issue`, `reproduction`, `design_specification`, `api_contract`, `documentation`,
or `user_journey`. Every source revision must be a real repository commit, and each supplied source
path must be a text blob in that commit. An optional quality-plan binding names real requirements in
an exact retained plan version. The implementation branch must currently resolve to the declared
commit; all test and fixture paths resolve there, and each fixture SHA-256 must match its blob.
Optional pull and workspace IDs must bind that same repository and commit.

Every rationale ID and revision pair is resolved before persistence. Issue citations must match an
exact commit retained by triage, a reproduction attempt, repair implementation/verification, or
delivery. Design specifications must match their implementation base. Replay scenarios, API contracts,
and documentation must expose the same exact commit. A user journey must be declared by a retained
repository quality-plan journey scope whose `source_revision` equals the citation revision. Quality
plan scopes may omit `source_revision` for compatibility, but such a journey cannot be reused as an
exact-revision scenario source.

Fixtures may only be `synthetic`, `generated`, or `template` assets classified as `synthetic`,
`anonymized`, or `public`; assumptions and provenance are mandatory. The API stores only their path,
digest, generator, and metadata, scans both the resolved blob and scenario metadata for secret-shaped
content, and never imports production
personal data or inaccessible evidence. Storage defaults to `$TEST_SCENARIO_STORAGE_ROOT`
(`test-scenarios`). Records grant no execution, Git, pull, workspace, environment, or data authority.

## Exploratory sessions

`POST /repositories/{id}/exploratory-sessions` opens a participant-controlled session whose source is
`pull_preview`, `release_candidate`, `issue`, or `quality_plan`. The source resource must exist and its
declared 40-character revision must resolve in repository Git; pull and release sources additionally
must equal their current candidate commit. The request supplies an explicit participant audience,
expiry of at most 24 hours, non-negative cost and agent-action ceilings, allowed actions, test data
limited to `synthetic`, `anonymized`, or `public`, and one or more risk charters. Charters assign a
current human participant or named approved agent and may only narrow the session actions.

`GET /repositories/{id}/exploratory-sessions` and `GET .../{session_id}` return only sessions whose
explicit audience contains the authenticated human reader. Reads derive `stale` when a pull moves or a
source becomes unavailable while preserving the original exact-revision record.

`POST .../{session_id}/events` appends against `expected_version`. Event kinds are `observation`,
`guide`, `pause`, `resume`, `reproduce`, `classify`, `discard`, and `close`. They retain route, sanitized
inputs, exact command, coverage, uncertainty, finding and reproduced-event links, classification,
attribution, and optional SHA-256-addressed screenshot, trace, recording, log, or command-output
metadata. Paused sessions accept only guidance, resume, or close; closed and expired sessions reject
new events. Agent credentials must match the exact charter assignee and share the session-wide action
budget. Agent lifecycle, guidance, reproduction, classification, and discard events additionally
require those exact actions in the charter; observation authority alone cannot control the session or
decide a finding, and classification fields are rejected on every non-decision event kind.
Human action events likewise require the authenticated participant to match the named human charter
and its narrowed action set; explicit audience membership alone permits only unchartered guidance.
Reproduction event IDs and classified or discarded finding IDs must resolve from a
prior observation or reproduction in the same session. Credential-shaped content is rejected. Storage defaults to
`$EXPLORATORY_SESSION_STORAGE_ROOT` (`exploratory-sessions`) and conveys no operational authority.

`POST .../{session_id}/findings/{finding_id}/repair` is human-collaborator controlled and accepts only a
current `bug` decision with a same-session reproduction. It compare-and-swap reserves the exact candidate,
selected observation/reproduction event IDs, minimized reproduction, bounded acceptance criteria,
human or agent assignee, and optional exact quality-plan version/requirements. It idempotently creates a
repository-visible issue and ordinary assigned proposal task; exact retries converge on the reserved issue,
proposal, and task identities. Candidate movement or changed handoff data is rejected. The task verification
plan requires the regression to fail at the affected revision, pass at the repair revision, and become a
maintainable reusable scenario. `flaky`, `duplicate`, `environment_specific`, and `not_reproducible`
classifications remain durable resolution paths but are not eligible for confirmed-bug repair.
While the retry-safe repair reservation is pending, further classify/discard events for that finding
are rejected. The reservation is therefore the publication boundary: deterministic issue/task creation
can be retried and finalized without concurrent supersession orphaning actionable work. Once linked,
later decisions may append normally and do not rewrite the retained repair history.

After the repair pull publishes its reusable scenario, `POST .../{session_id}/findings/{finding_id}/coverage`
links it back to the finding. The scenario must cite the linked issue, use the repair task's exact pull and
candidate commit, and match the handoff's quality-plan version and requirements. The resulting repair record
retains the scenario, pull, and regression commit alongside the issue/proposal/task links; reviews and check
evidence remain authoritative on that pull. Neither endpoint grants Git, workspace, review, merge, check,
agent-launch, or execution authority.

## Locale plans

`POST /repositories/{id}/locale-plans` publishes a complete localization support contract. The
revision includes a `repository`, `product`, `documentation_collection`, or `release` subject;
locales and regional fallbacks; terminology and formatting requirements; covered journeys;
translatable resources with exact 40-character source commit IDs; owners and reviewers; and release
thresholds. Each source commit must resolve in the target repository. Participants publish a
successor with `POST /repositories/{id}/locale-plans/{plan_id}/revisions` and the current
`expected_version`; stale writers receive `409 locale_plan_conflict`.

`GET /repositories/{id}/locale-plans` and `GET /repositories/{id}/locale-plans/{plan_id}` are
repository-readable and return the full immutable revision history plus derived `diagnostics` for
`missing_ownership`, `missing_reviewers`, `unsupported_format`, `conflicting_terminology`, and
`stale_coverage` against the current default branch. A locale plan is an inspectable support promise,
not release approval or operational authority.

### Pull localization review

`GET /repositories/{id}/pulls/{pull_id}/localization` projects the current pull source revision,
retained extraction snapshots and translation history, contextual units, and per-locale added,
changed, removed, reused, and untranslated counts. Repository visibility governs reads.

`POST /repositories/{id}/pulls/{pull_id}/localization/extractions` requires repository write access
and a `source_revision` equal to the current pull source commit. Its `map` names a stable ID, version,
include paths, and formats; `locales` declares the reviewed locales; every unit supplies a stable key,
message, context, optional screenshot, variables and plural rule, and one or more path/line source
locations. Unit IDs and source hashes are server-derived.

`POST /repositories/{id}/pulls/{pull_id}/localization/translations` requires only an authenticated
identity that can read the repository. It accepts the current `source_revision`, `unit_id`, `locale`,
translated `text`, and optional `note`. A newer proposal or affected source edit marks earlier work
`superseded` without deleting it. These endpoints grant no repository write, review, or merge authority.

`POST /repositories/{id}/pulls/{pull_id}/localization/verification` accepts participant-only
`publish_candidate` and `record_checks` mutations. A candidate binds an existing non-stale named-user
preview to the current pull, locale-plan version, locale, declared journey routes, route interface
hashes, and server-derived translation IDs. A check run must include `variables`, `pluralization`,
`formatting`, `terminology`, `links`, `layout_expansion`, `bidirectional_text`, `fallback_behavior`,
and `localized_journey` results, each grounded to an included route and affected units.

`POST /repositories/{id}/pulls/{pull_id}/localization/previews/{preview_id}/review` accepts `finding`
and `review` mutations from repository translators or named preview guests whose live invitation
permits testing or feedback. Findings retain locale, route, units, category, severity, and rationale;
reviews approve or reject only their named content and retain the translator or regional-reviewer
role. Both surfaces require the current `source_revision` and `expected_version`. Reads expose
separate `locale_plan`, `source_revision`, `translation:{unit_id}`, and `interface:{route}` stale
scopes. Check, finding, and decision writes revalidate the candidate's plan through the current-plan
mutation boundary, so a successor cannot receive evidence for superseded journeys. These
records and preview access grant no Git, merge, release, or repository authority.

Locale delivery uses `POST /repositories/{id}/localization-delivery-policies` (owner only) and
`POST /repositories/{id}/localization-dispositions` (owner only). A policy binds the current locale
plan/version, branch, locales, audience/risk selectors, exact named checks, and a minimum regional
review count. A disposition records `staged`, `deferred`, or `withdrawn` for one policy/locale at an
exact revision and optional release. `GET .../pulls/{pull_id}/localization-readiness` and
`GET .../releases/{release_id}/localization-readiness` project current requirements; pull results are
also included in ordinary merge readiness. Deferred and withdrawn locales remain visible but do not
block required locales.

`POST /repositories/{id}/localized-publications` retains an application or documentation resource's
locale, public version/URL, exact revision, current plan provenance, source/fallback locale, fallback
state (`complete`, `partial`, or `fallback`), and publication state. Readers use
`POST .../localized-publications/{publication_id}/findings` for exact-locale mistranslation, cultural
mismatch, broken formatting, or missing-content evidence. Participants validate or dismiss through
`POST .../localization-findings/{finding_id}/decision`; validated findings may name a human or agent
owner, linked proposal/task URL, and acceptance criteria. `GET /repositories/{id}/localization-delivery`
returns the repository-readable policy, disposition, publication, finding, and repair projection.

## Developer support threads

`GET` and `POST /repositories/{id}/support-threads` list readable questions and
open a contextual support thread. Creation accepts `title`, `body`, a `target`
whose kind is `repository`, `package`, `release`, `api`, `documented_journey`,
or `error`, structured `environment`, `goal`, ordered `attempted_steps`,
`urgency`, `audience`, `contact_preferences`, and attachments. Attachments are
restricted to `log`, `configuration`, or `sample_code`, base64 encoded, and at
most 1 MiB each with at most 10 attachments per thread. At least an in-thread reply or maintainer-contact route is
required.

`GET /repositories/{id}/support-threads/{thread_id}` returns the thread with
server-derived missing-context diagnostics and up to five readable related
answered threads or issues. Related projections contain no attachments or
discussion from the candidates. `PATCH` on that path requires
`expected_version`, `status`, and an optional history message. Current
repository participants can set `open`, `needs_context`, `answered`, or
`closed`; authors can close or reopen their own question. Contact email is
omitted for readers other than the author or a current repository participant.
Audience and support state grant no repository access.

`POST /repositories/{id}/support-threads/{thread_id}/replies` lets the asker or
a current repository participant append a bounded attributable reply using
`expected_version`. Replies share the thread's audience and mutation boundary,
cannot be added after closure, and never grant repository authority. A
participant reply appends an asker-only notification; other readers never
receive that notification projection.

The web support workspace accepts `?repository={id}` so a signed-in package
user can open a public repository's support context without first becoming a
collaborator; repository visibility is still resolved by the API.

## Project knowledge answers

`GET` and `POST /repositories/{id}/knowledge-answers` expose durable project guidance to its declared
`public` or `participants` audience. A proposal contains a developer question, answer summary and
body, plus independently inspectable claims. Every claim states confidence and cites one or more
exact `source`, `symbol`, `documentation`, `package`, `release`, `support_thread`, or `known_issue`
resources with applicable versions. Source, symbol, and documentation commits must be reachable from
a participant-visible non-`vivarium-security/*` branch; private support and issue evidence can never
be used to publish a public answer.

Current repository participants and exact repository-scoped agents use `POST .../{answer_id}/revisions`
with `expected_version` to supersede, but never overwrite, the current revision. Every agent claim
must include explicit uncertainty. Current human participants use `POST .../{answer_id}/responses`
to comment, request clarification, endorse, or challenge the exact current revision. Agents cannot
perform those review actions. The repository owner alone uses `PATCH .../{answer_id}` to mark the
current guidance `verified`, `needs_context`, or `retired`; verification grants no repository or
software authority. Records default beneath `$KNOWLEDGE_ANSWER_STORAGE_ROOT` (`knowledge-answers`).

## Support answer verification

An authorized repository participant launches the ordinary bounded workspace with `POST /workspaces`
using source kind `support_verification` and exact `support_thread_id`, `answer_id`, and
`answer_revision_id`. The thread must state a software version, the answer revision must cite that thread,
and the chosen commit's `.vivarium/workspace.json` remains the source of the isolated image, tools,
dependencies, setup, and resource policy. Selected thread attachments are staged only by a caller that
explicitly declares them sanitized; no platform or repository credentials enter workspace commands.
The attempt endpoint revalidates that the workspace source names that exact thread, answer, and revision.
Private workspaces cannot publish attempts because their raw command evidence would cross into the
repository-readable verification audience; launch a repository-shared sanitized verification workspace.

`GET|POST /repositories/{id}/support-threads/{thread_id}/verification-attempts` and the corresponding
`GET .../{attempt_id}` retain immutable proof from that workspace. Creation binds the exact answer body,
software version, declared environment, sanitized-input digest, commit and workspace-definition digest,
and exact retained command outcomes. It records bounded command text, sanitized logs/output, up to ten
explicitly sanitized 1 MiB artifacts with checksums, server-derived compute seconds, declared cost units,
notes, actor, and a `passed`, `failed`, or `inconclusive` result. Credential-shaped instructions, inputs,
outputs, notes, and artifacts are rejected rather than copied into reusable evidence.

A participant reruns proof in a fresh workspace from the same support source and supplies `rerun_of` when
publishing the new attempt. Reruns must preserve the exact answer revision and sanitized-input digest;
the earlier evidence is never overwritten. Reads derive staleness when the answer instructions, stated
software version, declared environment/dependencies, workspace revision, or workspace definition no
longer matches its frozen provenance. Records default beneath `$SUPPORT_VERIFICATION_STORAGE_ROOT`
(`support-verifications`) and grant no repository, workspace, credential, review, or answer-verification
authority.

## Reusable support solutions

`POST /repositories/{id}/support-threads/{thread_id}/solutions` lets the asker or a current repository
participant resolve a question with an exact passing, non-stale verification attempt. The request names the
answer and revision, attempt, reusable title and summary, `public` or `participants` audience, applicable
versions, limitations, and typed links. Publication freezes the tested instructions and gives attributable
credit to the asker, answer author, verifier, and publisher. Public publication requires a public repository,
thread, and knowledge answer. Documentation collection, `name@version` package, release, and current
contributor-guidance links are resolved server-side; a `search` link makes the solution directly discoverable.

`GET /repositories/{id}/support-solutions?q=...` searches readable active guidance across its title, summary,
instructions, versions, and limitations. `GET .../support-solutions/{solution_id}` returns its exact evidence,
credits, project links, append-only lifecycle events, and relevant credited-participant notifications. Merged
duplicates are omitted from search but remain directly readable for provenance.

Current repository participants use `POST .../support-solutions/{solution_id}/actions` with
`expected_version`. `request_revalidation` names newer versions and moves the projection to
`needs_revalidation`; `archive` marks advice obsolete; `merge_duplicate` names an active canonical solution
with the same audience. These actions never rewrite the original instructions, scope, discussion, authorship,
or verification evidence. Records default beneath `$SUPPORT_SOLUTION_STORAGE_ROOT` (`support-solutions`).
## Infrastructure definitions and observations

`GET /repositories/{id}/infrastructure` lists readable definitions and `GET
/repositories/{id}/infrastructure/{definition_id}` returns one definition with its immutable revision
history, append-only observations, and current diagnostics. Public repositories expose the declared
resource model anonymously. Participant-only observations retain their existence, attribution, status,
visibility, and time but replace provider identity, observed revision, and summary with `restricted` or
an explicit participant-only marker.

Current repository participants use `POST /repositories/{id}/infrastructure` and `POST
/repositories/{id}/infrastructure/{definition_id}/revisions`. A request contains `revision`; successor
requests also contain `expected_version`. Every revision must cite an exact Git commit in `revision`,
name current participant owners, and completely declare one or more `environment`, `service`, `network`,
`identity`, `data_store`, `compute`, or `external_dependency` resources. Resources include provider and
provider-access state, optional non-secret provider identity, exact existing release/environment links,
dependencies, configuration source and sensitivity boundaries, cost/capacity constraints, and security,
privacy, reliability, continuity, and regional commitments. Configuration values are never accepted.

`POST /repositories/{id}/infrastructure/{definition_id}/observations` appends one sanitized observation
bound to an exact retained definition version and provider revision. Managed observations name a resource
in that revision; unmanaged discoveries deliberately omit `resource_id`. Optional release and environment
IDs must resolve in the repository. Visibility is `public` or `participant`, status is `healthy`,
`degraded`, or `unknown`, and `observed_at` records source freshness. Credential-shaped provider,
revision, configuration, summary, or commitment text—including common provider token and access-key
formats—is rejected. Future timestamps are rejected; only current-definition observations from the
preceding 24 hours satisfy current observed-state coverage.

Reads derive diagnostics for unmanaged resources, inaccessible providers, observations older than 24
hours or attached to an earlier definition revision, conflicting owners for one provider identity,
secret-backed configuration boundaries, and resources without a current permitted observation. The
records explain intent and permitted evidence only: they grant no provider access, credentials, Git,
release, deployment, environment, or infrastructure execution authority. Storage defaults to
`$INFRASTRUCTURE_STORAGE_ROOT` (`infrastructure`).

Infrastructure change review is pull-scoped. `POST
/repositories/{id}/pulls/{pull_id}/infrastructure-plans` freezes the current pull source, infrastructure
definition and observations, a `candidate_source` JSON path and SHA-256 digest resolved from that exact
source commit, and candidate-commit policy paths with SHA-256 digests and expected effects. The candidate
declaration is parsed from the retained Git blob rather than accepted from the caller. It returns dependency-ordered operations, affected owners,
classified risks and mitigations, and rollback limits. `GET` on the collection re-resolves all inputs;
`fresh: false` and `stale_reasons` expose drift and stale records project no current acknowledgements.
The candidate file may omit `revision`: the server binds it to the guarded pull source because a Git blob cannot
embed the ID of the commit containing itself. If supplied, `revision` must already equal that exact source.

`POST .../infrastructure-plans/{plan_id}/events` appends an `assumption`, `impact`,
`acknowledgement_request`, or `owner_acknowledgement` with `expected_events` CAS ordering. Participants
and repository-bound read-only agents can investigate, while only a named affected human owner can
acknowledge. Stale plans reject new events. Plans grant no provider, credential, policy, review, merge,
deployment, environment, or execution authority and share `$INFRASTRUCTURE_STORAGE_ROOT`.

`POST .../infrastructure-plans/{plan_id}/rehearsals` freezes a current plan's isolated or
policy-approved ephemeral scope, credential expiry and changed-resource scope, synthetic or permitted
state, and repository checks. Checks cover `provisioning`, `connectivity`, `access`, `policy`,
`service_journey`, `failure`, `cost`, `teardown`, and `recovery`. Replace and destroy changes require
an `unsupported_effects` entry rather than being represented as rehearsed authoritative destruction.

`POST .../rehearsals/{rehearsal_id}/runs` accepts only `workspace_id` and all declared `check_ids`.
The caller-owned workspace must use the exact pull revision and retain one fresh sanitized outcome per
command before credential expiry. The server derives logs, exit status, timing, artifact digests,
candidate resource graph, attestations, and agent actions. Credential-shaped output and stale plans
are rejected; retained evidence grants no provider, production, deployment, review, merge, or
destructive authority.

# Authoritative infrastructure execution

`POST /repositories/{id}/infrastructure-executions` starts an apply only for an exact merged pull plan.
The server re-resolves current plan inputs, requires all affected-owner acknowledgements and the latest
selected rehearsal run to pass, verifies `environment_id` through the established deployment environment
store, and rejects a `budget_units` ceiling above reviewed resource cost constraints. The request also names
the satisfied `environment_policy`, short-lived `credential_expires_at`, and optional exact agent
`delegations`. The record freezes reviewed and merge commits, candidate digest, definition version, plan,
rehearsal, environment, controller, dependency-ordered steps, and resource/step/action credential metadata;
no credential secret is returned or persisted. `GET` on the collection or item exposes this collaboration.
The selected passing rehearsal must either use the exact execution environment ID or be a
`policy_approved_ephemeral` rehearsal whose frozen `policy_approval` exactly equals `environment_policy`;
ordinary isolated preview evidence cannot cross the authoritative environment boundary.

`POST .../infrastructure-executions/{execution_id}/reports` compare-and-swaps `expected_version` and accepts
one exact step's sanitized provider response, state, health, cumulative cost, blockers, next action, and
safety-point state. Dependencies must already have succeeded and aggregate cost cannot exceed the frozen
budget. Failed, degraded, or blocked evidence pauses the apply. A delegated agent can report only its exact
non-destructive step; it cannot steer the execution or gain provider, secret, acknowledgement, destructive,
or unrelated resource authority. The human controller uses `POST .../controls` with `pause`, `resume`, or
`cancel`; transitions occur only at declared safety points, and resume requires remediation evidence to have
cleared retained blockers before the short-lived scope expires.

`POST .../infrastructure-executions/{execution_id}/assessments` compare-and-swaps a terminal or paused execution
version and retains sanitized exact-provider resource outcomes, unmanaged resources, and failed cleanup. The server
compares resource presence and every service, security, privacy, cost, continuity, and candidate commitment measure
frozen at execution creation. It derives `converged: true` only for a succeeded execution with complete passing
evidence and no unmanaged or cleanup remainder; partial and failed evidence is immutable and cannot claim success.

`POST .../monitor-runs` is participant-only and records `granted`, `partial`, or `denied` provider visibility plus
provider availability. Granted or partial runs can retain bounded configuration-drift, unmanaged-change,
failed-cleanup, credential-expiry, and provider-loss findings with sanitized cause attribution; denied runs cannot
assert provider findings. `POST .../drift-responses` compare-and-swaps the execution and links a finding to an
accountable existing issue, proposal, proposal task, repository-scoped incident, or pull. Exception responses use
their ordinary proposal rather than an unverified free-form exception ID. These links
do not create infrastructure authority, rewrite the external observation, or bypass ordinary environment policy,
review, assignment, or merge controls.

# Design acceptance and interface-system evolution

- `POST|GET /repositories/{id}/design-acceptance-policies` publishes and lists
  repository policy; reads include inherited organization policy. Organization
  owners publish portfolio policy at `POST /organizations/{id}/design-acceptance-policies`.
  Selectors are `component`, `journey`, `path`, or `risk_class`; requirements name
  exact approvers for `design_owner`, `accessibility`, `content`, `localization`,
  or `invited_user`.
- `POST .../pulls/{pull_id}/design-acceptances` retains an accepted or rejected
  role decision at the current candidate and policy version. `POST
  .../design-exceptions` is policy-creator-only and cannot exceed the declared
  maximum or 30 days. `GET .../design-readiness` reports missing acceptance,
  unresolved deviation/regression, obsolete component use, stale previews, and
  expiring exceptions; the same projection and blockers appear in ordinary
  `merge-readiness`.
- `GET /repositories/{id}/releases/{release_id}/design-readiness` evaluates exact
  release paths without converting design evidence into release authority. A
  release freezes every included pull's source revision and changed paths; later
  pull synchronization cannot change that evaluation. For a pull revision, the
  latest durable interface check per definition and journey supersedes older
  failed attempts without suppressing independent definitions.
  `POST .../interface-systems/{system_id}/migration-work` creates ordinary
  repository proposal work for implementation or documentation, while `POST
  .../releases/{release_id}/design-repairs` connects feedback or observed
  regressions to ordinary repair work. Neither endpoint grants code, review,
  merge, release, deployment, organization, or repository authority.

# Durable-state migration work

`POST /repositories/{id}/durable-schemas/{schema_id}/migrations/{migration_id}/work`
creates one ordered, repository-owned implementation task. The caller must be able to
read the durable-state plan and already participate in the target repository. The
request compare-and-swaps `expected_version`, names an existing migration `step_id`,
an exact target `base_revision`, human or agent assignment, earlier work IDs, and a
complete compatibility contract covering old/new readers and writers, rollout flags,
idempotency, transformations, ownership, and rollback assumptions.

The response includes `schema`, `migration_work`, and the ordinary assigned `task`.
Subsequent sessions, workspaces, pulls, checks, reviews, and merges use their existing
repository APIs and permissions. Agent sessions reject unmet dependencies until the
earlier task contribution is merged. Schema reads omit work in target repositories the
reader cannot access; definitions, privacy metadata, and data samples are never copied
into the work contract.

# Durable-state migration rehearsals

`POST /repositories/{id}/durable-schemas/{schema_id}/migrations/{migration_id}/rehearsals`
compare-and-swaps `expected_version` and freezes a bounded proof plan. It requires an exact
repository application commit, the migration's exact schema and plan versions, dependency
revision/digest pairs, and either synthetic data metadata or representative metadata with an
explicit privacy method. Raw datasets are not accepted. Checks use the closed kinds `upgrade`,
`dual_read`, `dual_write`, `backfill`, `validation`, `rollback`, and `failure_injection`; each
retains a repository command, a separate repository-defined invariant command, the invariant
claim, and only the revision-input names that affect it.

`POST .../rehearsals/{rehearsal_id}/runs` accepts evidence only from the caller's bounded
workspace in that repository. A run has exactly one outcome per frozen check and retains its
status, exit code, sanitized log (up to 64 KiB), duration, before/after row and object counts,
invariant result, bounded artifact digests, cost, and attestations. A run can be `passed` only
when every outcome passed with exit zero and its invariant held. Status, exit code, log,
duration, invariant result, and the overall result are derived from one unambiguous retained
workspace command outcome per exact command digest rather than trusted from the request.
Caller-supplied counts, artifacts, costs, and attestations are rejected until a platform-retained
source can derive them; unsupported values remain zero or empty, and the platform adds only an
exact workspace/command-outcome attestation.
Both the work command and its distinct invariant command must have one retained outcome whose
start and completion are no earlier than the rehearsal's immutable creation time. Execution
passes only when both exit successfully; process success alone never proves the invariant.
`POST .../notes` appends a
bounded attributable investigation note to an existing run. Repository read authorization
controls all projections; these endpoints execute or review proof but confer no production,
deployment, environment, workspace, or durable-store authority.

# Durable-state production execution

`POST /repositories/{id}/durable-schemas/{schema_id}/migrations/{migration_id}/executions`
opens one compare-and-swap production collaboration only after every step's declared approver has
approved and the selected rehearsal has a passing immutable run. The request freezes an existing
repository release and established release environment, the active schema revision, controller,
old/new compatibility window, privacy constraints, cost ceiling, reversible-abort limit, and any
agent's exact phase, migration step, and bounded mandate. Only one nonterminal execution is allowed.

The server creates the closed ordered phases `expand`, `deploy`, `backfill`, `cutover`, and
`contract`. `POST .../executions/{execution_id}/controls` compare-and-swaps the execution version and
supports `start`, `pause`, `resume`, `throttle`, `report`, `advance`, and reversible `abort`. Reports
retain progress, lag, current invariants, service health, cost, blockers, and next actions. A phase
advances only at 100% progress with healthy service, at least one current invariant, and no blockers;
cost cannot exceed the frozen ceiling. Deployment evidence must resolve to a successful ordinary
promotion of the frozen release in the frozen environment. An agent-attributed request must name
the exact migration step and can report only when its authenticated agent ID, current phase, and
step match one frozen delegation. Agent reports persist as separate step evidence and never overwrite
controller-owned phase readiness fields or satisfy phase advancement; only a human operator can
synthesize phase progress after reviewing every required step. Advancement also requires the latest
report for every current-phase delegation to be 100% complete, healthy, invariant-backed, and
unblocked, so an operator cannot bypass incomplete delegated work. An agent cannot control the execution.
These records carry no command, credential, database, environment,
deployment, or destructive authority.
# Release confidence

Participants manage policy and evidence through `POST /repositories/{id}/quality-requirements`,
`POST /repositories/{id}/quality-attempts`, and `POST /repositories/{id}/quality-overrides`.
Policy updates use `expected_version`; stale updates return `409`. Evidence sources are mutually
exclusive and server-resolved: `scenario_id`, `exploratory_session_id`, or `check_run_id` with its
`pull_request_id`. Attempts name an exact 40-character `revision`, environment, summary, and one of
`passed`, `failed`, `flaky`, `gap`, or `quarantined`. Every attempt resolves an exact pull or release
target. Pull tests remain bound to their resolved pull; scenario coverage additionally requires an
exact-revision retained check whose command matches the published scenario, and its outcome is
derived from that check. Exploratory success requires a chartered retained `signoff` event and a
closed session. The release-signal endpoint server-binds sampled evidence to its selected release;
callers cannot choose the outcome. Matrices retain proof
aimed at another pull or release as stale rather than transferring it across a shared commit.

`GET /repositories/{id}/pulls/{pull_id}/quality-confidence` and
`GET /repositories/{id}/releases/{release_id}/quality-confidence` return the evaluated revision,
policy identity/version, overall `ready` state, and one cell per applicable requirement. Each cell
contains current attempts, invalidated `stale_attempts`, its effective state, and any active scoped
override. Pull merge-readiness embeds the identical projection as `quality_confidence` and reports
the `quality_confidence_incomplete` blocker when it is not ready.

`POST /repositories/{id}/releases/{release_id}/quality-signals` records a post-release sampled
scenario at the release's server-resolved commit. These APIs retain evidence and decisions only;
they grant no execution, environment, Git, review, merge, queue, release, or deployment authority.
