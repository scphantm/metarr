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
 * Shapes deliberately not used: .display, .subroutine, .internal-storage,
 * .gate — all four fake a concave cutout by painting a page-background-
 * colored pseudo-element over the shape, which only works against a solid
 * background. Node cards are transparent, so every shape below is either a
 * pure clip-path/border-radius silhouette or an additive (same-color, not
 * background-matching) pseudo-element.
 */

export type Accent = 'red' | 'orange' | 'yellow' | 'green' | 'cyan' | 'blue' | 'violet' | 'magenta'

export type NodeVisual = {
  shapeClassName: string
  // Tailwind utility appended alongside the shape class — e.g. rotate-180
  // for Parallel, which reuses Join's .merge shape flipped.
  shapeExtraClassName?: string
  shapeAccent: Accent
  // Independent of shapeAccent, even though every entry below currently
  // sets it to the same value — the hover-reveal border (NodeShell.tsx)
  // needs both resolvable on their own.
  borderAccent: Accent
}

// Literal so Tailwind's scanner can find every possible result written out
// as source text — see lib/typeColors.ts's handleClasses for the same
// constraint on the same kind of lookup. Three separate maps because each
// is a different Tailwind/CSS utility family, not three copies of one idea.
const BORDER_CLASS: Record<Accent, string> = {
  red: 'border-red',
  orange: 'border-orange',
  yellow: 'border-yellow',
  green: 'border-green',
  cyan: 'border-cyan',
  blue: 'border-blue',
  violet: 'border-violet',
  magenta: 'border-magenta',
}

// The /40 is Tailwind's own opacity modifier, applied to border-color —
// no new CSS needed here the way the background tints below did, since
// Tailwind already generates an opacity-mixed variant for any color
// utility, border-color included.
const HOVER_BORDER_CLASS: Record<Accent, string> = {
  red: 'hover:border-red/40',
  orange: 'hover:border-orange/40',
  yellow: 'hover:border-yellow/40',
  green: 'hover:border-green/40',
  cyan: 'hover:border-cyan/40',
  blue: 'hover:border-blue/40',
  violet: 'hover:border-violet/40',
  magenta: 'hover:border-magenta/40',
}

// Unlike the two maps above, these are safe to build from a template —
// color-<token> and accent-<token>-<opacity> are hand-written classes
// (shapes.css and index.css respectively), not Tailwind utilities, so they
// were never subject to the "must appear verbatim in source" scanning rule.
export function shapeColorClassForAccent(accent: Accent): string {
  return `color-${accent}`
}

export function borderColorClassForAccent(accent: Accent): string {
  return BORDER_CLASS[accent]
}

export function hoverBorderColorClassForAccent(accent: Accent): string {
  return HOVER_BORDER_CLASS[accent]
}

// The 32 accent-<accent>-<opacity> tints defined in index.css — for a
// layer that needs to sit visibly *under* other content (the quadrant
// notification layer) rather than read as a solid fill.
export function accentTintClassForAccent(accent: Accent, opacity: 20 | 40 | 60 | 80): string {
  return `accent-${accent}-${opacity}`
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
  }
}

const definitions: Record<string, NodeVisual> = {
  'core/start': visual('terminator', 'cyan'),
  'core/inputPath': visual('data-shape', 'cyan'),
  // The graph's one committing action — stands out from the plain exits
  // around it rather than reading as an ordinary terminator.
  'core/writeChanges': visual('terminator', 'magenta'),
  // The graph's one error terminal.
  'core/errorOutput': visual('terminator', 'red'),
  'core/note': visual('document', 'cyan'),
  'core/checkFlowVariable': visual('decision', 'yellow'),
  'core/trickplay': visual('process', 'violet'),
  'core/forEach': visual('loop-limit', 'cyan'),
  'core/collect': visual('stored-data', 'cyan'),
  // Same .merge triangle as join, just rotated — point-top/wide-bottom reads
  // as "one becomes many," matching parallel's single top-in/multi bottom-out.
  'core/parallel': visual('merge', 'cyan', { extraClass: 'rotate-180' }),
  // Wide-top/point-bottom reads as "many become one," matching join's multi
  // top-in/single bottom-out.
  'core/join': visual('merge', 'cyan'),
  'core/break': visual('terminator', 'cyan'),
  'core/end': visual('terminator', 'cyan'),
  'core/fail': visual('triangle', 'red'),

  'fs/listDirectory': visual('process', 'blue'),
  'fs/moveFile': visual('process', 'magenta'),
  'fs/copyFile': visual('process', 'green'),
  'fs/deleteFile': visual('process', 'magenta'),
  'fs/exists': visual('decision', 'yellow'),
  'fs/makeDirectory': visual('process', 'green'),
  'fs/fileSize': visual('process', 'blue'),
  'fs/readTextFile': visual('process', 'blue'),
  'fs/writeTextFile': visual('process', 'magenta'),

  'media/probe': visual('process', 'blue'),
  'media/transcode': visual('process', 'green'),
  'media/extractStream': visual('process', 'green'),
  'media/generateThumbnail': visual('process', 'green'),

  'nfo/read': visual('process', 'blue'),
  'nfo/write': visual('process', 'magenta'),

  'string/format': visual('process', 'blue'),
  'string/regexMatch': visual('decision', 'blue'),
  'string/concat': visual('process', 'blue'),
  'string/parseNumber': visual('decision', 'blue'),
  'string/trim': visual('process', 'blue'),
}

// Never a derivation from kind/category — just the same "don't render
// totally unstyled" guarantee the table already gave every declared node,
// applied to a catalog type that has no entry yet (e.g. one just added to
// the catalog before this table is updated for it).
const DEFAULT: NodeVisual = visual('process', 'blue')

export function nodeVisual(type: string): NodeVisual {
  return definitions[type] ?? DEFAULT
}
