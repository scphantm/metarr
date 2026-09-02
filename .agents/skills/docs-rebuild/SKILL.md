# Documentation rebuild

Metarr's documentation is built with Antora from AsciiDoc sources under `documentation/`. The build output goes to
`build/site/`, and the rendered HTML is viewable at `build/site/index.html` after a successful build.

## Setup (one-time only)

```bash
make docs-initialize
```

Downloads Node dependencies and the UI theme bundle. Only needed once, or after deleting `node_modules/` or
`documentation-theme/`.

## Build

```bash
make docs-build
```

Regenerates all HTML from `documentation/modules/*/pages/*.adoc`, including xref resolution across modules. Fast (~3–5s
on a warm build). Antora reports warnings (e.g., stale xrefs) but does not fail on them.

## Clean + rebuild (force fresh)

```bash
rm -rf build/site && make docs-build
```

Useful after:

- Renaming or deleting documentation files (orphaned xrefs don't get cleaned automatically)
- Switching between branches with incompatible doc structure
- Debugging a suspected cache corruption in Antora's intermediate state

A plain `make docs-build` (without the `rm`) is sufficient for regular edits — Antora re-renders pages incrementally.

## Verify the build

After running `make docs-build`, check:

- Exit code is 0 (no fatal errors)
- No `"level":"error"` or `"level":"fatal"` lines in the output
- `build/site/index.html` exists
- The page you edited renders at `build/site/metarr/~/<page>.html`

Warnings (`"level":"warn"`) are harmless — typically stale xref hints or missing attribute values in code examples.

## When to rebuild

- After editing any `.adoc` file under `documentation/`
- Before handing off work involving documentation changes (verify clean build, no new warnings)
- After merging documentation from another branch (catch xref divergence early)

Never required after code-only changes (Go, TypeScript, etc.) — the docs don't auto-generate from source comments.
