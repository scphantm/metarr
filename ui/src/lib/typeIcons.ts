import { elementType, isListType, type Type } from '../pages/workflows/connectionRules'

/*
 * Maps a workflow socket Type to a CSS class from lib/icons/typeIcons.css
 * (mask-image + currentColor — see that file and scripts/generate-icons.mjs
 * for how the classes are produced). react-icons is a devDependency used
 * only by that generation script; nothing under src/ imports it directly —
 * enforced by eslint.config.js's no-restricted-imports rule. The icon
 * itself is never embedded in markup, only a semantic class name.
 *
 * A small ordered literal list, longest/most-specific prefix first,
 * deliberately not exhaustive — a type with no listed icon renders no icon
 * at all (falls through to undefined), which must not regress that type's
 * existing (non-icon) appearance. Extend this list as new type families get
 * icons; nothing else needs to change.
 */

// Longest/most-specific prefix first — 'path' is the generic fallback for
// any path.* subtype not already claimed by a more specific entry above it
// (e.g. path.dir, path.file), so it must stay last.
const prefixIcons: [string, string][] = [
  ['path.dir', 'icon-directory'],
  ['path.file', 'icon-file'],
  ['path', 'icon-tree'],
  // A Mongo-backed record (scanmodel.MediaFile in the local_directory
  // collection), not a filesystem location — see design.md §4.1.
  ['media', 'icon-media'],
]

export function iconClassForType(type: Type): string | undefined {
  if (isListType(type)) {
    const inner = elementType(type) ?? ''
    if (inner === 'path.dir' || inner.startsWith('path.dir.')) return 'icon-list-directory'
    if (inner === 'path.file' || inner.startsWith('path.file.')) return 'icon-list-file'
    if (inner === 'path' || inner.startsWith('path.')) return 'icon-list'
    return undefined
  }
  for (const [prefix, className] of prefixIcons) {
    if (type === prefix || type.startsWith(`${prefix}.`)) return className
  }
  return undefined
}

// Control-flow port icons, keyed by exact port name rather than by prefix —
// control ports aren't a dotted type hierarchy the way data sockets are,
// just a small closed set of conventional names. Not exhaustive: a control
// port whose name isn't listed here (e.g. a Parallel branch's "branch1",
// ForEach's "body"/"done") renders no icon, same fallback behavior as
// iconClassForType above.
const controlPortIcons: Record<string, string> = {
  in: 'icon-control-in',
  error: 'icon-control-error',
  next: 'icon-control-next',
  yes: 'icon-control-yes',
  no: 'icon-control-no',
}

export function iconClassForControlPort(port: string): string | undefined {
  return controlPortIcons[port]
}

// The badge shown next to a data edge's source-end icon when its active
// transform implies an iterator (Transform.implies_iteration) — see
// edges/DataEdge.tsx.
export const ITERATE_ICON_CLASS = 'icon-iterate'

// The badge shown next to a data edge's target-end icon when the connection
// is a type-unsafe narrowing (Connection.typeUnsafe) — see
// connectionRules.ts and edges/DataEdge.tsx. Generic: fires for any
// supertype -> subtype pair, not scoped to the path family.
export const TYPE_UNSAFE_ICON_CLASS = 'icon-type-unsafe'

// The badge shown next to a path edge's target-end icon when that edge's
// settings.recursive is set (edges/EdgeSettingsEditor.tsx, opened by
// double-clicking the edge) — see edges/DataEdge.tsx.
export const RECURSIVE_ICON_CLASS = 'icon-recursive'
