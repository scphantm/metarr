# Makefile for Metarr: build, run, test, and lint targets for the Go backend.
# Includes UI tasks from make/ui.mk and documentation tasks from make/docs.mk.

include make/ui.mk
include make/docs.mk

.PHONY: help generate build build-server build-agent build-server-production run run-server run-agent run-production down test tidy \
	dist dist-agent-linux-amd64 dist-agent-linux-arm64 \
	dist-agent-windows-amd64 dist-agent-darwin-arm64 \
	docker-build lint lint-go

# Display this help message showing all available tasks and their descriptions.
help:
	@echo "Metarr Makefile - Available Tasks"
	@echo "=================================="
	@echo ""
	@echo "FULL STACK:"
	@echo "  make run              Start the full stack: docker compose + server + agent + ui"
	@echo "  make down             Stop all components and bring down docker compose"
	@echo ""
	@echo "BUILD:"
	@echo "  make generate         Run go:generate directives (Swagger, Sonarr client, gRPC-Web proto stubs)"
	@echo "  make build            Compile all Go binaries (no output)"
	@echo "  make build-server     Build metarr-server to bin/metarr-server"
	@echo "  make build-agent      Build metarr-agent to bin/metarr-agent"
	@echo "  make build-server-production   Build metarr-server with embedded UI"
	@echo ""
	@echo "RUN (Individual Components):"
	@echo "  make run-server       Start the metarr-server in foreground"
	@echo "  make run-agent        Start the metarr-agent in foreground"
	@echo "                        Override: METARR_AGENT_SLUG=nas-01 METARR_REDIS_HOST=10.0.0.5"
	@echo "  make run-production   Build and run metarr-server with embedded UI"
	@echo ""
	@echo "UI (from make/ui.mk):"
	@echo "  make ui-install       Install UI dependencies (yarn install)"
	@echo "  make ui-dev           Start Vite dev server on http://localhost:5173"
	@echo "  make ui-build         Build production UI bundle"
	@echo "  make ui-test          Run UI unit tests"
	@echo "  make ui-test-watch    Run UI tests in watch mode"
	@echo ""
	@echo "DISTRIBUTION (Cross-Compiled Binaries):"
	@echo "  make dist             Build all agent binaries (linux/arm64, linux/amd64, windows, darwin)"
	@echo "  make dist-agent-linux-amd64      Linux x86-64"
	@echo "  make dist-agent-linux-arm64      Linux ARM 64-bit (QNAP, Synology)"
	@echo "  make dist-agent-windows-amd64    Windows x86-64"
	@echo "  make dist-agent-darwin-arm64     macOS ARM 64-bit (Apple Silicon)"
	@echo ""
	@echo "DOCKER:"
	@echo "  make docker-build     Build metarr-server and metarr-agent Docker images"
	@echo "                        Use: make docker-build VERSION=1.0.0"
	@echo ""
	@echo "TEST & LINT:"
	@echo "  make test             Run all Go tests"
	@echo "  make lint             Run all linters (Go + UI)"
	@echo "  make lint-go          Lint Go code with golangci-lint"
	@echo "  make lint-ui          Lint UI code with ESLint"
	@echo ""
	@echo "MAINTENANCE:"
	@echo "  make tidy             Tidy go.mod and go.sum"
	@echo ""
	@echo "DOCUMENTATION (from make/docs.mk):"
	@echo "  make docs-install     Install documentation dependencies"
	@echo "  make docs-build       Build the Antora documentation site"
	@echo "  make docs-serve       Start a local documentation server"
	@echo "  make docs-shell       Launch interactive shell in documentation container (for debugging)"
	@echo ""

# Generate code: run all go:generate directives in the repo.
# Produces:
#   - Swagger docs:          api/docs.go, api/swagger.json, api/swagger.yaml
#   - Sonarr OpenAPI client: internal/gen/sonarr/sonarr.go
#   - gRPC-Web stubs:        internal/genproto/ (Go) and ui/src/gen/ (TypeScript), from proto/
# Run before build, test, or when generator directives or .proto files change.
generate:
	go generate ./...

# Build all Go binaries (compile and discard; use build-server/build-agent for output).
# Runs code generation first. Useful as a quick compile check.
build: generate
	go build ./...

# Build the metarr-server binary to bin/metarr-server.
# Runs code generation first. Output is a statically-linked executable.
build-server: generate
	go build -ldflags "$(LDFLAGS)" -o bin/metarr-server ./cmd/metarr-server

# Build the metarr-agent binary to bin/metarr-agent.
# Runs code generation first. Output is a statically-linked executable for deployment to NAS/remote hosts.
build-agent: generate
	go build -ldflags "$(LDFLAGS)" -o bin/metarr-agent ./cmd/metarr-agent

# Build metarr-server with the UI embedded, for production deployment.
# Builds the UI first (yarn workspace @metarr/metarr-ui run build), then compiles the Go binary with the embed_ui tag so the built
# assets are baked into the binary. Serves the UI at / and the API at /api from a single process.
build-server-production: generate ui-build
	go build -tags embed_ui -ldflags "$(LDFLAGS)" -o bin/metarr-server ./cmd/metarr-server

# Start the full Metarr stack: Docker services + server + agent + UI.
# Launches docker compose, builds and starts the server and agent in the background, then runs the UI in the foreground.
# The server listens on http://localhost:8080, the UI on http://localhost:5173.
# Logs for server and agent are written to /tmp/metarr-*.log. Stop with Ctrl-C.
#
# On Ctrl-C, a trap kills all background processes and runs `make down` to shut down docker compose.
#
# Alternative: Run components in separate terminals for independent control:
#   Terminal 1: docker compose up
#   Terminal 2: make run-server
#   Terminal 3: make run-agent
#   Terminal 4: make ui-dev
run: build-server build-agent
	@echo "=== Starting Metarr stack (docker compose + server + agent + ui) ===" && \
	docker compose up -d && sleep 2 && \
	trap "echo '=== Shutting down...'; pkill -f 'bin/metarr-server' || true; pkill -f 'bin/metarr-agent' || true; sleep 1; docker compose down" EXIT INT TERM && \
	export METARR_CONFIG_FILE=config/server.local.yaml && \
	./bin/metarr-server > /tmp/metarr-server.log 2>&1 & \
	METARR_AGENT_SLUG=local METARR_REDIS_HOST=localhost ./bin/metarr-agent > /tmp/metarr-agent.log 2>&1 & \
	echo "=== Server and agent running in background, logs at /tmp/metarr-*.log ===" && \
	echo "=== Starting UI in foreground (Ctrl-C to stop all) ===" && \
	yarn workspace @metarr/metarr-ui run dev

# Stop all Metarr components: Docker services, server, agent, and UI.
# Kills any lingering processes and brings down docker-compose.
down:
	@echo "=== Stopping Metarr stack ===" && \
	pkill -f 'bin/metarr-server' || true && \
	pkill -f 'bin/metarr-agent' || true && \
	pkill -f 'metarr-ui run dev' || true && \
	sleep 1 && \
	docker compose down && \
	echo "=== Stack stopped ==="

# Build and run metarr-server with the UI embedded for production.
# Builds the production binary (with -tags embed_ui) and runs it in the foreground.
# The server listens on http://0.0.0.0:8080 and serves the UI at / and the API at /api.
# Requires MongoDB and Redis running (docker compose up).
# Stop with Ctrl-C.
run-production: build-server-production
	METARR_CONFIG_FILE=config/server.local.yaml ./bin/metarr-server

# Start the metarr-server in the foreground.
# The server listens on http://0.0.0.0:8080 and connects to MongoDB + Redis.
# It reads config/server.yaml (or the file named by METARR_CONFIG_FILE env var) on startup.
# Stop with Ctrl-C. Requires: MongoDB and Redis running (docker compose up).
run-server: generate
	go run ./cmd/metarr-server

# Start the metarr-agent in the foreground.
# The agent connects to Redis only (no direct database access).
# Configurable via environment variables:
#   METARR_AGENT_SLUG    — unique identifier for this agent instance (default: "local")
#   METARR_REDIS_HOST    — Redis hostname (default: "localhost")
#   METARR_REDIS_PORT    — Redis port (default: "6379")
#   METARR_REDIS_PASSWORD — Redis password (must match config/server.yaml)
#
# Example: make run-agent METARR_AGENT_SLUG=nas-01 METARR_REDIS_HOST=192.168.1.100
run-agent: generate
	METARR_AGENT_SLUG=$(or $(METARR_AGENT_SLUG),local) \
	METARR_REDIS_HOST=$(or $(METARR_REDIS_HOST),localhost) \
	go run ./cmd/metarr-agent

# Build cross-compiled agent binaries for distribution to NAS/remote hosts.
# Produces static binaries (no CGO dependencies) for Linux (amd64, arm64),
# Windows (amd64), and macOS (arm64).
# Set VERSION to embed a version string: make dist VERSION=1.0.0
# Default VERSION is read from the VERSION file at the repo root.
VERSION ?= $(shell cat VERSION)
LDFLAGS := -s -w -X Metarr/internal/shared/version.Raw=$(VERSION)

dist: dist-agent-linux-amd64 dist-agent-linux-arm64 dist-agent-windows-amd64 dist-agent-darwin-arm64

# Build the metarr-agent for Linux x86-64 (Intel/AMD 64-bit).
# Output: bin/metarr-agent-linux-amd64
dist-agent-linux-amd64: generate
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
		-ldflags "$(LDFLAGS)" -o bin/metarr-agent-linux-amd64 ./cmd/metarr-agent

# Build the metarr-agent for Linux ARM 64-bit (QNAP, Synology, most modern NAS).
# Output: bin/metarr-agent-linux-arm64
dist-agent-linux-arm64: generate
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
		-ldflags "$(LDFLAGS)" -o bin/metarr-agent-linux-arm64 ./cmd/metarr-agent

# Build the metarr-agent for Windows x86-64.
# Output: bin/metarr-agent-windows-amd64.exe
dist-agent-windows-amd64: generate
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath \
		-ldflags "$(LDFLAGS)" -o bin/metarr-agent-windows-amd64.exe ./cmd/metarr-agent

# Build the metarr-agent for macOS ARM 64-bit (Apple Silicon).
# Output: bin/metarr-agent-darwin-arm64
dist-agent-darwin-arm64: generate
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath \
		-ldflags "$(LDFLAGS)" -o bin/metarr-agent-darwin-arm64 ./cmd/metarr-agent

# Build Docker images for metarr-server and metarr-agent.
# Produces: metarr-server:<VERSION> and metarr-agent:<VERSION>
# Set VERSION to tag the images: make docker-build VERSION=1.0.0
# Default VERSION is read from the VERSION file at the repo root.
docker-build:
	docker build --build-arg VERSION=$(VERSION) -f deploy/Dockerfile.server -t metarr-server:$(VERSION) .
	docker build --build-arg VERSION=$(VERSION) -f deploy/Dockerfile.agent -t metarr-agent:$(VERSION) .

# Run the Go test suite.
# Runs all tests in ./... and re-generates code first.
# For faster testing, use the go-tests skill instead, which runs scoped tests only.
test: generate
	go test ./...

# Tidy the Go module: remove unused dependencies and update go.sum.
# Run occasionally or before committing go.mod changes.
tidy:
	go mod tidy

# Lint all code: Go (golangci-lint) and UI (ESLint).
# Runs both lint-go and lint-ui. See those targets for details.
lint: lint-go lint-ui

# Lint the Go code using golangci-lint.
# Checks for style, correctness, and common mistakes across the entire Go codebase.
# Run after making changes to catch issues early.
lint-go:
	go tool golangci-lint run ./...
