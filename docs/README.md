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

## Current approved-agent trust

Approved-agent participation retains privacy-bounded delivered outcomes, cost and responsiveness,
review corrections, verification/reversion/violation evidence, periodic reevaluation deadlines,
material-profile consent, and actionable anomaly notices in the existing agent-evaluation store.
Organization-owner controls can narrow or suspend trust and retain structured replacement handoffs;
suspension and handoff revoke the linked access grant and credentials without deleting commits or
historical evidence. Profile claims, evaluation evidence, governance standing, and financial limits
remain distinct from technical authority.

The connected browser journey `agent-adoption-journey.spec.ts` proves the complete adoption boundary:
two public candidates are compared for bounded work, project-owned public and protected trials contain
unsafe or over-budget behavior, a sponsor-approved participation produces an ordinary session-backed,
reviewed, checked, and merged Git contribution, and the retained cost/outcome evidence survives a material
profile successor, failed reevaluation, and credential-free replacement handoff. A handoff retires authority;
later profile consent cannot implicitly reactivate it.

## Inspectable project knowledge

The repository knowledge workspace at `/repositories/{id}/knowledge` turns exact, currently visible
source, symbols, documentation, packages, releases, support answers, and known issues into versioned
guidance. Claims retain applicable versions and confidence; scoped-agent claims also retain explicit
uncertainty. Human participants can comment, request clarification, endorse, or challenge an exact
revision, and maintainers distinguish reviewed guidance from proposals or missing-context answers.
Successors preserve the earlier answer and discussion. Evidence visibility is checked before
publication, so a reference to restricted context cannot silently become public advice. These records
coordinate understanding only and grant no Git, package, release, support, or repository authority.

## Reliability contracts

Repository participants publish complete, compare-and-swap service-objective contracts through
`/repositories/{id}/service-objectives`; the web workspace is `/repositories/{id}/reliability`.
Each immutable revision scopes repository, release, or environment behavior and connects named
user journeys to indicators, calculations, measurement windows, targets, dependencies, error
budgets, ordered severity responses, accountable owners, and an exception policy. Links retain the
exact version of related product, performance, accessibility, privacy, and release commitments.

Reads derive attributable diagnostics for missing signals or ownership, unsupported calculations,
conflicting targets, and exceptions that are expired or within seven days of expiry. These records
declare expected dependability and response responsibility; they do not confer repository,
deployment, release, incident, or observability authority. Records default beneath
`$SERVICE_OBJECTIVE_STORAGE_ROOT` (`service-objectives`).

Each contract also retains compare-and-swap signal mappings and append-only observation windows.
Mappings bind an exact contract/objective version and instrumentation revision to repository-defined
metrics, logs, traces, health checks, support reports, deployments, releases, commits, pull requests,
packages, and dependent services. Every source declares how it was sanitized and whether its
reference is public or participant-only; public reads replace restricted references with an explicit
marker. Evidence windows bind the mapping version and delivered-software revisions, retain uncertainty
and known gaps, and derive current/historical attainment, target status, and error-budget consumption
from aggregate good/total counts for ratio/availability, native counts, or an explicit native value
for percentile/custom indicators. Directional native-unit budgets use target deviation rather than
availability math. A changed mapping version or window duration is projected as
incomparable instead of silently extending the earlier series. Credential-shaped source or evidence
text is rejected before persistence, and unsafe legacy references are redacted on every projection;
the records inventory evidence but grant no observability access.

Participants and repository-bound read-only agents can open a reliability investigation from an
exact objective revision, pull, deployment, or recorded budget-consumption window. The record freezes
baseline and affected observations, selected user journeys, and sanitized operational/code evidence.
Human and agent findings must cite that closed evidence set and state confidence and uncertainty;
participants may retain confirmations or disputes, and questions can be addressed only to the exact
service or dependency owners declared by the frozen contract. Successor contracts and missing evidence
project as stale, dependencies without owners remain explicit, and a missing or disputed conclusion is
inconclusive. An investigation can retain a reference to an issue, incident, decision, or planned
improvement without creating authority in that workflow. Investigations are omitted from anonymous
contract reads.
Pulls, deployments, releases, and commits must resolve in the target repository at the named revision;
operational evidence cites an exact retained objective observation, and dependency evidence cites the
frozen contract revision. The persistence store fails closed when this resolver is unavailable.
Read-only agents may open investigations and publish cited analysis, but only human participants may
confirm or dispute it, request or answer owner input, or conclude with resulting work.
Commit evidence must also be reachable from a participant-visible non-`vivarium-security/*` branch,
and every trigger observation belongs to the investigation's exact contract version and objective.

Repository owners apply those objectives through delivery policies scoped by branch, service,
environment, journey, and risk class. Revision-exact predicted and observed impacts cover pulls,
integration queues, releases, and deployments; readiness derives missing/stale evidence, predicted
regression, exhausted budget, dependency failure, required owner acknowledgement, active exception,
and the configured warn, slow, block, pause, or rollback effect with available next actions. Pull
merge readiness and queue admission fail closed for blocking or containment effects. The same public
readiness projection supports release and staged-deployment decisions without making a policy,
acknowledgement, exception, or agent a source of merge, release, or environment authority.
Current-evidence policy resolves each cited observation against the exact contract objective,
measurement window, and delivered resource/revision; error-budget consumption is server-derived from
that retained observation, and caller-supplied budget values never participate in readiness.

Participants convert one cited investigation finding or depleted-budget impact into an immutable
reliability improvement through `/repositories/{id}/service-objectives/{contract_id}/improvements`.
The handoff freezes the objective, baseline and affected observations, affected revisions,
dependency context, evidence, and acceptance criteria while creating an ordinary ordered proposal
task plan with explicit human or agent ownership. Repository authority and the exact default-branch
base are revalidated across proposal publication; the record itself grants no Git, review, merge,
release, or deployment authority.
Impact-authorized work must freeze the same trusted observation that crossed the governing depletion
threshold as a baseline; another same-objective window cannot substitute for that evidence.
The reliability side reserves a stable pending recovery identity before proposal persistence; exact
retries reuse and finish that link, so cross-store failure cannot leave unaccounted proposal work.
That reservation also retains the server-derived depletion observation and original implementation
base, preventing another frozen window or later default-branch movement from changing recovery.

Repository owners record governed release or deployment comparisons through the improvement's
`/verifications` collection. The current observation must cite the exact rollout resource and follow
the retained baseline. The server derives improvement and budget restoration from target attainment
and error-budget consumption: unsuccessful evidence can only retain containment, rollback, or
decision-revisit outcomes, while a successful comparison may restore current budget state without
mutating the earlier user-impact observations.

The connected Playwright reliability journey proves this as one public web/API workflow. It retains a
released-journey objective, dependency ownership, a noisy partial cohort and corrected burn window,
missing dependency evidence, and an exception attempt rejected outside the policy's approval boundary.
Human owners and a repository-bound read-only agent investigate exact deployment evidence before an
ordinary proposal task records scoped agent guidance, compute cost, source changes, checks, independent
review, and merge. A failed first repair stays contained with its rollout comparison; a later reviewed
repair passes staged deployment and restores objective attainment without rewriting the depleted window.

## Recovery commitments

Repository participants publish complete, compare-and-swap recovery contracts at
`/repositories/{id}/recovery-commitments`; the web workspace is `/repositories/{id}/recovery`.
Immutable revisions cover repositories, packages, artifacts, configuration, collaboration records,
and deployed service data. Each target names the user-facing capability it restores, accountable
owners, acceptable loss and restoration time in minutes, retention, storage jurisdictions,
validation criteria, dependencies, and declared exclusions. Typed references connect the agreement
to service objectives, environments, incidents, privacy rules, and governance decisions.

Reads preserve missing target ownership, dependencies without declared protection, dependency
timelines that make a target impossible, and expired or soon-expiring exceptions as attributed
diagnostics. Contract and exception approvers must remain current repository participants when a
revision is published. The records document continuity intent and grant no repository, package,
deployment, data, incident, privacy, or governance authority. Records default beneath
`$RECOVERY_COMMITMENT_STORAGE_ROOT` (`recovery-commitments`).

Repository participants turn current commitment targets into versioned protection plans through
`/repositories/{id}/protection-plans`. A plan binds an exact repository commit or current governed
environment definition, commitment revision, approved `vault://` destination, jurisdiction,
retention and freshness objectives, named recovery owners, and validation checks. Capture walks and
verifies the complete commit tree or reads the server-owned public environment definition, creates a
content and dependency manifest, and encrypts that manifest and its source payload with AES-GCM
beneath `$PROTECTION_PLAN_STORAGE_ROOT` (`protection-plans`). Environment credentials are excluded by
the deployment store's protected read boundary.

Repository-readable projections expose only entry and byte counts, manifest and source checksums,
validation, retention, location, freshness inputs, cost units, and responsible actors. Paths,
contents, ciphertext, nonces, and credentials remain omitted. Every read decrypts and checks the
retained manifest; corruption, key failure, retention expiry, or deletion of a bound commit or
environment changes the capture to non-recoverable. Each capture freezes its plan version, source
resources, and freshness interval, so a successor plan cannot retroactively change historical
evidence. A capture is evidence for later isolated
rehearsal, not routine content access, restoration, repository, environment, or deployment authority.

Declared recovery accessors who remain repository participants can launch exercises from an exact
capture at `/repositories/{id}/recovery-exercises`. The bounded runner restores into an ephemeral,
network-isolated environment without production credentials and accepts only typed restore,
manifest/dependency integrity, declared journey, and manual-confirmation commands. Evidence retains
ordered timing, redacted logs and artifacts, objectives, gaps, manual work, scenario, and actor.
Reads mark retained evidence non-current when its protection plan, commitment, capture, or protected
source changes. Exercises never write restored data into repository or governed environment stores;
records default beneath `$RECOVERY_EXERCISE_STORAGE_ROOT` (`recovery-exercises`).
Repository captures are materialized as checksum-verified Git blobs with restrictive permissions;
governed environment captures restore only their credential-free public projection. The registered
`smoke` journey requires one restored `.vivarium/recovery-smoke.json` v1 contract naming an exact
digest-pinned cached image, restored script/ELF entrypoint, bounded arguments/timeout, required zero
exit code, and exact stdout SHA-256. It runs that application with no network, a read-only source mount,
dropped capabilities, no new privileges, and CPU/memory/process limits. README/static-only captures,
missing entrypoints, or output mismatches fail. Unimplemented declared journey names fail instead of
producing completion evidence, and the ephemeral filesystem is removed after the run is recorded.

Authenticated repository readers, including repository-bound read-only agents, can turn a failed
exercise into a citation-bound investigation. Permitted references resolve to retained exercise
results, visible commits and releases, recovery configuration, dependencies, and accountable owners;
findings retain their author type, confidence, and uncertainty. Investigation never grants restore,
production, Git-write, policy, or delivery authority. The repository owner can freeze one finding and
the exact default-branch base into an ordinary proposal with ordered human/agent tasks. Work then uses
the existing assignment, session/workspace, pull, check, review, integration, release, and approval
controls. A later successful current exercise with changed plan or source evidence links back only
when it retains the same scenario, plan, commitment, step contract, and passes the originally failed
or cited results. This preserves both the original weakness and the accountable repair trail. Code
citations are limited to commits reachable from participant-visible non-security branches. Final
verification also re-resolves the exact proposal origin, acceptance criteria, and ordered task IDs;
every task must be completed by a merged contribution whose implementation descends from the frozen
base and whose context revision still matches the current task mandate. Fresh rehearsal evidence
cannot substitute for missing, obsolete, or unfinished governed delivery.

When an incident or confirmed loss makes normal operation unsafe, affected repository participants
activate one shared recovery operation through `/incidents/{incident_id}/recoveries`. Activation
freezes an exact verified, unexpired protection-plan capture, its manifest and source revision, the
estimated loss window, a versioned objective, independent named approvers, rollback option, and an
acyclic dependency-ordered restoration plan. `$RECOVERY_OPERATION_STORAGE_ROOT` defaults to
`recovery-operations`.

The incident workspace projects active control, approval evidence, progress, blockers, validation,
communications, and rollback. Every mutation compare-and-swaps the workspace version. Steps cannot
begin before required approval or before dependencies validate; rejected approval cannot be resumed,
and failed validation or explicit blockers pause the operation. Once a blocker is contained, an
approved resume moves only failed or blocked steps into an explicit retry state; validated dependencies
remain intact and return to service still requires fresh passing evidence. The creator cannot be an approver.
Validation criteria declare an evidence kind and require one unique passing result whose immutable
resource and SHA-256 reference resolves server-side to the frozen protection capture or retained
incident evidence. Incident evidence must belong to the recovery repository and name the current
step or an exact resource frozen by the selected capture. Agent steps require an exact named agent and explicit delegation, and
no recovery record grants repository, environment, secret, deployment, or destructive-cutover
authority. Returning service is separately recorded only after every frozen step validates.
Every recovery mutation revalidates authority against the operation's exact repository, so access to
another repository in a multi-repository incident cannot cross that boundary and collaborator removal
immediately prevents further response writes. The incident web workspace retains recovery responses in
their own state projection and exposes pause, bounded resume, rollback, and final service-return controls.
Repository-bound agents can mutate only a step that names their exact agent identity and retains explicit
delegation. They cannot approve, publish recovery communications, or use recovery-wide controls; those
controls require the human operation creator or a currently assigned human incident role.

The connected continuity journey carries a regional-loss commitment through encrypted exact-source
protection, isolated rehearsal, cited human-agent diagnosis, ordinary reviewed repair, a fresh successful
exercise, and incident activation. It retains corrupted-capture rejection, missing dependency containment,
revoked responder access, stale concurrent writes, a failed first cutover and bounded retry, public updates,
cost evidence, collaboration history, and the final validated return within the declared objectives.

## Locale coverage contracts

Repository participants publish complete, compare-and-swap locale plans through
`/repositories/{id}/locale-plans`; the web workspace is `/repositories/{id}/locales`. A plan scopes
support to a repository, product, documentation collection, or release and records target language
and region tags, fallback behavior, preferred and avoided terminology, date/time/number/currency/unit
formatting, required journeys, accountable owners and reviewers, and locale-specific release
thresholds. Every translatable resource binds an existing exact repository commit.

Reads compare resource revisions to the current default branch and retain missing ownership or
reviewers, unsupported extraction formats, conflicting preferred terminology, and stale coverage as
explicit diagnostics. Successors preserve prior revisions and require the current version. The
contract documents intended support and does not grant repository, review, or release authority.
Records default beneath `$LOCALE_PLAN_STORAGE_ROOT` (`locale-plans`).

Pull localization reviews at `/repositories/{id}/pulls/{pull_id}/localization` turn those contracts
into attributable work. A participant publishes a repository-defined extraction map and complete
message snapshot for the pull's exact source revision. The server derives stable unit identities and
source hashes from message keys, context, screenshots, variables, plural rules, and locations, then
projects added, changed, removed, reused, and untranslated counts per locale. Any authenticated
repository reader may propose a translation for a current unit without repository write access.
Proposals are append-only; a changed source hash supersedes only affected work and retains its author
and history. The same revision-exact workspace lets authenticated readers claim or hand off locale
units and discuss their context. Human requests bind one scoped agent to a current locale-plan
version and bounded product context; protected or embargoed requests fail closed. Only that agent
may publish a suggestion, which freezes source and plan provenance, cites terminology/prior-work and
source-context evidence, and declares uncertainty. Agent output is never approval: declared locale
reviewers retain attributed approve/reject decisions, while any human may escalate. Every mutation
uses the pull source lock and a workspace compare-and-swap version so concurrent edits are explicit.
Participants also bind the pull revision to an existing named-user preview, current locale plan, and
declared localized journey routes. Complete check suites retain variable, plural, formatting,
terminology, link, expansion, bidirectional-text, fallback, and journey results. Translators and
preview-invited regional reviewers attach findings and decisions to exact routes and units. Evidence
projects stale locale-plan, source, translation-unit, and interface-route dependencies independently,
and later checks or human evidence revalidate the candidate's bound plan version.
Records default beneath `$LOCALIZATION_STORAGE_ROOT` (`localization`).

Repository owners publish audience- and risk-aware delivery policies at
`/repositories/{id}/localization-delivery-policies`. Policies bind the current locale-plan version,
target branch, governed locales, exact check names, and minimum regional review count. Their
revision-exact projection participates in ordinary pull merge readiness and is available for release
candidates at `/repositories/{id}/releases/{release_id}/localization-readiness`. An attributed
per-locale disposition may stage, defer, or withdraw one locale; deferred and withdrawn locales stay
visible but do not block unaffected locale delivery. They also cannot be published until an owner
records a later staging decision. When the locale plan advances, the prior policy fails closed until
a current-version successor is published, then retires from aggregate readiness so it cannot block
fresh current-plan evidence.

Applications and documentation publish locale/version/source-revision/plan provenance and explicit
complete, partial, or fallback state through `/repositories/{id}/localized-publications`. Current
repository readers can report a mistranslation, cultural mismatch, broken formatting, or missing
content against the exact published route and locale. Repository participants validate or dismiss
the report and may bind a validated finding to a human- or approved-agent-owned proposal/task URL
with acceptance criteria. Reports and repair references grant no repository authority.

The connected browser journey proves this as one trail: a product-string revision stales only its
earlier translation, a repository-bound agent cites the current source and locale plan while a human
retains the language decision, and exact French Canadian and Arabic previews retain regional
authorship. Missing review blocks delivery; a confirmed right-to-left failure remains visible while
Arabic is withdrawn and unaffected French delivery passes ordinary checks, review, merge, and
publication. A post-release reader finding links to an ordinary reviewed repair and corrected locale
publication without erasing the prior release, withdrawal, or dissent. Playwright assigns locale-plan
and localization records per-run temporary roots.

## Permitted data handling

Repository collaborators publish complete, compare-and-swap data commitment revisions at
`/repositories/{id}/data`. A revision makes categories, subjects, purposes, collection,
processing, sharing, retention, residency, deletion, consent, and accountable owners inspectable
across repository, release, extension, experiment, and environment scopes before implementation.
Applicable policy and user-notice links are mandatory. Derived diagnostics keep missing ownership,
unsupported guarantees, declared conflicts, and expiring exceptions visible and attributed.
Records default beneath `$DATA_COMMITMENT_STORAGE_ROOT` (`data-commitments`) and document permitted
handling without granting access or operational authority.

Repository-defined data-flow maps on the same workspace bind an exact visible code revision and
exact affected source paths and commitment/data-use versions to a directed path. Privacy review
rejects a candidate map unless that scope covers every path changed by the pull. Typed nodes cover user interactions,
interfaces, packages, stores, extensions, releases, environments, audiences, and external
recipients; edges declare the categories, purpose, operation, and whether another retained copy is
created. Inaccessible dependencies and uncertainty remain explicit without copying their protected
contents. Current participants publish declarations, while participants and repository-bound
read-only agents may add bounded code citations and findings. Successor declarations make earlier
analysis stale, and projections keep undeclared flows and declared/observed differences visible.
Records default beneath `$DATA_FLOW_STORAGE_ROOT` (`data-flows`) and grant no data or repository
authority.

Pull requests compare a candidate data-flow revision and its exact commitment/data-use versions
with a target-revision flow at `/repositories/{id}/pulls/{pull_id}/privacy-review`. The server
classifies changed collection, purposes, recipients, retention, access, and user controls, then
derives required owner acknowledgement, notice, consent, migration, test, and exception work.
Current collaborators and repository-bound read-only agents may retain bounded challenges,
mitigations, residual-risk analysis, and source citations; only a human collaborator can acknowledge
every required action. Acceptance binds the exact pull source revision. Synchronization makes it
stale, and the next comparison retains the complete earlier review in history. Records default
beneath `$PRIVACY_REVIEW_STORAGE_ROOT` (`privacy-reviews`) and grant no data, review, or merge
authority.

Repository owners add runtime privacy gates with privacy-check policies. A policy selects target
branches and paths, the required collection, consent, minimization, access, retention, export,
deletion, telemetry, and recipient rules, synthetic journeys, and current human privacy owners.
Each retained run must bind the pull's exact source revision, a matching data-flow version, and an
existing ephemeral network-isolated preview; production data is rejected. Pull evidence contains
only bounded sanitized log/trace/artifact metadata, content digests, rule outcomes, and coverage.
Caller-authored result summaries and artifact display text are never retained: the server replaces
them with deterministic descriptions and restricts remaining labels to bounded identifier syntax.
Run journeys must resolve to the owner-published policy, and coverage is derived from validated rule
results rather than retained from the submitter.
Current evidence plus a named privacy-owner acknowledgement governs ordinary merge readiness and
the same exact revision/path context governs release readiness. Repository-owner exceptions name
only affected rules, expire within 90 days, preserve rationale, and require linked follow-up work.
Pull evidence, acknowledgement, and exceptions additionally bind the pull identity, preventing a
sibling pull at the same commit from reusing its proof; release evaluation uses an explicit
revision-wide context.
Records default beneath `$PRIVACY_CHECK_STORAGE_ROOT` (`privacy-checks`).

Permitted production observations close the loop after deployment without retaining the people or
captured values that exposed a gap. Private records at `/repositories/{id}/data-observations` accept
only digest-addressed aggregate/audit metadata and connect it to the exact flow, commitment/use,
release, environment, deployment, optional extension, and accountable owners. Human collaborators can
contain use, notify participants, retain a private incident or expiring governed exception, and create
a human- or approved-agent-owned ordinary proposal task carrying only sanitized evidence. Existing
review, verification, release, and deployment boundaries remain authoritative. Records default beneath
`$DATA_OBSERVATION_STORAGE_ROOT` (`data-observations`). New extension-scoped signals require the
installation to remain active for that repository; revocation preserves earlier evidence but fails
closed for later submissions.

The connected privacy-engineering browser journey freezes an existing account path, compares a new
external-recipient flow, retains a cited challenge, proves source movement makes the first analysis
stale, and verifies the revised design in an isolated synthetic preview. A non-owner cannot publish
an exception, current privacy-owner acknowledgement is required before ordinary review and merge,
and the commitment and evidence remain visible after release. Playwright isolates commitment, flow,
privacy-review, privacy-check, and production-observation records.

## Accountable product direction

The repository roadmap at `/repositories/{id}/roadmap` turns exact product-opportunity versions
into visible accepted, deferred, or rejected decisions. Every comparison retains goal fit,
capacity, dependencies, risks, governance decisions, existing commitments, and the reason for the
outcome. Accepted outcomes additionally name accountable owners, target horizons, success
measures, and explicit sequence.

Human repository participants alone publish commitments. Readers and agents may discuss tradeoffs
and propose visibly non-binding scenarios, but those scenarios cannot reserve resources. A roadmap
is append-only by revision and compare-and-swap protected; every revision after publication must
state an attributed replan reason and triggers such as scope movement, conflicting commitments,
owner unavailability, or target slip. Records default beneath `$ROADMAP_STORAGE_ROOT` (`roadmaps`).

Accepted roadmap items can be tested before delivery through revision-exact outcome validations at
`/repositories/{id}/outcome-validations`. A collaborator freezes the roadmap item and cited product-
opportunity version, then opens a technical decision, prototype, documentation concept, or product
experiment with representative success and guardrail measures traced to source evidence. Named
participants may be invited to the exact preview or bounded research revision; invitations require
explicit consent, permit later consent withdrawal, expire, and grant no repository access. Findings retain accessibility needs, dissent,
acceptance, and evidence quality. Conclusions can validate, revise, defer, or reject direction without
changing earlier roadmap revisions. Records default beneath `$OUTCOME_VALIDATION_STORAGE_ROOT`
(`outcome-validations`). An exact accepted roadmap item can also create an ordinary ordered proposal
and human/agent task plan. Each task traces to its frozen user need and success measures; linked
review, check, integration, release, and deployment evidence reports delivery
back to the roadmap. Shipping alone leaves the item delivering. All success measures require retained
measurement evidence before value is achieved, while changed assumptions, unresolved needs, policy
conflicts, or failed measures require an explicit decision revisit.

After a decision, preview, delivery, rejection, or measured outcome, maintainers publish
feedback-specific learning updates from the roadmap. Only the cited reporter and current repository
participants receive the retained rationale and inspectable work link; private source membership is
not exposed. Reporters can say improved, not improved, or unsure, append follow-up evidence, and leave
future conversation. Maintainer reviews compare promised and observed outcomes, retain lessons and
dissent, link resulting work, and continue, revise, or archive an opportunity as fulfilled or
unsupported without erasing its evidence or credit.

## Product experiment plans

Repository participants define versioned product-learning contracts at
`/repositories/{id}/product-experiments`; the web workspace is
`/repositories/{id}/experiments`. Each plan starts from a proposal, issue, technical decision,
pull request, preview, or release and names a hypothesis, variants, permitted audience, success
and guardrail metrics, minimum evidence, duration, owners, assumptions, and stop conditions.

Every metric binds an exact version of a declared product signal with its event, unit, privacy
boundary, and instrumentation status. Missing instrumentation, ineligible audiences, overlapping
audience/signal contracts, and approvals invalidated by successor assumptions remain explicit and
attributable. Discussion and approval bind the current plan version and grant no rollout, data,
release, or deployment authority. Records default beneath `$PRODUCT_EXPERIMENT_STORAGE_ROOT`
(`product-experiments`).

Before rollout, the repository owner freezes reviewed variants to an exact release with a public
audience contract covering eligibility, region/organization bounds, deterministic randomization,
mutual exclusion, allocation, consent, minimal data, and retention. Stable receipts retain salted
subject digests; stale, conflicting, over-allocated, unauthorized, or mismatched admission fails closed.

Live attempts bind that contract to successful established-environment deployments of the exact
release. Participants append governed allocation stages and observe exposure, measures, samples,
uncertainty, cost, data quality, consent, and operational health. Pause, resume, stop, and automatic
containment are retained as attempt history; containment stops new assignment without reallocating
any subject or deleting prior evidence. Guardrails use the launch-time plan revision, while every new
assignment fails closed against current authoritative health for all launched deployments.

When the plan's minimum evidence or a stop condition is reached, participants freeze analysis to
the exact plan and run versions. The record retains segment effects, uncertainty, excluded evidence,
guardrail outcomes, human- or agent-attributed interpretation, and dissent. A versioned outcome
adopts a treatment, retains control, extends the test, or declares it inconclusive, then creates
rollout, rollback, or follow-up work alongside required retirement tasks. Cleanup is complete only
after evidence links prove obsolete variants, targeting, credentials, and collection are removed;
aggregated observations and review, release, and deployment provenance remain durable.
Agent interpretation requires a current operator of an organization-approved agent and retains the
human operator separately as the authenticated writer; a client-supplied agent name alone is never
accepted as attribution.

## Product feedback

Authenticated users who can read a repository can submit broader product needs through
`/repositories/{id}/feedback`; the repository workspace links to the corresponding `/feedback`
page. A submission targets the project, a release, a documented journey, or a preview and records
the need, desired outcome, frequency, impact, audience, provenance, contact preference, discussion,
and append-only history.

Evidence is accepted only when the reporter explicitly marks the supplied summary as redacted and
assigns it an audience (`audience`, `maintainers`, or `reporter_only`). Reporter identity and direct
contact use separate projection controls. Organization-private submissions require an organization
repository and are visible only to the reporter and current repository participants. Related issue
and product-experiment links are resolved within the same repository, but linked protected content
is never copied into feedback projections. Records default beneath `$FEEDBACK_STORAGE_ROOT`
(`feedback`).

## Collaborative accessibility commitments

Repository participants publish complete, append-only accessibility contracts
through `/repositories/{id}/accessibility-commitments`; the web workspace is
`/repositories/{id}/accessibility`. Contracts cover repositories, documented
journeys, components, and releases, retaining applicable standards, assistive
technology/version/environment support, audiences and access needs, required
scenarios and observable outcomes, severity response policy, owners, testable
requirements, and narrowly scoped expiring exceptions. Roadmap outcome,
documentation, preview, and release-policy links retain their publisher.

Successors use compare-and-swap versions and preserve their author and rationale.
Reads keep missing scenario coverage, declared requirement conflicts, unsupported
environments, and expiring or expired exceptions explicit and attributable; the
contract grants no repository authority and is not itself evidence of
conformance. Records default beneath `$ACCESSIBILITY_COMMITMENT_STORAGE_ROOT`
(`accessibility-commitments`) with repository-scoped records: unrelated
corruption is isolated, while corruption in the requested collection is
reported rather than silently returning an incomplete contract.

Lived accessibility barriers are retained separately at
`/repositories/{id}/accessibility-reports` and rendered in the same accessibility workspace. A
permitted reporter freezes a release, page, documentation journey, or preview revision and shares
functional access needs, steps, expected behavior, and explicitly redacted visual, audio, tree, or
input evidence. Identity and sensitive device detail have independent consent controls. Current
repository participants can retain revision-exact bounded workspace/preview attempts classified as
reproducible, intermittent, environment-specific, or unconfirmed; the record supplies evidence, not
execution authority or proof of conformance. Storage defaults beneath
`$ACCESSIBILITY_REPORT_STORAGE_ROOT` (`accessibility-reports`).

Revision-exact assessment evidence is retained at
`/repositories/{id}/accessibility-assessments` and appears in both the accessibility workspace and
matching pull request. Repository-defined checks classify semantics, keyboard, focus, contrast,
motion, captions, and declared journeys as passed, failed, or unevaluated with audience and source
coverage. Participants and repository-bound read-only agents add exact-revision preview or
reproduction citations, severity, uncertainty, duplicate links, and human-evaluation needs; only a
human participant can accept or mark a false positive. Explicit source/journey invalidation clears
only affected evidence and acceptance. Storage defaults beneath
`$ACCESSIBILITY_ASSESSMENT_STORAGE_ROOT` (`accessibility-assessments`).

Accepted assessment findings can become ordinary governed repair proposals and human- or agent-owned
tasks without re-authoring the affected user's account. The handoff freezes the exact base revision,
selected commitment revision, permitted redacted reproduction references, acceptance criteria, and
component guidance into the task context used by shared workspaces and agent sessions. Connected pull
requests must separate design and code changes from interaction and content tradeoffs; current-revision
previews and task/pull status project back onto the original finding while established repository,
agent, review, check, and merge authority remains in force.

Accessibility delivery policies turn those contracts into exact-candidate gates. Owners select
branches, paths, journeys, or risk classes and require current automated checks, scenario coverage,
and named-role acknowledgements. Pull merge readiness and release readiness distinguish missing,
stale, unevaluated, failed, and unresolved barrier evidence. Evaluators use existing bounded preview
guest access and retain a revision-bound confirmation or rejection; dissent remains visible.
Expiring owner overrides require rationale and concrete follow-up work and never rewrite the result.
Records default beneath `$ACCESSIBILITY_DELIVERY_STORAGE_ROOT` (`accessibility-delivery`).

The connected accessibility browser journey starts with a privacy-bounded report against a released
journey and carries its retained reproduction through specialist classification, a corrected false
positive, an assignment-scoped agent repair, stock Git, exact-revision automation, and bounded
keyboard/screen-reader preview confirmation by the reporter. Advancing the candidate proves that an
earlier acknowledgement becomes stale; an expiring owner exception disappears without erasing its
follow-up. Ordinary independent review, merge, and release retain the accepted finding, authored
tradeoffs, current candidate assessment, and scenario as regression evidence. Release readiness
re-evaluates the merge revision independently and therefore reports pull-candidate evidence as stale
rather than silently treating it as release evidence.

## Collaborative performance goals

Repository participants publish complete, append-only performance-contract revisions through
`/repositories/{id}/performance-goals`; the web surface is `/repositories/{id}/performance`.
A contract identifies a repository, release, user journey, API, command, or service and records
workloads, metric target ranges and baselines, correctness constraints, supported environments,
owners, budgets, and issue, incident, preview, release, or decision links.

Successors use a compare-and-swap version and retain their author and rationale. Reads keep
missing measurements, incomparable environments, stale baselines, current target gaps, and
non-overlapping successor targets explicit and attributable. Goals are evidence context, not new
authority or proof that a benchmark passed. Records default beneath
`$PERFORMANCE_GOAL_STORAGE_ROOT` (`performance-goals`). Authorized collaborators attach exact-
revision or release-attested trials at `/repositories/{id}/performance-trials`, retaining sanitized
workload, environment, sampling, timing variance, resource, trace/log/artifact, and cost evidence.
Comparisons surface incompatible measurement conditions instead of presenting them as signal.

Owners may publish merge-performance policies for a target branch, optionally narrowed by changed
path or declared risk class. Each policy names current goals, an allowed regression percentage,
minimum statistical confidence, and whether correctness must pass. Pull merge readiness evaluates
the current exact-revision evaluation live, so missing, stale, incomparable, uncertain, incorrect,
or regressed evidence blocks ordinary merge and queue admission.

After integration, release observations bind that same evaluation revision to an immutable release,
its exact deployment, and a release- or production-derived trial. The projection recommends pausing,
restoring a known-good release, opening ordinary repair work, or revisiting a decision when results
regress or remain uncertain; execution stays behind existing deployment, recovery, review, check,
and merge authority.

The connected browser journey proves the complete concern-to-outcome trail through public API and rendered
performance/pull surfaces. It deliberately retains a noisy benchmark retry, a correctness-blocked evaluation,
ordered staging and production promotion, a missed production target with containment recommendations, and a
successful production remeasurement, all bound to the same goal, diagnosis, candidate revision, release, and
deployment evidence.

## Governed community decisions

The `/governance` workspace turns an active repository or organization charter decision class
into a reviewable community proposal. A proposal publishes its source, alternatives, citations,
affected resources, disclosures, discussion window, and implementation effects, and freezes the
charter version with its eligible roles, quorum, and approval threshold.

Humans must remain eligible when casting and when the deadline is tallied. Ballots are final and
unique; abstention and conflict recusal are explicit. Approved organization agents can publish
cited analysis but never vote. Attributable ballots retain dissent. A secret ballot exposes each
voter only their own receipt, while others see aggregate counts and a verification digest.
Contests and eligibility changes remain attached to the final tally. Records default beneath
`$GOVERNANCE_STORAGE_ROOT` (`governance`).

An accepted, uncontested result may issue one content-addressed decision receipt and
repository-owner-published ordinary task plan. Its steps distinguish the community mandate from
remaining resource approvals. Scope, cost, assumptions, and protected effects are frozen;
material changes require a new or amended decision.

## Federation identity

Every instance publishes a signed, versioned identity document at
`/.well-known/vivarium-federation`. Its stable instance ID is verified against
the retained immutable root Ed25519 key (the first key), and the signed document declares public endpoints,
capabilities, operators, and current/retired verification keys. Configuration
uses `FEDERATION_PUBLIC_URL`, `FEDERATION_INSTANCE_NAME`, and
`FEDERATION_OPERATORS` (comma-separated local user IDs with exclusive
administrative authority; empty fails closed); durable identity and peer trust default to
`FEDERATION_STORAGE_ROOT=federation`.
Authenticated operators rotate the instance signing key through
`POST /federation/identity/rotate`; rotation increments the signed document
version, retains the predecessor as explicitly retired verification history,
and has that predecessor authorize the successor document for trust continuity.

Authenticated users manage peer discovery at `/federation`. First contact
verifies the document signature, while later version/key changes enter a
visible `changed` state until accepted. Failed refreshes retain `unreachable`,
and revoked trust cannot be silently restored by refresh. Discovery permits
HTTP only for loopback development and otherwise requires HTTPS to exclusively
public IP addresses, validated both after DNS resolution and again at dial time.

Public actor cards resolve exact stable identities as
`{instance-id}:user:{user-id}` and
`{instance-id}:agent:{organization-id}:{agent-id}`. They intentionally expose
no actor directory or private organization/team membership. A federated actor
is attributable remote identity, never a local login, credential, permission,
or proof of current authorization; later collaboration protocols must verify
signed messages against the retained peer document and their own live policy.

Trusted peers advertise `repository-discovery.v1` and publish bounded signed
repository projections at `/federation/repositories/{id}`. Public projections
name the authoritative instance-qualified reference and exact default revision,
and may include branches, immutable releases, contributor guidance, public
issues, and open contribution opportunities. They contain metadata only: no Git
objects, private discussion, credentials, membership, or implied local control.

Authenticated developers resolve `{instance-id}:{repository-id}` through their
home instance. The home verifies the response against the exact retained peer
identity-document version and active Ed25519 key before caching it. Refreshes
retain explicit `current`, `unreachable`, `inaccessible`, `unsupported`, or
`invalid-signature` state; a prior permitted snapshot may remain visible with a
stale timestamp and error, but inaccessible content is never newly copied.
Following a reference is local account metadata used by `/federation/follows`;
it grants no remote or local repository authority. The `/federation` workspace
labels cached context as remote and links to its authoritative endpoint.

Trusted peers advertising `repository-contribution.v1` support independently
hosted contributions. A developer creates a private local fork from one exact
advertised public branch revision; the home verifies and imports only that
revision's reachable Git object closure. The fork uses ordinary local Git
credentials and can fast-forward its selected upstream branch after refreshing
and re-verifying the remote projection.

Opening a remote pull freezes the local source and remote target revisions. The
home signs an idempotent envelope with bounded purpose and instance-qualified
author attribution, then transfers only the source revision's reachable bundle.
The target verifies explicit peer trust, signature, target tip, bundle integrity,
and exact commit before creating ordinary review. Neither instance receives a
credential, collaborator grant, or repository write authority on the other.

Federated pull collaboration uses immutable signed events keyed by the original
contribution identity. Repository participants can share discussion, review and
requested-change decisions, exact revision updates, bounded check/preview
evidence, and closure state. Receivers verify current peer trust and signature,
deduplicate the origin/event identity, retain the remote actor and verification
status, and derive staleness against the currently adopted source revision.
Delivery failure is retained as `pending` (`202`) for safe retry; reuse of an
event identity with different content conflicts. Shared check and preview claims
are evidence only: upstream required checks, embargo filtering, repository
visibility, review permission, closure, and merge authority remain local.

After those local rules pass, the repository owner merges the immutable adopted
revision through the ordinary atomic pull boundary. The target retains the
source objects and authorship in its own Git history, then freezes the signed
collaboration events, source identity/revision, maintainer, and merge commit in
a locally signed `receipt`. Receipt delivery uses a durable outbox: an offline,
deleted, or later-untrusted source cannot undo accepted history or remain a
live authorization dependency, while identical retries cannot merge twice.

An authorized source-instance participant can delegate the contribution to an
approved agent they operate with `POST
/federation/contributions/{id}/agent-sessions`. The mandate freezes the current
source revision and only selected paths that exist there. Its short-lived
credential is bound to the local fork, exact branch, and contribution identity.
Completion accepts only descendant branch-tip commits, derives file evidence
locally, transfers the bounded exact revision, and shares a signed
`agent_session` summary with attributed commands, evidence, costs, and residual
concerns. Guidance, control, and revocation remain local; no peer credential,
secret, check, review, or merge access is granted.

## Living documentation

Repository owners define documentation collections through
`/repositories/{id}/documentation`. Each immutable revision selects an exact
source branch and commit, repository root path, supported source or release
versions, owners, audience, navigation, rendering, and review/publication
policy. Publication freezes every supported page's Git blob, SHA-256, title,
author, ordering, and links to symbols, packages, releases, decisions, issues,
and contributor guidance.

Reads compare the reviewed snapshot with current source and release mappings.
Missing owners, deleted paths, changed blobs, and moved versions remain explicit
diagnostics while history is retained. Records default beneath
`$DOCUMENTATION_STORAGE_ROOT` (`documentation`); the web workspace lives at
`/repositories/{id}/documentation`.

Repository participants can open evidence-bound documentation tasks from
proposals, issues, pulls, releases, investigations, or stewardship findings.
Each freezes an exact revision, reserves a scoped documentation branch, and
retains rendered drafts, citations, attributable collaboration, and sourced
agent assistance with explicit uncertainty without granting new authority.

Repositories verify those promises with versioned
`.vivarium/documentation-checks.json` configuration. Each declared check names
a documentation collection, selectors such as links, symbols, builds, samples,
commands, or tutorials, the repository paths that affect its evidence, and an
exact source, package, or release revision matrix. Every matrix cell runs as an
ordinary bounded check in a network-disabled, read-only repository snapshot;
`VIVARIUM_OUTPUT` retains generated pages, coverage, and expected/actual output
differences as downloadable artifacts. Pull surfaces retain the logs, artifacts,
target revision, selectors, and dependency digest. Generated names such as
`docs/guide [v1.0.0]` can be selected by the existing target-branch required
check policy, so documentation failures participate in normal merge readiness.
Matrix targets may omit `revision`; the server then freezes the exact candidate
commit it archives and executes. An explicitly supplied revision remains an
assertion and must equal that candidate, avoiding an impossible requirement for
a Git commit to contain its own SHA while preserving exact evidence.

Pull requests can additionally freeze a documentation experience review at
`/repositories/{id}/pulls/{pull-id}/documentation-review`. The review renders
changed pages from the exact candidate commit, compares reader navigation with
the target commit, joins retained documentation-check evidence, lists affected
versions, and retains explicit gaps. Comments, change requests, bounded
stakeholder feedback, and technical or audience decisions bind to a page
content SHA-256 and review area. Synchronization makes only changed-page
evidence stale. Expiring owner invitations scope non-participant access to
named review areas and grant no repository or publication authority.

Changing a declared dependency changes only the evidence digest and runs for
checks that name it. A target must always carry the exact 40-character
candidate Git revision that is archived and executed, including when its label
represents a package or release version;
unreadable dependencies or malformed matrices fail closed as
`documentation/configuration`.

When an owner merges an ordinary authorized pull into a collection's configured
publication branch, changed pages are frozen as a new immutable collection
revision and linked to that pull. Exact retries reuse the publication. Readers
open stable page slugs, select retained source/release labels, search only
currently visible collections, follow policy-owned permanent redirects, and
see older publications explicitly marked archived. Signed-in readers can bind
page feedback, failed examples, search misses, and version mismatches to the
exact publication with bounded evidence. Owners triage each retained signal
once by linking an existing issue, proposal, or human/agent documentation task;
triage grants no authority to the reporter.

The connected living-documentation journey proves this as one governed loop:
a contributor proposes behavior and guidance together, a grounded task-session
agent retains exact command evidence, an owner reviews rendered pages and
required version checks, and merge publishes an immutable revision. A reader
then reports a failed instruction against the archived release; maintainers
retain and reproduce that attributed outcome, link it to a release-frozen
documentation task, and publish corrected version-specific guidance through a
second ordinary pull, review, check, merge, and release. Playwright isolates
documentation records beneath `$DOCUMENTATION_STORAGE_ROOT` with its other
temporary API stores.

## Project governance charters

Repository and organization owners publish append-only charter revisions at
`/repositories/{id}/charter` and `/organizations/{id}/charter`. A revision defines named
governance roles and eligibility, decision classes, participation and quorum, approval rules,
protected branch/release/environment/security/agent resources, terms, removal and succession,
and amendment policy. Draft approval, activation, and time-bounded resource exceptions retain
their actor and time independently beneath `$CHARTER_STORAGE_ROOT` (`charters`).

Every read projects the current charter against live ownership, collaborators, organization
teams and policy, required checks, and agent authority. Activation repeats that preview and
rejects impossible quorum or unsupported protected-resource rules. Activation affects future
decisions only: earlier revisions, approvals, exceptions, and completed work remain historical
evidence. Eligibility is derived per decision class from closed live identity sources rather
than descriptive text, and exceptions must target a declared resource in the active revision.
The web surfaces live at `/repositories/{id}/charter` and
`/organizations/{id}/charter`.

The connected governance journey proves this as one collaboration loop: a repository adopts a
charter, admits evidence-backed contributor standing, retains dissent and recusal in an accepted
initiative, and routes human- and agent-owned tasks through ordinary Git, review, required checks,
merge, and release. A failed quorum, standing appeal, successor election and handoff, and
time-bounded relinquished emergency recovery preserve the active charter and history. Governance
standing and accepted results remain legitimacy records only; repository participation,
credentials, review, merge, and release authority are still approved and enforced independently.

Active charters also retain continuity actions for nomination, election, recall, succession,
and emergency recovery. Each action names the exact charter role, governed proposal, protected
resources, predecessor/successor standing where applicable, mandatory review, and automatic
expiry. Pending, active, completed, relinquished, and expired projections expose unresolved
handoffs and recovery review without changing repository identity or historical attribution.
These records grant no resource authority: repository and organization owners must separately
approve access and revoke any derived credentials at the owning authority boundary.

An active charter can admit time-bounded human governance standing through the scope's
`/charter/standing` API. Each invitation names an exact charter role and responsibilities and
retains reviewed contribution, review, support, ownership, or membership evidence. The invited
person alone accepts, declines, recuses with a disclosed conflict, or appeals a suspension or
revocation; the scope owner alone suspends, reinstates, or revokes. Every transition is an
actor-stamped append-only event.

Charter reads project stored standing together with expiry and current local identity or
membership loss, available nominations or appeals, and independently derived operational
access. Governance standing never mints credentials or grants code, secret, merge, deployment,
or repository access. Standing remains attached to the charter revision that admitted it, so a
later amendment cannot rewrite its evidence or history.

## Contributor pathways

Repository owners publish the project's participation contract before a
newcomer invests effort. Each contributor pathway is an immutable numbered
revision covering goals, prerequisites, conduct and security expectations,
supported setup and verification, communication, review, and work categories
for humans, agents, or either. Public repositories expose the current pathway
without authentication; private repositories retain their normal read boundary.

References to documentation, current ownership, releases, issues, proposals,
and workspace definitions are projected against live repository state on every
read. Missing, moved, or private requirements remain in history and are marked
stale or inaccessible rather than disappearing. Authenticated readers can
acknowledge an exact revision. Public reads expose only a non-identifying count;
owners receive full attribution and other authenticated readers only their own
acknowledgement. Publication and acknowledgement attribution are append-only beneath `$CONTRIBUTOR_PATHWAY_STORAGE_ROOT` (default
`contributor-pathways`). The repository's “How to contribute” surface supports
the same publish, inspect, health, history, and acknowledgement flow.

Repository owners can also publish source-grounded contribution opportunities
from triaged issues, proposals, planned tasks, and stewardship findings. Each
record freezes the current revision, expected outcome, bounded scope, required
skills, topical interests, dependencies, risk, estimate, mentors, and whether
agent assistance is available. `POST
/repositories/{id}/contribution-opportunity-matches` compares a reader's skills,
interests, time, risk preference, and assistance preference with live records
and returns both reasons and gaps; the profile is evaluated for the request and
is not retained.

Authenticated readers may claim one exact opportunity version for one hour to
14 days or release their claim. Claims are attributable, compare-and-swap
updates; an active claim excludes duplicate reservations and disappears from
the live projection after release or expiry. It is coordination metadata only:
claiming never adds repository collaboration, Git, branch, task, workspace, or
agent authority. Records default beneath
`$CONTRIBUTION_OPPORTUNITY_STORAGE_ROOT` (`contribution-opportunities`) and are
available in the repository's “How to contribute” matcher.

A contributor holding the live exact-version claim can launch that opportunity
as an independently owned private fork and shared workspace. Launch freezes the
opportunity revision, current contributor-pathway revision, source evidence,
expected outcome, scope, prerequisites, and setup guidance into the workspace.
The fork's `.vivarium/workspace.json` provisions the isolated environment and
the pathway verification commands append setup evidence; failures remain
visible as non-reproducible workspace state. Selected issue attachments may be
staged only after allowlisting and secret-like-content rejection. Missing
evidence, revisions, guidance, or workspace definitions fail before fork
creation, while obsolete instructions remain visible diagnostics. The
workspace links to exact-revision explanations and grants write access only to
the contributor's fork, never upstream.

Each launched contribution workspace carries a shared, version-guarded help
thread between its contributor and designated mentors. Questions, advice,
interventions, requested checkpoints and responses, and handoffs retain actor,
time, response target, status, and whether the contributor or maintainer still
owns the decision. Mentor availability stays visible; changed opportunity
scope or lost upstream mentor access derives an attributable reassignment or
exit path without deleting progress.

Approved-agent help remains subordinate to workspace control. An operated
approved agent may explain or diagnose only under a live guide lease, and may
record an edit only under the matching edit lease. The thread grants no
upstream authority and does not transfer authorship. It is retained with the
workspace beneath `$WORKSPACE_STORAGE_ROOT`.

The contributor can checkpoint that guided work and preflight it for upstream
publication. The preflight distinguishes fixable project requirements from
coaching needs, so unresolved support context is visible without silently
becoming a merge blocker. Once requirements and the frozen acceptance criteria
are confirmed, the platform commits only inspected checkpoint files to the
contributor's fork and opens an ordinary upstream pull. The pull preserves the
opportunity and pathway revisions, setup evidence, mentor guidance, approved
agent assistance, acceptance criteria, and contributor attribution; it gains
no review, check, acknowledgement, queue, permission, or merge bypass.

After that ordinary pull is merged and included in an immutable release, the
repository owner closes the exact in-progress opportunity through `POST
/repositories/{id}/contribution-opportunities/{opportunity-id}/completion`.
The server verifies the guided pull, its merge, release inclusion, and release
contributor credit before retaining implementation credit, bounded feedback,
server-derived setup and mentor/agent support counts, recognized skills, and
readiness for an optional currently-open next opportunity. Exact retries reuse
the outcome, and completed outcomes remain visible on the contributor surface.

## Accountable decision delivery

An accepted technical decision can be converted once per commitment into an
ordinary proposal with ordered human- and agent-owned tasks. The server freezes
the current default-branch revision and derives every task outcome and
verification plan from explicit decision constraints and success measures; the
plan is rejected unless all of them are covered. Decision and commitment
identity remains on proposal and task reasoning, so existing scoped sessions,
workspaces, linked pulls, checks, release inclusions, and deployments preserve
the rationale without gaining any new authority or bypass.

Collaborators append attributable delivery observations linked to a review,
check, integration attempt, release, or deployment. Confirmed coverage remains
evidence on the accepted commitment. A deviation, changed assumption, failed
measure, or incompatible work reopens the exact commitment and decision with an
actionable reason, requiring explicit reconsideration rather than allowing
downstream delivery to silently claim the obsolete choice.

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
commit. Definitions may set bounded `cpus`, `memory_mb`, and `storage_mb`; the
executor applies CPU and memory to the container and storage to live output and
artifact collection, while omitted values keep the established defaults. Each
command executes in a bounded, network-disabled OCI container
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
Pull revisions can opt into an exact change preview with version 1 of
`.vivarium/preview.json`. It freezes the image, build command, working
directory, scoped non-secret environment, output path, and bounded resources.
A current collaborator launches it from the pull; the network-disabled
executor builds that adopted commit and retains setup events and checksummed
output. Successful builds expose only `index.html` through an authenticated,
sandboxed URL. Records beneath `$PREVIEW_STORAGE_ROOT` (default `previews`)
retain creator, revision, definition SHA-256, lifecycle, logs, and failures.
The executor copies the immutable Git archive into a disposable bounded tmpfs
before building, so source evidence stays read-only while ordinary compilers
can create the declared, separately size-checked artifact output.
The platform measures and reserves the immutable source copy separately from
the preview's declared scratch allowance, so repository size cannot consume the
build's promised writable capacity. Reservation rounds files to allocation
pages and includes per-entry metadata rather than trusting logical byte length.
Pull synchronization marks older previews stale without replacing their URL or
evidence. Each definition fails closed with network `none`, artifact-only data,
named identity, and explicit view, test, or feedback actions. A repository owner
can invite a named user or expand current issue, decision, or proposal
participants into expiring roles. Invitation, first-entry, and revocation are
audited. Guests receive only authenticated sandboxed static assets: no logs,
source, credentials, workspaces, environments, deployments, private services,
forms, outbound connections, or production authority.
Feedback-role participants can turn an observation into an attributable
finding pinned to the preview revision and current route. Findings support
classification, severity, reproduction steps, discussion, duplicate links,
and version-guarded resolution or reopening. Policy-permitted screenshots,
recordings, console output, traces, and annotations remain readable only
through the preview audience boundary; bounded media types and sizes are
validated and secret-like text fields are redacted before retention. The pull
workspace projects this shared trail without copying inaccessible evidence into
ordinary comments.
The exact `/pulls/{repository-id}/{pull-id}/previews/{preview-id}` feedback
workspace admits a non-repository guest through the active invitation and
renders controls from its effective feedback role; it does not require or
project repository participation.
Repository owners can layer target-branch preview acceptance policy over this
evidence. Requirements select changed paths or owner-authored risk classes and
name promised scenarios plus the owner, contributor, pull-author, or invited
stakeholder role that
must evaluate them. Acceptance, rejection, and owner-only justified override
records are append-only and pinned to the adopted source commit and policy
version. Pull readiness
shows current and stale decisions separately, blocks on missing current
blocking scenarios or unresolved blocking preview findings, and reuses that
same decision for merge and integration-queue admission. Synchronizing a newer
commit therefore preserves the earlier evaluation as history while requiring
the affected behavior to be evaluated again. Stakeholders can decide only
while they hold a live feedback invitation to a preview of the exact adopted
revision; expiry, revocation, or synchronization removes that authority without
adding repository access.
Decision publication uses a stable caller idempotency key and synced atomic
storage, making an ambiguous retry converge on the original evidence. A
rejection remains a veto until an owner records a reasoned override. Queue
landing revalidates current readiness and pauses retained admitted work when
policy, decisions, findings, reviews, checks, or branch state changed.
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
approved agents. Organization owners publish append-only agent profile revisions through `PUT
/organizations/{organization_id}/agents/{agent_id}/profile`. Profiles disclose supported tasks and tools,
model and execution provenance, project-data use and retention, subprocessors and remote boundaries,
pricing/resources, requested capabilities, availability, support, and a change summary. Operator claims
remain visibly separate from platform-generated evidence for the stable `agent:{id}` principal and its
current organization operators. Public agents expose this history in the organization directory; a
profile never grants a capability, credential, installation, user identity, or repository authority.
Organization members can compare agents against a bounded task, proposal, issue, decision, incident,
stewardship mandate, or team-role reference through `POST
/organizations/{organization_id}/agent-matches`. The deterministic projection explains workflow fit,
live independent grants, effective policy, disclosed cost and availability, deployment boundaries,
profile freshness, conflicts, verified evaluations, and attributed outcomes on comparable work.
Missing or stale evidence remains visible; ordering grants no authority and copies no source content.
Private matching evidence is excluded from the broader public directory.
Organization owners can turn one explicitly approved evaluation run into a versioned participation
proposal through `/organizations/{organization_id}/agent-participations`. The proposal freezes the
agent profile and trial decision together with selected roles, resources, permitted actions,
cost/action/time budgets, schedule, data boundaries, referenced policy exceptions, and either a
current operator agreement or named human sponsor. Its `/preview` is always non-effective and reports
schedule, agreement, and live agent-policy blockers. Activation issues the stable
`agent-participation:{id}` attribution identity and links it to an ordinary organization access grant,
the only source of technical authority used by established collaboration workflows. Denials,
concurrent versions, trial evidence, agreements, expiry, activation, and revocation remain retained;
revocation retires the linked grant and derived credentials. Evaluation approval, budgets, policy
exceptions, and governance or sponsorship standing never create access on their own.
The linked request and grant retain the approved participation limits. Derived API/Git credentials
require an explicit `repository.read` or `repository.write` action and a compatible
`repository_metadata` or `repository_content` data boundary; write is never inferred from the role.
Credential derivations consume the action ceiling, and each credential lifetime is capped by the
agent-minute budget. Capacity is reserved inside the serialized grant mutation, so concurrent
derivations cannot exceed the ceiling. Cost remains a reporting ceiling and cannot expand technical
scopes. If a pending proposal's sponsor leaves, an owner can compare-and-swap a replacement current
member through its `/sponsor` endpoint; both identities remain in the participation event history.
Sponsor membership is held across the participation write, excluding concurrent member removal.
If activation loses its final participation compare-and-swap after provisional grant approval, rollback
revokes the grant and every credential derived during that interval.
Deployment matching compares exact closed profile values (`platform`, `operator_managed`,
`customer_managed`, or `external_service`); explanatory execution prose cannot imply compatibility.
An omitted structured disclosure stays visible as missing evidence and fails closed for an explicit
boundary search; only a non-empty incompatible disclosure is reported as a conflict.
The portfolio derives blockers, relevant policy exceptions,
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
The credential request may set `purpose` to `api_read` for repository-bound
`repositories:read` agent work such as evidence synthesis; otherwise it mints
the established Git credential appropriate to the grant role.

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

## Evidence-driven technical decisions

Technical choices can be opened before implementation from repository,
proposal, investigation, incident, evolution-plan, or stewardship-opportunity
context. The `/decisions` workspace makes the pending question, constraints,
success measures, deadline, affected resources, participants, and accountable
owner visible to current repository collaborators. Version-guarded scope
revisions and discussion share an attributable retained history. Related work
can discover the pending record by its source identity, but the record is
coordination context rather than a contribution gate.

Participants propose explicit alternatives rather than burying preferences in
discussion. Each alternative separates assumptions, tradeoffs, risks,
compatibility impact, cost, and expected outcomes, evaluates every shared
success measure, and cites exact code lines, dependency records, releases,
incidents, or usage windows. The comparison reports absent evidence classes
and citations older than 30 days. A participant can issue a short-lived
credential bound to one repository, decision, and selected alternative. It
carries `decisions:research` plus repository-bound read access: an agent can
inspect the selected context and append cited support, opposition, or neutral
findings and explicit uncertainty, but cannot mutate the repository. Findings may supersede earlier claims without
deleting the dissent or evidence that informed them.

An alternative can be tried in a bounded shared development workspace at one
exact commit. The repository owns the permitted experiment entry points as
named `experiments` in `.vivarium/workspace.json`; the ordinary workspace
resource, network, sharing, agent-control, and lifetime policy still applies.
The decision links the resulting attributed command logs and checkpoints plus
measurements, checksummed artifact metadata, diffs, and server-derived CPU,
memory, and storage use. Participants can reopen the same permitted workspace
to reproduce the run. The comparison flags evidence when the default branch,
workspace definition, or effective workspace policy has changed. Attaching an
experiment never publishes its checkpoint, opens a pull request, or merges
code; those remain explicit, separate governed workspace actions.
The launch baseline records what default-branch code and environment were
current at that moment independently of the selected revision, preserving
deliberate historical comparisons. Workspace creation and decision linking are
retry-safe: identical launches reuse one running workspace and an exact link
retry returns the retained experiment.

The accountable owner can request an acknowledgement from the current owner of
an affected repository or an approval tied to an applicable active organization
policy rule. Requests, approvals, rejections, and unresolved conflicts are
public to current decision readers. Publication freezes a numbered commitment:
the selected alternative, every rejected alternative, rationale, accepted
tradeoffs, dissent, conditions, review date, and the exact retained evidence
considered. It also snapshots the approval trail and any approved, expiring
policy exception, so downstream work can explain the temporary deviation.
Every current non-superseded opposing finding is retained exactly once. An
exception can use only the purpose approved by the policy owner, cannot outlive
the approval's frozen maximum expiry, and cannot reuse one approval twice.
Material scope, alternative, finding, or experiment changes reopen the decision,
supersede the old approvals, and retain the earlier commitment unchanged;
discussion can clarify a published choice without silently invalidating it.

The connected decision journey proves this is one collaboration loop rather
than a set of disconnected records. Two repository owners open and govern a
shared uncertainty, compare agent-researched alternatives with retained
dissent, and reproduce the selected prototype in separate exact-revision
workspaces. The accepted commitment becomes ordinary human- and agent-authored
tasks, checks, independent review, merges, and a release. A linked production
measure that disproves the selected assumption reopens the decision while its
alternatives, experiments, approval, dissent, implementation identities,
release evidence, permissions, and actor attribution remain inspectable.

## Delivery-team charters

Temporary teams make the operating contract around a planned outcome durable
before work begins. A repository collaborator can bind a charter to an existing
proposal, organization initiative, technical decision, incident follow-up, or
an explicitly named planned outcome. The charter identifies the organizer and
records each human or approved agent's complementary role, responsibility,
reason for involvement, budget, deadline, escalation route, and exact required
repository access.

Invitations start pending. Humans respond for themselves; an approved agent's
current organization operator responds for that agent. Responses and
organizer-only charter revisions use versions and append actor-stamped history,
so acceptance, replacement, removal, and changed boundaries cannot be silently
rewritten. Reads recompute whether each required access level is satisfied by
independent live repository participation or an organization grant. Team
membership itself creates no credential, collaborator relationship, Git scope,
review right, or merge authority; an unmet requirement stays visible as an
access gap. Durable records use `$DELIVERY_TEAM_STORAGE_ROOT`, defaulting to
`delivery-teams`.

Accepted team members collaboratively publish a separate versioned execution
plan inside the same record. Its ordered work streams freeze repository paths
and inputs to exact revisions and name an owner, artifacts, dependencies,
acceptance criteria, assumptions, budget, and integration position. The server
rejects malformed or cyclic graphs and projects overlap, duplicate artifact,
budget, and live access blockers on every read. Because a plan grants no
authority, an owner without independent repository write access remains
blocked. Material revisions require each affected human owner or approved
agent operator to accept the new boundary with team and plan versions.

An accepted stream owner can then bind the stream to an ordinary change
session, collaborative investigation, decision experiment, or workspace at an
exact repository revision. The team timeline retains structured findings,
questions, checkpoints, artifacts, decisions, and uncertainty as summaries
with citations rather than copying the source system's contents. Every read
rechecks the viewer's independent repository access and removes an inaccessible
timeline entry, context binding, or dependent handoff in full.

Handoff requests freeze the producing stream and plan revision, sender and
recipient participant slots, exact timeline inputs and their citations,
acceptance criteria, and residual uncertainty. Handoff is coordination, not
authority or reassignment: the execution plan must explicitly move ownership
before the recipient can publish a verification entry. Only that recipient (or
an approved agent's current operator) accepts, citing their own retained
verification and explaining what they checked. Team records never ingest
terminal input, credentials, hidden prompts, or evidence bodies from the
attached work system.

The same delivery-team workspace projects each parallel stream's live state,
progress, exact revision, resource consumption, active human or agent control,
blockers, questions, and predicted next action. Owners publish bounded status
snapshots; budget exhaustion and current access loss pause affected work with
an explicit recovery route. Accepted collaborators can guide or control an
individual stream, while whole-effort control and authority-changing reassign
or narrow operations remain organizer-only. Reassignment and narrowing create
fresh plan revisions and owner acceptance, preserving accepted artifacts,
timeline evidence, handoffs, and intervention history instead of moving work
or authority implicitly.

Completed parallel work returns through a durable integration manifest rather
than an unstructured bundle. Each stream contributes an exact live branch tip
or an already-published workspace checkpoint descended from the shared base.
The server derives changed paths and checkpoint authors/command evidence,
retains decisions, costs, and residual risks, and reports path overlaps,
incomplete streams, pending handoffs, planning blockers, and acceptance
criteria lacking same-stream evidence. A blocked manifest cannot publish.

A ready manifest opens one retry-safe ordinary pull per contribution in the
declared integration order. Pulls retain team, manifest, stream, and order
links. Every branch and repository permission is revalidated at publication;
required reviews, owner acknowledgements, checks, queues, release policy, and
stale-revision behavior remain authoritative. Neither team membership nor the
manifest grants merge authority. Publication starts each pull's ordinary
exact-revision repository checks only after the complete ordered pull set is
durably linked to the integration. A later pull or final manifest-write failure
starts no partial checks; exact retry first reconciles any existing pulls.
The same publication endpoint accepts a current-version or lost-response
pre-publication-version retry after the integration is published. It follows
the retained pull links and idempotently creates only missing check definitions
or resumes nonterminal runs, recovering a process stop or transient check-store
failure without duplicating completed evidence.
Repository access, configuration reads, and check-run persistence are part of
that recovery outcome. Any failure returns a retryable `503`; a successful
response means every linked pull's required check records were reconciled.

The connected delivery-team browser journey carries an accepted technical
decision through a chartered developer and two independently operated
specialist agents. Parallel execution retains a disputed finding and human
resolution, a failed stream and bounded redirect, resource costs, and an
agent-to-human ownership handoff with explicit verification. The resulting
ordered pulls pass repository checks, independent review, merge, and release.
Removing one agent operator revokes the derived Git credential while retaining
the team's legitimate charter, plan, evidence, interventions, authorship,
handoff, integration, and delivered outcome.

## Unexpected behavior stays with the repository

Repository participants can open a structured issue against current work or an
exact retained release. Reports keep reporter, visibility, severity,
environment, expected and observed behavior, ordered reproduction, bounded
allowlisted evidence, discussion, status, and actor-stamped history together.
Built-in templates make the minimum diagnostic context explicit, while
duplicate suggestions search only records the current participant can read and
never project candidate attachments or discussion.

The `/issues` workspace supports report creation and repository discovery; the
repository issue detail route keeps downloadable permitted evidence,
maintainer status changes, history, and discussion in one collaboration
surface. Issue visibility does not grant repository access, and attachment
limits keep this diagnostic path distinct from an arbitrary file store.

An issue becomes executable evidence through an `issue_reproduction` workspace
pinned to exact source or its attested release. Selected sanitized attachments
are staged without credentials, and only outcomes matching repository-declared
reproduction command hashes can enter an immutable attempt. Attempts retain the
environment, inputs, logs, exit codes, artifacts, observed result, revision,
and actor; failed and inconclusive runs remain available for collaborator reruns.

Triage then turns that shared reproduction into an explicit, attributable
working theory. Maintainers compare-and-swap classification, priority,
assignment, suspected exact revision and owners, and duplicate identity; they
can connect code, dependencies, releases, deployments, incidents, proposals,
pulls, and other issues without replacing those authoritative records. Missing
evidence is requested directly from the reporter and the answer stays beside
the request.

Human hypotheses, findings, and uncertainty must cite retained reproduction or
linked evidence. Challenges and superseding claims preserve disagreement rather
than turning a label or agent result into hidden truth. A maintainer can issue a
short-lived `issues:investigate` credential over one selected reproduction and
a bounded link set; the read-only agent can inspect only that packet and append
cited claims under its generated identity. Current repository participation is
revalidated on every ordinary issue read, while duplicate, ownership, evidence,
and access decisions remain actor-stamped in the issue history.

Once an attempt confirms the failure, a collaborator can select that exact
attempt, its affected commit, undisputed cited diagnostic findings, and explicit
acceptance criteria to create one governed implementation proposal and owned
task. Human owners must already participate and receive no new access; generated
agents receive only the existing assignment-scoped branch authority when their
task session starts. The issue retains the frozen evidence handoff and projects
the task and linked pull's live state, while commits, checks, discussion, review,
queueing, and merge continue through ordinary pull-request policy. Retrying the
same reproduction handoff converges on the existing proposal rather than
creating duplicate repair work.
If proposal publication wins but the issue update cannot be persisted, the
response is `202` with the stable task/proposal identities and
`Vivarium-Recovery-Implementation: pending`; replaying the exact request
completes the issue link without creating another repair plan.
Recovery requires the complete frozen reasoning origin to match, including the
issue version, affected revision, selected evidence, and finding snapshots;
changed evidence returns a conflict instead of attaching it to the older task.
For the resulting exact pull revision, collaborators can reserve issue-specific
resolution proof that replays the retained reproduction inputs and environment
beside every required check. The reporter sees the criteria and per-run evidence
and can confirm or reject the result. A later pull commit makes that confirmation
stale; owners may append a reasoned override, but reporter dissent remains.
A currently shared, running workspace at that same pull revision is projected
as the optional safe-preview handoff; no verification run publishes a port.
After the reporter confirms current passing proof, a maintainer can record the
delivered outcome only from authoritative release and deployment evidence. The
selected release commit must contain the exact verified repair revision, and
the selected promotion must have succeeded for that same release commit. The
resolved issue freezes the release version and commit, deployment environment,
artifact checksum, reporter decision, and recording actor, preserving a direct
trail from the original observation to what reached users. Replaying that exact
delivery is idempotent; a different delivery claim conflicts.

A run that does not reproduce is evidence rather than closure. Maintainers can
request the missing condition, the reporter answers on the issue, and a later
workspace attempt retains the corrected evidence and outcome without replacing
the earlier non-reproduction.
## Outcome funding

Outcome funding turns verified backing into an inspectable, versioned promise without turning
money into project authority. A current participant links one governed fund to an exact issue,
roadmap outcome, proposal, stewardship opportunity, incident follow-up, or security repair and
declares scope, acceptance criteria, evidence requirements, budget, deadline, contributor
eligibility, allocation and cancellation rules, dependencies, risks, conflicts, and optional
budget-balanced milestones. Backers can support the whole result or one milestone. Scope changes
require explicit pledge reconfirmation, while insufficient or aggregate shared-fund unsettled
backing, overlapping awards, embargoes, withdrawals, and cancellation remain visible as attributable replanning.
The repository workspace is `/repositories/{id}/funding`; funding never grants contribution,
acceptance, Git, review, merge, deployment, credential, or security authority.

That workspace also separates contributor choice from privilege. Eligible humans, organization
teams, and approved-agent operators can offer an approach, milestones, cost, dependencies,
availability, requested access, and attributed prior work. Recipients explicitly accept their
offer before a named fund steward can compare one or a complementary mix, retain a conflict
disclosure and rationale, reserve only cryptographically settled available value, and create
connected planned milestone tasks. Selection and compensation remain evidence—not repository,
secret, review, merge, environment, deployment, withdrawal, or agent authority.

Once selected, contributors can make the promise inspectable before the reservation is exhausted.
They report milestone progress and forecasts with exact task, session, workspace, fork, pull,
check, preview, delivery-team, release, and deployment evidence; agent compute and blockers remain
attributed beside the work. Evidence-backed expenses wait for a current fund steward, then move
only the approved amount from reserved to spent. The same workspace exposes pending spend,
remaining reservation, forecast completion, and failed handoffs. Stewards may pause or resume,
record revoked access, approve a policy-bounded budget increase, replace the recipient for
unfinished work after live eligibility checks, or cancel and return the unspent reservation.
Overrun, inactivity, failed handoff, revoked access, pause, and cancellation stop new spending but
retain prior evidence and legitimate contributions; funding controls never inherit or expand the
authority of the linked collaboration resources.
The initial inactivity deadline begins at selection, paused work can still report retained
progress and handoff evidence, and resume alone cannot clear an access-loss block. Approved
expense accounting uses a durable roll-forward journal so interrupted fund/outcome publication
is recovered under the same mutation boundary.
Recipient replacement must select a distinct principal, so reassignment to the revoked identity
cannot masquerade as access restoration.
Revoked principals cannot mutate retained completed-task history after unfinished work is assigned
to a distinct, live-validated replacement.

Every selected milestone also freezes a deterministic share of the original recipient allocation
and names current project participants as compensation reviewers. Reviewers inspect the latest
completed update's authorship, commits, handoffs, checks, previews, releases, deployments, and
recipient-declared outcome measures before accepting it, requesting correction, rejecting it,
approving a bounded partial award, or opening a dispute. Decisions retain rationale and dissent;
accepted and partial results atomically move only that milestone's allocation from reserved to
spent, and a partial award releases its unawarded remainder. Rejection and correction remain
appealable, while deadline timeout, recipient withdrawal, payment failure and retry, and policy
refunds have explicit attributed ledger transitions. These decisions recognize compensation
evidence only: they never merge a pull, publish a release, deploy an artifact, mint access, or
replace the authority of any linked project resource.

Each paid review is displayed as a settlement receipt. Its stable ID is also the suffix of the
fund's `milestone_award` ledger reference, connecting recipient credit and reviewer attribution to
the frozen update, delivery chain, and outcome measures. Reassigning unfinished agent or human
work preserves already completed, accepted, payment-failed, withdrawn, timed-out, or refunded milestone ownership.
Delivery updates cannot reopen any of those terminal states.
The connected browser journey proves scope reconfirmation, mixed developer/approved-agent
selection, overrun containment, replacement, dispute, rejection, appeal, award, and refund through
the public contract while the ordinary work links retain independent authority.

# Trusted external extensions

Developers can register independently operated tools at `/extensions` before a
project shares any context. A registration records the human owner and operator
contact, declared capabilities and supported events, callback/action endpoints,
requested resource permissions, and credential rotation policy. Both endpoints
must answer a live ownership challenge. The platform gives the integration a
stable `extension` principal distinct from users and approved agents, while the
effective-authority preview remains empty until a resource-owner
installation. No extension credential is issued during registration, so the
record is an inspectable contract rather than an implicit delegation of the
installer's access.

Repository and organization owners install that identity for an explicit
repository set and selected requested resource types. Each record approves or
denies every declared capability, permits only non-secret key/value settings,
and projects resulting actions. Create, upgrade, suspend, resume, transfer, and
removal are version-guarded and retain actor history. Current ownership is
revalidated; suspension and removal clear only credentials derived from that
installation, leaving unrelated installations and attribution intact.

Ongoing operations project attributed requests and actions, delivery health and
latency, consumption, permission use, credential health, configuration history,
and actionable notices. Reverified contract updates never change an installation
until renewed owner consent. Owners can rotate, narrow, pause, quarantine, or
uninstall without erasing prior attribution.

Active installations receive meaningful repository, pull request, check,
release, deployment, incident, issue, and proposal-task changes from the
durable collaboration activity ledger. Delivery uses an installation-scoped
v1 JSON envelope containing stable event/delivery IDs, a monotonic sequence,
repository ordering key, exact resource and actor identifiers, and occurrence
time. The exact envelope bytes are SHA-256 identified and Ed25519 signed; the
installation delivery API publishes the verification key and never projects a
resource lacking both repository scope and effective read access. Duplicate
source events are idempotent, unsupported event kinds are ignored, suspended
or removed installations receive nothing, and delayed events retain their
original occurrence time.

Owners inspect redacted payloads and attempt history beneath
`/extension-installations/{id}/deliveries`, record outcomes, retry the same
delivery, or replay its event as a new ordered delivery. Five failed attempts
move a delivery to the visible dead letter state; replay preserves the source
event identity while assigning a fresh delivery ID and sequence. Consumers
reject an unknown newer `schema_version` without acknowledging the delivery.

The repository performance workspace turns retained trials into collaborative diagnosis before
optimization begins. A diagnosis selects exact evidence and revision-aware symbols, dependencies,
commits, releases, and runtime paths; invites relevant owners; and retains cited hypotheses,
comparisons, uncertainty, conclusions, flame stacks, challenges, and confirmations. Findings are
visibly stale after same-context evidence changes revision, workload, or environment. Read-only
agent access is short-lived and bound to one packet, preventing unrelated restricted evidence from
propagating through its citations.

Supported performance diagnoses carry into ordinary pull review through exact-revision
optimization evaluations. Each compares a candidate trial to an investigation-selected compatible
baseline and presents server-derived confidence, timing/resource/cost deltas, correctness commands
and outcomes, affected scenarios, authorship, and residual risks. Advancing the pull retains the
earlier evaluation as stale.

Local extension development may set `EXTENSION_DEVELOPMENT_ENDPOINTS=1` on the
API process to verify sample services over HTTP at `localhost`, `127.0.0.1`, or
`::1`. The exception is deliberately limited to loopback and is disabled by
default; every other endpoint remains HTTPS-only and must resolve exclusively
to publicly routable addresses.

The connected extension journey registers a live sample service, installs it
for one repository, verifies a signed pull event, publishes an exact-revision
check with annotations, artifact, and a declared repair action, and takes the
updated Git change through ordinary review and merge. It also retains a failed
delivery and replay, requires an explicit installation upgrade after a new
capability is declared, and proves uninstall revokes the derived credential
without deleting the contribution, invocation, delivery, or installation
history. Playwright isolates extension records beneath
`$EXTENSION_STORAGE_ROOT` with its other temporary API stores.
## Evidence-backed product opportunities

Repository readers can inspect `/repositories/{id}/opportunities` to see recurring needs as an
auditable interpretation rather than a popularity score. Each version states its affected audiences,
severity, reach, confidence, expected value, uncertainty, minority needs, contradictions, and exact
source citations. Citations identify whether evidence supports, contradicts, represents a minority
need, or duplicates another signal; moved and inaccessible sources are visibly stale.

Project participants may publish new versions and attributable classification corrections. Any
authorized reader may challenge a synthesis, and a feedback reporter may detach their feedback
citation where the feedback policy permits, while the original source and earlier synthesis versions
remain unchanged. Read-only repository agents may create cited syntheses, but cannot revise or
correct them.

## Contextual developer support

The `/support` workspace keeps a developer's question beside the exact project
target, version, environment, goal, attempted steps, urgency, audience, contact
preferences, and permitted logs, configuration, or sample code. A question can
target a repository, package, release, API, documented journey, or error.
Server-derived diagnostics call out missing version, environment, goal, and
attempts instead of making maintainers infer them.

Public threads are available to authenticated readers of public repositories;
maintainer-audience threads remain limited to the author and current repository
participants. Contact email is never projected to an ordinary public reader.
Related answered threads and issues are ranked only from readable records and
return identity/title/status metadata, not candidate attachments or private
discussion. Status changes are compare-and-swap and retain actor-stamped
history. Records default beneath `$SUPPORT_THREAD_STORAGE_ROOT`
(`support-threads`) and grant no repository authority.
