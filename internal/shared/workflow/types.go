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
// Type names form a dotted-prefix hierarchy: "path.file.video" is a subtype
// of "path.file", which is a subtype of "path". That is the whole mechanism —
// there is no lattice structure to maintain, and a new type needs no code
// change beyond naming it. The one exception is list, which is a generic
// constructor written list<T> and is covariant in T.
type Type string

const (
	// TypeAny is the top of the hierarchy: everything is assignable to it.
	TypeAny Type = "any"

	TypeBool       Type = "bool"
	TypeNumber     Type = "number"
	TypeNumberInt  Type = "number.int"
	TypeString     Type = "string"
	TypeStringEnum Type = "string.enum"
	// TypeDuration is milliseconds.
	TypeDuration  Type = "duration"
	TypeBytes     Type = "bytes"
	TypeTimestamp Type = "timestamp"

	// Path types. Every path value inside a workflow is in server-canonical
	// space; translation to an agent's own space happens once, in the
	// dispatch layer. See design.md §4.2.
	TypePath             Type = "path"
	TypePathDir          Type = "path.dir"
	TypePathFile         Type = "path.file"
	TypePathFileVideo    Type = "path.file.video"
	TypePathFileImage    Type = "path.file.image"
	TypePathFileSubtitle Type = "path.file.subtitle"
	TypePathFileNFO      Type = "path.file.nfo"

	// Media types are database records, not paths. media.file is a scanned
	// MediaFile; path.file is a string naming a location. Conflating the two
	// is how every node ends up accepting "any".
	TypeMediaItem      Type = "media.item"
	TypeMediaFile      Type = "media.file"
	TypeMediaSidecar   Type = "media.sidecar"
	TypeMetadataNFO    Type = "metadata.nfo"
	TypeMetadataStream Type = "metadata.stream"

	TypeAgentSlug   Type = "agent.slug"
	TypeScannerSlug Type = "scanner.slug"

	// TypeError is what an error control-out's companion data-out carries.
	TypeError Type = "error"
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

// Transform is a named, explicit conversion between two types, recorded on
// the data edge that uses it.
//
// Transforms are deliberately single names rather than chains: a chain is
// unreadable on an edge and untestable. Useful compositions are registered
// here as their own transform instead — directoryPath is exactly that.
type Transform struct {
	Name string `json:"name"`
	From Type   `json:"from"`
	To   Type   `json:"to"`
	// Ambiguous marks a transform that must never be applied automatically
	// even when it is the only candidate, because more than one answer is
	// defensible and guessing wrong is silent. The UI always prompts.
	Ambiguous bool   `json:"ambiguous,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

// transformRegistry is the complete set of explicit conversions. Anything not
// listed here and not covered by subtyping is a connection error.
var transformRegistry = []Transform{
	{
		Name: "parentDir", From: TypePathFile, To: TypePathDir,
		Summary: "The directory containing the file",
	},
	{
		Name: "toString", From: TypePath, To: TypeString,
		Summary: "The path as text",
	},
	{
		Name: "fileName", From: TypePathFile, To: TypeString, Ambiguous: true,
		Summary: "File name with extension",
	},
	{
		Name: "baseName", From: TypePathFile, To: TypeString, Ambiguous: true,
		Summary: "File name without extension",
	},
	{
		Name: "extension", From: TypePathFile, To: TypeString, Ambiguous: true,
		Summary: "File extension only",
	},
	{
		Name: "filePath", From: TypeMediaFile, To: TypePathFile,
		Summary: "The record's own path",
	},
	{
		Name: "directoryPath", From: TypeMediaFile, To: TypePathDir,
		Summary: "The directory containing the record's file",
	},
	{
		Name: "itemPath", From: TypeMediaItem, To: TypePathDir,
		Summary: "The item's directory",
	},
	{
		Name: "seconds", From: TypeDuration, To: TypeNumber, Ambiguous: true,
		Summary: "Duration in seconds",
	},
	{
		Name: "milliseconds", From: TypeDuration, To: TypeNumber, Ambiguous: true,
		Summary: "Duration in milliseconds",
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

// FindTransform returns the registered transform with the given name.
func FindTransform(name string) (Transform, bool) {
	for _, transform := range transformRegistry {
		if transform.Name == name {
			return transform, true
		}
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

	var candidates []Transform
	for _, transform := range transformRegistry {
		// The source must satisfy the transform's input, and the transform's
		// output must satisfy the target — both by subtyping, so that
		// path.file.video reaches parentDir and its path.dir result reaches
		// a socket declared merely path.
		if IsSubtypeOf(from, transform.From) && IsSubtypeOf(transform.To, to) {
			candidates = append(candidates, transform)
		}
	}
	return Connection{Candidates: candidates}
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
