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
