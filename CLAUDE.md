# Planning

Claude is not permitted to edit any code under any circumstances without presenting a plan and having it approved first.  No exceptions.

# Code Quality
* All code should have descriptive variable names and follow standard accepted clean code practices.

# Changes to the configuration file structure

Any time changes are made to the configuration file structure, also execute these commands

* update the config api to include crud methods for the new configuration settings in the configuration router
* update the configuration UI to manage the new configuration settings
* update the database initialization in `/cmd/metarr-server/main.go` to initialize the setting.
* if the setting is something an agent needs, add it to `agentregistry.BuildProjection` — deliberately, since everything in the projection is readable by every agent host.

# Data update methods

When adding crud methods to the api for a block of data, instead of creating separate POST and PUT methods, 
create a single POST that upserts

# External API Calls

All http calls to external api's should be done with the cached http client.  This ensures that all calls are cached
and reusable based on the configuration settings.

# Unit Testing
When doing validations after making changes.  Generate regression unit tests that will allow for less number of tokens to be 
required in subsequent changes.

# metarr-server
* Metarr server is the only entity allowed to connect to mongo.  All data transmission between the server and the agent must go thru the event bus.

# metarr-agent

* in all file system operations, take into account the requirement that the agent may be running on one of the following operating systems
  * Windows
  * Linux
  * macOS.
* This cuts both ways: the *server* receives paths produced by an agent on any of those, so server-side code handling agent paths must not use `path/filepath`, which is compiled for the server's own OS. See `agentregistry.PathTranslator`.
* The no-mongo rule is enforced by `internal/agent/boundary_test.go`, which walks the real build graph. Do not weaken it.
* Package layout: `internal/agent/` (filesystem work), `internal/server/` (mongo, http, listeners), `internal/shared/` (event contracts, config, models). Nothing under `internal/agent/` may import `internal/server/`.
* The agent is configured locally with only a Redis connection and a slug. Everything else reaches it as a redacted projection built by `agentregistry.BuildProjection` — never by subscribing to `system_config_update`, whose payload carries the admin hash and every API key.
* Records are stored under the server's canonical paths. Results arriving from an agent are translated by `agentregistry.PathTranslator` before they are persisted.

# Logging

* Never build a log message from `fmt.Sprintf` or string concatenation. Pass dynamic values as trailing key-value pairs (`logger.Info("scan started", "scan_id", id)`), never embedded in the message string — the logging pipeline ships every attribute as a searchable field in OpenObserve, but a flattened message string is not searchable the same way.
* Every logger carries a fixed `source` field identifying which process emitted the record: `metarr-server`, or `metarr-agent-{slug}` for an agent. Never omit it, never overload it for anything else.
* Log level is runtime-adjustable, never a restart-requiring constant. The server's level lives in `appconfig.Logging.ServerLevel`; each agent's lives on its own `AgentConfig.LogLevel`. Both are set from the System > Logging screen.
* Logging must never block application code. A log call enqueues onto a bounded buffer and returns immediately; if the buffer is full, the record is dropped and counted rather than blocking the caller.
* Neither binary talks to OpenObserve (or any log vendor) directly. Both publish to the `logs.app` Redis Pub/Sub channel — this is as far as an agent's own responsibility goes. Fluent Bit has no Redis input plugin (checked against the real plugin list — do not assume otherwise), so only `metarr-server` goes one hop further: it also subscribes to `logs.app` and forwards each record over HTTP to Fluent Bit's `http` input (`internal/server/logforward`), configured via `logging.forward_url` in `config.yaml` — infrastructure wiring, not `appconfig`, since it's a deployment detail rather than a runtime setting. Swapping vendors is still only a `fluent-bit/fluent-bit.conf` change — the OUTPUT block, never the INPUT — so no Go code changes and no agent redeploys either way.