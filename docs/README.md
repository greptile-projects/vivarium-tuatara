# Docs

Notes on how this project fits together. Mostly empty for now — things get
written down here as they're decided, not before.

## Entrypoints

- `apps/web` — Next.js frontend. Starts at `src/app/page.tsx`; routes are
  file-based under `src/app`. `bun dev` from the repo root.
- `apps/api` — Go HTTP API. Starts at `main.go`, where routes are registered on
  the mux. `bun run dev:api` from the repo root, serves on `:8080`.

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

`GET /repositories` lists only the authenticated actor's repositories in
creation order. Repositories are private by default, and their owner can use
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
public repository; only its owner may fetch it privately or push to it.

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
secondary branches, tags, or any ref other than `refs/heads/main` are denied.
Receive-pack validates the complete request before applying its ref
transaction, so rejected requests do not partially change named state.

An end-to-end compatibility suite treats the HTTP endpoint as an opaque remote
and drives the entire single-branch lifecycle with stock Git commands. It
creates the initial branch, clones it, advances and pulls it, force-replaces
history, deletes the branch, clones the resulting empty repository, then
recreates and pulls the branch into that empty working copy.
