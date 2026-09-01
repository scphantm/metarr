---
name: workflow-nodes
description: Metarr's workflow UI node anatomy and workflow-engine execution invariants — edge kinds, port arity, handle ids, the dry-run capability model, and exec.effects. Use when adding or editing a workflow node, a catalog entry, or a node handler. The full schema/semantics/validation spec lives elsewhere; read it first for anything past node anatomy.
---

# Workflow nodes & engine

Full spec — schema, execution semantics, validation: `documentation/modules/design/pages/workflow_engine.adoc`. Per `AGENTS.md`, match that doc and never edit it without the user's explicit go-ahead. What follows is node anatomy and engine invariants only.

## UI node anatomy

* Top edge: control-in (leftmost) then data-ins. Bottom edge: control-outs (leftmost) then data-outs. Right edge: `error`.
* Control edges (thick, solid, neutral, animated) show what runs next; data edges (thin, coloured by type) wire a value. Never style them the same.
* Input nodes have no control-in (starting points); output nodes have no control-out (ending points) — driven by the catalog's `control` block, never a hardcoded category check.
* `category` is presentation-only — never drives behavior. Dispatch on `type`.
* Arity: control out-port exactly one edge, data-in exactly one, data-out many, control-in many. Parallelism uses an explicit `core/parallel` node, never a second wire off one output.
* Handle ids encode port kind: `c:in`, `c:next`, `c:error`, `d:source`.
* Port `name` is a permanent id stored edges reference — renaming breaks saved workflows. Display text is `label`, free to change.

## Engine execution

* Defaults to dry-run. Files can only be touched when dry-run is explicitly disabled (manual run, or automation in production mode).
* Dry-run is enforced by capability, not convention: a node handler never imports `os`/`os/exec` — it gets filesystem/process capability from the executor harness (`workflow.NodeContext`), and under dry-run those ops log-and-no-op rather than touch disk. A handler that forgets a flag check still can't write — it has no path to the filesystem. Never give a handler direct filesystem access "just this once."
* Every catalog entry must declare `exec.effects` (`read` | `write` | `destructive`) — missing it is a load error. Dry-run keys off this field; it can't be retrofitted without re-auditing every handler.
* The agent enforces dry-run itself too (the handler runs there) rather than trusting the server's decision.
