---
name: doc-theme
description:
  Rules for editing Metarr's Antora documentation theme under documentation-theme/. Use only when a task explicitly
  involves changing the doc site's theme; routine work ignores that directory entirely.
---

# Editing the documentation theme

- The theme bundles into a standard package consumed by Antora to theme the site. It must follow the specifications in
  `documentation-theme/docs`.
- Do not change anything under `documentation-theme/docs` — that is the spec, not the implementation.
- Outside a theme task, ignore `documentation-theme/` entirely (also stated in `AGENTS.md`).
