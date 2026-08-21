# Metarr

Media collection metadata management system.

Full documentation — growing into the complete user manual — lives in [`documentation/`](documentation/), built with [Antora](https://antora.org/) from AsciiDoc. Run `make docs-build && make docs-serve` and open the printed URL to browse it locally.

## Why Metarr

Radarr, Jellyfin, and your NAS's shared drive each end up holding their own copy of your media metadata, and none of them agree on which copy is the truth. Metarr fills that gap: it treats the files on your shared drive as the source of truth, edits them directly, and notifies Radarr, Jellyfin, and anything else watching that something changed — rather than each system quietly maintaining its own database. Delete and rebuild all  of those other tools (Metarr included) at any time; point them back at the share and the library comes back exactly as it was.

For the full reasoning — the metadata-ownership problem in detail, and why this needed its own project rather than another *arr — see [Why Metarr](documentation/modules/ROOT/pages/philosophy.adoc) in the docs.

## What this does not do

This does not look for missing media in your collection.  Other systems are designed to do that.  

## This sounds and looks like Tdarr

The easiest way to put it is Tdarr is used to encode video files; Metarr is for everything else.

## Architecture

Metarr is two binaries.

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

```
make run-server                                    # the API
make run-agent METARR_AGENT_SLUG=nas-01            # an agent alongside it
docker compose up                                  # both, plus Mongo and Redis
```

## Architectural features

**Caching.** I/O is the slowest operation in computer science, and this application does thousands of them. Metarr caches virtually everything to avoid rate-limiting your API keys and to keep the system responsive; every cache TTL is configurable.

**Event-driven backend.** Jobs spread across cores in parallel rather than running sequentially, following an eventually-consistent model: saving a change kicks off work in the background, and the write lands once it finishes — not instantly on click, but reliably.

## Gen AI position

This project is substantially AI-assisted — built on an architecture and feature set the author has designed professionally for years, not "vibe-coded." See [Why Metarr](documentation/modules/ROOT/pages/philosophy.adoc) in the docs for the full position, including how to feel about that if it matters to you as a user or contributor.



# References
This system interfaces other systems.  These were the specifications that were used in building Metarr

## Directory Structures
* https://jellyfin.org/docs/general/server/media/shows/
* https://jellyfin.org/docs/general/server/media/movies

## Naming Conventions
* https://support.plex.tv/articles/naming-and-organizing-your-movie-media-files/
* https://support.plex.tv/articles/naming-and-organizing-your-tv-show-files/

## NFO Format
https://kodi.wiki/view/NFO_files/Templates