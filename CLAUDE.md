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