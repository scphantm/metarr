# Documentation (Antora/AsciiDoc) build, serve, and theme tasks.
# Included by the main Makefile.

.PHONY: docs-initialize docs-install docs-theme-install docs-build docs-theme-pack \
	docs-antora-run docs-serve docs-watch docs-shell docs-build-presentations \
	subtree-docs-theme-add subtree-docs-theme-pull subtree-docs-theme-push \
	docs-theme-clean docs-clean

# Install all documentation and theme dependencies via yarn workspaces.
docs-initialize: docs-install

# Install documentation workspace dependencies.
docs-install: docs-theme-install
	yarn workspace @metarr/documentation install

# Install documentation-theme workspace dependencies.
docs-theme-install:
	yarn workspace @metarr/metarr_docs_theme install

# Build the documentation site: pack theme first, then run Antora.
docs-build: node-sync-version docs-theme-pack docs-antora-run

# Pack the documentation theme (create ui-bundle.zip).
docs-theme-pack: docs-theme-install
	yarn workspace @metarr/metarr_docs_theme run pack

# Run Antora to generate the documentation site.
docs-antora-run: docs-install
	npx antora generate documentation/antora-playbook.yml --stacktrace

# Serve the built documentation site (build/site) locally over HTTP.
# Uses `npx http-server`, fetched on demand the same way `docs-build` fetches antora via npx.
# Run `make docs-build` first if build/site doesn't exist yet.
#
# Usage: make docs-serve
# Then navigate to http://localhost:8090 in your browser.
docs-serve: docs-build
	npx http-server build/site -p 8090

# Serve the built documentation site (build/site) locally over HTTP, watching
# the documentation/ directory and regenerating the site automatically.
#
# Uses `npx browser-sync` (static server + auto browser reload) and
# `npx nodemon` (source watcher that reruns `docs-antora-run` on change),
# fetched on demand the same way `docs-build` fetches antora via npx.
#
# Usage: make docs-watch
# Then navigate to http://localhost:8090 in your browser.
docs-watch: docs-build
	trap 'kill 0 2>/dev/null; exit' EXIT INT TERM; \
	npx browser-sync start --server build/site --files "build/site/**/*" --port 8090 --no-open --no-ui & \
	npx nodemon --watch documentation --ext adoc,yml,yaml,png,jpg,jpeg,gif,svg --ignore documentation/node_modules --exec 'make docs-antora-run' & \
	wait

# Launch an interactive shell inside the deploy/Dockerfile.documentation container.
# Useful for debugging Antora builds, testing asciidoctor extensions, or running commands
# in the exact environment the CI uses. The repo is mounted at /workspace with the same
# directory structure, so `documentation/antora-playbook.yml` resolves the same way it
# does on the host (its paths are relative to the playbook file, not the working dir).
#
# Usage: make docs-shell
# Inside the container, you can run: yarn workspace @metarr/documentation run build
docs-shell:
	@echo "=== Building documentation container ===" && \
	docker build -f deploy/Dockerfile.documentation -t metarr-docs-shell:latest . && \
	echo "=== Launching shell in documentation container ===" && \
	echo "The repo is mounted at /workspace. To build docs:" && \
	echo "  cd /workspace && yarn workspace @metarr/documentation run build" && \
	echo "" && \
	docker run -it --rm \
		-v $$(pwd):/workspace \
		-w /workspace \
		metarr-docs-shell:latest \
		/bin/sh

# to configure the way the build presentations system works, check out this link
# https://docs.asciidoctor.org/reveal.js-converter/latest/setup/ruby-setup/
docs-build-presentations:
	BUNDLE_GEMFILE=documentation/Gemfile bundle exec asciidoctor-revealjs --attribute revealjsdir=https://cdn.jsdelivr.net/npm/reveal.js@4.1.2 --out-file documentation/modules/ROOT/attachments/presentation.html documentation/modules/ROOT/pages/tests/presentation.adoc

subtree-docs-theme-add:
	git subtree add --prefix documentation-theme git@github.com:scphantm/metarr-documentation-theme.git metarr-main --squash

subtree-docs-theme-pull:
	git subtree pull --prefix documentation-theme git@github.com:scphantm/metarr-documentation-theme.git metarr-main --squash

subtree-docs-theme-push:
	git subtree push --prefix documentation-theme git@github.com:scphantm/metarr-documentation-theme.git metarr-main

# Clean documentation theme build artifacts.
docs-theme-clean:
	rm -Rf documentation-theme/node_modules
	rm -Rf documentation-theme/build
	rm -Rf documentation-theme/public

docs-clean: docs-theme-clean
	rm -Rf node_modules
	rm -Rf build
