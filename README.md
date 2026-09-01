# Metarr

![Markdown Logo](documentation/modules/ROOT/images/logo.png)

Media collection metadata management system.

**[📖 Read the full documentation](https://scphantm.github.io/metarr/)** — built and published automatically to GitHub Pages.

For a local build: Full documentation — growing into the complete user manual — lives in [`documentation/`](documentation/), built with [Antora](https://antora.org/) from AsciiDoc. Run `make docs-initialize` once, then `make docs-build`, and open `build/site/index.html`.

## Why Metarr

Radarr, Jellyfin, and your NAS's shared drive each end up holding their own copy of your media metadata, and all of them believe they are the system of trueth. Metarr resolves the debate: it treats the files on your shared drive as the source of truth, edits them directly, and notifies Radarr, Jellyfin, and anything else watching that something changed, letting the other system eventually get in sync. — see [Why Metarr](documentation/modules/ROOT/pages/philosophy.adoc) in the docs.

## Project status

Metarr is under active development and is not yet a finished product. What works
end to end today:

* **Library scanning.** Agents walk your libraries and classify what they find —
  season and extras folders, disc structures (`VIDEO_TS`/`BDMV`), Jellyfin
  trickplay, multi-version and stacked files, Plex edition tags — and the server
  stores the result. See [Library scanning](documentation/modules/ROOT/pages/scanning.adoc).
* **Agent fleet management.** Deploy agents, watch presence and telemetry, map
  each library onto the path that machine knows it by.
* **Configuration and security.** Every setting above the connection layer is
  editable at runtime from the UI, with an admin account and four categories of
  API key.
* **Logging and observability.** Structured logs from both binaries, a live tail in the UI, and optional shipping to OpenObserve through Fluent Bit.
* **Workflow authoring and validation.** A graph editor over a large catalog, with continuous validation.
* **An AI assistant** that can see the workflow you are editing and propose changes to it.

What is **not** built yet, stated plainly because the code contains the
scaffolding for all of it:

* **Workflow execution.** Workflows can be authored, saved, versioned and
  validated. Nothing runs them — there is no executor, no node handlers and no
  run endpoint. The handler and dry-run contracts exist; nothing implements them.
* **Sonarr data fetching.** Sonarr instances can be configured and a generated
  client exists, but no Sonarr API call is ever made and no Sonarr data is cached.
* **Radarr, TMDB, TVDB, Plex and Jellyfin integration.** Not implemented. Metarr
  reads provider IDs out of existing NFO files; it does not call those APIs.
* **Scheduling and automation.** There is no cron, timer or trigger. Everything
  is user- or event-initiated.
* **Searches, Automations and Tasks.** These appear in the navigation marked
  "soon" and have no pages behind them.

## What this does not do

This does not look for missing media in your collection.  Other systems are designed to do that.  

## This sounds and looks like Tdarr

The easiest way to put it is Tdarr is used to encode video files; Metarr is for everything else.

## Architecture

Metarr is two Go binaries, a web UI, and the infrastructure they sit on:
MongoDB, Redis, and — optionally — Fluent Bit and OpenObserve for log shipping.

**metarr-server** owns the API, MongoDB, and orchestration. It never touches the
media library — it has no filesystem access to it at all.

**metarr-agent** is a small static binary deployed next to the storage or other machines in your network, ideally on the NAS itself or a machine with a GPU. 
It does every filesystem operation: walking
libraries, reading NFO files, inspecting artwork. It communicates with the metarr-server via a Redis event driven backend and
nothing else, holds no database credentials, and cannot open a database
connection — a test walks the build graph on every run to keep that true.

An agent is configured locally with two things: how to reach Redis, and its own
name. Everything else is published to it over Redis by the server. Start one and
it appears under **System > Agents** as connected but unconfigured; configuring
it means mapping the libraries the server knows about onto the paths that
machine knows them by. Records are always stored under the server's paths, so
the library reads the same however many agents produced it.

Agents run on Windows, Linux or macOS. Build one for a target with
`make dist`, which emits static binaries for each.

**The web UI** is a React application served separately — it is not embedded in
the server binary. In development it runs on Vite's dev server, which proxies
`/api` to the server; the Go API sends no CORS headers, so that proxy is required.

```
docker compose up                          # infrastructure: Mongo, Redis, Fluent Bit, OpenObserve, admin UIs
make run-server                            # the API (runs on the host, not in compose)
make run-agent METARR_AGENT_SLUG=nas-01    # an agent alongside it
make ui-dev                                # the web UI on :5173
```

On its first start the server generates an administrator password and a set of
API keys and prints them to stdout. Capture them then — they are not recoverable
afterwards.

## Architectural features

**Event-driven backend.** Work spreads across cores in parallel rather than
running sequentially, following an eventually-consistent model: saving a change
publishes an event and returns `202 Accepted`, and a listener persists it a
moment later. The change lands reliably, just not on the same tick as the click —
the UI shows each save moving from queued to confirmed.

**Deliberate transport choices.** Redis carries everything between the two
binaries, and what rides on which mechanism is a rule rather than a habit: keys
with a TTL for presence and telemetry (a dead agent simply expires), durable
streams for work that must survive a restart on either side, and pub/sub for
signals where a missed message costs latency rather than correctness.

**A hard agent boundary.** The agent holds no database credentials and cannot
open a database connection. That is enforced by a test that walks the real build
graph, not by convention — see [Architecture](documentation/modules/ROOT/pages/architecture.adoc).

## Documentation

The pages below live in [`documentation/`](documentation/) and build into a
searchable site with `make docs-build`.

| Page | What it covers |
| --- | --- |
| [Why Metarr](documentation/modules/ROOT/pages/philosophy.adoc) | The metadata-ownership problem, in full |
| [Architecture](documentation/modules/ROOT/pages/architecture.adoc) | Components, the agent boundary, event and transport reference |
| [Installation](documentation/modules/ROOT/pages/installation.adoc) | Running Metarr, first-boot credentials, ports |
| [Configuration](documentation/modules/ROOT/pages/configuration.adoc) | The two configuration layers, security model |
| [Agents](documentation/modules/ROOT/pages/agents.adoc) | Deploying agents and mapping libraries to them |
| [Library scanning](documentation/modules/ROOT/pages/scanning.adoc) | Scan directories, sidecar classification, what gets stored |
| [Workflows](documentation/modules/ROOT/pages/workflows.adoc) | The editor, validation, versioning — and what does not run yet |
| [Logging](documentation/modules/ROOT/pages/logging.adoc) | The log pipeline end to end |

### Design Documentation

Beyond the user manual, [`documentation/modules/design/`](documentation/modules/design/) holds the *authoritative* design specifications for the system — not just high-level philosophy, but the precise decisions about how things work, why alternatives were rejected, and what the implementation must match.

**For contributors:** Before implementing a change in an area the design covers, read what it specifies. If an implementation would require changing the design, ask before editing the design itself — it records decisions, not opinions.

**For AI agents:** This module is the source of truth. When designing or implementing anything in this codebase, verify that it matches what the design says.

## Gen AI position

This project is substantially AI-assisted — built on an architecture and feature set the author has designed professionally for years, not "vibe-coded." See [Why Metarr](documentation/modules/ROOT/pages/philosophy.adoc) in the docs for the full position, including how to feel about that if it matters to you as a user or contributor.



# References

Metarr's naming and metadata parsing conform to external specifications from
Jellyfin, Plex, and Kodi. The condensed, implementation-facing versions of these
live in the design docs — see the table below; the original sources are linked
from each page.

| Page | Source specs |
| --- | --- |
| [Movie naming](documentation/modules/design/pages/naming_movies.adoc) | [Jellyfin](https://jellyfin.org/docs/general/server/media/movies), [Plex](https://support.plex.tv/articles/naming-and-organizing-your-movie-media-files/) |
| [TV naming](documentation/modules/design/pages/naming_tv.adoc) | [Jellyfin](https://jellyfin.org/docs/general/server/media/shows/), [Plex](https://support.plex.tv/articles/naming-and-organizing-your-tv-show-files/) |
| [NFO format](documentation/modules/design/pages/nfo_format.adoc) | [Kodi](https://kodi.wiki/view/NFO_files/Templates) |