# HTTP API contract

The JSON API is the supported application boundary for browsers, agents, and
external consumers. Durable files beneath the configured storage roots are
private implementation details. Git clients use the smart HTTP URLs returned
by repository resources.

## Conventions

- Requests and responses use JSON. Successful resource creation returns `201`
  and a `Location` header; deletion returns `204`.
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
Responses contain the resource array and `next_cursor`, which is `null` on the
last page. Pass a non-null `next_cursor` unchanged as the next request's
`after`; cursors outside the authenticated collection return
`invalid_pagination`. Collection order is oldest creation first, with opaque ID
as the deterministic tie-breaker.

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
the current account. `GET /repositories` paginates that account's repositories.
`GET /repositories/{id}` returns an owned private repository or any public
repository. `PATCH /repositories/{id}` accepts `visibility` as `private` or
`public`; `DELETE /repositories/{id}` removes the owned repository and its Git
remote.

Repository responses include immutable `id` and `owner_id`, user-facing
`name`, `visibility`, `default_branch`, `created_at`, and `git_remote`. Use the
returned `git_remote` relative to the API origin. Private reads and all writes
require the owner and matching repository or Git credential scope.
