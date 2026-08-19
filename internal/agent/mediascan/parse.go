package mediascan

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// seasonArtworkTokens is the alternation of artwork words a season-scoped file
// may end in, e.g. the "poster" of "Season01-poster.jpg". It is a fragment
// spliced into the season artwork patterns below rather than a pattern of its
// own.
const seasonArtworkTokens = `(?:poster|banner|thumbs?|thumbnail|fanart|backdrop|background|landscape|clearart|clearlogo|logo|discart|cdart|keyart|characterart|art|cover|folder)`

// The order these patterns are applied in matters; see parseVideoName.
var (
	// threeDPattern matches the stereoscopic flag Jellyfin appends, e.g.
	// "Awesome 3D Movie (2022).3D.FTAB".
	threeDPattern = regexp.MustCompile(`(?i)[ ._-]3d(?:[ ._-](hsbs|fsbs|htab|ftab|mvc))?$`)

	// stackPattern matches the multi-part suffix of an item split across files,
	// e.g. "-cd1" or " Part 2". The separator is mandatory: without it, a title
	// ending in these letters would be misread as a part number.
	stackPattern = regexp.MustCompile(`(?i)[ ._-](cd|dvd|part|pt|disc|disk)[ ._-]?(\d{1,2})$`)

	// editionPattern matches Plex's edition tag, e.g. "{edition-Director's Cut}".
	editionPattern = regexp.MustCompile(`(?i)\{edition-([^}]+)\}`)

	// providerIDPattern matches the provider id tags both servers allow in
	// names. These are stripped so titles parse cleanly but deliberately not
	// recorded: external links come from NFO contents, the system of record.
	providerIDPattern = regexp.MustCompile(`(?i)\[(?:imdbid|tmdbid|tvdbid)-[^\]]*\]|\{(?:imdb|tmdb|tvdb)-[^}]*\}`)

	// parenthesizedYearPattern matches a year in parentheses, the form both
	// servers recommend.
	parenthesizedYearPattern = regexp.MustCompile(`\((19|20)\d{2}\)`)

	// bareYearPattern matches an unparenthesized year, common in scene-named
	// files like "Movie.Name.2019.1080p". Only consulted for movies and music
	// videos, since for TV it would collide with date-based episode numbering.
	bareYearPattern = regexp.MustCompile(`(?:^|[ ._-])((?:19|20)\d{2})(?:[ ._-]|$)`)

	// trailingBracketPattern matches a trailing bracketed or parenthesized
	// group, which Plex documents as ignorable release info.
	trailingBracketPattern = regexp.MustCompile(`[ ._-]*[\[(][^\[\]()]*[\])]\s*$`)

	// seasonEpisodePattern matches SxxExx along with any further episode numbers
	// that follow it, so "S01E01-E02" and "S01E01E02" both resolve to a range.
	seasonEpisodePattern = regexp.MustCompile(`(?i)\bs(\d{1,4})[ ._-]*((?:e\d{1,4}[ ._-]*-?[ ._-]*)+)`)

	// episodeNumberPattern pulls the individual numbers out of the group above.
	episodeNumberPattern = regexp.MustCompile(`(?i)e(\d{1,4})`)

	// crossEpisodePattern matches the alternate "1x05" numbering.
	crossEpisodePattern = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{1,3})\b`)

	// isoDatePattern matches a YYYY-MM-DD air date.
	isoDatePattern = regexp.MustCompile(`\b((?:19|20)\d{2})[ ._-](\d{1,2})[ ._-](\d{1,2})\b`)

	// dayFirstDatePattern matches Plex's DD-MM-YYYY air date form.
	dayFirstDatePattern = regexp.MustCompile(`\b(\d{1,2})[ ._-](\d{1,2})[ ._-]((?:19|20)\d{2})\b`)

	// seasonFolderPattern matches the "Season 01" folder naming both servers
	// require.
	seasonFolderPattern = regexp.MustCompile(`(?i)^season[ ._-]*(\d{1,4})$`)

	// abbreviatedSeasonFolderPattern matches the "S01"/"SE01" shorthand Jellyfin
	// explicitly tells users not to use. It is matched anyway so a library using
	// it still scans, with a warning attached.
	abbreviatedSeasonFolderPattern = regexp.MustCompile(`(?i)^s(?:e|eason)?[ ._-]*(\d{1,4})$`)

	// seasonArtworkPattern matches season artwork kept at the series root, the
	// layout Plex documents: "Season01.jpg", "Season 02-poster.jpg",
	// "season03_banner.jpg".
	//
	// The trailing token is optional but restricted to the artwork words the
	// naming conventions actually use. Accepting any trailing word would sweep
	// up things that merely mention a season — "Season 1 Trailer.mkv" is an
	// extra belonging to the series, not artwork belonging to season one.
	seasonArtworkPattern = regexp.MustCompile(`(?i)^season[ ._-]*(\d{1,4})(?:[ ._-]*` + seasonArtworkTokens + `)?$`)

	// seasonSpecialsArtworkPattern is the specials equivalent, which is season
	// zero.
	seasonSpecialsArtworkPattern = regexp.MustCompile(`(?i)^season[ ._-]*specials(?:[ ._-]*` + seasonArtworkTokens + `)?$`)

	// whitespaceRun collapses the runs left behind by separator substitution.
	whitespaceRun = regexp.MustCompile(`\s{2,}`)
)

// videoNameParts is what a video filename yields once every optional token has
// been peeled off it.
type videoNameParts struct {
	Title        string
	Year         int
	Edition      string
	VersionLabel string
	StackType    string
	StackNumber  int
	ThreeDFormat string
}

// parseVideoName pulls the structured pieces out of a video file's base name
// (no extension).
//
// The order is deliberate and each step consumes from the end of the name, so
// that by the time the title is taken, everything that isn't title has already
// been removed. folderName is the containing item folder, used only to decide
// whether a trailing " - something" is a version label; allowYearWithoutParens
// is off for TV, where a bare four-digit number is far more likely to be part of
// a date-based episode name than a release year.
func parseVideoName(baseName, folderName string, allowYearWithoutParens bool) videoNameParts {
	parts := videoNameParts{}
	working := baseName

	// 1. Stereoscopic flag.
	if match := threeDPattern.FindStringSubmatch(working); match != nil {
		parts.ThreeDFormat = strings.ToLower(match[1])
		working = working[:len(working)-len(match[0])]
	}

	// 2. Multi-part token.
	if match := stackPattern.FindStringSubmatch(working); match != nil {
		if number, err := strconv.Atoi(match[2]); err == nil {
			parts.StackType = strings.ToLower(match[1])
			parts.StackNumber = number
			working = working[:len(working)-len(match[0])]
		}
	}

	// 3. Edition tag.
	if match := editionPattern.FindStringSubmatch(working); match != nil {
		parts.Edition = strings.TrimSpace(match[1])
		working = editionPattern.ReplaceAllString(working, "")
	}

	// 4. Provider id tags: removed from the name, not recorded.
	working = providerIDPattern.ReplaceAllString(working, "")

	// 5. Version label, only where the specification defines one: the file is
	// named for its folder plus " - label".
	working, parts.VersionLabel = splitVersionLabel(working, folderName)

	// 6. Year.
	working, parts.Year = splitYear(working, allowYearWithoutParens)

	// 7. Whatever is left is the title.
	parts.Title = cleanTitle(working)
	return parts
}

// splitVersionLabel separates Jellyfin's multi-version suffix from a name.
//
// The rule is anchored to the folder name on purpose. Treating any trailing
// " - x" as a version label would mangle ordinary titles — "Mission - Impossible"
// would lose half its name — so the label is only recognized when what precedes
// it is the item's own folder name, which is exactly the layout the
// specification describes.
func splitVersionLabel(baseName, folderName string) (remaining, versionLabel string) {
	if folderName == "" {
		return baseName, ""
	}
	separatorIndex := strings.LastIndex(baseName, " - ")
	if separatorIndex <= 0 {
		return baseName, ""
	}

	prefix := baseName[:separatorIndex]
	if !strings.EqualFold(strings.TrimSpace(prefix), strings.TrimSpace(folderName)) {
		return baseName, ""
	}

	label := strings.TrimSpace(baseName[separatorIndex+len(" - "):])
	label = strings.TrimSpace(strings.Trim(label, "[]"))
	if label == "" {
		return baseName, ""
	}
	return prefix, label
}

// splitYear removes the release year from a name and returns it.
func splitYear(baseName string, allowYearWithoutParens bool) (remaining string, year int) {
	// Rightmost parenthesized year wins: a title may legitimately contain one,
	// as in "2012 (2009)".
	if matches := parenthesizedYearPattern.FindAllStringIndex(baseName, -1); matches != nil {
		last := matches[len(matches)-1]
		parsed, err := strconv.Atoi(baseName[last[0]+1 : last[1]-1])
		if err == nil {
			return baseName[:last[0]] + baseName[last[1]:], parsed
		}
	}

	if allowYearWithoutParens {
		if matches := bareYearPattern.FindAllStringSubmatchIndex(baseName, -1); matches != nil {
			last := matches[len(matches)-1]
			// Never treat the whole name as nothing but a year; a movie really
			// can be called "1917".
			if last[0] > 0 {
				parsed, err := strconv.Atoi(baseName[last[2]:last[3]])
				if err == nil {
					return baseName[:last[0]], parsed
				}
			}
		}
	}

	return baseName, 0
}

// cleanTitle turns the leftover of a filename into a readable title: scene
// separators become spaces, trailing release-info groups are dropped, and
// stray punctuation is trimmed.
func cleanTitle(raw string) string {
	title := raw
	for {
		trimmed := trailingBracketPattern.ReplaceAllString(title, "")
		if trimmed == title {
			break
		}
		title = trimmed
	}

	// Dots and underscores stand in for spaces in scene naming. A name already
	// containing spaces is left alone, so titles with meaningful dots
	// ("Mr. Robot") survive.
	if !strings.Contains(title, " ") {
		title = strings.NewReplacer(".", " ", "_", " ").Replace(title)
	}

	title = whitespaceRun.ReplaceAllString(title, " ")
	return strings.Trim(title, " ._-")
}

// episodeInfo is what a TV episode filename yields.
type episodeInfo struct {
	SeasonNumber   *int
	EpisodeNumbers []int
	AirDate        string
	SeriesTitle    string
	EpisodeTitle   string
	Matched        bool
}

// parseEpisodeName resolves the season and episode numbering in a TV filename.
// Patterns are tried in the order the specifications present them and the first
// match wins.
func parseEpisodeName(baseName string) episodeInfo {
	// 1. The canonical SxxExx form, including multi-episode ranges.
	if match := seasonEpisodePattern.FindStringSubmatchIndex(baseName); match != nil {
		full := baseName[match[0]:match[1]]
		season, err := strconv.Atoi(baseName[match[2]:match[3]])
		if err == nil {
			var episodes []int
			for _, numberMatch := range episodeNumberPattern.FindAllStringSubmatch(baseName[match[4]:match[5]], -1) {
				if number, err := strconv.Atoi(numberMatch[1]); err == nil {
					episodes = append(episodes, number)
				}
			}
			if len(episodes) > 0 {
				episodes = expandEpisodeRange(episodes, full)
				return episodeInfo{
					SeasonNumber:   &season,
					EpisodeNumbers: episodes,
					SeriesTitle:    cleanTitle(baseName[:match[0]]),
					EpisodeTitle:   cleanTitle(trimLeadingSeparators(baseName[match[1]:])),
					Matched:        true,
				}
			}
		}
	}

	// 2. The alternate "1x05" numbering.
	if match := crossEpisodePattern.FindStringSubmatchIndex(baseName); match != nil {
		season, seasonErr := strconv.Atoi(baseName[match[2]:match[3]])
		episode, episodeErr := strconv.Atoi(baseName[match[4]:match[5]])
		if seasonErr == nil && episodeErr == nil {
			return episodeInfo{
				SeasonNumber:   &season,
				EpisodeNumbers: []int{episode},
				SeriesTitle:    cleanTitle(baseName[:match[0]]),
				EpisodeTitle:   cleanTitle(trimLeadingSeparators(baseName[match[1]:])),
				Matched:        true,
			}
		}
	}

	// 3. Date-based naming, which some shows use instead of numbering.
	if info, ok := parseAirDate(baseName); ok {
		return info
	}

	return episodeInfo{}
}

// expandEpisodeRange fills in the episodes between the endpoints of a hyphenated
// range, so "S01E01-E03" covers episodes 1, 2 and 3 rather than just the two
// numbers written down.
func expandEpisodeRange(episodes []int, matched string) []int {
	if len(episodes) != 2 || !strings.Contains(matched, "-") {
		return episodes
	}
	first, last := episodes[0], episodes[1]
	if last <= first || last-first > 32 {
		return episodes
	}
	expanded := make([]int, 0, last-first+1)
	for number := first; number <= last; number++ {
		expanded = append(expanded, number)
	}
	return expanded
}

// parseAirDate recognizes the two date-based episode naming forms. Component
// ranges are validated so an arbitrary run of digits isn't mistaken for a date.
func parseAirDate(baseName string) (episodeInfo, bool) {
	if match := isoDatePattern.FindStringSubmatchIndex(baseName); match != nil {
		year, _ := strconv.Atoi(baseName[match[2]:match[3]])
		month, _ := strconv.Atoi(baseName[match[4]:match[5]])
		day, _ := strconv.Atoi(baseName[match[6]:match[7]])
		if isPlausibleDate(month, day) {
			return dateEpisodeInfo(baseName, match[0], match[1], year, month, day), true
		}
	}

	if match := dayFirstDatePattern.FindStringSubmatchIndex(baseName); match != nil {
		day, _ := strconv.Atoi(baseName[match[2]:match[3]])
		month, _ := strconv.Atoi(baseName[match[4]:match[5]])
		year, _ := strconv.Atoi(baseName[match[6]:match[7]])
		if isPlausibleDate(month, day) {
			return dateEpisodeInfo(baseName, match[0], match[1], year, month, day), true
		}
	}

	return episodeInfo{}, false
}

func dateEpisodeInfo(baseName string, start, end, year, month, day int) episodeInfo {
	return episodeInfo{
		AirDate:      fmt.Sprintf("%04d-%02d-%02d", year, month, day),
		SeriesTitle:  cleanTitle(baseName[:start]),
		EpisodeTitle: cleanTitle(trimLeadingSeparators(baseName[end:])),
		Matched:      true,
	}
}

func isPlausibleDate(month, day int) bool {
	return month >= 1 && month <= 12 && day >= 1 && day <= 31
}

// trimLeadingSeparators removes the punctuation that introduces an episode
// title, so "S01E01 - Pilot" yields "Pilot".
func trimLeadingSeparators(raw string) string {
	return strings.TrimLeft(raw, " ._-")
}

// parseSeasonFolder resolves a season folder name to its number. abbreviated
// reports the "S01" shorthand Jellyfin forbids, so the caller can warn while
// still scanning the folder.
func parseSeasonFolder(folderName string) (seasonNumber int, abbreviated, ok bool) {
	if strings.EqualFold(folderName, "specials") {
		return 0, false, true
	}
	if match := seasonFolderPattern.FindStringSubmatch(folderName); match != nil {
		number, err := strconv.Atoi(match[1])
		return number, false, err == nil
	}
	if match := abbreviatedSeasonFolderPattern.FindStringSubmatch(folderName); match != nil {
		number, err := strconv.Atoi(match[1])
		return number, true, err == nil
	}
	return 0, false, false
}

// parseSeasonArtworkName reads the season a sidecar names in its own filename.
//
// Both servers accept season artwork kept beside the series rather than inside
// the season folder, which means the file's position says nothing about which
// season it belongs to — the name is the only thing that does.
func parseSeasonArtworkName(baseName string) (seasonNumber int, ok bool) {
	if seasonSpecialsArtworkPattern.MatchString(baseName) {
		return 0, true
	}
	if match := seasonArtworkPattern.FindStringSubmatch(baseName); match != nil {
		number, err := strconv.Atoi(match[1])
		return number, err == nil
	}
	return 0, false
}

// extrasSuffixType reports whether a video's base name ends in one of the extras
// suffixes, which keeps it out of the media file records.
func extrasSuffixType(baseName string) (string, bool) {
	lowered := strings.ToLower(baseName)
	for stem, extraType := range extrasSuffixTypes {
		for _, separator := range []string{"-", ".", "_", " "} {
			if separator == " " && !spaceSeparableExtrasSuffixes[stem] {
				continue
			}
			if strings.HasSuffix(lowered, separator+stem) {
				return extraType, true
			}
		}
	}
	return "", false
}

// parseArtistAndTitle splits the "Artist - Title" convention used for music
// video filenames.
func parseArtistAndTitle(baseName string) (artist, title string) {
	separatorIndex := strings.Index(baseName, " - ")
	if separatorIndex <= 0 {
		return "", cleanTitle(baseName)
	}
	artist = strings.TrimSpace(baseName[:separatorIndex])
	title = cleanTitle(strings.TrimSpace(baseName[separatorIndex+len(" - "):]))
	return artist, title
}
