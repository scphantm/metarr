# Captures every word after "purge-git-history" as arguments instead of
# letting Make treat them as target names, so `make purge-git-history a b`
# works. This means a filename that happens to match another Makefile
# target's name (e.g. "build") can't be purged via this positional form —
# use PURGE_FILES=path instead in that case.
ifeq (purge-git-history,$(firstword $(MAKECMDGOALS)))
  PURGE_FILES := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
  $(eval $(PURGE_FILES):;@:)
endif

.PHONY: generate build run test tidy install-cli purge-git-history

generate:
	go generate ./...

build: generate
	go build ./...

run: generate
	go run ./cmd/api

test: generate
	go test ./...

tidy:
	go mod tidy

install-cli:
	go install ./cmd/metarrctl

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
