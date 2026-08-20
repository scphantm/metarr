import type { Type } from '../pages/workflows/catalogTypes'

/*
 * Maps a workflow socket Type to one of the eight Solarized accents so data
 * handles and data edges are colored by type, per CLAUDE.md's "data = thin/
 * type-coloured" edge rule. Deliberately a small closed set of literal class
 * names (never a template-literal-constructed class) — Tailwind's scanner
 * only picks up class names that appear verbatim as source text, so every
 * possible result has to be written out once, here.
 */

type AccentToken = 'cyan' | 'violet' | 'yellow' | 'orange' | 'blue' | 'magenta' | 'green' | 'red' | 'base1'

// Ordered longest-prefix-first so "path.file.video" resolves before the bare
// "path" entry — mirrors the dotted-prefix hierarchy in
// internal/shared/workflow/types.go, but this list is deliberately not
// exhaustive: an unlisted subtype falls through to its nearest listed
// ancestor prefix, and anything with no listed ancestor gets the neutral
// fallback rather than a new entry per new leaf type.
const prefixAccents: [string, AccentToken][] = [
  ['path.file.video', 'violet'],
  ['path.file.image', 'magenta'],
  ['path.file.subtitle', 'magenta'],
  ['path.file.nfo', 'yellow'],
  ['path.file', 'cyan'],
  ['path.dir', 'cyan'],
  ['path', 'cyan'],
  ['media', 'green'],
  ['metadata', 'yellow'],
  ['agent', 'orange'],
  ['scanner', 'orange'],
  ['error', 'red'],
]

function bareType(type: Type): Type {
  return type.startsWith('list<') && type.endsWith('>') ? type.slice(5, -1) : type
}

function accentTokenForType(type: Type): AccentToken {
  const bare = bareType(type)
  for (const [prefix, token] of prefixAccents) {
    if (bare === prefix || bare.startsWith(`${prefix}.`)) return token
  }
  // bool, number, number.int, string, string.enum, duration, bytes,
  // timestamp, and any — the scalar/unclassified default.
  return 'blue'
}

const handleClasses: Record<AccentToken, string> = {
  cyan: '!bg-cyan',
  violet: '!bg-violet',
  yellow: '!bg-yellow',
  orange: '!bg-orange',
  blue: '!bg-blue',
  magenta: '!bg-magenta',
  green: '!bg-green',
  red: '!bg-red',
  base1: '!bg-base1',
}

export function dataHandleClass(type: Type): string {
  return handleClasses[accentTokenForType(type)]
}

// A real CSS value (not a Tailwind class) for use as an SVG stroke/fill —
// safe to build dynamically since it's an inline style, not something
// Tailwind needs to find as literal source text. --color-<token> is defined
// in index.css's @theme block for all nine tokens used here.
export function typeStrokeColor(type: Type): string {
  return `var(--color-${accentTokenForType(type)})`
}

const categoryAccentClasses: Record<string, string> = {
  input: 'border-l-cyan',
  output: 'border-l-violet',
  check: 'border-l-yellow',
  note: 'border-l-orange',
  control: 'border-l-blue',
  filesystem: 'border-l-green',
  media: 'border-l-magenta',
  metadata: 'border-l-yellow',
  string: 'border-l-cyan',
}

// Cosmetic only, per CLAUDE.md: "category is presentation-only." Unlisted or
// missing categories fall back to blue rather than failing to render.
export function accentClassForCategory(category: string | undefined): string {
  return categoryAccentClasses[category ?? ''] ?? 'border-l-blue'
}
