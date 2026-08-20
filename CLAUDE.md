# Post processing cleanup

Claude has a habit of leaving node or metarr running after a prompt is processed.  always kill any spawned processes after the prompt is processed.

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

# Workflow UI Node Design Pattern

See `design.md` for the full workflow specification — schema, execution semantics, validation rules and the reasoning behind them. This section is only the UI-side node anatomy.

* All inputs on top, all outputs on the bottom, error handle on the side. Concretely: **top edge** = control-in (leftmost) then data-ins; **bottom edge** = control-outs (leftmost) then data-outs; **right edge** = `error`.
* There are two kinds of edge and they must be visually distinct: **control** edges (thick, solid, neutral, animated) say what runs next; **data** edges (thin, coloured by type family) wire a value into a parameter. Never style them the same.
* Input nodes do not have control inputs — they are starting points. Output nodes do not have control outputs — they are ending points. This comes from the catalog's `control` block (an empty `in` array *is* "starting point"), never from hardcoded category checks.
* `category` is a presentation-only grouping label. It must never drive behaviour — dispatch on the node's `type`.
* A control out-port takes **exactly one** edge; a data-in socket takes **exactly one**; a data-out may feed many; a control-in accepts many. Parallelism is expressed with an explicit `core/parallel` node, never by dragging a second wire off one output.
* Handle ids encode port kind so the client can pre-filter without a catalog lookup: `c:in`, `c:next`, `c:error`, `d:source`.
* A port's `name` is a permanent identifier that stored edges reference — renaming one breaks every saved workflow. Display text goes in `label`, which may change freely.

# Workflow Engine

* The workflow engine operates in dry-run mode by default. The only way a workflow function can manipulate files is if dry-run is disabled for that run — done manually when executing a workflow, or by an automation executing it in production mode.
* Dry-run is enforced by capability, not by convention. A node handler never imports `os` or `os/exec`. It receives its filesystem and process-execution capability from the executor harness (`workflow.NodeContext`), and under dry-run those mutating operations log the intended action and return success without touching the disk. A handler that forgets to check a flag still cannot write, because it has no path to the filesystem to write through. Never give a handler direct filesystem access "just this once" — that is the whole guarantee gone.
* Every catalog entry declares `exec.effects` (`read` | `write` | `destructive`). It is mandatory; an entry without it is a catalog load error. It is what dry-run keys off, and it cannot be retrofitted without re-auditing every handler ever written.
* The agent enforces dry-run itself rather than trusting the server's decision — the handler runs on the agent, so the gate has to be there too.