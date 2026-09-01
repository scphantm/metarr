import { create } from '@bufbuild/protobuf'
import type { Connection as RFConnection, Edge as RFEdge } from '@xyflow/react'

import {
  type WorkflowNodeType as NodeType,
  type WorkflowTransform as Transform,
  WorkflowTransformSchema,
} from '../../gen/metarr/v1/workflow_catalog_pb'

// A workflow value type is a dotted-prefix hierarchy ("path.dir" is a subtype
// of "path", "agent.slug" of "agent") plus the generic `list<T>` constructor.
// It is a free string on the wire — the catalog owns the vocabulary — so this
// is the one place the alias is declared; the subtyping/coercion logic below
// is the behavioural port of types.go and is deliberately not generated.
export type Type = string

/*
 * TS port of internal/shared/workflow/types.go's subtyping/coercion logic,
 * plus the connect-time arity/kind checks design.md §6.6 assigns to the
 * client: "port kind, arity, type compatibility, transform availability —
 * that is all isValidConnection needs." The whole-graph analyses (is this
 * value guaranteed to exist here, do these branches converge — MustHaveRun,
 * loop scope) stay server-only, in POST /api/workflows/validate; nothing in
 * this file may grow into a second copy of those.
 *
 * Every rule here is driven by the transforms array the catalog endpoint
 * serves, never a hand-duplicated registry — see useWorkflowCatalog.
 */

const LIST_PREFIX = 'list<'
const LIST_SUFFIX = '>'

export function isListType(type: Type): boolean {
  return type.startsWith(LIST_PREFIX) && type.endsWith(LIST_SUFFIX)
}

export function elementType(type: Type): Type | null {
  if (!isListType(type)) return null
  return type.slice(LIST_PREFIX.length, type.length - LIST_SUFFIX.length)
}

// Mirrors types.go's IsSubtypeOf exactly, including the dot-guard on the
// prefix test: "path.file" must not count as a subtype of "path.f".
export function isSubtypeOf(sub: Type, superType: Type): boolean {
  if (superType === 'any' || sub === superType) return true

  const subElement = elementType(sub)
  const superElement = elementType(superType)
  if (subElement != null || superElement != null) {
    if (subElement == null || superElement == null) return false
    return isSubtypeOf(subElement, superElement)
  }

  return sub.startsWith(`${superType}.`)
}

export type TypeConnection = {
  direct: boolean
  candidates: Transform[]
  // See types.go's Connection.TypeUnsafe: true when this Direct connection
  // is a narrowing (supertype -> subtype) rather than the safe covariant
  // direction — structural, not scoped to any one type family.
  typeUnsafe?: boolean
}

// Mirrors types.go's representationOf: only where a representation is
// shared by more than one type — an unshared one has no behavioral effect,
// nothing else could ever match it.
const representationOf: Partial<Record<Type, string>> = {
  string: 'primitive.string',
  'agent.slug': 'primitive.string',
  'scanner.slug': 'primitive.string',

  'media.file': 'io/fs.File',
  'path.file': 'io/fs.File',
}

// Mirrors types.go's SameRepresentation: two types sharing a real-world
// representation (design.md §4.1's table) need no transform in either
// direction — there's nothing to convert, only one shape of value. Distinct
// from isSubtypeOf: media.file and path.file share no dotted prefix and
// neither is a subtype of the other, but both are io/fs.File underneath.
function sameRepresentation(a: Type, b: Type): boolean {
  const aElement = elementType(a)
  const bElement = elementType(b)
  if (aElement != null || bElement != null) {
    if (aElement == null || bElement == null) return false
    return sameRepresentation(aElement, bElement)
  }

  const aRep = representationOf[a]
  const bRep = representationOf[b]
  return aRep != null && aRep === bRep
}

// wrapTransform mirrors types.go's wrapTransform: the synthetic "wrap" (T ->
// list<T>, design.md §4.3) can't be one entry in the transforms array from
// the catalog endpoint — it must match list<T> for every T, not one
// hardcoded element type — so it's computed per-connection instead.
function wrapTransform(from: Type, to: Type): Transform | null {
  if (isListType(from)) return null
  const element = elementType(to)
  if (element == null || !isSubtypeOf(from, element)) return null
  return create(WorkflowTransformSchema, {
    name: 'wrap',
    from,
    to,
    summary: 'Wraps the single value into a one-element list',
  })
}

export function canConnect(
  from: Type,
  to: Type,
  transforms: Transform[],
): TypeConnection {
  if (isSubtypeOf(from, to)) {
    return { direct: true, candidates: [] }
  }
  if (isSubtypeOf(to, from)) {
    return { direct: true, candidates: [], typeUnsafe: true }
  }
  if (sameRepresentation(from, to)) {
    // Not a narrowing — neither is a subtype of the other — so no
    // typeUnsafe: this is asserted equivalence, not an unverifiable guess.
    return { direct: true, candidates: [] }
  }
  const candidates = transforms.filter(
    (transform) =>
      isSubtypeOf(from, transform.from) && isSubtypeOf(transform.to, to),
  )
  const wrap = wrapTransform(from, to)
  if (wrap) candidates.push(wrap)
  return { direct: false, candidates }
}

export function connectionAllowed(connection: TypeConnection): boolean {
  return connection.direct || connection.candidates.length > 0
}

// The single transform the editor may attach without asking — exactly one
// unambiguous candidate. Anything else (zero, several, or the one candidate
// marked ambiguous) means the picker has to prompt.
export function autoApplyTransform(
  connection: TypeConnection,
): Transform | null {
  if (connection.direct || connection.candidates.length !== 1) return null
  const [only] = connection.candidates
  return only.ambiguous ? null : only
}

export function explainIncompatible(from: Type, to: Type): string {
  const fromIsList = isListType(from)
  const toIsList = isListType(to)
  if (fromIsList && !toIsList) {
    return 'A collection cannot feed a single value. Use a For Each node to work through it one item at a time.'
  }
  if (toIsList && !fromIsList) {
    return 'A single value cannot feed a collection. Collect values inside a loop instead.'
  }
  return `${from} cannot connect to ${to}.`
}

// ---- handle id encoding -----------------------------------------------------

// Handle ids encode which namespace a port lives in, per CLAUDE.md: "handle
// ids encode kind (c:in, c:next, c:error, d:source)". A control port and a
// data socket on the same node may share a bare name without colliding.
export const controlHandleId = (port: string) => `c:${port}`
export const dataHandleId = (name: string) => `d:${name}`

export type ParsedHandle = { kind: 'control' | 'data'; name: string }

export function parseHandleId(
  handleId: string | null | undefined,
): ParsedHandle | null {
  if (!handleId) return null
  if (handleId.startsWith('c:'))
    return { kind: 'control', name: handleId.slice(2) }
  if (handleId.startsWith('d:'))
    return { kind: 'data', name: handleId.slice(2) }
  return null
}

// ---- connect-time validation ------------------------------------------------

export type ConnectionVerdict =
  | { allowed: true; kind: 'control' }
  | {
      allowed: true
      kind: 'data'
      connection: TypeConnection
      fromType: Type
      toType: Type
    }
  | { allowed: false; reason: string }

// Everything isValidConnection and onConnect need to resolve a candidate
// connection: the two endpoints' NodeType (undefined for an unresolved/
// unknown type — always refused), the edges already on the canvas (for
// arity), and the live transform registry.
export function evaluateConnection(
  candidate: RFConnection,
  sourceType: NodeType | undefined,
  targetType: NodeType | undefined,
  existingEdges: RFEdge[],
  transforms: Transform[],
): ConnectionVerdict {
  if (!candidate.source || !candidate.target) {
    return { allowed: false, reason: 'Missing endpoint.' }
  }
  if (candidate.source === candidate.target) {
    return { allowed: false, reason: 'A node cannot connect to itself.' }
  }
  if (!sourceType || !targetType) {
    return { allowed: false, reason: 'Unknown node type.' }
  }

  const sourceHandle = parseHandleId(candidate.sourceHandle)
  const targetHandle = parseHandleId(candidate.targetHandle)
  if (!sourceHandle || !targetHandle) {
    return { allowed: false, reason: 'Malformed handle.' }
  }
  if (sourceHandle.kind !== targetHandle.kind) {
    return {
      allowed: false,
      reason:
        'Control ports connect only to control ports, data sockets only to data sockets.',
    }
  }

  if (sourceHandle.kind === 'control') {
    const sourceIsOut =
      sourceHandle.name === 'error'
        ? Boolean(sourceType.control?.error)
        : Boolean(sourceType.control?.out.includes(sourceHandle.name))
    const targetIsIn = Boolean(
      targetType.control?.in.includes(targetHandle.name),
    )
    if (!sourceIsOut || !targetIsIn) {
      return { allowed: false, reason: 'Not a valid control connection.' }
    }
    // A control out-port takes exactly one outgoing edge.
    const alreadyWired = existingEdges.some(
      (edge) =>
        edge.source === candidate.source &&
        edge.sourceHandle === candidate.sourceHandle,
    )
    if (alreadyWired) {
      return {
        allowed: false,
        reason: 'This control output already has a connection.',
      }
    }
    return { allowed: true, kind: 'control' }
  }

  const sourceSocket = sourceType.dataOut?.find(
    (socket) => socket.name === sourceHandle.name,
  )
  const targetSocket = targetType.dataIn?.find(
    (socket) => socket.name === targetHandle.name,
  )
  if (!sourceSocket || !targetSocket) {
    return { allowed: false, reason: 'Not a valid data connection.' }
  }
  // A data in-socket takes exactly one incoming edge.
  const alreadyWired = existingEdges.some(
    (edge) =>
      edge.target === candidate.target &&
      edge.targetHandle === candidate.targetHandle,
  )
  if (alreadyWired) {
    return { allowed: false, reason: 'This input already has a connection.' }
  }

  const connection = canConnect(
    sourceSocket.type,
    targetSocket.type,
    transforms,
  )
  if (!connectionAllowed(connection)) {
    return {
      allowed: false,
      reason: explainIncompatible(sourceSocket.type, targetSocket.type),
    }
  }
  return {
    allowed: true,
    kind: 'data',
    connection,
    fromType: sourceSocket.type,
    toType: targetSocket.type,
  }
}
