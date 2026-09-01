/*
 * Every catalog node's visual identity, one explicit entry per node type —
 * no derivation from `kind` or `category`, and no sharing across nodes that
 * happen to look alike. `category` still organizes the palette
 * (NodePalette.tsx groups by `entry.category` directly off the catalog),
 * but plays no part in a node's shape or color: this table supplies those
 * three required parts directly, per type — a background shape, a
 * shape-fill accent, and a border accent, independent of each other and of
 * every other node's entry. This is a requirement every node resolves
 * through, not a default most nodes fall through and a few override: every
 * one of the catalog's 34 entries has its own line below, verified against
 * the live catalog. Where several nodes end up with the same shape/color,
 * that's each entry saying so on its own, not inheritance.
 *
 * Color legend this table follows:
 *   Input / output / control flow / notes -> cyan
 *   Errors -> red
 *   Decisions -> yellow
 *   Processes: neutral (e.g. list) -> blue; creation (e.g. copy) -> green;
 *              destructive (move, delete, update) -> magenta;
 *              create + destroy / complex -> violet
 *
 * Every entry now uses a generated icon (iconVisual(), shapeIsIcon: true) —
 * a CSS-class mask, same mechanism as the data-type icons in
 * lib/typeIcons.ts (never a rendered icon-library component, see
 * eslint.config.js's no-restricted-imports rule); the icon and its class
 * name are visible directly on the relevant line below, not repeated here.
 * The standalone shapes.css clip-path/border-radius shape system (decision
 * diamonds, terminators, the document wave, etc.) was retired once its last
 * consumer moved to an icon and deleted outright — what's still load-bearing
 * (.box, .shape, .shape-icon, the .color-<accent> classes, and the
 * --accent/--accent-dim/--page-bg vars they resolve) now lives in
 * index.css. `process` never had a real CSS rule (shapes.css used to note
 * "plain rectangle, no extra rule needed" — just the bare .shape box);
 * DEFAULT below still names it as the fallback for a catalog type with no
 * entry yet, resolving to an unstyled .shape rectangle via index.css.
 */

export type Accent =
  | "red"
  | "orange"
  | "yellow"
  | "green"
  | "cyan"
  | "blue"
  | "violet"
  | "magenta";

export type NodeVisual = {
  shapeClassName: string;
  // A CSS class appended alongside the shape class — e.g. a rotation
  // utility for a node that reuses another's shape flipped.
  shapeExtraClassName?: string;
  shapeAccent: Accent;
  // Independent of shapeAccent, even though every entry below currently
  // sets it to the same value — the hover-reveal border (NodeShell.tsx)
  // needs both resolvable on their own.
  borderAccent: Accent;
  // True when shapeClassName names a generated icon-mask class
  // (lib/icons/typeIcons.css) rather than a plain index.css .shape div —
  // NodeShell.tsx renders the two differently (a masked icon vs. a filled
  // shape rectangle). Explicit, never inferred from the class name,
  // matching this table's "nothing here is derived" rule.
  shapeIsIcon?: boolean;
};

// color-<token>, hover-border-<token>, and accent-<token>-<opacity> are all
// hand-written classes in index.css, so — unlike when these were Tailwind
// utilities — building the name from a template is fine here.
export function shapeColorClassForAccent(accent: Accent): string {
  return `color-${accent}`;
}

export function hoverBorderColorClassForAccent(accent: Accent): string {
  return `hover-border-${accent}`;
}

// The 32 accent-<accent>-<opacity> tints defined in index.css — for a
// layer that needs to sit visibly *under* other content (the quadrant
// notification layer) rather than read as a solid fill.
export function accentTintClassForAccent(
  accent: Accent,
  opacity: 20 | 40 | 60 | 80,
): string {
  return `accent-${accent}-${opacity}`;
}

// `border` defaults to the shape's own accent when omitted, so the common
// case (shape and border matching, true for every node below today) stays
// a one-line entry — but shape and border are resolved from independent
// accents even then, not one value duplicated, so any entry can diverge by
// just passing `{ border: 'X' }` without touching how the others resolve.
function visual(
  shapeClassName: string,
  shapeAccent: Accent,
  options?: { border?: Accent; extraClass?: string },
): NodeVisual {
  return {
    shapeClassName,
    shapeExtraClassName: options?.extraClass,
    shapeAccent,
    borderAccent: options?.border ?? shapeAccent,
  };
}

// Same contract as visual(), for the icon-backed shapes (shapeIsIcon: true)
// — see the file header and index.css's .shape-icon.
function iconVisual(
  shapeClassName: string,
  shapeAccent: Accent,
  options?: { border?: Accent },
): NodeVisual {
  return {
    shapeClassName,
    shapeAccent,
    borderAccent: options?.border ?? shapeAccent,
    shapeIsIcon: true,
  };
}

const definitions: Record<string, NodeVisual> = {
  "core/start": iconVisual("shape-start", "cyan"),
  // The graph's one committing action — stands out from the plain exits
  // around it rather than reading as an ordinary terminator.
  "core/writeChanges": iconVisual("shape-write-changes", "magenta"),
  // The graph's one error terminal.
  "core/errorOutput": iconVisual("shape-error-output", "red"),
  "core/note": iconVisual("shape-note", "cyan"),
  "core/checkFlowVariable": iconVisual("shape-decision", "yellow"),
  "core/trickplay": iconVisual("shape-trickplay", "violet"),
  "core/forEach": iconVisual("shape-for-each", "cyan"),
  "core/collect": iconVisual("shape-collect", "cyan"),
  "core/parallel": iconVisual("shape-parallel", "cyan"),
  "core/join": iconVisual("shape-join", "cyan"),
  "core/break": iconVisual("shape-break", "cyan"),
  "core/end": iconVisual("shape-end", "cyan"),
  "core/fail": iconVisual("shape-fail", "red"),

  "fs/listDirectory": iconVisual("shape-list-directory", "blue"),
  "fs/moveFile": iconVisual("shape-move-file", "magenta"),
  "fs/copyFile": iconVisual("shape-copy-file", "green"),
  "fs/deleteFile": iconVisual("shape-delete-file", "magenta"),
  "fs/exists": iconVisual("shape-decision", "yellow"),
  "fs/isdir": iconVisual("shape-decision", "yellow"),
  "fs/isfile": iconVisual("shape-decision", "yellow"),
  "fs/makeDirectory": iconVisual("shape-make-directory", "green"),
  "fs/fileSize": iconVisual("shape-file-size", "blue"),
  "fs/readTextFile": iconVisual("shape-read-text-file", "blue"),
  "fs/writeTextFile": iconVisual("shape-write-text-file", "magenta"),

  "media/probe": iconVisual("shape-probe", "blue"),
  "media/transcode": iconVisual("shape-transcode", "green"),
  "media/extractStream": iconVisual("shape-extract-stream", "green"),
  "media/generateThumbnail": iconVisual("shape-generate-thumbnail", "green"),

  "nfo/read": iconVisual("shape-nfo-read", "blue"),
  "nfo/write": iconVisual("shape-nfo-write", "magenta"),

  "string/format": iconVisual("shape-format", "blue"),
  "string/regexMatch": iconVisual("shape-regex-match", "blue"),
  "string/concat": iconVisual("shape-concat", "blue"),
  "string/parseNumber": iconVisual("shape-parse-number", "blue"),
  "string/trim": iconVisual("shape-trim", "blue"),
};

// Never a derivation from kind/category — just the same "don't render
// totally unstyled" guarantee the table already gave every declared node,
// applied to a catalog type that has no entry yet (e.g. one just added to
// the catalog before this table is updated for it).
const DEFAULT: NodeVisual = visual("process", "blue");

export function nodeVisual(type: string): NodeVisual {
  return definitions[type] ?? DEFAULT;
}
