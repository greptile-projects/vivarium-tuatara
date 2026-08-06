<!--
Append-only agent log. Add one line per event in UTC:
YYYY-MM-DDTHH:MM:SSZ: what happened; notes for the next agent; etc.
Fetch the timestamp on Linux with: date -u '+%Y-%m-%dT%H:%M:%SZ'
-->

2026-07-26T22:57:40Z: Created this repository log; future agents should append concise context for whoever works here next.
2026-08-06T17:50:50Z: Added an atomic filesystem-backed bare Git repository lifecycle in `apps/api/storage`, with stable IDs, create/open/inspect operations, `main` as the unborn default branch, and stock Git compatibility tests.
2026-08-06T18:00:44Z: Tightened repository inspection after review to parse core Git configuration and reject missing or unsupported repository format versions before reopening.
2026-08-06T18:15:41Z: Made repository inspection honor Git-config quoted values, escapes, and inline comments while retaining strict repository format validation.
2026-08-06T18:26:56Z: Extended Git-config section parsing to accept valid trailing comments such as `[core] # repository settings`, with compatibility regression coverage.
2026-08-06T18:38:28Z: Added atomic, integrity-checked loose-object writes and reads for blobs, trees, commits, and annotated tags; compatibility tests verify exact platform and `git cat-file` round trips plus `git fsck --full`.
2026-08-06T18:51:26Z: Hardened object storage after review with file/directory durability syncs, a 100 MiB object bound and header-first decompression, and rejection of trailing loose-object bytes that stock Git considers corrupt.
2026-08-06T19:06:43Z: Made idempotent object-write retries repeat both fanout and parent objects-directory syncs, so a retry after a post-publication durability failure cannot acknowledge a still-unsynced fanout entry.
2026-08-06T19:17:13Z: Made repository creation sync the published bare repository directory and storage root after rename, preventing successful creation from acknowledging an unpersisted repository entry.
2026-08-06T19:26:32Z: Added deterministic, integrity-checked loose-object enumeration through `Repository.ListObjects`; compatibility coverage proves its identity, type, size, and content set matches `git cat-file --batch-all-objects`.
