---
name: ui-checks
description: Run Metarr's frontend regression checks (TypeScript, ESLint, and scoped Vitest) the cheap way — incremental tsc, diff-scoped lint, targeted test runs, no full rebuilds. Use after any change under ui/src, instead of defaulting to `yarn workspace @metarr/metarr-ui run build` or `eslint src` on the whole tree.
---

# Frontend regression checks

The regression signal after a frontend change is `tsc` (typecheck), `eslint` (lint) — together `make lint-ui` (`yarn workspace @metarr/metarr-ui run lint` = `lint:js` + `lint:css`) — and `vitest` (unit tests, `make ui-test` / `yarn workspace @metarr/metarr-ui run test`) for anything under `ui/src/**/__tests__/`. All three are quiet on success; the token cost to avoid is re-checking the whole tree when only a few files changed.

## Default loop

1. **Typecheck incrementally, not `--force`.** Plain `tsc -b` (no flag) reuses `tsconfig.tsbuildinfo` and only re-checks what changed — same correctness as a full check, far less to wait on:
   ```bash
   cd ui && npx tsc -b
   ```
   Reach for `npx tsc -b --force` only when you suspect the build-info cache itself is stale (e.g. after switching branches) or when doing a final pre-handoff pass — not as the routine check.

2. **Lint only the files you touched**, not `eslint src`:
   ```bash
   cd ui && npx eslint $(git diff --name-only --diff-filter=ACM -- '*.ts' '*.tsx' | sed 's#^ui/##')
   ```
   (Paths from `git diff` are repo-root-relative; strip the `ui/` prefix since the command runs from inside `ui/`.) Both `tsc` and `eslint` print nothing at all on a clean pass — no output to summarize means the check passed; don't re-run with `-v`-equivalent flags looking for confirmation.

3. **Full-tree `npx eslint src` / `yarn workspace @metarr/metarr-ui run lint`** is only worth it right before handing work off, or after a change to shared infrastructure (`tsconfig.json`, `eslint.config.*`, a widely-imported type in `catalogTypes.ts` or similar) where the blast radius isn't obviously limited to the files you edited.

4. **`yarn workspace @metarr/metarr-ui run build`** (full `tsc -b && vite build`) is the expensive one — it re-bundles and reports chunk sizes irrelevant to a correctness check. Reserve it for confirming the production build still succeeds, not as a stand-in for typecheck.

5. **Run Vitest scoped to what you touched**, not the whole suite:
   ```bash
   cd ui && npx vitest run src/lib/__tests__/useDebouncedValue.test.ts
   ```
   Existing suites live under `ui/src/**/__tests__/` (see `ui/TEST.md` for the convention). If your change affects logic a suite already covers — a hook, a query helper, a pure function like `connectionRules.ts` — run that suite. If it's new business logic worth protecting (not JSX rendering), add a test alongside the existing convention rather than skipping coverage, per CLAUDE.md's "add regression unit tests" rule.
   Full-suite `yarn workspace @metarr/metarr-ui run test` (or `make ui-test`) is worth it before handing off, or after touching something widely shared (a query client, a type several suites import).

## Reading output cheaply

- Silence from `tsc` or `eslint` is the pass signal — don't ask the tool to also print something on success.
- A `tsc` failure names the file, line, and error code (`TSxxxx`) directly; that's enough to act on without re-running with extra flags.
- An `eslint` failure lists rule id per line (`no-unused-vars`, etc.); fix and re-run scoped to just that file, not the batch you started with.

## Reporting results honestly

State plainly which of typecheck / lint / vitest actually ran and passed — don't say "tests pass" if you only ran tsc and eslint, and don't run the full Vitest suite just to be able to say so when a scoped run already covers the change. Vitest covers business logic (hooks, utilities, query helpers); it does not exercise JSX rendering or browser behavior — if the change is user-facing, say explicitly that it hasn't been checked in a real browser even when Vitest passed.
