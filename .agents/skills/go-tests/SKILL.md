---
name: go-tests
description: Run Metarr's Go regression suite the cheap way — scoped to the packages a change actually touches, quiet on success, verbose only on failure. Use after any change under internal/, cmd/, or api/, instead of defaulting to `go test ./...` or `make test` with full output. Also covers the mandatory internal/agent/boundary_test.go check and when a full-repo run is actually warranted.
---

# Go regression tests

Metarr has no CI to lean on here — every regression check in this loop is a token cost. The suite itself is fast (whole repo: well under 2s including "cached" packages), so the expensive part is never the test run — it's dumping 40 lines of `ok  Metarr/internal/...` boilerplate into context when only two packages could possibly have changed.

## Default loop

1. **Scope to what changed.** `git diff --name-only` (or the paths you just edited) tells you the Go package(s) in play. Test those packages directly rather than `./...`:
   ```bash
   go test ./internal/server/workflow/validate/...
   ```
   Multiple packages in one invocation is fine and still cheap:
   ```bash
   go test ./internal/agent/... ./internal/shared/scanmodel/...
   ```

2. **No `-v` on the first pass.** `go test` already prints nothing on success beyond one `ok` line per package — that's the quiet baseline. Verbose (`-v`) multiplies output per-subtest and should only come out once something has actually failed and you need to see which subtest.

3. **Target a single test while iterating on it** instead of re-running the whole package:
   ```bash
   go test ./internal/agent/mediascan/... -run TestParseVideoName
   ```
   `-run` is a regex matched against the full `Test.../Subtest...` path, so for a table-driven test (the repo's convention for multi-case tests — see `t.Run(test.name, ...)` in `internal/agent/mediascan/parse_test.go`, `internal/shared/scanmodel/sidecar_test.go`, `internal/agent/nfo/*_test.go`), `-run 'TestParseVideoName/numeric_title'` reaches one table case instead of the whole set. Spaces in a subtest name become underscores in the path Go matches against.

4. **`go build ./...` before `go test ./...`** when you've touched something widely imported (a shared type, a package under `internal/shared/`). A build failure is a one-line error; a compile failure surfaced through `go test` repeats it per broken package. `go vet ./...` is worth the same treatment for anything vet catches statically (unreachable code, format-string mismatches) — it's faster than a test run and fails loud.

5. **When a full-repo run is actually warranted**: after touching `internal/shared/` (imported by both binaries) or anything in `internal/agent/` or `internal/server/agentregistry/` (the agent/server data-transmission boundary — see CLAUDE.md). `go test ./...` at repo root; skip `-v`. Mongo-backed tests (`internal/server/mongostore/*_test.go`) skip themselves cleanly with no local MongoDB, so a full run never blocks on missing infra.

## The one test that's never optional

`internal/agent/boundary_test.go` (`TestAgentNeverDependsOnMongoOrServer`) walks the real build graph to enforce "nothing under `internal/agent/` may import `internal/server/` or reach mongo" (CLAUDE.md, "the no-mongo rule ... do not weaken it"). Run it explicitly any time a change touches an import line anywhere under `internal/agent/`, `internal/server/`, or `internal/shared/` — a passing `go test ./internal/agent/...` already covers it, but if you're scoping narrower than that for another reason, add it back:
```bash
go test ./internal/agent/... -run TestAgentNeverDependsOnMongoOrServer
```

## Reading output cheaply

- Success: one line per package (`ok  	Metarr/internal/foo	0.003s`). Don't re-read or summarize this back to the user — a clean pass needs no narration beyond "tests pass."
- Failure: `go test` already prints the failing `t.Error`/`t.Fatal` message and a `--- FAIL:` line naming the exact subtest; read that before reaching for `-v` or re-running.
- If a package prints `[no test files]` and it's not one you expected to touch, you scoped the command wider than necessary — narrow it next time instead of grepping the noise away.

## Writing new regression tests

Table-driven `t.Run` subtests are this repo's existing convention for anything with multiple cases (see the files listed above) — they let a future change target one case with `-run Test.../case_name` instead of re-running the whole file, which is the same token-saving principle applied at write-time instead of run-time. `internal/server/workflow/validate/validate_test.go` instead uses one flat `func TestXxx` per scenario (no subtests) — fine for validation rules that are each their own scenario, but reach for table-driven subtests when a change adds several small variations of the same behavior.
