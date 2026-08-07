# Vivarium web

The Next.js App Router frontend for Vivarium. Run it from the repository root:

```sh
bun dev
```

The persistent navigation and page boundary live in
`src/components/app-shell.tsx`. Shared visual primitives and icons live in
`src/components/ui.tsx` and `src/components/icons.tsx`; global design tokens
and accessibility defaults live in `src/app/globals.css`. Extend these shared
patterns before adding route-local variants.

Before writing frontend code, read `AGENTS.md` in this directory and the
relevant guides installed under `node_modules/next/dist/docs/`; the repository
uses Next.js 16 and its current behavior is authoritative.

Verify changes with:

```sh
bun run lint
cd apps/web && bunx tsc --noEmit
bun run build
```
