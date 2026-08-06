# Docs

Notes on how this project fits together. Mostly empty for now — things get
written down here as they're decided, not before.

## Entrypoints

- `apps/web` — Next.js frontend. Starts at `src/app/page.tsx`; routes are
  file-based under `src/app`. `bun dev` from the repo root.
- `apps/api` — Go HTTP API. Starts at `main.go`, where routes are registered on
  the mux. `bun run dev:api` from the repo root, serves on `:8080`.

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
