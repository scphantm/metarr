package mediascan

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"Metarr/internal/agent/nfo"
	"Metarr/internal/shared/metadata"
	"Metarr/internal/shared/scanmodel"
)

// Scan walks directoryPath recursively and classifies everything under it
// according to the naming conventions for directoryType, parsing any .nfo
// sidecars it finds.
//
// An error is returned only when the directory itself cannot be read. Anything
// ambiguous about an individual file becomes a warning on the result instead,
// because a single oddly named file in a large library must not fail the scan
// that covers it.
func Scan(directoryPath string, directoryType scanmodel.DirectoryType) (*scanmodel.ScanResult, error) {
	absolutePath, err := filepath.Abs(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("mediascan: resolving %s: %w", directoryPath, err)
	}

	info, err := os.Stat(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("mediascan: reading %s: %w", absolutePath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("mediascan: %s is not a directory", absolutePath)
	}

	scanner := &directoryScanner{
		rootPath:           absolutePath,
		folderName:         filepath.Base(absolutePath),
		directoryType:      directoryType,
		contexts:           map[string]folderContext{".": {}},
		mediaBaseNames:     map[string]map[string]int{},
		mediaRelativePaths: map[string]bool{},
		seasonSidecars:     map[int][]*scanmodel.SidecarFile{},
	}

	if err := scanner.walk(); err != nil {
		return nil, err
	}
	scanner.classifySidecars()
	return scanner.assemble(), nil
}

// folderContext is what a folder's position in the tree implies about the files
// inside it. It is inherited down the tree: a "Behind The Scenes" folder inside
// "Season 01" is both a season folder and an extras folder.
type folderContext struct {
	seasonNumber *int
	seasonFolder string
	extraType    string
	isDiscFolder bool
	trickplay    *trickplayContext
}

// trickplayContext is what a .trickplay folder says about the files beneath it:
// which media file they preview, and — once the resolution subfolder is reached
// — at what size. The tiles are named 0.jpg, 1.jpg and so on, so position in the
// tree is the only thing that identifies them as previews at all.
type trickplayContext struct {
	ownerFolder string // relative path of the folder holding the media file
	baseName    string // the media file's base name, taken from the folder name
	width       int    // zero until the resolution subfolder is entered
	tileWidth   int
	tileHeight  int
}

// walkedFile is one regular file found during the walk, before classification.
type walkedFile struct {
	relativePath string
	folderPath   string // relative path of the containing folder, "." at the root
	fileName     string
	baseName     string
	extension    string
	class        fileClass
	sizeBytes    int64
	modifiedAt   time.Time
	stat         *scanmodel.FileStat
	context      folderContext
}

// mediaCandidate is a video identified as playable content rather than an extra,
// together with the sidecars that were matched to it.
type mediaCandidate struct {
	file       walkedFile
	video      *metadata.VideoAttributes
	sidecars   []*scanmodel.SidecarFile
	nfoAttrs   *metadata.Metadata
	episodeIDs []*metadata.Link
	warnings   []string
}

type directoryScanner struct {
	rootPath      string
	folderName    string
	directoryType scanmodel.DirectoryType

	contexts map[string]folderContext
	files    []walkedFile

	// mediaBaseNames indexes, per containing folder, the base name of each media
	// candidate to its position in mediaCandidates. Sidecars are matched within
	// their own folder only.
	mediaBaseNames  map[string]map[string]int
	mediaCandidates []mediaCandidate

	// mediaRelativePaths identifies the media files themselves. Base name is not
	// enough to tell a media file from its own sidecar — "Movie.mkv" and
	// "Movie.nfo" share one — so identity is tracked by path.
	mediaRelativePaths map[string]bool

	// directorySidecars holds the non-media files that belong to the item as a
	// whole; seasonSidecars holds those scoped to one season, keyed by season
	// number. Files belonging to a specific media file live on that file's
	// candidate instead.
	directorySidecars []*scanmodel.SidecarFile
	seasonSidecars    map[int][]*scanmodel.SidecarFile

	// ownerNames resolves the uids and gids the walk collects into names, once
	// per distinct id rather than once per file.
	ownerNames ownerNameCache

	// trickplayOrphansWarned keys the trickplay folders already reported as
	// naming a media file that isn't there, so a stale folder warns once rather
	// than once per tile inside it.
	trickplayOrphansWarned map[string]bool

	externalLinks []*metadata.Link
	linkSeen      map[string]bool
	// tvshowMeta holds the series' own tvshow.nfo metadata, once one is found at
	// directory scope. It is set at most once per scan: later matches (a stray
	// duplicate, or the accidental re-discovery of the same scope) are skipped
	// rather than overwriting it. A season.nfo never reaches here even though it
	// shares the <tvshow> root element, because absorbNFO forces its scope to
	// ScopeSeason before this check runs.
	tvshowMeta *metadata.Metadata
	warnings   []string
}

// walk is pass one: collect every file worth looking at, resolve each folder's
// context, and decide which videos are media files.
func (s *directoryScanner) walk() error {
	walkErr := filepath.WalkDir(s.rootPath, func(currentPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subtree is worth reporting but not worth abandoning
			// the rest of the directory over.
			s.warnf("could not read %s: %v", s.relativeTo(currentPath), err)
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		relativePath := s.relativeTo(currentPath)
		if relativePath == "." {
			return nil
		}

		if entry.IsDir() {
			if isIgnoredFolder(entry.Name()) {
				return fs.SkipDir
			}
			s.contexts[relativePath] = s.folderContextFor(relativePath, entry.Name())
			return nil
		}

		if isIgnoredFile(entry.Name()) {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			s.warnf("could not stat %s: %v", relativePath, err)
			return nil
		}

		folderPath := path.Dir(relativePath)
		fileName := entry.Name()
		extension := strings.ToLower(strings.TrimPrefix(filepath.Ext(fileName), "."))

		// The walk already lstat'd this entry to produce info, so the full stat
		// record costs nothing beyond reading fields the scan would otherwise
		// discard.
		fileStat := fileStatFrom(info)
		s.ownerNames.resolve(fileStat)

		s.files = append(s.files, walkedFile{
			relativePath: relativePath,
			folderPath:   folderPath,
			fileName:     fileName,
			baseName:     strings.TrimSuffix(fileName, filepath.Ext(fileName)),
			extension:    extension,
			class:        classifyExtension(extension),
			sizeBytes:    info.Size(),
			modifiedAt:   info.ModTime().UTC(),
			stat:         fileStat,
			context:      s.contexts[folderPath],
		})
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("mediascan: walking %s: %w", s.rootPath, walkErr)
	}

	s.identifyMediaFiles()
	return nil
}

// folderContextFor derives a folder's context by inheriting its parent's and
// layering on whatever this folder's own name means.
func (s *directoryScanner) folderContextFor(relativePath, folderName string) folderContext {
	context := s.contexts[path.Dir(relativePath)]

	if context.seasonNumber == nil {
		if seasonNumber, abbreviated, ok := parseSeasonFolder(folderName); ok {
			number := seasonNumber
			context.seasonNumber = &number
			context.seasonFolder = folderName
			if abbreviated {
				s.warnf("season folder %q uses the abbreviated form; %q is the documented naming", relativePath, fmt.Sprintf("Season %02d", seasonNumber))
			}
		}
	}
	if context.extraType == "" {
		if extraType, ok := extrasFolderType(folderName); ok {
			context.extraType = extraType
		}
	}
	if isDiscStructureFolder(folderName) {
		context.isDiscFolder = true
	}

	// The trickplay layout is two folders deep — "Movie.trickplay/320 - 10x10" —
	// and the context is built across both, the outer folder naming the media
	// file and the inner one giving the size. Inheritance above is what carries
	// the outer folder's answer down to the inner one.
	if baseName, isTrickplay := trickplayFolderBaseName(folderName); isTrickplay {
		context.trickplay = &trickplayContext{
			ownerFolder: path.Dir(relativePath),
			baseName:    baseName,
		}
	} else if context.trickplay != nil && context.trickplay.width == 0 {
		if width, tileWidth, tileHeight, ok := parseTrickplayResolutionFolder(folderName); ok {
			// Copied rather than mutated: the parent's context is shared with
			// every sibling resolution folder, and each holds a different size.
			resolution := *context.trickplay
			resolution.width = width
			resolution.tileWidth = tileWidth
			resolution.tileHeight = tileHeight
			context.trickplay = &resolution
		}
	}
	return context
}

// identifyMediaFiles decides, for every video file found, whether it is playable
// content or an extra. Only the former become media file records.
func (s *directoryScanner) identifyMediaFiles() {
	for _, file := range s.files {
		if file.class != classVideo || file.context.isDiscFolder {
			continue
		}
		// A video inside an extras folder is an extra regardless of its name.
		if file.context.extraType != "" {
			continue
		}
		if _, isExtra := extrasSuffixType(file.baseName); isExtra {
			continue
		}

		candidate := mediaCandidate{file: file, video: s.videoAttributesFor(file, true)}
		s.mediaCandidates = append(s.mediaCandidates, candidate)
		s.mediaRelativePaths[file.relativePath] = true

		if s.mediaBaseNames[file.folderPath] == nil {
			s.mediaBaseNames[file.folderPath] = map[string]int{}
		}
		s.mediaBaseNames[file.folderPath][file.baseName] = len(s.mediaCandidates) - 1
	}
}

// videoAttributesFor parses a video file's name according to the directory's
// type.
//
// isMediaFile gates the unresolved-numbering warnings. Extras are expected to
// have no season or episode number — a trailer or a featurette simply isn't an
// episode — so warning about them would bury the genuine cases under noise from
// every extras folder in the library.
func (s *directoryScanner) videoAttributesFor(file walkedFile, isMediaFile bool) *metadata.VideoAttributes {
	// For TV, a bare four-digit number is much more likely to belong to a
	// date-based episode name than to be a release year.
	allowBareYear := s.directoryType != scanmodel.TypeTV
	parts := parseVideoName(file.baseName, s.folderName, allowBareYear)

	attributes := &metadata.VideoAttributes{
		Title:        parts.Title,
		Year:         int32(parts.Year),
		Edition:      parts.Edition,
		VersionLabel: parts.VersionLabel,
		StackType:    parts.StackType,
		StackNumber:  int32(parts.StackNumber),
		ThreeDFormat: parts.ThreeDFormat,
	}

	// Music video names are free-form, since no metadata is fetched for them;
	// the "Artist - Title" convention is the one structure worth reading.
	if s.directoryType == scanmodel.TypeMusicVideo {
		if _, title := parseArtistAndTitle(file.baseName); title != "" {
			attributes.Title = title
		}
		return attributes
	}

	if s.directoryType != scanmodel.TypeTV {
		return attributes
	}

	episode := parseEpisodeName(file.baseName)
	switch {
	case episode.Matched:
		attributes.SeasonNumber = int32PtrFromInt(episode.SeasonNumber)
		attributes.EpisodeNumbers = int32SliceFromInt(episode.EpisodeNumbers)
		// The name parser works in strings, since a date in a filename is only a
		// date once it has been validated; the record stores the parsed value.
		attributes.AirDate = metadata.ParseDate(episode.AirDate)
		attributes.EpisodeTitle = episode.EpisodeTitle
		if episode.SeriesTitle != "" {
			attributes.Title = episode.SeriesTitle
		}
	case file.context.seasonNumber != nil:
		// The season is known from the folder even though the filename carries
		// no recognizable episode number.
		attributes.SeasonNumber = int32PtrFromInt(file.context.seasonNumber)
		if isMediaFile {
			s.warnf("could not resolve an episode number from %q; recording it under season %d only", file.relativePath, *file.context.seasonNumber)
		}
	default:
		if isMediaFile {
			s.warnf("could not resolve season or episode numbering from %q", file.relativePath)
		}
	}

	// Fall back to the folder's season when the filename omitted one.
	if attributes.SeasonNumber == nil && file.context.seasonNumber != nil {
		attributes.SeasonNumber = int32PtrFromInt(file.context.seasonNumber)
	}
	if attributes.SeasonNumber != nil && *attributes.SeasonNumber == 0 {
		attributes.IsSpecial = true
	}
	return attributes
}

// classifySidecars is pass two: everything that is not a media file becomes a
// sidecar record, filed against whichever of the three owners it belongs to.
//
// The owner is decided by name first: a file whose name begins with a media
// file's base name in the same folder belongs to that media file. Matching is by
// longest prefix, which is what makes dotted names work —
// "Movie.Name.2019.1080p.mkv" and its "Movie.Name.2019.1080p.en.forced.srt"
// cannot be related by splitting on "." alone.
//
// Failing that, a file may name its own season: both servers accept season
// artwork kept beside the series rather than inside the season folder, so
// "Season01-poster.jpg" at the series root belongs to season 1 even though its
// position says nothing. That is checked before the folder, so a file is always
// read on its own terms first; ordinary artwork inside "Season 01" names no
// season and still takes its owner from where it sits.
//
// Trickplay tiles are the exception to all of it: they sit two folders below the
// video and are named for nothing but their position in the sequence, so their
// owner comes from the folder that names it and is resolved before any of the
// above is consulted.
func (s *directoryScanner) classifySidecars() {
	for _, file := range s.files {
		// A media file is a record in its own right, never its own sidecar.
		if s.mediaRelativePaths[file.relativePath] {
			continue
		}

		sidecar := s.sidecarFor(file)

		// An NFO is read for its contents as well as recorded, and which record
		// its metadata lands on depends on the same prefix match, so the two
		// decisions are made together.
		if file.class == classNFO {
			s.absorbNFO(file)
		}

		if index, owned := s.trickplayOwnerOf(file); owned {
			s.mediaCandidates[index].sidecars = append(s.mediaCandidates[index].sidecars, sidecar)
			continue
		}

		if targetBaseName, _, matched := s.matchMediaPrefix(file.folderPath, file.baseName); matched {
			index := s.mediaBaseNames[file.folderPath][targetBaseName]
			s.mediaCandidates[index].sidecars = append(s.mediaCandidates[index].sidecars, sidecar)
			continue
		}

		if seasonNumber, named := s.seasonNamedBy(file); named {
			s.seasonSidecars[seasonNumber] = append(s.seasonSidecars[seasonNumber], sidecar)
			continue
		}

		if file.context.seasonNumber != nil {
			seasonNumber := *file.context.seasonNumber
			s.seasonSidecars[seasonNumber] = append(s.seasonSidecars[seasonNumber], sidecar)
			continue
		}

		s.directorySidecars = append(s.directorySidecars, sidecar)
	}
}

// seasonNamedBy reports the season a sidecar names in its own filename, which is
// how season artwork kept at the series root finds its season.
//
// Only a series has seasons, so this is not consulted for a movie or music video
// directory: a stray file called "season01.jpg" beside a film should not conjure
// a season for it to belong to.
func (s *directoryScanner) seasonNamedBy(file walkedFile) (int, bool) {
	if s.directoryType != scanmodel.TypeTV {
		return 0, false
	}
	return parseSeasonArtworkName(file.baseName)
}

// trickplayOwnerOf resolves the media file a trickplay tile previews, returning
// its index among the candidates.
//
// The owner is named by the folder rather than found by the usual prefix match:
// a tile lives two folders below the video, and matchMediaPrefix only ever looks
// inside a file's own folder.
//
// A folder naming a video that is no longer there — what an upgraded media file
// leaves behind — owns nothing, and its tiles fall through to the directory. The
// warning is issued once for the folder rather than once per tile, since a stale
// folder holds dozens of them and would otherwise bury every other warning.
func (s *directoryScanner) trickplayOwnerOf(file walkedFile) (int, bool) {
	trickplay := file.context.trickplay
	if trickplay == nil {
		return 0, false
	}

	index, found := s.mediaBaseNames[trickplay.ownerFolder][trickplay.baseName]
	if found {
		return index, true
	}

	if s.trickplayOrphansWarned == nil {
		s.trickplayOrphansWarned = map[string]bool{}
	}
	orphanKey := path.Join(trickplay.ownerFolder, trickplay.baseName)
	if !s.trickplayOrphansWarned[orphanKey] {
		s.trickplayOrphansWarned[orphanKey] = true
		s.warnf("trickplay previews for %q have no media file of that name; recording them on the directory", orphanKey+trickplayFolderSuffix)
	}
	return 0, false
}

// sidecarFor builds the record for one non-media file, naming it from folder
// context first and the configured table second.
//
// Context wins because a file's position says things its name cannot: a video
// called "clip1.mkv" inside "Behind The Scenes/" is a behind-the-scenes extra,
// and no pattern matched against its name could know that.
func (s *directoryScanner) sidecarFor(file walkedFile) *scanmodel.SidecarFile {
	sidecarType := s.sidecarTypeFor(file)

	return &scanmodel.SidecarFile{
		RelativePath: file.relativePath,
		FileName:     file.fileName,
		Extension:    file.extension,
		SizeBytes:    file.sizeBytes,
		ModifiedAt:   timestamppb.New(file.modifiedAt),
		Type:         string(sidecarType),
		Category:     string(scanmodel.CategoryOf(sidecarType)),
		Stat:         file.stat,
		Image:        s.imageInfoFor(file),
		Trickplay:    trickplayInfoFor(file),
	}
}

// trickplayInfoFor places a tile within its media file's previews, returning nil
// for a file that is not one.
//
// A resolution of zero means the file sits directly in the trickplay folder
// rather than in one of its "320 - 10x10" subfolders. Jellyfin puts nothing
// there, so rather than record a preview at no resolution the file is left
// without a trickplay block — it is still typed as trickplay by its position.
func trickplayInfoFor(file walkedFile) *scanmodel.TrickplayInfo {
	trickplay := file.context.trickplay
	if trickplay == nil || trickplay.width == 0 {
		return nil
	}

	info := &scanmodel.TrickplayInfo{
		Width:      int32(trickplay.width),
		TileWidth:  int32(trickplay.tileWidth),
		TileHeight: int32(trickplay.tileHeight),
	}
	if tileIndex, err := strconv.Atoi(file.baseName); err == nil {
		index := int32(tileIndex)
		info.TileIndex = &index
	}
	return info
}

// imageInfoFor reads the header of an artwork sidecar, returning nil for a file
// that is not artwork at all.
//
// The gate is the extension class rather than the sidecar's category, matching
// how classNFO gates absorbNFO: category comes from the configurable table, so a
// user-defined type filed under "image" could otherwise point the decoder at a
// text file.
func (s *directoryScanner) imageInfoFor(file walkedFile) *scanmodel.ImageInfo {
	if file.class != classImage {
		return nil
	}

	imageInfo, err := readImageInfo(filepath.Join(s.rootPath, filepath.FromSlash(file.relativePath)))
	if err != nil {
		// An unreadable image is recorded, never fatal, the same way a malformed
		// NFO is: the error travels on the record so the file stays
		// distinguishable from one that was never examined.
		s.warnf("could not read the image header of %q: %v", file.relativePath, err)
		return &scanmodel.ImageInfo{Error: err.Error()}
	}
	return imageInfo
}

// sidecarTypeFor resolves what kind of sidecar a file is, in decreasing order of
// how much the answer can be trusted.
//
// Every context-derived answer is checked against the registry before it is
// accepted. These paths name a type without consulting the table, so without the
// check a type someone had switched off would still be assigned by them and
// "disabled" would not mean disabled.
func (s *directoryScanner) sidecarTypeFor(file walkedFile) scanmodel.SidecarType {
	registry := scanmodel.ActiveSidecarRegistry()

	// Everything beneath a VIDEO_TS or BDMV tree describes the disc, whatever it
	// is called and whatever its extension.
	if file.context.isDiscFolder && registry.IsEnabled(scanmodel.SidecarDiscStructure) {
		return scanmodel.SidecarDiscStructure
	}

	// Same reasoning for a trickplay folder: a tile is called "0.jpg", and no
	// pattern matched against that name could tell it from any other numbered
	// image.
	if file.context.trickplay != nil && registry.IsEnabled(scanmodel.SidecarTrickplay) {
		return scanmodel.SidecarTrickplay
	}

	if file.context.extraType != "" {
		if sidecarType, ok := extraTypeToSidecarType[file.context.extraType]; ok && registry.IsEnabled(sidecarType) {
			return sidecarType
		}
	}

	// The extras suffixes are a naming rule the servers document, so they are
	// trusted ahead of the configurable patterns — but still only for a type the
	// table has switched on.
	if file.class == classVideo {
		if extraType, ok := extrasSuffixType(file.baseName); ok {
			if sidecarType, ok := extraTypeToSidecarType[extraType]; ok && registry.IsEnabled(sidecarType) {
				return sidecarType
			}
		}
	}

	if definition, matched := registry.Match(file.fileName); matched {
		return definition.Type
	}

	return scanmodel.SidecarUnknown
}

// absorbNFO reads an NFO sidecar's contents into whichever record it describes.
// The file's own sidecar record is produced by sidecarFor; this is only about
// where the parsed metadata and the provider ids inside it end up.
func (s *directoryScanner) absorbNFO(file walkedFile) {
	attributes, err := nfo.ReadFile(filepath.Join(s.rootPath, filepath.FromSlash(file.relativePath)))
	if err != nil {
		// A malformed sidecar is recorded, never fatal: the rest of the library
		// is still worth indexing.
		attributes = &metadata.Metadata{ParseError: err.Error()}
		s.warnf("could not parse %q: %v", file.relativePath, err)
	}

	targetBaseName, _, matched := s.matchMediaPrefix(file.folderPath, file.baseName)
	if matched {
		attributes.Scope = metadata.ScopeVideo
		attributes.TargetBaseName = targetBaseName

		index := s.mediaBaseNames[file.folderPath][targetBaseName]
		s.mediaCandidates[index].nfoAttrs = attributes
		// The sidecar's own external ids belong to the episode, not the series.
		s.mediaCandidates[index].episodeLinks(metadata.ExtractLinks(attributes))
		return
	}

	// A directory- or season-level sidecar. Its ids describe the item as a
	// whole, so they feed the directory's external links.
	if strings.EqualFold(file.baseName, "season") && file.context.seasonNumber != nil {
		attributes.Scope = metadata.ScopeSeason
	} else {
		attributes.Scope = metadata.ScopeDirectory
	}
	// The series' own tvshow.nfo, captured once so it becomes the directory's
	// metadata in assemble. A season.nfo never reaches here: its scope was
	// forced to ScopeSeason above.
	if attributes.Scope == metadata.ScopeDirectory && attributes.Kind == metadata.KindTVShow && s.tvshowMeta == nil {
		s.tvshowMeta = attributes
	}
	s.addExternalLinks(metadata.ExtractLinks(attributes))
}

// matchMediaPrefix finds the media file in the same folder whose base name is
// the longest prefix of baseName, returning the remainder as the sidecar's
// suffix. Longest wins so that "Show S01E01" is preferred over a hypothetical
// shorter "Show S01E0" when both exist.
func (s *directoryScanner) matchMediaPrefix(folderPath, baseName string) (targetBaseName, suffix string, matched bool) {
	candidates := s.mediaBaseNames[folderPath]
	if len(candidates) == 0 {
		return "", "", false
	}

	best := ""
	for candidate := range candidates {
		if len(candidate) <= len(best) {
			continue
		}
		if candidate == baseName || strings.HasPrefix(baseName, candidate) {
			best = candidate
		}
	}
	if best == "" {
		return "", "", false
	}
	return best, baseName[len(best):], true
}

// addExternalLinks merges item-level provider ids, suppressing duplicates across
// the several NFO files a directory may hold.
func (s *directoryScanner) addExternalLinks(links []*metadata.Link) {
	if s.linkSeen == nil {
		s.linkSeen = map[string]bool{}
	}
	for _, link := range links {
		key := link.Key + "\x00" + link.Value
		if s.linkSeen[key] {
			continue
		}
		s.linkSeen[key] = true
		s.externalLinks = append(s.externalLinks, link)
	}
}

// episodeLinks attaches an episode's own provider ids to its media file.
func (c *mediaCandidate) episodeLinks(links []*metadata.Link) {
	c.episodeIDs = append(c.episodeIDs, links...)
}

// int32PtrFromInt narrows an optional int from the name parser to the int32
// the model stores.
func int32PtrFromInt(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

// int32SliceFromInt narrows a slice of ints from the name parser.
func int32SliceFromInt(in []int) []int32 {
	if in == nil {
		return nil
	}
	out := make([]int32, len(in))
	for i, v := range in {
		out[i] = int32(v)
	}
	return out
}

// nonNilLinks makes an empty link list encode as an array rather than null, so a
// caller reading external_links never has to distinguish "no ids" from "field
// absent".
func nonNilLinks(links []*metadata.Link) []*metadata.Link {
	if links == nil {
		return []*metadata.Link{}
	}
	return links
}

// assemble is pass three: turn the collected pieces into the records that get
// stored.
func (s *directoryScanner) assemble() *scanmodel.ScanResult {
	scannedAt := time.Now().UTC()

	directory := &scanmodel.TVSeries{
		RecordType: string(scanmodel.RecordTypeTVSeries),
		Path:       s.rootPath,
		Type:       string(s.directoryType),
		FolderName: s.folderName,

		ScannedAt: timestamppb.New(scannedAt),
		Seasons:   s.assembleSeasons(),
		Sidecars:  s.directorySidecars,
		Warnings:  s.warnings,
	}
	if directory.Sidecars == nil {
		directory.Sidecars = []*scanmodel.SidecarFile{}
	}

	directory.Metadata = s.directoryMetadata()

	mediaFiles := make([]*scanmodel.MediaFile, 0, len(s.mediaCandidates))
	for _, candidate := range s.mediaCandidates {
		sidecars := candidate.sidecars
		if sidecars == nil {
			sidecars = []*scanmodel.SidecarFile{}
		}
		// An episode's provider ids describe the episode, so they live on its own
		// metadata record rather than beside it. Assigning here rather than in
		// absorbNFO keeps the accumulating behaviour: several sidecars may match
		// one media file, and the ids from all of them belong to it even though
		// only the last one's parsed contents are kept.
		if candidate.nfoAttrs != nil {
			candidate.nfoAttrs.ExternalLinks = nonNilLinks(candidate.episodeIDs)
		}

		mediaFile := &scanmodel.MediaFile{
			RecordType:    string(scanmodel.RecordTypeMediaFile),
			DirectoryPath: s.rootPath,
			Type:          string(s.directoryType),
			Path:          filepath.Join(s.rootPath, filepath.FromSlash(candidate.file.relativePath)),
			RelativePath:  candidate.file.relativePath,
			FileName:      candidate.file.fileName,
			Extension:     candidate.file.extension,
			SizeBytes:     candidate.file.sizeBytes,
			ModifiedAt:    timestamppb.New(candidate.file.modifiedAt),
			ScannedAt:     timestamppb.New(scannedAt),
			Video:         candidate.video,
			Stat:          candidate.file.stat,
			Metadata:      candidate.nfoAttrs,
			Sidecars:      sidecars,
			Warnings:      candidate.warnings,
		}
		mediaFiles = append(mediaFiles, mediaFile)
	}

	if len(mediaFiles) == 0 {
		directory.Warnings = append(directory.Warnings, s.noMediaFilesWarning())
	}

	return &scanmodel.ScanResult{Directory: directory, MediaFiles: mediaFiles}
}

// directoryMetadata builds the series' own metadata record. It starts from the
// tvshow.nfo when the directory has one, falls back to what the folder name
// reveals for the title and year, and indexes the seasons found on disk.
func (s *directoryScanner) directoryMetadata() *metadata.Metadata {
	md := s.tvshowMeta
	if md == nil {
		md = &metadata.Metadata{}
	}
	md.Scope = metadata.ScopeDirectory

	// The ids gathered from every directory- and season-level sidecar describe
	// the item as a whole, so they belong to its metadata record. They are
	// assigned rather than merged: s.externalLinks is already the deduplicated
	// union of every NFO the scan read, including the tvshow.nfo this record may
	// have started from.
	md.ExternalLinks = nonNilLinks(s.externalLinks)

	folderParts := parseVideoName(s.folderName, "", true)
	if md.Title == "" {
		md.Title = folderParts.Title
	}
	if md.Year == 0 {
		md.Year = int32(folderParts.Year)
	}

	if s.directoryType == scanmodel.TypeTV {
		if seasons := s.summarizeSeasons(); len(seasons) > 0 {
			if md.TvShow == nil {
				md.TvShow = &metadata.TVShowFields{}
			}
			md.TvShow.Seasons = seasons
		}
	}
	return md
}

// seasonNumbers maps every season the scan found evidence of to the folder that
// declared it. It is the shared source for both season views: the scanmodel.TVSeason
// records on the directory, and the SeasonSummary index inside its metadata.
//
// Evidence is a folder on disk or a sidecar naming the season. A season known
// only from artwork carries an empty folder name — it is a real season, it just
// has no directory of its own, and recording it is what keeps its artwork from
// being dropped.
func (s *directoryScanner) seasonNumbers() map[int]string {
	folderNames := map[int]string{}
	for _, context := range s.contexts {
		if context.seasonNumber != nil && context.seasonFolder != "" {
			folderNames[*context.seasonNumber] = context.seasonFolder
		}
	}
	for seasonNumber := range s.seasonSidecars {
		if _, known := folderNames[seasonNumber]; !known {
			folderNames[seasonNumber] = ""
		}
	}
	return folderNames
}

// sortedSeasonNumbers returns the season numbers in folderNames in ascending
// order, so both season views are ordered the same way.
func sortedSeasonNumbers(folderNames map[int]string) []int {
	numbers := make([]int, 0, len(folderNames))
	for number := range folderNames {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	return numbers
}

// assembleSeasons builds the season records, each carrying the sidecars scoped
// to it. A season folder holding nothing but episodes still produces a record:
// the season exists on disk, and saying so is the point of the field.
func (s *directoryScanner) assembleSeasons() []*scanmodel.TVSeason {
	folderNames := s.seasonNumbers()
	if len(folderNames) == 0 {
		return nil
	}

	seasons := make([]*scanmodel.TVSeason, 0, len(folderNames))
	for _, number := range sortedSeasonNumbers(folderNames) {
		sidecars := s.seasonSidecars[number]
		if sidecars == nil {
			sidecars = []*scanmodel.SidecarFile{}
		}
		seasons = append(seasons, &scanmodel.TVSeason{
			SeasonNumber: int32(number),
			FolderName:   folderNames[number],
			Sidecars:     sidecars,
		})
	}
	return seasons
}

// summarizeSeasons indexes the seasons present on disk from the folder layout,
// ordered by season number. This is the view that ends up inside the directory's
// metadata and so feeds NFO writing, as distinct from the scanmodel.TVSeason records that
// describe what the scan actually found.
func (s *directoryScanner) summarizeSeasons() []*metadata.SeasonSummary {
	folderNames := s.seasonNumbers()
	if len(folderNames) == 0 {
		return nil
	}

	seasons := make([]*metadata.SeasonSummary, 0, len(folderNames))
	for _, number := range sortedSeasonNumbers(folderNames) {
		seasons = append(seasons, &metadata.SeasonSummary{
			SeasonNumber: int32(number),
			FolderName:   folderNames[number],
		})
	}
	return seasons
}

// noMediaFilesWarning explains why a directory produced no media file records.
//
// A ripped disc is the common benign case: the content is all there, just as
// disc structure rather than a single playable file, so saying "no movie file
// found" would send someone looking for a problem that isn't one.
func (s *directoryScanner) noMediaFilesWarning() string {
	if s.holdsDiscStructure() {
		return "no playable file found; this directory holds a ripped disc (VIDEO_TS or BDMV) rather than a single media file"
	}

	switch s.directoryType {
	case scanmodel.TypeMovie:
		return "no movie file found in this directory"
	case scanmodel.TypeTV:
		return "no episode files found in this directory"
	case scanmodel.TypeMusicVideo:
		return "no music video files found in this directory"
	default:
		return "no media files found in this directory"
	}
}

// holdsDiscStructure reports whether the scan found any part of a ripped disc,
// looking at both the directory's own sidecars and its seasons'.
func (s *directoryScanner) holdsDiscStructure() bool {
	for _, sidecar := range s.directorySidecars {
		if sidecar.Type == scanmodel.SidecarDiscStructure {
			return true
		}
	}
	for _, sidecars := range s.seasonSidecars {
		for _, sidecar := range sidecars {
			if sidecar.Type == scanmodel.SidecarDiscStructure {
				return true
			}
		}
	}
	return false
}

// relativeTo renders a path relative to the scan root using forward slashes, so
// stored paths are stable across platforms.
func (s *directoryScanner) relativeTo(currentPath string) string {
	relativePath, err := filepath.Rel(s.rootPath, currentPath)
	if err != nil {
		return currentPath
	}
	return filepath.ToSlash(relativePath)
}

func (s *directoryScanner) warnf(format string, args ...any) {
	s.warnings = append(s.warnings, fmt.Sprintf(format, args...))
}
