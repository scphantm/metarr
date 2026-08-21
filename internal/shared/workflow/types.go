// Package workflow defines the contract shared by the workflow authoring
// tools, the server-side engine, and agent-side node handlers.
//
// It is deliberately dependency-free and lives under internal/shared so both
// binaries can import it: nothing under internal/agent may import
// internal/server, so the handler contract has to live somewhere neither
// owns. See design.md for the full specification.
package workflow

import "strings"

// Type is a value type in the workflow type system.
//
// Type names form a dotted-prefix hierarchy: "path.dir" is a subtype of
// "path", "agent.slug" is a subtype of "agent". That is the whole mechanism —
// there is no lattice structure to maintain, and a new type needs no code
// change beyond naming it. The one exception is list, which is a generic
// constructor written list<T> and is covariant in T. See design.md §4.1.
type Type string

const (
	// TypeAny is the top of the hierarchy: everything is assignable to it.
	TypeAny Type = "any"

	TypeBool   Type = "bool"
	TypeNumber Type = "number"
	TypeString Type = "string"
	// TypeDatetime is a point in time.
	TypeDatetime Type = "datetime"

	// Path types. Every path value inside a workflow is in server-canonical
	// space; translation to an agent's own space happens once, in the
	// dispatch layer. See design.md §4.2.
	TypePath     Type = "path"
	TypePathDir  Type = "path.dir"
	TypePathFile Type = "path.file"

	// Media types are database records, not paths. media.file is a scanned
	// MediaFile; path.file is a string naming a location. Conflating the two
	// is how every node ends up accepting "any". TVSeries and Movie are
	// deliberately not media.* subtypes — separate root families, so they
	// stay non-interchangeable rather than folding into one loose catch-all.
	TypeMedia     Type = "media"
	TypeMediaFile Type = "media.file"
	TypeTVSeries  Type = "tvseries"
	// TypeMovie has no backing scanmodel struct yet — reserved.
	TypeMovie Type = "movie"

	TypeAgent       Type = "agent"
	TypeAgentSlug   Type = "agent.slug"
	TypeScanner     Type = "scanner"
	TypeScannerSlug Type = "scanner.slug"

	// TypeError is what an error control-out's companion data-out carries.
	TypeError Type = "error"
)

// Names for the generic list<T> instantiations used by the path family — not
// new leaf types, just symbolic aliases for list<path>/list<path.dir>/
// list<path.file> so Go code and tests can write TypePathListFile instead of
// the string literal. var, not const: ListOf is a function call. catalog.json
// may still write "list<path.file>" etc. literally; nothing requires a node
// to use these.
var (
	TypePathList     = ListOf(TypePath)
	TypePathListDir  = ListOf(TypePathDir)
	TypePathListFile = ListOf(TypePathFile)
)

const (
	listPrefix = "list<"
	listSuffix = ">"
)

// ListOf builds the list type for element.
func ListOf(element Type) Type {
	return Type(listPrefix + string(element) + listSuffix)
}

// IsList reports whether t is a list type.
func (t Type) IsList() bool {
	return strings.HasPrefix(string(t), listPrefix) && strings.HasSuffix(string(t), listSuffix)
}

// ElementType returns the element type of a list, and whether t was a list at
// all.
func (t Type) ElementType() (Type, bool) {
	if !t.IsList() {
		return "", false
	}
	inner := string(t)[len(listPrefix) : len(t)-len(listSuffix)]
	return Type(inner), true
}

// IsSubtypeOf reports whether a value of sub may be used where super is
// expected, with no transform.
//
// The dot guard on the prefix test matters: without it "path.file" would
// wrongly count as a subtype of "path.f".
func IsSubtypeOf(sub, super Type) bool {
	if super == TypeAny || sub == super {
		return true
	}

	subElement, subIsList := sub.ElementType()
	superElement, superIsList := super.ElementType()
	if subIsList || superIsList {
		// list<S> is a subtype of list<T> exactly when S is a subtype of T.
		// A list is never a subtype of a scalar, or the reverse — that would
		// silently smuggle a collection into a single-value socket.
		if !subIsList || !superIsList {
			return false
		}
		return IsSubtypeOf(subElement, superElement)
	}

	return strings.HasPrefix(string(sub), string(super)+".")
}

// representationOf names, for a type, the real-world value it's actually
// represented as at runtime (design.md §4.1's "Represented as" column) —
// only where that representation is shared by more than one type. An
// unshared representation has no behavioral effect, so it isn't worth
// encoding: nothing else could ever match it.
var representationOf = map[Type]string{
	TypeString:      "primitive.string",
	TypeAgentSlug:   "primitive.string",
	TypeScannerSlug: "primitive.string",

	TypeMediaFile: "io/fs.File",
	TypePathFile:  "io/fs.File",
}

// SameRepresentation reports whether a and b are, underneath, the same real
// value — design.md §4.1's table, not §4.3's dotted-prefix hierarchy. Two
// types sharing a representation need no transform in either direction:
// there's nothing to convert, because there's only one shape of value. This
// is deliberately distinct from IsSubtypeOf — media.file and path.file
// share no dotted prefix and neither is a subtype of the other, but both are
// io/fs.File underneath.
func SameRepresentation(a, b Type) bool {
	aElement, aIsList := a.ElementType()
	bElement, bIsList := b.ElementType()
	if aIsList || bIsList {
		if !aIsList || !bIsList {
			return false
		}
		return SameRepresentation(aElement, bElement)
	}

	aRep, aFound := representationOf[a]
	bRep, bFound := representationOf[b]
	return aFound && bFound && aRep == bRep
}

// Transform is a named, explicit conversion between two types, recorded on
// the data edge that uses it.
//
// Transforms are deliberately single names rather than chains: a chain is
// unreadable on an edge and untestable. Useful compositions are registered
// here as their own transform instead — media.file.parentDir is exactly that.
type Transform struct {
	Name string `json:"name"`
	From Type   `json:"from"`
	To   Type   `json:"to"`
	// Ambiguous marks a transform that must never be applied automatically
	// even when it is the only candidate, because more than one answer is
	// defensible and guessing wrong is silent. The UI always prompts.
	Ambiguous bool   `json:"ambiguous,omitempty"`
	Summary   string `json:"summary,omitempty"`
	// ImpliesIteration marks a transform whose value production is
	// one-to-many at the data level — eachFile fans a directory out to one
	// run per file, the way a SQL UNNEST fans a column out to one row per
	// element, without the graph author drawing an explicit loop. This is
	// deliberately NOT a control-flow construct: neither side is a list<T>,
	// no new frame or token is created, and no whole-graph analysis
	// (parallel/join arity, MustHaveRun) needs to reason about it — it's
	// local detail on one edge, the same category as a future string->date
	// transform needing a format parameter. The engine does not yet execute
	// the implied fan-out (design.md §13); today this is UI/documentation
	// metadata that drives the editor's iteration badge.
	ImpliesIteration bool `json:"implies_iteration,omitempty"`
}

// transformRegistry is the complete set of explicit conversions. Anything not
// listed here and not covered by subtyping is a connection error.
var transformRegistry = []Transform{
	{
		Name: "parentDir", From: TypePathFile, To: TypePathDir,
		Summary: "The directory containing the file",
	},
	{
		Name: "eachFile", From: TypePathDir, To: TypePathFile, ImpliesIteration: true,
		Summary: "Every file in the directory, one per downstream run",
	},
	{
		// Also covers path.dir/path.file -> string implicitly, via ordinary
		// subtyping (IsSubtypeOf(path.file, path) is already true) — no
		// separate registry entry needed per subtype. See design.md §4.3.
		Name: "toString", From: TypePath, To: TypeString,
		Summary: "The path as text",
	},
	{
		Name: "media.file.parentDir", From: TypeMediaFile, To: TypePathDir,
		Summary: "The directory containing the record's file",
	},
	{
		Name: "parseNumber", From: TypeString, To: TypeNumber,
		Summary: "Parse text as a number (may fail at run time)",
	},
	{
		Name: "format", From: TypeNumber, To: TypeString,
		Summary: "Format the number as text",
	},
}

// FindTransform returns the registered transform with the given name. It
// does not know about "wrap" (see ResolveTransform) — wrap is synthesized
// per-connection, not a static registry entry, since it must match list<T>
// for every T rather than one hardcoded element type.
func FindTransform(name string) (Transform, bool) {
	for _, transform := range transformRegistry {
		if transform.Name == name {
			return transform, true
		}
	}
	return Transform{}, false
}

// ResolveTransform is FindTransform plus the one case FindTransform can't
// handle alone: a persisted edge naming "wrap" resolves against the edge's
// actual from/to types, the same way CanConnect synthesizes it live. Callers
// re-validating a saved edge's named transform (as opposed to offering fresh
// candidates) should use this instead of FindTransform.
func ResolveTransform(name string, from, to Type) (Transform, bool) {
	if transform, found := FindTransform(name); found {
		return transform, true
	}
	if name == "wrap" {
		return wrapTransform(from, to)
	}
	return Transform{}, false
}

// Transforms returns the full registry, for serving to the editor alongside
// the catalog.
func Transforms() []Transform {
	registryCopy := make([]Transform, len(transformRegistry))
	copy(registryCopy, transformRegistry)
	return registryCopy
}

// Connection describes whether and how a data edge may be made between two
// sockets.
type Connection struct {
	// Direct is true when the source type is already assignable to the
	// target with no conversion, so no transform is recorded on the edge.
	Direct bool
	// Candidates are the explicit transforms that would bridge the gap, in
	// registry order. Empty when Direct is true.
	Candidates []Transform
	// TypeUnsafe marks a Direct connection made by narrowing — the source
	// declares a supertype and the target declares one of its subtypes
	// (e.g. path -> path.dir). Structural, not scoped to any one type
	// family: it fires whenever IsSubtypeOf(to, from) rather than the safe,
	// covariant IsSubtypeOf(from, to). The graph cannot verify at author
	// time that the runtime value really is the narrower type, so it's
	// allowed but flagged — see design.md §4.3.
	TypeUnsafe bool
}

// Allowed reports whether the connection may be made at all.
func (c Connection) Allowed() bool {
	return c.Direct || len(c.Candidates) > 0
}

// AutoApply returns the single transform the editor may attach without
// asking, and whether there is one.
//
// Exactly one unambiguous candidate is applied silently and shown as a chip
// on the edge; anything else prompts. This keeps the common file-to-directory
// case at one click while never guessing between defensible alternatives.
func (c Connection) AutoApply() (Transform, bool) {
	if c.Direct || len(c.Candidates) != 1 {
		return Transform{}, false
	}
	if c.Candidates[0].Ambiguous {
		return Transform{}, false
	}
	return c.Candidates[0], true
}

// CanConnect reports how a value of type from may reach a socket of type to.
func CanConnect(from, to Type) Connection {
	if IsSubtypeOf(from, to) {
		return Connection{Direct: true}
	}
	if IsSubtypeOf(to, from) {
		return Connection{Direct: true, TypeUnsafe: true}
	}
	if SameRepresentation(from, to) {
		// Not a narrowing — neither is a subtype of the other — so no
		// TypeUnsafe: this is asserted equivalence, not an unverifiable
		// guess about the runtime value's shape.
		return Connection{Direct: true}
	}

	var candidates []Transform
	for _, transform := range transformRegistry {
		// The source must satisfy the transform's input, and the transform's
		// output must satisfy the target — both by subtyping, so that
		// path.dir reaches media.file.parentDir's From and its path.dir
		// result reaches a socket declared merely path.
		if IsSubtypeOf(from, transform.From) && IsSubtypeOf(transform.To, to) {
			candidates = append(candidates, transform)
		}
	}
	if wrap, ok := wrapTransform(from, to); ok {
		candidates = append(candidates, wrap)
	}
	return Connection{Candidates: candidates}
}

// wrapTransform offers the synthetic "wrap" transform (T -> list<T>, design.md
// §4.3) when to is a list and from — a non-list — is a subtype of its element
// type. This can't be one static transformRegistry entry: a registry entry
// tests subtyping against a *concrete* From/To pair, but "wrap" must match
// list<T> for every T, not just one hardcoded element type.
func wrapTransform(from, to Type) (Transform, bool) {
	if _, fromIsList := from.ElementType(); fromIsList {
		return Transform{}, false
	}
	element, toIsList := to.ElementType()
	if !toIsList || !IsSubtypeOf(from, element) {
		return Transform{}, false
	}
	return Transform{
		Name: "wrap", From: from, To: to,
		Summary: "Wraps the single value into a one-element list",
	}, true
}

// ExplainIncompatible returns a human-readable reason a connection was
// refused, for the editor to show verbatim.
func ExplainIncompatible(from, to Type) string {
	if _, fromIsList := from.ElementType(); fromIsList {
		if _, toIsList := to.ElementType(); !toIsList {
			return "A collection cannot feed a single value. Use a For Each node to work through it one item at a time."
		}
	}
	if _, toIsList := to.ElementType(); toIsList {
		if _, fromIsList := from.ElementType(); !fromIsList {
			return "A single value cannot feed a collection. Collect values inside a loop instead."
		}
	}
	return string(from) + " cannot connect to " + string(to) + "."
}
