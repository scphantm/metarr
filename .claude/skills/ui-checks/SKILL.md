---
name: ui-checks
description: Run Metarr's frontend regression checks (TypeScript + ESLint — there is no unit test runner in ui/) the cheap way — incremental tsc, diff-scoped lint, no full rebuilds. Use after any change under ui/src, instead of defaulting to `npm run build` or `eslint src` on the whole tree.
---

# Frontend regression checks

`ui/` has no unit test framework installed (no vitest/jest, no `test` script in `ui/package.json`) — the regression signal after a frontend change is `tsc` (typecheck) and `eslint` (lint), covered together by `make lint-ui` (`npm run lint` = `lint:js` + `lint:css`). Both tools are already quiet on success; the token cost to avoid is re-checking the whole tree when only a few files changed.

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

3. **Full-tree `npx eslint src` / `npm run lint`** is only worth it right before handing work off, or after a change to shared infrastructure (`tsconfig.json`, `eslint.config.*`, a widely-imported type in `catalogTypes.ts` or similar) where the blast radius isn't obviously limited to the files you edited.

4. **`npm run build`** (full `tsc -b && vite build`) is the expensive one — it re-bundles and reports chunk sizes irrelevant to a correctness check. Reserve it for confirming the production build still succeeds, not as a stand-in for typecheck.

## Reading output cheaply

- Silence from `tsc` or `eslint` is the pass signal — don't ask the tool to also print something on success.
- A `tsc` failure names the file, line, and error code (`TSxxxx`) directly; that's enough to act on without re-running with extra flags.
- An `eslint` failure lists rule id per line (`no-unused-vars`, etc.); fix and re-run scoped to just that file, not the batch you started with.

## No unit tests here — don't invent a false signal

Don't claim "tests pass" for frontend changes — there's nothing to run. State plainly that typecheck/lint passed and, if the change is user-facing, that it hasn't been exercised in a real browser. If a change is complex enough to want real regression coverage (business logic worth protecting, not JSX rendering), say so and ask before adding a test framework — that's an infrastructure decision, not something to do silently mid-task.
