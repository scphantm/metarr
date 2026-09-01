# UI (Vite/React) build and development tasks for the Metarr web frontend.
# Included by the main Makefile.

.PHONY: ui-install ui-dev ui-build ui-sync-version ui-test ui-test-watch

# Install UI dependencies via yarn.
# Reads from yarn.lock to ensure reproducible installs.
# Run this once after cloning or after updating ui/package.json.
ui-install:
	yarn workspace @metarr/metarr-ui install

# Start the Vite development server on http://localhost:5173.
# The dev server proxies /api requests to http://localhost:8080 (the Metarr API).
# To point the API proxy elsewhere, set METARR_API_URL:
#   make ui-dev METARR_API_URL=http://other-host:8080
#
# Note: The API server must be running separately (make run-server) for /api calls to work.
ui-dev:
	yarn workspace @metarr/metarr-ui run dev

# Build the production bundle.
# Runs TypeScript type-checking and Vite bundling, producing optimized assets in ui/dist/.
# Use this to test production builds locally or before deploying.
ui-build: node-sync-version
	yarn workspace @metarr/metarr-ui run build

# Run the UI unit test suite once.
# Tests use Vitest and cover utilities, hooks, and core business logic.
# See ui/TEST.md for details on test organization and writing new tests.
ui-test:
	yarn workspace @metarr/metarr-ui run test

# Run the UI unit tests in watch mode.
# Reruns tests when source files change — useful during development.
ui-test-watch:
	yarn workspace @metarr/metarr-ui run test:watch
