---
name: lint
description: Run Metarr's linters the cheap way — golangci-lint for Go, stylelint for CSS. Use after any Go or CSS change, instead of defaulting to `make lint`. For TypeScript/TSX linting (ESLint), use the `ui-checks` skill instead — this one covers the rest of `make lint`'s surface.
---

# Lint

`make lint` runs `lint-go` (`go tool golangci-lint run ./...`) and `lint-ui` (`yarn workspace @metarr/metarr-ui run lint` = ESLint + stylelint). Both are already silent on a clean pass (`golangci-lint`: one line, `0 issues.`; `stylelint`: nothing at all) — there's no verbose flag to avoid here. The token cost to manage is running the wrong scope, not reading noisy output.

## Go — golangci-lint

```bash
go tool golangci-lint run ./...
```
Whole-repo is already fast on a warm build cache (measured ~1.4s) — golangci-lint has to type-check a package's full dependency graph regardless of how narrowly you scope it, so unlike `go test`, **scoping to a subpackage buys little and can even be slower on a cold cache** (it still walks the same dependency graph, just without the warm-cache head start a preceding `go build ./...` would have given it). Run it whole-repo; only narrow to `./internal/foo/...` if you specifically want a shorter list of issues to read back, not for speed.

## CSS — stylelint

```bash
cd ui && npx stylelint "src/**/*.css"
```
This repo has exactly one `.css` file (`src/index.css` as of this writing) — there is nothing to scope. Run the glob as-is; don't build a diff-scoped variant for a one-file target.

## TypeScript/TSX — see `ui-checks`

ESLint is `lint-ui`'s other half but has its own skill (`ui-checks`) alongside `tsc` and scoped Vitest runs, since all three together are the frontend's regression signal rather than a lint-only concern. Don't duplicate that scoping logic here — invoke `ui-checks` for anything touching `ui/src/**/*.{ts,tsx}`.

## When `make lint` (the full combined run) is worth it

Right before handing work off, or after a change wide enough that guessing the affected linter scope isn't worth the guess (a shared config file, a repo-wide rename). Otherwise run just the linter for the language you actually touched.
