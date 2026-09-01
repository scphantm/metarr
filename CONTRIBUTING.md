# Contributing to Metarr

Thanks for your interest in improving Metarr! This guide will help you understand the project structure, development workflow, and conventions.

## Before You Start

Read [`CLAUDE.md`](./CLAUDE.md) — it documents project-specific conventions, architectural boundaries, and rules that override general practices. Key areas:

- **Architectural boundaries**: `metarr-server` (MongoDB, HTTP, listeners) ↔ `metarr-agent` (filesystem, subprocess, Redis only)
- **Design documentation**: Read `documentation/modules/design/` before implementing changes in covered areas
- **Logging**: Always use key-value pairs, never `fmt.Sprintf`; never use `fmt.Sprintf` to build log messages
- **Configuration changes**: Require CRUD API routes, UI, and init/agent registry updates
- **Workflow engine**: Handlers never import `os`/`os/exec`; they get capability from the executor harness

## Development Workflow

### Setup

1. **Clone and install dependencies**
   ```bash
   git clone https://github.com/yourusername/Metarr.git
   cd Metarr
   make install
   ```

2. **Start the dev environment**
   ```bash
   make run        # builds and runs the full stack
   ```

   This starts `metarr-server`, `metarr-agent`, and the UI dev server.

3. **Run tests and checks frequently**
   ```bash
   go test ./...                    # Go tests
   make ui-test                     # UI tests
   make lint                        # All linters
   ```

### Making Changes

#### Go Changes (Server or Agent)

1. **Edit code in `cmd/`, `internal/server/`, or `internal/agent/`**
   - Server changes go in `internal/server/`; agent changes go in `internal/agent/`
   - **Never** let agent code import from `internal/server/` (enforced by `internal/agent/boundary_test.go`)
   - Use the `agentregistry.PathTranslator` for OS-agnostic agent paths (agent runs on Windows, Linux, macOS)
   - Use cached HTTP client for metadata calls (Sonarr, etc.); plain client for one-shots

2. **Add regression tests**
   After validating a change, add unit tests in the same package. Token efficiency matters; tests are rerun often.

3. **Build and test**
   ```bash
   go build ./...                   # Check compilation
   go test -run TestName ./...      # Run specific tests
   ```

4. **Lint**
   ```bash
   make lint   # or via Claude Code: /lint skill
   ```

#### TypeScript/UI Changes

1. **Edit files under `ui/src`**

2. **Type-check and lint incrementally**
   ```bash
   make ui-build                    # incremental tsc
   make lint-ui                     # diff-scoped eslint
   ```

3. **Test in the browser**
   The dev server watches for changes; refresh your browser to see them. Test the golden path and edge cases; watch for regressions in other features.

#### Configuration Structure Changes

Configuration changes also require:
1. CRUD methods in the config API router
2. UI to manage the new settings
3. Initialization in `/cmd/metarr-server/main.go`
4. If agent-needed: add to `agentregistry.BuildProjection` (readable by every agent host — add deliberately, never expose secrets)

#### Documentation Changes

1. **Design docs**: Read [`documentation/modules/design/`](./documentation/modules/design/) first. **Do not edit design docs** without explicit approval — ask first if a change requires the design itself to be different.

2. **System docs**: In `documentation/modules/ROOT/`. When asked to update docs, assume the code is correct and update the docs to match.

3. **AsciiDoc style**: Use code examples from the actual codebase, not made-up snippets. Verify xrefs before pushing.

4. **Build and serve locally**
   ```bash
   make docs_build                  # Fast rebuild (~3–5s)
   make docs_serve                  # Start live preview at http://localhost:5252
   ```

### Code Style & Conventions

- **Descriptive names**: Variables, functions, and types should be self-documenting.
- **Comments**: Only when *why* is non-obvious (hidden constraint, workaround, subtle invariant). Never explain *what* — well-named code already does that.
- **Log messages**: Pass dynamic values as trailing key-value pairs: `logger.Info("scan started", "scan_id", id)` — never `fmt.Sprintf`.
- **Error handling**: Validate at system boundaries (user input, external APIs). Trust internal code and framework guarantees.
- **No premature abstractions**: Three similar lines is better than a premature helper.

## Testing

- **Go tests**: Cover the packages your change touches. Run with `go test -run TestName ./...` for focused testing.
- **UI tests**: TypeScript type-checking via `make ui-build` catches most issues; manual browser testing for UI behavior.
- **Integration**: Before submitting, run `make test` (Go) and `make ui-test` (UI) to catch regressions.

## Submitting Changes

### Commits

- Create **one commit per logical change** (don't batch unrelated work).
- Write concise commit messages: 1–2 sentences describing the *why*, not the *what*.
  ```
  Add rate limiter to scan endpoint to prevent thundering herd

  Limits concurrent scans per agent to 5, queues excess requests.
  ```
- Include a `Claude-Session` line if using Claude Code (optional, helps with context).

### Pull Requests

- Open a PR early if the change is substantial — get feedback before investing heavily.
- Use the PR description to explain:
  - What problem does this solve?
  - How does this change the user experience (if at all)?
  - Which design docs or CLAUDE.md rules are relevant?
- Link any related issues or design docs.
- **Tests**: Include regression tests for any bug fixes or new features.
- **Lint**: Run `make lint` before pushing; CI will reject lint failures.

## Getting Help

- **Questions about Claude Code or the SDK**: See the [`claude-api` skill reference](https://claude.com/claude-api) or search GitHub issues.
- **Design questions**: Read `documentation/modules/design/` first, then open a discussion if you need clarification.
- **Stuck on a feature**: Open a draft PR and describe what you're trying to do — feedback often unblocks.

## Code Review Checklist

Before submitting, verify:

- [ ] Code compiles (`go build ./...` or `make ui-build`)
- [ ] Tests pass (`go test ./...` or `make ui-test`)
- [ ] Linters pass (`make lint`)
- [ ] No new warnings in the build output
- [ ] Design docs reviewed (if applicable)
- [ ] Architecture boundaries respected (agent ↔ server, no Mongo in agent, etc.)
- [ ] Log messages use key-value pairs, not `fmt.Sprintf`
- [ ] Configuration changes include API CRUD, UI, and init
- [ ] Commit message explains *why*, not *what*

## Project Structure

```
Metarr/
├── cmd/                          # Executables
│   ├── metarr-server/            # Main server
│   └── metarr-agent/             # Agent binary
├── internal/
│   ├── agent/                    # Agent-only code (filesystem, subprocess)
│   ├── server/                   # Server-only code (MongoDB, HTTP)
│   └── shared/                   # Event contracts, config, models
├── api/                          # API definitions
├── ui/                           # TypeScript/React frontend
├── config/                       # Runtime config (server/agent YAML, catalog, sidecar configs)
├── deploy/                       # Dockerfiles
├── make/                         # Makefile fragments (ui.mk, docs.mk)
├── documentation/                # AsciiDoc source
├── documentation-theme/          # Antora UI theme
└── CLAUDE.md                      # Project conventions (required reading!)
```

## Key Skills & Tools

These Claude Code skills automate common tasks:

- `/go-build` — Compile Go binaries the cheap way (runs `go build ./...` as a smoke test)
- `/go-tests` — Run scoped Go tests (only packages touched by your change)
- `/lint` — Run golangci-lint for Go; stylelint for CSS
- `/ui-checks` — Run TypeScript type-checking and scoped ESLint for UI changes
- `/run-metarr` — Build, run, and smoke-test the full stack
- `/code-review` — Review your changes for bugs and simplification opportunities
- `/docs-rebuild` — Rebuild documentation

## Questions?

Open an issue on GitHub or start a discussion. We welcome contributions of all kinds — code, documentation, design feedback, and bug reports.

Happy coding!
