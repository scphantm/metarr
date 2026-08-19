package scanmodel

// This file turns the stored sidecar classification table
// (appconfig.SidecarTypeDefinition) into something the scanner can ask
// questions of. The table itself is configuration so a library with unusual
// naming can be accommodated without a redeploy; the category vocabulary stays
// here in Go, because "give me every image" only means something if the set of
// categories is closed.

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"

	"Metarr/internal/shared/appconfig"
)

// SidecarCategory groups sidecar types so callers can ask for a whole class of
// files — every image, every extra video — without naming each type. This is a
// closed vocabulary: configuration may invent new types, but each one has to
// declare itself as one of these.
type SidecarCategory string

const (
	SidecarCategoryImage         SidecarCategory = "image"
	SidecarCategoryVideoExtra    SidecarCategory = "video_extra"
	SidecarCategorySubtitle      SidecarCategory = "subtitle"
	SidecarCategoryMetadata      SidecarCategory = "metadata"
	SidecarCategoryAudio         SidecarCategory = "audio"
	SidecarCategoryDiscStructure SidecarCategory = "disc_structure"
	// SidecarCategoryTrickplay is its own class rather than a kind of image
	// because a single video's previews run to hundreds of tiles, which would
	// swamp the artwork queries "every image" exists to answer.
	SidecarCategoryTrickplay SidecarCategory = "trickplay"
	SidecarCategoryUnknown   SidecarCategory = "unknown"
)

// validSidecarCategories is ordered so error messages list the vocabulary
// predictably.
var validSidecarCategories = []SidecarCategory{
	SidecarCategoryImage,
	SidecarCategoryVideoExtra,
	SidecarCategorySubtitle,
	SidecarCategoryMetadata,
	SidecarCategoryAudio,
	SidecarCategoryDiscStructure,
	SidecarCategoryTrickplay,
	SidecarCategoryUnknown,
}

// ParseSidecarCategory validates a category value from a stored sidecar type
// definition. It is the single source of truth for the vocabulary, shared by
// the config handlers that save a definition and the registry that compiles
// one, so an unusable category can't be stored in the first place.
func ParseSidecarCategory(category string) (SidecarCategory, error) {
	for _, candidate := range validSidecarCategories {
		if SidecarCategory(category) == candidate {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("mediascan: unknown sidecar category %q, expected one of %s", category, ValidSidecarCategoriesText())
}

// ValidSidecarCategoriesText renders the accepted categories for error
// messages.
func ValidSidecarCategoriesText() string {
	names := make([]string, 0, len(validSidecarCategories))
	for _, category := range validSidecarCategories {
		names = append(names, string(category))
	}
	return joinQuoted(names)
}

// SidecarType names one recognized kind of sidecar file. Unlike the categories
// this is an open vocabulary — configuration may define types this package has
// never heard of — so these constants name the built-in defaults rather than
// bounding what is possible.
type SidecarType string

const (
	// Artwork types, category "image".

	SidecarPoster    SidecarType = "poster"
	SidecarFanart    SidecarType = "fanart"
	SidecarBanner    SidecarType = "banner"
	SidecarClearLogo SidecarType = "clearlogo"
	SidecarClearArt  SidecarType = "clearart"
	SidecarDiscArt   SidecarType = "discart"
	SidecarThumb     SidecarType = "thumb"
	SidecarLandscape SidecarType = "landscape"

	// Extra video types, category "video_extra".

	SidecarTrailer         SidecarType = "trailer"
	SidecarBehindTheScenes SidecarType = "behind_the_scenes"
	SidecarDeletedScene    SidecarType = "deleted_scene"
	SidecarFeaturette      SidecarType = "featurette"
	SidecarInterview       SidecarType = "interview"
	SidecarScene           SidecarType = "scene"
	SidecarShort           SidecarType = "short"
	SidecarOtherExtra      SidecarType = "other_extra"

	SidecarSubtitle      SidecarType = "subtitle"
	SidecarNFO           SidecarType = "nfo"
	SidecarTheme         SidecarType = "theme"
	SidecarDiscStructure SidecarType = "disc_structure"

	// SidecarTrickplay is one tile sheet from Jellyfin's scrubbing previews.
	SidecarTrickplay SidecarType = "trickplay"

	// SidecarUnknown is a file the scanner recorded but the table could not
	// name, which is different from a file that was never looked at.
	SidecarUnknown SidecarType = "unknown"
)

// SidecarTypeDefinition is the compiled, typed form of one stored table entry.
// The stored form (appconfig.SidecarTypeDefinition) carries plain strings for
// JSON and BSON; this one carries the typed vocabulary and the compiled
// patterns the scanner actually matches against.
type SidecarTypeDefinition struct {
	ID         string
	Type       SidecarType
	Category   SidecarCategory
	Order      int
	Patterns   []string
	Extensions []string

	compiled []*regexp.Regexp
}

// Enabled reports whether this type takes part in classification. A definition
// ordered zero stays in the table, still editable and still inspectable, but is
// never evaluated — which is how a built-in type is switched off without losing
// its patterns.
func (definition SidecarTypeDefinition) Enabled() bool {
	return definition.Order != 0
}

// Matches reports whether a base file name — the name with its extension
// removed — matches any of this type's patterns.
func (definition SidecarTypeDefinition) Matches(baseName string) bool {
	for _, pattern := range definition.compiled {
		if pattern.MatchString(baseName) {
			return true
		}
	}
	return false
}

// AcceptsExtension reports whether a lowercase, dot-prefixed extension passes
// this type's extension gate. A type declaring no extensions accepts any.
func (definition SidecarTypeDefinition) AcceptsExtension(extension string) bool {
	if len(definition.Extensions) == 0 {
		return true
	}
	for _, accepted := range definition.Extensions {
		if accepted == extension {
			return true
		}
	}
	return false
}

// SidecarRegistry is a compiled sidecar classification table. Building one is
// the only place patterns are compiled, which is what lets a bad pattern be
// reported to whoever is saving the configuration rather than surfacing as a
// panic in the middle of a library scan.
//
// A registry is immutable once built. Configuration changes produce a new one
// and swap it in, so a scan already in flight keeps reading a consistent table.
type SidecarRegistry struct {
	// definitions holds the enabled entries in evaluation order. Match walks it
	// top to bottom, so this is where the ordering actually bites.
	definitions []SidecarTypeDefinition

	// all holds every entry including the disabled ones, in the same order.
	all []SidecarTypeDefinition

	// byName carries disabled entries too. CategoryOf reads it, and a type the
	// scanner assigned from folder context still needs its category resolved
	// even in the moment it is being switched off.
	byName map[SidecarType]SidecarTypeDefinition

	// byCategory is enabled entries only: a disabled type can never land on a
	// file, so listing it under "every image" would be a lie.
	byCategory map[SidecarCategory][]SidecarTypeDefinition
}

// NewSidecarRegistry compiles definitions into a registry.
//
// Every invalid entry is reported, not just the first, because the caller is
// usually a human fixing a configuration document and one round trip per typo
// is a poor way to spend their afternoon.
//
// An empty list yields the built-in defaults rather than a table that matches
// nothing, so a configuration written before this section existed still scans.
func NewSidecarRegistry(definitions []appconfig.SidecarTypeDefinition) (*SidecarRegistry, error) {
	if len(definitions) == 0 {
		definitions = appconfig.DefaultSidecarTypes()
	}

	registry := &SidecarRegistry{
		all:        make([]SidecarTypeDefinition, 0, len(definitions)),
		byName:     map[SidecarType]SidecarTypeDefinition{},
		byCategory: map[SidecarCategory][]SidecarTypeDefinition{},
	}

	var problems []string
	idsSeen := map[string]string{}
	ordersSeen := map[int]string{}

	for index, entry := range definitions {
		if entry.Type == "" {
			problems = append(problems, fmt.Sprintf("entry %d: type is required", index))
			continue
		}
		if entry.ID == "" {
			problems = append(problems, fmt.Sprintf("entry %d (%q): id is required", index, entry.Type))
			continue
		}
		if existing, duplicate := idsSeen[entry.ID]; duplicate {
			problems = append(problems, fmt.Sprintf("entry %d (%q): id %q is already used by %q", index, entry.Type, entry.ID, existing))
			continue
		}
		if _, duplicate := registry.byName[SidecarType(entry.Type)]; duplicate {
			problems = append(problems, fmt.Sprintf("entry %d (%q): duplicate type", index, entry.Type))
			continue
		}

		// Two enabled entries claiming one slot make "first match wins"
		// ambiguous, so the table is refused rather than quietly resolved by a
		// tiebreak nobody asked for. Zero is exempt: it is the disabled
		// sentinel rather than a position, so any number of types may hold it.
		if entry.Order != 0 {
			if existing, taken := ordersSeen[entry.Order]; taken {
				problems = append(problems, fmt.Sprintf("entry %d (%q): order %d is already used by %q", index, entry.Type, entry.Order, existing))
				continue
			}
		}

		category, err := ParseSidecarCategory(entry.Category)
		if err != nil {
			problems = append(problems, fmt.Sprintf("entry %d (%q): %v", index, entry.Type, err))
			continue
		}
		if len(entry.Patterns) == 0 {
			problems = append(problems, fmt.Sprintf("entry %d (%q): at least one pattern is required", index, entry.Type))
			continue
		}

		definition := SidecarTypeDefinition{
			ID:         entry.ID,
			Type:       SidecarType(entry.Type),
			Category:   category,
			Order:      entry.Order,
			Patterns:   append([]string(nil), entry.Patterns...),
			Extensions: normalizedExtensions(entry.Extensions),
			compiled:   make([]*regexp.Regexp, 0, len(entry.Patterns)),
		}

		// Patterns are compiled even for a disabled entry, so switching one back
		// on can never fail on something that was storable while it was off.
		badPattern := false
		for _, pattern := range entry.Patterns {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				problems = append(problems, fmt.Sprintf("entry %d (%q): pattern %q: %v", index, entry.Type, pattern, err))
				badPattern = true
				continue
			}
			definition.compiled = append(definition.compiled, compiled)
		}
		if badPattern {
			continue
		}

		idsSeen[definition.ID] = entry.Type
		if definition.Order != 0 {
			ordersSeen[definition.Order] = entry.Type
		}

		registry.all = append(registry.all, definition)
		registry.byName[definition.Type] = definition
		if definition.Enabled() {
			registry.definitions = append(registry.definitions, definition)
			registry.byCategory[definition.Category] = append(registry.byCategory[definition.Category], definition)
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("mediascan: invalid sidecar type table:\n  %s", strings.Join(problems, "\n  "))
	}

	// Order is the evaluation sequence; position in the stored array means
	// nothing. Duplicates having been rejected above, this sort is total, so no
	// tiebreak is needed for the result to be reproducible.
	sort.Slice(registry.definitions, func(i, j int) bool {
		return registry.definitions[i].Order < registry.definitions[j].Order
	})
	sort.SliceStable(registry.all, func(i, j int) bool {
		return sortKeyFor(registry.all[i]) < sortKeyFor(registry.all[j])
	})

	return registry, nil
}

// sortKeyFor orders the full table for display. Disabled entries have no
// position of their own, so they sort after every enabled one rather than
// bunching at the front where order zero would otherwise put them.
func sortKeyFor(definition SidecarTypeDefinition) int {
	if !definition.Enabled() {
		return math.MaxInt
	}
	return definition.Order
}

// normalizedExtensions lowercases extensions and gives them a leading dot, so a
// configuration written as "JPG" or "jpg" behaves the same as ".jpg".
func normalizedExtensions(extensions []string) []string {
	if len(extensions) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(extensions))
	for _, extension := range extensions {
		extension = strings.ToLower(strings.TrimSpace(extension))
		if extension == "" {
			continue
		}
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		normalized = append(normalized, extension)
	}
	return normalized
}

// Match classifies a file name, returning the first enabled entry whose
// patterns and extension gate both accept it. Entries are walked in order, which
// is why the defaults give the catch-alls the highest numbers; disabled entries
// are not walked at all.
func (r *SidecarRegistry) Match(fileName string) (SidecarTypeDefinition, bool) {
	extension := strings.ToLower(filepath.Ext(fileName))
	baseName := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))

	for _, definition := range r.definitions {
		if definition.AcceptsExtension(extension) && definition.Matches(baseName) {
			return definition, true
		}
	}
	return SidecarTypeDefinition{}, false
}

// ByCategory returns every enabled type in a category, in evaluation order. The
// returned slice is a copy, so callers cannot reorder the registry through it.
func (r *SidecarRegistry) ByCategory(category SidecarCategory) []SidecarTypeDefinition {
	definitions := r.byCategory[category]
	result := make([]SidecarTypeDefinition, len(definitions))
	copy(result, definitions)
	return result
}

// Lookup returns the entry for a type name, disabled entries included. A
// disabled type still has to resolve: the scanner can name one from folder
// context, and it needs a category for the record either way.
func (r *SidecarRegistry) Lookup(sidecarType SidecarType) (SidecarTypeDefinition, bool) {
	definition, found := r.byName[sidecarType]
	return definition, found
}

// IsEnabled reports whether a type is configured and switched on.
func (r *SidecarRegistry) IsEnabled(sidecarType SidecarType) bool {
	definition, found := r.byName[sidecarType]
	return found && definition.Enabled()
}

// Definitions returns the enabled table in evaluation order, as a copy.
func (r *SidecarRegistry) Definitions() []SidecarTypeDefinition {
	result := make([]SidecarTypeDefinition, len(r.definitions))
	copy(result, r.definitions)
	return result
}

// AllDefinitions returns the whole table as a copy, disabled entries included,
// ordered with the enabled ones first.
func (r *SidecarRegistry) AllDefinitions() []SidecarTypeDefinition {
	result := make([]SidecarTypeDefinition, len(r.all))
	copy(result, r.all)
	return result
}

// activeSidecarRegistry is the table the scanner classifies against. It is
// swapped atomically when configuration changes, so a running scan never sees a
// half-updated table.
var activeSidecarRegistry atomic.Pointer[SidecarRegistry]

// init installs the built-in defaults, so the package classifies correctly
// before any configuration has been loaded — which is also what makes this
// package usable from a test without standing up a database.
//
// A failure here means a broken constant in appconfig.DefaultSidecarTypes, not
// bad user input, so it panics: the same contract the hardcoded table had when
// it compiled its patterns with regexp.MustCompile.
func init() {
	registry, err := NewSidecarRegistry(nil)
	if err != nil {
		panic("mediascan: built-in sidecar type table is invalid: " + err.Error())
	}
	activeSidecarRegistry.Store(registry)
}

// SetSidecarRegistry installs registry as the table the scanner classifies
// against. A nil registry is ignored rather than leaving the scanner without
// one.
func SetSidecarRegistry(registry *SidecarRegistry) {
	if registry == nil {
		return
	}
	activeSidecarRegistry.Store(registry)
}

// ActiveSidecarRegistry returns the table currently in force.
func ActiveSidecarRegistry() *SidecarRegistry {
	return activeSidecarRegistry.Load()
}

// The package-level helpers below read the active registry. They are what the
// scanner and most callers use; reach for a *SidecarRegistry directly only when
// you need to ask questions of a table that isn't the active one, such as
// validating a configuration before saving it.

// MatchSidecarType classifies a file name against the active table. This is the
// entry point the scanner uses to identify a file it found on disk.
func MatchSidecarType(fileName string) (SidecarTypeDefinition, bool) {
	return ActiveSidecarRegistry().Match(fileName)
}

// SidecarTypesByCategory returns every sidecar type in a category — every
// image, every extra video — in table order.
func SidecarTypesByCategory(category SidecarCategory) []SidecarTypeDefinition {
	return ActiveSidecarRegistry().ByCategory(category)
}

// LookupSidecarType returns the active table's entry for a type name.
func LookupSidecarType(sidecarType SidecarType) (SidecarTypeDefinition, bool) {
	return ActiveSidecarRegistry().Lookup(sidecarType)
}

// CategoryOf returns the category a sidecar type belongs to, or
// SidecarCategoryUnknown for a name the active table doesn't carry.
func CategoryOf(sidecarType SidecarType) SidecarCategory {
	if definition, found := LookupSidecarType(sidecarType); found {
		return definition.Category
	}
	return SidecarCategoryUnknown
}

// ParseSidecarType validates a sidecar type value arriving from storage or an
// API request against the active table.
func ParseSidecarType(sidecarType string) (SidecarType, error) {
	if _, found := LookupSidecarType(SidecarType(sidecarType)); found {
		return SidecarType(sidecarType), nil
	}
	return "", fmt.Errorf("mediascan: unknown sidecar type %q, expected one of %s", sidecarType, ValidSidecarTypesText())
}

// ValidSidecarTypesText renders the configured sidecar types for error
// messages, in table order.
func ValidSidecarTypesText() string {
	definitions := ActiveSidecarRegistry().Definitions()
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, string(definition.Type))
	}
	return joinQuoted(names)
}
