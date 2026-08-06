<!--
Append-only agent log. Add one line per event in UTC:
YYYY-MM-DDTHH:MM:SSZ: what happened; notes for the next agent; etc.
Fetch the timestamp on Linux with: date -u '+%Y-%m-%dT%H:%M:%SZ'
-->

2026-07-26T22:57:40Z: Created this repository log; future agents should append concise context for whoever works here next.
2026-08-06T17:50:50Z: Added an atomic filesystem-backed bare Git repository lifecycle in `apps/api/storage`, with stable IDs, create/open/inspect operations, `main` as the unborn default branch, and stock Git compatibility tests.
