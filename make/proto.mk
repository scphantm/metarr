# Proto contract checks for the published metarr.bus.v1 event-bus contract.
# Included by the main Makefile.
#
# proto/metarr/bus/ is the language-agnostic event-bus contract (docs/adr/0008):
# a change there is a break of a published, versioned surface. The internal
# proto/metarr/v1/ CRUD/config/auth/stats protos deliberately carry no such
# guarantee and churn per release. .github/workflows/proto.yml runs these
# targets on every pull request and on pushes to main.

.PHONY: buf-lint buf-breaking proto-check

# Baseline a contract change is measured against. Overridden in CI to
# '.git#ref=origin/main' (a PR checkout has no local main branch). The '#' is
# escaped so make does not read it as a comment.
BUF_BREAKING_AGAINST ?= .git\#branch=main

# Lint every proto module against buf's STANDARD rules (see buf.yaml).
buf-lint:
	go tool buf lint

# Fail if the published metarr.bus.v1 contract has a breaking change relative
# to BUF_BREAKING_AGAINST. Scoped to proto/metarr/bus/ on purpose — only that
# module is frozen. Skips cleanly when the module does not yet exist on the
# baseline ref (the commit that first introduces it).
buf-breaking:
	@if go tool buf build "$(BUF_BREAKING_AGAINST)" --path proto/metarr/bus -o /dev/null >/dev/null 2>&1; then \
		echo "buf breaking: proto/metarr/bus vs $(BUF_BREAKING_AGAINST)"; \
		go tool buf breaking --against "$(BUF_BREAKING_AGAINST)" --path proto/metarr/bus; \
	else \
		echo "buf breaking: proto/metarr/bus absent on $(BUF_BREAKING_AGAINST); nothing to compare"; \
	fi

# Run every proto contract check.
proto-check: buf-lint buf-breaking
