package scanmodel

import (
	"strings"
	"testing"

	"Metarr/internal/shared/appconfig"
)

// TestMatchSidecarType pins the registry's classification behaviour, including
// the extension gate that keeps a type from claiming a file whose name happens
// to match but whose extension makes no sense for it.
func TestMatchSidecarType(t *testing.T) {
	testCases := []struct {
		name         string
		fileName     string
		wantMatched  bool
		wantType     SidecarType
		wantCategory SidecarCategory
	}{
		{
			name:         "suffixed trailer",
			fileName:     "The Movie (1999)-trailer.mkv",
			wantMatched:  true,
			wantType:     SidecarTrailer,
			wantCategory: SidecarCategoryVideoExtra,
		},
		{
			name:         "bare trailer",
			fileName:     "trailer.mp4",
			wantMatched:  true,
			wantType:     SidecarTrailer,
			wantCategory: SidecarCategoryVideoExtra,
		},
		{
			name:         "poster",
			fileName:     "poster.jpg",
			wantMatched:  true,
			wantType:     SidecarPoster,
			wantCategory: SidecarCategoryImage,
		},
		{
			name:         "folder art is a poster",
			fileName:     "folder.png",
			wantMatched:  true,
			wantType:     SidecarPoster,
			wantCategory: SidecarCategoryImage,
		},
		{
			name:        "poster name with a non-image extension is not a poster",
			fileName:    "poster.txt",
			wantMatched: false,
		},
		{
			name:        "trailer name with an image extension is not a trailer",
			fileName:    "movie-trailer.jpg",
			wantMatched: false,
		},
		{
			name:         "fanart",
			fileName:     "fanart2.jpg",
			wantMatched:  true,
			wantType:     SidecarFanart,
			wantCategory: SidecarCategoryImage,
		},
		{
			name:         "any nfo is metadata",
			fileName:     "The Movie (1999).nfo",
			wantMatched:  true,
			wantType:     SidecarNFO,
			wantCategory: SidecarCategoryMetadata,
		},
		{
			name:         "any subtitle extension is a subtitle",
			fileName:     "The Movie (1999).en.forced.srt",
			wantMatched:  true,
			wantType:     SidecarSubtitle,
			wantCategory: SidecarCategorySubtitle,
		},
		{
			name:         "theme music",
			fileName:     "theme.mp3",
			wantMatched:  true,
			wantType:     SidecarTheme,
			wantCategory: SidecarCategoryAudio,
		},
		{
			name:         "behind the scenes",
			fileName:     "Making Of-behind the scenes.mkv",
			wantMatched:  true,
			wantType:     SidecarBehindTheScenes,
			wantCategory: SidecarCategoryVideoExtra,
		},
		{
			name:         "disc structure by extension",
			fileName:     "VTS_01_0.IFO",
			wantMatched:  true,
			wantType:     SidecarDiscStructure,
			wantCategory: SidecarCategoryDiscStructure,
		},
		{
			name:        "an ordinary media file is not a sidecar",
			fileName:    "The Movie (1999).mkv",
			wantMatched: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			definition, matched := MatchSidecarType(testCase.fileName)
			if matched != testCase.wantMatched {
				t.Fatalf("MatchSidecarType(%q) matched = %v, want %v (got type %q)", testCase.fileName, matched, testCase.wantMatched, definition.Type)
			}
			if !testCase.wantMatched {
				return
			}
			if definition.Type != testCase.wantType {
				t.Errorf("MatchSidecarType(%q) type = %q, want %q", testCase.fileName, definition.Type, testCase.wantType)
			}
			if definition.Category != testCase.wantCategory {
				t.Errorf("MatchSidecarType(%q) category = %q, want %q", testCase.fileName, definition.Category, testCase.wantCategory)
			}
		})
	}
}

// TestSidecarTypesByCategory covers the "return all images" / "return all
// trailers" style query the categories exist to serve.
func TestSidecarTypesByCategory(t *testing.T) {
	imageTypes := SidecarTypesByCategory(SidecarCategoryImage)
	if len(imageTypes) == 0 {
		t.Fatal("SidecarTypesByCategory(image) returned nothing")
	}
	for _, definition := range imageTypes {
		if definition.Category != SidecarCategoryImage {
			t.Errorf("image category contains %q, whose category is %q", definition.Type, definition.Category)
		}
	}

	foundPoster := false
	for _, definition := range imageTypes {
		if definition.Type == SidecarPoster {
			foundPoster = true
		}
		if definition.Type == SidecarTrailer {
			t.Error("image category contains the trailer type")
		}
	}
	if !foundPoster {
		t.Error("image category is missing the poster type")
	}

	videoExtras := SidecarTypesByCategory(SidecarCategoryVideoExtra)
	foundTrailer := false
	for _, definition := range videoExtras {
		if definition.Type == SidecarTrailer {
			foundTrailer = true
		}
	}
	if !foundTrailer {
		t.Error("video_extra category is missing the trailer type")
	}

	if got := SidecarTypesByCategory("no_such_category"); len(got) != 0 {
		t.Errorf("SidecarTypesByCategory(unknown) = %v, want empty", got)
	}
}

// TestSidecarTypesByCategoryReturnsACopy guards the registry against a caller
// that sorts or truncates the slice it was handed.
func TestSidecarTypesByCategoryReturnsACopy(t *testing.T) {
	first := SidecarTypesByCategory(SidecarCategoryImage)
	first[0] = SidecarTypeDefinition{Type: "tampered"}

	second := SidecarTypesByCategory(SidecarCategoryImage)
	if second[0].Type == "tampered" {
		t.Error("SidecarTypesByCategory handed out the registry's own slice")
	}
}

func TestLookupSidecarTypeAndCategoryOf(t *testing.T) {
	definition, found := LookupSidecarType(SidecarTrailer)
	if !found {
		t.Fatal("LookupSidecarType(trailer) not found")
	}
	if definition.Category != SidecarCategoryVideoExtra {
		t.Errorf("trailer category = %q, want %q", definition.Category, SidecarCategoryVideoExtra)
	}
	if len(definition.Patterns) == 0 {
		t.Error("trailer definition carries no patterns")
	}

	if _, found := LookupSidecarType("no_such_type"); found {
		t.Error("LookupSidecarType(unknown) reported found")
	}

	if got := CategoryOf(SidecarPoster); got != SidecarCategoryImage {
		t.Errorf("CategoryOf(poster) = %q, want %q", got, SidecarCategoryImage)
	}
	if got := CategoryOf("no_such_type"); got != SidecarCategoryUnknown {
		t.Errorf("CategoryOf(unknown) = %q, want %q", got, SidecarCategoryUnknown)
	}
}

func TestParseSidecarType(t *testing.T) {
	parsed, err := ParseSidecarType("trailer")
	if err != nil {
		t.Fatalf("ParseSidecarType(trailer) returned %v", err)
	}
	if parsed != SidecarTrailer {
		t.Errorf("ParseSidecarType(trailer) = %q, want %q", parsed, SidecarTrailer)
	}

	if _, err := ParseSidecarType("nope"); err == nil {
		t.Fatal("ParseSidecarType(nope) returned no error")
	}
}

// withSidecarRegistry installs registry for the duration of one test and puts
// the previous one back afterwards, so a test that changes the table cannot
// leak that change into the ones that follow it.
func withSidecarRegistry(t *testing.T, registry *SidecarRegistry) {
	t.Helper()
	previous := ActiveSidecarRegistry()
	SetSidecarRegistry(registry)
	t.Cleanup(func() { SetSidecarRegistry(previous) })
}

// TestDefaultTableIsWhatThePackageShipsWith is the guard on the move into
// configuration: the stored defaults must classify exactly the way the package
// does out of the box. If these ever disagree, a fresh install and an install
// that has never touched its config would behave differently.
func TestDefaultTableIsWhatThePackageShipsWith(t *testing.T) {
	registry, err := NewSidecarRegistry(appconfig.DefaultSidecarTypes())
	if err != nil {
		t.Fatalf("NewSidecarRegistry(defaults) error = %v", err)
	}

	fileNames := []string{
		"poster.jpg", "folder.png", "fanart2.jpg", "backdrop-1.jpg", "banner.jpg",
		"clearlogo.png", "clearart.png", "discart.png", "landscape.jpg", "thumb.jpg",
		"Season01-poster.jpg", "season-specials-poster.jpg",
		"trailer.mp4", "The Movie (1999)-trailer.mkv", "Making Of-behind the scenes.mkv",
		"deleted scenes.mkv", "featurette.mkv", "interview.mkv", "theme.mp3",
		"The Movie (1999).en.forced.srt", "The Movie (1999).nfo", "VTS_01_0.IFO",
		"poster.txt", "The Movie (1999).mkv", "readme.txt",
	}

	for _, fileName := range fileNames {
		wantDefinition, wantMatched := registry.Match(fileName)
		gotDefinition, gotMatched := MatchSidecarType(fileName)

		if gotMatched != wantMatched {
			t.Errorf("%s: package matched = %v, stored defaults matched = %v", fileName, gotMatched, wantMatched)
			continue
		}
		if gotDefinition.Type != wantDefinition.Type || gotDefinition.Category != wantDefinition.Category {
			t.Errorf("%s: package = %q/%q, stored defaults = %q/%q",
				fileName, gotDefinition.Type, gotDefinition.Category, wantDefinition.Type, wantDefinition.Category)
		}
	}
}

// TestNewSidecarRegistryFallsBackToDefaults covers a configuration written
// before the table existed: it must scan, not classify everything as unknown.
func TestNewSidecarRegistryFallsBackToDefaults(t *testing.T) {
	registry, err := NewSidecarRegistry(nil)
	if err != nil {
		t.Fatalf("NewSidecarRegistry(nil) error = %v", err)
	}
	if len(registry.Definitions()) != len(appconfig.DefaultSidecarTypes()) {
		t.Errorf("empty table produced %d entries, want the %d defaults",
			len(registry.Definitions()), len(appconfig.DefaultSidecarTypes()))
	}
	if definition, matched := registry.Match("poster.jpg"); !matched || definition.Type != SidecarPoster {
		t.Errorf("poster.jpg = %q (matched %v), want %q", definition.Type, matched, SidecarPoster)
	}
}

// TestNewSidecarRegistryRejectsBadTables checks that a table saved through the
// API is validated where the person saving it can see the error, and that every
// problem is reported rather than only the first.
func TestNewSidecarRegistryRejectsBadTables(t *testing.T) {
	testCases := []struct {
		name        string
		definitions []appconfig.SidecarTypeDefinition
		wantInError []string
	}{
		{
			name: "invalid pattern",
			definitions: []appconfig.SidecarTypeDefinition{
				{ID: "a", Type: "broken", Category: "image", Order: 10, Patterns: []string{`(?i)^[unclosed`}},
			},
			wantInError: []string{"broken", "unclosed"},
		},
		{
			name: "unknown category",
			definitions: []appconfig.SidecarTypeDefinition{
				{ID: "a", Type: "bloopers", Category: "video_extras", Order: 10, Patterns: []string{`(?i)^bloopers$`}},
			},
			wantInError: []string{"bloopers", "video_extras"},
		},
		{
			name: "duplicate type",
			definitions: []appconfig.SidecarTypeDefinition{
				{ID: "a", Type: "poster", Category: "image", Order: 10, Patterns: []string{`(?i)^poster$`}},
				{ID: "b", Type: "poster", Category: "image", Order: 20, Patterns: []string{`(?i)^cover$`}},
			},
			wantInError: []string{"duplicate type"},
		},
		{
			name: "missing type",
			definitions: []appconfig.SidecarTypeDefinition{
				{ID: "a", Category: "image", Order: 10, Patterns: []string{`(?i)^poster$`}},
			},
			wantInError: []string{"type is required"},
		},
		{
			name: "missing id",
			definitions: []appconfig.SidecarTypeDefinition{
				{Type: "poster", Category: "image", Order: 10, Patterns: []string{`(?i)^poster$`}},
			},
			wantInError: []string{"poster", "id is required"},
		},
		{
			name: "duplicate id",
			definitions: []appconfig.SidecarTypeDefinition{
				{ID: "a", Type: "poster", Category: "image", Order: 10, Patterns: []string{`(?i)^poster$`}},
				{ID: "a", Type: "banner", Category: "image", Order: 20, Patterns: []string{`(?i)^banner$`}},
			},
			wantInError: []string{"banner", `id "a" is already used by "poster"`},
		},
		{
			// Two enabled entries in one slot leave "first match wins"
			// undecidable, so the table is refused rather than resolved.
			name: "duplicate order",
			definitions: []appconfig.SidecarTypeDefinition{
				{ID: "a", Type: "poster", Category: "image", Order: 10, Patterns: []string{`(?i)^poster$`}},
				{ID: "b", Type: "banner", Category: "image", Order: 10, Patterns: []string{`(?i)^banner$`}},
			},
			wantInError: []string{"banner", `order 10 is already used by "poster"`},
		},
		{
			name: "no patterns",
			definitions: []appconfig.SidecarTypeDefinition{
				{ID: "a", Type: "empty", Category: "image", Order: 10},
			},
			wantInError: []string{"at least one pattern"},
		},
		{
			// A disabled entry is still stored config that will be switched on
			// one day, so it has to be well formed now.
			name: "disabled entry with a broken pattern",
			definitions: []appconfig.SidecarTypeDefinition{
				{ID: "a", Type: "broken", Category: "image", Order: 0, Patterns: []string{`(?i)^[unclosed`}},
			},
			wantInError: []string{"broken", "unclosed"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			registry, err := NewSidecarRegistry(testCase.definitions)
			if err == nil {
				t.Fatalf("NewSidecarRegistry() accepted an invalid table, returning %+v", registry.Definitions())
			}
			for _, want := range testCase.wantInError {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

func TestNewSidecarRegistryReportsEveryProblem(t *testing.T) {
	_, err := NewSidecarRegistry([]appconfig.SidecarTypeDefinition{
		{ID: "a", Type: "first", Category: "nonsense", Order: 10, Patterns: []string{`(?i)^first$`}},
		{ID: "b", Type: "second", Category: "image", Order: 20, Patterns: []string{`(?i)^[unclosed`}},
		{ID: "c", Type: "third", Category: "image", Order: 30, Patterns: []string{`(?i)^third$`}},
		{ID: "d", Type: "fourth", Category: "image", Order: 30, Patterns: []string{`(?i)^fourth$`}},
	})
	if err == nil {
		t.Fatal("NewSidecarRegistry() accepted an invalid table")
	}
	// Three independent faults, all surfaced in one pass, because the caller is
	// usually a human fixing a document and one round trip per typo is a poor
	// way to spend an afternoon.
	for _, want := range []string{"first", "second", "fourth"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name every bad entry; %q is missing from %q", want, err)
		}
	}
}

// TestRejectedEntryDoesNotClaimAnOrder pins a consequence of reporting problems
// rather than stopping at the first: an entry that failed validation must not
// also be blamed for a duplicate order, or one typo would cascade into two
// errors and send someone chasing a slot that was never taken.
func TestRejectedEntryDoesNotClaimAnOrder(t *testing.T) {
	_, err := NewSidecarRegistry([]appconfig.SidecarTypeDefinition{
		{ID: "a", Type: "broken", Category: "image", Order: 10, Patterns: []string{`(?i)^[unclosed`}},
		{ID: "b", Type: "fine", Category: "image", Order: 10, Patterns: []string{`(?i)^fine$`}},
	})
	if err == nil {
		t.Fatal("NewSidecarRegistry() accepted a table with a broken pattern")
	}
	if strings.Contains(err.Error(), "already used") {
		t.Errorf("the valid entry was blamed for a duplicate order held by a rejected one: %q", err)
	}
}

// TestDisabledEntriesAreNotDuplicates covers the exemption that makes the
// sentinel usable: order zero is "off", not a slot, so more than one type has to
// be able to hold it.
func TestDisabledEntriesAreNotDuplicates(t *testing.T) {
	registry, err := NewSidecarRegistry([]appconfig.SidecarTypeDefinition{
		{ID: "a", Type: "poster", Category: "image", Order: 0, Patterns: []string{`(?i)^poster$`}},
		{ID: "b", Type: "banner", Category: "image", Order: 0, Patterns: []string{`(?i)^banner$`}},
		{ID: "c", Type: "fanart", Category: "image", Order: 10, Patterns: []string{`(?i)^fanart$`}},
	})
	if err != nil {
		t.Fatalf("NewSidecarRegistry() rejected several disabled entries: %v", err)
	}
	if enabled := registry.Definitions(); len(enabled) != 1 || enabled[0].Type != SidecarFanart {
		t.Errorf("enabled table = %+v, want just fanart", enabled)
	}
	if all := registry.AllDefinitions(); len(all) != 3 {
		t.Errorf("AllDefinitions() = %d entries, want 3 — disabled entries stay in the table", len(all))
	}
}

// TestOrderDecidesEvaluationNotPosition is the point of the change: the stored
// array here puts the catch-all first, but it is numbered last, so the narrow
// entry still wins.
func TestOrderDecidesEvaluationNotPosition(t *testing.T) {
	registry, err := NewSidecarRegistry([]appconfig.SidecarTypeDefinition{
		{ID: "a", Type: "catchall", Category: "unknown", Order: 90, Patterns: []string{`(?i)^.+$`}, Extensions: []string{".jpg"}},
		{ID: "b", Type: "poster", Category: "image", Order: 10, Patterns: []string{`(?i)^poster$`}, Extensions: []string{".jpg"}},
	})
	if err != nil {
		t.Fatalf("NewSidecarRegistry() error = %v", err)
	}
	withSidecarRegistry(t, registry)

	if definition, _ := MatchSidecarType("poster.jpg"); definition.Type != SidecarPoster {
		t.Errorf("poster.jpg = %q, want %q — order should beat array position", definition.Type, SidecarPoster)
	}
	if definition, _ := MatchSidecarType("something-else.jpg"); definition.Type != "catchall" {
		t.Errorf("something-else.jpg = %q, want the catch-all", definition.Type)
	}
}

// TestDisabledTypeIsNeverMatched covers what order zero is for, and what it must
// not break: the entry stops classifying but stays fully inspectable, so the
// scanner can still resolve a category for a type it names from folder context.
func TestDisabledTypeIsNeverMatched(t *testing.T) {
	registry, err := NewSidecarRegistry([]appconfig.SidecarTypeDefinition{
		{ID: "a", Type: "poster", Category: "image", Order: 0, Patterns: []string{`(?i)^poster$`}, Extensions: []string{".jpg"}},
		{ID: "b", Type: "fanart", Category: "image", Order: 20, Patterns: []string{`(?i)^fanart$`}, Extensions: []string{".jpg"}},
	})
	if err != nil {
		t.Fatalf("NewSidecarRegistry() error = %v", err)
	}
	withSidecarRegistry(t, registry)

	if definition, matched := MatchSidecarType("poster.jpg"); matched {
		t.Errorf("poster.jpg matched %q, but poster is disabled", definition.Type)
	}
	if _, matched := MatchSidecarType("fanart.jpg"); !matched {
		t.Error("fanart should still classify; only poster was disabled")
	}

	// Still in the table, still resolvable — just not evaluated.
	definition, found := LookupSidecarType(SidecarPoster)
	if !found {
		t.Fatal("a disabled type should still be found by Lookup")
	}
	if definition.Enabled() {
		t.Error("a type at order zero reported itself enabled")
	}
	if got := CategoryOf(SidecarPoster); got != SidecarCategoryImage {
		t.Errorf("CategoryOf(poster) = %q, want %q even while disabled", got, SidecarCategoryImage)
	}
	if ActiveSidecarRegistry().IsEnabled(SidecarPoster) {
		t.Error("IsEnabled(poster) is true for a disabled type")
	}

	// But it cannot be the answer to "give me every image".
	for _, definition := range SidecarTypesByCategory(SidecarCategoryImage) {
		if definition.Type == SidecarPoster {
			t.Error("a disabled type was listed under its category")
		}
	}
}

// TestDefaultIDsAreUniqueAndStable guards the hard-coded ids: they are what the
// config API addresses entries by, so a copy-paste collision would make two
// types indistinguishable to it.
func TestDefaultIDsAreUniqueAndStable(t *testing.T) {
	seen := map[string]string{}
	for _, entry := range appconfig.DefaultSidecarTypes() {
		if entry.ID == "" {
			t.Errorf("default type %q has no id", entry.Type)
			continue
		}
		if existing, duplicate := seen[entry.ID]; duplicate {
			t.Errorf("default types %q and %q share id %q", existing, entry.Type, entry.ID)
		}
		seen[entry.ID] = entry.Type
	}

	// Stable across calls, so a caller cannot be handed a fresh identity.
	first, second := appconfig.DefaultSidecarTypes(), appconfig.DefaultSidecarTypes()
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Errorf("default id for %q changed between calls", first[i].Type)
		}
	}
}

// TestConfiguredTypeUnknownToGo is the point of moving the table into
// configuration: a type this package has never heard of has to work, provided
// it declares a category the package does know.
func TestConfiguredTypeUnknownToGo(t *testing.T) {
	registry, err := NewSidecarRegistry([]appconfig.SidecarTypeDefinition{
		{
			ID:         "9f2b1c4a-0000-0000-0000-000000000001",
			Type:       "bloopers",
			Category:   string(SidecarCategoryVideoExtra),
			Order:      10,
			Patterns:   []string{`(?i)^bloopers$`, `(?i)[-._ ]bloopers$`},
			Extensions: []string{"mkv", "MP4"},
		},
	})
	if err != nil {
		t.Fatalf("NewSidecarRegistry() error = %v", err)
	}
	withSidecarRegistry(t, registry)

	definition, matched := MatchSidecarType("The Movie-bloopers.mkv")
	if !matched {
		t.Fatal("a configured type did not classify its own file")
	}
	if definition.Type != "bloopers" || definition.Category != SidecarCategoryVideoExtra {
		t.Errorf("got %q/%q, want %q/%q", definition.Type, definition.Category, "bloopers", SidecarCategoryVideoExtra)
	}

	// Extensions are normalized, so a table written without leading dots or in
	// upper case still gates correctly.
	if _, matched := MatchSidecarType("The Movie-bloopers.mp4"); !matched {
		t.Error("uppercase extension in the table should still match a lowercase file")
	}
	if _, matched := MatchSidecarType("The Movie-bloopers.txt"); matched {
		t.Error("extension gate let through a file the table does not accept")
	}

	// The swapped table is the whole table: the defaults are gone.
	if _, matched := MatchSidecarType("poster.jpg"); matched {
		t.Error("poster.jpg matched under a table that does not define it")
	}
	if got := CategoryOf(SidecarPoster); got != SidecarCategoryUnknown {
		t.Errorf("CategoryOf(poster) = %q under a table without it, want %q", got, SidecarCategoryUnknown)
	}
}

// TestSetSidecarRegistryIgnoresNil guards the scanner against ever being left
// with no table at all.
func TestSetSidecarRegistryIgnoresNil(t *testing.T) {
	before := ActiveSidecarRegistry()
	SetSidecarRegistry(nil)
	if ActiveSidecarRegistry() != before {
		t.Error("SetSidecarRegistry(nil) replaced the active registry")
	}
}

func TestParseSidecarCategory(t *testing.T) {
	if got, err := ParseSidecarCategory("image"); err != nil || got != SidecarCategoryImage {
		t.Errorf("ParseSidecarCategory(image) = %q, %v", got, err)
	}
	if _, err := ParseSidecarCategory("video_extras"); err == nil {
		t.Fatal("ParseSidecarCategory accepted an unknown category")
	} else if !strings.Contains(err.Error(), "video_extra") {
		t.Errorf("error should list the valid categories, got %q", err)
	}
}
