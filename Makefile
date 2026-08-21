include Makefile.documentation

.PHONY: generate build build-server build-agent run run-server run-agent test tidy \
	dist dist-agent-linux-amd64 dist-agent-linux-arm64 \
	dist-agent-windows-amd64 dist-agent-darwin-arm64 \
	ui-install ui-dev ui-build docker-build purge-git-history \
	lint lint-go lint-ui \
	docs-install docs-build docs-serve

generate:
	go generate ./...

build: generate
	go build ./...

# Per-binary builds that actually produce something, unlike `build`, which
# compiles and discards. Output goes to bin/.
build-server: generate
	go build -o bin/metarr-server ./cmd/metarr-server

build-agent: generate
	go build -o bin/metarr-agent ./cmd/metarr-agent

# `run` stays the server, which is what it has always meant.
run: run-server

run-server: generate
	go run ./cmd/metarr-server

# The agent needs only a slug and a Redis connection. Override either:
#   make run-agent METARR_AGENT_SLUG=nas-01 METARR_REDIS_HOST=10.0.0.5
run-agent: generate
	METARR_AGENT_SLUG=$(or $(METARR_AGENT_SLUG),local) \
	METARR_REDIS_HOST=$(or $(METARR_REDIS_HOST),localhost) \
	go run ./cmd/metarr-agent

# Cross-compiled agent binaries. The agent is meant to drop onto a NAS as a
# single static file, and arm64 covers most of them.
VERSION ?= dev
AGENT_LDFLAGS := -s -w -X main.version=$(VERSION)

dist: dist-agent-linux-amd64 dist-agent-linux-arm64 dist-agent-windows-amd64 dist-agent-darwin-arm64

dist-agent-linux-amd64: generate
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
		-ldflags "$(AGENT_LDFLAGS)" -o bin/metarr-agent-linux-amd64 ./cmd/metarr-agent

dist-agent-linux-arm64: generate
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
		-ldflags "$(AGENT_LDFLAGS)" -o bin/metarr-agent-linux-arm64 ./cmd/metarr-agent

dist-agent-windows-amd64: generate
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath \
		-ldflags "$(AGENT_LDFLAGS)" -o bin/metarr-agent-windows-amd64.exe ./cmd/metarr-agent

dist-agent-darwin-arm64: generate
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath \
		-ldflags "$(AGENT_LDFLAGS)" -o bin/metarr-agent-darwin-arm64 ./cmd/metarr-agent

docker-build:
	docker build -f Dockerfile.server -t metarr-server:$(VERSION) .
	docker build -f Dockerfile.agent -t metarr-agent:$(VERSION) .

test: generate
	go test ./...

tidy:
	go mod tidy

lint: lint-go lint-ui

lint-go:
	go tool golangci-lint run ./...

lint-ui:
	cd ui && npm run lint

ui-install:
	cd ui && npm install

# The dev server proxies /api to localhost:8080, so the API needs to be running
# alongside it (make run). Point the proxy elsewhere with METARR_API_URL.
ui-dev:
	cd ui && npm run dev

ui-build:
	cd ui && npm run build
