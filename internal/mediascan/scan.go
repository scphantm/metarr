package mediascan

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"Metarr/internal/nfo"
)

// Scan walks directoryPath recursively and classifies everything under it
// according to the naming conventions for directoryType, parsing any .nfo
// sidecars it finds.
//
// An error is returned only when the directory itself cannot be read. Anything
// ambiguous about an individual file becomes a warning on the result instead,
// because a single oddly named file in a large library must not fail the scan
// that covers it.
func Scan(directoryPath string, directoryType DirectoryType) (*ScanResult, error) {
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
		rootPath:       absolutePath,
		folderName:     filepath.Base(absolutePath),
		directoryType:  directoryType,
		contexts:       map[string]folderContext{".": {}},
		mediaBaseNames: map[string]map[string]int{},
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
	context      folderContext
}

// mediaCandidate is a video identified as playable content rather than an extra,
// together with the sidecars that were matched to it.
type mediaCandidate struct {
	file       walkedFile
	video      VideoAttributes
	subtitles  []DirectoryFile
	images     []DirectoryFile
	nfoAttrs   *NFOAttributes
	episodeIDs []nfo.Link
	warnings   []string
}

type directoryScanner struct {
	rootPath      string
	folderName    string
	directoryType DirectoryType

	contexts map[string]folderContext
	files    []walkedFile

	// mediaBaseNames indexes, per containing folder, the base name of each media
	// candidate to its position in mediaCandidates. Sidecars are matched within
	// their own folder only.
	mediaBaseNames  map[string]map[string]int
	mediaCandidates []mediaCandidate

	directoryFiles []DirectoryFile
	externalLinks  []nfo.Link
	linkSeen       map[nfo.Link]bool
	warnings       []string
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

		s.files = append(s.files, walkedFile{
			relativePath: relativePath,
			folderPath:   folderPath,
			fileName:     fileName,
			baseName:     strings.TrimSuffix(fileName, filepath.Ext(fileName)),
			extension:    extension,
			class:        classifyExtension(extension),
			sizeBytes:    info.Size(),
			modifiedAt:   info.ModTime().UTC(),
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
func (s *directoryScanner) videoAttributesFor(file walkedFile, isMediaFile bool) VideoAttributes {
	// For TV, a bare four-digit number is much more likely to belong to a
	// date-based episode name than to be a release year.
	allowBareYear := s.directoryType != TypeTV
	parts := parseVideoName(file.baseName, s.folderName, allowBareYear)

	attributes := VideoAttributes{
		Title:        parts.Title,
		Year:         parts.Year,
		Edition:      parts.Edition,
		VersionLabel: parts.VersionLabel,
		StackType:    parts.StackType,
		StackNumber:  parts.StackNumber,
		ThreeDFormat: parts.ThreeDFormat,
	}

	// Music video names are free-form, since no metadata is fetched for them;
	// the "Artist - Title" convention is the one structure worth reading.
	if s.directoryType == TypeMusicVideo {
		if _, title := parseArtistAndTitle(file.baseName); title != "" {
			attributes.Title = title
		}
		return attributes
	}

	if s.directoryType != TypeTV {
		return attributes
	}

	episode := parseEpisodeName(file.baseName)
	switch {
	case episode.Matched:
		attributes.SeasonNumber = episode.SeasonNumber
		attributes.EpisodeNumbers = episode.EpisodeNumbers
		attributes.AirDate = episode.AirDate
		attributes.EpisodeTitle = episode.EpisodeTitle
		if episode.SeriesTitle != "" {
			attributes.Title = episode.SeriesTitle
		}
	case file.context.seasonNumber != nil:
		// The season is known from the folder even though the filename carries
		// no recognizable episode number.
		attributes.SeasonNumber = file.context.seasonNumber
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
		attributes.SeasonNumber = file.context.seasonNumber
	}
	if attributes.SeasonNumber != nil && *attributes.SeasonNumber == 0 {
		attributes.IsSpecial = true
	}
	return attributes
}

// classifySidecars is pass two: everything that is not a media file gets a role,
// and files that belong to a particular media file are attached to it.
//
// Matching is by longest prefix against the media file base names in the same
// folder, which is what makes dotted names work: "Movie.Name.2019.1080p.mkv"
// and its "Movie.Name.2019.1080p.en.forced.srt" cannot be related by splitting
// on "." alone.
func (s *directoryScanner) classifySidecars() {
	for _, file := range s.files {
		switch {
		case file.context.isDiscFolder:
			s.addDirectoryFile(file, RoleDiscStructure, nil)

		case file.class == classVideo:
			s.classifyExtraVideo(file)

		case file.class == classSubtitle:
			s.classifySubtitle(file)

		case file.class == classImage:
			s.classifyImage(file)

		case file.class == classNFO:
			s.classifyNFO(file)

		case file.class == classAudio:
			s.classifyAudio(file)

		default:
			s.addDirectoryFile(file, RoleUnknown, nil)
		}
	}
}

// classifyExtraVideo records a video that is not playable content. Media files
// were already taken in pass one, so any video reaching here is an extra.
func (s *directoryScanner) classifyExtraVideo(file walkedFile) {
	if _, isMedia := s.mediaIndexFor(file.folderPath, file.baseName); isMedia {
		return
	}

	attributes := s.videoAttributesFor(file, false)
	// Numbering means nothing on an extra and would only be misleading.
	attributes.SeasonNumber = nil
	attributes.EpisodeNumbers = nil
	attributes.IsSpecial = false

	if file.context.extraType != "" {
		attributes.ExtraType = file.context.extraType
		attributes.ExtraSource = ExtraSourceFolder
	} else if extraType, ok := extrasSuffixType(file.baseName); ok {
		attributes.ExtraType = extraType
		attributes.ExtraSource = ExtraSourceSuffix
	}

	s.addDirectoryFile(file, RoleExtraVideo, &attributes)
}

func (s *directoryScanner) classifySubtitle(file walkedFile) {
	targetBaseName, suffix, matched := s.matchMediaPrefix(file.folderPath, file.baseName)

	attributes := parseSidecarSuffix(suffix)
	attributes.TargetBaseName = targetBaseName

	directoryFile := s.newDirectoryFile(file, RoleSubtitle, nil)
	directoryFile.Subtitle = &attributes

	if matched {
		index := s.mediaBaseNames[file.folderPath][targetBaseName]
		s.mediaCandidates[index].subtitles = append(s.mediaCandidates[index].subtitles, directoryFile)
		return
	}

	// No media file to attach to; keep it on the directory rather than losing it.
	s.directoryFiles = append(s.directoryFiles, directoryFile)
}

func (s *directoryScanner) classifyImage(file walkedFile) {
	// A per-video image is named for its media file plus an image token, e.g.
	// "Series S01E01-thumb.jpg".
	if targetBaseName, suffix, matched := s.matchMediaPrefix(file.folderPath, file.baseName); matched {
		imageType, index := parseImageSuffix(suffix)
		if imageType == "" {
			imageType = ImageThumb
		}
		attributes := ImageAttributes{
			ImageType:      imageType,
			Index:          index,
			Scope:          ScopeVideo,
			TargetBaseName: targetBaseName,
		}
		directoryFile := s.newDirectoryFile(file, RoleImage, nil)
		directoryFile.Image = &attributes

		mediaIndex := s.mediaBaseNames[file.folderPath][targetBaseName]
		s.mediaCandidates[mediaIndex].images = append(s.mediaCandidates[mediaIndex].images, directoryFile)
		return
	}

	info, recognized := parseImageName(file.baseName)
	if !recognized {
		// Artwork in the backdrops folder needs no recognizable name.
		if file.context.extraType == ExtraBackdrop {
			info = imageNameInfo{ImageType: ImageBackdrop}
		} else {
			s.addDirectoryFile(file, RoleUnknown, nil)
			return
		}
	}

	attributes := ImageAttributes{
		ImageType:    info.ImageType,
		Index:        info.Index,
		Scope:        ScopeDirectory,
		SeasonNumber: info.SeasonNumber,
	}
	// A season folder, or Plex's series-root season poster naming, scopes the
	// artwork to a season.
	if info.SeasonNumber != nil {
		attributes.Scope = ScopeSeason
	} else if file.context.seasonNumber != nil {
		attributes.Scope = ScopeSeason
		attributes.SeasonNumber = file.context.seasonNumber
	}

	directoryFile := s.newDirectoryFile(file, RoleImage, nil)
	directoryFile.Image = &attributes
	s.directoryFiles = append(s.directoryFiles, directoryFile)
}

// parseImageSuffix reads the image token off a per-video artwork suffix such as
// "-thumb" or "-fanart2".
func parseImageSuffix(suffix string) (imageType string, index int) {
	trimmed := strings.Trim(suffix, " ._-")
	if trimmed == "" {
		return "", 0
	}
	info, ok := parseImageName(trimmed)
	if !ok {
		return "", 0
	}
	return info.ImageType, info.Index
}

func (s *directoryScanner) classifyNFO(file walkedFile) {
	attributes := NFOAttributes{}

	document, err := nfo.ReadFile(filepath.Join(s.rootPath, filepath.FromSlash(file.relativePath)))
	if err != nil {
		// A malformed sidecar is recorded, never fatal: the rest of the library
		// is still worth indexing.
		attributes.ParseError = err.Error()
		s.warnf("could not parse %q: %v", file.relativePath, err)
	} else {
		attributes.Kind = document.Kind
		attributes.Movie = document.Movie
		attributes.TVShow = document.TVShow
		attributes.Episodes = document.Episodes
		attributes.MusicVideo = document.MusicVideo
	}

	targetBaseName, _, matched := s.matchMediaPrefix(file.folderPath, file.baseName)
	if matched {
		attributes.Scope = ScopeVideo
		attributes.TargetBaseName = targetBaseName
		attributes.SeasonNumber = file.context.seasonNumber

		directoryFile := s.newDirectoryFile(file, RoleNFO, nil)
		directoryFile.NFO = &attributes

		index := s.mediaBaseNames[file.folderPath][targetBaseName]
		s.mediaCandidates[index].nfoAttrs = &attributes
		// The sidecar's own external ids belong to the episode, not the series.
		if document != nil {
			s.mediaCandidates[index].episodeLinks(nfo.ExtractLinks(document))
		}
		return
	}

	// A directory- or season-level sidecar. Its ids describe the item as a
	// whole, so they feed the directory's external links.
	if strings.EqualFold(file.baseName, "season") && file.context.seasonNumber != nil {
		attributes.Scope = ScopeSeason
		attributes.SeasonNumber = file.context.seasonNumber
	} else {
		attributes.Scope = ScopeDirectory
	}
	s.addExternalLinks(nfo.ExtractLinks(document))

	directoryFile := s.newDirectoryFile(file, RoleNFO, nil)
	directoryFile.NFO = &attributes
	s.directoryFiles = append(s.directoryFiles, directoryFile)
}

func (s *directoryScanner) classifyAudio(file walkedFile) {
	if file.context.extraType == ExtraThemeMusic || strings.EqualFold(file.baseName, "theme") {
		s.addDirectoryFile(file, RoleThemeMusic, nil)
		return
	}
	s.addDirectoryFile(file, RoleUnknown, nil)
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

// mediaIndexFor reports whether a folder holds a media file with this exact base
// name.
func (s *directoryScanner) mediaIndexFor(folderPath, baseName string) (int, bool) {
	index, ok := s.mediaBaseNames[folderPath][baseName]
	return index, ok
}

func (s *directoryScanner) newDirectoryFile(file walkedFile, role FileRole, video *VideoAttributes) DirectoryFile {
	return DirectoryFile{
		RelativePath: file.relativePath,
		FileName:     file.fileName,
		Extension:    file.extension,
		SizeBytes:    file.sizeBytes,
		ModifiedAt:   file.modifiedAt,
		Role:         role,
		Video:        video,
	}
}

func (s *directoryScanner) addDirectoryFile(file walkedFile, role FileRole, video *VideoAttributes) {
	s.directoryFiles = append(s.directoryFiles, s.newDirectoryFile(file, role, video))
}

// addExternalLinks merges item-level provider ids, suppressing duplicates across
// the several NFO files a directory may hold.
func (s *directoryScanner) addExternalLinks(links []nfo.Link) {
	if s.linkSeen == nil {
		s.linkSeen = map[nfo.Link]bool{}
	}
	for _, link := range links {
		if s.linkSeen[link] {
			continue
		}
		s.linkSeen[link] = true
		s.externalLinks = append(s.externalLinks, link)
	}
}

// episodeLinks attaches an episode's own provider ids to its media file.
func (c *mediaCandidate) episodeLinks(links []nfo.Link) {
	c.episodeIDs = append(c.episodeIDs, links...)
}

// assemble is pass three: turn the collected pieces into the records that get
// stored.
func (s *directoryScanner) assemble() *ScanResult {
	scannedAt := time.Now().UTC()

	directory := &LocalDirectory{
		RecordType:    RecordTypeDirectory,
		Path:          s.rootPath,
		Type:          s.directoryType,
		FolderName:    s.folderName,
		ExternalLinks: s.externalLinks,
		ScannedAt:     scannedAt,
		Files:         s.directoryFiles,
		Warnings:      s.warnings,
	}
	if directory.ExternalLinks == nil {
		directory.ExternalLinks = []nfo.Link{}
	}
	if directory.Files == nil {
		directory.Files = []DirectoryFile{}
	}

	folderParts := parseVideoName(s.folderName, "", true)
	directory.Title = folderParts.Title
	directory.Year = folderParts.Year

	mediaFiles := make([]MediaFile, 0, len(s.mediaCandidates))
	for _, candidate := range s.mediaCandidates {
		video := candidate.video
		mediaFile := MediaFile{
			RecordType:    RecordTypeMediaFile,
			DirectoryPath: s.rootPath,
			Type:          s.directoryType,
			Path:          filepath.Join(s.rootPath, filepath.FromSlash(candidate.file.relativePath)),
			RelativePath:  candidate.file.relativePath,
			FileName:      candidate.file.fileName,
			Extension:     candidate.file.extension,
			SizeBytes:     candidate.file.sizeBytes,
			ModifiedAt:    candidate.file.modifiedAt,
			ScannedAt:     scannedAt,
			Video:         &video,
			NFO:           candidate.nfoAttrs,
			Subtitles:     candidate.subtitles,
			Images:        candidate.images,
			EpisodeIDs:    candidate.episodeIDs,
			Warnings:      candidate.warnings,
		}
		mediaFiles = append(mediaFiles, mediaFile)
	}

	switch s.directoryType {
	case TypeTV:
		directory.Seasons = summarizeSeasons(s.contexts, mediaFiles)
	case TypeMusicVideo:
		directory.Artist = s.deriveArtist(mediaFiles)
	}

	if len(mediaFiles) == 0 {
		directory.Warnings = append(directory.Warnings, s.noMediaFilesWarning())
	}

	return &ScanResult{Directory: directory, MediaFiles: mediaFiles}
}

// noMediaFilesWarning explains why a directory produced no media file records.
//
// A ripped disc is the common benign case: the content is all there, just as
// disc structure rather than a single playable file, so saying "no movie file
// found" would send someone looking for a problem that isn't one.
func (s *directoryScanner) noMediaFilesWarning() string {
	for _, file := range s.directoryFiles {
		if file.Role == RoleDiscStructure {
			return "no playable file found; this directory holds a ripped disc (VIDEO_TS or BDMV) rather than a single media file"
		}
	}

	switch s.directoryType {
	case TypeMovie:
		return "no movie file found in this directory"
	case TypeTV:
		return "no episode files found in this directory"
	case TypeMusicVideo:
		return "no music video files found in this directory"
	default:
		return "no media files found in this directory"
	}
}

// summarizeSeasons builds the queryable season index from the media files that
// resolved a season number.
func summarizeSeasons(contexts map[string]folderContext, mediaFiles []MediaFile) []SeasonSummary {
	counts := map[int]int{}
	for _, mediaFile := range mediaFiles {
		if mediaFile.Video == nil || mediaFile.Video.SeasonNumber == nil {
			continue
		}
		counts[*mediaFile.Video.SeasonNumber]++
	}
	if len(counts) == 0 {
		return nil
	}

	folderNames := map[int]string{}
	for _, context := range contexts {
		if context.seasonNumber != nil && context.seasonFolder != "" {
			folderNames[*context.seasonNumber] = context.seasonFolder
		}
	}

	summaries := make([]SeasonSummary, 0, len(counts))
	for seasonNumber, count := range counts {
		summaries = append(summaries, SeasonSummary{
			SeasonNumber: seasonNumber,
			FolderName:   folderNames[seasonNumber],
			EpisodeCount: count,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].SeasonNumber < summaries[j].SeasonNumber
	})
	return summaries
}

// deriveArtist resolves a music video directory's artist: from an "Artist - Title"
// filename where present, otherwise from the folder name, which is the
// convention when videos are grouped per artist.
func (s *directoryScanner) deriveArtist(mediaFiles []MediaFile) string {
	for _, mediaFile := range mediaFiles {
		baseName := strings.TrimSuffix(mediaFile.FileName, filepath.Ext(mediaFile.FileName))
		if artist, _ := parseArtistAndTitle(baseName); artist != "" {
			return artist
		}
	}
	return cleanTitle(s.folderName)
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
