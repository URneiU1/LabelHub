# LabelHub Web (`apps/web`)

React 18 + TypeScript (strict) + Semi Design SPA — the Owner / Labeler / Reviewer
front-end of the LabelHub monorepo. Built with Vite.

This package is not meant to be set up on its own. For architecture, the full
local-run quickstart, deployment, and the judge walkthrough, see the repository
root:

- [`../../README.md`](../../README.md) — project overview & quickstart
- [`../../submission/`](../../submission/) — judge deliverable package
- [`../../docs/ARCHITECTURE.md`](../../docs/ARCHITECTURE.md) — architecture & decisions

## Local commands (run from repo root)

```bash
make web              # Vite dev server — http://localhost:5173
pnpm -F web test      # vitest (pool: threads)
pnpm -F web gen:api   # regenerate schema.d.ts from docs/openapi.yaml
```
