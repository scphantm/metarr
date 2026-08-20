# Metarr Workflow Design

The design record for visually-designed, engine-executed workflows.

**Status: design agreed, not yet implemented.** No execution engine exists. This document is the specification the implementation follows, and the reason particular choices were made. Where a decision was made against an obvious alternative, the alternative and its failure mode are recorded so the decision is not silently relitigated later.

---

## 1. What a workflow is

A workflow is a graph the user draws. It describes work done to media: read a path, inspect it, branch on what was found, invoke ffmpeg or a built-in function, write results back.

Two things flow through the graph, and they are **not the same thing**:

- **Control flow** — *what runs next*. Branches, loops, parallelism, error paths.
- **Data flow** — *which value feeds which parameter*. Paths, numbers, records.

These are drawn as two visually distinct kinds of edge. This is the single most important decision in the design, and it is a deliberate departure from Tdarr (whose catalog format this project's originally derived from), which has one edge kind and a shared mutable variable bag.

**Why the departure.** The current catalog demonstrates the problem: `Check flow variable` declares outputs `yes` and `no`, typed `boolean`, described as *"Edge taken if the check succeeds"*. Those are not booleans and nothing consumes them as data — they are branches wearing a data costume. Meanwhile `Input Path` declares output `input` typed `path`, which is a genuine value. One field, two incompatible meanings. Separating them lets each be validated properly: control edges are checked for reachability and deadlock, data edges are checked for type compatibility and availability.

The cost is honest: more handles per node and more wires to draw than Tdarr requires. That is accepted in exchange for a graph that can be statically checked before it is ever run.

---

## 2. Engine: build a custom executor

Surveyed Temporal, Cadence, `go-workflows`, Conductor, Argo, Flyte, Prefect, Airflow, Dagger, and the small Go DAG libraries.

**Rejected, and why it is not primarily about weight.** Temporal and Cadence require a separate server cluster plus its own datastore; Temporalite is archived and SQLite persistence is documented dev-only. Conductor is a JVM server needing Redis *and* Elasticsearch. Argo requires Kubernetes. Flyte/Prefect/Airflow are Python-ecosystem with no real Go authoring path.

The decisive argument is that **none of them execute this model**. They all run dependency graphs, where any node whose inputs are satisfied fires. We run an explicit control-flow walk driven by drawn edges, with branches, loops, joins and error ports. Adopting any of them still means writing the graph interpreter ourselves — so we would pay for a daemon, inherit determinism constraints, and inherit the in-flight-versioning hazard, while writing the same interpreter regardless.

**The one to revisit, later, once.** `cschleiden/go-workflows` (MIT, actively maintained but effectively single-maintainer) is the only candidate adding durable resumable execution **without a daemon** — it is a library, and it has a **Redis backend**, which is already deployed here. The motivating scenario is concrete: a multi-hour ffmpeg transcode when `metarr` restarts. Keep the engine's interfaces swappable. Do not adopt it now.

**Borrowed designs, not dependencies.** Conductor's `${taskRef.output.field}` reference syntax and n8n's first-class `error` connection type are good prior art. Node-RED embeds edges inside the source node — worse than the separate `edges[]` array already in use here.

---

## 3. Schema

### 3.1 Node type definition (the catalog)

The catalog is a hand-edited JSON file **owned by the server**, read at startup and served over `GET /api/workflows/catalog`. The UI palette, server-side validation, and the engine all read that one source of truth. It currently lives in the UI bundle, where the server cannot see it — that must move.

```json
{
  "type": "core/trickplay",
  "typeVersion": "1.0.0",
  "name": "Generate Trickplay",
  "category": "function",
  "description": "Generates Jellyfin-compatible trickplay tiles.",

  "control": { "in": ["in"], "out": ["next"], "error": true },

  "dataIn":  [ { "name": "source", "label": "Source file", "type": "path.file", "required": true } ],
  "dataOut": [ { "name": "trickplayDir", "label": "Trickplay folder", "type": "path.dir" } ],

  "settings": [
    { "name": "width", "label": "Tile width", "type": "number.int", "default": 320 }
  ],

  "exec": {
    "runsOn": "agent",
    "agentSelector": "path",
    "timeout": "2h",
    "cancellable": true,
    "effects": "write",
    "retry": { "attempts": 3, "backoff": "30s" }
  }
}
```

This splits three concepts that are currently mashed into `parameters` plus a free-form `data` bag:

| Concept | Field | Meaning |
|---|---|---|
| Control ports | `control` | Execution wiring. Not values. |
| Data sockets | `dataIn` / `dataOut` | Typed values, wired on the canvas. |
| Settings | `settings` | Literals entered in the node's editor. Never wired. |

Two structural consequences:

- **`category` becomes a pure UI grouping label with no behavioural meaning.** Today `nodeSockets.deriveSockets` hardcodes `category === 'input'` / `'output'` to decide which handles exist. That moves into the catalog: a start node simply declares `"control": { "in": [], "out": ["next"] }`. An empty `in` array *is* "this is a starting point". The TypeScript special-casing disappears entirely.
- **`type` is the single dispatch key**, replacing today's incoherent branching (on `category` for notes, on `pluginName` for checks). The catalog's `id` field — currently the duplicated string `Mc5PmBxBL` across five of six entries — and its vestigial `position` are both deleted.

**`name` on a port is a permanent identifier.** Stored edges reference it. Renaming one silently breaks every saved workflow. Display text goes in `label`, which may change freely.

**`exec.effects` is mandatory on every entry**, not optional and not deferred — it is what the engine's dry-run enforcement keys off (§14.1). An entry without it is a catalog load error.

### 3.2 Node instance and edges

```json
{ "id": "<uuid>", "type": "core/trickplay", "typeVersion": "1.0.0",
  "position": { "x": 0, "y": 0 },
  "settings": { "width": 320 },
  "promoted": ["width"],
  "label": "optional user override" }
```

```json
{ "id": "e1", "kind": "control",
  "from": { "node": "n1", "port": "yes" }, "to": { "node": "n2", "port": "in" } }

{ "id": "e2", "kind": "data",
  "from": { "node": "n1", "port": "trickplayDir" }, "to": { "node": "n2", "port": "source" },
  "transform": "parentDir" }
```

`promoted` lists settings that have been turned into wired data-in sockets — users immediately want a computed setting ("width derived from source resolution"). The field is reserved in v1 even if the UI ships later; retrofitting it into the node shape afterwards is far more disruptive.

### 3.3 Workflow document

Persisted shape gains a schema version:

```json
{ "schema_version": 1, "name": "…", "description": "…", "tags": ["…"],
  "nodes": [ … ], "edges": [ … ], "viewport": { … } }
```

Store the **canonical shape above**, not React Flow's `toObject()` output, with an adapter in the UI. React Flow's `type` field means *which component renders this* — a presentation concern currently conflated with node identity (`catalogNode` / `notesNode` / `checkFlowVariableNode`). The adapter is also the correct place to validate on load, replacing the unchecked `as Node[]` casts in `WorkflowEditorPage.snapshotFromWorkflow`.

> **Do not lose unknown nodes.** Today's opaque `[]bson.M` storage preserves nodes of unrecognised types by accident. A typed Go schema will *silently drop them* unless an explicit passthrough is kept for unrecognised fields and node types. This is the easiest way in this entire design to destroy a user's work, and it must be handled deliberately — see §9.

---

## 4. Type system

### 4.1 The lattice

A **dotted-prefix nominal hierarchy** plus one generic constructor. `a.b` is a subtype of `a`; the subtype test is `sub == super || strings.HasPrefix(sub, super+".")`. Three lines in Go, three in TypeScript, no lattice structure to maintain, and new types need no release.

```
any                              (top)

bool
number          number.int
string          string.enum
duration                         (milliseconds)
bytes                            (int64)
timestamp

path            path.dir
                path.file
                  path.file.video
                  path.file.image
                  path.file.subtitle
                  path.file.nfo

media.item                       (a scanned directory record — scanmodel.TVSeries)
media.file                       (scanmodel.MediaFile)
media.sidecar                    (scanmodel.SidecarFile)
metadata.nfo
metadata.stream                  (scanmodel.StreamDetails)

agent.slug
scanner.slug
error                            ({node, frame, code, message, agent, at})

list<T>                          (covariant in T)
```

**`path.file` and `media.file` are different types.** One is a string naming a location; the other is a database record. Trickplay wants a path; "set NFO title" wants a record. Conflating them is how every node ends up accepting `any`.

### 4.2 Paths are always server-canonical

`CLAUDE.md` already requires that records are stored under server-canonical paths and that agent-reported paths are translated by `agentregistry.PathTranslator` on arrival. That rule is extended to the graph: **every `path.*` value inside a workflow is in server-canonical space, always.** Translation to agent space happens once, in the dispatch layer, at the boundary.

This means the type needs no "which machine" parameter, and transforms like `parentDir` operate on server-canonical paths.

> **Prerequisite: `PathTranslator` has no reverse.** It currently only exposes `Path(agentPath) → serverPath`. Dispatch needs `ToAgent(serverPath) → agentPath` with the same Windows case-folding and separator handling. This must be written before agent dispatch can work.

A runtime check falls out of this: a node dispatched to agent X that receives a path not under a library mapped to X fails loudly, rather than executing against a path that does not exist on that machine.

### 4.3 Coercion

**Implicit** — engine-inserted, no `transform` recorded, no UI:

| From | To |
|---|---|
| `T` | `T` |
| `T` | any supertype of `T` (`path.file.video → path.file → path`) |
| `number.int` | `number` |
| `T` | `any` |
| `list<S>` | `list<T>` where `S <: T` |

**Explicit** — a named `transform` on the edge, offered by the UI:

| From | To | transform |
|---|---|---|
| `path.file` | `path.dir` | `parentDir` |
| `path.*` | `string` | `toString` |
| `path.file` | `string` | `fileName` / `baseName` / `extension` (ambiguous — always prompts) |
| `media.file` | `path.file` | `filePath` |
| `media.file` | `path.dir` | `directoryPath` |
| `media.item` | `path.dir` | `itemPath` |
| `duration` | `number` | `seconds` / `milliseconds` (ambiguous — always prompts) |
| `string` | `number` | `parseNumber` (may fail at runtime → node error) |
| `number` | `string` | `format` |
| `T` | `list<T>` | `wrap` |
| `list<T>` | `T` | **forbidden** — use `forEach` |

**`path.file → path.dir` is explicit, never implicit.** This is the case that motivated typing in the first place: a file path feeding a consumer that wants a directory passes the *parent directory*. That changes which thing on disk is being pointed at, so it must be visible on the wire — silently rewriting "write into this directory" to mean the media folder is exactly the failure typing exists to prevent. But it costs **one click, not a hunt**.

`transform` is a **single name, never a chain**. Chains are unreadable on an edge and untestable; useful compositions are registered as named transforms (`directoryPath` above is exactly that).

### 4.4 Connect-time behaviour

1. **During drag** — `isValidConnection` greys out every incompatible handle. Compatible means an implicit coercion exists, or at least one explicit transform does.
2. **Implicit match** — edge created silently, no `transform`.
3. **Exactly one explicit transform** — edge created with `transform` pre-filled and a small chip on the edge reading e.g. `parentDir`. Clicking the chip opens the picker.
4. **Several candidates** — inline picker at the drop point, no default pre-selected.
5. **Incompatible** — refused, with both type names in the message.

---

## 5. Execution semantics

A **multi-token control-flow interpreter**. A token is `(nodeID, frame)`.

### 5.1 Frames

A frame is the scope holding loop-iteration bindings and recorded node outputs.

```go
type Frame struct {
    Path     string // "/", "/n7#3", "/n7#3/n12#0"
    Parent   *Frame
    LoopNode string // "" at the root
    Index    int
}
```

The frame path is **deterministic and hierarchical**, not a random UUID. Deterministic makes it a natural database key and makes resume-after-restart possible at all; hierarchical makes "everything under iteration 3 of loop n7" an anchored prefix query.

Resolving data-in port `P` on node `N` in frame `F`:

1. Find the inbound data edge; take source node `S`, port `Q`.
2. Take `scope(S)` — the innermost loop whose body contains `S`, computed statically at compile time and cached on the edge.
3. Walk `F` up its parents until `frame.LoopNode == scope(S)`; call it `F_S`.
4. Read `outputs[S][F_S.Path][Q]`, apply the edge's `transform`, hand it to `N`.

Nested loops need no extra machinery — step 3 walks as far as necessary, giving the "closure over enclosing scope" behaviour users expect.

### 5.2 Parallelism is explicit; fan-out is forbidden

**A control out-port has arity exactly one.** A second edge from the same port is a validation error:

> `next` already goes to **Probe**. To run two things at once, insert a **Parallel** node.

Implicit fan-out was considered and rejected. It is visually identical to a conditional but semantically opposite (two wires off `caseA`/`caseB` means *one of these*; two wires off `next` would mean *both, at once*). Dragging a second wire would silently convert a sequential flow into a race — in this domain, two ffmpeg processes writing the same output file. Two implicitly-forked paths reconverging on an ordinary node would execute it **twice** in the same frame, with the second write clobbering an output another node may already have read. And it makes join arity unknowable, which makes deadlock-freedom undecidable.

```
core/parallel
  control in : in
  control out: branch1..branchN, error
  settings   : branches (2..8)
               onBranchError: cancelSiblings | waitForSiblings   (default cancelSiblings)

core/join
  control in : one per branch of the paired parallel
  control out: next, error
```

**Pairing is computed, not declared.** Every `parallel` P has exactly one `join` J forming a single-entry single-exit region: **P dominates J**, and **J post-dominates every branch entry of P**. Both come from a dominator tree and a post-dominator tree of the reversed graph (Cooper–Harvey–Kennedy iterative; workflow graphs are tens of nodes, so the quadratic worst case is irrelevant).

Plus one prohibition that buys everything: **a parallel branch may not contain a terminal node, and neither may a loop body** (use `break`). With well-nested regions, out-port arity of one, and no terminals inside branches, every branch provably reaches J exactly once — so join arity *is* the paired parallel's branch count, a static number that cannot be wrong. **Deadlock becomes structurally impossible.**

The only remaining way a branch fails to arrive is an error, handled by **poison tokens**: an unhandled error in a branch sends a poison token to J; J proceeds once it holds one token per branch, and fires `error` instead of `next` if any is poison. So `next` firing still means *every branch completed normally*, which is what the validation rule in §6 depends on.

Join state is keyed `(joinNodeID, frameID)` — a parallel inside a loop joins per iteration, and iterations must not contaminate each other's barrier.

**Runtime backstop.** Never fully trust the static analysis. A per-join watchdog fails the run past `joinTimeout` with a specific message naming the join, the branch, and asking for the run id as a bug report. Loud beats hung.

**The more useful parallelism is `forEach.parallelism`** — "transcode four episodes at once" is the actual user need. Parallel/join is for the rarer "do these two *different* things concurrently".

### 5.3 Loops

```
core/forEach
  control in : in
  control out: body   (once per item)
               done   (once, after the last iteration; immediately if empty)
               error
  dataIn     : collection : list<T>   required
  dataOut    : item  : T           }  body-scoped
               index : number.int  }
               count : number.int     (body- and done-scoped)
               failedCount : number.int  (done-scoped)
  settings   : parallelism : number.int          (default 1)
               onItemError : abort | skip        (default abort)
```

**Both `body` and `done` emanate from the `forEach` node itself.** `done` must not be modelled as flowing out of the last body node. This is what makes the zero-iteration path exist in the graph, which is what makes the validation rule in §6 reject body→after-loop data edges *for free*, with no special case.

An iteration ends when its frame's token count reaches zero — leaving a control-out unwired *is* `continue`. There is no end-of-body node. `core/break` (control-in, no control-out, legal only inside a body) terminates the enclosing loop and fires `done`.

### 5.4 Values escaping a loop: `core/collect`

```
core/collect
  control: in → next
  dataIn : value : T
  dataOut: collected : list<T>
```

Placed **inside** the loop body. The formal trick that avoids a special case: **`collect.collected` is attributed to the enclosing `forEach`'s `done` transition, not to the `collect` node.** For validation purposes the producer *is* the loop header — so a consumer downstream of `done` passes automatically, and a consumer inside the body or on a sibling path fails automatically.

Alternatives rejected:

- **Auto-collect into an array** breaks the type system, which is the entire point of typing. A socket declared `path.dir` would be `path.dir` inside the loop and `list<path.dir>` outside it; the editor could not type a wire without knowing the loop nesting of both endpoints, and moving a node across the boundary would silently retype every wire leaving it.
- **Last-value-wins** is silently wrong on an empty collection (no value exists, so every `required` input becomes secretly nullable) and non-deterministic under `parallelism > 1` (last by completion, not by index).
- **Forbid outright** kills the most common real pattern: "for each episode, probe it; then write one summary."

`collect` writes into `slot[iterationIndex]`, never appends — `collected` materialises in **collection order, never completion order**. Non-deterministic output ordering produces bug reports nobody can reproduce. Empty collection yields `[]`, never null.

### 5.5 Errors

Every node's `error` control-out carries an implicit companion data-out typed `error`. Because it is produced by the error exit, it is only readable downstream of that port — which falls out of §6 with no new rule.

**Retries happen before `error` ever fires.** *Node errors* (the handler failed) are distinct from *infrastructure errors* (agent offline, Redis unavailable, dispatch timeout). Infra errors retry per `exec.retry`; only exhausted retries surface on the port. Without this, an agent restart fails every long-running flow.

**An unwired `error` port aborts the run.** Handler search order: inside a loop body → the `forEach`'s `onItemError` decides; inside a parallel branch → poison token to the join, which fires *its* `error`, recursing outward; otherwise → abort, status `failed`.

There is deliberately **no workflow-level "on error, continue"**. Silently continuing past a failure in a media tool produces a half-processed library nobody notices for a month. If a user wants tolerance they wire the port. Abort is the only defensible default.

**Parallel siblings are cancelled** when a branch fails unhandled — an ffmpeg whose result will be discarded costs GPU-hours. `onBranchError: waitForSiblings` is the escape hatch.

**Loops** use `onItemError`: `abort` (stop, cancel in-flight iterations, propagate) or `skip` (record on the item's execution record, continue, expose `failedCount` on `done`). `skip` is what makes "process 5,000 episodes, three of which are corrupt" usable — but `failedCount` must be exposed so the flow can branch on it, otherwise the failures are invisible. A third `collectErrors` mode is a natural later addition and is reserved.

`core/fail` and `core/end` are explicit terminals, both forbidden inside parallel branches and loop bodies. A `switch` with no matching case and no `default` port is a **runtime error, not a silent stop** — silent stops are the largest source of "my workflow did nothing".

---

## 6. Validation

Three independent checks per data edge, plus local arity rules.

### 6.1 Check 1 — `MustHaveRun`

The intuition "the source must be guaranteed to have run before the target" is right. **Classical dominance is the wrong relation for it**, because it models fan-out as a *choice* (OR) when parallel fan-out is a *concurrency* (AND).

Concretely: in `Parallel → {A, B} → Join → X`, is A on every path from entry to X? No — `entry → P → B → J → X` misses it. Classical dominance therefore rejects a data edge `A → X`, even though both branches always run and the join waits for both, so A provably completed before X started.

The corrected relation, computed over the control-flow graph with a **meet operator chosen per node kind**. Let `Out(e)` for control edge `u → v` be `MustHaveRun(u) ∪ {u}`:

```
MustHaveRun(start)               = ∅
MustHaveRun(v), v is a join      = ⋃ over branches i of ( ⋂ over inbound edges e from branch i of Out(e) )
MustHaveRun(v), otherwise        = ⋂ over all inbound edges e of Out(e)
```

Intersection at ordinary merges is textbook dominance (only one predecessor ran). **Union at joins is the correction** (every branch ran). Back-edges resolve at the fixed point exactly as in dominator analysis. Initialise `MustHaveRun(v) = V` for all `v ≠ start` and iterate a worklist; both operators are monotone-decreasing on a finite powerset lattice, so it terminates.

**Valid iff `source ∈ MustHaveRun(target)`.**

One necessary carve-out: **pure data-source nodes are exempt.** A literal path, a library selector, a constant — these have *no control ports at all*, are not execution steps, and their value is always available.

### 6.2 Check 2 — frame visibility

`scope(source)` must be an ancestor-or-equal of `scope(target)` in the loop-nesting tree.

This is not implied by Check 1. Check 1 establishes that *some* value exists; this establishes that *this iteration's* value exists. It also produces a far better error message than a dominance failure would.

### 6.3 Check 3 — type compatibility

Per §4, including transform resolution.

### 6.4 Arity

- A `dataIn` socket accepts **exactly one** inbound data edge (two is ambiguity).
- A `dataOut` may feed **many**.
- A `controlIn` accepts **many**.
- A `controlOut` accepts **exactly one** (§5.2).

Data-graph acyclicity within a frame is implied by Check 1 — a cycle would require each node in the other's `MustHaveRun` — so it needs no separate check.

### 6.5 Diagnostics

A generic "dominance violation" is useless. Every failure carries a **witness**: a BFS from `start` to `target` avoiding `source`, highlighted on the canvas.

> Cannot connect **Generate Trickplay → trickplayDir** to **Write NFO → source**. *Generate Trickplay* does not run on every path to *Write NFO* — it is skipped when **Check codec** takes its **no** branch. [Show path]

> **Probe** and **Transcode** run at the same time in different branches of **Parallel**. Connect from a node before the parallel, or from after the join.

> Cannot connect **Probe → duration** to **Summary → text**. *Probe* runs once per item inside **For each episode**; *Summary* runs once, after the loop. Insert a **Collect** node inside the loop. [Insert Collect]

### 6.6 Where checks run

**Do not implement the graph analysis twice.**

- **Client (TypeScript, instant, during drag)** — cheap local checks only: port kind, arity, type compatibility, transform availability. That is all `isValidConnection` needs.
- **Server (Go, authoritative, debounced ~300ms)** — `POST /api/workflows/validate` returns `{severity, code, nodeIds, edgeIds, message, witnessPath}`, painted as canvas markers.

**Invalid data edges block *running*, not *saving*.** People save half-built flows. A run requires zero `error`-severity diagnostics.

---

## 7. Run state

Two collections. **Not** the versioned store — that is for user-authored documents where every save is a new immutable version; a run is mutated hundreds of times. And **not** embedded node executions — a `forEach` over 5,000 episodes with six nodes each is 30,000 subdocuments, far past the 16MB BSON limit.

### `workflow_runs` — one document per run

```
_id
workflow_document_id, workflow_version
graph: { nodes, edges }                       // FROZEN compiled copy
catalog_snapshot: { "core/trickplay@1.0.0": {…} }   // only types actually used
trigger: { kind, by, payload }
inputs
mode      : development | production        // §14 — governs UI feedback and debugging
dry_run   : bool                            // §14 — DEFAULTS TRUE, engine-enforced
log_level : info | debug                    // §15 — per-run override of the process level
work_dir  : "workflows/<runID>"             // §16 — relative to each machine's metarr dir
status: queued|running|paused|cancelling|succeeded|failed|cancelled
started_at, finished_at
error: { node_id, frame, code, message }
counters: { executed, failed, skipped, cancelled }
engine_instance_id, lease_expires_at
cancel_requested_at
breakpoints: [ "<nodeID>" ]                 // §14 — development mode only
expires_at
```

**Version pinning alone is insufficient.** It protects against the *user* editing the flow mid-run, but the catalog is a hand-edited file on the server's disk and a run lasts hours — an operator editing it would silently change an in-flight run's semantics. Freeze **both** the compiled graph and a snapshot of the catalog entries actually used. That is also the complete answer to "what if a node type changed mid-run": it cannot.

### `workflow_node_executions` — one document per execution

```
_id, run_id, node_id
frame        : "/" | "/n7#3" | "/n7#3/n12#0"
attempt      : 1
status       : pending|dispatched|running|succeeded|failed|cancelled|skipped
agent_slug   : "nas-01" | null            (null = ran on the server)
dispatch_correlation_id                   // joins to OpenObserve logs
inputs_resolved, outputs                  // post-transform: what the node actually received
error, started_at, finished_at, duration_ms, expires_at
```

**Key: `(run_id, node_id, frame, attempt)`, unique.** The deterministic frame path is what makes this work — a random per-iteration id would make retries and resume impossible. Indexes on `{run_id, node_id, frame}`, `{run_id, status}` for the live view, and a TTL on `expires_at`.

### Redis is working memory; Mongo is the audit log

Resolving data edges against Mongo means a round trip per edge per node per iteration. Outputs are written to `metarr:run:{runID}:out:{nodeID}:{frame}` (hash, TTL = run timeout plus slack) on the hot path and written through to the execution document for the record. Individual values are capped at 256KB — reusing the scanner's `maxResultBytes` precedent — and oversized values are rejected with a clear error rather than silently degrading.

### Crash recovery

The engine holds a lease (`engine_instance_id` + renewed `lease_expires_at`). On startup, sweep runs whose lease expired. **v1: mark them `failed` with reason `"engine restarted"` and offer one-click re-run.** Full resume — re-deriving the live token set from execution records — is a v2 feature that the deterministic frame keys leave open without a schema change.

### Retention

TTL on `expires_at`, window configurable in `appconfig`, failed runs pinned longer. Note that per `CLAUDE.md`, adding an `appconfig` setting obligates the config CRUD API, the config UI, and initialisation in `cmd/metarr-server/main.go`.

---

## 8. Agent dispatch

### Package layout

`internal/agent/boundary_test.go` walks the real build graph, so any accidental path from an agent handler into server code fails the build. The layout follows from that:

```
internal/shared/workflow/          NEW — contract, imported by both binaries
    catalog.go     NodeType, ControlPorts, Socket, Setting, ExecSpec
    types.go       type lattice + transform registry
    exec.go        NodeExecRequest / NodeExecResult / NodeCancel
    handler.go     the Handler interface (type only, no registry)

internal/shared/agentproto/        EXTEND
    NodeExecEventName   = "agent.node_exec"
    NodeCancelEventName = "agent.node_cancel"
    NodeResultStream    = "events.agent_node_results"

internal/agent/nodes/              NEW — agent handlers (ffmpeg, trickplay, file ops)
internal/agent/runtime/executor.go NEW — mirrors scanner.go

internal/server/workflow/{catalog,validate,engine,dispatch,store}/   NEW
internal/server/workflow/nodes/    NEW — server handlers (conditionals, variable ops, Mongo, Sonarr)
```

The handler **interface** is shared; the **registries are separate, one per binary**. The handler sets are disjoint, and a shared registry is precisely how a server import sneaks into the agent's build graph.

### Where a node runs

**`agentToRun` as a per-instance setting is a category error and is removed.** *Whether* a node can run on an agent is a property of the node **type**; *which* agent should be derived, because a filesystem only exists on the machine that has it mounted.

- `runsOn: "server"` — no agent involvement.
- `agentSelector: "path"` — **the default.** The agent is derived from which library the node's primary input path belongs to, via `agentregistry`. Almost always correct, and requires no user decision.
- `agentSelector: "setting:agentToRun"` — explicit override, for the transcode-on-the-GPU-box case.
- `agentSelector: "any"` — any online agent. Valid only for nodes touching no filesystem, which here is nearly none. A rare, deliberate choice.

### Transport

Follows the existing three-transport split exactly:

- **Work → durable stream** `events.agent.{slug}.commands`, event `agent.node_exec`. Must survive an agent restart, same as scans.
- **Results → a new shared stream** `events.agent_node_results` with its own consumer group. Not reused from scan results: those are per-item streaming with different ack semantics, and mixing them makes the group's lag metric meaningless.
- **Cancel → Pub/Sub** `agent.{slug}.request`, event `agent.node_cancel`. Best-effort and latency-sensitive; a cancel arriving after the job finished is worthless.
- **Progress → Pub/Sub**, per-run channel, for the live UI. Never to Mongo — a two-hour transcode reporting every second is 7,200 writes nobody reads afterwards.

### Contract details

- **Never Nack.** A node failing for a reason that will not change would be redelivered forever. Report failure as a result event and ack, exactly as the scanner does.
- **Idempotency.** Commands carry `(run_id, node_id, frame, attempt)`. The server deduplicates by applying results with a conditional update on `status ∈ {dispatched, running}`, so a late duplicate cannot resurrect a terminal execution.
- **Path translation at the boundary only.** `dispatch` calls `ToAgent` on every `path.*` value outbound and `Path` on every one inbound. The engine core never sees an agent-space path.
- **Fail fast at run start.** Before executing anything, verify every agent-run node's target agent is present and that the paths it will receive are mapped to it. Failing three hours in because `nas-02` was never online is unacceptable.
- **Presence-based liveness.** An agent losing its presence key beyond `PresenceTTL` fails its in-flight executions with a *retriable infra* error, so the retry policy applies.

### Cancelling a long ffmpeg

1. User cancels → run `status: cancelling`; the engine stops dispatching.
2. `agent.node_cancel` published per in-flight execution.
3. The agent cancels the handler's context.
4. **`exec.CommandContext` alone is not enough.** ffmpeg spawns children; killing the parent leaves them running. Handlers start the process in its own process group (`Setpgid` on Unix, Job Object or `taskkill /T /F` on Windows) and kill the **group**. This is a silent leak on all three supported OSes otherwise.
5. Agent replies `status: cancelled`.
6. Backstop: no ack in N seconds → `cancelled (unconfirmed)`, finish the run. The agent self-cancels if it observes the run is gone.

> **Partial output cleanup — the domain trap.** A killed ffmpeg leaves a truncated `.mp4` beside the media file, which the scanner will happily classify as a legitimate sidecar on the next pass. **Every write-effect handler writes to a temp path and renames on success.** This belongs in the handler contract, not in each handler's good intentions.

---

## 9. Catalog versioning and drift

- **Catalog entries are immutable per `(type, version)`.** The server hashes each entry at load and stores seen hashes; a changed hash on an unchanged version is a **startup error**. This matters more than usual because the catalog is a hand-edited file with no review process.
- **Compatible changes** for a version bump: adding an optional data-in, adding a data-out, adding a setting with a default. **Incompatible**: removing or renaming a port, making an input required, narrowing a type.
- **Resolution status on load**, per node instance: `ok` | `upgradable` | `missing` | `incompatible`.
- **Tombstone rendering.** A `missing`/`incompatible` node renders as a distinct tombstone that **preserves its stored settings and edges verbatim**. The flow opens, displays, and re-saves without loss; it simply cannot run. See the warning in §3.3 — a typed schema loses this unless the passthrough is deliberate.
- **Cheap migrations**: `"migrations": [{ "from": "1.0.0", "renamePorts": { "src": "source" } }]` covers the common rename without a code release.

---

## 10. Node anatomy (UI contract)

Amending `CLAUDE.md`'s existing pattern for the four-category split while keeping its spirit — all inputs on top, all outputs on the bottom, error to the side:

- **Top edge** — control-in (leftmost), then data-ins.
- **Bottom edge** — control-outs (leftmost), then data-outs.
- **Right edge** — `error`.
- **Input-category nodes** have no control-in; **output-category nodes** have no control-out. This now comes from the catalog's `control` block rather than hardcoded category checks.

Handle ids encode the port kind so the client can pre-filter without a lookup: `c:in`, `c:next`, `c:error`, `d:source`, `d:trickplayDir`. Today `nodeSockets.tsx` uses raw parameter names — that is a migration point.

Control and data edges are distinct React Flow edge types: **control** thick, solid, neutral, keeping the existing animated treatment; **data** thin, coloured by type family. The current `defaultEdgeOptions` applies the animation to everything and must apply to control only.

**Sticky notes** (`category: "note"`) have no ports, are stripped before compilation, and are excluded from every validation pass. They should be attachable to a node (React Flow `parentId`) so they move with what they annotate.

---

## 11. Migration

Existing saved workflows are junk data and can be deleted.

---

## 12. Build order

1. **`internal/shared/workflow`** — catalog types, type lattice, transform registry, **plus the `NodeContext` handler contract, the `effects` classification, and the run-scoped logger** (§14–§15). These are contract rather than runtime, and every handler's shape depends on them, so they cannot be deferred. Nothing works before this.
2. **Catalog loader + `GET /api/workflows/catalog`**, with immutability hashing. Point the palette at it; delete the client-side JSON.
3. **Compiler + validator** (§6) + `POST /api/workflows/validate`. Buildable and testable with **no engine at all**, and it is where all the subtle bugs live — so it gets the heaviest regression-test coverage, over small hand-built graphs.
4. **Editor changes** — split handles, control/data edge types, `isValidConnection`, transform chips, diagnostic markers.
5. **Run store (§7) + engine (§5) with server-only handlers.** Fully exercisable with no agent involved.
6. **Agent dispatch (§8)** — `NodeExec` messages, `internal/agent/runtime/executor.go`, `PathTranslator.ToAgent`, cancellation with process-group kill and temp-file-plus-rename.

---

## 13. Deferred, but reserved in the schema

These are not built in v1. Each is listed because the schema must not foreclose it:

- **Triggers.** The `core/start` node carries a `trigger` setting slot **now**, or every workflow authored in the interim needs rewriting. Its data-outs vary by trigger kind (`item : media.item` for scan-complete).
- **Cross-run concurrency.** Two runs over the same library concurrently means two ffmpegs writing the same file. Policy `singleton` (default) | `parallel`, implemented with the existing lock pattern (`metarr:workflow:{documentID}:lock`, `SET NX` with TTL refresh).
- **Large collections by reference.** `list<media.file>` over 50,000 files fits in no Redis value, Mongo document, or stream entry. A database-sourced collection is a **query descriptor plus a count**, materialised page-by-page by `forEach`; only `forEach` may consume a reference list, everything else gets a hard cap with a clear error. Retrofitting this after the engine assumes materialised slices is a rewrite.
- **Promoted settings** (§3.2) and the loop `collectErrors` mode (§5.5).
- **Full crash resume** (§7).



---

## 14. Run modes, dry-run, and debugging

### 14.1 Dry-run is the default and is engine-enforced

**Every run is a dry run unless explicitly told otherwise.** `dry_run` defaults to `true`. It is cleared only by a deliberate act: a human choosing "run for real", or an automation executing the workflow in production mode. A workflow can never mutate a library by accident, by default, or because a node was written carelessly.

Every catalog entry therefore **must** declare `exec.effects`, in v1, not later:

| `effects` | Meaning |
|---|---|
| `read` | Inspects only. Unaffected by dry-run. |
| `write` | Creates or modifies files. Neutered in dry-run. |
| `destructive` | Deletes or overwrites existing library content. Neutered in dry-run, and badged in the editor. |

This cannot be retrofitted — adding it later means re-auditing every handler ever written — which is why it is mandatory from the first entry.

### 14.2 Dry-run is enforced by capability, not by convention

The obvious implementation — the engine passes a `dryRun` flag and each handler checks it — is **handler discipline, not enforcement**. It fails the first time someone writes a handler that forgets, and the failure mode is silently deleting a user's media.

So instead: **handlers have no independent path to the filesystem.** A handler never imports `os` or `os/exec` directly. It receives its filesystem and process-execution capability from the executor harness:

```go
type NodeContext struct {
    FS      WorkFS   // Create/Write/Rename/Remove — no-ops that log when dry-run
    Exec    Runner   // spawns processes; refuses write-effect commands in dry-run
    Log     *slog.Logger
    WorkDir string
    DryRun  bool
}
```

In dry-run, `WorkFS` mutating operations record the intended action and return success without touching the disk. A handler that "forgets to check" therefore still cannot write, because there is nothing for it to write *through*. This is the difference between a convention and a guarantee.

The corollary is a standing rule: **never hand a handler direct filesystem access, however convenient.** One exception and the guarantee is gone for the whole system, because the guarantee is exactly "there is no other path".

Two consequences worth stating plainly:

- **Enforcement is duplicated on the agent, not delegated to it.** The dispatch envelope carries `dry_run`, and the agent-side executor applies the identical gate before invoking the handler. The server deciding is not sufficient, because the handler runs on the agent. As a belt-and-braces check the agent executor refuses outright to invoke a `destructive` handler when `dry_run` is set.
- **A dry run must still produce plausible outputs**, or nothing downstream can be simulated. A write-effect handler in dry-run computes and returns its output values (the path it *would* have written) exactly as normal — only the side effect is suppressed. Skipping the node instead would break every downstream data edge and make simulation useless.

### 14.3 Development and production mode

`mode` is a separate axis from `dry_run`, and conflating them would be a mistake: a production run may still be a dry run (a rehearsal), and that must remain expressible.

| | `development` | `production` |
|---|---|---|
| Real-time node highlighting | yes | no |
| Breakpoints and stepping | yes | no |
| Log window attached | yes | on demand |
| Typical `dry_run` | `true` | `false`, deliberately |

**Development mode** is what the editor's "run" button uses: pick an input media file, simulate the flow, watch it execute. Node status streams live over the existing WebSocket topic layer (`wsbus` + `useTopic`) on a per-run topic — that infrastructure already exists and needs no new transport.

**Breakpoints are node-level only.** The engine pauses *before* dispatching a node whose id is in `run.breakpoints`, sets run status `paused`, publishes the paused state, and waits for `step` or `continue`. There is deliberately no stepping *inside* a node — that is what logs are for, and offering it would mean building a debugger into every handler.

Two interactions that must not be overlooked:

- **A paused run must suspend node timeouts and the join watchdog.** Otherwise sitting on a breakpoint for five minutes trips `exec.timeout` and fails the run — a debugger that kills the program it is debugging.
- **Pausing is only legal in development mode.** A production run ignores `breakpoints` entirely rather than trusting the field to be empty.

### 14.4 The engine waits for remote nodes

Nodes run on the server by default; a node type declares `exec.runsOn: "agent"` when it must run elsewhere. **For agent nodes the engine does not advance that token until the remote result arrives.**

To be precise about what this does *not* mean: it does not make the transport synchronous. Work still goes over the durable per-agent command stream and results still come back over the shared result stream, because an ffmpeg transcode runs for hours and must survive a restart on either side. "Wait" is a statement about the *token*, not the socket — the engine holds that branch of execution until the result event lands, while other parallel branches continue.

---

## 15. Logging and observability

A workflow run is the unit people debug, and its work is spread across the server and one or more agents. The logging pipeline already carries structured records from every process to OpenObserve; what this adds is the discipline that makes a run queryable as a single thing.

### 15.1 Mandatory fields

Every log record emitted during a run carries these as **structured key-value attributes**, never interpolated into the message (per the existing logging rules — the pipeline indexes attributes, and a flattened message string is not searchable the same way):

| Field | Purpose |
|---|---|
| `workflow_run_id` | The one field that makes a run queryable end-to-end |
| `workflow_id`, `workflow_version` | Which flow, pinned to which version |
| `node_id`, `node_type` | Which step |
| `frame` | Which loop iteration / parallel branch (`/n7#3`) |
| `attempt` | Distinguishes retries |
| `agent` | Which machine, absent for server-side |
| `dry_run`, `mode` | So a simulated action is never mistaken for a real one |

`source` (`metarr-server` / `metarr-agent-{slug}`) is already mandatory everywhere and is unchanged. Because both server and agent publish to the same `logs.app` channel, a single query on `workflow_run_id` returns the complete story of a run regardless of which machine did the work — which is exactly the requirement.

### 15.2 Every effect is logged

Handlers do not choose what to log about their side effects, because they do not perform their side effects directly (§14.2). The harness logs every `WorkFS` and `Runner` operation — files read, written, renamed, deleted, and every process invoked with its arguments — including in dry-run, where the record states the action was simulated. This makes "what would this actually have done?" answerable without running it for real.

### 15.3 Per-run log level

A run carries its own `log_level`, overriding the process level for that run only. Setting one workflow to `debug` must not put the whole server into `debug`.

This is new machinery: today levels are per-process (`appconfig.Logging.ServerLevel`, and each agent's own `AgentConfig.LogLevel`), runtime-adjustable. A run-scoped level means the logger handed to handlers via `NodeContext.Log` consults the run's level rather than the process leveler, and **the level travels in the dispatch envelope** so agent-side node logs honour it too. Without that propagation, "debug this run" would go quiet the moment work crossed to an agent — which is precisely when people need it most.

The override applies in both development and production mode; being able to turn up detail on one misbehaving production run without touching global logging is the point.

---

## 16. Working directories

Workflows need scratch space — the temp file that a transcode writes before being renamed into place (§8), extracted artwork, probe output.

**A new configuration setting defines a `metarr` directory.** Within it, each run gets `metarr/workflows/{runID}`, where `runID` is the run document's ObjectID. The directory is created at run start and removed when the run reaches a terminal state, subject to a retention setting for post-mortem inspection of failures.

This gives the temp-file-plus-rename rule from §8 a defined home rather than leaving each handler to invent one beside the media file — which is the failure mode that leaves a truncated `.mp4` for the scanner to misclassify as a legitimate sidecar.

**The `metarr` directory is a server-only construct.** It is an `appconfig` setting, which per the project's rules obligates config API CRUD methods, configuration UI, and initialisation in `cmd/metarr-server/main.go`. It is deliberately **not** added to `agentregistry.BuildProjection`: agents do not have one, so publishing the server's disk layout to every agent host would be both useless and inappropriate.

**Agents use the operating system's temp directory instead.** An agent creates its scratch space with `os.MkdirTemp`, which resolves per-OS (`TMPDIR`, `TEMP`, `/tmp`) and therefore satisfies the Windows/Linux/macOS requirement without the agent needing a configured path at all. It is removed when the node execution finishes.

`NodeContext.WorkDir` is always the local scratch directory for whichever machine is executing the node — under the server's `metarr/workflows/{runID}` on the server, an OS temp directory on an agent. Handlers do not know or care which.

> **The temp directory is scratch space, not the staging area for the atomic publish.** `os.Rename` is only atomic *within a filesystem*, and on Linux a cross-device rename fails outright with `EXDEV`. So the temp-file-plus-rename rule from §8 requires the temp file to sit **in the destination's own directory**, not in `WorkDir` — otherwise the final publish either fails or silently degrades to a non-atomic copy, which is exactly the truncated-`.mp4` failure the rule exists to prevent. `internal/agent/nfo/write.go` already does this correctly and is the pattern to follow: `os.CreateTemp(filepath.Dir(destination), …)` then rename. `WorkDir` is for genuine intermediates — probe output, extracted artwork, multi-pass logs.

---

## 17. Consequences of these additions

Folding §14–§16 in changes earlier sections rather than merely appending to them:

- **`exec.effects` moves from deferred to mandatory** (§3.1, §14.1). Every catalog entry declares it from the first one written.
- **The handler contract changes shape.** Handlers receive a `NodeContext` and may not touch `os`/`os/exec` directly (§14.2). This must be settled before any handler is written, because retrofitting it means rewriting all of them.
- **The dispatch envelope grows** `dry_run`, `log_level`, `mode`, and `work_dir` (§8, §14.2, §15.3).
- **Run status gains `paused`** and the run document gains `mode`, `dry_run`, `log_level`, `breakpoints`, `work_dir` (§7).
- **Build order shifts.** `NodeContext`, the effects classification, and the run-scoped logger belong in step 1 (`internal/shared/workflow`) rather than being deferred — they are contract, not runtime, and everything downstream depends on their shape.




