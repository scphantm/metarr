# Captures every word after "purge-git-history" as arguments instead of
# letting Make treat them as target names, so `make purge-git-history a b`
# works. This means a filename that happens to match another Makefile
# target's name (e.g. "build") can't be purged via this positional form —
# use PURGE_FILES=path instead in that case.
ifeq (purge-git-history,$(firstword $(MAKECMDGOALS)))
  PURGE_FILES := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
  $(eval $(PURGE_FILES):;@:)
endif

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

docs-install:
	cd documentation && npm install

# The Antora playbook builds from whatever's currently checked out (branches:
# HEAD against the repo itself), so this always reflects local changes —
# no publish/remote-fetch step needed to preview docs.
docs-build: docs-install
	cd documentation && npm run build

docs-serve: docs-build
	cd documentation && npm run serve

# Permanently removes one or more paths from every commit on every branch,
# using git-filter-repo (brew install git-filter-repo). This rewrites commit
# hashes repo-wide and cannot be undone except by restoring the backup
# bundle this target writes to .git/purge-backups/ before touching anything.
#
# Usage:
#   make purge-git-history secrets.env
#   make purge-git-history secrets.env http-client.env.json
#   make purge-git-history PURGE_FILES=path/with\ a\ space
#   make purge-git-history secrets.env FORCE=1     # skip the confirmation prompt
#
# If the repo has a remote, every affected branch needs a force-push
# afterwards, and anyone else's clone still has the data in its own history
# until they re-clone or hard-reset to match.
purge-git-history:
	@if [ -z "$(strip $(PURGE_FILES))" ]; then \
		echo "Usage: make purge-git-history <path> [<path> ...]"; \
		echo "   or: make purge-git-history PURGE_FILES=\"<path> [<path> ...]\""; \
		exit 1; \
	fi
	@command -v git-filter-repo >/dev/null 2>&1 || { \
		echo "git-filter-repo is required: brew install git-filter-repo"; \
		exit 1; \
	}
	@git rev-parse --is-inside-work-tree >/dev/null 2>&1 || { \
		echo "Not inside a git repository."; \
		exit 1; \
	}
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Working tree is not clean. Commit or stash changes first."; \
		exit 1; \
	fi
	@echo "This will permanently rewrite git history to remove: $(PURGE_FILES)"
	@echo "Every commit hash on every branch changes as a result."
	@if [ -z "$(FORCE)" ]; then \
		printf "Type 'purge' to continue: "; \
		read confirm; \
		if [ "$$confirm" != "purge" ]; then \
			echo "Aborted."; \
			exit 1; \
		fi; \
	fi
	@mkdir -p .git/purge-backups
	@backup=".git/purge-backups/pre-purge-$$(date +%Y%m%d-%H%M%S).bundle"; \
	git bundle create "$$backup" --all >/dev/null && \
	echo "Backup written to $$backup (restore with: git clone $$backup)"; \
	for f in $(PURGE_FILES); do \
		[ -f "$$f" ] && cp "$$f" "$$f.purge-bak"; \
	done; \
	path_args=""; \
	for f in $(PURGE_FILES); do path_args="$$path_args --path $$f"; done; \
	echo y | git filter-repo --force --invert-paths $$path_args; \
	status=$$?; \
	for f in $(PURGE_FILES); do \
		[ -f "$$f.purge-bak" ] && mv "$$f.purge-bak" "$$f"; \
	done; \
	if [ $$status -ne 0 ]; then \
		echo "git-filter-repo failed; repo state may be partially rewritten. Restore from $$backup if needed."; \
		exit $$status; \
	fi; \
	remaining=$$(git log --all --oneline -- $(PURGE_FILES) | wc -l | tr -d ' '); \
	if [ "$$remaining" = "0" ]; then \
		echo "Confirmed: no commit on any branch references $(PURGE_FILES) anymore."; \
	else \
		echo "WARNING: $$remaining commit(s) still reference these paths. Investigate before trusting the purge."; \
	fi; \
	if [ -n "$$(git remote)" ]; then \
		echo "This repo has a remote configured: force-push every affected branch,"; \
		echo "and note the data is still in the history of any existing clone."; \
	fi


docs_initialize:
	npm update
	gem install bundle
	bundle update

# to configure the way the build presentations system works, check out this link
# https://docs.asciidoctor.org/reveal.js-converter/latest/setup/ruby-setup/
docs_build_presentations:
	bundle exec asciidoctor-revealjs --attribute revealjsdir=https://cdn.jsdelivr.net/npm/reveal.js@4.1.2 --out-file documentation/modules/ROOT/attachments/presentation.html documentation/modules/ROOT/pages/tests/presentation.adoc


subtree_docs_theme-add:
	git subtree add --prefix documentation-theme git@github.com:scphantm/metarr-documentation-theme.git metarr-main --squash

subtree_docs_theme-pull:
	git subtree pull --prefix documentation-theme git@github.com:scphantm/metarr-documentation-theme.git metarr-main --squash

subtree_docs_theme-push:
	git subtree push --prefix documentation-theme git@github.com:scphantm/metarr-documentation-theme.git metarr-main