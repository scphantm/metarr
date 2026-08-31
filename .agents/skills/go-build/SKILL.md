---
name: go-build
description: Build Metarr's Go binaries the cheap way — plain `go build ./...` as the default compile check, when `go generate` is (and isn't) actually needed, and why `go clean -cache` should never be reached for. Use after any Go change, before or alongside go-tests, instead of defaulting to `make build` or a cross-compiled dist target.
---

# Go build

`go build ./...` is already cheap by default — Go's build cache makes a warm re-build near-instant (a full-repo build measured well under 2s here), so unlike `go test`, **there's little to gain from scoping the build itself to a subset of packages**. Run it whole-repo every time; a compile error names its own package regardless of scope.

## Default loop

1. **`go build ./...`, not `make build`.** `make build` chains `generate` first — see below for why that's usually unwanted. Plain `go build ./...` compiles everything and discards the output, which is all a correctness check needs.

2. **Don't run `go generate ./...` reflexively.** It regenerates `api/docs.go`/`api/swagger.json`/`api/swagger.yaml` (from `swag`, see `generate.go`) and the Sonarr client (from `oapi-codegen`) — ~30-40 lines of per-type log noise, and it **rewrites those committed files** if the source doc-comments they're generated from have drifted at all since the last commit, even from changes unrelated to your task. Only run it when you've actually changed: a Swagger-annotated handler's comments/types, or `api/sonarr_openapi.yaml`. If you do run it, check `git status` after — a regen touching files outside what you meant to change means those files were already stale before you started, and reverting them (`git checkout -- api/`) keeps that pre-existing drift out of your diff. Don't include `docs/` in that revert — it's the unrelated issue-tracker/domain-docs directory (see CLAUDE.md), not `swag` output, and checking it out would discard real work.

3. **Building an actual binary** (`make build-server`, `make build-agent`, output to `bin/`) is for when you need to run or exercise it — not a substitute check to reach for during a regression pass. `go build ./...` already proves it compiles.

4. **Cross-compiled `dist-agent-*` targets** (Linux/Windows/macOS × amd64/arm64) are release artifacts, not a build-health check — never run these to verify a code change.

## The mistake to not repeat

`go clean -cache` looked appealing once to force a "true" cold-build timing check — don't do this. It wipes Go's build cache **machine-wide**, not just for this module, so it also slows down the *next* build of every other Go project on the box, and the only thing it bought here was a one-time 9s build instead of 2s. The incremental cache is the reason `go build ./...` is already the cheap option; there is never a reason to defeat it deliberately.
