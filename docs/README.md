# Docs

Notes on how this project fits together. Mostly empty for now — things get
written down here as they're decided, not before.

## Sensitive history remediation

Current repository maintainers open restricted history-repair coordination through `POST
/repositories/{id}/history-remediations` or the repository's Sensitive history remediation page. A
caller-stable request freezes its security-finding, privacy-incident, support-case, or selected-object source;
a short payload-free description and reason; and exact repository objects plus optional revisions, refs,
releases, packages, artifact digests, and environments. The creator must currently maintain every affected
repository, and supplied commit revisions must resolve before publication.
Scope kinds are closed and authoritative: Git objects and refs resolve from stock Git, releases and
environments resolve from their repository stores, packages bind their published artifact ID and SHA-256, and
check artifacts bind `pull/run/artifact` identity plus digest. Repository-bound credentials cannot name another
repository. Exact retry reconciliation happens before these mutable checks, while new publication holds current
source participants and affected-repository maintainer access stable through persistence.
Reconciled POST responses use the same retained audience check as GET projections. Git refs are additionally
bound to the exact claimed object and optional revision, rather than accepted as an unrelated existing ref.

Discovery evidence retains only its source identity and SHA-256 digest, classification, safe note, and human
attribution. Matches, false matches, inaccessible resources, legal holds, and conflicting retention or
continuity commitments remain separate visible facts instead of being silently discarded. The API rejects
multiline or unbounded content descriptions so the remediation workspace does not become another copy of the
unsafe payload.
Whole-record screening rejects both labelled credentials and bare JWT-shaped values in root descriptions,
evidence notes, constraint reasons, and every other persisted string field.

Every record names a restricted disclosure audience, response owners, and approval roles with explicit
thresholds. Those principals must be current participants in the source repository; list and detail reads
return the record only to the creator, audience, owners, or approvers. Files are stored with owner-only
permissions beneath `$HISTORY_REMEDIATION_STORAGE_ROOT`. The ledger agrees on what must disappear and who may
coordinate, but grants no inspection, Git, object deletion, ref rewrite, package, artifact, release,
environment, disclosure, or delivery authority.

Authorized remediation participants and repository-bound read-only agents can append cited exposure findings
through `POST /repositories/{id}/history-remediations/{remediation_id}/exposure-findings`. Each compare-and-swap
entry maps one or more already-scoped affected object IDs, plus closed derived categories (`credential`,
`personal_data`, `confidential_data`, or `generated_artifact`), to a branch, tag, pull request, fork, federated
contribution, workspace, checkpoint, cache, package, release artifact, documentation location, deployment,
backup, or active clone. Entries distinguish `confirmed`, `suspected`, `unreachable`,
`independently_controlled`, and `unverifiable`, with a SHA-256 citation, bounded payload-free analysis,
uncertainty, actor, and time. Caller request identities make append retries stable, while the remediation
version prevents lost concurrent discoveries.

The web/API projection keeps newly found propagation visible immediately. A finding marked restricted retains
its classification, affected object IDs, derived-data categories, attribution, and the fact of uncertainty,
but replaces its copy and citation identities and omits caller prose. This permits teams to track an
independently controlled clone or backup without turning the remediation ledger into an access bypass or a new
copy of restricted contents. Exposure-map records grant no inspection, clone, cache, package, deployment,
backup, federation, Git, or remediation authority.

Response owners plan replacement history through `POST
/repositories/{id}/history-remediations/{remediation_id}/rewrite-candidates`. Each immutable rule removes a
scoped blob or substitutes another existing blob, and each selected ref is bound to its exact current commit.
The server discards caller-supplied mapping and impact fields, walks the full selected commit graph, writes replacement trees and commits as unreachable objects,
and never updates a ref. Unaffected objects and commit authorship remain byte-identical; a changed signed commit
is rebuilt without its now-invalid signature and is called out explicitly. The restricted candidate projection
retains the complete old/new commit and object maps, candidate tips, broken signatures and commit links, object
storage delta, independently controlled or unreachable copies, rollback limits, and required collaborator work.

Owners exercise a candidate through its `/rehearsals` route. A rehearsal must cover repository integrity, build,
ordinary checks, release production, dependency consumers, and representative clone and fetch behavior. Commands
run for every selected replacement tip from an exact detached candidate checkout with bounded time and output;
caller commands execute only in a named preinstalled, networkless, read-only, capability-free container, while
integrity and local transport scenarios are server-defined. Missing revision-appropriate commands stay explicit as unsupported evidence, any non-pass keeps
the rehearsal failed, and all evidence remains inside the remediation audience. Candidate assembly and rehearsal
grant no ref update, object deletion, release, package, collaborator-machine, or publication authority.

Publication is a separate `POST .../rewrite-candidates/{candidate_id}/publish` milestone available only to a
named human response owner after a complete passing rehearsal and every retained approval-role threshold is
represented by an eligible approver. The server derives approval time and quarantined scope, durably reserves a
`publishing` intent that activates the stock-Git push pause, then uses one Git `update-ref` transaction to
compare-and-swap every selected old tip to its attested candidate tip before finalizing the intent. One moved
ref aborts all updates; an exact retry reconciles either an active intent or already-new tips. Only the enforced
`pushes` pause is accepted; queue, session, workflow, release, and credential claims fail closed until their
authoritative stores expose containment adapters. The retained migration record freezes audience-appropriate
instructions for local branches, forks, federated copies, pull requests, and integrations.
Ordinary receive-pack discovery and pushes fail with actionable fetch/backup/rebase-or-reset guidance during
migration. Independently controlled targets project as `awaiting_owner`: the coordinator can explain and track
the necessary rewrite but gains no authority to perform it.

After publication, owners append immutable current-evidence passes through `POST .../containment-passes`.
The completion policy covers repository reachability, ordinary object access, fork and federation
acknowledgements, package and artifact replacement, credential rotation, deployment use, caches, and protected
recovery copies. The server discards caller claims for Git reachability and quarantine and derives those facts
from the authoritative repository and publication. A quarantined Git object passes only when it is unreachable
from every advertised ref, while upload-pack disables direct and reachable SHA wants. Other dimensions require bounded SHA-256 evidence. Failed,
unreachable, independently controlled, legally retained, reintroduced, and exceptional copies remain explicit;
exceptions expire within 30 days, and no pass claims erasure. At publication each pull/workspace target selects
one candidate ref and receives its replacement revision from the server. Owners can retain migration or
supersession with discussion and attribution preserved only for that exact resource/ref/revision tuple; another
target's otherwise valid replacement cannot be cross-mapped. `POST .../restorations` reopens only named push, automation, release, and contribution flows, and only
against the latest complete passing pass; a remaining push pause is enforced independently of display state.

The connected `history-remediation-journey.spec.ts` exercises this boundary through the browser, public API,
and stock Git. It retains a false match and unavailable/protected copies, maps branch, fork, pull, package,
release-artifact, and deployment exposure without copying the payload, records a failed rehearsal before the
passing candidate, publishes after attributed approval, rejects a stale clone with migration guidance, leaves
an independent fork owner to rewrite their own repository, and restores collaboration only after a fresh
ten-part passing containment check. The final state remains `contained_with_residuals` and never becomes an
erasure claim.

## Propagation campaigns

An authorized repository collaborator opens a durable shared delivery boundary through `POST
/repositories/{id}/propagation-campaigns` or the repository's Propagation campaigns page. The
campaign freezes its source kind and resource, every exact source commit, the intended proven
outcome, acceptance criteria, repository release lines and package lines, accountable target
owners, deadlines, target dependencies, and an all-target, minimum-target, or ordered completion
policy. A caller-stable `request_id` reconciles ambiguous create retries and rejects changed reuse.

List and detail reads project each target against current repository facts. Missing repositories and
release lines, unsupported package equivalence, revoked collaborator access, invalid current target
ownership, and a target whose tip already has the source outcome's exact Git tree remain distinct
states. This lets collaborators retain targets they cannot currently resolve instead of silently
dropping them. The retained authority statement is deliberately narrow: a campaign agrees what must
travel and in what order, but grants no branch, Git, package, review, merge, release, deployment, or
cross-repository authority.

Before implementation, a collaborator with access to a repository target can publish a target assessment at
`POST /repositories/{id}/propagation-campaigns/{campaign_id}/targets/{target_id}/assessments`. The server freezes
the source parent and outcome commits plus the target release-line tip, then compares exact changed blobs and
history, declared symbols, dependency manifests, interface and schema files, prior target commits touching the
change, and the campaign's release commitments. The result is one of `directly_applicable`, `already_satisfied`,
`adaptation_required`, `conflicting`, or `not_applicable`; matching names and similar commit messages are retained
only as leads and never promoted to behavioral equivalence. Repeating the same exact comparison reconciles to its
existing identity.

Humans and repository-bound read-only agents append CAS-versioned cited findings, risks, and uncertainty at the
assessment entry endpoint. Only a named human target owner can append an owner acknowledgement. Citations bind to
the frozen source/base/target revisions, and movement of one target release line projects only that target's
assessment as stale while preserving the campaign and every unaffected assessment. Assessment records grant no
Git, package, branch, implementation, review, merge, release, deployment, or target-repository authority.

A current human target-repository maintainer can turn one non-stale, implementable assessment into an ordinary
locally owned contribution plan at `POST
/repositories/{id}/propagation-campaigns/{campaign_id}/targets/{target_id}/contributions`. The handoff freezes the
assessment version and target tip, direct or adapted application, explained deviations, local constraints, chosen
local-branch, contributor-fork, or federated topology, and ordered human- or agent-owned tasks. Its ordinary proposal
and task reasoning carries the accepted source intent, relevant commits, original and target acceptance criteria,
cited assessment findings, and the local verification plan. Direct work explicitly preserves source Git authorship;
adapted work must explain why the target cannot appropriately take the same patch.

Task agents start through the existing assignment-bound session route, which creates a scoped branch and observable
workspace at the frozen target revision. Humans may use an ordinary local branch or fork, while federated repositories
continue through federation's own contribution boundary. Completed work becomes an ordinary task-linked pull request
and remains subject to target review and merge policy. Unavailable dependencies remain constraints, conflicts require
adaptation, and restricted or embargoed details stay outside the handoff. Contribution links project only to readers
who retain target access. Neither campaign ownership nor publication grants Git, fork, session, workspace, pull,
review, merge, package, release, federation, or cross-repository authority.

After implementations exist, collaborators demonstrate behavior through `POST
/repositories/{id}/propagation-campaigns/{campaign_id}/targets/{target_id}/equivalence-proofs`. The server derives the
reusable scenario set from the source outcome's exact `.vivarium/checks.json` and the evidence requirements from campaign
acceptance criteria. Every source scenario must map either to an exact adapted command using a target-defined bounded check
environment or to explicit revision-bound substitute evidence when the target cannot support that test. It also executes
every ordinary target check at the same exact target revision.

The resulting matrix retains source and target commands, criteria coverage, terminal state, bounded logs,
digest-addressed artifacts, compute cost, residual differences, and CAS-versioned decisions from named human target
owners. Failed ordinary checks cannot demonstrate equivalence, and accepting substitute evidence does not erase a
declared residual difference. Read projection rechecks the target release line, dependency manifests, and exact source
scenario/acceptance digest so movement invalidates only the affected proof while its historical execution stays intact.
The proof records provenance only and grant no check execution, Git, review, merge, release, deployment, or target authority.

Once a current proof is accepted, a named human target owner may bind it to the exact ordinary task-linked pull through
`POST /repositories/{id}/propagation-campaigns/{campaign_id}/targets/{target_id}/delivery-paths`. The owner names the
supported-user groups served by that path, but the campaign does not queue, merge, release, or deploy anything. Reads
instead project current target review decisions and integration-queue state, the release that includes the pull, its
ordinary deployment rollout and health signals, exposed users, path-local blockers, and the next decision from the pull,
release, and deployment stores. A failed or paused rollout therefore pauses only its target path.

Human coordinators can retain a newly discovered consumer, while named target owners can retain a superseded target or
an exception expiring within 30 days, at the campaign `scope-events` route. Each event requires a reason and follow-up.
Coverage is always derived: it distinguishes partial adoption, completion, and a completion policy whose threshold is met
while visible gaps remain. Rejections, unresolved discoveries, superseded paths, and live exceptions stay attributable
blockers, so minimum-target policy cannot turn them into hidden delivery. These records grant no review, queue, merge,
release, deployment, environment, repository, or target authority.

The connected propagation journey exercises this boundary through the browser, public API, and stock Git: a verified
regression correction is adapted by an independent v2 maintainer, split into human implementation and agent audit work,
fails its first bounded equivalence attempt, passes the corrected matrix and ordinary checks, receives owner acceptance,
and becomes an ordinary task pull. The same trail keeps an unsupported repository, a rejected non-owner delivery action,
a newly discovered consumer, and a seven-day upstream-review exception visible rather than allowing minimum coverage to
hide them. Playwright gives the propagation ledger its own temporary storage root so repeated journeys do not share state.

## Repository restructuring planning

Repository collaborators open topology proposals through `POST
/repositories/{id}/restructuring-plans` or the repository's Restructuring plans workspace. A
caller-stable request freezes one or more exact source commits, proposed destination names and
owners, visibility and default branches, identities that must remain recognizable, path and
history dispositions, a deadline, success criteria, and explicit rollback limits. Source commits
must resolve in Git, and repository-bound credentials cannot expand their authority to another
source.

The plan accounts for all twelve collaboration and delivery boundaries: refs, pull requests,
issues, tasks, releases, packages, documentation, policies, workspaces, automation, consumers,
and federated relationships. Each entry names an owner, exact source revision, proposed move,
remaining or divided destination, citation, and a resolved, inaccessible, ambiguous, or shared
state; gaps are first-class inventory rather than omitted from a successful plan. Humans and
repository-bound read-only agents can append version-bound findings only against retained inventory
items and with citations. Resolved identities must match their authoritative resource store and
applicable exact revision; unavailable identities remain explicit gaps. Catalog participation is
held through persistence, and a filesystem-wide ledger lock preserves finding CAS across API
processes. A federated relationship resolves only through contribution authority bound to the exact
local source or target repository and its distinct corresponding revision; an otherwise trusted
peer is not sufficient evidence. Issues require an exact implementation affected revision and tasks
an exact assignment base revision before either can be called resolved. Repository-bound organization
agents retain findings under their current access-grant boundary without borrowing operator authority.
This is a review and coordination boundary: it creates no repository,
moves no Git or collaboration record, changes no owner/visibility/policy, and grants no source or
destination write authority.

Current human collaborators assemble reviewed mappings through a plan's `candidate-sets` route.
Immutable bare candidates remain outside the repository catalog and retain mapped trees over the
requested source ancestry, source authorship, selected tag objects, license paths, required links,
exact tips/trees, object and byte costs, signature status, and SHA-256 provenance. Collisions,
unavailable history, policy gaps, inaccessible inventory, missing tags, and independently controlled
resources are explicit decisions. Exact retries return the same candidate.

Candidate rehearsals freeze exactly eleven required scenario kinds per destination, reject duplicates,
and enforce per-scenario and 900-second aggregate budgets. Stock Git performs integrity, clone,
fetch, and push into a disposable copy. Builds, checks, package and API resolution, documentation,
workspaces, and consumer journeys run from candidate clones in preinstalled networkless, read-only,
capability-free containers. Outcomes, bounded logs, cost, unsupported commands, and candidate gaps
remain inspectable; assembly and rehearsal never register destinations or move authoritative state.
Candidate assembly and ledger registration share a cross-process serialization boundary; bare repositories
publish from unique staging paths by atomic rename, and exact concurrent requests reconcile safely.

Active work is proposed separately through a plan's `collaboration-mappings` route. A mapping binds an
inventoried source resource and exact revision to a branch, pull, issue, proposal, task, decision, check,
session, workspace, or queue snapshot. The snapshot retains named authors, discussion and review identities,
dependencies, and original acceptance criteria; a divided change names every destination contribution,
revision, owner, dependency edge, and local criterion. Embargoed, inaccessible, rejected, and deliberately
unmigrated work stays blocked or archived instead of leaking context into another repository.

Named source authors inspect and decide mappings through revision-bound, append-only decisions. During plan
admission the server resolves each exact resource's author or controlling owner from its authoritative Git,
pull, issue, proposal-task, release, package, documentation, governance, workspace, workflow, dependency, or
repository record. Caller inventory must match and the ledger persists a server-owned copy. Mapping authors
must then exactly match that admitted set, so neither a plan nor mapping proposer can omit a required participant. Every author
must approve the same exact source revision before a mapping becomes approved, so an old review is preserved
as history but never silently promoted to current approval. Decisions recheck repository participation at
persistence. Blocked and archived mappings reject ordinary decisions and require a new successor mapping;
they cannot be revived through approvals. These mappings create no destination resource and grant the restructuring coordinator no Git,
review, queue, session, workspace, issue, task, or independently owned destination authority.

Dependent entry points and consumers are retained beneath a plan's `dependent-migrations` route. The closed
set covers clones, forks, packages, APIs, dependencies, extensions, workflows, documentation links,
deployments, and federated followers. Every entry freezes its accountable owner, audience, current state,
bounded compatibility window, next action, explicit replacement remotes, machine-readable dependency/link
mappings, and ordered safe synchronization guidance. Public-audience entries are also projected through the
plan's versioned `dependency-map` endpoint; private owner and participant context is never included there.

Replacement remotes must use credential-free HTTP(S) URLs and name a declared destination. Self-redirects,
duplicate destination redirects, and self-mappings are rejected rather than creating a loop. States keep
discovered, planned, in-progress, blocked, unavailable, rejected, stale-credential, and unmigrated clients
separate from adopted clients. Only the retained dependent owner can append state evidence, and adoption
requires nonempty evidence. A new migration owner must be a current source-repository participant, and both
creator and owner participation are held through persistence; event writes revalidate and hold that owner
access so revocation leaves adoption visibly blocked instead of producing an unreachable authority. An optional human- or agent-owned propagation reference can point to ordinary
target-repository task, pull, and release work; it records coordination without granting the restructuring
team authority in that repository. These records grant no Git, package, API, extension, workflow,
documentation, deployment, federation, pull, release, or consumer authority.

After a gap-free candidate's latest rehearsal passes and active-work mappings are approved or archived, a human
collaborator can stage cutover. Each destination owner approves independently. Activation binds exact pre-created
destination repositories, imports the immutable candidate commits, publishes default refs, and activates a source
write boundary; stock Git rejects pushes to the old authority with migration guidance. Activation authorization is
serialized before Git mutation, all destination refs are preflighted, exact prior publication reconciles, and a later
failure target-checks compensation for only refs created by that attempt. Before mutation, a durable `publishing`
intent pauses source writes. If publication or compensation fails, the
cutover retains `publication_blocked`, uncertain destination health, and an explicit blocker; exact retry reconciles
surviving candidate-tip refs while the source stays paused. The conflict response carries the current persisted plan,
and the workspace refreshes it before retry. When a
destination ref moved independently and cannot reconcile, the human controller may restore source authority without
mutating that destination ref; its state remains independently changed and requires its owner.
The workspace and API retain live source/destination state, active controls, approvals, dependency adoption, and bounded build, release,
permission, link, supported-consumer, ordinary-contribution, and Git-traffic evidence for every destination. Failed or residual current
evidence, late writes, and unadopted dependents prevent cleanup. A complete passing matrix applies the declared
read-only, archive, or removal policy; rollback restores source authority without erasing retained history.

## Shared regression search boundaries

Repository collaborators open regression investigations through `POST
/repositories/{id}/regression-investigations` or the repository's Regression investigations page.
The source must resolve to an issue, support thread, failed check (`pull_id/run_id`), release,
deployment, or demonstrated/reproduced debugging scenario (`workspace_id/scenario_id`). The server
resolves release and check commits, requires both 40-character boundary commits to exist, and proves
that known-good is an ancestor of known-bad before retaining the record. Creates include a stable
`request_id`; exact retries reconcile to the already-published record, while changed reuse conflicts.
Reconciliation occurs after authenticating current repository access but before mutable owner, source,
boundary, and ancestry validation, so an ambiguous successful publication remains discoverable.

The immutable starting context contains the expected and regressed behavior, good/bad labels and
revisions, affected environments, severity, owners, acceptance criteria, and evidence metadata.
Evidence availability is derived from repository-owned issue, support, check, release, deployment,
debugging, and Git stores rather than caller visibility prose. Unresolved evidence is retained as an
explicit unavailable diagnostic, and later missing revisions are projected as stale without rewriting history.
Reads also re-evaluate stateful evidence predicates: for example, rerunning a retained failed check
downgrades that evidence to stale and unavailable rather than preserving its old available projection.
If the same authoritative source later returns to its required state, projection clears that gap and
reports it available again without rewriting the retained creation record.
Discussion, hypotheses, environment scope, and open/bounded/paused/closed status append through a
compare-and-swap event route with actor and time attribution. This ledger agrees on what history to
search; it grants no Git, private-machine, check execution, environment, deployment, or evidence
access authority.

Participants and repository-bound agents can define immutable comparison scenarios beneath an
investigation. A scenario freezes its expected and regressed behavior, acceptance criteria, synthetic
or privacy-preserving inputs, preinstalled container image, revision-specific setup and exact command,
working directory, environment, timeout, CPU, memory, and storage limits. Attempts select an exact
repository commit or attested release and may pin named dependency revisions. The server resolves every
selected revision. Named dependencies include their authoritative readable repository, exact revision,
and deterministic workspace path; each bounded Git archive is digest-checked and materialized beneath
`/workspace/dependencies` before the workspace becomes read-only. The ordinary credential-free executor then runs: an exact Git archive is mounted
read-only with no network or Linux capabilities, while only bounded scratch and digest-addressed output
are writable. One to five clean repeats retain the complete environment, input digests, command, stdout,
logs, exit state, artifacts, duration, compute cost, actor, and target/dependency provenance.
Each caller-stable attempt request reserves its durable investigation identity before any check run is
created. Completion finalizes that identity independently of later investigation edits; retries reconcile
the reservation and deterministic run names, preventing completed but unattached duplicate work.
An overlapping exact retry that observes a queued, running, or cleanup-pending check returns the same
running reservation and never converts that nonterminal state into failure evidence. Environment
incompatibility derives only from trusted executor setup metadata. The executor verifies the preinstalled
image before starting the container and marks that failure source structurally; command-controlled stderr
and exit codes cannot change a completed behavioral failure into a setup classification.

Attempt classifications distinguish `passed`, `failed`, and mixed `flaky` runs from
`incompatible_setup`, `missing_dependencies`, `unsafe_fixture`, and `untestable_revision`. The latter
states are durable gaps rather than behavioral evidence, so collaborators can tell a code regression
from historical setup, data, dependency, or nondeterminism differences. Scenario records and attempts
grant no Git, package, network, environment, release, deployment, or general execution authority.

Once a scenario exists, collaborators schedule an evidence-driven search over the full ancestry path between
the frozen good and bad commits. The durable candidate set preserves merge parents and may include explicitly
selected readable cross-repository dependency revisions. Completed attempts from only that search's frozen scenario
are projected onto the set; the server recommends an untested midpoint and derives all still-supported,
graph-ancestry-proven working-to-regressed ranges
rather than publishing an opaque bisect answer. Each range reports its remaining candidates, confidence, and
merge or flaky ambiguity. Candidate views include the direct changed paths, commit author, linked ordinary pull
requests, and the exact attempts supporting their classification.
Dependency evidence joins on both repository identity and revision, preventing an equal hash from a different
repository from supplying or hiding a candidate outcome.
Hypothesis attempt citations use that same exact identity and reject revision selectors that identify more than one
search candidate. Candidate outcomes aggregate across all matching completed attempts without storage-order
precedence: mixed pass/fail or any flaky result remains explicitly flaky.

CAS-versioned collaborator guidance records working, regressed, flaky, invalid, excluded, and restored trials
with attribution and rationale. Rewritten or missing commits become explicit exclusions on reads. Causal
hypotheses name exact candidates and must cite retained investigation evidence or historical attempts from the
same frozen scenario and selected revision, keeping
agent explanations reviewable beside human decisions. Multiple ranges, interacting dependency changes, flaky
midpoints, and regressed merges remain competing claims until additional evidence resolves them. Searches are
evidence ledgers only and grant no Git, check execution, pull, package, agent, or repository authority.

Named human investigation owners can carry a supported search into a durable response comparison. The
comparison must include exactly one revert, rollout/configuration containment, dependency adjustment, and
forward-repair option, each with explicit benefits, risks, constraints, affected repository releases, affected
open or queued pulls, and backport targets. Its culprit ranges are frozen from the live evidence projection so
later readers can see what supported the decision rather than only its outcome. An owner may select one option
with a rationale and derive ordinary human- or agent-assigned proposal tasks whose reasoning retains the
investigation, response, known-bad revision, original expected behavior, acceptance criteria, tradeoffs, and
backport scope. Exact retries converge on the same proposal and task identities. Those tasks may later use the
platform's existing task session and shared-workspace paths and publish ordinary attributed pulls; the
regression ledger itself grants no rollback, Git, agent, workspace, pull, review, merge, queue, release,
deployment, configuration, or environment authority.

Every exact repair or backport is proved through `POST
/repositories/{id}/regression-investigations/{investigation_id}/corrections`. The candidate must be the current
revision of its published response task pull. Its retained historical scenario must have a completed passing
attempt on that exact revision, each affected check name must resolve to one successful exact-revision pull run,
every selected requirement must exist in a current quality plan, and the proof must carry all original change
acceptance criteria. Exact request retries reconcile instead of duplicating proof.

Reads derive the candidate's reviewed, merged, released, deployed, and observed state from ordinary pull,
release, and deployment records. They reopen proof when the pull moves, a good/bad baseline becomes stale, a
selected check disappears or fails on rerun, or deployment/health evidence disagrees with the correction.
Failed scenario runs and partial acceptance never become corrections, including failed backports. Requirement
IDs and check names retain the reviewable bridge for promoting a maintainable scenario into the quality plan and
required-check policy; the investigation itself does not mutate either system or grant delivery authority.

The connected `regression-investigation-journey.spec.ts` browser/API/stock-Git journey starts from a
released user report and retains a merge-shaped history, exact passing and failing comparisons, an
unbuildable revision, flaky midpoint ambiguity, a challenged false culprit, an evidence-supported
merge cause, a failed-revert tradeoff, containment, agent-owned forward repair and backport work, and
revoked agent evidence access. It proves that the web workspace and public ledgers preserve one
reviewable path from user impact through bounded experiments and ordinary governed recovery.

## Meaning-preserving conflict checkpoints

Conflict-reconciliation workspaces can turn a resolved working tree into an unreferenced, immutable
two-parent Git candidate without moving either contribution. Every checkpoint must cover all affected
required checks plus a reproduction, contract scenario, schema scenario, preview acceptance, and a
repository conflict test. Each retained criterion names whether it came from the source, target, or both;
its exact command and acceptance criteria; affected paths and owners; exit state, bounded logs,
digest-addressed artifacts, and cost. Each criterion is reset to the candidate before execution so generated
or modified files from one command cannot change the tree evaluated by another. A required-check name is bound
to its complete parsed check definition frozen from the exact target revision; callers cannot replace it with a
weaker command or workspace runtime. Required checks use the ordinary disposable, read-only, network-disabled
check executor with the frozen image, timeout, resources, inputs, environment, output contract, and injected
candidate identity, and the checkpoint retains the durable check-run identity. Dependency-manifest and effective-policy digests are resolved by the server, and the
store rechecks the initiating command lease and running state immediately before retaining evidence.
Candidate dependency lookup distinguishes a successful missing-path result from Git, storage, or executor failure;
only the former receives the stable absence digest, while operational uncertainty blocks the checkpoint.

Affected source and target owners can accept or reject individual deliberate behavior results with an
attributable rationale. A later checkpoint compares source, target, dependency, and policy revision inputs
and appends staleness only to criteria that consumed a changed input. Historical evidence and decisions stay
visible, but stale criteria cannot receive a new owner decision. Candidates and checkpoints remain evidence:
they grant no Git publication, review, merge, release, dependency, policy, workspace, or deployment authority.

Once every current criterion passes and its affected owners' latest decisions accept it, a current destination
repository participant can publish through a caller-stable reservation. Publication either compare-and-swaps the
verified source branch or creates a connected resolution branch and ordinary pull. The published two-parent commit
retains both inputs plus applied-resolution authorship, exact commands, and owner decisions; affected required checks
run again on that published identity. Ordinary revision-bound review and integration-queue rebuilding then decide
whether it lands. Withdrawn approval, moved inputs, concurrent branch updates, and occupied successor branches remain
durable actionable states and never overwrite either contribution.

The connected `conflict-resolution-journey.spec.ts` regression proves the complete loop with the browser, public
API, Docker-isolated verification, and stock Git. Two reviewed branches collide textually and on a changed declared
symbol after queue movement; stale evidence and a second rejected queue admission remain visible. Both owners and a
read-bounded approved agent enter the exact workspace, human review rejects uncertain agent advice, and a failed
combined criterion precedes the accepted checkpoint. The published two-parent result receives a fresh review and
required check, lands through the protected queue, and remains inspectable after one participant loses access.
Checkpoint calls use the API directly because evaluating all six isolated evidence classes can outlast a frontend
development proxy request without exceeding the API's own bounded execution contracts.

## Repository-reviewed collaboration workflows

Authorized repository collaborators use `/repositories/{id}/collaboration-workflows/preview` or the
repository workflow workspace to inspect recurring collaboration before it reacts to project events.
The caller supplies only an exact 40-character commit and repository-relative JSON path. The server
requires that commit to remain reachable from a non-security branch, reads and hashes the blob, and
derives typed event subscriptions, parallelizable dependency stages, and each step's effective
principal, authority grants, and authority boundary.

Definitions state a shared outcome and retain typed triggers and inputs, conditions, step dependencies,
platform/component/agent/workflow invocations, outputs, retries, timeouts, per-step and workflow action
budgets, accountable current-participant owners, and completion criteria. Platform actions and reusable
components are closed sets. Agent invocations must resolve to a gap-free reviewed agent project in the
same repository; reusable workflows must already be active there. Cyclic or missing dependencies,
self-invocation and subscribed-event loops, inaccessible resources, invalid conditions, missing owners,
budget violations, and allow/deny or required-authority policy conflicts become attributable preview
diagnostics and make activation impossible.

Activation and successor activation re-resolve the same reviewed blob and owner/resource facts. The
create request carries a caller-stable activation ID from which the server derives its workflow ID;
if directory sync fails after atomic publication, retrying that request therefore returns the same
record instead of activating a duplicate. Workflow-reference traversal is repeated inside the locked
revision transaction so concurrent successor publication cannot introduce direct or indirect
recursion. A
successful transition appends an immutable compare-and-swap version containing its exact source digest,
subscriptions, stages, authority preview, actor, and time beneath `$COLLABORATION_WORKFLOW_STORAGE_ROOT`
(`collaboration-workflows`). The record explains coordination only; it does not subscribe to events,
execute actions, grant agent or repository authority, or bypass runtime, secret, review, release, or
deployment controls.

Authorized repository event dispatchers create durable runs at
`/{workflow_id}/executions` with a caller-stable event ID, the selected immutable workflow version,
authenticated actor, occurrence time, declared trigger inputs, and exact resource commits that still
resolve from ordinary branches. Identical delivery retries reconcile to the same run; changed reuse is
a conflict. A workflow admits one active run and at most 60 starts per hour. Execution records freeze the
reviewed source, action budget, step attempts, output provenance, interruption or cancellation reason,
and terminal state.

Current step owners claim only dependency-ready work with the execution CAS version. A claim returns a
random, one-step capability that expires at the reviewed timeout and contains only that invocation's
declared authority plus event or successful predecessor outputs named by the step input mapping. The
ledger persists only its digest. Completion requires that capability, accounts actions against both
step and workflow ceilings, rejects undeclared or credential-shaped outputs, and never forwards hidden
event context. Failed or expired attempts remain visible and may be reclaimed only within the reviewed
retry bound; cancellation revokes outstanding capabilities deterministically. The capability authorizes
workflow bookkeeping only: invoked platform, repository, organization, agent, embargo, environment,
approval, review, release, and deployment operations still apply their own current policy.
Pull-trigger dispatch specifically binds the declared `pull_id` input to a same-repository pull and
requires its `pull_id` resource revision to equal the server-resolved current source commit; omitted,
invented, unreachable, and mismatched provenance is rejected. Successful completion retains a digest
of the exact token and result request, so a retry after an ambiguous rename, directory-sync, or response
failure returns the already committed execution without double-accounting. When one parallel step
exhausts its attempts, terminal publication cancels and clears every sibling lease in the same atomic
record.
The dispatch request itself contains only a workflow version and server-issued activity delivery ID.
Immutable pull-created, pull-synchronized, and pull-merged deliveries retain their platform actor,
time, resource identity, and exact source revision. The workflow runtime maps those records to reviewed
trigger names and derives every `TriggerEvent` field server-side; arbitrary participant-supplied event
IDs, names, times, actors, inputs, revisions, cross-repository deliveries, unsupported event kinds, and
deliveries made stale by pull movement all fail closed.
Repository-owner issue triage similarly emits an idempotency-keyed `issue.accepted` at the then-current
configured default-branch revision before returning a committed-but-durability-uncertain status mutation.
A repository without a default-branch commit cannot enter triage. Any later triage retry reconciles the
retained snapshot even after other issue mutations. Dispatch derives its issue, actor, time, and revision and
requires the activity to match that reachable snapshot; ordinary branch movement neither rewrites nor strands
the accepted event, while a mismatched or unreachable revision fails closed.

The workflow workspace and execution reads project a live dependency graph and preserve every attempt's
timing, resolved inputs, declared redacted outputs, sanitized logs, artifact digest metadata, agent session,
cost, failure, and source/event provenance after the run finishes. Capability and completion digests never
leave the store, and restricted artifacts are omitted. Waiting approval, requested-input, manual,
dependency, retry, and optional states feed predicted next actions. Current write collaborators use a single
CAS intervention route to pause, resume, cancel, retry, approve, provide a named non-secret input, skip a
reviewed optional step, or take over a declared manual step. The immutable intervention trail attributes the
actor, reason, target, time, and resulting version; it accepts neither private terminal streams nor
credential-shaped content and does not grant authority to the invoked system.

## Attested workflow components

Maintainers publish reusable workflow contracts through `POST
/repositories/{id}/workflow-components`; authenticated readers inspect every visible immutable version
through `/workflow-components`. Publication binds the exact component JSON blob and digest to a
40-character commit reachable from a non-security branch, an active package built from that same
commit, the package digest and publisher, typed input/output contracts, requested capabilities,
explicit data-use terms, workflow-format/platform compatibility, passing test digests, and a support
policy. A federation provenance claim additionally resolves a currently trusted peer. Catalog
projections keep changed repository ownership, changed package publisher, yanked/deprecated/
quarantined packages, unavailable or revoked peers, and declared breaking migrations visible instead
of treating an old attestation as current trust.

Consumers use `/repositories/{id}/workflow-component-installations` or the repository workflow
workspace to pin an exact semantic version through a current ordinary open pull. The server loads
`.vivarium/workflow-components/{local_name}.json` from that pull's exact source commit and derives the
component ID, mappings, configuration, and accepted data uses; callers cannot attach arbitrary terms
to an unrelated pull. They map every
requested capability to one repository-local permission, accept each exact data classification and
purpose, and provide only bounded credential-free local configuration. Compare-and-swap installation
successors retain the complete earlier pin and pull history. A workflow component invocation names
`local-installation@version`; activation fails unless that exact pin is current, its publisher ownership,
package digest/source/publisher/lifecycle, and optional federation-peer trust remain current, and every
declared authority is one of its local mappings. Existing execution records therefore remain attributable to
the old workflow revision when a consumer updates, retains, or replaces a component. The ledger
defaults beneath `$WORKFLOW_COMPONENT_STORAGE_ROOT` (`workflow-components`) and grants no package,
federation, Git, review, merge, repository, runtime, secret, or publisher authority.

### Governing workflow change and consequential effects

Repository owners can add a versioned workflow-governance baseline requiring independent reviews,
named simulated event cases, resource-owner acknowledgements, separation of duties, and bounded
approval windows for workflow definitions and merge, release, infrastructure, protected-evidence,
or spending effects. Exact-source candidate records compare effective permissions, expected effects,
action cost, and policy conflicts with the current workflow revision. Current attributable decisions
must make that exact candidate ready before activation.

Runs project expiring owner approval requests and immutable action receipts. An owner can disable a
workflow, stop it for anomalous behavior or changed authority, or select a prior immutable revision.
These controls prevent new starts and claims while preserving legitimate completed attempts, outputs,
costs, receipts, interventions, executions, and workflow revisions. Governance records coordinate
consent only and do not grant authority over the protected action itself.

`collaborative-workflow-journey.spec.ts` connects accepted issue triage, a reviewed bounded repair agent,
exact workflow activation and execution, human redirection, protected merge/release/deployment decisions,
retained evidence and receipts, and the browser run graph. It contains duplicate delivery, stale revision,
interruption, revoked lease, action-budget excess, and non-owner approval before the exact run succeeds.

## Evidence-backed software adoption

Authenticated collaborators use `/adoption-workspaces` or the web adoption workspace to ask whether
an exact external software version fits a declared outcome. A workspace begins from an admitted
roadmap outcome, support gap, incubator, decision, package, API, or federated repository and freezes
required journeys, target environments, constraints, budget, owners, weighted evaluation criteria,
and one or more candidate versions.

Provider maintainers and affected users remain pending until they explicitly accept their invitation.
Approved organization agents can participate only as read-only observers. Each candidate retains
separate capability, provenance, support, security, data-use, compatibility, and known-gap evidence.
Pending humans can discover only their invitation coordinates, role, inviter, and timestamp until
they accept. Reads recheck repository access and replace evidence outside the viewer's current
boundary with an opaque gap; typed human and organization-agent identities cannot collide.
Unavailable references remain missing or inaccessible, while evidence naming another candidate
version is derived as stale. Candidate fit stays `undetermined` whenever any dimension lacks current
evidence. The ledger defaults beneath `$ADOPTION_WORKSPACE_STORAGE_ROOT` (`adoption-workspaces`) and
grants no package, API, procurement, Git, environment, deployment, or provider-roadmap authority.

Participants can append CAS-versioned bounded trials to an exact candidate. A definition resolves
either an existing released repository release or an exact commit, narrows packages and APIs, uses
synthetic or explicitly permitted data, selects only the workspace's declared journeys, and freezes
policies, setup, configuration, commands, integration changes, and a cost ceiling. Human participants
and approved invited agents can retain immutable attempts with checks, previews, measurements,
artifact digests, costs, findings, and user feedback. Passed, failed, blocked, and non-reproducible
attempts remain side by side for cross-version comparison. Credential-shaped prose is rejected and
repository sources are authorized before resolution; the trial grants no execution, package, API,
repository, environment, production-data, or deployment authority.

After a passing attempt independently reproduced by a different consented human, a human adopter or consented provider maintainer
can publish a CAS-versioned adoption agreement. The agreement binds the exact candidate and trial to
the integration architecture, adopter/provider/shared configuration decisions, update and support
policy, service and data boundaries, explicit exceptions and remaining fit gaps, compatibility
promises, recurring cost, and an exit strategy. It also retains a strictly ordered set of human- or
agent-owned consumer-repository, environment, documentation, and permitted-upstream-fork work.
Repository targets and repository-scoped environments are resolved before publication while the
catalog holds their existence, ownership, visibility, and collaborator facts stable through the
agreement write. Each item previews the reader's current owner, collaborator, read-only,
inaccessible, or stale-target state; reads re-resolve targets, redact inaccessible details, and mark
removed owners or repositories stale. The preview and assignment are coordination
facts only: every item remains `no_authority_granted`, and execution must pass through the target
repository or environment's ordinary access, secret, review, deployment, and roadmap controls.

A consumer participant can then bind an agreement to an already-merged pull, its current human
approvals and exact-commit passing checks, the release that includes that pull, and a finished staged
deployment. The delivery snapshot derives provider, pull, merge, release, and environment revisions
from their owning stores and retains policy, rehearsal, support, user-acceptance, cost, rollout, and
health attestations. Unmet criteria or a failed/paused rollout remains an explicit safe pause; a later
successful recovery links back to that paused snapshot. Repository access is rechecked on reads and
restricted delivery identities are redacted. Adoption delivery records grant no review, merge,
release, deployment, pause, restoration, agent, or provider authority.

Adopters and invited agents can select trial findings, reproductions, support questions,
compatibility evidence, documentation feedback, and usage outcomes for upstream sharing. Each
record names exact trial/attempt or delivery provenance, the redactions applied, and provider-only,
participant, or public visibility. Pending content is disclosed only to its author and a consented
human provider maintainer; an embargo remains author-local, and a provider rejection becomes an
immutable local-only outcome. Accepted records can link to an existing ordinary issue or local,
fork-based, or federated pull after the API verifies repository topology and authenticated
authorship. The workspace creates no review, check, merge, or maintainer authority.

Provider unavailability, rejection, and embargo therefore preserve a consumer-owned local-pull
path without exposing evidence. When ordinary provider review produces a merged contribution and
an exact including release, the adopter can verify replacement of that local patch only through a
separate merged and passing-check consumer update, its including release, and a succeeded staged
deployment. The provider release must publish one unambiguous package version and the consumer
release's exact-commit inventory must resolve that version as a direct dependency. When a local pull
is named as replaced, it must share the finding provenance and every path it changed must also be
changed by the consumer update. The retained replacement connects both releases and the measured
usage outcome while leaving each repository and environment independently governed.

`adoption-journey.spec.ts` proves the complete loop through the browser, public API, stock Git, and
a scoped package client. An independent consumer compares an unsuitable candidate with an exact
provider release, retains inaccessible evidence as a gap, obtains explicit provider/user consent,
and has both an approved agent and target user reproduce the fit. The journey rejects a leaked
credential, records the denied shared-credential exception in the authority-free agreement,
delivers a reviewed pinned dependency, and retains a failed version-regression rollout. A redacted
finding survives provider rejection and unavailability before a consumer-authored fork pull is
reviewed and released by the provider; a separately reviewed, checked, inventoried, and deployed
consumer update then verifies the provider-native replacement and measured user outcome.

## Project incubators before repositories

Authenticated collaborators use `/incubators` or the web project-incubator workspace to establish
purpose and participation before choosing filenames, frameworks, or repository ownership. An
incubator records a working title, audience, problem, desired outcome, constraints, success
measures, sponsors, explicit decision rights, visibility, and human or approved-agent invitations.
Its starting point can be existing feedback, a support gap, a governed proposal, or an original
idea. The API resolves repository context only inside the creator's current read boundary and
retains `resolved`, `missing`, or opaque `inaccessible` diagnostics.

An invitation does not imply read participation: pending and declined humans receive no
participants-only context, and use only the narrow invitation consent endpoint until accepted.
Each human invitee must append explicit consent before contributing. Approved organization agents retain their already-governed identity; the
incubator grants no new runtime or tool authority. Versioned append-only events attribute
discussion, evidence, assumptions, scope changes, visibility changes, and consent to a stable human
or agent identity. Scope mutations select only the typed scope-change right; owner rules select the
declared owner, while majority and consensus/consent rules count exact-body support events from
distinct declared principals. Duplicate-looking initiatives remain separate and are reported explicitly to
readers who can see both. Storage defaults beneath `$INCUBATOR_STORAGE_ROOT` (`incubators`) and
grants no repository, Git, organization, implementation, review, release, or deployment authority.
Visibility mutations use the same rule evaluation through their separate typed right; support
events name the decision kind, preventing a scope vote from authorizing publication.
Publication first directory-syncs a conservative durability marker and clears it only with a second
synced canonical copy; post-rename sync failures therefore return committed, explicitly uncertain
state instead of inviting a duplicate retry.

### Initial public-life readiness

After an active bootstrap and running delivery slice exist, human participants publish a complete
`/{incubator_id}/launch-readiness` view for a declared experimental, limited, or public audience.
The immutable view covers ownership, support/governance, licensing/provenance, security/privacy,
accessibility, documentation, package/API adoption, service objectives, continuity, contributor
setup, operating budget, prototype debt, and target-user validation. Each expectation binds a
current/missing/unsafe/unsupported/failed/stale evidence statement and reference to an accepted
human participant owner.

Only that exact owner can accept current evidence or grant an exception. Exceptions require a
connected follow-up work reference, expire within 30 days, cannot override failed user validation,
and narrow a declared public audience to limited. Missing decisions, expired exceptions, missing
maintainers, unsafe defaults, unsupported promises, and failed validation remain explicit blockers.
The ledger recomputes effective audience and blockers on reads; it grants no release, package,
deployment, governance, budget, repository, or linked-work authority.

### Launch and continuing stewardship

A ready assessment can publish one immutable launch manifest at `/{incubator_id}/launches` for its
exact effective audience. The manifest identifies the existing attested release, documentation,
contributor opportunity, governed environment, and at least one package or API contract. Every
repository reference stays inside the readiness delivery boundary; the incubator records the
publication but grants none of those systems' authority.
Publication resolves every entry through its repository-owned release, documentation, package,
API-contract, contributor-opportunity, or environment store and permits exactly one first launch
per incubator. Missing resolvers and missing or mismatched resources fail closed.

Participants append adoption, support, reliability, cost, success-measure, and feedback observations
with their existing evidence identities. Those signals can connect a roadmap revision or ordinary
human- or agent-owned proposal task back to the launch. A human participant eventually records one
terminal disposition: graduation to an organization initiative, continued experiment, merge into an
existing project, or archive. Graduation and merge name their target; archive requires explicit
resource and obligation resolution, preventing a closed incubator from silently abandoning either.
After transition the launch trail is immutable.
Observations likewise resolve through feedback, support, service-objective, fund, or outcome stores;
roadmap/proposal work and graduation/merge targets must exist in their owning stores. Archive
resolution exhaustively maps every frozen launch artifact ID to an existing closed repository-scoped
governance proposal (`artifact_id=proposal_id`) and names every readiness evidence reference. Current
accepted obligations remain resolved; exception-backed obligations require their governed follow-up
proposal to be closed.
Artifact-resolution proposals must have an accepted, uncontested final tally and an affected-resource
reference exactly matching the artifact kind, resource ID, and revision; a rejected or merely
repository-related proposal cannot close stewardship.

`incubator-journey.spec.ts` proves the complete connected workflow through the browser, public API,
and stock Git. A creator and consenting domain expert admit an already-approved research agent,
compare and reject foundations using bounded evidence, activate a previewed project boundary, deliver
an ordinary reviewed exact-revision slice, retain invited-user validation, accept all thirteen launch
expectations, and publish repository-resolved release, documentation, API, contributor, and environment
artifacts. Adoption feedback becomes governed continuing work before an explicit stewardship
transition. Duplicate intent, an unavailable owner, budget rejection and rollback, unresolved preview
evidence, and an incomplete launch are retained or rejected at their own authority boundaries.

### Evidence-backed project shape exploration

Accepted participants use `/{incubator_id}/alternatives`, `/experiments`, experiment `/results`, and
`/research-notes` to compare a project before selecting its permanent foundation. Every alternative
states its product boundary, architecture, interfaces, dependencies, licenses, operating-cost model,
security and data risks, build/adopt/hybrid posture, and unknowns. References to public and
organization research or existing decisions, prototypes, packages, API contracts, and exact code are
classified as resolved, missing, or inaccessible at admission; inaccessible organization or
repository context is never promoted to evidence.
Exact-code evidence additionally resolves the object as a commit reachable from a visible,
non-`vivarium-security/` branch and proves that the selected path is a blob in that commit tree.
Nonexistent, non-commit, hidden, and missing-path selectors are retained only as opaque gaps.

An experiment freezes its candidate, question, bounded environment, exact commands, reproducible
inputs, expected measures, safety limits, and evidence references under a canonical SHA-256 digest.
Human participants and invited approved agents can append outcomes, measurements, artifact digests,
and remaining unknowns, while research notes preserve assumptions, dissent, measurements, unknowns,
and explicit supersession. The ledger labels every definition
`research_only_no_code_or_infrastructure_authority`: an experiment is evidence, not a repository,
prototype deployment, package publication, or grant to mutate code or infrastructure.

### Governed project bootstrap

An accepted, non-superseded direction can be carried into
`/{incubator_id}/bootstrap-previews`. A preview reserves one complete manifest across organization,
repository, team, package, agent-role, contributor-pathway, documentation, environment, review,
security, privacy, quality, and release boundaries. Each entry says whether it will be created or
connected and exposes its owners, server-derived effective access, explicitly unverified monthly
cost estimate, generated content, inherited baseline, and metadata source. Existing organizations
and repositories are resolved inside the requesting owner's current authority; named owners must be
current, consenting human incubator participants. Other resource kinds cannot claim an existing
connection until their authoritative resolver exists.

The manifest retains its incubator/alternative source and generator. Every distinct resource owner
must append an attributable decision against an exact plan version before activation. Activation is
one canonical incubator-ledger transition, so none of the reserved identities, defaults, or policy
claims become authoritative after a partial write. Previewed, approved, or rejected reservations can
be rolled back as a unit, and CAS retries reconcile against the durable state. An active boundary is
project context only: ordinary organization, repository, credential, package, environment, policy,
review, release, and deployment APIs retain their own authority.
Activation re-resolves connected organizations and repositories while holding their mutation locks,
requires every declared owner to retain current ownership, and refreshes server-derived access,
generated-content, inherited-policy, cost-basis, and template-source fields before committing.

### Incubator delivery proof

After activation, a consenting human participant can freeze one representative journey as an
ordered five-part delivery plan covering code, tests, documentation, infrastructure, and interface
work. Every item names an exact repository commit, explicit acceptance criteria, dependencies, and
a member of the temporary human-agent team. Input dependencies use earlier one-based integration
positions and are persisted as immutable work-item IDs, preventing later ordering ambiguity.

Team members append attributable workspace, pull request, preview, invited-user feedback, agent
action, handoff, cost, deviation, check, and review reports to the plan. Reports preserve repository,
resource, and revision coordinates so the incubator shows how the running slice relates to ordinary
delivery records; the incubator remains a context ledger and grants no workspace, Git, preview,
review, merge, environment, or deployment authority. The web workspace renders the ordered slice
and its evidence beside the accepted direction and active boundary.

## Versioned agent projects

Repository collaborators define intended agent behavior at `/repositories/{id}/agent-projects` and review
it in `/repositories/{id}/agents`. Immutable revisions connect prompts, instructions, knowledge,
dependencies, tools, models, tasks, outputs, prohibitions, memory/data terms, budgets, owners, escalation,
and deployment limits to exact repository files and commits reachable from visible non-security branches.
Public projections recheck and redact sources across the complete history when dependency access or commit
visibility changes; successful mutations use that same projection. Publication authorizes source repositories
before resolving their contents, synchronizes content before rename and directory metadata afterward, and marks
a committed record `durability_uncertain` if post-rename synchronization fails. The conservative marker is
persisted and remains visible after reload until the canonical rename has been confirmed durable. Projections derive effective
capability and keep missing ownership, conflicting instructions, inaccessible dependencies, and
unsupported guarantees explicit. The ledger defaults beneath `$AGENT_PROJECT_STORAGE_ROOT`
(`agent-projects`) and grants no implementation or runtime authority.

## Versioned assurance programs

Repository collaborators publish assurance programs through
`/repositories/{id}/assurance-programs` and inspect them at `/repositories/{id}/assurance`.
Complete immutable revisions select regulatory, contractual, and organization requirements with
exact citations, applicability, interpretations, inherited sources, owners, and review periods.
Controls connect obligations to an explicit satisfaction claim, objective, owner, evidence
criteria, and exact repository, policy, data-flow, infrastructure, environment, release, or
operational-procedure resources.

Reads retain attributable missing owners, conflicting interpretations, inherited obligations,
unmapped requirements, unsupported control claims, and expired or near-expiry exceptions. Named
owners and exception grantors are rechecked as current participants at publication. Records
default beneath `$ASSURANCE_PROGRAM_STORAGE_ROOT` (`assurance-programs`) and grant no Git,
policy, evidence, infrastructure, environment, release, or operational authority.
Scope publication resolves data-flow maps, infrastructure definitions, deployment environments,
and releases in their repository-owned ledgers. Policy and procedure resources resolve as exact
files in the containing repository, with `resource_id` equal to `path` and a retained Git revision;
invented and cross-repository associations are rejected.

### Continuous assurance evidence

Named control owners define immutable collection queries at
`/repositories/{id}/assurance-evidence`. A definition freezes the assurance program and control
version, assessment window, collection cadence, least-privilege audience, source categories, exact
selectors, and freshness expectations. Collection requests contain query IDs only; source identity,
revision, timing, provenance, outcome, and transformations resolve from repository-owned stores.
Collections retain sanitized source links rather than
screenshots or source payloads. The server derives source hashes, coverage,
missing/inaccessible/stale gaps, contradictions, a canonical manifest hash, and an actor/time
attestation. Restricted sources become opaque gaps with no identifying metadata. Records default
beneath `$ASSURANCE_EVIDENCE_STORAGE_ROOT` (`assurance-evidence`) and grant no linked authority.

Program owners can open bounded independent reviews through
`/repositories/{id}/assurance-assessments`. Each record freezes the exact program version, controls,
admitted system/release scope, assessment period, selected immutable evidence-package IDs, an
identified internal or external assessor, conflict disclosure, and a maximum-90-day access window.
Only the owner and invited assessor can read the record. Their separate event capabilities retain
questions, samples, walkthroughs, attestation verification, findings and responses, disagreements,
resolutions, appeals, and acknowledged scope changes without granting either side authority over Git,
production, releases, deployments, source systems, or evidence collection. Records persist beneath
`$ASSURANCE_ASSESSMENT_STORAGE_ROOT` (`assurance-assessments`).
If correction produces a later delivered release, the owner can propose that exact repository release
only when it descends from an originally selected candidate release. The assessor must explicitly
acknowledge the proposal before that delivered release is eligible for a statement; a pending proposal
cannot be silently replaced.

An assessment owner can convert an assessor finding into an ordered ordinary proposal plan at
`/{assessment_id}/remediations`. Each handoff freezes the finding and control, affected 40-character
revision, deadline, acceptance criteria, human or agent assignees, and task order. The resulting tasks
reuse the existing session, workspace, pull, review, policy, release, and operational boundaries; the
assessment record grants none of those authorities. Closure is not inferred from a ticket: every task
must have a merged contribution, and an owner or assessor must disposition fresh, gap-free,
post-correction evidence for the exact program and control. The affected commit resolves in the
repository, each ordered merged contribution descends from the prior boundary, accepted evidence names
the final merge revision, and a later statement release must descend from and include all those tasks.
The named release must resolve through a release scope retained by the assessment or the acknowledged
delivered-release amendment; publication retains the originally selected candidate scope and exact
delivered release rather than implying another assessed system.
A rejected or reopened disposition returns
the correction to open without removing its delivery history.

Program owners publish release-exact claims at `/repositories/{id}/assurance-statements`. Publication
requires the current assessed program revision and accepted verification for every included finding,
and freezes the human-readable claim, release commit, scope and period, controls, exception IDs, audience, expiry, and a
canonical digest of selected and verification evidence. The service signs the immutable payload with
Ed25519 and returns its public key. Audience-gated reads disclose the digest, never restricted sources,
and separately project `current`, `expired`, `revoked`, `drifted`, or `reopened`. Revocation and later
status changes do not rewrite the originally signed claim.

`assurance-journey.spec.ts` proves the connected private-repository workflow with browser, public API,
and stock Git clients: obligation mapping, missing and current evidence, rejected exception authority,
bounded independent access, contested findings, ordered human/agent remediation, pull assurance impact,
ordinary review and merge, fresh delivered-release evidence, acknowledged scope, Ed25519 publication,
program drift, and revocation. The Playwright server assigns isolated roots to every assurance store.

### Pre-merge assurance impact

Pull reviewers inspect revision-exact compliance impact through
`/repositories/{id}/pulls/{pull_id}/assurance-impact`. A human collaborator selects an exact
assurance-program revision; the server derives affected controls and paths from the exact Git diff
instead of accepting caller-authored scope. Decisions retain applicability, changed evidence,
tests, notices, retention work, exceptions, mitigations, residual risk, and required owners.

Humans and exact pull-task agents append cited analysis and challenges. Only current named human
control owners acknowledge. Pull movement makes the assessment stale; a program successor
invalidates only controls whose definition changed. A current program makes missing, uncertain,
stale, or unacknowledged impact an ordinary merge and integration-queue blocker, and the pull
workspace renders the same retained matrix. Storage defaults beneath
`$ASSURANCE_IMPACT_STORAGE_ROOT` (`assurance-impacts`) and grants no linked authority.

## Revision-exact capability inventory

Authorized repository collaborators publish immutable capability revisions at
`/repositories/{id}/capabilities` and inspect them in the repository `/capabilities`
workspace. Each revision binds its released provider commit and selected interfaces,
symbols, flags, packages, schemas, configuration, documentation, journeys, and release
surface to exact Git revisions and paths. It also records accountable owners, environments,
consumer revisions, usage evidence, discovery mode, and compatibility promises.

Named consumer repositories must remain readable to the publisher through the atomic
publication boundary. Derived diagnostics preserve unknown and runtime-discovered use plus
stale, inaccessible, and unmeasured evidence as blockers or warnings; none are converted into
an assumption that use is absent. Records default beneath `$CAPABILITY_STORAGE_ROOT`
(`capabilities`) and document footprint only: they grant no Git, consumer, release,
environment, migration, merge, deployment, or removal authority.

Human participants may open a retirement contract from an inventory revision. The contract names
supported replacements and migration guidance, what stops working for each affected audience,
ordered compatibility stages, owner-response and removal deadlines, success and rollback criteria,
communication cadence and escalation, and every consumer owner whose acknowledgement is required.
Humans and repository-bound read-only agents append cited impact assessments or challenges, but an
agent cannot approve removal or make policy. Exact owners acknowledge only for themselves.

The projection keeps unknown or stale usage, later inventory changes, missed acknowledgements,
conflicting compatibility promises, active maximum-30-day exceptions, and embargoed dependencies as
attributable blockers. Embargoed consumer detail is projected as a restricted affected audience for
readers without that repository access. A human may record only a bounded, expiring deferral with
follow-up work; neither an exception nor a retirement plan grants delivery or removal authority.

Provider- and affected-consumer-repository participants can turn that shared reason into ordered ordinary proposal tasks in
their own repository. A contribution freezes its inventory audience, exact legacy and supported
replacement contracts, acceptance criteria, required documentation changes, rollout stage, exact base,
and earlier work dependencies. Human or approved-agent assignment then reuses the normal task session,
workspace, fork, pull, review, and merge controls; an agent session cannot start until every selected
predecessor contribution merged at its current context. Plan reads derive visible task and pull progress,
but hide work in consumer repositories the reader cannot currently access. A participant who discovers
an omitted consumer may report exact commit, existing paths, evidence, and impact; the report blocks
retirement for reassessment and does not give the provider mutation authority in that repository.

Each plan can retain immutable coexistence candidates bound to its exact provider release, schema
and configuration paths, consumer revisions, and bounded environment. A complete matrix requires
old-only, dual-support, replacement, rollback, and named journey checks. Check status, sanitized logs,
artifact digests, duration, and resource-derived cost come only from exact-command outcomes retained by
the submitting participant's revision-matched workspace. Later proof supersedes without deleting failed
history. Reads invalidate only checks whose selected Git blobs changed, so unrelated history retains proof.

Affected owners append windowed usage observations against their frozen consumer revision. The public
plan preserves measured residual calls, explicit inaccessible or unmeasured use, acknowledgements, and
superseded observations while redacting repositories the reader cannot access. Removal readiness requires
every current matrix row to pass and every audience to have an owner-acknowledged measured zero-old-use
observation. These records grant no workspace, telemetry, consumer, Git, release, deployment, environment,
or removal authority.

After readiness and approvals remain satisfied, an authorized human capability owner may open a controlled
removal execution against that exact candidate. Its ordered public stage reports cite the ordinary merge queue,
release, schema or infrastructure migration, documentation, and protected-deployment resources that actually
deliver the change, and show remaining old use, health, controller, rollback boundary, and next action.
Residual use, degraded health, failed delivery, or an unexpected consumer pauses the execution. An explicit
compatibility restore retains the attempt and its provenance instead of pretending removal succeeded.
If capability ownership changes during an active execution, a current human owner explicitly transfers its
CAS-versioned controller; history retains the predecessor, successor, transferring owner, reason, and time so
the removal cannot be stranded or silently taken over.

The final stage awaits separate revision- and path-bound proof for obsolete code, flags, data, credentials,
telemetry, documentation, and policy exceptions. Only complete proof marks removal as a verified product
outcome. Every proof names the provider repository and a revision retained as succeeded delivery for that
execution. The candidate freezes every required obsolete surface by category, provider path, pre-removal
revision/blob, and expected removed or replaced outcome. Requirements can name only paths selected by the
frozen capability revision, every selected path must be covered, and duplicate category/path requirements are
rejected at admission. Completion accounts for every requirement ID
exactly once and derives its canonical digest from the absent or changed path at the delivered revision;
unrelated retained files cannot substitute for removal evidence.
These coordination records do not grant Git, queue, review, release, deployment, environment,
documentation, schema, infrastructure, credential, telemetry, or destructive authority.

The connected `capability-retirement-journey.spec.ts` browser/API/Git journey carries a released
legacy capability from exact inventory and independently owned consumer acknowledgement through
human- and agent-authored migration work, failed-first coexistence proof, superseded residual-use
evidence, staged disablement, and category-complete cleanup. It retains a missed acknowledgement,
a changed consumer signal, a hidden dependent, a compatibility restore before destructive removal,
and a late post-disable regression, then proves that each condition pauses delivery until corrected.
The final workspace trail connects the original release, decisions and citations, bounded work,
checks, observed use, delivery revisions, rollback authority, and verified absent artifacts.

## Revision-exact security confidence

Repository owners and organization owners publish scoped security requirements at
`/repositories/{id}/security-requirements` and
`/repositories/{id}/organization-security-requirements`. Requirements select branches,
components, protected assets, risk classes, and paths and require a current threat-model revision,
a passing owner-reviewed security scenario, named control-owner acknowledgements, or resolution of
scoped findings. Component, asset, and risk labels require an explicit repository-path mapping so
they cannot become global delivery selectors. Repository policy takes precedence over organization fallback policy.

Pull, release, and deployment `/security-confidence` reads derive an exact-revision matrix from the
existing threat-model, security-scenario, and private finding ledgers. Changed paths invalidate only
intersecting threat and scenario proof; unrelated changes retain evidence. Finding IDs outside the
reader's audience project as `restricted` while still blocking delivery. Pull merge and integration
queue readiness consume the same matrix.

Named requirement owners may create exact-revision exceptions lasting no more than 30 days. Each
exception freezes its selector, rationale, attribution, and an existing issue or proposal follow-up.
Deployment security signals retain only sanitized digest metadata against the exact release,
deployment, environment, requirement, assumptions, and controls. Violated assumptions or failed
controls can link to a repository-scoped private incident, security advisory, or governed repair;
the signal grants no production, disclosure, Git, review, queue, release, deployment, or agent
authority. Records default beneath `$SECURITY_CONFIDENCE_STORAGE_ROOT` (`security-confidence`).

The connected `security-assurance-journey.spec.ts` browser/API/Git journey carries a new privileged
credential-rotation workflow from versioned expectations and bounded agent uncertainty through
owner-reviewed threat analysis, safe executable replay, a confidential governed repair, ordinary review,
integration, release, and staged deployment. It retains an unsafe test, false positive, inaccessible
citation, stale model, failed first repair, and rejected exception before a sanitized changed-assumption
signal links private follow-up work. A repair scenario remains tied to the original finding's model revision
and may run only at that modeled commit or a verified Git descendant. Merge/release/deployment matrices reuse
the proof only when each policy-selected path resolves to identical content at the evidence and delivery
commits; changing any selected blob invalidates it.

## Governed security repairs

Security findings at `/repositories/{id}/security-findings` freeze an exact threat-model path and
candidate, keep permitted evidence inside an explicit participant audience, and retain owner-authored
classifications. Only confirmed findings can create ordinary human- or agent-assigned proposal work.
Completion links the task's exact pull only after a failed abuse attempt is retained for the affected
base and an owner-reviewed scenario passes on the repair commit. Finding records grant no Git,
workspace, review, merge, secret, deployment, or environment authority and non-repair resolutions
remain attributable in append-only history.

## Revision-exact release confidence

Repository participants publish versioned release-confidence requirements at
`/repositories/{id}/quality-requirements`. Each requirement selects branches, journeys, risk
classes, locales, platforms, releases, and changed paths, names accountable owners, and requires
one of reusable scenario coverage, closed exploratory sign-off, or a retained test result. Attempts
at `/quality-attempts` must resolve to the exact repository scenario, closed session, or check run;
they retain passed, failed, flaky, gap, and quarantined outcomes without rewriting earlier attempts.

Pull and release `quality-confidence` reads produce a revision-exact matrix. Evidence from an older
candidate remains usable only when its declared code and dependency paths do not intersect the new
change; affected proof becomes an explicit stale attempt. The same pull matrix is embedded in merge
readiness, so direct merges and integration queues remain blocked by current failures, flakes,
quarantines, and gaps. Owner-only overrides freeze a narrow selector and candidate revision, expire
within 30 days, retain rationale and attribution, and require an existing follow-up work reference.

Post-release sampled scenario signals append against the exact release commit and established named
environment. A failing sample reopens the linked requirement in the retained matrix; neither the
signal reporter nor any quality record gains check execution, environment, Git, review, merge,
queue, release, or deployment authority. Records default beneath
`$RELEASE_CONFIDENCE_STORAGE_ROOT` (`release-confidence`).

## Versioned quality plans

Repository quality intent is published at `/repositories/{id}/quality-plans` and rendered in the
`/repositories/{id}/quality` workspace. Complete immutable revisions cover repository, release,
journey, and interface scopes and retain the risks, expected behavior, test levels, representative
privacy-safe data, coverage goals, supported environments, owners, schedules, and release thresholds
the project intends to protect. Requirements trace to issue, decision, design, accessibility,
privacy, performance, or reliability sources and may reference existing automated or manual evidence.

Reads derive attributable missing-owner and missing-evidence states, explicit contradictory
requirements, claims without an observable verification method, and expired or seven-day-expiring
exceptions. Publication validates every named owner, release judge, and exception grantor as a
current repository participant inside the repository mutation boundary. The ledger defaults beneath
`$QUALITY_PLAN_STORAGE_ROOT` (`quality-plans`) and grants no check execution, preview, release,
environment, merge, or evidence authority.

## Versioned security expectations

Authorized repository collaborators publish immutable security-intent revisions at
`/repositories/{id}/security-expectations` and work with them at
`/repositories/{id}/security`. A complete revision scopes repositories, services, interfaces,
packages, extensions, environments, or user journeys and connects protected assets to explicit
trust boundaries, actor trust and capabilities, abuse cases, required preventive/detective/
response/recovery controls, accountable owners, severity response/release policy, and bounded
exceptions. Links preserve the relationship to design, privacy, infrastructure, API, quality, and
release commitments without copying or granting authority over those resources.

Every reference inside the asset/boundary/actor/abuse/control graph is validated before an
immutable version is retained, and compare-and-swap successors preserve the complete history.
Reads derive attributable blocking diagnostics for missing expectation, asset, or abuse-case
owners; explicitly contradictory boundaries; unsupported or unevidenced planned guarantees; and
expired exceptions. Exceptions expiring within seven days remain attributable warnings. Named
owners and exception grantors must be current repository participants. Records default beneath
`$SECURITY_EXPECTATION_STORAGE_ROOT` (`security-expectations`) and grant no Git, review, merge,
release, deployment, environment, control-execution, or linked-resource authority.

## Revision-bound change threat models

Collaborators open inspectable attack analysis at `/repositories/{id}/threat-models` from an exact
design-proposal, pull, API-contract, durable-schema, infrastructure-plan, or product-experiment
revision. A complete model connects entry points, privileges, data flows, dependencies, attacker
goals, abuse paths, mitigations, residual risk, alternatives, assumptions, cited evidence, and
affected owners. The repository `/security` workspace renders this graph beside versioned security
expectations so teams can compare safer designs before implementation hardens assumptions.

Human collaborators and agents whose credential matches the exact source pull task and branch can append cited findings, challenges,
alternative comparisons, and acknowledgement requests to the current model revision. Contributions
can cite only evidence retained as accessible. Reads reauthorize citation metadata through the
currently visible governed source; revoked or unresolvable evidence projects as a gap with its kind,
resource, revision, and summary redacted. Only a named human owner may acknowledge or request changes.
Citation authorization binds ID, kind, resource, and revision separately across model history;
events citing anything no longer permitted project without their contributor-controlled body.
Publication freezes server-derived source-snapshot fingerprints, and reads re-resolve them to project
source, architecture, trust-boundary, and dependency movement as stale without rewriting the
retained analysis. Records default beneath `$THREAT_MODEL_STORAGE_ROOT` (`threat-models`) and grant
no source, Git, review, merge, release, deployment, environment, infrastructure, experiment, or
general agent authority.

## Executable security scenarios

Threat-model abuse paths become reviewed, immutable evidence specifications at
`/repositories/{id}/security-scenarios` and are visible in the repository `/security` workspace.
Each scenario freezes the exact model version, path, mitigations, candidate commit,
candidate-defined check, attacker preconditions, bounded capabilities, safe fixture digests,
actions, and observable containment, detection, and recovery criteria. Human owners named by the
threat model review scenarios; repository-bound agents act only for their exact source pull.

Attempts resolve through an isolated workspace or successful current preview at the exact candidate.
Workspace commands are projected from retained outcomes; sanitized logs, artifact metadata, coverage,
gaps, costs, and provenance remain attached. Secret-shaped, destructive, production/user-data,
hidden-fixture, stale, and over-budget work is rejected. `unsafe` and `not_reproducible` are durable
results with required reasons. Records default beneath `$SECURITY_SCENARIO_STORAGE_ROOT`
(`security-scenarios`) and grant no execution, workspace, preview, secret, data, environment, Git,
review, merge, release, or deployment authority.

## Reusable test scenarios

The quality workspace also publishes immutable executable specifications through
`/repositories/{id}/test-scenarios`. A scenario names its exact issue, reproduction, design,
API-contract, documentation, or journey rationale and spells out parameters, preconditions,
actions, assertions, environments, cases, and assumptions. Another contributor can therefore see
both what the test proves and the command that reruns it without interpreting an opaque script.

Tests and fixtures are proposed through ordinary repository branches (and optionally an exact pull
or bounded workspace). Publication resolves the branch commit, every test path, every optional source
path, and each fixture SHA-256 from Git. Only synthetic, anonymized, or public fixture classifications
are reusable; resolved fixture bytes are scanned for credential-shaped content but are not copied into
this ledger. Source IDs must resolve through their repository-owned issue, replay, design, contract,
documentation, or quality-plan journey record. Generated scenarios identify their human
or agent authoring mode, framework, provenance, and assumptions. Records default beneath
`$TEST_SCENARIO_STORAGE_ROOT` (`test-scenarios`) and grant no Git, workspace, test-execution, review,
merge, environment, or data authority.

The cited commit must also be source-owned evidence, not merely another repository commit. Issues bind
through retained triage/reproduction/repair/delivery revisions, implemented designs through their base
revision, and journey scopes through an explicit quality-plan `source_revision`; replay, contract, and
documentation sources already retain their exact commit directly.

## Bounded exploratory sessions

Authorized repository participants open exact-revision sessions at
`/repositories/{id}/exploratory-sessions` from a pull preview, release candidate, issue, or quality
plan. Each session freezes its explicit human audience, maximum-24-hour expiry, privacy-safe test-data
classes, cost and agent-action ceilings, allowed actions, and risk-based human or approved-agent
charters. Human assignees and audience members must be repository participants when the session is
created; an agent can append only under its exact charter and cannot broaden the session's actions.
Control and finding-decision actions are separately chartered from observation, and every reproduced
event or decided finding must resolve from the same session's retained timeline.
Human and agent actions both require an exact assignee/action match; explicit-audience participants
who are not assigned a charter may guide the investigation but cannot exercise its actions.

The compare-and-swap shared timeline retains attributable observations, guidance, pause/resume/close
controls, reproductions, classifications, discards, exercised routes, sanitized input descriptions,
commands, coverage, uncertainty, and digest-addressed screenshots, traces, recordings, logs, or command
outputs. Credential-shaped metadata is rejected and artifact bytes are not copied. Pull movement and
unavailable source records mark retained observations stale on reads without rewriting history. The
ledger defaults beneath `$EXPLORATORY_SESSION_STORAGE_ROOT` (`exploratory-sessions`) and grants no
preview, workspace, runtime, environment, Git, deployment, data, or agent authority.

Confirmed reproduced bugs can enter ordinary governed repair directly from their finding. The handoff
reserves a retry-safe identity before publication, freezes the exact affected candidate, selected
audience-permitted timeline evidence, minimized reproduction, acceptance criteria, assignee, and optional
quality-plan requirements, then creates a linked repository issue and human- or agent-owned proposal task.
The task continues through ordinary branch, pull, review, check, and merge controls. Once its exact pull
publishes a reusable issue-sourced test scenario, the finding can link that scenario back only when its
pull task, candidate commit, issue, and frozen quality-plan requirements all match. The session therefore
retains bidirectional finding/issue/task/pull/commit/scenario provenance. `flaky`, `duplicate`,
`environment_specific`, and `not_reproducible` decisions remain explicit timeline classifications and
cannot silently enter the confirmed-bug repair path.
Once a repair identity is reserved, its finding disposition is frozen while ordinary issue/task
publication is pending. This prevents concurrent reclassification from orphaning actionable work;
exact publication retries reconcile the same identities. Later evidence may append a new classification
after the repair link is durable without rewriting either history.

## Versioned interface systems

Reusable product language is repository state, not an untracked style-guide
artifact. `GET /repositories/{id}/interface-systems` exposes every visible
system with immutable revisions and diagnostics; participants publish to that
collection and publish compare-and-swap successors at `/{system_id}/revisions`.
The repository `/interface-system` workspace renders examples and exposes usage,
accessibility, localization, responsive, adoption, theme, ownership,
implementation, history, and provenance context.

## Revision-exact interface verification

Pull requests expose repository-defined interface evidence at
`/repositories/{id}/pulls/{pull_id}/interface-checks`. A publication must bind the
current candidate, a successful isolated preview, the exact digest of its check
definition in that candidate, and the current accepted design proposal revision.
It records viewport, theme, content-length, locale, interaction-state, and
assistive-technology context alongside visual and behavioral differences,
recordings and other digest-addressed artifacts, coverage, performance budgets,
and affected requirements. Collaborators classify each difference once as an
intentional change, regression, or false positive with an attributable rationale.

Reads project current and stale evidence separately. Candidate movement or an
accepted design successor therefore invalidates only evidence and classifications
tied to the earlier code or design revision while retaining the audit trail. This
ledger reports bounded preview outcomes and grants
no workspace, Git, design-approval, review, merge, deployment, or environment
authority. Records default beneath `$INTERFACE_CHECK_STORAGE_ROOT`
(`interface-checks`).

## Governed design acceptance and evolution

Repositories and organizations publish scoped acceptance policy through their
`/design-acceptance-policies` collections. A policy selects components, journeys,
paths, or risk classes and names the exact people who may act as design owner,
accessibility, content, localization, or invited-user approvers. Pull decisions
freeze the policy version and candidate commit; candidate movement makes them
stale. Only the policy creator may issue an exception, bounded by the policy's
maximum (never more than 30 days), and readiness warns during its final seven days.

Pull `/design-readiness` combines those decisions with current interface-check
differences and interface-system implementation diagnostics. It is also embedded
in ordinary merge readiness, where unresolved deviations, known regressions,
obsolete component use, and stale preview evidence block alongside normal reviews
and repository checks. Release `/design-readiness` re-evaluates the exact release
commit and changed paths without manufacturing new acceptance. Interface-system
changes can seed repository-owned migration and documentation tasks; release
feedback or observed regressions can seed linked repairs. Both continue through
ordinary task, Git, review, check, merge, release, and deployment controls. Records
default beneath `$DESIGN_GOVERNANCE_STORAGE_ROOT` (`design-governance`).

The connected `interface-design-journey.spec.ts` browser/API/Git journey starts
with invited-user release feedback, compares a designer prototype with an
agent-assisted alternative, and retains a stale incomplete revision before owner
acceptance. Human and bounded-agent authorship then meet in ordinary task, Git,
pull, and review controls. Exact previews cover responsive, keyboard,
screen-reader, long-content, and localized contexts; stale evidence, a visual
regression, and a rejected implementation deviation remain visible before the
corrected evidence receives design, accessibility, localization, and invited-user
acceptance. The trail continues through merge, release, staged deployment,
measured follow-up feedback, a changed design token, and ordinary downstream
migration and repair work.

Each release freezes the source revision and changed paths for every included
pull. Release design readiness consumes that immutable snapshot rather than the
pull's later synchronized state. Within one pull revision, the latest durable
interface-check result for a definition and journey is authoritative, allowing a
successful rerun to supersede an earlier failed attempt while retaining both
records and preserving independent definitions that share the journey.

The API resolves the named release itself, derives its retained display version,
requires the exact release commit to exist, and verifies every component,
interaction, and content source path in that snapshot. Current implementations
for the defining repository must name that same release commit. Definitions in
another current system are reported as conflicts; owner gaps and stale, unknown,
or unsupported consumers remain visible. These records grant no release, Git,
review, merge, deployment, or downstream authority.

## Connected production debugging

The `debugging-journey.spec.ts` browser/API/Git journey carries an intermittent released-user failure through
an audience-bound debugging workspace, denied and narrowed trace consent, visible privacy transformations,
human/agent cited diagnosis, a revoked investigation, a failed replay and deterministic privacy-safe refinement,
and a failed then corrected repair. The reviewed merge reruns the frozen replay and ordinary required check at
the exact integration revision before release, staged deployment, production-signal validation, and web inspection.
The retained trail keeps user impact, gaps, costs, authorship, authority, delivery, and outcome connected without
granting the debugging workspace runtime, Git, review, merge, release, or deployment authority.

## Entrypoints

- `apps/web` — Next.js frontend. Starts at `src/app/page.tsx`; routes are
  file-based under `src/app`. `bun dev` from the repo root.
- `apps/api` — Go HTTP API. Starts at `main.go`, where routes are registered on
  the mux. `bun run dev:api` from the repo root, serves on `:8080`.

The supported consumer contract, including authentication, stable error
shapes, validation, and collection pagination, is documented in [API.md](API.md).
Consumers should use that HTTP boundary rather than reading storage roots.

## Durable-state contracts

Repository participants publish immutable persistent-store schemas through
`/repositories/{id}/durable-schemas`; the web workspace is
`/repositories/{id}/durable-state`. Database, queue, index, object-store,
event-log, cache, and other definitions bind ownership, compatibility guarantees,
retention and privacy commitments, service/environment links, and a definition
path whose exact contents resolve at the merge commit of the merged pull that reviewed them.

Migration plans compare two exact published versions and must cite an existing
repository pull request or affected technical decision. Each plan classifies
reads, writes, backfills, and explicitly destructive operations, names affected
owners and consumers, sequences measurable steps and participant approvals, and
states operation-specific and overall rollback limits. Append-only,
compare-and-swap events retain later approval and execution discussion without
rewriting the plan; approval events are accepted only from the named approver for
their exact step. Contracts and plans make deployment review inspectable but
grant no Git, deployment, environment, or persistent-store authority. Records
default beneath `$DURABLE_SCHEMA_STORAGE_ROOT` (`durable-schemas`).

Each migration may publish ordered repository-owned work for schema changes,
compatibility code, backfills, verification, and cleanup. The work link creates an
ordinary assigned proposal task at an exact target-repository base; normal task
sessions, bounded workspaces, pull publication, review, checks, and merge remain the
execution boundary. Its review context freezes old/new reader and writer behavior,
rollout flags, idempotency, transformations, ownership, and rollback assumptions.
Cross-repository agent work cannot start until its linked predecessors merge. Target
repository visibility is rechecked on reads, so the link does not copy restricted
schema definitions, privacy commitments, data samples, or access into another project.

Before authoritative state changes, participants can freeze a migration rehearsal against
the exact application commit, schema versions, migration version, dependency digests, and
synthetic or explicitly privacy-preserving representative dataset shape. Repository-defined
upgrade, dual-read/write, backfill, validation, rollback, and failure-injection commands run
in ordinary bounded workspaces. Immutable runs retain sanitized per-check logs, counts,
invariants, timing, artifacts, cost, and attestations; attributable notes let humans and
scoped agents investigate failures without copying production data. Each check declares the
revision inputs it depends on so later candidates can replace only affected proof. Rehearsal
records are review evidence, not deployment or data-store authority.

After declared approvals and a passing rehearsal, participants can open one governed production
execution against an existing release and established environment. Its fixed expand, deploy,
backfill, cutover, and contract phases expose the active revision, controller, compatibility window,
progress, lag, invariants, service health, privacy constraints, cost, blockers, and next actions.
Compare-and-swap controls pause, resume, throttle, advance, or abort work only while its frozen
rollback boundary remains reversible. Deployment proof resolves through the ordinary environment
promotion store; the migration record never receives credentials or performs production operations.
Agents can only report an exact phase and step delegated to their authenticated identity. Their
step evidence is retained separately and cannot populate or advance controller-owned phase readiness.
Every current-phase delegation must nevertheless have a latest complete, healthy, invariant-backed,
unblocked report before the human controller can advance.

Operational failures do not erase or restart an execution. Failed invariants, service regressions,
approval revocation, conflicting writes, capacity exhaustion, and interrupted backfills append their
bounded evidence and pause at a declared safety point. Recovery records are append-only and offer an
idempotent retry, an attested recovery-point restore, an existing-release traffic rollback before the
contract phase, or a connection to ordinary assigned human/agent migration work. Once an execution
finishes, its declared observation period must elapse and every candidate-schema owner must approve
retirement. The immutable completion then accounts for compatibility code and obsolete fields removed,
irreversible choices, exceptions, cost, retained and changed data, verified deletion, and current schema
version in every established repository environment.
Idempotent retry alone applies only to failed invariants and interrupted backfills. Service regressions,
capacity exhaustion, and conflicting writes require a remediation attestation, attested restore, or
compatibility-window traffic rollback before resume; merely opening linked repair work does not resume.

The connected `durable-state-journey.spec.ts` journey carries a reviewed breaking database revision
through human- and agent-owned compatibility work, a failed and then passing synthetic rehearsal,
real release and deployment evidence, old-writer fencing, an interrupted backfill, an invariant breach,
and contract-phase rollback rejection. The web record then exposes every contained failure and recovery
beside the observation-gated owner approval, per-environment schema version, cost, retained/changed data,
and verified obsolete-field deletion.

## Infrastructure as project state

Repository infrastructure lives at `/repositories/{id}/infrastructure` in both the API and web
application. Each immutable revision resolves to an exact repository commit and declares environments,
services, networks, identities, data stores, compute, and external dependencies together with owners,
providers, configuration boundaries, dependencies, cost/capacity constraints, and operational
commitments. Release and environment links resolve against existing repository records.

Append-only observations retain only permitted, sanitized provider state at an exact definition and
provider revision. Participant-only observation identity and summaries are redacted on public reads;
secret values and credential-shaped text are rejected. Derived diagnostics make inaccessible providers,
unmanaged discoveries, stale or missing observations, conflicting ownership, and secret-backed
configuration explicit. This inventory grants no cloud, deployment, environment, credential, or
infrastructure execution authority. Records default beneath `$INFRASTRUCTURE_STORAGE_ROOT`
(`infrastructure`).

Open pulls retain immutable infrastructure plans beneath their review API. Each plan freezes the exact
pull source, declaration and permitted observation fingerprint, candidate resources, and policy files,
then derives ordered create/change/replace/destroy operations, affected owners, risks, mitigations,
expected policy effects, and rollback limits. Humans and repository-bound read-only agents append
assumptions and impact analysis; only an affected human owner can acknowledge. Reads recheck every
source, provider observation, definition, and policy input, marking drift stale and suppressing prior
acknowledgements. These records remain review evidence without operational authority.

Authorized collaborators can attach isolated or policy-approved ephemeral rehearsals to a current
plan. Each freezes expiring provider scope limited to changed resources, synthetic or permitted state,
and repository checks for provisioning, connectivity, access, policy, service journeys, failure, cost,
teardown, and recovery. Publishing a run names only the exact-candidate bounded workspace and checks;
the platform derives sanitized logs, timing, artifacts, resource graphs, attestations, and agent
actions. Destructive effects remain explicitly unsupported, stale plans reject evidence, and no
production or provider authority is conferred.

Once that exact pull is merged, an authorized repository operator can create an authoritative apply at
`/repositories/{id}/infrastructure-executions`. Admission re-resolves the plan and policy files, requires
every affected owner acknowledgement and a latest passing rehearsal, checks the established deployment
environment, and caps the requested budget by reviewed resource limits. The immutable identity binds both
reviewed and merge commits, candidate digest, environment policy, rehearsal, controller, and a short-lived
resource/step/action credential scope.

The selected rehearsal must either name that authoritative environment directly or carry a
`policy_approved_ephemeral` scope whose frozen policy approval exactly matches the execution's environment-policy
reference. An isolated preview rehearsal without that explicit equivalence cannot admit production execution.

Compare-and-swap reports retain dependency-ordered progress, sanitized provider responses, health, cost,
blockers, next actions, controller, and declared safety points. Failure, degraded health, or a blocker pauses
without erasing evidence; the human controller can pause, resume after remediation evidence, or cancel at a
safety point. Agents may report only explicitly delegated non-destructive steps and receive no secret,
acknowledgement, destructive, environment, or unrelated provider authority. The web infrastructure workspace
exposes the same start, report, pause, resume, and cancel flow.

Terminal and safety-paused applies can be assessed against the resource presence and declared service, security,
privacy, cost, and continuity measures frozen from the approved candidate. Assessments retain exact sanitized
provider revisions, partial results, unmanaged resources, and failed cleanup. Convergence is derived only for a
succeeded apply with every measure passing and complete cleanup; unknown, partial, failed, paused, or cancelled work
remains explicitly divergent.

Current participants can append permission-aware monitoring runs after apply. Each run records whether provider
visibility was granted, partial, or denied, and retains bounded findings for configuration drift, unmanaged changes,
failed cleanup, expiring credentials, and provider loss with an available sanitized cause. Findings can link an
accountable owner and ordinary incident, exception, human/agent repair, reviewed adoption pull/proposal, or
declared-state restoration task. Adoption and restoration remain normal policy-governed review paths: monitoring
does not overwrite the external event or confer provider, environment, policy, review, or merge authority.

The connected `infrastructure-journey.spec.ts` browser/API/Git journey carries application and infrastructure
changes through exact planning by security and service owners, scoped-agent analysis, failed and passing isolated
teardown, protected merge, governed apply, partial and converged verification, out-of-band drift, and a reviewed
repair. Rejected wrong-owner approval, stale evidence, destructive delegation, provider failure, budget overrun,
undelegated or revoked credentials, and incomplete cleanup remain visible containment evidence rather than hidden
provider-side state.

Support guidance can be proven in ordinary revision-pinned development workspaces. Immutable attempts
bind an exact cited answer revision to sanitized thread inputs, declared environment, commands and
outputs, artifacts, cost, and result; fresh-workspace reruns preserve the original record and reads expose
stale provenance without retaining credentials or private machine state.

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

`agent-development-journey.spec.ts` connects the repository-owned development primitives into one lifecycle.
A team reviews human- and agent-authored behavior contracts, compares protected candidate evidence with a
baseline, contains leaked scenarios, prohibited actions, evaluator disagreement and cost overrun, and pilots the
exact candidate under explicit consent. Five independent current participants approve an attested release before
bounded work; sanitized production feedback then drives rollback, a reproduced model/contract repair, fresh
evaluation and pilot acceptance, a new attestation, ordinary review and merge, and a separately consented rollout.
The test assigns isolated Playwright roots to agent projects, candidates, pilots, and releases so no prior run can
supply evidence or authority.

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

## API contracts

Repository participants publish versioned service interfaces through
`/repositories/{id}/api-contracts`; the web workspace uses the same path. Each revision freezes the
exact merge commit and merged pull that were reviewed, plus optional exact-release provenance,
operations, schemas, errors, authentication, environments, limits, owners, stability, support
terms, compatibility promises, known gaps, and typed source, release, documentation, data-use, and
support links. The reviewed definition artifact is OpenAPI 3.x or Swagger 2.0 JSON with a paths
object; publication parses that exact commit blob and rejects unrelated files, detached definitions,
null or unusable path items, operations missing matching methods/responses, and release claims that
do not resolve to the reviewed commit. Reads retain every revision for comparison and derive unreleased
implementation, non-available environment, known-gap, unavailable-release, and default-branch
documentation-staleness diagnostics. Records default beneath `$API_CONTRACT_STORAGE_ROOT`
(`api-contracts`).

Authenticated consumers register independently owned applications against one exact contract revision
through `/repositories/{id}/api-contracts/{contract_id}/applications` and the linked `/integrations`
workspace. Requests select contract-declared environments and operation IDs; repository participants
approve a subset with an expiry or retain an attributable denial. Approved owners rotate short-lived
application secrets whose hashes alone persist. Rotation retires the predecessor; revocation, reported
exposure, expiry, and ownership transfer fail closed. A transfer revokes every secret and resets consent.
The credential authenticates only an inspectable synthetic `/sandbox` request/response pair with the
frozen quota and deterministic rate-limit, timeout, or server-error simulation. It never calls a declared
base URL or reads production data, and grants no account, repository, Git, deployment, or environment access.

Approved applications also open credential-free integration work from the same workspace. Human- or
agent-owned task, session, and workspace records freeze the exact consumer commit plus the reviewed contract,
SDK/example links, approved sandbox operations, and synthetic configuration. Producer and consumer pull
candidates retain their exact source commits and separately owned scenarios; append-only evidence keeps only
bounded sanitized request/response/log summaries, artifact checksums, coverage, cost, and authorship. It is
review context only and never carries credentials, artifact payloads, Git authority, checks, or merge permission.

Contract migrations beneath `/api-contracts/{contract_id}/migrations` connect a published predecessor and
candidate to an existing interface evolution plan. The producer classifies changes and defines ordered dual-run,
migration, and sunset stages with evidence and remaining-traffic thresholds; affected applications, owners, and
their existing integration work are discovered from retained records rather than an external inventory. Consumers
acknowledge work, attest an exact candidate only when every declared producer and consumer scenario passes, or
request a reasoned exception capped at 90 days. Readiness is derived anew from application access, candidate
evidence, exception expiry, and the latest exact-version operational window. Revoked or unresponsive consumers,
failed tests, expired exceptions, and traffic above policy remain explicit retirement blockers. The migration
record grants no Git, task, agent, fork, release, deployment, or evolution authority.

The connected Playwright journey `api-delivery-journey.spec.ts` proves the complete producer-consumer loop:
reviewed contract publication, narrow synthetic access, credential-free agent integration and independent release,
sanitized shared failure diagnosis, breaking-version review and rollout, measured migration, and safe retirement.
It retains overbroad-scope denial, exposed-secret revocation and ownership recovery, stale documentation, failed
conformance, temporarily unavailable ownership, a bounded sunset exception, and zero old-version traffic as
distinct evidence instead of allowing any of them to silently widen authority or bypass retirement policy.

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

API applications retain permission-filtered operational windows for aggregate availability, latency,
quota, errors, schema conformance, and usage, pinned to an exact contract, provider release, and environment.
Shared investigations cite only visible evidence, admit explicitly invited read-only agents, reproduce only
approved synthetic requests without payload retention, and freeze confirmed ownership into ordinary governed
work references without revealing credentials, private usage, or unrelated consumers.

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

## Product behavior before code

Repository collaborators can open design proposals through
`/repositories/{id}/design-proposals` and evaluate them in the `/repositories/{id}/design`
workspace. Immutable revisions keep their originating feedback, issue, roadmap outcome,
accessibility finding, or pull request beside explicit goals, journeys, states, product language,
constraints, alternatives, success measures, affected components, uncertainty, citations, and
audience-scoped wireframes or prototypes.

Comments, questions, dissent, and grounded-agent evidence bind to the revision they evaluated.
Only named current participant owners can acknowledge the current revision or request changes.
Artifact bodies and interactions are projected only to their explicit audience; inaccessible
research remains a citation gap rather than becoming repository-visible content. Design proposals
are review context and grant no implementation, Git, review, release, deployment, environment,
research, or asset access.

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

Conflict analysis is available before anyone edits conflict markers. `GET
/repositories/{id}/pulls/{pull_id}/conflict-analysis` compares the pull's exact
adopted source with its retained target snapshot, or a retained queue attempt when
`candidate_id` is supplied. `GET /repositories/{id}/conflict-analysis` compares
the current tips named by `source_branch` and `target_branch`. Both use a
read-only three-way index rooted at the exact merge base and return textual,
structural, schema/interface, and independently detected same-symbol semantic
overlap. The evidence connects each side to matching pull requests, authors,
tasks, proposals, discussion, review decisions, acceptance criteria, ownership,
and affected required checks. Live movement never rewrites an earlier candidate
analysis: the response reports stale or incomplete inputs and leaves both Git
references unchanged.

An authorized contributor or maintainer can turn that immutable evidence into one durable
reconciliation environment with `POST /repositories/{id}/pulls/{pull_id}/conflict-workspaces`.
A caller-stable launch ID reconciles retries to one workspace. The launch freezes the exact base,
source and target commits, overlapping paths and symbols, affected checks, attributable owners,
and any incomplete evidence; it uses the target commit's reviewed `.vivarium/workspace.json` and
preloads a local Git bundle with the complete ancestry exposed as `conflict/source` and
`conflict/target`. The ordinary workspace surface supplies shared editing, presence, discussion,
renewable human or approved-agent control, reconnectable commands, and checkpoints.

The creator may invite only affected current human participants or an agent already approved for
the repository organization. Human invitations require explicit acceptance; an agent receives only
the separately selected file, command, or lifecycle lease and no ambient repository credential.
The frozen publication targets explain that source- and target-branch changes still require their
ordinary repository permissions, review, checks, and merge controls. The reconciliation workspace
therefore shares context and isolated compute without transferring either side's Git authority.

Inside it, `GET /workspaces/{workspace_id}/conflict-comparison?path=...` returns one retained
overlap at the exact merge base, source, target, and current proposed checkout, including bounded
contents and SHA-256 identities. Participants append compare-and-swap revision-grounded questions
and answers through `/conflict-questions`; each statement carries residual uncertainty and exact
side/revision/path citations. They propose resolutions through `/conflict-resolutions`, recording
whether named acceptance criteria, design decisions, migrations, and user behaviors are preserved
or intentionally changed and why. A proposal does not edit code. Apply and undo require the active
file-control lease and persist an intent before editing. If runtime execution or later persistence
is interrupted, the visible `applying` or `undoing` state lets an identical retry compare the
before and intended digests, finish only the missing work, and deduplicate change provenance.
Human, agent, and operator attribution comes from the original retained intent.
Finalization rechecks the running lifecycle and the original principal's live file-control lease;
stop, suspension, expiry, takeover, release, or lease expiry leaves the action pending and appends
no completion or change provenance. Every retry requires the exact control version frozen by the
intent; reacquiring a newer lease under the same principal does not revive revoked authority.
These records grant no merge, secret, environment, branch, review, check, or publication authority.

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
Evaluation suite scenarios can be derived from governed issues, support threads, tasks, incidents,
decisions, or sanitized prior sessions without copying the source record. Each authored case declares
its visibility, source version, synthetic or sanitized inputs, permitted context, expected outcomes,
quality/safety rubric, prohibited behavior, budget, uncertainty, required human judgment, license,
and either prohibited training use or a requirement for separate explicit consent. Protected cases
project only opaque identity and metadata to ordinary members; their prompt, answer set, rubric,
context, source identity, and candidate output remain available only to the human owner evaluator.
Suites and favorable results remain evidence rather than permission to train, broaden evaluation,
access source records, or participate in repository work.
Pull requests assemble a separate immutable release candidate from an exact source revision, one exact
agent-project behavior contract, component-level prompt/instruction/knowledge/tool/model digests, and selected
suite/judge revisions. Each isolated run declares its network and permitted simulated or real services plus
hard tool-action, cost, and latency ceilings. Per-attempt evidence retains task and policy outcomes, human
corrections, uncertainty, digest-addressed traces/outputs/artifacts, evaluator decisions, and statistical limits.
Review comparisons exclude contaminated attempts, expose nondeterminism, and compare only suite digests shared
with the selected baseline; a changed suite invalidates its own evidence rather than flattening every result.
Advancing the pull marks the old candidate stale without altering its evidence or creating release authority.
Owners can publish one exact candidate into a separate collaboration pilot before granting durable
participation. A pilot lasts at most 30 days and freezes selected owner-controlled repositories, participant
roles, task kinds, invitations, read/draft-only actions, and cost/action/minute budgets. Each invitee sees
their effective access, explicitly consents, delegates only matching work, and can append guidance, stop a
run, or submit candidate-revision-bound feedback comparing observed and expected outcomes with a correction.
The pull workspace shows live session state, spend, escalations, unsafe events, policy denials, and feedback.
Consent revocation, expiry, access loss, budget exhaustion, unsafe behavior, or a moved candidate pauses the
pilot while retaining its evidence. The pilot ledger defaults beneath `$AGENT_PILOT_STORAGE_ROOT`
(`agent-pilots`) and supplies no merge, publish, disclose, deploy, release, environment, secret, or
authoritative mutation capability.
Accepted candidates advance into a separate attested release ledger only after current evaluation,
domain review, pilot acceptance, data-policy, and resource approvals. A release binds the existing
organization agent and selected roles to exact contract and model/tool versions. Deployment separately freezes
credential scopes, budgets, operator terms, and a same-agent rollback target, so updates never inherit consent.
Sanitized signals and narrow, pause, rollback, private-finding, or repair controls remain append-only beneath
`$AGENT_RELEASE_STORAGE_ROOT` (`agent-releases`); organization participation remains authoritative for access.
Create and revise resolve each source in its repository-owned store while holding the author's current
suite-repository participation boundary. The record must belong to that repository and its declared
decimal revision must equal the current version; exploratory sessions also require explicit audience or
creator access. Missing, cross-repository, stale, or inaccessible provenance fails closed before ledger
persistence.
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

Current repository collaborators can classify an unresolved question as a defect, documentation gap,
missing example, compatibility problem, or product opportunity. `POST
/repositories/{id}/support-threads/{thread_id}/escalations` creates a linked ordinary issue,
documentation task, proposal, or dependency-ordered human/agent plan from the current default-branch base.
The immutable handoff keeps the user goal, affected version, permitted reproduction steps, and acceptance
criteria, while attachments and contact details remain inside the support audience. The support history links
back to the governed resource so the asker can follow its pulls, checks, previews, releases, and documentation
publication through existing project surfaces; escalation itself grants no repository, agent, review, or merge
authority. A pending escalation identity is persisted before publication and reused by issue, documentation-task,
and proposal creation, so a failed final thread write can reconcile the same governed resource on retry.
The record also freezes its initiating thread version; repeating that exact completed request after a lost response
or thread refresh returns the published link without creating or mutating work. Its initial default-branch base is
also frozen before publication, so retries after branch movement reconcile the original proposal or documentation
task rather than constructing a conflicting origin.

Successful proof can be published from `/support` as reusable project guidance. A solution freezes its exact
support question, answer revision, passing attempt, tested instructions, applicable versions, limitations,
audience, links, and credits. Public records require an entirely public evidence chain; participant-only records
remain repository-scoped. Repository search (`GET /repositories/{id}/support-solutions?q=...`) discovers active
solutions, and resolved links can point at documentation collections, exact package versions, releases, or the
current contributor guidance. Maintainer duplicate merges, obsolete archives, and newer-version revalidation
requests append attributed events and outcome notifications without changing the original discussion or proof.

## Code-connected production debugging

The repository `/debugging` workspace gives responders one durable starting point for observed runtime
behavior. A collaborator selects a governed issue, incident, support thread, deployment, service objective,
trace, or manual observation and freezes the affected release, exact source commit, established environment,
bounded time window, user journey, accountable owners, severity, package/configuration/infrastructure
revisions, audience, and sanitized evidence. Missing or inaccessible evidence is retained as an explicit gap.
Restricted workspaces are visible only to their creator and named readers; repository workspaces still follow
ordinary repository visibility. Compare-and-swap status and hypothesis events preserve authorship and history.
The workspace is collaboration context, not authority to query production, collect traces, deploy, or change code.

Participants can request a scoped probe for logs, traces, profiles, state snapshots, or a dynamic diagnostic
defined at the frozen source revision. Before an affected-environment owner can approve it, the request previews
the exact audience and data categories plus privacy/security filtering, retention, sampling, maximum cost and
service load, and a maximum 24-hour lifetime. Approval can only narrow those bounds. The requester then retains
sanitized, digest-addressed collection metadata with exact timing, collector provenance, transformations, gaps,
and actions; raw production payloads and reusable credentials are not accepted. Expiry, owner revocation, access
denial, overload, and partial capture terminate or narrow the probe and can never appear as complete evidence.

The same workspace turns selected evidence into an inspectable live explanation. Server-resolved citations join
permitted runtime captures to exact released symbols and line ranges, commits, frozen dependencies and
configuration, infrastructure definitions, affected-environment deployments, and known issues. Humans and
bounded read-only agents publish hypotheses, queries, findings, and explicit uncertainty; collaborators append
support, disputes, and staleness without replacing the original reasoning, and inaccessible evidence remains an
opaque blocked dependency. Citation-bound questions can be directed to named code, service, privacy, or security
owners. The web canvas refreshes active reasoning while purpose-only agents can be guided, paused, resumed, or
revoked. Their selected citation packet and short-lived credential carry no secret, production, observability,
deployment, Git, or mutation authority.

Collaborators can turn those permitted citations into immutable minimized replay scenarios without retaining raw
production state. Each scenario freezes the affected commit, digest-only synthetic or privacy-preserving input
shape, repository-defined experiment commands, invariants, dependencies, production differences, unsafe effects,
and explicit gaps; refinements name their parent instead of rewriting prior evidence. Attempts run through ordinary
isolated revision-exact development workspaces and retain their exact environment, command outputs, sanitized trace
metadata, invariant results, costs, and gaps. The server derives outcomes from retained commands and requires two
distinct passing workspaces before projecting `reproduced`; disagreement remains `nondeterministic`, and unsafe,
changed-revision, missing-dependency, or irreducible production conditions remain explicit evidence rather than a
successful reproduction.

### Design implementation handoff

Accepted product design is delivered through the ordinary proposal-task boundary. Publication freezes the
accepted design revision and current default-branch commit, creates ordered human/agent assignments, and embeds
audience-safe immutable prototype references plus exact state, content, component, breakpoint, acceptance, and
asset-provenance requirements in each task's reasoning context. Exact artifact payloads stay behind the design
proposal's explicit-audience projection. Proposal-task workspaces and pull requests therefore reuse existing access, agent,
review, and merge controls. Implementers append requirement-to-code/surface mappings and disclose deviations;
only named design owners decide those deviations, without gaining repository authority from design ownership.
