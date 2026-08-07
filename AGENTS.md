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
  point is `src/app/page.tsx`, shared shell is `layout.tsx`, global styles are
  `globals.css`. Tailwind v4 is wired through PostCSS
  (`postcss.config.mjs`); there is no `tailwind.config` file.
- **API** — a single `main.go` registering handlers on a `net/http` mux with
  Go 1.22+ method-and-path patterns (`"GET /health"`). It has no third-party
  dependencies and no `go.sum`; adding a dependency means the api workflow's
  `cache: false` line should flip to `cache-dependency-path: apps/api/go.sum`.
  The port comes from `$PORT`, defaulting to `8080`. Bare Git repository
  lifecycle storage lives in `apps/api/storage`; callers create or reopen a
  stable storage ID there before performing repository operations.
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
  and deletion pushes for `main`. Other ref names remain denied; ordinary
  non-fast-forward pushes are rejected by stock clients unless force is
  requested, and receive-pack applies accepted updates transactionally. The
  end-to-end stock-client compatibility suite covers the complete workflow in
  sequence: initial push, clone, ordinary push and pull, forced replacement,
  deletion, empty clone, recovery push, and recovery pull.
  Durable human identities live in `apps/api/users` as atomic JSON records.
  Opaque 128-bit lowercase IDs are permanent attribution keys; handles are
  globally unique and, along with display names, are editable profile data.
  `POST /users`, `GET /users/{id}`, and `PATCH /users/{id}` expose the account
  lifecycle. `$USER_STORAGE_ROOT` selects their directory and defaults to
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
- **Docs** — `docs/README.md` records decisions once they're made, not before.
  Update it when you change how the apps fit together, not for every change.

## LADDER.md

`/LADDER.md` at the repo root is **not part of this repository**. It is placed
there from outside the checkout — as a symlink or a read-only bind mount — and
is gitignored for that reason. Read it for context on what is being built, but
never edit it, `git add` it, or delete it, and do not treat its presence as
uncommitted work.
