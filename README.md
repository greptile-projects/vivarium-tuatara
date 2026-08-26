# vivarium-tuatara

Monorepo.

```
apps/web    Next.js frontend (TypeScript, Tailwind)
apps/api    Go HTTP API
docs/       notes
```

Pull request review plans become current-coverage gates through
`GET /repositories/{id}/pulls/{pull_id}/review-readiness`; the same matrix is
embedded in ordinary merge readiness and shown on the pull page.

The connected `review-orchestration-journey.spec.ts` browser/API/stock-Git
journey proves planning, parallel human and read-only-agent specialist work,
finding disputes and recovery, exact-current verification, integration queue,
and merge as one public pull workflow. Agent review publication stays bounded
to an accepted exact area and grants no repository-write or approval authority.

Repository collaborators can publish immutable capacity-objective revisions at
`/repositories/{id}/capacity-objectives` and use the repository Capacity workspace
to agree on forecast demand, traffic shape, reliability, dependency, regional,
budget, lead-time, success, and rollback boundaries. Unsupported inputs and changing
commitments remain explicit rather than being treated as settled planning facts.

Supported capacity-test candidates can become phased programs at
`/repositories/{id}/capacity-plans`. The Capacity workspace records reservations,
quota/procurement dependencies, budgets, owners, decision points, and exit strategies,
then hands every phase to the ordinary proposal/task/session/workspace/pull pipeline
without granting spending, provider, repository, merge, or deployment authority.

Delivered plans progress through protected production environments at
`/repositories/{id}/capacity-rollouts`. The Capacity workspace joins exact deployed
revisions with usable headroom, load, reliability, dependency, regional, and cost evidence,
and gives authorized operators protected controls while deterministic risk signals contain
unsafe or wasteful scaling.

The `capacity-planning-journey.spec.ts` browser/API/stock-Git journey connects those
contracts from roadmap-linked demand through challenged modeling, bounded alternatives,
ordinary reviewed delivery, protected deployment, production containment, correction,
and verified headroom, reliability, and cost.

Repository writers publish immutable urgent-response coverage at
`/repositories/{id}/response-policies` and use the Response coverage workspace to map
signals and severities to teams, skills, targets, escalation, audiences, incident
criteria, and explicit authority boundaries. Coverage, ownership, skill, timing, and
exception gaps remain visible before an alert arrives.
The same workspace publishes policy-bound response rotations with time zones, availability,
qualifications, backups, workload and absence constraints; responders can acknowledge duty
and explicitly accept context-preserving swaps, delegations, and overrides. Membership,
schedule, qualification, workload, and handoff gaps remain actionable without minting access.
Repository-defined revision-bound signals become correlated alerts at
`/repositories/{id}/response-alerts`. Exact active-policy routing, duty ownership, permitted
evidence, uncertainty, user impact, response deadlines, delivery attempts, and explicit human
acknowledgement are visible in Response coverage and the actionable inbox; suppression,
maintenance, stale or inaccessible evidence, policy movement, and delivery failure remain gaps.
Routed responders use the alert's shared workspace to classify and steer response, invite or
reassign current owners, retain exact operational context and observations, run approved
read-only diagnostics, delegate a budgeted read-only agent investigation, and connect a qualifying
alert to an ordinary incident. The workspace never grants mitigation or environment authority.
Repository owners use the same surface to inspect consent-aware response outcomes, missed targets,
noise, handoffs, escalation, responder interruption, agent cost, incidents, and user results. They
can retain attributable reviews, pause noisy or unsafe routing, activate only declared backups, and
open ordinary human- or agent-owned improvement work without changing policy or operational authority.
The `on-call-coordination-journey.spec.ts` browser/API/stock-Git journey connects these contracts
for a released service: active primary/backup duty receives an exact signal, a human responder works
with a bounded agent and dependency owner, authorized mitigation and an accepted shift handoff retain
context, a severe recurrence becomes an incident, and ordinary reviewed work improves the runbook.
Duplicate, missed, absent, noisy, failed-delivery, revoked-access, and over-budget paths remain visible.

Repository writers publish immutable operational procedures at
`/repositories/{id}/runbooks` and use the Runbooks workspace to make service,
environment, dependency, and signal response knowledge reviewable before an emergency.
Every diagnostic, action, decision, and communication step explains its purpose,
evidence, rollback, ownership, skills, references, and effective authority; unsafe,
inaccessible, secret-bearing, unreviewed, unapproved, and policy-conflicting inputs
remain attributable gaps without granting operational authority.
Current participants and exact repository-bound agents can rehearse a selected immutable
revision in isolated or exactly policy-approved environments. The workspace retains
representative inputs, branches, commands, outputs, timing, artifacts, cost, permissions,
manual gaps, and simulated/excluded change effects, while revision, resource, policy, and
credential movement makes obsolete proof visibly stale.
Responders can submit exact live context for explained runbook recommendations, explicitly
choose a matching immutable revision, and retain a durable ready or blocked execution with
its source timeline, audience, evidence, signal window, releases, environment snapshot,
trusted precondition decisions, and current access. Caller assertions cannot establish
readiness; ambiguity, staleness, duplicate ready launches, unverified procedures, and
unavailable authority remain visible rather than starting work, while corrected attempts
preserve earlier blocked audit records.
Ready executions become shared guided-response records in the same workspace. Participants
can join and discuss while one explicit controller steers exact steps through version-bound,
caller-stable actions; approvals, evidence, decisions, cost, health, blockers, rollback, and
predicted next work remain visible as immutable receipts. Human handoff and bounded agent
delegation preserve separation of duties without granting operational access.
Completed or abandoned executions become immutable outcome and procedure-fitness assessments
against declared health, containment, recovery, communication, and rollback criteria. Owners
can retain evidence-supported deviations and participant feedback, open caller-stable ordinary
documentation, workflow, policy, infrastructure, or human/agent code work, and publish a
reviewed successor that must be rehearsed anew. Unsafe current use can be suspended only with
a passing exact fallback, while every prior execution remains bound to the version it used.

The `operational-runbook-journey.spec.ts` browser/API/stock-Git journey connects that full lifecycle:
reviewed operational knowledge is rehearsed by a human and approved agent, launched only from an
authoritatively verified revision-exact alert, executed through approval and handoff, assessed, improved
through an ordinary reviewed pull, published as a successor, and proven again with fresh rehearsal.

Repository participants and scoped read-only agents can turn an exact objective into
an immutable, permission-aware capacity model at `/repositories/{id}/capacity-models`.
The Capacity workspace exposes release-bound sanitized evidence, observation windows,
assumptions, workload segments, saturation uncertainty, costs, alternative scenarios,
and append-only challenges without treating restricted observations as absent proof.

## Getting started

```sh
bun install

bun dev          # frontend  → http://localhost:3000
bun run dev:api  # api       → http://localhost:8080/health
```
