# Lint and format tasks for the whole repo (Go + web UI).
# Included by the main Makefile.

.PHONY: lint lint-go lint-ui fmt fmt-check

# Lint all code: Go (golangci-lint) and the UI (ESLint + stylelint).
# Runs lint-go and lint-ui. See those targets for details.
lint: lint-go lint-ui

# Lint the Go code using golangci-lint.
# Checks for style, correctness, and common mistakes across the entire Go codebase.
# Run after making changes to catch issues early.
lint-go:
	go tool golangci-lint run ./...

# Lint the UI: ESLint over src/ (see ui/eslint.config.js) plus stylelint over
# the stylesheets. Delegates to the workspace `lint` script
# (`lint:js` + `lint:css`). TypeScript type-checking is separate (`tsc -b`).
lint-ui:
	yarn workspace @metarr/metarr-ui run lint

# Format the codebase in place: golangci-lint's formatters for Go, Prettier
# for the frontend + repo-level JS/JSON/CSS/Markdown/YAML.
fmt:
	go tool golangci-lint fmt ./...
	yarn prettier --write .

# Verify formatting without writing (for CI). Non-zero exit if anything is unformatted.
fmt-check:
	go tool golangci-lint fmt --diff ./...
	yarn prettier --check .
